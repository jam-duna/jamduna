package allocator

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	"github.com/jam-duna/jamduna/bmt/io"
)

// Store is a page allocator that manages page allocation with a free list.
// It provides Copy-on-Write (CoW) semantics for the B-tree.
type Store struct {
	file         *os.File
	pagePool     *io.PagePool
	freeList     *FreeList
	nextPage     atomic.Uint64
	mu           sync.RWMutex
	pageCache    map[PageNumber]io.Page // Cache for hot pages (random eviction when full)
	maxCacheSize int                    // Maximum 1000 pages (16 MB)
}

// NewStore creates a new Store.
func NewStore(file *os.File, pagePool *io.PagePool) *Store {
	return &Store{
		file:         file,
		pagePool:     pagePool,
		freeList:     NewFreeList(),
		pageCache:    make(map[PageNumber]io.Page),
		maxCacheSize: 1000, // Cache up to 1000 pages
	}
}

// AllocPage allocates a new page.
// It first tries to reuse a page from the free list,
// otherwise allocates a new page at the end of the file.
func (s *Store) AllocPage() PageNumber {
	// Try to get a page from the free list
	page := s.freeList.Pop()
	if page.IsValid() {
		return page
	}

	// Allocate a new page
	pageNum := PageNumber(s.nextPage.Add(1) - 1)
	return pageNum
}

// FreePage returns a page to the free list for reuse.
func (s *Store) FreePage(page PageNumber) {
	if !page.IsValid() {
		return
	}

	s.freeList.Push(page)

	// Remove from cache and return page to pool
	s.mu.Lock()
	if cached, ok := s.pageCache[page]; ok {
		s.pagePool.Dealloc(cached) // Return to pool before deleting
		delete(s.pageCache, page)
	}
	s.mu.Unlock()
}

// ReadPage reads a page from disk.
func (s *Store) ReadPage(page PageNumber) (io.Page, error) {
	if !page.IsValid() {
		return nil, fmt.Errorf("invalid page number")
	}

	// Check cache first
	s.mu.RLock()
	if cached, ok := s.pageCache[page]; ok {
		s.mu.RUnlock()
		// Return a copy to prevent mutation
		pageCopy := s.pagePool.Alloc()
		copy(pageCopy, cached)
		return pageCopy, nil
	}
	s.mu.RUnlock()

	// Read from disk
	pageData, err := io.ReadPage(s.file, s.pagePool, uint64(page))
	if err != nil {
		return nil, fmt.Errorf("failed to read page %d: %w", page, err)
	}

	// Add to cache (with eviction if needed)
	s.mu.Lock()
	if len(s.pageCache) >= s.maxCacheSize {
		// Simple eviction: remove a random page
		for k := range s.pageCache {
			s.pagePool.Dealloc(s.pageCache[k])
			delete(s.pageCache, k)
			break
		}
	}
	// Cache a copy
	cachedCopy := s.pagePool.Alloc()
	copy(cachedCopy, pageData)
	s.pageCache[page] = cachedCopy
	s.mu.Unlock()

	return pageData, nil
}

// WritePage writes a page to disk.
func (s *Store) WritePage(page PageNumber, data io.Page) error {
	if !page.IsValid() {
		return fmt.Errorf("invalid page number")
	}

	if len(data) != io.PageSize {
		return fmt.Errorf("invalid page size: %d", len(data))
	}

	// Write to disk
	err := io.WritePage(s.file, data, uint64(page))
	if err != nil {
		return fmt.Errorf("failed to write page %d: %w", page, err)
	}

	// Update cache
	s.mu.Lock()
	// Check if page is already cached
	if old, ok := s.pageCache[page]; ok {
		// Update existing cached entry unconditionally
		copy(old, data)
	} else if len(s.pageCache) < s.maxCacheSize {
		// Add new entry if there's space
		cachedCopy := s.pagePool.Alloc()
		copy(cachedCopy, data)
		s.pageCache[page] = cachedCopy
	} else {
		// Cache is full and page not cached - evict one entry and insert
		for k := range s.pageCache {
			s.pagePool.Dealloc(s.pageCache[k])
			delete(s.pageCache, k)
			break
		}
		cachedCopy := s.pagePool.Alloc()
		copy(cachedCopy, data)
		s.pageCache[page] = cachedCopy
	}
	s.mu.Unlock()

	return nil
}

// Sync synchronizes all cached data to disk.
func (s *Store) Sync() error {
	return s.file.Sync()
}

// NumFreePages returns the number of pages in the free list.
func (s *Store) NumFreePages() int {
	return s.freeList.Len()
}

// NextPageNumber returns the next page number that would be allocated.
func (s *Store) NextPageNumber() PageNumber {
	return PageNumber(s.nextPage.Load())
}

// Close closes the store and releases resources.
func (s *Store) Close() error {
	// Clear cache
	s.mu.Lock()
	for k, v := range s.pageCache {
		s.pagePool.Dealloc(v)
		delete(s.pageCache, k)
	}
	s.mu.Unlock()

	return s.file.Close()
}

// File returns the underlying file for this store.
// Returns nil if the store was created without a file (e.g., for testing).
func (s *Store) File() *os.File {
	return s.file
}

// PagePool returns the page pool used by this store.
func (s *Store) PagePool() *io.PagePool {
	return s.pagePool
}
