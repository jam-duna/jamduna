package ops

import (
	"crypto/sha256"
	"fmt"
	"sort"

	"github.com/jam-duna/jamduna/bmt/beatree"
	"github.com/jam-duna/jamduna/bmt/beatree/allocator"
	"github.com/jam-duna/jamduna/bmt/beatree/branch"
	"github.com/jam-duna/jamduna/bmt/beatree/leaf"
)

// UpdateResult contains the results of a tree update operation.
type UpdateResult struct {
	// NewRoot is the page number of the new root
	NewRoot allocator.PageNumber
	// FreedPages is a list of pages that can be freed
	FreedPages []allocator.PageNumber
	// AllocatedPages is a list of newly allocated pages
	AllocatedPages []allocator.PageNumber
}

// Update applies a changeset to the tree using copy-on-write semantics.
func Update(
	currentRoot allocator.PageNumber,
	changeset map[beatree.Key]*beatree.Change,
	index *beatree.Index,
	leafStore PageStore,
) (*UpdateResult, error) {
	if len(changeset) == 0 {
		return &UpdateResult{
			NewRoot:        currentRoot,
			FreedPages:     nil,
			AllocatedPages: nil,
		}, nil
	}

	if currentRoot == allocator.InvalidPageNumber {
		return createLeafFromChangeset(changeset, leafStore)
	}

	return updateTreeWithCoW(currentRoot, changeset, leafStore)
}

func createLeafFromChangeset(
	changeset map[beatree.Key]*beatree.Change,
	leafStore PageStore,
) (*UpdateResult, error) {
	var entries []sortableEntry
	for key, change := range changeset {
		if change.IsDelete() {
			continue
		}
		entries = append(entries, sortableEntry{key, change})
	}

	sortEntries(entries)

	var leafPages []allocator.PageNumber
	currentLeaf := leaf.NewNode()

	for _, entry := range entries {
		if currentLeaf.NumEntries() > 0 && willOverflow(currentLeaf, entry.key, entry.change) {
			pageNum, err := storeLeafNode(currentLeaf, leafStore)
			if err != nil {
				return nil, fmt.Errorf("failed to store leaf node: %w", err)
			}
			leafPages = append(leafPages, pageNum)
			currentLeaf = leaf.NewNode()
		}

		if err := insertIntoLeaf(currentLeaf, entry.key, entry.change, leafStore); err != nil {
			return nil, fmt.Errorf("failed to insert key: %w", err)
		}
	}

	if currentLeaf.NumEntries() > 0 {
		pageNum, err := storeLeafNode(currentLeaf, leafStore)
		if err != nil {
			return nil, fmt.Errorf("failed to store final leaf node: %w", err)
		}
		leafPages = append(leafPages, pageNum)
	}

	if len(leafPages) == 1 {
		return &UpdateResult{
			NewRoot:        leafPages[0],
			FreedPages:     nil,
			AllocatedPages: leafPages,
		}, nil
	}

	rootBranch, err := createBranchFromLeaves(leafPages, entries, leafStore)
	if err != nil {
		return nil, fmt.Errorf("failed to create branch node: %w", err)
	}

	allAllocatedPages := append(leafPages, rootBranch)

	return &UpdateResult{
		NewRoot:        rootBranch,
		FreedPages:     nil,
		AllocatedPages: allAllocatedPages,
	}, nil
}

func updateTreeWithCoW(
	currentRoot allocator.PageNumber,
	changeset map[beatree.Key]*beatree.Change,
	leafStore PageStore,
) (*UpdateResult, error) {
	newRoot, splitResult, allocatedPages, err := updateNode(currentRoot, changeset, leafStore)
	if err != nil {
		return nil, fmt.Errorf("failed to update tree: %w", err)
	}

	if splitResult != nil {
		newRootBranch := branch.NewNode(newRoot)
		separator := branch.Separator{
			Key:   splitResult.SeparatorKey,
			Child: splitResult.RightPage,
		}
		if err := newRootBranch.Insert(separator); err != nil {
			return nil, fmt.Errorf("failed to create new root: %w", err)
		}

		rootPageNum := leafStore.Alloc()
		branchData, err := newRootBranch.Serialize()
		if err != nil {
			return nil, fmt.Errorf("failed to serialize new root: %w", err)
		}
		leafStore.WritePage(rootPageNum, branchData)

		allocatedPages = append(allocatedPages, rootPageNum)
		newRoot = rootPageNum
	}

	return &UpdateResult{
		NewRoot:        newRoot,
		FreedPages:     nil,
		AllocatedPages: allocatedPages,
	}, nil
}

