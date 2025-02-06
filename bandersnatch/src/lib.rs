use ark_ec_vrfs::suites::bandersnatch::edwards as bandersnatch;
use ark_ec_vrfs::{prelude::ark_serialize, suites::bandersnatch::edwards::RingContext};
use ark_serialize::{CanonicalDeserialize, CanonicalSerialize};
use bandersnatch::{IetfProof, Input, Output, Public, RingProof, Secret, ScalarField};

#[cfg(feature = "tiny")]
const RING_SIZE: usize = 6;
#[cfg(feature = "small")]
const RING_SIZE: usize = 24;
#[cfg(feature = "medium")]
const RING_SIZE: usize = 48;
#[cfg(feature = "large")]
const RING_SIZE: usize = 96;
#[cfg(feature = "2xlarge")]
const RING_SIZE: usize = 384;
#[cfg(feature = "3xlarge")]
const RING_SIZE: usize = 576;
#[cfg(feature = "full")]
const RING_SIZE: usize = 1023;

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
        let filename = format!("{}/zcash-srs-2-11-uncompressed.bin", manifest_dir);
        let mut file = File::open(&filename)
            .unwrap_or_else(|e| panic!("Failed to open file '{}': {}", filename, e));
        let mut buf = Vec::new();
        file.read_to_end(&mut buf).unwrap();
        let pcs_params = PcsParams::deserialize_uncompressed_unchecked(&mut &buf[..]).unwrap();
        RingContext::from_srs(RING_SIZE, pcs_params).unwrap()
    })
}

// Construct VRF Input Point from arbitrary data (section 1.2)
fn vrf_input_point(vrf_input_data: &[u8]) -> Option<Input> {
    let point = match <bandersnatch::BandersnatchSha512Ell2 as ark_ec_vrfs::Suite>::data_to_point(vrf_input_data) {
        Some(point) => point,
        None => {
            eprintln!("Failed to convert VRF input data to point");
            return None;
        }
    };
    Some(Input::from(point))
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
    pub fn ring_vrf_sign(&self, vrf_input_data: &[u8], aux_data: &[u8]) -> Result<([u8; 32], Vec<u8>), ()>
    {
        use ark_ec_vrfs::ring::Prover as _;

        let input = match vrf_input_point(vrf_input_data) {
            Some(input) => input,
            None => {
                eprintln!("Failed to create VRF input point");
                return Err(()); // or handle the error accordingly
            }
        };
        let output = self.secret.output(input);

        // Backend currently requires the wrapped type (plain affine points)
        let pts: Vec<_> = self.ring.iter().map(|pk| pk.0).collect();

        // Proof construction
        let ring_ctx = ring_context();
        let prover_key = ring_ctx.prover_key(&pts);
        let prover = ring_ctx.prover(prover_key, self.prover_idx); //MK: this is using specific prover. Uestion: can we pass in ANY prover (e.g hardcoding prover0)
        let proof = self.secret.prove(input, output, aux_data, &prover);

        // Output and Ring Proof bundled together (as per section 2.2)
        let signature = RingVrfSignature { output, proof };
        let vrf_output = signature.output;

        let vrf_output_hash: [u8; 32] = match vrf_output.hash()[..32].try_into() {
            Ok(hash) => hash,
            Err(_) => {
                eprintln!("Failed to convert VRF output hash to array");
                return Err(());
            }
        };

        let mut buf = Vec::new();
        if let Err(_) = signature.serialize_compressed(&mut buf) {
            eprintln!("Failed to serialize signature");
            return Err(());
        }

        Ok((vrf_output_hash, buf))
    }

    /// Non-Anonymous VRF signature.
    ///
    /// Used for ticket claiming during block production.
    /// Not used with Safrole test vectors.
    pub fn ietf_vrf_sign(&self, vrf_input_data: &[u8], aux_data: &[u8]) -> Result<Vec<u8>, ()> {
        use ark_ec_vrfs::ietf::Prover as _;

        let input = match vrf_input_point(vrf_input_data) {
            Some(input) => input,
            None => {
                eprintln!("Failed to create VRF input point");
                return Err(()); // or handle the error accordingly
            }
        };
        let output = self.secret.output(input);

        let proof = self.secret.prove(input, output, aux_data);

        // Output and IETF Proof bundled together (as per section 2.2)
        let signature = IetfVrfSignature { output, proof };
        let mut buf = Vec::new();
        if let Err(_) = signature.serialize_compressed(&mut buf) {
            eprintln!("Failed to serialize signature");
            return Err(());
        }
        Ok(buf)
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
        let pts: Vec<_> = ring.iter().map(|pk| pk.0).collect();
        let verifier_key = ring_context().verifier_key(&pts);
        let commitment = verifier_key.commitment();

        Self { ring, commitment }
    }

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
                //println!("Failed to deserialize signature: {:?}", e);
                return Err(());
            }
        };

        let input = match vrf_input_point(vrf_input_data) {
            Some(input) => input,
            None => {
                eprintln!("Failed to create VRF input point");
                return Err(()); // or handle the error accordingly
            }
        };
        let output = signature.output;

        let ring_ctx = ring_context();
        let verifier_key = ring_ctx.verifier_key(&self.ring.iter().map(|pk| pk.0).collect::<Vec<_>>());
        let verifier = ring_ctx.verifier(verifier_key);

        if Public::verify(input, output, aux_data, &signature.proof, &verifier).is_err() {
            return Err(());
        }

        // Convert VRF output hash to array and handle errors
        let vrf_output_hash: [u8; 32] = match output.hash()[..32].try_into() {
            Ok(hash) => hash,
            Err(_) => {
                eprintln!("Failed to convert VRF output hash to array");
                return Err(());
            }
        };

        // Print VRF output hash if needed
        // println!("vrf-output-hash: {}", hex::encode(vrf_output_hash));

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

        let signature = match IetfVrfSignature::deserialize_compressed(signature) {
            Ok(sig) => sig,
            Err(_) => {
                eprintln!("Failed to deserialize compressed signature");
                return Err(());
            }
        };

        let input = match vrf_input_point(vrf_input_data) {
            Some(input) => input,
            None => {
                eprintln!("Failed to create VRF input point");
                return Err(()); // or handle the error accordingly
            }
        };
        let output = signature.output;

        let public = &self.ring[signer_key_index];
        if public
            .verify(input, output, aux_data, &signature.proof)
            .is_err()
        {
            println!("Ring signature verification failure");
            return Err(());
        }
        //println!("Ietf signature verified");

        // This is the actual value used as ticket-id/score
        // NOTE: as far as vrf_input_data is the same, this matches the one produced
        // using the ring-vrf (regardless of aux_data).
        let vrf_output_hash: [u8; 32] = match output.hash()[..32].try_into() {
            Ok(hash) => hash,
            Err(_) => {
                eprintln!("Failed to convert VRF output hash to array");
                return Err(());
            }
        };
        //println!(" vrf-output-hash: {}", hex::encode(vrf_output_hash));
        Ok(vrf_output_hash)
    }
}

