//! Contract Storage Sharding - Adaptive SSR for EVM Contract Storage
//!
//! # Overview
//!
//! This module implements **contract-specific** adaptive storage sharding using SSR (Sparse Split Registry).
//! It manages EVM contract storage with adaptive sharding and provides DA object identification.
//!
//! **USED BY**: Contract sharding ONLY (meta-sharding has separate structures in meta_sharding.rs)
//!
//! # Relationship with Other Modules
//!
//! - **Shared utilities from `da.rs`**: `ShardId`, `SplitPoint`, prefix helpers
//! - **Independent from meta-sharding**: Contract SSR structures are separate from meta-sharding SSR
//! - **Contract-specific**: All SSR structures and logic in this module are for contract storage only
//!
//! # Architecture
//!
//! - **Contract SSR**: Tracks split points and shard depths for contract storage (separate from meta SSR)
//! - **Contract Shards**: 4KB max storage buckets containing EVM key-value pairs
//! - **Adaptive Splitting**: Shards split when exceeding 55 entries
//! - **Object-based Storage**: Each shard is a separate DA object for parallel access
//!
//! # Data Structures
//!
//! - `SSRHeader`: Contract SSR header (13 bytes) - separate from meta-sharding SSRHeader
//! - `SSRData`: Contract SSR registry - separate from meta-sharding SSRData
//! - `EvmEntry`: EVM key-value pair (32B key_hash + 32B value)
//! - `ContractSSR`: Type alias for contract SSRData
//! - `ContractShard`: Contract shard containing EvmEntry entries
//! - `ContractStorage`: Complete storage for one contract (SSR + shards map)
//! - `ObjectKind`: DA object type identifiers (Code, StorageShard, Receipt, Block)
//!
//! # Key Functions
//!
//! - `ContractStorage::resolve_shard_id()`: Maps storage key → ShardId using contract SSR
//! - `ContractSSR::serialize()` / `ContractSSR::deserialize()`: Contract SSR serialization
//! - `ContractShard::serialize()` / `ContractShard::deserialize()`: Contract shard serialization
//! - `code_object_id()`, `contract_ssr_object_id()`: Construct 32-byte object IDs for DA lookup
//! - `format_object_id()`, `format_data_hex()`: Logging utilities

use alloc::format;
use alloc::string::String;
use alloc::vec::Vec;
use primitive_types::{H160, H256};

// Import generic SSR types and functions from da module
use crate::da::{self};


// Re-export shared types from da module (used by BOTH contract and meta-sharding)
pub use da::{ShardId};
/// EVM Entry - key-value pair in a contract shard (64 bytes)
///
/// **USED BY**: Contract sharding ONLY
///
/// Stores EVM storage key-value pairs in contract shards.
#[derive(Clone, PartialEq, Eq)]
pub struct EvmEntry {
    pub key_h: H256, // keccak256(key) - 32 bytes
    pub value: H256, // EVM word - 32 bytes
}

impl PartialOrd for EvmEntry {
    fn partial_cmp(&self, other: &Self) -> Option<core::cmp::Ordering> {
        Some(self.cmp(other))
    }
}

impl Ord for EvmEntry {
    fn cmp(&self, other: &Self) -> core::cmp::Ordering {
        self.key_h.cmp(&other.key_h)
    }
}



/// Contract Shard - contains EVM storage entries for a specific contract
///
/// **USED BY**: Contract sharding ONLY
/// Post-SSR: Single flat shard per contract, no merkle root needed
#[derive(Clone)]
pub struct ContractShard {
    pub entries: Vec<EvmEntry>, // Sorted by key_hash
}

impl ContractShard {
 
    /// Create a new empty contract shard
    pub fn new() -> Self {
        Self {
            entries: Vec::new(),
        }
    }

    /// Serialize shard data to DA format (sorted entries)
    ///
    /// Post-SSR Format: [2B count][EvmEntry...]
    /// Each EvmEntry: [32B key_h][32B value] (64 bytes total)
    ///
    /// # Panics
    /// - In debug builds: if shard exceeds MAX_ENTRIES
    pub fn serialize(&self) -> Vec<u8> {

        let mut result = Vec::new();

        // Entry count (2 bytes)
        let count = self.entries.len() as u16;
        result.extend_from_slice(&count.to_le_bytes());

        // Entries (64 bytes each: 32B key_h + 32B value)
        for entry in &self.entries {
            result.extend_from_slice(entry.key_h.as_bytes());
            result.extend_from_slice(entry.value.as_bytes());
        }

        result
    }

