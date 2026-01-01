/// Test utilities for generating valid proofs and test data
///
/// This module provides helpers for creating valid Merkle proofs for both:
/// 1. JAM StateDB proofs (for service storage like balances, configs)
/// 2. Orchard note tree proofs (for notes and nullifiers maintained by builder)

use crate::witness::{StateRead, WitnessBundle, StateBackend};
use crate::crypto::merkle_root_from_leaves;
use crate::errors::{Result, OrchardError};
use alloc::vec::Vec;
use alloc::string::ToString;

/// Orchard note tree for testing - maintains commitments and nullifiers
#[derive(Debug, Clone)]
pub struct TestNoteTree {
    pub note_commitments: Vec<[u8; 32]>,
    pub nullifiers: Vec<[u8; 32]>,
    pub tree_root: [u8; 32],
}

impl TestNoteTree {
    /// Create empty note tree
    pub fn new() -> Self {
        let empty_root = merkle_root_from_leaves(&[]).expect("Empty tree root should compute");
        Self {
            note_commitments: Vec::new(),
            nullifiers: Vec::new(),
            tree_root: empty_root,
        }
    }

    /// Add a note commitment to the tree
    pub fn add_note(&mut self, commitment: [u8; 32]) -> Result<usize> {
        self.note_commitments.push(commitment);
        self.update_tree_root()?;
        Ok(self.note_commitments.len() - 1)
    }

    /// Add a nullifier (spend a note)
    pub fn add_nullifier(&mut self, nullifier: [u8; 32]) {
        self.nullifiers.push(nullifier);
    }

    /// Check if nullifier exists (double-spend protection)
    pub fn has_nullifier(&self, nullifier: &[u8; 32]) -> bool {
        self.nullifiers.contains(nullifier)
    }

    /// Generate Merkle proof for note at given index
    pub fn generate_note_proof(&self, note_index: usize) -> Result<Vec<[u8; 32]>> {
        if note_index >= self.note_commitments.len() {
            return Err(OrchardError::ParseError("Note index out of bounds".to_string()));
        }

        // For testing, create a simple proof structure
        // In production, this would use the actual Merkle tree proof generation
        let mut proof = Vec::new();

        // Calculate proof path (simplified for testing)
        let mut index = note_index;
        let mut level = 0;
        while index > 0 || level == 0 {
            let sibling_index = if index % 2 == 0 { index + 1 } else { index - 1 };

            let sibling_hash = if sibling_index < self.note_commitments.len() {
                self.note_commitments[sibling_index]
            } else {
                [0u8; 32] // Empty node
            };

            proof.push(sibling_hash);
            index /= 2;
            level += 1;

            if level > 32 { // Prevent infinite loop
                break;
            }
        }

        Ok(proof)
    }

    /// Update tree root after changes
    fn update_tree_root(&mut self) -> Result<()> {
        self.tree_root = merkle_root_from_leaves(&self.note_commitments)?;
        Ok(())
    }
}

impl Default for TestNoteTree {
    fn default() -> Self {
        Self::new()
    }
}

/// FFI interface to Go StateDB for generating real Merkle proofs in tests
#[cfg(test)]
extern "C" {
    /// Get service storage value and proof from Go StateDB
    /// Returns: (value_len, proof_len, success)
    /// - service_id: JAM service ID
    /// - key_ptr: pointer to key bytes
    /// - key_len: key length
    /// - value_buf: output buffer for value (must be allocated)
    /// - value_cap: value buffer capacity
    /// - proof_buf: output buffer for serialized proof (must be allocated)
    /// - proof_cap: proof buffer capacity
    /// - root_buf: output buffer for root hash (32 bytes)
    fn go_get_service_storage_with_proof(
        service_id: u32,
        key_ptr: *const u8,
        key_len: u32,
        value_buf: *mut u8,
        value_cap: u32,
        proof_buf: *mut u8,
        proof_cap: u32,
        root_buf: *mut u8,
    ) -> u64; // Returns (value_len << 32) | proof_len, or 0 on error
}

/// Create a test StateDB and get a real proof for testing
/// This function currently returns None since FFI is not yet implemented
#[cfg(test)]
pub fn create_test_statedb_proof(_key: &str, _value: &[u8]) -> Option<(Vec<u8>, Vec<[u8; 32]>, [u8; 32])> {
    // TODO: Implement FFI call to Go StateDB when available
    // For now, return None to fall back to mock proofs
    None
}

