//! Bitcoin/Zcash P2PKH script validation
//!
//! Implements basic Pay-to-PubKey-Hash script validation for transparent verification.
//! This is a no_std implementation suitable for PolkaVM execution.

use alloc::vec::Vec;
use crate::errors::{OrchardError, Result};
use libsecp256k1::{Message, PublicKey, Signature, verify};
use ripemd::Ripemd160;
use sha2::{Digest, Sha256};

/// Bitcoin script opcodes (subset for P2PKH/P2SH + basic stack ops).
#[allow(dead_code)]
mod opcode {
    pub const OP_0: u8 = 0x00;
    pub const OP_PUSHDATA1: u8 = 0x4c;
    pub const OP_PUSHDATA2: u8 = 0x4d;
    pub const OP_PUSHDATA4: u8 = 0x4e;
    pub const OP_1NEGATE: u8 = 0x4f;
    pub const OP_1: u8 = 0x51;
    pub const OP_16: u8 = 0x60;
    pub const OP_NOP: u8 = 0x61;
    pub const OP_IF: u8 = 0x63;
    pub const OP_NOTIF: u8 = 0x64;
    pub const OP_ELSE: u8 = 0x67;
    pub const OP_ENDIF: u8 = 0x68;
    pub const OP_TOALTSTACK: u8 = 0x6b;
    pub const OP_FROMALTSTACK: u8 = 0x6c;
    pub const OP_2DROP: u8 = 0x6d;
    pub const OP_2DUP: u8 = 0x6e;
    pub const OP_3DUP: u8 = 0x6f;
    pub const OP_2OVER: u8 = 0x70;
    pub const OP_2ROT: u8 = 0x71;
    pub const OP_2SWAP: u8 = 0x72;
    pub const OP_IFDUP: u8 = 0x73;
    pub const OP_DEPTH: u8 = 0x74;
    pub const OP_DROP: u8 = 0x75;
    pub const OP_DUP: u8 = 0x76;
    pub const OP_NIP: u8 = 0x77;
    pub const OP_OVER: u8 = 0x78;
    pub const OP_PICK: u8 = 0x79;
    pub const OP_ROLL: u8 = 0x7a;
    pub const OP_ROT: u8 = 0x7b;
    pub const OP_SWAP: u8 = 0x7c;
    pub const OP_TUCK: u8 = 0x7d;
    pub const OP_SIZE: u8 = 0x82;
    pub const OP_1ADD: u8 = 0x8b;
    pub const OP_1SUB: u8 = 0x8c;
    pub const OP_NEGATE: u8 = 0x8f;
    pub const OP_ABS: u8 = 0x90;
    pub const OP_NOT: u8 = 0x91;
    pub const OP_0NOTEQUAL: u8 = 0x92;
    pub const OP_ADD: u8 = 0x93;
    pub const OP_SUB: u8 = 0x94;
    pub const OP_BOOLAND: u8 = 0x9a;
    pub const OP_BOOLOR: u8 = 0x9b;
    pub const OP_NUMEQUAL: u8 = 0x9c;
    pub const OP_NUMEQUALVERIFY: u8 = 0x9d;
    pub const OP_NUMNOTEQUAL: u8 = 0x9e;
    pub const OP_LESSTHAN: u8 = 0x9f;
    pub const OP_GREATERTHAN: u8 = 0xa0;
    pub const OP_LESSTHANOREQUAL: u8 = 0xa1;
    pub const OP_GREATERTHANOREQUAL: u8 = 0xa2;
    pub const OP_MIN: u8 = 0xa3;
    pub const OP_MAX: u8 = 0xa4;
    pub const OP_WITHIN: u8 = 0xa5;
    pub const OP_RIPEMD160: u8 = 0xa6;
    pub const OP_SHA1: u8 = 0xa7;
    pub const OP_SHA256: u8 = 0xa8;
    pub const OP_HASH160: u8 = 0xa9;
    pub const OP_HASH256: u8 = 0xaa;
    pub const OP_EQUAL: u8 = 0x87;
    pub const OP_EQUALVERIFY: u8 = 0x88;
    pub const OP_VERIFY: u8 = 0x69;
    pub const OP_RETURN: u8 = 0x6a;
    pub const OP_CHECKSIG: u8 = 0xac;
    pub const OP_CHECKSIGVERIFY: u8 = 0xad;
    pub const OP_CHECKMULTISIG: u8 = 0xae;
    pub const OP_CHECKMULTISIGVERIFY: u8 = 0xaf;
    pub const OP_CHECKLOCKTIMEVERIFY: u8 = 0xb1;
    pub const OP_CHECKSEQUENCEVERIFY: u8 = 0xb2;
}

const LOCK_TIME_THRESHOLD: u32 = 500_000_000;
const FINAL_SEQUENCE: u32 = 0xFFFF_FFFF;
const SEQUENCE_LOCKTIME_DISABLE_FLAG: u32 = 1 << 31;
const SEQUENCE_LOCKTIME_TYPE_FLAG: u32 = 1 << 22;
const SEQUENCE_LOCKTIME_MASK: u32 = 0x0000_FFFF;

/// Script execution context for timelock opcodes.
pub struct ScriptContext {
    pub lock_time: u32,
    pub sequence: u32,
    pub tx_version: u32,
}

/// Bitcoin script execution stack (minimal implementation).
pub struct ScriptStack {
    stack: Vec<Vec<u8>>,
    alt_stack: Vec<Vec<u8>>,
    max_size: usize,
}

impl ScriptStack {
    pub fn new() -> Self {
        Self {
            stack: Vec::new(),
            alt_stack: Vec::new(),
            max_size: 1000,
        }
    }

