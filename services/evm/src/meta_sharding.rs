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
//! - Each meta-shard segment: 4104 bytes with 68-byte entries
//! - Max 58 ObjectRefEntry per meta-shard (splits at 29 entries)
//! - Reuses SSR splitting logic from `da.rs`
//!
//! # Data Structures
//!
//! - `ObjectRefEntry`: ObjectID (32 bytes) + ObjectRef (37 bytes) = 69 bytes
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
use crate::da::{self, SSRData, SSREntry, SerializeError, ShardData, ShardId};

/// ObjectRefEntry - ObjectID mapped to ObjectRef
///
/// Stored in meta-shards in JAM DA. Full DA serialization format:
/// - object_id: 32 bytes (full ObjectID)
/// - object_ref: 37 bytes (32B wph + 5B packed)
/// Total: 69 bytes per entry (fixed size for DA storage)
#[derive(Clone, PartialEq)]
pub struct ObjectRefEntry {
    pub object_id: [u8; 32],   // 32 bytes internally for compatibility
    pub object_ref: ObjectRef, // Full ObjectRef internally
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
        // Full DA format: 32 bytes object_id + 37 bytes ObjectRef
        69
    }

    fn serialize(&self) -> Vec<u8> {
        // Full DA format: object_id (32B) + ObjectRef (37B)
        // This is what gets stored in JAM DA for witness proofs
        let mut buf = Vec::with_capacity(69);

        // Write full 32-byte object_id
        buf.extend_from_slice(&self.object_id);

        // Write full 37-byte serialized ObjectRef
        buf.extend_from_slice(&self.object_ref.serialize());

        buf
    }

    fn deserialize(data: &[u8]) -> Result<Self, SerializeError> {
        if data.len() != 69 {
            return Err(SerializeError::InvalidSize);
        }

        // Parse 32-byte object_id
        let mut object_id = [0u8; 32];
        object_id.copy_from_slice(&data[0..32]);

        // Parse 37-byte ObjectRef
        let object_ref =
            ObjectRef::deserialize(&data[32..69]).ok_or(SerializeError::InvalidObjectRef)?;

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
pub const META_SHARD_SPLIT_THRESHOLD: usize = 50;

/// Compute BMT root over meta-shard entries for Merkle proof generation
///
/// Each entry is hashed as: object_id (32 bytes) || object_ref (37 bytes) = 69 bytes
/// The BMT provides cryptographic proof that an object exists in the meta-shard
pub fn compute_entries_bmt_root(entries: &[ObjectRefEntry]) -> [u8; 32] {
    if entries.is_empty() {
        return [0u8; 32]; // Empty root
    }

    // Convert entries to (key, value) pairs for BMT
    // Key = object_id, Value = serialized object_ref
    let kvs: Vec<([u8; 32], Vec<u8>)> = entries
        .iter()
        .map(|entry| (entry.object_id, entry.object_ref.serialize().to_vec()))
        .collect();

    crate::bmt::compute_bmt_root(&kvs)
}

/// Maximum entries per meta-shard (hard DA segment limit)
/// Calculation: (4104 - 34) / 69 ≈ 59 max entries → capped at 58 for headroom
pub const META_SHARD_MAX_ENTRIES: usize = 58;

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
    service_id: u32,
) -> Vec<MetaShardWriteIntent> {
    if object_writes.is_empty() {
        return Vec::new();
    }

    log_info(&format!(
        "📦 Processing {} object writes for meta-sharding",
        object_writes.len()
    ));

    // Log all object_writes contents
    log_info(&format!(
        "📦 Object writes full dump ({} writes):",
        object_writes.len()
    ));
    for (idx, (object_id, object_ref)) in object_writes.iter().enumerate() {
        log_info(&format!(
            "  [{}] object_id={:?}, object_ref={{wph: {:?}, idx_start: {}, payload_len: {}, kind: {}}}",
            idx,
            object_id,
            object_ref.work_package_hash,
            object_ref.index_start,
            object_ref.payload_length,
            object_ref.object_kind
        ));
    }

    // Log meta_ssr state
    log_info(&format!(
        "📦 MetaSSR state: global_depth={}, {} entries, total_keys={}",
        meta_ssr.header.global_depth,
        meta_ssr.entries.len(),
        meta_ssr.header.total_keys
    ));
    for (idx, entry) in meta_ssr.entries.iter().enumerate() {
        log_info(&format!(
            "  SSR Entry [{}]: d={}, ld={}, prefix56={:?}",
            idx,
            entry.d,
            entry.ld,
            &entry.prefix56[..]
        ));
    }

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
        // For BUILD path: Try fetching from DA first, then fall back to cache
        // This ensures we get accumulated state from previous rounds
        let mut shard_data = if let Some(cached) = cached_meta_shards.get(&shard_id) {
            // Check if cached shard is empty - if so, try fetching from DA anyway
            // Empty cached shards might be stale (from previous rounds that didn't persist to DA yet)
            if cached.entries.is_empty() {
                // Try DA fetch for potentially accumulated state
                let object_id = meta_shard_object_id(service_id, shard_id.ld, &shard_id.prefix56);

                log_info(&format!(
                    "🔍 Cached shard empty, fetching from DA: object_id={}",
                    crate::sharding::format_object_id(&object_id)
                ));

                const MAX_META_SHARD_SIZE: usize = 16 * 1024;
                match ObjectRef::fetch(service_id, &object_id, MAX_META_SHARD_SIZE, 3) {
                    Some((_object_ref, payload)) => {
                        // Payload includes 8-byte header: [ld (1B)][prefix56 (7B)][merkle_root + entries...]
                        match deserialize_meta_shard_with_id_validated(&payload, &object_id, service_id) {
                            MetaShardDeserializeResult::ValidatedHeader(_shard_id, shard) => {
                                log_info(&format!(
                                    "  ✅ Fetched {} entries from DA (was cached as empty)",
                                    shard.entries.len()
                                ));
                                shard
                            }
                            MetaShardDeserializeResult::ValidationFailed => {
                                log_info("  ⚠️  Deserialization/validation failed, using empty cache");
                                cached.clone()
                            }
                        }
                    }
                    None => {
                        log_info("  Meta-shard not in DA (using empty cache)");
                        cached.clone()
                    }
                }
            } else {
                // Non-empty cached shard is valid
                cached.clone()
            }
        } else {
            // Not in cache - fetch from DA
            let object_id = meta_shard_object_id(service_id, shard_id.ld, &shard_id.prefix56);

            log_info(&format!(
                "🔍 Shard not cached, fetching from DA: object_id={}",
                crate::sharding::format_object_id(&object_id)
            ));

            const MAX_META_SHARD_SIZE: usize = 16 * 1024;
            match ObjectRef::fetch(service_id, &object_id, MAX_META_SHARD_SIZE, 3) {
                Some((_object_ref, payload)) => {
                    // Payload includes 8-byte header: [ld (1B)][prefix56 (7B)][merkle_root + entries...]
                    // Use deserialize_meta_shard_with_id_validated to strip header and validate
                    match deserialize_meta_shard_with_id_validated(&payload, &object_id, service_id) {
                        MetaShardDeserializeResult::ValidatedHeader(_shard_id, shard) => {
                            log_info(&format!("  ✅ Fetched {} entries from DA", shard.entries.len()));
                            shard
                        }
                        MetaShardDeserializeResult::ValidationFailed => {
                            log_info("  ⚠️  Deserialization/validation failed, using empty");
                            MetaShard {
                                merkle_root: [0u8; 32],
                                entries: Vec::new(),
                            }
                        }
                    }
                }
                None => {
                    log_info("  Meta-shard not in DA (new)");
                    MetaShard {
                        merkle_root: [0u8; 32],
                        entries: Vec::new(),
                    }
                }
            }
        };

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

        // Compute BMT root over entries for Merkle proofs
        shard_data.merkle_root = compute_entries_bmt_root(&shard_data.entries);

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
                    for (leaf_id, mut leaf_data) in leaf_shards {
                        // Compute BMT root for each leaf shard
                        leaf_data.merkle_root = compute_entries_bmt_root(&leaf_data.entries);

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
                    write_intents.push(MetaShardWriteIntent { shard_id, entries });
                }
            }
        } else {
            // Under threshold, just write updated shard
            let entries = shard_data.entries.clone();
            cached_meta_shards.insert(shard_id, shard_data);
            write_intents.push(MetaShardWriteIntent { shard_id, entries });
        }
    }

    // log_info(&format!(
    //     "✅ Generated {} meta-shard write intents",
    //     write_intents.len()
    // ));

    write_intents
}

