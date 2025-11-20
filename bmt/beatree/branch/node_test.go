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

// TestUpdateChild_Leftmost verifies updating the leftmost child
func TestUpdateChild_Leftmost(t *testing.T) {
	node := NewNode(allocator.PageNumber(100))

	// Add some separators
	key1 := beatree.Key{0x00, 0x00, 0x00, 0x10}
	key2 := beatree.Key{0x00, 0x00, 0x00, 0x20}
	node.Insert(Separator{Key: key1, Child: allocator.PageNumber(200)})
	node.Insert(Separator{Key: key2, Child: allocator.PageNumber(300)})

	// Update leftmost child
	found, err := node.UpdateChild(allocator.PageNumber(100), allocator.PageNumber(150))
	if err != nil {
		t.Fatalf("UpdateChild failed: %v", err)
	}
	if !found {
		t.Error("UpdateChild should return found=true for leftmost child")
	}

	// Verify update
	if node.LeftmostChild != allocator.PageNumber(150) {
		t.Errorf("LeftmostChild = %v, want 150", node.LeftmostChild)
	}

	// Verify separators unchanged
	if node.Separators[0].Child != allocator.PageNumber(200) {
		t.Error("separator 0 child should be unchanged")
	}
	if node.Separators[1].Child != allocator.PageNumber(300) {
		t.Error("separator 1 child should be unchanged")
	}
}

// TestUpdateChild_Separator verifies updating a separator's child
func TestUpdateChild_Separator(t *testing.T) {
	node := NewNode(allocator.PageNumber(100))

	// Add separators
	key1 := beatree.Key{0x00, 0x00, 0x00, 0x10}
	key2 := beatree.Key{0x00, 0x00, 0x00, 0x20}
	key3 := beatree.Key{0x00, 0x00, 0x00, 0x30}
	node.Insert(Separator{Key: key1, Child: allocator.PageNumber(200)})
	node.Insert(Separator{Key: key2, Child: allocator.PageNumber(300)})
	node.Insert(Separator{Key: key3, Child: allocator.PageNumber(400)})

	// Update middle separator's child
	found, err := node.UpdateChild(allocator.PageNumber(300), allocator.PageNumber(350))
	if err != nil {
		t.Fatalf("UpdateChild failed: %v", err)
	}
	if !found {
		t.Error("UpdateChild should return found=true")
	}

	// Verify update
	if node.Separators[1].Child != allocator.PageNumber(350) {
		t.Errorf("separator 1 child = %v, want 350", node.Separators[1].Child)
	}

	// Verify other children unchanged
	if node.LeftmostChild != allocator.PageNumber(100) {
		t.Error("leftmost child should be unchanged")
	}
	if node.Separators[0].Child != allocator.PageNumber(200) {
		t.Error("separator 0 child should be unchanged")
	}
	if node.Separators[2].Child != allocator.PageNumber(400) {
		t.Error("separator 2 child should be unchanged")
	}
}

// TestUpdateChild_NotFound verifies behavior when child not found
func TestUpdateChild_NotFound(t *testing.T) {
	node := NewNode(allocator.PageNumber(100))

	key := beatree.Key{0x00, 0x00, 0x00, 0x10}
	node.Insert(Separator{Key: key, Child: allocator.PageNumber(200)})

	// Try to update non-existent child
	found, err := node.UpdateChild(allocator.PageNumber(999), allocator.PageNumber(111))
	if err != nil {
		t.Fatalf("UpdateChild failed: %v", err)
	}
	if found {
		t.Error("UpdateChild should return found=false for non-existent child")
	}

	// Verify nothing changed
	if node.LeftmostChild != allocator.PageNumber(100) {
		t.Error("leftmost child should be unchanged")
	}
	if node.Separators[0].Child != allocator.PageNumber(200) {
		t.Error("separator child should be unchanged")
	}
}

