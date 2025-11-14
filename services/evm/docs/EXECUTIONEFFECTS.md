# ExecutionEffects Serialization

This document describes how ExecutionEffects are serialized and deserialized between the Rust EVM service (refine phase) and Go accumulator (accumulate phase).

## Overview

The ExecutionEffects format is used to communicate write operations from the refine phase to the accumulate phase. The format has evolved to be more compact and flexible, supporting both modern and legacy services.

## Binary Format

### Header (Variable Length)

The serialization format uses a variable-length header to support backward compatibility:

```
export_count (2B) | [gas_used (8B)] | [count (2B)] | ObjectCandidateWrite entries...
```

#### Field Details

1. **export_count** (2 bytes, LE): Number of objects exported to DA
   - Always present
   - Required field for all services

2. **gas_used** (8 bytes, LE): Total gas consumed during execution
   - Optional field for backward compatibility
   - Present if payload length >= 10 bytes
   - Defaults to 0 for legacy services

3. **count** (2 bytes, LE): Number of ObjectCandidateWrite entries
   - Optional field for backward compatibility
   - Present if payload length >= header + 2 bytes
   - Defaults to 0 for legacy services

### ObjectCandidateWrite Format

Each ObjectCandidateWrite entry contains:

```
object_id (32B) | object_ref (64B) | dep_count (2B) | dependencies (36B each) | [payload (variable)]
```

#### Field Details

1. **object_id** (32 bytes): Unique identifier for the DA object
2. **object_ref** (64 bytes): ObjectRef metadata structure
3. **dep_count** (2 bytes, LE): Number of dependencies
4. **dependencies** (36 bytes each): Array of ObjectDependency entries
   - object_id (32 bytes): Dependency object ID
   - required_version (4 bytes, LE): Required version
5. **payload** (variable length): Optional payload data
   - Present only for receipt objects (`ObjectKind::Receipt`)
   - Block intents do not carry payload in `ObjectCandidateWrite`
   - Length determined by `object_ref.payload_length`

## Rust Implementation (services/evm/src/writes.rs)

### Serialization

The `serialize_execution_effects()` function creates the binary format:

```rust
pub fn serialize_execution_effects(effects: &ExecutionEffects) -> Vec<u8> {
    let mut buffer = Vec::new();

    // Header
    buffer.extend_from_slice(&effects.export_count.to_le_bytes());
    buffer.extend_from_slice(&effects.gas_used.to_le_bytes());

    let count = writes.len() as u16;
    buffer.extend_from_slice(&count.to_le_bytes());

    // ObjectCandidateWrite entries
    for write in writes {
        let serialized = write.serialize();
        buffer.extend_from_slice(&serialized);
    }

    buffer
}
```

### Deserialization

The `deserialize_execution_effects()` function requires the full modern header:

```rust
pub fn deserialize_execution_effects(data: &[u8]) -> Option<ExecutionEffectsEnvelope> {
    // Export count (mandatory)
    if data.len() < 2 {
        return None;
    }
    let export_count = u16::from_le_bytes([data[0], data[1]]);
    let mut offset = 2;

    // Gas used (mandatory)
    if data.len() < 10 {
        return None;
    }
    let gas_used = u64::from_le_bytes([data[2], data[3], data[4], data[5],
                                     data[6], data[7], data[8], data[9]]);
    offset = 10;

    // Count (mandatory)
    if data.len() < offset + 2 {
        return None;
    }
    let count = u16::from_le_bytes([data[offset], data[offset + 1]]) as usize;
    offset += 2;

    // Deserialize ObjectCandidateWrite entries...
}
```

## Go Implementation (types/statewitness.go)

### Deserialization

The `DeserializeExecutionEffects()` function requires the full modern header:

```go
func DeserializeExecutionEffects(data []byte) (ExecutionEffects, error) {
    if len(data) == 0 {
        return ExecutionEffects{WriteIntents: []WriteIntent{}}, nil
    }

    offset := 0

    // Export count (mandatory)
    if len(data) < 2 {
        return ExecutionEffects{}, fmt.Errorf("need >= 2 bytes, got %d", len(data))
    }
    exportCount := binary.LittleEndian.Uint16(data[0:2])
    offset = 2

    // Gas used (mandatory)
    if len(data) < 10 {
        return ExecutionEffects{}, fmt.Errorf("need >= 10 bytes for gas_used, got %d", len(data))
    }
    gasUsed := binary.LittleEndian.Uint64(data[2:10])
    offset = 10

    // Count (mandatory)
    if len(data) < offset+2 {
        return ExecutionEffects{}, fmt.Errorf("need >= %d bytes for count, got %d", offset+2, len(data))
    }
    count := binary.LittleEndian.Uint16(data[offset : offset+2])
    offset += 2

    // Deserialize ObjectCandidateWrite entries...
}
```

