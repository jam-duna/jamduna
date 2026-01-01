//! Block Payload Structure

use alloc::{format, vec::Vec};
use core::convert::TryInto;
use utils::{
    constants::{BLOCK_NUMBER_KEY, NONE},
    effects::ObjectId,
    functions::{log_error, log_info},
    hash_functions::blake2b_hash,
    host_functions::{read as host_read, write as host_write},
};

const BLOCK_HEADER_SIZE: usize = 148; // 4+4+4+8+32+32+32+32 = 148 bytes 
const HASH_SIZE: usize = 32;

/// Verkle state delta for delta replay (see services/evm/docs/VERKLE.md)
#[derive(Debug, Clone)]
pub struct VerkleStateDelta {
    pub num_entries: u32,
    pub entries: Vec<u8>,  // Flattened [key(32B), value(32B)] pairs
}

impl VerkleStateDelta {
    pub fn serialize(&self) -> Vec<u8> {
        let mut buffer = Vec::new();
        buffer.extend_from_slice(&self.num_entries.to_le_bytes());
        buffer.extend_from_slice(&self.entries);
        buffer
    }

    pub fn deserialize(data: &[u8]) -> Result<Self, &'static str> {
        if data.len() < 4 {
            return Err("Delta too short");
        }
        let num_entries = u32::from_le_bytes(
            data[0..4].try_into().map_err(|_| "Invalid num_entries")?
        );
        let expected_len = 4 + (num_entries as usize * 64);
        if data.len() != expected_len {
            return Err("Invalid delta length");
        }
        Ok(VerkleStateDelta {
            num_entries,
            entries: data[4..].to_vec(),
        })
    }
}

/// EVM block payload structure
/// This structure is exported to DA as ObjectKind::Block
#[derive(Debug, Clone)]
pub struct EvmBlockPayload {
    // Fixed header (hashed for block ID) - reduced size without computed metadata
    /// Payload length
    pub payload_length: u32,
    /// Number of transactions
    pub num_transactions: u32,
    /// JAM timeslot
    pub timestamp: u32,
    /// Gas used
    pub gas_used: u64,
    /// Verkle tree root (state commitment)
    pub verkle_root: [u8; 32],
    /// Transactions root
    pub transactions_root: [u8; 32],
    /// Receipt root
    pub receipt_root: [u8; 32],
    /// Block Access List hash (Blake2b)
    pub block_access_list_hash: [u8; 32],

    pub tx_hashes: Vec<ObjectId>,
    pub receipt_hashes: Vec<ObjectId>,

    // NEW: Verkle state delta for replay (not included in header hash)
    pub verkle_delta: Option<VerkleStateDelta>,
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
        buffer[offset..offset + 4].copy_from_slice(&self.timestamp.to_le_bytes());
        offset += 4;
        buffer[offset..offset + 8].copy_from_slice(&self.gas_used.to_le_bytes());
        offset += 8;
        buffer[offset..offset + HASH_SIZE].copy_from_slice(&self.verkle_root);
        offset += HASH_SIZE;
        buffer[offset..offset + HASH_SIZE].copy_from_slice(&self.transactions_root);
        offset += HASH_SIZE;
        buffer[offset..offset + HASH_SIZE].copy_from_slice(&self.receipt_root);
        offset += HASH_SIZE;
        buffer[offset..offset + HASH_SIZE].copy_from_slice(&self.block_access_list_hash);

