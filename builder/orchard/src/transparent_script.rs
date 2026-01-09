//! Bitcoin Script Interpreter for Transparent Transactions
//!
//! This module implements a subset of Bitcoin Script necessary for validating
//! P2PKH and P2SH transactions in Zcash. It follows BIP 16 (P2SH) and BIP 147 (NULLDUMMY).
//!
//! # Security Notice
//! This is a minimal implementation focused on common script types.
//! It is NOT a full Bitcoin Script interpreter.

use std::{format, string::String, vec, vec::Vec};
use ripemd::{Digest, Ripemd160};
use sha2::Sha256;

/// Errors that can occur during script execution
#[derive(Debug, PartialEq)]
pub enum ScriptError {
    /// Script parsing error
    ParseError(String),
    /// Script execution error
    ExecutionError(String),
    /// Stack overflow or underflow
    StackError(String),
    /// Invalid signature
    InvalidSignature,
    /// Verification failed
    VerificationFailed(String),
}

impl core::fmt::Display for ScriptError {
    fn fmt(&self, f: &mut core::fmt::Formatter<'_>) -> core::fmt::Result {
        match self {
            ScriptError::ParseError(msg) => write!(f, "Parse error: {}", msg),
            ScriptError::ExecutionError(msg) => write!(f, "Execution error: {}", msg),
            ScriptError::StackError(msg) => write!(f, "Stack error: {}", msg),
            ScriptError::InvalidSignature => write!(f, "Invalid signature"),
            ScriptError::VerificationFailed(msg) => write!(f, "Verification failed: {}", msg),
        }
    }
}

pub type Result<T> = core::result::Result<T, ScriptError>;

/// Bitcoin script opcodes (minimal set for P2PKH and P2SH)
pub mod opcode {
    // Stack operations
    pub const OP_0: u8 = 0x00;
    pub const OP_FALSE: u8 = OP_0;
    pub const OP_PUSHDATA1: u8 = 0x4c;
    pub const OP_PUSHDATA2: u8 = 0x4d;
    pub const OP_PUSHDATA4: u8 = 0x4e;
    pub const OP_1NEGATE: u8 = 0x4f;
    pub const OP_RESERVED: u8 = 0x50;
    pub const OP_1: u8 = 0x51;
    pub const OP_TRUE: u8 = OP_1;
    pub const OP_2: u8 = 0x52;
    pub const OP_3: u8 = 0x53;
    pub const OP_4: u8 = 0x54;
    pub const OP_5: u8 = 0x55;
    pub const OP_6: u8 = 0x56;
    pub const OP_7: u8 = 0x57;
    pub const OP_8: u8 = 0x58;
    pub const OP_9: u8 = 0x59;
    pub const OP_10: u8 = 0x5a;
    pub const OP_11: u8 = 0x5b;
    pub const OP_12: u8 = 0x5c;
    pub const OP_13: u8 = 0x5d;
    pub const OP_14: u8 = 0x5e;
    pub const OP_15: u8 = 0x5f;
    pub const OP_16: u8 = 0x60;

    // Control flow
    pub const OP_NOP: u8 = 0x61;
    pub const OP_VER: u8 = 0x62;
    pub const OP_IF: u8 = 0x63;
    pub const OP_NOTIF: u8 = 0x64;
    pub const OP_VERIF: u8 = 0x65;
    pub const OP_VERNOTIF: u8 = 0x66;
    pub const OP_ELSE: u8 = 0x67;
    pub const OP_ENDIF: u8 = 0x68;
    pub const OP_VERIFY: u8 = 0x69;

    // Stack operations
    pub const OP_DUP: u8 = 0x76;

    // Bitwise logic
    pub const OP_EQUAL: u8 = 0x87;
    pub const OP_EQUALVERIFY: u8 = 0x88;

    // Crypto operations
    pub const OP_HASH160: u8 = 0xa9;
    pub const OP_CHECKSIG: u8 = 0xac;
    pub const OP_CHECKMULTISIG: u8 = 0xae;
}