    /// Deserialize shard data from DA format (post-SSR)
    pub fn deserialize(data: &[u8]) -> Option<Self> {
        if data.len() < 2 {
            // Minimum: 2 bytes for count
            return None;
        }

        let count = u16::from_le_bytes([data[0], data[1]]) as usize;
        let entry_size = 64; // 32 bytes key_h + 32 bytes value
        let expected_len = 2 + (count * entry_size);

        if data.len() < expected_len {
            return None;
        }

        let mut entries = Vec::new();
        let mut offset = 2;
        for _ in 0..count {
            let key_h = H256::from_slice(&data[offset..offset + 32]);
            let value = H256::from_slice(&data[offset + 32..offset + 64]);
            entries.push(EvmEntry { key_h, value });
            offset += entry_size;
        }

        Some(ContractShard {
            entries,
        })
    }
}

/// Contract Storage - Single shard for a contract (post witness transition)
///
/// **USED BY**: Contract sharding ONLY
///
/// After witness transition, each contract has a single flat shard with all storage.
/// SSR metadata and adaptive sharding have been removed.
pub struct ContractStorage {
    pub shard: ContractShard, // Single shard containing all contract storage
}

impl ContractStorage {
    pub fn new(_owner: H160) -> Self {
        Self {
            shard: ContractShard::new(),
        }
    }

}

// ===== ObjectKind =====

/// Identifies the type of DA object for an address
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[repr(u8)]
pub enum ObjectKind {
    /// Code object - bytecode storage (kind=0x00)
    Code = 0x00,
    /// Storage shard object (kind=0x01)
    StorageShard = 0x01,
    /// Balance object - balance updates for Verkle BasicData (kind=0x02)
    Balance = 0x02,

    /// Receipt object (kind=0x03) - contains full transaction data
    Receipt = 0x03,
    /// Meta-shard object (kind=0x04) - ObjectID→ObjectRef mappings
    MetaShard = 0x04,
    /// Block object (kind=0x05)
    Block = 0x05,
    /// Nonce object - nonce updates for Verkle BasicData (kind=0x06)
    Nonce = 0x06,
}

impl ObjectKind {
    /// Try to parse from byte value
    pub fn from_u8(value: u8) -> Option<Self> {
        match value {
            0x00 => Some(Self::Code),
            0x01 => Some(Self::StorageShard),
            0x02 => Some(Self::Balance),
            0x03 => Some(Self::Receipt),
            0x04 => Some(Self::MetaShard),
            0x05 => Some(Self::Block),
            0x06 => Some(Self::Nonce),
            _ => None,
        }
    }
}

// ===== ObjectID Construction =====

/// Format ObjectID as a hex string for logging
pub fn format_object_id(oid: &[u8; 32]) -> String {
    let mut result = String::from("0x");
    for byte in oid.iter() {
        result.push_str(&format!("{:02x}", byte));
    }
    result
}

/// Format data compactly as hex for logging
pub fn format_data_hex(data: &[u8]) -> String {
    if data.len() <= 8 {
        // For short data, show everything
        let mut result = String::from("0x");
        for byte in data.iter() {
            result.push_str(&format!("{:02x}", byte));
        }
        result
    } else {
        // For long data, show first 4 bytes and last 4 bytes
        let mut result = String::from("0x");
        for byte in data[..4].iter() {
            result.push_str(&format!("{:02x}", byte));
        }
        result.push_str("...");
        for byte in data[data.len() - 4..].iter() {
            result.push_str(&format!("{:02x}", byte));
        }
        result
    }
}

/// Construct code object ID: [20B address][11B zero][1B kind=0x00]
pub fn code_object_id(address: H160) -> [u8; 32] {
    let mut object_id = [0u8; 32];
    object_id[0..20].copy_from_slice(address.as_bytes());
    object_id[31] = ObjectKind::Code as u8;
    object_id
}

/// Construct contract storage shard object ID (post-SSR: always root shard)
/// Format: [20B address][1B 0xFF][1B ld=0][7B zeros][2B zero][1B kind=0x01]
pub fn contract_shard_object_id(address: H160) -> [u8; 32] {
    let mut object_id = [0u8; 32];
    object_id[0..20].copy_from_slice(address.as_bytes());
    object_id[20] = 0xFF;
    // Post-SSR: ld always 0, prefix56 always zeros
    object_id[21] = 0; // ld = 0
    // Bytes 22..31 stay zero
    object_id[31] = ObjectKind::StorageShard as u8;
    object_id
}

// All SSR-related tests removed - SSR has been eliminated post-witness transition


/*
/// Dump all entries in a storage shard for debugging
pub fn dump_entries(shard_data: &ContractShard) {
    use utils::functions::log_info;

    for (idx, entry) in shard_data.entries.iter().enumerate() {
        log_info(&format!(
            "Entry #{}/{}: key_h={}, value={}",
            idx,
            shard_data.entries.len(),
            format_object_id(&entry.key_h.0),
            format_object_id(&entry.value.0)
        ));
    }
}

 */