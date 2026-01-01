// Zcash v5 Orchard bundle serialization/deserialization.
//
// This matches the canonical Orchard bundle encoding used in Zcash v5
// transactions, so bundles can be transported as bytes inside JAM extrinsics.

use crate::{Error, OrchardBundle, Result};
use nonempty::NonEmpty;
use orchard::{
    Action, Anchor, Proof,
    bundle::{Authorized, Flags},
    note::{ExtractedNoteCommitment, Nullifier, TransmittedNoteCiphertext},
    primitives::redpallas::{self, SpendAuth},
    value::ValueCommitment,
};
use std::io::{Cursor, Read, Write};
use subtle::CtOption;

pub fn serialize_bundle(bundle: &OrchardBundle) -> Result<Vec<u8>> {
    let mut out = Vec::new();

    let action_count = bundle.actions().len() as u64;
    write_compactsize(&mut out, action_count)?;

    for action in bundle.actions().iter() {
        out.extend_from_slice(&action.cv_net().to_bytes());
        out.extend_from_slice(&action.nullifier().to_bytes());
        let rk_bytes: [u8; 32] = action.rk().into();
        out.extend_from_slice(&rk_bytes);
        out.extend_from_slice(&action.cmx().to_bytes());

        let encrypted = action.encrypted_note();
        out.extend_from_slice(&encrypted.epk_bytes);
        out.extend_from_slice(&encrypted.enc_ciphertext);
        out.extend_from_slice(&encrypted.out_ciphertext);
    }

    out.push(bundle.flags().to_byte());
    out.extend_from_slice(&(*bundle.value_balance()).to_le_bytes());
    out.extend_from_slice(&bundle.anchor().to_bytes());

    let proof_bytes = bundle.authorization().proof().as_ref();
    write_compactsize(&mut out, proof_bytes.len() as u64)?;
    out.extend_from_slice(proof_bytes);

    for action in bundle.actions().iter() {
        let sig_bytes: [u8; 64] = action.authorization().into();
        out.extend_from_slice(&sig_bytes);
    }

    let binding_sig: [u8; 64] = bundle.authorization().binding_signature().into();
    out.extend_from_slice(&binding_sig);

    Ok(out)
}

pub fn deserialize_bundle(bytes: &[u8]) -> Result<OrchardBundle> {
    let mut cursor = Cursor::new(bytes);
    let action_count = read_compactsize(&mut cursor)?;
    if action_count == 0 {
        return Err(Error::InvalidBundle("Orchard bundle is empty".to_string()));
    }

    let action_count = usize::try_from(action_count).map_err(|_| {
        Error::SerializationError("Action count does not fit in usize".to_string())
    })?;

    let mut action_parts = Vec::with_capacity(action_count);
    for _ in 0..action_count {
        let cv_net_bytes = read_bytes::<32, _>(&mut cursor)?;
        let cv_net = ct_to_result(
            ValueCommitment::from_bytes(&cv_net_bytes),
            "Invalid value commitment",
        )?;

        let nf_bytes = read_bytes::<32, _>(&mut cursor)?;
        let nf = ct_to_result(
            Nullifier::from_bytes(&nf_bytes),
            "Invalid nullifier",
        )?;

        let rk_bytes = read_bytes::<32, _>(&mut cursor)?;
        let rk = redpallas::VerificationKey::<SpendAuth>::try_from(rk_bytes)
            .map_err(|_| Error::SerializationError("Invalid spend auth key".to_string()))?;

        let cmx_bytes = read_bytes::<32, _>(&mut cursor)?;
        let cmx = ct_to_result(
            ExtractedNoteCommitment::from_bytes(&cmx_bytes),
            "Invalid note commitment",
        )?;

        let epk_bytes = read_bytes::<32, _>(&mut cursor)?;
        let enc_ciphertext = read_bytes::<580, _>(&mut cursor)?;
        let out_ciphertext = read_bytes::<80, _>(&mut cursor)?;
        let encrypted_note = TransmittedNoteCiphertext {
            epk_bytes,
            enc_ciphertext,
            out_ciphertext,
        };

        action_parts.push((nf, rk, cmx, encrypted_note, cv_net));
    }

    let flags_byte = read_bytes::<1, _>(&mut cursor)?[0];
    let flags = Flags::from_byte(flags_byte)
        .ok_or_else(|| Error::SerializationError("Invalid Orchard flags".to_string()))?;

    let value_balance_bytes = read_bytes::<8, _>(&mut cursor)?;
    let value_balance = i64::from_le_bytes(value_balance_bytes);

    let anchor_bytes = read_bytes::<32, _>(&mut cursor)?;
    let anchor = ct_to_result(
        Anchor::from_bytes(anchor_bytes),
        "Invalid Orchard anchor",
    )?;

    let proof_len = read_compactsize(&mut cursor)?;
    let proof_len = usize::try_from(proof_len).map_err(|_| {
        Error::SerializationError("Proof length does not fit in usize".to_string())
    })?;

    let mut proof_bytes = vec![0u8; proof_len];
    cursor.read_exact(&mut proof_bytes).map_err(map_io)?;
    let proof = Proof::new(proof_bytes);

    let mut actions = Vec::with_capacity(action_count);
    for (nf, rk, cmx, encrypted_note, cv_net) in action_parts {
        let sig_bytes = read_bytes::<64, _>(&mut cursor)?;
        let spend_auth = redpallas::Signature::<SpendAuth>::from(sig_bytes);
        actions.push(Action::from_parts(
            nf,
            rk,
            cmx,
            encrypted_note,
            cv_net,
            spend_auth,
        ));
    }

    let binding_sig_bytes = read_bytes::<64, _>(&mut cursor)?;
    let binding_sig = redpallas::Signature::from(binding_sig_bytes);

    if cursor.position() as usize != bytes.len() {
        return Err(Error::SerializationError(
            "Trailing bytes in Orchard bundle encoding".to_string(),
        ));
    }

    let actions = NonEmpty::from_vec(actions).ok_or_else(|| {
        Error::InvalidBundle("Orchard bundle actions cannot be empty".to_string())
    })?;
    let authorization = Authorized::from_parts(proof, binding_sig);

    Ok(OrchardBundle::from_parts(
        actions,
        flags,
        value_balance,
        anchor,
        authorization,
    ))
}

