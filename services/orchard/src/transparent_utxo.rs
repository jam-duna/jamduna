//! UTXO Merkle tree accumulator for transparent transactions
//!
//! Sorted Merkle tree implementation matching the Go builder's TransparentUtxoTree.
//! Uses SHA256d for leaf and node hashing.

use alloc::vec::Vec;
use alloc::collections::BTreeMap;
use sha2::{Sha256, Digest};
use crate::errors::{OrchardError, Result};

/// Outpoint uniquely identifies a transaction output
#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord)]
pub struct Outpoint {
    pub txid: [u8; 32],  // Internal byte order (big-endian)
    pub vout: u32,
}

/// UTXO data for a single unspent output
#[derive(Debug, Clone)]
pub struct UtxoData {
    pub value: u64,
    pub script_pubkey: Vec<u8>,
    pub height: u32,  // Block height created (for coinbase maturity)
    pub is_coinbase: bool,
}

/// Merkle proof for UTXO existence
#[derive(Debug, Clone)]
pub struct UtxoMerkleProof {
    pub outpoint: Outpoint,
    pub value: u64,
    pub script_pubkey: Vec<u8>,
    pub height: u32,
    pub is_coinbase: bool,
    pub tree_position: u64,
    pub siblings: Vec<[u8; 32]>,
}

/// Sorted Merkle tree of UTXOs
pub struct TransparentUtxoTree {
    utxos: BTreeMap<Outpoint, UtxoData>,
    root: [u8; 32],
    dirty: bool,
}

impl TransparentUtxoTree {
    /// Create a new empty UTXO tree
    pub fn new() -> Self {
        Self {
            utxos: BTreeMap::new(),
            root: [0u8; 32],
            dirty: true,
        }
    }

    /// Insert a UTXO into the tree
    pub fn insert(&mut self, outpoint: Outpoint, utxo: UtxoData) {
        self.utxos.insert(outpoint, utxo);
        self.dirty = true;
    }

    /// Remove a UTXO from the tree
    pub fn remove(&mut self, outpoint: &Outpoint) -> Result<()> {
        if self.utxos.remove(outpoint).is_none() {
            return Err(OrchardError::ParseError("UTXO not found".into()));
        }
        self.dirty = true;
        Ok(())
    }

    /// Get the Merkle root of the UTXO set
    pub fn get_root(&mut self) -> Result<[u8; 32]> {
        if self.dirty {
            self.recompute_root()?;
        }
        Ok(self.root)
    }

    /// Get the number of UTXOs in the tree
    pub fn size(&self) -> u64 {
        self.utxos.len() as u64
    }

    /// Check if a UTXO exists
    pub fn contains(&self, outpoint: &Outpoint) -> bool {
        self.utxos.contains_key(outpoint)
    }

    /// Get UTXO data
    pub fn get(&self, outpoint: &Outpoint) -> Option<&UtxoData> {
        self.utxos.get(outpoint)
    }

    /// Generate a Merkle proof for a UTXO
    pub fn get_proof(&self, outpoint: &Outpoint) -> Result<UtxoMerkleProof> {
        let utxo = self
            .utxos
            .get(outpoint)
            .ok_or_else(|| OrchardError::ParseError("UTXO not found".into()))?;

        // Find position in sorted order
        let sorted_outpoints: Vec<_> = self.utxos.keys().copied().collect();
        let position = sorted_outpoints
            .iter()
            .position(|op| op == outpoint)
            .ok_or_else(|| OrchardError::ParseError("UTXO position not found".into()))?;

        // Build Merkle path
        let leaves: Vec<[u8; 32]> = sorted_outpoints
            .iter()
            .map(|op| {
                let u = &self.utxos[op];
                hash_utxo_leaf(*op, u)
            })
            .collect();

        let siblings = compute_merkle_siblings(&leaves, position);

        Ok(UtxoMerkleProof {
            outpoint: *outpoint,
            value: utxo.value,
            script_pubkey: utxo.script_pubkey.clone(),
            height: utxo.height,
            is_coinbase: utxo.is_coinbase,
            tree_position: position as u64,
            siblings,
        })
    }

    /// Recompute the Merkle root
    fn recompute_root(&mut self) -> Result<()> {
        if self.utxos.is_empty() {
            self.root = [0u8; 32];
            self.dirty = false;
            return Ok(());
        }

        // Hash all UTXOs in sorted order
        let mut leaves = Vec::with_capacity(self.utxos.len());
        for (outpoint, utxo) in &self.utxos {
            leaves.push(hash_utxo_leaf(*outpoint, utxo));
        }

        // Build Merkle tree (Bitcoin-style)
        self.root = build_utxo_merkle_root(&leaves);
        self.dirty = false;
        Ok(())
    }
}

/// Hash a UTXO leaf using SHA256d
/// Leaf = SHA256d(txid || vout || value || script_hash || height || is_coinbase)
fn hash_utxo_leaf(outpoint: Outpoint, utxo: &UtxoData) -> [u8; 32] {
    let mut data = Vec::with_capacity(32 + 4 + 8 + 32 + 4 + 1);

    // Outpoint (36 bytes)
    data.extend_from_slice(&outpoint.txid);
    data.extend_from_slice(&outpoint.vout.to_le_bytes());

    // Value (8 bytes)
    data.extend_from_slice(&utxo.value.to_le_bytes());

    // Script hash (32 bytes)
    let script_hash = Sha256::digest(&utxo.script_pubkey);
    data.extend_from_slice(&script_hash);

    // Height (4 bytes)
    data.extend_from_slice(&utxo.height.to_le_bytes());

    // Coinbase flag (1 byte)
    data.push(u8::from(utxo.is_coinbase));

    // SHA256d
    sha256d(&data)
}

