//! Verkle proof verification - currently delegates to Go FFI.
//!
//! Phase 7 plan:
//! - Deserialize VerkleProof (commitments, extension statuses, IPA proof)
//! - Rebuild pre-state commitment tree and verify against pre-state root
//! - Run multiproof + IPA verification
//! - Apply StateDiff to compute post-state root
//!
//! This module currently holds data structures and lightweight helpers for
//! Verkle proof parsing. Full cryptographic verification still delegates to
//! the Go FFI; native verification will build on these types.

use alloc::vec::Vec;
use core::fmt;

use ark_ff::{PrimeField, Zero, BigInteger};
use utils::functions::log_error;
use crate::verkle::{
    BandersnatchPoint, Fr, IPAConfig, IPAProof, Transcript, generate_srs_points, verify_ipa_proof,
};

/// A single query in a multiproof: commitment, evaluation point, and result.
///
/// Used for proper multiproof aggregation per go-ipa specification.
#[derive(Clone, Debug)]
pub struct MultiproofQuery {
    pub commitment: BandersnatchPoint,
    pub eval_point: Fr,  // z_i
    pub result: Fr,      // y_i
}

/// Compute powers of r for multiproof aggregation: [1, r, r², r³, ..., r^(n-1)]
///
/// Used to compute challenge scalar powers for commitment/evaluation aggregation.
pub fn compute_r_powers(r: Fr, count: usize) -> Vec<Fr> {
    let mut powers = Vec::with_capacity(count);
    let mut current = Fr::from(1u64);
    for _ in 0..count {
        powers.push(current);
        current *= r;
    }
    powers
}

/// Aggregate commitments using r-powers: C_agg = Σᵢ r^i · Cᵢ
///
/// **Algorithm**: C_agg = MSM(commitments, r_powers)
pub fn aggregate_commitments(
    commitments: &[BandersnatchPoint],
    r_powers: &[Fr],
) -> Result<BandersnatchPoint, VerkleProofError> {
    if commitments.len() != r_powers.len() {
        return Err(VerkleProofError::LengthMismatch);
    }
    Ok(BandersnatchPoint::msm(commitments, r_powers))
}

/// Aggregate evaluations: g₂(t) = Σᵢ (r^i · yᵢ) / (t - zᵢ)
///
/// **Algorithm**:
/// 1. Compute denominators: dᵢ = t - zᵢ
/// 2. Batch invert: dᵢ ← 1/dᵢ
/// 3. Sum: g₂(t) = Σᵢ r^i · yᵢ · (1/(t - zᵢ))
pub fn aggregate_evaluations(
    results: &[Fr],           // y_i values
    eval_points: &[Fr],       // z_i values
    r_powers: &[Fr],
    t: &Fr,
) -> Result<Fr, VerkleProofError> {
    if results.len() != eval_points.len() || results.len() != r_powers.len() {
        return Err(VerkleProofError::LengthMismatch);
    }

    // Compute denominators: t - z_i
    let mut denominators: Vec<Fr> = eval_points
        .iter()
        .map(|z_i| *t - z_i)
        .collect();

    // Reject proofs that produce a zero denominator (t equals some z_i)
    if denominators.iter().any(|d| d.is_zero()) {
        return Err(VerkleProofError::VerificationFailed);
    }

    // Batch invert to get 1/(t - z_i)
    crate::verkle::batch_invert(&mut denominators);

    // Compute Σᵢ (r^i · y_i) / (t - z_i)
    let mut g2_t = Fr::zero();
    for i in 0..results.len() {
        let term = r_powers[i] * results[i] * denominators[i];
        g2_t += term;
    }

    Ok(g2_t)
}

/// Compact Verkle proof (matches go-verkle v0.2.2 wire format).
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct VerkleProof {
    pub other_stems: Vec<[u8; 32]>,
    pub depth_extension_present: Vec<u8>,
    pub commitments_by_path: Vec<[u8; 32]>,
    pub d: [u8; 32],
    pub ipa_proof: IPAProofCompact,
    /// go-verkle indices (zᵢ) for multiproof verification; required for IPA.
    pub indices: Vec<u8>,
    /// go-verkle values (yᵢ) for multiproof verification; same order as indices.
    pub values: Vec<[u8; 32]>,
}

/// IPA sub-proof inside VerkleProof (compressed L/R/A).
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct IPAProofCompact {
    pub cl: Vec<[u8; 32]>,
    pub cr: Vec<[u8; 32]>,
    pub final_evaluation: [u8; 32],
}

/// Per-suffix value change for a single stem.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct SuffixStateDiff {
    pub suffix: u8,
    pub current_value: Option<[u8; 32]>,
    pub new_value: Option<[u8; 32]>,
}

/// All suffix updates for a stem.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct StemStateDiff {
    pub stem: [u8; 31],
    pub suffix_diffs: Vec<SuffixStateDiff>,
}

pub type StateDiff = Vec<StemStateDiff>;

/// Errors in Verkle proof parsing / preparation.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum VerkleProofError {
    LengthMismatch,
    InvalidPoint,
    EmptyCommitments,
    RootMismatch,
    PathMismatch,
    VerificationFailed,
    NotImplemented,
    ParseError,
    InvalidProofStructure,
    UnusedCommitments,
}

impl fmt::Display for VerkleProofError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            VerkleProofError::LengthMismatch => write!(f, "length mismatch"),
            VerkleProofError::InvalidPoint => write!(f, "invalid point"),
            VerkleProofError::EmptyCommitments => write!(f, "no commitments in proof"),
            VerkleProofError::RootMismatch => write!(f, "pre-state root mismatch"),
            VerkleProofError::PathMismatch => write!(f, "path commitment mismatch"),
            VerkleProofError::VerificationFailed => write!(f, "ipa verification failed"),
            VerkleProofError::NotImplemented => write!(f, "not implemented"),
            VerkleProofError::ParseError => write!(f, "proof parsing error"),
            VerkleProofError::InvalidProofStructure => write!(f, "invalid proof structure"),
            VerkleProofError::UnusedCommitments => write!(f, "proof has unused commitments"),
        }
    }
}

impl IPAProofCompact {
    /// Convert compact form into fully deserialized IPAProof (decompress L/R, parse A).
    pub fn to_ipa_proof(&self) -> Result<IPAProof, VerkleProofError> {
        if self.cl.len() != self.cr.len() {
            return Err(VerkleProofError::LengthMismatch);
        }
        let mut l = Vec::with_capacity(self.cl.len());
        let mut r = Vec::with_capacity(self.cr.len());
        for bytes in &self.cl {
            let p = BandersnatchPoint::from_bytes(bytes)
                .map_err(|_| VerkleProofError::InvalidPoint)?;
            l.push(p);
        }
        for bytes in &self.cr {
            let p = BandersnatchPoint::from_bytes(bytes)
                .map_err(|_| VerkleProofError::InvalidPoint)?;
            r.push(p);
        }
        // CRITICAL: Fixture format stores finalEvaluation in BIG-ENDIAN (unlike other field elements which are LE)
        // This matches go-verkle's JSON serialization format
        let a = Fr::from_be_bytes_mod_order(&self.final_evaluation);

        #[cfg(feature = "verkle-debug")]
        {
            extern crate std;
            use ark_ff::BigInteger;
            std::println!("IPA Proof Parsing:");
            std::println!("  final_evaluation (raw bytes) = {}", hex::encode(&self.final_evaluation));
            std::println!("  a scalar (LE) = {}", hex::encode(a.into_bigint().to_bytes_le()));
        }

        Ok(IPAProof { l, r, a })
    }
}

impl VerkleProof {
    /// Build a VerkleProof from raw components, validating basic shape.
    pub fn from_parts(
        other_stems: Vec<[u8; 32]>,
        depth_extension_present: Vec<u8>,
        commitments_by_path: Vec<[u8; 32]>,
        d: [u8; 32],
        ipa_proof: IPAProofCompact,
        indices: Vec<u8>,
        values: Vec<[u8; 32]>,
    ) -> Result<Self, VerkleProofError> {
        // Require IPA L/R lengths to match.
        if ipa_proof.cl.len() != ipa_proof.cr.len() {
            return Err(VerkleProofError::LengthMismatch);
        }
        if indices.is_empty() || values.is_empty() || indices.len() * 32 != values.len() * 32 {
            // Require both to be present and length-consistent; go-verkle emits |indices| == |values|
            return Err(VerkleProofError::LengthMismatch);
        }
        Ok(Self {
            other_stems,
            depth_extension_present,
            commitments_by_path,
            d,
            ipa_proof,
            indices,
            values,
        })
    }
}

/// Validate that the root commitment is present and matches the expected root.
///
/// **SECURITY**: All Verkle proofs MUST include the root commitment as commitments_by_path[0].
/// Rootless proofs are ALWAYS REJECTED because they cannot cryptographically bind the claimed
/// root to the proof commitments.
///
/// Returns:
/// - `Ok(true)` if commitments_by_path[0] matches pre_state_root (valid proof)
/// - `Err(RootMismatch)` if commitments_by_path[0] exists but doesn't match pre_state_root
/// - `Err(EmptyCommitments)` if commitments_by_path is empty (rootless proof)
///
/// **NOTE**: This function does NOT validate commitments_by_path length against the expected
/// 1 + total_depth formula. Length validation is deferred to the tree building stage.
///
/// **CRITICAL**: Used by both pre-state and post-state verification to enforce root presence.
fn is_root_present(
    pre_state_root: &[u8; 32],
    proof: &VerkleProof,
) -> Result<bool, VerkleProofError> {
    // Check if first commitment matches pre_state_root
    // If yes → root is present and valid
    // If no → root mismatch (reject)
    // If empty → no commitments (reject)
    match proof.commitments_by_path.first() {
        Some(c) if c == pre_state_root => Ok(true),
        Some(_) => Err(VerkleProofError::RootMismatch),
        None => Err(VerkleProofError::EmptyCommitments),
    }
}

/// Validate that commitments_by_path starts with the expected root commitment.
///
/// **SECURITY**: Ensures the proof is cryptographically bound to the expected state root.
pub fn ensure_root_commitment(
    state_root: &[u8; 32],
    _depth_extension_present: &[u8],
    commitments_by_path: Vec<[u8; 32]>,
) -> Result<Vec<[u8; 32]>, VerkleProofError> {
    // SECURITY: Verify root commitment matches to ensure cryptographic binding.

    if commitments_by_path.is_empty() {
        return Err(VerkleProofError::EmptyCommitments);
    }

    // Validate that first commitment matches expected state_root
    if &commitments_by_path[0] != state_root {
        return Err(VerkleProofError::RootMismatch);
    }

    // Note: We don't validate exact length because go-verkle may share commitments
    // across stems with common prefixes. Tree reconstruction will fail naturally
    // if the proof structure is malformed (too few or too many commitments).
    Ok(commitments_by_path)
}

/// Verify pre-state commitment consistency by checking root and validating path structure.
///
/// **What this checks**:
/// 1. All commitments and D are well-formed Bandersnatch points
/// 2. Root commitment validation (via `is_root_present`):
///    - commitments_by_path[0] MUST match pre_state_root
///    - Rootless proofs are detected and rejected in `is_root_present`
///
/// **What this does NOT check**:
/// - Commitment count formula (len == 1 + total_depth): Not validated because go-verkle
///   proofs share internal node commitments for stems with common prefixes
/// - Full path structure: Delegated to `verify_path_structure`
/// - Cryptographic validity: Delegated to `verify_ipa_multiproof`
///
/// **SECURITY**: Root must be present and correct. Rootless proofs are rejected because they
/// cannot bind the claimed root to the proof commitments.
pub fn verify_prestate_commitment(
    pre_state_root: &[u8; 32],
    proof: &VerkleProof,
) -> Result<(), VerkleProofError> {
    if proof.commitments_by_path.is_empty() {
        return Err(VerkleProofError::EmptyCommitments);
    }

    let mut root_bytes = [0u8; 32];
    root_bytes.copy_from_slice(pre_state_root);

    // Validate all commitments and D are well-formed points.
    for c in &proof.commitments_by_path {
        BandersnatchPoint::from_bytes(c).map_err(|_| VerkleProofError::InvalidPoint)?;
    }
    BandersnatchPoint::from_bytes(&proof.d).map_err(|_| VerkleProofError::InvalidPoint)?;
    BandersnatchPoint::from_bytes(&root_bytes).map_err(|_| VerkleProofError::InvalidPoint)?;

    // Phase 1: Enforce root presence and validation
    // This validates that commitments_by_path[0] == pre_state_root
    is_root_present(pre_state_root, proof)?;

    // Note: We don't validate commitment count against depth_extension_present formula
    // because go-verkle's depth encoding doesn't account for the manually-prepended root
    // The IPA verification will fail if the proof structure is invalid

    Ok(())
}

/// Verify path structure: map depth_extension_present onto stem prefixes and ensure
/// ordering matches go-verkle's non-decreasing path traversal (shared prefixes allowed).
///
/// **Root required**: Assumes a root commitment is present at commitments_by_path[0].
pub fn verify_path_structure(
    proof: &VerkleProof,
    stems: &[[u8; 31]],
) -> Result<(), VerkleProofError> {
    if stems.len() != proof.depth_extension_present.len() {
        return Err(VerkleProofError::LengthMismatch);
    }

    if proof.commitments_by_path.is_empty() {
        return Err(VerkleProofError::EmptyCommitments);
    }

    // Note: We don't validate commitment count against depth formula here
    // because go-verkle's depth encoding doesn't account for the manually-prepended root
    // The root presence is validated in verify_prestate_commitment

    // Enforce go-verkle's lexicographic ordering (non-decreasing) while allowing
    // shared prefixes. Proofs routinely reuse internal nodes, so duplicate prefixes
    // must not be rejected. Depth MUST be non-zero and within the stem length.
    let mut previous_prefix: Option<alloc::vec::Vec<u8>> = None;
    for (stem, es) in stems.iter().zip(proof.depth_extension_present.iter()) {
        let depth = (es >> 3) as usize; // depth includes the leaf hop
        if depth == 0 || depth > stem.len() {
            return Err(VerkleProofError::PathMismatch);
        }

        let prefix = stem[..depth].to_vec();
        if let Some(prev) = &previous_prefix {
            if prev > &prefix {
                return Err(VerkleProofError::PathMismatch);
            }
        }
        previous_prefix = Some(prefix);
    }

    Ok(())
}

