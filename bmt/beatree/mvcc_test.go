package beatree

import (
	"bytes"
	"testing"

	"github.com/jam-duna/jamduna/bmt/beatree/allocator"
)

func setupMVCCTest(t *testing.T) (*Shared, *ReadTransactionCounter) {
	// Use nil stores since we're testing MVCC logic, not disk I/O
	var branchStore *allocator.Store
	var leafStore *allocator.Store

	shared := NewShared(branchStore, leafStore)
	counter := NewReadTransactionCounter()
	return shared, counter
}

// TestReadTransactionCounter tests the read transaction counter.
func TestReadTransactionCounter(t *testing.T) {
	counter := NewReadTransactionCounter()

	if counter.Count() != 0 {
		t.Errorf("Initial count should be 0, got %d", counter.Count())
	}

	// Add transactions
	counter.AddOne()
	counter.AddOne()
	counter.AddOne()

	if counter.Count() != 3 {
		t.Errorf("Count should be 3 after adding 3, got %d", counter.Count())
	}

	// Release transactions
	counter.ReleaseOne()
	if counter.Count() != 2 {
		t.Errorf("Count should be 2 after releasing 1, got %d", counter.Count())
	}

	counter.ReleaseOne()
	counter.ReleaseOne()

	if counter.Count() != 0 {
		t.Errorf("Count should be 0 after releasing all, got %d", counter.Count())
	}
}

// TestReadTransactionCounterBlockUntilZero tests blocking until all transactions complete.
func TestReadTransactionCounterBlockUntilZero(t *testing.T) {
	counter := NewReadTransactionCounter()

	// Add a transaction
	counter.AddOne()

	// Start a goroutine that will release after a delay
	done := make(chan bool)
	go func() {
		counter.BlockUntilZero()
		done <- true
	}()

	// Release the transaction
	counter.ReleaseOne()

	// Should unblock quickly
	select {
	case <-done:
		// Success
	case <-make(chan bool):
		t.Error("BlockUntilZero did not unblock")
	}
}

// TestSharedStaging tests the staging mechanisms.
func TestSharedStaging(t *testing.T) {
	shared, _ := setupMVCCTest(t)

	key1 := KeyFromBytes(bytes.Repeat([]byte{1}, 32))
	key2 := KeyFromBytes(bytes.Repeat([]byte{2}, 32))

	// Commit some changes
	changeset := map[Key]*Change{
		key1: NewInsertChange([]byte("value1")),
		key2: NewInsertChange([]byte("value2")),
	}

	shared.CommitChanges(changeset)

	// Verify primary staging
	primary := shared.PrimaryStaging()
	if len(primary) != 2 {
		t.Errorf("Expected 2 entries in primary staging, got %d", len(primary))
	}

	if primary[key1].Value == nil || !bytes.Equal(primary[key1].Value, []byte("value1")) {
		t.Error("key1 not found in primary staging")
	}

	// Take staged changeset (moves to secondary)
	staged := shared.TakeStagedChangeset()
	if len(staged) != 2 {
		t.Errorf("Expected 2 entries in staged changeset, got %d", len(staged))
	}

	// Primary should now be empty
	primary = shared.PrimaryStaging()
	if len(primary) != 0 {
		t.Errorf("Expected empty primary staging after take, got %d entries", len(primary))
	}

	// Secondary should have the changes
	secondary := shared.SecondaryStaging()
	if secondary == nil {
		t.Fatal("Secondary staging should not be nil after take")
	}
	if len(secondary) != 2 {
		t.Errorf("Expected 2 entries in secondary staging, got %d", len(secondary))
	}

	// Clear secondary
	shared.ClearSecondaryStaging()
	secondary = shared.SecondaryStaging()
	if secondary != nil {
		t.Error("Secondary staging should be nil after clear")
	}
}

// TestReadTransaction tests read transaction snapshot isolation.
func TestReadTransaction(t *testing.T) {
	shared, counter := setupMVCCTest(t)

	key1 := KeyFromBytes(bytes.Repeat([]byte{1}, 32))

	// Commit a change
	changeset := map[Key]*Change{
		key1: NewInsertChange([]byte("version1")),
	}
	shared.CommitChanges(changeset)

	// Create a read transaction (snapshots current state)
	rtx1 := NewReadTransaction(shared, counter)
	defer rtx1.Close()

	// Verify counter incremented
	if counter.Count() != 1 {
		t.Errorf("Expected counter=1, got %d", counter.Count())
	}

	// Lookup in read transaction should see version1
	result, err := rtx1.Lookup(key1)
	if err != nil {
		t.Fatalf("Lookup failed: %v", err)
	}
	if !bytes.Equal(result, []byte("version1")) {
		t.Errorf("Expected version1, got %v", result)
	}

	// Commit another change (should NOT be visible in rtx1)
	changeset2 := map[Key]*Change{
		key1: NewInsertChange([]byte("version2")),
	}
	shared.CommitChanges(changeset2)

	// rtx1 should still see version1 (snapshot isolation)
	result, err = rtx1.Lookup(key1)
	if err != nil {
		t.Fatalf("Lookup failed: %v", err)
	}
	if !bytes.Equal(result, []byte("version1")) {
		t.Errorf("Read transaction should see version1, got %v", result)
	}

	// New read transaction should see version2
	rtx2 := NewReadTransaction(shared, counter)
	defer rtx2.Close()

	result, err = rtx2.Lookup(key1)
	if err != nil {
		t.Fatalf("Lookup failed: %v", err)
	}
	if !bytes.Equal(result, []byte("version2")) {
		t.Errorf("New transaction should see version2, got %v", result)
	}

	// Close rtx1
	rtx1.Close()
	if counter.Count() != 1 {
		t.Errorf("Expected counter=1 after closing rtx1, got %d", counter.Count())
	}

	// Close rtx2
	rtx2.Close()
	if counter.Count() != 0 {
		t.Errorf("Expected counter=0 after closing all, got %d", counter.Count())
	}
}

