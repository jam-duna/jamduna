//! Block Accumulator - manages block finalization and storage operations

use crate::mmr::MMR;
use crate::{
    block::{EvmBlockPayload, block_number_to_object_id},
    sharding::format_object_id,
};
use alloc::{format, vec::Vec};
use utils::{
    constants::FULL,
    functions::{AccumulateInput, format_segment, log_error, log_info},
};

/// BlockAccumulator manages block storage and retrieval operations
pub struct BlockAccumulator {
    #[allow(dead_code)]
    pub service_id: u32,
    pub timeslot: u32,
    pub block_number: u32,
    pub mmr: MMR,
}

impl BlockAccumulator {
    /// Accumulate all rollup blocks and finalize
    ///
    /// Processes each work package by:
    /// 1. Deserializing compact meta-shard format from refine output
    /// 2. Writing MetaSSR (ld byte) to special key
    /// 3. Writing MetaShard ObjectRefs to JAM State
    /// 4. Writing block mappings (blocknumber ↔ work_package_hash)
    /// 5. Appending work_package_hash to MMR
    ///
    /// Returns the MMR root committing to all blocks processed so far
    pub fn accumulate(
        service_id: u32,
        timeslot: u32,
        accumulate_inputs: &[AccumulateInput],
    ) -> Option<[u8; 32]> {
        log_info(&format!("📥 Accumulate: Processing {} inputs", accumulate_inputs.len()));
        let mut accumulator = Self::new(service_id, timeslot)?;

        for (idx, input) in accumulate_inputs.iter().enumerate() {
            log_info(&format!("📥 Processing input #{}", idx));
            let AccumulateInput::OperandElements(operand) = input else {
                log_error(&format!("  Input #{}: Not OperandElements, skipping", idx));
                continue;
            };

            log_info(&format!("  Input #{}: OperandElements, calling rollup_block", idx));
            // Process this work package's rollup block
            accumulator.rollup_block(operand)?;

            // Delete old blocks to prevent unbounded state growth
            const BLOCK_RETENTION_LIMIT: u32 = 10000;
            if accumulator.block_number > BLOCK_RETENTION_LIMIT {
                let old_block_number = accumulator.block_number - BLOCK_RETENTION_LIMIT;
                crate::block::EvmBlockPayload::delete_block(service_id, old_block_number);
            }
        }
        EvmBlockPayload::write_blocknumber_key(accumulator.block_number);
        let accumulate_root = match accumulator.mmr.write_mmr() {
            Some(root) => {
                log_info(&format!(
                    "✅ Accumulate complete. Next BN {}. Root: {}",
                    accumulator.block_number,
                    format_object_id(&root)
                ));
                root
            }
            None => {
                log_error("❌ FATAL: Failed to write MMR to JAM State - accumulate aborted");
                return None;
            }
        };
        Some(accumulate_root)
    }

    /// Process a single rollup block from refine output
    ///
    /// Deserializes compact meta-shard format and writes to JAM State:
    /// - MetaSSR (ld byte) → special key service_id || "SSR"
    /// - MetaShard ObjectRefs → indexed by meta-shard object_id
    /// - Block mapping → bidirectional (blocknumber ↔ work_package_hash)
    fn rollup_block(
        &mut self,
        operand: &utils::functions::AccumulateOperandElements,
    ) -> Option<()> {
        use utils::functions::log_info;

        // Debug: check if result.ok is None
        if operand.result.ok.is_none() {
            log_info("❌ rollup_block: operand.result.ok is None!");
            if operand.result.err.is_some() {
                log_info(&format!("  Error code: {}", operand.result.err.unwrap()));
            }
            return None;
        }

        let data = operand.result.ok.as_ref()?;
        let wph = operand.work_package_hash;

        // Parse compact format: N entries × (1B ld + prefix_bytes + 5B packed ObjectRef)
        // Note: Each entry has its own ld value (meta-shards can have different depths after splits)
        //
        // CRITICAL: Even if data.is_empty() (no meta-shard updates), we MUST still write
        // the block mapping and update the MMR. Otherwise "idle" blocks (empty or no splits)
        // vanish from the block history, breaking bidirectional lookups and finalization.

        let global_ld = if data.is_empty() {
            log_info("📥 Empty compact format, no meta-shard entries (idle block)");
            0u8 // No meta-shard updates, use ld=0
        } else {
            // log_info(&format!(
            //     "📥 Deserializing compact format: {} bytes total",
            //     data.len()
            // ));

            // Parse variable-length entries
            let mut offset = 0;
            let mut num_entries = 0;
            let mut max_ld = 0u8; // Track maximum ld for MetaSSR

            while offset < data.len() {
                if offset + 1 > data.len() {
                    log_error(&format!(
                        "❌ Invalid compact format: truncated ld at offset {}",
                        offset
                    ));
                    return None;
                }

                let ld = data[offset];
                max_ld = max_ld.max(ld);
                offset += 1;

                let prefix_bytes = ((ld + 7) / 8) as usize;
                let entry_size = prefix_bytes + 5; // prefix + 5-byte packed ObjectRef

                if offset + entry_size > data.len() {
                    log_error(&format!(
                        "❌ Invalid compact format: truncated entry at offset {} (need {} more bytes)",
                        offset, entry_size
                    ));
                    return None;
                }

                let entry_bytes = &data[offset..offset + entry_size];
                self.write_metashard(entry_bytes, ld, prefix_bytes, wph, num_entries)?;

                offset += entry_size;
                num_entries += 1;
            }

            // log_info(&format!(
            //     "📥 Deserialized {} meta-shard entries, max ld={}",
            //     num_entries, max_ld
            // ));
            max_ld
        };

        // ALWAYS write MetaSSR hint, even for idle blocks (ensures SSR key exists for Go lookups)
        self.write_metassr(global_ld);

        // ALWAYS write block mapping, even for idle blocks
        // This ensures every block appears in the bidirectional mapping and MMR
        self.write_block(wph, self.timeslot)?;

        Some(())
    }

