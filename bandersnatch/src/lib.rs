use ark_ec_vrfs::suites::bandersnatch::edwards as bandersnatch;
use ark_ec_vrfs::{prelude::ark_serialize, suites::bandersnatch::edwards::RingContext};
use ark_serialize::{CanonicalDeserialize, CanonicalSerialize};
use bandersnatch::{IetfProof, Input, Output, Public, RingProof, Secret};

const RING_SIZE: usize = 6;

// This is the IETF `Prove` procedure output as described in section 2.2
// of the Bandersnatch VRFs specification
#[derive(CanonicalSerialize, CanonicalDeserialize)]
struct IetfVrfSignature {
    output: Output,
    proof: IetfProof,
}

// This is the IETF `Prove` procedure output as described in section 4.2
// of the Bandersnatch VRFs specification
#[derive(CanonicalSerialize, CanonicalDeserialize)]
struct RingVrfSignature {
    output: Output,
    // This contains both the Pedersen proof and actual ring proof.
    proof: RingProof,
}

// "Static" ring context data
fn ring_context() -> &'static RingContext {
    use std::sync::OnceLock;
    static RING_CTX: OnceLock<RingContext> = OnceLock::new();
    RING_CTX.get_or_init(|| {
        use bandersnatch::PcsParams;
        use std::{fs::File, io::Read};
        let manifest_dir =
            std::env::var("CARGO_MANIFEST_DIR").expect("CARGO_MANIFEST_DIR is not set");
        let filename = format!("{}/data/zcash-srs-2-11-uncompressed.bin", manifest_dir);
        let mut file = File::open(filename).unwrap();
        let mut buf = Vec::new();
        file.read_to_end(&mut buf).unwrap();
        let pcs_params = PcsParams::deserialize_uncompressed_unchecked(&mut &buf[..]).unwrap();
        RingContext::from_srs(RING_SIZE, pcs_params).unwrap()
    })
}

// Construct VRF Input Point from arbitrary data (section 1.2)
fn vrf_input_point(vrf_input_data: &[u8]) -> Input {
    let point =
        <bandersnatch::BandersnatchSha512Ell2 as ark_ec_vrfs::Suite>::data_to_point(vrf_input_data)
            .unwrap();
    Input::from(point)
}

// Prover actor.
struct Prover {
    pub prover_idx: usize,
    pub secret: Secret,
    pub ring: Vec<Public>,
}

impl Prover {
    pub fn new(ring: Vec<Public>, prover_idx: usize, seed: &[u8]) -> Self {
        Self {
            prover_idx,
            secret: Secret::from_seed(seed), //Check: seed is being hash again before being turned into ScalarField. Not sure if this is corret
            ring,
        }
    }

    /// Anonymous VRF signature.
    ///
    /// Used for tickets submission.
    pub fn ring_vrf_sign(&self, vrf_input_data: &[u8], aux_data: &[u8]) -> Vec<u8> {
        use ark_ec_vrfs::ring::Prover as _;

        let input = vrf_input_point(vrf_input_data);
        let output = self.secret.output(input);

        // Backend currently requires the wrapped type (plain affine points)
        let pts: Vec<_> = self.ring.iter().map(|pk| pk.0).collect();

        // Proof construction
        let ring_ctx = ring_context();
        let prover_key = ring_ctx.prover_key(&pts);
        let prover = ring_ctx.prover(prover_key, self.prover_idx);
        let proof = self.secret.prove(input, output, aux_data, &prover);

        // Output and Ring Proof bundled together (as per section 2.2)
        let signature = RingVrfSignature { output, proof };
        let mut buf = Vec::new();
        signature.serialize_compressed(&mut buf).unwrap();
        buf
    }

    /// Non-Anonymous VRF signature.
    ///
    /// Used for ticket claiming during block production.
    /// Not used with Safrole test vectors.
    pub fn ietf_vrf_sign(&self, vrf_input_data: &[u8], aux_data: &[u8]) -> Vec<u8> {
        use ark_ec_vrfs::ietf::Prover as _;

        let input = vrf_input_point(vrf_input_data);
        let output = self.secret.output(input);

        let proof = self.secret.prove(input, output, aux_data);

        // Output and IETF Proof bundled together (as per section 2.2)
        let signature = IetfVrfSignature { output, proof };
        let mut buf = Vec::new();
        signature.serialize_compressed(&mut buf).unwrap();
        buf
    }
}

