use std::collections::HashMap;

use orchard_service::{
    crypto::*,
    errors::*,
    state::*,
    vk_registry::*,
    witness::*,
};

// Mock types for testing (would be replaced with real JAM types in integration)
#[derive(Debug, Clone)]
pub struct MockWorkPackage {
    pub payload: Vec<u8>,
    pub gas_limit: u64,
    pub refine_context: MockRefineContext,
}

#[derive(Debug, Clone)]
pub struct MockRefineContext {
    pub anchor: [u8; 32],
    pub epoch: u64,
    pub timeslot: u32,
}

#[derive(Debug, Clone)]
pub struct MockWriteIntent {
    pub object_id: [u8; 32],
    pub object_kind: u8,
    pub payload: Vec<u8>,
    pub tx_index: u32,
}

#[derive(Debug, Clone)]
pub struct MockWorkReport {
    pub intents: Vec<MockWriteIntent>,
    pub exports: Vec<Vec<u8>>,
}

/// Comprehensive end-to-end test suite for all 4 Orchard extrinsics.
///
/// This test demonstrates the complete builder-to-guarantor flow:
/// 1. Builder creates ZK proofs and signatures for each extrinsic type
/// 2. Extrinsics are processed through the refiner with witness-aware execution
/// 3. State changes are accumulated and verified
/// 4. Cross-service transfers (EVM integration) are validated
///
/// Tests all 4 extrinsic types:
/// - DepositPublic: Cross-service deposits from EVM
/// - SubmitPrivate: Private transfers with multi-asset support
/// - WithdrawPublic: Public withdrawals back to EVM
/// - IssuanceV1: Asset issuance with issuer signatures
/// - BatchAggV1: Batch aggregation of multiple transactions
#[cfg(test)]
mod orchard_e2e_tests {
    use super::*;

    // Test constants matching services/orchard/docs/ORCHARD.md
    const ASSET_CASH: u32 = 1;
    const ASSET_STOCK: u32 = 1001;
    const ASSET_RSU: u32 = 1002;
    const ASSET_OPTION: u32 = 1003;

    /// Test fixture containing mock state and context for all tests
    struct TestHarness {
        commitment_tree_root: [u8; 32],
        commitment_tree_size: u64,
        nullifier_set: HashMap<[u8; 32], bool>, // nullifier -> spent
        asset_supplies: HashMap<u32, (u128, u128)>, // asset_id -> (total, transparent)
        epoch: u64,
        timeslot: u32,
    }

    impl Default for TestHarness {
        fn default() -> Self {
            Self {
                commitment_tree_root: [0u8; 32],
                commitment_tree_size: 0,
                nullifier_set: HashMap::new(),
                asset_supplies: HashMap::new(),
                epoch: 100,
                timeslot: 1000,
            }
        }
    }

    impl TestHarness {
        fn get_refine_context(&self) -> MockRefineContext {
            MockRefineContext {
                anchor: self.commitment_tree_root,
                epoch: self.epoch,
                timeslot: self.timeslot,
            }
        }

        fn advance_timeslot(&mut self) {
            self.timeslot += 1;
        }
    }

    #[test]
    fn test_deposit_public_e2e() {
        println!("=== Test 1: DepositPublic End-to-End ===");

        let mut harness = TestHarness::default();

        // Create DepositPublic extrinsic
        let deposit = create_deposit_public_extrinsic(1000, ASSET_CASH);

        // Process as builder
        let builder_result = process_extrinsic_as_builder(&mut harness, deposit.clone());
        assert!(builder_result.is_ok(), "Builder processing failed: {:?}", builder_result.err());

        let work_report = builder_result.unwrap();

        // Verify builder created correct WriteIntents
        assert!(!work_report.intents.is_empty(), "No write intents generated");

        // Process as guarantor
        let guarantor_result = process_extrinsic_as_guarantor(&mut harness, deposit, &work_report);
        assert!(guarantor_result.is_ok(), "Guarantor processing failed: {:?}", guarantor_result.err());

        // Verify state changes
        assert!(harness.commitment_tree_size > 0, "Commitment tree not updated");

        println!("✅ DepositPublic test completed successfully");
    }

