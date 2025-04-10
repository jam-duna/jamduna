package trie

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

// WBTNode is a node in the well-balanced tree.
// Leaves can hold raw data (e.g. 64 bytes), and internal nodes store 32-byte hashes.
type WBTNode struct {
	Hash  []byte // NOTE: THIS IS NOT A 32-byte HASH ONLY, it can be a 64 byte leaf
	Value []byte
	Left  *WBTNode
	Right *WBTNode
}

type WellBalancedTree struct {
	root     *WBTNode
	leaves   []*WBTNode
	hashType string
}

// Build the tree from the leaves.
func (wbt *WellBalancedTree) buildWellBalancedTree() {
	if len(wbt.leaves) == 0 {
		var zero common.Hash
		wbt.root = &WBTNode{Hash: zero[:]}
		return
	}
	wbt.root = buildTreeRecursive(wbt.leaves, wbt.hashType)
}

// Return the root in raw bytes (32 bytes).
func (tree *WellBalancedTree) Root() []byte {
	return tree.root.Hash
}

// Return the root as common.Hash (32 bytes).
func (tree *WellBalancedTree) RootHash() common.Hash {
	return common.BytesToHash(tree.root.Hash)
}

// Recursively merge children with ceil-splitting.
func buildTreeRecursive(nodes []*WBTNode, hashType string) *WBTNode {
	if len(nodes) == 1 {
		return nodes[0]
	}
	mid := int(math.Ceil(float64(len(nodes)) / 2))
	left := buildTreeRecursive(nodes[:mid], hashType)
	right := buildTreeRecursive(nodes[mid:], hashType)
	combined := append(left.Hash, right.Hash...)
	parentHash := computeNode(combined, hashType)
	return &WBTNode{Hash: parentHash, Left: left, Right: right}
}

// Build a tree from raw leaves (possibly 64 bytes). Single leaf is hashed to 32 bytes.
func NewWellBalancedTree(values [][]byte, hashTypes ...string) *WellBalancedTree {
	leaves := make([]*WBTNode, len(values))
	hashType := types.Blake2b
	if len(hashTypes) > 0 && hashTypes[0] == types.Keccak {
		hashType = types.Keccak
	}

	if len(values) == 1 {
		v := values[0]
		leaves[0] = &WBTNode{Hash: computeLeaf(v, hashType), Value: v}
	} else {
		for i, v := range values {
			leaves[i] = &WBTNode{Hash: v, Value: v}
		}
	}
	wbt := &WellBalancedTree{leaves: leaves, hashType: hashType}
	wbt.buildWellBalancedTree()
	return wbt
}

// Debug print.
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

// Get leaf data by index.
func (tree *WellBalancedTree) Get(index int) ([]byte, error) {
	if index < 0 || index >= len(tree.leaves) {
		return nil, errors.New("index out of range")
	}
	return tree.leaves[index].Value, nil
}

// Trace returns the proof path (leaf->root). Leaves might be 64 bytes, siblings can be 64 or 32.
func (tree *WellBalancedTree) Trace(index int) (int, []byte, [][]byte, bool, error) {
	treeLen := len(tree.leaves)
	if index < 0 || index >= treeLen {
		return treeLen, nil, nil, false, errors.New("index out of range")
	}
	var path [][]byte
	current := tree.leaves[index]
	leafHash := current.Hash

	for current != tree.root {
		parent := findWBTParent(tree.root, current)
		if parent == nil {
			break
		}
		sibling := findWBTSibling(parent, current)
		if sibling != nil {
			path = append(path, sibling.Hash)
		} else {
			path = append(path, make([]byte, 32))
		}
		current = parent
	}
	return treeLen, leafHash, path, true, nil
}

// Verify merges the leaf and siblings using directions from top->down, then reversing them bottom->up.
func VerifyWBT(treeLen int, index int, root common.Hash, leafHash []byte, path [][]byte) (common.Hash, bool, error) {
	if index < 0 || index >= treeLen {
		return common.Hash{}, false, errors.New("index out of range")
	}
	levels := len(path)
	dirs := computeDirectionsForIndex(index, treeLen, levels)
	reverseInts(dirs)

	current := leafHash
	for i, dir := range dirs {
		sib := path[i]
		if dir == 0 {
			combined := append(current, sib...)
			current = computeNode(combined)
		} else {
			combined := append(sib, current...)
			current = computeNode(combined)
		}
	}
	ok := bytes.Equal(root.Bytes(), current)
	return common.BytesToHash(current), ok, nil
}

// Parent & sibling.
func findWBTParent(root, node *WBTNode) *WBTNode {
	if root == nil || root == node {
		return nil
	}
	if root.Left == node || root.Right == node {
		return root
	}
	if p := findWBTParent(root.Left, node); p != nil {
		return p
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

// Directions from top->down using ceil-splitting.
func computeDirectionsForIndex(index, totalLeaves, levels int) []int {
	var dirs []int
	n := totalLeaves
	for i := 0; i < levels; i++ {
		if n <= 1 {
			break
		}
		leftCount := int(math.Ceil(float64(n) / 2))
		if index < leftCount {
			dirs = append(dirs, 0)
			n = leftCount
		} else {
			dirs = append(dirs, 1)
			index -= leftCount
			n -= leftCount
		}
	}
	return dirs
}

func reverseInts(a []int) {
	for i := 0; i < len(a)/2; i++ {
		j := len(a) - i - 1
		a[i], a[j] = a[j], a[i]
	}
}

func computeProofPath(targetIdx int, currentStart int, currentEnd int, leafSize int) (pathLength int, rawDataSizeSum int) {
	if currentStart == currentEnd {
		return 0, 0
	}

	if targetIdx < currentStart || targetIdx > currentEnd || currentStart > currentEnd {
		return 0, 0
	}
	n := currentEnd - currentStart + 1
	if n <= 1 {
		return 0, 0
	}

	midOffset := int(math.Ceil(float64(n) / 2.0))
	leftEnd := currentStart + midOffset - 1
	rightStart := currentStart + midOffset

	var currentRawSiblingSize int
	var nextStart, nextEnd int

	// differentiate leaf vs internal sibling
	hashSize := 32
	if targetIdx <= leftEnd {
		nextStart, nextEnd = currentStart, leftEnd
		if rightStart == currentEnd {
			currentRawSiblingSize = leafSize
		} else {

			currentRawSiblingSize = hashSize
		}
	} else {
		nextStart, nextEnd = rightStart, currentEnd
		if currentStart == leftEnd {
			currentRawSiblingSize = leafSize
		} else {
			currentRawSiblingSize = hashSize
		}
	}
	lengthFromDeeper, sizeSumFromDeeper := computeProofPath(targetIdx, nextStart, nextEnd, leafSize)
	pathLength = lengthFromDeeper + 1
	rawDataSizeSum = sizeSumFromDeeper + currentRawSiblingSize

	return pathLength, rawDataSizeSum
}

func ComputeEncodedProofSize(shardIdx int, numShards int, leafSize int) (proofSize int) {
	if numShards <= 0 || leafSize <= 0 || (shardIdx < 0 || shardIdx >= numShards) || numShards == 1 {
		return 0
	}
	totalPathLength, totalRawDataSizeSum := computeProofPath(shardIdx, 0, numShards-1, leafSize)
	proofSize = totalPathLength + totalRawDataSizeSum
	return proofSize
}
