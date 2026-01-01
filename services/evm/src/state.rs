//! Runtime trait implementations and MajikOverlay coordination
//!
//! This module contains:
//! - MajikOverlay struct and coordination logic
//! - All EVM runtime trait implementations for MajikBackend and MajikOverlay
//!
//! Organized by trait:
//! - TransactionalBackend - Substate push/pop for MajikOverlay
//! - RuntimeEnvironment - Block metadata (for both MajikBackend and MajikOverlay)
//! - RuntimeBaseBackend - Read operations (for both MajikBackend and MajikOverlay)
//! - RuntimeBackend - Write operations (for both MajikBackend and MajikOverlay)

use alloc::{
    collections::{BTreeMap, BTreeSet},
    format,
    vec::Vec,
};
use core::cell::RefCell;
use evm::MergeStrategy;
use evm::backend::{
    OverlayedBackend, RuntimeBackend as EvmRuntimeBackend, RuntimeEnvironment, TransactionalBackend,
};
use evm::interpreter::ExitError;
use evm::interpreter::runtime::{
    Log, RuntimeBaseBackend, RuntimeConfig, SetCodeOrigin, TouchKind, Transfer,
};
use primitive_types::{H160, H256, U256};
use utils::effects::ObjectId;
use utils::functions::{log_error, log_info};
use utils::objects::ObjectRef;

use crate::contractsharding::{ContractShard, ContractStorage};
use crate::contractsharding::{ObjectKind, code_object_id, contract_shard_object_id};
use crate::receipt::TransactionReceiptRecord;
use crate::witness_events::{VerkleEventTracker, is_precompile, is_system_contract};
use utils::hash_functions::keccak256;

// ===== Type Conversions =====

/// Create H160 address from low 64 bits (big-endian)
pub fn h160_from_low_u64_be(value: u64) -> H160 {
    let mut bytes = [0u8; 20];
    bytes[12..].copy_from_slice(&value.to_be_bytes());
    H160::from_slice(&bytes)
}

/// Convenience helper mirroring primitive-types implementation for tests/utilities
#[allow(dead_code)]
pub fn h256_from_low_u64_be(value: u64) -> H256 {
    let mut bytes = [0u8; 32];
    bytes[24..32].copy_from_slice(&value.to_be_bytes());
    H256::from(bytes)
}

/// Compute storage key for balance in 0x01 precompile
/// Uses Solidity mapping hash: keccak256(abi.encode(address, slot))
/// where slot=0 for balances mapping
pub fn balance_storage_key(address: H160) -> H256 {
    let mut input = [0u8; 64];
    // Left-pad address to 32 bytes (bytes 12-31)
    input[12..32].copy_from_slice(address.as_bytes());
    // Slot 0 for balances mapping (already zeroed)
    let key = keccak256(&input);
    // log_info(&format!(
    //     "🔑 balance_storage_key({:?}) -> {} (Solidity mapping hash)",
    //     address,
    //     crate::contractsharding::format_object_id(&key.0)
    // ));
    key
}

/// Compute storage key for nonce in 0x01 precompile
/// Uses Solidity mapping hash: keccak256(abi.encode(address, slot))
/// where slot=1 for nonces mapping
pub fn nonce_storage_key(address: H160) -> H256 {
    let mut input = [0u8; 64];
    // Left-pad address to 32 bytes (bytes 12-31)
    input[12..32].copy_from_slice(address.as_bytes());
    // Slot 1 for nonces mapping
    input[63] = 0x01;
    let key = keccak256(&input);
    // log_info(&format!(
    //     "🔑 nonce_storage_key({:?}) -> {} (Solidity mapping hash)",
    //     address,
    //     crate::contractsharding::format_object_id(&key.0)
    // ));
    key
}

/// Track storage presence (present vs absent) for witness fill detection
pub fn mark_storage_presence(
    present_set: &mut BTreeSet<(H160, H256)>,
    absent_set: &mut BTreeSet<(H160, H256)>,
    address: H160,
    key: H256,
    present: bool,
) {
    if present {
        present_set.insert((address, key));
        absent_set.remove(&(address, key));
    } else {
        absent_set.insert((address, key));
        present_set.remove(&(address, key));
    }
}

// ===== EnvironmentData =====

/// Environment metadata for EVM execution
pub struct EnvironmentData {
    pub block_number: U256,
    pub block_timestamp: U256,
    pub block_coinbase: H160,
    pub block_gas_limit: U256,
    pub block_base_fee_per_gas: U256,
    pub chain_id: U256,
    pub block_difficulty: U256,
    pub block_randomness: Option<H256>,
    pub service_id: u32, // Service ID for DA imports
}

// ===== MajikBackend =====

/// Execution mode for MajikBackend
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ExecutionMode {
    /// Builder mode: uses Verkle host functions for reads (Go maintains verkleReadLog)
    Builder,
    /// Guarantor mode: uses cache-only reads (panics on miss)
    Guarantor,
}

/// Majik backend with DA-style storage (no InMemoryBackend)
pub struct MajikBackend {
    pub code_storage: RefCell<BTreeMap<H160, Vec<u8>>>, // address -> bytecode
    pub code_hashes: RefCell<BTreeMap<H160, H256>>,     // address -> keccak256(code) cache
    pub negative_code_cache: RefCell<BTreeMap<H160, ()>>, // address -> () for EOAs with no code
    pub storage_shards: RefCell<BTreeMap<H160, ContractStorage>>, // address -> SSR + shards
    pub storage_present: RefCell<BTreeSet<(H160, H256)>>, // Tracks slots proven present
    pub storage_absent: RefCell<BTreeSet<(H160, H256)>>, // Tracks slots proven absent
    pub balances: RefCell<BTreeMap<H160, U256>>,        // address -> balance
    pub nonces: RefCell<BTreeMap<H160, U256>>,          // address -> nonce
    pub environment: EnvironmentData,
    pub service_id: u32, // Service ID for DA imports
    pub imported_objects: RefCell<BTreeMap<ObjectId, (ObjectRef, Vec<u8>)>>, // object_id -> (ref, payload)

    // Meta-shard state (JAM State object index)
    pub meta_ssr: RefCell<crate::meta_sharding::MetaSSR>, // Global SSR for all ObjectRefs
    pub meta_shards: RefCell<BTreeMap<crate::da::ShardId, crate::meta_sharding::MetaShard>>, // Cached meta-shards

    // Verkle mode (builder vs guarantor)
    pub execution_mode: ExecutionMode,

    // Current transaction index (set by MajikOverlay for BAL tracking)
    pub current_tx_index: RefCell<Option<usize>>,
}

