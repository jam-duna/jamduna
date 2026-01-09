/// Orchard refine implementation - CompactBlock verification with witnesses
///
/// Verifies CompactBlock transactions using witnesses and payload roots:
/// 1. Collects 2-3 extrinsics (PreStateWitness, optional PostStateWitness, BundleProof)
/// 2. Validates witness bundle structure and nullifier absence proofs
/// 3. Verifies nullifiers are absent in pre-state (no double-spends)
/// 4. Verifies commitments are added to tree correctly
/// 5. Verifies bundle ZK proof with provided public inputs
/// 6. Outputs CompactBlock as write intent after verification

use alloc::{format, vec::Vec};
use alloc::collections::BTreeMap as HashMap;

use crate::compactblock::CompactBlock;
use crate::crypto::{
    merkle_append_with_frontier,
    merkle_root_from_frontier,
    merkle_root_from_leaves,
    verify_commitment_branch,
    TREE_DEPTH,
};
use crate::errors::{OrchardError, Result};
use crate::bundle_codec::{
    decode_issue_bundle,
    decode_swap_bundle,
    decode_zsa_bundle,
    DecodedActionV6,
    DecodedSwapBundle,
};
use crate::objects::ObjectKind;
use crate::nu7_types::OrchardBundleType;
use crate::state::{
    hash_sparse_node,
    sparse_empty_leaf,
    sparse_hash_leaf,
    sparse_position_for,
    MerkleProof,
    OrchardExtrinsic,
    OrchardServiceState,
};
use crate::vk_registry::get_vk_registry;
use crate::signature_verifier::{
    compute_issue_bundle_commitments,
    compute_swap_bundle_commitment,
    compute_zsa_bundle_commitment,
    verify_issue_bundle,
    verify_swap_bundle,
    verify_zsa_bundle_signatures,
};
use crate::witness::WitnessBundle;
use halo2_proofs::arithmetic::CurveAffine;
use halo2_proofs::pasta::group::ff::PrimeField;
use halo2_proofs::pasta::group::{Curve, GroupEncoding};
use halo2_proofs::pasta::pallas;
use utils::effects::WriteIntent;
use utils::functions::{log_info, RefineContext};

// Updated to include transparent state: +32 (merkle) +32 (utxo_root) +8 (utxo_size) = +72 bytes
const PRE_STATE_PAYLOAD_BASE_LEN: usize = 32 + 8 + 4 + 32 + 8 + 32 + 32 + 8 + 4;

// Fee and gas policy constants (Phase 4 security fix)
const MIN_FEE_PER_ACTION: u64 = 1000; // Minimum fee per Orchard action (in smallest units)
const MAX_GAS_PER_WORK_PACKAGE: u64 = 10_000_000; // Maximum gas per work package
const GAS_PER_ACTION: u64 = 50_000; // Estimated gas cost per Orchard action
const GAS_BASE_COST: u64 = 100_000; // Base gas cost for bundle verification
const GAS_PER_PROOF_BYTE: u64 = 10; // Gas cost per proof byte (for DoS protection)
const GAS_PER_BUNDLE_BYTE: u64 = 1;  // Gas cost per bundle byte (for DoS protection)
const FEE_TRANSFER_GAS: u64 = 10_000; // JAM transfer gas (charged during accumulate)
const NULLIFIER_TREE_DEPTH: usize = TREE_DEPTH;
const SLOT_DURATION_SECONDS: u32 = 6;

#[derive(Debug, Clone)]
pub struct PreState {
    pub commitment_root: [u8; 32],
    pub commitment_size: u64,
    pub commitment_frontier: Vec<[u8; 32]>,
    pub nullifier_root: [u8; 32],
    pub nullifier_size: u64,
    pub spent_commitment_proofs: Vec<SpentCommitmentProof>,

    // Transparent transaction state (added for transparent verification)
    pub transparent_merkle_root: [u8; 32],
    pub transparent_utxo_root: [u8; 32],
    pub transparent_utxo_size: u64,
}

#[derive(Debug, Clone)]
pub struct SpentCommitmentProof {
    pub nullifier: [u8; 32],
    pub commitment: [u8; 32],
    pub tree_position: u64,
    pub branch_siblings: Vec<[u8; 32]>,
}

#[derive(Debug, Clone)]
pub struct PostState {
    pub commitment_root: [u8; 32],
    pub commitment_size: u64,
    pub nullifier_root: [u8; 32],
    pub nullifier_size: u64,

    // Transparent transaction state (added for transparent verification)
    pub transparent_merkle_root: [u8; 32],
    pub transparent_utxo_root: [u8; 32],
    pub transparent_utxo_size: u64,
}

#[derive(Debug, Clone)]
struct VerifiedStateTransition {
    new_commitment_root: [u8; 32],
    new_commitment_size: u64,
    new_nullifier_root: [u8; 32],
    new_nullifier_size: u64,
    new_commitments: Vec<[u8; 32]>,
    new_nullifiers: Vec<[u8; 32]>,
    new_transparent_merkle_root: [u8; 32],
    new_transparent_utxo_root: [u8; 32],
    new_transparent_utxo_size: u64,
    transparent_verified: bool,
    fee_paid: u64,
}

#[derive(Debug, Clone)]
struct BundleProofV6 {
    bundle_type: OrchardBundleType,
    orchard_bundle_bytes: Vec<u8>,
    issue_bundle_bytes: Option<Vec<u8>>,
}

enum BundleProofPayload<'a> {
    V5(&'a (u32, Vec<[u8; 32]>, Vec<u8>, Vec<u8>)),
    V6(&'a BundleProofV6),
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord)]
struct SparseNodeKey {
    depth: usize,
    index: u64,
}

pub fn parse_pre_state_payload(payload: &[u8]) -> Result<PreState> {
    if payload.len() < PRE_STATE_PAYLOAD_BASE_LEN {
        return Err(OrchardError::ParseError(format!(
            "pre-state payload too short: {} bytes, expected at least {}",
            payload.len(), PRE_STATE_PAYLOAD_BASE_LEN
        )));
    }

    let mut cursor = 0usize;
    let commitment_root = read_array::<32>(payload, &mut cursor)?;
    let commitment_size = read_u64(payload, &mut cursor)?;
    let frontier_len = read_u32(payload, &mut cursor)? as usize;
    let mut commitment_frontier = Vec::with_capacity(frontier_len);
    for _ in 0..frontier_len {
        commitment_frontier.push(read_array::<32>(payload, &mut cursor)?);
    }
    let nullifier_root = read_array::<32>(payload, &mut cursor)?;
    let nullifier_size = read_u64(payload, &mut cursor)?;

    // Transparent state (72 bytes: 32 + 32 + 8)
    let transparent_merkle_root = read_array::<32>(payload, &mut cursor)?;
    let transparent_utxo_root = read_array::<32>(payload, &mut cursor)?;
    let transparent_utxo_size = read_u64(payload, &mut cursor)?;

    let spent_count = read_u32(payload, &mut cursor)? as usize;
    let mut spent_commitment_proofs = Vec::with_capacity(spent_count);
    for _ in 0..spent_count {
        let nullifier = read_array::<32>(payload, &mut cursor)?;
        let commitment = read_array::<32>(payload, &mut cursor)?;
        let tree_position = read_u64(payload, &mut cursor)?;
        let siblings_len = read_u32(payload, &mut cursor)? as usize;
        let mut branch_siblings = Vec::with_capacity(siblings_len);
        for _ in 0..siblings_len {
            branch_siblings.push(read_array::<32>(payload, &mut cursor)?);
        }
        spent_commitment_proofs.push(SpentCommitmentProof {
            nullifier,
            commitment,
            tree_position,
            branch_siblings,
        });
    }

    Ok(PreState {
        commitment_root,
        commitment_size,
        commitment_frontier,
        nullifier_root,
        nullifier_size,
        spent_commitment_proofs,
        transparent_merkle_root,
        transparent_utxo_root,
        transparent_utxo_size,
    })
}

/// Parse PostStateWitness extrinsic (tag=2)
/// Format (expanded to include transparent roots):
///   pre_nullifier_root (32) + post_commitment_root (32) + reads (4) + writes (4)
///   + transparent_merkle_root (32) + transparent_utxo_root (32) + transparent_utxo_size (8)
pub fn parse_post_state_witness(witness_bytes: &[u8]) -> Result<PostState> {
    // Legacy length: 32 + 32 + 4 + 4 = 72 bytes
    // Expanded length with transparent roots: 32 + 32 + 4 + 4 + 32 + 32 + 8 = 144 bytes
    const LEGACY_LEN: usize = 32 + 32 + 4 + 4;
    const EXPANDED_LEN: usize = 32 + 32 + 4 + 4 + 32 + 32 + 8;
    if witness_bytes.len() < LEGACY_LEN {
        return Err(OrchardError::ParseError(format!(
            "post-state witness too short: {} bytes, expected at least {}",
            witness_bytes.len(), LEGACY_LEN
        )));
    }
    if witness_bytes.len() != LEGACY_LEN && witness_bytes.len() < EXPANDED_LEN {
        return Err(OrchardError::ParseError(format!(
            "post-state witness invalid length: {} bytes",
            witness_bytes.len()
        )));
    }

    let mut cursor = 0usize;

    // Skip pre-nullifier root (unused, zeros)
    let _pre_nullifier_root = read_array::<32>(witness_bytes, &mut cursor)?;

    // Post-commitment root
    let commitment_root = read_array::<32>(witness_bytes, &mut cursor)?;

    // State read/write counts (unused for now)
    let _reads = read_u32(witness_bytes, &mut cursor)?;
    let _writes = read_u32(witness_bytes, &mut cursor)?;

    // Transparent state (NEW - 72 bytes total)
    let mut transparent_merkle_root = [0u8; 32];
    let mut transparent_utxo_root = [0u8; 32];
    let mut transparent_utxo_size = 0u64;
    if witness_bytes.len() >= EXPANDED_LEN {
        transparent_merkle_root = read_array::<32>(witness_bytes, &mut cursor)?;
        transparent_utxo_root = read_array::<32>(witness_bytes, &mut cursor)?;
        transparent_utxo_size = read_u64(witness_bytes, &mut cursor)?;
    }

    Ok(PostState {
        commitment_root,
        commitment_size: 0,  // Not included in PostStateWitness, must be computed
        nullifier_root: [0u8; 32],  // Not included in PostStateWitness
        nullifier_size: 0,  // Not included in PostStateWitness
        transparent_merkle_root,
        transparent_utxo_root,
        transparent_utxo_size,
    })
}

/// Output from witness-aware refiner execution
#[derive(Debug, Clone)]
#[allow(dead_code)]
pub enum RefineOutput {
    Builder {
        intents: Vec<WriteIntent>,
    },
    Guarantor {
        intents: Vec<WriteIntent>,
    },
}