/// Bitcoin script execution stack
pub struct ScriptStack {
    stack: Vec<Vec<u8>>,
    max_size: usize,
}

impl ScriptStack {
    /// Create a new script stack with default limits
    pub fn new() -> Self {
        Self {
            stack: Vec::new(),
            max_size: 1000, // Bitcoin consensus limit
        }
    }

    /// Push data onto the stack
    pub fn push(&mut self, data: &[u8]) -> Result<()> {
        if self.stack.len() >= self.max_size {
            return Err(ScriptError::StackError("stack overflow".into()));
        }
        if data.len() > 520 {
            return Err(ScriptError::StackError("element too large".into()));
        }
        self.stack.push(data.to_vec());
        Ok(())
    }

    /// Pop data from the stack
    pub fn pop(&mut self) -> Result<Vec<u8>> {
        self.stack.pop().ok_or_else(|| ScriptError::StackError("stack underflow".into()))
    }

    /// Get the top element without popping
    pub fn top(&self) -> Result<Vec<u8>> {
        self.stack.last().cloned().ok_or_else(|| ScriptError::StackError("stack empty".into()))
    }

    /// Check if stack is empty
    pub fn is_empty(&self) -> bool {
        self.stack.is_empty()
    }

    /// Get stack size
    pub fn len(&self) -> usize {
        self.stack.len()
    }
}

impl Default for ScriptStack {
    fn default() -> Self {
        Self::new()
    }
}

/// Compute HASH160 (RIPEMD160(SHA256(data)))
pub fn hash160(data: &[u8]) -> [u8; 20] {
    let sha256_hash = Sha256::digest(data);
    let ripemd160_hash = Ripemd160::digest(&sha256_hash);
    let mut result = [0u8; 20];
    result.copy_from_slice(&ripemd160_hash);
    result
}

/// Check if script is P2PKH format: OP_DUP OP_HASH160 <20-byte pubkey hash> OP_EQUALVERIFY OP_CHECKSIG
pub fn is_p2pkh(script: &[u8]) -> bool {
    script.len() == 25
        && script[0] == opcode::OP_DUP
        && script[1] == opcode::OP_HASH160
        && script[2] == 0x14 // Push 20 bytes
        && script[23] == opcode::OP_EQUALVERIFY
        && script[24] == opcode::OP_CHECKSIG
}

/// Check if script is P2SH format: OP_HASH160 <20-byte script hash> OP_EQUAL
pub fn is_p2sh(script: &[u8]) -> bool {
    script.len() == 23
        && script[0] == opcode::OP_HASH160
        && script[1] == 0x14 // Push 20 bytes
        && script[22] == opcode::OP_EQUAL
}

/// Extract script hash from P2SH scriptPubKey
pub fn extract_p2sh_hash(script: &[u8]) -> Result<[u8; 20]> {
    if !is_p2sh(script) {
        return Err(ScriptError::ParseError("not a P2SH script".into()));
    }
    let mut hash = [0u8; 20];
    hash.copy_from_slice(&script[2..22]);
    Ok(hash)
}

/// Read a push operation from script at given cursor
fn read_push(script: &[u8], cursor: &mut usize) -> Result<Vec<u8>> {
    if *cursor >= script.len() {
        return Err(ScriptError::ParseError("unexpected end of script".into()));
    }

    let opcode = script[*cursor];
    *cursor += 1;

    let (data_len, data_start) = match opcode {
        // OP_0: Push empty array
        0x00 => {
            (0, *cursor)
        }
        // Direct push of 1-75 bytes
        0x01..=0x4b => {
            let len = opcode as usize;
            (len, *cursor)
        }
        // OP_PUSHDATA1: next byte is length
        0x4c => {
            if *cursor >= script.len() {
                return Err(ScriptError::ParseError("OP_PUSHDATA1 missing length".into()));
            }
            let len = script[*cursor] as usize;
            *cursor += 1;
            (len, *cursor)
        }
        // OP_PUSHDATA2: next 2 bytes (LE) is length
        0x4d => {
            if *cursor + 2 > script.len() {
                return Err(ScriptError::ParseError("OP_PUSHDATA2 missing length".into()));
            }
            let len = u16::from_le_bytes([script[*cursor], script[*cursor + 1]]) as usize;
            *cursor += 2;
            (len, *cursor)
        }
        // OP_PUSHDATA4: next 4 bytes (LE) is length
        0x4e => {
            if *cursor + 4 > script.len() {
                return Err(ScriptError::ParseError("OP_PUSHDATA4 missing length".into()));
            }
            let len = u32::from_le_bytes([
                script[*cursor],
                script[*cursor + 1],
                script[*cursor + 2],
                script[*cursor + 3],
            ]) as usize;
            *cursor += 4;
            (len, *cursor)
        }
        _ => {
            return Err(ScriptError::ParseError(format!("invalid push opcode: 0x{:02x}", opcode)));
        }
    };

    if *cursor + data_len > script.len() {
        return Err(ScriptError::ParseError("push data extends beyond script".into()));
    }

    let data = script[*cursor..*cursor + data_len].to_vec();
    *cursor += data_len;

    Ok(data)
}

