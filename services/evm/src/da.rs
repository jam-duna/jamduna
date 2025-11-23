//! Generic DA (Data Availability) Sharding with SSR (Sparse Split Registry)
//!
//! # Overview
//!
//! This module provides generic, type-parameterized SSR (Sparse Split Registry) logic
//! for adaptive sharding of arbitrary data structures stored in JAM DA segments.
//!
//! The SSR pattern can be reused for:
//! - Contract storage shards (EvmEntry with H256 key-value pairs)
//! - Meta-shards for JAM State (ObjectRefEntry with ObjectID→ObjectRef mappings)
//! - Any future DA objects requiring adaptive sharding
//!
//! # Architecture
//!
//! - **SSREntry trait**: Defines serialization interface for any entry type
//! - **SSRData<E>**: Generic SSR metadata tracking split points and shard depths
//! - **ShardData<E>**: Generic shard containing sorted entries of type E
//! - **ShardId**: Shard identifier with local depth (ld) and 56-bit routing prefix
//!
//! # Key Functions
//!
//! - `resolve_shard_id<E>()`: Maps entry key → ShardId using SSR routing
//! - `maybe_split_shard_recursive<E>()`: Recursively splits oversized shards
//! - `serialize_shard<E>()` / `deserialize_shard<E>()`: DA segment format conversion
//!
//! # Type Parameters
//!
//! `E: SSREntry` - Entry type stored in shards, must provide:
//! - `key_hash()` - 32-byte routing key for shard assignment
//! - `serialized_size()` - Fixed size per entry in bytes
//! - `serialize()` / `deserialize()` - DA format conversion

use alloc::format;
use alloc::vec;
use alloc::vec::Vec;
use core::cmp::Ordering;
use utils::functions::{log_crit, log_info};

// ===== Error Types =====

#[derive(Debug)]
pub enum SerializeError {
    InvalidSize,
    InvalidFormat,
    InvalidDepth,
}

// ===== SSR Entry Trait =====

/// Generic entry type for SSR-based sharding
///
/// Any data structure that implements this trait can be sharded using the SSR system.
/// The trait defines how entries are routed to shards and serialized to DA segments.
pub trait SSREntry: Clone {
    /// Returns the 32-byte hash used for shard routing
    ///
    /// This hash determines which shard the entry belongs to by using its
    /// first N bits as a routing prefix (where N is determined by shard depth).
    fn key_hash(&self) -> [u8; 32];

    /// Returns the fixed serialized size of this entry type in bytes
    ///
    /// Used to calculate shard capacity and split thresholds.
    /// Must be constant across all instances of this type.
    fn serialized_size() -> usize;

    /// Serializes this entry to bytes for DA storage
    fn serialize(&self) -> Vec<u8>;

    /// Deserializes an entry from DA bytes
    fn deserialize(data: &[u8]) -> Result<Self, SerializeError>;
}

// ===== SSR Data Structures =====

/// SSR Header - contract/meta-shard storage metadata (13 bytes)
/// - global_depth: 1 byte
/// - entry_count: 4 bytes (u32 to support >255 splits for 1M shards)
/// - total_keys: 4 bytes
/// - version: 4 bytes
#[derive(Clone)]
pub struct SSRHeader {
    pub global_depth: u8, // Default depth for all shards
    pub entry_count: u32, // Number of exceptions (u32 to support >255 splits for 1M shards)
    pub total_keys: u32,  // Total entries across all shards
    pub version: u32,     // Increments on update
}

/// SSR Entry Metadata - tracks a split exception (9 bytes)
///
/// An SSR entry records: "All keys whose first `d` bits match `prefix56`
/// are governed at depth `ld` (instead of global_depth)."
#[derive(Clone, Copy, PartialEq, Eq)]
pub struct SSREntryMeta {
    pub d: u8,             // Split point (prefix bit position)
    pub ld: u8,            // Local depth at this prefix
    pub prefix56: [u8; 7], // 56-bit routing prefix (first d bits significant)
}

