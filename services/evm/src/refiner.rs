//! Block Refiner - manages block construction and imported objects during refine

/// Set to true to enable verbose refine logging
const REFINE_VERBOSE: bool = false;

use crate::{
    contractsharding::format_data_hex,
    genesis,
    state::{MajikBackend, MajikOverlay},
    tx::{DecodedCallCreate, decode_call_args, decode_transact_args},
    witness_events::{is_precompile, is_system_contract},
};
use alloc::{collections::BTreeMap, format, string::{String, ToString}, vec, vec::Vec};
use evm::backend::RuntimeBackend;
use evm::interpreter::ExitError;
use evm::interpreter::runtime::Log;
use primitive_types::{H160, H256, U256};
use utils::effects::WriteEffectEntry;
use utils::{
    effects::{ExecutionEffects, ObjectId, ObjectRef, WriteIntent},
    functions::{
        RefineArgs, WorkItem, fetch_extrinsic, format_object_id, format_segment, log_debug, log_error, log_info, log_warn,
    },
    hash_functions::keccak256,
    host_functions::AccumulateInstruction,
};

/// Payload type discriminator
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum PayloadType {
    Builder,
    Transactions,
    Genesis,
    Call,
}

/// Metadata extracted from work item payload
#[derive(Debug, Clone, Copy)]
pub struct PayloadMetadata {
    pub payload_type: PayloadType,
    pub payload_size: u32,
    pub global_depth: u8,
    pub witness_count: u16,
    pub block_access_list_hash: [u8; 32],
}

/// Parse work item payload to extract type and metadata
///
/// All payload formats use the same structure:
/// - type_byte (1) + count (4 LE) + global_depth (1) + witness_count (2 LE) + block_access_list_hash (32)
///
/// Where:
/// - 0x00 (Builder): count = tx_count
/// - 0x01 (Transactions): count = tx_count
/// - 0x02 (Genesis): count = extrinsics_count (bootstrap commands like 'K')
/// - 0x03 (Call): count = extrinsics_count
///
/// Returns None if payload is malformed.
pub fn parse_payload_metadata(payload: &[u8]) -> Option<PayloadMetadata> {
    if payload.is_empty() {
        log_error("Payload is empty");
        return None;
    }

    let payload_type = match payload[0] {
        0 => PayloadType::Builder,
        1 => PayloadType::Transactions,
        2 => PayloadType::Genesis,
        3 => PayloadType::Call,
        other => {
            log_error(&format!("Unknown payload type byte: 0x{:02x}", other));
            return None;
        }
    };

    // All payloads: type_byte (1) + count (4 LE) + global_depth (1) + witness_count (2 LE) + block_access_list_hash (32)
    if payload.len() < 40 {
        log_error(&format!(
            "Payload too short for {:?}: len={}, expected >=40",
            payload_type,
            payload.len()
        ));
        return None;
    }

    let count = u32::from_le_bytes(payload[1..5].try_into().unwrap());
    let global_depth = payload[5];
    let witness_count = u16::from_le_bytes(payload[6..8].try_into().unwrap());
    let mut block_access_list_hash = [0u8; 32];
    block_access_list_hash.copy_from_slice(&payload[8..40]);

    Some(PayloadMetadata {
        payload_type,
        payload_size: count,
        witness_count,
        global_depth,
        block_access_list_hash,
    })
}

fn hex_head_tail(data: &[u8]) -> (String, String) {
    if data.is_empty() {
        return (String::new(), String::new());
    }
    let head_len = core::cmp::min(64, data.len());
    let tail_len = core::cmp::min(64, data.len());
    let head = format_segment(&data[..head_len]);
    let tail = format_segment(&data[data.len() - tail_len..]);
    (head, tail)
}

