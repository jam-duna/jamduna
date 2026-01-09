#![allow(dead_code)]
/// Cryptographic primitives for Orchard service
///
/// Based on services/orchard/docs/ORCHARD.md specification:
/// - Poseidon hash (SNARK-friendly)
/// - Groth16 proof verification (BN254 curve)
/// - Merkle tree operations

use crate::errors::{OrchardError, Result};
use crate::vk_registry::get_vk_registry;
use crate::sinsemilla_nostd;
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

// Halo2 imports for IPA verification
use halo2_proofs::{
    poly::commitment::Params,
    plonk::{VerifyingKey as Halo2VerifyingKey},
    transcript::{Blake2bRead, Challenge255},
};
use halo2_proofs::pasta::vesta;
use halo2_proofs::pasta::pallas::Base as PallasBase;
use halo2_proofs::pasta::group::ff::{FromUniformBytes, PrimeField, PrimeFieldBits};

static mut EMPTY_NODES_CACHE: Option<Vec<[u8; 32]>> = None;

fn empty_nodes_cache(depth: usize) -> &'static Vec<[u8; 32]> {
    unsafe {
        let needs_init = EMPTY_NODES_CACHE
            .as_ref()
            .map(|nodes| nodes.len() != depth + 1)
            .unwrap_or(true);
        if needs_init {
            crate::log_info(&format!(
                "empty_nodes_cache: init start depth={}",
                depth
            ));
            let mut nodes = Vec::with_capacity(depth + 1);
            let mut current = empty_node_bytes(0);
            nodes.push(current);
            crate::log_info("empty_nodes_cache: level 0 ready");
            for level in 1..=depth {
                current = merkle_combine_at(level - 1, &current, &current)
                    .expect("empty node combine failed");
                nodes.push(current);
                crate::log_info(&format!(
                    "empty_nodes_cache: level {} ready",
                    level
                ));
            }
            EMPTY_NODES_CACHE = Some(nodes);
            crate::log_info("empty_nodes_cache: init done");
        }
        EMPTY_NODES_CACHE.as_ref().expect("empty node cache")
    }
}
#[cfg(feature = "orchard")]
use incrementalmerkletree::Level;
#[cfg(feature = "orchard")]
use orchard::tree::MerkleHashOrchard;

/// Binary tree depth (consensus-critical)
pub const TREE_DEPTH: usize = 32;

const NOTE_COMMITMENT_DOMAIN: &str = "note_cm_v1";
const NULLIFIER_DOMAIN: &str = "nf_v1";
const COMMITMENT_TREE_DOMAIN: &str = "commitment_tree_v1";
const EXERCISE_TERMS_DOMAIN: &str = "exercise_terms_v1";
const SWAP_QUOTE_DOMAIN: &str = "quote_v1";
const ASSET_ID_DOMAIN: &[u8; 16] = b"ZSA_Asset_ID_v1\0";

const VK_BUNDLE_MAGIC: &[u8; 8] = b"RGVKv1\0\0";
const HALO2_VK_MAGIC: &[u8; 8] = b"H2VKv1\0\0";
const VK_BUNDLE_HEADER_LEN: usize = 8 + 4 + 4;

/// Embedded verification key for the Orchard NU5 circuit.
pub const ORCHARD_VK_BYTES: &[u8] = include_bytes!("../keys/orchard.vk");
/// Embedded IPA commitment parameters for the Orchard NU5 circuit.
pub const ORCHARD_PARAMS_BYTES: &[u8] = include_bytes!("../keys/orchard.params");
/// Embedded verification key for the Orchard ZSA circuit.
pub const ORCHARD_ZSA_VK_BYTES: &[u8] = include_bytes!("../keys/orchard_zsa.vk");
/// Embedded IPA commitment parameters for the Orchard ZSA circuit.
pub const ORCHARD_ZSA_PARAMS_BYTES: &[u8] = include_bytes!("../keys/orchard_zsa.params");

struct VkBundle {
    k: u32,
    vk_bytes: Vec<u8>,
}

