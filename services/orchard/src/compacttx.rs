/// CompactTx module for Orchard light client protocol
///
/// This module implements the CompactTx structure and related types for the Orchard
/// zero-knowledge transaction system. It follows the Zcash light client specification
/// but is adapted for Orchard's unique requirements including Orchard-style commitments
/// and nullifiers without legacy Sprout/Sapling support.

use alloc::vec::Vec;
use alloc::string::ToString;
use serde::{Deserialize, Serialize};
use crate::errors::OrchardError;
use crate::crypto::poseidon_hash_with_domain;

const COMPACT_TX_SHARED_SECRET_DOMAIN: &str = "compact_tx_shared_secret_v1";

/// Extrinsic types supported by Orchard compact transactions
#[derive(Debug, Clone, Copy, Serialize, Deserialize, PartialEq, Eq)]
#[repr(u32)]
pub enum ExtrinsicType {
    /// Deposit public funds into shielded pool
    DepositPublic = 0,
    /// Submit private shielded transaction
    SubmitPrivate = 1,
    /// Withdraw from shielded pool to public account
    WithdrawPublic = 2,
    /// Issue new tokens with compliance constraints
    IssuanceV1 = 3,
    /// Aggregate multiple transactions into single proof
    BatchAggV1 = 4,
}

impl From<u32> for ExtrinsicType {
    fn from(value: u32) -> Self {
        match value {
            0 => ExtrinsicType::DepositPublic,
            1 => ExtrinsicType::SubmitPrivate,
            2 => ExtrinsicType::WithdrawPublic,
            3 => ExtrinsicType::IssuanceV1,
            4 => ExtrinsicType::BatchAggV1,
            _ => ExtrinsicType::SubmitPrivate, // Default fallback
        }
    }
}

/// Compact representation of a Orchard transaction
///
/// Contains minimum information needed by light clients to:
/// - Detect relevant payments and spends
/// - Update witness trees for proof generation
/// - Track nullifiers for double-spend prevention
/// - Extract transaction metadata
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct CompactTx {
    /// Transaction index within the block
    pub index: u64,

    /// Transaction ID (hash) - 32 bytes in protocol order
    pub txid: [u8; 32],

    /// Transaction fee in zatoshis (if computable)
    pub fee: Option<u32>,

    /// Type of Orchard extrinsic
    pub extrinsic_type: ExtrinsicType,

    /// Nullifiers for spent notes (spend detection)
    pub spends: Vec<CompactSpend>,

    /// Compact outputs for new notes (payment detection)
    pub outputs: Vec<CompactOutput>,

    /// Compact actions combining spends and outputs (Orchard-style)
    pub actions: Vec<CompactAction>,
}

/// Compact representation of a spend (nullifier)
/// Equivalent to CompactSaplingSpend but for Orchard
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct CompactSpend {
    /// Nullifier (32 bytes) for double-spend prevention
    pub nullifier: [u8; 32],
}

/// Compact representation of an output (new note)
/// Similar to CompactSaplingOutput but adapted for Orchard
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct CompactOutput {
    /// Note commitment (32 bytes)
    pub commitment: [u8; 32],

    /// Ephemeral public key for ECIES encryption (32 bytes)
    pub ephemeral_key: [u8; 32],

    /// First 52 bytes of encrypted note ciphertext
    #[serde(with = "serde_bytes")]
    pub ciphertext: Vec<u8>,
}

/// Compact representation of a Orchard action (Orchard-style)
/// Combines spend and output in a single cryptographic action
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct CompactAction {
    /// Nullifier of the input note (32 bytes)
    pub nullifier: [u8; 32],

    /// Commitment of the output note (32 bytes)
    pub commitment: [u8; 32],

    /// Ephemeral public key (32 bytes)
    pub ephemeral_key: [u8; 32],

    /// First 52 bytes of encrypted ciphertext
    #[serde(with = "serde_bytes")]
    pub ciphertext: Vec<u8>,
}

/// Transaction filter for querying specific transactions
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct TxFilter {
    /// Block identifier (height or hash)
    pub block: Option<BlockID>,

    /// Transaction index within block
    pub index: Option<u64>,

    /// Direct transaction hash lookup
    pub hash: Option<[u8; 32]>,
}