type splitInfo struct {
	RightPage    allocator.PageNumber
	SeparatorKey beatree.Key
}

func updateNode(
	pageNum allocator.PageNumber,
	changeset map[beatree.Key]*beatree.Change,
	leafStore PageStore,
) (allocator.PageNumber, *splitInfo, []allocator.PageNumber, error) {
	pageData, err := leafStore.Page(pageNum)
	if err != nil {
		return allocator.InvalidPageNumber, nil, nil, fmt.Errorf("failed to load page: %w", err)
	}

	if len(pageData) < 5 {
		return allocator.InvalidPageNumber, nil, nil, fmt.Errorf("page too small")
	}
	pageType := pageData[4]

	if pageType == 0x01 {
		return updateLeafNode(pageNum, pageData, changeset, leafStore)
	} else if pageType == 0x02 {
		return updateBranchNode(pageNum, pageData, changeset, leafStore)
	}

	return updateLeafNode(pageNum, pageData, changeset, leafStore)
}

func updateLeafNode(
	pageNum allocator.PageNumber,
	pageData []byte,
	changeset map[beatree.Key]*beatree.Change,
	leafStore PageStore,
) (allocator.PageNumber, *splitInfo, []allocator.PageNumber, error) {
	oldLeaf, err := leaf.DeserializeLeafNode(pageData)
	if err != nil {
		return allocator.InvalidPageNumber, nil, nil, fmt.Errorf("failed to deserialize leaf: %w", err)
	}

	// OPTIMIZATION: Shallow clone instead of full copy
	// Clone copies the Entries slice but shares value buffers (immutable)
	// This is O(M) where M = number of entries, not O(M × avg_value_size)
	newLeaf := oldLeaf.Clone()

	// Apply changes using in-place updates
	for key, change := range changeset {
		if change.IsDelete() {
			// Use DeleteEntry for O(log M) deletion
			_, err := newLeaf.DeleteEntry(key)
			if err != nil {
				return allocator.InvalidPageNumber, nil, nil, fmt.Errorf("failed to delete key: %w", err)
			}
		} else if change.IsInsert() {
			// For inline values, use UpdateEntry for O(log M) update/insert
			if len(change.Value) <= leaf.MaxLeafValueSize {
				_, needsSplit, err := newLeaf.UpdateEntry(key, change.Value)
				if err != nil {
					return allocator.InvalidPageNumber, nil, nil, fmt.Errorf("failed to update entry: %w", err)
				}
				// Note: needsSplit is checked per-key, but we handle the final split below
				_ = needsSplit
			} else {
				// For overflow values, fall back to insertIntoLeaf (handles overflow page allocation)
				if err := insertIntoLeaf(newLeaf, key, change, leafStore); err != nil {
					return allocator.InvalidPageNumber, nil, nil, fmt.Errorf("failed to insert overflow key: %w", err)
				}
			}
		} else if change.IsOverflow() {
			// Overflow values always use insertIntoLeaf
			if err := insertIntoLeaf(newLeaf, key, change, leafStore); err != nil {
				return allocator.InvalidPageNumber, nil, nil, fmt.Errorf("failed to insert overflow key: %w", err)
			}
		} else {
			return allocator.InvalidPageNumber, nil, nil, fmt.Errorf("unknown change type")
		}
	}

	// Check if split is needed after all changes applied
	if newLeaf.EstimateSize() >= leaf.LeafNodeSize {
		rightLeaf, separator, err := newLeaf.Split()
		if err != nil {
			return allocator.InvalidPageNumber, nil, nil, fmt.Errorf("failed to split leaf: %w", err)
		}

		leftPageNum, err := storeLeafNode(newLeaf, leafStore)
		if err != nil {
			return allocator.InvalidPageNumber, nil, nil, fmt.Errorf("failed to store left leaf: %w", err)
		}

		rightPageNum, err := storeLeafNode(rightLeaf, leafStore)
		if err != nil {
			return allocator.InvalidPageNumber, nil, nil, fmt.Errorf("failed to store right leaf: %w", err)
		}

		return leftPageNum, &splitInfo{
			RightPage:    rightPageNum,
			SeparatorKey: separator,
		}, []allocator.PageNumber{leftPageNum, rightPageNum}, nil
	}

	// Single allocation for modified leaf (CoW semantics)
	newPageNum := leafStore.Alloc()
	leafData, err := newLeaf.Serialize()
	if err != nil {
		return allocator.InvalidPageNumber, nil, nil, fmt.Errorf("failed to serialize updated leaf node: %w", err)
	}
	leafStore.WritePage(newPageNum, leafData)

	return newPageNum, nil, []allocator.PageNumber{newPageNum}, nil
}

