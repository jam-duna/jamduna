//! Meta-Sharding for JAM State Object Registry
//!
//! # Overview
//!
//! This module implements adaptive sharding for ObjectRef entries stored in JAM State.
//! It uses the generic SSR (Sparse Split Registry) from `da.rs` to efficiently manage
//! millions of ObjectRefs without bloating JAM State or exceeding W_R limits.
//!
//! # Problem
//!
//! - Each EVM contract has multiple objects: Code, SSR metadata, Storage shards, Receipts, Blocks
//! - 1M contracts × 10 objects avg = 10M ObjectRefs
//! - At 72 bytes/ObjectRef, that's 720MB in JAM State (too large!)
//! - With W_R=48KB, can only update 414 ObjectRefs per core per block (not scalable!)
//!
//! # Solution: Two-Level Sharding
//!
//! **Layer 1 (JAM State):** Store 1M meta-shard commitments (not individual ObjectRefs)
//! - Each meta-shard holds ~1,000 ObjectRefs
//! - Total JAM State: 1M × 72 bytes = 72MB (bounded!)
//!
//! **Layer 2 (JAM DA):** Store ObjectID→ObjectRef mappings in meta-shard segments
//! - Each meta-shard segment: 4104 bytes with 96-byte entries
//! - Max 41 ObjectRefEntry per meta-shard (splits at 21 entries)
//! - Reuses SSR splitting logic from `da.rs`
//!
//! # Data Structures
//!
//! - `ObjectRefEntry`: ObjectID (32 bytes) + ObjectRef (64 bytes) = 96 bytes
//! - `MetaSSR`: Type alias for SSRData<ObjectRefEntry>
//! - `MetaShard`: Type alias for ShardData<ObjectRefEntry>
//!
//! # Key Functions
//!
//! - `process_object_writes()`: Groups ObjectRef writes by meta-shard, handles splits
//! - `serialize_meta_shard()`: Converts meta-shard to DA segments
//! - `deserialize_meta_shard()`: Loads meta-shard from DA segments
//!
//! # Scaling Benefits
//!
//! - JAM State size: 72MB (vs 720MB without meta-sharding)
//! - Work report capacity: 414 meta-shard updates/WP × 1K ObjectRefs/meta-shard = **414K ObjectRef updates/WP**
//! - Throughput: 141K meta-shard updates/block across 341 cores

use alloc::collections::BTreeMap;
use alloc::format;
use alloc::vec::Vec;
use utils::functions::log_info;
use utils::objects::ObjectRef;

// Import generic SSR types and functions from da module
use crate::da::{self, SSREntry, SerializeError, ShardId, SSRData, ShardData};

/// ObjectRefEntry - ObjectID mapped to ObjectRef (32 + 64 = 96 bytes)
///
/// Stored in meta-shards in JAM DA. Each entry maps a 32-byte ObjectID
/// to its corresponding ObjectRef (64 bytes total).
#[derive(Clone, PartialEq)]
pub struct ObjectRefEntry {
    pub object_id: [u8; 32],  // 32 bytes - ObjectID for routing and lookup
    pub object_ref: ObjectRef, // 64 bytes - see utils::objects::ObjectRef
}

impl Eq for ObjectRefEntry {}

impl PartialOrd for ObjectRefEntry {
    fn partial_cmp(&self, other: &Self) -> Option<core::cmp::Ordering> {
        Some(self.cmp(other))
    }
}

impl Ord for ObjectRefEntry {
    fn cmp(&self, other: &Self) -> core::cmp::Ordering {
        self.object_id.cmp(&other.object_id)
    }
}

impl SSREntry for ObjectRefEntry {
    fn key_hash(&self) -> [u8; 32] {
        self.object_id
    }

    fn serialized_size() -> usize {
        96 // 32 (object_id) + 64 (ObjectRef::SERIALIZED_SIZE)
    }

