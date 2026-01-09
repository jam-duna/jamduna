// No_std Orchard bundle parser for signature verification
//
// This module parses Zcash v5 Orchard bundle bytes to extract signatures
// for verification. It does NOT require the full orchard crate.

#![allow(dead_code)]

use crate::errors::{OrchardError, Result};
use alloc::vec::Vec;
use core::convert::TryFrom;

// Bundle size limits for DoS protection
const MAX_ACTIONS_PER_BUNDLE: usize = 100;          // Reasonable upper bound
const MAX_PROOF_SIZE: usize = 1_000_000;            // 1MB max proof size
const MAX_WITNESS_SIZE: usize = 10_000_000;         // 10MB max witness size
const MAX_BUNDLE_SIZE: usize = 50_000_000;          // 50MB max total bundle size

/// Parsed Orchard bundle with extracted signatures
#[derive(Debug)]
pub struct ParsedBundle {
    pub action_count: usize,
    pub actions: Vec<ParsedAction>,
    pub flags: u8,
    pub value_balance: i64,
    pub anchor: [u8; 32],
    pub proof_bytes: Vec<u8>,
    pub spend_auth_sigs: Vec<[u8; 64]>,
    pub binding_signature: [u8; 64],
}

/// Parsed action with signature verification data
#[derive(Debug)]
pub struct ParsedAction {
    pub cv_net: [u8; 32],
    pub nullifier: [u8; 32],
    pub rk: [u8; 32], // Spend authorization verification key
    pub cmx: [u8; 32],
    // Encrypted note data (not needed for signature verification)
    pub epk_bytes: [u8; 32],
    pub enc_ciphertext: [u8; 580],
    pub out_ciphertext: [u8; 80],
}

/// Convert ParsedBundle to CompactBlock for state transitions
///
/// Extracts nullifiers and commitments from bundle actions
pub fn bundle_to_compact_block(bundle: &ParsedBundle, height: u64) -> crate::compactblock::CompactBlock {
    use crate::compactblock::{CompactBlock, CompactTx, OrchardCompactOutput};

    // Extract nullifiers and commitments from actions
    let nullifiers: Vec<[u8; 32]> = bundle.actions.iter()
        .map(|action| action.nullifier)
        .collect();

    let commitments: Vec<OrchardCompactOutput> = bundle.actions.iter()
        .map(|action| OrchardCompactOutput {
            commitment: action.cmx,
            ephemeral_key: action.epk_bytes,
            ciphertext: action.enc_ciphertext.to_vec(),
        })
        .collect();

    // Create single compact transaction
    let tx = CompactTx {
        index: 0,
        txid: [0u8; 32],  // Not needed for verification
        nullifiers,
        commitments,
    };

    CompactBlock {
        proto_version: 1,
        height,
        hash: [0u8; 32],      // Not needed for verification
        prev_hash: [0u8; 32], // Not needed for verification
        time: 0,              // Not needed for verification
        header: Vec::new(),   // Not needed for verification
        vtx: vec![tx],
        chain_metadata: crate::compactblock::ChainMetadata::default(),
    }
}