func updateBranchNode(
	pageNum allocator.PageNumber,
	pageData []byte,
	changeset map[beatree.Key]*beatree.Change,
	leafStore PageStore,
) (allocator.PageNumber, *splitInfo, []allocator.PageNumber, error) {
	oldBranch, err := branch.DeserializeBranchNode(pageData)
	if err != nil {
		return allocator.InvalidPageNumber, nil, nil, fmt.Errorf("failed to deserialize branch: %w", err)
	}

	// Group changes by child
	childChangesets := make(map[allocator.PageNumber]map[beatree.Key]*beatree.Change)
	for key, change := range changeset {
		childPage := oldBranch.FindChild(key)
		if childChangesets[childPage] == nil {
			childChangesets[childPage] = make(map[beatree.Key]*beatree.Change)
		}
		childChangesets[childPage][key] = change
	}

	// OPTIMIZATION: Shallow clone instead of full copy
	// Clone copies the Separators slice but all values are value types
	// This is O(S) where S = number of separators, not O(S × N) for full subtree copy
	newBranch := oldBranch.Clone()

	var allAllocatedPages []allocator.PageNumber

	// Update ONLY the affected children using point updates
	for childPage, childChanges := range childChangesets {
		newChildPage, childSplit, childAllocated, err := updateNode(childPage, childChanges, leafStore)
		if err != nil {
			return allocator.InvalidPageNumber, nil, nil, fmt.Errorf("failed to update child: %w", err)
		}

		allAllocatedPages = append(allAllocatedPages, childAllocated...)

		// Point update - modify only this child pointer using UpdateChild
		found, err := newBranch.UpdateChild(childPage, newChildPage)
		if err != nil {
			return allocator.InvalidPageNumber, nil, nil, fmt.Errorf("failed to update child pointer: %w", err)
		}
		if !found {
			return allocator.InvalidPageNumber, nil, nil, fmt.Errorf("child page %v not found in branch", childPage)
		}

		if childSplit != nil {
			// Insert separator without copying existing ones
			newSep := branch.Separator{
				Key:   childSplit.SeparatorKey,
				Child: childSplit.RightPage,
			}
			if err := newBranch.InsertSeparator(newSep); err != nil {
				return allocator.InvalidPageNumber, nil, nil, fmt.Errorf("failed to insert split separator: %w", err)
			}
		}
	}

	if newBranch.EstimateSize() >= branch.BranchNodeSize {
		rightBranch, separator, err := splitBranch(newBranch)
		if err != nil {
			return allocator.InvalidPageNumber, nil, nil, fmt.Errorf("failed to split branch: %w", err)
		}

		leftPageNum, err := storeBranchNode(newBranch, leafStore)
		if err != nil {
			return allocator.InvalidPageNumber, nil, nil, fmt.Errorf("failed to store left branch: %w", err)
		}

		rightPageNum, err := storeBranchNode(rightBranch, leafStore)
		if err != nil {
			return allocator.InvalidPageNumber, nil, nil, fmt.Errorf("failed to store right branch: %w", err)
		}

		allAllocatedPages = append(allAllocatedPages, leftPageNum, rightPageNum)

		return leftPageNum, &splitInfo{
			RightPage:    rightPageNum,
			SeparatorKey: separator,
		}, allAllocatedPages, nil
	}

	newPageNum := leafStore.Alloc()
	branchData, err := newBranch.Serialize()
	if err != nil {
		return allocator.InvalidPageNumber, nil, nil, fmt.Errorf("failed to serialize branch: %w", err)
	}
	leafStore.WritePage(newPageNum, branchData)

	allAllocatedPages = append(allAllocatedPages, newPageNum)

	return newPageNum, nil, allAllocatedPages, nil
}

