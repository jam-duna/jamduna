// JAM Orchard Service State
//
// Defines the minimal consensus-critical state maintained by the JAM Orchard service.
// Builders maintain full off-chain state and provide Merkle witnesses.

use crate::merkle_impl::{verify_commitment_proof, SparseMerkleTree, sparse_empty_leaf};
use crate::{Error, Result};
use incrementalmerkletree::{Hashable, Level};
use orchard::tree::MerkleHashOrchard;
use serde::{Deserialize, Serialize};

/// JAM Orchard service state (consensus-critical, on-chain)
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct OrchardState {
    /// Orchard commitment tree root (Sinsemilla Merkle root, depth 32)
    pub commitment_root: [u8; 32],

    /// Tree size for deterministic appends
    pub commitment_size: u64,

    /// Nullifier set root (builder-maintained tree, only root on-chain)
    pub nullifier_root: [u8; 32],

    /// Minimum gas limit
    pub gas_min: u64,

    /// Maximum gas limit
    pub gas_max: u64,
}

impl Default for OrchardState {
    fn default() -> Self {
        let commitment_root =
            MerkleHashOrchard::empty_root(Level::from(ORCHARD_MERKLE_DEPTH as u8)).to_bytes();
        let mut nullifier_tree = SparseMerkleTree::new(ORCHARD_MERKLE_DEPTH);
        let nullifier_root = nullifier_tree.root();
        Self {
            commitment_root,
            commitment_size: 0,
            nullifier_root,
            gas_min: 21000,
            gas_max: 10_000_000,
        }
    }
}

const ORCHARD_MERKLE_DEPTH: usize = 32;

/// Merkle proof for a single leaf.
pub type MerkleProof = crate::merkle_impl::MerkleProof;

/// Pre-execution state witnesses (builder provides these)
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct StateWitnesses {
    /// Commitment tree root and size witnesses
    pub commitment_root_proof: MerkleProof,
    pub commitment_size: u64,

    /// Nullifier non-membership proofs (one per spent note)
    pub nullifier_absence_proofs: Vec<MerkleProof>,

    /// Gas policy witnesses (gas_min, gas_max from state)
    pub gas_min: u64,
    pub gas_max: u64,
}

/// Write intents computed during refine phase
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct WriteIntents {
    /// New commitments to append to commitment tree
    pub new_commitments: Vec<[u8; 32]>,

    /// Nullifiers to mark as spent
    pub new_nullifiers: Vec<[u8; 32]>,

    /// Expected new roots
    pub expected_commitment_root: [u8; 32],
    pub expected_nullifier_root: [u8; 32],
}

/// Post-execution state updates
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct StateUpdates {
    pub new_commitment_root: [u8; 32],
    pub new_commitment_size: u64,
    pub new_nullifier_root: [u8; 32],
}

impl StateUpdates {
    pub fn success() -> Self {
        Self {
            new_commitment_root: [0u8; 32],
            new_commitment_size: 0,
            new_nullifier_root: [0u8; 32],
        }
    }
}

/// Verify state witnesses against pre-state
pub fn verify_pre_witnesses(
    state: &OrchardState,
    witnesses: &StateWitnesses,
) -> Result<()> {
    // Verify commitment root witness
    if witnesses.commitment_root_proof.root != state.commitment_root {
        return Err(Error::InvalidWitness(
            "Commitment root mismatch".to_string()
        ));
    }
    if !verify_commitment_proof(&witnesses.commitment_root_proof, state.commitment_root) {
        return Err(Error::InvalidWitness(
            "Invalid commitment root proof".to_string()
        ));
    }

    if witnesses.commitment_size != state.commitment_size {
        return Err(Error::InvalidWitness(
            "Commitment size mismatch".to_string()
        ));
    }

    // Verify nullifier absence proofs
    for proof in &witnesses.nullifier_absence_proofs {
        if proof.root != state.nullifier_root {
            return Err(Error::InvalidWitness(
                "Nullifier root mismatch".to_string()
            ));
        }
        if proof.leaf != sparse_empty_leaf() || !proof.verify() {
            return Err(Error::InvalidWitness(
                "Invalid nullifier absence proof".to_string()
            ));
        }
    }

    // Verify gas policy
    if witnesses.gas_min != state.gas_min
        || witnesses.gas_max != state.gas_max {
        return Err(Error::InvalidWitness(
            "Gas policy mismatch".to_string()
        ));
    }

    Ok(())
}

/// Compute new state roots from write intents
pub fn compute_post_state(
    state: &OrchardState,
    intents: &WriteIntents,
) -> Result<StateUpdates> {
    // Compute new commitment root by appending new commitments
    let mut new_root = state.commitment_root;
    let mut new_size = state.commitment_size;

    for commitment in &intents.new_commitments {
        new_root = append_to_tree(new_root, new_size, *commitment);
        new_size += 1;
    }

    // Compute new nullifier root by inserting new nullifiers
    let mut nullifier_root = state.nullifier_root;
    for nullifier in &intents.new_nullifiers {
        nullifier_root = insert_nullifier(nullifier_root, *nullifier);
    }

    // Verify computed roots match expected
    if new_root != intents.expected_commitment_root {
        return Err(Error::InvalidState(
            "Commitment root mismatch".to_string()
        ));
    }
    if nullifier_root != intents.expected_nullifier_root {
        return Err(Error::InvalidState(
            "Nullifier root mismatch".to_string()
        ));
    }
    Ok(StateUpdates {
        new_commitment_root: new_root,
        new_commitment_size: new_size,
        new_nullifier_root: nullifier_root,
    })
}

// Placeholder hash functions (to be replaced with actual Sinsemilla/Poseidon)
fn sinsemilla_hash(left: &[u8; 32], right: &[u8; 32]) -> [u8; 32] {
    use blake2b_simd::Params;
    let mut hash = [0u8; 32];
    hash.copy_from_slice(
        Params::new()
            .hash_length(32)
            .to_state()
            .update(left)
            .update(right)
            .finalize()
            .as_bytes()
    );
    hash
}

fn append_to_tree(root: [u8; 32], _size: u64, leaf: [u8; 32]) -> [u8; 32] {
    // Simplified append - real implementation uses incremental Merkle tree
    sinsemilla_hash(&root, &leaf)
}

fn insert_nullifier(root: [u8; 32], nullifier: [u8; 32]) -> [u8; 32] {
    // Simplified insert - real implementation uses sparse Merkle tree
    sinsemilla_hash(&root, &nullifier)
}
