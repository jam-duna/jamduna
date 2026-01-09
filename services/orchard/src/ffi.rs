/// FFI (Foreign Function Interface) module for Orchard service
///
/// This module provides C-compatible functions that can be called from Go
/// to perform Halo2 proof generation, verification, and cryptographic operations.

use crate::crypto::{commitment, nullifier};
use crate::bundle_codec::{decode_issue_bundle, decode_swap_bundle, decode_zsa_bundle};
use crate::refiner::{parse_pre_state_payload, process_extrinsics_witness_aware};
use crate::nu7_types::OrchardBundleType;
use crate::state::OrchardExtrinsic;
use crate::witness::{WitnessBundle, StateRead};
use alloc::vec::Vec;
use alloc::string::ToString;
use core::slice;
use core::ptr;
use blake2::Blake2bVar;
use blake2::digest::{Update, VariableOutput};
use utils::functions::RefineContext;

/// Extrinsic type constants for FFI
#[repr(C)]
pub enum ExtrinsicType {
    DepositPublic = 0,
    SubmitPrivate = 1,
}

/// FFI Result codes
#[repr(C)]
pub enum FFIResult {
    Success = 0,
    InvalidInput = 1,
    ProofGenerationFailed = 2,
    VerificationFailed = 3,
    SerializationError = 4,
    InternalError = 5,
}

// Halo2 proof size in bytes, measured from services/orchard/keys/*.proof outputs.
// ❌ REMOVED: Mock proof size constant - no longer needed after removing insecure mock proof functions

/// Generate nullifier with explicit service ID
///
/// # Safety
/// All pointers must be valid for the specified lengths
#[no_mangle]
pub extern "C" fn orchard_nullifier_with_service_id(
    sk_spend: *const u8,      // 32 bytes
    rho: *const u8,           // 32 bytes
    commitment: *const u8,    // 32 bytes
    output: *mut u8,          // 32 bytes output buffer
    service_id: u32,          // Service ID for domain separation
) -> u32 {
    if sk_spend.is_null() || rho.is_null() || commitment.is_null() || output.is_null() {
        return FFIResult::InvalidInput as u32;
    }

    unsafe {
        // Convert pointers to arrays
        let sk_array = slice::from_raw_parts(sk_spend, 32);
        let rho_array = slice::from_raw_parts(rho, 32);
        let commitment_array = slice::from_raw_parts(commitment, 32);

        let mut sk_bytes = [0u8; 32];
        let mut rho_bytes = [0u8; 32];
        let mut commitment_bytes = [0u8; 32];

        sk_bytes.copy_from_slice(sk_array);
        rho_bytes.copy_from_slice(rho_array);
        commitment_bytes.copy_from_slice(commitment_array);

        // Generate nullifier with service ID for domain separation
        // Note: In a full implementation, service_id would be used for domain separation in the hash
        // For now, we use the existing nullifier function but this parameter allows future extension
        let _ = service_id; // Mark as used to avoid warnings
        match nullifier(&sk_bytes, &rho_bytes, &commitment_bytes) {
            Ok(result) => {
                let output_slice = slice::from_raw_parts_mut(output, 32);
                output_slice.copy_from_slice(&result);
                FFIResult::Success as u32
            }
            Err(_) => FFIResult::InternalError as u32,
        }
    }
}

/// Generate note commitment
///
/// # Safety
/// All pointers must be valid for the specified lengths

