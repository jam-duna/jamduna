#![allow(dead_code)]
//! Zcash v5/v6 Orchard bundle decoding for the Orchard service.
//!
//! This module parses the Orchard bundle bytes embedded in JAM extrinsics and
//! exposes the minimal fields needed by the refiner.
//!
//! Supports:
//! - V5 Vanilla bundles (single action group, native USDx only)
//! - V6 ZSA bundles (single action group with multi-asset support)
//! - V6 SwapBundle (multiple action groups for atomic swaps)
//! - V6 IssueBundle (asset issuance)

use alloc::vec::Vec;
use crate::errors::{OrchardError, Result};
use crate::nu7_types::{BurnRecord, AssetBase};

#[derive(Debug, Clone)]
pub struct DecodedAction {
    pub cv_net: [u8; 32],
    pub nullifier: [u8; 32],
    pub rk: [u8; 32],
    pub cmx: [u8; 32],
    pub epk_bytes: [u8; 32],
    pub enc_ciphertext: [u8; 580],
    pub out_ciphertext: [u8; 80],
}

/// ZSA encrypted note ciphertext size (compact note + memo + AEAD tag).
pub const ZSA_ENC_CIPHERTEXT_SIZE: usize = 612;

#[derive(Debug, Clone)]
pub struct DecodedActionV6 {
    pub cv_net: [u8; 32],
    pub nullifier: [u8; 32],
    pub rk: [u8; 32],
    pub cmx: [u8; 32],
    pub epk_bytes: [u8; 32],
    pub enc_ciphertext: [u8; ZSA_ENC_CIPHERTEXT_SIZE],
    pub out_ciphertext: [u8; 80],
}

#[derive(Debug, Clone)]
pub struct DecodedBundle {
    pub actions: Vec<DecodedAction>,
    pub flags: u8,
    pub value_balance: i64,
    pub anchor: [u8; 32],
    pub proof: Vec<u8>,
    pub spend_auth_sigs: Vec<[u8; 64]>,
    pub binding_sig: [u8; 64],
}

pub fn decode_bundle(bytes: &[u8]) -> Result<DecodedBundle> {
    let mut cursor = 0usize;
    let action_count = read_compactsize(bytes, &mut cursor)?;
    if action_count == 0 {
        return Err(OrchardError::ParseError("Orchard bundle has no actions".into()));
    }
    let action_count = usize::try_from(action_count)
        .map_err(|_| OrchardError::ParseError("action count overflow".into()))?;

    let mut actions = Vec::with_capacity(action_count);
    for _ in 0..action_count {
        let cv_net = read_array::<32>(bytes, &mut cursor)?;
        let nullifier = read_array::<32>(bytes, &mut cursor)?;
        let rk = read_array::<32>(bytes, &mut cursor)?;
        let cmx = read_array::<32>(bytes, &mut cursor)?;
        let epk_bytes = read_array::<32>(bytes, &mut cursor)?;
        let enc_ciphertext = read_array::<580>(bytes, &mut cursor)?;
        let out_ciphertext = read_array::<80>(bytes, &mut cursor)?;

        actions.push(DecodedAction {
            cv_net,
            nullifier,
            rk,
            cmx,
            epk_bytes,
            enc_ciphertext,
            out_ciphertext,
        });
    }

    let flags = read_u8(bytes, &mut cursor)?;
    let value_balance = read_i64(bytes, &mut cursor)?;
    let anchor = read_array::<32>(bytes, &mut cursor)?;

    let proof_len = read_compactsize(bytes, &mut cursor)?;
    let proof_len = usize::try_from(proof_len)
        .map_err(|_| OrchardError::ParseError("proof length overflow".into()))?;
    let proof = read_vec(bytes, &mut cursor, proof_len)?;

    let mut spend_auth_sigs = Vec::with_capacity(action_count);
    for _ in 0..action_count {
        spend_auth_sigs.push(read_array::<64>(bytes, &mut cursor)?);
    }
    let binding_sig = read_array::<64>(bytes, &mut cursor)?;

    if cursor != bytes.len() {
        return Err(OrchardError::ParseError("trailing bytes in bundle".into()));
    }

    Ok(DecodedBundle {
        actions,
        flags,
        value_balance,
        anchor,
        proof,
        spend_auth_sigs,
        binding_sig,
    })
}

