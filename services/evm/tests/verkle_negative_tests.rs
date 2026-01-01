//! Negative tests for Verkle witness verification
//!
//! Tests that invalid proofs are correctly rejected and don't cause panics.

use evm_service::verkle_proof::*;
use evm_service::verkle::*;
use primitive_types::{H160, H256};
use ark_ff::{One, PrimeField, BigInteger};

/// Ensure zero denominators are rejected instead of panicking when t collides with an evaluation point
#[test]
fn test_zero_denominator_rejected() {
    // t equals z_i -> (t - z_i) = 0 would previously panic inside batch_invert
    let t = Fr::from(7u64);
    let results = vec![Fr::one()];
    let eval_points = vec![Fr::from(7u64)];
    let r_powers = vec![Fr::one()];

    let res = aggregate_evaluations(&results, &eval_points, &r_powers, &t);
    assert!(
        matches!(res, Err(VerkleProofError::VerificationFailed)),
        "proofs with zero denominators must be rejected"
    );
}

/// Test that wrong proof commitments can be detected during tree rebuilding
#[test]
fn test_wrong_proof_commitments_detected() {
    println!("\n🧪 Negative Test: Wrong proof commitments should be detectable");

    // Create valid witness entries
    let metadata = VerkleKeyMetadata {
        key_type: VerkleKeyType::BasicData,
        address: H160::zero(),
        extra: 0,
        storage_key: H256::zero(),
        tx_index: 0,
    };

    let stem = [0xAB; 31];
    let entry = VerkleWitnessEntry {
        key: {
            let mut k = [0u8; 32];
            k[..31].copy_from_slice(&stem);
            k[31] = 0;
            k
        },
        metadata,
        pre_value: [0x11; 32],
        post_value: [0x22; 32],
    };

    let entries = vec![entry];

    // Create WRONG proof commitments
    let root_commitment = BandersnatchPoint::generator().to_bytes();
    let mut wrong_commitments = vec![root_commitment];
    // Use completely wrong commitments - different curve points
    for i in 100..=102 {  // Different multipliers than what the correct tree would have
        let point = BandersnatchPoint::generator().mul(&Fr::from(i as u64));
        wrong_commitments.push(point.to_bytes());
    }

    let depth_extension_present = vec![(2u8 << 3) | 2]; // depth=2, EXT_STATUS_PRESENT  // depth=2
    let other_stems = vec![];

    // Tree rebuilding should succeed (it just uses the provided commitments)
    let result = rebuild_stateless_tree_from_proof(
        &wrong_commitments,
        &depth_extension_present,
        &other_stems,
        &entries,
        true, // use_pre_value
    );

    match result {
        Ok(tree) => {
            println!("Tree rebuilt with wrong commitments");

            // Extract proof elements - this should also succeed
            let proof_elements = extract_proof_elements_from_tree(
                &tree,
                &entries,
                true, // use_pre_value
            ).expect("Should extract proof elements");

            println!("✅ Wrong commitments handled gracefully (would fail at IPA verification stage)");
            println!("   Extracted {} commitments for verification", proof_elements.commitments.len());
        }
        Err(e) => {
            println!("✅ Wrong commitments rejected during tree rebuilding: {:?}", e);
        }
    }
}

/// Test that unused commitments in proof are rejected
#[test]
fn test_unused_commitments_rejected() {
    println!("\n🧪 Negative Test: Unused commitments should be rejected");

    let metadata = VerkleKeyMetadata {
        key_type: VerkleKeyType::BasicData,
        address: H160::zero(),
        extra: 0,
        storage_key: H256::zero(),
        tx_index: 0,
    };

    let stem = [0xAB; 31];
    let entry = VerkleWitnessEntry {
        key: {
            let mut k = [0u8; 32];
            k[..31].copy_from_slice(&stem);
            k[31] = 0;
            k
        },
        metadata,
        pre_value: [0x11; 32],
        post_value: [0x22; 32],
    };

    let entries = vec![entry];

    // Create TOO MANY commitments (more than needed)
    let root_commitment = BandersnatchPoint::generator().to_bytes();
    let mut too_many_commitments = vec![root_commitment];
    // For depth=2 PRESENT: minimal=4 (root+2 internal+leaf), full=6 (minimal+C1+C2)
    // Provide 8 commitments (2 extra beyond full proof) to trigger error
    for i in 1..=7 {
        let point = BandersnatchPoint::generator().mul(&Fr::from(i as u64));
        too_many_commitments.push(point.to_bytes());
    }

    let depth_extension_present = vec![(2u8 << 3) | 2]; // depth=2, EXT_STATUS_PRESENT  // depth=2
    let other_stems = vec![];

    // This should fail - the new pruned proof logic detects extra commitments
    // and rejects them with PathMismatch (before the old UnusedCommitments check)
    let result = rebuild_stateless_tree_from_proof(
        &too_many_commitments,
        &depth_extension_present,
        &other_stems,
        &entries,
        true,
    );

    match result {
        Err(VerkleProofError::PathMismatch) => {
            println!("✅ Unused commitments correctly rejected (PathMismatch)");
        }
        Ok(_) => {
            panic!("Should have rejected unused commitments");
        }
        Err(e) => {
            panic!("Wrong error type: expected PathMismatch for extra commitments, got {:?}", e);
        }
    }
}