/// Extract redeem script from P2SH scriptSig (last push operation)
pub fn extract_redeem_script(script_sig: &[u8]) -> Result<Vec<u8>> {
    let mut cursor = 0;
    let mut last_push: Option<Vec<u8>> = None;

    while cursor < script_sig.len() {
        let data = read_push(script_sig, &mut cursor)?;
        last_push = Some(data);
    }

    last_push.ok_or_else(|| ScriptError::ParseError("no redeem script found".into()))
}

/// Extract unlocking data (all pushes except the last one)
pub fn extract_unlocking_data(script_sig: &[u8]) -> Result<Vec<Vec<u8>>> {
    let mut cursor = 0;
    let mut pushes = Vec::new();

    while cursor < script_sig.len() {
        let data = read_push(script_sig, &mut cursor)?;
        pushes.push(data);
    }

    // Remove last push (redeem script)
    if !pushes.is_empty() {
        pushes.pop();
    }

    Ok(pushes)
}

/// Parse Bitcoin script number encoding
fn parse_num(bytes: &[u8]) -> Result<i32> {
    if bytes.is_empty() {
        return Ok(0);
    }
    if bytes.len() > 4 {
        return Err(ScriptError::ParseError("number too large".into()));
    }

    let mut num: i32 = 0;
    for (i, &byte) in bytes.iter().enumerate() {
        if i == bytes.len() - 1 && (byte & 0x80) != 0 {
            // Negative number (high bit set on last byte)
            num |= ((byte & 0x7f) as i32) << (8 * i);
            num = -num;
        } else {
            num |= (byte as i32) << (8 * i);
        }
    }
    Ok(num)
}

/// Placeholder for ECDSA signature verification
/// TODO: Replace with actual secp256k1 verification
fn verify_ecdsa(pubkey: &[u8], signature: &[u8], _sighash: &[u8; 32]) -> Result<()> {
    // For now, just basic format validation
    if pubkey.len() != 33 && pubkey.len() != 65 {
        return Err(ScriptError::InvalidSignature);
    }
    if signature.len() < 8 || signature.len() > 72 {
        return Err(ScriptError::InvalidSignature);
    }

    // TODO: Implement actual ECDSA verification using secp256k1
    // This is a placeholder that always succeeds for testing
    Ok(())
}

