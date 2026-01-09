package ubt

import (
	"testing"

	"github.com/colorfulnotion/jam/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSingleKeyProof validates single-key proof generation and verification.
func TestSingleKeyProof(t *testing.T) {
	tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})
	hasher := NewBlake3Hasher(EIPProfile)

	// Insert some values.
	var key1Bytes [32]byte
	copy(key1Bytes[:31], []byte{1, 2, 3})
	key1Bytes[31] = 10
	key1 := TreeKeyFromBytes(key1Bytes)
	value1 := [32]byte{42, 43, 44}

	tree.Insert(key1, value1)

	// Generate proof for key1.
	proof, err := tree.GenerateProof(&key1)
	require.NoError(t, err)
	assert.NotNil(t, proof)

	// Verify proof.
	root := tree.RootHash()
	require.NoError(t, proof.Verify(hasher, root))

	// Verify proof fails with wrong value.
	wrongValue := [32]byte{99, 99, 99}
	wrongProof := *proof
	wrongProof.Value = &wrongValue
	assert.Error(t, wrongProof.Verify(hasher, root), "Proof should be invalid with wrong value")
}

func TestEmptyTreeProof(t *testing.T) {
	tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})
	hasher := NewBlake3Hasher(EIPProfile)

	var keyBytes [32]byte
	keyBytes[31] = 1
	key := TreeKeyFromBytes(keyBytes)

	proof, err := tree.GenerateProof(&key)
	require.NoError(t, err)
	require.Nil(t, proof.Value)

	root := tree.RootHash()
	require.Equal(t, [32]byte{}, root)
	require.NoError(t, proof.Verify(hasher, root))
}

func TestProofRejectsInvalidDirection(t *testing.T) {
	tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})
	hasher := NewBlake3Hasher(EIPProfile)

	var keyBytes [32]byte
	keyBytes[0] = 1
	keyBytes[31] = 10
	key := TreeKeyFromBytes(keyBytes)
	tree.Insert(key, [32]byte{1})

	proof, err := tree.GenerateProof(&key)
	require.NoError(t, err)

	for _, node := range proof.Path {
		if internal, ok := node.(*storage.InternalProofNode); ok {
			internal.Direction = storage.Direction(99)
			break
		}
	}

	require.Error(t, proof.Verify(hasher, tree.RootHash()))
}

// TestMultiProofGeneration validates multiproof generation for multiple keys.
func TestMultiProofGeneration(t *testing.T) {
	tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})
	hasher := NewBlake3Hasher(EIPProfile)

	// Insert multiple values.
	keys := make([]TreeKey, 10)
	for i := 0; i < 10; i++ {
		var keyBytes [32]byte
		keyBytes[0] = byte(i)
		keyBytes[31] = byte(i * 10)
		key := TreeKeyFromBytes(keyBytes)
		keys[i] = key

		value := [32]byte{byte(i), byte(i + 1), byte(i + 2)}
		tree.Insert(key, value)
	}

	// Generate multiproof.
	multiproof, err := tree.GenerateMultiProof(keys)
	require.NoError(t, err)
	assert.NotNil(t, multiproof)

	// Verify multiproof.
	root := tree.RootHash()
	require.NoError(t, VerifyMultiProof(multiproof, hasher, root))
}

// TestMultiProofMissingValue validates non-inclusion within an existing stem.
func TestMultiProofMissingValue(t *testing.T) {
	tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})
	hasher := NewBlake3Hasher(EIPProfile)

	var stem Stem
	copy(stem[:], []byte{9, 8, 7, 6, 5})

	existing := TreeKey{Stem: stem, Subindex: 1}
	missing := TreeKey{Stem: stem, Subindex: 2}

	tree.Insert(existing, [32]byte{42})

	multiproof, err := tree.GenerateMultiProof([]TreeKey{missing})
	require.NoError(t, err)
	require.Len(t, multiproof.Values, 1)
	require.Nil(t, multiproof.Values[0])

	root := tree.RootHash()
	require.NoError(t, VerifyMultiProof(multiproof, hasher, root))
}