/// Block identifier for transaction filters
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct BlockID {
    /// Block height
    pub height: Option<u64>,

    /// Block hash
    pub hash: Option<[u8; 32]>,
}

impl CompactTx {
    /// Create a new CompactTx
    pub fn new(index: u64, txid: [u8; 32], extrinsic_type: ExtrinsicType) -> Self {
        Self {
            index,
            txid,
            fee: None,
            extrinsic_type,
            spends: Vec::new(),
            outputs: Vec::new(),
            actions: Vec::new(),
        }
    }

    /// Add a spend (nullifier) to this transaction
    pub fn add_spend(&mut self, nullifier: [u8; 32]) {
        self.spends.push(CompactSpend { nullifier });
    }

    /// Add an output (commitment) to this transaction
    pub fn add_output(&mut self, commitment: [u8; 32], ephemeral_key: [u8; 32], ciphertext: Vec<u8>) {
        self.outputs.push(CompactOutput {
            commitment,
            ephemeral_key,
            ciphertext,
        });
    }

    /// Add an action (spend+output) to this transaction
    pub fn add_action(&mut self,
        nullifier: [u8; 32],
        commitment: [u8; 32],
        ephemeral_key: [u8; 32],
        ciphertext: Vec<u8>
    ) {
        self.actions.push(CompactAction {
            nullifier,
            commitment,
            ephemeral_key,
            ciphertext,
        });
    }

    /// Set transaction fee
    pub fn set_fee(&mut self, fee: u32) {
        self.fee = Some(fee);
    }

    /// Get all nullifiers in this transaction
    pub fn get_nullifiers(&self) -> Vec<[u8; 32]> {
        let mut nullifiers = Vec::new();

        // Add spend nullifiers
        for spend in &self.spends {
            nullifiers.push(spend.nullifier);
        }

        // Add action nullifiers
        for action in &self.actions {
            nullifiers.push(action.nullifier);
        }

        nullifiers
    }

    /// Get all commitments in this transaction
    pub fn get_commitments(&self) -> Vec<[u8; 32]> {
        let mut commitments = Vec::new();

        // Add output commitments
        for output in &self.outputs {
            commitments.push(output.commitment);
        }

        // Add action commitments
        for action in &self.actions {
            commitments.push(action.commitment);
        }

        commitments
    }

    /// Check if transaction contains a specific nullifier
    pub fn contains_nullifier(&self, nullifier: &[u8; 32]) -> bool {
        // Check spends
        for spend in &self.spends {
            if &spend.nullifier == nullifier {
                return true;
            }
        }

        // Check actions
        for action in &self.actions {
            if &action.nullifier == nullifier {
                return true;
            }
        }

        false
    }

    /// Check if transaction contains a specific commitment
    pub fn contains_commitment(&self, commitment: &[u8; 32]) -> bool {
        // Check outputs
        for output in &self.outputs {
            if &output.commitment == commitment {
                return true;
            }
        }

        // Check actions
        for action in &self.actions {
            if &action.commitment == commitment {
                return true;
            }
        }

        false
    }

    /// Get total spend count
    pub fn spend_count(&self) -> usize {
        self.spends.len() + self.actions.len()
    }

    /// Get total output count
    pub fn output_count(&self) -> usize {
        self.outputs.len() + self.actions.len()
    }

    /// Check if this is a coinbase transaction (index 0)
    pub fn is_coinbase(&self) -> bool {
        self.index == 0
    }

    /// Serialize to bytes
    pub fn to_bytes(&self) -> Result<Vec<u8>, OrchardError> {
        serde_cbor::to_vec(self).map_err(|e| OrchardError::SerializationError(e.to_string()))
    }

    /// Deserialize from bytes
    pub fn from_bytes(data: &[u8]) -> Result<Self, OrchardError> {
        serde_cbor::from_slice(data).map_err(|e| OrchardError::SerializationError(e.to_string()))
    }
}

impl CompactSpend {
    /// Create a new compact spend
    pub fn new(nullifier: [u8; 32]) -> Self {
        Self { nullifier }
    }

    /// Get the nullifier
    pub fn get_nullifier(&self) -> [u8; 32] {
        self.nullifier
    }
}

