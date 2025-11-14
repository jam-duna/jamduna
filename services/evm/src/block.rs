//! Block Payload Structure

use alloc::{vec::Vec, format};
use core::convert::TryInto;
use hex;
use utils::{
    constants::{BLOCK_NUMBER_KEY, NONE, WHAT, FULL},
    effects::{ObjectId, WriteEffectEntry, ObjectRef},
    functions::{log_info, log_error, format_segment},
    host_functions::{read as host_read, write as host_write},
    hash_functions::{blake2b_hash, keccak256},
};
use crate::{
    receipt::{
        TransactionReceiptRecord,
        compute_per_receipt_logs_bloom,
        encode_canonical_receipt_rlp,
    },
    sharding::{format_object_id,  ObjectKind},
};

const BLOCK_HEADER_SIZE: usize = 432; // 8+32+32+8+32+32+8+8+8+8+256 = 432 bytes with logs_bloom field
const METADATA_SIZE: usize = 96; // 32 + 32 + 32 for the 3 metadata fields
const HASH_SIZE: usize = 32;
const BLOOM_SIZE: usize = 256;
const MAX_BLOCKS_IN_JAM_STATE: u64 = 403_200; // 28 days worth of blocks at 6s each

/// Convert block number to ObjectId key format for EvmBlockPayload storage
/// Format: 0xFF repeated 28 times + block_number as 4-byte LE
pub fn block_number_to_object_id(block_number: u32) -> ObjectId {
    let mut key = [0xFF; 32];
    let block_bytes = block_number.to_le_bytes();
    key[28..32].copy_from_slice(&block_bytes);
    key
}

/// Check if an object_id is a block number witness and extract the block number
/// Returns Some(block_number) if object_id starts with 0xFF repeated 28 times, None otherwise
pub fn is_block_number_state_witness(object_id: &ObjectId) -> Option<u32> {
    // Check if first 28 bytes are all 0xFF
    for i in 0..28 {
        if object_id[i] != 0xFF {
            return None;
        }
    }

    // Extract block number from last 4 bytes (little-endian)
    let block_number = u32::from_le_bytes([
        object_id[28],
        object_id[29],
        object_id[30],
        object_id[31],
    ]);

    Some(block_number)
}

/// Convert block number to ObjectId key format for EvmBlockMetadata storage
/// Format: 0xFE repeated 28 times + block_number as 4-byte LE
#[allow(dead_code)]
pub fn block_metadata_to_object_id(block_number: u32) -> ObjectId {
    let mut key = [0xFE; 32];
    let block_bytes = block_number.to_le_bytes();
    key[28..32].copy_from_slice(&block_bytes);
    key
}

/// EVM block metadata structure containing computed values
/// This structure is exported to DA as ObjectKind::BlockMetadata
#[derive(Debug, Clone)]
pub struct EvmBlockMetadata {
    /// BMT root of tx hashes
    pub transactions_root: [u8; 32],
    /// BMT root of receipts
    pub receipts_root: [u8; 32],
    /// MMR root
    pub mmr_root: [u8; 32],
}

/// EVM block payload structure
/// This structure is exported to DA as ObjectKind::Block
#[derive(Debug, Clone)]
pub struct EvmBlockPayload {
    // Fixed header (hashed for block ID) - reduced size without computed metadata
    /// Block number
    pub number: u64,
    /// Previous block hash
    pub parent_hash: [u8; 32],
    /// JAM state root
    pub state_root: [u8; 32],
    /// Log index start
    pub log_index_start: u64,
    /// Extrinsics hash
    pub extrinsics_hash: [u8; 32],
    /// Parent header hash
    pub parent_header_hash: [u8; 32],
    /// Block size
    pub size: u64,
    /// Gas limit
    pub gas_limit: u64,
    /// Gas used
    pub gas_used: u64,
    /// JAM timeslot
    pub timestamp: u64,
    /// Aggregated logs bloom filter
    pub logs_bloom: [u8; 256],

    // Variable data (not hashed to form block hash): tx_hashes and receipt_hashes
    pub tx_hashes: Vec<ObjectId>,
    pub receipt_hashes: Vec<ObjectId>,
}