type RingCommitment = ark_ec_vrfs::ring::RingCommitment<bandersnatch::BandersnatchSha512Ell2>;

// Verifier actor.
struct Verifier {
    pub commitment: RingCommitment,
    pub ring: Vec<Public>,
}

impl Verifier {
    fn new(ring: Vec<Public>) -> Self {
        // Backend currently requires the wrapped type (plain affine points)
        let pts: Vec<_> = ring.iter().map(|pk| pk.0).collect();
        let verifier_key = ring_context().verifier_key(&pts);
        let commitment = verifier_key.commitment();
        Self { ring, commitment }
    }

    /// Anonymous VRF signature verification.
    ///
    /// Used for tickets verification.
    ///
    /// On success returns the VRF output hash.
    pub fn ring_vrf_verify(
        &self,
        vrf_input_data: &[u8],
        aux_data: &[u8],
        signature: &[u8],
    ) -> Result<[u8; 32], ()> {
        use ark_ec_vrfs::ring::Verifier as _;

        // Gracefully handle invalid signature
        let signature = match RingVrfSignature::deserialize_compressed(signature) {
            Ok(sig) => sig,
            Err(e) => {
                println!("Failed to deserialize signature: {:?}", e);
                return Err(());
            }
        };

        let input = vrf_input_point(vrf_input_data);
        let output = signature.output;

        let ring_ctx = ring_context();

        let verifier_key = ring_ctx.verifier_key_from_commitment(self.commitment.clone());
        let verifier = ring_ctx.verifier(verifier_key);
        if Public::verify(input, output, aux_data, &signature.proof, &verifier).is_err() {
            println!("Ring signature verification failure");
            return Err(());
        }
        println!("Ring signature verified");

        let vrf_output_hash: [u8; 32] = output.hash()[..32].try_into().unwrap();
        println!(" vrf-output-hash: {}", hex::encode(vrf_output_hash));
        Ok(vrf_output_hash)
    }

    /// Non-Anonymous VRF signature verification.
    ///
    /// Used for ticket claim verification during block import.
    /// Not used with Safrole test vectors.
    ///
    /// On success returns the VRF output hash.
    pub fn ietf_vrf_verify(
        &self,
        vrf_input_data: &[u8],
        aux_data: &[u8],
        signature: &[u8],
        signer_key_index: usize,
    ) -> Result<[u8; 32], ()> {
        use ark_ec_vrfs::ietf::Verifier as _;

        let signature = IetfVrfSignature::deserialize_compressed(signature).unwrap();

        let input = vrf_input_point(vrf_input_data);
        let output = signature.output;

        let public = &self.ring[signer_key_index];
        if public
            .verify(input, output, aux_data, &signature.proof)
            .is_err()
        {
            println!("Ring signature verification failure");
            return Err(());
        }
        println!("Ietf signature verified");

        // This is the actual value used as ticket-id/score
        // NOTE: as far as vrf_input_data is the same, this matches the one produced
        // using the ring-vrf (regardless of aux_data).
        let vrf_output_hash: [u8; 32] = output.hash()[..32].try_into().unwrap();
        println!(" vrf-output-hash: {}", hex::encode(vrf_output_hash));
        Ok(vrf_output_hash)
    }
}

use std::os::raw::{c_int, c_uchar};
use std::slice;
use hex;

