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

use alloc::{collections::BTreeMap, format, vec::Vec};
use core::cell::RefCell;
use evm::MergeStrategy;
use evm::backend::{
    OverlayedBackend, RuntimeBackend as EvmRuntimeBackend, RuntimeEnvironment, TransactionalBackend,
};
use evm::interpreter::{ExitError, ExitException};
use evm::interpreter::runtime::{
    Log, RuntimeBaseBackend, RuntimeConfig, SetCodeOrigin, TouchKind, Transfer,
};
use primitive_types::{H160, H256, U256};
use utils::effects::ObjectId;
use utils::functions::{log_error, log_info, log_debug};
use utils::objects::ObjectRef;
// ObjectDependency replaced with ObjectId
use utils::tracking::OverlayTracker;

use crate::receipt::TransactionReceiptRecord;
use crate::sharding::{
    ObjectKind, code_object_id, shard_object_id,
};
use crate::sharding::{ContractStorage, ContractShard, SSRHeader, ShardId, deserialize_shard, deserialize_ssr};
use utils::hash_functions::keccak256;
use utils::tracking::WriteKey;

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
    log_info(&format!(
        "🔑 balance_storage_key({:?}) -> {} (Solidity mapping hash)",
        address,
        crate::sharding::format_object_id(&key.0)
    ));
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
    log_info(&format!(
        "🔑 nonce_storage_key({:?}) -> {} (Solidity mapping hash)",
        address,
        crate::sharding::format_object_id(&key.0)
    ));
    key
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

}

// ===== MajikBackend =====

/// Majik backend with DA-style storage (no InMemoryBackend)
pub struct MajikBackend {
    pub code_storage: RefCell<BTreeMap<H160, Vec<u8>>>, // address -> bytecode
    pub code_hashes: RefCell<BTreeMap<H160, H256>>,     // address -> keccak256(code) cache
    pub negative_code_cache: RefCell<BTreeMap<H160, ()>>, // address -> () for EOAs with no code
    pub storage_shards: RefCell<BTreeMap<H160, ContractStorage>>, // address -> SSR + shards
    pub balances: RefCell<BTreeMap<H160, U256>>,        // address -> balance
    pub nonces: RefCell<BTreeMap<H160, U256>>,          // address -> nonce
    pub environment: EnvironmentData,
    pub service_id: u32, // Service ID for DA imports
    pub imported_objects: RefCell<BTreeMap<ObjectId, (ObjectRef, Vec<u8>)>>, // object_id -> (ref, payload)

    // Meta-shard state (JAM State object index)
    pub meta_ssr: RefCell<crate::meta_sharding::MetaSSR>,  // Global SSR for all ObjectRefs
    pub meta_shards: RefCell<BTreeMap<crate::da::ShardId, crate::meta_sharding::MetaShard>>,  // Cached meta-shards
}

