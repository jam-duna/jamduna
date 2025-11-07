//! Generic JAM DA helper functions
//!
//! This module contains generic JAM DA utilities that may be shared across services.
//!
//! Note: Cryptographic hashing functions (keccak256, blake2b) are in hash_functions.rs
//! Note: EVM-specific helpers (type conversions, transaction decoding, ObjectID construction) are in services/evm/src/helpers.rs

use alloc::vec::Vec;
use crate::constants::FIRST_READABLE_ADDRESS;

/// Leaks a buffer and returns pointer/length tuple for FFI
/// This transfers ownership of the buffer to the caller
pub fn leak_output(buffer: Vec<u8>) -> (u64, u64) {
    let ptr = buffer.as_ptr() as u64;
    let len = buffer.len() as u64;
    core::mem::forget(buffer);
    (ptr, len)
}

/// Returns an empty output for error cases
/// Uses FIRST_READABLE_ADDRESS with length 0 to indicate no output
pub fn empty_output() -> (u64, u64) {
    (FIRST_READABLE_ADDRESS as u64, 0)
}