impl EvmBlockPayload {
    /// Serialize just the fixed header (used for block hash computation)
    pub fn serialize_header(&self) -> [u8; BLOCK_HEADER_SIZE] {
        let mut buffer = [0u8; BLOCK_HEADER_SIZE];
        let mut offset = 0;

        // Fixed-size fields with logs_bloom
        buffer[offset..offset + 8].copy_from_slice(&self.number.to_le_bytes());
        offset += 8;
        buffer[offset..offset + HASH_SIZE].copy_from_slice(&self.parent_hash);
        offset += HASH_SIZE;
        buffer[offset..offset + HASH_SIZE].copy_from_slice(&self.state_root);
        offset += HASH_SIZE;
        buffer[offset..offset + 8].copy_from_slice(&self.log_index_start.to_le_bytes());
        offset += 8;
        buffer[offset..offset + HASH_SIZE].copy_from_slice(&self.extrinsics_hash);
        offset += HASH_SIZE;
        buffer[offset..offset + HASH_SIZE].copy_from_slice(&self.parent_header_hash);
        offset += HASH_SIZE;
        buffer[offset..offset + 8].copy_from_slice(&self.size.to_le_bytes());
        offset += 8;
        buffer[offset..offset + 8].copy_from_slice(&self.gas_limit.to_le_bytes());
        offset += 8;
        buffer[offset..offset + 8].copy_from_slice(&self.gas_used.to_le_bytes());
        offset += 8;
        buffer[offset..offset + 8].copy_from_slice(&self.timestamp.to_le_bytes());
        offset += 8;
        buffer[offset..offset + BLOOM_SIZE].copy_from_slice(&self.logs_bloom);

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

        // Parse fixed header (reduced size without metadata)
        let number = u64::from_le_bytes(data[offset..offset + 8].try_into().map_err(|_| "Invalid number")?);
        offset += 8;

        let parent_hash: [u8; HASH_SIZE] = data[offset..offset + HASH_SIZE]
            .try_into().map_err(|_| "Invalid parent_hash")?;
        offset += HASH_SIZE;

        let state_root: [u8; HASH_SIZE] = data[offset..offset + HASH_SIZE]
            .try_into().map_err(|_| "Invalid state_root")?;
        offset += HASH_SIZE;

        let log_index_start = u64::from_le_bytes(data[offset..offset + 8].try_into().map_err(|_| "Invalid log_index_start")?);
        offset += 8;

        let extrinsics_hash: [u8; HASH_SIZE] = data[offset..offset + HASH_SIZE]
            .try_into().map_err(|_| "Invalid extrinsics_hash")?;
        offset += HASH_SIZE;

        let parent_header_hash: [u8; HASH_SIZE] = data[offset..offset + HASH_SIZE]
            .try_into().map_err(|_| "Invalid parent_header_hash")?;
        offset += HASH_SIZE;

        let size = u64::from_le_bytes(
            data[offset..offset + 8].try_into().map_err(|_| "Invalid size")?
        );
        offset += 8;

        let gas_limit = u64::from_le_bytes(
            data[offset..offset + 8].try_into().map_err(|_| "Invalid gas_limit")?
        );
        offset += 8;

        let gas_used = u64::from_le_bytes(
            data[offset..offset + 8].try_into().map_err(|_| "Invalid gas_used")?
        );
        offset += 8;

        let timestamp = u64::from_le_bytes(
            data[offset..offset + 8].try_into().map_err(|_| "Invalid timestamp")?
        );
        offset += 8;

        let logs_bloom: [u8; BLOOM_SIZE] = data[offset..offset + BLOOM_SIZE]
            .try_into().map_err(|_| "Invalid logs_bloom")?;
        offset += BLOOM_SIZE;

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
            number,
            parent_hash,
            state_root,
            log_index_start,
            extrinsics_hash,
            parent_header_hash,
            size,
            gas_limit,
            gas_used,
            timestamp,
            logs_bloom,
            tx_hashes,
            receipt_hashes,
        })
    }

    /// Create a new EvmBlockPayload with given parameters
    pub fn new(
        number: u64,
        timestamp: u64,
        parent_hash: [u8; 32],
        header: utils::functions::BlockHeader,
        log_index_start: u64,
    ) -> Self {
        use utils::functions::{log_info, format_object_id};

        // Log all 4 header attributes
        log_info(&format!("🏗️ Creating EVM block with header attributes:"));
        log_info(&format!("  📄 Parent Header Hash: {}", format_object_id(header.parent_header_hash)));
        log_info(&format!("  🌳 Parent State Root: {}", format_object_id(header.parent_state_root)));
        log_info(&format!("  📦 Extrinsic Hash: {}", format_object_id(header.extrinsic_hash)));
        log_info(&format!("  🕐 Slot: {} Timestamp: {}", header.slot, timestamp));
        log_info(&format!("  🔢 Log Index Start: {}", log_index_start));

        Self {
            number,
            parent_hash,
            state_root: header.parent_state_root,
            log_index_start,
            extrinsics_hash: header.extrinsic_hash,
            parent_header_hash: header.parent_header_hash,
            size: 0,
            gas_limit: 30_000_000, // Default gas limit
            gas_used: 0,
            timestamp,
            logs_bloom: [0u8; 256], // Initialize empty bloom filter
            tx_hashes: Vec::new(),
            receipt_hashes: Vec::new(),
        }
    }

    /// Write EvmBlockPayload to storage at block_number key
    pub fn write(&self, service_id: u32) -> Option<()> {
        use utils::host_functions::write as host_write;
        use utils::constants::{WHAT, FULL};

        log_info(&format!("🏗️ Writing current_block {} to storage", self.number));

        let key = block_number_to_object_id(self.number as u32);
        let buffer = self.serialize();

        let result = unsafe {
            host_write(
                key.as_ptr() as u64,
                key.len() as u64,
                buffer.as_ptr() as u64,
                buffer.len() as u64,
            )
        };

        // Check for failure codes
        if result == WHAT || result == FULL {
            log_error(&format!("❌ Failed to write block {}, error code: {}", self.number, result));
            return None;
        }
        if self.number > MAX_BLOCKS_IN_JAM_STATE as u64 {
            Self::delete_block(service_id, (self.number - MAX_BLOCKS_IN_JAM_STATE as u64) as u32)?;
        }

        log_info(
            &format!(
                "✅ Wrote block {} with {} transactions",
                self.number,
                self.tx_hashes.len()
            ),
        );

        Some(())
    }

  
    /// Read EvmBlockPayload from storage for given block_number
    /// Returns (EvmBlockPayload, parent_block_hash) if found, otherwise returns None
    pub fn read(service_id: u32, timestamp: u64) -> Option<Self> {
        use utils::functions::{log_info, log_error};
        log_info(&format!("🔍 EvmBlockPayload::read called with service_id={}, timestamp={}", service_id, timestamp));

        // Read current block number from BLOCK_NUMBER_KEY storage
        let block_number = match Self::read_blocknumber_key(service_id) {
            Some(num) => {
                num
            },
            None => {
                return None;
            }
        };

        let key = block_number_to_object_id(block_number);
        log_info(&format!("🔑 Generated object_id key: {}", format_object_id(&key)));

        // Read from storage - allocate a large buffer to read the block data
        let mut buffer = alloc::vec![0u8; 10000];
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

        log_info(&format!("📖 host_read returned {} bytes for block {}", len, block_number));
        if len == NONE {
            log_error(&format!("❌ No data found for block {} (key not in storage, object_id: {})", block_number, format_object_id(&key)));
            return None;
        }
        let len = len as usize;

        buffer.truncate(len);
        let current_block = match Self::deserialize(&buffer) {
            Ok(block) => {
                log_info(&format!("✅ Successfully deserialized block {}, timestamp={}", block.number, block.timestamp));
                block
            },
            Err(e) => {
                log_error(&format!("❌ Failed to deserialize block data: {:?}", e));
                return None;
            }
        };

        if current_block.timestamp == timestamp {
            // our accumulate matches the block timestamp, so we can accumulate!
            log_info(&format!("🎯 Timestamp match! Using existing block {} for accumulation", current_block.number));
            Some(current_block)
        } else if current_block.timestamp > timestamp {
            // our accumulate is somehow too late!
            log_error(&format!("⏰ Accumulate too late! current_block.timestamp={} > timestamp={}", current_block.timestamp, timestamp));
            return None;
        } else {
            let parent_block_hash = blake2b_hash(&current_block.serialize_header());
            Self::write_blocknumber_key(block_number + 1, &parent_block_hash);
            log_info(&format!("📝 Updated BLOCK_NUMBER_KEY to block {}", block_number + 1));

            // Fetch the current block header from the host environment
            use utils::functions::fetch_header;
            // Calculate next log index start: previous block's log_index_start + number of tx_hashes in previous block
            let next_log_index_start = current_block.log_index_start + current_block.tx_hashes.len() as u64;
            let new_block = match fetch_header() {
                Some(header) => {
                    Self::new((block_number + 1) as u64, timestamp, parent_block_hash, header, next_log_index_start)
                },
                None => {
                    log_error(&format!("❌ Failed to fetch block header, creating block with default values"));
                    return None;
                }
            };
            log_info(&format!("🎉 Created new block {} with timestamp={}", new_block.number, new_block.timestamp));
            Some(new_block)
        }
    }

    /// Add a receipt to the block, updating transaction tracking and gas usage
    pub fn add_receipt(&mut self, candidate: &mut crate::writes::ObjectCandidateWrite, record: TransactionReceiptRecord, log_index_start: u64) -> ([u8; 32], u64) {
        candidate.object_ref.timeslot = self.timestamp as u32;
        candidate.object_ref.evm_block = self.number as u32;
        candidate.object_ref.tx_slot = self.tx_hashes.len() as u16;
        candidate.object_ref.log_index = log_index_start as u8;
        self.tx_hashes.push(candidate.object_id);
        self.gas_used = self.gas_used.saturating_add(candidate.object_ref.gas_used as u64);

        // Compute per_receipt_bloom for receipt RLP encoding
        let per_receipt_bloom = compute_per_receipt_logs_bloom(&record.logs);

        // Aggregate the receipt's bloom filter into the block's bloom filter
        for (dst, src) in self.logs_bloom.iter_mut().zip(per_receipt_bloom.iter()) {
            *dst |= src;
        }

        // Canonical Ethereum receipt hash: (optional) tx type prefix + RLP([status, cumulative_gas, logs_bloom, logs])
        let receipt_rlp = encode_canonical_receipt_rlp(&record, self.gas_used, &per_receipt_bloom);
        let receipt_hash: [u8; 32] = keccak256(&receipt_rlp).into();
        self.receipt_hashes.push(receipt_hash);
        return (receipt_hash, log_index_start + record.logs.len() as u64);
    }

    /// Compute block hash from current block state
    pub fn compute_block_hash(&self) -> [u8; 32] {
        blake2b_hash(&self.serialize_header())
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

    /// Write block_hash → block_number mapping for RPC lookup
    /// This allows GetBlockByHash to efficiently find the block
    /// Public wrapper for use by accumulator
    pub fn write_blockhash_mapping_pub(block_hash: &[u8; 32], block_number: u32) {
        Self::write_blockhash_mapping(block_hash, block_number);
    }

    /// Internal implementation of block hash mapping
    fn write_blockhash_mapping(block_hash: &[u8; 32], block_number: u32) {
        use utils::host_functions::write as host_write;
        use utils::constants::{WHAT, FULL};

        log_info(&format!("🔑 write_blockhash_mapping called for block {} with hash {}",
            block_number, format_object_id(block_hash)));

        let block_number_bytes = block_number.to_le_bytes();

        let result = unsafe {
            host_write(
                block_hash.as_ptr() as u64,
                block_hash.len() as u64,
                block_number_bytes.as_ptr() as u64,
                block_number_bytes.len() as u64,
            )
        };

        if result == WHAT || result == FULL {
            log_error(&format!("❌ Failed to write block_hash mapping for block {}", block_number));
        } else {
            log_info(&format!("✅ Wrote block_hash → block_number mapping for block {}", block_number));
        }
    }

    /// Read block from service storage and return the EvmBlockPayload
    pub fn read_block(service_id: u32, block_number: u32) -> Option<Self> {
        let key = block_number_to_object_id(block_number);
        let mut buffer = alloc::vec![0u8; 1000000];

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

        buffer.truncate(len);

        // Deserialize and return the block
        match Self::deserialize(&buffer) {
            Ok(block) => Some(block),
            Err(_) => {
                log_error(&format!("❌ Failed to deserialize block {}", block_number));
                None
            }
        }
    }

    /// Delete old block entry from storage
    pub fn delete_block(service_id: u32, block_number: u32) -> Option<()> {
        use utils::constants::{WHAT, FULL};

        // Read the block to compute its hash
        let block = match Self::read_block(service_id, block_number) {
            Some(block) => block,
            None => {
                log_info(&format!("Block {} not found, skipping deletion", block_number));
                return Some(());
            }
        };

        // Compute the block hash from the header, Delete the block_hash key
        let block_hash = blake2b_hash(&block.serialize_header());
        let result = unsafe {
            host_write(block_hash.as_ptr() as u64, block_hash.len() as u64, 0, 0)
        };
        if result == WHAT || result == FULL {
            log_error(&format!("❌ Failed to delete block_hash for block {}, error code: {}", block_number, result));
            return None;
        }

        // Delete the block by number key
        let key = block_number_to_object_id(block_number);
        let result = unsafe {
            host_write(key.as_ptr() as u64, key.len() as u64, 0, 0)
        };
        if result == WHAT || result == FULL {
            log_error(&format!("❌ Failed to delete block {} entry, error code: {}", block_number, result));
            return None;
        }

        log_info(&format!("Successfully deleted block {} and its hash", block_number));
        Some(())
    }


}


