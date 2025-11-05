//! Transaction decoding for EVM
//!
//! This module handles decoding of Ethereum transactions from RLP format.
//! Supports:
//! - Legacy transactions (pre-EIP-155 and EIP-155)
//! - EIP-2930 (Access List transactions)
//! - EIP-1559 (Dynamic fee transactions)
//!
//! Also includes signature recovery and sender address derivation.

use alloc::{format, vec::Vec};
use primitive_types::{H160, H256, U256};
use utils::hash_functions::keccak256;
use utils::functions::{log_error, log_info};

/// Decoded transaction arguments from host function call
pub struct DecodedTransactArgs {
    pub caller: H160,
    pub gas_limit: U256,
    pub gas_price: U256,
    pub value: U256,
    pub call_create: DecodedCallCreate,
}

/// Call or Create variant for transaction
pub enum DecodedCallCreate {
    Call { address: H160, data: Vec<u8> },
    Create { init_code: Vec<u8> },
}

/// Decode transaction arguments from raw memory buffer containing RLP-encoded Ethereum transaction
/// Supports legacy (EIP-155), EIP-2930 (access list), and EIP-1559 (dynamic fee) transactions
pub fn decode_transact_args(
    args_addr: u64,
    args_len: u64,
    _arg_header_len: usize,
) -> Option<DecodedTransactArgs> {
    if args_len == 0 {
        return None;
    }

    let rlp_bytes = unsafe { core::slice::from_raw_parts(args_addr as *const u8, args_len as usize) };

    // Check for typed transaction (EIP-2718)
    // Typed transactions start with a single byte < 0xc0 (not an RLP list marker)
    if !rlp_bytes.is_empty() && rlp_bytes[0] < 0xc0 {
        let tx_type = rlp_bytes[0];
        let payload = &rlp_bytes[1..];

        match tx_type {
            0x01 => decode_eip2930_transaction(payload),
            0x02 => decode_eip1559_transaction(payload),
            _ => {
                log_error(&format!("❌ Unsupported transaction type: 0x{:02x}", tx_type));
                None
            }
        }
    } else {
        // Legacy transaction
        decode_legacy_transaction(rlp_bytes)
    }
}

/// Decode legacy EIP-155 transaction
fn decode_legacy_transaction(rlp_bytes: &[u8]) -> Option<DecodedTransactArgs> {
    let rlp = rlp::Rlp::new(rlp_bytes);
    if !rlp.is_list() || rlp.item_count().ok()? != 9 {
        log_error("❌ Invalid legacy transaction RLP structure");
        return None;
    }

    // Parse EIP-155 transaction fields: [nonce, gasPrice, gasLimit, to, value, data, v, r, s]
    let nonce = rlp.at(0).ok()?.as_val::<U256>().ok()?;
    let gas_price = rlp.at(1).ok()?.as_val::<U256>().ok()?;
    let gas_limit = rlp.at(2).ok()?.as_val::<U256>().ok()?;
    let to_bytes = rlp.at(3).ok()?.data().ok()?;
    let value = rlp.at(4).ok()?.as_val::<U256>().ok()?;
    let data = rlp.at(5).ok()?.data().ok()?.to_vec();
    let v = rlp.at(6).ok()?.as_val::<u64>().ok()?;
    let r = rlp.at(7).ok()?.as_val::<U256>().ok()?;
    let s = rlp.at(8).ok()?.as_val::<U256>().ok()?;

    // Recover sender address from signature
    let caller = recover_sender_legacy(nonce, gas_price, gas_limit, to_bytes, value, &data, v, r, s)?;

    // Determine if this is a contract creation (empty 'to' field) or call
    let call_create = if to_bytes.is_empty() {
        DecodedCallCreate::Create { init_code: data }
    } else {
        let address = H160::from_slice(to_bytes);
        DecodedCallCreate::Call { address, data }
    };

    log_info(&format!(
        "✅ Decoded legacy tx: from={:?}, gas_limit={}, gas_price={}, value={}",
        caller, gas_limit, gas_price, value
    ));

    Some(DecodedTransactArgs {
        caller,
        gas_limit,
        gas_price,
        value,
        call_create,
    })
}

