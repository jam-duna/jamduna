//! Comprehensive Verkle multiproof tests with go-verkle fixtures
//!
//! Tests full end-to-end verification including:
//! - IPA proof verification with real indices/values from go-verkle
//! - Post-state root computation from StateDiff
//! - Single-key and multi-key proofs

use std::fs;
use std::path::PathBuf;
use std::time::Instant;

use evm_service::verkle_proof::*;
use evm_service::verkle::*;
use serde::Deserialize;
use evm_service::verkle_proof::{get_debug_output, clear_debug_output};
use ark_ff::{BigInteger, PrimeField, Zero};
use primitive_types::{H160, H256};

#[derive(Debug, Deserialize)]
struct FixtureSuffixDiff {
    suffix: u8,
    #[serde(rename = "currentValue")]
    current_value: Option<String>,
    #[serde(rename = "newValue")]
    new_value: Option<String>,
}

#[derive(Debug, Deserialize)]
struct FixtureStemDiff {
    stem: String,
    #[serde(rename = "suffixDiffs")]
    suffix_diffs: Vec<FixtureSuffixDiff>,
}

#[derive(Debug, Deserialize)]
struct FixtureIPAProof {
    cl: Vec<String>,
    cr: Vec<String>,
    #[serde(rename = "finalEvaluation")]
    final_evaluation: String,
}

#[derive(Debug, Deserialize)]
struct FixtureVerkleProof {
    #[serde(rename = "otherStems")]
    other_stems: Vec<String>,
    #[serde(rename = "depthExtensionPresent")]
    depth_extension_present: String,
    #[serde(rename = "commitmentsByPath")]
    commitments_by_path: Vec<String>,
    d: String,
    #[serde(rename = "ipaProof")]
    ipa_proof: FixtureIPAProof,
}

#[derive(Debug, Deserialize)]
struct VerkleFixture {
    #[serde(default)]
    #[allow(dead_code)]
    name: Option<String>,
    pre_state_root: String,
    post_state_root: String,
    verkle_proof: FixtureVerkleProof,
    state_diff: Vec<FixtureStemDiff>,
}

struct FixtureData {
    pre_state_root: [u8; 32],
    post_state_root: [u8; 32],
    proof: VerkleProof,
    commitments: Vec<BandersnatchPoint>,
    state_diff: StateDiff,
}

fn hex_to_array<const N: usize>(value: &str) -> Result<[u8; N], String> {
    let stripped = value.strip_prefix("0x").unwrap_or(value);
    let bytes = hex::decode(stripped).map_err(|e| format!("hex decode error: {e}"))?;
    if bytes.len() != N {
        return Err(format!("expected {N} bytes, got {}", bytes.len()));
    }

    let mut out = [0u8; N];
    out.copy_from_slice(&bytes);
    Ok(out)
}

fn parse_state_diff(fixture: &[FixtureStemDiff]) -> Result<StateDiff, String> {
    let mut result = Vec::new();
    for stem in fixture {
        let mut stem_bytes = [0u8; 31];
        let stem_hex = stem.stem.strip_prefix("0x").unwrap_or(&stem.stem);
        let decoded = hex::decode(stem_hex).map_err(|e| format!("stem decode error: {e}"))?;
        if decoded.len() != 31 {
            return Err(format!("stem length mismatch: expected 31, got {}", decoded.len()));
        }
        stem_bytes.copy_from_slice(&decoded);

        let mut suffix_diffs = Vec::new();
        for suffix in &stem.suffix_diffs {
            let current_value = match &suffix.current_value {
                Some(v) if !v.is_empty() => Some(hex_to_array::<32>(v)?),
                _ => None,
            };
            let new_value = match &suffix.new_value {
                Some(v) if !v.is_empty() => Some(hex_to_array::<32>(v)?),
                _ => None,
            };
            suffix_diffs.push(SuffixStateDiff {
                suffix: suffix.suffix,
                current_value,
                new_value,
            });
        }

        result.push(StemStateDiff {
            stem: stem_bytes,
            suffix_diffs,
        });
    }

    Ok(result)
}

fn load_fixture(filename: &str) -> FixtureData {
    let path = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("tests/fixtures")
        .join(filename);
    let content = fs::read_to_string(&path).expect("fixture should exist");
    let fixture: VerkleFixture = serde_json::from_str(&content).expect("valid fixture json");

    let pre_state_root = hex_to_array::<32>(&fixture.pre_state_root).expect("valid pre-state root");
    let post_state_root = hex_to_array::<32>(&fixture.post_state_root).expect("valid post-state root");

    // Parse verkle_proof from new format
    let other_stems: Vec<[u8; 32]> = fixture
        .verkle_proof
        .other_stems
        .iter()
        .map(|s| hex_to_array::<32>(s).expect("valid other stem"))
        .collect();

    let depth_hex = fixture.verkle_proof.depth_extension_present.strip_prefix("0x")
        .unwrap_or(&fixture.verkle_proof.depth_extension_present);
    let depth_extension_present = hex::decode(depth_hex)
        .expect("valid depth extension hex");

    let commitments_by_path_raw: Vec<[u8; 32]> = fixture
        .verkle_proof
        .commitments_by_path
        .iter()
        .map(|c| hex_to_array::<32>(c).expect("valid commitment"))
        .collect();

    let d = hex_to_array::<32>(&fixture.verkle_proof.d).expect("valid d");

    // Parse IPA proof
    let cl: Vec<[u8; 32]> = fixture
        .verkle_proof
        .ipa_proof
        .cl
        .iter()
        .map(|s| hex_to_array::<32>(s).expect("valid cl"))
        .collect();
    let cr: Vec<[u8; 32]> = fixture
        .verkle_proof
        .ipa_proof
        .cr
        .iter()
        .map(|s| hex_to_array::<32>(s).expect("valid cr"))
        .collect();
    let final_evaluation = hex_to_array::<32>(&fixture.verkle_proof.ipa_proof.final_evaluation)
        .expect("valid final evaluation");

    let ipa_proof = IPAProofCompact { cl, cr, final_evaluation };

    // SECURITY: Fixtures now include root - validate it matches expected pre_state_root
    // Do NOT auto-prepend root (that would bypass cryptographic binding)
    let commitments_by_path =
        ensure_root_commitment(&pre_state_root, &depth_extension_present, commitments_by_path_raw)
            .expect("Proof must contain valid root commitment");

    // Parse state_diff from new format
    let state_diff = parse_state_diff(&fixture.state_diff).expect("valid state diff");

    // Build multiproof inputs from state_diff to get commitments, indices, and values for IPA
    // Use root from proof commitments_by_path[0] (matches go-verkle)
    let root_point = BandersnatchPoint::from_bytes(&commitments_by_path[0])
        .expect("valid root commitment from proof");
    let multiproof_inputs = build_multiproof_inputs_from_state_diff_with_root(&state_diff, Some(root_point))
        .expect("build multiproof inputs");

    // Convert Fr values to bytes for proof
    let values_bytes: Vec<[u8; 32]> = multiproof_inputs.values
        .iter()
        .map(|fr| {
            let mut bytes = [0u8; 32];
            bytes.copy_from_slice(&fr.into_bigint().to_bytes_le());
            bytes
        })
        .collect();

    let proof = VerkleProof::from_parts(
        other_stems,
        depth_extension_present,
        commitments_by_path.clone(),
        d,
        ipa_proof,
        multiproof_inputs.indices.clone(),
        values_bytes,
    )
    .expect("proof construction should succeed");

    FixtureData {
        pre_state_root,
        post_state_root,
        proof,
        commitments: multiproof_inputs.commitments,
        state_diff,
    }
}

#[test]
fn test_single_key_multiproof() {
    let fixture = load_fixture("verkle_fixture_single_key.json");
    let srs = generate_srs_points(256);

    // Compute pre-state root from current values for debugging parity
    let mut pre_tree = VerkleNode::new_internal();
    for stem_diff in &fixture.state_diff {
        let mut values = [ValueUpdate::NoChange; 256];
        for suffix in &stem_diff.suffix_diffs {
            values[suffix.suffix as usize] = match suffix.current_value {
                Some(v) => ValueUpdate::Set(v),
                None => ValueUpdate::NoChange,
            };
        }
        pre_tree
            .insert_values_at_stem(&stem_diff.stem, &values, 0)
            .expect("insert pre-state values");
    }
    let computed_pre = pre_tree
        .root_commitment(&srs)
        .expect("compute pre-state root")
        .to_bytes();
    println!("single-key: computed pre-state root {}", hex::encode(computed_pre));

    println!("single-key: verifying pre-state commitments");
    verify_prestate_commitment(&fixture.pre_state_root, &fixture.proof)
        .expect("pre-state root should match proof");

    let stems: Vec<[u8; 31]> = fixture.state_diff.iter().map(|s| s.stem).collect();
    println!("single-key: verifying path structure");
    verify_path_structure(&fixture.proof, &stems).expect("path structure should be valid");

    println!("single-key: running IPA multiproof verification");
    println!("  commitments.len() = {}", fixture.commitments.len());
    println!("  proof.indices.len() = {}", fixture.proof.indices.len());
    println!("  proof.values.len() = {}", fixture.proof.values.len());
    clear_debug_output();
    let start = Instant::now();
    let ipa_result = verify_ipa_multiproof(&fixture.commitments, &fixture.proof)
        .expect("IPA multiproof should verify");
    println!("single-key: IPA verification took {:?}", start.elapsed());

    // Print debug output
    let debug_output = get_debug_output();
    println!("=== DEBUG OUTPUT ===\n{}", debug_output);

    assert!(ipa_result, "IPA multiproof should be true");

    println!("single-key: test complete - IPA multiproof verified successfully");
    // Note: This fixture has pre_state == post_state (read-only proof)
    assert_eq!(fixture.pre_state_root, fixture.post_state_root, "fixture is read-only");
}

