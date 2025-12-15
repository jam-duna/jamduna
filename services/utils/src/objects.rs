//! Object structures for JAM service execution
//!
//! This module contains the ObjectRef structures used across services.

extern crate alloc;
use alloc::format;
use alloc::vec;
use alloc::vec::Vec;

use crate::constants::FIRST_READABLE_ADDRESS;
use crate::effects::WriteEffectEntry;
use crate::functions::{call_log, fetch_imported_segment, HarnessError, HarnessResult};
use crate::host_functions::read;

/// Type alias for ObjectId (32-byte array)
pub type ObjectId = [u8; 32];

/// Reference to an object with work package location info
///
/// Fields filled at REFINE time (deterministic):
/// - service_id, work_package_hash, index_start, index_end, version, payload_length
///
/// Fields filled at ACCUMULATE time (ordering-dependent):
/// - timeslot: JAM block slot (maps 1:1 to canonical block hash)
/// - gas_used: Gas consumed (non-zero for receipts)
/// - evm_block: Sequential Ethereum block number
/// - tx_slot_kind: lower 24 bits = tx_index, upper 8 bits = ObjectKind
#[derive(Debug, Clone, PartialEq)]
pub struct ObjectRef {
    // Refine-time fields (deterministic at work package creation)
    pub work_package_hash: [u8; 32], // Work package hash (32 bytes)
    pub index_start: u16,            // Starting index (2 bytes)
    pub payload_length: u32,         // Payload length (4 bytes)
    pub object_kind: u8,             // Object kind (1 byte)
}

impl Default for ObjectRef {
    fn default() -> Self {
        Self {
            work_package_hash: [0u8; 32],
            index_start: 0,
            payload_length: 0,
            object_kind: 0,
        }
    }
}

impl ObjectRef {
    pub const SERIALIZED_SIZE: usize = 37; // 32 (wph) + 5 (packed 40 bits)

    /// Creates a new ObjectRef with refine-time fields set
    ///
    /// Fields:
    /// - work_package_hash, payload_length, object_kind (provided)
    /// - index_start (set to 0, filled later by runtime)
    pub fn new(work_package_hash: [u8; 32], payload_length: u32, object_kind: u8) -> Self {
        Self {
            work_package_hash,
            index_start: 0,
            payload_length,
            object_kind,
        }
    }

    /// Helper to calculate number of segments from payload_length
    /// Uses SEGMENT_SIZE (4104 bytes) per segment except possibly the last one
    fn calculate_segments_and_last_bytes(payload_length: u32) -> (u16, u16) {
        use crate::constants::SEGMENT_SIZE;
        if payload_length == 0 {
            return (0, 0);
        }
        let segment_size = SEGMENT_SIZE as u32;
        let full_segments = (payload_length - 1) / segment_size;
        let last_segment_bytes = payload_length - (full_segments * segment_size);
        let num_segments = full_segments + 1;
        (num_segments as u16, last_segment_bytes as u16)
    }

    /// Helper to calculate payload_length from segments and last_segment_bytes
    fn calculate_payload_length(num_segments: u16, last_segment_bytes: u16) -> u32 {
        use crate::constants::SEGMENT_SIZE;
        if num_segments == 0 {
            return 0;
        }
        if num_segments == 1 {
            return last_segment_bytes as u32;
        }
        let segment_size = SEGMENT_SIZE as u32;
        ((num_segments as u32 - 1) * segment_size) + last_segment_bytes as u32
    }

    /// Deserializes an ObjectRef from a byte buffer - opposite of serialize
    /// Unpacks 5 bytes (40 bits) into: index_start (12) + index_end (12) + last_segment_size (12) + object_kind (4)
    pub fn deserialize(data: &[u8]) -> Option<Self> {
        if data.len() != Self::SERIALIZED_SIZE {
            return None;
        }

        let mut offset = 0;

        let mut work_package_hash = [0u8; 32];
        work_package_hash.copy_from_slice(&data[offset..offset + 32]);
        offset += 32;

        // Unpack 5 bytes (40 bits) into fields
        let packed: u64 = ((data[offset] as u64) << 32)
            | ((data[offset + 1] as u64) << 24)
            | ((data[offset + 2] as u64) << 16)
            | ((data[offset + 3] as u64) << 8)
            | (data[offset + 4] as u64);

        // Extract fields: index_start (12) | index_end (12) | last_segment_size (12) | object_kind (4)
        let index_start = ((packed >> 28) & 0xFFF) as u16; // Bits 28-39 (12 bits)
        let index_end = ((packed >> 16) & 0xFFF) as u16; // Bits 16-27 (12 bits)
        let mut last_segment_size = ((packed >> 4) & 0xFFF) as u16; // Bits 4-15 (12 bits)
        let object_kind = (packed & 0xF) as u8; // Bits 0-3 (4 bits)

        // Calculate payload_length from segments
        let num_segments = if index_end > index_start {
            index_end - index_start
        } else {
            0
        };
        if num_segments > 0 && last_segment_size == 0 {
            last_segment_size = crate::constants::SEGMENT_SIZE as u16; // encoded full segment
        }
        let payload_length = Self::calculate_payload_length(num_segments, last_segment_size);

        Some(Self {
            work_package_hash,
            index_start,
            payload_length,
            object_kind,
        })
    }

