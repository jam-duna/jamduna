//! Zcash v5 (NU5) transparent transaction parser
//!
//! Implements ZIP-244 BLAKE2b TxID computation and signature hash for v5 transactions.
//! This is a no_std implementation suitable for PolkaVM execution.

use alloc::vec::Vec;
use blake2b_simd::Params;
use crate::errors::{OrchardError, Result};

const SAPLING_ENC_CIPHERTEXT_SIZE: usize = 580;
const SAPLING_OUT_CIPHERTEXT_SIZE: usize = 80;
const GROTH_PROOF_SIZE: usize = 192;
const REDJUBJUB_SIG_SIZE: usize = 64;
const REDPALLAS_SIG_SIZE: usize = 64;

/// Create BLAKE2b-256 hasher with personalization string
fn blake2b_personal(personalization: &[u8; 16]) -> blake2b_simd::State {
    Params::new()
        .hash_length(32)
        .personal(personalization)
        .to_state()
}

/// Zcash v5 transaction structure (transparent-only subset)
#[derive(Debug, Clone)]
pub struct ZcashTxV5 {
    pub version: u32,
    pub version_group_id: u32,
    pub consensus_branch_id: u32,
    pub lock_time: u32,
    pub expiry_height: u32,
    pub inputs: Vec<TxInput>,
    pub outputs: Vec<TxOutput>,
    pub sapling_spends: Vec<SaplingSpend>,
    pub sapling_outputs: Vec<SaplingOutput>,
    pub value_balance_sapling: u64,
    pub sapling_anchor: [u8; 32],
    pub orchard_actions: Vec<OrchardAction>,
    pub flags_orchard: u8,
    pub value_balance_orchard: u64,
    pub orchard_anchor: [u8; 32],
}

#[derive(Debug, Clone)]
pub struct TxInput {
    pub prevout_hash: [u8; 32],
    pub prevout_index: u32,
    pub script_sig: Vec<u8>,
    pub sequence: u32,
}

#[derive(Debug, Clone)]
pub struct TxOutput {
    pub value: u64,
    pub script_pubkey: Vec<u8>,
}

#[derive(Debug, Clone)]
pub struct SaplingSpend {
    pub cv: [u8; 32],
    pub nullifier: [u8; 32],
    pub rk: [u8; 32],
}

#[derive(Debug, Clone)]
pub struct SaplingOutput {
    pub cv: [u8; 32],
    pub cmu: [u8; 32],
    pub ephemeral_key: [u8; 32],
    pub enc_ciphertext: Vec<u8>,
    pub out_ciphertext: Vec<u8>,
}

#[derive(Debug, Clone)]
pub struct OrchardAction {
    pub cv: [u8; 32],
    pub nullifier: [u8; 32],
    pub rk: [u8; 32],
    pub cmx: [u8; 32],
    pub ephemeral_key: [u8; 32],
    pub enc_ciphertext: Vec<u8>,
    pub out_ciphertext: Vec<u8>,
}

impl TxInput {
    /// Check if this input is a coinbase input
    pub fn is_coinbase(&self) -> bool {
        self.prevout_hash == [0u8; 32] && self.prevout_index == 0xFFFFFFFF
    }
}