/// Parse accumulate instruction from event log
/// Returns Some(AccumulateInstruction) if the log matches an accumulate event
fn parse_accumulate_event(log: &Log) -> Option<AccumulateInstruction> {
    if log.topics.is_empty() {
        return None;
    }

    let event_sig = log.topics[0];

    // Event signatures (keccak256 of event signature string)
    // Bless(uint64,uint64,uint64,uint64,bytes,bytes)
    let bless_sig = H256(keccak256(b"Bless(uint64,uint64,uint64,uint64,bytes,bytes)").0);
    // Assign(uint64,uint64,uint64)
    let assign_sig = H256(keccak256(b"Assign(uint64,bytes,uint64)").0);
    // Designate(bytes)
    let designate_sig = H256(keccak256(b"Designate(bytes)").0);
    // New(bytes32,uint64,uint64,uint64,uint64,uint64)
    let new_sig = H256(keccak256(b"New(bytes32,uint64,uint64,uint64,uint64,uint64)").0);
    // Upgrade(bytes32,uint64,uint64)
    let upgrade_sig = H256(keccak256(b"Upgrade(bytes32,uint64,uint64)").0);
    // TransferAccumulate(uint64,uint64,uint64,bytes)
    let transfer_sig = H256(keccak256(b"TransferAccumulate(uint64,uint64,uint64,bytes)").0);
    // Eject(uint64,bytes32)
    let eject_sig = H256(keccak256(b"Eject(uint64,bytes32)").0);
    // Write(bytes,bytes)
    let write_sig = H256(keccak256(b"Write(bytes,bytes)").0);
    // Solicit(bytes32,uint64)
    let solicit_sig = H256(keccak256(b"Solicit(bytes32,uint64)").0);
    // Forget(bytes32,uint64)
    let forget_sig = H256(keccak256(b"Forget(bytes32,uint64)").0);
    // Provide(uint64,bytes)
    let provide_sig = H256(keccak256(b"Provide(uint64,bytes)").0);

    // Helper to decode u64 from bytes at offset
    let decode_u64_head = |data: &[u8], index: usize| -> Option<u64> {
        let offset = index.checked_mul(32)?;
        if offset + 32 > data.len() {
            return None;
        }
        Some(U256::from_big_endian(&data[offset..offset + 32]).low_u64())
    };

    // Helper to decode bytes32 from data
    let decode_bytes32 = |data: &[u8], offset: usize| -> Option<[u8; 32]> {
        if offset + 32 > data.len() {
            return None;
        }
        let mut result = [0u8; 32];
        result.copy_from_slice(&data[offset..offset + 32]);
        Some(result)
    };

    let decode_dynamic_bytes = |data: &[u8], head_index: usize| -> Option<Vec<u8>> {
        let offset = decode_u64_head(data, head_index)? as usize;
        if offset + 32 > data.len() {
            return None;
        }

        let len = U256::from_big_endian(&data[offset..offset + 32]).low_u64() as usize;
        let start = offset + 32;
        let end = start + len;

        if end > data.len() {
            return None;
        }

        Some(data[start..end].to_vec())
    };

    let data = &log.data;

    if event_sig == bless_sig {
        // Bless(uint64 m, uint64 v, uint64 r, uint64 n, bytes boldA, bytes boldZ)
        let m = decode_u64_head(data, 0)?;
        let v = decode_u64_head(data, 1)?;
        let r = decode_u64_head(data, 2)?;
        let n = decode_u64_head(data, 3)?;
        let bold_a = decode_dynamic_bytes(data, 4)?;
        let bold_z = decode_dynamic_bytes(data, 5)?;

        if bold_a.len() % 4 != 0 {
            return None;
        }

        if bold_z.len() % 12 != 0 || bold_z.len() / 12 != n as usize {
            return None;
        }

        let mut combined = bold_a;
        combined.extend_from_slice(&bold_z);

        Some(AccumulateInstruction::new(
            AccumulateInstruction::BLESS,
            vec![m, v, r, n],
            combined,
        ))
    } else if event_sig == assign_sig {
        // Assign(uint64 c, bytes queueWorkReport, uint64 a)
        const MAX_AUTHORIZATION_QUEUE_ITEMS: usize = 80;
        const QUEUE_BYTES: usize = MAX_AUTHORIZATION_QUEUE_ITEMS * 32;

        let c = decode_u64_head(data, 0)?;
        let queue_work_report = decode_dynamic_bytes(data, 1)?;
        let a = decode_u64_head(data, 2)?;

        if queue_work_report.len() != QUEUE_BYTES {
            return None;
        }

        Some(AccumulateInstruction::new(
            AccumulateInstruction::ASSIGN,
            vec![c, a],
            queue_work_report,
        ))
    } else if event_sig == designate_sig {
        // Designate(bytes validators)
        const VALIDATOR_BYTES: usize = 336;
        const TOTAL_VALIDATORS: usize = 6;

        let validators = decode_dynamic_bytes(data, 0)?;
        if validators.len() != VALIDATOR_BYTES * TOTAL_VALIDATORS {
            return None;
        }

        Some(AccumulateInstruction::new(
            AccumulateInstruction::DESIGNATE,
            vec![],
            validators,
        ))
    } else if event_sig == new_sig {
        // New(bytes32 codeHash, uint64 l, uint64 g, uint64 m, uint64 f, uint64 i)
        let code_hash = decode_bytes32(data, 0)?;
        let l = decode_u64_head(data, 1)?;
        let g = decode_u64_head(data, 2)?;
        let m = decode_u64_head(data, 3)?;
        let f = decode_u64_head(data, 4)?;
        let i = decode_u64_head(data, 5)?;
        Some(AccumulateInstruction::new(
            AccumulateInstruction::NEW,
            vec![l, g, m, f, i],
            code_hash.to_vec(),
        ))
    } else if event_sig == upgrade_sig {
        // Upgrade(bytes32 codeHash, uint64 g, uint64 m)
        let code_hash = decode_bytes32(data, 0)?;
        let g = decode_u64_head(data, 1)?;
        let m = decode_u64_head(data, 2)?;
        Some(AccumulateInstruction::new(
            AccumulateInstruction::UPGRADE,
            vec![g, m],
            code_hash.to_vec(),
        ))
    } else if event_sig == transfer_sig {
        // TransferAccumulate(uint64 d, uint64 a, uint64 g, bytes memo)
        let d = decode_u64_head(data, 0)?;
        let a = decode_u64_head(data, 1)?;
        let g = decode_u64_head(data, 2)?;
        let memo = decode_dynamic_bytes(data, 3)?;

        if memo.len() != 128 {
            return None;
        }

        Some(AccumulateInstruction::new(
            AccumulateInstruction::TRANSFER,
            vec![d, a, g],
            memo,
        ))
    } else if event_sig == eject_sig {
        // Eject(uint64 d, bytes32 hashData)
        let d = decode_u64_head(data, 0)?;
        let hash_data = decode_bytes32(data, 32)?;
        Some(AccumulateInstruction::new(
            AccumulateInstruction::EJECT,
            vec![d],
            hash_data.to_vec(),
        ))
    } else if event_sig == write_sig {
        // Write(bytes key, bytes value)
        let key_bytes = decode_dynamic_bytes(data, 0)?;
        let value_bytes = decode_dynamic_bytes(data, 1)?;

        if key_bytes.is_empty() || value_bytes.is_empty() {
            return None;
        }

        let mut combined_data = key_bytes;
        let value_len = value_bytes.len() as u64;
        let key_len = (combined_data.len()) as u64;
        combined_data.extend_from_slice(&value_bytes);

        Some(AccumulateInstruction::new(
            AccumulateInstruction::WRITE,
            vec![key_len, value_len],
            combined_data,
        ))
    } else if event_sig == solicit_sig {
        // Solicit(bytes32 hashData, uint64 z)
        let hash_data = decode_bytes32(data, 0)?;
        let z = decode_u64_head(data, 1)?;
        Some(AccumulateInstruction::new(
            AccumulateInstruction::SOLICIT,
            vec![z],
            hash_data.to_vec(),
        ))
    } else if event_sig == forget_sig {
        // Forget(bytes32 hashData, uint64 z)
        let hash_data = decode_bytes32(data, 0)?;
        let z = decode_u64_head(data, 1)?;
        Some(AccumulateInstruction::new(
            AccumulateInstruction::FORGET,
            vec![z],
            hash_data.to_vec(),
        ))
    } else if event_sig == provide_sig {
        // Provide(uint64 s, bytes data)
        let s = decode_u64_head(data, 0)?;
        let payload = decode_dynamic_bytes(data, 1)?;
        let z = payload.len() as u64;

        Some(AccumulateInstruction::new(
            AccumulateInstruction::PROVIDE,
            vec![s, z],
            payload,
        ))
    } else {
        None
    }
}

/// BlockRefiner struct for managing block construction and imported objects
pub struct BlockRefiner {
    pub objects_map: BTreeMap<ObjectId, (ObjectRef, Vec<u8>)>,
    pub accumulate_instructions: Vec<AccumulateInstruction>,
}

impl BlockRefiner {
    pub fn new() -> Self {
        BlockRefiner {
            objects_map: BTreeMap::new(),
            accumulate_instructions: Vec::new(),
        }
    }

    /// Build block from receipt ObjectRefs and serialize all execution effects
    /// Returns execution effects for DA export including block and receipt write intents
    #[allow(dead_code)]
    pub fn serialize_blocks(&self) -> Vec<WriteIntent> {
        let mut write_intents: Vec<WriteIntent> = Vec::new();
        for (object_id, (object_ref, payload)) in &self.objects_map {
            if object_ref.object_kind == crate::contractsharding::ObjectKind::Block as u8 {
                // Create WriteIntent for DA export with proper segment calculation
                let block_write_intent = WriteIntent {
                    effect: WriteEffectEntry {
                        object_id: *object_id, // Block hash from Blake2b(header)
                        ref_info: object_ref.clone(),
                        payload: payload.clone(),
                        tx_index: 0,
                    },
                };
                write_intents.push(block_write_intent);
            }
        }
        write_intents
    }