impl MajikBackend {
    pub fn new(
        environment: EnvironmentData,
        imported_objects: BTreeMap<ObjectId, (ObjectRef, Vec<u8>)>,
        global_depth: u8,
        execution_mode: ExecutionMode,
    ) -> Self {
        // Load imported DA objects into backend state
        let service_id = environment.service_id;

        let mut backend = Self {
            code_storage: RefCell::new(BTreeMap::new()),
            code_hashes: RefCell::new(BTreeMap::new()),
            negative_code_cache: RefCell::new(BTreeMap::new()),
            storage_shards: RefCell::new(BTreeMap::new()),
            storage_present: RefCell::new(BTreeSet::new()),
            storage_absent: RefCell::new(BTreeSet::new()),
            balances: RefCell::new(BTreeMap::new()),
            nonces: RefCell::new(BTreeMap::new()),
            environment,
            service_id,
            imported_objects: RefCell::new(BTreeMap::new()),

            // Initialize meta-shard tracking with global_depth from JAM State
            // This ensures builder and guarantor use the same MetaSSR routing structure
            meta_ssr: RefCell::new(crate::meta_sharding::MetaSSR::from_parts(
                global_depth,
                Vec::new(),
            )),
            meta_shards: RefCell::new(BTreeMap::new()),

            execution_mode,
            current_tx_index: RefCell::new(None),
        };

        // Initialize system contract (0x01) with empty SSR for balance/nonce tracking
        // NOTE: System contract uses its own global_depth=0, NOT the meta-shard global_depth!
        let system_contract = h160_from_low_u64_be(0x01);

        // Post-SSR: No need for initial SSR metadata, ContractStorage has single empty shard
        backend
            .storage_shards
            .borrow_mut()
            .insert(system_contract, ContractStorage::new(system_contract));

        for (object_id, (object_ref, payload)) in imported_objects.into_iter() {
            // Store in imported_objects cache for reuse
            backend
                .imported_objects
                .borrow_mut()
                .insert(object_id, (object_ref.clone(), payload.clone()));

            // Determine object type from ObjectRef.object_kind() instead of object_id[31]
            // since object IDs no longer reliably encode kind in the last byte
            let mut address_bytes = [0u8; 20];
            address_bytes.copy_from_slice(&object_id[0..20]);
            let address = H160::from(address_bytes);

            log_info(&format!(
                "Processing imported object: address={:?}, object_id={}, object_kind={}",
                address,
                crate::contractsharding::format_object_id(&object_id),
                object_ref.object_kind
            ));
            match ObjectKind::from_u8(object_ref.object_kind as u8) {
                Some(ObjectKind::Code) => {
                    // Code object - load bytecode directly from witness
                    backend.load_code(address, payload);
                }
                Some(ObjectKind::StorageShard) => {
                    // Storage shard object - load directly from witness
                    // Layout: [20B address][0xFF][ld][prefix56 (7B)][padding (2B)][kind]

                    let mut prefix56 = [0u8; 7];
                    prefix56.copy_from_slice(&object_id[22..29]);

                    if let Some(shard_data) = ContractShard::deserialize(&payload) {
                        let mut storage_shards = backend.storage_shards.borrow_mut();
                        let contract_storage = storage_shards
                            .entry(address)
                            .or_insert_with(|| ContractStorage::new(address));
                        // Post-SSR: Single shard per contract, directly assign
                        contract_storage.shard = shard_data;
                    }
                }
                Some(ObjectKind::MetaShard) => {
                    // CRITICAL: Import meta-shards from witnesses to populate cache
                    // Without this, process_object_writes() creates empty shards and overwrites
                    // existing DA data, causing catastrophic data loss
                    use crate::meta_sharding::{MetaShard, MetaShardDeserializeResult};

                    // Deserialize with validated header
                    match MetaShard::deserialize_with_id_validated(
                        &payload,
                        &object_id,
                        backend.service_id,
                    ) {
                        MetaShardDeserializeResult::ValidatedHeader(shard_id, meta_shard) => {
                            log_info(&format!(
                                "  ✅ Imported MetaShard: shard_id={:?}, {} entries",
                                shard_id,
                                meta_shard.entries.len()
                            ));

                            // Insert into cache, even if empty (empty shards must be tracked!)
                            backend
                                .meta_shards
                                .borrow_mut()
                                .insert(shard_id, meta_shard);
                        }
                        MetaShardDeserializeResult::ValidationFailed => {
                            // Header present but validation failed - REJECT
                            log_error(&format!(
                                "  ❌ REJECTED MetaShard with invalid header: object_id={}",
                                crate::contractsharding::format_object_id(&object_id)
                            ));
                        }
                    }
                }
                Some(ObjectKind::Receipt) | Some(ObjectKind::Block) => {
                    // These object kinds are not imported during constructor - skip
                }
                Some(ObjectKind::Balance) | Some(ObjectKind::Nonce) => {
                    // Balance/Nonce updates are applied to Verkle tree post-execution, not during constructor
                    // Skip during witness import
                }
                None => {
                    // Unknown object kind - skip
                }
            }
        }

        // CRITICAL: Rebuild meta_ssr routing table from imported meta-shards
        // Without this, resolve_shard_id() routes all objects to root shard (ld=0)
        // because meta_ssr starts empty. This corrupts the split tree immediately.
        {
            let meta_shards = backend.meta_shards.borrow();
            let shard_ids: Vec<crate::da::ShardId> = meta_shards.keys().cloned().collect();

            if !shard_ids.is_empty() {
                let rebuilt_ssr = crate::meta_sharding::rebuild_meta_ssr_from_shards(&shard_ids);
                *backend.meta_ssr.borrow_mut() = rebuilt_ssr;
            }
        }

        // Seed storage presence map from imported storage shards
        {
            let storage_shards = backend.storage_shards.borrow();
            let mut present = backend.storage_present.borrow_mut();
            for (address, contract_storage) in storage_shards.iter() {
                for entry in &contract_storage.shard.entries {
                    present.insert((*address, entry.key_h));
                }
            }
        }

        backend
    }

