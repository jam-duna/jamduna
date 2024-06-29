// Copyright (c) 2022-2023 Web 3 Foundation

#![cfg_attr(not(feature = "std"), no_std)]
//#![deny(unsafe_code)]
#![doc = include_str!("../README.md")]

pub mod ring;
pub mod zcash_consts;

use ark_ff::MontFp;
use ark_ec::{
    AffineRepr, CurveGroup,
    // hashing::{HashToCurveError, curve_maps, map_to_curve_hasher::MapToCurveBasedHasher, HashToCurve},
};
use ark_std::vec::Vec;   // io::{Read, Write}
use ark_std::iter;
use ring::{KZG};

pub use ark_serialize::{CanonicalSerialize, CanonicalDeserialize, SerializationError, Compress, Validate};

#[cfg(not(feature = "substrate-curves"))]
mod curves {
    pub use ark_ed_on_bls12_381_bandersnatch as bandersnatch;
    pub use ark_bls12_381 as bls12_381;
}

#[cfg(feature = "substrate-curves")]
mod curves {
    pub use sp_ark_ed_on_bls12_381_bandersnatch as bandersnatch;
    pub use sp_ark_bls12_381 as bls12_381;
}

pub use curves::*;

// Conversion discussed in https://github.com/arkworks-rs/curves/pull/76#issuecomment-929121470

pub use dleq_vrf::{
    Transcript, IntoTranscript, transcript,
    error::{SignatureResult, SignatureError},
    vrf::{self, IntoVrfInput},
    EcVrfSecret,EcVrfSigner,EcVrfVerifier,
    VrfSignatureVec,
    scale,
};

use bandersnatch::SWAffine as Jubjub;

pub type VrfInput = dleq_vrf::vrf::VrfInput<Jubjub>;
pub type VrfPreOut = dleq_vrf::vrf::VrfPreOut<Jubjub>;
pub type VrfInOut = dleq_vrf::vrf::VrfInOut<Jubjub>;

pub struct Message<'a> {
    pub domain: &'a [u8],
    pub message: &'a [u8],
}

/*
type H2C = MapToCurveBasedHasher::<
G1Projective,
    DefaultFieldHasher<sha2::Sha256>,
    curve_maps::wb::WBMap<curve::g1::Config>,
>;

pub fn hash_to_bandersnatch_curve(domain: &[u8],message: &[u8]) -> Result<VrfInput,HashToCurveError> {
    dleq_vrf::vrf::ark_hash_to_curve::<Jubjub,H2C>(domain,message)
}
*/

impl<'a> IntoVrfInput<Jubjub> for Message<'a> {
    fn into_vrf_input(self) -> VrfInput {
        let label = b"TemporaryDoNotDeploy".as_ref();
        let mut t = Transcript::new_labeled(label);
        t.label(b"domain");
        t.append(self.domain);
        t.label(b"message");
        t.append(self.message);
        let p: <Jubjub as AffineRepr>::Group = t.challenge(b"vrf-input").read_uniform();
        vrf::VrfInput(p.into_affine())
    }
}

pub const BLINDING_BASE: Jubjub = {
    const X: bandersnatch::Fq = MontFp!("4956610287995045830459834427365747411162584416641336688940534788579455781570");
    const Y: bandersnatch::Fq = MontFp!("52360910621642801549936840538960627498114783432181489929217988668068368626761");
    Jubjub::new_unchecked(X, Y)
};


type ThinVrf = dleq_vrf::ThinVrf<Jubjub>;

/// Then VRF configured by the G1 generator for signatures.
pub fn thin_vrf() -> ThinVrf {
    dleq_vrf::ThinVrf::default()  //  keying_base: Jubjub::generator()
}

type PedersenVrf = dleq_vrf::PedersenVrf<Jubjub>;

/// Pedersen VRF configured by the G1 generator for public key certs.
pub fn pedersen_vrf() -> PedersenVrf {
    thin_vrf().pedersen_vrf([ BLINDING_BASE ])
}