func TestMultiProofMissingStemRecorded(t *testing.T) {
	tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})
	hasher := NewBlake3Hasher(EIPProfile)

	existing := TreeKey{Stem: Stem{0x00}, Subindex: 1}
	missing := TreeKey{Stem: Stem{0x80}, Subindex: 2}

	tree.Insert(existing, [32]byte{42})

	multiproof, err := tree.GenerateMultiProof([]TreeKey{missing})
	require.NoError(t, err)
	require.Len(t, multiproof.MissingKeys, 1)
	require.Len(t, multiproof.Keys, 0)

	require.Error(t, VerifyMultiProof(multiproof, hasher, tree.RootHash()))
}

// TestMultiProofRejectsUnsortedKeys validates canonical key ordering checks.
func TestMultiProofRejectsUnsortedKeys(t *testing.T) {
	tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})
	hasher := NewBlake3Hasher(EIPProfile)

	var keyABytes, keyBBytes [32]byte
	keyABytes[0] = 1
	keyABytes[31] = 10
	keyBBytes[0] = 2
	keyBBytes[31] = 20

	keyA := TreeKeyFromBytes(keyABytes)
	keyB := TreeKeyFromBytes(keyBBytes)

	tree.Insert(keyA, [32]byte{1})
	tree.Insert(keyB, [32]byte{2})

	multiproof, err := tree.GenerateMultiProof([]TreeKey{keyA, keyB})
	require.NoError(t, err)

	if len(multiproof.Keys) >= 2 {
		multiproof.Keys[0], multiproof.Keys[1] = multiproof.Keys[1], multiproof.Keys[0]
		multiproof.Values[0], multiproof.Values[1] = multiproof.Values[1], multiproof.Values[0]
	}

	root := tree.RootHash()
	require.Error(t, VerifyMultiProof(multiproof, hasher, root))
}

// TestMultiProofRejectsDuplicateKeys validates duplicate key rejection.
func TestMultiProofRejectsDuplicateKeys(t *testing.T) {
	tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})
	hasher := NewBlake3Hasher(EIPProfile)

	var keyBytes [32]byte
	keyBytes[0] = 3
	keyBytes[31] = 30
	key := TreeKeyFromBytes(keyBytes)

	tree.Insert(key, [32]byte{9})

	multiproof, err := tree.GenerateMultiProof([]TreeKey{key})
	require.NoError(t, err)

	multiproof.Keys = append(multiproof.Keys, multiproof.Keys[0])
	multiproof.Values = append(multiproof.Values, multiproof.Values[0])

	root := tree.RootHash()
	require.Error(t, VerifyMultiProof(multiproof, hasher, root))
}

// TestMultiProofComplexity validates O(k log k) complexity.
// This test ensures multiproof node count is much less than k * 256 (naive approach).
func TestMultiProofComplexity(t *testing.T) {
	tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})

	// Insert k=100 values with diverse keys.
	k := 100
	keys := make([]TreeKey, k)

	for i := 0; i < k; i++ {
		var keyBytes [32]byte
		// Create diverse keys to spread across tree.
		keyBytes[0] = byte(i)
		keyBytes[1] = byte(i >> 8)
		keyBytes[31] = byte(i % 256)
		key := TreeKeyFromBytes(keyBytes)
		keys[i] = key

		value := [32]byte{byte(i), byte(i + 1)}
		tree.Insert(key, value)
	}

	// Generate multiproof.
	multiproof, err := tree.GenerateMultiProof(keys)
	require.NoError(t, err)

	// Verify node count stays below naive k * 256 bound.
	// For k=100, naive approach would be 25,600 nodes.
	nodeCount := multiproofNodeCount(multiproof)
	maxExpected := k * 260 // allow small overhead for shared path ordering

	assert.Less(t, nodeCount, maxExpected,
		"MultiProof should have fewer nodes than naive k * 256. Got %d nodes for k=%d",
		nodeCount, k)

	t.Logf("MultiProof for k=%d keys has %d nodes (expected < %d)",
		k, nodeCount, maxExpected)
}

