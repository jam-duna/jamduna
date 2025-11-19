package rollback

import (
	"testing"

	"github.com/colorfulnotion/jam/bmt/seglog"
)

func TestInMemoryPushGet(t *testing.T) {
	im := NewInMemory(5)

	// Push a delta
	delta := NewDelta()
	delta.AddPrior([32]byte{1}, []byte("value1"))

	im.Push(seglog.RecordId(1), delta)

	// Get it back
	retrieved, found := im.Get(seglog.RecordId(1))
	if !found {
		t.Fatal("Expected to find delta")
	}

	if len(retrieved.Priors) != 1 {
		t.Errorf("Expected 1 prior, got %d", len(retrieved.Priors))
	}
}

func TestInMemoryPop(t *testing.T) {
	im := NewInMemory(5)

	// Push deltas
	for i := 1; i <= 3; i++ {
		delta := NewDelta()
		delta.AddPrior([32]byte{byte(i)}, []byte{byte(i)})
		im.Push(seglog.RecordId(i), delta)
	}

	if im.Size() != 3 {
		t.Errorf("Expected size 3, got %d", im.Size())
	}

	// Pop most recent
	recordId, delta, ok := im.Pop()
	if !ok {
		t.Fatal("Expected successful pop")
	}

	if recordId != seglog.RecordId(3) {
		t.Errorf("Expected RecordId 3, got %d", recordId)
	}

	if len(delta.Priors) != 1 {
		t.Errorf("Expected 1 prior, got %d", len(delta.Priors))
	}

	if im.Size() != 2 {
		t.Errorf("Expected size 2 after pop, got %d", im.Size())
	}
}

func TestInMemoryEviction(t *testing.T) {
	im := NewInMemory(3) // Small capacity

	// Push more than capacity
	for i := 1; i <= 5; i++ {
		delta := NewDelta()
		delta.AddPrior([32]byte{byte(i)}, []byte{byte(i)})
		im.Push(seglog.RecordId(i), delta)
	}

	// Should only have last 3
	if im.Size() != 3 {
		t.Errorf("Expected size 3, got %d", im.Size())
	}

	// Oldest should be RecordId 3 (1 and 2 evicted)
	oldest, ok := im.OldestRecordId()
	if !ok {
		t.Fatal("Expected to find oldest")
	}

	if oldest != seglog.RecordId(3) {
		t.Errorf("Expected oldest RecordId 3, got %d", oldest)
	}

	// Newest should be RecordId 5
	newest, ok := im.NewestRecordId()
	if !ok {
		t.Fatal("Expected to find newest")
	}

	if newest != seglog.RecordId(5) {
		t.Errorf("Expected newest RecordId 5, got %d", newest)
	}

	// RecordId 1 should not be found
	_, found := im.Get(seglog.RecordId(1))
	if found {
		t.Error("RecordId 1 should have been evicted")
	}
}

func TestInMemoryGetRange(t *testing.T) {
	im := NewInMemory(10)

	// Push deltas
	for i := 1; i <= 5; i++ {
		delta := NewDelta()
		delta.AddPrior([32]byte{byte(i)}, []byte{byte(i)})
		im.Push(seglog.RecordId(i), delta)
	}

	// Get range from RecordId 3 onwards
	entries := im.GetRange(seglog.RecordId(3))

	// Should get 3, 4, 5 (in descending order)
	if len(entries) != 3 {
		t.Fatalf("Expected 3 entries, got %d", len(entries))
	}

	// Check order (newest first)
	if entries[0].RecordId != seglog.RecordId(5) {
		t.Errorf("Expected first entry to be RecordId 5, got %d", entries[0].RecordId)
	}

	if entries[2].RecordId != seglog.RecordId(3) {
		t.Errorf("Expected last entry to be RecordId 3, got %d", entries[2].RecordId)
	}
}

func TestInMemoryClear(t *testing.T) {
	im := NewInMemory(5)

	// Push deltas
	for i := 1; i <= 3; i++ {
		delta := NewDelta()
		im.Push(seglog.RecordId(i), delta)
	}

	if im.Size() != 3 {
		t.Errorf("Expected size 3, got %d", im.Size())
	}

	// Clear
	im.Clear()

	if im.Size() != 0 {
		t.Errorf("Expected size 0 after clear, got %d", im.Size())
	}

	_, found := im.Get(seglog.RecordId(1))
	if found {
		t.Error("Expected not to find RecordId 1 after clear")
	}
}

func TestInMemoryEmptyPop(t *testing.T) {
	im := NewInMemory(5)

	_, _, ok := im.Pop()
	if ok {
		t.Error("Expected pop to fail on empty buffer")
	}
}

func TestInMemoryEmptyOldestNewest(t *testing.T) {
	im := NewInMemory(5)

	_, ok := im.OldestRecordId()
	if ok {
		t.Error("Expected OldestRecordId to fail on empty buffer")
	}

	_, ok = im.NewestRecordId()
	if ok {
		t.Error("Expected NewestRecordId to fail on empty buffer")
	}
}

func TestInMemoryDefaultCapacity(t *testing.T) {
	im := NewInMemory(0) // Invalid capacity

	if im.Capacity() <= 0 {
		t.Errorf("Expected positive default capacity, got %d", im.Capacity())
	}
}