pub type SecretKey = dleq_vrf::SecretKey<Jubjub>;

pub const PUBLIC_KEY_LENGTH: usize = 33;
pub type PublicKeyBytes = [u8; PUBLIC_KEY_LENGTH];

pub type PublicKey = dleq_vrf::PublicKey<Jubjub>;

pub fn serialize_publickey(pk: &PublicKey) -> PublicKeyBytes {
    let mut bytes = [0u8; PUBLIC_KEY_LENGTH];
    pk.serialize_compressed(bytes.as_mut_slice())
    .expect("Curve needs more than 33 bytes compressed!");
    bytes
}

pub fn deserialize_publickey(reader: &[u8]) -> Result<PublicKey, SerializationError> {
    PublicKey::deserialize_compressed(reader)
}


type ThinVrfProof = dleq_vrf::Batchable<ThinVrf>;

pub type ThinVrfSignature<const N: usize> = dleq_vrf::VrfSignature<ThinVrfProof,N>;


type PedersenVrfProof = dleq_vrf::Batchable<PedersenVrf>;

#[derive(Clone,CanonicalSerialize,CanonicalDeserialize)]
pub struct RingVrfProof {
    pub dleq_proof: PedersenVrfProof,
    pub ring_proof: ring::RingProof,
}

impl dleq_vrf::EcVrfProof for RingVrfProof {
    type H = Jubjub;
}

// TODO: Can you impl Debug+Eq+PartialEq for ring::RingProof please Sergey?  We'll then derive Debug.
mod tmp {
    use ark_std::fmt::{Debug,Formatter,Error};
    impl Debug for crate::RingVrfProof {
        fn fmt(&self, f: &mut Formatter<'_>) -> Result<(), Error> {
            self.dleq_proof.fmt(f)
        }
    }
    impl Eq for crate::RingVrfProof {}
    impl PartialEq for crate::RingVrfProof {
        fn eq(&self, other: &Self) -> bool {
            // Ignore ring_proof for now
            self.dleq_proof == other.dleq_proof
        }
    }
}

impl scale::ArkScaleMaxEncodedLen for RingVrfProof {
    fn max_encoded_len(compress: Compress) -> usize {
        <PedersenVrfProof as scale::ArkScaleMaxEncodedLen>::max_encoded_len(compress)
        + 4096  // TODO: How large is RingProof, Sergey?
    }
}

// TODO: Sergey, should this be #[derive(Debug,Clone)] ?
pub struct RingVerifier<'a>(pub &'a ring::RingVerifier);

//TODO. need a way to Initialize this from signature
pub type RingVrfSignature<const N: usize> = dleq_vrf::VrfSignature<RingVrfProof,N>;

impl EcVrfVerifier for RingVerifier<'_> {
    type Proof = RingVrfProof;
    type Error = SignatureError;

    fn vrf_verify_detached<'a>(
        &self,
        t: impl IntoTranscript,
        ios: &'a [VrfInOut],
        signature: &RingVrfProof,
    ) -> Result<&'a [VrfInOut],Self::Error> {
        let ring_verifier = &self.0;
        pedersen_vrf().verify_pedersen_vrf(t,ios.as_ref(),&signature.dleq_proof) ?;

        let key_commitment = signature.dleq_proof.as_key_commitment();
        match ring_verifier.verify_ring_proof(signature.ring_proof.clone(), key_commitment.0.clone()) {
            true => Ok(ios),
            false => Err(SignatureError::Invalid),
        }
    }
}

impl RingVerifier<'_> {
    pub fn verify_ring_vrf<const N: usize>(
        &self,
        t: impl IntoTranscript,
        inputs: impl IntoIterator<Item = impl IntoVrfInput<Jubjub>>,
        signature: &RingVrfSignature<N>,
    ) -> Result<[VrfInOut; N],SignatureError>
    {
        self.vrf_verify(t, inputs, signature)
    }
}


