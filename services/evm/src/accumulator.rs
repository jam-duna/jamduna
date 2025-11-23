//! Block Accumulator - manages block finalization and storage operations

use alloc::{format, vec::Vec};
use utils::{
    functions::{log_error, log_debug, log_info, AccumulateInput},
    constants::FULL,
};
use crate::{
    sharding::{ObjectKind, format_object_id},
    writes::{ExecutionEffectsEnvelope, deserialize_execution_effects},
};

/// BlockAccumulator manages block storage and retrieval operations
pub struct BlockAccumulator {
    #[allow(dead_code)]
    pub service_id: u32,
    pub envelopes: Vec<ExecutionEffectsEnvelope>,
}

impl BlockAccumulator {
    /// Accumulate all execution effects and finalize block
    ///
    /// Returns the accumulate root - MMR super_peak committing to all blocks
    /// Appends work_package_hashes to MMR and returns super_peak as commitment
    /// This ensures finalization commits to entire block history, never constant zero
    pub fn accumulate(service_id: u32, timeslot: u32, accumulate_inputs: &[AccumulateInput]) -> Option<[u8; 32]> {
        let accumulator = Self::new(service_id, timeslot, accumulate_inputs)?;
        let envelope_count = accumulator.envelopes.len();

        for envelope_idx in 0..envelope_count {
            let envelope = &accumulator.envelopes[envelope_idx];
            log_info(&format!("📦 Processing envelope #{} with {} candidates", envelope_idx, envelope.writes.len()));
            let writes = envelope.writes.clone();
            for candidate in &writes {
                let candidate = candidate.clone();

                // IMPORTANT: Accumulate ONLY writes meta-sharding infrastructure to JAM State.
                //
                // Meta-shard ObjectRefs (MetaShard, MetaSsrMetadata) are written to JAM State
                // because they provide the routing table for locating all other objects.
                //
                // All other objects (Code, Receipts, Blocks, BlockMetadata, etc.) remain as
                // DA-only artifacts - their payloads are already in JAM DA segments, and they
                // do NOT need ObjectRef pointers in JAM State. These objects are accessed
                // directly via their deterministic ObjectIDs during refine import.
                //
                // This design keeps JAM State minimal (only routing infrastructure) while
                // JAM DA holds all actual data (code, receipts, blocks, storage).
                match candidate.object_ref.object_kind {
                    kind if kind == ObjectKind::MetaShard as u8 => {
                        // Meta-shard objects - write ObjectRef to JAM State
                        // Payload is stored in DA, JAM State only tracks ObjectRef pointer
                        log_info(&format!("📝 Writing MetaShard ObjectRef for object_id: {}", format_object_id(&candidate.object_id)));
                        candidate.object_ref.write(&candidate.object_id);
                    }
                    kind if kind == ObjectKind::MetaSsrMetadata as u8 => {
                        // Meta-SSR metadata - write ObjectRef to JAM State
                        log_info(&format!("📝 Writing MetaSsrMetadata ObjectRef for object_id: {}", format_object_id(&candidate.object_id)));
                        candidate.object_ref.write(&candidate.object_id);
                    }
                    _ => {
                        // Other object kinds (Code, Receipt, Block, BlockMetadata, etc.)
                        // are intentionally NOT written to JAM State during accumulate.
                        // They remain DA-only artifacts, accessible via deterministic ObjectIDs.
                        log_debug(&format!(
                            "  Skipping ObjectKind {} (DA-only): object_id={}",
                            candidate.object_ref.object_kind,
                            format_object_id(&candidate.object_id)
                        ));
                    }
                }
            }
        }

        // Build block index: collect all unique work_package_hashes from envelopes
        let mut work_package_hashes = Vec::new();
        for envelope in &accumulator.envelopes {
            // Get work_package_hash from the first write intent in each envelope
            if let Some(first_write) = envelope.writes.first() {
                let wph = first_write.object_ref.work_package_hash;
                // Only add if not already present (deduplicate)
                if !work_package_hashes.contains(&wph) {
                    work_package_hashes.push(wph);
                }
            }
        }

        // Write block_number → work_package_hash mapping to JAM State
        // This enables queries like "what work packages contributed to block N?"
        let block_number = timeslot; // For now, use timeslot as block_number (1:1 mapping)
        if !work_package_hashes.is_empty() {
            Self::write_block_index(block_number, &work_package_hashes);
        }

        // Compute accumulate root using MMR (Merkle Mountain Range)
        // Append all work_package_hashes to MMR and return super_peak as commitment
        use crate::mmr::MMR;

        // Read existing MMR from JAM State (or create new if first block)
        let mut mmr = MMR::read_mmr(service_id).unwrap_or_else(|| {
            log_info("📋 Creating new MMR (first block)");
            MMR::new()
        });

        // Append each work_package_hash to the MMR
        // This creates an incremental commitment to all blocks
        for wph in &work_package_hashes {
            mmr.append(*wph);
        }

        // Write updated MMR back to JAM State BEFORE computing root
        // CRITICAL: MMR write must succeed, otherwise auditors can't reproduce the commitment
        // If write fails (WHAT/FULL), we must return None to signal accumulate failure
        let accumulate_root = match mmr.write_mmr() {
            Some(root) => {
                log_info(&format!(
                    "✅ Accumulate complete. Appended {} work packages to MMR. Root: {}",
                    work_package_hashes.len(),
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

    /// Create new BlockAccumulator and determine next block number from storage
    fn new(service_id: u32, timeslot: u32, accumulate_inputs: &[AccumulateInput]) -> Option<Self> {
        log_info(&format!("🏭 Creating BlockAccumulator for service_id={}, timeslot={}, inputs={}", service_id, timeslot, accumulate_inputs.len()));
        
        // Read storage and validate block sequence BEFORE processing any inputs.
        // This ensures we fail fast if the inputs are for the wrong block number,
        // preventing any storage corruption.
        let envelopes =
            match Self::collect_envelopes(accumulate_inputs) {
                Some(result) => {
                    log_info(&format!("✅ Collected {} envelopes from {} inputs", result.len(), accumulate_inputs.len()));
                    result
                },
                None => {
                    log_error("❌ Failed to collect envelopes from accumulate inputs");
                    return None;
                }
            };


        // First check if we can read the block number key
        use crate::block::EvmBlockPayload;
        match EvmBlockPayload::read_blocknumber_key(service_id) {
            Some(block_num) => {
                log_info(&format!("📊 Successfully read block_number={} from BLOCK_NUMBER_KEY", block_num));
            },
            None => {
                log_error(&format!("❌ Failed to read BLOCK_NUMBER_KEY for service_id={}", service_id));
                return None;
            }
        }

        Some(BlockAccumulator {
            service_id,
            envelopes,
        })
    }

    /// Deserialize accumulate inputs into envelopes
    fn collect_envelopes(
        accumulate_inputs: &[AccumulateInput],
    ) -> Option<Vec<ExecutionEffectsEnvelope>> {
        let mut envelopes: Vec<ExecutionEffectsEnvelope> = Vec::new();
        let mut envelope_idx: usize = 0;

        for (idx, input) in accumulate_inputs.iter().enumerate() {
            let AccumulateInput::OperandElements(operand) = input else {
                log_error(&format!("  Input #{}: Not OperandElements, skipping", idx));
                continue;
            };

            let Some(ok_data) = operand.result.ok.as_ref() else {
                log_error("    ❌ No ok_data (result failed)");
                continue;
            };

            match deserialize_execution_effects(ok_data) {
                Some(envelope) => {
                    log_debug(&format!(
                        "  Deserialized input #{}: {} candidate writes, export_count={}, gas_used={}",
                        envelope_idx,
                        envelope.writes.len(),
                        envelope.export_count,
                        envelope.gas_used,
                    ));

                    envelopes.push(envelope);
                    envelope_idx += 1;
                }
                None => {
                    log_error(&format!("  ❌ Failed to deserialize input #{}", idx));
                }
            }
        }

        if envelopes.is_empty() {
            log_error("Accumulate: no envelopes deserialized from inputs");
            return None;
        }

        Some(envelopes)
    }

    // ===== Block Index Functions =====

    /// Compute JAM State key for block_number → work_package_hash array
    ///
    /// Key format: keccak256("block_index" || block_number)
    fn block_index_key(block_number: u32) -> [u8; 32] {
        use utils::hash_functions::keccak256;

        let mut preimage = alloc::vec::Vec::new();
        preimage.extend_from_slice(b"block_index");
        preimage.extend_from_slice(&block_number.to_le_bytes());

        *keccak256(&preimage).as_fixed_bytes()
    }

    /// Write block_number → Vec<work_package_hash> mapping to JAM State
    ///
    /// Format: [4B count][32B wph1][32B wph2]...
    /// This enables queries like "what work packages contributed to block N?"
    fn write_block_index(block_number: u32, work_package_hashes: &[[u8; 32]]) {
        use utils::host_functions::write as host_write;

        // Serialize: count (4 bytes) + hashes (32 bytes each)
        let mut data = alloc::vec::Vec::new();
        data.extend_from_slice(&(work_package_hashes.len() as u32).to_le_bytes());
        for wph in work_package_hashes {
            data.extend_from_slice(wph);
        }

        let key = Self::block_index_key(block_number);

        // Write to JAM State
        let result = unsafe {
            host_write(
                key.as_ptr() as u64,
                key.len() as u64,
                data.as_ptr() as u64,
                data.len() as u64,
            )
        };

        // Check for failure codes
        if result == FULL {
            log_error(&format!(
                "❌ Failed to write block index for block {} (result={})",
                block_number, result
            ));
        } else {
            log_info(&format!(
                "📚 Wrote block index: block {} has {} work packages ({} bytes)",
                block_number,
                work_package_hashes.len(),
                data.len()
            ));
        }
    }

    /// Read block_number → Vec<work_package_hash> mapping from JAM State
    #[allow(dead_code)]
    pub fn read_block_index(service_id: u32, block_number: u32) -> Option<Vec<[u8; 32]>> {
        use utils::host_functions::read as host_read;

        let key = Self::block_index_key(block_number);

        // Allocate buffer for reading block index data
        let mut buffer = alloc::vec![0u8; 10000]; // Large enough for ~300 work package hashes

        let result = unsafe {
            host_read(
                service_id as u64,
                key.as_ptr() as u64,
                key.len() as u64,
                buffer.as_mut_ptr() as u64,
                0 as u64,
                buffer.len() as u64,
            )
        };

        // Check for failure codes
        if result == u64::MAX || result == u64::MAX - 1 {
            return None;
        }

        let data_len = result as usize;
        if data_len < 4 {
            return None;
        }

        // Parse count
        let count = u32::from_le_bytes([buffer[0], buffer[1], buffer[2], buffer[3]]) as usize;

        // Verify size
        let expected_size = 4 + count * 32;
        if data_len != expected_size {
            log_error(&format!(
                "Block index size mismatch: expected {}, got {}",
                expected_size,
                data_len
            ));
            return None;
        }

        // Parse hashes
        let mut hashes = Vec::new();
        for i in 0..count {
            let offset = 4 + i * 32;
            let mut hash = [0u8; 32];
            hash.copy_from_slice(&buffer[offset..offset + 32]);
            hashes.push(hash);
        }

        Some(hashes)
    }

}

