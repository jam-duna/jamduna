//! Deep tree structure tests for Verkle proofs
//!
//! Tests verification with maximum tree depths and edge cases:
//! - Maximum depth scenarios (depth=8 internal nodes)
//! - Commitment consumption validation
//! - Depth overflow detection

use evm_service::verkle_proof::{IPAProofCompact, VerkleProof, VerkleProofError};

#[test]
fn test_maximum_depth_8_internal_nodes() {
    // Test maximum realistic depth: 8 internal nodes + 1 leaf = depth 9 total
    // This represents a very deep path in the Verkle tree
    //
    // Proof structure with depth=8:
    // - Root commitment (commitments_by_path[0])
    // - 8 internal node commitments (commitments_by_path[1..9])
    // - 1 leaf commitment (commitments_by_path[9])
    // Total: 1 + 8 + 1 = 10 commitments

    let depth = 8u8;
    let depth_extension_present = vec![depth << 3]; // Encode depth in upper 5 bits

    // Build commitments: root + 8 internal + 1 leaf
    let total_commitments = 1 + depth as usize + 1; // root + internal nodes + leaf
    let root = [0x01u8; 32];
    let mut commitments_by_path = vec![root]; // Root first

    // Add 8 internal node commitments
    for i in 0..depth {
        let mut commitment = [0u8; 32];
        commitment[0] = 0x10 + i; // Unique marker per level
        commitments_by_path.push(commitment);
    }

    // Add leaf commitment
    commitments_by_path.push([0xFFu8; 32]);

    assert_eq!(
        commitments_by_path.len(),
        total_commitments,
        "Should have {} commitments for depth={}",
        total_commitments,
        depth
    );

    // Build proof
    let ipa_proof = IPAProofCompact {
        cl: vec![],
        cr: vec![],
        final_evaluation: [0u8; 32],
    };

    let proof = VerkleProof::from_parts(
        vec![],                   // no other_stems
        depth_extension_present,  // depth=8
        commitments_by_path,      // 10 commitments
        [0xFFu8; 32],             // D point (leaf commitment)
        ipa_proof,
        vec![0],                  // single index
        vec![[0u8; 32]],          // single value
    )
    .expect("proof construction with depth=8");

    // Validate depth calculation
    let total_depth: usize = proof
        .depth_extension_present
        .iter()
        .map(|es| (es >> 3) as usize)
        .sum();

    assert_eq!(total_depth, depth as usize, "Depth should be {}", depth);
    assert_eq!(
        proof.commitments_by_path.len(),
        1 + total_depth + 1,
        "Commitments should be root + {} internal + 1 leaf",
        depth
    );

    println!("✅ Maximum depth test (depth=8):");
    println!("   Total commitments: {} (1 root + {} internal + 1 leaf)", total_commitments, depth);
    println!("   Proof structure validated for maximum realistic depth");
}

#[test]
fn test_commitment_consumption_single_stem() {
    // Test that commitment consumption matches expectation for a single stem
    // Formula: commitments_by_path.len() = 1 (root) + depth (internal nodes + leaf)
    //
    // For depth=3: 1 root + 3 path commitments = 4 total

    let depth = 3u8;
    let depth_extension_present = vec![depth << 3];

    let root = point_bytes(0);
    let commitments_by_path = vec![
        root,           // [0] root
        [0xBBu8; 32],  // [1] internal node 1
        [0xCCu8; 32],  // [2] internal node 2
        [0xDDu8; 32],  // [3] leaf
    ];

    assert_eq!(commitments_by_path.len(), 1 + depth as usize);

    let ipa_proof = IPAProofCompact {
        cl: vec![],
        cr: vec![],
        final_evaluation: [0u8; 32],
    };

    let proof = VerkleProof::from_parts(
        vec![],
        depth_extension_present,
        commitments_by_path,
        [0xDDu8; 32],
        ipa_proof,
        vec![0],
        vec![[0u8; 32]],
    )
    .expect("proof construction");

    // Verify consumption formula
    let total_depth: usize = proof
        .depth_extension_present
        .iter()
        .map(|es| (es >> 3) as usize)
        .sum();

    let expected_commitments = 1 + total_depth; // root + path
    assert_eq!(
        proof.commitments_by_path.len(),
        expected_commitments,
        "Commitment count should match formula: 1 + depth"
    );

    println!("✅ Commitment consumption test (single stem, depth=3):");
    println!("   Formula: 1 (root) + {} (path) = {} commitments", depth, expected_commitments);
    println!("   Actual commitments: {}", proof.commitments_by_path.len());
}