/// Result type for meta-shard deserialization with security validation
pub enum MetaShardDeserializeResult {
    /// Successfully deserialized with validated header
    ValidatedHeader(ShardId, MetaShard),
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
    /*
    use utils::functions::log_info;
    // Log detailed meta-shard entries before serialization
    for (i, entry) in shard.entries.iter().enumerate() {
        use utils::functions::{format_object_id, log_debug};
        log_debug(&format!(
            "  Entry {}: object_id={}, wph={}, idx_start={}, payload_len={}, kind={}",
            i,
            format_object_id(entry.object_id),
            format_object_id(entry.object_ref.work_package_hash),
            entry.object_ref.index_start,
            entry.object_ref.payload_length,
            entry.object_ref.object_kind
        ));
    }
    */
    let shard_data = da::serialize_shard(shard, META_SHARD_MAX_ENTRIES);

    // log_info(&format!(
    //     "📦 serialize_meta_shard_with_id: ld={}, shard_data_len={}, total_with_header={}",
    //     shard_id.ld,
    //     shard_data.len(),
    //     8 + shard_data.len()
    // ));

    data.extend_from_slice(&shard_data);
    data
}

/// Deserialize meta-shard from DA format with shard_id header and validate
///
/// **Security**: Validates header (ld, prefix56) against expected object_id
/// Rejects payloads where header disagrees with object_id to prevent routing corruption
///
/// **Tri-state return**:
/// - `ValidatedHeader(shard_id, shard)`: Header validated successfully
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
        log_error("❌ Meta-shard payload too short for header");
        return MetaShardDeserializeResult::ValidationFailed;
    }

    // Extract shard_id from header
    let ld = data[0];
    let mut prefix56 = [0u8; 7];
    prefix56.copy_from_slice(&data[1..8]);
    let shard_id = ShardId { ld, prefix56 };

    // CRITICAL: Validate header against object_id
    // Recompute object_id from header and verify it matches
    let computed_object_id = meta_shard_object_id(service_id, ld, &prefix56);
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