    /// Write MetaSSR global_depth hint to JAM State special key (monotonic increase only)
    ///
    /// Key format: service_id (4 bytes) || "SSR" (3 bytes) || padding
    /// Value: single ld byte indicating maximum meta-sharding tree depth
    ///
    /// The global_depth is monotonically increasing - we only write if the new value
    /// is greater than the existing value. This ensures the hint is always valid.
    ///
    /// CRITICAL: We must detect NONE/WHAT sentinels properly. If the key is missing,
    /// we MUST write it (even if new_ld=0) so that Go's metashard_lookup.go can read it.
    fn write_metassr(&self, new_ld: u8) {
        use utils::constants::{NONE, WHAT};
        use utils::host_functions::{read, write};

        let mut ssr_key = [0u8; 32];
        ssr_key[0..4].copy_from_slice(&self.service_id.to_le_bytes());
        ssr_key[4..7].copy_from_slice(b"SSR");

        // Read current global_depth
        // read(s: service_id, ko: key_offset, kz: key_size, o: out_offset, f: out_offset_within, l: length)
        let mut buffer = [0u8; 1];
        let result = unsafe {
            read(
                self.service_id as u64,     // s: service_id
                ssr_key.as_ptr() as u64,    // ko: key offset
                ssr_key.len() as u64,       // kz: key size
                buffer.as_mut_ptr() as u64, // o: output offset
                0,                          // f: offset within (0 = start)
                buffer.len() as u64,        // l: length to read
            )
        };

        // Check for sentinel values
        let current_ld_opt = if result == NONE {
            None // Key doesn't exist yet
        } else if result == WHAT {
            log_error("❌ write_metassr: WHAT error (wrong mode)");
            return; // Shouldn't happen, but bail out
        } else if result > 0 {
            Some(buffer[0]) // Key exists, return the value
        } else {
            None // Zero bytes read = empty value, treat as missing
        };

        // Determine if we should write
        let should_write = match current_ld_opt {
            None => {
                // Key doesn't exist - MUST write even if new_ld=0
                // Otherwise Go's metashard_lookup.go will error out
                log_info(&format!(
                    "📝 Initializing MetaSSR global_depth: {} (first write)",
                    new_ld
                ));
                true
            }
            Some(current_ld) => {
                // Key exists - only write if monotonically increasing
                if new_ld > current_ld {
                    log_info(&format!(
                        "📝 Updating MetaSSR global_depth: {} → {} (monotonic increase)",
                        current_ld, new_ld
                    ));
                    true
                } else {
                    false
                }
            }
        };

        if should_write {
            unsafe {
                write(
                    ssr_key.as_ptr() as u64,
                    ssr_key.len() as u64,
                    [new_ld].as_ptr() as u64,
                    1,
                );
            }
        }
    }

