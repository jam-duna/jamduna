//! Effects handling for JAM service execution
//!
//! This module contains functions for managing execution effects,
//! including exporting write effects and processing state witnesses.

extern crate alloc;

use crate::constants::SEGMENT_SIZE;
use crate::functions::{call_log, format_object_id, log_debug, log_error};
use crate::functions::{HarnessError, HarnessResult};
use alloc::format;
use alloc::vec::Vec;
use core::convert::TryFrom;

/// Type alias for ObjectId (32-byte array)
pub type ObjectId = [u8; 32];

// Re-export object types from utils
pub use crate::objects::ObjectRef;



/// Serialized size (in bytes) of a single dependency entry.
const _DEPENDENCY_ENTRY_SIZE: usize = 32 + core::mem::size_of::<u32>();

/// Global buffer for segment serialization
#[unsafe(no_mangle)]
static mut SEGMENT_BUFFER: [u8; SEGMENT_SIZE as usize] = [0u8; SEGMENT_SIZE as usize];

/// Complete effects of execution
#[derive(Debug, Clone)]
pub struct ExecutionEffects {
    pub write_intents: Vec<WriteIntent>,
}

impl ExecutionEffects {
    /// Log a tally of write intents by ObjectKind, including receipt version breakdown.
    /// Validates receipt pipeline invariants when tracking parameters are provided.
    pub fn log_tally(&self) {
        // Implementation placeholder
    }
    

}




/// Write intent consisting of a write effect and its prerequisite dependencies.
#[derive(Debug, Clone)]
pub struct WriteIntent {
    pub effect: WriteEffectEntry,
    pub dependencies: Vec<ObjectId>,
}

impl WriteIntent {
    /// Serializes this intent into the provided buffer at the given cursor position.
    /// Returns the number of bytes written.
    pub fn serialize_into(&self, buffer: &mut [u8], cursor: usize) -> HarnessResult<usize> {
        let bytes_written = self.effect.serialize_into(buffer, cursor)?;
        /*        let dep_count: u16 = self
            .dependencies
            .len()
            .try_into()
            .map_err(|_| HarnessError::ParseError)?;
        buffer.extend_from_slice(&dep_count.to_le_bytes());

        for dependency in &self.dependencies {
            buffer.extend_from_slice(dependency);
        } */
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

        // Write ObjectRef (36 bytes)
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
            let result = unsafe {
                export(
                    self.payload.as_ptr() as u64,
                    self.payload.len() as u64
                )
            };

            // Check if export succeeded (result should be the next segment index)
            if result == u64::MAX {
                return Err(HarnessError::HostFetchFailed);
            }

            // Calculate next index based on payload_length and segment size
            // Each segment is SEGMENT_SIZE bytes (4104), so we need ceil(payload_length / SEGMENT_SIZE) segments
            let num_segments = (self.ref_info.payload_length as u64 + SEGMENT_SIZE - 1) / SEGMENT_SIZE;
            let next_index = start_index as u64 + num_segments;
            u16::try_from(next_index).map_err(|_| HarnessError::ParseError)
        } else {
            // Empty payload, just return next index
            start_index.checked_add(1).ok_or(HarnessError::ParseError)
        }
    }
}
/// Record of an object witnessed during refine execution and recorded by accumulate
#[derive(Debug, Clone)]
pub struct StateWitness {
    pub object_id: ObjectId,
    pub ref_info: ObjectRef,
    /// Merkle proof fields
    pub path: Vec<[u8; 32]>,
}

impl StateWitness {
    /// Deserializes state witness data from binary format
    /// Format: object_id (32 bytes) + object_ref (64 bytes) + proofs (32 bytes each)
    /// Legacy payloads may insert a 4-byte little-endian proof_count before the proofs; both forms are accepted.
    pub fn deserialize(data: &[u8]) -> HarnessResult<Self> {
        const OBJECT_ID_SIZE: usize = 32;
        const MIN_SIZE: usize = OBJECT_ID_SIZE + ObjectRef::SERIALIZED_SIZE;

        if data.len() < MIN_SIZE {
            return Err(HarnessError::TruncatedInput);
        }

        let mut cursor = 0;

        // Extract object_id
        let obj_bytes: [u8; OBJECT_ID_SIZE] = data[cursor..cursor + OBJECT_ID_SIZE]
            .try_into()
            .map_err(|_| HarnessError::TruncatedInput)?;
        let object_id: ObjectId = obj_bytes;
        cursor += OBJECT_ID_SIZE;

        // Extract ObjectRef using its deserializer
        let ref_info = ObjectRef::deserialize_from(data, &mut cursor)?;

        let mut path = Vec::new();
        let remaining_bytes = data.len() - cursor;
        if remaining_bytes == 0 {
            // No proofs attached
            return Ok(StateWitness {
                object_id,
                ref_info,
                path,
            });
        }

        let mut proof_bytes_total = remaining_bytes;

        // Legacy format: optional 4-byte proof count preceding proofs
        if remaining_bytes >= 4 && (remaining_bytes - 4) % 32 == 0 {
            let declared = u32::from_le_bytes([
                data[cursor],
                data[cursor + 1],
                data[cursor + 2],
                data[cursor + 3],
            ]) as usize;
            cursor += 4;
            proof_bytes_total = data.len() - cursor;

            // Validate declared count matches available bytes
            if declared * 32 != proof_bytes_total {
                return Err(HarnessError::ParseError);
            }
        } else if remaining_bytes % 32 != 0 {
            // New format must align to 32-byte hashes
            return Err(HarnessError::ParseError);
        }

        let proof_count = proof_bytes_total / 32;
        path.reserve(proof_count);
        for _ in 0..proof_count {
            let mut hash = [0u8; 32];
            hash.copy_from_slice(&data[cursor..cursor + 32]);
            path.push(hash);
            cursor += 32;
        }

        Ok(StateWitness {
            object_id,
            ref_info,
            path,
        })
    }

