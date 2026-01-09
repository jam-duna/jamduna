/// CompactBlock module for Orchard light client protocol
///
/// This module implements the CompactBlock structure following the Zcash light client
/// specification adapted for Orchard zero-knowledge transactions. CompactBlocks contain
/// only the minimal data needed by light clients to:
/// 1. Detect payments to their shielded addresses
/// 2. Detect spends of their shielded notes
/// 3. Update witnesses for generating new proofs
/// 4. Track nullifiers for double-spend prevention

use alloc::vec::Vec;
use core::fmt;
use serde::{Deserialize, Serialize};

/// The wire format version for storage compatibility
pub const COMPACT_BLOCK_PROTO_VERSION: u32 = 1;

/// Compact representation of a Orchard block
///
/// This contains ONLY the data needed by light clients for:
/// - Detecting incoming shielded payments
/// - Detecting outgoing shielded spends
/// - Maintaining witness trees for proof generation
/// - Tracking nullifiers for double-spend prevention
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct CompactBlock {
    /// Wire format version for storage
    pub proto_version: u32,

    /// Block height in the chain
    pub height: u64,

    /// Block hash (32 bytes)
    pub hash: [u8; 32],

    /// Previous block hash (32 bytes)
    pub prev_hash: [u8; 32],

    /// Unix epoch timestamp when block was created
    pub time: u32,

    /// Full block header for proof verification
    pub header: Vec<u8>,

    /// Compact transactions in this block
    pub vtx: Vec<CompactTx>,

    /// Chain metadata about commitment tree state
    pub chain_metadata: ChainMetadata,
}

/// Information about the chain state as of a given block
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq, Default)]
pub struct ChainMetadata {
    /// Size of the Orchard note commitment tree at end of this block
    pub orchard_commitment_tree_size: u32,
}

impl CompactBlock {
    /// Create a new CompactBlock
    pub fn new(
        height: u64,
        hash: [u8; 32],
        prev_hash: [u8; 32],
        time: u32,
        header: Vec<u8>,
    ) -> Self {
        Self {
            proto_version: COMPACT_BLOCK_PROTO_VERSION,
            height,
            hash,
            prev_hash,
            time,
            header,
            vtx: Vec::new(),
            chain_metadata: ChainMetadata::default(),
        }
    }

    /// Add a compact transaction to this block
    pub fn add_transaction(&mut self, tx: CompactTx) {
        self.vtx.push(tx);
    }

    /// Get the number of transactions in this block
    pub fn transaction_count(&self) -> usize {
        self.vtx.len()
    }

    /// Check if block contains any nullifiers (for spend detection)
    pub fn has_nullifiers(&self) -> bool {
        self.vtx.iter().any(|tx| !tx.nullifiers.is_empty())
    }

    /// Check if block contains any commitments (for payment detection)
    pub fn has_commitments(&self) -> bool {
        self.vtx.iter().any(|tx| !tx.commitments.is_empty())
    }

    /// Get all nullifiers in this block (for double-spend checking)
    pub fn get_all_nullifiers(&self) -> Vec<[u8; 32]> {
        let mut nullifiers = Vec::new();
        for tx in &self.vtx {
            nullifiers.extend_from_slice(&tx.nullifiers);
        }
        nullifiers
    }

    /// Get all commitments in this block (for payment scanning)
    pub fn get_all_commitments(&self) -> Vec<OrchardCompactOutput> {
        let mut commitments = Vec::new();
        for tx in &self.vtx {
            commitments.extend_from_slice(&tx.commitments);
        }
        commitments
    }

    /// Serialize to bytes for storage/transmission
    pub fn to_bytes(&self) -> Result<Vec<u8>, crate::errors::OrchardError> {
        serde_cbor::to_vec(self).map_err(|_| crate::errors::OrchardError::SerializationError("CBOR serialization failed".into()))
    }

    /// Deserialize from bytes
    pub fn from_bytes(data: &[u8]) -> Result<Self, crate::errors::OrchardError> {
        serde_cbor::from_slice(data).map_err(|_| crate::errors::OrchardError::SerializationError("CBOR deserialization failed".into()))
    }
}

/// Compact representation of a Orchard transaction
///
/// Contains only the cryptographic elements needed by light clients:
/// - Transaction index and ID for correlation
/// - Nullifiers for spend detection
/// - Commitments for payment detection
/// - Encrypted ciphertext for trial decryption
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct CompactTx {
    /// Transaction index within the block
    pub index: u64,

    /// Transaction ID (32 bytes)
    pub txid: [u8; 32],

    /// Nullifiers for spent notes (32 bytes each)
    pub nullifiers: Vec<[u8; 32]>,

    /// Compact output commitments for new notes
    pub commitments: Vec<OrchardCompactOutput>,
}

/// Compact representation of a Orchard output (new note)
///
/// This is the minimal data needed for light clients to:
/// - Attempt trial decryption to detect payments
/// - Extract note value and metadata if decryption succeeds
/// - Update witness trees for proof generation
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct OrchardCompactOutput {
    /// Note commitment (32 bytes)
    pub commitment: [u8; 32],

    /// Ephemeral public key for ECIES encryption (32 bytes)
    pub ephemeral_key: [u8; 32],

    /// Encrypted note data (first 52 bytes for trial decryption)
    #[serde(with = "serde_bytes")]
    pub ciphertext: Vec<u8>,
}

