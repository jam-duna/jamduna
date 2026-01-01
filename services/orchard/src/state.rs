#![allow(dead_code)]
/// Orchard service state structure (see services/orchard/docs/ORCHARD.md, "Service State")
///
/// This struct defines the consensus-critical state for the Orchard privacy service.
/// All fields are stored in the JAM state tree with Verkle proofs.

use alloc::vec::Vec;
use core::convert::TryInto;

use crate::errors::OrchardError;

/// Sparse Merkle proof for nullifier absence
#[derive(Debug, Clone)]
pub struct MerkleProof {
    pub leaf: [u8; 32],
    pub path: Vec<[u8; 32]>,
    pub root: [u8; 32],
}

/// Extrinsic types (see services/orchard/docs/ORCHARD.md "Extrinsic Interface")
#[derive(Debug, Clone)]
pub enum OrchardExtrinsic {
    /// Submit Zcash Orchard bundle (standard v5 transaction format)
    ///
    /// Uses production Orchard verification:
    /// - Bundle contains actions (spend/output pairs) with Halo2 proof
    /// - Anchor must equal state.commitment_root
    /// - Nullifiers must be absent (verified via sparse Merkle proofs)
    /// - Value balance must be zero (fully shielded, no transparent pool)
    BundleSubmit {
        /// Serialized Zcash v5 Orchard bundle (Bundle<Authorized, i64>)
        bundle_bytes: Vec<u8>,

        /// Sparse Merkle proofs for nullifier absence (JAM-specific)
        /// One proof per action's nullifier, proving leaf == 0 in nullifier tree
        nullifier_absence_proofs: Vec<MerkleProof>,
    },
}

impl MerkleProof {
    /// Verify sparse Merkle proof for nullifier absence
    pub fn verify(&self) -> bool {
        let mut current = self.leaf;
        for sibling in &self.path {
            current = hash_pair(&current, sibling);
        }
        current == self.root
    }
}

pub fn hash_pair(left: &[u8; 32], right: &[u8; 32]) -> [u8; 32] {
    use utils::hash_functions::blake2b_hash;
    let mut input = [0u8; 64];
    input[..32].copy_from_slice(left);
    input[32..].copy_from_slice(right);
    blake2b_hash(&input)
}

impl OrchardExtrinsic {
    const TAG_BUNDLE_SUBMIT: u8 = 0;

    /// Serialize to binary format: [tag:u8][bundle_len:u32][bundle_bytes][num_proofs:u32][proofs...]
    pub fn serialize(&self) -> Vec<u8> {
        let mut out = Vec::new();
        match self {
            OrchardExtrinsic::BundleSubmit {
                bundle_bytes,
                nullifier_absence_proofs,
            } => {
                out.push(Self::TAG_BUNDLE_SUBMIT);
                write_vec(&mut out, bundle_bytes);

                // Serialize proofs
                out.extend_from_slice(&(nullifier_absence_proofs.len() as u32).to_le_bytes());
                for proof in nullifier_absence_proofs {
                    out.extend_from_slice(&proof.leaf);
                    out.extend_from_slice(&(proof.path.len() as u32).to_le_bytes());
                    for node in &proof.path {
                        out.extend_from_slice(node);
                    }
                    out.extend_from_slice(&proof.root);
                }
            }
        }
        out
    }

    /// Deserialize from the format produced by `serialize`.
    pub fn deserialize(bytes: &[u8]) -> core::result::Result<Self, OrchardError> {
        let mut cursor = 0usize;
        let tag = *bytes.get(cursor).ok_or_else(|| OrchardError::ParseError("missing tag".into()))?;
        cursor += 1;

        match tag {
            Self::TAG_BUNDLE_SUBMIT => {
                let bundle_bytes = read_vec(bytes, &mut cursor)?;

                // Read proofs
                let num_proofs = u32::from_le_bytes(
                    read_fixed(bytes, &mut cursor, 4)?.try_into().unwrap()
                ) as usize;

                let mut nullifier_absence_proofs = Vec::with_capacity(num_proofs);
                for _ in 0..num_proofs {
                    let leaf = read_fixed(bytes, &mut cursor, 32)?.try_into().unwrap();
                    let path_len = u32::from_le_bytes(
                        read_fixed(bytes, &mut cursor, 4)?.try_into().unwrap()
                    ) as usize;

                    let mut path = Vec::with_capacity(path_len);
                    for _ in 0..path_len {
                        path.push(read_fixed(bytes, &mut cursor, 32)?.try_into().unwrap());
                    }

                    let root = read_fixed(bytes, &mut cursor, 32)?.try_into().unwrap();

                    nullifier_absence_proofs.push(MerkleProof { leaf, path, root });
                }

                Ok(OrchardExtrinsic::BundleSubmit {
                    bundle_bytes,
                    nullifier_absence_proofs,
                })
            }
            _ => Err(OrchardError::ParseError("unknown extrinsic tag".into())),
        }
    }
}

fn write_vec(out: &mut Vec<u8>, data: &[u8]) {
    out.extend_from_slice(&(data.len() as u32).to_le_bytes());
    out.extend_from_slice(data);
}

fn read_fixed<'a>(bytes: &'a [u8], cursor: &mut usize, n: usize) -> core::result::Result<&'a [u8], OrchardError> {
    let slice = bytes
        .get(*cursor..*cursor + n)
        .ok_or_else(|| OrchardError::ParseError("unexpected end of input".into()))?;
    *cursor += n;
    Ok(slice)
}

fn read_vec(bytes: &[u8], cursor: &mut usize) -> core::result::Result<Vec<u8>, OrchardError> {
    let len_bytes: [u8; 4] = bytes
        .get(*cursor..*cursor + 4)
        .ok_or_else(|| OrchardError::ParseError("unexpected end of input".into()))?
        .try_into()
        .unwrap();
    let len = u32::from_le_bytes(len_bytes) as usize;
    *cursor += 4;
    let data = bytes
        .get(*cursor..*cursor + len)
        .ok_or_else(|| OrchardError::ParseError("unexpected end of input".into()))?;
    *cursor += len;
    Ok(data.to_vec())
}

/// Format nullifier key as "nullifier_<hex>" to match documentation
pub fn format_nullifier_key(nullifier: &[u8; 32]) -> alloc::string::String {
    use alloc::format;
    format!("nullifier_{}", hex::encode(nullifier))
}