func splitBranch(branchNode *branch.Node) (*branch.Node, beatree.Key, error) {
	if len(branchNode.Separators) < 2 {
		return nil, beatree.Key{}, fmt.Errorf("cannot split branch with less than 2 separators")
	}

	splitIdx := len(branchNode.Separators) / 2
	separator := branchNode.Separators[splitIdx]

	rightBranch := branch.NewNode(separator.Child)
	for i := splitIdx + 1; i < len(branchNode.Separators); i++ {
		if err := rightBranch.Insert(branchNode.Separators[i]); err != nil {
			return nil, beatree.Key{}, err
		}
	}

	branchNode.Separators = branchNode.Separators[:splitIdx]

	return rightBranch, separator.Key, nil
}

func storeBranchNode(branchNode *branch.Node, leafStore PageStore) (allocator.PageNumber, error) {
	pageNum := leafStore.Alloc()

	branchData, err := branchNode.Serialize()
	if err != nil {
		return allocator.InvalidPageNumber, fmt.Errorf("failed to serialize branch: %w", err)
	}

	if len(branchData) > branch.BranchNodeSize {
		return allocator.InvalidPageNumber, fmt.Errorf("BUG: branch serialized to %d bytes (max %d), has %d separators", len(branchData), branch.BranchNodeSize, branchNode.NumSeparators())
	}

	leafStore.WritePage(pageNum, branchData)
	return pageNum, nil
}

func insertIntoLeaf(
	leafNode *leaf.Node,
	key beatree.Key,
	change *beatree.Change,
	leafStore PageStore,
) error {
	if change.IsOverflow() || len(change.Value) > leaf.MaxLeafValueSize {
		valueHash := sha256.Sum256(change.Value)
		cell, overflowPages, err := WriteOverflow(change.Value, valueHash, leafStore)
		if err != nil {
			return fmt.Errorf("failed to write overflow: %w", err)
		}

		valueSize, _, pages, err := beatree.DecodeCell(cell)
		if err != nil {
			return fmt.Errorf("failed to decode overflow cell: %w", err)
		}

		if len(pages) == 0 {
			return fmt.Errorf("overflow cell has no pages")
		}

		valuePrefix := make([]byte, min(32, len(change.Value)))
		copy(valuePrefix, change.Value[:len(valuePrefix)])

		entry := leaf.NewOverflowEntryWithPages(key, pages, valueSize, valuePrefix)
		_ = overflowPages

		return leafNode.Insert(entry)
	}

	entry, err := leaf.NewInlineEntry(key, change.Value)
	if err != nil {
		return fmt.Errorf("failed to create inline entry: %w", err)
	}

	return leafNode.Insert(entry)
}

type sortableEntry struct {
	key    beatree.Key
	change *beatree.Change
}

func sortEntries(entries []sortableEntry) {
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].key.Compare(entries[j].key) < 0
	})
}

func estimateEntrySize(key beatree.Key, change *beatree.Change) int {
	entrySize := 32 + 1

	if change.IsOverflow() || len(change.Value) > leaf.MaxLeafValueSize {
		entrySize += 8 + 8 + 2 + 32
	} else {
		entrySize += 4 + len(change.Value)
	}

	return entrySize
}

func willOverflow(leafNode *leaf.Node, key beatree.Key, change *beatree.Change) bool {
	return leafNode.EstimateSize()+estimateEntrySize(key, change) >= leaf.LeafNodeSize
}