use hex;
use std::os::raw::{c_int, c_uchar};
use std::slice;

#[no_mangle]
pub extern "C" fn get_public_key(
    seed_bytes: *const c_uchar,
    seed_len: usize,
    pub_key: *mut c_uchar,
    pub_key_len: usize,
) {
    use std::ptr;
    use std::slice;
    use ark_serialize::CanonicalSerialize;
    use ark_serialize::CanonicalDeserialize;

    // Convert seed bytes to a slice
    let seed_slice = unsafe { slice::from_raw_parts(seed_bytes, seed_len) };

    // Generate the private key from the scalar
    let secret = Secret::from_seed(seed_slice);
    //println!("Reconstructed Public Key: {:?}", secret.public);

/*
    // Deserialize the private key from bytes
    let secret = match Secret::deserialize_compressed(&seed_slice[..]) {
        Ok(secret) => secret,
        Err(e) => {
            eprintln!("Failed to deserialize private key: {}", e);
            return;
        }
    };
*/

    // Get the public key from the private key
    let public_key = secret.public();
    //println!("Public Key: {:?}", public_key);

    // Serialize the public key to bytes
    let mut pk_bytes = Vec::new();
    if let Err(e) = public_key.serialize_compressed(&mut pk_bytes) {
        eprintln!("Failed to serialize public key: {}", e);
        return;
    }

    // Convert the public key bytes to a hex string
    let pk_hex = hex::encode(&pk_bytes);
    //println!("Public Key(hex): {}", pk_hex);

    // Ensure the provided buffer is large enough to hold the public key bytes
    if pub_key_len < pk_bytes.len() {
        eprintln!("Provided buffer is too small for the public key");
        return;
    }

    // Copy the public key bytes into the provided buffer
    unsafe {
        ptr::copy_nonoverlapping(pk_bytes.as_ptr(), pub_key, pk_bytes.len());
    }
}

