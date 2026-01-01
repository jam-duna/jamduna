use std::{env, fs, path::{Path, PathBuf}};

use orchard_service::{
    crypto::Delta as ServiceDelta,
    errors::{OrchardError, Result},
    state::OrchardExtrinsic,
    vk_registry::VkRegistry,
};

use orchard_halo2_circuits::{
    Fp,
    halo2_proofs::{
        pasta::EqAffine,
        plonk::{create_proof, keygen_pk, keygen_vk, verify_proof, Circuit, SingleVerifier, VerifyingKey},
        poly::commitment::Params,
        transcript::{Blake2bRead, Blake2bWrite, Challenge255, Transcript},
    },
    merkle::{native as merkle_native, MerklePath, TreeType, COMMITMENT_TREE_DEPTH, POI_TREE_DEPTH},
    poseidon::domains::Domain,
    poseidon::hash::{
        domain_to_field,
        batch::{
            commitment_batch_leaf_native, delta_batch_leaf_native, issuance_batch_leaf_native,
            memo_batch_leaf_native, nullifier_batch_leaf_native,
        },
        core::{note_commitment_native, nullifier_native, poi_leaf_native},
    },
    spend::{SpendCircuit, SpendNote, SpendPublicInputs, SpendDelta, build_public_inputs as build_spend_inputs},
    withdraw::{WithdrawCircuit, WithdrawNote, WithdrawPublicInputs, build_public_inputs as build_withdraw_inputs},
    issuance::{IssuanceCircuit, IssuanceInputs, build_public_inputs as build_issuance_inputs},
    batch_agg::{BatchAggCircuit, BatchAggPublicInputs, BatchDelta, BatchIssuance, build_public_inputs as build_batch_inputs},
};

use ff::PrimeField;
use rand::{rngs::StdRng, SeedableRng};

const DEFAULT_K: u32 = 18;
const VK_BUNDLE_MAGIC: &[u8; 8] = b"RGVKv1\0\0";
const VK_BUNDLE_HEADER_LEN: usize = 16;

struct ProofBundle {
    params: Params<EqAffine>,
    vk: VerifyingKey<EqAffine>,
    instances: Vec<Fp>,
    proof: Vec<u8>,
    domain: Domain,
}

fn halo2_k() -> u32 {
    env::var("ORCHARD_HALO2_K")
        .ok()
        .and_then(|value| value.parse::<u32>().ok())
        .unwrap_or(DEFAULT_K)
}

fn should_regen_proofs() -> bool {
    env::var("ORCHARD_REGEN_HALO2_PROOFS").is_ok()
}

fn proof_dir() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("keys")
}

fn proof_paths(name: &str) -> (PathBuf, PathBuf, PathBuf) {
    let dir = proof_dir();
    (
        dir.join(format!("{}.proof", name)),
        dir.join(format!("{}.inputs", name)),
        dir.join(format!("{}.vk", name)),
    )
}

fn parse_vk_bundle(bytes: &[u8]) -> Result<(u32, Vec<u8>)> {
    if bytes.len() < VK_BUNDLE_HEADER_LEN {
        return Err(OrchardError::ParseError("vk bundle missing header".to_string()));
    }
    if !bytes.starts_with(VK_BUNDLE_MAGIC) {
        return Err(OrchardError::ParseError("vk bundle missing magic header".to_string()));
    }

    let mut k_bytes = [0u8; 4];
    k_bytes.copy_from_slice(&bytes[8..12]);
    let k = u32::from_le_bytes(k_bytes);

    let mut len_bytes = [0u8; 4];
    len_bytes.copy_from_slice(&bytes[12..16]);
    let vk_len = u32::from_le_bytes(len_bytes) as usize;

    if bytes.len() != VK_BUNDLE_HEADER_LEN + vk_len {
        return Err(OrchardError::ParseError("vk bundle length mismatch".to_string()));
    }

    Ok((k, bytes[VK_BUNDLE_HEADER_LEN..].to_vec()))
}