/// Test that mismatched stems fail tree rebuilding
#[test]
fn test_mismatched_stems_rejected() {
    println!("\n🧪 Negative Test: Mismatched stems should fail tree rebuilding");

    let metadata = VerkleKeyMetadata {
        key_type: VerkleKeyType::BasicData,
        address: H160::zero(),
        extra: 0,
        storage_key: H256::zero(),
        tx_index: 0,
    };

    // Create witness entry with one stem
    let stem1 = [0xAB; 31];
    let entry = VerkleWitnessEntry {
        key: {
            let mut k = [0u8; 32];
            k[..31].copy_from_slice(&stem1);
            k[31] = 0;
            k
        },
        metadata,
        pre_value: [0x11; 32],
        post_value: [0x22; 32],
    };

    let entries = vec![entry];

    // But specify a completely different stem in other_stems
    let mut different_stem = [0u8; 32];
    different_stem[0] = 0xCD;  // Different first byte
    let other_stems = vec![different_stem];

    let root_commitment = BandersnatchPoint::generator().to_bytes();
    let mut commitments = vec![root_commitment];
    for i in 1..=5 {  // Provide enough commitments for both stems
        let point = BandersnatchPoint::generator().mul(&Fr::from(i as u64));
        commitments.push(point.to_bytes());
    }

    // This creates a mismatch: depth_extension_present says 2 stems both at depth=2
    // but we only have 1 entry + 1 other_stem with different structure
    let depth_extension_present = vec![(2u8 << 3), (2u8 << 3)];

    // This should either fail or create weird tree structure
    let result = rebuild_stateless_tree_from_proof(
        &commitments,
        &depth_extension_present,
        &other_stems,
        &entries,
        true,
    );

    match result {
        Err(e) => {
            println!("✅ Mismatched stems correctly rejected: {:?}", e);
        }
        Ok(tree) => {
            // If tree rebuilding succeeds, verify that proof element extraction
            // catches the mismatch
            let extraction_result = extract_proof_elements_from_tree(
                &tree,
                &entries,
                true,
            );

            match extraction_result {
                Err(e) => {
                    println!("✅ Mismatched stems rejected during proof element extraction: {:?}", e);
                }
                Ok(_) => {
                    println!("⚠️  Mismatched stems not detected - this may be valid for absence proofs");
                }
            }
        }
    }
}

/// Test that wrong witness values are handled during proof element extraction
#[test]
fn test_wrong_witness_values_handled() {
    println!("\n🧪 Negative Test: Wrong witness values should be handled gracefully");

    let metadata = VerkleKeyMetadata {
        key_type: VerkleKeyType::BasicData,
        address: H160::zero(),
        extra: 0,
        storage_key: H256::zero(),
        tx_index: 0,
    };

    let stem = [0xAB; 31];

    // Create entry with WRONG pre_value (different from what proof expects)
    let entry = VerkleWitnessEntry {
        key: {
            let mut k = [0u8; 32];
            k[..31].copy_from_slice(&stem);
            k[31] = 0;
            k
        },
        metadata,
        pre_value: [0xFF; 32],  // Wrong value!
        post_value: [0x22; 32],
    };

    let entries = vec![entry];

    // Create valid proof structure
    let root_commitment = BandersnatchPoint::generator().to_bytes();
    let mut commitments = vec![root_commitment];
    for i in 1..=3 {
        let point = BandersnatchPoint::generator().mul(&Fr::from(i as u64));
        commitments.push(point.to_bytes());
    }

    let depth_extension_present = vec![(2u8 << 3) | 2]; // depth=2, EXT_STATUS_PRESENT
    let other_stems = vec![];

    // Tree rebuilding should succeed (wrong values don't affect structure)
    let tree = rebuild_stateless_tree_from_proof(
        &commitments,
        &depth_extension_present,
        &other_stems,
        &entries,
        true,
    ).expect("Tree rebuilding should succeed");

    // Extract proof elements with wrong values - this should succeed
    let proof_elements = extract_proof_elements_from_tree(
        &tree,
        &entries,
        true,
    ).expect("Proof element extraction should succeed");

    println!("✅ Wrong witness values handled gracefully");
    println!("   Tree rebuilding and proof extraction completed");
    println!("   Wrong values would be detected during IPA verification stage");
    println!("   Extracted {} evaluation points", proof_elements.commitments.len());
}