    /// Deserialize and write a single MetaShard ObjectRef to JAM State
    ///
    /// Compact entry format: ld_prefix (variable) + 5-byte packed ObjectRef
    /// Packed ObjectRef: index_start (12 bits) | index_end (12 bits) | last_segment_size (12 bits) | object_kind (4 bits)
    ///
    /// Validates object_kind is MetaShard, then writes ObjectRef indexed by meta-shard object_id
    fn write_metashard(
        &self,
        entry_bytes: &[u8],
        ld: u8,
        prefix_bytes: usize,
        work_package_hash: [u8; 32],
        entry_index: usize,
    ) -> Option<()> {
        use crate::meta_sharding::meta_shard_object_id;
        use crate::sharding::ObjectKind;
        use utils::objects::ObjectRef;

        // Reconstruct meta-shard object_id from ld and prefix
        let mut prefix56 = [0u8; 7];
        if prefix_bytes > 0 {
            prefix56[..prefix_bytes].copy_from_slice(&entry_bytes[0..prefix_bytes]);
        }
        let object_id = meta_shard_object_id(self.service_id, ld, &prefix56);

        // Unpack 5-byte ObjectRef from compact format
        let packed_bytes = &entry_bytes[prefix_bytes..prefix_bytes + 5];
        let packed: u64 = ((packed_bytes[0] as u64) << 32)
            | ((packed_bytes[1] as u64) << 24)
            | ((packed_bytes[2] as u64) << 16)
            | ((packed_bytes[3] as u64) << 8)
            | (packed_bytes[4] as u64);

        let index_start = ((packed >> 28) & 0xFFF) as u16; // Bits 28-39: DA segment start
        let index_end = ((packed >> 16) & 0xFFF) as u16; // Bits 16-27: DA segment end
        let last_segment_size = ((packed >> 4) & 0xFFF) as u16; // Bits 4-15: bytes in last segment
        let object_kind = (packed & 0xF) as u8; // Bits 0-3: object kind

        // Validate object_kind is MetaShard
        if object_kind != ObjectKind::MetaShard as u8 {
            log_error(&format!(
                "❌ Entry {}: Expected MetaShard (kind={}), got kind={}",
                entry_index,
                ObjectKind::MetaShard as u8,
                object_kind
            ));
            return None;
        }

        // Reconstruct total payload_length from DA segment info
        use utils::constants::SEGMENT_SIZE;
        let num_segments = index_end.saturating_sub(index_start);
        let payload_length = if num_segments == 0 {
            0 // Empty payload
        } else if num_segments == 1 {
            last_segment_size as u32 // Single segment
        } else {
            // Multiple segments: (n-1) full segments + last partial segment
            ((num_segments - 1) as u32 * SEGMENT_SIZE as u32) + last_segment_size as u32
        };

        log_info(&format!(
            "📝 Entry {}: Writing MetaShard ld={}, object_id={}, index_start={}, payload_len={}, bytes={}",
            entry_index,
            ld,
            format_object_id(&object_id),
            index_start,
            payload_length,
            format_segment(entry_bytes)
        ));

        // Create full ObjectRef with work_package_hash and write to JAM State
        let object_ref = ObjectRef {
            work_package_hash,
            index_start,
            payload_length,
            object_kind,
        };

        let result = object_ref.write(&object_id, self.timeslot, self.block_number);
        if result == FULL as u64 {
            log_error("❌ MetaShard ObjectRef write failed: JAM State FULL");
            return None;
        }

        // If write returned NONE, this meta-shard is new (first write)
        // This happens in two cases:
        // 1. Genesis: first time writing any meta-shard
        // 2. Split: child shards are being written for the first time
        //
        // In case 2, we MUST delete all ancestor shards at lower depths to avoid
        // ambiguity during probe-backwards lookup. Otherwise lookup would find the
        // stale parent shard instead of the correct child shard!
        use utils::constants::NONE;
        if result == NONE {
            log_info(&format!(
                "🆕 First write for meta-shard ld={} - deleting ancestor shards",
                ld
            ));
            self.delete_ancestor_shards(&object_id, ld)?;
        }

        // log_info(&format!(
        //     "✅ MetaShard ObjectRef written successfully, result={}",
        //     result
        // ));

        Some(())
    }

    /// Delete all ancestor meta-shards at lower depths to prevent stale lookups
    ///
    /// When a meta-shard splits, we must delete the parent shard at `ld-1`, `ld-2`, ... 0
    /// that could match this object_id's prefix. Otherwise probe-backwards lookup would
    /// find the stale parent instead of the correct child shard.
    ///
    /// Algorithm:
    /// - Start at `ld-1` and work down to 0
    /// - For each depth, compute the ancestor object_id by masking the prefix
    /// - Delete that key from JAM State
    fn delete_ancestor_shards(&self, object_id: &[u8; 32], current_ld: u8) -> Option<()> {
        use crate::meta_sharding::meta_shard_object_id;
        use utils::host_functions::write;

        if current_ld == 0 {
            // No ancestors to delete
            return Some(());
        }

        // Extract ld and prefix56 from new 0xAA-prefixed object_id format
        let (current_ld_parsed, prefix56) = crate::meta_sharding::parse_meta_shard_object_id(&object_id);
        debug_assert_eq!(current_ld_parsed, current_ld, "ld mismatch in delete_ancestor_shards");

        for ancestor_ld in (0..current_ld).rev() {
            // Compute ancestor prefix by masking to ancestor_ld bits
            let prefix_bytes = ((ancestor_ld + 7) / 8) as usize;
            let mut masked_prefix = [0u8; 7];

            if prefix_bytes > 0 {
                masked_prefix[..prefix_bytes].copy_from_slice(&prefix56[..prefix_bytes]);

                // Mask the last byte to clear unused bits
                let remaining_bits = ancestor_ld % 8;
                if remaining_bits > 0 && prefix_bytes > 0 {
                    let mask = 0xFF << (8 - remaining_bits);
                    masked_prefix[prefix_bytes - 1] &= mask;
                }
            }

            let ancestor_object_id =
                meta_shard_object_id(self.service_id, ancestor_ld, &masked_prefix);

            log_info(&format!(
                "🗑️  Deleting ancestor meta-shard: ld={}, object_id={}",
                ancestor_ld,
                format_object_id(&ancestor_object_id)
            ));

            // Delete by writing empty value (JAM State semantics)
            unsafe {
                write(
                    ancestor_object_id.as_ptr() as u64,
                    ancestor_object_id.len() as u64,
                    0, // NULL pointer = delete
                    0, // 0 length = delete
                );
            }
        }

        Some(())
    }