/// Parse a Zcash v5 transaction from raw bytes
pub fn parse_zcash_tx_v5(raw_tx: &[u8]) -> Result<ZcashTxV5> {
    let mut cursor = 0usize;

    // Header (20 bytes)
    let version = read_u32_le(raw_tx, &mut cursor)?;
    if (version & 0x7fff_ffff) != 5 {
        return Err(OrchardError::ParseError(alloc::format!(
            "expected v5 transaction, got version {}",
            version
        )));
    }

    let version_group_id = read_u32_le(raw_tx, &mut cursor)?;
    let consensus_branch_id = read_u32_le(raw_tx, &mut cursor)?;
    let lock_time = read_u32_le(raw_tx, &mut cursor)?;
    let expiry_height = read_u32_le(raw_tx, &mut cursor)?;

    // Transparent inputs
    let vin_count = read_varint(raw_tx, &mut cursor)?;
    let mut inputs = Vec::with_capacity(vin_count.min(1000) as usize);
    for _ in 0..vin_count {
        let prevout_hash = read_array_32(raw_tx, &mut cursor)?;
        let prevout_index = read_u32_le(raw_tx, &mut cursor)?;
        let script_len = read_varint(raw_tx, &mut cursor)?;
        let script_sig = read_bytes(raw_tx, &mut cursor, script_len as usize)?;
        let sequence = read_u32_le(raw_tx, &mut cursor)?;

        inputs.push(TxInput {
            prevout_hash,
            prevout_index,
            script_sig,
            sequence,
        });
    }

    // Transparent outputs
    let vout_count = read_varint(raw_tx, &mut cursor)?;
    let mut outputs = Vec::with_capacity(vout_count.min(1000) as usize);
    for _ in 0..vout_count {
        let value = read_u64_le(raw_tx, &mut cursor)?;
        let script_len = read_varint(raw_tx, &mut cursor)?;
        let script_pubkey = read_bytes(raw_tx, &mut cursor, script_len as usize)?;

        outputs.push(TxOutput {
            value,
            script_pubkey,
        });
    }

    // Sapling
    let sapling_spend_count = read_varint(raw_tx, &mut cursor)?;
    let mut sapling_spends = Vec::with_capacity(sapling_spend_count.min(1000) as usize);
    for _ in 0..sapling_spend_count {
        let cv = read_array_32(raw_tx, &mut cursor)?;
        let nullifier = read_array_32(raw_tx, &mut cursor)?;
        let rk = read_array_32(raw_tx, &mut cursor)?;
        sapling_spends.push(SaplingSpend { cv, nullifier, rk });
    }

    let sapling_output_count = read_varint(raw_tx, &mut cursor)?;
    let mut sapling_outputs = Vec::with_capacity(sapling_output_count.min(1000) as usize);
    for _ in 0..sapling_output_count {
        let cv = read_array_32(raw_tx, &mut cursor)?;
        let cmu = read_array_32(raw_tx, &mut cursor)?;
        let ephemeral_key = read_array_32(raw_tx, &mut cursor)?;
        let enc_ciphertext = read_bytes(raw_tx, &mut cursor, SAPLING_ENC_CIPHERTEXT_SIZE)?;
        let out_ciphertext = read_bytes(raw_tx, &mut cursor, SAPLING_OUT_CIPHERTEXT_SIZE)?;
        sapling_outputs.push(SaplingOutput {
            cv,
            cmu,
            ephemeral_key,
            enc_ciphertext,
            out_ciphertext,
        });
    }

    let has_sapling = !sapling_spends.is_empty() || !sapling_outputs.is_empty();
    let mut value_balance_sapling = 0u64;
    let mut sapling_anchor = [0u8; 32];
    if has_sapling {
        value_balance_sapling = read_u64_le(raw_tx, &mut cursor)?;
    }
    if !sapling_spends.is_empty() {
        sapling_anchor = read_array_32(raw_tx, &mut cursor)?;
        for _ in 0..sapling_spend_count {
            skip_bytes(raw_tx, &mut cursor, GROTH_PROOF_SIZE)?;
        }
        for _ in 0..sapling_spend_count {
            skip_bytes(raw_tx, &mut cursor, REDJUBJUB_SIG_SIZE)?;
        }
    }
    for _ in 0..sapling_output_count {
        skip_bytes(raw_tx, &mut cursor, GROTH_PROOF_SIZE)?;
    }
    if has_sapling {
        skip_bytes(raw_tx, &mut cursor, REDJUBJUB_SIG_SIZE)?;
    }

    // Orchard
    let orchard_action_count = read_varint(raw_tx, &mut cursor)?;
    let mut orchard_actions = Vec::with_capacity(orchard_action_count.min(1000) as usize);
    for _ in 0..orchard_action_count {
        let cv = read_array_32(raw_tx, &mut cursor)?;
        let nullifier = read_array_32(raw_tx, &mut cursor)?;
        let rk = read_array_32(raw_tx, &mut cursor)?;
        let cmx = read_array_32(raw_tx, &mut cursor)?;
        let ephemeral_key = read_array_32(raw_tx, &mut cursor)?;
        let enc_ciphertext = read_bytes(raw_tx, &mut cursor, SAPLING_ENC_CIPHERTEXT_SIZE)?;
        let out_ciphertext = read_bytes(raw_tx, &mut cursor, SAPLING_OUT_CIPHERTEXT_SIZE)?;
        orchard_actions.push(OrchardAction {
            cv,
            nullifier,
            rk,
            cmx,
            ephemeral_key,
            enc_ciphertext,
            out_ciphertext,
        });
    }

    let mut flags_orchard = 0u8;
    let mut value_balance_orchard = 0u64;
    let mut orchard_anchor = [0u8; 32];
    if !orchard_actions.is_empty() {
        flags_orchard = read_u8(raw_tx, &mut cursor)?;
        value_balance_orchard = read_u64_le(raw_tx, &mut cursor)?;
        orchard_anchor = read_array_32(raw_tx, &mut cursor)?;

        let proofs_len = read_varint(raw_tx, &mut cursor)?;
        skip_bytes(raw_tx, &mut cursor, proofs_len as usize)?;

        for _ in 0..orchard_action_count {
            skip_bytes(raw_tx, &mut cursor, REDPALLAS_SIG_SIZE)?;
        }
        skip_bytes(raw_tx, &mut cursor, REDPALLAS_SIG_SIZE)?;
    }

    // Verify we've consumed the entire transaction
    if cursor != raw_tx.len() {
        return Err(OrchardError::ParseError(alloc::format!(
            "excess bytes in transaction: consumed {} of {} bytes",
            cursor, raw_tx.len()
        )));
    }

    Ok(ZcashTxV5 {
        version,
        version_group_id,
        consensus_branch_id,
        lock_time,
        expiry_height,
        inputs,
        outputs,
        sapling_spends,
        sapling_outputs,
        value_balance_sapling,
        sapling_anchor,
        orchard_actions,
        flags_orchard,
        value_balance_orchard,
        orchard_anchor,
    })
}