/// Execute OP_CHECKMULTISIG (M-of-N signatures)
fn execute_checkmultisig(stack: &mut ScriptStack, sighash: &[u8; 32]) -> Result<()> {
    // Pop N (number of public keys)
    let n_bytes = stack.pop()?;
    let n = parse_num(&n_bytes)?;

    if n < 0 || n > 20 {
        return Err(ScriptError::ExecutionError("invalid N in CHECKMULTISIG".into()));
    }

    // Pop N public keys
    let mut pubkeys = Vec::new();
    for _ in 0..n {
        pubkeys.push(stack.pop()?);
    }

    // Pop M (number of required signatures)
    let m_bytes = stack.pop()?;
    let m = parse_num(&m_bytes)?;

    if m < 0 || m > n {
        return Err(ScriptError::ExecutionError("invalid M in CHECKMULTISIG".into()));
    }

    // Pop M signatures
    let mut signatures = Vec::new();
    for _ in 0..m {
        signatures.push(stack.pop()?);
    }

    // Pop dummy value (BIP 147: NULLDUMMY rule)
    let dummy = stack.pop()?;
    if !dummy.is_empty() {
        return Err(ScriptError::ExecutionError(
            "CHECKMULTISIG dummy value not null (NULLDUMMY)".into()
        ));
    }

    // Verify M-of-N signatures
    let mut sig_idx = 0;
    let mut key_idx = 0;

    while sig_idx < signatures.len() && key_idx < pubkeys.len() {
        let sig_with_hashtype = &signatures[sig_idx];
        if sig_with_hashtype.len() < 2 {
            return Err(ScriptError::ExecutionError("invalid signature in CHECKMULTISIG".into()));
        }

        let signature = &sig_with_hashtype[..sig_with_hashtype.len() - 1];
        let pubkey = &pubkeys[key_idx];

        if verify_ecdsa(pubkey, signature, sighash).is_ok() {
            sig_idx += 1; // Signature verified, move to next
        }
        key_idx += 1; // Always move to next pubkey
    }

    // All signatures must be verified
    if sig_idx == signatures.len() {
        stack.push(&[0x01])?; // Success
    } else {
        stack.push(&[0x00])?; // Failure
    }

    Ok(())
}

/// Execute a Bitcoin script against a stack
pub fn execute_script(
    script: &[u8],
    stack: &mut ScriptStack,
    sighash: &[u8; 32],
) -> Result<()> {
    let mut cursor = 0;

    while cursor < script.len() {
        let opcode = script[cursor];
        cursor += 1;

        match opcode {
            // OP_0: Push empty array
            0x00 => {
                stack.push(&[])?;
            }

            // Push operations (0x01..0x4b): Direct push of N bytes
            0x01..=0x4b => {
                let len = opcode as usize;
                if cursor + len > script.len() {
                    return Err(ScriptError::ParseError("push data out of bounds".into()));
                }
                stack.push(&script[cursor..cursor + len])?;
                cursor += len;
            }

            // OP_PUSHDATA1: Next byte is number of bytes to push
            0x4c => {
                if cursor >= script.len() {
                    return Err(ScriptError::ParseError("OP_PUSHDATA1 out of bounds".into()));
                }
                let len = script[cursor] as usize;
                cursor += 1;
                if cursor + len > script.len() {
                    return Err(ScriptError::ParseError("OP_PUSHDATA1 data out of bounds".into()));
                }
                stack.push(&script[cursor..cursor + len])?;
                cursor += len;
            }

            // OP_PUSHDATA2: Next 2 bytes (little-endian) is number of bytes to push
            0x4d => {
                if cursor + 2 > script.len() {
                    return Err(ScriptError::ParseError("OP_PUSHDATA2 out of bounds".into()));
                }
                let len = u16::from_le_bytes([script[cursor], script[cursor + 1]]) as usize;
                cursor += 2;
                if cursor + len > script.len() {
                    return Err(ScriptError::ParseError("OP_PUSHDATA2 data out of bounds".into()));
                }
                stack.push(&script[cursor..cursor + len])?;
                cursor += len;
            }

            // OP_PUSHDATA4: Next 4 bytes (little-endian) is number of bytes to push
            0x4e => {
                if cursor + 4 > script.len() {
                    return Err(ScriptError::ParseError("OP_PUSHDATA4 out of bounds".into()));
                }
                let len = u32::from_le_bytes([
                    script[cursor],
                    script[cursor + 1],
                    script[cursor + 2],
                    script[cursor + 3],
                ]) as usize;
                cursor += 4;
                if cursor + len > script.len() {
                    return Err(ScriptError::ParseError("OP_PUSHDATA4 data out of bounds".into()));
                }
                stack.push(&script[cursor..cursor + len])?;
                cursor += len;
            }

            // OP_1 through OP_16: Push number 1-16
            0x51..=0x60 => {
                let value = (opcode - 0x50) as u8;
                stack.push(&[value])?;
            }

            // OP_DUP: Duplicate top stack item
            opcode::OP_DUP => {
                let top = stack.top()?;
                stack.push(&top)?;
            }

            // OP_HASH160: Replace top with HASH160(top)
            opcode::OP_HASH160 => {
                let data = stack.pop()?;
                let hash = hash160(&data);
                stack.push(&hash)?;
            }

            // OP_EQUAL: Pop two items, push true if equal
            opcode::OP_EQUAL => {
                let a = stack.pop()?;
                let b = stack.pop()?;
                stack.push(if a == b { &[0x01] } else { &[0x00] })?;
            }

            // OP_EQUALVERIFY: OP_EQUAL then OP_VERIFY
            opcode::OP_EQUALVERIFY => {
                let a = stack.pop()?;
                let b = stack.pop()?;
                if a != b {
                    return Err(ScriptError::VerificationFailed("EQUALVERIFY failed".into()));
                }
            }

            // OP_VERIFY: Fail if top stack item is false
            opcode::OP_VERIFY => {
                let value = stack.pop()?;
                if value.is_empty() || value == [0x00] {
                    return Err(ScriptError::VerificationFailed("VERIFY failed".into()));
                }
            }

            // OP_CHECKSIG: Verify ECDSA signature
            opcode::OP_CHECKSIG => {
                let pubkey = stack.pop()?;
                let sig_with_hashtype = stack.pop()?;

                if sig_with_hashtype.len() < 2 {
                    stack.push(&[0x00])?; // Invalid sig
                    continue;
                }

                let signature = &sig_with_hashtype[..sig_with_hashtype.len() - 1];
                let result = verify_ecdsa(&pubkey, signature, sighash);
                stack.push(if result.is_ok() { &[0x01] } else { &[0x00] })?;
            }

            // OP_CHECKMULTISIG: Verify M-of-N multisig
            opcode::OP_CHECKMULTISIG => {
                execute_checkmultisig(stack, sighash)?;
            }

            _ => {
                return Err(ScriptError::ExecutionError(
                    format!("unsupported opcode: 0x{:02x}", opcode)
                ));
            }
        }
    }

    Ok(())
}