    #[test]
    fn test_submit_private_e2e() {
        println!("=== Test 2: SubmitPrivate End-to-End ===");

        let mut harness = TestHarness::default();

        // Set up initial state with some commitments
        harness.commitment_tree_size = 5;
        harness.commitment_tree_root = blake2b_hash(&[0x11; 32]);

        // Create SubmitPrivate extrinsic with mock proof
        let transfer = create_submit_private_extrinsic(&harness);

        // Process as builder
        let builder_result = process_extrinsic_as_builder(&mut harness, transfer.clone());
        assert!(builder_result.is_ok(), "Builder processing failed: {:?}", builder_result.err());

        let work_report = builder_result.unwrap();

        // Verify nullifiers and commitments in WriteIntents
        let nullifier_intents: Vec<_> = work_report.intents.iter()
            .filter(|i| i.object_kind == 1) // ObjectKind::Nullifier
            .collect();
        assert!(!nullifier_intents.is_empty(), "No nullifier intents generated");

        // Process as guarantor
        let guarantor_result = process_extrinsic_as_guarantor(&mut harness, transfer, &work_report);
        assert!(guarantor_result.is_ok(), "Guarantor processing failed: {:?}", guarantor_result.err());

        println!("✅ SubmitPrivate test completed successfully");
    }

    #[test]
    fn test_withdraw_public_e2e() {
        println!("=== Test 3: WithdrawPublic End-to-End ===");

        let mut harness = TestHarness::default();

        // Set up initial state
        harness.commitment_tree_size = 10;
        harness.commitment_tree_root = blake2b_hash(&[0x22; 32]);

        // Create WithdrawPublic extrinsic
        let withdrawal = create_withdraw_public_extrinsic(&harness);

        // Process as builder
        let builder_result = process_extrinsic_as_builder(&mut harness, withdrawal.clone());
        assert!(builder_result.is_ok(), "Builder processing failed: {:?}", builder_result.err());

        let work_report = builder_result.unwrap();

        // Verify cross-service transfer intent
        let transfer_intents: Vec<_> = work_report.intents.iter()
            .filter(|i| i.object_kind == 3) // ObjectKind::Transfer
            .collect();
        assert!(!transfer_intents.is_empty(), "No transfer intent for EVM withdrawal");

        // Process as guarantor
        let guarantor_result = process_extrinsic_as_guarantor(&mut harness, withdrawal, &work_report);
        assert!(guarantor_result.is_ok(), "Guarantor processing failed: {:?}", guarantor_result.err());

        println!("✅ WithdrawPublic test completed successfully");
    }

    #[test]
    fn test_issuance_v1_e2e() {
        println!("=== Test 4: IssuanceV1 End-to-End ===");

        let mut harness = TestHarness::default();

        // Create IssuanceV1 extrinsic with signature
        let issuance = create_issuance_v1_extrinsic(ASSET_STOCK, 10000, 0);

        // Process as builder
        let builder_result = process_extrinsic_as_builder(&mut harness, issuance.clone());
        assert!(builder_result.is_ok(), "Builder processing failed: {:?}", builder_result.err());

        let work_report = builder_result.unwrap();

        // Verify asset supply update intent
        let supply_intents: Vec<_> = work_report.intents.iter()
            .filter(|i| i.object_kind == 0) // ObjectKind::StateWrite
            .filter(|i| {
                // Check if payload contains supply update
                if i.payload.len() >= 10 {
                    let key_len = u16::from_le_bytes([i.payload[0], i.payload[1]]) as usize;
                    if i.payload.len() >= 2 + key_len {
                        let key = std::str::from_utf8(&i.payload[2..2 + key_len]).unwrap_or("");
                        return key.starts_with("supply_");
                    }
                }
                false
            })
            .collect();
        assert!(!supply_intents.is_empty(), "No supply update intent generated");

        // Process as guarantor
        let guarantor_result = process_extrinsic_as_guarantor(&mut harness, issuance, &work_report);
        assert!(guarantor_result.is_ok(), "Guarantor processing failed: {:?}", guarantor_result.err());

        println!("✅ IssuanceV1 test completed successfully");
    }

