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
use crate::sharding::{
    ContractStorage, EvmEntry, ShardData, ShardId, serialize_shard, serialize_ssr,
};
use crate::sharding::{code_object_id, shard_object_id, ssr_object_id};
use crate::state::{MajikBackend, balance_storage_key, h160_from_low_u64_be, nonce_storage_key};
use alloc::{
    collections::{BTreeMap, BTreeSet},
    format,
    vec::Vec,
};
use evm::backend::OverlayedChangeSet;
use primitive_types::{H160, H256};
use utils::effects::{WriteEffectEntry, WriteIntent};
use utils::functions::{log_crit, log_debug, log_error, log_info};
use utils::objects::{ObjectId, ObjectRef};
use utils::tracking::TrackerOutput;

impl MajikBackend {
    /// Build a mapping from object IDs to transaction indices that wrote to them
    /// This tracking (of which transactions modified which objects) is critical for
    /// conflict detection and resolution during the accumulate phase.
    /// TODO: serialize this mapping into refine → accumulate output and use it!
    #[allow(dead_code)]
    fn build_object_tx_map(
        &self,
        tracker_output: &TrackerOutput,
    ) -> BTreeMap<[u8; 32], BTreeSet<usize>> {
        let mut object_tx_map: BTreeMap<[u8; 32], BTreeSet<usize>> = BTreeMap::new();
        for write in &tracker_output.writes {
            let tx_index = write.tx_index;
            match &write.key.kind {
                utils::tracking::WriteKind::Storage(key) => {
                    let shard_id = self.resolve_shard_id_impl(write.key.address, *key);
                    let object_id = shard_object_id(write.key.address, shard_id);
                    object_tx_map.entry(object_id).or_default().insert(tx_index);
                }
                utils::tracking::WriteKind::TransientStorage(_) => {}
                utils::tracking::WriteKind::Balance => {
                    let system_contract = h160_from_low_u64_be(0x01);
                    let storage_key = balance_storage_key(write.key.address);
                    let shard_id = self.resolve_shard_id_impl(system_contract, storage_key);
                    let object_id = shard_object_id(system_contract, shard_id);
                    object_tx_map.entry(object_id).or_default().insert(tx_index);
                }
                utils::tracking::WriteKind::Nonce => {
                    let system_contract = h160_from_low_u64_be(0x01);
                    let storage_key = nonce_storage_key(write.key.address);
                    let shard_id = self.resolve_shard_id_impl(system_contract, storage_key);
                    let object_id = shard_object_id(system_contract, shard_id);
                    object_tx_map.entry(object_id).or_default().insert(tx_index);
                }
                utils::tracking::WriteKind::Code => {
                    let object_id = code_object_id(write.key.address);
                    object_tx_map.entry(object_id).or_default().insert(tx_index);
                }
                utils::tracking::WriteKind::StorageReset => {
                    let object_id = ssr_object_id(write.key.address);
                    object_tx_map.entry(object_id).or_default().insert(tx_index);
                }
                utils::tracking::WriteKind::Delete => {
                    let code_object = code_object_id(write.key.address);
                    object_tx_map
                        .entry(code_object)
                        .or_default()
                        .insert(tx_index);
                    let ssr_object = ssr_object_id(write.key.address);
                    object_tx_map
                        .entry(ssr_object)
                        .or_default()
                        .insert(tx_index);
                }
                utils::tracking::WriteKind::Create => {
                    let ssr_object = ssr_object_id(write.key.address);
                    object_tx_map
                        .entry(ssr_object)
                        .or_default()
                        .insert(tx_index);
                }
                utils::tracking::WriteKind::Log => {}
            }
        }

        object_tx_map
    }