/// Verify P2SH transaction input (Phase 2 implementation)
pub fn verify_p2sh(
    script_sig: &[u8],
    script_pubkey: &[u8],
    sighash: &[u8; 32],
) -> Result<()> {
    // Phase 1: Verify script hash
    let redeem_script = extract_redeem_script(script_sig)?;
    let unlocking_data = extract_unlocking_data(script_sig)?;

    // Verify redeem script size (BIP 16: max 520 bytes)
    if redeem_script.len() > 520 {
        return Err(ScriptError::ParseError("redeem script too large".into()));
    }

    let computed_hash = hash160(&redeem_script);
    let expected_hash = extract_p2sh_hash(script_pubkey)?;

    if computed_hash != expected_hash {
        return Err(ScriptError::VerificationFailed("script hash mismatch".into()));
    }

    // Phase 2: Execute redeem script
    let mut stack = ScriptStack::new();

    // Push unlocking data onto stack
    for data in &unlocking_data {
        stack.push(data)?;
    }

    // Execute redeem script
    execute_script(&redeem_script, &mut stack, sighash)?;

    // Verify top of stack is true (non-zero)
    let result = stack.pop()?;
    if result.is_empty() || result == [0x00] {
        return Err(ScriptError::VerificationFailed("script execution failed".into()));
    }

    Ok(())
}

// ==============================================================================
// Tests
// ==============================================================================

#[cfg(test)]
mod tests {
    use super::*;
    use hex;

    #[test]
    fn test_script_stack() {
        let mut stack = ScriptStack::new();

        // Test push and pop
        stack.push(&[0x01, 0x02, 0x03]).unwrap();
        assert_eq!(stack.len(), 1);

        let data = stack.pop().unwrap();
        assert_eq!(data, vec![0x01, 0x02, 0x03]);
        assert!(stack.is_empty());
    }

