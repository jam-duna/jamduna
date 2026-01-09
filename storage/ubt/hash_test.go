package ubt

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestZeroHashSpecialCaseBlake3 validates that Hash64 returns ZERO32 for (ZERO32, ZERO32).
// This is a critical property per EIP-7864: hash64(0x00...00, 0x00...00) = 0x00...00
func TestZeroHashSpecialCaseBlake3(t *testing.T) {
	hasher := NewBlake3Hasher(EIPProfile)

	zero := [32]byte{}

	// Hash64 has zero-hash rule
	result64 := hasher.Hash64(&zero, &zero)
	assert.Equal(t, zero, result64, "Hash64(ZERO32, ZERO32) must return ZERO32")

	// Note: Hash32 does NOT have zero-hash rule (returns actual Blake3 hash)
}

// TestHash32NoZeroRule validates that Hash32 returns actual hash, not zero.
// This confirms Hash32(ZERO32) != ZERO32
func TestHash32NoZeroRule(t *testing.T) {
	hasher := NewBlake3Hasher(EIPProfile)

	zero := [32]byte{}
	result := hasher.Hash32(&zero)

	// Hash32 should return actual Blake3 hash, not zero
	assert.NotEqual(t, zero, result, "Hash32(ZERO32) must NOT return ZERO32")
}

// TestNonZeroHashBlake3 validates that non-zero inputs produce non-zero hashes.
func TestNonZeroHashBlake3(t *testing.T) {
	hasher := NewBlake3Hasher(EIPProfile)

	value := [32]byte{}
	for i := range value {
		value[i] = 0x42
	}

	zero := [32]byte{}

	// Hash32 should return non-zero for non-zero input
	hash32 := hasher.Hash32(&value)
	assert.NotEqual(t, zero, hash32, "Hash32(0x42...) must be non-zero")

	// Hash64 with at least one non-zero input should return non-zero
	hash64 := hasher.Hash64(&value, &zero)
	assert.NotEqual(t, zero, hash64, "Hash64(0x42..., 0x00...) must be non-zero")
}

// TestBlake3Deterministic validates that the same input produces the same hash.
func TestBlake3Deterministic(t *testing.T) {
	hasher := NewBlake3Hasher(EIPProfile)

	value := [32]byte{}
	for i := range value {
		value[i] = 0x42
	}

	hash1 := hasher.Hash32(&value)
	hash2 := hasher.Hash32(&value)

	assert.Equal(t, hash1, hash2, "Hash32 must be deterministic")
}

// TestBlake3ProfileSeparation validates that different profiles produce different hashes.
func TestBlake3ProfileSeparation(t *testing.T) {
	hasherEIP := NewBlake3Hasher(EIPProfile)
	hasherJAM := NewBlake3Hasher(JAMProfile)

	value := [32]byte{}
	for i := range value {
		value[i] = 0x42
	}

	// Non-zero inputs should hash differently across profiles
	hashEIP := hasherEIP.Hash32(&value)
	hashJAM := hasherJAM.Hash32(&value)

	assert.NotEqual(t, hashEIP, hashJAM, "EIP and JAM profiles must produce different hashes for non-zero input")

	// But both should still respect zero-hash rule for Hash64
	zero := [32]byte{}
	assert.Equal(t, zero, hasherEIP.Hash64(&zero, &zero), "EIP Hash64(ZERO32, ZERO32) = ZERO32")
	assert.Equal(t, zero, hasherJAM.Hash64(&zero, &zero), "JAM Hash64(ZERO32, ZERO32) = ZERO32")
}

// TestEmptyTreeRootHash validates that empty tree root hash is ZERO32.
// This is separate from Hash32/Hash64 - it's a tree-level property.
func TestEmptyTreeRootHash(t *testing.T) {
	tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})

	root := tree.RootHash()
	zero := [32]byte{}

	assert.Equal(t, zero, root, "Empty tree RootHash() must return ZERO32")
}
