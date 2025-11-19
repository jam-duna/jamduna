package seglog

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// SegmentedLog is an append-only log with segment-based storage and pruning.
type SegmentedLog struct {
	mu sync.RWMutex

	// Configuration
	dir            string
	maxSegmentSize uint64

	// State
	nextRecordId RecordId
	segments     []*Segment
	headWriter   *SegmentFileWriter
}

// Config holds configuration for a SegmentedLog.
type Config struct {
	Dir            string
	MaxSegmentSize uint64
}

// NewSegmentedLog creates a new segmented log.
// It scans the directory for existing segments and recovers state.
func NewSegmentedLog(cfg Config) (*SegmentedLog, error) {
	if cfg.MaxSegmentSize == 0 {
		cfg.MaxSegmentSize = DEFAULT_SEGMENT_SIZE
	}
	if cfg.MaxSegmentSize < MIN_SEGMENT_SIZE {
		return nil, fmt.Errorf("max segment size %d is below minimum %d", cfg.MaxSegmentSize, MIN_SEGMENT_SIZE)
	}

	// Create directory if needed
	if err := os.MkdirAll(cfg.Dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	sl := &SegmentedLog{
		dir:            cfg.Dir,
		maxSegmentSize: cfg.MaxSegmentSize,
		nextRecordId:   RecordId(1), // Start from 1 (0 is invalid)
		segments:       make([]*Segment, 0),
	}

	// Recover existing segments
	if err := sl.recoverSegments(); err != nil {
		return nil, fmt.Errorf("failed to recover segments: %w", err)
	}

	// Create initial head segment if none exist
	if len(sl.segments) == 0 {
		if err := sl.rotateSegment(); err != nil {
			return nil, fmt.Errorf("failed to create initial segment: %w", err)
		}
	}

	return sl, nil
}

// recoverSegments scans the directory and recovers segment metadata.
func (sl *SegmentedLog) recoverSegments() error {
	entries, err := os.ReadDir(sl.dir)
	if err != nil {
		return fmt.Errorf("failed to read directory: %w", err)
	}

	// Find all segment files
	var segmentNums []uint64
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		segmentNum, ok := ParseSegmentFilename(entry.Name())
		if !ok {
			continue
		}
		segmentNums = append(segmentNums, segmentNum)
	}

	// Sort by segment number
	sort.Slice(segmentNums, func(i, j int) bool {
		return segmentNums[i] < segmentNums[j]
	})

	// Recover each segment
	for _, segmentNum := range segmentNums {
		filePath := filepath.Join(sl.dir, SegmentFilename(segmentNum))
		segment, err := sl.recoverSegment(segmentNum, filePath)
		if err != nil {
			return fmt.Errorf("failed to recover segment %d: %w", segmentNum, err)
		}
		sl.segments = append(sl.segments, segment)

		// Update next record ID
		if !segment.IsEmpty() && segment.LastRecordId >= sl.nextRecordId {
			sl.nextRecordId = segment.LastRecordId.Next()
		}
	}

	// Open head segment for appending
	// Note: Must be done AFTER recoverSegment completes any truncation
	if len(sl.segments) > 0 {
		headSegment := sl.segments[len(sl.segments)-1]
		writer, err := OpenSegmentFileWriter(headSegment.FilePath)
		if err != nil {
			return fmt.Errorf("failed to open head segment for writing: %w", err)
		}
		sl.headWriter = writer
	}

	return nil
}

// recoverSegment reads a segment file and builds its metadata.
func (sl *SegmentedLog) recoverSegment(segmentNum uint64, filePath string) (*Segment, error) {
	segment := NewSegment(segmentNum, filePath)

	// Get file size
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}
	segment.SizeBytes = uint64(info.Size())

	// Read records to find first and last RecordId
	reader, err := NewSegmentFileReader(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open for reading: %w", err)
	}
	defer reader.Close()

	lastValidOffset := uint64(0)
	needsTruncation := false
	for {
		recordId, _, err := reader.ReadRecord()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Tolerate partial writes at end of file (torn write from crash)
			// Truncate to last valid offset
			if segment.SizeBytes > lastValidOffset {
				segment.SizeBytes = lastValidOffset
				needsTruncation = true
			}
			break
		}

		if segment.FirstRecordId == InvalidRecordId {
			segment.FirstRecordId = recordId
		}
		segment.LastRecordId = recordId
		lastValidOffset = reader.Offset()
	}

	// Actually truncate the file if we detected corruption
	if needsTruncation {
		if err := os.Truncate(filePath, int64(lastValidOffset)); err != nil {
			return nil, fmt.Errorf("failed to truncate corrupt segment: %w", err)
		}
		segment.SizeBytes = lastValidOffset
	}

	return segment, nil
}