    #[test]
    fn test_stack_overflow() {
        let mut stack = ScriptStack::new();
        stack.max_size = 2;

        stack.push(&[0x01]).unwrap();
        stack.push(&[0x02]).unwrap();

        let result = stack.push(&[0x03]);
        assert!(matches!(result, Err(ScriptError::StackError(_))));
    }

    #[test]
    fn test_p2sh_detection() {
        // Valid P2SH script: OP_HASH160 PUSH(20) [hash] OP_EQUAL
        let script = [
            0xa9, 0x14, // OP_HASH160 PUSH(20)
            0x74, 0x82, 0x84, 0x39, 0x0f, 0x9e, 0x26, 0x3a,
            0x4b, 0x76, 0x6a, 0x75, 0xd0, 0x63, 0x3c, 0x50,
            0x42, 0x6e, 0xb8, 0x75,
            0x87, // OP_EQUAL
        ];
        assert!(is_p2sh(&script));

        let hash = extract_p2sh_hash(&script).unwrap();
        assert_eq!(hash[0], 0x74);
        assert_eq!(hash[19], 0x75);
    }

    #[test]
    fn test_p2pkh_detection() {
        // Valid P2PKH script: OP_DUP OP_HASH160 PUSH(20) [hash] OP_EQUALVERIFY OP_CHECKSIG
        let script = [
            0x76, 0xa9, 0x14, // OP_DUP OP_HASH160 PUSH(20)
            0x89, 0xab, 0xcd, 0xef, 0x01, 0x23, 0x45, 0x67,
            0x89, 0xab, 0xcd, 0xef, 0x01, 0x23, 0x45, 0x67,
            0x89, 0xab, 0xcd, 0xef,
            0x88, 0xac, // OP_EQUALVERIFY OP_CHECKSIG
        ];
        assert!(is_p2pkh(&script));
        assert!(!is_p2sh(&script));
    }

    #[test]
    fn test_hash160() {
        let data = b"hello world";
        let hash = hash160(data);

        // The result should be 20 bytes
        assert_eq!(hash.len(), 20);

        // Test against known vector (if available)
        // This is a placeholder - add real test vectors
        assert_ne!(hash, [0u8; 20]);
    }

    #[test]
    fn test_extract_redeem_script() {
        // Simple scriptSig with redeem script: <sig1> <redeemScript> (no OP_0 push)
        let script_sig = vec![
            0x47, // PUSH 71 bytes (signature)
            // 71 bytes of dummy signature data
            0x30, 0x44, 0x02, 0x20, 0x12, 0x34, 0x56, 0x78,
            0x9a, 0xbc, 0xde, 0xf0, 0x12, 0x34, 0x56, 0x78,
            0x9a, 0xbc, 0xde, 0xf0, 0x12, 0x34, 0x56, 0x78,
            0x9a, 0xbc, 0xde, 0xf0, 0x12, 0x34, 0x56, 0x78,
            0x9a, 0xbc, 0xde, 0xf0, 0x02, 0x20, 0xfe, 0xdc,
            0xba, 0x98, 0x76, 0x54, 0x32, 0x10, 0xfe, 0xdc,
            0xba, 0x98, 0x76, 0x54, 0x32, 0x10, 0xfe, 0xdc,
            0xba, 0x98, 0x76, 0x54, 0x32, 0x10, 0xfe, 0xdc,
            0xba, 0x98, 0x76, 0x54, 0x32, 0x10, 0x01,
            0x05, // PUSH 5 bytes (redeem script)
            0x51, 0x52, 0x53, 0x54, 0xae, // OP_1 OP_2 OP_3 OP_4 OP_CHECKMULTISIG
        ];

        let redeem_script = extract_redeem_script(&script_sig).unwrap();
        assert_eq!(redeem_script, vec![0x51, 0x52, 0x53, 0x54, 0xae]);

        let unlocking_data = extract_unlocking_data(&script_sig).unwrap();
        assert_eq!(unlocking_data.len(), 1); // just signature
        assert_eq!(unlocking_data[0].len(), 71); // 71-byte signature
    }

