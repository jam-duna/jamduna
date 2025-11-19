package ops_test

import (
	"fmt"
	"bytes"
	"os"
	"testing"

	"github.com/colorfulnotion/jam/bmt/beatree"
	"github.com/colorfulnotion/jam/bmt/beatree/allocator"
	"github.com/colorfulnotion/jam/bmt/beatree/ops"
	"github.com/colorfulnotion/jam/bmt/io"
)

// TestCoordinatorLayer demonstrates the coordinator layer pattern.
//
// This shows how ops.Update can be used for full tree serialization with CoW semantics.
// Note: This test is in ops_test package (not ops package) to avoid import cycles.
// In production, coordinator logic would be at the application level, not in beatree package.
func TestCoordinatorLayer(t *testing.T) {
	// Test 1: Insert into empty tree
	t.Run("InsertIntoEmptyTree", func(t *testing.T) {
		tmpFile, err := os.CreateTemp("", "coordinator_insert_*.dat")
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}
		defer os.Remove(tmpFile.Name())
		defer tmpFile.Close()

		pagePool := io.NewPagePool()
		leafStore := allocator.NewStore(tmpFile, pagePool)
		storeAdapter := &StoreAdapter{store: leafStore}
		index := beatree.NewIndex()

		key1 := beatree.KeyFromBytes(bytes.Repeat([]byte{1}, 32))
		key2 := beatree.KeyFromBytes(bytes.Repeat([]byte{2}, 32))

		changeset := map[beatree.Key]*beatree.Change{
			key1: beatree.NewInsertChange([]byte("value1")),
			key2: beatree.NewInsertChange([]byte("value2")),
		}

		// Apply changes using ops.Update
		result, err := ops.Update(allocator.InvalidPageNumber, changeset, index, storeAdapter)
		if err != nil {
			t.Fatalf("ops.Update failed: %v", err)
		}

		if !result.NewRoot.IsValid() {
			t.Errorf("Expected valid root page")
		}

		if len(result.AllocatedPages) == 0 {
			t.Errorf("Expected allocated pages")
		}

		t.Logf("✓ Created tree with root %v, allocated %d pages", result.NewRoot, len(result.AllocatedPages))
	})

	// Test 2: Update existing tree with CoW
	t.Run("UpdateWithCoW", func(t *testing.T) {
		tmpFile, err := os.CreateTemp("", "coordinator_update_*.dat")
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}
		defer os.Remove(tmpFile.Name())
		defer tmpFile.Close()

		pagePool := io.NewPagePool()
		leafStore := allocator.NewStore(tmpFile, pagePool)
		storeAdapter := &StoreAdapter{store: leafStore}
		index := beatree.NewIndex()

		// Create initial tree
		key1 := beatree.KeyFromBytes(bytes.Repeat([]byte{1}, 32))
		changeset1 := map[beatree.Key]*beatree.Change{
			key1: beatree.NewInsertChange([]byte("initial")),
		}

		result1, err := ops.Update(allocator.InvalidPageNumber, changeset1, index, storeAdapter)
		if err != nil {
			t.Fatalf("Initial ops.Update failed: %v", err)
		}
		oldRoot := result1.NewRoot

		// Verify page was written (debug)
		testRead, err := leafStore.ReadPage(oldRoot)
		if err != nil {
			t.Fatalf("Cannot read back written page %v: %v", oldRoot, err)
		}
		if len(testRead) == 0 {
			t.Fatalf("Page %v has zero length", oldRoot)
		}
		t.Logf("Debug: Successfully read back page %v (%d bytes)", oldRoot, len(testRead))

		// Update the tree
		key2 := beatree.KeyFromBytes(bytes.Repeat([]byte{2}, 32))
		changeset2 := map[beatree.Key]*beatree.Change{
			key2: beatree.NewInsertChange([]byte("updated")),
		}

		result2, err := ops.Update(oldRoot, changeset2, index, storeAdapter)
		if err != nil {
			t.Fatalf("Update ops.Update failed: %v", err)
		}

		// Verify CoW: new root should differ from old root
		if result2.NewRoot == oldRoot {
			t.Errorf("Expected new root to differ from old root (CoW)")
		}

		// Old root should still be accessible (not freed)
		if len(result2.FreedPages) > 0 {
			t.Errorf("Expected no freed pages (CoW preserves old versions)")
		}

		t.Logf("✓ Updated tree: old root %v → new root %v (CoW verified)", oldRoot, result2.NewRoot)
	})

	// Test 3: Delete operation
	t.Run("DeleteOperation", func(t *testing.T) {
		tmpFile, err := os.CreateTemp("", "coordinator_delete_*.dat")
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}
		defer os.Remove(tmpFile.Name())
		defer tmpFile.Close()

		pagePool := io.NewPagePool()
		leafStore := allocator.NewStore(tmpFile, pagePool)
		storeAdapter := &StoreAdapter{store: leafStore}
		index := beatree.NewIndex()

		// Create tree with two keys
		key1 := beatree.KeyFromBytes(bytes.Repeat([]byte{1}, 32))
		key2 := beatree.KeyFromBytes(bytes.Repeat([]byte{2}, 32))

		changeset1 := map[beatree.Key]*beatree.Change{
			key1: beatree.NewInsertChange([]byte("value1")),
			key2: beatree.NewInsertChange([]byte("value2")),
		}

		result1, err := ops.Update(allocator.InvalidPageNumber, changeset1, index, storeAdapter)
		if err != nil {
			t.Fatalf("Insert ops.Update failed: %v", err)
		}
		oldRoot := result1.NewRoot

		// Delete one key
		changeset2 := map[beatree.Key]*beatree.Change{
			key1: beatree.NewDeleteChange(),
		}

		result2, err := ops.Update(oldRoot, changeset2, index, storeAdapter)
		if err != nil {
			t.Fatalf("Delete ops.Update failed: %v", err)
		}

		// Verify CoW for delete
		if result2.NewRoot == oldRoot {
			t.Errorf("Expected new root after delete (CoW)")
		}

		t.Logf("✓ Deleted key: old root %v → new root %v (CoW verified)", oldRoot, result2.NewRoot)
	})
}