impl MajikBackend {
    pub fn new(
        environment: EnvironmentData,
        imported_objects: BTreeMap<ObjectId, (ObjectRef, Vec<u8>)>,
    ) -> Self {
        // Load imported DA objects into backend state

        // Extract service_id from environment.chain_id
        // The service ID is passed via chain_id in the environment and used for DA operations
        let service_id = environment.chain_id.as_u32();

        let mut backend = Self {
            code_storage: RefCell::new(BTreeMap::new()),
            code_hashes: RefCell::new(BTreeMap::new()),
            negative_code_cache: RefCell::new(BTreeMap::new()),
            storage_shards: RefCell::new(BTreeMap::new()),
            balances: RefCell::new(BTreeMap::new()),
            nonces: RefCell::new(BTreeMap::new()),
            environment,
            service_id,
            imported_objects: RefCell::new(BTreeMap::new()),

            // Initialize meta-shard tracking
            meta_ssr: RefCell::new(crate::meta_sharding::MetaSSR::new()),
            meta_shards: RefCell::new(BTreeMap::new()),
        };

        // Initialize system contract (0x01) with empty SSR for balance/nonce tracking
        let system_contract = h160_from_low_u64_be(0x01);
        let initial_header = SSRHeader {
            global_depth: 0,
            entry_count: 0,
            total_keys: 0,
            version: 0, // Genesis version
        };
        let initial_ssr = crate::sharding::ssr_from_parts(initial_header, Vec::new());
        backend.storage_shards.borrow_mut().insert(
            system_contract,
            ContractStorage {
                ssr: initial_ssr,
                shards: BTreeMap::new(),
            },
        );

        for (object_id, (object_ref, payload)) in imported_objects.into_iter() {
            // Store in imported_objects cache for reuse
            backend.imported_objects.borrow_mut().insert(object_id, (object_ref.clone(), payload.clone()));

            // Determine object type from ObjectRef.object_kind() instead of object_id[31]
            // since object IDs no longer reliably encode kind in the last byte
            let mut address_bytes = [0u8; 20];
            address_bytes.copy_from_slice(&object_id[0..20]);
            let address = H160::from(address_bytes);

            match ObjectKind::from_u8(object_ref.object_kind as u8) {
                Some(ObjectKind::Code) => {
                    // Code object - load bytecode
                    backend.load_code(address, payload);
                }
                Some(ObjectKind::StorageShard) => {
                    // Storage shard object
                    // Layout: [20B address][0xFF][ld][prefix56 (7B)][padding (2B)][kind]
                    let ld = object_id[21];
                    let mut prefix56 = [0u8; 7];
                    prefix56.copy_from_slice(&object_id[22..29]);
                    let shard_id = ShardId { ld, prefix56 };

                    if let Some(shard_data) = deserialize_shard(&payload) {
                        let mut storage_shards = backend.storage_shards.borrow_mut();
                        let contract_storage = storage_shards
                            .entry(address)
                            .or_insert_with(|| ContractStorage::new(address));
                        contract_storage.shards.insert(shard_id, shard_data);
                    }
                }
                Some(ObjectKind::SsrMetadata) => {
                    // SSR metadata object
                    if let Some(ssr_data) = deserialize_ssr(&payload) {
                        let mut storage_shards = backend.storage_shards.borrow_mut();
                        let contract_storage = storage_shards
                            .entry(address)
                            .or_insert_with(|| ContractStorage::new(address));
                        contract_storage.ssr = ssr_data;
                    }
                }
                Some(ObjectKind::MetaShard) => {
                    // CRITICAL: Import meta-shards from witnesses to populate cache
                    // Without this, process_object_writes() creates empty shards and overwrites
                    // existing DA data, causing catastrophic data loss
                    use crate::meta_sharding::{
                        deserialize_meta_shard_with_id_validated,
                        MetaShardDeserializeResult,
                    };

                    // Deserialize with validated header
                    match deserialize_meta_shard_with_id_validated(&payload, &object_id, backend.service_id) {
                        MetaShardDeserializeResult::ValidatedHeader(shard_id, meta_shard) => {
                            log_info(&format!(
                                "  ✅ Imported MetaShard: shard_id={:?}, {} entries",
                                shard_id,
                                meta_shard.entries.len()
                            ));

                            // Insert into cache, even if empty (empty shards must be tracked!)
                            backend.meta_shards.borrow_mut().insert(shard_id, meta_shard);
                        }
                        MetaShardDeserializeResult::ValidationFailed => {
                            // Header present but validation failed - REJECT
                            log_error(&format!(
                                "  ❌ REJECTED MetaShard with invalid header: object_id={}",
                                crate::sharding::format_object_id(&object_id)
                            ));
                        }
                    }
                }
                Some(ObjectKind::Receipt) | Some(ObjectKind::Block) => {
                    // These object kinds are not imported during constructor - skip
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

        backend
    }

    /// Load code into backend (helper for constructor)
    pub fn load_code(&mut self, address: H160, code: Vec<u8>) {
        let code_hash = keccak256(&code);
        self.code_storage.borrow_mut().insert(address, code.clone());
        self.code_hashes.borrow_mut().insert(address, code_hash);
        // Version will be set when DA object is imported with metadata
    }

    /// Import storage shard from DA using fetch_object host function
    fn import_shard(&self, address: H160, shard_id: ShardId) -> Option<(ObjectRef, ContractShard)> {
        let object_id = shard_object_id(address, shard_id);

        log_debug(&format!(
            "  import_shard: address={:?}, shard_id (ld={}, prefix={:02x}{:02x}...), object_id={}",
            address,
            shard_id.ld,
            shard_id.prefix56[0],
            shard_id.prefix56[1],
            crate::sharding::format_object_id(&object_id)
        ));

        // Check if already in imported_objects cache
        {
            let imported = self.imported_objects.borrow();
            if let Some((object_ref, payload)) = imported.get(&object_id) {
                log_debug(&format!(
                    "  Found in imported_cache: address={:?}, shard_id (ld={}, prefix={:02x}{:02x}...), object_id={}",
                    address,
                    shard_id.ld,
                    shard_id.prefix56[0],
                    shard_id.prefix56[1],
                    crate::sharding::format_object_id(&object_id)
                ));
                let shard_data = deserialize_shard(payload)?;
                return Some((object_ref.clone(), shard_data));
            }
        }

        log_debug(&format!(
            "  Not in imported_cache, fetching from DA: address={:?}, shard_id (ld={}, prefix={:02x}{:02x}...), object_id={}",
            address,
            shard_id.ld,
            shard_id.prefix56[0],
            shard_id.prefix56[1],
            crate::sharding::format_object_id(&object_id)
        ));

        // Use fetch_object host function (254) to get both ObjectRef and payload
        const MAX_SHARD_SIZE: usize = 64 * 1024; // 64KB max shard size

        // For storage shards, missing from DA means empty (never written) - this is normal
        let (object_ref, shard_data) = match ObjectRef::fetch(self.service_id, &object_id, MAX_SHARD_SIZE) {
            Some((object_ref, payload)) => {
                // Shard exists in DA - deserialize it
                let shard_data = deserialize_shard(&payload)?;
                (object_ref, shard_data)
            }
            None => {
                // Shard doesn't exist in DA - return empty shard (normal for uninitialized storage)
                log_debug(&format!(
                    "  Shard not in DA (empty): address={:?}, shard_id (ld={}, prefix={:02x}{:02x}...) - returning empty shard",
                    address,
                    shard_id.ld,
                    shard_id.prefix56[0],
                    shard_id.prefix56[1]
                ));

                // Create a dummy ObjectRef for the empty shard
                let dummy_ref = ObjectRef {
                    work_package_hash: [0u8; 32],
                    index_start: 0,
                    payload_length: 0,
                    object_kind: 1, // StorageShard
                };

                let empty_shard = ContractShard {
                    merkle_root: [0u8; 32],
                    entries: Vec::new(),
                };

                (dummy_ref, empty_shard)
            }
        };

        // Cache imported shard for future lookups to avoid repeated DA fetches
        {
            let mut storage_shards = self.storage_shards.borrow_mut();
            let contract_storage = storage_shards
                .entry(address)
                .or_insert_with(|| ContractStorage::new(address));
            contract_storage.shards.insert(shard_id, shard_data.clone());
        }

        Some((object_ref, shard_data))
    }

    /// Import code from DA using fetch_object host function
    fn import_code(&self, address: H160) -> Option<(ObjectRef, Vec<u8>)> {
        let object_id = code_object_id(address);

        log_debug(&format!(
            "  import_code: address={:?}, object_id={}",
            address,
            crate::sharding::format_object_id(&object_id)
        ));

        // Check imported_objects cache first
        {
            let imported = self.imported_objects.borrow();
            if let Some((object_ref, payload)) = imported.get(&object_id) {
                log_debug(&format!(
                    "  Found in imported_cache: address={:?}, object_id={}",
                    address,
                    crate::sharding::format_object_id(&object_id)
                ));
                return Some((object_ref.clone(), payload.clone()));
            }
        }

        log_debug(&format!(
            "  Not in imported_cache, fetching from DA: address={:?}, object_id={}",
            address,
            crate::sharding::format_object_id(&object_id)
        ));

        // Use fetch_object host function (254)
        const MAX_CODE_SIZE: usize = 24576; // 24KB max contract size (EIP-170)
        let (object_ref, payload) = ObjectRef::fetch(self.service_id, &object_id, MAX_CODE_SIZE)?;

        self.imported_objects
            .borrow_mut()
            .insert(object_id, (object_ref.clone(), payload.clone()));

        Some((object_ref, payload))
    }

    /// Resolve shard ID for a given contract address and storage key hash
    pub(crate) fn resolve_shard_id_impl(&self, address: H160, storage_key: H256) -> ShardId {
        use crate::sharding::resolve_shard_id;
        self.storage_shards
            .borrow()
            .get(&address)
            .map(|cs| resolve_shard_id(cs, storage_key))
            .unwrap_or(ShardId {
                ld: 0,
                prefix56: [0u8; 7],
            })
    }

    /// Resolve storage dependency
    pub fn object_dependency_for_storage(&self, address: H160, key: H256) -> ObjectId {

        let shard_id = self.resolve_shard_id_impl(address, key);

        shard_object_id(address, shard_id)
    }

    /// Resolve balance dependency (stored in 0x01 precompile shards)
    pub fn object_dependency_for_balance(&self, address: H160) -> ObjectId {
        let system_contract = h160_from_low_u64_be(0x01);
        let balance_key = balance_storage_key(address);
        self.object_dependency_for_storage(system_contract, balance_key)
    }

    /// Resolve nonce dependency (stored in 0x01 precompile shards)
    pub fn object_dependency_for_nonce(&self, address: H160) -> ObjectId {
        let system_contract = h160_from_low_u64_be(0x01);
        let nonce_key = nonce_storage_key(address);
        self.object_dependency_for_storage(system_contract, nonce_key)
    }

    /// Resolve code ObjectId for bytecode reads
    pub fn object_dependency_for_code(&self, address: H160) -> ObjectId {
        code_object_id(address)
    }

    // ===== Meta-shard access methods =====

    /// Get mutable reference to meta-SSR for processing object writes
    pub fn meta_ssr_mut(&self) -> core::cell::RefMut<'_, crate::meta_sharding::MetaSSR> {
        self.meta_ssr.borrow_mut()
    }

    /// Get mutable reference to meta-shards cache
    pub fn meta_shards_mut(&self) -> core::cell::RefMut<'_, BTreeMap<crate::da::ShardId, crate::meta_sharding::MetaShard>> {
        self.meta_shards.borrow_mut()
    }

}

// ===== MajikOverlay struct and coordination =====

/// MajikOverlay wraps vendor OverlayedBackend with dependency tracking
///
/// Primary runtime entry point that coordinates:
/// - Vendor OverlayedBackend for change buffering
/// - OverlayTracker for read/write dependency tracking
/// - Transaction isolation and frame management
pub struct MajikOverlay<'config> {
    pub(crate) overlay: OverlayedBackend<'config, MajikBackend>,
    pub(crate) tracker: RefCell<OverlayTracker>,
    /// Index of the transaction currently executing.
    /// Initialized to `None` in `new`, set in `begin_transaction`, and cleared in `commit_transaction`/`revert_transaction`.
    current_tx_index: Option<usize>,
    /// Scratch buffer for logs emitted by the active transaction.
    /// Created empty in `new`, reset on `begin_transaction`, filled by `log`, and drained on commit/revert.
    current_tx_logs: Vec<Log>,
    /// Per-transaction log storage retained after commits so refine can fetch logs later.
    /// Resized in `begin_transaction`, populated in `commit_transaction`, and drained via `take_transaction_logs`.
    tx_logs: Vec<Vec<Log>>,
}

