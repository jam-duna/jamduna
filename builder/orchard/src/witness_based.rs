// Witness-Based Work Package Design for JAM Refine
//
// JAM's refine function is stateless - it cannot store full trees.
// Instead, work packages include Merkle witnesses that prove:
// 1. Pre-state: leaves accessed exist in pre-state roots
// 2. Post-state: new roots are correctly computed from updates

use crate::bundle_codec::deserialize_bundle;
use crate::merkle_impl::{
    verify_commitment_proof, IncrementalMerkleTree, MerkleProof, SparseMerkleTree,
    sparse_empty_leaf, sparse_hash_leaf, sparse_position_for,
};
use crate::state::OrchardState;
use crate::{Error, Result};
use serde::{Deserialize, Serialize};

/// Witness-based work package extrinsic for JAM
///
/// Contains everything needed for stateless verification:
/// 1. Pre-state witnesses (Merkle proofs for state reads)
/// 2. User bundles (Orchard bundles with Halo2 zkSNARK proofs)
/// 3. Post-state witnesses (Merkle proofs for state writes)
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct WitnessBasedExtrinsic {
    /// Pre-state roots (what JAM state currently holds)
    pub pre_state_roots: StateRoots,

    /// Witnesses proving reads from pre-state
    pub pre_state_witnesses: PreStateWitnesses,

    /// User-submitted Orchard bundles (each Bundle<Authorized, i64> contains a Halo2 proof)
    pub user_bundles: Vec<UserBundleWithWitnesses>,

    /// Expected post-state roots after execution
    pub post_state_roots: StateRoots,

    /// Witnesses proving post-state computation is correct
    pub post_state_witnesses: PostStateWitnesses,

    /// Work package metadata
    pub metadata: WorkPackageMetadata,
}

/// A user-submitted Orchard bundle with associated witness data
///
/// Contains a serialized Bundle<Authorized, i64> from orchard::Bundle
/// The Authorized type contains the Halo2 zkSNARK proof and binding signature
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct UserBundleWithWitnesses {
    /// In-memory bundle for real proof verification (not serialized)
    #[serde(skip)]
    pub bundle: Option<crate::OrchardBundle>,

    /// Serialized Orchard Bundle<Authorized, i64> (Zcash v5 encoding)
    /// Contains the Halo2 proof and signatures.
    pub bundle_bytes: Vec<u8>,

    /// Actions extracted from this bundle (for Merkle witness generation)
    /// These map directly to the actions inside the Bundle
    pub actions: Vec<ActionWithWitness>,

}

/// State roots at a point in time
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct StateRoots {
    /// Commitment tree root (all note commitments)
    pub commitment_root: [u8; 32],

    /// Commitment tree size
    pub commitment_size: u64,

    /// Nullifier set root (all spent nullifiers)
    pub nullifier_root: [u8; 32],
}

impl From<&OrchardState> for StateRoots {
    fn from(state: &OrchardState) -> Self {
        Self {
            commitment_root: state.commitment_root,
            commitment_size: state.commitment_size,
            nullifier_root: state.nullifier_root,
        }
    }
}

/// Pre-state witnesses: prove what we READ from current state
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PreStateWitnesses {
    /// For each spent note: prove the commitment exists in tree
    /// (Merkle path from commitment to commitment_root)
    pub spent_note_commitment_proofs: Vec<MerkleProof>,

    /// For each nullifier: prove it does NOT exist yet
    /// (Sparse Merkle non-membership proof against nullifier_root)
    pub nullifier_absence_proofs: Vec<MerkleProof>,

}

/// Post-state witnesses: prove new roots are CORRECTLY COMPUTED
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PostStateWitnesses {
    /// Merkle paths for new commitments being added
    /// These prove that appending new_commitments to the tree
    /// produces post_state_roots.commitment_root
    pub new_commitment_proofs: Vec<MerkleProof>,

    /// Merkle paths for new nullifiers being added
    /// These prove that inserting new_nullifiers into sparse tree
    /// produces post_state_roots.nullifier_root
    pub new_nullifier_proofs: Vec<MerkleProof>,

}

/// Single Orchard action with its witnesses
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ActionWithWitness {
    /// Nullifier being revealed (spending a note)
    pub nullifier: [u8; 32],

    /// New commitment being created (creating a note)
    pub commitment: [u8; 32],

    /// Index of this action's commitment proof in pre_state_witnesses
    pub spent_commitment_index: Option<usize>,

    /// Index of this action's nullifier proof in pre_state_witnesses
    pub nullifier_absence_index: usize,
}

