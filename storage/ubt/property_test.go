package ubt

import (
	"math/rand"
	"testing"
)

// isNonZero checks if a value is non-zero (required for UBT "zero means absent" semantics).
func isNonZero(v [32]byte) bool {
	for _, b := range v {
		if b != 0 {
			return true
		}
	}
	return false
}

func randBytes32(r *rand.Rand) [32]byte {
	var out [32]byte
	for i := range out {
		out[i] = byte(r.Intn(256))
	}
	return out
}

func randBytes64(r *rand.Rand) [64]byte {
	var out [64]byte
	for i := range out {
		out[i] = byte(r.Intn(256))
	}
	return out
}

// TestPropGetInsertSame verifies that get after insert returns the inserted value.
func TestPropGetInsertSame(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 200; i++ {
		keyBytes := randBytes32(r)
		valueBytes := randBytes32(r)
		if !isNonZero(valueBytes) {
			continue
		}

		tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})
		key := TreeKeyFromBytes(keyBytes)
		tree.Insert(key, valueBytes)

		result, found, err := tree.Get(&key)
		if err != nil || !found || result != valueBytes {
			t.Fatalf("get after insert mismatch")
		}
	}
}

// TestPropGetInsertOther verifies that inserting a key doesn't affect other keys.
func TestPropGetInsertOther(t *testing.T) {
	r := rand.New(rand.NewSource(2))
	for i := 0; i < 200; i++ {
		k1 := randBytes32(r)
		k2 := randBytes32(r)
		v1 := randBytes32(r)
		v2 := randBytes32(r)
		if k1 == k2 || !isNonZero(v1) || !isNonZero(v2) {
			continue
		}

		tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})
		key1 := TreeKeyFromBytes(k1)
		key2 := TreeKeyFromBytes(k2)

		tree.Insert(key1, v1)
		tree.Insert(key2, v2)

		r1, found1, err1 := tree.Get(&key1)
		r2, found2, err2 := tree.Get(&key2)

		if err1 != nil || !found1 || r1 != v1 || err2 != nil || !found2 || r2 != v2 {
			t.Fatalf("insert should not affect other keys")
		}
	}
}

// TestPropInsertDelete verifies that insert then delete returns not found.
func TestPropInsertDelete(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	for i := 0; i < 200; i++ {
		keyBytes := randBytes32(r)
		valueBytes := randBytes32(r)
		if !isNonZero(valueBytes) {
			continue
		}

		tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})
		key := TreeKeyFromBytes(keyBytes)

		tree.Insert(key, valueBytes)
		tree.Delete(&key)

		_, found, err := tree.Get(&key)
		if err != nil || found {
			t.Fatalf("expected delete to remove key")
		}
	}
}

// TestPropHashDeterminism verifies root hash is order-independent.
func TestPropHashDeterminism(t *testing.T) {
	r := rand.New(rand.NewSource(4))
	for i := 0; i < 100; i++ {
		n := r.Intn(50)
		entries := make([][64]byte, n)
		for j := 0; j < n; j++ {
			entries[j] = randBytes64(r)
		}

		tree1 := NewUnifiedBinaryTree(Config{Profile: EIPProfile})
		tree2 := NewUnifiedBinaryTree(Config{Profile: EIPProfile})

		for _, entry := range entries {
			var keyBytes, value [32]byte
			copy(keyBytes[:], entry[:32])
			copy(value[:], entry[32:])
			if !isNonZero(value) {
				continue
			}
			key := TreeKeyFromBytes(keyBytes)
			tree1.Insert(key, value)
		}

		for j := len(entries) - 1; j >= 0; j-- {
			var keyBytes, value [32]byte
			copy(keyBytes[:], entries[j][:32])
			copy(value[:], entries[j][32:])
			if !isNonZero(value) {
				continue
			}
			key := TreeKeyFromBytes(keyBytes)
			tree2.Insert(key, value)
		}

		if tree1.RootHash() != tree2.RootHash() {
			t.Fatalf("root hash should be order-independent")
		}
	}
}

// TestPropGetFromEmptyTree verifies that get from empty tree returns not found.
func TestPropGetFromEmptyTree(t *testing.T) {
	r := rand.New(rand.NewSource(5))
	for i := 0; i < 200; i++ {
		keyBytes := randBytes32(r)
		key := TreeKeyFromBytes(keyBytes)

		tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})
		_, found, err := tree.Get(&key)
		if err != nil || found {
			t.Fatalf("expected empty tree get to return not found")
		}
	}
}
