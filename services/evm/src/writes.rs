//! ObjectCandidateWrite and ExecutionEffects serialization for refine → accumulate communication

use crate::sharding::{format_object_id, ObjectKind};
use alloc::{format, vec::Vec};
use utils::effects::ObjectDependency;
use utils::functions::{log_error, log_debug, log_trace};
use utils::objects::ObjectRef;

/// ObjectCandidateWrite for refine → accumulate communication
/// Contains ObjectID, ObjectRef, and dependencies without payload
#[derive(Debug, Clone)]
pub struct ObjectCandidateWrite {
    pub object_id: [u8; 32],
    pub object_ref: ObjectRef,
    pub dependencies: Vec<ObjectDependency>,
    pub payload: Vec<u8>, // only used for receipt 
}

impl ObjectCandidateWrite {
    /// Serialize ObjectCandidateWrite to bytes
    /// Format: object_id (32B) + object_ref (variable) + dep_count (2B) + dependencies
    pub fn serialize(&self) -> Vec<u8> {
        let mut buffer = Vec::new();

        // Object ID (32 bytes)
        buffer.extend_from_slice(&self.object_id);

        // Object ref (variable length)
        let ref_bytes = self.object_ref.serialize();
        buffer.extend_from_slice(&ref_bytes);

        // Dependencies count (2 bytes)
        let dep_count = self.dependencies.len() as u16;
        buffer.extend_from_slice(&dep_count.to_le_bytes());

        // Dependencies (12 bytes each: 32B object_id + 4B version)
        for dep in &self.dependencies {
            buffer.extend_from_slice(&dep.object_id);
            buffer.extend_from_slice(&dep.required_version.to_le_bytes());
        }

        // Serialize payload for receipt and block metadata objects (other payloads are handled separately)
        if self.object_ref.object_kind == ObjectKind::Receipt as u8 || self.object_ref.object_kind == ObjectKind::BlockMetadata as u8 {
            buffer.extend_from_slice(&self.payload);
        }
        buffer
    }

    /// Deserialize ObjectCandidateWrite from bytes
    pub fn deserialize(data: &[u8]) -> Option<(Self, usize)> {
        if data.len() < 32 {
            log_error(&format!(
                "    ❌ ObjectCandidateWrite::deserialize: data too short for object_id (need >= 32, have {})",
                data.len()
            ));
            return None;
        }

        let mut offset = 0;

        // Object ID (32 bytes)
        let mut object_id = [0u8; 32];
        object_id.copy_from_slice(&data[offset..offset + 32]);
        offset += 32;

        // Object ref (fixed 64 bytes)

        // ObjectRef::deserialize requires exactly 64 bytes
        if data.len() < offset + ObjectRef::SERIALIZED_SIZE {
            log_error(&format!(
                "    ❌ Not enough data for ObjectRef (need offset+64={}, have {})",
                offset + 64,
                data.len()
            ));
            return None;
        }

        let object_ref = match ObjectRef::deserialize(
            &data[offset..offset + ObjectRef::SERIALIZED_SIZE],
        ) {
            Some(r) => {
                log_debug(&format!(
                    "    🔬 data={} bytes, object_id={}, ObjectRef@{} → service_id={}, version={}, kind={}, payload_len={}",
                    data.len(),
                    format_object_id(&object_id),
                    offset,
                    r.service_id,
                    r.version,
                    r.object_kind,
                    r.payload_length
                ));
                r
            }
            None => {
                log_error(&format!("    ❌ ObjectRef::deserialize failed at offset {}", offset));
                return None;
            }
        };
        offset += ObjectRef::SERIALIZED_SIZE;

        // Dependencies count (2 bytes)
        if data.len() < offset + 2 {
            log_error(&format!(
                "    ❌ Not enough data for dep_count (need offset+2={}, have {})",
                offset + 2,
                data.len()
            ));
            return None;
        }
        let dep_count = u16::from_le_bytes([data[offset], data[offset + 1]]) as usize;
        offset += 2;

        // Dependencies (36 bytes each)
        if data.len() < offset + dep_count * 36 {
            log_error(&format!(
                "    ❌ Not enough data for deps (need {}={} + {}*36, have {})",
                offset + dep_count * 36,
                offset,
                dep_count,
                data.len()
            ));
            return None;
        }

        let mut dependencies = Vec::with_capacity(dep_count);
        for i in 0..dep_count {
            let mut dep_object_id = [0u8; 32];
            dep_object_id.copy_from_slice(&data[offset..offset + 32]);
            offset += 32;

            let required_version = u32::from_le_bytes([
                data[offset],
                data[offset + 1],
                data[offset + 2],
                data[offset + 3],
            ]);
            offset += 4;

            log_trace(&format!(
                "    🔗 Dep #{}: object_id={}, version={}",
                i,
                format_object_id(&dep_object_id),
                required_version
            ));

            dependencies.push(ObjectDependency {
                object_id: dep_object_id,
                required_version,
            });
        }

        // Deserialize payload for receipt and block metadata objects
        let payload = if object_ref.object_kind == ObjectKind::Receipt as u8 || object_ref.object_kind == ObjectKind::BlockMetadata as u8 {
            let payload_len = object_ref.payload_length as usize;
            if data.len() < offset + payload_len {
                log_error(&format!(
                    "    ❌ Not enough data for object payload (kind={}, need {}, have {})",
                    object_ref.object_kind,
                    offset + payload_len,
                    data.len()
                ));
                return None;
            }
            let payload_data = data[offset..offset + payload_len].to_vec();
            offset += payload_len;
            payload_data
        } else {
            Vec::new()
        };

        log_debug(&format!(
            "    📥 ObjectCandidateWrite parsed: kind={}, declared_payload_len={}, actual_payload_len={}",
            object_ref.object_kind,
            object_ref.payload_length,
            payload.len()
        ));

        Some((
            ObjectCandidateWrite {
                object_id,
                object_ref,
                dependencies,
                payload,
            },
            offset,
        ))
    }
}

