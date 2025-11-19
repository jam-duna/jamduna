package beatree

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/colorfulnotion/jam/bmt/beatree/allocator"
)

// TestIteratorEmpty tests iteration over empty staging maps.
func TestIteratorEmpty(t *testing.T) {
	iter := NewIterator(nil, nil, nil, nil, allocator.InvalidPageNumber, Key{}, nil)

	key, value, ok := iter.Next()
	if ok {
		t.Errorf("Expected no items, got key=%v value=%v", key, value)
	}
}

// TestIteratorSingleStaging tests iteration over primary staging only.
func TestIteratorSingleStaging(t *testing.T) {
	primary := make(map[Key]*Change)

	// Insert keys 1, 3, 5 (out of order)
	primary[keyFromInt(5)] = NewInsertChange([]byte("value5"))
	primary[keyFromInt(1)] = NewInsertChange([]byte("value1"))
	primary[keyFromInt(3)] = NewInsertChange([]byte("value3"))

	iter := NewIterator(primary, nil, nil, nil, allocator.InvalidPageNumber, Key{}, nil)

	// Should return in sorted order: 1, 3, 5
	expected := []int{1, 3, 5}
	for i, exp := range expected {
		key, value, ok := iter.Next()
		if !ok {
			t.Fatalf("Expected item %d, got end of iterator", i)
		}

		expectedKey := keyFromInt(exp)
		if key != expectedKey {
			t.Errorf("Item %d: expected key %v, got %v", i, expectedKey, key)
		}

		expectedValue := fmt.Sprintf("value%d", exp)
		if string(value) != expectedValue {
			t.Errorf("Item %d: expected value %s, got %s", i, expectedValue, value)
		}
	}

	// Should be exhausted
	_, _, ok := iter.Next()
	if ok {
		t.Errorf("Expected end of iterator, got more items")
	}
}

// TestIteratorPrimaryOverridesSecondary tests that primary staging takes precedence.
func TestIteratorPrimaryOverridesSecondary(t *testing.T) {
	secondary := make(map[Key]*Change)
	primary := make(map[Key]*Change)

	// Secondary has keys 1, 2, 3
	secondary[keyFromInt(1)] = NewInsertChange([]byte("old1"))
	secondary[keyFromInt(2)] = NewInsertChange([]byte("old2"))
	secondary[keyFromInt(3)] = NewInsertChange([]byte("old3"))

	// Primary overrides key 2 and adds key 4
	primary[keyFromInt(2)] = NewInsertChange([]byte("new2"))
	primary[keyFromInt(4)] = NewInsertChange([]byte("value4"))

	iter := NewIterator(primary, secondary, nil, nil, allocator.InvalidPageNumber, Key{}, nil)

	// Expected: 1(old), 2(new), 3(old), 4
	expected := []struct {
		key   int
		value string
	}{
		{1, "old1"},
		{2, "new2"}, // Primary overrides secondary
		{3, "old3"},
		{4, "value4"},
	}

	for i, exp := range expected {
		key, value, ok := iter.Next()
		if !ok {
			t.Fatalf("Expected item %d, got end of iterator", i)
		}

		expectedKey := keyFromInt(exp.key)
		if key != expectedKey {
			t.Errorf("Item %d: expected key %v, got %v", i, expectedKey, key)
		}

		if string(value) != exp.value {
			t.Errorf("Item %d: expected value %s, got %s", i, exp.value, value)
		}
	}

	// Should be exhausted
	_, _, ok := iter.Next()
	if ok {
		t.Errorf("Expected end of iterator, got more items")
	}
}

// TestIteratorDeleteMarker tests that delete markers hide values.
func TestIteratorDeleteMarker(t *testing.T) {
	secondary := make(map[Key]*Change)
	primary := make(map[Key]*Change)

	// Secondary has keys 1, 2, 3
	secondary[keyFromInt(1)] = NewInsertChange([]byte("value1"))
	secondary[keyFromInt(2)] = NewInsertChange([]byte("value2"))
	secondary[keyFromInt(3)] = NewInsertChange([]byte("value3"))

	// Primary deletes key 2
	primary[keyFromInt(2)] = NewDeleteChange()

	iter := NewIterator(primary, secondary, nil, nil, allocator.InvalidPageNumber, Key{}, nil)

	// Expected: 1, 3 (2 is deleted)
	expected := []int{1, 3}
	for i, exp := range expected {
		key, value, ok := iter.Next()
		if !ok {
			t.Fatalf("Expected item %d, got end of iterator", i)
		}

		expectedKey := keyFromInt(exp)
		if key != expectedKey {
			t.Errorf("Item %d: expected key %v, got %v", i, expectedKey, key)
		}

		expectedValue := fmt.Sprintf("value%d", exp)
		if string(value) != expectedValue {
			t.Errorf("Item %d: expected value %s, got %s", i, expectedValue, value)
		}
	}

	// Should be exhausted
	_, _, ok := iter.Next()
	if ok {
		t.Errorf("Expected end of iterator, got more items")
	}
}