/// Convenience: verify root commitment and path structure using StateDiff stems.
pub fn verify_prestate_with_paths(
    pre_state_root: &[u8; 32],
    proof: &VerkleProof,
    state_diff: &StateDiff,
) -> Result<(), VerkleProofError> {
    verify_prestate_commitment(pre_state_root, proof)?;
    let stems: Vec<[u8; 31]> = state_diff.iter().map(|s| s.stem).collect();
    verify_path_structure(proof, &stems)
}

/// Complete Verkle proof verification including cryptographic multiproof.
///
/// Performs three verification steps:
/// 1. Structure validation (commitments are valid points, root matches)
/// 2. Path structure validation (depths match, no duplicate paths)
/// 3. Cryptographic verification via IPA multiproof
///
/// This is the recommended high-level API for Verkle proof verification.
pub fn verify_verkle_proof(
    pre_state_root: &[u8; 32],
    proof: &VerkleProof,
    commitments: &[BandersnatchPoint],
    state_diff: &StateDiff,
) -> Result<bool, VerkleProofError> {
    // Step 1: Validate structure and root commitment
    verify_prestate_commitment(pre_state_root, proof)?;

    // Step 2: Validate path structure
    let stems: Vec<[u8; 31]> = state_diff.iter().map(|s| s.stem).collect();
    verify_path_structure(proof, &stems)?;

    // Step 3: Cryptographic verification via IPA multiproof
    verify_ipa_multiproof(commitments, proof)
}

/// Build IPA evaluation points (z, y) from witness entries.
///
/// For each witness entry, this function determines the evaluation points for the leaf polynomial
/// and computes expected values from witness data. This builds ONLY the indices and values,
/// NOT the commitments (which come from the proof).
///
/// **Evaluation structure per stem**:
/// - Base entries: (z=0, y=1), (z=1, y=stem)
/// - C1/C2 pointers: (z=2, y=C1), (z=3, y=C2) if any suffixes in range
/// - Suffix entries: (z=2*suffix, y=value_lo), (z=2*suffix+1, y=value_hi) for PRESENT keys only
///
/// **Returns**: (indices, values) where each pair (indices[i], values[i]) represents an
/// evaluation point (z, y) for the IPA multiproof.
/// **LEGACY TEST UTILITY** - Bug 3 (zero-value handling) has been fixed.
///
/// Use `extract_proof_elements_from_tree` for production code.
pub fn build_evaluation_points_from_witness(
    entries: &[crate::verkle::VerkleWitnessEntry],
    use_pre_value: bool,
) -> Result<(Vec<u8>, Vec<Fr>), VerkleProofError> {
    use alloc::collections::BTreeMap;

    let mut indices = Vec::new();
    let mut values = Vec::new();

    // Group entries by stem
    let mut entries_by_stem: BTreeMap<[u8; 31], Vec<&crate::verkle::VerkleWitnessEntry>> = BTreeMap::new();

    for entry in entries {
        let mut stem = [0u8; 31];
        stem.copy_from_slice(&entry.key[0..31]);
        entries_by_stem.entry(stem).or_insert_with(Vec::new).push(entry);
    }

    // Process each stem
    for (stem, stem_entries) in entries_by_stem.iter() {
        // Build stem scalar for base entry
        let mut stem_bytes = [0u8; 32];
        stem_bytes[..stem.len()].copy_from_slice(stem);
        let stem_scalar = Fr::from_le_bytes_mod_order(&stem_bytes);

        // Base entry 0: marker = 1
        indices.push(0);
        values.push(Fr::from(1u64));

        // Base entry 1: stem value
        indices.push(1);
        values.push(stem_scalar);

        // Build C1 and C2 polynomials to compute expected values at indices 2 and 3
        // NOTE: Both C1 and C2 are ALWAYS present in the leaf polynomial (tree.rs:202-203),
        // even if one or both are zero (identity point mapped to scalar).
        let srs = generate_srs_points(256);
        let mut c1_poly = [Fr::zero(); 256];
        let mut c2_poly = [Fr::zero(); 256];

        for entry in stem_entries.iter() {
            let suffix = entry.key[31];
            let value = if use_pre_value { entry.pre_value } else { entry.post_value };

            // Bug 3 fix: Witness entries with [0u8; 32] are PRESENT (marker bit = 1), not absent
            let (lo, hi) = encode_value_to_field(Some(value));
            let suffix_idx = suffix as usize;

            if suffix_idx < 128 {
                let offset = suffix_idx * 2;
                c1_poly[offset] = lo;
                c1_poly[offset + 1] = hi;
            } else {
                let offset = (suffix_idx - 128) * 2;
                c2_poly[offset] = lo;
                c2_poly[offset + 1] = hi;
            }
        }

        // Compute C1 and C2 commitments (ALWAYS include both at indices 2 and 3)
        let c1 = BandersnatchPoint::msm(&srs, &c1_poly);
        let c2 = BandersnatchPoint::msm(&srs, &c2_poly);

        indices.push(2);
        values.push(c1.map_to_scalar_field());

        indices.push(3);
        values.push(c2.map_to_scalar_field());

        // Suffix entries - include all witness entries (zero values are PRESENT)
        let mut sorted_entries: Vec<_> = stem_entries.iter().collect();
        sorted_entries.sort_by_key(|e| e.key[31]);

        for entry in sorted_entries {
            let suffix = entry.key[31];
            let value = if use_pre_value { entry.pre_value } else { entry.post_value };
            let (lo, hi) = encode_value_to_field(Some(value));

            let suffix_idx = suffix as usize;
            // Leaf polynomial structure:
            // [0] = marker, [1] = stem, [2] = C1, [3] = C2
            // [4+] = suffix data, starting at index 4 + 2*suffix
            let z0 = (4 + 2 * suffix_idx) as u8;
            let z1 = z0 + 1;

            indices.push(z0);
            values.push(lo);

            indices.push(z1);
            values.push(hi);
        }
    }

    Ok((indices, values))
}

/// Multiproof inputs matching go-verkle `ProofElements` ordering.
pub struct MultiproofInputs {
    pub commitments: Vec<BandersnatchPoint>,
    pub indices: Vec<u8>,
    pub values: Vec<Fr>,
}

/// Encode a 32-byte value into the two-field-element representation used by go-verkle.
fn encode_value_to_field(value: Option<[u8; 32]>) -> (Fr, Fr) {
    match value {
        Some(bytes) => {
            let mut lo_with_marker = [0u8; 17];
            lo_with_marker[..16].copy_from_slice(&bytes[..16]);
            lo_with_marker[16] = 1;

            let mut hi_bytes = [0u8; 16];
            hi_bytes.copy_from_slice(&bytes[16..]);

            (
                Fr::from_le_bytes_mod_order(&lo_with_marker),
                Fr::from_le_bytes_mod_order(&hi_bytes),
            )
        }
        None => (Fr::zero(), Fr::zero()),
    }
}

// Extension status encoding (matches go-verkle doc.go)
const EXT_STATUS_ABSENT_EMPTY: u8 = 0;
const EXT_STATUS_ABSENT_OTHER: u8 = 1;
const EXT_STATUS_PRESENT: u8 = 2;

/// Rebuild the multiproof inputs (commitments, indices, values) directly from StateDiff, matching
/// go-verkle's `GetProofItems` ordering.
///
/// Ordering (per stem, following go-verkle leaf GetProofItems):
/// 1. Leaf commitment with indices 0 and 1 (1 || stem)
/// 2. Optional indices 2/3 for c1/c2 when that half of the leaf is present
/// 3. For every suffix (sorted), two evaluations (2*s, 2*s+1 mod 256) on c1 or c2
/// Build multiproof inputs matching go-verkle's GetProofItems layout.
///
/// **Go-verkle GetProofItems order:**
/// 1. Root commitment at z=STEM_FROM_KEY (66) with stem value (if root provided)
/// 2. Leaf commitment base: (z=0, v=1), (z=1, v=stem)
/// 3. Leaf commitment C1/C2 pointers: (z=2, v=C1), (z=3, v=C2) if present
/// 4. C1/C2 suffix evaluations: **ONLY for PRESENT keys** (absent keys excluded)
/// **LEGACY TEST UTILITY** - Do not use in production code.
///
/// This function is preserved for existing test cases but should not be used
/// in new code. Use the three-step verification instead:
/// 1. `rebuild_stateless_tree_from_proof`
/// 2. `extract_proof_elements_from_tree`
/// 3. `verify_ipa_multiproof_with_evals`
pub fn build_multiproof_inputs_from_state_diff_with_root(
    state_diff: &StateDiff,
    root_commitment: Option<BandersnatchPoint>,
) -> Result<MultiproofInputs, VerkleProofError> {
    let srs = generate_srs_points(256);
    let mut commitments = Vec::new();
    let mut indices = Vec::new();
    let mut values = Vec::new();

    // Process stems in deterministic order to mirror go-verkle's key sorting.
    let mut stems_sorted: Vec<&StemStateDiff> = state_diff.iter().collect();
    stems_sorted.sort_by_key(|sd| sd.stem);

    // Track which root indices we've already added to avoid duplicates
    // Stems sharing the same prefix (stem[0]) should only add ONE root evaluation
    use alloc::collections::BTreeSet;
    let mut seen_root_indices = BTreeSet::new();

    for stem_diff in stems_sorted {
        // Build polynomials first, as we need leaf_commitment for root entry value
        let mut c1_poly = [Fr::zero(); 256];
        let mut c2_poly = [Fr::zero(); 256];

        for suffix_diff in &stem_diff.suffix_diffs {
            let (lo, hi) = encode_value_to_field(suffix_diff.current_value);
            let suffix_idx = suffix_diff.suffix as usize;
            if suffix_idx < 128 {
                let offset = suffix_idx * 2;
                c1_poly[offset] = lo;
                c1_poly[offset + 1] = hi;
            } else {
                let offset = (suffix_idx - 128) * 2;
                c2_poly[offset] = lo;
                c2_poly[offset + 1] = hi;
            }
        }

        let c1 = BandersnatchPoint::msm(&srs, &c1_poly);
        let c2 = BandersnatchPoint::msm(&srs, &c2_poly);

        // Root-level polynomial: [1, stem, map_to_scalar(c1), map_to_scalar(c2)]
        let mut root_poly = [Fr::zero(); 256];
        root_poly[0] = Fr::from(1u64);

        let mut stem_bytes = [0u8; 32];
        stem_bytes[..stem_diff.stem.len()].copy_from_slice(&stem_diff.stem);
        root_poly[1] = Fr::from_le_bytes_mod_order(&stem_bytes);
        root_poly[2] = c1.map_to_scalar_field();
        root_poly[3] = c2.map_to_scalar_field();

        let leaf_commitment = BandersnatchPoint::msm(&srs, &root_poly);

        // Step 1: Add root commitment at z=stem[0] if provided
        // SECURITY: Only add root evaluation ONCE per unique stem[0] to avoid duplicate evaluations
        // Stems sharing the same prefix (stem[0]) share the same internal node commitment
        if let Some(root) = root_commitment {
            let root_index = stem_diff.stem[0];
            if seen_root_indices.insert(root_index) {
                // First time seeing this stem[0] - add root evaluation
                #[cfg(feature = "verkle-debug")]
                {
                    extern crate std;
                    std::println!("Adding root: C={} at z={}", hex::encode(&root.to_bytes()[..8]), root_index);
                }
                commitments.push(root);
                indices.push(root_index);
                values.push(leaf_commitment.map_to_scalar_field());
            } else {
                #[cfg(feature = "verkle-debug")]
                {
                    extern crate std;
                    std::println!("Skipping duplicate root at z={} (already added for shared prefix)", root_index);
                }
            }
        }

        // Step 2: Leaf base entries
        commitments.push(leaf_commitment);
        indices.push(0);
        values.push(root_poly[0]);

        commitments.push(leaf_commitment);
        indices.push(1);
        values.push(root_poly[1]);

        let has_c1 = stem_diff.suffix_diffs.iter().any(|sd| sd.suffix < 128);
        let has_c2 = stem_diff.suffix_diffs.iter().any(|sd| sd.suffix >= 128);

        if has_c1 {
            commitments.push(leaf_commitment);
            indices.push(2);
            values.push(root_poly[2]);
        }
        if has_c2 {
            commitments.push(leaf_commitment);
            indices.push(3);
            values.push(root_poly[3]);
        }

        // Suffix-level entries in ascending suffix order
        // CRITICAL: go-verkle GetProofItems ONLY includes PRESENT keys, not absent ones
        let mut suffixes: Vec<_> = stem_diff
            .suffix_diffs
            .iter()
            .filter(|sd| sd.current_value.is_some()) // Exclude absent keys
            .collect();
        suffixes.sort_by_key(|sd| sd.suffix);

        for suffix_diff in suffixes {
            let suffix_idx = suffix_diff.suffix as usize;
            let (poly, commitment) = if suffix_idx < 128 {
                (&c1_poly, c1)
            } else {
                (&c2_poly, c2)
            };

            let offset = if suffix_idx < 128 {
                suffix_idx * 2
            } else {
                (suffix_idx - 128) * 2
            };

            let z0 = ((2 * suffix_idx) % 256) as u8;
            let z1 = z0.wrapping_add(1);

            commitments.push(commitment);
            indices.push(z0);
            values.push(poly[offset]);

            commitments.push(commitment);
            indices.push(z1);
            values.push(poly[offset + 1]);
        }
    }

    Ok(MultiproofInputs {
        commitments,
        indices,
        values,
    })
}

