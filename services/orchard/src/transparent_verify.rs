//! Transparent transaction verification for JAM Orchard service
//!
//! Parses tag=4 TransparentTxData extrinsic and verifies:
//! - Zcash v5 transaction validity
//! - UTXO existence and inclusion proofs
//! - Script validation (P2PKH)
//! - Consensus rules (locktime, expiry, value conservation)

use alloc::vec::Vec;
use crate::errors::{OrchardError, Result};
use crate::transparent_parser::{compute_signature_hash_v5, parse_zcash_tx_v5, ZcashTxV5};
use crate::transparent_script;
use crate::transparent_utxo::{UtxoMerkleProof, Outpoint};
use sha2::{Sha256, Digest};

const CONSENSUS_BRANCH_NU5: u32 = 0xC2D6D0B4;
const LOCK_TIME_THRESHOLD: u32 = 500_000_000;
const FINAL_SEQUENCE: u32 = 0xFFFF_FFFF;
const MAX_INPUTS: usize = 1000;
const MAX_OUTPUTS: usize = 1000;
const COINBASE_MATURITY: u32 = 100;

/// Parsed TransparentTxData extrinsic (tag=4)
///
/// Format:
///   tx_count (u32) +
///   for each tx:
///     tx_len (u32) + tx_bytes +
///     input_proof_count (u32) +
///     for each input:
///       proof_len (u32) + utxo_merkle_proof
pub struct TransparentTxData {
    pub transactions: Vec<ZcashTxV5>,
    pub utxo_proofs: Vec<Vec<UtxoMerkleProof>>, // utxo_proofs[tx_idx][input_idx]
    pub utxo_snapshot: Option<Vec<UtxoSnapshotEntry>>,
}

pub struct TransparentStateTransition {
    pub new_merkle_root: [u8; 32],
    pub new_utxo_root: [u8; 32],
    pub new_utxo_size: u64,
}

pub struct UtxoSnapshotEntry {
    pub outpoint: Outpoint,
    pub value: u64,
    pub script_pubkey: Vec<u8>,
    pub height: u32,
    pub is_coinbase: bool,
}

/// Parse tag=4 TransparentTxData extrinsic
pub fn parse_transparent_tx_data(data: &[u8]) -> Result<TransparentTxData> {
    let mut cursor = 0usize;

    let tx_count = read_u32(data, &mut cursor)? as usize;
    let mut transactions = Vec::with_capacity(tx_count.min(100));
    let mut utxo_proofs = Vec::with_capacity(tx_count.min(100));

    for _ in 0..tx_count {
        // Read transaction
        let tx_len = read_u32(data, &mut cursor)? as usize;
        if cursor + tx_len > data.len() {
            return Err(OrchardError::ParseError("tx exceeds data bounds".into()));
        }
        let tx_bytes = &data[cursor..cursor + tx_len];
        cursor += tx_len;

        let tx = parse_zcash_tx_v5(tx_bytes)?;

        // Read UTXO proofs for this transaction's inputs
        let proof_count = read_u32(data, &mut cursor)? as usize;
        if proof_count != tx.inputs.len() {
            return Err(OrchardError::ParseError(alloc::format!(
                "proof count {} != input count {}",
                proof_count, tx.inputs.len()
            )));
        }

        let mut tx_proofs = Vec::with_capacity(proof_count);
        for _ in 0..proof_count {
            let proof_len = read_u32(data, &mut cursor)? as usize;
            if cursor + proof_len > data.len() {
                return Err(OrchardError::ParseError("proof exceeds data bounds".into()));
            }
            let proof_bytes = &data[cursor..cursor + proof_len];
            cursor += proof_len;

            let proof = parse_utxo_merkle_proof(proof_bytes)?;
            tx_proofs.push(proof);
        }

        transactions.push(tx);
        utxo_proofs.push(tx_proofs);
    }

    let mut utxo_snapshot = None;
    if cursor < data.len() {
        let snapshot_count = read_u32(data, &mut cursor)? as usize;
        let mut snapshot = Vec::with_capacity(snapshot_count);
        for _ in 0..snapshot_count {
            snapshot.push(parse_utxo_snapshot_entry(data, &mut cursor)?);
        }
        utxo_snapshot = Some(snapshot);
    }

    if cursor != data.len() {
        return Err(OrchardError::ParseError("trailing bytes in TransparentTxData".into()));
    }

    Ok(TransparentTxData {
        transactions,
        utxo_proofs,
        utxo_snapshot,
    })
}