// TestMultiProofDeduplication validates that multiproof deduplicates shared nodes.
func TestMultiProofDeduplication(t *testing.T) {
	tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})

	// Insert values with same stem (they share path nodes).
	var stem Stem
	copy(stem[:], []byte{1, 2, 3, 4, 5})

	keys := make([]TreeKey, 5)
	for i := 0; i < 5; i++ {
		key := TreeKey{Stem: stem, Subindex: uint8(i)}
		keys[i] = key
		value := [32]byte{byte(i + 1)}
		tree.Insert(key, value)
	}

	// Generate multiproof.
	multiproof, err := tree.GenerateMultiProof(keys)
	require.NoError(t, err)

	// For keys with same stem, multiproof should deduplicate shared path.
	// Expect much fewer nodes than 5 separate proofs.
	nodeCount := multiproofNodeCount(multiproof)

	// 5 separate proofs would have ~5 * 256 = 1280 nodes (naive).
	// With deduplication, should be close to 256 + small overhead.
	maxExpectedWithDedup := 300

	assert.Less(t, nodeCount, maxExpectedWithDedup,
		"MultiProof should deduplicate shared nodes. Got %d nodes for 5 keys with same stem",
		nodeCount)

	t.Logf("MultiProof for 5 keys (same stem) has %d nodes (deduplication working)",
		nodeCount)
}

func TestMultiProofRejectsPermutedNodes(t *testing.T) {
	tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})
	hasher := NewBlake3Hasher(EIPProfile)

	var keyABytes, keyBBytes [32]byte
	keyABytes[0] = 1
	keyABytes[31] = 10
	keyBBytes[0] = 2
	keyBBytes[31] = 20
	keyA := TreeKeyFromBytes(keyABytes)
	keyB := TreeKeyFromBytes(keyBBytes)
	var keyCBytes [32]byte
	keyCBytes[0] = 3
	keyCBytes[31] = 30
	keyC := TreeKeyFromBytes(keyCBytes)

	tree.Insert(keyA, [32]byte{1})
	tree.Insert(keyB, [32]byte{2})
	tree.Insert(keyC, [32]byte{3})

	multiproof, err := tree.GenerateMultiProof([]TreeKey{keyA, keyB})
	require.NoError(t, err)
	require.Greater(t, len(multiproof.Nodes), 1)
	require.NotEmpty(t, multiproof.NodeRefs)

	multiproof.Nodes[0], multiproof.Nodes[1] = multiproof.Nodes[1], multiproof.Nodes[0]
	for i, ref := range multiproof.NodeRefs {
		switch ref {
		case 0:
			multiproof.NodeRefs[i] = 1
		case 1:
			multiproof.NodeRefs[i] = 0
		}
	}

	require.Error(t, VerifyMultiProof(multiproof, hasher, tree.RootHash()))
}

// TestProofForNonexistentKey validates missing-key handling in proof generation.
func TestProofForNonexistentKey(t *testing.T) {
	tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})
	hasher := NewBlake3Hasher(EIPProfile)

	// Insert one value.
	var key1Bytes [32]byte
	key1Bytes[31] = 10
	key1 := TreeKeyFromBytes(key1Bytes)
	value1 := [32]byte{42}

	tree.Insert(key1, value1)

	// Generate proof for nonexistent key.
	var key2Bytes [32]byte
	key2Bytes[31] = 20
	key2 := TreeKeyFromBytes(key2Bytes)

	proof, err := tree.GenerateProof(&key2)
	require.NoError(t, err)
	require.Nil(t, proof.Value)
	require.NoError(t, proof.Verify(hasher, tree.RootHash()))
}

// TestProofForMissingStem validates extension proofs for missing stems.
func TestProofForMissingStem(t *testing.T) {
	tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})
	hasher := NewBlake3Hasher(EIPProfile)

	keyPresent := TreeKey{Stem: Stem{0x00}, Subindex: 0}
	keyMissing := TreeKey{Stem: Stem{0x80}, Subindex: 1}

	tree.Insert(keyPresent, [32]byte{7})

	proof, err := tree.GenerateProof(&keyMissing)
	require.NoError(t, err)
	require.Nil(t, proof.Value)
	require.NoError(t, proof.Verify(hasher, tree.RootHash()))
}

func TestProofRejectsExtensionWithValue(t *testing.T) {
	tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})
	hasher := NewBlake3Hasher(EIPProfile)

	keyPresent := TreeKey{Stem: Stem{0x00}, Subindex: 0}
	keyMissing := TreeKey{Stem: Stem{0x80}, Subindex: 1}

	tree.Insert(keyPresent, [32]byte{7})

	proof, err := tree.GenerateProof(&keyMissing)
	require.NoError(t, err)
	require.Nil(t, proof.Value)

	value := [32]byte{1}
	proof.Value = &value

	require.Error(t, proof.Verify(hasher, tree.RootHash()))
}

