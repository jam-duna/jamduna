//! JAM Epoch Verkle Index for Service-Aware Light Clients
//!
//! This module implements an epoch-wide Verkle index over EVM logs, allowing light clients
//! to efficiently query which blocks and DA chunks contain events relevant to them.
//!
//! See `services/evm/docs/VERKLE.md` for full specification.
//!
//! ## Design Overview
//!
//! - **SNARK-free**: No prover infrastructure required
//! - **Consensus-enforced**: No false negatives allowed
//! - **Verkle-based**: Small proofs, efficient updates
//! - **DA-granular**: Light clients download only relevant DA chunks
//!
//! ## Implementation Phases
//!
//! **Phase 1 (Prototype)**: `SimpleVerkleTree` with in-memory `BTreeMap<IndexKey, Vec<LogId>>`
//! **Phase 2 (Production)**: Real Verkle tree with KZG commitments over BLS12-381

use alloc::collections::BTreeMap;
use alloc::vec::Vec;
use primitive_types::{H160, H256};
use evm::interpreter::runtime::Log;

// ============================================================================
// Type Aliases
// ============================================================================

/// 32-byte index key (output of blake2b hash)
pub type IndexKey = [u8; 32];

/// 32-byte Verkle root commitment
pub type VerkleRoot = [u8; 32];

/// Prototype: in-memory pointer lists
pub type PointerList = Vec<LogId>;

// ============================================================================
// Index Key Type Discriminators
// ============================================================================

/// Index key type discriminators for different query patterns
pub mod key_types {
    pub const CONTRACT_KEY: u8 = 0x01;      // Find logs from specific contract
    pub const EVENT_TOPIC0_KEY: u8 = 0x02;  // Find logs of specific event type
    pub const INDEXED_ADDR_KEY: u8 = 0x03;  // Find logs with address in indexed params
    pub const INDEXED_UINT_KEY: u8 = 0x04;  // Find logs with uint256 in indexed params
    pub const INDEXED_BYTES32_KEY: u8 = 0x05; // Find logs with bytes32 in indexed params
}

// ============================================================================
// LogId: Compact 128-bit Pointer with DA Granularity
// ============================================================================

/// Compact reference into the epoch's DA + log space.
///
/// Bit layout (MSB→LSB):
/// ```text
/// [ 16 bits epoch_local_block ]   // up to 65,535 blocks per epoch
/// [ 16 bits tx_index ]            // up to 65,535 txs per block
/// [ 16 bits log_index ]           // up to 65,535 logs per tx
/// [ 16 bits da_chunk_id ]         // up to 65,535 DA chunks in epoch
/// [ 32 bits offset_in_chunk ]     // byte offset within DA chunk
/// Total: 128 bits (16 bytes)
/// ```
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash)]
pub struct LogId(u128);

/// Error types for LogId construction
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum LogIdError {
    BlockInEpochTooLarge,
    TxIndexTooLarge,
    LogIndexTooLarge,
    DaChunkIdTooLarge,
    OffsetTooLarge,
}

impl LogId {
    /// Maximum values for validation (defined by bit allocation)
    pub const MAX_BLOCK_IN_EPOCH: u32 = 0xFFFF;  // 16 bits
    pub const MAX_TX_INDEX: u32 = 0xFFFF;         // 16 bits
    pub const MAX_LOG_INDEX: u32 = 0xFFFF;        // 16 bits
    pub const MAX_DA_CHUNK_ID: u32 = 0xFFFF;      // 16 bits
    pub const MAX_OFFSET_IN_CHUNK: u64 = 0xFFFFFFFF; // 32 bits

    /// Create a new LogId with validation
    ///
    /// Returns `Err` if any field exceeds its bit allocation.
    pub fn new(
        block_in_epoch: u16,
        tx_index: u16,
        log_index: u16,
        da_chunk_id: u16,
        offset_in_chunk: u32,
    ) -> Result<Self, LogIdError> {
        // All u16/u32 inputs are inherently within range,
        // but this API documents the constraints explicitly.
        Ok(Self::new_unchecked(
            block_in_epoch,
            tx_index,
            log_index,
            da_chunk_id,
            offset_in_chunk,
        ))
    }