/// Parse Orchard bundle bytes according to Zcash v5 spec
///
/// Format (per https://zips.z.cash/protocol/protocol.pdf#txnencoding):
/// - nActionsOrchard: CompactSize
/// - vActionsOrchard: Action[nActionsOrchard]
///   - Each Action: cv_net(32) || nf(32) || rk(32) || cmx(32) || ephemeralKey(32) || encCiphertext(580) || outCiphertext(80)
/// - flagsOrchard: byte
/// - valueBalanceOrchard: int64
/// - anchorOrchard: bytes[32]
/// - sizeProofs: CompactSize
/// - proofsOrchard: bytes[sizeProofs]
/// - vSpendAuthSigsOrchard: Signature[nActionsOrchard] (64 bytes each)
/// - bindingSigOrchard: Signature (64 bytes)
pub fn parse_bundle(bytes: &[u8]) -> Result<ParsedBundle> {
    let mut cursor = 0;

    // Validate total bundle size first
    if bytes.len() > MAX_BUNDLE_SIZE {
        return Err(OrchardError::StateError(format!(
            "Bundle size {} exceeds maximum {}",
            bytes.len(), MAX_BUNDLE_SIZE
        )));
    }

    // Read action count
    let (action_count, bytes_read) = read_compactsize(bytes, cursor)?;
    cursor += bytes_read;

    if action_count == 0 {
        return Err(OrchardError::StateError("Bundle has zero actions".into()));
    }

    let action_count = usize::try_from(action_count)
        .map_err(|_| OrchardError::StateError("Action count too large".into()))?;

    // Validate action count against reasonable limits
    if action_count > MAX_ACTIONS_PER_BUNDLE {
        return Err(OrchardError::StateError(format!(
            "Action count {} exceeds maximum {}",
            action_count, MAX_ACTIONS_PER_BUNDLE
        )));
    }

    // Parse actions
    let mut actions = Vec::with_capacity(action_count);
    for _ in 0..action_count {
        let cv_net = read_bytes_32(bytes, cursor)?;
        cursor += 32;

        let nullifier = read_bytes_32(bytes, cursor)?;
        cursor += 32;

        let rk = read_bytes_32(bytes, cursor)?;
        cursor += 32;

        let cmx = read_bytes_32(bytes, cursor)?;
        cursor += 32;

        let epk_bytes = read_bytes_32(bytes, cursor)?;
        cursor += 32;

        let enc_ciphertext = read_bytes_580(bytes, cursor)?;
        cursor += 580;

        let out_ciphertext = read_bytes_80(bytes, cursor)?;
        cursor += 80;

        actions.push(ParsedAction {
            cv_net,
            nullifier,
            rk,
            cmx,
            epk_bytes,
            enc_ciphertext,
            out_ciphertext,
        });
    }

    // Read flags
    let flags = read_byte(bytes, cursor)?;
    cursor += 1;

    // Read value balance (int64 little-endian)
    let value_balance = read_i64_le(bytes, cursor)?;
    cursor += 8;

    // Read anchor
    let anchor = read_bytes_32(bytes, cursor)?;
    cursor += 32;

    // Read proof size and bytes
    let (proof_size, bytes_read) = read_compactsize(bytes, cursor)?;
    cursor += bytes_read;

    let proof_size = usize::try_from(proof_size)
        .map_err(|_| OrchardError::StateError("Proof size too large".into()))?;

    // Validate proof size against DoS limit
    if proof_size > MAX_PROOF_SIZE {
        return Err(OrchardError::StateError(format!(
            "Proof size {} exceeds maximum {}",
            proof_size, MAX_PROOF_SIZE
        )));
    }

    let proof_bytes = read_bytes_vec(bytes, cursor, proof_size)?;
    cursor += proof_size;

    // Read spend authorization signatures (64 bytes each)
    let mut spend_auth_sigs = Vec::with_capacity(action_count);
    for _ in 0..action_count {
        let sig = read_bytes_64(bytes, cursor)?;
        cursor += 64;
        spend_auth_sigs.push(sig);
    }

    // Read binding signature (64 bytes)
    let binding_signature = read_bytes_64(bytes, cursor)?;
    cursor += 64;

    // Verify we consumed all bytes
    if cursor != bytes.len() {
        return Err(OrchardError::StateError(format!(
            "Bundle has {} extra bytes after parsing",
            bytes.len() - cursor
        )));
    }

    Ok(ParsedBundle {
        action_count,
        actions,
        flags,
        value_balance,
        anchor,
        proof_bytes,
        spend_auth_sigs,
        binding_signature,
    })
}