fn read_compactsize(bytes: &[u8], cursor: &mut usize) -> Result<u64> {
    let tag = read_u8(bytes, cursor)?;
    match tag {
        0x00..=0xfc => Ok(tag as u64),
        0xfd => {
            let value = read_u16(bytes, cursor)?;
            if value < 0xfd {
                return Err(OrchardError::ParseError("non-canonical CompactSize".into()));
            }
            Ok(value as u64)
        }
        0xfe => {
            let value = read_u32(bytes, cursor)?;
            if value <= 0xffff {
                return Err(OrchardError::ParseError("non-canonical CompactSize".into()));
            }
            Ok(value as u64)
        }
        0xff => {
            let value = read_u64(bytes, cursor)?;
            if value <= 0xffff_ffff {
                return Err(OrchardError::ParseError("non-canonical CompactSize".into()));
            }
            Ok(value)
        }
    }
}

fn read_u8(bytes: &[u8], cursor: &mut usize) -> Result<u8> {
    let value = *bytes.get(*cursor)
        .ok_or_else(|| OrchardError::ParseError("unexpected end of input".into()))?;
    *cursor += 1;
    Ok(value)
}

fn read_u16(bytes: &[u8], cursor: &mut usize) -> Result<u16> {
    let value = read_array::<2>(bytes, cursor)?;
    Ok(u16::from_le_bytes(value))
}

fn read_u32(bytes: &[u8], cursor: &mut usize) -> Result<u32> {
    let value = read_array::<4>(bytes, cursor)?;
    Ok(u32::from_le_bytes(value))
}

fn read_u64(bytes: &[u8], cursor: &mut usize) -> Result<u64> {
    let value = read_array::<8>(bytes, cursor)?;
    Ok(u64::from_le_bytes(value))
}

fn read_i64(bytes: &[u8], cursor: &mut usize) -> Result<i64> {
    let value = read_array::<8>(bytes, cursor)?;
    Ok(i64::from_le_bytes(value))
}

fn read_array<const N: usize>(bytes: &[u8], cursor: &mut usize) -> Result<[u8; N]> {
    let end = *cursor + N;
    let slice = bytes
        .get(*cursor..end)
        .ok_or_else(|| OrchardError::ParseError("unexpected end of input".into()))?;
    *cursor = end;
    let mut out = [0u8; N];
    out.copy_from_slice(slice);
    Ok(out)
}

fn read_vec(bytes: &[u8], cursor: &mut usize, len: usize) -> Result<Vec<u8>> {
    let end = *cursor + len;
    let slice = bytes
        .get(*cursor..end)
        .ok_or_else(|| OrchardError::ParseError("unexpected end of input".into()))?;
    *cursor = end;
    Ok(slice.to_vec())
}

const ORCHARD_SIGHASH_INFO_V0: [u8; 1] = [0u8];

fn read_versioned_signature(bytes: &[u8], cursor: &mut usize) -> Result<[u8; 64]> {
    let info_len = read_compactsize(bytes, cursor)?;
    let info_len = usize::try_from(info_len)
        .map_err(|_| OrchardError::ParseError("sighash_info length overflow".into()))?;
    let info = read_vec(bytes, cursor, info_len)?;
    if info.as_slice() != ORCHARD_SIGHASH_INFO_V0 {
        return Err(OrchardError::ParseError("unsupported Orchard sighash_info".into()));
    }
    read_array::<64>(bytes, cursor)
}

// ============================================================================
// V6 ZSA Bundle (Single Action Group with Assets)
// ============================================================================

#[derive(Debug, Clone)]
pub struct DecodedZSABundle {
    pub actions: Vec<DecodedActionV6>,
    pub flags: u8,
    pub value_balance: i64,
    pub anchor: [u8; 32],
    pub expiry_height: u32,
    pub burns: Vec<BurnRecord>,
    pub proof: Vec<u8>,
    pub spend_auth_sigs: Vec<[u8; 64]>,
    pub binding_sig: [u8; 64],
}

