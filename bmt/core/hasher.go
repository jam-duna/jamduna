package core

// NodeHasher is a trie node hash function specialized for 64 bytes of data.
//
// Note that it is illegal for the produced hash to equal [0; 32], as this value is reserved
// for the terminator node.
//
// A node hasher should domain-separate internal and leaf nodes in some specific way. The
// recommended approach for binary hashes is to set the MSB to 0 or 1 depending on the node kind.
// However, for other kinds of hashes (e.g. Poseidon2 or other algebraic hashes), other labeling
// schemes may be required.
type NodeHasher interface {
	// HashLeaf hashes a leaf node. This should domain-separate the hash
	// according to the node kind.
	HashLeaf(data *LeafData) Node

	// HashInternal hashes an internal node. This should domain-separate
	// the hash according to the node kind.
	HashInternal(data *InternalData) Node

	// NodeKind returns the kind of the given node.
	NodeKind(node Node) NodeKind
}

// ValueHasher is a hasher for arbitrary-length values.
type ValueHasher interface {
	// HashValue hashes an arbitrary-length value.
	HashValue(value []byte) ValueHash
}

// BinaryHasher is a simple interface for representing binary hash functions.
type BinaryHasher interface {
	// Hash produces a 32-byte hash from input.
	Hash(input []byte) [32]byte

	// Hash2x32Concat hashes two 32-byte inputs concatenated together.
	// This is an optional specialization for performance.
	Hash2x32Concat(left, right [32]byte) [32]byte
}

// NodeKindByMSB determines the node kind according to a most-significant bit labeling scheme.
//
// If the MSB is true, it's a leaf. If the node is empty, it's a Terminator. Otherwise, it's
// an internal node.
func NodeKindByMSB(node Node) NodeKind {
	if node[0]>>7 == 1 {
		return NodeKindLeaf
	} else if IsTerminator(node) {
		return NodeKindTerminator
	} else {
		return NodeKindInternal
	}
}

// SetMSB sets the most-significant bit of the node.
func SetMSB(node *Node) {
	node[0] |= 0b10000000
}

// UnsetMSB unsets the most-significant bit of the node.
func UnsetMSB(node *Node) {
	node[0] &= 0b01111111
}

// StandardNodeHasher is a node and value hasher constructed from a simple binary hasher.
//
// This implements a NodeHasher and ValueHasher where the node kind is tagged by setting
// or unsetting the MSB of the hash value.
//
// The binary hash wrapped by this structure must behave approximately like a random oracle over
// the space 2^256, i.e. all 256-bit outputs are valid and inputs are uniformly distributed.
//
// Functions like SHA-2/Blake3/Keccak/Grøstl all meet these criteria.
type StandardNodeHasher struct {
	binaryHasher BinaryHasher
}

// NewStandardNodeHasher creates a new standard node hasher from a binary hasher.
func NewStandardNodeHasher(bh BinaryHasher) *StandardNodeHasher {
	return &StandardNodeHasher{binaryHasher: bh}
}

// HashValue implements ValueHasher.
func (h *StandardNodeHasher) HashValue(value []byte) ValueHash {
	return h.binaryHasher.Hash(value)
}

// HashLeaf implements NodeHasher.
func (h *StandardNodeHasher) HashLeaf(data *LeafData) Node {
	hash := h.binaryHasher.Hash2x32Concat(data.KeyPath, data.ValueHash)
	node := Node(hash)
	SetMSB(&node)
	return node
}

// HashInternal implements NodeHasher.
func (h *StandardNodeHasher) HashInternal(data *InternalData) Node {
	hash := h.binaryHasher.Hash2x32Concat(data.Left, data.Right)
	node := Node(hash)
	UnsetMSB(&node)
	return node
}

// NodeKind implements NodeHasher.
func (h *StandardNodeHasher) NodeKind(node Node) NodeKind {
	return NodeKindByMSB(node)
}
