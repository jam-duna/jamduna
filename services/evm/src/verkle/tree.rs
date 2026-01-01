//! Minimal Verkle tree implementation for post-state validation
//!
//! Implements tree building and commitment computation required for
//! applying StateDiff and verifying post-state roots.

use alloc::boxed::Box;
use alloc::vec;
use alloc::collections::BTreeMap;
use ark_ff::{One, PrimeField};

use super::{BandersnatchPoint, Fr};
use crate::verkle_proof::{StateDiff, VerkleProofError};

const NODE_WIDTH: usize = 256;

/// Value update operation for leaf nodes
#[derive(Clone, Copy, Debug)]
pub enum ValueUpdate {
    /// No change to this suffix
    NoChange,
    /// Update to a new value (or insert if not present)
    Set([u8; 32]),
    /// Delete this suffix (remove from tree)
    Delete,
}

/// Minimal Verkle tree node for post-state validation
#[derive(Clone, Debug)]
pub enum VerkleNode {
    /// Internal node with 256 children and a commitment
    Internal {
        children: BTreeMap<u8, Box<VerkleNode>>,
        commitment: Option<BandersnatchPoint>,
    },
    /// Leaf node with stem and 256 values
    Leaf {
        stem: [u8; 31],
        values: [Option<[u8; 32]>; NODE_WIDTH],
        commitment: Option<BandersnatchPoint>,
    },
    /// Empty node
    Empty,
}

impl VerkleNode {
    /// Create new empty internal node
    pub fn new_internal() -> Self {
        VerkleNode::Internal {
            children: BTreeMap::new(),
            commitment: None,
        }
    }

    /// Create new leaf node with given stem
    pub fn new_leaf(stem: [u8; 31]) -> Self {
        VerkleNode::Leaf {
            stem,
            values: [None; NODE_WIDTH],
            commitment: None,
        }
    }

    /// Insert, update, or delete values at a stem, creating path as needed
    pub fn insert_values_at_stem(
        &mut self,
        stem: &[u8; 31],
        values: &[ValueUpdate; NODE_WIDTH],
        depth: usize,
    ) -> Result<(), VerkleProofError> {
        match self {
            VerkleNode::Empty => {
                // Convert empty to internal and recurse
                *self = VerkleNode::new_internal();
                self.insert_values_at_stem(stem, values, depth)
            }
            VerkleNode::Internal { children, commitment } => {
                if depth >= 31 {
                    return Err(VerkleProofError::PathMismatch);
                }

                let index = stem[depth];
                *commitment = None; // Invalidate cached commitment

                let child = children
                    .entry(index)
                    .or_insert_with(|| Box::new(VerkleNode::new_leaf(*stem)));

                child.insert_values_at_stem(stem, values, depth + 1)
            }
            VerkleNode::Leaf { stem: leaf_stem, values: leaf_values, commitment } => {
                if leaf_stem != stem {
                    return Err(VerkleProofError::PathMismatch);
                }

                // Apply updates using ValueUpdate enum
                for (i, update) in values.iter().enumerate() {
                    match update {
                        ValueUpdate::NoChange => {
                            // Don't modify this suffix
                        }
                        ValueUpdate::Set(value) => {
                            // Update to new value (or insert if not present)
                            leaf_values[i] = Some(*value);
                        }
                        ValueUpdate::Delete => {
                            // Remove value from tree (true deletion, not zero)
                            leaf_values[i] = None;
                        }
                    }
                }

                // If all suffixes were removed, drop the leaf entirely (go-verkle deletes empty stems).
                if leaf_values.iter().all(|v| v.is_none()) {
                    *self = VerkleNode::Empty;
                    return Ok(());
                }

                *commitment = None; // Invalidate cached commitment
                Ok(())
            }
        }
    }

    /// Compute and cache commitment for this node
    pub fn commit(&mut self, srs: &[BandersnatchPoint]) -> Result<BandersnatchPoint, VerkleProofError> {
        match self {
            VerkleNode::Empty => {
                Ok(BandersnatchPoint::identity())
            }
            VerkleNode::Internal { children, commitment } => {
                if let Some(c) = commitment {
                    return Ok(*c);
                }

                let mut poly = vec![Fr::from(0u64); NODE_WIDTH];
                for (index, child) in children.iter_mut() {
                    let child_commitment = child.commit(srs)?;
                    poly[*index as usize] = child_commitment.map_to_scalar_field();
                }

                let c = BandersnatchPoint::msm(srs, &poly);
                *commitment = Some(c);
                Ok(c)
            }
            VerkleNode::Leaf { stem, values, commitment, .. } => {
                if let Some(c) = commitment {
                    return Ok(*c);
                }

                let c = commit_leaf(stem, values, srs)?;
                *commitment = Some(c);
                Ok(c)
            }
        }
    }