/// Generate note commitment with explicit service ID
///
/// # Safety
/// All pointers must be valid for the specified lengths
#[no_mangle]
pub extern "C" fn orchard_commitment_with_service_id(
    asset_id: u32,
    amount_lo: u64,
    amount_hi: u64,
    owner_pk: *const u8,      // 32 bytes
    rho: *const u8,           // 32 bytes
    note_rseed: *const u8,    // 32 bytes
    unlock_height: u64,
    memo_hash: *const u8,     // 32 bytes
    output: *mut u8,          // 32 bytes output buffer
    service_id: u32,          // Service ID for domain separation
) -> u32 {
    if owner_pk.is_null() || rho.is_null() || note_rseed.is_null()
        || memo_hash.is_null() || output.is_null() {
        return FFIResult::InvalidInput as u32;
    }

    unsafe {
        // Convert pointers to arrays
        let owner_pk_slice = slice::from_raw_parts(owner_pk, 32);
        let rho_slice = slice::from_raw_parts(rho, 32);
        let note_rseed_slice = slice::from_raw_parts(note_rseed, 32);
        let memo_hash_slice = slice::from_raw_parts(memo_hash, 32);

        let mut owner_pk_bytes = [0u8; 32];
        let mut rho_bytes = [0u8; 32];
        let mut note_rseed_bytes = [0u8; 32];
        let mut memo_hash_bytes = [0u8; 32];

        owner_pk_bytes.copy_from_slice(owner_pk_slice);
        rho_bytes.copy_from_slice(rho_slice);
        note_rseed_bytes.copy_from_slice(note_rseed_slice);
        memo_hash_bytes.copy_from_slice(memo_hash_slice);

        // Reconstruct full amount
        let amount = ((amount_hi as u128) << 64) | (amount_lo as u128);

        // Generate commitment with service ID for domain separation
        // Note: In a full implementation, service_id would be used for domain separation in the hash
        // For now, we use the existing commitment function but this parameter allows future extension
        let _ = service_id; // Mark as used to avoid warnings
        match commitment(
            asset_id,
            amount,
            &owner_pk_bytes,
            &rho_bytes,
            &note_rseed_bytes,
            unlock_height,
            &memo_hash_bytes,
        ) {
            Ok(result) => {
                let output_slice = slice::from_raw_parts_mut(output, 32);
                output_slice.copy_from_slice(&result);
                FFIResult::Success as u32
            }
            Err(_) => FFIResult::InternalError as u32,
        }
    }
}

/// Refine a work package in-process using witness-aware validation.
///
/// # Safety
/// All pointers must be valid for the specified lengths.
#[no_mangle]
pub extern "C" fn orchard_refine_witness_aware(
    service_id: u32,
    pre_state_payload_ptr: *const u8,
    pre_state_payload_len: u32,
    pre_witness_ptr: *const u8,
    pre_witness_len: u32,
    post_witness_ptr: *const u8,
    post_witness_len: u32,
    bundle_proof_ptr: *const u8,
    bundle_proof_len: u32,
) -> u32 {
    if pre_state_payload_ptr.is_null() || pre_state_payload_len == 0 {
        return FFIResult::InvalidInput as u32;
    }
    if pre_witness_ptr.is_null() || pre_witness_len == 0 {
        return FFIResult::InvalidInput as u32;
    }
    if bundle_proof_ptr.is_null() || bundle_proof_len == 0 {
        return FFIResult::InvalidInput as u32;
    }

    let pre_state_payload = unsafe {
        slice::from_raw_parts(pre_state_payload_ptr, pre_state_payload_len as usize)
    };
    let pre_state = match parse_pre_state_payload(pre_state_payload) {
        Ok(state) => state,
        Err(_) => return FFIResult::InvalidInput as u32,
    };

    let pre_witness_bytes = unsafe {
        slice::from_raw_parts(pre_witness_ptr, pre_witness_len as usize)
    };
    let pre_extrinsic = match OrchardExtrinsic::deserialize(pre_witness_bytes) {
        Ok(extrinsic) => extrinsic,
        Err(_) => return FFIResult::SerializationError as u32,
    };

    let mut extrinsics = Vec::new();
    extrinsics.push(pre_extrinsic);

    if post_witness_len > 0 {
        if post_witness_ptr.is_null() {
            return FFIResult::InvalidInput as u32;
        }
        let post_witness_bytes = unsafe {
            slice::from_raw_parts(post_witness_ptr, post_witness_len as usize)
        };
        let post_extrinsic = match OrchardExtrinsic::deserialize(post_witness_bytes) {
            Ok(extrinsic) => extrinsic,
            Err(_) => return FFIResult::SerializationError as u32,
        };
        extrinsics.push(post_extrinsic);
    }

    let bundle_proof_bytes = unsafe {
        slice::from_raw_parts(bundle_proof_ptr, bundle_proof_len as usize)
    };
    let bundle_extrinsic = match OrchardExtrinsic::deserialize(bundle_proof_bytes) {
        Ok(extrinsic) => extrinsic,
        Err(_) => return FFIResult::SerializationError as u32,
    };
    extrinsics.push(bundle_extrinsic);

    let refine_context = RefineContext {
        anchor: [0u8; 32],
        state_root: [0u8; 32],
        beefy_root: [0u8; 32],
        lookup_anchor: [0u8; 32],
        lookup_anchor_slot: 0,
        prerequisites: Vec::new(),
    };
    let work_package_hash = [0u8; 32];
    let refine_gas_limit = 10_000_000u64;

    match process_extrinsics_witness_aware(
        service_id,
        &refine_context,
        &work_package_hash,
        refine_gas_limit,
        &pre_state,
        &extrinsics,
        None,
    ) {
        Ok(_) => FFIResult::Success as u32,
        Err(_) => FFIResult::VerificationFailed as u32,
    }
}