/// Decode ZSA bundle (V6 single action group with asset support)
///
/// Wire format per librustzcash (consensus-critical):
/// - CompactSize: action_count (NonEmpty)
/// - actions: action_count * Action
/// - flags: u8
/// - anchor: [u8; 32]
/// - expiry_height: u32 LE (must be 0 for ZSA)
/// - CompactSize + burns
/// - CompactSize + proof
/// - spend_auth_sigs: action_count * VersionedSignature
/// - value_balance: i64 LE  (CRITICAL: bundle-level USDx balance)
/// - binding_sig: VersionedSignature
pub fn decode_zsa_bundle(bytes: &[u8]) -> Result<DecodedZSABundle> {
    let mut cursor = 0usize;
    let action_count = read_compactsize(bytes, &mut cursor)?;
    if action_count == 0 {
        return Err(OrchardError::ParseError("ZSA bundle has no actions".into()));
    }
    let action_count = usize::try_from(action_count)
        .map_err(|_| OrchardError::ParseError("action count overflow".into()))?;

    let mut actions = Vec::with_capacity(action_count);
    for _ in 0..action_count {
        let cv_net = read_array::<32>(bytes, &mut cursor)?;
        let nullifier = read_array::<32>(bytes, &mut cursor)?;
        let rk = read_array::<32>(bytes, &mut cursor)?;
        let cmx = read_array::<32>(bytes, &mut cursor)?;
        let epk_bytes = read_array::<32>(bytes, &mut cursor)?;
        let enc_ciphertext = read_array::<ZSA_ENC_CIPHERTEXT_SIZE>(bytes, &mut cursor)?;
        let out_ciphertext = read_array::<80>(bytes, &mut cursor)?;

        actions.push(DecodedActionV6 {
            cv_net,
            nullifier,
            rk,
            cmx,
            epk_bytes,
            enc_ciphertext,
            out_ciphertext,
        });
    }

    let flags = read_u8(bytes, &mut cursor)?;
    let anchor = read_array::<32>(bytes, &mut cursor)?;
    let expiry_height = read_u32(bytes, &mut cursor)?;

    // Enforce NU7 consensus rule: expiry_height MUST be 0
    if expiry_height != 0 {
        return Err(OrchardError::ParseError(format!(
            "ZSA bundle expiry_height must be 0, got {}",
            expiry_height
        )));
    }

    // Parse burns
    let burn_count = read_compactsize(bytes, &mut cursor)?;
    let burn_count = usize::try_from(burn_count)
        .map_err(|_| OrchardError::ParseError("burn count overflow".into()))?;

    let mut burns = Vec::with_capacity(burn_count);
    for _ in 0..burn_count {
        let asset_bytes = read_array::<32>(bytes, &mut cursor)?;
        let amount = read_u64(bytes, &mut cursor)?;
        burns.push(BurnRecord {
            asset: AssetBase::from_bytes(asset_bytes),
            amount,
        });
    }

    let proof_len = read_compactsize(bytes, &mut cursor)?;
    let proof_len = usize::try_from(proof_len)
        .map_err(|_| OrchardError::ParseError("proof length overflow".into()))?;
    let proof = read_vec(bytes, &mut cursor, proof_len)?;

    let mut spend_auth_sigs = Vec::with_capacity(action_count);
    for _ in 0..action_count {
        spend_auth_sigs.push(read_versioned_signature(bytes, &mut cursor)?);
    }

    // CRITICAL: value_balance appears BEFORE binding_sig in ZSA bundles
    // Per librustzcash write_bundle_balance_metadata:
    //   writer.write_all(&value_balance.to_i64_le_bytes())?;
    //   binding_signature.write(&mut writer)?;
    let value_balance = read_i64(bytes, &mut cursor)?;
    let binding_sig = read_versioned_signature(bytes, &mut cursor)?;

    if cursor != bytes.len() {
        return Err(OrchardError::ParseError("trailing bytes in ZSA bundle".into()));
    }

    Ok(DecodedZSABundle {
        actions,
        flags,
        value_balance,
        anchor,
        expiry_height,
        burns,
        proof,
        spend_auth_sigs,
        binding_sig,
    })
}

// ============================================================================
// V6 SwapBundle (Multiple Action Groups)
// ============================================================================

#[derive(Debug, Clone)]
pub struct DecodedActionGroup {
    pub actions: Vec<DecodedActionV6>,
    pub flags: u8,
    pub anchor: [u8; 32],
    pub expiry_height: u32,
    pub burns: Vec<BurnRecord>,
    pub proof: Vec<u8>,
    pub spend_auth_sigs: Vec<[u8; 64]>,
}

#[derive(Debug, Clone)]
pub struct DecodedSwapBundle {
    pub action_groups: Vec<DecodedActionGroup>,
    pub value_balance: i64,
    pub binding_sig: [u8; 64],
}

