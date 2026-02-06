package ops

import (
	"bytes"
	"fmt"

	"github.com/jam-duna/jamduna/bmt/beatree"
	"github.com/jam-duna/jamduna/bmt/beatree/allocator"
	"github.com/jam-duna/jamduna/bmt/beatree/branch"
	"github.com/jam-duna/jamduna/bmt/beatree/leaf"
)

// PartialLookup determines which leaf page might contain the key.
// Returns InvalidPageNumber if the key is definitely non-existent.
//
// This uses the branch index to find the correct leaf page without
// loading the leaf itself. The caller must load the leaf to finish the lookup.
func PartialLookup(key beatree.Key, index *beatree.Index) allocator.PageNumber {
	_, branchData, found := index.Lookup(key)
	if !found {
		return allocator.InvalidPageNumber
	}

	// Deserialize the branch node
	branchNode, err := branch.DeserializeBranchNode(branchData)
	if err != nil {
		return allocator.InvalidPageNumber
	}

	// Search the branch to find the child page
	_, childPage, found := SearchBranch(branchNode, key)
	if !found {
		return allocator.InvalidPageNumber
	}

	return childPage
}

// SearchBranch searches a branch node for the child page containing the key.
// Returns the index of the separator and the child page number.
//
// This finds the last child node pointer whose separator is <= the given key.
func SearchBranch(branchNode *branch.Node, key beatree.Key) (int, allocator.PageNumber, bool) {
	found, pos := FindKeyPos(branchNode, key, -1)

	if found {
		// Exact match - return this position
		if pos >= len(branchNode.Separators) {
			return -1, allocator.InvalidPageNumber, false
		}
		return pos, branchNode.Separators[pos].Child, true
	}

	if pos == 0 {
		// Key is less than all separators - return leftmost child
		return -1, branchNode.LeftmostChild, true
	}

	// Key is greater than separator at pos-1, so return that child
	return pos - 1, branchNode.Separators[pos-1].Child, true
}

// FindKeyPos performs binary search for a key within a branch node.
// Returns (true, index) if key is found, or (false, index) where index
// is the position of the first key greater than the target.
//
// The low parameter allows overriding the starting point of the search.
// Use -1 for default (start from 0).
func FindKeyPos(branchNode *branch.Node, key beatree.Key, low int) (bool, int) {
	n := len(branchNode.Separators)
	if n == 0 {
		return false, 0
	}

	if low < 0 {
		low = 0
	}
	high := n

	for low < high {
		mid := low + (high-low)/2
		cmp := bytes.Compare(key[:], branchNode.Separators[mid].Key[:])

		if cmp == 0 {
			return true, mid
		} else if cmp < 0 {
			high = mid
		} else {
			low = mid + 1
		}
	}

	return false, high
}

// FinishLookup finds the value associated with a key in a leaf node.
// Handles both inline and overflow values using ReadOverflow for large values.
func FinishLookup(key beatree.Key, leafNode *leaf.Node, leafStore PageStore) ([]byte, error) {
	entry, found := leafNode.Lookup(key)
	if !found {
		return nil, nil
	}

	// Handle inline values
	if entry.ValueType == leaf.ValueTypeInline {
		return entry.Value, nil
	}

	// Handle overflow values
	if entry.ValueType == leaf.ValueTypeOverflow {
		return ReadOverflow(entry.Value, leafStore)
	}

	return nil, fmt.Errorf("unknown value type: %d", entry.ValueType)
}

// LookupInTreeWithStores performs tree traversal to lookup a key.
// This is designed to be called from the beatree package without import cycles.
func LookupInTreeWithStores(key beatree.Key, root allocator.PageNumber, branchStore, leafStore PageStore) ([]byte, error) {
	if root == allocator.InvalidPageNumber {
		return nil, nil // Empty tree
	}

	// Traverse the tree to find the leaf page
	currentPage := root
	for {
		// Determine if current page is a leaf by trying to read it as a leaf
		isLeaf, err := isLeafPageFromStores(currentPage, leafStore)
		if err != nil {
			return nil, fmt.Errorf("failed to determine page type: %w", err)
		}

		if isLeaf {
			// Found leaf page - perform lookup
			leafNode, err := getLeafNodeFromStore(currentPage, leafStore)
			if err != nil {
				return nil, err
			}

			return FinishLookup(key, leafNode, leafStore)
		} else {
			// This is a branch page - find child to follow
			branchNode, err := getBranchNodeFromStore(currentPage, branchStore)
			if err != nil {
				return nil, fmt.Errorf("failed to read branch node: %w", err)
			}

			// Find the appropriate child page for this key
			childPage := branchNode.FindChild(key)
			if childPage == allocator.InvalidPageNumber {
				return nil, nil // Invalid child - key not found
			}

			currentPage = childPage
		}
	}
}

// isLeafPageFromStores determines if a page contains a leaf node.
func isLeafPageFromStores(page allocator.PageNumber, leafStore PageStore) (bool, error) {
	leafData, err := leafStore.Page(page)
	if err != nil {
		return false, err
	}

	// Check if it can be deserialized as a leaf
	_, err = leaf.DeserializeLeafNode(leafData)
	return err == nil, nil
}

// getLeafNodeFromStore retrieves a leaf node from storage.
func getLeafNodeFromStore(page allocator.PageNumber, leafStore PageStore) (*leaf.Node, error) {
	pageData, err := leafStore.Page(page)
	if err != nil {
		return nil, fmt.Errorf("failed to read page %v: %w", page, err)
	}

	// Deserialize leaf node
	leafNode, err := leaf.DeserializeLeafNode(pageData)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize leaf node: %w", err)
	}

	return leafNode, nil
}

// getBranchNodeFromStore retrieves a branch node from storage.
func getBranchNodeFromStore(page allocator.PageNumber, branchStore PageStore) (*branch.Node, error) {
	pageData, err := branchStore.Page(page)
	if err != nil {
		return nil, fmt.Errorf("failed to read page %v: %w", page, err)
	}

	// Deserialize branch node
	branchNode, err := branch.DeserializeBranchNode(pageData)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize branch node: %w", err)
	}

	return branchNode, nil
}

// LookupBlocking performs a complete key lookup using blocking I/O.
// This is a convenience function that combines partial lookup and finish lookup.
// Handles both inline and overflow values.
//
// Note: Leaf cache integration should be done at the beatree package level,
// not here in ops. Async I/O support is a future enhancement.
func LookupBlocking(
	key beatree.Key,
	index *beatree.Index,
	leafStore PageStore,
) ([]byte, error) {
	// Partial lookup to find leaf page
	leafPn := PartialLookup(key, index)
	if leafPn == allocator.InvalidPageNumber {
		return nil, nil
	}

	// Load leaf node using proper serialization
	leafPageData, err := leafStore.Page(leafPn)
	if err != nil {
		return nil, fmt.Errorf("failed to load leaf page: %w", err)
	}

	// Deserialize the leaf node
	leafNode, err := leaf.DeserializeLeafNode(leafPageData)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize leaf node: %w", err)
	}

	// Finish lookup in the leaf
	return FinishLookup(key, leafNode, leafStore)
}
