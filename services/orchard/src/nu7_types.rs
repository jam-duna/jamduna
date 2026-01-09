//! NU7 (ZSA) type definitions for V6 transactions
//!
//! This module extends the Orchard service to support TransactionV6 with:
//! - Multi-asset support (ZSA)
//! - Asset issuance (IssueBundle)
//! - Multi-party swaps (SwapBundle)
//! - Asset burns

#![allow(dead_code)]

use alloc::vec::Vec;
use crate::errors::{OrchardError, Result};

// ============================================================================
// Asset Support
// ============================================================================

/// Asset identifier (32 bytes)
///
/// Uniquely identifies a custom asset in the Orchard pool.
/// Native USDx is represented as [0u8; 32].
#[derive(Clone, Copy, Debug, PartialEq, Eq, PartialOrd, Ord)]
pub struct AssetBase(pub [u8; 32]);

impl AssetBase {
    /// Native USDx asset (zero bytes)
    pub const NATIVE: Self = Self([0u8; 32]);

    pub fn from_bytes(bytes: [u8; 32]) -> Self {
        Self(bytes)
    }

    pub fn to_bytes(&self) -> [u8; 32] {
        self.0
    }

    pub fn is_native(&self) -> bool {
        self.0 == [0u8; 32]
    }
}

/// Burn record - public asset destruction
#[derive(Clone, Debug)]
pub struct BurnRecord {
    pub asset: AssetBase,
    pub amount: u64,
}

// ============================================================================
// OrchardBundle Enum (V6)
// ============================================================================

/// OrchardBundle type discriminator for V6 transactions
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
#[repr(u8)]
pub enum OrchardBundleType {
    /// V5-compatible vanilla Orchard (USDx only)
    Vanilla = 0x00,

    /// V6 single action group with multi-asset support
    ZSA = 0x01,

    /// V6 multi-party swap with multiple action groups
    Swap = 0x02,
}

impl OrchardBundleType {
    pub fn from_u8(value: u8) -> Option<Self> {
        match value {
            0x00 => Some(Self::Vanilla),
            0x01 => Some(Self::ZSA),
            0x02 => Some(Self::Swap),
            _ => None,
        }
    }
}

/// Parsed V6 Orchard bundle (enum wrapper)
#[derive(Debug)]
pub enum ParsedBundleV6 {
    Vanilla(crate::bundle_parser::ParsedBundle),
    ZSA(ParsedZSABundle),
    Swap(ParsedSwapBundle),
}

// ============================================================================
// ZSA Bundle (V6 Single Action Group)
// ============================================================================

/// Parsed ZSA bundle (single action group with assets + burns)
#[derive(Debug)]
pub struct ParsedZSABundle {
    pub action_count: usize,
    pub actions: Vec<ParsedActionV6>,
    pub flags: u8,
    pub value_balance: i64,        // Native USDx balance
    pub anchor: [u8; 32],
    pub expiry_height: u32,         // Reserved, must be 0 for ZSA
    pub burns: Vec<BurnRecord>,     // Asset burns
    pub proof_bytes: Vec<u8>,
    pub spend_auth_sigs: Vec<[u8; 64]>,
    pub binding_signature: [u8; 64],
}

/// Parsed V6 action with asset support
#[derive(Debug)]
pub struct ParsedActionV6 {
    pub cv_net: [u8; 32],
    pub nullifier: [u8; 32],
    pub rk: [u8; 32],
    pub cmx: [u8; 32],
    pub epk_bytes: [u8; 32],
    pub enc_ciphertext: [u8; 580],
    pub out_ciphertext: [u8; 80],
    // Asset is encoded in cv_net value commitment, not explicitly in wire format
}

// ============================================================================
// Swap Bundle (V6 Multi-Party Swaps)
// ============================================================================

/// Parsed SwapBundle (multiple action groups)
#[derive(Debug)]
pub struct ParsedSwapBundle {
    pub action_groups: Vec<ParsedActionGroup>,
    pub value_balance: i64,              // Aggregated USDx balance
    pub binding_signature: [u8; 64],
}