    pub fn push(&mut self, data: &[u8]) -> Result<()> {
        if self.stack.len() >= self.max_size {
            return Err(OrchardError::ParseError("stack overflow".into()));
        }
        if data.len() > 520 {
            return Err(OrchardError::ParseError("stack element too large".into()));
        }
        self.stack.push(data.to_vec());
        Ok(())
    }

    pub fn pop(&mut self) -> Result<Vec<u8>> {
        self.stack
            .pop()
            .ok_or_else(|| OrchardError::ParseError("stack underflow".into()))
    }

    pub fn top(&self) -> Result<Vec<u8>> {
        self.stack
            .last()
            .cloned()
            .ok_or_else(|| OrchardError::ParseError("stack empty".into()))
    }

    pub fn len(&self) -> usize {
        self.stack.len()
    }

    pub fn peek(&self, index_from_top: usize) -> Result<Vec<u8>> {
        let len = self.stack.len();
        if index_from_top >= len {
            return Err(OrchardError::ParseError("stack underflow".into()));
        }
        Ok(self.stack[len - 1 - index_from_top].clone())
    }

    pub fn remove_from_top(&mut self, index_from_top: usize) -> Result<Vec<u8>> {
        let len = self.stack.len();
        if index_from_top >= len {
            return Err(OrchardError::ParseError("stack underflow".into()));
        }
        Ok(self.stack.remove(len - 1 - index_from_top))
    }

    pub fn push_alt(&mut self, data: &[u8]) -> Result<()> {
        if self.alt_stack.len() >= self.max_size {
            return Err(OrchardError::ParseError("alt stack overflow".into()));
        }
        if data.len() > 520 {
            return Err(OrchardError::ParseError("stack element too large".into()));
        }
        self.alt_stack.push(data.to_vec());
        Ok(())
    }

    pub fn pop_alt(&mut self) -> Result<Vec<u8>> {
        self.alt_stack
            .pop()
            .ok_or_else(|| OrchardError::ParseError("alt stack underflow".into()))
    }

    pub fn items(&self) -> &[Vec<u8>] {
        &self.stack
    }
}

/// P2PKH scriptPubKey format:
/// OP_DUP OP_HASH160 <20-byte pubkey hash> OP_EQUALVERIFY OP_CHECKSIG
pub fn is_p2pkh(script: &[u8]) -> bool {
    script.len() == 25
        && script[0] == opcode::OP_DUP
        && script[1] == opcode::OP_HASH160
        && script[2] == 0x14 // Push 20 bytes
        && script[23] == opcode::OP_EQUALVERIFY
        && script[24] == opcode::OP_CHECKSIG
}

/// Extract pubkey hash from P2PKH scriptPubKey
pub fn extract_p2pkh_hash(script: &[u8]) -> Result<[u8; 20]> {
    if !is_p2pkh(script) {
        return Err(OrchardError::ParseError("not a P2PKH script".into()));
    }
    let mut hash = [0u8; 20];
    hash.copy_from_slice(&script[3..23]);
    Ok(hash)
}

/// P2SH scriptPubKey format:
/// OP_HASH160 <20-byte script hash> OP_EQUAL
pub fn is_p2sh(script: &[u8]) -> bool {
    script.len() == 23
        && script[0] == opcode::OP_HASH160
        && script[1] == 0x14
        && script[22] == opcode::OP_EQUAL
}

/// Extract script hash from P2SH scriptPubKey
pub fn extract_p2sh_hash(script: &[u8]) -> Result<[u8; 20]> {
    if !is_p2sh(script) {
        return Err(OrchardError::ParseError("not a P2SH script".into()));
    }
    let mut hash = [0u8; 20];
    hash.copy_from_slice(&script[2..22]);
    Ok(hash)
}

/// P2WPKH scriptPubKey format:
/// OP_0 <20-byte witness program>
pub fn is_p2wpkh(script: &[u8]) -> bool {
    script.len() == 22
        && script[0] == opcode::OP_0
        && script[1] == 0x14
}

/// P2WSH scriptPubKey format:
/// OP_0 <32-byte witness program>
pub fn is_p2wsh(script: &[u8]) -> bool {
    script.len() == 34
        && script[0] == opcode::OP_0
        && script[1] == 0x20
}

/// Parsed P2PKH scriptSig components.
pub struct P2PKHScriptSig {
    pub signature: Vec<u8>, // DER signature (without sighash byte)
    pub hash_type: u8,
    pub pubkey: Vec<u8>,
}

/// Parse P2PKH scriptSig into signature + hash type + pubkey.
pub fn parse_p2pkh_script_sig(script_sig: &[u8]) -> Result<P2PKHScriptSig> {
    let mut cursor = 0usize;
    let sig = read_push(script_sig, &mut cursor)?;
    let pubkey = read_push(script_sig, &mut cursor)?;

    if cursor != script_sig.len() {
        return Err(OrchardError::ParseError("unexpected trailing bytes in scriptSig".into()));
    }
    if sig.len() < 2 {
        return Err(OrchardError::ParseError("signature too short".into()));
    }

    let hash_type = sig[sig.len() - 1];
    let signature = sig[..sig.len() - 1].to_vec();

    Ok(P2PKHScriptSig {
        signature,
        hash_type,
        pubkey,
    })
}

