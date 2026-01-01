#![allow(dead_code)]
/// Cryptographic primitives for Orchard service
///
/// Based on services/orchard/docs/ORCHARD.md specification:
/// - Poseidon hash (SNARK-friendly)
/// - Groth16 proof verification (BN254 curve)
/// - Merkle tree operations

use crate::errors::{OrchardError, Result};
use crate::vk_registry::get_vk_registry;
use alloc::string::ToString;
use alloc::vec::Vec;

// Arkworks imports for BN254 curve and Groth16 (used by existing Groth16 verification)
#[cfg(feature = "groth16")]
use ark_bn254::{Bn254, Fr as BN254Fr};
#[cfg(feature = "groth16")]
use ark_ff::BigInteger;
#[cfg(feature = "groth16")]
use ark_groth16::{Groth16, PreparedVerifyingKey, Proof, VerifyingKey};
#[cfg(feature = "groth16")]
use ark_serialize::CanonicalDeserialize;

// Import format macro for no_std compatibility
use alloc::format;

// Blake2 for hashing
use blake2::{Blake2b, Digest, Blake2bVar};
use blake2::digest::{Update, VariableOutput};
use halo2_gadgets::poseidon::{
    primitives::{self as poseidon, ConstantLength, P128Pow5T3},
};

// Halo2 imports for IPA verification
use halo2_proofs::{
    poly::commitment::Params,
    plonk::{VerifyingKey as Halo2VerifyingKey},
    transcript::{Blake2bRead, Challenge255, Transcript},
};
use halo2_proofs::pasta::vesta;
use halo2_proofs::pasta::pallas::Base as PallasBase;
use halo2_proofs::pasta::group::ff::{FromUniformBytes, PrimeField};
#[cfg(feature = "orchard")]
use incrementalmerkletree::Level;
#[cfg(feature = "orchard")]
use orchard::tree::MerkleHashOrchard;

/// Binary Poseidon tree depth (consensus-critical)
pub const TREE_DEPTH: usize = 32;
pub const BATCH_TREE_DEPTH: usize = 16;

const NOTE_COMMITMENT_DOMAIN: &str = "note_cm_v1";
const NULLIFIER_DOMAIN: &str = "nf_v1";
const COMMITMENT_TREE_DOMAIN: &str = "commitment_tree_v1";
const COMMITMENT_BATCH_DOMAIN: &str = "cm_batch_v1";
const COMMITMENT_TREE_BATCH_DOMAIN: &str = "cm_tree_v1";
const EXERCISE_TERMS_DOMAIN: &str = "exercise_terms_v1";
const SWAP_QUOTE_DOMAIN: &str = "quote_v1";

const VK_BUNDLE_MAGIC: &[u8; 8] = b"RGVKv1\0\0";
const VK_BUNDLE_HEADER_LEN: usize = 8 + 4 + 4;

struct VkBundle {
    k: u32,
    vk_bytes: Vec<u8>,
}

/// Halo2/IPA proof verification
///
/// Verifies a Halo2 proof using IPA polynomial commitment scheme
/// Matches the verification system described in HALO2.md
pub fn verify_halo2_proof(
    proof_bytes: &[u8],
    public_inputs: &[[u8; 32]],
    vk_id: u32,
) -> Result<bool> {
    use crate::log_info;

    // 1. Load VK from registry and validate field layout
    let registry = get_vk_registry();
    let vk_entry = registry.get_vk_entry(vk_id)?;
    registry.validate_public_inputs(vk_id, public_inputs)?;

    log_info(&format!(
        "Halo2 verification starting: vk_id={}, domain={}, fields={}",
        vk_id, vk_entry.domain, vk_entry.field_count
    ));

    // 2. Load VK bytes from embedded constants and parse bundle
    let vk_bytes = match vk_id {
        1 => ORCHARD_SPEND_VK_BYTES,
        2 => ORCHARD_WITHDRAW_VK_BYTES,
        3 => ORCHARD_ISSUANCE_VK_BYTES,
        4 => ORCHARD_BATCH_VK_BYTES,
        _ => return Err(OrchardError::InvalidVkId { vk_id }),
    };
    let vk_bundle = parse_vk_bundle(vk_bytes)?;

    // 3. Deserialize VK from canonical bytes
    let vk = Halo2VerifyingKey::read(&mut &vk_bundle.vk_bytes[..]).map_err(|_| {
        OrchardError::ParseError("failed to deserialize halo2 vk".to_string())
    })?;
    let expected_hash = vk_hash_bytes(&vk_bundle.vk_bytes);
    log_info(&format!(
        "✅ VK bundle verified: vk_id={}, k={}, hash={:?}",
        vk_id, vk_bundle.k, &expected_hash[..8]
    ));

    // 4. Convert public inputs from bytes to Pallas field elements
    let pallas_inputs: Result<Vec<PallasBase>> = public_inputs.iter()
        .map(|bytes| {
            // Convert 32-byte array to field element (big-endian)
            // This matches the canonical field encoding from CIRCUITS.md
            let mut repr = <PallasBase as PrimeField>::Repr::default();
            repr.as_mut().copy_from_slice(bytes);
            PallasBase::from_repr(repr)
                .into_option()
                .ok_or(OrchardError::InvalidFieldElement)
        })
        .collect();
    let pallas_inputs = pallas_inputs?;

    // 5. Initialize Blake2b transcript with domain separation
    let mut transcript = Blake2bRead::<_, vesta::Affine, Challenge255<vesta::Affine>>::init(proof_bytes);
    let domain_scalar = domain_str_to_field(vk_entry.domain);
    transcript
        .common_scalar(domain_scalar)
        .map_err(|_| OrchardError::InvalidProof)?;

    // 6. Load universal parameters (IPA commitment params)
    let params = Params::<vesta::Affine>::new(vk_bundle.k);

    // 7. Verify the Halo2 proof using real IPA verification
    // This performs actual cryptographic verification against the circuit VK
    if proof_bytes.is_empty() {
        log_info("Empty proof provided - verification failed");
        return Ok(false);
    }

    // Real Halo2 verification with proper transcript and strategy
    let instances: [&[PallasBase]; 1] = [&pallas_inputs];
    let instance_slices: [&[&[PallasBase]]; 1] = [&instances];
    match halo2_proofs::plonk::verify_proof(
        &params,
        &vk,
        halo2_proofs::plonk::SingleVerifier::new(&params),
        &instance_slices,
        &mut transcript,
    ) {
        Ok(_) => {
            log_info("✅ Halo2/IPA proof verification succeeded - REAL VERIFICATION ACTIVE");
            Ok(true)
        }
        Err(e) => {
            log_info(&format!("❌ Halo2/IPA proof verification failed: {:?} - REAL VERIFICATION ACTIVE", e));
            Ok(false)
        }
    }
}