/// Compute v5 TxID using ZIP-244 BLAKE2b
pub fn compute_v5_txid(tx: &ZcashTxV5) -> [u8; 32] {
    // Personalization: "ZcashTxHash_" || consensus_branch_id (16 bytes)
    let mut personalization = [0u8; 16];
    personalization[..12].copy_from_slice(b"ZcashTxHash_");
    personalization[12..16].copy_from_slice(&tx.consensus_branch_id.to_le_bytes());

    // Component digests
    let header_digest = hash_v5_header(tx);
    let transparent_digest = hash_v5_transparent(tx);
    let sapling_digest = hash_v5_sapling(tx);
    let orchard_digest = hash_v5_orchard(tx);

    // Final TxID = BLAKE2b with personalization
    let mut hasher = blake2b_personal(&personalization);
    hasher.update(&header_digest);
    hasher.update(&transparent_digest);
    hasher.update(&sapling_digest);
    hasher.update(&orchard_digest);

    let mut txid = [0u8; 32];
    txid.copy_from_slice(hasher.finalize().as_bytes());
    txid
}

/// Hash v5 header (ZIP-244)
fn hash_v5_header(tx: &ZcashTxV5) -> [u8; 32] {
    let mut hasher = blake2b_personal(b"ZTxIdHeadersHash");
    hasher.update(&tx.version.to_le_bytes());
    hasher.update(&tx.version_group_id.to_le_bytes());
    hasher.update(&tx.consensus_branch_id.to_le_bytes());
    hasher.update(&tx.lock_time.to_le_bytes());
    hasher.update(&tx.expiry_height.to_le_bytes());

    let mut result = [0u8; 32];
    result.copy_from_slice(hasher.finalize().as_bytes());
    result
}