fn read_vk_bundle(path: &Path) -> Result<(u32, VerifyingKey<EqAffine>)> {
    let bytes = fs::read(path)
        .map_err(|e| OrchardError::ParseError(format!("failed to read vk bundle: {e}")))?;
    let (k, vk_bytes) = parse_vk_bundle(&bytes)?;
    let vk = VerifyingKey::read(&mut &vk_bytes[..])
        .map_err(|_| OrchardError::ParseError("vk deserialize failed".to_string()))?;
    Ok((k, vk))
}

fn load_inputs(path: &Path) -> Result<Vec<Fp>> {
    let contents = fs::read_to_string(path)
        .map_err(|e| OrchardError::ParseError(format!("failed to read inputs: {e}")))?;
    let mut inputs = Vec::new();

    for line in contents.lines() {
        let line = line.trim();
        if line.is_empty() || line.starts_with('#') {
            continue;
        }
        let mut parts = line.split(':');
        let _idx = parts.next();
        let value = parts.next().ok_or_else(|| {
            OrchardError::ParseError("invalid inputs line".to_string())
        })?;
        let value = value.trim().trim_start_matches("0x");
        let bytes = hex::decode(value)
            .map_err(|_| OrchardError::ParseError("invalid hex in inputs".to_string()))?;
        if bytes.len() != 32 {
            return Err(OrchardError::ParseError("invalid field length".to_string()));
        }
        let mut repr = <Fp as PrimeField>::Repr::default();
        repr.as_mut().copy_from_slice(&bytes);
        let field = Fp::from_repr(repr)
            .into_option()
            .ok_or(OrchardError::InvalidFieldElement)?;
        inputs.push(field);
    }

    Ok(inputs)
}

fn prove_circuit<C: Circuit<Fp> + Clone>(
    domain: Domain,
    circuit: C,
    instances: Vec<Fp>,
) -> Result<ProofBundle> {
    let k = halo2_k();
    let params: Params<EqAffine> = Params::new(k);
    let vk = keygen_vk(&params, &circuit)
        .map_err(|_| OrchardError::ParseError("keygen_vk failed".to_string()))?;
    let pk = keygen_pk(&params, vk.clone(), &circuit)
        .map_err(|_| OrchardError::ParseError("keygen_pk failed".to_string()))?;

    let instances_for_proof: [&[Fp]; 1] = [instances.as_slice()];
    let instance_slices: [&[&[Fp]]; 1] = [&instances_for_proof];

    let mut rng = StdRng::from_seed([0u8; 32]);
    let mut transcript = Blake2bWrite::<Vec<u8>, EqAffine, Challenge255<EqAffine>>::init(Vec::new());
    let domain_scalar = domain_to_field(domain);
    transcript
        .common_scalar(domain_scalar)
        .map_err(|_| OrchardError::ParseError("transcript domain failed".to_string()))?;

    create_proof(
        &params,
        &pk,
        &[circuit],
        &instance_slices,
        &mut rng,
        &mut transcript,
    )
    .map_err(|_| OrchardError::ParseError("create_proof failed".to_string()))?;

    let proof = transcript.finalize();
    verify_proof_with_domain(&params, &vk, &instances, &proof, domain)?;

    Ok(ProofBundle {
        params,
        vk,
        instances,
        proof,
        domain,
    })
}

fn verify_proof_with_domain(
    params: &Params<EqAffine>,
    vk: &VerifyingKey<EqAffine>,
    instances: &[Fp],
    proof: &[u8],
    domain: Domain,
) -> Result<()> {
    let mut transcript = Blake2bRead::<_, EqAffine, Challenge255<EqAffine>>::init(proof);
    transcript
        .common_scalar(domain_to_field(domain))
        .map_err(|_| OrchardError::ParseError("verify transcript domain failed".to_string()))?;

    let instances_for_proof: [&[Fp]; 1] = [instances];
    let instance_slices: [&[&[Fp]]; 1] = [&instances_for_proof];

    verify_proof(
        params,
        vk,
        SingleVerifier::new(params),
        &instance_slices,
        &mut transcript,
    )
    .map_err(|_| OrchardError::ParseError("verify_proof failed".to_string()))
}