impl CompactOutput {
    /// Create a new compact output
    pub fn new(commitment: [u8; 32], ephemeral_key: [u8; 32], ciphertext: Vec<u8>) -> Self {
        Self {
            commitment,
            ephemeral_key,
            ciphertext,
        }
    }

    /// Get the commitment
    pub fn get_commitment(&self) -> [u8; 32] {
        self.commitment
    }

    /// Get ephemeral key for decryption
    pub fn get_ephemeral_key(&self) -> [u8; 32] {
        self.ephemeral_key
    }

    /// Get ciphertext for trial decryption
    pub fn get_ciphertext(&self) -> &Vec<u8> {
        &self.ciphertext
    }

    /// Attempt to decrypt the output with a given viewing key
    /// Returns Ok(Some(note_data)) if decryption succeeds, Ok(None) if not our note, Err on failure
    pub fn try_decrypt(&self, viewing_key: &[u8; 32]) -> Result<Option<NoteData>, OrchardError> {
        // Simplified decryption logic - in practice this would use ECIES
        // For now, just check if viewing key matches a simple pattern

        // Derive shared secret from ephemeral key and viewing key
        let shared_secret = poseidon_hash_with_domain(
            COMPACT_TX_SHARED_SECRET_DOMAIN,
            &[*viewing_key, self.ephemeral_key],
        );

        // Try to decrypt first 32 bytes of ciphertext using shared secret
        let mut decrypted = [0u8; 32];
        for i in 0..32 {
            decrypted[i] = self.ciphertext[i] ^ shared_secret[i];
        }

        // Check if decryption looks valid (first 4 bytes should be asset ID)
        let asset_id = u32::from_le_bytes([decrypted[0], decrypted[1], decrypted[2], decrypted[3]]);
        if asset_id == 0 || asset_id > 1000000 { // Simple validity check
            return Ok(None); // Not our note
        }

        // Extract note data (simplified)
        let amount = u64::from_le_bytes([
            decrypted[4], decrypted[5], decrypted[6], decrypted[7],
            decrypted[8], decrypted[9], decrypted[10], decrypted[11],
        ]);

        Ok(Some(NoteData {
            asset_id,
            amount,
            commitment: self.commitment,
        }))
    }
}

impl CompactAction {
    /// Create a new compact action
    pub fn new(
        nullifier: [u8; 32],
        commitment: [u8; 32],
        ephemeral_key: [u8; 32],
        ciphertext: Vec<u8>
    ) -> Self {
        Self {
            nullifier,
            commitment,
            ephemeral_key,
            ciphertext,
        }
    }

    /// Get the nullifier (spend side)
    pub fn get_nullifier(&self) -> [u8; 32] {
        self.nullifier
    }

    /// Get the commitment (output side)
    pub fn get_commitment(&self) -> [u8; 32] {
        self.commitment
    }

    /// Attempt to decrypt the action's output
    pub fn try_decrypt(&self, viewing_key: &[u8; 32]) -> Result<Option<NoteData>, OrchardError> {
        // Reuse the output decryption logic
        let temp_output = CompactOutput {
            commitment: self.commitment,
            ephemeral_key: self.ephemeral_key,
            ciphertext: self.ciphertext.clone(),
        };
        temp_output.try_decrypt(viewing_key)
    }
}

/// Decrypted note data from a compact output
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct NoteData {
    /// Asset ID of the note
    pub asset_id: u32,

    /// Amount in the note
    pub amount: u64,

    /// Note commitment
    pub commitment: [u8; 32],
}

impl TxFilter {
    /// Create filter by hash
    pub fn by_hash(hash: [u8; 32]) -> Self {
        Self {
            block: None,
            index: None,
            hash: Some(hash),
        }
    }

    /// Create filter by block and index
    pub fn by_block_index(height: u64, index: u64) -> Self {
        Self {
            block: Some(BlockID {
                height: Some(height),
                hash: None,
            }),
            index: Some(index),
            hash: None,
        }
    }