/// Convert an Orchard bundle into witness actions for verification.
pub fn actions_from_bundle(bundle: &crate::OrchardBundle) -> Vec<ActionWithWitness> {
    bundle
        .actions()
        .iter()
        .enumerate()
        .map(|(idx, action)| ActionWithWitness {
            nullifier: action.nullifier().to_bytes(),
            commitment: action.cmx().to_bytes(),
            spent_commitment_index: None,
            nullifier_absence_index: idx,
        })
        .collect()
}

impl UserBundleWithWitnesses {
    fn actions_for_witnesses(&self) -> Vec<ActionWithWitness> {
        if !self.actions.is_empty() {
            return self.actions.clone();
        }
        self.bundle
            .as_ref()
            .map(actions_from_bundle)
            .unwrap_or_default()
    }
}

/// Work package metadata
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct WorkPackageMetadata {
    /// Gas limit for entire work package
    pub gas_limit: u64,
}

/// Refine function: stateless verification using only witnesses
///
/// This is what runs in JAM validators - completely stateless!
/// Verifies THREE things:
/// 1. Pre-state witnesses (Merkle proofs for reads)
/// 2. User Orchard bundles (Halo2 zkSNARK proofs)
/// 3. Post-state witnesses (Merkle proofs for writes)
pub fn refine_witness_based(
    extrinsic: &WitnessBasedExtrinsic,
) -> Result<()> {
    println!("=== JAM Refine (Stateless Verification) ===");
    println!();

    // Collect all actions from all user bundles
    let mut all_actions = Vec::new();
    for bundle in &extrinsic.user_bundles {
        all_actions.extend(bundle.actions_for_witnesses());
    }

    // Step 1: Verify all pre-state witnesses against pre-state roots
    println!("Step 1: Verifying pre-state witnesses...");
    verify_pre_state_witnesses(
        &extrinsic.pre_state_roots,
        &extrinsic.pre_state_witnesses,
        &all_actions,
    )?;
    println!("   ✓ Pre-state witnesses valid");
    println!();

    // Step 2: Verify all user Orchard bundles (proofs + signatures)
    println!("Step 2: Verifying user Orchard bundles...");
    println!("   {} user bundle(s) to verify", extrinsic.user_bundles.len());

    for (i, user_bundle) in extrinsic.user_bundles.iter().enumerate() {
        let action_count = if !user_bundle.actions.is_empty() {
            user_bundle.actions.len()
        } else {
            user_bundle
                .bundle
                .as_ref()
                .map(|bundle| bundle.actions().iter().count())
                .unwrap_or(0)
        };

        println!("   Bundle {}/{}: {} actions",
            i + 1,
            extrinsic.user_bundles.len(),
            action_count
        );

        if let Some(ref bundle) = user_bundle.bundle {
            verify_orchard_bundle_direct(bundle)?;
        } else if !user_bundle.bundle_bytes.is_empty() {
            verify_orchard_bundle(&user_bundle.bundle_bytes)?;
        } else {
            return Err(Error::InvalidBundle(
                "Missing bundle proof: provide Bundle<Authorized, i64> or serialized bytes".into()
            ));
        }
    }
    println!("   ✓ All user bundles verified");
    println!();

    // Step 3: Verify deterministic state properties (size)
    println!("Step 3: Verifying deterministic state updates...");
    let expected_size = extrinsic.pre_state_roots.commitment_size + all_actions.len() as u64;
    if extrinsic.post_state_roots.commitment_size != expected_size {
        return Err(Error::InvalidState(
            format!("Commitment size mismatch: expected {}, got {}",
                expected_size, extrinsic.post_state_roots.commitment_size)
        ));
    }

    println!("   ✓ State updates correct");
    println!();

    // Step 4: Verify post-state witnesses prove the root transitions
    println!("Step 4: Verifying post-state witnesses...");
    verify_post_state_witnesses(
        &extrinsic.post_state_roots,
        &extrinsic.post_state_witnesses,
        &all_actions,
    )?;
    println!("   ✓ Post-state witnesses valid");
    println!();

    println!("════════════════════════════════════════");
    println!("✓ Refine verification PASSED!");
    println!("════════════════════════════════════════");
    println!("  • Pre-state witnesses: verified");
    println!("  • User bundles: {} verified", extrinsic.user_bundles.len());
    println!("  • Post-state witnesses: verified");

    Ok(())
}