fn write_compactsize<W: Write>(writer: &mut W, value: u64) -> Result<()> {
    if value < 0xfd {
        writer.write_all(&[value as u8]).map_err(map_io)?;
        return Ok(());
    }
    if value <= 0xffff {
        writer.write_all(&[0xfd]).map_err(map_io)?;
        writer
            .write_all(&(value as u16).to_le_bytes())
            .map_err(map_io)?;
        return Ok(());
    }
    if value <= 0xffff_ffff {
        writer.write_all(&[0xfe]).map_err(map_io)?;
        writer
            .write_all(&(value as u32).to_le_bytes())
            .map_err(map_io)?;
        return Ok(());
    }
    writer.write_all(&[0xff]).map_err(map_io)?;
    writer
        .write_all(&value.to_le_bytes())
        .map_err(map_io)?;
    Ok(())
}

fn read_compactsize<R: Read>(reader: &mut R) -> Result<u64> {
    let tag = read_bytes::<1, _>(reader)?[0];
    match tag {
        0x00..=0xfc => Ok(tag as u64),
        0xfd => {
            let value = u16::from_le_bytes(read_bytes::<2, _>(reader)?);
            if value < 0xfd {
                return Err(Error::SerializationError(
                    "Non-canonical CompactSize encoding".to_string(),
                ));
            }
            Ok(value as u64)
        }
        0xfe => {
            let value = u32::from_le_bytes(read_bytes::<4, _>(reader)?);
            if value <= 0xffff {
                return Err(Error::SerializationError(
                    "Non-canonical CompactSize encoding".to_string(),
                ));
            }
            Ok(value as u64)
        }
        0xff => {
            let value = u64::from_le_bytes(read_bytes::<8, _>(reader)?);
            if value <= 0xffff_ffff {
                return Err(Error::SerializationError(
                    "Non-canonical CompactSize encoding".to_string(),
                ));
            }
            Ok(value)
        }
    }
}

fn read_bytes<const N: usize, R: Read>(reader: &mut R) -> Result<[u8; N]> {
    let mut buf = [0u8; N];
    reader.read_exact(&mut buf).map_err(map_io)?;
    Ok(buf)
}

fn ct_to_result<T>(value: CtOption<T>, message: &str) -> Result<T> {
    if bool::from(value.is_some()) {
        Ok(value.unwrap())
    } else {
        Err(Error::SerializationError(message.to_string()))
    }
}

fn map_io(err: std::io::Error) -> Error {
    Error::SerializationError(err.to_string())
}
