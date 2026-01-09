//! ZIP-244 transparent transaction test vectors (TxID + SIGHASH).

use orchard_service::transparent_parser::{
    compute_signature_hash_v5, compute_v5_txid, parse_zcash_tx_v5,
};

include!("../../../zcash-test-vectors/test-vectors/rust/zip_0244.rs");

const SIGHASH_ALL: u8 = 0x01;
const SIGHASH_NONE: u8 = 0x02;
const SIGHASH_SINGLE: u8 = 0x03;
const SIGHASH_ANYONECANPAY: u8 = 0x80;

#[test]
fn test_zip244_txid_vectors() {
    let vectors = test_vectors();
    for (index, vector) in vectors.iter().enumerate() {
        let tx = parse_zcash_tx_v5(&vector.tx)
            .unwrap_or_else(|err| panic!("vector {} parse failed: {}", index, err));
        let computed = compute_v5_txid(&tx);
        assert_eq!(computed, vector.txid, "txid mismatch at vector {}", index);
    }
}

#[test]
fn test_zip244_sighash_vectors() {
    let vectors = test_vectors();
    for (index, vector) in vectors.iter().enumerate() {
        let Some(input_index) = vector.transparent_input else {
            continue;
        };
        let input_index = input_index as usize;
        let tx = parse_zcash_tx_v5(&vector.tx)
            .unwrap_or_else(|err| panic!("vector {} parse failed: {}", index, err));

        let input_values: Vec<u64> = vector
            .amounts
            .iter()
            .map(|value| {
                u64::try_from(*value).unwrap_or_else(|_| {
                    panic!("vector {} has negative input amount", index)
                })
            })
            .collect();
        let script_pubkeys = vector.script_pubkeys.clone();

        if let Some(expected) = vector.sighash_all {
            let computed = compute_signature_hash_v5(
                &tx,
                input_index,
                SIGHASH_ALL,
                &input_values,
                &script_pubkeys,
            )
            .unwrap_or_else(|err| panic!("vector {} sighash all failed: {}", index, err));
            assert_eq!(computed, expected, "sighash all mismatch at vector {}", index);
        }

        if let Some(expected) = vector.sighash_none {
            let computed = compute_signature_hash_v5(
                &tx,
                input_index,
                SIGHASH_NONE,
                &input_values,
                &script_pubkeys,
            )
            .unwrap_or_else(|err| panic!("vector {} sighash none failed: {}", index, err));
            assert_eq!(computed, expected, "sighash none mismatch at vector {}", index);
        }

        if let Some(expected) = vector.sighash_single {
            let computed = compute_signature_hash_v5(
                &tx,
                input_index,
                SIGHASH_SINGLE,
                &input_values,
                &script_pubkeys,
            )
            .unwrap_or_else(|err| panic!("vector {} sighash single failed: {}", index, err));
            assert_eq!(computed, expected, "sighash single mismatch at vector {}", index);
        }

        if let Some(expected) = vector.sighash_all_anyone {
            let computed = compute_signature_hash_v5(
                &tx,
                input_index,
                SIGHASH_ALL | SIGHASH_ANYONECANPAY,
                &input_values,
                &script_pubkeys,
            )
            .unwrap_or_else(|err| panic!("vector {} sighash all anyone failed: {}", index, err));
            assert_eq!(
                computed, expected,
                "sighash all anyone mismatch at vector {}",
                index
            );
        }

        if let Some(expected) = vector.sighash_none_anyone {
            let computed = compute_signature_hash_v5(
                &tx,
                input_index,
                SIGHASH_NONE | SIGHASH_ANYONECANPAY,
                &input_values,
                &script_pubkeys,
            )
            .unwrap_or_else(|err| panic!("vector {} sighash none anyone failed: {}", index, err));
            assert_eq!(
                computed, expected,
                "sighash none anyone mismatch at vector {}",
                index
            );
        }

        if let Some(expected) = vector.sighash_single_anyone {
            let computed = compute_signature_hash_v5(
                &tx,
                input_index,
                SIGHASH_SINGLE | SIGHASH_ANYONECANPAY,
                &input_values,
                &script_pubkeys,
            )
            .unwrap_or_else(|err| panic!("vector {} sighash single anyone failed: {}", index, err));
            assert_eq!(
                computed, expected,
                "sighash single anyone mismatch at vector {}",
                index
            );
        }
    }
}