    /// Create a MajikBackend from a Verkle witness (Guarantor mode)
    ///
    /// This verifies the Verkle proof and populates the backend caches
    /// with the pre-state values from the witness.
    ///
    /// Returns None if the witness is invalid or verification fails.
    pub fn from_verkle_witness(
        environment: EnvironmentData,
        witness_data: &[u8],
        global_depth: u8,
    ) -> Option<Self> {
        // Native Rust Verkle proof verification (production ready)
        // Cryptographic verification via verify_verkle_witness:
        // - verify_prestate_commitment: validates pre-values exist in pre_state_root (IPA proof)
        // - verify_post_state_commitment: validates post-values exist in post_state_root (IPA proof)
        // - verify_ipa_multiproof: full line-by-line port of go-ipa verifier
        //
        // Transition verification happens during cache-only execution:
        // - Pre-witness values loaded into read-only cache below
        // - Any read not in cache = GUARANTOR ... MISS panic (execution invalid)
        // - Deterministic re-execution + valid witnesses = valid state transition
        //
        // See services/evm/docs/VERKLE.md for JAM execution model details

        log_info("⚙️  Guarantor: Running Verkle proof verification");
        // Currently uses Go FFI for witness parsing, but core crypto is native Rust
        if !crate::verkle_proof::verify_verkle_witness(witness_data) {
            log_error("❌ Guarantor: Verkle proof verification FAILED");
            return None;
        }
        log_info("✅ Guarantor: Verkle proof verification PASSED");

        // Deserialize the witness
        let witness = crate::verkle::VerkleWitness::deserialize(witness_data)?;

        // Parse witness to extract caches
        let (balances, nonces, code, code_hashes, storage) = witness.parse_to_caches();
        log_info(&format!(
            "from_verkle_witness: caches loaded balances={} nonces={} code={} storage={}",
            balances.len(),
            nonces.len(),
            code.len(),
            storage.len()
        ));

        // Extract service_id before moving environment
        let service_id = environment.service_id;

        // Convert flat storage map to ContractStorage with shards
        let mut storage_shards_map = BTreeMap::new();
        for ((address, key), value) in storage {
            let contract_storage =
                storage_shards_map
                    .entry(address)
                    .or_insert_with(|| ContractStorage {
                        shard: ContractShard {
                            entries: Vec::new(),
                        },
                    });

            // Insert in sorted order
            let shard = &mut contract_storage.shard;
            match shard
                .entries
                .binary_search_by_key(&key, |entry| entry.key_h)
            {
                Ok(idx) => {
                    // Update existing entry
                    shard.entries[idx].value = value;
                }
                Err(idx) => {
                    // Insert new entry
                    shard
                        .entries
                        .insert(idx, crate::contractsharding::EvmEntry { key_h: key, value });
                }
            }
        }

        // Create backend in Guarantor mode with populated caches
        let backend = Self {
            code_storage: RefCell::new(code),
            code_hashes: RefCell::new(code_hashes),
            negative_code_cache: RefCell::new(BTreeMap::new()),
            storage_shards: RefCell::new(storage_shards_map),
            storage_present: RefCell::new(BTreeSet::new()),
            storage_absent: RefCell::new(BTreeSet::new()),
            balances: RefCell::new(balances),
            nonces: RefCell::new(nonces),
            environment,
            service_id,
            imported_objects: RefCell::new(BTreeMap::new()),
            meta_ssr: RefCell::new(crate::meta_sharding::MetaSSR::from_parts(
                global_depth,
                Vec::new(),
            )),
            meta_shards: RefCell::new(BTreeMap::new()),
            execution_mode: ExecutionMode::Guarantor,
            current_tx_index: RefCell::new(None),
        };

        {
            let storage_shards = backend.storage_shards.borrow();
            let mut present = backend.storage_present.borrow_mut();
            for (address, contract_storage) in storage_shards.iter() {
                for entry in &contract_storage.shard.entries {
                    present.insert((*address, entry.key_h));
                }
            }
        }

        Some(backend)
    }

    /// Load code into backend (helper for constructor)
    pub fn load_code(&mut self, address: H160, code: Vec<u8>) {
        let code_hash = keccak256(&code);
        self.code_storage.borrow_mut().insert(address, code.clone());
        self.code_hashes.borrow_mut().insert(address, code_hash);
        // Version will be set when DA object is imported with metadata
    }

    /// Import storage shard from DA using fetch_object host function
    /// Phase 5: Guarantor checks imported_objects cache first (witnesses pre-loaded)
    /// Resolve storage dependency
    #[allow(dead_code)]
    pub fn object_dependency_for_storage(&self, address: H160, _key: H256) -> ObjectId {
        contract_shard_object_id(address)
    }

    /// Resolve balance dependency (stored in 0x01 precompile shards)
    #[allow(dead_code)]
    pub fn object_dependency_for_balance(&self, address: H160) -> ObjectId {
        let system_contract = h160_from_low_u64_be(0x01);
        let balance_key = balance_storage_key(address);
        self.object_dependency_for_storage(system_contract, balance_key)
    }

    /// Resolve nonce dependency (stored in 0x01 precompile shards)
    #[allow(dead_code)]
    pub fn object_dependency_for_nonce(&self, address: H160) -> ObjectId {
        let system_contract = h160_from_low_u64_be(0x01);
        let nonce_key = nonce_storage_key(address);
        self.object_dependency_for_storage(system_contract, nonce_key)
    }

    /// Resolve code ObjectId for bytecode reads
    #[allow(dead_code)]
    pub fn object_dependency_for_code(&self, address: H160) -> ObjectId {
        code_object_id(address)
    }

    /// Update presence tracking for a storage slot (present vs absent)
    pub fn update_storage_presence(&self, address: H160, key: H256, present: bool) {
        let mut present_set = self.storage_present.borrow_mut();
        let mut absent_set = self.storage_absent.borrow_mut();
        mark_storage_presence(&mut present_set, &mut absent_set, address, key, present);
    }

    /// Return known presence information for a storage slot, if any.
    /// Some(true) → observed in witness/cache, Some(false) → confirmed absent, None → unknown.
    pub fn storage_presence(&self, address: H160, key: H256) -> Option<bool> {
        {
            let present_set = self.storage_present.borrow();
            if present_set.contains(&(address, key)) {
                return Some(true);
            }
        }
        {
            let absent_set = self.storage_absent.borrow();
            if absent_set.contains(&(address, key)) {
                return Some(false);
            }
        }
        None
    }

    // ===== Meta-shard access methods =====

    /// Get mutable reference to meta-SSR for processing object writes
    pub fn meta_ssr_mut(&self) -> core::cell::RefMut<'_, crate::meta_sharding::MetaSSR> {
        self.meta_ssr.borrow_mut()
    }

    /// Get mutable reference to meta-shards cache
    pub fn meta_shards_mut(
        &self,
    ) -> core::cell::RefMut<'_, BTreeMap<crate::da::ShardId, crate::meta_sharding::MetaShard>> {
        self.meta_shards.borrow_mut()
    }
}

// ===== MajikOverlay struct and coordination =====