// #[derive(Clone)]
pub struct RingProver<'a> {
    pub ring_prover: &'a ring::RingProver,
    pub secret: &'a SecretKey,
}

impl<'a> core::borrow::Borrow<SecretKey> for RingProver<'a> {
    fn borrow(&self) -> &SecretKey { &self.secret }
}

impl<'a> EcVrfSigner for RingProver<'a> {
    type Proof = RingVrfProof;
    type Error = ();
    type Secret = SecretKey;
    fn vrf_sign_detached(
        &self,
        t: impl IntoTranscript,
        ios: &[VrfInOut]
    ) -> Result<RingVrfProof,()>
    {
        let RingProver { ring_prover, secret } = *self;
        let secret_blinding = None; // TODO: Set this first so we can hash the ring proof
        let (dleq_proof,secret_blinding) = pedersen_vrf().sign_pedersen_vrf(t, ios, secret_blinding, secret);
        let ring_proof = ring_prover.prove(secret_blinding.0[0]);
        Ok(RingVrfProof { dleq_proof, ring_proof, })
    }
}

impl<'a> RingProver<'a> {
    pub fn sign_ring_vrf<const N: usize>(
        &self,
        t: impl IntoTranscript,
        ios: &[VrfInOut; N],
    ) -> RingVrfSignature<N>
    {
        self.vrf_sign(t, ios).expect("no failure modes")
    }
}

fn ring_test_init_with_pks(pks: Vec<PublicKey>, target_pk: PublicKey) -> (ring::RingProver, ring::RingVerifier) {
    // Ensure pks is not empty
    if pks.is_empty() {
        panic!("The list of public keys cannot be empty.");
    }

    let kzg = KZG::testing_kzg_setup([0; 32], 2u32.pow(10));
    let keyset_size = pks.len();

    // Ensure there are no more public keys than the max keyset size
    if keyset_size > kzg.max_keyset_size() {
        panic!("The number of public keys exceeds the maximum keyset size.");
    }

    // Convert target public key to bytes for comparison
    let mut target_pk_bytes = vec![];
    target_pk.serialize(&mut target_pk_bytes).unwrap();

    // Find the index of the target public key
    let secret_key_idx = match pks.iter().position(|pk| {
        let mut pk_bytes = vec![];
        pk.serialize(&mut pk_bytes).unwrap();
        pk_bytes == target_pk_bytes
    }) {
        Some(idx) => idx,
        None => panic!("The target public key is not in the list of public keys."),
    };

    // Convert public keys to the format expected by the prover and verifier
    let converted_pks: Vec<_> = pks.iter().map(|pk| pk.0.into()).collect();

    let prover_key = kzg.prover_key(converted_pks.clone());
    let ring_prover = kzg.init_ring_prover(prover_key, secret_key_idx);

    let verifier_key = kzg.verifier_key(converted_pks);
    let ring_verifier = kzg.init_ring_verifier(verifier_key);

    (ring_prover, ring_verifier)
}