/// Decode SwapBundle (V6 multi-party swap with multiple action groups)
///
/// Wire format:
/// - CompactSize: num_action_groups (NonEmpty)
/// - For each action group:
///   - CompactSize: action_count (NonEmpty)
///   - actions: action_count * Action
///   - flags: u8
///   - anchor: [u8; 32]
///   - expiry_height: u32 LE (must be 0 for ZSA)
///   - CompactSize + burns
///   - CompactSize + proof
///   - spend_auth_sigs: action_count * VersionedSignature
/// - value_balance: i64 LE (aggregated USDx balance)
/// - binding_sig: VersionedSignature (aggregated binding signature)
pub fn decode_swap_bundle(bytes: &[u8]) -> Result<DecodedSwapBundle> {
    let mut cursor = 0usize;

    let num_groups = read_compactsize(bytes, &mut cursor)?;
    if num_groups == 0 {
        return Err(OrchardError::ParseError("SwapBundle has no action groups".into()));
    }
    let num_groups = usize::try_from(num_groups)
        .map_err(|_| OrchardError::ParseError("action group count overflow".into()))?;

    let mut action_groups = Vec::with_capacity(num_groups);

    for _ in 0..num_groups {
        let action_count = read_compactsize(bytes, &mut cursor)?;
        if action_count == 0 {
            return Err(OrchardError::ParseError("action group has no actions".into()));
        }
        let action_count = usize::try_from(action_count)
            .map_err(|_| OrchardError::ParseError("action count overflow".into()))?;

        let mut actions = Vec::with_capacity(action_count);
        for _ in 0..action_count {
            let cv_net = read_array::<32>(bytes, &mut cursor)?;
            let nullifier = read_array::<32>(bytes, &mut cursor)?;
            let rk = read_array::<32>(bytes, &mut cursor)?;
            let cmx = read_array::<32>(bytes, &mut cursor)?;
            let epk_bytes = read_array::<32>(bytes, &mut cursor)?;
            let enc_ciphertext = read_array::<ZSA_ENC_CIPHERTEXT_SIZE>(bytes, &mut cursor)?;
            let out_ciphertext = read_array::<80>(bytes, &mut cursor)?;

            actions.push(DecodedActionV6 {
                cv_net,
                nullifier,
                rk,
                cmx,
                epk_bytes,
                enc_ciphertext,
                out_ciphertext,
            });
        }

        let flags = read_u8(bytes, &mut cursor)?;
        let anchor = read_array::<32>(bytes, &mut cursor)?;
        let expiry_height = read_u32(bytes, &mut cursor)?;

        // Enforce NU7 consensus rule: expiry_height MUST be 0
        if expiry_height != 0 {
            return Err(OrchardError::ParseError(format!(
                "Swap action group expiry_height must be 0, got {}",
                expiry_height
            )));
        }

        // Parse burns for this action group
        let burn_count = read_compactsize(bytes, &mut cursor)?;
        let burn_count = usize::try_from(burn_count)
            .map_err(|_| OrchardError::ParseError("burn count overflow".into()))?;

        let mut burns = Vec::with_capacity(burn_count);
        for _ in 0..burn_count {
            let asset_bytes = read_array::<32>(bytes, &mut cursor)?;
            let amount = read_u64(bytes, &mut cursor)?;
            burns.push(BurnRecord {
                asset: AssetBase::from_bytes(asset_bytes),
                amount,
            });
        }

        let proof_len = read_compactsize(bytes, &mut cursor)?;
        let proof_len = usize::try_from(proof_len)
            .map_err(|_| OrchardError::ParseError("proof length overflow".into()))?;
        let proof = read_vec(bytes, &mut cursor, proof_len)?;

        let mut spend_auth_sigs = Vec::with_capacity(action_count);
        for _ in 0..action_count {
            spend_auth_sigs.push(read_versioned_signature(bytes, &mut cursor)?);
        }

        action_groups.push(DecodedActionGroup {
            actions,
            flags,
            anchor,
            expiry_height,
            burns,
            proof,
            spend_auth_sigs,
        });
    }

    let value_balance = read_i64(bytes, &mut cursor)?;
    let binding_sig = read_versioned_signature(bytes, &mut cursor)?;

    if cursor != bytes.len() {
        return Err(OrchardError::ParseError("trailing bytes in SwapBundle".into()));
    }

    Ok(DecodedSwapBundle {
        action_groups,
        value_balance,
        binding_sig,
    })
}

// ============================================================================
// V6 IssueBundle (Asset Issuance)
// ============================================================================

#[derive(Debug, Clone)]
pub struct DecodedIssuedNote {
    pub recipient: [u8; 43],
    pub value: u64,
    pub rho: [u8; 32],
    pub rseed: [u8; 32],
}

#[derive(Debug, Clone)]
pub struct DecodedIssueAction {
    pub asset_desc_hash: [u8; 32],
    pub notes: Vec<DecodedIssuedNote>,
    pub finalize: bool,
}

#[derive(Debug, Clone)]
pub struct DecodedIssueBundle {
    pub issuer_key: Vec<u8>,
    pub actions: Vec<DecodedIssueAction>,
    pub sighash_info: Vec<u8>,
    pub signature: Vec<u8>,
}