/// Parse UTXO Merkle proof from bytes
///
/// Format:
///   outpoint: txid (32) + vout (4)
///   utxo_data: value (8) + script_len (u32) + script + height (4)
///   proof: tree_position (u64) + sibling_count (u32) + siblings (32 * count)
fn parse_utxo_merkle_proof(data: &[u8]) -> Result<UtxoMerkleProof> {
    let mut cursor = 0usize;

    // Outpoint
    let mut txid = [0u8; 32];
    if cursor + 32 > data.len() {
        return Err(OrchardError::ParseError("txid out of bounds".into()));
    }
    txid.copy_from_slice(&data[cursor..cursor + 32]);
    cursor += 32;

    let vout = read_u32(data, &mut cursor)?;
    let outpoint = Outpoint { txid, vout };

    // UTXO data
    let value = read_u64(data, &mut cursor)?;
    let script_len = read_u32(data, &mut cursor)? as usize;
    if cursor + script_len > data.len() {
        return Err(OrchardError::ParseError("script out of bounds".into()));
    }
    let script_pubkey = data[cursor..cursor + script_len].to_vec();
    cursor += script_len;

    let height = read_u32(data, &mut cursor)?;

    // Coinbase flag (1 byte)
    if cursor >= data.len() {
        return Err(OrchardError::ParseError("coinbase flag out of bounds".into()));
    }
    let is_coinbase = data[cursor] != 0;
    cursor += 1;

    // Merkle proof position + siblings
    let tree_position = read_u64(data, &mut cursor)?;
    let sibling_count = read_u32(data, &mut cursor)? as usize;
    let mut siblings = Vec::with_capacity(sibling_count);
    for _ in 0..sibling_count {
        let mut sibling = [0u8; 32];
        if cursor + 32 > data.len() {
            return Err(OrchardError::ParseError("sibling out of bounds".into()));
        }
        sibling.copy_from_slice(&data[cursor..cursor + 32]);
        cursor += 32;
        siblings.push(sibling);
    }

    Ok(UtxoMerkleProof {
        outpoint,
        value,
        script_pubkey,
        height,
        is_coinbase,
        tree_position,
        siblings,
    })
}

fn parse_utxo_snapshot_entry(data: &[u8], cursor: &mut usize) -> Result<UtxoSnapshotEntry> {
    let mut txid = [0u8; 32];
    if *cursor + 32 > data.len() {
        return Err(OrchardError::ParseError("snapshot txid out of bounds".into()));
    }
    txid.copy_from_slice(&data[*cursor..*cursor + 32]);
    *cursor += 32;

    let vout = read_u32(data, cursor)?;
    let outpoint = Outpoint { txid, vout };

    let value = read_u64(data, cursor)?;
    let script_len = read_u32(data, cursor)? as usize;
    if *cursor + script_len > data.len() {
        return Err(OrchardError::ParseError("snapshot script out of bounds".into()));
    }
    let script_pubkey = data[*cursor..*cursor + script_len].to_vec();
    *cursor += script_len;

    let height = read_u32(data, cursor)?;
    if *cursor >= data.len() {
        return Err(OrchardError::ParseError("snapshot coinbase flag out of bounds".into()));
    }
    let is_coinbase = data[*cursor] != 0;
    *cursor += 1;

    Ok(UtxoSnapshotEntry {
        outpoint,
        value,
        script_pubkey,
        height,
        is_coinbase,
    })
}