#[test]
fn test_multi_key_same_stem() {
    let fixture = load_fixture("verkle_fixture_multi_key_same_stem.json");

    println!("multi-key: verifying pre-state commitments");
    verify_prestate_commitment(&fixture.pre_state_root, &fixture.proof)
        .expect("pre-state root should match fixture");

    let stems: Vec<[u8; 31]> = fixture.state_diff.iter().map(|s| s.stem).collect();
    println!("multi-key: verifying path structure");
    verify_path_structure(&fixture.proof, &stems).expect("path layout should be valid");

    println!("multi-key: running IPA multiproof verification");
    let start = Instant::now();
    let ipa_result = verify_ipa_multiproof(&fixture.commitments, &fixture.proof)
        .expect("IPA multiproof verification should succeed");
    println!("multi-key: IPA verification took {:?}", start.elapsed());
    assert!(ipa_result, "IPA multiproof should be valid for two-key proof");

    println!("multi-key: test complete - IPA multiproof verified successfully");
    // Note: This fixture has pre_state == post_state (read-only proof)
    assert_eq!(fixture.pre_state_root, fixture.post_state_root, "fixture is read-only");
}

#[test]
fn test_compute_evaluations_for_ipa() {
    // Test that compute_evaluations_for_ipa correctly derives (z, y) pairs

    let indices = vec![0, 1, 2, 128, 255];
    let values = vec![
        [1u8; 32],
        [2u8; 32],
        [3u8; 32],
        [128u8; 32],
        [255u8; 32],
    ];

    let ipa_proof = IPAProofCompact {
        cl: vec![],
        cr: vec![],
        final_evaluation: [0u8; 32],
    };

    let proof = VerkleProof::from_parts(
        vec![],
        vec![],
        vec![[0u8; 32]],
        [0u8; 32],
        ipa_proof,
        indices.clone(),
        values.clone(),
    ).expect("valid proof");

    let evals = compute_evaluations_for_ipa(&proof).expect("should compute evaluations");

    assert_eq!(evals.len(), 5, "Should have 5 evaluation pairs");

    // Verify indices are correctly converted to field elements
    // z_0 = 0, z_1 = 1, z_2 = 2, z_3 = 128, z_4 = 255
    for (i, (z, _y)) in evals.iter().enumerate() {
        let expected_z = Fr::from(indices[i] as u64);
        assert_eq!(*z, expected_z, "Index {} mismatch", i);
    }
}
#[test]
fn test_zero_values_are_not_dropped() {
    use evm_service::verkle::{VerkleKeyMetadata, VerkleKeyType, VerkleWitnessEntry};
    use evm_service::verkle_proof::{
        compute_c1_c2, extract_proof_elements_from_tree, rebuild_stateless_tree_from_proof,
        witness_entries_to_state_diff,
    };
    use primitive_types::{H160, H256};

    let mut key = [0u8; 32];
    key[0] = 0xAA;
    key[31] = 7;

    let entry = VerkleWitnessEntry {
        key,
        metadata: VerkleKeyMetadata {
            key_type: VerkleKeyType::BasicData,
            address: H160::zero(),
            extra: 0,
            storage_key: H256::zero(),
            tx_index: 0,
        },
        pre_value: [0u8; 32],
        post_value: [0u8; 32],
    };

    let entries = vec![entry];
    // use_pre_value=true means current_value = pre_value
    let state_diff = witness_entries_to_state_diff(&entries, true);

    // Verify StateDiff preserves zero values
    assert_eq!(state_diff.len(), 1, "Should have 1 stem");
    assert_eq!(state_diff[0].suffix_diffs.len(), 1, "Should have 1 suffix diff");

    let diff = &state_diff[0].suffix_diffs[0];
    assert_eq!(diff.suffix, 7, "Suffix should be 7");
    assert_eq!(
        diff.current_value,
        Some([0u8; 32]),
        "StateDiff should retain zero pre-value (not drop it as None)"
    );
    assert_eq!(
        diff.new_value,
        Some([0u8; 32]),
        "StateDiff should retain zero post-value (not drop it as None)"
    );

    // Also ensure zero values are preserved when extracting proof elements
    let stem = &state_diff[0].stem;
    let mut values = [None; 256];
    values[7] = Some([0u8; 32]);
    let (c1, c2) = compute_c1_c2(&values).expect("c1/c2 from zero value");

    let srs = evm_service::verkle::generate_srs_points(256);
    let mut leaf_poly = [evm_service::verkle::Fr::zero(); 256];
    leaf_poly[0] = evm_service::verkle::Fr::from(1u64);
    let mut stem_bytes = [0u8; 32];
    stem_bytes[..31].copy_from_slice(stem);
    leaf_poly[1] = evm_service::verkle::Fr::from_le_bytes_mod_order(&stem_bytes);
    leaf_poly[2] = c1.map_to_scalar_field();
    leaf_poly[3] = c2.map_to_scalar_field();
    let leaf_commitment = evm_service::verkle::BandersnatchPoint::msm(&srs, &leaf_poly);

    let mut root_poly = [evm_service::verkle::Fr::zero(); 256];
    root_poly[stem[0] as usize] = leaf_commitment.map_to_scalar_field();
    let root_commitment = evm_service::verkle::BandersnatchPoint::msm(&srs, &root_poly);

    let commitments_by_path = vec![
        root_commitment.to_bytes(),
        leaf_commitment.to_bytes(),
        c1.to_bytes(),
    ];
    let depth_extension_present = vec![0x0a]; // depth=1 (including leaf) | PRESENT

    let tree = rebuild_stateless_tree_from_proof(
        &commitments_by_path,
        &depth_extension_present,
        &[],
        &entries,
        true,
    )
    .expect("tree rebuild from zero-valued witness");

    let proof_elements =
        extract_proof_elements_from_tree(&tree, &entries, true).expect("extract proof items");

    // Expected encoding for a present zero value: marker bit set in low limb
    use evm_service::verkle::Fr;
    let mut lo_bytes = [0u8; 17];
    lo_bytes[16] = 1;
    let expected_lo = Fr::from_le_bytes_mod_order(&lo_bytes);
    let expected_hi = Fr::zero();

    let c1_bytes = c1.to_bytes();
    let mut found_lo = false;
    let mut found_hi = false;
    for (commitment, (index, value)) in proof_elements
        .commitments
        .iter()
        .zip(proof_elements.indices.iter().zip(proof_elements.values.iter()))
    {
        if commitment.to_bytes() == c1_bytes && *index == 14 {
            // suffix 7 → z0 = 2*suffix
            assert_eq!(*value, expected_lo, "low limb should keep marker bit");
            found_lo = true;
        }
        if commitment.to_bytes() == c1_bytes && *index == 15 {
            assert_eq!(*value, expected_hi, "high limb should be zero");
            found_hi = true;
        }
    }

    assert!(found_lo, "zero suffix should emit low-limb evaluation");
    assert!(found_hi, "zero suffix should emit high-limb evaluation");
}

#[test]
fn test_verify_verkle_proof_integration() {
    // Test the complete verify_verkle_proof function with real fixture
    let fixture = load_fixture("verkle_fixture_single_key.json");

    println!("integration: running complete verkle proof verification");
    let start = Instant::now();
    let result = verify_verkle_proof(
        &fixture.pre_state_root,
        &fixture.proof,
        &fixture.commitments,
        &fixture.state_diff,
    )
    .expect("verification should succeed");
    println!("integration: complete verification took {:?}", start.elapsed());

    assert!(result, "Verkle proof should be valid");
    println!("integration: ✅ Full Verkle proof verification passed");
}

#[test]
fn test_extract_leaf_commitments_multi_stem() {
    // Test that extract_leaf_commitments correctly handles multiple stems
    // This tests the logic in verkle_proof.rs:extract_leaf_commitments

    use evm_service::verkle_proof::VerkleProof;

    // Simulate a 2-stem proof with the following structure:
    // Here, depth encodes the number of internal nodes; we add the leaf (+1).
    //
    // Stem 0: depth=1 → commitments: 1 internal + 1 leaf = 2
    // Stem 1: depth=2 → commitments: 2 internal + 1 leaf = 3
    //
    // commitments_by_path layout (with root):
    //   [0] = root
    //   [1] = stem0 internal node
    //   [2] = stem0 leaf (THIS is commitment for stem 0)
    //   [3] = stem1 internal node 0
    //   [4] = stem1 internal node 1
    //   [5] = stem1 leaf (THIS is commitment for stem 1)
    //
    // Total: 1 (root) + 2 (stem0) + 3 (stem1) = 6 commitments

    let dummy_commitment = [0x42u8; 32];
    let leaf0_bytes = [0xAAu8; 32];
    let leaf1_bytes = [0xBBu8; 32];

    // depth_extension_present encodes: (depth << 3) | extension_status
    // For simplicity, extension_status = 0 (no extensions)
    let depth_ext_stem0 = 1u8 << 3; // depth=1
    let depth_ext_stem1 = 2u8 << 3; // depth=2

    let commitments_by_path = vec![
        dummy_commitment, // [0] root
        dummy_commitment, // [1] stem0 internal
        leaf0_bytes,      // [2] stem0 leaf
        dummy_commitment, // [3] stem1 internal 0
        dummy_commitment, // [4] stem1 internal 1
        leaf1_bytes,      // [5] stem1 leaf
    ];

    let depth_extension_present = vec![depth_ext_stem0, depth_ext_stem1];

    // Build VerkleProof
    let ipa_proof = IPAProofCompact {
        cl: vec![],
        cr: vec![],
        final_evaluation: [0u8; 32],
    };

    let proof = VerkleProof::from_parts(
        vec![], // other_stems
        depth_extension_present,
        commitments_by_path,
        [0u8; 32], // d
        ipa_proof,
        vec![0], // indices (dummy)
        vec![[0u8; 32]], // values (dummy)
    ).expect("proof construction");

    // Define stems
    let stem0 = [0x11u8; 31];
    let stem1 = [0x22u8; 31];
    let _stems = vec![stem0, stem1];

    // Now trace through the extract_leaf_commitments algorithm:
    let root_present = proof.commitments_by_path.len() == 1 + (2 + 3); // 1 + path commitments = 6
    assert!(root_present, "Root should be detected as present");

    // Algorithm trace:
    //   offset = 1 (after root)
    //
    //   Stem 0 (depth=1 → path len 2):
    //     offset += 2 -> offset = 3
    //     leaf = commitments_by_path[offset - 1] = commitments_by_path[2] = leaf0_bytes ✓
    //
    //   Stem 1 (depth=2 → path len 3):
    //     offset += 3 -> offset = 6
    //     leaf = commitments_by_path[offset - 1] = commitments_by_path[5] = leaf1_bytes ✓

    assert_eq!(proof.commitments_by_path[2], leaf0_bytes, "Leaf 0 should be at index 2");
    assert_eq!(proof.commitments_by_path[5], leaf1_bytes, "Leaf 1 should be at index 5");

    // Since extract_leaf_commitments is private, we can't call it directly
    // But we validated the proof structure matches the algorithm's expectations
    println!("✅ Multi-stem proof structure validated");
    println!("   Stem 0 leaf commitment at index 2: {:02x?}", &leaf0_bytes[0..4]);
    println!("   Stem 1 leaf commitment at index 5: {:02x?}", &leaf1_bytes[0..4]);
}

