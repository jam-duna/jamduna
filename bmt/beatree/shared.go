package beatree

import (
	"sync"

	"github.com/colorfulnotion/jam/bmt/beatree/allocator"
)

// Shared represents the shared mutable state of the Beatree.
//
// This contains:
// - The current tree root
// - Primary staging: changes committed but not yet synced
// - Secondary staging: changes currently being synced (if any)
// - Store references
//
// Focuses on MVCC semantics. Disk I/O coordination handled by SyncController.
type Shared struct {
	mu sync.RWMutex

	// Current root of the tree
	root allocator.PageNumber

	// Stores for branch and leaf pages
	branchStore *allocator.Store
	leafStore   *allocator.Store

	// Primary staging collects changes that are committed but not synced yet.
	// Upon sync, changes from here are moved to secondary staging.
	primaryStaging map[Key]*Change

	// Secondary staging collects committed changes that are currently being synced.
	// This is nil if there is no sync in progress.
	secondaryStaging map[Key]*Change
}

// NewShared creates a new shared state.
func NewShared(branchStore, leafStore *allocator.Store) *Shared {
	return &Shared{
		root:             allocator.InvalidPageNumber,
		branchStore:      branchStore,
		leafStore:        leafStore,
		primaryStaging:   make(map[Key]*Change),
		secondaryStaging: nil,
	}
}

// Root returns the current root page number.
// Caller must hold at least read lock.
func (s *Shared) Root() allocator.PageNumber {
	return s.root
}

// SetRoot sets the new root page number.
// Caller must hold write lock.
func (s *Shared) SetRoot(root allocator.PageNumber) {
	s.root = root
}

// PrimaryStaging returns a copy of the primary staging map.
// This is used by read transactions to snapshot the current uncommitted state.
func (s *Shared) PrimaryStaging() map[Key]*Change {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Create a copy
	staging := make(map[Key]*Change, len(s.primaryStaging))
	for k, v := range s.primaryStaging {
		staging[k] = v
	}
	return staging
}

// SecondaryStaging returns a copy of the secondary staging map.
// Returns nil if no sync is in progress.
func (s *Shared) SecondaryStaging() map[Key]*Change {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.secondaryStaging == nil {
		return nil
	}

	// Create a copy
	staging := make(map[Key]*Change, len(s.secondaryStaging))
	for k, v := range s.secondaryStaging {
		staging[k] = v
	}
	return staging
}

// CommitChanges adds a changeset to the primary staging.
// This makes changes visible to new read transactions but doesn't persist them.
func (s *Shared) CommitChanges(changeset map[Key]*Change) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for key, change := range changeset {
		s.primaryStaging[key] = change
	}
}

// TakeStagedChangeset moves primary staging to secondary staging and returns a copy.
// This is called at the start of sync to freeze the changeset being synced.
// Panics if a sync is already in progress.
func (s *Shared) TakeStagedChangeset() map[Key]*Change {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.secondaryStaging != nil {
		panic("cannot take staged changeset: sync already in progress")
	}

	// Move primary to secondary
	s.secondaryStaging = s.primaryStaging
	s.primaryStaging = make(map[Key]*Change)

	// Return a copy of the secondary staging
	changeset := make(map[Key]*Change, len(s.secondaryStaging))
	for k, v := range s.secondaryStaging {
		changeset[k] = v
	}
	return changeset
}

// ClearSecondaryStaging clears the secondary staging after sync completes.
func (s *Shared) ClearSecondaryStaging() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.secondaryStaging = nil
}

// BranchStore returns the branch store.
// Used for creating iterators and other operations that need direct store access.
func (s *Shared) BranchStore() *allocator.Store {
	return s.branchStore
}

// LeafStore returns the leaf store.
// Used for creating iterators and other operations that need direct store access.
func (s *Shared) LeafStore() *allocator.Store {
	return s.leafStore
}
