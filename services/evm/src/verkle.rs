//! Verkle tree integration for EVM state reads
//!
//! This module implements Verkle tree integration for the JAM EVM service:
//! - Builder mode: All reads go through host_fetch_verkle (Go maintains verkleReadLog)
//! - Guarantor mode: Verifies Verkle witness, populates cache, uses cache-only reads
//!
//! See docs/VERKLE.md for complete architecture documentation.

use alloc::vec;
use alloc::vec::Vec;
use alloc::collections::BTreeMap;
use alloc::format;
use primitive_types::{H160, H256, U256};
use utils::hash_functions::keccak256;

// ===== FFI Declarations =====

#[polkavm_derive::polkavm_import]
extern "C" {
    /// Verify Verkle proof via host function (index 253)
    ///
    /// Parameters (via PVM registers):
    /// - r7: witness_ptr (pointer to serialized VerkleWitness)
    /// - r8: witness_len (length of serialized witness)
    ///
    /// Returns (via r7):
    /// - 1 if proof is valid
    /// - 0 if proof is invalid
    #[polkavm_import(index = 253)]
    pub fn host_verify_verkle_proof(witness_ptr: u64, witness_len: u64) -> u64;

    /// Unified Verkle fetch function (two-step API) (index 255)
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
    pub fn host_fetch_verkle(
        fetch_type: u64,
        address_ptr: u64,
        key_ptr: u64,
        output_ptr: u64,
        output_max_len: u64,
        tx_index: u64,
    ) -> u64;

    /// Compute Block Access List hash from Verkle witness (index 256)
    ///
    /// Parameters (via PVM registers):
    /// - r7: witness_ptr (pointer to serialized Verkle witness)
    /// - r8: witness_len (length of witness)
    /// - r9: output_ptr (32-byte buffer for Blake2b hash)
    ///
    /// Returns (via r7):
    /// - 1 if successful
    /// - 0 if failed
    #[polkavm_import(index = 256)]
    pub fn host_compute_bal_hash(witness_ptr: u64, witness_len: u64, output_ptr: u64) -> u64;
}

// ===== Fetch Type Constants =====

pub const FETCH_BALANCE: u8 = 0;
pub const FETCH_NONCE: u8 = 1;
pub const FETCH_CODE: u8 = 2;
pub const FETCH_CODE_HASH: u8 = 3;
pub const FETCH_STORAGE: u8 = 4;
const MAX_CODE_SIZE: u64 = 0x6000; // 24 KB per EVM spec (EIP-170)

// ===== Safe Wrappers =====