#[no_mangle]
pub extern "C" fn get_private_key(
    seed_bytes: *const c_uchar,
    seed_len: usize,
    secret_bytes: *mut c_uchar,
    secret_len: usize,
) {
    use std::ptr;
    use std::slice;
    use ark_serialize::CanonicalSerialize;
    use ark_serialize::CanonicalDeserialize;

    // Convert seed bytes to a slice
    let seed_slice = unsafe { slice::from_raw_parts(seed_bytes, seed_len) };


    // Generate the private key from the seed
    let secret = Secret::from_seed(seed_slice);
    //println!("Secret: {:?}", secret);

/*
    // Deserialize the private key from bytes
    let secret = match Secret::deserialize_compressed(&seed_slice[..]) {
        Ok(secret) => secret,
        Err(e) => {
            eprintln!("Failed to deserialize private key: {}", e);
            return;
        }
    };
*/
    // Serialize the secret to bytes
    let mut secret_bytes_vec = Vec::new();
    if let Err(e) = secret.scalar.serialize_compressed(&mut secret_bytes_vec) {
        eprintln!("Failed to serialize secret: {}", e);
        return;
    }

    // Convert the priv key bytes to a hex string
    let priv_hex = hex::encode(&secret_bytes_vec);
    //println!("Priv Key(hex): {}", priv_hex);

    // Ensure the provided buffer is large enough to hold the secret bytes
    if secret_len < secret_bytes_vec.len() {
        eprintln!("Provided buffer is too small for the secret");
        return;
    }

    // Copy the secret bytes into the provided buffer
    unsafe {
        ptr::copy_nonoverlapping(secret_bytes_vec.as_ptr(), secret_bytes, secret_bytes_vec.len());
    }
}


#[no_mangle]
pub extern "C" fn ietf_vrf_sign(
    private_key_bytes: *const c_uchar,
    private_key_len: usize,
    vrf_input_data_bytes: *const c_uchar,
    vrf_input_data_len: usize,
    aux_data_bytes: *const c_uchar,
    aux_data_len: usize,
    sig: *mut c_uchar,
    sig_len: usize,
    vrf_output: *mut c_uchar,
    vrf_output_len: usize,
) {
    use std::slice;
    use std::ptr;
    use ark_serialize::CanonicalDeserialize;

    // Convert private key bytes to a slice
    let private_key_slice = unsafe { slice::from_raw_parts(private_key_bytes, private_key_len) };

    // Deserialize the private key from bytes
    let private_key = match Secret::deserialize_compressed(&private_key_slice[..]) {
        Ok(secret) => secret,
        Err(e) => {
            eprintln!("Failed to deserialize private key: {}", e);
            return;
        }
    };

    // Convert input data to slices
    let vrf_input_data = unsafe { slice::from_raw_parts(vrf_input_data_bytes, vrf_input_data_len) };
    let aux_data = unsafe { slice::from_raw_parts(aux_data_bytes, aux_data_len) };

    // Use the Prover instance to sign the data
    let signature = {
        use ark_ec_vrfs::ietf::Prover as _;

        let input = match vrf_input_point(vrf_input_data) {
            Some(input) => input,
            None => {
                eprintln!("Failed to create VRF input point");
                return; // or handle the error accordingly
            }
        };
        let output = private_key.output(input);

        let proof =  private_key.prove(input, output, aux_data);

        // Output and IETF Proof bundled together (as per section 2.2)
        let signature = IetfVrfSignature { output, proof };
        let mut buf = Vec::new();
        match signature.serialize_compressed(&mut buf) {
            Ok(_) => (),
            Err(e) => {
                eprintln!("Failed to serialize signature: {}", e);
                return;
            }
        }

        let output = signature.output;
        let vrf_output_hash: [u8; 32] = match output.hash()[..32].try_into() {
            Ok(hash) => hash,
            Err(e) => {
                eprintln!("Failed to convert VRF output hash to array: {}", e);
                return;
            }
        };

        // Ensure the provided buffer is large enough to hold the VRF output
        assert!(
            vrf_output_len >= vrf_output_hash.len(),
            "Provided buffer is too small for the VRF output"
        );

        // Copy the VRF output bytes into the provided buffer
        unsafe {
            ptr::copy_nonoverlapping(vrf_output_hash.as_ptr(), vrf_output, vrf_output_hash.len());
        }
        buf
    };

    // Ensure the provided buffer is large enough to hold the signature
    assert!(
        sig_len >= signature.len(),
        "Provided buffer is too small for the signature"
    );

    // Copy the signature bytes into the provided buffer
    unsafe {
        ptr::copy_nonoverlapping(signature.as_ptr(), sig, signature.len());
    }
}


#[no_mangle]
pub extern "C" fn get_ietf_vrf_output(
    sig: *const c_uchar,
    sig_len: usize,
    vrf_output: *mut c_uchar,
    vrf_output_len: usize,
) -> c_int {

    use std::ptr;

    // Convert the signature pointer to a slice
    let signature_slice = unsafe { slice::from_raw_parts(sig, sig_len) };

    // Deserialize the signature
    let signature = match IetfVrfSignature::deserialize_compressed(signature_slice) {
        Ok(sig) => sig,
        Err(_) => return 0, // Return 0 on deserialization failure
    };

    // Compute the VRF output hash
    let vrf_output_hash: [u8; 32] = match signature.output.hash()[..32].try_into() {
        Ok(hash) => hash,
        Err(_) => return 0, // Return 0 on conversion failure
    };

    // Ensure the provided buffer is large enough to hold the VRF output
    if vrf_output_len < vrf_output_hash.len() {
        return 0; // Return 0 if buffer is too small
    }

    // Copy the VRF output hash into the provided buffer
    unsafe {
        ptr::copy_nonoverlapping(vrf_output_hash.as_ptr(), vrf_output, vrf_output_hash.len());
    }

    1 // Return 1 on success
}

