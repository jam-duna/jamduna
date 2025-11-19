package core

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
)

// PageId is a unique identifier for a Page in a tree of pages with branching factor 2^6
// and a maximum depth of 42, with the root page counted as depth 0.
//
// Each PageId consists of a list of numbers between 0 and 2^6 - 1 (0-63),
// which encodes a path through the tree. The list may have between 0 and 42 (inclusive) items.
//
// Page IDs also have a disambiguated 256-bit representation which is given by starting with
// a blank bit pattern, and then repeatedly shifting it to the left by 6 bits, then adding the
// next child index, then adding 1.
//
// # Ordering
//
// Page IDs are ordered "depth-first" such that:
//   - An ID is always less than its child IDs.
//   - An ID's child IDs are ordered ascending by child index.
//   - An ID's child IDs are always less than any sibling IDs to the right of the ID.
type PageId struct {
	path []uint8 // Each element is 0-63 (6 bits)
}

const (
	// MaxPageDepth is the maximum depth of the page tree.
	MaxPageDepth = 42
	// MaxChildIndex is the maximum child index (2^6 - 1 = 63).
	MaxChildIndex = (1 << Depth) - 1
)

var (
	// RootPageId is the root page containing the sub-trie directly descending from the root node.
	RootPageId = PageId{path: []uint8{}}

	// ErrInvalidPageIdBytes is returned when decoding invalid page ID bytes.
	ErrInvalidPageIdBytes = errors.New("invalid page ID bytes")
	// ErrPageIdOverflow is returned when creating a child would exceed max depth.
	ErrPageIdOverflow = errors.New("page ID overflow: depth exceeds maximum")
	// ErrInvalidChildIndex is returned when child index is out of range.
	ErrInvalidChildIndex = errors.New("invalid child index: must be 0-63")
)

// Highest valid encoded page ID at layer 42 (used for validation)
var highestEncoded42 = func() *big.Int {
	// This is the hex representation: 0x104104104104104104104104104104104104104104104104104040
	bytes := []byte{
		16, 65, 4, 16, 65, 4, 16, 65, 4, 16, 65, 4, 16, 65, 4, 16, 65, 4, 16, 65,
		4, 16, 65, 4, 16, 65, 4, 16, 65, 4, 16, 64,
	}
	return new(big.Int).SetBytes(bytes)
}()

// NewPageId creates a new PageId from a path.
func NewPageId(path []uint8) (*PageId, error) {
	if len(path) > MaxPageDepth {
		return nil, ErrPageIdOverflow
	}
	for _, idx := range path {
		if idx > MaxChildIndex {
			return nil, ErrInvalidChildIndex
		}
	}
	return &PageId{path: path}, nil
}

// Decode decodes a page ID from its disambiguated 256-bit representation.
func DecodePageId(encoded [32]byte) (*PageId, error) {
	uint := new(big.Int).SetBytes(encoded[:])

	if uint.Cmp(highestEncoded42) > 0 {
		return nil, ErrInvalidPageIdBytes
	}

	leadingZeros := 256 - uint.BitLen()
	bitCount := 256 - leadingZeros
	sextets := (bitCount + 5) / 6

	if bitCount == 0 {
		return &RootPageId, nil
	}

	// Iterate sextets from least significant to most significant,
	// subtracting out 1 from each sextet
	path := make([]uint8, 0, sextets)
	one := big.NewInt(1)
	mask := big.NewInt(0b111111)

	for i := 0; i < sextets-1; i++ {
		uint.Sub(uint, one)
		x := new(big.Int).And(uint, mask)
		path = append(path, uint8(x.Uint64()))
		uint.Rsh(uint, Depth)
	}

	// Handle the last sextet
	if uint.Sign() != 0 {
		uint.Sub(uint, one)
		path = append(path, uint8(uint.Uint64()))
	}

	// Reverse the path since we built it from LSB to MSB
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}

	return &PageId{path: path}, nil
}