/// Error types for compact block operations
#[derive(Debug, Clone)]
pub enum OrchardError {
    /// Serialization/deserialization error
    SerializationError,
    /// Invalid block data
    InvalidBlock,
    /// Missing required field
    MissingField,
}

impl fmt::Display for OrchardError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            OrchardError::SerializationError => write!(f, "Serialization error"),
            OrchardError::InvalidBlock => write!(f, "Invalid block data"),
            OrchardError::MissingField => write!(f, "Missing required field"),
        }
    }
}

impl CompactTx {
    /// Create a new CompactTx
    pub fn new(index: u64, txid: [u8; 32]) -> Self {
        Self {
            index,
            txid,
            nullifiers: Vec::new(),
            commitments: Vec::new(),
        }
    }

    /// Add a nullifier (spent note)
    pub fn add_nullifier(&mut self, nullifier: [u8; 32]) {
        self.nullifiers.push(nullifier);
    }

    /// Add a compact output (new note commitment)
    pub fn add_commitment(&mut self, output: OrchardCompactOutput) {
        self.commitments.push(output);
    }

    /// Check if this transaction contains a specific nullifier
    pub fn contains_nullifier(&self, nullifier: &[u8; 32]) -> bool {
        self.nullifiers.contains(nullifier)
    }

    /// Get commitment count
    pub fn commitment_count(&self) -> usize {
        self.commitments.len()
    }

    /// Get nullifier count
    pub fn nullifier_count(&self) -> usize {
        self.nullifiers.len()
    }
}

impl OrchardCompactOutput {
    /// Create a new compact output
    pub fn new(
        commitment: [u8; 32],
        ephemeral_key: [u8; 32],
        ciphertext: Vec<u8>,
    ) -> Self {
        Self {
            commitment,
            ephemeral_key,
            ciphertext,
        }
    }

    /// Get the commitment hash for this output
    pub fn get_commitment(&self) -> [u8; 32] {
        self.commitment
    }

    /// Get ephemeral key for decryption attempts
    pub fn get_ephemeral_key(&self) -> [u8; 32] {
        self.ephemeral_key
    }

    /// Get ciphertext prefix for trial decryption
    pub fn get_ciphertext(&self) -> &Vec<u8> {
        &self.ciphertext
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_compact_block_creation() {
        let mut block = CompactBlock::new(
            100,
            [1u8; 32],
            [2u8; 32],
            1234567890,
            vec![0xde, 0xad, 0xbe, 0xef],
        );

        assert_eq!(block.height, 100);
        assert_eq!(block.hash, [1u8; 32]);
        assert_eq!(block.prev_hash, [2u8; 32]);
        assert_eq!(block.time, 1234567890);
        assert_eq!(block.transaction_count(), 0);
        assert!(!block.has_nullifiers());
        assert!(!block.has_commitments());

        // Add a transaction
        let mut tx = CompactTx::new(0, [3u8; 32]);
        tx.add_nullifier([4u8; 32]);
        tx.add_commitment(OrchardCompactOutput::new(
            [5u8; 32],
            [6u8; 32],
            vec![7u8; 52],
        ));

        block.add_transaction(tx);

        assert_eq!(block.transaction_count(), 1);
        assert!(block.has_nullifiers());
        assert!(block.has_commitments());

        let nullifiers = block.get_all_nullifiers();
        assert_eq!(nullifiers.len(), 1);
        assert_eq!(nullifiers[0], [4u8; 32]);

        let commitments = block.get_all_commitments();
        assert_eq!(commitments.len(), 1);
        assert_eq!(commitments[0].commitment, [5u8; 32]);
    }

    #[test]
    fn test_compact_tx_operations() {
        let mut tx = CompactTx::new(42, [0xaa; 32]);

        assert_eq!(tx.index, 42);
        assert_eq!(tx.txid, [0xaa; 32]);
        assert_eq!(tx.nullifier_count(), 0);
        assert_eq!(tx.commitment_count(), 0);

        // Add nullifiers
        tx.add_nullifier([0xbb; 32]);
        tx.add_nullifier([0xcc; 32]);
        assert_eq!(tx.nullifier_count(), 2);
        assert!(tx.contains_nullifier(&[0xbb; 32]));
        assert!(tx.contains_nullifier(&[0xcc; 32]));
        assert!(!tx.contains_nullifier(&[0xdd; 32]));

        // Add commitments
        tx.add_commitment(OrchardCompactOutput::new(
            [0xee; 32],
            [0xff; 32],
            vec![0x11; 52],
        ));
        assert_eq!(tx.commitment_count(), 1);
    }

    #[test]
    fn test_serialization() {
        let block = CompactBlock::new(
            200,
            [0x42; 32],
            [0x43; 32],
            9999999,
            vec![1, 2, 3, 4],
        );

        let bytes = block.to_bytes().expect("Serialization should succeed");
        assert!(!bytes.is_empty());

        let recovered = CompactBlock::from_bytes(&bytes)
            .expect("Deserialization should succeed");

        assert_eq!(block, recovered);
    }

    #[test]
    fn test_compact_output() {
        let output = OrchardCompactOutput::new(
            [0x12; 32],
            [0x34; 32],
            vec![0x56; 52],
        );

        assert_eq!(output.get_commitment(), [0x12; 32]);
        assert_eq!(output.get_ephemeral_key(), [0x34; 32]);
        assert_eq!(output.get_ciphertext(), &vec![0x56; 52]);
    }
}