    fn serialize(&self) -> Vec<u8> {
        let mut buf = Vec::with_capacity(96);
        buf.extend_from_slice(&self.object_id);
        buf.extend_from_slice(&self.object_ref.serialize());
        buf
    }

    fn deserialize(data: &[u8]) -> Result<Self, SerializeError> {
        if data.len() != 96 {
            return Err(SerializeError::InvalidSize);
        }

        let mut object_id = [0u8; 32];
        object_id.copy_from_slice(&data[0..32]);

        let object_ref = ObjectRef::deserialize(&data[32..96])
            .ok_or(SerializeError::InvalidFormat)?;

        Ok(ObjectRefEntry {
            object_id,
            object_ref,
        })
    }
}

/// Type alias for meta-shard SSR (using ObjectRefEntry)
pub type MetaSSR = SSRData<ObjectRefEntry>;

/// Type alias for meta-shard data (using ObjectRefEntry)
pub type MetaShard = ShardData<ObjectRefEntry>;

/// Meta-shard split threshold - split when exceeding this many entries (~50% of max)
pub const META_SHARD_SPLIT_THRESHOLD: usize = 21;

/// Maximum entries per meta-shard (hard DA segment limit)
/// Calculation: (4008 - 2) / 96 = 41.729... → 41 max entries
pub const META_SHARD_MAX_ENTRIES: usize = 41;

/// Meta-shard write intent - output from process_object_writes()
#[derive(Clone)]
pub struct MetaShardWriteIntent {
    pub shard_id: ShardId,
    pub entries: Vec<ObjectRefEntry>,
}

