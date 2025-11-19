package overlay

import (
	"fmt"

	"github.com/colorfulnotion/jam/bmt/beatree"
	"github.com/colorfulnotion/jam/bmt/beatree/allocator"
)

// LiveOverlay represents a mutable overlay during an active session.
// It can be frozen into an immutable Overlay for committing.
type LiveOverlay struct {
	// Root hashes
	prevRoot [32]byte
	root     [32]byte

	// Mutable data
	data *Data

	// Parent overlay (if any)
	parent *Overlay
}

// NewLiveOverlay creates a new live overlay with an optional parent.
func NewLiveOverlay(prevRoot [32]byte, parent *Overlay) *LiveOverlay {
	return &LiveOverlay{
		prevRoot: prevRoot,
		root:     prevRoot, // Initially same as prevRoot
		data:     NewData(),
		parent:   parent,
	}
}

// SetValue records a value change (insert/update/delete).
func (lo *LiveOverlay) SetValue(key beatree.Key, change *beatree.Change) error {
	if !lo.data.Status().IsLive() {
		return fmt.Errorf("overlay is no longer live")
	}

	lo.data.SetValue(key, change)
	return nil
}

// SetPage records a page modification.
func (lo *LiveOverlay) SetPage(pageNum allocator.PageNumber, data []byte) error {
	if !lo.data.Status().IsLive() {
		return fmt.Errorf("overlay is no longer live")
	}

	lo.data.SetPage(pageNum, data)
	return nil
}

// Lookup retrieves a value, checking this overlay and parent chain.
func (lo *LiveOverlay) Lookup(key beatree.Key) ([]byte, error) {
	// Check this overlay first
	if change, ok := lo.data.GetValue(key); ok {
		if change.IsDelete() {
			return nil, nil
		}
		return change.Value, nil
	}

	// Check parent if exists
	if lo.parent != nil {
		return lo.parent.Lookup(key)
	}

	return nil, nil
}

// GetPage retrieves a page, checking this overlay and parent chain.
func (lo *LiveOverlay) GetPage(pageNum allocator.PageNumber) ([]byte, bool) {
	// Check this overlay first
	if data, ok := lo.data.GetPage(pageNum); ok {
		return data, true
	}

	// Check parent if exists
	if lo.parent != nil {
		return lo.parent.GetPage(pageNum)
	}

	return nil, false
}

// SetRoot updates the root hash after changes.
func (lo *LiveOverlay) SetRoot(root [32]byte) {
	lo.root = root
}

// Root returns the current root hash.
func (lo *LiveOverlay) Root() [32]byte {
	return lo.root
}

// PrevRoot returns the root hash before changes.
func (lo *LiveOverlay) PrevRoot() [32]byte {
	return lo.prevRoot
}

// Freeze converts this live overlay into an immutable Overlay.
// After freezing, the live overlay can no longer be modified.
func (lo *LiveOverlay) Freeze() (*Overlay, error) {
	if !lo.data.Status().IsLive() {
		return nil, fmt.Errorf("overlay is no longer live")
	}

	// Mark as committed to prevent future modifications
	lo.data.Status().MarkCommitted()

	// Build ancestor chain
	var ancestors []*Data
	if lo.parent != nil {
		// Add parent's data
		ancestors = append(ancestors, lo.parent.data)
		// Add parent's ancestors
		ancestors = append(ancestors, lo.parent.ancestors...)
	}

	// Create frozen overlay
	overlay := NewOverlay(lo.prevRoot, lo.root, lo.data, ancestors)

	return overlay, nil
}

// Drop marks this overlay as dropped (discarded).
func (lo *LiveOverlay) Drop() {
	lo.data.Status().MarkDropped()
}

// ValueCount returns the number of value changes in this overlay.
func (lo *LiveOverlay) ValueCount() int {
	return lo.data.ValueCount()
}

// PageCount returns the number of page changes in this overlay.
func (lo *LiveOverlay) PageCount() int {
	return lo.data.PageCount()
}

// HasParent returns true if this overlay has a parent.
func (lo *LiveOverlay) HasParent() bool {
	return lo.parent != nil
}

// Parent returns the parent overlay (may be nil).
func (lo *LiveOverlay) Parent() *Overlay {
	return lo.parent
}

// GetAllValues returns all value changes from this overlay.
func (lo *LiveOverlay) GetAllValues() map[beatree.Key]*beatree.Change {
	return lo.data.GetAllValues()
}

// GetAllPages returns all page changes from this overlay.
func (lo *LiveOverlay) GetAllPages() map[allocator.PageNumber]*DirtyPage {
	return lo.data.GetAllPages()
}
