//! Block Refiner - manages block construction and imported objects during refine

use crate::{
    genesis,
    sharding::format_data_hex,
    state::{MajikBackend, MajikOverlay},
    tx::{DecodedCallCreate, decode_call_args, decode_transact_args},
};
use alloc::{collections::BTreeMap, format, string::ToString, vec, vec::Vec};
use evm::backend::{RuntimeBackend, RuntimeEnvironment};
use evm::interpreter::ExitError;
use primitive_types::{H160, U256};
use utils::effects::WriteEffectEntry;
use utils::{
    effects::{ExecutionEffects, ObjectId, ObjectRef, WriteIntent},
    functions::{
        RefineArgs, WorkItem, format_object_id, format_segment, log_debug, log_error, log_info,
    },
    hash_functions::keccak256,
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
    pub witness_count: u16,
}

/// Parse work item payload to extract type and metadata
///
/// All payload formats use the same structure:
/// - type_byte (1) + count (4 LE) + [witness_count (2 LE)]
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

    // All payloads: type_byte (1) + count (4 LE) + [witness_count (2 LE)]
    if payload.len() < 5 {
        log_error(&format!(
            "Payload too short for {:?}: len={}, expected >=5",
            payload_type,
            payload.len()
        ));
        return None;
    }

    let count = u32::from_le_bytes(payload[1..5].try_into().unwrap());
    let witness_count = if payload.len() >= 7 {
        u16::from_le_bytes(payload[5..7].try_into().unwrap())
    } else {
        0
    };

    Some(PayloadMetadata {
        payload_type,
        payload_size: count,
        witness_count,
    })
}

/// BlockRefiner struct for managing block construction and imported objects
pub struct BlockRefiner {
    pub objects_map: BTreeMap<ObjectId, (ObjectRef, Vec<u8>)>,
}

impl BlockRefiner {
    pub fn new() -> Self {
        BlockRefiner {
            objects_map: BTreeMap::new(),
        }
    }

