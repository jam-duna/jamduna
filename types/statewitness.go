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

type StateWitnessRaw struct {
	ObjectID      common.Hash   `json:"objectId"`
	ServiceID     uint32        `json:"serviceId"`
	Path          []common.Hash `json:"path,omitempty"`
	PayloadLength uint32        `json:"payloadLength"`
	Payload       []byte        `json:"-"` // Not serialized, populated when FetchJAMDASegments=true
}

// SerializeWitnessRaw serializes a StateWitnessRaw to binary format
// Format: format_byte (0) + object_id (32 bytes) + service_id (4 bytes LE) + payload_length (4 bytes LE) + payload (variable) + proofs (32 bytes each)
func (w StateWitnessRaw) SerializeWitnessRaw() []byte {
	// Calculate total size: 1 (format) + 32 (object_id) + 4 (service_id) + 4 (payload_length) + payload + proofs
	size := 1 + 32 + 4 + 4 + len(w.Payload) + (len(w.Path) * 32)
	buf := make([]byte, 0, size)

	// 0. Format byte (0 = RAW)
	buf = append(buf, 0)

	// 1. ObjectID (32 bytes)
	buf = append(buf, w.ObjectID[:]...)

	// 2. ServiceID (4 bytes LE)
	serviceIDBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(serviceIDBytes, w.ServiceID)
	buf = append(buf, serviceIDBytes...)

	// 3. PayloadLength (4 bytes LE)
	lengthBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(lengthBytes, w.PayloadLength)
	buf = append(buf, lengthBytes...)

	// 4. Payload (variable length)
	buf = append(buf, w.Payload...)

	// 5. Proof hashes (32 bytes each)
	for _, hash := range w.Path {
		buf = append(buf, hash[:]...)
	}

	return buf
}

// DeserializeStateWitnessRaw deserializes a StateWitnessRaw from binary format
// Format: format_byte (0) + object_id (32 bytes) + service_id (4 bytes LE) + payload_length (4 bytes LE) + payload (variable) + proofs (32 bytes each)
func DeserializeStateWitnessRaw(data []byte) (StateWitnessRaw, error) {
	const minSize = 1 + 32 + 4 + 4 // format_byte + object_id + service_id + payload_length
	if len(data) < minSize {
		return StateWitnessRaw{}, fmt.Errorf("DeserializeStateWitnessRaw: need at least %d bytes, got %d", minSize, len(data))
	}

	offset := 0

	// 0. Format byte (must be 0 for RAW)
	formatByte := data[offset]
	offset++
	if formatByte != 0 {
		return StateWitnessRaw{}, fmt.Errorf("DeserializeStateWitnessRaw: expected format byte 0, got %d", formatByte)
	}

	// 1. ObjectID (32 bytes)
	var objectID common.Hash
	copy(objectID[:], data[offset:offset+32])
	offset += 32

	// 2. ServiceID (4 bytes LE)
	serviceID := binary.LittleEndian.Uint32(data[offset : offset+4])
	offset += 4

	// 3. PayloadLength (4 bytes LE)
	payloadLength := binary.LittleEndian.Uint32(data[offset : offset+4])
	offset += 4

	// 4. Payload (variable length)
	if len(data) < offset+int(payloadLength) {
		return StateWitnessRaw{}, fmt.Errorf("DeserializeStateWitnessRaw: need %d bytes for payload at offset %d, got %d", payloadLength, offset, len(data)-offset)
	}
	payload := make([]byte, payloadLength)
	copy(payload, data[offset:offset+int(payloadLength)])
	offset += int(payloadLength)

	// 5. Proof hashes (32 bytes each)
	remaining := len(data) - offset
	if remaining%32 != 0 {
		return StateWitnessRaw{}, fmt.Errorf("DeserializeStateWitnessRaw: proof data not aligned to 32 bytes (remaining=%d)", remaining)
	}
	proofCount := remaining / 32
	path := make([]common.Hash, proofCount)
	for i := 0; i < proofCount; i++ {
		copy(path[i][:], data[offset:offset+32])
		offset += 32
	}

	return StateWitnessRaw{
		ObjectID:      objectID,
		ServiceID:     serviceID,
		Path:          path,
		PayloadLength: payloadLength,
		Payload:       payload,
	}, nil
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
	LogIndex uint8  `json:"logIndex"` // Reserved/padding (1 byte)
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
	buf = append(buf, o.LogIndex)                // 1 byte
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
		// New field order: ObjKind, LogIndex, TxSlot, Timeslot, GasUsed, EvmBlock
		ObjKind:  data[*offset+48],
		LogIndex: data[*offset+49],
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
	WriteIntents []WriteIntent `json:"writeIntents,omitempty"`
}