// TestIteratorRangeBounds tests start and end range bounds.
func TestIteratorRangeBounds(t *testing.T) {
	primary := make(map[Key]*Change)

	// Insert keys 1-10
	for i := 1; i <= 10; i++ {
		primary[keyFromInt(i)] = NewInsertChange([]byte(fmt.Sprintf("value%d", i)))
	}

	// Test [3, 7) range
	start := keyFromInt(3)
	end := keyFromInt(7)
	iter := NewIterator(primary, nil, nil, nil, allocator.InvalidPageNumber, start, &end)

	expected := []int{3, 4, 5, 6}
	for i, exp := range expected {
		key, value, ok := iter.Next()
		if !ok {
			t.Fatalf("Expected item %d, got end of iterator", i)
		}

		expectedKey := keyFromInt(exp)
		if key != expectedKey {
			t.Errorf("Item %d: expected key %v, got %v", i, expectedKey, key)
		}

		expectedValue := fmt.Sprintf("value%d", exp)
		if string(value) != expectedValue {
			t.Errorf("Item %d: expected value %s, got %s", i, expectedValue, value)
		}
	}

	// Should be exhausted
	_, _, ok := iter.Next()
	if ok {
		t.Errorf("Expected end of iterator, got more items")
	}
}

// TestIteratorStartBound tests that start bound is inclusive.
func TestIteratorStartBound(t *testing.T) {
	primary := make(map[Key]*Change)

	// Insert keys 1, 2, 3, 4, 5
	for i := 1; i <= 5; i++ {
		primary[keyFromInt(i)] = NewInsertChange([]byte(fmt.Sprintf("value%d", i)))
	}

	// Start at key 3 (inclusive)
	start := keyFromInt(3)
	iter := NewIterator(primary, nil, nil, nil, allocator.InvalidPageNumber, start, nil)

	expected := []int{3, 4, 5}
	for i, exp := range expected {
		key, _, ok := iter.Next()
		if !ok {
			t.Fatalf("Expected item %d, got end of iterator", i)
		}

		expectedKey := keyFromInt(exp)
		if key != expectedKey {
			t.Errorf("Item %d: expected key %v, got %v", i, expectedKey, key)
		}
	}

	// Should be exhausted
	_, _, ok := iter.Next()
	if ok {
		t.Errorf("Expected end of iterator, got more items")
	}
}

// TestIteratorEndBound tests that end bound is exclusive.
func TestIteratorEndBound(t *testing.T) {
	primary := make(map[Key]*Change)

	// Insert keys 1, 2, 3, 4, 5
	for i := 1; i <= 5; i++ {
		primary[keyFromInt(i)] = NewInsertChange([]byte(fmt.Sprintf("value%d", i)))
	}

	// End at key 3 (exclusive)
	end := keyFromInt(3)
	iter := NewIterator(primary, nil, nil, nil, allocator.InvalidPageNumber, Key{}, &end)

	expected := []int{1, 2}
	for i, exp := range expected {
		key, _, ok := iter.Next()
		if !ok {
			t.Fatalf("Expected item %d, got end of iterator", i)
		}

		expectedKey := keyFromInt(exp)
		if key != expectedKey {
			t.Errorf("Item %d: expected key %v, got %v", i, expectedKey, key)
		}
	}

	// Should be exhausted
	_, _, ok := iter.Next()
	if ok {
		t.Errorf("Expected end of iterator, got more items")
	}
}

// TestIteratorMergeOrdering tests proper merge of multiple sorted streams.
func TestIteratorMergeOrdering(t *testing.T) {
	secondary := make(map[Key]*Change)
	primary := make(map[Key]*Change)

	// Secondary has odd keys: 1, 3, 5, 7, 9
	for i := 1; i <= 9; i += 2 {
		secondary[keyFromInt(i)] = NewInsertChange([]byte(fmt.Sprintf("sec%d", i)))
	}

	// Primary has even keys: 2, 4, 6, 8, 10
	for i := 2; i <= 10; i += 2 {
		primary[keyFromInt(i)] = NewInsertChange([]byte(fmt.Sprintf("pri%d", i)))
	}

	iter := NewIterator(primary, secondary, nil, nil, allocator.InvalidPageNumber, Key{}, nil)

	// Should return 1, 2, 3, 4, 5, 6, 7, 8, 9, 10 in order
	for i := 1; i <= 10; i++ {
		key, value, ok := iter.Next()
		if !ok {
			t.Fatalf("Expected item %d, got end of iterator", i)
		}

		expectedKey := keyFromInt(i)
		if key != expectedKey {
			t.Errorf("Item %d: expected key %v, got %v", i, expectedKey, key)
		}

		// Check value prefix
		var expectedPrefix string
		if i%2 == 0 {
			expectedPrefix = "pri"
		} else {
			expectedPrefix = "sec"
		}

		if !bytes.HasPrefix(value, []byte(expectedPrefix)) {
			t.Errorf("Item %d: expected value prefix %s, got %s", i, expectedPrefix, value)
		}
	}

	// Should be exhausted
	_, _, ok := iter.Next()
	if ok {
		t.Errorf("Expected end of iterator, got more items")
	}
}

// Helper function to create a Key from an integer (for testing)
func keyFromInt(n int) Key {
	var key Key
	// Put the integer in the first 4 bytes (big-endian)
	key[0] = byte(n >> 24)
	key[1] = byte(n >> 16)
	key[2] = byte(n >> 8)
	key[3] = byte(n)
	return key
}
