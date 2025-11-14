//! SSR (Sparse Split Registry) - Sharded Storage and DA Objects for EVM
//!
//! # Overview
//!
//! This module implements adaptive storage sharding using a sparse split registry (SSR)
//! and provides DA object identification for all EVM artifacts (code, shards, receipts, blocks).
//!
//! # Architecture
//!
//! - **SSR (Sparse Split Registry)**: Metadata tracking split points and shard depths
//! - **Shards**: 4KB max storage buckets containing key-value pairs
//! - **Adaptive Splitting**: Shards split when exceeding 34 entries (~50% fill rate)
//! - **Object-based Storage**: Each shard is a separate DA object for parallel access
//!
//! # Data Structures
//!
//! - `SSRHeader`: Global metadata (8 bytes: depth, entry count, total keys, version)
//! - `SSREntry`: Split exception (9 bytes: d, ld, prefix56)
//! - `ShardId`: Shard identifier (ld + 56-bit prefix)
//! - `ShardData`: Key-value entries within a shard (max 63 entries)
//! - `ContractStorage`: Complete storage for one contract (SSR + shards map)
//! - `ObjectKind`: DA object type identifiers (Code, StorageShard, SsrMetadata, Receipt, Block)
//!
//! # Key Functions
//!
//! - `resolve_shard_id()`: Maps storage key → ShardId using SSR
//! - `maybe_split_shard()`: Splits oversized shards and updates SSR
//! - `serialize_ssr()` / `deserialize_ssr()`: DA format conversion
//! - `serialize_shard()` / `deserialize_shard()`: DA format conversion
//! - `code_object_id()`, `ssr_object_id()`, `shard_object_id()`: Construct 32-byte object IDs for DA lookup
//! - `format_object_id()`, `format_data_hex()`: Logging utilities

use alloc::collections::BTreeMap;
use alloc::format;
use alloc::string::String;
use alloc::vec;
use alloc::vec::Vec;
use primitive_types::{H160, H256};
use utils::functions::log_info;

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

/// SSR Header - contract storage metadata (8 bytes)
pub struct SSRHeader {
    pub global_depth: u8, // Default depth for all shards
    pub entry_count: u8,  // Number of exceptions
    pub total_keys: u32,  // Total storage entries
    pub version: u32,
}

/// SSR Entry - tracks a split exception (9 bytes)
#[derive(Clone, Copy)]
pub struct SSREntry {
    pub d: u8,             // Split point (prefix bit position)
    pub ld: u8,            // Local depth at this prefix
    pub prefix56: [u8; 7], // 56-bit routing prefix
}

/// SSR Data - sparse split registry for a contract
pub struct SSRData {
    pub header: SSRHeader,
    pub entries: Vec<SSREntry>, // Sorted exceptions
}

/// Shard ID - identifies a storage shard
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct ShardId {
    pub ld: u8,            // Local depth
    pub prefix56: [u8; 7], // 56-bit prefix
}

impl PartialOrd for ShardId {
    fn partial_cmp(&self, other: &Self) -> Option<core::cmp::Ordering> {
        Some(self.cmp(other))
    }
}

impl Ord for ShardId {
    fn cmp(&self, other: &Self) -> core::cmp::Ordering {
        self.ld
            .cmp(&other.ld)
            .then_with(|| self.prefix56.cmp(&other.prefix56))
    }
}

/// EVM Entry - key-value pair in a shard
#[derive(Clone)]
pub struct EvmEntry {
    pub key_h: H256, // keccak256(key)
    pub value: H256,
}

/// Shard Data - storage entries in a single shard
#[derive(Clone)]
pub struct ShardData {
    pub entries: Vec<EvmEntry>, // Sorted by key_h, max 63 entries
}

/// Contract Storage - SSR + shards for a contract
pub struct ContractStorage {
    pub ssr: SSRData,                         // Sparse Split Registry
    pub shards: BTreeMap<ShardId, ShardData>, // ShardId -> KV entries
}

impl ContractStorage {
    pub fn new(_owner: H160) -> Self {
        Self {
            ssr: SSRData {
                header: SSRHeader {
                    global_depth: 0,
                    entry_count: 0,
                    total_keys: 0,
                    version: 1,
                },
                entries: Vec::new(),
            },
            shards: BTreeMap::new(),
        }
    }
}

// ===== Prefix Manipulation Utilities =====

