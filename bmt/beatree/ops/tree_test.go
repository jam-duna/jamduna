package ops

import (
	"bytes"
	"testing"

	"github.com/jam-duna/jamduna/bmt/beatree"
	"github.com/jam-duna/jamduna/bmt/beatree/allocator"
)

func setupTestTree(t *testing.T) *Tree {
	branchStore := NewTestStore()
	leafStore := NewTestStore()
	return NewTree(branchStore, leafStore)
}

func TestTreeLookup(t *testing.T) {
	tree := setupTestTree(t)

	key1 := beatree.KeyFromBytes(bytes.Repeat([]byte{1}, 32))
	value1 := []byte("value1")

	// Insert and commit
	if err := tree.Insert(key1, value1); err != nil {
		t.Fatalf("Insert failed: %v", err)
	}

	if _, err := tree.Commit(); err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	// Lookup
	result, err := tree.Lookup(key1)
	if err != nil {
		t.Fatalf("Lookup failed: %v", err)
	}

	if !bytes.Equal(result, value1) {
		t.Errorf("Expected value %v, got %v", value1, result)
	}
}

func TestTreeLookupMissing(t *testing.T) {
	tree := setupTestTree(t)

	key1 := beatree.KeyFromBytes(bytes.Repeat([]byte{1}, 32))

	// Lookup non-existent key
	result, err := tree.Lookup(key1)
	if err != nil {
		t.Fatalf("Lookup failed: %v", err)
	}

	if result != nil {
		t.Errorf("Expected nil for missing key, got %v", result)
	}
}

func TestTreeInsert(t *testing.T) {
	tree := setupTestTree(t)

	key1 := beatree.KeyFromBytes(bytes.Repeat([]byte{1}, 32))
	key2 := beatree.KeyFromBytes(bytes.Repeat([]byte{2}, 32))
	value1 := []byte("value1")
	value2 := []byte("value2")

	// Insert multiple keys
	if err := tree.Insert(key1, value1); err != nil {
		t.Fatalf("Insert key1 failed: %v", err)
	}

	if err := tree.Insert(key2, value2); err != nil {
		t.Fatalf("Insert key2 failed: %v", err)
	}

	if _, err := tree.Commit(); err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	// Verify both keys
	result1, _ := tree.Lookup(key1)
	if !bytes.Equal(result1, value1) {
		t.Errorf("Key1 value mismatch")
	}

	result2, _ := tree.Lookup(key2)
	if !bytes.Equal(result2, value2) {
		t.Errorf("Key2 value mismatch")
	}
}

func TestTreeUpdate(t *testing.T) {
	tree := setupTestTree(t)

	key := beatree.KeyFromBytes(bytes.Repeat([]byte{1}, 32))
	value1 := []byte("original")
	value2 := []byte("updated")

	// Insert original value
	tree.Insert(key, value1)
	tree.Commit()

	// Verify original
	result, err := tree.Lookup(key)
	if err != nil {
		t.Fatalf("Lookup failed: %v", err)
	}
	if !bytes.Equal(result, value1) {
		t.Errorf("Original value mismatch")
	}

	// Update value
	tree.Insert(key, value2)
	tree.Commit()

	// Verify updated
	result, err = tree.Lookup(key)
	if err != nil {
		t.Fatalf("Lookup failed: %v", err)
	}
	if !bytes.Equal(result, value2) {
		t.Errorf("Updated value mismatch")
	}
}

func TestTreeDelete(t *testing.T) {
	tree := setupTestTree(t)

	key := beatree.KeyFromBytes(bytes.Repeat([]byte{1}, 32))
	value := []byte("value")

	// Insert and commit
	tree.Insert(key, value)
	tree.Commit()

	// Verify exists
	result, _ := tree.Lookup(key)
	if result == nil {
		t.Fatalf("Key should exist after insert")
	}

	// Delete and commit
	tree.Delete(key)
	tree.Commit()

	// Verify deleted
	result, _ = tree.Lookup(key)
	if result != nil {
		t.Errorf("Key should be deleted, but got value: %v", result)
	}
}

func TestTreeCoW(t *testing.T) {
	tree := setupTestTree(t)

	key := beatree.KeyFromBytes(bytes.Repeat([]byte{1}, 32))
	value1 := []byte("version1")
	value2 := []byte("version2")

	// Insert and commit version 1
	tree.Insert(key, value1)
	root1, err := tree.Commit()
	if err != nil {
		t.Fatalf("Commit v1 failed: %v", err)
	}

	// Update and commit version 2
	tree.Insert(key, value2)
	root2, err := tree.Commit()
	if err != nil {
		t.Fatalf("Commit v2 failed: %v", err)
	}

	// Verify root pages are different (CoW created new page)
	if root1 == root2 {
		t.Errorf("CoW should create new root page, but roots are the same: %d", root1)
	}

	if root1 == allocator.InvalidPageNumber {
		t.Errorf("Root1 should be valid")
	}

	if root2 == allocator.InvalidPageNumber {
		t.Errorf("Root2 should be valid")
	}

	// Current tree should have version 2
	result, _ := tree.Lookup(key)
	if !bytes.Equal(result, value2) {
		t.Errorf("Current tree should have version 2")
	}
}