/// Rebuild MetaSSR routing table from imported meta-shard ShardIds
///
/// After importing meta-shards from witnesses, we need to reconstruct the SSR
/// routing table so that object writes route to the correct split shards.
///
/// # Critical Fix
/// We must build the COMPLETE chain from global_depth to each leaf, not just
/// the last step. If intermediate shards have been split away, we need entries
/// at every depth where a split occurred.
///
/// # Algorithm
/// 1. Find minimum ld across all shards → global_depth
/// 2. For each leaf shard, trace the full path from global_depth to leaf
/// 3. At each depth transition, check if BOTH children exist (indicating a split)
/// 4. Record each split point as an SSREntryMeta
///
/// # Example (all intermediates split away)
/// Imported shards: {ld=3, prefix=0x00...}, {ld=3, prefix=0x20...}, ...
/// → global_depth = 3
/// → Must create entries for EVERY step in the chain:
///    {d: 0, ld: 1, prefix: 0x00...} (split at depth 0)
///    {d: 1, ld: 2, prefix: 0x00...} (split at depth 1)
///    {d: 2, ld: 3, prefix: 0x00...} (split at depth 2)
///
/// Without the full chain, resolve_shard_id() stops at global_depth and produces
/// an object_id that doesn't exist, causing all writes to pile into obsolete shards.
pub fn rebuild_meta_ssr_from_shards(shard_ids: &[da::ShardId]) -> MetaSSR {
    use alloc::collections::BTreeSet;

    if shard_ids.is_empty() {
        return MetaSSR::new();
    }

    // Find global_depth (minimum ld) - this is the deepest level with a shard
    let global_depth = shard_ids.iter().map(|s| s.ld).min().unwrap_or(0);

    // Build set of all unique (ld, prefix) pairs for EXISTING shards
    let mut existing_shards: BTreeSet<(u8, [u8; 7])> = BTreeSet::new();
    for shard_id in shard_ids {
        let masked_prefix = da::mask_prefix56(shard_id.prefix56, shard_id.ld);
        existing_shards.insert((shard_id.ld, masked_prefix));
    }

    // Build set of ALL nodes (existing + inferred ancestors) in the tree
    // We need this to identify where splits occurred
    let mut all_nodes: BTreeSet<(u8, [u8; 7])> = existing_shards.clone();

    // For each existing shard, add all ancestors from global_depth up to parent
    for &(leaf_ld, leaf_prefix) in &existing_shards {
        let mut current_ld = leaf_ld;
        while current_ld > global_depth {
            current_ld -= 1;
            let ancestor_prefix = da::mask_prefix56(leaf_prefix, current_ld);
            all_nodes.insert((current_ld, ancestor_prefix));
        }
    }

    // Now identify split points: a node at depth d split if BOTH children exist
    let mut ssr_entries = Vec::new();

    for &(parent_ld, parent_prefix) in &all_nodes {
        // Skip if we're at maximum depth
        if parent_ld >= 255 {
            continue;
        }

        let child_ld = parent_ld + 1;

        // Compute both children's prefixes
        let left_prefix = parent_prefix; // Left child keeps same prefix (bit=0)

        let mut right_prefix = parent_prefix;
        let split_bit = parent_ld as usize;
        if split_bit < 56 {
            let byte_idx = split_bit / 8;
            let bit_idx = split_bit % 8;
            if byte_idx < 7 {
                right_prefix[byte_idx] |= 0x80 >> bit_idx; // Right child sets bit=1
            }
        }

        // Check if both children exist in the tree
        let left_masked = da::mask_prefix56(left_prefix, child_ld);
        let right_masked = da::mask_prefix56(right_prefix, child_ld);

        let left_exists = all_nodes.contains(&(child_ld, left_masked));
        let right_exists = all_nodes.contains(&(child_ld, right_masked));

        // If both children exist, this parent split
        if left_exists && right_exists {
            // Add SSR entry: "at depth parent_ld with prefix parent_prefix, split to child_ld"
            ssr_entries.push(da::SSREntryMeta {
                d: parent_ld,
                ld: child_ld,
                prefix56: parent_prefix,
            });
        }
    }

    // Sort SSR entries by depth for cleaner debug output
    ssr_entries.sort_by_key(|e| (e.d, e.prefix56));

    let header = da::SSRHeader {
        global_depth,
        entry_count: ssr_entries.len() as u32,
        total_keys: 0, // Unknown without loading shard data
        version: 1,
    };

    log_info(&format!(
        "🔄 Rebuilt MetaSSR: global_depth={}, {} existing shards, {} split entries",
        global_depth,
        existing_shards.len(),
        ssr_entries.len()
    ));

    MetaSSR::from_parts(header, ssr_entries)
}