/// Extract first 56 bits (7 bytes) from hash as routing prefix
fn take_prefix56(key: &H256) -> [u8; 7] {
    let mut prefix = [0u8; 7];
    prefix.copy_from_slice(&key.as_bytes()[0..7]);
    prefix
}

/// Check if key_prefix matches entry_prefix for first 'bits' bits
fn matches_prefix(key_prefix: [u8; 7], entry_prefix: [u8; 7], bits: u8) -> bool {
    // Compare first 'bits' bits of both prefixes
    let bytes_to_compare = (bits / 8) as usize;
    let remaining_bits = bits % 8;

    // Check full bytes
    if key_prefix[0..bytes_to_compare] != entry_prefix[0..bytes_to_compare] {
        return false;
    }

    // Check remaining bits
    if remaining_bits > 0 {
        let mask = 0xFF << (8 - remaining_bits);
        if (key_prefix[bytes_to_compare] & mask) != (entry_prefix[bytes_to_compare] & mask) {
            return false;
        }
    }

    true
}

/// Mask prefix to first 'bits' bits, zero out the rest
fn mask_prefix56(prefix: [u8; 7], bits: u8) -> [u8; 7] {
    use utils::functions::log_error;

    // Validate bits doesn't exceed 56 (7 bytes * 8 bits)
    if bits > 56 {
        log_error(&format!(
            "⚠️  mask_prefix56: bits={} exceeds maximum 56! Clamping to 56.",
            bits
        ));
        // Return full prefix (all 56 bits) when bits exceeds maximum
        let mut masked = [0u8; 7];
        masked.copy_from_slice(&prefix);
        return masked;
    }

    let mut masked = [0u8; 7];
    let full_bytes = (bits / 8) as usize;
    let remaining_bits = bits % 8;

    //Copy full bytes
    masked[0..full_bytes].copy_from_slice(&prefix[0..full_bytes]);

    // Mask remaining bits
    if remaining_bits > 0 && full_bytes < 7 {
        let mask = 0xFF << (8 - remaining_bits);
        masked[full_bytes] = prefix[full_bytes] & mask;
    }

    masked
}

// ===== Shard Resolution & Splitting =====

/// Resolve which shard a storage key belongs to using SSR
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
    let key_prefix56 = take_prefix56(&key);

    // Start at the global depth (root shard) and walk down the split tree.
    let mut current_ld = contract_storage.ssr.header.global_depth;
    let mut current_prefix = mask_prefix56(key_prefix56, current_ld);

    loop {
        let mut found_split = false;

        for entry in &contract_storage.ssr.entries {
            if entry.d == current_ld && matches_prefix(key_prefix56, entry.prefix56, entry.d) {
                // Descend one level deeper based on this split exception.
                current_ld = entry.ld;
                current_prefix = mask_prefix56(key_prefix56, current_ld);
                found_split = true;
                break;
            }
        }

        if !found_split {
            break;
        }
    }

    ShardId {
        ld: current_ld,
        prefix56: current_prefix,
    }
}

/// Split threshold - shards split when exceeding this many entries
pub const SPLIT_THRESHOLD: usize = 34;

/// Perform a single binary split of a shard
///
/// Helper function that splits a shard into left and right children based on one bit.
/// Used by recursive splitting logic.
///
/// # Returns
/// (left_shard_id, left_shard, right_shard_id, right_shard, ssr_entry)
fn split_once(
    shard_id: ShardId,
    shard_data: &ShardData,
) -> (ShardId, ShardData, ShardId, ShardData, SSREntry) {
    assert!(shard_id.ld < 255, "Cannot split shard at maximum depth");
    let new_depth = shard_id.ld + 1;
    let split_bit = shard_id.ld as usize;

    // Split entries based on the next bit position
    let mut left_entries = Vec::new();
    let mut right_entries = Vec::new();

    for entry in &shard_data.entries {
        let key_prefix = take_prefix56(&entry.key_h);

        if split_bit < 56 {
            let byte_idx = split_bit / 8;
            let bit_idx = split_bit % 8;
            let bit_mask = 0x80 >> bit_idx;

            if byte_idx < 7 && (key_prefix[byte_idx] & bit_mask) == 0 {
                left_entries.push(entry.clone());
            } else {
                right_entries.push(entry.clone());
            }
        } else {
            // If depth exceeds 56 bits, put all in left shard
            left_entries.push(entry.clone());
        }
    }

    let left_shard = ShardData {
        entries: left_entries,
    };
    let right_shard = ShardData {
        entries: right_entries,
    };

    // Left shard keeps the same prefix (bit=0)
    let left_shard_id = ShardId {
        ld: new_depth,
        prefix56: shard_id.prefix56,
    };

    // Right shard has the split bit set to 1
    let mut right_prefix = shard_id.prefix56;
    if split_bit < 56 {
        let byte_idx = split_bit / 8;
        let bit_idx = split_bit % 8;
        if byte_idx < 7 {
            right_prefix[byte_idx] |= 0x80 >> bit_idx;
        }
    }
    let right_shard_id = ShardId {
        ld: new_depth,
        prefix56: right_prefix,
    };

    // Create SSR entry for the split point
    let ssr_entry = SSREntry {
        d: shard_id.ld,          // Split point (previous depth)
        ld: new_depth,           // New local depth
        prefix56: shard_id.prefix56, // Original routing prefix
    };

    (left_shard_id, left_shard, right_shard_id, right_shard, ssr_entry)
}

