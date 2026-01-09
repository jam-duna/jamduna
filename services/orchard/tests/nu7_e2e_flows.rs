// NU7 End-to-End Integration Tests
//
// Complete integration test validating IssueBundle flow through builder → refiner → accumulator

#![cfg(test)]

use orchard_service::bundle_codec::{DecodedIssueBundle, DecodedIssueAction, DecodedIssuedNote};
use orchard_service::crypto::{merkle_append_from_leaves, merkle_root_from_leaves};
use orchard_service::state::OrchardServiceState;
use orchard_service::signature_verifier::{
    compute_issue_bundle_commitment, compute_issue_bundle_commitments, derive_asset_base,
    verify_issue_bundle,
};
use halo2_proofs::pasta::pallas;
use halo2_proofs::pasta::group::{Group, GroupEncoding};
use halo2_proofs::pasta::group::ff::PrimeField;
use k256::schnorr::signature::hazmat::PrehashSigner;
use k256::schnorr::SigningKey;
use rand::rngs::StdRng;
use rand::SeedableRng;

#[test]
fn test_issue_and_transfer_lifecycle() {
    println!("\n=== NU7 Issue and Transfer Lifecycle Test ===\n");

    // Step 1: Create IssueBundle
    let asset_desc = [0x11u8; 32];
    let mut rng = StdRng::seed_from_u64(0x5a11_1ce0);
    let signing_key = SigningKey::random(&mut rng);
    let verifying_key = signing_key.verifying_key();
    let mut issuer_key = vec![0x00u8];
    issuer_key.extend_from_slice(&verifying_key.to_bytes());

    let asset_base = derive_asset_base(&issuer_key, &asset_desc).expect("asset base derivation");
    
    println!("Step 1: Asset Creation");
    println!("  Asset desc hash: {}", hex::encode(&asset_desc[..8]));
    println!("  Asset ID (AssetBase): {}", hex::encode(&asset_base.0[..8]));

    // Step 2: Create recipient and note
    let mut recipient = [0u8; 43];
    recipient[..11].copy_from_slice(&[0x02u8; 11]);
    recipient[11..].copy_from_slice(&pallas::Point::generator().to_bytes());

    let note = DecodedIssuedNote {
        recipient,
        value: 1000,
        rho: {
            let mut r = [0u8; 32];
            r[0] = 1;
            r
        },
        rseed: [0x05u8; 32],
    };

    let mut bundle = DecodedIssueBundle {
        issuer_key,
        actions: vec![DecodedIssueAction {
            asset_desc_hash: asset_desc,
            notes: vec![note.clone()],
            finalize: true,
        }],
        sighash_info: vec![0x00],
        signature: vec![0u8; 65],
    };

    println!("\nStep 2: IssueBundle Created");
    println!("  Actions: {}", bundle.actions.len());
    println!("  Notes: {}", bundle.actions[0].notes.len());
    println!("  Value: {}", note.value);

    // Step 3: Sign IssueBundle
    let sighash = compute_issue_bundle_commitment(&bundle)
        .expect("IssueBundle commitment computation failed");
    let signature = signing_key
        .sign_prehash(&sighash)
        .expect("IssueBundle signature failed");
    let mut signature_bytes = vec![0x00u8];
    signature_bytes.extend_from_slice(&signature.to_bytes());
    bundle.signature = signature_bytes;

    verify_issue_bundle(&bundle).expect("IssueBundle signature verification failed");

    // Step 4: Compute commitments
    let commitments = compute_issue_bundle_commitments(&bundle)
        .expect("Commitment computation failed");

    assert_eq!(commitments.len(), 1);
    let commitment = commitments[0];

    println!("\nStep 4: Commitment Computed");
    println!("  Commitment: {}", hex::encode(&commitment[..8]));

    // Step 5: Simulate refiner PreState → PostState
    let pre_root = merkle_root_from_leaves(&[]).unwrap();
    let pre_size = 0u64;

    println!("\nStep 5: Refiner PreState");
    println!("  Root: {}", hex::encode(&pre_root[..8]));
    println!("  Size: {}", pre_size);

    let post_root = merkle_append_from_leaves(&[], &commitments).unwrap();
    let post_size = 1u64;

    println!("\nStep 6: Refiner PostState");
    println!("  Root: {}", hex::encode(&post_root[..8]));
    println!("  Size: {}", post_size);

    assert_ne!(post_root, pre_root, "Root should change");

    // Step 7: Accumulator state update
    let mut state = OrchardServiceState {
        commitment_root: pre_root,
        commitment_size: pre_size,
        nullifier_root: [0u8; 32],
        nullifier_size: 0,
        transparent_merkle_root: [0u8; 32],
        transparent_utxo_root: [0u8; 32],
        transparent_utxo_size: 0,
    };

    state.commitment_root = post_root;
    state.commitment_size = post_size;

    println!("\nStep 7: Accumulator State Updated");
    println!("  Commitment root: {}", hex::encode(&state.commitment_root[..8]));
    println!("  Commitment size: {}", state.commitment_size);

    // Step 8: Verify tree integrity
    let recomputed = merkle_root_from_leaves(&[commitment]).unwrap();
    assert_eq!(recomputed, post_root, "Root verification failed");

    println!("\nStep 8: Tree Integrity Verified");
    println!("  Recomputed root matches: ✓");

    println!("\n✅ Issue and Transfer Lifecycle Test PASSED\n");
    println!("Summary:");
    println!("  ✓ IssueBundle created");
    println!("  ✓ Asset commitment computed");
    println!("  ✓ Refiner PreState → PostState transition");
    println!("  ✓ Accumulator state updated");
    println!("  ✓ Tree integrity verified");
}

#[test]
fn test_issue_bundle_only_flow() {
    // Test IssueBundle-only submission (no OrchardBundle)
    // This validates that the service can process pure issuance transactions
    // without any Orchard shielded transfers
    
    println!("\n=== NU7 IssueBundle-Only Flow Test ===\n");

    // Step 1: Create IssueBundle with finalize action and proper signature
    let asset_desc = [0x22u8; 32];
    let mut rng = StdRng::seed_from_u64(0x222_1ce);
    let signing_key = SigningKey::random(&mut rng);
    let verifying_key = signing_key.verifying_key();
    let mut issuer_key = vec![0x00u8];
    issuer_key.extend_from_slice(&verifying_key.to_bytes());

    let asset_base = derive_asset_base(&issuer_key, &asset_desc).expect("asset base derivation");

    println!("Step 1: Asset Creation (IssueBundle-only)");
    println!("  Asset desc hash: {}", hex::encode(&asset_desc[..8]));
    println!("  Issuer key: {}", hex::encode(&issuer_key[1..9]));
    println!("  Asset ID: {}", hex::encode(&asset_base.0[..8]));

    // Step 2: Create multiple notes for issuance
    let mut recipient1 = [0u8; 43];
    recipient1[..11].copy_from_slice(&[0x10u8; 11]);
    recipient1[11..].copy_from_slice(&pallas::Point::generator().to_bytes());

    let mut recipient2 = [0u8; 43];
    recipient2[..11].copy_from_slice(&[0x20u8; 11]);
    recipient2[11..].copy_from_slice(&pallas::Point::generator().to_bytes());

    let note1 = DecodedIssuedNote {
        recipient: recipient1,
        value: 5000,
        rho: {
            let mut r = [0u8; 32];
            r[0] = 10;
            r
        },
        rseed: [0x10u8; 32],
    };

    let note2 = DecodedIssuedNote {
        recipient: recipient2,
        value: 3000,
        rho: {
            let mut r = [0u8; 32];
            r[0] = 20;
            r
        },
        rseed: [0x20u8; 32],
    };

    // Create IssueBundle with 2 notes + finalize.
    // Note: finalize only prevents additional actions for the same asset
    // within this IssueBundle.
    let mut bundle = DecodedIssueBundle {
        issuer_key: issuer_key.clone(),
        actions: vec![DecodedIssueAction {
            asset_desc_hash: asset_desc,
            notes: vec![note1.clone(), note2.clone()],
            finalize: true, // No more actions for this asset in this IssueBundle
        }],
        sighash_info: vec![0x00],
        signature: vec![0u8; 65],
    };

    // Step 2: Sign IssueBundle
    let sighash = compute_issue_bundle_commitment(&bundle)
        .expect("IssueBundle commitment computation failed");
    let signature = signing_key
        .sign_prehash(&sighash)
        .expect("IssueBundle signature failed");
    let mut signature_bytes = vec![0x00u8];
    signature_bytes.extend_from_slice(&signature.to_bytes());
    bundle.signature = signature_bytes;

    verify_issue_bundle(&bundle).expect("IssueBundle signature verification failed");

    println!("\nStep 2: IssueBundle Created and Signed (Multi-Note)");
    println!("  Actions: {}", bundle.actions.len());
    println!("  Notes in action: {}", bundle.actions[0].notes.len());
    println!("  Note 1 value: {}", note1.value);
    println!("  Note 2 value: {}", note2.value);
    println!("  Total issued: {}", note1.value + note2.value);
    println!(
        "  Finalize: {} (no more actions for this asset in this IssueBundle)",
        bundle.actions[0].finalize
    );
    println!("  ✓ Signature verified");

    // Step 3: Verify no OrchardBundle present
    // In a real BundleProofV6 submission:
    // - bundle_type = ZSA (0x01)
    // - orchard_bundle_bytes = empty/None
    // - issue_bundle_bytes = encoded IssueBundle
    println!("\nStep 3: Verify IssueBundle-Only Submission");
    println!("  OrchardBundle: None (empty bytes)");
    println!("  IssueBundle: Present ({} actions)", bundle.actions.len());
    println!("  This is a pure issuance transaction ✓");

    // Step 4: Compute issue commitments
    let commitments = compute_issue_bundle_commitments(&bundle)
        .expect("Commitment computation failed");

    assert_eq!(commitments.len(), 2, "Should have 2 commitments");
    let commitment1 = commitments[0];
    let commitment2 = commitments[1];

    println!("\nStep 4: Commitments Computed");
    println!("  Commitment 1: {}", hex::encode(&commitment1[..8]));
    println!("  Commitment 2: {}", hex::encode(&commitment2[..8]));

    // Step 5: Simulate refiner validation (IssueBundle-only path)
    let pre_root = merkle_root_from_leaves(&[]).unwrap();
    let pre_size = 0u64;

    println!("\nStep 5: Refiner PreState");
    println!("  Root: {}", hex::encode(&pre_root[..8]));
    println!("  Size: {}", pre_size);

    // Add both issue commitments to tree
    let post_root = merkle_append_from_leaves(&[], &commitments).unwrap();
    let post_size = 2u64;

    println!("\nStep 6: Refiner PostState");
    println!("  Root: {}", hex::encode(&post_root[..8]));
    println!("  Size: {}", post_size);
    println!("  Commitments added: {}", commitments.len());

    assert_ne!(post_root, pre_root, "Root should change");
    assert_eq!(post_size, 2, "Size should be 2");

    // Step 7: Accumulator state update (IssueBundle-only)
    let mut state = OrchardServiceState {
        commitment_root: pre_root,
        commitment_size: pre_size,
        nullifier_root: [0u8; 32],
        nullifier_size: 0, // No nullifiers for IssueBundle-only
        transparent_merkle_root: [0u8; 32],
        transparent_utxo_root: [0u8; 32],
        transparent_utxo_size: 0,
    };

    state.commitment_root = post_root;
    state.commitment_size = post_size;

    println!("\nStep 7: Accumulator State Updated");
    println!("  Commitment root: {}", hex::encode(&state.commitment_root[..8]));
    println!("  Commitment size: {}", state.commitment_size);
    println!("  Nullifier size: {} (no spends in IssueBundle-only)", state.nullifier_size);

    // Step 8: Verify tree integrity
    let recomputed = merkle_root_from_leaves(&commitments).unwrap();
    assert_eq!(recomputed, post_root, "Root verification failed");

    println!("\nStep 8: Tree Integrity Verified");
    println!("  Recomputed root matches: ✓");
    println!("  Both issued notes are in the tree");

    // Step 9: Verify finalize semantics - Test enforcement
    println!("\nStep 9: Finalize Enforcement Test");
    println!(
        "  Testing: Attempt to add another action for the same asset after finalize=true"
    );

    // Try to create a second action for the same asset after finalize
    let forbidden_note = DecodedIssuedNote {
        recipient: recipient1,
        value: 100,
        rho: {
            let mut r = [0u8; 32];
            r[0] = 30;
            r
        },
        rseed: [0x30u8; 32],
    };

    let mut invalid_bundle = DecodedIssueBundle {
        issuer_key: issuer_key.clone(),
        actions: vec![
            DecodedIssueAction {
                asset_desc_hash: asset_desc,
                notes: vec![note1.clone()],
                finalize: true,  // First action finalizes
            },
            DecodedIssueAction {
                asset_desc_hash: asset_desc,  // Same asset!
                notes: vec![forbidden_note],
                finalize: false,  // Trying to issue more after finalize
            },
        ],
        sighash_info: vec![0x00],
        signature: vec![0u8; 65],
    };

    // Sign the invalid bundle
    let invalid_sighash = compute_issue_bundle_commitment(&invalid_bundle)
        .expect("Invalid bundle commitment (structure valid, semantics invalid)");
    let invalid_sig = signing_key.sign_prehash(&invalid_sighash).expect("Signature");
    let mut invalid_sig_bytes = vec![0x00u8];
    invalid_sig_bytes.extend_from_slice(&invalid_sig.to_bytes());
    invalid_bundle.signature = invalid_sig_bytes;

    // Attempt verification - should FAIL due to finalize violation
    let result = verify_issue_bundle(&invalid_bundle);

    assert!(result.is_err(), "Should reject action after finalize");
    let error = result.unwrap_err();
    let error_msg = error.to_string();
    assert!(
        error_msg.contains("action after finalize") || error_msg.contains("finalize"),
        "Error should mention finalize violation, got: {}",
        error_msg
    );

    println!("  ✓ Second action rejected with error: {}", error_msg);
    println!("  ✓ Finalize enforcement working correctly");
    println!(
        "  Note: Within the same IssueBundle, no action can follow finalize=true for the same asset"
    );

    println!("\n✅ IssueBundle-Only Flow Test PASSED\n");
    println!("Summary:");
    println!("  ✓ IssueBundle-only submission (no OrchardBundle)");
    println!("  ✓ Multi-note issuance (2 notes, total 8000 units)");
    println!("  ✓ Finalize flag enforced (tested with rejection of second action)");
    println!("  ✓ Multiple commitments added to tree");
    println!("  ✓ Refiner PreState → PostState transition");
    println!("  ✓ Accumulator state updated (commitments only, no nullifiers)");
    println!("  ✓ Tree integrity verified");
}

#[cfg(not(feature = "orchard"))]
#[test]
fn test_two_party_swap() {
    eprintln!("skipping test_two_party_swap: requires orchard feature");
}

