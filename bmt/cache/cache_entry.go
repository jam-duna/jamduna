package cache

import (
	"github.com/colorfulnotion/jam/bmt/core"
	"github.com/colorfulnotion/jam/bmt/io"
)

// CacheEntry represents a cached page with LRU tracking.
type CacheEntry struct {
	// Page data
	PageId core.PageId
	Page   io.Page

	// LRU chain pointers (intrusive linked list)
	prev *CacheEntry
	next *CacheEntry

	// Reference counting for safety
	refs int32

	// State tracking
	dirty   bool // Page has been modified
	loading bool // Page is currently being loaded
}

// NewCacheEntry creates a new cache entry.
func NewCacheEntry(pageId core.PageId, page io.Page) *CacheEntry {
	return &CacheEntry{
		PageId:  pageId,
		Page:    page,
		prev:    nil,
		next:    nil,
		refs:    0,
		dirty:   false,
		loading: false,
	}
}

// AddRef increments the reference count.
// Returns the new reference count.
func (e *CacheEntry) AddRef() int32 {
	e.refs++
	return e.refs
}

// Release decrements the reference count.
// Returns the new reference count.
func (e *CacheEntry) Release() int32 {
	if e.refs > 0 {
		e.refs--
	}
	return e.refs
}

// RefCount returns the current reference count.
func (e *CacheEntry) RefCount() int32 {
	return e.refs
}

// MarkDirty marks the page as modified.
func (e *CacheEntry) MarkDirty() {
	e.dirty = true
}

// IsDirty returns whether the page has been modified.
func (e *CacheEntry) IsDirty() bool {
	return e.dirty
}

// MarkLoading marks the page as currently being loaded.
func (e *CacheEntry) MarkLoading() {
	e.loading = true
}

// MarkLoaded marks the page as finished loading.
func (e *CacheEntry) MarkLoaded() {
	e.loading = false
}

// IsLoading returns whether the page is currently being loaded.
func (e *CacheEntry) IsLoading() bool {
	return e.loading
}