/// Process Orchard extrinsics with witness verification
///
/// Expects 2-3 extrinsics in the work package:
/// 1. PreStateWitness (required)
/// 2. PostStateWitness (optional - not used, legacy compatibility)
/// 3. BundleProof (required)
pub fn process_extrinsics_witness_aware(
    service_id: u32,
    refine_context: &RefineContext,
    work_package_hash: &[u8; 32],
    refine_gas_limit: u64,
    pre_state: &PreState,
    extrinsics: &[OrchardExtrinsic],
    _witness: Option<&()>,
) -> Result<RefineOutput> {
    // Step 1: Collect extrinsics
    let mut pre_witness: Option<WitnessBundle> = None;
    let post_witness: Option<WitnessBundle> = None;
    let mut post_state: Option<PostState> = None;
    let mut bundle_proof: Option<(u32, Vec<[u8; 32]>, Vec<u8>, Vec<u8>)> = None;
    let mut bundle_proof_v6: Option<BundleProofV6> = None;
    let mut transparent_tx_data: Option<Vec<u8>> = None;

    for extrinsic in extrinsics {
        match extrinsic {
            OrchardExtrinsic::BundleSubmit { .. } => {
                return Err(OrchardError::ParseError(
                    "BundleSubmit extrinsic is not supported in refine".into(),
                ));
            }
            OrchardExtrinsic::PreStateWitness { witness_bytes } => {
                if pre_witness.is_some() {
                    return Err(OrchardError::ParseError("Duplicate PreStateWitness".into()));
                }
                pre_witness = Some(WitnessBundle::deserialize(witness_bytes)?);
            }
            OrchardExtrinsic::PostStateWitness { witness_bytes } => {
                if post_state.is_some() {
                    return Err(OrchardError::ParseError("Duplicate PostStateWitness".into()));
                }
                // Parse expanded format with transparent roots (optional)
                post_state = Some(parse_post_state_witness(witness_bytes)?);
                log_info("  PostStateWitness parsed");
            }
            OrchardExtrinsic::BundleProof { vk_id, public_inputs, proof_bytes, bundle_bytes } => {
                if bundle_proof.is_some() {
                    return Err(OrchardError::ParseError("Duplicate BundleProof".into()));
                }
                bundle_proof = Some((*vk_id, public_inputs.clone(), proof_bytes.clone(), bundle_bytes.clone()));
            }
            OrchardExtrinsic::BundleProofV6 {
                bundle_type,
                orchard_bundle_bytes,
                issue_bundle_bytes,
            } => {
                if bundle_proof_v6.is_some() || bundle_proof.is_some() {
                    return Err(OrchardError::ParseError("Duplicate BundleProof".into()));
                }
                bundle_proof_v6 = Some(BundleProofV6 {
                    bundle_type: *bundle_type,
                    orchard_bundle_bytes: orchard_bundle_bytes.clone(),
                    issue_bundle_bytes: issue_bundle_bytes.clone(),
                });
            }
            OrchardExtrinsic::TransparentTxData { data_bytes } => {
                if transparent_tx_data.is_some() {
                    return Err(OrchardError::ParseError("Duplicate TransparentTxData".into()));
                }
                transparent_tx_data = Some(data_bytes.clone());
            }
        }
    }

    // Step 2: Verify required extrinsics are present
    let pre_witness = pre_witness.ok_or_else(||
        OrchardError::ParseError("Missing PreStateWitness extrinsic".into()))?;
    let pre_state = validate_pre_state_roots_with_witness(
        pre_state,
        &pre_witness,
        refine_context,
    )?;
    let bundle_payload = if let Some(v5) = bundle_proof.as_ref() {
        BundleProofPayload::V5(v5)
    } else if let Some(v6) = bundle_proof_v6.as_ref() {
        BundleProofPayload::V6(v6)
    } else {
        return Err(OrchardError::ParseError(
            "Missing BundleProof extrinsic".into(),
        ));
    };

    // PostStateWitness is optional (used for transparent post-state checks when present)
    if post_state.is_some() {
        log_info("  PostStateWitness provided");
    }

    // Step 3: Parse bundle and derive CompactBlock from bundle bytes
    let mut compact_block = match &bundle_payload {
        BundleProofPayload::V5(v5) => {
            let bundle_bytes = &v5.3;
            let parsed_bundle = crate::bundle_parser::parse_bundle(bundle_bytes)?;
            crate::bundle_parser::bundle_to_compact_block(&parsed_bundle, 0)
        }
        BundleProofPayload::V6(v6) => match v6.bundle_type {
            OrchardBundleType::Vanilla => {
                let parsed_bundle = crate::bundle_parser::parse_bundle(&v6.orchard_bundle_bytes)?;
                crate::bundle_parser::bundle_to_compact_block(&parsed_bundle, 0)
            }
            OrchardBundleType::ZSA => {
                if v6.orchard_bundle_bytes.is_empty() {
                    compact_block_from_actions_v6(&[], 0)
                } else {
                    let bundle = decode_zsa_bundle(&v6.orchard_bundle_bytes)?;
                    compact_block_from_actions_v6(&bundle.actions, 0)
                }
            }
            OrchardBundleType::Swap => {
                let bundle = decode_swap_bundle(&v6.orchard_bundle_bytes)?;
                let all_actions = collect_swap_actions(&bundle);
                compact_block_from_actions_v6(&all_actions, 0)
            }
        },
    };

    if let BundleProofPayload::V6(v6) = &bundle_payload {
        append_issue_bundle_commitments(&mut compact_block, v6.issue_bundle_bytes.as_ref())?;
    }

    log_info(&format!("📦 Processing CompactBlock: height={}, {} txs, {} commitments, {} nullifiers",
        compact_block.height,
        compact_block.transaction_count(),
        compact_block.get_all_commitments().len(),
        compact_block.get_all_nullifiers().len()));

    // Step 3: Verify witnesses against CompactBlock
    let mut verified_transition = verify_compact_block_with_witnesses(
        service_id,
        refine_context,
        work_package_hash,
        refine_gas_limit,
        &pre_state,
        &compact_block,
        &pre_witness,
        post_witness.as_ref(),
        &bundle_payload,
    )?;

    log_info("✅ CompactBlock verification complete");

    if let Some(post_state) = post_state.as_ref() {
        if post_state.commitment_root != verified_transition.new_commitment_root {
            return Err(OrchardError::StateError(
                "post-state commitment root mismatch".into(),
            ));
        }
    }

    let transparent_state_present = pre_state.transparent_utxo_size > 0
        || pre_state.transparent_utxo_root != [0u8; 32]
        || pre_state.transparent_merkle_root != [0u8; 32];

    if transparent_tx_data.is_none() && transparent_state_present {
        return Err(OrchardError::StateError(
            "Missing TransparentTxData extrinsic for non-zero transparent state".into(),
        ));
    }

    // Step 3.5: Verify transparent transactions if present
    if let Some(tx_data_bytes) = transparent_tx_data {
        log_info("🔍 Verifying transparent transactions");
        let tx_data = crate::transparent_verify::parse_transparent_tx_data(&tx_data_bytes)?;
        // lookup_anchor_slot is the only chain-time signal available in refine context.
        let current_height = refine_context.lookup_anchor_slot;
        let current_time = refine_context
            .lookup_anchor_slot
            .saturating_mul(SLOT_DURATION_SECONDS);
        crate::transparent_verify::verify_transparent_txs(
            &tx_data,
            &pre_state.transparent_utxo_root,
            current_height,
            current_time,
        )?;
        let transition = crate::transparent_verify::compute_transparent_transition(
            &tx_data,
            &pre_state.transparent_utxo_root,
            pre_state.transparent_utxo_size,
            current_height,
        )?;
        verified_transition.new_transparent_merkle_root = transition.new_merkle_root;
        verified_transition.new_transparent_utxo_root = transition.new_utxo_root;
        verified_transition.new_transparent_utxo_size = transition.new_utxo_size;
        verified_transition.transparent_verified = true;
        if let Some(post_state) = post_state.as_ref() {
            let has_post_transparent = post_state.transparent_utxo_root != [0u8; 32]
                || post_state.transparent_utxo_size != 0
                || post_state.transparent_merkle_root != [0u8; 32];
            if has_post_transparent {
                if post_state.transparent_merkle_root != transition.new_merkle_root
                    || post_state.transparent_utxo_root != transition.new_utxo_root
                    || post_state.transparent_utxo_size != transition.new_utxo_size
                {
                    return Err(OrchardError::StateError(
                        "transparent post-state mismatch".into(),
                    ));
                }
            }
        }
        log_info(&format!("✅ Verified {} transparent transactions", tx_data.transactions.len()));
    }
    // When no tag=4 extrinsic is present, still emit transparent state transitions so
    // accumulate can bind the builder-supplied pre-state to JAM storage. This prevents
    // a malicious builder from zeroing the transparent roots to bypass verification.
    if !verified_transition.transparent_verified {
        verified_transition.transparent_verified = true;
        verified_transition.new_transparent_merkle_root = pre_state.transparent_merkle_root;
        verified_transition.new_transparent_utxo_root = pre_state.transparent_utxo_root;
        verified_transition.new_transparent_utxo_size = pre_state.transparent_utxo_size;
        log_info("ℹ️ Transparent state unchanged (no tag=4 extrinsic); emitting no-op transitions");
    }

    // Step 4: Generate write intents for CompactBlock and state updates
    let compact_block_bytes = compact_block.to_bytes()?;

    let object_id = compute_storage_opaque_key(service_id, b"compact_block");
    let intent = WriteIntent {
        effect: utils::effects::WriteEffectEntry {
            object_id,
            ref_info: utils::effects::ObjectRef {
                work_package_hash: refine_context.anchor,
                index_start: 0,
                payload_length: compact_block_bytes.len() as u32,
                object_kind: ObjectKind::Block as u8,
            },
            payload: compact_block_bytes,
            tx_index: 0,
        },
    };

    let mut intents = Vec::new();
    intents.push(intent);
    intents.extend(build_state_write_intents(
        service_id,
        refine_context,
        &pre_state,
        &verified_transition,
    )?);

    Ok(RefineOutput::Guarantor { intents })
}