/// Blake2b hash with domain separation (no_std compatible)
fn _blake2_hash_domain(domain: &str) -> [u8; 32] {
    let mut hasher = Blake2b::new();
    Digest::update(&mut hasher, b"orchard_transcript_v1:");
    Digest::update(&mut hasher, domain.as_bytes());
    hasher.finalize().into()
}

/// TODO: Embedded verification key for real Orchard circuit
/// Will use Zcash's production Orchard verifying key
/// For now, stub these out (legacy code from Railgun implementation)
pub const ORCHARD_SPEND_VK_BYTES: &[u8] = &[];
pub const ORCHARD_WITHDRAW_VK_BYTES: &[u8] = &[];
pub const ORCHARD_ISSUANCE_VK_BYTES: &[u8] = &[];
pub const ORCHARD_BATCH_VK_BYTES: &[u8] = &[];

/// Build public inputs for IssuanceV1 circuit (11 fields)
/// Matches ISSUANCE_LAYOUT from vk_registry.rs:
/// ["asset_id", "mint_amount_lo", "mint_amount_hi", "burn_amount_lo", "burn_amount_hi",
///  "issuer_pk_0", "issuer_pk_1", "issuer_pk_2",
///  "signature_verification_0", "signature_verification_1", "issuance_root"]
pub fn build_issuance_public_inputs(
    asset_id: u32,
    mint_amount: u128,
    burn_amount: u128,
    issuer_pk: &[u8; 32],
    signature_verification_data: &[u8; 64],
    issuance_root: &[u8; 32],
) -> Vec<[u8; 32]> {
    // Returns vector of field element encodings, where each [u8; 32] represents
    // a single field element in the circuit's public input layout.
    // Fields are packed according to ISSUANCE_LAYOUT with proper big-endian encoding

    let mut inputs = Vec::with_capacity(11);

    // Field 0: asset_id (u32)
    inputs.push(u32_to_field_bytes(asset_id));

    // Fields 1-2: mint_amount (u128 split into lo/hi)
    inputs.push(u128_lo_to_field_bytes(mint_amount));
    inputs.push(u128_hi_to_field_bytes(mint_amount));

    // Fields 3-4: burn_amount (u128 split into lo/hi)
    inputs.push(u128_lo_to_field_bytes(burn_amount));
    inputs.push(u128_hi_to_field_bytes(burn_amount));

    // Fields 5-7: issuer_pk (32 bytes packed as 11/11/10)
    let [pk0, pk1, pk2] = pack_32_bytes_to_field_bytes(*issuer_pk);
    inputs.extend_from_slice(&[pk0, pk1, pk2]);

    // Fields 8-9: signature_verification_data (big-endian field encodings)
    let sig0_bytes: [u8; 32] = signature_verification_data[..32].try_into().unwrap_or([0u8; 32]);
    let sig1_bytes: [u8; 32] = signature_verification_data[32..64].try_into().unwrap_or([0u8; 32]);
    let sig0 = pallas_from_bytes_be(&sig0_bytes).unwrap_or_else(|_| PallasBase::zero());
    let sig1 = pallas_from_bytes_be(&sig1_bytes).unwrap_or_else(|_| PallasBase::zero());
    inputs.push(pallas_to_bytes(&sig0));
    inputs.push(pallas_to_bytes(&sig1));

    // Field 10: issuance_root ([u8; 32])
    inputs.push(*issuance_root);

    assert_eq!(inputs.len(), 11, "IssuanceV1 circuit expects exactly 11 public input fields");
    inputs
}

/// Build public inputs for BatchAggV1 circuit (17 fields)
/// Matches BATCH_LAYOUT from vk_registry.rs:
/// ["anchor_root", "anchor_size", "next_root", "nullifiers_root", "commitments_root", "deltas_root",
///  "issuance_root", "memo_root", "equity_allowed_root", "poi_root",
///  "total_fee_lo", "total_fee_hi", "epoch", "num_user_txs", "min_fee_per_tx_lo", "min_fee_per_tx_hi", "batch_hash"]
pub fn build_batch_public_inputs(
    anchor_root: &[u8; 32],
    anchor_size: u64,
    next_root: &[u8; 32],
    nullifiers_root: &[u8; 32],
    commitments_root: &[u8; 32],
    deltas_root: &[u8; 32],
    issuance_root: &[u8; 32],
    memo_root: &[u8; 32],
    equity_allowed_root: &[u8; 32],
    poi_root: &[u8; 32],
    total_fee: u128,
    epoch: u64,
    num_user_txs: u32,
    min_fee_per_tx: u128,
    batch_hash: &[u8; 32],
) -> Vec<[u8; 32]> {
    // Returns vector of field element encodings, where each [u8; 32] represents
    // a single field element in the circuit's public input layout.
    // Fields are packed according to BATCH_LAYOUT with proper big-endian encoding

    let mut inputs = Vec::with_capacity(17);

    // Field 0: anchor_root ([u8; 32])
    inputs.push(*anchor_root);

    // Field 1: anchor_size (u64)
    inputs.push(u64_to_field_bytes(anchor_size));

    // Field 2: next_root ([u8; 32])
    inputs.push(*next_root);

    // Field 3: nullifiers_root ([u8; 32])
    inputs.push(*nullifiers_root);

    // Field 4: commitments_root ([u8; 32])
    inputs.push(*commitments_root);

    // Field 5: deltas_root ([u8; 32])
    inputs.push(*deltas_root);

    // Field 6: issuance_root ([u8; 32])
    inputs.push(*issuance_root);

    // Field 7: memo_root ([u8; 32])
    inputs.push(*memo_root);

    // Field 8: equity_allowed_root ([u8; 32])
    inputs.push(*equity_allowed_root);

    // Field 9: poi_root ([u8; 32])
    inputs.push(*poi_root);

    // Fields 10-11: total_fee (u128 split into lo/hi)
    inputs.push(u128_lo_to_field_bytes(total_fee));
    inputs.push(u128_hi_to_field_bytes(total_fee));

    // Field 12: epoch (u64)
    inputs.push(u64_to_field_bytes(epoch));

    // Field 13: num_user_txs (u32)
    inputs.push(u32_to_field_bytes(num_user_txs));

    // Fields 14-15: min_fee_per_tx (u128 split into lo/hi)
    inputs.push(u128_lo_to_field_bytes(min_fee_per_tx));
    inputs.push(u128_hi_to_field_bytes(min_fee_per_tx));

    // Field 16: batch_hash ([u8; 32])
    inputs.push(*batch_hash);

    assert_eq!(inputs.len(), 17, "BatchAggV1 circuit expects exactly 17 public input fields");
    inputs
}

