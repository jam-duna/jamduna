use ark_vrf::reexports::{
    ark_serialize::{self, CanonicalDeserialize, CanonicalSerialize},
};
use ark_vrf::suites::bandersnatch;
use bandersnatch::{
    IetfProof, Input, Output, Public, RingProof,
    RingProofParams, Secret,
};

use sha2::Sha256;
use ark_bls12_381::Bls12_381;
use ark_ff::Zero;

use w3f_bls::{
    serialize::SerializableToBytes,
    double::{DoublePublicKeyScheme, DoublePublicKey},
    single_pop_aggregator::SignatureAggregatorAssumingPoP,
    EngineBLS, Message, TinyBLS, TinyBLS381,
    PublicKey as BlsPublicKey, PublicKeyInSignatureGroup,
    Signature as BlsSignature, SecretKey as BlsSecretKey,
    Signed,
};

use reed_solomon_simd::{ReedSolomonDecoder, ReedSolomonEncoder};
use std::os::raw::{c_int, c_uchar, c_uint};
use std::sync::OnceLock;
use std::collections::HashMap;

#[derive(CanonicalSerialize, CanonicalDeserialize)]
struct IetfVrfSignature {
    output: Output,
    proof: IetfProof,
}

#[derive(CanonicalSerialize, CanonicalDeserialize)]
struct RingVrfSignature {
    output: Output,
    proof: RingProof, // pedersen + ring proof bundled
}

// cache ring proof params - deserializing zcash SRS takes ~100ms
static PCS_PARAMS_CACHE: OnceLock<std::sync::Mutex<HashMap<usize, Box<RingProofParams>>>> = OnceLock::new();

fn init_common_ring_sizes() {
    let cache = PCS_PARAMS_CACHE.get_or_init(|| std::sync::Mutex::new(HashMap::new()));
    let mut cache_guard = cache.lock().unwrap();
    
    // precompute for common JAM ring sizes to avoid first-call penalty
    let common_sizes = [6, 1023];
    
    use bandersnatch::PcsParams;
    let buf: &[u8] = include_bytes!("../zcash-srs-2-11-uncompressed.bin");
    let pcs_params = PcsParams::deserialize_uncompressed_unchecked(&mut &buf[..]).unwrap();
    
    for &size in &common_sizes {
        if let Ok(params) = RingProofParams::from_pcs_params(size, pcs_params.clone()) {
            cache_guard.insert(size, Box::new(params));
        }
    }
}

fn ring_proof_params(ring_size: usize) -> RingProofParams {
    let cache = PCS_PARAMS_CACHE.get_or_init(|| std::sync::Mutex::new(HashMap::new()));
    let mut cache_guard = cache.lock().unwrap();
    
    if let Some(cached_params) = cache_guard.get(&ring_size) {
        return (**cached_params).clone();
    }
    
    // not cached, compute params
    use bandersnatch::PcsParams;
    let buf: &[u8] = include_bytes!("../zcash-srs-2-11-uncompressed.bin");
    let pcs_params = PcsParams::deserialize_uncompressed_unchecked(&mut &buf[..]).unwrap();
    let params = RingProofParams::from_pcs_params(ring_size, pcs_params).unwrap();
    
    cache_guard.insert(ring_size, Box::new(params.clone()));
    params
}

fn vrf_input_point(vrf_input_data: &[u8]) -> Input {
    Input::new(vrf_input_data).unwrap()
}

struct Prover {
    pub prover_idx: usize,
    pub secret: Secret,
    pub ring: Vec<Public>,
}

impl Prover {