#[no_mangle]
pub extern "C" fn ring_vrf_sign(
    private_key_bytes: *const c_uchar,
    private_key_len: usize,
    ring_set_bytes: *const c_uchar,
    ring_set_len: usize,
    vrf_input_data_bytes: *const c_uchar,
    vrf_input_data_len: usize,
    aux_data_bytes: *const c_uchar,
    aux_data_len: usize,
    sig: *mut c_uchar,
    sig_len: usize,
    vrf_output: *mut c_uchar,
    vrf_output_len: usize,
    //prover_idx: usize,
) {
    use std::slice;
    use std::ptr;
    use ark_serialize::CanonicalDeserialize;

    // Convert private key bytes to a slice
    let private_key_slice = unsafe { slice::from_raw_parts(private_key_bytes, private_key_len) };

    // Deserialize the private key from bytes
    let private_key = match Secret::deserialize_compressed(&private_key_slice[..]) {
        Ok(secret) => secret,
        Err(e) => {
            eprintln!("Failed to deserialize private key: {}", e);
            return;
        }
    };

    // Convert input data to slices
    let vrf_input_data = unsafe { slice::from_raw_parts(vrf_input_data_bytes, vrf_input_data_len) };
    let aux_data = unsafe { slice::from_raw_parts(aux_data_bytes, aux_data_len) };

    // Deserialize the ring set from the provided bytes
    let ring_set_slice = unsafe { slice::from_raw_parts(ring_set_bytes, ring_set_len) };
    let mut ring_set: Vec<Public> = Vec::new();
    for i in 0..(ring_set_len / 32) {
        let pubkey_bytes = &ring_set_slice[i * 32..(i + 1) * 32];
        let public_key = match Public::deserialize_compressed(pubkey_bytes) {
            Ok(public_key) => public_key,
            Err(e) => {
                eprintln!("Failed to deserialize public key: {}", e);
                return;
            }
        };
        ring_set.push(public_key);
    }

    // Set prover_idx based on pubkey's index in ring_set

    // Derive the public key from the private key
    let derived_public_key = private_key.public();

    // Serialize the derived public key to bytes
    let mut derived_pk_bytes = Vec::new();
    if let Err(e) = derived_public_key.serialize_compressed(&mut derived_pk_bytes) {
        eprintln!("Failed to serialize derived public key: {}", e);
        return;
    }

    // Find the index of the derived public key in the ring set
    let derived_prover_idx = match ring_set.iter().position(|pk| {
        let mut pk_bytes = Vec::new();
        pk.serialize_compressed(&mut pk_bytes).unwrap();
        pk_bytes == derived_pk_bytes
    }) {
        Some(idx) => idx,
        None => {
            eprintln!("Derived public key not found in the ring set");
            return;
        }
    };
    //println!("derived_public_key {:?} idx:{:?}", derived_public_key, derived_prover_idx);

    // Create the Prover instance
    let prover = Prover {
        prover_idx:derived_prover_idx, //use prover_idx
        secret: private_key,
        ring: ring_set,
    };

    // Use the Prover instance to sign the data
    let (vrf_output_hash, signature) = match prover.ring_vrf_sign(vrf_input_data, aux_data) {
        Ok(result) => result,
        Err(e) => {
            eprintln!("Failed to sign the data: {:?}", e);
            return;
        }
    };

    // Ensure the provided buffer is large enough to hold the signature
    assert!(
        sig_len >= signature.len(),
        "Provided buffer is too small for the signature"
    );

    // Copy the signature bytes into the provided buffer
    unsafe {
        ptr::copy_nonoverlapping(signature.as_ptr(), sig, signature.len());
    }

    // Ensure the provided buffer is large enough to hold the VRF output
    assert!(
        vrf_output_len >= vrf_output_hash.len(),
        "Provided buffer is too small for the VRF output"
    );

    // Copy the VRF output bytes into the provided buffer
    unsafe {
        ptr::copy_nonoverlapping(vrf_output_hash.as_ptr(), vrf_output, vrf_output_hash.len());
    }
}


