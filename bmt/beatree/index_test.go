package beatree

import (
	"bytes"
	"testing"
)

// TestIndexEmpty tests operations on an empty index.
func TestIndexEmpty(t *testing.T) {
	idx := NewIndex()

	if idx.Len() != 0 {
		t.Errorf("Empty index should have len 0, got %d", idx.Len())
	}

	// Lookup on empty index
	_, _, found := idx.Lookup(keyFromInt(5))
	if found {
		t.Error("Lookup on empty index should return false")
	}

	// NextKey on empty index
	_, found = idx.NextKey(keyFromInt(5))
	if found {
		t.Error("NextKey on empty index should return false")
	}

	// Remove from empty index
	_, existed := idx.Remove(keyFromInt(5))
	if existed {
		t.Error("Remove from empty index should return false")
	}
}

// TestIndexInsertLookup tests basic insert and lookup.
func TestIndexInsertLookup(t *testing.T) {
	idx := NewIndex()

	// Insert some branches
	branch1 := []byte("branch1")
	branch2 := []byte("branch2")
	branch3 := []byte("branch3")

	idx.Insert(keyFromInt(10), branch1)
	idx.Insert(keyFromInt(20), branch2)
	idx.Insert(keyFromInt(30), branch3)

	if idx.Len() != 3 {
		t.Errorf("Expected len 3, got %d", idx.Len())
	}

	// Lookup exact match
	sep, branch, found := idx.Lookup(keyFromInt(20))
	if !found {
		t.Fatal("Expected to find branch at key 20")
	}
	if sep != keyFromInt(20) {
		t.Errorf("Expected separator 20, got %v", sep)
	}
	if !bytes.Equal(branch, branch2) {
		t.Errorf("Expected branch2, got %v", branch)
	}

	// Lookup between separators - should return lower bound
	sep, branch, found = idx.Lookup(keyFromInt(25))
	if !found {
		t.Fatal("Expected to find branch at key 25 (should return 20)")
	}
	if sep != keyFromInt(20) {
		t.Errorf("Expected separator 20, got %v", sep)
	}
	if !bytes.Equal(branch, branch2) {
		t.Errorf("Expected branch2, got %v", branch)
	}

	// Lookup above all separators
	sep, branch, found = idx.Lookup(keyFromInt(35))
	if !found {
		t.Fatal("Expected to find branch at key 35 (should return 30)")
	}
	if sep != keyFromInt(30) {
		t.Errorf("Expected separator 30, got %v", sep)
	}
	if !bytes.Equal(branch, branch3) {
		t.Errorf("Expected branch3, got %v", branch)
	}

	// Lookup below all separators
	_, _, found = idx.Lookup(keyFromInt(5))
	if found {
		t.Error("Lookup below all separators should return false")
	}
}

// TestIndexNextKey tests the NextKey operation.
func TestIndexNextKey(t *testing.T) {
	idx := NewIndex()

	// Insert separators 10, 20, 30
	idx.Insert(keyFromInt(10), []byte("b1"))
	idx.Insert(keyFromInt(20), []byte("b2"))
	idx.Insert(keyFromInt(30), []byte("b3"))

	// NextKey(5) should return 10
	next, found := idx.NextKey(keyFromInt(5))
	if !found {
		t.Fatal("Expected to find next key after 5")
	}
	if next != keyFromInt(10) {
		t.Errorf("Expected next key 10, got %v", next)
	}

	// NextKey(10) should return 20
	next, found = idx.NextKey(keyFromInt(10))
	if !found {
		t.Fatal("Expected to find next key after 10")
	}
	if next != keyFromInt(20) {
		t.Errorf("Expected next key 20, got %v", next)
	}

	// NextKey(15) should return 20
	next, found = idx.NextKey(keyFromInt(15))
	if !found {
		t.Fatal("Expected to find next key after 15")
	}
	if next != keyFromInt(20) {
		t.Errorf("Expected next key 20, got %v", next)
	}

	// NextKey(30) should return nothing
	_, found = idx.NextKey(keyFromInt(30))
	if found {
		t.Error("NextKey after last separator should return false")
	}

	// NextKey(35) should return nothing
	_, found = idx.NextKey(keyFromInt(35))
	if found {
		t.Error("NextKey above all separators should return false")
	}
}

// TestIndexRemove tests branch removal.
func TestIndexRemove(t *testing.T) {
	idx := NewIndex()

	// Insert and remove
	branch1 := []byte("branch1")
	idx.Insert(keyFromInt(10), branch1)
	idx.Insert(keyFromInt(20), []byte("branch2"))

	removed, existed := idx.Remove(keyFromInt(10))
	if !existed {
		t.Fatal("Expected branch to exist before removal")
	}
	if !bytes.Equal(removed, branch1) {
		t.Errorf("Expected removed branch to be branch1, got %v", removed)
	}

	if idx.Len() != 1 {
		t.Errorf("Expected len 1 after removal, got %d", idx.Len())
	}

	// Lookup removed key should fail
	_, _, found := idx.Lookup(keyFromInt(10))
	if found {
		t.Error("Lookup of removed key should return false")
	}

	// Remove non-existent key
	_, existed = idx.Remove(keyFromInt(99))
	if existed {
		t.Error("Remove non-existent key should return false")
	}
}

