package merkle

import (
	"sync"

	"github.com/colorfulnotion/jam/bmt/core"
	"github.com/colorfulnotion/jam/bmt/io"
)

// PageSet manages a set of pages for batch operations.
// It provides efficient lookups and batch processing capabilities.
type PageSet struct {
	mu    sync.RWMutex
	pages map[[32]byte]*PageSetEntry
}

// PageSetEntry represents an entry in the page set.
type PageSetEntry struct {
	PageId   core.PageId
	Page     io.Page
	Updates  []PageUpdate
	Priority int
	Dirty    bool
}

// NewPageSet creates a new page set.
func NewPageSet() *PageSet {
	return &PageSet{
		pages: make(map[[32]byte]*PageSetEntry),
	}
}

// Add adds a page to the set with associated updates.
func (ps *PageSet) Add(pageId core.PageId, page io.Page, updates []PageUpdate, priority int) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	key := pageId.Encode()
	ps.pages[key] = &PageSetEntry{
		PageId:   pageId,
		Page:     page,
		Updates:  updates,
		Priority: priority,
		Dirty:    true,
	}
}

// Get retrieves a page from the set.
func (ps *PageSet) Get(pageId core.PageId) (*PageSetEntry, bool) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	key := pageId.Encode()
	entry, exists := ps.pages[key]
	return entry, exists
}

// Remove removes a page from the set.
func (ps *PageSet) Remove(pageId core.PageId) *PageSetEntry {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	key := pageId.Encode()
	entry, exists := ps.pages[key]
	if exists {
		delete(ps.pages, key)
	}
	return entry
}

// GetAll returns all pages in the set.
func (ps *PageSet) GetAll() []*PageSetEntry {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	result := make([]*PageSetEntry, 0, len(ps.pages))
	for _, entry := range ps.pages {
		result = append(result, entry)
	}
	return result
}

// Size returns the number of pages in the set.
func (ps *PageSet) Size() int {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	return len(ps.pages)
}

// Clear removes all pages from the set.
func (ps *PageSet) Clear() {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	ps.pages = make(map[[32]byte]*PageSetEntry)
}

// GetDirtyPages returns all dirty pages in the set.
func (ps *PageSet) GetDirtyPages() []*PageSetEntry {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	result := make([]*PageSetEntry, 0)
	for _, entry := range ps.pages {
		if entry.Dirty {
			result = append(result, entry)
		}
	}
	return result
}

// MarkClean marks a page as clean (not dirty).
func (ps *PageSet) MarkClean(pageId core.PageId) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	key := pageId.Encode()
	if entry, exists := ps.pages[key]; exists {
		entry.Dirty = false
	}
}