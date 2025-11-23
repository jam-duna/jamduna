//! SSR (Sparse Split Registry) - Sharded Storage and DA Objects for EVM
//!
//! # Overview
//!
//! This module implements adaptive storage sharding using a sparse split registry (SSR)
//! and provides DA object identification for all EVM artifacts (code, shards, receipts, blocks).
//!
//! This module uses the generic SSR implementation from `da.rs` with EVM-specific entry types.
//!
//! # Architecture
//!
//! - **SSR (Sparse Split Registry)**: Metadata tracking split points and shard depths (from da.rs)
//! - **Shards**: 4KB max storage buckets containing key-value pairs
//! - **Adaptive Splitting**: Shards split when exceeding 34 entries (~50% fill rate)
//! - **Object-based Storage**: Each shard is a separate DA object for parallel access
//!
//! # Data Structures
//!
//! - `EvmEntry`: EVM key-value pair (implements da::SSREntry trait)
//! - `ContractSSR`: Type alias for SSRData<EvmEntry>
//! - `ContractShard`: Type alias for ShardData<EvmEntry>
//! - `ContractStorage`: Complete storage for one contract (SSR + shards map)
//! - `ObjectKind`: DA object type identifiers (Code, StorageShard, SsrMetadata, Receipt, Block)
//!
//! # Key Functions
//!
//! - `resolve_shard_id()`: Maps storage key → ShardId using SSR (uses da::resolve_shard_id)
//! - `maybe_split_shard_recursive()`: Splits oversized shards (uses da::maybe_split_shard_recursive)
//! - `serialize_ssr()` / `deserialize_ssr()`: DA format conversion (uses da::serialize_ssr/deserialize_ssr)
//! - `serialize_shard()` / `deserialize_shard()`: DA format conversion (uses da::serialize_shard/deserialize_shard)
//! - `code_object_id()`, `ssr_object_id()`, `shard_object_id()`: Construct 32-byte object IDs for DA lookup
//! - `format_object_id()`, `format_data_hex()`: Logging utilities

use alloc::collections::BTreeMap;
use alloc::format;
use alloc::string::String;
use alloc::vec::Vec;
use primitive_types::{H160, H256};

// Import generic SSR types and functions from da module
use crate::da::{self, SerializeError};

/// Read a specific bit from a 256-bit hash, indexed from the most significant bit
#[cfg(test)]
pub fn get_bit_at(hash: &H256, index: usize) -> bool {
    let bytes = hash.as_bytes();
    if index / 8 >= bytes.len() {
        return false;
    }
    let byte = bytes[index / 8];
    let shift = 7 - (index % 8);
    (byte >> shift) & 1 == 1
}

// Re-export generic types from da module
pub use da::{ShardId, SSRData, SSRHeader, SSREntryMeta as SSREntry, ShardData};

/// EVM Entry - key-value pair in a shard (64 bytes)
///
/// Implements SSREntry trait from da module for generic SSR sharding.
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

impl da::SSREntry for EvmEntry {
    fn key_hash(&self) -> [u8; 32] {
        *self.key_h.as_fixed_bytes()
    }

    fn serialized_size() -> usize {
        64 // 32 bytes key_h + 32 bytes value
    }

    fn serialize(&self) -> Vec<u8> {
        let mut buf = Vec::with_capacity(64);
        buf.extend_from_slice(self.key_h.as_bytes());
        buf.extend_from_slice(self.value.as_bytes());
        buf
    }

    fn deserialize(data: &[u8]) -> Result<Self, SerializeError> {
        if data.len() != 64 {
            return Err(SerializeError::InvalidSize);
        }
        Ok(EvmEntry {
            key_h: H256::from_slice(&data[0..32]),
            value: H256::from_slice(&data[32..64]),
        })
    }
}

/// Type alias for contract-specific SSR (using EvmEntry)
pub type ContractSSR = SSRData<EvmEntry>;

/// Type alias for contract-specific shard (using EvmEntry)
pub type ContractShard = da::ShardData<EvmEntry>;

/// Contract Storage - SSR + shards for a contract
pub struct ContractStorage {
    pub ssr: ContractSSR,                         // Sparse Split Registry (generic)
    pub shards: BTreeMap<ShardId, ContractShard>, // ShardId -> KV entries
}

impl ContractStorage {
    pub fn new(_owner: H160) -> Self {
        Self {
            ssr: SSRData::new(),
            shards: BTreeMap::new(),
        }
    }
}

