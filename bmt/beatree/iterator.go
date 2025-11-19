package beatree

import (
	"bytes"
	"fmt"

	"github.com/colorfulnotion/jam/bmt/beatree/allocator"
)

// Iterator provides ordered iteration over the beatree state.
//
// This combines in-memory staging (primary and secondary) with on-disk leaf values.
// Currently synchronous. Future: Async page loading for better concurrency.
type Iterator struct {
	// Staging iterators (in-memory overlays)
	primaryIter   *stagingIterator
	secondaryIter *stagingIterator

	// Leaf iterator (on-disk values)
	// Currently nil - disk iteration to be added
	leafIter *leafIterator

	// Range bounds
	start Key
	end   *Key // nil means unbounded
}

// NewIterator creates a new iterator over the beatree.
//
// The iterator will return all key-value pairs in [start, end).
// If end is nil, the iterator is unbounded (returns all keys >= start).
//
// This includes both staging (in-memory) and committed (on-disk) data.
func NewIterator(
	primaryStaging map[Key]*Change,
	secondaryStaging map[Key]*Change,
	branchStore *allocator.Store,
	leafStore *allocator.Store,
	root allocator.PageNumber,
	start Key,
	end *Key,
) *Iterator {
	var leafIter *leafIterator
	if root.IsValid() && leafStore != nil {
		leafIter = newLeafIterator(branchStore, leafStore, root, start, end)
	} else {
	}

	return &Iterator{
		primaryIter:   newStagingIterator(primaryStaging, start, end),
		secondaryIter: newStagingIterator(secondaryStaging, start, end),
		leafIter:      leafIter,
		start:         start,
		end:           end,
	}
}

// Next returns the next key-value pair from the iterator.
// Returns nil key when the iterator is exhausted.
//
// The iterator merges three sorted streams:
// 1. Primary staging (most recent, highest priority)
// 2. Secondary staging (sync in progress)
// 3. Leaf values (on-disk, lowest priority)
//
// Delete markers in staging will hide values from lower layers.
func (it *Iterator) Next() (Key, []byte, bool) {
	for {
		// Peek at the next item from each stream
		primaryKey, primaryChange, primaryValid := it.primaryIter.Peek()
		secondaryKey, secondaryChange, secondaryValid := it.secondaryIter.Peek()

		var leafKey Key
		var leafValue []byte
		var leafValid bool
		if it.leafIter != nil {
			leafKey, leafValue, leafValid = it.leafIter.Peek()
		}

		// Determine which stream has the smallest key
		var takeFrom iteratorSource
		var selectedKey Key

		if !primaryValid && !secondaryValid && !leafValid {
			// All streams exhausted
			return Key{}, nil, false
		}

		// Find the minimum key among all valid streams
		candidates := []struct {
			key    Key
			valid  bool
			source iteratorSource
		}{
			{primaryKey, primaryValid, sourcePrimary},
			{secondaryKey, secondaryValid, sourceSecondary},
			{leafKey, leafValid, sourceLeaf},
		}

		selectedKey = Key{}
		takeFrom = sourcePrimary
		minSet := false

		for _, candidate := range candidates {
			if candidate.valid {
				if !minSet || bytes.Compare(candidate.key[:], selectedKey[:]) < 0 {
					selectedKey = candidate.key
					takeFrom = candidate.source
					minSet = true
				}
			}
		}

		// DEBUG: Log which stream we're taking from
		/*
		if takeFrom == sourceLeaf {
		} else if takeFrom == sourcePrimary {
		} else {
		}
		*/

		// Advance the selected stream(s) and handle same-key merging
		var change *Change
		var value []byte

		switch takeFrom {
		case sourcePrimary:
			it.primaryIter.Next() // Consume primary
			change = primaryChange

			// If secondary has the same key, consume it too (primary overrides)
			if secondaryValid && bytes.Compare(selectedKey[:], secondaryKey[:]) == 0 {
				it.secondaryIter.Next()
			}
			// If leaf has the same key, consume it too (primary overrides)
			if leafValid && bytes.Compare(selectedKey[:], leafKey[:]) == 0 {
				it.leafIter.Next()
			}

			// Handle delete markers - skip to next item
			if change.IsDelete() {
				continue
			}
			value = change.Value

		case sourceSecondary:
			it.secondaryIter.Next() // Consume secondary
			change = secondaryChange

			// If leaf has the same key, consume it too (secondary overrides)
			if leafValid && bytes.Compare(selectedKey[:], leafKey[:]) == 0 {
				it.leafIter.Next()
			}

			// Handle delete markers - skip to next item
			if change.IsDelete() {
				continue
			}
			value = change.Value

		case sourceLeaf:
			it.leafIter.Next() // Consume leaf
			value = leafValue
		}

		// Return the value
		return selectedKey, value, true
	}
}