func storeLeafNode(leafNode *leaf.Node, leafStore PageStore) (allocator.PageNumber, error) {
	pageNum := leafStore.Alloc()

	leafData, err := leafNode.Serialize()
	if err != nil {
		return allocator.InvalidPageNumber, fmt.Errorf("failed to serialize leaf node: %w", err)
	}

	if len(leafData) > leaf.LeafNodeSize {
		return allocator.InvalidPageNumber, fmt.Errorf("BUG: leaf serialized to %d bytes (max %d), has %d entries", len(leafData), leaf.LeafNodeSize, leafNode.NumEntries())
	}

	leafStore.WritePage(pageNum, leafData)
	return pageNum, nil
}

// rebuildTreeFromIterator rebuilds the entire tree by iterating all existing entries
// and merging with the changeset. This is used when updating a tree with a branch root.
func rebuildTreeFromIterator(
	currentRoot allocator.PageNumber,
	changeset map[beatree.Key]*beatree.Change,
	leafStore PageStore,
) (*UpdateResult, error) {
	// Collect all existing entries from the tree
	existingEntries, err := collectAllEntries(currentRoot, leafStore)
	if err != nil {
		return nil, fmt.Errorf("failed to collect existing entries: %w", err)
	}

	// Build a sorted list of all final entries
	var finalEntries []sortableEntry

	// Create a set of keys in the changeset for quick lookup
	changesetKeys := make(map[beatree.Key]bool)
	for key := range changeset {
		changesetKeys[key] = true
	}

	// Add existing entries that aren't being modified or deleted
	for _, entry := range existingEntries {
		change, hasChange := changeset[entry.Key]

		if hasChange && change.IsDelete() {
			// Skip deleted entries
			continue
		}

		if hasChange {
			// Use new value from changeset
			finalEntries = append(finalEntries, sortableEntry{entry.Key, change})
		} else {
			// Keep existing entry - convert to change
			var existingChange *beatree.Change
			if entry.ValueType == leaf.ValueTypeInline {
				existingChange = beatree.NewInsertChange(entry.Value)
			} else {
				// For overflow entries, we can't easily convert back to a change
				// because we'd need to read all the overflow pages
				// Instead, mark this for direct reinsertion
				existingChange = &beatree.Change{
					Type:  beatree.ValueInsertOverflow,
					Value: entry.Value, // Just the prefix - we'll handle this specially
				}
			}
			finalEntries = append(finalEntries, sortableEntry{entry.Key, existingChange})
		}
	}

	// Add new entries from changeset that don't exist in the tree
	for key, change := range changeset {
		if change.IsDelete() {
			continue
		}
		// Check if this is a new key
		found := false
		for _, entry := range existingEntries {
			if entry.Key == key {
				found = true
				break
			}
		}
		if !found {
			finalEntries = append(finalEntries, sortableEntry{key, change})
		}
	}

	// Sort entries by key
	sortEntries(finalEntries)

	// Create leaves, splitting as needed
	var leafPages []allocator.PageNumber
	currentLeaf := leaf.NewNode()

	// Keep a map of existing entries for overflow handling
	existingEntriesMap := make(map[beatree.Key]leaf.Entry)
	for _, entry := range existingEntries {
		existingEntriesMap[entry.Key] = entry
	}

	for _, entry := range finalEntries {
		// Check if adding this entry would overflow the current leaf
		if currentLeaf.NumEntries() > 0 && willOverflow(currentLeaf, entry.key, entry.change) {
			// Save current leaf and start a new one
			pageNum, err := storeLeafNode(currentLeaf, leafStore)
			if err != nil {
				return nil, fmt.Errorf("failed to store leaf node: %w", err)
			}
			leafPages = append(leafPages, pageNum)
			currentLeaf = leaf.NewNode()
		}

		// Check if this is an overflow entry from existing tree
		if oldEntry, exists := existingEntriesMap[entry.key]; exists && oldEntry.ValueType == leaf.ValueTypeOverflow {
			// Reuse the existing overflow entry directly
			if err := currentLeaf.Insert(oldEntry); err != nil {
				return nil, fmt.Errorf("failed to insert existing overflow entry: %w", err)
			}
		} else {
			// New entry or inline entry - insert normally
			if err := insertIntoLeaf(currentLeaf, entry.key, entry.change, leafStore); err != nil {
				return nil, fmt.Errorf("failed to insert key: %w", err)
			}
		}
	}

	// Store the last leaf
	if currentLeaf.NumEntries() > 0 {
		pageNum, err := storeLeafNode(currentLeaf, leafStore)
		if err != nil {
			return nil, fmt.Errorf("failed to store final leaf node: %w", err)
		}
		leafPages = append(leafPages, pageNum)
	}

	// If we only created one leaf, return it as the root
	if len(leafPages) == 1 {
		return &UpdateResult{
			NewRoot:        leafPages[0],
			FreedPages:     nil,
			AllocatedPages: leafPages,
		}, nil
	}

	// Multiple leaves - create a branch node to hold them
	rootBranch, err := createBranchFromLeaves(leafPages, finalEntries, leafStore)
	if err != nil {
		return nil, fmt.Errorf("failed to create branch node: %w", err)
	}

	// All allocated pages include leaves + branch root
	allAllocatedPages := append(leafPages, rootBranch)

	return &UpdateResult{
		NewRoot:        rootBranch,
		FreedPages:     nil,
		AllocatedPages: allAllocatedPages,
	}, nil
}

