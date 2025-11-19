package ops

import (
	"fmt"
	"sync"

	"github.com/colorfulnotion/jam/bmt/beatree"
	"github.com/colorfulnotion/jam/bmt/beatree/allocator"
	"github.com/colorfulnotion/jam/bmt/beatree/branch"
	"github.com/colorfulnotion/jam/bmt/beatree/leaf"
)

// Tree represents a B-epsilon tree with copy-on-write semantics.
// Provides basic tree operations. MVCC coordination available via beatree package.
type Tree struct {
	mu sync.RWMutex

	// Root page number (InvalidPageNumber for empty tree)
	root allocator.PageNumber

	// Page stores (separate for branch and leaf nodes)
	branchStore PageStore
	leafStore   PageStore

	// Staging area for uncommitted changes
	staging map[beatree.Key]*beatree.Change
}

// NewTree creates a new empty Beatree.
func NewTree(branchStore, leafStore PageStore) *Tree {
	return &Tree{
		root:        allocator.InvalidPageNumber,
		branchStore: branchStore,
		leafStore:   leafStore,
		staging:     make(map[beatree.Key]*beatree.Change),
	}
}

// Lookup retrieves the value for a given key.
// Returns nil if the key doesn't exist.
func (t *Tree) Lookup(key beatree.Key) ([]byte, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// Check staging first (uncommitted changes)
	if change, exists := t.staging[key]; exists {
		if change.IsDelete() {
			return nil, nil
		}
		return change.Value, nil
	}

	// If tree is empty, key not found
	if t.root == allocator.InvalidPageNumber {
		return nil, nil
	}

	// Traverse tree to find key
	return t.lookupInTree(key)
}

// Insert inserts or updates a key-value pair.
// The change is staged until Commit() is called.
func (t *Tree) Insert(key beatree.Key, value []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Stage the insert
	t.staging[key] = beatree.NewInsertChange(value)
	return nil
}

// Delete deletes a key.
// The change is staged until Commit() is called.
func (t *Tree) Delete(key beatree.Key) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Stage the delete
	t.staging[key] = beatree.NewDeleteChange()
	return nil
}

// Commit applies all staged changes to the tree.
// Returns the new root page number.
func (t *Tree) Commit() (allocator.PageNumber, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if len(t.staging) == 0 {
		// No changes to commit
		return t.root, nil
	}

	// Apply staged changes to tree
	newRoot, err := t.applyChanges()
	if err != nil {
		return allocator.InvalidPageNumber, err
	}

	// Clear staging
	t.staging = make(map[beatree.Key]*beatree.Change)

	// Update root
	t.root = newRoot

	return newRoot, nil
}

// Root returns the current root page number.
func (t *Tree) Root() allocator.PageNumber {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.root
}

