package core

import (
	"testing"
)

// TestChildNodeIndicesInNextPage tests that InNextPage correctly identifies
// when child nodes are at the page boundary (depth 6), meaning THEIR children
// would be in the next page.
func TestChildNodeIndicesInNextPage(t *testing.T) {
	// Test case 1: At depth 4, children are at depth 5 (NOT at boundary)
	// InNextPage should be false
	path1 := KeyPath{}
	tp1 := NewTriePositionFromPath(path1, 4)
	children1 := tp1.ChildNodeIndices()
	if children1.InNextPage() {
		t.Errorf("At depth 4, children (depth 5) should NOT be in next page, but InNextPage() = true")
	}

	// Test case 2: At depth 5, children are at depth 6 (page boundary)
	// InNextPage should be TRUE because depth-6 nodes' children are in child pages
	path2 := KeyPath{}
	tp2 := NewTriePositionFromPath(path2, 5)
	children2 := tp2.ChildNodeIndices()
	if !children2.InNextPage() {
		t.Errorf("At depth 5, children (depth 6) ARE at page boundary, InNextPage() should be true")
	}

	// Test case 3: At depth 1, children at depth 2 (not at boundary)
	path3 := KeyPath{}
	tp3 := NewTriePositionFromPath(path3, 1)
	children3 := tp3.ChildNodeIndices()
	if children3.InNextPage() {
		t.Errorf("At depth 1, children (depth 2) should NOT be in next page")
	}

	// Test case 4: Verify across different paths at depth 5
	// All should return true for InNextPage
	for i := 0; i < 32; i++ {
		path := KeyPath{}
		// Set varying bit patterns
		for j := 0; j < 5; j++ {
			if (i & (1 << j)) != 0 {
				setBit(path[:], j, true)
			}
		}
		tp := NewTriePositionFromPath(path, 5)
		children := tp.ChildNodeIndices()
		if !children.InNextPage() {
			t.Errorf("At depth 5 with path pattern %d, InNextPage() should be true", i)
		}
	}

	t.Logf("✅ InNextPage() correctly identifies depth-6 boundary (parent at depth 5)")
}

// TestTriePositionNavigation tests basic navigation functions.
func TestTriePositionNavigation(t *testing.T) {
	tp := NewTriePosition()

	if !tp.IsRoot() {
		t.Error("New position should be at root")
	}

	// Move down left (bit 0)
	tp.Down(false)
	if tp.Depth() != 1 {
		t.Errorf("Expected depth 1, got %d", tp.Depth())
	}
	if tp.PeekLastBit() != false {
		t.Error("Expected last bit to be false")
	}

	// Move down right (bit 1)
	tp.Down(true)
	if tp.Depth() != 2 {
		t.Errorf("Expected depth 2, got %d", tp.Depth())
	}
	if tp.PeekLastBit() != true {
		t.Error("Expected last bit to be true")
	}

	// Move up 1
	tp.Up(1)
	if tp.Depth() != 1 {
		t.Errorf("Expected depth 1, got %d", tp.Depth())
	}

	// Test sibling
	tp.Sibling()
	if tp.PeekLastBit() != true {
		t.Error("Expected last bit to be true after sibling move")
	}
}