/// Verify transparent transactions against pre-state
///
/// Checks:
/// 1. UTXO existence via Merkle proofs
/// 2. Script validation (P2PKH only for now)
/// 3. Consensus rules (locktime, expiry, value conservation)
/// 4. No double spends within the same block
pub fn verify_transparent_txs(
    txs_data: &TransparentTxData,
    pre_utxo_root: &[u8; 32],
    current_block_height: u32,
    current_block_time: u32,
) -> Result<()> {
    use alloc::collections::BTreeSet;

    let mut spent_outpoints = BTreeSet::new();

    for (tx_idx, tx) in txs_data.transactions.iter().enumerate() {
        if tx.consensus_branch_id != CONSENSUS_BRANCH_NU5 {
            return Err(OrchardError::ParseError(alloc::format!(
                "unsupported consensus branch id: 0x{:08x}",
                tx.consensus_branch_id
            )));
        }
        if tx.inputs.is_empty() {
            return Err(OrchardError::ParseError("transaction has no inputs".into()));
        }
        if tx.outputs.is_empty() {
            return Err(OrchardError::ParseError("transaction has no outputs".into()));
        }
        if tx.inputs.len() > MAX_INPUTS {
            return Err(OrchardError::ParseError(alloc::format!(
                "too many inputs: {} > {}",
                tx.inputs.len(),
                MAX_INPUTS
            )));
        }
        if tx.outputs.len() > MAX_OUTPUTS {
            return Err(OrchardError::ParseError(alloc::format!(
                "too many outputs: {} > {}",
                tx.outputs.len(),
                MAX_OUTPUTS
            )));
        }

        validate_lock_time(tx, current_block_height, current_block_time)?;
        validate_expiry(tx, current_block_height)?;

        let proofs = &txs_data.utxo_proofs[tx_idx];
        let mut input_values = Vec::with_capacity(proofs.len());
        let mut input_scripts = Vec::with_capacity(proofs.len());
        for proof in proofs {
            input_values.push(proof.value);
            input_scripts.push(proof.script_pubkey.clone());
        }
        let mut sum_inputs: u64 = 0;
        let mut script_codes = input_scripts.clone();
        for (script_idx, input) in tx.inputs.iter().enumerate() {
            let proof = &proofs[script_idx];
            if transparent_script::is_p2sh(&proof.script_pubkey) {
                let redeem_script =
                    transparent_script::extract_redeem_script(&input.script_sig)?;
                script_codes[script_idx] = redeem_script;
            }
        }

        // Verify each input
        for (input_idx, input) in tx.inputs.iter().enumerate() {
            let proof = &proofs[input_idx];
            if proof.outpoint.txid != input.prevout_hash || proof.outpoint.vout != input.prevout_index {
                return Err(OrchardError::ParseError(
                    "UTXO proof outpoint does not match input".into()
                ));
            }

            if input.is_coinbase() {
                return Err(OrchardError::ParseError(
                    "coinbase inputs are not allowed in mempool".into(),
                ));
            }

            // Check for double-spend within this set of transactions
            let outpoint = Outpoint {
                txid: input.prevout_hash,
                vout: input.prevout_index,
            };
            if !spent_outpoints.insert(outpoint.clone()) {
                return Err(OrchardError::ParseError(
                    "double-spend detected".into()
                ));
            }

            // Verify UTXO Merkle proof
            if !crate::transparent_utxo::verify_utxo_proof(proof, pre_utxo_root) {
                return Err(OrchardError::ParseError(
                    "UTXO Merkle proof verification failed".into()
                ));
            }

            // Enforce coinbase maturity using proof height.
            if proof.is_coinbase {
                let mature_height = proof.height.saturating_add(COINBASE_MATURITY);
                if current_block_height < mature_height {
                    return Err(OrchardError::ParseError(alloc::format!(
                        "coinbase output immature: height {} < mature {}",
                        current_block_height,
                        mature_height
                    )));
                }
            }

            sum_inputs = add_u64(sum_inputs, proof.value)?;

            if transparent_script::is_p2pkh(&proof.script_pubkey) {
                let sig = transparent_script::parse_p2pkh_script_sig(&input.script_sig)?;
                let sighash = compute_signature_hash_v5(
                    tx,
                    input_idx,
                    sig.hash_type,
                    &input_values,
                    &script_codes,
                )?;
                transparent_script::verify_p2pkh(
                    &sig.pubkey,
                    &sig.signature,
                    &proof.script_pubkey,
                    &sighash,
                )?;
            } else if transparent_script::is_p2sh(&proof.script_pubkey) {
                let hash_type =
                    transparent_script::extract_p2sh_sighash_type(&input.script_sig)?;
                let sighash = compute_signature_hash_v5(
                    tx,
                    input_idx,
                    hash_type,
                    &input_values,
                    &script_codes,
                )?;
                let ctx = transparent_script::ScriptContext {
                    lock_time: tx.lock_time,
                    sequence: input.sequence,
                    tx_version: tx.version,
                };
                transparent_script::verify_p2sh_with_context(
                    &input.script_sig,
                    &proof.script_pubkey,
                    &sighash,
                    Some(&ctx),
                )?;
            } else if transparent_script::is_p2wpkh(&proof.script_pubkey)
                || transparent_script::is_p2wsh(&proof.script_pubkey)
            {
                return Err(OrchardError::ParseError(
                    "witness program unsupported".into(),
                ));
            } else {
                return Err(OrchardError::ParseError(
                    "unsupported script type".into(),
                ));
            }
        }

        let mut sum_outputs: u64 = 0;
        for output in &tx.outputs {
            sum_outputs = add_u64(sum_outputs, output.value)?;
        }
        if sum_outputs > sum_inputs {
            return Err(OrchardError::ParseError(alloc::format!(
                "outputs exceed inputs: {} > {}",
                sum_outputs,
                sum_inputs
            )));
        }
    }

    Ok(())
}