    /// Get the root commitment
    pub fn root_commitment(&mut self, srs: &[BandersnatchPoint]) -> Result<BandersnatchPoint, VerkleProofError> {
        self.commit(srs)
    }
}

fn leaf_to_comms(poly: &mut [Fr], value: &[u8]) -> Result<(), VerkleProofError> {
    if value.is_empty() {
        return Ok(());
    }
    if value.len() > 32 {
        return Err(VerkleProofError::LengthMismatch);
    }

    let mut lo_with_marker = [0u8; 17];
    let lo_len = core::cmp::min(16, value.len());
    lo_with_marker[..lo_len].copy_from_slice(&value[..lo_len]);
    lo_with_marker[16] = 1;
    poly[0] = Fr::from_le_bytes_mod_order(&lo_with_marker);

    if value.len() >= 16 {
        let mut hi = [0u8; 16];
        let hi_len = value.len() - 16;
        hi[..hi_len].copy_from_slice(&value[16..]);
        poly[1] = Fr::from_le_bytes_mod_order(&hi);
    }

    Ok(())
}

fn commit_leaf(
    stem: &[u8; 31],
    values: &[Option<[u8; 32]>; NODE_WIDTH],
    srs: &[BandersnatchPoint],
) -> Result<BandersnatchPoint, VerkleProofError> {
    let mut c1_poly = vec![Fr::from(0u64); NODE_WIDTH];
    for (idx, maybe_val) in values.iter().enumerate().take(NODE_WIDTH / 2) {
        if let Some(v) = maybe_val {
            leaf_to_comms(&mut c1_poly[idx * 2..], v)?;
        }
    }
    let c1 = BandersnatchPoint::msm(srs, &c1_poly);

    let mut c2_poly = vec![Fr::from(0u64); NODE_WIDTH];
    for (idx, maybe_val) in values.iter().enumerate().skip(NODE_WIDTH / 2) {
        if let Some(v) = maybe_val {
            let offset = (idx - NODE_WIDTH / 2) * 2;
            leaf_to_comms(&mut c2_poly[offset..], v)?;
        }
    }
    let c2 = BandersnatchPoint::msm(srs, &c2_poly);

    let mut stem_bytes = [0u8; 32];
    stem_bytes[..stem.len()].copy_from_slice(stem);

    let mut poly = vec![Fr::from(0u64); NODE_WIDTH];
    poly[0] = Fr::one();
    poly[1] = Fr::from_le_bytes_mod_order(&stem_bytes);
    poly[2] = c1.map_to_scalar_field();
    poly[3] = c2.map_to_scalar_field();

    Ok(BandersnatchPoint::msm(srs, &poly))
}

/// Build a Verkle tree from StateDiff
pub fn tree_from_state_diff(state_diff: &StateDiff) -> Result<VerkleNode, VerkleProofError> {
    let mut root = VerkleNode::new_internal();

    for stem_diff in state_diff {
        let mut values = [ValueUpdate::NoChange; NODE_WIDTH];

        for suffix_diff in &stem_diff.suffix_diffs {
            values[suffix_diff.suffix as usize] = match suffix_diff.new_value {
                Some(v) => ValueUpdate::Set(v),
                None => ValueUpdate::Delete,
            };
        }

        root.insert_values_at_stem(&stem_diff.stem, &values, 0)?;
    }

    Ok(root)
}

/// Apply StateDiff to an existing tree (for post-state computation)
pub fn apply_state_diff_to_tree(
    mut tree: VerkleNode,
    state_diff: &StateDiff,
) -> Result<VerkleNode, VerkleProofError> {
    for stem_diff in state_diff {
        let mut values = [ValueUpdate::NoChange; NODE_WIDTH];

        for suffix_diff in &stem_diff.suffix_diffs {
            values[suffix_diff.suffix as usize] = match suffix_diff.new_value {
                Some(v) => ValueUpdate::Set(v),
                None => ValueUpdate::Delete,
            };
        }

        tree.insert_values_at_stem(&stem_diff.stem, &values, 0)?;
    }

    Ok(tree)
}
