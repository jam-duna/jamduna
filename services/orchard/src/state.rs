#![allow(dead_code)]
/// Orchard service state structure (see services/orchard/docs/ORCHARD.md, "Service State")
///
/// This struct defines the consensus-critical state for the Orchard privacy service.
/// All fields are stored in the JAM state tree with UBT proofs.

use alloc::vec::Vec;
use core::convert::TryInto;

use crate::errors::{OrchardError, Result};
use blake2::Blake2bVar;
use blake2::digest::{Update, VariableOutput};

/// Consolidated Orchard service state (stored under single key "orchard_state")
///
/// All state consolidated to minimize storage overhead and enable atomic updates.
/// Total size: 152 bytes (fixed-width, consensus-critical encoding)
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct OrchardServiceState {
    /// Orchard commitment tree root (Merkle root of all note commitments)
    pub commitment_root: [u8; 32],
    /// Number of commitments in the tree
    pub commitment_size: u64,
    /// Orchard nullifier tree root (Sparse Merkle Tree of nullifiers)
    pub nullifier_root: [u8; 32],
    /// Number of nullifiers in the set
    pub nullifier_size: u64,
    /// Transparent transaction Merkle root (all transparent txs)
    pub transparent_merkle_root: [u8; 32],
    /// Transparent UTXO set commitment (Merkle root of UTXO set)
    pub transparent_utxo_root: [u8; 32],
    /// Number of UTXOs in the transparent UTXO set
    pub transparent_utxo_size: u64,
}

impl OrchardServiceState {
    /// Create empty initial state
    pub fn new() -> Self {
        Self {
            commitment_root: [0u8; 32],
            commitment_size: 0,
            nullifier_root: [0u8; 32],
            nullifier_size: 0,
            transparent_merkle_root: [0u8; 32],
            transparent_utxo_root: [0u8; 32],
            transparent_utxo_size: 0,
        }
    }

    /// Serialize to fixed-width bytes (consensus-critical encoding)
    ///
    /// Layout: [commitment_root(32) | commitment_size(8) | nullifier_root(32) | nullifier_size(8) |
    ///          transparent_merkle_root(32) | transparent_utxo_root(32) | transparent_utxo_size(8)]
    /// Total: 152 bytes
    pub fn to_bytes(&self) -> [u8; 152] {
        let mut out = [0u8; 152];
        out[0..32].copy_from_slice(&self.commitment_root);
        out[32..40].copy_from_slice(&self.commitment_size.to_le_bytes());
        out[40..72].copy_from_slice(&self.nullifier_root);
        out[72..80].copy_from_slice(&self.nullifier_size.to_le_bytes());
        out[80..112].copy_from_slice(&self.transparent_merkle_root);
        out[112..144].copy_from_slice(&self.transparent_utxo_root);
        out[144..152].copy_from_slice(&self.transparent_utxo_size.to_le_bytes());
        out
    }

    /// Deserialize from bytes
    pub fn from_bytes(bytes: &[u8]) -> Result<Self> {
        if bytes.len() != 152 {
            return Err(OrchardError::InvalidStateLength {
                expected: 152,
                got: bytes.len(),
            });
        }
        Ok(Self {
            commitment_root: bytes[0..32].try_into().unwrap(),
            commitment_size: u64::from_le_bytes(bytes[32..40].try_into().unwrap()),
            nullifier_root: bytes[40..72].try_into().unwrap(),
            nullifier_size: u64::from_le_bytes(bytes[72..80].try_into().unwrap()),
            transparent_merkle_root: bytes[80..112].try_into().unwrap(),
            transparent_utxo_root: bytes[112..144].try_into().unwrap(),
            transparent_utxo_size: u64::from_le_bytes(bytes[144..152].try_into().unwrap()),
        })
    }
}

impl Default for OrchardServiceState {
    fn default() -> Self {
        Self::new()
    }
}

/// Sparse Merkle proof for nullifier absence
#[derive(Debug, Clone)]
pub struct MerkleProof {
    pub leaf: [u8; 32],
    pub path: Vec<[u8; 32]>,
    pub root: [u8; 32],
}