    /// Refine payload transactions - execute transactions and ingest receipts into this BlockRefiner
    /// Returns None if any transaction fails to decode or if commit/revert operations fail
    /// Returns (ExecutionEffects, MajikBackend) to allow access to meta-shard state
    pub fn refine_payload_transactions(
        &mut self,
        mut backend: MajikBackend,
        config: &evm::standard::Config,
        work_item_index: u16,
        tx_start_index: usize,
        tx_count_expected: u32,
        refine_args: &RefineArgs,
        refine_context: &utils::functions::RefineContext,
        service_id: u32,
        post_state_root: Option<[u8; 32]>,
    ) -> Option<(ExecutionEffects, MajikBackend)> {
        use crate::receipt::TransactionReceiptRecord;
        use crate::state::MajikOverlay;
        use evm::interpreter::etable::DispatchEtable;
        use evm::interpreter::trap::CallCreateTrap;
        use evm::standard::{
            EtableResolver, Invoker, State, TransactArgs, TransactArgsCallCreate, TransactGasPrice,
        };

        genesis::load_precompiles(&mut backend);
        let mut receipts: Vec<TransactionReceiptRecord> = Vec::new();
        let mut overlay = MajikOverlay::new(backend, &config.runtime);
        let mut total_gas_used: u64 = 0;
        let mut total_log_count: u32 = 0;

        // Fetch and execute transactions (only tx_count_expected, not witnesses)
        for tx_offset in 0..tx_count_expected {
            let tx_index = tx_offset as usize;
            let extrinsic_index = tx_start_index + tx_index;
            let extrinsic = match fetch_extrinsic(work_item_index, extrinsic_index as u32) {
                Ok(bytes) => bytes,
                Err(e) => {
                    log_error(&format!(
                        "  Failed to fetch extrinsic {}: {:?}",
                        extrinsic_index, e
                    ));
                    return None;
                }
            };
            if extrinsic.is_empty() {
                log_error(&format!(
                    "  Empty extrinsic at index {}",
                    extrinsic_index
                ));
                return None;
            }
            // Compute transaction hash from RLP-encoded signed transaction
            let tx_hash_bytes = keccak256(&extrinsic);
            let mut tx_hash = [0u8; 32];
            tx_hash.copy_from_slice(tx_hash_bytes.as_bytes());

            log_debug(&format!(
                "  Processing extrinsic {}: {} bytes, tx_hash={}, data={:?}",
                tx_index,
                extrinsic.len(),
                format_object_id(tx_hash_bytes.0),
                extrinsic
            ));

            // Decode extrinsic into transaction format
            const ARG_HEADER_LEN: usize = 148; // caller(20) + target(20) + gas_limit(32) + gas_price(32) + value(32) + call_kind(4) + data_len(8)
            let decoded = match decode_transact_args(
                extrinsic.as_ptr() as u64,
                extrinsic.len() as u64,
                ARG_HEADER_LEN,
            ) {
                Some(d) => d,
                None => {
                    log_error(&format!("  Extrinsic {} decode failed", tx_index));
                    return None;
                }
            };

            if REFINE_VERBOSE {
                log_info(&format!(
                    "  TXHASH {} Decoded: caller={:?}, gas_limit={}, value={}",
                    hex::encode(tx_hash),
                    decoded.caller,
                    decoded.gas_limit,
                    decoded.value
                ));
            }
            overlay.begin_transaction(tx_index);

            // Record transaction-level witness events (Phase A: EIP-4762)
            // FIX Issue 5: Target access happens always, write only if value > 0
            // Phase D: Filter precompiles/system contracts per EIP-4762
            let tx_target = match &decoded.call_create {
                DecodedCallCreate::Call { address, .. } => {
                    // Phase D: Skip precompile/system contract code/header from witness
                    // but still charge for execution
                    if is_precompile(*address) || is_system_contract(*address) {
                        None  // Exclude from transaction-level witness events
                    } else {
                        Some(*address)
                    }
                }
                DecodedCallCreate::Create { .. } => None, // CREATE target unknown at tx start
            };

            // Always record origin access/write (even if origin is precompile - unlikely but possible)
            overlay.record_tx_origin_witness(decoded.caller);

            // Always record target access (if exists and not filtered)
            if let Some(target) = tx_target {
                overlay.record_tx_target_access(target);

                // Only record target write if value > 0
                if decoded.value > U256::zero() {
                    overlay.record_tx_target_write(target);
                }
            }

            let mut _committed = false;
            let etable: DispatchEtable<State<'_>, MajikOverlay<'_>, CallCreateTrap> =
                DispatchEtable::runtime();
            let resolver = EtableResolver::new(&(), &etable);
            let invoker = Invoker::new(&resolver);

            if REFINE_VERBOSE { log_info("  🔨 Matching call_create..."); }
            let call_create = match decoded.call_create {
                DecodedCallCreate::Call { address, ref data } => {
                    if data.len() >= 4 {
                        let mut selector = [0u8; 4];
                        selector.copy_from_slice(&data[0..4]);

                        let mut log_parts = vec![
                            format!("CALL {:?}→{:?}", decoded.caller, address),
                            format!("calldata=0x{}", hex::encode(&data)),
                            format!("sel=0x{}", hex::encode(&selector)),
                        ];

                        if data.len() >= 4 + 32 {
                            let mut addr_bytes = [0u8; 20];
                            addr_bytes.copy_from_slice(&data[4 + 12..4 + 32]);
                            let decoded_addr = H160::from(addr_bytes);
                            log_parts.push(format!("arg0={:?}", decoded_addr));
                        }
                        if data.len() >= 4 + 32 + 32 {
                            let amount = U256::from_big_endian(&data[4 + 32..4 + 64]);
                            log_parts.push(format!("arg1={}", amount));
                        }

                        log_debug(&format!("  {}", log_parts.join(", ")));
                    } else {
                        log_debug(&format!(
                            "  CALL {:?}→{:?}, calldata={}",
                            decoded.caller,
                            address,
                            format_data_hex(&data)
                        ));
                    }
                    TransactArgsCallCreate::Call {
                        address,
                        data: data.clone(),
                    }
                }
                DecodedCallCreate::Create { ref init_code } => {
                    log_debug(&format!(
                        "  CREATE with {} bytes init_code",
                        init_code.len()
                    ));
                    TransactArgsCallCreate::Create {
                        init_code: init_code.clone(),
                        salt: None,
                    }
                }
            };

            let args = TransactArgs {
                caller: decoded.caller,
                gas_limit: decoded.gas_limit,
                gas_price: TransactGasPrice::Legacy(decoded.gas_price), // Use transaction gas price
                value: decoded.value,
                call_create,
                access_list: Vec::new(),
                config,
            };

            // Execute transaction with standard EVM gas table
            if REFINE_VERBOSE { log_info(&format!("  ⚡ Starting evm::transact for tx {}", tx_index)); }
            let result = evm::transact(args, None, &mut overlay, &invoker);
            if REFINE_VERBOSE { log_info(&format!("  ⚡ Finished evm::transact for tx {}", tx_index)); }

            // Log transaction result details (disabled for performance)
            if let Err(e) = &result {
                log_error(&format!("  ❌ Transaction {} failed: {:?}", tx_index, e));
            }

            // Log shard entries after transaction completion
            //log_cached_shard_entries(&overlay);

            match result {
                Ok(tx_result) => {
                    // Track gas consumption (EVM already handled fee payment in finalize_transact)
                    let evm_gas = tx_result.used_gas;

                    // Phase B Option C: Calculate witness gas from tracked events
                    //let witness_gas = overlay.take_witness_gas();
                    let witness_gas = 0u64; // FIX: Placeholder until witness gas calculation is implemented
                    let total_tx_gas = evm_gas.saturating_add(witness_gas.into());

                    // Revert if total gas (EVM + witness) exceeds the transaction gas limit
                    // Users must set gas_limit to cover both EVM execution and UBT witness costs
                    if total_tx_gas > decoded.gas_limit {
                        log_warn(&format!(
                            "  ⚠️ Transaction {} exceeds gas limit (limit={}, evm={}, witness={}, total={}) - reverting",
                            tx_index, decoded.gas_limit, evm_gas, witness_gas, total_tx_gas
                        ));
                        match overlay.revert_transaction() {
                            Ok(_) => {}
                            Err(_) => {
                                log_error(&format!("  Transaction {} revert failed", tx_index));
                                return None;
                            }
                        }

                        let estimated_gas = decoded.gas_limit;
                        total_gas_used = total_gas_used.saturating_add(estimated_gas.as_u64());

                        receipts.push(TransactionReceiptRecord {
                            hash: tx_hash,
                            success: false,
                            used_gas: estimated_gas,
                            cumulative_gas: total_gas_used as u32,
                            log_index_start: total_log_count,
                            tx_index: tx_index as u32,
                            logs: Vec::new(),
                            payload: extrinsic.clone(),
                        });
                        continue;
                    }

                    match overlay.commit_transaction() {
                        Ok(_) => {
                            if REFINE_VERBOSE { log_info(&format!("  Transaction {} committed", tx_index)); }
                            _committed = true;
                            let logs = overlay.take_transaction_logs(tx_index);
                            let log_count = logs.len() as u32;

                            // Parse accumulate events from logs
                            for log in &logs {
                                if let Some(instruction) = parse_accumulate_event(log) {
                                    log_debug(&format!(
                                        "  📝 Parsed accumulate instruction: opcode={}, params={:?}",
                                        instruction.opcode, instruction.params
                                    ));
                                    self.accumulate_instructions.push(instruction);
                                }
                            }

                            // Charge witness gas fees (not handled by evm::finalize_transact)
                            // Note: evm_gas, witness_gas, total_tx_gas already calculated above
                            if witness_gas > 0 {
                                let gas_price = decoded.gas_price;
                                if let Some(witness_fee) =
                                    U256::from(witness_gas).checked_mul(gas_price)
                                {
                                    if let Err(err) = overlay.withdrawal(decoded.caller, witness_fee)
                                    {
                                        log_error(&format!(
                                            "  ❌ Failed to charge witness gas from {:?}: {:?}",
                                            decoded.caller, err
                                        ));
                                        return None;
                                    }

                                    // Pay witness fee to block coinbase (consistent with EVM fee flow)
                                    overlay.deposit(
                                        overlay.overlay.backend().environment.block_coinbase,
                                        witness_fee,
                                    );
                                } else {
                                    log_error("  ❌ Witness fee overflow - aborting block");
                                    return None;
                                }
                            }

                            // Accumulate gas across all successful transactions
                            total_gas_used =
                                total_gas_used.saturating_add(total_tx_gas.as_u64());

                            if REFINE_VERBOSE {
                                log_info(&format!(
                                    "  ⛽ Gas used: {} (EVM: {}, Witness: {}, total: {})",
                                    total_tx_gas, evm_gas, witness_gas, total_gas_used
                                ));
                            }

                            receipts.push(TransactionReceiptRecord {
                                hash: tx_hash,
                                success: true,
                                used_gas: total_tx_gas,  // Phase B: Include witness gas
                                cumulative_gas: total_gas_used as u32,
                                log_index_start: total_log_count,
                                tx_index: tx_index as u32,
                                logs,
                                payload: extrinsic.clone(),
                            });

                            // Update total log count for next transaction
                            total_log_count = total_log_count.saturating_add(log_count);
                        }
                        Err(_) => {
                            log_error(&format!("  Transaction {} commit failed", tx_index));
                            return None;
                        }
                    }
                }
                Err(error) => {
                    // Enhanced error logging to diagnose transaction failures
                    let error_detail = match &error {
                        ExitError::Exception(exc) => format!("Exception({:?})", exc),
                        ExitError::Reverted => "Reverted".to_string(),
                        ExitError::Fatal(fatal) => format!("Fatal({:?})", fatal),
                    };

                    // Log detailed transaction context on failure
                    log_error(&format!(
                        "  ❌ Transaction {} failed: {} ({:?})",
                        tx_index, error_detail, error
                    ));
                    log_error(&format!("     Caller: {:?}", decoded.caller));
                    log_error(&format!("     Gas limit: {}", decoded.gas_limit));
                    log_error(&format!("     Gas price: {}", decoded.gas_price));
                    log_error(&format!("     Value: {}", decoded.value));
                    match &decoded.call_create {
                        DecodedCallCreate::Call { address, data } => {
                            log_error(&format!("     Call to: {:?}", address));
                            log_error(&format!("     Calldata: {} bytes", data.len()));
                            if data.len() >= 4 {
                                log_error(&format!(
                                    "     Selector: 0x{:02x}{:02x}{:02x}{:02x}",
                                    data[0], data[1], data[2], data[3]
                                ));
                            }
                        }
                        DecodedCallCreate::Create { init_code } => {
                            log_error(&format!(
                                "     Create with {} bytes init_code",
                                init_code.len()
                            ));
                        }
                    };
                    match overlay.revert_transaction() {
                        Ok(_) => {}
                        Err(_) => {
                            log_error(&format!("  Transaction {} revert failed", tx_index));
                            return None;
                        }
                    }

                    // Note: EVM's finalize_transact already charged gas before returning error
                    // We use gas_limit as conservative estimate for accounting since the actual
                    // gas consumed is not available in the error return value
                    // TODO: Modify vendor EVM to include gas_used in error variant for accurate reporting
                    let estimated_gas = decoded.gas_limit;

                    total_gas_used = total_gas_used.saturating_add(estimated_gas.as_u64());

                    if REFINE_VERBOSE {
                        log_info(&format!(
                            "  ⛽ Gas (failed tx): ~{} est (total: {})",
                            estimated_gas, total_gas_used
                        ));
                    }

                    receipts.push(TransactionReceiptRecord {
                        hash: tx_hash,
                        success: false,
                        used_gas: estimated_gas, // Conservative estimate (actual gas not available in error)
                        cumulative_gas: total_gas_used as u32,
                        log_index_start: total_log_count,
                        tx_index: tx_index as u32,
                        logs: Vec::new(),
                        payload: extrinsic.clone(),
                    });
                    // Note: total_log_count doesn't change since failed tx has no logs
                }
            }
        }

        // Use post_state_root if available (guarantor mode), otherwise use refine_context.state_root
        let state_root_for_block = post_state_root.unwrap_or(refine_context.state_root);

        // Phase A: Log witness event statistics
        overlay.log_witness_stats();

        let (effects, backend) = overlay.deconstruct(
            refine_args.wphash,
            &receipts,
            state_root_for_block,
            refine_context.lookup_anchor_slot,
            total_gas_used,
            service_id,
        );

        if REFINE_VERBOSE {
            log_info(&format!(
                "🎯 Gas Summary: total_consumed={} transactions={} total_writes={}",
                total_gas_used,
                receipts.len(),
                effects.write_intents.len()
            ));
        }

        Some((effects, backend))
    }