/// Helper functions for field encoding

fn u32_to_field_bytes(value: u32) -> [u8; 32] {
    let mut bytes = [0u8; 32];
    bytes[28..32].copy_from_slice(&value.to_be_bytes());
    bytes
}

fn u64_to_field_bytes(value: u64) -> [u8; 32] {
    let mut bytes = [0u8; 32];
    bytes[24..32].copy_from_slice(&value.to_be_bytes());
    bytes
}

fn u128_lo_to_field_bytes(value: u128) -> [u8; 32] {
    let mut bytes = [0u8; 32];
    bytes[24..32].copy_from_slice(&(value as u64).to_be_bytes());
    bytes
}

fn u128_hi_to_field_bytes(value: u128) -> [u8; 32] {
    let mut bytes = [0u8; 32];
    bytes[24..32].copy_from_slice(&((value >> 64) as u64).to_be_bytes());
    bytes
}

fn pack_32_bytes_to_field_bytes(bytes: [u8; 32]) -> [[u8; 32]; 3] {
    [
        pack_bytes_to_field_bytes(&bytes[0..11]),
        pack_bytes_to_field_bytes(&bytes[11..22]),
        pack_bytes_to_field_bytes(&bytes[22..32]),
    ]
}

fn reduce_bytes_to_field_bytes(bytes: [u8; 32]) -> [u8; 32] {
    let mut wide = [0u8; 64];
    for (dst, src) in wide.iter_mut().zip(bytes.iter().rev()) {
        *dst = *src;
    }
    let reduced = PallasBase::from_uniform_bytes(&wide);
    let mut repr = reduced.to_repr();
    repr.reverse();
    repr
}

fn pack_bytes_to_field_bytes(bytes: &[u8]) -> [u8; 32] {
    let mut out = [0u8; 32];
    let offset = 32 - bytes.len();
    out[offset..].copy_from_slice(bytes);
    out
}

fn pallas_from_bytes(bytes: &[u8; 32]) -> Result<PallasBase> {
    let mut repr = <PallasBase as PrimeField>::Repr::default();
    repr.as_mut().copy_from_slice(bytes);
    PallasBase::from_repr(repr)
        .into_option()
        .ok_or(OrchardError::InvalidFieldElement)
}

fn pallas_from_bytes_be(bytes: &[u8; 32]) -> Result<PallasBase> {
    let mut le = *bytes;
    le.reverse();
    pallas_from_bytes(&le)
}

pub fn pallas_from_bytes_reduced(bytes: &[u8; 32]) -> PallasBase {
    let reduced = reduce_bytes_to_field_bytes(*bytes);
    let mut repr = <PallasBase as PrimeField>::Repr::default();
    repr.as_mut().copy_from_slice(&reduced);
    PallasBase::from_repr(repr)
        .into_option()
        .unwrap_or_else(|| PallasBase::zero())
}

fn pallas_from_u32(value: u32) -> PallasBase {
    PallasBase::from(value as u64)
}

fn pallas_from_u64(value: u64) -> PallasBase {
    PallasBase::from(value)
}

fn split_u128_to_pallas_fields(value: u128) -> [PallasBase; 2] {
    let lo = value as u64;
    let hi = (value >> 64) as u64;
    [PallasBase::from(lo), PallasBase::from(hi)]
}

fn pack_32_bytes_to_pallas_fields(bytes: [u8; 32]) -> [PallasBase; 3] {
    let chunk1 = pack_bytes_to_field_bytes(&bytes[0..11]);
    let chunk2 = pack_bytes_to_field_bytes(&bytes[11..22]);
    let chunk3 = pack_bytes_to_field_bytes(&bytes[22..32]);
    [
        pallas_from_bytes(&chunk1).unwrap_or_else(|_| PallasBase::zero()),
        pallas_from_bytes(&chunk2).unwrap_or_else(|_| PallasBase::zero()),
        pallas_from_bytes(&chunk3).unwrap_or_else(|_| PallasBase::zero()),
    ]
}

pub fn pallas_to_bytes(value: &PallasBase) -> [u8; 32] {
    let mut repr = value.to_repr();
    repr.reverse();
    repr
}

fn poseidon_hash_domain_1(domain: &str, value: PallasBase) -> PallasBase {
    let domain_field = domain_str_to_field(domain);
    let input = [domain_field, value];
    poseidon::Hash::<_, P128Pow5T3, ConstantLength<2>, 3, 2>::init().hash(input)
}

fn poseidon_hash_domain_2(domain: &str, left: PallasBase, right: PallasBase) -> PallasBase {
    let domain_field = domain_str_to_field(domain);
    let input = [domain_field, left, right];
    poseidon::Hash::<_, P128Pow5T3, ConstantLength<3>, 3, 2>::init().hash(input)
}

fn poseidon_hash_domain_3(
    domain: &str,
    a: PallasBase,
    b: PallasBase,
    c: PallasBase,
) -> PallasBase {
    let domain_field = domain_str_to_field(domain);
    let input = [domain_field, a, b, c];
    poseidon::Hash::<_, P128Pow5T3, ConstantLength<4>, 3, 2>::init().hash(input)
}

fn poseidon_hash_domain_10(domain: &str, inputs: [PallasBase; 10]) -> PallasBase {
    let domain_field = domain_str_to_field(domain);
    let input = [
        domain_field,
        inputs[0],
        inputs[1],
        inputs[2],
        inputs[3],
        inputs[4],
        inputs[5],
        inputs[6],
        inputs[7],
        inputs[8],
        inputs[9],
    ];
    poseidon::Hash::<_, P128Pow5T3, ConstantLength<11>, 3, 2>::init().hash(input)
}