// iteratorSource identifies which stream we're taking from
type iteratorSource int

const (
	sourcePrimary iteratorSource = iota
	sourceSecondary
	sourceLeaf
)

// stagingIterator iterates over a staging map in sorted key order.
type stagingIterator struct {
	items []stagingItem
	index int
}

type stagingItem struct {
	key    Key
	change *Change
}

// newStagingIterator creates an iterator over a staging map.
// Returns items in [start, end) range.
func newStagingIterator(staging map[Key]*Change, start Key, end *Key) *stagingIterator {
	if staging == nil {
		return &stagingIterator{
			items: []stagingItem{},
			index: 0,
		}
	}

	// Collect items in range
	items := make([]stagingItem, 0, len(staging))
	for key, change := range staging {
		// Check if key is in range [start, end)
		if bytes.Compare(key[:], start[:]) < 0 {
			continue // Before start
		}
		if end != nil && bytes.Compare(key[:], (*end)[:]) >= 0 {
			continue // At or after end
		}

		items = append(items, stagingItem{key: key, change: change})
	}

	// Sort by key
	sortStagingItems(items)

	return &stagingIterator{
		items: items,
		index: 0,
	}
}

// Peek returns the next item without consuming it.
// Returns (zero key, nil, false) if exhausted.
func (si *stagingIterator) Peek() (Key, *Change, bool) {
	if si.index >= len(si.items) {
		return Key{}, nil, false
	}
	item := si.items[si.index]
	return item.key, item.change, true
}

// Next advances the iterator and returns the current item.
// Returns (zero key, nil, false) if exhausted.
func (si *stagingIterator) Next() (Key, *Change, bool) {
	key, change, valid := si.Peek()
	if valid {
		si.index++
	}
	return key, change, valid
}

// sortStagingItems sorts items by key (simple insertion sort for small arrays)
func sortStagingItems(items []stagingItem) {
	for i := 1; i < len(items); i++ {
		key := items[i]
		j := i - 1
		for j >= 0 && bytes.Compare(items[j].key[:], key.key[:]) > 0 {
			items[j+1] = items[j]
			j--
		}
		items[j+1] = key
	}
}

// leafEntry represents a key-value pair in a leaf node for iteration.
type leafEntry struct {
	Key           Key
	Value         []byte
	ValueType     uint8 // 0 = inline, 1 = overflow
	OverflowPage  allocator.PageNumber
	OverflowSize  uint64
	OverflowPages []allocator.PageNumber // All page numbers for values ≤15 pages
}

// leafNode represents a simplified leaf node for iteration purposes.
type leafNode struct {
	Entries []leafEntry
}

// leafIterator iterates over on-disk leaf values.
// This provides disk-backed iteration by walking the committed tree structure.
type leafIterator struct {
	branchStore *allocator.Store
	leafStore   *allocator.Store

	// Current iteration state
	currentPage  allocator.PageNumber
	currentIndex int          // Index within current leaf page
	currentNode  *leafNode    // Currently loaded leaf node

	// Range bounds
	start Key
	end   *Key // nil means unbounded

	// Stack for tree traversal
	traversalStack []traversalEntry

	// Iterator state
	finished bool
}

// traversalEntry represents a position in the tree traversal
type traversalEntry struct {
	pageNum allocator.PageNumber
	index   int // Index of next child to explore in branch node
}