/// Verify CompactBlock using witness data and pre-state payload
fn verify_compact_block_with_witnesses(
    service_id: u32,
    _refine_context: &RefineContext,
    _work_package_hash: &[u8; 32],
    refine_gas_limit: u64,
    pre_state: &PreState,
    compact_block: &CompactBlock,
    pre_witness: &WitnessBundle,
    post_witness: Option<&WitnessBundle>,
    bundle_proof: &BundleProofPayload,
) -> Result<VerifiedStateTransition> {
    log_info("🔍 Verifying CompactBlock with witnesses");

    if pre_state.commitment_frontier.len() != TREE_DEPTH {
        return Err(OrchardError::StateError(format!(
            "commitment_frontier length mismatch: expected {}, got {}",
            TREE_DEPTH,
            pre_state.commitment_frontier.len()
        )));
    }

    let recomputed_pre_root = merkle_root_from_frontier(
        &pre_state.commitment_frontier,
        pre_state.commitment_size,
    )?;
    if recomputed_pre_root != pre_state.commitment_root {
        return Err(OrchardError::StateError(
            "commitment_root mismatch with commitment_frontier".into(),
        ));
    }

    // Extract all nullifiers and commitments from CompactBlock
    let nullifiers = compact_block.get_all_nullifiers();
    let commitments = compact_block.get_all_commitments();

    log_info(&format!("  {} nullifiers, {} commitments", nullifiers.len(), commitments.len()));

    // Step 0: Verify bundle ZK proof and signatures (signatures first)
    let fee_paid = match bundle_proof {
        BundleProofPayload::V5(v5) => {
            let bundle_bytes = &v5.3;
            // ZIP-244 signatures are bound to the bundle commitment only.
            verify_bundle_proof(
                v5,
                bundle_bytes,
                refine_gas_limit,
                pre_state,
                &nullifiers,
                &commitments,
            )?
        }
        BundleProofPayload::V6(v6) => verify_bundle_proof_v6(
            v6,
            refine_gas_limit,
            pre_state,
        )?,
    };

    // Step 1: Verify pre-state witness structure (nullifier absence proofs only)
    crate::witness::validate_witness_bundle(service_id, pre_witness)?;
    crate::witness::detect_witness_attacks(pre_witness)?;

    // Post-witness is optional and not used (legacy compatibility)
    if let Some(post_witness) = post_witness {
        if !post_witness.writes.is_empty() {
            return Err(OrchardError::StateError("Unexpected post-state writes".into()));
        }
        log_info("  ✅ PostStateWitness verified empty (not used)");
    }

    // Step 2: Verify spent commitments are in pre-state
    verify_spent_commitments(&nullifiers, pre_state)?;

    // Step 3: Verify all nullifiers are absent in pre-state (no double-spends)
    let nullifier_proofs = verify_nullifiers_absent(service_id, &nullifiers, pre_state, pre_witness)?;

    // Step 3.1: Compute new nullifier root from validated absence proofs
    log_info("  🔧 Computing new nullifier root");
    let new_nullifier_root = compute_nullifier_root_after_spends(
        pre_state.nullifier_root,
        &nullifiers,
        &nullifier_proofs,
    )?;
    let new_nullifier_size = pre_state
        .nullifier_size
        .checked_add(nullifiers.len() as u64)
        .ok_or_else(|| OrchardError::StateError("nullifier_size overflow".into()))?;
    log_info(&format!(
        "  ✅ New nullifier root computed: {:?}",
        &new_nullifier_root[..8]
    ));

    // Step 4: Compute new commitment tree state
    let new_commitments: Vec<[u8; 32]> = commitments.iter()
        .map(|c| c.commitment)
        .collect();

    log_info(&format!(
        "  🔧 Computing new commitment root ({} new commitments)",
        new_commitments.len()
    ));
    log_info(&format!(
        "  🧮 Commitment append input: pre_root={:?}, size={}, frontier_len={}",
        &pre_state.commitment_root[..8],
        pre_state.commitment_size,
        pre_state.commitment_frontier.len()
    ));
    let new_commitment_root = merkle_append_with_frontier(
        &pre_state.commitment_root,
        pre_state.commitment_size,
        &pre_state.commitment_frontier,
        &new_commitments,
    )?;
    log_info(&format!(
        "  ✅ New commitment root computed: {:?}",
        &new_commitment_root[..8]
    ));
    let new_commitment_size = pre_state.commitment_size + new_commitments.len() as u64;
    log_info(&format!(
        "  ✅ Commitment root update complete (new size={})",
        new_commitment_size
    ));

    log_info(&format!("  New commitment tree: size={}", new_commitment_size));

    log_info("✅ Witness verification complete");
    Ok(VerifiedStateTransition {
        new_commitment_root,
        new_commitment_size,
        new_nullifier_root,
        new_nullifier_size,
        new_commitments,
        new_nullifiers: nullifiers,
        new_transparent_merkle_root: pre_state.transparent_merkle_root,
        new_transparent_utxo_root: pre_state.transparent_utxo_root,
        new_transparent_utxo_size: pre_state.transparent_utxo_size,
        transparent_verified: false,
        fee_paid,
    })
}

fn compact_block_from_actions_v6(actions: &[DecodedActionV6], height: u64) -> CompactBlock {
    use crate::compactblock::{CompactBlock, CompactTx, OrchardCompactOutput};

    let nullifiers: Vec<[u8; 32]> = actions.iter().map(|action| action.nullifier).collect();
    let commitments: Vec<OrchardCompactOutput> = actions
        .iter()
        .map(|action| OrchardCompactOutput {
            commitment: action.cmx,
            ephemeral_key: action.epk_bytes,
            ciphertext: action.enc_ciphertext.to_vec(),
        })
        .collect();

    let tx = CompactTx {
        index: 0,
        txid: [0u8; 32],
        nullifiers,
        commitments,
    };

    CompactBlock {
        proto_version: 1,
        height,
        hash: [0u8; 32],
        prev_hash: [0u8; 32],
        time: 0,
        header: Vec::new(),
        vtx: vec![tx],
        chain_metadata: crate::compactblock::ChainMetadata::default(),
    }
}

fn append_issue_bundle_commitments(
    compact_block: &mut CompactBlock,
    issue_bundle_bytes: Option<&Vec<u8>>,
) -> Result<()> {
    let Some(issue_bundle_bytes) = issue_bundle_bytes else {
        return Ok(());
    };

    let Some(issue_bundle) = decode_issue_bundle(issue_bundle_bytes)? else {
        return Ok(());
    };

    let commitments = compute_issue_bundle_commitments(&issue_bundle)?;
    if commitments.is_empty() {
        return Ok(());
    }

    let tx = compact_block.vtx.get_mut(0).ok_or_else(|| {
        OrchardError::StateError("IssueBundle requires a compact tx".into())
    })?;

    for commitment in commitments {
        tx.commitments.push(crate::compactblock::OrchardCompactOutput {
            commitment,
            ephemeral_key: [0u8; 32],
            ciphertext: Vec::new(),
        });
    }

    Ok(())
}

fn collect_swap_actions(bundle: &DecodedSwapBundle) -> Vec<DecodedActionV6> {
    let mut actions = Vec::new();
    for group in &bundle.action_groups {
        actions.extend_from_slice(&group.actions);
    }
    actions
}

/// Verify pre-state witness proves the claimed commitment_root and nullifier_root
/// Verify spent commitments are members of the pre-state commitment tree.
fn verify_spent_commitments(
    nullifiers: &[[u8; 32]],
    pre_state: &PreState,
) -> Result<()> {
    if pre_state.spent_commitment_proofs.len() != nullifiers.len() {
        return Err(OrchardError::StateError(format!(
            "Spent commitment proofs mismatch: expected {}, got {}",
            nullifiers.len(),
            pre_state.spent_commitment_proofs.len()
        )));
    }

    let mut proofs_by_nullifier: HashMap<[u8; 32], &SpentCommitmentProof> = HashMap::new();
    for proof in &pre_state.spent_commitment_proofs {
        if proofs_by_nullifier.insert(proof.nullifier, proof).is_some() {
            return Err(OrchardError::StateError(
                "Duplicate spent commitment proof for nullifier".into()
            ));
        }
    }

    for nullifier in nullifiers {
        let proof = proofs_by_nullifier.get(nullifier).ok_or_else(|| {
            OrchardError::StateError(format!(
                "Missing spent commitment proof for nullifier {:?}",
                &nullifier[..4]
            ))
        })?;
        verify_commitment_branch(
            &proof.commitment,
            proof.tree_position,
            &proof.branch_siblings,
            &pre_state.commitment_root,
        )?;
    }

    Ok(())
}

