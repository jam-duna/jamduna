//! ExecutionEffects conversion for JAM DA export
//!
//! This module contains the to_execution_effects method that converts
//! OverlayedChangeSet → ExecutionEffects for DA export at accumulate phase.
//!
//! Handles conversion of:
//! - Storage writes → Serialized shards (count + 64B entries)
//! - Balance/nonce writes → mirrored into standard storage shards (0x01 contract)
//! - Code writes → Code objects with version tracking
//! - SSR updates → Serialized SSR (28B header + 9B entries)
//! - Account deletions → Code tombstones (empty payload)
//! - Logs → Receipt objects

use crate::receipt::{TransactionReceiptRecord, receipt_object_id_from_receipt, serialize_receipt};
use crate::contractsharding::{
    ContractShard, ContractStorage, EvmEntry, ShardId,
};
use crate::contractsharding::{code_object_id, contract_shard_object_id};
use crate::state::MajikBackend;
use alloc::{
    collections::{BTreeMap, BTreeSet},
    format,
    vec::Vec,
};
use primitive_types::U256;
use evm::backend::OverlayedChangeSet;
use primitive_types::{H160, H256};
use utils::effects::{WriteEffectEntry, WriteIntent};
use utils::functions::{log_debug, log_info};
use utils::objects::{ObjectId, ObjectRef};
use crate::format_object_id;
impl MajikBackend {
    /// Build a mapping from object IDs to transaction indices that wrote to them
    /// This tracking (of which transactions modified which objects) is critical for
    /// conflict detection and resolution during the accumulate phase.
    /// TODO: serialize this mapping into refine → accumulate output and use it!
    #[allow(dead_code)]
    fn build_object_tx_map(
        &self,
    ) -> BTreeMap<[u8; 32], BTreeSet<usize>> {
        let object_tx_map: BTreeMap<[u8; 32], BTreeSet<usize>> = BTreeMap::new();
        // Note: tracker_output no longer available - function is unused anyway
        /*for write in &tracker_output.writes {
            let tx_index = write.tx_index;
            match &write.key.kind {
                utils::tracking::WriteKind::Storage(_key) => {
                    let object_id = contract_shard_object_id(write.key.address);
                    object_tx_map.entry(object_id).or_default().insert(tx_index);
                }
                utils::tracking::WriteKind::TransientStorage(_) => {}
                utils::tracking::WriteKind::Balance => {
                    let system_contract = h160_from_low_u64_be(0x01);
                    let object_id = contract_shard_object_id(system_contract);
                    object_tx_map.entry(object_id).or_default().insert(tx_index);
                }
                utils::tracking::WriteKind::Nonce => {
                    let system_contract = h160_from_low_u64_be(0x01);
                    let object_id = contract_shard_object_id(system_contract);
                    object_tx_map.entry(object_id).or_default().insert(tx_index);
                }
                utils::tracking::WriteKind::Code => {
                    let object_id = code_object_id(write.key.address);
                    object_tx_map.entry(object_id).or_default().insert(tx_index);
                }
                utils::tracking::WriteKind::StorageReset => {
                    // Post-SSR: No SSR metadata to track
                }
                utils::tracking::WriteKind::Delete => {
                    let code_object = code_object_id(write.key.address);
                    object_tx_map
                        .entry(code_object)
                        .or_default()
                        .insert(tx_index);
                    // Post-SSR: No SSR metadata to track
                }
                utils::tracking::WriteKind::Create => {
                    // Post-SSR: No SSR metadata to track
                }
                utils::tracking::WriteKind::Log => {}
            }
        }*/

        object_tx_map
    }

