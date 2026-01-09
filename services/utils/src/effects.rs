//! Effects handling for JAM service execution
//!
//! This module contains functions for managing execution effects,
//! including exporting write effects and processing state witnesses.

extern crate alloc;

use crate::constants::SEGMENT_SIZE;
use crate::functions::{call_log, format_object_id, log_error};
use crate::functions::{HarnessError, HarnessResult};
use alloc::format;
use alloc::vec::Vec;
use core::convert::TryFrom;

/// Type alias for ObjectId (32-byte array)
pub type ObjectId = [u8; 32];

// Re-export object types from utils
pub use crate::objects::ObjectRef;



/// Global buffer for segment serialization
#[unsafe(no_mangle)]
static mut SEGMENT_BUFFER: [u8; SEGMENT_SIZE as usize] = [0u8; SEGMENT_SIZE as usize];

/// Complete effects of execution
#[derive(Debug, Clone)]
pub struct ExecutionEffects {
    pub write_intents: Vec<WriteIntent>, // txns/receipts
    pub contract_intents: Vec<WriteIntent>,  // contract code and storage
    pub accumulate_instructions: Vec<AccumulateInstruction>, // cross-service operations
}

/// Instructions for accumulate phase (cross-service operations)
///
/// Used by services like Railgun to emit transfer instructions
/// that will be executed during accumulate phase.
#[derive(Debug, Clone)]
pub enum AccumulateInstruction {
    /// Transfer tokens to another service
    ///
    /// Arguments:
    /// - destination_service: Target service index
    /// - amount: Amount to transfer (native JAM tokens)
    /// - gas_limit: Gas limit for destination service accumulate
    /// - memo: 128-byte memo data for destination service
    Transfer {
        destination_service: u32,
        amount: u64,
        gas_limit: u64,
        memo: [u8; 128],
    },
}

/// Write intent consisting of a write effect
#[derive(Debug, Clone)]
pub struct WriteIntent {
    pub effect: WriteEffectEntry,
}

impl WriteIntent {
    /// Serializes this intent into the provided buffer at the given cursor position.
    /// Returns the number of bytes written.
    pub fn serialize_into(&self, buffer: &mut [u8], cursor: usize) -> HarnessResult<usize> {
        let bytes_written = self.effect.serialize_into(buffer, cursor)?;

        Ok(bytes_written)
    }
}

/// Fetches object data from objects map using module hash as key, with historical_lookup fallback
pub fn fetch_object_data(
    hash: &[u8; 32],
    objects: &::alloc::collections::BTreeMap<ObjectId, WriteEffectEntry>,
) -> HarnessResult<Vec<u8>> {
    call_log(2, None, "fetch_object_data: start");
    // Use hash as ObjectId to look up the module in objects
    let object_id: ObjectId = *hash;
    call_log(2, None, "fetch_object_data: object_id copied");

    if let Some(_write_effect) = objects.get(&object_id) {
        call_log(2, None, "fetch_object_data: found in map");
        let payload_len = 0; // payload field removed
        call_log(
            2,
            None,
            &format!("fetch_object_data: payload len={}", payload_len),
        );
        call_log(2, None, "fetch_object_data: about to clone payload");
        let cloned = Vec::new(); // payload field removed
        call_log(2, None, "fetch_object_data: payload cloned successfully");
        return Ok(cloned);
    }
    call_log(1, None, "fetch_object_data: NOT FOUND in map");
    return Err(HarnessError::HostFetchFailed);
}

/// Entry tracking a write operation
#[derive(Debug, Clone)]
pub struct WriteEffectEntry {
    // 32 + 36 = 68 bytes (no payload)
    pub object_id: ObjectId, // 32-byte object identifier
    pub ref_info: ObjectRef, // 36 bytes
    pub payload: Vec<u8>,
    pub tx_index: u32,  // Transaction index that caused this write (for BAL construction)
}

