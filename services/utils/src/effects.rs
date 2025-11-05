//! Effects handling for JAM service execution
//!
//! This module contains functions for managing execution effects,
//! including exporting write effects and processing state witnesses.

extern crate alloc;

use crate::constants::SEGMENT_SIZE;
use crate::functions::{call_log, format_object_id, log_debug, log_error, log_info};
use crate::functions::{HarnessError, HarnessResult};
use alloc::format;
use alloc::vec::Vec;
use core::cmp;
use core::convert::TryFrom;

/// Type alias for ObjectId (32-byte array)
pub type ObjectId = [u8; 32];
use crate::host_functions::export;

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
    /// Total number of host export calls performed when materializing payloads.
    pub export_count: u16,
    /// Total gas used across all committed transactions in refine.
    pub gas_used: u64,
    /// Constructed by BlockRefiner (post-bootstrap state root, receipts root, etc.)
    pub state_root: [u8; 32],
    pub write_intents: Vec<WriteIntent>,
}

impl ExecutionEffects {
    /// Log a tally of write intents by ObjectKind, including receipt version breakdown.
    /// Validates receipt pipeline invariants when tracking parameters are provided.
    pub fn show_tally_object_kinds(&self) {
        let mut code_count: u32 = 0;
        let mut shard_count: u32 = 0;
        let mut ssr_count: u32 = 0;
        let mut receipt_count: u32 = 0;
        let mut block_count: u32 = 0;
        let mut receipt_v0: u32 = 0;
        let mut receipt_v1: u32 = 0;

        for intent in &self.write_intents {
            match intent.effect.ref_info.object_kind {
                0x00 => code_count = code_count.saturating_add(1),
                0x01 => shard_count = shard_count.saturating_add(1),
                0x02 => ssr_count = ssr_count.saturating_add(1),
                0x03 => {
                    receipt_count = receipt_count.saturating_add(1);
                    if intent.effect.ref_info.version == 0 {
                        receipt_v0 = receipt_v0.saturating_add(1);
                    } else if intent.effect.ref_info.version >= 1 {
                        receipt_v1 = receipt_v1.saturating_add(1);
                    }
                }
                0x05 => block_count = block_count.saturating_add(1),
                _ => {}
            }
        }

        if code_count > 0 {
            call_log(
                0,
                None,
                &format!("ExecutionEffects tally: Code objects = {}", code_count),
            );
        }
        if shard_count > 0 {
            call_log(
                0,
                None,
                &format!("ExecutionEffects tally: StorageShard objects = {}", shard_count),
            );
        }
        if ssr_count > 0 {
            call_log(
                0,
                None,
                &format!("ExecutionEffects tally: SsrMetadata objects = {}", ssr_count),
            );
        }
        if receipt_count > 0 {
            call_log(
                0,
                None,
                &format!(
                    "ExecutionEffects tally: Receipt objects = {} (v0={}, v1={})",
                    receipt_count, receipt_v0, receipt_v1
                ),
            );
        }

        if block_count > 0 {
            call_log(
                0,
                None,
                &format!("ExecutionEffects tally: Block objects = {}", block_count),
            );
        }
    }

}



/// Dependency that must be satisfied before a write intent can be committed.
#[derive(Debug, Clone)]
pub struct ObjectDependency {
    pub object_id: ObjectId,
    pub required_version: u32,
}

impl core::fmt::Display for ObjectDependency {
    fn fmt(&self, f: &mut core::fmt::Formatter<'_>) -> core::fmt::Result {
        write!(
            f,
            "{}@v{}",
            format_object_id(self.object_id),
            self.required_version
        )
    }
}

/// Write intent consisting of a write effect and its prerequisite dependencies.
#[derive(Debug, Clone)]
pub struct WriteIntent {
    pub effect: WriteEffectEntry,
    pub dependencies: Vec<ObjectDependency>,
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
            buffer.extend_from_slice(&dependency.object_id);
            buffer.extend_from_slice(&dependency.required_version.to_le_bytes());
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

    if let Some(write_effect) = objects.get(&object_id) {
        call_log(2, None, "fetch_object_data: found in map");
        let payload_len = write_effect.payload.len();
        call_log(
            2,
            None,
            &format!("fetch_object_data: payload len={}", payload_len),
        );
        call_log(2, None, "fetch_object_data: about to clone payload");
        let cloned = write_effect.payload.clone();
        call_log(2, None, "fetch_object_data: payload cloned successfully");
        return Ok(cloned);
    }
    call_log(1, None, "fetch_object_data: NOT FOUND in map");
    return Err(HarnessError::HostFetchFailed);
}

