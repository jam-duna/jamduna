package trie

import (
	"errors"
	"fmt"
	"math"
	"github.com/ethereum/go-ethereum/common"
	//"golang.org/x/crypto/blake2b"
)

// CDMerkleTree represents the Merkle Tree structure
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
		leaves: padLeaves(leaves),
	}
	tree.root = tree.buildTree(tree.leaves)
	fmt.Printf("NewCDMerkleTree: root = %x\n", tree.root)
	return tree, nil
}

// padLeaves pads the leaves to the next power of two with EMPTYHASH
func padLeaves(leaves [][]byte) [][]byte {
	powerOfTwoSize := nextPowerOfTwo(len(leaves))
	paddedLeaves := make([][]byte, powerOfTwoSize)
	copy(paddedLeaves, leaves)
	for i := len(leaves); i < powerOfTwoSize; i++ {
		paddedLeaves[i] = EMPTYHASH
	}
	fmt.Printf("Input Len: %d -> Padded Len: %d\n", len(leaves), len(paddedLeaves))
	return paddedLeaves
}

// nextPowerOfTwo returns the next power of two greater than or equal to n
func nextPowerOfTwo(n int) int {
	if n <= 0 {
		return 1
	}
	power := 1
	for power < n {
		power <<= 1
	}
	return power
}

// buildTree constructs the Merkle Tree and updates the root hash
func (mt *CDMerkleTree) buildTree(leaves [][]byte) []byte {
	if len(leaves) == 1 {
		mt.root = leaves[0]
		fmt.Printf("leaves[0]=%x, update root=%x\n", leaves[0], mt.root)
		return mt.root
	}
	var parentLevel [][]byte
	for i := 0; i < len(leaves); i += 2 {
		var combined []byte
		if i+1 < len(leaves) {
			combined = append(leaves[i], leaves[i+1]...)
		} else {
			combined = append(leaves[i], leaves[i]...)
		}
		parentHash := computeHash(combined)
		//fmt.Printf("Combined: %x + %x = %x\n", leaves[i], leaves[i+1], parentHash)
		parentLevel = append(parentLevel, parentHash)
	}
	//fmt.Printf("Parent level: %x\n", parentLevel)
	return mt.buildTree(parentLevel)
}

// Root returns the root of the Merkle Tree
func (mt *CDMerkleTree) Root() common.Hash {
	return BytesToHash(mt.root)
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
			nextLevel = append(nextLevel, computeHash(combined))
		}
		currentLevel = nextLevel
	}
	return justification, nil
}

// JustifyX returns the justification for a given index and size x (function J_x)
func (mt *CDMerkleTree) JustifyX(index int, x int) ([][]byte, error) {
	if index < 0 || index >= len(mt.leaves) {
		return nil, errors.New("index out of range")
	}
	justification := make([][]byte, x)
	currentLevel := mt.leaves
	for d := 0; d < x; d++ {
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
			nextLevel = append(nextLevel, computeHash(combined))
		}
		currentLevel = nextLevel
	}
	return justification, nil
}

func verifyJustification(leafHash []byte, index int, justification [][]byte) []byte {
	currentHash := leafHash
	for _, siblingHash := range justification {
		if index%2 == 0 {
			currentHash = computeHash(append(currentHash, siblingHash...))
		} else {
			currentHash = computeHash(append(siblingHash, currentHash...))
		}
		index /= 2
	}
	return currentHash
}