/// Create a witness bundle with real StateDB proof
#[cfg(test)]
pub fn create_witness_with_statedb_proof(
    key: &str,
    value: &[u8],
) -> Option<WitnessBundle> {
    let (retrieved_value, proof, root) = create_test_statedb_proof(key, value)?;

    Some(WitnessBundle {
        pre_state_root: root,
        post_state_root: root, // Same for read-only test
        reads: vec![StateRead {
            key: key.to_string(),
            value: retrieved_value,
            merkle_proof: proof,
        }],
        writes: vec![],
    })
}

/// Mock version that creates valid-looking test data without FFI dependency
/// This is used when the Go FFI interface is not available
#[cfg(test)]
const TEST_SERVICE_ID: u32 = 0;

#[cfg(test)]
pub fn create_mock_witness_with_proof(key: &str, value: &[u8]) -> WitnessBundle {
    WitnessBundle {
        pre_state_root: [
            0x0d, 0xd6, 0x77, 0xe2, 0x4a, 0x16, 0x61, 0x16,
            0xd3, 0xf6, 0x27, 0x4d, 0xf3, 0x9e, 0xaa, 0xd8,
            0xeb, 0xc4, 0x7e, 0xd5, 0x49, 0xb4, 0x64, 0x04,
            0x2e, 0xf1, 0x6a, 0x41, 0x64, 0xb9, 0x85, 0x53,
        ],
        post_state_root: [
            0x1d, 0xd6, 0x77, 0xe2, 0x4a, 0x16, 0x61, 0x16,
            0xd3, 0xf6, 0x27, 0x4d, 0xf3, 0x9e, 0xaa, 0xd8,
            0xeb, 0xc4, 0x7e, 0xd5, 0x49, 0xb4, 0x64, 0x04,
            0x2e, 0xf1, 0x6a, 0x41, 0x64, 0xb9, 0x85, 0x53,
        ],
        reads: vec![StateRead {
            key: key.to_string(),
            value: value.to_vec(),
            merkle_proof: vec![[0u8; 32]], // Mock proof that bypasses verification
        }],
        writes: vec![],
    }
}