// DeserializeExecutionEffects deserializes ExecutionEffects from refine output.
// Modern format: export_count (2B) | gas_used (8B) | count (2B) | ObjectCandidateWrite entries.
// Supports payload data for Receipt and Block objects only.
func DeserializeExecutionEffects(data []byte) (ExecutionEffects, error) {
	if len(data) == 0 {
		return ExecutionEffects{
			WriteIntents: []WriteIntent{},
		}, nil
	}

	offset := 0

	// Count (2 bytes) - mandatory
	if len(data) < 2 {
		return ExecutionEffects{}, fmt.Errorf("DeserializeExecutionEffects: need >= 2 bytes for count, got %d", len(data))
	}
	count := binary.LittleEndian.Uint16(data[0:2])
	offset = 2

	// State root is no longer included in the serialized format

	writeIntents := make([]WriteIntent, 0, count)
	for i := uint16(0); i < count; i++ {
		candidate, deps, _, err := DeserializeObjectCandidateWrite(data, &offset)
		if err != nil {
			return ExecutionEffects{}, fmt.Errorf("DeserializeExecutionEffects: candidate %d: %w", i, err)
		}

		// Retain inline payload for receipt and block metadata objects (used by accumulate).
		// All other object kinds should not carry inline payload in ExecutionEffects.
		payload := candidate.Payload
		if candidate.RefInfo.ObjKind != uint8(common.ObjectKindReceipt) && candidate.RefInfo.ObjKind != uint8(common.ObjectKindBlockMetadata) {
			payload = nil
		}

		writeIntents = append(writeIntents, WriteIntent{
			Effect: WriteEffectEntry{
				ObjectID: candidate.ObjectID,
				RefInfo:  candidate.RefInfo,
				Payload:  payload,
			},
			Dependencies: deps,
		})
	}

	return ExecutionEffects{
		WriteIntents: writeIntents,
	}, nil
}

// ObjectCandidateWrite represents metadata-only object information from refine
type ObjectCandidateWrite struct {
	ObjectID common.Hash
	RefInfo  ObjectRef
	Payload  []byte // Payload data for Receipt objects only
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

	// Read payload if present for Receipt and BlockMetadata objects (payload serialized inline after dependencies)
	var payload []byte
	if refInfo.ObjKind == uint8(common.ObjectKindReceipt) || refInfo.ObjKind == uint8(common.ObjectKindBlockMetadata) {
		payloadLen := int(refInfo.PayloadLength)
		if len(data) < *offset+payloadLen {
			return ObjectCandidateWrite{}, nil, 0, fmt.Errorf(
				"DeserializeObjectCandidateWrite: need %d bytes for object payload (kind=%d) at offset %d, have %d",
				payloadLen, refInfo.ObjKind, *offset, len(data)-*offset,
			)
		}
		payload = make([]byte, payloadLen)
		copy(payload, data[*offset:*offset+payloadLen])
		*offset += payloadLen
	}

	candidate := ObjectCandidateWrite{
		ObjectID: objectID,
		RefInfo:  refInfo,
		Payload:  payload,
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
// Format: format_byte (1) + object_id (32 bytes) + object_ref (64 bytes) + proofs (32 bytes each, no length prefix)
func (w StateWitness) SerializeWitness() []byte {
	// Calculate total size
	size := 1 + 32 + 64 + (len(w.Path) * 32)
	buf := make([]byte, 0, size)

	// 0. Format byte (1 = REF)
	buf = append(buf, 1)

	// 1. ObjectID (32 bytes)
	buf = append(buf, w.ObjectID[:]...)

	// 2. ObjectRef (64 bytes) - use existing Serialize method
	buf = append(buf, w.Ref.Serialize()...)

	// 3. Proof hashes (32 bytes each)
	for _, hash := range w.Path {
		buf = append(buf, hash[:]...)
	}

	return buf
}