    /// Prepare storage changes by address and shard.
    /// Returns the grouped storage mutations and the set of addresses whose SSR metadata needs updating.
    fn prepare_storage_changes(
        &self,
        change_set: &OverlayedChangeSet,
        _write_tx_index: &BTreeMap<crate::state::WriteKey, u32>,
        _work_package_hash: [u8; 32],

        _write_intents: &mut Vec<WriteIntent>,
    ) -> (
        BTreeMap<H160, Vec<(H256, H256)>>,  // Post-SSR: Single shard per address
        BTreeSet<H160>,
    ) {
        // Post-SSR: Group by address only (single root shard per contract)
        let mut storage_by_address: BTreeMap<H160, Vec<(H256, H256)>> = BTreeMap::new();
        for ((address, key), value) in &change_set.storages {
            storage_by_address
                .entry(*address)
                .or_insert_with(Vec::new)
                .push((*key, *value));
        }

        // NOTE: In Verkle mode, balance/nonce changes are written directly to the Verkle tree
        // as BasicData leaf values by the Go overlay commit, NOT as contract storage to 0x01.
        // This legacy USDM-based mirroring is disabled in Verkle mode.
        // The balance/nonce caches are still updated in to_execution_effects (lines 629-640)
        // to maintain consistency for subsequent reads within the same block.

        // VERKLE MODE: Skip contract 0x01 storage writes (handled by Verkle tree directly)
        // if !change_set.balances.is_empty() || !change_set.nonces.is_empty() {
        //     let system_contract = h160_from_low_u64_be(0x01);
        //     ... (commented out for Verkle mode)
        // }

        let contract_addresses = storage_by_address.keys().copied().collect::<BTreeSet<_>>();

        (storage_by_address, contract_addresses)
    }

    /// Process meta-shards AFTER export: group ObjectRef writes by meta-shard and export to DA
    /// `object_refs` contains the ObjectId/ObjectRef pairs for the exported writes with correct index_start values.
    /// Returns the number of meta-shard write intents added
    pub fn process_meta_shards_after_export(
        &self,
        work_package_hash: [u8; 32],
        object_refs: &[(ObjectId, ObjectRef)],
        service_id: u32,
        payload_type: u8,
    ) -> Vec<WriteIntent> {
        use crate::meta_sharding::meta_shard_object_id;
        use crate::contractsharding::ObjectKind;
        use utils::effects::WriteEffectEntry;

        // Collect all (object_id, ObjectRef) pairs from write_intents
        // NOTE: At this point ref_info.index_start has correct values since exports have already happened.
        let object_writes: Vec<([u8; 32], utils::objects::ObjectRef)> = object_refs
            .iter()
            .map(|(object_id, object_ref)| (*object_id, object_ref.clone()))
            .collect();

        // Get meta-shard state from backend (persistent across refine calls)
        let mut cached_meta_shards = self.meta_shards_mut();
        let mut meta_ssr = self.meta_ssr_mut();


        // Process object writes - groups by meta-shard, handles splits
        let meta_shard_writes = crate::meta_sharding::process_object_writes(
            object_writes,
            &mut cached_meta_shards,
            &mut meta_ssr,
            service_id,
            payload_type,
        );
        let mut meta_write_intents = Vec::with_capacity(meta_shard_writes.len());

        // Export meta-shard segments to DA and create write intents
        for meta_write in meta_shard_writes {
            // Serialize meta-shard to DA segment format with shard_id header
            // Note: merkle_root was already computed in process_meta_shards
            let merkle_root = crate::meta_sharding::compute_entries_bmt_root(&meta_write.entries);
            let meta_shard = crate::meta_sharding::MetaShard {
                merkle_root,
                entries: meta_write.entries.clone(),
            };
            let meta_shard_bytes = meta_shard.serialize_with_id(meta_write.shard_id, crate::meta_sharding::META_SHARD_MAX_ENTRIES);

            // Compute object_id for this meta-shard
            let object_id = meta_shard_object_id(
                service_id,
                meta_write.shard_id.ld,
                &meta_write.shard_id.prefix56,
            );

            // log_info(&format!(
            //     "📤 EXPORTING MetaShard: object_id={}, shard_id=(ld={}, prefix={:02x}{:02x}...), {} entries, payload_len={}, object_kind={}",
            //     crate::contractsharding::format_object_id(&object_id),
            //     meta_write.shard_id.ld,
            //     meta_write.shard_id.prefix56[0], meta_write.shard_id.prefix56[1],
            //     meta_write.entries.len(),
            //     meta_shard_bytes.len(),
            //     ObjectKind::MetaShard as u8
            // ));

            // Create ObjectRef for this meta-shard
            let object_ref = utils::objects::ObjectRef::new(
                work_package_hash,
                meta_shard_bytes.len() as u32,
                ObjectKind::MetaShard as u8,
            );


            // Create write intent
            meta_write_intents.push(WriteIntent {
                effect: WriteEffectEntry {
                    object_id,
                    ref_info: object_ref,
                    payload: meta_shard_bytes,
                    tx_index: 0,  // TODO: Track per-transaction writes to attribute correctly
                },
            });
        }

        // Note: MetaSSR (global_depth routing table) is NOT exported to DA.
        // Accumulate writes global_depth hint directly to JAM State SSR key.
        // The backend tracks meta_ssr internally for split detection only.
        // log_info(&format!(
        //     "📝 Meta-SSR state: {} routing entries (tracked internally for splits)",
        //     meta_ssr.entries.len()
        // ));

        meta_write_intents
    }