    /// Prepare storage changes by address and shard.
    /// Returns the grouped storage mutations and the set of addresses whose SSR metadata needs updating.
    fn prepare_storage_changes(
        &self,
        change_set: &OverlayedChangeSet,
        _work_package_hash: [u8; 32],
        _tracker_output: &TrackerOutput,
        _write_intents: &mut Vec<WriteIntent>,
    ) -> (
        BTreeMap<H160, BTreeMap<ShardId, Vec<(H256, H256)>>>,
        BTreeSet<H160>,
    ) {
        // 2. Handle storage changes - group by address and shard
        let mut storage_by_address: BTreeMap<H160, BTreeMap<ShardId, Vec<(H256, H256)>>> =
            BTreeMap::new();
        for ((address, key), value) in &change_set.storages {
            let shard_id = self.resolve_shard_id_impl(*address, *key);
            storage_by_address
                .entry(*address)
                .or_insert_with(BTreeMap::new)
                .entry(shard_id)
                .or_insert_with(Vec::new)
                .push((*key, *value));
        }

        if !change_set.balances.is_empty() || !change_set.nonces.is_empty() {
            let system_contract = h160_from_low_u64_be(0x01);
            let mut system_shards: BTreeMap<ShardId, Vec<(H256, H256)>> = BTreeMap::new();

            for (address, balance) in &change_set.balances {
                let storage_key = balance_storage_key(*address);
                let mut encoded = [0u8; 32];
                balance.to_big_endian(&mut encoded);
                let value = H256::from(encoded);
                let shard_id = self.resolve_shard_id_impl(system_contract, storage_key);
                system_shards
                    .entry(shard_id)
                    .or_insert_with(Vec::new)
                    .push((storage_key, value));
            }

            for (address, nonce) in &change_set.nonces {
                let storage_key = nonce_storage_key(*address);
                let mut encoded = [0u8; 32];
                nonce.to_big_endian(&mut encoded);
                let value = H256::from(encoded);
                let shard_id = self.resolve_shard_id_impl(system_contract, storage_key);
                system_shards
                    .entry(shard_id)
                    .or_insert_with(Vec::new)
                    .push((storage_key, value));
            }

            if !system_shards.is_empty() {
                let entry = storage_by_address
                    .entry(system_contract)
                    .or_insert_with(BTreeMap::new);
                for (shard_id, entries) in system_shards {
                    let shard_entries = entry.entry(shard_id).or_insert_with(Vec::new);

                    for (key, value) in entries {
                        // Keep existing shard value if contract storage already recorded it.
                        // This prevents mirrored balance writes (used for native token bookkeeping)
                        // from overwriting ERC-20 storage updates that already deducted transfers.
                        if shard_entries
                            .iter()
                            .any(|(existing_key, _)| *existing_key == key)
                        {
                            continue;
                        }
                        shard_entries.push((key, value));
                    }
                }
            }
        }

        let ssr_addresses = storage_by_address.keys().copied().collect::<BTreeSet<_>>();

        (storage_by_address, ssr_addresses)
    }

    /// Process meta-shards AFTER export: group ObjectRef writes by meta-shard and export to DA
    /// `object_refs` contains the ObjectId/ObjectRef pairs for the exported writes with correct index_start values.
    /// Returns the number of meta-shard write intents added
    pub fn process_meta_shards_after_export(
        &self,
        work_package_hash: [u8; 32],
        object_refs: &[(ObjectId, ObjectRef)],
        service_id: u32,
    ) -> Vec<WriteIntent> {
        use crate::meta_sharding::{meta_shard_object_id, serialize_meta_shard_with_id};
        use crate::sharding::ObjectKind;
        use utils::effects::WriteEffectEntry;
/*
        use utils::functions::log_info;
        for (idx, (object_id, object_ref)) in object_refs.iter().enumerate() {
            log_info(&format!(
                "  [{}] object_id={:?}, ref_info={{wph: {:?}, idx_start: {}, payload_len: {}, kind: {}}}",
                idx,
                object_id,
                object_ref.work_package_hash,
                object_ref.index_start,
                object_ref.payload_length,
                object_ref.object_kind
            ));
        }
 */
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
        );
        let mut meta_write_intents = Vec::with_capacity(meta_shard_writes.len());