    /// Serializes the ObjectRef to a byte vector
    /// Packs 40 bits: index_start (12) + index_end (12) + last_segment_size (12) + object_kind (4) into 5 bytes
    pub fn serialize(&self) -> Vec<u8> {
        let mut buffer = Vec::with_capacity(Self::SERIALIZED_SIZE);
        buffer.extend_from_slice(&self.work_package_hash); // 32 bytes

        // Calculate segments from payload_length
        let (num_segments, last_segment_size) =
            Self::calculate_segments_and_last_bytes(self.payload_length);
        let encoded_last_segment = if last_segment_size == crate::constants::SEGMENT_SIZE as u16 {
            0u16 // 0 encodes a full 4104-byte last segment to fit into 12 bits
        } else {
            last_segment_size
        };
        let index_end = self.index_start + num_segments;

        // Pack 40 bits: index_start (12) | index_end (12) | last_segment_size (12) | object_kind (4)
        let packed: u64 = ((self.index_start as u64 & 0xFFF) << 28) |   // Bits 28-39
                         ((index_end as u64 & 0xFFF) << 16) |           // Bits 16-27
                         ((encoded_last_segment as u64 & 0xFFF) << 4) | // Bits 4-15 (0 encodes full last segment)
                         (self.object_kind as u64 & 0xF); // Bits 0-3

        // Store as 5 bytes (40 bits)
        buffer.push((packed >> 32) as u8);
        buffer.push((packed >> 24) as u8);
        buffer.push((packed >> 16) as u8);
        buffer.push((packed >> 8) as u8);
        buffer.push(packed as u8);

        buffer
    }

    /// Serializes the ObjectRef into the provided buffer at the given position.
    /// Returns the number of bytes written (always SERIALIZED_SIZE).
    pub fn serialize_into(&self, buffer: &mut [u8], pos: usize) -> HarnessResult<usize> {
        if pos + Self::SERIALIZED_SIZE > buffer.len() {
            return Err(HarnessError::ParseError);
        }

        let mut offset = pos;
        buffer[offset..offset + 32].copy_from_slice(&self.work_package_hash);
        offset += 32;

        // Calculate segments from payload_length
        let (num_segments, last_segment_size) =
            Self::calculate_segments_and_last_bytes(self.payload_length);
        let encoded_last_segment = if last_segment_size == crate::constants::SEGMENT_SIZE as u16 {
            0u16 // 0 encodes a full 4104-byte last segment to fit into 12 bits
        } else {
            last_segment_size
        };
        let index_end = self.index_start + num_segments;

        // Pack 40 bits: index_start (12) | index_end (12) | last_segment_size (12) | object_kind (4)
        let packed: u64 = ((self.index_start as u64 & 0xFFF) << 28)
            | ((index_end as u64 & 0xFFF) << 16)
            | ((encoded_last_segment as u64 & 0xFFF) << 4)
            | (self.object_kind as u64 & 0xF);

        buffer[offset] = (packed >> 32) as u8;
        buffer[offset + 1] = (packed >> 24) as u8;
        buffer[offset + 2] = (packed >> 16) as u8;
        buffer[offset + 3] = (packed >> 8) as u8;
        buffer[offset + 4] = packed as u8;

        Ok(Self::SERIALIZED_SIZE)
    }

