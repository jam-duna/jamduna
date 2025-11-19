package beatree

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/colorfulnotion/jam/bmt/beatree/allocator"
)

// TestLeafCacheBasic tests basic cache operations.
func TestLeafCacheBasic(t *testing.T) {
	cache := NewLeafCache(3)

	if cache.Len() != 0 {
		t.Errorf("New cache should be empty, len=%d", cache.Len())
	}

	if cache.Capacity() != 3 {
		t.Errorf("Expected capacity 3, got %d", cache.Capacity())
	}

	// Get from empty cache
	_, found := cache.Get(allocator.PageNumber(1))
	if found {
		t.Error("Get from empty cache should return false")
	}

	// Put and get
	data1 := []byte("page1")
	cache.Put(allocator.PageNumber(1), data1)

	if cache.Len() != 1 {
		t.Errorf("Expected len 1, got %d", cache.Len())
	}

	retrieved, found := cache.Get(allocator.PageNumber(1))
	if !found {
		t.Fatal("Expected to find page 1")
	}

	if !bytes.Equal(retrieved, data1) {
		t.Errorf("Expected %v, got %v", data1, retrieved)
	}
}

// TestLeafCacheUpdate tests updating existing entries.
func TestLeafCacheUpdate(t *testing.T) {
	cache := NewLeafCache(3)

	data1 := []byte("old")
	cache.Put(allocator.PageNumber(1), data1)

	data2 := []byte("new")
	cache.Put(allocator.PageNumber(1), data2)

	// Should still have only 1 entry
	if cache.Len() != 1 {
		t.Errorf("Update should not increase len, got %d", cache.Len())
	}

	// Should return new data
	retrieved, _ := cache.Get(allocator.PageNumber(1))
	if !bytes.Equal(retrieved, data2) {
		t.Errorf("Expected updated data %v, got %v", data2, retrieved)
	}
}

// TestLeafCacheEviction tests LRU eviction.
func TestLeafCacheEviction(t *testing.T) {
	cache := NewLeafCache(3)

	// Fill cache
	cache.Put(allocator.PageNumber(1), []byte("page1"))
	cache.Put(allocator.PageNumber(2), []byte("page2"))
	cache.Put(allocator.PageNumber(3), []byte("page3"))

	if cache.Len() != 3 {
		t.Fatalf("Expected len 3, got %d", cache.Len())
	}

	// Add one more - should evict page 1 (least recently used)
	cache.Put(allocator.PageNumber(4), []byte("page4"))

	if cache.Len() != 3 {
		t.Errorf("Cache should stay at capacity 3, got %d", cache.Len())
	}

	// Page 1 should be evicted
	_, found := cache.Get(allocator.PageNumber(1))
	if found {
		t.Error("Page 1 should have been evicted")
	}

	// Pages 2, 3, 4 should still be present
	for _, pn := range []uint64{2, 3, 4} {
		_, found := cache.Get(allocator.PageNumber(pn))
		if !found {
			t.Errorf("Page %d should still be in cache", pn)
		}
	}
}

// TestLeafCacheLRUOrder tests that access updates LRU order.
func TestLeafCacheLRUOrder(t *testing.T) {
	cache := NewLeafCache(3)

	// Fill cache: 1, 2, 3 (in that order)
	cache.Put(allocator.PageNumber(1), []byte("page1"))
	cache.Put(allocator.PageNumber(2), []byte("page2"))
	cache.Put(allocator.PageNumber(3), []byte("page3"))

	// Access page 1 - should move it to most recently used
	cache.Get(allocator.PageNumber(1))

	// Add page 4 - should evict page 2 (now least recently used)
	cache.Put(allocator.PageNumber(4), []byte("page4"))

	// Page 2 should be evicted
	_, found := cache.Get(allocator.PageNumber(2))
	if found {
		t.Error("Page 2 should have been evicted")
	}

	// Page 1 should still be present (was accessed recently)
	_, found = cache.Get(allocator.PageNumber(1))
	if !found {
		t.Error("Page 1 should still be in cache")
	}
}

// TestLeafCacheRemove tests manual removal.
func TestLeafCacheRemove(t *testing.T) {
	cache := NewLeafCache(3)

	cache.Put(allocator.PageNumber(1), []byte("page1"))
	cache.Put(allocator.PageNumber(2), []byte("page2"))

	if cache.Len() != 2 {
		t.Fatalf("Expected len 2, got %d", cache.Len())
	}

	// Remove page 1
	removed := cache.Remove(allocator.PageNumber(1))
	if !removed {
		t.Error("Remove should return true for existing page")
	}

	if cache.Len() != 1 {
		t.Errorf("Expected len 1 after remove, got %d", cache.Len())
	}

	// Verify page 1 is gone
	_, found := cache.Get(allocator.PageNumber(1))
	if found {
		t.Error("Page 1 should be removed")
	}

	// Remove non-existent page
	removed = cache.Remove(allocator.PageNumber(99))
	if removed {
		t.Error("Remove should return false for non-existent page")
	}
}