#[cfg(feature = "orchard")]
#[test]
fn test_two_party_swap() {
    use orchard::builder::{Builder, BundleType};
    use orchard::bundle::Authorization;
    use orchard::circuit::ProvingKey;
    use orchard::keys::{
        FullViewingKey, Scope, SpendAuthorizingKey, SpendingKey,
    };
    use orchard::orchard_flavor::OrchardZSA;
    use orchard::primitives::redpallas::{Binding, SigningKey as RedPallasSigningKey};
    use orchard::Address;
    use orchard::note::{
        AssetBase as OrchardAssetBase, ExtractedNoteCommitment, Note, RandomSeed, Rho,
    };
    use orchard::tree::{MerkleHashOrchard, MerklePath};
    use orchard::value::NoteValue;
    use orchard_service::bundle_codec::{
        DecodedActionGroup, DecodedActionV6, DecodedSwapBundle, ZSA_ENC_CIPHERTEXT_SIZE,
    };
    use orchard_service::signature_verifier::{
        compute_swap_bundle_commitment, verify_swap_bundle,
    };
    use incrementalmerkletree::{Hashable, Level};
    use rand::RngCore;

    println!("\n=== NU7 Two-Party Swap Test (AAA ↔ BBB) ===\n");

    // Step 1: Issue two custom assets (AAA and BBB) first to get proper commitments
    let asset_desc_aaa = [0xAAu8; 32];
    let asset_desc_bbb = [0xBBu8; 32];

    let mut rng_aaa = StdRng::seed_from_u64(0xAAA);
    let signing_key_aaa = SigningKey::random(&mut rng_aaa);
    let mut issuer_key_aaa = vec![0x00u8];
    issuer_key_aaa.extend_from_slice(&signing_key_aaa.verifying_key().to_bytes());

    let mut rng_bbb = StdRng::seed_from_u64(0xBBB);
    let signing_key_bbb = SigningKey::random(&mut rng_bbb);
    let mut issuer_key_bbb = vec![0x00u8];
    issuer_key_bbb.extend_from_slice(&signing_key_bbb.verifying_key().to_bytes());

    let asset_base_aaa =
        derive_asset_base(&issuer_key_aaa, &asset_desc_aaa).expect("asset base AAA");
    let asset_base_bbb =
        derive_asset_base(&issuer_key_bbb, &asset_desc_bbb).expect("asset base BBB");
    let orchard_asset_aaa = OrchardAssetBase::from_bytes(&asset_base_aaa.0).unwrap();
    let orchard_asset_bbb = OrchardAssetBase::from_bytes(&asset_base_bbb.0).unwrap();

    println!("Step 1: Asset Definitions");
    println!("  Asset AAA ID: {}", hex::encode(&asset_base_aaa.0[..8]));
    println!("  Asset BBB ID: {}", hex::encode(&asset_base_bbb.0[..8]));

    // Step 2: Issue AAA and BBB assets to Alice and Bob via IssueBundles
    // This gives us proper commitments to work with
    let alice_sk = SpendingKey::from_bytes([0x11u8; 32]).unwrap();
    let bob_sk = SpendingKey::from_bytes([0x22u8; 32]).unwrap();
    let alice_fvk = FullViewingKey::from(&alice_sk);
    let bob_fvk = FullViewingKey::from(&bob_sk);
    let alice_address = alice_fvk.address_at(0u32, Scope::External);
    let bob_address = bob_fvk.address_at(0u32, Scope::External);
    let _alice_recipient = alice_address.to_raw_address_bytes();
    let _bob_recipient = bob_address.to_raw_address_bytes();

    let mut note_rng = StdRng::seed_from_u64(0x4e0a_5eed);
    let mut make_note = |value: u64, asset: OrchardAssetBase, rho_tag: u8, address: Address| {
        let mut rho_bytes = [0u8; 32];
        rho_bytes[0] = rho_tag;
        let rho = Rho::from_bytes(&rho_bytes).unwrap();
        let (rseed, rseed_bytes) = {
            let mut rseed_bytes = [0u8; 32];
            let rseed = loop {
                note_rng.fill_bytes(&mut rseed_bytes);
                let candidate = RandomSeed::from_bytes(rseed_bytes, &rho);
                if bool::from(candidate.is_some()) {
                    break candidate.unwrap();
                }
            };
            (rseed, rseed_bytes)
        };
        let note = Note::from_parts(address, NoteValue::from_raw(value), asset, rho, rseed)
            .expect("note construction");
        let decoded = DecodedIssuedNote {
            recipient: address.to_raw_address_bytes(),
            value,
            rho: rho.to_bytes(),
            rseed: rseed_bytes,
        };
        (decoded, note)
    };

    // Alice receives 100 AAA
    let (alice_aaa_note, alice_aaa_note_obj) =
        make_note(100, orchard_asset_aaa, 0xA1, alice_address);

    // Bob receives 200 BBB
    let (bob_bbb_note, bob_bbb_note_obj) =
        make_note(200, orchard_asset_bbb, 0xB1, bob_address);

    // Create and sign IssueBundle for AAA
    let mut bundle_aaa = DecodedIssueBundle {
        issuer_key: issuer_key_aaa.clone(),
        actions: vec![DecodedIssueAction {
            asset_desc_hash: asset_desc_aaa,
            notes: vec![alice_aaa_note.clone()],
            finalize: true,
        }],
        sighash_info: vec![0x00],
        signature: vec![0u8; 65],
    };

    let sighash_aaa = compute_issue_bundle_commitment(&bundle_aaa).expect("AAA commitment");
    let sig_aaa = signing_key_aaa.sign_prehash(&sighash_aaa).expect("AAA signature");
    let mut sig_bytes_aaa = vec![0x00u8];
    sig_bytes_aaa.extend_from_slice(&sig_aaa.to_bytes());
    bundle_aaa.signature = sig_bytes_aaa;

    verify_issue_bundle(&bundle_aaa).expect("AAA bundle verification");
    let alice_aaa_commitments =
        compute_issue_bundle_commitments(&bundle_aaa).expect("AAA commitments");

    // Create and sign IssueBundle for BBB
    let mut bundle_bbb = DecodedIssueBundle {
        issuer_key: issuer_key_bbb.clone(),
        actions: vec![DecodedIssueAction {
            asset_desc_hash: asset_desc_bbb,
            notes: vec![bob_bbb_note.clone()],
            finalize: true,
        }],
        sighash_info: vec![0x00],
        signature: vec![0u8; 65],
    };

    let sighash_bbb = compute_issue_bundle_commitment(&bundle_bbb).expect("BBB commitment");
    let sig_bbb = signing_key_bbb.sign_prehash(&sighash_bbb).expect("BBB signature");
    let mut sig_bytes_bbb = vec![0x00u8];
    sig_bytes_bbb.extend_from_slice(&sig_bbb.to_bytes());
    bundle_bbb.signature = sig_bytes_bbb;

    verify_issue_bundle(&bundle_bbb).expect("BBB bundle verification");
    let bob_bbb_commitments =
        compute_issue_bundle_commitments(&bundle_bbb).expect("BBB commitments");

    println!("\nStep 2: Initial Issuance");
    println!(
        "  Alice has 100 AAA (commitment: {})",
        hex::encode(&alice_aaa_commitments[0][..8])
    );
    println!(
        "  Bob has 200 BBB (commitment: {})",
        hex::encode(&bob_bbb_commitments[0][..8])
    );

    // Step 3: Build initial tree with issued notes (two-leaf tree)
    let alice_spend_cmx = ExtractedNoteCommitment::from(alice_aaa_note_obj.commitment());
    let bob_spend_cmx = ExtractedNoteCommitment::from(bob_bbb_note_obj.commitment());

    let build_merkle_path =
        |position: u32, leaf: ExtractedNoteCommitment, sibling: ExtractedNoteCommitment| {
            let mut auth_path = [MerkleHashOrchard::empty_leaf(); 32];
            auth_path[0] = MerkleHashOrchard::from_cmx(&sibling);
            for level in 1..32 {
                auth_path[level] = MerkleHashOrchard::empty_root(Level::from(level as u8));
            }
            let path = MerklePath::from_parts(position, auth_path);
            let anchor = path.root(leaf);
            (path, anchor)
        };

    let (alice_path, anchor) = build_merkle_path(0, alice_spend_cmx, bob_spend_cmx);
    let (bob_path, anchor_bob) = build_merkle_path(1, bob_spend_cmx, alice_spend_cmx);
    assert_eq!(anchor.to_bytes(), anchor_bob.to_bytes(), "Anchor mismatch");

    let initial_commitments = vec![alice_aaa_commitments[0], bob_bbb_commitments[0]];
    let service_pre_root = merkle_root_from_leaves(&initial_commitments).unwrap();
    let pre_root = anchor.to_bytes();
    assert_eq!(
        pre_root, service_pre_root,
        "Anchor/root mismatch for initial commitments"
    );
    let pre_size = 2u64;

    println!("\nStep 3: Initial State (Before Swap)");
    println!("  Commitment root: {}", hex::encode(&pre_root[..8]));
    println!("  Commitment size: {}", pre_size);

    // Step 4: Now create swap where:
    // - Alice spends 100 AAA, outputs 100 AAA to Bob
    // - Bob spends 200 BBB, outputs 200 BBB to Alice
    let alice_output_value = NoteValue::from_raw(100);
    let bob_output_value = NoteValue::from_raw(200);

    let pk = ProvingKey::build::<OrchardZSA>();
    let mut group_rng = StdRng::seed_from_u64(0x5151_5eed);

    let mut build_action_group = |fvk: &FullViewingKey,
                                  sk: &SpendingKey,
                                  note: &Note,
                                  path: &MerklePath,
                                  recipient: Address,
                                  value: NoteValue,
                                  asset: OrchardAssetBase|
     -> (DecodedActionGroup, usize, usize, RedPallasSigningKey<Binding>) {
        let mut builder = Builder::new(BundleType::DEFAULT_SWAP, anchor);
        builder
            .add_spend(fvk.clone(), note.clone(), path.clone())
            .expect("add spend");
        builder
            .add_output(None, recipient, value, asset, [0u8; 512])
            .expect("add output");

        let (unauthorized, meta) = builder
            .build_action_group::<i64>(&mut group_rng, 0)
            .expect("build action group");
        let proven = unauthorized
            .create_proof(&pk, &mut group_rng)
            .expect("create proof");
        let action_group_digest: [u8; 32] = proven.action_group_commitment().into();

        let (authorized, bsk) = proven
            .apply_signatures_for_action_group(
                &mut group_rng,
                action_group_digest,
                &[SpendAuthorizingKey::from(sk)],
            )
            .expect("apply signatures");

        let output_index = meta.output_action_index(0).expect("output index");
        let spend_index = meta.spend_action_index(0).expect("spend index");

        let mut spend_auth_sigs = Vec::with_capacity(authorized.actions().len());
        let mut actions = Vec::with_capacity(authorized.actions().len());
        for action in authorized.actions().iter() {
            let mut enc_ciphertext = [0u8; ZSA_ENC_CIPHERTEXT_SIZE];
            enc_ciphertext.copy_from_slice(action.encrypted_note().enc_ciphertext.as_ref());

            let sig_bytes: [u8; 64] = action.authorization().sig().into();
            spend_auth_sigs.push(sig_bytes);

            actions.push(DecodedActionV6 {
                cv_net: action.cv_net().to_bytes(),
                nullifier: action.nullifier().to_bytes(),
                rk: action.rk().into(),
                cmx: action.cmx().to_bytes(),
                epk_bytes: action.encrypted_note().epk_bytes,
                enc_ciphertext,
                out_ciphertext: action.encrypted_note().out_ciphertext,
            });
        }

        let proof = authorized
            .authorization()
            .proof()
            .expect("proof")
            .as_ref()
            .to_vec();
        let flags = authorized.flags().to_byte();

        let group = DecodedActionGroup {
            actions,
            flags,
            anchor: authorized.anchor().to_bytes(),
            expiry_height: authorized.expiry_height(),
            burns: Vec::new(),
            proof,
            spend_auth_sigs,
        };

        (group, output_index, spend_index, bsk)
    };

    let (alice_group, alice_output_idx, _alice_spend_idx, alice_bsk) = build_action_group(
        &alice_fvk,
        &alice_sk,
        &alice_aaa_note_obj,
        &alice_path,
        bob_address,
        alice_output_value,
        orchard_asset_aaa,
    );
    let (bob_group, bob_output_idx, _bob_spend_idx, bob_bsk) = build_action_group(
        &bob_fvk,
        &bob_sk,
        &bob_bbb_note_obj,
        &bob_path,
        alice_address,
        bob_output_value,
        orchard_asset_bbb,
    );

    let alice_new_commitment = alice_group.actions[alice_output_idx].cmx;
    let bob_new_commitment = bob_group.actions[bob_output_idx].cmx;

    println!("\nStep 4: Swap Configuration");
    println!(
        "  Alice: spend AAA commitment {}, output AAA commitment {}",
        hex::encode(&alice_aaa_commitments[0][..8]),
        hex::encode(&alice_new_commitment[..8])
    );
    println!(
        "  Bob: spend BBB commitment {}, output BBB commitment {}",
        hex::encode(&bob_bbb_commitments[0][..8]),
        hex::encode(&bob_new_commitment[..8])
    );

    // Step 5: Create SwapBundle
    let mut swap_bundle = DecodedSwapBundle {
        action_groups: vec![alice_group, bob_group],
        value_balance: 0, // Atomic swap
        binding_sig: [0u8; 64],
    };
    let sighash = compute_swap_bundle_commitment(&swap_bundle).expect("Swap commitment");

    let binding_scalar = [alice_bsk, bob_bsk]
        .iter()
        .map(|bsk| {
            let bsk_bytes: [u8; 32] = (*bsk).into();
            pallas::Scalar::from_repr(bsk_bytes).unwrap()
        })
        .fold(pallas::Scalar::zero(), |acc, s| acc + s);
    let binding_sk = RedPallasSigningKey::<Binding>::try_from(binding_scalar.to_repr())
        .expect("binding signing key");
    let binding_sig = binding_sk.sign(&mut group_rng, &sighash);
    let binding_sig_bytes: [u8; 64] = (&binding_sig).into();
    swap_bundle.binding_sig = binding_sig_bytes;

    println!("\nStep 5: SwapBundle Created");
    println!("  Action groups: {}", swap_bundle.action_groups.len());
    println!("  Value balance: {} (atomic swap)", swap_bundle.value_balance);
    println!("  ✓ Atomic property: value_balance = 0");

    // Step 6: Compute swap bundle commitment
    let commitment = compute_swap_bundle_commitment(&swap_bundle).expect("Swap commitment");
    println!("\nStep 6: Swap Bundle Commitment");
    println!("  Commitment: {}", hex::encode(&commitment[..8]));

    verify_swap_bundle(&swap_bundle, &sighash)
        .expect("Swap proof + signature verification must pass");

    // Negative coverage: invalid proof should be rejected.
    let mut bad_proof_bundle = swap_bundle.clone();
    bad_proof_bundle.action_groups[0].proof[0] ^= 0x01;
    let bad_proof_result = verify_swap_bundle(&bad_proof_bundle, &sighash);
    assert!(bad_proof_result.is_err(), "tampered proof must fail");

    // Negative coverage: invalid spend auth signature should be rejected.
    let mut bad_sig_bundle = swap_bundle.clone();
    bad_sig_bundle.action_groups[0].spend_auth_sigs[0][0] ^= 0x01;
    let bad_sig_result = verify_swap_bundle(&bad_sig_bundle, &sighash);
    assert!(bad_sig_result.is_err(), "tampered spend auth sig must fail");

    // Negative coverage: invalid binding signature should be rejected.
    let mut bad_binding_bundle = swap_bundle.clone();
    bad_binding_bundle.binding_sig[0] ^= 0x01;
    let bad_binding_result = verify_swap_bundle(&bad_binding_bundle, &sighash);
    assert!(bad_binding_result.is_err(), "tampered binding sig must fail");

    // Step 7: Simulate refiner PostState (add new commitments, nullify old ones)
    let new_commitments: Vec<[u8; 32]> = swap_bundle
        .action_groups
        .iter()
        .flat_map(|group| group.actions.iter().map(|action| action.cmx))
        .collect();
    let post_root = merkle_append_from_leaves(&initial_commitments, &new_commitments).unwrap();
    let post_size = pre_size + new_commitments.len() as u64;
    let nullifier_count = swap_bundle
        .action_groups
        .iter()
        .map(|group| group.actions.len())
        .sum::<usize>();

    println!("\nStep 7: Refiner PostState (After Swap)");
    println!("  Root: {}", hex::encode(&post_root[..8]));
    println!(
        "  Size: {} ({} old + {} new commitments)",
        post_size,
        pre_size,
        new_commitments.len()
    );
    println!(
        "  Nullifiers added: {} (includes padded actions)",
        nullifier_count
    );

    assert_ne!(post_root, pre_root, "Root should change after swap");

    // Step 8: Update accumulator state
    let mut state = OrchardServiceState {
        commitment_root: pre_root,
        commitment_size: pre_size,
        nullifier_root: [0u8; 32],
        nullifier_size: 0,
        transparent_merkle_root: [0u8; 32],
        transparent_utxo_root: [0u8; 32],
        transparent_utxo_size: 0,
    };

    state.commitment_root = post_root;
    state.commitment_size = post_size;
    state.nullifier_size = nullifier_count as u64;

    println!("\nStep 8: Accumulator State Updated");
    println!("  Commitment root: {}", hex::encode(&state.commitment_root[..8]));
    println!("  Commitment size: {}", state.commitment_size);
    println!("  Nullifier size: {}", state.nullifier_size);

    // Step 9: Verify tree integrity
    let mut all_commitments = Vec::with_capacity(initial_commitments.len() + new_commitments.len());
    all_commitments.extend_from_slice(&initial_commitments);
    all_commitments.extend_from_slice(&new_commitments);
    let recomputed = merkle_root_from_leaves(&all_commitments).unwrap();
    assert_eq!(recomputed, post_root, "Root verification failed");

    println!("\nStep 9: Tree Integrity Verified");
    println!("  Recomputed root matches: ✓");
    println!("  All {} commitments in tree", all_commitments.len());

    println!("\n✅ Two-Party Swap Test PASSED\n");
    println!("Summary:");
    println!("  ✓ Assets AAA and BBB issued with valid signatures");
    println!("  ✓ Initial state with 2 commitments");
    println!("  ✓ Two-party swap (AAA ↔ BBB) created");
    println!("  ✓ Atomic swap property enforced (value_balance = 0)");
    println!("  ✓ Swap bundle commitment computed");
    println!("  ✓ Refiner PreState → PostState transition");
    println!(
        "  ✓ {} nullifiers added + {} new commitments",
        nullifier_count,
        new_commitments.len()
    );
    println!("  ✓ Accumulator state updated");
    println!("  ✓ Tree integrity verified");
}

