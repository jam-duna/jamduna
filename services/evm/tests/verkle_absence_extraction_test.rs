use evm_service::verkle::*;
use evm_service::verkle_proof::{extract_proof_elements_from_tree, rebuild_stateless_tree_from_proof};
use primitive_types::{H160, H256};
use ark_ff::Zero;

#[test]
fn suffix_evaluations_include_absent_values() {
    // Requested keys with zero values must still produce suffix evaluations (encoded as zeros)
    // to mirror go-verkle's LeafNode.GetProofItems behavior.
    let metadata = VerkleKeyMetadata {
        key_type: VerkleKeyType::BasicData,
        address: H160::zero(),
        extra: 0,
        storage_key: H256::zero(),
        tx_index: 0,
    };

    let stem = [0xAAu8; 31];
    let suffix = 5u8;

    let mut key = [0u8; 32];
    key[..31].copy_from_slice(&stem);
    key[31] = suffix;

    let entry = VerkleWitnessEntry {
        key,
        metadata,
        pre_value: [0u8; 32],  // Present with zero value in pre-state
        post_value: [0u8; 32], // Present with zero value in post-state
    };
    let entries = vec![entry.clone()];

    // Minimal proof layout: root + leaf commitment, depth=1 (encoded with leaf included).
    let root_commitment = BandersnatchPoint::generator().to_bytes();
    let leaf_commitment = BandersnatchPoint::generator()
        .mul(&Fr::from(2u64))
        .to_bytes();
    let commitments_by_path = vec![root_commitment, leaf_commitment];
    let depth_extension_present = vec![(1u8 << 3) | 2]; // depth_internal=0, encoded depth=1, EXT_STATUS_PRESENT

    let tree = rebuild_stateless_tree_from_proof(
        &commitments_by_path,
        &depth_extension_present,
        &[],
        &entries,
        true,
    )
    .expect("tree builds from commitments");

    let proof_elements =
        extract_proof_elements_from_tree(&tree, &entries, true).expect("proof elements");

    // Expect: 1 root eval + marker/stem + C1 pointer + two suffix slots = 6 evaluations
    assert_eq!(
        proof_elements.commitments.len(),
        proof_elements.indices.len()
    );
    assert_eq!(proof_elements.indices.len(), proof_elements.values.len());
    assert_eq!(proof_elements.indices.len(), 6);

    // C1 pointer should still be emitted because the suffix lives in the C1 half.
    assert!(proof_elements.indices.contains(&2));

    // The requested suffix must emit two evaluations (lo/hi) for the zero-valued entry.
    // Bug 3 fix: Zero-valued entries are PRESENT (marker bit = 1), not absent (marker bit = 0).
    // So lo field element has marker bit = 1 in byte 16, making it non-zero.
    let suffix_slots: Vec<_> = proof_elements
        .indices
        .iter()
        .zip(proof_elements.values.iter())
        .filter(|(z, _)| **z == 2 * suffix || **z == 2 * suffix + 1)
        .collect();
    assert_eq!(suffix_slots.len(), 2, "suffix slots must be present");

    // lo field element: marker bit = 1 in byte 16, so NOT zero
    // hi field element: all zero bytes, so IS zero
    assert!(!suffix_slots[0].1.is_zero(), "lo should have marker bit");
    assert!(suffix_slots[1].1.is_zero(), "hi should be zero");
}