// Extern "C" functions
#[no_mangle]
pub extern "C" fn ring_vrf_sign_c_pks(
    secret: *const u8,
    domain: *const u8,
    domain_len: usize,
    message: *const u8,
    message_len: usize,
    transcript: *const u8,
    transcript_len: usize,
    pks_ptr: *const u8,
    pks_len: usize,
    signature: *mut u8,
    signature_len: usize
) -> usize {
    // Deserialize the secret key
    let secret_slice = unsafe { std::slice::from_raw_parts(secret, 32) };
    let secret_array: &[u8; 32] = match secret_slice.try_into() {
        Ok(arr) => arr,
        Err(_) => return 7,
    };
    let secret_key = SecretKey::from_seed(secret_array);

    // Derive the public key from the secret key
    let derived_pubkey = secret_key.as_publickey();

    // Deserialize the public keys
    let pks_slice = unsafe { std::slice::from_raw_parts(pks_ptr, pks_len) };
    let pks: Vec<PublicKey> = pks_slice
        .chunks(33) // Assuming each public key is 33 bytes
        .map(|chunk| PublicKey::deserialize(chunk).expect("Invalid public key"))
        .collect();

    if pks.is_empty() {
        return 8; // Error code for empty public keys list
    }

    // Ensure there are no more public keys than the max keyset size
    let kzg = KZG::testing_kzg_setup([0; 32], 2u32.pow(10));
    let keyset_size = pks.len();

    if keyset_size > kzg.max_keyset_size() {
        return 9; // Error code for exceeding keyset size
    }

    // Find the index of the derived public key
    let mut derived_pubkey_bytes = vec![];
    derived_pubkey.serialize(&mut derived_pubkey_bytes).unwrap();

    let secret_key_idx = match pks.iter().position(|pk| {
        let mut pk_bytes = vec![];
        pk.serialize(&mut pk_bytes).unwrap();
        pk_bytes == derived_pubkey_bytes
    }) {
        Some(idx) => idx,
        None => return 12, // Error code for public key not found
    };
    println!("signer index: {}", secret_key_idx);

    // Convert public keys to the format expected by the prover and verifier
    let converted_pks: Vec<_> = pks.iter().map(|pk| pk.0.into()).collect();

    // Create the Message structure
    let domain_slice = unsafe { std::slice::from_raw_parts(domain, domain_len) };
    let message_slice = unsafe { std::slice::from_raw_parts(message, message_len) };
    let input = Message {
        domain: domain_slice,
        message: message_slice,
    }.into_vrf_input();

    // Compute VrfInOut
    let io = secret_key.vrf_inout(input.clone());

    // Create the transcript
    let transcript_slice = unsafe { std::slice::from_raw_parts(transcript, transcript_len) };

    // Initialize ring prover with provided public keys
    let prover_key = kzg.prover_key(converted_pks.clone());
    let ring_prover = kzg.init_ring_prover(prover_key, secret_key_idx);
    let ring_prover = RingProver {
        ring_prover: &ring_prover,
        secret: &secret_key,
    };

    // Sign the input and get the signature
    let signature_obj: RingVrfSignature<1> = ring_prover.sign_ring_vrf(transcript_slice, &[io]);

    // Serialize the signature
    let mut serialized_signature = vec![];
    if signature_obj.proof.dleq_proof.serialize(&mut serialized_signature).is_err() {
        return 10; // Error code for serialization failure
    }

    if serialized_signature.len() > signature_len {
        return 11; // Error code for insufficient signature buffer length
    }

    unsafe {
        std::ptr::copy_nonoverlapping(serialized_signature.as_ptr(), signature, serialized_signature.len());
    }

    // Initialize ring verifier with provided public keys
    let verifier_key = kzg.verifier_key(converted_pks);
    let ring_verifier = kzg.init_ring_verifier(verifier_key);
    let result = RingVerifier(&ring_verifier)
        .verify_ring_vrf(transcript_slice, iter::once(input), &signature_obj);

    if result.is_ok() {
        0 // Success
    } else {
        1 // Verification failed
    }
}

