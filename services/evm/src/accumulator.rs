//! Block Accumulator - manages block finalization and storage operations

use crate::mmr::MMR;
use crate::{
    block::{EvmBlockPayload, block_number_to_object_id},
    contractsharding::format_object_id,
};
use alloc::{format, vec::Vec};
use utils::{
    constants::FULL,
    functions::{AccumulateInput, format_segment, log_error, log_info},
    host_functions::AccumulateInstruction,
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
        log_info(&format!(
            "📥 Accumulate: Processing {} inputs",
            accumulate_inputs.len()
        ));
        let mut accumulator = Self::new(service_id, timeslot)?;

        for (idx, input) in accumulate_inputs.iter().enumerate() {
            log_info(&format!("📥 Processing input #{}", idx));

            match input {
                AccumulateInput::OperandElements(operand) => {
                    log_info(&format!(
                        "  Input #{}: OperandElements, calling rollup_block",
                        idx
                    ));

                    // Process this work package's rollup block
                    accumulator.rollup_block(operand)?;

                    // Delete old blocks to prevent unbounded state growth
                    const BLOCK_RETENTION_LIMIT: u32 = 10000;
                    if accumulator.block_number > BLOCK_RETENTION_LIMIT {
                        let old_block_number = accumulator.block_number - BLOCK_RETENTION_LIMIT;
                        crate::block::EvmBlockPayload::delete_block(service_id, old_block_number);
                    }
                }
                AccumulateInput::DeferredTransfer(transfer) => {
                    log_info(&format!(
                        "  Input #{}: DeferredTransfer from service {}, amount={}",
                        idx, transfer.sender_index, transfer.amount
                    ));

                    // Process Railgun withdrawal
                    accumulator.process_railgun_withdrawal(transfer)?;
                }
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
        let segment_root = operand.exported_segment_root;

        log_info(&format!(
            "📥 Accumulate: Received {} bytes of input data",
            data.len()
        ));

        // DEBUG: Show first few bytes of input data
        if !data.is_empty() {
            let preview_len = core::cmp::min(20, data.len());
            let preview_bytes = &data[0..preview_len];
            log_info(&format!(
                "📥 Input data preview: {:02x?}",
                preview_bytes
            ));
        }

        // Parse compact format: N entries × (1B ld + prefix_bytes + 5B packed ObjectRef)
        // Note: Each entry has its own ld value (meta-shards can have different depths after splits)
        //
        // CRITICAL: Even if data.is_empty() (no meta-shard updates), we MUST still write
        // the block mapping and update the MMR. Otherwise "idle" blocks (empty or no splits)
        // vanish from the block history, breaking bidirectional lookups and finalization.

        let global_ld = if data.is_empty() {
            log_info("📥 Empty compact format, no meta-shard entries (idle block)");
            0u8 // No meta-shard updates, use ld=0
        } else if data.len() < 2 {
            log_error("❌ Invalid compact format: too short for metashard count");
            return None;
        } else {
            // Read metashard count from first 2 bytes
            let metashard_count = u16::from_le_bytes([data[0], data[1]]);
            log_info(&format!(
                "📥 Accumulate: Received {} meta-shard entries, {} bytes total",
                metashard_count, data.len()
            ));

            if metashard_count == 0 {
                log_info("📥 Empty compact format, no meta-shard entries (idle block)");
                0u8 // No meta-shard updates, use ld=0
            } else {
            // log_info(&format!(
            //     "📥 Deserializing compact format: {} bytes total",
            //     data.len()
            // ));

                // Parse variable-length entries starting after metashard count (offset 2)
                let mut offset = 2;
                let mut num_entries = 0;
                let mut max_ld = 0u8; // Track maximum ld for MetaSSR

                while num_entries < metashard_count && offset < data.len() {
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

                    log_info(&format!(
                        "📥 Parsing entry {}: ld={}, offset={}, remaining_bytes={}",
                        num_entries, ld, offset, data.len() - offset
                    ));

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
                    self.write_metashard(entry_bytes, ld, prefix_bytes, wph, num_entries as usize)?;

                    offset += entry_size;
                    num_entries += 1;
                }
                max_ld
            }
        };

        // ALWAYS write MetaSSR hint, even for idle blocks (ensures SSR key exists for Go lookups)
        self.write_metassr(global_ld);

        // ALWAYS write block mapping, even for idle blocks
        // This ensures every block appears in the bidirectional mapping and MMR
        self.write_block(wph, segment_root, self.timeslot)?;

        // After processing meta-shards, check for and execute accumulate instructions
        // The format is: [2B metashard_count][metashard entries...][2B contract witness][4B contract witness][accumulate instructions...]
        // We need to skip past metashard data + contract witness metadata (6 bytes) to find accumulate instructions

        // Calculate where metashard data ends
        let metashard_count = if data.len() >= 2 {
            u16::from_le_bytes([data[0], data[1]])
        } else {
            0
        };

        let mut meta_end_offset = 2; // Start after metashard count
        for _i in 0..metashard_count {
            if meta_end_offset >= data.len() {
                break;
            }
            let ld = data[meta_end_offset];
            meta_end_offset += 1;
            let prefix_bytes = ((ld + 7) / 8) as usize;
            let entry_size = prefix_bytes + 5;
            meta_end_offset += entry_size;
        }

        // Skip contract witness metadata (6 bytes)
        let accumulate_start_offset = meta_end_offset + 6;

        // Check if there are remaining bytes (accumulate instructions)
        if accumulate_start_offset < data.len() {
            let accumulate_bytes = &data[accumulate_start_offset..];
            log_info(&format!(
                "📥 Found {} bytes of accumulate instructions at offset {}",
                accumulate_bytes.len(), accumulate_start_offset
            ));

            // Deserialize and execute accumulate instructions
            let instructions = AccumulateInstruction::deserialize_all(accumulate_bytes);
            if !instructions.is_empty() {
                log_info(&format!(
                    "🔧 Executing {} accumulate instructions",
                    instructions.len()
                ));
                self.execute_accumulate_instructions(&instructions)?;
            }
        }

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

        // Read current global_depth
        // read(s: service_id, ko: key_offset, kz: key_size, o: out_offset, f: out_offset_within, l: length)
        let ssr_key = b"SSR";
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
        use crate::contractsharding::ObjectKind;
        use crate::meta_sharding::meta_shard_object_id;
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
        let (current_ld_parsed, prefix56) =
            crate::meta_sharding::parse_meta_shard_object_id(&object_id);
        debug_assert_eq!(
            current_ld_parsed, current_ld,
            "ld mismatch in delete_ancestor_shards"
        );

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
    fn write_block(
        &mut self,
        work_package_hash: [u8; 32],
        segment_root: [u8; 32],
        timeslot: u32,
    ) -> Option<()> {
        use utils::host_functions::write as host_write;

        // Write blocknumber → work_package_hash (32 bytes) + timeslot (4 bytes) + segment_root (32 bytes)
        let bn_key = block_number_to_object_id(self.block_number);
        let mut bn_data = Vec::with_capacity(68);
        bn_data.extend_from_slice(&work_package_hash);
        bn_data.extend_from_slice(&timeslot.to_le_bytes());
        bn_data.extend_from_slice(&segment_root);

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
        // Write reverse mapping: work_package_hash → blocknumber (4 bytes) + timeslot (4 bytes) + segment_root (32 bytes)
        let wph_key = work_package_hash;
        let mut wph_data = Vec::with_capacity(40);
        wph_data.extend_from_slice(&self.block_number.to_le_bytes());
        wph_data.extend_from_slice(&timeslot.to_le_bytes());
        wph_data.extend_from_slice(&segment_root);

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
            "📚 Wrote block # {} : BNOBJ {} ↔ WPH {}=timeslot={}",
            self.block_number,
            format_object_id(&bn_key),
            format_object_id(&work_package_hash),
            timeslot
        ));

        // Update MMR with this block's work_package_hash
        self.mmr.append(work_package_hash);

        // Increment block_number for next block
        self.block_number += 1;

        Some(())
    }

    /// Process Railgun withdrawal (DeferredTransfer from Railgun service → EVM)
    ///
    /// Memo v1 format: [version=0x01 | recipient(20) | asset_id(4) | reserved(103)]
    /// Total: 128 bytes
    ///
    /// Validates memo format and credits recipient's EVM account with withdrawn amount.
    /// The JAM runtime has already transferred the balance from Railgun service to EVM service.
    fn process_railgun_withdrawal(&self, transfer: &utils::functions::DeferredTransfer) -> Option<()> {

        // Validate memo version (must be 0x01)
        if transfer.memo[0] != 0x01 {
            log_error(&format!(
                "❌ Invalid memo version: expected 0x01, got {}",
                transfer.memo[0]
            ));
            return None;
        }

        // Extract recipient address (20 bytes at offset 1)
        let mut recipient_bytes = [0u8; 20];
        recipient_bytes.copy_from_slice(&transfer.memo[1..21]);

        // Extract asset_id (4 bytes at offset 21, little-endian)
        let asset_id = u32::from_le_bytes([
            transfer.memo[21],
            transfer.memo[22],
            transfer.memo[23],
            transfer.memo[24],
        ]);

        // Validate reserved bytes are zero (bytes 25..128)
        for (idx, &byte) in transfer.memo[25..128].iter().enumerate() {
            if byte != 0 {
                log_error(&format!(
                    "❌ Reserved byte at offset {} is non-zero: {}",
                    25 + idx,
                    byte
                ));
                return None;
            }
        }

        log_info(&format!(
            "💸 Railgun withdrawal: {} wei (asset {}) → 0x{}",
            transfer.amount,
            asset_id,
            hex::encode(&recipient_bytes)
        ));

        // Credit recipient's EVM account balance
        use primitive_types::{H160, U256};
        use crate::state::balance_storage_key;
        use utils::host_functions::read;

        let recipient_address = H160::from_slice(&recipient_bytes);
        let balance_key = balance_storage_key(recipient_address);

        // Read current balance
        let mut current_balance_bytes = [0u8; 32];
        unsafe {
            let bytes_read = read(
                0, // current service
                balance_key.as_ptr() as u64,
                32,
                current_balance_bytes.as_mut_ptr() as u64,
                0,
                32,
            );
            if bytes_read != 32 {
                log_error(&format!("❌ Failed to read balance for 0x{}", hex::encode(&recipient_bytes)));
                return None;
            }
        }

        let current_balance = U256::from_little_endian(&current_balance_bytes);
        let new_balance = current_balance + U256::from(transfer.amount);

        log_info(&format!(
            "  Current balance: {} wei, adding {} wei → {} wei",
            current_balance,
            transfer.amount,
            new_balance
        ));

        // Write new balance
        let mut new_balance_bytes = [0u8; 32];
        new_balance.to_little_endian(&mut new_balance_bytes);

        unsafe {
            let result = utils::host_functions::write(
                balance_key.as_ptr() as u64,
                32,
                new_balance_bytes.as_ptr() as u64,
                32,
            );
            if result != 0 {
                log_error(&format!("❌ Failed to write balance for 0x{}", hex::encode(&recipient_bytes)));
                return None;
            }
        }

        log_info(&format!(
            "✅ Withdrawal processed: recipient=0x{}, amount={}, asset_id={}, new_balance={}",
            hex::encode(&recipient_bytes),
            transfer.amount,
            asset_id,
            new_balance
        ));

        Some(())
    }

    /// Execute accumulate instructions by calling the appropriate host functions
    fn execute_accumulate_instructions(
        &self,
        instructions: &[AccumulateInstruction],
    ) -> Option<()> {
        use utils::host_functions as hf;

        for (idx, instruction) in instructions.iter().enumerate() {
            log_info(&format!(
                "  Executing instruction {}: opcode={}",
                idx, instruction.opcode
            ));

            // Call the appropriate host function based on opcode
            match instruction.opcode {
                AccumulateInstruction::BLESS => {
                    // bless(m, a_ptr, v, r, o_ptr, n)
                    if instruction.params.len() != 4 {
                        log_error(&format!(
                            "Invalid BLESS params count: {}",
                            instruction.params.len()
                        ));
                        return None;
                    }

                    let m = instruction.params[0];
                    let v = instruction.params[1];
                    let r = instruction.params[2];
                    let n = instruction.params[3];

                    const TOTAL_CORES: usize = 2;

                    let bold_z_len = n.checked_mul(12).map(|v| v as usize).unwrap_or(usize::MAX);
                    if bold_z_len == usize::MAX || instruction.data.len() < bold_z_len {
                        log_error(&format!(
                            "Invalid BLESS data lengths: data_len={}, expected_bold_z_len={}",
                            instruction.data.len(),
                            bold_z_len
                        ));
                        return None;
                    }

                    let bold_a_len = instruction.data.len() - bold_z_len;
                    if bold_a_len != TOTAL_CORES * 4 {
                        log_error(&format!(
                            "Invalid BLESS bold_a length: bold_a_len={}, bold_z_len={}",
                            bold_a_len, bold_z_len
                        ));
                        return None;
                    }

                    let bold_a_ptr = instruction.data.as_ptr() as u64;
                    let bold_z_ptr = bold_a_ptr + bold_a_len as u64;

                    unsafe {
                        hf::bless(m, bold_a_ptr, v, r, bold_z_ptr, n);
                    }
                }
                AccumulateInstruction::ASSIGN => {
                    // assign(c, o, a) where o points to queue work report bytes
                    if instruction.params.len() != 2 {
                        log_error(&format!(
                            "Invalid ASSIGN params count: {}",
                            instruction.params.len()
                        ));
                        return None;
                    }

                    const MAX_AUTHORIZATION_QUEUE_ITEMS: usize = 80;
                    const QUEUE_BYTES: usize = MAX_AUTHORIZATION_QUEUE_ITEMS * 32;

                    if instruction.data.len() != QUEUE_BYTES {
                        log_error(&format!(
                            "Invalid ASSIGN queue length: got {}, expected {}",
                            instruction.data.len(),
                            QUEUE_BYTES
                        ));
                        return None;
                    }

                    unsafe {
                        hf::assign(
                            instruction.params[0],            // c
                            instruction.data.as_ptr() as u64, // o: queue work report
                            instruction.params[1],            // a
                        );
                    }
                }
                AccumulateInstruction::DESIGNATE => {
                    // designate(o) - no params, validators bytes in data
                    if instruction.params.len() != 0 {
                        log_error(&format!(
                            "Invalid DESIGNATE params count: {}",
                            instruction.params.len()
                        ));
                        return None;
                    }

                    const VALIDATOR_BYTES: usize = 336;
                    const TOTAL_VALIDATORS: usize = 6;
                    let expected_len = VALIDATOR_BYTES * TOTAL_VALIDATORS;

                    if instruction.data.len() != expected_len {
                        log_error(&format!(
                            "Invalid DESIGNATE data length: got {}, expected {}",
                            instruction.data.len(),
                            expected_len
                        ));
                        return None;
                    }

                    unsafe {
                        hf::designate(instruction.data.as_ptr() as u64);
                    }
                }
                AccumulateInstruction::NEW => {
                    // new(o, l, g, m, f, i) where o points to code_hash bytes
                    if instruction.params.len() != 5 || instruction.data.len() != 32 {
                        log_error(&format!(
                            "Invalid NEW params: {} params, {} data bytes",
                            instruction.params.len(),
                            instruction.data.len()
                        ));
                        return None;
                    }
                    unsafe {
                        hf::new(
                            instruction.data.as_ptr() as u64, // o: offset to code hash
                            instruction.params[0],            // l
                            instruction.params[1],            // g
                            instruction.params[2],            // m
                            instruction.params[3],            // f
                            instruction.params[4],            // i
                        );
                    }
                }
                AccumulateInstruction::UPGRADE => {
                    // upgrade(o, g, m) where o points to code_hash bytes
                    if instruction.params.len() != 2 || instruction.data.len() != 32 {
                        log_error(&format!(
                            "Invalid UPGRADE params: {} params, {} data bytes",
                            instruction.params.len(),
                            instruction.data.len()
                        ));
                        return None;
                    }
                    unsafe {
                        hf::upgrade(
                            instruction.data.as_ptr() as u64, // o: offset to code hash
                            instruction.params[0],            // g
                            instruction.params[1],            // m
                        );
                    }
                }
                AccumulateInstruction::TRANSFER => {
                    // transfer(d, a, g, o) where o points to memo bytes
                    if instruction.params.len() != 3 {
                        log_error(&format!(
                            "Invalid TRANSFER params count: {}",
                            instruction.params.len()
                        ));
                        return None;
                    }

                    const MEMO_SIZE: usize = 128;
                    if instruction.data.len() != MEMO_SIZE {
                        log_error(&format!(
                            "Invalid TRANSFER memo size: got {}, expected {}",
                            instruction.data.len(),
                            MEMO_SIZE
                        ));
                        return None;
                    }

                    unsafe {
                        hf::transfer(
                            instruction.params[0],            // d
                            instruction.params[1],            // a
                            instruction.params[2],            // g
                            instruction.data.as_ptr() as u64, // o: offset to memo
                        );
                    }
                }
                AccumulateInstruction::EJECT => {
                    // eject(d, o) where o points to hash bytes
                    if instruction.params.len() != 1 || instruction.data.len() != 32 {
                        log_error(&format!(
                            "Invalid EJECT params: {} params, {} data bytes",
                            instruction.params.len(),
                            instruction.data.len()
                        ));
                        return None;
                    }
                    unsafe {
                        hf::eject(
                            instruction.params[0],            // d
                            instruction.data.as_ptr() as u64, // o: offset to hash
                        );
                    }
                }
                AccumulateInstruction::WRITE => {
                    // write(ko, kz, vo, vz) - key and value from instruction.data
                    if instruction.params.len() != 2 {
                        log_error(&format!(
                            "Invalid WRITE params: {} params (expected key/value lengths)",
                            instruction.params.len()
                        ));
                        return None;
                    }

                    let key_len = instruction.params[0] as usize;
                    let value_len = instruction.params[1] as usize;
                    let expected_len = key_len + value_len;

                    if instruction.data.len() != expected_len {
                        log_error(&format!(
                            "Invalid WRITE data length: got {}, expected {} (key_len={}, value_len={})",
                            instruction.data.len(),
                            expected_len,
                            key_len,
                            value_len
                        ));
                        return None;
                    }

                    let key_ptr = instruction.data.as_ptr() as u64;
                    let value_ptr = (instruction.data.as_ptr() as u64) + key_len as u64;
                    unsafe {
                        hf::write(
                            key_ptr,          // ko: key offset
                            key_len as u64,   // kz: key size
                            value_ptr,        // vo: value offset
                            value_len as u64, // vz: value size
                        );
                    }
                }
                AccumulateInstruction::SOLICIT => {
                    // solicit(o, z) where o points to hash bytes
                    if instruction.params.len() != 1 || instruction.data.len() != 32 {
                        log_error(&format!(
                            "Invalid SOLICIT params: {} params, {} data bytes",
                            instruction.params.len(),
                            instruction.data.len()
                        ));
                        return None;
                    }
                    unsafe {
                        hf::solicit(
                            instruction.data.as_ptr() as u64, // o: offset to hash
                            instruction.params[0],            // z
                        );
                    }
                }
                AccumulateInstruction::FORGET => {
                    // forget(o, z) where o points to hash bytes
                    if instruction.params.len() != 1 || instruction.data.len() != 32 {
                        log_error(&format!(
                            "Invalid FORGET params: {} params, {} data bytes",
                            instruction.params.len(),
                            instruction.data.len()
                        ));
                        return None;
                    }
                    unsafe {
                        hf::forget(
                            instruction.data.as_ptr() as u64, // o: offset to hash
                            instruction.params[0],            // z
                        );
                    }
                }
                AccumulateInstruction::PROVIDE => {
                    // provide(s, o, z) where o points to raw data
                    if instruction.params.len() != 2 {
                        log_error(&format!(
                            "Invalid PROVIDE params count: {}",
                            instruction.params.len()
                        ));
                        return None;
                    }

                    let s = instruction.params[0];
                    let z = instruction.params[1] as usize;

                    if instruction.data.len() != z {
                        log_error(&format!(
                            "Invalid PROVIDE data length: got {}, expected {}",
                            instruction.data.len(),
                            z
                        ));
                        return None;
                    }

                    unsafe {
                        hf::provide(s, instruction.data.as_ptr() as u64, z as u64);
                    }
                }
                _ => {
                    log_error(&format!(
                        "Unknown accumulate opcode: {}",
                        instruction.opcode
                    ));
                    return None;
                }
            }
        }

        log_info("✅ All accumulate instructions executed successfully");
        Some(())
    }
}
