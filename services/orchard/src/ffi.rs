/// FFI (Foreign Function Interface) module for Orchard service
///
/// This module provides C-compatible functions that can be called from Go
/// to perform Halo2 proof generation, verification, and cryptographic operations.

use crate::crypto::{commitment, nullifier, poseidon_hash_with_domain};
use crate::witness::{WitnessBundle, StateRead};
use alloc::vec::Vec;
use alloc::string::ToString;
use core::slice;
use core::ptr;
use core::str;

/// Extrinsic type constants for FFI
#[repr(C)]
pub enum ExtrinsicType {
    DepositPublic = 0,
    SubmitPrivate = 1,
    WithdrawPublic = 2,
    IssuanceV1 = 3,
    BatchAggV1 = 4,
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

/// Generate Poseidon hash with domain separation
///
/// # Safety
/// All pointers must be valid for the specified lengths
/// MEMORY-SAFE: Enhanced bounds checking and overflow protection
#[no_mangle]
pub extern "C" fn orchard_poseidon_hash(
    domain: *const u8,        // Domain tag bytes (UTF-8)
    domain_len: u32,          // Domain length (1-64 bytes)
    inputs: *const u8,        // Input field elements (32 bytes each, big-endian)
    input_count: u32,         // Number of inputs (1-10)
    output: *mut u8,          // 32 bytes output buffer
) -> u32 {
    // SECURITY: Validate input parameters with strict bounds
    const MAX_POSEIDON_INPUTS: u32 = 10; // Domain + up to 10 inputs
    const MAX_DOMAIN_LEN: u32 = 64;

    if domain.is_null() || inputs.is_null() || output.is_null() {
        return FFIResult::InvalidInput as u32;
    }
    if domain_len == 0 || domain_len > MAX_DOMAIN_LEN {
        return FFIResult::InvalidInput as u32;
    }
    if input_count == 0 || input_count > MAX_POSEIDON_INPUTS {
        return FFIResult::InvalidInput as u32;
    }

    unsafe {
        let domain_slice = slice::from_raw_parts(domain, domain_len as usize);
        let domain_str = match str::from_utf8(domain_slice) {
            Ok(value) => value,
            Err(_) => return FFIResult::InvalidInput as u32,
        };

        // SECURITY: Check for overflow when calculating total size
        let total_input_bytes = match input_count.checked_mul(32) {
            Some(size) => size as usize,
            None => return FFIResult::InvalidInput as u32, // Overflow detected
        };

        let inputs_slice = slice::from_raw_parts(inputs, total_input_bytes);
        let mut field_inputs: Vec<[u8; 32]> = Vec::new();

        // SECURITY: Safe chunk processing with exact size validation
        for chunk in inputs_slice.chunks_exact(32) {
            let mut bytes = [0u8; 32];
            bytes.copy_from_slice(chunk);
            field_inputs.push(bytes);
        }

        // Verify we got the expected number of field elements
        if field_inputs.len() != input_count as usize {
            return FFIResult::InvalidInput as u32;
        }

        let result = poseidon_hash_with_domain(domain_str, &field_inputs);

        let output_slice = slice::from_raw_parts_mut(output, 32);
        output_slice.copy_from_slice(&result);
        FFIResult::Success as u32
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

    // ❌ REMOVED: Mock proof tests that used the insecure mock proof functions
    // These tests validated predictable fake proofs instead of real cryptographic validation
    // Production systems must use real Halo2 circuit tests with proper proving/verifying keys
}
