package trie

import (
	//"crypto"
	//"encoding/hex"
	"errors"
	//"golang.org/x/crypto/blake2b"
	"hash"
)

type BalancedMerkleTree struct {
	Root *Node
	Hash hash.Hash
}

func NewBalancedMerkleTree(data [][]byte) *BalancedMerkleTree {
	if len(data) == 0 {
		return &BalancedMerkleTree{nil, nil}
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

	return &BalancedMerkleTree{Root: nodes[0], Hash: createHash()}
}

func (t *BalancedMerkleTree) MB(data [][]byte) []byte {
	if len(data) == 0 {
		return nil
	}
	if len(data) == 1 {
		return hashData(data[0])
	}
	return hashDataNodes(data)
}

func hashDataNodes(data [][]byte) []byte {
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

	return nodes[0].Hash
}

func (t *BalancedMerkleTree) Trace(index int, data [][]byte) ([][]byte, error) {
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

func (t *BalancedMerkleTree) trace(node *Node, index, total int, tracePath *[][]byte) {
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