/// Decode EIP-2930 access list transaction (type 0x01)
fn decode_eip2930_transaction(payload: &[u8]) -> Option<DecodedTransactArgs> {
    let rlp = rlp::Rlp::new(payload);
    if !rlp.is_list() || rlp.item_count().ok()? != 11 {
        log_error("❌ Invalid EIP-2930 transaction structure");
        return None;
    }

    // EIP-2930 fields: [chainId, nonce, gasPrice, gasLimit, to, value, data, accessList, v, r, s]
    let chain_id = rlp.at(0).ok()?.as_val::<u64>().ok()?;
    let nonce = rlp.at(1).ok()?.as_val::<U256>().ok()?;
    let gas_price = rlp.at(2).ok()?.as_val::<U256>().ok()?;
    let gas_limit = rlp.at(3).ok()?.as_val::<U256>().ok()?;
    let to_bytes = rlp.at(4).ok()?.data().ok()?;
    let value = rlp.at(5).ok()?.as_val::<U256>().ok()?;
    let data = rlp.at(6).ok()?.data().ok()?.to_vec();
    // accessList at index 7 (ignored for now)
    let v = rlp.at(8).ok()?.as_val::<u64>().ok()?;
    let r = rlp.at(9).ok()?.as_val::<U256>().ok()?;
    let s = rlp.at(10).ok()?.as_val::<U256>().ok()?;

    // Recover sender for EIP-2930
    let caller = recover_sender_eip2930(chain_id, nonce, gas_price, gas_limit, to_bytes, value, &data, payload, v, r, s)?;

    let call_create = if to_bytes.is_empty() {
        DecodedCallCreate::Create { init_code: data }
    } else {
        let address = H160::from_slice(to_bytes);
        DecodedCallCreate::Call { address, data }
    };

    log_info(&format!(
        "✅ Decoded EIP-2930 tx: from={:?}, gas_limit={}, gas_price={}, chain_id={}",
        caller, gas_limit, gas_price, chain_id
    ));

    Some(DecodedTransactArgs {
        caller,
        gas_limit,
        gas_price,
        value,
        call_create,
    })
}

/// Decode EIP-1559 dynamic fee transaction (type 0x02)
fn decode_eip1559_transaction(payload: &[u8]) -> Option<DecodedTransactArgs> {
    let rlp = rlp::Rlp::new(payload);
    if !rlp.is_list() || rlp.item_count().ok()? != 12 {
        log_error("❌ Invalid EIP-1559 transaction structure");
        return None;
    }

    // EIP-1559 fields: [chainId, nonce, maxPriorityFeePerGas, maxFeePerGas, gasLimit, to, value, data, accessList, v, r, s]
    let chain_id = rlp.at(0).ok()?.as_val::<u64>().ok()?;
    let nonce = rlp.at(1).ok()?.as_val::<U256>().ok()?;
    let max_priority_fee = rlp.at(2).ok()?.as_val::<U256>().ok()?;
    let max_fee_per_gas = rlp.at(3).ok()?.as_val::<U256>().ok()?;
    let gas_limit = rlp.at(4).ok()?.as_val::<U256>().ok()?;
    let to_bytes = rlp.at(5).ok()?.data().ok()?;
    let value = rlp.at(6).ok()?.as_val::<U256>().ok()?;
    let data = rlp.at(7).ok()?.data().ok()?.to_vec();
    // accessList at index 8 (ignored for now)
    let v = rlp.at(9).ok()?.as_val::<u64>().ok()?;
    let r = rlp.at(10).ok()?.as_val::<U256>().ok()?;
    let s = rlp.at(11).ok()?.as_val::<U256>().ok()?;

    // Recover sender for EIP-1559
    let caller = recover_sender_eip1559(chain_id, nonce, max_priority_fee, max_fee_per_gas, gas_limit, to_bytes, value, &data, payload, v, r, s)?;

    let call_create = if to_bytes.is_empty() {
        DecodedCallCreate::Create { init_code: data }
    } else {
        let address = H160::from_slice(to_bytes);
        DecodedCallCreate::Call { address, data }
    };

    log_info(&format!(
        "✅ Decoded EIP-1559 tx: from={:?}, gas_limit={}, max_fee={}, chain_id={}",
        caller, gas_limit, max_fee_per_gas, chain_id
    ));

    // Use max_fee_per_gas as gas_price for fee calculation
    Some(DecodedTransactArgs {
        caller,
        gas_limit,
        gas_price: max_fee_per_gas,
        value,
        call_create,
    })
}