// Encode encodes this page ID to its disambiguated (fixed-width) 256-bit representation.
func (p *PageId) Encode() [32]byte {
	var result [32]byte

	if len(p.path) < 10 {
		// Optimization for small depths: use uint64
		// 7 + 6*9 = 61 - max bit-width of a page with depth 9
		var word uint64
		for _, limb := range p.path {
			word <<= 6
			word += uint64(limb + 1)
		}
		// Convert to big-endian bytes
		for i := 0; i < 8; i++ {
			result[24+i] = byte(word >> (56 - i*8))
		}
	} else {
		// Use big.Int for larger depths
		uint := big.NewInt(0)
		for _, limb := range p.path {
			uint.Lsh(uint, Depth)
			uint.Add(uint, big.NewInt(int64(limb+1)))
		}
		// Convert to big-endian bytes
		uintBytes := uint.Bytes()
		copy(result[32-len(uintBytes):], uintBytes)
	}

	return result
}

// Depth returns the depth of the page ID. The depth of the RootPageId is 0.
func (p *PageId) Depth() int {
	return len(p.path)
}

// ChildPageId constructs the child PageId given the child index.
//
// Child index must be a 6-bit integer (0-63).
// The PageId must be located at a depth below 42.
func (p *PageId) ChildPageId(childIndex uint8) (*PageId, error) {
	if childIndex > MaxChildIndex {
		return nil, ErrInvalidChildIndex
	}
	if len(p.path) >= MaxPageDepth {
		return nil, ErrPageIdOverflow
	}

	newPath := make([]uint8, len(p.path)+1)
	copy(newPath, p.path)
	newPath[len(p.path)] = childIndex

	return &PageId{path: newPath}, nil
}

// ParentPageId extracts the parent PageId.
// If this is the root page, returns the root page itself.
func (p *PageId) ParentPageId() *PageId {
	if len(p.path) == 0 {
		return &RootPageId
	}

	newPath := make([]uint8, len(p.path)-1)
	copy(newPath, p.path)

	return &PageId{path: newPath}
}

// ChildIndexAtLevel returns the child index at the given depth level.
// Panics if the depth of the page is not at least depth + 1.
func (p *PageId) ChildIndexAtLevel(depth int) uint8 {
	if depth >= len(p.path) {
		panic(fmt.Sprintf("depth %d out of range for page depth %d", depth, len(p.path)))
	}
	return p.path[depth]
}

// IsDescendantOf returns whether this page is a descendant of the other.
func (p *PageId) IsDescendantOf(other *PageId) bool {
	if len(p.path) < len(other.path) {
		return false
	}
	for i := range other.path {
		if p.path[i] != other.path[i] {
			return false
		}
	}
	return true
}

// MaxDescendant returns the maximum descendant of this page
// (the rightmost descendant at maximum depth).
func (p *PageId) MaxDescendant() *PageId {
	newPath := make([]uint8, MaxPageDepth)
	copy(newPath, p.path)
	for i := len(p.path); i < MaxPageDepth; i++ {
		newPath[i] = MaxChildIndex
	}
	return &PageId{path: newPath}
}

// Equals returns true if two PageIds are equal.
func (p *PageId) Equals(other *PageId) bool {
	return bytes.Equal(p.path, other.path)
}

// Compare compares two PageIds for ordering (depth-first).
// Returns -1 if p < other, 0 if p == other, 1 if p > other.
func (p *PageId) Compare(other *PageId) int {
	minLen := len(p.path)
	if len(other.path) < minLen {
		minLen = len(other.path)
	}

	for i := 0; i < minLen; i++ {
		if p.path[i] < other.path[i] {
			return -1
		} else if p.path[i] > other.path[i] {
			return 1
		}
	}

	if len(p.path) < len(other.path) {
		return -1
	} else if len(p.path) > len(other.path) {
		return 1
	}
	return 0
}

// String returns a string representation of the PageId.
func (p *PageId) String() string {
	if len(p.path) == 0 {
		return "PageId(root)"
	}
	return fmt.Sprintf("PageId(%v)", p.path)
}
