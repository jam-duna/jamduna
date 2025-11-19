package core

import (
	"encoding/hex"
	"testing"
)

func TestBlake2bGpHasher(t *testing.T) {
	hasher := NewGpNodeHasher()

	// Test empty trie (terminator node)
	terminator := Terminator
	if !IsTerminator(terminator) {
		t.Error("Terminator should be recognized as terminator")
	}

	// Test hashing a simple leaf with embedded value
	keyPath := KeyPath{}
	copy(keyPath[:], []byte("test_key_path_000000000000000"))
	value := []byte("small_value")
	valueHash := hasher.HashValue(value)

	leafData := NewLeafDataGP(keyPath, value, valueHash)
	leafHash := hasher.HashLeaf(leafData)

	t.Logf("Leaf hash (embedded): %s", hex.EncodeToString(leafHash[:]))

	// Verify leaf hash is not terminator
	if IsTerminator(leafHash) {
		t.Error("Leaf hash should not be terminator")
	}

	// Test hashing a leaf with large value (hashed, not embedded)
	largeValue := make([]byte, 100)
	for i := range largeValue {
		largeValue[i] = byte(i)
	}
	largeValueHash := hasher.HashValue(largeValue)

	leafDataLarge := NewLeafDataGPOverflow(keyPath, uint32(len(largeValue)), largeValueHash)
	leafHashLarge := hasher.HashLeaf(leafDataLarge)

	t.Logf("Leaf hash (hashed): %s", hex.EncodeToString(leafHashLarge[:]))

	// Verify the two leaf hashes are different
	if leafHash == leafHashLarge {
		t.Error("Embedded and hashed leaf should produce different hashes")
	}

	// Test hashing an internal node
	left := leafHash
	right := leafHashLarge

	internalData := &InternalData{
		Left:  left,
		Right: right,
	}

	internalHash := hasher.HashInternal(internalData)
	t.Logf("Internal hash: %s", hex.EncodeToString(internalHash[:]))

	// Verify internal hash is not terminator
	if IsTerminator(internalHash) {
		t.Error("Internal hash should not be terminator")
	}
}

func TestNodeKindByMSB(t *testing.T) {
	// Test terminator
	if NodeKindByMSB(Terminator) != NodeKindTerminator {
		t.Error("Terminator should be recognized as NodeKindTerminator")
	}

	// Test leaf (MSB = 1)
	leafNode := Node{}
	leafNode[0] = 0b10000000
	if NodeKindByMSB(leafNode) != NodeKindLeaf {
		t.Error("Node with MSB=1 should be NodeKindLeaf")
	}

	// Test internal (MSB = 0, not terminator)
	internalNode := Node{}
	internalNode[0] = 0b01111111
	internalNode[1] = 0x01 // Make it non-zero
	if NodeKindByMSB(internalNode) != NodeKindInternal {
		t.Error("Node with MSB=0 (not terminator) should be NodeKindInternal")
	}
}

func TestSetUnsetMSB(t *testing.T) {
	node := Node{}
	node[0] = 0b01010101

	SetMSB(&node)
	if node[0] != 0b11010101 {
		t.Errorf("SetMSB failed: got %08b, want %08b", node[0], 0b11010101)
	}

	UnsetMSB(&node)
	if node[0] != 0b01010101 {
		t.Errorf("UnsetMSB failed: got %08b, want %08b", node[0], 0b01010101)
	}
}