fn validate_pre_state_roots_with_witness(
    pre_state: &PreState,
    pre_witness: &WitnessBundle,
    refine_context: &RefineContext,
) -> Result<PreState> {
    let allow_unbound = refine_context.state_root == [0u8; 32];

    if pre_witness.pre_state_root == [0u8; 32] {
        if !allow_unbound {
            return Err(OrchardError::StateError(
                "pre-state witness missing state_root".into(),
            ));
        }
    } else if !allow_unbound && pre_witness.pre_state_root != refine_context.state_root {
        return Err(OrchardError::StateError(
            "pre-state witness root mismatch".into(),
        ));
    }

    let mut orchard_state_bytes: Option<&[u8]> = None;
    let mut orchard_state_proved = false;
    let mut commitment_root_read: Option<[u8; 32]> = None;
    let mut commitment_root_proved = false;
    let mut commitment_size_read: Option<u64> = None;
    let mut commitment_size_proved = false;
    let mut nullifier_root_read: Option<[u8; 32]> = None;
    let mut nullifier_root_proved = false;
    let mut nullifier_size_read: Option<u64> = None;
    let mut nullifier_size_proved = false;
    let mut transparent_merkle_root_read: Option<[u8; 32]> = None;
    let mut transparent_merkle_root_proved = false;
    let mut transparent_utxo_root_read: Option<[u8; 32]> = None;
    let mut transparent_utxo_root_proved = false;
    let mut transparent_utxo_size_read: Option<u64> = None;
    let mut transparent_utxo_size_proved = false;
    let mut commitment_entries: HashMap<u64, [u8; 32]> = HashMap::new();
    let mut max_commitment_index: Option<u64> = None;

    for read in &pre_witness.reads {
        if read.key == "orchard_state" {
            if orchard_state_bytes.is_some() {
                return Err(OrchardError::StateError(
                    "Duplicate orchard_state read in witness".into(),
                ));
            }
            orchard_state_bytes = Some(&read.value);
            orchard_state_proved = !read.merkle_proof.is_empty();
            continue;
        }

        match read.key.as_str() {
            "commitment_root" => {
                commitment_root_read = Some(parse_state_root(&read.value, "commitment_root")?);
                commitment_root_proved = !read.merkle_proof.is_empty();
            }
            "commitment_size" => {
                commitment_size_read = Some(parse_state_u64(&read.value, "commitment_size")?);
                commitment_size_proved = !read.merkle_proof.is_empty();
            }
            "nullifier_root" => {
                nullifier_root_read = Some(parse_state_root(&read.value, "nullifier_root")?);
                nullifier_root_proved = !read.merkle_proof.is_empty();
            }
            "nullifier_size" => {
                nullifier_size_read = Some(parse_state_u64(&read.value, "nullifier_size")?);
                nullifier_size_proved = !read.merkle_proof.is_empty();
            }
            "transparent_merkle_root" => {
                transparent_merkle_root_read = Some(parse_state_root(&read.value, "transparent_merkle_root")?);
                transparent_merkle_root_proved = !read.merkle_proof.is_empty();
            }
            "transparent_utxo_root" => {
                transparent_utxo_root_read = Some(parse_state_root(&read.value, "transparent_utxo_root")?);
                transparent_utxo_root_proved = !read.merkle_proof.is_empty();
            }
            "transparent_utxo_size" => {
                transparent_utxo_size_read = Some(parse_state_u64(&read.value, "transparent_utxo_size")?);
                transparent_utxo_size_proved = !read.merkle_proof.is_empty();
            }
            _ => {
                if let Some(index) = parse_commitment_index(&read.key) {
                    let commitment = parse_state_root(&read.value, "commitment_entry")?;
                    if commitment_entries.insert(index, commitment).is_some() {
                        return Err(OrchardError::StateError(
                            "Duplicate commitment entry in witness".into(),
                        ));
                    }
                    max_commitment_index = Some(max_commitment_index.map_or(index, |prev| prev.max(index)));
                }
            }
        }
    }

    let derived = if let Some(state_bytes) = orchard_state_bytes {
        if !allow_unbound && !orchard_state_proved {
            return Err(OrchardError::StateError(
                "orchard_state witness missing merkle proof".into(),
            ));
        }
        OrchardServiceState::from_bytes(state_bytes)?
    } else {
        if !allow_unbound {
            if commitment_root_read.is_none() || !commitment_root_proved {
                return Err(OrchardError::StateError(
                    "commitment_root witness missing merkle proof".into(),
                ));
            }
            if commitment_size_read.is_none() || !commitment_size_proved {
                return Err(OrchardError::StateError(
                    "commitment_size witness missing merkle proof".into(),
                ));
            }
            if nullifier_root_read.is_none() || !nullifier_root_proved {
                return Err(OrchardError::StateError(
                    "nullifier_root witness missing merkle proof".into(),
                ));
            }
            if nullifier_size_read.is_none() || !nullifier_size_proved {
                return Err(OrchardError::StateError(
                    "nullifier_size witness missing merkle proof".into(),
                ));
            }
            if transparent_merkle_root_read.is_none() || !transparent_merkle_root_proved {
                return Err(OrchardError::StateError(
                    "transparent_merkle_root witness missing merkle proof".into(),
                ));
            }
            if transparent_utxo_root_read.is_none() || !transparent_utxo_root_proved {
                return Err(OrchardError::StateError(
                    "transparent_utxo_root witness missing merkle proof".into(),
                ));
            }
            if transparent_utxo_size_read.is_none() || !transparent_utxo_size_proved {
                return Err(OrchardError::StateError(
                    "transparent_utxo_size witness missing merkle proof".into(),
                ));
            }
        }

        OrchardServiceState {
            commitment_root: commitment_root_read.unwrap_or(pre_state.commitment_root),
            commitment_size: commitment_size_read.unwrap_or(pre_state.commitment_size),
            nullifier_root: nullifier_root_read.unwrap_or(pre_state.nullifier_root),
            nullifier_size: nullifier_size_read.unwrap_or(pre_state.nullifier_size),
            transparent_merkle_root: transparent_merkle_root_read
                .unwrap_or(pre_state.transparent_merkle_root),
            transparent_utxo_root: transparent_utxo_root_read
                .unwrap_or(pre_state.transparent_utxo_root),
            transparent_utxo_size: transparent_utxo_size_read
                .unwrap_or(pre_state.transparent_utxo_size),
        }
    };

    if !commitment_entries.is_empty() {
        let snapshot_size = if let Some(size) = commitment_size_read {
            size
        } else {
            max_commitment_index.ok_or_else(|| {
                OrchardError::StateError("commitment snapshot missing max index".into())
            })?
            .saturating_add(1)
        };
        if snapshot_size == 0 {
            return Err(OrchardError::StateError(
                "commitment snapshot empty but entries provided".into(),
            ));
        }
        if commitment_entries.len() != snapshot_size as usize {
            return Err(OrchardError::StateError(format!(
                "commitment snapshot incomplete: expected {}, got {}",
                snapshot_size,
                commitment_entries.len()
            )));
        }

        let mut leaves = Vec::with_capacity(snapshot_size as usize);
        for index in 0..snapshot_size {
            let commitment = commitment_entries.get(&index).ok_or_else(|| {
                OrchardError::StateError(format!(
                    "commitment snapshot missing index {}",
                    index
                ))
            })?;
            leaves.push(*commitment);
        }

        let computed_root = merkle_root_from_leaves(&leaves)?;
        if computed_root != derived.commitment_root {
            return Err(OrchardError::StateError(
                "commitment snapshot root mismatch".into(),
            ));
        }
        if snapshot_size != derived.commitment_size {
            return Err(OrchardError::StateError(
                "commitment snapshot size mismatch".into(),
            ));
        }
    }

    if derived.commitment_root != pre_state.commitment_root {
        return Err(OrchardError::StateError(
            "pre-state commitment_root mismatch with witness".into(),
        ));
    }
    if derived.commitment_size != pre_state.commitment_size {
        return Err(OrchardError::StateError(
            "pre-state commitment_size mismatch with witness".into(),
        ));
    }
    if derived.nullifier_root != pre_state.nullifier_root {
        return Err(OrchardError::StateError(
            "pre-state nullifier_root mismatch with witness".into(),
        ));
    }
    if derived.nullifier_size != pre_state.nullifier_size {
        return Err(OrchardError::StateError(
            "pre-state nullifier_size mismatch with witness".into(),
        ));
    }
    if derived.transparent_merkle_root != pre_state.transparent_merkle_root {
        return Err(OrchardError::StateError(
            "pre-state transparent_merkle_root mismatch with witness".into(),
        ));
    }
    if derived.transparent_utxo_root != pre_state.transparent_utxo_root {
        return Err(OrchardError::StateError(
            "pre-state transparent_utxo_root mismatch with witness".into(),
        ));
    }
    if derived.transparent_utxo_size != pre_state.transparent_utxo_size {
        return Err(OrchardError::StateError(
            "pre-state transparent_utxo_size mismatch with witness".into(),
        ));
    }

    let mut verified = pre_state.clone();
    verified.commitment_root = derived.commitment_root;
    verified.commitment_size = derived.commitment_size;
    verified.nullifier_root = derived.nullifier_root;
    verified.nullifier_size = derived.nullifier_size;
    verified.transparent_merkle_root = derived.transparent_merkle_root;
    verified.transparent_utxo_root = derived.transparent_utxo_root;
    verified.transparent_utxo_size = derived.transparent_utxo_size;

    Ok(verified)
}

fn validate_direct_state_reads(
    pre_state: &PreState,
    commitment_root_read: Option<[u8; 32]>,
    commitment_size_read: Option<u64>,
    nullifier_root_read: Option<[u8; 32]>,
    nullifier_size_read: Option<u64>,
    transparent_merkle_root_read: Option<[u8; 32]>,
    transparent_utxo_root_read: Option<[u8; 32]>,
    transparent_utxo_size_read: Option<u64>,
) -> Result<()> {
    if let Some(root) = commitment_root_read {
        if root != pre_state.commitment_root {
            return Err(OrchardError::StateError(
                "pre-state commitment_root mismatch with witness read".into(),
            ));
        }
    }
    if let Some(size) = commitment_size_read {
        if size != pre_state.commitment_size {
            return Err(OrchardError::StateError(
                "pre-state commitment_size mismatch with witness read".into(),
            ));
        }
    }
    if let Some(root) = nullifier_root_read {
        if root != pre_state.nullifier_root {
            return Err(OrchardError::StateError(
                "pre-state nullifier_root mismatch with witness read".into(),
            ));
        }
    }
    if let Some(size) = nullifier_size_read {
        if size != pre_state.nullifier_size {
            return Err(OrchardError::StateError(
                "pre-state nullifier_size mismatch with witness read".into(),
            ));
        }
    }
    if let Some(root) = transparent_merkle_root_read {
        if root != pre_state.transparent_merkle_root {
            return Err(OrchardError::StateError(
                "pre-state transparent_merkle_root mismatch with witness read".into(),
            ));
        }
    }
    if let Some(root) = transparent_utxo_root_read {
        if root != pre_state.transparent_utxo_root {
            return Err(OrchardError::StateError(
                "pre-state transparent_utxo_root mismatch with witness read".into(),
            ));
        }
    }
    if let Some(size) = transparent_utxo_size_read {
        if size != pre_state.transparent_utxo_size {
            return Err(OrchardError::StateError(
                "pre-state transparent_utxo_size mismatch with witness read".into(),
            ));
        }
    }
    Ok(())
}

fn validate_commitment_snapshot(
    pre_state: &PreState,
    commitment_size_read: Option<u64>,
    max_commitment_index: Option<u64>,
    commitment_entries: &HashMap<u64, [u8; 32]>,
) -> Result<()> {
    if commitment_entries.is_empty() {
        return Ok(());
    }

    let snapshot_size = if let Some(size) = commitment_size_read {
        size
    } else {
        max_commitment_index
            .ok_or_else(|| OrchardError::StateError("commitment snapshot missing max index".into()))?
            .saturating_add(1)
    };

    if snapshot_size == 0 {
        return Err(OrchardError::StateError(
            "commitment snapshot empty but entries provided".into(),
        ));
    }

    if commitment_entries.len() != snapshot_size as usize {
        return Err(OrchardError::StateError(format!(
            "commitment snapshot incomplete: expected {}, got {}",
            snapshot_size,
            commitment_entries.len()
        )));
    }

    let mut leaves = Vec::with_capacity(snapshot_size as usize);
    for index in 0..snapshot_size {
        let commitment = commitment_entries.get(&index).ok_or_else(|| {
            OrchardError::StateError(format!(
                "commitment snapshot missing index {}",
                index
            ))
        })?;
        leaves.push(*commitment);
    }

    let computed_root = merkle_root_from_leaves(&leaves)?;
    if computed_root != pre_state.commitment_root {
        return Err(OrchardError::StateError(
            "pre-state commitment_root mismatch with commitment snapshot".into(),
        ));
    }
    if snapshot_size != pre_state.commitment_size {
        return Err(OrchardError::StateError(
            "pre-state commitment_size mismatch with commitment snapshot".into(),
        ));
    }

    Ok(())
}

fn parse_commitment_index(key: &str) -> Option<u64> {
    let suffix = key.strip_prefix("commitment_")?;
    if suffix.is_empty() || !suffix.as_bytes().iter().all(|b| b.is_ascii_digit()) {
        return None;
    }
    suffix.parse::<u64>().ok()
}

fn parse_state_root(value: &[u8], label: &str) -> Result<[u8; 32]> {
    if value.len() != 32 {
        return Err(OrchardError::StateError(format!(
            "{} length mismatch: expected 32, got {}",
            label,
            value.len()
        )));
    }
    let mut out = [0u8; 32];
    out.copy_from_slice(value);
    Ok(out)
}

fn parse_state_u64(value: &[u8], label: &str) -> Result<u64> {
    if value.len() != 8 {
        return Err(OrchardError::StateError(format!(
            "{} length mismatch: expected 8, got {}",
            label,
            value.len()
        )));
    }
    let mut buf = [0u8; 8];
    buf.copy_from_slice(value);
    Ok(u64::from_le_bytes(buf))
}