    fn emit_receipts_and_block(
        &self,
        receipts: &[TransactionReceiptRecord],
        work_package_hash: [u8; 32],
        write_intents: &mut Vec<WriteIntent>,
        verkle_root: [u8; 32],
        timeslot: u32,
        total_gas_used: u64,
    ) {
        use crate::receipt::encode_canonical_receipt_rlp;
        use utils::functions::log_info;
        use utils::hash_functions::keccak256;


        // Collect transaction hashes and receipt hashes for block assembly
        let mut tx_hashes = Vec::with_capacity(receipts.len());
        let mut receipt_hashes = Vec::with_capacity(receipts.len());
        let mut cumulative_gas = 0u64;

        // Phase 1: Export receipt payloads to DA and collect hashes
        // Receipt ObjectID is the transaction hash (no modification)
        // The receipt payload contains all transaction data, so we don't export transactions separately
        for record in receipts {
            // Collect transaction hash
            tx_hashes.push(record.hash);

            cumulative_gas += record.used_gas.low_u64();

            // Compute canonical receipt RLP hash for block
            let receipt_rlp = encode_canonical_receipt_rlp(record, cumulative_gas);
            let receipt_hash = keccak256(&receipt_rlp).0;
            receipt_hashes.push(receipt_hash);

            // Export receipt to DA
            let receipt_object_id = receipt_object_id_from_receipt(record);
            let receipt_payload = serialize_receipt(record);

            let receipt_object_ref = utils::objects::ObjectRef::new(
                work_package_hash,
                receipt_payload.len() as u32,
                crate::contractsharding::ObjectKind::Receipt as u8,
            );

            write_intents.push(WriteIntent {
                effect: WriteEffectEntry {
                    object_id: receipt_object_id,
                    ref_info: receipt_object_ref,
                    payload: receipt_payload,
                    tx_index: 0,  // TODO: Track per-transaction writes to attribute correctly
                },
            });
        }


        log_info(&format!(
            "DRAFT BLOCK: {} txns, verkleroot={}, timestamp={}, total_gas_used={}",
            receipts.len(),
            format_object_id(&verkle_root),
            timeslot,
            total_gas_used
        ));

        // Log tx_hashes and receipt_hashes
        for (i, tx_hash) in tx_hashes.iter().enumerate() {
            log_info(&format!("  tx_hash[{}]: {}", i, hex::encode(tx_hash)));
        }
        for (i, receipt_hash) in receipt_hashes.iter().enumerate() {
            log_info(&format!("  receipt_hash[{}]: {}", i, hex::encode(receipt_hash)));
        }

        // Assemble EvmBlockPayload
        let mut block_payload = crate::block::EvmBlockPayload {
            payload_length: 0, // Will be computed after serialization
            num_transactions: receipts.len() as u32,
            timestamp: timeslot,
            gas_used: total_gas_used,
            verkle_root,
            transactions_root: [0u8; 32], // Will be computed by prepare_for_da_export
            receipt_root: [0u8; 32],      // Will be computed by prepare_for_da_export
            block_access_list_hash: [0u8; 32], // TODO: Compute from BAL in refine
            tx_hashes,
            receipt_hashes,
            verkle_delta: None, // TODO: Extract from post-state witness (see services/evm/docs/VERKLE.md)
        };

        // Compute roots and finalize block
        block_payload.prepare_for_da_export();

        // Compute actual payload length from serialization.
        // Note: The first serialization uses payload_length=0, so we must
        // re-serialize after setting the correct length to keep the header
        // consistent with the ObjectRef payload_length written to JAM State.
        let mut serialized = block_payload.serialize();
        block_payload.payload_length = serialized.len() as u32;
        serialized = block_payload.serialize();

        // log_info(&format!(
        //     "📦 Block assembled: {} txs, {} gas, {} bytes",
        //     block_payload.num_transactions, block_payload.gas_used, block_payload.payload_length
        // ));

        // Export block to DA
        let block_object_ref = utils::objects::ObjectRef::new(
            work_package_hash,
            block_payload.payload_length,
            crate::contractsharding::ObjectKind::Block as u8,
        );

        write_intents.push(WriteIntent {
            effect: WriteEffectEntry {
                object_id: work_package_hash,
                ref_info: block_object_ref,
                payload: serialized,
                tx_index: 0,  // TODO: Track per-transaction writes to attribute correctly
            },
        });
    }

