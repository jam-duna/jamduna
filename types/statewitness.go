package types

import (
	"encoding/binary"
	"fmt"

	"github.com/colorfulnotion/jam/common"
)

const (
	jamSegmentSize = 4104
)

type StateWitness struct {
	ObjectID common.Hash   `json:"objectId"`
	Ref      ObjectRef     `json:"ref"`
	Path     []common.Hash `json:"path,omitempty"`
	Payload  []byte        `json:"-"` // Not serialized, populated when FetchJAMDASegments=true
}

// ObjectRef contains metadata for a JAM service object
//
// Fields filled at REFINE time (deterministic):
// - ServiceID, WorkPackageHash, IndexStart, IndexEnd, Version, PayloadLength
//
// Fields filled at ACCUMULATE time (ordering-dependent):
// - Timeslot: JAM block slot (maps 1:1 to canonical block hash)
// - GasUsed: Gas consumed (non-zero for receipts)
// - EvmBlock: Sequential Ethereum block number
// - TxIndex: Position within block (lower 24 bits)
// - ObjectKind: ObjectKind enum discriminant (replaces HashSuffix hack)
type ObjectRef struct {
	// Refine-time fields (deterministic at work package creation)
	ServiceID       uint32      `json:"serviceId"`
	WorkPackageHash common.Hash `json:"workPackageHash"`
	IndexStart      uint16      `json:"indexStart"`
	IndexEnd        uint16      `json:"indexEnd"`
	Version         uint32      `json:"version"`
	PayloadLength   uint32      `json:"payloadLength"`
	ObjKind         uint8       `json:"objectKind"` // ObjectKind (1 byte)

	// Accumulate-time fields (finalized ordering metadata)
	TxSlot2  uint8  `json:"txSlot2"`  // Reserved/padding (1 byte)
	TxSlot   uint16 `json:"txSlot"`   // Transaction slot/index (2 bytes)
	Timeslot uint32 `json:"timeslot"` // JAM block slot; maps 1:1 to canonical block hash
	GasUsed  uint32 `json:"gasUsed"`  // Non-zero for receipt objects
	EvmBlock uint32 `json:"evmBlock"` // Sequential Ethereum block number
}

// TxIndex extracts tx_index from TxSlot field
func (o ObjectRef) TxIndex() uint32 {
	return uint32(o.TxSlot)
}

// ObjectKind extracts ObjectKind from ObjKind field
func (o ObjectRef) ObjectKind() uint32 {
	return uint32(o.ObjKind)
}

func (o ObjectRef) Serialize() []byte {
	buf := make([]byte, 0, 64)

	four := make([]byte, 4)
	binary.LittleEndian.PutUint32(four, o.ServiceID)
	buf = append(buf, four...)

	buf = append(buf, o.WorkPackageHash[:]...)

	tmp := make([]byte, 2)
	binary.LittleEndian.PutUint16(tmp, o.IndexStart)
	buf = append(buf, tmp...)

	binary.LittleEndian.PutUint16(tmp, o.IndexEnd)
	buf = append(buf, tmp...)

	binary.LittleEndian.PutUint32(four, o.Version)
	buf = append(buf, four...)

	binary.LittleEndian.PutUint32(four, o.PayloadLength)
	buf = append(buf, four...)

	buf = append(buf, o.ObjKind)                 // 1 byte
	buf = append(buf, o.TxSlot2)                 // 1 byte
	binary.LittleEndian.PutUint16(tmp, o.TxSlot) // 2 bytes
	buf = append(buf, tmp...)

	binary.LittleEndian.PutUint32(four, o.Timeslot)
	buf = append(buf, four...)

	binary.LittleEndian.PutUint32(four, o.GasUsed)
	buf = append(buf, four...)

	binary.LittleEndian.PutUint32(four, o.EvmBlock)
	buf = append(buf, four...)

	return buf
}