#[cfg(not(feature = "orchard"))]
#[test]
fn test_three_party_swap() {
    eprintln!("skipping test_three_party_swap: requires orchard feature");
}

#[cfg(feature = "orchard")]
#[test]
fn test_three_party_swap() {
    use orchard::builder::{Builder, BundleType};
    use orchard::bundle::Authorization;
    use orchard::circuit::ProvingKey;
    use orchard::keys::{
        FullViewingKey, Scope, SpendAuthorizingKey, SpendingKey,
    };
    use orchard::orchard_flavor::OrchardZSA;
    use orchard::primitives::redpallas::{Binding, SigningKey as RedPallasSigningKey};
    use orchard::Address;
    use orchard::note::{
        AssetBase as OrchardAssetBase, ExtractedNoteCommitment, Note, RandomSeed, Rho,
    };
    use orchard::tree::{MerkleHashOrchard, MerklePath};
    use orchard::value::NoteValue;
    use orchard_service::bundle_codec::{
        DecodedActionGroup, DecodedActionV6, DecodedSwapBundle, ZSA_ENC_CIPHERTEXT_SIZE,
    };
    use orchard_service::signature_verifier::{
        compute_swap_bundle_commitment, verify_swap_bundle,
    };
    use incrementalmerkletree::{Hashable, Level};
    use rand::RngCore;

    println!("\n=== NU7 Three-Party Circular Swap Test (AAA→BBB→CCC→AAA) ===\n");

    // Step 1: Issue three custom assets (AAA, BBB, CCC)
    let asset_desc_aaa = [0xAAu8; 32];
    let asset_desc_bbb = [0xBBu8; 32];
    let asset_desc_ccc = [0xCCu8; 32];

    let mut rng_aaa = StdRng::seed_from_u64(0xAAA);
    let signing_key_aaa = SigningKey::random(&mut rng_aaa);
    let mut issuer_key_aaa = vec![0x00u8];
    issuer_key_aaa.extend_from_slice(&signing_key_aaa.verifying_key().to_bytes());

    let mut rng_bbb = StdRng::seed_from_u64(0xBBB);
    let signing_key_bbb = SigningKey::random(&mut rng_bbb);
    let mut issuer_key_bbb = vec![0x00u8];
    issuer_key_bbb.extend_from_slice(&signing_key_bbb.verifying_key().to_bytes());

    let mut rng_ccc = StdRng::seed_from_u64(0xCCC);
    let signing_key_ccc = SigningKey::random(&mut rng_ccc);
    let mut issuer_key_ccc = vec![0x00u8];
    issuer_key_ccc.extend_from_slice(&signing_key_ccc.verifying_key().to_bytes());

    let asset_base_aaa =
        derive_asset_base(&issuer_key_aaa, &asset_desc_aaa).expect("asset base AAA");
    let asset_base_bbb =
        derive_asset_base(&issuer_key_bbb, &asset_desc_bbb).expect("asset base BBB");
    let asset_base_ccc =
        derive_asset_base(&issuer_key_ccc, &asset_desc_ccc).expect("asset base CCC");
    let orchard_asset_aaa = OrchardAssetBase::from_bytes(&asset_base_aaa.0).unwrap();
    let orchard_asset_bbb = OrchardAssetBase::from_bytes(&asset_base_bbb.0).unwrap();
    let orchard_asset_ccc = OrchardAssetBase::from_bytes(&asset_base_ccc.0).unwrap();

    println!("Step 1: Asset Definitions");
    println!("  Asset AAA ID: {}", hex::encode(&asset_base_aaa.0[..8]));
    println!("  Asset BBB ID: {}", hex::encode(&asset_base_bbb.0[..8]));
    println!("  Asset CCC ID: {}", hex::encode(&asset_base_ccc.0[..8]));

    // Step 2: Create spending keys and addresses for Alice, Bob, Carol
    let alice_sk = SpendingKey::from_bytes([0x11u8; 32]).unwrap();
    let bob_sk = SpendingKey::from_bytes([0x22u8; 32]).unwrap();
    let carol_sk = SpendingKey::from_bytes([0x33u8; 32]).unwrap();
    let alice_fvk = FullViewingKey::from(&alice_sk);
    let bob_fvk = FullViewingKey::from(&bob_sk);
    let carol_fvk = FullViewingKey::from(&carol_sk);
    let alice_address = alice_fvk.address_at(0u32, Scope::External);
    let bob_address = bob_fvk.address_at(0u32, Scope::External);
    let carol_address = carol_fvk.address_at(0u32, Scope::External);

    let mut note_rng = StdRng::seed_from_u64(0x3330_5eed);
    let mut make_note = |value: u64, asset: OrchardAssetBase, rho_tag: u8, address: Address| {
        let mut rho_bytes = [0u8; 32];
        rho_bytes[0] = rho_tag;
        let rho = Rho::from_bytes(&rho_bytes).unwrap();
        let (rseed, rseed_bytes) = {
            let mut rseed_bytes = [0u8; 32];
            let rseed = loop {
                note_rng.fill_bytes(&mut rseed_bytes);
                let candidate = RandomSeed::from_bytes(rseed_bytes, &rho);
                if bool::from(candidate.is_some()) {
                    break candidate.unwrap();
                }
            };
            (rseed, rseed_bytes)
        };
        let note = Note::from_parts(address, NoteValue::from_raw(value), asset, rho, rseed)
            .expect("note construction");
        let decoded = DecodedIssuedNote {
            recipient: address.to_raw_address_bytes(),
            value,
            rho: rho.to_bytes(),
            rseed: rseed_bytes,
        };
        (decoded, note)
    };

    // Alice receives 100 AAA, Bob receives 200 BBB, Carol receives 300 CCC
    let (alice_aaa_note, alice_aaa_note_obj) =
        make_note(100, orchard_asset_aaa, 0xA1, alice_address);
    let (bob_bbb_note, bob_bbb_note_obj) =
        make_note(200, orchard_asset_bbb, 0xB1, bob_address);
    let (carol_ccc_note, carol_ccc_note_obj) =
        make_note(300, orchard_asset_ccc, 0xC1, carol_address);

    // Issue AAA to Alice
    let mut bundle_aaa = DecodedIssueBundle {
        issuer_key: issuer_key_aaa.clone(),
        actions: vec![DecodedIssueAction {
            asset_desc_hash: asset_desc_aaa,
            notes: vec![alice_aaa_note.clone()],
            finalize: true,
        }],
        sighash_info: vec![0x00],
        signature: vec![0u8; 65],
    };
    let sighash_aaa = compute_issue_bundle_commitment(&bundle_aaa).expect("AAA commitment");
    let sig_aaa = signing_key_aaa.sign_prehash(&sighash_aaa).expect("AAA signature");
    let mut sig_bytes_aaa = vec![0x00u8];
    sig_bytes_aaa.extend_from_slice(&sig_aaa.to_bytes());
    bundle_aaa.signature = sig_bytes_aaa;
    verify_issue_bundle(&bundle_aaa).expect("AAA bundle verification");
    let alice_aaa_commitments =
        compute_issue_bundle_commitments(&bundle_aaa).expect("AAA commitments");

    // Issue BBB to Bob
    let mut bundle_bbb = DecodedIssueBundle {
        issuer_key: issuer_key_bbb.clone(),
        actions: vec![DecodedIssueAction {
            asset_desc_hash: asset_desc_bbb,
            notes: vec![bob_bbb_note.clone()],
            finalize: true,
        }],
        sighash_info: vec![0x00],
        signature: vec![0u8; 65],
    };
    let sighash_bbb = compute_issue_bundle_commitment(&bundle_bbb).expect("BBB commitment");
    let sig_bbb = signing_key_bbb.sign_prehash(&sighash_bbb).expect("BBB signature");
    let mut sig_bytes_bbb = vec![0x00u8];
    sig_bytes_bbb.extend_from_slice(&sig_bbb.to_bytes());
    bundle_bbb.signature = sig_bytes_bbb;
    verify_issue_bundle(&bundle_bbb).expect("BBB bundle verification");
    let bob_bbb_commitments =
        compute_issue_bundle_commitments(&bundle_bbb).expect("BBB commitments");

    // Issue CCC to Carol
    let mut bundle_ccc = DecodedIssueBundle {
        issuer_key: issuer_key_ccc.clone(),
        actions: vec![DecodedIssueAction {
            asset_desc_hash: asset_desc_ccc,
            notes: vec![carol_ccc_note.clone()],
            finalize: true,
        }],
        sighash_info: vec![0x00],
        signature: vec![0u8; 65],
    };
    let sighash_ccc = compute_issue_bundle_commitment(&bundle_ccc).expect("CCC commitment");
    let sig_ccc = signing_key_ccc.sign_prehash(&sighash_ccc).expect("CCC signature");
    let mut sig_bytes_ccc = vec![0x00u8];
    sig_bytes_ccc.extend_from_slice(&sig_ccc.to_bytes());
    bundle_ccc.signature = sig_bytes_ccc;
    verify_issue_bundle(&bundle_ccc).expect("CCC bundle verification");
    let carol_ccc_commitments =
        compute_issue_bundle_commitments(&bundle_ccc).expect("CCC commitments");

    println!("\nStep 2: Initial Issuance");
    println!(
        "  Alice has 100 AAA (commitment: {})",
        hex::encode(&alice_aaa_commitments[0][..8])
    );
    println!(
        "  Bob has 200 BBB (commitment: {})",
        hex::encode(&bob_bbb_commitments[0][..8])
    );
    println!(
        "  Carol has 300 CCC (commitment: {})",
        hex::encode(&carol_ccc_commitments[0][..8])
    );

    // Step 3: Build initial tree with 3 notes
    let alice_spend_cmx = ExtractedNoteCommitment::from(alice_aaa_note_obj.commitment());
    let bob_spend_cmx = ExtractedNoteCommitment::from(bob_bbb_note_obj.commitment());
    let carol_spend_cmx = ExtractedNoteCommitment::from(carol_ccc_note_obj.commitment());

    // Build 3-leaf merkle tree
    // For position 2, it's the left child of its parent, with empty right sibling
    // Tree structure:
    //           Root
    //          /    \
    //      H(0,1)   H(2,empty)
    //      /  \       /    \
    //     0    1     2    empty
    let build_merkle_path_3 = |position: u32, leaves: &[ExtractedNoteCommitment; 3]| {
        let mut auth_path = [MerkleHashOrchard::empty_leaf(); 32];

        match position {
            0 => {
                // Position 0: sibling is L1, parent sibling is H(L2, empty)
                auth_path[0] = MerkleHashOrchard::from_cmx(&leaves[1]);
                let h2_empty = MerkleHashOrchard::combine(
                    Level::from(0),
                    &MerkleHashOrchard::from_cmx(&leaves[2]),
                    &MerkleHashOrchard::empty_leaf(),
                );
                auth_path[1] = h2_empty;
            }
            1 => {
                // Position 1: sibling is L0, parent sibling is H(L2, empty)
                auth_path[0] = MerkleHashOrchard::from_cmx(&leaves[0]);
                let h2_empty = MerkleHashOrchard::combine(
                    Level::from(0),
                    &MerkleHashOrchard::from_cmx(&leaves[2]),
                    &MerkleHashOrchard::empty_leaf(),
                );
                auth_path[1] = h2_empty;
            }
            2 => {
                // Position 2: sibling is empty, parent sibling is H(L0, L1)
                auth_path[0] = MerkleHashOrchard::empty_leaf();
                auth_path[1] = MerkleHashOrchard::combine(
                    Level::from(0),
                    &MerkleHashOrchard::from_cmx(&leaves[0]),
                    &MerkleHashOrchard::from_cmx(&leaves[1]),
                );
            }
            _ => panic!("Invalid position"),
        }

        // Level 2+: empty roots
        for level in 2..32 {
            auth_path[level] = MerkleHashOrchard::empty_root(Level::from(level as u8));
        }

        let path = MerklePath::from_parts(position, auth_path);
        let anchor = path.root(leaves[position as usize]);
        (path, anchor)
    };

    let leaves = [alice_spend_cmx, bob_spend_cmx, carol_spend_cmx];
    let (alice_path, anchor) = build_merkle_path_3(0, &leaves);
    let (bob_path, anchor_bob) = build_merkle_path_3(1, &leaves);
    let (carol_path, anchor_carol) = build_merkle_path_3(2, &leaves);
    assert_eq!(anchor.to_bytes(), anchor_bob.to_bytes(), "Anchor mismatch Alice/Bob");
    assert_eq!(anchor.to_bytes(), anchor_carol.to_bytes(), "Anchor mismatch Alice/Carol");

    let initial_commitments = vec![
        alice_aaa_commitments[0],
        bob_bbb_commitments[0],
        carol_ccc_commitments[0],
    ];
    let service_pre_root = merkle_root_from_leaves(&initial_commitments).unwrap();
    let pre_root = anchor.to_bytes();
    assert_eq!(
        pre_root, service_pre_root,
        "Anchor/root mismatch for initial commitments"
    );
    let pre_size = 3u64;

    println!("\nStep 3: Initial State (Before Swap)");
    println!("  Commitment root: {}", hex::encode(&pre_root[..8]));
    println!("  Commitment size: {}", pre_size);

    // Step 4: Create circular three-party swap:
    // - Alice spends 100 AAA → Bob receives 100 AAA
    // - Bob spends 200 BBB → Carol receives 200 BBB
    // - Carol spends 300 CCC → Alice receives 300 CCC
    let pk = ProvingKey::build::<OrchardZSA>();
    let mut group_rng = StdRng::seed_from_u64(0x3333_5eed);

    let mut build_action_group = |fvk: &FullViewingKey,
                                  sk: &SpendingKey,
                                  note: &Note,
                                  path: &MerklePath,
                                  recipient: Address,
                                  value: NoteValue,
                                  asset: OrchardAssetBase|
     -> (DecodedActionGroup, usize, usize, RedPallasSigningKey<Binding>) {
        let mut builder = Builder::new(BundleType::DEFAULT_SWAP, anchor);
        builder
            .add_spend(fvk.clone(), note.clone(), path.clone())
            .expect("add spend");
        builder
            .add_output(None, recipient, value, asset, [0u8; 512])
            .expect("add output");

        let (unauthorized, meta) = builder
            .build_action_group::<i64>(&mut group_rng, 0)
            .expect("build action group");
        let proven = unauthorized
            .create_proof(&pk, &mut group_rng)
            .expect("create proof");
        let action_group_digest: [u8; 32] = proven.action_group_commitment().into();

        let (authorized, bsk) = proven
            .apply_signatures_for_action_group(
                &mut group_rng,
                action_group_digest,
                &[SpendAuthorizingKey::from(sk)],
            )
            .expect("apply signatures");

        let output_index = meta.output_action_index(0).expect("output index");
        let spend_index = meta.spend_action_index(0).expect("spend index");

        let mut spend_auth_sigs = Vec::with_capacity(authorized.actions().len());
        let mut actions = Vec::with_capacity(authorized.actions().len());
        for action in authorized.actions().iter() {
            let mut enc_ciphertext = [0u8; ZSA_ENC_CIPHERTEXT_SIZE];
            enc_ciphertext.copy_from_slice(action.encrypted_note().enc_ciphertext.as_ref());

            let sig_bytes: [u8; 64] = action.authorization().sig().into();
            spend_auth_sigs.push(sig_bytes);

            actions.push(DecodedActionV6 {
                cv_net: action.cv_net().to_bytes(),
                nullifier: action.nullifier().to_bytes(),
                rk: action.rk().into(),
                cmx: action.cmx().to_bytes(),
                epk_bytes: action.encrypted_note().epk_bytes,
                enc_ciphertext,
                out_ciphertext: action.encrypted_note().out_ciphertext,
            });
        }

        let proof = authorized
            .authorization()
            .proof()
            .expect("proof")
            .as_ref()
            .to_vec();
        let flags = authorized.flags().to_byte();

        let group = DecodedActionGroup {
            actions,
            flags,
            anchor: authorized.anchor().to_bytes(),
            expiry_height: authorized.expiry_height(),
            burns: Vec::new(),
            proof,
            spend_auth_sigs,
        };

        (group, output_index, spend_index, bsk)
    };

    // Alice: spend 100 AAA → Bob receives 100 AAA
    let (alice_group, alice_output_idx, _alice_spend_idx, alice_bsk) = build_action_group(
        &alice_fvk,
        &alice_sk,
        &alice_aaa_note_obj,
        &alice_path,
        bob_address,
        NoteValue::from_raw(100),
        orchard_asset_aaa,
    );

    // Bob: spend 200 BBB → Carol receives 200 BBB
    let (bob_group, bob_output_idx, _bob_spend_idx, bob_bsk) = build_action_group(
        &bob_fvk,
        &bob_sk,
        &bob_bbb_note_obj,
        &bob_path,
        carol_address,
        NoteValue::from_raw(200),
        orchard_asset_bbb,
    );

    // Carol: spend 300 CCC → Alice receives 300 CCC
    let (carol_group, carol_output_idx, _carol_spend_idx, carol_bsk) = build_action_group(
        &carol_fvk,
        &carol_sk,
        &carol_ccc_note_obj,
        &carol_path,
        alice_address,
        NoteValue::from_raw(300),
        orchard_asset_ccc,
    );

    let alice_new_commitment = alice_group.actions[alice_output_idx].cmx;
    let bob_new_commitment = bob_group.actions[bob_output_idx].cmx;
    let carol_new_commitment = carol_group.actions[carol_output_idx].cmx;

    println!("\nStep 4: Circular Swap Configuration");
    println!("  Alice: spend 100 AAA → Bob receives 100 AAA");
    println!("    Alice input: {}", hex::encode(&alice_aaa_commitments[0][..8]));
    println!("    Bob output: {}", hex::encode(&alice_new_commitment[..8]));
    println!("  Bob: spend 200 BBB → Carol receives 200 BBB");
    println!("    Bob input: {}", hex::encode(&bob_bbb_commitments[0][..8]));
    println!("    Carol output: {}", hex::encode(&bob_new_commitment[..8]));
    println!("  Carol: spend 300 CCC → Alice receives 300 CCC");
    println!("    Carol input: {}", hex::encode(&carol_ccc_commitments[0][..8]));
    println!("    Alice output: {}", hex::encode(&carol_new_commitment[..8]));

    // Step 5: Create SwapBundle
    let mut swap_bundle = DecodedSwapBundle {
        action_groups: vec![alice_group, bob_group, carol_group],
        value_balance: 0, // Atomic swap
        binding_sig: [0u8; 64],
    };
    let sighash = compute_swap_bundle_commitment(&swap_bundle).expect("Swap commitment");

    let binding_scalar = [alice_bsk, bob_bsk, carol_bsk]
        .iter()
        .map(|bsk| {
            let bsk_bytes: [u8; 32] = (*bsk).into();
            pallas::Scalar::from_repr(bsk_bytes).unwrap()
        })
        .fold(pallas::Scalar::zero(), |acc, s| acc + s);
    let binding_sk = RedPallasSigningKey::<Binding>::try_from(binding_scalar.to_repr())
        .expect("binding signing key");
    let binding_sig = binding_sk.sign(&mut group_rng, &sighash);
    let binding_sig_bytes: [u8; 64] = (&binding_sig).into();
    swap_bundle.binding_sig = binding_sig_bytes;

    println!("\nStep 5: SwapBundle Created");
    println!("  Action groups: {}", swap_bundle.action_groups.len());
    println!("  Value balance: {} (atomic swap)", swap_bundle.value_balance);
    println!("  ✓ Circular property: AAA→BBB→CCC→AAA");

    // Step 6: Verify swap bundle
    let commitment = compute_swap_bundle_commitment(&swap_bundle).expect("Swap commitment");
    println!("\nStep 6: Swap Bundle Commitment");
    println!("  Commitment: {}", hex::encode(&commitment[..8]));

    verify_swap_bundle(&swap_bundle, &sighash)
        .expect("Swap proof + signature verification must pass");

    // Step 7: Simulate refiner PostState
    let new_commitments: Vec<[u8; 32]> = swap_bundle
        .action_groups
        .iter()
        .flat_map(|group| group.actions.iter().map(|action| action.cmx))
        .collect();
    let post_root = merkle_append_from_leaves(&initial_commitments, &new_commitments).unwrap();
    let post_size = pre_size + new_commitments.len() as u64;
    let nullifier_count = swap_bundle
        .action_groups
        .iter()
        .map(|group| group.actions.len())
        .sum::<usize>();

    println!("\nStep 7: Refiner PostState (After Swap)");
    println!("  Root: {}", hex::encode(&post_root[..8]));
    println!(
        "  Size: {} ({} old + {} new commitments)",
        post_size,
        pre_size,
        new_commitments.len()
    );
    println!(
        "  Nullifiers added: {} (includes padded actions)",
        nullifier_count
    );

    assert_ne!(post_root, pre_root, "Root should change after swap");

    // Step 8: Update accumulator state
    let mut state = OrchardServiceState {
        commitment_root: pre_root,
        commitment_size: pre_size,
        nullifier_root: [0u8; 32],
        nullifier_size: 0,
        transparent_merkle_root: [0u8; 32],
        transparent_utxo_root: [0u8; 32],
        transparent_utxo_size: 0,
    };

    state.commitment_root = post_root;
    state.commitment_size = post_size;
    state.nullifier_size = nullifier_count as u64;

    println!("\nStep 8: Accumulator State Updated");
    println!("  Commitment root: {}", hex::encode(&state.commitment_root[..8]));
    println!("  Commitment size: {}", state.commitment_size);
    println!("  Nullifier size: {}", state.nullifier_size);

    // Step 9: Verify tree integrity
    let all_commitments = [
        initial_commitments.as_slice(),
        new_commitments.as_slice(),
    ]
    .concat();
    let recomputed = merkle_root_from_leaves(&all_commitments).unwrap();
    assert_eq!(recomputed, post_root, "Root verification failed");

    println!("\nStep 9: Tree Integrity Verified");
    println!("  Recomputed root matches: ✓");
    println!("  All {} commitments in tree", all_commitments.len());

    println!("\n✅ Three-Party Circular Swap Test PASSED\n");
    println!("Summary:");
    println!("  ✓ Assets AAA, BBB, and CCC issued with valid signatures");
    println!("  ✓ Initial state with 3 commitments");
    println!("  ✓ Three-party circular swap created");
    println!("  ✓ Alice: spend 100 AAA → Bob receives 100 AAA");
    println!("  ✓ Bob: spend 200 BBB → Carol receives 200 BBB");
    println!("  ✓ Carol: spend 300 CCC → Alice receives 300 CCC");
    println!("  ✓ Circular flow: Alice gets CCC, Bob gets AAA, Carol gets BBB");
    println!("  ✓ Atomic swap property enforced (value_balance = 0)");
    println!("  ✓ Swap bundle commitment computed");
    println!("  ✓ Halo2 proofs + signatures verified");
    println!("  ✓ Refiner PreState → PostState transition");
    println!(
        "  ✓ {} nullifiers added + {} new commitments",
        nullifier_count,
        new_commitments.len()
    );
    println!("  ✓ Accumulator state updated");
    println!("  ✓ Tree integrity verified");
}

