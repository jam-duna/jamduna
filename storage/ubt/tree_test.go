package ubt

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEmptyTree validates that a new tree is empty and has ZERO32 root hash.
func TestEmptyTree(t *testing.T) {
	tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})

	assert.True(t, tree.IsEmpty(), "New tree should be empty")
	assert.Equal(t, [32]byte{}, tree.RootHash(), "Empty tree root hash must be ZERO32")
}

// TestSingleInsert validates basic insert and get operations.
func TestSingleInsert(t *testing.T) {
	tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})

	var keyBytes [32]byte
	keyBytes[0] = 0x01
	key := TreeKeyFromBytes(keyBytes)

	value := [32]byte{}
	for i := range value {
		value[i] = 0x42
	}

	tree.Insert(key, value)

	assert.False(t, tree.IsEmpty(), "Tree should not be empty after insert")

	result, found, err := tree.Get(&key)
	require.NoError(t, err, "Get should not return error")
	assert.True(t, found, "Value should be found")
	assert.Equal(t, value, result, "Retrieved value should match inserted value")

	root := tree.RootHash()
	assert.NotEqual(t, [32]byte{}, root, "Root hash should be non-zero after insert")
}

// TestMultipleInsertsSameStem validates inserting multiple values with the same stem.
func TestMultipleInsertsSameStem(t *testing.T) {
	tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})

	stem := Stem{} // all zeros

	key1 := TreeKey{Stem: stem, Subindex: 0}
	key2 := TreeKey{Stem: stem, Subindex: 1}

	value1 := [32]byte{}
	value1[0] = 0x01

	value2 := [32]byte{}
	value2[0] = 0x02

	tree.Insert(key1, value1)
	tree.Insert(key2, value2)

	result1, found1, _ := tree.Get(&key1)
	assert.True(t, found1, "Value 1 should be found")
	assert.Equal(t, value1, result1, "Value 1 should match")

	result2, found2, _ := tree.Get(&key2)
	assert.True(t, found2, "Value 2 should be found")
	assert.Equal(t, value2, result2, "Value 2 should match")

	assert.Equal(t, 2, tree.Len(), "Tree should have 2 values")
}

// TestMultipleInsertsDifferentStems validates inserting values with different stems.
func TestMultipleInsertsDifferentStems(t *testing.T) {
	tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})

	stem1 := Stem{0x00}
	stem2 := Stem{0x80} // first bit set

	key1 := TreeKey{Stem: stem1, Subindex: 0}
	key2 := TreeKey{Stem: stem2, Subindex: 0}

	value1 := [32]byte{}
	value1[0] = 0x01

	value2 := [32]byte{}
	value2[0] = 0x02

	tree.Insert(key1, value1)
	tree.Insert(key2, value2)

	assert.Equal(t, 2, tree.Len(), "Tree should have 2 values")

	root := tree.RootHash()
	assert.NotEqual(t, [32]byte{}, root, "Root hash should be non-zero")
}

// TestDelete validates delete operation.
func TestDelete(t *testing.T) {
	tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})

	var keyBytes [32]byte
	keyBytes[0] = 0x01
	key := TreeKeyFromBytes(keyBytes)

	value := [32]byte{}
	value[0] = 0x42

	tree.Insert(key, value)

	_, found, _ := tree.Get(&key)
	assert.True(t, found, "Value should be found after insert")

	tree.Delete(&key)

	_, found2, _ := tree.Get(&key)
	assert.False(t, found2, "Value should not be found after delete")
}

// TestRootHashDeterministic validates that root hash is order-independent.
func TestRootHashDeterministic(t *testing.T) {
	tree1 := NewUnifiedBinaryTree(Config{Profile: EIPProfile})
	tree2 := NewUnifiedBinaryTree(Config{Profile: EIPProfile})

	key1Bytes := [32]byte{}
	key1Bytes[0] = 0x01
	key1 := TreeKeyFromBytes(key1Bytes)

	key2Bytes := [32]byte{}
	key2Bytes[0] = 0x02
	key2 := TreeKeyFromBytes(key2Bytes)

	value1 := [32]byte{}
	value1[0] = 0x11

	value2 := [32]byte{}
	value2[0] = 0x22

	// Insert in different order
	tree1.Insert(key1, value1)
	tree1.Insert(key2, value2)

	tree2.Insert(key2, value2)
	tree2.Insert(key1, value1)

	assert.Equal(t, tree1.RootHash(), tree2.RootHash(), "Root hash must be order-independent")
}