/// Serialize witness bundle to JSON
///
/// # Safety
/// All pointers must be valid, witness_ptr must point to valid WitnessBundle
/// MEMORY-SAFE: Enhanced buffer size validation and bounds checking
#[no_mangle]
pub extern "C" fn orchard_serialize_witness(
    witness_ptr: *const WitnessBundle,
    output_buf: *mut u8,
    output_cap: u32,
) -> u32 {
    // SECURITY: Validate input parameters
    const MAX_OUTPUT_BUFFER_SIZE: u32 = 64 * 1024; // Max 64KB output

    if witness_ptr.is_null() || output_buf.is_null() {
        return 0; // Return 0 length on error
    }
    if output_cap == 0 || output_cap > MAX_OUTPUT_BUFFER_SIZE {
        return 0; // Invalid buffer size
    }

    unsafe {
        let witness = &*witness_ptr;

        // SECURITY: Validate witness data to prevent excessive memory usage
        if witness.reads.len() > 1000 || witness.writes.len() > 1000 {
            return 0; // Too many reads/writes, potential DoS
        }

        // Create a simplified JSON representation
        // In practice, this would use a proper JSON serialization library
        let json_str = format!(
            "{{\"pre_state_root\":\"{}\",\"post_state_root\":\"{}\",\"reads_count\":{},\"writes_count\":{}}}",
            hex::encode(&witness.pre_state_root),
            hex::encode(&witness.post_state_root),
            witness.reads.len(),
            witness.writes.len()
        );

        let json_bytes = json_str.as_bytes();

        // SECURITY: Double-check buffer size constraints
        if json_bytes.len() > output_cap as usize {
            return 0; // Buffer too small
        }
        if json_bytes.is_empty() {
            return 0; // Empty serialization
        }

        // SECURITY: Safe buffer write with exact size
        let output_slice = slice::from_raw_parts_mut(output_buf, json_bytes.len());
        output_slice.copy_from_slice(json_bytes);

        json_bytes.len() as u32
    }
}


// ❌ REMOVED: Mock proof functions completely bypassed cryptographic validation
// These functions were CRITICAL security vulnerabilities that allowed:
// - Complete proof forgery without cryptographic validation
// - Asset theft through fake spend proofs
// - Double-spending via predictable proof patterns
// - Total privacy breach (no real zero-knowledge guarantees)
//
// All mock proof functions have been removed to enforce real Halo2 proof validation.
// Production systems must use real cryptographic proof generation and verification.

/// Create witness bundle for Orchard extrinsic
/// This creates a basic witness structure for testing
///
/// # Safety
/// All pointers must be valid for the specified lengths
#[no_mangle]
pub extern "C" fn orchard_create_witness_bundle(
    _extrinsic_type: u32,
    pre_state_root: *const u8,    // 32 bytes
    post_state_root: *const u8,   // 32 bytes
    output_ptr: *mut WitnessBundle,
) -> u32 {
    if pre_state_root.is_null() || post_state_root.is_null() || output_ptr.is_null() {
        return FFIResult::InvalidInput as u32;
    }

    unsafe {
        let pre_root_slice = slice::from_raw_parts(pre_state_root, 32);
        let post_root_slice = slice::from_raw_parts(post_state_root, 32);

        let mut pre_root = [0u8; 32];
        let mut post_root = [0u8; 32];
        pre_root.copy_from_slice(pre_root_slice);
        post_root.copy_from_slice(post_root_slice);

        // Create a basic witness bundle for the extrinsic type
        let witness = WitnessBundle {
            pre_state_root: pre_root,
            post_state_root: post_root,
            reads: Vec::new(),
            writes: Vec::new(),
        };

        // Write the witness to the output pointer
        ptr::write(output_ptr, witness);
        FFIResult::Success as u32
    }
}