// collectAllEntries recursively collects all entries from the tree
func collectAllEntries(pageNum allocator.PageNumber, leafStore PageStore) ([]leaf.Entry, error) {
	pageData, err := leafStore.Page(pageNum)
	if err != nil {
		return nil, fmt.Errorf("failed to read page %d: %w", pageNum, err)
	}

	// Check if this is a leaf or branch
	if len(pageData) < 5 {
		return nil, fmt.Errorf("page %d too small", pageNum)
	}

	pageType := pageData[4]

	if pageType == 0x01 { // Leaf
		// Deserialize leaf and return its entries
		leafNode, err := leaf.DeserializeLeafNode(pageData)
		if err != nil {
			return nil, fmt.Errorf("failed to deserialize leaf page %d: %w", pageNum, err)
		}
		return leafNode.Entries, nil
	} else if pageType == 0x02 { // Branch
		// Deserialize branch and recursively collect from all children
		branchNode, err := branch.DeserializeBranchNode(pageData)
		if err != nil {
			return nil, fmt.Errorf("failed to deserialize branch page %d: %w", pageNum, err)
		}

		var allEntries []leaf.Entry

		// Collect from leftmost child
		leftEntries, err := collectAllEntries(branchNode.LeftmostChild, leafStore)
		if err != nil {
			return nil, err
		}
		allEntries = append(allEntries, leftEntries...)

		// Collect from each separator's child
		for _, sep := range branchNode.Separators {
			childEntries, err := collectAllEntries(sep.Child, leafStore)
			if err != nil {
				return nil, err
			}
			allEntries = append(allEntries, childEntries...)
		}

		return allEntries, nil
	}

	return nil, fmt.Errorf("unknown page type 0x%02x for page %d", pageType, pageNum)
}

func createBranchFromLeaves(
	leafPages []allocator.PageNumber,
	entries []sortableEntry,
	leafStore PageStore,
) (allocator.PageNumber, error) {
	if len(leafPages) == 0 {
		return allocator.InvalidPageNumber, fmt.Errorf("no leaf pages to create branch from")
	}

	if len(leafPages) == 1 {
		return leafPages[0], nil
	}

	branchNode := branch.NewNode(leafPages[0])

	for i := 1; i < len(leafPages); i++ {
		leafData, err := leafStore.Page(leafPages[i])
		if err != nil {
			return allocator.InvalidPageNumber, fmt.Errorf("failed to read leaf page %d: %w", leafPages[i], err)
		}

		leafNode, err := leaf.DeserializeLeafNode(leafData)
		if err != nil {
			return allocator.InvalidPageNumber, fmt.Errorf("failed to deserialize leaf page %d: %w", leafPages[i], err)
		}

		if len(leafNode.Entries) == 0 {
			return allocator.InvalidPageNumber, fmt.Errorf("leaf page %d is empty", leafPages[i])
		}

		separator := branch.Separator{
			Key:   leafNode.Entries[0].Key,
			Child: leafPages[i],
		}

		if err := branchNode.Insert(separator); err != nil {
			return allocator.InvalidPageNumber, fmt.Errorf("failed to insert separator: %w", err)
		}
	}

	branchPageNum := leafStore.Alloc()
	branchData, err := branchNode.Serialize()
	if err != nil {
		return allocator.InvalidPageNumber, fmt.Errorf("failed to serialize branch node: %w", err)
	}

	leafStore.WritePage(branchPageNum, branchData)
	return branchPageNum, nil
}