// ===== Shard Resolution & Splitting =====

/// Resolve which shard a storage key belongs to using SSR
///
/// Wrapper around da::resolve_shard_id() for EVM-specific contract storage.
///
/// # Algorithm
/// 1. Start with global_depth from SSR header
/// 2. Extract 56-bit prefix from key
/// 3. Search SSR entries for matching prefix exception
/// 4. Return ShardId with resolved depth and masked prefix
///
/// # Returns
/// ShardId with (ld, prefix56) identifying the target shard
pub fn resolve_shard_id(contract_storage: &ContractStorage, key: H256) -> ShardId {
    let key_hash = *key.as_fixed_bytes();
    da::resolve_shard_id(key_hash, &contract_storage.ssr)
}

/// Split threshold - shards split when exceeding this many entries
pub const SPLIT_THRESHOLD: usize = 34;

/// Maximum entries per shard (hard DA segment limit)
pub const MAX_ENTRIES: usize = 63;

/// Recursively split a shard until all result shards meet threshold
///
/// Wrapper around da::maybe_split_shard_recursive() for EVM-specific contract storage.
///
/// # Algorithm
/// 1. Use work queue to process shards that need splitting
/// 2. For each shard > SPLIT_THRESHOLD, perform binary split
/// 3. Check if children still exceed threshold and queue them for further splitting
/// 4. Continue until all result shards ≤ SPLIT_THRESHOLD
///
/// # Returns
/// Some((leaf_shards, ssr_entries)) where:
/// - leaf_shards: Vec of (ShardId, ContractShard) for all result leaves
/// - ssr_entries: Vec of SSREntry for internal split points (N leaves → N-1 entries)
pub fn maybe_split_shard_recursive(
    shard_id: ShardId,
    shard_data: &ContractShard,
) -> Option<(Vec<(ShardId, ContractShard)>, Vec<SSREntry>)> {
    da::maybe_split_shard_recursive(shard_id, shard_data.clone(), SPLIT_THRESHOLD)
}

// ===== Serialization Functions =====

/// Serialize SSR metadata to DA format (10B header + 9B entries)
///
/// Wrapper around da::serialize_ssr() for EVM-specific contract storage.
///
/// Format: [1B global_depth][1B entry_count][4B total_keys][4B version][SSREntry...]
/// Each SSREntry: [1B d][1B ld][7B prefix56]
pub fn serialize_ssr(ssr: &ContractSSR) -> Vec<u8> {
    da::serialize_ssr(ssr)
}

/// Deserialize SSR data from DA format
///
/// Wrapper around da::deserialize_ssr() for EVM-specific contract storage.
/// Used for loading SSR objects from DA during state reconstruction.
pub fn deserialize_ssr(data: &[u8]) -> Option<ContractSSR> {
    da::deserialize_ssr(data).ok()
}

/// Create SSRData from header and entries
///
/// Helper to construct SSRData when you have separate header and entries.
pub fn ssr_from_parts(header: SSRHeader, entries: Vec<SSREntry>) -> ContractSSR {
    SSRData::from_parts(header, entries)
}

/// Serialize shard data to DA format (sorted entries)
///
/// Wrapper around da::serialize_shard() for EVM-specific contract storage.
///
/// Format: [2B count][EvmEntry...]
/// Each EvmEntry: [32B key_h][32B value] (64 bytes total)
pub fn serialize_shard(shard: &ContractShard) -> Vec<u8> {
    da::serialize_shard(shard, MAX_ENTRIES)
}

/// Deserialize shard data from DA format
///
/// Wrapper around da::deserialize_shard() for EVM-specific contract storage.
pub fn deserialize_shard(data: &[u8]) -> Option<ContractShard> {
    da::deserialize_shard(data).ok()
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
    /// SSR metadata object (kind=0x02)
    SsrMetadata = 0x02,
    /// Receipt object (kind=0x03) - contains full transaction data
    Receipt = 0x03,
    /// Meta-shard object (kind=0x04) - ObjectID→ObjectRef mappings
    MetaShard = 0x04,
    /// Block object (kind=0x05)
    Block = 0x05,
    
    /// Meta-SSR metadata object (kind=0x07) - routing metadata for meta-shards
    MetaSsrMetadata = 0x07,
}