/// Recover sender address from legacy EIP-155 signed transaction
fn recover_sender_legacy(
    nonce: U256,
    gas_price: U256,
    gas_limit: U256,
    to: &[u8],
    value: U256,
    data: &[u8],
    v: u64,
    r: U256,
    s: U256,
) -> Option<H160> {
    // Extract chain ID from v (EIP-155: v = CHAIN_ID * 2 + 35 + recovery_id)
    let chain_id = if v >= 35 {
        Some((v - 35) / 2)
    } else {
        None
    };

    let recovery_id = if let Some(_chain_id) = chain_id {
        ((v - 35) % 2) as u8
    } else {
        (v - 27) as u8
    };

    // Build signing hash (hash of unsigned transaction with chain_id for EIP-155)
    let mut stream = rlp::RlpStream::new();
    stream.begin_list(if chain_id.is_some() { 9 } else { 6 });
    stream.append(&nonce);
    stream.append(&gas_price);
    stream.append(&gas_limit);
    if to.is_empty() {
        stream.append_empty_data();
    } else {
        stream.append(&to);
    }
    stream.append(&value);
    stream.append(&data);

    if let Some(chain_id) = chain_id {
        stream.append(&chain_id);
        stream.append(&0u8);
        stream.append(&0u8);
    }

    let signing_hash = keccak256(stream.as_raw());

    // Prepare signature bytes for recovery
    let mut sig_bytes = [0u8; 64];
    r.to_big_endian(&mut sig_bytes[0..32]);
    s.to_big_endian(&mut sig_bytes[32..64]);

    // Recover public key using secp256k1
    let signature = libsecp256k1::Signature::parse_standard(&sig_bytes).ok()?;
    let recovery_id = libsecp256k1::RecoveryId::parse(recovery_id).ok()?;
    let message = libsecp256k1::Message::parse(&signing_hash.0);

    let public_key = libsecp256k1::recover(&message, &signature, &recovery_id).ok()?;

    // Derive Ethereum address from public key
    // Address = last 20 bytes of keccak256(public_key[1..65])
    let pub_key_bytes = public_key.serialize();
    let pub_key_hash = keccak256(&pub_key_bytes[1..65]); // Skip first byte (0x04 prefix)
    let address = H160::from_slice(&pub_key_hash.0[12..32]);

    log_info(&format!("🔐 Recovered sender: {:?} (v={}, chain_id={:?})", address, v, chain_id));

    Some(address)
}

/// Recover sender address from EIP-2930 signed transaction
#[allow(clippy::too_many_arguments)]
fn recover_sender_eip2930(
    chain_id: u64,
    nonce: U256,
    gas_price: U256,
    gas_limit: U256,
    to: &[u8],
    value: U256,
    data: &[u8],
    full_payload: &[u8],
    v: u64,
    r: U256,
    s: U256,
) -> Option<H160> {
    // For EIP-2930, v is the raw recovery ID (0 or 1)
    let recovery_id = v as u8;

    // Build signing hash for EIP-2930 by reconstructing unsigned transaction
    // Hash = keccak256(0x01 || rlp([chainId, nonce, gasPrice, gasLimit, to, value, data, accessList]))
    // Must exclude v, r, s from the hash
    let rlp = rlp::Rlp::new(full_payload);
    let access_list = rlp.at(7).ok()?.as_raw();

    let mut stream = rlp::RlpStream::new_list(8);
    stream.append(&chain_id);
    stream.append(&nonce);
    stream.append(&gas_price);
    stream.append(&gas_limit);
    if to.is_empty() {
        stream.append_empty_data();
    } else {
        stream.append(&to);
    }
    stream.append(&value);
    stream.append(&data);
    stream.append_raw(access_list, 1);

    let mut hash_input = alloc::vec![0x01];
    hash_input.extend_from_slice(stream.as_raw());
    let signing_hash = keccak256(&hash_input);

    // Recover sender
    recover_from_signature(r, s, recovery_id, &signing_hash)
}

/// Recover sender address from EIP-1559 signed transaction
#[allow(clippy::too_many_arguments)]
fn recover_sender_eip1559(
    chain_id: u64,
    nonce: U256,
    max_priority_fee_per_gas: U256,
    max_fee_per_gas: U256,
    gas_limit: U256,
    to: &[u8],
    value: U256,
    data: &[u8],
    full_payload: &[u8],
    v: u64,
    r: U256,
    s: U256,
) -> Option<H160> {
    // For EIP-1559, v is the raw recovery ID (0 or 1)
    let recovery_id = v as u8;

    // Build signing hash for EIP-1559 by reconstructing unsigned transaction
    // Hash = keccak256(0x02 || rlp([chainId, nonce, maxPriorityFeePerGas, maxFeePerGas, gasLimit, to, value, data, accessList]))
    // Must exclude v, r, s from the hash
    let rlp = rlp::Rlp::new(full_payload);
    let access_list = rlp.at(8).ok()?.as_raw();

    let mut stream = rlp::RlpStream::new_list(9);
    stream.append(&chain_id);
    stream.append(&nonce);
    stream.append(&max_priority_fee_per_gas);
    stream.append(&max_fee_per_gas);
    stream.append(&gas_limit);
    if to.is_empty() {
        stream.append_empty_data();
    } else {
        stream.append(&to);
    }
    stream.append(&value);
    stream.append(&data);
    stream.append_raw(access_list, 1);

    let mut hash_input = alloc::vec![0x02];
    hash_input.extend_from_slice(stream.as_raw());
    let signing_hash = keccak256(&hash_input);

    // Recover sender
    recover_from_signature(r, s, recovery_id, &signing_hash)
}