/// Verify P2PKH signature against scriptPubKey and sighash.
pub fn verify_p2pkh(
    pubkey: &[u8],
    signature: &[u8],
    script_pubkey: &[u8],
    sighash: &[u8; 32],
) -> Result<()> {
    if !is_p2pkh(script_pubkey) {
        return Err(OrchardError::ParseError("scriptPubKey is not P2PKH".into()));
    }

    let expected_hash = extract_p2pkh_hash(script_pubkey)?;
    let computed_hash = hash160(pubkey);
    if computed_hash != expected_hash {
        return Err(OrchardError::ParseError("pubkey hash mismatch".into()));
    }

    let sig64 = parse_der_signature(signature)?;
    let sig = Signature::parse_standard(&sig64)
        .map_err(|_| OrchardError::ParseError("invalid DER signature".into()))?;
    let msg = Message::parse(sighash);
    let pk = parse_pubkey(pubkey)?;

    if !verify(&msg, &sig, &pk) {
        return Err(OrchardError::ParseError("invalid ECDSA signature".into()));
    }

    Ok(())
}

/// Extract redeem script from a P2SH scriptSig (last push).
pub fn extract_redeem_script(script_sig: &[u8]) -> Result<Vec<u8>> {
    let mut cursor = 0usize;
    let mut last_push: Option<Vec<u8>> = None;
    while cursor < script_sig.len() {
        let data = read_push(script_sig, &mut cursor)?;
        last_push = Some(data);
    }
    last_push.ok_or_else(|| OrchardError::ParseError("no redeem script found".into()))
}

/// Extract unlocking data pushes (all pushes except the last one).
pub fn extract_unlocking_data(script_sig: &[u8]) -> Result<Vec<Vec<u8>>> {
    let mut cursor = 0usize;
    let mut pushes = Vec::new();
    while cursor < script_sig.len() {
        let data = read_push(script_sig, &mut cursor)?;
        pushes.push(data);
    }
    if !pushes.is_empty() {
        pushes.pop();
    }
    Ok(pushes)
}

/// Extract a hash type byte for P2SH sighash computation.
pub fn extract_p2sh_sighash_type(script_sig: &[u8]) -> Result<u8> {
    let unlocking = extract_unlocking_data(script_sig)?;
    for data in unlocking.iter().rev() {
        if data.is_empty() {
            continue;
        }
        return Ok(data[data.len() - 1]);
    }
    Ok(0x01)
}

fn parse_num(bytes: &[u8]) -> Result<i64> {
    if bytes.is_empty() {
        return Ok(0);
    }
    if bytes.len() > 4 {
        return Err(OrchardError::ParseError("number too large".into()));
    }

    let mut num: i64 = 0;
    for (i, &byte) in bytes.iter().enumerate() {
        if i == bytes.len() - 1 && (byte & 0x80) != 0 {
            num |= ((byte & 0x7f) as i64) << (8 * i);
            num = -num;
        } else {
            num |= (byte as i64) << (8 * i);
        }
    }
    Ok(num)
}

fn number_to_bytes(num: i64) -> Vec<u8> {
    if num == 0 {
        return Vec::new();
    }

    let negative = num < 0;
    let mut abs = if negative {
        (-num) as u64
    } else {
        num as u64
    };
    let mut out = Vec::new();

    while abs > 0 {
        out.push((abs & 0xff) as u8);
        abs >>= 8;
    }

    if let Some(last) = out.last_mut() {
        if *last & 0x80 != 0 {
            out.push(if negative { 0x80 } else { 0x00 });
        } else if negative {
            *last |= 0x80;
        }
    }

    out
}

fn is_true(data: &[u8]) -> bool {
    if data.is_empty() {
        return false;
    }

    for &byte in &data[..data.len() - 1] {
        if byte != 0 {
            return true;
        }
    }

    let last = data[data.len() - 1];
    last != 0 && last != 0x80
}

fn op_verify(stack: &mut ScriptStack) -> Result<()> {
    let value = stack.pop()?;
    if !is_true(&value) {
        return Err(OrchardError::ParseError("VERIFY failed".into()));
    }
    Ok(())
}

fn is_executing(exec_stack: &[bool]) -> bool {
    exec_stack.iter().all(|flag| *flag)
}

fn op_checklocktimeverify(stack: &mut ScriptStack, ctx: Option<&ScriptContext>) -> Result<()> {
    let ctx = ctx.ok_or_else(|| OrchardError::ParseError("missing script context".into()))?;
    let lock_bytes = stack.peek(0)?;
    let lock_value = parse_num(&lock_bytes)?;
    if lock_value < 0 {
        return Err(OrchardError::ParseError("negative locktime".into()));
    }
    let required = lock_value as u32;
    let tx_lock_time = ctx.lock_time;

    if required < LOCK_TIME_THRESHOLD && tx_lock_time >= LOCK_TIME_THRESHOLD {
        return Err(OrchardError::ParseError("locktime type mismatch".into()));
    }
    if required >= LOCK_TIME_THRESHOLD && tx_lock_time < LOCK_TIME_THRESHOLD {
        return Err(OrchardError::ParseError("locktime type mismatch".into()));
    }
    if tx_lock_time < required {
        return Err(OrchardError::ParseError("locktime not yet reached".into()));
    }
    if ctx.sequence == FINAL_SEQUENCE {
        return Err(OrchardError::ParseError("sequence disables locktime".into()));
    }
    Ok(())
}

fn op_checksequenceverify(stack: &mut ScriptStack, ctx: Option<&ScriptContext>) -> Result<()> {
    let ctx = ctx.ok_or_else(|| OrchardError::ParseError("missing script context".into()))?;
    let seq_bytes = stack.peek(0)?;
    let seq_value = parse_num(&seq_bytes)?;
    if seq_value < 0 {
        return Err(OrchardError::ParseError("negative sequence".into()));
    }
    if ctx.tx_version < 2 {
        return Err(OrchardError::ParseError("tx version too low for CSV".into()));
    }
    let required = seq_value as u32;
    if (required & SEQUENCE_LOCKTIME_DISABLE_FLAG) != 0 {
        return Err(OrchardError::ParseError("sequence disable flag set".into()));
    }
    if (ctx.sequence & SEQUENCE_LOCKTIME_DISABLE_FLAG) != 0 {
        return Err(OrchardError::ParseError("input sequence disables CSV".into()));
    }

    let required_type = required & SEQUENCE_LOCKTIME_TYPE_FLAG;
    let tx_type = ctx.sequence & SEQUENCE_LOCKTIME_TYPE_FLAG;
    if required_type != tx_type {
        return Err(OrchardError::ParseError("sequence type mismatch".into()));
    }

    let required_masked = required & SEQUENCE_LOCKTIME_MASK;
    let tx_masked = ctx.sequence & SEQUENCE_LOCKTIME_MASK;
    if tx_masked < required_masked {
        return Err(OrchardError::ParseError("sequence not yet satisfied".into()));
    }
    Ok(())
}

