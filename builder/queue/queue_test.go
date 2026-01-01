package queue

import (
	"testing"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

func TestNewQueueState(t *testing.T) {
	qs := NewQueueState(0)
	if qs == nil {
		t.Fatal("NewQueueState returned nil")
	}
	if qs.serviceID != 0 {
		t.Errorf("Expected serviceID 0, got %d", qs.serviceID)
	}
	if qs.config.MaxQueueDepth != DefaultMaxQueueDepth {
		t.Errorf("Expected MaxQueueDepth %d, got %d", DefaultMaxQueueDepth, qs.config.MaxQueueDepth)
	}
}

func TestEnqueue(t *testing.T) {
	qs := NewQueueState(0)

	// Create a mock bundle
	bundle := &types.WorkPackageBundle{
		WorkPackage: types.WorkPackage{},
	}

	bn, err := qs.Enqueue(bundle)
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}
	if bn != 1 {
		t.Errorf("Expected block number 1, got %d", bn)
	}

	stats := qs.GetStats()
	if stats.QueuedCount != 1 {
		t.Errorf("Expected 1 queued item, got %d", stats.QueuedCount)
	}
}

func TestQueueFull(t *testing.T) {
	config := DefaultConfig()
	config.MaxQueueDepth = 2
	qs := NewQueueStateWithConfig(0, config)

	bundle := &types.WorkPackageBundle{
		WorkPackage: types.WorkPackage{},
	}

	// Fill the queue
	_, err := qs.Enqueue(bundle)
	if err != nil {
		t.Fatalf("First enqueue failed: %v", err)
	}
	_, err = qs.Enqueue(bundle)
	if err != nil {
		t.Fatalf("Second enqueue failed: %v", err)
	}

	// Should fail - queue full
	_, err = qs.Enqueue(bundle)
	if err == nil {
		t.Error("Expected error when queue is full, got nil")
	}
}

func TestDequeue(t *testing.T) {
	qs := NewQueueState(0)

	bundle1 := &types.WorkPackageBundle{
		WorkPackage: types.WorkPackage{},
	}
	bundle2 := &types.WorkPackageBundle{
		WorkPackage: types.WorkPackage{},
	}

	qs.Enqueue(bundle1)
	qs.Enqueue(bundle2)

	// Should dequeue lowest block number first
	item := qs.Dequeue()
	if item == nil {
		t.Fatal("Dequeue returned nil")
	}
	if item.BlockNumber != 1 {
		t.Errorf("Expected block number 1, got %d", item.BlockNumber)
	}

	item = qs.Dequeue()
	if item == nil {
		t.Fatal("Second dequeue returned nil")
	}
	if item.BlockNumber != 2 {
		t.Errorf("Expected block number 2, got %d", item.BlockNumber)
	}

	// Queue should be empty now
	item = qs.Dequeue()
	if item != nil {
		t.Error("Expected nil from empty queue")
	}
}

func TestInflightLimit(t *testing.T) {
	config := DefaultConfig()
	config.MaxInflight = 2
	qs := NewQueueStateWithConfig(0, config)

	bundle := &types.WorkPackageBundle{
		WorkPackage: types.WorkPackage{},
	}

	// Enqueue 3 items
	qs.Enqueue(bundle)
	qs.Enqueue(bundle)
	qs.Enqueue(bundle)

	// Dequeue and mark as submitted
	item1 := qs.Dequeue()
	qs.MarkSubmitted(item1, common.Hash{1})

	item2 := qs.Dequeue()
	qs.MarkSubmitted(item2, common.Hash{2})

	// Should not be able to dequeue more (inflight limit reached)
	item3 := qs.Dequeue()
	if item3 != nil {
		t.Error("Expected nil due to inflight limit, got item")
	}

	// After guaranteed, should still be at limit
	qs.OnGuaranteed(common.Hash{1})
	item3 = qs.Dequeue()
	if item3 != nil {
		t.Error("Expected nil after guarantee (still at limit)")
	}

	// After accumulated, inflight decreases
	qs.OnAccumulated(common.Hash{1})
	// Status is now Accumulated, not Submitted/Guaranteed, so inflight should decrease

	// Now we should be able to dequeue
	item3 = qs.Dequeue()
	if item3 == nil {
		t.Error("Expected to dequeue after accumulation reduced inflight")
	}
}

func TestStatusTransitions(t *testing.T) {
	qs := NewQueueState(0)

	bundle := &types.WorkPackageBundle{
		WorkPackage: types.WorkPackage{},
	}

	bn, _ := qs.Enqueue(bundle)
	wpHash := common.Hash{1, 2, 3}

	// Initial status
	if qs.Status[bn] != StatusQueued {
		t.Errorf("Expected StatusQueued, got %v", qs.Status[bn])
	}

	// Dequeue and submit
	item := qs.Dequeue()
	qs.MarkSubmitted(item, wpHash)
	if qs.Status[bn] != StatusSubmitted {
		t.Errorf("Expected StatusSubmitted, got %v", qs.Status[bn])
	}

	// Guaranteed
	qs.OnGuaranteed(wpHash)
	if qs.Status[bn] != StatusGuaranteed {
		t.Errorf("Expected StatusGuaranteed, got %v", qs.Status[bn])
	}

	// Accumulated
	qs.OnAccumulated(wpHash)
	if qs.Status[bn] != StatusAccumulated {
		t.Errorf("Expected StatusAccumulated, got %v", qs.Status[bn])
	}

	// Finalized
	qs.OnFinalized(wpHash)
	if qs.Status[bn] != StatusFinalized {
		t.Errorf("Expected StatusFinalized, got %v", qs.Status[bn])
	}

	// Should be in Finalized map
	if _, ok := qs.Finalized[bn]; !ok {
		t.Error("Expected item in Finalized map")
	}
}

