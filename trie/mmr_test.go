package trie

import (
	"bytes"
	//"github.com/ethereum/go-ethereum/common"
	"testing"
)

// TestMMR tests the Merkle Mountain Range implementation
func TestMMR(t *testing.T) {
	t.Skip()
	mmr := NewMMR()
	leaves := [][]byte{
		computeHash([]byte("leaf1")),
		computeHash([]byte("leaf2")),
		computeHash([]byte("leaf3")),
		computeHash([]byte("leaf4")),
	}
	for _, leaf := range leaves {
		mmr.Append(leaf)
	}
	root := mmr.Root()
	expectedRoot := computeHash(append(computeHash(computeHash(append(computeHash([]byte("leaf1")), computeHash([]byte("leaf2"))...))), computeHash(computeHash(append(computeHash([]byte("leaf3")), computeHash([]byte("leaf4"))...)))...))
	if !bytes.Equal(root.Bytes(), expectedRoot) {
		t.Fatalf("unexpected root hash: got %x, want %x", root, expectedRoot)
	}
}