impl ObjectKind {
    /// Try to parse from byte value
    pub fn from_u8(value: u8) -> Option<Self> {
        match value {
            0x00 => Some(Self::Code),
            0x01 => Some(Self::StorageShard),
            0x02 => Some(Self::SsrMetadata),
            0x03 => Some(Self::Receipt),
            0x04 => Some(Self::MetaShard),
            0x05 => Some(Self::Block),
            0x07 => Some(Self::MetaSsrMetadata),
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

/// Construct SSR object ID: [20B address][11B zero][1B kind=0x02]
pub fn ssr_object_id(address: H160) -> [u8; 32] {
    let mut object_id = [0u8; 32];
    object_id[0..20].copy_from_slice(address.as_bytes());
    object_id[31] = ObjectKind::SsrMetadata as u8;
    object_id
}

/// Construct storage shard object ID: [20B address][1B 0xFF][1B ld][7B prefix56][2B zero][1B kind=0x01]
pub fn shard_object_id(address: H160, shard_id: ShardId) -> [u8; 32] {
    let mut object_id = [0u8; 32];
    object_id[0..20].copy_from_slice(address.as_bytes());
    object_id[20] = 0xFF;
    object_id[21] = shard_id.ld;
    object_id[22..29].copy_from_slice(&shard_id.prefix56);
    // Bytes 29..31 stay zero for future use
    object_id[31] = ObjectKind::StorageShard as u8;
    object_id
}

#[cfg(test)]
mod tests {
    use super::*;
    use alloc::vec;

    #[test]
    fn test_ssr_serialization_roundtrip() {
        let ssr = SSRData {
            header: SSRHeader {
                global_depth: 2,
                entry_count: 1,
                total_keys: 100,
                version: 5,
            },
            entries: vec![SSREntry {
                d: 2,
                ld: 4,
                prefix56: [0x12, 0x34, 0x56, 0x78, 0x9A, 0xBC, 0xDE],
            }],
        };

        let serialized = serialize_ssr(&ssr);
        let deserialized = deserialize_ssr(&serialized).expect("Failed to deserialize SSR");

        assert_eq!(deserialized.header.global_depth, ssr.header.global_depth);
        assert_eq!(deserialized.header.entry_count, ssr.header.entry_count);
        assert_eq!(deserialized.header.total_keys, ssr.header.total_keys);
        assert_eq!(deserialized.header.version, ssr.header.version);
        assert_eq!(deserialized.entries.len(), ssr.entries.len());
        assert_eq!(deserialized.entries[0].d, ssr.entries[0].d);
        assert_eq!(deserialized.entries[0].ld, ssr.entries[0].ld);
        assert_eq!(deserialized.entries[0].prefix56, ssr.entries[0].prefix56);
    }

    #[test]
    fn test_shard_serialization_roundtrip() {
        let shard = ShardData {
            entries: vec![
                EvmEntry {
                    key_h: crate::state::h256_from_low_u64_be(1),
                    value: crate::state::h256_from_low_u64_be(100),
                },
                EvmEntry {
                    key_h: crate::state::h256_from_low_u64_be(2),
                    value: crate::state::h256_from_low_u64_be(200),
                },
            ],
        };

        let serialized = serialize_shard(&shard);
        let deserialized = deserialize_shard(&serialized).expect("Failed to deserialize shard");

        assert_eq!(deserialized.entries.len(), shard.entries.len());
        assert_eq!(deserialized.entries[0].key_h, shard.entries[0].key_h);
        assert_eq!(deserialized.entries[0].value, shard.entries[0].value);
        assert_eq!(deserialized.entries[1].key_h, shard.entries[1].key_h);
        assert_eq!(deserialized.entries[1].value, shard.entries[1].value);
    }

    #[test]
    fn test_bit_operations() {
        let hash = H256::from_slice(&[
            0b10110011, 0b01010101, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
            0, 0, 0, 0, 0, 0, 0, 0, 0,
        ]);

        assert_eq!(get_bit_at(&hash, 0), true);
        assert_eq!(get_bit_at(&hash, 1), false);
        assert_eq!(get_bit_at(&hash, 2), true);
        assert_eq!(get_bit_at(&hash, 3), true);
        assert_eq!(get_bit_at(&hash, 4), false);
        assert_eq!(get_bit_at(&hash, 5), false);
        assert_eq!(get_bit_at(&hash, 6), true);
        assert_eq!(get_bit_at(&hash, 7), true);
        assert_eq!(get_bit_at(&hash, 8), false);
        assert_eq!(get_bit_at(&hash, 9), true);
    }

    #[test]
    fn test_resolve_shard_id_empty_ssr() {
        let contract_storage = ContractStorage::new(H160::zero());
        let key = crate::state::h256_from_low_u64_be(0x1234);

        let shard_id = resolve_shard_id(&contract_storage, key);

        assert_eq!(shard_id.ld, 0);
        assert_eq!(shard_id.prefix56, [0u8; 7]);
    }

    #[test]
    fn test_resolve_shard_id_recursive_path() {
        let mut contract_storage = ContractStorage::new(H160::zero());

        // Simulate two recursive splits on the left branch (bit pattern 00...)
        contract_storage.ssr.entries = vec![
            SSREntry {
                d: 0,
                ld: 1,
                prefix56: [0u8; 7],
            },
            SSREntry {
                d: 1,
                ld: 2,
                prefix56: [0u8; 7],
            },
        ];
        contract_storage.ssr.header.entry_count = contract_storage.ssr.entries.len() as u32;

        let key_left = H256::zero();
        let shard_left = resolve_shard_id(&contract_storage, key_left);
        assert_eq!(shard_left.ld, 2, "Should descend through both SSR entries");
        assert_eq!(
            shard_left.prefix56,
            mask_prefix56(take_prefix56(&key_left), shard_left.ld)
        );

        // Key with MSB = 1 should only match the first split (d=0 → ld=1)
        let mut key_right_bytes = [0u8; 32];
        key_right_bytes[0] = 0x80; // Set highest bit
        let key_right = H256::from(key_right_bytes);
        let shard_right = resolve_shard_id(&contract_storage, key_right);
        assert_eq!(shard_right.ld, 1, "Only the root split should apply to this key");
        assert_eq!(
            shard_right.prefix56,
            mask_prefix56(take_prefix56(&key_right), shard_right.ld)
        );
    }

    #[test]
    fn test_shard_object_id_construction() {
        let contract = crate::state::h160_from_low_u64_be(0x42);
        let shard_id = ShardId {
            ld: 5,
            prefix56: [0x12, 0x34, 0x56, 0x78, 0x9A, 0xBC, 0xDE],
        };

        let object_id = shard_object_id(contract, shard_id);

        // Verify format: [20B address][0xFF][ld][7B prefix56][2B zero][kind]
        assert_eq!(&object_id[0..20], contract.as_bytes());
        assert_eq!(object_id[20], 0xFF); // marker
        assert_eq!(object_id[21], 5); // ld
        assert_eq!(
            &object_id[22..29],
            &[0x12, 0x34, 0x56, 0x78, 0x9A, 0xBC, 0xDE]
        ); // prefix56
        assert_eq!(&object_id[29..31], &[0, 0]); // padding
        assert_eq!(object_id[31], 0x01); // kind
    }

    #[test]
    fn test_maybe_split_shard_below_threshold() {
        let shard_id = ShardId {
            ld: 2,
            prefix56: [0; 7],
        };

        // Create shard with 20 entries (below 34 threshold)
        let mut entries = Vec::new();
        for i in 0..20 {
            entries.push(EvmEntry {
                key_h: crate::state::h256_from_low_u64_be(i),
                value: crate::state::h256_from_low_u64_be(i * 100),
            });
        }
        let shard_data = ShardData { entries };

        // Should not split
        let result = maybe_split_shard_recursive(shard_id, &shard_data);
        assert!(result.is_none());
    }

    #[test]
    fn test_maybe_split_shard_above_threshold() {
        let shard_id = ShardId {
            ld: 2,
            prefix56: [0; 7],
        };

        // Create shard with 40 entries (above 34 threshold)
        let mut entries = Vec::new();
        for i in 0..40 {
            entries.push(EvmEntry {
                key_h: crate::state::h256_from_low_u64_be(i),
                value: crate::state::h256_from_low_u64_be(i * 100),
            });
        }
        let shard_data = ShardData { entries };

        // Should split
        let result = maybe_split_shard_recursive(shard_id, &shard_data);
        assert!(result.is_some());

        let (shards, ssr_entries) = result.unwrap();

        // Check that all entries are distributed
        let total_entries: usize = shards.iter().map(|(_, sd)| sd.entries.len()).sum();
        assert_eq!(total_entries, 40);

        // Check SSR entries exist
        assert!(!ssr_entries.is_empty());
    }

    #[test]
    fn test_split_threshold_constant() {
        assert_eq!(SPLIT_THRESHOLD, 34);
    }

    #[test]
    fn test_split_256_entries() {
        // Create shard with 256 deterministic entries with varying prefixes
        let mut entries = Vec::new();
        for i in 0..256 {
            // Spread the counter across multiple bytes to ensure prefix variation
            // Put i in the first byte to maximize prefix diversity for first 256 entries
            let mut key_bytes = [0u8; 32];
            key_bytes[0] = i as u8;  // Byte 0 gets full range 0-255
            key_bytes[1] = (i >> 8) as u8;  // Higher bytes for larger numbers
            entries.push(EvmEntry {
                key_h: H256::from(key_bytes),
                value: crate::state::h256_from_low_u64_be(i * 100),
            });
        }

        let shard_data = ShardData { entries };
        let shard_id = ShardId {
            ld: 0,
            prefix56: [0; 7],
        };

        // Should produce 7-8 shards, all with ≤34 entries
        let (leaf_shards, ssr_entries) = maybe_split_shard_recursive(shard_id, &shard_data).unwrap();

        assert!(
            leaf_shards.len() >= 7,
            "Should create at least 7 shards for 256 entries, got {} shards with sizes: {:?}",
            leaf_shards.len(),
            leaf_shards.iter().map(|(_, s)| s.entries.len()).collect::<Vec<_>>()
        );

        // Verify binary tree invariant: N leaves → N-1 internal nodes
        assert_eq!(
            ssr_entries.len(),
            leaf_shards.len() - 1,
            "SSR entries should be N-1 for N leaves"
        );

        for (i, (_, shard)) in leaf_shards.iter().enumerate() {
            assert!(
                shard.entries.len() <= SPLIT_THRESHOLD,
                "Each result shard should have ≤{} entries, shard {} has {}. All shards: {:?}",
                SPLIT_THRESHOLD,
                i,
                shard.entries.len(),
                leaf_shards.iter().map(|(_, s)| s.entries.len()).collect::<Vec<_>>()
            );
            assert!(
                shard.entries.len() <= 63,
                "Each result shard MUST have ≤63 entries for DA segment limit, found {}",
                shard.entries.len()
            );
        }

        // Verify all entries preserved
        let total_entries: usize = leaf_shards.iter().map(|(_, s)| s.entries.len()).sum();
        assert_eq!(
            total_entries, 256,
            "All entries must be preserved after split"
        );
    }

    #[test]
    fn test_worst_case_biased_distribution() {
        // Worst case: craft entries whose prefixes all start with 0b0...
        // So the first split produces a heavily skewed child
        let mut entries = Vec::new();
        for i in 0u64..129 {
            // Create keys with varying prefixes (spread counter to ensure diversity)
            let mut key_bytes = [0u8; 32];
            key_bytes[0] = i as u8;
            key_bytes[1] = (i >> 8) as u8;
            // Ensure MSB is 0 (bit 0 of byte 0) to create initial bias
            key_bytes[0] &= 0x7F;
            entries.push(EvmEntry {
                key_h: H256::from(key_bytes),
                value: crate::state::h256_from_low_u64_be(i * 100),
            });
        }

        let shard_data = ShardData { entries };
        let shard_id = ShardId {
            ld: 0,
            prefix56: [0; 7],
        };

        // Should recursively split until all shards ≤34 entries
        let (leaf_shards, ssr_entries) = maybe_split_shard_recursive(shard_id, &shard_data).unwrap();

        // Verify binary tree invariant: N leaves → N-1 internal nodes
        assert_eq!(
            ssr_entries.len(),
            leaf_shards.len() - 1,
            "SSR entries should be N-1 for N leaves"
        );

        for (_, shard) in &leaf_shards {
            assert!(
                shard.entries.len() <= SPLIT_THRESHOLD,
                "Biased distribution should still be split to ≤{} entries, found {}",
                SPLIT_THRESHOLD,
                shard.entries.len()
            );
            assert!(
                shard.entries.len() <= 63,
                "Each result shard MUST have ≤63 entries for DA segment limit, found {}",
                shard.entries.len()
            );
        }

        // Verify all entries preserved
        let total_entries: usize = leaf_shards.iter().map(|(_, s)| s.entries.len()).sum();
        assert_eq!(
            total_entries, 129,
            "All entries must be preserved after split"
        );
    }
}

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
