package trie

import (
	"testing"
)

func TestWellBalancedTree(t *testing.T) {
	tree := NewWellBalancedTree()

	data1 := []byte("data1")
	data2 := []byte("data2")
	data3 := []byte("data3")
	data4 := []byte("data4")

	tree.Insert(data1)
	tree.Insert(data2)
	tree.Insert(data3)
	tree.Insert(data4)

	rootHash := tree.RootHash()
	t.Logf("rootHash=%x\n", rootHash)
	if len(rootHash) != 32 {
		t.Fatalf("unexpected root hash length: got %d, want 32", len(rootHash))
	}

	hash3 := computeHash(data3)

	// Test Trace
	tracePath, err := tree.Trace(hash3)
	if err != nil {
		t.Fatalf("Trace error: %v", err)
	}
	t.Logf("Trace path: %x\n", tracePath)

	// Test VerifyProof
	if !tree.VerifyProof(hash3, data3, tracePath) {
		t.Errorf("Proof verification failed for key '%x'", hash3)
	} else {
		t.Logf("Proof verification succeeded for key '%x'", hash3)
	}

	// Test failed verification
	hash1 := computeHash(data1)
	if tree.VerifyProof(hash1, data3, tracePath) {
		t.Errorf("Proof verification should have failed for mismatched data")
	} else {
		t.Logf("Proof verification correctly failed for mismatched data")
	}
}