/// Compute evaluation pairs (zᵢ, yᵢ) for IPA verification.
/// Placeholder: requires full path/key ordering; returns NotImplemented for now.
pub fn compute_evaluations_for_ipa(
    proof: &VerkleProof,
) -> Result<Vec<(Fr, Fr)>, VerkleProofError> {
    if proof.indices.len() != proof.values.len() {
        return Err(VerkleProofError::LengthMismatch);
    }

    let mut evals = Vec::with_capacity(proof.indices.len());
    for (_idx, (z_byte, y_bytes)) in proof.indices.iter().zip(proof.values.iter()).enumerate() {
        let z = Fr::from(*z_byte as u64);
        // go-verkle stores Y values in little-endian form (via FromLEBytes)
        let y = Fr::from_le_bytes_mod_order(y_bytes);

        evals.push((z, y));
    }
    Ok(evals)
}

/// Verify a single IPA sub-proof using native IPA verifier.
///
/// Caller must supply:
/// - `commitment`: commitment for this evaluation (C in go-ipa)
/// - `eval_point` (zᵢ)
/// - `result` (yᵢ)
pub fn verify_ipa_subproof(
    commitment: &BandersnatchPoint,
    ipa_proof: &IPAProofCompact,
    eval_point: Fr,
    result: Fr,
) -> Result<bool, VerkleProofError> {
    let full_proof = ipa_proof.to_ipa_proof()?;
    let mut transcript = Transcript::new(b"ipa");
    let config = IPAConfig::new();
    verify_ipa_proof(&mut transcript, &config, commitment, &full_proof, eval_point, result)
        .map_err(|_| VerkleProofError::VerificationFailed)
}

/// Debug helper to write bytes to a static buffer for inspection
/// Using lazy_static would be better but we're in no_std
#[allow(static_mut_refs)]
static mut DEBUG_BUFFER: alloc::vec::Vec<u8> = alloc::vec::Vec::new();

#[allow(static_mut_refs)]
fn debug_write(label: &str, data: &[u8]) {
    use core::fmt::Write;
    use alloc::string::String;

    unsafe {
        let mut line = String::new();
        write!(line, "{}: ", label).ok();
        for byte in data.iter().take(16) {
            write!(line, "{:02x}", byte).ok();
        }
        if data.len() > 16 {
            write!(line, "...").ok();
        }
        write!(line, "\n").ok();
        DEBUG_BUFFER.extend_from_slice(line.as_bytes());
    }
}

#[allow(static_mut_refs)]
pub fn get_debug_output() -> alloc::string::String {
    unsafe {
        alloc::string::String::from_utf8_lossy(&DEBUG_BUFFER).into_owned()
    }
}

#[allow(static_mut_refs)]
pub fn clear_debug_output() {
    unsafe {
        DEBUG_BUFFER.clear();
    }
}

/// Verify IPA multiproof using proper aggregation per go-verkle spec.
///
/// Algorithm (from go-ipa multiproof):
/// 1. Build transcript: domain_sep → Cs → zs → ys → challenge r
/// 2. Compute r-powers: [1, r, r², ..., r^(n-1)]
/// 3. Add D point to transcript → challenge t
/// 4. Aggregate commitments: C_agg = Σᵢ r^i · Cᵢ
/// 5. Aggregate evaluations: g₂(t) = Σᵢ (r^i · yᵢ) / (t - zᵢ)
/// 6. Verify: IPA_Proof(C_agg - D, t, g₂(t))
pub fn verify_ipa_multiproof(
    commitments: &[BandersnatchPoint],
    proof: &VerkleProof,
) -> Result<bool, VerkleProofError> {
    let evals = compute_evaluations_for_ipa(proof)?;
    verify_ipa_multiproof_with_evals(commitments, proof, &evals)
}

/// Verify IPA multiproof with explicit evaluation pairs (zᵢ, yᵢ).
pub fn verify_ipa_multiproof_with_evals(
    commitments: &[BandersnatchPoint],
    proof: &VerkleProof,
    evals: &[(Fr, Fr)],
) -> Result<bool, VerkleProofError> {
    utils::functions::log_info(&alloc::format!(
        "    🔐 IPA: Entry with {} commitments, {} evals",
        commitments.len(),
        evals.len()
    ));

    debug_write("=== MULTIPROOF START ===", &[]);

    if evals.len() != commitments.len() {
        return Err(VerkleProofError::LengthMismatch);
    }

    let n = commitments.len();
    if n == 0 {
        return Ok(true);
    }

    utils::functions::log_info("    🔐 IPA: Extracting eval points and results");
    // Extract eval_points (zs) and results (ys) from evals
    let eval_points: Vec<Fr> = evals.iter().map(|(z, _)| *z).collect();
    let results: Vec<Fr> = evals.iter().map(|(_, y)| *y).collect();
    for i in 0..core::cmp::min(20, n) {
        use ark_ff::BigInteger;
        let c_bytes = commitments[i].to_bytes();
        let z_bytes = eval_points[i].into_bigint().to_bytes_le();
        let y_bytes = results[i].into_bigint().to_bytes_le();
        utils::functions::log_info(&alloc::format!(
            "    🔐 Transcript[{}]: C={} z={} y_le={}",
            i,
            hex::encode(&c_bytes),
            hex::encode(&z_bytes),
            hex::encode(&y_bytes)
        ));
    }

    utils::functions::log_info("    🔐 IPA: Validating evaluation domain");
    // Enforce evaluation domain: all z_i must fit in a single byte (0..255).
    // This mirrors go-verkle's domain indices and prevents truncation of high bytes
    // when grouping by z later in this function (line 1030-1036).
    for (_i, z) in eval_points.iter().enumerate() {
        let z_bytes = z.into_bigint().to_bytes_le();
        if z_bytes.iter().skip(1).any(|b| *b != 0) {
            #[cfg(test)]
            {
                extern crate std;
                std::println!("Invalid evaluation point at index {}: z must be in [0,255] domain, got bytes {:?}", _i, &z_bytes[..8]);
            }
            return Err(VerkleProofError::VerificationFailed);
        }
    }

    utils::functions::log_info("    🔐 IPA: Building transcript");
    // Step 1: Build transcript - MUST match go-ipa exactly
    // go-ipa: tr := common.NewTranscript("vt") then transcript.DomainSep("multiproof")
    let mut transcript = Transcript::new(b"vt");
    transcript.domain_sep(b"multiproof");

    // Step 2: Add Cs, zs, ys to transcript (matching go-ipa lines 186-191)
    #[cfg(feature = "verkle-debug")]
    {
        extern crate std;
        std::println!("\nRUST: Adding to transcript:");
    }

    for i in 0..n {
        #[cfg(feature = "verkle-debug")]
        {
            use ark_ff::BigInteger;
            extern crate std;
            let c_bytes = commitments[i].to_bytes();
            let z_bytes = eval_points[i].into_bigint().to_bytes_le();
            let y_bytes = results[i].into_bigint().to_bytes_le();
            // Log all elements (increased from 5 to 20) to compare with Go
            if i < 20 {
                std::println!("RUST transcript[{}]: C={} z={:02x} y_le={}",
                    i,
                    hex::encode(&c_bytes[..4]),
                    z_bytes[0],
                    hex::encode(&y_bytes[..4])
                );
            }
        }

        transcript.append_point(&commitments[i], b"C");
        transcript.append_scalar(&eval_points[i], b"z");
        transcript.append_scalar(&results[i], b"y");
    }

    #[cfg(feature = "verkle-debug")]
    {
        extern crate std;
        if n > 20 {
            std::println!("  ... ({} total)", n);
        } else {
            std::println!("RUST: {} total proof elements added to transcript", n);
        }
    }

    utils::functions::log_info("    🔐 IPA: Adding Cs/zs/ys to transcript");
    // Step 3: Generate challenge r (go-ipa line 193)
    let r = transcript.challenge_scalar(b"r");
    {
        use ark_ff::BigInteger;
        let r_bytes = r.into_bigint().to_bytes_le();
        utils::functions::log_info(&alloc::format!(
            "    🔐 IPA: r = {}",
            hex::encode(&r_bytes)
        ));
    }

    #[cfg(feature = "verkle-debug")]
    {
        use ark_ff::BigInteger;
        extern crate std;
        let r_bytes = r.into_bigint().to_bytes_le(); // LE to match Go output
        std::println!("RUST r (LE): {}", hex::encode(&r_bytes));
    }

    utils::functions::log_info("    🔐 IPA: Computing r powers");
    let r_powers = compute_r_powers(r, n);

    utils::functions::log_info("    🔐 IPA: Parsing D point and generating t");
    // Step 4: Append D and generate challenge t (go-ipa lines 196-197)
    let d_point = BandersnatchPoint::from_bytes(&proof.d)
        .map_err(|_| VerkleProofError::InvalidPoint)?;

    utils::functions::log_info(&alloc::format!(
        "    🔐 IPA: D = {}",
        hex::encode(&proof.d)
    ));

    #[cfg(test)]
    {
        extern crate std;
        std::println!("IPA DEBUG: D point = {}", hex::encode(&proof.d));
    }

    transcript.append_point(&d_point, b"D");
    let t = transcript.challenge_scalar(b"t");
    {
        use ark_ff::BigInteger;
        let t_bytes = t.into_bigint().to_bytes_le();
        utils::functions::log_info(&alloc::format!(
            "    🔐 IPA: t = {}",
            hex::encode(&t_bytes)
        ));
    }

    #[cfg(feature = "verkle-debug")]
    {
        use ark_ff::BigInteger;
        extern crate std;
        let t_bytes = t.into_bigint().to_bytes_le();
        std::println!("RUST t (LE): {}", hex::encode(&t_bytes));
    }

    utils::functions::log_info("    🔐 IPA: Grouping evaluations by z");
    // Step 5: Group evaluations by z value (go-ipa lines 201-210)
    // groupedEvals[z] = SUM (r^i * y_i) for all i where z_i == z
    let mut grouped_evals = [Fr::zero(); 256];
    for i in 0..n {
        // z is the evaluation point as u8 (0-255 domain index)
        let z_u8 = {
            let z_bigint = eval_points[i].into_bigint();
            let z_bytes = z_bigint.to_bytes_le();
            z_bytes[0]
        };
        let scaled = r_powers[i] * results[i];
        grouped_evals[z_u8 as usize] += scaled;
    }

    utils::functions::log_info("    🔐 IPA: Computing helper scalars and batch invert");
    // Compute helper_scalar_den = 1 / (t - z_i) for ALL 256 domain points (go-ipa lines 213-219)
    let mut helper_scalar_den = [Fr::zero(); 256];
    for i in 0..256 {
        let z_i = Fr::from(i as u64);
        helper_scalar_den[i] = t - z_i;
    }

    // If any denominator is zero (t hits the evaluation domain), the proof is invalid.
    if helper_scalar_den.iter().any(|d| d.is_zero()) {
        return Err(VerkleProofError::VerificationFailed);
    }
    crate::verkle::batch_invert(&mut helper_scalar_den);

    utils::functions::log_info("    🔐 IPA: Computing g2(t)");
    // Compute g_2(t) = SUM (y_i * r^i) / (t - z_i) (go-ipa lines 222-230)
    let mut g2_t = Fr::zero();
    for i in 0..256 {
        if grouped_evals[i].is_zero() {
            continue;
        }
        g2_t += grouped_evals[i] * helper_scalar_den[i];
    }

    #[cfg(feature = "verkle-debug")]
    {
        use ark_ff::BigInteger;
        extern crate std;
        let g2_bytes = g2_t.into_bigint().to_bytes_le();
        std::println!("RUST g2(t) (LE): {}", hex::encode(&g2_bytes));
    }
    {
        use ark_ff::BigInteger;
        let g2_bytes = g2_t.into_bigint().to_bytes_le();
        utils::functions::log_info(&alloc::format!(
            "    🔐 IPA: g2(t) = {}",
            hex::encode(&g2_bytes)
        ));
    }

    utils::functions::log_info("    🔐 IPA: Computing E via MSM");
    // Step 6: Compute E = SUM C_i * (r^i / (t - z_i)) (go-ipa lines 232-244)
    let mut msm_scalars: Vec<Fr> = Vec::with_capacity(n);
    for i in 0..n {
        let z_u8 = {
            let z_bigint = eval_points[i].into_bigint();
            let z_bytes = z_bigint.to_bytes_le();
            z_bytes[0]
        };
        msm_scalars.push(r_powers[i] * helper_scalar_den[z_u8 as usize]);
    }
    let e_point = BandersnatchPoint::msm(commitments, &msm_scalars);
    utils::functions::log_info("    🔐 IPA: MSM complete");
    utils::functions::log_info(&alloc::format!(
        "    🔐 IPA: E = {}",
        hex::encode(e_point.to_bytes())
    ));

    #[cfg(feature = "verkle-debug")]
    {
        extern crate std;
        std::println!("RUST E (compressed): {}", hex::encode(e_point.to_bytes()));
    }

    utils::functions::log_info("    🔐 IPA: Appending E to transcript");
    // Step 7: Append E to transcript (go-ipa line 245)
    transcript.append_point(&e_point, b"E");

    utils::functions::log_info("    🔐 IPA: Computing E - D");
    // Step 8: Compute E - D (go-ipa lines 247-248)
    // For twisted Edwards curves, negation is (-x, y), not (-x, -y)
    let neg_d = BandersnatchPoint {
        x: -d_point.x,
        y: d_point.y,
    };
    let e_minus_d = e_point.add(&neg_d);
    utils::functions::log_info(&alloc::format!(
        "    🔐 IPA: E-D = {}",
        hex::encode(e_minus_d.to_bytes())
    ));

    #[cfg(feature = "verkle-debug")]
    {
        extern crate std;
        std::println!("RUST E-D (compressed): {}", hex::encode(e_minus_d.to_bytes()));
    }

    utils::functions::log_info("    🔐 IPA: Converting proof and calling inner IPA verification");
    // Step 9: Verify IPA proof (go-ipa line 250)
    let full_proof = proof.ipa_proof.to_ipa_proof()?;
    if !full_proof.l.is_empty() && !full_proof.r.is_empty() {
        utils::functions::log_info(&alloc::format!(
            "    🔐 IPA: proof L0={} R0={}",
            hex::encode(full_proof.l[0].to_bytes()),
            hex::encode(full_proof.r[0].to_bytes())
        ));
    }
    utils::functions::log_info("    🔐 IPA: Building IPA config (SRS + weights)");
    let config = IPAConfig::new();
    utils::functions::log_info("    🔐 IPA: Config ready, invoking verifier");

    #[cfg(feature = "verkle-debug")]
    {
        use ark_ff::BigInteger;
        extern crate std;
        std::println!("\n=== CALLING INNER IPA VERIFICATION ===");
        std::println!("  commitment (E-D): {}", hex::encode(e_minus_d.to_bytes()));
        std::println!("  eval_point (t): {}", hex::encode(t.into_bigint().to_bytes_le()));
        std::println!("  result (g2_t): {}", hex::encode(g2_t.into_bigint().to_bytes_le()));
        std::println!("  proof.l.len(): {}", full_proof.l.len());
        std::println!("  proof.r.len(): {}", full_proof.r.len());
        let a_bytes = full_proof.a.into_bigint().to_bytes_le();
        std::println!("  proof.a: {}", hex::encode(&a_bytes));
    }

    let result = verify_ipa_proof(
        &mut transcript,
        &config,
        &e_minus_d,
        &full_proof,
        t,
        g2_t,
    )
    .map_err(|_| VerkleProofError::VerificationFailed)?;

    if result {
        utils::functions::log_info("    🔐 IPA: Verification success");
    } else {
        utils::functions::log_error("    🔐 IPA: Verification returned false");
    }

    Ok(result)
}