        buffer
    }

    /// Serialize the block payload to bytes for DA export
    pub fn serialize(&self) -> Vec<u8> {
        let mut buffer = Vec::new();

        // Add the reduced-size fixed header
        buffer.extend_from_slice(&self.serialize_header());

        // Transaction hashes (no length prefix - use num_transactions from header)
        for tx_hash in &self.tx_hashes {
            buffer.extend_from_slice(tx_hash);
        }

        // Receipt hashes (no length prefix - use num_transactions from header)
        for receipt_hash in &self.receipt_hashes {
            buffer.extend_from_slice(receipt_hash);
        }

        // Verkle delta (optional, for services/evm/docs/VERKLE.md delta replay)
        if let Some(delta) = &self.verkle_delta {
            buffer.extend_from_slice(&delta.serialize());
        }

        buffer
    }

    /// Deserialize block payload from bytes
    #[allow(dead_code)]
    pub fn deserialize(data: &[u8]) -> Result<Self, &'static str> {
        // Minimum size: fixed header (116)
        if data.len() < BLOCK_HEADER_SIZE {
            return Err("Block payload too short");
        }

        let mut offset = 0;

        // Parse fixed header: payload_length, numeric fields, then hash fields
        let payload_length = u32::from_le_bytes(
            data[offset..offset + 4]
                .try_into()
                .map_err(|_| "Invalid payload_length")?,
        );
        offset += 4;

        let num_transactions = u32::from_le_bytes(
            data[offset..offset + 4]
                .try_into()
                .map_err(|_| "Invalid num_transactions")?,
        );
        offset += 4;

        let timestamp = u32::from_le_bytes(
            data[offset..offset + 4]
                .try_into()
                .map_err(|_| "Invalid timestamp")?,
        );
        offset += 4;

        let gas_used = u64::from_le_bytes(
            data[offset..offset + 8]
                .try_into()
                .map_err(|_| "Invalid gas_used")?,
        );
        offset += 8;

        let verkle_root: [u8; HASH_SIZE] = data[offset..offset + HASH_SIZE]
            .try_into()
            .map_err(|_| "Invalid verkle_root")?;
        offset += HASH_SIZE;

        let transactions_root: [u8; HASH_SIZE] = data[offset..offset + HASH_SIZE]
            .try_into()
            .map_err(|_| "Invalid transactions_root")?;
        offset += HASH_SIZE;

        let receipt_root: [u8; HASH_SIZE] = data[offset..offset + HASH_SIZE]
            .try_into()
            .map_err(|_| "Invalid receipt_root")?;
        offset += HASH_SIZE;

        let block_access_list_hash: [u8; HASH_SIZE] = data[offset..offset + HASH_SIZE]
            .try_into()
            .map_err(|_| "Invalid block_access_list_hash")?;
        offset += HASH_SIZE;

        // Parse variable data: transaction hashes (use num_transactions from header)
        let tx_count = num_transactions as usize;
        let mut tx_hashes = Vec::with_capacity(tx_count);
        for _ in 0..tx_count {
            if offset + 32 > data.len() {
                return Err("Insufficient data for tx_hashes");
            }
            let tx_hash: [u8; 32] = data[offset..offset + 32]
                .try_into()
                .map_err(|_| "Invalid tx_hash")?;
            tx_hashes.push(tx_hash);
            offset += 32;
        }

        // Parse variable data: receipt_hashes (use num_transactions from header)
        let receipt_count = num_transactions as usize;
        let mut receipt_hashes = Vec::with_capacity(receipt_count);
        for _ in 0..receipt_count {
            if offset + 32 > data.len() {
                return Err("Insufficient data for receipt_hashes");
            }
            let receipt_hash: [u8; 32] = data[offset..offset + 32]
                .try_into()
                .map_err(|_| "Invalid receipt_hash")?;
            receipt_hashes.push(receipt_hash);
            offset += 32;
        }

        // Parse verkle delta if present (optional field)
        let verkle_delta = if offset < data.len() {
            Some(VerkleStateDelta::deserialize(&data[offset..])?)
        } else {
            None
        };

        Ok(EvmBlockPayload {
            payload_length,
            verkle_root,
            transactions_root,
            receipt_root,
            block_access_list_hash,
            num_transactions,
            gas_used,
            timestamp,
            tx_hashes,
            receipt_hashes,
            verkle_delta,
        })
    }

    /// Prepare EvmBlockPayload for DA export with computed roots
    /// This replaces the old write() method since blocks are now exported to DA
    pub fn prepare_for_da_export(&mut self) {
        // Compute transaction_root from tx_hashes
        self.transactions_root = self.compute_transactions_root();

        // Compute receipt_root from receipt_hashes
        self.receipt_root = self.compute_receipts_root();

        // Update transaction count
        self.num_transactions = self.tx_hashes.len() as u32;
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
        let kvs: Vec<([u8; 32], Vec<u8>)> = self
            .tx_hashes
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
        let kvs: Vec<([u8; 32], Vec<u8>)> = self
            .receipt_hashes
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
        let mut buffer = [0u8; 4]; // 4 bytes block_number 

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

        Some(u32::from_le_bytes([
            buffer[0], buffer[1], buffer[2], buffer[3],
        ]))
    }

    /// Write BLOCK_NUMBER_KEY with current block_number
    pub fn write_blocknumber_key(block_number: u32) {
        let mut buffer = Vec::with_capacity(4 + 32);
        buffer.extend_from_slice(&block_number.to_le_bytes());

        unsafe {
            host_write(
                BLOCK_NUMBER_KEY.as_ptr() as u64,
                BLOCK_NUMBER_KEY.len() as u64,
                buffer.as_ptr() as u64,
                buffer.len() as u64,
            );
        }
    }

    /// Delete old block entry from storage
    pub fn delete_block(service_id: u32, block_number: u32) -> Option<()> {
        use utils::constants::{FULL, WHAT};

        // 1) Resolve work_package_hash from blocknumber → wph mapping
        let bn_key = block_number_to_object_id(block_number);
        let mut buffer = [0u8; 36]; // 32 bytes wph + 4 bytes timeslot
        let len = unsafe {
            host_read(
                service_id as u64,          // service_id (s)
                bn_key.as_ptr() as u64,     // key_offset (ko)
                bn_key.len() as u64,        // key_size (kz)
                buffer.as_mut_ptr() as u64, // value_offset (o)
                0,                          // offset into buffer (f)
                buffer.len() as u64,        // buffer length (l)
            )
        };

        if len < 32 {
            log_error(&format!(
                "❌ Failed to resolve block {} mapping for deletion (len={})",
                block_number, len
            ));
            return None;
        }

        let mut work_package_hash = [0u8; 32];
        work_package_hash.copy_from_slice(&buffer[0..32]);

        // 2) Delete blocknumber → wph mapping
        let result_bn = unsafe { host_write(bn_key.as_ptr() as u64, bn_key.len() as u64, 0, 0) };
        if result_bn == WHAT || result_bn == FULL {
            log_error(&format!(
                "❌ Failed to delete blocknumber mapping for {} (code={})",
                block_number, result_bn
            ));
            return None;
        }

        // 3) Delete work_package_hash → (block_number, timeslot) mapping
        let result_wph = unsafe {
            host_write(
                work_package_hash.as_ptr() as u64,
                work_package_hash.len() as u64,
                0,
                0,
            )
        };
        if result_wph == WHAT || result_wph == FULL {
            log_error(&format!(
                "❌ Failed to delete work_package_hash mapping for block {} (code={})",
                block_number, result_wph
            ));
            return None;
        }

        // Note: block payload itself lives in DA and is pruned by retention policies outside host storage

        log_info(&format!(
            "✅ Successfully deleted block {}, its metadata, and hash mapping",
            block_number
        ));
        Some(())
    }
}
