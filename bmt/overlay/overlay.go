package overlay

import (
	"fmt"

	"github.com/jam-duna/jamduna/bmt/beatree"
	"github.com/jam-duna/jamduna/bmt/beatree/allocator"
)

// Overlay represents a frozen, immutable overlay with ancestry tracking.
// It contains changes made in a session and maintains weak references to ancestors.
type Overlay struct {
	// Root hashes before and after changes
	prevRoot [32]byte
	root     [32]byte

	// Index maps keys to ancestor indices for O(1) lookup
	index *Index

	// This overlay's changes
	data *Data

	// Ancestor overlays (weak references via direct pointers)
	// Index 0 is parent, 1 is grandparent, etc.
	ancestors []*Data
}

// NewOverlay creates a new frozen overlay from live overlay data.
func NewOverlay(prevRoot, root [32]byte, data *Data, ancestors []*Data) *Overlay {
	// Build index for this overlay's keys
	index := NewIndex()

	// Add ancestor keys first (so they can be overridden)
	for i, ancestorData := range ancestors {
		for key := range ancestorData.GetAllValues() {
			// Ancestor index is i+1 (0 is reserved for this overlay)
			if !index.Has(key) {
				index.Set(key, i+1)
			}
		}
	}

	// Add this overlay's keys (overrides ancestors)
	for key := range data.GetAllValues() {
		index.Set(key, 0) // This overlay (index 0)
	}

	return &Overlay{
		prevRoot:  prevRoot,
		root:      root,
		index:     index,
		data:      data,
		ancestors: ancestors,
	}
}

// PrevRoot returns the root hash before changes.
func (o *Overlay) PrevRoot() [32]byte {
	return o.prevRoot
}

// Root returns the root hash after changes.
func (o *Overlay) Root() [32]byte {
	return o.root
}

// Data returns the data for this overlay.
func (o *Overlay) Data() *Data {
	return o.data
}

// Lookup retrieves a value by key, checking this overlay and ancestors.
func (o *Overlay) Lookup(key beatree.Key) ([]byte, error) {
	// Check index for which ancestor has this key
	ancestorIdx, found := o.index.Get(key)
	if !found {
		// Key not in any overlay
		return nil, nil
	}

	// Get the data from the appropriate ancestor
	var targetData *Data
	if ancestorIdx == 0 {
		targetData = o.data
	} else if ancestorIdx-1 < len(o.ancestors) {
		targetData = o.ancestors[ancestorIdx-1]
	} else {
		return nil, fmt.Errorf("invalid ancestor index: %d", ancestorIdx)
	}

	// Retrieve the change
	change, ok := targetData.GetValue(key)
	if !ok {
		return nil, nil
	}

	// If it's a delete, return nil
	if change.IsDelete() {
		return nil, nil
	}

	return change.Value, nil
}

// GetPage retrieves a page, checking this overlay and ancestors.
func (o *Overlay) GetPage(pageNum allocator.PageNumber) ([]byte, bool) {
	// Check this overlay first
	if data, ok := o.data.GetPage(pageNum); ok {
		return data, true
	}

	// Check ancestors
	for _, ancestorData := range o.ancestors {
		if data, ok := ancestorData.GetPage(pageNum); ok {
			return data, true
		}
	}

	return nil, false
}

// HasValue returns true if the key has a value in this overlay or ancestors.
func (o *Overlay) HasValue(key beatree.Key) bool {
	return o.index.Has(key)
}

// Status returns the status of this overlay.
func (o *Overlay) Status() OverlayStatus {
	return o.data.Status().Get()
}

// MarkCommitted marks this overlay as committed.
func (o *Overlay) MarkCommitted() bool {
	return o.data.Status().MarkCommitted()
}

// MarkDropped marks this overlay as dropped.
func (o *Overlay) MarkDropped() bool {
	return o.data.Status().MarkDropped()
}

// ValueCount returns the number of value changes in this overlay (not ancestors).
func (o *Overlay) ValueCount() int {
	return o.data.ValueCount()
}

// PageCount returns the number of page changes in this overlay (not ancestors).
func (o *Overlay) PageCount() int {
	return o.data.PageCount()
}

// AncestorCount returns the number of ancestors.
func (o *Overlay) AncestorCount() int {
	return len(o.ancestors)
}

// GetAllValues returns all value changes from this overlay.
func (o *Overlay) GetAllValues() map[beatree.Key]*beatree.Change {
	return o.data.GetAllValues()
}

// GetAllPages returns all page changes from this overlay.
func (o *Overlay) GetAllPages() map[allocator.PageNumber]*DirtyPage {
	return o.data.GetAllPages()
}