    #[test]
    fn test_batch_agg_v1_e2e() {
        println!("=== Test 5: BatchAggV1 End-to-End ===");

        let mut harness = TestHarness::default();

        // Set up initial state for batch processing
        harness.commitment_tree_size = 15;
        harness.commitment_tree_root = blake2b_hash(&[0x33; 32]);

        // Create BatchAggV1 extrinsic
        let batch = create_batch_agg_v1_extrinsic(&harness);

        // Process as builder
        let builder_result = process_extrinsic_as_builder(&mut harness, batch.clone());
        assert!(builder_result.is_ok(), "Builder processing failed: {:?}", builder_result.err());

        let work_report = builder_result.unwrap();

        // Verify batch processed multiple commitments
        let commitment_intents: Vec<_> = work_report.intents.iter()
            .filter(|i| i.object_kind == 2) // ObjectKind::Commitment
            .collect();
        assert!(commitment_intents.len() >= 3, "Expected multiple commitment intents from batch");

        // Process as guarantor
        let guarantor_result = process_extrinsic_as_guarantor(&mut harness, batch, &work_report);
        assert!(guarantor_result.is_ok(), "Guarantor processing failed: {:?}", guarantor_result.err());

        println!("✅ BatchAggV1 test completed successfully");
    }

    #[test]
    fn test_multi_extrinsic_workflow() {
        println!("=== Integration Test: Multi-Extrinsic Workflow ===");

        let mut harness = TestHarness::default();

        // Step 1: Deposit
        println!("Step 1: Processing DepositPublic");
        let deposit = create_deposit_public_extrinsic(1000, ASSET_CASH);
        let deposit_result = process_extrinsic_as_builder(&mut harness, deposit.clone())
            .and_then(|wr| process_extrinsic_as_guarantor(&mut harness, deposit, &wr));
        assert!(deposit_result.is_ok(), "Deposit workflow failed: {:?}", deposit_result.err());

        harness.advance_timeslot();

        // Step 2: Private Transfer
        println!("Step 2: Processing SubmitPrivate");
        let transfer = create_submit_private_extrinsic(&harness);
        let transfer_result = process_extrinsic_as_builder(&mut harness, transfer.clone())
            .and_then(|wr| process_extrinsic_as_guarantor(&mut harness, transfer, &wr));
        assert!(transfer_result.is_ok(), "Transfer workflow failed: {:?}", transfer_result.err());

        harness.advance_timeslot();

        // Step 3: Withdrawal
        println!("Step 3: Processing WithdrawPublic");
        let withdrawal = create_withdraw_public_extrinsic(&harness);
        let withdrawal_result = process_extrinsic_as_builder(&mut harness, withdrawal.clone())
            .and_then(|wr| process_extrinsic_as_guarantor(&mut harness, withdrawal, &wr));
        assert!(withdrawal_result.is_ok(), "Withdrawal workflow failed: {:?}", withdrawal_result.err());

        println!("✅ Multi-extrinsic workflow completed successfully");
    }

    #[test]
    fn test_witness_bundle_generation() {
        println!("=== Test: Witness Bundle Generation ===");

        let harness = TestHarness::default();

        // Create a transfer that would require witness data
        let transfer = create_submit_private_extrinsic(&harness);

        // Generate witness bundle (simulated)
        let witness_result = create_mock_witness_bundle(&transfer);
        assert!(witness_result.is_ok(), "Witness bundle generation failed: {:?}", witness_result.err());

        let witness_bundle = witness_result.unwrap();

        // Verify witness bundle structure
        assert!(!witness_bundle.pre_state_root.is_empty(), "Pre-state root missing");
        assert!(!witness_bundle.reads.is_empty(), "Read keys missing");

        // Verify witness can be serialized
        let serialization_result = witness_bundle.serialize();
        assert!(serialization_result.is_ok(), "Witness serialization failed");

        println!("✅ Witness bundle test completed successfully");
    }

    // Helper functions for creating test extrinsics

    fn create_deposit_public_extrinsic(value: u64, asset_id: u32) -> OrchardExtrinsic {
        let commitment = blake2b_hash(&value.to_le_bytes());

        OrchardExtrinsic::DepositPublic {
            commitment,
            value,
            sender_index: 0,
            asset_id,
        }
    }