    /// Deserializes an [`ObjectRef`] from `data`, advancing `offset` by the number
    /// of bytes consumed.
    pub fn deserialize_from(data: &[u8], offset: &mut usize) -> HarnessResult<Self> {
        let end = offset.saturating_add(Self::SERIALIZED_SIZE);
        if end > data.len() {
            call_log(
                2,
                None,
                &format!(
                    "ObjectRef::deserialize_from: end={} > data.len()={}",
                    end,
                    data.len()
                ),
            );
            return Err(HarnessError::TruncatedInput);
        }

        let result = Self::deserialize(&data[*offset..end]).ok_or(HarnessError::ParseError)?;
        *offset = end;
        Ok(result)
    }

    /// Reads the current on-chain state for an object_id and returns an ObjectRef
    pub fn read(service_id: u32, object_id: &ObjectId) -> Option<Self> {
        // Use object_id directly as key for the read operation
        let key_buffer = *object_id;

        // Allocate buffer for the result
        let mut result_buffer = vec![0u8; Self::SERIALIZED_SIZE];

        unsafe {
            let _result_len = read(
                service_id as u64, // service index
                key_buffer.as_ptr() as u64,
                key_buffer.len() as u64,
                result_buffer.as_mut_ptr() as u64,
                FIRST_READABLE_ADDRESS as u64,
                result_buffer.len() as u64,
            );

            // Use deserialize to create ObjectRef
            Self::deserialize(&result_buffer)
        }
    }

    /// Fetches an object from DA using the fetch_object host function (index 254)
    ///
    /// This retrieves both the ObjectRef metadata and the full object payload in one call.
    /// Similar to fetch/lookup/historical_lookup but specifically for DA object retrieval.
    ///
    /// Parameters:
    /// - service_id: Service ID
    /// - object_id: Object ID (32 bytes)
    /// - max_payload_size: Maximum expected payload size
    ///
    /// Returns: Some((ObjectRef, payload)) on success, None on failure
    #[cfg(not(test))]
    pub fn fetch(
        service_id: u32,
        object_id: &ObjectId,
        max_payload_size: usize,
        payload_type: u8,
    ) -> Option<(Self, Vec<u8>)> {
        use crate::functions::{format_object_id, log_info};
        use crate::host_functions::fetch_object;

        // Do not fetch anything for Transactions
        if payload_type == 1 {
            return None;
        }

        let key_buffer = *object_id;

        log_info(&alloc::format!(
            "📡 fetch_object HOST CALL: service_id={}, object_id={:02x}{:02x}{:02x}{:02x}..., max_size={}",
            service_id,
            object_id[0], object_id[1], object_id[2], object_id[3],
            max_payload_size
        ));

        // Allocate buffer: ObjectRef (37 bytes) + payload
        let total_size = Self::SERIALIZED_SIZE + max_payload_size;
        let mut result_buffer = vec![0u8; total_size];

        unsafe {
            let result_len = fetch_object(
                service_id as u64,
                key_buffer.as_ptr() as u64,
                key_buffer.len() as u64,
                result_buffer.as_mut_ptr() as u64,
                FIRST_READABLE_ADDRESS as u64,
                result_buffer.len() as u64,
            );

            log_info(&alloc::format!(
                "📡 fetch_object RETURNED: payload_type={} result_len={} (0=not found)",
                payload_type,
                result_len
            ));

            if result_len == 0 {
                return None;
            }

            // Parse ObjectRef from first 37 bytes
            let object_ref = Self::deserialize(&result_buffer[0..Self::SERIALIZED_SIZE])?;

            // Extract payload (remaining bytes)
            let payload_len = (result_len as usize).saturating_sub(Self::SERIALIZED_SIZE);
            let mut payload = vec![0u8; payload_len];
            payload.copy_from_slice(
                &result_buffer[Self::SERIALIZED_SIZE..Self::SERIALIZED_SIZE + payload_len],
            );

            call_log(2, None, &format!("ObjectRef::fetch: object_id={}, service_id={}, max_payload={}, buffer={}, result_len={}, index={}, payload_length={}, actual_payload={}, wph={}",
                format_object_id(*object_id), service_id, max_payload_size, total_size, result_len, object_ref.index_start, object_ref.payload_length, payload_len, format_object_id(object_ref.work_package_hash)));
            if payload_len != object_ref.payload_length as usize {
                call_log(1, None, &format!("  WARNING: Payload length mismatch! Got {} bytes but ObjectRef.payload_length indicates {} bytes, wph={}",
                    payload_len, object_ref.payload_length, format_object_id(object_ref.work_package_hash)));
            }

            Some((object_ref, payload))
        }
    }