### ObjectCandidateWrite Deserialization

```go
func DeserializeObjectCandidateWrite(data []byte, offset *int) (ObjectCandidateWrite, []ObjectDependency, int, error) {
    // ObjectID (32 bytes)
    var objectID common.Hash
    copy(objectID[:], data[*offset:*offset+32])
    *offset += 32

    // ObjectRef (64 bytes)
    refInfo, err := DeserializeObjectRef(data, offset)
    if err != nil {
        return ObjectCandidateWrite{}, nil, 0, err
    }

    // Dependencies count (2 bytes)
    depCount := binary.LittleEndian.Uint16(data[*offset : *offset+2])
    *offset += 2

    // Dependencies (36 bytes each)
    dependencies := make([]ObjectDependency, depCount)
    for i := uint16(0); i < depCount; i++ {
        var depObjectID common.Hash
        copy(depObjectID[:], data[*offset:*offset+32])
        *offset += 32

        requiredVersion := binary.LittleEndian.Uint32(data[*offset : *offset+4])
        *offset += 4

        dependencies[i] = ObjectDependency{
            ObjectID:        depObjectID,
            RequiredVersion: requiredVersion,
        }
    }

    // Read payload if present for Receipt objects only
    var payload []byte
    if refInfo.ObjKind == uint8(common.ObjectKindReceipt) {
        payloadLen := int(refInfo.PayloadLength)
        if len(data) < *offset+payloadLen {
            return ObjectCandidateWrite{}, nil, 0, fmt.Errorf("insufficient payload data for receipt object")
        }
        payload = make([]byte, payloadLen)
        copy(payload, data[*offset:*offset+payloadLen])
        *offset += payloadLen
    }

    // Return candidate and dependencies...
}
```

## Format Requirements

The ExecutionEffects format requires the modern payload structure:

- **Modern payload** (12+ bytes): Full header with all required fields
  - export_count (2 bytes, mandatory)
  - gas_used (8 bytes, mandatory)
  - count (2 bytes, mandatory)
  - ObjectCandidateWrite entries (variable length)

### Error Handling

Both Rust and Go implementations validate the complete payload structure:

- **Empty payload**: Returns empty ExecutionEffects with no writes
- **Insufficient data**: Returns detailed error messages with context
- **Malformed entries**: Returns detailed error messages with context

## Object Payload Handling

Certain object types require special handling due to their embedded payload:

### Receipt Objects (ObjectKind::Receipt = 0x03)

1. **Refine Output**: Receipts append their receipt payload after dependencies
2. **Deserialization**: Payload length is read from ObjectRef.payload_length
3. **JAM State Storage**: Values start with ObjectRef followed by the receipt payload data
4. **StateWitness Reading**: Go ReadStateWitness extracts receipt payload directly from JAM State (no DA fetch needed)
5. **Accumulate Phase**: Receipt payloads are used for log extraction and block metadata

### Block Objects (ObjectKind::Block = 0x05)

1. **Refine Output**: Block payloads stored to DA during refine
2. **ObjectID Pattern**: `0xFF` repeated 28 times + block_number (4 bytes LE)
3. **JAM State Storage**: Values contain the complete EvmBlockPayload WITHOUT ObjectRef prefix
4. **Content Structure**: Contains serialized EvmBlockPayload with:
   - Fixed header (432 bytes): number, parent_hash, logs_bloom, roots, gas_used, timestamp, etc.
   - Variable data: tx_hashes[], receipt_hashes[]
5. **StateWitness Reading**: Go ReadStateWitness puts EvmBlockPayload directly in Payload field
6. **Accumulate Phase**: Block payloads written via `EvmBlockPayload::write()`, stored at block_number ObjectID

### Block Metadata Objects (ObjectKind::BlockMetadata = 0x06)