// newLeafIterator creates a new leaf iterator starting from the given root.
// Implements disk-backed iteration with tree traversal.
func newLeafIterator(
	branchStore *allocator.Store,
	leafStore *allocator.Store,
	root allocator.PageNumber,
	start Key,
	end *Key,
) *leafIterator {
	iter := &leafIterator{
		branchStore:    branchStore,
		leafStore:      leafStore,
		currentPage:    allocator.InvalidPageNumber,
		currentIndex:   0,
		currentNode:    nil,
		start:          start,
		end:            end,
		traversalStack: make([]traversalEntry, 0, 8), // Pre-allocate for typical tree depth
		finished:       false,
	}

	// Start by finding the first leaf page that might contain keys >= start
	if root.IsValid() {
		iter.seekToStart(root)
	} else {
		iter.finished = true
	}

	return iter
}

// seekToStart navigates to the first leaf page that might contain keys >= start.
func (li *leafIterator) seekToStart(root allocator.PageNumber) {
	// Start from root and traverse down the tree
	currentPageId := root

	for {
		if !currentPageId.IsValid() {
			li.finished = true
			return
		}

		// Load page data to determine if it's a leaf or branch
		pageData, err := li.leafStore.ReadPage(currentPageId)
		if err != nil {
			li.finished = true
			return
		}

		// Check if this is a leaf page
		if li.isLeafPage(pageData) {
			// Found a leaf page - load it and find start index
			li.leafStore.PagePool().Dealloc(pageData)
			li.currentPage = currentPageId
			li.loadCurrentPage()
			li.findStartIndex()
			return
		}


		// This is a branch page - find child to follow
		childPageId := li.findChildInBranch(pageData, li.start)
		li.leafStore.PagePool().Dealloc(pageData)

		if !childPageId.IsValid() {
			li.finished = true
			return
		}

		// Push current branch to traversal stack for later navigation
		li.traversalStack = append(li.traversalStack, traversalEntry{
			pageNum: currentPageId,
			index:   0, // Will be updated if we need to backtrack
		})

		currentPageId = childPageId
	}
}

// loadCurrentPage loads the current page into currentNode.
func (li *leafIterator) loadCurrentPage() {

	if !li.currentPage.IsValid() {
		li.finished = true
		return
	}

	// Read leaf page from disk
	pageData, err := li.leafStore.ReadPage(li.currentPage)
	if err != nil {
		li.finished = true
		return
	}
	defer li.leafStore.PagePool().Dealloc(pageData)

	// Deserialize the page data
	li.currentNode = li.deserializeLeafPage(pageData)
	if li.currentNode == nil {
		li.finished = true
	} else {
	}
}