/// Decode IssueBundle (V6 asset issuance)
///
/// Wire format:
/// - CompactSize: issuer_key_len
/// - issuer_key: issuer_key_len bytes (IssueValidatingKey)
/// - CompactSize: action_count (NonEmpty)
/// - For each action:
///   - asset_desc_hash: [u8; 32]
///   - CompactSize: note_count (NonEmpty)
///   - For each note:
///     - recipient: [u8; 43] (Orchard raw address)
///     - value: u64 LE
///     - rho: [u8; 32]
///     - rseed: [u8; 32]
///   - finalize: u8 (bool)
/// - CompactSize: sighash_info_len
/// - sighash_info: sighash_info_len bytes
/// - CompactSize: signature_len
/// - signature: signature_len bytes
///
/// None encoding: issuer_key_len=0 && action_count=0
pub fn decode_issue_bundle(bytes: &[u8]) -> Result<Option<DecodedIssueBundle>> {
    let mut cursor = 0usize;

    // Check for None encoding
    if bytes.len() < 2 {
        return Err(OrchardError::ParseError("IssueBundle too short".into()));
    }

    let issuer_len = read_compactsize(bytes, &mut cursor)?;
    if issuer_len == 0 {
        // Check if action_count is also 0 (None encoding)
        let action_count = read_compactsize(bytes, &mut cursor)?;
        if action_count == 0 {
            if cursor != bytes.len() {
                return Err(OrchardError::ParseError(
                    "trailing bytes in IssueBundle None encoding".into(),
                ));
            }
            return Ok(None);
        } else {
            return Err(OrchardError::ParseError("Invalid IssueBundle: issuer_len=0 but action_count>0".into()));
        }
    }

    let issuer_len = usize::try_from(issuer_len)
        .map_err(|_| OrchardError::ParseError("issuer key length overflow".into()))?;
    let issuer_key = read_vec(bytes, &mut cursor, issuer_len)?;

    let action_count = read_compactsize(bytes, &mut cursor)?;
    if action_count == 0 {
        return Err(OrchardError::ParseError("IssueBundle has no actions".into()));
    }
    let action_count = usize::try_from(action_count)
        .map_err(|_| OrchardError::ParseError("action count overflow".into()))?;

    let mut actions = Vec::with_capacity(action_count);
    for _ in 0..action_count {
        let asset_desc_hash = read_array::<32>(bytes, &mut cursor)?;

        let note_count = read_compactsize(bytes, &mut cursor)?;
        if note_count == 0 {
            return Err(OrchardError::ParseError("IssueAction has no notes".into()));
        }
        let note_count = usize::try_from(note_count)
            .map_err(|_| OrchardError::ParseError("note count overflow".into()))?;

        let mut notes = Vec::with_capacity(note_count);
        for _ in 0..note_count {
            let recipient = read_array::<43>(bytes, &mut cursor)?;
            let value = read_u64(bytes, &mut cursor)?;
            let rho = read_array::<32>(bytes, &mut cursor)?;
            let rseed = read_array::<32>(bytes, &mut cursor)?;

            notes.push(DecodedIssuedNote {
                recipient,
                value,
                rho,
                rseed,
            });
        }

        let finalize = read_u8(bytes, &mut cursor)? != 0;

        actions.push(DecodedIssueAction {
            asset_desc_hash,
            notes,
            finalize,
        });
    }

    let sighash_len = read_compactsize(bytes, &mut cursor)?;
    let sighash_len = usize::try_from(sighash_len)
        .map_err(|_| OrchardError::ParseError("sighash_info length overflow".into()))?;
    let sighash_info = read_vec(bytes, &mut cursor, sighash_len)?;

    let sig_len = read_compactsize(bytes, &mut cursor)?;
    let sig_len = usize::try_from(sig_len)
        .map_err(|_| OrchardError::ParseError("signature length overflow".into()))?;
    let signature = read_vec(bytes, &mut cursor, sig_len)?;

    if cursor != bytes.len() {
        return Err(OrchardError::ParseError("trailing bytes in IssueBundle".into()));
    }

    Ok(Some(DecodedIssueBundle {
        issuer_key,
        actions,
        sighash_info,
        signature,
    }))
}

