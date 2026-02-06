package ops

import (
	"testing"

	"github.com/jam-duna/jamduna/bmt/beatree"
	"github.com/jam-duna/jamduna/bmt/beatree/allocator"
	"github.com/jam-duna/jamduna/bmt/beatree/branch"
)

func TestReconstructFromBranches_Empty(t *testing.T) {
	index, err := ReconstructFromBranches(nil)
	if err != nil {
		t.Fatalf("Reconstruction failed: %v", err)
	}

	if index.Len() != 0 {
		t.Errorf("Expected empty index, got %d entries", index.Len())
	}
}

func TestReconstructFromBranches_SingleBranch(t *testing.T) {
	// Create a simple branch node
	branchNode := branch.NewNode(allocator.PageNumber(100))
	branchNode.Separators = []branch.Separator{
		{Key: beatree.Key{0x10}, Child: allocator.PageNumber(101)},
		{Key: beatree.Key{0x20}, Child: allocator.PageNumber(102)},
	}

	// Reconstruct index
	index, err := ReconstructFromBranches([]*branch.Node{branchNode})
	if err != nil {
		t.Fatalf("Reconstruction failed: %v", err)
	}

	// Should have one entry (indexed by first separator)
	if index.Len() != 1 {
		t.Errorf("Expected 1 index entry, got %d", index.Len())
	}

	// Verify the separator key is the first separator
	expectedKey := beatree.Key{0x10}
	_, _, found := index.Lookup(expectedKey)
	if !found {
		t.Error("Expected to find branch by first separator key")
	}
}

func TestReconstructFromBranches_MultipleBranches(t *testing.T) {
	// Create multiple branch nodes with different first separators
	branch1 := branch.NewNode(allocator.PageNumber(100))
	branch1.Separators = []branch.Separator{
		{Key: beatree.Key{0x10}, Child: allocator.PageNumber(101)},
		{Key: beatree.Key{0x20}, Child: allocator.PageNumber(102)},
	}

	branch2 := branch.NewNode(allocator.PageNumber(200))
	branch2.Separators = []branch.Separator{
		{Key: beatree.Key{0x30}, Child: allocator.PageNumber(201)},
		{Key: beatree.Key{0x40}, Child: allocator.PageNumber(202)},
	}

	branch3 := branch.NewNode(allocator.PageNumber(300))
	branch3.Separators = []branch.Separator{
		{Key: beatree.Key{0x50}, Child: allocator.PageNumber(301)},
	}

	// Reconstruct index
	index, err := ReconstructFromBranches([]*branch.Node{branch1, branch2, branch3})
	if err != nil {
		t.Fatalf("Reconstruction failed: %v", err)
	}

	// Should have three entries
	if index.Len() != 3 {
		t.Errorf("Expected 3 index entries, got %d", index.Len())
	}

	// Verify all branches can be found
	keys := []beatree.Key{{0x10}, {0x30}, {0x50}}
	for _, key := range keys {
		_, _, found := index.Lookup(key)
		if !found {
			t.Errorf("Expected to find branch with separator %v", key)
		}
	}
}

func TestReconstructFromBranches_DuplicateSeparator(t *testing.T) {
	// Create two branches with the same first separator (should error)
	branch1 := branch.NewNode(allocator.PageNumber(100))
	branch1.Separators = []branch.Separator{
		{Key: beatree.Key{0x10}, Child: allocator.PageNumber(101)},
	}

	branch2 := branch.NewNode(allocator.PageNumber(200))
	branch2.Separators = []branch.Separator{
		{Key: beatree.Key{0x10}, Child: allocator.PageNumber(201)}, // Duplicate!
	}

	// Should return error
	_, err := ReconstructFromBranches([]*branch.Node{branch1, branch2})
	if err == nil {
		t.Error("Expected error for duplicate separator, got nil")
	}
}

