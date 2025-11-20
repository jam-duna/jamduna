package leaf

import (
	"fmt"

	"github.com/colorfulnotion/jam/bmt/beatree"
	"github.com/colorfulnotion/jam/bmt/beatree/allocator"
)

// ValueType indicates whether a value is inline or overflow.
type ValueType uint8

const (
	// ValueTypeInline indicates the value is stored inline.
	ValueTypeInline ValueType = 0
	// ValueTypeOverflow indicates the value is stored in overflow pages.
	ValueTypeOverflow ValueType = 1
)

// Entry represents a key-value pair in a leaf node.
type Entry struct {
	Key       beatree.Key
	ValueType ValueType
	// For inline values: the actual value data
	// For overflow values: first 32 bytes of the value (for caching)
	Value []byte
	// For overflow values: the page number of the overflow chain (first page)
	OverflowPage allocator.PageNumber
	// For overflow values: the total size of the value
	OverflowSize uint64
	// For overflow values: ALL page numbers (for values ≤15 pages)
	// This allows reading the value without following overflow page pointers
	// For values >15 pages, this should be empty and we follow pointers instead
	OverflowPages []allocator.PageNumber
}

// Node represents a leaf node in the B-tree.
// Leaf nodes contain key-value pairs.
type Node struct {
	// Entries are sorted by key
	Entries []Entry
}

// NewNode creates a new empty leaf node.
func NewNode() *Node {
	return &Node{
		Entries: make([]Entry, 0),
	}
}

// Insert inserts or updates an entry in the leaf node.
func (n *Node) Insert(entry Entry) error {
	// Binary search for insertion point
	pos := 0
	for pos < len(n.Entries) && n.Entries[pos].Key.Compare(entry.Key) < 0 {
		pos++
	}

	// Check if key already exists (update case)
	if pos < len(n.Entries) && n.Entries[pos].Key.Equal(entry.Key) {
		n.Entries[pos] = entry
		return nil
	}

	// Insert new entry
	n.Entries = append(n.Entries, Entry{})
	copy(n.Entries[pos+1:], n.Entries[pos:])
	n.Entries[pos] = entry

	return nil
}

// Lookup finds an entry by key.
func (n *Node) Lookup(key beatree.Key) (*Entry, bool) {
	// Binary search
	left, right := 0, len(n.Entries)
	for left < right {
		mid := (left + right) / 2
		cmp := n.Entries[mid].Key.Compare(key)
		if cmp == 0 {
			return &n.Entries[mid], true
		} else if cmp < 0 {
			left = mid + 1
		} else {
			right = mid
		}
	}
	return nil, false
}

// Delete removes an entry by key.
func (n *Node) Delete(key beatree.Key) bool {
	// Binary search
	pos := 0
	for pos < len(n.Entries) && n.Entries[pos].Key.Compare(key) < 0 {
		pos++
	}

	if pos >= len(n.Entries) || !n.Entries[pos].Key.Equal(key) {
		return false
	}

	// Remove entry
	copy(n.Entries[pos:], n.Entries[pos+1:])
	n.Entries = n.Entries[:len(n.Entries)-1]

	return true
}

// NumEntries returns the number of entries in the node.
func (n *Node) NumEntries() int {
	return len(n.Entries)
}

// EstimateSize estimates the serialized size of the node in bytes.
func (n *Node) EstimateSize() int {
	size := 8 // Header (num entries, etc.)
	for i := range n.Entries {
		size += 32 // Key
		size += 1  // ValueType
		if n.Entries[i].ValueType == ValueTypeInline {
			size += 4 // Value length
			size += len(n.Entries[i].Value)
		} else {
			size += 8  // OverflowPage
			size += 8  // OverflowSize
			size += 2  // numPages (uint16)
			size += 8 * len(n.Entries[i].OverflowPages) // Page numbers array
			size += 32 // Cached value prefix
		}
	}
	return size
}

