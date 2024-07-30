package trie

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
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
	root   *WBTNode
	leaves []*WBTNode
}

// buildWellBalancedTree constructs a well-balanced binary tree from the given leaves
func (wbt *WellBalancedTree) buildWellBalancedTree() {
	if len(wbt.leaves) == 0 {
		// If no leaves, return hash of 0
		wbt.root = &WBTNode{Hash: computeHash([]byte{0})}
		return
	}
	wbt.root = buildTreeRecursive(wbt.leaves)
}

func (tree *WellBalancedTree) Root() []byte {
	return tree.root.Hash
}

// buildTreeRecursive recursively constructs the tree and returns the root node
func buildTreeRecursive(nodes []*WBTNode) *WBTNode {
	if len(nodes) == 1 {
		return nodes[0]
	}

	mid := int(math.Round(float64(len(nodes)) / 2))
	left := buildTreeRecursive(nodes[:mid])
	right := buildTreeRecursive(nodes[mid:])

	combinedValue := append(left.Hash, right.Hash...)
	hash := computeNode(combinedValue)

	return &WBTNode{
		Hash:  hash,
		Left:  left,
		Right: right,
	}
}

// NewWellBalancedTree creates a new well-balanced tree with the given values
func NewWellBalancedTree(values [][]byte) *WellBalancedTree {
	leaves := make([]*WBTNode, len(values))
	for i, value := range values {
		leaves[i] = &WBTNode{
			Hash:  computeNode(value),
			Value: value,
		}
	}
	wbt := &WellBalancedTree{leaves: leaves}
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
		fmt.Printf("%s[Leaf %s]: %s\n", prefix, pos, hex.EncodeToString(node.Hash))
	} else {
		fmt.Printf("%s[Branch %s]: %s\n", prefix, pos, hex.EncodeToString(node.Hash))
	}
	printNode(node.Left, level+1, "Left")
	printNode(node.Right, level+1, "Right")
}

// Trace returns the proof path for a given value
func (tree *WellBalancedTree) Trace(value []byte) ([][2][]byte, bool) {
	path := [][2][]byte{}
	found := tracePath(tree.root, computeNode(value), &path)
	return path, found
}

// tracePath recursively finds the path to the given hash
func tracePath(node *WBTNode, hash []byte, path *[][2][]byte) bool {
	if node == nil {
		return false
	}
	if string(node.Hash) == string(hash) {
		return true
	}
	if tracePath(node.Left, hash, path) {
		*path = append(*path, [2][]byte{node.Right.Hash, []byte("Right")})
		return true
	}
	if tracePath(node.Right, hash, path) {
		*path = append(*path, [2][]byte{node.Left.Hash, []byte("Left")})
		return true
	}
	return false
}

// Verify verifies the proof path for a given value and root hash
func Verify(rootHash, value []byte, path [][2][]byte) bool {
	hash := computeNode(value)
	for _, sibling := range path {
		siblingHash := sibling[0]
		direction := sibling[1]
		if string(direction) == "Left" {
			fmt.Printf("computeNode(%x, %x)): %x \n", hash, siblingHash, computeNode(append(siblingHash, hash...)))
			hash = computeNode(append(siblingHash, hash...))
		} else {
			fmt.Printf("computeNode(%x, %x)): %x \n", siblingHash, hash, computeNode(append(hash, siblingHash...)))
			hash = computeNode(append(hash, siblingHash...))
		}
	}
	return compareBytes(hash, rootHash)
}

// TraceByIndex returns the proof path for a given index
func (tree *WellBalancedTree) TraceByIndex(index int) ([][2][]byte, bool, error) {
	if index < 0 || index >= len(tree.leaves) {
		return nil, false, errors.New("index out of leaf range")
	}
	path := [][2][]byte{}
	found := tracePathByIndex(tree.root, index, &path, 0, len(tree.leaves)-1)
	return path, found, nil
}

// tracePathByIndex recursively finds the path to the given index
func tracePathByIndex(node *WBTNode, index int, path *[][2][]byte, start, end int) bool {
	if node == nil {
		return false
	}
	if start == end {
		return true
	}
	mid := (start + end) / 2
	if index <= mid {
		if tracePathByIndex(node.Left, index, path, start, mid) {
			*path = append(*path, [2][]byte{node.Right.Hash, []byte("Right")})
			return true
		}
	} else {
		if tracePathByIndex(node.Right, index, path, mid+1, end) {
			*path = append(*path, [2][]byte{node.Left.Hash, []byte("Left")})
			return true
		}
	}
	return false
}

// VerifyByIndex verifies the proof path for a given index and root hash
func VerifyByIndex(rootHash []byte, leafHash []byte, path [][2][]byte) bool {
	hash := leafHash
	for _, sibling := range path {
		siblingHash := sibling[0]
		direction := sibling[1]
		if string(direction) == "Left" {
			hash = computeNode(append(siblingHash, hash...))
		} else {
			hash = computeNode(append(hash, siblingHash...))
		}
	}
	return compareBytes(hash, rootHash)
}