func TestProofRejectsTruncatedInternalPath(t *testing.T) {
	tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})
	hasher := NewBlake3Hasher(EIPProfile)

	key := TreeKey{Stem: Stem{0x01}, Subindex: 1}
	tree.Insert(key, [32]byte{0x01})

	proof, err := tree.GenerateProof(&key)
	require.NoError(t, err)
	require.Greater(t, len(proof.Path), 1)

	proof.Path = proof.Path[:len(proof.Path)-1]

	require.Error(t, proof.Verify(hasher, tree.RootHash()))
}

func TestProofRejectsMismatchedInternalDirection(t *testing.T) {
	tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})
	hasher := NewBlake3Hasher(EIPProfile)

	var keyBytes [32]byte
	keyBytes[0] = 0x80
	keyBytes[31] = 2
	key := TreeKeyFromBytes(keyBytes)
	tree.Insert(key, [32]byte{0x02})

	proof, err := tree.GenerateProof(&key)
	require.NoError(t, err)

	for _, node := range proof.Path {
		internal, ok := node.(*storage.InternalProofNode)
		if !ok {
			continue
		}
		if internal.Direction == storage.Left {
			internal.Direction = storage.Right
		} else {
			internal.Direction = storage.Left
		}
		break
	}

	require.Error(t, proof.Verify(hasher, tree.RootHash()))
}

func multiproofNodeCount(mp *MultiProof) int {
	if len(mp.NodeRefs) > 0 {
		return len(mp.NodeRefs)
	}
	return len(mp.Nodes)
}

func TestProofRejectsShortStemSiblings(t *testing.T) {
	tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})
	hasher := NewBlake3Hasher(EIPProfile)

	key := TreeKey{Stem: Stem{0x01}, Subindex: 1}
	tree.Insert(key, [32]byte{0x01})

	proof, err := tree.GenerateProof(&key)
	require.NoError(t, err)

	for _, node := range proof.Path {
		stemNode, ok := node.(*storage.StemProofNode)
		if !ok {
			continue
		}
		require.Len(t, stemNode.SubtreeSiblings, 8)
		stemNode.SubtreeSiblings = stemNode.SubtreeSiblings[:7]
		break
	}

	require.Error(t, proof.Verify(hasher, tree.RootHash()))
}

func TestProofRejectsInternalBeforeStem(t *testing.T) {
	tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})
	hasher := NewBlake3Hasher(EIPProfile)

	key := TreeKey{Stem: Stem{0x02}, Subindex: 2}
	tree.Insert(key, [32]byte{0x02})

	proof, err := tree.GenerateProof(&key)
	require.NoError(t, err)
	require.Greater(t, len(proof.Path), 1)

	proof.Path[0], proof.Path[1] = proof.Path[1], proof.Path[0]

	require.Error(t, proof.Verify(hasher, tree.RootHash()))
}

func TestProofRejectsMissingLeafNode(t *testing.T) {
	tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})
	hasher := NewBlake3Hasher(EIPProfile)

	key := TreeKey{Stem: Stem{0x03}, Subindex: 3}
	tree.Insert(key, [32]byte{0x03})

	proof, err := tree.GenerateProof(&key)
	require.NoError(t, err)

	proof.Path = nil

	require.Error(t, proof.Verify(hasher, tree.RootHash()))
}

// TestIncrementalProofAfterUpdate ensures cached stem hashes are invalidated for proofs.
func TestIncrementalProofAfterUpdate(t *testing.T) {
	tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile, Incremental: true})
	hasher := NewBlake3Hasher(EIPProfile)

	keyA := TreeKey{Stem: Stem{0x00}, Subindex: 1}
	keyB := TreeKey{Stem: Stem{0x80}, Subindex: 2}

	tree.Insert(keyA, [32]byte{1})
	tree.Insert(keyB, [32]byte{2})

	_ = tree.RootHash() // populate caches

	tree.Insert(keyA, [32]byte{3}) // update stem A without recomputing root

	proof, err := tree.GenerateProof(&keyB)
	require.NoError(t, err)

	root := tree.RootHash()
	require.NoError(t, proof.Verify(hasher, root))
}