// Serialize serializes the leaf node to bytes.
func (n *Node) Serialize() ([]byte, error) {
	estimatedSize := n.EstimateSize()
	buf := make([]byte, 0, estimatedSize)

	// Write header: number of entries (4 bytes) + page type (1 byte) + reserved (3 bytes)
	numEntries := uint32(len(n.Entries))
	buf = append(buf, byte(numEntries), byte(numEntries>>8), byte(numEntries>>16), byte(numEntries>>24))
	buf = append(buf, 0x01) // Page type: 0x01 = leaf
	buf = append(buf, 0, 0, 0) // Reserved

	// Write entries
	for _, entry := range n.Entries {
		// Write key (32 bytes)
		buf = append(buf, entry.Key[:]...)

		// Write value type (1 byte)
		buf = append(buf, byte(entry.ValueType))

		if entry.ValueType == ValueTypeInline {
			// Write inline value
			valueLen := uint32(len(entry.Value))
			buf = append(buf, byte(valueLen), byte(valueLen>>8), byte(valueLen>>16), byte(valueLen>>24))
			buf = append(buf, entry.Value...)
		} else {
			// Write overflow reference
			pageNum := uint64(entry.OverflowPage)
			buf = append(buf,
				byte(pageNum), byte(pageNum>>8), byte(pageNum>>16), byte(pageNum>>24),
				byte(pageNum>>32), byte(pageNum>>40), byte(pageNum>>48), byte(pageNum>>56))

			overflowSize := entry.OverflowSize
			buf = append(buf,
				byte(overflowSize), byte(overflowSize>>8), byte(overflowSize>>16), byte(overflowSize>>24),
				byte(overflowSize>>32), byte(overflowSize>>40), byte(overflowSize>>48), byte(overflowSize>>56))

			// Write number of overflow pages (2 bytes, up to 65535 pages)
			numPages := uint16(len(entry.OverflowPages))
			buf = append(buf, byte(numPages), byte(numPages>>8))

			// Write overflow page numbers (8 bytes each)
			for _, page := range entry.OverflowPages {
				pn := uint64(page)
				buf = append(buf,
					byte(pn), byte(pn>>8), byte(pn>>16), byte(pn>>24),
					byte(pn>>32), byte(pn>>40), byte(pn>>48), byte(pn>>56))
			}

			// Write cached value prefix (32 bytes)
			if len(entry.Value) >= 32 {
				buf = append(buf, entry.Value[:32]...)
			} else {
				buf = append(buf, entry.Value...)
				// Pad with zeros
				for i := len(entry.Value); i < 32; i++ {
					buf = append(buf, 0)
				}
			}
		}
	}

	return buf, nil
}

// Deserialize deserializes bytes into a leaf node.
func DeserializeLeafNode(data []byte) (*Node, error) {
	if len(data) < 8 {
		return nil, fmt.Errorf("insufficient data for leaf node header")
	}

	// Read header
	numEntries := uint32(data[0]) | uint32(data[1])<<8 | uint32(data[2])<<16 | uint32(data[3])<<24
	// Skip reserved bytes (4-7)

	node := NewNode()
	offset := 8

	for i := uint32(0); i < numEntries; i++ {
		if offset+33 > len(data) { // 32 bytes key + 1 byte type
			return nil, fmt.Errorf("insufficient data for entry %d", i)
		}

		// Read key
		var key beatree.Key
		copy(key[:], data[offset:offset+32])
		offset += 32

		// Read value type
		valueType := ValueType(data[offset])
		offset++

		entry := Entry{
			Key:       key,
			ValueType: valueType,
		}

		if valueType == ValueTypeInline {
			if offset+4 > len(data) {
				return nil, fmt.Errorf("insufficient data for inline value length at entry %d", i)
			}

			valueLen := uint32(data[offset]) | uint32(data[offset+1])<<8 | uint32(data[offset+2])<<16 | uint32(data[offset+3])<<24
			offset += 4

			if offset+int(valueLen) > len(data) {
				return nil, fmt.Errorf("insufficient data for inline value at entry %d", i)
			}

			entry.Value = make([]byte, valueLen)
			copy(entry.Value, data[offset:offset+int(valueLen)])
			offset += int(valueLen)
		} else {
			if offset+18 > len(data) { // 8 bytes page + 8 bytes size + 2 bytes num pages (minimum)
				return nil, fmt.Errorf("insufficient data for overflow value at entry %d", i)
			}

			// Read overflow page number
			pageNum := uint64(data[offset]) | uint64(data[offset+1])<<8 | uint64(data[offset+2])<<16 | uint64(data[offset+3])<<24 |
				uint64(data[offset+4])<<32 | uint64(data[offset+5])<<40 | uint64(data[offset+6])<<48 | uint64(data[offset+7])<<56
			offset += 8
			entry.OverflowPage = allocator.PageNumber(pageNum)

			// Read overflow size
			overflowSize := uint64(data[offset]) | uint64(data[offset+1])<<8 | uint64(data[offset+2])<<16 | uint64(data[offset+3])<<24 |
				uint64(data[offset+4])<<32 | uint64(data[offset+5])<<40 | uint64(data[offset+6])<<48 | uint64(data[offset+7])<<56
			offset += 8
			entry.OverflowSize = overflowSize

			// Read number of overflow pages
			numPages := uint16(data[offset]) | uint16(data[offset+1])<<8
			offset += 2

			// Read overflow page numbers
			if offset+int(numPages)*8 > len(data) {
				return nil, fmt.Errorf("insufficient data for overflow pages at entry %d", i)
			}
			entry.OverflowPages = make([]allocator.PageNumber, numPages)
			for j := uint16(0); j < numPages; j++ {
				pn := uint64(data[offset]) | uint64(data[offset+1])<<8 | uint64(data[offset+2])<<16 | uint64(data[offset+3])<<24 |
					uint64(data[offset+4])<<32 | uint64(data[offset+5])<<40 | uint64(data[offset+6])<<48 | uint64(data[offset+7])<<56
				entry.OverflowPages[j] = allocator.PageNumber(pn)
				offset += 8
			}

			// Read cached value prefix
			if offset+32 > len(data) {
				return nil, fmt.Errorf("insufficient data for cached value prefix at entry %d", i)
			}
			entry.Value = make([]byte, 32)
			copy(entry.Value, data[offset:offset+32])
			offset += 32
		}

		node.Entries = append(node.Entries, entry)
	}

	return node, nil
}