/// Construct a StateDiff from key/value slices (pre/post), enforcing suffix uniqueness per stem.
pub fn build_state_diff(
    keys: &[[u8; 32]],
    pre_values: &[Option<[u8; 32]>],
    post_values: &[Option<[u8; 32]>],
) -> Result<StateDiff, VerkleProofError> {
    if keys.len() != pre_values.len() || keys.len() != post_values.len() {
        return Err(VerkleProofError::LengthMismatch);
    }

    let mut stems: Vec<StemStateDiff> = Vec::new();

    for i in 0..keys.len() {
        let mut stem = [0u8; 31];
        stem.copy_from_slice(&keys[i][0..31]);
        let suffix = keys[i][31];

        // Find or insert stem entry preserving first-seen order.
        let pos = stems
            .iter()
            .position(|s| s.stem == stem)
            .unwrap_or_else(|| {
                stems.push(StemStateDiff {
                    stem,
                    suffix_diffs: Vec::new(),
                });
                stems.len() - 1
            });

        // Enforce one entry per suffix per stem.
        let stem_entry = &mut stems[pos];
        if stem_entry
            .suffix_diffs
            .iter()
            .any(|sd| sd.suffix == suffix)
        {
            return Err(VerkleProofError::LengthMismatch);
        }

        stem_entry.suffix_diffs.push(SuffixStateDiff {
            suffix,
            current_value: pre_values[i],
            new_value: post_values[i],
        });
    }

    // Within each stem, keep suffixes in ascending order for determinism.
    for s in stems.iter_mut() {
        s.suffix_diffs
            .sort_by_key(|sd| sd.suffix);
    }

    Ok(stems)
}

/// Verify post-state root by applying StateDiff to pre-state tree.
///
/// **CRITICAL FIX**: Now properly validates pre→post transition:
/// 1. Build pre-state tree from StateDiff current_value fields
/// 2. Apply transitions: update each modified key from current_value to new_value
/// 3. Compute post-state root commitment
/// 4. Compare with expected post_state_root
///
/// **Algorithm** (from go-verkle PostStateTreeFromStateDiff):
/// - Build initial tree with current (pre) values
/// - Apply new values to get post-state
/// - Verify the transition is valid
///
/// See go-verkle `PostStateTreeFromStateDiff` (proof_ipa.go:588-617)
/// Verify post-state root by building pre-state tree and applying transitions.
///
/// **IMPORTANT**: This is a TEST HELPER only. The guarantor execution model does NOT use this function.
/// Guarantors verify state transitions through cache-only re-execution + IPA witness verification,
/// NOT by recomputing post-state roots. Recomputing roots would be expensive and unnecessary since
/// IPA proofs cryptographically bind witness values to roots.
///
/// **Phase 3 fixes**:
/// 1. Handles deletions correctly: new_value=None means delete (set to zero bytes)
/// 2. Properly transitions pre→post by applying all updates including deletions
///
/// **NOTE**: This still doesn't validate against proof commitments (see Known Issues #4)
pub fn verify_post_state_root(
    state_diff: &StateDiff,
    post_state_root: &[u8; 32],
) -> Result<bool, VerkleProofError> {
    use crate::verkle::{VerkleNode, generate_srs_points, ValueUpdate};

    // Step 1: Build pre-state tree from current_value fields
    let mut pre_state_tree = VerkleNode::new_internal();
    for stem_diff in state_diff {
        let mut pre_values = [ValueUpdate::NoChange; 256];
        for suffix_diff in &stem_diff.suffix_diffs {
            pre_values[suffix_diff.suffix as usize] = match suffix_diff.current_value {
                Some(v) => ValueUpdate::Set(v),
                None => ValueUpdate::NoChange, // Not present in pre-state
            };
        }
        pre_state_tree.insert_values_at_stem(&stem_diff.stem, &pre_values, 0)?;
    }

    // Step 2: Apply new_value updates to transition to post-state
    // FIXED: Handle deletions correctly - new_value=None means delete (remove from tree)
    for stem_diff in state_diff {
        let mut new_values = [ValueUpdate::NoChange; 256];
        for suffix_diff in &stem_diff.suffix_diffs {
            // Only apply updates where new_value differs from current_value
            if suffix_diff.new_value != suffix_diff.current_value {
                new_values[suffix_diff.suffix as usize] = match suffix_diff.new_value {
                    Some(v) => ValueUpdate::Set(v),
                    None => ValueUpdate::Delete, // True deletion (remove from tree)
                };
            }
        }
        // This updates the tree in-place, transitioning pre→post
        pre_state_tree.insert_values_at_stem(&stem_diff.stem, &new_values, 0)?;
    }

    // Step 3: Generate SRS for commitment computation
    let srs = generate_srs_points(256);

    // Step 4: Compute post-state root commitment
    let computed_root = pre_state_tree.root_commitment(&srs)?;
    let computed_bytes = computed_root.to_bytes();

    // Step 5: Compare with expected post-state root
    Ok(&computed_bytes == post_state_root)
}

