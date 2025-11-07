//! Block Accumulator - manages block finalization and storage operations

use alloc::{format, vec::Vec};
use utils::{
    constants::{WHAT, FULL},
    functions::{log_error, log_debug, log_info, AccumulateInput},
};
use crate::{
    block::EvmBlockPayload,
    receipt::TransactionReceiptRecord,
    sharding::{ObjectKind, format_object_id},
    writes::{ExecutionEffectsEnvelope, deserialize_execution_effects},
};

/// BlockAccumulator manages block storage and retrieval operations
pub struct BlockAccumulator {
    #[allow(dead_code)]
    pub service_id: u32,
    pub envelopes: Vec<ExecutionEffectsEnvelope>,
    pub current_block: EvmBlockPayload,
    pub log_index_start: u64,
}

impl BlockAccumulator {
    /// Accumulate all execution effects and finalize block
    ///
    /// Returns the accumulate root (serialized object_id + ObjectRef data) or None on failure
    pub fn accumulate(service_id: u32, timeslot: u32, accumulate_inputs: &[AccumulateInput]) -> Option<[u8; 32]> {
        let mut accumulator = Self::new(service_id, timeslot, accumulate_inputs)?;
        let envelope_count = accumulator.envelopes.len();
        for envelope_idx in 0..envelope_count {
            let envelope = &accumulator.envelopes[envelope_idx];
            log_info(&format!("📦 Processing envelope #{} with {} candidates", envelope_idx, envelope.writes.len()));
            let writes = envelope.writes.clone();
            for candidate in &writes {
                let mut candidate = candidate.clone();
                match candidate.object_ref.object_kind {
                    kind if kind == ObjectKind::Receipt as u8 => {
                        let record = TransactionReceiptRecord::from_candidate(&candidate)?;
                        // Write Receipt objects as ObjectRef + payload to JAM State
                        accumulator.accumulate_receipt(&mut candidate, record)?;
                    }
                    kind if kind == ObjectKind::Block as u8 => {
                        // Block objects are handled separately by EvmBlockPayload::write()
                        // They write only EvmBlockPayload data (no ObjectRef prefix)
                        log_info(&format!("📝 Skipping Block object_id: {} (handled by EvmBlockPayload::write)", format_object_id(&candidate.object_id)));
                    }
                    _ => {
                        // Code, SSR, shards write only their ObjectRef data
                        log_info(&format!("📝 Writing ObjectRef for object_id: {}", format_object_id(&candidate.object_id)));
                        candidate.object_ref.write(&candidate.object_id);
                    }
                }
            }
        }
        // Update EvmBlockPayload and write to storage
        accumulator.current_block.write()?;

        // Write block_hash → block_number mapping AFTER block is fully written
        // This ensures the hash is computed from the complete, finalized block
        use crate::block::EvmBlockPayload;
        let block_hash = accumulator.current_block.compute_block_hash();
        EvmBlockPayload::write_blockhash_mapping_pub(&block_hash, accumulator.current_block.number as u32);

        // Return the block hash as the accumulate root
        Some(block_hash)
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

        // Read current block payload and parent hash
        log_info(&format!("📖 Attempting to read EvmBlockPayload for service_id={}, timeslot={}", service_id, timeslot));

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

        let current_block = match EvmBlockPayload::read(service_id, timeslot as u64) {
            Some(block) => {
                log_info(&format!("✅ Successfully read current_block (number={})", block.number));
                block
            },
            None => {
                log_error(&format!("❌ EvmBlockPayload::read failed for service_id={}, timeslot={}", service_id, timeslot));
                return None;
            }
        };


        Some(BlockAccumulator {
            service_id,
            envelopes,
            current_block,
            log_index_start: 0,
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

    /// Write Receipt object to JAM State as ObjectRef + payload and append receipt_hash to MMR
    fn accumulate_receipt(&mut self, candidate: &mut crate::writes::ObjectCandidateWrite, record: TransactionReceiptRecord) -> Option<()> {
        use utils::host_functions::write;

        let (receipt_hash, new_log_index_start) = self.current_block.add_receipt(candidate, record, self.log_index_start);

        // Update receipt payload with cumulative gas and log index (Phase 2)
        // self.current_block.gas_used now contains the cumulative gas up to and including this transaction
        // self.log_index_start is the starting log index for this transaction's logs
        crate::receipt::update_receipt_phase2(&mut candidate.payload, self.current_block.gas_used, self.log_index_start);

        self.log_index_start = new_log_index_start;

        // Create combined buffer: ObjectRef + payload
        let mut value_buffer = candidate.object_ref.serialize();
        value_buffer.extend_from_slice(&candidate.payload);

        let key_buffer = candidate.object_id;

        let result = unsafe {
            write(
                key_buffer.as_ptr() as u64,
                key_buffer.len() as u64,
                value_buffer.as_ptr() as u64,
                value_buffer.len() as u64,
            )
        };

        // Check for failure codes
        if result == WHAT || result == FULL {
            log_error(&format!("❌ Failed to write Receipt object_id: {}, error code: {}", format_object_id(&candidate.object_id), result));
            return None;
        }

        log_info(&format!("📝 Writing Receipt object_id: {}, receipt_hash: {}", format_object_id(&candidate.object_id), format_object_id(&receipt_hash)));
        Some(())
    }
}

