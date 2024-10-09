package trie

import (
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
	paddedLeaves := values
	if len(values) != 1 {
		paddedLeaves = padLeaves(values)
	}

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
		fmt.Printf("%s[Leaf %s]: %s\n", prefix, pos, common.Bytes2Hex(node.Hash))
	} else {
		fmt.Printf("%s[Branch %s]: %s\n", prefix, pos, common.Bytes2Hex(node.Hash))
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

func (mt *CDMerkleTree) Length() int {
	return len(mt.leaves)
}

func (mt *CDMerkleTree) IsOutOfRange(index int) (bool, error) {
	if index < 0 || index >= mt.Length() {
		return true, fmt.Errorf("index %v out of range", index)
	}
	return false, nil
}

// Get returns the value of the leaf at the given index
func (mt *CDMerkleTree) Get(index int) ([]byte, error) {
	if _, err := mt.IsOutOfRange(index); err != nil {
		return nil, err
	}
	return mt.leaves[index].Value, nil
}

// Justify returns the justification for a given index
func (mt *CDMerkleTree) Justify(index int) ([][]byte, error) {
	if _, err := mt.IsOutOfRange(index); err != nil {
		return nil, err
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
	if _, err := mt.IsOutOfRange(index); err != nil {
		return nil, err
	}
	justification := make([][]byte, 0)
	if len(mt.leaves) == 1 {
		return append(justification, mt.Root()), nil
	}
	currentNode := mt.leaves[index]
	var parent *CDTNode
	for currentNode != mt.root && x > 0 {
		parent = findParent(mt.root, currentNode)
		sibling := findSibling(parent, currentNode)
		if sibling != nil {
			justification = append(justification, sibling.Hash)
		} else {
			justification = append(justification, make([]byte, 32))
		}
		currentNode = parent
		x--
	}
	justification = append(justification, parent.Hash)
	return justification, nil
}

func VerifyJustification(leafHash []byte, index int, justification [][]byte) []byte {
	return verifyJustification(leafHash, index, justification)
}

// verifyJustification verifies the justification for a given index
func verifyJustification(leafHash []byte, index int, justification [][]byte) []byte {
	if len(justification) == 1 {
		return leafHash
	}
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
	for i := 0; i < x && i < len(justification)-1; i++ {
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
func GeneratePageProof(segments [][]byte) ([][]byte, error) {
	return generatePageProof(segments)
}

/* generatePageProof creates paged proofs from segments
 */
func generatePageProof(segments [][]byte) ([][]byte, error) {
	// Build the Merkle tree
	tree := NewCDMerkleTree(segments)
	// Count the number of pages
	pageSize := 64
	numPages := (len(segments) + pageSize - 1) / pageSize // ceiling function
	results := make([][]byte, 0)
	for page := 0; page < numPages; page++ {
		// The start and end index of the current page
		start := page * 64
		end := start + 64
		if end > len(segments) {
			end = len(segments)
		}

		// Calculate the index of the leaf node
		i := page * 64

		// Get the justification for the leaf node
		tracePath, err := tree.JustifyX(i, 6)
		if err != nil {
			return results, err
		}

		// Encode the trace path and the segments
		combinedData := append(tracePath, segments[start:end]...)
		encoded, err := types.Encode(combinedData)

		if err != nil {
			return results, err
		}

		// Pad the encoded data to W_C * W_S
		// paddingSize := types.W_C * types.W_S
		// paddedOutput := make([]byte, paddingSize)
		// copy(paddedOutput, encoded) // If the encoded data is larger than the padding size, it will be truncated
		results = append(results, encoded)
	}

	return results, nil
}

// splitPageProof splits the page proof into trace path, root
func splitPageProof(pageProof [][]byte) ([][]byte, []byte, [][]byte) {
	switch {
	case len(pageProof) == 2:
		return pageProof[0:1], pageProof[0], pageProof[1:]
	case len(pageProof) == 4:
		return pageProof[0:2], pageProof[1], pageProof[2:]
	case len(pageProof) >= 6 && len(pageProof) <= 7:
		return pageProof[0:3], pageProof[2], pageProof[3:]
	case len(pageProof) >= 9 && len(pageProof) <= 12:
		return pageProof[0:4], pageProof[3], pageProof[4:]
	case len(pageProof) >= 14 && len(pageProof) <= 21:
		return pageProof[0:5], pageProof[4], pageProof[5:]
	case len(pageProof) >= 23 && len(pageProof) <= 38:
		return pageProof[0:6], pageProof[5], pageProof[6:]
	default:
		return pageProof[0:7], pageProof[6], pageProof[7:]
	}
}

func VerifyPageProof(pageProof [][]byte, pageIndex int) (bool, error) {
	if pageIndex >= 1 {
		paddingSize := 71 - len(pageProof)
		if len(pageProof) != 71 {
			for i := 0; i < paddingSize; i++ {
				pageProof = append(pageProof, make([]byte, 32))
			}
		}
	}
	tracePath, treeRoot, segments := splitPageProof(pageProof)
	// fmt.Printf("Trace path: %x\n", tracePath)
	// fmt.Printf("Tree root: %x\n", treeRoot)
	// fmt.Printf("Segments: %x\n", segments)
	// Verify the root hash
	tree := NewCDMerkleTree(segments)
	// tree.PrintTree()
	if !compareBytes(tree.Root(), treeRoot) {
		fmt.Printf("Root hash mismatch, expected %x, got %x\n", tree.Root(), treeRoot)
		return false, errors.New("Root hash mismatch")
	}
	// Verify the justification for the leaf
	leafHash := computeLeaf(segments[0])
	computedRoot := VerifyJustifyX(leafHash, 0, tracePath, 6)
	expectedRoot := treeRoot

	if !compareBytes(computedRoot, expectedRoot) {
		fmt.Printf("Root hash mismatch: expected %x, got %x", expectedRoot, computedRoot)
		return false, errors.New("Root hash mismatch")
	} else {
		fmt.Printf("Root hash verified: %x\n", computedRoot)
	}
	return true, nil
}

func FindPositions(nums [][]byte, target []byte) int {
	return findPositions(nums, target)
}

func findPositions(nums [][]byte, target []byte) int {
	positions := -1
	for i, num := range nums {
		if compareBytes(num, target) {
			return i
		}
	}
	return positions
}