// IsFull returns true if the node cannot accept more entries.
func (n *Node) IsFull() bool {
	return n.EstimateSize() >= LeafNodeSize
}

// Split splits this leaf node into two nodes at the midpoint.
// Returns the new right sibling and the separator key (first key of right node).
// The left node (this node) keeps the first half of entries.
func (n *Node) Split() (*Node, beatree.Key, error) {
	if len(n.Entries) < 2 {
		return nil, beatree.Key{}, fmt.Errorf("cannot split leaf with less than 2 entries")
	}

	// Find split point (middle)
	splitIdx := len(n.Entries) / 2

	// Create right sibling with second half of entries
	rightNode := NewNode()
	rightNode.Entries = make([]Entry, len(n.Entries)-splitIdx)
	copy(rightNode.Entries, n.Entries[splitIdx:])

	// Separator is the first key of the right node
	separator := rightNode.Entries[0].Key

	// Keep first half in this node
	n.Entries = n.Entries[:splitIdx]

	return rightNode, separator, nil
}

// NewInlineEntry creates a new inline entry.
func NewInlineEntry(key beatree.Key, value []byte) (Entry, error) {
	if len(value) > MaxLeafValueSize {
		return Entry{}, fmt.Errorf("value too large for inline storage: %d > %d", len(value), MaxLeafValueSize)
	}
	// Defensive copy to prevent caller mutation
	valueCopy := make([]byte, len(value))
	copy(valueCopy, value)
	return Entry{
		Key:       key,
		ValueType: ValueTypeInline,
		Value:     valueCopy,
	}, nil
}

// NewOverflowEntry creates a new overflow entry.
func NewOverflowEntry(key beatree.Key, overflowPage allocator.PageNumber, overflowSize uint64, valuePrefix []byte) Entry {
	// Defensive copy to prevent caller mutation
	// Limit prefix to 32 bytes
	prefixLen := len(valuePrefix)
	if prefixLen > 32 {
		prefixLen = 32
	}
	prefixCopy := make([]byte, prefixLen)
	copy(prefixCopy, valuePrefix[:prefixLen])

	return Entry{
		Key:          key,
		ValueType:    ValueTypeOverflow,
		Value:        prefixCopy,
		OverflowPage: overflowPage,
		OverflowSize: overflowSize,
	}
}

// NewOverflowEntryWithPages creates an overflow entry with ALL page numbers.
// This is used for overflow values that fit in ≤15 pages, where all page numbers
// are known upfront and stored in the leaf entry.
func NewOverflowEntryWithPages(key beatree.Key, pages []allocator.PageNumber, overflowSize uint64, valuePrefix []byte) Entry {
	// Defensive copy to prevent caller mutation
	// Limit prefix to 32 bytes
	prefixLen := len(valuePrefix)
	if prefixLen > 32 {
		prefixLen = 32
	}
	prefixCopy := make([]byte, prefixLen)
	copy(prefixCopy, valuePrefix[:prefixLen])

	// Copy page numbers
	pagesCopy := make([]allocator.PageNumber, len(pages))
	copy(pagesCopy, pages)

	return Entry{
		Key:           key,
		ValueType:     ValueTypeOverflow,
		Value:         prefixCopy,
		OverflowPage:  pages[0], // First page for compatibility
		OverflowSize:  overflowSize,
		OverflowPages: pagesCopy, // All pages for direct access
	}
}