impl EvmBlockMetadata {
    /// Create new EvmBlockMetadata by computing roots from EvmBlockPayload
    pub fn new(payload: &EvmBlockPayload) -> Self {
        Self {
            transactions_root: payload.compute_transactions_root(),
            receipts_root: payload.compute_receipts_root(),
            mmr_root: Self::compute_mmr_root(&payload.receipt_hashes),
        }
    }
    pub fn to_write_effect_entry(&self, payload: &EvmBlockPayload, service_id: u32, work_package_hash: [u8; 32]) -> WriteEffectEntry {
        let serialized = self.serialize();

        // Serialize header to get exact bytes for object_id computation
        let header_bytes = payload.serialize_header();


        let object_id = blake2b_hash(&header_bytes);

        let ref_info = ObjectRef {
            service_id,
            work_package_hash,
            index_start: 0,
            index_end: 0,
            version: 1,
            payload_length: serialized.len() as u32,
            object_kind: ObjectKind::BlockMetadata as u8,
            timeslot: payload.timestamp as u32,
            evm_block: payload.number as u32,
            tx_slot: payload.tx_hashes.len() as u16,
            log_index: payload.log_index_start as u8,
            gas_used: payload.gas_used as u32,
        };
        WriteEffectEntry {
            object_id,
            ref_info: ref_info.clone(),
            payload: serialized.to_vec(),
        }
    }