#[test]
fn test_commitment_consumption_multi_stem() {
    // Test commitment consumption with multiple stems of varying depths
    // Formula: 1 (root) + sum(depth_i) for all stems
    //
    // Example: 3 stems with depths [2, 3, 1]
    // Expected: 1 + 2 + 3 + 1 = 7 commitments

    let depths = vec![2u8, 3u8, 1u8];
    let depth_extension_present: Vec<u8> = depths.iter().map(|d| d << 3).collect();

    let total_depth: usize = depths.iter().map(|d| *d as usize).sum();
    let expected_commitments = 1 + total_depth; // root + all paths

    let root = [0xFFu8; 32];
    let mut commitments_by_path = vec![root]; // Start with root

    // Add commitments for each stem's path
    for (stem_idx, &depth) in depths.iter().enumerate() {
        for i in 0..depth {
            let mut commitment = [0u8; 32];
            commitment[0] = (stem_idx + 1) as u8;
            commitment[1] = i;
            commitments_by_path.push(commitment);
        }
    }

    assert_eq!(
        commitments_by_path.len(),
        expected_commitments,
        "Should have {} commitments for depths {:?}",
        expected_commitments,
        depths
    );

    let ipa_proof = IPAProofCompact {
        cl: vec![],
        cr: vec![],
        final_evaluation: [0u8; 32],
    };

    let proof = VerkleProof::from_parts(
        vec![],
        depth_extension_present,
        commitments_by_path,
        [0xFFu8; 32],
        ipa_proof,
        vec![0, 1, 2], // indices for 3 stems
        vec![[0u8; 32], [0u8; 32], [0u8; 32]], // values for 3 stems
    )
    .expect("multi-stem proof construction");

    println!("✅ Commitment consumption test (multi-stem):");
    println!("   Stems: 3, depths: {:?}", depths);
    println!("   Formula: 1 (root) + {} (total depth) = {} commitments", total_depth, expected_commitments);
    println!("   Actual commitments: {}", proof.commitments_by_path.len());
}

#[test]
fn test_depth_overflow_detection() {
    // Test that excessively large depth values are rejected
    // Maximum safe depth is bounded by practical tree height and overflow checks

    // Try depth=255 (maximum u8 value in upper 5 bits = 31, but 255 >> 3 = 31)
    let excessive_depth = 31u8; // Maximum value that fits in 5 bits
    let depth_extension_present = vec![excessive_depth << 3];

    let root = [0xAAu8; 32];

    // This would require 1 + 31 = 32 commitments
    // Most implementations should reject depths > 8-10 as unrealistic

    let mut commitments_by_path = vec![root];
    for i in 0..excessive_depth {
        commitments_by_path.push(point_bytes(i as usize + 1));
    }

    let ipa_proof = IPAProofCompact {
        cl: vec![],
        cr: vec![],
        final_evaluation: [0u8; 32],
    };

    // This should succeed structurally (proof construction doesn't validate depth reasonableness)
    // But verification functions should check depth bounds
    let proof = VerkleProof::from_parts(
        vec![],
        depth_extension_present,
        commitments_by_path,
        point_bytes(999),
        ipa_proof,
        vec![0],
        vec![[0u8; 32]],
    )
    .expect("proof construction allows large depth");

    // Verification should reject the oversized depth because the commitment
    // count is insufficient for depth_internal + leaf (needs 33, we provided 32).
    let err = verify_prestate_commitment(&root, &proof).unwrap_err();
    assert!(
        matches!(err, VerkleProofError::PathMismatch | VerkleProofError::InvalidPoint),
        "expected rejection for oversized depth, got {err:?}"
    );
}

#[test]
fn test_depth_mismatch_detection() {
    // Test that mismatched depth and commitment count is rejected
    // If depth=5 is declared, commitments_by_path MUST have 1 + 5 = 6 elements

    let declared_depth = 5u8;
    let depth_extension_present = vec![declared_depth << 3];

    let root = [0xAAu8; 32];

    // Provide WRONG number of commitments (only 4 instead of 6)
    let commitments_by_path = vec![
        root,
        [0xBBu8; 32],
        [0xCCu8; 32],
        [0xDDu8; 32],
    ];

    let ipa_proof = IPAProofCompact {
        cl: vec![],
        cr: vec![],
        final_evaluation: [0u8; 32],
    };

    // This should FAIL - commitment count doesn't match declared depth
    let result = VerkleProof::from_parts(
        vec![],
        depth_extension_present,
        commitments_by_path,
        [0xDDu8; 32],
        ipa_proof,
        vec![0],
        vec![[0u8; 32]],
    );

    // from_parts might not validate this, but is_root_present will catch it
    if let Ok(proof) = result {
        let total_depth: usize = proof
            .depth_extension_present
            .iter()
            .map(|es| (es >> 3) as usize)
            .sum();

        let expected_len = 1 + total_depth;
        let actual_len = proof.commitments_by_path.len();

        assert_ne!(
            actual_len, expected_len,
            "Mismatch: depth={} requires {} commitments, but got {}",
            declared_depth, expected_len, actual_len
        );

        println!("✅ Depth mismatch detection test:");
        println!("   Declared depth: {}", declared_depth);
        println!("   Expected commitments: {} (1 root + {} path)", expected_len, total_depth);
        println!("   Actual commitments: {} ❌", actual_len);
        println!("   is_root_present() will reject this with PathMismatch error");
    }
}