fn op_equal(stack: &mut ScriptStack) -> Result<()> {
    if stack.len() < 2 {
        return Err(OrchardError::ParseError("stack underflow".into()));
    }
    let a = stack.pop()?;
    let b = stack.pop()?;
    if a == b {
        stack.push(&[0x01])?;
    } else {
        stack.push(&[])?;
    }
    Ok(())
}

fn hash256(data: &[u8]) -> [u8; 32] {
    let first = Sha256::digest(data);
    let second = Sha256::digest(first);
    let mut out = [0u8; 32];
    out.copy_from_slice(&second);
    out
}

fn verify_ecdsa(pubkey: &[u8], signature: &[u8], sighash: &[u8; 32]) -> Result<()> {
    let sig64 = parse_der_signature(signature)?;
    let sig = Signature::parse_standard(&sig64)
        .map_err(|_| OrchardError::ParseError("invalid DER signature".into()))?;
    let msg = Message::parse(sighash);
    let pk = parse_pubkey(pubkey)?;

    if !verify(&msg, &sig, &pk) {
        return Err(OrchardError::ParseError("invalid ECDSA signature".into()));
    }

    Ok(())
}

fn execute_checkmultisig(stack: &mut ScriptStack, sighash: &[u8; 32]) -> Result<()> {
    let n_bytes = stack.pop()?;
    let n = parse_num(&n_bytes)?;
    if n < 0 || n > 20 {
        return Err(OrchardError::ParseError("invalid N in CHECKMULTISIG".into()));
    }

    let mut pubkeys = Vec::new();
    for _ in 0..(n as usize) {
        pubkeys.push(stack.pop()?);
    }

    let m_bytes = stack.pop()?;
    let m = parse_num(&m_bytes)?;
    if m < 0 || m > n {
        return Err(OrchardError::ParseError("invalid M in CHECKMULTISIG".into()));
    }

    let mut signatures = Vec::new();
    for _ in 0..(m as usize) {
        signatures.push(stack.pop()?);
    }

    let dummy = stack.pop()?;
    if !dummy.is_empty() {
        return Err(OrchardError::ParseError(
            "CHECKMULTISIG dummy value not null (NULLDUMMY)".into(),
        ));
    }

    let mut sig_idx = 0usize;
    let mut key_idx = 0usize;

    while sig_idx < signatures.len() && key_idx < pubkeys.len() {
        let sig_with_hashtype = &signatures[sig_idx];
        if sig_with_hashtype.len() < 2 {
            return Err(OrchardError::ParseError("invalid signature in CHECKMULTISIG".into()));
        }
        let signature = &sig_with_hashtype[..sig_with_hashtype.len() - 1];
        let pubkey = &pubkeys[key_idx];

        if verify_ecdsa(pubkey, signature, sighash).is_ok() {
            sig_idx += 1;
        }
        key_idx += 1;
    }

    if sig_idx == signatures.len() {
        stack.push(&[0x01])?;
    } else {
        stack.push(&[])?;
    }

    Ok(())
}

pub fn execute_script(
    script: &[u8],
    stack: &mut ScriptStack,
    sighash: &[u8; 32],
) -> Result<()> {
    execute_script_with_context(script, stack, sighash, None)
}