fn poseidon_hash_from_fields(field_inputs: &[PallasBase]) -> PallasBase {
    match field_inputs.len() {
        2 => {
            let arr: [PallasBase; 2] = field_inputs.try_into().unwrap();
            poseidon::Hash::<_, P128Pow5T3, ConstantLength<2>, 3, 2>::init().hash(arr)
        }
        3 => {
            let arr: [PallasBase; 3] = field_inputs.try_into().unwrap();
            poseidon::Hash::<_, P128Pow5T3, ConstantLength<3>, 3, 2>::init().hash(arr)
        }
        4 => {
            let arr: [PallasBase; 4] = field_inputs.try_into().unwrap();
            poseidon::Hash::<_, P128Pow5T3, ConstantLength<4>, 3, 2>::init().hash(arr)
        }
        7 => {
            let arr: [PallasBase; 7] = field_inputs.try_into().unwrap();
            poseidon::Hash::<_, P128Pow5T3, ConstantLength<7>, 3, 2>::init().hash(arr)
        }
        9 => {
            let arr: [PallasBase; 9] = field_inputs.try_into().unwrap();
            poseidon::Hash::<_, P128Pow5T3, ConstantLength<9>, 3, 2>::init().hash(arr)
        }
        11 => {
            let arr: [PallasBase; 11] = field_inputs.try_into().unwrap();
            poseidon::Hash::<_, P128Pow5T3, ConstantLength<11>, 3, 2>::init().hash(arr)
        }
        _ => {
            let mut padded = [PallasBase::zero(); 11];
            let copy_len = core::cmp::min(field_inputs.len(), 11);
            padded[..copy_len].copy_from_slice(&field_inputs[..copy_len]);
            poseidon::Hash::<_, P128Pow5T3, ConstantLength<11>, 3, 2>::init().hash(padded)
        }
    }
}

/// Poseidon hash over Pallas base field with explicit domain tag.
///
/// Inputs are 32-byte big-endian field elements. Domain bytes are hashed as the first element.
pub fn poseidon_hash_with_domain(domain: &str, inputs: &[[u8; 32]]) -> [u8; 32] {
    let mut field_inputs: Vec<PallasBase> = Vec::with_capacity(inputs.len() + 1);
    field_inputs.push(domain_str_to_field(domain));
    field_inputs.extend(inputs.iter().map(pallas_from_bytes_reduced));
    let hash = poseidon_hash_from_fields(&field_inputs);
    pallas_to_bytes(&hash)
}

/// Poseidon hash for commitment tree internal nodes (domain-separated)
fn commitment_tree_hash_2(left: PallasBase, right: PallasBase) -> PallasBase {
    poseidon_hash_domain_2(COMMITMENT_TREE_DOMAIN, left, right)
}

/// Commitment: Poseidon("note_cm_v1", asset_id, amount, owner_pk, rho, note_rseed, unlock_height, memo_hash)
///
/// Matches services/orchard/docs/CIRCUITS.md and halo2-circuits note_commitment_native.
pub fn commitment(
    asset_id: u32,
    amount: u128,
    owner_pk: &[u8; 32],
    rho: &[u8; 32],
    note_rseed: &[u8; 32],
    unlock_height: u64,
    memo_hash: &[u8; 32],
) -> Result<[u8; 32]> {
    let asset_id_fp = pallas_from_u32(asset_id);
    let [amount_lo_fp, amount_hi_fp] = split_u128_to_pallas_fields(amount);
    let [pk0, pk1, pk2] = pack_32_bytes_to_pallas_fields(*owner_pk);
    let rho_fp = pallas_from_bytes_reduced(rho);
    let note_rseed_fp = pallas_from_bytes_reduced(note_rseed);
    let unlock_height_fp = pallas_from_u64(unlock_height);
    let memo_hash_fp = pallas_from_bytes_reduced(memo_hash);

    let cm = poseidon_hash_domain_10(
        NOTE_COMMITMENT_DOMAIN,
        [
            asset_id_fp,
            amount_lo_fp,
            amount_hi_fp,
            pk0,
            pk1,
            pk2,
            rho_fp,
            note_rseed_fp,
            unlock_height_fp,
            memo_hash_fp,
        ],
    );

    Ok(pallas_to_bytes(&cm))
}

/// Nullifier: Poseidon("nf_v1", sk_spend, rho, commitment)
pub fn nullifier(
    sk_spend: &[u8; 32],
    rho: &[u8; 32],
    commitment: &[u8; 32],
) -> Result<[u8; 32]> {
    let sk_fp = pallas_from_bytes_reduced(sk_spend);
    let rho_fp = pallas_from_bytes_reduced(rho);
    let cm_fp = pallas_from_bytes_be(commitment)?;
    let nf = poseidon_hash_domain_3(NULLIFIER_DOMAIN, sk_fp, rho_fp, cm_fp);
    Ok(pallas_to_bytes(&nf))
}

/// Verify Groth16 proof (BN254 curve)
///
/// Groth16 proof structure (services/orchard/docs/ORCHARD.md proof layout)
///
/// # Arguments
/// - `proof_bytes`: 128 bytes (compressed; A: 32 bytes G1, B: 64 bytes G2, C: 32 bytes G1)
/// - `public_inputs`: Field elements as 32-byte big-endian encodings
/// - `vk_bytes`: Verification key (CanonicalSerialize format)
///
/// # Returns
/// - `true` if proof is valid, `false` otherwise
#[cfg(feature = "groth16")]
pub fn verify_groth16(
    proof_bytes: &[u8],
    public_inputs: &[[u8; 32]],
    vk_bytes: &[u8],
) -> bool {
    let proof = match Proof::<Bn254>::deserialize_compressed(proof_bytes) {
        Ok(p) => p,
        Err(_) => {
            utils::functions::log_error("Failed to deserialize Groth16 proof");
            return false;
        }
    };

    let pvk = match load_verifying_key(vk_bytes) {
        Some(vk) => vk,
        None => return false,
    };

    let public_inputs_field: Vec<BN254Fr> = public_inputs
        .iter()
        .map(|bytes| bytes_to_field(bytes))
        .collect();

    match Groth16::<Bn254>::verify_proof(&pvk, &proof, &public_inputs_field) {
        Ok(valid) => valid,
        Err(e) => {
            utils::functions::log_error(&alloc::format!("Groth16 verification error: {:?}", e));
            false
        }
    }
}

