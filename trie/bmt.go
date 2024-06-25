package trie

import (
	"errors"
	"hash"

	"golang.org/x/crypto/blake2b"
)

// Node represents a node in the Merkle Tree
type Node struct {
	Hash  []byte
	Left  *Node
	Right *Node
}

// MerkleTree represents the entire Merkle Tree
type MerkleTree struct {
	Root *Node
}

// NewMerkleTree creates a new Merkle Tree from the provided data
func NewMerkleTree(data [][]byte) *MerkleTree {
	if len(data) == 0 {
		return &MerkleTree{nil}
	}

	var nodes []*Node
	for _, d := range data {
		nodes = append(nodes, &Node{Hash: hashData(d)})
	}

	for len(nodes) > 1 {
		var newLevel []*Node

		for i := 0; i < len(nodes); i += 2 {
			if i+1 < len(nodes) {
				newLevel = append(newLevel, &Node{
					Hash:  hashNodes(nodes[i], nodes[i+1]),
					Left:  nodes[i],
					Right: nodes[i+1],
				})
			} else {
				newLevel = append(newLevel, nodes[i])
			}
		}

		nodes = newLevel
	}

	return &MerkleTree{Root: nodes[0]}
}

func hashData(data []byte) []byte {
	h, _ := blake2b.New256(nil)
	h.Write(data)
	return h.Sum(nil)
}

func hashNodes(left, right *Node) []byte {
	h, _ := blake2b.New256(nil)
	h.Write([]byte("$node"))
	h.Write(left.Hash)
	h.Write(right.Hash)
	return h.Sum(nil)
}

func createHash() hash.Hash {
	h, _ := blake2b.New256(nil)
	return h
}

func (t *MerkleTree) Trace(index int, data [][]byte) ([][]byte, error) {
	if index < 0 || index >= len(data) {
		return nil, errors.New("index out of bounds")
	}
	if t.Root == nil {
		return nil, errors.New("empty tree")
	}

	var tracePath [][]byte
	t.trace(t.Root, index, len(data), &tracePath)
	return tracePath, nil
}

func (t *MerkleTree) trace(node *Node, index, total int, tracePath *[][]byte) {
	if node.Left == nil && node.Right == nil {
		return
	}

	mid := total / 2
	if index < mid {
		*tracePath = append(*tracePath, node.Right.Hash)
		t.trace(node.Left, index, mid, tracePath)
	} else {
		*tracePath = append(*tracePath, node.Left.Hash)
		t.trace(node.Right, index-mid, total-mid, tracePath)
	}
}