/// Extrinsic types (see services/orchard/docs/ORCHARD.md "Extrinsic Interface")
///
/// A work package must contain three extrinsics for verification:
/// 1. PreStateWitness - Merkle proofs for state reads
/// 2. PostStateWitness - Merkle proofs for state writes
/// 3. BundleProof - Halo2 proof + public inputs + Orchard bundle
///
/// NOTE: CompactBlock is derived from bundle_bytes, no separate extrinsic needed
#[derive(Debug, Clone)]
pub enum OrchardExtrinsic {
    /// Legacy bundle submit format used by older tests/tools.
    ///
    /// NOTE: This is kept for test compatibility; production uses BundleProof.
    BundleSubmit {
        bundle_bytes: Vec<u8>,
        nullifier_absence_proofs: Vec<MerkleProof>,
    },
    /// Submit pre-state witness with Merkle proofs for reads
    ///
    /// Proves nullifier absence and other read proofs against the claimed pre-state root
    /// (roots themselves are carried in the pre-state payload, not proven via JAM Merkle proofs).
    PreStateWitness {
        witness_bytes: Vec<u8>,
    },

    /// Submit post-state witness with Merkle proofs for writes
    ///
    /// Optional consistency witness for post-state roots (not JAM Merkle proofs).
    PostStateWitness {
        witness_bytes: Vec<u8>,
    },

    /// Submit the bundle proof, public inputs, and Orchard bundle for ZK verification.
    /// SECURITY: bundle_bytes contains the actual Orchard v5 bundle with signatures
    /// CompactBlock is derived from bundle_bytes during verification
    BundleProof {
        vk_id: u32,
        public_inputs: Vec<[u8; 32]>,
        proof_bytes: Vec<u8>,
        bundle_bytes: Vec<u8>,  // Zcash v5 Orchard bundle (signatures + nullifiers + commitments)
    },
    /// Submit a V6 Orchard bundle (ZSA or Swap) plus optional IssueBundle bytes.
    ///
    /// Proof bytes are embedded inside the V6 Orchard bundle (per action group).
    BundleProofV6 {
        bundle_type: crate::nu7_types::OrchardBundleType,
        orchard_bundle_bytes: Vec<u8>,
        issue_bundle_bytes: Option<Vec<u8>>,
    },

    /// Submit transparent transaction data with UTXO Merkle proofs
    ///
    /// Contains Zcash v5 transparent-only transactions and inclusion proofs
    /// for spent UTXOs against the pre-state UTXO root.
    TransparentTxData {
        data_bytes: Vec<u8>,
    },
}

impl MerkleProof {
    /// Verify sparse Merkle proof with a known leaf position.
    pub fn verify_with_position(&self, mut position: u64) -> bool {
        let mut current = self.leaf;
        for sibling in &self.path {
            let (left, right) = if position & 1 == 0 {
                (current, *sibling)
            } else {
                (*sibling, current)
            };
            current = hash_sparse_node(&left, &right);
            position >>= 1;
        }
        current == self.root
    }

    /// Verify Merkle proof by folding siblings in-order (legacy compatibility).
    pub fn verify(&self) -> bool {
        let mut current = self.leaf;
        for sibling in &self.path {
            current = hash_pair(&current, sibling);
        }
        current == self.root
    }
}