#[cfg(all(feature = "orchard", feature = "std"))]
pub fn to_orchard_bundle(
    decoded: &DecodedBundle,
) -> Result<orchard::Bundle<orchard::bundle::Authorized, i64, orchard::orchard_flavor::OrchardVanilla>>
{
    use orchard::{
        Action, Anchor, Proof,
        bundle::{Authorized, Flags},
        note::{ExtractedNoteCommitment, Nullifier, TransmittedNoteCiphertext},
        orchard_flavor::OrchardVanilla,
        orchard_sighash_versioning::{OrchardSighashVersion, VerBindingSig, VerSpendAuthSig},
        primitives::redpallas::{self, Binding, SpendAuth},
        value::ValueCommitment,
    };
    use nonempty::NonEmpty;
    use zcash_note_encryption::note_bytes::NoteBytesData;

    if decoded.actions.is_empty() {
        return Err(OrchardError::ParseError("bundle has no actions".into()));
    }

    let flags = Flags::from_byte(decoded.flags)
        .ok_or_else(|| OrchardError::ParseError("invalid Orchard flags".into()))?;
    let anchor = ct_to_result(Anchor::from_bytes(decoded.anchor), "invalid anchor")?;
    let proof = Proof::new(decoded.proof.clone());
    let binding_sig = redpallas::Signature::<Binding>::from(decoded.binding_sig);
    let authorization = Authorized::from_parts(
        proof,
        VerBindingSig::new(OrchardSighashVersion::NoVersion, binding_sig),
    );

    let mut actions = Vec::with_capacity(decoded.actions.len());
    for (idx, action) in decoded.actions.iter().enumerate() {
        let cv_net = ct_to_result(
            ValueCommitment::from_bytes(&action.cv_net),
            "invalid value commitment",
        )?;
        let nullifier = ct_to_result(
            Nullifier::from_bytes(&action.nullifier),
            "invalid nullifier",
        )?;
        let rk = redpallas::VerificationKey::<SpendAuth>::try_from(action.rk)
            .map_err(|_| OrchardError::ParseError("invalid spend auth key".into()))?;
        let cmx = ct_to_result(
            ExtractedNoteCommitment::from_bytes(&action.cmx),
            "invalid note commitment",
        )?;
        let encrypted_note = TransmittedNoteCiphertext::<OrchardVanilla> {
            epk_bytes: action.epk_bytes,
            enc_ciphertext: NoteBytesData(action.enc_ciphertext),
            out_ciphertext: action.out_ciphertext,
        };
        let sig_bytes = decoded.spend_auth_sigs.get(idx)
            .ok_or_else(|| OrchardError::ParseError("missing spend auth signature".into()))?;
        let spend_auth = redpallas::Signature::<SpendAuth>::from(*sig_bytes);
        let spend_auth = VerSpendAuthSig::new(OrchardSighashVersion::NoVersion, spend_auth);

        actions.push(Action::from_parts(
            nullifier,
            rk,
            cmx,
            encrypted_note,
            cv_net,
            spend_auth,
        ));
    }

    let actions = NonEmpty::from_vec(actions)
        .ok_or_else(|| OrchardError::ParseError("bundle has no actions".into()))?;

    Ok(orchard::Bundle::from_parts(
        actions,
        flags,
        decoded.value_balance,
        Vec::new(),
        anchor,
        0,
        authorization,
    ))
}

