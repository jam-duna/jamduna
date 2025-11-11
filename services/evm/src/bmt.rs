//! JAM Binary Merkle Trie (BMT) implementation
//!
//! This module implements the JAM Binary Merkle Trie following the Graypaper D.4 specification.
//! It matches the Go implementation in trie/bpt.go for compatibility.

use alloc::vec::Vec;
use utils::hash_functions::blake2b_hash as blake2b;

/// Compute JAM Binary Merkle Trie (BMT) root from key-value pairs
///
/// This implementation follows JAM's BMT specification (Graypaper D.4):
/// - Uses Blake2b-256 for hashing
/// - Proper JAM leaf encoding with embedded values or hash references
/// - Proper JAM branch encoding with 255-bit left + 256-bit right
/// - Bit-based tree construction matching bpt.go implementation
pub fn compute_bmt_root(kvs: &[([u8; 32], Vec<u8>)]) -> [u8; 32] {
    // Build the BMT recursively using bit-based splitting
    build_bmt_recursive(kvs, 0)
}

/// Recursive BMT construction following JAM specification
fn build_bmt_recursive(kvs: &[([u8; 32], Vec<u8>)], bit_index: usize) -> [u8; 32] {
    // Base case: empty data
    if kvs.is_empty() {
        return empty_bmt_root();
    }

    // Base case: single key-value pair - create leaf
    if kvs.len() == 1 {
        let (key, value) = &kvs[0];
        let leaf_encoded = encode_jam_leaf(key, value);
        return blake2b(&leaf_encoded);
    }

    // Recursive case: split by bit and create branch
    let mut left_kvs = Vec::new();
    let mut right_kvs = Vec::new();

    for (key, value) in kvs {
        if get_bit(key, bit_index) {
            right_kvs.push((*key, value.clone()));
        } else {
            left_kvs.push((*key, value.clone()));
        }
    }

    let left_hash = build_bmt_recursive(&left_kvs, bit_index + 1);
    let right_hash = build_bmt_recursive(&right_kvs, bit_index + 1);

    let branch_encoded = encode_jam_branch(&left_hash, &right_hash);
    blake2b(&branch_encoded)
}

/// Encode JAM leaf node according to Graypaper D.4
/// Format: [header|31B key|32B value_or_hash]
///
/// This function MUST match the Go implementation in trie/bpt.go::leaf()
fn encode_jam_leaf(key: &[u8; 32], value: &[u8]) -> [u8; 64] {
    let mut result = [0u8; 64];

    if value.len() <= 32 {
        // Embedded-value leaf node
        result[0] = 0b10000000 | (value.len() as u8);
        // Copy first 31 bytes of key (matching Go: k[:31])
        result[1..32].copy_from_slice(&key[0..31]);
        result[32..32+value.len()].copy_from_slice(value);
    } else {
        // Regular leaf node with hash reference
        result[0] = 0b11000000;
        // Copy first 31 bytes of key (matching Go: k[:31])
        result[1..32].copy_from_slice(&key[0..31]);

        let value_hash = blake2b(value);
        result[32..64].copy_from_slice(&value_hash);
    }

    result
}

/// Encode JAM branch node according to Graypaper D.4
/// Format: [255-bit left | 256-bit right] = 63.875 bytes
fn encode_jam_branch(left: &[u8; 32], right: &[u8; 32]) -> [u8; 64] {
    let mut result = [0u8; 64];

    // Clear LSB of first byte of left hash (255 bits)
    let head = left[0] & 0x7f;
    result[0] = head;
    result[1..32].copy_from_slice(&left[1..]);

    // Full 256 bits of right hash
    result[32..64].copy_from_slice(right);

    result
}

/// Get bit at position i from key (MSB first)
fn get_bit(key: &[u8; 32], bit_index: usize) -> bool {
    if bit_index >= 256 {
        return false;
    }
    let byte_index = bit_index / 8;
    let bit_offset = 7 - (bit_index % 8);
    (key[byte_index] >> bit_offset) & 1 == 1
}

/// Returns the root hash of an empty BMT (all zeros)
pub fn empty_bmt_root() -> [u8; 32] {
    [0u8; 32]
}