/// Add a state read to a witness bundle
///
/// # Safety
/// All pointers must be valid, witness_ptr must point to valid WitnessBundle
/// MEMORY-SAFE: Comprehensive bounds checking and input validation
#[no_mangle]
pub extern "C" fn orchard_witness_add_read(
    witness_ptr: *mut WitnessBundle,
    key_ptr: *const u8,
    key_len: u32,
    value_ptr: *const u8,
    value_len: u32,
    proof_ptr: *const u8,  // Merkle proof (32-byte chunks)
    proof_len: u32,        // Number of 32-byte proof elements
) -> u32 {
    // SECURITY: Validate all input parameters
    const MAX_KEY_SIZE: u32 = 1024;        // Max 1KB key
    const MAX_VALUE_SIZE: u32 = 1024 * 1024; // Max 1MB value
    const MAX_PROOF_ELEMENTS: u32 = 64;     // Max 64 proof elements

    if witness_ptr.is_null() || key_ptr.is_null() || value_ptr.is_null() {
        return FFIResult::InvalidInput as u32;
    }

    // SECURITY: Validate length bounds to prevent excessive memory allocation
    if key_len == 0 || key_len > MAX_KEY_SIZE {
        return FFIResult::InvalidInput as u32;
    }
    if value_len == 0 || value_len > MAX_VALUE_SIZE {
        return FFIResult::InvalidInput as u32;
    }
    if proof_len > MAX_PROOF_ELEMENTS {
        return FFIResult::InvalidInput as u32;
    }

    unsafe {
        let witness = &mut *witness_ptr;

        // SECURITY: Safe string conversion with bounds checking
        let key_slice = slice::from_raw_parts(key_ptr, key_len as usize);
        let key_string = match core::str::from_utf8(key_slice) {
            Ok(s) => s.to_string(),
            Err(_) => return FFIResult::InvalidInput as u32,
        };

        // SECURITY: Safe value copy with bounds checking
        let value_slice = slice::from_raw_parts(value_ptr, value_len as usize);
        let value = value_slice.to_vec();

        // SECURITY: Safe proof copy with overflow protection
        let mut merkle_proof = Vec::new();
        if !proof_ptr.is_null() && proof_len > 0 {
            // Check for potential overflow: proof_len * 32
            let total_proof_bytes = match proof_len.checked_mul(32) {
                Some(size) => size as usize,
                None => return FFIResult::InvalidInput as u32, // Overflow detected
            };

            let proof_slice = slice::from_raw_parts(proof_ptr, total_proof_bytes);
            for i in 0..proof_len {
                let start = (i * 32) as usize;
                let end = start + 32;
                if end > total_proof_bytes {
                    return FFIResult::InvalidInput as u32; // Bounds check
                }
                let mut proof_element = [0u8; 32];
                proof_element.copy_from_slice(&proof_slice[start..end]);
                merkle_proof.push(proof_element);
            }
        }

        let state_read = StateRead {
            key: key_string,
            value,
            merkle_proof,
        };

        witness.reads.push(state_read);
        FFIResult::Success as u32
    }
}


/// Set the service ID for subsequent operations (placeholder for compatibility)
/// Note: Service ID is now passed through refine/accumulate calls in production
/// Returns FFI_SUCCESS if valid, FFI_INVALID_INPUT if service_id is 0
#[no_mangle]
pub extern "C" fn orchard_set_service_id(service_id: u32) -> u32 {
    if service_id == 0 {
        FFIResult::InvalidInput as u32
    } else {
        // In production, service_id is passed from refine/accumulate calls
        // This function exists for FFI compatibility
        FFIResult::Success as u32
    }
}