// TestSyncController tests the sync coordination.
func TestSyncController(t *testing.T) {
	shared, counter := setupMVCCTest(t)

	key1 := KeyFromBytes(bytes.Repeat([]byte{1}, 32))
	key2 := KeyFromBytes(bytes.Repeat([]byte{2}, 32))

	// Create sync controller
	sc := NewSyncController(shared, counter, nil)

	// Begin sync with changeset
	changeset := map[Key]*Change{
		key1: NewInsertChange([]byte("value1")),
		key2: NewInsertChange([]byte("value2")),
	}

	err := sc.BeginSync(changeset)
	if err != nil {
		t.Fatalf("BeginSync failed: %v", err)
	}

	// Wait for sync
	err = sc.WaitSync()
	if err != nil {
		t.Fatalf("WaitSync failed: %v", err)
	}

	// Finish sync
	sc.FinishSync()

	// Verify secondary staging is cleared
	secondary := shared.SecondaryStaging()
	if secondary != nil {
		t.Error("Secondary staging should be nil after FinishSync")
	}
}

// TestMVCCConcurrentReadersWriter tests concurrent readers with a writer.
func TestMVCCConcurrentReadersWriter(t *testing.T) {
	shared, counter := setupMVCCTest(t)

	key1 := KeyFromBytes(bytes.Repeat([]byte{1}, 32))

	// Initial commit
	changeset := map[Key]*Change{
		key1: NewInsertChange([]byte("v1")),
	}
	shared.CommitChanges(changeset)

	// Create multiple read transactions
	rtx1 := NewReadTransaction(shared, counter)
	rtx2 := NewReadTransaction(shared, counter)
	rtx3 := NewReadTransaction(shared, counter)

	// All should see v1
	for i, rtx := range []*ReadTransaction{rtx1, rtx2, rtx3} {
		result, err := rtx.Lookup(key1)
		if err != nil {
			t.Fatalf("Reader %d lookup failed: %v", i, err)
		}
		if !bytes.Equal(result, []byte("v1")) {
			t.Errorf("Reader %d should see v1, got %v", i, result)
		}
	}

	// Commit a new version (readers should still see v1)
	changeset2 := map[Key]*Change{
		key1: NewInsertChange([]byte("v2")),
	}
	shared.CommitChanges(changeset2)

	// Old readers still see v1
	result, _ := rtx1.Lookup(key1)
	if !bytes.Equal(result, []byte("v1")) {
		t.Errorf("Old reader should still see v1, got %v", result)
	}

	// New reader sees v2
	rtx4 := NewReadTransaction(shared, counter)
	result, _ = rtx4.Lookup(key1)
	if !bytes.Equal(result, []byte("v2")) {
		t.Errorf("New reader should see v2, got %v", result)
	}

	// Close all
	rtx1.Close()
	rtx2.Close()
	rtx3.Close()
	rtx4.Close()

	if counter.Count() != 0 {
		t.Errorf("All readers closed, counter should be 0, got %d", counter.Count())
	}

	t.Logf("✅ MVCC: 3 concurrent readers maintained snapshot isolation while writer committed v2")
}

// TestReadTransactionOverflowValue tests that overflow values (>1KB) are visible in snapshots.
func TestReadTransactionOverflowValue(t *testing.T) {
	shared := NewShared(nil, nil)

	// Create large value (>1KB to trigger overflow)
	largeValue := make([]byte, 2000)
	for i := range largeValue {
		largeValue[i] = byte(i % 256)
	}

	key := Key{1, 2, 3}

	// Commit large value to primary staging
	changeset := make(map[Key]*Change)
	changeset[key] = NewInsertChange(largeValue)

	// Verify it's marked as overflow
	if !changeset[key].IsOverflow() {
		t.Fatal("Expected 2KB value to be marked as overflow")
	}

	shared.CommitChanges(changeset)

	// Create read transaction - should see the large value
	counter := NewReadTransactionCounter()
	rtx := NewReadTransaction(shared, counter)

	retrieved, err := rtx.Lookup(key)
	if err != nil {
		t.Fatalf("Lookup failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("Overflow value should be visible in snapshot, got nil")
	}

	if len(retrieved) != len(largeValue) {
		t.Errorf("Value size mismatch: expected %d, got %d", len(largeValue), len(retrieved))
	}

	// Verify content
	for i := range largeValue {
		if retrieved[i] != largeValue[i] {
			t.Errorf("Byte %d mismatch: expected %d, got %d", i, largeValue[i], retrieved[i])
			break
		}
	}

	rtx.Close()

	t.Log("✅ Overflow values (>1KB) are visible in read transaction snapshots")
}