pub fn execute_script_with_context(
    script: &[u8],
    stack: &mut ScriptStack,
    sighash: &[u8; 32],
    ctx: Option<&ScriptContext>,
) -> Result<()> {
    let mut cursor = 0usize;
    let mut exec_stack: Vec<bool> = Vec::new();

    while cursor < script.len() {
        let opcode = script[cursor];
        cursor += 1;

        let executing = is_executing(&exec_stack);

        match opcode {
            opcode::OP_0 => {
                if executing {
                    stack.push(&[])?;
                }
            }
            0x01..=0x4b => {
                let len = opcode as usize;
                if cursor + len > script.len() {
                    return Err(OrchardError::ParseError("push data out of bounds".into()));
                }
                if executing {
                    stack.push(&script[cursor..cursor + len])?;
                }
                cursor += len;
            }
            opcode::OP_PUSHDATA1 => {
                if cursor >= script.len() {
                    return Err(OrchardError::ParseError("OP_PUSHDATA1 out of bounds".into()));
                }
                let len = script[cursor] as usize;
                cursor += 1;
                if cursor + len > script.len() {
                    return Err(OrchardError::ParseError("OP_PUSHDATA1 data out of bounds".into()));
                }
                if executing {
                    stack.push(&script[cursor..cursor + len])?;
                }
                cursor += len;
            }
            opcode::OP_PUSHDATA2 => {
                if cursor + 2 > script.len() {
                    return Err(OrchardError::ParseError("OP_PUSHDATA2 out of bounds".into()));
                }
                let len = u16::from_le_bytes([script[cursor], script[cursor + 1]]) as usize;
                cursor += 2;
                if cursor + len > script.len() {
                    return Err(OrchardError::ParseError("OP_PUSHDATA2 data out of bounds".into()));
                }
                if executing {
                    stack.push(&script[cursor..cursor + len])?;
                }
                cursor += len;
            }
            opcode::OP_PUSHDATA4 => {
                if cursor + 4 > script.len() {
                    return Err(OrchardError::ParseError("OP_PUSHDATA4 out of bounds".into()));
                }
                let len = u32::from_le_bytes([
                    script[cursor],
                    script[cursor + 1],
                    script[cursor + 2],
                    script[cursor + 3],
                ]) as usize;
                cursor += 4;
                if cursor + len > script.len() {
                    return Err(OrchardError::ParseError("OP_PUSHDATA4 data out of bounds".into()));
                }
                if executing {
                    stack.push(&script[cursor..cursor + len])?;
                }
                cursor += len;
            }
            opcode::OP_1NEGATE => {
                if executing {
                    stack.push(&[0x81])?;
                }
            }
            opcode::OP_1..=opcode::OP_16 => {
                if executing {
                    let value = (opcode - 0x50) as u8;
                    stack.push(&[value])?;
                }
            }
            opcode::OP_NOP => {}
            opcode::OP_RETURN => {
                if executing {
                    return Err(OrchardError::ParseError("OP_RETURN executed".into()));
                }
            }
            opcode::OP_IF => {
                if executing {
                    let value = stack.pop()?;
                    exec_stack.push(is_true(&value));
                } else {
                    exec_stack.push(false);
                }
            }
            opcode::OP_NOTIF => {
                if executing {
                    let value = stack.pop()?;
                    exec_stack.push(!is_true(&value));
                } else {
                    exec_stack.push(false);
                }
            }
            opcode::OP_ELSE => {
                if exec_stack.is_empty() {
                    return Err(OrchardError::ParseError("ELSE without IF".into()));
                }
                let parent_exec = is_executing(&exec_stack[..exec_stack.len() - 1]);
                let current = *exec_stack.last().unwrap();
                let next = if parent_exec { !current } else { false };
                *exec_stack.last_mut().unwrap() = next;
            }
            opcode::OP_ENDIF => {
                if exec_stack.is_empty() {
                    return Err(OrchardError::ParseError("ENDIF without IF".into()));
                }
                exec_stack.pop();
            }
            opcode::OP_TOALTSTACK => {
                if executing {
                    let data = stack.pop()?;
                    stack.push_alt(&data)?;
                }
            }
            opcode::OP_FROMALTSTACK => {
                if executing {
                    let data = stack.pop_alt()?;
                    stack.push(&data)?;
                }
            }
            opcode::OP_2DROP => {
                if executing {
                    if stack.len() < 2 {
                        return Err(OrchardError::ParseError("stack underflow".into()));
                    }
                    stack.pop()?;
                    stack.pop()?;
                }
            }
            opcode::OP_2DUP => {
                if executing {
                    if stack.len() < 2 {
                        return Err(OrchardError::ParseError("stack underflow".into()));
                    }
                    let first = stack.peek(1)?;
                    let second = stack.peek(0)?;
                    stack.push(&first)?;
                    stack.push(&second)?;
                }
            }
            opcode::OP_3DUP => {
                if executing {
                    if stack.len() < 3 {
                        return Err(OrchardError::ParseError("stack underflow".into()));
                    }
                    let first = stack.peek(2)?;
                    let second = stack.peek(1)?;
                    let third = stack.peek(0)?;
                    stack.push(&first)?;
                    stack.push(&second)?;
                    stack.push(&third)?;
                }
            }
            opcode::OP_2OVER => {
                if executing {
                    if stack.len() < 4 {
                        return Err(OrchardError::ParseError("stack underflow".into()));
                    }
                    let first = stack.peek(3)?;
                    let second = stack.peek(2)?;
                    stack.push(&first)?;
                    stack.push(&second)?;
                }
            }
            opcode::OP_2ROT => {
                if executing {
                    if stack.len() < 6 {
                        return Err(OrchardError::ParseError("stack underflow".into()));
                    }
                    let x6 = stack.pop()?;
                    let x5 = stack.pop()?;
                    let x4 = stack.pop()?;
                    let x3 = stack.pop()?;
                    let x2 = stack.pop()?;
                    let x1 = stack.pop()?;
                    stack.push(&x3)?;
                    stack.push(&x4)?;
                    stack.push(&x5)?;
                    stack.push(&x6)?;
                    stack.push(&x1)?;
                    stack.push(&x2)?;
                }
            }
            opcode::OP_2SWAP => {
                if executing {
                    if stack.len() < 4 {
                        return Err(OrchardError::ParseError("stack underflow".into()));
                    }
                    let x4 = stack.pop()?;
                    let x3 = stack.pop()?;
                    let x2 = stack.pop()?;
                    let x1 = stack.pop()?;
                    stack.push(&x3)?;
                    stack.push(&x4)?;
                    stack.push(&x1)?;
                    stack.push(&x2)?;
                }
            }
            opcode::OP_IFDUP => {
                if executing {
                    let value = stack.peek(0)?;
                    if is_true(&value) {
                        stack.push(&value)?;
                    }
                }
            }
            opcode::OP_DEPTH => {
                if executing {
                    let depth = number_to_bytes(stack.len() as i64);
                    stack.push(&depth)?;
                }
            }
            opcode::OP_DROP => {
                if executing {
                    stack.pop()?;
                }
            }
            opcode::OP_DUP => {
                if executing {
                    let top = stack.top()?;
                    stack.push(&top)?;
                }
            }
            opcode::OP_NIP => {
                if executing {
                    if stack.len() < 2 {
                        return Err(OrchardError::ParseError("stack underflow".into()));
                    }
                    let x2 = stack.pop()?;
                    stack.pop()?;
                    stack.push(&x2)?;
                }
            }
            opcode::OP_OVER => {
                if executing {
                    if stack.len() < 2 {
                        return Err(OrchardError::ParseError("stack underflow".into()));
                    }
                    let value = stack.peek(1)?;
                    stack.push(&value)?;
                }
            }
            opcode::OP_PICK => {
                if executing {
                    let index_bytes = stack.pop()?;
                    let index = parse_num(&index_bytes)?;
                    if index < 0 {
                        return Err(OrchardError::ParseError("invalid stack index".into()));
                    }
                    let value = stack.peek(index as usize)?;
                    stack.push(&value)?;
                }
            }
            opcode::OP_ROLL => {
                if executing {
                    let index_bytes = stack.pop()?;
                    let index = parse_num(&index_bytes)?;
                    if index < 0 {
                        return Err(OrchardError::ParseError("invalid stack index".into()));
                    }
                    let value = stack.remove_from_top(index as usize)?;
                    stack.push(&value)?;
                }
            }
            opcode::OP_ROT => {
                if executing {
                    if stack.len() < 3 {
                        return Err(OrchardError::ParseError("stack underflow".into()));
                    }
                    let x3 = stack.pop()?;
                    let x2 = stack.pop()?;
                    let x1 = stack.pop()?;
                    stack.push(&x2)?;
                    stack.push(&x3)?;
                    stack.push(&x1)?;
                }
            }
            opcode::OP_SWAP => {
                if executing {
                    if stack.len() < 2 {
                        return Err(OrchardError::ParseError("stack underflow".into()));
                    }
                    let x2 = stack.pop()?;
                    let x1 = stack.pop()?;
                    stack.push(&x2)?;
                    stack.push(&x1)?;
                }
            }
            opcode::OP_TUCK => {
                if executing {
                    if stack.len() < 2 {
                        return Err(OrchardError::ParseError("stack underflow".into()));
                    }
                    let x2 = stack.pop()?;
                    let x1 = stack.pop()?;
                    stack.push(&x2)?;
                    stack.push(&x1)?;
                    stack.push(&x2)?;
                }
            }
            opcode::OP_SIZE => {
                if executing {
                    let value = stack.peek(0)?;
                    let size = number_to_bytes(value.len() as i64);
                    stack.push(&size)?;
                }
            }
            opcode::OP_HASH160 => {
                if executing {
                    let data = stack.pop()?;
                    let hash = hash160(&data);
                    stack.push(&hash)?;
                }
            }
            opcode::OP_RIPEMD160 => {
                if executing {
                    let data = stack.pop()?;
                    let hash = Ripemd160::digest(data);
                    let mut out = [0u8; 20];
                    out.copy_from_slice(&hash);
                    stack.push(&out)?;
                }
            }
            opcode::OP_SHA1 => {
                if executing {
                    return Err(OrchardError::ParseError("OP_SHA1 disabled".into()));
                }
            }
            opcode::OP_SHA256 => {
                if executing {
                    let data = stack.pop()?;
                    let hash = Sha256::digest(data);
                    let mut out = [0u8; 32];
                    out.copy_from_slice(&hash);
                    stack.push(&out)?;
                }
            }
            opcode::OP_HASH256 => {
                if executing {
                    let data = stack.pop()?;
                    let hash = hash256(&data);
                    stack.push(&hash)?;
                }
            }
            opcode::OP_EQUAL => {
                if executing {
                    op_equal(stack)?;
                }
            }
            opcode::OP_EQUALVERIFY => {
                if executing {
                    op_equal(stack)?;
                    op_verify(stack)?;
                }
            }
            opcode::OP_VERIFY => {
                if executing {
                    op_verify(stack)?;
                }
            }
            opcode::OP_1ADD => {
                if executing {
                    let value = parse_num(&stack.pop()?)?;
                    let result = number_to_bytes(value + 1);
                    stack.push(&result)?;
                }
            }
            opcode::OP_1SUB => {
                if executing {
                    let value = parse_num(&stack.pop()?)?;
                    let result = number_to_bytes(value - 1);
                    stack.push(&result)?;
                }
            }
            opcode::OP_NEGATE => {
                if executing {
                    let value = parse_num(&stack.pop()?)?;
                    let result = number_to_bytes(-value);
                    stack.push(&result)?;
                }
            }
            opcode::OP_ABS => {
                if executing {
                    let value = parse_num(&stack.pop()?)?;
                    let result = number_to_bytes(value.abs());
                    stack.push(&result)?;
                }
            }
            opcode::OP_NOT => {
                if executing {
                    let value = stack.pop()?;
                    stack.push(if is_true(&value) { &[] } else { &[0x01] })?;
                }
            }
            opcode::OP_0NOTEQUAL => {
                if executing {
                    let value = stack.pop()?;
                    stack.push(if is_true(&value) { &[0x01] } else { &[] })?;
                }
            }
            opcode::OP_ADD => {
                if executing {
                    let b = parse_num(&stack.pop()?)?;
                    let a = parse_num(&stack.pop()?)?;
                    let result = number_to_bytes(a + b);
                    stack.push(&result)?;
                }
            }
            opcode::OP_SUB => {
                if executing {
                    let b = parse_num(&stack.pop()?)?;
                    let a = parse_num(&stack.pop()?)?;
                    let result = number_to_bytes(a - b);
                    stack.push(&result)?;
                }
            }
            opcode::OP_BOOLAND => {
                if executing {
                    let b = stack.pop()?;
                    let a = stack.pop()?;
                    stack.push(if is_true(&a) && is_true(&b) { &[0x01] } else { &[] })?;
                }
            }
            opcode::OP_BOOLOR => {
                if executing {
                    let b = stack.pop()?;
                    let a = stack.pop()?;
                    stack.push(if is_true(&a) || is_true(&b) { &[0x01] } else { &[] })?;
                }
            }
            opcode::OP_NUMEQUAL => {
                if executing {
                    let b = parse_num(&stack.pop()?)?;
                    let a = parse_num(&stack.pop()?)?;
                    stack.push(if a == b { &[0x01] } else { &[] })?;
                }
            }
            opcode::OP_NUMEQUALVERIFY => {
                if executing {
                    let b = parse_num(&stack.pop()?)?;
                    let a = parse_num(&stack.pop()?)?;
                    stack.push(if a == b { &[0x01] } else { &[] })?;
                    op_verify(stack)?;
                }
            }
            opcode::OP_NUMNOTEQUAL => {
                if executing {
                    let b = parse_num(&stack.pop()?)?;
                    let a = parse_num(&stack.pop()?)?;
                    stack.push(if a != b { &[0x01] } else { &[] })?;
                }
            }
            opcode::OP_LESSTHAN => {
                if executing {
                    let b = parse_num(&stack.pop()?)?;
                    let a = parse_num(&stack.pop()?)?;
                    stack.push(if a < b { &[0x01] } else { &[] })?;
                }
            }
            opcode::OP_GREATERTHAN => {
                if executing {
                    let b = parse_num(&stack.pop()?)?;
                    let a = parse_num(&stack.pop()?)?;
                    stack.push(if a > b { &[0x01] } else { &[] })?;
                }
            }
            opcode::OP_LESSTHANOREQUAL => {
                if executing {
                    let b = parse_num(&stack.pop()?)?;
                    let a = parse_num(&stack.pop()?)?;
                    stack.push(if a <= b { &[0x01] } else { &[] })?;
                }
            }
            opcode::OP_GREATERTHANOREQUAL => {
                if executing {
                    let b = parse_num(&stack.pop()?)?;
                    let a = parse_num(&stack.pop()?)?;
                    stack.push(if a >= b { &[0x01] } else { &[] })?;
                }
            }
            opcode::OP_MIN => {
                if executing {
                    let b = parse_num(&stack.pop()?)?;
                    let a = parse_num(&stack.pop()?)?;
                    stack.push(&number_to_bytes(core::cmp::min(a, b)))?;
                }
            }
            opcode::OP_MAX => {
                if executing {
                    let b = parse_num(&stack.pop()?)?;
                    let a = parse_num(&stack.pop()?)?;
                    stack.push(&number_to_bytes(core::cmp::max(a, b)))?;
                }
            }
            opcode::OP_WITHIN => {
                if executing {
                    let max = parse_num(&stack.pop()?)?;
                    let min = parse_num(&stack.pop()?)?;
                    let x = parse_num(&stack.pop()?)?;
                    stack.push(if x >= min && x < max { &[0x01] } else { &[] })?;
                }
            }
            opcode::OP_CHECKSIG => {
                if executing {
                    if stack.len() < 2 {
                        return Err(OrchardError::ParseError("stack underflow".into()));
                    }
                    let pubkey = stack.pop()?;
                    let sig_with_hashtype = stack.pop()?;
                    if sig_with_hashtype.len() < 2 {
                        stack.push(&[])?;
                        continue;
                    }
                    let signature = &sig_with_hashtype[..sig_with_hashtype.len() - 1];
                    let result = verify_ecdsa(&pubkey, signature, sighash);
                    stack.push(if result.is_ok() { &[0x01] } else { &[] })?;
                }
            }
            opcode::OP_CHECKSIGVERIFY => {
                if executing {
                    if stack.len() < 2 {
                        return Err(OrchardError::ParseError("stack underflow".into()));
                    }
                    let pubkey = stack.pop()?;
                    let sig_with_hashtype = stack.pop()?;
                    if sig_with_hashtype.len() < 2 {
                        stack.push(&[])?;
                        op_verify(stack)?;
                        continue;
                    }
                    let signature = &sig_with_hashtype[..sig_with_hashtype.len() - 1];
                    let result = verify_ecdsa(&pubkey, signature, sighash);
                    stack.push(if result.is_ok() { &[0x01] } else { &[] })?;
                    op_verify(stack)?;
                }
            }
            opcode::OP_CHECKMULTISIG => {
                if executing {
                    execute_checkmultisig(stack, sighash)?;
                }
            }
            opcode::OP_CHECKMULTISIGVERIFY => {
                if executing {
                    execute_checkmultisig(stack, sighash)?;
                    op_verify(stack)?;
                }
            }
            opcode::OP_CHECKLOCKTIMEVERIFY => {
                if executing {
                    op_checklocktimeverify(stack, ctx)?;
                }
            }
            opcode::OP_CHECKSEQUENCEVERIFY => {
                if executing {
                    op_checksequenceverify(stack, ctx)?;
                }
            }
            _ => {
                if executing {
                    return Err(OrchardError::ParseError(alloc::format!(
                        "unsupported opcode: 0x{:02x}",
                        opcode
                    )));
                }
            }
        }
    }

    if !exec_stack.is_empty() {
        return Err(OrchardError::ParseError("unterminated conditional".into()));
    }

    Ok(())
}