/// Verify all nullifiers are absent in pre-state using sparse Merkle proofs
fn verify_nullifiers_absent(
    _service_id: u32,
    nullifiers: &[[u8; 32]],
    pre_state: &PreState,
    pre_witness: &WitnessBundle,
) -> Result<HashMap<[u8; 32], Vec<[u8; 32]>>> {
    log_info(&format!("  Verifying {} nullifiers absent", nullifiers.len()));

    // Build map of nullifier -> proof from witness
    let mut nullifier_proofs: HashMap<[u8; 32], Vec<[u8; 32]>> = HashMap::new();
    let mut seen_nullifiers: HashMap<[u8; 32], ()> = HashMap::new();

    for read in &pre_witness.reads {
        if read.key.starts_with("nullifier_") {
            // Extract nullifier from key "nullifier_<hex>"
            if let Some(hex_str) = read.key.strip_prefix("nullifier_") {
                if let Ok(nullifier_bytes) = hex::decode(hex_str) {
                    if nullifier_bytes.len() == 32 {
                        let mut nullifier = [0u8; 32];
                        nullifier.copy_from_slice(&nullifier_bytes);
                        if read.value.len() != 32 || read.value.iter().any(|b| *b != 0) {
                            return Err(OrchardError::StateError(
                                "Nullifier absence proof must have empty value".into()
                            ));
                        }
                        if nullifier_proofs.insert(nullifier, read.merkle_proof.clone()).is_some() {
                            return Err(OrchardError::StateError(
                                "Duplicate nullifier proof in witness".into()
                            ));
                        }
                    }
                }
            }
        }
    }

    // Verify each nullifier has an absence proof
    for nullifier in nullifiers {
        if seen_nullifiers.insert(*nullifier, ()).is_some() {
            return Err(OrchardError::NullifierReusedInWorkItem { nullifier: *nullifier });
        }
        let proof_path = nullifier_proofs.get(nullifier)
            .ok_or_else(|| OrchardError::StateError(
                format!("Missing absence proof for nullifier {:?}", &nullifier[..4])
            ))?;

        if proof_path.len() != NULLIFIER_TREE_DEPTH {
            return Err(OrchardError::StateError(format!(
                "Nullifier proof depth mismatch: expected {}, got {}",
                NULLIFIER_TREE_DEPTH,
                proof_path.len()
            )));
        }

        // Verify sparse Merkle proof shows nullifier is absent (leaf = 0)
        let proof = MerkleProof {
            leaf: sparse_empty_leaf(),
            path: proof_path.clone(),
            root: pre_state.nullifier_root,
        };

        let position = sparse_position_for(*nullifier, proof_path.len());
        if !proof.verify_with_position(position) {
            return Err(OrchardError::InvalidProof);
        }

        // Verify leaf is empty (nullifier not spent)
        if proof.leaf != sparse_empty_leaf() {
            return Err(OrchardError::NullifierAlreadySpent { nullifier: *nullifier });
        }
    }

    log_info("  ✅ All nullifiers verified absent");
    Ok(nullifier_proofs)
}

/// Validate fee and gas bounds (Phase 4 security fix)
///
/// Enforces:
/// - Minimum fee per action (prevents economic DOS)
/// - Maximum gas limits (prevents resource exhaustion)
/// - Fee covers estimated gas cost
fn validate_fee_and_gas(
    parsed_bundle: &crate::bundle_parser::ParsedBundle,
    bundle_bytes: &[u8],
    refine_gas_limit: u64,
) -> Result<u64> {
    let action_count = parsed_bundle.action_count as u64;
    if refine_gas_limit == 0 {
        return Err(OrchardError::StateError(
            "Refine gas limit must be non-zero".into()
        ));
    }

    // Calculate minimum required fee based on action count
    let min_fee = MIN_FEE_PER_ACTION * action_count;

    // value_balance is the net value leaving Orchard pool (fees in Zcash)
    // Positive value_balance means value is leaving (paying fees)
    // Negative value_balance means value is entering (receiving)
    let fee = if parsed_bundle.value_balance > 0 {
        parsed_bundle.value_balance as u64
    } else {
        // If value_balance is negative or zero, no fee is being paid
        0u64
    };

    // Enforce minimum fee requirement
    if fee < min_fee {
        return Err(OrchardError::StateError(format!(
            "Fee below minimum: {} < {} (required {} per action × {} actions)",
            fee, min_fee, MIN_FEE_PER_ACTION, action_count
        )));
    }

    // Calculate estimated gas cost including bundle and proof size
    let bundle_size_gas = (bundle_bytes.len() as u64) * GAS_PER_BUNDLE_BYTE;
    let proof_size_gas = (parsed_bundle.proof_bytes.len() as u64) * GAS_PER_PROOF_BYTE;
    let transfer_gas = if fee > 0 { FEE_TRANSFER_GAS } else { 0 };
    let estimated_gas = GAS_BASE_COST
        + (GAS_PER_ACTION * action_count)
        + bundle_size_gas
        + proof_size_gas
        + transfer_gas;

    let max_gas = core::cmp::min(refine_gas_limit, MAX_GAS_PER_WORK_PACKAGE);

    // Enforce maximum gas limit (prevent resource exhaustion)
    if estimated_gas > max_gas {
        return Err(OrchardError::StateError(format!(
            "Estimated gas {} exceeds maximum {} (bundle has {} actions, refine limit {})",
            estimated_gas, max_gas, action_count, refine_gas_limit
        )));
    }

    // Enforce fee covers estimated gas
    if fee < estimated_gas {
        return Err(OrchardError::StateError(format!(
            "Fee insufficient for gas: {} < estimated {}",
            fee, estimated_gas
        )));
    }

    log_info(&format!(
        "  💰 Fee validation: {} units for {} actions (min: {}, estimated gas: {} [base:{} + actions:{} + bundle:{} + proof:{} + transfer:{}], refine limit: {})",
        fee,
        action_count,
        min_fee,
        estimated_gas,
        GAS_BASE_COST,
        GAS_PER_ACTION * action_count,
        bundle_size_gas,
        proof_size_gas,
        transfer_gas,
        refine_gas_limit
    ));

    Ok(fee)
}

/// Verify Orchard bundle proof and signatures
///
/// Uses ZIP-244 bundle commitment for sighash computation (not work package binding).
/// Replay protection comes from nullifier absence proofs + accumulate old-root validation.
fn verify_bundle_proof(
    bundle_proof: &(u32, Vec<[u8; 32]>, Vec<u8>, Vec<u8>),
    bundle_bytes: &[u8],  // Bundle bytes from BundleProof extrinsic for signature verification
    refine_gas_limit: u64,
    pre_state: &PreState,
    nullifiers: &[[u8; 32]],
    commitments: &[crate::compactblock::OrchardCompactOutput],
) -> Result<u64> {
    log_info("  🔐 Verifying bundle (ZK proof + signatures)");

    let (vk_id, public_inputs, proof_bytes, _bundle_bytes_in_proof) = bundle_proof;
    if proof_bytes.is_empty() {
        return Err(OrchardError::StateError(
            "Missing bundle proof bytes".into()
        ));
    }

    if bundle_bytes.is_empty() {
        return Err(OrchardError::StateError(
            "Missing bundle bytes for signature verification".into()
        ));
    }

    // Validate public input layout (must be multiple of 9 for Orchard NU5)
    let registry = get_vk_registry();
    registry.validate_public_inputs(*vk_id, public_inputs)?;

    let action_count = registry.get_action_count(*vk_id, public_inputs)?;

    // Parse bundle and verify signatures
    log_info("  🔏 Parsing bundle for signature verification");
    let parsed_bundle = crate::bundle_parser::parse_bundle(bundle_bytes)?;

    // Compute bundle commitment per ZIP-244 (Zcash standard)
    // This MUST match what the builder uses when signing: proven_bundle.commitment()
    let sighash = crate::signature_verifier::compute_bundle_commitment(&parsed_bundle);

    log_info("  🔏 Verifying spend auth + binding signatures");
    crate::signature_verifier::verify_bundle_signatures(&parsed_bundle, &sighash)?;
    log_info("  ✅ All signatures verified");

    // Validate fee and gas bounds
    let fee_paid = validate_fee_and_gas(&parsed_bundle, bundle_bytes, refine_gas_limit)?;

    // Verify public inputs match Orchard NU5 layout
    verify_anchor_root_nu5(public_inputs, pre_state, action_count)?;
    verify_nullifiers_match_nu5(public_inputs, nullifiers, action_count)?;
    verify_commitments_match_nu5(public_inputs, commitments, action_count)?;

    // Verify bundle structure matches public inputs
    if parsed_bundle.action_count != action_count {
        return Err(OrchardError::StateError(format!(
            "Action count mismatch: bundle has {}, public inputs have {}",
            parsed_bundle.action_count, action_count
        )));
    }

    if parsed_bundle.actions.len() != nullifiers.len() {
        return Err(OrchardError::StateError(format!(
            "Nullifier count mismatch: bundle has {} actions, compact block has {} nullifiers",
            parsed_bundle.actions.len(), nullifiers.len()
        )));
    }

    if parsed_bundle.actions.len() != commitments.len() {
        return Err(OrchardError::StateError(format!(
            "Commitment count mismatch: bundle has {} actions, compact block has {} commitments",
            parsed_bundle.actions.len(), commitments.len()
        )));
    }

    // SECURITY FIX #2: Verify value_balance and anchor match public inputs
    verify_bundle_consistency(&parsed_bundle, public_inputs, pre_state)?;

    // Verify Halo2 proof after cheap structure/signature/fee checks pass.
    log_info("  🔐 Verifying Halo2 ZK proof");
    let verified = crate::crypto::verify_halo2_proof(
        proof_bytes,
        public_inputs,
        *vk_id,
    )?;
    if !verified {
        return Err(OrchardError::StateError(
            "Bundle ZK proof verification failed".into()
        ));
    }

    log_info("  ✅ Bundle fully verified (signatures + proof)");
    Ok(fee_paid)
}