// deserializeLeafPage creates a leaf node from page data.
func (li *leafIterator) deserializeLeafPage(pageData []byte) *leafNode {
	// Deserialize leaf page data directly to avoid import cycle
	// This implements the same logic as leaf.DeserializeLeafNode but without the import


	if len(pageData) < 8 {
		return nil // insufficient data for leaf node header
	}

	// Read header
	numEntries := uint32(pageData[0]) | uint32(pageData[1])<<8 | uint32(pageData[2])<<16 | uint32(pageData[3])<<24
	// Skip reserved bytes (4-7)

	node := &leafNode{
		Entries: make([]leafEntry, 0, numEntries),
	}
	offset := 8

	for i := uint32(0); i < numEntries; i++ {
		if offset+33 > len(pageData) { // 32 bytes key + 1 byte type
			return nil // insufficient data for entry
		}

		// Read key
		var key Key
		copy(key[:], pageData[offset:offset+32])
		offset += 32

		// Read value type
		valueType := pageData[offset]
		offset++

		entry := leafEntry{
			Key:       key,
			ValueType: valueType,
		}

		if valueType == 0 { // ValueTypeInline
			if offset+4 > len(pageData) {
				return nil // insufficient data for inline value length
			}

			valueLen := uint32(pageData[offset]) | uint32(pageData[offset+1])<<8 | uint32(pageData[offset+2])<<16 | uint32(pageData[offset+3])<<24
			offset += 4

			if offset+int(valueLen) > len(pageData) {
				return nil // insufficient data for inline value
			}

			entry.Value = make([]byte, valueLen)
			copy(entry.Value, pageData[offset:offset+int(valueLen)])
			offset += int(valueLen)
		} else { // ValueTypeOverflow
			if offset+18 > len(pageData) { // 8 bytes page + 8 bytes size + 2 bytes num pages (minimum)
				return nil // insufficient data for overflow value
			}

			// Read overflow page number (8 bytes)
			overflowPage := allocator.PageNumber(
				uint64(pageData[offset]) | uint64(pageData[offset+1])<<8 |
					uint64(pageData[offset+2])<<16 | uint64(pageData[offset+3])<<24 |
					uint64(pageData[offset+4])<<32 | uint64(pageData[offset+5])<<40 |
					uint64(pageData[offset+6])<<48 | uint64(pageData[offset+7])<<56,
			)
			offset += 8

			// Read overflow size (8 bytes)
			overflowSize := uint64(pageData[offset]) | uint64(pageData[offset+1])<<8 |
				uint64(pageData[offset+2])<<16 | uint64(pageData[offset+3])<<24 |
				uint64(pageData[offset+4])<<32 | uint64(pageData[offset+5])<<40 |
				uint64(pageData[offset+6])<<48 | uint64(pageData[offset+7])<<56
			offset += 8

			// Read number of overflow pages (2 bytes)
			numPages := uint16(pageData[offset]) | uint16(pageData[offset+1])<<8
			offset += 2

			// Read overflow page numbers
			if offset+int(numPages)*8 > len(pageData) {
				return nil // insufficient data for overflow pages
			}
			overflowPages := make([]allocator.PageNumber, numPages)
			for j := uint16(0); j < numPages; j++ {
				pn := allocator.PageNumber(
					uint64(pageData[offset]) | uint64(pageData[offset+1])<<8 |
						uint64(pageData[offset+2])<<16 | uint64(pageData[offset+3])<<24 |
						uint64(pageData[offset+4])<<32 | uint64(pageData[offset+5])<<40 |
						uint64(pageData[offset+6])<<48 | uint64(pageData[offset+7])<<56,
				)
				overflowPages[j] = pn
				offset += 8
			}

			// Read cached value prefix (32 bytes)
			if offset+32 > len(pageData) {
				return nil // insufficient data for cached value prefix
			}
			valuePrefix := make([]byte, 32)
			copy(valuePrefix, pageData[offset:offset+32])
			offset += 32

			entry.OverflowPage = overflowPage
			entry.OverflowSize = overflowSize
			entry.OverflowPages = overflowPages
			entry.Value = valuePrefix
		}

		node.Entries = append(node.Entries, entry)
	}

	return node
}

// isLeafPage determines if page data represents a leaf page.
// Check page type using the page type byte in the header.
func (li *leafIterator) isLeafPage(pageData []byte) bool {
	if len(pageData) < 8 {
		return false
	}

	// Read the page type byte (byte 4)
	pageType := pageData[4]

	// Page types:
	// 0x00 = unknown/legacy (use heuristic)
	// 0x01 = leaf
	// 0x02 = branch
	if pageType == 0x01 {
		return true
	} else if pageType == 0x02 {
		return false
	}

	// Legacy pages without type indicator - use heuristic
	count := uint32(pageData[0]) | uint32(pageData[1])<<8 | uint32(pageData[2])<<16 | uint32(pageData[3])<<24

	if count == 0 {
		return true // Empty leaf
	}

	// Heuristic for legacy pages: assume leaf if count is large
	return count > 100 // Branch pages typically have < 100 separators
}