/// Initialize Orchard FFI system
/// This can be used for any one-time setup
#[no_mangle]
pub extern "C" fn orchard_init() -> u32 {
    // Perform any necessary initialization
    // For now, just return success
    FFIResult::Success as u32
}

/// Cleanup Orchard FFI system
#[no_mangle]
pub extern "C" fn orchard_cleanup() {
    // Perform any necessary cleanup
    // Nothing needed for current implementation
}

// ========================================================================
// MOCK PROOF FUNCTIONS COMPLETELY REMOVED
// ========================================================================
// All mock proof functions have been completely removed to eliminate security vulnerabilities.
// The previous stub functions provided a bypass path that could be exploited in production.
//
// Real Halo2 proof generation and verification must be implemented using:
// - Proper circuit compilation with proving keys
// - Cryptographic proof generation via Halo2 APIs
// - Secure proof verification with public inputs validation
//
// Any attempt to use mock proofs will result in compilation errors.

/// Get current service ID for FFI compatibility
#[no_mangle]
pub extern "C" fn orchard_get_service_id() -> u32 {
    // Return default service ID
    // TODO: Implement proper service ID management
    1
}

/// Legacy nullifier function (wrapper around service ID version)
#[no_mangle]
pub extern "C" fn orchard_nullifier(
    sk_spend: *const u8,
    rho: *const u8,
    commitment: *const u8,
    output: *mut u8,
) -> u32 {
    orchard_nullifier_with_service_id(sk_spend, rho, commitment, output, 1)
}

/// Legacy commitment function (wrapper around service ID version)
#[no_mangle]
pub extern "C" fn orchard_commitment(
    asset_id: u32,
    amount_lo: u64,
    amount_hi: u64,
    owner_pk: *const u8,
    rho: *const u8,
    note_rseed: *const u8,
    unlock_height: u64,
    memo_hash: *const u8,
    output: *mut u8,
) -> u32 {
    orchard_commitment_with_service_id(
        asset_id,
        amount_lo,
        amount_hi,
        owner_pk,
        rho,
        note_rseed,
        unlock_height,
        memo_hash,
        output,
        1,
    )
}

/// Domain-separated Poseidon-like hash for FFI callers.
/// Note: Uses Blake2b as a deterministic placeholder for legacy clients.
#[no_mangle]
pub extern "C" fn orchard_poseidon_hash(
    domain: *const u8,
    domain_len: u32,
    inputs: *const u8,
    input_count: u32,
    output: *mut u8,
) -> u32 {
    if domain.is_null() || inputs.is_null() || output.is_null() {
        return FFIResult::InvalidInput as u32;
    }
    if domain_len == 0 || domain_len > 64 || input_count == 0 || input_count > 10 {
        return FFIResult::InvalidInput as u32;
    }

    let domain_slice = unsafe { slice::from_raw_parts(domain, domain_len as usize) };
    let inputs_len = (input_count as usize)
        .checked_mul(32)
        .unwrap_or(0);
    if inputs_len == 0 {
        return FFIResult::InvalidInput as u32;
    }
    let inputs_slice = unsafe { slice::from_raw_parts(inputs, inputs_len) };

    let mut hasher = match Blake2bVar::new(32) {
        Ok(h) => h,
        Err(_) => return FFIResult::InternalError as u32,
    };
    hasher.update(domain_slice);
    hasher.update(&(input_count as u32).to_le_bytes());
    hasher.update(inputs_slice);

    let mut out = [0u8; 32];
    if hasher.finalize_variable(&mut out).is_err() {
        return FFIResult::InternalError as u32;
    }

    unsafe {
        ptr::copy_nonoverlapping(out.as_ptr(), output, out.len());
    }
    FFIResult::Success as u32
}