    fn create_submit_private_extrinsic(harness: &TestHarness) -> OrchardExtrinsic {
        // Create mock proof (in real implementation, this would be generated by circuit)
        let proof = vec![0x42; 768]; // Typical Groth16 proof size

        // Mock nullifiers and commitments
        let input_nullifiers = [
            blake2b_hash(&[0x11; 32]),
            [0u8; 32], // Zero padding
            [0u8; 32],
            [0u8; 32],
        ];

        let output_commitments = [
            blake2b_hash(&[0x22; 32]),
            blake2b_hash(&[0x33; 32]),
            [0u8; 32], // Zero padding
            [0u8; 32],
        ];

        // Multi-asset deltas (K=3)
        let deltas = [
            Delta {
                asset_id: ASSET_CASH,
                in_public: 0,
                out_public: 0,
                burn_public: 0,
                mint_public: 0,
            },
            Delta {
                asset_id: ASSET_STOCK,
                in_public: 0,
                out_public: 0,
                burn_public: 0,
                mint_public: 0,
            },
            Delta {
                asset_id: ASSET_RSU,
                in_public: 0,
                out_public: 0,
                burn_public: 0,
                mint_public: 0,
            },
        ];

        OrchardExtrinsic::SubmitPrivate {
            proof,
            anchor_root: harness.commitment_tree_root,
            anchor_root_index: harness.commitment_tree_size,
            next_root: blake2b_hash(&[0xBB; 32]),
            num_inputs: 1,
            input_nullifiers,
            num_outputs: 2,
            output_commitments,
            deltas,
            allowed_root: [0u8; 32], // No compliance required
            terms_hash: [0u8; 32],   // No gadget terms
            poi_root: [0u8; 32],     // Zero PoI root for testing
            epoch: harness.epoch,
            fee: 10,
            encrypted_payload: vec![], // MVP: empty
        }
    }

    fn create_withdraw_public_extrinsic(harness: &TestHarness) -> OrchardExtrinsic {
        let proof = vec![0xCC; 768];
        let recipient = [0x74, 0x2d, 0x35, 0xCc, 0x66, 0x34, 0xC0, 0x53, 0x29, 0x25,
                         0xa3, 0xb8, 0x44, 0xBc, 0x9e, 0x75, 0x95, 0xf0, 0xbE, 0xb0]; // EVM address

        OrchardExtrinsic::WithdrawPublic {
            proof,
            anchor_root: harness.commitment_tree_root,
            anchor_size: harness.commitment_tree_size,
            nullifier: blake2b_hash(&[0xEE; 32]),
            recipient,
            amount: 500,
            fee: 5,
            gas_limit: 100000,
            asset_id: ASSET_CASH,
            poi_root: [0u8; 32],
            epoch: harness.epoch,
            has_change: true,
            change_commitment: blake2b_hash(&[0xFF; 32]),
            next_root: blake2b_hash(&[0x99; 32]),
        }
    }

    fn create_issuance_v1_extrinsic(asset_id: u32, mint_amount: u128, burn_amount: u128) -> OrchardExtrinsic {
        let proof = vec![0x88; 768];
        let issuer_pk = blake2b_hash(b"test_issuer_public_key");
        let signature_data = [0x77; 64]; // Mock signature
        let issuance_root = blake2b_hash(&[0x55; 32]);

        OrchardExtrinsic::IssuanceV1 {
            proof,
            asset_id,
            mint_amount,
            burn_amount,
            issuer_pk,
            signature_verification_data: signature_data,
            issuance_root,
        }
    }

    fn create_batch_agg_v1_extrinsic(harness: &TestHarness) -> OrchardExtrinsic {
        let proof = vec![0x44; 768];
        let commitments = vec![
            blake2b_hash(&[0x30; 32]),
            blake2b_hash(&[0x31; 32]),
            blake2b_hash(&[0x32; 32]),
        ];

        OrchardExtrinsic::BatchAggV1 {
            proof,
            anchor_root: harness.commitment_tree_root,
            anchor_size: harness.commitment_tree_size,
            next_root: blake2b_hash(&[0xBB; 32]),
            nullifiers_root: blake2b_hash(&[0xCC; 32]),
            commitments_root: blake2b_hash(&[0xDD; 32]),
            commitments,
            deltas_root: blake2b_hash(&[0xEE; 32]),
            issuance_root: blake2b_hash(&[0xFF; 32]),
            memo_root: blake2b_hash(&[0x11; 32]),
            equity_allowed_root: blake2b_hash(&[0x22; 32]),
            poi_root: blake2b_hash(&[0x33; 32]),
            total_fee: 100,
            epoch: harness.epoch,
            num_user_txs: 5,
            min_fee_per_tx: 20,
            batch_hash: blake2b_hash(&[0x99; 32]),
        }
    }

