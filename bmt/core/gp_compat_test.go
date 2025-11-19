package core

import (
	"encoding/hex"
	"testing"
)

// TestGPLeafEncoding validates GP leaf encoding for embedded and hashed values.
func TestGPLeafEncoding(t *testing.T) {
	// Test embedded value (≤ 32 bytes)
	keyHex := "f2a9fcaf8ae0ff770b0908ebdee1daf8457c0ef5e1106c89ad364236333c5fb3"
	key, _ := hex.DecodeString(keyHex)
	var keyPath KeyPath
	copy(keyPath[:], key)

	value := []byte("hello")
	hasher := NewGpNodeHasher()
	valueHash := hasher.HashValue(value)

	leafData := NewLeafDataGP(keyPath, value, valueHash)
	encoded := EncodeGpLeaf(leafData)

	// Check header byte: 0b10 + 6-bit length (5)
	expectedHeader := byte(0b10000000 | 5)
	if encoded[0] != expectedHeader {
		t.Errorf("Header mismatch: got %08b, want %08b", encoded[0], expectedHeader)
	}

	// Check key (31 bytes)
	if !bytesEqual(encoded[1:32], keyPath[:31]) {
		t.Error("Key encoding mismatch")
	}

	// Check embedded value
	if !bytesEqual(encoded[32:37], value) {
		t.Error("Value embedding mismatch")
	}

	t.Log("✓ GP leaf encoding test passed (embedded value)")
}

func TestGPLeafEncodingLargeValue(t *testing.T) {
	// Test large value (> 32 bytes)
	keyHex := "f2a9fcaf8ae0ff770b0908ebdee1daf8457c0ef5e1106c89ad364236333c5fb3"
	key, _ := hex.DecodeString(keyHex)
	var keyPath KeyPath
	copy(keyPath[:], key)

	valueHex := "22c62f84ee5775d1e75ba6519f6dfae571eb1888768f2a203281579656b6a29097f7c7e2cf44e38da9a541d9b4c773db8b71e1d3"
	value, _ := hex.DecodeString(valueHex)

	hasher := NewGpNodeHasher()
	valueHash := hasher.HashValue(value)

	leafData := NewLeafDataGPOverflow(keyPath, uint32(len(value)), valueHash)
	encoded := EncodeGpLeaf(leafData)

	// Check header byte: 0b11000000
	if encoded[0] != 0b11000000 {
		t.Errorf("Header mismatch: got %08b, want %08b", encoded[0], 0b11000000)
	}

	// Check key (31 bytes)
	if !bytesEqual(encoded[1:32], keyPath[:31]) {
		t.Error("Key encoding mismatch")
	}

	// Check value hash (32 bytes)
	expectedValueHash := hasher.HashValue(value)
	if !bytesEqual(encoded[32:64], expectedValueHash[:]) {
		t.Error("Value hash mismatch")
	}

	t.Log("✓ GP leaf encoding test passed (large value)")
}

func TestGPBranchEncoding(t *testing.T) {
	// Test branch encoding (GP Equation D.3)
	left := Node{}
	right := Node{}
	for i := range left {
		left[i] = 0xFF // MSB set
		right[i] = 0xAA
	}

	internalData := &InternalData{
		Left:  left,
		Right: right,
	}

	encoded := EncodeGpInternal(internalData)

	// Check that left MSB was cleared
	if encoded[0]&0x80 != 0 {
		t.Error("Left MSB should be cleared")
	}

	if encoded[0] != 0xFF&0b01111111 {
		t.Errorf("Left should have MSB cleared: got %02x, want %02x", encoded[0], 0xFF&0b01111111)
	}

	// Check rest of left
	if encoded[1] != 0xFF {
		t.Error("Rest of left should be unchanged")
	}

	// Check right is unchanged
	if encoded[32] != 0xAA {
		t.Error("Right should be unchanged")
	}

	// Hash the branch
	hasher := Blake2bBinaryHasher{}
	branchHash := hasher.Hash(encoded[:])
	t.Logf("✓ GP branch encoding test passed")
	t.Logf("  Branch hash: %s", hex.EncodeToString(branchHash[:]))
}

func TestBlake2b256(t *testing.T) {
	// Verify Blake2b-256 is working correctly
	data := []byte("test data")
	hasher := Blake2bBinaryHasher{}
	hash := hasher.Hash(data)

	if len(hash) != 32 {
		t.Errorf("Blake2b-256 should produce 32-byte hash, got %d", len(hash))
	}

	// Verify it's deterministic
	hash2 := hasher.Hash(data)
	if hash != hash2 {
		t.Error("Hashing should be deterministic")
	}

	t.Logf("✓ Blake2b-256 test passed")
	t.Logf("  Hash: %s", hex.EncodeToString(hash[:]))
}

// Helper function to compare byte slices
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