    /// Create a new LogId without validation (for internal use)
    #[inline]
    pub fn new_unchecked(
        block_in_epoch: u16,
        tx_index: u16,
        log_index: u16,
        da_chunk_id: u16,
        offset_in_chunk: u32,
    ) -> Self {
        let mut val: u128 = 0;
        val |= (block_in_epoch as u128) << 112;
        val |= (tx_index as u128) << 96;
        val |= (log_index as u128) << 80;
        val |= (da_chunk_id as u128) << 64;
        val |= offset_in_chunk as u128;
        LogId(val)
    }

    /// Extract block index within the epoch (0-based)
    #[inline]
    pub fn block_in_epoch(&self) -> u16 {
        ((self.0 >> 112) & 0xFFFF) as u16
    }

    /// Extract transaction index within the block
    #[inline]
    pub fn tx_index(&self) -> u16 {
        ((self.0 >> 96) & 0xFFFF) as u16
    }

    /// Extract log index within the transaction
    #[inline]
    pub fn log_index(&self) -> u16 {
        ((self.0 >> 80) & 0xFFFF) as u16
    }

    /// Extract DA chunk ID within the epoch
    #[inline]
    pub fn da_chunk_id(&self) -> u16 {
        ((self.0 >> 64) & 0xFFFF) as u16
    }

    /// Extract byte offset within the DA chunk
    #[inline]
    pub fn offset_in_chunk(&self) -> u32 {
        (self.0 & 0xFFFFFFFF) as u32
    }

    /// Get raw 128-bit value
    #[inline]
    pub fn as_u128(&self) -> u128 {
        self.0
    }

    /// Construct from raw 128-bit value
    #[inline]
    pub fn from_u128(val: u128) -> Self {
        LogId(val)
    }
}

// ============================================================================
// IndexValue: Small Fixed-Size Value for Production Verkle Leaves
// ============================================================================

/// Small, fixed-size value stored directly in the Verkle leaf per (key, epoch).
///
/// In production, the actual pointer lists are stored externally (in DA or a side-tree).
/// The Verkle tree only commits to their existence and integrity.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct IndexValue {
    /// Total number of logs for this key in this epoch
    pub total_count: u64,
    /// Merkle root of LogId list stored in DA or side-tree
    pub pointer_list_root: [u8; 32],
}

// ============================================================================
// Verkle Proof Structures
// ============================================================================

/// Verkle proof for batch membership/non-membership verification
#[derive(Debug, Clone)]
pub struct VerkleBatchProof {
    /// Implementation-specific proof structure
    /// For production: KZG commitments + opening proofs.
    /// For prototype: serialized key-value pairs
    pub proof_data: Vec<u8>,
}

// ============================================================================
// VerkleTree Trait
// ============================================================================

/// Verkle tree for epoch-wide log indexing
///
/// This trait abstracts over both prototype and production implementations.
pub trait VerkleTree {
    /// Create a new empty Verkle tree
    fn new() -> Self;

    /// Insert or update the value for a key
    ///
    /// Production implementation: stores `IndexValue` directly
    fn upsert(&mut self, key: IndexKey, value: IndexValue);

    /// Get the value for a given key
    ///
    /// Returns `None` if key is not present in the tree
    fn get(&self, key: &IndexKey) -> Option<IndexValue>;

    /// Compute and return the Verkle root commitment
    ///
    /// This should be called after all insertions for a block/epoch
    fn commit(&self) -> VerkleRoot;

    /// Generate a batch proof for multiple keys
    ///
    /// Includes both membership (key exists) and non-membership (key absent).
    fn prove_batch(&self, keys: &[IndexKey]) -> VerkleBatchProof;

    /// Verify a batch proof against a known root
    ///
    /// `values` maps keys to expected `IndexValue`.
    fn verify_batch(
        root: VerkleRoot,
        keys: &[IndexKey],
        values: &BTreeMap<IndexKey, IndexValue>,
        proof: &VerkleBatchProof,
    ) -> bool;
}

// ============================================================================
// SimpleVerkleTree: Prototype Implementation
// ============================================================================

