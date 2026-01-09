// Sinsemilla-based Merkle Tree Hashing for Orchard
//
// This module provides `no_std` compatible Merkle root computation using
// Orchard's Sinsemilla hash function. It can be used by both the builder
// and service to ensure consistent commitment tree roots.
//
// Key functions:
// - `compute_merkle_root`: Compute root from a set of leaves (tree reconstruction)
// - `merkle_append`: Append new commitments to existing tree (incremental)
//
// Hash function: Sinsemilla (via MerkleHashOrchard from zcash/orchard)
// Tree structure: Binary Merkle tree, depth 32, left-to-right appending

#![cfg_attr(not(feature = "std"), no_std)]

extern crate alloc;
use alloc::vec::Vec;

use incrementalmerkletree::{Hashable, Level};
use orchard::tree::MerkleHashOrchard;

pub const TREE_DEPTH: usize = 32;

/// Compute Merkle root for a contiguous set of commitment leaves.
///
/// This reconstructs a full binary Merkle tree from the given leaves,
/// padding with empty leaves to the right and hashing up to the root.
///
/// # Arguments
/// * `leaves` - Commitment values (e.g., note commitments)
///
/// # Returns
/// 32-byte Merkle root using Orchard's Sinsemilla hash
///
/// # Example
/// ```ignore
/// let leaves = vec![[1u8; 32], [2u8; 32]];
/// let root = compute_merkle_root(&leaves);
/// ```
pub fn compute_merkle_root(leaves: &[[u8; 32]]) -> Result<[u8; 32], &'static str> {
    if leaves.is_empty() {
        return Ok(MerkleHashOrchard::empty_root(Level::from(TREE_DEPTH as u8)).to_bytes());
    }

    if leaves.len() > (1usize << TREE_DEPTH) {
        return Err("Too many leaves for tree depth");
    }

    // Convert byte arrays to MerkleHashOrchard
    let mut current_level: Vec<MerkleHashOrchard> = leaves
        .iter()
        .map(|bytes| {
            Option::from(MerkleHashOrchard::from_bytes(bytes))
                .ok_or("Invalid commitment bytes")
        })
        .collect::<Result<Vec<_>, _>>()?;

    // Build tree bottom-up
    for level in 0..TREE_DEPTH {
        if current_level.len() == 1 {
            // Reached root, but need to pad with empty subtrees up to TREE_DEPTH
            let mut root = current_level[0];
            for height in level..TREE_DEPTH {
                let empty = MerkleHashOrchard::empty_root(Level::from(height as u8));
                root = MerkleHashOrchard::combine(Level::from(height as u8), &root, &empty);
            }
            return Ok(root.to_bytes());
        }

        // Hash pairs at this level
        let mut next_level = Vec::with_capacity((current_level.len() + 1) / 2);
        let empty = MerkleHashOrchard::empty_root(Level::from(level as u8));

        for pair in current_level.chunks(2) {
            let left = pair[0];
            let right = if pair.len() == 2 { pair[1] } else { empty };
            next_level.push(MerkleHashOrchard::combine(Level::from(level as u8), &left, &right));
        }

        current_level = next_level;
    }

    Ok(current_level[0].to_bytes())
}

/// Append new commitments to an existing Merkle tree.
///
/// This computes the new Merkle root after appending `new_commitments`
/// to a tree that currently has `current_size` leaves.
///
/// **IMPORTANT**: This is a simplified implementation that only works correctly
/// when starting from an empty tree (current_size == 0). For non-zero current_size,
/// it uses placeholder empty leaves which produces incorrect roots.
///
/// For production use, this should reconstruct the tree from actual commitment
/// values or use an incremental Merkle tree data structure.
///
/// # Arguments
/// * `_current_root` - Current Merkle root (unused in this implementation)
/// * `current_size` - Number of existing leaves in the tree
/// * `new_commitments` - New commitment values to append
///
/// # Returns
/// 32-byte Merkle root of the updated tree
///
/// # Example
/// ```ignore
/// let empty_root = [0u8; 32];
/// let new_commits = vec![[1u8; 32], [2u8; 32]];
/// let new_root = merkle_append(&empty_root, 0, &new_commits)?;
/// ```
pub fn merkle_append(
    _current_root: &[u8; 32],
    current_size: u64,
    new_commitments: &[[u8; 32]],
) -> Result<[u8; 32], &'static str> {
    if new_commitments.is_empty() {
        // No new commitments, return empty root or reconstruct from current_root
        if current_size == 0 {
            return Ok(MerkleHashOrchard::empty_root(Level::from(TREE_DEPTH as u8)).to_bytes());
        }
        // TODO: Should reconstruct tree from current_root + current_size
        return Err("Cannot reconstruct tree from root alone");
    }

    // Build all_leaves = [existing leaves (placeholders)] + [new commitments]
    let total_size = current_size as usize + new_commitments.len();
    let mut all_leaves = Vec::with_capacity(total_size);

    // LIMITATION: Using empty leaf placeholders for existing commitments
    // This only produces correct roots when current_size == 0
    for _ in 0..current_size {
        all_leaves.push(MerkleHashOrchard::empty_leaf().to_bytes());
    }

    // Add new commitments
    all_leaves.extend_from_slice(new_commitments);

    // Compute root from full leaf set
    compute_merkle_root(&all_leaves)
}