/// MajikOverlay wraps vendor OverlayedBackend with dependency tracking
///
/// Primary runtime entry point that coordinates:
/// - Vendor OverlayedBackend for change buffering
/// - Transaction isolation and frame management
pub struct MajikOverlay<'config> {
    pub(crate) overlay: OverlayedBackend<'config, MajikBackend>,
    /// Index of the transaction currently executing.
    /// Initialized to `None` in `new`, set in `begin_transaction`, and cleared in `commit_transaction`/`revert_transaction`.
    current_tx_index: Option<usize>,
    /// Scratch buffer for logs emitted by the active transaction.
    /// Created empty in `new`, reset on `begin_transaction`, filled by `log`, and drained on commit/revert.
    current_tx_logs: Vec<Log>,
    /// Per-transaction log storage retained after commits so refine can fetch logs later.
    /// Resized in `begin_transaction`, populated in `commit_transaction`, and drained via `take_transaction_logs`.
    tx_logs: Vec<Vec<Log>>,
    /// Track which transaction modified which state keys (for BAL tx_index attribution)
    /// Maps state key → tx_index that last modified it
    write_tx_index: BTreeMap<WriteKey, u32>,
    /// Verkle witness event tracker (Phase A: tracking only, no gas charging yet)
    witness_events: RefCell<VerkleEventTracker>,
}

/// Identifies a unique state write location
#[derive(Debug, Clone, PartialEq, Eq, PartialOrd, Ord)]
pub enum WriteKey {
    Balance(H160),
    Nonce(H160),
    Code(H160),
    Storage(H160, U256),
}

impl<'config> MajikOverlay<'config> {
    /// Create new overlay with backend and runtime config
    pub fn new(backend: MajikBackend, config: &'config RuntimeConfig) -> Self {
        Self {
            overlay: OverlayedBackend::new(backend, config),
            current_tx_index: None,
            current_tx_logs: Vec::new(),
            tx_logs: Vec::new(),
            write_tx_index: BTreeMap::new(),
            witness_events: RefCell::new(VerkleEventTracker::new()),
        }
    }

    /// Begin new transaction with isolated state
    pub fn begin_transaction(&mut self, tx_index: usize) {
        self.overlay.push_substate();
        self.current_tx_index = Some(tx_index);
        // Also update backend's current_tx_index for verkle fetches
        *self.overlay.backend().current_tx_index.borrow_mut() = Some(tx_index);
        // Reset witness tracking so each transaction accounts for its own events
        self.witness_events.borrow_mut().clear();
        // Reset witness event tracking for this transaction to avoid
        // carrying over access/write events (and gas) from previous transactions.
        //*self.witness_events.borrow_mut() = VerkleEventTracker::new();
        self.current_tx_logs.clear();
        if self.tx_logs.len() <= tx_index {
            self.tx_logs.resize(tx_index + 1, Vec::new());
        }
        self.tx_logs[tx_index].clear();
    }

    /// Commit transaction
    pub fn commit_transaction(&mut self) -> Result<(), ExitError> {
        self.overlay.pop_substate(MergeStrategy::Commit)?;
        if let Some(tx_index) = self.current_tx_index.take() {
            if self.tx_logs.len() <= tx_index {
                self.tx_logs.resize(tx_index + 1, Vec::new());
            }
            self.tx_logs[tx_index] = core::mem::take(&mut self.current_tx_logs);
        }
        Ok(())
    }

    /// Revert transaction and discard changes
    pub fn revert_transaction(&mut self) -> Result<(), ExitError> {
        self.overlay.pop_substate(MergeStrategy::Revert)?;
        self.current_tx_index = None;
        self.current_tx_logs.clear();
        Ok(())
    }

    /// Drain logs collected for a transaction after it has been committed.
    pub fn take_transaction_logs(&mut self, tx_index: usize) -> Vec<Log> {
        if self.tx_logs.len() <= tx_index {
            Vec::new()
        } else {
            core::mem::take(&mut self.tx_logs[tx_index])
        }
    }

    /// Record transaction origin witness events (Phase A: EIP-4762)
    ///
    /// Per EIP-4762: tx.origin always generates access/write events
    /// that do NOT incur edit/fill charges (tx-level events are special)
    pub fn record_tx_origin_witness(&mut self, caller: H160) {
        // Skip precompile/system contract headers from witness tracking
        if is_precompile(caller) || is_system_contract(caller) {
            return;
        }
        let mut witness = self.witness_events.borrow_mut();
        witness.record_tx_origin_access(caller);
        witness.record_tx_origin_write(caller);
    }

    /// Record transaction target access (Phase A: EIP-4762)
    ///
    /// Per EIP-4762: tx.target access happens regardless of value
    /// FIX Issue 5: Separated from write (which only happens if value > 0)
    pub fn record_tx_target_access(&mut self, target: H160) {
        // Skip precompile/system contract header accesses (writes still tracked separately)
        if is_precompile(target) || is_system_contract(target) {
            return;
        }
        let mut witness = self.witness_events.borrow_mut();
        witness.record_tx_target_access(target);
    }

    /// Record transaction target write (Phase A: EIP-4762)
    ///
    /// Per EIP-4762: tx.target write only happens if value > 0
    /// FIX Issue 5: Separated from access (which always happens)
    pub fn record_tx_target_write(&mut self, target: H160) {
        let mut witness = self.witness_events.borrow_mut();
        witness.record_tx_target_write(target);
    }

    /// Get witness gas cost for transaction (Phase B Option C)
    ///
    /// Returns the total witness gas cost calculated from all tracked events.
    /// This should be added to the EVM gas_used after transaction execution completes.
    ///
    /// NOTE: Currently unused - use `take_witness_gas()` instead for automatic reset
    #[allow(dead_code)]
    pub fn get_witness_gas(&self) -> u64 {
        self.witness_events.borrow().calculate_total_witness_gas()
    }

    /// Take witness gas and reset tracker for next transaction (Phase B Option C)
    ///
    /// Returns the total witness gas cost and clears all tracked events.
    /// This ensures each transaction has its own isolated witness gas accounting.
    pub fn take_witness_gas(&mut self) -> u64 {
        let mut witness = self.witness_events.borrow_mut();
        let gas = witness.calculate_total_witness_gas();
        witness.clear();
        gas
    }

    /// Log current witness tracking statistics for debugging/visibility
    pub fn log_witness_stats(&self) {
        let (branches, leaves, edited_subtrees, edited_leaves, fills) =
            self.witness_events.borrow().stats();
        log_info(&format!(
            "Verkle witness stats: branches={} leaves={} edited_subtrees={} edited_leaves={} fills={}",
            branches, leaves, edited_subtrees, edited_leaves, fills
        ));
    }

    /// Deconstructs the overlay and converts changes to ExecutionEffects
    ///
    /// Returns ExecutionEffects for the accumulate phase by calling to_execution_effects
    /// Also returns the backend so caller can access meta-shard state
    pub fn deconstruct(
        self,
        work_package_hash: [u8; 32],
        receipts: &[TransactionReceiptRecord],
        verkle_root: [u8; 32],
        timeslot: u32,
        total_gas_used: u64,
        _service_id: u32,
    ) -> (utils::effects::ExecutionEffects, MajikBackend) {
        let (backend, change_set) = self.overlay.deconstruct();

        // Convert to ExecutionEffects and assemble block (block logged but not returned)
        // Pass write_tx_index map for correct tx attribution
        let effects = backend.to_execution_effects(
            &change_set,
            &self.write_tx_index,
            work_package_hash,
            receipts,
            verkle_root,
            timeslot,
            total_gas_used,
        );

        (effects, backend)
    }
}