    pub fn ring_vrf_sign(&self, vrf_input_data: &[u8], aux_data: &[u8]) -> Result<([u8; 32], Vec<u8>), ()>
    {
        use ark_vrf::ring::Prover as _;

       let input = vrf_input_point(vrf_input_data);
        let output = self.secret.output(input);

        let pts: Vec<_> = self.ring.iter().map(|pk| pk.0).collect();

        let params = ring_proof_params(self.ring.len());
        let prover_key = params.prover_key(&pts);
        let prover = params.prover(prover_key, self.prover_idx);
        let proof = self.secret.prove(input, output, aux_data, &prover);

        let signature = RingVrfSignature { output, proof };
        let vrf_output = signature.output;

        let vrf_output_hash: [u8; 32] = match vrf_output.hash()[..32].try_into() {
            Ok(hash) => hash,
            Err(_) => {
                eprintln!("vrf output hash conversion failed");
                return Err(());
            }
        };

        let mut buf = Vec::new();
        if let Err(_) = signature.serialize_compressed(&mut buf) {
            eprintln!("signature serialize failed");
            return Err(());
        }

        Ok((vrf_output_hash, buf))
    }

}

type RingCommitment = ark_vrf::ring::RingCommitment<bandersnatch::BandersnatchSha512Ell2>;
type RingVerifierKey = ark_vrf::ring::RingVerifierKey<bandersnatch::BandersnatchSha512Ell2>;

struct Verifier {
    pub commitment: RingCommitment,
    pub ring: Vec<Public>,
    verifier_key: RingVerifierKey, // cached - expensive to recompute
    params: RingProofParams,
}

impl Verifier {
    fn new(ring: Vec<Public>) -> Self {
        let pts: Vec<_> = ring.iter().map(|pk| pk.0).collect();
        let params = ring_proof_params(ring.len());
        let verifier_key = params.verifier_key(&pts);
        let commitment = verifier_key.commitment();

        Self { ring, commitment, verifier_key, params }
    }

    pub fn ring_vrf_verify(
        &self,
        vrf_input_data: &[u8],
        aux_data: &[u8],
        signature: &[u8],
    ) -> Result<[u8; 32], ()> {
        use ark_vrf::ring::Verifier as _;

        let signature = match RingVrfSignature::deserialize_compressed_unchecked(signature) {
            Ok(sig) => sig,
            Err(_) => return Err(()),
        };

        let input = vrf_input_point(vrf_input_data);
        let output = signature.output;

        let verifier = self.params.verifier(self.verifier_key.clone());

        if Public::verify(input, output, aux_data, &signature.proof, &verifier).is_err() {
            return Err(());
        }

        let vrf_output_hash: [u8; 32] = match output.hash()[..32].try_into() {
            Ok(hash) => hash,
            Err(_) => {
                eprintln!("vrf output hash conversion failed");
                return Err(());
            }
        };

        Ok(vrf_output_hash)
    }

}

use std::slice;