/// Fetch balance from Verkle tree via host function
pub fn fetch_balance_verkle(address: H160, tx_index: u32) -> U256 {
    let mut output = [0u8; 32];

    unsafe {
        let written = host_fetch_verkle(
            FETCH_BALANCE as u64,
            address.as_ptr() as u64,
            0, // No key for balance
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

/// Fetch nonce from Verkle tree via host function
pub fn fetch_nonce_verkle(address: H160, tx_index: u32) -> U256 {
    let mut output = [0u8; 32];

    unsafe {
        let written = host_fetch_verkle(
            FETCH_NONCE as u64,
            address.as_ptr() as u64,
            0, // No key for nonce
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

/// Fetch code from Verkle tree via host function (two-step pattern)
pub fn fetch_code_verkle(address: H160, tx_index: u32) -> Vec<u8> {
    // Step 1: Query size
    let code_size = unsafe {
        host_fetch_verkle(
            FETCH_CODE as u64,
            address.as_ptr() as u64,
            0, // No key for code
            0, // No buffer
            0, // Query size
            tx_index as u64,
        )
    };

    if code_size == 0 {
        return Vec::new();
    }

    if code_size > MAX_CODE_SIZE {
        utils::functions::log_error(&format!(
            "fetch_code_verkle: code size {} exceeds EVM limit {}",
            code_size, MAX_CODE_SIZE
        ));
        return Vec::new();
    }

    let code_size_usize: usize = match code_size.try_into() {
        Ok(size) => size,
        Err(_) => {
            utils::functions::log_error(&format!(
                "fetch_code_verkle: code size {} does not fit in usize",
                code_size
            ));
            return Vec::new();
        }
    };

    // Step 2: Fetch data
    let mut code = vec![0u8; code_size_usize];
    let written = unsafe {
        host_fetch_verkle(
            FETCH_CODE as u64,
            address.as_ptr() as u64,
            0,
            code.as_mut_ptr() as u64,
            code_size,
            tx_index as u64,
        )
    };

    if written != code_size {
        // Insufficient buffer or fetch failed
        return Vec::new();
    }

    code
}

/// Fetch code hash from Verkle tree via host function
pub fn fetch_code_hash_verkle(address: H160, tx_index: u32) -> H256 {
    let mut output = [0u8; 32];

    unsafe {
        let written = host_fetch_verkle(
            FETCH_CODE_HASH as u64,
            address.as_ptr() as u64,
            0, // No key for code hash
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

/// Fetch storage value from Verkle tree via host function and return presence flag.
/// Presence is true when the host returns a 32-byte value, false when the slot is absent.
pub fn fetch_storage_verkle_with_presence(address: H160, key: H256, tx_index: u32) -> (H256, bool) {
    let mut output = [0u8; 32];

    unsafe {
        let written = host_fetch_verkle(
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

// ===== Verkle Witness =====

/// Key type metadata to identify what each Verkle key represents
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[repr(u8)]
pub enum VerkleKeyType {
    BasicData = 0,      // Balance + Nonce + CodeSize
    CodeHash = 1,       // Code hash
    CodeChunk = 2,      // Code chunk
    Storage = 3,        // Storage slot
}

/// Metadata for a Verkle key that helps parse it back to semantic meaning
#[derive(Debug, Clone)]
pub struct VerkleKeyMetadata {
    pub key_type: VerkleKeyType,
    pub address: H160,
    pub extra: u64,               // chunk_id for CodeChunk
    pub storage_key: H256,        // Full 32-byte storage key (only for Storage type)
    pub tx_index: u32,            // Transaction index from witness (for BAL attribution)
}

/// VerkleWitnessEntry keeps a single key/value pair together with its metadata.
#[derive(Debug, Clone)]
pub struct VerkleWitnessEntry {
    pub key: [u8; 32],
    pub metadata: VerkleKeyMetadata,
    pub pre_value: [u8; 32],
    pub post_value: [u8; 32],
}

/// VerkleWitness represents a complete Verkle tree proof for both pre- and post-state sections.
/// Builder mode emits two multiproofs (reads + writes) and guarantor mode consumes both.
#[derive(Debug, Clone)]
pub struct VerkleWitness {
    /// Pre-state root (before execution)
    pub pre_state_root: H256,

    /// Entries proven in the pre-state witness (all reads)
    pub read_entries: Vec<VerkleWitnessEntry>,

    /// Serialized Verkle multiproof for the read_entries section
    pub pre_proof_data: Vec<u8>,

    /// Post-state root (after execution, provided by builder)
    pub post_state_root: H256,

    /// Entries proven in the post-state witness (all writes)
    pub write_entries: Vec<VerkleWitnessEntry>,

    /// Serialized Verkle multiproof for the write_entries section
    pub post_proof_data: Vec<u8>,
}

impl VerkleWitness {
    /// Deserialize VerkleWitness from bytes
    ///
    /// Format (all big-endian):
    /// - 32B pre_state_root
    /// - 4B read_key_count
    /// - read_key_count × 161B entries
    /// - 4B pre_proof_len + pre_proof_len bytes
    /// - 32B post_state_root
    /// - 4B write_key_count
    /// - write_key_count × 161B entries
    /// - 4B post_proof_len + post_proof_len bytes
    pub fn deserialize(data: &[u8]) -> Option<Self> {
        utils::functions::log_info(&format!(
            "VerkleWitness::deserialize: data_len={}",
            data.len()
        ));
        if data.len() < 72 {
            utils::functions::log_error(&format!(
                "VerkleWitness::deserialize: data too small: {} < 72",
                data.len()
            ));
            return None;
        }

        let mut offset = 0;

        // Read pre_state_root
        let mut pre_state_root = [0u8; 32];
        pre_state_root.copy_from_slice(&data[offset..offset + 32]);
        offset += 32;

        // Read num_keys
        if data.len() < offset + 4 {
            return None;
        }
        let read_key_count = u32::from_be_bytes([
            data[offset],
            data[offset + 1],
            data[offset + 2],
            data[offset + 3],
        ]) as usize;
        offset += 4;

        let mut read_entries = Vec::with_capacity(read_key_count);
        for _ in 0..read_key_count {
            if data.len() < offset + 161 {
                return None;
            }
            let mut key = [0u8; 32];
            key.copy_from_slice(&data[offset..offset + 32]);
            offset += 32;

            let key_type = match data[offset] {
                0 => VerkleKeyType::BasicData,
                1 => VerkleKeyType::CodeHash,
                2 => VerkleKeyType::CodeChunk,
                3 => VerkleKeyType::Storage,
                _ => return None,
            };
            offset += 1;

            let mut address = [0u8; 20];
            address.copy_from_slice(&data[offset..offset + 20]);
            offset += 20;

            let extra = u64::from_be_bytes([
                data[offset],
                data[offset + 1],
                data[offset + 2],
                data[offset + 3],
                data[offset + 4],
                data[offset + 5],
                data[offset + 6],
                data[offset + 7],
            ]);
            offset += 8;

            let mut storage_key = [0u8; 32];
            storage_key.copy_from_slice(&data[offset..offset + 32]);
            offset += 32;

            let mut pre_value = [0u8; 32];
            pre_value.copy_from_slice(&data[offset..offset + 32]);
            offset += 32;

            let mut post_value = [0u8; 32];
            post_value.copy_from_slice(&data[offset..offset + 32]);
            offset += 32;

            // TxIndex (4 bytes, big-endian) - skip for now (not needed in verifier)
            if data.len() < offset + 4 {
                return None;
            }
            let tx_index = u32::from_be_bytes([
                data[offset],
                data[offset + 1],
                data[offset + 2],
                data[offset + 3],
            ]);
            offset += 4;

            read_entries.push(VerkleWitnessEntry {
                key,
                metadata: VerkleKeyMetadata {
                    key_type,
                    address: H160::from(address),
                    extra,
                    storage_key: H256::from(storage_key),
                    tx_index,
                },
                pre_value,
                post_value,
            });
        }

        if data.len() < offset + 4 {
            return None;
        }
        let pre_proof_len = u32::from_be_bytes([
            data[offset],
            data[offset + 1],
            data[offset + 2],
            data[offset + 3],
        ]) as usize;
        offset += 4;

        if data.len() < offset + pre_proof_len {
            return None;
        }
        let pre_proof_data = data[offset..offset + pre_proof_len].to_vec();
        offset += pre_proof_len;

        if data.len() < offset + 32 {
            return None;
        }
        let mut post_state_root = [0u8; 32];
        post_state_root.copy_from_slice(&data[offset..offset + 32]);
        offset += 32;

        if data.len() < offset + 4 {
            return None;
        }
        let write_key_count = u32::from_be_bytes([
            data[offset],
            data[offset + 1],
            data[offset + 2],
            data[offset + 3],
        ]) as usize;
        offset += 4;

        let mut write_entries = Vec::with_capacity(write_key_count);
        for _ in 0..write_key_count {
            if data.len() < offset + 161 {
                return None;
            }
            let mut key = [0u8; 32];
            key.copy_from_slice(&data[offset..offset + 32]);
            offset += 32;

            let key_type = match data[offset] {
                0 => VerkleKeyType::BasicData,
                1 => VerkleKeyType::CodeHash,
                2 => VerkleKeyType::CodeChunk,
                3 => VerkleKeyType::Storage,
                _ => return None,
            };
            offset += 1;

            let mut address = [0u8; 20];
            address.copy_from_slice(&data[offset..offset + 20]);
            offset += 20;

            let extra = u64::from_be_bytes([
                data[offset],
                data[offset + 1],
                data[offset + 2],
                data[offset + 3],
                data[offset + 4],
                data[offset + 5],
                data[offset + 6],
                data[offset + 7],
            ]);
            offset += 8;

            let mut storage_key = [0u8; 32];
            storage_key.copy_from_slice(&data[offset..offset + 32]);
            offset += 32;

            let mut pre_value = [0u8; 32];
            pre_value.copy_from_slice(&data[offset..offset + 32]);
            offset += 32;

            let mut post_value = [0u8; 32];
            post_value.copy_from_slice(&data[offset..offset + 32]);
            offset += 32;

            // TxIndex (4 bytes, big-endian) - skip for now (not needed in verifier)
            if data.len() < offset + 4 {
                return None;
            }
            let tx_index = u32::from_be_bytes([
                data[offset],
                data[offset + 1],
                data[offset + 2],
                data[offset + 3],
            ]);
            offset += 4;

            write_entries.push(VerkleWitnessEntry {
                key,
                metadata: VerkleKeyMetadata {
                    key_type,
                    address: H160::from(address),
                    extra,
                    storage_key: H256::from(storage_key),
                    tx_index,
                },
                pre_value,
                post_value,
            });
        }

        // Read proof_data_len
        if data.len() < offset + 4 {
            return None;
        }
        let post_proof_len = u32::from_be_bytes([
            data[offset],
            data[offset + 1],
            data[offset + 2],
            data[offset + 3],
        ]) as usize;
        offset += 4;

        // Read proof_data
        if data.len() < offset + post_proof_len {
            return None;
        }
        let post_proof_data = data[offset..offset + post_proof_len].to_vec();

        Some(VerkleWitness {
            pre_state_root: H256::from(pre_state_root),
            read_entries,
            pre_proof_data,
            post_state_root: H256::from(post_state_root),
            write_entries,
            post_proof_data,
        })
    }

    /// Serialize VerkleWitness to bytes matching the dual-section layout.
    #[allow(dead_code)]
    pub fn serialize(&self) -> Vec<u8> {
        let mut result = Vec::new();

        // Write pre_state_root
        result.extend_from_slice(self.pre_state_root.as_bytes());

        // Write read section
        result.extend_from_slice(&(self.read_entries.len() as u32).to_be_bytes());
        for entry in &self.read_entries {
            result.extend_from_slice(&entry.key);
            result.push(entry.metadata.key_type as u8);
            result.extend_from_slice(entry.metadata.address.as_bytes());
            result.extend_from_slice(&entry.metadata.extra.to_be_bytes());
            result.extend_from_slice(entry.metadata.storage_key.as_bytes());
            result.extend_from_slice(&entry.pre_value);
            result.extend_from_slice(&entry.post_value);
            result.extend_from_slice(&entry.metadata.tx_index.to_be_bytes());
        }
        result.extend_from_slice(&(self.pre_proof_data.len() as u32).to_be_bytes());
        result.extend_from_slice(&self.pre_proof_data);

        // Write post_state_root and post section
        result.extend_from_slice(self.post_state_root.as_bytes());
        result.extend_from_slice(&(self.write_entries.len() as u32).to_be_bytes());
        for entry in &self.write_entries {
            result.extend_from_slice(&entry.key);
            result.push(entry.metadata.key_type as u8);
            result.extend_from_slice(entry.metadata.address.as_bytes());
            result.extend_from_slice(&entry.metadata.extra.to_be_bytes());
            result.extend_from_slice(entry.metadata.storage_key.as_bytes());
            result.extend_from_slice(&entry.pre_value);
            result.extend_from_slice(&entry.post_value);
            result.extend_from_slice(&entry.metadata.tx_index.to_be_bytes());
        }
        result.extend_from_slice(&(self.post_proof_data.len() as u32).to_be_bytes());
        result.extend_from_slice(&self.post_proof_data);

        result
    }

    /// Verify the Verkle proof via host function using raw serialized bytes
    ///
    /// This allocates a buffer in PVM RAM sized to the witness data and passes its
    /// address to the host function.
    ///
    /// Returns true if the proof is valid, false otherwise
    pub fn verify_raw(witness_data: &[u8]) -> bool {
        utils::functions::log_info(&format!(
            "verify_raw: input witness_data.len()={}",
            witness_data.len()
        ));

        // Allocate buffer sized to the incoming witness
        // NOTE: Witnesses can exceed 64KB after dual-proof expansion; a fixed stack
        // array would truncate verification to 64KB and falsely reject valid blocks.
        // Heap-allocate to fit the exact witness length while keeping a contiguous
        // region inside PVM RAM for the host call.
        let mut buffer = alloc::vec![0u8; witness_data.len()];

        // Copy witness to PVM RAM buffer
        buffer.copy_from_slice(witness_data);

        utils::functions::log_info(&format!(
            "verify_raw: calling host with ptr={:#x}, len={}",
            buffer.as_ptr() as u64,
            witness_data.len()
        ));

        // Pass PVM RAM address to host function
        unsafe {
            let result = host_verify_verkle_proof(
                buffer.as_ptr() as u64,
                witness_data.len() as u64,
            );
            utils::functions::log_info(&format!("verify_raw: host returned {}", result));
            result == 1
        }
    }

    /// Verify the Verkle proof via host function
    ///
    /// Returns true if the proof is valid, false otherwise
    #[allow(dead_code)]
    pub fn verify(&self) -> bool {
        let serialized = self.serialize();
        Self::verify_raw(&serialized)
    }

    /// Parse the witness to extract state data into structured caches
    ///
    /// Returns:
    /// - balances: address -> balance
    /// - nonces: address -> nonce
    /// - code: address -> bytecode
    /// - code_hashes: address -> code_hash
    /// - storage: (address, key) -> value
    pub fn parse_to_caches(&self) -> (
        BTreeMap<H160, U256>,
        BTreeMap<H160, U256>,
        BTreeMap<H160, Vec<u8>>,
        BTreeMap<H160, H256>,
        BTreeMap<(H160, H256), H256>,
    ) {
        utils::functions::log_info(&format!(
            "📋 parse_to_caches: Processing {} read_entries",
            self.read_entries.len()
        ));

        let mut balances = BTreeMap::new();
        let mut nonces = BTreeMap::new();
        let mut code_sizes = BTreeMap::new();
        let mut code_chunks: BTreeMap<H160, BTreeMap<u64, Vec<u8>>> = BTreeMap::new();
        let mut code = BTreeMap::new();
        let mut code_hashes = BTreeMap::new();
        let mut storage = BTreeMap::new();

        // Parse read entries from pre-state section (use read_entries for cache population)
        for (idx, entry) in self.read_entries.iter().enumerate() {
            let metadata = &entry.metadata;
            let pre_value = &entry.pre_value;
            let address = metadata.address;

            utils::functions::log_info(&format!(
                "  Entry[{}]: key_type={:?}, address={:?}, pre_value[0..8]={:02x?}",
                idx,
                metadata.key_type,
                address,
                &pre_value[0..8.min(pre_value.len())]
            ));

            match metadata.key_type {
                VerkleKeyType::BasicData => {
                    // BasicData format (32 bytes):
                    // - version(1) + reserved(4) + code_size(3) + nonce(8) + balance(16)

                    // Extract code_size (offset 5-7, 3 bytes, big-endian)
                    let code_size =
                        ((pre_value[5] as u32) << 16) | ((pre_value[6] as u32) << 8) | pre_value[7] as u32;
                    code_sizes.insert(address, code_size);

                    // Extract nonce (offset 8-15, 8 bytes, big-endian)
                    let nonce_bytes = &pre_value[8..16];
                    let nonce = U256::from_big_endian(nonce_bytes);
                    nonces.insert(address, nonce);

                    // Extract balance (offset 16-31, 16 bytes, big-endian)
                    let mut balance_bytes = [0u8; 32];
                    balance_bytes[16..32].copy_from_slice(&pre_value[16..32]);
                    let balance = U256::from_big_endian(&balance_bytes);
                    balances.insert(address, balance);

                    utils::functions::log_info(&format!(
                        "    ✅ Cached BasicData: address={:?}, balance={}, nonce={}",
                        address, balance, nonce
                    ));
                }

                VerkleKeyType::CodeHash => {
                    // Code hash is the full 32-byte pre_value
                    // If zero (absent in verkle tree), treat as empty code hash
                    let code_hash = if *pre_value == [0u8; 32] {
                        H256::from(keccak256(&[]))
                    } else {
                        H256::from(*pre_value)
                    };
                    code_hashes.insert(address, code_hash);
                }

                VerkleKeyType::CodeChunk => {
                    // Code chunk: chunk_id in metadata.extra, chunk data in pre_value
                    let chunk_id = metadata.extra;
                    let chunk_data = pre_value.to_vec();

                    code_chunks
                        .entry(address)
                        .or_insert_with(BTreeMap::new)
                        .insert(chunk_id, chunk_data);
                }

                VerkleKeyType::Storage => {
                    // Storage: metadata.storage_key contains the full 32-byte storage slot
                    let storage_key = metadata.storage_key;

                    // pre_value contains the 32-byte storage value
                    let storage_value = H256::from(*pre_value);
                    storage.insert((address, storage_key), storage_value);
                }
            }
        }

        // Aggregate code chunks into complete bytecode
        for (address, chunks) in code_chunks {
            let mut bytecode = Vec::new();

            // Chunks are stored in order by chunk_id
            for (_chunk_id, chunk_data) in chunks {
                bytecode.extend_from_slice(&chunk_data);
            }

            // Trim bytecode to the declared code_size (from BasicData) to remove padding
            if let Some(code_size) = code_sizes.get(&address) {
                let code_size_usize = *code_size as usize;
                if code_size_usize < bytecode.len() {
                    bytecode.truncate(code_size_usize);
                }
            }

            code.insert(address, bytecode);
        }

        utils::functions::log_info(&format!(
            "📦 parse_to_caches: Final cache sizes - balances={}, nonces={}, code={}, code_hashes={}, storage={}",
            balances.len(),
            nonces.len(),
            code.len(),
            code_hashes.len(),
            storage.len()
        ));

        utils::functions::log_info(&format!(
            "📦 Cached addresses (balance): {:?}",
            balances.keys().collect::<Vec<_>>()
        ));

        utils::functions::log_info(&format!(
            "📦 Cached addresses (nonce): {:?}",
            nonces.keys().collect::<Vec<_>>()
        ));

        (balances, nonces, code, code_hashes, storage)
    }
}
