package types

import (
	"encoding/binary"
	"fmt"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
)

const (
	jamSegmentSize = 4104
)

type StateWitness struct {
	ServiceID   uint32        `json:"serviceId,omitempty"`
	ObjectID    common.Hash   `json:"objectId"`
	Ref         ObjectRef     `json:"ref"`
	BlockNumber uint32        `json:"blockNumber,omitempty"`
	Timeslot    uint32        `json:"timeslot,omitempty"`
	Path        []common.Hash `json:"path,omitempty"`
	Payload     []byte        `json:"-"` // Not serialized, populated when FetchJAMDASegments=true
	Value       []byte        `json:"-"` // Not serialized, populated when FetchJAMDASegments=true

	// Meta-shard proof chain (for objects stored in meta-shards)
	MetaShardMerkleRoot [32]byte                      `json:"metaShardMerkleRoot,omitempty"` // BMT root from meta-shard payload
	MetaShardPayload    []byte                        `json:"-"`                             // Meta-shard payload from DA (for verification)
	ObjectProofs        map[common.Hash][]common.Hash `json:"-"`                             // cache of per-object proofs in the meta-shard

	// Optimization: Cache of objects found in this meta-shard (populated on first access)
	ObjectRefs map[common.Hash]ObjectRef `json:"-"` // objectID => ObjectRef (parsed from MetaShardPayload)
	Payloads   map[common.Hash][]byte    `json:"-"` // objectID => object payload bytes (fetched from DA)
}

// ObjectRef contains metadata for a JAM service object
//
// Fields filled at REFINE time (deterministic):
// - work_package_hash, index_start, payload_length, object_kind
//
// This matches the Rust implementation in services/utils/src/objects.rs
// Serialized size: 37 bytes (32 + 5 packed 40 bits)
// Packed format: index_start (12 bits) | index_end (12 bits) | last_segment_size (12 bits) | object_kind (4 bits)
type ObjectRef struct {
	WorkPackageHash common.Hash `json:"workPackageHash"` // 32 bytes
	IndexStart      uint16      `json:"indexStart"`      // 2 bytes (packed into 12 bits)
	PayloadLength   uint32      `json:"payloadLength"`   // 4 bytes (calculated from num_segments + last_segment_size)
	ObjectKind      uint8       `json:"objectKind"`      // 1 byte (packed into 4 bits)
}

const (
	ObjectRefSerializedSize = 37
	// Note: SegmentSize is defined in const.go as 4104
	// Both Go and Rust now use SEGMENT_SIZE = 4104
)

// CalculateSegmentsAndLastBytes calculates num_segments and last_segment_bytes from payload_length
// Matches Rust implementation in services/utils/src/objects.rs
func CalculateSegmentsAndLastBytes(payloadLength uint32) (numSegments uint16, lastSegmentBytes uint16) {
	if payloadLength == 0 {
		return 0, 0
	}
	fullSegments := (payloadLength - 1) / SegmentSize
	lastSegmentBytes = uint16(payloadLength - (fullSegments * SegmentSize))
	numSegments = uint16(fullSegments + 1)
	return
}

// CalculatePayloadLength calculates payload_length from num_segments and last_segment_bytes
// Matches Rust implementation in services/utils/src/objects.rs
func CalculatePayloadLength(numSegments, lastSegmentBytes uint16) uint32 {
	if numSegments == 0 {
		return 0
	}
	if numSegments == 1 {
		return uint32(lastSegmentBytes)
	}
	return (uint32(numSegments)-1)*SegmentSize + uint32(lastSegmentBytes)
}

