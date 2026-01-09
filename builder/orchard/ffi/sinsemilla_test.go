package ffi

import (
	"testing"
)

func TestSinsemillaMerkleTree(t *testing.T) {
	tree := NewSinsemillaMerkleTree()

	// Test with empty tree
	if tree.GetSize() != 0 {
		t.Errorf("Expected empty tree size 0, got %d", tree.GetSize())
	}

	// Generate test leaves (use simple values instead of random)
	testLeaves := make([][32]byte, 4)
	for i := range testLeaves {
		// Use predictable values instead of random for testing
		testLeaves[i][0] = byte(i + 1)
		// Rest are zeros, which are valid field elements
	}

	// Test ComputeRoot
	err := tree.ComputeRoot(testLeaves)
	if err != nil {
		t.Fatalf("ComputeRoot failed: %v", err)
	}

	if tree.GetSize() != 4 {
		t.Errorf("Expected tree size 4 after ComputeRoot, got %d", tree.GetSize())
	}
	if len(tree.GetFrontier()) != treeDepth {
		t.Errorf("Expected frontier depth %d after ComputeRoot, got %d", treeDepth, len(tree.GetFrontier()))
	}

	root1 := tree.GetRoot()
	if root1 == [32]byte{} {
		t.Error("Expected non-zero root after ComputeRoot")
	}

	// Test AppendWithFrontier
	newLeaves := make([][32]byte, 2)
	for i := range newLeaves {
		// Use predictable values
		newLeaves[i][0] = byte(i + 5)
		// Rest are zeros
	}

	err = tree.AppendWithFrontier(newLeaves)
	if err != nil {
		t.Fatalf("AppendWithFrontier failed: %v", err)
	}

	if tree.GetSize() != 6 {
		t.Errorf("Expected tree size 6 after append, got %d", tree.GetSize())
	}
	if len(tree.GetFrontier()) != treeDepth {
		t.Errorf("Expected frontier depth %d after append, got %d", treeDepth, len(tree.GetFrontier()))
	}

	root2 := tree.GetRoot()
	if root2 == root1 {
		t.Error("Expected different root after appending leaves")
	}

	// Test GenerateProof
	allCommitments := append(testLeaves, newLeaves...)
	proof, err := tree.GenerateProof(testLeaves[0], 0, allCommitments)
	if err != nil {
		t.Fatalf("GenerateProof failed: %v", err)
	}

	if len(proof) == 0 {
		t.Error("Expected non-empty proof")
	}

	t.Logf("Generated proof with %d siblings", len(proof))
}

func TestSinsemillaErrors(t *testing.T) {
	tree := NewSinsemillaMerkleTree()

	// Test out of bounds proof generation
	testLeaves := make([][32]byte, 2)
	for i := range testLeaves {
		testLeaves[i][0] = byte(i + 1)
	}

	if err := tree.ComputeRoot(testLeaves); err != nil {
		t.Fatalf("ComputeRoot failed: %v", err)
	}

	_, err := tree.GenerateProof(testLeaves[0], 5, testLeaves)
	if err == nil {
		t.Error("Expected error for out of bounds position")
	}

	// Test commitment mismatch
	var wrongCommitment [32]byte
	wrongCommitment[0] = 99
	_, err = tree.GenerateProof(wrongCommitment, 0, testLeaves)
	if err == nil {
		t.Error("Expected error for commitment mismatch")
	}
}

func BenchmarkSinsemillaComputeRoot(b *testing.B) {
	// Generate test data (use valid field elements)
	leaves := make([][32]byte, 100)
	for i := range leaves {
		leaves[i][0] = byte(i % 256)
		// Rest are zeros, which are valid field elements
	}

	tree := NewSinsemillaMerkleTree()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := tree.ComputeRoot(leaves)
		if err != nil {
			b.Fatalf("ComputeRoot failed: %v", err)
		}
	}
}