pub fn verify_p2sh(
    script_sig: &[u8],
    script_pubkey: &[u8],
    sighash: &[u8; 32],
) -> Result<()> {
    verify_p2sh_with_context(script_sig, script_pubkey, sighash, None)
}

pub fn verify_p2sh_with_context(
    script_sig: &[u8],
    script_pubkey: &[u8],
    sighash: &[u8; 32],
    ctx: Option<&ScriptContext>,
) -> Result<()> {
    let redeem_script = extract_redeem_script(script_sig)?;
    let unlocking_data = extract_unlocking_data(script_sig)?;

    if redeem_script.len() > 520 {
        return Err(OrchardError::ParseError("redeem script too large".into()));
    }

    let computed_hash = hash160(&redeem_script);
    let expected_hash = extract_p2sh_hash(script_pubkey)?;
    if computed_hash != expected_hash {
        return Err(OrchardError::ParseError("script hash mismatch".into()));
    }

    let mut stack = ScriptStack::new();
    for data in &unlocking_data {
        stack.push(data)?;
    }

    execute_script_with_context(&redeem_script, &mut stack, sighash, ctx)?;

    let result = stack.pop()?;
    if !is_true(&result) {
        return Err(OrchardError::ParseError("script execution failed".into()));
    }

    Ok(())
}