// Append appends a record to the log.
// Returns the RecordId assigned to the record.
func (sl *SegmentedLog) Append(payload []byte) (RecordId, error) {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	// Validate payload size
	if len(payload) > int(MAX_RECORD_PAYLOAD) {
		return InvalidRecordId, fmt.Errorf("payload size %d exceeds maximum %d", len(payload), MAX_RECORD_PAYLOAD)
	}

	// Check if we need to rotate segment
	recordSize := HEADER_SIZE + len(payload)
	alignedSize := ((recordSize + RECORD_ALIGNMENT - 1) / RECORD_ALIGNMENT) * RECORD_ALIGNMENT

	headSegment := sl.segments[len(sl.segments)-1]
	if headSegment.SizeBytes+uint64(alignedSize) > sl.maxSegmentSize {
		if err := sl.rotateSegment(); err != nil {
			return InvalidRecordId, fmt.Errorf("failed to rotate segment: %w", err)
		}
		headSegment = sl.segments[len(sl.segments)-1]
	}

	// Assign RecordId
	recordId := sl.nextRecordId
	sl.nextRecordId = sl.nextRecordId.Next()

	// Write record
	offset, err := sl.headWriter.WriteRecord(recordId, payload)
	if err != nil {
		return InvalidRecordId, fmt.Errorf("failed to write record: %w", err)
	}

	// Flush immediately to ensure read-your-write semantics
	if err := sl.headWriter.writer.Flush(); err != nil {
		return InvalidRecordId, fmt.Errorf("failed to flush after write: %w", err)
	}

	// Update segment metadata
	if headSegment.FirstRecordId == InvalidRecordId {
		headSegment.FirstRecordId = recordId
	}
	headSegment.LastRecordId = recordId
	headSegment.SizeBytes = offset + uint64(alignedSize)

	return recordId, nil
}

// Read reads a record by RecordId.
// Returns (payload, nil) if found, (nil, ErrRecordNotFound) if not found.
func (sl *SegmentedLog) Read(recordId RecordId) ([]byte, error) {
	sl.mu.RLock()
	defer sl.mu.RUnlock()

	// Find segment containing this RecordId
	segment := sl.findSegment(recordId)
	if segment == nil {
		return nil, ErrRecordNotFound
	}

	// Open reader
	reader, err := NewSegmentFileReader(segment.FilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open segment for reading: %w", err)
	}
	defer reader.Close()

	// Scan for the record
	for {
		rid, payload, err := reader.ReadRecord()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read record: %w", err)
		}
		if rid == recordId {
			return payload, nil
		}
	}

	return nil, ErrRecordNotFound
}

// findSegment finds the segment containing the given RecordId.
// Must be called with lock held.
func (sl *SegmentedLog) findSegment(recordId RecordId) *Segment {
	for _, segment := range sl.segments {
		if segment.Contains(recordId) {
			return segment
		}
	}
	return nil
}

// Sync flushes buffered writes to disk.
func (sl *SegmentedLog) Sync() error {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	if sl.headWriter != nil {
		if err := sl.headWriter.Sync(); err != nil {
			return fmt.Errorf("failed to sync head segment: %w", err)
		}
	}
	return nil
}

// rotateSegment closes the current head segment and creates a new one.
// Must be called with lock held.
func (sl *SegmentedLog) rotateSegment() error {
	// Close current head writer
	if sl.headWriter != nil {
		if err := sl.headWriter.Close(); err != nil {
			return fmt.Errorf("failed to close head segment: %w", err)
		}
		sl.headWriter = nil
	}

	// Create new segment
	var segmentNum uint64
	if len(sl.segments) > 0 {
		segmentNum = sl.segments[len(sl.segments)-1].SegmentNum + 1
	} else {
		segmentNum = 0
	}

	filePath := filepath.Join(sl.dir, SegmentFilename(segmentNum))
	writer, err := NewSegmentFileWriter(filePath)
	if err != nil {
		return fmt.Errorf("failed to create new segment: %w", err)
	}

	segment := NewSegment(segmentNum, filePath)
	sl.segments = append(sl.segments, segment)
	sl.headWriter = writer

	return nil
}

// Prune removes segments with RecordIds less than the given threshold.
// This allows freeing disk space for old log entries.
func (sl *SegmentedLog) Prune(beforeRecordId RecordId) error {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	// Find segments to prune
	pruneCount := 0
	for i, segment := range sl.segments {
		// Don't prune head segment
		if i == len(sl.segments)-1 {
			break
		}
		// Prune if entire segment is before threshold
		if !segment.IsEmpty() && segment.LastRecordId < beforeRecordId {
			pruneCount++
		} else {
			break
		}
	}

	if pruneCount == 0 {
		return nil
	}

	// Delete segment files
	for i := 0; i < pruneCount; i++ {
		segment := sl.segments[i]
		if err := os.Remove(segment.FilePath); err != nil {
			return fmt.Errorf("failed to remove segment %d: %w", segment.SegmentNum, err)
		}
	}

	// Update segments list
	sl.segments = sl.segments[pruneCount:]

	return nil
}

// Close closes the segmented log.
func (sl *SegmentedLog) Close() error {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	if sl.headWriter != nil {
		if err := sl.headWriter.Close(); err != nil {
			return fmt.Errorf("failed to close head segment: %w", err)
		}
		sl.headWriter = nil
	}

	return nil
}

// SegmentCount returns the number of segments in the log.
func (sl *SegmentedLog) SegmentCount() int {
	sl.mu.RLock()
	defer sl.mu.RUnlock()
	return len(sl.segments)
}

// NextRecordId returns the next RecordId that will be assigned.
func (sl *SegmentedLog) NextRecordId() RecordId {
	sl.mu.RLock()
	defer sl.mu.RUnlock()
	return sl.nextRecordId
}

// ErrRecordNotFound is returned when a record is not found.
var ErrRecordNotFound = fmt.Errorf("record not found")
