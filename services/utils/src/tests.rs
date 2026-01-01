//! Tests for witness verification
//!
//! This module contains tests that verify Merkle proof validation
//! using real witness data exported from the Go test suite.

#![cfg(test)]

extern crate alloc;
extern crate std;

use crate::effects::StateWitness;
use alloc::{string::String, vec, vec::Vec};
use std::println;

fn read_test_service_id() -> Option<u32> {
    std::env::var("JAM_TEST_SERVICE_ID")
        .ok()
        .and_then(|value| value.parse().ok())
}

/// Test witness verification with real data from Go tests
///
/// To generate the test data, run:
/// ```bash
/// cd node && go test -tags network_test -run TestEVM
/// ```
/// This will create witness_test_*.bin files containing binary-encoded StateWitness data.
/// Each file format: object_id (32) + object_ref (64) + proofs (32 each).
/// Legacy exports may include a 4-byte little-endian proof_count before the proofs; both forms are accepted.
///
/// To run verification tests on the exported data:
/// ```bash
/// cargo test --lib test_verify_exported_witnesses -- --nocapture
/// ```
#[test]
fn verify_test() {
    // Test basic witness verification using real exported data if available

    use crate::effects::verify_merkle_proof;

    // Try to load a real witness file
    let witness_file = "0000000000000000000000000000000000000001000000000000000000000000-1-b1c7e52f8638ad944425874da47c59747fc42f0230d3bb55a8b1acc57b53e061.bin";

    match std::fs::read(witness_file) {
        Ok(data) => {
            // Test with real witness data
            match StateWitness::deserialize(&data) {
                Ok(witness) => {
                    let Some(service_id) = read_test_service_id() else {
                        println!("JAM_TEST_SERVICE_ID not set; skipping real witness verification.");
                        return;
                    };

                    // Parse state root from filename
                    let state_root_hex =
                        "b1c7e52f8638ad944425874da47c59747fc42f0230d3bb55a8b1acc57b53e061";
                    let mut state_root = [0u8; 32];
                    for i in 0..32 {
                        state_root[i] =
                            u8::from_str_radix(&state_root_hex[i * 2..i * 2 + 2], 16).unwrap();
                    }

                    // Test 1: Real witness should verify with correct state root
                    assert!(
                        witness.verify(service_id, state_root),
                        "Real witness data should verify against its state root"
                    );

                    // Test 2: Should fail with wrong state root
                    let wrong_root = [0xFFu8; 32];
                    assert!(
                        !witness.verify(service_id, wrong_root),
                        "Witness should fail verification with wrong state root"
                    );

                    // Test 3: Manually verify with verify_merkle_proof
                    let value = &witness.value;
                    assert!(
                        verify_merkle_proof(
                            service_id,
                            &witness.object_id,
                            value,
                            &state_root,
                            &witness.path
                        ),
                        "Manual verify_merkle_proof should pass"
                    );

                    println!("✓ Real witness verification tests passed!");
                }
                Err(e) => {
                    panic!("Failed to deserialize real witness file: {:?}", e);
                }
            }
        }
        Err(_) => {
            // No real witness files available - test basic properties
            println!("No real witness files found, testing basic verification properties");

            // Test: Wrong proof should fail
            let service_id = 35u32;
            let object_id = [0x01u8; 32];
            let value = vec![0x12, 0x34];
            let wrong_proof = vec![[0xFFu8; 32]];
            let wrong_root = [0xAAu8; 32];

            assert!(
                !verify_merkle_proof(service_id, &object_id, &value, &wrong_root, &wrong_proof),
                "Verification should fail with wrong proof and root"
            );

            println!("✓ Basic verification property tests passed!");
        }
    }
}

/// Helper test to verify compute_storage_opaque_key matches Go implementation
#[test]
fn test_compute_storage_opaque_key() {
    use crate::effects::compute_storage_opaque_key;
    use crate::hash_functions::blake2b_hash;

    // Test 1: service_id=0, object_id=all zeros
    {
        let service_id = 0u32;
        let object_id = [0u8; 32];

        let opaque_key = compute_storage_opaque_key(service_id, &object_id);

        // Manually compute what it should be:
        // as_internal_key = [0xFF, 0xFF, 0xFF, 0xFF] ++ object_id
        let mut as_internal_key = alloc::vec![0xFFu8, 0xFF, 0xFF, 0xFF];
        as_internal_key.extend_from_slice(&object_id);
        let hash = blake2b_hash(&as_internal_key);

        // Verify structure: [s0, h0, s1, h1, s2, h2, s3, h3, h4..h26, 0]
        assert_eq!(opaque_key[0], 0, "s0 should be 0");
        assert_eq!(opaque_key[1], hash[0], "h0 should match");
        assert_eq!(opaque_key[2], 0, "s1 should be 0");
        assert_eq!(opaque_key[3], hash[1], "h1 should match");
        assert_eq!(opaque_key[4], 0, "s2 should be 0");
        assert_eq!(opaque_key[5], hash[2], "h2 should match");
        assert_eq!(opaque_key[6], 0, "s3 should be 0");
        assert_eq!(opaque_key[7], hash[3], "h3 should match");

        // Check h[4..27] is copied to positions 8..31
        for i in 0..23 {
            assert_eq!(
                opaque_key[8 + i],
                hash[4 + i],
                "h{} should match at position {}",
                4 + i,
                8 + i
            );
        }

        // Last byte should be 0
        assert_eq!(opaque_key[31], 0, "last byte should be 0");
    }

    // Test 2: service_id=42, object_id=0x01010101...
    {
        let service_id = 42u32;
        let object_id = [0x01u8; 32];

        let opaque_key = compute_storage_opaque_key(service_id, &object_id);

        // Service ID 42 in little-endian = [42, 0, 0, 0]
        assert_eq!(opaque_key[0], 42, "s0 should be 42");
        assert_eq!(opaque_key[2], 0, "s1 should be 0");
        assert_eq!(opaque_key[4], 0, "s2 should be 0");
        assert_eq!(opaque_key[6], 0, "s3 should be 0");

        // Last byte should be 0
        assert_eq!(opaque_key[31], 0, "last byte should be 0");
    }

    println!("compute_storage_opaque_key test passed!");
}