/// Read CompactSize as defined in Zcash protocol
/// Returns (value, bytes_read)
fn read_compactsize(bytes: &[u8], offset: usize) -> Result<(u64, usize)> {
    if offset >= bytes.len() {
        return Err(OrchardError::StateError("Unexpected EOF reading CompactSize".into()));
    }

    let first = bytes[offset];
    match first {
        0..=252 => Ok((first as u64, 1)),
        253 => {
            if offset + 3 > bytes.len() {
                return Err(OrchardError::StateError("Unexpected EOF in CompactSize (uint16)".into()));
            }
            let val = u16::from_le_bytes([bytes[offset + 1], bytes[offset + 2]]);
            Ok((val as u64, 3))
        }
        254 => {
            if offset + 5 > bytes.len() {
                return Err(OrchardError::StateError("Unexpected EOF in CompactSize (uint32)".into()));
            }
            let val = u32::from_le_bytes([
                bytes[offset + 1],
                bytes[offset + 2],
                bytes[offset + 3],
                bytes[offset + 4],
            ]);
            Ok((val as u64, 5))
        }
        255 => {
            if offset + 9 > bytes.len() {
                return Err(OrchardError::StateError("Unexpected EOF in CompactSize (uint64)".into()));
            }
            let val = u64::from_le_bytes([
                bytes[offset + 1],
                bytes[offset + 2],
                bytes[offset + 3],
                bytes[offset + 4],
                bytes[offset + 5],
                bytes[offset + 6],
                bytes[offset + 7],
                bytes[offset + 8],
            ]);
            Ok((val, 9))
        }
    }
}

fn read_byte(bytes: &[u8], offset: usize) -> Result<u8> {
    if offset >= bytes.len() {
        return Err(OrchardError::StateError("Unexpected EOF reading byte".into()));
    }
    Ok(bytes[offset])
}

fn read_bytes_32(bytes: &[u8], offset: usize) -> Result<[u8; 32]> {
    if offset + 32 > bytes.len() {
        return Err(OrchardError::StateError("Unexpected EOF reading 32 bytes".into()));
    }
    let mut result = [0u8; 32];
    result.copy_from_slice(&bytes[offset..offset + 32]);
    Ok(result)
}

fn read_bytes_64(bytes: &[u8], offset: usize) -> Result<[u8; 64]> {
    if offset + 64 > bytes.len() {
        return Err(OrchardError::StateError("Unexpected EOF reading 64 bytes".into()));
    }
    let mut result = [0u8; 64];
    result.copy_from_slice(&bytes[offset..offset + 64]);
    Ok(result)
}

fn read_bytes_80(bytes: &[u8], offset: usize) -> Result<[u8; 80]> {
    if offset + 80 > bytes.len() {
        return Err(OrchardError::StateError("Unexpected EOF reading 80 bytes".into()));
    }
    let mut result = [0u8; 80];
    result.copy_from_slice(&bytes[offset..offset + 80]);
    Ok(result)
}

fn read_bytes_580(bytes: &[u8], offset: usize) -> Result<[u8; 580]> {
    if offset + 580 > bytes.len() {
        return Err(OrchardError::StateError("Unexpected EOF reading 580 bytes".into()));
    }
    let mut result = [0u8; 580];
    result.copy_from_slice(&bytes[offset..offset + 580]);
    Ok(result)
}

fn read_bytes_vec(bytes: &[u8], offset: usize, len: usize) -> Result<Vec<u8>> {
    if offset + len > bytes.len() {
        return Err(OrchardError::StateError(format!(
            "Unexpected EOF reading {} bytes",
            len
        )));
    }
    Ok(bytes[offset..offset + len].to_vec())
}

fn read_i64_le(bytes: &[u8], offset: usize) -> Result<i64> {
    if offset + 8 > bytes.len() {
        return Err(OrchardError::StateError("Unexpected EOF reading i64".into()));
    }
    let val = i64::from_le_bytes([
        bytes[offset],
        bytes[offset + 1],
        bytes[offset + 2],
        bytes[offset + 3],
        bytes[offset + 4],
        bytes[offset + 5],
        bytes[offset + 6],
        bytes[offset + 7],
    ]);
    Ok(val)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_compactsize_single_byte() {
        let bytes = [42u8];
        let (val, size) = read_compactsize(&bytes, 0).unwrap();
        assert_eq!(val, 42);
        assert_eq!(size, 1);
    }

    #[test]
    fn test_compactsize_uint16() {
        let bytes = [253, 0xab, 0xcd]; // 0xcdab in little-endian
        let (val, size) = read_compactsize(&bytes, 0).unwrap();
        assert_eq!(val, 0xcdab);
        assert_eq!(size, 3);
    }

    #[test]
    fn test_parse_bundle_empty_fails() {
        let bytes = [0u8]; // Zero actions
        assert!(parse_bundle(&bytes).is_err());
    }
}