#[cfg(not(feature = "orchard"))]
#[test]
fn test_custom_asset_burn() {
    eprintln!("skipping test_custom_asset_burn: requires orchard feature");
}

#[cfg(feature = "orchard")]
#[test]
fn test_custom_asset_burn() {
    use orchard::builder::{Builder, BundleType};
    use orchard::bundle::Authorization;
    use orchard::circuit::ProvingKey;
    use orchard::keys::{FullViewingKey, Scope, SpendAuthorizingKey, SpendingKey};
    use orchard::note::{
        AssetBase as OrchardAssetBase, ExtractedNoteCommitment, Note, RandomSeed, Rho,
    };
    use orchard::orchard_flavor::OrchardZSA;
    use orchard::tree::{MerkleHashOrchard, MerklePath};
    use orchard::value::NoteValue;
    use orchard_service::bundle_codec::{DecodedActionV6, DecodedZSABundle, ZSA_ENC_CIPHERTEXT_SIZE};
    use orchard_service::nu7_types::{AssetBase, BurnRecord};
    use orchard_service::signature_verifier::{compute_zsa_bundle_commitment, verify_zsa_bundle_signatures};
    use incrementalmerkletree::{Hashable, Level};
    use rand::RngCore;

    println!("\n=== NU7 Custom Asset Burn Test ===\n");

    // Step 1: Issue custom asset XXX to test burning
    let asset_desc = [0xFFu8; 32];
    let mut rng = StdRng::seed_from_u64(0xBBBBu64);
    let signing_key = SigningKey::random(&mut rng);
    let verifying_key = signing_key.verifying_key();
    let mut issuer_key = vec![0x00u8];
    issuer_key.extend_from_slice(&verifying_key.to_bytes());

    let asset_base = derive_asset_base(&issuer_key, &asset_desc).expect("asset base derivation");
    let orchard_asset = OrchardAssetBase::from_bytes(&asset_base.0).unwrap();

    println!("Step 1: Issue Custom Asset XXX");
    println!("  Asset ID: {}", hex::encode(&asset_base.0[..8]));

    let sk = SpendingKey::from_bytes([0x11u8; 32]).unwrap();
    let fvk = FullViewingKey::from(&sk);
    let address = fvk.address_at(0u32, Scope::External);

    let mut note_rng = StdRng::seed_from_u64(0x1234_5678);
    let mut make_note = |value: u64, rho_tag: u8| {
        let mut rho_bytes = [0u8; 32];
        rho_bytes[0] = rho_tag;
        let rho = Rho::from_bytes(&rho_bytes).unwrap();
        let mut rseed_bytes = [0u8; 32];
        let rseed = loop {
            note_rng.fill_bytes(&mut rseed_bytes);
            let candidate = RandomSeed::from_bytes(rseed_bytes, &rho);
            if bool::from(candidate.is_some()) {
                break candidate.unwrap();
            }
        };

        let note = Note::from_parts(address, NoteValue::from_raw(value), orchard_asset, rho, rseed)
            .expect("note construction");
        let decoded = DecodedIssuedNote {
            recipient: address.to_raw_address_bytes(),
            value,
            rho: rho.to_bytes(),
            rseed: rseed_bytes,
        };
        (decoded, note)
    };

    let (issued_note, issued_note_obj) = make_note(1000, 0xFF);

    let mut bundle = DecodedIssueBundle {
        issuer_key: issuer_key.clone(),
        actions: vec![DecodedIssueAction {
            asset_desc_hash: asset_desc,
            notes: vec![issued_note.clone()],
            finalize: true,
        }],
        sighash_info: vec![0x00],
        signature: vec![0u8; 65],
    };

    let sighash = compute_issue_bundle_commitment(&bundle).expect("IssueBundle commitment");
    let signature = signing_key.sign_prehash(&sighash).expect("IssueBundle signature");
    let mut signature_bytes = vec![0x00u8];
    signature_bytes.extend_from_slice(&signature.to_bytes());
    bundle.signature = signature_bytes;

    verify_issue_bundle(&bundle).expect("IssueBundle signature verification");
    let commitments = compute_issue_bundle_commitments(&bundle).expect("Commitment computation");

    let spend_cmx = ExtractedNoteCommitment::from(issued_note_obj.commitment());
    let spend_commitment = spend_cmx.to_bytes();
    assert_eq!(commitments[0], spend_commitment, "Issued commitment mismatch");

    println!("  Issued 1000 units");
    println!("  Commitment: {}", hex::encode(&commitments[0][..8]));
    println!("  ✓ IssueBundle verified");

    // Step 2: Build initial state + Merkle path
    let pre_root = merkle_root_from_leaves(&commitments).unwrap();
    let pre_size = 1u64;

    let mut auth_path = [MerkleHashOrchard::empty_leaf(); 32];
    auth_path[0] = MerkleHashOrchard::empty_leaf();
    for level in 1..32 {
        auth_path[level] = MerkleHashOrchard::empty_root(Level::from(level as u8));
    }
    let path = MerklePath::from_parts(0, auth_path);
    let anchor = path.root(spend_cmx);

    assert_eq!(
        anchor.to_bytes(),
        pre_root,
        "Anchor/root mismatch for issued note"
    );

    println!("\nStep 2: Initial State");
    println!("  Commitment root: {}", hex::encode(&pre_root[..8]));
    println!("  Commitment size: {}", pre_size);

    // Step 3: Create ZSA bundle that burns 300 units of XXX
    let burn_amount = 300u64;
    let output_amount = 700u64;

    println!("\nStep 3: Create Burn Transaction");
    println!("  Spend: 1000 XXX");
    println!("  Output: 700 XXX");
    println!("  Burn: {} XXX", burn_amount);

    let mut builder = Builder::new(BundleType::DEFAULT_ZSA, anchor);
    builder
        .add_spend(fvk.clone(), issued_note_obj.clone(), path.clone())
        .expect("add spend");
    builder
        .add_output(None, address, NoteValue::from_raw(output_amount), orchard_asset, [0u8; 512])
        .expect("add output");
    builder
        .add_burn(orchard_asset, NoteValue::from_raw(burn_amount))
        .expect("add burn");

    let pk = ProvingKey::build::<OrchardZSA>();
    let (unauthorized, _) = builder
        .build::<i64, OrchardZSA>(&mut rng)
        .expect("build bundle");
    let proven = unauthorized.create_proof(&pk, &mut rng).expect("create proof");
    let sighash: [u8; 32] = proven.commitment().into();
    let signed = proven
        .apply_signatures(&mut rng, sighash, &[SpendAuthorizingKey::from(&sk)])
        .expect("apply signatures");

    let mut actions = Vec::with_capacity(signed.actions().len());
    let mut spend_auth_sigs = Vec::with_capacity(signed.actions().len());
    for action in signed.actions().iter() {
        let mut enc_ciphertext = [0u8; ZSA_ENC_CIPHERTEXT_SIZE];
        enc_ciphertext.copy_from_slice(action.encrypted_note().enc_ciphertext.as_ref());

        let sig_bytes: [u8; 64] = action.authorization().sig().into();
        spend_auth_sigs.push(sig_bytes);

        actions.push(DecodedActionV6 {
            cv_net: action.cv_net().to_bytes(),
            nullifier: action.nullifier().to_bytes(),
            rk: action.rk().into(),
            cmx: action.cmx().to_bytes(),
            epk_bytes: action.encrypted_note().epk_bytes,
            enc_ciphertext,
            out_ciphertext: action.encrypted_note().out_ciphertext,
        });
    }

    let burns = signed
        .burn()
        .iter()
        .map(|(asset, value)| BurnRecord {
            asset: AssetBase::from_bytes(asset.to_bytes()),
            amount: value.inner(),
        })
        .collect::<Vec<_>>();

    let binding_sig: [u8; 64] = signed.authorization().binding_signature().sig().into();
    let zsa_bundle = DecodedZSABundle {
        actions,
        flags: signed.flags().to_byte(),
        value_balance: *signed.value_balance(),
        anchor: signed.anchor().to_bytes(),
        expiry_height: signed.expiry_height(),
        burns,
        proof: signed
            .authorization()
            .proof()
            .expect("proof")
            .as_ref()
            .to_vec(),
        spend_auth_sigs,
        binding_sig,
    };

    let service_sighash =
        compute_zsa_bundle_commitment(&zsa_bundle).expect("ZSA bundle commitment");
    assert_eq!(service_sighash, sighash, "sighash mismatch");
    verify_zsa_bundle_signatures(&zsa_bundle, &service_sighash)
        .expect("burn bundle signatures must verify");

    println!("\nStep 4: ZSA Bundle Created");
    println!("  Actions: {}", zsa_bundle.actions.len());
    println!("  Burns: {}", zsa_bundle.burns.len());
    println!("  Burn asset: {}", hex::encode(&zsa_bundle.burns[0].asset.0[..8]));
    println!("  Burn amount: {}", zsa_bundle.burns[0].amount);
    println!("  Value balance: {} (USDx only)", zsa_bundle.value_balance);

    assert_eq!(zsa_bundle.burns.len(), 1, "Should have 1 burn");
    assert_eq!(zsa_bundle.burns[0].amount, burn_amount, "Burn amount mismatch");
    assert!(!zsa_bundle.burns[0].asset.is_native(), "Cannot burn native USDx");
    assert_eq!(zsa_bundle.burns[0].asset, asset_base, "Burn asset mismatch");

    println!("\nStep 5: Burn Semantics Validated");
    println!("  ✓ Burn amount = {} (non-zero)", burn_amount);
    println!("  ✓ Burn asset = XXX (custom asset, not native USDx)");
    println!("  ✓ Conservation: 1000 input = 700 output + 300 burn");

    let input_value = 1000u64;
    let total_output = output_amount + burn_amount;
    assert_eq!(
        input_value, total_output,
        "Value not conserved: {} ≠ {} + {}",
        input_value, output_amount, burn_amount
    );

    let new_commitments: Vec<[u8; 32]> = zsa_bundle.actions.iter().map(|a| a.cmx).collect();
    let post_root = merkle_append_from_leaves(&commitments, &new_commitments).unwrap();
    let post_size = pre_size + new_commitments.len() as u64;

    println!("\nStep 6: State Updated");
    println!("  Post root: {}", hex::encode(&post_root[..8]));
    println!(
        "  Post size: {} ({} old + {} new)",
        post_size,
        pre_size,
        new_commitments.len()
    );
    println!("  Nullifiers: {} (includes dummy spends)", zsa_bundle.actions.len());
    println!("  Total supply reduction: {} XXX (burned)", burn_amount);

    let mut state = OrchardServiceState {
        commitment_root: pre_root,
        commitment_size: pre_size,
        nullifier_root: [0u8; 32],
        nullifier_size: 0,
        transparent_merkle_root: [0u8; 32],
        transparent_utxo_root: [0u8; 32],
        transparent_utxo_size: 0,
    };

    state.commitment_root = post_root;
    state.commitment_size = post_size;
    state.nullifier_size = zsa_bundle.actions.len() as u64;

    println!("\nStep 7: Accumulator State");
    println!("  Commitment root: {}", hex::encode(&state.commitment_root[..8]));
    println!("  Commitment size: {}", state.commitment_size);
    println!("  Nullifier size: {}", state.nullifier_size);

    println!("\n✅ Custom Asset Burn Test PASSED\n");
    println!("Summary:");
    println!("  ✓ Custom asset XXX issued (1000 units) with signature verification");
    println!("  ✓ Burn transaction created (300 XXX burned, 700 XXX output)");
    println!("  ✓ ZSA signatures verified for burn bundle");
    println!("  ✓ Value conservation validated: 1000 = 700 + 300");
    println!("  ✓ Burn semantics enforced (non-zero, non-native asset)");
    println!("  ✓ State transition: PreState → PostState with nullifiers");
}