pub fn compute_transparent_transition(
    txs_data: &TransparentTxData,
    pre_utxo_root: &[u8; 32],
    pre_utxo_size: u64,
    current_height: u32,
) -> Result<TransparentStateTransition> {
    use crate::transparent_utxo::{TransparentUtxoTree, UtxoData};

    let txids: Vec<[u8; 32]> = txs_data
        .transactions
        .iter()
        .map(crate::transparent_parser::compute_v5_txid)
        .collect();

    let mut txids_sorted = txids.clone();
    txids_sorted.sort_unstable();
    let new_merkle_root = build_txid_merkle_root(&txids_sorted);

    let mut tree = TransparentUtxoTree::new();
    if let Some(snapshot) = &txs_data.utxo_snapshot {
        for entry in snapshot {
            tree.insert(
                entry.outpoint,
                UtxoData {
                    value: entry.value,
                    script_pubkey: entry.script_pubkey.clone(),
                    height: entry.height,
                    is_coinbase: entry.is_coinbase,
                },
            );
        }
        let snapshot_root = tree.get_root()?;
        if snapshot_root != *pre_utxo_root {
            return Err(OrchardError::StateError(
                "transparent UTXO snapshot root mismatch".into(),
            ));
        }
        if tree.size() != pre_utxo_size {
            return Err(OrchardError::StateError(
                "transparent UTXO snapshot size mismatch".into(),
            ));
        }
    } else if *pre_utxo_root != [0u8; 32] || pre_utxo_size != 0 {
        return Err(OrchardError::StateError(
            "missing transparent UTXO snapshot for non-zero state".into(),
        ));
    }

    for tx_proofs in &txs_data.utxo_proofs {
        for proof in tx_proofs {
            let Some(existing) = tree.get(&proof.outpoint) else {
                return Err(OrchardError::StateError(
                    "UTXO snapshot missing spent outpoint".into(),
                ));
            };
            if existing.value != proof.value
                || existing.script_pubkey != proof.script_pubkey
                || existing.height != proof.height
                || existing.is_coinbase != proof.is_coinbase
            {
                return Err(OrchardError::StateError(
                    "UTXO snapshot does not match proof data".into(),
                ));
            }
            tree.remove(&proof.outpoint)?;
        }
    }

    for (tx_idx, tx) in txs_data.transactions.iter().enumerate() {
        let txid = txids[tx_idx];
        for (vout, output) in tx.outputs.iter().enumerate() {
            tree.insert(
                Outpoint {
                    txid,
                    vout: vout as u32,
                },
                UtxoData {
                    value: output.value,
                    script_pubkey: output.script_pubkey.clone(),
                    height: current_height,
                    is_coinbase: false,
                },
            );
        }
    }

    let new_utxo_root = tree.get_root()?;
    let new_utxo_size = tree.size();

    Ok(TransparentStateTransition {
        new_merkle_root,
        new_utxo_root,
        new_utxo_size,
    })
}