#[no_mangle]
pub extern "C" fn ring_vrf_verify_c_pks(
    domain: *const u8,
    domain_len: usize,
    message: *const u8,
    message_len: usize,
    transcript: *const u8,
    transcript_len: usize,
    pks_ptr: *const u8,
    pks_len: usize,
    signature_ptr: *const u8,
    signature_len: usize
) -> usize {
    println!("Received domain_len: {}", domain_len);
    println!("Received message_len: {}", message_len);
    println!("Received transcript_len: {}", transcript_len);
    println!("Received pks_len: {}", pks_len);
    println!("Received signature_len: {}", signature_len);

    // Deserialize the public keys
    let pks_slice = unsafe { std::slice::from_raw_parts(pks_ptr, pks_len) };
    println!("Public keys slice: {:?}", pks_slice);

    let pks: Vec<PublicKey> = match pks_slice
        .chunks(33) // Assuming each public key is 33 bytes
        .map(|chunk| {
            println!("Deserializing public key chunk: {:?}", chunk);
            PublicKey::deserialize(chunk)
        })
        .collect::<Result<Vec<_>, _>>() {
        Ok(keys) => keys,
        Err(e) => {
            eprintln!("Error deserializing public key: {:?}", e);
            return 10; // Error code for invalid public key
        }
    };

    if pks.is_empty() {
        eprintln!("Error: Public keys list is empty");
        return 8; // Error code for empty public keys list
    }

    // Ensure there are no more public keys than the max keyset size
    let kzg = KZG::testing_kzg_setup([0; 32], 2u32.pow(10));
    let keyset_size = pks.len();

    if keyset_size > kzg.max_keyset_size() {
        eprintln!("Error: Number of public keys exceeds the maximum keyset size");
        return 9; // Error code for exceeding keyset size
    }

    // Convert public keys to the format expected by the prover and verifier
    let converted_pks: Vec<_> = pks.iter().map(|pk| pk.0.into()).collect();
    println!("Converted public keys: {:?}", converted_pks);

    // Create the Message structure
    let domain_slice = unsafe { std::slice::from_raw_parts(domain, domain_len) };
    let message_slice = unsafe { std::slice::from_raw_parts(message, message_len) };
    println!("Domain slice: {:?}", domain_slice);
    println!("Message slice: {:?}", message_slice);

    let input = Message {
        domain: domain_slice,
        message: message_slice,
    }.into_vrf_input();

    // Create the transcript
    let transcript_slice = unsafe { std::slice::from_raw_parts(transcript, transcript_len) };
    println!("Transcript slice: {:?}", transcript_slice);

    // Deserialize the signature
    let signature_slice = unsafe { std::slice::from_raw_parts(signature_ptr, signature_len) };
    println!("Signature slice: {:?}", signature_slice);

    //TODO: how reconstruct signature_obj properly 
    let signature_obj: RingVrfSignature<1> = match RingVrfSignature::deserialize_with_mode(signature_slice, Compress::Yes, Validate::Yes) {
        Ok(sig) => sig,
        Err(e) => {
            eprintln!("Error: Invalid signature: {:?}", e);
            return 10; // Error code for invalid signature
        }
    };
    println!("Deserialized signature: {:?}", signature_obj);

    // Initialize ring verifier with provided public keys
    let verifier_key = kzg.verifier_key(converted_pks);
    let ring_verifier = kzg.init_ring_verifier(verifier_key);

    // Verify the signature using the ring verifier
    let result = RingVerifier(&ring_verifier)
        .verify_ring_vrf(transcript_slice, iter::once(input), &signature_obj);

    match result {
        Ok(_) => {
            println!("Signature verification succeeded");
            0 // Success
        },
        Err(e) => {
            eprintln!("Error: Verification failed: {:?}", e);
            1 // Verification failed
        }
    }
}

