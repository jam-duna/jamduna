//! Consensus rule tests (Category 4): locktime, expiry, value conservation.

use orchard_service::transparent_parser::{compute_signature_hash_v5, TxInput, TxOutput, ZcashTxV5};
use orchard_service::transparent_script::parse_p2pkh_script_sig;
use orchard_service::transparent_utxo::{Outpoint, TransparentUtxoTree, UtxoData};
use orchard_service::transparent_verify::{verify_transparent_txs, TransparentTxData};
use libsecp256k1::{Message, PublicKey, SecretKey, sign};
use ripemd::Ripemd160;
use sha2::{Digest, Sha256};

const VERSION_V5: u32 = 0x8000_0005;
const VERSION_GROUP_ID: u32 = 0x26A7_270A;
const CONSENSUS_BRANCH_ID: u32 = 0xC2D6_D0B4;
const SIGHASH_ALL: u8 = 0x01;
const FINAL_SEQUENCE: u32 = 0xFFFF_FFFF;
const LOCK_TIME_THRESHOLD: u32 = 500_000_000;

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

fn sign_sighash(sk: &SecretKey, sighash: [u8; 32]) -> Vec<u8> {
    let msg = Message::parse(&sighash);
    let (sig, _) = sign(&msg, sk);
    sig.serialize_der().as_ref().to_vec()
}

fn build_signed_tx(
    lock_time: u32,
    expiry_height: u32,
    sequence: u32,
    input_value: u64,
    output_values: &[u64],
) -> (TransparentTxData, [u8; 32]) {
    let sk = SecretKey::parse(&[7u8; 32]).expect("secret key");
    let pk = PublicKey::from_secret_key(&sk).serialize_compressed();
    let script_pubkey = p2pkh_script_pubkey(&pk);

    let prevout_hash = [9u8; 32];
    let prevout_index = 0u32;

    let mut tx = ZcashTxV5 {
        version: VERSION_V5,
        version_group_id: VERSION_GROUP_ID,
        consensus_branch_id: CONSENSUS_BRANCH_ID,
        lock_time,
        expiry_height,
        inputs: vec![TxInput {
            prevout_hash,
            prevout_index,
            script_sig: Vec::new(),
            sequence,
        }],
        outputs: output_values
            .iter()
            .map(|value| TxOutput {
                value: *value,
                script_pubkey: script_pubkey.clone(),
            })
            .collect(),
        sapling_spends: Vec::new(),
        sapling_outputs: Vec::new(),
        value_balance_sapling: 0,
        sapling_anchor: [0u8; 32],
        orchard_actions: Vec::new(),
        flags_orchard: 0,
        value_balance_orchard: 0,
        orchard_anchor: [0u8; 32],
    };

    let sighash = compute_signature_hash_v5(
        &tx,
        0,
        SIGHASH_ALL,
        &[input_value],
        &[script_pubkey.clone()],
    )
    .expect("sighash");

    let mut sig = sign_sighash(&sk, sighash);
    sig.push(SIGHASH_ALL);

    let mut script_sig = push_data(&sig);
    script_sig.extend_from_slice(&push_data(&pk));

    let parsed = parse_p2pkh_script_sig(&script_sig).expect("scriptSig");
    tx.inputs[0].script_sig = script_sig;
    assert_eq!(parsed.hash_type, SIGHASH_ALL);

    let outpoint = Outpoint {
        txid: prevout_hash,
        vout: prevout_index,
    };
    let mut tree = TransparentUtxoTree::new();
    tree.insert(
        outpoint,
        UtxoData {
            value: input_value,
            script_pubkey: script_pubkey.clone(),
            height: 1,
            is_coinbase: false,
        },
    );
    let root = tree.get_root().expect("root");
    let proof = tree.get_proof(&outpoint).expect("proof");

    (
        TransparentTxData {
            transactions: vec![tx],
            utxo_proofs: vec![vec![proof]],
            utxo_snapshot: None,
        },
        root,
    )
}