        // Export meta-shard segments to DA and create write intents
        for meta_write in meta_shard_writes {
            // Serialize meta-shard to DA segment format with shard_id header
            // Note: merkle_root was already computed in process_meta_shards
            let merkle_root = crate::meta_sharding::compute_entries_bmt_root(&meta_write.entries);
            let meta_shard = crate::da::ShardData {
                merkle_root,
                entries: meta_write.entries.clone(),
            };
            let meta_shard_bytes = serialize_meta_shard_with_id(&meta_shard, meta_write.shard_id);

            // Compute object_id for this meta-shard
            let object_id = meta_shard_object_id(
                service_id,
                meta_write.shard_id.ld,
                &meta_write.shard_id.prefix56,
            );

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
                },
                dependencies: Vec::new(),
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

    /// Emit placeholder (v0) receipts as write intents.
    ///
    /// This is the Phase 1 path: it emits version 0 receipts (placeholders)
    /// so refine can ship transaction data before canonical fields are known.
    /// Phase 2 will later replace these with version 1 receipts.
    fn emit_receipts_and_block(
        &self,
        receipts: &[TransactionReceiptRecord],
        work_package_hash: [u8; 32],
        write_intents: &mut Vec<WriteIntent>,
        state_root: [u8; 32],
        timeslot: u32,
        total_gas_used: u64,
    ) {
        use crate::receipt::encode_canonical_receipt_rlp;
        use utils::functions::log_info;
        use utils::hash_functions::keccak256;

        let initial_count = write_intents.len();
        let receipts_count = receipts.len();

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
                crate::sharding::ObjectKind::Receipt as u8,
            );

