package trie

import (
	"bytes"
	"testing"
)

// TestMMR tests the Merkle Mountain Range implementation
func TestMMR(t *testing.T) {
	mmr := NewMMR()
	leaves := [][]byte{
		bhash([]byte("leaf1")),
		bhash([]byte("leaf2")),
		bhash([]byte("leaf3")),
		bhash([]byte("leaf4")),
	}
	for _, leaf := range leaves {
		mmr.Append(leaf)
	}
	root := mmr.Root()
	expectedRoot := bhash(append(bhash(bhash(append(bhash([]byte("leaf1")), bhash([]byte("leaf2"))...))), bhash(bhash(append(bhash([]byte("leaf3")), bhash([]byte("leaf4"))...)))...))
	if !bytes.Equal(root, expectedRoot) {
		t.Fatalf("unexpected root hash: got %x, want %x", root, expectedRoot)
	}
}