impl PartialOrd for SSREntryMeta {
    fn partial_cmp(&self, other: &Self) -> Option<Ordering> {
        Some(self.cmp(other))
    }
}

impl Ord for SSREntryMeta {
    fn cmp(&self, other: &Self) -> Ordering {
        self.d
            .cmp(&other.d)
            .then_with(|| self.prefix56.cmp(&other.prefix56))
    }
}

/// SSR Data - sparse split registry (type-parameterized)
#[derive(Clone)]
pub struct SSRData<E: SSREntry> {
    pub header: SSRHeader,
    pub entries: Vec<SSREntryMeta>, // Sorted exceptions
    _phantom: core::marker::PhantomData<E>,
}

impl<E: SSREntry> SSRData<E> {
    pub fn new() -> Self {
        Self {
            header: SSRHeader {
                global_depth: 0,
                entry_count: 0,
                total_keys: 0,
                version: 1,
            },
            entries: Vec::new(),
            _phantom: core::marker::PhantomData,
        }
    }

    pub fn from_parts(header: SSRHeader, entries: Vec<SSREntryMeta>) -> Self {
        Self {
            header,
            entries,
            _phantom: core::marker::PhantomData,
        }
    }
}

/// Shard ID - identifies a storage shard (8 bytes)
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub struct ShardId {
    pub ld: u8,            // Local depth (0-56)
    pub prefix56: [u8; 7], // 56-bit routing prefix
}

impl PartialOrd for ShardId {
    fn partial_cmp(&self, other: &Self) -> Option<Ordering> {
        Some(self.cmp(other))
    }
}

impl Ord for ShardId {
    fn cmp(&self, other: &Self) -> Ordering {
        self.ld
            .cmp(&other.ld)
            .then_with(|| self.prefix56.cmp(&other.prefix56))
    }
}

impl ShardId {
    /// Create root shard (depth 0, all zeros)
    pub fn root() -> Self {
        Self {
            ld: 0,
            prefix56: [0u8; 7],
        }
    }

    /// Encode ShardId as u32 for routing (top 20 bits for meta-sharding)
    pub fn to_u32(&self) -> u32 {
        // Use first 20 bits: 8 bits from ld + 12 bits from prefix56[0..2]
        let b0 = self.ld as u32;
        let b1 = self.prefix56[0] as u32;
        let b2 = (self.prefix56[1] >> 4) as u32; // Top 4 bits of second byte
        (b0 << 12) | (b1 << 4) | b2
    }

    /// Decode ShardId from u32 routing key
    pub fn from_u32(val: u32) -> Self {
        let ld = ((val >> 12) & 0xFF) as u8;
        let mut prefix56 = [0u8; 7];
        prefix56[0] = ((val >> 4) & 0xFF) as u8;
        prefix56[1] = ((val & 0x0F) << 4) as u8;
        Self { ld, prefix56 }
    }
}

/// Shard Data - sorted entries of type E
#[derive(Clone)]
pub struct ShardData<E: SSREntry> {
    pub entries: Vec<E>, // Sorted by key_hash
}

impl<E: SSREntry> ShardData<E> {
    pub fn new() -> Self {
        Self {
            entries: Vec::new(),
        }
    }
}

// ===== Prefix Manipulation Utilities =====

/// Extract first 56 bits (7 bytes) from 32-byte hash as routing prefix
fn take_prefix56(key_hash: &[u8; 32]) -> [u8; 7] {
    let mut prefix = [0u8; 7];
    prefix.copy_from_slice(&key_hash[0..7]);
    prefix
}

/// Check if key_prefix matches entry_prefix for first 'bits' bits
fn matches_prefix(key_prefix: [u8; 7], entry_prefix: [u8; 7], bits: u8) -> bool {
    let bytes_to_compare = (bits / 8) as usize;
    let remaining_bits = bits % 8;

    // Check full bytes
    if bytes_to_compare > 0 && key_prefix[0..bytes_to_compare] != entry_prefix[0..bytes_to_compare]
    {
        return false;
    }

    // Check remaining bits
    if remaining_bits > 0 && bytes_to_compare < 7 {
        let mask = 0xFF << (8 - remaining_bits);
        if (key_prefix[bytes_to_compare] & mask) != (entry_prefix[bytes_to_compare] & mask) {
            return false;
        }
    }

    true
}