// TestLeafCacheClear tests clearing the cache.
func TestLeafCacheClear(t *testing.T) {
	cache := NewLeafCache(3)

	// Fill cache
	for i := 1; i <= 3; i++ {
		cache.Put(allocator.PageNumber(uint64(i)), []byte(fmt.Sprintf("page%d", i)))
	}

	if cache.Len() != 3 {
		t.Fatalf("Expected len 3, got %d", cache.Len())
	}

	// Clear
	cache.Clear()

	if cache.Len() != 0 {
		t.Errorf("Expected len 0 after clear, got %d", cache.Len())
	}

	// All pages should be gone
	for i := 1; i <= 3; i++ {
		_, found := cache.Get(allocator.PageNumber(uint64(i)))
		if found {
			t.Errorf("Page %d should be cleared", i)
		}
	}
}

// TestLeafCacheSingleEntry tests cache with capacity 1.
func TestLeafCacheSingleEntry(t *testing.T) {
	cache := NewLeafCache(1)

	// Add first page
	cache.Put(allocator.PageNumber(1), []byte("page1"))

	_, found := cache.Get(allocator.PageNumber(1))
	if !found {
		t.Error("Page 1 should be in cache")
	}

	// Add second page - should immediately evict first
	cache.Put(allocator.PageNumber(2), []byte("page2"))

	if cache.Len() != 1 {
		t.Errorf("Cache should have len 1, got %d", cache.Len())
	}

	_, found = cache.Get(allocator.PageNumber(1))
	if found {
		t.Error("Page 1 should be evicted")
	}

	_, found = cache.Get(allocator.PageNumber(2))
	if !found {
		t.Error("Page 2 should be in cache")
	}
}

// TestLeafCacheZeroCapacityPanics tests that zero capacity panics.
func TestLeafCacheZeroCapacityPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic for zero capacity")
		}
	}()

	NewLeafCache(0)
}

// TestLeafCacheNegativeCapacityPanics tests that negative capacity panics.
func TestLeafCacheNegativeCapacityPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic for negative capacity")
		}
	}()

	NewLeafCache(-1)
}

// TestLeafCacheMultipleAccesses tests repeated accesses.
func TestLeafCacheMultipleAccesses(t *testing.T) {
	cache := NewLeafCache(2)

	data1 := []byte("page1")
	cache.Put(allocator.PageNumber(1), data1)

	// Access multiple times
	for i := 0; i < 5; i++ {
		retrieved, found := cache.Get(allocator.PageNumber(1))
		if !found {
			t.Fatalf("Access %d: page 1 should be in cache", i)
		}
		if !bytes.Equal(retrieved, data1) {
			t.Errorf("Access %d: data mismatch", i)
		}
	}

	// Should still have len 1
	if cache.Len() != 1 {
		t.Errorf("Expected len 1, got %d", cache.Len())
	}
}

// TestLeafCacheAccessPattern tests realistic access pattern.
func TestLeafCacheAccessPattern(t *testing.T) {
	cache := NewLeafCache(3)

	// Simulate sequential access
	for i := 1; i <= 5; i++ {
		data := []byte(fmt.Sprintf("page%d", i))
		cache.Put(allocator.PageNumber(uint64(i)), data)
	}

	// Cache should have pages 3, 4, 5 (last 3)
	for i := 1; i <= 2; i++ {
		_, found := cache.Get(allocator.PageNumber(uint64(i)))
		if found {
			t.Errorf("Page %d should be evicted", i)
		}
	}

	for i := 3; i <= 5; i++ {
		_, found := cache.Get(allocator.PageNumber(uint64(i)))
		if !found {
			t.Errorf("Page %d should be in cache", i)
		}
	}

	// Access page 3 again (make it most recent)
	cache.Get(allocator.PageNumber(3))

	// Add page 6 - should evict page 4
	cache.Put(allocator.PageNumber(6), []byte("page6"))

	_, found := cache.Get(allocator.PageNumber(4))
	if found {
		t.Error("Page 4 should be evicted")
	}

	// Page 3 should still be there
	_, found = cache.Get(allocator.PageNumber(3))
	if !found {
		t.Error("Page 3 should still be in cache")
	}
}

// TestLeafCacheLargeData tests caching large page data.
func TestLeafCacheLargeData(t *testing.T) {
	cache := NewLeafCache(2)

	// Create 16KB page data
	largeData := make([]byte, 16384)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	cache.Put(allocator.PageNumber(1), largeData)

	retrieved, found := cache.Get(allocator.PageNumber(1))
	if !found {
		t.Fatal("Large page should be in cache")
	}

	if !bytes.Equal(retrieved, largeData) {
		t.Error("Large page data corrupted")
	}
}