    /// Compute MMR root by appending receipt hashes to a new MMR and getting super_peak
    fn compute_mmr_root(receipt_hashes: &[ObjectId]) -> [u8; 32] {
        use crate::mmr::MMR;

        let mut mmr = MMR::new();
        for receipt_hash in receipt_hashes {
            mmr.append(*receipt_hash);
        }
        mmr.super_peak()
    }

    /// Serialize the metadata to bytes for DA export
    pub fn serialize(&self) -> [u8; METADATA_SIZE] {
        let mut buffer = [0u8; METADATA_SIZE];
        let mut offset = 0;

        // transactions_root
        buffer[offset..offset + HASH_SIZE].copy_from_slice(&self.transactions_root);
        offset += HASH_SIZE;

        // receipts_root
        buffer[offset..offset + HASH_SIZE].copy_from_slice(&self.receipts_root);
        offset += HASH_SIZE;

        // mmr_root
        buffer[offset..offset + HASH_SIZE].copy_from_slice(&self.mmr_root);

        buffer
    }

    /// Deserialize metadata from bytes
    pub fn deserialize(data: &[u8]) -> Result<Self, &'static str> {
        if data.len() < METADATA_SIZE {
            return Err("Metadata payload too short");
        }

        let mut offset = 0;

        let transactions_root: [u8; HASH_SIZE] = data[offset..offset + HASH_SIZE]
            .try_into().map_err(|_| "Invalid transactions_root")?;
        offset += HASH_SIZE;