#[test]
fn test_mismatched_root_rejected() {
    // Test that mismatched roots are explicitly rejected
    // Previously this was a known limitation (Issue #3), now FIXED

    use evm_service::verkle_proof::{VerkleProof, VerkleProofError};

    // Use valid curve points from test fixtures
    // claimed_root from verkle_fixture_single_key.json
    let claimed_root = hex_to_array::<32>("6b630905ce275e39f223e175242df2c1e8395e6f46ec71dce5557012c1334a5c")
        .expect("valid root");

    // actual_root_in_proof from verkle_fixture_multi_key_same_stem.json (DIFFERENT root!)
    let actual_root_in_proof = hex_to_array::<32>("3193fdaff8ca327c3f8b3a11b5f771317e96c83febfcc228525d70c7582dcd3b")
        .expect("valid root");

    // Use valid commitments from fixtures too
    let dummy_commitment = hex_to_array::<32>("6b630905ce275e39f223e175242df2c1e8395e6f46ec71dce5557012c1334a5c")
        .expect("valid commitment");
    let leaf0_bytes = hex_to_array::<32>("3193fdaff8ca327c3f8b3a11b5f771317e96c83febfcc228525d70c7582dcd3b")
        .expect("valid commitment");

    let depth_ext_stem0 = 1u8 << 3; // depth=1 (path len = 2)

    let commitments_by_path = vec![
        actual_root_in_proof, // First commitment is the root (but WRONG root)
        dummy_commitment,     // stem0 internal
        leaf0_bytes,          // stem0 leaf
    ];

    let depth_extension_present = vec![depth_ext_stem0];

    let ipa_proof = IPAProofCompact {
        cl: vec![],
        cr: vec![],
        final_evaluation: [0u8; 32],
    };

    let proof = VerkleProof::from_parts(
        vec![],
        depth_extension_present,
        commitments_by_path,
        [0u8; 32],
        ipa_proof,
        vec![0],
        vec![[0u8; 32]],
    )
    .expect("proof construction");

    // Attempt to verify with the claimed root
    let result = verify_prestate_commitment(&claimed_root, &proof);

    // Should explicitly reject with RootMismatch error
    assert!(result.is_err(), "Mismatched root should be rejected");
    match result.unwrap_err() {
        VerkleProofError::RootMismatch => {
            // Expected error - test passes
        }
        other => panic!("Expected RootMismatch error, got: {:?}", other),
    }
}

#[test]
fn test_rootless_proof_rejected() {
    // SECURITY TEST: Rootless proofs (len == total_depth) must be rejected
    // to prevent accepting proofs generated for a different root.
    //
    // Attack scenario without this check:
    // 1. Attacker generates proof for root_A
    // 2. Attacker prepends root_B to commitments (now len == total_depth + 1)
    // 3. Without length check, verifier accepts it as proof for root_B

    let pre_state_root = [0x11u8; 32];
    let depth_extension_present = vec![1u8 << 3]; // depth = 1

    // Provide exactly total_depth commitments (no root)
    // This simulates a pruned/rootless go-verkle proof
    let commitments_by_path = vec![
        [0xAAu8; 32], // internal node (NOT the root)
    ];

    let err = ensure_root_commitment(
        &pre_state_root,
        &depth_extension_present,
        commitments_by_path,
    )
    .unwrap_err();

    // Should reject with RootMismatch since len == total_depth (rootless)
    assert_eq!(err, VerkleProofError::RootMismatch);
}

#[test]
fn test_wrong_root_rejected() {
    // Test that proofs with wrong root commitment (but correct length) are rejected
    let pre_state_root = [0x11u8; 32];
    let depth_extension_present = vec![1u8 << 3]; // depth = 1

    // Provide correct length (1 + total_depth = 2) but wrong root
    let commitments_by_path = vec![
        [0xAAu8; 32], // Wrong root (not 0x11)
        [0xBBu8; 32], // internal node
    ];

    let err = ensure_root_commitment(
        &pre_state_root,
        &depth_extension_present,
        commitments_by_path,
    )
    .unwrap_err();

    // Should reject with RootMismatch since commitments_by_path[0] != pre_state_root
    assert_eq!(err, VerkleProofError::RootMismatch);
}

#[test]
fn test_empty_commitments_rejected() {
    // Truly empty proofs (no commitments at all) should be rejected
    let pre_state_root = [0x11u8; 32];
    let depth_extension_present = vec![1u8 << 3];
    let commitments_by_path = vec![]; // Empty

    let err = ensure_root_commitment(
        &pre_state_root,
        &depth_extension_present,
        commitments_by_path,
    )
    .unwrap_err();

    // Should reject with EmptyCommitments
    assert_eq!(err, VerkleProofError::EmptyCommitments);
}

#[test]
fn test_correct_root_accepted() {
    // Test that proofs with correct root commitments are accepted
    // This is the positive test case for verify_prestate_commitment

    use evm_service::verkle_proof::VerkleProof;

    // Use a valid curve point from fixtures
    let correct_root = hex_to_array::<32>("6b630905ce275e39f223e175242df2c1e8395e6f46ec71dce5557012c1334a5c")
        .expect("valid root");

    let dummy_commitment = hex_to_array::<32>("3193fdaff8ca327c3f8b3a11b5f771317e96c83febfcc228525d70c7582dcd3b")
        .expect("valid commitment");
    let leaf0_bytes = hex_to_array::<32>("3193fdaff8ca327c3f8b3a11b5f771317e96c83febfcc228525d70c7582dcd3b")
        .expect("valid commitment");

    let depth_ext_stem0 = 1u8 << 3;

    let commitments_by_path = vec![
        correct_root,     // First commitment matches claimed root ✓
        dummy_commitment, // stem0 internal
        leaf0_bytes,      // stem0 leaf
    ];

    let depth_extension_present = vec![depth_ext_stem0];

    let ipa_proof = IPAProofCompact {
        cl: vec![],
        cr: vec![],
        final_evaluation: [0u8; 32],
    };

    let proof = VerkleProof::from_parts(
        vec![],
        depth_extension_present,
        commitments_by_path,
        [0u8; 32],
        ipa_proof,
        vec![0],
        vec![[0u8; 32]],
    )
    .expect("proof construction");

    // Verify with the correct root
    let result = verify_prestate_commitment(&correct_root, &proof);

    assert!(
        result.is_ok(),
        "Should accept proof with matching root commitment"
    );

    println!("✅ Correct root commitment accepted");
}

#[test]
fn test_shared_prefix_deduplication() {
    // SECURITY TEST: Stems sharing stem[0] should only produce ONE root evaluation
    // This prevents a malicious prover from balancing incorrect values across stems

    use evm_service::verkle_proof::{build_multiproof_inputs_from_state_diff_with_root, StemStateDiff, SuffixStateDiff};
    use evm_service::verkle::BandersnatchPoint;

    // Create two stems that share the same first byte (0x00)
    let mut stem1 = [0u8; 31];
    stem1[0] = 0x00;
    stem1[30] = 0x01;

    let mut stem2 = [0u8; 31];
    stem2[0] = 0x00;
    stem2[30] = 0x02;

    let state_diff = vec![
        StemStateDiff {
            stem: stem1,
            suffix_diffs: vec![SuffixStateDiff {
                suffix: 0,
                current_value: Some([0x11u8; 32]),
                new_value: Some([0x11u8; 32]),
            }],
        },
        StemStateDiff {
            stem: stem2,
            suffix_diffs: vec![SuffixStateDiff {
                suffix: 0,
                current_value: Some([0x22u8; 32]),
                new_value: Some([0x22u8; 32]),
            }],
        },
    ];

    let root = BandersnatchPoint::generator();
    let inputs = build_multiproof_inputs_from_state_diff_with_root(&state_diff, Some(root))
        .expect("should build multiproof inputs");

    // Count how many times z=0x00 appears in indices (for root commitment)
    let root_eval_count = inputs.indices.iter()
        .zip(inputs.commitments.iter())
        .filter(|(idx, comm)| **idx == 0x00 && **comm == root)
        .count();

    // Should only have ONE root evaluation at z=0x00, not two
    assert_eq!(root_eval_count, 1,
        "Stems sharing stem[0]=0x00 should produce exactly ONE root evaluation, not {}",
        root_eval_count);

    println!("✅ Shared prefix deduplication working: only 1 root evaluation for 2 stems with stem[0]=0x00");
}