/// Parsed action group (one per swap party)
#[derive(Debug)]
pub struct ParsedActionGroup {
    pub action_count: usize,
    pub actions: Vec<ParsedActionV6>,
    pub flags: u8,
    pub anchor: [u8; 32],
    pub expiry_height: u32,              // Must be 0 for ZSA
    pub burns: Vec<BurnRecord>,
    pub proof_bytes: Vec<u8>,
    pub spend_auth_sigs: Vec<[u8; 64]>,
}

// ============================================================================
// Issue Bundle (Asset Creation)
// ============================================================================

/// Parsed IssueBundle (asset issuance)
#[derive(Debug)]
pub struct ParsedIssueBundle {
    pub issuer_key: [u8; 32],            // IssueValidatingKey
    pub actions: Vec<ParsedIssueAction>,
    pub authorization: IssueAuthorization,
}

/// Parsed issue action (create one asset)
#[derive(Debug)]
pub struct ParsedIssueAction {
    pub asset_desc_hash: [u8; 32],       // BLAKE2b hash of asset description
    pub notes: Vec<ParsedIssuedNote>,
    pub finalize: bool,                   // If true, no more issuance allowed
}

/// Parsed issued note
#[derive(Debug)]
pub struct ParsedIssuedNote {
    pub recipient: [u8; 43],              // Orchard raw address
    pub value: u64,
    pub rho: [u8; 32],
    pub rseed: [u8; 32],
}

/// Issue authorization (signature over issuance)
#[derive(Debug)]
pub struct IssueAuthorization {
    pub sighash_info: Vec<u8>,
    pub signature: Vec<u8>,
}

// ============================================================================
// Extrinsic Payload
// ============================================================================

/// JAM Orchard extrinsic for NU7 support
///
/// This is the payload submitted to the JAM Orchard service for verification.
#[derive(Debug)]
pub enum OrchardExtrinsic {
    /// Legacy V5 bundle proof
    BundleProof {
        bundle_bytes: Vec<u8>,
        witness_bytes: Option<Vec<u8>>,
    },

    /// V6 bundle proof with optional IssueBundle
    BundleProofV6 {
        bundle_type: OrchardBundleType,
        orchard_bundle_bytes: Vec<u8>,
        issue_bundle_bytes: Option<Vec<u8>>,
        witness_bytes: Option<Vec<u8>>,
    },
}

impl OrchardExtrinsic {
    /// Serialize extrinsic to bytes
    pub fn to_bytes(&self) -> Vec<u8> {
        let mut bytes = Vec::new();

        match self {
            OrchardExtrinsic::BundleProof {
                bundle_bytes,
                witness_bytes,
            } => {
                // Type discriminator
                bytes.push(0x00);

                // Bundle bytes (length-prefixed)
                let len = bundle_bytes.len() as u32;
                bytes.extend_from_slice(&len.to_le_bytes());
                bytes.extend_from_slice(bundle_bytes);

                // Witness bytes (optional, length-prefixed)
                if let Some(witness) = witness_bytes {
                    bytes.push(0x01); // Present
                    let len = witness.len() as u32;
                    bytes.extend_from_slice(&len.to_le_bytes());
                    bytes.extend_from_slice(witness);
                } else {
                    bytes.push(0x00); // Not present
                }
            }

            OrchardExtrinsic::BundleProofV6 {
                bundle_type,
                orchard_bundle_bytes,
                issue_bundle_bytes,
                witness_bytes,
            } => {
                // Type discriminator
                bytes.push(0x01);

                // Bundle type
                bytes.push(*bundle_type as u8);

                // Orchard bundle bytes (length-prefixed)
                let len = orchard_bundle_bytes.len() as u32;
                bytes.extend_from_slice(&len.to_le_bytes());
                bytes.extend_from_slice(orchard_bundle_bytes);

                // Issue bundle bytes (optional, length-prefixed)
                if let Some(issue_bundle) = issue_bundle_bytes {
                    bytes.push(0x01); // Present
                    let len = issue_bundle.len() as u32;
                    bytes.extend_from_slice(&len.to_le_bytes());
                    bytes.extend_from_slice(issue_bundle);
                } else {
                    bytes.push(0x00); // Not present
                }

                // Witness bytes (optional, length-prefixed)
                if let Some(witness) = witness_bytes {
                    bytes.push(0x01); // Present
                    let len = witness.len() as u32;
                    bytes.extend_from_slice(&len.to_le_bytes());
                    bytes.extend_from_slice(witness);
                } else {
                    bytes.push(0x00); // Not present
                }
            }
        }

        bytes
    }