func DeserializeObjectRef(data []byte, offset *int) (ObjectRef, error) {
	if len(data) < *offset+64 {
		return ObjectRef{}, fmt.Errorf("DeserializeObjectRef: truncated input at offset %d", *offset)
	}
	// Layout: service_id(4) + work_package_hash(32) + index_start(2) + index_end(2) +
	//         version(4) + payload_length(4) + obj_kind(1) + tx_slot_2(1) + tx_slot(2) +
	//         timeslot(4) + gas_used(4) + evm_block(4) = 64 bytes
	ref := ObjectRef{
		ServiceID:     binary.LittleEndian.Uint32(data[*offset : *offset+4]),
		IndexStart:    binary.LittleEndian.Uint16(data[*offset+36 : *offset+38]),
		IndexEnd:      binary.LittleEndian.Uint16(data[*offset+38 : *offset+40]),
		Version:       binary.LittleEndian.Uint32(data[*offset+40 : *offset+44]),
		PayloadLength: binary.LittleEndian.Uint32(data[*offset+44 : *offset+48]),
		// New field order: ObjKind, TxSlot2, TxSlot, Timeslot, GasUsed, EvmBlock
		ObjKind:  data[*offset+48],
		TxSlot2:  data[*offset+49],
		TxSlot:   binary.LittleEndian.Uint16(data[*offset+50 : *offset+52]),
		Timeslot: binary.LittleEndian.Uint32(data[*offset+52 : *offset+56]),
		GasUsed:  binary.LittleEndian.Uint32(data[*offset+56 : *offset+60]),
		EvmBlock: binary.LittleEndian.Uint32(data[*offset+60 : *offset+64]),
	}
	copy(ref.WorkPackageHash[:], data[*offset+4:*offset+36])
	*offset += 64
	return ref, nil
}

// WriteEffectEntry represents a DA object write with metadata and optional payload.
// In modern refine→accumulate flow, payload is nil (metadata-only).
type WriteEffectEntry struct {
	ObjectID common.Hash `json:"objectId"`
	RefInfo  ObjectRef   `json:"refInfo"`
	Payload  []byte      `json:"payload,omitempty"` // nil for ObjectCandidateWrite format
}

type WriteIntent struct {
	Effect       WriteEffectEntry   `json:"effect"`
	Dependencies []ObjectDependency `json:"dependencies,omitempty"`
}

type ObjectDependency struct {
	ObjectID        common.Hash `json:"objectId"`
	RequiredVersion uint32      `json:"requiredVersion"`
}

type ExecutionEffects struct {
	ExportCount  uint16        `json:"exportCount"`
	GasUsed      uint64        `json:"gasUsed"`
	WriteIntents []WriteIntent `json:"writeIntents,omitempty"`
}

// DeserializeExecutionEffects deserializes ExecutionEffects from refine output.
// Format: export_count (2B) | gas_used (8B) | tx_count (2B) | count (2B) | ObjectCandidateWrite entries (metadata-only, no payloads).
func DeserializeExecutionEffects(data []byte) (ExecutionEffects, error) {
	// Phase 1B format (current): export_count (2B) | gas_used (8B) | tx_count (2B) | count (2B) | ObjectCandidateWrites...
	if len(data) < 14 {
		return ExecutionEffects{}, fmt.Errorf("DeserializeExecutionEffects: data too short (need >= 14, got %d)", len(data))
	}

	exportCount := binary.LittleEndian.Uint16(data[0:2])
	gasUsed := binary.LittleEndian.Uint64(data[2:10])
	txCount := binary.LittleEndian.Uint16(data[10:12])

	// ObjectCandidateWrite array format: 2-byte count
	count := binary.LittleEndian.Uint16(data[12:14])
	offset := 14

	writeIntents := make([]WriteIntent, 0, count)
	for i := uint16(0); i < count; i++ {
		candidate, deps, _, err := DeserializeObjectCandidateWrite(data, &offset)
		if err != nil {
			return ExecutionEffects{}, fmt.Errorf("DeserializeExecutionEffects: candidate %d: %w", i, err)
		}

		writeIntents = append(writeIntents, WriteIntent{
			Effect: WriteEffectEntry{
				ObjectID: candidate.ObjectID,
				RefInfo:  candidate.RefInfo,
				Payload:  nil, // ObjectCandidateWrite doesn't include payloads
			},
			Dependencies: deps,
		})
	}

	_ = txCount // txCount available for future use

	return ExecutionEffects{
		ExportCount:  exportCount,
		GasUsed:      gasUsed,
		WriteIntents: writeIntents,
	}, nil
}

// ObjectCandidateWrite represents metadata-only object information from refine
type ObjectCandidateWrite struct {
	ObjectID common.Hash
	RefInfo  ObjectRef
}