// findChildInBranch finds the appropriate child page in a branch node for the given key.
func (li *leafIterator) findChildInBranch(pageData []byte, key Key) allocator.PageNumber {
	if len(pageData) < 16 { // Need at least header + leftmost child
		return allocator.InvalidPageNumber
	}

	// Read header
	numSeps := uint32(pageData[0]) | uint32(pageData[1])<<8 | uint32(pageData[2])<<16 | uint32(pageData[3])<<24

	// Read leftmost child (8 bytes starting at offset 8)
	leftChild := uint64(pageData[8]) | uint64(pageData[9])<<8 | uint64(pageData[10])<<16 | uint64(pageData[11])<<24 |
		uint64(pageData[12])<<32 | uint64(pageData[13])<<40 | uint64(pageData[14])<<48 | uint64(pageData[15])<<56

	offset := 16

	// Search through separators to find the right child
	for i := uint32(0); i < numSeps; i++ {
		if offset+40 > len(pageData) { // 32 bytes key + 8 bytes child page
			break
		}

		// Read separator key
		var sepKey Key
		copy(sepKey[:], pageData[offset:offset+32])
		offset += 32

		// Read child page number
		childPage := uint64(pageData[offset]) | uint64(pageData[offset+1])<<8 | uint64(pageData[offset+2])<<16 | uint64(pageData[offset+3])<<24 |
			uint64(pageData[offset+4])<<32 | uint64(pageData[offset+5])<<40 | uint64(pageData[offset+6])<<48 | uint64(pageData[offset+7])<<56
		offset += 8

		// If our key is less than this separator, return the previous child
		if bytes.Compare(key[:], sepKey[:]) < 0 {
			if i == 0 {
				return allocator.PageNumber(leftChild)
			}
			// Return the child from the previous separator (we need to track it)
			// For simplicity, re-read the previous separator's child
			prevOffset := int(16 + (i-1)*40 + 32) // Skip to previous child page
			if prevOffset+8 <= len(pageData) {
				prevChild := uint64(pageData[prevOffset]) | uint64(pageData[prevOffset+1])<<8 |
					uint64(pageData[prevOffset+2])<<16 | uint64(pageData[prevOffset+3])<<24 |
					uint64(pageData[prevOffset+4])<<32 | uint64(pageData[prevOffset+5])<<40 |
					uint64(pageData[prevOffset+6])<<48 | uint64(pageData[prevOffset+7])<<56
				return allocator.PageNumber(prevChild)
			}
			return allocator.PageNumber(leftChild)
		}

		// If this is the last separator and key >= separator, return this child
		if i == numSeps-1 {
			return allocator.PageNumber(childPage)
		}
	}

	// If no separators or key is less than first separator, return leftmost child
	return allocator.PageNumber(leftChild)
}

// findStartIndex finds the first entry in currentNode that is >= start.
func (li *leafIterator) findStartIndex() {
	if li.currentNode == nil {
		li.currentIndex = 0
		return
	}

	// Binary search for the first entry >= start
	li.currentIndex = 0
	for li.currentIndex < len(li.currentNode.Entries) {
		entryKey := li.currentNode.Entries[li.currentIndex].Key
		if bytes.Compare(entryKey[:], li.start[:]) >= 0 {
			break
		}
		li.currentIndex++
	}

	// If no entries >= start, we need to advance to the next leaf page
	if li.currentIndex >= len(li.currentNode.Entries) {
		li.advanceToNextLeaf()
	}
}

// advanceToNextLeaf moves to the next leaf page in the tree.
func (li *leafIterator) advanceToNextLeaf() {
	// Use traversal stack to find next leaf page
	// This implements a simplified in-order tree traversal

	for len(li.traversalStack) > 0 {
		// Peek at the top of the stack (don't pop yet)
		entry := li.traversalStack[len(li.traversalStack)-1]

		// Load the branch page to find next child
		pageData, err := li.leafStore.ReadPage(entry.pageNum)
		if err != nil {
			// Pop invalid entry and continue
			li.traversalStack = li.traversalStack[:len(li.traversalStack)-1]
			continue
		}

		// Find next child in this branch (after the current index)
		nextChild := li.findNextChildInBranch(pageData, entry.index)
		li.leafStore.PagePool().Dealloc(pageData)

		if nextChild.IsValid() {
			// Increment the index for this branch entry and keep it on the stack
			li.traversalStack[len(li.traversalStack)-1].index++

			// Found next child - traverse down to find next leaf
			li.seekToLeafFrom(nextChild)
			return
		} else {
			// No more children in this branch - pop it and try parent
			li.traversalStack = li.traversalStack[:len(li.traversalStack)-1]
		}
	}

	// No more leaves to traverse
	li.finished = true
}

