package trie

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/colorfulnotion/jam/common"
)

func TestCDMerkleTree(t *testing.T) {
	leaves := [][]byte{
		[]byte("leaf1"),
		[]byte("leaf2"),
		[]byte("leaf3"),
		[]byte("leaf4"),
		[]byte("leaf5"),
	}

	// Create the Merkle tree
	tree := NewCDMerkleTree(leaves)

	// Verify the tree structure and hash values
	if tree.root == nil {
		t.Fatal("Expected root to be non-nil")
	} else {
		fmt.Printf("Root: %x\n", tree.root.Hash)
	}
	tree.PrintTree()

}

func TestJustify(t *testing.T) {
	leaves := [][]byte{
		[]byte("leaf1"),
		[]byte("leaf2"),
		[]byte("leaf3"),
	}

	// Create the Merkle tree
	tree := NewCDMerkleTree(leaves)

	// Test justification for each leaf
	for i, leaf := range leaves {
		justification, err := tree.Justify(i)
		if err != nil {
			t.Fatalf("Error justifying leaf %d: %v", i, err)
		}

		leafHash := computeLeaf(leaf)
		computedRoot := verifyJustification(leafHash, i, justification)

		if !compareBytes(computedRoot, tree.Root()) {
			t.Errorf("Justification failed for leaf %d: expected root %s, got %s", i, hex.EncodeToString(tree.Root()), hex.EncodeToString(computedRoot))
		} else {
			t.Logf("Justification verified for leaf %d", i)
		}
	}
}

func TestJustifyX(t *testing.T) {
	leaves := [][]byte{
		[]byte("leaf1"),
		[]byte("leaf2"),
		[]byte("leaf3"),
	}

	// Create the Merkle tree
	tree := NewCDMerkleTree(leaves)
	tree.PrintTree()

	// Define the index and size x
	index := 1
	x := 2

	// Get the justification
	justification, err := tree.JustifyX(index, x)
	if err != nil {
		t.Fatalf("JustifyX returned an error: %v", err)
	}

	// Verify the length of the justification
	if len(justification) != x {
		t.Fatalf("JustifyX returned %d elements, expected %d", len(justification), x)
	}

	// Expected justification values (this may need to be adjusted based on your actual tree structure)
	expectedJustification := [][]byte{
		tree.leaves[0].Hash,
		computeNode(append(tree.leaves[2].Hash, make([]byte, 32)...)), // Assuming the padding is a zero hash
	}

	// Verify the justification values
	for i, expectedHash := range expectedJustification {
		if !compareBytes(justification[i], expectedHash) {
			t.Errorf("JustifyX element %d hash mismatch: expected %x, got %x", i, expectedHash, justification[i])
		} else {
			fmt.Printf("JustifyX element %d hash verified: %x\n", i, justification[i])
		}
	}
}

func TestVerifyJustification(t *testing.T) {
	leaves := [][]byte{
		[]byte("leaf1"),
		[]byte("leaf2"),
		[]byte("leaf3"),
	}

	// Build Merkle Tree
	tree := NewCDMerkleTree(leaves)
	tree.PrintTree()

	// Choose a leaf to verify
	index := 1 // Choose leaf2
	leafHash := computeLeaf(leaves[index])

	// Get justification
	justification, err := tree.Justify(index)
	if err != nil {
		t.Fatalf("Failed to get justification: %v", err)
	}

	// Verify justification
	computedRoot := verifyJustification(leafHash, index, justification)
	expectedRoot := tree.Root()

	if !compareBytes(computedRoot, expectedRoot) {
		t.Errorf("Root hash mismatch: expected %x, got %x", expectedRoot, computedRoot)
	} else {
		fmt.Printf("Root hash verified: %x\n", computedRoot)
	}
}

func TestVerifyJustifyX(t *testing.T) {
	// Initialize the tree with some values
	leaves := [][]byte{
		[]byte("leaf1"),
		[]byte("leaf2"),
		[]byte("leaf3"),
		[]byte("leaf4"),
	}

	// Build Merkle Tree
	tree := NewCDMerkleTree(leaves)
	tree.PrintTree()

	// Index of the leaf to justify
	index := 2 // leaf3
	leafHash := computeLeaf([]byte("leaf3"))

	// Get justification for the leaf
	x := 3 // The depth of the tree or less
	justification, err := tree.JustifyX(index, x)
	if err != nil {
		t.Fatalf("JustifyX failed: %v", err)
	}

	// Verify the justification
	computedRoot := verifyJustifyX(leafHash, index, justification, x)
	expectedRoot := tree.Root()

	if !compareBytes(computedRoot, expectedRoot) {
		t.Errorf("Root hash mismatch: expected %x, got %x", expectedRoot, computedRoot)
	} else {
		fmt.Printf("Root hash verified: %x\n", computedRoot)
	}
}

func TestCDTGet(t *testing.T) {
	leaves := [][]byte{
		[]byte("leaf1"),
		[]byte("leaf2"),
		[]byte("leaf3"),
		[]byte("leaf4"),
	}
	// Build Merkle Tree
	tree := NewCDMerkleTree(leaves)
	tree.PrintTree()
	for i := 0; i <= len(leaves); i++ {
		{
			if i < len(leaves) {
				leaf, err := tree.Get(i)
				if err == nil {
					fmt.Printf("Get %d: %s\n", i, string(leaf))
				} else {
					t.Errorf("Get %d should be Error: %v\n", i, err)
				}
			} else {
				_, err := tree.Get(i)
				if err != nil {
					fmt.Printf("Error: %v\n", err)
				} else {
					t.Errorf("Get %d should be Error: %v\n", i, err)
				}
			}
		}
	}
}

// TestGeneratePageProof tests the generation of page proofs
func TestGeneratePageProof(t *testing.T) {
	var segments []common.Segment

	for i := 1; i <= 65; i++ {
		data := []byte(fmt.Sprintf("segment%d", i))
		segments = append(segments, common.Segment{Data: data})
	}

	pagedProofs := generatePageProof(segments)

	if len(pagedProofs) == 0 {
		t.Fatalf("Expected non-empty page proofs")
	}

	// print the Merkle Root of each page
	for i, proof := range pagedProofs {
		t.Logf("Page %d MerkleRoot: %s", i, hex.EncodeToString(proof.MerkleRoot[:]))
		for j, segmentHash := range proof.SegmentHashes {
			if segmentHash != (common.Hash{}) { // if the hash is not empty
				t.Logf("Segment %d Hash: %s", j, hex.EncodeToString(segmentHash[:]))
			}
		}
	}

	// check the number of segment hashes in each page
	for i, proof := range pagedProofs {
		if len(proof.SegmentHashes) != 64 {
			t.Errorf("Page %d does not have 64 segment hashes, but %d", i, len(proof.SegmentHashes))
		}
	}
}
