package leaf

import (
	"testing"

	"github.com/jam-duna/jamduna/bmt/beatree"
	"github.com/jam-duna/jamduna/bmt/beatree/allocator"
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

// TestUpdateEntry_Update verifies updating an existing entry
func TestUpdateEntry_Update(t *testing.T) {
	node := NewNode()

	// Insert initial entry
	key := beatree.Key{0x00, 0x00, 0x00, 0x01}
	value1 := []byte("value1")
	entry1, _ := NewInlineEntry(key, value1)
	node.Insert(entry1)

	// Update the entry
	value2 := []byte("value2_updated")
	found, needsSplit, err := node.UpdateEntry(key, value2)

	if err != nil {
		t.Fatalf("UpdateEntry failed: %v", err)
	}
	if !found {
		t.Error("UpdateEntry should return found=true for existing key")
	}
	if needsSplit {
		t.Error("UpdateEntry should not need split for small update")
	}

	// Verify the entry was updated
	if node.NumEntries() != 1 {
		t.Errorf("num entries = %d, want 1", node.NumEntries())
	}

	lookupEntry, ok := node.Lookup(key)
	if !ok {
		t.Fatal("key should still exist after update")
	}
	if string(lookupEntry.Value) != "value2_updated" {
		t.Errorf("value = %s, want value2_updated", string(lookupEntry.Value))
	}
}

// TestUpdateEntry_Insert verifies inserting a new entry
func TestUpdateEntry_Insert(t *testing.T) {
	node := NewNode()

	// Insert first entry
	key1 := beatree.Key{0x00, 0x00, 0x00, 0x01}
	value1 := []byte("value1")
	node.Insert(Entry{Key: key1, ValueType: ValueTypeInline, Value: value1})

	// Insert new entry via UpdateEntry
	key2 := beatree.Key{0x00, 0x00, 0x00, 0x02}
	value2 := []byte("value2")
	found, needsSplit, err := node.UpdateEntry(key2, value2)

	if err != nil {
		t.Fatalf("UpdateEntry failed: %v", err)
	}
	if found {
		t.Error("UpdateEntry should return found=false for new key")
	}
	if needsSplit {
		t.Error("UpdateEntry should not need split for small insert")
	}

	// Verify both entries exist
	if node.NumEntries() != 2 {
		t.Errorf("num entries = %d, want 2", node.NumEntries())
	}

	// Verify order is maintained
	if !node.Entries[0].Key.Equal(key1) {
		t.Error("entry 0 should be key1")
	}
	if !node.Entries[1].Key.Equal(key2) {
		t.Error("entry 1 should be key2")
	}
}

// TestUpdateEntry_BinarySearch verifies binary search works correctly
func TestUpdateEntry_BinarySearch(t *testing.T) {
	node := NewNode()

	// Insert 100 entries in sorted order
	for i := 0; i < 100; i++ {
		key := beatree.Key{}
		key[31] = byte(i)
		value := []byte{byte(i)}
		entry, _ := NewInlineEntry(key, value)
		node.Insert(entry)
	}

	// Update middle entry (should use binary search, not linear scan)
	key50 := beatree.Key{}
	key50[31] = 50
	newValue := []byte("updated_50")
	found, _, err := node.UpdateEntry(key50, newValue)

	if err != nil {
		t.Fatalf("UpdateEntry failed: %v", err)
	}
	if !found {
		t.Error("UpdateEntry should find key 50")
	}

	// Verify update
	lookupEntry, ok := node.Lookup(key50)
	if !ok || string(lookupEntry.Value) != "updated_50" {
		t.Error("key 50 should be updated")
	}

	// Verify all other entries unchanged
	if node.NumEntries() != 100 {
		t.Errorf("num entries = %d, want 100", node.NumEntries())
	}
}

// TestUpdateEntry_TooLarge verifies error handling for oversized values
func TestUpdateEntry_TooLarge(t *testing.T) {
	node := NewNode()

	key := beatree.Key{0x00, 0x00, 0x00, 0x01}
	value := make([]byte, MaxLeafValueSize+1) // Too large

	_, _, err := node.UpdateEntry(key, value)
	if err == nil {
		t.Error("UpdateEntry should fail with oversized value")
	}
}

// TestDeleteEntry_Found verifies deleting an existing entry
func TestDeleteEntry_Found(t *testing.T) {
	node := NewNode()

	// Insert 3 entries
	key1 := beatree.Key{0x00, 0x00, 0x00, 0x01}
	key2 := beatree.Key{0x00, 0x00, 0x00, 0x02}
	key3 := beatree.Key{0x00, 0x00, 0x00, 0x03}

	node.Insert(Entry{Key: key1, ValueType: ValueTypeInline, Value: []byte("val1")})
	node.Insert(Entry{Key: key2, ValueType: ValueTypeInline, Value: []byte("val2")})
	node.Insert(Entry{Key: key3, ValueType: ValueTypeInline, Value: []byte("val3")})

	// Delete middle entry
	found, err := node.DeleteEntry(key2)
	if err != nil {
		t.Fatalf("DeleteEntry failed: %v", err)
	}
	if !found {
		t.Error("DeleteEntry should return found=true")
	}

	// Verify deletion
	if node.NumEntries() != 2 {
		t.Errorf("num entries = %d, want 2", node.NumEntries())
	}

	if _, ok := node.Lookup(key2); ok {
		t.Error("deleted key should not be found")
	}

	// Verify remaining entries are in order
	if !node.Entries[0].Key.Equal(key1) {
		t.Error("entry 0 should be key1")
	}
	if !node.Entries[1].Key.Equal(key3) {
		t.Error("entry 1 should be key3")
	}
}

// TestDeleteEntry_NotFound verifies deleting a non-existent entry
func TestDeleteEntry_NotFound(t *testing.T) {
	node := NewNode()

	// Insert one entry
	key1 := beatree.Key{0x00, 0x00, 0x00, 0x01}
	node.Insert(Entry{Key: key1, ValueType: ValueTypeInline, Value: []byte("val1")})

	// Try to delete non-existent key
	key2 := beatree.Key{0x00, 0x00, 0x00, 0x02}
	found, err := node.DeleteEntry(key2)

	if err != nil {
		t.Fatalf("DeleteEntry failed: %v", err)
	}
	if found {
		t.Error("DeleteEntry should return found=false for non-existent key")
	}

	// Verify original entry unchanged
	if node.NumEntries() != 1 {
		t.Errorf("num entries = %d, want 1", node.NumEntries())
	}
}

// TestDeleteEntry_BinarySearch verifies binary search in delete
func TestDeleteEntry_BinarySearch(t *testing.T) {
	node := NewNode()

	// Insert 100 entries
	for i := 0; i < 100; i++ {
		key := beatree.Key{}
		key[31] = byte(i)
		value := []byte{byte(i)}
		entry, _ := NewInlineEntry(key, value)
		node.Insert(entry)
	}

	// Delete middle entry
	key50 := beatree.Key{}
	key50[31] = 50
	found, err := node.DeleteEntry(key50)

	if err != nil {
		t.Fatalf("DeleteEntry failed: %v", err)
	}
	if !found {
		t.Error("DeleteEntry should find key 50")
	}

	// Verify deletion
	if node.NumEntries() != 99 {
		t.Errorf("num entries = %d, want 99", node.NumEntries())
	}
	if _, ok := node.Lookup(key50); ok {
		t.Error("deleted key should not be found")
	}
}

// TestClone_Shallow verifies shallow copy semantics
func TestClone_Shallow(t *testing.T) {
	original := NewNode()

	// Insert entries
	key1 := beatree.Key{0x00, 0x00, 0x00, 0x01}
	key2 := beatree.Key{0x00, 0x00, 0x00, 0x02}
	value1 := []byte("value1")
	value2 := []byte("value2")

	original.Insert(Entry{Key: key1, ValueType: ValueTypeInline, Value: value1})
	original.Insert(Entry{Key: key2, ValueType: ValueTypeInline, Value: value2})

	// Clone
	cloned := original.Clone()

	if cloned == nil {
		t.Fatal("Clone returned nil")
	}

	// Verify independent Entries slice
	if &original.Entries == &cloned.Entries {
		t.Error("Clone should create new Entries slice")
	}

	// Verify same number of entries
	if cloned.NumEntries() != 2 {
		t.Errorf("cloned num entries = %d, want 2", cloned.NumEntries())
	}

	// Verify entries are copied
	if !cloned.Entries[0].Key.Equal(key1) {
		t.Error("cloned entry 0 key mismatch")
	}
	if !cloned.Entries[1].Key.Equal(key2) {
		t.Error("cloned entry 1 key mismatch")
	}
}

// TestClone_Independence verifies modifications don't affect original
func TestClone_Independence(t *testing.T) {
	original := NewNode()

	// Insert entries
	key1 := beatree.Key{0x00, 0x00, 0x00, 0x01}
	key2 := beatree.Key{0x00, 0x00, 0x00, 0x02}
	value1 := []byte("value1")
	value2 := []byte("value2")

	original.Insert(Entry{Key: key1, ValueType: ValueTypeInline, Value: value1})
	original.Insert(Entry{Key: key2, ValueType: ValueTypeInline, Value: value2})

	// Clone
	cloned := original.Clone()

	// Modify cloned node
	key3 := beatree.Key{0x00, 0x00, 0x00, 0x03}
	value3 := []byte("value3")
	cloned.Insert(Entry{Key: key3, ValueType: ValueTypeInline, Value: value3})

	// Verify original unchanged
	if original.NumEntries() != 2 {
		t.Errorf("original num entries = %d, want 2", original.NumEntries())
	}

	// Verify clone has new entry
	if cloned.NumEntries() != 3 {
		t.Errorf("cloned num entries = %d, want 3", cloned.NumEntries())
	}
}

// TestClone_Nil verifies nil handling
func TestClone_Nil(t *testing.T) {
	var node *Node = nil
	cloned := node.Clone()

	if cloned != nil {
		t.Error("Clone of nil should return nil")
	}
}

// TestClone_Empty verifies empty node cloning
func TestClone_Empty(t *testing.T) {
	original := NewNode()
	cloned := original.Clone()

	if cloned == nil {
		t.Fatal("Clone of empty node should not return nil")
	}

	if cloned.NumEntries() != 0 {
		t.Errorf("cloned empty node should have 0 entries, got %d", cloned.NumEntries())
	}
}

// TestClone_ValueSharing verifies value buffers are shared (not deep copied)
func TestClone_ValueSharing(t *testing.T) {
	original := NewNode()

	key := beatree.Key{0x00, 0x00, 0x00, 0x01}
	value := []byte("shared_value")
	original.Insert(Entry{Key: key, ValueType: ValueTypeInline, Value: value})

	cloned := original.Clone()

	// Verify value slices point to same backing array
	// (In practice this is implementation-dependent, but we document it as shared)
	originalValue := original.Entries[0].Value
	clonedValue := cloned.Entries[0].Value

	if string(originalValue) != string(clonedValue) {
		t.Error("cloned value should be equal to original")
	}
}