impl<'config> MajikOverlay<'config> {
    /// Create new overlay with backend and runtime config
    pub fn new(backend: MajikBackend, config: &'config RuntimeConfig) -> Self {
        Self {
            overlay: OverlayedBackend::new(backend, config),
            tracker: RefCell::new(OverlayTracker::new()),
            current_tx_index: None,
            current_tx_logs: Vec::new(),
            tx_logs: Vec::new(),
        }
    }

    /// Begin new transaction with isolated read tracking
    pub fn begin_transaction(&mut self, tx_index: usize) {
        self.overlay.push_substate();
        self.tracker.borrow_mut().begin_transaction(tx_index);
        self.current_tx_index = Some(tx_index);
        self.current_tx_logs.clear();
        if self.tx_logs.len() <= tx_index {
            self.tx_logs.resize(tx_index + 1, Vec::new());
        }
        self.tx_logs[tx_index].clear();
    }

    /// Commit transaction and snapshot reads
    pub fn commit_transaction(&mut self) -> Result<(), ExitError> {
        self.overlay.pop_substate(MergeStrategy::Commit)?;
        self.tracker.borrow_mut().commit_transaction();
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
        self.tracker.borrow_mut().revert_transaction();
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

    /// Deconstructs the overlay and converts changes to ExecutionEffects
    ///
    /// Returns ExecutionEffects for the accumulate phase by calling to_execution_effects
    /// Also returns the backend so caller can access meta-shard state
    pub fn deconstruct(
        self,
        work_package_hash: [u8; 32],
        receipts: &[TransactionReceiptRecord],
        state_root: [u8; 32],
        timeslot: u32,
        total_gas_used: u64,
        _service_id: u32,
    ) -> (utils::effects::ExecutionEffects, MajikBackend) {
        let tracker = self.tracker.into_inner().into_output();
        let (backend, change_set) = self.overlay.deconstruct();

        // Convert to ExecutionEffects and assemble block (block logged but not returned)
        let effects = backend.to_execution_effects(
            &change_set,
            &tracker,
            work_package_hash,
            receipts,
            state_root,
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
        self.tracker.borrow_mut().enter_frame();
    }

    fn pop_substate(&mut self, strategy: MergeStrategy) -> Result<(), ExitError> {
        self.overlay.pop_substate(strategy)?;
        let mut tracker = self.tracker.borrow_mut();
        match strategy {
            MergeStrategy::Commit => tracker.commit_frame(),
            MergeStrategy::Revert | MergeStrategy::Discard => tracker.revert_frame(),
        }
        Ok(())
    }
}

impl RuntimeBaseBackend for MajikOverlay<'_> {
    /// Balance read via system-contract storage with TRAP prevention
    /// JAM DA: Balance stored as storage slot in 0x01 contract, key = keccak256(address)
    /// Dependency tracking: Records shard ObjectID and version for work package
    fn balance(&self, address: H160) -> U256 {
        // Cache miss - call through to OverlayedBackend
        let value = self.overlay.balance(address);
        let dependency = self
            .overlay
            .backend()
            .object_dependency_for_balance(address);
        self.tracker.borrow_mut().record_read(dependency);
        value
    }

    /// Code ObjectID resolution and DA import
    /// JAM DA: Code stored as [20B address][1B kind=0x00][11B zero] in 4KB segments
    /// Dependency tracking: Records code object version
    fn code(&self, address: H160) -> Vec<u8> {
        let bytes = self.overlay.code(address);
        let dependency = self.overlay.backend().object_dependency_for_code(address);
        self.tracker.borrow_mut().record_read(dependency);
        bytes
    }

    /// Code hash with caching
    /// JAM DA: Code hash computed on load and cached in code_hashes BTreeMap
    /// Optimization: EXTCODEHASH can use cached value, avoiding keccak256 recomputation
    /// Dependency tracking: Records code object read for JAM work package validation
    fn code_hash(&self, address: H160) -> H256 {
        // Overlay handles the actual lookup (checks cache, then calls backend)
        let hash = self.overlay.code_hash(address);
        let dependency = self.overlay.backend().object_dependency_for_code(address);
        self.tracker.borrow_mut().record_read(dependency);
        hash
    }

    /// Storage read via SSR shard resolution
    /// JAM DA: Resolves ShardId via SSR, binary search within shard, tracks dependency
    /// Dependency tracking: Records shard ObjectID and version
    fn storage(&self, address: H160, index: H256) -> H256 {
        let value = self.overlay.storage(address, index);
        let dependency = self
            .overlay
            .backend()
            .object_dependency_for_storage(address, index);
        self.tracker.borrow_mut().record_read(dependency);
        value
    }

    /// Transient storage (EIP-1153) handled by overlay
    /// JAM DA: NOT persisted to DA, purely in-memory, cleared at transaction boundaries
    /// Implementation: Vendor OverlayedBackend maintains in-memory BTreeMap, cleared on transaction commit/revert
    /// No dependency tracking needed - ephemeral scratch space
    fn transient_storage(&self, address: H160, index: H256) -> H256 {
        self.overlay.transient_storage(address, index)
    }

    /// EIP-161 exists check handled by overlay
    /// JAM DA: Returns true if (balance > 0) OR (nonce > 0) OR (code_size > 0)
    /// Implementation: Overlay checks eip161_empty_check config, then calls balance/nonce/code_size
    /// Backend provides: Balance/nonce from system-contract storage, code_size from code objects
    /// Empty account: All three values are zero (account does not exist per EIP-161)
    fn exists(&self, address: H160) -> bool {
        self.overlay.exists(address)
    }

    /// Nonce read via system-contract storage
    /// JAM DA: Storage key = keccak256(address || "nonce") in 0x01 contract
    /// Dependency tracking: Records shard ObjectID and version
    fn nonce(&self, address: H160) -> U256 {
        let value = self.overlay.nonce(address);
        let dependency = self.overlay.backend().object_dependency_for_nonce(address);
        self.tracker.borrow_mut().record_read(dependency);
        value
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

    /// Storage write with copy-on-write shard export
    /// JAM DA: Cached in overlay, exported to DA at accumulate with COW
    /// Write tracking: Records transaction index and instance counter
    fn set_storage(&mut self, address: H160, key: H256, value: H256) -> Result<(), ExitError> {
        self.overlay.set_storage(address, key, value)?;
        self.tracker
            .borrow_mut()
            .record_write(WriteKey::storage(address, key));
        Ok(())
    }

    /// Set transient storage (EIP-1153) handled by overlay
    /// JAM DA: In-memory only, cleared at transaction boundaries, no DA export
    /// Implementation: Vendor OverlayedBackend inserts into transient_storage BTreeMap
    /// Write tracking: Records writes for deterministic ordering, but not persisted beyond transaction
    fn set_transient_storage(
        &mut self,
        address: H160,
        key: H256,
        value: H256,
    ) -> Result<(), ExitError> {
        self.overlay.set_transient_storage(address, key, value)?;
        self.tracker
            .borrow_mut()
            .record_write(WriteKey::transient_storage(address, key));
        Ok(())
    }

    /// Accumulate logs for DA receipt export
    /// JAM DA: Logs collected in overlay and exported to Receipt objects (kind=0x03) in to_execution_effects
    /// Serialization: serialize_logs() packs all logs into binary format for DA
    /// Write tracking: Records log emission for deterministic ordering
    fn log(&mut self, log: Log) -> Result<(), ExitError> {
        self.overlay.log(log.clone())?;
        self.tracker
            .borrow_mut()
            .record_write(WriteKey::log(log.address));
        self.current_tx_logs.push(log);
        Ok(())
    }

    /// Account deletion with EIP-6780 restrictions
    /// JAM DA: Deletion handled via change_set.deletes - exports code tombstones (empty payload)
    /// Export: to_execution_effects() exports empty code objects and storage_resets for deleted accounts
    /// Storage reset: SSR reset to empty state with incremented version, all shards cleared
    /// Gas refund: 24000 gas refund handled by EVM execution layer (not in DA export)
    fn mark_delete_reset(&mut self, address: H160) {
        self.overlay.mark_delete_reset(address);
        self.tracker
            .borrow_mut()
            .record_write(WriteKey::delete(address));
    }

    /// Mark account as created for EIP-6780 checks
    /// JAM DA: Tracked in overlay, enables same-transaction SELFDESTRUCT
    /// Write tracking: Records creation event with transaction index
    fn mark_create(&mut self, address: H160) {
        self.overlay.mark_create(address);
        self.tracker
            .borrow_mut()
            .record_write(WriteKey::create(address));
    }

    /// Clear storage SSR and all shards before CREATE deployment
    /// JAM DA: Removes SSR header and all shard ObjectRefs, creates fresh SSR
    /// EIP-7610: Should only occur for empty targets (collision detection)
    fn reset_storage(&mut self, address: H160) {
        self.overlay.reset_storage(address);
        self.tracker
            .borrow_mut()
            .record_write(WriteKey::storage_reset(address));
    }

    /// Export bytecode to DA as code objects
    /// JAM DA: Code stored as [20B address][1B kind=0x00][11B zero] in DA
    /// Export: to_execution_effects() exports code changes to code objects with version tracking
    /// Write tracking: Records code installation, version bumped on change
    fn set_code(
        &mut self,
        address: H160,
        code: Vec<u8>,
        origin: SetCodeOrigin,
    ) -> Result<(), ExitError> {
        self.overlay.set_code(address, code, origin)?;
        self.tracker
            .borrow_mut()
            .record_write(WriteKey::code(address));
        Ok(())
    }

    /// Balance deposits tracked via standard storage shards
    /// JAM DA: Balance updates go through storage system (contract 0x01) to avoid conflicts with USDM transfers
    /// Export: to_execution_effects() persists balance changes through regular shard objects
    /// Write tracking: Records storage write for balance
    fn deposit(&mut self, target: H160, value: U256) {
        // Use storage operations instead of balance operations to avoid conflicts with USDM
        let system_contract = h160_from_low_u64_be(0x01);
        let storage_key = balance_storage_key(target);

        // Read current balance from storage
        let current_balance = self.overlay.storage(system_contract, storage_key);
        let current_value = U256::from_big_endian(current_balance.as_bytes());

        // Add deposit amount
        let new_value = current_value.saturating_add(value);
        let mut encoded = [0u8; 32];
        new_value.to_big_endian(&mut encoded);
        let new_storage_value = H256::from(encoded);

        // Write new balance to storage
        self.overlay.set_storage(system_contract, storage_key, new_storage_value).unwrap();
        self.tracker
            .borrow_mut()
            .record_write(WriteKey::storage(system_contract, storage_key));
    }

    /// Balance withdrawals tracked via standard storage shards
    /// JAM DA: Balance updates go through storage system (contract 0x01) to avoid conflicts with USDM transfers
    /// Export: to_execution_effects() persists balance changes through regular shard objects
    /// Write tracking: Records storage write for balance
    fn withdrawal(&mut self, source: H160, value: U256) -> Result<(), ExitError> {
        // Use storage operations instead of balance operations to avoid conflicts with USDM
        let system_contract = h160_from_low_u64_be(0x01);
        let storage_key = balance_storage_key(source);

        // Read current balance from storage
        let current_balance = self.overlay.storage(system_contract, storage_key);
        let current_value = U256::from_big_endian(current_balance.as_bytes());

        // Check sufficient balance
        if current_value < value {
            return Err(ExitException::OutOfFund.into());
        }

        // Subtract withdrawal amount
        let new_value = current_value - value;
        let mut encoded = [0u8; 32];
        new_value.to_big_endian(&mut encoded);
        let new_storage_value = H256::from(encoded);

        // Write new balance to storage
        self.overlay.set_storage(system_contract, storage_key, new_storage_value).unwrap();
        self.tracker
            .borrow_mut()
            .record_write(WriteKey::storage(system_contract, storage_key));

        Ok(())
    }

    /// Atomic transfers mirrored into standard storage shards
    /// JAM DA: May touch two separate shard objects for source/target balances
    /// Export: to_execution_effects() records both balance changes through shard writes
    /// Write tracking: Records both balance changes, depends on both shard versions
    fn transfer(&mut self, transfer: Transfer) -> Result<(), ExitError> {
        let result = self.overlay.transfer(transfer.clone());
        if result.is_ok() {
            let mut tracker = self.tracker.borrow_mut();
            tracker.record_write(WriteKey::balance(transfer.source));
            tracker.record_write(WriteKey::balance(transfer.target));
        }
        result
    }

    /// Nonce increments mirrored into system-contract storage
    /// JAM DA: Nonce shares the same shard keys as balances (0x01 contract)
    /// Export: to_execution_effects() persists updates via standard shard objects
    /// Write tracking: Records nonce increment for replay protection
    fn inc_nonce(&mut self, address: H160) -> Result<(), ExitError> {
        self.overlay.inc_nonce(address)?;
        self.tracker
            .borrow_mut()
            .record_write(WriteKey::nonce(address));
        Ok(())
    }
}

// ===== MajikBackend trait implementations =====

impl RuntimeBaseBackend for MajikBackend {
    /// Balance with DA import on cache miss
    /// Accounts are backed by the 0x01 system contract storage just like any other contract.
    /// On cache miss we read the corresponding storage slot (keccak256(address)) via storage().
    fn balance(&self, address: H160) -> U256 {
        // Check cache first
        {
            let balances = self.balances.borrow();
            if let Some(balance) = balances.get(&address) {
                return *balance;
            }
        }

        let system_contract = h160_from_low_u64_be(0x01);
        let storage_key = balance_storage_key(address);
        let value_h256 = self.storage(system_contract, storage_key);
        let balance = U256::from_big_endian(value_h256.as_bytes());

        // Cache the balance (including zero) to prevent infinite DA fetches
        self.balances.borrow_mut().insert(address, balance);
        balance
    }

    /// Code from cache with DA import on miss
    /// DA-STORAGE.md §10: On miss, Read(code_object_id) → Import 4KB segments → reassemble
    /// Current: Calls import_code() which fetches and reassembles from DA
    /// Note: Caching requires mut access - handled by caller or RefCell wrapper
    fn code(&self, address: H160) -> Vec<u8> {
        // Check positive cache first
        {
            let code_storage = self.code_storage.borrow();
            if let Some(code) = code_storage.get(&address) {
                return code.clone();
            }
        }

        // Check negative cache - if we already know this address has no code, return empty
        {
            let negative_cache = self.negative_code_cache.borrow();
            if negative_cache.contains_key(&address) {
                return Vec::new();
            }
        }

        // Cache miss - attempt DA import via import_code()
        if let Some((_object_ref, bytecode)) = self.import_code(address) {
            // Cache the imported code and its hash
            let code_hash = keccak256(&bytecode);
            self.code_storage
                .borrow_mut()
                .insert(address, bytecode.clone());
            self.code_hashes.borrow_mut().insert(address, code_hash);
            return bytecode;
        }

        // Negative caching: remember EOAs with no bytecode so we do not keep hitting DA
        self.negative_code_cache.borrow_mut().insert(address, ());

        Vec::new()
    }

    /// Code hash with caching optimization
    /// JAM DA: Code hash cached in code_hashes BTreeMap on code load
    /// Optimization: EXTCODEHASH opcode uses cached hash, avoiding expensive recomputation
    /// Fallback: If not cached, computes hash from code (for imported-but-not-cached scenario)
    fn code_hash(&self, address: H160) -> H256 {
        // Check cache first
        {
            let code_hashes = self.code_hashes.borrow();
            if let Some(hash) = code_hashes.get(&address) {
                return *hash;
            }
        }

        // Cache miss - compute from code if exists
        if self.exists(address) {
            let code = self.code(address);
            let hash = keccak256(&code[..]);

            // Cache the computed hash
            self.code_hashes.borrow_mut().insert(address, hash);
            hash
        } else {
            H256::zero()
        }
    }

    /// Storage via SSR shard resolution with DA import on miss
    /// DA-STORAGE.md §10: On miss, Read(shard_object_id) → Import 4KB shard → deserialize entries
    /// Current: Calls import_shard() which fetches and deserializes from DA
    /// Note: Caching requires mut access - handled by caller or RefCell wrapper
    fn storage(&self, address: H160, key: H256) -> H256 {
        let shard_id = self.resolve_shard_id_impl(address, key);
        // Check cache first
        {
            let storage_shards = self.storage_shards.borrow();
            if let Some(contract_storage) = storage_shards.get(&address) {
                if let Some(shard_data) = contract_storage.shards.get(&shard_id) {
                    match shard_data
                        .entries
                        .binary_search_by_key(&key, |entry| entry.key_h)
                    {
                        Ok(idx) => return shard_data.entries[idx].value,
                        Err(_) => return H256::zero(),
                    }
                }
            }
        }
        // TODO: Re-enable this check once we properly handle DA imports for different payload types
        // if self.environment.payload_type != crate::refiner::PayloadType::Builder {
        //     return H256::zero();
        // }
        // Cache miss - attempt DA import via import_shard()
        if let Some((_object_ref, shard_data)) = self.import_shard(address, shard_id) {
            // Search in the freshly imported shard
            match shard_data
                .entries
                .binary_search_by_key(&key, |entry| entry.key_h)
            {
                Ok(idx) => {
                    let value = shard_data.entries[idx].value;
                    // Note: Can't cache here due to &self, caller must handle caching
                    return value;
                }
                Err(_) => {
                    log_debug(&format!(
                        "  Key not found in imported shard: address={:?}, key={:?}, shard_id (ld={}, prefix={:02x}{:02x}...)",
                        address,
                        crate::sharding::format_object_id(&key.0),
                        shard_id.ld,
                        shard_id.prefix56[0],
                        shard_id.prefix56[1]
                    ));
                }
            }
        } else {
            log_error(&format!(
                "  Failed to import shard from DA: address={:?}, key={:?}, shard_id (ld={}, prefix={:02x}{:02x}{:02x}{:02x}{:02x}{:02x}{:02x})",
                address,
                crate::sharding::format_object_id(&key.0),
                shard_id.ld,
                shard_id.prefix56[0],
                shard_id.prefix56[1],
                shard_id.prefix56[2],
                shard_id.prefix56[3],
                shard_id.prefix56[4],
                shard_id.prefix56[5],
                shard_id.prefix56[6]
            ));
        }

        H256::zero()
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
    /// Nonces are stored alongside balances inside the 0x01 system contract.
    fn nonce(&self, address: H160) -> U256 {
        // Check cache first
        {
            let nonces = self.nonces.borrow();
            if let Some(nonce) = nonces.get(&address) {
                return *nonce;
            }
        }

        let system_contract = h160_from_low_u64_be(0x01);
        let storage_key = nonce_storage_key(address);
        let value_h256 = self.storage(system_contract, storage_key);
        let nonce = U256::from_big_endian(value_h256.as_bytes());

        // Cache the nonce (including zero) to prevent infinite DA fetches
        self.nonces.borrow_mut().insert(address, nonce);
        nonce
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