// ===== MajikOverlay trait implementations =====

impl TransactionalBackend for MajikOverlay<'_> {
    fn push_substate(&mut self) {
        self.overlay.push_substate();
    }

    fn pop_substate(&mut self, strategy: MergeStrategy) -> Result<(), ExitError> {
        self.overlay.pop_substate(strategy)?;
        Ok(())
    }
}

impl RuntimeBaseBackend for MajikOverlay<'_> {
    /// Balance read - delegates to overlay which supports Verkle
    fn balance(&self, address: H160) -> U256 {
        // Track account header access for witness (Phase A: no gas charging yet)
        self.witness_events
            .borrow_mut()
            .record_account_header_access(address);
        self.overlay.balance(address)
    }

    /// Code read - delegates to overlay which supports Verkle
    fn code(&self, address: H160) -> Vec<u8> {
        let code = self.overlay.code(address);

        // LIMITATION (Issue 3): This tracks ALL code chunks on any code() call
        // EIP-4762 requires per-PC execution tracking (chunk accessed per instruction)
        // Alternative chosen: Keep bulk tracking for Phase A, fix in Phase B with VM hooks
        // Rationale: Proper fix requires EVM step hooks; bulk logging is safe (over-tracks)
        // Impact: Witness will include all code chunks, not just executed ones
        // TODO Phase B: Implement per-PC chunk tracking via EVM step hook
        let num_chunks = (code.len() + 30) / 31;
        let mut witness = self.witness_events.borrow_mut();
        for chunk_index in 0..num_chunks {
            witness.record_code_chunk_access(address, chunk_index);
        }
        code
    }

    /// Code hash - delegates to overlay which supports Verkle
    fn code_hash(&self, address: H160) -> H256 {
        // Track code hash access for witness (Phase A: no gas charging yet)
        self.witness_events
            .borrow_mut()
            .record_codehash_access(address);
        self.overlay.code_hash(address)
    }

    /// Storage read - delegates to overlay
    fn storage(&self, address: H160, index: H256) -> H256 {
        // Track storage slot access for witness (Phase A: no gas charging yet)
        let slot = U256::from_big_endian(index.as_bytes());
        self.witness_events
            .borrow_mut()
            .record_storage_access(address, slot);
        self.overlay.storage(address, index)
    }

    /// Transient storage (EIP-1153) - ephemeral, not persisted
    fn transient_storage(&self, address: H160, index: H256) -> H256 {
        self.overlay.transient_storage(address, index)
    }

    /// EIP-161 exists check
    fn exists(&self, address: H160) -> bool {
        self.overlay.exists(address)
    }

    /// Nonce read - delegates to overlay which supports Verkle
    fn nonce(&self, address: H160) -> U256 {
        self.overlay.nonce(address)
    }
}