/// Hash v5 transparent bundle (ZIP-244)
fn hash_v5_transparent(tx: &ZcashTxV5) -> [u8; 32] {
    if tx.inputs.is_empty() && tx.outputs.is_empty() {
        return finalize_empty_hash(b"ZTxIdTranspaHash");
    }

    let prevouts_digest = hash_v5_prevouts(tx);
    let sequence_digest = hash_v5_sequences(tx);
    let outputs_digest = hash_v5_outputs(tx);

    let mut hasher = blake2b_personal(b"ZTxIdTranspaHash");
    hasher.update(&prevouts_digest);
    hasher.update(&sequence_digest);
    hasher.update(&outputs_digest);

    let mut result = [0u8; 32];
    result.copy_from_slice(hasher.finalize().as_bytes());
    result
}

fn hash_v5_sapling(tx: &ZcashTxV5) -> [u8; 32] {
    let mut hasher = blake2b_personal(b"ZTxIdSaplingHash");
    if !tx.sapling_spends.is_empty() || !tx.sapling_outputs.is_empty() {
        let spends_digest = hash_v5_sapling_spends(tx);
        let outputs_digest = hash_v5_sapling_outputs(tx);
        hasher.update(&spends_digest);
        hasher.update(&outputs_digest);
        hasher.update(&tx.value_balance_sapling.to_le_bytes());
    }

    let mut result = [0u8; 32];
    result.copy_from_slice(hasher.finalize().as_bytes());
    result
}

fn hash_v5_sapling_spends(tx: &ZcashTxV5) -> [u8; 32] {
    let mut hasher = blake2b_personal(b"ZTxIdSSpendsHash");
    if !tx.sapling_spends.is_empty() {
        let compact = hash_v5_sapling_spends_compact(tx);
        let noncompact = hash_v5_sapling_spends_noncompact(tx);
        hasher.update(&compact);
        hasher.update(&noncompact);
    }

    let mut result = [0u8; 32];
    result.copy_from_slice(hasher.finalize().as_bytes());
    result
}

fn hash_v5_sapling_spends_compact(tx: &ZcashTxV5) -> [u8; 32] {
    let mut hasher = blake2b_personal(b"ZTxIdSSpendCHash");
    for spend in &tx.sapling_spends {
        hasher.update(&spend.nullifier);
    }
    let mut result = [0u8; 32];
    result.copy_from_slice(hasher.finalize().as_bytes());
    result
}

fn hash_v5_sapling_spends_noncompact(tx: &ZcashTxV5) -> [u8; 32] {
    let mut hasher = blake2b_personal(b"ZTxIdSSpendNHash");
    for spend in &tx.sapling_spends {
        hasher.update(&spend.cv);
        hasher.update(&tx.sapling_anchor);
        hasher.update(&spend.rk);
    }
    let mut result = [0u8; 32];
    result.copy_from_slice(hasher.finalize().as_bytes());
    result
}

fn hash_v5_sapling_outputs(tx: &ZcashTxV5) -> [u8; 32] {
    let mut hasher = blake2b_personal(b"ZTxIdSOutputHash");
    if !tx.sapling_outputs.is_empty() {
        let compact = hash_v5_sapling_outputs_compact(tx);
        let memos = hash_v5_sapling_outputs_memos(tx);
        let noncompact = hash_v5_sapling_outputs_noncompact(tx);
        hasher.update(&compact);
        hasher.update(&memos);
        hasher.update(&noncompact);
    }

    let mut result = [0u8; 32];
    result.copy_from_slice(hasher.finalize().as_bytes());
    result
}

fn hash_v5_sapling_outputs_compact(tx: &ZcashTxV5) -> [u8; 32] {
    let mut hasher = blake2b_personal(b"ZTxIdSOutC__Hash");
    for output in &tx.sapling_outputs {
        hasher.update(&output.cmu);
        hasher.update(&output.ephemeral_key);
        hasher.update(&output.enc_ciphertext[..52]);
    }
    let mut result = [0u8; 32];
    result.copy_from_slice(hasher.finalize().as_bytes());
    result
}

