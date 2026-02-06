package bitbox

import (
	"testing"

	"github.com/jam-duna/jamduna/bmt/core"
)

func TestHashPageId(t *testing.T) {
	// Create a PageId (root node)
	pageId, err := core.NewPageId([]byte{})
	if err != nil {
		t.Fatalf("Failed to create root PageId: %v", err)
	}

	// Hash it
	seed := uint64(0x123456789ABCDEF0)
	hash := HashPageId(*pageId, seed)

	// Should produce a non-zero hash (though 0 is technically possible)
	// Just verify it returns a uint64
	_ = hash

	// Create another PageId (with path)
	pageId2, err := core.NewPageId([]byte{1, 2, 3})
	if err != nil {
		t.Fatalf("Failed to create PageId with path: %v", err)
	}
	hash2 := HashPageId(*pageId2, seed)

	// Different PageIds should produce different hashes (very likely)
	if hash == hash2 {
		t.Errorf("Different PageIds should produce different hashes")
	}

	// Hash should be a full uint64 (can be any value)
	if hash > ^uint64(0) {
		t.Errorf("Hash should fit in uint64")
	}
}

func TestHashConsistency(t *testing.T) {
	// Same input should always produce same hash
	pageId, err := core.NewPageId([]byte{5, 10, 15})
	if err != nil {
		t.Fatalf("Failed to create PageId: %v", err)
	}
	seed := uint64(0xDEADBEEF)

	hash1 := HashPageId(*pageId, seed)
	hash2 := HashPageId(*pageId, seed)
	hash3 := HashPageId(*pageId, seed)

	if hash1 != hash2 || hash2 != hash3 {
		t.Errorf("Same PageId should produce consistent hash: %x, %x, %x", hash1, hash2, hash3)
	}

	// Different seeds should produce different hashes
	seed2 := uint64(0xCAFEBABE)
	hash4 := HashPageId(*pageId, seed2)

	if hash1 == hash4 {
		t.Errorf("Different seeds should produce different hashes")
	}

	// Test with multiple PageIds
	testCases := []struct {
		path []byte
		seed uint64
	}{
		{[]byte{}, 0},
		{[]byte{0}, 1},
		{[]byte{1}, 0},
		{[]byte{0, 0, 0}, 12345},
		{[]byte{63, 63, 63}, 67890},
		{[]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, 0xFFFFFFFFFFFFFFFF},
	}

	for _, tc := range testCases {
		pageId, err := core.NewPageId(tc.path)
		if err != nil {
			t.Fatalf("Failed to create PageId for path %v: %v", tc.path, err)
		}
		h1 := HashPageId(*pageId, tc.seed)
		h2 := HashPageId(*pageId, tc.seed)

		if h1 != h2 {
			t.Errorf("Inconsistent hash for path %v, seed %x: %x vs %x", tc.path, tc.seed, h1, h2)
		}
	}
}

func TestHashDistribution(t *testing.T) {
	// Test that hashes are well-distributed
	// Create many different PageIds and verify hashes spread across range
	seed := uint64(42)
	hashSet := make(map[uint64]bool)

	// Create PageIds with valid child indices (0-63)
	for i := 0; i < 64; i++ {
		path := []byte{byte(i)}
		pageId, err := core.NewPageId(path)
		if err != nil {
			t.Fatalf("Failed to create PageId for i=%d: %v", i, err)
		}
		hash := HashPageId(*pageId, seed)
		hashSet[hash] = true
	}

	// Also test with longer paths
	for i := 0; i < 36; i++ {
		path := []byte{byte(i % 64), byte((i + 10) % 64)}
		pageId, err := core.NewPageId(path)
		if err != nil {
			t.Fatalf("Failed to create PageId for i=%d: %v", i, err)
		}
		hash := HashPageId(*pageId, seed)
		hashSet[hash] = true
	}

	// Should have 100 unique hashes (no collisions for these simple inputs)
	if len(hashSet) != 100 {
		t.Errorf("Expected 100 unique hashes, got %d (some collisions occurred)", len(hashSet))
	}

	// Check that hashes use the full range (not clustered in one area)
	// Divide uint64 range into 8 buckets and check each has some hashes
	buckets := make([]int, 8)
	for hash := range hashSet {
		bucket := hash / (^uint64(0) / 8)
		if bucket >= 8 {
			bucket = 7 // Handle edge case
		}
		buckets[bucket]++
	}

	// At least 4 of the 8 buckets should have some hashes (reasonable distribution)
	nonEmptyBuckets := 0
	for _, count := range buckets {
		if count > 0 {
			nonEmptyBuckets++
		}
	}

	if nonEmptyBuckets < 4 {
		t.Errorf("Hash distribution too narrow: only %d/8 buckets used: %v", nonEmptyBuckets, buckets)
	}
}