    /// Process account deletions (SELFDESTRUCT) and generate write intents
    /// Returns the number of write intents added
    fn process_account_deletions(
        &self,
        deletes: &BTreeSet<H160>,
        work_package_hash: [u8; 32],
        
        write_intents: &mut Vec<WriteIntent>,
    ) -> usize {
        let initial_count = write_intents.len();
        let deletes_count = deletes.len();

        for address in deletes {
            log_info(&format!("  Account deletion: {:?}", address));
            // Account deletion should clear code, storage, balance, nonce
            // This is already handled by storage_resets, but we should also export tombstone for code object
            let code_object_id_val = code_object_id(*address);

            let object_ref = utils::objects::ObjectRef::new(
                work_package_hash,
                0, // Empty payload = deleted
                crate::contractsharding::ObjectKind::Code as u8,
            );

            log_debug(&format!(
                "  Code deletion: {:?}",
                address,
            ));

            write_intents.push(WriteIntent {
                effect: WriteEffectEntry {
                    object_id: code_object_id_val,
                    ref_info: object_ref,
                    payload: Vec::new(), // Empty payload for deletion
                    tx_index: 0,  // TODO: Track per-transaction writes to attribute correctly
                },
            });
        }

        let added_count = write_intents.len() - initial_count;
        if deletes_count > 0 || added_count > 0 {
            log_debug(&format!(
                "Account deletions: {} changes → {} write intents",
                deletes_count, added_count
            ));
        }

        added_count
    }

    /// Process code changes and generate write intents
    /// Returns the number of write intents added
    fn process_code_changes(
        &self,
        codes: &BTreeMap<H160, Vec<u8>>,
        write_tx_index: &BTreeMap<crate::state::WriteKey, u32>,
        work_package_hash: [u8; 32],

        write_intents: &mut Vec<WriteIntent>,
    ) -> usize {
        let initial_count = write_intents.len();
        let codes_count = codes.len();

        for (address, bytecode) in codes {
            let object_id = code_object_id(*address);

            // Create ObjectRef
            let object_ref = utils::objects::ObjectRef::new(
                work_package_hash,
                bytecode.len() as u32,
                crate::contractsharding::ObjectKind::Code as u8,
            );

            // Lookup tx_index for this code write
            let tx_index = write_tx_index
                .get(&crate::state::WriteKey::Code(*address))
                .copied()
                .unwrap_or(0);

            log_debug(&format!(
                "  Code write: {:?} ({} bytes) tx_index={}",
                address,
                bytecode.len(),
                tx_index
            ));

            write_intents.push(WriteIntent {
                effect: WriteEffectEntry {
                    object_id,
                    ref_info: object_ref,
                    payload: bytecode.clone(),
                    tx_index,
                },
            });
        }

        let added_count = write_intents.len() - initial_count;
        if codes_count > 0 || added_count > 0 {
            log_debug(&format!(
                "Code changes: {} changes → {} write intents",
                codes_count, added_count
            ));
        }

        added_count
    }

