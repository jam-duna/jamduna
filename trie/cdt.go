package trie

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

//"golang.org/x/crypto/blake2b"

// CDT Node structure represents a node in the CDT
type CDTNode struct {
	Hash  []byte
	Value []byte
	Left  *CDTNode
	Right *CDTNode
}

// CDMerkleTree represents the Merkle Tree structure
type CDMerkleTree struct {
	root   *CDTNode
	leaves []*CDTNode
}

// PagedProof represents a paged proof
type PagedProof struct {
	SegmentHashes [64]common.Hash
	MerkleRoot    common.Hash
}

// NewCDMerkleTree creates a new constant-depth Merkle tree
func NewCDMerkleTree(values [][]byte) *CDMerkleTree {
	// If there are no leaves, return a tree with a hash of 0
	emptyHash := make([]byte, 32)
	if len(values) == 0 {
		return &CDMerkleTree{
			root: &CDTNode{Hash: emptyHash},
		}
	}

	// Padding leaves to the next power of 2
	paddedLeaves := padLeaves(values)

	// Creating leaf nodes
	var leaves []*CDTNode
	for _, value := range paddedLeaves {
		if !compareBytes(value, emptyHash) {
			leaves = append(leaves, &CDTNode{Hash: computeLeaf(value), Value: value})
		} else {
			leaves = append(leaves, &CDTNode{Hash: emptyHash, Value: emptyHash})
		}
	}

	// Build the tree recursively
	root := buildTree(leaves)

	return &CDMerkleTree{
		root:   root,
		leaves: leaves,
	}
}

// padLeaves pads the leaves to the next power of 2
func padLeaves(values [][]byte) [][]byte {
	n := len(values)
	nextPowerOfTwo := 1
	for nextPowerOfTwo < n {
		nextPowerOfTwo <<= 1
	}

	// Padding with zero hashes
	for len(values) < nextPowerOfTwo {
		values = append(values, make([]byte, 32))
	}

	return values
}

// buildTree recursively builds the tree from the leaf nodes
func buildTree(leaves []*CDTNode) *CDTNode {
	// Base case: only one node, return it
	if len(leaves) == 1 {
		return leaves[0]
	}

	var nextLevel []*CDTNode

	// Combine nodes in pairs
	for i := 0; i < len(leaves); i += 2 {
		left := leaves[i]
		right := leaves[i+1]
		parent := &CDTNode{
			Hash:  computeNode(append(left.Hash, right.Hash...)),
			Left:  left,
			Right: right,
		}
		nextLevel = append(nextLevel, parent)
	}

	return buildTree(nextLevel)
}

// PrintTree prints the tree structure for debugging
func (tree *CDMerkleTree) PrintTree() {
	printCDTNode(tree.root, 0, "Root")
}

func printCDTNode(node *CDTNode, level int, pos string) {
	if node == nil {
		return
	}
	prefix := strings.Repeat("  ", level)
	if node.Left == nil && node.Right == nil {
		fmt.Printf("%s[Leaf %s]: %s\n", prefix, pos, hex.EncodeToString(node.Hash))
	} else {
		fmt.Printf("%s[Branch %s]: %s\n", prefix, pos, hex.EncodeToString(node.Hash))
	}
	printCDTNode(node.Left, level+1, "Left")
	printCDTNode(node.Right, level+1, "Right")
}

// // Root returns the root of the Merkle Tree
func (tree *CDMerkleTree) Root() []byte {
	return tree.root.Hash
}

func (tree *CDMerkleTree) RootHash() common.Hash {
	return common.BytesToHash(tree.Root())
}

// Get returns the value of the leaf at the given index
func (mt *CDMerkleTree) Get(index int) ([]byte, error) {
	if index < 0 || index >= len(mt.leaves) {
		return nil, errors.New("index out of leaf range")
	}
	return mt.leaves[index].Value, nil
}

// Justify returns the justification for a given index
func (mt *CDMerkleTree) Justify(index int) ([][]byte, error) {
	if index < 0 || index >= len(mt.leaves) {
		return nil, errors.New("index out of range")
	}
	justification := make([][]byte, 0)
	currentNode := mt.leaves[index]
	for currentNode != mt.root {
		parent := findParent(mt.root, currentNode)
		sibling := findSibling(parent, currentNode)
		if sibling != nil {
			justification = append(justification, sibling.Hash)
		} else {
			justification = append(justification, make([]byte, 32))
		}
		currentNode = parent
	}
	return justification, nil
}

