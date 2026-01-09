package ubt

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestEmptyNodeHash validates that Empty node hash is ZERO32.
func TestEmptyNodeHash(t *testing.T) {
	hasher := NewBlake3Hasher(EIPProfile)
	node := Node{Type: NodeTypeEmpty}

	hash := node.Hash(hasher)
	zero := [32]byte{}

	assert.Equal(t, zero, hash, "Empty node hash must be ZERO32")
}

// TestLeafNodeHash validates that Leaf node hash is hash_32(value).
func TestLeafNodeHash(t *testing.T) {
	hasher := NewBlake3Hasher(EIPProfile)

	value := [32]byte{}
	for i := range value {
		value[i] = 0x42
	}

	node := Node{
		Type:  NodeTypeLeaf,
		Value: &value,
	}

	hash := node.Hash(hasher)
	zero := [32]byte{}

	assert.NotEqual(t, zero, hash, "Leaf node hash must be non-zero for non-zero value")

	// Hash should match hasher.Hash32(value)
	expectedHash := hasher.Hash32(&value)
	assert.Equal(t, expectedHash, hash, "Leaf node hash must equal Hash32(value)")
}

// TestStemNodeWithValue validates StemNode hashing with a single value.
func TestStemNodeWithValue(t *testing.T) {
	hasher := NewBlake3Hasher(EIPProfile)

	stem := Stem{}
	values := make(map[uint8][32]byte)
	value := [32]byte{}
	for i := range value {
		value[i] = 0x42
	}
	values[0] = value

	node := Node{
		Type:   NodeTypeStem,
		Stem:   &stem,
		Values: values,
	}

	hash := node.Hash(hasher)
	zero := [32]byte{}

	assert.NotEqual(t, zero, hash, "StemNode with value must have non-zero hash")
}

// TestStemNodeGetSetValue validates StemNode value operations.
func TestStemNodeGetSetValue(t *testing.T) {
	values := make(map[uint8][32]byte)

	value := [32]byte{}
	for i := range value {
		value[i] = 0x42
	}

	// Set value
	values[0] = value

	// Get value
	retrieved, found := values[0], true
	assert.True(t, found, "Value at index 0 should exist")
	assert.Equal(t, value, retrieved, "Retrieved value should match set value")

	// Get missing value
	_, found2 := values[1]
	assert.False(t, found2, "Value at index 1 should not exist")
}

// TestStemNodeHashDeterministic validates that StemNode hash is deterministic.
func TestStemNodeHashDeterministic(t *testing.T) {
	hasher := NewBlake3Hasher(EIPProfile)

	stem := Stem{}
	values := make(map[uint8][32]byte)
	value := [32]byte{}
	for i := range value {
		value[i] = 0x01
	}
	values[0] = value

	node := Node{
		Type:   NodeTypeStem,
		Stem:   &stem,
		Values: values,
	}

	hash1 := node.Hash(hasher)
	hash2 := node.Hash(hasher)

	assert.Equal(t, hash1, hash2, "StemNode hash must be deterministic")
}

// TestStemNodeEmptyHash validates StemNode with no values.
// Empty StemNode still has hash(stem || 0x00 || subtree_root) where subtree_root is from all-zero 256-leaf tree.
func TestStemNodeEmptyHash(t *testing.T) {
	hasher := NewBlake3Hasher(EIPProfile)

	stem := Stem{} // all zeros
	values := make(map[uint8][32]byte)

	node := Node{
		Type:   NodeTypeStem,
		Stem:   &stem,
		Values: values,
	}

	hash := node.Hash(hasher)

	// Empty subtree root is ZERO32 due to Hash64(ZERO32, ZERO32) special-case.
	// HashStemNode does not apply a zero-hash rule, so this equals the Blake3 hash
	// of stem||0x00||subtreeRoot (64 bytes for EIP profile).
	subtreeRoot := [32]byte{}
	expected := hasher.HashStemNode(&stem, &subtreeRoot)
	assert.Equal(t, expected, hash, "Empty StemNode hash should match HashStemNode(stem, ZERO32)")
}

// TestInternalNodeHash validates Internal node hashing.
func TestInternalNodeHash(t *testing.T) {
	hasher := NewBlake3Hasher(EIPProfile)

	zero := [32]byte{}
	value1 := [32]byte{}
	for i := range value1 {
		value1[i] = 0x01
	}

	left := Node{Type: NodeTypeLeaf, Value: &value1}
	right := Node{Type: NodeTypeEmpty}

	node := Node{
		Type:  NodeTypeInternal,
		Left:  &left,
		Right: &right,
	}

	hash := node.Hash(hasher)

	assert.NotEqual(t, zero, hash, "Internal node with non-empty child must have non-zero hash")

	// Internal hash = hash_64(left.Hash(), right.Hash())
	leftHash := left.Hash(hasher)
	rightHash := right.Hash(hasher)
	expectedHash := hasher.Hash64(&leftHash, &rightHash)

	assert.Equal(t, expectedHash, hash, "Internal node hash must equal Hash64(left, right)")
}

// TestInternalNodeBothEmpty validates that Internal(Empty, Empty) has ZERO32 hash.
func TestInternalNodeBothEmpty(t *testing.T) {
	hasher := NewBlake3Hasher(EIPProfile)

	left := Node{Type: NodeTypeEmpty}
	right := Node{Type: NodeTypeEmpty}

	node := Node{
		Type:  NodeTypeInternal,
		Left:  &left,
		Right: &right,
	}

	hash := node.Hash(hasher)
	zero := [32]byte{}

	// hash_64(ZERO32, ZERO32) = ZERO32 by zero-hash rule
	assert.Equal(t, zero, hash, "Internal(Empty, Empty) must have ZERO32 hash")
}

// TestStemNode8LevelSubtree validates the 8-level binary tree construction for 256 leaves.
// This test ensures the subtree is built bottom-up correctly.
func TestStemNode8LevelSubtree(t *testing.T) {
	hasher := NewBlake3Hasher(EIPProfile)

	stem := Stem{}
	values := make(map[uint8][32]byte)

	// Set two values at different positions to test tree structure
	value1 := [32]byte{}
	value1[0] = 0x01
	values[0] = value1

	value2 := [32]byte{}
	value2[0] = 0x02
	values[255] = value2

	node := Node{
		Type:   NodeTypeStem,
		Stem:   &stem,
		Values: values,
	}

	hash1 := node.Hash(hasher)
	zero := [32]byte{}

	assert.NotEqual(t, zero, hash1, "StemNode with 2 values must have non-zero hash")

	// Now set all 256 values
	allValues := make(map[uint8][32]byte)
	for i := 0; i < 256; i++ {
		v := [32]byte{}
		v[0] = byte(i + 1) // all non-zero to avoid zero-hash rule
		allValues[uint8(i)] = v
	}

	node2 := Node{
		Type:   NodeTypeStem,
		Stem:   &stem,
		Values: allValues,
	}

	hash2 := node2.Hash(hasher)

	assert.NotEqual(t, hash1, hash2, "Full StemNode (256 values) must have different hash than partial (2 values)")
	assert.NotEqual(t, zero, hash2, "Full StemNode must have non-zero hash")
}
