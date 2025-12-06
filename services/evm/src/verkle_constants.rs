//! Verkle tree layout constants (EIP-6800)
//!
//! These constants define the Verkle tree key structure for EVM state.
//! They are shared between Rust (witness tracking) and Go (state access).
//!
//! Reference: statedb/evmtypes/verkle.go:90-100

use primitive_types::{H160, U256};

// ===== JAM Precompile Addresses =====

/// USDM token contract (0x0000000000000000000000000000000000000001)
pub const USDM_ADDRESS: H160 = H160([0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,1]);

/// Governor contract (0x0000000000000000000000000000000000000002)
pub const GOVERNOR_ADDRESS: H160 = H160([0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,2]);

/// Math precompile (0x0000000000000000000000000000000000000003)
pub const MATH_PRECOMPILE_ADDRESS: H160 = H160([0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,3]);

// ===== Tree Layout Constants =====

/// Verkle tree branch width (number of children per node)
pub const VERKLE_NODE_WIDTH: usize = 256;

/// Storage slot offset for slots 0-63 (HEADER_STORAGE range)
pub const HEADER_STORAGE_OFFSET: usize = 64;

/// Code chunk offset (code starts at position 128)
pub const CODE_OFFSET: usize = 128;

/// Storage slot offset for slots 64+ (MAIN_STORAGE range)
/// Equal to 256^31 per EIP-6800
pub const MAIN_STORAGE_OFFSET: U256 = U256([0, 0, 0, 0x100000000000000]); // 256^31 = 2^248

// ===== Account BASIC_DATA Layout =====

/// Leaf key for BASIC_DATA (version, nonce, balance, code size)
pub const BASIC_DATA_LEAF_KEY: u8 = 0;

/// Leaf key for code hash
pub const CODE_HASH_LEAF_KEY: u8 = 1;

/// Offset of version field in BASIC_DATA (1 byte)
/// NOTE: Reserved for future BasicData parsing in Rust
#[allow(dead_code)]
pub const BASIC_DATA_VERSION_OFFSET: usize = 0;

/// Offset of code size field in BASIC_DATA (3 bytes starting at offset 5)
/// NOTE: Reserved for future BasicData parsing in Rust
#[allow(dead_code)]
pub const BASIC_DATA_CODE_SIZE_OFFSET: usize = 5;

/// Offset of nonce field in BASIC_DATA (8 bytes starting at offset 8)
/// NOTE: Reserved for future BasicData parsing in Rust
#[allow(dead_code)]
pub const BASIC_DATA_NONCE_OFFSET: usize = 8;

/// Offset of balance field in BASIC_DATA (16 bytes starting at offset 16)
/// NOTE: Reserved for future BasicData parsing in Rust
#[allow(dead_code)]
pub const BASIC_DATA_BALANCE_OFFSET: usize = 16;

// ===== EIP-4762 Gas Costs =====

/// Cost to access a branch (first time per transaction)
pub const WITNESS_BRANCH_COST: u64 = 1900;

/// Cost to access a chunk/leaf (first time per transaction)
pub const WITNESS_CHUNK_COST: u64 = 200;

/// Cost to edit a subtree (first time per transaction)
pub const SUBTREE_EDIT_COST: u64 = 3000;

/// Cost to edit a chunk/leaf (first time per transaction)
pub const CHUNK_EDIT_COST: u64 = 500;

/// Cost to fill a chunk (write to previously absent key)
pub const CHUNK_FILL_COST: u64 = 6200;