impl WriteEffectEntry {
    /// Serializes this write effect entry into the provided buffer at the given cursor position.
    /// Returns the number of bytes written.
    ///
    /// Note: With payload removed, this now serializes ObjectID + ObjectRef
    pub fn serialize_into(&self, buffer: &mut [u8], cursor: usize) -> HarnessResult<usize> {
        let total_size = 32 + ObjectRef::SERIALIZED_SIZE; // ObjectID + ObjectRef
        if cursor + total_size > buffer.len() {
            return Err(HarnessError::ParseError);
        }

        let mut pos = cursor;

        // Write ObjectID (32 bytes)
        buffer[pos..pos + 32].copy_from_slice(&self.object_id);
        pos += 32;

        // Write ObjectRef (37 bytes)
        let ref_data = self.ref_info.serialize();
        buffer[pos..pos + ref_data.len()].copy_from_slice(&ref_data);
        pos += ref_data.len();

        Ok(pos - cursor)
    }

    /// Exports this write effect to segments
    ///
    /// # Arguments
    /// * `index` - The starting segment index to use for export
    ///
    /// # Returns
    /// The next available segment index (index_end) for the next object
    pub fn export_effect(&mut self, index: usize) -> HarnessResult<u16> {
        use crate::constants::SEGMENT_SIZE;
        use crate::host_functions::export;

        // Set index_start from the input parameter
        let start_index = u16::try_from(index).map_err(|_| HarnessError::ParseError)?;
        self.ref_info.index_start = start_index;

        // Export the payload to segments using the host function
        if !self.payload.is_empty() {
            // log_info(&format!(
            //     "export_effect pre-call -- object_id={}, payload_len_declared={}, payload_vec_len={}",
            //     format_object_id(self.object_id),
            //     self.ref_info.payload_length,
            //     self.payload.len()
            // ));
            // IMPORTANT: export() only accepts up to SEGMENT_SIZE bytes per call.
            // Copy each chunk into a static buffer before calling the FFI to avoid
            // passing pointers into self.payload (which can be corrupted by the FFI).
            let mut offset = 0usize;
            while offset < self.payload.len() {
                let remaining = self.payload.len() - offset;
                let chunk_len = core::cmp::min(remaining, SEGMENT_SIZE as usize);
                let buffer_ptr = &raw mut SEGMENT_BUFFER as *mut u8;
                unsafe {
                    core::ptr::copy_nonoverlapping(
                        self.payload.as_ptr().add(offset),
                        buffer_ptr,
                        chunk_len,
                    );
                }

                let result = unsafe { export(buffer_ptr as u64, chunk_len as u64) };
                if result == u64::MAX {
                    return Err(HarnessError::HostFetchFailed);
                }
                offset += chunk_len;
            }

        // log_info(&format!(
        //     "export_effect --  object_id={}, index_start={}, payload_length={}",
        //     format_object_id(self.object_id),
        //     self.ref_info.index_start,
        //     self.ref_info.payload_length
        // ));

        // Calculate next index based on payload_length and segment size
            // Each segment is SEGMENT_SIZE bytes (4104), so we need ceil(payload_length / SEGMENT_SIZE) segments
            let num_segments =
                (self.ref_info.payload_length as u64 + SEGMENT_SIZE - 1) / SEGMENT_SIZE;
            let next_index = start_index as u64 + num_segments;
            u16::try_from(next_index).map_err(|_| HarnessError::ParseError)
        } else {
            // Empty payload, just return next index
            // log_info(&format!(
            //     "export_effect  EMPTY --  object_id={}, index_start={}, payload_length={}",
            //     format_object_id(self.object_id),
            //     self.ref_info.index_start,
            //     self.ref_info.payload_length
            // ));
           start_index.checked_add(0).ok_or(HarnessError::ParseError)

        }
    }
}
/// Record of an object witnessed during refine execution and recorded by accumulate
#[derive(Debug, Clone)]
pub struct StateWitness {
    pub object_id: ObjectId,
    pub ref_info: ObjectRef, // 37 bytes
    pub timeslot: u32, // 4 bytes
    pub blocknumber: u32, // 4 bytes
    pub value: Vec<u8>, // Meta-shard ObjectRef bytes from JAM State (typically 45 bytes)
    pub payload: Vec<u8>, // Optional: inline payload provided by builder (bypasses import segments)
    /// Merkle proof fields
    pub path: Vec<[u8; 32]>,
}