            write_intents.push(WriteIntent {
                effect: WriteEffectEntry {
                    object_id: receipt_object_id,
                    ref_info: receipt_object_ref,
                    payload: receipt_payload,
                },
                dependencies: Vec::new(),
            });
        }

        let added_count = write_intents.len() - initial_count;
        if receipts_count != added_count {
            log_error(&format!(
                "ERROR: Receipts mismatch: {} receipts → {} write intents",
                receipts_count, added_count
            ));
        } else if receipts_count > 0 {
            log_debug(&format!(
                "Receipts: {} receipts → write intents",
                receipts_count
            ));
        }

        // Assemble EvmBlockPayload
        let mut block_payload = crate::block::EvmBlockPayload {
            payload_length: 0, // Will be computed after serialization
            num_transactions: receipts.len() as u32,
            timestamp: timeslot,
            gas_used: total_gas_used,
            state_root,
            transactions_root: [0u8; 32], // Will be computed by prepare_for_da_export
            receipt_root: [0u8; 32],      // Will be computed by prepare_for_da_export
            tx_hashes,
            receipt_hashes,
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

        log_info(&format!(
            "📦 Block assembled: {} txs, {} gas, {} bytes",
            block_payload.num_transactions, block_payload.gas_used, block_payload.payload_length
        ));

        // Export block to DA
        let block_object_ref = utils::objects::ObjectRef::new(
            work_package_hash,
            block_payload.payload_length,
            crate::sharding::ObjectKind::Block as u8,
        );

        write_intents.push(WriteIntent {
            effect: WriteEffectEntry {
                object_id: work_package_hash,
                ref_info: block_object_ref,
                payload: serialized,
            },
            dependencies: Vec::new(),
        });
    }

    /// Process storage resets (SELFDESTRUCT or full contract storage clear) and generate write intents
    /// Returns the number of write intents added
    fn process_storage_resets(
        &self,
        storage_resets: &BTreeSet<H160>,
        work_package_hash: [u8; 32],
        tracker_output: &TrackerOutput,
        write_intents: &mut Vec<WriteIntent>,
    ) -> usize {
        let initial_count = write_intents.len();
        let changes_count = storage_resets.len();

        for address in storage_resets {
            log_info(&format!("  Storage reset for contract {:?}", address));
            // Reset creates new empty SSR with incremented version
            if let Some(contract_storage) = self.storage_shards.borrow().get(address) {
                let ssr_object_id_val = ssr_object_id(*address);

                // Create new SSR with empty entries but incremented version
                let reset_header = crate::sharding::SSRHeader {
                    global_depth: 0,
                    entry_count: 0,
                    total_keys: 0,
                    version: contract_storage.ssr.header.version + 1,
                };
                let reset_ssr = crate::sharding::ssr_from_parts(reset_header, Vec::new());

                let ssr_payload = serialize_ssr(&reset_ssr);

                let object_ref = utils::objects::ObjectRef::new(
                    work_package_hash,
                    ssr_payload.len() as u32,
                    crate::sharding::ObjectKind::SsrMetadata as u8,
                );

                // Filter out self-dependencies to prevent version conflicts
                let dependencies: Vec<ObjectId> = tracker_output
                    .block_reads
                    .iter()
                    .filter(|dep| **dep != ssr_object_id_val)
                    .cloned()
                    .collect();

                log_debug(&format!(
                    "  SSR reset: {:?} (version={}, {} deps)",
                    address,
                    reset_ssr.header.version,
                    dependencies.len()
                ));

                write_intents.push(WriteIntent {
                    effect: WriteEffectEntry {
                        object_id: ssr_object_id_val,
                        ref_info: object_ref,
                        payload: ssr_payload,
                    },
                    dependencies,
                });
            }
        }

        let added_count = write_intents.len() - initial_count;
        if changes_count > 0 || added_count > 0 {
            log_debug(&format!(
                "Storage resets: {} changes → {} write intents",
                changes_count, added_count
            ));
        }

        added_count
    }

    /// Process account deletions (SELFDESTRUCT) and generate write intents
    /// Returns the number of write intents added
    fn process_account_deletions(
        &self,
        deletes: &BTreeSet<H160>,
        work_package_hash: [u8; 32],
        tracker_output: &TrackerOutput,
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
                crate::sharding::ObjectKind::Code as u8,
            );

            // Filter out self-dependencies to prevent version conflicts
            let dependencies: Vec<ObjectId> = tracker_output
                .block_reads
                .iter()
                .filter(|dep| **dep != code_object_id_val)
                .cloned()
                .collect();

            log_debug(&format!(
                "  Code deletion: {:?} ({} deps)",
                address,
                dependencies.len()
            ));

            write_intents.push(WriteIntent {
                effect: WriteEffectEntry {
                    object_id: code_object_id_val,
                    ref_info: object_ref,
                    payload: Vec::new(), // Empty payload for deletion
                },
                dependencies,
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
        work_package_hash: [u8; 32],
        tracker_output: &TrackerOutput,
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
                crate::sharding::ObjectKind::Code as u8,
            );

            // Collect dependencies - filter out self-dependencies to prevent version conflicts
            let dependencies: Vec<ObjectId> = tracker_output
                .block_reads
                .iter()
                .filter(|dep| **dep != object_id)
                .cloned()
                .collect();

            log_debug(&format!(
                "  Code write: {:?} ({} bytes, {} deps)",
                address,
                bytecode.len(),
                dependencies.len()
            ));

            write_intents.push(WriteIntent {
                effect: WriteEffectEntry {
                    object_id,
                    ref_info: object_ref,
                    payload: bytecode.clone(),
                },
                dependencies,
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

    /// Process a single shard - merges existing + new entries, checks for split
    /// Returns the number of write intents added (1 for normal, 2 for split)
    fn process_single_shard(
        &self,
        address: H160,
        shard_id: ShardId,
        entries: Vec<(H256, H256)>,
        work_package_hash: [u8; 32],
        tracker_output: &TrackerOutput,
        write_intents: &mut Vec<WriteIntent>,
    ) -> usize {
        // Build ShardData - merge with existing shard entries
        let mut entries_map: BTreeMap<H256, H256> = BTreeMap::new();
        let mut previous_entry_count = 0usize;

        // First, add all existing entries from the shard (if it exists)
        if let Some(contract_storage) = self.storage_shards.borrow().get(&address) {
            if let Some(existing_shard) = contract_storage.shards.get(&shard_id) {
                previous_entry_count = existing_shard.entries.len();
                for entry in &existing_shard.entries {
                    entries_map.insert(entry.key_h, entry.value);
                }
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

        let shard_data = ShardData {
            merkle_root: [0u8; 32], // TODO: Compute BMT root for contract shards
            entries: shard_entries,
        };

        // Check if shard needs to be split due to exceeding threshold
        // Use recursive splitter to handle cascading splits
        if let Some((leaf_shards, ssr_entries)) =
            crate::sharding::maybe_split_shard_recursive(shard_id, &shard_data)
        {
            self.handle_shard_split_recursive(
                address,
                shard_id,
                &shard_data,
                leaf_shards,
                ssr_entries,
                work_package_hash,
                write_intents,
                previous_entry_count,
            )
        } else {
            self.handle_normal_shard_write(
                address,
                shard_id,
                shard_data,
                work_package_hash,
                tracker_output,
                write_intents,
            )
        }
    }

    /// Handle recursive shard split - creates write intents for all leaf shards
    /// and updates SSR with internal split entries
    /// Returns the number of write intents added
    fn handle_shard_split_recursive(
        &self,
        address: H160,
        original_shard_id: ShardId,
        original_shard_data: &crate::sharding::ContractShard,
        leaf_shards: Vec<(ShardId, crate::sharding::ContractShard)>,
        ssr_entries: Vec<crate::sharding::SSREntry>,
        work_package_hash: [u8; 32],
        write_intents: &mut Vec<WriteIntent>,
        previous_entry_count: usize,
    ) -> usize {
        let new_entry_count = original_shard_data.entries.len();
        let num_leaf_shards = leaf_shards.len();
        let num_ssr_entries = ssr_entries.len();

        log_info(&format!(
            "Pre-split shard snapshot: shard_id={:?} entries={}",
            original_shard_id, new_entry_count
        ));

        let delta_entries = new_entry_count.saturating_sub(previous_entry_count);
        log_info(&format!(
            "Recursive split triggered - threshold exceeded (prev_entries={}, total_after_updates={}, delta={}) → {} leaf shards, {} SSR entries",
            previous_entry_count, new_entry_count, delta_entries, num_leaf_shards, num_ssr_entries
        ));

        // Verify all leaf shards meet constraints
        const MAX_ENTRIES: usize = 63;
        for (i, (_shard_id, shard_data)) in leaf_shards.iter().enumerate() {
            if shard_data.entries.len() > MAX_ENTRIES {
                log_crit(&format!(
                    "❌ FATAL: Leaf shard {} has {} entries, exceeds hard limit of {}!",
                    i,
                    shard_data.entries.len(),
                    MAX_ENTRIES
                ));
                #[cfg(debug_assertions)]
                panic!("Leaf shard exceeds DA segment limit after recursive split");
            }
        }

        // Remove the original unsplit parent shard to avoid stale data
        {
            let mut storage_shards_mut = self.storage_shards.borrow_mut();
            if let Some(contract_storage) = storage_shards_mut.get_mut(&address) {
                contract_storage.shards.remove(&original_shard_id);
            }
        }

        // Process each leaf shard
        for (i, (shard_id, shard_data)) in leaf_shards.iter().enumerate() {
            let entry_count = shard_data.entries.len();

            log_info(&format!(
                "  Leaf shard {}/{}: ld={}, entries={}",
                i + 1,
                num_leaf_shards,
                shard_id.ld,
                entry_count
            ));

            // Serialize and create write intent for this shard
            let payload = serialize_shard(shard_data);
            let object_id = shard_object_id(address, *shard_id);

            let object_ref = utils::objects::ObjectRef::new(
                work_package_hash,
                payload.len() as u32,
                crate::sharding::ObjectKind::StorageShard as u8,
            );

            write_intents.push(WriteIntent {
                effect: WriteEffectEntry {
                    object_id,
                    ref_info: object_ref,
                    payload,
                },
                dependencies: Vec::new(), // TODO: Add proper dependencies
            });

            // Store shard in local cache
            {
                let mut storage_shards_mut = self.storage_shards.borrow_mut();
                let contract_storage = storage_shards_mut
                    .entry(address)
                    .or_insert_with(|| ContractStorage::new(address));
                contract_storage
                    .shards
                    .insert(*shard_id, shard_data.clone());
            }
        }

        // Update SSR metadata with internal split entries
        // Key insight: binary tree with N leaves has N-1 internal nodes (splits)
        // We only add SSR entries for the internal splits, not for the leaves
        {
            let mut storage_shards_mut = self.storage_shards.borrow_mut();
            let contract_storage = storage_shards_mut
                .entry(address)
                .or_insert_with(|| ContractStorage::new(address));

            for ssr_entry in ssr_entries {
                contract_storage.ssr.entries.push(ssr_entry);
                contract_storage.ssr.header.entry_count += 1;
            }
            contract_storage.ssr.header.version += 1;
        }

        log_info(&format!(
            "✅ Recursive split complete: {} entries → {} leaf shards, {} SSR entries, {} write intents created",
            new_entry_count, num_leaf_shards, num_ssr_entries, num_leaf_shards
        ));

        num_leaf_shards
    }

    /// Handle normal (non-split) shard write
    /// Returns the number of write intents added (always 1)
    fn handle_normal_shard_write(
        &self,
        address: H160,
        shard_id: ShardId,
        shard_data: crate::sharding::ContractShard,
        work_package_hash: [u8; 32],
        tracker_output: &TrackerOutput,
        write_intents: &mut Vec<WriteIntent>,
    ) -> usize {
        let object_id = shard_object_id(address, shard_id);

        // Dump entries for debugging
        // crate::sharding::dump_entries(&shard_data);

        let payload = serialize_shard(&shard_data);

        // Create ObjectRef for this shard
        let object_ref = utils::objects::ObjectRef::new(
            work_package_hash,
            payload.len() as u32,
            crate::sharding::ObjectKind::StorageShard as u8,
        );

        // Collect dependencies - all reads from this shard
        // Filter out self-dependencies (same object_id and version) to prevent version conflicts
        let dependencies: Vec<ObjectId> = tracker_output
            .block_reads
            .iter()
            .filter(|dep| **dep != object_id)
            .cloned()
            .collect();

        log_debug(&format!(
            "  Shard write: {:?} shard_id={:?} ({} entries, {} deps)",
            address,
            shard_id,
            shard_data.entries.len(),
            dependencies.len()
        ));

        write_intents.push(WriteIntent {
            effect: WriteEffectEntry {
                object_id,
                ref_info: object_ref,
                payload,
            },
            dependencies,
        });

        // Update storage_shards with the new shard data so subsequent reads see updated values
        {
            let mut storage_shards_mut = self.storage_shards.borrow_mut();
            let contract_storage = storage_shards_mut
                .entry(address)
                .or_insert_with(|| ContractStorage::new(address));
            contract_storage.shards.insert(shard_id, shard_data);
        }

        1
    }

    /// Export SSR metadata for the provided addresses after shard updates have been applied.
    fn export_updated_ssr_metadata(
        &self,
        addresses: &BTreeSet<H160>,
        work_package_hash: [u8; 32],
        tracker_output: &TrackerOutput,
        write_intents: &mut Vec<WriteIntent>,
    ) {
        for address in addresses {
            let ssr_object_id_val = ssr_object_id(*address);

            let (ssr_payload, ssr_version, ssr_entry_count) = {
                let mut storage_shards = self.storage_shards.borrow_mut();
                let Some(contract_storage) = storage_shards.get_mut(address) else {
                    continue;
                };

                // Update metadata counts based on latest shard contents
                let total_keys: u32 = contract_storage
                    .shards
                    .values()
                    .map(|shard| shard.entries.len() as u32)
                    .sum();
                contract_storage.ssr.header.total_keys = total_keys;
                contract_storage.ssr.header.entry_count = contract_storage.ssr.entries.len() as u32;

                let payload = serialize_ssr(&contract_storage.ssr);
                (
                    payload,
                    contract_storage.ssr.header.version,
                    contract_storage.ssr.entries.len(),
                )
            };

            let object_ref = utils::objects::ObjectRef::new(
                work_package_hash,
                ssr_payload.len() as u32,
                crate::sharding::ObjectKind::SsrMetadata as u8,
            );

            // Filter out self-dependencies to prevent version conflicts
            let dependencies: Vec<ObjectId> = tracker_output
                .block_reads
                .iter()
                .filter(|dep| **dep != ssr_object_id_val)
                .cloned()
                .collect();

            log_debug(&format!(
                "  SSR write: {:?} (version={}, {} entries, {} bytes, {} deps)",
                address,
                ssr_version,
                ssr_entry_count,
                ssr_payload.len(),
                dependencies.len()
            ));

            write_intents.push(WriteIntent {
                effect: WriteEffectEntry {
                    object_id: ssr_object_id_val,
                    ref_info: object_ref,
                    payload: ssr_payload,
                },
                dependencies,
            });
        }
    }

    /// Process storage shard changes and generate write intents
    /// Returns the number of write intents added
    fn process_storage_shards(
        &self,
        change_set: &OverlayedChangeSet,
        work_package_hash: [u8; 32],
        tracker_output: &TrackerOutput,
        write_intents: &mut Vec<WriteIntent>,
    ) -> usize {
        let initial_count = write_intents.len();
        let storage_changes_count = change_set.storages.len();
        let balance_changes_count = change_set.balances.len();
        let nonce_changes_count = change_set.nonces.len();

        // Gather storage changes and track which contracts require SSR metadata updates
        let (storage_by_address, ssr_addresses) = self.prepare_storage_changes(
            change_set,
            work_package_hash,
            tracker_output,
            write_intents,
        );

        // Serialize each modified shard
        for (address, shards) in storage_by_address {
            for (shard_id, entries) in shards {
                self.process_single_shard(
                    address,
                    shard_id,
                    entries,
                    work_package_hash,
                    tracker_output,
                    write_intents,
                );
            }
        }

        // Export SSR metadata after shards reflect the latest mutations
        self.export_updated_ssr_metadata(
            &ssr_addresses,
            work_package_hash,
            tracker_output,
            write_intents,
        );

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
    ///   - `dependencies`: Vec<ObjectId> from TrackerOutput
    pub fn to_execution_effects(
        &self,
        change_set: &OverlayedChangeSet,
        tracker_output: &TrackerOutput,
        work_package_hash: [u8; 32],
        receipts: &[TransactionReceiptRecord],
        state_root: [u8; 32],
        timeslot: u32,
        total_gas_used: u64,
    ) -> utils::effects::ExecutionEffects {
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

        let mut write_intents = Vec::new();

        // 1. Handle storage resets (SELFDESTRUCT or full contract storage clear)
        self.process_storage_resets(
            &change_set.storage_resets,
            work_package_hash,
            &tracker_output,
            &mut write_intents,
        );

        // 2. Handle storage changes - group by address and shard, export SSR, and process shards
        self.process_storage_shards(
            change_set,
            work_package_hash,
            &tracker_output,
            &mut write_intents,
        );

        // 3. Handle code changes
        self.process_code_changes(
            &change_set.codes,
            work_package_hash,
            &tracker_output,
            &mut write_intents,
        );

        // 4. Handle account deletions (SELFDESTRUCT)
        self.process_account_deletions(
            &change_set.deletes,
            work_package_hash,
            &tracker_output,
            &mut write_intents,
        );

        // 5. Handle receipts and assemble block
        if total_gas_used > 0 {
            self.emit_receipts_and_block(
                receipts,
                work_package_hash,
                &mut write_intents,
                state_root,
                timeslot,
                total_gas_used,
            );
        }
        // NOTE: Meta-shard processing now happens AFTER export in refiner.rs
        // This ensures ObjectRefs have correct index_start values before being
        // embedded in meta-shard entries.

        utils::effects::ExecutionEffects { write_intents }
    }
}