#[no_mangle]
pub extern "C" fn get_public_key(
    seed_bytes: *const c_uchar,
    seed_len: usize,
    pub_key: *mut c_uchar,
    pub_key_len: usize,
) {
    use std::ptr;

    // Convert seed bytes to a slice
    let seed_slice = unsafe { slice::from_raw_parts(seed_bytes, seed_len) };

    let secret = Secret::from_seed(seed_slice);

    let public_key = secret.public();

    // Serialize the public key to bytes
    let mut pk_bytes = Vec::new();
    if let Err(e) = public_key.serialize_compressed(&mut pk_bytes) {
        eprintln!("Failed to serialize public key: {}", e);
        return;
    }

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

    // Convert seed bytes to a slice
    let seed_slice = unsafe { slice::from_raw_parts(seed_bytes, seed_len) };


    let secret = Secret::from_seed(seed_slice);

    // Serialize the secret to bytes
    let mut secret_bytes_vec = Vec::new();
    if let Err(e) = secret.scalar.serialize_compressed(&mut secret_bytes_vec) {
        eprintln!("Failed to serialize secret: {}", e);
        return;
    }

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
    //use std::slice;
    use std::ptr;
    //use ark_serialize::CanonicalDeserialize;

    // Convert private key bytes to a slice
    let private_key_slice = unsafe { slice::from_raw_parts(private_key_bytes, private_key_len) };

    // Deserialize the private key from bytes
    let private_key = match Secret::deserialize_compressed_unchecked(&private_key_slice[..]) {
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
        use ark_vrf::ietf::Prover as _;

        let input = vrf_input_point(vrf_input_data);
        let output = private_key.output(input);

        let proof =  private_key.prove(input, output, aux_data);

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
    let signature = match IetfVrfSignature::deserialize_compressed_unchecked(signature_slice) {
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
    ring_size: usize,
    vrf_input_data_bytes: *const c_uchar,
    vrf_input_data_len: usize,
    aux_data_bytes: *const c_uchar,
    aux_data_len: usize,
    sig: *mut c_uchar,
    sig_len: usize,
    vrf_output: *mut c_uchar,
    vrf_output_len: usize,
    //prover_idx: usize,
)-> c_int {
    //use std::slice;
    use std::ptr;
    //use ark_serialize::CanonicalDeserialize;

    // Convert private key bytes to a slice
    let private_key_slice = unsafe { slice::from_raw_parts(private_key_bytes, private_key_len) };

    // Deserialize the private key from bytes
    let private_key = match Secret::deserialize_compressed_unchecked(&private_key_slice[..]) {
        Ok(secret) => secret,
        Err(e) => {
            eprintln!("Failed to deserialize private key: {}", e);
            return 0;
        }
    };

    // Convert input data to slices
    let vrf_input_data = unsafe { slice::from_raw_parts(vrf_input_data_bytes, vrf_input_data_len) };
    let aux_data = unsafe { slice::from_raw_parts(aux_data_bytes, aux_data_len) };

    // Deserialize the ring set from the provided bytes
    let ring_set_slice = unsafe { slice::from_raw_parts(ring_set_bytes, ring_set_len) };
    let mut ring_set: Vec<Public> = Vec::new();
    let _params = ring_proof_params(ring_size);
    let  padding_point = Public::from(RingProofParams::padding_point());
    for i in 0..ring_size {
        let pubkey_bytes = &ring_set_slice[i * 32..(i + 1) * 32];
        let public_key = if pubkey_bytes.iter().all(|&b| b == 0) {
            padding_point.clone()
        } else {
            match Public::deserialize_compressed_unchecked(pubkey_bytes) {
            Ok(public_key) => public_key,
            Err(e) => {
                eprintln!("Failed to deserialize public key: {}", e);
                return 0;
            }
            }
        };
        ring_set.push(public_key);
    }

    // find our key in the ring
    // Derive the public key from the private key
    let derived_public_key = private_key.public();

    // Serialize the derived public key to bytes
    let mut derived_pk_bytes = Vec::new();
    if let Err(e) = derived_public_key.serialize_compressed(&mut derived_pk_bytes) {
        eprintln!("Failed to serialize derived public key: {}", e);
        return 0;
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
            return 0;
        }
    };
    // Create the Prover instance
    let prover = Prover {
        prover_idx:derived_prover_idx,
        secret: private_key,
        ring: ring_set,
    };

    // Use the Prover instance to sign the data
    let (vrf_output_hash, signature) = match prover.ring_vrf_sign(vrf_input_data, aux_data) {
        Ok(result) => result,
        Err(e) => {
            eprintln!("Failed to sign the data: {:?}", e);
            return 0;
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
    return 1;
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
    let prover_public_slice = unsafe { slice::from_raw_parts(prover_public_bytes, prover_public_len) };
    let signature_slice = unsafe { slice::from_raw_parts(signature_bytes, signature_len) };
    let vrf_input_data_slice = unsafe { slice::from_raw_parts(vrf_input_data_bytes, vrf_input_data_len) };
    let aux_data_slice = unsafe { slice::from_raw_parts(aux_data_bytes, aux_data_len) };

    let prover_public = match Public::deserialize_compressed_unchecked(prover_public_slice) {
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
    use ark_vrf::ietf::Verifier as _;

    let signature = match IetfVrfSignature::deserialize_compressed_unchecked(signature) {
        Ok(signature) => signature,
        Err(e) => {
            eprintln!("Failed to deserialize IETF VRF signature: {}", e);
            return Err(());
        }
    };

    let input = vrf_input_point(vrf_input_data);
    let output = signature.output;

    if prover_public
        .verify(input, output, aux_data, &signature.proof)
        .is_err()
    {
        return Err(());
    }

    let vrf_output_hash: [u8; 32] = match output.hash()[..32].try_into() {
        Ok(hash) => hash,
        Err(e) => {
            eprintln!("Failed to convert VRF output hash to array: {}", e);
            return Err(());
        }
    };

    Ok(vrf_output_hash)
}

#[no_mangle]
pub extern "C" fn get_ring_commitment(
    ring_set_bytes: *const c_uchar,
    ring_set_len: usize,
    ring_size: usize,
    commitment: *mut c_uchar,
    commitment_len: usize,
) {
    //use std::slice;
    use std::ptr;
    //use ark_serialize::CanonicalDeserialize;

    // Deserialize the ring set from the provided bytes
    let ring_set_slice = unsafe { slice::from_raw_parts(ring_set_bytes, ring_set_len) };
    let mut ring_set: Vec<Public> = Vec::new();
    let _params = ring_proof_params(ring_size);
    let  padding_point = Public::from(RingProofParams::padding_point());
    for i in 0..ring_size {
        let pubkey_bytes = &ring_set_slice[i * 32..(i + 1) * 32];
        let public_key = if pubkey_bytes.iter().all(|&b| b == 0) {
            padding_point.clone()
        } else {
            match Public::deserialize_compressed_unchecked(pubkey_bytes) {
            Ok(public_key) => public_key,
            Err(e) => {
                eprintln!("Failed to deserialize public key: {}", e);
                return;
            }
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
    ring_size: usize,
    signature_bytes: *const c_uchar,
    signature_hex_len: usize,
    vrf_input_data_bytes: *const c_uchar,
    vrf_input_data_len: usize,
    aux_data_bytes: *const c_uchar,
    aux_data_len: usize,
    vrf_output: *mut c_uchar,
    vrf_output_len: usize
) -> c_int {

    // Convert input pointers to slices
    let _pubkeys_slice = unsafe { slice::from_raw_parts(pubkeys_bytes, pubkeys_length) };
    let signature_slice = unsafe { slice::from_raw_parts(signature_bytes, signature_hex_len) };
    let vrf_input_data_slice =
        unsafe { slice::from_raw_parts(vrf_input_data_bytes, vrf_input_data_len) };
    let aux_data_slice = if aux_data_len == 0 {
        &[]
    } else {
        unsafe { slice::from_raw_parts(aux_data_bytes, aux_data_len) }
    };

    // each pubkey is 32 bytes
    let mut ring_set: Vec<Public> = Vec::new();
    let _params = ring_proof_params(ring_size);
    let  padding_point = Public::from(RingProofParams::padding_point());
    let pubkeys_slice = unsafe { slice::from_raw_parts(pubkeys_bytes, pubkeys_length) };
    for i in 0..ring_size {
        let pubkey_bytes = &pubkeys_slice[i * 32..(i + 1) * 32];
        let public_key = if pubkey_bytes.iter().all(|&b| b == 0) {
            padding_point.clone()
        } else {
            match Public::deserialize_compressed_unchecked(pubkey_bytes) {
            Ok(public_key) => public_key,
            Err(e) => {
                eprintln!("Failed to deserialize public key: {}", e);
                return 0;
            }
            }
        };
        ring_set.push(public_key);
    }

    // Create the Verifier
    let verifier = Verifier::new(ring_set);
    // Perform the verification
    let res = verifier.ring_vrf_verify(vrf_input_data_slice, aux_data_slice, signature_slice);

    // Store the VRF output hash
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
        Err(_e) => {
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
    let signature = match RingVrfSignature::deserialize_compressed_unchecked(signature_slice) {
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
pub extern "C" fn get_secret_key(
    seed_bytes: *const c_uchar,
    seed_len: usize,
    secret_key: *mut c_uchar,
    secret_key_len: usize,
) {
    let seed_slice = unsafe { std::slice::from_raw_parts(seed_bytes, seed_len) };
    let secret = BlsSecretKey::<TinyBLS<Bls12_381, ark_bls12_381::Config>>::from_seed(&seed_slice);
    let mut secret_key_bytes = secret.to_bytes();
    
    if secret_key_bytes.len() != secret_key_len {
        eprintln!("Buffer size mismatch for secret key: expected {}, got {}", 
                  secret_key_bytes.len(), secret_key_len);
        return;
    }
    
    unsafe {
        std::ptr::copy(secret_key_bytes.as_mut_ptr(), secret_key, secret_key_len);
    }
}

#[no_mangle]
pub extern "C" fn get_double_pubkey(
    seed_bytes: *const c_uchar,
    seed_len: usize,
    pub_key: *mut c_uchar,
    pub_key_len: usize,
) {
    let seed_slice = unsafe { std::slice::from_raw_parts(seed_bytes, seed_len) };
    let secret = BlsSecretKey::<TinyBLS<Bls12_381, ark_bls12_381::Config>>::from_seed(&seed_slice);
    let secret_vt = secret.into_vartime();
    let double_public_key = secret_vt.into_double_public_key();
    let mut pub_key_bytes = double_public_key.to_bytes();
    
    if pub_key_bytes.len() != pub_key_len {
        eprintln!("Buffer size mismatch for public key: expected {}, got {}",
                  pub_key_bytes.len(), pub_key_len);
        return;
    }
    
    unsafe {
        std::ptr::copy(pub_key_bytes.as_mut_ptr(), pub_key, pub_key_len);
    }
}

#[no_mangle]
pub extern "C" fn get_pubkey_g2(
    seed_bytes: *const c_uchar,
    seed_len: usize,
    pub_key: *mut c_uchar,
    pub_key_len: usize,
){
    let seed_slice = unsafe { std::slice::from_raw_parts(seed_bytes, seed_len) };
    let secret = BlsSecretKey::<TinyBLS<Bls12_381, ark_bls12_381::Config>>::from_seed(&seed_slice);
    let public_key = secret.into_public();
    let mut pub_key_bytes = public_key.to_bytes();
    if pub_key_bytes.len() != pub_key_len {
        eprintln!("Provided buffer is too small for the public key");
        return;
    }
    unsafe {
        std::ptr::copy(pub_key_bytes.as_mut_ptr(), pub_key, pub_key_len);
    }
}

#[no_mangle]
pub extern "C" fn sign(
    secret_key_bytes: *const c_uchar,
    secret_key_len: usize,
    message_bytes: *const c_uchar,
    message_len: usize,
    signature: *mut c_uchar,
    signature_len: usize,
){
    let secret_key_slice = unsafe { std::slice::from_raw_parts(secret_key_bytes, secret_key_len) };
    let mut secret = BlsSecretKey::<TinyBLS381>::from_bytes(&secret_key_slice).unwrap();
    let message_slice = unsafe { std::slice::from_raw_parts(message_bytes, message_len) };
    let message = Message::from(message_slice);

    let signature_bytes = secret.sign_once(&message).to_bytes();
    
    if signature_bytes.len() != signature_len {
        eprintln!("Provided buffer is too small for the signature");
        return;
    }
    unsafe {
        std::ptr::copy(signature_bytes.as_ptr(), signature, signature_len);
    }
}

#[no_mangle]
pub extern "C" fn verify(
    pub_key_bytes: *const c_uchar,
    pub_key_len: usize,
    message_bytes: *const c_uchar,
    message_len: usize,
    signature_bytes: *const c_uchar,
    signature_len: usize,
) -> c_int {
    let pub_key_slice = unsafe { std::slice::from_raw_parts(pub_key_bytes, pub_key_len) };
    let public_key = BlsPublicKey::<TinyBLS381>::from_bytes(&pub_key_slice).unwrap();
    let message_slice = unsafe { std::slice::from_raw_parts(message_bytes, message_len) };
    let message = Message::from(message_slice);
    let signature_slice = unsafe { std::slice::from_raw_parts(signature_bytes, signature_len) };
    let signature = BlsSignature::<TinyBLS381>::from_bytes(&signature_slice).unwrap();
    
    if signature.verify(&message, &public_key) {
        return 1;
    } else {
        return 0;
    }
}

#[no_mangle]
pub extern "C" fn aggregate_sign (
    signatures_bytes: *const c_uchar,
    signatures_len: usize,
    message_bytes: *const c_uchar,
    message_len: usize,
    aggregated_signature: *mut c_uchar,
    aggregated_signature_len: usize,
){
    let signatures_slice = unsafe { std::slice::from_raw_parts(signatures_bytes, signatures_len) };
    let message_slice = unsafe { std::slice::from_raw_parts(message_bytes, message_len) };
    let message = Message::from(message_slice);
    let mut prover_aggregator = SignatureAggregatorAssumingPoP::<TinyBLS381>::new(message.clone());
    for i in 0..signatures_slice.len()/48 {
        let signature = BlsSignature::<TinyBLS381>::from_bytes(&signatures_slice[i*48..(i+1)*48]).unwrap();
        prover_aggregator.add_signature(&signature);
    }
    let sig = (&prover_aggregator).signature();
    let sig_bytes = sig.to_bytes();
    if sig_bytes.len() != aggregated_signature_len {
        eprintln!("Provided buffer is too small for the aggregated signature");
        return;
    }
    unsafe {
        std::ptr::copy(sig_bytes.as_ptr(), aggregated_signature, aggregated_signature_len);
    }
}

#[no_mangle]
pub extern "C" fn aggregate_verify_by_signature (
    pub_keys_bytes: *const c_uchar,
    pub_keys_len: usize,
    message_bytes: *const c_uchar,
    message_len: usize,
    signature_bytes: *const c_uchar,
    signature_len: usize,
) -> c_int {
    let pub_keys_slice = unsafe { std::slice::from_raw_parts(pub_keys_bytes, pub_keys_len) };
    let message_slice = unsafe { std::slice::from_raw_parts(message_bytes, message_len) };
    let message = Message::from(message_slice);
    let mut verifier_aggregator = SignatureAggregatorAssumingPoP::<TinyBLS381>::new(message.clone());
    let mut aggregated_public_key = <TinyBLS381 as EngineBLS>::PublicKeyGroup::zero();
    for i in 0..pub_keys_slice.len()/144 {
        let pub_key = DoublePublicKey::<TinyBLS381>::from_bytes(&pub_keys_slice[i*144..(i+1)*144]).unwrap();
        verifier_aggregator.add_auxiliary_public_key(&PublicKeyInSignatureGroup(pub_key.0));
        let pub_key_g2_bytes = &pub_key.to_bytes()[48..144];
        let pub_key_g2 = BlsPublicKey::<TinyBLS381>::from_bytes(pub_key_g2_bytes).unwrap();
        aggregated_public_key += pub_key_g2.0;
    }
    let signature_slice = unsafe { std::slice::from_raw_parts(signature_bytes, signature_len) };
    let signature = BlsSignature::<TinyBLS381>::from_bytes(&signature_slice).unwrap();
    verifier_aggregator.add_signature(&signature);
    verifier_aggregator.add_publickey(&BlsPublicKey(aggregated_public_key));

    if verifier_aggregator.verify_using_aggregated_auxiliary_public_keys::<Sha256>() {
        return 1;
    } else {
        return 0;
    }
}

#[no_mangle]
pub extern "C" fn encode(
    input_ptr: *const c_uchar,
    input_len: usize,
    V: usize,
    output_ptr: *mut c_uchar,
    _shard_size: usize,
) {
    let C = V / 3;
    let W_E = C * 2;
    let data = unsafe { std::slice::from_raw_parts(input_ptr, input_len) };

    let mut padded = data.to_vec();
    if padded.len() % W_E != 0 {
        let pad_len = W_E - (padded.len() % W_E);
        padded.extend(std::iter::repeat(0).take(pad_len));
    }
    let shard_size = padded.len() / C;
    let k = padded.len() / W_E;
    let mut original_shards: Vec<Vec<u8>> = vec![Vec::with_capacity(2 * k); C];
    let mut recovery_shards: Vec<Vec<u8>> = vec![Vec::with_capacity(2 * k); C*2];
    for i in 0..k {
        let mut encoder = ReedSolomonEncoder::new(C, C*2, 2).unwrap();
        for c in 0..C {
            let shard = [
                padded[i * 2 + c * shard_size],
                padded[i * 2 + c * shard_size + 1],
            ];
            let _ = encoder.add_original_shard(&shard);
            original_shards[c].extend_from_slice(&shard);
        }

        let encoded = encoder.encode().unwrap();
        for (j, shard) in encoded.recovery_iter().enumerate() {
            recovery_shards[j].extend_from_slice(shard);
        }
    }

    let mut offset = 0;
    for shard in original_shards.iter().chain(recovery_shards.iter()) {
        unsafe {
            std::ptr::copy_nonoverlapping(shard.as_ptr(), output_ptr.add(offset), shard.len());
        }
        offset += shard.len();
    }
}

#[no_mangle]
pub extern "C" fn decode(
    shards_ptr: *const c_uchar,
    indexes_ptr: *const c_uint,
    V: usize,
    shard_size: usize,
    output_ptr: *mut c_uchar,
    _output_size: usize,  // For compatibility with BLS interface
) {
    let C = V / 3;
    let R = V - C;
    let k = shard_size / 2;
    let shards = unsafe { std::slice::from_raw_parts(shards_ptr, (C + R) * shard_size) };
    let indexes = unsafe { std::slice::from_raw_parts(indexes_ptr, C) };

    for i in 0..k {
        let mut decoder = ReedSolomonDecoder::new(C, R, 2).unwrap();
        for (j, &index) in indexes.iter().enumerate() {
            let shard_offset = j * shard_size + i * 2;
            let shard = &shards[shard_offset..shard_offset + 2];
            if (index as usize) < C {
                decoder.add_original_shard(index as usize, shard).unwrap();
                let offset = i * 2 + index as usize * shard_size;
                unsafe {
                    std::ptr::copy_nonoverlapping(shard.as_ptr(), output_ptr.add(offset), 2);
                }
            } else {
                decoder.add_recovery_shard(index as usize - C, shard).unwrap();
            }
        }

        {
            let result = decoder.decode();
            match result {
                Ok(decoded) => {
                    for (index, segment) in decoded.restored_original_iter() {
                        let offset = i * 2 + index * shard_size;
                        unsafe {
                            std::ptr::copy_nonoverlapping(segment.as_ptr(), output_ptr.add(offset), 2);
                        }
                    }
                }
                Err(_) => {
                    println!("⚠️ decode failed at row {}", i);
                }
            }
        }
    }
}

// FFI functions for verifier caching
#[no_mangle]
pub extern "C" fn create_verifier(
    ring_set_bytes: *const c_uchar,
    ring_set_len: usize,
    ring_size: usize,
) -> *mut Verifier {
    let ring_set_slice = unsafe { slice::from_raw_parts(ring_set_bytes, ring_set_len) };
    let mut ring_set: Vec<Public> = Vec::new();
    let _params = ring_proof_params(ring_size);
    let padding_point = Public::from(RingProofParams::padding_point());
    
    for i in 0..ring_size {
        let pubkey_bytes = &ring_set_slice[i * 32..(i + 1) * 32];
        let public_key = if pubkey_bytes.iter().all(|&b| b == 0) {
            padding_point.clone()
        } else {
            match Public::deserialize_compressed_unchecked(pubkey_bytes) {
                Ok(public_key) => public_key,
                Err(_) => return std::ptr::null_mut(),
            }
        };
        ring_set.push(public_key);
    }

    let verifier = Box::new(Verifier::new(ring_set));
    Box::into_raw(verifier)
}

#[no_mangle]
pub extern "C" fn ring_vrf_verify_with_verifier(
    verifier: *mut Verifier,
    signature_bytes: *const c_uchar,
    signature_len: usize,
    vrf_input_data_bytes: *const c_uchar,
    vrf_input_data_len: usize,
    aux_data_bytes: *const c_uchar,
    aux_data_len: usize,
    vrf_output: *mut c_uchar,
    _vrf_output_len: usize
) -> c_int {
    let verifier = unsafe { &*verifier };
    
    let signature_slice = unsafe { slice::from_raw_parts(signature_bytes, signature_len) };
    let vrf_input_data_slice = unsafe { slice::from_raw_parts(vrf_input_data_bytes, vrf_input_data_len) };
    let aux_data_slice = if aux_data_len == 0 {
        &[]
    } else {
        unsafe { slice::from_raw_parts(aux_data_bytes, aux_data_len) }
    };

    let res = verifier.ring_vrf_verify(vrf_input_data_slice, aux_data_slice, signature_slice);

    match res {
        Ok(vrf_output_hash) => {
            unsafe {
                std::ptr::copy_nonoverlapping(vrf_output_hash.as_ptr(), vrf_output, 32);
            }
            1
        }
        Err(_) => 0
    }
}

#[no_mangle]
pub extern "C" fn free_verifier(verifier: *mut Verifier) {
    if !verifier.is_null() {
        unsafe {
            let _ = Box::from_raw(verifier);
        }
    }
}

#[no_mangle]
pub extern "C" fn init_cache() {
    init_common_ring_sizes();
}

#[no_mangle]
pub extern "C" fn validate_bandersnatch_pubkey(
    pubkey_bytes: *const c_uchar,
    pubkey_len: usize,
) -> c_int {
    use std::ptr;

    // Convert pubkey bytes to a slice
    let pubkey_slice = unsafe { slice::from_raw_parts(pubkey_bytes, pubkey_len) };

    // Check if it's all zeros (padding point is valid)
    if pubkey_slice.iter().all(|&b| b == 0) {
        return 1; // Valid (padding point)
    }

    // Check correct length
    if pubkey_len != 32 {
        return 0; // Invalid length
    }

    // Try to deserialize the public key
    match Public::deserialize_compressed_unchecked(pubkey_slice) {
        Ok(_) => 1, // Valid public key
        Err(_) => 0, // Invalid public key
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use hex;
    use rand::Rng;
    const SCALAR_SIZE: usize = 32;
    const PUB_KEY_SIZE: usize = 32;
    // const IETF_SIGNATURE_SIZE: usize = 96;
    // const RING_SIGNATURE_SIZE: usize = 784;
    // const VRF_OUTPUT_SIZE: usize = 32;
    const SECRET_SIZE: usize = 64; // Adjust as per the actual size of the compressed secret
    macro_rules! _measure_time {
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

        let ring_size = 6;
        const SEED_SIZE: usize = 32;
        const SECRET_SIZE: usize = 32;

        // Step 1: Generate ring_size private keys
        let mut seeds: Vec<[u8; SEED_SIZE]> = Vec::new();
        for _i in 0..ring_size {
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
            let public_key = match Public::deserialize_compressed_unchecked(&pub_key_bytes[..]) {
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
        let result = ring_vrf_sign(
            private_keys[prover_idx].as_ptr(),
            private_keys[prover_idx].len(),
            ring_set_bytes.as_ptr(),
            ring_set_bytes.len(),
            ring_size,
            vrf_input_data.as_ptr(),
            vrf_input_data.len(),
            aux_data.as_ptr(),
            aux_data.len(),
            signature.as_mut_ptr(),
            signature.len(),
            vrf_output.as_mut_ptr(),
            vrf_output.len(),
        );

        assert_eq!(result, 1, "Failed to sign the data");

        println!("Signature: {:?}", hex::encode(&signature));
        println!("VRF Output: {:?}", hex::encode(&vrf_output));

        // Step 3: Verify the signature
        let result = ring_vrf_verify(
            ring_set_bytes.as_ptr(),
            ring_set_bytes.len(),
            ring_size,
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
        let secret = match Secret::deserialize_compressed_unchecked(&secret_slice[..]) {
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
        let ring_size = 6;
        // Step 1: Generate ring_size private keys
        let mut seeds: Vec<[u8; SEED_SIZE]> = Vec::new();
        for _i in 0..ring_size {
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
            let public_key = match Public::deserialize_compressed_unchecked(&pub_key_bytes[..]) {
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
            ring_size,
            commitment.as_mut_ptr(),
            commitment.len(),
        );

        println!("Commitment: {:?}", hex::encode(&commitment));
    }
}

