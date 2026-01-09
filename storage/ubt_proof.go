package storage

import (
	"errors"
	"sort"
)

var (
	ErrKeyNotFound  = errors.New("key not found")
	ErrInvalidProof = errors.New("invalid proof")
)

// Direction indicates which side the current hash occupies.
type Direction int

const (
	Left Direction = iota
	Right
)

// ProofNode represents one level in the proof path.
type ProofNode interface {
	isProofNode()
}

// InternalProofNode is a sibling hash on the stem trie path.
type InternalProofNode struct {
	Sibling   [32]byte
	Direction Direction
}

// StemProofNode contains siblings for the 8-level stem subtree.
type StemProofNode struct {
	Stem            Stem
	SubtreeSiblings [][32]byte // leaf->root order (8 entries)
}

// ExtensionProofNode represents a divergent stem in non-existence proofs.
type ExtensionProofNode struct {
	Stem            Stem
	StemHash        [32]byte
	DivergenceDepth uint16
}

func (*InternalProofNode) isProofNode()  {}
func (*StemProofNode) isProofNode()      {}
func (*ExtensionProofNode) isProofNode() {}

// Proof verifies a key-value pair in the tree.
type Proof struct {
	Key   TreeKey
	Value *[32]byte
	Path  []ProofNode // leaf->root order
}

// GenerateStemProof builds a proof for an existing key.
func GenerateStemProof(tree *UnifiedBinaryTree, key TreeKey) (*Proof, error) {
	value, found, err := tree.Get(&key)
	if err != nil {
		return nil, err
	}
	stemNode, ok := tree.getStemNode(key.Stem)
	if ok {
		subtreeSiblings := stemSubtreeSiblings(stemNode, key.Subindex, tree.hasher)
		path := make([]ProofNode, 0, 1+248)
		path = append(path, &StemProofNode{
			Stem:            key.Stem,
			SubtreeSiblings: subtreeSiblings,
		})

		stems := tree.sortedStems()
		internalNodes := tree.collectStemTrieSiblings(stems, key.Stem)
		path = append(path, internalNodes...)

		var valuePtr *[32]byte
		if found {
			val := value
			valuePtr = &val
		}
		return &Proof{Key: key, Value: valuePtr, Path: path}, nil
	}

	stems := tree.sortedStems()
	if len(stems) == 0 {
		return &Proof{Key: key, Value: nil, Path: nil}, nil
	}
	extStem, ok := closestStem(stems, key.Stem)
	if !ok {
		return nil, ErrKeyNotFound
	}
	diff := extStem.FirstDifferingBit(&key.Stem)
	if diff == nil {
		return nil, ErrInvalidProof
	}
	stemHash, ok := tree.getStemHash(extStem)
	if !ok {
		stemHash = tree.computeStemHash(extStem)
	}
	path := make([]ProofNode, 0, 1+248)
	path = append(path, &ExtensionProofNode{
		Stem:            extStem,
		StemHash:        stemHash,
		DivergenceDepth: uint16(*diff),
	})
	internalNodes := tree.collectStemTrieSiblings(stems, extStem)
	path = append(path, internalNodes...)
	return &Proof{Key: key, Value: nil, Path: path}, nil
}

