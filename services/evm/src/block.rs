//! Block Payload Structure

use alloc::{vec::Vec, format};
use core::convert::TryInto;
use utils::{
    constants::{BLOCK_NUMBER_KEY, NONE},
    effects::ObjectId,
    functions::{log_info, log_error},
    host_functions::{read as host_read, write as host_write},
    hash_functions::{blake2b_hash, keccak256},
};
use crate::{
    receipt::{
        TransactionReceiptRecord,
        compute_per_receipt_logs_bloom,
        encode_canonical_receipt_rlp,
    },
};

const BLOCK_HEADER_SIZE: usize = 120; // 4+32+32+32+4+4+8+4 = 120 bytes without parent_hash and logs_bloom fields
const HASH_SIZE: usize = 32;

/// EVM block payload structure
/// This structure is exported to DA as ObjectKind::Block
#[derive(Debug, Clone)]
pub struct EvmBlockPayload {
    // Fixed header (hashed for block ID) - reduced size without computed metadata
    /// Payload length
    pub payload_length: u32,
    /// Number of transactions
    pub num_transactions: u32,
    /// Number of receipts
    pub num_receipts: u32,
    /// JAM timeslot
    pub timestamp: u32,
    /// Gas used
    pub gas_used: u64,
    /// JAM state root
    pub state_root: [u8; 32],
    /// Transactions root
    pub transactions_root: [u8; 32],
    /// Receipt root
    pub receipt_root: [u8; 32],

    pub tx_hashes: Vec<ObjectId>,
    pub receipt_hashes: Vec<ObjectId>,
}

pub fn block_number_to_object_id(block_number: u32) -> ObjectId {
    let mut key = [0xFF; 32];
    let block_bytes = block_number.to_le_bytes();
    key[28..32].copy_from_slice(&block_bytes);
    key
}

impl EvmBlockPayload {
    /// Serialize just the fixed header (used for block hash computation)
    pub fn serialize_header(&self) -> [u8; BLOCK_HEADER_SIZE] {
        let mut buffer = [0u8; BLOCK_HEADER_SIZE];
        let mut offset = 0;

        // Fixed-size fields: payload_length, numeric fields, then hash fields
        buffer[offset..offset + 4].copy_from_slice(&self.payload_length.to_le_bytes());
        offset += 4;
        buffer[offset..offset + 4].copy_from_slice(&self.num_transactions.to_le_bytes());
        offset += 4;
        buffer[offset..offset + 4].copy_from_slice(&self.num_receipts.to_le_bytes());
        offset += 4;
        buffer[offset..offset + 4].copy_from_slice(&self.timestamp.to_le_bytes());
        offset += 4;
        buffer[offset..offset + 8].copy_from_slice(&self.gas_used.to_le_bytes());
        offset += 8;
        buffer[offset..offset + HASH_SIZE].copy_from_slice(&self.state_root);
        offset += HASH_SIZE;
        buffer[offset..offset + HASH_SIZE].copy_from_slice(&self.transactions_root);
        offset += HASH_SIZE;
        buffer[offset..offset + HASH_SIZE].copy_from_slice(&self.receipt_root);

        buffer
    }

    /// Serialize the block payload to bytes for DA export
    pub fn serialize(&self) -> Vec<u8> {
        let mut buffer = Vec::new();

        // Add the reduced-size fixed header
        buffer.extend_from_slice(&self.serialize_header());

        // Transaction hashes with length prefix
        buffer.extend_from_slice(&(self.tx_hashes.len() as u32).to_le_bytes());
        for tx_hash in &self.tx_hashes {
            buffer.extend_from_slice(tx_hash);
        }

        // Receipt hashes with length prefix (its the same as the above length prefix)
        buffer.extend_from_slice(&(self.receipt_hashes.len() as u32).to_le_bytes());
        for receipt_hash in &self.receipt_hashes {
            buffer.extend_from_slice(receipt_hash);
        }

        buffer
    }

    /// Deserialize block payload from bytes
    pub fn deserialize(data: &[u8]) -> Result<Self, &'static str> {
        if data.len() < BLOCK_HEADER_SIZE + 4 { // minimum size: header + first length prefix
            return Err("Block payload too short");
        }

        let mut offset = 0;

        // Parse fixed header: payload_length, numeric fields, then hash fields
        let payload_length = u32::from_le_bytes(data[offset..offset + 4].try_into().map_err(|_| "Invalid payload_length")?);
        offset += 4;

        let num_transactions = u32::from_le_bytes(
            data[offset..offset + 4].try_into().map_err(|_| "Invalid num_transactions")?
        );
        offset += 4;

        let num_receipts = u32::from_le_bytes(
            data[offset..offset + 4].try_into().map_err(|_| "Invalid num_receipts")?
        );
        offset += 4;

        let timestamp = u32::from_le_bytes(
            data[offset..offset + 4].try_into().map_err(|_| "Invalid timestamp")?
        );
        offset += 4;

        let gas_used = u64::from_le_bytes(
            data[offset..offset + 8].try_into().map_err(|_| "Invalid gas_used")?
        );
        offset += 8;

