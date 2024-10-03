use sha2::Sha256;
use ark_bls12_381::Bls12_381;
use ark_ff::Zero;
use rand::thread_rng;
use hex;
use w3f_bls::serialize::SerializableToBytes;
use w3f_bls::{
    double::{DoublePublicKeyScheme, DoubleSignature,DoublePublicKey},
    single_pop_aggregator::SignatureAggregatorAssumingPoP, EngineBLS, Keypair, Message, PublicKey, PublicKeyInSignatureGroup, Signed, TinyBLS, TinyBLS381,
};
use w3f_bls::Signature;
use w3f_bls::{SecretKey, BLSPoP};
use std::ptr;
use std::slice;
use ark_serialize::CanonicalSerialize;
use ark_serialize::CanonicalDeserialize;
use rand::Rng;
use std::os::raw::{c_int, c_uchar};

#[no_mangle]
pub extern "C" fn get_secret_key(
    seed_bytes: *const c_uchar,
    seed_len: usize,
    secret_key: *mut c_uchar,
    secret_key_len: usize,
){
    // Convert seed bytes to a slice
    let seed_slice = unsafe { slice::from_raw_parts(seed_bytes, seed_len) };
    let secret = SecretKey::<TinyBLS<Bls12_381, ark_bls12_381::Config>>::from_seed(&seed_slice);
    let mut secret_key_bytes = secret.to_bytes();
    if  secret_key_bytes.len()!=secret_key_len{
        eprintln!("Provided buffer is too small for the secret key");
        return;
    }
    unsafe {
        ptr::copy(secret_key_bytes.as_mut_ptr(), secret_key, secret_key_len);
    }
}


#[no_mangle]
pub extern "C" fn get_double_pubkey(
    seed_bytes: *const c_uchar,
    seed_len: usize,
    pub_key: *mut c_uchar,
    pub_key_len: usize,
){
    // Convert seed bytes to a slice
    let seed_slice = unsafe { slice::from_raw_parts(seed_bytes, seed_len) };
    let secret = SecretKey::<TinyBLS<Bls12_381, ark_bls12_381::Config>>::from_seed(&seed_slice);
    let secret_vt = secret.into_vartime();
    let double_public_key = secret_vt.into_double_public_key();
    let mut pub_key_bytes = double_public_key.to_bytes();
    if  pub_key_bytes.len()!=pub_key_len{
        eprintln!("Provided buffer is too small for the public key");
        return;
    }
    unsafe {
        ptr::copy(pub_key_bytes.as_mut_ptr(), pub_key, pub_key_len);
    }
}