fn verify_bundle_proof_v6(
    bundle_proof: &BundleProofV6,
    refine_gas_limit: u64,
    pre_state: &PreState,
) -> Result<u64> {
    log_info("  🔐 Verifying V6 bundle (ZSA/Swap)");

    let issue_bundle = if let Some(issue_bundle_bytes) = bundle_proof.issue_bundle_bytes.as_ref() {
        decode_issue_bundle(issue_bundle_bytes)?
    } else {
        None
    };

    if bundle_proof.orchard_bundle_bytes.is_empty() {
        if bundle_proof.bundle_type != OrchardBundleType::ZSA {
            return Err(OrchardError::StateError(
                "IssueBundle-only submissions must use ZSA bundle type".into(),
            ));
        }
        let issue_bundle = issue_bundle.ok_or_else(|| {
            OrchardError::StateError("IssueBundle-only submission requires issue bundle bytes".into())
        })?;
        verify_issue_bundle(&issue_bundle)?;
        log_info("  ✅ IssueBundle-only verification complete");
        let issue_bundle_bytes = bundle_proof.issue_bundle_bytes.as_ref().ok_or_else(|| {
            OrchardError::StateError("IssueBundle-only submission requires issue bundle bytes".into())
        })?;
        return validate_fee_and_gas_v6(
            0,
            0,
            issue_bundle_bytes,
            0,
            refine_gas_limit,
        );
    }

    let (action_count, total_proof_bytes, value_balance) = match bundle_proof.bundle_type {
        OrchardBundleType::Vanilla => {
            return Err(OrchardError::StateError(
                "BundleProofV6 does not support Vanilla bundles".into(),
            ));
        }
        OrchardBundleType::ZSA => {
            let bundle = decode_zsa_bundle(&bundle_proof.orchard_bundle_bytes)?;
            validate_orchard_flags(bundle.flags)?;
            if bundle.anchor != pre_state.commitment_root {
                return Err(OrchardError::StateError(
                    "ZSA bundle anchor mismatch".into(),
                ));
            }

            let sighash = compute_zsa_bundle_commitment(&bundle)?;
            verify_zsa_bundle_signatures(&bundle, &sighash)?;

            let public_inputs =
                build_zsa_public_inputs(&bundle.actions, bundle.flags, &bundle.anchor)?;
            let verified = crate::crypto::verify_halo2_proof(
                &bundle.proof,
                &public_inputs,
                2,
            )?;
            if !verified {
                return Err(OrchardError::StateError(
                    "ZSA bundle proof verification failed".into(),
                ));
            }

            (
                bundle.actions.len() as u64,
                bundle.proof.len() as u64,
                bundle.value_balance,
            )
        }
        OrchardBundleType::Swap => {
            let bundle = decode_swap_bundle(&bundle_proof.orchard_bundle_bytes)?;
            let mut total_actions = 0u64;
            let mut proof_bytes = 0u64;

            for (idx, group) in bundle.action_groups.iter().enumerate() {
                validate_orchard_flags(group.flags)?;
                if group.anchor != pre_state.commitment_root {
                    return Err(OrchardError::StateError(format!(
                        "Swap action group {} anchor mismatch",
                        idx
                    )));
                }

                total_actions += group.actions.len() as u64;
                proof_bytes += group.proof.len() as u64;
            }

            let sighash = compute_swap_bundle_commitment(&bundle)?;
            verify_swap_bundle(&bundle, &sighash)?;

            (total_actions, proof_bytes, bundle.value_balance)
        }
    };

    if let Some(issue_bundle) = issue_bundle.as_ref() {
        verify_issue_bundle(issue_bundle)?;
    }

    validate_fee_and_gas_v6(
        action_count,
        value_balance,
        &bundle_proof.orchard_bundle_bytes,
        total_proof_bytes,
        refine_gas_limit,
    )
}

fn validate_orchard_flags(flags: u8) -> Result<()> {
    if (flags & 0xFC) != 0 {
        return Err(OrchardError::StateError(
            "Invalid Orchard flags: only bits 0-1 are allowed".into(),
        ));
    }
    Ok(())
}

fn build_zsa_public_inputs(
    actions: &[DecodedActionV6],
    flags: u8,
    anchor: &[u8; 32],
) -> Result<Vec<[u8; 32]>> {
    validate_orchard_flags(flags)?;
    let enable_spend = flag_to_field_bytes((flags & 0x01) != 0);
    let enable_output = flag_to_field_bytes((flags & 0x02) != 0);
    let enable_zsa = flag_to_field_bytes(true);

    let mut inputs = Vec::with_capacity(actions.len() * 10);
    for action in actions {
        let (cv_net_x, cv_net_y) = point_bytes_to_xy(&action.cv_net, true, "cv_net")?;
        let (rk_x, rk_y) = point_bytes_to_xy(&action.rk, false, "rk")?;

        inputs.push(*anchor);
        inputs.push(cv_net_x);
        inputs.push(cv_net_y);
        inputs.push(action.nullifier);
        inputs.push(rk_x);
        inputs.push(rk_y);
        inputs.push(action.cmx);
        inputs.push(enable_spend);
        inputs.push(enable_output);
        inputs.push(enable_zsa);
    }

    Ok(inputs)
}

fn validate_fee_and_gas_v6(
    action_count: u64,
    value_balance: i64,
    bundle_bytes: &[u8],
    proof_bytes: u64,
    refine_gas_limit: u64,
) -> Result<u64> {
    if refine_gas_limit == 0 {
        return Err(OrchardError::StateError(
            "Refine gas limit must be non-zero".into(),
        ));
    }

    let min_fee = MIN_FEE_PER_ACTION * action_count;
    let fee = if value_balance > 0 {
        value_balance as u64
    } else {
        0u64
    };

    if fee < min_fee {
        return Err(OrchardError::StateError(format!(
            "Fee below minimum: {} < {} (required {} per action × {} actions)",
            fee, min_fee, MIN_FEE_PER_ACTION, action_count
        )));
    }

    let bundle_size_gas = (bundle_bytes.len() as u64) * GAS_PER_BUNDLE_BYTE;
    let proof_size_gas = proof_bytes * GAS_PER_PROOF_BYTE;
    let transfer_gas = if fee > 0 { FEE_TRANSFER_GAS } else { 0 };
    let estimated_gas = GAS_BASE_COST
        + (GAS_PER_ACTION * action_count)
        + bundle_size_gas
        + proof_size_gas
        + transfer_gas;

    let max_gas = core::cmp::min(refine_gas_limit, MAX_GAS_PER_WORK_PACKAGE);
    if estimated_gas > max_gas {
        return Err(OrchardError::StateError(format!(
            "Estimated gas {} exceeds maximum {} (bundle has {} actions, refine limit {})",
            estimated_gas, max_gas, action_count, refine_gas_limit
        )));
    }

    if fee < estimated_gas {
        return Err(OrchardError::StateError(format!(
            "Fee insufficient for gas: {} < estimated {}",
            fee, estimated_gas
        )));
    }

    log_info(&format!(
        "  💰 V6 fee validation: {} units for {} actions (min: {}, estimated gas: {} [base:{} + actions:{} + bundle:{} + proof:{} + transfer:{}], refine limit: {})",
        fee,
        action_count,
        min_fee,
        estimated_gas,
        GAS_BASE_COST,
        GAS_PER_ACTION * action_count,
        bundle_size_gas,
        proof_size_gas,
        transfer_gas,
        refine_gas_limit
    ));

    Ok(fee)
}

/// Verify anchor root matches pre_state using Orchard NU5 layout (field 0 of each action)
fn verify_anchor_root_nu5(
    public_inputs: &[[u8; 32]],
    pre_state: &PreState,
    action_count: usize,
) -> Result<()> {
    // Orchard NU5 layout: each action has 9 fields, anchor is field 0
    for action_idx in 0..action_count {
        let anchor_field_idx = action_idx * 9;
        let anchor_from_pi = &public_inputs[anchor_field_idx];

        if anchor_from_pi != &pre_state.commitment_root {
            return Err(OrchardError::StateError(format!(
                "Action {} anchor mismatch: public input {:?} != pre_state root {:?}",
                action_idx, anchor_from_pi, pre_state.commitment_root
            )));
        }
    }
    Ok(())
}

/// Verify nullifiers match compact block using Orchard NU5 layout (field 3 of each action)
fn verify_nullifiers_match_nu5(
    public_inputs: &[[u8; 32]],
    nullifiers: &[[u8; 32]],
    action_count: usize,
) -> Result<()> {
    if nullifiers.len() != action_count {
        return Err(OrchardError::StateError(format!(
            "Nullifier count mismatch: expected {}, got {}",
            action_count, nullifiers.len()
        )));
    }

    // Orchard NU5 layout: each action has 9 fields, nf_old is field 3
    for action_idx in 0..action_count {
        let nf_field_idx = action_idx * 9 + 3;
        let nf_from_pi = &public_inputs[nf_field_idx];
        let nf_from_compact = &nullifiers[action_idx];

        if nf_from_pi != nf_from_compact {
            return Err(OrchardError::StateError(format!(
                "Action {} nullifier mismatch: public input {:?} != compact block {:?}",
                action_idx, nf_from_pi, nf_from_compact
            )));
        }
    }
    Ok(())
}

/// Verify commitments match compact block using Orchard NU5 layout (field 6 of each action)
fn verify_commitments_match_nu5(
    public_inputs: &[[u8; 32]],
    commitments: &[crate::compactblock::OrchardCompactOutput],
    action_count: usize,
) -> Result<()> {
    if commitments.len() != action_count {
        return Err(OrchardError::StateError(format!(
            "Commitment count mismatch: expected {}, got {}",
            action_count, commitments.len()
        )));
    }

    // Orchard NU5 layout: each action has 9 fields, cmx is field 6
    for action_idx in 0..action_count {
        let cmx_field_idx = action_idx * 9 + 6;
        let cmx_from_pi = &public_inputs[cmx_field_idx];
        let cmx_from_compact = &commitments[action_idx].commitment;

        if cmx_from_pi != cmx_from_compact {
            return Err(OrchardError::StateError(format!(
                "Action {} commitment mismatch: public input {:?} != compact block {:?}",
                action_idx, cmx_from_pi, cmx_from_compact
            )));
        }
    }
    Ok(())
}

