package trie

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/colorfulnotion/jam/common"

	"github.com/colorfulnotion/jam/types"
)

// WBTNode represents a node in the WBT
type WBTNode struct {
	Hash  []byte
	Value []byte
	Left  *WBTNode
	Right *WBTNode
}

// WellBalancedTree represents the WBT structure
type WellBalancedTree struct {
	root     *WBTNode
	leaves   []*WBTNode
	hashType string
}

// Equation(E.3) in GP 0.6.2
// buildWellBalancedTree constructs a well-balanced binary tree from the given leaves
func (wbt *WellBalancedTree) buildWellBalancedTree() {
	if len(wbt.leaves) == 0 {
		// If no leaves, return hash of 0
		hash0 := common.Hash{}
		wbt.root = &WBTNode{Hash: hash0[:]}
		return
	}
	wbt.root = buildTreeRecursive(wbt.leaves, wbt.hashType)
}

func (tree *WellBalancedTree) Root() []byte {
	return tree.root.Hash
}

func (tree *WellBalancedTree) RootHash() common.Hash {
	return common.Hash(tree.root.Hash)
}

// buildTreeRecursive recursively constructs the tree and returns the root node
func buildTreeRecursive(nodes []*WBTNode, hashType string) *WBTNode {
	if len(nodes) == 1 {
		return nodes[0]
	}
	mid := int(math.Ceil(float64(len(nodes)) / 2))
	left := buildTreeRecursive(nodes[:mid], hashType)
	right := buildTreeRecursive(nodes[mid:], hashType)
	combinedValue := append(left.Hash, right.Hash...)
	hash := computeNode(combinedValue, hashType)
	return &WBTNode{
		Hash:  hash,
		Left:  left,
		Right: right,
	}
}

// NewWellBalancedTree creates a new well-balanced tree with the given values
func NewWellBalancedTree(values [][]byte, hashTypes ...string) *WellBalancedTree {
	leaves := make([]*WBTNode, len(values))
	hashType := types.Blake2b
	if len(hashTypes) > 0 && hashTypes[0] == types.Keccak {
		hashType = types.Keccak
	}
	if len(values) == 1 {
		// special case H(v0) https://graypaper.fluffylabs.dev/#/85129da/3a0a013a0a01?v=0.6.3
		value := values[0]
		leaves[0] = &WBTNode{
			Hash:  computeLeaf(value, hashType),
			Value: value,
		}
	} else {
		for i, value := range values {
			leaves[i] = &WBTNode{
				Hash:  value, // WAS: computeLeaf(value, hashType)
				Value: value,
			}
			// fmt.Printf("add leaf %d: %x\n", i, leaves[i].Value)
		}
	}
	wbt := &WellBalancedTree{leaves: leaves, hashType: hashType}
	wbt.buildWellBalancedTree()
	return wbt
}

// Get returns the value of the leaf at the given index
func (tree *WellBalancedTree) Get(index int) ([]byte, error) {
	if index < 0 || index >= len(tree.leaves) {
		return nil, errors.New("index out of leaf range")
	}
	return tree.leaves[index].Value, nil
}

// PrintTree prints the tree structure for debugging
func (tree *WellBalancedTree) PrintTree() {
	printNode(tree.root, 0, "Root")
}

func printNode(node *WBTNode, level int, pos string) {
	if node == nil {
		return
	}
	prefix := strings.Repeat("  ", level)
	if node.Left == nil && node.Right == nil {
		fmt.Printf("%s[Leaf %s]: %s\n", prefix, pos, common.Bytes2Hex(node.Hash))
	} else {
		fmt.Printf("%s[Branch %s]: %s\n", prefix, pos, common.Bytes2Hex(node.Hash))
	}
	printNode(node.Left, level+1, "Left")
	printNode(node.Right, level+1, "Right")
}

