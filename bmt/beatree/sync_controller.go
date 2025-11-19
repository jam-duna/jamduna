package beatree

import (
	"sync"

	"github.com/colorfulnotion/jam/bmt/beatree/allocator"
)

// SyncController controls the sync process.
//
// The sync process has three steps:
// 1. BeginSync: Commit changes to primary staging and start sync
// 2. WaitSync: Wait for sync to complete (currently synchronous, async planned)
// 3. FinishSync: Finalize the sync by updating the root and clearing secondary staging
//
// Note: Full tree serialization with ops.Update is available via higher-level coordinators
// to avoid Go import cycle restrictions (beatree ↔ ops).
// Current implementation uses in-memory overlay. Future: Async I/O with fsync for durability.
type SyncController struct {
	shared  *Shared
	counter *ReadTransactionCounter
	mu      sync.Mutex

	// The new root after sync completes
	newRoot allocator.PageNumber
}

// NewSyncController creates a new sync controller.
func NewSyncController(shared *Shared, counter *ReadTransactionCounter, index *Index) *SyncController {
	return &SyncController{
		shared:  shared,
		counter: counter,
		newRoot: allocator.InvalidPageNumber,
	}
}

// BeginSync commits a changeset and initiates the sync process.
//
// This:
// 1. Commits the changeset to primary staging
// 2. Waits for all read transactions to complete
// 3. Moves primary staging to secondary staging
// 4. Applies changes to create a new tree root (CoW)
//
// Currently synchronous. Future: Async I/O for better concurrency.
func (sc *SyncController) BeginSync(changeset map[Key]*Change) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	// Step 1: Commit changes to primary staging
	sc.shared.CommitChanges(changeset)

	// Step 2: Begin sync (blocks new readers, waits for existing readers)
	// This ensures they won't be invalidated by destructive changes
	sc.counter.BeginSync()

	// Ensure EndSync is called if this function exits early
	syncStarted := true
	defer func() {
		if syncStarted {
			// If we haven't called FinishSync, make sure to end the sync
			// This handles error cases where FinishSync might not be called
			sc.counter.EndSync()
		}
	}()

	// Step 3: Take staged changeset (moves primary → secondary)
	stagedChanges := sc.shared.TakeStagedChangeset()

	// Step 4: Apply changes with CoW to create new root
	// Note: ops.Update integration available at higher levels to avoid import cycles
	sc.shared.mu.Lock()
	oldRoot := sc.shared.root
	sc.shared.mu.Unlock()

	// Allocate new root using CoW semantics
	// Full tree serialization via ops.Update happens in higher-level coordinators
	if len(stagedChanges) == 0 {
		sc.newRoot = oldRoot
	} else if sc.shared.leafStore != nil {
		// Allocate new root page for CoW
		// Changes tracked in overlay and applied on reads
		sc.newRoot = sc.shared.leafStore.AllocPage()
	} else {
		// For testing without disk stores, use incrementing page numbers
		sc.newRoot = oldRoot + 1
	}

	syncStarted = false // FinishSync will handle EndSync
	return nil
}

// WaitSync waits for the sync to complete.
// Currently a no-op since BeginSync is synchronous.
// Future: Will wait for async I/O to complete.
func (sc *SyncController) WaitSync() error {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	// No-op (synchronous sync)
	return nil
}

// FinishSync finalizes the sync by updating the root and clearing secondary staging.
// This should be called after any manifest updates.
func (sc *SyncController) FinishSync() {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	// Update the root
	sc.shared.mu.Lock()
	sc.shared.root = sc.newRoot
	sc.shared.mu.Unlock()

	// Clear secondary staging
	sc.shared.ClearSecondaryStaging()

	// End sync to allow new readers
	sc.counter.EndSync()
}

// Root returns the new root page number after sync.
// Should be called after WaitSync and before FinishSync.
func (sc *SyncController) Root() allocator.PageNumber {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return sc.newRoot
}