        let state_root: [u8; HASH_SIZE] = data[offset..offset + HASH_SIZE]
            .try_into().map_err(|_| "Invalid state_root")?;
        offset += HASH_SIZE;

        let transactions_root: [u8; HASH_SIZE] = data[offset..offset + HASH_SIZE]
            .try_into().map_err(|_| "Invalid transactions_root")?;
        offset += HASH_SIZE;

        let receipt_root: [u8; HASH_SIZE] = data[offset..offset + HASH_SIZE]
            .try_into().map_err(|_| "Invalid receipt_root")?;
        offset += HASH_SIZE;

        // Parse variable data: transaction hashes
        let tx_count = u32::from_le_bytes(
            data[offset..offset + 4].try_into().map_err(|_| "Invalid tx_count")?
        ) as usize;
        offset += 4;

        let mut tx_hashes = Vec::with_capacity(tx_count);
        for _ in 0..tx_count {
            if offset + 32 > data.len() {
                return Err("Insufficient data for tx_hashes");
            }
            let tx_hash: [u8; 32] = data[offset..offset + 32]
                .try_into().map_err(|_| "Invalid tx_hash")?;
            tx_hashes.push(tx_hash);
            offset += 32;
        }

        // Parse variable data: receipt_hashes (full ObjectRef structures)
        let receipt_count = u32::from_le_bytes(
            data[offset..offset + 4].try_into().map_err(|_| "Invalid receipt_count")?
        ) as usize;
        offset += 4;

        let mut receipt_hashes = Vec::with_capacity(receipt_count);
        for _ in 0..receipt_count {
            let receipt_hash: [u8; 32] = data[offset..offset + 32]
                .try_into().map_err(|_| "Invalid receipt_hash")?;
            receipt_hashes.push(receipt_hash);
            offset += 32;
        }