**NEW**: Introduced for automatic metadata computation from block witnesses

1. **Refine Output**: Created when block witness detected in PayloadTransactions
   - Detection: Check if witness object_id matches pattern `0xFF...FF[block_number:4 LE]`
   - Helper: `is_block_number_state_witness(object_id) -> Option<u32>`
2. **Computation**: Deserialize EvmBlockPayload from witness, compute:
   - `transactions_root`: BMT root of tx_hashes
   - `receipts_root`: BMT root of receipt_hashes
   - `mmr_root`: MMR root from accumulated receipts
3. **ObjectID**: `blake2b_hash(block.serialize_header()[0..HEADER_SIZE])` where HEADER_SIZE = 432
4. **Payload Format**: 96 bytes total (32 bytes × 3)
   ```
   transactions_root (32) || receipts_root (32) || mmr_root (32)
   ```
5. **Storage**:
   - **Rust Write**: Uses `write()` host function directly to service storage (NOT DA export)
   - **Key**: Block hash (computed from 432-byte header via Blake2b-256)
   - **Value**: 96-byte serialized EvmBlockMetadata
   - **Critical**: Metadata written to service storage, NOT exported to DA
6. **Accumulate Phase**: Metadata written via `EvmBlockMetadata::write(&candidate.object_id)`
   - `candidate.object_id` is the block hash computed in refine
   - Stored as simple key-value in service storage
7. **Go Read**: Uses `ReadServiceStorage(serviceID, blockHash)` to retrieve metadata
   - **NOT `ReadStateWitnessRaw()`** - that function has special handling for blocks/receipts
   - Reads raw 96-byte value directly from service storage
8. **Purpose**: Enables fast block metadata queries without DA fetch

**Block Witness Flow**:
```
Timeslot N (Accumulate):
  - Block N finalized (contains all tx/receipt hashes)
  - Block N written to DA (object_id = 0xFF...FF[N])

Timeslot N+1 (Refine PayloadTransactions):
  - Witness for block N included in extrinsics
  - is_block_number_state_witness() detects block witness
  - Deserialize EvmBlockPayload from witness
  - Compute EvmBlockMetadata (roots)
  - Create WriteIntent for metadata
  - Transactions execute normally

Timeslot N+1 (Accumulate):
  - Block N metadata written to DA
  - Block N+1 created (not yet finalized)
```

**Implementation References**:
- Detection: `services/evm/src/refiner.rs:770-816`
- Helper: `services/evm/src/block.rs:37-56`
- Metadata: `services/evm/src/block.rs` (EvmBlockMetadata struct)

### Key Differences

- **Receipt objects**: JAM State = `ObjectRef + receipt_payload`
- **Block objects**: JAM State = `EvmBlockPayload` (no ObjectRef prefix)
- **Block metadata objects**: JAM State = `EvmBlockMetadata` (computed from block witness)

## Error Recovery

### Common Issues

1. **"data too short for X bytes"**: Insufficient data for expected field
   - Check payload length vs. expected header size
   - Verify ObjectCandidateWrite entry boundaries

2. **"DeserializeObjectRef failed"**: Corrupted ObjectRef data
   - Ensure 64-byte ObjectRef boundary alignment
   - Check endianness and field ordering

3. **"need X bytes for Y deps"**: Dependency array overflow
   - Verify dep_count field accuracy
   - Check for offset calculation errors

### Debugging

Enable detailed logging to trace deserialization:

```rust
log_error(&format!("deserialize_execution_effects: insufficient data for header (len={})", data.len()));
log_error(&format!("failed at candidate #{}, offset {}, remaining {} bytes", i, offset, remaining));
```

```go
fmt.Errorf("DeserializeExecutionEffects: candidate %d: %w", i, err)
```

## Performance Considerations

1. **Zero-copy deserialization**: Data is read directly from byte slices
2. **Minimal allocations**: Pre-sized vectors and slices based on count fields
3. **Early validation**: Header validation before processing entries
4. **Streaming**: ObjectCandidateWrite entries processed sequentially

## Future Extensions

The variable-length header design allows for future extensions:

1. **Additional metadata**: New fields can be added after count
2. **Version indicators**: Header can include format version information
3. **Compression**: Payload can be compressed with appropriate headers
4. **Type safety**: Stronger typing for ObjectKind and dependency validation
