//! P2PKH signature verification tests (Category 3).

use orchard_service::transparent_parser::{compute_signature_hash_v5, TxInput, TxOutput, ZcashTxV5};
use orchard_service::transparent_script::{parse_p2pkh_script_sig, verify_p2pkh};
use libsecp256k1::{Message, PublicKey, SecretKey, sign};
use ripemd::Ripemd160;
use sha2::{Digest, Sha256};

const VERSION_V5: u32 = 0x8000_0005;
const VERSION_GROUP_ID: u32 = 0x26A7_270A;
const CONSENSUS_BRANCH_ID: u32 = 0xC2D6_D0B4;
const SIGHASH_ALL: u8 = 0x01;

fn hash160(data: &[u8]) -> [u8; 20] {
    let sha = Sha256::digest(data);
    let ripemd = Ripemd160::digest(sha);
    let mut out = [0u8; 20];
    out.copy_from_slice(&ripemd);
    out
}

fn p2pkh_script_pubkey(pubkey: &[u8]) -> Vec<u8> {
    let mut script = Vec::with_capacity(25);
    script.push(0x76); // OP_DUP
    script.push(0xa9); // OP_HASH160
    script.push(0x14); // Push 20 bytes
    script.extend_from_slice(&hash160(pubkey));
    script.push(0x88); // OP_EQUALVERIFY
    script.push(0xac); // OP_CHECKSIG
    script
}

fn push_data(data: &[u8]) -> Vec<u8> {
    let mut out = Vec::with_capacity(1 + data.len());
    out.push(data.len() as u8);
    out.extend_from_slice(data);
    out
}

fn build_tx(script_sig: Vec<u8>, output_script: Vec<u8>) -> ZcashTxV5 {
    ZcashTxV5 {
        version: VERSION_V5,
        version_group_id: VERSION_GROUP_ID,
        consensus_branch_id: CONSENSUS_BRANCH_ID,
        lock_time: 0,
        expiry_height: 0,
        inputs: vec![TxInput {
            prevout_hash: [1u8; 32],
            prevout_index: 0,
            script_sig,
            sequence: 0xFFFF_FFFE,
        }],
        outputs: vec![TxOutput {
            value: 40_000,
            script_pubkey: output_script,
        }],
        sapling_spends: Vec::new(),
        sapling_outputs: Vec::new(),
        value_balance_sapling: 0,
        sapling_anchor: [0u8; 32],
        orchard_actions: Vec::new(),
        flags_orchard: 0,
        value_balance_orchard: 0,
        orchard_anchor: [0u8; 32],
    }
}

fn sign_sighash(sk: &SecretKey, sighash: [u8; 32]) -> Vec<u8> {
    let msg = Message::parse(&sighash);
    let (sig, _) = sign(&msg, sk);
    sig.serialize_der().as_ref().to_vec()
}

#[test]
fn test_p2pkh_signature_valid() {
    let sk = SecretKey::parse(&[1u8; 32]).expect("secret key");
    let pk = PublicKey::from_secret_key(&sk).serialize_compressed();
    let script_pubkey = p2pkh_script_pubkey(&pk);

    let tx = build_tx(Vec::new(), script_pubkey.clone());
    let sighash = compute_signature_hash_v5(&tx, 0, SIGHASH_ALL, &[50_000], &[script_pubkey.clone()])
        .expect("sighash");

    let mut sig = sign_sighash(&sk, sighash);
    sig.push(SIGHASH_ALL);

    let mut script_sig = push_data(&sig);
    script_sig.extend_from_slice(&push_data(&pk));

    let parsed = parse_p2pkh_script_sig(&script_sig).expect("parse scriptSig");
    verify_p2pkh(&parsed.pubkey, &parsed.signature, &script_pubkey, &sighash)
        .expect("valid signature");
}

#[test]
fn test_p2pkh_signature_invalid() {
    let sk = SecretKey::parse(&[2u8; 32]).expect("secret key");
    let pk = PublicKey::from_secret_key(&sk).serialize_compressed();
    let script_pubkey = p2pkh_script_pubkey(&pk);

    let tx = build_tx(Vec::new(), script_pubkey.clone());
    let sighash = compute_signature_hash_v5(&tx, 0, SIGHASH_ALL, &[50_000], &[script_pubkey.clone()])
        .expect("sighash");

    let mut sig = sign_sighash(&sk, sighash);
    if let Some(last) = sig.last_mut() {
        *last ^= 0x01;
    }
    sig.push(SIGHASH_ALL);

    let mut script_sig = push_data(&sig);
    script_sig.extend_from_slice(&push_data(&pk));

    let parsed = parse_p2pkh_script_sig(&script_sig).expect("parse scriptSig");
    assert!(
        verify_p2pkh(&parsed.pubkey, &parsed.signature, &script_pubkey, &sighash).is_err(),
        "expected signature verification failure"
    );
}

#[test]
fn test_p2pkh_wrong_pubkey() {
    let sk_expected = SecretKey::parse(&[3u8; 32]).expect("secret key");
    let pk_expected = PublicKey::from_secret_key(&sk_expected).serialize_compressed();
    let script_pubkey = p2pkh_script_pubkey(&pk_expected);

    let sk_wrong = SecretKey::parse(&[4u8; 32]).expect("secret key");
    let pk_wrong = PublicKey::from_secret_key(&sk_wrong).serialize_compressed();

    let tx = build_tx(Vec::new(), script_pubkey.clone());
    let sighash = compute_signature_hash_v5(&tx, 0, SIGHASH_ALL, &[50_000], &[script_pubkey.clone()])
        .expect("sighash");

    let mut sig = sign_sighash(&sk_wrong, sighash);
    sig.push(SIGHASH_ALL);

    let mut script_sig = push_data(&sig);
    script_sig.extend_from_slice(&push_data(&pk_wrong));

    let parsed = parse_p2pkh_script_sig(&script_sig).expect("parse scriptSig");
    assert!(
        verify_p2pkh(&parsed.pubkey, &parsed.signature, &script_pubkey, &sighash).is_err(),
        "expected pubkey hash mismatch"
    );
}

#[test]
fn test_p2pkh_malformed_der() {
    let sk = SecretKey::parse(&[5u8; 32]).expect("secret key");
    let pk = PublicKey::from_secret_key(&sk).serialize_compressed();
    let script_pubkey = p2pkh_script_pubkey(&pk);

    let tx = build_tx(Vec::new(), script_pubkey.clone());
    let sighash = compute_signature_hash_v5(&tx, 0, SIGHASH_ALL, &[50_000], &[script_pubkey.clone()])
        .expect("sighash");

    let mut sig = vec![0x01, 0x02, 0x03, 0x04, 0x05];
    sig.push(SIGHASH_ALL);

    let mut script_sig = push_data(&sig);
    script_sig.extend_from_slice(&push_data(&pk));

    let parsed = parse_p2pkh_script_sig(&script_sig).expect("parse scriptSig");
    assert!(
        verify_p2pkh(&parsed.pubkey, &parsed.signature, &script_pubkey, &sighash).is_err(),
        "expected DER parsing failure"
    );
}