// DeserializeObjectCandidateWrite deserializes a single ObjectCandidateWrite entry
// Format: ObjectID (32B) + ObjectRef (64B) + dep_count (2B) + dependencies (36B each)
// Returns: candidate, dependencies, bytes_consumed, error
func DeserializeObjectCandidateWrite(data []byte, offset *int) (ObjectCandidateWrite, []ObjectDependency, int, error) {
	startOffset := *offset

	// ObjectID (32 bytes)
	if len(data) < *offset+32 {
		return ObjectCandidateWrite{}, nil, 0, fmt.Errorf("DeserializeObjectCandidateWrite: need 32 bytes for ObjectID at offset %d, have %d", *offset, len(data)-*offset)
	}
	var objectID common.Hash
	copy(objectID[:], data[*offset:*offset+32])
	*offset += 32

	// ObjectRef (64 bytes)
	refInfo, err := DeserializeObjectRef(data, offset)
	if err != nil {
		return ObjectCandidateWrite{}, nil, 0, fmt.Errorf("DeserializeObjectCandidateWrite: ObjectRef: %w", err)
	}

	// Dependencies count (2 bytes)
	if len(data) < *offset+2 {
		return ObjectCandidateWrite{}, nil, 0, fmt.Errorf("DeserializeObjectCandidateWrite: need 2 bytes for dep_count at offset %d, have %d", *offset, len(data)-*offset)
	}
	depCount := binary.LittleEndian.Uint16(data[*offset : *offset+2])
	*offset += 2

	// Dependencies (36 bytes each: 32B object_id + 4B version)
	if len(data) < *offset+int(depCount)*36 {
		return ObjectCandidateWrite{}, nil, 0, fmt.Errorf("DeserializeObjectCandidateWrite: need %d bytes for %d deps at offset %d, have %d",
			int(depCount)*36, depCount, *offset, len(data)-*offset)
	}

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

	candidate := ObjectCandidateWrite{
		ObjectID: objectID,
		RefInfo:  refInfo,
	}

	consumed := *offset - startOffset
	return candidate, dependencies, consumed, nil
}

func CreateImportSegmentsAndWitness(
	objectID common.Hash,
	workPackageHash common.Hash,
	payload []byte,
	serviceID uint32,
	startIndex uint16,
	version uint32,
) (StateWitness, [][]byte) {
	// First segment includes metadata: ObjectID (32) + ObjectRef (64) = 96 bytes
	const metadataSize = 96
	firstPayloadCapacity := jamSegmentSize - metadataSize
	if firstPayloadCapacity < 0 {
		firstPayloadCapacity = 0
	}

	// Build ObjectRef first to include in first segment
	ref := ObjectRef{
		ServiceID:       serviceID,
		WorkPackageHash: workPackageHash,
		IndexStart:      startIndex,
		IndexEnd:        0, // Will be updated after counting segments
		Version:         version,
		PayloadLength:   uint32(len(payload)),
	}

	var segments [][]byte
	remaining := payload

	// First segment: [ObjectID || ObjectRef || payload_chunk]
	firstChunk := firstPayloadCapacity
	if firstChunk > len(remaining) {
		firstChunk = len(remaining)
	}
	firstSegment := make([]byte, metadataSize+firstChunk)
	copy(firstSegment[0:32], objectID[:])
	copy(firstSegment[32:96], ref.Serialize())
	copy(firstSegment[96:], remaining[:firstChunk])
	segments = append(segments, firstSegment)
	remaining = remaining[firstChunk:]

	// Subsequent segments: payload-only
	for len(remaining) > 0 {
		chunk := jamSegmentSize
		if chunk > len(remaining) {
			chunk = len(remaining)
		}
		segment := make([]byte, chunk)
		copy(segment, remaining[:chunk])
		segments = append(segments, segment)
		remaining = remaining[chunk:]
	}

	// Update IndexEnd now that we know segment count
	ref.IndexEnd = startIndex + uint16(len(segments))

	witness := StateWitness{
		ObjectID: objectID,
		Ref:      ref,
		Path:     []common.Hash{}, // TODO
	}
	return witness, segments
}

// SerializeWitness serializes a StateWitness to the fixed binary format expected by Rust
// Format: object_id (32 bytes) + object_ref (64 bytes) + proof_count (4 bytes LE) + proofs (32 bytes each)
func (w StateWitness) SerializeWitness() []byte {
	// Calculate total size
	size := 32 + 64 + 4 + (len(w.Path) * 32)
	buf := make([]byte, 0, size)

	// 1. ObjectID (32 bytes)
	buf = append(buf, w.ObjectID[:]...)

	// 2. ObjectRef (64 bytes) - use existing Serialize method
	buf = append(buf, w.Ref.Serialize()...)

	// 3. Proof count (4 bytes, little endian)
	proofCount := uint32(len(w.Path))
	buf = append(buf, byte(proofCount), byte(proofCount>>8), byte(proofCount>>16), byte(proofCount>>24))

	// 4. Proof hashes (32 bytes each)
	for _, hash := range w.Path {
		buf = append(buf, hash[:]...)
	}

	return buf
}