        Ok(EvmBlockPayload {
            payload_length,
            state_root,
            transactions_root,
            receipt_root,
            num_transactions,
            num_receipts,
            gas_used,
            timestamp,
            tx_hashes,
            receipt_hashes,
        })
    }

    /// Create a new EvmBlockPayload with given parameters
    pub fn new(
        payload_length: u32,
        timestamp: u32,
        state_root: [u8; 32]
    ) -> Self {
        use utils::functions::{log_info, format_object_id};

        // Log all header attributes
        log_info(&format!("🏗️ Creating EVM block with attributes:"));
        log_info(&format!("  🌳 State Root: {}", format_object_id(state_root)));
        log_info(&format!("  🕐 Timestamp: {}", timestamp));

        Self {
            payload_length,
            state_root,
            transactions_root: [0u8; 32], // Will be computed later
            receipt_root: [0u8; 32], // Will be computed later
            num_transactions: 0,
            num_receipts: 0,
            gas_used: 0,
            timestamp,
            tx_hashes: Vec::new(),
            receipt_hashes: Vec::new(),
        }
    }

    /// Prepare EvmBlockPayload for DA export with computed roots
    /// This replaces the old write() method since blocks are now exported to DA
    pub fn prepare_for_da_export(&mut self) {
        use utils::functions::log_info;

        // Compute transaction_root from tx_hashes
        self.transactions_root = self.compute_transactions_root();

        // Compute receipt_root from receipt_hashes
        self.receipt_root = self.compute_receipts_root();

        // Update counts
        self.num_transactions = self.tx_hashes.len() as u32;
        self.num_receipts = self.receipt_hashes.len() as u32;

        log_info(&format!(
            "✅ Prepared block for DA export: payload_length={}, {} transactions, {} receipts",
            self.payload_length,
            self.num_transactions,
            self.num_receipts
        ));
    }

    /// Add a receipt to the block, updating transaction tracking and gas usage
    pub fn add_receipt(&mut self, candidate: &mut crate::writes::ObjectCandidateWrite, record: TransactionReceiptRecord, log_index_start: u64) -> ([u8; 32], u64) {
        // Note: ObjectRef fields have been simplified - removed timeslot, evm_block, tx_slot, log_index, gas_used
        self.tx_hashes.push(candidate.object_id);

        // Compute per_receipt_bloom for receipt RLP encoding
        let per_receipt_bloom = compute_per_receipt_logs_bloom(&record.logs);

        // Canonical Ethereum receipt hash: (optional) tx type prefix + RLP([status, cumulative_gas, logs_bloom, logs])
        let receipt_rlp = encode_canonical_receipt_rlp(&record, self.gas_used, &per_receipt_bloom);
        let receipt_hash: [u8; 32] = keccak256(&receipt_rlp).into();
        self.receipt_hashes.push(receipt_hash);
        return (receipt_hash, log_index_start + record.logs.len() as u64);
    }

    /// Compute transaction root using JAM BMT from this block's transaction hashes
    ///
    /// The transaction root is computed from transaction hashes indexed by their
    /// position in the block (0, 1, 2, ...) following Ethereum's MPT structure.
    pub fn compute_transactions_root(&self) -> [u8; 32] {
        if self.tx_hashes.is_empty() {
            // Ethereum empty trie root
            return blake2b_hash(&[]);
        }

        // Convert tx_hashes to key-value pairs for proper JAM BMT computation
        let kvs: Vec<([u8; 32], Vec<u8>)> = self.tx_hashes
            .iter()
            .enumerate()
            .map(|(index, hash)| {
                let mut key = [0u8; 32];
                // Use little-endian encoding at the start of the key (natural position)
                let index_bytes = (index as u32).to_le_bytes();
                key[0..4].copy_from_slice(&index_bytes);
                (key, hash.to_vec())
            })
            .collect();

        crate::bmt::compute_bmt_root(&kvs)
    }

    /// Compute receipts root using JAM BMT from this block's receipt hashes
    ///
    /// The receipts root is computed from receipt hashes indexed by their
    /// position in the block (0, 1, 2, ...) following Ethereum's MPT structure.
    pub fn compute_receipts_root(&self) -> [u8; 32] {
        if self.receipt_hashes.is_empty() {
            // Ethereum empty trie root
            return blake2b_hash(&[]);
        }

        // Convert receipt_hashes to key-value pairs for proper JAM BMT computation
        let kvs: Vec<([u8; 32], Vec<u8>)> = self.receipt_hashes
            .iter()
            .enumerate()
            .map(|(index, hash)| {
                let mut key = [0u8; 32];
                // Use little-endian encoding at the start of the key (natural position)
                let index_bytes = (index as u32).to_le_bytes();
                key[0..4].copy_from_slice(&index_bytes);
                (key, hash.to_vec())
            })
            .collect();

        crate::bmt::compute_bmt_root(&kvs)
    }

    /// Read the current block number from service storage
    pub fn read_blocknumber_key(service_id: u32) -> Option<u32> {
        let mut buffer = [0u8; 36]; // 4 bytes block_number + 32 bytes parent_hash

        let len = unsafe {
            host_read(
                service_id as u64,
                BLOCK_NUMBER_KEY.as_ptr() as u64,
                BLOCK_NUMBER_KEY.len() as u64,
                buffer.as_mut_ptr() as u64,
                0,
                buffer.len() as u64,
            )
        };

        if len == NONE {
            return None; // Key not found
        }

        if len < 4 {
            return None; // Insufficient data
        }

        Some(u32::from_le_bytes([buffer[0], buffer[1], buffer[2], buffer[3]]))
    }

    /// Write BLOCK_NUMBER_KEY with current block_number + parent_hash
    pub fn write_blocknumber_key(block_number: u32, parent_hash: &[u8; 32]) {
        let mut buffer = Vec::with_capacity(4 + 32);
        buffer.extend_from_slice(&block_number.to_le_bytes());
        buffer.extend_from_slice(parent_hash);

        unsafe {
            host_write(
                BLOCK_NUMBER_KEY.as_ptr() as u64,
                BLOCK_NUMBER_KEY.len() as u64,
                buffer.as_ptr() as u64,
                buffer.len() as u64,
            );
        }
    }
    pub fn write_block_hashes(block_number: u32, hashes: &Vec<ObjectId>) {
        let key = block_number_to_object_id(block_number);
        let mut buffer = Vec::with_capacity(hashes.len() * HASH_SIZE);
        for hash in hashes {
            buffer.extend_from_slice(hash);
        }

        unsafe {
            host_write(
                key.as_ptr() as u64,
                key.len() as u64,
                buffer.as_ptr() as u64,
                buffer.len() as u64,
            );
        }
    }
    
    ///  Read block from service storage and return a Vec<ObjectIDs> (work package hashes)
    pub fn read_block_hashes(service_id: u32, block_number: u32) -> Option<Vec<ObjectId>> {
        let key = block_number_to_object_id(block_number);
        let mut buffer = alloc::vec![0u8; 341*HASH_SIZE];

        let len = unsafe {
            host_read(
                service_id as u64,
                key.as_ptr() as u64,
                key.len() as u64,
                buffer.as_mut_ptr() as u64,
                0 as u64,
                buffer.len() as u64,
            )
        };

        if len == NONE {
            return None; // Key not found
        }
        let len = len as usize;
        // num of hashes = len / 32
        let num_hashes = len / 32;
        buffer.truncate(len);
        let mut hashes = Vec::with_capacity(num_hashes);
        for i in 0..num_hashes {
            let start = i * 32;
            let end = start + 32;
            let hash: [u8; 32] = buffer[start..end].try_into()
                .map_err(|_| "Invalid hash data").ok()?;
            hashes.push(hash);  
        }
        Some(hashes)
    }

    /// Delete old block entry from storage
    pub fn delete_block(_service_id: u32, block_number: u32) -> Option<()> {
        use utils::constants::{WHAT, FULL};

        // Delete the block payload by number key
        let key = block_number_to_object_id(block_number);
        let result = unsafe {
            host_write(key.as_ptr() as u64, key.len() as u64, 0, 0)
        };
        if result == WHAT || result == FULL {
            log_error(&format!("❌ Failed to delete block {} payload, error code: {}", block_number, result));
            return None;
        }

        log_info(&format!("✅ Successfully deleted block {}, its metadata, and hash mapping", block_number));
        Some(())
    }


}