    /// Create default environment for a given service (chain_id)
    fn create_environment(
        service_id: u32,
        _payload_type: PayloadType,
    ) -> crate::state::EnvironmentData {
        use primitive_types::{H160, U256};

        // Coinbase address: 0xEaf3223589Ed19bcd171875AC1D0F99D31A5969c
        const COINBASE_ADDRESS: H160 = H160([
            0xEa, 0xf3, 0x22, 0x35, 0x89, 0xEd, 0x19, 0xbc, 0xd1, 0x71, 0x87, 0x5A, 0xC1, 0xD0,
            0xF9, 0x9D, 0x31, 0xA5, 0x96, 0x9c,
        ]);

        crate::state::EnvironmentData {
            block_number: U256::zero(),
            block_timestamp: U256::zero(),
            block_coinbase: COINBASE_ADDRESS,
            block_gas_limit: U256::from(30_000_000u64),
            block_base_fee_per_gas: U256::zero(),
            chain_id: U256::from(0x1107),
            block_difficulty: U256::zero(),
            block_randomness: None,
            service_id,
        }
    }

    /// Process work item and execute transactions, returning ExecutionEffects
    ///
    /// This method:
    /// 1. Parses and verifies state witnesses from extrinsics
    /// 2. Creates backend with verified objects
    /// 3. Executes transactions (bootstrap, call, or normal transactions)
    /// 4. Exports payloads to DA segments
    ///
    /// Returns Some(ExecutionEffects) or None if any operation fails.
    pub fn from_work_item(
        work_item_index: u16,
        work_item: &WorkItem,
        num_extrinsics: usize,
        refine_context: &utils::functions::RefineContext,
        refine_args: &RefineArgs,
    ) -> Option<(ExecutionEffects, Vec<AccumulateInstruction>, u16, u32)> {
        use utils::effects::StateWitness;

        log_info(&format!(
            "📦 from_work_item: parsing payload len={} head={}",
            work_item.payload.len(),
            format_segment(&work_item.payload)
        ));
        // Parse payload metadata
        let metadata = match parse_payload_metadata(&work_item.payload) {
            Some(metadata) => metadata,
            None => {
                log_error("📦 from_work_item: parse_payload_metadata failed");
                return None;
            }
        };
        log_info(&format!(
            "📦 Payload metadata: type={:?}, tx_count={}, global_depth={}, witness_count={}, extrinsics={}",
            metadata.payload_type,
            metadata.payload_size,
            metadata.global_depth,
            metadata.witness_count,
            num_extrinsics
        ));
        log_info(&format!(
            "📦 Payload raw: len={}, bytes=0x{}",
            work_item.payload.len(),
            hex::encode(&work_item.payload)
        ));
        let extrinsic_sizes: Vec<String> = work_item
            .work_item_extrinsics
            .iter()
            .map(|ext| ext.len.to_string())
            .collect();
        log_info(&format!(
            "📦 Extrinsic sizes: [{}]",
            extrinsic_sizes.join(", ")
        ));
        let fetch_extrinsic_by_index = |idx: usize| -> Option<Vec<u8>> {
            match fetch_extrinsic(work_item_index, idx as u32) {
                Ok(bytes) => Some(bytes),
                Err(e) => {
                    log_error(&format!(
                        "❌ Refine: fetch_extrinsic failed idx={} err={:?}",
                        idx, e
                    ));
                    None
                }
            }
        };
        let mut ubt_pre_witness: Option<crate::ubt::UBTWitnessSection> = None;
        let mut ubt_post_witness: Option<crate::ubt::UBTWitnessSection> = None;
        let mut ubt_witness_count = 0usize;

        if num_extrinsics >= 2 {
            let pre_bytes = match fetch_extrinsic_by_index(0) {
                Some(bytes) => bytes,
                None => return None,
            };
            if crate::ubt::UBTWitnessSection::deserialize(&pre_bytes).is_ok() {
                let post_bytes = match fetch_extrinsic_by_index(1) {
                    Some(bytes) => bytes,
                    None => return None,
                };
                match crate::ubt::verify_witness_section(
                    &pre_bytes,
                    None,
                    crate::ubt::WitnessValueKind::Pre,
                ) {
                    Ok(pre) => {
                        match crate::ubt::verify_witness_section(
                            &post_bytes,
                            None,
                            crate::ubt::WitnessValueKind::Post,
                        ) {
                        Ok(post) => {
                            ubt_pre_witness = Some(pre);
                            ubt_post_witness = Some(post);
                            ubt_witness_count = 2;
                            log_info(&format!(
                                "🔐 Detected UBT witness extrinsics (pre={}, post={})",
                                pre_bytes.len(),
                                post_bytes.len()
                            ));
                            if let (Some(pre_w), Some(post_w)) =
                                (ubt_pre_witness.as_ref(), ubt_post_witness.as_ref())
                            {
                                let (pre_head, pre_tail) = hex_head_tail(&pre_bytes);
                                let (post_head, post_tail) = hex_head_tail(&post_bytes);
                                log_info(&format!(
                                    "✅ UBT verified: pre_root=0x{}, pre_entries={}, pre_keys={}, pre_nodes={}, pre_refs={}, pre_stems={}, pre_ext={}, post_root=0x{}, post_entries={}, post_keys={}, post_nodes={}, post_refs={}, post_stems={}, post_ext={}",
                                    hex::encode(pre_w.root),
                                    pre_w.entries.len(),
                                    pre_w.proof.keys.len(),
                                    pre_w.proof.nodes.len(),
                                    pre_w.proof.node_refs.len(),
                                    pre_w.proof.stems.len(),
                                    pre_w.extensions.len(),
                                    hex::encode(post_w.root),
                                    post_w.entries.len(),
                                    post_w.proof.keys.len(),
                                    post_w.proof.nodes.len(),
                                    post_w.proof.node_refs.len(),
                                    post_w.proof.stems.len(),
                                    post_w.extensions.len()
                                ));
                                log_info(&format!(
                                    "🔍 UBT witness bytes: pre_len={}, pre_head64={}, pre_tail64={}, post_len={}, post_head64={}, post_tail64={}",
                                    pre_bytes.len(),
                                    pre_head,
                                    pre_tail,
                                    post_bytes.len(),
                                    post_head,
                                    post_tail
                                ));
                            }
                        }
                        Err(err) => {
                            log_error(&format!(
                                "❌ UBT pre witness found but post witness verification failed: {:?}",
                                err
                            ));
                            return None;
                        }
                        }
                    }
                    Err(err) => {
                        log_error(&format!(
                            "❌ UBT pre witness verification failed: {:?}",
                            err
                        ));
                        return None;
                    }
                }
            }
        }

        // Only process witnesses for PayloadTransaction/PayloadBuilder
        let tx_count = metadata.payload_size as usize;
        let witness_start_idx = tx_count + ubt_witness_count;
        if witness_start_idx > num_extrinsics {
            log_error(&format!(
                "❌ Refine: extrinsic count too small: witness_start_idx={} num_extrinsics={}",
                witness_start_idx, num_extrinsics
            ));
            return None;
        }
        let mut block_builder = BlockRefiner::new();
        let mut imported_objects: BTreeMap<ObjectId, (ObjectRef, Vec<u8>)> = BTreeMap::new();

        for idx in witness_start_idx..num_extrinsics {
            let extrinsic = match fetch_extrinsic_by_index(idx) {
                Some(bytes) => bytes,
                None => return None,
            };
            // log_debug(&format!(
            //         "🔍 Attempting to decode witness extrinsic idx={} len={}",
            //         idx,
            //         extrinsic.len()
            // ));
            if metadata.payload_type == PayloadType::Transactions
                || metadata.payload_type == PayloadType::Builder
            {
                // Witness extrinsics are serialized directly without format byte prefix
                if extrinsic.is_empty() {
                    log_error(&format!("Empty extrinsic at index {}", idx));
                    return None;
                }
                
                // Deserialize and verify state witness (no format byte prefix)
                match StateWitness::deserialize_and_verify(
                    work_item.service,
                    &extrinsic,
                    refine_context.state_root,
                ) {
                    Ok(state_witness) => {
                        let object_id = state_witness.object_id;
                        let object_ref = state_witness.ref_info.clone();

                        let payload_type_str = if metadata.payload_type == PayloadType::Transactions
                        {
                            "Transactions"
                        } else {
                            "Builder"
                        };

                        if let Some(payload) =
                            state_witness.fetch_object_payload(work_item, work_item_index)
                        {
                            if REFINE_VERBOSE {
                                let payload_len = payload.len();
                                log_info(&format!(
                                    "✅ Payload{}: Verified + imported object {} (payload_length={})",
                                    payload_type_str,
                                    format_object_id(object_id),
                                    payload_len
                                ));
                            }
                            imported_objects.insert(object_id, (object_ref, payload));
                        } else {
                            log_warn(&format!(
                                "⚠️  Payload{}: Verified witness but could not fetch payload for object {}",
                                payload_type_str,
                                format_object_id(object_id)
                            ));
                        }
                    }
                    Err(e) => {
                        log_error(&format!(
                            "Failed to deserialize or verify witness at index {}: {:?}",
                            idx, e
                        ));
                        return None;
                    }
                }
            } else if metadata.payload_type == PayloadType::Call {
                // Handle Call payload - early exit handled below
                // No witness processing needed for Call payloads
            }
        }

        if REFINE_VERBOSE {
            log_info(&format!(
                "📥 Refine: Set up block builder for {} objects",
                imported_objects.len()
            ));
        }

        // Create environment/backend from work item context
        use crate::state::MajikBackend;
        use evm::standard::Config;

        let config = Config::shanghai();
        let environment = Self::create_environment(work_item.service, metadata.payload_type);

        let mut post_state_root: Option<[u8; 32]> = None;

        let mut backend = if let (Some(pre_witness), Some(post_witness)) =
            (ubt_pre_witness.as_ref(), ubt_post_witness.as_ref())
        {
            log_info("🔐 Guarantor mode: Verifying UBT witnesses...");
            post_state_root = Some(post_witness.root);

            match MajikBackend::from_ubt_entries(
                environment,
                &pre_witness.entries,
                metadata.global_depth,
            ) {
                Some(backend) => {
                    log_info("✅ UBT witnesses verified successfully");
                    backend
                }
                None => {
                    log_error("❌ UBT witness cache load failed");
                    return None;
                }
            }
        } else {
            log_info("🔨 Builder mode: Will generate UBT witnesses after execution");
            let execution_mode = crate::state::ExecutionMode::Builder;

            MajikBackend::new(
                environment,
                imported_objects.clone(),
                metadata.global_depth,
                execution_mode,
            )
        };

        // Populate block_builder.objects_map with imported_objects for 'B' command access
        block_builder.objects_map = imported_objects.clone();

        // Execute based on payload type
        let (mut execution_effects, accumulate_instructions) =
            if metadata.payload_type == PayloadType::Call {
                // Call mode - return early from refine_call_payload (no accumulate instructions in Call mode)
                return Self::refine_call_payload(backend, &config, work_item_index, num_extrinsics)
                    .map(|effects| (effects, Vec::new(), 0u16, 0u32));
            } else if metadata.payload_type == PayloadType::Transactions
                || metadata.payload_type == PayloadType::Builder
            {
                // Execute transactions
                let mut block_builder = block_builder;

                match block_builder.refine_payload_transactions(
                    backend,
                    &config,
                    work_item_index,
                    ubt_witness_count,
                    metadata.payload_size,
                    refine_args,
                    refine_context,
                    work_item.service,
                    post_state_root,
                ) {
                    Some((effects, returned_backend)) => {
                        backend = returned_backend;
                        let instructions = block_builder.accumulate_instructions;

                        // Guarantor BAL verification (Phase 4D)
                        // Only performed when UBT witnesses are present (guarantor mode)
                        let builder_claimed_bal = metadata.block_access_list_hash != [0u8; 32];
                        let is_transaction_payload = metadata.payload_type == PayloadType::Transactions
                            || metadata.payload_type == PayloadType::Builder;

                        if let (Some(pre_witness), Some(post_witness)) =
                            (ubt_pre_witness.as_ref(), ubt_post_witness.as_ref())
                        {
                            // Guarantor mode: enforce BAL rules
                            if is_transaction_payload && !builder_claimed_bal {
                                // Transaction payload MUST have non-zero BAL hash
                                log_error("❌ Guarantor: Transaction payload missing BAL hash (all zeros)");
                                return None;
                            }

                            if builder_claimed_bal {
                                // Compute BAL hash from witness (native Rust implementation)
                                let bal = crate::block_access_list::BlockAccessList::from_ubt_entries(
                                    &pre_witness.entries,
                                    &post_witness.entries,
                                );
                                let computed_bal_hash = bal.hash();

                                if REFINE_VERBOSE {
                                    log_info(&format!(
                                        "Guarantor: Computed BAL hash = {}",
                                        hex::encode(computed_bal_hash)
                                    ));
                                }

                                // Compare computed hash with builder's claimed hash from payload
                                if computed_bal_hash != metadata.block_access_list_hash {
                                    log_error(&format!(
                                        "❌ Guarantor: BAL hash mismatch! Builder claimed: {}, Guarantor computed: {}",
                                        hex::encode(metadata.block_access_list_hash),
                                        hex::encode(computed_bal_hash)
                                    ));
                                    return None;
                                }

                                if REFINE_VERBOSE {
                                    log_info(&format!(
                                        "✅ Guarantor: BAL hash verified: {}",
                                        hex::encode(computed_bal_hash)
                                    ));
                                }
                            } else {
                                // No BAL hash claimed (genesis/call payloads)
                                if REFINE_VERBOSE { log_info("Guarantor: No BAL hash in payload (genesis/call mode)"); }
                            }
                        } else {
                            // Builder mode: BAL hash will be computed and injected after refine in Go
                            // Skip verification here to allow builder to proceed
                        }

                        (effects, instructions)
                    }
                    None => {
                        log_error("Refine: refine_payload_transactions failed");
                        return None;
                    }
                }
            } else {
                log_error(&format!("❌ Unknown payload type: {:?}", work_item.payload));
                return None;
            };

        // Capture the ObjectId/ObjectRef pairs we need for meta-sharding before exports run.
        // This avoids relying on the original write intents staying intact if the host FFI mutates them.
        let mut meta_object_refs: Vec<(ObjectId, ObjectRef)> = execution_effects
            .write_intents
            .iter()
            .map(|intent| (intent.effect.object_id, intent.effect.ref_info.clone()))
            .collect();

        let mut export_count: u16 = 0;

        // Export block first, then other intents
        let block_kind = crate::contractsharding::ObjectKind::Block as u8;
        for pass in [true, false] {
            // pass=true: export blocks only, pass=false: export non-blocks
            for (idx, intent) in execution_effects.write_intents.iter_mut().enumerate() {
                let is_block = intent.effect.ref_info.object_kind == block_kind;
                if is_block != pass {
                    continue;
                }
                let mut export_entry = WriteEffectEntry {
                    object_id: intent.effect.object_id,
                    ref_info: intent.effect.ref_info.clone(),
                    payload: intent.effect.payload.clone(),
                    tx_index: intent.effect.tx_index,
                };

                match export_entry.export_effect(export_count as usize) {
                    Ok(next_index) => {
                        export_count = next_index;
                        if let Some((_, object_ref)) = meta_object_refs.get_mut(idx) {
                            object_ref.index_start = export_entry.ref_info.index_start;
                        }
                        intent.effect.ref_info.index_start = export_entry.ref_info.index_start;
                    }
                    Err(e) => {
                        log_error(&format!(
                            "  ❌ Failed to export write intent {}: {:?}",
                            idx, e
                        ));
                        return None;
                    }
                }
            }
        }

        // Process contract_intents: Build ONE payload containing all contract storage/code blobs
        // Format: Multiple blobs, each: [20B address][1B kind][4B payload_length][...payload...]
        let contract_witness_index_start: u16;
        let contract_witness_payload_length: u32;

        if REFINE_VERBOSE {
            log_info(&format!(
                "📦 Processing {} contract_intents for witness blob export",
                execution_effects.contract_intents.len()
            ));
        }

        if !execution_effects.contract_intents.is_empty() {
            let mut contract_blob_buffer = Vec::new();

            for intent in &execution_effects.contract_intents {
                let object_kind = intent.effect.ref_info.object_kind;
                let payload_length = intent.effect.payload.len() as u32;
                let tx_index = intent.effect.tx_index;

                // Extract address from object_id
                // object_id format: [20B address][12B discriminator]
                let address: [u8; 20] = intent.effect.object_id[0..20].try_into().unwrap();

                // Build blob: [20B address][1B kind][4B payload_length][4B tx_index][...payload...]
                contract_blob_buffer.extend_from_slice(&address);
                contract_blob_buffer.push(object_kind);
                contract_blob_buffer.extend_from_slice(&payload_length.to_le_bytes());
                contract_blob_buffer.extend_from_slice(&tx_index.to_le_bytes());
                contract_blob_buffer.extend_from_slice(&intent.effect.payload);

                log_debug(&format!(
                    "  Contract blob: addr={:?}, kind={}, payload_len={}, tx_index={}",
                    H160::from(address), object_kind, payload_length, tx_index
                ));
            }

            // Export the entire contract blob buffer as ONE segment range
            let mut contract_export_entry = WriteEffectEntry {
                object_id: [0u8; 32], // Dummy object_id, not used for witness data
                ref_info: ObjectRef::new(
                    refine_args.wphash,
                    contract_blob_buffer.len() as u32,
                    0xFF, // Special kind for contract witness blob
                ),
                payload: contract_blob_buffer,
                tx_index: 0,
            };

            match contract_export_entry.export_effect(export_count as usize) {
                Ok(next_index) => {
                    contract_witness_index_start = contract_export_entry.ref_info.index_start;
                    contract_witness_payload_length = contract_export_entry.ref_info.payload_length;
                    export_count = next_index;

                    if REFINE_VERBOSE {
                        log_info(&format!(
                            "  ✅ Exported contract witness blob: {} intents, index_start={}, total_len={}, export_count: {} → {}",
                            execution_effects.contract_intents.len(),
                            contract_witness_index_start,
                            contract_witness_payload_length,
                            contract_export_entry.ref_info.index_start,
                            next_index
                        ));
                    }
                }
                Err(e) => {
                    log_error(&format!("  ❌ Failed to export contract witness blob: {:?}", e));
                    return None;
                }
            }
        } else {
            // No contract intents - use placeholder values
            contract_witness_index_start = 0;
            contract_witness_payload_length = 0;
        }

        // Process meta-shards AFTER export so ObjectRefs have correct index_start values
        let mut meta_write_intents = backend.process_meta_shards_after_export(
            refine_args.wphash,
            &meta_object_refs,
            work_item.service,
            metadata.payload_type as u8,
        );
        if REFINE_VERBOSE {
            log_info(&format!(
                "📦 Generated {} meta-shard write intents",
                meta_write_intents.len()
            ));
        }

        // Clone meta_write_intents BEFORE export since export_effect may corrupt memory via FFI
        let mut meta_intents_for_output = meta_write_intents.clone();

        // Export meta-shard payloads to DA segments
        if REFINE_VERBOSE {
            log_info(&format!(
                "📤 Exporting {} meta-shard intents...",
                meta_write_intents.len()
            ));
        }
        for (idx, intent) in meta_write_intents.iter_mut().enumerate() {
            let start_index = export_count;

            match intent.effect.export_effect(start_index as usize) {
                Ok(next_index) => {
                    if REFINE_VERBOSE {
                        log_info(&format!(
                            "  ✅ Exported meta-shard[{}]: payload_len={}, export_count: {} → {}, wph={}, index_start={}, object_id={}",
                            idx,
                            intent.effect.ref_info.payload_length,
                            export_count,
                            next_index,
                            crate::contractsharding::format_object_id(&intent.effect.ref_info.work_package_hash),
                            start_index,
                            crate::contractsharding::format_object_id(&intent.effect.object_id)
                        ));
                    }
                    export_count = next_index;

                    // Update the cloned version with the index_start value that was set during export
                    // We use start_index (captured before the FFI call) because intent.effect is corrupted after export
                    meta_intents_for_output[idx].effect.ref_info.index_start = start_index;
                }
                Err(e) => {
                    log_error(&format!(
                        "  ❌ Failed to export meta-shard intent {}: {:?}",
                        idx, e
                    ));
                    return None;
                }
            }
        }

        // Note: meta_write_intents is safe to drop now because export_effect()
        // creates a copy of the payload before passing it to the FFI, so the
        // original struct is never corrupted.

        // CRITICAL: Store meta-shard payloads in imported_objects cache for builder witnesses
        // The builder must provide these payloads to the guarantor so it can import the same meta-shards
        // Otherwise guarantor creates different meta-shard structure → non-deterministic ExportCount!
        for intent in &meta_intents_for_output {
            let object_id = intent.effect.object_id;
            let object_ref = intent.effect.ref_info.clone();
            let payload = intent.effect.payload.clone();

            if REFINE_VERBOSE {
                log_info(&format!(
                    "  📝 Storing meta-shard in imported_objects cache: object_id={:?}, payload_len={}",
                    object_id,
                    payload.len()
                ));
            }

            backend
                .imported_objects
                .borrow_mut()
                .insert(object_id, (object_ref, payload));
        }

        // CRITICAL: APPEND meta_intents to execution_effects.write_intents, don't replace!
        // The original write_intents contain storage shards, code, etc. that must be serialized.
        execution_effects.write_intents.extend(meta_intents_for_output);

        if REFINE_VERBOSE {
            log_info(&format!(
                "📋 Collected {} accumulate instructions",
                accumulate_instructions.len()
            ));
        }

        Some((
            execution_effects,
            accumulate_instructions,
            contract_witness_index_start,
            contract_witness_payload_length,
        ))
    }