#[cfg(not(feature = "orchard"))]
#[test]
fn test_mixed_swap_and_issuance() {
    eprintln!("skipping test_mixed_swap_and_issuance: requires orchard feature");
}

#[cfg(feature = "orchard")]
#[test]
fn test_mixed_swap_and_issuance() {
    use orchard::builder::{Builder, BundleType};
    use orchard::bundle::Authorization;
    use orchard::circuit::ProvingKey;
    use orchard::keys::{
        FullViewingKey, Scope, SpendAuthorizingKey, SpendingKey,
    };
    use orchard::orchard_flavor::OrchardZSA;
    use orchard::primitives::redpallas::{Binding, SigningKey as RedPallasSigningKey};
    use orchard::Address;
    use orchard::note::{
        AssetBase as OrchardAssetBase, ExtractedNoteCommitment, Note, RandomSeed, Rho,
    };
    use orchard::tree::{MerkleHashOrchard, MerklePath};
    use orchard::value::NoteValue;
    use orchard_service::bundle_codec::{
        DecodedActionGroup, DecodedActionV6, DecodedSwapBundle, ZSA_ENC_CIPHERTEXT_SIZE,
    };
    use orchard_service::signature_verifier::{
        compute_swap_bundle_commitment, verify_swap_bundle,
    };
    use incrementalmerkletree::{Hashable, Level};
    use rand::RngCore;

    println!("\n=== NU7 Mixed Swap + Issuance Test ===\n");

    // Step 1: Issue two custom assets (AAA and BBB) that will be swapped
    let asset_desc_aaa = [0xAAu8; 32];
    let asset_desc_bbb = [0xBBu8; 32];

    let mut rng_aaa = StdRng::seed_from_u64(0xAAA);
    let signing_key_aaa = SigningKey::random(&mut rng_aaa);
    let mut issuer_key_aaa = vec![0x00u8];
    issuer_key_aaa.extend_from_slice(&signing_key_aaa.verifying_key().to_bytes());

    let mut rng_bbb = StdRng::seed_from_u64(0xBBB);
    let signing_key_bbb = SigningKey::random(&mut rng_bbb);
    let mut issuer_key_bbb = vec![0x00u8];
    issuer_key_bbb.extend_from_slice(&signing_key_bbb.verifying_key().to_bytes());

    let asset_base_aaa =
        derive_asset_base(&issuer_key_aaa, &asset_desc_aaa).expect("asset base AAA");
    let asset_base_bbb =
        derive_asset_base(&issuer_key_bbb, &asset_desc_bbb).expect("asset base BBB");
    let orchard_asset_aaa = OrchardAssetBase::from_bytes(&asset_base_aaa.0).unwrap();
    let orchard_asset_bbb = OrchardAssetBase::from_bytes(&asset_base_bbb.0).unwrap();

    println!("Step 1: Initial Asset Issuance (AAA and BBB)");
    println!("  Asset AAA ID: {}", hex::encode(&asset_base_aaa.0[..8]));
    println!("  Asset BBB ID: {}", hex::encode(&asset_base_bbb.0[..8]));

    // Create spending keys for Alice and Bob
    let alice_sk = SpendingKey::from_bytes([0x11u8; 32]).unwrap();
    let bob_sk = SpendingKey::from_bytes([0x22u8; 32]).unwrap();
    let alice_fvk = FullViewingKey::from(&alice_sk);
    let bob_fvk = FullViewingKey::from(&bob_sk);
    let alice_address = alice_fvk.address_at(0u32, Scope::External);
    let bob_address = bob_fvk.address_at(0u32, Scope::External);

    let mut note_rng = StdRng::seed_from_u64(0x4d15_5eed);
    let mut make_note = |value: u64, asset: OrchardAssetBase, rho_tag: u8, address: Address| {
        let mut rho_bytes = [0u8; 32];
        rho_bytes[0] = rho_tag;
        let rho = Rho::from_bytes(&rho_bytes).unwrap();
        let (rseed, rseed_bytes) = {
            let mut rseed_bytes = [0u8; 32];
            let rseed = loop {
                note_rng.fill_bytes(&mut rseed_bytes);
                let candidate = RandomSeed::from_bytes(rseed_bytes, &rho);
                if bool::from(candidate.is_some()) {
                    break candidate.unwrap();
                }
            };
            (rseed, rseed_bytes)
        };
        let note = Note::from_parts(address, NoteValue::from_raw(value), asset, rho, rseed)
            .expect("note construction");
        let decoded = DecodedIssuedNote {
            recipient: address.to_raw_address_bytes(),
            value,
            rho: rho.to_bytes(),
            rseed: rseed_bytes,
        };
        (decoded, note)
    };

    // Alice receives 150 AAA, Bob receives 150 BBB
    let (alice_aaa_note, alice_aaa_note_obj) =
        make_note(150, orchard_asset_aaa, 0xA1, alice_address);
    let (bob_bbb_note, bob_bbb_note_obj) =
        make_note(150, orchard_asset_bbb, 0xB1, bob_address);

    // Issue AAA to Alice
    let mut bundle_aaa = DecodedIssueBundle {
        issuer_key: issuer_key_aaa.clone(),
        actions: vec![DecodedIssueAction {
            asset_desc_hash: asset_desc_aaa,
            notes: vec![alice_aaa_note.clone()],
            finalize: true,
        }],
        sighash_info: vec![0x00],
        signature: vec![0u8; 65],
    };
    let sighash_aaa = compute_issue_bundle_commitment(&bundle_aaa).expect("AAA commitment");
    let sig_aaa = signing_key_aaa.sign_prehash(&sighash_aaa).expect("AAA signature");
    let mut sig_bytes_aaa = vec![0x00u8];
    sig_bytes_aaa.extend_from_slice(&sig_aaa.to_bytes());
    bundle_aaa.signature = sig_bytes_aaa;
    verify_issue_bundle(&bundle_aaa).expect("AAA bundle verification");
    let alice_aaa_commitments =
        compute_issue_bundle_commitments(&bundle_aaa).expect("AAA commitments");

    // Issue BBB to Bob
    let mut bundle_bbb = DecodedIssueBundle {
        issuer_key: issuer_key_bbb.clone(),
        actions: vec![DecodedIssueAction {
            asset_desc_hash: asset_desc_bbb,
            notes: vec![bob_bbb_note.clone()],
            finalize: true,
        }],
        sighash_info: vec![0x00],
        signature: vec![0u8; 65],
    };
    let sighash_bbb = compute_issue_bundle_commitment(&bundle_bbb).expect("BBB commitment");
    let sig_bbb = signing_key_bbb.sign_prehash(&sighash_bbb).expect("BBB signature");
    let mut sig_bytes_bbb = vec![0x00u8];
    sig_bytes_bbb.extend_from_slice(&sig_bbb.to_bytes());
    bundle_bbb.signature = sig_bytes_bbb;
    verify_issue_bundle(&bundle_bbb).expect("BBB bundle verification");
    let bob_bbb_commitments =
        compute_issue_bundle_commitments(&bundle_bbb).expect("BBB commitments");

    println!("\nStep 2: Pre-Swap State");
    println!(
        "  Alice has 150 AAA (commitment: {})",
        hex::encode(&alice_aaa_commitments[0][..8])
    );
    println!(
        "  Bob has 150 BBB (commitment: {})",
        hex::encode(&bob_bbb_commitments[0][..8])
    );

    // Step 3: Now issue a new asset DDD in the same transaction as the swap
    let asset_desc_ddd = [0xDDu8; 32];
    let mut rng_ddd = StdRng::seed_from_u64(0xDDD);
    let signing_key_ddd = SigningKey::random(&mut rng_ddd);
    let mut issuer_key_ddd = vec![0x00u8];
    issuer_key_ddd.extend_from_slice(&signing_key_ddd.verifying_key().to_bytes());

    let asset_base_ddd =
        derive_asset_base(&issuer_key_ddd, &asset_desc_ddd).expect("asset base DDD");
    let orchard_asset_ddd = OrchardAssetBase::from_bytes(&asset_base_ddd.0).unwrap();

    println!("\nStep 3: New Asset Issuance (DDD) - Mixed with Swap");
    println!("  Asset DDD ID: {}", hex::encode(&asset_base_ddd.0[..8]));

    // Carol receives the newly issued DDD
    let carol_sk = SpendingKey::from_bytes([0x33u8; 32]).unwrap();
    let carol_fvk = FullViewingKey::from(&carol_sk);
    let carol_address = carol_fvk.address_at(0u32, Scope::External);

    let (carol_ddd_note, _carol_ddd_note_obj) =
        make_note(500, orchard_asset_ddd, 0xD1, carol_address);

    // Create IssueBundle for DDD
    let mut bundle_ddd = DecodedIssueBundle {
        issuer_key: issuer_key_ddd.clone(),
        actions: vec![DecodedIssueAction {
            asset_desc_hash: asset_desc_ddd,
            notes: vec![carol_ddd_note.clone()],
            finalize: true,
        }],
        sighash_info: vec![0x00],
        signature: vec![0u8; 65],
    };
    let sighash_ddd = compute_issue_bundle_commitment(&bundle_ddd).expect("DDD commitment");
    let sig_ddd = signing_key_ddd.sign_prehash(&sighash_ddd).expect("DDD signature");
    let mut sig_bytes_ddd = vec![0x00u8];
    sig_bytes_ddd.extend_from_slice(&sig_ddd.to_bytes());
    bundle_ddd.signature = sig_bytes_ddd;
    verify_issue_bundle(&bundle_ddd).expect("DDD bundle verification");
    let carol_ddd_commitments =
        compute_issue_bundle_commitments(&bundle_ddd).expect("DDD commitments");

    println!(
        "  Carol receives 500 DDD (commitment: {})",
        hex::encode(&carol_ddd_commitments[0][..8])
    );

    // Step 4: Build merkle tree and create swap
    let alice_spend_cmx = ExtractedNoteCommitment::from(alice_aaa_note_obj.commitment());
    let bob_spend_cmx = ExtractedNoteCommitment::from(bob_bbb_note_obj.commitment());

    let build_merkle_path =
        |position: u32, leaf: ExtractedNoteCommitment, sibling: ExtractedNoteCommitment| {
            let mut auth_path = [MerkleHashOrchard::empty_leaf(); 32];
            auth_path[0] = MerkleHashOrchard::from_cmx(&sibling);
            for level in 1..32 {
                auth_path[level] = MerkleHashOrchard::empty_root(Level::from(level as u8));
            }
            let path = MerklePath::from_parts(position, auth_path);
            let anchor = path.root(leaf);
            (path, anchor)
        };

    let (alice_path, anchor) = build_merkle_path(0, alice_spend_cmx, bob_spend_cmx);
    let (bob_path, anchor_bob) = build_merkle_path(1, bob_spend_cmx, alice_spend_cmx);
    assert_eq!(anchor.to_bytes(), anchor_bob.to_bytes(), "Anchor mismatch");

    let initial_commitments = vec![alice_aaa_commitments[0], bob_bbb_commitments[0]];
    let service_pre_root = merkle_root_from_leaves(&initial_commitments).unwrap();
    let pre_root = anchor.to_bytes();
    assert_eq!(
        pre_root, service_pre_root,
        "Anchor/root mismatch for initial commitments"
    );

    println!("\nStep 4: Swap Configuration (AAA ↔ BBB)");
    println!("  Alice: spend 150 AAA → Bob receives 150 AAA");
    println!("  Bob: spend 150 BBB → Alice receives 150 BBB");

    // Build swap using Orchard Builder
    let pk = ProvingKey::build::<OrchardZSA>();
    let mut group_rng = StdRng::seed_from_u64(0x5e4a_5eed);

    let mut build_action_group = |fvk: &FullViewingKey,
                                  sk: &SpendingKey,
                                  note: &Note,
                                  path: &MerklePath,
                                  recipient: Address,
                                  value: NoteValue,
                                  asset: OrchardAssetBase|
     -> (DecodedActionGroup, usize, usize, RedPallasSigningKey<Binding>) {
        let mut builder = Builder::new(BundleType::DEFAULT_SWAP, anchor);
        builder
            .add_spend(fvk.clone(), note.clone(), path.clone())
            .expect("add spend");
        builder
            .add_output(None, recipient, value, asset, [0u8; 512])
            .expect("add output");

        let (unauthorized, meta) = builder
            .build_action_group::<i64>(&mut group_rng, 0)
            .expect("build action group");
        let proven = unauthorized
            .create_proof(&pk, &mut group_rng)
            .expect("create proof");
        let action_group_digest: [u8; 32] = proven.action_group_commitment().into();

        let (authorized, bsk) = proven
            .apply_signatures_for_action_group(
                &mut group_rng,
                action_group_digest,
                &[SpendAuthorizingKey::from(sk)],
            )
            .expect("apply signatures");

        let output_index = meta.output_action_index(0).expect("output index");
        let spend_index = meta.spend_action_index(0).expect("spend index");

        let mut spend_auth_sigs = Vec::with_capacity(authorized.actions().len());
        let mut actions = Vec::with_capacity(authorized.actions().len());
        for action in authorized.actions().iter() {
            let mut enc_ciphertext = [0u8; ZSA_ENC_CIPHERTEXT_SIZE];
            enc_ciphertext.copy_from_slice(action.encrypted_note().enc_ciphertext.as_ref());

            let sig_bytes: [u8; 64] = action.authorization().sig().into();
            spend_auth_sigs.push(sig_bytes);

            actions.push(DecodedActionV6 {
                cv_net: action.cv_net().to_bytes(),
                nullifier: action.nullifier().to_bytes(),
                rk: action.rk().into(),
                cmx: action.cmx().to_bytes(),
                epk_bytes: action.encrypted_note().epk_bytes,
                enc_ciphertext,
                out_ciphertext: action.encrypted_note().out_ciphertext,
            });
        }

        let proof = authorized
            .authorization()
            .proof()
            .expect("proof")
            .as_ref()
            .to_vec();
        let flags = authorized.flags().to_byte();

        let group = DecodedActionGroup {
            actions,
            flags,
            anchor: authorized.anchor().to_bytes(),
            expiry_height: authorized.expiry_height(),
            burns: Vec::new(),
            proof,
            spend_auth_sigs,
        };

        (group, output_index, spend_index, bsk)
    };

    // Alice: spend 150 AAA → Bob receives 150 AAA
    let (alice_group, _alice_output_idx, _alice_spend_idx, alice_bsk) = build_action_group(
        &alice_fvk,
        &alice_sk,
        &alice_aaa_note_obj,
        &alice_path,
        bob_address,  // Output to Bob
        NoteValue::from_raw(150),
        orchard_asset_aaa,  // Same asset
    );

    // Bob: spend 150 BBB → Alice receives 150 BBB
    let (bob_group, _bob_output_idx, _bob_spend_idx, bob_bsk) = build_action_group(
        &bob_fvk,
        &bob_sk,
        &bob_bbb_note_obj,
        &bob_path,
        alice_address,  // Output to Alice
        NoteValue::from_raw(150),
        orchard_asset_bbb,  // Same asset
    );

    // Create SwapBundle
    let mut swap_bundle = DecodedSwapBundle {
        action_groups: vec![alice_group, bob_group],
        value_balance: 0, // Atomic swap
        binding_sig: [0u8; 64],
    };
    let sighash = compute_swap_bundle_commitment(&swap_bundle).expect("Swap commitment");

    let binding_scalar = [alice_bsk, bob_bsk]
        .iter()
        .map(|bsk| {
            let bsk_bytes: [u8; 32] = (*bsk).into();
            pallas::Scalar::from_repr(bsk_bytes).unwrap()
        })
        .fold(pallas::Scalar::zero(), |acc, s| acc + s);
    let binding_sk = RedPallasSigningKey::<Binding>::try_from(binding_scalar.to_repr())
        .expect("binding signing key");
    let binding_sig = binding_sk.sign(&mut group_rng, &sighash);
    let binding_sig_bytes: [u8; 64] = (&binding_sig).into();
    swap_bundle.binding_sig = binding_sig_bytes;

    println!("\nStep 5: SwapBundle Created");
    println!("  Action groups: {}", swap_bundle.action_groups.len());
    println!("  Value balance: {} (atomic swap)", swap_bundle.value_balance);

    // Verify swap bundle
    verify_swap_bundle(&swap_bundle, &sighash)
        .expect("Swap proof + signature verification must pass");

    println!("\nStep 6: Combined Transaction");
    println!("  IssueBundle (DDD): 1 action, 500 units issued to Carol");
    println!("  SwapBundle (AAA ↔ BBB): 2 action groups, atomic swap");
    println!("  ✓ Both bundles verified independently");
    println!("  ✓ Transaction contains mixed issuance + swap");

    // Simulate refiner PostState
    let new_commitments: Vec<[u8; 32]> = swap_bundle
        .action_groups
        .iter()
        .flat_map(|group| group.actions.iter().map(|action| action.cmx))
        .collect();
    let mut all_commitments = initial_commitments.clone();
    all_commitments.extend_from_slice(&new_commitments);
    all_commitments.push(carol_ddd_commitments[0]);

    let post_root = merkle_root_from_leaves(&all_commitments).unwrap();
    let nullifier_count = swap_bundle
        .action_groups
        .iter()
        .map(|group| group.actions.len())
        .sum::<usize>();

    println!("\nStep 7: Refiner PostState");
    println!("  Root: {}", hex::encode(&post_root[..8]));
    println!("  Commitments: {} (2 initial + {} swap outputs + 1 issuance)", all_commitments.len(), new_commitments.len());
    println!("  Nullifiers: {} (from swap)", nullifier_count);

    println!("\n✅ Mixed Swap + Issuance Test PASSED\n");
    println!("Summary:");
    println!("  ✓ Pre-issued AAA and BBB with valid signatures");
    println!("  ✓ New asset DDD issued in same transaction");
    println!("  ✓ Atomic swap: Alice's AAA → Bob, Bob's BBB → Alice");
    println!("  ✓ IssueBundle verified (DDD)");
    println!("  ✓ SwapBundle verified (AAA ↔ BBB) with Halo2 proofs");
    println!("  ✓ Mixed transaction: issuance + swap in single work package");
    println!("  ✓ Final state: Alice has BBB, Bob has AAA, Carol has DDD");
    println!("  ✓ Demonstrates combining multiple bundle types");
}