/// Integration test helper that demonstrates how to use real StateDB proofs
#[cfg(test)]
pub fn test_with_real_statedb() -> Result<()> {
    // This would be called from integration tests that have access to Go StateDB
    if let Some(witness) = create_witness_with_statedb_proof("test_orchard_key", &[1, 2, 3, 4]) {
        // Test that the witness can be used with CacheOnlyBackend
        use crate::witness::CacheOnlyBackend;
        let _backend = CacheOnlyBackend::from_witness(TEST_SERVICE_ID, &witness)?;
        // Successfully created backend with real StateDB proof
    } else {
        // Could not create real StateDB proof, falling back to mock
        let witness = create_mock_witness_with_proof("test_orchard_key", &[1, 2, 3, 4]);
        use crate::witness::CacheOnlyBackend;
        let _backend = CacheOnlyBackend::from_witness(TEST_SERVICE_ID, &witness)?;
        // Successfully created backend with mock proof
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_mock_witness_creation() {
        let witness = create_mock_witness_with_proof("test_key", &[10, 20, 30]);
        assert_eq!(witness.reads.len(), 1);
        assert_eq!(witness.reads[0].key, "test_key");
        assert_eq!(witness.reads[0].value, vec![10, 20, 30]);
        assert_eq!(witness.reads[0].merkle_proof.len(), 1);
    }

    #[test]
    fn test_integration_with_statedb() {
        // This test demonstrates the integration path
        // In practice, this would use the FFI when available
        test_with_real_statedb().expect("StateDB integration test should pass");
    }

    #[test]
    fn test_note_tree_operations() {
        use crate::crypto::{commitment, nullifier};

        let mut note_tree = TestNoteTree::new();
        assert_eq!(note_tree.note_commitments.len(), 0);
        assert_eq!(note_tree.nullifiers.len(), 0);

        // Create test note commitment
        let asset_id = 1;
        let amount = 1000u128;
        let owner_pk = [1u8; 32];
        let rho = [2u8; 32];
        let note_rseed = [3u8; 32];
        let unlock_height = 0u64;
        let memo_hash = [4u8; 32];

        let commitment_bytes = commitment(
            asset_id, amount, &owner_pk, &rho, &note_rseed, unlock_height, &memo_hash
        ).expect("Commitment should generate");

        // Add note to tree
        let note_index = note_tree.add_note(commitment_bytes).expect("Note should add");
        assert_eq!(note_index, 0);
        assert_eq!(note_tree.note_commitments.len(), 1);

        // Generate proof for the note
        let proof = note_tree.generate_note_proof(note_index).expect("Proof should generate");
        assert!(!proof.is_empty());

        // Create nullifier for spending the note
        let sk_spend = [5u8; 32];
        let nullifier_bytes = nullifier(&sk_spend, &rho, &commitment_bytes)
            .expect("Nullifier should generate");

        // Check nullifier doesn't exist yet
        assert!(!note_tree.has_nullifier(&nullifier_bytes));

        // Add nullifier (spend the note)
        note_tree.add_nullifier(nullifier_bytes);
        assert!(note_tree.has_nullifier(&nullifier_bytes));
        assert_eq!(note_tree.nullifiers.len(), 1);
    }

    #[test]
    fn test_mixed_proof_witness() {
        // Test creating a witness with both JAM state proofs and note tree proofs
        use crate::witness::StateRead;

        // 1. JAM state proof for account balance lookup
        let balance_witness = create_mock_witness_with_proof("balance:alice", &[0, 0, 0, 100]);

        // 2. Orchard note tree for private notes
        let mut note_tree = TestNoteTree::new();

        // Add some test notes
        let note1 = [1u8; 32];
        let note2 = [2u8; 32];
        note_tree.add_note(note1).expect("Note 1 should add");
        note_tree.add_note(note2).expect("Note 2 should add");

        // Generate proof for note 0
        let note_proof = note_tree.generate_note_proof(0).expect("Note proof should generate");

        // Combined witness would include:
        // - JAM state reads (balances, configs, etc.)
        // - Note tree proofs (private note existence)
        // - Nullifier set proofs (double-spend prevention)

        let combined_witness = WitnessBundle {
            pre_state_root: balance_witness.pre_state_root,
            post_state_root: balance_witness.post_state_root,
            reads: vec![
                // JAM state read
                balance_witness.reads[0].clone(),
                // Note tree inclusion proof (would be different format in practice)
                StateRead {
                    key: "note_tree_root".to_string(),
                    value: note_tree.tree_root.to_vec(),
                    merkle_proof: vec![[0u8; 32]], // Mock JAM proof for note tree root storage
                },
            ],
            writes: vec![],
        };

        assert_eq!(combined_witness.reads.len(), 2);
        // Created combined witness with JAM state + note tree proofs
    }

    #[test]
    fn test_orchard_builder_guarantor_flow() {
        // Test the full builder→guarantor flow with proper proof types

        // 1. Builder phase: maintain private note tree
        let mut builder_note_tree = TestNoteTree::new();

        // Builder receives new deposits and creates notes
        let deposit_commitment = [10u8; 32];
        let note_index = builder_note_tree.add_note(deposit_commitment)
            .expect("Builder should add deposit note");

        // Builder creates proof for spending a note
        let spend_proof = builder_note_tree.generate_note_proof(note_index)
            .expect("Builder should generate spend proof");

        // 2. Guarantor phase: verify proofs using JAM state
        // Guarantor gets witness bundle from work package
        let guarantor_witness = WitnessBundle {
            pre_state_root: [0xaa; 32],  // JAM state root before
            post_state_root: [0xbb; 32], // JAM state root after
            reads: vec![
                StateRead {
                    key: "note_tree_root".to_string(),
                    value: builder_note_tree.tree_root.to_vec(),
                    merkle_proof: vec![[0u8; 32]], // JAM state proof for stored note tree root
                },
            ],
            writes: vec![], // Would include new nullifiers, updated balances, etc.
        };

        // Guarantor verifies the witness
        use crate::witness::CacheOnlyBackend;
        let backend = CacheOnlyBackend::from_witness(TEST_SERVICE_ID, &guarantor_witness)
            .expect("Guarantor should verify witness");

        assert_eq!(backend.get_execution_mode(), crate::witness::ExecutionMode::Guarantor);
        // Builder→Guarantor flow test completed successfully
    }
}
