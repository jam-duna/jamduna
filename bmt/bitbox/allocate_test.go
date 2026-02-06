package bitbox

import (
	"testing"

	"github.com/jam-duna/jamduna/bmt/core"
)

func TestBucketAllocation(t *testing.T) {
	m := NewMetaMap(1000)
	seed := uint64(42)

	// Allocate a bucket for a PageId
	pageId, err := core.NewPageId([]byte{1, 2, 3})
	if err != nil {
		t.Fatalf("Failed to create PageId: %v", err)
	}

	bucket, err := AllocateBucket(*pageId, seed, m)
	if err != nil {
		t.Fatalf("Failed to allocate bucket: %v", err)
	}

	if !bucket.IsValid() {
		t.Errorf("Allocated bucket should be valid")
	}

	// Verify the bucket is marked as full
	if m.HintEmpty(int(bucket)) {
		t.Errorf("Bucket %d should not be empty after allocation", bucket)
	}

	// Allocate another bucket for a different PageId
	pageId2, _ := core.NewPageId([]byte{4, 5, 6})
	bucket2, err := AllocateBucket(*pageId2, seed, m)
	if err != nil {
		t.Fatalf("Failed to allocate second bucket: %v", err)
	}

	if bucket == bucket2 {
		t.Errorf("Different PageIds should get different buckets (unless hash collision)")
	}

	// Verify both buckets are full
	if m.FullCount() < 2 {
		t.Errorf("Should have at least 2 full buckets, got %d", m.FullCount())
	}
}

func TestBucketExhaustion(t *testing.T) {
	// Small map to trigger exhaustion
	m := NewMetaMap(10)
	seed := uint64(123)

	// Fill the map
	for i := 0; i < 20; i++ {
		pageId, _ := core.NewPageId([]byte{byte(i)})
		_, err := AllocateBucket(*pageId, seed, m)
		if err != nil {
			// Should eventually get exhaustion error
			if _, ok := err.(BucketExhaustion); ok {
				// Success - got exhaustion
				return
			}
			t.Fatalf("Unexpected error: %v", err)
		}
	}

	t.Errorf("Should have triggered bucket exhaustion")
}

func TestFreeBucket(t *testing.T) {
	m := NewMetaMap(100)
	seed := uint64(99)

	// Allocate a bucket
	pageId, _ := core.NewPageId([]byte{10})
	bucket, err := AllocateBucket(*pageId, seed, m)
	if err != nil {
		t.Fatalf("Failed to allocate bucket: %v", err)
	}

	// Verify it's full
	if m.HintEmpty(int(bucket)) {
		t.Errorf("Bucket should be full")
	}

	// Free the bucket
	err = FreeBucket(bucket, m)
	if err != nil {
		t.Fatalf("Failed to free bucket: %v", err)
	}

	// Verify it's a tombstone
	if !m.HintTombstone(int(bucket)) {
		t.Errorf("Bucket should be a tombstone after freeing")
	}

	// Verify it's not empty
	if m.HintEmpty(int(bucket)) {
		t.Errorf("Tombstone should not be empty")
	}
}