fn hash_v5_sapling_outputs_memos(tx: &ZcashTxV5) -> [u8; 32] {
    let mut hasher = blake2b_personal(b"ZTxIdSOutM__Hash");
    for output in &tx.sapling_outputs {
        hasher.update(&output.enc_ciphertext[52..564]);
    }
    let mut result = [0u8; 32];
    result.copy_from_slice(hasher.finalize().as_bytes());
    result
}

fn hash_v5_sapling_outputs_noncompact(tx: &ZcashTxV5) -> [u8; 32] {
    let mut hasher = blake2b_personal(b"ZTxIdSOutN__Hash");
    for output in &tx.sapling_outputs {
        hasher.update(&output.cv);
        hasher.update(&output.enc_ciphertext[564..]);
        hasher.update(&output.out_ciphertext);
    }
    let mut result = [0u8; 32];
    result.copy_from_slice(hasher.finalize().as_bytes());
    result
}

fn hash_v5_orchard(tx: &ZcashTxV5) -> [u8; 32] {
    let mut hasher = blake2b_personal(b"ZTxIdOrchardHash");
    if !tx.orchard_actions.is_empty() {
        let compact = hash_v5_orchard_actions_compact(tx);
        let memos = hash_v5_orchard_actions_memos(tx);
        let noncompact = hash_v5_orchard_actions_noncompact(tx);
        hasher.update(&compact);
        hasher.update(&memos);
        hasher.update(&noncompact);
        hasher.update(&[tx.flags_orchard]);
        hasher.update(&tx.value_balance_orchard.to_le_bytes());
        hasher.update(&tx.orchard_anchor);
    }

    let mut result = [0u8; 32];
    result.copy_from_slice(hasher.finalize().as_bytes());
    result
}

fn hash_v5_orchard_actions_compact(tx: &ZcashTxV5) -> [u8; 32] {
    let mut hasher = blake2b_personal(b"ZTxIdOrcActCHash");
    for action in &tx.orchard_actions {
        hasher.update(&action.nullifier);
        hasher.update(&action.cmx);
        hasher.update(&action.ephemeral_key);
        hasher.update(&action.enc_ciphertext[..52]);
    }
    let mut result = [0u8; 32];
    result.copy_from_slice(hasher.finalize().as_bytes());
    result
}

fn hash_v5_orchard_actions_memos(tx: &ZcashTxV5) -> [u8; 32] {
    let mut hasher = blake2b_personal(b"ZTxIdOrcActMHash");
    for action in &tx.orchard_actions {
        hasher.update(&action.enc_ciphertext[52..564]);
    }
    let mut result = [0u8; 32];
    result.copy_from_slice(hasher.finalize().as_bytes());
    result
}

fn hash_v5_orchard_actions_noncompact(tx: &ZcashTxV5) -> [u8; 32] {
    let mut hasher = blake2b_personal(b"ZTxIdOrcActNHash");
    for action in &tx.orchard_actions {
        hasher.update(&action.cv);
        hasher.update(&action.rk);
        hasher.update(&action.enc_ciphertext[564..]);
        hasher.update(&action.out_ciphertext);
    }
    let mut result = [0u8; 32];
    result.copy_from_slice(hasher.finalize().as_bytes());
    result
}

/// Hash prevouts (ZIP-244)
fn hash_v5_prevouts(tx: &ZcashTxV5) -> [u8; 32] {
    let mut hasher = blake2b_personal(b"ZTxIdPrevoutHash");
    for input in &tx.inputs {
        hasher.update(&input.prevout_hash);
        hasher.update(&input.prevout_index.to_le_bytes());
    }

    let mut result = [0u8; 32];
    result.copy_from_slice(hasher.finalize().as_bytes());
    result
}

/// Hash sequences (ZIP-244)
fn hash_v5_sequences(tx: &ZcashTxV5) -> [u8; 32] {
    let mut hasher = blake2b_personal(b"ZTxIdSequencHash");
    for input in &tx.inputs {
        hasher.update(&input.sequence.to_le_bytes());
    }

    let mut result = [0u8; 32];
    result.copy_from_slice(hasher.finalize().as_bytes());
    result
}

