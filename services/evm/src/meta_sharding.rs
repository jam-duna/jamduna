//! Meta-Sharding for JAM State Object Registry
//!
//! # Overview
//!
//! This module implements adaptive sharding for ObjectRef entries stored in JAM State.
//! It manages millions of ObjectRefs without bloating JAM State or exceeding W_R limits
//! through a two-level sharding architecture.
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
//! - Each meta-shard segment: 4104 bytes with 69-byte entries
//! - Max 58 ObjectRefEntry per meta-shard (splits at 29 entries)
//! - Uses binary tree splitting with SSR routing metadata
//!
//! # Data Structures
//!
//! - `ObjectRefEntry`: ObjectID (32 bytes) + ObjectRef (37 bytes) = 69 bytes
//! - `MetaSSR`: Type alias for `SSRData` - routing table for split meta-shards
//! - `MetaShard`: Container for ObjectRefEntry mappings with BMT root
//!
//! # Core Methods
//!
//! **MetaShard:**
//! - `serialize()` / `deserialize()`: Basic DA format (merkle_root + entries)
//! - `serialize_with_id()` / `deserialize_with_id_validated()`: DA format with shard_id header
//! - `maybe_split()`: Recursive binary splitting when threshold exceeded
//! - `split_once()`: Single binary split operation
//!
//! **Module Functions:**
//! - `process_object_writes()`: Groups ObjectRef writes by meta-shard, handles splits
//! - `rebuild_meta_ssr()`: Reconstructs SSR routing table from imported shard IDs
//!
//! # Scaling Benefits
//!
//! - JAM State size: 72MB (vs 720MB without meta-sharding)
//! - Work report capacity: 414 meta-shard updates/WP × 1K ObjectRefs/meta-shard = **414K ObjectRef updates/WP**
//! - Throughput: 141K meta-shard updates/block across 341 cores

use alloc::collections::BTreeMap;
use alloc::format;
use alloc::vec;
use alloc::vec::Vec;
use utils::functions::log_info;
use utils::objects::ObjectRef;

// Import shared types and helper functions from da module
use crate::da::{self, SplitPoint, SerializeError, ShardId, take_prefix56, mask_prefix56, matches_prefix};

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

// ObjectRefEntry serialization functions are now standalone (no trait)
impl ObjectRefEntry {
    /// Serialize to DA format: 32 bytes object_id + 37 bytes ObjectRef = 69 bytes total
    pub fn serialize(&self) -> Vec<u8> {
        let mut buf = Vec::with_capacity(69);
        buf.extend_from_slice(&self.object_id);
        buf.extend_from_slice(&self.object_ref.serialize());
        buf
    }

    /// Deserialize from DA format
    pub fn deserialize(data: &[u8]) -> Result<Self, SerializeError> {
        if data.len() != 69 {
            return Err(SerializeError::InvalidSize);
        }

        let mut object_id = [0u8; 32];
        object_id.copy_from_slice(&data[0..32]);

        let object_ref =
            ObjectRef::deserialize(&data[32..69]).ok_or(SerializeError::InvalidObjectRef)?;

        Ok(ObjectRefEntry {
            object_id,
            object_ref,
        })
    }
}

// ===== Meta-Sharding SSR Structures =====

/// MetaSSR - sparse split registry for meta-sharding
///
/// Runtime-only routing structure that tracks where meta-shards have been split.
/// Never serialized to DA or JAM State - reconstructed from MetaShard ObjectIDs.
///
/// **USED BY**: Meta-sharding ONLY (contract sharding has its own SSRData in contractsharding.rs)
#[derive(Clone)]
pub struct MetaSSR {
    pub global_depth: u8,           // Default depth for all meta-shards
    pub entries: Vec<SplitPoint>, // Sorted split point exceptions
}

impl MetaSSR {
    pub fn new() -> Self {
        Self {
            global_depth: 0,
            entries: Vec::new(),
        }
    }

    pub fn from_parts(global_depth: u8, entries: Vec<SplitPoint>) -> Self {
        Self {
            global_depth,
            entries,
        }
    }
}

/// Meta-shard data containing ObjectRefEntry mappings
#[derive(Clone)]
pub struct MetaShard {
    pub merkle_root: [u8; 32],       // BMT root over entries for inclusion proofs
    pub entries: Vec<ObjectRefEntry>, // Sorted by object_id
}