    /// Initialize BlockAccumulator by reading state from JAM State
    ///
    /// Reads:
    /// - block_number: Next block number to write (defaults to 1 if not found)
    /// - MMR: Merkle Mountain Range of all previous blocks (creates new if genesis)
    fn new(service_id: u32, timeslot: u32) -> Option<Self> {
        log_info(&format!(
            "🏭 Creating BlockAccumulator for service_id={}, timeslot={}",
            service_id, timeslot
        ));

        // Read current block number from JAM State
        let block_number = match EvmBlockPayload::read_blocknumber_key(service_id) {
            Some(block_num) => {
                log_info(&format!(
                    "📊 Read block_number={} from JAM State",
                    block_num
                ));
                block_num
            }
            None => {
                log_error(&format!(
                    "❌ Failed to read block_number for service_id={}, defaulting to 1",
                    service_id
                ));
                1
            }
        };

        // Read MMR from JAM State (or create new if genesis)
        let mmr = MMR::read_mmr(service_id).unwrap_or_else(|| {
            log_info("📋 Creating new MMR (genesis)");
            MMR::new()
        });

        Some(BlockAccumulator {
            service_id,
            timeslot,
            block_number,
            mmr,
        })
    }

    /// Write bidirectional block mappings to JAM State and update MMR
    ///
    /// Writes:
    /// - blocknumber → (work_package_hash, timeslot)
    /// - work_package_hash → (blocknumber, timeslot)
    ///
    /// Then appends work_package_hash to MMR and increments block_number
    ///
    /// Returns None if any write fails (FULL), Some(()) on success
    fn write_block(&mut self, work_package_hash: [u8; 32], timeslot: u32) -> Option<()> {
        use utils::host_functions::write as host_write;

        // Write blocknumber → work_package_hash (32 bytes) + timeslot (4 bytes)
        let bn_key = block_number_to_object_id(self.block_number);
        let mut bn_data = Vec::with_capacity(36);
        bn_data.extend_from_slice(&work_package_hash);
        bn_data.extend_from_slice(&timeslot.to_le_bytes());

        let result1 = unsafe {
            host_write(
                bn_key.as_ptr() as u64,
                bn_key.len() as u64,
                bn_data.as_ptr() as u64,
                bn_data.len() as u64,
            )
        };

        // Validate first write succeeded
        if result1 == FULL {
            log_error(&format!(
                "❌ Failed to write blocknumber→wph mapping for block {}: JAM State FULL",
                self.block_number
            ));
            return None;
        }

        // Write reverse mapping: work_package_hash → blocknumber (4 bytes) + timeslot (4 bytes)
        let wph_key = work_package_hash;
        let mut wph_data = Vec::with_capacity(8);
        wph_data.extend_from_slice(&self.block_number.to_le_bytes());
        wph_data.extend_from_slice(&timeslot.to_le_bytes());

        let result2 = unsafe {
            host_write(
                wph_key.as_ptr() as u64,
                wph_key.len() as u64,
                wph_data.as_ptr() as u64,
                wph_data.len() as u64,
            )
        };

        // Validate second write succeeded
        if result2 == FULL {
            log_error(&format!(
                "❌ Failed to write wph→blocknumber mapping for block {}: JAM State FULL",
                self.block_number
            ));
            return None;
        }

        // Both writes succeeded - log and update internal state
        log_info(&format!(
            "📚 Wrote block mappings: {} ↔ {} (@ timeslot {})",
            self.block_number,
            format_object_id(&work_package_hash),
            timeslot
        ));

        // Update MMR with this block's work_package_hash
        self.mmr.append(work_package_hash);

        // Increment block_number for next block
        self.block_number += 1;

        Some(())
    }
}