/// Simple in-memory "Verkle" tree using BTreeMap (for prototyping)
///
/// Replace with real Verkle (256-way branching, KZG commitments) for production.
///
/// This implementation:
/// - Stores `Vec<LogId>` directly in memory
/// - Uses deterministic iteration order via `BTreeMap`
/// - Computes a simple hash-based "root" for testing
pub struct SimpleVerkleTree {
    /// In-memory storage: IndexKey → Vec<LogId>
    pub entries: BTreeMap<IndexKey, PointerList>,
}

impl SimpleVerkleTree {
    /// Insert log IDs for a given key
    ///
    /// This is a convenience method for the prototype.
    pub fn insert_log_ids(&mut self, key: IndexKey, new_ids: &[LogId]) {
        self.entries
            .entry(key)
            .or_insert_with(Vec::new)
            .extend_from_slice(new_ids);
    }

    /// Helper: Compute pointer list root (simple hash for prototype)
    fn compute_pointer_list_root(ids: &[LogId]) -> [u8; 32] {
        use utils::hash_functions::blake2b_hash;

        let mut bytes = Vec::new();
        for id in ids {
            bytes.extend_from_slice(&id.as_u128().to_be_bytes());
        }
        blake2b_hash(&bytes)
    }
}

impl VerkleTree for SimpleVerkleTree {
    fn new() -> Self {
        Self {
            entries: BTreeMap::new(),
        }
    }

    fn upsert(&mut self, key: IndexKey, value: IndexValue) {
        // Prototype: ignore IndexValue and keep raw ids
        // In production, you'd store IndexValue directly.
        let _ = (key, value);
        // For prototype, use insert_log_ids() instead
    }

    fn get(&self, key: &IndexKey) -> Option<IndexValue> {
        // Prototype: derive IndexValue from Vec<LogId>
        self.entries.get(key).map(|ids| {
            let total_count = ids.len() as u64;
            let pointer_list_root = Self::compute_pointer_list_root(ids);

            IndexValue {
                total_count,
                pointer_list_root,
            }
        })
    }

    fn commit(&self) -> VerkleRoot {
        // Prototype: hash all (key, ids) pairs
        // Production: compute actual Verkle polynomial commitment
        use utils::hash_functions::blake2b_hash;

        let mut serialized = Vec::new();
        for (key, ids) in &self.entries {
            serialized.extend_from_slice(key);
            serialized.extend_from_slice(&(ids.len() as u32).to_le_bytes());
            for id in ids {
                serialized.extend_from_slice(&id.as_u128().to_be_bytes());
            }
        }

        blake2b_hash(&serialized)
    }

    fn prove_batch(&self, keys: &[IndexKey]) -> VerkleBatchProof {
        // Prototype: serialize all data; no real Verkle proof.
        // Production: generate actual Verkle KZG opening proofs
        let mut proof_data = Vec::new();
        for key in keys {
            proof_data.extend_from_slice(key);
        }
        VerkleBatchProof { proof_data }
    }

    fn verify_batch(
        root: VerkleRoot,
        keys: &[IndexKey],
        values: &BTreeMap<IndexKey, IndexValue>,
        _proof: &VerkleBatchProof,
    ) -> bool {
        // Prototype: recompute root from values; compare.
        // Production: verify KZG opening proofs against polynomial commitments
        use utils::hash_functions::blake2b_hash;

        let mut serialized = Vec::new();
        for key in keys {
            if let Some(v) = values.get(key) {
                serialized.extend_from_slice(key);
                serialized.extend_from_slice(&v.total_count.to_le_bytes());
                serialized.extend_from_slice(&v.pointer_list_root);
            }
        }

        blake2b_hash(&serialized) == root
    }
}

// ============================================================================
// Index Key Derivation
// ============================================================================