/// Verify bundle value_balance and anchor match public inputs
fn verify_bundle_consistency(
    parsed_bundle: &crate::bundle_parser::ParsedBundle,
    public_inputs: &[[u8; 32]],
    pre_state: &PreState,
) -> Result<()> {
    // Verify anchor from bundle matches pre_state
    if parsed_bundle.anchor != pre_state.commitment_root {
        return Err(OrchardError::StateError(format!(
            "Bundle anchor mismatch: {:?} != pre_state root {:?}",
            parsed_bundle.anchor, pre_state.commitment_root
        )));
    }

    // Verify anchor from bundle matches public inputs (field 0 of first action)
    if !public_inputs.is_empty() {
        let anchor_from_pi = &public_inputs[0];
        if &parsed_bundle.anchor != anchor_from_pi {
            return Err(OrchardError::StateError(format!(
                "Bundle anchor {:?} != public input anchor {:?}",
                parsed_bundle.anchor, anchor_from_pi
            )));
        }
    }

    if (parsed_bundle.flags & 0xFC) != 0 {
        return Err(OrchardError::StateError(
            "Invalid Orchard flags: only bits 0-1 are allowed".into()
        ));
    }

    let expected_spend = flag_to_field_bytes((parsed_bundle.flags & 0x01) != 0);
    let expected_output = flag_to_field_bytes((parsed_bundle.flags & 0x02) != 0);

    // CRITICAL: Verify ALL per-action fields match between bundle and public inputs
    // This prevents signature swap attacks where builder signs bundle A but submits proof for bundle B
    for (action_idx, action) in parsed_bundle.actions.iter().enumerate() {
        let base_idx = action_idx * 9;

        // Field 0: anchor (already checked above)

        // Fields 1-2: cv_net x/y - MUST MATCH
        let (cv_net_x, cv_net_y) =
            point_bytes_to_xy(&action.cv_net, true, "cv_net")?;
        let cv_net_x_idx = base_idx + 1;
        let cv_net_y_idx = base_idx + 2;
        if cv_net_y_idx < public_inputs.len() {
            let cv_net_x_from_pi = &public_inputs[cv_net_x_idx];
            let cv_net_y_from_pi = &public_inputs[cv_net_y_idx];
            if &cv_net_x != cv_net_x_from_pi || &cv_net_y != cv_net_y_from_pi {
                return Err(OrchardError::StateError(format!(
                    "Action {} cv_net mismatch: bundle {:?}/{:?} != public input {:?}/{:?}",
                    action_idx, cv_net_x, cv_net_y, cv_net_x_from_pi, cv_net_y_from_pi
                )));
            }
        }

        // Field 3: nf_old (nullifier) - MUST MATCH
        let nf_idx = base_idx + 3;
        if nf_idx < public_inputs.len() {
            let nf_from_pi = &public_inputs[nf_idx];
            if &action.nullifier != nf_from_pi {
                return Err(OrchardError::StateError(format!(
                    "Action {} nullifier mismatch: bundle {:?} != public input {:?}",
                    action_idx, action.nullifier, nf_from_pi
                )));
            }
        }

        // Fields 4-5: rk x/y - MUST MATCH
        let (rk_x, rk_y) = point_bytes_to_xy(&action.rk, false, "rk")?;
        let rk_x_idx = base_idx + 4;
        let rk_y_idx = base_idx + 5;
        if rk_y_idx < public_inputs.len() {
            let rk_x_from_pi = &public_inputs[rk_x_idx];
            let rk_y_from_pi = &public_inputs[rk_y_idx];
            if &rk_x != rk_x_from_pi || &rk_y != rk_y_from_pi {
                return Err(OrchardError::StateError(format!(
                    "Action {} rk mismatch: bundle {:?}/{:?} != public input {:?}/{:?}",
                    action_idx, rk_x, rk_y, rk_x_from_pi, rk_y_from_pi
                )));
            }
        }

        // Field 6: cmx - MUST MATCH
        let cmx_idx = base_idx + 6;
        if cmx_idx < public_inputs.len() {
            let cmx_from_pi = &public_inputs[cmx_idx];
            if &action.cmx != cmx_from_pi {
                return Err(OrchardError::StateError(format!(
                    "Action {} cmx mismatch: bundle {:?} != public input {:?}",
                    action_idx, action.cmx, cmx_from_pi
                )));
            }
        }

        // Fields 7-8: enable_spend, enable_output (bundle-level flags)
        let enable_spend_idx = base_idx + 7;
        let enable_output_idx = base_idx + 8;
        if enable_output_idx < public_inputs.len() {
            if public_inputs[enable_spend_idx] != expected_spend {
                return Err(OrchardError::StateError(format!(
                    "Action {} enable_spend mismatch: public input {:?} != expected {:?}",
                    action_idx, public_inputs[enable_spend_idx], expected_spend
                )));
            }
            if public_inputs[enable_output_idx] != expected_output {
                return Err(OrchardError::StateError(format!(
                    "Action {} enable_output mismatch: public input {:?} != expected {:?}",
                    action_idx, public_inputs[enable_output_idx], expected_output
                )));
            }
        }
    }

    Ok(())
}

fn flag_to_field_bytes(enabled: bool) -> [u8; 32] {
    let value = if enabled { 1u64 } else { 0u64 };
    pallas::Base::from(value).to_repr()
}

fn point_bytes_to_xy(
    bytes: &[u8; 32],
    allow_identity: bool,
    label: &str,
) -> Result<([u8; 32], [u8; 32])> {
    let point = pallas::Point::from_bytes(bytes)
        .into_option()
        .ok_or_else(|| OrchardError::StateError(format!("Invalid {} point bytes", label)))?;
    let coords = point.to_affine().coordinates().into_option();

    if let Some(coords) = coords {
        Ok((coords.x().to_repr(), coords.y().to_repr()))
    } else if allow_identity {
        let zero = pallas::Base::from(0u64).to_repr();
        Ok((zero, zero))
    } else {
        Err(OrchardError::StateError(format!("{} point is identity", label)))
    }
}