func TestStaleVersionIgnored(t *testing.T) {
	qs := NewQueueState(0)

	bundle := &types.WorkPackageBundle{
		WorkPackage: types.WorkPackage{},
	}

	bn, _ := qs.Enqueue(bundle)
	wpHash1 := common.Hash{1}

	// Submit v1
	item := qs.Dequeue()
	qs.MarkSubmitted(item, wpHash1)

	// Simulate timeout/failure - requeues with v2
	qs.OnTimeoutOrFailure(bn)

	// Dequeue v2 and submit
	item = qs.Dequeue()
	wpHash2 := common.Hash{2}
	qs.MarkSubmitted(item, wpHash2)

	// Old hash (v1) guaranteed - should be ignored
	qs.OnGuaranteed(wpHash1)
	if qs.Status[bn] != StatusSubmitted {
		t.Errorf("Stale guarantee should be ignored, expected StatusSubmitted, got %v", qs.Status[bn])
	}

	// New hash (v2) guaranteed - should be accepted
	qs.OnGuaranteed(wpHash2)
	if qs.Status[bn] != StatusGuaranteed {
		t.Errorf("Expected StatusGuaranteed for current version, got %v", qs.Status[bn])
	}
}

func TestMaxVersionRetries(t *testing.T) {
	config := DefaultConfig()
	config.MaxVersionRetries = 2
	qs := NewQueueStateWithConfig(0, config)

	bundle := &types.WorkPackageBundle{
		WorkPackage: types.WorkPackage{},
	}

	bn, _ := qs.Enqueue(bundle)

	// Submit v1
	item := qs.Dequeue()
	qs.MarkSubmitted(item, common.Hash{1})

	// Fail v1 -> v2
	qs.OnTimeoutOrFailure(bn)
	if qs.Status[bn] != StatusQueued {
		t.Errorf("Expected StatusQueued after first failure, got %v", qs.Status[bn])
	}

	// Submit v2
	item = qs.Dequeue()
	qs.MarkSubmitted(item, common.Hash{2})

	// Fail v2 -> should be dropped (max retries = 2)
	qs.OnTimeoutOrFailure(bn)
	if qs.Status[bn] != StatusFailed {
		t.Errorf("Expected StatusFailed after max retries, got %v", qs.Status[bn])
	}

	// Should not be in queued or inflight
	if _, ok := qs.Queued[bn]; ok {
		t.Error("Failed item should not be in Queued")
	}
	if _, ok := qs.Inflight[bn]; ok {
		t.Error("Failed item should not be in Inflight")
	}
}

func TestStatusChangeCallback(t *testing.T) {
	qs := NewQueueState(0)

	var callbackCalled bool
	var lastOldStatus, lastNewStatus WorkPackageBundleStatus
	var lastBlockNumber uint64

	qs.SetStatusChangeCallback(func(bn uint64, old, new WorkPackageBundleStatus) {
		callbackCalled = true
		lastBlockNumber = bn
		lastOldStatus = old
		lastNewStatus = new
	})

	bundle := &types.WorkPackageBundle{
		WorkPackage: types.WorkPackage{},
	}

	bn, _ := qs.Enqueue(bundle)
	wpHash := common.Hash{1}

	item := qs.Dequeue()
	qs.MarkSubmitted(item, wpHash)

	if !callbackCalled {
		t.Error("Expected callback to be called")
	}
	if lastBlockNumber != bn {
		t.Errorf("Expected block number %d, got %d", bn, lastBlockNumber)
	}
	if lastOldStatus != StatusQueued {
		t.Errorf("Expected old status Queued, got %v", lastOldStatus)
	}
	if lastNewStatus != StatusSubmitted {
		t.Errorf("Expected new status Submitted, got %v", lastNewStatus)
	}
}

func TestCheckTimeouts(t *testing.T) {
	config := DefaultConfig()
	config.SubmissionTimeout = 10 * time.Millisecond // Very short for testing
	qs := NewQueueStateWithConfig(0, config)

	bundle := &types.WorkPackageBundle{
		WorkPackage: types.WorkPackage{},
	}

	bn, _ := qs.Enqueue(bundle)
	wpHash := common.Hash{1}

	item := qs.Dequeue()
	qs.MarkSubmitted(item, wpHash)

	// Wait for timeout
	time.Sleep(20 * time.Millisecond)

	// Check timeouts should requeue
	qs.CheckTimeouts()

	// Item should be back in queue with new version
	if qs.Status[bn] != StatusQueued {
		t.Errorf("Expected StatusQueued after timeout, got %v", qs.Status[bn])
	}
	if qs.CurrentVer[bn] != 2 {
		t.Errorf("Expected version 2 after timeout, got %d", qs.CurrentVer[bn])
	}
}