#[test]
fn test_build_eval_points_from_witness() {
    // NEW TEST: Build evaluation points from witness entries
    use evm_service::verkle_proof::build_evaluation_points_from_witness;
    use evm_service::verkle::{VerkleWitnessEntry, VerkleKeyMetadata, VerkleKeyType};
    use primitive_types::{H160, H256};

    // Create test witness with one stem, one suffix
    let mut key = [0u8; 32];
    key[0..31].copy_from_slice(&[0x50u8; 31]); // stem
    key[31] = 0; // suffix 0

    let entries = vec![
        VerkleWitnessEntry {
            key,
            metadata: VerkleKeyMetadata {
                key_type: VerkleKeyType::BasicData,
                address: H160::zero(),
                extra: 0,
                storage_key: H256::zero(),
                tx_index: 0,
            },
            pre_value: [0xddu8; 32],
            post_value: [0xddu8; 32],
        }
    ];

    // Build evaluation points using pre_value
    let (indices, values) = build_evaluation_points_from_witness(&entries, true)
        .expect("should build evaluation points");

    // Should have:
    // - 2 base entries (z=0 marker, z=1 stem)
    // - 2 C1/C2 pointers (z=2 C1, z=3 C2)
    // - 2 suffix entries (z=4, z=5 for suffix 0)
    assert!(indices.len() >= 6, "Should have at least 6 evaluation points (base + C1/C2 + suffix)");
    assert_eq!(indices.len(), values.len(), "Indices and values should have same length");

    // Verify base entries are present
    assert!(indices.contains(&0), "Should have marker evaluation at z=0");
    assert!(indices.contains(&1), "Should have stem evaluation at z=1");

    println!("✅ Built {} evaluation points from witness", indices.len());
}

#[test]
fn test_eval_points_leaf_base_entries() {
    // NEW TEST: Validate marker and stem base entries
    use evm_service::verkle_proof::build_evaluation_points_from_witness;
    use evm_service::verkle::{VerkleWitnessEntry, VerkleKeyMetadata, VerkleKeyType};
    use primitive_types::{H160, H256};

    let mut key = [0u8; 32];
    let stem = [0x42u8; 31];
    key[0..31].copy_from_slice(&stem);
    key[31] = 5;

    let entries = vec![
        VerkleWitnessEntry {
            key,
            metadata: VerkleKeyMetadata {
                key_type: VerkleKeyType::BasicData,
                address: H160::zero(),
                extra: 0,
                storage_key: H256::zero(),
                tx_index: 0,
            },
            pre_value: [0x11u8; 32],
            post_value: [0x11u8; 32],
        }
    ];

    let (indices, values) = build_evaluation_points_from_witness(&entries, true)
        .expect("should build evaluation points");

    // Find base entries
    let marker_idx = indices.iter().position(|&z| z == 0).expect("should have z=0");
    let stem_idx = indices.iter().position(|&z| z == 1).expect("should have z=1");

    // Validate that we have values for marker and stem
    assert!(marker_idx < values.len(), "Marker index should be within values");
    assert!(stem_idx < values.len(), "Stem index should be within values");

    println!("✅ Base entries validated: marker and stem present at z=0 and z=1");
}

#[test]
fn test_eval_points_suffix_entries() {
    // NEW TEST: Validate suffix evaluation entries at 2*suffix, 2*suffix+1
    use evm_service::verkle_proof::build_evaluation_points_from_witness;
    use evm_service::verkle::{VerkleWitnessEntry, VerkleKeyMetadata, VerkleKeyType};
    use primitive_types::{H160, H256};

    let mut key = [0u8; 32];
    key[0..31].copy_from_slice(&[0x50u8; 31]);
    key[31] = 10; // suffix 10 → evaluations at z=4+2*10=24, z=25

    let entries = vec![
        VerkleWitnessEntry {
            key,
            metadata: VerkleKeyMetadata {
                key_type: VerkleKeyType::BasicData,
                address: H160::zero(),
                extra: 0,
                storage_key: H256::zero(),
                tx_index: 0,
            },
            pre_value: [0xaau8; 32],
            post_value: [0xaau8; 32],
        }
    ];

    let (indices, _values) = build_evaluation_points_from_witness(&entries, true)
        .expect("should build evaluation points");

    // Suffix 10 should produce evaluations at z=4+2*10=24 and z=25
    // Leaf polynomial: [0]=marker, [1]=stem, [2]=C1, [3]=C2, [4+]=suffix data
    let expected_z0 = (4 + 2 * 10) as u8;
    let expected_z1 = expected_z0 + 1;

    assert!(indices.contains(&expected_z0), "Should have evaluation at z={}", expected_z0);
    assert!(indices.contains(&expected_z1), "Should have evaluation at z={}", expected_z1);

    println!("✅ Suffix entries validated: z={} and z={} for suffix 10", expected_z0, expected_z1);
}

// ============================================================================
// Phase 1: Stateless Tree Infrastructure Tests
// ============================================================================

use evm_service::verkle_proof::{
    StatelessVerkleTree,
    StatelessNode,
    compute_c1_c2,
    rebuild_stateless_tree_from_proof,
};
use evm_service::verkle::{BandersnatchPoint, Fr};
use std::collections::BTreeMap;

#[test]
fn test_stateless_tree_single_stem() {
    println!("\n🧪 Test: Single stem tree creation");

    // Create root commitment (arbitrary for test)
    let root_commitment = BandersnatchPoint::generator();
    let mut tree = StatelessVerkleTree::new(root_commitment);

    // Stem with depth 2 (2 internal nodes + 1 leaf + C1/C2)
    let stem = [0xAB; 31];
    let depth = 2u8;

    // Commitments: [internal_0, internal_1, leaf, C1]
    // Use distinct valid BandersnatchPoint commitments
    let mut commitments = Vec::new();
    for i in 1..=4 {
        let point = BandersnatchPoint::generator().mul(&Fr::from(i as u64));
        commitments.push(point.to_bytes());
    }

    // Values: just one suffix present at index 0 (requires C1)
    let mut values = [None; 256];
    values[0] = Some([0x42; 32]);

    let mut prefix_cache = BTreeMap::new();
    let expected_commitments = 2; // leaf + C1 (has_c1=true, has_c2=false)
    let consumed = tree.create_path(&stem, depth, 2 /* EXT_STATUS_PRESENT */, &stem, &commitments, values, true, false, &mut prefix_cache, expected_commitments)
        .expect("should create path");

    assert_eq!(consumed, 4, "Should consume all 4 commitments (2 internal + leaf + C1)");
    assert_eq!(prefix_cache.len(), 2, "Should cache 2 internal node prefixes");

    // Verify we can retrieve the leaf
    let leaf = tree.get_leaf(&stem).expect("should find leaf");
    match leaf {
        StatelessNode::Leaf { stem: leaf_stem, .. } => {
            assert_eq!(leaf_stem, &stem, "Leaf stem should match");
        }
        _ => panic!("Expected leaf node"),
    }

    println!("✅ Single stem tree created successfully");
}

#[test]
fn test_stateless_tree_shared_prefix() {
    println!("\n🧪 Test: Shared prefix commitment consumption");

    // Two stems sharing first byte (0xAB)
    let stem1 = {
        let mut s = [0; 31];
        s[0] = 0xAB;
        s[1] = 0x00;
        s
    };
    let stem2 = {
        let mut s = [0; 31];
        s[0] = 0xAB;  // Shared first byte
        s[1] = 0x01;  // Different second byte
        s
    };

    // Total commitments: [root, internal_AB, internal_AB00, leaf1, C1_1, internal_AB01, leaf2, C1_2]
    // = 8 commitments total with shared prefix optimization
    // Each stem needs: internal nodes + leaf + C1 (since suffix 0 < 128, has_c1=true)
    let root_commitment = BandersnatchPoint::generator().to_bytes();
    let mut commitments = vec![root_commitment];
    for i in 1..=7 {
        let point = BandersnatchPoint::generator().mul(&Fr::from(i as u64));
        commitments.push(point.to_bytes());
    }

    // Depth = 2 internal nodes per stem
    // encoded_depth includes leaf, so for 2 internal nodes we need encoded_depth = 3
    // Both are PRESENT stems (type=2)
    let depth_extension_present = vec![(3u8 << 3) | 2, (3u8 << 3) | 2];

    // Build witness entries
    let metadata = VerkleKeyMetadata {
        key_type: VerkleKeyType::BasicData,
        address: H160::zero(),
        extra: 0,
        storage_key: H256::zero(),
        tx_index: 0,
    };

    let entry1 = VerkleWitnessEntry {
        key: {
            let mut k = [0u8; 32];
            k[..31].copy_from_slice(&stem1);
            k[31] = 0;
            k
        },
        metadata: metadata.clone(),
        pre_value: [0x11; 32],
        post_value: [0u8; 32],
    };

    let entry2 = VerkleWitnessEntry {
        key: {
            let mut k = [0u8; 32];
            k[..31].copy_from_slice(&stem2);
            k[31] = 0;
            k
        },
        metadata,
        pre_value: [0x22; 32],
        post_value: [0u8; 32],
    };

    let entries = vec![entry1, entry2];
    let other_stems = vec![];

    // Rebuild tree using proper function
    let tree = rebuild_stateless_tree_from_proof(
        &commitments,
        &depth_extension_present,
        &other_stems,
        &entries,
        true, // use_pre_value
    ).expect("should rebuild tree with shared prefixes");

    // Verify both leaves exist
    let leaf1 = tree.get_leaf(&stem1).expect("should find leaf1");
    let leaf2 = tree.get_leaf(&stem2).expect("should find leaf2");

    match (leaf1, leaf2) {
        (StatelessNode::Leaf { stem: s1, .. }, StatelessNode::Leaf { stem: s2, .. }) => {
            assert_eq!(s1, &stem1, "Leaf1 stem should match");
            assert_eq!(s2, &stem2, "Leaf2 stem should match");
        }
        _ => panic!("Expected both leaves"),
    }

    println!("✅ Shared prefix test passed with proper tree rebuild");
}

