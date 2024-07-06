package trie

import (
	"bytes"
	"testing"
)

// TestCDMerkleTree tests the Merkle Tree implementation
func TestCDMerkleTree(t *testing.T) {
	leaves := [][]byte{
		computeHash([]byte("leaf1")),
		computeHash([]byte("leaf2")),
		computeHash([]byte("leaf3")),
	}

	tree, err := NewCDMerkleTree(leaves)
	if err != nil {
		t.Fatalf("failed to create Merkle Tree: %v", err)
	}

	// Calculate expected root hash
	H1 := computeHash([]byte("leaf1"))
	H2 := computeHash([]byte("leaf2"))
	H3 := computeHash([]byte("leaf3"))
	H4 := EMPTYHASH

	L1 := computeHash(append(H1, H2...))
	L2 := computeHash(append(H3, H4...))
	expectedRoot := computeHash(append(L1, L2...))

	// Log hashes for verification
	t.Logf("H1: %x\n", H1)
	t.Logf("H2: %x\n", H2)
	t.Logf("H3: %x\n", H3)
	t.Logf("H4: %x\n", H4)
	t.Logf("L1: %x\n", L1)
	t.Logf("L2: %x\n", L2)
	t.Logf("expectedRoot: %x\n", expectedRoot)
	t.Logf("tree.Root(): %x\n", tree.Root())

	if !bytes.Equal(tree.Root().Bytes(), expectedRoot) {
		t.Fatalf("unexpected root hash: got %x, want %x", tree.Root(), expectedRoot)
	}

	justification, err := tree.Justify(2)
	if err != nil {
		t.Fatalf("failed to get justification: %v", err)
	}
	if len(justification) != tree.depth {
		t.Fatalf("unexpected justification length: got %d, want %d", len(justification), tree.depth)
	}

	// Verify justification
	leafHash := leaves[2]
	recomputedRoot := verifyJustification(leafHash, 2, justification)
	if !bytes.Equal(recomputedRoot, expectedRoot) {
		t.Fatalf("justification verification failed: got %x, want %x", recomputedRoot, expectedRoot)
	}else{
		t.Logf("VERIFIED. leafHash=%x, justification=%x\n", leafHash, justification)
	}
}

func TestCDMerkleTreeWithPadding(t *testing.T) {
	leaves := [][]byte{
		computeHash([]byte("leaf1")),
		computeHash([]byte("leaf2")),
		computeHash([]byte("leaf3")),
	}

	tree, err := NewCDMerkleTree(leaves)
	if err != nil {
		t.Fatalf("failed to create Merkle Tree: %v", err)
	}

	powerOfTwoSize := 4 // Next power of two for 3 leaves
	expectedLeavesCount := powerOfTwoSize
	if len(tree.leaves) != expectedLeavesCount {
		t.Fatalf("unexpected number of leaves: got %d, want %d", len(tree.leaves), expectedLeavesCount)
	}

	root := tree.Root()
	if root == BytesToHash(EMPTYHASH) {
		t.Fatalf("unexpected nil root hash")
	}
}

func TestJustifyX(t *testing.T) {
	leaves := [][]byte{
		computeHash([]byte("leaf1")),
		computeHash([]byte("leaf2")),
		computeHash([]byte("leaf3")),
		computeHash([]byte("leaf4")),
	}

	tree, err := NewCDMerkleTree(leaves)
	if err != nil {
		t.Fatalf("failed to create Merkle Tree: %v", err)
	}

	justification, err := tree.JustifyX(2, 2)
	if err != nil {
		t.Fatalf("failed to get justification: %v", err)
	}
	if len(justification) != 2 {
		t.Fatalf("unexpected justification length: got %d, want %d", len(justification), 2)
	}
}