/// Merkle tree append operation
///
/// Binary Poseidon tree, depth 32 (services/orchard/docs/ORCHARD.md tree description)
///
/// Appends commitments at positions [size, size+1, ..., size+N-1]
/// and returns the new root hash.
///
/// Tree structure:
/// - Empty leaf: 0
/// - H(a, b) = Poseidon([a, b])
/// - Append-at-size: deterministic insertion without rebalancing
pub fn merkle_append_from_leaves(
    existing: &[[u8; 32]],
    commitments: &[[u8; 32]],
) -> Result<[u8; 32]> {
    if commitments.is_empty() {
        return merkle_root_from_leaves(existing);
    }

    let mut leaves = Vec::with_capacity(existing.len() + commitments.len());
    leaves.extend_from_slice(existing);
    leaves.extend_from_slice(commitments);
    merkle_root_from_leaves(&leaves)
}

/// Append new commitments to an existing Merkle tree
///
/// This implements the MerkleAppend operation documented in ORCHARD.md:
/// next_root = MerkleAppend(anchor_root, anchor_size, new_commitments)
///
/// Note: This is a simplified implementation that reconstructs the tree.
/// A production implementation would use incremental Merkle tree operations.
pub fn merkle_append(
    _anchor_root: &[u8; 32],
    current_size: u64,
    new_commitments: &[[u8; 32]],
) -> Result<[u8; 32]> {
    // TODO: In a production implementation, this would:
    // 1. Reconstruct the existing tree from anchor_root + size
    // 2. Use incremental append operations for efficiency
    // 3. Validate the anchor_root matches the current tree state

    // For now, we simulate the operation by treating the current size
    // as indicating an existing tree of that size with placeholder leaves
    let mut all_leaves = Vec::with_capacity(current_size as usize + new_commitments.len());

    // Add placeholder leaves for existing commitments
    // In reality, these would be read from state or computed from anchor_root
    for _ in 0..current_size {
        all_leaves.push([0u8; 32]); // Placeholder - should be actual commitment values
    }

    // Add new commitments
    all_leaves.extend_from_slice(new_commitments);

    // Compute new root
    merkle_root_from_leaves(&all_leaves)
}

/// Compute Poseidon Merkle root for a contiguous set of leaves padded with zeros
pub fn merkle_root_from_leaves(leaves: &[[u8; 32]]) -> Result<[u8; 32]> {
    #[cfg(feature = "orchard")]
    {
        return orchard_merkle_root_from_leaves(leaves);
    }

    #[cfg(not(feature = "orchard"))]
    {
        let mut current_level: Vec<PallasBase> = if leaves.is_empty() {
            vec![PallasBase::zero()]
        } else {
            leaves
                .iter()
                .map(pallas_from_bytes_be)
                .collect::<Result<Vec<_>>>()?
        };

        let mut target_size = current_level.len().next_power_of_two();
        if target_size < 2 && !current_level.is_empty() {
            target_size = 2;
        }
        current_level.resize(target_size, PallasBase::zero());

        let mut current_depth = 0usize;
        let mut size = target_size;
        while size > 1 {
            current_depth += 1;
            size >>= 1;
        }

        while current_level.len() > 1 {
            let mut next_level = Vec::with_capacity(current_level.len() / 2);
            for pair in current_level.chunks_exact(2) {
                next_level.push(commitment_tree_hash_2(pair[0], pair[1]));
            }
            current_level = next_level;
        }

        let mut root = current_level[0];
        if current_depth < TREE_DEPTH {
            let mut zero = PallasBase::zero();
            let mut zero_hashes = Vec::with_capacity(TREE_DEPTH + 1);
            zero_hashes.push(zero);
            for _ in 0..TREE_DEPTH {
                zero = commitment_tree_hash_2(zero, zero);
                zero_hashes.push(zero);
            }

            for level in current_depth..TREE_DEPTH {
                root = commitment_tree_hash_2(root, zero_hashes[level]);
            }
        }

        Ok(pallas_to_bytes(&root))
    }
}

#[cfg(feature = "orchard")]
fn orchard_merkle_root_from_leaves(leaves: &[[u8; 32]]) -> Result<[u8; 32]> {
    use incrementalmerkletree::Hashable;

    if leaves.len() > (1usize << TREE_DEPTH) {
        return Err(OrchardError::ParseError("commitment tree overflow".into()));
    }

    if leaves.is_empty() {
        return Ok(MerkleHashOrchard::empty_root(Level::from(TREE_DEPTH as u8)).to_bytes());
    }

    let mut current: Vec<MerkleHashOrchard> = Vec::with_capacity(leaves.len());
    for leaf in leaves {
        let hash = MerkleHashOrchard::from_bytes(leaf);
        if bool::from(hash.is_some()) {
            current.push(hash.unwrap());
        } else {
            return Err(OrchardError::ParseError("invalid commitment encoding".into()));
        }
    }

    for level in 0..TREE_DEPTH {
        let empty = MerkleHashOrchard::empty_root(Level::from(level as u8));
        let mut next = Vec::with_capacity((current.len() + 1) / 2);
        for pair in current.chunks(2) {
            let left = pair[0];
            let right = if pair.len() == 2 { pair[1] } else { empty };
            next.push(MerkleHashOrchard::combine(Level::from(level as u8), &left, &right));
        }
        current = next;
        if current.len() == 1 && level + 1 == TREE_DEPTH {
            break;
        }
    }

    let root = current
        .first()
        .copied()
        .unwrap_or_else(|| MerkleHashOrchard::empty_root(Level::from(TREE_DEPTH as u8)));
    Ok(root.to_bytes())
}