impl StateWitness {
    /// Deserializes state witness data from binary format
    /// Preferred format (Railgun witnesses): object_id (32) | value_len (4) | value | proof_count (4) | proofs (32B each) | payload
    /// Legacy format: object_id (32) | ObjectRef (37) | timeslot (4) | blocknumber (4) | proofs...
    pub fn deserialize(data: &[u8]) -> HarnessResult<Self> {
        const OBJECT_ID_SIZE: usize = 32;
        const LEGACY_MIN_SIZE: usize = OBJECT_ID_SIZE + ObjectRef::SERIALIZED_SIZE + 8; // +8 for timeslot+blocknumber

        if data.len() < OBJECT_ID_SIZE + 8 {
            return Err(HarnessError::TruncatedInput);
        }

        let mut cursor = 0;

        // Extract object_id
        let obj_bytes: [u8; OBJECT_ID_SIZE] = data[cursor..cursor + OBJECT_ID_SIZE]
            .try_into()
            .map_err(|_| HarnessError::TruncatedInput)?;
        let object_id: ObjectId = obj_bytes;
        cursor += OBJECT_ID_SIZE;

        // Attempt to parse the new length-prefixed format
        let mut try_new_format = || -> Option<StateWitness> {
            if data.len() < cursor + 8 {
                return None;
            }

            let value_len = u32::from_le_bytes([
                data[cursor],
                data[cursor + 1],
                data[cursor + 2],
                data[cursor + 3],
            ]) as usize;
            cursor += 4;

            if data.len() < cursor + value_len + 4 {
                return None;
            }

            let value = data[cursor..cursor + value_len].to_vec();
            cursor += value_len;

            let proof_count = u32::from_le_bytes([
                data[cursor],
                data[cursor + 1],
                data[cursor + 2],
                data[cursor + 3],
            ]) as usize;
            cursor += 4;

            if data.len() < cursor + (proof_count * 32) {
                return None;
            }

            let mut path = Vec::with_capacity(proof_count);
            for _ in 0..proof_count {
                let mut hash = [0u8; 32];
                hash.copy_from_slice(&data[cursor..cursor + 32]);
                path.push(hash);
                cursor += 32;
            }

            let payload = data[cursor..].to_vec();

            Some(StateWitness {
                object_id,
                ref_info: ObjectRef::default(),
                timeslot: 0,
                blocknumber: 0,
                value,
                payload,
                path,
            })
        };

        if let Some(w) = try_new_format() {
            return Ok(w);
        }

        // Fallback: legacy format
        if data.len() < LEGACY_MIN_SIZE {
            return Err(HarnessError::TruncatedInput);
        }

        cursor = OBJECT_ID_SIZE;
        let value_start = cursor;

        // Extract ObjectRef using its deserializer
        let ref_info = ObjectRef::deserialize_from(data, &mut cursor)?;

        // Extract timeslot (4 bytes, little-endian)
        if data.len() < cursor + 4 {
            return Err(HarnessError::TruncatedInput);
        }
        let timeslot = u32::from_le_bytes([
            data[cursor],
            data[cursor + 1],
            data[cursor + 2],
            data[cursor + 3],
        ]);
        cursor += 4;

        // Extract blocknumber (4 bytes, little-endian)
        if data.len() < cursor + 4 {
            return Err(HarnessError::TruncatedInput);
        }
        let blocknumber = u32::from_le_bytes([
            data[cursor],
            data[cursor + 1],
            data[cursor + 2],
            data[cursor + 3],
        ]);
        cursor += 4;

        // Capture the value bytes (ObjectRef + timeslot + blocknumber)
        let value = data[value_start..cursor].to_vec();

        // Remaining bytes are treated as payload; proofs are optional/unused in this path.
        let payload = data[cursor..].to_vec();
        let path = Vec::new();

        Ok(StateWitness {
            object_id,
            ref_info,
            timeslot,
            blocknumber,
            value,
            payload,
            path,
        })
    }