// Verify recomputes the root and validates extension bindings.
func (p *Proof) Verify(hasher Hasher, expectedRoot [32]byte) error {
	var currentHash [32]byte
	if p.Value != nil {
		currentHash = hasher.Hash32(p.Value)
	}
	if len(p.Path) == 0 {
		if p.Value == nil && expectedRoot == ([32]byte{}) {
			return nil
		}
		return ErrInvalidProof
	}

	sawStem := false
	sawExtension := false
	internalCount := 0
	expectedDepth := 247
	var pathStem Stem
	hasPathStem := false
	for _, node := range p.Path {
		switch n := node.(type) {
		case *StemProofNode:
			if sawStem || sawExtension {
				return ErrInvalidProof
			}
			if p.Key.Stem != n.Stem {
				return ErrInvalidProof
			}
			if len(n.SubtreeSiblings) != 8 {
				return ErrInvalidProof
			}
			for i, sibling := range n.SubtreeSiblings {
				// Subtree siblings are ordered leaf->root (LSB->MSB).
				bit := (p.Key.Subindex >> i) & 1
				if bit == 0 {
					currentHash = hasher.Hash64(&currentHash, &sibling)
				} else {
					currentHash = hasher.Hash64(&sibling, &currentHash)
				}
			}
			currentHash = hasher.HashStemNode(&n.Stem, &currentHash)
			sawStem = true
			pathStem = n.Stem
			hasPathStem = true
			internalCount = 0
			expectedDepth = 247
		case *InternalProofNode:
			if !sawStem && !sawExtension {
				return ErrInvalidProof
			}
			if n.Direction != Left && n.Direction != Right {
				return ErrInvalidProof
			}
			if !hasPathStem {
				return ErrInvalidProof
			}
			if expectedDepth < 0 {
				return ErrInvalidProof
			}
			expectedDir := Left
			if pathStem.Bit(expectedDepth) == 1 {
				expectedDir = Right
			}
			if n.Direction != expectedDir {
				return ErrInvalidProof
			}
			if n.Direction == Left {
				currentHash = combineTrieHashes(hasher, currentHash, n.Sibling)
			} else {
				currentHash = combineTrieHashes(hasher, n.Sibling, currentHash)
			}
			internalCount++
			expectedDepth--
		case *ExtensionProofNode:
			if sawStem || sawExtension {
				return ErrInvalidProof
			}
			if p.Value != nil {
				return ErrInvalidProof
			}
			if err := verifyExtensionPrefix(p.Key.Stem, n); err != nil {
				return err
			}
			currentHash = n.StemHash
			sawExtension = true
			pathStem = n.Stem
			hasPathStem = true
			internalCount = 0
			expectedDepth = 247
		default:
			return ErrInvalidProof
		}
	}
	if !sawStem && !sawExtension {
		return ErrInvalidProof
	}
	if internalCount != 248 {
		return ErrInvalidProof
	}

	if currentHash != expectedRoot {
		return ErrInvalidProof
	}
	return nil
}

// collectStemTrieSiblings returns internal nodes in leaf->root order.
func (t *UnifiedBinaryTree) collectStemTrieSiblings(stems []Stem, target Stem) []ProofNode {
	proof := make([]ProofNode, 0, 248)
	for depth := 247; ; depth-- {
		bit := target.Bit(depth)
		prefix := CanonicalPrefix(target, uint16(depth))
		byteIndex := depth / 8
		bitIndex := uint8(7 - (depth % 8))

		siblingPrefix := prefix
		direction := Left
		if bit == 0 {
			siblingPrefix[byteIndex] |= 1 << bitIndex
			direction = Left
		} else {
			siblingPrefix[byteIndex] &^= 1 << bitIndex
			direction = Right
		}

		siblingStems := filterStemsByPrefix(stems, siblingPrefix, depth+1)
		var siblingHash [32]byte
		if len(siblingStems) != 0 {
			siblingHash = t.hashStemTrie(siblingStems, uint16(depth+1), siblingPrefix)
		}

		proof = append(proof, &InternalProofNode{Sibling: siblingHash, Direction: direction})
		if depth == 0 {
			break
		}
	}
	return proof
}

func filterStemsByPrefix(stems []Stem, prefix Stem, depthBits int) []Stem {
	filtered := make([]Stem, 0, len(stems))
	for _, stem := range stems {
		if stemHasPrefix(stem, prefix, depthBits) {
			filtered = append(filtered, stem)
		}
	}
	return filtered
}

