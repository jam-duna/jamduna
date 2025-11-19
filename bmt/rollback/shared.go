package rollback

import (
	"io"
	"sync"

	"github.com/colorfulnotion/jam/bmt/seglog"
)

// Shared provides thread-safe access to rollback state.
// It coordinates between in-memory deltas and segmented log persistence.
type Shared struct {
	mu       sync.RWMutex
	inMemory *InMemory
	segLog   *seglog.SegmentedLog
}

// NewShared creates a new shared rollback state.
func NewShared(inMemory *InMemory, segLog *seglog.SegmentedLog) *Shared {
	return &Shared{
		inMemory: inMemory,
		segLog:   segLog,
	}
}

// Commit persists a delta and adds it to the in-memory cache.
// Returns the RecordId assigned to this delta.
func (s *Shared) Commit(delta *Delta) (seglog.RecordId, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Encode delta
	encoded := delta.Encode()

	// Append to segmented log
	recordId, err := s.segLog.Append(encoded)
	if err != nil {
		return seglog.InvalidRecordId, err
	}

	// Add to in-memory cache
	s.inMemory.Push(recordId, delta)

	return recordId, nil
}

// GetDelta retrieves a delta by RecordId.
// Checks in-memory cache first, then falls back to segmented log.
func (s *Shared) GetDelta(recordId seglog.RecordId) (*Delta, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Check in-memory cache first
	if delta, found := s.inMemory.Get(recordId); found {
		return delta, nil
	}

	// Fall back to segmented log
	encoded, err := s.segLog.Read(recordId)
	if err != nil {
		return nil, err
	}

	// Decode delta
	delta, err := DecodeBytes(encoded)
	if err != nil {
		return nil, err
	}

	return delta, nil
}

// Sync flushes the segmented log to disk.
func (s *Shared) Sync() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.segLog.Sync()
}

// Prune removes deltas older than the given RecordId.
func (s *Shared) Prune(beforeRecordId seglog.RecordId) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Prune from segmented log (removes complete segments)
	return s.segLog.Prune(beforeRecordId)
}

// Close closes the shared state.
func (s *Shared) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.segLog.Close()
}

// InMemorySize returns the number of deltas in the in-memory cache.
func (s *Shared) InMemorySize() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.inMemory.Size()
}

// NextRecordId returns the next RecordId that will be assigned.
func (s *Shared) NextRecordId() seglog.RecordId {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.segLog.NextRecordId()
}

// DecodeBytes is a helper to decode delta from bytes (for compatibility with seglog).
func DecodeBytes(data []byte) (*Delta, error) {
	return Decode(newBytesReader(data))
}

// bytesReader wraps a byte slice to implement io.Reader.
type bytesReader struct {
	data []byte
	pos  int
}

func newBytesReader(data []byte) *bytesReader {
	return &bytesReader{data: data, pos: 0}
}

func (r *bytesReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	if r.pos >= len(r.data) {
		return n, io.EOF
	}
	return n, nil
}