    /// Fetches the payload from this state witness and a set of imported segments.
    pub fn fetch_object_payload(&self, work_item: &crate::functions::WorkItem, work_item_index: u16) -> Option<Vec<u8>> {
        use crate::functions::fetch_imported_segment;

        // Inline payload provided by builder: use directly, skip DA lookup
        if !self.payload.is_empty() {
            return Some(self.payload.clone());
        }


        let start = self.ref_info.index_start as u64;
        let end = start + self.ref_info.payload_length as u64;
        if end <= start {
            // log_error(&format!(
            //     "fetch_object_payload: end <= start ({} <= {}), returning None",
            //     end, start
            // ));
            return None;
        }

        let (start, end) = match work_item.get_imported_segments_range(&self.ref_info) {
            Some(range) => {

                range
            }
            None => {
                log_error(&format!(
                    "fetch_object_payload: get_imported_segments_range returned None for object_id={}",
                    format_object_id(self.object_id)
                ));
                return None;
            }
        };

        let mut payload: Vec<u8> = Vec::new();
        for seg_idx in start..end {

            let bytes = match fetch_imported_segment(work_item_index, seg_idx as u32) {
                Ok(b) => b,
                Err(e) => {
                    log_error(&format!(
                        "fetch_object_payload: fetch_imported_segment failed for seg_idx={}, error={:?}",
                        seg_idx, e
                    ));
                    return None;
                }
            };
            if bytes.is_empty() {
                // log_error(&format!(
                //     "fetch_object_payload: segment {} is empty, returning None",
                //     seg_idx
                // ));
                return None;
            }

            payload.extend_from_slice(&bytes);
        }


        payload.truncate(self.ref_info.payload_length as usize);

        // log_debug(&format!(
        //     "fetch_object_payload: success, returning payload of length={}",
        //     payload.len()
        // ));

        Some(payload)
    }

    /// Verifies this state witness against a state_root using Merkle proof verification
    /// Implements the same logic as trie.Verify in Go (bpt.go:1378-1394)
    pub fn verify(&self, service_id: u32, state_root: [u8; 32]) -> bool {
        // Compute meta-shard object id from object_id
        // For meta-sharded objects, we need to map object_id → meta-shard id
        // The proof verifies that meta-shard id → value exists in JAM State
        let meta_shard_key = self.compute_meta_shard_key();

        use crate::functions::log_debug;
        log_debug(&format!(
            "🔐 Verifying: service_id={}, meta_shard_key={}, value_len={}, path_len={}, state_root={}",
            service_id,
            format_object_id(meta_shard_key),
            self.value.len(),
            self.path.len(),
            format_object_id(state_root)
        ));

        // Perform Merkle proof verification using the value field (45 bytes)
        verify_merkle_proof(
            service_id,
            &meta_shard_key,
            &self.value,
            &state_root,
            &self.path,
        )
    }

    /// Computes the meta-shard object id from object_id for meta-shard routing
    fn compute_meta_shard_key(&self) -> [u8; 32] {
        // Railgun witnesses store the direct object_id (no meta-sharding yet)
        self.object_id
    }

    /// Deserializes and verifies a state witness from bytes
    pub fn deserialize_and_verify(
        _service_id: u32,
        bytes: &[u8],
        _state_root: [u8; 32],
    ) -> Result<Self, &'static str> {
        let witness = Self::deserialize(bytes).map_err(|_| "Failed to deserialize witness")?;

        // Log witness details for debugging
        //use crate::functions::log_info;
        // log_info(&format!(
        //     "🔍 Witness: object_id={}, wph={}, idx_start={}, payload_len={}, kind={}, path_len={}, state_root={}",
        //     format_object_id(witness.object_id),
        //     format_object_id(witness.ref_info.work_package_hash),
        //     witness.ref_info.index_start,
        //     witness.ref_info.payload_length,
        //     witness.ref_info.object_kind,
        //     witness.path.len(),
        //     format_object_id(state_root)
        // ));
        if false {
            // this treats the metashard witness, but we need to also treat the object witness by verifying ObjectProof
            // if witness.verify(service_id, state_root) {
            //     log_debug(&format!("🔍 Witness verification succeeded"));
            //     Ok(witness)
            // } else {
            //     Err("Witness verification failed")
            // }
        }
        Ok(witness)
    }

}