impl EvmRuntimeBackend for MajikOverlay<'_> {
    /// Original storage value for EIP-2200/EIP-1283 gas refund calculations
    /// JAM DA: Overlay caches first read per storage slot as "original" value
    /// Gas calculation: Uses (original → current → new) triple to determine cost and refunds:
    ///   - If original == current and original == 0: SSTORE_SET (20000 gas, storing to empty slot)
    ///   - If original == current and original != 0: SSTORE_RESET (5000 gas, updating existing slot)
    ///   - If original != current: SLOAD (warm 100 gas, modifying already-modified slot)
    ///   - Refunds: Clearing storage (new == 0, original != 0) refunds gas
    /// Implementation: Overlay calls backend.storage() on first access, caches as "original"
    /// Note: OverlayedBackend handles caching automatically, MajikBackend just provides storage reads
    fn original_storage(&self, address: H160, index: H256) -> H256 {
        self.overlay.original_storage(address, index)
    }

    /// Track deleted accounts for DA object removal at accumulate
    /// JAM DA: Overlay maintains BTreeSet of deleted addresses in change_set.deletes
    /// Export: to_execution_effects() exports code tombstones (empty payload) and storage resets for deleted accounts
    /// Implementation: Overlay.deleted() checks substate.deletes BTreeSet (populated by mark_delete_reset)
    /// Gas refund: 24000 gas per deletion handled by EVM execution layer (not in DA export)
    fn deleted(&self, address: H160) -> bool {
        self.overlay.deleted(address)
    }

    /// Track created accounts for EIP-6780 SELFDESTRUCT rules
    /// JAM DA: At accumulate, export new code object, set nonce=1 via system-contract storage
    /// EIP-6780: Allows SELFDESTRUCT only for accounts created in same transaction
    fn created(&self, address: H160) -> bool {
        self.overlay.created(address)
    }

    /// Cold/warm tracking for EIP-2929 gas accounting
    /// JAM DA: In-memory only, no DA persistence, purely for gas metering
    /// Implementation: Overlay maintains BTreeSet of accessed (address, Option<slot>) pairs
    /// First access: +2600 gas surcharge (cold); subsequent: warm (cheaper, e.g., 100 gas for SLOAD)
    fn is_cold(&self, address: H160, index: Option<H256>) -> bool {
        self.overlay.is_cold(address, index)
    }

    /// Mark address/account as hot for EIP-2929 warmth tracking
    /// JAM DA: In-memory BTreeSet, cleared at transaction boundaries
    /// TouchKind::Access: Adds to accessed set (no state change)
    /// TouchKind::StateChange: Adds to touched set (for EIP-161 empty account cleanup)
    fn mark_hot(&mut self, address: H160, kind: TouchKind) {
        self.overlay.mark_hot(address, kind);
    }

    /// Mark storage slot as hot for EIP-2929 warmth tracking
    /// JAM DA: In-memory BTreeSet of (address, Some(slot)) tuples, no DA persistence
    /// Implementation: Overlay inserts into accessed set, subsequent SLOAD/SSTORE pay warm price
    /// Access list: Can prewarm slots before transaction execution for gas optimization
    fn mark_storage_hot(&mut self, address: H160, index: H256) {
        self.overlay.mark_storage_hot(address, index);
    }

    /// Storage write
    fn set_storage(&mut self, address: H160, key: H256, value: H256) -> Result<(), ExitError> {
        // Track storage write for witness (Phase A: no gas charging yet)
        let slot = U256::from_big_endian(key.as_bytes());
        let original_value = self.overlay.original_storage(address, key);

        // Determine presence vs absence using explicit tracking from backend fetches
        let presence = {
            let backend = self.overlay.backend();
            backend.storage_presence(address, key)
        };
        let original = match presence {
            Some(true) => Some(U256::from_big_endian(original_value.as_bytes())),
            Some(false) => None,
            None => {
                // Fallback: use prior heuristic when presence is unknown
                let is_first_access = self.overlay.is_cold(address, Some(key));
                if is_first_access && original_value == H256::zero() {
                    None
                } else {
                    Some(U256::from_big_endian(original_value.as_bytes()))
                }
            }
        };

        let new_value = U256::from_big_endian(value.as_bytes());
        self.witness_events
            .borrow_mut()
            .record_storage_write(address, slot, original, new_value);

        self.overlay.set_storage(address, key, value)?;
        // After a write, the slot is known-present
        self.overlay
            .backend_mut()
            .update_storage_presence(address, key, true);
        // Track which tx modified this storage slot
        if let Some(tx_index) = self.current_tx_index {
            self.write_tx_index
                .insert(WriteKey::Storage(address, slot), tx_index as u32);
        }
        Ok(())
    }

    /// Set transient storage (EIP-1153)
    fn set_transient_storage(
        &mut self,
        address: H160,
        key: H256,
        value: H256,
    ) -> Result<(), ExitError> {
        self.overlay.set_transient_storage(address, key, value)?;
        Ok(())
    }

    /// Accumulate logs
    fn log(&mut self, log: Log) -> Result<(), ExitError> {
        self.overlay.log(log.clone())?;
        self.current_tx_logs.push(log);
        Ok(())
    }

    /// Account deletion with EIP-6780 restrictions
    fn mark_delete_reset(&mut self, address: H160) {
        // FIX Issue 6: Track account write for SELFDESTRUCT
        self.witness_events
            .borrow_mut()
            .record_account_write(address);

        self.overlay.mark_delete_reset(address);
    }

    /// Mark account as created for EIP-6780 checks
    fn mark_create(&mut self, address: H160) {
        // FIX Issue 6: Track account write for account creation
        self.witness_events
            .borrow_mut()
            .record_account_write(address);

        self.overlay.mark_create(address);
    }

    /// Clear storage before CREATE deployment
    fn reset_storage(&mut self, address: H160) {
        self.overlay.reset_storage(address);
    }

    /// Export bytecode
    fn set_code(
        &mut self,
        address: H160,
        code: Vec<u8>,
        origin: SetCodeOrigin,
    ) -> Result<(), ExitError> {
        // Track code write for witness (Phase A: no gas charging yet)
        self.witness_events
            .borrow_mut()
            .record_code_write(address, code.len());

        self.overlay.set_code(address, code, origin)?;
        // Track which tx modified this code
        if let Some(tx_index) = self.current_tx_index {
            self.write_tx_index
                .insert(WriteKey::Code(address), tx_index as u32);
        }
        Ok(())
    }

    /// Balance deposits for native ETH transfers
    /// Delegates to overlay which updates both cache and changeset
    fn deposit(&mut self, target: H160, value: U256) {
        // Track account write for witness (Phase A: no gas charging yet)
        self.witness_events
            .borrow_mut()
            .record_account_write(target);

        self.overlay.deposit(target, value);
        // Track which tx modified this balance
        if let Some(tx_index) = self.current_tx_index {
            self.write_tx_index
                .insert(WriteKey::Balance(target), tx_index as u32);
        }
    }

    /// Balance withdrawals for native ETH transfers
    /// Delegates to overlay which updates both cache and changeset
    fn withdrawal(&mut self, source: H160, value: U256) -> Result<(), ExitError> {
        // Track account write for witness (Phase A: no gas charging yet)
        self.witness_events
            .borrow_mut()
            .record_account_write(source);

        self.overlay.withdrawal(source, value)?;
        // Track which tx modified this balance
        if let Some(tx_index) = self.current_tx_index {
            self.write_tx_index
                .insert(WriteKey::Balance(source), tx_index as u32);
        }
        Ok(())
    }

    /// Atomic transfers
    fn transfer(&mut self, transfer: Transfer) -> Result<(), ExitError> {
        // Track account writes for witness (Phase A: no gas charging yet)
        self.witness_events
            .borrow_mut()
            .record_account_write(transfer.source);
        self.witness_events
            .borrow_mut()
            .record_account_write(transfer.target);

        // Track which tx modified these balances (before transfer consumes the value)
        if let Some(tx_index) = self.current_tx_index {
            self.write_tx_index
                .insert(WriteKey::Balance(transfer.source), tx_index as u32);
            self.write_tx_index
                .insert(WriteKey::Balance(transfer.target), tx_index as u32);
        }
        self.overlay.transfer(transfer)?;
        Ok(())
    }

    /// Nonce increments
    fn inc_nonce(&mut self, address: H160) -> Result<(), ExitError> {
        // FIX Issue 6: Track account write for nonce change
        self.witness_events
            .borrow_mut()
            .record_account_write(address);

        self.overlay.inc_nonce(address)?;
        // Track which tx modified this nonce
        if let Some(tx_index) = self.current_tx_index {
            self.write_tx_index
                .insert(WriteKey::Nonce(address), tx_index as u32);
        }
        Ok(())
    }
}

// ===== MajikBackend trait implementations =====

impl RuntimeBaseBackend for MajikBackend {
    /// Balance with DA import on cache miss
    /// Builder mode: uses Verkle host function (Go maintains verkleReadLog)
    /// Guarantor mode: uses cache-only (panics on miss)
    fn balance(&self, address: H160) -> U256 {
        log_info(&format!(
            "balance() called addr={:?} mode={:?}",
            address, self.execution_mode
        ));
        // Check cache first
        {
            let balances = self.balances.borrow();
            if let Some(balance) = balances.get(&address) {
                log_info(&format!("balance HIT {:?}={}", address, balance));
                return *balance;
            }
        }
        log_info(&format!("balance MISS {:?}", address));

        match self.execution_mode {
            ExecutionMode::Builder => {
                // Builder: fetch from Verkle tree via host function
                let tx_index = self.current_tx_index.borrow().unwrap_or(0) as u32;
                let balance = crate::verkle::fetch_balance_verkle(address, tx_index);
                self.balances.borrow_mut().insert(address, balance);
                log_info(&format!(
                    "balance fetched via verkle ({:?}) value={} tx_index={}",
                    address, balance, tx_index
                ));
                balance
            }
            ExecutionMode::Guarantor => {
                // Guarantor: cache-only, panic on miss
                log_error(&format!("GUARANTOR BALANCE MISS {:?}", address));
                panic!("GUARANTOR BALANCE MISS");
            }
        }
    }