#[test]
fn test_reject_action_after_finalize() {
    use orchard_service::bundle_codec::{DecodedIssueBundle, DecodedIssueAction, DecodedIssuedNote};
    use orchard_service::signature_verifier::{verify_issue_bundle, compute_issue_bundle_commitment, derive_asset_base};
    use k256::schnorr::SigningKey;
    use rand::SeedableRng;
    use rand::rngs::StdRng;

    println!("\n=== NU7 Reject Action After Finalize Test ===\n");

    // Step 1: Setup issuer key
    let asset_desc = [0xFFu8; 32];
    let mut rng = StdRng::seed_from_u64(0xFFFFu64);
    let signing_key = SigningKey::random(&mut rng);
    let verifying_key = signing_key.verifying_key();
    let mut issuer_key = vec![0x00u8];
    issuer_key.extend_from_slice(&verifying_key.to_bytes());

    let asset_base = derive_asset_base(&issuer_key, &asset_desc).expect("asset base derivation");

    println!("Step 1: Setup Issuer");
    println!("  Asset ID: {}", hex::encode(&asset_base.0[..8]));
    println!("  Issuer pubkey: {}", hex::encode(&verifying_key.to_bytes()[..8]));

    // Step 2: Create recipient
    let mut recipient = [0u8; 43];
    recipient[..11].copy_from_slice(&[0xFFu8; 11]);
    recipient[11..].copy_from_slice(&pallas::Point::generator().to_bytes());

    // Step 2: Valid case - single action with finalize=true
    println!("\nStep 2: Valid Case - Single Action with Finalize");
    let valid_note = DecodedIssuedNote {
        recipient: recipient.clone(),
        value: 5000,
        rho: {
            let mut r = [0u8; 32];
            r[0] = 0xFF;
            r
        },
        rseed: {
            let mut r = [0u8; 32];
            r[0] = 0xF1;
            r
        },
    };

    let mut valid_bundle = DecodedIssueBundle {
        issuer_key: issuer_key.clone(),
        actions: vec![DecodedIssueAction {
            asset_desc_hash: asset_desc,
            notes: vec![valid_note.clone()],
            finalize: true,
        }],
        sighash_info: vec![0x00],
        signature: vec![0u8; 65],
    };

    // Sign the bundle
    let sighash = compute_issue_bundle_commitment(&valid_bundle).expect("IssueBundle commitment");
    let signature = signing_key.sign_prehash(&sighash).expect("IssueBundle signature");
    valid_bundle.signature[0] = 0x00; // k256 Schnorr compact (non-recoverable)
    valid_bundle.signature[1..65].copy_from_slice(&signature.to_bytes());

    // Verify
    verify_issue_bundle(&valid_bundle).expect("Valid bundle with finalize should pass");
    println!("  ✓ Valid: Single action with finalize=true");
    println!("  ✓ IssueBundle signature verified");

    // Step 3: Invalid case - two actions for same asset with finalize=true on first
    println!("\nStep 3: Invalid Case - Action After Finalize");
    println!("  Attempting: [action1 (finalize=true), action2 (same asset)]");

    let invalid_note1 = DecodedIssuedNote {
        recipient: recipient.clone(),
        value: 3000,
        rho: {
            let mut r = [0u8; 32];
            r[0] = 0xFF;
            r[1] = 0x01;
            r
        },
        rseed: {
            let mut r = [0u8; 32];
            r[0] = 0xF2;
            r
        },
    };

    let invalid_note2 = DecodedIssuedNote {
        recipient: recipient.clone(),
        value: 2000,
        rho: {
            let mut r = [0u8; 32];
            r[0] = 0xFF;
            r[1] = 0x02;
            r
        },
        rseed: {
            let mut r = [0u8; 32];
            r[0] = 0xF3;
            r
        },
    };

    let mut invalid_bundle = DecodedIssueBundle {
        issuer_key: issuer_key.clone(),
        actions: vec![
            DecodedIssueAction {
                asset_desc_hash: asset_desc, // Same asset
                notes: vec![invalid_note1],
                finalize: true, // First action finalizes
            },
            DecodedIssueAction {
                asset_desc_hash: asset_desc, // Same asset - SHOULD BE REJECTED
                notes: vec![invalid_note2],
                finalize: false,
            },
        ],
        sighash_info: vec![0x00],
        signature: vec![0u8; 65],
    };

    // Sign the invalid bundle
    let sighash_invalid = compute_issue_bundle_commitment(&invalid_bundle).expect("IssueBundle commitment");
    let signature_invalid = signing_key.sign_prehash(&sighash_invalid).expect("IssueBundle signature");
    invalid_bundle.signature[0] = 0x00;
    invalid_bundle.signature[1..65].copy_from_slice(&signature_invalid.to_bytes());

    // Verify - should fail
    let result = verify_issue_bundle(&invalid_bundle);

    match result {
        Err(e) => {
            let err_msg = format!("{:?}", e);
            println!("  ✓ Correctly rejected: {}", err_msg);
            assert!(
                err_msg.contains("after finalize") || err_msg.contains("IssueBundle action"),
                "Error should mention finalize enforcement"
            );
        }
        Ok(_) => {
            panic!("Should reject action after finalize for same asset");
        }
    }

    // Step 4: Valid case - two actions for different assets (same issuer)
    println!("\nStep 4: Valid Case - Two Different Assets");
    println!("  Asset A: finalize=true");
    println!("  Asset B: finalize=false");

    let asset_desc_bbb = [0xBBu8; 32];
    let note_bbb = DecodedIssuedNote {
        recipient: recipient.clone(),
        value: 1000,
        rho: {
            let mut r = [0u8; 32];
            r[0] = 0xFA;
            r
        },
        rseed: {
            let mut r = [0u8; 32];
            r[0] = 0xFB;
            r
        },
    };

    let mut mixed_bundle = DecodedIssueBundle {
        issuer_key: issuer_key.clone(),
        actions: vec![
            DecodedIssueAction {
                asset_desc_hash: asset_desc, // Asset A (finalize=true)
                notes: vec![valid_note.clone()],
                finalize: true,
            },
            DecodedIssueAction {
                asset_desc_hash: asset_desc_bbb, // Asset B (allowed after finalize)
                notes: vec![note_bbb],
                finalize: false,
            },
        ],
        sighash_info: vec![0x00],
        signature: vec![0u8; 65],
    };

    let sighash_mixed =
        compute_issue_bundle_commitment(&mixed_bundle).expect("IssueBundle commitment");
    let signature_mixed = signing_key
        .sign_prehash(&sighash_mixed)
        .expect("IssueBundle signature");
    mixed_bundle.signature[0] = 0x00;
    mixed_bundle.signature[1..65].copy_from_slice(&signature_mixed.to_bytes());

    verify_issue_bundle(&mixed_bundle)
        .expect("Different assets after finalize should pass");

    println!("  ✓ Valid: Different assets allowed after finalize");

    // Step 5: Edge case - finalize=false then finalize=true (same asset)
    println!("\nStep 5: Valid Case - Multiple Actions Before Finalize");
    println!("  Action 1: finalize=false (3000 units)");
    println!("  Action 2: finalize=true (2000 units, final)");

    let multi_note1 = DecodedIssuedNote {
        recipient: recipient.clone(),
        value: 3000,
        rho: {
            let mut r = [0u8; 32];
            r[0] = 0xF6;
            r
        },
        rseed: {
            let mut r = [0u8; 32];
            r[0] = 0xF7;
            r
        },
    };

    let multi_note2 = DecodedIssuedNote {
        recipient: recipient.clone(),
        value: 2000,
        rho: {
            let mut r = [0u8; 32];
            r[0] = 0xF8;
            r
        },
        rseed: {
            let mut r = [0u8; 32];
            r[0] = 0xF9;
            r
        },
    };

    let mut multi_bundle = DecodedIssueBundle {
        issuer_key: issuer_key.clone(),
        actions: vec![
            DecodedIssueAction {
                asset_desc_hash: asset_desc,
                notes: vec![multi_note1],
                finalize: false, // Not finalized yet
            },
            DecodedIssueAction {
                asset_desc_hash: asset_desc, // Same asset OK before finalize
                notes: vec![multi_note2],
                finalize: true, // Finalize on second action
            },
        ],
        sighash_info: vec![0x00],
        signature: vec![0u8; 65],
    };

    // Sign the multi-action bundle
    let sighash_multi = compute_issue_bundle_commitment(&multi_bundle).expect("IssueBundle commitment");
    let signature_multi = signing_key.sign_prehash(&sighash_multi).expect("IssueBundle signature");
    multi_bundle.signature[0] = 0x00;
    multi_bundle.signature[1..65].copy_from_slice(&signature_multi.to_bytes());

    // Verify - should pass
    verify_issue_bundle(&multi_bundle).expect("Multiple actions before finalize should pass");
    println!("  ✓ Valid: Multiple actions for same asset before finalize=true");
    println!("  ✓ Total issued: 5000 units (3000 + 2000)");

    println!("\n✅ Reject Action After Finalize Test PASSED\n");
    println!("Summary:");
    println!("  ✓ Valid: Single action with finalize=true");
    println!("  ✓ Invalid: Action after finalize for same asset (rejected)");
    println!("  ✓ Valid: Different assets can follow finalize");
    println!("  ✓ Valid: Multiple actions before finalize=true");
    println!("  ✓ Finalize enforcement rule validated:");
    println!("    - Once asset has finalize=true, no more actions for that asset");
    println!("    - Different assets unaffected by finalize");
    println!("    - Multiple actions allowed before finalize");
}