/// Compute IssueBundle commitments from encoded IssueBundle bytes.
///
/// # Safety
/// All pointers must be valid for the specified lengths.
#[no_mangle]
pub extern "C" fn orchard_issue_bundle_commitments(
    issue_bundle_ptr: *const u8,
    issue_bundle_len: u32,
    commitments_ptr: *mut u8,
    commitments_cap: u32,
    commitments_len_out: *mut u32,
) -> u32 {
    if issue_bundle_ptr.is_null() || commitments_ptr.is_null() || commitments_len_out.is_null() {
        return FFIResult::InvalidInput as u32;
    }
    if issue_bundle_len == 0 {
        unsafe {
            *commitments_len_out = 0;
        }
        return FFIResult::Success as u32;
    }

    let issue_bundle = unsafe { slice::from_raw_parts(issue_bundle_ptr, issue_bundle_len as usize) };
    let decoded = match decode_issue_bundle(issue_bundle) {
        Ok(Some(bundle)) => bundle,
        Ok(None) => {
            unsafe {
                *commitments_len_out = 0;
            }
            return FFIResult::Success as u32;
        }
        Err(_) => return FFIResult::SerializationError as u32,
    };

    let commitments = match crate::signature_verifier::compute_issue_bundle_commitments(&decoded) {
        Ok(values) => values,
        Err(_) => return FFIResult::SerializationError as u32,
    };

    let total_bytes = match commitments.len().checked_mul(32) {
        Some(value) => value,
        None => return FFIResult::InvalidInput as u32,
    };
    if total_bytes > commitments_cap as usize {
        return FFIResult::InvalidInput as u32;
    }

    unsafe {
        let out = slice::from_raw_parts_mut(commitments_ptr, total_bytes);
        for (idx, commitment) in commitments.iter().enumerate() {
            let offset = idx * 32;
            out[offset..offset + 32].copy_from_slice(commitment);
        }
        *commitments_len_out = commitments.len() as u32;
    }

    FFIResult::Success as u32
}