fn parse_pubkey(pubkey: &[u8]) -> Result<PublicKey> {
    PublicKey::parse_slice(pubkey, None)
        .map_err(|_| OrchardError::ParseError("invalid pubkey encoding".into()))
}

fn hash160(data: &[u8]) -> [u8; 20] {
    let sha = Sha256::digest(data);
    let ripemd = Ripemd160::digest(sha);
    let mut out = [0u8; 20];
    out.copy_from_slice(&ripemd);
    out
}

fn read_push(script: &[u8], cursor: &mut usize) -> Result<Vec<u8>> {
    if *cursor >= script.len() {
        return Err(OrchardError::ParseError("script push out of bounds".into()));
    }

    let opcode = script[*cursor];
    *cursor += 1;

    let len = match opcode {
        opcode::OP_0 => 0,
        0x01..=0x4b => opcode as usize,
        opcode::OP_PUSHDATA1 => {
            if *cursor >= script.len() {
                return Err(OrchardError::ParseError("pushdata1 out of bounds".into()));
            }
            let len = script[*cursor] as usize;
            *cursor += 1;
            len
        }
        opcode::OP_PUSHDATA2 => {
            if *cursor + 2 > script.len() {
                return Err(OrchardError::ParseError("pushdata2 out of bounds".into()));
            }
            let len = u16::from_le_bytes([script[*cursor], script[*cursor + 1]]) as usize;
            *cursor += 2;
            len
        }
        opcode::OP_PUSHDATA4 => {
            if *cursor + 4 > script.len() {
                return Err(OrchardError::ParseError("pushdata4 out of bounds".into()));
            }
            let len = u32::from_le_bytes([
                script[*cursor],
                script[*cursor + 1],
                script[*cursor + 2],
                script[*cursor + 3],
            ]) as usize;
            *cursor += 4;
            len
        }
        _ => {
            return Err(OrchardError::ParseError(
                "unsupported push opcode in scriptSig".into(),
            ));
        }
    };

    if *cursor + len > script.len() {
        return Err(OrchardError::ParseError("pushdata exceeds script length".into()));
    }
    let data = script[*cursor..*cursor + len].to_vec();
    *cursor += len;
    Ok(data)
}