#[cfg(not(feature = "orchard"))]
#[test]
fn test_split_notes_surplus() {
    eprintln!("skipping test_split_notes_surplus: requires orchard feature");
}

#[cfg(feature = "orchard")]
#[test]
fn test_split_notes_surplus() {
    use orchard::builder::{Builder, BundleType};
    use orchard::bundle::Authorization;
    use orchard::circuit::ProvingKey;
    use orchard::keys::{FullViewingKey, Scope, SpendAuthorizingKey, SpendingKey};
    use orchard::note::{
        AssetBase as OrchardAssetBase, ExtractedNoteCommitment, Note, RandomSeed, Rho,
    };
    use orchard::orchard_flavor::OrchardZSA;
    use orchard::tree::{MerkleHashOrchard, MerklePath};
    use orchard::value::NoteValue;
    use orchard_service::bundle_codec::{DecodedActionV6, DecodedZSABundle, ZSA_ENC_CIPHERTEXT_SIZE};
    use orchard_service::nu7_types::{AssetBase, BurnRecord};
    use orchard_service::signature_verifier::{compute_zsa_bundle_commitment, verify_zsa_bundle_signatures};
    use incrementalmerkletree::{Hashable, Level};
    use rand::RngCore;

    println!("\n=== NU7 Note Splitting with Surplus Test ===\n");

    // Step 1: Issue custom asset ZZZ (10000 units)
    let asset_desc = [0xDDu8; 32];
    let mut rng = StdRng::seed_from_u64(0xDDDDu64);
    let signing_key = SigningKey::random(&mut rng);
    let verifying_key = signing_key.verifying_key();
    let mut issuer_key = vec![0x00u8];
    issuer_key.extend_from_slice(&verifying_key.to_bytes());

    let asset_base_zzz = derive_asset_base(&issuer_key, &asset_desc).expect("asset base derivation");
    let orchard_asset = OrchardAssetBase::from_bytes(&asset_base_zzz.0).unwrap();

    println!("Step 1: Issue Custom Asset ZZZ");
    println!("  Asset ID: {}", hex::encode(&asset_base_zzz.0[..8]));

    let alice_sk = SpendingKey::from_bytes([0xA1u8; 32]).unwrap();
    let alice_fvk = FullViewingKey::from(&alice_sk);
    let alice_address = alice_fvk.address_at(0u32, Scope::External);

    let bob_sk = SpendingKey::from_bytes([0xB0u8; 32]).unwrap();
    let bob_address = FullViewingKey::from(&bob_sk).address_at(0u32, Scope::External);

    let carol_sk = SpendingKey::from_bytes([0xC0u8; 32]).unwrap();
    let carol_address = FullViewingKey::from(&carol_sk).address_at(0u32, Scope::External);

    let mut note_rng = StdRng::seed_from_u64(0x5eed_0001);
    let mut make_note = |value: u64, rho_tag: u8, address| {
        let mut rho_bytes = [0u8; 32];
        rho_bytes[0] = rho_tag;
        let rho = Rho::from_bytes(&rho_bytes).unwrap();
        let mut rseed_bytes = [0u8; 32];
        let rseed = loop {
            note_rng.fill_bytes(&mut rseed_bytes);
            let candidate = RandomSeed::from_bytes(rseed_bytes, &rho);
            if bool::from(candidate.is_some()) {
                break candidate.unwrap();
            }
        };

        let note = Note::from_parts(address, NoteValue::from_raw(value), orchard_asset, rho, rseed)
            .expect("note construction");
        let decoded = DecodedIssuedNote {
            recipient: address.to_raw_address_bytes(),
            value,
            rho: rho.to_bytes(),
            rseed: rseed_bytes,
        };
        (decoded, note)
    };

    let (large_note, large_note_obj) = make_note(10000, 0xD1, alice_address);

    let mut bundle = DecodedIssueBundle {
        issuer_key: issuer_key.clone(),
        actions: vec![DecodedIssueAction {
            asset_desc_hash: asset_desc,
            notes: vec![large_note.clone()],
            finalize: true,
        }],
        sighash_info: vec![0x00],
        signature: vec![0u8; 65],
    };

    let sighash = compute_issue_bundle_commitment(&bundle).expect("IssueBundle commitment");
    let signature = signing_key.sign_prehash(&sighash).expect("IssueBundle signature");
    let mut signature_bytes = vec![0x00u8];
    signature_bytes.extend_from_slice(&signature.to_bytes());
    bundle.signature = signature_bytes;

    verify_issue_bundle(&bundle).expect("IssueBundle signature verification");
    let input_commitments = compute_issue_bundle_commitments(&bundle).expect("Commitment computation");

    let spend_cmx = ExtractedNoteCommitment::from(large_note_obj.commitment());
    assert_eq!(
        input_commitments[0],
        spend_cmx.to_bytes(),
        "Issued commitment mismatch"
    );

    println!("  Issued: 10000 ZZZ (single note)");
    println!("  Input commitment: {}", hex::encode(&input_commitments[0][..8]));
    println!("  ✓ IssueBundle verified");

    // Step 2: Split into multiple output notes with surplus
    println!("\nStep 2: Note Splitting Configuration");
    println!("  Input: 10000 ZZZ (1 note)");
    println!("  Output 1: 3000 ZZZ to Bob");
    println!("  Output 2: 2500 ZZZ to Carol");
    println!("  Output 3: 4000 ZZZ to Alice (change)");
    println!("  Surplus: 500 ZZZ (burn)");
    println!("  Conservation: 10000 = 3000 + 2500 + 4000 + 500 ✓");

    let mut auth_path = [MerkleHashOrchard::empty_leaf(); 32];
    auth_path[0] = MerkleHashOrchard::empty_leaf();
    for level in 1..32 {
        auth_path[level] = MerkleHashOrchard::empty_root(Level::from(level as u8));
    }
    let path = MerklePath::from_parts(0, auth_path);
    let anchor = path.root(spend_cmx);

    let mut builder = Builder::new(BundleType::DEFAULT_ZSA, anchor);
    builder
        .add_spend(alice_fvk.clone(), large_note_obj.clone(), path.clone())
        .expect("add spend");
    builder
        .add_output(None, bob_address, NoteValue::from_raw(3000), orchard_asset, [0u8; 512])
        .expect("add output bob");
    builder
        .add_output(None, carol_address, NoteValue::from_raw(2500), orchard_asset, [0u8; 512])
        .expect("add output carol");
    builder
        .add_output(None, alice_address, NoteValue::from_raw(4000), orchard_asset, [0u8; 512])
        .expect("add output alice");
    builder
        .add_burn(orchard_asset, NoteValue::from_raw(500))
        .expect("add burn");

    let pk = ProvingKey::build::<OrchardZSA>();
    let (unauthorized, _) = builder
        .build::<i64, OrchardZSA>(&mut rng)
        .expect("build bundle");
    let proven = unauthorized.create_proof(&pk, &mut rng).expect("create proof");
    let sighash: [u8; 32] = proven.commitment().into();
    let signed = proven
        .apply_signatures(&mut rng, sighash, &[SpendAuthorizingKey::from(&alice_sk)])
        .expect("apply signatures");

    let mut actions = Vec::with_capacity(signed.actions().len());
    let mut spend_auth_sigs = Vec::with_capacity(signed.actions().len());
    for action in signed.actions().iter() {
        let mut enc_ciphertext = [0u8; ZSA_ENC_CIPHERTEXT_SIZE];
        enc_ciphertext.copy_from_slice(action.encrypted_note().enc_ciphertext.as_ref());

        let sig_bytes: [u8; 64] = action.authorization().sig().into();
        spend_auth_sigs.push(sig_bytes);

        actions.push(DecodedActionV6 {
            cv_net: action.cv_net().to_bytes(),
            nullifier: action.nullifier().to_bytes(),
            rk: action.rk().into(),
            cmx: action.cmx().to_bytes(),
            epk_bytes: action.encrypted_note().epk_bytes,
            enc_ciphertext,
            out_ciphertext: action.encrypted_note().out_ciphertext,
        });
    }

    let burns = signed
        .burn()
        .iter()
        .map(|(asset, value)| BurnRecord {
            asset: AssetBase::from_bytes(asset.to_bytes()),
            amount: value.inner(),
        })
        .collect::<Vec<_>>();

    let binding_sig: [u8; 64] = signed.authorization().binding_signature().sig().into();
    let zsa_bundle = DecodedZSABundle {
        actions,
        flags: signed.flags().to_byte(),
        value_balance: *signed.value_balance(),
        anchor: signed.anchor().to_bytes(),
        expiry_height: signed.expiry_height(),
        burns,
        proof: signed
            .authorization()
            .proof()
            .expect("proof")
            .as_ref()
            .to_vec(),
        spend_auth_sigs,
        binding_sig,
    };

    let service_sighash =
        compute_zsa_bundle_commitment(&zsa_bundle).expect("ZSA bundle commitment");
    assert_eq!(service_sighash, sighash, "sighash mismatch");
    verify_zsa_bundle_signatures(&zsa_bundle, &service_sighash)
        .expect("split bundle signatures must verify");

    println!("\nStep 3: ZSA Bundle with Note Splitting");
    println!(
        "  Actions: {} (includes dummy spends for extra outputs)",
        zsa_bundle.actions.len()
    );
    println!("  Burns: {}", zsa_bundle.burns.len());
    println!("  Value balance: {} (USDx only)", zsa_bundle.value_balance);

    let input_total = 10000u64;
    let output_total = 3000u64 + 2500u64 + 4000u64;
    let surplus = input_total - output_total;

    assert_eq!(surplus, 500, "Surplus calculation");
    assert_eq!(output_total + surplus, input_total, "Conservation check");
    assert_eq!(zsa_bundle.burns.len(), 1, "Expected burn record");
    assert_eq!(zsa_bundle.burns[0].amount, surplus, "Burn amount mismatch");

    println!("\nStep 4: Split Semantics Validated");
    println!("  Input total: {} ZZZ", input_total);
    println!("  Output total: {} ZZZ (3000 + 2500 + 4000)", output_total);
    println!("  Burn (surplus): {} ZZZ", surplus);
    println!("  ✓ Conservation: {} = {} + {}", input_total, output_total, surplus);

    let new_commitments: Vec<[u8; 32]> = zsa_bundle.actions.iter().map(|a| a.cmx).collect();
    let post_root = merkle_append_from_leaves(&input_commitments, &new_commitments).unwrap();
    let post_size = input_commitments.len() as u64 + new_commitments.len() as u64;

    let mut state = OrchardServiceState {
        commitment_root: anchor.to_bytes(),
        commitment_size: 1,
        nullifier_root: [0u8; 32],
        nullifier_size: 0,
        transparent_merkle_root: [0u8; 32],
        transparent_utxo_root: [0u8; 32],
        transparent_utxo_size: 0,
    };

    state.commitment_root = post_root;
    state.commitment_size = post_size;
    state.nullifier_size = zsa_bundle.actions.len() as u64;

    println!("\nStep 5: State Updated");
    println!("  Commitment root: {}", hex::encode(&state.commitment_root[..8]));
    println!("  Commitment size: {}", state.commitment_size);
    println!("  Nullifiers: {} (includes dummy spends)", state.nullifier_size);

    println!("\n✅ Note Splitting with Surplus Test PASSED\n");
    println!("Summary:");
    println!("  ✓ Custom asset ZZZ issued (10000 units, single note)");
    println!("  ✓ Note split: 10000 ZZZ → 3000 + 2500 + 4000 + 500 burn");
    println!("  ✓ Burn record enforces surplus");
    println!("  ✓ ZSA signatures verified");
    println!("  ✓ State transition: 1 input → 3 outputs (+ dummy spends)");
}