/// Process object writes and group them by meta-shard
///
/// Takes a list of ObjectID→ObjectRef mappings and:
/// 1. Routes each ObjectID to its meta-shard using SSR
/// 2. Merges new ObjectRefs with cached meta-shard entries
/// 3. Checks if meta-shard needs splitting (>21 entries)
/// 4. Returns write intents for all touched meta-shards
///
/// # Arguments
/// - `object_writes`: Vec of (ObjectID, ObjectRef) pairs to write
/// - `cached_meta_shards`: BTreeMap of currently loaded meta-shards
/// - `meta_ssr`: Global SSR for routing ObjectIDs to meta-shards
///
/// # Returns
/// Vec of MetaShardWriteIntent with shard_id and updated entries
pub fn process_object_writes(
    object_writes: Vec<([u8; 32], ObjectRef)>,
    cached_meta_shards: &mut BTreeMap<ShardId, MetaShard>,
    meta_ssr: &mut MetaSSR,
) -> Vec<MetaShardWriteIntent> {
    if object_writes.is_empty() {
        return Vec::new();
    }

    log_info(&format!(
        "📦 Processing {} object writes for meta-sharding",
        object_writes.len()
    ));

    // 1. Group object writes by meta-shard
    let mut shard_updates: BTreeMap<ShardId, Vec<ObjectRefEntry>> = BTreeMap::new();

    for (object_id, object_ref) in object_writes {
        let shard_id = da::resolve_shard_id(object_id, meta_ssr);

        shard_updates
            .entry(shard_id)
            .or_insert_with(Vec::new)
            .push(ObjectRefEntry {
                object_id,
                object_ref,
            });
    }

    log_info(&format!(
        "  Grouped into {} meta-shards",
        shard_updates.len()
    ));

    let mut write_intents = Vec::new();

    // 2. For each touched meta-shard, merge updates and check for splits
    for (shard_id, new_entries) in shard_updates {
        let mut shard_data = cached_meta_shards
            .get(&shard_id)
            .cloned()
            .unwrap_or_else(|| MetaShard { entries: Vec::new() });

        log_info(&format!(
            "  Meta-shard {:?}: {} existing + {} new entries",
            shard_id,
            shard_data.entries.len(),
            new_entries.len()
        ));

        // Merge new entries (update or insert)
        for new_entry in new_entries {
            if let Some(existing) = shard_data
                .entries
                .iter_mut()
                .find(|e| e.object_id == new_entry.object_id)
            {
                *existing = new_entry;
            } else {
                shard_data.entries.push(new_entry);
            }
        }
        shard_data.entries.sort();

        // Check if split needed
        if shard_data.entries.len() > META_SHARD_SPLIT_THRESHOLD {
            log_info(&format!(
                "  🔀 Meta-shard split triggered: {} entries > {} threshold",
                shard_data.entries.len(),
                META_SHARD_SPLIT_THRESHOLD
            ));

            match da::maybe_split_shard_recursive(
                shard_id,
                shard_data.clone(),
                META_SHARD_SPLIT_THRESHOLD,
            ) {
                Some((leaf_shards, ssr_entries)) => {
                    log_info(&format!(
                        "  ✅ Split complete: {} leaf shards, {} SSR entries",
                        leaf_shards.len(),
                        ssr_entries.len()
                    ));

                    // Update MetaSSR with new split entries
                    for ssr_entry in ssr_entries {
                        meta_ssr.entries.push(ssr_entry);
                    }
                    meta_ssr.entries.sort();

                    // Update header (entry_count is now u32, supports >255 entries)
                    meta_ssr.header.entry_count = meta_ssr.entries.len() as u32;

                    // Sanity check: Warn if approaching u32::MAX (should never happen with 1M shards)
                    if meta_ssr.entries.len() > 1_000_000 {
                        log_info(&format!(
                            "  ⚠️  Meta-SSR has {} routing entries (unusually high!)",
                            meta_ssr.entries.len()
                        ));
                    }

                    // Create write intents for all leaf shards
                    for (leaf_id, leaf_data) in leaf_shards {
                        cached_meta_shards.insert(leaf_id, leaf_data.clone());
                        write_intents.push(MetaShardWriteIntent {
                            shard_id: leaf_id,
                            entries: leaf_data.entries,
                        });
                    }

                    // Remove stale parent shard
                    cached_meta_shards.remove(&shard_id);
                }
                None => {
                    // No split needed, just write updated shard
                    let entries = shard_data.entries.clone();
                    cached_meta_shards.insert(shard_id, shard_data);
                    write_intents.push(MetaShardWriteIntent {
                        shard_id,
                        entries,
                    });
                }
            }
        } else {
            // Under threshold, just write updated shard
            let entries = shard_data.entries.clone();
            cached_meta_shards.insert(shard_id, shard_data);
            write_intents.push(MetaShardWriteIntent {
                shard_id,
                entries,
            });
        }
    }

    log_info(&format!(
        "✅ Generated {} meta-shard write intents",
        write_intents.len()
    ));

    write_intents
}

/// Result type for meta-shard deserialization with security validation
pub enum MetaShardDeserializeResult {
    /// Successfully deserialized with validated header
    ValidatedHeader(ShardId, MetaShard),
    /// Payload is too short to contain header - likely legacy format
    MaybeLegacy,
    /// Header present but validation failed - malicious or corrupt payload
    ValidationFailed,
}

/// Serialize meta-shard to DA format with shard_id header
///
/// Format: [1B ld][7B prefix56][...shard entries]
/// The shard_id header enables witness imports to identify the shard without SSR routing
pub fn serialize_meta_shard_with_id(shard: &MetaShard, shard_id: ShardId) -> Vec<u8> {
    let mut data = Vec::new();
    data.push(shard_id.ld);
    data.extend_from_slice(&shard_id.prefix56);
    data.extend_from_slice(&da::serialize_shard(shard, META_SHARD_MAX_ENTRIES));
    data
}