fn build_txid_merkle_root(leaves: &[[u8; 32]]) -> [u8; 32] {
    if leaves.is_empty() {
        return [0u8; 32];
    }
    if leaves.len() == 1 {
        return leaves[0];
    }

    let mut level = leaves.to_vec();
    while level.len() > 1 {
        let mut next_level = Vec::with_capacity((level.len() + 1) / 2);
        for chunk in level.chunks(2) {
            let left = chunk[0];
            let right = if chunk.len() == 2 { chunk[1] } else { chunk[0] };
            let mut data = [0u8; 64];
            data[..32].copy_from_slice(&left);
            data[32..].copy_from_slice(&right);
            next_level.push(sha256d(&data));
        }
        level = next_level;
    }
    level[0]
}


fn sha256d(data: &[u8]) -> [u8; 32] {
    let first = Sha256::digest(data);
    let second = Sha256::digest(&first);
    let mut result = [0u8; 32];
    result.copy_from_slice(&second);
    result
}

// Helper functions

fn read_u32(data: &[u8], cursor: &mut usize) -> Result<u32> {
    if *cursor + 4 > data.len() {
        return Err(OrchardError::ParseError("u32 out of bounds".into()));
    }
    let mut bytes = [0u8; 4];
    bytes.copy_from_slice(&data[*cursor..*cursor + 4]);
    *cursor += 4;
    Ok(u32::from_le_bytes(bytes))
}

fn read_u64(data: &[u8], cursor: &mut usize) -> Result<u64> {
    if *cursor + 8 > data.len() {
        return Err(OrchardError::ParseError("u64 out of bounds".into()));
    }
    let mut bytes = [0u8; 8];
    bytes.copy_from_slice(&data[*cursor..*cursor + 8]);
    *cursor += 8;
    Ok(u64::from_le_bytes(bytes))
}

fn validate_lock_time(tx: &ZcashTxV5, current_height: u32, current_time: u32) -> Result<()> {
    if tx.lock_time == 0 {
        return Ok(());
    }
    if !tx_uses_lock_time(tx) {
        return Ok(());
    }

    if tx.lock_time < LOCK_TIME_THRESHOLD {
        if current_height == 0 {
            return Ok(());
        }
        if current_height < tx.lock_time {
            return Err(OrchardError::ParseError(alloc::format!(
                "locktime not yet reached: {} < {}",
                current_height,
                tx.lock_time
            )));
        }
        return Ok(());
    }

    if current_time == 0 {
        return Ok(());
    }
    if current_time < tx.lock_time {
        return Err(OrchardError::ParseError(alloc::format!(
            "locktime not yet reached: {} < {}",
            current_time,
            tx.lock_time
        )));
    }

    Ok(())
}

fn validate_expiry(tx: &ZcashTxV5, current_height: u32) -> Result<()> {
    if tx.expiry_height == 0 {
        return Ok(());
    }
    if current_height == 0 {
        return Ok(());
    }
    if current_height > tx.expiry_height {
        return Err(OrchardError::ParseError(alloc::format!(
            "transaction expired: height {} > expiry {}",
            current_height,
            tx.expiry_height
        )));
    }
    Ok(())
}

fn tx_uses_lock_time(tx: &ZcashTxV5) -> bool {
    tx.inputs.iter().any(|input| input.sequence != FINAL_SEQUENCE)
}

fn add_u64(a: u64, b: u64) -> Result<u64> {
    a.checked_add(b).ok_or_else(|| {
        OrchardError::ParseError("value overflow".into())
    })
}