/// Mask prefix to first 'bits' bits, zero out the rest
fn mask_prefix56(prefix: [u8; 7], bits: u8) -> [u8; 7] {
    if bits > 56 {
        // Clamp to 56 bits max
        return prefix;
    }

    let mut masked = [0u8; 7];
    let full_bytes = (bits / 8) as usize;
    let remaining_bits = bits % 8;

    // Copy full bytes
    if full_bytes > 0 {
        masked[0..full_bytes].copy_from_slice(&prefix[0..full_bytes]);
    }

    // Mask remaining bits
    if remaining_bits > 0 && full_bytes < 7 {
        let mask = 0xFF << (8 - remaining_bits);
        masked[full_bytes] = prefix[full_bytes] & mask;
    }

    masked
}

// ===== Shard Resolution & Splitting =====

/// Resolve which shard an entry belongs to using SSR
///
/// # Algorithm
/// 1. Start with global_depth from SSR header
/// 2. Extract 56-bit prefix from entry's key_hash
/// 3. Walk down SSR exceptions, descending into deeper shards when prefix matches
/// 4. Return ShardId with final depth and masked prefix
///
/// # Returns
/// ShardId identifying the target shard for this entry
pub fn resolve_shard_id<E: SSREntry>(key_hash: [u8; 32], ssr: &SSRData<E>) -> ShardId {
    let key_prefix56 = take_prefix56(&key_hash);

    // Start at the global depth (root shard) and walk down the split tree
    let mut current_ld = ssr.header.global_depth;
    let mut current_prefix = mask_prefix56(key_prefix56, current_ld);

    loop {
        let mut found_split = false;

        for entry in &ssr.entries {
            if entry.d == current_ld && matches_prefix(key_prefix56, entry.prefix56, entry.d) {
                // Descend one level deeper based on this split exception
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

/// Perform a single binary split of a shard
///
/// Helper function that splits a shard into left and right children based on one bit.
/// Used by recursive splitting logic.
///
/// # Returns
/// (left_shard_id, left_shard, right_shard_id, right_shard, ssr_entry_meta)
fn split_once<E: SSREntry>(
    shard_id: ShardId,
    shard_data: &ShardData<E>,
) -> (
    ShardId,
    ShardData<E>,
    ShardId,
    ShardData<E>,
    SSREntryMeta,
) {
    assert!(shard_id.ld < 255, "Cannot split shard at maximum depth");
    let new_depth = shard_id.ld + 1;
    let split_bit = shard_id.ld as usize;

    // Split entries based on the next bit position
    let mut left_entries = Vec::new();
    let mut right_entries = Vec::new();

    for entry in &shard_data.entries {
        let key_hash = entry.key_hash();
        let key_prefix = take_prefix56(&key_hash);

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

    // Create SSR entry metadata for the split point
    let ssr_entry_meta = SSREntryMeta {
        d: shard_id.ld,          // Split point (previous depth)
        ld: new_depth,           // New local depth
        prefix56: shard_id.prefix56, // Original routing prefix
    };

    (
        left_shard_id,
        left_shard,
        right_shard_id,
        right_shard,
        ssr_entry_meta,
    )
}

/// Recursively split a shard until all result shards meet threshold
///
/// # Algorithm
/// 1. Use work queue to process shards that need splitting
/// 2. For each shard > threshold, perform binary split
/// 3. Check if children still exceed threshold and queue them for further splitting
/// 4. Continue until all result shards ≤ threshold
///
/// # Returns
/// Some((leaf_shards, ssr_entries)) where:
/// - leaf_shards: Vec of (ShardId, ShardData<E>) for all result leaves
/// - ssr_entries: Vec of SSREntryMeta for internal split points (N leaves → N-1 entries)
pub fn maybe_split_shard_recursive<E: SSREntry>(
    shard_id: ShardId,
    shard_data: ShardData<E>,
    threshold: usize,
) -> Option<(Vec<(ShardId, ShardData<E>)>, Vec<SSREntryMeta>)> {
    if shard_data.entries.len() <= threshold {
        return None;
    }

    log_info(&format!(
        "🔀 Recursive split starting: {} entries in shard ld={}",
        shard_data.entries.len(),
        shard_id.ld
    ));

    let mut leaf_shards = Vec::new();
    let mut work_queue = vec![(shard_id, shard_data)];
    let mut ssr_entries = Vec::new();

    while let Some((current_id, current_data)) = work_queue.pop() {
        if current_data.entries.len() <= threshold {
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
        if left_data.entries.len() > threshold {
            work_queue.push((left_id, left_data));
        } else {
            leaf_shards.push((left_id, left_data));
        }

        if right_data.entries.len() > threshold {
            work_queue.push((right_id, right_data));
        } else {
            leaf_shards.push((right_id, right_data));
        }
    }

    log_info(&format!(
        "✅ Recursive split complete: {} leaf shards, {} SSR entries",
        leaf_shards.len(),
        ssr_entries.len()
    ));

    Some((leaf_shards, ssr_entries))
}

// ===== Serialization Functions =====

/// Serialize SSR metadata to DA format (13B header + 9B entries)
///
/// Format: [1B global_depth][4B entry_count][4B total_keys][4B version][SSREntryMeta...]
/// Each SSREntryMeta: [1B d][1B ld][7B prefix56]
pub fn serialize_ssr<E: SSREntry>(ssr: &SSRData<E>) -> Vec<u8> {
    let mut result = Vec::new();

    // SSR Header (13 bytes)
    result.push(ssr.header.global_depth);                          // 1 byte
    result.extend_from_slice(&ssr.header.entry_count.to_le_bytes()); // 4 bytes
    result.extend_from_slice(&ssr.header.total_keys.to_le_bytes());  // 4 bytes
    result.extend_from_slice(&ssr.header.version.to_le_bytes());     // 4 bytes

    // SSR Entries (9 bytes each)
    for entry in &ssr.entries {
        result.push(entry.d);
        result.push(entry.ld);
        result.extend_from_slice(&entry.prefix56);
    }

    result
}

/// Deserialize SSR data from DA format
pub fn deserialize_ssr<E: SSREntry>(data: &[u8]) -> Result<SSRData<E>, SerializeError> {
    if data.len() < 13 {
        return Err(SerializeError::InvalidSize);
    }

    // Parse header (13 bytes)
    let global_depth = data[0];
    let entry_count = u32::from_le_bytes([data[1], data[2], data[3], data[4]]);
    let total_keys = u32::from_le_bytes([data[5], data[6], data[7], data[8]]);
    let version = u32::from_le_bytes([data[9], data[10], data[11], data[12]]);

    // Parse entries
    let mut entries = Vec::new();
    let mut offset = 13;
    for _ in 0..entry_count {
        if offset + 9 > data.len() {
            return Err(SerializeError::InvalidFormat);
        }
        let d = data[offset];
        let ld = data[offset + 1];
        let mut prefix56 = [0u8; 7];
        prefix56.copy_from_slice(&data[offset + 2..offset + 9]);
        entries.push(SSREntryMeta { d, ld, prefix56 });
        offset += 9;
    }

    Ok(SSRData {
        header: SSRHeader {
            global_depth,
            entry_count,
            total_keys,
            version,
        },
        entries,
        _phantom: core::marker::PhantomData,
    })
}

/// Serialize shard data to DA format
///
/// Format: [2B count][Entry...]
/// Each entry serialized via E::serialize()
///
/// # Panics
/// - In debug builds: if shard exceeds max_entries
pub fn serialize_shard<E: SSREntry>(shard: &ShardData<E>, max_entries: usize) -> Vec<u8> {
    // Emergency validation: enforce hard DA segment limit
    if shard.entries.len() > max_entries {
        log_crit(&format!(
            "❌ FATAL: Shard has {} entries, exceeds limit of {}!",
            shard.entries.len(),
            max_entries
        ));
        #[cfg(debug_assertions)]
        panic!(
            "Shard entry count exceeds limit: {} > {}",
            shard.entries.len(),
            max_entries
        );
    }

    let mut result = Vec::new();

    // Entry count (2 bytes)
    let count = shard.entries.len() as u16;
    result.extend_from_slice(&count.to_le_bytes());

    // Entries (E::serialized_size() bytes each)
    for entry in &shard.entries {
        result.extend_from_slice(&entry.serialize());
    }

    result
}

/// Deserialize shard data from DA format
pub fn deserialize_shard<E: SSREntry>(data: &[u8]) -> Result<ShardData<E>, SerializeError> {
    if data.len() < 2 {
        return Err(SerializeError::InvalidSize);
    }

    let count = u16::from_le_bytes([data[0], data[1]]) as usize;
    let entry_size = E::serialized_size();
    let expected_len = 2 + (count * entry_size);

    if data.len() < expected_len {
        return Err(SerializeError::InvalidSize);
    }

    let mut entries = Vec::new();
    let mut offset = 2;
    for _ in 0..count {
        let entry = E::deserialize(&data[offset..offset + entry_size])?;
        entries.push(entry);
        offset += entry_size;
    }

    Ok(ShardData { entries })
}

#[cfg(test)]
mod tests {
    use super::*;

    // Mock entry type for testing
    #[derive(Clone, PartialEq, Eq)]
    struct MockEntry {
        key: [u8; 32],
        value: u32,
    }

    impl SSREntry for MockEntry {
        fn key_hash(&self) -> [u8; 32] {
            self.key
        }

        fn serialized_size() -> usize {
            36 // 32 bytes key + 4 bytes value
        }

        fn serialize(&self) -> Vec<u8> {
            let mut buf = Vec::with_capacity(36);
            buf.extend_from_slice(&self.key);
            buf.extend_from_slice(&self.value.to_le_bytes());
            buf
        }

        fn deserialize(data: &[u8]) -> Result<Self, SerializeError> {
            if data.len() != 36 {
                return Err(SerializeError::InvalidSize);
            }
            let mut key = [0u8; 32];
            key.copy_from_slice(&data[0..32]);
            let value = u32::from_le_bytes([data[32], data[33], data[34], data[35]]);
            Ok(MockEntry { key, value })
        }
    }

    #[test]
    fn test_shard_id_u32_roundtrip() {
        let shard = ShardId {
            ld: 5,
            prefix56: [0x12, 0x34, 0x56, 0x78, 0x9A, 0xBC, 0xDE],
        };
        let encoded = shard.to_u32();
        let decoded = ShardId::from_u32(encoded);

        // First 20 bits should match
        assert_eq!(decoded.ld, shard.ld);
        assert_eq!(decoded.prefix56[0], shard.prefix56[0]);
        // Top 4 bits of second byte preserved
        assert_eq!(decoded.prefix56[1] & 0xF0, shard.prefix56[1] & 0xF0);
    }

    #[test]
    fn test_serialize_deserialize_shard() {
        let shard = ShardData {
            entries: vec![
                MockEntry {
                    key: [0x01; 32],
                    value: 42,
                },
                MockEntry {
                    key: [0x02; 32],
                    value: 99,
                },
            ],
        };

        let serialized = serialize_shard(&shard, 100);
        let deserialized: ShardData<MockEntry> = deserialize_shard(&serialized).unwrap();

        assert_eq!(deserialized.entries.len(), 2);
        assert_eq!(deserialized.entries[0].value, 42);
        assert_eq!(deserialized.entries[1].value, 99);
    }
}
