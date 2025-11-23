//! Receipt and Log serialization for JAM DA export
//!
//! This module handles serialization and deserialization of:
//! - EVM logs (events emitted by contracts)
//! - Transaction receipts (execution results with logs and transaction data)
//!
//! Receipt format includes the FULL transaction payload, eliminating the need
//! for separate transaction objects in DA storage.

use alloc::vec::Vec;
use evm::interpreter::runtime::Log;
use primitive_types::{H160, H256, U256};
use utils::hash_functions::keccak256;
use rlp::RlpStream;

/// Metadata collected for each executed transaction so receipts can be exported.
#[derive(Clone)]
pub struct TransactionReceiptRecord {
    /// Zero-based index of the transaction within the work package ordering.
    pub tx_index: usize,
    /// Keccak-256 hash of the original transaction payload.
    pub hash: [u8; 32],
    /// True when the transaction executed successfully without reverting.
    pub success: bool,
    /// Total gas consumed by the transaction according to the EVM interpreter.
    pub used_gas: U256,
    /// Logs emitted during execution, captured when the transaction commits.
    pub logs: Vec<Log>,
    /// Original transaction payload (extrinsic data).
    pub payload: Vec<u8>,
}

impl TransactionReceiptRecord {
    /// Create a TransactionReceiptRecord from an ObjectCandidateWrite
    /// This deserializes the receipt payload from the candidate write
    ///
    /// Receipt payload format (from serialize_receipt):
    /// [logs_len:4][logs_payload:bytes][version:1][hash:32][type:1][success:1][gas:8][tx_len:4][tx:bytes]
    ///
    /// We extract the logs_payload portion to deserialize logs for block metadata (logs_bloom, receipt_hashes)
    pub fn from_candidate(candidate: &crate::writes::ObjectCandidateWrite) -> Option<Self> {
        if candidate.object_ref.object_kind != crate::sharding::ObjectKind::Receipt as u8 {
            return None;
        }

        // Receipt payload starts with [logs_len:4] followed by logs_payload
        // We need to skip the logs_len prefix to get to the actual logs data
        if candidate.payload.len() < 4 {
            return None; // Malformed receipt - too short
        }

        let logs_len = u32::from_le_bytes([
            candidate.payload[0],
            candidate.payload[1],
            candidate.payload[2],
            candidate.payload[3],
        ]) as usize;

        // Verify we have enough data for the logs payload
        if candidate.payload.len() < 4 + logs_len {
            return None; // Malformed receipt - logs truncated
        }

        // Extract logs from the logs_payload portion (skip 4-byte logs_len prefix)
        let logs = deserialize_logs(&candidate.payload[4..4 + logs_len]).ok()?;

        Some(TransactionReceiptRecord {
            tx_index: 0, // tx_slot field removed from ObjectRef
            hash: candidate.object_id, // Transaction hash from object_id
            success: true, // log_index field removed from ObjectRef - using default
            used_gas: 0.into(), // gas_used field removed from ObjectRef
            logs,
            payload: candidate.payload.clone(),
        })
    }
}

/// Returns the ObjectID for a receipt (transaction hash)
pub fn receipt_object_id_from_receipt(record: &TransactionReceiptRecord) -> [u8; 32] {
    // Receipt ObjectID = transaction hash for direct RPC lookup
    record.hash
}

/// Serialize logs to DA format for receipt objects
/// Format: [log_count:u16][Log...][Log...]
/// Each Log: [address:20B][topic_count:u8][topics:32B*N][data_len:u32][data:bytes]
fn serialize_logs(logs: &[Log]) -> Vec<u8> {
    let mut result = Vec::new();

    // Log count (2 bytes)
    let count = logs.len() as u16;
    result.extend_from_slice(&count.to_le_bytes());

    // Serialize each log
    for log in logs {
        // Address (20 bytes)
        result.extend_from_slice(log.address.as_bytes());

        // Topic count (1 byte)
        let topic_count = log.topics.len() as u8;
        result.push(topic_count);

        // Topics (32 bytes each)
        for topic in &log.topics {
            result.extend_from_slice(topic.as_bytes());
        }

        // Data length (4 bytes)
        let data_len = log.data.len() as u32;
        result.extend_from_slice(&data_len.to_le_bytes());

        // Data (variable length)
        result.extend_from_slice(&log.data);
    }

    result
}