/// Recursively split a shard until all result shards meet threshold
///
/// # Algorithm
/// 1. Use work queue to process shards that need splitting
/// 2. For each shard > SPLIT_THRESHOLD, perform binary split
/// 3. Check if children still exceed threshold and queue them for further splitting
/// 4. Continue until all result shards ≤ SPLIT_THRESHOLD
///
/// # Returns
/// Some((leaf_shards, ssr_entries)) where:
/// - leaf_shards: Vec of (ShardId, ShardData) for all result leaves
/// - ssr_entries: Vec of SSREntry for internal split points (N leaves → N-1 entries)
pub fn maybe_split_shard_recursive(
    shard_id: ShardId,
    shard_data: &ShardData,
) -> Option<(Vec<(ShardId, ShardData)>, Vec<SSREntry>)> {
    use utils::functions::log_info;

    if shard_data.entries.len() <= SPLIT_THRESHOLD {
        return None;
    }

    log_info(&format!(
        "🔀 Recursive split starting: {} entries in shard ld={}",
        shard_data.entries.len(),
        shard_id.ld
    ));

    let mut leaf_shards = Vec::new();
    let mut work_queue = vec![(shard_id, shard_data.clone())];
    let mut ssr_entries = Vec::new();

    while let Some((current_id, current_data)) = work_queue.pop() {
        if current_data.entries.len() <= SPLIT_THRESHOLD {
            // Base case: shard is acceptable size, add to leaves
            leaf_shards.push((current_id, current_data));
            continue;
        }

        // Check if we've reached maximum depth
        if current_id.ld >= 255 {
            // Cannot split further, keep as leaf even if over threshold
            log_info(&format!(
                "⚠️  Max depth reached: {} entries remain in shard",
                current_data.entries.len()
            ));
            leaf_shards.push((current_id, current_data));
            continue;
        }

        // Perform binary split
        let (left_id, left_data, right_id, right_data, ssr_entry) =
            split_once(current_id, &current_data);

        log_info(&format!(
            "  Split ld={} → ld={}: {} entries → [{}, {}]",
            current_id.ld,
            left_id.ld,
            current_data.entries.len(),
            left_data.entries.len(),
            right_data.entries.len()
        ));

        // Record SSR entry for this internal split point
        ssr_entries.push(ssr_entry);

        // Check if children need further splitting
        if left_data.entries.len() > SPLIT_THRESHOLD {
            work_queue.push((left_id, left_data));
        } else {
            leaf_shards.push((left_id, left_data));
        }

        if right_data.entries.len() > SPLIT_THRESHOLD {
            work_queue.push((right_id, right_data));
        } else {
            leaf_shards.push((right_id, right_data));
        }
    }

    log_info(&format!(
        "✅ Recursive split complete: {} input entries → {} leaf shards, {} SSR entries (internal splits)",
        shard_data.entries.len(),
        leaf_shards.len(),
        ssr_entries.len()
    ));

    Some((leaf_shards, ssr_entries))
}

// ===== Serialization Functions =====