pub fn sparse_empty_leaf() -> [u8; 32] {
    sparse_hash_leaf(&[0u8; 32])
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

pub fn sparse_hash_leaf(value: &[u8; 32]) -> [u8; 32] {
    let mut hasher = Blake2bVar::new(32).expect("blake2b config");
    hasher.update(&[0x00]);
    hasher.update(value);
    let mut out = [0u8; 32];
    hasher.finalize_variable(&mut out).expect("blake2b finalize");
    out
}

pub fn hash_sparse_node(left: &[u8; 32], right: &[u8; 32]) -> [u8; 32] {
    let mut hasher = Blake2bVar::new(32).expect("blake2b config");
    hasher.update(&[0x01]);
    hasher.update(left);
    hasher.update(right);
    let mut out = [0u8; 32];
    hasher.finalize_variable(&mut out).expect("blake2b finalize");
    out
}

/// Hash a left/right pair (legacy helper for tests).
pub fn hash_pair(left: &[u8; 32], right: &[u8; 32]) -> [u8; 32] {
    hash_sparse_node(left, right)
}

impl OrchardExtrinsic {
    const TAG_BUNDLE_SUBMIT: u8 = 0;
    const TAG_PRE_STATE_WITNESS: u8 = 1;
    const TAG_POST_STATE_WITNESS: u8 = 2;
    const TAG_BUNDLE_PROOF: u8 = 3;
    const TAG_TRANSPARENT_TX_DATA: u8 = 4;
    const TAG_BUNDLE_PROOF_V6: u8 = 5;

    /// Serialize to binary format: [tag:u8][tag-specific payload]
    pub fn serialize(&self) -> Vec<u8> {
        let mut out = Vec::new();
        match self {
            OrchardExtrinsic::BundleSubmit { bundle_bytes, nullifier_absence_proofs } => {
                out.push(Self::TAG_BUNDLE_SUBMIT);
                write_vec(&mut out, bundle_bytes);
                write_merkle_proofs(&mut out, nullifier_absence_proofs);
            }
            OrchardExtrinsic::PreStateWitness { witness_bytes } => {
                out.push(Self::TAG_PRE_STATE_WITNESS);
                write_vec(&mut out, witness_bytes);
            }
            OrchardExtrinsic::PostStateWitness { witness_bytes } => {
                out.push(Self::TAG_POST_STATE_WITNESS);
                write_vec(&mut out, witness_bytes);
            }
            OrchardExtrinsic::BundleProof { vk_id, public_inputs, proof_bytes, bundle_bytes } => {
                out.push(Self::TAG_BUNDLE_PROOF);
                out.extend_from_slice(&vk_id.to_le_bytes());
                write_inputs(&mut out, public_inputs);
                write_vec(&mut out, proof_bytes);
                write_vec(&mut out, bundle_bytes);
            }
            OrchardExtrinsic::BundleProofV6 {
                bundle_type,
                orchard_bundle_bytes,
                issue_bundle_bytes,
            } => {
                out.push(Self::TAG_BUNDLE_PROOF_V6);
                out.push(*bundle_type as u8);
                write_vec(&mut out, orchard_bundle_bytes);
                match issue_bundle_bytes {
                    Some(issue_bundle) => {
                        out.push(0x01);
                        write_vec(&mut out, issue_bundle);
                    }
                    None => {
                        out.push(0x00);
                    }
                }
            }
            OrchardExtrinsic::TransparentTxData { data_bytes } => {
                out.push(Self::TAG_TRANSPARENT_TX_DATA);
                write_vec(&mut out, data_bytes);
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
                let nullifier_absence_proofs = read_merkle_proofs(bytes, &mut cursor)?;
                Ok(OrchardExtrinsic::BundleSubmit {
                    bundle_bytes,
                    nullifier_absence_proofs,
                })
            }
            Self::TAG_PRE_STATE_WITNESS => {
                let witness_bytes = read_vec(bytes, &mut cursor)?;
                Ok(OrchardExtrinsic::PreStateWitness { witness_bytes })
            }
            Self::TAG_POST_STATE_WITNESS => {
                let witness_bytes = read_vec(bytes, &mut cursor)?;
                Ok(OrchardExtrinsic::PostStateWitness { witness_bytes })
            }
            Self::TAG_BUNDLE_PROOF => {
                let vk_id = read_u32(bytes, &mut cursor)?;
                let public_inputs = read_inputs(bytes, &mut cursor)?;
                let proof_bytes = read_vec(bytes, &mut cursor)?;
                let bundle_bytes = read_vec(bytes, &mut cursor)?;
                Ok(OrchardExtrinsic::BundleProof {
                    vk_id,
                    public_inputs,
                    proof_bytes,
                    bundle_bytes,
                })
            }
            Self::TAG_BUNDLE_PROOF_V6 => {
                let bundle_type = read_u8(bytes, &mut cursor)?;
                let bundle_type = crate::nu7_types::OrchardBundleType::from_u8(bundle_type)
                    .ok_or_else(|| OrchardError::ParseError("invalid V6 bundle type".into()))?;
                let orchard_bundle_bytes = read_vec(bytes, &mut cursor)?;
                let issue_bundle_bytes = read_optional_vec(bytes, &mut cursor)?;
                Ok(OrchardExtrinsic::BundleProofV6 {
                    bundle_type,
                    orchard_bundle_bytes,
                    issue_bundle_bytes,
                })
            }
            Self::TAG_TRANSPARENT_TX_DATA => {
                let data_bytes = read_vec(bytes, &mut cursor)?;
                Ok(OrchardExtrinsic::TransparentTxData { data_bytes })
            }
            _ => Err(OrchardError::ParseError("unknown extrinsic tag".into())),
        }
    }
}

fn write_vec(out: &mut Vec<u8>, data: &[u8]) {
    out.extend_from_slice(&(data.len() as u32).to_le_bytes());
    out.extend_from_slice(data);
}

fn write_inputs(out: &mut Vec<u8>, inputs: &[[u8; 32]]) {
    out.extend_from_slice(&(inputs.len() as u32).to_le_bytes());
    for input in inputs {
        out.extend_from_slice(input);
    }
}

fn read_optional_vec(bytes: &[u8], cursor: &mut usize) -> crate::errors::Result<Option<Vec<u8>>> {
    let tag = read_u8(bytes, cursor)?;
    if tag == 0x00 {
        return Ok(None);
    }
    if tag != 0x01 {
        return Err(OrchardError::ParseError("invalid optional vec tag".into()));
    }
    let data = read_vec(bytes, cursor)?;
    Ok(Some(data))
}

fn write_merkle_proofs(out: &mut Vec<u8>, proofs: &[MerkleProof]) {
    out.extend_from_slice(&(proofs.len() as u32).to_le_bytes());
    for proof in proofs {
        out.extend_from_slice(&proof.leaf);
        out.extend_from_slice(&(proof.path.len() as u32).to_le_bytes());
        for node in &proof.path {
            out.extend_from_slice(node);
        }
        out.extend_from_slice(&proof.root);
    }
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

fn read_u32(bytes: &[u8], cursor: &mut usize) -> core::result::Result<u32, OrchardError> {
    let data = read_fixed(bytes, cursor, 4)?;
    Ok(u32::from_le_bytes(data.try_into().unwrap()))
}

fn read_u8(bytes: &[u8], cursor: &mut usize) -> core::result::Result<u8, OrchardError> {
    let data = read_fixed(bytes, cursor, 1)?;
    Ok(data[0])
}

fn read_inputs(bytes: &[u8], cursor: &mut usize) -> core::result::Result<Vec<[u8; 32]>, OrchardError> {
    let count_bytes: [u8; 4] = bytes
        .get(*cursor..*cursor + 4)
        .ok_or_else(|| OrchardError::ParseError("unexpected end of input".into()))?
        .try_into()
        .unwrap();
    let count = u32::from_le_bytes(count_bytes) as usize;
    *cursor += 4;
    let mut inputs = Vec::with_capacity(count);
    for _ in 0..count {
        let data = read_fixed(bytes, cursor, 32)?;
        let mut out = [0u8; 32];
        out.copy_from_slice(data);
        inputs.push(out);
    }
    Ok(inputs)
}

fn read_merkle_proofs(bytes: &[u8], cursor: &mut usize) -> core::result::Result<Vec<MerkleProof>, OrchardError> {
    let count_bytes: [u8; 4] = bytes
        .get(*cursor..*cursor + 4)
        .ok_or_else(|| OrchardError::ParseError("unexpected end of input".into()))?
        .try_into()
        .unwrap();
    let count = u32::from_le_bytes(count_bytes) as usize;
    *cursor += 4;
    let mut proofs = Vec::with_capacity(count);

    for _ in 0..count {
        let leaf_bytes = read_fixed(bytes, cursor, 32)?;
        let mut leaf = [0u8; 32];
        leaf.copy_from_slice(leaf_bytes);

        let path_len_bytes = read_fixed(bytes, cursor, 4)?;
        let path_len = u32::from_le_bytes(path_len_bytes.try_into().unwrap()) as usize;
        let mut path = Vec::with_capacity(path_len);
        for _ in 0..path_len {
            let node_bytes = read_fixed(bytes, cursor, 32)?;
            let mut node = [0u8; 32];
            node.copy_from_slice(node_bytes);
            path.push(node);
        }

        let root_bytes = read_fixed(bytes, cursor, 32)?;
        let mut root = [0u8; 32];
        root.copy_from_slice(root_bytes);

        proofs.push(MerkleProof { leaf, path, root });
    }

    Ok(proofs)
}

/// Format nullifier key as "nullifier_<hex>" to match documentation
pub fn format_nullifier_key(nullifier: &[u8; 32]) -> alloc::string::String {
    use alloc::format;
    format!("nullifier_{}", hex::encode(nullifier))
}
