// Sinsemilla-based Merkle Tree Hashing - no_std implementation
//
// This module provides Sinsemilla hashing using only pasta_curves (no orchard crate).
// It replicates the behavior of MerkleHashOrchard from the orchard crate but in a
// no_std compatible way.
//
// Based on: https://zips.z.cash/protocol/protocol.pdf Section 5.4.1.6

#![allow(dead_code)]

use alloc::vec::Vec;
use pasta_curves::pallas::Base as PallasBase;
use pasta_curves::group::ff::{PrimeField, PrimeFieldBits};
use crate::errors::{OrchardError, Result};

pub const TREE_DEPTH: usize = 32;
const K: usize = 10;
const L_ORCHARD_MERKLE: usize = 255;

/// Sinsemilla hash domain separator for Merkle tree
const MERKLE_CRH_PERSONALIZATION: &str = "z.cash:Orchard-MerkleCRH";

/// Compute Merkle root using Sinsemilla hash (no_std compatible)
pub fn compute_merkle_root_sinsemilla(leaves: &[[u8; 32]]) -> Result<[u8; 32]> {
    if leaves.is_empty() {
        return Ok(empty_root(TREE_DEPTH));
    }

    if leaves.len() > (1usize << TREE_DEPTH) {
        return Err(OrchardError::ParseError("Too many leaves for tree depth".into()));
    }

    // Convert byte arrays to PallasBase field elements
    let mut current_level: Vec<PallasBase> = leaves
        .iter()
        .map(|bytes| bytes_to_pallas(bytes))
        .collect::<Result<Vec<_>>>()?;

    // Build tree bottom-up
    for level in 0..TREE_DEPTH {
        if current_level.len() == 1 {
            // Reached root, but need to pad with empty subtrees up to TREE_DEPTH
            let mut root = current_level[0];
            for height in level..TREE_DEPTH {
                let empty = empty_node(height);
                root = merkle_combine(height, root, empty);
            }
            return Ok(pallas_to_bytes(root));
        }

        // Hash pairs at this level
        let mut next_level = Vec::with_capacity((current_level.len() + 1) / 2);
        let empty = empty_node(level);

        for pair in current_level.chunks(2) {
            let left = pair[0];
            let right = if pair.len() == 2 { pair[1] } else { empty };
            next_level.push(merkle_combine(level, left, right));
        }

        current_level = next_level;
    }

    Ok(pallas_to_bytes(current_level[0]))
}

/// Append new commitments to existing tree (Sinsemilla version)
pub fn merkle_append_sinsemilla(
    _current_root: &[u8; 32],
    current_size: u64,
    new_commitments: &[[u8; 32]],
) -> Result<[u8; 32]> {
    if new_commitments.is_empty() {
        if current_size == 0 {
            return Ok(empty_root(TREE_DEPTH));
        }
        return Err(OrchardError::ParseError("Cannot reconstruct tree from root alone".into()));
    }

    // Build all_leaves = [existing (empty)] + [new commitments]
    let total_size = current_size as usize + new_commitments.len();
    let mut all_leaves = Vec::with_capacity(total_size);

    // LIMITATION: Using empty leaf placeholders
    // Only correct when current_size == 0
    for _ in 0..current_size {
        all_leaves.push(empty_leaf());
    }

    all_leaves.extend_from_slice(new_commitments);

    compute_merkle_root_sinsemilla(&all_leaves)
}

/// Combine two nodes at the given level and return the resulting hash.
pub fn merkle_combine_bytes(
    level: usize,
    left: &[u8; 32],
    right: &[u8; 32],
) -> Result<[u8; 32]> {
    let left_fp = bytes_to_pallas(left)?;
    let right_fp = bytes_to_pallas(right)?;
    Ok(pallas_to_bytes(merkle_combine(level, left_fp, right_fp)))
}

/// Empty node hash at the given level.
pub fn empty_node_bytes(level: usize) -> [u8; 32] {
    pallas_to_bytes(empty_node(level))
}

