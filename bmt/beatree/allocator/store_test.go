package allocator

import (
	"os"
	"testing"

	"github.com/jam-duna/jamduna/bmt/io"
)

// TestStoreFreePagDeallocatesCache verifies that FreePage returns cached pages to the pool.
// This tests the fix for Bug #1: FreePage must dealloc cached pages before deleting.
func TestStoreFreePageDeallocatesCache(t *testing.T) {
	// Create temporary file
	tmpFile, err := os.CreateTemp("", "store_test_*.dat")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	pagePool := io.NewPagePool()
	store := NewStore(tmpFile, pagePool)

	// Allocate a page
	pageNum := store.AllocPage()

	// Write to the page (this will cache it)
	pageData := pagePool.Alloc()
	for i := range pageData {
		pageData[i] = byte(i % 256)
	}
	err = store.WritePage(pageNum, pageData)
	if err != nil {
		t.Fatalf("WritePage failed: %v", err)
	}
	pagePool.Dealloc(pageData)

	// Get initial pool stats
	statsBeforeFree := pagePool.Stats()

	// Free the page (should dealloc cached copy)
	store.FreePage(pageNum)

	// Get stats after free
	statsAfterFree := pagePool.Stats()

	// Verify that deallocation happened
	if statsAfterFree.DeallocCount <= statsBeforeFree.DeallocCount {
		t.Errorf("FreePage did not deallocate cached page: dealloc count before=%d, after=%d",
			statsBeforeFree.DeallocCount, statsAfterFree.DeallocCount)
	}

	// Verify page is removed from cache
	store.mu.RLock()
	_, cached := store.pageCache[pageNum]
	store.mu.RUnlock()

	if cached {
		t.Error("Page should not be in cache after FreePage")
	}
}

// TestStoreWritePageUpdatesCache verifies that WritePage updates cached entries unconditionally.
// This tests the fix for Bug #2: WritePage must update cache even when full.
func TestStoreWritePageUpdatesCache(t *testing.T) {
	// Create temporary file
	tmpFile, err := os.CreateTemp("", "store_test_*.dat")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	pagePool := io.NewPagePool()
	store := NewStore(tmpFile, pagePool)
	store.maxCacheSize = 5 // Small cache to test full condition

	// Fill the cache with pages
	pages := make([]PageNumber, 5)
	for i := 0; i < 5; i++ {
		pages[i] = store.AllocPage()
		pageData := pagePool.Alloc()
		for j := range pageData {
			pageData[j] = byte(i)
		}
		err := store.WritePage(pages[i], pageData)
		if err != nil {
			t.Fatalf("WritePage failed: %v", err)
		}
		pagePool.Dealloc(pageData)
	}

	// Verify cache is full
	store.mu.RLock()
	cacheSize := len(store.pageCache)
	store.mu.RUnlock()

	if cacheSize != 5 {
		t.Fatalf("Expected cache size 5, got %d", cacheSize)
	}

	// Update an existing cached page with new data
	updatePageNum := pages[2]
	newData := pagePool.Alloc()
	for i := range newData {
		newData[i] = 0xFF // Different from original (byte(2))
	}

	err = store.WritePage(updatePageNum, newData)
	if err != nil {
		t.Fatalf("WritePage update failed: %v", err)
	}
	pagePool.Dealloc(newData)

	// Read from cache and verify it has new data
	store.mu.RLock()
	cached, ok := store.pageCache[updatePageNum]
	store.mu.RUnlock()

	if !ok {
		t.Fatal("Page should still be in cache after update")
	}

	// Verify cached data matches new data (not old data)
	if cached[0] != 0xFF {
		t.Errorf("Cache was not updated: expected 0xFF, got 0x%02X", cached[0])
	}
	if cached[100] != 0xFF {
		t.Errorf("Cache was not updated: expected 0xFF at offset 100, got 0x%02X", cached[100])
	}
}