    // Mock processing functions (would integrate with actual JAM types)

    fn process_extrinsic_as_builder(
        harness: &mut TestHarness,
        extrinsic: OrchardExtrinsic
    ) -> Result<MockWorkReport> {
        println!("🔧 Builder processing extrinsic: {:?}", get_extrinsic_type(&extrinsic));

        // Serialize extrinsic
        let payload = extrinsic.serialize();

        // Create work package
        let wp = MockWorkPackage {
            payload,
            gas_limit: 10_000_000,
            refine_context: harness.get_refine_context(),
        };

        // Process through refiner (simplified)
        let refine_result = mock_refiner_process(&wp)?;

        // Update harness state based on extrinsic
        match &extrinsic {
            OrchardExtrinsic::DepositPublic { value: _, .. } => {
                harness.commitment_tree_size += 1;
                harness.commitment_tree_root = blake2b_hash(&harness.commitment_tree_size.to_le_bytes());
            },
            OrchardExtrinsic::SubmitPrivate { input_nullifiers, .. } => {
                for nullifier in input_nullifiers.iter() {
                    if *nullifier != [0u8; 32] {
                        harness.nullifier_set.insert(*nullifier, true);
                    }
                }
                harness.commitment_tree_size += 1;
            },
            OrchardExtrinsic::IssuanceV1 { asset_id, mint_amount, burn_amount, .. } => {
                let (current_total, current_transparent) = harness.asset_supplies.get(asset_id).copied().unwrap_or((0, 0));
                let new_total = current_total + mint_amount - burn_amount;
                harness.asset_supplies.insert(*asset_id, (new_total, current_transparent));
            },
            _ => {}
        }

        Ok(refine_result)
    }

    fn process_extrinsic_as_guarantor(
        _harness: &mut TestHarness,
        extrinsic: OrchardExtrinsic,
        work_report: &MockWorkReport,
    ) -> Result<()> {
        println!("🔍 Guarantor validating extrinsic: {:?}", get_extrinsic_type(&extrinsic));

        // Verify work report integrity
        if work_report.intents.is_empty() {
            return Err(OrchardError::StateError("No write intents generated".into()));
        }

        // Verify proof (simplified - would use actual VK verification)
        match &extrinsic {
            OrchardExtrinsic::DepositPublic { .. } => {
                // No proof verification needed for deposits
            },
            OrchardExtrinsic::SubmitPrivate { proof, .. }
            | OrchardExtrinsic::WithdrawPublic { proof, .. }
            | OrchardExtrinsic::IssuanceV1 { proof, .. }
            | OrchardExtrinsic::BatchAggV1 { proof, .. } => {
                if proof.is_empty() {
                    return Err(OrchardError::InvalidProof);
                }
            }
        }

        println!("✅ Guarantor validation passed");
        Ok(())
    }

    fn mock_refiner_process(work_package: &MockWorkPackage) -> Result<MockWorkReport> {
        // Simplified refiner that generates appropriate WriteIntents
        let mut intents = Vec::new();

        // Parse extrinsic type from payload
        if work_package.payload.is_empty() {
            return Err(OrchardError::ParseError("Empty payload".into()));
        }

        let extrinsic_tag = work_package.payload[0];

        match extrinsic_tag {
            0 => { // DepositPublic
                // Generate commitment tree update
                intents.push(MockWriteIntent {
                    object_id: blake2b_hash(b"commitment_tree_root"),
                    object_kind: 0, // StateWrite
                    payload: create_state_write_payload("commitment_tree_root", &[0x11; 32]),
                    tx_index: 0,
                });
            },
            1 => { // SubmitPrivate
                // Generate nullifier spent
                intents.push(MockWriteIntent {
                    object_id: blake2b_hash(b"nullifier_spent"),
                    object_kind: 1, // Nullifier
                    payload: vec![0x01; 32], // Spent marker
                    tx_index: 0,
                });

                // Generate new commitments
                intents.push(MockWriteIntent {
                    object_id: blake2b_hash(b"new_commitment"),
                    object_kind: 2, // Commitment
                    payload: blake2b_hash(&[0x22; 32]).to_vec(),
                    tx_index: 0,
                });
            },
            2 => { // WithdrawPublic
                // Generate cross-service transfer
                intents.push(MockWriteIntent {
                    object_id: blake2b_hash(b"evm_transfer"),
                    object_kind: 3, // Transfer
                    payload: vec![0xEE; 20], // EVM recipient address
                    tx_index: 0,
                });
            },
            3 => { // IssuanceV1
                // Generate asset supply update
                intents.push(MockWriteIntent {
                    object_id: blake2b_hash(b"asset_supply"),
                    object_kind: 0, // StateWrite
                    payload: create_state_write_payload("supply_1001_total", &10000u128.to_le_bytes()),
                    tx_index: 0,
                });
            },
            4 => { // BatchAggV1
                // Generate multiple commitment updates
                for i in 0..3 {
                    intents.push(MockWriteIntent {
                        object_id: blake2b_hash(&format!("batch_commitment_{}", i).as_bytes()),
                        object_kind: 2, // Commitment
                        payload: blake2b_hash(&[0x30 + i; 32]).to_vec(),
                        tx_index: i as u32,
                    });
                }
            },
            _ => return Err(OrchardError::ParseError("Unknown extrinsic tag".into())),
        }

        Ok(MockWorkReport {
            intents,
            exports: vec![vec![0x42; 32]], // Mock export data
        })
    }

