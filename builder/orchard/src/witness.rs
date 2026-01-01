// Witness Generation for Orchard Work Packages
//
// Builders maintain full off-chain state and generate Merkle witnesses
// for all state accesses required by the JAM service.

use crate::merkle_impl::{
    verify_commitment_proof, IncrementalMerkleTree, MerkleProof, SparseMerkleTree, sparse_empty_leaf,
};
use crate::state::{OrchardState, StateWitnesses, WriteIntents};
use crate::{Error, Result, OrchardBundle};
use incrementalmerkletree::{Hashable, Level};
use orchard::tree::MerkleHashOrchard;

/// Builder's off-chain state (full data, not just roots)
#[derive(Clone)]
pub struct BuilderState {
    /// Full commitment tree (incremental Merkle tree)
    pub commitment_tree: IncrementalMerkleTree,

    /// Full nullifier set (sparse Merkle tree)
    pub nullifier_set: SparseMerkleTree,

    /// Current on-chain state snapshot
    pub chain_state: OrchardState,
}

impl BuilderState {
    pub fn new(chain_state: OrchardState) -> Self {
        let commitment_tree = IncrementalMerkleTree::with_root(
            chain_state.commitment_root,
            chain_state.commitment_size as usize,
            ORCHARD_MERKLE_DEPTH,
        );
        let nullifier_set = SparseMerkleTree::with_root(
            chain_state.nullifier_root,
            ORCHARD_MERKLE_DEPTH,
        );
        Self {
            commitment_tree,
            nullifier_set,
            chain_state,
        }
    }

    /// Sync builder state with latest on-chain state
    pub fn sync(&mut self, new_state: OrchardState) -> Result<()> {
        // Verify builder's computed roots match on-chain roots
        if self.commitment_tree.root() != new_state.commitment_root {
            return Err(Error::InvalidState("Commitment tree desync".to_string()));
        }
        if self.nullifier_set.root() != new_state.nullifier_root {
            return Err(Error::InvalidState("Nullifier set desync".to_string()));
        }

        self.chain_state = new_state;
        Ok(())
    }
}

/// Build state witnesses for a bundle before execution
pub fn build_witnesses(
    builder_state: &BuilderState,
    bundle: &OrchardBundle,
) -> Result<StateWitnesses> {
    // Extract nullifiers from bundle
    let nullifiers = extract_nullifiers(bundle)?;

    // Build commitment tree witness
    let commitment_root_proof = build_commitment_root_proof(&builder_state.commitment_tree);

    // Build nullifier absence proofs
    let mut nullifier_absence_proofs = Vec::new();
    for nullifier in &nullifiers {
        let proof = builder_state.nullifier_set.prove_absence(*nullifier);
        nullifier_absence_proofs.push(proof);
    }

    Ok(StateWitnesses {
        commitment_root_proof,
        commitment_size: builder_state.chain_state.commitment_size,
        nullifier_absence_proofs,
        gas_min: builder_state.chain_state.gas_min,
        gas_max: builder_state.chain_state.gas_max,
    })
}

/// Verify witnesses match bundle and state
pub fn verify_witnesses(
    state: &OrchardState,
    witnesses: &StateWitnesses,
    bundle: &OrchardBundle,
) -> Result<()> {
    // Verify commitment root witness
    if !verify_commitment_proof(&witnesses.commitment_root_proof, state.commitment_root) {
        return Err(Error::InvalidWitness("Invalid commitment root proof".to_string()));
    }
    if witnesses.commitment_root_proof.root != state.commitment_root {
        return Err(Error::InvalidWitness("Commitment root mismatch".to_string()));
    }

    // Verify nullifier count matches bundle actions
    let action_count = bundle.actions().len();
    if witnesses.nullifier_absence_proofs.len() != action_count {
        return Err(Error::InvalidWitness("Nullifier count mismatch".to_string()));
    }

    // Verify each nullifier absence proof
    for proof in &witnesses.nullifier_absence_proofs {
        if proof.root != state.nullifier_root {
            return Err(Error::InvalidWitness("Nullifier root mismatch".to_string()));
        }
        if proof.leaf != sparse_empty_leaf() || !proof.verify() {
            return Err(Error::InvalidWitness("Invalid nullifier absence proof".to_string()));
        }
    }

    // Verify gas policy
    if witnesses.gas_min != state.gas_min
        || witnesses.gas_max != state.gas_max {
        return Err(Error::InvalidWitness("Gas policy mismatch".to_string()));
    }

    Ok(())
}