fn parse_der_signature(sig: &[u8]) -> Result<[u8; 64]> {
    if sig.len() < 8 || sig[0] != 0x30 {
        return Err(OrchardError::ParseError("invalid DER signature".into()));
    }

    let total_len = sig[1] as usize;
    if total_len + 2 != sig.len() {
        return Err(OrchardError::ParseError("invalid DER length".into()));
    }

    if sig[2] != 0x02 {
        return Err(OrchardError::ParseError("invalid DER integer for r".into()));
    }
    let r_len = sig[3] as usize;
    let r_start = 4;
    let r_end = r_start + r_len;
    if r_len == 0 || r_len > 33 || r_end > sig.len() {
        return Err(OrchardError::ParseError("invalid r length".into()));
    }

    if sig[r_end] != 0x02 {
        return Err(OrchardError::ParseError("invalid DER integer for s".into()));
    }
    let s_len = sig[r_end + 1] as usize;
    let s_start = r_end + 2;
    let s_end = s_start + s_len;
    if s_len == 0 || s_len > 33 || s_end != sig.len() {
        return Err(OrchardError::ParseError("invalid s length".into()));
    }

    let r_bytes = strip_der_padding(&sig[r_start..r_end])?;
    let s_bytes = strip_der_padding(&sig[s_start..s_end])?;

    let mut out = [0u8; 64];
    out[32-r_bytes.len()..32].copy_from_slice(r_bytes);
    out[64-s_bytes.len()..64].copy_from_slice(s_bytes);
    Ok(out)
}

fn strip_der_padding(bytes: &[u8]) -> Result<&[u8]> {
    if bytes.len() == 33 {
        if bytes[0] != 0x00 {
            return Err(OrchardError::ParseError("invalid DER padding".into()));
        }
        return Ok(&bytes[1..]);
    }
    if bytes.len() > 32 {
        return Err(OrchardError::ParseError("invalid DER integer size".into()));
    }
    Ok(bytes)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_p2pkh_detection() {
        // Valid P2PKH script
        let script = [
            0x76, 0xa9, 0x14, // OP_DUP OP_HASH160 PUSH(20)
            0x89, 0xab, 0xcd, 0xef, 0x01, 0x23, 0x45, 0x67,
            0x89, 0xab, 0xcd, 0xef, 0x01, 0x23, 0x45, 0x67,
            0x89, 0xab, 0xcd, 0xef,
            0x88, 0xac, // OP_EQUALVERIFY OP_CHECKSIG
        ];
        assert!(is_p2pkh(&script));

        // Extract hash
        let hash = extract_p2pkh_hash(&script).unwrap();
        assert_eq!(&hash[..4], &[0x89, 0xab, 0xcd, 0xef]);
    }
}
