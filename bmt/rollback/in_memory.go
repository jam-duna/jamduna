package rollback

import (
	"sync"

	"github.com/colorfulnotion/jam/bmt/seglog"
)

// InMemory is a ring buffer for recent deltas stored in memory.
// Older deltas are evicted when the capacity is reached.
type InMemory struct {
	mu       sync.RWMutex
	deltas   []DeltaEntry
	capacity int
	head     int // Next write position
	size     int // Current number of entries
}

// DeltaEntry pairs a RecordId with its delta.
type DeltaEntry struct {
	RecordId seglog.RecordId
	Delta    *Delta
}

// NewInMemory creates a new in-memory delta ring buffer.
func NewInMemory(capacity int) *InMemory {
	if capacity <= 0 {
		capacity = 1000 // Default capacity
	}
	return &InMemory{
		deltas:   make([]DeltaEntry, capacity),
		capacity: capacity,
		head:     0,
		size:     0,
	}
}

// Push adds a delta to the ring buffer.
// If the buffer is full, the oldest entry is evicted.
func (im *InMemory) Push(recordId seglog.RecordId, delta *Delta) {
	im.mu.Lock()
	defer im.mu.Unlock()

	im.deltas[im.head] = DeltaEntry{
		RecordId: recordId,
		Delta:    delta,
	}

	im.head = (im.head + 1) % im.capacity

	if im.size < im.capacity {
		im.size++
	}
}

// Get retrieves a delta by RecordId.
// Returns (delta, true) if found, (nil, false) otherwise.
func (im *InMemory) Get(recordId seglog.RecordId) (*Delta, bool) {
	im.mu.RLock()
	defer im.mu.RUnlock()

	// Search backwards from most recent
	for i := 0; i < im.size; i++ {
		idx := (im.head - 1 - i + im.capacity) % im.capacity
		entry := im.deltas[idx]
		if entry.RecordId == recordId {
			return entry.Delta, true
		}
	}

	return nil, false
}

// GetRange retrieves all deltas with RecordId >= minRecordId.
// Returns them in descending order (newest first).
func (im *InMemory) GetRange(minRecordId seglog.RecordId) []DeltaEntry {
	im.mu.RLock()
	defer im.mu.RUnlock()

	var result []DeltaEntry

	// Scan backwards from most recent
	for i := 0; i < im.size; i++ {
		idx := (im.head - 1 - i + im.capacity) % im.capacity
		entry := im.deltas[idx]
		if entry.RecordId >= minRecordId {
			result = append(result, entry)
		} else {
			// Since we're going backwards, we can stop once we hit an older record
			break
		}
	}

	return result
}

// Pop removes and returns the most recent delta.
// Returns (recordId, delta, true) if successful, (InvalidRecordId, nil, false) if empty.
func (im *InMemory) Pop() (seglog.RecordId, *Delta, bool) {
	im.mu.Lock()
	defer im.mu.Unlock()

	if im.size == 0 {
		return seglog.InvalidRecordId, nil, false
	}

	// Get most recent entry
	idx := (im.head - 1 + im.capacity) % im.capacity
	entry := im.deltas[idx]

	// Clear the entry
	im.deltas[idx] = DeltaEntry{}

	// Move head back
	im.head = idx
	im.size--

	return entry.RecordId, entry.Delta, true
}

// Size returns the current number of deltas in the buffer.
func (im *InMemory) Size() int {
	im.mu.RLock()
	defer im.mu.RUnlock()
	return im.size
}

// Capacity returns the maximum number of deltas the buffer can hold.
func (im *InMemory) Capacity() int {
	return im.capacity
}

// Clear removes all deltas from the buffer.
func (im *InMemory) Clear() {
	im.mu.Lock()
	defer im.mu.Unlock()

	im.deltas = make([]DeltaEntry, im.capacity)
	im.head = 0
	im.size = 0
}

// OldestRecordId returns the RecordId of the oldest delta in the buffer.
// Returns (recordId, true) if buffer is not empty, (InvalidRecordId, false) otherwise.
func (im *InMemory) OldestRecordId() (seglog.RecordId, bool) {
	im.mu.RLock()
	defer im.mu.RUnlock()

	if im.size == 0 {
		return seglog.InvalidRecordId, false
	}

	// Oldest is at (head - size)
	idx := (im.head - im.size + im.capacity) % im.capacity
	return im.deltas[idx].RecordId, true
}

// NewestRecordId returns the RecordId of the newest delta in the buffer.
// Returns (recordId, true) if buffer is not empty, (InvalidRecordId, false) otherwise.
func (im *InMemory) NewestRecordId() (seglog.RecordId, bool) {
	im.mu.RLock()
	defer im.mu.RUnlock()

	if im.size == 0 {
		return seglog.InvalidRecordId, false
	}

	// Newest is at (head - 1)
	idx := (im.head - 1 + im.capacity) % im.capacity
	return im.deltas[idx].RecordId, true
}