func TestTreeMultipleKeys(t *testing.T) {
	tree := setupTestTree(t)

	// Insert 10 keys
	for i := 0; i < 10; i++ {
		key := beatree.KeyFromBytes(bytes.Repeat([]byte{byte(i)}, 32))
		value := []byte{byte(i * 10)}

		if err := tree.Insert(key, value); err != nil {
			t.Fatalf("Insert key %d failed: %v", i, err)
		}
	}

	if _, err := tree.Commit(); err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	// Verify all keys
	for i := 0; i < 10; i++ {
		key := beatree.KeyFromBytes(bytes.Repeat([]byte{byte(i)}, 32))
		expectedValue := []byte{byte(i * 10)}

		result, err := tree.Lookup(key)
		if err != nil {
			t.Fatalf("Lookup key %d failed: %v", i, err)
		}

		if !bytes.Equal(result, expectedValue) {
			t.Errorf("Key %d: expected %v, got %v", i, expectedValue, result)
		}
	}
}

func TestTreeEmptyCommit(t *testing.T) {
	tree := setupTestTree(t)

	// Commit with no changes
	root1 := tree.Root()
	root2, err := tree.Commit()

	if err != nil {
		t.Fatalf("Empty commit should not fail: %v", err)
	}

	if root1 != root2 {
		t.Errorf("Empty commit should not change root")
	}
}

func TestTreeStagingIsolation(t *testing.T) {
	tree := setupTestTree(t)

	key := beatree.KeyFromBytes(bytes.Repeat([]byte{1}, 32))
	value := []byte("staged")

	// Insert but don't commit
	tree.Insert(key, value)

	// Should see staged value
	result, _ := tree.Lookup(key)
	if !bytes.Equal(result, value) {
		t.Errorf("Should see staged value before commit")
	}

	// Create new tree instance (simulating another reader)
	tree2 := setupTestTree(t)

	// Should not see uncommitted value
	result2, _ := tree2.Lookup(key)
	if result2 != nil {
		t.Errorf("Other tree should not see uncommitted value")
	}
}


func TestTreeHistoricalRootAccess(t *testing.T) {
	// This test verifies true CoW semantics: old roots remain readable after new commits
	leafStore := NewTestStore()
	branchStore := NewTestStore()
	tree := NewTree(branchStore, leafStore)

	key := beatree.KeyFromBytes(bytes.Repeat([]byte{1}, 32))
	value1 := []byte("version1")
	value2 := []byte("version2")
	value3 := []byte("version3")

	// Commit version 1
	tree.Insert(key, value1)
	root1, err := tree.Commit()
	if err != nil {
		t.Fatalf("Commit v1 failed: %v", err)
	}

	// Commit version 2
	tree.Insert(key, value2)
	root2, err := tree.Commit()
	if err != nil {
		t.Fatalf("Commit v2 failed: %v", err)
	}

	// Commit version 3
	tree.Insert(key, value3)
	root3, err := tree.Commit()
	if err != nil {
		t.Fatalf("Commit v3 failed: %v", err)
	}

	// All three roots should be different
	if root1 == root2 || root2 == root3 || root1 == root3 {
		t.Errorf("Each commit should create a new root page: root1=%d, root2=%d, root3=%d", root1, root2, root3)
	}

	// CRITICAL TEST: Create new tree instances pointing to old roots
	// This simulates historical queries / snapshot reads
	tree1 := &Tree{
		root:        root1,
		branchStore: branchStore,
		leafStore:   leafStore,
		staging:     make(map[beatree.Key]*beatree.Change),
	}

	tree2 := &Tree{
		root:        root2,
		branchStore: branchStore,
		leafStore:   leafStore,
		staging:     make(map[beatree.Key]*beatree.Change),
	}

	tree3 := &Tree{
		root:        root3,
		branchStore: branchStore,
		leafStore:   leafStore,
		staging:     make(map[beatree.Key]*beatree.Change),
	}

	// Verify each historical root returns its correct version
	result1, err := tree1.Lookup(key)
	if err != nil {
		t.Fatalf("Failed to lookup in root1: %v", err)
	}
	if !bytes.Equal(result1, value1) {
		t.Errorf("root1: expected %q, got %q", value1, result1)
	}

	result2, err := tree2.Lookup(key)
	if err != nil {
		t.Fatalf("Failed to lookup in root2: %v", err)
	}
	if !bytes.Equal(result2, value2) {
		t.Errorf("root2: expected %q, got %q", value2, result2)
	}

	result3, err := tree3.Lookup(key)
	if err != nil {
		t.Fatalf("Failed to lookup in root3: %v", err)
	}
	if !bytes.Equal(result3, value3) {
		t.Errorf("root3: expected %q, got %q", value3, result3)
	}

	t.Logf("✅ CoW semantics verified: all 3 historical roots remain accessible")
}
