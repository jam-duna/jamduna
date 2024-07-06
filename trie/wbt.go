package trie

import (
	"errors"
	"fmt"
)

// WBTNode represents a node in the Well Balanced Tree
type WBTNode struct {
	Hash  []byte
	Data  []byte
	Left  *WBTNode
	Right *WBTNode
}

// WellBalancedTree represents the entire Well Balanced Tree
type WellBalancedTree struct {
	Root *WBTNode
}

// NewWellBalancedTree creates a new Well Balanced Tree
func NewWellBalancedTree() *WellBalancedTree {
	return &WellBalancedTree{Root: nil}
}

// Insert inserts a new leaf into the Well Balanced Tree
func (t *WellBalancedTree) Insert(data []byte) {
	newWBTNode := &WBTNode{Hash: computeHash(data), Data: data}

	if t.Root == nil {
		fmt.Printf("Inserting root node: %x\n", newWBTNode.Hash)
		t.Root = newWBTNode
	} else {
		fmt.Printf("Inserting new node: %x\n", newWBTNode.Hash)
		t.Root = t.insertWBTNode(t.Root, newWBTNode)
	}
}

// insertWBTNode inserts a new WBTNode into the tree and rebalances it
func (t *WellBalancedTree) insertWBTNode(root, newWBTNode *WBTNode) *WBTNode {
	if root == nil {
		return newWBTNode
	}

	if root.Left == nil {
		root.Left = newWBTNode
		fmt.Printf("Inserted at left of node: %x\n", root.Hash)
	} else if root.Right == nil {
		root.Right = newWBTNode
		fmt.Printf("Inserted at right of node: %x\n", root.Hash)
	} else {
		leftHeight := t.height(root.Left)
		rightHeight := t.height(root.Right)

		if leftHeight <= rightHeight {
			fmt.Printf("Inserting into left subtree of node: %x\n", root.Hash)
			root.Left = t.insertWBTNode(root.Left, newWBTNode)
		} else {
			fmt.Printf("Inserting into right subtree of node: %x\n", root.Hash)
			root.Right = t.insertWBTNode(root.Right, newWBTNode)
		}
	}

	leftHash := []byte{}
	if root.Left != nil {
		leftHash = root.Left.Hash
	}

	rightHash := []byte{}
	if root.Right != nil {
		rightHash = root.Right.Hash
	}

	root.Hash = computeHash(append(leftHash, rightHash...))
	fmt.Printf("Updated node hash: %x\n", root.Hash)

	return root
}

// height returns the height of the tree rooted at the given WBTNode
func (t *WellBalancedTree) height(node *WBTNode) int {
	if node == nil {
		return 0
	}
	leftHeight := t.height(node.Left)
	rightHeight := t.height(node.Right)

	if leftHeight > rightHeight {
		return leftHeight + 1
	}
	return rightHeight + 1
}

// RootHash returns the root hash of the Well Balanced Tree
func (t *WellBalancedTree) RootHash() []byte {
	if t.Root == nil {
		return make([]byte, 32)
	}
	return t.Root.Hash
}

// Trace provides the trace path for a given hash
func (t *WellBalancedTree) Trace(hash []byte) ([][]byte, error) {
	var tracePath [][]byte
	found, err := t.trace(t.Root, hash, &tracePath)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errors.New("hash not found")
	}
	return tracePath, nil
}

// trace traces the path to the given hash
func (t *WellBalancedTree) trace(node *WBTNode, hash []byte, tracePath *[][]byte) (bool, error) {
	if node == nil {
		return false, nil
	}

	*tracePath = append(*tracePath, node.Hash)
	fmt.Printf("Added %x to tracePath\n", node.Hash)

	if compareBytes(node.Hash, hash) {
		fmt.Printf("Found hash %x in tracePath\n", node.Hash)
		return true, nil
	}

	if node.Left != nil {
		found, err := t.trace(node.Left, hash, tracePath)
		if found || err != nil {
			return found, err
		}
	}

	if node.Right != nil {
		return t.trace(node.Right, hash, tracePath)
	}

	return false, nil
}

// VerifyProof verifies the proof of a given hash
func (t *WellBalancedTree) VerifyProof(hash, data []byte, tracePath [][]byte) bool {
	currentHash := computeHash(data)
	fmt.Printf("Starting verification with initial hash: %x\n", currentHash)

	for i := 0; i < len(tracePath); i++ {
		currentHash = computeHash(append(tracePath[i], currentHash...))
		fmt.Printf("Intermediate hash at step %d: %x\n", i, currentHash)
	}

	isValid := compareBytes(currentHash, t.RootHash())
	if isValid {
		fmt.Println("Proof verification succeeded")
	} else {
		fmt.Println("Proof verification failed")
	}

	return isValid
}
