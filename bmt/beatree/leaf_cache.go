package beatree

import (
	"container/list"
	"sync"

	"github.com/colorfulnotion/jam/bmt/beatree/allocator"
)

// LeafCache is an LRU cache for leaf pages.
//
// This caches recently accessed leaf pages to avoid redundant disk I/O.
// Uses simple LRU with mutex synchronization.
//
// The Rust NOMT version uses more sophisticated concurrency primitives.
// Future: Upgrade to lock-free structures if performance bottleneck.
type LeafCache struct {
	mu       sync.Mutex
	capacity int
	cache    map[allocator.PageNumber]*list.Element
	lru      *list.List // Least recently used list (front = oldest, back = newest)
}

// cacheEntry represents an entry in the cache.
type cacheEntry struct {
	pageNum allocator.PageNumber
	data    []byte
}

// NewLeafCache creates a new leaf cache with the given capacity.
// Capacity is the maximum number of pages to cache.
func NewLeafCache(capacity int) *LeafCache {
	if capacity <= 0 {
		panic("cache capacity must be positive")
	}

	return &LeafCache{
		capacity: capacity,
		cache:    make(map[allocator.PageNumber]*list.Element),
		lru:      list.New(),
	}
}

// Get retrieves a page from the cache.
// Returns (data, true) if found, (nil, false) if not in cache.
func (lc *LeafCache) Get(pageNum allocator.PageNumber) ([]byte, bool) {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	elem, exists := lc.cache[pageNum]
	if !exists {
		return nil, false
	}

	// Move to back (most recently used)
	lc.lru.MoveToBack(elem)

	entry := elem.Value.(*cacheEntry)
	return entry.data, true
}

// Put inserts a page into the cache.
// If the cache is full, evicts the least recently used page.
func (lc *LeafCache) Put(pageNum allocator.PageNumber, data []byte) {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	// Check if already exists
	if elem, exists := lc.cache[pageNum]; exists {
		// Update data and move to back
		lc.lru.MoveToBack(elem)
		entry := elem.Value.(*cacheEntry)
		entry.data = data
		return
	}

	// Check if we need to evict
	if lc.lru.Len() >= lc.capacity {
		// Evict least recently used (front of list)
		oldest := lc.lru.Front()
		if oldest != nil {
			lc.lru.Remove(oldest)
			entry := oldest.Value.(*cacheEntry)
			delete(lc.cache, entry.pageNum)
		}
	}

	// Add new entry
	entry := &cacheEntry{
		pageNum: pageNum,
		data:    data,
	}
	elem := lc.lru.PushBack(entry)
	lc.cache[pageNum] = elem
}

// Remove removes a page from the cache.
// Returns true if the page was in the cache.
func (lc *LeafCache) Remove(pageNum allocator.PageNumber) bool {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	elem, exists := lc.cache[pageNum]
	if !exists {
		return false
	}

	lc.lru.Remove(elem)
	delete(lc.cache, pageNum)
	return true
}

// Clear removes all entries from the cache.
func (lc *LeafCache) Clear() {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	lc.cache = make(map[allocator.PageNumber]*list.Element)
	lc.lru = list.New()
}

// Len returns the number of pages currently in the cache.
func (lc *LeafCache) Len() int {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	return lc.lru.Len()
}

// Capacity returns the maximum number of pages the cache can hold.
func (lc *LeafCache) Capacity() int {
	return lc.capacity
}