// TestWithCapacity validates tree creation with pre-allocated capacity.
func TestWithCapacity(t *testing.T) {
	tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile, Capacity: 1000})

	assert.True(t, tree.IsEmpty(), "Tree with capacity should start empty")
	assert.Equal(t, [32]byte{}, tree.RootHash(), "Empty tree with capacity should have ZERO32 root")
}

// TestStemCount validates stem counting.
func TestStemCount(t *testing.T) {
	tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})

	assert.Equal(t, 0, tree.StemCount(), "Empty tree should have 0 stems")

	stem := Stem{}
	tree.Insert(TreeKey{Stem: stem, Subindex: 0}, [32]byte{0x01})
	tree.Insert(TreeKey{Stem: stem, Subindex: 1}, [32]byte{0x02})

	assert.Equal(t, 1, tree.StemCount(), "Tree should have 1 stem")

	stem2 := Stem{0xFF}
	tree.Insert(TreeKey{Stem: stem2, Subindex: 0}, [32]byte{0x03})

	assert.Equal(t, 2, tree.StemCount(), "Tree should have 2 stems")
}

// TestSortedStemsProduceDeterministicRoot validates that sorting stems produces deterministic roots.
func TestSortedStemsProduceDeterministicRoot(t *testing.T) {
	tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})

	// Insert many keys with different stems
	for i := 0; i <= 255; i++ {
		stem := Stem{}
		stem[0] = byte(i)
		stem[15] = byte(i + 128)

		key := TreeKey{Stem: stem, Subindex: byte(i)}
		value := [32]byte{}
		value[0] = byte(i)
		if value[0] == 0 {
			value[0] = 1 // avoid zero values
		}

		tree.Insert(key, value)
	}

	root1 := tree.RootHash()

	// Create another tree with same data in reverse order
	tree2 := NewUnifiedBinaryTree(Config{Profile: EIPProfile})
	for i := 255; i >= 0; i-- {
		stem := Stem{}
		stem[0] = byte(i)
		stem[15] = byte(i + 128)

		key := TreeKey{Stem: stem, Subindex: byte(i)}
		value := [32]byte{}
		value[0] = byte(i)
		if value[0] == 0 {
			value[0] = 1
		}

		tree2.Insert(key, value)
	}

	root2 := tree2.RootHash()

	assert.Equal(t, root1, root2, "Sorted stem tree building should produce deterministic roots")
}

// TestDeferredRootComputation validates lazy root hash computation.
func TestDeferredRootComputation(t *testing.T) {
	tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})

	key1Bytes := [32]byte{0x01}
	key2Bytes := [32]byte{0x02}
	key3Bytes := [32]byte{0x03}

	key1 := TreeKeyFromBytes(key1Bytes)
	key2 := TreeKeyFromBytes(key2Bytes)
	key3 := TreeKeyFromBytes(key3Bytes)

	tree.Insert(key1, [32]byte{0x11})
	tree.Insert(key2, [32]byte{0x22})
	tree.Insert(key3, [32]byte{0x33})

	hash1 := tree.RootHash()
	assert.NotEqual(t, [32]byte{}, hash1, "Root hash should be non-zero")

	// Calling root_hash() again should return same value
	hash2 := tree.RootHash()
	assert.Equal(t, hash1, hash2, "Calling RootHash() again should return same value")

	// Create tree2 with same data
	tree2 := NewUnifiedBinaryTree(Config{Profile: EIPProfile})
	tree2.Insert(key1, [32]byte{0x11})
	tree2.Insert(key2, [32]byte{0x22})
	tree2.Insert(key3, [32]byte{0x33})

	hash3 := tree2.RootHash()
	assert.Equal(t, hash1, hash3, "Deferred computation should produce same hash")
}

// TestDeleteToEmptyResetsRootHash validates that deleting all values resets root to ZERO32.
func TestDeleteToEmptyResetsRootHash(t *testing.T) {
	tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})

	keyBytes := [32]byte{0x01}
	key := TreeKeyFromBytes(keyBytes)

	tree.Insert(key, [32]byte{0x42})
	assert.NotEqual(t, [32]byte{}, tree.RootHash(), "Non-empty tree should have non-zero root")

	tree.Delete(&key)
	root := tree.RootHash()

	assert.Equal(t, [32]byte{}, root, "Empty tree after delete should have ZERO32 root")
}

// TestContainsKey validates ContainsKey method.
func TestContainsKey(t *testing.T) {
	tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})

	keyBytes := [32]byte{0x01}
	key := TreeKeyFromBytes(keyBytes)

	assert.False(t, tree.ContainsKey(&key), "Empty tree should not contain key")

	tree.Insert(key, [32]byte{0x42})
	assert.True(t, tree.ContainsKey(&key), "Tree should contain inserted key")

	tree.Delete(&key)
	assert.False(t, tree.ContainsKey(&key), "Tree should not contain deleted key")
}