/// Compute Merkle root from leaves using the same algorithm as IncrementalMerkleTree.
///
/// This is a more efficient version that matches the builder's internal logic.
/// It builds the tree level-by-level from bottom to top.
///
/// # Arguments
/// * `leaves` - MerkleHashOrchard leaves (already parsed from bytes)
/// * `depth` - Tree depth (typically 32 for Orchard)
///
/// # Returns
/// MerkleHashOrchard root value
fn compute_root_from_hashes(
    leaves: &[MerkleHashOrchard],
    depth: usize,
) -> MerkleHashOrchard {
    if leaves.is_empty() {
        return MerkleHashOrchard::empty_root(Level::from(depth as u8));
    }

    let levels = build_tree_levels(leaves, depth);
    levels
        .last()
        .and_then(|level| level.first())
        .copied()
        .unwrap_or_else(|| MerkleHashOrchard::empty_root(Level::from(depth as u8)))
}

/// Build all levels of the Merkle tree from leaves to root.
///
/// Returns a vector where:
/// - levels[0] = leaves
/// - levels[1] = hashes of pairs from level 0
/// - ...
/// - levels[depth] = [root]
fn build_tree_levels(
    leaves: &[MerkleHashOrchard],
    depth: usize,
) -> Vec<Vec<MerkleHashOrchard>> {
    let mut levels = Vec::with_capacity(depth + 1);
    levels.push(leaves.to_vec());

    for level in 0..depth {
        let current = &levels[level];
        if current.is_empty() {
            break;
        }

        let mut next = Vec::with_capacity((current.len() + 1) / 2);
        let empty = MerkleHashOrchard::empty_root(Level::from(level as u8));

        for pair in current.chunks(2) {
            let left = pair[0];
            let right = if pair.len() == 2 { pair[1] } else { empty };
            next.push(MerkleHashOrchard::combine(Level::from(level as u8), &left, &right));
        }

        levels.push(next);
    }

    levels
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_empty_tree_root() {
        let root = compute_merkle_root(&[]).unwrap();
        let expected = MerkleHashOrchard::empty_root(Level::from(TREE_DEPTH as u8)).to_bytes();
        assert_eq!(root, expected);
    }

    #[test]
    fn test_single_leaf() {
        // Use a non-empty leaf for this test
        let leaf = [42u8; 32];
        let root = compute_merkle_root(&[leaf]).unwrap();

        // Root should be different from leaf
        assert_ne!(root, leaf);

        // Root should be different from empty tree
        let empty_root = MerkleHashOrchard::empty_root(Level::from(TREE_DEPTH as u8)).to_bytes();
        assert_ne!(root, empty_root);
    }

    #[test]
    fn test_two_leaves() {
        let leaf1 = MerkleHashOrchard::empty_leaf().to_bytes();
        let leaf2 = MerkleHashOrchard::empty_leaf().to_bytes();

        let root = compute_merkle_root(&[leaf1, leaf2]).unwrap();

        // Root should be deterministic
        let root2 = compute_merkle_root(&[leaf1, leaf2]).unwrap();
        assert_eq!(root, root2);
    }

    #[test]
    fn test_merkle_append_from_empty() {
        let empty_root = MerkleHashOrchard::empty_root(Level::from(TREE_DEPTH as u8)).to_bytes();
        let leaf1 = MerkleHashOrchard::empty_leaf().to_bytes();
        let leaf2 = MerkleHashOrchard::empty_leaf().to_bytes();

        // Append 2 leaves to empty tree
        let new_root = merkle_append(&empty_root, 0, &[leaf1, leaf2]).unwrap();

        // Should match direct computation
        let expected = compute_merkle_root(&[leaf1, leaf2]).unwrap();
        assert_eq!(new_root, expected);
    }

    #[test]
    fn test_different_leaves_different_roots() {
        let leaf1 = [1u8; 32];
        let leaf2 = [2u8; 32];

        let root1 = compute_merkle_root(&[leaf1]).unwrap();
        let root2 = compute_merkle_root(&[leaf2]).unwrap();

        assert_ne!(root1, root2);
    }

    #[test]
    fn test_order_matters() {
        let leaf1 = [1u8; 32];
        let leaf2 = [2u8; 32];

        let root_12 = compute_merkle_root(&[leaf1, leaf2]).unwrap();
        let root_21 = compute_merkle_root(&[leaf2, leaf1]).unwrap();

        // Different order = different root
        assert_ne!(root_12, root_21);
    }
}