// findNextChildInBranch finds the next child after the given separator index.
func (li *leafIterator) findNextChildInBranch(pageData []byte, separatorIdx int) allocator.PageNumber {
	if len(pageData) < 16 {
		return allocator.InvalidPageNumber
	}

	// Read header
	numSeps := uint32(pageData[0]) | uint32(pageData[1])<<8 | uint32(pageData[2])<<16 | uint32(pageData[3])<<24

	// If we haven't exhausted separators, return the child of the next separator
	if separatorIdx < int(numSeps) {
		offset := 16 + separatorIdx*40 + 32 // Skip to child page of separator at separatorIdx
		if offset+8 <= len(pageData) {
			childPage := uint64(pageData[offset]) | uint64(pageData[offset+1])<<8 |
				uint64(pageData[offset+2])<<16 | uint64(pageData[offset+3])<<24 |
				uint64(pageData[offset+4])<<32 | uint64(pageData[offset+5])<<40 |
				uint64(pageData[offset+6])<<48 | uint64(pageData[offset+7])<<56
			return allocator.PageNumber(childPage)
		}
	}

	return allocator.InvalidPageNumber
}

// seekToLeafFrom traverses down from the given page to find a leaf.
func (li *leafIterator) seekToLeafFrom(pageId allocator.PageNumber) {
	currentPageId := pageId

	for {
		if !currentPageId.IsValid() {
			li.finished = true
			return
		}

		// Load page data
		pageData, err := li.leafStore.ReadPage(currentPageId)
		if err != nil {
			li.finished = true
			return
		}

		// Check if this is a leaf page
		if li.isLeafPage(pageData) {
			// Found a leaf page
			li.leafStore.PagePool().Dealloc(pageData)
			li.currentPage = currentPageId
			li.loadCurrentPage()
			li.currentIndex = 0 // Start from beginning of new leaf
			return
		}

		// This is a branch - find leftmost child
		leftmostChild := li.getLeftmostChildFromBranch(pageData)
		li.leafStore.PagePool().Dealloc(pageData)

		if !leftmostChild.IsValid() {
			li.finished = true
			return
		}

		currentPageId = leftmostChild
	}
}

// getLeftmostChildFromBranch gets the leftmost child from a branch page.
func (li *leafIterator) getLeftmostChildFromBranch(pageData []byte) allocator.PageNumber {
	if len(pageData) < 16 {
		return allocator.InvalidPageNumber
	}

	// Read leftmost child (8 bytes starting at offset 8)
	leftChild := uint64(pageData[8]) | uint64(pageData[9])<<8 | uint64(pageData[10])<<16 | uint64(pageData[11])<<24 |
		uint64(pageData[12])<<32 | uint64(pageData[13])<<40 | uint64(pageData[14])<<48 | uint64(pageData[15])<<56

	return allocator.PageNumber(leftChild)
}

// Peek returns the next entry without consuming it.
func (li *leafIterator) Peek() (Key, []byte, bool) {
	if li.finished {
		return Key{}, nil, false
	}
	if li.currentNode == nil {
		return Key{}, nil, false
	}

	// Check if we're past the end of current page
	if li.currentIndex >= len(li.currentNode.Entries) {
		li.advanceToNextLeaf()
		if li.finished {
			return Key{}, nil, false
		}
	}

	// Check if we're beyond the end bound
	if li.currentIndex >= len(li.currentNode.Entries) {
		return Key{}, nil, false
	}

	entry := li.currentNode.Entries[li.currentIndex]

	// Check end bound
	if li.end != nil && bytes.Compare(entry.Key[:], (*li.end)[:]) >= 0 {
		li.finished = true
		return Key{}, nil, false
	}

	// Check if value is overflow - if so, read full value from overflow pages
	if entry.ValueType == 1 { // ValueTypeOverflow
		var fullValue []byte
		var err error

		// Check if we have all pages stored in the entry
		if len(entry.OverflowPages) > 0 {
			// We have all page numbers - read directly without following pointers
			fullValue, err = li.readOverflowValueWithPages(entry.OverflowPages, entry.OverflowSize)
		} else {
			// Need to follow overflow page pointers (for values >15 pages)
			fullValue, err = li.readOverflowValue(entry.OverflowPage, entry.OverflowSize)
		}

		if err != nil {
			// Propagate error - don't kill the entire iterator
			// Skip this entry by advancing to next
			li.currentIndex++
			// Recursively call Peek to get the next entry
			return li.Peek()
		}
		if fullValue == nil {
			// Size mismatch or not enough pages - skip and continue
			li.currentIndex++
			return li.Peek()
		}
		return entry.Key, fullValue, true
	}

	return entry.Key, entry.Value, true
}