/// Deserialize logs from binary format (inverse of serialize_logs)
///
/// Format: [log_count:2][logs...]
/// Each log: [address:20][topic_count:1][topics:32*N][data_len:4][data]
fn deserialize_logs(data: &[u8]) -> Result<Vec<Log>, &'static str> {
    if data.len() < 2 {
        return Err("logs payload too short for count");
    }

    let mut offset = 0;

    // Parse log count (2 bytes)
    let count = u16::from_le_bytes([data[offset], data[offset + 1]]) as usize;
    offset += 2;

    let mut logs = Vec::with_capacity(count);

    for _ in 0..count {
        // Parse address (20 bytes)
        if offset + 20 > data.len() {
            return Err("incomplete log address");
        }
        let mut address_bytes = [0u8; 20];
        address_bytes.copy_from_slice(&data[offset..offset + 20]);
        let address = H160::from(address_bytes);
        offset += 20;

        // Parse topic count (1 byte)
        if offset >= data.len() {
            return Err("incomplete log topic count");
        }
        let topic_count = data[offset] as usize;
        offset += 1;

        // Parse topics (32 bytes each)
        let mut topics = Vec::with_capacity(topic_count);
        for _ in 0..topic_count {
            if offset + 32 > data.len() {
                return Err("incomplete log topic");
            }
            let mut topic_bytes = [0u8; 32];
            topic_bytes.copy_from_slice(&data[offset..offset + 32]);
            topics.push(H256::from(topic_bytes));
            offset += 32;
        }

        // Parse data length (4 bytes)
        if offset + 4 > data.len() {
            return Err("incomplete log data length");
        }
        let data_len = u32::from_le_bytes([
            data[offset],
            data[offset + 1],
            data[offset + 2],
            data[offset + 3],
        ]) as usize;
        offset += 4;

        // Parse data
        if offset + data_len > data.len() {
            return Err("incomplete log data");
        }
        let log_data = data[offset..offset + data_len].to_vec();
        offset += data_len;

        logs.push(Log {
            address,
            topics,
            data: log_data,
        });
    }

    Ok(logs)
}

/// Detect transaction type from RLP payload (EIP-2718)
///
/// Returns:
/// - 0: Legacy transaction (RLP list)
/// - 1: EIP-2930 (AccessList) - payload starts with 0x01
/// - 2: EIP-1559 (Dynamic fee) - payload starts with 0x02
fn detect_tx_type(payload: &[u8]) -> u8 {
    if payload.is_empty() {
        return 0; // Default to legacy
    }

    match payload[0] {
        0x01 => 1, // EIP-2930
        0x02 => 2, // EIP-1559
        _ => 0,    // Legacy (RLP list starts with 0xf8+ or other RLP markers)
    }
}

/// Serialize a transaction receipt and its logs for DA storage
///
/// **IMPORTANT**: The receipt contains the FULL transaction data (raw RLP payload),
/// eliminating the need for separate transaction objects. RPC can reconstruct the
/// complete transaction from the receipt alone.
///
/// Unified receipt format (version 0 = version 1 structure with placeholders):
/// [version:1][tx_hash:32][tx_type:1][status:1][used_gas:8][tx_payload_len:4][tx_payload]
/// [logs_payload_len:4][logs_payload][tx_index:2][cumulative_gas:8][logs_bloom:256][receipt_hash:32]
///
/// Phase 1B emits with version=0 and placeholder values (0xFFFF for tx_index, zeros for bloom/hash)
/// Phase 2B rewrites with version=1 and canonical values computed from block context
///
/// tx_type values (EIP-2718):
/// - 0: Legacy transaction
/// - 1: EIP-2930 (AccessList)
/// - 2: EIP-1559 (Dynamic fee)
pub fn serialize_receipt(record: &TransactionReceiptRecord) -> Vec<u8> {
    let mut result = Vec::new();
    // Logs payload length (4 bytes) + logs
    let logs_payload = serialize_logs(&record.logs);
    result.extend_from_slice(&(logs_payload.len() as u32).to_le_bytes());
    result.extend_from_slice(&logs_payload);

    result.push(0);  // version byte = 0 (Phase 1B preliminary)
    result.extend_from_slice(&record.hash);

    // Detect tx_type from transaction payload (EIP-2718)
    let tx_type = detect_tx_type(&record.payload);
    result.push(tx_type);
    result.push(if record.success { 1 } else { 0 });

    // Gas as u64 (8 bytes) - sufficient for all practical gas values
    let gas_u64 = record.used_gas.low_u64();
    result.extend_from_slice(&gas_u64.to_le_bytes());

    // Transaction payload (raw RLP) - this is the FULL transaction data
    // RPC uses this to reconstruct the transaction without a separate tx object
    result.extend_from_slice(&(record.payload.len() as u32).to_le_bytes());
    result.extend_from_slice(&record.payload);

    // Compute and append logs bloom (256 bytes)
    let logs_bloom = compute_per_receipt_logs_bloom(&record.logs);
    result.extend_from_slice(&logs_bloom);

    // Append cumulative_gas placeholder (8 bytes) - will be updated in accumulator
    result.extend_from_slice(&0u64.to_le_bytes());

    // Append log_index_start placeholder (8 bytes) - will be updated in accumulator
    result.extend_from_slice(&0u64.to_le_bytes());

    result
}

