//! Verkle proof verification - delegates to Go FFI until rust-verkle is ready

/// Verify dual-proof Verkle witness
/// 
/// Currently delegates to Go implementation via FFI.
/// TODO: Implement native Rust verification when rust-verkle compiles.
pub fn verify_verkle_witness(witness_data: &[u8]) -> bool {
    // Call Go host function for now
    verify_with_go_ffi(witness_data)
}

fn verify_with_go_ffi(witness_data: &[u8]) -> bool {
    let mut buffer = alloc::vec![0u8; witness_data.len()];
    buffer.copy_from_slice(witness_data);

    unsafe {
        let result = crate::verkle::host_verify_verkle_proof(
            buffer.as_ptr() as u64,
            witness_data.len() as u64,
        );
        result == 1
    }
}