// Next advances the iterator and returns the current entry.
func (li *leafIterator) Next() (Key, []byte, bool) {
	key, value, valid := li.Peek()
	if valid {
		li.currentIndex++
	}
	return key, value, valid
}

// readOverflowValueWithPages reads overflow value when we have all page numbers.
// This is more efficient than following pointers.
func (li *leafIterator) readOverflowValueWithPages(pages []allocator.PageNumber, valueSize uint64) ([]byte, error) {
	// Allocate result buffer
	value := make([]byte, 0, valueSize)

	// Read each page and collect data
	var firstOverflowPage *OverflowPage
	for i, pageNum := range pages {
		pageData, err := li.leafStore.ReadPage(pageNum)
		if err != nil {
			return nil, err
		}

		// Decode overflow page
		overflowPage, err := DecodeOverflowPage(pageData)
		if err != nil {
			return nil, err
		}

		// Save first overflow page for potential pointer following
		if i == 0 {
			firstOverflowPage = overflowPage
		}

		// Add data bytes from this page
		value = append(value, overflowPage.Bytes...)
	}

	// Check if we need more pages (value requires >15 pages)
	// The cell can only store 15 page numbers, so if we need more,
	// we must follow the pointers in the first overflow page
	if uint64(len(value)) < valueSize && firstOverflowPage != nil && len(firstOverflowPage.Pointers) > 0 {
		// Follow pointers to get remaining pages
		for _, pageNum := range firstOverflowPage.Pointers {
			pageData, err := li.leafStore.ReadPage(pageNum)
			if err != nil {
				return nil, err
			}

			overflowPage, err := DecodeOverflowPage(pageData)
			if err != nil {
				return nil, err
			}

			value = append(value, overflowPage.Bytes...)
		}
	}

	// Verify we got the right amount of data
	if len(value) != int(valueSize) {
		return nil, fmt.Errorf("size mismatch: got %d, expected %d", len(value), valueSize)
	}

	return value, nil
}

// readOverflowValue reads the full value from overflow pages by following pointers.
// Uses the first overflow page number and total size from the entry.
// This is used when OverflowPages is empty (for values >15 pages).
func (li *leafIterator) readOverflowValue(firstPage allocator.PageNumber, valueSize uint64) ([]byte, error) {
	// Calculate total pages needed
	totalPages := TotalNeededPages(int(valueSize))

	// Allocate result buffer
	value := make([]byte, 0, valueSize)

	// Collect all page numbers (start with first page, then read more from pages)
	pageNumbers := make([]allocator.PageNumber, 0, totalPages)
	pageNumbers = append(pageNumbers, firstPage)

	// Read pages and collect both data and additional page numbers
	for i := 0; i < totalPages; i++ {
		if i >= len(pageNumbers) {
			return nil, fmt.Errorf("not enough page numbers: need %d, have %d", totalPages, len(pageNumbers))
		}

		// Read the page
		pageData, err := li.leafStore.ReadPage(pageNumbers[i])
		if err != nil {
			return nil, err
		}

		// Decode overflow page
		overflowPage, err := DecodeOverflowPage(pageData)
		if err != nil {
			return nil, err
		}

		// Add any page numbers from this page
		pageNumbers = append(pageNumbers, overflowPage.Pointers...)

		// Add data bytes from this page
		value = append(value, overflowPage.Bytes...)
	}

	// Verify we got the right amount of data
	if len(value) != int(valueSize) {
		return nil, fmt.Errorf("size mismatch: got %d, expected %d", len(value), valueSize)
	}

	return value, nil
}
