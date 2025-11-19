package seglog

import (
	"encoding/binary"
	"fmt"
)

// RecordId is a unique identifier for a record in the segmented log.
// RecordIds are monotonically increasing and never reused.
type RecordId uint64

// InvalidRecordId represents an invalid/uninitialized record ID.
const InvalidRecordId RecordId = 0

// Next returns the next RecordId in sequence.
func (r RecordId) Next() RecordId {
	return r + 1
}

// Prev returns the previous RecordId in sequence.
// Panics if called on InvalidRecordId.
func (r RecordId) Prev() RecordId {
	if r == InvalidRecordId {
		panic("cannot get previous of InvalidRecordId")
	}
	return r - 1
}

// IsValid returns true if this is a valid RecordId (non-zero).
func (r RecordId) IsValid() bool {
	return r != InvalidRecordId
}

// Bytes returns the little-endian byte representation of this RecordId.
func (r RecordId) Bytes() []byte {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, uint64(r))
	return buf
}

// RecordIdFromBytes decodes a RecordId from little-endian bytes.
func RecordIdFromBytes(b []byte) (RecordId, error) {
	if len(b) != 8 {
		return InvalidRecordId, fmt.Errorf("RecordId must be 8 bytes, got %d", len(b))
	}
	return RecordId(binary.LittleEndian.Uint64(b)), nil
}

// String returns a string representation of this RecordId.
func (r RecordId) String() string {
	if !r.IsValid() {
		return "RecordId(invalid)"
	}
	return fmt.Sprintf("RecordId(%d)", uint64(r))
}

// Compare compares two RecordIds.
// Returns -1 if r < other, 0 if r == other, 1 if r > other.
func (r RecordId) Compare(other RecordId) int {
	if r < other {
		return -1
	} else if r > other {
		return 1
	}
	return 0
}