    /// Code from cache with DA import on miss
    /// Builder mode: uses Verkle host function (Go maintains verkleReadLog)
    /// Guarantor mode: uses cache-only (panics on miss)
    fn code(&self, address: H160) -> Vec<u8> {
        log_info(&format!(
            "code() called addr={:?} mode={:?}",
            address, self.execution_mode
        ));
        // Check positive cache first
        {
            let code_storage = self.code_storage.borrow();
            if let Some(code) = code_storage.get(&address) {
                log_info(&format!("code HIT {:?} len={}", address, code.len()));
                return code.clone();
            }
        }

        // Check negative cache - if we already know this address has no code, return empty
        {
            let negative_cache = self.negative_code_cache.borrow();
            if negative_cache.contains_key(&address) {
                log_info(&format!("code HIT (negative) {:?}", address));
                return Vec::new();
            }
        }
        log_info(&format!("code MISS {:?}", address));

        match self.execution_mode {
            ExecutionMode::Builder => {
                // Builder: fetch from Verkle tree via host function
                let tx_index = self.current_tx_index.borrow().unwrap_or(0) as u32;
                let bytecode = crate::verkle::fetch_code_verkle(address, tx_index);

                if bytecode.is_empty() {
                    // Negative caching: remember EOAs with no bytecode
                    self.negative_code_cache.borrow_mut().insert(address, ());
                } else {
                    // Cache the imported code and its hash
                    let code_hash = keccak256(&bytecode);
                    self.code_storage
                        .borrow_mut()
                        .insert(address, bytecode.clone());
                    self.code_hashes.borrow_mut().insert(address, code_hash);
                }

                bytecode
            }
            ExecutionMode::Guarantor => {
                // Guarantor: Check code_hashes cache to determine if account is EOA
                // If code_hash is empty code hash (or not in cache), account is EOA
                let code_hashes = self.code_hashes.borrow();
                if let Some(code_hash) = code_hashes.get(&address) {
                    // Code hash is in witness
                    let empty_code_hash = keccak256(&[]);
                    if code_hash.as_bytes() == empty_code_hash.as_bytes() {
                        // Empty code hash = EOA (no code)
                        log_info(&format!("code() EOA (empty hash) {:?}", address));
                        self.negative_code_cache.borrow_mut().insert(address, ());
                        return Vec::new();
                    } else {
                        // Non-empty code hash but code not in cache = missing witness data
                        log_error(&format!(
                            "GUARANTOR: CodeHash present but code missing for {:?}",
                            address
                        ));
                        panic!("GUARANTOR: CodeHash present but code missing");
                    }
                } else {
                    // No code hash in witness = assume EOA (conservative default)
                    log_info(&format!("code() EOA (no hash in witness) {:?}", address));
                    self.negative_code_cache.borrow_mut().insert(address, ());
                    return Vec::new();
                }
            }
        }
    }

    /// Code hash with caching optimization
    /// Builder mode: uses Verkle host function (Go maintains verkleReadLog)
    /// Guarantor mode: uses cache-only (panics on miss)
    fn code_hash(&self, address: H160) -> H256 {
        // Check cache first
        {
            let code_hashes = self.code_hashes.borrow();
            if let Some(hash) = code_hashes.get(&address) {
                return *hash;
            }
        }

        match self.execution_mode {
            ExecutionMode::Builder => {
                // Builder: fetch from Verkle tree via host function
                let tx_index = self.current_tx_index.borrow().unwrap_or(0) as u32;
                let hash = crate::verkle::fetch_code_hash_verkle(address, tx_index);
                self.code_hashes.borrow_mut().insert(address, hash);
                hash
            }
            ExecutionMode::Guarantor => {
                // Guarantor: cache-only, panic on miss
                panic!("GUARANTOR: Code hash cache miss for address={:?}", address);
            }
        }
    }

    /// Storage via Verkle tree
    /// Builder mode: uses Verkle host function (Go maintains verkleReadLog)
    /// Guarantor mode: uses cache-only (panics on miss)
    fn storage(&self, address: H160, key: H256) -> H256 {
        // Check cache first (for within-transaction reads)
        {
            let storage_shards = self.storage_shards.borrow();
            if let Some(contract_storage) = storage_shards.get(&address) {
                // Post-SSR: Single shard per contract, access directly
                let shard_data = &contract_storage.shard;

                match shard_data
                    .entries
                    .binary_search_by_key(&key, |entry| entry.key_h)
                {
                    Ok(idx) => {
                        let value = shard_data.entries[idx].value;
                        self.update_storage_presence(address, key, true);
                        return value;
                    }
                    Err(_) => {
                        // Key not in cache - continue to fetch
                    }
                }
            }
        }

        match self.execution_mode {
            ExecutionMode::Builder => {
                // Builder: fetch from Verkle tree via host function
                let tx_index = self.current_tx_index.borrow().unwrap_or(0) as u32;
                let (value, found) =
                    crate::verkle::fetch_storage_verkle_with_presence(address, key, tx_index);

                // Cache the value for within-transaction reads
                let mut storage_shards = self.storage_shards.borrow_mut();
                let contract_storage =
                    storage_shards
                        .entry(address)
                        .or_insert_with(|| ContractStorage {
                            shard: ContractShard {
                                entries: Vec::new(),
                            },
                        });

                // Insert in sorted order
                let shard = &mut contract_storage.shard;
                match shard
                    .entries
                    .binary_search_by_key(&key, |entry| entry.key_h)
                {
                    Ok(_) => {
                        // Already exists (shouldn't happen, but handle gracefully)
                    }
                    Err(idx) => {
                        shard
                            .entries
                            .insert(idx, crate::contractsharding::EvmEntry { key_h: key, value });
                    }
                }

                self.update_storage_presence(address, key, found);

                value
            }
            ExecutionMode::Guarantor => {
                // Guarantor: cache-only, panic on miss
                panic!(
                    "GUARANTOR: Storage cache miss for address={:?}, key={:?}",
                    address, key
                );
            }
        }
    }

    /// Transient storage backend (EIP-1153) - unused, overlay handles it
    /// DA-STORAGE.md: Not persisted, cleared at transaction boundaries
    /// Implementation: OverlayedBackend never calls this - uses its own in-memory map
    /// No DA import/export needed - purely ephemeral
    fn transient_storage(&self, _address: H160, _key: H256) -> H256 {
        H256::zero() // Unused - overlay handles transient storage
    }

    /// EIP-161 exists check from in-memory cache
    /// DA-STORAGE.md: Returns true if (balance > 0) OR (nonce > 0) OR (code_size > 0)
    /// Implementation: Checks in-memory maps for any account data
    /// Note: Overlay wrapper applies EIP-161 logic (checks if all three are zero)
    /// Note: DA import handled by balance()/nonce()/code() on cache miss
    fn exists(&self, address: H160) -> bool {
        if self.code_storage.borrow().contains_key(&address)
            || self.balances.borrow().contains_key(&address)
            || self.nonces.borrow().contains_key(&address)
        {
            return true;
        }

        self.storage_shards.borrow().contains_key(&address)
    }

