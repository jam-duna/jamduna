//! Receipt and Log serialization for JAM DA export
//!
//! This module handles serialization and deserialization of:
//! - EVM logs (events emitted by contracts)
//! - Transaction receipts (execution results with logs and transaction data)
//!
//! Receipt format includes the FULL transaction payload, eliminating the need
//! for separate transaction objects in DA storage.

use alloc::vec::Vec;
use alloc::format;
use evm::interpreter::runtime::Log;
use primitive_types::U256;
use rlp::RlpStream;

/// Metadata collected for each executed transaction so receipts can be exported.
#[derive(Clone)]
pub struct TransactionReceiptRecord {
    /// Keccak-256 hash of the original transaction payload.
    pub hash: [u8; 32],
    /// True when the transaction executed successfully without reverting.
    pub success: bool,
    /// Total gas consumed by the transaction according to the EVM interpreter.
    pub used_gas: U256,
    /// Cumulative gas used in the block up to and including this transaction.
    pub cumulative_gas: u32,
    /// Starting index for logs in the block's log array.
    pub log_index_start: u32,
    /// Transaction index within the block (position in the block's transaction list).
    pub tx_index: u32,
    /// Logs emitted during execution, captured when the transaction commits.
    pub logs: Vec<Log>,
    /// Original transaction payload (extrinsic data).
    pub payload: Vec<u8>,
}

impl TransactionReceiptRecord {}

/// Returns the ObjectID for a receipt (transaction hash)
/// DEPRECATED: Use block_receipts_object_id for block-scoped receipt storage
/// Meta-shard overwrites make per-tx object IDs unreliable for multi-bundle scenarios.
pub fn receipt_object_id_from_receipt(record: &TransactionReceiptRecord) -> [u8; 32] {
    // Receipt ObjectID = transaction hash for direct RPC lookup
    record.hash
}

/// Returns the ObjectID for block receipts blob keyed by BlockCommitment
/// Format: [0xFE][block_commitment bytes 1-31] (first byte replaced with 0xFE prefix)
///
/// BlockCommitment is stable across bundle rebuilds because it's computed from:
/// - Block header fields (PayloadLength, NumTransactions, Timestamp, GasUsed)
/// - State roots (UBTRoot, TransactionsRoot, ReceiptRoot, BlockAccessListHash)
///
/// These are all deterministic given the same transactions, unlike WPH which
/// changes based on RefineContext (anchor, prerequisites).
///
/// This avoids collision with:
/// - Block metadata objects (0xFF prefix)
/// - Meta-shard objects (txHash-based)
/// - Other object types
///
/// Slow path flow for receipt lookup:
/// 1. Scan blocks to find which block contains txHash
/// 2. Compute BlockCommitment from the block
/// 3. Fetch block receipts via block_receipts_object_id(block_commitment)
/// 4. Search receipts for matching txHash
pub fn block_receipts_object_id(block_commitment: [u8; 32]) -> [u8; 32] {
    let mut key = block_commitment;
    key[0] = 0xFE; // Replace first byte with prefix to avoid collisions
    key
}

/// Serialize all receipts for a block into a single blob
/// Format: [receipt_count:4][receipt_0][receipt_1]...[receipt_n]
/// Each receipt: [receipt_len:4][receipt_data:receipt_len]
pub fn serialize_block_receipts(receipts: &[&TransactionReceiptRecord]) -> Vec<u8> {
    let mut result = Vec::new();

    // Receipt count (4 bytes)
    result.extend_from_slice(&(receipts.len() as u32).to_le_bytes());

    // Serialize each receipt with length prefix
    for record in receipts {
        let receipt_data = serialize_receipt(record);
        result.extend_from_slice(&(receipt_data.len() as u32).to_le_bytes());
        result.extend_from_slice(&receipt_data);
    }

    result
}

/// Set to true to enable verbose receipt serialization logs
const RECEIPT_VERBOSE: bool = false;

/// Serialize logs to DA format for receipt objects
/// Format: [log_count:u16][Log...][Log...]
/// Each Log: [address:20B][topic_count:u8][topics:32B*N][data_len:u32][data:bytes]
fn serialize_logs(logs: &[Log]) -> Vec<u8> {
    #[allow(unused_imports)]
    use utils::functions::log_info;

    let mut result = Vec::new();

    // Log count (2 bytes)
    let count = logs.len() as u16;
    if RECEIPT_VERBOSE {
        log_info(&format!("📝 Serializing {} logs to receipt", count));

        for (i, log) in logs.iter().enumerate() {
            log_info(&format!("  Log {}: address={:?}, topics={}, data_len={}",
                i, log.address, log.topics.len(), log.data.len()));
        }
    }

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
/// Receipt format:
/// [logs_payload_len:4][logs_payload:variable][tx_hash:32][tx_type:1][status:1]
/// [used_gas:8][cumulative_gas:4][log_index_start:4][tx_index:4][tx_payload_len:4][tx_payload:variable]
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

    result.extend_from_slice(&record.hash);

    // Detect tx_type from transaction payload (EIP-2718)
    let tx_type = detect_tx_type(&record.payload);
    result.push(tx_type);
    result.push(if record.success { 1 } else { 0 });

    // Gas as u64 (8 bytes) - sufficient for all practical gas values
    let gas_u64 = record.used_gas.low_u64();
    result.extend_from_slice(&gas_u64.to_le_bytes());

    // Cumulative gas (4 bytes) - updated in accumulator
    result.extend_from_slice(&record.cumulative_gas.to_le_bytes());

    // Log index start (4 bytes) - updated in accumulator
    result.extend_from_slice(&record.log_index_start.to_le_bytes());

    // Transaction index (4 bytes) - position within block
    result.extend_from_slice(&record.tx_index.to_le_bytes());

    // Transaction payload (raw RLP) - this is the FULL transaction data
    // RPC uses this to reconstruct the transaction without a separate tx object
    result.extend_from_slice(&(record.payload.len() as u32).to_le_bytes());
    result.extend_from_slice(&record.payload);

    result
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
) -> Vec<u8> {
    let status_byte: u8 = if record.success { 1 } else { 0 };

    let mut stream = RlpStream::new_list(4);
    stream.append(&status_byte);
    stream.append(&cumulative_gas);
    // Use 256 bytes for RLP compatibility
    let zero_bloom = [0u8; 256];
    let bloom_slice: &[u8] = &zero_bloom[..];
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