fn verify_anchor_root(
    public_inputs: &[[u8; 32]],
    pre_state: &PreState,
    layout: &[&'static str],
) -> Result<()> {
    let anchor_pos = layout.iter()
        .position(|field| *field == "anchor_root")
        .ok_or_else(|| OrchardError::StateError(
            "anchor_root not found in circuit layout".into()
        ))?;

    if public_inputs[anchor_pos] != pre_state.commitment_root {
        return Err(OrchardError::StateError(
            "Anchor root mismatch in public inputs".into()
        ));
    }

    Ok(())
}

fn verify_nullifiers_match(
    public_inputs: &[[u8; 32]],
    nullifiers: &[[u8; 32]],
    layout: &[&'static str],
) -> Result<()> {
    let positions: Vec<usize> = layout.iter()
        .enumerate()
        .filter_map(|(idx, field)| {
            if field.starts_with("input_nullifier_") {
                Some(idx)
            } else {
                None
            }
        })
        .collect();

    if positions.is_empty() {
        return Ok(());
    }

    if nullifiers.len() > positions.len() {
        return Err(OrchardError::StateError(
            "Too many nullifiers for circuit layout".into()
        ));
    }

    for (idx, nullifier) in nullifiers.iter().enumerate() {
        if public_inputs[positions[idx]] != *nullifier {
            return Err(OrchardError::StateError(format!(
                "Nullifier {} mismatch in public inputs",
                idx
            )));
        }
    }

    for idx in nullifiers.len()..positions.len() {
        if public_inputs[positions[idx]] != [0u8; 32] {
            return Err(OrchardError::StateError(format!(
                "Nullifier padding {} should be zero",
                idx
            )));
        }
    }

    Ok(())
}

fn verify_commitments_match(
    public_inputs: &[[u8; 32]],
    commitments: &[crate::compactblock::OrchardCompactOutput],
    layout: &[&'static str],
) -> Result<()> {
    let positions: Vec<usize> = layout.iter()
        .enumerate()
        .filter_map(|(idx, field)| {
            if field.starts_with("output_commitment_") {
                Some(idx)
            } else {
                None
            }
        })
        .collect();

    if positions.is_empty() {
        return Ok(());
    }

    if commitments.len() > positions.len() {
        return Err(OrchardError::StateError(
            "Too many commitments for circuit layout".into()
        ));
    }

    for (idx, commitment) in commitments.iter().enumerate() {
        if public_inputs[positions[idx]] != commitment.commitment {
            return Err(OrchardError::StateError(format!(
                "Commitment {} mismatch in public inputs",
                idx
            )));
        }
    }

    for idx in commitments.len()..positions.len() {
        if public_inputs[positions[idx]] != [0u8; 32] {
            return Err(OrchardError::StateError(format!(
                "Commitment padding {} should be zero",
                idx
            )));
        }
    }

    Ok(())
}

fn compute_nullifier_root_after_spends(
    pre_root: [u8; 32],
    nullifiers: &[[u8; 32]],
    nullifier_proofs: &HashMap<[u8; 32], Vec<[u8; 32]>>,
) -> Result<[u8; 32]> {
    if nullifiers.is_empty() {
        return Ok(pre_root);
    }

    log_info(&format!(
        "  🧮 Nullifier root update: {} nullifiers",
        nullifiers.len()
    ));

    let mut depth: Option<usize> = None;
    let mut updated_nodes: HashMap<SparseNodeKey, [u8; 32]> = HashMap::new();
    let mut proof_siblings: HashMap<SparseNodeKey, [u8; 32]> = HashMap::new();

    for nullifier in nullifiers {
        let proof_path = nullifier_proofs.get(nullifier)
            .ok_or_else(|| OrchardError::StateError(
                "Missing nullifier proof when computing post-state root".into()
            ))?;
        if proof_path.len() != NULLIFIER_TREE_DEPTH {
            return Err(OrchardError::StateError(format!(
                "Nullifier proof depth mismatch: expected {}, got {}",
                NULLIFIER_TREE_DEPTH,
                proof_path.len()
            )));
        }
        let proof_depth = proof_path.len();
        if proof_depth == 0 {
            return Err(OrchardError::StateError(
                "Nullifier proof has zero depth".into()
            ));
        }
        if let Some(expected) = depth {
            if expected != proof_depth {
                return Err(OrchardError::StateError(
                    "Nullifier proof depths are inconsistent".into()
                ));
            }
        } else {
            depth = Some(proof_depth);
        }

        let position = sparse_position_for(*nullifier, proof_depth);
        let leaf_key = SparseNodeKey { depth: 0, index: position };
        insert_sparse_node(&mut updated_nodes, leaf_key, sparse_hash_leaf(nullifier))?;

        let mut pos = position;
        for (level, sibling) in proof_path.iter().enumerate() {
            let sibling_key = SparseNodeKey { depth: level, index: pos ^ 1 };
            insert_sparse_node(&mut proof_siblings, sibling_key, *sibling)?;
            pos >>= 1;
        }
    }

    let depth = depth.unwrap_or(0);
    log_info(&format!("  🧮 Nullifier root depth: {}", depth));
    for level in 0..depth {
        log_info(&format!(
            "  🧮 Nullifier root level {}/{}",
            level + 1,
            depth
        ));
        let mut parent_indices = Vec::new();
        for key in updated_nodes.keys() {
            if key.depth == level {
                let parent = key.index >> 1;
                if !parent_indices.contains(&parent) {
                    parent_indices.push(parent);
                }
            }
        }

        for parent_index in parent_indices {
            let left_index = parent_index << 1;
            let right_index = left_index + 1;
            let left = get_sparse_node_hash(&updated_nodes, &proof_siblings, level, left_index)?;
            let right = get_sparse_node_hash(&updated_nodes, &proof_siblings, level, right_index)?;
            let parent_hash = hash_sparse_node(&left, &right);
            let parent_key = SparseNodeKey { depth: level + 1, index: parent_index };
            insert_sparse_node(&mut updated_nodes, parent_key, parent_hash)?;
        }
    }

    let root_key = SparseNodeKey { depth, index: 0 };
    let root = updated_nodes.get(&root_key).copied().ok_or_else(|| {
        OrchardError::StateError("Failed to compute nullifier root".into())
    })?;
    log_info("  ✅ Nullifier root update complete");
    Ok(root)
}

fn get_sparse_node_hash(
    updated_nodes: &HashMap<SparseNodeKey, [u8; 32]>,
    proof_siblings: &HashMap<SparseNodeKey, [u8; 32]>,
    depth: usize,
    index: u64,
) -> Result<[u8; 32]> {
    let key = SparseNodeKey { depth, index };
    if let Some(hash) = updated_nodes.get(&key) {
        return Ok(*hash);
    }
    if let Some(hash) = proof_siblings.get(&key) {
        return Ok(*hash);
    }
    Err(OrchardError::StateError(
        "Missing sibling hash for nullifier update".into()
    ))
}

fn insert_sparse_node(
    nodes: &mut HashMap<SparseNodeKey, [u8; 32]>,
    key: SparseNodeKey,
    value: [u8; 32],
) -> Result<()> {
    if let Some(existing) = nodes.get(&key) {
        if *existing != value {
            return Err(OrchardError::StateError(
                "Conflicting sparse tree update".into()
            ));
        }
    } else {
        nodes.insert(key, value);
    }
    Ok(())
}

fn build_state_write_intents(
    service_id: u32,
    refine_context: &RefineContext,
    pre_state: &PreState,
    transition: &VerifiedStateTransition,
) -> Result<Vec<WriteIntent>> {
    let mut intents = Vec::new();
    intents.push(build_kv_transition_intent(
        service_id,
        refine_context,
        "commitment_root",
        &pre_state.commitment_root,
        &transition.new_commitment_root,
        ObjectKind::StateWrite,
    )?);
    intents.push(build_kv_transition_intent(
        service_id,
        refine_context,
        "commitment_size",
        &pre_state.commitment_size.to_le_bytes(),
        &transition.new_commitment_size.to_le_bytes(),
        ObjectKind::StateWrite,
    )?);
    intents.push(build_kv_transition_intent(
        service_id,
        refine_context,
        "nullifier_root",
        &pre_state.nullifier_root,
        &transition.new_nullifier_root,
        ObjectKind::StateWrite,
    )?);
    intents.push(build_kv_transition_intent(
        service_id,
        refine_context,
        "nullifier_size",
        &pre_state.nullifier_size.to_le_bytes(),
        &transition.new_nullifier_size.to_le_bytes(),
        ObjectKind::StateWrite,
    )?);
    if transition.transparent_verified {
        intents.push(build_kv_transition_intent(
            service_id,
            refine_context,
            "transparent_merkle_root",
            &pre_state.transparent_merkle_root,
            &transition.new_transparent_merkle_root,
            ObjectKind::StateWrite,
        )?);
        intents.push(build_kv_transition_intent(
            service_id,
            refine_context,
            "transparent_utxo_root",
            &pre_state.transparent_utxo_root,
            &transition.new_transparent_utxo_root,
            ObjectKind::StateWrite,
        )?);
        intents.push(build_kv_transition_intent(
            service_id,
            refine_context,
            "transparent_utxo_size",
            &pre_state.transparent_utxo_size.to_le_bytes(),
            &transition.new_transparent_utxo_size.to_le_bytes(),
            ObjectKind::StateWrite,
        )?);
    }

    for (offset, commitment) in transition.new_commitments.iter().enumerate() {
        let index = pre_state.commitment_size + offset as u64;
        let key = format!("commitment_{}", index);
        intents.push(build_kv_intent(
            service_id,
            refine_context,
            &key,
            commitment,
            ObjectKind::Commitment,
        )?);
    }

    if transition.fee_paid > 0 {
        intents.push(build_fee_intent(
            service_id,
            refine_context,
            transition.fee_paid,
        )?);
    }

    Ok(intents)
}

fn build_fee_intent(
    service_id: u32,
    refine_context: &RefineContext,
    fee_paid: u64,
) -> Result<WriteIntent> {
    let object_id = compute_storage_opaque_key(service_id, b"fee_tally");
    let payload = (fee_paid as u128).to_le_bytes().to_vec();

    Ok(WriteIntent {
        effect: utils::effects::WriteEffectEntry {
            object_id,
            ref_info: utils::effects::ObjectRef {
                work_package_hash: refine_context.anchor,
                index_start: 0,
                payload_length: payload.len() as u32,
                object_kind: ObjectKind::FeeTally as u8,
            },
            payload,
            tx_index: 0,
        },
    })
}

fn build_kv_intent(
    service_id: u32,
    refine_context: &RefineContext,
    key: &str,
    value: &[u8],
    object_kind: ObjectKind,
) -> Result<WriteIntent> {
    let payload = encode_kv_payload(key, value)?;
    let object_id = compute_storage_opaque_key(service_id, key.as_bytes());

    Ok(WriteIntent {
        effect: utils::effects::WriteEffectEntry {
            object_id,
            ref_info: utils::effects::ObjectRef {
                work_package_hash: refine_context.anchor,
                index_start: 0,
                payload_length: payload.len() as u32,
                object_kind: object_kind as u8,
            },
            payload,
            tx_index: 0,
        },
    })
}

fn build_kv_transition_intent(
    service_id: u32,
    refine_context: &RefineContext,
    key: &str,
    old_value: &[u8],
    new_value: &[u8],
    object_kind: ObjectKind,
) -> Result<WriteIntent> {
    let payload = encode_transition_payload(key, old_value, new_value)?;
    let object_id = compute_storage_opaque_key(service_id, key.as_bytes());

    Ok(WriteIntent {
        effect: utils::effects::WriteEffectEntry {
            object_id,
            ref_info: utils::effects::ObjectRef {
                work_package_hash: refine_context.anchor,
                index_start: 0,
                payload_length: payload.len() as u32,
                object_kind: object_kind as u8,
            },
            payload,
            tx_index: 0,
        },
    })
}

fn encode_kv_payload(key: &str, value: &[u8]) -> Result<Vec<u8>> {
    let key_len = u16::try_from(key.len())
        .map_err(|_| OrchardError::ParseError("State key too long".into()))?;
    let mut payload = Vec::with_capacity(2 + key.len() + value.len());
    payload.extend_from_slice(&key_len.to_le_bytes());
    payload.extend_from_slice(key.as_bytes());
    payload.extend_from_slice(value);
    Ok(payload)
}

fn encode_transition_payload(key: &str, old_value: &[u8], new_value: &[u8]) -> Result<Vec<u8>> {
    if old_value.len() != new_value.len() {
        return Err(OrchardError::ParseError(
            "Transition payload value length mismatch".into()
        ));
    }

    let key_len = u16::try_from(key.len())
        .map_err(|_| OrchardError::ParseError("State key too long".into()))?;
    let mut payload = Vec::with_capacity(3 + key.len() + (old_value.len() * 2));
    payload.extend_from_slice(&key_len.to_le_bytes());
    payload.extend_from_slice(key.as_bytes());
    payload.push(1u8);
    payload.extend_from_slice(old_value);
    payload.extend_from_slice(new_value);
    Ok(payload)
}

fn read_u64(bytes: &[u8], cursor: &mut usize) -> Result<u64> {
    let value = read_array::<8>(bytes, cursor)?;
    Ok(u64::from_le_bytes(value))
}

fn read_u32(bytes: &[u8], cursor: &mut usize) -> Result<u32> {
    let value = read_array::<4>(bytes, cursor)?;
    Ok(u32::from_le_bytes(value))
}

fn read_array<const N: usize>(bytes: &[u8], cursor: &mut usize) -> Result<[u8; N]> {
    let end = *cursor + N;
    let slice = bytes
        .get(*cursor..end)
        .ok_or_else(|| OrchardError::ParseError("unexpected end of input".into()))?;
    *cursor = end;
    let mut out = [0u8; N];
    out.copy_from_slice(slice);
    Ok(out)
}

/// Compute JAM storage opaque key
fn compute_storage_opaque_key(service_id: u32, raw_key: &[u8]) -> [u8; 32] {
    use utils::hash_functions::blake2b_hash;

    // Step 1: Compute_storageKey_internal: k -> E4(2^32-1)++k
    let mut as_internal_key = Vec::with_capacity(4 + raw_key.len());
    let prefix = u32::MAX;
    as_internal_key.extend_from_slice(&prefix.to_le_bytes());
    as_internal_key.extend_from_slice(raw_key);

    // Step 2: Hash the internal key
    let hash = blake2b_hash(&as_internal_key);

    // Step 3: Interleave service_id with hash
    let n = service_id.to_le_bytes();
    let mut state_key = [0u8; 32];
    for i in 0..4 {
        state_key[2 * i] = n[i];
        state_key[2 * i + 1] = hash[i];
    }
    state_key[8..31].copy_from_slice(&hash[4..27]);
    // Last byte stays 0

    state_key
}

/// Serialize execution effects for accumulate
pub fn serialize_effects(effects: &[WriteIntent]) -> Result<Vec<u8>> {
    let mut output = Vec::new();
    output.extend_from_slice(&(effects.len() as u32).to_le_bytes());

    for (idx, effect) in effects.iter().enumerate() {
        log_info(&format!(
            "  Write intent {}: object_id={:?}, kind={}, data_len={}",
            idx, &effect.effect.object_id[..8],
            effect.effect.ref_info.object_kind,
            effect.effect.payload.len()
        ));
        output.extend_from_slice(&effect.effect.object_id);
        output.push(effect.effect.ref_info.object_kind);
        output.extend_from_slice(&(effect.effect.payload.len() as u32).to_le_bytes());
        output.extend_from_slice(&effect.effect.payload);
    }

    Ok(output)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::bundle_codec::{DecodedIssueAction, DecodedIssueBundle, DecodedIssuedNote};
    use crate::compactblock::{CompactBlock, CompactTx, OrchardCompactOutput};
    use crate::crypto::{merkle_append_with_frontier, TREE_DEPTH};
    use crate::signature_verifier::compute_issue_bundle_commitments;
    use pasta_curves::group::Group;
    use pasta_curves::pallas;

    #[test]
    fn test_issue_bundle_commitments_update_tree_without_nullifiers() {
        let mut recipient = [0u8; 43];
        recipient[..11].copy_from_slice(&[1u8; 11]);
        recipient[11..].copy_from_slice(&pallas::Point::generator().to_bytes());

        let mut rho = [0u8; 32];
        rho[0] = 1;

        let note = DecodedIssuedNote {
            recipient,
            value: 5,
            rho,
            rseed: [2u8; 32],
        };

        let bundle = DecodedIssueBundle {
            issuer_key: vec![0u8; 33],
            actions: vec![DecodedIssueAction {
                asset_desc_hash: [3u8; 32],
                notes: vec![note],
                finalize: false,
            }],
            sighash_info: Vec::new(),
            signature: Vec::new(),
        };

        let commitments = compute_issue_bundle_commitments(&bundle).unwrap();

        let mut compact_block = CompactBlock::new(0, [0u8; 32], [0u8; 32], 0, Vec::new());
        let tx = CompactTx {
            index: 0,
            txid: [0u8; 32],
            nullifiers: Vec::new(),
            commitments: commitments
                .iter()
                .map(|cm| OrchardCompactOutput {
                    commitment: *cm,
                    ephemeral_key: [0u8; 32],
                    ciphertext: Vec::new(),
                })
                .collect(),
        };
        compact_block.add_transaction(tx);

        assert!(compact_block.get_all_nullifiers().is_empty());
        assert_eq!(compact_block.get_all_commitments().len(), commitments.len());

        let frontier = vec![[0u8; 32]; TREE_DEPTH];
        let new_root = merkle_append_with_frontier(
            &[0u8; 32],
            0,
            &frontier,
            &commitments,
        )
        .unwrap();

        assert_ne!(new_root, [0u8; 32]);
    }
}