/// Derive index keys from a log entry for Verkle insertion
///
/// Basic implementation that indexes:
/// - Contract address (always)
/// - Event topic0 (if present)
/// - All indexed parameters as addresses (topic1/2/3)
///
/// For type-aware indexing (distinguishing uint256, bytes32, etc.),
/// use `derive_index_keys_typed()` with ABI metadata.
pub fn derive_index_keys_from_log(log: &Log) -> Vec<IndexKey> {
    use utils::hash_functions::blake2b_hash;

    let mut keys = Vec::new();

    // 1. Contract address key
    let mut contract_preimage = Vec::with_capacity(1 + 20);
    contract_preimage.push(key_types::CONTRACT_KEY);
    contract_preimage.extend_from_slice(log.address.as_bytes());
    keys.push(blake2b_hash(&contract_preimage));

    // 2. Event topic0 key (if present)
    if let Some(topic0) = log.topics.get(0) {
        let mut topic0_preimage = Vec::with_capacity(1 + 32);
        topic0_preimage.push(key_types::EVENT_TOPIC0_KEY);
        topic0_preimage.extend_from_slice(topic0.as_bytes());
        keys.push(blake2b_hash(&topic0_preimage));
    }

    // 3. Indexed address keys (topic1, topic2, topic3)
    for (position, topic) in log.topics.iter().skip(1).take(3).enumerate() {
        let position_byte = (position + 1) as u8; // 1, 2, or 3
        let mut indexed_preimage = Vec::with_capacity(1 + 1 + 32);
        indexed_preimage.push(key_types::INDEXED_ADDR_KEY);
        indexed_preimage.push(position_byte);
        indexed_preimage.extend_from_slice(topic.as_bytes());
        keys.push(blake2b_hash(&indexed_preimage));
    }

    keys
}

/// Derive user index keys for light client queries
///
/// Example: A user tracking specific contracts and events
pub fn derive_user_index_keys(
    contracts: &[H160],       // Contracts user cares about
    event_sigs: &[&str],      // Event signatures like "Transfer(address,address,uint256)"
    user_address: H160,       // User's own address
) -> Vec<IndexKey> {
    use utils::hash_functions::{blake2b_hash, keccak256};

    let mut keys = Vec::new();

    // Track all events from specific contracts
    for contract in contracts {
        let mut preimage = Vec::with_capacity(1 + 20);
        preimage.push(key_types::CONTRACT_KEY);
        preimage.extend_from_slice(contract.as_bytes());
        keys.push(blake2b_hash(&preimage));
    }

    // Track specific event types globally
    for sig in event_sigs {
        let topic0 = keccak256(sig.as_bytes());
        let mut preimage = Vec::with_capacity(1 + 32);
        preimage.push(key_types::EVENT_TOPIC0_KEY);
        preimage.extend_from_slice(topic0.as_bytes());
        keys.push(blake2b_hash(&preimage));
    }

    // Track events where user's address appears in indexed params (topic1/2/3)
    let user_topic = {
        let mut buf = [0u8; 32];
        buf[12..32].copy_from_slice(user_address.as_bytes());
        H256(buf)
    };

    for position in 1..=3 {
        let mut preimage = Vec::with_capacity(1 + 1 + 32);
        preimage.push(key_types::INDEXED_ADDR_KEY);
        preimage.push(position);
        preimage.extend_from_slice(user_topic.as_bytes());
        keys.push(blake2b_hash(&preimage));
    }

    keys
}

// ============================================================================
// Per-Block Processing
// ============================================================================

/// Process a block's logs and update the epoch index
///
/// This function should be called once per block during epoch processing.
///
/// # Arguments
///
/// * `tree` - The epoch's Verkle tree (mutable)
/// * `epoch_start_block` - Absolute block number of epoch start
/// * `block_number` - Current absolute block number
/// * `logs` - All logs from this block (grouped by transaction)
/// * `da_chunk_id` - DA chunk ID for this block's data
/// * `offset_fn` - Function to compute offset within DA chunk for each log
///
/// # Returns
///
/// The updated epoch index root after processing this block
pub fn process_block_for_epoch_index<T: VerkleTree>(
    tree: &mut SimpleVerkleTree,  // TODO: Make generic over T: VerkleTree
    epoch_start_block: u64,
    block_number: u64,
    logs_by_tx: &[Vec<Log>],
    da_chunk_id: u16,
    offset_fn: impl Fn(usize, usize) -> u32, // (tx_index, log_index) -> offset
) -> VerkleRoot {
    let block_in_epoch = (block_number - epoch_start_block) as u16;

    for (tx_index, tx_logs) in logs_by_tx.iter().enumerate() {
        for (log_index, log) in tx_logs.iter().enumerate() {
            let index_keys = derive_index_keys_from_log(log);
            let offset_in_chunk = offset_fn(tx_index, log_index);

            let log_id = LogId::new_unchecked(
                block_in_epoch,
                tx_index as u16,
                log_index as u16,
                da_chunk_id,
                offset_in_chunk,
            );

            // Insert into epoch index
            for key in index_keys {
                tree.insert_log_ids(key, &[log_id]);
            }
        }
    }

    tree.commit()
}