    fn process_balance_changes(
        &self,
        balances: &BTreeMap<H160, U256>,
        write_tx_index: &BTreeMap<crate::state::WriteKey, u32>,
        work_package_hash: [u8; 32],
        write_intents: &mut Vec<WriteIntent>,
    ) -> usize {
        let initial_count = write_intents.len();
        let balances_count = balances.len();

        for (address, balance) in balances {
            // Serialize balance: U256 → 32 bytes, then take rightmost 16 bytes
            let mut balance_bytes_full = [0u8; 32];
            balance.to_big_endian(&mut balance_bytes_full);
            let balance_bytes = &balance_bytes_full[16..32]; // Take rightmost 16 bytes

            // Create object_id: [20B address][11B zero][1B kind=0x02]
            let mut object_id = [0u8; 32];
            object_id[0..20].copy_from_slice(address.as_bytes());
            object_id[31] = crate::contractsharding::ObjectKind::Balance as u8;

            // Create ObjectRef
            let object_ref = utils::objects::ObjectRef::new(
                work_package_hash,
                16, // balance is 16 bytes
                crate::contractsharding::ObjectKind::Balance as u8,
            );

            // Lookup tx_index for this balance write
            let tx_index = write_tx_index
                .get(&crate::state::WriteKey::Balance(*address))
                .copied()
                .unwrap_or(0);

            log_debug(&format!(
                "  Balance write: {:?} ({}) tx_index={}",
                address,
                balance,
                tx_index
            ));

            write_intents.push(WriteIntent {
                effect: WriteEffectEntry {
                    object_id,
                    ref_info: object_ref,
                    payload: balance_bytes.to_vec(),
                    tx_index,
                },
            });
        }

        let added_count = write_intents.len() - initial_count;
        if balances_count > 0 || added_count > 0 {
            log_debug(&format!(
                "Balance changes: {} changes → {} write intents",
                balances_count, added_count
            ));
        }

        added_count
    }

    fn process_nonce_changes(
        &self,
        nonces: &BTreeMap<H160, U256>,
        write_tx_index: &BTreeMap<crate::state::WriteKey, u32>,
        work_package_hash: [u8; 32],
        write_intents: &mut Vec<WriteIntent>,
    ) -> usize {
        let initial_count = write_intents.len();
        let nonces_count = nonces.len();

        for (address, nonce) in nonces {
            // Serialize nonce as 8 bytes (big-endian)
            let mut nonce_bytes = [0u8; 8];
            let nonce_u64 = nonce.low_u64();
            nonce_bytes.copy_from_slice(&nonce_u64.to_be_bytes());

            // Create object_id: [20B address][11B zero][1B kind=0x06]
            let mut object_id = [0u8; 32];
            object_id[0..20].copy_from_slice(address.as_bytes());
            object_id[31] = crate::contractsharding::ObjectKind::Nonce as u8;

            // Create ObjectRef
            let object_ref = utils::objects::ObjectRef::new(
                work_package_hash,
                8, // nonce is 8 bytes
                crate::contractsharding::ObjectKind::Nonce as u8,
            );

            // Lookup tx_index for this nonce write
            let tx_index = write_tx_index
                .get(&crate::state::WriteKey::Nonce(*address))
                .copied()
                .unwrap_or(0);

            log_debug(&format!(
                "  Nonce write: {:?} ({}) tx_index={}",
                address,
                nonce,
                tx_index
            ));

            write_intents.push(WriteIntent {
                effect: WriteEffectEntry {
                    object_id,
                    ref_info: object_ref,
                    payload: nonce_bytes.to_vec(),
                    tx_index,
                },
            });
        }

        let added_count = write_intents.len() - initial_count;
        if nonces_count > 0 || added_count > 0 {
            log_debug(&format!(
                "Nonce changes: {} changes → {} write intents",
                nonces_count, added_count
            ));
        }

        added_count
    }

