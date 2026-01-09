// Proper Merkle Tree Implementations for Orchard State
//
// This module implements an Orchard-compatible incremental commitment tree
// and a sparse Merkle tree for nullifiers, with real witness paths.

use blake2b_simd::Params;
use incrementalmerkletree::{Hashable, Level};
use orchard::tree::MerkleHashOrchard;

/// Simple incremental Merkle tree for note commitments
///
/// Uses Orchard's Sinsemilla hashing and empty root conventions.
#[derive(Clone, Debug)]
pub struct IncrementalMerkleTree {
    /// Tree depth
    depth: usize,
    /// Leaves stored in order
    leaves: Vec<MerkleHashOrchard>,
    /// Cached root (recomputed on changes)
    cached_root: Option<MerkleHashOrchard>,
}

impl IncrementalMerkleTree {
    pub fn new(depth: usize) -> Self {
        Self {
            depth,
            leaves: Vec::new(),
            cached_root: None,
        }
    }

    pub fn with_root(root: [u8; 32], size: usize, depth: usize) -> Self {
        // In production, we'd reconstruct from the full commitment tree.
        // For initialization, we create an all-empty tree of the given size.
        let leaves = vec![MerkleHashOrchard::empty_leaf(); size];
        let cached_root = Option::from(MerkleHashOrchard::from_bytes(&root))
            .unwrap_or_else(|| MerkleHashOrchard::empty_root(Level::from(depth as u8)));
        let mut tree = Self {
            depth,
            leaves,
            cached_root: Some(cached_root),
        };
        // Force recomputation to ensure consistency when size is non-zero.
        if size > 0 {
            tree.cached_root = None;
            tree.root();
        }
        tree
    }

    /// Append a new leaf to the tree
    pub fn append(&mut self, leaf: [u8; 32]) {
        let hash = Option::from(MerkleHashOrchard::from_bytes(&leaf))
            .expect("commitment bytes must be canonical");
        self.leaves.push(hash);
        self.cached_root = None; // Invalidate cache
    }

    /// Get the current root
    pub fn root(&mut self) -> [u8; 32] {
        if let Some(root) = self.cached_root {
            return root.to_bytes();
        }

        let root = self.compute_root();
        self.cached_root = Some(root);
        root.to_bytes()
    }

    /// Get the tree size (number of leaves)
    pub fn size(&self) -> usize {
        self.leaves.len()
    }

    /// Return all leaf commitments in insertion order.
    pub fn leaves(&self) -> Vec<[u8; 32]> {
        self.leaves.iter().map(|leaf| leaf.to_bytes()).collect()
    }

    /// Return the right-edge frontier nodes for incremental append.
    pub fn frontier(&self) -> Vec<[u8; 32]> {
        compute_frontier_from_leaves(&self.leaves, self.depth)
    }

    /// Compute the Merkle root from leaves
    fn compute_root(&self) -> MerkleHashOrchard {
        if self.leaves.is_empty() {
            return MerkleHashOrchard::empty_root(Level::from(self.depth as u8));
        }

        let levels = build_commitment_levels(&self.leaves, self.depth);
        levels
            .last()
            .and_then(|level| level.first())
            .copied()
            .unwrap_or_else(|| MerkleHashOrchard::empty_root(Level::from(self.depth as u8)))
    }

    /// Generate a Merkle proof for a leaf at given index
    pub fn prove(&self, index: usize) -> Option<MerkleProof> {
        if index >= self.leaves.len() {
            return None;
        }

        let levels = build_commitment_levels(&self.leaves, self.depth);
        let mut siblings = Vec::with_capacity(self.depth);
        let mut current_index = index;

        for level in 0..self.depth {
            let sibling_index = current_index ^ 1;
            let empty = MerkleHashOrchard::empty_root(Level::from(level as u8));
            let sibling = levels
                .get(level)
                .and_then(|nodes| nodes.get(sibling_index))
                .copied()
                .unwrap_or(empty);
            siblings.push(sibling.to_bytes());
            current_index >>= 1;
        }

        Some(MerkleProof {
            leaf: self.leaves[index].to_bytes(),
            siblings,
            position: index as u64,
            root: self.compute_root().to_bytes(),
        })
    }
}

fn compute_frontier_from_leaves(
    leaves: &[MerkleHashOrchard],
    depth: usize,
) -> Vec<[u8; 32]> {
    let mut frontier: Vec<Option<MerkleHashOrchard>> = vec![None; depth];
    let mut size: u64 = 0;

    for leaf in leaves {
        let mut node = *leaf;
        let mut level = 0usize;
        let mut cursor = size;

        while (cursor & 1) == 1 {
            let left = frontier[level]
                .expect("frontier missing node for occupied subtree");
            node = MerkleHashOrchard::combine(Level::from(level as u8), &left, &node);
            frontier[level] = None;
            level += 1;
            cursor >>= 1;
        }

        frontier[level] = Some(node);
        size += 1;
    }

    frontier
        .into_iter()
        .enumerate()
        .map(|(level, node)| {
            node.unwrap_or_else(|| MerkleHashOrchard::empty_root(Level::from(level as u8)))
                .to_bytes()
        })
        .collect()
}

