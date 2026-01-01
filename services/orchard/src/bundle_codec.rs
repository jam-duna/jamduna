#![allow(dead_code)]
//! Zcash v5 Orchard bundle decoding for the Orchard service.
//!
//! This module parses the Orchard bundle bytes embedded in JAM extrinsics and
//! exposes the minimal fields needed by the refiner.

use alloc::vec::Vec;
use crate::errors::{OrchardError, Result};

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

#[cfg(all(feature = "orchard", feature = "std"))]
pub fn to_orchard_bundle(
    decoded: &DecodedBundle,
) -> Result<orchard::Bundle<orchard::bundle::Authorized, i64>> {
    use orchard::{
        Action, Anchor, Proof,
        bundle::{Authorized, Flags},
        note::{ExtractedNoteCommitment, Nullifier, TransmittedNoteCiphertext},
        primitives::redpallas::{self, Binding, SpendAuth},
        value::ValueCommitment,
    };
    use nonempty::NonEmpty;

    if decoded.actions.is_empty() {
        return Err(OrchardError::ParseError("bundle has no actions".into()));
    }

    let flags = Flags::from_byte(decoded.flags)
        .ok_or_else(|| OrchardError::ParseError("invalid Orchard flags".into()))?;
    let anchor = ct_to_result(Anchor::from_bytes(decoded.anchor), "invalid anchor")?;
    let proof = Proof::new(decoded.proof.clone());
    let binding_sig = redpallas::Signature::<Binding>::from(decoded.binding_sig);
    let authorization = Authorized::from_parts(proof, binding_sig);

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
        let encrypted_note = TransmittedNoteCiphertext {
            epk_bytes: action.epk_bytes,
            enc_ciphertext: action.enc_ciphertext,
            out_ciphertext: action.out_ciphertext,
        };
        let sig_bytes = decoded.spend_auth_sigs.get(idx)
            .ok_or_else(|| OrchardError::ParseError("missing spend auth signature".into()))?;
        let spend_auth = redpallas::Signature::<SpendAuth>::from(*sig_bytes);

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
        anchor,
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
