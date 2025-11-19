package seglog

// Segment represents metadata for a single segment file.
type Segment struct {
	// SegmentNum is the unique segment number.
	SegmentNum uint64

	// FirstRecordId is the first RecordId in this segment.
	FirstRecordId RecordId

	// LastRecordId is the last RecordId in this segment.
	// InvalidRecordId if segment is empty.
	LastRecordId RecordId

	// FilePath is the absolute path to the segment file.
	FilePath string

	// SizeBytes is the current size of the segment file in bytes.
	SizeBytes uint64
}

// NewSegment creates a new segment with the given segment number and file path.
func NewSegment(segmentNum uint64, filePath string) *Segment {
	return &Segment{
		SegmentNum:    segmentNum,
		FirstRecordId: InvalidRecordId,
		LastRecordId:  InvalidRecordId,
		FilePath:      filePath,
		SizeBytes:     0,
	}
}

// IsEmpty returns true if this segment contains no records.
func (s *Segment) IsEmpty() bool {
	return s.FirstRecordId == InvalidRecordId
}

// Contains returns true if this segment contains the given RecordId.
func (s *Segment) Contains(id RecordId) bool {
	if s.IsEmpty() {
		return false
	}
	return id >= s.FirstRecordId && id <= s.LastRecordId
}

// RecordCount returns the number of records in this segment.
func (s *Segment) RecordCount() uint64 {
	if s.IsEmpty() {
		return 0
	}
	return uint64(s.LastRecordId - s.FirstRecordId + 1)
}