#[cfg(all(feature = "orchard", feature = "std"))]
fn ct_to_result<T>(value: subtle::CtOption<T>, message: &str) -> Result<T> {
    if bool::from(value.is_some()) {
        Ok(value.unwrap())
    } else {
        Err(OrchardError::ParseError(message.into()))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_decode_zsa_bundle_minimal() {
        let mut bytes = Vec::new();

        // action_count = 1
        bytes.push(0x01);

        // Action
        bytes.extend_from_slice(&[1u8; 32]); // cv_net
        bytes.extend_from_slice(&[2u8; 32]); // nullifier
        bytes.extend_from_slice(&[3u8; 32]); // rk
        bytes.extend_from_slice(&[4u8; 32]); // cmx
        bytes.extend_from_slice(&[5u8; 32]); // epk_bytes
        bytes.extend_from_slice(&[6u8; ZSA_ENC_CIPHERTEXT_SIZE]); // enc_ciphertext
        bytes.extend_from_slice(&[7u8; 80]); // out_ciphertext

        // flags
        bytes.push(0x01);

        // anchor
        bytes.extend_from_slice(&[8u8; 32]);

        // expiry_height (u32 LE)
        bytes.extend_from_slice(&0u32.to_le_bytes());

        // burn_count = 0
        bytes.push(0x00);

        // proof_len = 4, proof bytes
        bytes.push(0x04);
        bytes.extend_from_slice(&[0xaa, 0xbb, 0xcc, 0xdd]);

        // spend_auth_sig (versioned)
        bytes.push(0x01);
        bytes.push(0x00);
        bytes.extend_from_slice(&[9u8; 64]);

        // value_balance (i64 LE) - CRITICAL FIELD
        bytes.extend_from_slice(&1000i64.to_le_bytes());

        // binding_sig (versioned)
        bytes.push(0x01);
        bytes.push(0x00);
        bytes.extend_from_slice(&[10u8; 64]);

        let decoded = decode_zsa_bundle(&bytes).unwrap();
        assert_eq!(decoded.actions.len(), 1);
        assert_eq!(decoded.flags, 0x01);
        assert_eq!(decoded.expiry_height, 0);
        assert_eq!(decoded.burns.len(), 0);
        assert_eq!(decoded.proof, vec![0xaa, 0xbb, 0xcc, 0xdd]);
        assert_eq!(decoded.spend_auth_sigs.len(), 1);
        assert_eq!(decoded.value_balance, 1000);
    }

    #[test]
    fn test_decode_zsa_bundle_with_burns() {
        let mut bytes = Vec::new();

        // action_count = 1
        bytes.push(0x01);

        // Action
        bytes.extend_from_slice(&[1u8; 32]); // cv_net
        bytes.extend_from_slice(&[2u8; 32]); // nullifier
        bytes.extend_from_slice(&[3u8; 32]); // rk
        bytes.extend_from_slice(&[4u8; 32]); // cmx
        bytes.extend_from_slice(&[5u8; 32]); // epk_bytes
        bytes.extend_from_slice(&[6u8; ZSA_ENC_CIPHERTEXT_SIZE]); // enc_ciphertext
        bytes.extend_from_slice(&[7u8; 80]); // out_ciphertext

        // flags
        bytes.push(0x01);

        // anchor
        bytes.extend_from_slice(&[8u8; 32]);

        // expiry_height
        bytes.extend_from_slice(&0u32.to_le_bytes());

        // burn_count = 2
        bytes.push(0x02);
        bytes.extend_from_slice(&[0u8; 32]); // Native USDx
        bytes.extend_from_slice(&100u64.to_le_bytes());
        bytes.extend_from_slice(&[1u8; 32]); // Custom asset
        bytes.extend_from_slice(&200u64.to_le_bytes());

        // proof_len = 0
        bytes.push(0x00);

        // spend_auth_sig (versioned)
        bytes.push(0x01);
        bytes.push(0x00);
        bytes.extend_from_slice(&[9u8; 64]);

        // value_balance (i64 LE)
        bytes.extend_from_slice(&(-500i64).to_le_bytes());

        // binding_sig (versioned)
        bytes.push(0x01);
        bytes.push(0x00);
        bytes.extend_from_slice(&[10u8; 64]);

        let decoded = decode_zsa_bundle(&bytes).unwrap();
        assert_eq!(decoded.burns.len(), 2);
        assert!(decoded.burns[0].asset.is_native());
        assert_eq!(decoded.burns[0].amount, 100);
        assert!(!decoded.burns[1].asset.is_native());
        assert_eq!(decoded.burns[1].amount, 200);
        assert_eq!(decoded.value_balance, -500);
    }

    #[test]
    fn test_decode_swap_bundle_two_groups() {
        let mut bytes = Vec::new();

        // num_action_groups = 2
        bytes.push(0x02);

        // Action group 1
        bytes.push(0x01); // action_count = 1
        bytes.extend_from_slice(&[1u8; 32]); // cv_net
        bytes.extend_from_slice(&[2u8; 32]); // nullifier
        bytes.extend_from_slice(&[3u8; 32]); // rk
        bytes.extend_from_slice(&[4u8; 32]); // cmx
        bytes.extend_from_slice(&[5u8; 32]); // epk_bytes
        bytes.extend_from_slice(&[6u8; ZSA_ENC_CIPHERTEXT_SIZE]); // enc_ciphertext
        bytes.extend_from_slice(&[7u8; 80]); // out_ciphertext
        bytes.push(0x01); // flags
        bytes.extend_from_slice(&[8u8; 32]); // anchor
        bytes.extend_from_slice(&0u32.to_le_bytes()); // expiry_height
        bytes.push(0x00); // burn_count = 0
        bytes.push(0x00); // proof_len = 0
        bytes.push(0x01);
        bytes.push(0x00);
        bytes.extend_from_slice(&[9u8; 64]); // spend_auth_sig (versioned)

        // Action group 2
        bytes.push(0x01); // action_count = 1
        bytes.extend_from_slice(&[11u8; 32]); // cv_net
        bytes.extend_from_slice(&[12u8; 32]); // nullifier
        bytes.extend_from_slice(&[13u8; 32]); // rk
        bytes.extend_from_slice(&[14u8; 32]); // cmx
        bytes.extend_from_slice(&[15u8; 32]); // epk_bytes
        bytes.extend_from_slice(&[16u8; ZSA_ENC_CIPHERTEXT_SIZE]); // enc_ciphertext
        bytes.extend_from_slice(&[17u8; 80]); // out_ciphertext
        bytes.push(0x02); // flags
        bytes.extend_from_slice(&[18u8; 32]); // anchor
        bytes.extend_from_slice(&0u32.to_le_bytes()); // expiry_height
        bytes.push(0x00); // burn_count = 0
        bytes.push(0x00); // proof_len = 0
        bytes.push(0x01);
        bytes.push(0x00);
        bytes.extend_from_slice(&[19u8; 64]); // spend_auth_sig (versioned)

        // Bundle-level fields
        bytes.extend_from_slice(&1000i64.to_le_bytes()); // value_balance
        bytes.push(0x01);
        bytes.push(0x00);
        bytes.extend_from_slice(&[20u8; 64]); // binding_sig (versioned)

        let decoded = decode_swap_bundle(&bytes).unwrap();
        assert_eq!(decoded.action_groups.len(), 2);
        assert_eq!(decoded.action_groups[0].flags, 0x01);
        assert_eq!(decoded.action_groups[1].flags, 0x02);
        assert_eq!(decoded.value_balance, 1000);
    }

    #[test]
    fn test_decode_issue_bundle_none_encoding() {
        let mut bytes = Vec::new();
        bytes.push(0x00); // issuer_len = 0
        bytes.push(0x00); // action_count = 0

        let decoded = decode_issue_bundle(&bytes).unwrap();
        assert!(decoded.is_none());
    }

    #[test]
    fn test_decode_issue_bundle_single_action() {
        let mut bytes = Vec::new();

        // issuer_len = 33 (algo byte + key)
        bytes.push(0x21);
        bytes.push(0x00); // algorithm byte
        bytes.extend_from_slice(&[1u8; 32]); // issuer_key

        // action_count = 1
        bytes.push(0x01);

        // IssueAction
        bytes.extend_from_slice(&[2u8; 32]); // asset_desc_hash
        bytes.push(0x01); // note_count = 1
        bytes.extend_from_slice(&[3u8; 43]); // recipient
        bytes.extend_from_slice(&1000u64.to_le_bytes()); // value
        bytes.extend_from_slice(&[4u8; 32]); // rho
        bytes.extend_from_slice(&[5u8; 32]); // rseed
        bytes.push(0x01); // finalize = true

        // sighash_info_len = 4
        bytes.push(0x04);
        bytes.extend_from_slice(&[0xaa, 0xbb, 0xcc, 0xdd]);

        // signature_len = 4
        bytes.push(0x04);
        bytes.extend_from_slice(&[0x11, 0x22, 0x33, 0x44]);

        let decoded = decode_issue_bundle(&bytes).unwrap().unwrap();
        let mut expected_issuer = vec![0x00];
        expected_issuer.extend_from_slice(&[1u8; 32]);
        assert_eq!(decoded.issuer_key, expected_issuer);
        assert_eq!(decoded.actions.len(), 1);
        assert_eq!(decoded.actions[0].asset_desc_hash, [2u8; 32]);
        assert_eq!(decoded.actions[0].notes.len(), 1);
        assert_eq!(decoded.actions[0].notes[0].value, 1000);
        assert!(decoded.actions[0].finalize);
        assert_eq!(decoded.sighash_info, vec![0xaa, 0xbb, 0xcc, 0xdd]);
        assert_eq!(decoded.signature, vec![0x11, 0x22, 0x33, 0x44]);
    }

    #[test]
    fn test_decode_swap_bundle_rejects_empty() {
        let bytes = vec![0x00]; // num_groups = 0
        let result = decode_swap_bundle(&bytes);
        assert!(result.is_err());
        assert!(matches!(result.unwrap_err(), OrchardError::ParseError(_)));
    }

    #[test]
    fn test_decode_zsa_bundle_rejects_empty_actions() {
        let bytes = vec![0x00]; // action_count = 0
        let result = decode_zsa_bundle(&bytes);
        assert!(result.is_err());
        assert!(matches!(result.unwrap_err(), OrchardError::ParseError(_)));
    }

    #[test]
    fn test_decode_zsa_bundle_rejects_nonzero_expiry() {
        let mut bytes = Vec::new();
        bytes.push(0x01); // action_count = 1
        bytes.extend_from_slice(&[1u8; 32]); // cv_net
        bytes.extend_from_slice(&[2u8; 32]); // nullifier
        bytes.extend_from_slice(&[3u8; 32]); // rk
        bytes.extend_from_slice(&[4u8; 32]); // cmx
        bytes.extend_from_slice(&[5u8; 32]); // epk_bytes
        bytes.extend_from_slice(&[6u8; ZSA_ENC_CIPHERTEXT_SIZE]); // enc_ciphertext
        bytes.extend_from_slice(&[7u8; 80]); // out_ciphertext
        bytes.push(0x01); // flags
        bytes.extend_from_slice(&[8u8; 32]); // anchor
        bytes.extend_from_slice(&12345u32.to_le_bytes()); // INVALID: non-zero expiry

        let result = decode_zsa_bundle(&bytes);
        assert!(result.is_err());
        let err_msg = format!("{:?}", result.unwrap_err());
        assert!(err_msg.contains("expiry_height must be 0"));
    }
}