// TestIndexUpdate tests updating existing branches.
func TestIndexUpdate(t *testing.T) {
	idx := NewIndex()

	// Insert initial branch
	branch1 := []byte("branch1")
	prev, existed := idx.Insert(keyFromInt(10), branch1)
	if existed {
		t.Error("First insert should return false for existed")
	}
	if prev != nil {
		t.Error("First insert should return nil for previous")
	}

	// Update with new branch
	branch2 := []byte("branch2")
	prev, existed = idx.Insert(keyFromInt(10), branch2)
	if !existed {
		t.Error("Update should return true for existed")
	}
	if !bytes.Equal(prev, branch1) {
		t.Errorf("Update should return old branch, got %v", prev)
	}

	// Verify new branch is stored
	_, branch, found := idx.Lookup(keyFromInt(10))
	if !found {
		t.Fatal("Expected to find updated branch")
	}
	if !bytes.Equal(branch, branch2) {
		t.Errorf("Expected branch2, got %v", branch)
	}

	// Length should still be 1
	if idx.Len() != 1 {
		t.Errorf("Expected len 1 after update, got %d", idx.Len())
	}
}

// TestIndexKeys tests key enumeration.
func TestIndexKeys(t *testing.T) {
	idx := NewIndex()

	// Insert out of order
	idx.Insert(keyFromInt(30), []byte("b3"))
	idx.Insert(keyFromInt(10), []byte("b1"))
	idx.Insert(keyFromInt(20), []byte("b2"))

	keys := idx.Keys()
	if len(keys) != 3 {
		t.Fatalf("Expected 3 keys, got %d", len(keys))
	}

	// Should be sorted
	expected := []int{10, 20, 30}
	for i, exp := range expected {
		if keys[i] != keyFromInt(exp) {
			t.Errorf("Key %d: expected %v, got %v", i, keyFromInt(exp), keys[i])
		}
	}
}

// TestIndexClone tests index cloning.
func TestIndexClone(t *testing.T) {
	idx1 := NewIndex()

	// Insert some branches
	idx1.Insert(keyFromInt(10), []byte("b1"))
	idx1.Insert(keyFromInt(20), []byte("b2"))

	// Clone
	idx2 := idx1.Clone()

	// Verify clone has same data
	if idx2.Len() != idx1.Len() {
		t.Errorf("Clone should have same length as original")
	}

	_, branch, found := idx2.Lookup(keyFromInt(10))
	if !found || !bytes.Equal(branch, []byte("b1")) {
		t.Error("Clone should have same data as original")
	}

	// Modify original
	idx1.Insert(keyFromInt(30), []byte("b3"))

	// Clone should not be affected
	if idx2.Len() != 2 {
		t.Errorf("Clone should not be affected by changes to original, len=%d", idx2.Len())
		t.Logf("idx2 keys: %v", idx2.Keys())
	}

	sep30, _, found30 := idx2.Lookup(keyFromInt(30))
	// Lookup will return true (finding separator 20) but separator should NOT be 30
	if found30 && sep30 == keyFromInt(30) {
		t.Error("Clone should not have separator 30 from original")
	}

	// Better test: check if separator 30 exists directly
	_, existed30 := idx2.Remove(keyFromInt(30))
	if existed30 {
		t.Error("Clone should not have separator 30 from original")
	}
}

// TestIndexLookupEdgeCases tests edge cases in lookup.
func TestIndexLookupEdgeCases(t *testing.T) {
	idx := NewIndex()

	// Single branch
	idx.Insert(keyFromInt(50), []byte("b1"))

	// Lookup exactly at separator
	sep, _, found := idx.Lookup(keyFromInt(50))
	if !found || sep != keyFromInt(50) {
		t.Error("Lookup at exact separator should succeed")
	}

	// Lookup above separator
	sep, _, found = idx.Lookup(keyFromInt(100))
	if !found || sep != keyFromInt(50) {
		t.Error("Lookup above separator should return that separator")
	}

	// Lookup below separator
	_, _, found = idx.Lookup(keyFromInt(25))
	if found {
		t.Error("Lookup below only separator should fail")
	}

	// Add a second branch
	idx.Insert(keyFromInt(100), []byte("b2"))

	// Lookup between separators
	sep, _, found = idx.Lookup(keyFromInt(75))
	if !found || sep != keyFromInt(50) {
		t.Error("Lookup between separators should return lower separator")
	}
}