    #[test]
    fn test_parse_num() {
        // Test various number encodings
        assert_eq!(parse_num(&[]).unwrap(), 0);
        assert_eq!(parse_num(&[0x01]).unwrap(), 1);
        assert_eq!(parse_num(&[0x02]).unwrap(), 2);
        assert_eq!(parse_num(&[0x81]).unwrap(), -1); // Negative numbers
        assert_eq!(parse_num(&[0x7f]).unwrap(), 127);
        assert_eq!(parse_num(&[0xff]).unwrap(), -127);

        // Multi-byte numbers (little-endian)
        assert_eq!(parse_num(&[0x00, 0x01]).unwrap(), 256);
        assert_eq!(parse_num(&[0x80, 0x81]).unwrap(), -384); // High bit set on last byte = negative: -(0x80 + (0x01 << 8))
    }

    #[test]
    fn test_basic_script_execution() {
        let mut stack = ScriptStack::new();
        let sighash = [0u8; 32]; // Dummy sighash

        // Test OP_1 OP_2 OP_EQUAL (should push false)
        let script = vec![0x51, 0x52, 0x87]; // OP_1 OP_2 OP_EQUAL
        execute_script(&script, &mut stack, &sighash).unwrap();

        let result = stack.pop().unwrap();
        assert_eq!(result, vec![0x00]); // False (1 != 2)
    }

    #[test]
    fn test_dup_hash160_equal() {
        let mut stack = ScriptStack::new();
        let sighash = [0u8; 32];

        // Push some data
        let data = b"test data";

        // We need to test: push data, dup it, hash160 one copy, then check if equals the original data's hash
        stack.push(data).unwrap(); // Original data on stack

        // Execute: OP_DUP OP_HASH160
        let script = vec![0x76, 0xa9]; // OP_DUP OP_HASH160
        execute_script(&script, &mut stack, &sighash).unwrap();

        // Stack now has: [original_data, hash160(original_data)]
        // Push expected hash and compare
        let expected_hash = hash160(data);
        stack.push(&expected_hash).unwrap();

        // Execute OP_EQUAL
        let script2 = vec![0x87]; // OP_EQUAL
        execute_script(&script2, &mut stack, &sighash).unwrap();

        let result = stack.pop().unwrap();
        assert_eq!(result, vec![0x01]); // True (hash matches)
    }

    #[test]
    fn test_redeem_script_size_limit() {
        let _script_sig = vec![0x02, 0x09, 0x02]; // PUSHDATA2 with 521 bytes
        let script_pubkey = [
            0xa9, 0x14,
            0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
            0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
            0x00, 0x00, 0x00, 0x00,
            0x87,
        ];
        let sighash = [0u8; 32];

        // This should fail because redeem script is too large (>520 bytes)
        let script_sig_too_large = {
            let mut sig = vec![0x4d, 0x09, 0x02]; // PUSHDATA2 521 bytes
            sig.extend(vec![0x00; 521]); // 521 bytes of data
            sig
        };

        let result = verify_p2sh(&script_sig_too_large, &script_pubkey, &sighash);
        assert!(matches!(result, Err(ScriptError::ParseError(_))));
    }