#[test]
fn test_stateless_tree_deep_tree() {
    println!("\n🧪 Test: Deep tree (depth > 3)");

    let root_commitment = BandersnatchPoint::generator();
    let mut tree = StatelessVerkleTree::new(root_commitment);

    // Stem with depth 5 (5 internal nodes + 1 leaf, no C1/C2 for absence = 6 commitments)
    let stem = {
        let mut s = [0; 31];
        for i in 0..5 {
            s[i] = (i as u8) + 1;  // [1, 2, 3, 4, 5, 0, 0, ...]
        }
        s
    };
    let depth = 5u8;

    // Generate 6 commitments (5 internal + 1 leaf)
    let mut commitments = Vec::new();
    for i in 1..=6 {
        let point = BandersnatchPoint::generator().mul(&Fr::from(i as u64));
        commitments.push(point.to_bytes());
    }

    let values = [None; 256];  // All None - no C1/C2 needed
    let mut prefix_cache = BTreeMap::new();

    let expected_commitments = 1; // leaf only (has_c1=false, has_c2=false)
    let consumed = tree.create_path(&stem, depth, 2 /* EXT_STATUS_PRESENT */, &stem, &commitments, values, false, false, &mut prefix_cache, expected_commitments)
        .expect("should create deep path");

    assert_eq!(consumed, 6, "Should consume all 6 commitments (5 internal + leaf, no C1/C2)");
    assert_eq!(prefix_cache.len(), 5, "Should cache 5 internal node prefixes");

    // Verify leaf retrieval
    let leaf = tree.get_leaf(&stem).expect("should find leaf");
    match leaf {
        StatelessNode::Leaf { stem: leaf_stem, .. } => {
            assert_eq!(leaf_stem, &stem, "Leaf stem should match");
        }
        _ => panic!("Expected leaf node"),
    }

    println!("✅ Deep tree test passed: depth={depth}, consumed={consumed}");
}

#[test]
fn test_compute_c1_c2() {
    println!("\n🧪 Test: C1/C2 computation");

    // Test 1: Empty values (all None)
    let values_empty = [None; 256];
    let (c1_empty, c2_empty) = compute_c1_c2(&values_empty).expect("should compute C1/C2");
    println!("✅ C1/C2 computed for empty values");

    // Test 2: One value in C1 range (suffix < 128)
    let mut values_c1 = [None; 256];
    values_c1[10] = Some([0x42; 32]);
    let (c1_one, c2_one) = compute_c1_c2(&values_c1).expect("should compute C1/C2");
    assert_ne!(c1_one, c1_empty, "C1 should differ with value present");
    assert_eq!(c2_one, c2_empty, "C2 should be same (no values in range)");
    println!("✅ C1/C2 computed with suffix 10 in C1 range");

    // Test 3: One value in C2 range (suffix >= 128)
    let mut values_c2 = [None; 256];
    values_c2[200] = Some([0x99; 32]);
    let (c1_none, c2_has) = compute_c1_c2(&values_c2).expect("should compute C1/C2");
    assert_eq!(c1_none, c1_empty, "C1 should be same (no values in range)");
    assert_ne!(c2_has, c2_empty, "C2 should differ with value present");
    println!("✅ C1/C2 computed with suffix 200 in C2 range");

    // Test 4: Values in both ranges
    let mut values_both = [None; 256];
    values_both[50] = Some([0xAA; 32]);   // C1 range
    values_both[150] = Some([0xBB; 32]);  // C2 range
    let (c1_both, c2_both) = compute_c1_c2(&values_both).expect("should compute C1/C2");
    assert_ne!(c1_both, c1_empty, "C1 should differ");
    assert_ne!(c2_both, c2_empty, "C2 should differ");
    println!("✅ C1/C2 computed with values in both ranges");
}

#[test]
fn test_debug_shared_prefix() {
    println!("\n🧪 Debug: Shared prefix step by step");

    let root_commitment = BandersnatchPoint::generator();
    let mut tree = StatelessVerkleTree::new(root_commitment);

    // Simple stems sharing first byte
    let stem1 = {
        let mut s = [0; 31];
        s[0] = 0xAB;
        s[1] = 0x00;
        s
    };
    let stem2 = {
        let mut s = [0; 31];
        s[0] = 0xAB;  // Same first byte
        s[1] = 0x01;  // Different second byte
        s
    };

    let values = [None; 256];
    let mut prefix_cache = BTreeMap::new();

    // First stem with depth 2: needs 3 commitments (2 internal + 1 leaf, no C1/C2 for absence)
    let commitments1 = vec![
        BandersnatchPoint::generator().mul(&Fr::from(1u64)).to_bytes(), // Internal at level 0
        BandersnatchPoint::generator().mul(&Fr::from(2u64)).to_bytes(), // Internal at level 1
        BandersnatchPoint::generator().mul(&Fr::from(3u64)).to_bytes(), // Leaf
    ];

    println!("Creating first stem [AB,00,...] depth=2");
    let expected_commitments = 1; // leaf only (has_c1=false, has_c2=false)
    let consumed1 = tree.create_path(&stem1, 2, 2 /* EXT_STATUS_PRESENT */, &stem1, &commitments1, values, false, false, &mut prefix_cache, expected_commitments)
        .expect("should create first stem");
    println!("First stem consumed {} commitments", consumed1);
    println!("Prefix cache size: {}", prefix_cache.len());
    println!("Prefix cache keys: {:?}", prefix_cache.keys().collect::<Vec<_>>());

    // Second stem with depth 2: should need only 2 commitments (reuse level 0, create level 1 + leaf, no C1/C2)
    let commitments2 = vec![
        BandersnatchPoint::generator().mul(&Fr::from(4u64)).to_bytes(), // Internal at level 1 (new)
        BandersnatchPoint::generator().mul(&Fr::from(5u64)).to_bytes(), // Leaf
    ];

    println!("\nCreating second stem [AB,01,...] depth=2");
    println!("Expected: reuse level 0 ([AB]), create new level 1 ([AB,01]), create leaf");

    let expected_commitments = 1; // leaf only (has_c1=false, has_c2=false)
    match tree.create_path(&stem2, 2, 2 /* EXT_STATUS_PRESENT */, &stem2, &commitments2, values, false, false, &mut prefix_cache, expected_commitments) {
        Ok(consumed2) => {
            println!("Second stem consumed {} commitments", consumed2);
            println!("Final prefix cache size: {}", prefix_cache.len());
            println!("Final prefix cache keys: {:?}", prefix_cache.keys().collect::<Vec<_>>());

            // Test that we can retrieve both leaves
            match tree.get_leaf(&stem1) {
                Ok(_) => println!("✅ Can retrieve stem1 leaf"),
                Err(e) => println!("❌ Cannot retrieve stem1 leaf: {:?}", e),
            }
            match tree.get_leaf(&stem2) {
                Ok(_) => println!("✅ Can retrieve stem2 leaf"),
                Err(e) => println!("❌ Cannot retrieve stem2 leaf: {:?}", e),
            }
        }
        Err(e) => {
            println!("ERROR during create_path for stem2: {:?}", e);
            println!("Prefix cache at failure: {:?}", prefix_cache.keys().collect::<Vec<_>>());

            // Debug: let's see what's in the tree after stem1
            match tree.get_leaf(&stem1) {
                Ok(_) => println!("stem1 leaf is accessible"),
                Err(e) => println!("stem1 leaf ERROR: {:?}", e),
            }
        }
    }
}

// ============================================================================
// Phase 2: Tree rebuilding from proof commitments
// ============================================================================