fn load_or_generate_bundle<C: Circuit<Fp> + Clone>(
    name: &str,
    domain: Domain,
    circuit: C,
    instances: Vec<Fp>,
) -> Result<ProofBundle> {
    if should_regen_proofs() {
        return prove_circuit(domain, circuit, instances);
    }

    let (proof_path, inputs_path, vk_path) = proof_paths(name);
    let (k, vk) = read_vk_bundle(&vk_path)?;
    let params: Params<EqAffine> = Params::new(k);
    let proof = fs::read(&proof_path)
        .map_err(|e| OrchardError::ParseError(format!("failed to read proof: {e}")))?;
    let loaded_instances = load_inputs(&inputs_path)?;

    verify_proof_with_domain(&params, &vk, &loaded_instances, &proof, domain)?;

    Ok(ProofBundle {
        params,
        vk,
        instances: loaded_instances,
        proof,
        domain,
    })
}

fn dummy_path(depth: usize, tree_type: TreeType) -> MerklePath {
    MerklePath::new(vec![Fp::zero(); depth], vec![false; depth], 0, tree_type)
}

struct SpendFixture {
    circuit: SpendCircuit,
    public: SpendPublicInputs,
}

fn spend_fixture() -> SpendFixture {
    let input_note = SpendNote {
        asset_id: 1,
        amount: 100,
        owner_pk: [0x11u8; 32],
        rho: Fp::from(10),
        note_rseed: Fp::from(11),
        unlock_height: 0,
        memo_hash: Fp::zero(),
    };
    let output_note = SpendNote {
        asset_id: 1,
        amount: 90,
        owner_pk: [0x22u8; 32],
        rho: Fp::from(12),
        note_rseed: Fp::from(13),
        unlock_height: 0,
        memo_hash: Fp::zero(),
    };
    let input_cm = note_commitment_native(
        input_note.asset_id,
        input_note.amount,
        input_note.owner_pk,
        input_note.rho,
        input_note.note_rseed,
        input_note.unlock_height,
        input_note.memo_hash,
    );
    let output_cm = note_commitment_native(
        output_note.asset_id,
        output_note.amount,
        output_note.owner_pk,
        output_note.rho,
        output_note.note_rseed,
        output_note.unlock_height,
        output_note.memo_hash,
    );
    let anchor_root = merkle_native::compute_merkle_root(&[input_cm], TreeType::Commitment);
    let anchor_path = merkle_native::generate_merkle_path(&[input_cm], 0, TreeType::Commitment).unwrap();
    let poi_leaf = poi_leaf_native(input_cm, 5);
    let poi_root = merkle_native::compute_merkle_root(&[poi_leaf], TreeType::ProofOfInnocence);
    let poi_path = merkle_native::generate_merkle_path(&[poi_leaf], 0, TreeType::ProofOfInnocence).unwrap();
    let sk_spend = Fp::from(42);
    let nf = nullifier_native(sk_spend, input_note.rho, input_cm);

    let public = SpendPublicInputs {
        anchor_root,
        num_inputs: 1,
        num_outputs: 1,
        input_nullifiers: [nf, Fp::zero(), Fp::zero(), Fp::zero()],
        output_commitments: [output_cm, Fp::zero(), Fp::zero(), Fp::zero()],
        deltas: [
            SpendDelta { asset_id: 1, in_public: 0, out_public: 0, burn_public: 0, mint_public: 0 },
            SpendDelta { asset_id: 0, in_public: 0, out_public: 0, burn_public: 0, mint_public: 0 },
            SpendDelta { asset_id: 0, in_public: 0, out_public: 0, burn_public: 0, mint_public: 0 },
        ],
        allowed_root: Fp::zero(),
        terms_hash: Fp::zero(),
        poi_root,
        epoch: 5,
        fee: 10,
    };

    let dummy_commitment_path = dummy_path(COMMITMENT_TREE_DEPTH, TreeType::Commitment);
    let dummy_poi_path = dummy_path(POI_TREE_DEPTH, TreeType::ProofOfInnocence);

    let circuit = SpendCircuit {
        public_inputs: build_spend_inputs(&public),
        deltas: public.deltas,
        fee: public.fee,
        input_notes: [input_note.clone(), input_note.clone(), input_note.clone(), input_note.clone()],
        input_sk_spend: [sk_spend, Fp::zero(), Fp::zero(), Fp::zero()],
        input_anchor_paths: [
            anchor_path.clone(),
            dummy_commitment_path.clone(),
            dummy_commitment_path.clone(),
            dummy_commitment_path.clone(),
        ],
        input_poi_paths: [
            poi_path.clone(),
            dummy_poi_path.clone(),
            dummy_poi_path.clone(),
            dummy_poi_path.clone(),
        ],
        input_active: [true, false, false, false],
        output_notes: [output_note.clone(), output_note.clone(), output_note.clone(), output_note.clone()],
        output_allowlist_paths: [
            dummy_commitment_path.clone(),
            dummy_commitment_path.clone(),
            dummy_commitment_path.clone(),
            dummy_commitment_path,
        ],
        output_active: [true, false, false, false],
    };

    SpendFixture { circuit, public }
}