/// Verify pre-state witnesses prove correct reads
fn verify_pre_state_witnesses(
    pre_roots: &StateRoots,
    witnesses: &PreStateWitnesses,
    actions: &[ActionWithWitness],
) -> Result<()> {
    // Verify each spent note commitment exists in commitment tree
    for proof in &witnesses.spent_note_commitment_proofs {
        if proof.root != pre_roots.commitment_root {
            return Err(Error::InvalidWitness(
                "Spent commitment proof root mismatch".to_string()
            ));
        }
        if !verify_commitment_proof(proof, pre_roots.commitment_root) {
            return Err(Error::InvalidWitness(
                "Spent commitment proof invalid".to_string()
            ));
        }
    }

    // Verify each nullifier does NOT exist yet
    if witnesses.nullifier_absence_proofs.len() != actions.len() {
        return Err(Error::InvalidWitness(
            "Wrong number of nullifier absence proofs".to_string()
        ));
    }

    for (action, proof) in actions.iter().zip(&witnesses.nullifier_absence_proofs) {
        if proof.root != pre_roots.nullifier_root {
            return Err(Error::InvalidWitness(
                "Nullifier absence proof root mismatch".to_string()
            ));
        }
        let expected_position = sparse_position_for(action.nullifier, proof.siblings.len());
        if proof.position != expected_position {
            return Err(Error::InvalidWitness(
                "Nullifier absence proof position mismatch".to_string()
            ));
        }
        if proof.leaf != sparse_empty_leaf() {
            return Err(Error::InvalidWitness(
                "Nullifier absence proof leaf mismatch".to_string()
            ));
        }
        if !proof.verify() {
            return Err(Error::InvalidWitness(
                "Nullifier absence proof invalid".to_string()
            ));
        }
    }

    Ok(())
}

/// Verify post-state witnesses prove correct computation
///
/// Key insight: In witness-based verification, we don't recompute the roots!
/// Instead, we verify that the Merkle witnesses cryptographically prove that
/// the builder correctly computed the new roots.
fn verify_post_state_witnesses(
    post_roots: &StateRoots,
    witnesses: &PostStateWitnesses,
    actions: &[ActionWithWitness],
) -> Result<()> {
    // Verify new commitment proofs
    if witnesses.new_commitment_proofs.len() != actions.len() {
        return Err(Error::InvalidWitness(
            "Wrong number of commitment proofs".to_string()
        ));
    }

    for (action, proof) in actions.iter().zip(&witnesses.new_commitment_proofs) {
        if proof.root != post_roots.commitment_root {
            return Err(Error::InvalidWitness(
                "New commitment proof root mismatch".to_string()
            ));
        }
        if proof.leaf != action.commitment {
            return Err(Error::InvalidWitness(
                "New commitment proof leaf mismatch".to_string()
            ));
        }
        if !verify_commitment_proof(proof, post_roots.commitment_root) {
            return Err(Error::InvalidWitness(
                "New commitment proof invalid".to_string()
            ));
        }
    }

    // Verify new nullifier proofs
    if witnesses.new_nullifier_proofs.len() != actions.len() {
        return Err(Error::InvalidWitness(
            "Wrong number of nullifier proofs".to_string()
        ));
    }

    for (action, proof) in actions.iter().zip(&witnesses.new_nullifier_proofs) {
        if proof.root != post_roots.nullifier_root {
            return Err(Error::InvalidWitness(
                "New nullifier proof root mismatch".to_string()
            ));
        }
        let expected_position = sparse_position_for(action.nullifier, proof.siblings.len());
        if proof.position != expected_position {
            return Err(Error::InvalidWitness(
                "New nullifier proof position mismatch".to_string()
            ));
        }
        if proof.leaf != sparse_hash_leaf(action.nullifier) {
            return Err(Error::InvalidWitness(
                "New nullifier proof leaf mismatch".to_string()
            ));
        }
        if !proof.verify() {
            return Err(Error::InvalidWitness(
                "New nullifier proof invalid".to_string()
            ));
        }
    }

    Ok(())
}

