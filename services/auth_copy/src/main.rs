#![no_std]
#![no_main]

extern crate alloc;
use alloc::format;

const SIZE0: usize = 0x10000;
// allocate memory for stack
use polkavm_derive::min_stack_size;
min_stack_size!(SIZE0);

const SIZE1: usize = 0x10000;
// allocate memory for heap
use simplealloc::SimpleAlloc;
#[global_allocator]
static ALLOCATOR: SimpleAlloc<SIZE1> = SimpleAlloc::new();

use utils::constants::FIRST_READABLE_ADDRESS;
use utils::functions::call_log;
use utils::hash_functions::keccak256;
use utils::host_functions::fetch;

// Import secp256k1 for signature verification
use libsecp256k1::{recover, Message, RecoveryId, Signature};

/// Authorizer entry point: validates ECDSA signature over work package hash
///
/// **ABI Contract**:
/// - **Symbol**: `is_authorized` (exported PVM entry point)
/// - **Arguments**: `(start_address: u64, length: u64)` - pointer to JAM codec core value
/// - **Returns**: `(FIRST_READABLE_ADDRESS, 0)` on success, `(FIRST_READABLE_ADDRESS, 1)` on failure
///
/// **Input Format**: JAM codec u16 core index (LE, exactly 2 bytes for cores 0..340)
/// - JAM runtime passes: `types.Encode(uint16(core))` → `E_l(core, 2)` → 2 bytes LE
/// - Authorizer strictly validates: `length == 2` and parses `u16::from_le_bytes([lo, hi])`
///
/// **Verification steps**:
/// 1. Parse core from JAM codec input argument (exactly 2 bytes LE)
/// 2. Fetch 74-byte configuration blob (builder[20] || declared_value[16] || salt[32] || core[2] || cycle[4])
/// 3. Fetch 65-byte authorization token (ECDSA signature: r[32] || s[32] || v[1])
/// 4. Fetch 32-byte work package hash (Blake2b of encoded WP with AuthorizationToken=empty)
/// 5. Recover signer using secp256k1 with Ethereum's keccak256 address derivation
/// 6. Verify recovered address matches builder address in config blob
/// 7. Verify core in config blob matches core argument (defense-in-depth)
#[polkavm_derive::polkavm_export]
extern "C" fn is_authorized(start_address: u64, length: u64) -> (u64, u64) {
    // Allocate 64KB heap for signature verification (libsecp256k1 uses stack-based computation)
    polkavm_derive::sbrk(64 * 1024);

    // Parse core from input argument: JAM codec u16 (LE)
    // JAM passes types.Encode(uint16(c)) which produces E_l(c, 2) = 2 bytes LE
    if length != 2 {
        call_log(2, None, &format!("is_authorized: expected 2-byte SCALE u16 input, got {} bytes", length));
        return (FIRST_READABLE_ADDRESS as u64, 1);
    }

    let input_slice = unsafe { core::slice::from_raw_parts(start_address as *const u8, 2) };
    let core = u16::from_le_bytes([input_slice[0], input_slice[1]]);

    // Fetch constants from AUTH.md Section 8
    const WORK_PACKAGE_HASH: u64 = 0;  // fetch datatype: work package hash
    const CONFIG_BLOB: u64 = 8;         // fetch datatype: configuration blob
    const AUTH_TOKEN: u64 = 9;          // fetch datatype: authorization token

    // Allocate buffers
    let mut config_blob = [0u8; 74];   // builder[20] || declared_value[16] || salt[32] || core[2] || cycle[4]
    let mut auth_token = [0u8; 65];    // ECDSA signature: r[32] || s[32] || v[1] (v in {27,28})
    let mut wp_hash = [0u8; 32];       // Blake2b(Encode(WorkPackage) with AuthorizationToken=empty)

    // Fetch configuration blob (exactly 74 bytes required)
    let config_blob_ptr = config_blob.as_mut_ptr() as u64;
    let config_len = unsafe { fetch(config_blob_ptr, 0, 74, CONFIG_BLOB, 0, 0) };
    if config_len != 74 {
        call_log(2, None, &format!("is_authorized: config blob must be exactly 74 bytes, got {}", config_len));
        return (FIRST_READABLE_ADDRESS as u64, 1);
    }

    // Fetch authorization token (exactly 65-byte ECDSA signature)
    let auth_token_ptr = auth_token.as_mut_ptr() as u64;
    let auth_len = unsafe { fetch(auth_token_ptr, 0, 65, AUTH_TOKEN, 0, 0) };
    if auth_len != 65 {
        call_log(2, None, &format!("is_authorized: auth token must be exactly 65 bytes, got {}", auth_len));
        return (FIRST_READABLE_ADDRESS as u64, 1);
    }

    // Fetch work package hash (exactly 32 bytes)
    let wp_hash_ptr = wp_hash.as_mut_ptr() as u64;
    let hash_len = unsafe { fetch(wp_hash_ptr, 0, 32, WORK_PACKAGE_HASH, 0, 0) };
    if hash_len != 32 {
        call_log(2, None, &format!("is_authorized: wp hash must be exactly 32 bytes, got {}", hash_len));
        return (FIRST_READABLE_ADDRESS as u64, 1);
    }

    // Parse ECDSA signature: r[32] || s[32] || v[1]
    let mut sig_bytes = [0u8; 64];
    sig_bytes[..32].copy_from_slice(&auth_token[0..32]);  // r
    sig_bytes[32..64].copy_from_slice(&auth_token[32..64]); // s
    let v_raw = auth_token[64];  // v (Ethereum convention: 27 or 28)

    // Normalize v: Ethereum uses 27/28, secp256k1 expects 0/1
    // Strictly enforce Ethereum convention: only accept v ∈ {0, 1, 27, 28}
    let recovery_id = if v_raw == 27 {
        0
    } else if v_raw == 28 {
        1
    } else if v_raw == 0 {
        0  // Allow raw 0 for compatibility
    } else if v_raw == 1 {
        1  // Allow raw 1 for compatibility
    } else {
        call_log(2, None, &format!("is_authorized: invalid v value {} (must be 0,1,27, or 28)", v_raw));
        return (FIRST_READABLE_ADDRESS as u64, 1);
    };
    // Note: recovery_id is guaranteed to be 0 or 1 at this point (no need for additional check)

    // Recover signer from signature
    let signature = match Signature::parse_standard(&sig_bytes) {
        Ok(sig) => sig,
        Err(_) => {
            call_log(2, None, &format!("is_authorized: invalid signature format"));
            return (FIRST_READABLE_ADDRESS as u64, 1);
        }
    };

    let recid = match RecoveryId::parse(recovery_id) {
        Ok(id) => id,
        Err(_) => {
            call_log(2, None, &format!("is_authorized: RecoveryId::parse failed for {}", recovery_id));
            return (FIRST_READABLE_ADDRESS as u64, 1);
        }
    };

    let message = Message::parse(&wp_hash);

    let recovered_pubkey = match recover(&message, &signature, &recid) {
        Ok(pubkey) => pubkey,
        Err(_) => {
            call_log(2, None, &format!("is_authorized: signature recovery failed"));
            return (FIRST_READABLE_ADDRESS as u64, 1);
        }
    };

    // Compute Ethereum address from recovered public key
    // Address = last 20 bytes of keccak256(pubkey[1..65])
    // pubkey[0] = 0x04 (uncompressed point marker), skip it
    let pubkey_bytes = recovered_pubkey.serialize();
    let pubkey_hash = keccak256(&pubkey_bytes[1..65]);
    let recovered_address = &pubkey_hash.0[12..32]; // Last 20 bytes (.0 for H256 inner array)

    // Extract builder address from config blob (first 20 bytes)
    let expected_builder = &config_blob[0..20];

    // Verify recovered address matches builder in config blob
    if recovered_address != expected_builder {
        call_log(
            2,
            None,
            &format!(
                "is_authorized: signature mismatch - recovered={:02x?}, expected={:02x?}",
                recovered_address, expected_builder
            ),
        );
        return (FIRST_READABLE_ADDRESS as u64, 1);
    }

    // Defense-in-depth: verify core and cycle fields in config blob
    // Core field (bytes 68-69, BE u16)
    let config_core = u16::from_be_bytes([config_blob[68], config_blob[69]]);
    if config_core != core {
        call_log(
            2,
            None,
            &format!(
                "is_authorized: core mismatch - config has {}, validating {}",
                config_core, core
            ),
        );
        return (FIRST_READABLE_ADDRESS as u64, 1);
    }

    // Cycle field validation (bytes 70-73, BE u32)
    // Note: We don't have expected cycle in input, but we validate format consistency
    let _config_cycle = u32::from_be_bytes([
        config_blob[70], config_blob[71], config_blob[72], config_blob[73]
    ]);
    // TODO: If guarantor provides expected cycle, add validation:
    // if config_cycle != expected_cycle { return error; }

    call_log(
        2,
        None,
        &format!(
            "is_authorized: SUCCESS - builder {:02x?} authorized for core {}",
            expected_builder, core
        ),
    );

    // Success: signature valid, builder authorized
    (FIRST_READABLE_ADDRESS as u64, 0)
}

#[polkavm_derive::polkavm_export]
extern "C" fn refine(_start_address: u64, _length: u64) -> (u64, u64) {
    (FIRST_READABLE_ADDRESS as u64, 0)
}

#[polkavm_derive::polkavm_export]
extern "C" fn accumulate(_start_address: u64, _length: u64) -> (u64, u64) {
    (FIRST_READABLE_ADDRESS as u64, 0)
}
