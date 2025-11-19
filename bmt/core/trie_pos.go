package core

import "fmt"

// TriePosition encapsulates logic for moving around in paged storage for a binary trie.
type TriePosition struct {
	// The bits after depth are irrelevant
	path      [32]byte
	depth     uint16
	nodeIndex int
}

// NewTriePosition creates a new TriePosition at the root.
func NewTriePosition() *TriePosition {
	return &TriePosition{
		path:      [32]byte{},
		depth:     0,
		nodeIndex: 0,
	}
}

// NewTriePositionFromPath creates a new TriePosition based on the first `depth` bits of `path`.
// Panics if depth is zero.
func NewTriePositionFromPath(path KeyPath, depth uint16) *TriePosition {
	if depth == 0 {
		panic("depth must be non-zero")
	}
	if depth > 256 {
		panic("depth must not exceed 256")
	}

	pagePath := lastPagePath(path[:], depth)
	return &TriePosition{
		path:      path,
		depth:     depth,
		nodeIndex: nodeIndexFromBits(pagePath, len(pagePath)),
	}
}

// IsRoot returns whether the position is at the root.
func (tp *TriePosition) IsRoot() bool {
	return tp.depth == 0
}

// Depth returns the current depth of the position.
func (tp *TriePosition) Depth() uint16 {
	return tp.depth
}

// RawPath returns the raw key at the current position.
// Note that if you have called Up, this might have bits beyond depth which are set.
func (tp *TriePosition) RawPath() [32]byte {
	return tp.path
}

// Down moves the position down by 1, towards either the left or right child.
// Panics on depth out of range.
func (tp *TriePosition) Down(bit bool) {
	if tp.depth == 256 {
		panic("can't descend past 256 bits")
	}

	if int(tp.depth)%Depth == 0 {
		if bit {
			tp.nodeIndex = 1
		} else {
			tp.nodeIndex = 0
		}
	} else {
		children := tp.ChildNodeIndices()
		if bit {
			tp.nodeIndex = children.Right()
		} else {
			tp.nodeIndex = children.Left()
		}
	}

	setBit(tp.path[:], int(tp.depth), bit)
	tp.depth++
}

// Up moves the position up by d bits.
// Panics if d is greater than the current depth.
func (tp *TriePosition) Up(d uint16) {
	prevDepth := tp.depth
	if d > tp.depth {
		panic(fmt.Sprintf("can't move up by %d bits from depth %d", d, prevDepth))
	}

	newDepth := tp.depth - d
	if newDepth == 0 {
		*tp = *NewTriePosition()
		return
	}

	tp.depth = newDepth
	prevPageDepth := (int(prevDepth) + Depth - 1) / Depth
	newPageDepth := (int(tp.depth) + Depth - 1) / Depth

	if prevPageDepth == newPageDepth {
		for i := uint16(0); i < d; i++ {
			tp.nodeIndex = parentNodeIndex(tp.nodeIndex)
		}
	} else {
		pagePath := lastPagePath(tp.path[:], tp.depth)
		tp.nodeIndex = nodeIndexFromBits(pagePath, len(pagePath))
	}
}

// Sibling moves the position to the sibling node.
// Panics if at the root.
func (tp *TriePosition) Sibling() {
	if tp.depth == 0 {
		panic("can't move to sibling of root node")
	}

	bitIdx := int(tp.depth) - 1
	currentBit := getBit(tp.path[:], bitIdx)
	setBit(tp.path[:], bitIdx, !currentBit)
	tp.nodeIndex = siblingIndex(tp.nodeIndex)
}

// PeekLastBit peeks at the last bit of the path.
// Panics if at the root.
func (tp *TriePosition) PeekLastBit() bool {
	if tp.depth == 0 {
		panic("can't peek at root node")
	}
	return getBit(tp.path[:], int(tp.depth)-1)
}

// NodeIndex returns the index of the current node within a page.
func (tp *TriePosition) NodeIndex() int {
	return tp.nodeIndex
}

// DepthInPage returns the number of bits traversed in the current page.
// Every page has traversed at least 1 bit, therefore the return value is
// between 1 and Depth, with the exception of the root node (0 bits).
func (tp *TriePosition) DepthInPage() int {
	if tp.depth == 0 {
		return 0
	}
	return int(tp.depth) - ((int(tp.depth)-1)/Depth)*Depth
}

// ChildNodeIndices returns the child node indices within a page.
// Panics if the node is not at a depth in the range 1..=5
func (tp *TriePosition) ChildNodeIndices() *ChildNodeIndices {
	depth := tp.DepthInPage()
	if depth == 0 || depth > Depth-1 {
		panic(fmt.Sprintf("%d out of bounds 1..=%d", depth, Depth-1))
	}
	left := tp.nodeIndex*2 + 2
	return &ChildNodeIndices{left: left, depthInPage: depth}
}