/// Build Bitcoin-style Merkle root from leaves
fn build_utxo_merkle_root(leaves: &[[u8; 32]]) -> [u8; 32] {
    if leaves.is_empty() {
        return [0u8; 32];
    }
    if leaves.len() == 1 {
        return leaves[0];
    }

    let mut level = leaves.to_vec();

    while level.len() > 1 {
        let mut next_level = Vec::new();

        for chunk in level.chunks(2) {
            let hash = if chunk.len() == 2 {
                // Hash pair
                let mut data = [0u8; 64];
                data[..32].copy_from_slice(&chunk[0]);
                data[32..].copy_from_slice(&chunk[1]);
                sha256d(&data)
            } else {
                // Odd node: duplicate last element (Bitcoin convention)
                let mut data = [0u8; 64];
                data[..32].copy_from_slice(&chunk[0]);
                data[32..].copy_from_slice(&chunk[0]);
                sha256d(&data)
            };
            next_level.push(hash);
        }

        level = next_level;
    }

    level[0]
}

/// Compute Merkle siblings for a proof
fn compute_merkle_siblings(leaves: &[[u8; 32]], position: usize) -> Vec<[u8; 32]> {
    if leaves.len() <= 1 {
        return Vec::new();
    }

    let mut siblings = Vec::new();
    let mut level = leaves.to_vec();
    let mut pos = position;

    while level.len() > 1 {
        // Get sibling
        let sibling_pos = if pos % 2 == 0 {
            pos + 1
        } else {
            pos - 1
        };

        let sibling = if sibling_pos < level.len() {
            level[sibling_pos]
        } else {
            // Odd node: sibling is itself (Bitcoin convention)
            level[pos]
        };

        siblings.push(sibling);

        // Build next level
        let mut next_level = Vec::new();
        for chunk in level.chunks(2) {
            let hash = if chunk.len() == 2 {
                let mut data = [0u8; 64];
                data[..32].copy_from_slice(&chunk[0]);
                data[32..].copy_from_slice(&chunk[1]);
                sha256d(&data)
            } else {
                let mut data = [0u8; 64];
                data[..32].copy_from_slice(&chunk[0]);
                data[32..].copy_from_slice(&chunk[0]);
                sha256d(&data)
            };
            next_level.push(hash);
        }

        level = next_level;
        pos /= 2;
    }

    siblings
}

/// Verify a Merkle proof against a root
pub fn verify_utxo_proof(proof: &UtxoMerkleProof, expected_root: &[u8; 32]) -> bool {
    // Compute leaf hash
    let leaf_hash = hash_utxo_leaf(
        proof.outpoint,
        &UtxoData {
            value: proof.value,
            script_pubkey: proof.script_pubkey.clone(),
            height: proof.height,
            is_coinbase: proof.is_coinbase,
        },
    );

    // Recompute root from proof
    let computed_root = recompute_root_from_proof(leaf_hash, proof.tree_position, &proof.siblings);

    computed_root == *expected_root
}

/// Recompute Merkle root from a proof path
fn recompute_root_from_proof(
    mut current: [u8; 32],
    mut position: u64,
    siblings: &[[u8; 32]],
) -> [u8; 32] {
    for sibling in siblings {
        let mut data = [0u8; 64];
        if position % 2 == 0 {
            // Current is left
            data[..32].copy_from_slice(&current);
            data[32..].copy_from_slice(sibling);
        } else {
            // Current is right
            data[..32].copy_from_slice(sibling);
            data[32..].copy_from_slice(&current);
        }
        current = sha256d(&data);
        position /= 2;
    }
    current
}

/// Double SHA256 (Bitcoin-style)
fn sha256d(data: &[u8]) -> [u8; 32] {
    let first = Sha256::digest(data);
    let second = Sha256::digest(&first);
    let mut result = [0u8; 32];
    result.copy_from_slice(&second);
    result
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_utxo_tree_basic() {
        let mut tree = TransparentUtxoTree::new();

        let outpoint = Outpoint {
            txid: [1u8; 32],
            vout: 0,
        };
        let utxo = UtxoData {
            value: 100_000_000,
            script_pubkey: vec![0x76, 0xa9, 0x14], // P2PKH prefix
            height: 1000,
            is_coinbase: false,
        };

        tree.insert(outpoint, utxo);
        assert_eq!(tree.size(), 1);
        assert!(tree.contains(&outpoint));

        let root = tree.get_root().unwrap();
        assert_ne!(root, [0u8; 32]);
    }

    #[test]
    fn test_utxo_proof() {
        let mut tree = TransparentUtxoTree::new();

        let outpoint = Outpoint {
            txid: [1u8; 32],
            vout: 0,
        };
        let utxo = UtxoData {
            value: 100_000_000,
            script_pubkey: vec![0x76, 0xa9, 0x14],
            height: 1000,
            is_coinbase: false,
        };

        tree.insert(outpoint, utxo);
        let root = tree.get_root().unwrap();

        let proof = tree.get_proof(&outpoint).unwrap();
        assert!(verify_utxo_proof(&proof, &root));
    }
}
