package overlay

import (
	"sync"

	"github.com/jam-duna/jamduna/bmt/beatree"
	"github.com/jam-duna/jamduna/bmt/beatree/allocator"
)

// DirtyPage represents a modified page in an overlay.
type DirtyPage struct {
	PageNum allocator.PageNumber
	Data    []byte
}

// Data holds the changes made in a single overlay.
// It is immutable once the overlay is frozen and can be shared between overlays.
type Data struct {
	mu sync.RWMutex

	// Pages modified in this overlay
	pages map[allocator.PageNumber]*DirtyPage

	// Values modified in this overlay
	values map[beatree.Key]*beatree.Change

	// Status tracker for this overlay
	status *StatusTracker
}

// NewData creates a new empty data structure.
func NewData() *Data {
	return &Data{
		pages:  make(map[allocator.PageNumber]*DirtyPage),
		values: make(map[beatree.Key]*beatree.Change),
		status: NewStatusTracker(),
	}
}

// SetPage records a modified page.
func (d *Data) SetPage(pageNum allocator.PageNumber, data []byte) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Make a copy of the data
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)

	d.pages[pageNum] = &DirtyPage{
		PageNum: pageNum,
		Data:    dataCopy,
	}
}

// GetPage retrieves a modified page.
// Returns (page, true) if found, (nil, false) otherwise.
func (d *Data) GetPage(pageNum allocator.PageNumber) ([]byte, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	page, ok := d.pages[pageNum]
	if !ok {
		return nil, false
	}

	// Return a copy to maintain immutability
	dataCopy := make([]byte, len(page.Data))
	copy(dataCopy, page.Data)
	return dataCopy, true
}

// SetValue records a value change (insert/delete).
func (d *Data) SetValue(key beatree.Key, change *beatree.Change) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.values[key] = change
}

// GetValue retrieves a value change.
// Returns (change, true) if found, (nil, false) otherwise.
func (d *Data) GetValue(key beatree.Key) (*beatree.Change, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	change, ok := d.values[key]
	return change, ok
}

// HasValue returns true if the key has a change in this overlay.
func (d *Data) HasValue(key beatree.Key) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	_, ok := d.values[key]
	return ok
}

// ValueCount returns the number of value changes in this overlay.
func (d *Data) ValueCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return len(d.values)
}

// PageCount returns the number of modified pages in this overlay.
func (d *Data) PageCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return len(d.pages)
}

// Status returns the status tracker for this data.
func (d *Data) Status() *StatusTracker {
	return d.status
}

// GetAllValues returns a copy of all value changes.
func (d *Data) GetAllValues() map[beatree.Key]*beatree.Change {
	d.mu.RLock()
	defer d.mu.RUnlock()

	result := make(map[beatree.Key]*beatree.Change, len(d.values))
	for k, v := range d.values {
		result[k] = v
	}
	return result
}

// GetAllPages returns a copy of all page changes.
func (d *Data) GetAllPages() map[allocator.PageNumber]*DirtyPage {
	d.mu.RLock()
	defer d.mu.RUnlock()

	result := make(map[allocator.PageNumber]*DirtyPage, len(d.pages))
	for k, v := range d.pages {
		// Deep copy
		dataCopy := make([]byte, len(v.Data))
		copy(dataCopy, v.Data)
		result[k] = &DirtyPage{
			PageNum: v.PageNum,
			Data:    dataCopy,
		}
	}
	return result
}
