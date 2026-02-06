package branch

import (
	"fmt"

	"github.com/jam-duna/jamduna/bmt/beatree"
	"github.com/jam-duna/jamduna/bmt/beatree/allocator"
)

// Separator represents a key separator and its corresponding child page.
type Separator struct {
	Key   beatree.Key
	Child allocator.PageNumber
}

// Node represents a branch node in the B-tree.
// Branch nodes contain separators that divide the key space and point to child pages.
type Node struct {
	// Separators are sorted by key
	Separators []Separator
	// LeftmostChild is the page number for keys less than the first separator
	LeftmostChild allocator.PageNumber
}

// NewNode creates a new empty branch node.
func NewNode(leftmostChild allocator.PageNumber) *Node {
	return &Node{
		Separators:    make([]Separator, 0, MaxSeparators),
		LeftmostChild: leftmostChild,
	}
}

// Insert inserts a separator into the branch node.
// Separators must be inserted in sorted order.
func (n *Node) Insert(sep Separator) error {
	if len(n.Separators) >= MaxSeparators {
		return fmt.Errorf("branch node full: cannot insert separator")
	}

	// Binary search for insertion point
	pos := 0
	for pos < len(n.Separators) && n.Separators[pos].Key.Compare(sep.Key) < 0 {
		pos++
	}

	// Check for duplicate
	if pos < len(n.Separators) && n.Separators[pos].Key.Equal(sep.Key) {
		return fmt.Errorf("separator key already exists")
	}

	// Insert at position
	n.Separators = append(n.Separators, Separator{})
	copy(n.Separators[pos+1:], n.Separators[pos:])
	n.Separators[pos] = sep

	return nil
}

// FindChild returns the page number for a given key.
func (n *Node) FindChild(key beatree.Key) allocator.PageNumber {
	// Binary search for the right separator
	for i := 0; i < len(n.Separators); i++ {
		if key.Compare(n.Separators[i].Key) < 0 {
			// Key is less than this separator, return previous child
			if i == 0 {
				return n.LeftmostChild
			}
			return n.Separators[i-1].Child
		}
	}

	// Key is >= all separators, return last child
	if len(n.Separators) == 0 {
		return n.LeftmostChild
	}
	return n.Separators[len(n.Separators)-1].Child
}


// NumSeparators returns the number of separators in the node.
func (n *Node) NumSeparators() int {
	return len(n.Separators)
}

// EstimateSize estimates the serialized size of the branch node in bytes.
func (n *Node) EstimateSize() int {
	// Header: 4 bytes (separator count) + 4 bytes (reserved) + 8 bytes (leftmost child)
	size := 16
	// Each separator: 32 bytes (key) + 8 bytes (child page number)
	size += len(n.Separators) * 40
	return size
}

// Serialize serializes the branch node to bytes.
func (n *Node) Serialize() ([]byte, error) {
	estimatedSize := n.EstimateSize()
	buf := make([]byte, 0, estimatedSize)

	// Write header: number of separators (4 bytes) + page type (1 byte) + reserved (3 bytes)
	numSeps := uint32(len(n.Separators))
	buf = append(buf, byte(numSeps), byte(numSeps>>8), byte(numSeps>>16), byte(numSeps>>24))
	buf = append(buf, 0x02) // Page type: 0x02 = branch
	buf = append(buf, 0, 0, 0) // Reserved

	// Write leftmost child (8 bytes)
	leftChild := uint64(n.LeftmostChild)
	buf = append(buf,
		byte(leftChild), byte(leftChild>>8), byte(leftChild>>16), byte(leftChild>>24),
		byte(leftChild>>32), byte(leftChild>>40), byte(leftChild>>48), byte(leftChild>>56))

	// Write separators
	for _, sep := range n.Separators {
		// Write separator key (32 bytes)
		buf = append(buf, sep.Key[:]...)

		// Write child page number (8 bytes)
		childPage := uint64(sep.Child)
		buf = append(buf,
			byte(childPage), byte(childPage>>8), byte(childPage>>16), byte(childPage>>24),
			byte(childPage>>32), byte(childPage>>40), byte(childPage>>48), byte(childPage>>56))
	}

	return buf, nil
}