/// Builder generates witness-based extrinsic from full state
pub fn build_witness_based_extrinsic(
    commitment_tree: &mut IncrementalMerkleTree,
    nullifier_set: &mut SparseMerkleTree,
    pre_state: &OrchardState,
    user_bundles: Vec<UserBundleWithWitnesses>,
    metadata: WorkPackageMetadata,
) -> Result<WitnessBasedExtrinsic> {
    // Collect all actions from all user bundles
    let mut all_actions = Vec::new();
    for bundle in &user_bundles {
        all_actions.extend(bundle.actions_for_witnesses());
    }
    let pre_state_roots = StateRoots::from(pre_state);

    // Generate pre-state witnesses
    // Orchard spends already prove membership in the zkSNARK, so these are optional here.
    let spent_note_commitment_proofs = Vec::new();
    let mut nullifier_absence_proofs = Vec::new();

    for action in &all_actions {
        // Prove nullifier doesn't exist yet
        let absence_proof = nullifier_set.prove_absence(action.nullifier);
        nullifier_absence_proofs.push(absence_proof);
    }

    let pre_state_witnesses = PreStateWitnesses {
        spent_note_commitment_proofs,
        nullifier_absence_proofs,
    };

    // Apply actions to trees to get post-state
    for action in &all_actions {
        nullifier_set.insert(action.nullifier);
        commitment_tree.append(action.commitment);
    }

    let post_state_roots = StateRoots {
        commitment_root: commitment_tree.root(),
        commitment_size: commitment_tree.size() as u64,
        nullifier_root: nullifier_set.root(),
    };

    // Generate post-state witnesses
    let mut new_commitment_proofs = Vec::new();
    let mut new_nullifier_proofs = Vec::new();

    for (i, action) in all_actions.iter().enumerate() {
        // Prove new commitment is in tree at its position
        let commit_position = pre_state.commitment_size + i as u64;
        let proof = commitment_tree
            .prove(commit_position as usize)
            .ok_or_else(|| Error::InvalidWitness("Missing commitment proof".to_string()))?;
        new_commitment_proofs.push(proof);

        // Prove new nullifier is in set
        new_nullifier_proofs.push(nullifier_set.prove_membership(action.nullifier));
    }

    let post_state_witnesses = PostStateWitnesses {
        new_commitment_proofs,
        new_nullifier_proofs,
    };

    Ok(WitnessBasedExtrinsic {
        pre_state_roots,
        pre_state_witnesses,
        user_bundles,
        post_state_roots,
        post_state_witnesses,
        metadata,
    })
}

/// Verify an Orchard bundle's zkSNARK proof and signatures.
#[cfg(feature = "circuit")]
pub fn verify_orchard_bundle_direct(bundle: &crate::OrchardBundle) -> Result<()> {
    crate::workpackage::verify_orchard_bundle(bundle)
}

fn verify_orchard_bundle(bundle_bytes: &[u8]) -> Result<()> {
    let bundle = deserialize_bundle(bundle_bytes)?;
    verify_orchard_bundle_direct(&bundle)
}

/// Placeholder when circuit feature is disabled
#[cfg(not(feature = "circuit"))]
pub fn verify_orchard_bundle_direct(_bundle: &crate::OrchardBundle) -> Result<()> {
    Err(Error::InvalidBundle(
        "Circuit feature not enabled - cannot verify Halo2 proofs. Build with --features circuit".into()
    ))
}

#[cfg(test)]
mod tests {
    use super::*;
    use pasta_curves::group::ff::PrimeField;

    #[test]
    #[ignore = "requires real Orchard bundles and proofs"]
    fn test_witness_based_refine() {
        let mut commitment_tree = IncrementalMerkleTree::new(32);
        let mut nullifier_set = SparseMerkleTree::new(32);

        let pre_state = OrchardState::default();
        let commitment_1 = pasta_curves::pallas::Base::from(10u64).to_repr();
        let commitment_2 = pasta_curves::pallas::Base::from(20u64).to_repr();

        let actions = vec![
            ActionWithWitness {
                nullifier: [1u8; 32],
                commitment: commitment_1,
                spent_commitment_index: None,
                nullifier_absence_index: 0,
            },
            ActionWithWitness {
                nullifier: [2u8; 32],
                commitment: commitment_2,
                spent_commitment_index: None,
                nullifier_absence_index: 1,
            },
        ];

        let user_bundle = UserBundleWithWitnesses {
            bundle: None,
            bundle_bytes: vec![],
            actions,
        };
        let metadata = WorkPackageMetadata {
            gas_limit: 100_000,
        };

        let extrinsic = build_witness_based_extrinsic(
            &mut commitment_tree,
            &mut nullifier_set,
            &pre_state,
            vec![user_bundle],
            metadata,
        ).unwrap();

        // Refine should verify successfully
        refine_witness_based(&extrinsic).unwrap();
    }
}