// PageId returns the page ID this position lands in. Returns nil at the root.
func (tp *TriePosition) PageId() *PageId {
	if tp.IsRoot() {
		return nil
	}

	pageId := &RootPageId
	for i := 0; i < int(tp.depth)/Depth; i++ {
		if (i+1)*Depth == int(tp.depth) {
			return pageId
		}

		// Extract 6 bits for child index
		childIdx := extractBits(tp.path[:], i*Depth, Depth)
		var err error
		pageId, err = pageId.ChildPageId(childIdx)
		if err != nil {
			panic(err)
		}
	}

	return pageId
}

// ChildNodeIndices represents two child node indices within a page.
type ChildNodeIndices struct {
	left        int
	depthInPage int
}

// Left returns the index of the left child.
func (c *ChildNodeIndices) Left() int {
	return c.left
}

// Right returns the index of the right child.
func (c *ChildNodeIndices) Right() int {
	return c.left + 1
}

// InNextPage returns whether these children are in the next page.
// This is true when the parent is at the second-to-last depth (depth 5),
// meaning children are at the last depth (6) which represents leaf nodes
// that have their own child pages.
//
// Bug fix: Previously checked c.left >= NodesPerPage, but since ChildNodeIndices()
// only works for depths 1-5, c.left can never exceed 124 (< 126), so this always
// returned false. The correct check is whether the parent is at depth Depth-1 (5),
// because children at depth Depth (6) are at the page boundary and their children
// would be in child pages.
func (c *ChildNodeIndices) InNextPage() bool {
	// At depth 5, children are at depth 6 (the last/bottom layer of the page).
	// Nodes at depth 6 have their children in separate child pages.
	return c.depthInPage == Depth-1
}

// Helper functions

// lastPagePath extracts the relevant portion of the key path to the last page.
func lastPagePath(path []byte, depth uint16) []bool {
	if depth == 0 {
		panic("depth must be non-zero")
	}
	prevPageEnd := ((int(depth) - 1) / Depth) * Depth
	length := int(depth) - prevPageEnd

	bits := make([]bool, length)
	for i := 0; i < length; i++ {
		bits[i] = getBit(path, prevPageEnd+i)
	}
	return bits
}

// nodeIndexFromBits transforms a bit-path to an index in a page.
// The expected length is between 1 and Depth. A length of 0 returns 0.
func nodeIndexFromBits(pagePath []bool, depth int) int {
	if depth > Depth {
		depth = Depth
	}
	if depth == 0 {
		return 0
	}

	// Each node is stored at (2^depth - 2) + as_uint(path)
	base := (1 << depth) - 2
	pathValue := 0
	for i := 0; i < depth; i++ {
		if pagePath[i] {
			pathValue |= 1 << (depth - 1 - i)
		}
	}
	return base + pathValue
}

// siblingIndex returns the sibling index of a node.
func siblingIndex(nodeIndex int) int {
	if nodeIndex%2 == 0 {
		return nodeIndex + 1
	}
	return nodeIndex - 1
}

// parentNodeIndex returns the parent node index.
// Panics if nodeIndex is 0 or 1 (first two nodes in a page).
func parentNodeIndex(nodeIndex int) int {
	if nodeIndex < 2 {
		panic("can't get parent of first two nodes in page")
	}
	return (nodeIndex - 2) / 2
}

// Bit manipulation helpers

// getBit returns the bit at the given index in the byte slice.
func getBit(data []byte, bitIndex int) bool {
	byteIndex := bitIndex / 8
	bitOffset := 7 - (bitIndex % 8) // MSB first
	return (data[byteIndex] & (1 << bitOffset)) != 0
}

// setBit sets the bit at the given index in the byte slice.
func setBit(data []byte, bitIndex int, value bool) {
	byteIndex := bitIndex / 8
	bitOffset := 7 - (bitIndex % 8) // MSB first
	if value {
		data[byteIndex] |= 1 << bitOffset
	} else {
		data[byteIndex] &^= 1 << bitOffset
	}
}

// extractBits extracts n bits starting from startBit and returns as uint8.
func extractBits(data []byte, startBit, n int) uint8 {
	var result uint8
	for i := 0; i < n; i++ {
		if getBit(data, startBit+i) {
			result |= 1 << (n - 1 - i)
		}
	}
	return result
}

// String returns a string representation of the TriePosition.
func (tp *TriePosition) String() string {
	if tp.depth == 0 {
		return "TriePosition(root)"
	}
	// Build bit string
	bits := make([]byte, tp.depth)
	for i := uint16(0); i < tp.depth; i++ {
		if getBit(tp.path[:], int(i)) {
			bits[i] = '1'
		} else {
			bits[i] = '0'
		}
	}
	return fmt.Sprintf("TriePosition(%s)", string(bits))
}