// DeserializeBranchNode deserializes bytes into a branch node.
func DeserializeBranchNode(data []byte) (*Node, error) {
	if len(data) < 16 {
		return nil, fmt.Errorf("insufficient data for branch node header")
	}

	// Read header
	numSeps := uint32(data[0]) | uint32(data[1])<<8 | uint32(data[2])<<16 | uint32(data[3])<<24
	// Skip reserved bytes (4-7)

	// Read leftmost child
	leftChild := uint64(data[8]) | uint64(data[9])<<8 | uint64(data[10])<<16 | uint64(data[11])<<24 |
		uint64(data[12])<<32 | uint64(data[13])<<40 | uint64(data[14])<<48 | uint64(data[15])<<56

	node := NewNode(allocator.PageNumber(leftChild))
	offset := 16

	// Read separators
	for i := uint32(0); i < numSeps; i++ {
		if offset+40 > len(data) { // 32 bytes key + 8 bytes child page
			return nil, fmt.Errorf("insufficient data for separator %d", i)
		}

		// Read separator key
		var key beatree.Key
		copy(key[:], data[offset:offset+32])
		offset += 32

		// Read child page number
		childPage := uint64(data[offset]) | uint64(data[offset+1])<<8 | uint64(data[offset+2])<<16 | uint64(data[offset+3])<<24 |
			uint64(data[offset+4])<<32 | uint64(data[offset+5])<<40 | uint64(data[offset+6])<<48 | uint64(data[offset+7])<<56
		offset += 8

		separator := Separator{
			Key:   key,
			Child: allocator.PageNumber(childPage),
		}

		node.Separators = append(node.Separators, separator)
	}

	return node, nil
}

// IsFull returns true if the node cannot accept more separators.
func (n *Node) IsFull() bool {
	return n.EstimateSize() >= BranchNodeSize
}

// UpdateChild modifies a single child pointer in-place using binary search.
// Updates either LeftmostChild or a child pointer in Separators.
// Returns: (found bool, error)
//   - found: true if oldChild was found and replaced with newChild
//   - error: non-nil if operation failed
func (n *Node) UpdateChild(oldChild, newChild allocator.PageNumber) (bool, error) {
	// Check leftmost child first
	if n.LeftmostChild == oldChild {
		n.LeftmostChild = newChild
		return true, nil
	}

	// Binary search through separators for the old child
	for i := range n.Separators {
		if n.Separators[i].Child == oldChild {
			n.Separators[i].Child = newChild
			return true, nil
		}
	}

	// Child not found
	return false, nil
}

// InsertSeparator adds a separator without copying all existing separators.
// This is a direct alias to Insert() for consistency with the CoW API pattern.
// Returns error if separator already exists or node is full.
func (n *Node) InsertSeparator(sep Separator) error {
	return n.Insert(sep)
}

// Clone creates a shallow copy of the node with clear ownership semantics.
//
// OWNERSHIP SEMANTICS:
//   - Separators slice: COPIED (new backing array allocated)
//   - Separator.Key: COPIED (array type, copied by value)
//   - Separator.Child: COPIED (scalar type, copied by value)
//   - LeftmostChild: COPIED (scalar type, copied by value)
//
// RATIONALE:
//   - Separators slice is copied so the new node can add/remove separators without affecting the original
//   - All fields are value types, so no shared references
//   - This provides CoW semantics: modifications are isolated to the cloned node
//
// SAFETY:
//   - Safe for concurrent reads of the original node
//   - NOT safe for concurrent writes (caller must ensure exclusive access during clone+modify)
//
// PERFORMANCE:
//   - O(S) where S = number of separators (copies separator metadata only)
//   - Each separator is 40 bytes (32-byte key + 8-byte page number)
//   - Much cheaper than copying child subtrees
func (n *Node) Clone() *Node {
	if n == nil {
		return nil
	}

	// Allocate new separators slice with same capacity to minimize reallocations
	newSeparators := make([]Separator, len(n.Separators), cap(n.Separators))

	// Copy separators (all fields are value types, no shared references)
	copy(newSeparators, n.Separators)

	return &Node{
		Separators:    newSeparators,
		LeftmostChild: n.LeftmostChild,
	}
}