// Serialize serializes ObjectRef to 37 bytes matching Rust format:
// - work_package_hash (32 bytes)
// - packed 5 bytes (40 bits): index_start (12) | index_end (12) | last_segment_size (12) | object_kind (4)
func (o ObjectRef) Serialize() []byte {
	buf := make([]byte, ObjectRefSerializedSize)

	// Work package hash (32 bytes)
	copy(buf[0:32], o.WorkPackageHash[:])

	// Calculate segments from payload_length
	numSegments, lastSegmentSize := CalculateSegmentsAndLastBytes(o.PayloadLength)
	// A full last segment (4104 bytes) cannot fit in the 12-bit last_segment_size field.
	// Encode this case as 0 and expand back to SegmentSize during deserialization.
	encodedLastSegment := lastSegmentSize
	if lastSegmentSize == SegmentSize {
		encodedLastSegment = 0
	}
	indexEnd := o.IndexStart + numSegments

	// Pack 40 bits: index_start (12) | index_end (12) | last_segment_size (12) | object_kind (4)
	packed := (uint64(o.IndexStart&0xFFF) << 28) | // Bits 28-39
		(uint64(indexEnd&0xFFF) << 16) | // Bits 16-27
		(uint64(encodedLastSegment&0xFFF) << 4) | // Bits 4-15 (0 encodes a full last segment)
		uint64(o.ObjectKind&0xF) // Bits 0-3

	// Store as 5 bytes (40 bits)
	buf[32] = byte(packed >> 32)
	buf[33] = byte(packed >> 24)
	buf[34] = byte(packed >> 16)
	buf[35] = byte(packed >> 8)
	buf[36] = byte(packed)

	return buf
}