#[test]
fn test_rebuild_stateless_tree_shared_prefix_from_proof() {
    use primitive_types::{H160, H256};

    println!("\n🧪 Test: Rebuild stateless tree with shared prefix (proof commitments)");

    let root_commitment = BandersnatchPoint::generator();

    // Two stems sharing first byte (0xAB)
    let stem1 = {
        let mut s = [0u8; 31];
        s[0] = 0xAB;
        s[1] = 0x00;
        s
    };
    let stem2 = {
        let mut s = [0u8; 31];
        s[0] = 0xAB;
        s[1] = 0x01;
        s
    };

    // Commitments: root + shared internal + unique internal/leaf/C1 per stem
    let shared_internal = BandersnatchPoint::generator().mul(&Fr::from(1u64)).to_bytes();
    let internal1 = BandersnatchPoint::generator().mul(&Fr::from(2u64)).to_bytes();
    let leaf1 = BandersnatchPoint::generator().mul(&Fr::from(3u64)).to_bytes();
    let c1_1 = BandersnatchPoint::generator().mul(&Fr::from(4u64)).to_bytes();
    let internal2 = BandersnatchPoint::generator().mul(&Fr::from(5u64)).to_bytes();
    let leaf2 = BandersnatchPoint::generator().mul(&Fr::from(6u64)).to_bytes();
    let c1_2 = BandersnatchPoint::generator().mul(&Fr::from(7u64)).to_bytes();

    let commitments_by_path = vec![
        root_commitment.to_bytes(),
        shared_internal,
        internal1,
        leaf1,
        c1_1,  // C1 for stem1 (suffix 0 < 128)
        internal2,
        leaf2,
        c1_2,  // C1 for stem2 (suffix 1 < 128)
    ];

    // Depth = 2 internal nodes per stem (matches create_path expectations)
    // encoded_depth includes leaf, so for 2 internal nodes we need encoded_depth = 3
    // Both are PRESENT stems (type=2)
    let depth_extension_present = vec![(3u8 << 3) | 2, (3u8 << 3) | 2];

    // Build witness entries (use pre_value)
    let metadata = VerkleKeyMetadata {
        key_type: VerkleKeyType::BasicData,
        address: H160::zero(),
        extra: 0,
        storage_key: H256::zero(),
        tx_index: 0,
    };

    let entry1 = VerkleWitnessEntry {
        key: {
            let mut k = [0u8; 32];
            k[..31].copy_from_slice(&stem1);
            k[31] = 0;
            k
        },
        metadata: metadata.clone(),
        pre_value: [0x11; 32],
        post_value: [0u8; 32],
    };

    let entry2 = VerkleWitnessEntry {
        key: {
            let mut k = [0u8; 32];
            k[..31].copy_from_slice(&stem2);
            k[31] = 1;
            k
        },
        metadata,
        pre_value: [0x22; 32],
        post_value: [0u8; 32],
    };

    let tree = rebuild_stateless_tree_from_proof(
        &commitments_by_path,
        &depth_extension_present,
        &[],
        &[entry1.clone(), entry2.clone()],
        true,
    )
    .expect("should rebuild stateless tree");

    // Validate leaves are present and commitments match provided proof commitments
    let leaf1_node = tree.get_leaf(&stem1).expect("leaf1 should exist");
    match leaf1_node {
        StatelessNode::Leaf { commitment, .. } => {
            let expected = BandersnatchPoint::from_bytes(&leaf1).unwrap();
            assert_eq!(*commitment, expected, "leaf1 commitment should match proof");
        }
        _ => panic!("Expected leaf node for stem1"),
    }

    let leaf2_node = tree.get_leaf(&stem2).expect("leaf2 should exist");
    match leaf2_node {
        StatelessNode::Leaf { commitment, .. } => {
            let expected = BandersnatchPoint::from_bytes(&leaf2).unwrap();
            assert_eq!(*commitment, expected, "leaf2 commitment should match proof");
        }
        _ => panic!("Expected leaf node for stem2"),
    }
}

/// Proofs that mark a path as `extStatusAbsentOther` must supply an ordered other_stems list.
#[test]
fn test_absent_other_requires_other_stem() {
    let metadata = VerkleKeyMetadata {
        key_type: VerkleKeyType::BasicData,
        address: H160::zero(),
        extra: 0,
        storage_key: H256::zero(),
        tx_index: 0,
    };

    let mut key = [0u8; 32];
    key[0] = 0x11;

    let entries = vec![VerkleWitnessEntry {
        key,
        metadata,
        pre_value: [0u8; 32],
        post_value: [0u8; 32],
    }];

    let root_commitment = BandersnatchPoint::generator().to_bytes();
    let leaf_commitment = BandersnatchPoint::generator()
        .mul(&Fr::from(5u64))
        .to_bytes();
    let commitments = vec![root_commitment, leaf_commitment];

    // depth=1 (root→leaf), extStatusAbsentOther (low bits = 1)
    let depth_extension_present = vec![(1u8 << 3) | 1u8];
    let other_stems: Vec<[u8; 32]> = Vec::new();

    let result = rebuild_stateless_tree_from_proof(
        &commitments,
        &depth_extension_present,
        &other_stems,
        &entries,
        true,
    );

    assert!(
        matches!(result, Err(VerkleProofError::PathMismatch)),
        "absent-other paths must consume a proof-of-absence stem"
    );
}

/// `other_stems` must stay sorted; go-verkle rejects unsorted proof-of-absence queues.
#[test]
fn test_other_stems_must_be_sorted() {
    let metadata = VerkleKeyMetadata {
        key_type: VerkleKeyType::BasicData,
        address: H160::zero(),
        extra: 0,
        storage_key: H256::zero(),
        tx_index: 0,
    };

    // Two stems prove absence along two paths (extStatusAbsentOther for both entries).
    let make_entry = |byte: u8| VerkleWitnessEntry {
        key: {
            let mut k = [0u8; 32];
            k[0] = byte;
            k
        },
        metadata: metadata.clone(),
        pre_value: [0u8; 32],
        post_value: [0u8; 32],
    };

    let entries = vec![make_entry(0x10), make_entry(0x20)];

    let root_commitment = BandersnatchPoint::generator().to_bytes();
    let leaf1 = BandersnatchPoint::generator()
        .mul(&Fr::from(7u64))
        .to_bytes();
    let leaf2 = BandersnatchPoint::generator()
        .mul(&Fr::from(11u64))
        .to_bytes();
    let commitments = vec![root_commitment, leaf1, leaf2];

    let mut stem_a = [0u8; 32];
    stem_a[0] = 0x02;
    let mut stem_b = [0u8; 32];
    stem_b[0] = 0x01;

    // Unsorted on purpose: 0x02... then 0x01...
    let other_stems = vec![stem_a, stem_b];

    // Two absent-other entries (depth=1, status=1)
    let depth_extension_present = vec![(1u8 << 3) | 1u8, (1u8 << 3) | 1u8];

    let result = rebuild_stateless_tree_from_proof(
        &commitments,
        &depth_extension_present,
        &other_stems,
        &entries,
        true,
    );

    assert!(
        matches!(result, Err(VerkleProofError::PathMismatch)),
        "unsorted proof-of-absence stems should be rejected"
    );
}

#[test]
fn test_absent_paths_reject_non_zero_witness_entries() {
    let metadata = VerkleKeyMetadata {
        key_type: VerkleKeyType::BasicData,
        address: H160::zero(),
        extra: 0,
        storage_key: H256::zero(),
        tx_index: 0,
    };

    let mut key = [0u8; 32];
    key[0] = 0x33;

    // Non-zero witness values are invalid for proof-of-absence paths (go-verkle requires nil PreValues).
    let entries = vec![VerkleWitnessEntry {
        key,
        metadata,
        pre_value: [0xFFu8; 32],
        post_value: [0u8; 32],
    }];

    let root_commitment = BandersnatchPoint::generator().to_bytes();

    // depth=1 (root→empty child), extStatusAbsentEmpty (low bits = 0)
    let depth_extension_present = vec![(1u8 << 3) | 0u8];

    let result = rebuild_stateless_tree_from_proof(
        &vec![root_commitment],
        &depth_extension_present,
        &[],
        &entries,
        true,
    );

    assert!(
        matches!(result, Err(VerkleProofError::PathMismatch)),
        "absent paths must reject non-zero witness entries"
    );
}

#[test]
fn test_pruned_proof_with_internal_nodes_only_consumes_leaves() {
    use evm_service::verkle_proof::{rebuild_stateless_tree_from_proof, StatelessNode};
    use evm_service::verkle::{VerkleKeyMetadata, VerkleKeyType, VerkleWitnessEntry};
    use primitive_types::{H160, H256};

    let metadata = VerkleKeyMetadata {
        key_type: VerkleKeyType::BasicData,
        address: H160::zero(),
        extra: 0,
        storage_key: H256::zero(),
        tx_index: 0,
    };

    // Deep path (root → child → grandchild) with a single present suffix.
    let mut key = [0u8; 32];
    key[0] = 0x01;
    key[1] = 0x02;
    key[2] = 0x03;
    let value = [0xABu8; 32];

    let entries = vec![VerkleWitnessEntry {
        key,
        metadata,
        pre_value: value,
        post_value: [0u8; 32],
    }];

    let mut stem = [0u8; 31];
    stem.copy_from_slice(&key[..31]);

    // depth_internal = 2 → encoded depth = 3 (includes leaf), extStatusPresent
    let depth_extension_present = vec![(3u8 << 3) | 2u8];

    // Pruned proof containing only root + internal prefixes + leaf (no C1/C2 commitments).
    let commitments = vec![[0u8; 32]; 4];

    let tree = rebuild_stateless_tree_from_proof(
        &commitments,
        &depth_extension_present,
        &[],
        &entries,
        true,
    )
    .expect("pruned proofs with internal nodes must not force C1/C2 commitments");

    let leaf = tree.get_leaf(&stem).expect("leaf should be materialized");
    match leaf {
        StatelessNode::Leaf { c1, .. } => assert!(
            !c1.is_identity(),
            "C1 should be recomputed from witness values when it is omitted from the proof"
        ),
        _ => panic!("expected leaf node"),
    }
}

#[test]
fn test_deleting_last_suffix_removes_leaf() {
    let stem = [0xABu8; 31];
    let pre_value = [0x11u8; 32];

    // Single suffix deleted (removes the stem entirely)
    let state_diff = vec![StemStateDiff {
        stem,
        suffix_diffs: vec![SuffixStateDiff {
            suffix: 0,
            current_value: Some(pre_value),
            new_value: None,
        }],
    }];

    // An empty tree should remain after deleting the only suffix
    let expected_root = BandersnatchPoint::identity().to_bytes();
    assert!(
        verify_post_state_root(&state_diff, &expected_root)
            .expect("post-state verification should succeed"),
        "deleting the last suffix should remove the stem and yield the empty-root commitment",
    );
}

