package ops

import (
	"fmt"

	"github.com/colorfulnotion/jam/bmt/beatree"
	"github.com/colorfulnotion/jam/bmt/beatree/allocator"
	"github.com/colorfulnotion/jam/bmt/beatree/branch"
)

// Reconstruct rebuilds the in-memory branch index from the branch node store.
//
// Algorithm:
//  1. Read all bottom-level branch nodes (BBNs) from storage
//  2. Skip freed pages using the provided freelist
//  3. Order them by their first separator key
//  4. Build the index mapping separator -> branch node
//
// Works with any PageStore implementation (TestStore for tests, allocator.Store for production).
// Currently handles single-level branch reconstruction.
//
// Future enhancements:
//  - Use mmap for sequential file reading (performance optimization)
//  - Multi-level branch reconstruction (for very large trees)
func Reconstruct(
	branchStore PageStore,
	freeList map[allocator.PageNumber]bool,
	bump allocator.PageNumber,
) (*beatree.Index, error) {
	index := beatree.NewIndex()

	// Iterate through all allocated pages up to bump
	for pn := allocator.PageNumber(1); pn < bump; pn++ {
		// Skip pages that are on the freelist
		if freeList != nil && freeList[pn] {
			continue
		}

		// Try to read and deserialize branch node data
		pageData, err := branchStore.Page(pn)
		if err != nil {
			// Page doesn't exist or error reading - skip it
			continue
		}

		// Try to deserialize as a branch node
		branchNode, err := branch.DeserializeBranchNode(pageData)
		if err != nil {
			// Not a valid branch page - skip it
			continue
		}

		// Skip empty branch nodes
		if len(branchNode.Separators) == 0 {
			continue
		}

		// Extract first separator for indexing
		firstSeparator := branchNode.Separators[0]
		// Store the serialized branch data in the index
		index.Insert(firstSeparator.Key, pageData)
	}

	return index, nil
}

// ReconstructFromBranches rebuilds an index from a list of branch nodes.
// This is a helper for testing and for incremental index updates.
func ReconstructFromBranches(branches []*branch.Node) (*beatree.Index, error) {
	index := beatree.NewIndex()

	for _, branchNode := range branches {
		if branchNode == nil {
			continue
		}

		// Skip empty branch nodes
		if len(branchNode.Separators) == 0 {
			continue
		}

		// The first separator is the key for this branch in the index
		separator := branchNode.Separators[0].Key

		// Serialize the branch node to bytes
		serialized, err := branchNode.Serialize()
		if err != nil {
			return nil, fmt.Errorf("failed to serialize branch node: %w", err)
		}

		// Insert into index
		_, existed := index.Insert(separator, serialized)
		if existed {
			return nil, fmt.Errorf("duplicate separator key: %v", separator)
		}
	}

	return index, nil
}

// ExtractFirstSeparator extracts the first separator key from a branch node.
// This is used to determine the index key for the branch.
//
// The separator is the first key in the separators list.
// Note: The Go implementation stores complete keys in separators.
// The Rust implementation uses prefix compression (prefix bits + separator bits).
func ExtractFirstSeparator(branchNode *branch.Node) (beatree.Key, error) {
	if branchNode == nil {
		return beatree.Key{}, fmt.Errorf("nil branch node")
	}

	if len(branchNode.Separators) == 0 {
		return beatree.Key{}, fmt.Errorf("branch node has no separators")
	}

	// Return the first separator's key
	// (Go stores complete keys, unlike Rust which uses prefix compression)
	return branchNode.Separators[0].Key, nil
}

// ValidateBranchNode validates that a branch node is well-formed.
// This is used during reconstruction to detect corruption.
func ValidateBranchNode(branchNode *branch.Node) error {
	if branchNode == nil {
		return fmt.Errorf("nil branch node")
	}

	// Check separator count is within bounds
	if len(branchNode.Separators) > branch.MaxSeparators {
		return fmt.Errorf("too many separators: %d > %d", len(branchNode.Separators), branch.MaxSeparators)
	}

	// Check separators are sorted
	for i := 1; i < len(branchNode.Separators); i++ {
		prev := branchNode.Separators[i-1].Key
		curr := branchNode.Separators[i].Key

		if prev.Compare(curr) >= 0 {
			return fmt.Errorf("separators not sorted at index %d", i)
		}
	}

	// Check child page numbers are valid
	if branchNode.LeftmostChild == allocator.InvalidPageNumber {
		return fmt.Errorf("invalid leftmost child page number")
	}

	for i, sep := range branchNode.Separators {
		if sep.Child == allocator.InvalidPageNumber {
			return fmt.Errorf("invalid child page number at separator %d", i)
		}
	}

	return nil
}