// DeserializeObjectRef deserializes ObjectRef from 37 bytes matching Rust format:
// - work_package_hash (32 bytes)
// - packed 5 bytes (40 bits): index_start (12) | index_end (12) | last_segment_size (12) | object_kind (4)
// could also return block number + timeslot (8 bytes)
func DeserializeObjectRef(data []byte, offset *int) (ObjectRef, error) {
	if len(data) < *offset+ObjectRefSerializedSize {
		return ObjectRef{}, fmt.Errorf("DeserializeObjectRef: need %d bytes at offset %d, have %d",
			ObjectRefSerializedSize, *offset, len(data)-*offset)
	}

	var ref ObjectRef

	// Work package hash (32 bytes)
	copy(ref.WorkPackageHash[:], data[*offset:*offset+32])

	// Unpack 5 bytes (40 bits): index_start (12) | index_end (12) | last_segment_size (12) | object_kind (4)
	packed := (uint64(data[*offset+32]) << 32) |
		(uint64(data[*offset+33]) << 24) |
		(uint64(data[*offset+34]) << 16) |
		(uint64(data[*offset+35]) << 8) |
		uint64(data[*offset+36])

	indexStart := uint16((packed >> 28) & 0xFFF)     // Bits 28-39 (12 bits)
	indexEnd := uint16((packed >> 16) & 0xFFF)       // Bits 16-27 (12 bits)
	lastSegmentSize := uint16((packed >> 4) & 0xFFF) // Bits 4-15 (12 bits)
	objectKind := uint8(packed & 0xF)                // Bits 0-3 (4 bits)

	// Calculate payload_length from segments
	var numSegments uint16
	if indexEnd > indexStart {
		numSegments = indexEnd - indexStart
	} else {
		numSegments = 0
	}

	// 0 encodes a full 4104-byte last segment; disambiguate from empty payload (numSegments==0)
	if numSegments > 0 && lastSegmentSize == 0 {
		lastSegmentSize = SegmentSize
	}

	ref.IndexStart = indexStart
	ref.PayloadLength = CalculatePayloadLength(numSegments, lastSegmentSize)
	ref.ObjectKind = objectKind
	log.Trace(log.SDB, "---- Deserialized ObjectRef", "workPackageHash", ref.WorkPackageHash,
		"packed", fmt.Sprintf("%x", packed),
		"indexStart", ref.IndexStart, "indexEnd", indexEnd,
		"lastSegmentSize", lastSegmentSize,
		"payloadLength", ref.PayloadLength,
		"objectKind", ref.ObjectKind)
	*offset += ObjectRefSerializedSize
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
	Effect       WriteEffectEntry `json:"effect"`
	Dependencies []common.Hash    `json:"dependencies,omitempty"`
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
		if candidate.RefInfo.ObjectKind != uint8(common.ObjectKindReceipt) && candidate.RefInfo.ObjectKind != uint8(common.ObjectKindBlockMetadata) {
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
// Format: ObjectID (32B) + ObjectRef (36B) + [optional payload for Receipt/BlockMetadata]
// Note: Dependencies are no longer serialized (matching Rust implementation)
// Returns: candidate, dependencies (empty), bytes_consumed, error
func DeserializeObjectCandidateWrite(data []byte, offset *int) (ObjectCandidateWrite, []common.Hash, int, error) {
	startOffset := *offset

	// ObjectID (32 bytes)
	if len(data) < *offset+32 {
		return ObjectCandidateWrite{}, nil, 0, fmt.Errorf("DeserializeObjectCandidateWrite: need 32 bytes for ObjectID at offset %d, have %d", *offset, len(data)-*offset)
	}
	var objectID common.Hash
	copy(objectID[:], data[*offset:*offset+32])
	*offset += 32

	// ObjectRef (37 bytes)
	refInfo, err := DeserializeObjectRef(data, offset)
	if err != nil {
		return ObjectCandidateWrite{}, nil, 0, fmt.Errorf("DeserializeObjectCandidateWrite: ObjectRef: %w", err)
	}

	// Dependencies are no longer serialized (matching Rust implementation)
	var dependencies []common.Hash

	// Read payload if present for Receipt and BlockMetadata objects (payload serialized inline after dependencies)
	var payload []byte
	if refInfo.ObjectKind == uint8(common.ObjectKindReceipt) || refInfo.ObjectKind == uint8(common.ObjectKindBlockMetadata) {
		payloadLen := int(refInfo.PayloadLength)
		if len(data) < *offset+payloadLen {
			return ObjectCandidateWrite{}, nil, 0, fmt.Errorf(
				"DeserializeObjectCandidateWrite: need %d bytes for object payload (kind=%d) at offset %d, have %d",
				payloadLen, refInfo.ObjectKind, *offset, len(data)-*offset,
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
	objectKind uint8,
	startIndex uint16,
) (StateWitness, [][]byte) {
	// First segment includes metadata: ObjectID (32) + ObjectRef (37) = 69 bytes
	const metadataSize = 69
	firstPayloadCapacity := jamSegmentSize - metadataSize
	if firstPayloadCapacity < 0 {
		firstPayloadCapacity = 0
	}

	// Build ObjectRef first to include in first segment
	ref := ObjectRef{
		WorkPackageHash: workPackageHash,
		IndexStart:      startIndex,
		PayloadLength:   uint32(len(payload)),
		ObjectKind:      objectKind,
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
	copy(firstSegment[32:69], ref.Serialize())
	copy(firstSegment[69:], remaining[:firstChunk])
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

	witness := StateWitness{
		ObjectID: objectID,
		Ref:      ref,
		Path:     []common.Hash{}, // TODO
	}
	return witness, segments
}

// SerializeWitness serializes a StateWitness to the fixed binary format expected by Rust
// Format: object_id (32 bytes) + value (meta-shard ObjectRef from JAM State) + proofs (32 bytes each)
func (w StateWitness) SerializeWitness() []byte {
	// Calculate total size: 32 + len(Value) + N*32
	size := 32 + len(w.Value) + (len(w.Path) * 32)
	buf := make([]byte, 0, size)

	// 1. ObjectID (32 bytes)
	buf = append(buf, w.ObjectID[:]...)

	// 2. Value (meta-shard ObjectRef bytes from JAM State - typically 45 bytes)
	buf = append(buf, w.Value...)

	// 3. Proof hashes (32 bytes each)
	for _, hash := range w.Path {
		buf = append(buf, hash[:]...)
	}

	return buf
}
