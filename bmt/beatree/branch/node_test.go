package branch

import (
	"testing"

	"github.com/colorfulnotion/jam/bmt/beatree"
	"github.com/colorfulnotion/jam/bmt/beatree/allocator"
)

func TestBranchNodeInsert(t *testing.T) {
	node := NewNode(allocator.PageNumber(0))

	key1 := beatree.Key{0x00, 0x00, 0x00, 0x01}
	key2 := beatree.Key{0x00, 0x00, 0x00, 0x02}
	key3 := beatree.Key{0x00, 0x00, 0x00, 0x03}

	if err := node.Insert(Separator{Key: key2, Child: allocator.PageNumber(2)}); err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	if err := node.Insert(Separator{Key: key1, Child: allocator.PageNumber(1)}); err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	if err := node.Insert(Separator{Key: key3, Child: allocator.PageNumber(3)}); err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	// Check order
	if node.NumSeparators() != 3 {
		t.Errorf("num separators = %d, want 3", node.NumSeparators())
	}

	if !node.Separators[0].Key.Equal(key1) {
		t.Error("separator 0 should be key1")
	}

	if !node.Separators[1].Key.Equal(key2) {
		t.Error("separator 1 should be key2")
	}

	if !node.Separators[2].Key.Equal(key3) {
		t.Error("separator 2 should be key3")
	}
}

func TestBranchNodeFindChild(t *testing.T) {
	node := NewNode(allocator.PageNumber(0))

	key1 := beatree.Key{0x00, 0x00, 0x00, 0x10}
	key2 := beatree.Key{0x00, 0x00, 0x00, 0x20}
	key3 := beatree.Key{0x00, 0x00, 0x00, 0x30}

	node.Insert(Separator{Key: key1, Child: allocator.PageNumber(1)})
	node.Insert(Separator{Key: key2, Child: allocator.PageNumber(2)})
	node.Insert(Separator{Key: key3, Child: allocator.PageNumber(3)})

	// Test finding children
	testKey := beatree.Key{0x00, 0x00, 0x00, 0x05} // < key1
	if child := node.FindChild(testKey); child != allocator.PageNumber(0) {
		t.Errorf("FindChild(%v) = %v, want 0", testKey, child)
	}

	testKey = beatree.Key{0x00, 0x00, 0x00, 0x15} // Between key1 and key2
	if child := node.FindChild(testKey); child != allocator.PageNumber(1) {
		t.Errorf("FindChild(%v) = %v, want 1", testKey, child)
	}

	testKey = beatree.Key{0x00, 0x00, 0x00, 0x25} // Between key2 and key3
	if child := node.FindChild(testKey); child != allocator.PageNumber(2) {
		t.Errorf("FindChild(%v) = %v, want 2", testKey, child)
	}

	testKey = beatree.Key{0x00, 0x00, 0x00, 0x35} // > key3
	if child := node.FindChild(testKey); child != allocator.PageNumber(3) {
		t.Errorf("FindChild(%v) = %v, want 3", testKey, child)
	}
}

func TestBranchNodeDuplicateKey(t *testing.T) {
	node := NewNode(allocator.PageNumber(0))

	key := beatree.Key{0x00, 0x00, 0x00, 0x01}

	if err := node.Insert(Separator{Key: key, Child: allocator.PageNumber(1)}); err != nil {
		t.Fatalf("first insert failed: %v", err)
	}

	if err := node.Insert(Separator{Key: key, Child: allocator.PageNumber(2)}); err == nil {
		t.Error("duplicate key insert should fail")
	}
}
