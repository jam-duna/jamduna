// New unpacked deserialize function
pub fn deserialize(data: &[u8]) -> Option<Self> {
    if data.len() != Self::SERIALIZED_SIZE {
        return None;
    }

    let mut offset = 0;

    // Work package hash (32 bytes)
    let mut work_package_hash = [0u8; 32];
    work_package_hash.copy_from_slice(&data[offset..offset + 32]);
    offset += 32;

    // index_start (2 bytes, little-endian)
    let index_start = u16::from_le_bytes([data[offset], data[offset + 1]]);
    offset += 2;

    // index_end (2 bytes, little-endian)
    let index_end = u16::from_le_bytes([data[offset], data[offset + 1]]);
    offset += 2;

    // last_segment_bytes (2 bytes, little-endian)
    let last_segment_bytes = u16::from_le_bytes([data[offset], data[offset + 1]]);
    offset += 2;

    // Calculate num_segments and payload_length
    let num_segments = if index_end > index_start {
        index_end - index_start
    } else {
        0
    };
    let payload_length = Self::calculate_payload_length(num_segments, last_segment_bytes);

    // Read object_kind (1 byte)
    let object_kind = data[offset];

    Some(Self {
        work_package_hash,
        index_start,
        payload_length,
        object_kind,
    })
}

// New unpacked serialize function
pub fn serialize(&self) -> Vec<u8> {
    let mut buffer = Vec::with_capacity(Self::SERIALIZED_SIZE);

    // Work package hash (32 bytes)
    buffer.extend_from_slice(&self.work_package_hash);

    // Calculate segments from payload_length
    let (num_segments, last_segment_bytes) = Self::calculate_segments_and_last_bytes(self.payload_length);

    // index_start (2 bytes, little-endian)
    buffer.extend_from_slice(&self.index_start.to_le_bytes());

    // index_end (2 bytes, little-endian)
    let index_end = self.index_start + num_segments;
    buffer.extend_from_slice(&index_end.to_le_bytes());

    // last_segment_bytes (2 bytes, little-endian)
    buffer.extend_from_slice(&last_segment_bytes.to_le_bytes());

    // object_kind (1 byte)
    buffer.push(self.object_kind);

    buffer
}
