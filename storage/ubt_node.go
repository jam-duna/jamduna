package storage

import "sort"

// NodeKind identifies the node slab for a NodeRef.
type NodeKind uint8

const (
	NodeEmpty NodeKind = iota
	NodeInternal
	NodeStem
)

// NodeRef points into typed slabs for each node kind.
type NodeRef struct {
	Kind  NodeKind
	Index uint32
}

// EmptyNodeRef is the root sentinel for an empty tree.
var EmptyNodeRef = NodeRef{Kind: NodeEmpty, Index: 0}

// InternalNode has left/right children.
type InternalNode struct {
	Left  NodeRef
	Right NodeRef
}

// StemNode roots a 256-value subtree.
// Values MUST be non-nil (use NewStemNode).
type StemNode struct {
	Stem   Stem
	Values map[uint8][32]byte

	// Incremental subtree cache (8 levels = 255 internal nodes).
	subtreeCache [255][32]byte
	cacheValid   bool
	cacheInit    bool // false until full subtree is built at least once
	dirtyLeaves  [4]uint64
}

func NewStemNode(stem Stem) *StemNode {
	return &StemNode{
		Stem:   stem,
		Values: make(map[uint8][32]byte),
	}
}

func (s *StemNode) SortedSubindices() []uint8 {
	indices := make([]uint8, 0, len(s.Values))
	for idx := range s.Values {
		indices = append(indices, idx)
	}
	sort.Slice(indices, func(i, j int) bool { return indices[i] < indices[j] })
	return indices
}

func (s *StemNode) SetValue(idx uint8, value [32]byte) {
	if value == ([32]byte{}) {
		delete(s.Values, idx)
	} else {
		s.Values[idx] = value
	}
	s.dirtyLeaves[idx/64] |= 1 << (idx % 64)
	s.cacheValid = false
}

func (s *StemNode) SetValueRaw(idx uint8, value [32]byte) {
	s.Values[idx] = value
	s.dirtyLeaves[idx/64] |= 1 << (idx % 64)
	s.cacheValid = false
}

func (s *StemNode) DeleteValue(idx uint8) {
	delete(s.Values, idx)
	s.dirtyLeaves[idx/64] |= 1 << (idx % 64)
	s.cacheValid = false
}

func (s *StemNode) Hash(h Hasher) [32]byte {
	if !s.cacheValid {
		if !s.cacheInit {
			s.recomputeFullSubtree(h)
			s.cacheInit = true
		} else {
			s.recomputeDirtySubtree(h)
		}
		s.cacheValid = true
	}
	subtreeRoot := s.subtreeCache[0]
	return h.HashStemNode(&s.Stem, &subtreeRoot)
}

// subtreeCache uses heap indexing for internal nodes only (255 nodes total).
// Level 0 (root) at index 0, level 7 at indices 127..254 (parents of leaves).
func (s *StemNode) recomputeDirtySubtree(h Hasher) {
	var dirtyNodes [255]bool
	for word := 0; word < len(s.dirtyLeaves); word++ {
		dirty := s.dirtyLeaves[word]
		if dirty == 0 {
			continue
		}
		for bit := 0; bit < 64; bit++ {
			if (dirty>>bit)&1 == 0 {
				continue
			}
			leaf := word*64 + bit
			if leaf >= 256 {
				continue
			}
			leafIdx := uint8(leaf)
			node := 127 + int(leafIdx>>1)
			for {
				dirtyNodes[node] = true
				if node == 0 {
					break
				}
				node = (node - 1) / 2
			}
		}
		s.dirtyLeaves[word] = 0
	}

	for node := 254; ; node-- {
		if !dirtyNodes[node] {
			if node == 0 {
				break
			}
			continue
		}
		if node >= 127 {
			idx := node - 127
			leftIdx := uint8(idx * 2)
			rightIdx := uint8(idx*2 + 1)
			left := s.leafHashAt(h, leftIdx)
			right := s.leafHashAt(h, rightIdx)
			s.subtreeCache[node] = h.Hash64(&left, &right)
		} else {
			leftChild := s.subtreeCache[node*2+1]
			rightChild := s.subtreeCache[node*2+2]
			s.subtreeCache[node] = h.Hash64(&leftChild, &rightChild)
		}
		if node == 0 {
			break
		}
	}
}

func (s *StemNode) recomputeFullSubtree(h Hasher) {
	for i := 0; i < 128; i++ {
		left := s.leafHashAt(h, uint8(i*2))
		right := s.leafHashAt(h, uint8(i*2+1))
		s.subtreeCache[127+i] = h.Hash64(&left, &right)
	}
	for node := 126; ; node-- {
		leftChild := s.subtreeCache[node*2+1]
		rightChild := s.subtreeCache[node*2+2]
		s.subtreeCache[node] = h.Hash64(&leftChild, &rightChild)
		if node == 0 {
			break
		}
	}
	for i := range s.dirtyLeaves {
		s.dirtyLeaves[i] = 0
	}
}

func (s *StemNode) leafHashAt(h Hasher, idx uint8) [32]byte {
	if val, ok := s.Values[idx]; ok {
		return h.Hash32(&val)
	}
	return [32]byte{}
}