#[no_mangle]
pub extern "C" fn ietf_vrf_verify(
    prover_public_bytes: *const c_uchar,
    prover_public_len: usize,
    signature_bytes: *const c_uchar,
    signature_len: usize,
    vrf_input_data_bytes: *const c_uchar,
    vrf_input_data_len: usize,
    aux_data_bytes: *const c_uchar,
    aux_data_len: usize,
    vrf_output: *mut c_uchar,
    vrf_output_len: usize
) -> c_int {
    use std::slice;
    use ark_ec_vrfs::suites::bandersnatch::edwards::Public;
    use ark_serialize::CanonicalDeserialize;
    use std::os::raw::c_uchar;

    let prover_public_slice = unsafe { slice::from_raw_parts(prover_public_bytes, prover_public_len) };
    let signature_slice = unsafe { slice::from_raw_parts(signature_bytes, signature_len) };
    let vrf_input_data_slice = unsafe { slice::from_raw_parts(vrf_input_data_bytes, vrf_input_data_len) };
    let aux_data_slice = unsafe { slice::from_raw_parts(aux_data_bytes, aux_data_len) };

    //println!("Prover public slice (hex): {:?}", hex::encode(prover_public_slice));

    // Check the length of the public key slice
    //println!("Prover public slice length: {}", prover_public_slice.len());

    let prover_public = match Public::deserialize_compressed(prover_public_slice) {
        Ok(public_key) => public_key,
        Err(e) => {
            println!("Failed to deserialize prover public key: {:?}", e);
            return 0;
        }
    };

    let res = ietf_vrf_verify_iml(
        &prover_public,
        vrf_input_data_slice,
        aux_data_slice,
        signature_slice,
    );

    match res {
        Ok(vrf_output_hash) => {
            // Ensure the provided buffer is large enough to hold the VRF output
            if vrf_output_len < vrf_output_hash.len() {
                return 0;
            }
            unsafe {
                std::ptr::copy_nonoverlapping(vrf_output_hash.as_ptr(), vrf_output, 32);
            }
            return 1
        }
        Err(_) => return 0,
    }
}

fn ietf_vrf_verify_iml(
    prover_public: &Public,
    vrf_input_data: &[u8],
    aux_data: &[u8],
    signature: &[u8],
) -> Result<[u8; 32], ()> {
    use ark_ec_vrfs::ietf::Verifier as _;

    let signature = match IetfVrfSignature::deserialize_compressed(signature) {
        Ok(signature) => signature,
        Err(e) => {
            eprintln!("Failed to deserialize IETF VRF signature: {}", e);
            return Err(());
        }
    };

    let input = match vrf_input_point(vrf_input_data) {
        Some(input) => input,
        None => {
            eprintln!("Failed to create VRF input point");
            return Err(()); // or handle the error accordingly
        }
    };
    let output = signature.output;

    //println!("Public key: {:?}", prover_public);
    if prover_public
        .verify(input, output, aux_data, &signature.proof)
        .is_err()
    {
        //println!("Ietf signature verification failure");
        return Err(());
    }
    //println!("Ietf signature verified");

    let vrf_output_hash: [u8; 32] = match output.hash()[..32].try_into() {
        Ok(hash) => hash,
        Err(e) => {
            eprintln!("Failed to convert VRF output hash to array: {}", e);
            return Err(());
        }
    };

    //println!(" vrf-output-hash: {}", hex::encode(vrf_output_hash));
    Ok(vrf_output_hash)
}

#[no_mangle]
pub extern "C" fn get_ring_commitment(
    ring_set_bytes: *const c_uchar,
    ring_set_len: usize,
    commitment: *mut c_uchar,
    commitment_len: usize,
) {
    use std::slice;
    use std::ptr;
    use ark_serialize::CanonicalDeserialize;

    // Deserialize the ring set from the provided bytes
    let ring_set_slice = unsafe { slice::from_raw_parts(ring_set_bytes, ring_set_len) };
    let mut ring_set: Vec<Public> = Vec::new();
    for i in 0..(ring_set_len / 32) {
        let pubkey_bytes = &ring_set_slice[i * 32..(i + 1) * 32];
        let public_key = match Public::deserialize_compressed(pubkey_bytes) {
            Ok(public_key) => public_key,
            Err(e) => {
                eprintln!("Failed to deserialize public key: {}", e);
                return;
            }
        };
        ring_set.push(public_key);
    }

    // Create the Verifier instance
    let verifier = Verifier::new(ring_set);

    // Get the commitment from the verifier
    let mut commitment_bytes = Vec::new();
    match verifier.commitment.serialize_compressed(&mut commitment_bytes){
        Ok(_) => (),
        Err(e) => {
            eprintln!("Failed to serialize commitment: {}", e);
            return;
        }
    }
    //println!("Verifier commitment: {:?}", hex::encode(&commitment_bytes));
    // Ensure the provided buffer is large enough to hold the commitment
    if commitment_len < commitment_bytes.len() {
        eprintln!("Provided buffer is too small for the commitment");
        return;
    }

    // Copy the commitment bytes into the provided buffer
    unsafe {
        ptr::copy_nonoverlapping(commitment_bytes.as_ptr(), commitment, commitment_len);
    }
}