    /// Process a single shard - merges existing + new entries and creates write intent
    /// Post-SSR: No shard splitting, always creates exactly 1 write intent
    /// Returns the number of write intents added (always 1)
    fn process_single_shard(
        &self,
        address: H160,
        shard_id: ShardId,
        entries: Vec<(H256, H256)>,
        work_package_hash: [u8; 32],
        
        write_intents: &mut Vec<WriteIntent>,
    ) -> usize {
        // Build ShardData - merge with existing shard entries
        let mut entries_map: BTreeMap<H256, H256> = BTreeMap::new();

        // First, add all existing entries from the shard (if it exists)
        if let Some(contract_storage) = self.storage_shards.borrow().get(&address) {
            // Post-SSR: Single shard per contract, no shard_id lookup needed
            let existing_shard = &contract_storage.shard;

            for entry in &existing_shard.entries {
                entries_map.insert(entry.key_h, entry.value);
            }
        }

        // Then, overlay the modified entries (last value wins)
        for (key, value) in entries {
            entries_map.insert(key, value);
        }

        let mut shard_entries = Vec::new();
        for (key, value) in entries_map {
            shard_entries.push(EvmEntry { key_h: key, value });
        }
        shard_entries.sort_by_key(|entry| entry.key_h);

        let shard_data = ContractShard {
            entries: shard_entries,
        };

        // Post-SSR: contract_shard_object_id() always uses ld=0 (no shard_id param)
        let object_id = contract_shard_object_id(address);

        let payload = shard_data.serialize();

        // Create ObjectRef for this shard
        let object_ref = utils::objects::ObjectRef::new(
            work_package_hash,
            payload.len() as u32,
            crate::contractsharding::ObjectKind::StorageShard as u8,
        );


        log_debug(&format!(
            "  Shard write: {:?} shard_id={:?} ({} entries)",
            address,
            shard_id,
            shard_data.entries.len(),
        ));

        write_intents.push(WriteIntent {
            effect: WriteEffectEntry {
                object_id,
                ref_info: object_ref,
                payload,
                tx_index: 0,  // TODO: Track per-transaction writes to attribute correctly
            },
        });

        // Update storage_shards with the new shard data so subsequent reads see updated values
        {
            let mut storage_shards_mut = self.storage_shards.borrow_mut();
            let contract_storage = storage_shards_mut
                .entry(address)
                .or_insert_with(|| ContractStorage::new(address));
            // Post-SSR: Single shard per contract, directly assign
            contract_storage.shard = shard_data;
        }

        1
    }

    /// Process storage shard changes and generate write intents
    /// Returns the number of write intents added
    fn process_storage_shards(
        &self,
        change_set: &OverlayedChangeSet,
        write_tx_index: &BTreeMap<crate::state::WriteKey, u32>,
        work_package_hash: [u8; 32],

        write_intents: &mut Vec<WriteIntent>,
    ) -> usize {
        let initial_count = write_intents.len();
        let storage_changes_count = change_set.storages.len();
        let balance_changes_count = change_set.balances.len();
        let nonce_changes_count = change_set.nonces.len();

        // Gather storage changes and track which contracts require SSR metadata updates
        // Phase 4: ssr_addresses no longer used since SSR export is disabled
        let (storage_by_address, _ssr_addresses) = self.prepare_storage_changes(
            change_set,
            write_tx_index,
            work_package_hash,

            write_intents,
        );

        // Post-SSR: Serialize single shard per contract
        let root_shard_id = ShardId::root();
        for (address, entries) in storage_by_address {
            self.process_single_shard(
                address,
                root_shard_id,
                entries,
                work_package_hash,

                write_intents,
            );
        }

        let added_count = write_intents.len() - initial_count;
        let total_changes = storage_changes_count + balance_changes_count + nonce_changes_count;
        if total_changes > 0 || added_count > 0 {
            log_debug(&format!(
                "Storage/balance/nonce changes: {} storage, {} balance, {} nonce → {} write intents",
                storage_changes_count, balance_changes_count, nonce_changes_count, added_count
            ));
        }

        added_count
    }