// lookupInTree performs the actual tree traversal to find a key.
// Caller must hold read lock.
func (t *Tree) lookupInTree(key beatree.Key) ([]byte, error) {
	if t.root == allocator.InvalidPageNumber {
		return nil, nil // Empty tree
	}

	// Traverse the tree to find the leaf page
	currentPage := t.root
	for {
		// Determine if current page is a leaf
		isLeaf, err := t.isLeafPage(currentPage)
		if err != nil {
			return nil, fmt.Errorf("failed to determine page type: %w", err)
		}

		if isLeaf {
			// Found leaf page - perform lookup
			leafNode, err := t.getLeafNode(currentPage)
			if err != nil {
				return nil, err
			}

			entry, found := leafNode.Lookup(key)
			if !found {
				return nil, nil
			}

			// Handle overflow values
			if entry.ValueType == leaf.ValueTypeOverflow {
				return ReadOverflow(entry.Value, t.leafStore)
			}

			return entry.Value, nil
		} else {
			// This is a branch page - find child to follow
			branchNode, err := t.getBranchNode(currentPage)
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

// getLeafNode retrieves a leaf node from storage.
func (t *Tree) getLeafNode(page allocator.PageNumber) (*leaf.Node, error) {
	// Try TestStore first for backward compatibility
	if store, ok := t.leafStore.(*TestStore); ok {
		return store.GetNode(page)
	}

	// Use proper serialization for production stores
	pageData, err := t.leafStore.Page(page)
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

// setLeafNode stores a leaf node to storage.
func (t *Tree) setLeafNode(page allocator.PageNumber, node *leaf.Node) error {
	// Try TestStore first for backward compatibility
	if store, ok := t.leafStore.(*TestStore); ok {
		store.SetNode(page, node)
		return nil
	}

	// Use proper serialization for production stores
	serializedData, err := node.Serialize()
	if err != nil {
		return fmt.Errorf("failed to serialize leaf node: %w", err)
	}

	// Write serialized data directly to storage
	t.leafStore.WritePage(page, serializedData)

	return nil
}

// getBranchNode retrieves a branch node from storage.
func (t *Tree) getBranchNode(page allocator.PageNumber) (*branch.Node, error) {
	// Use serialization-based approach for all PageStore implementations
	pageData, err := t.branchStore.Page(page)
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

// setBranchNode stores a branch node to storage.
func (t *Tree) setBranchNode(page allocator.PageNumber, node *branch.Node) error {
	// Use serialization-based approach for all PageStore implementations
	serializedData, err := node.Serialize()
	if err != nil {
		return fmt.Errorf("failed to serialize branch node: %w", err)
	}

	// Write serialized data to storage
	t.branchStore.WritePage(page, serializedData)

	return nil
}

// isLeafPage determines if a page contains a leaf node by examining the page structure.
func (t *Tree) isLeafPage(page allocator.PageNumber) (bool, error) {
	// Try to read as leaf first - simpler heuristic
	leafData, err := t.leafStore.Page(page)
	if err != nil {
		return false, err
	}

	// Check if it can be deserialized as a leaf
	_, err = leaf.DeserializeLeafNode(leafData)
	return err == nil, nil
}

// applyChanges applies all staged changes to create a new tree version.
// This implements copy-on-write semantics.
// Caller must hold write lock.
func (t *Tree) applyChanges() (allocator.PageNumber, error) {
	// For an empty tree with inserts, create a new leaf
	if t.root == allocator.InvalidPageNumber {
		return t.createLeafFromStaging()
	}

	// For existing tree, apply changes with CoW
	return t.updateTreeWithCoW()
}

// createLeafFromStaging creates a new leaf node from staged changes.
func (t *Tree) createLeafFromStaging() (allocator.PageNumber, error) {
	leafNode := leaf.NewNode()

	for key, change := range t.staging {
		if change.IsDelete() {
			continue // Skip deletes for new tree
		}

		if change.IsOverflow() {
			// Write overflow value
			cell, allocatedPages, err := WriteOverflow(change.Value, change.Hash, t.leafStore)
			if err != nil {
				return allocator.InvalidPageNumber, fmt.Errorf("failed to write overflow: %w", err)
			}

			// Decode cell to get first page number and size
			valueSize, _, pages, err := beatree.DecodeCell(cell)
			if err != nil {
				// Free allocated pages on error
				for _, page := range allocatedPages {
					t.leafStore.FreePage(page)
				}
				return allocator.InvalidPageNumber, fmt.Errorf("failed to decode overflow cell: %w", err)
			}

			if len(pages) == 0 {
				// Free allocated pages on error
				for _, page := range allocatedPages {
					t.leafStore.FreePage(page)
				}
				return allocator.InvalidPageNumber, fmt.Errorf("overflow cell has no pages")
			}

			// Use first 32 bytes of value as prefix
			valuePrefix := make([]byte, min(32, len(change.Value)))
			copy(valuePrefix, change.Value[:len(valuePrefix)])

			// Create overflow entry
			entry := leaf.NewOverflowEntry(key, pages[0], valueSize, valuePrefix)

			if err := leafNode.Insert(entry); err != nil {
				// Free allocated pages on error
				for _, page := range allocatedPages {
					t.leafStore.FreePage(page)
				}
				return allocator.InvalidPageNumber, fmt.Errorf("failed to insert overflow key: %w", err)
			}
		} else {
			// Handle inline entry
			entry, err := leaf.NewInlineEntry(key, change.Value)
			if err != nil {
				return allocator.InvalidPageNumber, fmt.Errorf("failed to create entry: %w", err)
			}

			if err := leafNode.Insert(entry); err != nil {
				return allocator.InvalidPageNumber, fmt.Errorf("failed to insert key: %w", err)
			}
		}
	}

	// Allocate page and store node
	pageNum := t.leafStore.Alloc()
	if err := t.setLeafNode(pageNum, leafNode); err != nil {
		t.leafStore.FreePage(pageNum)
		return allocator.InvalidPageNumber, fmt.Errorf("failed to store leaf node: %w", err)
	}

	return pageNum, nil
}

// updateTreeWithCoW applies changes to existing tree with copy-on-write.
// Currently handles single-leaf trees.
func (t *Tree) updateTreeWithCoW() (allocator.PageNumber, error) {
	// Simple case: single-leaf tree updates
	// Future: Full CoW with branch node updates and multi-level trees

	// Read current root (assuming it's a leaf for now)
	oldLeafNode, err := t.getLeafNode(t.root)
	if err != nil {
		return allocator.InvalidPageNumber, fmt.Errorf("failed to read root leaf: %w", err)
	}

	// Create a copy of the leaf node for CoW
	leafNode := leaf.NewNode()
	for _, entry := range oldLeafNode.Entries {
		leafNode.Insert(entry)
	}

	// Apply changes to the copied leaf
	for key, change := range t.staging {
		if change.IsDelete() {
			leafNode.Delete(key)
		} else if change.IsInsert() {
			entry, err := leaf.NewInlineEntry(key, change.Value)
			if err != nil {
				return allocator.InvalidPageNumber, fmt.Errorf("failed to create entry: %w", err)
			}

			if err := leafNode.Insert(entry); err != nil {
				return allocator.InvalidPageNumber, fmt.Errorf("failed to insert key: %w", err)
			}
		} else if change.IsOverflow() {
			// Write overflow value
			cell, allocatedPages, err := WriteOverflow(change.Value, change.Hash, t.leafStore)
			if err != nil {
				return allocator.InvalidPageNumber, fmt.Errorf("failed to write overflow: %w", err)
			}

			// Decode cell to get first page number and size
			valueSize, _, pages, err := beatree.DecodeCell(cell)
			if err != nil {
				// Free allocated pages on error
				for _, page := range allocatedPages {
					t.leafStore.FreePage(page)
				}
				return allocator.InvalidPageNumber, fmt.Errorf("failed to decode overflow cell: %w", err)
			}

			if len(pages) == 0 {
				// Free allocated pages on error
				for _, page := range allocatedPages {
					t.leafStore.FreePage(page)
				}
				return allocator.InvalidPageNumber, fmt.Errorf("overflow cell has no pages")
			}

			// Use first 32 bytes of value as prefix
			valuePrefix := make([]byte, min(32, len(change.Value)))
			copy(valuePrefix, change.Value[:len(valuePrefix)])

			// Create overflow entry
			entry := leaf.NewOverflowEntry(key, pages[0], valueSize, valuePrefix)

			if err := leafNode.Insert(entry); err != nil {
				// Free allocated pages on error
				for _, page := range allocatedPages {
					t.leafStore.FreePage(page)
				}
				return allocator.InvalidPageNumber, fmt.Errorf("failed to insert overflow key: %w", err)
			}
		} else {
			return allocator.InvalidPageNumber, fmt.Errorf("unknown change type")
		}
	}

	// Allocate new page for CoW and store the new node
	newPageNum := t.leafStore.Alloc()
	if err := t.setLeafNode(newPageNum, leafNode); err != nil {
		t.leafStore.FreePage(newPageNum)
		return allocator.InvalidPageNumber, fmt.Errorf("failed to store updated leaf node: %w", err)
	}

	// IMPORTANT: Do NOT free the old root page!
	// Copy-on-write semantics require that old versions remain accessible.
	// Readers holding the previous root must be able to read it.
	// Page reclamation should be handled by higher layers (e.g., MVCC garbage collection).

	return newPageNum, nil
}