#[no_mangle]
pub extern "C" fn get_pubkey_g2(
    seed_bytes: *const c_uchar,
    seed_len: usize,
    pub_key: *mut c_uchar,
    pub_key_len: usize,
){
    // Convert seed bytes to a slice
    let seed_slice = unsafe { slice::from_raw_parts(seed_bytes, seed_len) };
    let secret = SecretKey::<TinyBLS<Bls12_381, ark_bls12_381::Config>>::from_seed(&seed_slice);
    let public_key = secret.into_public();
    let mut pub_key_bytes = public_key.to_bytes();
    if  pub_key_bytes.len()!=pub_key_len{
        eprintln!("Provided buffer is too small for the public key");
        return;
    }
    unsafe {
        ptr::copy(pub_key_bytes.as_mut_ptr(), pub_key, pub_key_len);
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
    // Convert secret key bytes to a slice
    let secret_key_slice = unsafe { slice::from_raw_parts(secret_key_bytes, secret_key_len) };
    let mut secret = SecretKey::<TinyBLS381>::from_bytes(&secret_key_slice).unwrap();
    // Convert message bytes to a slice
    let message_slice = unsafe { slice::from_raw_parts(message_bytes, message_len) };
    let message = Message::from(message_slice);

    let signature_bytes = secret.sign_once(&message).to_bytes();
    
    if  signature_bytes.len()!=signature_len{
        eprintln!("Provided buffer is too small for the signature");
        return;
    }
    unsafe {
        ptr::copy(signature_bytes.as_ptr(), signature, signature_len);
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
    // Convert public key bytes to a slice
    let pub_key_slice = unsafe { slice::from_raw_parts(pub_key_bytes, pub_key_len) };
    let public_key = PublicKey::<TinyBLS381>::from_bytes(&pub_key_slice).unwrap();
    // Convert message bytes to a slice
    let message_slice = unsafe { slice::from_raw_parts(message_bytes, message_len) };
    let message = Message::from(message_slice);
    // Convert signature bytes to a slice
    let signature_slice = unsafe { slice::from_raw_parts(signature_bytes, signature_len) };
    let signature = Signature::<TinyBLS381>::from_bytes(&signature_slice).unwrap();
    
    if signature.verify(&message, &public_key) {
        return 1;
    } else {
        return 0;
    }
}

// we use the following functions to aggregate signatures
// aggregate_signatures, we pass the signatures(1 dimension array flatterned data)
// by using w3f bls, we can aggregate the signatures into one signature
#[no_mangle]
pub extern "C" fn aggregate_sign (
    signatures_bytes: *const c_uchar,
    signatures_len: usize,
    message_bytes: *const c_uchar,
    message_len: usize,
    aggregated_signature: *mut c_uchar,
    aggregated_signature_len: usize,
){
    // Convert signatures bytes to a slice
    let signatures_slice = unsafe { slice::from_raw_parts(signatures_bytes, signatures_len) };
    let message_slice = unsafe { slice::from_raw_parts(message_bytes, message_len) };
    let message = Message::from(message_slice);
    let mut prover_aggregator =SignatureAggregatorAssumingPoP::<TinyBLS381>::new(message.clone());
    for i in 0..signatures_slice.len()/48 {
        let signature = Signature::<TinyBLS381>::from_bytes(&signatures_slice[i*48..(i+1)*48]).unwrap();
        prover_aggregator.add_signature(&signature);
    }
    let sig = (&prover_aggregator).signature();
    let sig_bytes = sig.to_bytes();
    if  sig_bytes.len()!=aggregated_signature_len{
        eprintln!("Provided buffer is too small for the aggregated signature");
        return;
    }
    unsafe {
        ptr::copy(sig_bytes.as_ptr(), aggregated_signature, aggregated_signature_len);
    }
}
// aggregate_public_keys, we pass the public keys(1 dimension array flatterned data)
// by using w3f bls, we can aggregate the public keys into a aggregated public key
#[no_mangle]
pub extern "C" fn aggregate_verify_by_signature (
    pub_keys_bytes: *const c_uchar,
    pub_keys_len: usize,
    message_bytes: *const c_uchar,
    message_len: usize,
    signature_bytes: *const c_uchar,
    signature_len: usize,
) -> c_int {
    // parse the message and public keys
    let pub_keys_slice = unsafe { slice::from_raw_parts(pub_keys_bytes, pub_keys_len) };
    let message_slice = unsafe { slice::from_raw_parts(message_bytes, message_len) };
    let message = Message::from(message_slice);
    let mut verifier_aggregator = SignatureAggregatorAssumingPoP::<TinyBLS381>::new(message.clone());
    let mut aggregated_public_key = <TinyBLS381 as EngineBLS>::PublicKeyGroup::zero();
    // pubkeys is double public keys, length 144 bytes
    for i in 0..pub_keys_slice.len()/144 {
        let pub_key = DoublePublicKey::<TinyBLS381>::from_bytes(&pub_keys_slice[i*144..(i+1)*144]).unwrap();
        verifier_aggregator.add_auxiliary_public_key(&PublicKeyInSignatureGroup(pub_key.0));
        let pub_key_g2_bytes = &pub_key.to_bytes()[48..144]; // Extract the G2 part of the double public key
        let pub_key_g2 = PublicKey::<TinyBLS381>::from_bytes(pub_key_g2_bytes).unwrap();
        aggregated_public_key += pub_key_g2.0;
    }
    let signature_slice = unsafe { slice::from_raw_parts(signature_bytes, signature_len) };
    let signature = Signature::<TinyBLS381>::from_bytes(&signature_slice).unwrap();
    verifier_aggregator.add_signature(&signature);
    verifier_aggregator.add_publickey(&PublicKey(aggregated_public_key));

    if verifier_aggregator.verify_using_aggregated_auxiliary_public_keys::<Sha256>() {
        return 1;
    } else {
        return 0;
    }

}

// test function
// case 1: generate a secret key and a public key
#[test]
fn test_key_generation() {
    // use the function to generate a secret key
    // use random seed
    let mut rng = thread_rng();
    let seed: Vec<u8> = (0..32).map(|_| rng.gen()).collect();
    let mut secret_key = vec![0u8; 32];
    get_secret_key(seed.as_ptr(), seed.len(), secret_key.as_mut_ptr(), secret_key.len());
    println!("secret key (hex): {}", hex::encode(&secret_key));
    println!("secret key length: {}", secret_key.len());
    // use the function to generate a public key
    let mut pub_key = vec![0u8; 144];
    get_double_pubkey(seed.as_ptr(), seed.len(), pub_key.as_mut_ptr(), pub_key.len());
    println!("public key (hex): {}", hex::encode(&pub_key));
    println!("public key length: {}", pub_key.len());
}
// case 2: sign a message and verify the signature
#[test]
fn test_sign_and_verify(){
    // use the function to generate a secret key
    // use random seed
    let mut rng = thread_rng();
    let seed: Vec<u8> = (0..32).map(|_| rng.gen()).collect();
    let mut secret_key = vec![0u8; 32];
    get_secret_key(seed.as_ptr(), seed.len(), secret_key.as_mut_ptr(), secret_key.len());
    // use the function to generate a public key
    let mut pub_key = vec![0u8; 96];
    get_pubkey_g2(seed.as_ptr(), seed.len(), pub_key.as_mut_ptr(), pub_key.len());
    // use the function to sign a message
    let message = b"ctxI'd far rather be happy than right any day.";
    let mut signature = vec![0u8; 48];
    sign(secret_key.as_ptr(), secret_key.len(), message.as_ptr(), message.len(), signature.as_mut_ptr(), signature.len());
    // use the function to verify the signature
    let result = verify(pub_key.as_ptr(), pub_key.len(), message.as_ptr(), message.len(), signature.as_ptr(), signature.len());
    assert_eq!(result, 1);
}

// case 3: aggregate signatures and verify the aggregated signature
#[test]
fn test_aggregate_signatures(){

    // Generate 6 secret keys and corresponding public keys
    let mut rng = thread_rng();
    let mut secret_keys = Vec::new();
    let mut pub_keys = Vec::new();
    for _ in 0..6 {
        let seed: Vec<u8> = (0..32).map(|_| rng.gen()).collect();
        let mut secret_key = vec![0u8; 32];
        get_secret_key(seed.as_ptr(), seed.len(), secret_key.as_mut_ptr(), secret_key.len());
        secret_keys.push(secret_key);

        let mut pub_key = vec![0u8; 144];
        get_double_pubkey(seed.as_ptr(), seed.len(), pub_key.as_mut_ptr(), pub_key.len());
        pub_keys.push(pub_key);
    }

    // Sign the message "colorful notion" with each secret key
    let message = b"colorful notion";
    let mut signatures = Vec::new();
    for secret_key in &secret_keys {
        let mut signature = vec![0u8; 48];
        sign(secret_key.as_ptr(), secret_key.len(), message.as_ptr(), message.len(), signature.as_mut_ptr(), signature.len());
        signatures.push(signature);
    }

    // Flatten the signatures into a single array
    let flattened_signatures: Vec<u8> = signatures.iter().flat_map(|s| s.iter()).cloned().collect();

    // Aggregate the signatures
    let mut aggregated_signature = vec![0u8; 48];
    aggregate_sign(
        flattened_signatures.as_ptr(),
        flattened_signatures.len(),
        message.as_ptr(),
        message.len(),
        aggregated_signature.as_mut_ptr(),
        aggregated_signature.len(),
    );

    // Flatten the public keys into a single array
    let flattened_pub_keys: Vec<u8> = pub_keys.iter().flat_map(|pk| pk.iter()).cloned().collect();
    
    // Verify the aggregated signature
    let result = aggregate_verify_by_signature(
        flattened_pub_keys.as_ptr(),
        flattened_pub_keys.len(),
        message.as_ptr(),
        message.len(),
        aggregated_signature.as_ptr(),
        aggregated_signature.len(),
    );


    assert_eq!(result, 1);

    // use the wrong massage to verify the aggregated signature
    // it should return 0
    let message = b"handsome carlos";
    let result = aggregate_verify_by_signature(
        flattened_pub_keys.as_ptr(),
        flattened_pub_keys.len(),
        message.as_ptr(),
        message.len(),
        aggregated_signature.as_ptr(),
        aggregated_signature.len(),
    );

    assert_eq!(result, 0);
}