    /// Convert OverlayedChangeSet + TrackerOutput → ExecutionEffects
    ///
    /// Maps overlay mutations to WriteIntents for DA export by calling helper functions:
    /// 1. `process_storage_resets` - Handle SELFDESTRUCT storage resets
    /// 2. `process_storage_shards` - Serialize storage/balance/nonce changes and export SSR
    /// 3. `process_code_changes` - Export code changes with version tracking
    /// 4. `process_account_deletions` - Export code tombstones for deleted accounts
    /// 5. `emit_receipts` - Export transaction receipts
    ///
    /// Output format (ExecutionEffects):
    /// - `export_count`: Number of exports (currently 0)
    /// - `gas_used`: Gas used (currently 0)
    /// - `write_intents`: Vec<WriteIntent> where each WriteIntent contains:
    ///   - `object_id`: 32-byte ObjectId (shard/code/ssr/receipt)
    ///   - `payload`: Serialized DA segments
    pub fn to_execution_effects(
        &self,
        change_set: &OverlayedChangeSet,
        write_tx_index: &BTreeMap<crate::state::WriteKey, u32>,
        work_package_hash: [u8; 32],
        receipts: &[TransactionReceiptRecord],
        verkle_root: [u8; 32],
        timeslot: u32,
        total_gas_used: u64,
    ) -> utils::effects::ExecutionEffects {
        let balance_count = change_set.balances.len();
        let nonce_count = change_set.nonces.len();
        if let Some((addr, bal)) = change_set.balances.iter().next() {
            log_info(&format!(
                "to_execution_effects: balances={} (first {:?}={})",
                balance_count, addr, bal
            ));
        } else {
            log_info("to_execution_effects: balances=0");
        }
        if let Some((addr, nonce)) = change_set.nonces.iter().next() {
            log_info(&format!(
                "to_execution_effects: nonces={} (first {:?}={})",
                nonce_count, addr, nonce
            ));
        } else {
            log_info("to_execution_effects: nonces=0");
        }

        {
            let mut balance_cache = self.balances.borrow_mut();
            for (address, balance) in &change_set.balances {
                balance_cache.insert(*address, *balance);
            }
        }

        {
            let mut nonce_cache = self.nonces.borrow_mut();
            for (address, nonce) in &change_set.nonces {
                nonce_cache.insert(*address, *nonce);
            }
        }

        let mut write_intents: Vec<WriteIntent> = Vec::new();
        let mut contract_intents: Vec<WriteIntent> = Vec::new();

        // 1. Handle storage resets (SELFDESTRUCT or full contract storage clear)
        // TODO:  export storage shard tombstones if needed


        // 2. Handle storage changes - group by address and shard, export SSR, and process shards
        self.process_storage_shards(
            change_set,
            write_tx_index,
            work_package_hash,

            &mut contract_intents,
        );

        // 3. Handle code changes
        self.process_code_changes(
            &change_set.codes,
            write_tx_index,
            work_package_hash,

            &mut contract_intents,
        );

        // 3a. Handle balance changes
        log_info(&format!("🔧 Processing {} balance changes", change_set.balances.len()));
        self.process_balance_changes(
            &change_set.balances,
            write_tx_index,
            work_package_hash,
            &mut contract_intents,
        );

        // 3b. Handle nonce changes
        log_info(&format!("🔧 Processing {} nonce changes", change_set.nonces.len()));
        self.process_nonce_changes(
            &change_set.nonces,
            write_tx_index,
            work_package_hash,
            &mut contract_intents,
        );

        // 4. Handle account deletions (SELFDESTRUCT)
        self.process_account_deletions(
            &change_set.deletes,
            work_package_hash,
            
            &mut write_intents,
        );

        // 5. Handle receipts and assemble block
        if total_gas_used > 0 {
            log_info(&format!(
                "📝 Emitting block with verkle_root (POST-state root): {}",
                format_object_id(&verkle_root)
            ));
            self.emit_receipts_and_block(
                receipts,
                work_package_hash,
                &mut write_intents,
                verkle_root,
                timeslot,
                total_gas_used,
            );
        }
        // NOTE: Meta-shard processing now happens AFTER export in refiner.rs
        // This ensures ObjectRefs have correct index_start values before being
        // embedded in meta-shard entries.

        utils::effects::ExecutionEffects {
            write_intents,
            contract_intents,
            accumulate_instructions: Vec::new(),
        }
    }
}