impl MetaShard {
    /// Serialize meta-shard data to DA format
    ///
    /// Format: [32B merkle_root][2B count][ObjectRefEntry...]
    /// Each ObjectRefEntry: 69 bytes (32B object_id + 37B ObjectRef)
    pub fn serialize(&self, max_entries: usize) -> Vec<u8> {
        use utils::functions::log_crit;

        if self.entries.len() > max_entries {
            log_crit(&format!(
                "❌ FATAL: Meta-shard has {} entries, exceeds limit of {}!",
                self.entries.len(),
                max_entries
            ));
            #[cfg(debug_assertions)]
            panic!(
                "Meta-shard entry count exceeds limit: {} > {}",
                self.entries.len(),
                max_entries
            );
        }

        let mut result = Vec::new();
        result.extend_from_slice(&self.merkle_root);
        let count = self.entries.len() as u16;
        result.extend_from_slice(&count.to_le_bytes());

        for entry in &self.entries {
            result.extend_from_slice(&entry.serialize());
        }

        result
    }

    /// Deserialize meta-shard data from DA format
    pub fn deserialize(data: &[u8]) -> Result<Self, SerializeError> {
        if data.len() < 34 {
            return Err(SerializeError::InvalidSize);
        }

        let mut merkle_root = [0u8; 32];
        merkle_root.copy_from_slice(&data[0..32]);

        let count = u16::from_le_bytes([data[32], data[33]]) as usize;
        let entry_size = 69; // 32B object_id + 37B ObjectRef
        let expected_len = 34 + (count * entry_size);

        if data.len() < expected_len {
            return Err(SerializeError::InvalidSize);
        }

        let mut entries = Vec::new();
        let mut offset = 34;
        for _ in 0..count {
            let entry = ObjectRefEntry::deserialize(&data[offset..offset + entry_size])?;
            entries.push(entry);
            offset += entry_size;
        }

        Ok(MetaShard {
            merkle_root,
            entries,
        })
    }

    /// Perform a single binary split of this meta-shard
    ///
    /// Splits the shard into left and right children based on one bit.
    /// Used by recursive splitting logic for ObjectRefEntry shards.
    ///
    /// # Returns
    /// (left_shard_id, left_shard, right_shard_id, right_shard, split_point)
    fn split_once(
        self,
        shard_id: ShardId,
    ) -> (ShardId, MetaShard, ShardId, MetaShard, SplitPoint) {
        assert!(shard_id.ld < 255, "Cannot split shard at maximum depth");
        let new_depth = shard_id.ld + 1;
        let split_bit = shard_id.ld as usize;

        // Split entries based on the next bit position
        let mut left_entries = Vec::new();
        let mut right_entries = Vec::new();

        for entry in self.entries {
            let key_hash = entry.object_id;
            let key_prefix = take_prefix56(&key_hash);

            if split_bit < 56 {
                let byte_idx = split_bit / 8;
                let bit_idx = split_bit % 8;
                let bit_mask = 0x80 >> bit_idx;

                if byte_idx < 7 && (key_prefix[byte_idx] & bit_mask) == 0 {
                    left_entries.push(entry);
                } else {
                    right_entries.push(entry);
                }
            } else {
                // If depth exceeds 56 bits, put all in left shard
                left_entries.push(entry);
            }
        }

        let left_shard = MetaShard {
            merkle_root: [0u8; 32], // Will be computed by caller (type-specific BMT)
            entries: left_entries,
        };
        let right_shard = MetaShard {
            merkle_root: [0u8; 32], // Will be computed by caller (type-specific BMT)
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

        // Create split point metadata for the split point
        let split_point = SplitPoint {
            d: shard_id.ld,              // Split point (previous depth)
            ld: new_depth,               // New local depth
            prefix56: shard_id.prefix56, // Original routing prefix
        };

        (
            left_shard_id,
            left_shard,
            right_shard_id,
            right_shard,
            split_point,
        )
    }

    /// Recursively split this meta-shard until all result shards meet threshold
    ///
    /// # Algorithm
    /// 1. Use work queue to process shards that need splitting
    /// 2. For each shard > threshold, perform binary split
    /// 3. Check if children still exceed threshold and queue them for further splitting
    /// 4. Continue until all result shards ≤ threshold
    ///
    /// # Returns
    /// Some((leaf_shards, split_entries)) where:
    /// - leaf_shards: Vec of (ShardId, MetaShard) for all result leaves
    /// - split_entries: Vec of SplitPoint for internal split points (N leaves → N-1 entries)
    pub fn maybe_split(
        self,
        shard_id: ShardId,
        threshold: usize,
    ) -> Option<(Vec<(ShardId, MetaShard)>, Vec<SplitPoint>)> {
        if self.entries.len() <= threshold {
            return None;
        }

        log_info(&format!(
            "🔀 Recursive split starting: {} entries in shard ld={}",
            self.entries.len(),
            shard_id.ld
        ));

        let mut leaf_shards = Vec::new();
        let mut work_queue = vec![(shard_id, self)];
        let mut split_entries = Vec::new();

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
            let (left_id, left_data, right_id, right_data, split_entry) =
                current_data.split_once(current_id);

            let total_entries = left_data.entries.len() + right_data.entries.len();
            log_info(&format!(
                "  Split ld={} → ld={}: {} entries → [{}, {}]",
                current_id.ld,
                left_id.ld,
                total_entries,
                left_data.entries.len(),
                right_data.entries.len()
            ));

            // Record split point for this internal split point
            split_entries.push(split_entry);

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
            "✅ Recursive split complete: {} leaf shards, {} split points",
            leaf_shards.len(),
            split_entries.len()
        ));