// ===== ObjectID Construction for Meta-Shards =====

/// Compute ObjectID for a meta-shard from ld and prefix bytes
///
pub fn meta_shard_object_id(_service_id: u32, ld: u8, prefix56: &[u8; 7]) -> [u8; 32] {
    // Object ID format used in JAM State:
    // [ld (1B)] || [prefix bytes ((ld+7)/8)] || zero padding
    let mut object_id = [0u8; 32];
    object_id[0] = ld;

    let prefix_bytes = ((ld + 7) / 8) as usize;
    if prefix_bytes > 0 {
        object_id[1..1 + prefix_bytes].copy_from_slice(&prefix56[..prefix_bytes]);
    }

    object_id
}

/// Extract ld and prefix56 from a meta-shard object_id
pub fn parse_meta_shard_object_id(object_id: &[u8; 32]) -> (u8, [u8; 7]) {
    let ld = object_id[0];
    let prefix_bytes = ((ld + 7) / 8) as usize;
    let mut prefix56 = [0u8; 7];
    if prefix_bytes > 0 {
        prefix56[..prefix_bytes].copy_from_slice(&object_id[1..1 + prefix_bytes]);
    }
    (ld, prefix56)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_object_ref_entry_serialization() {
        let entry = ObjectRefEntry {
            object_id: [0x42; 32],
            object_ref: ObjectRef {
                work_package_hash: [0xBB; 32],
                index_start: 10,
                payload_length: 100,
                object_kind: 1,
            },
        };

        let serialized = entry.serialize();
        assert_eq!(serialized.len(), 69);

        let deserialized = ObjectRefEntry::deserialize(&serialized).unwrap();
        assert_eq!(deserialized.object_id, entry.object_id);
        assert_eq!(
            deserialized.object_ref.work_package_hash,
            entry.object_ref.work_package_hash
        );
        assert_eq!(
            deserialized.object_ref.index_start,
            entry.object_ref.index_start
        );
    }

    #[test]
    fn test_meta_shard_capacity() {
        // Verify our capacity calculations
        let entry_size = ObjectRefEntry::serialized_size();
        assert_eq!(entry_size, 69);

        // DA segment: 4104 bytes total, 34 bytes for merkle_root + count header
        let available = 4104 - 34;
        let max_entries = available / entry_size;
        assert_eq!(max_entries, 59); // (4070 / 69 ≈ 59.0)

        // We use 58 to have some buffer
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

        let intents = process_object_writes(writes, &mut cached_shards, &mut meta_ssr, 0);

        assert_eq!(intents.len(), 1);
        assert_eq!(intents[0].entries.len(), 1);
        assert_eq!(intents[0].entries[0].object_id, [0x01; 32]);
    }
}