/// Test witness verification using exported binary files from Go tests
///
/// This test loads files with format: objectid-version-stateroot.bin
/// generated by ExportStateWitnesses and verifies each witness.
///
/// The state root is parsed from the filename, so each file is self-contained.
///
/// Note: This test will be skipped if no witness files are found.
#[test]
fn test_verify_exported_witnesses() {
    use crate::effects::verify_merkle_proof;

    // Find all .bin files in current directory
    let entries = match std::fs::read_dir(".") {
        Ok(entries) => entries,
        Err(_) => {
            println!("Failed to read directory. Skipping test.");
            return;
        }
    };

    let mut witness_files: Vec<(String, Vec<u8>)> = Vec::new();
    for entry in entries.flatten() {
        if let Ok(filename) = entry.file_name().into_string() {
            // Match files with pattern: <64-hex>-<version>-<64-hex>.bin
            if filename.ends_with(".bin") && filename.matches('-').count() == 2 {
                if let Ok(data) = std::fs::read(&filename) {
                    witness_files.push((filename, data));
                }
            }
        }
    }

    if witness_files.is_empty() {
        println!("No witness files found. Run Go tests to generate <objectid>-<version>-<stateroot>.bin files.");
        println!("Skipping test.");
        return;
    }

    let Some(service_id) = read_test_service_id() else {
        println!("JAM_TEST_SERVICE_ID not set; skipping exported witness verification.");
        return;
    };

    println!("Found {} witness files", witness_files.len());

    let mut successful_verifications = 0;
    let mut failed_verifications = 0;
    let mut failed_deserializations = 0;
    let mut failed_filename_parse = 0;

    for (filename, data) in &witness_files {
        // Parse filename: objectid-version-stateroot.bin
        let parts: Vec<&str> = filename.trim_end_matches(".bin").split('-').collect();
        if parts.len() != 3 {
            failed_filename_parse += 1;
            println!("✗ {}: Invalid filename format", filename);
            continue;
        }

        // Parse state root from filename (last part, 64 hex chars)
        let state_root_hex = parts[2];
        if state_root_hex.len() != 64 {
            failed_filename_parse += 1;
            println!("✗ {}: Invalid state root hex length", filename);
            continue;
        }

        let mut state_root = [0u8; 32];
        for i in 0..32 {
            let byte_str = &state_root_hex[i * 2..i * 2 + 2];
            match u8::from_str_radix(byte_str, 16) {
                Ok(byte) => state_root[i] = byte,
                Err(_) => {
                    failed_filename_parse += 1;
                    println!("✗ {}: Invalid state root hex", filename);
                    continue;
                }
            }
        }

        match StateWitness::deserialize(data) {
            Ok(witness) => {
                // Verify the witness against the state root from filename
                let verified = verify_merkle_proof(
                    service_id,
                    &witness.object_id,
                    &witness.value,
                    &state_root,
                    &witness.path,
                );

                if verified {
                    successful_verifications += 1;
                    println!(
                        "✓ {}: VERIFIED (service_id={}, path_len={})",
                        filename,
                        service_id,
                        witness.path.len()
                    );
                } else {
                    failed_verifications += 1;
                    println!("✗ {}: FAILED verification", filename);
                }
            }
            Err(e) => {
                failed_deserializations += 1;
                println!("✗ {}: Failed to deserialize: {:?}", filename, e);
            }
        }
    }

    println!("\nResults:");
    println!("  Successful verifications: {}", successful_verifications);
    println!("  Failed verifications: {}", failed_verifications);
    println!("  Failed deserializations: {}", failed_deserializations);
    println!("  Failed filename parsing: {}", failed_filename_parse);

    assert!(
        successful_verifications > 0,
        "At least one witness should verify successfully"
    );
    assert_eq!(
        failed_deserializations, 0,
        "All witnesses should deserialize successfully"
    );
    assert_eq!(
        failed_verifications, 0,
        "All witnesses should verify successfully"
    );
    assert_eq!(
        failed_filename_parse, 0,
        "All filenames should parse successfully"
    );
}
