// Mixed-asset commitment tree tests
//
// Validates that the commitment tree correctly handles:
// - Native USDx commitments
// - Custom asset (ZSA) commitments
// - Mixed trees with both native and custom assets
// - Deterministic root computation regardless of asset type
//
// Note: This tests the Merkle tree structure itself with different
// asset commitments. The actual Orchard note commitment derivation
// uses Sinsemilla hashing to produce valid Pallas field elements.

use orchard_service::crypto::{merkle_root_from_leaves, merkle_append_from_leaves, commitment};

const NATIVE_ASSET: u32 = 0;
const CUSTOM_ASSET_1: u32 = 1;
const CUSTOM_ASSET_2: u32 = 2;
const CUSTOM_ASSET_3: u32 = 3;

/// Create a test commitment with asset type embedded
fn create_test_commitment(index: u8, asset_id: u32, value: u64) -> [u8; 32] {
    // Create a real Orchard commitment using Sinsemilla hashing
    // This produces valid Pallas field elements that the Merkle tree can handle
    let mut owner_pk = [0u8; 32];
    owner_pk[0] = index;  // Unique owner for each note

    let mut rho = [0u8; 32];
    rho[0] = index;  // Unique rho for each note

    let mut note_rseed = [0u8; 32];
    note_rseed[0] = index;  // Unique rseed for each note

    let memo_hash = [0u8; 32];  // Empty memo
    let unlock_height = 0u64;

    commitment(
        asset_id,
        value as u128,
        &owner_pk,
        &rho,
        &note_rseed,
        unlock_height,
        &memo_hash,
    ).expect("Commitment should generate valid Pallas field element")
}

#[test]
fn test_mixed_asset_tree_native_only() {
    // Tree with only native USDx commitments
    let cm1 = create_test_commitment(1, NATIVE_ASSET, 100);
    let cm2 = create_test_commitment(2, NATIVE_ASSET, 200);
    let cm3 = create_test_commitment(3, NATIVE_ASSET, 300);

    let commitments = [cm1, cm2, cm3];
    let root = merkle_root_from_leaves(&commitments)
        .expect("Should compute root for native-only tree");

    // Root should be deterministic
    let root2 = merkle_root_from_leaves(&commitments)
        .expect("Should compute root for native-only tree");
    assert_eq!(root, root2, "Root should be deterministic");

    // Root should not be empty
    assert_ne!(root, [0u8; 32], "Root should not be all zeros");
}

#[test]
fn test_mixed_asset_tree_custom_only() {
    // Tree with only custom asset commitments
    let cm1 = create_test_commitment(1, CUSTOM_ASSET_1, 100);
    let cm2 = create_test_commitment(2, CUSTOM_ASSET_1, 200);

    let commitments = [cm1, cm2];
    let root = merkle_root_from_leaves(&commitments)
        .expect("Should compute root for custom-only tree");

    // Root should be deterministic
    let root2 = merkle_root_from_leaves(&commitments)
        .expect("Should compute root for custom-only tree");
    assert_eq!(root, root2, "Root should be deterministic");

    assert_ne!(root, [0u8; 32], "Root should not be all zeros");
}

#[test]
fn test_mixed_asset_tree_native_and_custom() {
    // Tree with both native and custom asset commitments interleaved
    let cm_native = create_test_commitment(1, NATIVE_ASSET, 100);
    let cm_custom1 = create_test_commitment(2, CUSTOM_ASSET_1, 200);
    let cm_native2 = create_test_commitment(3, NATIVE_ASSET, 300);
    let cm_custom2 = create_test_commitment(4, CUSTOM_ASSET_2, 400);

    let commitments = [cm_native, cm_custom1, cm_native2, cm_custom2];
    let root = merkle_root_from_leaves(&commitments)
        .expect("Should compute root for mixed tree");

    // Root should be deterministic
    let root2 = merkle_root_from_leaves(&commitments)
        .expect("Should compute root for mixed tree");
    assert_eq!(root, root2, "Root should be deterministic for mixed assets");

    assert_ne!(root, [0u8; 32], "Root should not be all zeros");
}

#[test]
fn test_mixed_asset_tree_order_matters() {
    // Verify that commitment order affects the root
    let cm_native = create_test_commitment(1, NATIVE_ASSET, 100);
    let cm_custom = create_test_commitment(2, CUSTOM_ASSET_1, 200);

    // Order 1: native, then custom
    let commitments1 = [cm_native, cm_custom];
    let root1 = merkle_root_from_leaves(&commitments1)
        .expect("Should compute root");

    // Order 2: custom, then native
    let commitments2 = [cm_custom, cm_native];
    let root2 = merkle_root_from_leaves(&commitments2)
        .expect("Should compute root");

    assert_ne!(root1, root2, "Different commitment order should produce different roots");
}

