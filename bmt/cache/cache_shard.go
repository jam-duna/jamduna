package cache

import (
	"sync"

	"github.com/colorfulnotion/jam/bmt/core"
	"github.com/colorfulnotion/jam/bmt/io"
)

// CacheShard represents a single shard of the page cache with LRU eviction.
// Each shard has its own lock to reduce contention in multi-threaded scenarios.
type CacheShard struct {
	mu sync.RWMutex

	// Cache storage (using encoded PageId as key)
	entries map[[32]byte]*CacheEntry

	// LRU tracking (intrusive doubly-linked list)
	head *CacheEntry // Most recently used
	tail *CacheEntry // Least recently used

	// Configuration
	maxEntries int
	pagePool   *io.PagePool

	// Statistics
	hits   int64
	misses int64
	evictions int64
}

// NewCacheShard creates a new cache shard with the specified capacity.
func NewCacheShard(maxEntries int, pagePool *io.PagePool) *CacheShard {
	if maxEntries <= 0 {
		maxEntries = 100 // Default capacity per shard
	}

	return &CacheShard{
		entries:    make(map[[32]byte]*CacheEntry),
		head:       nil,
		tail:       nil,
		maxEntries: maxEntries,
		pagePool:   pagePool,
		hits:       0,
		misses:     0,
		evictions:  0,
	}
}

// Get retrieves a page from the cache.
// Returns the cache entry and true if found, nil and false otherwise.
func (s *CacheShard) Get(pageId core.PageId) (*CacheEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := pageId.Encode()
	entry, exists := s.entries[key]
	if !exists {
		s.misses++
		return nil, false
	}

	// Move to front of LRU list
	s.moveToFront(entry)
	entry.AddRef()
	s.hits++

	return entry, true
}

// Put adds a page to the cache.
// If the cache is full, evicts the least recently used entry.
// Returns the evicted entry if any.
func (s *CacheShard) Put(pageId core.PageId, page io.Page) (*CacheEntry, *CacheEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if already exists
	key := pageId.Encode()
	if existing, exists := s.entries[key]; exists {
		// Update existing entry and move to front
		if existing.Page != nil {
			s.pagePool.Dealloc(existing.Page)
		}
		existing.Page = page
		s.moveToFront(existing)
		return existing, nil
	}

	// Create new entry
	entry := NewCacheEntry(pageId, page)
	s.entries[key] = entry

	// Add to front of LRU list
	s.addToFront(entry)

	// Check if eviction is needed
	var evicted *CacheEntry
	if len(s.entries) > s.maxEntries {
		evicted = s.evictLRU()
	}

	return entry, evicted
}

// Remove removes a page from the cache.
// Returns the removed entry if it existed.
func (s *CacheShard) Remove(pageId core.PageId) *CacheEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := pageId.Encode()
	entry, exists := s.entries[key]
	if !exists {
		return nil
	}

	// Remove from map
	delete(s.entries, key)

	// Remove from LRU list
	s.removeFromList(entry)

	return entry
}

// Clear removes all entries from the cache.
func (s *CacheShard) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Deallocate all pages
	for _, entry := range s.entries {
		if entry.Page != nil {
			s.pagePool.Dealloc(entry.Page)
		}
	}

	// Clear map and list
	s.entries = make(map[[32]byte]*CacheEntry)
	s.head = nil
	s.tail = nil
}

// Stats returns cache statistics.
func (s *CacheShard) Stats() (hits, misses, evictions int64, size int) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.hits, s.misses, s.evictions, len(s.entries)
}

// evictLRU evicts the least recently used entry.
// Must be called with lock held.
func (s *CacheShard) evictLRU() *CacheEntry {
	if s.tail == nil {
		return nil
	}

	// Can't evict entries that are still referenced
	entry := s.tail
	for entry != nil && entry.RefCount() > 0 {
		entry = entry.prev
	}

	if entry == nil {
		// All entries are referenced, can't evict anything
		return nil
	}

	// Remove from map
	key := entry.PageId.Encode()
	delete(s.entries, key)

	// Remove from LRU list
	s.removeFromList(entry)

	// Deallocate page
	if entry.Page != nil {
		s.pagePool.Dealloc(entry.Page)
		entry.Page = nil
	}

	s.evictions++
	return entry
}

// moveToFront moves an entry to the front of the LRU list.
// Must be called with lock held.
func (s *CacheShard) moveToFront(entry *CacheEntry) {
	if entry == s.head {
		return // Already at front
	}

	// Remove from current position
	s.removeFromList(entry)

	// Add to front
	s.addToFront(entry)
}

// addToFront adds an entry to the front of the LRU list.
// Must be called with lock held.
func (s *CacheShard) addToFront(entry *CacheEntry) {
	entry.next = s.head
	entry.prev = nil

	if s.head != nil {
		s.head.prev = entry
	}
	s.head = entry

	if s.tail == nil {
		s.tail = entry
	}
}

// removeFromList removes an entry from the LRU list.
// Must be called with lock held.
func (s *CacheShard) removeFromList(entry *CacheEntry) {
	if entry.prev != nil {
		entry.prev.next = entry.next
	} else {
		s.head = entry.next
	}

	if entry.next != nil {
		entry.next.prev = entry.prev
	} else {
		s.tail = entry.prev
	}

	entry.prev = nil
	entry.next = nil
}