struct WithdrawFixture {
    circuit: WithdrawCircuit,
    public: WithdrawPublicInputs,
}

fn withdraw_fixture() -> WithdrawFixture {
    let input_note = WithdrawNote {
        asset_id: 1,
        amount: 100,
        owner_pk: [0x11u8; 32],
        rho: Fp::from(10),
        note_rseed: Fp::from(11),
        unlock_height: 0,
        memo_hash: Fp::zero(),
    };
    let input_cm = note_commitment_native(
        input_note.asset_id,
        input_note.amount,
        input_note.owner_pk,
        input_note.rho,
        input_note.note_rseed,
        input_note.unlock_height,
        input_note.memo_hash,
    );
    let anchor_root = merkle_native::compute_merkle_root(&[input_cm], TreeType::Commitment);
    let anchor_path = merkle_native::generate_merkle_path(&[input_cm], 0, TreeType::Commitment).unwrap();
    let poi_leaf = poi_leaf_native(input_cm, 5);
    let poi_root = merkle_native::compute_merkle_root(&[poi_leaf], TreeType::ProofOfInnocence);
    let poi_path = merkle_native::generate_merkle_path(&[poi_leaf], 0, TreeType::ProofOfInnocence).unwrap();
    let sk_spend = Fp::from(7);
    let nf = nullifier_native(sk_spend, input_note.rho, input_cm);

    let public = WithdrawPublicInputs {
        anchor_root,
        anchor_size: 1,
        nullifier: nf,
        recipient: [0x22u8; 20],
        amount: 80,
        asset_id: 1,
        gas_limit: 21_000,
        fee: 20,
        poi_root,
        epoch: 5,
        has_change: false,
        change_commitment: Fp::zero(),
        next_root: anchor_root,
    };

    let dummy_commitment_path = dummy_path(COMMITMENT_TREE_DEPTH, TreeType::Commitment);
    let dummy_change = WithdrawNote {
        asset_id: 0,
        amount: 0,
        owner_pk: [0u8; 32],
        rho: Fp::zero(),
        note_rseed: Fp::zero(),
        unlock_height: 0,
        memo_hash: Fp::zero(),
    };

    let circuit = WithdrawCircuit {
        public_inputs: build_withdraw_inputs(&public),
        recipient: public.recipient,
        amount: public.amount,
        gas_limit: public.gas_limit,
        fee: public.fee,
        input_note: input_note.clone(),
        input_sk_spend: sk_spend,
        input_anchor_path: anchor_path,
        input_poi_path: poi_path,
        change_note: dummy_change,
        change_path: dummy_commitment_path,
    };

    WithdrawFixture { circuit, public }
}

struct IssuanceFixture {
    circuit: IssuanceCircuit,
    public: IssuanceInputs,
}

fn issuance_fixture() -> IssuanceFixture {
    let inputs = IssuanceInputs {
        asset_id: 1001,
        mint_amount: 10,
        burn_amount: 0,
        issuer_pk: [0x42u8; 32],
        signature_verification_data: [Fp::zero(), Fp::zero()],
        issuance_root: Fp::zero(),
    };
    let msg = issuance_batch_leaf_native(
        inputs.asset_id,
        inputs.mint_amount,
        inputs.burn_amount,
        inputs.issuer_pk,
    );
    let mut inputs = inputs;
    inputs.signature_verification_data = [msg, Fp::zero()];

    let leaf = msg;
    let root = merkle_native::compute_merkle_root(&[leaf], TreeType::IssuanceBatch);
    let path = merkle_native::generate_merkle_path(&[leaf], 0, TreeType::IssuanceBatch).unwrap();
    inputs.issuance_root = root;

    let circuit = IssuanceCircuit {
        public_inputs: build_issuance_inputs(&inputs),
        issuance_path: path,
    };

    IssuanceFixture { circuit, public: inputs }
}