/// Deserialize meta-shard from DA format with shard_id header and validate
///
/// **Security**: Validates header (ld, prefix56) against expected object_id
/// Rejects payloads where header disagrees with object_id to prevent routing corruption
///
/// **Tri-state return**:
/// - `ValidatedHeader(shard_id, shard)`: Header validated successfully
/// - `MaybeLegacy`: Payload too short for header, try legacy deserialization
/// - `ValidationFailed`: Header present but invalid - REJECT, do NOT fall back to legacy
///
/// **CRITICAL**: Caller must treat `ValidationFailed` as fatal, not fall back to legacy.
/// Otherwise malicious witnesses can craft bytes that fail validation then get accepted as legacy.
pub fn deserialize_meta_shard_with_id_validated(
    data: &[u8],
    expected_object_id: &[u8; 32],
    service_id: u32,
) -> MetaShardDeserializeResult {
    use utils::functions::log_error;

    // Check minimum size for header
    if data.len() < 8 {
        return MetaShardDeserializeResult::MaybeLegacy;
    }

    // Extract shard_id from header
    let ld = data[0];
    let mut prefix56 = [0u8; 7];
    prefix56.copy_from_slice(&data[1..8]);
    let shard_id = ShardId { ld, prefix56 };

    // CRITICAL: Validate header against object_id
    // Recompute object_id from header and verify it matches
    let computed_object_id = meta_shard_object_id(service_id, shard_id);
    if &computed_object_id != expected_object_id {
        // Header claims shard_id A but object_id corresponds to shard_id B
        // This is either corruption or a malicious witness
        // MUST NOT fall back to legacy - return ValidationFailed
        log_error("❌ Meta-shard header validation failed: object_id mismatch");
        return MetaShardDeserializeResult::ValidationFailed;
    }

    // Deserialize shard data after validated header
    match da::deserialize_shard(&data[8..]) {
        Ok(shard) => MetaShardDeserializeResult::ValidatedHeader(shard_id, shard),
        Err(_) => {
            // Header validated but shard data corrupt
            log_error("❌ Meta-shard header valid but shard data corrupt");
            MetaShardDeserializeResult::ValidationFailed
        }
    }
}

/// Deserialize meta-shard from DA format with shard_id header (unvalidated, legacy)
///
/// Returns (shard_id, meta_shard) tuple
///
/// **DEPRECATED**: Use `deserialize_meta_shard_with_id_validated()` instead
/// This version does not validate the header and is unsafe for untrusted witnesses
pub fn deserialize_meta_shard_with_id(data: &[u8]) -> Option<(ShardId, MetaShard)> {
    if data.len() < 8 {
        return None;
    }

    let ld = data[0];
    let mut prefix56 = [0u8; 7];
    prefix56.copy_from_slice(&data[1..8]);
    let shard_id = ShardId { ld, prefix56 };

    let shard = da::deserialize_shard(&data[8..]).ok()?;
    Some((shard_id, shard))
}

/// Serialize meta-shard to DA format (legacy, no shard_id header)
///
/// Wrapper around da::serialize_shard() for ObjectRefEntry type.
pub fn serialize_meta_shard(shard: &MetaShard) -> Vec<u8> {
    da::serialize_shard(shard, META_SHARD_MAX_ENTRIES)
}

/// Deserialize meta-shard from DA format (legacy, no shard_id header)
///
/// Wrapper around da::deserialize_shard() for ObjectRefEntry type.
pub fn deserialize_meta_shard(data: &[u8]) -> Option<MetaShard> {
    da::deserialize_shard(data).ok()
}

/// Serialize MetaSSR to DA format
///
/// Wrapper around da::serialize_ssr() for ObjectRefEntry type.
pub fn serialize_meta_ssr(ssr: &MetaSSR) -> Vec<u8> {
    da::serialize_ssr(ssr)
}

/// Deserialize MetaSSR from DA format
///
/// Wrapper around da::deserialize_ssr() for ObjectRefEntry type.
pub fn deserialize_meta_ssr(data: &[u8]) -> Option<MetaSSR> {
    da::deserialize_ssr(data).ok()
}

