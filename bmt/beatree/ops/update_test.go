package ops

import (
	"testing"

	"github.com/jam-duna/jamduna/bmt/beatree"
	"github.com/jam-duna/jamduna/bmt/beatree/allocator"
)

func TestUpdate_EmptyTree(t *testing.T) {
	leafStore := NewTestStore()

	// Create changeset
	changeset := map[beatree.Key]*beatree.Change{
		{0x01}: beatree.NewInsertChange([]byte("value1")),
		{0x02}: beatree.NewInsertChange([]byte("value2")),
		{0x03}: beatree.NewInsertChange([]byte("value3")),
	}

	// Update empty tree
	result, err := Update(allocator.InvalidPageNumber, changeset, nil, leafStore)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Verify new root was created
	if result.NewRoot == allocator.InvalidPageNumber {
		t.Fatal("Expected new root, got InvalidPageNumber")
	}

	// Verify pages were allocated
	if len(result.AllocatedPages) != 1 {
		t.Errorf("Expected 1 allocated page, got %d", len(result.AllocatedPages))
	}

	// Verify no pages were freed
	if len(result.FreedPages) != 0 {
		t.Errorf("Expected 0 freed pages, got %d", len(result.FreedPages))
	}

	// Verify we can read the values back
	leaf, err := leafStore.GetNode(result.NewRoot)
	if err != nil {
		t.Fatalf("Failed to read leaf: %v", err)
	}

	for key, change := range changeset {
		entry, found := leaf.Lookup(key)
		if !found {
			t.Errorf("Key %v not found", key)
			continue
		}
		if string(entry.Value) != string(change.Value) {
			t.Errorf("Key %v: expected %q, got %q", key, change.Value, entry.Value)
		}
	}
}

func TestUpdate_ExistingTree(t *testing.T) {
	leafStore := NewTestStore()

	// Create initial tree
	changeset1 := map[beatree.Key]*beatree.Change{
		{0x01}: beatree.NewInsertChange([]byte("value1")),
		{0x02}: beatree.NewInsertChange([]byte("value2")),
	}

	result1, err := Update(allocator.InvalidPageNumber, changeset1, nil, leafStore)
	if err != nil {
		t.Fatalf("Initial update failed: %v", err)
	}

	oldRoot := result1.NewRoot

	// Update with new changes
	changeset2 := map[beatree.Key]*beatree.Change{
		{0x02}: beatree.NewInsertChange([]byte("updated2")), // Update existing
		{0x03}: beatree.NewInsertChange([]byte("value3")),   // Insert new
		{0x01}: beatree.NewDeleteChange(),                   // Delete existing
	}

	result2, err := Update(oldRoot, changeset2, nil, leafStore)
	if err != nil {
		t.Fatalf("Second update failed: %v", err)
	}

	// Verify CoW: new root is different
	if result2.NewRoot == oldRoot {
		t.Error("Expected new root for CoW, got same root")
	}

	// Verify new leaf has correct values
	newLeaf, err := leafStore.GetNode(result2.NewRoot)
	if err != nil {
		t.Fatalf("Failed to read new leaf: %v", err)
	}

	// Check key 0x01 was deleted
	if _, found := newLeaf.Lookup(beatree.Key{0x01}); found {
		t.Error("Expected key 0x01 to be deleted")
	}

	// Check key 0x02 was updated
	if entry, found := newLeaf.Lookup(beatree.Key{0x02}); found {
		if string(entry.Value) != "updated2" {
			t.Errorf("Expected 'updated2', got %q", entry.Value)
		}
	} else {
		t.Error("Expected key 0x02 to exist")
	}

	// Check key 0x03 was inserted
	if entry, found := newLeaf.Lookup(beatree.Key{0x03}); found {
		if string(entry.Value) != "value3" {
			t.Errorf("Expected 'value3', got %q", entry.Value)
		}
	} else {
		t.Error("Expected key 0x03 to exist")
	}

	// Verify old version is still accessible (CoW semantics)
	oldLeaf, err := leafStore.GetNode(oldRoot)
	if err != nil {
		t.Fatalf("Failed to read old leaf: %v", err)
	}

	// Old leaf should still have original values
	if entry, found := oldLeaf.Lookup(beatree.Key{0x01}); found {
		if string(entry.Value) != "value1" {
			t.Errorf("Old leaf: expected 'value1', got %q", entry.Value)
		}
	} else {
		t.Error("Old leaf: expected key 0x01 to exist")
	}
}

func TestUpdate_EmptyChangeset(t *testing.T) {
	leafStore := NewTestStore()

	// Create initial tree
	changeset1 := map[beatree.Key]*beatree.Change{
		{0x01}: beatree.NewInsertChange([]byte("value1")),
	}

	result1, err := Update(allocator.InvalidPageNumber, changeset1, nil, leafStore)
	if err != nil {
		t.Fatalf("Initial update failed: %v", err)
	}

	// Update with empty changeset
	result2, err := Update(result1.NewRoot, map[beatree.Key]*beatree.Change{}, nil, leafStore)
	if err != nil {
		t.Fatalf("Empty changeset update failed: %v", err)
	}

	// Should return same root
	if result2.NewRoot != result1.NewRoot {
		t.Error("Expected same root for empty changeset")
	}

	// Should not allocate or free any pages
	if len(result2.AllocatedPages) != 0 {
		t.Errorf("Expected 0 allocated pages, got %d", len(result2.AllocatedPages))
	}
	if len(result2.FreedPages) != 0 {
		t.Errorf("Expected 0 freed pages, got %d", len(result2.FreedPages))
	}
}

func TestMergeChangesets(t *testing.T) {
	cs1 := map[beatree.Key]*beatree.Change{
		{0x01}: beatree.NewInsertChange([]byte("v1")),
		{0x02}: beatree.NewInsertChange([]byte("v2")),
	}

	cs2 := map[beatree.Key]*beatree.Change{
		{0x02}: beatree.NewInsertChange([]byte("v2_updated")),
		{0x03}: beatree.NewInsertChange([]byte("v3")),
	}

	merged := MergeChangesets(cs1, cs2)

	// Should have 3 keys
	if len(merged) != 3 {
		t.Errorf("Expected 3 keys, got %d", len(merged))
	}

	// Key 0x01 should have v1
	if string(merged[beatree.Key{0x01}].Value) != "v1" {
		t.Error("Key 0x01 should be 'v1'")
	}

	// Key 0x02 should have updated value
	if string(merged[beatree.Key{0x02}].Value) != "v2_updated" {
		t.Error("Key 0x02 should be 'v2_updated'")
	}

	// Key 0x03 should have v3
	if string(merged[beatree.Key{0x03}].Value) != "v3" {
		t.Error("Key 0x03 should be 'v3'")
	}
}