// Trace returns the proof path for a given value
func (tree *WellBalancedTree) Trace(index int) (int, common.Hash, []common.Hash, bool, error) {
	treeLen, leafHash, path, isFound, err := tree.trace(index)
	if err != nil {
		fmt.Printf("Get proof path error: %v\n", err)
		return treeLen, common.Hash{}, nil, false, err
	}
	return treeLen, leafHash, path, isFound, nil
}

func (tree *WellBalancedTree) trace(index int) (int, common.Hash, []common.Hash, bool, error) {
	treeLen := len(tree.leaves)
	if index < 0 || index >= len(tree.leaves) {
		return treeLen, common.Hash{}, nil, false, errors.New("index out of leaf range")
	}

	tracePath := make([]common.Hash, 0)
	currentNode := tree.leaves[index]
	leafHash := common.Hash(computeLeaf(currentNode.Value, tree.hashType))
	for currentNode != tree.root {
		parent := findWBTParent(tree.root, currentNode) // Find parent node
		sibling := findWBTSibling(parent, currentNode)  // Find sibling node

		if sibling != nil {
			tracePath = append(tracePath, common.BytesToHash(sibling.Hash)) // Add sibling hash
		} else {
			tracePath = append(tracePath, BytesToHash(make([]byte, 32))) // If sibling is nil, add empty hash
		}
		currentNode = parent
	}
	return treeLen, leafHash, tracePath, true, nil
}

func VerifyWBT(treeLen int, index int, erasureRoot common.Hash, leafHash common.Hash, tracePath []common.Hash, hashTypes ...string) (common.Hash, bool, error) {
	hashType := types.Blake2b
	if len(hashTypes) != 0 && hashTypes[0] == types.Keccak {
		hashType = types.Keccak
	}
	if index < 0 || index >= treeLen {
		return common.Hash{}, false, errors.New("index out of range")
	}
	start, end := 0, treeLen-1
	currentHash := leafHash

	// Compute the direction of the path
	direction := computeDirection(index, start, end, tracePath)
	direction = reverse(direction)

	// Compute the root hash
	for i, dir := range direction {
		siblingHash := tracePath[i]

		if dir == 0 {
			currentHash = common.BytesToHash(computeNode(append(currentHash[:], siblingHash[:]...), hashType))
		} else {
			currentHash = common.BytesToHash(computeNode(append(siblingHash[:], currentHash[:]...), hashType))
		}
	}
	isValid := erasureRoot.String() == currentHash.String()

	return currentHash, isValid, nil
}

func computeDirection(index, start, end int, tracePath []common.Hash) []int {
	direction := []int{}
	for i := 0; i < len(tracePath); i++ {
		mid := (start + end) / 2

		if index <= mid {
			end = mid
			direction = append(direction, 0)
		} else {
			start = mid + 1
			direction = append(direction, 1)
		}
	}
	return direction
}

func reverse(direction []int) []int {
	for i := 0; i < len(direction)/2; i++ {
		j := len(direction) - i - 1
		direction[i], direction[j] = direction[j], direction[i]
	}
	return direction
}

// findParent finds the parent of the given node
func findWBTParent(root, node *WBTNode) *WBTNode {
	if root == nil || root == node {
		return nil
	}
	if root.Left == node || root.Right == node {
		return root
	}

	if leftParent := findWBTParent(root.Left, node); leftParent != nil {
		return leftParent
	}
	return findWBTParent(root.Right, node)
}

func findWBTSibling(parent, node *WBTNode) *WBTNode {
	if parent == nil {
		return nil
	}
	if parent.Left == node {
		return parent.Right
	}
	return parent.Left
}

func ComputeExpectedWBTCopathSize(numLeaves int) int {
	if numLeaves <= 1 {
		// If there's only one leaf, the co-path size is 0 (no siblings).
		return 0
	}

	// Calculate the number of levels in the tree (log2 of the number of leaves, rounded up)
	return int(math.Ceil(math.Log2(float64(numLeaves))))
}