    /// Nonce with DA import on cache miss
    /// Builder mode: uses Verkle host function (Go maintains verkleReadLog)
    /// Guarantor mode: uses cache-only (panics on miss)
    fn nonce(&self, address: H160) -> U256 {
        log_info(&format!(
            "nonce() called addr={:?} mode={:?}",
            address, self.execution_mode
        ));
        // Check cache first
        {
            let nonces = self.nonces.borrow();
            if let Some(nonce) = nonces.get(&address) {
                log_info(&format!("nonce HIT {:?}={}", address, nonce));
                return *nonce;
            }
        }
        log_info(&format!("nonce MISS {:?}", address));

        match self.execution_mode {
            ExecutionMode::Builder => {
                // Builder: fetch from Verkle tree via host function
                let tx_index = self.current_tx_index.borrow().unwrap_or(0) as u32;
                let nonce = crate::verkle::fetch_nonce_verkle(address, tx_index);
                self.nonces.borrow_mut().insert(address, nonce);
                log_info(&format!(
                    "nonce fetched via verkle ({:?}) value={} tx_index={}",
                    address, nonce, tx_index
                ));
                nonce
            }
            ExecutionMode::Guarantor => {
                // Guarantor: cache-only, panic on miss
                log_error(&format!(
                    "GUARANTOR: Nonce cache miss for address={:?}",
                    address
                ));
                panic!("GUARANTOR: Nonce cache miss for address={:?}", address);
            }
        }
    }
}

impl EvmRuntimeBackend for MajikBackend {
    // ===== Write operations (never called - OverlayedBackend intercepts) =====

    fn set_storage(
        &mut self,
        _address: H160,
        _key: H256,
        _value: H256,
    ) -> Result<(), evm::interpreter::ExitError> {
        Ok(()) // Unused: OverlayedBackend buffers writes
    }

    fn inc_nonce(&mut self, _address: H160) -> Result<(), evm::interpreter::ExitError> {
        Ok(()) // Unused: OverlayedBackend handles nonces
    }

    fn log(
        &mut self,
        _log: evm::interpreter::runtime::Log,
    ) -> Result<(), evm::interpreter::ExitError> {
        Ok(()) // Unused: OverlayedBackend buffers logs
    }

    /// Set transient storage backend - unused, overlay handles it
    /// Implementation: OverlayedBackend never calls this - writes to its own in-memory map
    fn set_transient_storage(
        &mut self,
        _address: H160,
        _key: H256,
        _value: H256,
    ) -> Result<(), evm::interpreter::ExitError> {
        Ok(()) // Unused: OverlayedBackend handles transient storage
    }

    fn deposit(&mut self, _target: H160, _value: U256) {
        // Unused: OverlayedBackend buffers balance changes
    }

    fn withdrawal(
        &mut self,
        _source: H160,
        _value: U256,
    ) -> Result<(), evm::interpreter::ExitError> {
        Ok(()) // Unused: OverlayedBackend buffers balance changes
    }

    fn mark_delete_reset(&mut self, _address: H160) {
        // Unused: OverlayedBackend tracks deletions
    }

    fn mark_create(&mut self, _address: H160) {
        // Unused: OverlayedBackend tracks creations
    }

    fn reset_storage(&mut self, _address: H160) {
        // Unused: OverlayedBackend buffers storage resets
    }

    fn set_code(
        &mut self,
        _address: H160,
        _code: Vec<u8>,
        _origin: evm::interpreter::runtime::SetCodeOrigin,
    ) -> Result<(), evm::interpreter::ExitError> {
        Ok(()) // Unused: OverlayedBackend buffers code changes
    }

    // ===== Required by trait (cannot remove) =====

    fn original_storage(&self, address: H160, key: H256) -> H256 {
        self.storage(address, key)
    }

    fn is_cold(&self, _address: H160, _index: Option<H256>) -> bool {
        true
    }

    fn mark_hot(&mut self, _address: H160, _kind: evm::interpreter::runtime::TouchKind) {}

    fn mark_storage_hot(&mut self, _address: H160, _key: H256) {}

    fn deleted(&self, _address: H160) -> bool {
        false
    }

    fn created(&self, _address: H160) -> bool {
        false
    }

    fn transfer(&mut self, _transfer: Transfer) -> Result<(), ExitError> {
        Ok(()) // Unused: OverlayedBackend handles transfers
    }
}

impl RuntimeEnvironment for MajikOverlay<'_> {
    fn block_hash(&self, number: U256) -> H256 {
        self.overlay.block_hash(number)
    }

    fn block_number(&self) -> U256 {
        self.overlay.block_number()
    }

    fn block_coinbase(&self) -> H160 {
        self.overlay.block_coinbase()
    }

    fn block_timestamp(&self) -> U256 {
        self.overlay.block_timestamp()
    }

    fn block_difficulty(&self) -> U256 {
        self.overlay.block_difficulty()
    }

    fn block_randomness(&self) -> Option<H256> {
        self.overlay.block_randomness()
    }

    fn block_gas_limit(&self) -> U256 {
        self.overlay.block_gas_limit()
    }

    fn block_base_fee_per_gas(&self) -> U256 {
        self.overlay.block_base_fee_per_gas()
    }

    fn blob_base_fee_per_gas(&self) -> U256 {
        self.overlay.blob_base_fee_per_gas()
    }

    fn blob_versioned_hash(&self, index: U256) -> H256 {
        self.overlay.blob_versioned_hash(index)
    }

    fn chain_id(&self) -> U256 {
        self.overlay.chain_id()
    }
}

impl RuntimeEnvironment for MajikBackend {
    fn block_hash(&self, _number: U256) -> H256 {
        H256::zero() // Simplified
    }

    fn block_number(&self) -> U256 {
        self.environment.block_number
    }

    fn block_coinbase(&self) -> H160 {
        self.environment.block_coinbase
    }

    fn block_timestamp(&self) -> U256 {
        self.environment.block_timestamp
    }

    fn block_difficulty(&self) -> U256 {
        self.environment.block_difficulty
    }

    fn block_randomness(&self) -> Option<H256> {
        self.environment.block_randomness
    }

    fn block_gas_limit(&self) -> U256 {
        self.environment.block_gas_limit
    }

    fn block_base_fee_per_gas(&self) -> U256 {
        self.environment.block_base_fee_per_gas
    }

    fn blob_base_fee_per_gas(&self) -> U256 {
        U256::zero() // Simplified
    }

    fn blob_versioned_hash(&self, _index: U256) -> H256 {
        H256::zero() // Simplified
    }

    fn chain_id(&self) -> U256 {
        self.environment.chain_id
    }
}