    // ===== Payload Processing Functions =====

    /// Refine call payload - handle EstimateGas/Call mode
    pub fn refine_call_payload(
        mut backend: MajikBackend,
        config: &evm::standard::Config,
        work_item_index: u16,
        num_extrinsics: usize,
    ) -> Option<ExecutionEffects> {
        use evm::interpreter::etable::DispatchEtable;
        use evm::interpreter::trap::CallCreateTrap;
        use evm::standard::{
            EtableResolver, Invoker, State, TransactArgs, TransactArgsCallCreate, TransactGasPrice,
        };

        genesis::load_precompiles(&mut backend);
        let mut overlay = MajikOverlay::new(backend, &config.runtime);

        if num_extrinsics != 1 {
            log_error(&format!(
                "❌ Payload Call expects exactly 1 extrinsic, got {}",
                num_extrinsics
            ));
            return None;
        }

        let extrinsic = match fetch_extrinsic(work_item_index, 0) {
            Ok(bytes) => bytes,
            Err(e) => {
                log_error(&format!(
                    "❌ Payload Call: failed to fetch extrinsic: {:?}",
                    e
                ));
                return None;
            }
        };
        if extrinsic.is_empty() {
            log_error("❌ Payload Call: empty extrinsic");
            return None;
        }

        // Decode unsigned call arguments
        let decoded = match decode_call_args(extrinsic.as_ptr() as u64, extrinsic.len() as u64) {
            Some(d) => d,
            None => {
                log_error("❌ Payload Call: Failed to decode call args");
                return None;
            }
        };

        if REFINE_VERBOSE {
            log_info(&format!(
                "📞 PayloadCall from={:?}, gas_limit={}, value={}",
                decoded.caller, decoded.gas_limit, decoded.value
            ));
        }

        let etable: DispatchEtable<State<'_>, MajikOverlay<'_>, CallCreateTrap> =
            DispatchEtable::runtime();
        let resolver = EtableResolver::new(&(), &etable);
        let invoker = Invoker::new(&resolver);

        let call_create = match decoded.call_create {
            DecodedCallCreate::Call { address, data } => {
                if REFINE_VERBOSE {
                    log_info(&format!(
                        "  CALL {:?}→{:?}, data_len={}",
                        decoded.caller,
                        address,
                        data.len()
                    ));
                }
                TransactArgsCallCreate::Call { address, data }
            }
            DecodedCallCreate::Create { init_code } => {
                if REFINE_VERBOSE {
                    log_info(&format!(
                        "  CREATE with {} bytes init_code",
                        init_code.len()
                    ));
                }
                TransactArgsCallCreate::Create {
                    init_code,
                    salt: None,
                }
            }
        };

        let args = TransactArgs {
            caller: decoded.caller,
            gas_limit: decoded.gas_limit,
            gas_price: TransactGasPrice::Legacy(decoded.gas_price), // Use transaction gas price
            value: decoded.value,
            call_create,
            access_list: Vec::new(),
            config,
        };

        // Execute the call with standard EVM gas table
        let result = evm::transact(args, None, &mut overlay, &invoker);

        match result {
            Ok(tx_result) => {
                let gas_used = tx_result.used_gas.as_u64();
                // Extract output from call result
                let output = match tx_result.call_create {
                    evm::standard::TransactValueCallCreate::Call { retval, .. } => retval,
                    evm::standard::TransactValueCallCreate::Create { .. } => Vec::new(),
                };

                if REFINE_VERBOSE {
                    log_info(&format!(
                        "✅ Payload Call: Success, gas_used={}, output_len={}, output={}",
                        gas_used,
                        output.len(),
                        format_segment(&output)
                    ));
                }

                // create WriteIntent
                let write_intent = WriteIntent {
                    effect: WriteEffectEntry {
                        object_id: [0xCCu8; 32],
                        ref_info: ObjectRef {
                            work_package_hash: [0u8; 32],
                            index_start: 0,
                            payload_length: 0,
                            object_kind: 0,
                        },
                        payload: output,
                        tx_index: 0,
                    },
                };

                // Create ExecutionEffects and return
                let effects = ExecutionEffects {
                    write_intents: vec![write_intent],
                    contract_intents: vec![],
                    accumulate_instructions: Vec::new(),
                };
                Some(effects)
            }
            Err(error) => {
                log_error(&format!("❌ PayloadCall failed: {:?}", error));
                None
            }
        }
    }

}