        Some((leaf_shards, split_entries))
    }

    /// Serialize meta-shard to DA format with shard_id header
    ///
    /// Format: [1B ld][7B prefix56][...shard entries]
    /// The shard_id header enables witness imports to identify the shard without SSR routing
    pub fn serialize_with_id(&self, shard_id: ShardId, max_entries: usize) -> Vec<u8> {
        let mut data = Vec::new();
        data.push(shard_id.ld);
        data.extend_from_slice(&shard_id.prefix56);
        /*
        use utils::functions::log_info;
        // Log detailed meta-shard entries before serialization
        for (i, entry) in self.entries.iter().enumerate() {
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
        let shard_data = self.serialize(max_entries);

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
    pub fn deserialize_with_id_validated(
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
        match MetaShard::deserialize(&data[8..]) {
            Ok(shard) => MetaShardDeserializeResult::ValidatedHeader(shard_id, shard),
            Err(_) => {
                // Header validated but shard data corrupt
                log_error("❌ Meta-shard header valid but shard data corrupt");
                MetaShardDeserializeResult::ValidationFailed
            }
        }
    }
}

/// Meta-shard split threshold - split when exceeding this many entries (~50% of max)
pub const META_SHARD_SPLIT_THRESHOLD: usize = 29;

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
    payload_type: u8,
) -> Vec<MetaShardWriteIntent> {
    if object_writes.is_empty() {
        return Vec::new();
    }

    // Log all object_writes contents
    log_info(&format!(
        "📦 Processing {} object writes for meta-sharding:",
        object_writes.len()
    ));
    for (idx, (object_id, object_ref)) in object_writes.iter().enumerate() {
        log_info(&format!(
            "  [{}] object_id={}, object_ref={{wph: {}, idx_start: {}, payload_len: {}, kind: {}}}",
            idx,
            crate::contractsharding::format_object_id(object_id),
            crate::contractsharding::format_object_id(&object_ref.work_package_hash),
            object_ref.index_start,
            object_ref.payload_length,
            object_ref.object_kind
        ));
    }

    // Log meta_ssr state
    log_info(&format!(
        "📦 MetaSSR state: global_depth={}, {} entries",
        meta_ssr.global_depth,
        meta_ssr.entries.len()
    ));
    for (idx, entry) in meta_ssr.entries.iter().enumerate() {
        log_info(&format!(
            "  Split Point [{}]: d={}, ld={}, prefix56={:?}",
            idx,
            entry.d,
            entry.ld,
            &entry.prefix56[..]
        ));
    }

    // 1. Group object writes by meta-shard
    let mut shard_updates: BTreeMap<ShardId, Vec<ObjectRefEntry>> = BTreeMap::new();

    for (object_id, object_ref) in object_writes {
        use crate::contractsharding::ObjectKind;

        // Phase 7: Filter out ObjectKind::StorageShard and ObjectKind::Code
        // Witnesses now provide complete state upfront, no need to track in meta-shards
        let kind = ObjectKind::from_u8(object_ref.object_kind);
        match kind {
            Some(ObjectKind::StorageShard) | Some(ObjectKind::Code) => {
                // Skip - witnesses handle these
                continue;
            }
            Some(ObjectKind::Block) => {
                // Skip blocks (already filtered)
                continue;
            }
            _ => {
                // Keep receipts, meta-shards, and other metadata
            }
        }

        let shard_id = resolve_meta_shard_id(object_id, meta_ssr);

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


                const MAX_META_SHARD_SIZE: usize = 16 * 1024;
                match ObjectRef::fetch(service_id, &object_id, MAX_META_SHARD_SIZE, payload_type) {
                    Some((_object_ref, payload)) => {
                        // Payload includes 8-byte header: [ld (1B)][prefix56 (7B)][merkle_root + entries...]
                        match MetaShard::deserialize_with_id_validated(&payload, &object_id, service_id) {
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


            const MAX_META_SHARD_SIZE: usize = 16 * 1024;
            match ObjectRef::fetch(service_id, &object_id, MAX_META_SHARD_SIZE, payload_type) {
                Some((_object_ref, payload)) => {
                    // Payload includes 8-byte header: [ld (1B)][prefix56 (7B)][merkle_root + entries...]
                    match MetaShard::deserialize_with_id_validated(&payload, &object_id, service_id) {
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

        // Check if shard needs splitting
        match shard_data.clone().maybe_split(shard_id, META_SHARD_SPLIT_THRESHOLD) {
            Some((leaf_shards, split_entries)) => {
                log_info(&format!(
                    "  ✅ Split complete: {} leaf shards, {} split points",
                    leaf_shards.len(),
                    split_entries.len()
                ));

                // Update MetaSSR with new split entries
                for split_entry in split_entries {
                    meta_ssr.entries.push(split_entry);
                }
                meta_ssr.entries.sort();

                // Sanity check: Warn if entry count is unusually high
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
/// 4. Record each split point as an SplitPoint
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
    let mut split_entries = Vec::new();

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
            // Add split point: "at depth parent_ld with prefix parent_prefix, split to child_ld"
            split_entries.push(da::SplitPoint {
                d: parent_ld,
                ld: child_ld,
                prefix56: parent_prefix,
            });
        }
    }

    // Sort split points by depth for cleaner debug output
    split_entries.sort_by_key(|e| (e.d, e.prefix56));

    log_info(&format!(
        "🔄 Rebuilt MetaSSR: global_depth={}, {} existing shards, {} split entries",
        global_depth,
        existing_shards.len(),
        split_entries.len()
    ));

    MetaSSR::from_parts(global_depth, split_entries)
}

// ===== Meta-Shard Resolution & Splitting =====

/// Resolve which meta-shard contains a given object_id by walking the MetaSSR tree.
///
/// This is specifically for meta-sharding (ObjectRefEntry routing), NOT contract storage.
///
/// # Algorithm
/// 1. Start with global_depth from MetaSSR header
/// 2. Extract 56-bit prefix from object_id
/// 3. Walk Metasplit points: if we find a split at current depth matching our prefix, descend
/// 4. Return ShardId with resolved depth and masked prefix
///
/// # Returns
/// ShardId with (ld, prefix56) identifying the target meta-shard
pub fn resolve_meta_shard_id(object_id: [u8; 32], meta_ssr: &MetaSSR) -> ShardId {
    let key_prefix56 = take_prefix56(&object_id);

    // Start at the global depth (root meta-shard) and walk down the split tree
    let mut current_ld = meta_ssr.global_depth;
    let mut current_prefix = mask_prefix56(key_prefix56, current_ld);

    loop {
        let mut found_split = false;

        for entry in &meta_ssr.entries {
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
        let entry_size = 69; // ObjectRefEntry serialized size: 32B object_id + 37B ObjectRef
        assert_eq!(entry_size, 69);

        // DA segment: 4104 bytes total, 34 bytes for merkle_root + count header
        let available = 4104 - 34;
        let max_entries = available / entry_size;
        assert_eq!(max_entries, 58); // (4070 / 69 = 58.99)

        // We use 58 to have some buffer
        assert!(META_SHARD_MAX_ENTRIES <= max_entries);
    }

}
