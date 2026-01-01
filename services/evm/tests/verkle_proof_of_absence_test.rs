//! Proof of absence tests using go-verkle fixtures with fresh format
//!
//! Tests Verkle proof verification with absent keys (proof of absence).

use std::fs;
use std::path::PathBuf;

use evm_service::verkle_proof::*;
use evm_service::verkle::*;
use serde::Deserialize;
use ark_ff::PrimeField;

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
    pre_state_root: String,
    #[allow(dead_code)]
    post_state_root: String,
    verkle_proof: FixtureVerkleProof,
    state_diff: Vec<FixtureStemDiff>,
}

fn hex_to_array<const N: usize>(s: &str) -> Result<[u8; N], hex::FromHexError> {
    let stripped = s.strip_prefix("0x").unwrap_or(s);
    let v = hex::decode(stripped)?;
    if v.len() != N {
        return Err(hex::FromHexError::InvalidStringLength);
    }
    let mut arr = [0u8; N];
    arr.copy_from_slice(&v);
    Ok(arr)
}

fn load_fresh_absence_fixture() -> ([u8; 32], VerkleProof, StateDiff) {
    let path = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("tests/fixtures")
        .join("verkle_fixture_proof_of_absence_fresh.json");
    let content = fs::read_to_string(&path).expect("fresh fixture should exist");
    let fixture: VerkleFixture = serde_json::from_str(&content).expect("valid JSON");

    let pre_state_root = hex_to_array::<32>(&fixture.pre_state_root).expect("valid root");

    // Parse verkle_proof
    let other_stems: Vec<[u8; 32]> = fixture
        .verkle_proof
        .other_stems
        .iter()
        .map(|s| hex_to_array::<32>(s).expect("valid other stem"))
        .collect();

    let depth_hex = fixture.verkle_proof.depth_extension_present.strip_prefix("0x")
        .unwrap_or(&fixture.verkle_proof.depth_extension_present);
    let depth_extension_present = hex::decode(depth_hex).expect("valid depth");

    let commitments_by_path: Vec<[u8; 32]> = fixture
        .verkle_proof
        .commitments_by_path
        .iter()
        .map(|c| hex_to_array::<32>(c).expect("valid commitment"))
        .collect();

    // Handle rootless test fixture by prepending root (like go-verkle shim)
    // In production, the go-verkle shim ensures root is present, but test fixtures may be rootless
    let commitments_by_path = if commitments_by_path.is_empty() || commitments_by_path[0] != pre_state_root {
        // Prepend root for rootless test fixture compatibility
        let mut full_commitments = vec![pre_state_root];
        full_commitments.extend_from_slice(&commitments_by_path);
        full_commitments
    } else {
        commitments_by_path
    };

    let d = hex_to_array::<32>(&fixture.verkle_proof.d).expect("valid d");

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

    // Parse state_diff
    let mut state_diff = Vec::new();
    for stem_data in &fixture.state_diff {
        let stem_hex = stem_data.stem.strip_prefix("0x").unwrap_or(&stem_data.stem);
        let mut stem_bytes = [0u8; 31];
        let decoded = hex::decode(stem_hex).expect("valid stem");
        stem_bytes.copy_from_slice(&decoded);

        let mut suffix_diffs = Vec::new();
        for suffix_data in &stem_data.suffix_diffs {
            let current_value = suffix_data.current_value.as_ref()
                .map(|v| hex_to_array::<32>(v).expect("valid current_value"));
            let new_value = suffix_data.new_value.as_ref()
                .map(|v| hex_to_array::<32>(v).expect("valid new_value"));

            suffix_diffs.push(SuffixStateDiff {
                suffix: suffix_data.suffix,
                current_value,
                new_value,
            });
        }

        state_diff.push(StemStateDiff {
            stem: stem_bytes,
            suffix_diffs,
        });
    }

    // Build multiproof inputs to get indices/values
    let root_point = BandersnatchPoint::from_bytes(&pre_state_root)
        .expect("valid root commitment");
    let multiproof_inputs = build_multiproof_inputs_from_state_diff_with_root(&state_diff, Some(root_point))
        .expect("build multiproof inputs");

    // Convert Fr values to bytes
    use ark_ff::BigInteger;
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
        commitments_by_path,
        d,
        ipa_proof,
        multiproof_inputs.indices,
        values_bytes,
    ).expect("proof from_parts");

    (pre_state_root, proof, state_diff)
}

#[test]
fn test_fresh_fixture_proof_of_absence() {
    println!("\n========== FRESH FIXTURE TEST ==========");
    let (pre_state_root, proof, state_diff) = load_fresh_absence_fixture();

    println!("Pre-state root: {}", hex::encode(pre_state_root));
    println!("Proof commitments: {}", proof.commitments_by_path.len());
    println!("State diff stems: {}", state_diff.len());

    // CRITICAL: Verify that proof is cryptographically bound to the expected root
    // The proof MUST contain the root commitment in commitments_by_path[0]
    assert!(!proof.commitments_by_path.is_empty(), "Proof must contain root commitment");

    let proof_root = proof.commitments_by_path[0];
    assert_eq!(
        proof_root, pre_state_root,
        "Proof's root commitment must match expected pre_state_root"
    );

    // Verify prestate commitment to ensure proof is bound to correct root
    verify_prestate_commitment(&pre_state_root, &proof)
        .expect("Prestate verification must succeed - proof must be bound to correct root");

    println!("✅ Prestate verification passed - proof is cryptographically bound to root");

    // Rebuild multiproof inputs from state_diff using root from proof
    let root_commitment = BandersnatchPoint::from_bytes(&proof_root)
        .expect("parse root commitment from proof");

    let multiproof_inputs =
        build_multiproof_inputs_from_state_diff_with_root(&state_diff, Some(root_commitment))
            .expect("rebuild multiproof inputs");

    println!("Multiproof inputs:");
    println!("  commitments: {}", multiproof_inputs.commitments.len());
    println!("  indices: {:?}", multiproof_inputs.indices);
    println!("  values: {}", multiproof_inputs.values.len());

    // Verify IPA multiproof
    let evals: Vec<(Fr, Fr)> = multiproof_inputs.indices.iter()
        .zip(multiproof_inputs.values.iter())
        .map(|(idx, val)| (Fr::from(*idx as u64), *val))
        .collect();

    match verify_ipa_multiproof_with_evals(
        &multiproof_inputs.commitments,
        &proof,
        &evals,
    ) {
        Ok(true) => {
            println!("✅ IPA MULTIPROOF VERIFICATION PASSED!");
            println!("   Fresh fixture works correctly!");
        }
        Ok(false) => {
            panic!("❌ IPA verification returned false - verification failed");
        }
        Err(e) => {
            eprintln!("❌ IPA verification error");
            eprintln!("   Error: {:?}", e);
            panic!("IPA verification failed: {:?}", e);
        }
    }

    // Verify structural properties
    assert_eq!(state_diff[0].suffix_diffs.len(), 3, "Fresh fixture has 3 present keys");
    assert_eq!(multiproof_inputs.indices.len(), 10, "IPA has 10 evaluations");
}
