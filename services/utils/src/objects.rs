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
    pub service_id: u32,             // Service ID (4 bytes)
    pub work_package_hash: [u8; 32], // Work package hash (32 bytes)
    pub index_start: u16,            // Starting index (2 bytes)
    pub index_end: u16,              // Ending index (2 bytes)
    pub version: u32,                // Version (4 bytes)
    pub payload_length: u32,         // Payload length (4 bytes)

    // Accumulate-time fields (finalized ordering metadata)
    pub object_kind: u8, // ObjectKind (1 byte)
    pub log_index: u8,   // Reserved/padding (1 byte)
    pub tx_slot: u16,    // Transaction slot/index (2 bytes)
    pub timeslot: u32,   // JAM block slot; maps 1:1 to canonical block hash (4 bytes)
    pub gas_used: u32,   // Non-zero for receipt objects (4 bytes)
    pub evm_block: u32,  // Sequential Ethereum block number (4 bytes)
}

impl Default for ObjectRef {
    fn default() -> Self {
        Self {
            service_id: 0,
            work_package_hash: [0u8; 32],
            index_start: 0,
            index_end: 0,
            version: 0,
            payload_length: 0,
            object_kind: 0,
            log_index: 0,
            tx_slot: 0,
            timeslot: 0,
            gas_used: 0,
            evm_block: 0,
        }
    }
}

impl ObjectRef {
    pub const SERIALIZED_SIZE: usize = 64;

    /// Creates a new ObjectRef with refine-time fields set and accumulate-time fields zeroed
    ///
    /// Refine-time fields (deterministic):
    /// - service_id, work_package_hash, version, payload_length, object_kind, tx_slot
    ///
    /// Accumulate-time fields (set to 0, filled later by runtime):
    /// - index_start, index_end, log_index, timeslot, gas_used, evm_block
    pub fn new(
        service_id: u32,
        work_package_hash: [u8; 32],
        version: u32,
        payload_length: u32,
        object_kind: u8,
        tx_slot: u16,
    ) -> Self {
        Self {
            service_id,
            work_package_hash,
            index_start: 0,
            index_end: 0,
            version,
            payload_length,
            object_kind,
            log_index: 0,
            tx_slot,
            timeslot: 0,
            gas_used: 0,
            evm_block: 0,
        }
    }

    /// Extracts tx_index from tx_slot field
    pub fn tx_index(&self) -> u32 {
        self.tx_slot as u32
    }

    /// Extracts ObjectKind from object_kind field
    pub fn object_kind(&self) -> u32 {
        self.object_kind as u32
    }

    /// Sets the object_kind and tx_slot fields
    pub fn set_object_kind_and_tx_index(&mut self, object_kind: u8, tx_index: u16) {
        self.object_kind = object_kind;
        self.tx_slot = tx_index;
    }

    /// Deserializes an ObjectRef from a byte buffer - opposite of serialize
    pub fn deserialize(data: &[u8]) -> Option<Self> {
        if data.len() != Self::SERIALIZED_SIZE {
            return None;
        }

        let mut offset = 0;

        let service_id = u32::from_le_bytes(data[offset..offset + 4].try_into().ok()?);
        offset += 4;

        let mut work_package_hash = [0u8; 32];
        work_package_hash.copy_from_slice(&data[offset..offset + 32]);
        offset += 32;

        let index_start = u16::from_le_bytes(data[offset..offset + 2].try_into().ok()?);
        offset += 2;
        let index_end = u16::from_le_bytes(data[offset..offset + 2].try_into().ok()?);
        offset += 2;
        let version = u32::from_le_bytes(data[offset..offset + 4].try_into().ok()?);
        offset += 4;
        let payload_length = u32::from_le_bytes(data[offset..offset + 4].try_into().ok()?);
        offset += 4;

        // New field order: object_kind, log_index, tx_slot, timeslot, gas_used, evm_block
        let object_kind = data[offset];
        offset += 1;
        let log_index = data[offset];
        offset += 1;
        let tx_slot = u16::from_le_bytes(data[offset..offset + 2].try_into().ok()?);
        offset += 2;
        let timeslot = u32::from_le_bytes(data[offset..offset + 4].try_into().ok()?);
        offset += 4;
        let gas_used = u32::from_le_bytes(data[offset..offset + 4].try_into().ok()?);
        offset += 4;
        let evm_block = u32::from_le_bytes(data[offset..offset + 4].try_into().ok()?);

        Some(Self {
            service_id,
            work_package_hash,
            index_start,
            index_end,
            version,
            payload_length,
            object_kind,
            log_index,
            tx_slot,
            timeslot,
            gas_used,
            evm_block,
        })
    }