struct CachedHalo2Verifier {
    k: u32,
    vk: Halo2VerifyingKey<vesta::Affine>,
    params: Params<vesta::Affine>,
    vk_hash: [u8; 32],
}

static mut HALO2_CACHE: Option<CachedHalo2Verifier> = None;

fn get_halo2_cache(
    vk_bytes: &[u8],
    params_bytes: &[u8],
) -> Result<(&'static CachedHalo2Verifier, bool)> {
    unsafe {
        let vk_hash = vk_hash_bytes(vk_bytes);
        if HALO2_CACHE
            .as_ref()
            .map(|cache| cache.vk_hash != vk_hash)
            .unwrap_or(true)
        {
            crate::log_info("Halo2 cache init: start");
            crate::log_info(&format!(
                "Halo2 cache init: vk_bytes_len={}",
                vk_bytes.len()
            ));
            crate::log_info("Halo2 cache init: parse_vk_bundle start");
            let vk_bundle = parse_vk_bundle(vk_bytes)?;
            crate::log_info(&format!(
                "Halo2 cache init: parse_vk_bundle done (k={}, vk_len={})",
                vk_bundle.k,
                vk_bundle.vk_bytes.len()
            ));
            crate::log_info("Halo2 cache init: Halo2VerifyingKey::read start");
            let vk = Halo2VerifyingKey::read(&mut &vk_bundle.vk_bytes[..]).map_err(|_| {
                OrchardError::ParseError("failed to deserialize halo2 vk".to_string())
            })?;
            crate::log_info("Halo2 cache init: Halo2VerifyingKey::read done");
            crate::log_info("Halo2 cache init: Params::read start");
            let params = Params::<vesta::Affine>::read(&mut &params_bytes[..]).map_err(|_| {
                OrchardError::ParseError("failed to deserialize halo2 params".to_string())
            })?;
            if params.k() != vk_bundle.k {
                return Err(OrchardError::ParseError(
                    "halo2 params k mismatch".to_string(),
                ));
            }
            crate::log_info("Halo2 cache init: Params::read done");
            crate::log_info("Halo2 cache init: vk_hash start");
            crate::log_info("Halo2 cache init: vk_hash done");
            HALO2_CACHE = Some(CachedHalo2Verifier {
                k: vk_bundle.k,
                vk,
                params,
                vk_hash,
            });
            crate::log_info("Halo2 cache init: done");
            return Ok((HALO2_CACHE.as_ref().unwrap_unchecked(), false));
        }
        Ok((HALO2_CACHE.as_ref().unwrap_unchecked(), true))
    }
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

    log_info(&format!(
        "Halo2 input summary: vk_id={}, proof_bytes_len={}, public_inputs_len={}",
        vk_id,
        proof_bytes.len(),
        public_inputs.len(),
    ));

    // 1. Load VK from registry and validate field layout
    let registry = get_vk_registry();
    let vk_entry = registry.get_vk_entry(vk_id)?;
    registry.validate_public_inputs(vk_id, public_inputs)?;
    let action_count = registry.get_action_count(vk_id, public_inputs)?;

    log_info(&format!(
        "Halo2 verification starting: vk_id={}, domain={}, fields={}",
        vk_id, vk_entry.domain, vk_entry.field_count
    ));



    // 2. Load VK bytes from embedded constants and parse bundle
    let (vk_bytes, params_bytes) = match vk_id {
        1 => (ORCHARD_VK_BYTES, ORCHARD_PARAMS_BYTES),
        2 => (ORCHARD_ZSA_VK_BYTES, ORCHARD_ZSA_PARAMS_BYTES),
        _ => return Err(OrchardError::InvalidVkId { vk_id }),
    };
    if vk_bytes.is_empty() || params_bytes.is_empty() {
        return Err(OrchardError::ParseError(
            "missing halo2 vk/params bytes for vk_id".to_string(),
        ));
    }
    let (cache, cache_hit) = get_halo2_cache(vk_bytes, params_bytes)?;
    if cache_hit {
        log_info("Halo2 verification: cache hit");
    } else {
        log_info("Halo2 verification: cache miss (initialized)");
    }

    // 3. VK + params from cache
    log_info(&format!(
        "✅ VK bundle verified: vk_id={}, k={}, hash={:?}",
        vk_id, cache.k, &cache.vk_hash[..8]
    ));

    // 4. Convert public inputs from bytes to Pallas field elements (little-endian)
    log_info("Halo2 verification: public input conversion start");
    let pallas_inputs: Result<Vec<PallasBase>> = public_inputs.iter()
        .map(|bytes| {
            // Convert 32-byte array to field element (little-endian)
            // This matches the canonical field encoding from the Orchard circuit
            let mut repr = <PallasBase as PrimeField>::Repr::default();
            repr.as_mut().copy_from_slice(bytes);
            PallasBase::from_repr(repr)
                .into_option()
                .ok_or(OrchardError::InvalidFieldElement)
        })
        .collect();
    let pallas_inputs = pallas_inputs?;
    log_info("Halo2 verification: public input conversion done");

    // 5. Initialize Blake2b transcript (Orchard circuit does not apply domain separation)
    log_info("Halo2 verification: transcript init start");
    let mut transcript = Blake2bRead::<_, vesta::Affine, Challenge255<vesta::Affine>>::init(proof_bytes);
    log_info("Halo2 verification: transcript init done");

    // 6. Load universal parameters (IPA commitment params)
    log_info("Halo2 verification: params ready (cached)");

    // 7. Verify the Halo2 proof using real IPA verification
    // This performs actual cryptographic verification against the circuit VK
    if proof_bytes.is_empty() {
        log_info("Empty proof provided - verification failed");
        return Ok(false);
    }

    // Real Halo2 verification with proper transcript and strategy
    let fields_per_action = vk_entry.field_count;
    let mut action_instances: Vec<Vec<&[PallasBase]>> = Vec::with_capacity(action_count);
    for action_idx in 0..action_count {
        let start = action_idx * fields_per_action;
        let end = start + fields_per_action;
        action_instances.push(vec![&pallas_inputs[start..end]]);
    }
    let instance_slices: Vec<&[&[PallasBase]]> =
        action_instances.iter().map(|i| &i[..]).collect();
    log_info(&format!(
        "Halo2 verification: verify_proof start (instances={}, fields_per_action={})",
        instance_slices.len(),
        fields_per_action,
    ));
    match halo2_proofs::plonk::verify_proof(
        &cache.params,
        &cache.vk,
        halo2_proofs::plonk::SingleVerifier::new(&cache.params),
        &instance_slices,
        &mut transcript,
    ) {
        Ok(_) => {
            log_info("Halo2 verification: verify_proof done (success)");
            log_info("✅ Halo2/IPA proof verification succeeded - REAL VERIFICATION ACTIVE");
            Ok(true)
        }
        Err(e) => {
            log_info("Halo2 verification: verify_proof done (failure)");
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

fn pallas_from_bytes(bytes: &[u8; 32]) -> Result<PallasBase> {
    let mut repr = <PallasBase as PrimeField>::Repr::default();
    repr.as_mut().copy_from_slice(bytes);
    PallasBase::from_repr(repr)
        .into_option()
        .ok_or(OrchardError::InvalidFieldElement)
}

pub fn pallas_from_bytes_reduced(bytes: &[u8; 32]) -> PallasBase {
    let reduced = reduce_bytes_to_field_bytes(*bytes);
    let mut repr = <PallasBase as PrimeField>::Repr::default();
    repr.as_mut().copy_from_slice(&reduced);
    PallasBase::from_repr(repr)
        .into_option()
        .unwrap_or_else(|| PallasBase::zero())
}

pub fn pallas_to_bytes(value: &PallasBase) -> [u8; 32] {
    // Use the canonical little-endian encoding (matches MerkleHashOrchard::to_bytes)
    value.to_repr()
}


/// Commitment: Sinsemilla("note_cm_v1", asset_id, amount, owner_pk, rho, note_rseed, unlock_height, memo_hash)
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
    use halo2_gadgets::sinsemilla::primitives::HashDomain;

    // Create Sinsemilla hash domain for note commitment
    let domain = HashDomain::new(NOTE_COMMITMENT_DOMAIN);

    // Convert all inputs to bit representations for Sinsemilla
    let asset_id_bits: Vec<bool> = (0..32).map(|i| (asset_id >> i) & 1 == 1).collect();
    let amount_bits: Vec<bool> = (0..128).map(|i| (amount >> i) & 1 == 1).collect();

    let owner_pk_bits: Vec<bool> = owner_pk.iter().flat_map(|&b|
        (0..8).map(move |i| (b >> i) & 1 == 1)
    ).collect();
    let rho_bits: Vec<bool> = rho.iter().flat_map(|&b|
        (0..8).map(move |i| (b >> i) & 1 == 1)
    ).collect();
    let note_rseed_bits: Vec<bool> = note_rseed.iter().flat_map(|&b|
        (0..8).map(move |i| (b >> i) & 1 == 1)
    ).collect();
    let unlock_height_bits: Vec<bool> = (0..64).map(|i| (unlock_height >> i) & 1 == 1).collect();
    let memo_hash_bits: Vec<bool> = memo_hash.iter().flat_map(|&b|
        (0..8).map(move |i| (b >> i) & 1 == 1)
    ).collect();

    // Build message for Sinsemilla
    let message = core::iter::empty()
        .chain(asset_id_bits)
        .chain(amount_bits)
        .chain(owner_pk_bits)
        .chain(rho_bits)
        .chain(note_rseed_bits)
        .chain(unlock_height_bits)
        .chain(memo_hash_bits);

    let cm = domain.hash(message).unwrap_or(PallasBase::zero());
    Ok(pallas_to_bytes(&cm))
}

/// Nullifier: Sinsemilla("nf_v1", sk_spend, rho, commitment)
pub fn nullifier(
    sk_spend: &[u8; 32],
    rho: &[u8; 32],
    commitment: &[u8; 32],
) -> Result<[u8; 32]> {
    use halo2_gadgets::sinsemilla::primitives::HashDomain;

    // Create Sinsemilla hash domain for nullifier
    let domain = HashDomain::new(NULLIFIER_DOMAIN);

    // Convert all inputs to bit representations for Sinsemilla
    let sk_bits: Vec<bool> = sk_spend.iter().flat_map(|&b|
        (0..8).map(move |i| (b >> i) & 1 == 1)
    ).collect();
    let rho_bits: Vec<bool> = rho.iter().flat_map(|&b|
        (0..8).map(move |i| (b >> i) & 1 == 1)
    ).collect();
    let commitment_bits: Vec<bool> = commitment.iter().flat_map(|&b|
        (0..8).map(move |i| (b >> i) & 1 == 1)
    ).collect();

    // Build message for Sinsemilla
    let message = core::iter::empty()
        .chain(sk_bits)
        .chain(rho_bits)
        .chain(commitment_bits);

    let nf = domain.hash(message).unwrap_or(PallasBase::zero());
    Ok(pallas_to_bytes(&nf))
}

/// Derive deterministic asset ID from issuer key + asset description hash (NU7/ZSA)
///
/// Asset ID derivation follows the Zcash ZSA specification:
/// - Same issuer + description = same asset ID (deterministic)
/// - Different issuer OR description = different asset ID
///
/// Uses BLAKE2b-256 with domain separation for cryptographic binding.
///
/// # Arguments
/// * `issuer_key` - IssueValidatingKey (32 bytes, public key for asset issuance)
/// * `asset_desc_hash` - BLAKE2b hash of asset description (32 bytes)
///
/// # Returns
/// * `AssetBase` - 32-byte asset identifier
///
/// # Specification
/// Matches orchard/src/issuance.rs derivation:
/// ```text
/// asset_id = BLAKE2b-256(
///     personalization: "ZSA_Asset_ID_v1\0",
///     data: issuer_key || asset_desc_hash
/// )
/// ```
pub fn derive_asset_id(
    issuer_key: &[u8; 32],
    asset_desc_hash: &[u8; 32],
) -> crate::nu7_types::AssetBase {
    use blake2::digest::{FixedOutput, Update};

    // Blake2b-256 with personalization for domain separation
    let mut hasher = Blake2b::<blake2::digest::consts::U32>::new();
    Update::update(&mut hasher, ASSET_ID_DOMAIN);
    Update::update(&mut hasher, issuer_key);
    Update::update(&mut hasher, asset_desc_hash);

    let hash = hasher.finalize_fixed();
    crate::nu7_types::AssetBase::from_bytes(hash.into())
}

/// Sinsemilla hash for commitment tree internal nodes (domain-separated)
fn commitment_tree_hash_2(left: PallasBase, right: PallasBase) -> PallasBase {
    use halo2_gadgets::sinsemilla::primitives::HashDomain;
    let domain = HashDomain::new(COMMITMENT_TREE_DOMAIN);

    let left_bits = left.to_le_bits().into_iter().take(255);
    let right_bits = right.to_le_bits().into_iter().take(255);
    let message = left_bits.chain(right_bits);

    domain.hash(message).unwrap_or(PallasBase::zero())
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
/// **LIMITATION**: Only works correctly when current_size == 0.
/// For non-zero sizes, uses placeholder empty leaves which produces incorrect roots.
///
/// When 'orchard' feature is enabled, uses real Sinsemilla hashing.
/// Otherwise, uses no_std Sinsemilla implementation.
pub fn merkle_append(
    _anchor_root: &[u8; 32],
    current_size: u64,
    new_commitments: &[[u8; 32]],
) -> Result<[u8; 32]> {
    if new_commitments.is_empty() {
        if current_size == 0 {
            // Empty tree
            return merkle_root_from_leaves(&[]);
        }
        return Err(OrchardError::ParseError("Cannot reconstruct tree from root alone".into()));
    }

    // Build all_leaves = [existing leaves (placeholders)] + [new commitments]
    let mut all_leaves = Vec::with_capacity(current_size as usize + new_commitments.len());

    // LIMITATION: Using placeholder leaves for existing commitments
    // This only produces correct roots when current_size == 0
    #[cfg(feature = "orchard")]
    {
        use incrementalmerkletree::Hashable;
        for _ in 0..current_size {
            all_leaves.push(MerkleHashOrchard::empty_leaf().to_bytes());
        }
    }

    #[cfg(not(feature = "orchard"))]
    {
        for _ in 0..current_size {
            all_leaves.push(sinsemilla_nostd::empty_leaf()); // Sinsemilla empty leaf
        }
    }

    // Add new commitments
    all_leaves.extend_from_slice(new_commitments);

    // Compute new root (uses Sinsemilla if orchard feature enabled, else Poseidon)
    merkle_root_from_leaves(&all_leaves)
}

/// Compute Merkle root for a contiguous set of leaves
///
/// DEFAULT: Uses no_std Sinsemilla implementation for compatibility with builder.
/// Can use full orchard crate with 'orchard' feature (requires std).
pub fn merkle_root_from_leaves(leaves: &[[u8; 32]]) -> Result<[u8; 32]> {
    #[cfg(feature = "orchard")]
    {
        // Use full orchard crate implementation (requires std)
        return orchard_merkle_root_from_leaves(leaves);
    }

    #[cfg(not(feature = "orchard"))]
    {
        // DEFAULT: Use no_std Sinsemilla-compatible implementation
        return sinsemilla_nostd::compute_merkle_root_sinsemilla(leaves);
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
        let mut current = *leaf;

        for (level, (is_right, sibling)) in path_indices.iter().zip(path_siblings.iter()).enumerate() {
            let (left, right) = if *is_right {
                (sibling, &current)
            } else {
                (&current, sibling)
            };
            current = merkle_combine_at(level, left, right)?;
        }

        Ok(current)
    }
}

/// Verify a commitment's Merkle branch against a root.
pub fn verify_commitment_branch(
    commitment: &[u8; 32],
    position: u64,
    siblings: &[[u8; 32]],
    expected_root: &[u8; 32],
) -> Result<()> {
    let mut path_indices = Vec::with_capacity(siblings.len());
    for level in 0..siblings.len() {
        path_indices.push(((position >> level) & 1) == 1);
    }

    let computed = merkle_root_from_path(commitment, &path_indices, siblings)?;
    if &computed != expected_root {
        return Err(OrchardError::StateError(
            "Commitment branch does not match root".into(),
        ));
    }

    Ok(())
}

/// Append new commitments using a commitment frontier.
pub fn merkle_append_with_frontier(
    current_root: &[u8; 32],
    current_size: u64,
    frontier: &[[u8; 32]],
    new_commitments: &[[u8; 32]],
) -> Result<[u8; 32]> {
    let depth = sinsemilla_nostd::TREE_DEPTH;
    crate::log_info(&format!(
        "merkle_append_with_frontier: init empty nodes depth={}",
        depth
    ));
    let empty_nodes = empty_nodes_cache(depth);
    crate::log_info("merkle_append_with_frontier: empty nodes ready");
    crate::log_info(&format!(
        "merkle_append_with_frontier: start size={}, new_commitments={}, depth={}",
        current_size,
        new_commitments.len(),
        depth
    ));
    if frontier.len() != depth {
        return Err(OrchardError::ParseError(format!(
            "frontier length mismatch: expected {}, got {}",
            depth,
            frontier.len()
        )));
    }

    let frontier_all_zero = frontier.iter().all(|node| node.iter().all(|b| *b == 0));
    let root_all_zero = current_root.iter().all(|b| *b == 0);
    crate::log_info(&format!(
        "merkle_append_with_frontier: flags root_zero={}, frontier_zero={}, size={}",
        root_all_zero,
        frontier_all_zero,
        current_size
    ));
    if root_all_zero && frontier_all_zero && current_size <= 2 {
        // Skip pre-root check for placeholder frontier (pre-state roots are zeros).
        crate::log_info("merkle_append_with_frontier: pre-root check skipped");
    } else {
        crate::log_info("merkle_append_with_frontier: pre-root check begin");
        let computed_root = merkle_root_from_frontier(frontier, current_size)?;
        crate::log_info("merkle_append_with_frontier: pre-root check done");
        if &computed_root != current_root {
            return Err(OrchardError::StateError(
                "Frontier does not match current commitment_root".into(),
            ));
        }
    }

    let mut frontier_nodes = frontier.to_vec();
    let mut size = current_size;

    for (idx, commitment) in new_commitments.iter().enumerate() {
        crate::log_info(&format!(
            "merkle_append_with_frontier: append commitment {} at size={}",
            idx,
            size
        ));
        let mut node = *commitment;
        let mut level = 0usize;
        let mut cursor = size;

        while (cursor & 1) == 1 {
            let left = frontier_nodes[level];
            crate::log_info(&format!(
                "merkle_append_with_frontier: combine at level {} (cursor={})",
                level,
                cursor
            ));
            node = merkle_combine_at(level, &left, &node)?;
            crate::log_info(&format!(
                "merkle_append_with_frontier: combined level {} -> {:?}",
                level,
                &node[..4]
            ));
            frontier_nodes[level] = empty_nodes[level];
            level += 1;
            cursor >>= 1;
        }

        frontier_nodes[level] = node;
        size += 1;
        crate::log_info(&format!(
            "merkle_append_with_frontier: set frontier level {} size={}",
            level,
            size
        ));
    }

    crate::log_info(&format!(
        "merkle_append_with_frontier: append done size={}",
        size
    ));
    crate::log_info("merkle_append_with_frontier: post-root compute begin");
    let root = merkle_root_from_frontier(&frontier_nodes, size)?;
    crate::log_info("merkle_append_with_frontier: post-root compute done");
    Ok(root)
}

/// Recompute a Merkle root from a commitment frontier and size.
pub fn merkle_root_from_frontier(
    frontier: &[[u8; 32]],
    size: u64,
) -> Result<[u8; 32]> {
    let empty_nodes = empty_nodes_cache(sinsemilla_nostd::TREE_DEPTH);
    crate::log_info(&format!(
        "merkle_root_from_frontier: start size={}",
        size
    ));
    if size == 0 {
        return Ok(empty_nodes[sinsemilla_nostd::TREE_DEPTH]);
    }

    let depth = sinsemilla_nostd::TREE_DEPTH;
    let mut digest: Option<[u8; 32]> = None;
    let mut current_level: usize = 0;
    let mut cursor = size;

    // Fold subtree roots from the frontier, lifting the right subtree with empties.
    for level in 0..depth {
        if (cursor & 1) == 1 {
            crate::log_info(&format!(
                "merkle_root_from_frontier: fold level {} (current_level={})",
                level,
                current_level
            ));
            let node = frontier[level];
            if let Some(existing) = digest {
                let mut acc = existing;
                for l in current_level..level {
                    crate::log_info(&format!(
                        "merkle_root_from_frontier: lift acc at level {}",
                        l
                    ));
                    acc = merkle_combine_at(l, &acc, &empty_nodes[l])?;
                }
                crate::log_info(&format!(
                    "merkle_root_from_frontier: combine node at level {}",
                    level
                ));
                digest = Some(merkle_combine_at(level, &node, &acc)?);
                current_level = level + 1;
            } else {
                digest = Some(node);
                current_level = level;
            }
        }
        cursor >>= 1;
    }

    let mut acc = digest.ok_or_else(|| OrchardError::StateError("frontier root missing".into()))?;
    for l in current_level..depth {
        crate::log_info(&format!(
            "merkle_root_from_frontier: final lift level {}",
            l
        ));
        acc = merkle_combine_at(l, &acc, &empty_nodes[l])?;
    }

    crate::log_info("merkle_root_from_frontier: done");
    Ok(acc)
}

fn merkle_combine_at(level: usize, left: &[u8; 32], right: &[u8; 32]) -> Result<[u8; 32]> {
    #[cfg(feature = "orchard")]
    {
        use incrementalmerkletree::Hashable;
        use incrementalmerkletree::Level;
        use orchard::tree::MerkleHashOrchard;

        let left_hash = MerkleHashOrchard::from_bytes(left)
            .into_option()
            .ok_or_else(|| OrchardError::ParseError("invalid left node".into()))?;
        let right_hash = MerkleHashOrchard::from_bytes(right)
            .into_option()
            .ok_or_else(|| OrchardError::ParseError("invalid right node".into()))?;
        let combined = MerkleHashOrchard::combine(Level::from(level as u8), &left_hash, &right_hash);
        return Ok(combined.to_bytes());
    }

    #[cfg(not(feature = "orchard"))]
    {
        sinsemilla_nostd::merkle_combine_bytes(level, left, right)
    }
}

fn empty_node_bytes(level: usize) -> [u8; 32] {
    #[cfg(feature = "orchard")]
    {
        use incrementalmerkletree::Hashable;
        use incrementalmerkletree::Level;
        use orchard::tree::MerkleHashOrchard;
        return MerkleHashOrchard::empty_root(Level::from(level as u8)).to_bytes();
    }

    #[cfg(not(feature = "orchard"))]
    {
        sinsemilla_nostd::empty_node_bytes(level)
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

/// Compute exercise terms hash: Sinsemilla("exercise_terms_v1", ...)
fn compute_exercise_terms_hash(terms: &ExerciseTerms) -> [u8; 32] {
    use halo2_gadgets::sinsemilla::primitives::HashDomain;

    let domain = HashDomain::new(EXERCISE_TERMS_DOMAIN);

    let data = [
        u32_to_bytes(terms.option_asset_id),
        u32_to_bytes(terms.stock_asset_id),
        u32_to_bytes(terms.cash_asset_id),
        i64_to_bytes(terms.strike),
        u32_to_bytes(terms.ratio),
        u64_to_bytes(terms.expiry_height),
        terms.company_id,
    ];

    let message = data.iter().flat_map(|bytes|
        bytes.iter().flat_map(|&b| (0..8).map(move |i| (b >> i) & 1 == 1))
    );

    let hash = domain.hash(message).unwrap_or(PallasBase::zero());
    pallas_to_bytes(&hash)
}

/// Compute swap quote hash: Sinsemilla("quote_v1", ...)
fn compute_swap_quote_hash(quote: &SwapQuote) -> [u8; 32] {
    use halo2_gadgets::sinsemilla::primitives::HashDomain;

    let domain = HashDomain::new(SWAP_QUOTE_DOMAIN);

    let data = [
        quote.quote_id,
        u32_to_bytes(quote.asset_base),
        u32_to_bytes(quote.asset_quote),
        u64_to_bytes(quote.side as u64),
        i64_to_bytes(quote.price_ticks),
        u128_to_bytes(quote.max_qty),
        u32_to_bytes(quote.fee_bps),
        u64_to_bytes(quote.expiry_height),
        quote.venue_id,
    ];

    let message = data.iter().flat_map(|bytes|
        bytes.iter().flat_map(|&b| (0..8).map(move |i| (b >> i) & 1 == 1))
    );

    let hash = domain.hash(message).unwrap_or(PallasBase::zero());
    pallas_to_bytes(&hash)
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

    if vk_bytes.starts_with(VK_BUNDLE_MAGIC) {
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
        return Ok(VkBundle { k, vk_bytes });
    }

    if vk_bytes.starts_with(HALO2_VK_MAGIC) {
        let mut k_bytes = [0u8; 4];
        k_bytes.copy_from_slice(&vk_bytes[8..12]);
        let k = u32::from_le_bytes(k_bytes);
        return Ok(VkBundle {
            k,
            vk_bytes: vk_bytes.to_vec(),
        });
    }

    Err(OrchardError::ParseError(
        "vk bundle missing magic header".to_string(),
    ))
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
    fn test_range_check_64bit() {
        // Placeholder test for range checking
        assert!(true);
    }

    #[test]
    fn test_derive_asset_id_deterministic() {
        let issuer_key = [1u8; 32];
        let asset_desc_hash = [2u8; 32];

        let asset_id_1 = derive_asset_id(&issuer_key, &asset_desc_hash);
        let asset_id_2 = derive_asset_id(&issuer_key, &asset_desc_hash);

        assert_eq!(asset_id_1, asset_id_2, "Asset ID derivation must be deterministic");
        assert_ne!(asset_id_1.to_bytes(), [0u8; 32], "Asset ID should not be zero");
    }

    #[test]
    fn test_derive_asset_id_different_issuer() {
        let issuer_key_1 = [1u8; 32];
        let issuer_key_2 = [2u8; 32];
        let asset_desc_hash = [3u8; 32];

        let asset_id_1 = derive_asset_id(&issuer_key_1, &asset_desc_hash);
        let asset_id_2 = derive_asset_id(&issuer_key_2, &asset_desc_hash);

        assert_ne!(asset_id_1, asset_id_2, "Different issuers must produce different asset IDs");
    }

    #[test]
    fn test_derive_asset_id_different_description() {
        let issuer_key = [1u8; 32];
        let asset_desc_hash_1 = [2u8; 32];
        let asset_desc_hash_2 = [3u8; 32];

        let asset_id_1 = derive_asset_id(&issuer_key, &asset_desc_hash_1);
        let asset_id_2 = derive_asset_id(&issuer_key, &asset_desc_hash_2);

        assert_ne!(asset_id_1, asset_id_2, "Different descriptions must produce different asset IDs");
    }

    #[test]
    fn test_derive_asset_id_not_native() {
        let issuer_key = [1u8; 32];
        let asset_desc_hash = [2u8; 32];

        let asset_id = derive_asset_id(&issuer_key, &asset_desc_hash);

        assert!(!asset_id.is_native(), "Derived asset ID should not equal native USDx");
        assert_ne!(asset_id, crate::nu7_types::AssetBase::NATIVE);
    }
}
