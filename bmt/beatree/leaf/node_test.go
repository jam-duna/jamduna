package leaf

import (
	"testing"

	"github.com/colorfulnotion/jam/bmt/beatree"
	"github.com/colorfulnotion/jam/bmt/beatree/allocator"
)

func TestLeafNodeInsertLookup(t *testing.T) {
	node := NewNode()

	key1 := beatree.Key{0x00, 0x00, 0x00, 0x01}
	value1 := []byte("value1")

	entry1, err := NewInlineEntry(key1, value1)
	if err != nil {
		t.Fatalf("NewInlineEntry failed: %v", err)
	}

	if err := node.Insert(entry1); err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	// Lookup
	found, ok := node.Lookup(key1)
	if !ok {
		t.Fatal("lookup failed: key not found")
	}

	if !found.Key.Equal(key1) {
		t.Error("lookup returned wrong key")
	}

	if string(found.Value) != "value1" {
		t.Errorf("lookup returned wrong value: %s", string(found.Value))
	}
}

func TestLeafNodeUpdate(t *testing.T) {
	node := NewNode()

	key := beatree.Key{0x00, 0x00, 0x00, 0x01}
	value1 := []byte("value1")
	value2 := []byte("value2")

	entry1, _ := NewInlineEntry(key, value1)
	node.Insert(entry1)

	entry2, _ := NewInlineEntry(key, value2)
	node.Insert(entry2)

	// Should have only 1 entry (update case)
	if node.NumEntries() != 1 {
		t.Errorf("num entries = %d, want 1", node.NumEntries())
	}

	found, _ := node.Lookup(key)
	if string(found.Value) != "value2" {
		t.Errorf("value = %s, want value2", string(found.Value))
	}
}

func TestLeafNodeDelete(t *testing.T) {
	node := NewNode()

	key1 := beatree.Key{0x00, 0x00, 0x00, 0x01}
	key2 := beatree.Key{0x00, 0x00, 0x00, 0x02}

	entry1, _ := NewInlineEntry(key1, []byte("value1"))
	entry2, _ := NewInlineEntry(key2, []byte("value2"))

	node.Insert(entry1)
	node.Insert(entry2)

	if node.NumEntries() != 2 {
		t.Errorf("num entries = %d, want 2", node.NumEntries())
	}

	// Delete key1
	if !node.Delete(key1) {
		t.Error("delete should return true")
	}

	if node.NumEntries() != 1 {
		t.Errorf("num entries after delete = %d, want 1", node.NumEntries())
	}

	// Verify key1 is gone
	if _, ok := node.Lookup(key1); ok {
		t.Error("deleted key should not be found")
	}

	// Verify key2 still exists
	if _, ok := node.Lookup(key2); !ok {
		t.Error("key2 should still exist")
	}

	// Delete non-existent key
	key3 := beatree.Key{0x00, 0x00, 0x00, 0x03}
	if node.Delete(key3) {
		t.Error("deleting non-existent key should return false")
	}
}

func TestLeafNodeSorted(t *testing.T) {
	node := NewNode()

	key3 := beatree.Key{0x00, 0x00, 0x00, 0x03}
	key1 := beatree.Key{0x00, 0x00, 0x00, 0x01}
	key2 := beatree.Key{0x00, 0x00, 0x00, 0x02}

	// Insert out of order
	node.Insert(Entry{Key: key3, ValueType: ValueTypeInline, Value: []byte("val3")})
	node.Insert(Entry{Key: key1, ValueType: ValueTypeInline, Value: []byte("val1")})
	node.Insert(Entry{Key: key2, ValueType: ValueTypeInline, Value: []byte("val2")})

	// Verify sorted order
	if !node.Entries[0].Key.Equal(key1) {
		t.Error("entry 0 should be key1")
	}

	if !node.Entries[1].Key.Equal(key2) {
		t.Error("entry 1 should be key2")
	}

	if !node.Entries[2].Key.Equal(key3) {
		t.Error("entry 2 should be key3")
	}
}

func TestLeafNodeOverflowEntry(t *testing.T) {
	key := beatree.Key{0x00, 0x00, 0x00, 0x01}
	overflowPage := allocator.PageNumber(100)
	overflowSize := uint64(5000)
	valuePrefix := []byte("prefix...")

	entry := NewOverflowEntry(key, overflowPage, overflowSize, valuePrefix)

	if entry.ValueType != ValueTypeOverflow {
		t.Error("entry should be overflow type")
	}

	if entry.OverflowPage != overflowPage {
		t.Errorf("overflow page = %v, want %v", entry.OverflowPage, overflowPage)
	}

	if entry.OverflowSize != overflowSize {
		t.Errorf("overflow size = %d, want %d", entry.OverflowSize, overflowSize)
	}
}

func TestLeafNodeInlineTooBig(t *testing.T) {
	key := beatree.Key{0x00, 0x00, 0x00, 0x01}
	value := make([]byte, MaxLeafValueSize+1) // Too big

	_, err := NewInlineEntry(key, value)
	if err == nil {
		t.Error("NewInlineEntry should fail with oversized value")
	}
}