// UpdateEntry modifies an existing entry in-place or inserts if not found.
// Uses binary search for O(log M) lookup where M = number of entries.
// Returns: (found bool, needsSplit bool, error)
//   - found: true if key existed and was updated, false if inserted
//   - needsSplit: true if node size exceeds LeafNodeSize after update
//   - error: non-nil if operation failed
func (n *Node) UpdateEntry(key beatree.Key, value []byte) (bool, bool, error) {
	// Binary search for the key position
	left, right := 0, len(n.Entries)
	for left < right {
		mid := (left + right) / 2
		cmp := n.Entries[mid].Key.Compare(key)
		if cmp == 0 {
			// Key found - update in-place
			// Create new entry to replace the old one
			var newEntry Entry
			var err error
			if len(value) <= MaxLeafValueSize {
				newEntry, err = NewInlineEntry(key, value)
			} else {
				// For overflow values, caller must use Insert with pre-allocated overflow pages
				// UpdateEntry is designed for inline values or same-size updates
				return false, false, fmt.Errorf("value too large for UpdateEntry: %d > %d (use Insert for overflow values)", len(value), MaxLeafValueSize)
			}
			if err != nil {
				return false, false, err
			}

			n.Entries[mid] = newEntry
			needsSplit := n.EstimateSize() >= LeafNodeSize
			return true, needsSplit, nil
		} else if cmp < 0 {
			left = mid + 1
		} else {
			right = mid
		}
	}

	// Key not found - insert at position 'left'
	var newEntry Entry
	var err error
	if len(value) <= MaxLeafValueSize {
		newEntry, err = NewInlineEntry(key, value)
	} else {
		return false, false, fmt.Errorf("value too large for UpdateEntry: %d > %d (use Insert for overflow values)", len(value), MaxLeafValueSize)
	}
	if err != nil {
		return false, false, err
	}

	// Insert at position 'left' (maintains sorted order)
	n.Entries = append(n.Entries, Entry{})
	copy(n.Entries[left+1:], n.Entries[left:])
	n.Entries[left] = newEntry

	needsSplit := n.EstimateSize() >= LeafNodeSize
	return false, needsSplit, nil
}

// DeleteEntry removes an entry in-place using binary search.
// Uses O(log M) lookup where M = number of entries.
// Returns: (found bool, error)
//   - found: true if key was found and deleted, false if key didn't exist
//   - error: non-nil if operation failed
func (n *Node) DeleteEntry(key beatree.Key) (bool, error) {
	// Binary search for the key position
	left, right := 0, len(n.Entries)
	for left < right {
		mid := (left + right) / 2
		cmp := n.Entries[mid].Key.Compare(key)
		if cmp == 0 {
			// Key found - delete it by shifting entries
			copy(n.Entries[mid:], n.Entries[mid+1:])
			n.Entries = n.Entries[:len(n.Entries)-1]
			return true, nil
		} else if cmp < 0 {
			left = mid + 1
		} else {
			right = mid
		}
	}

	// Key not found
	return false, nil
}

// Clone creates a shallow copy of the node with clear ownership semantics.
//
// OWNERSHIP SEMANTICS:
//   - Entries slice: COPIED (new backing array allocated)
//   - Entry.Key: COPIED (array type, copied by value)
//   - Entry.Value: SHARED (slice points to same backing array)
//   - Entry.OverflowPages: SHARED (slice points to same backing array)
//
// RATIONALE:
//   - Entries slice is copied so the new node can add/remove entries without affecting the original
//   - Entry.Value is shared because values are immutable in practice (never modified in-place)
//   - Entry.OverflowPages is shared for the same reason
//   - This provides CoW semantics: modifications to entry list are isolated, but value data is shared
//
// SAFETY:
//   - Safe for concurrent reads of the original node
//   - NOT safe for concurrent writes (caller must ensure exclusive access during clone+modify)
//   - Value buffers MUST NOT be modified after insertion (treat as immutable)
//
// PERFORMANCE:
//   - O(M) where M = number of entries (copies entry metadata only, not value data)
//   - Much cheaper than deep copy which would be O(M × avg_value_size)
func (n *Node) Clone() *Node {
	if n == nil {
		return nil
	}

	// Allocate new entries slice with same capacity to minimize reallocations
	newEntries := make([]Entry, len(n.Entries), cap(n.Entries))

	// Copy entries (shallow copy - Value and OverflowPages slices are shared)
	// This is safe because:
	// 1. Entry.Key is [32]byte (copied by value)
	// 2. Entry.Value and Entry.OverflowPages are shared (immutable in practice)
	copy(newEntries, n.Entries)

	return &Node{
		Entries: newEntries,
	}
}