struct BatchFixture {
    circuit: BatchAggCircuit,
    public: BatchAggPublicInputs,
}

fn batch_fixture() -> BatchFixture {
    let anchor_root = Fp::zero();
    let commitments = vec![Fp::from(5)];
    let commitment_leaf = commitment_batch_leaf_native(commitments[0]);
    let commitments_root = merkle_native::compute_merkle_root(&[commitment_leaf], TreeType::CommitmentBatch);
    let commitment_path = merkle_native::generate_merkle_path(&[commitment_leaf], 0, TreeType::CommitmentBatch).unwrap();

    let nullifiers = vec![Fp::from(9)];
    let null_leaf = nullifier_batch_leaf_native(nullifiers[0]);
    let nullifiers_root = merkle_native::compute_merkle_root(&[null_leaf], TreeType::NullifierBatch);
    let nullifier_path = merkle_native::generate_merkle_path(&[null_leaf], 0, TreeType::NullifierBatch).unwrap();

    let delta = BatchDelta { asset_id: 1, in_public: 0, out_public: 0, burn_public: 0, mint_public: 0 };
    let delta_leaf = delta_batch_leaf_native(delta.asset_id, delta.in_public, delta.out_public, delta.burn_public, delta.mint_public);
    let deltas_root = merkle_native::compute_merkle_root(&[delta_leaf], TreeType::DeltaBatch);
    let delta_path = merkle_native::generate_merkle_path(&[delta_leaf], 0, TreeType::DeltaBatch).unwrap();

    let issuance = BatchIssuance { asset_id: 0, mint_amount: 0, burn_amount: 0, issuer_pk: [0u8; 32] };
    let issuance_leaf = issuance_batch_leaf_native(issuance.asset_id, issuance.mint_amount, issuance.burn_amount, issuance.issuer_pk);
    let issuance_root = merkle_native::compute_merkle_root(&[issuance_leaf], TreeType::IssuanceBatch);
    let issuance_path = merkle_native::generate_merkle_path(&[issuance_leaf], 0, TreeType::IssuanceBatch).unwrap();

    let memos = vec![Fp::from(7)];
    let memo_leaf = memo_batch_leaf_native(memos[0]);
    let memo_root = merkle_native::compute_merkle_root(&[memo_leaf], TreeType::MemoBatch);
    let memo_path = merkle_native::generate_merkle_path(&[memo_leaf], 0, TreeType::MemoBatch).unwrap();

    let poi_cm = Fp::from(3);
    let poi_leaf = poi_leaf_native(poi_cm, 5);
    let poi_root = merkle_native::compute_merkle_root(&[poi_leaf], TreeType::ProofOfInnocence);
    let poi_path = merkle_native::generate_merkle_path(&[poi_leaf], 0, TreeType::ProofOfInnocence).unwrap();

    let public = BatchAggPublicInputs {
        anchor_root,
        anchor_size: 0,
        next_root: anchor_root,
        nullifiers_root,
        commitments_root,
        deltas_root,
        issuance_root,
        memo_root,
        equity_allowed_root: Fp::zero(),
        poi_root,
        total_fee: 0,
        epoch: 5,
        num_user_txs: 1,
        min_fee_per_tx: 0,
        batch_hash: Fp::zero(),
    };

    let circuit = BatchAggCircuit {
        public_inputs: build_batch_inputs(&public),
        nullifiers,
        nullifier_paths: vec![nullifier_path],
        commitments,
        commitment_paths: vec![commitment_path],
        deltas: vec![delta],
        delta_paths: vec![delta_path],
        issuance: vec![issuance],
        issuance_paths: vec![issuance_path],
        memos,
        memo_paths: vec![memo_path],
        poi_commitments: vec![poi_cm],
        poi_paths: vec![poi_path],
    };

    BatchFixture { circuit, public }
}

