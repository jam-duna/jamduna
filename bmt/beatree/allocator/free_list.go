package allocator

import (
	"sync"
)

// FreeList manages a list of free pages available for reuse.
// It uses a simple stack-based approach for efficient allocation/deallocation.
type FreeList struct {
	mu    sync.Mutex
	pages []PageNumber
}

// NewFreeList creates a new empty free list.
func NewFreeList() *FreeList {
	return &FreeList{
		pages: make([]PageNumber, 0),
	}
}

// Push adds a page to the free list.
func (fl *FreeList) Push(page PageNumber) {
	fl.mu.Lock()
	defer fl.mu.Unlock()

	if !page.IsValid() {
		return
	}

	fl.pages = append(fl.pages, page)
}

// Pop removes and returns a page from the free list.
// Returns InvalidPageNumber if the list is empty.
func (fl *FreeList) Pop() PageNumber {
	fl.mu.Lock()
	defer fl.mu.Unlock()

	if len(fl.pages) == 0 {
		return InvalidPageNumber
	}

	page := fl.pages[len(fl.pages)-1]
	fl.pages = fl.pages[:len(fl.pages)-1]
	return page
}

// Len returns the number of pages in the free list.
func (fl *FreeList) Len() int {
	fl.mu.Lock()
	defer fl.mu.Unlock()
	return len(fl.pages)
}

// IsEmpty returns true if the free list is empty.
func (fl *FreeList) IsEmpty() bool {
	fl.mu.Lock()
	defer fl.mu.Unlock()
	return len(fl.pages) == 0
}

// Clear empties the free list.
func (fl *FreeList) Clear() {
	fl.mu.Lock()
	defer fl.mu.Unlock()
	fl.pages = fl.pages[:0]
}