/// Sparse Merkle tree for nullifier set
///
/// Stores nullifiers as keys with value=1 (spent)
#[derive(Clone, Debug)]
pub struct SparseMerkleTree {
    /// Tree depth
    depth: usize,
    /// Stored nullifier leaves (position -> leaf hash)
    nullifiers: std::collections::BTreeMap<u64, [u8; 32]>,
    /// Cached root
    cached_root: Option<[u8; 32]>,
    /// Empty subtree roots by height
    empty_roots: Vec<[u8; 32]>,
}

impl SparseMerkleTree {
    pub fn new(depth: usize) -> Self {
        let empty_roots = build_empty_sparse_roots(depth);
        Self {
            depth,
            nullifiers: std::collections::BTreeMap::new(),
            cached_root: None,
            empty_roots,
        }
    }

    pub fn with_root(root: [u8; 32], depth: usize) -> Self {
        let empty_roots = build_empty_sparse_roots(depth);
        Self {
            depth,
            nullifiers: std::collections::BTreeMap::new(),
            cached_root: Some(root),
            empty_roots,
        }
    }

    /// Insert a nullifier (mark as spent)
    pub fn insert(&mut self, nullifier: [u8; 32]) {
        let position = sparse_position_for(nullifier, self.depth);
        self.nullifiers
            .insert(position, sparse_hash_leaf(nullifier));
        self.cached_root = None; // Invalidate cache
    }

    /// Check if nullifier exists
    pub fn contains(&self, nullifier: &[u8; 32]) -> bool {
        let position = sparse_position_for(*nullifier, self.depth);
        self.nullifiers.contains_key(&position)
    }

    /// Get the current root
    pub fn root(&mut self) -> [u8; 32] {
        if let Some(root) = self.cached_root {
            return root;
        }

        let root = self.compute_root();
        self.cached_root = Some(root);
        root
    }

    /// Number of stored nullifiers.
    pub fn len(&self) -> usize {
        self.nullifiers.len()
    }

    /// Compute sparse Merkle root
    fn compute_root(&self) -> [u8; 32] {
        if self.nullifiers.is_empty() {
            return self.empty_roots[self.depth];
        }

        self.subtree_hash(self.depth, 0)
    }

    /// Generate a proof of absence for a nullifier
    pub fn prove_absence(&self, nullifier: [u8; 32]) -> MerkleProof {
        let position = sparse_position_for(nullifier, self.depth);
        let siblings = self.sibling_hashes(position);
        MerkleProof {
            leaf: sparse_empty_leaf(),
            siblings,
            position,
            root: self.cached_root.unwrap_or(self.compute_root()),
        }
    }

    /// Generate a proof of membership for a nullifier
    pub fn prove_membership(&self, nullifier: [u8; 32]) -> MerkleProof {
        let position = sparse_position_for(nullifier, self.depth);
        let siblings = self.sibling_hashes(position);
        MerkleProof {
            leaf: sparse_hash_leaf(nullifier),
            siblings,
            position,
            root: self.cached_root.unwrap_or(self.compute_root()),
        }
    }

    fn sibling_hashes(&self, position: u64) -> Vec<[u8; 32]> {
        let mut siblings = Vec::with_capacity(self.depth);
        for height in 0..self.depth {
            let prefix = position >> height;
            let sibling_prefix = prefix ^ 1;
            let sibling = self.subtree_hash(height, sibling_prefix);
            siblings.push(sibling);
        }
        siblings
    }

    fn subtree_hash(&self, height: usize, prefix: u64) -> [u8; 32] {
        if !self.has_leaf_in_subtree(height, prefix) {
            return self.empty_roots[height];
        }

        if height == 0 {
            return self
                .nullifiers
                .get(&prefix)
                .copied()
                .unwrap_or_else(sparse_empty_leaf);
        }

        let left = self.subtree_hash(height - 1, prefix << 1);
        let right = self.subtree_hash(height - 1, (prefix << 1) | 1);
        hash_sparse_node(&left, &right)
    }

    fn has_leaf_in_subtree(&self, height: usize, prefix: u64) -> bool {
        let start = prefix << height;
        let end = (prefix + 1) << height;
        self.nullifiers.range(start..end).next().is_some()
    }
}

/// Merkle proof structure
#[derive(Clone, Debug, serde::Serialize, serde::Deserialize)]
pub struct MerkleProof {
    pub leaf: [u8; 32],
    pub siblings: Vec<[u8; 32]>,
    pub position: u64,
    pub root: [u8; 32],
}