/// Decode a V6 Orchard bundle and return action groups, nullifiers, commitments, and bundle type.
///
/// # Safety
/// All pointers must be valid for the specified lengths.
#[no_mangle]
pub extern "C" fn orchard_decode_bundle_v6(
    orchard_bundle_ptr: *const u8,
    orchard_bundle_len: u32,
    issue_bundle_ptr: *const u8,
    issue_bundle_len: u32,
    bundle_type_out: *mut u8,
    group_sizes_ptr: *mut u32,
    group_sizes_cap: u32,
    group_count_out: *mut u32,
    nullifiers_ptr: *mut u8,
    nullifiers_cap: u32,
    commitments_ptr: *mut u8,
    commitments_cap: u32,
    issue_bundle_out: *mut u8,
    issue_bundle_cap: u32,
    issue_bundle_len_out: *mut u32,
) -> u32 {
    if bundle_type_out.is_null()
        || group_sizes_ptr.is_null()
        || group_count_out.is_null()
        || nullifiers_ptr.is_null()
        || commitments_ptr.is_null()
    {
        return FFIResult::InvalidInput as u32;
    }
    if orchard_bundle_len == 0 && issue_bundle_len == 0 {
        return FFIResult::InvalidInput as u32;
    }
    if orchard_bundle_len > 0 && orchard_bundle_ptr.is_null() {
        return FFIResult::InvalidInput as u32;
    }
    if issue_bundle_len > 0 && issue_bundle_ptr.is_null() {
        return FFIResult::InvalidInput as u32;
    }

    let mut group_sizes: Vec<usize> = Vec::new();
    let mut total_actions: usize = 0;
    let mut zsa_bundle = None;

    let mut swap_bundle = None;
    let bundle_type = if orchard_bundle_len == 0 {
        OrchardBundleType::ZSA
    } else {
        let orchard_bundle =
            unsafe { slice::from_raw_parts(orchard_bundle_ptr, orchard_bundle_len as usize) };
        let mut decoded_swap = decode_swap_bundle(orchard_bundle).ok();
        if let Some(bundle) = decoded_swap.as_ref() {
            group_sizes.reserve(bundle.action_groups.len());
            for group in &bundle.action_groups {
                group_sizes.push(group.actions.len());
                total_actions = total_actions.saturating_add(group.actions.len());
            }
            swap_bundle = decoded_swap.take();
            OrchardBundleType::Swap
        } else if let Ok(bundle) = decode_zsa_bundle(orchard_bundle) {
            group_sizes.push(bundle.actions.len());
            total_actions = bundle.actions.len();
            zsa_bundle = Some(bundle);
            OrchardBundleType::ZSA
        } else {
            return FFIResult::SerializationError as u32;
        }
    };

    if group_sizes.len() > group_sizes_cap as usize {
        return FFIResult::InvalidInput as u32;
    }

    let total_bytes = match total_actions.checked_mul(32) {
        Some(value) => value,
        None => return FFIResult::InvalidInput as u32,
    };
    if total_bytes > nullifiers_cap as usize || total_bytes > commitments_cap as usize {
        return FFIResult::InvalidInput as u32;
    }

    if issue_bundle_len > 0 {
        let issue_bundle =
            unsafe { slice::from_raw_parts(issue_bundle_ptr, issue_bundle_len as usize) };
        if decode_issue_bundle(issue_bundle).is_err() {
            return FFIResult::SerializationError as u32;
        }
        if !issue_bundle_out.is_null() {
            if issue_bundle_len as usize > issue_bundle_cap as usize {
                return FFIResult::InvalidInput as u32;
            }
            unsafe {
                let out = slice::from_raw_parts_mut(issue_bundle_out, issue_bundle_len as usize);
                out.copy_from_slice(issue_bundle);
            }
        }
    }

    unsafe {
        *bundle_type_out = bundle_type as u8;
        *group_count_out = group_sizes.len() as u32;
        if !issue_bundle_len_out.is_null() {
            *issue_bundle_len_out = issue_bundle_len;
        }

        let group_sizes_out = slice::from_raw_parts_mut(group_sizes_ptr, group_sizes.len());
        for (idx, size) in group_sizes.iter().enumerate() {
            group_sizes_out[idx] = *size as u32;
        }

        if total_bytes > 0 {
            let nullifiers_out = slice::from_raw_parts_mut(nullifiers_ptr, total_bytes);
            let commitments_out = slice::from_raw_parts_mut(commitments_ptr, total_bytes);

            let mut action_idx = 0usize;
            match bundle_type {
                OrchardBundleType::Swap => {
                    if let Some(bundle) = swap_bundle.as_ref() {
                        for group in &bundle.action_groups {
                            for action in &group.actions {
                                let offset = action_idx * 32;
                                nullifiers_out[offset..offset + 32].copy_from_slice(&action.nullifier);
                                commitments_out[offset..offset + 32].copy_from_slice(&action.cmx);
                                action_idx += 1;
                            }
                        }
                    }
                }
                OrchardBundleType::ZSA => {
                    let Some(bundle) = zsa_bundle.as_ref() else {
                        return FFIResult::SerializationError as u32;
                    };
                    for action in &bundle.actions {
                        let offset = action_idx * 32;
                        nullifiers_out[offset..offset + 32].copy_from_slice(&action.nullifier);
                        commitments_out[offset..offset + 32].copy_from_slice(&action.cmx);
                        action_idx += 1;
                    }
                }
                OrchardBundleType::Vanilla => {
                    return FFIResult::InvalidInput as u32;
                }
            }
        }
    }

    FFIResult::Success as u32
}

