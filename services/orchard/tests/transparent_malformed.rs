//! Malformed transaction tests (Category 5).

use libsecp256k1::{sign, Message, PublicKey, SecretKey};
use ripemd::Ripemd160;
use sha2::{Digest, Sha256};

use orchard_service::transparent_parser::{compute_signature_hash_v5, TxInput, TxOutput, ZcashTxV5};
use orchard_service::transparent_utxo::{Outpoint, TransparentUtxoTree, UtxoData, UtxoMerkleProof};
use orchard_service::transparent_verify::{verify_transparent_txs, TransparentTxData};

const VERSION_V5: u32 = 0x8000_0005;
const VERSION_GROUP_ID: u32 = 0x26A7_270A;
const CONSENSUS_BRANCH_ID: u32 = 0xC2D6_D0B4;
const FINAL_SEQUENCE: u32 = 0xFFFF_FFFF;
const SIGHASH_ALL: u8 = 0x01;
const MAX_INPUTS: usize = 1000;
const MAX_OUTPUTS: usize = 1000;

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
    input_value: u64,
    output_value: u64,
) -> (ZcashTxV5, UtxoMerkleProof, [u8; 32]) {
    let sk = SecretKey::parse(&[7u8; 32]).expect("secret key");
    let pk = PublicKey::from_secret_key(&sk).serialize_compressed();
    let script_pubkey = p2pkh_script_pubkey(&pk);

    let prevout_hash = [9u8; 32];
    let prevout_index = 0u32;

    let mut tx = ZcashTxV5 {
        version: VERSION_V5,
        version_group_id: VERSION_GROUP_ID,
        consensus_branch_id: CONSENSUS_BRANCH_ID,
        lock_time: 0,
        expiry_height: 0,
        inputs: vec![TxInput {
            prevout_hash,
            prevout_index,
            script_sig: Vec::new(),
            sequence: FINAL_SEQUENCE,
        }],
        outputs: vec![TxOutput {
            value: output_value,
            script_pubkey: script_pubkey.clone(),
        }],
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
    tx.inputs[0].script_sig = script_sig;

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

    (tx, proof, root)
}

fn build_double_spend_tx(
    input_value: u64,
    output_value: u64,
) -> (ZcashTxV5, UtxoMerkleProof, [u8; 32]) {
    let sk = SecretKey::parse(&[7u8; 32]).expect("secret key");
    let pk = PublicKey::from_secret_key(&sk).serialize_compressed();
    let script_pubkey = p2pkh_script_pubkey(&pk);

    let prevout_hash = [9u8; 32];
    let prevout_index = 0u32;

    let input0 = TxInput {
        prevout_hash,
        prevout_index,
        script_sig: Vec::new(),
        sequence: FINAL_SEQUENCE,
    };
    let input1 = TxInput {
        prevout_hash,
        prevout_index,
        script_sig: Vec::new(),
        sequence: FINAL_SEQUENCE,
    };

    let mut tx = ZcashTxV5 {
        version: VERSION_V5,
        version_group_id: VERSION_GROUP_ID,
        consensus_branch_id: CONSENSUS_BRANCH_ID,
        lock_time: 0,
        expiry_height: 0,
        inputs: vec![input0, input1],
        outputs: vec![TxOutput {
            value: output_value,
            script_pubkey: script_pubkey.clone(),
        }],
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
        &[input_value, input_value],
        &[script_pubkey.clone(), script_pubkey.clone()],
    )
    .expect("sighash");

    let mut sig = sign_sighash(&sk, sighash);
    sig.push(SIGHASH_ALL);

    let mut script_sig = push_data(&sig);
    script_sig.extend_from_slice(&push_data(&pk));
    tx.inputs[0].script_sig = script_sig;

    let outpoint = Outpoint {
        txid: prevout_hash,
        vout: prevout_index,
    };
    let mut tree = TransparentUtxoTree::new();
    tree.insert(
        outpoint,
        UtxoData {
            value: input_value,
            script_pubkey,
            height: 1,
            is_coinbase: false,
        },
    );
    let root = tree.get_root().expect("root");
    let proof = tree.get_proof(&outpoint).expect("proof");

    (tx, proof, root)
}

fn assert_err_contains(result: orchard_service::errors::Result<()>, expected: &str) {
    match result {
        Ok(_) => panic!("expected error containing {expected}"),
        Err(err) => {
            let message = err.to_string();
            assert!(
                message.contains(expected),
                "expected error containing {expected}, got {message}"
            );
        }
    }
}

#[test]
fn test_invalid_consensus_branch_id() {
    let (mut tx, proof, root) = build_signed_tx(50_000, 40_000);
    tx.consensus_branch_id = 0xDEAD_BEEF;

    let tx_data = TransparentTxData {
        transactions: vec![tx],
        utxo_proofs: vec![vec![proof]],
        utxo_snapshot: None,
    };

    assert_err_contains(
        verify_transparent_txs(&tx_data, &root, 100, 0),
        "unsupported consensus branch id",
    );
}

#[test]
fn test_too_many_inputs() {
    let script_pubkey = p2pkh_script_pubkey(&[0x02u8; 33]);
    let mut inputs = Vec::with_capacity(MAX_INPUTS + 1);
    for i in 0..(MAX_INPUTS + 1) {
        inputs.push(TxInput {
            prevout_hash: [1u8; 32],
            prevout_index: i as u32,
            script_sig: Vec::new(),
            sequence: FINAL_SEQUENCE,
        });
    }

    let tx = ZcashTxV5 {
        version: VERSION_V5,
        version_group_id: VERSION_GROUP_ID,
        consensus_branch_id: CONSENSUS_BRANCH_ID,
        lock_time: 0,
        expiry_height: 0,
        inputs,
        outputs: vec![TxOutput {
            value: 1,
            script_pubkey,
        }],
        sapling_spends: Vec::new(),
        sapling_outputs: Vec::new(),
        value_balance_sapling: 0,
        sapling_anchor: [0u8; 32],
        orchard_actions: Vec::new(),
        flags_orchard: 0,
        value_balance_orchard: 0,
        orchard_anchor: [0u8; 32],
    };

    let tx_data = TransparentTxData {
        transactions: vec![tx],
        utxo_proofs: vec![Vec::new()],
        utxo_snapshot: None,
    };

    assert_err_contains(
        verify_transparent_txs(&tx_data, &[0u8; 32], 100, 0),
        "too many inputs",
    );
}

#[test]
fn test_too_many_outputs() {
    let script_pubkey = p2pkh_script_pubkey(&[0x02u8; 33]);
    let mut outputs = Vec::with_capacity(MAX_OUTPUTS + 1);
    for _ in 0..(MAX_OUTPUTS + 1) {
        outputs.push(TxOutput {
            value: 1,
            script_pubkey: script_pubkey.clone(),
        });
    }

    let tx = ZcashTxV5 {
        version: VERSION_V5,
        version_group_id: VERSION_GROUP_ID,
        consensus_branch_id: CONSENSUS_BRANCH_ID,
        lock_time: 0,
        expiry_height: 0,
        inputs: vec![TxInput {
            prevout_hash: [1u8; 32],
            prevout_index: 0,
            script_sig: Vec::new(),
            sequence: FINAL_SEQUENCE,
        }],
        outputs,
        sapling_spends: Vec::new(),
        sapling_outputs: Vec::new(),
        value_balance_sapling: 0,
        sapling_anchor: [0u8; 32],
        orchard_actions: Vec::new(),
        flags_orchard: 0,
        value_balance_orchard: 0,
        orchard_anchor: [0u8; 32],
    };

    let tx_data = TransparentTxData {
        transactions: vec![tx],
        utxo_proofs: vec![Vec::new()],
        utxo_snapshot: None,
    };

    assert_err_contains(
        verify_transparent_txs(&tx_data, &[0u8; 32], 100, 0),
        "too many outputs",
    );
}

#[test]
fn test_invalid_scriptsig_rejected() {
    let (mut tx, proof, root) = build_signed_tx(50_000, 40_000);
    tx.inputs[0].script_sig = vec![0x01];

    let tx_data = TransparentTxData {
        transactions: vec![tx],
        utxo_proofs: vec![vec![proof]],
        utxo_snapshot: None,
    };

    assert!(verify_transparent_txs(&tx_data, &root, 100, 0).is_err());
}

#[test]
fn test_coinbase_input_rejected() {
    let script_pubkey = p2pkh_script_pubkey(&[0x02u8; 33]);
    let outpoint = Outpoint {
        txid: [0u8; 32],
        vout: 0xFFFF_FFFF,
    };
    let proof = UtxoMerkleProof {
        outpoint,
        value: 50_000,
        script_pubkey: script_pubkey.clone(),
        height: 0,
        is_coinbase: true,
        tree_position: 0,
        siblings: Vec::new(),
    };

    let tx = ZcashTxV5 {
        version: VERSION_V5,
        version_group_id: VERSION_GROUP_ID,
        consensus_branch_id: CONSENSUS_BRANCH_ID,
        lock_time: 0,
        expiry_height: 0,
        inputs: vec![TxInput {
            prevout_hash: outpoint.txid,
            prevout_index: outpoint.vout,
            script_sig: Vec::new(),
            sequence: FINAL_SEQUENCE,
        }],
        outputs: vec![TxOutput {
            value: 40_000,
            script_pubkey,
        }],
        sapling_spends: Vec::new(),
        sapling_outputs: Vec::new(),
        value_balance_sapling: 0,
        sapling_anchor: [0u8; 32],
        orchard_actions: Vec::new(),
        flags_orchard: 0,
        value_balance_orchard: 0,
        orchard_anchor: [0u8; 32],
    };

    let tx_data = TransparentTxData {
        transactions: vec![tx],
        utxo_proofs: vec![vec![proof]],
        utxo_snapshot: None,
    };

    assert_err_contains(
        verify_transparent_txs(&tx_data, &[0u8; 32], 100, 0),
        "coinbase inputs are not allowed",
    );
}

#[test]
fn test_double_spend_rejected() {
    let (tx, proof, root) = build_double_spend_tx(50_000, 40_000);

    let tx_data = TransparentTxData {
        transactions: vec![tx],
        utxo_proofs: vec![vec![proof.clone(), proof]],
        utxo_snapshot: None,
    };

    assert_err_contains(
        verify_transparent_txs(&tx_data, &root, 100, 0),
        "double-spend detected",
    );
}