impl MerkleProof {
    pub fn verify(&self) -> bool {
        let mut current = self.leaf;
        let mut pos = self.position;

        for sibling in &self.siblings {
            let (left, right) = if pos & 1 == 0 {
                (current, *sibling)
            } else {
                (*sibling, current)
            };
            current = hash_sparse_node(&left, &right);
            pos >>= 1;
        }

        current == self.root
    }
}

pub fn verify_commitment_proof(proof: &MerkleProof, expected_root: [u8; 32]) -> bool {
    let leaf = match Option::from(MerkleHashOrchard::from_bytes(&proof.leaf)) {
        Some(value) => value,
        None => return false,
    };

    let mut current = leaf;
    let mut pos = proof.position;

    for (level, sibling_bytes) in proof.siblings.iter().enumerate() {
        let sibling = match Option::from(MerkleHashOrchard::from_bytes(sibling_bytes)) {
            Some(value) => value,
            None => return false,
        };
        let (left, right) = if pos & 1 == 0 {
            (current, sibling)
        } else {
            (sibling, current)
        };
        current = MerkleHashOrchard::combine(Level::from(level as u8), &left, &right);
        pos >>= 1;
    }

    current.to_bytes() == expected_root
}

pub fn sparse_empty_leaf() -> [u8; 32] {
    hash_sparse_leaf(&[0u8; 32])
}

pub fn sparse_hash_leaf(value: [u8; 32]) -> [u8; 32] {
    hash_sparse_leaf(&value)
}

pub fn sparse_position_for(value: [u8; 32], depth: usize) -> u64 {
    let mut prefix = [0u8; 8];
    prefix.copy_from_slice(&value[..8]);
    let mut position = u64::from_le_bytes(prefix);
    if depth < 64 {
        position &= (1u64 << depth) - 1;
    }
    position
}

fn build_commitment_levels(
    leaves: &[MerkleHashOrchard],
    depth: usize,
) -> Vec<Vec<MerkleHashOrchard>> {
    let mut levels = Vec::with_capacity(depth + 1);
    levels.push(leaves.to_vec());

    for level in 0..depth {
        let current = levels[level].clone();
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

fn build_empty_sparse_roots(depth: usize) -> Vec<[u8; 32]> {
    let mut roots = Vec::with_capacity(depth + 1);
    roots.push(sparse_empty_leaf());

    for height in 0..depth {
        let current = roots[height];
        roots.push(hash_sparse_node(&current, &current));
    }

    roots
}

fn hash_sparse_leaf(value: &[u8; 32]) -> [u8; 32] {
    let mut hash = [0u8; 32];
    hash.copy_from_slice(
        Params::new()
            .hash_length(32)
            .to_state()
            .update(&[0x00])
            .update(value)
            .finalize()
            .as_bytes(),
    );
    hash
}

/// Hash two nodes together for the sparse Merkle tree.
fn hash_sparse_node(left: &[u8; 32], right: &[u8; 32]) -> [u8; 32] {
    let mut hash = [0u8; 32];
    hash.copy_from_slice(
        Params::new()
            .hash_length(32)
            .to_state()
            .update(&[0x01])
            .update(left)
            .update(right)
            .finalize()
            .as_bytes()
    );
    hash
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_incremental_tree() {
        let mut tree = IncrementalMerkleTree::new(32);

        let leaf1 = orchard::tree::MerkleHashOrchard::empty_leaf().to_bytes();
        let leaf2 = orchard::tree::MerkleHashOrchard::empty_leaf().to_bytes();

        tree.append(leaf1);
        let root1 = tree.root();

        tree.append(leaf2);
        let root2 = tree.root();

        assert_ne!(root1, root2);
        assert_eq!(tree.size(), 2);
    }

    #[test]
    fn test_merkle_proof() {
        let mut tree = IncrementalMerkleTree::new(32);

        let leaf = orchard::tree::MerkleHashOrchard::empty_leaf().to_bytes();
        tree.append(leaf);
        tree.append(leaf);
        tree.append(leaf);

        let proof = tree.prove(1).unwrap();
        assert!(verify_commitment_proof(&proof, tree.root()));
    }

    #[test]
    fn test_sparse_tree() {
        let mut tree = SparseMerkleTree::new(32);

        let nf1 = [10u8; 32];
        let nf2 = [20u8; 32];

        assert!(!tree.contains(&nf1));

        tree.insert(nf1);
        assert!(tree.contains(&nf1));
        assert!(!tree.contains(&nf2));

        let root1 = tree.root();
        tree.insert(nf2);
        let root2 = tree.root();

        assert_ne!(root1, root2);

        let absence = tree.prove_absence([30u8; 32]);
        assert!(absence.verify());

        let membership = tree.prove_membership(nf1);
        assert!(membership.verify());
    }
}
