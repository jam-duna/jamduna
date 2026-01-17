//! ExecutionEffects serialization for refine → accumulate communication

use alloc::{vec::Vec, format};

/// Serialize ExecutionEffects to refine output.
///
/// Format:
/// 1. Meta-shards (OLD compact format): [2B count][N entries × (1B ld + prefix_bytes + 5B packed ObjectRef)]
/// 2. Contract witness metadata: [2B index_start][4B total_payload_length]
///    - Points to segment range containing serialized contract storage/code blobs
///    - Each blob in segments: [20B address][1B kind][4B payload_length][...payload...]
/// 3. Block receipts metadata: [1B has_receipts][if 1: 2B index_start][4B payload_length]
///    - Points to segment range containing block receipts blob
///    - Blob format: [4B count][receipt_len:4][receipt_data]...
/// 4. AccumulateInstructions (appended in main.rs)
///
/// This allows:
/// - Accumulate to parse meta-shards (section 1)
/// - Go side to read contract storage/code from segments (section 2)
/// - Go side to extract receipts from segments (section 3)
pub fn serialize_execution_effects(
    effects: &utils::effects::ExecutionEffects,
    contract_witness_index_start: u16,
    contract_witness_payload_length: u32,
) -> Vec<u8> {
    use crate::contractsharding::ObjectKind;
    use crate::meta_sharding::parse_meta_shard_object_id;
    use utils::functions::log_info;

    log_info(&format!(
        "📤 serialize_execution_effects: Processing {} write intents",
        effects.write_intents.len()
    ));

    let mut buffer = Vec::new();

    // Start with number of meta-shard entries (2 bytes)
    let metashard_count: u16 = effects.write_intents
        .iter()
        .filter(|intent| intent.effect.ref_info.object_kind == ObjectKind::MetaShard as u8)
        .count() as u16;

    buffer.extend_from_slice(&metashard_count.to_le_bytes());

    log_info(&format!(
        "📤 Refine: Writing {} meta-shard entries to compact format",
        metashard_count
    ));

    // Section 1: Serialize meta-shards in OLD compact format for accumulate
    let mut actual_metashard_count = 0;
    for intent in &effects.write_intents {
        if intent.effect.ref_info.object_kind == ObjectKind::MetaShard as u8 {
            actual_metashard_count += 1;

            // Extract ld and prefix56 from meta-shard object_id
            let (ld, prefix56) = parse_meta_shard_object_id(&intent.effect.object_id);
            let prefix_bytes = ((ld + 7) / 8) as usize;

            // Write ld for this entry
            buffer.push(ld);

            // Write prefix bytes from prefix56
            if prefix_bytes > 0 {
                buffer.extend_from_slice(&prefix56[..prefix_bytes]);
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
            let packed: u64 = ((obj_ref.index_start as u64 & 0xFFF) << 28)
                | ((index_end as u64 & 0xFFF) << 16)
                | ((last_segment_size as u64 & 0xFFF) << 4)
                | (obj_ref.object_kind as u64 & 0xF);

            buffer.push((packed >> 32) as u8);
            buffer.push((packed >> 24) as u8);
            buffer.push((packed >> 16) as u8);
            buffer.push((packed >> 8) as u8);
            buffer.push(packed as u8);
        }
    }

    log_info(&format!(
        "📤 Section 1: Serialized {} meta-shards in compact format",
        actual_metashard_count
    ));

    // Section 2: Contract witness metadata (ONE pair for ALL contract storage/code)
    // Write [2B index_start][4B total_payload_length]
    buffer.extend_from_slice(&contract_witness_index_start.to_le_bytes());
    buffer.extend_from_slice(&contract_witness_payload_length.to_le_bytes());

    if contract_witness_payload_length > 0 {
        log_info(&format!(
            "📤 Section 2: Contract witness metadata - index_start={}, total_length={} bytes",
            contract_witness_index_start, contract_witness_payload_length
        ));
    } else {
        log_info("📤 Section 2: No contract storage/code objects");
    }

    // Section 3: Block receipts metadata
    // Find the block receipts write intent (keyed by BlockCommitment with 0xFE prefix)
    // Filter by both ObjectKind::Receipt AND object_id[0] == 0xFE to distinguish
    // block-scoped receipts from any future per-tx receipt objects
    let receipt_intent = effects.write_intents
        .iter()
        .find(|intent| {
            intent.effect.ref_info.object_kind == ObjectKind::Receipt as u8
                && intent.effect.object_id[0] == 0xFE
        });

    if let Some(intent) = receipt_intent {
        let obj_ref = &intent.effect.ref_info;

        // has_receipts = 1
        buffer.push(1u8);

        // index_start (2 bytes)
        buffer.extend_from_slice(&obj_ref.index_start.to_le_bytes());

        // payload_length (4 bytes)
        buffer.extend_from_slice(&obj_ref.payload_length.to_le_bytes());

        log_info(&format!(
            "📤 Section 3: Block receipts metadata - index_start={}, payload_length={} bytes",
            obj_ref.index_start, obj_ref.payload_length
        ));
    } else {
        // has_receipts = 0
        buffer.push(0u8);
        log_info("📤 Section 3: No block receipts");
    }

    log_info(&format!(
        "📤 serialize_execution_effects: Total buffer size: {} bytes",
        buffer.len()
    ));

    buffer
}