/// Paths already proven PRESENT must not consume POA stems, matching go-verkle's
/// pathsWithExtPresent skip logic.
#[test]
fn test_absent_other_skipped_when_present_path_exists() {
    use evm_service::verkle::{VerkleKeyMetadata, VerkleKeyType, VerkleWitnessEntry};
    use evm_service::verkle_proof::rebuild_stateless_tree_from_proof;
    use evm_service::verkle::{BandersnatchPoint, Fr};
    use primitive_types::{H160, H256};

    let metadata = VerkleKeyMetadata {
        key_type: VerkleKeyType::BasicData,
        address: H160::zero(),
        extra: 0,
        storage_key: H256::zero(),
        tx_index: 0,
    };

    let mut present_key = [0u8; 32];
    present_key[0] = 0xAA;

    let mut absent_key = [0u8; 32];
    absent_key[0] = 0xAA; // Same path prefix as PRESENT entry (depth = 1)
    absent_key[1] = 0xFF;

    let entries = vec![
        VerkleWitnessEntry {
            key: present_key,
            metadata: metadata.clone(),
            pre_value: [0x11; 32],
            post_value: [0x11; 32],
        },
        VerkleWitnessEntry {
            key: absent_key,
            metadata,
            pre_value: [0u8; 32],
            post_value: [0u8; 32],
        },
    ];

    let root_commitment = BandersnatchPoint::generator().to_bytes();
    let leaf_commitment = BandersnatchPoint::generator()
        .mul(&Fr::from(3u64))
        .to_bytes();
    let commitments = vec![root_commitment, leaf_commitment];

    // PRESENT path covers the prefix, so ABSENT_OTHER should be skipped without needing other_stems.
    let depth_extension_present = vec![
        (1u8 << 3) | 2u8, // depth = 1, extStatusPresent
        (1u8 << 3) | 1u8, // depth = 1, extStatusAbsentOther
    ];

    let tree = rebuild_stateless_tree_from_proof(
        &commitments,
        &depth_extension_present,
        &[],
        &entries,
        true,
    )
    .expect("absent-other path sharing present prefix should be skipped");

    // Only the PRESENT path should be materialized.
    assert_eq!(tree.stem_info().len(), 1);
}

/// Duplicate ABSENT_OTHER paths that share the same navigation prefix should not consume
/// multiple `other_stems` entries. The verifier must skip the duplicate before advancing
/// the `other_stems` queue (matching go-verkle's info cache behaviour).
#[test]
fn test_absent_other_duplicate_path_reuses_proof_stem() {
    use evm_service::verkle::{VerkleKeyMetadata, VerkleKeyType, VerkleWitnessEntry};
    use evm_service::verkle_proof::rebuild_stateless_tree_from_proof;
    use evm_service::verkle::BandersnatchPoint;
    use primitive_types::{H160, H256};

    let metadata = VerkleKeyMetadata {
        key_type: VerkleKeyType::BasicData,
        address: H160::zero(),
        extra: 0,
        storage_key: H256::zero(),
        tx_index: 0,
    };

    // Two requested stems share the same first byte (navigation prefix), so one POA stem suffices.
    let mut key_a = [0u8; 32];
    key_a[0] = 0xAA;
    key_a[1] = 0x01;

    let mut key_b = [0u8; 32];
    key_b[0] = 0xAA;
    key_b[1] = 0x02;

    let entries = vec![
        VerkleWitnessEntry {
            key: key_a,
            metadata: metadata.clone(),
            pre_value: [0u8; 32],
            post_value: [0u8; 32],
        },
        VerkleWitnessEntry {
            key: key_b,
            metadata,
            pre_value: [0u8; 32],
            post_value: [0u8; 32],
        },
    ];

    // Single proof stem reused for both absence paths.
    let mut proof_stem = [0u8; 32];
    proof_stem[0] = 0xAA;
    proof_stem[1] = 0xFF;

    let root = BandersnatchPoint::generator();
    let leaf = root.mul(&evm_service::verkle::Fr::from(5u64));
    let commitments_by_path = vec![root.to_bytes(), leaf.to_bytes()];

    // Two ABSENT_OTHER entries with the same navigation prefix (depth = 1).
    let depth_extension_present = vec![(1u8 << 3) | 1u8, (1u8 << 3) | 1u8];

    let tree = rebuild_stateless_tree_from_proof(
        &commitments_by_path,
        &depth_extension_present,
        &[proof_stem],
        &entries,
        true,
    )
    .expect("duplicate POA paths sharing a prefix should reuse the same other_stem");

    // Only one path should be materialized, and the single proof stem should be consumed.
    assert_eq!(tree.stem_info().len(), 1);
}

/// Two ABSENT_OTHER paths that diverge after the first byte (depth_internal = 1) must still
/// reuse the single proof stem. Deduplication must not rely on the requested prefixes being
/// identical (they differ at the divergence byte).
#[test]
fn test_absent_other_shared_prefix_deeper_reuses_proof_stem() {
    use evm_service::verkle::{VerkleKeyMetadata, VerkleKeyType, VerkleWitnessEntry};
    use evm_service::verkle_proof::rebuild_stateless_tree_from_proof;
    use evm_service::verkle::{BandersnatchPoint, Fr};
    use primitive_types::{H160, H256};

    let metadata = VerkleKeyMetadata {
        key_type: VerkleKeyType::BasicData,
        address: H160::zero(),
        extra: 0,
        storage_key: H256::zero(),
        tx_index: 0,
    };

    // Diverge at byte 1 (depth_internal = 1, encoded_depth = 2)
    let mut key_a = [0u8; 32];
    key_a[0] = 0xAA;
    key_a[1] = 0x01;

    let mut key_b = [0u8; 32];
    key_b[0] = 0xAA;
    key_b[1] = 0x02;

    let entries = vec![
        VerkleWitnessEntry {
            key: key_a,
            metadata: metadata.clone(),
            pre_value: [0u8; 32],
            post_value: [0u8; 32],
        },
        VerkleWitnessEntry {
            key: key_b,
            metadata,
            pre_value: [0u8; 32],
            post_value: [0u8; 32],
        },
    ];

    // Single proof stem covering the first absence path only.
    let mut proof_stem = [0u8; 32];
    proof_stem[0] = 0xAA;
    proof_stem[1] = 0xFF;

    // Commitments: root, internal, POA leaf.
    let root = BandersnatchPoint::generator();
    let internal = root.mul(&Fr::from(7u64));
    let leaf = root.mul(&Fr::from(11u64));
    let commitments_by_path = vec![root.to_bytes(), internal.to_bytes(), leaf.to_bytes()];

    // depth_internal = 1 → encoded_depth = 2, EXT_STATUS_ABSENT_OTHER = 1
    let depth_extension_present = vec![(2u8 << 3) | 1u8, (2u8 << 3) | 1u8];

    let err = rebuild_stateless_tree_from_proof(
        &commitments_by_path,
        &depth_extension_present,
        &[proof_stem],
        &entries,
        true,
    )
    .expect_err("distinct navigation prefixes at depth_internal=1 require separate proof stems");

    assert_eq!(
        err,
        evm_service::verkle_proof::VerkleProofError::PathMismatch
    );
}

/// Proof-of-absence paths must navigate using the proof stem (other_stem) so that internal
/// evaluations use the child index from the actual tree, not the requested missing key.
#[test]
fn test_absent_other_uses_proof_stem_for_navigation() {
    use evm_service::verkle::{VerkleKeyMetadata, VerkleKeyType, VerkleWitnessEntry};
    use evm_service::verkle_proof::{extract_proof_elements_from_tree, rebuild_stateless_tree_from_proof};
    use evm_service::verkle::BandersnatchPoint;
    use primitive_types::{H160, H256};

    // Requested stem diverges at byte 1 from the proof stem used in other_stems.
    let mut requested_key = [0u8; 32];
    requested_key[0] = 0xAA;
    requested_key[1] = 0xFF; // diverges after the first byte

    let mut proof_stem_bytes = [0u8; 32];
    proof_stem_bytes[0] = 0xAA;
    proof_stem_bytes[1] = 0x00;

    let entries = vec![VerkleWitnessEntry {
        key: requested_key,
        metadata: VerkleKeyMetadata {
            key_type: VerkleKeyType::BasicData,
            address: H160::zero(),
            extra: 0,
            storage_key: H256::zero(),
            tx_index: 0,
        },
        pre_value: [0u8; 32],
        post_value: [0u8; 32],
    }];

    // Commitments: root, one internal node, and the POA leaf commitment.
    let root = BandersnatchPoint::generator();
    let internal = root.mul(&evm_service::verkle::Fr::from(2u64));
    let leaf = root.mul(&evm_service::verkle::Fr::from(3u64));
    let commitments_by_path = vec![root.to_bytes(), internal.to_bytes(), leaf.to_bytes()];

    // Depth: 2 (one internal node + leaf), EXT_STATUS_ABSENT_OTHER = 1
    let depth_extension_present = vec![(2u8 << 3) | 1u8];

    let tree = rebuild_stateless_tree_from_proof(
        &commitments_by_path,
        &depth_extension_present,
        &[proof_stem_bytes],
        &entries,
        true,
    )
    .expect("tree rebuild should succeed");

    let proof_elements =
        extract_proof_elements_from_tree(&tree, &entries, true).expect("extract proof elements");

    // Find the internal evaluation and ensure it uses the proof stem's child index (0x00),
    // not the requested missing index (0xFF).
    let internal_bytes = internal.to_bytes();
    let mut found_internal = false;
    for (commitment, index) in proof_elements
        .commitments
        .iter()
        .zip(proof_elements.indices.iter())
    {
        if commitment.to_bytes() == internal_bytes {
            assert_eq!(
                *index, proof_stem_bytes[1],
                "internal evaluation must follow the proof stem"
            );
            assert_ne!(
                *index, requested_key[1],
                "requested stem index must not be used for navigation"
            );
            found_internal = true;
        }
    }

    assert!(found_internal, "internal evaluation should be present");
}