/// Verify Halo2 proof using the embedded Orchard verifying key.
///
/// # Safety
/// All pointers must be valid for the specified lengths.
#[no_mangle]
pub extern "C" fn orchard_verify_halo2_proof(
    vk_id: u32,
    proof_ptr: *const u8,
    proof_len: u32,
    public_inputs_ptr: *const u8,
    public_inputs_len: u32, // number of 32-byte elements
) -> u32 {
    if proof_ptr.is_null() || public_inputs_ptr.is_null() {
        return FFIResult::InvalidInput as u32;
    }
    if public_inputs_len == 0 {
        return FFIResult::InvalidInput as u32;
    }

    let proof = unsafe { slice::from_raw_parts(proof_ptr, proof_len as usize) };
    let inputs_bytes_len = match (public_inputs_len as usize).checked_mul(32) {
        Some(len) => len,
        None => return FFIResult::InvalidInput as u32,
    };
    let inputs_bytes = unsafe { slice::from_raw_parts(public_inputs_ptr, inputs_bytes_len) };

    let mut inputs = Vec::with_capacity(public_inputs_len as usize);
    for i in 0..public_inputs_len as usize {
        let start = i * 32;
        let end = start + 32;
        if end > inputs_bytes.len() {
            return FFIResult::InvalidInput as u32;
        }
        let mut input = [0u8; 32];
        input.copy_from_slice(&inputs_bytes[start..end]);
        inputs.push(input);
    }

    match crate::crypto::verify_halo2_proof(proof, &inputs, vk_id) {
        Ok(true) => FFIResult::Success as u32,
        Ok(false) => FFIResult::VerificationFailed as u32,
        Err(_) => FFIResult::VerificationFailed as u32,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use alloc::vec::Vec;

    #[test]
    fn test_ffi_nullifier() {
        let sk = [1u8; 32];
        let rho = [2u8; 32];
        let commitment = [3u8; 32];
        let mut output = [0u8; 32];

        let result = orchard_nullifier(
            sk.as_ptr(),
            rho.as_ptr(),
            commitment.as_ptr(),
            output.as_mut_ptr(),
        );

        assert_eq!(result, FFIResult::Success as u32);
        assert_ne!(output, [0u8; 32]); // Should have generated some output
    }

    #[test]
    fn test_ffi_commitment() {
        let asset_id = 1;
        let amount_lo = 1000u64;
        let amount_hi = 0u64;
        let owner_pk = [1u8; 32];
        let rho = [2u8; 32];
        let note_rseed = [3u8; 32];
        let unlock_height = 0u64;
        let memo_hash = [4u8; 32];
        let mut output = [0u8; 32];

        let result = orchard_commitment(
            asset_id,
            amount_lo,
            amount_hi,
            owner_pk.as_ptr(),
            rho.as_ptr(),
            note_rseed.as_ptr(),
            unlock_height,
            memo_hash.as_ptr(),
            output.as_mut_ptr(),
        );

        assert_eq!(result, FFIResult::Success as u32);
        assert_ne!(output, [0u8; 32]); // Should have generated some output
    }

    #[test]
    fn test_ffi_decode_issue_only_v6() {
        let mut issue_bundle = Vec::new();
        issue_bundle.push(1); // issuer_len
        issue_bundle.push(0x01); // issuer_key
        issue_bundle.push(1); // action_count
        issue_bundle.extend_from_slice(&[0u8; 32]); // asset_desc_hash
        issue_bundle.push(1); // note_count
        issue_bundle.extend_from_slice(&[0u8; 43]); // recipient
        issue_bundle.extend_from_slice(&1u64.to_le_bytes()); // value
        issue_bundle.extend_from_slice(&[0u8; 32]); // rho
        issue_bundle.extend_from_slice(&[0u8; 32]); // rseed
        issue_bundle.push(0); // finalize
        issue_bundle.push(0); // sighash_info_len
        issue_bundle.push(0); // signature_len

        let mut bundle_type: u8 = 0;
        let mut group_sizes = [0u32; 8];
        let mut group_count: u32 = 0;
        let mut nullifiers = [0u8; 32 * 4];
        let mut commitments = [0u8; 32 * 4];
        let mut issue_out = vec![0u8; issue_bundle.len()];
        let mut issue_len_out: u32 = 0;

        let result = orchard_decode_bundle_v6(
            core::ptr::null(),
            0,
            issue_bundle.as_ptr(),
            issue_bundle.len() as u32,
            &mut bundle_type as *mut u8,
            group_sizes.as_mut_ptr(),
            group_sizes.len() as u32,
            &mut group_count as *mut u32,
            nullifiers.as_mut_ptr(),
            nullifiers.len() as u32,
            commitments.as_mut_ptr(),
            commitments.len() as u32,
            issue_out.as_mut_ptr(),
            issue_out.len() as u32,
            &mut issue_len_out as *mut u32,
        );

        assert_eq!(result, FFIResult::Success as u32);
        assert_eq!(group_count, 0);
        assert_eq!(issue_len_out as usize, issue_bundle.len());
        assert_eq!(issue_out, issue_bundle);
    }

    // ❌ REMOVED: Mock proof tests that used the insecure mock proof functions
    // These tests validated predictable fake proofs instead of real cryptographic validation
    // Production systems must use real Halo2 circuit tests with proper proving/verifying keys
}