// TestInsertSeparator verifies InsertSeparator works correctly
func TestInsertSeparator(t *testing.T) {
	node := NewNode(allocator.PageNumber(0))

	key1 := beatree.Key{0x00, 0x00, 0x00, 0x10}
	key2 := beatree.Key{0x00, 0x00, 0x00, 0x20}

	// Insert separators using InsertSeparator
	if err := node.InsertSeparator(Separator{Key: key2, Child: allocator.PageNumber(2)}); err != nil {
		t.Fatalf("InsertSeparator failed: %v", err)
	}

	if err := node.InsertSeparator(Separator{Key: key1, Child: allocator.PageNumber(1)}); err != nil {
		t.Fatalf("InsertSeparator failed: %v", err)
	}

	// Verify order maintained
	if node.NumSeparators() != 2 {
		t.Errorf("num separators = %d, want 2", node.NumSeparators())
	}
	if !node.Separators[0].Key.Equal(key1) {
		t.Error("separator 0 should be key1")
	}
	if !node.Separators[1].Key.Equal(key2) {
		t.Error("separator 1 should be key2")
	}
}

// TestClone_Shallow verifies shallow copy semantics for branch nodes
func TestClone_Shallow(t *testing.T) {
	original := NewNode(allocator.PageNumber(100))

	// Insert separators
	key1 := beatree.Key{0x00, 0x00, 0x00, 0x10}
	key2 := beatree.Key{0x00, 0x00, 0x00, 0x20}
	original.Insert(Separator{Key: key1, Child: allocator.PageNumber(200)})
	original.Insert(Separator{Key: key2, Child: allocator.PageNumber(300)})

	// Clone
	cloned := original.Clone()

	if cloned == nil {
		t.Fatal("Clone returned nil")
	}

	// Verify independent Separators slice
	if &original.Separators == &cloned.Separators {
		t.Error("Clone should create new Separators slice")
	}

	// Verify same number of separators
	if cloned.NumSeparators() != 2 {
		t.Errorf("cloned num separators = %d, want 2", cloned.NumSeparators())
	}

	// Verify leftmost child copied
	if cloned.LeftmostChild != allocator.PageNumber(100) {
		t.Errorf("cloned leftmost child = %v, want 100", cloned.LeftmostChild)
	}

	// Verify separators copied
	if !cloned.Separators[0].Key.Equal(key1) {
		t.Error("cloned separator 0 key mismatch")
	}
	if cloned.Separators[0].Child != allocator.PageNumber(200) {
		t.Error("cloned separator 0 child mismatch")
	}
	if !cloned.Separators[1].Key.Equal(key2) {
		t.Error("cloned separator 1 key mismatch")
	}
	if cloned.Separators[1].Child != allocator.PageNumber(300) {
		t.Error("cloned separator 1 child mismatch")
	}
}

// TestClone_Independence verifies modifications don't affect original
func TestClone_Independence(t *testing.T) {
	original := NewNode(allocator.PageNumber(100))

	key1 := beatree.Key{0x00, 0x00, 0x00, 0x10}
	key2 := beatree.Key{0x00, 0x00, 0x00, 0x20}
	original.Insert(Separator{Key: key1, Child: allocator.PageNumber(200)})
	original.Insert(Separator{Key: key2, Child: allocator.PageNumber(300)})

	// Clone
	cloned := original.Clone()

	// Modify cloned node
	key3 := beatree.Key{0x00, 0x00, 0x00, 0x30}
	cloned.Insert(Separator{Key: key3, Child: allocator.PageNumber(400)})
	cloned.LeftmostChild = allocator.PageNumber(999)

	// Verify original unchanged
	if original.NumSeparators() != 2 {
		t.Errorf("original num separators = %d, want 2", original.NumSeparators())
	}
	if original.LeftmostChild != allocator.PageNumber(100) {
		t.Errorf("original leftmost child = %v, want 100", original.LeftmostChild)
	}

	// Verify clone has new separator
	if cloned.NumSeparators() != 3 {
		t.Errorf("cloned num separators = %d, want 3", cloned.NumSeparators())
	}
	if cloned.LeftmostChild != allocator.PageNumber(999) {
		t.Errorf("cloned leftmost child = %v, want 999", cloned.LeftmostChild)
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
	original := NewNode(allocator.PageNumber(100))
	cloned := original.Clone()

	if cloned == nil {
		t.Fatal("Clone of empty node should not return nil")
	}

	if cloned.NumSeparators() != 0 {
		t.Errorf("cloned empty node should have 0 separators, got %d", cloned.NumSeparators())
	}

	if cloned.LeftmostChild != allocator.PageNumber(100) {
		t.Errorf("cloned leftmost child = %v, want 100", cloned.LeftmostChild)
	}
}