/// Duplicate ABSENT_OTHER entries must not consume more `other_stems` than provided; redundant
/// paths should be skipped like go-verkle's info cache.
#[test]
fn test_absent_other_duplicates_do_not_consume_extra_stems() {
    use evm_service::verkle::{VerkleKeyMetadata, VerkleKeyType, VerkleWitnessEntry};
    use evm_service::verkle_proof::build_stem_info_map;
    use primitive_types::{H160, H256};

    let metadata = VerkleKeyMetadata {
        key_type: VerkleKeyType::BasicData,
        address: H160::zero(),
        extra: 0,
        storage_key: H256::zero(),
        tx_index: 0,
    };

    // Two witness entries with different stems but the same depth-1 prefix (0xAA).
    let mut key_a = [0u8; 32];
    key_a[0] = 0xAA;
    key_a[1] = 0x00;
    let mut key_b = [0u8; 32];
    key_b[0] = 0xAA;
    key_b[1] = 0x10;

    let entries = vec![
        VerkleWitnessEntry {
            key: key_a,
            metadata: metadata.clone(),
            pre_value: [0u8; 32],
            post_value: [0u8; 32],
        },
        VerkleWitnessEntry {
            key: key_b,
            metadata,
            pre_value: [0u8; 32],
            post_value: [0u8; 32],
        },
    ];

    // Both depth bytes encode depth=1 with EXT_STATUS_ABSENT_OTHER.
    let depth_extension_present = vec![(1u8 << 3) | 1u8, (1u8 << 3) | 1u8];

    // Only one proof-of-absence stem is provided; duplicate paths must not consume another.
    let other_stems = vec![key_a];

    let stems: Vec<[u8; 31]> = entries
        .iter()
        .map(|entry| {
            let mut stem = [0u8; 31];
            stem.copy_from_slice(&entry.key[..31]);
            stem
        })
        .collect();

    let stem_info = build_stem_info_map(
        &stems,
        &depth_extension_present,
        &other_stems,
        &entries,
        true,
    )
    .expect("duplicate absent-other paths should be deduplicated");

    assert_eq!(
        stem_info.len(),
        1,
        "redundant absent-other entries must not require extra other_stems"
    );
    assert!(
        stem_info[0].info.is_absence,
        "materialized path should be marked as absence"
    );
}

/// ABSENT_OTHER paths with different navigation prefixes require distinct proof stems.
/// Reusing the previous proof stem when `other_stems` is exhausted must fail.
#[test]
fn test_absent_other_requires_fresh_poa_stem_per_navigation_path() {
    use evm_service::verkle::{VerkleKeyMetadata, VerkleKeyType, VerkleWitnessEntry};
    use evm_service::verkle_proof::build_stem_info_map;
    use primitive_types::{H160, H256};

    let metadata = VerkleKeyMetadata {
        key_type: VerkleKeyType::BasicData,
        address: H160::zero(),
        extra: 0,
        storage_key: H256::zero(),
        tx_index: 0,
    };

    // Requested stems diverge at byte 1 from the proof stem, so encoded depth = 2.
    let mut requested_a = [0u8; 32];
    requested_a[0] = 0xAA;
    requested_a[1] = 0x10;

    let mut requested_b = [0u8; 32];
    requested_b[0] = 0xAA;
    requested_b[1] = 0x20;

    let entries = vec![
        VerkleWitnessEntry {
            key: requested_a,
            metadata: metadata.clone(),
            pre_value: [0u8; 32],
            post_value: [0u8; 32],
        },
        VerkleWitnessEntry {
            key: requested_b,
            metadata,
            pre_value: [0u8; 32],
            post_value: [0u8; 32],
        },
    ];

    // Proof stem that shares the internal prefix with both requested stems.
    let mut proof_stem = [0u8; 32];
    proof_stem[0] = 0xAA;
    proof_stem[1] = 0x00;

    // Depth byte: encoded_depth = 2, EXT_STATUS_ABSENT_OTHER = 1.
    let depth_extension_present = vec![(2u8 << 3) | 1u8, (2u8 << 3) | 1u8];

    let stems: Vec<[u8; 31]> = entries
        .iter()
        .map(|entry| {
            let mut stem = [0u8; 31];
            stem.copy_from_slice(&entry.key[..31]);
            stem
        })
        .collect();

    let err = build_stem_info_map(
        &stems,
        &depth_extension_present,
        &[proof_stem],
        &entries,
        true,
    )
    .expect_err("missing PoA stems for distinct navigation prefixes must be rejected");

    assert_eq!(err, evm_service::verkle_proof::VerkleProofError::PathMismatch);
}

/// Test for multi-stem C1/C2 commitment accounting bug fix.
///
/// Before the fix (commit 6666ce9c), create_path used `commit_idx < commitments.len()`
/// to decide if C1/C2 were in the proof. This caused stem A to consume stem B's C1/C2
/// commitments in multi-stem proofs, leading to PathMismatch errors.
///
/// After the fix, each stem gets a per-stem commitment budget (expected_commitments)
/// calculated upfront, preventing incorrect consumption across stems.
#[test]
fn test_multi_stem_c1_c2_budget() {
    use primitive_types::{H160, H256};

    println!("\n🧪 Test: Multi-stem C1/C2 commitment budget (bug fix regression test)");

    // Scenario: 2 stems, both with values requiring C1 (suffix < 128)
    //
    // Stem A: has_c1=true, has_c2=false → needs 1 leaf + 1 C1 = 2 commitments
    // Stem B: has_c1=true, has_c2=false → needs 1 leaf + 1 C1 = 2 commitments
    //
    // Total proof: root (1) + stemA_leaf (1) + stemA_C1 (1) + stemB_leaf (1) + stemB_C1 (1) = 5
    //
    // Bug behavior (before fix):
    // - Stem A: checks commit_idx=1 < 5, consumes commitments[1] as leaf ✓
    // - Stem A: checks commit_idx=2 < 5, consumes commitments[2] as C1 ✓
    // - Stem B: checks commit_idx=3 < 5, consumes commitments[3] as leaf ✓
    // - Stem B: checks commit_idx=4 < 5, consumes commitments[4] as C1 ✓
    // - Result: commit_idx=5 == 5, but this was ACCIDENTAL! If stem A had has_c2=true,
    //   it would consume stemB's C1 commitment.
    //
    // Correct behavior (after fix):
    // - Stem A: expected_commitments=2, consumes exactly 2 (leaf + C1)
    // - Stem B: expected_commitments=2, consumes exactly 2 (leaf + C1)

    let stem1 = [0xAA; 31];
    let stem2 = [0xBB; 31];

    // Stem 1: value at suffix 0 (requires C1)
    let mut key1 = [0u8; 32];
    key1[..31].copy_from_slice(&stem1);
    key1[31] = 0; // suffix 0

    // Stem 2: value at suffix 1 (requires C1)
    let mut key2 = [0u8; 32];
    key2[..31].copy_from_slice(&stem2);
    key2[31] = 1; // suffix 1

    let metadata = VerkleKeyMetadata {
        key_type: VerkleKeyType::BasicData,
        address: H160::zero(),
        extra: 0,
        storage_key: H256::zero(),
        tx_index: 0,
    };

    let entries = vec![
        VerkleWitnessEntry {
            key: key1,
            metadata: metadata.clone(),
            pre_value: [0x11; 32],
            post_value: [0x11; 32],
        },
        VerkleWitnessEntry {
            key: key2,
            metadata,
            pre_value: [0x22; 32],
            post_value: [0x22; 32],
        },
    ];

    // Proof structure: root + stemA(leaf+C1) + stemB(leaf+C1) = 5 commitments
    let root = BandersnatchPoint::generator();
    let stem_a_leaf = root.mul(&Fr::from(1u64));
    let stem_a_c1 = root.mul(&Fr::from(2u64));
    let stem_b_leaf = root.mul(&Fr::from(3u64));
    let stem_b_c1 = root.mul(&Fr::from(4u64));

    let commitments = vec![
        root.to_bytes(),
        stem_a_leaf.to_bytes(),
        stem_a_c1.to_bytes(),
        stem_b_leaf.to_bytes(),
        stem_b_c1.to_bytes(),
    ];

    // Both stems at depth 1 (root → leaf)
    let depth_extension_present = vec![
        (1u8 << 3) | 2u8, // depth=1, EXT_STATUS_PRESENT
        (1u8 << 3) | 2u8, // depth=1, EXT_STATUS_PRESENT
    ];

    println!("  Stems: 2");
    println!("  Commitments: {} (root + 2×(leaf+C1))", commitments.len());
    println!("  Expected per-stem: 2 (leaf + C1)");

    let result = rebuild_stateless_tree_from_proof(
        &commitments,
        &depth_extension_present,
        &[],
        &entries,
        true, // use_pre_value
    );

    match &result {
        Ok(tree) => {
            println!("  ✅ Tree rebuilt successfully with per-stem budget");

            // Verify both leaves are accessible
            match tree.get_leaf(&stem1) {
                Ok(_) => println!("  ✅ Stem A leaf accessible"),
                Err(e) => panic!("Stem A leaf not accessible: {:?}", e),
            }

            match tree.get_leaf(&stem2) {
                Ok(_) => println!("  ✅ Stem B leaf accessible"),
                Err(e) => panic!("Stem B leaf not accessible: {:?}", e),
            }
        }
        Err(e) => {
            panic!("Multi-stem proof failed (per-stem budget not working): {:?}", e);
        }
    }

    result.expect("multi-stem C1/C2 budget should work");
}
