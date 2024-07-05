use ark_ec_vrfs::suites::bandersnatch::edwards as bandersnatch;
use ark_ec_vrfs::{prelude::ark_serialize, suites::bandersnatch::edwards::RingContext};
use bandersnatch::{IetfProof, Input, Output, Public, RingProof, Secret};

use ark_serialize::{CanonicalDeserialize, CanonicalSerialize};

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
        println!("ring_context loaded from {}", filename);
        let mut file = File::open(filename).unwrap();
        let mut buf = Vec::new();
        file.read_to_end(&mut buf).unwrap();
        let pcs_params = PcsParams::deserialize_uncompressed_unchecked(&mut &buf[..]).unwrap();
        RingContext::from_srs(pcs_params, RING_SIZE).unwrap()
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
pub struct Prover {
    pub prover_idx: usize,
    pub secret: Secret,
    pub ring: Vec<Public>,
}

impl Prover {
    pub fn new(ring: Vec<Public>, prover_idx: usize) -> Self {
        Self {
            prover_idx,
            secret: Secret::from_seed(&prover_idx.to_le_bytes()),
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
pub struct Verifier {
    pub commitment: RingCommitment,
    pub ring: Vec<Public>,
}

impl Verifier {
    pub fn new(ring: Vec<Public>) -> Self {
        // Backend currently requires the wrapped type (plain affine points)
        let pts: Vec<_> = ring.iter().map(|pk| pk.0).collect();
        let verifier_key = ring_context().verifier_key(&pts);
        let commitment = verifier_key.commitment();
        Self { ring, commitment }
    }

    /// Anonymous VRF signature verification.
    ///
    /// Used for tickets verification.
    pub fn ring_vrf_verify(
        &self,
        vrf_input_data: &[u8],
        aux_data: &[u8],
        signature: &[u8],
    ) -> bool {
        use ark_ec_vrfs::ring::prelude::fflonk::pcs::PcsParams;
        use ark_ec_vrfs::ring::Verifier as _;
        use bandersnatch::VerifierKey;

        let signature = RingVrfSignature::deserialize_compressed(signature).unwrap();

        let input = vrf_input_point(vrf_input_data);
        let output = signature.output;

        let ring_ctx = ring_context();

        // The verifier key is reconstructed from the commitment and the constant
        // verifier key component of the SRS in order to verify some proof.
        // As an alternative we can construct the verifier key using the
        // RingContext::verifier_key() method, but is more expensive.
        // In other words, we prefer computing the commitment once, when the keyset changes.
        let verifier_key = VerifierKey::from_commitment_and_kzg_vk(
            self.commitment.clone(),
            ring_ctx.pcs_params.raw_vk(),
        );
        let verifier = ring_ctx.verifier(verifier_key);
        let result = Public::verify(input, output, aux_data, &signature.proof, &verifier).is_ok();

        // Print vrf_input_data, aux_data, and signature in hex form
        //println!("vrf_input_data: {}", hex::encode(vrf_input_data));
        //println!("aux_data: {}", hex::encode(aux_data));

        let mut signature_buf = Vec::new();
        signature.serialize_compressed(&mut signature_buf).unwrap();
        //println!("signature: {}", hex::encode(signature_buf));

        if !result {
            println!("Ring signature verification failure");
            return result;
        }
        //println!("Ring signature verified");

        // This truncated hash is the actual value used as ticket-id/score
        //println!(" vrf-output-hash: {}", hex::encode(&output.hash()[..32]));
        result
    }

    /// Non-Anonymous VRF signature verification.
    ///
    /// Used for ticket claim verification during block import.
    /// Not used with Safrole test vectors.
    pub fn ietf_vrf_verify(
        &self,
        vrf_input_data: &[u8],
        aux_data: &[u8],
        signature: &[u8],
        signer_key_index: usize,
    ) -> bool {
        use ark_ec_vrfs::ietf::Verifier as _;

        let signature = IetfVrfSignature::deserialize_compressed(signature).unwrap();

        let input = vrf_input_point(vrf_input_data);
        let output = signature.output;

        let public = &self.ring[signer_key_index];
        let result = public
            .verify(input, output, aux_data, &signature.proof)
            .is_ok();
        if !result {
            println!("Ring signature verification failure");
            return result;
        }
        println!("Ietf signature verified");

        // This is the actual value used as ticket-id/score
        // NOTE: as far as vrf_input_data is the same, this matches the one produced
        // using the ring-vrf (regardless of aux_data).
        println!(" vrf-output-hash: {}", hex::encode(&output.hash()[..32]));
        result
    }
}

use std::os::raw::{c_int, c_uchar};
use std::slice;

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
/*    println!(
        "pubkeys_length {} | signature_hex_len {} | vrf_input_data_len {} | aux_data_len {}\n",
        pubkeys_length, signature_hex_len, vrf_input_data_len, aux_data_len
    ); */
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

    // Assuming each pubkey is 32 bytes, split the pubkeys slice into individual pubkeys
    let mut ring_set: Vec<Public> = Vec::new();
    for i in 0..(pubkeys_length / 32) {
        let pubkey_bytes = &pubkeys_slice[i * 32..(i + 1) * 32];
        let public = Public::deserialize_compressed(pubkey_bytes).expect("Deserialization failed");
        ring_set.push(public);
    }

    // Create the Verifier
    let verifier = Verifier::new(ring_set);
    // Perform the verification
    let res = verifier.ring_vrf_verify(vrf_input_data_slice, aux_data_slice, signature_slice);

    // Store the VRF output hash
    if res {
        let signature = RingVrfSignature::deserialize_compressed(signature_slice).unwrap();
        let output = signature.output;
        let vrf_output_hash = &output.hash()[..32];
        unsafe {
            std::ptr::copy_nonoverlapping(vrf_output_hash.as_ptr(), vrf_output, 32);
        }
    }

    // Return the result
    if res {
        1
    } else {
        0
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_vrf_signatures() {
        let ring_set: Vec<_> = (0..RING_SIZE)
            .map(|i| Secret::from_seed(&i.to_le_bytes()).public())
            .collect();
        let prover_key_index = 3;

        let prover = Prover::new(ring_set.clone(), prover_key_index);
        let verifier = Verifier::new(ring_set);

        let vrf_input_data = b"foo";

        //--- Anonymous VRF

        let aux_data = b"bar";

        // Prover signs some data.
        let ring_signature = prover.ring_vrf_sign(vrf_input_data, aux_data);

        // Verifier checks it without knowing who is the signer.
        let res = verifier.ring_vrf_verify(vrf_input_data, aux_data, &ring_signature);
        assert!(res);

        //--- Non anonymous VRF

        let other_aux_data = b"hello";

        // Prover signs the same vrf-input data (we want the output to match)
        // But different aux data.
        let ietf_signature = prover.ietf_vrf_sign(vrf_input_data, other_aux_data);

        // Verifier checks the signature knowing the signer identity.
        let res = verifier.ietf_vrf_verify(
            vrf_input_data,
            other_aux_data,
            &ietf_signature,
            prover_key_index,
        );
        assert!(res);
    }

    #[test]
    fn test_ring_vrf_verify_external() {
        let ring_set_hex = [
            "1c8407d352ad3242986c77753920cba2c8985eb6df57fa0c38d8a8ba053aff56",
            "f01c64a4e57dbf0296d6b276b87672a46f310d8f56114e073389fea58d783b51",
            "1132440f6ca1ef5ff92c40b0f725cc073e0a1c7082e3c678cda301c148b8ca13",
            "7518285dfdb55145d235f129b81192cd491abedbe1b1393c4592d6ff7a01d015",
            "3d8ed07ffd95a84e92fd4941500289bb86816db418cc6076946f200f5c57b62a",
            "5ec02b561d1e8156769929defb4f72aed0812c61c3d16696a5cc4e80cab12fe9",
        ];

        let pubkeys_bytes: Vec<u8> = ring_set_hex
            .iter()
            .flat_map(|hex_str| hex::decode(hex_str).expect("Decoding hex string failed"))
            .collect();

        let vrf_input_data = hex::decode("666f6f").expect("Decoding hex string failed");
        let aux_data = hex::decode("626172").expect("Decoding hex string failed");

        let signature_hex = "3d31c3db69741dbeef0b29aeb1f9370d4bbe2365c18586510db8a640101aa42266709f8da7d09a212d5282804e3b824fa8c0c1ffd13a65633ad16f5dab2002bde335816fb40de9cf2ee723ee7621da69b73676d727bc0a28828a08984620f0d625e5c5bd6eb407733c04877094b35c32a9293bc73637cd3da19a00410b74a6cdccbb1be20256cce76f68db1e1e386119870b8bcbc4ac0969f332715754d49009879ad324888180e0064831edc42201301477554180585eb0cf1199de7b512f02b907a350299c589b7ffef560f9c8e161dca8a254ab5879db14ef3e06d034c8caec51d71d9106d515eb412e7b2d0ab81398293fae82eaef776a6e81524316cb814fec990db23be61b5dd9d4729d2708cbc5dbe6a1aa3917e93235e1f7eecfb185acc2bb425b1b735a2b90a12a9a6f60edd10b7e58295a9ac0b487ac3972b49b5cb2764745fc5af1b3212b1491d96f3c06acfb444c49a8af32ec1a0552aa7f4a5de4e06db2bb9233c5ba5818e4d78c50556b7130d8185d47074f0c147248f1ef382dab5155b65529ee967607a4534f99ee1d7a3d8005234eff9eda1252e649ea1144ce7bb4e8196f216442e6142325d4f4787f07ce092dcbd67faf87e05e27e25b059bdb218d034e941ae577aae21da1dcab44e515e9fd232717b94bace237a8377b3c865f9e61d116bde1b610ae64734fda63ced0f6eba9dc04bb5940b80ffb590f6097c7d954a128336910f8ae447189e567bb235e71d9b3a1daa131b95e554254902ffa07f4d65f84b22ddb71aabac0fc72a9b7e0506626f67fada248e42a3ecc1c8397283c2eee0bb8233b22988e0cee4564d3684928b0bc454e5842bba571b840f85ee13ea9bb4fc2e8ff64437c0c2163882da1faf8a2091e8937c4ab199cdb2a12ef9665ce91c7372cc29ac6eec33b6c3e69b9be09ca4702cd247043e6dc42be86d68d16f78a9bb0f532930cb82392d4d6ae7b1ea843889647f71f8cbea54a96c32d51be02df833d135981c9613d32512aa4f5f339d9c454c642a382f802b35cdbc0fd6179b9d7bf3786d72c7bbc602d64a535534c0b778b6fc6038f2849d16682ae6c0a73f50356fdfaccec68c4";
        let signature_bytes = hex::decode(signature_hex).expect("Decoding hex string failed");

        let mut vrf_output = [0u8; 32];

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
}