#[no_mangle]
pub extern "C" fn ring_vrf_verify_external(
    pubkeys_bytes: *const c_uchar,
    pubkeys_length: usize,
    signature_bytes: *const c_uchar,
    signature_hex_len: usize,
    vrf_input_data_bytes: *const c_uchar,
    vrf_input_data_len: usize,
    aux_data_bytes: *const c_uchar,
    aux_data_len: usize,
    vrf_output: *mut c_uchar,
) -> c_int {
    println!("pubkeys_length: {}, signature_hex_len: {}, vrf_input_data_len: {}, aux_data_len: {}", pubkeys_length, signature_hex_len, vrf_input_data_len, aux_data_len);

    // Convert input pointers to slices
    let pubkeys_slice = unsafe { slice::from_raw_parts(pubkeys_bytes, pubkeys_length) };
    let signature_slice = unsafe { slice::from_raw_parts(signature_bytes, signature_hex_len) };
    let vrf_input_data_slice =
        unsafe { slice::from_raw_parts(vrf_input_data_bytes, vrf_input_data_len) };
    let aux_data_slice = if aux_data_len == 0 {
        &[]
    } else {
        unsafe { slice::from_raw_parts(aux_data_bytes, aux_data_len) }
    };

    println!("pubkeys_slice: {}", hex::encode(pubkeys_slice));
    println!("signature_slice: {}", hex::encode(signature_slice));
    println!("vrf_input_data_slice: {}", hex::encode(vrf_input_data_slice));
    println!("aux_data_slice: {}", hex::encode(aux_data_slice));

    // Assuming each pubkey is 32 bytes, split the pubkeys slice into individual pubkeys
    let mut ring_set: Vec<Public> = Vec::new();
    for i in 0..(pubkeys_length / 32) {
        let pubkey_bytes = &pubkeys_slice[i * 32..(i + 1) * 32];
        match Public::deserialize_compressed(pubkey_bytes) {
            Ok(public_key) => {
                println!("Deserialized public key {}: {}", i, hex::encode(pubkey_bytes));
                ring_set.push(public_key);
            }
            Err(e) => {
                println!("Deserialization failed for public key {}: {:?}", i, e);
                return 0;
            }
        };
    }

    // Create the Verifier
    let verifier = Verifier::new(ring_set);
    // Perform the verification
    let res = verifier.ring_vrf_verify(vrf_input_data_slice, aux_data_slice, signature_slice);

    // Store the VRF output hash
    match res {
        Ok(vrf_output_hash) => {
            println!("Verification successful");
            println!("VRF output hash: {}", hex::encode(vrf_output_hash));
            unsafe {
                std::ptr::copy_nonoverlapping(vrf_output_hash.as_ptr(), vrf_output, 32);
            }
            1
        }
        Err(e) => {
            println!("Verification failed: {:?}", e);
            0
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use hex;
    use rand::{Rng, SeedableRng};
    use rand::rngs::StdRng;

    macro_rules! measure_time {
        ($func_name:expr, $func_call:expr) => {{
            let start = std::time::Instant::now();
            let result = $func_call;
            let duration = start.elapsed();
            println!("* Time taken by {}: {:?}", $func_name, duration);
            result
        }};
    }

    #[test]
    fn test_ring_vrf_verify_external() {

        let ring_set_hex = [
            "5e465beb01dbafe160ce8216047f2155dd0569f058afd52dcea601025a8d161d",
            "3d5e5a51aab2b048f8686ecd79712a80e3265a114cc73f14bdb2a59233fb66d0",
            "aa2b95f7572875b0d0f186552ae745ba8222fc0b5bd456554bfe51c68938f8bc",
            "7f6190116d118d643a98878e294ccf62b509e214299931aad8ff9764181a4e33",
            "48e5fcdce10e0b64ec4eebd0d9211c7bac2f27ce54bca6f7776ff6fee86ab3e3",
            "f16e5352840afb47e206b5c89f560f2611835855cf2e6ebad1acc9520a72591d",
        ];
        let signature_hex = "b342bf8f6fa69c745daad2e99c92929b1da2b840f67e5e8015ac22dd1076343ea95c5bb4b69c197bfdc1b7d2f484fe455fb19bba7e8d17fcaf309ba5814bf54f3a74d75b408da8d3b99bf07f7cde373e4fd757061b1c99e0aac4847f1e393e892b566c14a7f8643a5d976ced0a18d12e32c660d59c66c271332138269cb0fe9c2462d5b3c1a6e9f5ed330ff0d70f64218010ff337b0b69b531f916c67ec564097cd842306df1b4b44534c95ff4efb73b17a14476057fdf8678683b251dc78b0b94712179345c794b6bd99aa54b564933651aee88c93b648e91a613c87bc3f445fff571452241e03e7d03151600a6ee259051a23086b408adec7c112dd94bd8123cf0bed88fddac46b7f891f34c29f13bf883771725aa234d398b13c39fd2a871894f1b1e2dbc7fffbc9c65c49d1e9fd5ee0da133bef363d4ebebe63de2b50328b5d7e020303499d55c07cae617091e33a1ee72ba1b65f940852e93e2905fdf577adcf62be9c74ebda9af59d3f11bece8996773f392a2b35693a45a5a042d88a3dc816b689fe596762d4ea7c6024da713304f56dc928be6e8048c651766952b6c40d0f48afc067ca7cbd77763a2d4f11e88e16033b3343f39bf519fe734db8a139d148ccead4331817d46cf469befa64ae153b5923869144dfa669da36171c20e1f757ed5231fa5a08827d83f7b478ddfb44c9bceb5c6c920b8761ff1e3edb03de48fb55884351f0ac5a7a1805b9b6c49c0529deb97e994deaf2dfd008825e8704cdc04b621f316b505fde26ab71b31af7becbc1154f9979e43e135d35720b93b367bedbe6c6182bb6ed99051f28a3ad6d348ba5b178e3ea0ec0bb4a03fe36604a9eeb609857f8334d3b4b34867361ed2ff9163acd9a27fa20303abe9fc29f2d6c921a8ee779f7f77d940b48bc4fce70a58eed83a206fb7db4c1c7ebe7658603495bb40f6a581dd9e235ba0583165b1569052f8fb4a3e604f2dd74ad84531c6b96723c867b06b6fdd1c4ba150cf9080aa6bbf44cc29041090973d56913b9dc755960371568ef1cf03f127fe8eca209db5d18829f5bfb5826f98833e3f42472b47fad995a9a8bb0e41a1df45ead20285a8";
        let signature_bytes = hex::decode(signature_hex).expect("Decoding hex string failed");
        let decoded_hex = hex::decode("bb30a42c1e62f0afda5f0a4e8a562f7a13a24cea00ee81917b86b89e801314aa").expect("Decoding hex string failed");

        let mut vrf_input_data = Vec::new();
        vrf_input_data.extend_from_slice(b"jam_ticket_seal");
        vrf_input_data.extend_from_slice(&decoded_hex);
        vrf_input_data.push(1);

        let aux_data = Vec::new();
        let mut vrf_output = [0u8; 32];

        let pubkeys_bytes: Vec<u8> = ring_set_hex
            .iter()
            .flat_map(|hex_str| hex::decode(hex_str).expect("Decoding hex string failed"))
            .collect();

        let result = ring_vrf_verify_external(
            pubkeys_bytes.as_ptr(),
            pubkeys_bytes.len(),
            signature_bytes.as_ptr(),
            signature_bytes.len(),
            vrf_input_data.as_ptr(),
            vrf_input_data.len(),
            aux_data.as_ptr(),
            aux_data.len(),
            vrf_output.as_mut_ptr(),
        );

        assert_eq!(result, 1);
        println!("Verification result: {}", result);
        println!("VRF output hash: {}", hex::encode(vrf_output));
    }

    #[test]
    fn test_sign_and_verify() {

        // Step 1: Generate N seeds
        let mut rng = StdRng::seed_from_u64(42); // Use a fixed seed for reproducibility

        let seeds: Vec<[u8; 32]> = (0..RING_SIZE)
            .map(|_| {
                let mut seed = [0u8; 32];
                rng.fill(&mut seed);
                seed
            })
            .collect();

        // Step 2: Use the seeds to create the ring_set of pubkeys
        let ring_set: Vec<_> = seeds
            .iter()
            .map(|seed| Secret::from_seed(seed).public())
            .collect();

        let prover_key_index = 3;

        // Generate a secret key using a specific seed from the seeds
        let prover_seed = seeds[prover_key_index];
        let prover = Prover::new(ring_set.clone(), prover_key_index, &prover_seed);

        let verifier = Verifier::new(ring_set);

        let vrf_input_data = b"foo";

        //--- Anonymous VRF

        let aux_data = b"bar";

        // Prover signs some data.
        let ring_signature = measure_time! {
            "ring-vrf-sign",
            prover.ring_vrf_sign(vrf_input_data, aux_data)
        };

        // Verifier checks it without knowing who is the signer.
        let ring_vrf_output = measure_time! {
            "ring-vrf-verify",
            verifier.ring_vrf_verify(vrf_input_data, aux_data, &ring_signature).unwrap()
        };

        //--- Non anonymous VRF

        let other_aux_data = b"hello";

        // Prover signs the same vrf-input data (we want the output to match)
        // But different aux data.
        let ietf_signature = measure_time! {
            "ietf-vrf-sign",
            prover.ietf_vrf_sign(vrf_input_data, other_aux_data)
        };

        // Verifier checks the signature knowing the signer identity.
        let ietf_vrf_output = measure_time! {
            "ietf-vrf-verify",
            verifier.ietf_vrf_verify(vrf_input_data, other_aux_data, &ietf_signature, prover_key_index).unwrap()
        };

        // Must match
        assert_eq!(ring_vrf_output, ietf_vrf_output);
    }
}