// Extern "C" functions
#[no_mangle]
pub extern "C" fn ring_vrf_sign_c(
    secret: *const u8,
    domain: *const u8,
    domain_len: usize,
    message: *const u8,
    message_len: usize,
    transcript: *const u8,
    transcript_len: usize,
    signature: *mut u8,
    signature_len: usize
) -> usize {
    use rand_core::RngCore;
    //use core::iter;
    //use ark_serialize::CanonicalSerialize;
    // Deserialize the secret key
    let secret_slice = unsafe { std::slice::from_raw_parts(secret, 32) };
    let secret_array: &[u8; 32] = match secret_slice.try_into() {
        Ok(arr) => arr,
        Err(_) => return 7,
    };

    // Deserialize the secret key
    let secret_key = SecretKey::from_seed(secret_array);

    // Create the Message structure
    let domain_slice = unsafe { std::slice::from_raw_parts(domain, domain_len) };
    let message_slice = unsafe { std::slice::from_raw_parts(message, message_len) };
    let input = Message {
        domain: domain_slice,
        message: message_slice,
    }.into_vrf_input();

    // Compute VrfInOut
    let io = secret_key.vrf_inout(input.clone());

    // Create the transcript
    let transcript_slice = unsafe { std::slice::from_raw_parts(transcript, transcript_len) };

    // Initialize ring prover
    use ark_std::UniformRand;

    let kzg = ring::KZG::testing_kzg_setup([0; 32], 2u32.pow(10));
    let keyset_size = kzg.max_keyset_size();

    let mut rng = rand_core::OsRng;
    let mut l = [0u8; 8];
    rng.fill_bytes(&mut l);
    let keyset_size = usize::from_le_bytes(l) % keyset_size;

    // Gen a bunch of random public keys
    let mut pks: Vec<_> = (0..keyset_size).map(|_| Jubjub::rand(&mut rng)).collect();
    // Just select one index for the actual key we are for signing
    let pk: PublicKey = secret_key.to_public();
    let secret_key_idx = keyset_size / 2;
    pks[secret_key_idx] = pk.0.into();

    let prover_key = kzg.prover_key(pks.clone());
    let ring_prover = kzg.init_ring_prover(prover_key, secret_key_idx);

    let ring_prover = RingProver {
        ring_prover: &ring_prover,
        secret: &secret_key,
    };
    // Sign the input and get the signature
    let signature_obj: RingVrfSignature<1> = ring_prover.sign_ring_vrf(transcript_slice, &[io]);

    // Serialize the signature in signature_obj
    let mut serialized_signature = vec![];
    if signature_obj.proof.dleq_proof.serialize(&mut serialized_signature).is_err() {
        return 8;
    }

    if serialized_signature.len() > signature_len {
        return 9;
    }

    unsafe {
        std::ptr::copy_nonoverlapping(serialized_signature.as_ptr(), signature, serialized_signature.len());
    }

    let kzg = ring::KZG::testing_kzg_setup([0; 32], 2u32.pow(10));
    let verifier_key = kzg.verifier_key(pks);
    let ring_verifier = kzg.init_ring_verifier(verifier_key);
    let result = RingVerifier(&ring_verifier)
        .verify_ring_vrf(transcript_slice, iter::once(input), &signature_obj);

    if result.is_ok() {
        0
    }  else {
        1
    }
}

#[no_mangle]
pub extern "C" fn ring_vrf_verify_c(
    _pks: *const u8,
    _pubkey: *const u8,
    _pubkey_len: usize,
    _domain: *const u8,
    _domain_len: usize,
    _message: *const u8,
    _message_len: usize,
    _transcript: *const u8,
    _transcript_len: usize,
    _signature: *const u8,
    _signature_len: usize
) -> u8 {
    // stub
    0
}

#[no_mangle]
pub extern "C" fn ring_vrf_public_key(
    secret: *const u8,
    pubkey: *mut u8,
    pubkey_len: usize
) -> usize {
    //use ark_serialize::CanonicalSerialize;

    // Deserialize the secret key
    let secret_slice = unsafe { std::slice::from_raw_parts(secret, 32) };
    let secret_array: &[u8; 32] = match secret_slice.try_into() {
        Ok(arr) => arr,
        Err(_) => return 1,
    };

    let secret_key = SecretKey::from_seed(secret_array);

    // Generate the public key
    let public_key = secret_key.to_public();
    let mut serialized_pubkey = vec![0u8; PUBLIC_KEY_LENGTH];
    public_key.serialize_compressed(serialized_pubkey.as_mut_slice())
        .expect("Serialization failed");

    // Check if the provided buffer is large enough
    if serialized_pubkey.len() > pubkey_len {
        return 2;
    }

    // Copy the serialized public key to the provided buffer
    unsafe {
        std::ptr::copy_nonoverlapping(serialized_pubkey.as_ptr(), pubkey, serialized_pubkey.len());
    }

    0 // Success
}