/// Verify dual-proof Verkle witness (full end-to-end verification).
///
/// **Status**: ✅ NATIVE RUST IMPLEMENTATION COMPLETE (Phase 4 done)
///
/// Performs complete Verkle proof verification:
/// 1. Deserialize witness data into VerkleWitness (✅ native Rust)
/// 2. Parse proof data into VerkleProof (✅ native Rust - parse_compact_proof)
/// 3. Rebuild a stateless tree from proof commitments + witness values
/// 4. Extract per-evaluation commitments/values (proof elements)
/// 5. Verify pre-state commitment and path structure (✅ native Rust)
/// 6. Cryptographic verification via IPA multiproof (✅ native Rust)
/// 7. Pre→post transition verification happens during execution (not in this verifier)
///
/// **Implementation** (Phase 4 complete):
/// - ✅ Witness deserializer (verkle.rs:VerkleWitness::deserialize)
/// - ✅ Proof data parsing (parse_compact_proof - lines 979-1074)
/// - ✅ Stateless tree reconstruction (rebuild_stateless_tree_from_proof)
/// - ✅ Proof element extraction (extract_proof_elements_from_tree)
/// - ✅ Native verification pipeline (verify_witness_section - lines 860-908)
///
/// **No Go FFI fallback** - Pure native Rust verification.
pub fn verify_verkle_witness(witness_data: &[u8]) -> bool {
    // Native Rust verification - no Go FFI fallback

    // Step 1: Deserialize witness
    let witness = match crate::verkle::VerkleWitness::deserialize(witness_data) {
        Some(w) => w,
        None => {
            log_error("❌ Verkle witness deserialization failed");
            return false; // Deserialization failed
        }
    };

    // Step 2: Verify pre-state (read_entries)
    if !witness.read_entries.is_empty() {
        if let Err(e) = verify_witness_section(
            &witness.pre_state_root.0,
            &witness.pre_proof_data,
            &witness.read_entries,
            true, // pre-state: use pre_value
        ) {
            log_error(&alloc::format!("❌ Pre-state verification failed: {:?}", e));
            return false; // Pre-state verification failed
        }
    }

    // Step 3: Verify post-state (write_entries)
    if !witness.write_entries.is_empty() {
        if let Err(e) = verify_witness_section(
            &witness.post_state_root.0,
            &witness.post_proof_data,
            &witness.write_entries,
            false, // post-state: use post_value
        ) {
            log_error(&alloc::format!("❌ Post-state verification failed: {:?}", e));
            return false; // Post-state verification failed
        }
    }

    // JAM Execution Model - Verification Complete:
    // This function verifies cryptographic proofs (sufficient for security):
    //   1. read_entries.pre_value exists in pre_state_root ✓ (IPA proof)
    //   2. write_entries.post_value exists in post_state_root ✓ (IPA proof)
    //
    // Transition verification happens during cache-only execution (see state.rs):
    //   - Guarantor re-executes with reads from pre-witness cache
    //   - Any cache miss = execution invalid (GUARANTOR ... MISS panics)
    //   - Writes are compared during execution against expected post-state
    //   - If execution is deterministic + witnesses valid = state transition valid
    //
    // NO need to recompute post_state_root from pre_state_root + writes:
    //   - Would require expensive tree rebuilding
    //   - Would actually WEAKEN security (attacker could provide self-consistent invalid witness)
    //   - Cryptographic proofs + deterministic execution are sufficient
    //
    // See services/evm/docs/VERKLE.md "JAM Execution Model" section for full explanation

    // Both pre-state and post-state verified successfully
    true
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum WitnessSection {
    Pre,
    Post,
}

/// Verify a single witness section (pre or post) from serialized witness bytes.
pub fn verify_verkle_witness_section(
    witness_data: &[u8],
    section: WitnessSection,
) -> Result<(), VerkleProofError> {
    let witness = crate::verkle::VerkleWitness::deserialize(witness_data)
        .ok_or(VerkleProofError::ParseError)?;

    match section {
        WitnessSection::Pre => {
            if witness.read_entries.is_empty() {
                return Ok(());
            }
            verify_witness_section(
                &witness.pre_state_root.0,
                &witness.pre_proof_data,
                &witness.read_entries,
                true,
            )
        }
        WitnessSection::Post => {
            if witness.write_entries.is_empty() {
                return Ok(());
            }
            verify_witness_section(
                &witness.post_state_root.0,
                &witness.post_proof_data,
                &witness.write_entries,
                false,
            )
        }
    }
}

#[cfg(test)]
mod witness_file_test {
    extern crate std;

    use super::{verify_verkle_witness_section, WitnessSection};
    use std::{env, fs, string::String};

    #[test]
    fn verify_witness_section_from_file() {
        let path = match env::var("JAM_VERKLE_WITNESS_FILE") {
            Ok(value) => value,
            Err(_) => return,
        };
        let section = env::var("JAM_VERKLE_WITNESS_SECTION")
            .unwrap_or_else(|_| String::from("pre"));
        let witness_data = fs::read(&path).expect("failed to read witness file");

        match section.as_str() {
            "pre" => {
                verify_verkle_witness_section(&witness_data, WitnessSection::Pre)
                    .expect("pre-state verification failed");
            }
            "post" => {
                verify_verkle_witness_section(&witness_data, WitnessSection::Post)
                    .expect("post-state verification failed");
            }
            "both" => {
                verify_verkle_witness_section(&witness_data, WitnessSection::Pre)
                    .expect("pre-state verification failed");
                verify_verkle_witness_section(&witness_data, WitnessSection::Post)
                    .expect("post-state verification failed");
            }
            _ => panic!("invalid JAM_VERKLE_WITNESS_SECTION (use pre|post|both)"),
        }
    }
}

/// Extract unique stems from witness entries (31-byte prefix of each key).
fn extract_stems_from_witness(entries: &[crate::verkle::VerkleWitnessEntry]) -> Vec<[u8; 31]> {
    use alloc::collections::BTreeSet;

    let mut stems_set = BTreeSet::new();
    for entry in entries {
        let mut stem = [0u8; 31];
        stem.copy_from_slice(&entry.key[0..31]);
        stems_set.insert(stem);
    }

    stems_set.into_iter().collect()
}

/// Verify a single witness section (pre-state or post-state).
///
/// **Architecture** (refactored implementation):
/// 1. Parse proof to get commitments_by_path and IPA proof components
/// 2. Extract leaf commitments FROM PROOF (not rebuilt)
/// 3. Build evaluation points FROM WITNESS values
/// 4. Verify IPA multiproof with proof commitments + witness evaluations
///
/// This is the correct verification approach: proof provides commitments,
/// witness provides values to check against those commitments.
///
/// **Previous approach** (removed):
/// - Rebuilt all commitments from witness (expensive, incorrect for verification)
/// - Used rebuilt commitments for verification (bypassed proof validation)
fn verify_witness_section(
    state_root: &[u8; 32],
    proof_data: &[u8],
    entries: &[crate::verkle::VerkleWitnessEntry],
    use_pre_value: bool,
) -> Result<(), VerkleProofError> {
    utils::functions::log_info(&alloc::format!(
        "🔍 ENTRY verify_witness_section: entries={}, use_pre_value={}",
        entries.len(),
        use_pre_value
    ));

    // 1. Parse compact proof format
    utils::functions::log_info("  Step 1: Parsing compact proof...");
    let (other_stems, depth_extension_present, commitments_by_path_raw, d, ipa_proof) =
        parse_compact_proof(proof_data)?;
    utils::functions::log_info(&alloc::format!(
        "    ✅ Parsed: other_stems={}, depths={}, commitments={}",
        other_stems.len(),
        depth_extension_present.len(),
        commitments_by_path_raw.len()
    ));

    // 2. Validate root and rebuild stateless tree from proof commitments + witness values
    utils::functions::log_info("  Step 2: Ensuring root commitment...");
    let commitments_by_path =
        ensure_root_commitment(state_root, &depth_extension_present, commitments_by_path_raw)?;
    utils::functions::log_info("    ✅ Root validated");

    utils::functions::log_info("  Step 3: Rebuilding stateless tree...");
    let tree = rebuild_stateless_tree_from_proof(
        &commitments_by_path,
        &depth_extension_present,
        &other_stems,
        entries,
        use_pre_value,
    )?;
    utils::functions::log_info("    ✅ Tree rebuilt successfully");

    // Surface the distribution of root-child indices from the witness keys (sorted).
    let mut key0_bytes: alloc::vec::Vec<u8> = entries.iter().map(|e| e.key[0]).collect();
    key0_bytes.sort();
    let mut key0_unique = key0_bytes.clone();
    key0_unique.dedup();
    utils::functions::log_info(&alloc::format!(
        "    🔑 key[0] bytes (sorted): {:?} | unique: {:?}",
        key0_bytes, key0_unique
    ));

    // 3. Extract per-evaluation commitments/indices/values from rebuilt tree
    utils::functions::log_info("  Step 4: Extracting proof elements from tree...");
    let proof_elements = extract_proof_elements_from_tree(
        &tree,
        entries,
        use_pre_value,
    )?;
    utils::functions::log_info(&alloc::format!(
        "    ✅ Extracted: {} commitments, {} indices, {} values",
        proof_elements.commitments.len(),
        proof_elements.indices.len(),
        proof_elements.values.len()
    ));
    for (i, (c, (z, y))) in proof_elements
        .commitments
        .iter()
        .zip(proof_elements.indices.iter().zip(proof_elements.values.iter()))
        .enumerate()
    {
        utils::functions::log_info(&alloc::format!(
            "    🔎 Proof element {}: z={} commitment[0..4]={:02x?} y[0..4]={:02x?}",
            i,
            z,
            &c.to_bytes()[..4],
            &y.into_bigint().to_bytes_le()[..4]
        ));
    }

    // 4. Build evaluation pairs (z, y)
    utils::functions::log_info("  Step 5: Building evaluation pairs...");
    let evals: Vec<(Fr, Fr)> = proof_elements
        .indices
        .iter()
        .zip(proof_elements.values.iter())
        .map(|(z_byte, y_fr)| (Fr::from(*z_byte as u64), *y_fr))
        .collect();
    utils::functions::log_info(&alloc::format!("    ✅ Built {} evaluation pairs", evals.len()));

    // 5. Assemble VerkleProof container for IPA verification (indices/values unused here)
    // Note: We use direct struct construction instead of from_parts() because indices/values
    // are unused in this verification path (we build evaluation points separately)
    utils::functions::log_info("  Step 6: Assembling VerkleProof container...");
    let proof = VerkleProof {
        other_stems,
        depth_extension_present,
        commitments_by_path,
        d,
        ipa_proof,
        indices: Vec::new(),
        values: Vec::new(),
    };
    utils::functions::log_info("    ✅ Proof container assembled");

    // 6. Verify IPA multiproof
    utils::functions::log_info("  Step 7: Starting IPA multiproof verification...");
    let result = verify_ipa_multiproof_with_evals(&proof_elements.commitments, &proof, &evals)?;
    utils::functions::log_info(&alloc::format!("    ✅ IPA verification result: {}", result));

    if result {
        Ok(())
    } else {
        Err(VerkleProofError::VerificationFailed)
    }
}

/// Parse go-verkle compact proof format into components.
///
/// **Format** (go-verkle wire format):
/// - 1B: other_stems count
/// - N × 32B: other_stems (stems not in keys list)
/// - 1B: depth_extension_present length
/// - depth_len bytes: depth_extension_present array
/// - 1B: commitments_by_path count
/// - M × 32B: commitments_by_path
/// - 32B: D point
/// - 1B: IPA proof vector length (call it L)
/// - L × 32B: CL points (left commitments)
/// - L × 32B: CR points (right commitments)
/// - 32B: final_evaluation (A scalar)
///
/// Returns: (other_stems, depth_extension_present, commitments_by_path, d, ipa_proof)
pub fn parse_compact_proof(
    bytes: &[u8],
) -> Result<(Vec<[u8; 32]>, Vec<u8>, Vec<[u8; 32]>, [u8; 32], IPAProofCompact), VerkleProofError> {
    let mut cursor = 0usize;

    let read_byte = |cursor: &mut usize| -> Result<u8, VerkleProofError> {
        if *cursor >= bytes.len() {
            return Err(VerkleProofError::ParseError);
        }
        let b = bytes[*cursor];
        *cursor += 1;
        Ok(b)
    };

    let read_block = |cursor: &mut usize, len: usize| -> Result<&[u8], VerkleProofError> {
        if *cursor + len > bytes.len() {
            return Err(VerkleProofError::ParseError);
        }
        let slice = &bytes[*cursor..*cursor + len];
        *cursor += len;
        Ok(slice)
    };

    let read_array_32 = |cursor: &mut usize| -> Result<[u8; 32], VerkleProofError> {
        let block = read_block(cursor, 32)?;
        let mut arr = [0u8; 32];
        arr.copy_from_slice(block);
        Ok(arr)
    };

    // Parse other_stems
    let other_count = read_byte(&mut cursor)? as usize;
    let mut other_stems = Vec::with_capacity(other_count);
    for _ in 0..other_count {
        other_stems.push(read_array_32(&mut cursor)?);
    }

    // Parse depth_extension_present
    let depth_len = read_byte(&mut cursor)? as usize;
    let depth_extension_present = read_block(&mut cursor, depth_len)?.to_vec();

    // Parse commitments_by_path
    let commitment_len = read_byte(&mut cursor)? as usize;
    let mut commitments_by_path = Vec::with_capacity(commitment_len);
    for _ in 0..commitment_len {
        commitments_by_path.push(read_array_32(&mut cursor)?);
    }

    // Parse D point
    let d = read_array_32(&mut cursor)?;

    // Parse IPA proof
    let ipa_len = read_byte(&mut cursor)? as usize;
    let mut cl = Vec::with_capacity(ipa_len);
    for _ in 0..ipa_len {
        cl.push(read_array_32(&mut cursor)?);
    }
    let mut cr = Vec::with_capacity(ipa_len);
    for _ in 0..ipa_len {
        cr.push(read_array_32(&mut cursor)?);
    }

    // Binary wire format encodes finalEvaluation in LITTLE-ENDIAN (go-verkle ipa/prover.go:177)
    // but IPAProofCompact expects BIG-ENDIAN (matching JSON fixture format).
    // Convert LE → BE so to_ipa_proof() can uniformly use from_be_bytes_mod_order.
    let final_evaluation_le = read_array_32(&mut cursor)?;
    let mut final_evaluation = final_evaluation_le;
    final_evaluation.reverse(); // LE → BE conversion

    // Verify no trailing bytes
    if cursor != bytes.len() {
        return Err(VerkleProofError::ParseError);
    }

    // Log parsed proof structure
    utils::functions::log_info(&alloc::format!(
        "📊 RUST PARSED PROOF: other_stems={}, depth_len={}, commitments={}, ipa_depth={}",
        other_stems.len(),
        depth_extension_present.len(),
        commitments_by_path.len(),
        ipa_len
    ));

    if !other_stems.is_empty() {
        utils::functions::log_info(&alloc::format!(
            "  📌 other_stems[0] = {:02x?}...",
            &other_stems[0][..8]
        ));
    }
    if !commitments_by_path.is_empty() {
        utils::functions::log_info(&alloc::format!(
            "  📌 commitments[0] (ROOT) = {:02x?}...",
            &commitments_by_path[0][..8]
        ));
    }
    if commitments_by_path.len() > 1 {
        utils::functions::log_info(&alloc::format!(
            "  📌 commitments[1] = {:02x?}...",
            &commitments_by_path[1][..8]
        ));
    }

    Ok((
        other_stems,
        depth_extension_present,
        commitments_by_path,
        d,
        IPAProofCompact {
            cl,
            cr,
            final_evaluation,
        },
    ))
}

/// Convert witness entries to a StateDiff, choosing the "current" side based on verification mode.
///
/// Groups entries by stem (first 31 bytes of key), extracts suffix (last byte),
/// and maps values to represent the state transition:
///
/// - `use_pre_value=true`: current_value = pre_value, new_value = post_value (pre-state verification)
/// - `use_pre_value=false`: current_value = post_value, new_value = post_value (post-state verification)
pub fn witness_entries_to_state_diff(
    entries: &[crate::verkle::VerkleWitnessEntry],
    use_pre_value: bool,
) -> StateDiff {
    use alloc::collections::BTreeMap;

    let mut stem_map: BTreeMap<[u8; 31], Vec<SuffixStateDiff>> = BTreeMap::new();

    for entry in entries {
        let mut stem = [0u8; 31];
        stem.copy_from_slice(&entry.key[0..31]);
        let suffix = entry.key[31];

        let current_value = if use_pre_value {
            Some(entry.pre_value)
        } else {
            Some(entry.post_value)
        };

        stem_map
            .entry(stem)
            .or_insert_with(Vec::new)
            .push(SuffixStateDiff {
                suffix,
                current_value,
                // The transition always targets the post-state value
                new_value: Some(entry.post_value),
            });
    }

    stem_map
        .into_iter()
        .map(|(stem, suffix_diffs)| StemStateDiff {
            stem,
            suffix_diffs,
        })
        .collect()
}

/// A stateless Verkle tree node that stores commitments but not full state.
///
/// This mirrors go-verkle's stateless internal/leaf nodes used in `PreStateTreeFromProof`.
/// Nodes store proof-provided commitments and witness values, enabling per-evaluation-point
/// commitment extraction without full tree state.
#[derive(Clone, Debug)]
pub enum StatelessNode {
    /// Internal node at a given depth in the tree.
    Internal {
        depth: u8,
        commitment: BandersnatchPoint,
        /// Children indexed by byte value (0-255).
        /// Most children are None (sparse tree).
        children: alloc::boxed::Box<[Option<alloc::boxed::Box<StatelessNode>>; 256]>,
    },
    /// Known-empty child (proof-of-absence empty path).
    Empty(BandersnatchPoint),
    /// Leaf node containing witness values for a stem.
    Leaf {
        stem: [u8; 31],
        commitment: BandersnatchPoint,
        /// Values indexed by suffix (0-255).
        /// None = absent key, Some = present key with value.
        values: [Option<[u8; 32]>; 256],
        /// C1 commitment (covers suffixes 0-127).
        c1: BandersnatchPoint,
        /// C2 commitment (covers suffixes 128-255).
        c2: BandersnatchPoint,
        /// Proof-of-absence stub (no suffix data, C1/C2 must be identity).
        is_poa_stub: bool,
    },
}

impl StatelessNode {
    /// Create a new internal node with the given depth and commitment.
    pub fn new_internal(depth: u8, commitment: BandersnatchPoint) -> Self {
        StatelessNode::Internal {
            depth,
            commitment,
            children: alloc::boxed::Box::new([const { None }; 256]),
        }
    }

    /// Create a new leaf node with the given stem, commitment, and values.
    pub fn new_leaf(
        stem: [u8; 31],
        commitment: BandersnatchPoint,
        values: [Option<[u8; 32]>; 256],
        c1: BandersnatchPoint,
        c2: BandersnatchPoint,
        is_poa_stub: bool,
    ) -> Result<Self, VerkleProofError> {
        Ok(StatelessNode::Leaf {
            stem,
            commitment,
            values,
            c1,
            c2,
            is_poa_stub,
        })
    }

    /// Get the commitment for this node.
    pub fn commitment(&self) -> &BandersnatchPoint {
        match self {
            StatelessNode::Internal { commitment, .. } => commitment,
            StatelessNode::Leaf { commitment, .. } => commitment,
            StatelessNode::Empty(identity) => identity,
        }
    }

    /// Get a mutable reference to a child at the given index (internal nodes only).
    fn child_mut(&mut self, index: u8) -> Option<&mut Option<alloc::boxed::Box<StatelessNode>>> {
        match self {
            StatelessNode::Internal { children, .. } => Some(&mut children[index as usize]),
            StatelessNode::Leaf { .. } => None,
            StatelessNode::Empty(_) => None,
        }
    }

    /// Get a reference to a child at the given index (internal nodes only).
    pub fn child(&self, index: u8) -> Option<&StatelessNode> {
        match self {
            StatelessNode::Internal { children, .. } => {
                children[index as usize].as_ref().map(|boxed| boxed.as_ref())
            }
            StatelessNode::Leaf { .. } => None,
            StatelessNode::Empty(_) => None,
        }
    }
}

/// Compute C1/C2 commitments from leaf values.
///
/// Matches go-verkle's leaf commitment computation:
/// - C1 polynomial: suffixes 0-127 at indices [2*i, 2*i+1]
/// - C2 polynomial: suffixes 128-255 at indices [2*(i-128), 2*(i-128)+1]
pub fn compute_c1_c2(
    values: &[Option<[u8; 32]>; 256],
) -> Result<(BandersnatchPoint, BandersnatchPoint), VerkleProofError> {
    let mut c1_poly = [Fr::zero(); 256];
    let mut c2_poly = [Fr::zero(); 256];

    for (suffix, value) in values.iter().enumerate() {
        if let Some(v) = value {
            let (lo, hi) = encode_value_to_field(Some(*v));
            if suffix < 128 {
                c1_poly[suffix * 2] = lo;
                c1_poly[suffix * 2 + 1] = hi;
            } else {
                c2_poly[(suffix - 128) * 2] = lo;
                c2_poly[(suffix - 128) * 2 + 1] = hi;
            }
        }
    }

    let srs = generate_srs_points(256);
    let c1 = BandersnatchPoint::msm(&srs, &c1_poly);
    let c2 = BandersnatchPoint::msm(&srs, &c2_poly);

    Ok((c1, c2))
}

/// A stateless Verkle tree for proof verification.
///
/// Mirrors go-verkle's `PreStateTreeFromProof` - rebuilds tree structure from
/// proof commitments and witness values, enabling per-evaluation-point commitment
/// extraction.
#[derive(Debug)]
pub struct StatelessVerkleTree {
    root: StatelessNode,
    stem_info: alloc::vec::Vec<StemPathInfo>,
}

impl StatelessVerkleTree {
    /// Create a new stateless tree with the given root commitment.
    pub fn new(root_commitment: BandersnatchPoint) -> Self {
        Self {
            root: StatelessNode::new_internal(0, root_commitment),
            stem_info: alloc::vec::Vec::new(),
        }
    }

    /// Get the root commitment.
    pub fn root_commitment(&self) -> &BandersnatchPoint {
        self.root.commitment()
    }

    /// Access path-aligned stem info captured during reconstruction.
    pub fn stem_info(&self) -> &[StemPathInfo] {
        &self.stem_info
    }

    /// Create a path in the tree for the given stem.
    ///
    /// **Algorithm** (Question #1 resolution):
    /// - Use prefix caching to avoid consuming commitments for shared paths
    /// - Consume commitments only when creating NEW internal nodes or leaves
    /// - Track cursor position in `commitments` array
    ///
    /// **Inputs**:
    /// - `stem`: 31-byte stem prefix
    /// - `depth`: Number of internal nodes (from `depth_extension_present`)
    /// - `commitments`: Slice of commitments for this path (internal nodes + leaf)
    /// - `values`: Leaf values (256 suffix slots)
    /// - `prefix_cache`: Shared cache mapping prefix → existing node
    /// - `expected_commitments`: Expected number of commitments for this stem (for pruned proof validation)
    ///
    /// **Returns**: Number of commitments consumed
    pub fn create_path(
        &mut self,
        path_stem: &[u8; 31],
        depth: u8,
        stem_type: u8,
        stem: &[u8; 31],
        commitments: &[[u8; 32]],
        values: [Option<[u8; 32]>; 256],
        has_c1: bool,
        has_c2: bool,
        prefix_cache: &mut alloc::collections::BTreeMap<Vec<u8>, ()>,
        expected_commitments: usize,
    ) -> Result<usize, VerkleProofError> {
        utils::functions::log_info(&alloc::format!(
            "    🛤️  create_path: depth={}, stem_type={}, commitments_available={}",
            depth,
            stem_type,
            commitments.len()
        ));

        let mut cursor = &mut self.root;
        let mut commit_idx = 0usize;

        // Navigate through internal nodes (0..depth)
        utils::functions::log_info(&alloc::format!("      Navigating {} levels", depth));
        for level in 0..depth {
            utils::functions::log_info(&alloc::format!("        Level {}/{}", level, depth));
            let child_index = path_stem[level as usize];
            let prefix = path_stem[0..=(level as usize)].to_vec();

            match cursor.child(child_index) {
                Some(_) => {
                    // Child exists - navigate to it (no commitment consumed)
                    cursor = match cursor.child_mut(child_index) {
                        Some(Some(child_box)) => child_box.as_mut(),
                        _ => return Err(VerkleProofError::PathMismatch),
                    };
                }
                None => {
                    // No child exists - create new internal node and consume commitment
                    if commit_idx >= commitments.len() {
                        return Err(VerkleProofError::PathMismatch);
                    }
                    let commitment = BandersnatchPoint::from_bytes(&commitments[commit_idx])
                        .map_err(|_| VerkleProofError::InvalidPoint)?;
                    commit_idx += 1;

                    let new_node = StatelessNode::new_internal(level + 1, commitment);

                    match cursor.child_mut(child_index) {
                        Some(child_slot) => {
                            *child_slot = Some(alloc::boxed::Box::new(new_node));
                            cursor = child_slot.as_mut().unwrap().as_mut();
                        }
                        None => return Err(VerkleProofError::PathMismatch),
                    }

                    prefix_cache.insert(prefix, ());
                }
            }
        }

        // For ABSENT_EMPTY, mark the child as explicitly empty (no leaf commitment consumed).
        if stem_type == EXT_STATUS_ABSENT_EMPTY {
            let leaf_index = path_stem[depth as usize];
            match cursor.child_mut(leaf_index) {
                Some(child_slot) => {
                    *child_slot = Some(alloc::boxed::Box::new(StatelessNode::Empty(
                        BandersnatchPoint::identity(),
                    )));
                }
                None => return Err(VerkleProofError::PathMismatch),
            }
            return Ok(commit_idx);
        }

        // Create leaf node (must consume leaf commitment from proof)
        if commit_idx >= commitments.len() {
            #[cfg(test)]
            {
                extern crate std;
                std::println!("ERROR: create_path leaf: commit_idx={} >= commitments.len()={}", commit_idx, commitments.len());
            }
            return Err(VerkleProofError::PathMismatch);
        }
        let leaf_commitment = BandersnatchPoint::from_bytes(&commitments[commit_idx])
            .map_err(|_| VerkleProofError::InvalidPoint)?;
        commit_idx += 1;

        let is_poa_stub = stem_type == EXT_STATUS_ABSENT_OTHER;

        // Determine if C1/C2 are in the proof based on expected_commitments budget.
        // Expected commitments: 1 (leaf) + (has_c1 ? 1 : 0) + (has_c2 ? 1 : 0)
        // If expected_commitments matches, commitments are in proof (non-pruned).
        // If expected_commitments = 1 (only leaf), proof is pruned - compute C1/C2 from values.
        #[cfg(test)]
        let mut expected_c1_c2_count = 0;
        #[cfg(test)]
        if has_c1 {
            expected_c1_c2_count += 1;
        }
        #[cfg(test)]
        if has_c2 {
            expected_c1_c2_count += 1;
        }
        let is_pruned = expected_commitments == 1; // Only leaf commitment provided

        #[cfg(test)]
        {
            extern crate std;
            if expected_commitments != 1 + expected_c1_c2_count {
                std::println!("WARNING: expected_commitments={} but calculated 1 + {} = {}",
                    expected_commitments, expected_c1_c2_count, 1 + expected_c1_c2_count);
            }
        }

        // Consume C1 commitment if has_c1 is true
        let c1 = if has_c1 {
            if !is_pruned && commit_idx < commitments.len() {
                let c1_commitment = BandersnatchPoint::from_bytes(&commitments[commit_idx])
                    .map_err(|_| VerkleProofError::InvalidPoint)?;
                commit_idx += 1;
                c1_commitment
            } else {
                // C1 not in proof - compute from values (pruned proof)
                let (computed_c1, _) = compute_c1_c2(&values)?;
                computed_c1
            }
        } else {
            // Identity point for unused C1 (matching go-verkle: newchild.c1 = new(Point))
            BandersnatchPoint::identity()
        };

        // Consume C2 commitment if has_c2 is true
        let c2 = if has_c2 {
            if !is_pruned && commit_idx < commitments.len() {
                let c2_commitment = BandersnatchPoint::from_bytes(&commitments[commit_idx])
                    .map_err(|_| VerkleProofError::InvalidPoint)?;
                commit_idx += 1;
                c2_commitment
            } else {
                // C2 not in proof - compute from values (pruned proof)
                let (_, computed_c2) = compute_c1_c2(&values)?;
                computed_c2
            }
        } else {
            // Identity point for unused C2
            BandersnatchPoint::identity()
        };

        let leaf_node =
            StatelessNode::new_leaf(*stem, leaf_commitment, values, c1, c2, is_poa_stub)?;

        // Attach leaf to parent
        let leaf_index = path_stem[depth as usize];
        match cursor.child_mut(leaf_index) {
            Some(child_slot) => {
                *child_slot = Some(alloc::boxed::Box::new(leaf_node));
            }
            None => {
                #[cfg(test)]
                {
                    extern crate std;
                    std::println!("ERROR: create_path attach leaf: cursor.child_mut({}) returned None (cursor is likely a Leaf, not Internal)", leaf_index);
                }
                return Err(VerkleProofError::PathMismatch);
            }
        }

        Ok(commit_idx)
    }

    /// Get a reference to the leaf node for the given stem.
    ///
    /// Navigates the tree following the stem bytes as child indices.
    /// Since depth varies per stem, we navigate until we find a leaf.
    pub fn get_leaf(&self, stem: &[u8; 31]) -> Result<&StatelessNode, VerkleProofError> {
        let mut cursor = &self.root;

        // Navigate through the tree following stem bytes until we find a leaf
        for (_level, &byte) in stem.iter().enumerate() {
            match cursor {
                StatelessNode::Leaf { stem: leaf_stem, .. } => {
                    // Found a leaf - verify it matches our stem
                    if leaf_stem == stem {
                        return Ok(cursor);
                    } else {
                        return Err(VerkleProofError::PathMismatch);
                    }
                }
                StatelessNode::Internal { .. } => {
                    // Continue navigating
                    match cursor.child(byte) {
                        Some(child) => cursor = child,
                        None => return Err(VerkleProofError::PathMismatch),
                    }
                }
                StatelessNode::Empty(_) => {
                    return Err(VerkleProofError::PathMismatch);
                }
            }
        }

        // After navigating all 31 levels, we should have a leaf
        match cursor {
            StatelessNode::Leaf { stem: leaf_stem, .. } => {
                if leaf_stem == stem {
                    Ok(cursor)
                } else {
                    Err(VerkleProofError::PathMismatch)
                }
            }
            StatelessNode::Empty(_) => Err(VerkleProofError::PathMismatch),
            _ => Err(VerkleProofError::PathMismatch),
        }
    }

    /// Get the commitment of the root child at the given index.
    pub fn get_child_commitment_at_index(
        &self,
        index: u8,
    ) -> Result<BandersnatchPoint, VerkleProofError> {
        match &self.root {
            StatelessNode::Internal { children, .. } => {
                match &children[index as usize] {
                    Some(child) => Ok(child.commitment().clone()),
                    None => Err(VerkleProofError::PathMismatch),
                }
            }
            _ => Err(VerkleProofError::PathMismatch),
        }
    }
}

/// Stem metadata used for rebuilding stateless trees from proof commitments.
#[derive(Clone, Debug)]
pub struct StemInfo {
    pub depth: u8,
    pub stem_type: u8,
    pub stem: [u8; 31],
    pub values: [Option<[u8; 32]>; 256],
    pub is_absence: bool,
    pub has_c1: bool,  // True if any suffix < 128 has a value
    pub has_c2: bool,  // True if any suffix >= 128 has a value
}

/// Path stem used for navigation (may differ from requested stem for absence proofs).
#[derive(Clone, Debug)]
pub struct StemPathInfo {
    pub path_stem: [u8; 31],
    pub info: StemInfo,
}

/// Extract unique stems (31-byte prefixes) from witness entries, preserving witness order.
///
/// **CRITICAL**: Extracts ALL unique stems (present + absent) to match go-verkle's behavior.
///
/// **Why include all stems**: go-verkle generates one extension status byte per unique stem,
/// including absent keys. The proof covers all stems (using absent-empty or absent-other status),
/// not just present ones.
///
/// **Ordering**: Preserves the order stems first appear in the witness. Go-verkle sorts keys
/// before proving, so we trust the witness arrives in proof order.
///
/// **Deduplication**: First occurrence of each stem wins (matches go-verkle's deduplicate logic).
fn extract_requested_stems(
    entries: &[crate::verkle::VerkleWitnessEntry],
) -> alloc::vec::Vec<[u8; 31]> {
    let mut seen = alloc::collections::BTreeSet::new();
    let mut stems = alloc::vec::Vec::new();

    for entry in entries {
        let mut stem = [0u8; 31];
        stem.copy_from_slice(&entry.key[..31]);
        if seen.insert(stem) {
            // First time seeing this stem - preserve witness order
            stems.push(stem);
        }
    }

    stems
}

/// Build a values array for each present stem (absence stems remain all-None).
fn build_values_by_stem(
    entries: &[crate::verkle::VerkleWitnessEntry],
    use_pre_value: bool,
) -> alloc::collections::BTreeMap<[u8; 31], [Option<[u8; 32]>; 256]> {
    use alloc::collections::BTreeMap;

    let mut map: BTreeMap<[u8; 31], [Option<[u8; 32]>; 256]> = BTreeMap::new();

    for entry in entries {
        let mut stem = [0u8; 31];
        stem.copy_from_slice(&entry.key[..31]);
        let suffix = entry.key[31] as usize;

        let selected = if use_pre_value {
            entry.pre_value
        } else {
            entry.post_value
        };

        let values = map.entry(stem).or_insert([None; 256]);
        values[suffix] = Some(selected);  // Preserve zero-valued entries (marker bit matters)
    }

    map
}

/// Build per-stem info (depth + values) from proof metadata.
///
/// **CRITICAL**: This function mirrors go-verkle's PreStateTreeFromProof (proof_ipa.go:456-584)
/// and MUST match its behavior exactly, including:
/// - pathsWithExtPresent guard to skip duplicate absent-other paths
/// - poaStems consumption order and validation
/// - Enforcing nil values for absence proofs
/// - Setting has_c1/has_c2 for present stems
///
/// **Algorithm** (from go-verkle lines 485-552):
/// 1. Build pathsWithExtPresent cache (line 486-493)
/// 2. For each (stem, extStatus) pair (line 496):
///    - Decode depth and stemType (line 498-499)
///    - path = stem[:depth] (line 501)
///    - Switch on stemType:
///      a. ABSENT_EMPTY: validate nil values, add to info (line 503-510)
///      b. ABSENT_OTHER: check pathsWithExtPresent, check info cache, consume poaStems (line 511-536)
///      c. PRESENT: populate values/has_c1/has_c2, add to info (line 537-546)
/// 3. Validate all poaStems consumed (line 554-556)
pub fn build_stem_info_map(
    stems: &[[u8; 31]],
    depth_extension_present: &[u8],
    other_stems: &[[u8; 32]],
    entries: &[crate::verkle::VerkleWitnessEntry],
    use_pre_value: bool,
) -> Result<alloc::vec::Vec<StemPathInfo>, VerkleProofError> {
    utils::functions::log_info(&alloc::format!(
        "📋 ENTRY build_stem_info_map: stems={}, depths={}, others={}, entries={}",
        stems.len(), depth_extension_present.len(), other_stems.len(), entries.len()
    ));

    // Line 470-472: validate stems.len() == proof.ExtStatus.len()
    if stems.len() != depth_extension_present.len() {
        utils::functions::log_error("❌ Length mismatch!");
        return Err(VerkleProofError::LengthMismatch);
    }

    // Line 477: poas = proof.PoaStems
    let mut poas: alloc::collections::VecDeque<[u8; 31]> = other_stems
        .iter()
        .map(|s| {
            let mut stem = [0u8; 31];
            stem.copy_from_slice(&s[..31]);
            stem
        })
        .collect();

    // Line 481-483: validate poas are sorted
    for i in 1..poas.len() {
        if poas[i - 1] > poas[i] {
            return Err(VerkleProofError::PathMismatch);
        }
    }

    // Line 486-493: build pathsWithExtPresent cache
    let mut paths_with_ext_present = alloc::collections::BTreeSet::<alloc::vec::Vec<u8>>::new();
    for (i, es) in depth_extension_present.iter().enumerate() {
        if es & 3 == EXT_STATUS_PRESENT {
            let depth = (es >> 3) as usize;
            paths_with_ext_present.insert(stems[i][..depth].to_vec());
        }
    }
    utils::functions::log_info(&alloc::format!(
        "  ✅ Built pathsWithExtPresent cache: {} entries",
        paths_with_ext_present.len()
    ));

    // Line 474: info = map[string]stemInfo{}
    let mut info = alloc::collections::BTreeMap::<alloc::vec::Vec<u8>, StemInfo>::new();
    // Line 475: paths [][]byte
    let mut paths = alloc::vec::Vec::<alloc::vec::Vec<u8>>::new();

    // Line 496-552: for i, es := range proof.ExtStatus
    for (i, es) in depth_extension_present.iter().enumerate() {
        // Line 497-500: decode stemInfo
        let depth = es >> 3;
        let stem_type = es & 3;
        let path = stems[i][..(depth as usize)].to_vec();

        let mut si = StemInfo {
            stem: [0u8; 31],
            depth,
            stem_type,
            has_c1: false,
            has_c2: false,
            values: [None; 256],
            is_absence: stem_type != EXT_STATUS_PRESENT,
        };

        // Line 502-549: switch si.stemType
        match stem_type {
            // Line 503-510: case extStatusAbsentEmpty
            EXT_STATUS_ABSENT_EMPTY => {
                // Validate all keys with this prefix have nil values (line 506-509)
                for entry in entries {
                    let mut entry_stem = [0u8; 31];
                    entry_stem.copy_from_slice(&entry.key[..31]);
                    if entry_stem == stems[i] {
                        let selected_value = if use_pre_value {
                            entry.pre_value
                        } else {
                            entry.post_value
                        };
                        if !selected_value.iter().all(|b| *b == 0) {
                            return Err(VerkleProofError::PathMismatch);
                        }
                    }
                }
                // si.stem remains zero (no actual stem for absent-empty)
            }

            // Line 511-536: case extStatusAbsentOther
            EXT_STATUS_ABSENT_OTHER => {
                // Validate all keys with this prefix have nil values (line 514-518)
                for entry in entries {
                    let mut entry_stem = [0u8; 31];
                    entry_stem.copy_from_slice(&entry.key[..31]);
                    if entry_stem == stems[i] {
                        let selected_value = if use_pre_value {
                            entry.pre_value
                        } else {
                            entry.post_value
                        };
                        if !selected_value.iter().all(|b| *b == 0) {
                            return Err(VerkleProofError::PathMismatch);
                        }
                    }
                }

                // Line 524-526: if pathsWithExtPresent[path], continue
                if paths_with_ext_present.contains(&path) {
                    utils::functions::log_info(&alloc::format!(
                        "    ⏭️  Skipping ABSENT_OTHER path (has present): {:02x?}",
                        &path
                    ));
                    continue;
                }

                // Line 531-533: if info[path] already exists, continue
                if info.contains_key(&path) {
                    utils::functions::log_info(&alloc::format!(
                        "    ⏭️  Skipping ABSENT_OTHER path (already in info): {:02x?}",
                        &path
                    ));
                    continue;
                }

                // Line 535-536: consume poas[0]
                si.stem = poas.pop_front().ok_or(VerkleProofError::PathMismatch)?;
            }

            // Line 537-546: case extStatusPresent
            EXT_STATUS_PRESENT => {
                si.stem = stems[i];
                // Line 540-545: populate values and has_c1/has_c2
                for entry in entries {
                    let mut entry_stem = [0u8; 31];
                    entry_stem.copy_from_slice(&entry.key[..31]);
                    if entry_stem == si.stem {
                        let suffix = entry.key[31];
                        let selected_value = if use_pre_value {
                            entry.pre_value
                        } else {
                            entry.post_value
                        };
                        si.has_c1 = si.has_c1 || (suffix < 128);
                        si.has_c2 = si.has_c2 || (suffix >= 128);

                        let is_zero = selected_value.iter().all(|b| *b == 0);
                        let is_absent = matches!(
                            entry.metadata.key_type,
                            crate::verkle::VerkleKeyType::CodeHash
                        ) && is_zero;
                        if !is_absent {
                            si.values[suffix as usize] = Some(selected_value);
                        }
                    }
                }
            }

            _ => {
                return Err(VerkleProofError::PathMismatch);
            }
        }

        // Line 550-551: info[path] = si, paths = append(paths, path)
        info.insert(path.clone(), si);
        paths.push(path);
    }

    // Line 554-556: validate all poas consumed
    if !poas.is_empty() {
        return Err(VerkleProofError::PathMismatch);
    }

    utils::functions::log_info(&alloc::format!(
        "✅ build_stem_info_map complete: {} paths processed",
        paths.len()
    ));

    // Convert to Vec<StemPathInfo> for compatibility with rebuild_stateless_tree_from_proof
    let mut result = alloc::vec::Vec::with_capacity(paths.len());
    for path in paths {
        let si = info.get(&path).unwrap();

        // Convert encoded depth to internal depth (go-verkle's depth includes leaf, we use internal nodes only)
        let depth_internal = if si.depth > 0 { si.depth - 1 } else { 0 };

        // path_stem is the actual stem for navigation (from si.stem, NOT from the path bytes)
        let mut path_stem = [0u8; 31];
        path_stem.copy_from_slice(&si.stem);

        result.push(StemPathInfo {
            path_stem,
            info: StemInfo {
                depth: depth_internal,
                stem_type: si.stem_type,
                stem: si.stem,
                values: si.values,
                is_absence: si.stem_type != EXT_STATUS_PRESENT,
                has_c1: si.has_c1,
                has_c2: si.has_c2,
            },
        });
    }

    Ok(result)
}

/// **Step 1 of 3-step verification**: Rebuild a stateless Verkle tree from proof commitments and witness values.
///
/// This function mirrors go-verkle's `PreStateTreeFromProof` function, which is the foundation
/// of the correct verification approach. It reconstructs the tree structure using:
/// - **Tree Structure**: From proof's `commitments_by_path`
/// - **Values**: From witness entries
/// - **Result**: A stateless tree that can compute polynomial commitments for any evaluation point
///
/// ## Algorithm
///
/// 1. **Extract stems**: Collect stems from witness entries only (proof order). `other_stems`
///    are consumed on-demand when `EXT_STATUS_ABSENT_OTHER` is encountered during rebuilding.
/// 2. **Build stem info**: Map each requested stem to its depth, values, and presence/absence status
/// 3. **Create tree**: Initialize with root commitment from `commitments_by_path[0]`
/// 4. **Shared prefix optimization**: Reuse internal nodes when stems share prefixes
/// 5. **Advance-only consumption**: Only consume new commitments when creating new nodes
/// 6. **Validation**: Ensure all commitments are consumed (reject unused tail)
///
/// ## Key Features
///
/// - **Go-verkle compatibility**: Matches `PreStateTreeFromProof` algorithm exactly
/// - **Shared prefix handling**: Stems with common bytes reuse internal node commitments
/// - **Absence proof support**: `other_stems` create structural placeholders with no values
/// - **Commitment validation**: Rejects proofs with unused commitments (security)
///
/// Use with `extract_proof_elements_from_tree` and `verify_ipa_multiproof_with_evals` for complete verification.
pub fn rebuild_stateless_tree_from_proof(
    commitments_by_path: &[[u8; 32]],
    depth_extension_present: &[u8],
    other_stems: &[[u8; 32]],
    entries: &[crate::verkle::VerkleWitnessEntry],
    use_pre_value: bool,
) -> Result<StatelessVerkleTree, VerkleProofError> {
    if commitments_by_path.is_empty() {
        return Err(VerkleProofError::EmptyCommitments);
    }

    // CRITICAL: Preserve proof order. The depth_extension_present array is aligned with
    // the proof key order, so we must keep stems in that same order.
    let stems = extract_requested_stems(entries);

    utils::functions::log_info(&alloc::format!(
        "📊 RUST WITNESS: entries={}, unique_stems={}, depth_bytes={}",
        entries.len(),
        stems.len(),
        depth_extension_present.len()
    ));

    if !entries.is_empty() {
        utils::functions::log_info(&alloc::format!(
            "  📌 entry[0].key = {:02x?}",
            entries[0].key
        ));
    }
    if !stems.is_empty() {
        utils::functions::log_info(&alloc::format!(
            "  📌 stem[0] = {:02x?}...",
            &stems[0][..8]
        ));
    }

    if stems.len() != depth_extension_present.len() {
        utils::functions::log_error(&alloc::format!(
            "❌ MISMATCH: stems.len()={} != depth_extension_present.len()={}",
            stems.len(), depth_extension_present.len()
        ));
        return Err(VerkleProofError::LengthMismatch);
    }

    utils::functions::log_info("📋 Calling build_stem_info_map...");
    let stem_info = build_stem_info_map(
        &stems,
        depth_extension_present,
        other_stems,
        entries,
        use_pre_value,
    )?;
    utils::functions::log_info(&alloc::format!(
        "✅ build_stem_info_map complete: {} stems processed",
        stem_info.len()
    ));

    let root_commitment = BandersnatchPoint::from_bytes(&commitments_by_path[0])
        .map_err(|_| VerkleProofError::InvalidPoint)?;
    let mut tree = StatelessVerkleTree::new(root_commitment);

    // Compute the minimum commitment budget implied by the navigation paths:
    //   - Root commitment (always present)
    //   - One commitment per UNIQUE internal prefix (consumed when first created)
    //   - One commitment per NON-empty leaf (absent-empty paths consume 0 leaf commitments)
    //
    // This mirrors go-verkle's PreStateTreeFromProof consumption pattern and correctly
    // handles pruned proofs that still include internal node commitments.
    let mut internal_prefixes = alloc::collections::BTreeSet::<alloc::vec::Vec<u8>>::new();
    for path_info in &stem_info {
        for level in 0..path_info.info.depth {
            internal_prefixes.insert(path_info.path_stem[..=(level as usize)].to_vec());
        }
    }
    let leaf_commitments_needed = stem_info
        .iter()
        .filter(|info| info.info.stem_type != EXT_STATUS_ABSENT_EMPTY)
        .count();
    let minimal_commitments = 1 /* root */ + internal_prefixes.len() + leaf_commitments_needed;
    if commitments_by_path.len() < minimal_commitments {
        return Err(VerkleProofError::PathMismatch);
    }

    // Pre-calculate expected commitment count for each stem (leaf + optional C1/C2).
    // Internal node commitments are NOT counted per-stem because they may be shared via prefix_cache.
    let expected_per_stem: alloc::vec::Vec<usize> = stem_info
        .iter()
        .map(|path_info| {
            let info = &path_info.info;
            let mut count = if info.stem_type == EXT_STATUS_ABSENT_EMPTY {
                0 // absent-empty paths do not consume a leaf commitment
            } else {
                1 // leaf commitment
            };
            if info.has_c1 {
                count += 1;
            }
            if info.has_c2 {
                count += 1;
            }
            count
        })
        .collect();

    let total_c1_c2: usize = stem_info
        .iter()
        .map(|info| {
            let mut count = 0;
            if info.info.has_c1 {
                count += 1;
            }
            if info.info.has_c2 {
                count += 1;
            }
            count
        })
        .sum();

    // Determine if this is a pruned proof.
    // Pruned proof: only root + internals + leaves (no C1/C2 commitments in the proof).
    // Full proof: includes C1/C2 commitments for present stems.
    let is_pruned_proof = commitments_by_path.len() == minimal_commitments;
    if !is_pruned_proof && commitments_by_path.len() != minimal_commitments + total_c1_c2 {
        // Missing commitments for some C1/C2 slices or extra garbage at the tail.
        return Err(VerkleProofError::PathMismatch);
    }

    #[cfg(test)]
    {
        extern crate std;
        std::println!(
            "Commitment budget: minimal={} total_commitments={} stems={} is_pruned={} total_c1_c2={}",
            minimal_commitments,
            commitments_by_path.len(),
            stem_info.len(),
            is_pruned_proof,
            total_c1_c2
        );
    }

    let mut commit_idx = 1usize; // skip root
    let mut prefix_cache = alloc::collections::BTreeMap::new();

    utils::functions::log_info(&alloc::format!(
        "🔨 Starting tree building: {} stems, {} commitments",
        stem_info.len(),
        commitments_by_path.len()
    ));

    for (stem_idx, path_info) in stem_info.iter().enumerate() {
        let info = &path_info.info;

        utils::functions::log_info(&alloc::format!(
            "  🌿 Processing stem {}/{}: depth={}, type={}, has_c1={}, has_c2={}",
            stem_idx + 1,
            stem_info.len(),
            info.depth,
            info.stem_type,
            info.has_c1,
            info.has_c2
        ));

        if commit_idx >= commitments_by_path.len() {
            utils::functions::log_error("❌ Ran out of commitments");
            return Err(VerkleProofError::PathMismatch);
        }

        let expected_commitments = if is_pruned_proof {
            1 // Pruned proof: only leaf commitment, compute C1/C2
        } else {
            expected_per_stem[stem_idx]
        };

        let consumed = tree.create_path(
            &path_info.path_stem,
            info.depth,
            info.stem_type,
            &info.stem,
            &commitments_by_path[commit_idx..],
            info.values,
            info.has_c1,
            info.has_c2,
            &mut prefix_cache,
            expected_commitments,
        )?;

        utils::functions::log_info(&alloc::format!(
            "    ✅ Consumed {} commitments, total used: {}",
            consumed,
            commit_idx + consumed
        ));

        commit_idx += consumed;
    }

    if commit_idx != commitments_by_path.len() {
        return Err(VerkleProofError::UnusedCommitments);
    }

    tree.stem_info = stem_info;

    Ok(tree)
}

/// Proof elements extracted from rebuilt stateless tree (one commitment per evaluation point).
pub struct ProofElements {
    pub commitments: alloc::vec::Vec<BandersnatchPoint>,
    pub indices: alloc::vec::Vec<u8>,
    pub values: alloc::vec::Vec<Fr>,
}

/// Extract per-evaluation-point commitments from the rebuilt tree (go-verkle compatible).
/// **Step 2 of 3-step verification**: Mirrors go-verkle's `GetProofItems` breadth-first walk.
///
/// ## Algorithm
/// 1. Sort witness keys lexicographically (Keylist sort from go-verkle).
/// 2. Traverse the stateless tree breadth-first:
///    - **Internal nodes**: group keys by `key[depth]`, append `(parent_commitment, child_idx, child_commitment_scalar)` for each group, then recurse into non-empty children in group order.
///    - **Leaf nodes**: always emit base entries `(z=0 -> 1, z=1 -> stem)`, optionally emit C1/C2 pointers if that stem appears in `keys`, then emit suffix evaluations for matching stems using `(z=2*suffix, 2*suffix+1)` with C1 for suffix < 128 and C2 for suffix ≥ 128 (wrapping multiplication to match go-verkle).
/// 3. Arrays remain aligned: `len(commitments) == len(indices) == len(values)`.
///
/// Use with `rebuild_stateless_tree_from_proof` and `verify_ipa_multiproof_with_evals` for complete verification.
pub fn extract_proof_elements_from_tree(
    tree: &StatelessVerkleTree,
    entries: &[crate::verkle::VerkleWitnessEntry],
    use_pre_value: bool,
) -> Result<ProofElements, VerkleProofError> {
    // go-verkle sorts the proven keys before extracting proof items (Keylist sort).
    let mut keys: alloc::vec::Vec<[u8; 32]> = entries.iter().map(|e| e.key).collect();
    keys.sort();

    // Debug: surface the root-child distribution derived from the witness keys.
    let key0_bytes: alloc::vec::Vec<u8> = keys.iter().map(|k| k[0]).collect();
    let mut unique_key0 = key0_bytes.clone();
    unique_key0.dedup();
    utils::functions::log_info(&alloc::format!(
        "    🔑 key[0] bytes (sorted): {:?} | unique: {:?}",
        key0_bytes, unique_key0
    ));

    // Debug: group keys by stem to mirror Go's keylist grouping
    use alloc::collections::BTreeMap;
    let mut stem_map: BTreeMap<[u8; 4], alloc::vec::Vec<u8>> = BTreeMap::new();
    for k in keys.iter() {
        let mut stem4 = [0u8; 4];
        stem4.copy_from_slice(&k[..4]);
        stem_map.entry(stem4).or_default().push(k[31]);
    }
    for (_stem, suffixes) in stem_map.iter_mut() {
        suffixes.sort();
    }
    for (stem, suffixes) in stem_map.iter() {
        utils::functions::log_info(&alloc::format!(
            "    🔑 Rust proof keys (stem → suffixes): {:02x?} -> {:?}",
            stem,
            suffixes
        ));
    }

    let mut commitments = alloc::vec::Vec::new();
    let mut indices = alloc::vec::Vec::new();
    let mut values = alloc::vec::Vec::new();

    // Group keys by the byte at the current depth (matches go-verkle's groupKeys).
    fn group_keys<'a>(
        keys: &'a [[u8; 32]],
        depth: usize,
    ) -> alloc::vec::Vec<&'a [[u8; 32]]> {
        let mut groups = alloc::vec::Vec::new();
        if keys.is_empty() {
            return groups;
        }

        let mut start = 0usize;
        for i in 1..keys.len() {
            if keys[i][depth] != keys[i - 1][depth] {
                groups.push(&keys[start..i]);
                start = i;
            }
        }
        groups.push(&keys[start..]);

        groups
    }

    // Breadth-first extraction that mirrors go-verkle's GetProofItems traversal.
    fn traverse(
        node: &StatelessNode,
        depth: usize,
        keys: &[[u8; 32]],
        commitments: &mut alloc::vec::Vec<BandersnatchPoint>,
        indices: &mut alloc::vec::Vec<u8>,
        values: &mut alloc::vec::Vec<Fr>,
    ) -> Result<(), VerkleProofError> {
        match node {
            StatelessNode::Internal {
                children,
                commitment,
                ..
            } => {
                let groups = group_keys(keys, depth);

                // Build polynomial fi[256] with scalar mappings of ALL children (go-verkle line 957-991)
                // This mirrors InternalNode.GetProofItems exactly:
                // 1. Create points[256] array with child commitments (identity for missing)
                // 2. BatchMapToScalarField(fiPtrs[:], points[:]) → converts all to scalars
                // 3. Extract y = fi[childIdx] for each group
                let mut fi = [Fr::from(0u64); 256];

                for (i, child_opt) in children.iter().enumerate() {
                    let point = if let Some(child_box) = child_opt {
                        child_box.commitment()
                    } else {
                        &BandersnatchPoint::identity()
                    };
                    fi[i] = point.map_to_scalar_field();
                }

                // Pass 1: emit this level's evaluations (one per grouped key byte).
                // Extract y from fi[childIdx] polynomial (go-verkle line 997-1003)
                for group in &groups {
                    let child_idx = group[0][depth] as usize;

                    commitments.push(commitment.clone());
                    indices.push(child_idx as u8);
                    values.push(fi[child_idx]);  // Extract from polynomial, not direct map!
                }

                // Pass 2: recurse into children in the same order (breadth-first).
                // ONLY recurse if child actually exists.
                for group in groups {
                    let child_idx = group[0][depth];

                    if let Some(child_box) = &children[child_idx as usize] {
                        match child_box.as_ref() {
                            StatelessNode::Empty(_) => continue,
                            _ => traverse(child_box.as_ref(), depth + 1, group, commitments, indices, values)?,
                        }
                    }
                    // If child doesn't exist, skip recursion (already emitted identity in Pass 1)
                }
            }
            StatelessNode::Leaf {
                stem,
                commitment,
                values: leaf_values,
                c1,
                c2,
                is_poa_stub,
            } => {
                // Debug: show which suffixes from the witness land in this leaf.
                let mut suffixes: alloc::vec::Vec<u8> = keys
                    .iter()
                    .filter(|k| k[..31] == stem[..])
                    .map(|k| k[31])
                    .collect();
                suffixes.sort();
                utils::functions::log_info(&alloc::format!(
                    "    🍃 Leaf stem={:02x?} suffixes={:?}",
                    &stem[..4],
                    suffixes
                ));

                // Determine presence of C1/C2 based on keys that actually belong to this stem.
                let mut has_c1 = false;
                let mut has_c2 = false;
                for key in keys.iter() {
                    if key[..31] == stem[..] {
                        if key[31] < 128 {
                            has_c1 = true;
                        } else {
                            has_c2 = true;
                        }
                    }
                }

                if *is_poa_stub && (has_c1 || has_c2) {
                    return Err(VerkleProofError::PathMismatch);
                }

                // Build polynomial poly[256] for leaf node (go-verkle line 1509-1535)
                // poly[0] = marker (1)
                // poly[1] = stem as Fr
                // poly[2] = C1 scalar mapping (if not POA stub)
                // poly[3] = C2 scalar mapping (if not POA stub)
                // poly[4+] = suffix values
                let mut poly = [Fr::from(0u64); 256];

                // Line 1510: poly[0].SetUint64(1)
                poly[0] = Fr::from(1u64);

                // Line 1511-1513: StemFromLEBytes(&poly[1], n.stem)
                let mut stem_bytes = [0u8; 32];
                stem_bytes[..31].copy_from_slice(stem);
                poly[1] = Fr::from_le_bytes_mod_order(&stem_bytes);

                // Line 1532-1535: Map C1/C2 to poly[2], poly[3] (if not POA stub)
                if !*is_poa_stub {
                    poly[2] = c1.map_to_scalar_field();
                    poly[3] = c2.map_to_scalar_field();
                }

                // Debug: show poly[0-3] values
                use ark_ff::BigInteger;
                let p0_bytes = poly[0].into_bigint().to_bytes_le();
                let p1_bytes = poly[1].into_bigint().to_bytes_le();
                let p2_bytes = poly[2].into_bigint().to_bytes_le();
                utils::functions::log_info(&alloc::format!(
                    "    🔬 Leaf poly: [0]={:02x?} [1]={:02x?} [2]={:02x?} (is_poa_stub={})",
                    &p0_bytes[..4],
                    &p1_bytes[..4],
                    &p2_bytes[..4],
                    is_poa_stub
                ));

                // Now emit base evaluations by extracting from poly[] (line 1497-1499)
                commitments.push(commitment.clone());
                indices.push(0);
                values.push(poly[0]);  // Extract from polynomial!

                commitments.push(commitment.clone());
                indices.push(1);
                values.push(poly[1]);  // Extract from polynomial!

                // Optional pointers to C1/C2 (line 1544-1555).
                if has_c1 {
                    commitments.push(commitment.clone());
                    indices.push(2);
                    values.push(poly[2]);  // Extract from polynomial!
                }
                if has_c2 {
                    commitments.push(commitment.clone());
                    indices.push(3);
                    values.push(poly[3]);  // Extract from polynomial!
                }

                // Suffix evaluations in key order (only for matching stems).
                for key in keys.iter() {
                    if key[..31] != stem[..] {
                        continue;
                    }

                    let suffix = key[31] as usize;
                    let (lo, hi) = encode_value_to_field(leaf_values[suffix]);

                    let z0 = (suffix as u8).wrapping_mul(2);
                    let z1 = z0.wrapping_add(1);

                    if suffix < 128 {
                        commitments.push(c1.clone());
                        indices.push(z0);
                        values.push(lo);

                        commitments.push(c1.clone());
                        indices.push(z1);
                        values.push(hi);
                    } else {
                        commitments.push(c2.clone());
                        indices.push(z0);
                        values.push(lo);

                        commitments.push(c2.clone());
                        indices.push(z1);
                        values.push(hi);
                    }
                }
            }
            StatelessNode::Empty(_) => return Err(VerkleProofError::PathMismatch),
        }

        Ok(())
    }

    // Traverse starting at the root (depth = 0) over all sorted keys.
    traverse(
        &tree.root,
        0,
        &keys,
        &mut commitments,
        &mut indices,
        &mut values,
    )?;

    // Debug: Log first 15 proof elements to compare with Go
    utils::functions::log_info(&alloc::format!(
        "    📊 Total proof elements: {} (C/z/y triples)",
        commitments.len()
    ));
    for i in 0..core::cmp::min(15, commitments.len()) {
        use ark_ff::BigInteger;
        let c_bytes = commitments[i].to_bytes();
        let y_bytes = values[i].into_bigint().to_bytes_le();
        utils::functions::log_info(&alloc::format!(
            "      [{}] z={:3} (0x{:02x}), C={:02x?}..., y={:02x?}...",
            i,
            indices[i],
            indices[i],
            &c_bytes[..4],
            &y_bytes[..4]
        ));
    }

    Ok(ProofElements {
        commitments,
        indices,
        values,
    })
}
