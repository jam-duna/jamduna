package leaf

import (
	"bytes"
	"testing"

	"github.com/colorfulnotion/jam/bmt/beatree"
	"github.com/colorfulnotion/jam/bmt/beatree/allocator"
)

// TestInlineEntryDefensiveCopy verifies that NewInlineEntry makes a defensive copy.
// This tests the fix for Bug #3: prevent caller mutation from corrupting stored values.
func TestInlineEntryDefensiveCopy(t *testing.T) {
	key := beatree.Key{0x01, 0x02, 0x03}
	value := []byte("original value")

	// Create entry
	entry, err := NewInlineEntry(key, value)
	if err != nil {
		t.Fatalf("NewInlineEntry failed: %v", err)
	}

	// Verify initial value
	if !bytes.Equal(entry.Value, value) {
		t.Error("Initial value mismatch")
	}

	// Mutate the original slice
	for i := range value {
		value[i] = 0xFF
	}

	// Verify entry value is unchanged (defensive copy worked)
	if bytes.Equal(entry.Value, value) {
		t.Error("Entry value was mutated by caller - defensive copy failed!")
	}

	expectedValue := []byte("original value")
	if !bytes.Equal(entry.Value, expectedValue) {
		t.Errorf("Entry value corrupted: expected %q, got %q", expectedValue, entry.Value)
	}
}

// TestOverflowEntryDefensiveCopy verifies that NewOverflowEntry makes a defensive copy.
func TestOverflowEntryDefensiveCopy(t *testing.T) {
	key := beatree.Key{0x01, 0x02, 0x03}
	prefix := []byte("prefix data for caching")
	overflowPage := allocator.PageNumber(100)
	overflowSize := uint64(5000)

	// Create entry
	entry := NewOverflowEntry(key, overflowPage, overflowSize, prefix)

	// Verify initial prefix
	originalPrefix := make([]byte, len(prefix))
	copy(originalPrefix, prefix)

	if !bytes.Equal(entry.Value, originalPrefix) {
		t.Error("Initial prefix mismatch")
	}

	// Mutate the original prefix slice
	for i := range prefix {
		prefix[i] = 0xAA
	}

	// Verify entry prefix is unchanged (defensive copy worked)
	if bytes.Equal(entry.Value, prefix) {
		t.Error("Entry prefix was mutated by caller - defensive copy failed!")
	}

	if !bytes.Equal(entry.Value, originalPrefix) {
		t.Errorf("Entry prefix corrupted: expected %v, got %v", originalPrefix, entry.Value)
	}
}

// TestOverflowEntryPrefixTruncation verifies prefix is truncated to 32 bytes.
func TestOverflowEntryPrefixTruncation(t *testing.T) {
	key := beatree.Key{0x01}
	longPrefix := make([]byte, 100) // > 32 bytes
	for i := range longPrefix {
		longPrefix[i] = byte(i)
	}

	entry := NewOverflowEntry(key, allocator.PageNumber(1), 100, longPrefix)

	// Verify prefix is truncated to 32 bytes
	if len(entry.Value) != 32 {
		t.Errorf("Expected prefix length 32, got %d", len(entry.Value))
	}

	// Verify content matches first 32 bytes
	for i := 0; i < 32; i++ {
		if entry.Value[i] != byte(i) {
			t.Errorf("Prefix byte %d: expected %d, got %d", i, i, entry.Value[i])
		}
	}

	// Mutate original to verify defensive copy
	longPrefix[0] = 0xFF
	if entry.Value[0] == 0xFF {
		t.Error("Prefix mutation leaked to entry - defensive copy failed!")
	}
}

// TestLeafNodeValueMutationIsolation verifies that values stored in nodes
// are isolated from caller mutations.
func TestLeafNodeValueMutationIsolation(t *testing.T) {
	node := NewNode()

	key1 := beatree.Key{0x01}
	value1 := []byte("value one")

	key2 := beatree.Key{0x02}
	value2 := []byte("value two")

	// Insert entries
	entry1, _ := NewInlineEntry(key1, value1)
	entry2, _ := NewInlineEntry(key2, value2)

	node.Insert(entry1)
	node.Insert(entry2)

	// Mutate original value slices
	for i := range value1 {
		value1[i] = 0xAA
	}
	for i := range value2 {
		value2[i] = 0xBB
	}

	// Lookup and verify values are unchanged
	found1, ok1 := node.Lookup(key1)
	if !ok1 {
		t.Fatal("Failed to lookup key1")
	}

	found2, ok2 := node.Lookup(key2)
	if !ok2 {
		t.Fatal("Failed to lookup key2")
	}

	// Values should not be mutated
	expectedValue1 := []byte("value one")
	expectedValue2 := []byte("value two")

	if !bytes.Equal(found1.Value, expectedValue1) {
		t.Errorf("Value1 corrupted: expected %q, got %q", expectedValue1, found1.Value)
	}

	if !bytes.Equal(found2.Value, expectedValue2) {
		t.Errorf("Value2 corrupted: expected %q, got %q", expectedValue2, found2.Value)
	}
}

// TestLeafNodeUpdateDefensiveCopy verifies that updates also use defensive copies.
func TestLeafNodeUpdateDefensiveCopy(t *testing.T) {
	node := NewNode()

	key := beatree.Key{0x01}
	value1 := []byte("initial value")

	// Insert initial entry
	entry1, _ := NewInlineEntry(key, value1)
	node.Insert(entry1)

	// Update with new value
	value2 := []byte("updated value")
	entry2, _ := NewInlineEntry(key, value2)
	node.Insert(entry2)

	// Mutate both original slices
	for i := range value1 {
		value1[i] = 0xAA
	}
	for i := range value2 {
		value2[i] = 0xBB
	}

	// Lookup and verify current value is unchanged
	found, ok := node.Lookup(key)
	if !ok {
		t.Fatal("Failed to lookup key")
	}

	expectedValue := []byte("updated value")
	if !bytes.Equal(found.Value, expectedValue) {
		t.Errorf("Updated value corrupted: expected %q, got %q", expectedValue, found.Value)
	}
}

// TestEmptyValueDefensiveCopy verifies defensive copy works with empty values.
func TestEmptyValueDefensiveCopy(t *testing.T) {
	key := beatree.Key{0x01}
	emptyValue := []byte{}

	entry, err := NewInlineEntry(key, emptyValue)
	if err != nil {
		t.Fatalf("NewInlineEntry failed: %v", err)
	}

	if len(entry.Value) != 0 {
		t.Errorf("Expected empty value, got length %d", len(entry.Value))
	}

	// Verify it's a new slice (not nil)
	if entry.Value == nil {
		t.Error("Entry value should be empty slice, not nil")
	}
}