    /// Create filter by block hash and index
    pub fn by_block_hash_index(block_hash: [u8; 32], index: u64) -> Self {
        Self {
            block: Some(BlockID {
                height: None,
                hash: Some(block_hash),
            }),
            index: Some(index),
            hash: None,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_compact_tx_creation() {
        let mut tx = CompactTx::new(5, [0xaa; 32], ExtrinsicType::SubmitPrivate);

        assert_eq!(tx.index, 5);
        assert_eq!(tx.txid, [0xaa; 32]);
        assert_eq!(tx.extrinsic_type, ExtrinsicType::SubmitPrivate);
        assert_eq!(tx.spend_count(), 0);
        assert_eq!(tx.output_count(), 0);
        assert!(!tx.is_coinbase());

        // Add spend
        tx.add_spend([0xbb; 32]);
        assert_eq!(tx.spend_count(), 1);
        assert!(tx.contains_nullifier(&[0xbb; 32]));
        assert!(!tx.contains_nullifier(&[0xcc; 32]));

        // Add output
        tx.add_output([0xdd; 32], [0xee; 32], vec![0xff; 52]);
        assert_eq!(tx.output_count(), 1);
        assert!(tx.contains_commitment(&[0xdd; 32]));
        assert!(!tx.contains_commitment(&[0x11; 32]));

        // Add action
        tx.add_action([0x11; 32], [0x22; 32], [0x33; 32], vec![0x44; 52]);
        assert_eq!(tx.spend_count(), 2); // spends + actions
        assert_eq!(tx.output_count(), 2); // outputs + actions
        assert!(tx.contains_nullifier(&[0x11; 32]));
        assert!(tx.contains_commitment(&[0x22; 32]));
    }

    #[test]
    fn test_extrinsic_type_conversion() {
        assert_eq!(ExtrinsicType::from(0), ExtrinsicType::DepositPublic);
        assert_eq!(ExtrinsicType::from(1), ExtrinsicType::SubmitPrivate);
        assert_eq!(ExtrinsicType::from(2), ExtrinsicType::WithdrawPublic);
        assert_eq!(ExtrinsicType::from(3), ExtrinsicType::IssuanceV1);
        assert_eq!(ExtrinsicType::from(4), ExtrinsicType::BatchAggV1);
        assert_eq!(ExtrinsicType::from(999), ExtrinsicType::SubmitPrivate); // fallback
    }

    #[test]
    fn test_compact_output_operations() {
        let output = CompactOutput::new([0x12; 32], [0x34; 32], vec![0x56; 52]);

        assert_eq!(output.get_commitment(), [0x12; 32]);
        assert_eq!(output.get_ephemeral_key(), [0x34; 32]);
        assert_eq!(output.get_ciphertext(), &vec![0x56; 52]);
    }

    #[test]
    fn test_compact_action_operations() {
        let action = CompactAction::new([0xaa; 32], [0xbb; 32], [0xcc; 32], vec![0xdd; 52]);

        assert_eq!(action.get_nullifier(), [0xaa; 32]);
        assert_eq!(action.get_commitment(), [0xbb; 32]);
    }

    #[test]
    fn test_tx_filter_creation() {
        let hash_filter = TxFilter::by_hash([0x42; 32]);
        assert_eq!(hash_filter.hash, Some([0x42; 32]));
        assert!(hash_filter.block.is_none());
        assert!(hash_filter.index.is_none());

        let block_filter = TxFilter::by_block_index(100, 5);
        assert!(block_filter.hash.is_none());
        assert_eq!(block_filter.index, Some(5));
        assert!(block_filter.block.is_some());
        assert_eq!(block_filter.block.as_ref().unwrap().height, Some(100));
    }

    #[test]
    fn test_serialization() {
        let mut tx = CompactTx::new(10, [0x99; 32], ExtrinsicType::BatchAggV1);
        tx.add_spend([0x88; 32]);
        tx.add_output([0x77; 32], [0x66; 32], vec![0x55; 52]);
        tx.set_fee(1000);

        let bytes = tx.to_bytes().expect("Serialization should succeed");
        let recovered = CompactTx::from_bytes(&bytes).expect("Deserialization should succeed");

        assert_eq!(tx, recovered);
    }

    #[test]
    fn test_coinbase_detection() {
        let coinbase_tx = CompactTx::new(0, [0x00; 32], ExtrinsicType::DepositPublic);
        assert!(coinbase_tx.is_coinbase());

        let regular_tx = CompactTx::new(1, [0x11; 32], ExtrinsicType::SubmitPrivate);
        assert!(!regular_tx.is_coinbase());
    }
}