func MergeChangesets(changesets ...map[beatree.Key]*beatree.Change) map[beatree.Key]*beatree.Change {
	result := make(map[beatree.Key]*beatree.Change)

	for _, changeset := range changesets {
		for key, change := range changeset {
			result[key] = change
		}
	}

	return result
}

func SplitLeaf(leafNode *leaf.Node) (*leaf.Node, *leaf.Node, beatree.Key, error) {
	entries := leafNode.Entries
	if len(entries) < 2 {
		return nil, nil, beatree.Key{}, fmt.Errorf("cannot split leaf with < 2 entries")
	}

	// Calculate split point targeting 75% fullness in left node
	// This leaves the right node at ~25% for insert-heavy workloads
	targetSize := (leaf.LeafNodeSize * 3) / 4 // 75% of max size

	leftNode := leaf.NewNode()
	rightNode := leaf.NewNode()
	splitIndex := 0

	// Add entries to left node until we reach ~75% fullness
	for i := 0; i < len(entries); i++ {
		if err := leftNode.Insert(entries[i]); err != nil {
			return nil, nil, beatree.Key{}, err
		}

		// Check if we've reached target size
		if leftNode.EstimateSize() >= targetSize && i < len(entries)-1 {
			splitIndex = i + 1
			break
		}
	}

	// If we didn't find a good split point (very uniform entries),
	// fall back to midpoint split
	if splitIndex == 0 {
		splitIndex = len(entries) / 2
		leftNode = leaf.NewNode()
		for i := 0; i < splitIndex; i++ {
			if err := leftNode.Insert(entries[i]); err != nil {
				return nil, nil, beatree.Key{}, err
			}
		}
	}

	// Add remaining entries to right node
	for i := splitIndex; i < len(entries); i++ {
		if err := rightNode.Insert(entries[i]); err != nil {
			return nil, nil, beatree.Key{}, err
		}
	}

	// Separator is the first key of the right node
	separator := entries[splitIndex].Key

	return leftNode, rightNode, separator, nil
}

// MergeLeaves merges two adjacent leaf nodes.
// Used when a node becomes too sparse after deletions.
//
// Note: Automatic merge triggering is not yet implemented in the update path.
// This function can be called manually when needed.
func MergeLeaves(left, right *leaf.Node) (*leaf.Node, error) {
	merged := leaf.NewNode()

	// Copy all entries from left
	for _, entry := range left.Entries {
		if err := merged.Insert(entry); err != nil {
			return nil, err
		}
	}

	// Copy all entries from right
	for _, entry := range right.Entries {
		if err := merged.Insert(entry); err != nil {
			return nil, err
		}
	}

	return merged, nil
}

// Constants for splitting/merging thresholds
// These match the Rust implementation
const (
	// LEAF_MERGE_THRESHOLD - merge when node is less than 50% full
	LEAF_MERGE_THRESHOLD = leaf.LeafNodeSize / 2

	// LEAF_BULK_SPLIT_THRESHOLD - bulk split at 180% fullness
	LEAF_BULK_SPLIT_THRESHOLD = (leaf.LeafNodeSize * 9) / 5

	// LEAF_BULK_SPLIT_TARGET - target 75% fullness when bulk splitting
	LEAF_BULK_SPLIT_TARGET = (leaf.LeafNodeSize * 3) / 4
)