// TestStoreWritePageEvictsWhenFull verifies that WritePage evicts when cache is full.
func TestStoreWritePageEvictsWhenFull(t *testing.T) {
	// Create temporary file
	tmpFile, err := os.CreateTemp("", "store_test_*.dat")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	pagePool := io.NewPagePool()
	store := NewStore(tmpFile, pagePool)
	store.maxCacheSize = 3 // Small cache

	// Fill the cache
	for i := 0; i < 3; i++ {
		pageNum := store.AllocPage()
		pageData := pagePool.Alloc()
		err := store.WritePage(pageNum, pageData)
		if err != nil {
			t.Fatalf("WritePage failed: %v", err)
		}
		pagePool.Dealloc(pageData)
	}

	// Cache should be full
	store.mu.RLock()
	cacheSize := len(store.pageCache)
	store.mu.RUnlock()

	if cacheSize != 3 {
		t.Fatalf("Expected cache size 3, got %d", cacheSize)
	}

	// Write a new page (not in cache, cache is full)
	newPageNum := store.AllocPage()
	newPageData := pagePool.Alloc()
	for i := range newPageData {
		newPageData[i] = 0xAA
	}

	err = store.WritePage(newPageNum, newPageData)
	if err != nil {
		t.Fatalf("WritePage failed: %v", err)
	}

	// Verify new page is now in cache
	store.mu.RLock()
	_, ok := store.pageCache[newPageNum]
	finalCacheSize := len(store.pageCache)
	store.mu.RUnlock()

	if !ok {
		t.Error("New page should be in cache after eviction")
	}

	if finalCacheSize != 3 {
		t.Errorf("Cache size should remain 3 after eviction, got %d", finalCacheSize)
	}

	// Read the page and verify data
	readData, err := store.ReadPage(newPageNum)
	if err != nil {
		t.Fatalf("ReadPage failed: %v", err)
	}

	if readData[0] != 0xAA {
		t.Errorf("Expected 0xAA, got 0x%02X", readData[0])
	}

	pagePool.Dealloc(readData)
	pagePool.Dealloc(newPageData)
}

// TestStoreReadWriteRoundTrip verifies basic read/write functionality.
func TestStoreReadWriteRoundTrip(t *testing.T) {
	// Create temporary file
	tmpFile, err := os.CreateTemp("", "store_test_*.dat")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	pagePool := io.NewPagePool()
	store := NewStore(tmpFile, pagePool)

	// Allocate a page
	pageNum := store.AllocPage()

	// Create test data
	writeData := pagePool.Alloc()
	for i := range writeData {
		writeData[i] = byte(i % 256)
	}

	// Write
	err = store.WritePage(pageNum, writeData)
	if err != nil {
		t.Fatalf("WritePage failed: %v", err)
	}

	// Read
	readData, err := store.ReadPage(pageNum)
	if err != nil {
		t.Fatalf("ReadPage failed: %v", err)
	}

	// Verify
	for i := range readData {
		if readData[i] != writeData[i] {
			t.Errorf("Data mismatch at offset %d: expected 0x%02X, got 0x%02X",
				i, writeData[i], readData[i])
			break
		}
	}

	pagePool.Dealloc(writeData)
	pagePool.Dealloc(readData)
}

// TestStoreFreeListIntegration verifies free list integration.
func TestStoreFreeListIntegration(t *testing.T) {
	// Create temporary file
	tmpFile, err := os.CreateTemp("", "store_test_*.dat")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	pagePool := io.NewPagePool()
	store := NewStore(tmpFile, pagePool)

	// Allocate pages
	page1 := store.AllocPage()
	page2 := store.AllocPage()
	page3 := store.AllocPage()

	if page1 == page2 || page2 == page3 || page1 == page3 {
		t.Error("AllocPage returned duplicate page numbers")
	}

	// Free some pages
	store.FreePage(page2)

	// Next allocation should reuse page2
	page4 := store.AllocPage()
	if page4 != page2 {
		t.Errorf("Expected to reuse page %d, got %d", page2, page4)
	}

	// Free multiple pages
	store.FreePage(page1)
	store.FreePage(page3)

	if store.NumFreePages() != 2 {
		t.Errorf("Expected 2 free pages, got %d", store.NumFreePages())
	}
}