    /// Writes this ObjectRef state for the given object_id
    pub fn write(&self, object_id: &ObjectId, timeslot: u32, blocknumber: u32) -> u64 {
        use crate::host_functions::write;

        // Use object_id directly as key for the write operation
        let key_buffer = *object_id;

        // Serialize this ObjectRef to get the value buffer
        let mut value_buffer = self.serialize();

        // Add timeslot at the end (4 bytes, little-endian)
        value_buffer.extend_from_slice(&timeslot.to_le_bytes());
        // Add blocknumber at the end (4 bytes, little-endian)
        value_buffer.extend_from_slice(&blocknumber.to_le_bytes());

        unsafe {
            write(
                key_buffer.as_ptr() as u64,
                key_buffer.len() as u64,
                value_buffer.as_ptr() as u64,
                value_buffer.len() as u64,
            )
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

        // Extract path (remaining bytes, each hash is 32 bytes)
        let remaining_bytes = data.len() - cursor;
        let mut path = Vec::new();
        if remaining_bytes == 0 {
            return Ok(StateWitness {
                object_id,
                ref_info,
                path,
            });
        }

        let mut proof_bytes_total = remaining_bytes;
        if remaining_bytes >= 4 && (remaining_bytes - 4) % 32 == 0 {
            let declared = u32::from_le_bytes([
                data[cursor],
                data[cursor + 1],
                data[cursor + 2],
                data[cursor + 3],
            ]) as usize;
            cursor += 4;
            proof_bytes_total = data.len() - cursor;
            if declared * 32 != proof_bytes_total {
                return Err(HarnessError::ParseError);
            }
        } else if remaining_bytes % 32 != 0 {
            return Err(HarnessError::ParseError);
        }

        let path_count = proof_bytes_total / 32;
        path.reserve(path_count);
        for _ in 0..path_count {
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
    /// Uses the segment at index_start to extract the payload after metadata.
    /// Validates that the payload length equals the payload_length declared in the
    /// witness' ObjectRef.
    pub fn create_write_effect(&self, work_item_index: u16) -> Option<WriteEffectEntry> {
        let start = self.ref_info.index_start as u64;

        // Fetch segment bytes (expected to contain [ObjectId || ObjectRef || payload])
        let bytes = fetch_imported_segment(work_item_index, start as u32).ok()?;
        if bytes.is_empty() {
            return None;
        }

        // Skip metadata: object_id (32 bytes) + ObjectRef (35 bytes) = 67 bytes
        let metadata_size = 32 + ObjectRef::SERIALIZED_SIZE;
        if bytes.len() < metadata_size {
            return None;
        }
        let offset = metadata_size;

        // Extract payload from the remainder of the segment
        let mut payload = Vec::with_capacity(self.ref_info.payload_length as usize);
        payload.extend_from_slice(&bytes[offset..]);

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
            tx_index: 0,  // TODO: Track tx_index properly
        })
    }

    /// Verifies this state witness against a state_root using Merkle proof
    pub fn verify(&self, state_root: [u8; 32]) -> bool {
        use crate::effects::verify_merkle_proof;
        use crate::functions::format_object_id;

        call_log(
            3,
            None,
            &format!(
                "🔍 Verifying witness for object {}",
                format_object_id(self.object_id)
            ),
        );
        // Service ID removed from ObjectRef
        call_log(
            3,
            None,
            &format!("  State root: {}", format_object_id(state_root)),
        );
        call_log(
            3,
            None,
            &format!("  Proof path length: {}", self.path.len()),
        );

        // Serialize ObjectRef as the value for verification
        let value = self.ref_info.serialize();

        // Verify using JAM Merkle proof verification (service_id=0 since removed)
        let verified = verify_merkle_proof(
            0, // service_id removed from ObjectRef
            &self.object_id,
            &value,
            &state_root,
            &self.path,
        );

        if verified {
            call_log(
                2,
                None,
                &format!(
                    "✅ Witness verified for object {}",
                    format_object_id(self.object_id)
                ),
            );
        } else {
            call_log(
                1,
                None,
                &format!(
                    "❌ Witness verification FAILED for object {}",
                    format_object_id(self.object_id)
                ),
            );
            // Service ID removed from ObjectRef
            call_log(
                1,
                None,
                &format!("  Object ID: {}", format_object_id(self.object_id)),
            );
            call_log(
                1,
                None,
                &format!("  State root: {}", format_object_id(state_root)),
            );
            call_log(1, None, &format!("  Path length: {}", self.path.len()));
        }

        verified
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