/// Hash outputs (ZIP-244)
fn hash_v5_outputs(tx: &ZcashTxV5) -> [u8; 32] {
    let mut hasher = blake2b_personal(b"ZTxIdOutputsHash");
    for output in &tx.outputs {
        hasher.update(&output.value.to_le_bytes());

        // Serialize script length as varint
        let script_len = output.script_pubkey.len();
        if script_len < 0xFD {
            hasher.update(&[script_len as u8]);
        } else if script_len <= 0xFFFF {
            hasher.update(&[0xFD]);
            hasher.update(&(script_len as u16).to_le_bytes());
        } else {
            hasher.update(&[0xFE]);
            hasher.update(&(script_len as u32).to_le_bytes());
        }

        hasher.update(&output.script_pubkey);
    }

    let mut result = [0u8; 32];
    result.copy_from_slice(hasher.finalize().as_bytes());
    result
}

const SIGHASH_ALL: u8 = 0x01;
const SIGHASH_NONE: u8 = 0x02;
const SIGHASH_SINGLE: u8 = 0x03;
const SIGHASH_ANYONECANPAY: u8 = 0x80;

/// Compute ZIP-244 signature hash for a specific input.
pub fn compute_signature_hash_v5(
    tx: &ZcashTxV5,
    input_idx: usize,
    hash_type: u8,
    input_values: &[u64],
    script_pubkeys: &[Vec<u8>],
) -> Result<[u8; 32]> {
    if input_idx >= tx.inputs.len() {
        return Err(OrchardError::ParseError("input index out of bounds".into()));
    }
    if input_values.len() != tx.inputs.len() || script_pubkeys.len() != tx.inputs.len() {
        return Err(OrchardError::ParseError(
            "input metadata length mismatch".into(),
        ));
    }

    let base_hash_type = hash_type & 0x1f;
    if base_hash_type != SIGHASH_ALL
        && base_hash_type != SIGHASH_NONE
        && base_hash_type != SIGHASH_SINGLE
    {
        return Err(OrchardError::ParseError(
            "unsupported sighash type".into(),
        ));
    }
    if base_hash_type == SIGHASH_SINGLE && input_idx >= tx.outputs.len() {
        return Err(OrchardError::ParseError(
            "sighash single input index out of bounds".into(),
        ));
    }

    // Personalization: "ZcashTxHash_" || consensus_branch_id
    let mut personalization = [0u8; 16];
    personalization[..12].copy_from_slice(b"ZcashTxHash_");
    personalization[12..16].copy_from_slice(&tx.consensus_branch_id.to_le_bytes());

    let header_digest = hash_v5_header(tx);
    let transparent_sig_digest =
        hash_v5_transparent_sig(tx, hash_type, input_idx, input_values, script_pubkeys)?;
    let sapling_digest = hash_v5_sapling(tx);
    let orchard_digest = hash_v5_orchard(tx);

    let mut hasher = blake2b_personal(&personalization);
    hasher.update(&header_digest);
    hasher.update(&transparent_sig_digest);
    hasher.update(&sapling_digest);
    hasher.update(&orchard_digest);

    let mut result = [0u8; 32];
    result.copy_from_slice(hasher.finalize().as_bytes());
    Ok(result)
}

fn hash_v5_transparent_sig(
    tx: &ZcashTxV5,
    hash_type: u8,
    input_idx: usize,
    input_values: &[u64],
    script_pubkeys: &[Vec<u8>],
) -> Result<[u8; 32]> {
    if tx.inputs.is_empty() || is_coinbase_tx(tx) {
        return Ok(hash_v5_transparent(tx));
    }

    let mut hasher = blake2b_personal(b"ZTxIdTranspaHash");
    hasher.update(&[hash_type]);
    hasher.update(&hash_v5_prevouts_sig(tx, hash_type));
    hasher.update(&hash_v5_amounts_sig(input_values, hash_type));
    hasher.update(&hash_v5_scriptpubkeys_sig(script_pubkeys, hash_type));
    hasher.update(&hash_v5_sequences_sig(tx, hash_type));
    hasher.update(&hash_v5_outputs_sig(tx, hash_type, input_idx));
    hasher.update(&hash_v5_txin_sig(
        &tx.inputs[input_idx],
        input_values[input_idx],
        &script_pubkeys[input_idx],
    ));

    let mut result = [0u8; 32];
    result.copy_from_slice(hasher.finalize().as_bytes());
    Ok(result)
}