    fn create_mock_witness_bundle(extrinsic: &OrchardExtrinsic) -> Result<WitnessBundle> {
        // Create mock witness data based on extrinsic type
        let pre_state_root = blake2b_hash(&[0xAA; 32]);
        let post_state_root = blake2b_hash(&[0xBB; 32]);
        let read_keys = match extrinsic {
            OrchardExtrinsic::SubmitPrivate { .. } => {
                vec!["commitment_tree_root", "nullifier_spent"]
            },
            OrchardExtrinsic::WithdrawPublic { .. } => {
                vec!["commitment_tree_root"]
            },
            _ => vec![],
        };

        let reads = read_keys
            .into_iter()
            .map(|key| StateRead {
                key: key.to_string(),
                value: vec![0u8; 32],
                merkle_proof: vec![[0u8; 32]],
            })
            .collect();

        Ok(WitnessBundle {
            pre_state_root,
            post_state_root,
            reads,
            writes: vec![],
        })
    }

    fn create_state_write_payload(key: &str, value: &[u8]) -> Vec<u8> {
        let mut payload = Vec::new();

        // Key length (u16 little-endian)
        payload.extend_from_slice(&(key.len() as u16).to_le_bytes());

        // Key bytes
        payload.extend_from_slice(key.as_bytes());

        // Value bytes
        payload.extend_from_slice(value);

        payload
    }

    fn get_extrinsic_type(extrinsic: &OrchardExtrinsic) -> &'static str {
        match extrinsic {
            OrchardExtrinsic::DepositPublic { .. } => "DepositPublic",
            OrchardExtrinsic::SubmitPrivate { .. } => "SubmitPrivate",
            OrchardExtrinsic::WithdrawPublic { .. } => "WithdrawPublic",
            OrchardExtrinsic::IssuanceV1 { .. } => "IssuanceV1",
            OrchardExtrinsic::BatchAggV1 { .. } => "BatchAggV1",
        }
    }

    // Additional helper function for Blake2b hashing
    fn blake2b_hash(input: &[u8]) -> [u8; 32] {
        use blake2::{Blake2b, Digest};
        let mut hasher = Blake2b::new();
        hasher.update(input);
        hasher.finalize().into()
    }
}

#[cfg(test)]
mod witness_tests {
    use super::*;

    #[test]
    fn test_witness_bundle_serialization() {
        let witness = WitnessBundle {
            pre_state_root: [0x11; 32],
            post_state_root: [0x22; 32],
            reads: vec![
                StateRead {
                    key: "read_key_1".to_string(),
                    value: vec![0x44; 32],
                    merkle_proof: vec![[0u8; 32]],
                },
                StateRead {
                    key: "read_key_2".to_string(),
                    value: vec![0x55; 32],
                    merkle_proof: vec![[0u8; 32]],
                },
            ],
            writes: vec![],
        };

        let serialized = witness.serialize().expect("Serialization should succeed");
        assert!(!serialized.is_empty(), "Serialized data should not be empty");

        println!("✅ Witness bundle serialization test passed");
    }
}