    /// Deserialize extrinsic from bytes
    pub fn from_bytes(bytes: &[u8]) -> Result<Self> {
        let mut cursor = 0;

        // Read type discriminator
        if bytes.is_empty() {
            return Err(OrchardError::StateError("Empty extrinsic bytes".into()));
        }

        let extrinsic_type = bytes[cursor];
        cursor += 1;

        match extrinsic_type {
            0x00 => {
                // Legacy V5 BundleProof
                if cursor + 4 > bytes.len() {
                    return Err(OrchardError::StateError("Truncated bundle length".into()));
                }

                let bundle_len = u32::from_le_bytes([
                    bytes[cursor],
                    bytes[cursor + 1],
                    bytes[cursor + 2],
                    bytes[cursor + 3],
                ]) as usize;
                cursor += 4;

                if cursor + bundle_len > bytes.len() {
                    return Err(OrchardError::StateError("Truncated bundle bytes".into()));
                }

                let bundle_bytes = bytes[cursor..cursor + bundle_len].to_vec();
                cursor += bundle_len;

                // Read witness presence flag
                if cursor >= bytes.len() {
                    return Err(OrchardError::StateError("Missing witness flag".into()));
                }

                let witness_bytes = if bytes[cursor] == 0x01 {
                    cursor += 1;
                    if cursor + 4 > bytes.len() {
                        return Err(OrchardError::StateError("Truncated witness length".into()));
                    }

                    let witness_len = u32::from_le_bytes([
                        bytes[cursor],
                        bytes[cursor + 1],
                        bytes[cursor + 2],
                        bytes[cursor + 3],
                    ]) as usize;
                    cursor += 4;

                    if cursor + witness_len > bytes.len() {
                        return Err(OrchardError::StateError("Truncated witness bytes".into()));
                    }

                    let witness = bytes[cursor..cursor + witness_len].to_vec();
                    cursor += witness_len;
                    Some(witness)
                } else {
                    // Presence flag is 0x00 (None) - must consume it
                    cursor += 1;
                    None
                };

                // Validate no trailing bytes
                if cursor != bytes.len() {
                    return Err(OrchardError::StateError(format!(
                        "BundleProof has trailing bytes: {} unconsumed of {}",
                        bytes.len() - cursor,
                        bytes.len()
                    )));
                }

                Ok(OrchardExtrinsic::BundleProof {
                    bundle_bytes,
                    witness_bytes,
                })
            }

            0x01 => {
                // V6 BundleProofV6
                if cursor >= bytes.len() {
                    return Err(OrchardError::StateError("Missing bundle type".into()));
                }

                let bundle_type = OrchardBundleType::from_u8(bytes[cursor])
                    .ok_or_else(|| OrchardError::StateError("Invalid bundle type".into()))?;
                cursor += 1;

                // Read orchard bundle
                if cursor + 4 > bytes.len() {
                    return Err(OrchardError::StateError("Truncated orchard bundle length".into()));
                }

                let orchard_len = u32::from_le_bytes([
                    bytes[cursor],
                    bytes[cursor + 1],
                    bytes[cursor + 2],
                    bytes[cursor + 3],
                ]) as usize;
                cursor += 4;

                if cursor + orchard_len > bytes.len() {
                    return Err(OrchardError::StateError("Truncated orchard bundle bytes".into()));
                }

                let orchard_bundle_bytes = bytes[cursor..cursor + orchard_len].to_vec();
                cursor += orchard_len;

                // Read issue bundle (optional)
                if cursor >= bytes.len() {
                    return Err(OrchardError::StateError("Missing issue bundle flag".into()));
                }

                let issue_bundle_bytes = if bytes[cursor] == 0x01 {
                    cursor += 1;
                    if cursor + 4 > bytes.len() {
                        return Err(OrchardError::StateError("Truncated issue bundle length".into()));
                    }

                    let issue_len = u32::from_le_bytes([
                        bytes[cursor],
                        bytes[cursor + 1],
                        bytes[cursor + 2],
                        bytes[cursor + 3],
                    ]) as usize;
                    cursor += 4;

                    if cursor + issue_len > bytes.len() {
                        return Err(OrchardError::StateError("Truncated issue bundle bytes".into()));
                    }

                    let issue = bytes[cursor..cursor + issue_len].to_vec();
                    cursor += issue_len;
                    Some(issue)
                } else {
                    // Presence flag is 0x00 (None) - must consume it
                    cursor += 1;
                    None
                };

                // Read witness (optional)
                if cursor >= bytes.len() {
                    return Err(OrchardError::StateError("Missing witness flag".into()));
                }

                let witness_bytes = if bytes[cursor] == 0x01 {
                    cursor += 1;
                    if cursor + 4 > bytes.len() {
                        return Err(OrchardError::StateError("Truncated witness length".into()));
                    }

                    let witness_len = u32::from_le_bytes([
                        bytes[cursor],
                        bytes[cursor + 1],
                        bytes[cursor + 2],
                        bytes[cursor + 3],
                    ]) as usize;
                    cursor += 4;

                    if cursor + witness_len > bytes.len() {
                        return Err(OrchardError::StateError("Truncated witness bytes".into()));
                    }

                    let witness = bytes[cursor..cursor + witness_len].to_vec();
                    cursor += witness_len;
                    Some(witness)
                } else {
                    // Presence flag is 0x00 (None) - must consume it
                    cursor += 1;
                    None
                };

                // Validate no trailing bytes
                if cursor != bytes.len() {
                    return Err(OrchardError::StateError(format!(
                        "BundleProofV6 has trailing bytes: {} unconsumed of {}",
                        bytes.len() - cursor,
                        bytes.len()
                    )));
                }

                Ok(OrchardExtrinsic::BundleProofV6 {
                    bundle_type,
                    orchard_bundle_bytes,
                    issue_bundle_bytes,
                    witness_bytes,
                })
            }

            _ => Err(OrchardError::StateError(format!("Unknown extrinsic type: {}", extrinsic_type))),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_asset_base_native() {
        let native = AssetBase::NATIVE;
        assert!(native.is_native());
        assert_eq!(native.to_bytes(), [0u8; 32]);
    }

    #[test]
    fn test_asset_base_custom() {
        let custom = AssetBase::from_bytes([1u8; 32]);
        assert!(!custom.is_native());
        assert_eq!(custom.to_bytes(), [1u8; 32]);
    }

    #[test]
    fn test_extrinsic_v5_roundtrip() {
        let original = OrchardExtrinsic::BundleProof {
            bundle_bytes: vec![1, 2, 3, 4],
            witness_bytes: Some(vec![5, 6, 7, 8]),
        };

        let serialized = original.to_bytes();
        let deserialized = OrchardExtrinsic::from_bytes(&serialized).unwrap();

        match deserialized {
            OrchardExtrinsic::BundleProof { bundle_bytes, witness_bytes } => {
                assert_eq!(bundle_bytes, vec![1, 2, 3, 4]);
                assert_eq!(witness_bytes, Some(vec![5, 6, 7, 8]));
            }
            _ => panic!("Wrong variant"),
        }
    }

    #[test]
    fn test_extrinsic_v6_roundtrip() {
        let original = OrchardExtrinsic::BundleProofV6 {
            bundle_type: OrchardBundleType::Swap,
            orchard_bundle_bytes: vec![1, 2, 3],
            issue_bundle_bytes: Some(vec![4, 5]),
            witness_bytes: None,
        };

        let serialized = original.to_bytes();
        let deserialized = OrchardExtrinsic::from_bytes(&serialized).unwrap();

        match deserialized {
            OrchardExtrinsic::BundleProofV6 {
                bundle_type,
                orchard_bundle_bytes,
                issue_bundle_bytes,
                witness_bytes,
            } => {
                assert_eq!(bundle_type, OrchardBundleType::Swap);
                assert_eq!(orchard_bundle_bytes, vec![1, 2, 3]);
                assert_eq!(issue_bundle_bytes, Some(vec![4, 5]));
                assert_eq!(witness_bytes, None);
            }
            _ => panic!("Wrong variant"),
        }
    }
}
