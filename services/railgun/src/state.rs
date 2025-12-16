/// Railgun service state structure (RAILGUN.md lines 22-45)
///
/// This struct defines the consensus-critical state for the Railgun privacy service.
/// All fields are stored in the JAM state tree with Verkle proofs.

/// Shielded note set commitment tree
pub struct CommitmentTree {
    pub root: [u8; 32],        // Merkle root of note commitments
    pub size: u64,             // Current append index / size (consensus-critical)
}

/// Double-spend prevention nullifier set
pub struct NullifierSet {
    // Stored as Verkle tree: nullifier_hash -> [0u8; 32] (unspent) | [1u8; 32] (spent)
}

/// Fee accounting (epoch-based)
pub struct FeeTally {
    // Map: epoch_id -> accumulated fees (u128)
    // Distributed to guaranteeing validators at epoch start
}

/// Anti-spam parameters (consensus-critical)
pub struct AntiSpamParams {
    pub min_fee: u64,          // Minimum fee for SubmitPrivate and WithdrawPublic
}

/// Gas bounds for cross-service transfers (consensus-critical)
pub struct GasBounds {
    pub gas_min: u64,          // Lower bound (initially 50_000)
    pub gas_max: u64,          // Upper bound (initially 5_000_000)
}

/// Complete Railgun service state
pub struct RailgunState {
    pub commitment_tree: CommitmentTree,
    pub min_fee: u64,
    pub gas_min: u64,
    pub gas_max: u64,
}

impl RailgunState {
    pub const TREE_CAPACITY: u64 = 1u64 << 32; // 2^32 max leaves (depth 32 Merkle tree)
    pub const DEFAULT_MIN_FEE: u64 = 1000;
    pub const DEFAULT_GAS_MIN: u64 = 50_000;
    pub const DEFAULT_GAS_MAX: u64 = 5_000_000;

    pub fn default() -> Self {
        Self {
            commitment_tree: CommitmentTree {
                root: [0u8; 32],
                size: 0,
            },
            min_fee: Self::DEFAULT_MIN_FEE,
            gas_min: Self::DEFAULT_GAS_MIN,
            gas_max: Self::DEFAULT_GAS_MAX,
        }
    }
}

/// Write intent for state updates (output from refine)
#[derive(Clone)]
pub struct WriteIntent {
    pub object_id: [u8; 32],   // C(service_id, hash(key))
    pub data: alloc::vec::Vec<u8>, // New value to write
}

/// Extrinsic types (RAILGUN.md)
#[derive(Debug, Clone)]
pub enum RailgunExtrinsic {
    /// Deposit from cross-service transfer (triggered by EVM contract)
    DepositPublic {
        commitment: [u8; 32],
        value: u64,
        sender_index: u32,
    },

    /// Private transfer (unsigned, submitted by builder)
    SubmitPrivate {
        proof: alloc::vec::Vec<u8>,
        anchor_root: [u8; 32],
        anchor_size: u64,
        next_root: [u8; 32],
        input_nullifiers: alloc::vec::Vec<[u8; 32]>,
        output_commitments: alloc::vec::Vec<[u8; 32]>,
        fee: u64,
        encrypted_payload: alloc::vec::Vec<u8>, // MVP: must be zero-length
    },

    /// Withdraw to public (exit to EVM)
    WithdrawPublic {
        proof: alloc::vec::Vec<u8>,
        anchor_root: [u8; 32],
        anchor_size: u64,
        nullifier: [u8; 32],
        recipient: [u8; 20],     // EVM address
        amount: u64,
        fee: u64,
        gas_limit: u64,          // MUST be in proof public inputs
        has_change: bool,        // Explicit flag (not sentinel)
        change_commitment: [u8; 32],
        next_root: [u8; 32],
    },
}