#[no_mangle]
pub extern "C" fn ring_vrf_verify(
    pubkeys_bytes: *const c_uchar,
    pubkeys_length: usize,
    signature_bytes: *const c_uchar,
    signature_hex_len: usize,
    vrf_input_data_bytes: *const c_uchar,
    vrf_input_data_len: usize,
    aux_data_bytes: *const c_uchar,
    aux_data_len: usize,
    vrf_output: *mut c_uchar,
    vrf_output_len: usize
) -> c_int {
    /*println!(
        "pubkeys_length: {}, signature_hex_len: {}, vrf_input_data_len: {}, aux_data_len: {}",
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

    /*
    println!("pubkeys_slice: {}", hex::encode(pubkeys_slice));
    println!("signature_slice: {}", hex::encode(signature_slice));
    println!(
        "vrf_input_data_slice: {}",
        hex::encode(vrf_input_data_slice)
    );
    println!("aux_data_slice: {}", hex::encode(aux_data_slice));
    */
    // Assuming each pubkey is 32 bytes, split the pubkeys slice into individual pubkeys
    let mut ring_set: Vec<Public> = Vec::new();
    for i in 0..(pubkeys_length / 32) {
        let pubkey_bytes = &pubkeys_slice[i * 32..(i + 1) * 32];
        match Public::deserialize_compressed(pubkey_bytes) {
            Ok(public_key) => {
                /*println!(
                    "Deserialized public key {}: {}",
                    i,
                    hex::encode(pubkey_bytes)
                ); */
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
            //println!("Verification successful");
            //println!("VRF output hash: {}", hex::encode(vrf_output_hash));
            // Ensure the provided buffer is large enough to hold the VRF output
            if vrf_output_len < vrf_output_hash.len() {
                return 0;
            }
            unsafe {
                std::ptr::copy_nonoverlapping(vrf_output_hash.as_ptr(), vrf_output, 32);
            }
            return 1
        }
        Err(e) => {
            return 0
        }
    }
}

#[no_mangle]
pub extern "C" fn get_ring_vrf_output(
    sig: *const c_uchar,
    sig_len: usize,
    vrf_output: *mut c_uchar,
    vrf_output_len: usize,
) -> c_int {

    use std::ptr;

    // Convert the signature pointer to a slice
    let signature_slice = unsafe { slice::from_raw_parts(sig, sig_len) };

    // Deserialize the signature
    let signature = match RingVrfSignature::deserialize_compressed(signature_slice) {
        Ok(sig) => sig,
        Err(_) => return 0, // Return 0 on deserialization failure
    };

    // Compute the VRF output hash
    let vrf_output_hash: [u8; 32] = match signature.output.hash()[..32].try_into() {
        Ok(hash) => hash,
        Err(_) => return 0, // Return 0 on conversion failure
    };

    // Ensure the provided buffer is large enough to hold the VRF output
    if vrf_output_len < vrf_output_hash.len() {
        return 0; // Return 0 if buffer is too small
    }

    // Copy the VRF output hash into the provided buffer
    unsafe {
        ptr::copy_nonoverlapping(vrf_output_hash.as_ptr(), vrf_output, vrf_output_hash.len());
    }

    1 // Return 1 on success
}


#[cfg(test)]
mod tests {
    use super::*;
    use hex;
    use rand::rngs::StdRng;
    use rand::{Rng, SeedableRng};
    const SCALAR_SIZE: usize = 32;
    const PUB_KEY_SIZE: usize = 32;
    const IETF_SIGNATURE_SIZE: usize = 96;
    const RING_SIGNATURE_SIZE: usize = 784;
    const VRF_OUTPUT_SIZE: usize = 32;
    const SECRET_SIZE: usize = 64; // Adjust as per the actual size of the compressed secret

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
    fn test_ietf_vrf_sign_and_verify() {
        use ark_serialize::CanonicalSerialize;
        use ark_serialize::CanonicalDeserialize;
        use hex;
        use rand::Rng;

        // Step 1: Generate a random valid private key
        let mut rng = rand::thread_rng();
        let mut seed: [u8; SCALAR_SIZE] = rng.gen();
        seed.reverse(); // Ensure little-endian order
        println!("Generated seed (little-endian): {:?}", seed);

        // Allocate buffer for the public key
        let mut prover_public_bytes = vec![0u8; PUB_KEY_SIZE];

        // Get the public key from the private key
        get_public_key(
            seed.as_ptr(),
            seed.len(),
            prover_public_bytes.as_mut_ptr(),
            prover_public_bytes.len(),
        );

        println!("Seed bytes (hex): {:?}", hex::encode(&seed));
        println!("Prover public bytes (hex): {:?}", hex::encode(&prover_public_bytes));

        // Data to be signed
        let vrf_input_data = b"example input data";
        let aux_data = b"example aux data";

        // Allocate buffer for signature
        let mut signature = vec![0u8; 96]; // Adjust size as needed

        // Allocate buffer for VRF output
        let mut vrf_output0 = [0u8; 32];
        //generate prover private key
        let mut secret_bytes = vec![0u8; SECRET_SIZE];

        get_private_key(
            seed.as_ptr(),
            seed.len(),
            secret_bytes.as_mut_ptr(),
            secret_bytes.len(),
        );


        // Sign the data
        ietf_vrf_sign(
            secret_bytes.as_ptr(),
            secret_bytes.len(),
            vrf_input_data.as_ptr(),
            vrf_input_data.len(),
            aux_data.as_ptr(),
            aux_data.len(),
            signature.as_mut_ptr(),
            signature.len(),
            vrf_output0.as_mut_ptr(),
            vrf_output0.len(),
        );

        println!("VRF Output0: {:?}", hex::encode(vrf_output0));
        println!("Signature: {:?}", hex::encode(&signature));

        // Verify the signature and generate VRF output
        let mut vrf_output = [0u8; 32];
        let result = ietf_vrf_verify(
            prover_public_bytes.as_ptr(),
            prover_public_bytes.len(),
            signature.as_ptr(),
            signature.len(),
            vrf_input_data.as_ptr(),
            vrf_input_data.len(),
            aux_data.as_ptr(),
            aux_data.len(),
            vrf_output.as_mut_ptr(),
            vrf_output.len(),
        );

        // Check if the verification was successful
        if result == 1 {
            println!("Verification successful");
            println!("VRF Output1: {:?}", hex::encode(vrf_output));
        } 
    }

    #[test]
    fn test_ring_vrf_sign_and_verify() {
        use ark_serialize::CanonicalSerialize;
        use ark_serialize::CanonicalDeserialize;
        use hex;
        use std::os::raw::c_uchar;

        const SEED_SIZE: usize = 32;
        const SECRET_SIZE: usize = 32;

        // Step 1: Generate RING_SIZE private keys
        let mut seeds: Vec<[u8; SEED_SIZE]> = Vec::new();
        for _i in 0..RING_SIZE {
            let mut seed = [0u8; SEED_SIZE];
            for (i, byte) in seed.iter_mut().enumerate() {
                *byte = i as u8;
            }
            seeds.push(seed);
        }

        let mut public_keys: Vec<Public> = Vec::new();
        let mut private_keys: Vec<[u8; SECRET_SIZE]> = Vec::new();
        for seed in &seeds {
            // Allocate buffers for public key and secret
            let mut pub_key_bytes = vec![0u8; 32];
            let mut secret_bytes = vec![0u8; SECRET_SIZE];

            // Generate public key from the seed
            get_public_key(
                seed.as_ptr(),
                seed.len(),
                pub_key_bytes.as_mut_ptr(),
                pub_key_bytes.len(),
            );

            // Generate private key from the seed
            get_private_key(
                seed.as_ptr(),
                seed.len(),
                secret_bytes.as_mut_ptr(),
                secret_bytes.len(),
            );

            // Deserialize public key
            let public_key = match Public::deserialize_compressed(&pub_key_bytes[..]) {
                Ok(pk) => pk,
                Err(e) => {
                    eprintln!("Failed to deserialize public key: {}", e);
                    return;
                }
            };
            public_keys.push(public_key);

            let mut private_key_array = [0u8; SECRET_SIZE];
            private_key_array.copy_from_slice(&secret_bytes);
            private_keys.push(private_key_array);
        }

        // Serialize the public keys
        let mut ring_set_bytes: Vec<u8> = Vec::new();
        for pk in &public_keys {
            let mut pk_bytes = Vec::new();
            if let Err(e) = pk.serialize_compressed(&mut pk_bytes) {
                eprintln!("Failed to serialize public key: {}", e);
                return;
            }
            ring_set_bytes.extend(pk_bytes);
        }

        // Data to be signed
        let vrf_input_data = b"example input data";
        let aux_data = b"example aux data";

        // Allocate buffer for signature
        let mut signature = vec![0u8; 784]; // Adjust size as needed

        // Allocate buffer for VRF output
        let mut vrf_output = [0u8; 32];

        // Step 2: Use one of the private keys to sign the data
        let prover_idx = 2; // Choose the third key as the prover
        ring_vrf_sign(
            private_keys[prover_idx].as_ptr(),
            private_keys[prover_idx].len(),
            ring_set_bytes.as_ptr(),
            ring_set_bytes.len(),
            vrf_input_data.as_ptr(),
            vrf_input_data.len(),
            aux_data.as_ptr(),
            aux_data.len(),
            signature.as_mut_ptr(),
            signature.len(),
            vrf_output.as_mut_ptr(),
            vrf_output.len(),
        );

        println!("Signature: {:?}", hex::encode(&signature));
        println!("VRF Output: {:?}", hex::encode(&vrf_output));

        // Step 3: Verify the signature
        let result = ring_vrf_verify(
            ring_set_bytes.as_ptr(),
            ring_set_bytes.len(),
            signature.as_ptr(),
            signature.len(),
            vrf_input_data.as_ptr(),
            vrf_input_data.len(),
            aux_data.as_ptr(),
            aux_data.len(),
            vrf_output.as_mut_ptr(),
            vrf_output.len(),
        );

        // Check if the verification was successful
        if result == 1 {
            println!("Verification successful");
            println!("VRF Output: {:?}", hex::encode(vrf_output));
        }
    }

    const SEED_SIZE: usize = 32;
    #[test]
    fn test_key_generation_and_consistency() {
       // Step 1: Generate a random scalar and convert to little-endian
       let mut rng = rand::thread_rng();
       let mut seed: [u8; SEED_SIZE] = rng.gen();
       seed.reverse(); // Ensure little-endian order
       println!("Generated seed (little-endian): {:?}", seed);

        // Allocate buffers for public key and secret
        let mut pub_key_bytes = vec![0u8; PUB_KEY_SIZE];
        let mut secret_bytes = vec![0u8; SECRET_SIZE];

        // Generate public key from the seed
        get_public_key(
            seed.as_ptr(),
            seed.len(),
            pub_key_bytes.as_mut_ptr(),
            pub_key_bytes.len(),
        );

        // Generate private key from the seed
        get_private_key(
            seed.as_ptr(),
            seed.len(),
            secret_bytes.as_mut_ptr(),
            secret_bytes.len(),
        );

        // Check if the private key can regenerate the same public key
        let secret_slice = unsafe { slice::from_raw_parts(secret_bytes.as_ptr(), secret_bytes.len()) };
        let secret = match Secret::deserialize_compressed(&secret_slice[..]) {
            Ok(secret) => secret,
            Err(e) => {
                eprintln!("Failed to deserialize secret: {}", e);
                return;
            }
        };

        let regenerated_pub_key = secret.public();

        // Serialize the regenerated public key to compare
        let mut regenerated_pub_key_bytes = Vec::new();
        if let Err(e) = regenerated_pub_key.serialize_compressed(&mut regenerated_pub_key_bytes) {
            eprintln!("Failed to serialize regenerated public key: {}", e);
            return;
        }

        assert_eq!(pub_key_bytes, regenerated_pub_key_bytes, "Public keys do not match");
    }

    #[test]
    fn test_ring_commitment() {
        // Step 1: Generate RING_SIZE private keys
        let mut seeds: Vec<[u8; SEED_SIZE]> = Vec::new();
        for _i in 0..RING_SIZE {
            let mut seed = [0u8; SEED_SIZE];
            for (i, byte) in seed.iter_mut().enumerate() {
                *byte = i as u8;
            }
            seeds.push(seed);
        }

        let mut public_keys: Vec<Public> = Vec::new();
        let mut private_keys: Vec<[u8; SECRET_SIZE]> = Vec::new();
        for seed in &seeds {
            // Allocate buffers for public key and secret
            let mut pub_key_bytes = vec![0u8; 32];
            let mut secret_bytes = vec![0u8; SECRET_SIZE];

            // Generate public key from the seed
            get_public_key(
                seed.as_ptr(),
                seed.len(),
                pub_key_bytes.as_mut_ptr(),
                pub_key_bytes.len(),
            );

            // Generate private key from the seed
            get_private_key(
                seed.as_ptr(),
                seed.len(),
                secret_bytes.as_mut_ptr(),
                secret_bytes.len(),
            );

            // Deserialize public key
            let public_key = match Public::deserialize_compressed(&pub_key_bytes[..]) {
                Ok(pk) => pk,
                Err(e) => {
                    eprintln!("Failed to deserialize public key: {}", e);
                    return;
                }
            };
            public_keys.push(public_key);

            let mut private_key_array = [0u8; SECRET_SIZE];
            private_key_array.copy_from_slice(&secret_bytes);
            private_keys.push(private_key_array);
        }

        // Serialize the public keys
        let mut ring_set_bytes: Vec<u8> = Vec::new();
        for pk in &public_keys {
            let mut pk_bytes = Vec::new();
            if let Err(e) = pk.serialize_compressed(&mut pk_bytes) {
                eprintln!("Failed to serialize public key: {}", e);
                return;
            }
            ring_set_bytes.extend(pk_bytes);
        }

        // Get the commitment from the verifier
        let mut commitment = vec![0u8; 144];
        get_ring_commitment(
            ring_set_bytes.as_ptr(),
            ring_set_bytes.len(),
            commitment.as_mut_ptr(),
            commitment.len(),
        );

        println!("Commitment: {:?}", hex::encode(&commitment));
    }
}