// ============================================================================
// Tests
// ============================================================================

#[cfg(test)]
mod tests {
    use super::*;
    use alloc::vec;

    #[test]
    fn test_log_id_packing() {
        let log_id = LogId::new_unchecked(100, 5, 3, 42, 1234);

        assert_eq!(log_id.block_in_epoch(), 100);
        assert_eq!(log_id.tx_index(), 5);
        assert_eq!(log_id.log_index(), 3);
        assert_eq!(log_id.da_chunk_id(), 42);
        assert_eq!(log_id.offset_in_chunk(), 1234);

        // Round-trip
        let val = log_id.as_u128();
        let log_id2 = LogId::from_u128(val);
        assert_eq!(log_id, log_id2);
    }

    #[test]
    fn test_log_id_max_values() {
        let log_id = LogId::new_unchecked(
            0xFFFF,
            0xFFFF,
            0xFFFF,
            0xFFFF,
            0xFFFFFFFF,
        );

        assert_eq!(log_id.block_in_epoch(), 0xFFFF);
        assert_eq!(log_id.tx_index(), 0xFFFF);
        assert_eq!(log_id.log_index(), 0xFFFF);
        assert_eq!(log_id.da_chunk_id(), 0xFFFF);
        assert_eq!(log_id.offset_in_chunk(), 0xFFFFFFFF);
    }

    #[test]
    fn test_simple_verkle_tree() {
        let mut tree = SimpleVerkleTree::new();

        let key1 = [1u8; 32];
        let key2 = [2u8; 32];

        let log_id1 = LogId::new_unchecked(0, 0, 0, 0, 100);
        let log_id2 = LogId::new_unchecked(0, 0, 1, 0, 200);
        let log_id3 = LogId::new_unchecked(1, 0, 0, 0, 300);

        // Insert logs
        tree.insert_log_ids(key1, &[log_id1, log_id2]);
        tree.insert_log_ids(key2, &[log_id3]);

        // Verify retrieval
        let value1 = tree.get(&key1).unwrap();
        assert_eq!(value1.total_count, 2);

        let value2 = tree.get(&key2).unwrap();
        assert_eq!(value2.total_count, 1);

        // Verify commitment
        let root = tree.commit();
        assert_ne!(root, [0u8; 32]);
    }

    #[test]
    fn test_derive_index_keys() {
        let mut topic0_bytes = [0u8; 32];
        topic0_bytes[31] = 0xAA;
        let mut topic1_bytes = [0u8; 32];
        topic1_bytes[31] = 0xBB;

        let log = Log {
            address: H160::zero(),
            topics: vec![
                H256(topic0_bytes), // topic0
                H256(topic1_bytes), // topic1
            ],
            data: vec![],
        };

        let keys = derive_index_keys_from_log(&log);

        // Should have: contract key, topic0 key, topic1 key
        assert_eq!(keys.len(), 3);
    }

    #[test]
    fn test_deterministic_commit() {
        let mut tree1 = SimpleVerkleTree::new();
        let mut tree2 = SimpleVerkleTree::new();

        let key1 = [1u8; 32];
        let log_id1 = LogId::new_unchecked(0, 0, 0, 0, 100);

        tree1.insert_log_ids(key1, &[log_id1]);
        tree2.insert_log_ids(key1, &[log_id1]);

        // Same inputs should produce same root
        assert_eq!(tree1.commit(), tree2.commit());
    }
}