func TestReconstructFromBranches_SkipEmpty(t *testing.T) {
	// Create mix of valid and empty branches
	validBranch := branch.NewNode(allocator.PageNumber(100))
	validBranch.Separators = []branch.Separator{
		{Key: beatree.Key{0x10}, Child: allocator.PageNumber(101)},
	}

	emptyBranch := branch.NewNode(allocator.PageNumber(200))
	// No separators - should be skipped

	// Reconstruct index
	index, err := ReconstructFromBranches([]*branch.Node{validBranch, emptyBranch, nil})
	if err != nil {
		t.Fatalf("Reconstruction failed: %v", err)
	}

	// Should only have one entry (empty and nil branches skipped)
	if index.Len() != 1 {
		t.Errorf("Expected 1 index entry, got %d", index.Len())
	}
}

func TestExtractFirstSeparator(t *testing.T) {
	branchNode := branch.NewNode(allocator.PageNumber(100))
	branchNode.Separators = []branch.Separator{
		{Key: beatree.Key{0x10}, Child: allocator.PageNumber(101)},
		{Key: beatree.Key{0x20}, Child: allocator.PageNumber(102)},
	}

	key, err := ExtractFirstSeparator(branchNode)
	if err != nil {
		t.Fatalf("ExtractFirstSeparator failed: %v", err)
	}

	expected := beatree.Key{0x10}
	if key != expected {
		t.Errorf("Expected key %v, got %v", expected, key)
	}
}

func TestExtractFirstSeparator_Empty(t *testing.T) {
	branchNode := branch.NewNode(allocator.PageNumber(100))
	// No separators

	_, err := ExtractFirstSeparator(branchNode)
	if err == nil {
		t.Error("Expected error for empty branch, got nil")
	}
}

func TestExtractFirstSeparator_Nil(t *testing.T) {
	_, err := ExtractFirstSeparator(nil)
	if err == nil {
		t.Error("Expected error for nil branch, got nil")
	}
}

func TestValidateBranchNode(t *testing.T) {
	t.Run("valid branch", func(t *testing.T) {
		branchNode := branch.NewNode(allocator.PageNumber(100))
		branchNode.Separators = []branch.Separator{
			{Key: beatree.Key{0x10}, Child: allocator.PageNumber(101)},
			{Key: beatree.Key{0x20}, Child: allocator.PageNumber(102)},
		}

		if err := ValidateBranchNode(branchNode); err != nil {
			t.Errorf("Expected valid branch, got error: %v", err)
		}
	})

	t.Run("nil branch", func(t *testing.T) {
		if err := ValidateBranchNode(nil); err == nil {
			t.Error("Expected error for nil branch")
		}
	})

	t.Run("unsorted separators", func(t *testing.T) {
		branchNode := branch.NewNode(allocator.PageNumber(100))
		branchNode.Separators = []branch.Separator{
			{Key: beatree.Key{0x20}, Child: allocator.PageNumber(102)},
			{Key: beatree.Key{0x10}, Child: allocator.PageNumber(101)}, // Out of order
		}

		if err := ValidateBranchNode(branchNode); err == nil {
			t.Error("Expected error for unsorted separators")
		}
	})

	t.Run("invalid leftmost child", func(t *testing.T) {
		branchNode := branch.NewNode(allocator.InvalidPageNumber) // Invalid!
		branchNode.Separators = []branch.Separator{
			{Key: beatree.Key{0x10}, Child: allocator.PageNumber(101)},
		}

		if err := ValidateBranchNode(branchNode); err == nil {
			t.Error("Expected error for invalid leftmost child")
		}
	})

	t.Run("invalid separator child", func(t *testing.T) {
		branchNode := branch.NewNode(allocator.PageNumber(100))
		branchNode.Separators = []branch.Separator{
			{Key: beatree.Key{0x10}, Child: allocator.InvalidPageNumber}, // Invalid!
		}

		if err := ValidateBranchNode(branchNode); err == nil {
			t.Error("Expected error for invalid separator child")
		}
	})
}