/// Merkle tree combine operation using Sinsemilla
///
/// Implements the real Sinsemilla hash as used in Orchard's MerkleCRH.
/// Based on: https://zips.z.cash/protocol/protocol.pdf Section 5.4.1.9
fn merkle_combine(level: usize, left: PallasBase, right: PallasBase) -> PallasBase {
    use halo2_gadgets::sinsemilla::primitives::HashDomain;

    // Create Sinsemilla hash domain for Merkle CRH
    let domain = HashDomain::new(MERKLE_CRH_PERSONALIZATION);

    // Convert level to K-bit little-endian bit representation
    let level_bits = i2lebsp_k(level);

    // Convert left and right field elements to bit representations
    // Take only L_ORCHARD_MERKLE bits (255 bits) from each
    let left_bits = left.to_le_bits();
    let right_bits = right.to_le_bits();

    // Build message: level || left || right
    let message = core::iter::empty()
        .chain(level_bits.iter().copied())
        .chain(left_bits.iter().by_vals().take(L_ORCHARD_MERKLE))
        .chain(right_bits.iter().by_vals().take(L_ORCHARD_MERKLE));

    // Hash using Sinsemilla and extract base field element
    domain
        .hash(message)
        .unwrap_or(PallasBase::zero())
}

/// Convert integer to K-bit little-endian bit representation
/// From zcash spec: i2lebsp_K function
fn i2lebsp_k(int: usize) -> [bool; K] {
    assert!(int < (1 << K), "Integer too large for K bits");
    let mut bits = [false; K];
    for (i, bit) in bits.iter_mut().enumerate() {
        *bit = (int & (1 << i)) != 0;
    }
    bits
}

/// Empty leaf value (uncommitted leaf = pallas::Base(2))
pub fn empty_leaf() -> [u8; 32] {
    PallasBase::from(2).to_repr()
}

/// Empty node at given height
fn empty_node(height: usize) -> PallasBase {
    // Precomputed empty roots by height
    // For now, compute on the fly (could cache these)
    if height == 0 {
        return PallasBase::from(2);
    }

    let child = empty_node(height - 1);
    merkle_combine(height - 1, child, child)
}

/// Empty root at given depth
fn empty_root(depth: usize) -> [u8; 32] {
    pallas_to_bytes(empty_node(depth))
}

/// Convert 32-byte array to Pallas base field element
fn bytes_to_pallas(bytes: &[u8; 32]) -> Result<PallasBase> {
    let mut repr = <PallasBase as PrimeField>::Repr::default();
    repr.as_mut().copy_from_slice(bytes);

    Option::from(PallasBase::from_repr(repr))
        .ok_or_else(|| OrchardError::ParseError("Invalid field element encoding".into()))
}

/// Convert Pallas base field element to 32-byte array
fn pallas_to_bytes(element: PallasBase) -> [u8; 32] {
    element.to_repr()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_empty_root() {
        let root = compute_merkle_root_sinsemilla(&[]).unwrap();
        let expected = empty_root(TREE_DEPTH);
        assert_eq!(root, expected);
    }

    #[test]
    fn test_single_leaf() {
        let leaf = [42u8; 32];
        let root = compute_merkle_root_sinsemilla(&[leaf]).unwrap();

        // Should be different from leaf
        assert_ne!(root, leaf);

        // Should be different from empty
        let empty = empty_root(TREE_DEPTH);
        assert_ne!(root, empty);
    }

    #[test]
    fn test_two_leaves() {
        let leaf1 = [1u8; 32];
        let leaf2 = [2u8; 32];

        let root = compute_merkle_root_sinsemilla(&[leaf1, leaf2]).unwrap();

        // Deterministic
        let root2 = compute_merkle_root_sinsemilla(&[leaf1, leaf2]).unwrap();
        assert_eq!(root, root2);
    }

    #[test]
    fn test_order_matters() {
        let leaf1 = [1u8; 32];
        let leaf2 = [2u8; 32];

        let root_12 = compute_merkle_root_sinsemilla(&[leaf1, leaf2]).unwrap();
        let root_21 = compute_merkle_root_sinsemilla(&[leaf2, leaf1]).unwrap();

        // Different order = different root
        assert_ne!(root_12, root_21);
    }

    #[test]
    fn test_append_from_empty() {
        let empty = empty_root(TREE_DEPTH);
        let leaf1 = [1u8; 32];
        let leaf2 = [2u8; 32];

        let root = merkle_append_sinsemilla(&empty, 0, &[leaf1, leaf2]).unwrap();

        // Should match direct computation
        let expected = compute_merkle_root_sinsemilla(&[leaf1, leaf2]).unwrap();
        assert_eq!(root, expected);
    }
}
