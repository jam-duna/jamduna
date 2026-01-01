// Sequential Work Package Generation
//
// Generates a sequence of work packages where each one builds on the previous state,
// creating a chain of Merkle root transitions representing Orchard state evolution.

use crate::merkle_impl::{IncrementalMerkleTree, SparseMerkleTree};
use crate::state::OrchardState;
use crate::{Error, Result};
use pasta_curves::group::ff::PrimeField;
use pasta_curves::pallas;
use serde::{Deserialize, Serialize};

/// A sequence of work packages with state transitions
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct WorkPackageSequence {
    /// Initial state before any work packages
    pub initial_state: OrchardState,

    /// Sequence of state transitions
    pub transitions: Vec<StateTransition>,
}

/// A single state transition representing one work package execution
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct StateTransition {
    /// Work package index in sequence
    pub index: usize,

    /// Pre-state (matches previous transition's post-state)
    pub pre_state: StateSnapshot,

    /// Actions in this work package
    pub actions: Vec<ActionSummary>,

    /// Post-state after applying this work package
    pub post_state: StateSnapshot,

    /// Gas used
    pub gas_used: u64,

}

/// State snapshot at a point in time
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct StateSnapshot {
    /// Commitment tree root
    pub commitment_root: [u8; 32],

    /// Commitment tree size
    pub commitment_size: u64,

    /// Nullifier set root
    pub nullifier_root: [u8; 32],

    /// Number of spent nullifiers
    pub nullifier_count: u64,

}

/// Summary of an Orchard action
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ActionSummary {
    /// Nullifier (spent note)
    pub nullifier: [u8; 32],

    /// New commitment (created note)
    pub commitment: [u8; 32],

    /// Whether this is a dummy action
    pub is_dummy: bool,
}

/// Builder for generating work package sequences
pub struct SequenceBuilder {
    /// Current commitment tree
    commitment_tree: IncrementalMerkleTree,

    /// Current nullifier set
    nullifier_set: SparseMerkleTree,

    /// Current state
    current_state: OrchardState,

    /// Transitions so far
    transitions: Vec<StateTransition>,

}

impl SequenceBuilder {
    /// Create a new sequence builder from initial state
    pub fn new(initial_state: OrchardState) -> Self {
        let commitment_tree = IncrementalMerkleTree::with_root(
            initial_state.commitment_root,
            initial_state.commitment_size as usize,
            32,
        );

        let nullifier_set = SparseMerkleTree::with_root(
            initial_state.nullifier_root,
            32,
        );

        Self {
            commitment_tree,
            nullifier_set,
            current_state: initial_state,
            transitions: Vec::new(),
        }
    }

    /// Add a new work package to the sequence
    ///
    /// Generates actions, updates state, and records the transition
    pub fn add_work_package(
        &mut self,
        num_actions: usize,
        gas_limit: u64,
    ) -> Result<StateTransition> {
        // Capture pre-state
        let pre_state = self.capture_state();

        // Generate synthetic actions for sequence testing
        let actions = self.generate_actions(num_actions)?;

        // Apply actions to state
        for action in &actions {
            // Check nullifier doesn't already exist
            if self.nullifier_set.contains(&action.nullifier) {
                return Err(Error::NullifierExists);
            }

            // Add nullifier to spent set
            self.nullifier_set.insert(action.nullifier);

            // Add commitment to tree
            self.commitment_tree.append(action.commitment);
        }

        // Update current state
        self.current_state.commitment_root = self.commitment_tree.root();
        self.current_state.commitment_size = self.commitment_tree.size() as u64;
        self.current_state.nullifier_root = self.nullifier_set.root();

        // Capture post-state
        let post_state = self.capture_state();

        // Create transition
        let transition = StateTransition {
            index: self.transitions.len(),
            pre_state,
            actions,
            post_state: post_state.clone(),
            gas_used: gas_limit, // Simplified - assume full gas used
        };

        self.transitions.push(transition.clone());

        Ok(transition)
    }

    /// Generate dummy Orchard actions
    fn generate_actions(&self, count: usize) -> Result<Vec<ActionSummary>> {
        let mut actions = Vec::new();

        for i in 0..count {
            // Generate pseudo-random nullifier and commitment
            // In production, these would come from actual Orchard notes
            let mut nullifier = [0u8; 32];
            let commitment = pallas::Base::from(
                (i as u64) + 1 + (self.transitions.len() as u64) * 1000,
            )
            .to_repr();

            // Simple pseudo-random generation based on index and current state
            nullifier[0..8].copy_from_slice(&(i as u64).to_le_bytes());
            nullifier[8..16].copy_from_slice(&self.transitions.len().to_le_bytes());
            nullifier[16..32].copy_from_slice(&self.current_state.commitment_root[0..16]);

            actions.push(ActionSummary {
                nullifier,
                commitment,
                is_dummy: false,
            });
        }

        Ok(actions)
    }