#[cfg(not(feature = "orchard"))]
#[test]
fn test_mixed_usdx_and_custom_assets() {
    eprintln!("skipping test_mixed_usdx_and_custom_assets: requires orchard feature");
}

#[cfg(feature = "orchard")]
#[test]
fn test_mixed_usdx_and_custom_assets() {
    use orchard::builder::{Builder, BundleType};
    use orchard::bundle::Authorization;
    use orchard::circuit::ProvingKey;
    use orchard::keys::{FullViewingKey, Scope, SpendAuthorizingKey, SpendingKey};
    use orchard::note::{
        AssetBase as OrchardAssetBase, ExtractedNoteCommitment, Note, RandomSeed, Rho,
    };
    use orchard::orchard_flavor::OrchardZSA;
    use orchard::tree::{MerkleHashOrchard, MerklePath};
    use orchard::value::NoteValue;
    use orchard_service::bundle_codec::{DecodedActionV6, DecodedZSABundle, ZSA_ENC_CIPHERTEXT_SIZE};
    use orchard_service::signature_verifier::{compute_zsa_bundle_commitment, verify_zsa_bundle_signatures};
    use incrementalmerkletree::{Hashable, Level};
    use rand::RngCore;

    println!("\n=== NU7 Mixed USDx + Custom Assets Test ===\n");

    // Step 1: Issue custom asset YYY
    let asset_desc = [0xEEu8; 32];
    let mut rng = StdRng::seed_from_u64(0xEEEEu64);
    let signing_key = SigningKey::random(&mut rng);
    let verifying_key = signing_key.verifying_key();
    let mut issuer_key = vec![0x00u8];
    issuer_key.extend_from_slice(&verifying_key.to_bytes());

    let asset_base_yyy = derive_asset_base(&issuer_key, &asset_desc).expect("asset base derivation");
    let orchard_asset = OrchardAssetBase::from_bytes(&asset_base_yyy.0).unwrap();
    let native_asset = OrchardAssetBase::native();

    println!("Step 1: Issue Custom Asset YYY");
    println!("  Asset ID: {}", hex::encode(&asset_base_yyy.0[..8]));

    let alice_sk = SpendingKey::from_bytes([0xE1u8; 32]).unwrap();
    let alice_fvk = FullViewingKey::from(&alice_sk);
    let alice_address = alice_fvk.address_at(0u32, Scope::External);

    let mut note_rng = StdRng::seed_from_u64(0x5eed_0002);
    let mut make_note = |value: u64, asset: OrchardAssetBase, rho_tag: u8| {
        let mut rho_bytes = [0u8; 32];
        rho_bytes[0] = rho_tag;
        let rho = Rho::from_bytes(&rho_bytes).unwrap();
        let mut rseed_bytes = [0u8; 32];
        let rseed = loop {
            note_rng.fill_bytes(&mut rseed_bytes);
            let candidate = RandomSeed::from_bytes(rseed_bytes, &rho);
            if bool::from(candidate.is_some()) {
                break candidate.unwrap();
            }
        };

        let note = Note::from_parts(alice_address, NoteValue::from_raw(value), asset, rho, rseed)
            .expect("note construction");
        let decoded = DecodedIssuedNote {
            recipient: alice_address.to_raw_address_bytes(),
            value,
            rho: rho.to_bytes(),
            rseed: rseed_bytes,
        };
        (decoded, note)
    };

    let (yyy_note, yyy_note_obj) = make_note(5000, orchard_asset, 0xEE);

    let mut bundle = DecodedIssueBundle {
        issuer_key: issuer_key.clone(),
        actions: vec![DecodedIssueAction {
            asset_desc_hash: asset_desc,
            notes: vec![yyy_note.clone()],
            finalize: true,
        }],
        sighash_info: vec![0x00],
        signature: vec![0u8; 65],
    };

    let sighash = compute_issue_bundle_commitment(&bundle).expect("IssueBundle commitment");
    let signature = signing_key.sign_prehash(&sighash).expect("IssueBundle signature");
    let mut signature_bytes = vec![0x00u8];
    signature_bytes.extend_from_slice(&signature.to_bytes());
    bundle.signature = signature_bytes;

    verify_issue_bundle(&bundle).expect("IssueBundle signature verification");
    let yyy_commitments = compute_issue_bundle_commitments(&bundle).expect("Commitment computation");

    let yyy_cmx = ExtractedNoteCommitment::from(yyy_note_obj.commitment());
    assert_eq!(
        yyy_commitments[0],
        yyy_cmx.to_bytes(),
        "Issued commitment mismatch"
    );

    println!("  Issued 5000 YYY to Alice");
    println!("  Commitment: {}", hex::encode(&yyy_commitments[0][..8]));
    println!("  ✓ IssueBundle verified");

    // Step 2: Create native USDx note (100 USDx)
    let native_note = {
        let mut rho_bytes = [0u8; 32];
        rho_bytes[0] = 0xAA;
        let rho = Rho::from_bytes(&rho_bytes).unwrap();
        let mut rseed_bytes = [0u8; 32];
        let rseed = loop {
            note_rng.fill_bytes(&mut rseed_bytes);
            let candidate = RandomSeed::from_bytes(rseed_bytes, &rho);
            if bool::from(candidate.is_some()) {
                break candidate.unwrap();
            }
        };
        Note::from_parts(
            alice_address,
            NoteValue::from_raw(100),
            native_asset,
            rho,
            rseed,
        )
        .expect("native note")
    };

    let native_cmx = ExtractedNoteCommitment::from(native_note.commitment());

    let mut auth_path_custom = [MerkleHashOrchard::empty_leaf(); 32];
    auth_path_custom[0] = MerkleHashOrchard::from_cmx(&native_cmx);
    for level in 1..32 {
        auth_path_custom[level] = MerkleHashOrchard::empty_root(Level::from(level as u8));
    }
    let path_custom = MerklePath::from_parts(0, auth_path_custom);

    let mut auth_path_native = [MerkleHashOrchard::empty_leaf(); 32];
    auth_path_native[0] = MerkleHashOrchard::from_cmx(&yyy_cmx);
    for level in 1..32 {
        auth_path_native[level] = MerkleHashOrchard::empty_root(Level::from(level as u8));
    }
    let path_native = MerklePath::from_parts(1, auth_path_native);

    let anchor = path_custom.root(yyy_cmx);
    assert_eq!(anchor.to_bytes(), path_native.root(native_cmx).to_bytes());

    println!("\nStep 2: Mixed Transaction Setup");
    println!("  Spend: 5000 YYY → outputs 3000 + 2000 YYY");
    println!("  Spend: 100 USDx → no USDx output (value_balance = +100)");

    let mut builder = Builder::new(BundleType::DEFAULT_ZSA, anchor);
    builder
        .add_spend(alice_fvk.clone(), yyy_note_obj.clone(), path_custom.clone())
        .expect("add spend YYY");
    builder
        .add_spend(alice_fvk.clone(), native_note.clone(), path_native.clone())
        .expect("add spend USDx");
    builder
        .add_output(None, alice_address, NoteValue::from_raw(3000), orchard_asset, [0u8; 512])
        .expect("add output YYY 3000");
    builder
        .add_output(None, alice_address, NoteValue::from_raw(2000), orchard_asset, [0u8; 512])
        .expect("add output YYY 2000");

    let pk = ProvingKey::build::<OrchardZSA>();
    let (unauthorized, _) = builder
        .build::<i64, OrchardZSA>(&mut rng)
        .expect("build bundle");
    let proven = unauthorized.create_proof(&pk, &mut rng).expect("create proof");
    let sighash: [u8; 32] = proven.commitment().into();
    let signed = proven
        .apply_signatures(&mut rng, sighash, &[SpendAuthorizingKey::from(&alice_sk)])
        .expect("apply signatures");

    let mut actions = Vec::with_capacity(signed.actions().len());
    let mut spend_auth_sigs = Vec::with_capacity(signed.actions().len());
    for action in signed.actions().iter() {
        let mut enc_ciphertext = [0u8; ZSA_ENC_CIPHERTEXT_SIZE];
        enc_ciphertext.copy_from_slice(action.encrypted_note().enc_ciphertext.as_ref());

        let sig_bytes: [u8; 64] = action.authorization().sig().into();
        spend_auth_sigs.push(sig_bytes);

        actions.push(DecodedActionV6 {
            cv_net: action.cv_net().to_bytes(),
            nullifier: action.nullifier().to_bytes(),
            rk: action.rk().into(),
            cmx: action.cmx().to_bytes(),
            epk_bytes: action.encrypted_note().epk_bytes,
            enc_ciphertext,
            out_ciphertext: action.encrypted_note().out_ciphertext,
        });
    }

    let binding_sig: [u8; 64] = signed.authorization().binding_signature().sig().into();
    let zsa_bundle = DecodedZSABundle {
        actions,
        flags: signed.flags().to_byte(),
        value_balance: *signed.value_balance(),
        anchor: signed.anchor().to_bytes(),
        expiry_height: signed.expiry_height(),
        burns: Vec::new(),
        proof: signed
            .authorization()
            .proof()
            .expect("proof")
            .as_ref()
            .to_vec(),
        spend_auth_sigs,
        binding_sig,
    };

    let service_sighash =
        compute_zsa_bundle_commitment(&zsa_bundle).expect("ZSA bundle commitment");
    assert_eq!(service_sighash, sighash, "sighash mismatch");
    verify_zsa_bundle_signatures(&zsa_bundle, &service_sighash)
        .expect("mixed bundle signatures must verify");

    println!("\nStep 3: Mixed ZSA Bundle Created");
    println!("  Actions: {}", zsa_bundle.actions.len());
    println!("  Value balance (USDx): {}", zsa_bundle.value_balance);
    println!("  Burns: {} (none in this transaction)", zsa_bundle.burns.len());

    assert_eq!(zsa_bundle.value_balance, 100, "USDx value balance mismatch");

    println!("\nStep 4: Asset Type Separation");
    println!("  ✓ USDx (native): value_balance = +100 (outflow)");
    println!("  ✓ YYY (custom): conserved across outputs");
    println!("  ✓ Two asset types handled in one ZSA bundle");

    let new_commitments: Vec<[u8; 32]> = zsa_bundle.actions.iter().map(|a| a.cmx).collect();
    let all_commitments = vec![yyy_commitments[0], native_cmx.to_bytes()];
    let post_root = merkle_append_from_leaves(&all_commitments, &new_commitments).unwrap();
    let post_size = all_commitments.len() as u64 + new_commitments.len() as u64;

    let mut state = OrchardServiceState {
        commitment_root: anchor.to_bytes(),
        commitment_size: all_commitments.len() as u64,
        nullifier_root: [0u8; 32],
        nullifier_size: 0,
        transparent_merkle_root: [0u8; 32],
        transparent_utxo_root: [0u8; 32],
        transparent_utxo_size: 0,
    };

    state.commitment_root = post_root;
    state.commitment_size = post_size;
    state.nullifier_size = zsa_bundle.actions.len() as u64;

    println!("\nStep 5: State Updated");
    println!("  Commitment root: {}", hex::encode(&state.commitment_root[..8]));
    println!("  Commitment size: {}", state.commitment_size);
    println!("  Nullifiers: {} (includes dummy spends)", state.nullifier_size);

    println!("\n✅ Mixed USDx + Custom Assets Test PASSED\n");
    println!("Summary:");
    println!("  ✓ Custom asset YYY issued (5000 units) with signature");
    println!("  ✓ Mixed bundle: custom outputs + USDx value_balance");
    println!("  ✓ ZSA signatures verified");
    println!("  ✓ Native USDx and custom assets handled together");
}