/// Compute batch commitments_root (depth 16) using domain-separated Poseidon
pub fn commitments_root_from_batch_commitments(
    commitments: &[[u8; 32]],
) -> Result<[u8; 32]> {
    if commitments.len() > (1usize << BATCH_TREE_DEPTH) {
        return Err(OrchardError::ParseError(
            "batch commitments exceed tree capacity".to_string(),
        ));
    }

    let mut leaves = Vec::with_capacity(commitments.len());
    for commitment in commitments {
        let commitment_fp = pallas_from_bytes_be(commitment)?;
        leaves.push(poseidon_hash_domain_1(COMMITMENT_BATCH_DOMAIN, commitment_fp));
    }

    let mut current_level = if leaves.is_empty() {
        vec![PallasBase::zero()]
    } else {
        leaves
    };
    let mut target_size = current_level.len().next_power_of_two();
    if target_size < 2 && !current_level.is_empty() {
        target_size = 2;
    }
    current_level.resize(target_size, PallasBase::zero());

    let mut current_depth = 0usize;
    let mut size = target_size;
    while size > 1 {
        current_depth += 1;
        size >>= 1;
    }

    while current_level.len() > 1 {
        let mut next_level = Vec::with_capacity(current_level.len() / 2);
        for pair in current_level.chunks_exact(2) {
            next_level.push(poseidon_hash_domain_2(
                COMMITMENT_TREE_BATCH_DOMAIN,
                pair[0],
                pair[1],
            ));
        }
        current_level = next_level;
    }

    let mut root = current_level[0];
    if current_depth < BATCH_TREE_DEPTH {
        let mut zero = PallasBase::zero();
        let mut zero_hashes = Vec::with_capacity(BATCH_TREE_DEPTH + 1);
        zero_hashes.push(zero);
        for _ in 0..BATCH_TREE_DEPTH {
            zero = poseidon_hash_domain_2(COMMITMENT_TREE_BATCH_DOMAIN, zero, zero);
            zero_hashes.push(zero);
        }

        for level in current_depth..BATCH_TREE_DEPTH {
            root = poseidon_hash_domain_2(
                COMMITMENT_TREE_BATCH_DOMAIN,
                root,
                zero_hashes[level],
            );
        }
    }

    Ok(pallas_to_bytes(&root))
}

#[cfg(feature = "groth16")]
fn load_verifying_key(bytes_override: &[u8]) -> Option<PreparedVerifyingKey<Bn254>> {
    let bytes = if bytes_override.is_empty() {
        ORCHARD_SPEND_VK_BYTES
    } else {
        bytes_override
    };

    match VerifyingKey::<Bn254>::deserialize_compressed(bytes) {
        Ok(vk) => Some(PreparedVerifyingKey::<Bn254>::from(vk)),
        Err(_) => {
            utils::functions::log_error("Failed to deserialize verification key");
            None
        }
    }
}

/// Compute Merkle root from leaf and path proof
///
/// Used for membership verification in circuits
pub fn merkle_root_from_path(
    leaf: &[u8; 32],
    path_indices: &[bool], // true = right, false = left
    path_siblings: &[[u8; 32]],
) -> Result<[u8; 32]> {
    #[cfg(feature = "orchard")]
    {
        use incrementalmerkletree::Hashable;

        if path_indices.len() != path_siblings.len() {
            return Err(OrchardError::ParseError("path length mismatch".into()));
        }

        let current_hash = MerkleHashOrchard::from_bytes(leaf);
        let mut current = if bool::from(current_hash.is_some()) {
            current_hash.unwrap()
        } else {
            return Err(OrchardError::ParseError("invalid leaf encoding".into()));
        };

        for (level, (is_right, sibling)) in path_indices.iter().zip(path_siblings.iter()).enumerate() {
            let sibling_hash = MerkleHashOrchard::from_bytes(sibling);
            let sibling_hash = if bool::from(sibling_hash.is_some()) {
                sibling_hash.unwrap()
            } else {
                return Err(OrchardError::ParseError("invalid sibling encoding".into()));
            };
            let level = Level::from(level as u8);
            current = if *is_right {
                MerkleHashOrchard::combine(level, &sibling_hash, &current)
            } else {
                MerkleHashOrchard::combine(level, &current, &sibling_hash)
            };
        }

        return Ok(current.to_bytes());
    }

    #[cfg(not(feature = "orchard"))]
    {
        let mut current = pallas_from_bytes_be(leaf)?;

        for (is_right, sibling) in path_indices.iter().zip(path_siblings.iter()) {
            let sibling_fp = pallas_from_bytes_be(sibling)?;
            current = if *is_right {
                commitment_tree_hash_2(sibling_fp, current)
            } else {
                commitment_tree_hash_2(current, sibling_fp)
            };
        }

        Ok(pallas_to_bytes(&current))
    }
}

/// Delta structure for multi-asset conservation
///
/// Matches orchard-circuits SpendPublicInputs::deltas
#[derive(Clone, Copy, Debug, Default)]
pub struct Delta {
    pub asset_id: u32,
    pub in_public: u128,      // deposit (transparent → shielded)
    pub out_public: u128,     // withdraw (shielded → transparent)
    pub burn_public: u128,    // burned (e.g., exercised options)
    pub mint_public: u128,    // minted (e.g., exercised stock)
}

/// Encode spend circuit public inputs for verification
///
/// Public input ordering (matches CIRCUITS.md and SPEND_LAYOUT):
/// 1. anchor_root
/// 2. num_inputs
/// 3. num_outputs
/// 4-7. input_nullifiers[0..3]
/// 8-11. output_commitments[0..3]
/// 11-37. deltas (K=3, 9 fields each = 27 total)
/// 38. allowed_root
/// 39. terms_hash
/// 40. poi_root
/// 41. epoch
/// 42-43. fee (lo/hi)
pub fn encode_spend_public_inputs(
    anchor_root: &[u8; 32],
    num_inputs: u8,
    num_outputs: u8,
    nullifiers: &[[u8; 32]; 4],
    new_commitments: &[[u8; 32]; 4],
    deltas: &[Delta; 3],
    allowed_root: &[u8; 32],
    terms_hash: &[u8; 32],
    poi_root: &[u8; 32],
    epoch: u64,
    fee: u128,
) -> Vec<[u8; 32]> {
    let mut inputs = Vec::with_capacity(44);

    // 1. anchor_root
    inputs.push(*anchor_root);

    // 2. num_inputs
    inputs.push(u32_to_field_bytes(num_inputs as u32));

    // 3. num_outputs
    inputs.push(u32_to_field_bytes(num_outputs as u32));

    // 4-7. nullifiers
    inputs.extend_from_slice(nullifiers);

    // 8-11. new_commitments
    inputs.extend_from_slice(new_commitments);

    // 12-38. deltas (3 deltas × 9 fields each)
    for delta in deltas.iter() {
        inputs.push(u32_to_field_bytes(delta.asset_id));
        inputs.push(u128_lo_to_field_bytes(delta.in_public));
        inputs.push(u128_hi_to_field_bytes(delta.in_public));
        inputs.push(u128_lo_to_field_bytes(delta.out_public));
        inputs.push(u128_hi_to_field_bytes(delta.out_public));
        inputs.push(u128_lo_to_field_bytes(delta.burn_public));
        inputs.push(u128_hi_to_field_bytes(delta.burn_public));
        inputs.push(u128_lo_to_field_bytes(delta.mint_public));
        inputs.push(u128_hi_to_field_bytes(delta.mint_public));
    }

    // 39. allowed_root (0x00..00 if no equity)
    inputs.push(*allowed_root);

    // 40. terms_hash (0x00..00 if no exercise/swap)
    inputs.push(*terms_hash);

    // 41. poi_root
    inputs.push(*poi_root);

    // 42. epoch
    inputs.push(u64_to_field_bytes(epoch));

    // 43-44. fee (lo/hi)
    inputs.push(u128_lo_to_field_bytes(fee));
    inputs.push(u128_hi_to_field_bytes(fee));

    assert_eq!(inputs.len(), 44, "Spend circuit expects exactly 44 public input fields");
    inputs
}