#[test]
fn test_mixed_asset_tree_append() {
    // Test incremental append with mixed assets
    let cm_native = create_test_commitment(1, NATIVE_ASSET, 100);
    let cm_custom = create_test_commitment(2, CUSTOM_ASSET_1, 200);

    // Start with native commitment
    let base = [cm_native];
    let root_after_append = merkle_append_from_leaves(&base, &[cm_custom])
        .expect("Should append custom commitment to native tree");

    // Compare with building full tree from scratch
    let full_tree = [cm_native, cm_custom];
    let root_full = merkle_root_from_leaves(&full_tree)
        .expect("Should compute full tree root");

    assert_eq!(
        root_after_append, root_full,
        "Appending should produce same root as building full tree"
    );
}

#[test]
fn test_mixed_asset_commitments_are_unique() {
    // Same index and value but different assets should produce different commitments
    let cm_native = create_test_commitment(1, NATIVE_ASSET, 100);
    let cm_custom = create_test_commitment(1, CUSTOM_ASSET_1, 100);

    assert_ne!(
        cm_native, cm_custom,
        "Same parameters with different assets should produce different commitments"
    );
}

#[test]
fn test_mixed_asset_tree_large_scale() {
    // Test tree with many mixed commitments
    let mut commitments = Vec::new();

    // Add 20 commitments alternating between different assets
    for i in 0..20u8 {
        let asset = match i % 4 {
            0 => NATIVE_ASSET,
            1 => CUSTOM_ASSET_1,
            2 => CUSTOM_ASSET_2,
            _ => CUSTOM_ASSET_3,
        };

        let cm = create_test_commitment(i, asset, (i as u64) * 100);
        commitments.push(cm);
    }

    let root = merkle_root_from_leaves(&commitments)
        .expect("Should compute root for large mixed tree");

    // Verify determinism
    let root2 = merkle_root_from_leaves(&commitments)
        .expect("Should compute root for large mixed tree");

    assert_eq!(root, root2, "Large tree root should be deterministic");
    assert_ne!(root, [0u8; 32], "Root should not be all zeros");
}

#[test]
fn test_mixed_asset_tree_empty() {
    // Empty tree should have deterministic empty root
    let empty_commitments: [[u8; 32]; 0] = [];

    let root1 = merkle_root_from_leaves(&empty_commitments)
        .expect("Should compute empty root");
    let root2 = merkle_root_from_leaves(&empty_commitments)
        .expect("Should compute empty root");

    assert_eq!(root1, root2, "Empty tree root should be deterministic");
}

#[test]
fn test_mixed_asset_tree_single_commitment() {
    // Single commitment tree (native)
    let cm_native = create_test_commitment(1, NATIVE_ASSET, 100);
    let root1 = merkle_root_from_leaves(&[cm_native])
        .expect("Should compute single-node root");

    // Single commitment tree (custom)
    let cm_custom = create_test_commitment(1, CUSTOM_ASSET_1, 100);
    let root2 = merkle_root_from_leaves(&[cm_custom])
        .expect("Should compute single-node root");

    // Both should compute successfully but have different roots
    assert_ne!(root1, root2, "Single-node trees with different assets should have different roots");
}

#[test]
fn test_mixed_asset_tree_incremental_build() {
    // Test building tree incrementally with mixed assets
    let mut commitments = Vec::new();

    // Start empty
    let cm1 = create_test_commitment(1, NATIVE_ASSET, 100);
    commitments.push(cm1);
    let root1 = merkle_root_from_leaves(&commitments)
        .expect("Should compute root with 1 commitment");

    // Add custom asset
    let cm2 = create_test_commitment(2, CUSTOM_ASSET_1, 200);
    commitments.push(cm2);
    let root2 = merkle_root_from_leaves(&commitments)
        .expect("Should compute root with 2 commitments");

    // Add another native
    let cm3 = create_test_commitment(3, NATIVE_ASSET, 300);
    commitments.push(cm3);
    let root3 = merkle_root_from_leaves(&commitments)
        .expect("Should compute root with 3 commitments");

    // Each root should be different
    assert_ne!(root1, root2, "Adding commitment should change root");
    assert_ne!(root2, root3, "Adding another commitment should change root again");
    assert_ne!(root1, root3, "Roots should all be different");
}

#[test]
fn test_mixed_asset_different_values_same_asset() {
    // Same asset but different values should produce different commitments
    let cm1 = create_test_commitment(1, CUSTOM_ASSET_1, 100);
    let cm2 = create_test_commitment(2, CUSTOM_ASSET_1, 200);
    let cm3 = create_test_commitment(3, CUSTOM_ASSET_1, 300);

    // All should be unique
    assert_ne!(cm1, cm2, "Different values should produce different commitments");
    assert_ne!(cm2, cm3, "Different values should produce different commitments");
    assert_ne!(cm1, cm3, "Different values should produce different commitments");

    // Tree with all same asset but different values should work
    let commitments = [cm1, cm2, cm3];
    let root = merkle_root_from_leaves(&commitments)
        .expect("Should compute root for same-asset different-value tree");

    assert_ne!(root, [0u8; 32], "Root should not be all zeros");
}