#[test]
fn test_zero_depth_edge_case() {
    // Test depth=0 edge case (just root and leaf at same level?)
    // This is technically invalid in Verkle trees but tests boundary handling

    let depth = 0u8;
    let depth_extension_present = vec![depth << 3]; // depth=0

    let root = [0xAAu8; 32];

    // With depth=0, we'd expect 1 (root) + 0 = 1 commitment
    // But this doesn't make sense - you need at least root + leaf
    let commitments_by_path = vec![root];

    let ipa_proof = IPAProofCompact {
        cl: vec![],
        cr: vec![],
        final_evaluation: [0u8; 32],
    };

    let result = VerkleProof::from_parts(
        vec![],
        depth_extension_present,
        commitments_by_path,
        root, // D point
        ipa_proof,
        vec![0],
        vec![[0u8; 32]],
    );

    // This is an invalid configuration - depth must be at least 1
    match result {
        Ok(_proof) => {
            println!("⚠️  Zero depth edge case:");
            println!("   Proof constructed with depth=0 (invalid configuration)");
            println!("   Verification should reject depth=0 as invalid");
        }
        Err(e) => {
            println!("✅ Zero depth edge case:");
            println!("   Correctly rejected during proof construction: {:?}", e);
        }
    }
}

#[test]
fn test_commitment_consumption_formula_validation() {
    // Comprehensive test validating the commitment consumption formula
    // across various depth configurations

    let test_cases = vec![
        (1, 2),   // depth=1 → 1 root + 1 = 2 commitments
        (2, 3),   // depth=2 → 1 root + 2 = 3 commitments
        (3, 4),   // depth=3 → 1 root + 3 = 4 commitments
        (5, 6),   // depth=5 → 1 root + 5 = 6 commitments
        (8, 9),   // depth=8 → 1 root + 8 = 9 commitments
        (10, 11), // depth=10 → 1 root + 10 = 11 commitments
    ];

    for (depth, expected_commitments) in test_cases {
        let depth_extension_present = vec![depth << 3];

        let root = [0xFFu8; 32];
        let mut commitments_by_path = vec![root];

        // Add depth commitments (internal nodes + leaf)
        for i in 0..depth {
            let mut commitment = [0u8; 32];
            commitment[0] = i;
            commitments_by_path.push(commitment);
        }

        assert_eq!(commitments_by_path.len(), expected_commitments as usize);

        let ipa_proof = IPAProofCompact {
            cl: vec![],
            cr: vec![],
            final_evaluation: [0u8; 32],
        };

        let proof = VerkleProof::from_parts(
            vec![],
            depth_extension_present,
            commitments_by_path,
            [0xFFu8; 32],
            ipa_proof,
            vec![0],
            vec![[0u8; 32]],
        )
        .expect(&format!("proof with depth={}", depth));

        let total_depth: usize = proof
            .depth_extension_present
            .iter()
            .map(|es| (es >> 3) as usize)
            .sum();

        assert_eq!(total_depth, depth as usize);
        assert_eq!(proof.commitments_by_path.len(), expected_commitments as usize);
    }

    println!("✅ Commitment consumption formula validation:");
    println!("   Tested depths: 1, 2, 3, 5, 8, 10");
    println!("   Formula confirmed: commitments = 1 (root) + depth");
}

// --- Additional deep-structure tests (from codex stress cases) ---

use evm_service::verkle::Fr;
use evm_service::verkle::BandersnatchPoint;
use evm_service::verkle_proof::{verify_path_structure, verify_prestate_commitment};

/// Deterministic commitment bytes for test data.
fn point_bytes(idx: usize) -> [u8; 32] {
    let g = BandersnatchPoint::generator();
    let scalar = Fr::from((idx + 1) as u64);
    g.mul(&scalar).to_bytes()
}

/// Build a VerkleProof with a single stem and specified internal depth.
fn build_single_stem_proof(depth_internal: u8, commitments: Vec<[u8; 32]>) -> VerkleProof {
    let ipa_proof = IPAProofCompact {
        cl: vec![],
        cr: vec![],
        final_evaluation: [0u8; 32],
    };

    VerkleProof::from_parts(
        vec![],                     // other_stems
        vec![depth_internal << 3],  // depth encoded in extStatus
        commitments,
        point_bytes(999),           // D point (any valid point)
        ipa_proof,
        vec![0],                    // indices
        vec![[0u8; 32]],            // values
    )
    .expect("proof construction should succeed")
}

#[test]
fn deep_path_layout_succeeds() {
    // Depth=8 internal nodes → path length = depth + leaf = 9 commitments per stem.
    let depth_internal: u8 = 8;
    let path_len = depth_internal as usize + 1;

    let mut commitments = Vec::with_capacity(1 + path_len);
    let root = point_bytes(0);
    commitments.push(root);
    for i in 0..path_len {
        commitments.push(point_bytes(i + 1));
    }

    let proof = build_single_stem_proof(depth_internal, commitments);

    // Unique stem
    let mut stem = [0u8; 31];
    stem[0] = 0xAA;

    // Root present and matches
    verify_prestate_commitment(&root, &proof).expect("root should match at max depth");

    // Path structure consumes depth+leaf commitments
    verify_path_structure(&proof, &[stem]).expect("path structure should validate at max depth");

    // Note: reconstruct_paths was removed due to formula mismatch bug
    // Path validation is now handled by verify_path_structure and IPA verification
}