/// Update receipt payload with cumulative gas and log index (Phase 2 update)
///
/// The receipt must have been serialized with serialize_receipt first.
/// This function updates the cumulative_gas and log_index_start fields at the end of the receipt.
///
/// Receipt layout: [logs_len:4][logs][version:1][hash:32][type:1][status:1]
///                 [used_gas:8][tx_len:4][tx][bloom:256][cumulative_gas:8][log_index_start:8]
pub fn update_receipt_phase2(receipt_payload: &mut [u8], cumulative_gas: u64, log_index_start: u64) {
    if receipt_payload.len() < 16 {
        return; // Invalid receipt
    }

    // Cumulative gas is at offset -16 (second to last 8 bytes)
    let gas_offset = receipt_payload.len() - 16;
    receipt_payload[gas_offset..gas_offset+8].copy_from_slice(&cumulative_gas.to_le_bytes());

    // Log index start is the last 8 bytes
    let log_offset = receipt_payload.len() - 8;
    receipt_payload[log_offset..log_offset+8].copy_from_slice(&log_index_start.to_le_bytes());
}


/// Compute per-receipt logs bloom filter from parsed logs using Ethereum bloom algorithm
///
/// Ethereum bloom filter includes:
/// - Log contract address
/// - Each topic in the log
pub fn compute_per_receipt_logs_bloom(logs: &[Log]) -> [u8; 256] {
    if logs.is_empty() {
        return [0u8; 256];
    }

    // Ethereum uses 2048-bit bloom filter (256 bytes)
    let mut bloom = [0u8; 256];
    let bloom_bits = 2048;

    for log in logs {
        // Add contract address to bloom
        add_to_bloom(&mut bloom, bloom_bits, log.address.as_bytes());

        // Add each topic to bloom
        for topic in &log.topics {
            add_to_bloom(&mut bloom, bloom_bits, topic.as_bytes());
        }
    }

    bloom
}

/// Add item to bloom filter using Ethereum 3-hash algorithm
fn add_to_bloom(bits: &mut [u8], bloom_bits: usize, item: &[u8]) {
    let hash = keccak256(item);

    // Extract three 16-bit indices from bytes [0..1], [2..3], [4..5]
    let idx1 = u16::from_be_bytes([hash.0[0], hash.0[1]]) as usize;
    let idx2 = u16::from_be_bytes([hash.0[2], hash.0[3]]) as usize;
    let idx3 = u16::from_be_bytes([hash.0[4], hash.0[5]]) as usize;

    // Set bits using modulo matching the actual bloom size
    set_bit(bits, idx1 % bloom_bits);
    set_bit(bits, idx2 % bloom_bits);
    set_bit(bits, idx3 % bloom_bits);
}

/// Set a specific bit in the bloom filter (big-endian bit order)
fn set_bit(bits: &mut [u8], index: usize) {
    let byte_index = index / 8;
    let bit_index = 7 - (index % 8); // big-endian bit order within byte
    if byte_index < bits.len() {
        bits[byte_index] |= 1 << bit_index;
    }
}

/// Detects transaction type from raw payload (EIP-2718 typed envelopes)
pub fn detect_tx_type_from_payload(payload: &[u8]) -> u8 {
    if payload.is_empty() {
        return 0;
    }

    match payload[0] {
        0x01 => 1, // EIP-2930
        0x02 => 2, // EIP-1559
        _ => 0,    // Legacy transaction
    }
}

/// Encode canonical Ethereum receipt RLP (with optional typed tx prefix)
pub fn encode_canonical_receipt_rlp(
    record: &TransactionReceiptRecord,
    cumulative_gas: u64,
    logs_bloom: &[u8; 256],
) -> Vec<u8> {
    let status_byte: u8 = if record.success { 1 } else { 0 };

    let mut stream = RlpStream::new_list(4);
    stream.append(&status_byte);
    stream.append(&cumulative_gas);
    let bloom_slice: &[u8] = &logs_bloom[..];
    stream.append(&bloom_slice);

    stream.begin_list(record.logs.len());
    for log in &record.logs {
        stream.begin_list(3);
        let address_slice: &[u8] = log.address.as_bytes();
        stream.append(&address_slice);

        stream.begin_list(log.topics.len());
        for topic in &log.topics {
            let topic_slice: &[u8] = topic.as_bytes();
            stream.append(&topic_slice);
        }

        let data_slice: &[u8] = log.data.as_slice();
        stream.append(&data_slice);
    }

    let canonical = stream.out().to_vec();

    match detect_tx_type_from_payload(&record.payload) {
        0 => canonical,
        tx_type => {
            let mut typed = Vec::with_capacity(1 + canonical.len());
            typed.push(tx_type);
            typed.extend_from_slice(&canonical);
            typed
        }
    }
}
