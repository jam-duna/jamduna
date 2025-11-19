package core

// Pages: efficient node storage.
//
// Because each node in the trie is exactly 32 bytes, we can easily pack groups of nodes into
// a predictable paged representation regardless of the information in the trie.
//
// Each page is 4096 bytes and stores up to 126 nodes plus a unique 32-byte page identifier,
// with 32 bytes left over.
//
// A page stores a rootless sub-tree with depth 6: that is, it stores up to
// 2 + 4 + 8 + 16 + 32 + 64 nodes at known positions.
// Semantically, all nodes within the page should descend from the layer above, and the
// top two nodes are expected to be siblings. Each page logically has up to 64 child pages, which
// correspond to the rootless sub-tree descending from each of the 64 child nodes on the bottom
// layer.
//
// Every page is referred to by a unique ID, given by parent_id * 2^6 + child_index + 1, where
// the root page has ID 0x00..00. The child index ranges from 0 to 63 and therefore can be
// represented as a 6-bit string.

const (
	// Depth is the depth of the rootless sub-binary tree stored in a page.
	Depth = 6

	// NodesPerPage is the total number of nodes stored in one page.
	// It depends on the Depth of the rootless sub-binary tree stored in a page
	// following this formula: (2^(Depth + 1)) - 2
	NodesPerPage = (1 << (Depth + 1)) - 2 // = 126

	// NumChildren is the number of child pages per page (2^Depth = 64).
	NumChildren = 1 << Depth // = 64
)

// RawPage is a page data structure that holds NodesPerPage nodes (126 nodes).
// Each node is 32 bytes, so a RawPage is 126 * 32 = 4032 bytes.
type RawPage [NodesPerPage][32]byte

// NodeIndex returns the index of a node at a given layer and position within that layer.
// Layer 0 contains 2 nodes (indices 0-1)
// Layer 1 contains 4 nodes (indices 2-5)
// Layer 2 contains 8 nodes (indices 6-13)
// Layer 3 contains 16 nodes (indices 14-29)
// Layer 4 contains 32 nodes (indices 30-61)
// Layer 5 contains 64 nodes (indices 62-125)
func NodeIndex(layer, positionInLayer int) int {
	if layer < 0 || layer >= Depth {
		panic("invalid layer")
	}
	nodesBeforeLayer := (1 << (layer + 1)) - 2
	return nodesBeforeLayer + positionInLayer
}

// LayerStart returns the starting index of a given layer.
func LayerStart(layer int) int {
	if layer < 0 || layer >= Depth {
		panic("invalid layer")
	}
	return (1 << (layer + 1)) - 2
}

// LayerSize returns the number of nodes in a given layer.
func LayerSize(layer int) int {
	if layer < 0 || layer >= Depth {
		panic("invalid layer")
	}
	return 1 << (layer + 1)
}