func stemSubtreeSiblings(node *StemNode, subindex uint8, hasher Hasher) [][32]byte {
	siblings := make([][32]byte, 0, 8)
	for level := 0; level < 8; level++ {
		depth := 7 - level
		prefixLen := depth + 1
		prefix := subindexPrefix(subindex, prefixLen)
		bitPos := 7 - depth
		prefix ^= 1 << bitPos
		sibling := stemSubtreeHashForPrefix(node, hasher, prefix, prefixLen)
		siblings = append(siblings, sibling)
	}
	return siblings
}

func closestStem(stems []Stem, target Stem) (Stem, bool) {
	if len(stems) == 0 {
		return Stem{}, false
	}
	if len(stems) == 1 {
		return stems[0], true
	}
	idx := sort.Search(len(stems), func(i int) bool {
		return !stemLess(stems[i], target)
	})
	candidates := make([]Stem, 0, 2)
	if idx < len(stems) {
		candidates = append(candidates, stems[idx])
	}
	if idx > 0 {
		candidates = append(candidates, stems[idx-1])
	}
	best := candidates[0]
	bestDepth := sharedPrefixLen(best, target)
	for _, stem := range candidates[1:] {
		depth := sharedPrefixLen(stem, target)
		if depth > bestDepth || (depth == bestDepth && stemLess(stem, best)) {
			best = stem
			bestDepth = depth
		}
	}
	return best, true
}

func sharedPrefixLen(a, b Stem) int {
	diff := a.FirstDifferingBit(&b)
	if diff == nil {
		return 248
	}
	return *diff
}

func subindexPrefix(subindex uint8, prefixLen int) uint8 {
	if prefixLen >= 8 {
		return subindex
	}
	mask := uint8(0xFF << (8 - prefixLen))
	return subindex & mask
}

func stemSubtreeHashForPrefix(node *StemNode, hasher Hasher, prefix uint8, prefixLen int) [32]byte {
	if prefixLen >= 8 {
		return node.leafHashAt(hasher, prefix)
	}
	bitPos := 7 - prefixLen
	leftPrefix := prefix &^ (1 << bitPos)
	rightPrefix := prefix | (1 << bitPos)
	left := stemSubtreeHashForPrefix(node, hasher, leftPrefix, prefixLen+1)
	right := stemSubtreeHashForPrefix(node, hasher, rightPrefix, prefixLen+1)
	return hasher.Hash64(&left, &right)
}

// Verify stems are strictly sorted by bytes.
func stemsSorted(stems []Stem) bool {
	for i := 1; i < len(stems); i++ {
		if !stemLess(stems[i-1], stems[i]) {
			return false
		}
	}
	return true
}

// Sort and deduplicate TreeKeys by (stem, subindex).
func uniqueSortedKeys(keys []TreeKey) []TreeKey {
	sorted := make([]TreeKey, len(keys))
	copy(sorted, keys)
	sort.Slice(sorted, func(i, j int) bool {
		if stemLess(sorted[i].Stem, sorted[j].Stem) {
			return true
		}
		if stemLess(sorted[j].Stem, sorted[i].Stem) {
			return false
		}
		return sorted[i].Subindex < sorted[j].Subindex
	})

	out := sorted[:0]
	for i, key := range sorted {
		if i == 0 {
			out = append(out, key)
			continue
		}
		prev := out[len(out)-1]
		if prev.Stem != key.Stem || prev.Subindex != key.Subindex {
			out = append(out, key)
		}
	}
	return out
}

// verifyExtensionPrefix validates divergence binding for extension proofs.
func verifyExtensionPrefix(queryStem Stem, n *ExtensionProofNode) error {
	depth := n.DivergenceDepth
	if depth >= 248 {
		return ErrInvalidProof
	}
	for i := uint16(0); i < depth; i++ {
		if queryStem.Bit(int(i)) != n.Stem.Bit(int(i)) {
			return ErrInvalidProof
		}
	}
	if queryStem.Bit(int(depth)) == n.Stem.Bit(int(depth)) {
		return ErrInvalidProof
	}
	return nil
}