#[test]
fn test_locktime_disabled_with_final_sequence() {
    let (tx_data, root) = build_signed_tx(200, 0, FINAL_SEQUENCE, 50_000, &[40_000]);
    assert!(
        verify_transparent_txs(&tx_data, &root, 100, 0).is_ok(),
        "final sequence should disable locktime"
    );
}

#[test]
fn test_locktime_block_based_valid() {
    let (tx_data, root) = build_signed_tx(100, 0, FINAL_SEQUENCE - 1, 50_000, &[40_000]);
    assert!(
        verify_transparent_txs(&tx_data, &root, 200, 0).is_ok(),
        "block height should satisfy locktime"
    );
}

#[test]
fn test_locktime_block_based_too_early() {
    let (tx_data, root) = build_signed_tx(200, 0, FINAL_SEQUENCE - 1, 50_000, &[40_000]);
    assert!(
        verify_transparent_txs(&tx_data, &root, 100, 0).is_err(),
        "block height below locktime should fail"
    );
}

#[test]
fn test_locktime_time_based_valid() {
    let (tx_data, root) = build_signed_tx(LOCK_TIME_THRESHOLD + 10, 0, FINAL_SEQUENCE - 1, 50_000, &[40_000]);
    assert!(
        verify_transparent_txs(&tx_data, &root, 1, LOCK_TIME_THRESHOLD + 1000).is_ok(),
        "block time should satisfy time-based locktime"
    );
}

#[test]
fn test_locktime_time_based_too_early() {
    let (tx_data, root) = build_signed_tx(LOCK_TIME_THRESHOLD + 1000, 0, FINAL_SEQUENCE - 1, 50_000, &[40_000]);
    assert!(
        verify_transparent_txs(&tx_data, &root, 1, LOCK_TIME_THRESHOLD + 100).is_err(),
        "block time below locktime should fail"
    );
}

#[test]
fn test_expiry_disabled() {
    let (tx_data, root) = build_signed_tx(0, 0, FINAL_SEQUENCE - 1, 50_000, &[40_000]);
    assert!(
        verify_transparent_txs(&tx_data, &root, 1, 0).is_ok(),
        "expiry disabled should pass"
    );
}

#[test]
fn test_expiry_not_expired() {
    let (tx_data, root) = build_signed_tx(0, 200, FINAL_SEQUENCE - 1, 50_000, &[40_000]);
    assert!(
        verify_transparent_txs(&tx_data, &root, 100, 0).is_ok(),
        "height below expiry should pass"
    );
}

#[test]
fn test_expiry_at_height_ok() {
    let (tx_data, root) = build_signed_tx(0, 200, FINAL_SEQUENCE - 1, 50_000, &[40_000]);
    assert!(
        verify_transparent_txs(&tx_data, &root, 200, 0).is_ok(),
        "height equal to expiry should pass"
    );
}

#[test]
fn test_expiry_expired() {
    let (tx_data, root) = build_signed_tx(0, 100, FINAL_SEQUENCE - 1, 50_000, &[40_000]);
    assert!(
        verify_transparent_txs(&tx_data, &root, 101, 0).is_err(),
        "height above expiry should fail"
    );
}

#[test]
fn test_value_conservation_exceeded() {
    let (tx_data, root) = build_signed_tx(0, 0, FINAL_SEQUENCE - 1, 50_000, &[50_001]);
    assert!(
        verify_transparent_txs(&tx_data, &root, 1, 0).is_err(),
        "outputs exceeding inputs should fail"
    );
}

#[test]
fn test_value_overflow() {
    let (tx_data, root) = build_signed_tx(0, 0, FINAL_SEQUENCE - 1, u64::MAX, &[u64::MAX, u64::MAX]);
    assert!(
        verify_transparent_txs(&tx_data, &root, 1, 0).is_err(),
        "output summation overflow should fail"
    );
}