// TestIterator validates iteration over tree entries.
func TestIterator(t *testing.T) {
	tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})

	stem := Stem{}
	tree.Insert(TreeKey{Stem: stem, Subindex: 0}, [32]byte{0x01})
	tree.Insert(TreeKey{Stem: stem, Subindex: 1}, [32]byte{0x02})

	count := 0
	for range tree.Iter() {
		count++
	}

	assert.Equal(t, 2, count, "Iterator should yield 2 entries")
}

// TestProfileSeparation validates that EIP and JAM profiles produce different roots.
func TestProfileSeparation(t *testing.T) {
	treeEIP := NewUnifiedBinaryTree(Config{Profile: EIPProfile})
	treeJAM := NewUnifiedBinaryTree(Config{Profile: JAMProfile})

	key := TreeKeyFromBytes([32]byte{0x01})
	value := [32]byte{0x42}

	treeEIP.Insert(key, value)
	treeJAM.Insert(key, value)

	rootEIP := treeEIP.RootHash()
	rootJAM := treeJAM.RootHash()

	assert.NotEqual(t, rootEIP, rootJAM, "EIP and JAM profiles must produce different root hashes")
}

// TestBatchInsert validates batch insertion.
func TestBatchInsert(t *testing.T) {
	tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})

	entries := make([]KeyValue, 10)
	for i := 0; i < 10; i++ {
		keyBytes := [32]byte{}
		keyBytes[0] = byte(i)
		entries[i] = KeyValue{
			Key:   TreeKeyFromBytes(keyBytes),
			Value: [32]byte{byte(i + 1)},
		}
	}

	tree.InsertBatch(entries)

	assert.Equal(t, 10, tree.Len(), "Tree should have 10 entries after batch insert")

	// Verify all entries
	for i := 0; i < 10; i++ {
		result, found, _ := tree.Get(&entries[i].Key)
		assert.True(t, found, "Entry %d should be found", i)
		assert.Equal(t, entries[i].Value, result, "Entry %d value should match", i)
	}
}

// TestGetMissing validates Get on missing key returns not found.
func TestGetMissing(t *testing.T) {
	tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})

	keyBytes := [32]byte{0x01}
	key := TreeKeyFromBytes(keyBytes)

	_, found, err := tree.Get(&key)
	require.NoError(t, err, "Get should not error on missing key")
	assert.False(t, found, "Get should return not found for missing key")
}

// TestZeroValueDelete validates that inserting zero value is equivalent to delete.
// Per UBT semantics: zero means absent.
func TestZeroValueDelete(t *testing.T) {
	tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})

	keyBytes := [32]byte{0x01}
	key := TreeKeyFromBytes(keyBytes)

	// Insert non-zero value
	tree.Insert(key, [32]byte{0x42})
	_, found1, _ := tree.Get(&key)
	assert.True(t, found1, "Value should be found after insert")

	// Insert zero value (should delete)
	tree.Insert(key, [32]byte{})
	_, found2, _ := tree.Get(&key)
	assert.False(t, found2, "Zero value insert should delete the entry")
}

// TestStemNodeCount validates stem counting with multiple values per stem.
func TestStemNodeCount(t *testing.T) {
	tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})

	stem := Stem{}

	// Insert 10 values with same stem
	for i := 0; i < 10; i++ {
		key := TreeKey{Stem: stem, Subindex: uint8(i)}
		value := [32]byte{}
		value[0] = byte(i + 1)
		tree.Insert(key, value)
	}

	assert.Equal(t, 1, tree.StemCount(), "Should have 1 stem")
	assert.Equal(t, 10, tree.Len(), "Should have 10 values")
}

// TestEmptyAfterAllDeletes validates that tree is empty after deleting all entries.
func TestEmptyAfterAllDeletes(t *testing.T) {
	tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})

	keys := make([]TreeKey, 5)
	for i := 0; i < 5; i++ {
		keyBytes := [32]byte{}
		keyBytes[0] = byte(i)
		keys[i] = TreeKeyFromBytes(keyBytes)
		tree.Insert(keys[i], [32]byte{byte(i + 1)})
	}

	assert.False(t, tree.IsEmpty(), "Tree should not be empty")

	// Delete all
	for i := 0; i < 5; i++ {
		tree.Delete(&keys[i])
	}

	assert.True(t, tree.IsEmpty(), "Tree should be empty after deleting all")
	assert.Equal(t, [32]byte{}, tree.RootHash(), "Empty tree should have ZERO32 root")
}