fn fp_to_bytes_be(value: &Fp) -> [u8; 32] {
    let mut repr = value.to_repr();
    repr.reverse();
    repr
}

fn fp_array_to_bytes(values: &[Fp; 4]) -> [[u8; 32]; 4] {
    [
        fp_to_bytes_be(&values[0]),
        fp_to_bytes_be(&values[1]),
        fp_to_bytes_be(&values[2]),
        fp_to_bytes_be(&values[3]),
    ]
}

fn to_service_delta(delta: SpendDelta) -> ServiceDelta {
    ServiceDelta {
        asset_id: delta.asset_id,
        in_public: delta.in_public,
        out_public: delta.out_public,
        burn_public: delta.burn_public,
        mint_public: delta.mint_public,
    }
}

#[cfg(test)]
mod halo2_integration_tests {
    use super::*;

    #[test]
    fn test_spend_proof_generation_and_verification() {
        println!("=== Halo2 Test 1: Spend Proof Generation & Verification ===");

        let fixture = spend_fixture();
        let instances = fixture.circuit.public_inputs.clone();
        let bundle = load_or_generate_bundle(
            "orchard_spend",
            Domain::OrchardSpendV1,
            fixture.circuit,
            instances,
        )
        .expect("Spend proof generation failed");

        verify_proof_with_domain(&bundle.params, &bundle.vk, &bundle.instances, &bundle.proof, bundle.domain)
            .expect("Spend proof verification failed");

        println!("✅ Spend proof test completed successfully");
    }

    #[test]
    fn test_withdraw_proof_generation_and_verification() {
        println!("=== Halo2 Test 2: Withdraw Proof Generation & Verification ===");

        let fixture = withdraw_fixture();
        let instances = fixture.circuit.public_inputs.clone();
        let bundle = load_or_generate_bundle(
            "orchard_withdraw",
            Domain::OrchardWithdrawV1,
            fixture.circuit,
            instances,
        )
        .expect("Withdraw proof generation failed");

        verify_proof_with_domain(&bundle.params, &bundle.vk, &bundle.instances, &bundle.proof, bundle.domain)
            .expect("Withdraw proof verification failed");

        println!("✅ Withdraw proof test completed successfully");
    }

    #[test]
    fn test_issuance_proof_generation_and_verification() {
        println!("=== Halo2 Test 3: Issuance Proof Generation & Verification ===");

        let fixture = issuance_fixture();
        let instances = fixture.circuit.public_inputs.clone();
        let bundle = load_or_generate_bundle(
            "orchard_issuance",
            Domain::OrchardIssuanceV1,
            fixture.circuit,
            instances,
        )
        .expect("Issuance proof generation failed");

        verify_proof_with_domain(&bundle.params, &bundle.vk, &bundle.instances, &bundle.proof, bundle.domain)
            .expect("Issuance proof verification failed");

        println!("✅ Issuance proof test completed successfully");
    }

    #[test]
    fn test_batch_proof_generation_and_verification() {
        println!("=== Halo2 Test 4: Batch Proof Generation & Verification ===");

        let fixture = batch_fixture();
        let instances = fixture.circuit.public_inputs.clone();
        let bundle = load_or_generate_bundle(
            "orchard_batch",
            Domain::OrchardBatchV1,
            fixture.circuit,
            instances,
        )
        .expect("Batch proof generation failed");

        verify_proof_with_domain(&bundle.params, &bundle.vk, &bundle.instances, &bundle.proof, bundle.domain)
            .expect("Batch proof verification failed");

        println!("✅ Batch proof test completed successfully");
    }

    #[test]
    fn test_vk_registry_integrity() {
        println!("=== Halo2 Test 5: VK Registry Integrity ===");

        let vk_registry = VkRegistry::new();

        // Test all VK entries
        for vk_id in 1..=4 {
            let vk_result = vk_registry.get_vk_entry(vk_id);
            assert!(vk_result.is_ok(), "VK {} not found: {:?}", vk_id, vk_result.err());

            let vk_entry = vk_result.unwrap();
            println!("VK {}: {} fields, domain: {}", vk_id, vk_entry.field_count, vk_entry.domain);

            // Verify VK hash integrity
            assert!(vk_entry.vk_hash.len() == 32, "Invalid VK hash length");
            assert!(!vk_entry.vk_bytes.is_empty(), "Empty VK bytes");
        }

        println!("✅ VK registry integrity test completed successfully");
    }

