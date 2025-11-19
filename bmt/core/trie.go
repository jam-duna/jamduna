// Package core provides the foundation layer for the Binary Merkle Trie (BMT).
//
// This package defines the types of a binary merkle trie, generalized over a 256-bit hash function.
// All lookup paths in the trie are 256 bits.
//
// There are three kinds of nodes:
//  1. Internal nodes, which each have two children. The value of an internal node is
//     given by hashing the concatenation of the two child nodes and setting the MSB to 0.
//  2. Leaf nodes, which have zero children. The value of a leaf node is given by hashing
//     the concatenation of the 256-bit lookup path and the hash of the value stored at the leaf,
//     and setting the MSB to 1.
//  3. Terminator nodes, which have the special value of all 0s. These nodes have no children
//     and serve as a stand-in for an empty sub-trie at any height.
//
// All node preimages are 512 bits.
package core

import "bytes"

// Node is a node in the binary trie. It is always 256 bits and is the hash of either
// a LeafData or InternalData, or zeroed if it's a Terminator.
//
// Nodes are labeled by the NodeHasher to indicate whether they are leaves or internal nodes.
// Typically, this is done by setting the MSB.
type Node [32]byte

// KeyPath is the path to a key. All paths have a 256-bit fixed length.
type KeyPath [32]byte

// ValueHash is the hash of a value. It is always 256 bits.
type ValueHash [32]byte

// Terminator is the special node hash value denoting an empty sub-tree.
// When this appears at a given location in the trie, it implies that no key
// with a path beginning with the location has a value.
//
// This value may appear at any height.
var Terminator = Node{}

// NodeKind represents the kind of a node.
type NodeKind int

const (
	// NodeKindTerminator indicates an empty sub-trie.
	NodeKindTerminator NodeKind = iota
	// NodeKindLeaf indicates a sub-trie with a single leaf.
	NodeKindLeaf
	// NodeKindInternal indicates at least two values.
	NodeKindInternal
)

// String returns the string representation of NodeKind.
func (k NodeKind) String() string {
	switch k {
	case NodeKindTerminator:
		return "Terminator"
	case NodeKindLeaf:
		return "Leaf"
	case NodeKindInternal:
		return "Internal"
	default:
		return "Unknown"
	}
}

// InternalData is the data of an internal (branch) node.
type InternalData struct {
	// Left is the hash of the left child of this node.
	Left Node
	// Right is the hash of the right child of this node.
	Right Node
}

// LeafData is the data of a leaf node.
type LeafData struct {
	// KeyPath is the total path to this value within the trie.
	// The actual location of this node may be anywhere along this path,
	// depending on the other data within the trie.
	KeyPath KeyPath
	// ValueHash is the hash of the value carried in this leaf.
	ValueHash ValueHash
	// ValueInline stores small values inline (≤32 bytes) for GP encoding.
	// This allows proper GP encoding that embeds small values directly
	// rather than hashing them.
	ValueInline []byte
	// ValueLen tracks the full value length for GP encoding.
	// This is needed to determine whether to embed or hash the value
	// according to GP spec (Equation D.4).
	ValueLen uint32
}

// NewLeafData creates a new LeafData for standard (non-GP) mode.
func NewLeafData(keyPath KeyPath, valueHash ValueHash) *LeafData {
	return &LeafData{
		KeyPath:   keyPath,
		ValueHash: valueHash,
	}
}

// NewLeafDataGP creates a new LeafData with GP encoding support.
//
// For values ≤32 bytes, stores the value inline for embedded encoding.
// For values >32 bytes, only stores the hash.
func NewLeafDataGP(keyPath KeyPath, value []byte, valueHash ValueHash) *LeafData {
	valueLen := uint32(len(value))
	var valueInline []byte
	if len(value) <= 32 {
		valueInline = make([]byte, len(value))
		copy(valueInline, value)
	}

	return &LeafData{
		KeyPath:     keyPath,
		ValueHash:   valueHash,
		ValueInline: valueInline,
		ValueLen:    valueLen,
	}
}

// NewLeafDataGPOverflow creates a new LeafData for overflow values where we have size but not bytes.
// This is used when loading from Beatree overflow cells.
//
// For values ≤32 bytes, this will panic - use NewLeafDataGP instead with actual bytes.
// For values >32 bytes, creates GP-encoded leaf with hashed value.
func NewLeafDataGPOverflow(keyPath KeyPath, valueSize uint32, valueHash ValueHash) *LeafData {
	if valueSize <= 32 {
		panic("Overflow values must be > 32 bytes, use NewLeafDataGP() for small values")
	}

	return &LeafData{
		KeyPath:     keyPath,
		ValueHash:   valueHash,
		ValueInline: nil, // Hashed values don't store inline bytes
		ValueLen:    valueSize,
	}
}

// IsTerminator returns whether the node is a terminator (all zeros).
func IsTerminator(node Node) bool {
	return bytes.Equal(node[:], Terminator[:])
}

// IsLeaf returns whether the node hash indicates the node is a leaf.
func IsLeaf(hasher NodeHasher, node Node) bool {
	return hasher.NodeKind(node) == NodeKindLeaf
}

// IsInternal returns whether the node hash indicates the node is an internal node.
func IsInternal(hasher NodeHasher, node Node) bool {
	return hasher.NodeKind(node) == NodeKindInternal
}