fn hash_v5_prevouts_sig(tx: &ZcashTxV5, hash_type: u8) -> [u8; 32] {
    if hash_type & SIGHASH_ANYONECANPAY != 0 {
        return finalize_empty_hash(b"ZTxIdPrevoutHash");
    }
    hash_v5_prevouts(tx)
}

fn hash_v5_amounts_sig(values: &[u64], hash_type: u8) -> [u8; 32] {
    if hash_type & SIGHASH_ANYONECANPAY != 0 {
        return finalize_empty_hash(b"ZTxTrAmountsHash");
    }
    let mut hasher = blake2b_personal(b"ZTxTrAmountsHash");
    for value in values {
        hasher.update(&value.to_le_bytes());
    }
    let mut result = [0u8; 32];
    result.copy_from_slice(hasher.finalize().as_bytes());
    result
}

fn hash_v5_scriptpubkeys_sig(script_pubkeys: &[Vec<u8>], hash_type: u8) -> [u8; 32] {
    if hash_type & SIGHASH_ANYONECANPAY != 0 {
        return finalize_empty_hash(b"ZTxTrScriptsHash");
    }
    let mut hasher = blake2b_personal(b"ZTxTrScriptsHash");
    for script in script_pubkeys {
        update_compact_size(&mut hasher, script.len());
        hasher.update(script);
    }
    let mut result = [0u8; 32];
    result.copy_from_slice(hasher.finalize().as_bytes());
    result
}

fn hash_v5_sequences_sig(tx: &ZcashTxV5, hash_type: u8) -> [u8; 32] {
    if hash_type & SIGHASH_ANYONECANPAY != 0 {
        return finalize_empty_hash(b"ZTxIdSequencHash");
    }
    hash_v5_sequences(tx)
}

fn hash_v5_outputs_sig(tx: &ZcashTxV5, hash_type: u8, input_idx: usize) -> [u8; 32] {
    let base_hash_type = hash_type & 0x1f;
    if base_hash_type != SIGHASH_SINGLE && base_hash_type != SIGHASH_NONE {
        return hash_v5_outputs(tx);
    }
    if base_hash_type == SIGHASH_SINGLE && input_idx < tx.outputs.len() {
        return hash_v5_output(&tx.outputs[input_idx]);
    }
    finalize_empty_hash(b"ZTxIdOutputsHash")
}

fn hash_v5_output(output: &TxOutput) -> [u8; 32] {
    let mut hasher = blake2b_personal(b"ZTxIdOutputsHash");
    hasher.update(&output.value.to_le_bytes());
    update_compact_size(&mut hasher, output.script_pubkey.len());
    hasher.update(&output.script_pubkey);

    let mut result = [0u8; 32];
    result.copy_from_slice(hasher.finalize().as_bytes());
    result
}

fn hash_v5_txin_sig(input: &TxInput, value: u64, script_pubkey: &[u8]) -> [u8; 32] {
    let mut hasher = blake2b_personal(b"Zcash___TxInHash");
    hasher.update(&input.prevout_hash);
    hasher.update(&input.prevout_index.to_le_bytes());
    hasher.update(&value.to_le_bytes());

    // Serialize script length as varint
    update_compact_size(&mut hasher, script_pubkey.len());
    hasher.update(script_pubkey);
    hasher.update(&input.sequence.to_le_bytes());

    let mut result = [0u8; 32];
    result.copy_from_slice(hasher.finalize().as_bytes());
    result
}

fn finalize_empty_hash(personalization: &[u8; 16]) -> [u8; 32] {
    let hasher = blake2b_personal(personalization);
    let mut result = [0u8; 32];
    result.copy_from_slice(hasher.finalize().as_bytes());
    result
}