    /// Serializes the ObjectRef to a byte vector
    pub fn serialize(&self) -> Vec<u8> {
        let mut buffer = Vec::with_capacity(Self::SERIALIZED_SIZE);
        buffer.extend_from_slice(&self.service_id.to_le_bytes()); // 4 bytes
        buffer.extend_from_slice(&self.work_package_hash); // 32 bytes
        buffer.extend_from_slice(&self.index_start.to_le_bytes()); // 2 bytes
        buffer.extend_from_slice(&self.index_end.to_le_bytes()); // 2 bytes
        buffer.extend_from_slice(&self.version.to_le_bytes()); // 4 bytes
        buffer.extend_from_slice(&self.payload_length.to_le_bytes()); // 4 bytes
                                                                      // New field order: object_kind, log_index, tx_slot, timeslot, gas_used, evm_block
        buffer.push(self.object_kind); // 1 byte
        buffer.push(self.log_index); // 1 byte
        buffer.extend_from_slice(&self.tx_slot.to_le_bytes()); // 2 bytes
        buffer.extend_from_slice(&self.timeslot.to_le_bytes()); // 4 bytes
        buffer.extend_from_slice(&self.gas_used.to_le_bytes()); // 4 bytes
        buffer.extend_from_slice(&self.evm_block.to_le_bytes()); // 4 bytes
        buffer
    }

    /// Serializes the ObjectRef into the provided buffer at the given position.
    /// Returns the number of bytes written (always SERIALIZED_SIZE).
    pub fn serialize_into(&self, buffer: &mut [u8], pos: usize) -> HarnessResult<usize> {
        if pos + Self::SERIALIZED_SIZE > buffer.len() {
            return Err(HarnessError::ParseError);
        }

        let mut offset = pos;
        buffer[offset..offset + 4].copy_from_slice(&self.service_id.to_le_bytes());
        offset += 4;
        buffer[offset..offset + 32].copy_from_slice(&self.work_package_hash);
        offset += 32;
        buffer[offset..offset + 2].copy_from_slice(&self.index_start.to_le_bytes());
        offset += 2;
        buffer[offset..offset + 2].copy_from_slice(&self.index_end.to_le_bytes());
        offset += 2;
        buffer[offset..offset + 4].copy_from_slice(&self.version.to_le_bytes());
        offset += 4;
        buffer[offset..offset + 4].copy_from_slice(&self.payload_length.to_le_bytes());
        offset += 4;
        // New field order: object_kind, log_index, tx_slot, timeslot, gas_used, evm_block
        buffer[offset] = self.object_kind;
        offset += 1;
        buffer[offset] = self.log_index;
        offset += 1;
        buffer[offset..offset + 2].copy_from_slice(&self.tx_slot.to_le_bytes());
        offset += 2;
        buffer[offset..offset + 4].copy_from_slice(&self.timeslot.to_le_bytes());
        offset += 4;
        buffer[offset..offset + 4].copy_from_slice(&self.gas_used.to_le_bytes());
        offset += 4;
        buffer[offset..offset + 4].copy_from_slice(&self.evm_block.to_le_bytes());

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
    pub fn fetch(
        service_id: u32,
        object_id: &ObjectId,
        max_payload_size: usize,
    ) -> Option<(Self, Vec<u8>)> {
        use crate::functions::format_object_id;
        use crate::host_functions::fetch_object;

        let key_buffer = *object_id;

        // Allocate buffer: ObjectRef (64 bytes) + payload
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

            if result_len == 0 {
                // Object not found -- this is normal eg for EOA accounts you wont find code!
                return None;
            }

            // Parse ObjectRef from first 64 bytes
            let object_ref = Self::deserialize(&result_buffer[0..Self::SERIALIZED_SIZE])?;

            // Extract payload (remaining bytes)
            let payload_len = (result_len as usize).saturating_sub(Self::SERIALIZED_SIZE);
            let mut payload = vec![0u8; payload_len];
            payload.copy_from_slice(
                &result_buffer[Self::SERIALIZED_SIZE..Self::SERIALIZED_SIZE + payload_len],
            );

            call_log(2, None, &format!("ObjectRef::fetch: object_id={}, service_id={}, max_payload={}, buffer={}, result_len={}, version={}, index={}..{}, payload_length={}, actual_payload={}",
                format_object_id(*object_id), service_id, max_payload_size, total_size, result_len, object_ref.version, object_ref.index_start, object_ref.index_end, object_ref.payload_length, payload_len));
            if payload_len != object_ref.payload_length as usize {
                call_log(1, None, &format!("  WARNING: Payload length mismatch! Got {} bytes but ObjectRef.payload_length indicates {} bytes",
                    payload_len, object_ref.payload_length));
            }

            Some((object_ref, payload))
        }
    }

    /// Writes this ObjectRef state for the given object_id
    pub fn write(&self, object_id: &ObjectId) -> u64 {
        use crate::host_functions::write;

        // Use object_id directly as key for the write operation
        let key_buffer = *object_id;

        // Serialize this ObjectRef to get the value buffer
        let value_buffer = self.serialize();

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
    /// Uses the first segment (at index_start) to extract the initial payload chunk after
    /// metadata. Appends payload from subsequent segments up to but not including index_end.
    /// Validates that the final payload length equals the payload_length declared in the
    /// witness' ObjectRef.
    pub fn create_write_effect(&self, work_item_index: u16) -> Option<WriteEffectEntry> {
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

        // Skip metadata: object_id (32 bytes) + ObjectRef (68 bytes) = 100 bytes
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
        call_log(
            3,
            None,
            &format!("  Service ID: {}", self.ref_info.service_id),
        );
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

        // Verify using JAM Merkle proof verification
        let verified = verify_merkle_proof(
            self.ref_info.service_id,
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
            call_log(
                1,
                None,
                &format!("  Service ID: {}", self.ref_info.service_id),
            );
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