#[cfg(all(test, feature = "getrandom"))]
mod tests {
    use super::*;
    use core::iter;
    use ark_std::rand::RngCore;

    #[test]
    fn check_blinding_base() {
        let mut t = b"Bandersnatch VRF blinding base".into_transcript();
        let blinding_base: <Jubjub as AffineRepr>::Group = t.challenge(b"vrf-input").read_uniform();
        debug_assert_eq!(blinding_base.into_affine(), BLINDING_BASE);
    }

    #[test]
    fn good_max_encoded_len() {
        use dleq_vrf::scale::MaxEncodedLen;
        assert_eq!(crate::PUBLIC_KEY_LENGTH, <PublicKey as MaxEncodedLen>::max_encoded_len());
    }

    #[test]
    fn thin_sign_verify() {
        let secret = SecretKey::from_seed(&[0; 32]);
        let public = secret.to_public();
        assert_eq!(public.compressed_size(), PUBLIC_KEY_LENGTH);
        let public = serialize_publickey(&public);
        let public = deserialize_publickey(&public).unwrap();

        let input = Message {
            domain: b"domain",
            message: b"message",
        }.into_vrf_input();
        let io = secret.vrf_inout(input.clone());
        let transcript = Transcript::new_labeled(b"label");

        let signature: ThinVrfSignature<1> = secret.sign_thin_vrf(transcript.clone(), &[io.clone()]);

        let result = public.verify_thin_vrf(transcript, iter::once(input), &signature);

        assert!(result.is_ok());
        let io2 = result.unwrap();
        assert_eq!(io2[0].preoutput, io.preoutput);
    }

    fn ring_test_init(pk: PublicKey) -> (ring::RingProver, ring::RingVerifier) {
        use ark_std::UniformRand;

        let kzg = ring::KZG::testing_kzg_setup([0; 32], 2u32.pow(10));
        let keyset_size = kzg.max_keyset_size();

        let mut rng = rand_core::OsRng;
        let mut l = [0u8; 8];
        rng.fill_bytes(&mut l);
        let keyset_size = usize::from_le_bytes(l) % keyset_size;

        // Gen a bunch of random public keys
        let mut pks: Vec<_> = (0..keyset_size).map(|_| Jubjub::rand(&mut rng)).collect();
        // Just select one index for the actual key we are for signing
        let secret_key_idx = keyset_size / 2;
        pks[secret_key_idx] = pk.0.into();

        let prover_key = kzg.prover_key(pks.clone());
        let ring_prover = kzg.init_ring_prover(prover_key, secret_key_idx);

        let verifier_key = kzg.verifier_key(pks);
        let ring_verifier = kzg.init_ring_verifier(verifier_key);

        (ring_prover, ring_verifier)
    }

    #[test]
    fn ring_sign_verify() {
        let secret = & SecretKey::from_seed(&[0; 32]);
        let (ring_prover, ring_verifier) = ring_test_init(secret.to_public());

        let input = Message {
            domain: b"domain",
            message: b"message",
        }.into_vrf_input();
        let io = secret.vrf_inout(input.clone());
        let transcript: &[u8] = b"Meow";  // Transcript::new_labeled(b"label");

        let signature: RingVrfSignature<1> = RingProver {
            ring_prover: &ring_prover, secret,
        }.sign_ring_vrf(transcript, &[io]);

        // TODO: serialize signature

        let result = RingVerifier(&ring_verifier)
        .verify_ring_vrf(transcript, iter::once(input), &signature);
        assert!(result.is_ok());
    }
}
