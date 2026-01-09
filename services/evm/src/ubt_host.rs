//! UBT host fetch wrappers for EVM state reads.

extern crate alloc;

use alloc::vec;
use alloc::vec::Vec;
use alloc::format;
use primitive_types::{H160, H256, U256};

// ===== FFI Declarations =====

#[polkavm_derive::polkavm_import]
extern "C" {
    /// Unified UBT fetch function (two-step API) (index 255)
    ///
    /// Parameters (via PVM registers):
    /// - r7: fetch_type (0=Balance, 1=Nonce, 2=Code, 3=CodeHash, 4=Storage)
    /// - r8: address_ptr (20 bytes)
    /// - r9: key_ptr (32 bytes, only for Storage, else null)
    /// - r10: output_ptr (buffer for result)
    /// - r11: output_max_len (0 = query size, >0 = fetch data)
    /// - r12: tx_index (transaction index within work package)
    ///
    /// Returns (via r7):
    /// - Step 1 (output_max_len=0): actual size needed
    /// - Step 2 (output_max_len>0): bytes written (0 = insufficient buffer or not found)
    #[polkavm_import(index = 255)]
    pub fn host_fetch_ubt(
        fetch_type: u64,
        address_ptr: u64,
        key_ptr: u64,
        output_ptr: u64,
        output_max_len: u64,
        tx_index: u64,
    ) -> u64;
}

// ===== Fetch Type Constants =====

pub const FETCH_BALANCE: u8 = 0;
pub const FETCH_NONCE: u8 = 1;
pub const FETCH_CODE: u8 = 2;
pub const FETCH_CODE_HASH: u8 = 3;
pub const FETCH_STORAGE: u8 = 4;
const MAX_CODE_SIZE: u64 = 0x6000; // 24 KB per EVM spec (EIP-170)

// ===== Safe Wrappers =====

/// Fetch balance via host function.
pub fn fetch_balance_ubt(address: H160, tx_index: u32) -> U256 {
    let mut output = [0u8; 32];

    unsafe {
        let written = host_fetch_ubt(
            FETCH_BALANCE as u64,
            address.as_ptr() as u64,
            0,
            output.as_mut_ptr() as u64,
            32,
            tx_index as u64,
        );

        if written != 32 {
            return U256::zero();
        }
    }

    U256::from_big_endian(&output)
}

/// Fetch nonce via host function.
pub fn fetch_nonce_ubt(address: H160, tx_index: u32) -> U256 {
    let mut output = [0u8; 32];

    unsafe {
        let written = host_fetch_ubt(
            FETCH_NONCE as u64,
            address.as_ptr() as u64,
            0,
            output.as_mut_ptr() as u64,
            32,
            tx_index as u64,
        );

        if written != 32 {
            return U256::zero();
        }
    }

    U256::from_big_endian(&output)
}

/// Fetch code via host function (two-step pattern).
pub fn fetch_code_ubt(address: H160, tx_index: u32) -> Vec<u8> {
    let code_size = unsafe {
        host_fetch_ubt(
            FETCH_CODE as u64,
            address.as_ptr() as u64,
            0,
            0,
            0,
            tx_index as u64,
        )
    };

    if code_size == 0 {
        return Vec::new();
    }

    if code_size > MAX_CODE_SIZE {
        utils::functions::log_error(&format!(
            "fetch_code_ubt: code size {} exceeds EVM limit {}",
            code_size, MAX_CODE_SIZE
        ));
        return Vec::new();
    }

    let code_size_usize: usize = match code_size.try_into() {
        Ok(size) => size,
        Err(_) => {
            utils::functions::log_error(&format!(
                "fetch_code_ubt: code size {} does not fit in usize",
                code_size
            ));
            return Vec::new();
        }
    };

    let mut code = vec![0u8; code_size_usize];
    let written = unsafe {
        host_fetch_ubt(
            FETCH_CODE as u64,
            address.as_ptr() as u64,
            0,
            code.as_mut_ptr() as u64,
            code_size,
            tx_index as u64,
        )
    };

    if written != code_size {
        return Vec::new();
    }

    code
}

/// Fetch code hash via host function.
pub fn fetch_code_hash_ubt(address: H160, tx_index: u32) -> H256 {
    let mut output = [0u8; 32];

    unsafe {
        let written = host_fetch_ubt(
            FETCH_CODE_HASH as u64,
            address.as_ptr() as u64,
            0,
            output.as_mut_ptr() as u64,
            32,
            tx_index as u64,
        );

        if written != 32 {
            return H256::zero();
        }
    }

    H256::from(output)
}

/// Fetch storage value via host function and return presence flag.
pub fn fetch_storage_ubt_with_presence(address: H160, key: H256, tx_index: u32) -> (H256, bool) {
    let mut output = [0u8; 32];

    unsafe {
        let written = host_fetch_ubt(
            FETCH_STORAGE as u64,
            address.as_ptr() as u64,
            key.as_ptr() as u64,
            output.as_mut_ptr() as u64,
            32,
            tx_index as u64,
        );

        if written != 32 {
            return (H256::zero(), false);
        }
    }

    (H256::from(output), true)
}