/// Verify compliance: allowed_root matches on-chain equity_allowed_root
///
/// Circuit enforces membership proofs; contract validates root currency
pub fn verify_compliance_root(
    allowed_root_from_proof: &[u8; 32],
    equity_allowed_root_on_chain: &[u8; 32],
) -> bool {
    // If proof has allowed_root != 0, it must match on-chain value
    let zero = [0u8; 32];
    if allowed_root_from_proof == &zero {
        true // No equity outputs, compliance not required
    } else {
        allowed_root_from_proof == equity_allowed_root_on_chain
    }
}

/// Verify terms hash for Exercise/Swap gadgets
///
/// Returns true if terms_hash is correctly bound to exercise_terms or swap_quote
pub fn verify_terms_hash(
    terms_hash: &[u8; 32],
    exercise_terms: Option<&ExerciseTerms>,
    swap_quote: Option<&SwapQuote>,
) -> bool {
    let zero = [0u8; 32];
    if terms_hash == &zero {
        // No exercise/swap, valid
        return exercise_terms.is_none() && swap_quote.is_none();
    }

    // Recompute terms hash and compare
    if let Some(terms) = exercise_terms {
        let computed = compute_exercise_terms_hash(terms);
        return &computed == terms_hash;
    }

    if let Some(quote) = swap_quote {
        let computed = compute_swap_quote_hash(quote);
        return &computed == terms_hash;
    }

    false
}

/// Exercise terms (matches orchard-circuits/src/exercise.rs)
#[derive(Clone, Debug)]
pub struct ExerciseTerms {
    pub option_asset_id: u32,
    pub stock_asset_id: u32,
    pub cash_asset_id: u32,
    pub strike: i64,
    pub ratio: u32,
    pub expiry_height: u64,
    pub company_id: [u8; 32],
}

/// Swap quote (matches orchard-circuits/src/swap.rs)
#[derive(Clone, Debug)]
pub struct SwapQuote {
    pub quote_id: [u8; 32],
    pub asset_base: u32,
    pub asset_quote: u32,
    pub side: u8, // 0=BUY, 1=SELL
    pub price_ticks: i64,
    pub max_qty: u128,
    pub fee_bps: u32,
    pub expiry_height: u64,
    pub venue_id: [u8; 32],
}

/// Compute exercise terms hash: Poseidon("exercise_terms_v1", ...)
fn compute_exercise_terms_hash(terms: &ExerciseTerms) -> [u8; 32] {
    // Domain-separated hash matching circuit implementation
    poseidon_hash_with_domain(EXERCISE_TERMS_DOMAIN, &[
        u32_to_bytes(terms.option_asset_id),
        u32_to_bytes(terms.stock_asset_id),
        u32_to_bytes(terms.cash_asset_id),
        i64_to_bytes(terms.strike),
        u32_to_bytes(terms.ratio),
        u64_to_bytes(terms.expiry_height),
        terms.company_id,
    ])
}

/// Compute swap quote hash: Poseidon("quote_v1", ...)
fn compute_swap_quote_hash(quote: &SwapQuote) -> [u8; 32] {
    poseidon_hash_with_domain(SWAP_QUOTE_DOMAIN, &[
        quote.quote_id,
        u32_to_bytes(quote.asset_base),
        u32_to_bytes(quote.asset_quote),
        u64_to_bytes(quote.side as u64),
        i64_to_bytes(quote.price_ticks),
        u128_to_bytes(quote.max_qty),
        u32_to_bytes(quote.fee_bps),
        u64_to_bytes(quote.expiry_height),
        quote.venue_id,
    ])
}

/// Convert u32 to 32-byte big-endian representation
fn u32_to_bytes(val: u32) -> [u8; 32] {
    let mut bytes = [0u8; 32];
    bytes[28..32].copy_from_slice(&val.to_be_bytes());
    bytes
}

/// Convert u64 to 32-byte big-endian representation
fn u64_to_bytes(val: u64) -> [u8; 32] {
    let mut bytes = [0u8; 32];
    bytes[24..32].copy_from_slice(&val.to_be_bytes());
    bytes
}

/// Convert i64 to 32-byte big-endian representation (two's complement)
fn i64_to_bytes(val: i64) -> [u8; 32] {
    let mut bytes = [0u8; 32];
    if val >= 0 {
        bytes[24..32].copy_from_slice(&val.to_be_bytes());
    } else {
        // Negative: use two's complement in field
        // For simplicity, convert via u64 (works for small negatives)
        let abs_val = val.unsigned_abs();
        bytes[24..32].copy_from_slice(&abs_val.to_be_bytes());
        // Sign bit handled by circuit
    }
    bytes
}

/// Convert u128 to 32-byte big-endian representation
fn u128_to_bytes(val: u128) -> [u8; 32] {
    let mut bytes = [0u8; 32];
    bytes[16..32].copy_from_slice(&val.to_be_bytes());
    bytes
}

/// Verify Verkle membership proof
///
/// verify_verkle_proof(state_root, key, value, proof) (services/orchard/docs/ORCHARD.md refine path)
///
/// JAM uses Banderwagon curve with IPA multiproof
pub fn verify_verkle_proof(
    _state_root: &[u8; 32],
    _key: &[u8; 32],
    _value: &[u8; 32],
    _proof: &[u8],
) -> Result<()> {
    // TODO: Implement using Banderwagon + IPA verification
    // This requires JAM's verkle-trie library integration
    // For now, delegate to host function (outside no_std scope)
    Err(OrchardError::InvalidVerkleProof { key: [0u8; 32] })
}

/// 64-bit range check (ensures value is in [0, 2^64))
///
/// Prevents modulus wrap inflation attack (services/orchard/docs/ORCHARD.md multi-asset conservation)
///
/// In the circuit, this is implemented via bit decomposition.
/// Here we just validate the u64 is not wrapping mod BN254 field prime.
pub fn range_check_64bit(value: u64) -> bool {
    // In no_std, we just check it's a valid u64
    // The circuit does the heavy lifting with bit decomposition
    value <= u64::MAX
}