/// Compact representation of ExecutionEffects exchanged between refine and accumulate.
pub struct ExecutionEffectsEnvelope {
    pub export_count: u16,
    pub gas_used: u64,
    pub writes: Vec<ObjectCandidateWrite>,
}

/// Serialize ExecutionEffects metadata and candidate writes.
/// Format: count (2B) | ObjectCandidateWrite entries
pub fn serialize_execution_effects(
    effects: &utils::effects::ExecutionEffects,
) -> Vec<u8> {

    use crate::state::MajikBackend;

    // Convert ExecutionEffects to ObjectCandidateWrite array (without payloads)
    let writes = MajikBackend::to_object_candidate_writes(effects);


    let mut buffer = Vec::new();

    let count = writes.len() as u16;
    buffer.extend_from_slice(&count.to_le_bytes());

    for write in writes {
        let serialized = write.serialize();
        buffer.extend_from_slice(&serialized);
    }

    buffer
}

/// Deserialize ExecutionEffects envelope from refine output buffer.
pub fn deserialize_execution_effects(data: &[u8]) -> Option<ExecutionEffectsEnvelope> {
    if data.is_empty() {
        return Some(ExecutionEffectsEnvelope {
            export_count: 0,
            gas_used: 0,
            writes: Vec::new(),
        });
    }

    // Write count (2 bytes) – mandatory
    if data.len() < 2 {
        log_error("❌ deserialize_execution_effects: data too short for count");
        return None;
    }
    let count = u16::from_le_bytes([data[0], data[1]]) as usize;
    let mut offset = 2;

    let mut writes = Vec::with_capacity(count);
    for i in 0..count {
        if let Some((write, size)) = ObjectCandidateWrite::deserialize(&data[offset..]) {
            writes.push(write);
            offset += size;
        } else {
            log_error(&format!(
                "❌ deserialize_execution_effects: failed at candidate #{}, offset {}, remaining {} bytes",
                i,
                offset,
                data.len().saturating_sub(offset)
            ));
            return None;
        }
    }

    Some(ExecutionEffectsEnvelope {
        export_count: 0,
        gas_used: 0,
        writes,
    })
}