    /// Build block from receipt ObjectRefs and serialize all execution effects
    /// Returns execution effects for DA export including block and receipt write intents
    #[allow(dead_code)]
    pub fn serialize_blocks(&self) -> Vec<WriteIntent> {
        let mut write_intents: Vec<WriteIntent> = Vec::new();
        for (object_id, (object_ref, payload)) in &self.objects_map {
            if object_ref.object_kind == crate::sharding::ObjectKind::Block as u8 {
                // Create WriteIntent for DA export with proper segment calculation
                let block_write_intent = WriteIntent {
                    effect: WriteEffectEntry {
                        object_id: *object_id, // Block hash from Blake2b(header)
                        ref_info: object_ref.clone(),
                        payload: payload.clone(),
                    },
                    dependencies: Vec::new(),
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
        extrinsics: &[Vec<u8>],
        tx_count_expected: u32,
        refine_args: &RefineArgs,
        refine_context: &utils::functions::RefineContext,
        service_id: u32,
    ) -> Option<(ExecutionEffects, MajikBackend)> {
        use crate::jam_gas::JAMGasState;
        use crate::receipt::TransactionReceiptRecord;
        use crate::state::MajikOverlay;
        use evm::interpreter::etable::DispatchEtable;
        use evm::interpreter::trap::CallCreateTrap;
        use evm::standard::{
            EtableResolver, Invoker, TransactArgs, TransactArgsCallCreate, TransactGasPrice,
        };

        genesis::load_precompiles(&mut backend);
        let mut receipts: Vec<TransactionReceiptRecord> = Vec::new();
        let mut overlay = MajikOverlay::new(backend, &config.runtime);
        let mut total_jam_gas_used: u64 = 0;
        let mut total_log_count: u32 = 0;

        // Execute transactions from extrinsics (only first tx_count_expected, not witnesses)
        let tx_extrinsics = extrinsics.iter().take(tx_count_expected as usize);
        for (tx_index, extrinsic) in tx_extrinsics.enumerate() {
            // Compute transaction hash from RLP-encoded signed transaction
            let tx_hash_bytes = keccak256(extrinsic);
            let mut tx_hash = [0u8; 32];
            tx_hash.copy_from_slice(tx_hash_bytes.as_bytes());

            log_debug(&format!(
                "  Processing extrinsic {}: {} bytes, tx_hash={}",
                tx_index,
                extrinsic.len(),
                format_object_id(tx_hash_bytes.0)
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

            log_info(&format!(
                "  Decoded: caller={:?}, gas_limit={}, value={}",
                decoded.caller, decoded.gas_limit, decoded.value
            ));
            overlay.begin_transaction(tx_index);
            let mut _committed = false;
            let etable: DispatchEtable<JAMGasState<'_>, MajikOverlay<'_>, CallCreateTrap> =
                DispatchEtable::runtime();
            let resolver = EtableResolver::new(&(), &etable);
            let invoker = Invoker::new(&resolver);

            log_info("  🔨 Matching call_create...");
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
                gas_price: TransactGasPrice::Legacy(U256::zero()), // Disable vendor charging; JAM model charges manually
                value: decoded.value,
                call_create,
                access_list: Vec::new(),
                config,
            };

            // Execute transaction with JAM gas model (zero-cost opcodes + JAM host measurement)
            let result = evm::transact(args, None, &mut overlay, &invoker);

            // Log transaction result details
            match &result {
                Ok(tx_result) => {
                    log_info(&format!(
                        "  ✅ Transaction {} completed successfully Used gas: {}",
                        tx_index, tx_result.used_gas
                    ));

                    use evm::standard::TransactValueCallCreate;
                    match &tx_result.call_create {
                        TransactValueCallCreate::Call { succeed, retval } => {
                            log_info(&format!("    Call result: {:?} Return data length: {} bytes", succeed, retval.len()));
                            if !retval.is_empty() {
                                if retval.len() <= 64 {
                                    log_info(&format!(
                                        "    Return data: 0x{}",
                                        hex::encode(retval)
                                    ));
                                } else {
                                    log_info(&format!(
                                        "    Return data (first 64 bytes): 0x{}",
                                        hex::encode(&retval[..64])
                                    ));
                                    log_info(&format!(
                                        "    Return data (last 32 bytes): 0x{}",
                                        hex::encode(&retval[retval.len() - 32..])
                                    ));
                                }
                            }
                        }
                        TransactValueCallCreate::Create { succeed, address } => {
                            log_info(&format!("    Create result: {:?} Contract address: {:?}", succeed, address));

                        }
                    }
                }
                Err(e) => {
                    log_error(&format!("  ❌ Transaction {} failed: {:?}", tx_index, e));
                }
            }

            // Log shard entries after transaction completion
            //log_cached_shard_entries(&overlay);

            match result {
                Ok(tx_result) => {
                    match overlay.commit_transaction() {
                        Ok(_) => {
                            log_info(&format!("  Transaction {} committed", tx_index));
                            _committed = true;
                            let logs = overlay.take_transaction_logs(tx_index);
                            let log_count = logs.len() as u32;

                            // JAM Gas Model: Get gas consumption from JAMGasState (zero-cost opcodes + JAM host measurement)
                            let jam_gas_used = tx_result.used_gas; // This is now JAM gas, not vendor gas
                            let _vendor_gas_reported = tx_result.used_gas; // Same value since vendor opcodes cost 0

                            // Accumulate JAM gas across all successful transactions
                            total_jam_gas_used =
                                total_jam_gas_used.saturating_add(jam_gas_used.as_u64());

                            log_info(&format!(
                                "  ⛽ JAM Gas: used={} total_so_far={} (zero-cost opcodes + host measurement)",
                                jam_gas_used, total_jam_gas_used
                            ));

                            // Charge caller based on JAM gas consumption only
                            let gas_price = decoded.gas_price;
                            let gas_fee = jam_gas_used.saturating_mul(gas_price);
                            let coinbase = overlay.block_coinbase();

                            // Simple charging: caller pays JAM gas, coinbase receives it
                            match overlay.withdrawal(decoded.caller, gas_fee) {
                                Ok(_) => {
                                    overlay.deposit(coinbase, gas_fee);
                                    log_debug(&format!(
                                        "  💰 Charged JAM gas fee: {} wei to coinbase",
                                        gas_fee
                                    ));
                                }
                                Err(_) => {
                                    log_error(&format!(
                                        "  ⚠️  Caller insufficient balance for JAM gas fee (need {})",
                                        gas_fee
                                    ));
                                }
                            }

                            receipts.push(TransactionReceiptRecord {
                                hash: tx_hash,
                                success: true,
                                used_gas: jam_gas_used, // Pure JAM gas measurement
                                cumulative_gas: total_jam_gas_used as u32,
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

                    // JAM Gas Model: Even on revert, charge JAM gas consumed up to failure point
                    // The JAMGasState tracks consumption throughout execution
                    // Note: Since we can't easily access the state here after error, we use a conservative approach
                    let jam_gas_used = decoded.gas_limit; // Conservative: charge full limit on error

                    // Accumulate JAM gas across all transactions (including failed ones)
                    total_jam_gas_used = total_jam_gas_used.saturating_add(jam_gas_used.as_u64());

                    log_info(&format!(
                        "  ⛽ JAM Gas (reverted): used={} total_so_far={} (conservative charging)",
                        jam_gas_used, total_jam_gas_used
                    ));

                    let gas_price = decoded.gas_price;
                    let gas_fee = jam_gas_used.saturating_mul(gas_price);
                    let coinbase = overlay.block_coinbase();

                    match overlay.withdrawal(decoded.caller, gas_fee) {
                        Ok(_) => {
                            // Pay the gas fee to coinbase (conserve total supply)
                            overlay.deposit(coinbase, gas_fee);
                            log_debug(&format!(
                                "  💰 Charged JAM gas fee (reverted): {} wei to coinbase",
                                gas_fee
                            ));
                        }
                        Err(_) => {
                            log_error(&format!(
                                "  ⚠️  Caller insufficient balance for JAM gas fee (need {} = {} x {})",
                                gas_fee, jam_gas_used, gas_price
                            ));
                        }
                    }

                    receipts.push(TransactionReceiptRecord {
                        hash: tx_hash,
                        success: false,
                        used_gas: jam_gas_used, // Conservative gas charging on error
                        cumulative_gas: total_jam_gas_used as u32,
                        log_index_start: total_log_count,
                        tx_index: tx_index as u32,
                        logs: Vec::new(),
                        payload: extrinsic.clone(),
                    });
                    // Note: total_log_count doesn't change since failed tx has no logs
                }
            }
        }

        let (effects, backend) = overlay.deconstruct(
            refine_args.wphash,
            &receipts,
            refine_context.state_root,
            refine_context.lookup_anchor_slot,
            total_jam_gas_used,
            service_id,
        );

        log_info(&format!(
            "🎯 JAM Gas Summary: total_consumed={} transactions={} total_writes={}",
            total_jam_gas_used,
            receipts.len(),
            effects.write_intents.len()
        ));

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
        extrinsics: &[Vec<u8>],
        refine_context: &utils::functions::RefineContext,
        refine_args: &RefineArgs,
    ) -> Option<ExecutionEffects> {
        use utils::effects::StateWitness;

        // Parse payload metadata
        let metadata = parse_payload_metadata(&work_item.payload)?;
        log_info(&format!(
            "📦 Payload metadata: type={:?}, tx_count={}, witness_count={}, extrinsics={}",
            metadata.payload_type,
            metadata.payload_size,
            metadata.witness_count,
            extrinsics.len()
        ));
        // Only process witnesses for PayloadTransaction/PayloadBuilder
        let witness_start_idx = metadata.payload_size as usize;
        let mut block_builder = BlockRefiner::new();
        let mut imported_objects: BTreeMap<ObjectId, (ObjectRef, Vec<u8>)> = BTreeMap::new();

        for (idx, extrinsic) in extrinsics.iter().enumerate() {
            // Skip transaction extrinsics
            if idx < witness_start_idx {
                continue;
            }
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
                    extrinsic,
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
                            imported_objects.insert(object_id, (object_ref, payload));
                            // let payload_len = payload.len();
                            // log_info(&format!(
                            //     "✅ Payload{}: Verified + imported object {} (payload_length={})",
                            //     payload_type_str,
                            //     format_object_id(object_id),
                            //     payload_len
                            // ));
                        } else {
                            log_info(&format!(
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

        log_info(&format!(
            "📥 Refine: Set up block builder for {} objects",
            imported_objects.len()
        ));

        // Create environment/backend from work item context
        use crate::state::MajikBackend;
        use evm::standard::Config;

        let config = Config::shanghai();
        let environment = Self::create_environment(work_item.service, metadata.payload_type);
        let mut backend = MajikBackend::new(environment, imported_objects.clone(), metadata.payload_type);

        // Populate block_builder.objects_map with imported_objects for 'B' command access
        block_builder.objects_map = imported_objects.clone();

        // Execute based on payload type
        let mut execution_effects = if metadata.payload_type == PayloadType::Call {
            // Call mode - return early from refine_call_payload
            return Self::refine_call_payload(backend, &config, extrinsics);
        } else if metadata.payload_type == PayloadType::Genesis {
            // Genesis mode - process K extrinsics as storage writes
            match Self::refine_genesis_payload(
                backend,
                extrinsics,
                refine_args,
                refine_context,
                work_item.service,
            ) {
                Some((effects, returned_backend)) => {
                    backend = returned_backend;
                    effects
                }
                None => {
                    log_error("Refine: refine_genesis_payload failed");
                    return None;
                }
            }
        } else if metadata.payload_type == PayloadType::Transactions
            || metadata.payload_type == PayloadType::Builder
        {
            // Execute transactions
            let mut block_builder = block_builder;
            match block_builder.refine_payload_transactions(
                backend,
                &config,
                extrinsics,
                metadata.payload_size,
                refine_args,
                refine_context,
                work_item.service,
            ) {
                Some((effects, returned_backend)) => {
                    backend = returned_backend;
                    effects
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

        // Debug log the write intents we are about to export so we can catch corruption early.
        // log_info(&format!(
        //     "📦 Pre-export write intents: {} entries",
        //     execution_effects.write_intents.len()
        // ));
        // for (idx, intent) in execution_effects.write_intents.iter().enumerate() {
        //     log_info(&format!(
        //         "  intent[{}]: object_id={}, payload_len={}, kind={}",
        //         idx,
        //         utils::functions::format_object_id(intent.effect.object_id),
        //         intent.effect.ref_info.payload_length,
        //         intent.effect.ref_info.object_kind
        //     ));
        // }

        // Export payloads to DA segments (receipts, code, storage, block, etc.)
        let mut export_count: u16 = 0;
        for (idx, intent) in execution_effects.write_intents.iter_mut().enumerate() {
            // Export using a cloned WriteEffectEntry so that any host-side memory
            // corruption (export_effect mutates payload buffers) does not clobber
            // the original intent metadata we still need later for meta-shards.
            let mut export_entry = WriteEffectEntry {
                object_id: intent.effect.object_id,
                ref_info: intent.effect.ref_info.clone(),
                payload: intent.effect.payload.clone(),
            };

            match export_entry.export_effect(export_count as usize) {
                Ok(next_index) => {
                    // let num_segments = (export_entry.ref_info.payload_length as u64
                    //     + utils::constants::SEGMENT_SIZE
                    //     - 1)
                    //     / utils::constants::SEGMENT_SIZE;
                    // log_info(&format!(
                    //     "📤 Export: object_id={} start_index={} num_segments={} payload_length={}",
                    //     utils::functions::format_object_id(export_entry.object_id),
                    //     export_count,
                    //     num_segments,
                    //     export_entry.ref_info.payload_length
                    // ));
                    // log_info(&format!(
                    //     "  ↪ export_result[{}]: next_index={}",
                    //     idx, next_index
                    // ));
                    export_count = next_index;

                    // Update the snapshot ObjectRef with the index assigned during export
                    if let Some((_, object_ref)) = meta_object_refs.get_mut(idx) {
                        object_ref.index_start = export_entry.ref_info.index_start;
                    }

                    // Also update the actual intent's ObjectRef with the new index_start
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

        // Process meta-shards AFTER export so ObjectRefs have correct index_start values
        let mut meta_write_intents = backend.process_meta_shards_after_export(
            refine_args.wphash,
            &meta_object_refs,
            work_item.service,
        );
        // log_info(&format!(
        //     "📦 Generated {} meta-shard write intents",
        //     meta_write_intents.len()
        // ));

        // Clone meta_write_intents BEFORE export since export_effect may corrupt memory via FFI
        let mut meta_intents_for_output = meta_write_intents.clone();

        // Export meta-shard payloads to DA segments
        for (idx, intent) in meta_write_intents.iter_mut().enumerate() {
            let start_index = export_count;

            match intent.effect.export_effect(start_index as usize) {
                Ok(next_index) => {
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

        execution_effects.write_intents = meta_intents_for_output;
        Some(execution_effects)
    }

    // ===== Payload Processing Functions =====

    /// Refine call payload - handle EstimateGas/Call mode
    pub fn refine_call_payload(
        mut backend: MajikBackend,
        config: &evm::standard::Config,
        extrinsics: &[Vec<u8>],
    ) -> Option<ExecutionEffects> {
        use crate::jam_gas::JAMGasState;
        use evm::interpreter::etable::DispatchEtable;
        use evm::interpreter::trap::CallCreateTrap;
        use evm::standard::{
            EtableResolver, Invoker, TransactArgs, TransactArgsCallCreate, TransactGasPrice,
        };

        genesis::load_precompiles(&mut backend);
        let mut overlay = MajikOverlay::new(backend, &config.runtime);

        if extrinsics.len() != 1 {
            log_error(&format!(
                "❌ Payload Call expects exactly 1 extrinsic, got {}",
                extrinsics.len()
            ));
            return None;
        }

        let extrinsic = &extrinsics[0];

        // Decode unsigned call arguments
        let decoded = match decode_call_args(extrinsic.as_ptr() as u64, extrinsic.len() as u64) {
            Some(d) => d,
            None => {
                log_error("❌ Payload Call: Failed to decode call args");
                return None;
            }
        };

        log_info(&format!(
            "📞 PayloadCall from={:?}, gas_limit={}, value={}",
            decoded.caller, decoded.gas_limit, decoded.value
        ));

        let etable: DispatchEtable<JAMGasState<'_>, MajikOverlay<'_>, CallCreateTrap> =
            DispatchEtable::runtime();
        let resolver = EtableResolver::new(&(), &etable);
        let invoker = Invoker::new(&resolver);

        let call_create = match decoded.call_create {
            DecodedCallCreate::Call { address, data } => {
                log_info(&format!(
                    "  CALL {:?}→{:?}, data_len={}",
                    decoded.caller,
                    address,
                    data.len()
                ));
                TransactArgsCallCreate::Call { address, data }
            }
            DecodedCallCreate::Create { init_code } => {
                log_info(&format!(
                    "  CREATE with {} bytes init_code",
                    init_code.len()
                ));
                TransactArgsCallCreate::Create {
                    init_code,
                    salt: None,
                }
            }
        };

        let args = TransactArgs {
            caller: decoded.caller,
            gas_limit: decoded.gas_limit,
            gas_price: TransactGasPrice::Legacy(U256::zero()), // Zero gas price for read-only calls
            value: decoded.value,
            call_create,
            access_list: Vec::new(),
            config,
        };

        // Execute the call/estimate
        let result = evm::transact(args, None, &mut overlay, &invoker);

        match result {
            Ok(tx_result) => {
                let gas_used = tx_result.used_gas.as_u64();
                // Extract output from call result
                let output = match tx_result.call_create {
                    evm::standard::TransactValueCallCreate::Call { retval, .. } => retval,
                    evm::standard::TransactValueCallCreate::Create { .. } => Vec::new(),
                };

                log_info(&format!(
                    "✅ Payload Call: Success, gas_used={}, output_len={}, output={}",
                    gas_used,
                    output.len(),
                    format_segment(&output)
                ));

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
                    },
                    dependencies: Vec::new(),
                };

                // Create ExecutionEffects and return
                let effects = ExecutionEffects {
                    write_intents: vec![write_intent],
                };
                Some(effects)
            }
            Err(error) => {
                log_error(&format!("❌ PayloadCall failed: {:?}", error));
                None
            }
        }
    }

    /// Refine genesis payload - process K extrinsics as storage writes
    ///
    /// Genesis extrinsics are K commands: [address:20][storage_key:32][value:32]
    /// These are applied as direct storage writes without EVM execution
    pub fn refine_genesis_payload(
        mut backend: MajikBackend,
        extrinsics: &[Vec<u8>],
        refine_args: &RefineArgs,
        refine_context: &utils::functions::RefineContext,
        service_id: u32,
    ) -> Option<(ExecutionEffects, MajikBackend)> {
        use evm::standard::Config;

        log_info(&format!(
            "🌱 Genesis: Processing {} K extrinsics",
            extrinsics.len()
        ));

        genesis::load_precompiles(&mut backend);
        let config = Config::shanghai();
        let mut overlay = MajikOverlay::new(backend, &config.runtime);

        // Parse and apply each K extrinsic as storage write
        for (idx, extrinsic) in extrinsics.iter().enumerate() {
            // K extrinsic format: [address:20][storage_key:32][value:32] = 84 bytes
            if extrinsic.len() != 84 {
                log_error(&format!(
                    "Genesis: K extrinsic {} has invalid length: {} (expected 84)",
                    idx,
                    extrinsic.len()
                ));
                return None;
            }

            // Extract address (first 20 bytes)
            let mut address_bytes = [0u8; 20];
            address_bytes.copy_from_slice(&extrinsic[0..20]);
            let address = H160(address_bytes);

            // Extract storage key (next 32 bytes)
            let mut key = [0u8; 32];
            key.copy_from_slice(&extrinsic[20..52]);

            // Extract value (final 32 bytes)
            let mut value = [0u8; 32];
            value.copy_from_slice(&extrinsic[52..84]);

            log_debug(&format!(
                "Genesis: K extrinsic {}: address={:?} key={:?} value={:?}",
                idx, address, key, value
            ));

            // Apply storage write directly to overlay
            let _ = overlay.set_storage(address, key.into(), value.into());
        }

        log_info(&format!(
            "🌱 Genesis: Applied {} storage writes",
            extrinsics.len()
        ));

        // Deconstruct overlay to create storage SSR objects
        let (effects, backend) = overlay.deconstruct(
            refine_args.wphash,
            &[], // No receipts for genesis
            refine_context.state_root,
            refine_context.lookup_anchor_slot,
            0, // No gas for genesis
            service_id,
        );

        // Count different object types
        let mut code_count = 0;
        let mut storage_shard_count = 0;
        let mut storage_ssr_count = 0;
        let mut receipt_count = 0;
        let mut meta_shard_count = 0;
        let mut block_count = 0;
        let mut unknown_count = 0;

        for intent in &effects.write_intents {
            match intent.effect.ref_info.object_kind {
                kind if kind == crate::sharding::ObjectKind::Code as u8 => code_count += 1,
                kind if kind == crate::sharding::ObjectKind::StorageShard as u8 => {
                    storage_shard_count += 1
                }
                kind if kind == crate::sharding::ObjectKind::SsrMetadata as u8 => {
                    storage_ssr_count += 1
                }
                kind if kind == crate::sharding::ObjectKind::Receipt as u8 => receipt_count += 1,
                kind if kind == crate::sharding::ObjectKind::MetaShard as u8 => {
                    meta_shard_count += 1
                }
                kind if kind == crate::sharding::ObjectKind::Block as u8 => block_count += 1,
                _ => unknown_count += 1,
            }
        }

        log_info(&format!(
            "🌱 Genesis: Created {} write intents (Code: {}, Shards: {}, SSR: {}, Receipts: {}, Meta: {}, Block: {}, Unknown: {}) - Meta-shard processing will occur after export.",
            effects.write_intents.len(),
            code_count,
            storage_shard_count,
            storage_ssr_count,
            receipt_count,
            meta_shard_count,
            block_count,
            unknown_count
        ));

        Some((effects, backend))
    }
}
