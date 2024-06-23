package merkle

import (
	"errors"
	"golang.org/x/crypto/blake2b"
	"math"
)

// Hash function using Blake2b
func bhash(data []byte) []byte {
	hash := blake2b.Sum256(data)
	return hash[:]
}

// CDCDMerkleTree represents the Merkle Tree structure
type CDMerkleTree struct {
	depth  int
	root   []byte
	leaves [][]byte
}

// NewCDMerkleTree creates a new Merkle Tree with the given leaves
func NewCDMerkleTree(leaves [][]byte) (*CDMerkleTree, error) {
	if len(leaves) == 0 {
		return nil, errors.New("no leaves to construct the Merkle Tree")
	}
	tree := &CDMerkleTree{
		depth:  int(math.Ceil(math.Log2(float64(len(leaves))))),
		leaves: leaves,
	}
	tree.root = tree.buildTree(leaves)
	return tree, nil
}

// buildTree constructs the Merkle Tree and returns the root hash
func (mt *CDMerkleTree) buildTree(leaves [][]byte) []byte {
	if len(leaves) == 1 {
		return bhash(leaves[0])
	}
	var parentLevel [][]byte
	for i := 0; i < len(leaves); i += 2 {
		var combined []byte
		if i+1 < len(leaves) {
			combined = append(leaves[i], leaves[i+1]...)
		} else {
			combined = append(leaves[i], leaves[i]...)
		}
		parentLevel = append(parentLevel, bhash(combined))
	}
	return mt.buildTree(parentLevel)
}

// Root returns the root of the Merkle Tree
func (mt *CDMerkleTree) Root() []byte {
	return mt.root
}

// Justify returns the justification for a given index
func (mt *CDMerkleTree) Justify(index int) ([][]byte, error) {
	if index < 0 || index >= len(mt.leaves) {
		return nil, errors.New("index out of range")
	}
	justification := make([][]byte, mt.depth)
	currentLevel := mt.leaves
	for d := 0; d < mt.depth; d++ {
		var siblingIndex int
		if index%2 == 0 {
			siblingIndex = index + 1
		} else {
			siblingIndex = index - 1
		}
		if siblingIndex < len(currentLevel) {
			justification[d] = currentLevel[siblingIndex]
		} else {
			justification[d] = currentLevel[index]
		}
		index /= 2
		var nextLevel [][]byte
		for i := 0; i < len(currentLevel); i += 2 {
			var combined []byte
			if i+1 < len(currentLevel) {
				combined = append(currentLevel[i], currentLevel[i+1]...)
			} else {
				combined = append(currentLevel[i], currentLevel[i]...)
			}
			nextLevel = append(nextLevel, bhash(combined))
		}
		currentLevel = nextLevel
	}
	return justification, nil
}
