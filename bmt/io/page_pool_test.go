package io

import (
	"runtime"
	"sync"
	"testing"
)

func TestPagePoolAlloc(t *testing.T) {
	pool := NewPagePool()

	page := pool.Alloc()
	if page == nil {
		t.Fatal("Alloc returned nil")
	}
	if len(page) != PageSize {
		t.Errorf("Page size mismatch: got %d, want %d", len(page), PageSize)
	}

	pool.Dealloc(page)
}

func TestPagePoolRecycle(t *testing.T) {
	pool := NewPagePool()

	// Allocate and mark a page
	page1 := pool.Alloc()
	page1[0] = 0x42
	page1[PageSize-1] = 0x43

	// Return it to the pool
	pool.Dealloc(page1)

	// Allocate again - should get the same underlying buffer from sync.Pool
	page2 := pool.Alloc()

	// Verify it's reused (data should still be there from previous use)
	if page2[0] != 0x42 || page2[PageSize-1] != 0x43 {
		t.Log("Note: Page was not reused (sync.Pool may have cleared it)")
	}

	stats := pool.Stats()
	if stats.AllocCount != 2 {
		t.Errorf("AllocCount mismatch: got %d, want 2", stats.AllocCount)
	}
	if stats.DeallocCount != 1 {
		t.Errorf("DeallocCount mismatch: got %d, want 1", stats.DeallocCount)
	}

	pool.Dealloc(page2)
}

func TestPagePoolNoLeaks(t *testing.T) {
	pool := NewPagePool()

	// Allocate many pages
	const numPages = 1000
	pages := make([]Page, numPages)

	for i := 0; i < numPages; i++ {
		pages[i] = pool.Alloc()
		if pages[i] == nil {
			t.Fatalf("Alloc %d returned nil", i)
		}
	}

	// Return all pages
	for i := 0; i < numPages; i++ {
		pool.Dealloc(pages[i])
	}

	// Force GC to clean up
	runtime.GC()

	stats := pool.Stats()
	if stats.AllocCount != numPages {
		t.Errorf("AllocCount mismatch: got %d, want %d", stats.AllocCount, numPages)
	}
	if stats.DeallocCount != numPages {
		t.Errorf("DeallocCount mismatch: got %d, want %d", stats.DeallocCount, numPages)
	}
}

func TestPagePoolConcurrent(t *testing.T) {
	pool := NewPagePool()
	const numGoroutines = 20
	const opsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()

			for j := 0; j < opsPerGoroutine; j++ {
				page := pool.Alloc()
				if page == nil {
					t.Error("Alloc returned nil")
					return
				}
				if len(page) != PageSize {
					t.Errorf("Page size mismatch: got %d, want %d", len(page), PageSize)
					return
				}

				// Write some data
				page[0] = byte(j)
				page[PageSize-1] = byte(j)

				// Return to pool
				pool.Dealloc(page)
			}
		}()
	}

	wg.Wait()

	stats := pool.Stats()
	expectedOps := int64(numGoroutines * opsPerGoroutine)
	if stats.AllocCount != expectedOps {
		t.Errorf("AllocCount mismatch: got %d, want %d", stats.AllocCount, expectedOps)
	}
	if stats.DeallocCount != expectedOps {
		t.Errorf("DeallocCount mismatch: got %d, want %d", stats.DeallocCount, expectedOps)
	}

	t.Logf("Concurrent test passed: %d allocs, %d deallocs, %d reuses",
		stats.AllocCount, stats.DeallocCount, stats.ReuseCount)
}

func TestFatPage(t *testing.T) {
	pool := NewPagePool()

	fatPage := pool.AllocFatPage()
	if fatPage == nil {
		t.Fatal("AllocFatPage returned nil")
	}

	data := fatPage.Data()
	if len(data) != PageSize {
		t.Errorf("FatPage size mismatch: got %d, want %d", len(data), PageSize)
	}

	// Write some data
	data[0] = 0x42
	data[PageSize-1] = 0x43

	// Clone the page
	cloned := fatPage.Clone()
	if cloned.Data()[0] != 0x42 || cloned.Data()[PageSize-1] != 0x43 {
		t.Error("Cloned page data mismatch")
	}

	// Release both pages
	fatPage.Release()
	cloned.Release()

	stats := pool.Stats()
	if stats.DeallocCount != 2 {
		t.Errorf("DeallocCount mismatch: got %d, want 2", stats.DeallocCount)
	}
}

func BenchmarkPagePoolAlloc(b *testing.B) {
	pool := NewPagePool()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		page := pool.Alloc()
		pool.Dealloc(page)
	}
}

func BenchmarkPagePoolConcurrent(b *testing.B) {
	pool := NewPagePool()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			page := pool.Alloc()
			pool.Dealloc(page)
		}
	})
}