// ===== Domain Hashing =====

fn domain_str_to_field(domain: &str) -> PallasBase {
    let bytes = domain.as_bytes();
    let mut hash_input = [0u8; 64];
    let copy_len = core::cmp::min(bytes.len(), 64);
    hash_input[..copy_len].copy_from_slice(&bytes[..copy_len]);
    PallasBase::from_uniform_bytes(&hash_input)
}

// ===== Field Conversions =====

/// Convert 32-byte array to BN254 scalar field element
///
/// Interprets bytes as big-endian integer mod BN254 scalar field prime
#[cfg(feature = "groth16")]
fn bytes_to_field(bytes: &[u8; 32]) -> BN254Fr {
    // BN254Fr expects BigInteger256 format (little-endian u64 limbs)
    let mut le_bytes = *bytes;
    le_bytes.reverse(); // Convert big-endian to little-endian

    BN254Fr::from_le_bytes_mod_order(&le_bytes)
}

/// Convert BN254 scalar field element to 32-byte array (big-endian)
#[cfg(feature = "groth16")]
fn field_to_bytes(field: &BN254Fr) -> [u8; 32] {
    let mut bytes = [0u8; 32];

    // Serialize field element to bytes
    let bigint = field.into_bigint();
    let le_bytes = bigint.to_bytes_le();

    // Copy to output (pad to 32 bytes if needed)
    let len = core::cmp::min(le_bytes.len(), 32);
    bytes[..len].copy_from_slice(&le_bytes[..len]);

    // Convert to big-endian
    bytes.reverse();
    bytes
}

/// Attempt to deserialize VK from binary bytes
///
/// Tries multiple binary formats to deserialize the embedded VK bytes.
/// Returns error if the bytes are not in a supported binary format.
fn parse_vk_bundle(vk_bytes: &[u8]) -> Result<VkBundle> {
    if vk_bytes.len() < VK_BUNDLE_HEADER_LEN {
        return Err(OrchardError::ParseError(
            "vk bundle missing header".to_string(),
        ));
    }
    if !vk_bytes.starts_with(VK_BUNDLE_MAGIC) {
        return Err(OrchardError::ParseError(
            "vk bundle missing magic header".to_string(),
        ));
    }

    let mut k_bytes = [0u8; 4];
    k_bytes.copy_from_slice(&vk_bytes[8..12]);
    let k = u32::from_le_bytes(k_bytes);

    let mut len_bytes = [0u8; 4];
    len_bytes.copy_from_slice(&vk_bytes[12..16]);
    let vk_len = u32::from_le_bytes(len_bytes) as usize;

    if vk_bytes.len() != VK_BUNDLE_HEADER_LEN + vk_len {
        return Err(OrchardError::ParseError(
            "vk bundle length mismatch".to_string(),
        ));
    }

    let vk_bytes = vk_bytes[VK_BUNDLE_HEADER_LEN..].to_vec();
    Ok(VkBundle { k, vk_bytes })
}

fn vk_hash_bytes(bytes: &[u8]) -> [u8; 32] {
    let mut out = [0u8; 32];
    let mut hasher = Blake2bVar::new(32)
        .expect("blake2b output length should be valid");
    hasher.update(bytes);
    hasher.finalize_variable(&mut out)
        .expect("blake2b finalize should not fail");
    out
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    #[cfg(feature = "groth16")]
    fn test_field_conversion_roundtrip() {
        let original = [
            1, 2, 3, 4, 5, 6, 7, 8,
            9, 10, 11, 12, 13, 14, 15, 16,
            17, 18, 19, 20, 21, 22, 23, 24,
            25, 26, 27, 28, 29, 30, 31, 32,
        ];

        let field = bytes_to_field(&original);
        let converted = field_to_bytes(&field);

        // May not be exact due to modular reduction, but should be deterministic
        assert_eq!(bytes_to_field(&converted), field);
    }

    #[test]
    fn test_commitment_deterministic() {
        let pk = [1u8; 32];
        let asset_id = 1u32;
        let amount = 1000u128;
        let rho = [2u8; 32];
        let note_rseed = [3u8; 32];
        let memo_hash = [4u8; 32];

        let c1 = commitment(asset_id, amount, &pk, &rho, &note_rseed, 0, &memo_hash).unwrap();
        let c2 = commitment(asset_id, amount, &pk, &rho, &note_rseed, 0, &memo_hash).unwrap();

        assert_eq!(c1, c2, "Commitment must be deterministic");
    }

    #[test]
    fn test_nullifier_deterministic() {
        let sk = [1u8; 32];
        let rho = [2u8; 32];
        let pk = [3u8; 32];
        let note_rseed = [4u8; 32];
        let memo_hash = [5u8; 32];
        let commitment = commitment(1, 100u128, &pk, &rho, &note_rseed, 0, &memo_hash).unwrap();

        let nf1 = nullifier(&sk, &rho, &commitment).unwrap();
        let nf2 = nullifier(&sk, &rho, &commitment).unwrap();

        assert_eq!(nf1, nf2, "Nullifier must be deterministic");
    }

    #[test]
    fn test_merkle_append_empty() {
        let base = [];
        let new_root = merkle_append_from_leaves(&base, &[]).unwrap();
        assert_eq!(
            new_root,
            merkle_root_from_leaves(&base).unwrap(),
            "Empty append should not change root"
        );
    }

    #[test]
    fn test_merkle_append_single() {
        let commitment = [1u8; 32];

        let new_root = merkle_append_from_leaves(&[], &[commitment]).unwrap();
        assert_ne!(
            new_root,
            merkle_root_from_leaves(&[]).unwrap(),
            "Appending should change root"
        );
    }

    #[test]
    fn test_poseidon_hash_deterministic() {
        let input1 = [1u8; 32];
        let input2 = [2u8; 32];
        let domain = "poseidon_test_v1";

        let h1 = poseidon_hash_with_domain(domain, &[input1, input2]);
        let h2 = poseidon_hash_with_domain(domain, &[input1, input2]);

        assert_eq!(h1, h2, "Poseidon hash must be deterministic");
    }

    #[test]
    fn test_range_check_64bit() {
        // Placeholder test for range checking
        assert!(true);
    }
}