/// Compute write intents from bundle execution
pub fn compute_write_intents(
    builder_state: &mut BuilderState,
    bundle: &OrchardBundle,
) -> Result<WriteIntents> {
    // Extract new commitments and nullifiers from bundle
    let new_commitments = extract_commitments(bundle)?;
    let new_nullifiers = extract_nullifiers(bundle)?;

    // Simulate state updates to compute expected roots
    let mut simulated_tree = builder_state.commitment_tree.clone();
    for commitment in &new_commitments {
        simulated_tree.append(*commitment);
    }

    let mut simulated_nullifiers = builder_state.nullifier_set.clone();
    for nullifier in &new_nullifiers {
        simulated_nullifiers.insert(*nullifier);
    }

    Ok(WriteIntents {
        new_commitments,
        new_nullifiers,
        expected_commitment_root: simulated_tree.root(),
        expected_nullifier_root: simulated_nullifiers.root(),
    })
}

/// Apply write intents to a builder state (used for sequential batch building).
pub fn apply_write_intents(builder_state: &mut BuilderState, intents: &WriteIntents) -> Result<()> {
    for commitment in &intents.new_commitments {
        builder_state.commitment_tree.append(*commitment);
    }
    for nullifier in &intents.new_nullifiers {
        builder_state.nullifier_set.insert(*nullifier);
    }

    builder_state.chain_state.commitment_size = builder_state
        .chain_state
        .commitment_size
        .saturating_add(intents.new_commitments.len() as u64);

    let commitment_root = builder_state.commitment_tree.root();
    let nullifier_root = builder_state.nullifier_set.root();

    if commitment_root != intents.expected_commitment_root {
        return Err(Error::InvalidState("Commitment root mismatch".to_string()));
    }
    if nullifier_root != intents.expected_nullifier_root {
        return Err(Error::InvalidState("Nullifier root mismatch".to_string()));
    }

    builder_state.chain_state.commitment_root = commitment_root;
    builder_state.chain_state.nullifier_root = nullifier_root;

    Ok(())
}

// Helper functions

fn extract_nullifiers(bundle: &OrchardBundle) -> Result<Vec<[u8; 32]>> {
    let mut nullifiers = Vec::new();
    for action in bundle.actions() {
        let nf_bytes = action.nullifier().to_bytes();
        nullifiers.push(nf_bytes);
    }
    Ok(nullifiers)
}

fn extract_commitments(bundle: &OrchardBundle) -> Result<Vec<[u8; 32]>> {
    let mut commitments = Vec::new();
    for action in bundle.actions() {
        let cmx_bytes = action.cmx().to_bytes();
        commitments.push(cmx_bytes);
    }
    Ok(commitments)
}

const ORCHARD_MERKLE_DEPTH: usize = 32;

fn build_commitment_root_proof(tree: &IncrementalMerkleTree) -> MerkleProof {
    if tree.size() == 0 {
        return empty_commitment_proof();
    }

    tree.prove(0).unwrap_or_else(empty_commitment_proof)
}

fn empty_commitment_proof() -> MerkleProof {
    let leaf = MerkleHashOrchard::empty_leaf().to_bytes();
    let siblings = (0..ORCHARD_MERKLE_DEPTH)
        .map(|level| MerkleHashOrchard::empty_root(Level::from(level as u8)).to_bytes())
        .collect();
    let root = MerkleHashOrchard::empty_root(Level::from(ORCHARD_MERKLE_DEPTH as u8)).to_bytes();

    MerkleProof {
        leaf,
        siblings,
        position: 0,
        root,
    }
}
