package trie

import (
	"bytes"
	"testing"
)

// TestCDMerkleTree tests the Merkle Tree implementation
func TestCDMerkleTree(t *testing.T) {
	leaves := [][]byte{
		bhash([]byte("leaf1")),
		bhash([]byte("leaf2")),
		bhash([]byte("leaf3")),
		bhash([]byte("leaf4")),
	}
	tree, err := NewCDMerkleTree(leaves)
	if err != nil {
		t.Fatalf("failed to create Merkle Tree: %v", err)
	}
	root := tree.Root()
	expectedRoot := bhash(append(bhash(append(bhash([]byte("leaf1")), bhash([]byte("leaf2"))...)), bhash(append(bhash([]byte("leaf3")), bhash([]byte("leaf4"))...))...))
	if !bytes.Equal(root, expectedRoot) {
		t.Fatalf("unexpected root hash: got %x, want %x", root, expectedRoot)
	}
	justification, err := tree.Justify(2)
	if err != nil {
		t.Fatalf("failed to get justification: %v", err)
	}
	if len(justification) != tree.depth {
		t.Fatalf("unexpected justification length: got %d, want %d", len(justification), tree.depth)
	}
	// Add more specific checks for justification values as needed
}