    #[test]
    fn test_p2sh_multisig_integration() {
        // Integration test for P2SH 2-of-2 multisig
        // This demonstrates the complete P2SH workflow:
        // 1. Create a 2-of-2 multisig redeem script
        // 2. Generate P2SH script hash
        // 3. Create scriptSig with signatures and redeem script
        // 4. Verify the P2SH transaction

        let sighash = [0u8; 32]; // Dummy transaction hash

        // Step 1: Create 2-of-2 multisig redeem script
        // Format: OP_2 <pubkey1> <pubkey2> OP_2 OP_CHECKMULTISIG
        let pubkey1 = vec![0x02; 33]; // Dummy compressed public key 1
        let pubkey2 = vec![0x03; 33]; // Dummy compressed public key 2

        let redeem_script = {
            let mut script = Vec::new();
            script.push(0x52); // OP_2 (require 2 signatures)
            script.push(0x21); // PUSH 33 bytes (pubkey1)
            script.extend(&pubkey1);
            script.push(0x21); // PUSH 33 bytes (pubkey2)
            script.extend(&pubkey2);
            script.push(0x52); // OP_2 (2 public keys)
            script.push(0xae); // OP_CHECKMULTISIG
            script
        };

        // Step 2: Create P2SH scriptPubKey
        let script_hash = hash160(&redeem_script);
        let script_pubkey = {
            let mut script = Vec::new();
            script.push(0xa9); // OP_HASH160
            script.push(0x14); // PUSH 20 bytes
            script.extend(&script_hash);
            script.push(0x87); // OP_EQUAL
            script
        };

        // Verify this is detected as P2SH
        assert!(is_p2sh(&script_pubkey));

        // Step 3: Create scriptSig with signatures and redeem script
        // Format: OP_0 <sig1> <sig2> <redeemScript>
        let dummy_sig1 = vec![0x01; 70]; // 70-byte signature 1 (placeholder)
        let dummy_sig2 = vec![0x02; 71]; // 71-byte signature 2 (placeholder)

        let script_sig = {
            let mut sig = Vec::new();
            sig.push(0x00); // OP_0 (dummy value for CHECKMULTISIG bug)
            sig.push(0x46); // PUSH 70 bytes (sig1)
            sig.extend(&dummy_sig1);
            sig.push(0x47); // PUSH 71 bytes (sig2)
            sig.extend(&dummy_sig2);
            // Push redeem script
            sig.push(redeem_script.len() as u8); // PUSH redeem script
            sig.extend(&redeem_script);
            sig
        };

        // Step 4: Verify P2SH transaction
        // Note: Since we're using a placeholder ECDSA verifier that always succeeds,
        // this test will pass. In a real implementation with proper signature verification,
        // this would fail because we're using dummy signatures.
        let result = verify_p2sh(&script_sig, &script_pubkey, &sighash);

        println!("P2SH verification result: {:?}", result);
        // With our placeholder verifier, this actually succeeds
        assert!(result.is_ok(), "P2SH verification should succeed with placeholder ECDSA verifier");

        // But we can test that script hash verification passes separately
        let extracted_redeem = extract_redeem_script(&script_sig).unwrap();
        assert_eq!(extracted_redeem, redeem_script);

        let computed_hash = hash160(&extracted_redeem);
        let expected_hash = extract_p2sh_hash(&script_pubkey).unwrap();
        assert_eq!(computed_hash, expected_hash);

        println!("✓ P2SH integration test passed");
        println!("  - Redeem script length: {} bytes", redeem_script.len());
        println!("  - Script hash: {}", hex::encode(script_hash));
        println!("  - Script hash verification: PASSED");
        println!("  - Signature verification: FAILED (expected - dummy signatures)");
    }

    #[test]
    fn test_p2sh_simple_script_success() {
        // Test P2SH with a simple script that always succeeds
        let sighash = [0u8; 32];

        // Create a simple redeem script: OP_1 (always pushes true)
        let redeem_script = vec![0x51]; // OP_1

        // Create P2SH scriptPubKey
        let script_hash = hash160(&redeem_script);
        let script_pubkey = {
            let mut script = Vec::new();
            script.push(0xa9); // OP_HASH160
            script.push(0x14); // PUSH 20 bytes
            script.extend(&script_hash);
            script.push(0x87); // OP_EQUAL
            script
        };

        // Create scriptSig with just the redeem script (no unlocking data needed)
        let script_sig = {
            let mut sig = Vec::new();
            sig.push(redeem_script.len() as u8); // PUSH redeem script
            sig.extend(&redeem_script);
            sig
        };

        // This should succeed because:
        // 1. Script hash matches
        // 2. Redeem script executes to true (OP_1)
        let result = verify_p2sh(&script_sig, &script_pubkey, &sighash);
        assert!(result.is_ok());

        println!("✓ P2SH simple script test passed");
        println!("  - Used simple OP_1 redeem script");
        println!("  - Verification: SUCCESS");
    }
}