/// Compute digest of meta-shard entries
///
/// Used for conflict detection in accumulate phase.
/// Returns keccak256 hash of all serialized entries.
pub fn compute_meta_shard_digest(entries: &[ObjectRefEntry]) -> [u8; 32] {
    use utils::hash_functions::keccak256;

    let mut data = Vec::new();
    for entry in entries {
        data.extend_from_slice(&entry.serialize());
    }

    *keccak256(&data).as_fixed_bytes()
}

// ===== ObjectID Construction for Meta-Shards =====

/// Compute ObjectID for a meta-shard
///
/// ObjectID = keccak256(service_id || "meta_shard" || ld || prefix56)
pub fn meta_shard_object_id(service_id: u32, shard_id: ShardId) -> [u8; 32] {
    use utils::hash_functions::keccak256;

    let mut data = Vec::new();
    data.extend_from_slice(&service_id.to_le_bytes());
    data.extend_from_slice(b"meta_shard");
    data.push(shard_id.ld);
    data.extend_from_slice(&shard_id.prefix56);

    *keccak256(&data).as_fixed_bytes()
}

/// Compute ObjectID for the meta-SSR metadata object
///
/// ObjectID = keccak256(service_id || "meta_ssr")
pub fn meta_ssr_object_id(service_id: u32) -> [u8; 32] {
    use utils::hash_functions::keccak256;

    let mut data = Vec::new();
    data.extend_from_slice(&service_id.to_le_bytes());
    data.extend_from_slice(b"meta_ssr");

    *keccak256(&data).as_fixed_bytes()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_object_ref_entry_serialization() {
        let entry = ObjectRefEntry {
            object_id: [0x42; 32],
            object_ref: ObjectRef {
                service_id: 1,
                work_package_hash: [0xBB; 32],
                index_start: 10,
                index_end: 15,
                version: 1,
                payload_length: 100,
                object_kind: 1,
                log_index: 0,
                tx_slot: 5,
                timeslot: 100,
                gas_used: 21000,
                evm_block: 42,
            },
        };

        let serialized = entry.serialize();
        assert_eq!(serialized.len(), 96);

        let deserialized = ObjectRefEntry::deserialize(&serialized).unwrap();
        assert_eq!(deserialized.object_id, entry.object_id);
        assert_eq!(deserialized.object_ref.version, entry.object_ref.version);
        assert_eq!(
            deserialized.object_ref.work_package_hash,
            entry.object_ref.work_package_hash
        );
        assert_eq!(
            deserialized.object_ref.index_start,
            entry.object_ref.index_start
        );
        assert_eq!(
            deserialized.object_ref.index_end,
            entry.object_ref.index_end
        );
    }

    #[test]
    fn test_meta_shard_capacity() {
        // Verify our capacity calculations
        let entry_size = ObjectRefEntry::serialized_size();
        assert_eq!(entry_size, 96);

        // DA segment: 4104 bytes total, 2 bytes for count header
        let available = 4104 - 2;
        let max_entries = available / entry_size;
        assert_eq!(max_entries, 42); // (4102 / 96 = 42.729...)

        // We use 41 to have some buffer
        assert!(META_SHARD_MAX_ENTRIES <= max_entries);
    }

    #[test]
    fn test_process_single_object_write() {
        let mut cached_shards = BTreeMap::new();
        let mut meta_ssr = MetaSSR::new();

        let writes = vec![(
            [0x01; 32],
            ObjectRef {
                work_package_hash: [0xBB; 32],
                index_start: 0,
                payload_length: 100,
                object_kind: 1,
            },
        )];

        let intents = process_object_writes(writes, &mut cached_shards, &mut meta_ssr);

        assert_eq!(intents.len(), 1);
        assert_eq!(intents[0].entries.len(), 1);
        assert_eq!(intents[0].entries[0].object_id, [0x01; 32]);
    }
}