/// Serialize SSR metadata to DA format (8B header + 9B entries)
///
/// Format: [1B global_depth][1B entry_count][4B total_keys][4B version][SSREntry...]
/// Each SSREntry: [1B d][1B ld][7B prefix56]
pub fn serialize_ssr(ssr: &SSRData) -> Vec<u8> {
    let mut result = Vec::new();

    // SSR Header (8 bytes)
    result.push(ssr.header.global_depth);
    result.push(ssr.header.entry_count);
    result.extend_from_slice(&ssr.header.total_keys.to_le_bytes());
    result.extend_from_slice(&ssr.header.version.to_le_bytes());

    // SSR Entries (9 bytes each)
    for entry in &ssr.entries {
        result.push(entry.d);
        result.push(entry.ld);
        result.extend_from_slice(&entry.prefix56);
    }

    result
}

/// Deserialize SSR data from DA format
/// Used for loading SSR objects from DA during state reconstruction
pub fn deserialize_ssr(data: &[u8]) -> Option<SSRData> {
    if data.len() < 10 {
        return None;
    }

    // Parse header (10 bytes)
    let global_depth = data[0];
    let entry_count = data[1];
    let total_keys = u32::from_le_bytes([data[2], data[3], data[4], data[5]]);
    let version = u32::from_le_bytes([data[6], data[7], data[8], data[9]]);

    // Parse entries
    let mut entries = Vec::new();
    let mut offset = 10;
    for _ in 0..entry_count {
        if offset + 9 > data.len() {
            return None;
        }
        let d = data[offset];
        let ld = data[offset + 1];
        let mut prefix56 = [0u8; 7];
        prefix56.copy_from_slice(&data[offset + 2..offset + 9]);
        entries.push(SSREntry { d, ld, prefix56 });
        offset += 9;
    }

    Some(SSRData {
        header: SSRHeader {
            global_depth,
            entry_count,
            total_keys,
            version,
        },
        entries,
    })
}

/// Serialize shard data to DA format (sorted entries)
///
/// Format: [2B count][EvmEntry...]
/// Each EvmEntry: [32B key_h][32B value] (64 bytes total)
pub fn serialize_shard(shard: &ShardData) -> Vec<u8> {
    use utils::functions::log_crit;

    const MAX_ENTRIES: usize = 63;

    // Emergency validation: enforce hard DA segment limit
    if shard.entries.len() > MAX_ENTRIES {
        log_crit(&format!(
            "❌ FATAL: Shard has {} entries, exceeds hard limit of {}! Segment will fail DA import.",
            shard.entries.len(),
            MAX_ENTRIES
        ));
        // Panic in debug builds to catch during development
        #[cfg(debug_assertions)]
        panic!("Shard entry count exceeds DA segment limit: {} > {}", shard.entries.len(), MAX_ENTRIES);
    }

    let mut result = Vec::new();

    // Entry count (2 bytes)
    let count = shard.entries.len() as u16;
    result.extend_from_slice(&count.to_le_bytes());

    // Entries (64 bytes each: 32 bytes key_h + 32 bytes value)
    for entry in &shard.entries {
        result.extend_from_slice(entry.key_h.as_bytes());
        result.extend_from_slice(entry.value.as_bytes());
    }

    result
}

/// Deserialize shard data from DA format
pub fn deserialize_shard(data: &[u8]) -> Option<ShardData> {
    if data.len() < 2 {
        return None;
    }

    let count = u16::from_le_bytes([data[0], data[1]]) as usize;
    let expected_len = 2 + (count * 64);
    if data.len() < expected_len {
        return None;
    }

    let mut entries = Vec::new();
    let mut offset = 2;
    for _ in 0..count {
        let key_h = H256::from_slice(&data[offset..offset + 32]);
        let value = H256::from_slice(&data[offset + 32..offset + 64]);
        entries.push(EvmEntry { key_h, value });
        offset += 64;
    }

    Some(ShardData { entries })
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
    /// Block object (kind=0x05)
    Block = 0x05,
    /// Block metadata object (kind=0x06) - contains computed values
    BlockMetadata = 0x06,
}

impl ObjectKind {
    /// Try to parse from byte value
    pub fn from_u8(value: u8) -> Option<Self> {
        match value {
            0x00 => Some(Self::Code),
            0x01 => Some(Self::StorageShard),
            0x02 => Some(Self::SsrMetadata),
            0x03 => Some(Self::Receipt),
            0x05 => Some(Self::Block),
            0x06 => Some(Self::BlockMetadata),
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
        contract_storage.ssr.header.entry_count = contract_storage.ssr.entries.len() as u8;

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
pub fn dump_entries(shard_data: &ShardData) {

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