    /// Fetches the payload from this state witness and a set of imported segments.
    pub fn fetch_object_payload(&self, work_item: &crate::functions::WorkItem, work_item_index: u16) -> Option<Vec<u8>> {
        use crate::functions::fetch_imported_segment;

        log_debug(&format!(
            "fetch_object_payload: object_id={}, ref_info.index_start={}, ref_info.payload_length={}",
            format_object_id(self.object_id),
            self.ref_info.index_start,
            self.ref_info.payload_length
        ));

        let start = self.ref_info.index_start as u64;
        let end = start + self.ref_info.payload_length as u64;
        if end <= start {
            log_error(&format!(
                "fetch_object_payload: end <= start ({} <= {}), returning None",
                end, start
            ));
            return None;
        }

        let (start, end) = match work_item.get_imported_segments_range(&self.ref_info) {
            Some(range) => {
                log_debug(&format!(
                    "fetch_object_payload: found imported_segments range: start={}, end={}",
                    range.0, range.1
                ));
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
            log_debug(&format!(
                "fetch_object_payload: fetching segment {} for work_item_index={}",
                seg_idx, work_item_index
            ));
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
                log_error(&format!(
                    "fetch_object_payload: segment {} is empty, returning None",
                    seg_idx
                ));
                return None;
            }
            log_debug(&format!(
                "fetch_object_payload: segment {} fetched {} bytes",
                seg_idx, bytes.len()
            ));
            payload.extend_from_slice(&bytes);
        }

        log_debug(&format!(
            "fetch_object_payload: assembled payload length={}, expected={}",
            payload.len(),
            self.ref_info.payload_length
        ));

        payload.truncate(self.ref_info.payload_length as usize);

        log_debug(&format!(
            "fetch_object_payload: success, returning payload of length={}",
            payload.len()
        ));

        Some(payload)
    }

    /// Verifies this state witness against a state_root using Merkle proof verification
    /// Implements the same logic as trie.Verify in Go (bpt.go:1378-1394)
    pub fn verify(&self, state_root: [u8; 32]) -> bool {
        // Get the value to verify (the serialized ObjectRef)
        let value = self.ref_info.serialize();

        // Perform Merkle proof verification
        verify_merkle_proof(
            0, // service_id removed from ObjectRef
            &self.object_id,
            &value,
            &state_root,
            &self.path,
        )
    }

    /// Deserializes and verifies a state witness from bytes
    pub fn deserialize_and_verify(
        bytes: &[u8],
        state_root: [u8; 32],
    ) -> Result<Self, &'static str> {
        let witness = Self::deserialize(bytes).map_err(|_| "Failed to deserialize witness")?;
        if witness.verify(state_root) {
            Ok(witness)
        } else {
            Err("Witness verification failed")
        }
    }

}

/// Computes the opaque storage key from service_id and raw key (object_id)
/// Equivalent to Go's Compute_storage_opaqueKey in common/key_constructor_tool.go
pub(crate) fn compute_storage_opaque_key(service_id: u32, raw_key: &[u8; 32]) -> [u8; 32] {
    use crate::hash_functions::blake2b_hash;

    // Step 1: Compute_storageKey_internal: k -> E4(2^32-1)++k
    let mut as_internal_key = alloc::vec::Vec::with_capacity(4 + 32);
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
pub(crate) fn verify_merkle_proof(
    service_id: u32,
    object_id: &[u8; 32],
    value: &[u8],
    root_hash: &[u8; 32],
    path: &[[u8; 32]],
) -> bool {
    use crate::hash_functions::blake2b_hash;

    // Compute opaque key from service_id and object_id
    let opaque_key = compute_storage_opaque_key(service_id, object_id);

    // Compute leaf hash
    let leaf_data = create_leaf(&opaque_key, value);
    let mut current_hash = blake2b_hash(&leaf_data);

    // Handle empty path case - leaf is the root
    if path.is_empty() {
        return compare_bytes(&current_hash, root_hash);
    }

    // Walk up the tree using the proof path
    // Note: path is in bottom-up order (index len-1 is deepest)
    for i in (0..path.len()).rev() {
        let sibling_hash = &path[i];

        // Determine branch direction based on key bit at depth i
        if get_bit(&opaque_key, i) {
            // Bit is 1: current node is right child
            let branch_data = create_branch(sibling_hash, &current_hash);
            current_hash = blake2b_hash(&branch_data);
        } else {
            // Bit is 0: current node is left child
            let branch_data = create_branch(&current_hash, sibling_hash);
            current_hash = blake2b_hash(&branch_data);
        }
    }

    compare_bytes(&current_hash, root_hash)
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