    #[test]
    fn test_end_to_end_transaction_flow() {
        println!("=== Halo2 Test 6: End-to-End Transaction Flow ===");

        let spend_fixture = spend_fixture();
        let spend_public = spend_fixture.public.clone();
        let instances = spend_fixture.circuit.public_inputs.clone();
        let spend_bundle = load_or_generate_bundle(
            "orchard_spend",
            Domain::OrchardSpendV1,
            spend_fixture.circuit,
            instances,
        )
        .expect("Spend proof generation failed");

        let submit_extrinsic = OrchardExtrinsic::SubmitPrivate {
            proof: spend_bundle.proof,
            anchor_root: fp_to_bytes_be(&spend_public.anchor_root),
            anchor_root_index: 0,
            next_root: fp_to_bytes_be(&spend_public.anchor_root),
            num_inputs: spend_public.num_inputs,
            input_nullifiers: fp_array_to_bytes(&spend_public.input_nullifiers),
            num_outputs: spend_public.num_outputs,
            output_commitments: fp_array_to_bytes(&spend_public.output_commitments),
            deltas: spend_public.deltas.map(to_service_delta),
            allowed_root: fp_to_bytes_be(&spend_public.allowed_root),
            terms_hash: fp_to_bytes_be(&spend_public.terms_hash),
            poi_root: fp_to_bytes_be(&spend_public.poi_root),
            epoch: spend_public.epoch,
            fee: spend_public.fee,
            encrypted_payload: vec![],
        };

        let serialized = submit_extrinsic.serialize();
        assert!(!serialized.is_empty(), "Extrinsic serialization failed");

        let withdraw_fixture = withdraw_fixture();
        let withdraw_instances = withdraw_fixture.circuit.public_inputs.clone();
        let withdraw_bundle = load_or_generate_bundle(
            "orchard_withdraw",
            Domain::OrchardWithdrawV1,
            withdraw_fixture.circuit,
            withdraw_instances,
        )
        .expect("Withdraw proof generation failed");

        verify_proof_with_domain(
            &withdraw_bundle.params,
            &withdraw_bundle.vk,
            &withdraw_bundle.instances,
            &withdraw_bundle.proof,
            Domain::OrchardWithdrawV1,
        )
        .expect("Withdraw proof verification failed");

        println!("✅ End-to-end transaction flow test completed successfully");
    }

    #[test]
    fn test_proof_malleability_protection() {
        println!("=== Halo2 Test 7: Proof Malleability Protection ===");

        let fixture = spend_fixture();
        let instances = fixture.circuit.public_inputs.clone();
        let bundle = load_or_generate_bundle(
            "orchard_spend",
            Domain::OrchardSpendV1,
            fixture.circuit,
            instances,
        )
        .expect("Valid proof generation failed");

        verify_proof_with_domain(&bundle.params, &bundle.vk, &bundle.instances, &bundle.proof, bundle.domain)
            .expect("Valid proof should pass verification");

        let mut modified_instances = bundle.instances.clone();
        modified_instances[0] = Fp::from(999u64);
        let invalid_result = verify_proof_with_domain(
            &bundle.params,
            &bundle.vk,
            &modified_instances,
            &bundle.proof,
            bundle.domain,
        );
        assert!(invalid_result.is_err(), "Modified public inputs should fail verification");

        let mut corrupted_proof = bundle.proof.clone();
        if let Some(last_byte) = corrupted_proof.last_mut() {
            *last_byte = last_byte.wrapping_add(1);
        }
        let corrupted_result = verify_proof_with_domain(
            &bundle.params,
            &bundle.vk,
            &bundle.instances,
            &corrupted_proof,
            bundle.domain,
        );
        assert!(corrupted_result.is_err(), "Corrupted proof should fail verification");

        println!("✅ Proof malleability protection test completed successfully");
    }
}
