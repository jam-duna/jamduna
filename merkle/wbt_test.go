package merkle

import (
	"encoding/hex"
	"testing"
)

// Test function
func TestBalancedMerkleTree(t *testing.T) {
	data := [][]byte{
		[]byte("a"),
		[]byte("b"),
		[]byte("c"),
		[]byte("d"),
	}
	tree := NewBalancedMerkleTree(data)
	tracePath, err := tree.Trace(2, data)
	if err != nil {
		t.Fatalf("Failed to trace: %v", err)
	}

	expectedHashes := []string{
		"expected_hash_1",
		"expected_hash_2",
	}

	for i, hash := range tracePath {
		encodedHash := hex.EncodeToString(hash)
		if encodedHash != expectedHashes[i] {
			t.Errorf("Trace path hash mismatch at index %d: got %s, want %s", i, encodedHash, expectedHashes[i])
		}
	}
}