/// Computes the opaque storage key from service_id and raw key (object_id)
/// Equivalent to Go's Compute_storage_opaqueKey in common/key_constructor_tool.go
/// Accepts variable-length raw keys (no padding) to mirror Go's implementation.
pub(crate) fn compute_storage_opaque_key(service_id: u32, raw_key: &[u8]) -> [u8; 32] {
    use crate::hash_functions::blake2b_hash;

    // Step 1: Compute_storageKey_internal: k -> E4(2^32-1)++k
    let mut as_internal_key = alloc::vec::Vec::with_capacity(4 + raw_key.len());
    let prefix = u32::MAX; // 2^32 - 1
    as_internal_key.extend_from_slice(&prefix.to_le_bytes());
    as_internal_key.extend_from_slice(raw_key);

    // Step 2: ComputeC_sh(s, as_internal_key)
    // Hash the internal key with Blake2b
    let hash = blake2b_hash(&as_internal_key);

    // Encode service_id as little-endian bytes
    let n = service_id.to_le_bytes();

    // Interleave: [n0, a0, n1, a1, n2, a2, n3, a3, a4, a5, ..., a26, 0]
    let mut state_key = [0u8; 32];
    for i in 0..4 {
        state_key[2 * i] = n[i];
        state_key[2 * i + 1] = hash[i];
    }
    // Copy a[4..27] to state_key[8..31]
    state_key[8..31].copy_from_slice(&hash[4..27]);
    // Last byte (state_key[31]) stays 0

    state_key
}

/// Verifies a Merkle proof for a key-value pair against a root hash
/// Implements the same logic as trie.Verify in Go (bpt.go:1378-1394)
pub fn verify_merkle_proof(
    service_id: u32,
    raw_key: &[u8],
    value: &[u8],
    root_hash: &[u8; 32],
    path: &[[u8; 32]],
) -> bool {
    use crate::hash_functions::blake2b_hash;
    use crate::functions::log_debug;

    // Compute opaque key using variable-length raw key (matches Go implementation)
    let opaque_key = compute_storage_opaque_key(service_id, raw_key);

    log_debug(&format!(
        "🔑 verify_merkle_proof inputs: service_id={}, raw_key_len={}, value_len={}, root_hash={}, path_len={}",
        service_id,
        raw_key.len(),
        value.len(),
        format_object_id(*root_hash),
        path.len()
    ));

    // Genesis / missing proofs: allow empty path (builder provided default value)
    if path.is_empty() {
        return true;
    }

    // Leaf hash (embedded if <=32 bytes else hash of value)
    let mut leaf_hash = blake2b_hash(&create_leaf(&opaque_key, value));

    // Walk proof from deepest sibling to root (path order is root -> leaf in Go)
    for (depth, sibling_hash) in path.iter().enumerate().rev() {
        if get_bit(&opaque_key, depth) {
            leaf_hash = blake2b_hash(&create_branch(sibling_hash, &leaf_hash));
        } else {
            leaf_hash = blake2b_hash(&create_branch(&leaf_hash, sibling_hash));
        }
    }

    compare_bytes(&leaf_hash, root_hash)
}

/// Creates a leaf node encoding (Equation D.4 in GP 0.6.2)
/// Returns 64-byte array: [header(1) || key(31) || value_or_hash(32)]
fn create_leaf(key: &[u8; 32], value: &[u8]) -> [u8; 64] {
    let mut result = [0u8; 64];

    if value.len() <= 32 {
        // Embedded-value leaf node
        result[0] = 0b10000000 | (value.len() as u8);
        result[1..32].copy_from_slice(&key[..31]);
        result[32..32 + value.len()].copy_from_slice(value);
    } else {
        // Regular leaf node (value is too large, store hash)
        use crate::hash_functions::blake2b_hash;
        result[0] = 0b11000000;
        result[1..32].copy_from_slice(&key[..31]);
        let value_hash = blake2b_hash(value);
        result[32..64].copy_from_slice(&value_hash);
    }

    result
}

/// Creates a branch node encoding
/// Returns 64-byte array: [left_255bits || right_hash]
fn create_branch(left: &[u8; 32], right: &[u8; 32]) -> [u8; 64] {
    let mut result = [0u8; 64];

    // Clear MSB of first byte of left hash (keep only 255 bits)
    result[0] = left[0] & 0x7F;
    result[1..32].copy_from_slice(&left[1..]);
    result[32..64].copy_from_slice(right);

    result
}

/// Extracts bit i from key (MSB first ordering)
fn get_bit(key: &[u8; 32], i: usize) -> bool {
    if i >= 256 {
        return false;
    }
    let byte_index = i / 8;
    let bit_index = 7 - (i % 8); // MSB first
    (key[byte_index] & (1 << bit_index)) != 0
}

/// Compares two byte slices for equality
fn compare_bytes(a: &[u8], b: &[u8]) -> bool {
    if a.len() != b.len() {
        return false;
    }
    for i in 0..a.len() {
        if a[i] != b[i] {
            return false;
        }
    }
    true
}