        let receipts_root: [u8; HASH_SIZE] = data[offset..offset + HASH_SIZE]
            .try_into().map_err(|_| "Invalid receipts_root")?;
        offset += HASH_SIZE;

        let mmr_root: [u8; HASH_SIZE] = data[offset..offset + HASH_SIZE]
            .try_into().map_err(|_| "Invalid mmr_root")?;

        Ok(EvmBlockMetadata {
            transactions_root,
            receipts_root,
            mmr_root,
        })
    }

    /// Write metadata to JAM state storage at the given object_id
    pub fn write(&self, object_id: &ObjectId) ->Option<()>  {
        use utils::host_functions::write;

        let serialized = self.serialize();
        let key_buffer = *object_id;

        let result = unsafe {
            write(
                key_buffer.as_ptr() as u64,
                key_buffer.len() as u64,
                serialized.as_ptr() as u64,
                serialized.len() as u64,
            )
        };
        // Check for failure codes
        if result == WHAT || result == FULL {
            log_error(&format!("❌ Failed to write Block Metadata object_id: {}, error code: {}", format_object_id(&object_id), result));
            return None;
        }

        log_info(&format!("📝 Writing Block Metadata object_id: {}, serialized: {}", format_object_id(&object_id), format_segment(&serialized)));
        Some(())
    }
}