    /// Capture current state snapshot
    fn capture_state(&mut self) -> StateSnapshot {
        StateSnapshot {
            commitment_root: self.commitment_tree.root(),
            commitment_size: self.commitment_tree.size() as u64,
            nullifier_root: self.nullifier_set.root(),
            nullifier_count: 0, // TODO: track this properly
        }
    }

    /// Build the final sequence
    pub fn build(self, initial_state: OrchardState) -> WorkPackageSequence {
        WorkPackageSequence {
            initial_state,
            transitions: self.transitions,
        }
    }

    /// Get current state
    pub fn current_state(&self) -> &OrchardState {
        &self.current_state
    }
}

impl WorkPackageSequence {
    /// Verify the sequence is valid (each transition chains correctly)
    pub fn verify(&self) -> Result<()> {
        if self.transitions.is_empty() {
            return Ok(());
        }

        // Verify first transition starts from initial state
        let first = &self.transitions[0];
        if first.pre_state.commitment_root != self.initial_state.commitment_root {
            return Err(Error::InvalidState(
                "First transition doesn't match initial state".to_string()
            ));
        }

        // Verify each transition chains to the next
        for i in 1..self.transitions.len() {
            let prev = &self.transitions[i - 1];
            let curr = &self.transitions[i];

            if curr.pre_state.commitment_root != prev.post_state.commitment_root {
                return Err(Error::InvalidState(
                    format!("Transition {} pre-state doesn't match previous post-state", i)
                ));
            }

            if curr.pre_state.nullifier_root != prev.post_state.nullifier_root {
                return Err(Error::InvalidState(
                    format!("Transition {} nullifier root doesn't chain", i)
                ));
            }
        }

        Ok(())
    }

    /// Get the final state after all transitions
    pub fn final_state(&self) -> Option<&StateSnapshot> {
        self.transitions.last().map(|t| &t.post_state)
    }

    /// Pretty-print the sequence
    pub fn print_summary(&self) {
        println!("Work Package Sequence Summary");
        println!("==============================");
        println!();
        println!("Initial State:");
        println!("  Commitment Root: {}", hex::encode(self.initial_state.commitment_root));
        println!("  Commitment Size: {}", self.initial_state.commitment_size);
        println!("  Nullifier Root:  {}", hex::encode(self.initial_state.nullifier_root));
        println!();

        for (i, transition) in self.transitions.iter().enumerate() {
            println!("Transition #{}", i);
            println!("  Actions: {}", transition.actions.len());
            println!("  Gas Used: {}", transition.gas_used);
            println!("  Pre-State:");
            println!("    Commitment Root: {}", hex::encode(transition.pre_state.commitment_root));
            println!("    Commitment Size: {}", transition.pre_state.commitment_size);
            println!("    Nullifier Root:  {}", hex::encode(transition.pre_state.nullifier_root));
            println!("  Post-State:");
            println!("    Commitment Root: {}", hex::encode(transition.post_state.commitment_root));
            println!("    Commitment Size: {}", transition.post_state.commitment_size);
            println!("    Nullifier Root:  {}", hex::encode(transition.post_state.nullifier_root));
            println!();
        }

        if let Some(final_state) = self.final_state() {
            println!("Final State:");
            println!("  Commitment Root: {}", hex::encode(final_state.commitment_root));
            println!("  Commitment Size: {}", final_state.commitment_size);
            println!("  Nullifier Root:  {}", hex::encode(final_state.nullifier_root));
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_sequence_builder() {
        let initial_state = OrchardState::default();
        let mut builder = SequenceBuilder::new(initial_state.clone());

        // Add first work package
        let t1 = builder.add_work_package(2, 100_000).unwrap();
        assert_eq!(t1.index, 0);
        assert_eq!(t1.actions.len(), 2);

        // Add second work package
        let t2 = builder.add_work_package(3, 150_000).unwrap();
        assert_eq!(t2.index, 1);
        assert_eq!(t2.actions.len(), 3);

        // Verify post-state of t1 matches pre-state of t2
        assert_eq!(t1.post_state.commitment_root, t2.pre_state.commitment_root);
        assert_eq!(t1.post_state.nullifier_root, t2.pre_state.nullifier_root);

        // Build and verify sequence
        let sequence = builder.build(initial_state);
        sequence.verify().unwrap();
    }
}