/// Entry tracking a write operation
#[derive(Debug, Clone)]
pub struct WriteEffectEntry {
    // 32 + 64 = 96 bytes + payload
    pub object_id: ObjectId, // 32-byte object identifier
    pub ref_info: ObjectRef, // 64 bytes
    pub payload: Vec<u8>,    // variable length payload data
}

impl WriteEffectEntry {
    /// Serializes this write effect entry into the provided buffer at the given cursor position.
    /// Returns the number of bytes written.
    ///
    /// OPTIMIZATION: Only exports payload to DA segments (removes 96 bytes overhead per object)
    /// ObjectID can be recomputed from payload, ObjectRef is created by accumulate step
    pub fn serialize_into(&self, buffer: &mut [u8], cursor: usize) -> HarnessResult<usize> {
        let total_size = self.payload.len(); // Only payload, no ObjectID/ObjectRef
        if cursor + total_size > buffer.len() {
            return Err(HarnessError::ParseError);
        }

        let mut pos = cursor;

        // Only write payload to DA segment (ObjectID/ObjectRef are redundant)
        buffer[pos..pos + self.payload.len()].copy_from_slice(&self.payload);
        pos += self.payload.len();

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
        const SEGMENT_PAYLOAD_SIZE: usize = SEGMENT_SIZE as usize;
        const FULL: u64 = 3172;

        let payload_len = self.payload.len();

        // Calculate number of segments needed (all segments are payload-only now)
        let num_segments = (payload_len + SEGMENT_PAYLOAD_SIZE - 1) / SEGMENT_PAYLOAD_SIZE;

        // Set index_start from the input parameter
        let start_index = u16::try_from(index).map_err(|_| HarnessError::ParseError)?;
        let end_index = start_index
            .checked_add(u16::try_from(num_segments).map_err(|_| HarnessError::ParseError)?)
            .ok_or(HarnessError::ParseError)?;

        self.ref_info.index_start = start_index;
        self.ref_info.index_end = end_index;

        // Export all segments as payload-only (no metadata)
        for segment_idx in 0..num_segments {
            let test_buffer = unsafe {
                let ptr = &raw mut SEGMENT_BUFFER;
                &mut *ptr.cast::<[u8; SEGMENT_SIZE as usize]>()
            };
            test_buffer.fill(0);

            // Calculate offset and size for this segment
            let offset = segment_idx * SEGMENT_PAYLOAD_SIZE;
            let remaining = payload_len - offset;
            let size = cmp::min(SEGMENT_PAYLOAD_SIZE, remaining);

            // Copy payload chunk into buffer
            test_buffer[0..size].copy_from_slice(&self.payload[offset..offset + size]);

            let result = unsafe { export(test_buffer.as_ptr() as u64, SEGMENT_SIZE as u64) };
            if result == FULL {
                call_log(
                    1,
                    None,
                    &format!(
                        "Failed to export segment {}/{} of write effect {}: object_id={}",
                        segment_idx + 1,
                        num_segments,
                        index,
                        format_object_id(self.object_id)
                    ),
                );
                return Err(HarnessError::HostExportFailed);
            }
        }

        // Return the next available index (index_end)
        Ok(end_index)
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

    /// Creates a write effect from this state witness and a set of imported segments.
    /// Uses the first segment (at index_start) to extract the initial payload chunk after
    /// metadata. Appends payload from subsequent segments up to but not including index_end.
    /// Validates that the final payload length equals the payload_length declared in the
    /// witness' ObjectRef.
    pub fn create_write_effect(&self, work_item_index: u16) -> Option<WriteEffectEntry> {
        use crate::functions::fetch_imported_segment;

        let start = self.ref_info.index_start as u64;
        let end = self.ref_info.index_end as u64;
        if end <= start {
            return None;
        }

        // Fetch first segment bytes (expected to contain [ObjectId || ObjectRef || payload_part0])
        let first_bytes = fetch_imported_segment(work_item_index, start as u32).ok()?;
        if first_bytes.is_empty() {
            return None;
        }

        // Skip metadata: object_id (32 bytes) + ObjectRef (64 bytes) = 96 bytes
        let metadata_size = 32 + 64;
        if first_bytes.len() < metadata_size {
            return None;
        }
        let offset = metadata_size;

        // Begin assembling full payload: remainder of first segment after metadata
        let mut payload = Vec::with_capacity(self.ref_info.payload_length as usize);
        payload.extend_from_slice(&first_bytes[offset..]);

        // Append payload from subsequent segments [start+1, end)
        for seg_idx in (start + 1)..end {
            let bytes = fetch_imported_segment(work_item_index, seg_idx as u32).ok()?;
            if bytes.is_empty() {
                return None;
            }
            // Subsequent segments are payload-only
            payload.extend_from_slice(&bytes);
        }

        // Validate total length matches declared payload_length in the witness' ObjectRef
        let expected_len = self.ref_info.payload_length as usize;
        if payload.len() != expected_len {
            call_log(
                2,
                None,
                &format!(
                    "create_write_effect: assembled payload length = {} DOES NOT match expected = {}",
                    payload.len(),
                    expected_len
                ),
            );
            return None;
        }

        Some(WriteEffectEntry {
            object_id: self.object_id,
            ref_info: self.ref_info.clone(),
            payload,
        })
    }

    /// Verifies this state witness against a state_root using Merkle proof verification
    /// Implements the same logic as trie.Verify in Go (bpt.go:1378-1394)
    pub fn verify(&self, state_root: [u8; 32]) -> bool {
        // Get the value to verify (the serialized ObjectRef)
        let value = self.ref_info.serialize();

        // Perform Merkle proof verification
        verify_merkle_proof(
            self.ref_info.service_id,
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

    /// Deserializes RAW state witness data and synthesizes an ObjectRef
    /// Format: object_id (32 bytes) + service_id (4 bytes LE) + payload_length (4 bytes LE) + payload (variable) + proofs (32 bytes each)
    ///
    /// This is used for:
    /// - BLOCK_NUMBER_KEY witness (object_id = 0xFF×32)
    /// - Block witnesses (object_id = 0xFF×28 + block_number in last 4 bytes)
    ///
    /// NOTE: For RAW witnesses, the value stored in the trie is just the raw payload bytes,
    /// not a serialized ObjectRef. We verify first, then synthesize the ObjectRef.
    pub fn deserialize_raw_and_verify(
        bytes: &[u8],
        state_root: [u8; 32],
    ) -> Result<Self, &'static str> {
        const OBJECT_ID_SIZE: usize = 32;
        const SERVICE_ID_SIZE: usize = 4;
        const PAYLOAD_LENGTH_SIZE: usize = 4;
        const MIN_SIZE: usize = OBJECT_ID_SIZE + SERVICE_ID_SIZE + PAYLOAD_LENGTH_SIZE;


        if bytes.len() < MIN_SIZE {
            return Err("RAW witness data too short");
        }

        let mut cursor = 0;

        // 1. Extract object_id (32 bytes)
        let obj_bytes: [u8; OBJECT_ID_SIZE] = bytes[cursor..cursor + OBJECT_ID_SIZE]
            .try_into()
            .map_err(|_| {
                "Failed to extract object_id"
            })?;
        let object_id: ObjectId = obj_bytes;
        cursor += OBJECT_ID_SIZE;

        // 2. Extract service_id (4 bytes LE)
        let service_id = u32::from_le_bytes([
            bytes[cursor],
            bytes[cursor + 1],
            bytes[cursor + 2],
            bytes[cursor + 3],
        ]);
        cursor += SERVICE_ID_SIZE;

        // 3. Extract payload_length (4 bytes LE)
        let payload_length = u32::from_le_bytes([
            bytes[cursor],
            bytes[cursor + 1],
            bytes[cursor + 2],
            bytes[cursor + 3],
        ]);
        cursor += PAYLOAD_LENGTH_SIZE;

        // 4. Extract payload (variable length)
        if bytes.len() < cursor + payload_length as usize {
            return Err("Insufficient data for payload");
        }
        let payload_start = cursor;
        let payload_end = cursor + payload_length as usize;
        let payload = &bytes[payload_start..payload_end];
        cursor = payload_end;

        // 5. Extract proof hashes (32 bytes each)
        let remaining = bytes.len() - cursor;
        if remaining % 32 != 0 {
            return Err("Proof data not aligned to 32 bytes");
        }

        let proof_count = remaining / 32;
        let mut path = Vec::with_capacity(proof_count);
        for _ in 0..proof_count {
            let mut hash = [0u8; 32];
            hash.copy_from_slice(&bytes[cursor..cursor + 32]);
            path.push(hash);
            cursor += 32;
        }

        // VERIFY FIRST: For RAW witnesses, the trie stores the raw payload bytes
        // (not a serialized ObjectRef like normal witnesses)
        if !verify_merkle_proof(service_id, &object_id, payload, &state_root, &path) {
            return Err("RAW witness Merkle proof verification failed");
        }

        // Only after verification passes, synthesize the ObjectRef
        let ref_info = Self::synthesize_object_ref(object_id, service_id, payload_length, payload)?;

        Ok(StateWitness {
            object_id,
            ref_info,
            path,
        })
    }

    /// Synthesizes an ObjectRef for RAW storage witnesses
    /// Handles BLOCK_NUMBER_KEY (0xFF×32) and Block objects (0xFF×28 + block_number)
    fn synthesize_object_ref(
        object_id: ObjectId,
        service_id: u32,
        payload_length: u32,
        payload: &[u8],
    ) -> Result<ObjectRef, &'static str> {
        log_info(&format!("synthesize_object_ref: object_id={:?}, service_id={}, payload_length={}, payload_len={}",
            format_object_id(object_id), service_id, payload_length, payload.len()));

        // Check if this is BLOCK_NUMBER_KEY (all 0xFF)
        let is_block_number_key = object_id.iter().all(|&b| b == 0xFF);
        log_info(&format!("synthesize_object_ref: is_block_number_key={}", is_block_number_key));

        if is_block_number_key {
            log_info("synthesize_object_ref: creating BLOCK_NUMBER_KEY ObjectRef");
            // BLOCK_NUMBER_KEY: RAW storage with no specific ObjectKind
            return Ok(ObjectRef {
                service_id,
                work_package_hash: [0u8; 32],
                index_start: 0,
                index_end: 0,
                version: 0,
                payload_length,
                object_kind: 0, // RAW storage kind
                log_index: 0,
                tx_slot: 0,
                timeslot: 0,
                gas_used: 0,
                evm_block: 0,
            });
        }

        // Check if this is a Block object (0xFF×28 + block_number in last 4 bytes)
        let is_block_object = object_id[0..28].iter().all(|&b| b == 0xFF);
        log_info(&format!("synthesize_object_ref: is_block_object={}", is_block_object));

        if is_block_object {
            // Extract block number from last 4 bytes (little-endian)
            let block_number = u32::from_le_bytes([
                object_id[28],
                object_id[29],
                object_id[30],
                object_id[31],
            ]);
            log_info(&format!("synthesize_object_ref: extracted block_number={}", block_number));

            // Verify the payload starts with the expected block number
            if payload.len() >= 8 {
                let payload_block_number = u64::from_le_bytes([
                    payload[0],
                    payload[1],
                    payload[2],
                    payload[3],
                    payload[4],
                    payload[5],
                    payload[6],
                    payload[7],
                ]);
                log_info(&format!("synthesize_object_ref: payload_block_number={}, expected={}", payload_block_number, block_number));

                if payload_block_number != block_number as u64 {
                    log_error(&format!("synthesize_object_ref: block number mismatch: payload={}, expected={}", payload_block_number, block_number));
                    return Err("Block number mismatch between object_id and payload");
                }
                log_info("synthesize_object_ref: block number verification PASSED");
            } else {
                log_debug(&format!("synthesize_object_ref: payload too short for block number verification: {} < 8", payload.len()));
            }

            log_info("synthesize_object_ref: creating Block ObjectRef");
            // Block object: ObjectKind = 5
            return Ok(ObjectRef {
                service_id,
                work_package_hash: [0u8; 32],
                index_start: 0,
                index_end: 0,
                version: 0,
                payload_length,
                object_kind: 5, // Block kind
                log_index: 0,
                tx_slot: 0,
                timeslot: 0,
                gas_used: 0,
                evm_block: block_number,
            });
        }

        log_error("synthesize_object_ref: unknown RAW witness object_id pattern");
        Err("Unknown RAW witness object_id pattern")
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