/// Common signature recovery logic
fn recover_from_signature(r: U256, s: U256, recovery_id: u8, signing_hash: &H256) -> Option<H160> {
    // Prepare signature bytes
    let mut sig_bytes = [0u8; 64];
    r.to_big_endian(&mut sig_bytes[0..32]);
    s.to_big_endian(&mut sig_bytes[32..64]);

    // Recover public key
    let signature = libsecp256k1::Signature::parse_standard(&sig_bytes).ok()?;
    let recovery_id = libsecp256k1::RecoveryId::parse(recovery_id).ok()?;
    let message = libsecp256k1::Message::parse(&signing_hash.0);

    let public_key = libsecp256k1::recover(&message, &signature, &recovery_id).ok()?;

    // Derive Ethereum address
    let pub_key_bytes = public_key.serialize();
    let pub_key_hash = keccak256(&pub_key_bytes[1..65]);
    let address = H160::from_slice(&pub_key_hash.0[12..32]);

    log_info(&format!("🔐 Recovered sender: {:?}", address));

    Some(address)
}

/// Decode call arguments for unsigned transactions (EstimateGas/Call RPCs)
/// Format: caller(20) + target(20) + gas_limit(32) + gas_price(32) + value(32) + call_kind(4) + data_len(8) + data
pub fn decode_call_args(
    args_addr: u64,
    args_len: u64,
) -> Option<DecodedTransactArgs> {
    const MIN_LEN: usize = 148; // 20+20+32+32+32+4+8
    if args_len < MIN_LEN as u64 {
        log_error(&format!("❌ decode_call_args: buffer too short ({} bytes, expected at least {})", args_len, MIN_LEN));
        return None;
    }

    let buffer = unsafe { core::slice::from_raw_parts(args_addr as *const u8, args_len as usize) };

    let mut offset = 0;

    // Parse caller (20 bytes)
    let caller = H160::from_slice(&buffer[offset..offset + 20]);
    offset += 20;

    // Parse target (20 bytes)
    let target_bytes = &buffer[offset..offset + 20];
    offset += 20;

    // Parse gas_limit (32 bytes, big-endian)
    let gas_limit = U256::from_big_endian(&buffer[offset..offset + 32]);
    offset += 32;

    // Parse gas_price (32 bytes, big-endian)
    let gas_price = U256::from_big_endian(&buffer[offset..offset + 32]);
    offset += 32;

    // Parse value (32 bytes, big-endian)
    let value = U256::from_big_endian(&buffer[offset..offset + 32]);
    offset += 32;

    // Parse call_kind (4 bytes, little-endian)
    let call_kind = u32::from_le_bytes([buffer[offset], buffer[offset + 1], buffer[offset + 2], buffer[offset + 3]]);
    offset += 4;

    // Parse data_len (8 bytes, little-endian)
    let data_len = u64::from_le_bytes([
        buffer[offset],
        buffer[offset + 1],
        buffer[offset + 2],
        buffer[offset + 3],
        buffer[offset + 4],
        buffer[offset + 5],
        buffer[offset + 6],
        buffer[offset + 7],
    ]);
    offset += 8;

    // Parse data
    let data = if data_len > 0 && offset + data_len as usize <= buffer.len() {
        buffer[offset..offset + data_len as usize].to_vec()
    } else if data_len == 0 {
        Vec::new()
    } else {
        log_error(&format!("❌ decode_call_args: data length {} exceeds buffer size {}", data_len, buffer.len() - offset));
        return None;
    };

    // Determine if this is a contract creation or call
    let call_create = if call_kind == 1 {
        // CREATE
        DecodedCallCreate::Create { init_code: data }
    } else {
        // CALL (call_kind == 0)
        let address = H160::from_slice(target_bytes);
        DecodedCallCreate::Call { address, data }
    };

    log_info(&format!(
        "✅ Decoded call args: from={:?}, gas_limit={}, gas_price={}, value={}, call_kind={}",
        caller, gas_limit, gas_price, value, call_kind
    ));

    Some(DecodedTransactArgs {
        caller,
        gas_limit,
        gas_price,
        value,
        call_create,
    })
}