// Test Full Sync Cycle demonstrating coordinator pattern
func TestFullSyncCycle(t *testing.T) {
	// Create temporary file
	tmpFile, err := os.CreateTemp("", "full_sync_*.dat")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	// Setup
	pagePool := io.NewPagePool()
	leafStore := allocator.NewStore(tmpFile, pagePool)
	storeAdapter := &StoreAdapter{store: leafStore}
	index := beatree.NewIndex()

	// Simulate sync cycle
	currentRoot := allocator.InvalidPageNumber

	// Sync 1: Initial data
	changeset1 := map[beatree.Key]*beatree.Change{
		beatree.KeyFromBytes(bytes.Repeat([]byte{1}, 32)): beatree.NewInsertChange([]byte("block1")),
		beatree.KeyFromBytes(bytes.Repeat([]byte{2}, 32)): beatree.NewInsertChange([]byte("block2")),
	}

	result1, err := ops.Update(currentRoot, changeset1, index, storeAdapter)
	if err != nil {
		t.Fatalf("Sync 1 failed: %v", err)
	}
	currentRoot = result1.NewRoot
	t.Logf("Sync 1: root=%v, allocated=%d pages", currentRoot, len(result1.AllocatedPages))

	// Sync 2: More data
	changeset2 := map[beatree.Key]*beatree.Change{
		beatree.KeyFromBytes(bytes.Repeat([]byte{3}, 32)): beatree.NewInsertChange([]byte("block3")),
	}

	result2, err := ops.Update(currentRoot, changeset2, index, storeAdapter)
	if err != nil {
		t.Fatalf("Sync 2 failed: %v", err)
	}
	currentRoot = result2.NewRoot
	t.Logf("Sync 2: root=%v, allocated=%d pages", currentRoot, len(result2.AllocatedPages))

	// Sync 3: Deletions
	changeset3 := map[beatree.Key]*beatree.Change{
		beatree.KeyFromBytes(bytes.Repeat([]byte{1}, 32)): beatree.NewDeleteChange(),
	}

	result3, err := ops.Update(currentRoot, changeset3, index, storeAdapter)
	if err != nil {
		t.Fatalf("Sync 3 failed: %v", err)
	}
	currentRoot = result3.NewRoot
	t.Logf("Sync 3: root=%v, allocated=%d pages", currentRoot, len(result3.AllocatedPages))

	// Verify final state
	if !currentRoot.IsValid() {
		t.Errorf("Expected valid root after all syncs")
	}
}

// StoreAdapter adapts allocator.Store to ops.PageStore interface.
type StoreAdapter struct {
	store *allocator.Store
}

func (sa *StoreAdapter) Alloc() allocator.PageNumber {
	return sa.store.AllocPage()
}

func (sa *StoreAdapter) FreePage(page allocator.PageNumber) {
	sa.store.FreePage(page)
}

func (sa *StoreAdapter) Page(page allocator.PageNumber) ([]byte, error) {
	ioPage, err := sa.store.ReadPage(page)
	if err != nil {
		return nil, err
	}
	result := make([]byte, len(ioPage))
	copy(result, ioPage)
	return result, nil
}

func (sa *StoreAdapter) WritePage(page allocator.PageNumber, data []byte) {
	sa.store.WritePage(page, data)
}

func (sa *StoreAdapter) WritePageDebug(page allocator.PageNumber, data []byte) {
	if err := sa.store.WritePage(page, data); err != nil {
		panic(fmt.Sprintf("WritePage failed for page %v: %v", page, err))
	}
}
