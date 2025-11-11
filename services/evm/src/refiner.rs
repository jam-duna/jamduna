//! Block Refiner - manages block construction and imported objects during refine

use alloc::{collections::BTreeMap, format, string::ToString, vec, vec::Vec};
use primitive_types::{H160, U256};
use evm::interpreter::ExitError;
use evm::backend::{RuntimeBackend, RuntimeEnvironment};
use utils::{
    effects::{ObjectRef, ObjectId, ExecutionEffects, WriteIntent},
    functions::{log_error, log_info, log_debug, format_object_id, format_segment, RefineArgs, WorkItem},
    hash_functions::keccak256,
};
use crate::{
    sharding::{format_data_hex, ObjectKind},
    state::{MajikOverlay, MajikBackend},
    tx::{DecodedCallCreate, decode_call_args, decode_transact_args},
    genesis,
};
use utils::effects::WriteEffectEntry;

/// Payload type discriminator
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum PayloadType {
    Builder,
    Transactions,
    Blocks,
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
/// - 0x00 (Blocks): count = extrinsics_count (bootstrap commands like 'A'/'K')
/// - 0x01 (Call): count = extrinsics_count
/// - 0x02 (Transactions): count = tx_count
/// - 0x03 (Builder): count = tx_count
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
        2 => PayloadType::Blocks,
        3 => PayloadType::Call,
        other => {
            log_error(&format!("Unknown payload type byte: 0x{:02x}", other));
            return None;
        }
    };

    // All payloads: type_byte (1) + count (4 LE) + [witness_count (2 LE)]
    if payload.len() < 5 {
        log_error(&format!("Payload too short for {:?}: len={}, expected >=5", payload_type, payload.len()));
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

    /// Refine Blocks payload - execute bootstrap commands and create DA objects
    /// Returns ExecutionEffects with Code, Shard, and SSR write intents
    pub fn refine_blocks_payload(
        &mut self,
        mut backend: MajikBackend,
        extrinsics: Vec<Vec<u8>>,
        work_package_hash: [u8; 32],
    ) -> Result<ExecutionEffects, utils::functions::HarnessError> {
        use primitive_types::{H160, H256};
        use alloc::collections::BTreeMap;
        use crate::sharding::{
            code_object_id, shard_object_id, ssr_object_id, format_object_id,
            EvmEntry, ShardData, ShardId, SSRData, SSRHeader,
            serialize_shard, serialize_ssr, ObjectKind,
        };

        #[derive(Default)]
        struct ContractBootstrap {
            code: Vec<u8>,
            storage: BTreeMap<[u8; 32], [u8; 32]>,
        }

        let mut contracts: BTreeMap<H160, ContractBootstrap> = BTreeMap::new();

        log_info(&format!("🌱 Processing {} bootstrap extrinsics", extrinsics.len()));

        // Parse bootstrap commands
        for (idx, blob) in extrinsics.iter().enumerate() {
            if blob.is_empty() {
                log_error(&format!("  Extrinsic {} empty, skipping", idx));
                continue;
            }

            match blob[0] {
                b'A' => {
                    // Parse: [0x41][address:20][code_len:4 LE][code:code_len]
                    if blob.len() < 25 {
                        log_error(&format!("  Extrinsic {} 'A' command too short", idx));
                        continue;
                    }
                    let address = H160::from_slice(&blob[1..21]);
                    let code_len =
                        u32::from_le_bytes([blob[21], blob[22], blob[23], blob[24]]) as usize;
                    if blob.len() < 25 + code_len {
                        log_error(&format!("  Extrinsic {} 'A' command payload truncated", idx));
                        continue;
                    }
                    let code = blob[25..25 + code_len].to_vec();
                    log_info(&format!(
                        "  'A' command: Deploy {} bytes code to {:?}",
                        code.len(),
                        address
                    ));
                    contracts.entry(address).or_default().code = code;
                }
                b'K' => {
                    // Parse: [0x4B][address:20][key:32][value:32]
                    if blob.len() != 85 {
                        log_error(&format!(
                            "  Extrinsic {} 'K' command wrong length (expected 85, got {})",
                            idx,
                            blob.len()
                        ));
                        continue;
                    }
                    let address = H160::from_slice(&blob[1..21]);
                    let key = <[u8; 32]>::try_from(&blob[21..53]).unwrap();
                    let value = <[u8; 32]>::try_from(&blob[53..85]).unwrap();
                    log_info(&format!(
                        "  'K' command: Set storage for {:?}, key={:?}, value={:?}",
                        address,
                        format_object_id(&key),
                        format_object_id(&value)
                    ));
                    contracts
                        .entry(address)
                        .or_default()
                        .storage
                        .insert(key, value);
                }
                other => {
                    log_error(&format!("  Extrinsic {} unknown command byte 0x{:02x}", idx, other));
                }
            }
        }

        log_info(&format!("✅ Parsed bootstrap for {} contracts", contracts.len()));

        // Load contracts into backend
        for (address, contract_data) in contracts.iter() {
            if !contract_data.code.is_empty() {
                log_info(&format!(
                    "  Loading code for {:?}: {} bytes",
                    address,
                    contract_data.code.len()
                ));
                backend.load_code(*address, contract_data.code.clone());
                backend.code_versions.insert(*address, 1); // Initial version
            }

            if !contract_data.storage.is_empty() {
                log_info(&format!(
                    "  Loading {} storage entries for {:?}",
                    contract_data.storage.len(),
                    address
                ));
                // Storage will be exported via to_execution_effects
            }
        }

        // Create write intents for Code, Shard, and SSR objects
        let mut write_intents = Vec::new();
        log_info(&format!("📦 Converting {} contracts to DA objects", contracts.len()));

        for (address, contract_data) in contracts.iter() {
            // 1. Code object (if contract has code)
            if !contract_data.code.is_empty() {
                let object_id = code_object_id(*address);

                log_info(&format!(
                    "  📝 Code object for {:?}: {} bytes, object_id={}",
                    address,
                    contract_data.code.len(),
                    format_object_id(&object_id)
                ));

                write_intents.push(WriteIntent {
                    effect: WriteEffectEntry {
                        object_id,
                        ref_info: ObjectRef {
                            service_id: backend.service_id,
                            work_package_hash,
                            index_start: 0,
                            index_end: 0,
                            version: 1,
                            payload_length: contract_data.code.len() as u32,
                            timeslot: 0,
                            gas_used: 0,
                            evm_block: 0,
                            object_kind: ObjectKind::Code as u8,
                            log_index: 0,
                            tx_slot: 0,
                        },
                        payload: contract_data.code.clone(),
                    },
                    dependencies: Vec::new(),
                });
            }

            // 2. Storage objects (if contract has storage)
            if !contract_data.storage.is_empty() {
                // Build storage shard with all entries
                log_info(&format!(
                    "  💾 Building storage shard for {:?} with {} storage entries",
                    address,
                    contract_data.storage.len()
                ));

                let mut entries: Vec<EvmEntry> = Vec::new();
                for (idx, (key, value)) in contract_data.storage.iter().enumerate() {
                    // key is already hashed (from Solidity mapping slot calculation or system key hash)
                    // Don't hash again - use it directly for shard lookup
                    let entry = EvmEntry {
                        key_h: H256::from(*key),
                        value: H256::from(*value),
                    };

                    // Log each storage entry
                    log_info(&format!(
                        "    Entry #{}: key_h={:02x}{:02x}..{:02x}{:02x}, value={:02x}{:02x}..{:02x}{:02x}",
                        idx,
                        key[0],
                        key[1],
                        key[30],
                        key[31],
                        value[0],
                        value[1],
                        value[30],
                        value[31]
                    ));

                    entries.push(entry);
                }

                // Sort by key_h for binary search
                entries.sort_by_key(|e| e.key_h);

                // Create single shard at depth 0 for bootstrap (simple, no tree structure)
                let shard_id = ShardId {
                    ld: 0,
                    prefix56: [0u8; 7],
                };
                let shard_data = ShardData { entries };

                // Serialize and create shard object
                let shard_payload = serialize_shard(&shard_data);
                let shard_object_id = shard_object_id(*address, shard_id);

                log_info(&format!(
                    "  💾 Storage shard for {:?}: {} entries, {} bytes (2 + 64*{}), object_id={}",
                    address,
                    shard_data.entries.len(),
                    shard_payload.len(),
                    shard_data.entries.len(),
                    format_object_id(&shard_object_id)
                ));

                // Dump all entries in the serialized shard
                crate::sharding::dump_entries(&shard_data);

                write_intents.push(WriteIntent {
                    effect: WriteEffectEntry {
                        object_id: shard_object_id,
                        ref_info: ObjectRef {
                            service_id: backend.service_id,
                            work_package_hash,
                            index_start: 0,
                            index_end: 0,
                            version: 1,
                            payload_length: shard_payload.len() as u32,
                            timeslot: 0,
                            gas_used: 0,
                            evm_block: 0,
                            object_kind: ObjectKind::StorageShard as u8,
                            log_index: 0,
                            tx_slot: 0,
                        },
                        payload: shard_payload,
                    },
                    dependencies: Vec::new(),
                });

                // 3. SSR metadata object
                let ssr_header = SSRHeader {
                    global_depth: 0,
                    entry_count: 0, // No exceptions for bootstrap - simple single shard
                    total_keys: shard_data.entries.len() as u32,
                    version: 1,
                };

                let ssr_data = SSRData {
                    header: ssr_header,
                    entries: Vec::new(), // No exceptions - all keys go to single shard at depth 0
                };

                let ssr_payload = serialize_ssr(&ssr_data);
                let ssr_object_id = ssr_object_id(*address);

                log_info(&format!(
                    "  📊 SSR metadata for {:?}: {} total keys, {} bytes, object_id={}",
                    address,
                    ssr_data.header.total_keys,
                    ssr_payload.len(),
                    format_object_id(&ssr_object_id)
                ));

                write_intents.push(WriteIntent {
                    effect: WriteEffectEntry {
                        object_id: ssr_object_id,
                        ref_info: ObjectRef {
                            service_id: backend.service_id,
                            work_package_hash,
                            index_start: 0,
                            index_end: 0,
                            version: 1,
                            payload_length: ssr_payload.len() as u32,
                            timeslot: 0,
                            gas_used: 0,
                            evm_block: 0,
                            object_kind: ObjectKind::SsrMetadata as u8,
                            log_index: 0,
                            tx_slot: 0,
                        },
                        payload: ssr_payload,
                    },
                    dependencies: Vec::new(),
                });
            }
        }

        log_info(&format!(
            "✅ Generated {} write intents for bootstrap",
            write_intents.len()
        ));

        Ok(ExecutionEffects {
            write_intents,
        })
    }

    /// Refine payload transactions - execute transactions and ingest receipts into this BlockRefiner
    /// Returns None if any transaction fails to decode or if commit/revert operations fail
    pub fn refine_payload_transactions(
        &mut self,
        mut backend: MajikBackend,
        config: &evm::standard::Config,
        extrinsics: &[Vec<u8>],
        tx_count_expected: u32,
        refine_args: &RefineArgs,
    ) -> Option<ExecutionEffects> {
        use evm::interpreter::etable::DispatchEtable;
        use evm::interpreter::trap::CallCreateTrap;
        use evm::standard::{
            EtableResolver, Invoker, TransactArgs, TransactArgsCallCreate,
            TransactGasPrice,
        };
        use crate::jam_gas::JAMGasState;
        use crate::receipt::TransactionReceiptRecord;
        use crate::state::MajikOverlay;

        genesis::load_precompiles(&mut backend);
        let mut receipts: Vec<TransactionReceiptRecord> = Vec::new();
        let mut overlay = MajikOverlay::new(backend, &config.runtime);
        let mut total_jam_gas_used: u64 = 0;

        // Execute transactions from extrinsics (only first tx_count_expected, not witnesses)
        let tx_extrinsics = extrinsics.iter().take(tx_count_expected as usize);
        for (tx_index, extrinsic) in tx_extrinsics.enumerate() {
            // Compute transaction hash from RLP-encoded signed transaction
            let tx_hash_bytes = keccak256(extrinsic);
            let mut tx_hash = [0u8; 32];
            tx_hash.copy_from_slice(tx_hash_bytes.as_bytes());

            log_debug(
                &format!(
                    "  Processing extrinsic {}: {} bytes, tx_hash={}",
                    tx_index,
                    extrinsic.len(),
                    format_object_id(tx_hash_bytes.0)
                ),
            );

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

            log_info( &format!("  Decoded: caller={:?}, gas_limit={}, value={}", decoded.caller, decoded.gas_limit, decoded.value));
            overlay.begin_transaction(tx_index);
            let mut _committed = false;
            let etable: DispatchEtable<JAMGasState<'_>, MajikOverlay<'_>, CallCreateTrap> =
                DispatchEtable::runtime();
            let resolver = EtableResolver::new(&(), &etable);
            let invoker = Invoker::new(&resolver);

            log_info( "  🔨 Matching call_create...");
            let call_create = match decoded.call_create {
                DecodedCallCreate::Call { address, ref data } => {
                    if data.len() >= 4 {
                        let mut selector = [0u8; 4];
                        selector.copy_from_slice(&data[0..4]);

                        let mut log_parts = vec![
                            format!("CALL {:?}→{:?}", decoded.caller, address),
                            format!("calldata={}", format_data_hex(&data)),
                            format!("sel={}", format_data_hex(&selector)),
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

                        log_debug( &format!("  {}", log_parts.join(", ")));
                    } else {
                        log_debug(
                            &format!(
                                "  CALL {:?}→{:?}, calldata={}",
                                decoded.caller,
                                address,
                                format_data_hex(&data)
                            ),
                        );
                    }
                    TransactArgsCallCreate::Call { address, data: data.clone() }
                }
                DecodedCallCreate::Create { ref init_code } => {
                    log_debug(&format!("  CREATE with {} bytes init_code", init_code.len()));
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
            log_info( &format!("  ✅ Transaction {} completed", tx_index));

            match result {
                Ok(tx_result) => {
                    match overlay.commit_transaction() {
                        Ok(_) => {
                            log_info( &format!("  Transaction {} committed", tx_index));
                            _committed = true;
                            let logs = overlay.take_transaction_logs(tx_index);

                            // JAM Gas Model: Get gas consumption from JAMGasState (zero-cost opcodes + JAM host measurement)
                            let jam_gas_used = tx_result.used_gas; // This is now JAM gas, not vendor gas
                            let _vendor_gas_reported = tx_result.used_gas; // Same value since vendor opcodes cost 0

                            // Accumulate JAM gas across all successful transactions
                            total_jam_gas_used = total_jam_gas_used.saturating_add(jam_gas_used.as_u64());

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
                                tx_index,
                                hash: tx_hash,
                                success: true,
                                used_gas: jam_gas_used, // Pure JAM gas measurement
                                logs,
                                payload: extrinsic.clone(),
                            });
                           
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
                    log_error(&format!("  ❌ Transaction {} failed: {} ({:?})", tx_index, error_detail, error));
                    log_error(&format!("     Caller: {:?}", decoded.caller));
                    log_error(&format!("     Gas limit: {}", decoded.gas_limit));
                    log_error(&format!("     Gas price: {}", decoded.gas_price));
                    log_error(&format!("     Value: {}", decoded.value));
                    match &decoded.call_create {
                        DecodedCallCreate::Call { address, data } => {
                            log_error(&format!("     Call to: {:?}", address));
                            log_error(&format!("     Calldata: {} bytes", data.len()));
                            if data.len() >= 4 {
                                log_error(&format!("     Selector: 0x{:02x}{:02x}{:02x}{:02x}",
                                    data[0], data[1], data[2], data[3]));
                            }
                        }
                        DecodedCallCreate::Create { init_code } => {
                            log_error(&format!("     Create with {} bytes init_code", init_code.len()));
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

                    log_info(
                        &format!(
                            "  ⛽ JAM Gas (reverted): used={} total_so_far={} (conservative charging)",
                            jam_gas_used, total_jam_gas_used
                        ),
                    );

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
                        tx_index,
                        hash: tx_hash,
                        success: false,
                        used_gas: jam_gas_used, // Conservative gas charging on error
                        logs: Vec::new(),
                        payload: extrinsic.clone(),
                    });
                }
            }
        }


        let effects = overlay.deconstruct(refine_args.wphash, &receipts);

        // Note: gas_used field removed from ExecutionEffects - was: total_jam_gas_used

        log_info(
            &format!(
                "🎯 JAM Gas Summary: total_consumed={} transactions={} total_writes={}",
                total_jam_gas_used, receipts.len(), effects.write_intents.len()
            ),
        );
        Some(effects)
    }

    /// Create default environment for a given service (chain_id)
    fn create_environment(service_id: u32, payload_type: PayloadType) -> crate::state::EnvironmentData {
        use primitive_types::{U256, H160};

        // Coinbase address: 0xEaf3223589Ed19bcd171875AC1D0F99D31A5969c
        const COINBASE_ADDRESS: H160 = H160([
            0xEa, 0xf3, 0x22, 0x35, 0x89, 0xEd, 0x19, 0xbc, 0xd1, 0x71,
            0x87, 0x5A, 0xC1, 0xD0, 0xF9, 0x9D, 0x31, 0xA5, 0x96, 0x9c,
        ]);

        crate::state::EnvironmentData {
            block_number: U256::zero(),
            block_timestamp: U256::zero(),
            block_coinbase: COINBASE_ADDRESS,
            block_gas_limit: U256::from(30_000_000u64),
            block_base_fee_per_gas: U256::zero(),
            chain_id: U256::from(service_id),
            block_difficulty: U256::zero(),
            block_randomness: None,
            payload_type,
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
        refine_state_root: [u8; 32],
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
            log_debug(&format!(
                    "🔍 Attempting to decode witness extrinsic idx={} len={}",
                    idx,
                    extrinsic.len()
            ));

            if metadata.payload_type == PayloadType::Transactions || metadata.payload_type == PayloadType::Builder {
                // Parse and verify state witness
                match StateWitness::deserialize_and_verify(extrinsic, refine_state_root) {
                    Ok(state_witness) => {
                        let object_id = state_witness.object_id;
                        let version = state_witness.ref_info.version;
                        let object_ref = state_witness.ref_info.clone();
                        if metadata.payload_type == PayloadType::Transactions {
                            if let Some(payload) = state_witness.fetch_object_payload( work_item, work_item_index) {
                                let payload_len = payload.len();
                                imported_objects.insert(object_id, (object_ref, payload));
                                log_info(&format!("✅ PayloadTransactions: Verified + imported object {} (version {}, payload_length={})",
                                        format_object_id(object_id),  version, payload_len));
                            } else {
                                log_info(&format!("⚠️  PayloadTransactions: Verified witness but could not fetch payload for object {} (version {})",
                                        format_object_id(object_id), version));
                            }
                        } else {
                            if let Some(payload) = state_witness.fetch_object_payload( work_item, work_item_index) {
                                let payload_len = payload.len();
                                imported_objects.insert(object_id, (object_ref, payload));
                                log_info(&format!("✅ PayloadBuilder (WHY???) Verified + imported object {} (version {}, payload_length={})",
                                        format_object_id(object_id), version, payload_len));
                            } else {
                                log_info(&format!("⚠️  PayloadBuilder: Verified witness but could not fetch payload for object {} (version {})",
                                        format_object_id(object_id), version));
                            }
                        }
                    }
                    Err(e) => {
                        log_error(&format!("Failed to deserialize or verify witness at index {}: {:?}", idx, e));
                        return None;
                    }
                }
            } else if metadata.payload_type == PayloadType::Blocks {
                // Parse and verify state witness of RAW format, which do not have object refs
                match StateWitness::deserialize_raw_and_verify(extrinsic, refine_state_root) {
                    Ok(state_witness) => {
                        let object_id = state_witness.object_id;
                        let version = state_witness.ref_info.version;
                        let object_ref = state_witness.ref_info.clone();
                        let payload_len = state_witness.payload.as_ref().map(|p| p.len()).unwrap_or(0);
                        imported_objects.insert(object_id, (object_ref, state_witness.payload.unwrap_or_default()));
                        log_info(&format!("✅ PayloadBlocks: Verified + imported object {} (version {}, payload_length={})",
                            format_object_id(object_id),  version, payload_len));

                    }
                    Err(e) => {
                        log_error(&format!("Failed to deserialize or verify witness at index {}: {:?}", idx, e));
                        return None;
                    }
                }
            } else  if metadata.payload_type == PayloadType::Call {
                // Handle Call payload - early exit handled below
                // No witness processing needed for Call payloads
            }
        }

        log_info(&format!("📥 Refine: Set up block builder for {} objects", imported_objects.len()));

        // Create environment/backend from work item context
        use crate::state::{MajikBackend};
        use evm::standard::Config;

        let config = Config::shanghai();
        let environment = Self::create_environment(work_item.service, metadata.payload_type);
        let backend = MajikBackend::new(environment, imported_objects.clone());

        // Execute based on payload type
        let mut execution_effects = if metadata.payload_type == PayloadType::Blocks {
            // Bootstrap execution
            match block_builder.refine_blocks_payload(backend, extrinsics[..metadata.payload_size as usize].to_vec(), refine_args.wphash) {
                Ok(effects) => effects,
                Err(e) => {
                    log_error(&format!("Refine: Bootstrap execution failed: {:?}", e));
                    return None;
                }
            }
        } else if metadata.payload_type == PayloadType::Call {
            // Call mode - return early from refine_call_payload
            return Self::refine_call_payload(backend, &config, extrinsics);
        } else if metadata.payload_type == PayloadType::Transactions || metadata.payload_type == PayloadType::Builder {
            // Execute transactions
            let mut block_builder = block_builder;
            match block_builder.refine_payload_transactions(
                backend,
                &config,
                extrinsics,
                metadata.payload_size,
                refine_args,
            ) {
                Some(effects) => effects,
                None => {
                    log_error("Refine: refine_payload_transactions failed");
                    return None;
                }
            }
        } else {
            log_error(&format!("❌ Unknown payload type: {:?}", work_item.payload));
            return None;
        };

        // Export payloads to DA segments
        let mut export_count: u16 = 0;
        for (idx, intent) in execution_effects.write_intents.iter_mut().enumerate() {
            match intent.effect.export_effect(export_count as usize) {
                Ok(next_index) => {
                    export_count = next_index;
                }
                Err(e) => {
                    log_error(&format!("  ❌ Failed to export write intent {}: {:?}", idx, e));
                }
            }
        }
        // Note: export_count field removed from ExecutionEffects - was: export_count

        Some(execution_effects)
    }

    // ===== Payload Processing Functions =====

    /// Refine call payload - handle EstimateGas/Call mode
    pub fn refine_call_payload(
        mut backend: MajikBackend,
        config: &evm::standard::Config,
        extrinsics: &[Vec<u8>],
    ) -> Option<ExecutionEffects> {
        use evm::interpreter::etable::DispatchEtable;
        use evm::interpreter::trap::CallCreateTrap;
        use evm::standard::{
            EtableResolver, Invoker, TransactArgs, TransactArgsCallCreate,
            TransactGasPrice,
        };
        use crate::jam_gas::JAMGasState;
    
        genesis::load_precompiles(&mut backend);
        let mut overlay = MajikOverlay::new(backend, &config.runtime);

        if extrinsics.len() != 1 {
            log_error(
                &format!("❌ Payload Call expects exactly 1 extrinsic, got {}", extrinsics.len()),
            );
            return None;
        }

        let extrinsic = &extrinsics[0];

        // Decode unsigned call arguments
        let decoded = match decode_call_args(extrinsic.as_ptr() as u64, extrinsic.len() as u64) {
            Some(d) => d,
            None => {
                log_error( "❌ Payload Call: Failed to decode call args");
                return None;
            }
        };

        log_info(
            &format!(
                "📞 PayloadCall from={:?}, gas_limit={}, value={}",
                decoded.caller, decoded.gas_limit, decoded.value
            ),
        );

        let etable: DispatchEtable<JAMGasState<'_>, MajikOverlay<'_>, CallCreateTrap> =
            DispatchEtable::runtime();
        let resolver = EtableResolver::new(&(), &etable);
        let invoker = Invoker::new(&resolver);

        let call_create = match decoded.call_create {
            DecodedCallCreate::Call { address, data } => {
                log_info(
                    &format!("  CALL {:?}→{:?}, data_len={}", decoded.caller, address, data.len()),
                );
                TransactArgsCallCreate::Call { address, data }
            }
            DecodedCallCreate::Create { init_code } => {
                log_info(
                    &format!("  CREATE with {} bytes init_code", init_code.len()),
                );
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

                log_info(
                    &format!(
                        "✅ Payload Call: Success, gas_used={}, output_len={}, output={}",
                        gas_used,
                        output.len(),
                        format_segment(&output)
                    ),
                );

                // create WriteIntent
                let write_intent = WriteIntent {
                    effect: WriteEffectEntry {
                        object_id: [0xCCu8; 32],
                        ref_info: ObjectRef {
                            service_id: 0,
                            work_package_hash: [0u8; 32],
                            index_start: 0,
                            index_end: 0,
                            version: 0,
                            payload_length: 0,
                            timeslot: 0,
                            gas_used: gas_used as u32,
                            evm_block: 0,
                            object_kind: ObjectKind::Receipt as u8,
                            log_index: 0,
                            tx_slot: 0,
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
                log_error(
                    &format!("❌ PayloadCall failed: {:?}", error),
                );
                None
            }
        }
    }

}