// JustifyX returns the justification for a given index and size x (function J_x)
func (mt *CDMerkleTree) JustifyX(index int, x int) ([][]byte, error) {
	if index < 0 || index >= len(mt.leaves) {
		return nil, errors.New("index out of range")
	}
	justification := make([][]byte, 0)
	currentNode := mt.leaves[index]
	for currentNode != mt.root && x > 0 {
		parent := findParent(mt.root, currentNode)
		sibling := findSibling(parent, currentNode)
		if sibling != nil {
			justification = append(justification, sibling.Hash)
		} else {
			justification = append(justification, make([]byte, 32))
		}
		currentNode = parent
		x--
	}
	return justification, nil
}

func VerifyJustification(leafHash []byte, index int, justification [][]byte) []byte {
	return verifyJustification(leafHash, index, justification)
}

// verifyJustification verifies the justification for a given index
func verifyJustification(leafHash []byte, index int, justification [][]byte) []byte {
	currentHash := leafHash
	for _, siblingHash := range justification {
		if index%2 == 0 {
			currentHash = computeNode(append(currentHash, siblingHash...))
		} else {
			currentHash = computeNode(append(siblingHash, currentHash...))
		}
		index /= 2
	}
	return currentHash
}

func VerifyJustifyX(leafHash []byte, index int, justification [][]byte, x int) []byte {
	return verifyJustifyX(leafHash, index, justification, x)
}

// verifyJustifyX verifies the justification for a given index and size x
func verifyJustifyX(leafHash []byte, index int, justification [][]byte, x int) []byte {
	currentHash := leafHash
	for i := 0; i < x && i < len(justification); i++ {
		siblingHash := justification[i]
		if index%2 == 0 {
			currentHash = computeNode(append(currentHash, siblingHash...))
		} else {
			currentHash = computeNode(append(siblingHash, currentHash...))
		}
		index /= 2
	}
	return currentHash
}

func findParent(root, node *CDTNode) *CDTNode {
	if root == nil || root == node {
		return nil
	}
	if root.Left == node || root.Right == node {
		return root
	}
	parent := findParent(root.Left, node)
	if parent != nil {
		return parent
	}
	return findParent(root.Right, node)
}

func findSibling(parent, node *CDTNode) *CDTNode {
	if parent == nil {
		return nil
	}
	if parent.Left == node {
		return parent.Right
	}
	return parent.Left
}

// generatePageProof creates paged proofs from segments
func GeneratePageProof(segments []types.Segment) []PagedProof {
	return generatePageProof(segments)
}

// generatePageProof creates paged proofs from segments
func generatePageProof(segments []types.Segment) []PagedProof {
	var pagedProofs []PagedProof
	var currentPageSegments []common.Hash

	for _, segment := range segments {
		data := segment.Data

		// put the hash of the segment into the current page
		currentPageSegments = append(currentPageSegments, common.Hash(computeLeaf(data)))

		// if the current page is full, create a paged proof
		if len(currentPageSegments) == 64 {
			pagedProofs = append(pagedProofs, createPagedProof(currentPageSegments))
			currentPageSegments = []common.Hash{}
		}
	}

	// if there are remaining segments, create a paged proof
	if len(currentPageSegments) > 0 {
		for len(currentPageSegments) < 64 {
			// append empty hashes to fill the page
			currentPageSegments = append(currentPageSegments, common.Hash{})
		}
		pagedProofs = append(pagedProofs, createPagedProof(currentPageSegments))
	}

	return pagedProofs
}

// createPagedProof creates a paged proof
func createPagedProof(segmentHashes []common.Hash) PagedProof {
	var segmentHashesArray [64]common.Hash
	copy(segmentHashesArray[:], segmentHashes)

	merkleRoot := generateMerkleRoot(segmentHashes)

	return PagedProof{
		SegmentHashes: segmentHashesArray,
		MerkleRoot:    merkleRoot,
	}
}

// generates the Merkle root from the segment hashes
func generateMerkleRoot(hashes []common.Hash) common.Hash {
	// transform the hashes to byte slices
	var hashBytes [][]byte
	for _, hash := range hashes {
		hashBytes = append(hashBytes, hash[:])
	}

	// create a new CDMerkleTree by the hashes
	tree := NewCDMerkleTree(hashBytes)

	return common.BytesToHash(tree.Root())
}

// Trasnform PagedProof to bytes
func (pp PagedProof) ToBytes() []byte {
	var result []byte

	// Trsnform the segment hashes to bytes
	for _, hash := range pp.SegmentHashes {
		result = append(result, hash[:]...)
	}

	result = append(result, pp.MerkleRoot[:]...)

	return result
}