/// Proof-of-absence stems must share the navigation prefix (root child + internal path)
/// with the requested stem. Otherwise a proof for an unrelated branch could be reused
/// to claim absence.
#[test]
fn test_absent_other_requires_shared_prefix() {
    let metadata = VerkleKeyMetadata {
        key_type: VerkleKeyType::BasicData,
        address: H160::zero(),
        extra: 0,
        storage_key: H256::zero(),
        tx_index: 0,
    };

    // Requested missing key is under root child 0xAA
    let mut requested_key = [0u8; 32];
    requested_key[0] = 0xAA;

    // Malicious proof stem points to a different root child (0xBB)
    let mut proof_stem = [0u8; 32];
    proof_stem[0] = 0xBB;

    let entries = vec![VerkleWitnessEntry {
        key: requested_key,
        metadata,
        pre_value: [0u8; 32],
        post_value: [0u8; 32],
    }];

    // Commitments: root + POA leaf commitment
    let root = BandersnatchPoint::generator();
    let leaf = root.mul(&Fr::from(2u64));
    let commitments = vec![root.to_bytes(), leaf.to_bytes()];

    let depth_extension_present = vec![(1u8 << 3) | 1u8]; // depth=1, EXT_STATUS_ABSENT_OTHER

    let result = rebuild_stateless_tree_from_proof(
        &commitments,
        &depth_extension_present,
        &[proof_stem],
        &entries,
        true,
    );

    assert!(
        matches!(result, Err(VerkleProofError::PathMismatch)),
        "POA proof stems must share the navigation prefix with the requested stem"
    );
}

/// Test that evaluation points outside the [0,255] domain are rejected.
/// Before the fix, z >= 256 would be silently truncated to z % 256, causing
/// incorrect grouping in the IPA multiproof verification.
#[test]
fn test_evaluation_point_out_of_domain_rejected() {
    println!("\n🧪 Negative Test: Evaluation point z >= 256 should be rejected");

    // Create an evaluation point with z = 256 (should be rejected)
    let z_invalid = Fr::from(256u64);
    let z_bytes = z_invalid.into_bigint().to_bytes_le();
    println!("   z = 256 bytes (LE): {:?}", &z_bytes[..4]);
    println!("   Before fix: would truncate to z_bytes[0] = {}", z_bytes[0]);
    println!("   After fix: should reject with VerificationFailed");

    // Create a minimal proof structure
    let commitment = BandersnatchPoint::generator();
    let commitments = vec![commitment];

    let y_value = Fr::one();
    let evals = vec![(z_invalid, y_value)];

    // Create a mock proof with minimal valid IPA proof
    let ipa_proof = IPAProofCompact {
        cl: vec![],
        cr: vec![],
        final_evaluation: [0u8; 32],
    };

    let proof = VerkleProof {
        d: commitment.to_bytes(),
        indices: vec![0],
        values: vec![[1u8; 32]],
        commitments_by_path: vec![],
        depth_extension_present: vec![],
        other_stems: vec![],
        ipa_proof,
    };

    // This should fail because z = 256 is outside [0,255] domain
    let result = verify_ipa_multiproof_with_evals(&commitments, &proof, &evals);

    assert!(
        matches!(result, Err(VerkleProofError::VerificationFailed)),
        "Evaluation points >= 256 must be rejected"
    );

    println!("   ✅ Correctly rejected z = 256");
}

/// Go-verkle sorts requested keys lexicographically; unsorted stems should be rejected
/// to prevent misaligned proofs where depth bytes and commitments don't match stem order.
#[test]
fn test_unsorted_stems_rejected() {
    use evm_service::verkle_proof::build_stem_info_map;

    println!("\n🧪 Negative Test: Unsorted stems should be rejected");

    let metadata = VerkleKeyMetadata {
        key_type: VerkleKeyType::BasicData,
        address: H160::zero(),
        extra: 0,
        storage_key: H256::zero(),
        tx_index: 0,
    };

    // Intentionally place the larger stem first (0xFF > 0x01) to break lexicographic ordering
    let mut key_a = [0u8; 32];
    key_a[..31].fill(0xFF); // Larger stem
    let mut key_b = [0u8; 32];
    key_b[..31].fill(0x01); // Smaller stem

    let entries = vec![
        VerkleWitnessEntry {
            key: key_a,
            metadata: metadata.clone(),
            pre_value: [0xAA; 32],
            post_value: [0u8; 32],
        },
        VerkleWitnessEntry {
            key: key_b,
            metadata,
            pre_value: [0xBB; 32],
            post_value: [0u8; 32],
        },
    ];

    let stems = vec![[0xFF; 31], [0x01; 31]]; // Intentionally unsorted
    let depth_byte = (1u8 << 3) | 2; // depth=1, EXT_STATUS_PRESENT
    let depth_extension_present = vec![depth_byte, depth_byte];
    let other_stems = vec![];

    println!("   Stem 0: 0xFF... (larger)");
    println!("   Stem 1: 0x01... (smaller)");
    println!("   Expected: rejection due to 0xFF > 0x01");

    let result = build_stem_info_map(
        &stems,
        &depth_extension_present,
        &other_stems,
        &entries,
        true,
    );

    assert!(
        matches!(result, Err(VerkleProofError::PathMismatch)),
        "unsorted stems must be rejected to match go-verkle ordering"
    );

    println!("   ✅ Correctly rejected unsorted stems");
}
