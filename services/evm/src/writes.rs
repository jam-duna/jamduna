//! ExecutionEffects serialization for refine → accumulate communication

use alloc::{format, vec::Vec};

/// Serialize ExecutionEffects to compact meta-shard format.
///
/// Format: N entries × (1B ld + prefix_bytes + 5B packed ObjectRef)
/// - Each entry has its own ld value (meta-shards can have different depths after splits)
/// - Only ObjectKind::MetaShard objects are serialized
///
/// For genesis with ld=0: N × 6 bytes (1B ld + 0B prefix + 5B ObjectRef)
pub fn serialize_execution_effects(
    effects: &utils::effects::ExecutionEffects,
) -> Vec<u8> {
    use crate::sharding::ObjectKind;
    use utils::functions::log_info;

    let mut buffer = Vec::new();

    // Serialize meta-shards with per-entry ld values
    // Format: N entries × (1B ld + prefix_bytes + 5B packed ObjectRef)
    // Note: After splits, different meta-shards can have different ld values!
    let mut meta_shard_count = 0;
    for intent in &effects.write_intents {
        if intent.effect.ref_info.object_kind == ObjectKind::MetaShard as u8 {
            // Extract ld from this specific meta-shard's object_id
            let ld = intent.effect.object_id[0];
            let prefix_bytes = ((ld + 7) / 8) as usize;

            // Write ld for this entry
            buffer.push(ld);

            // Write prefix bytes from meta-shard object_id (skip ld at position 0)
            if prefix_bytes > 0 {
                buffer.extend_from_slice(&intent.effect.object_id[1..1 + prefix_bytes]);
            }

            // Write 5-byte packed ObjectRef (no work_package_hash)
            let obj_ref = &intent.effect.ref_info;
            let (num_segments, last_segment_size) = {
                use utils::constants::SEGMENT_SIZE;
                let payload_length = obj_ref.payload_length;
                if payload_length == 0 {
                    (0u16, 0u16)
                } else {
                    let segment_size = SEGMENT_SIZE as u32;
                    let full_segments = (payload_length - 1) / segment_size;
                    let last_bytes = payload_length - (full_segments * segment_size);
                    let num_seg = full_segments + 1;
                    (num_seg as u16, last_bytes as u16)
                }
            };

            let index_end = obj_ref.index_start + num_segments;
            let packed: u64 = ((obj_ref.index_start as u64 & 0xFFF) << 28) |
                             ((index_end as u64 & 0xFFF) << 16) |
                             ((last_segment_size as u64 & 0xFFF) << 4) |
                             (obj_ref.object_kind as u64 & 0xF);

            buffer.push((packed >> 32) as u8);
            buffer.push((packed >> 24) as u8);
            buffer.push((packed >> 16) as u8);
            buffer.push((packed >> 8) as u8);
            buffer.push(packed as u8);

            meta_shard_count += 1;
        }
    }

    log_info(&format!(
        "📤 Serialized compact: {} meta-shards, {} bytes total",
        meta_shard_count,
        buffer.len()
    ));

    // Debug: log first 50 bytes
    if buffer.len() > 0 {
        let preview_len = buffer.len().min(50);
        let hex: alloc::string::String = buffer[..preview_len]
            .iter()
            .map(|b| format!("{:02x}", b))
            .collect::<alloc::vec::Vec<_>>()
            .join("");
        log_info(&format!("📤 Buffer preview (first {} bytes): {}", preview_len, hex));
    }

    buffer
}