fn update_compact_size(hasher: &mut blake2b_simd::State, value: usize) {
    if value < 0xFD {
        hasher.update(&[value as u8]);
    } else if value <= 0xFFFF {
        let mut buf = [0u8; 3];
        buf[0] = 0xFD;
        buf[1..3].copy_from_slice(&(value as u16).to_le_bytes());
        hasher.update(&buf);
    } else {
        let mut buf = [0u8; 5];
        buf[0] = 0xFE;
        buf[1..5].copy_from_slice(&(value as u32).to_le_bytes());
        hasher.update(&buf);
    }
}

fn is_coinbase_tx(tx: &ZcashTxV5) -> bool {
    tx.inputs.len() == 1 && tx.inputs[0].is_coinbase()
}

// Parsing helpers

fn read_u32_le(data: &[u8], cursor: &mut usize) -> Result<u32> {
    let bytes = read_array_4(data, cursor)?;
    Ok(u32::from_le_bytes(bytes))
}

fn read_u64_le(data: &[u8], cursor: &mut usize) -> Result<u64> {
    let bytes = read_array_8(data, cursor)?;
    Ok(u64::from_le_bytes(bytes))
}

fn read_u8(data: &[u8], cursor: &mut usize) -> Result<u8> {
    if *cursor >= data.len() {
        return Err(OrchardError::ParseError("unexpected EOF".into()));
    }
    let value = data[*cursor];
    *cursor += 1;
    Ok(value)
}

fn read_array_4(data: &[u8], cursor: &mut usize) -> Result<[u8; 4]> {
    if *cursor + 4 > data.len() {
        return Err(OrchardError::ParseError("unexpected EOF".into()));
    }
    let mut result = [0u8; 4];
    result.copy_from_slice(&data[*cursor..*cursor + 4]);
    *cursor += 4;
    Ok(result)
}

fn read_array_8(data: &[u8], cursor: &mut usize) -> Result<[u8; 8]> {
    if *cursor + 8 > data.len() {
        return Err(OrchardError::ParseError("unexpected EOF".into()));
    }
    let mut result = [0u8; 8];
    result.copy_from_slice(&data[*cursor..*cursor + 8]);
    *cursor += 8;
    Ok(result)
}

fn read_array_32(data: &[u8], cursor: &mut usize) -> Result<[u8; 32]> {
    if *cursor + 32 > data.len() {
        return Err(OrchardError::ParseError("unexpected EOF".into()));
    }
    let mut result = [0u8; 32];
    result.copy_from_slice(&data[*cursor..*cursor + 32]);
    *cursor += 32;
    Ok(result)
}

fn read_varint(data: &[u8], cursor: &mut usize) -> Result<u64> {
    if *cursor >= data.len() {
        return Err(OrchardError::ParseError("unexpected EOF".into()));
    }
    let first = data[*cursor];
    *cursor += 1;

    match first {
        0..=0xFC => Ok(first as u64),
        0xFD => {
            // Read 2 bytes for u16
            if *cursor + 2 > data.len() {
                return Err(OrchardError::ParseError("unexpected EOF".into()));
            }
            let mut bytes = [0u8; 2];
            bytes.copy_from_slice(&data[*cursor..*cursor + 2]);
            *cursor += 2;
            Ok(u16::from_le_bytes(bytes) as u64)
        }
        0xFE => {
            let bytes = read_array_4(data, cursor)?;
            Ok(u32::from_le_bytes(bytes) as u64)
        }
        0xFF => {
            let bytes = read_array_8(data, cursor)?;
            Ok(u64::from_le_bytes(bytes))
        }
    }
}

fn read_bytes(data: &[u8], cursor: &mut usize, len: usize) -> Result<Vec<u8>> {
    if *cursor + len > data.len() {
        return Err(OrchardError::ParseError("unexpected EOF".into()));
    }
    let result = data[*cursor..*cursor + len].to_vec();
    *cursor += len;
    Ok(result)
}

fn skip_bytes(data: &[u8], cursor: &mut usize, len: usize) -> Result<()> {
    if *cursor + len > data.len() {
        return Err(OrchardError::ParseError("unexpected EOF".into()));
    }
    *cursor += len;
    Ok(())
}
