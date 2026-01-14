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

	bn, err := qs.Enqueue(bundle, 0) // Pass coreIndex
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
	_, err := qs.Enqueue(bundle, 0)
	if err != nil {
		t.Fatalf("First enqueue failed: %v", err)
	}
	_, err = qs.Enqueue(bundle, 0)
	if err != nil {
		t.Fatalf("Second enqueue failed: %v", err)
	}

	// Should fail - queue full
	_, err = qs.Enqueue(bundle, 0)
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

	qs.Enqueue(bundle1, 0)
	qs.Enqueue(bundle2, 1)

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
	qs.Enqueue(bundle, 0)
	qs.Enqueue(bundle, 1)
	qs.Enqueue(bundle, 0)

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

	// After guaranteed, core is FREE - inflight() only counts Submitted
	qs.OnGuaranteed(common.Hash{1})
	item3 = qs.Dequeue()
	if item3 == nil {
		t.Error("Expected to dequeue after guarantee (core freed)")
	}
}

func TestStatusTransitions(t *testing.T) {
	qs := NewQueueState(0)

	bundle := &types.WorkPackageBundle{
		WorkPackage: types.WorkPackage{},
	}

	bn, _ := qs.Enqueue(bundle, 0)
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

func TestOldVersionAccepted(t *testing.T) {
	qs := NewQueueState(0)

	bundle := &types.WorkPackageBundle{
		WorkPackage: types.WorkPackage{},
	}

	bn, _ := qs.Enqueue(bundle, 0)
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

	// Old hash (v1) guaranteed - should be ACCEPTED as winner (first guarantee wins)
	qs.OnGuaranteed(wpHash1)
	if qs.Status[bn] != StatusGuaranteed {
		t.Errorf("Old version guarantee should be accepted as winner, expected StatusGuaranteed, got %v", qs.Status[bn])
	}

	// Winning version should be set to v1
	if qs.WinningVer[bn] != 1 {
		t.Errorf("Expected WinningVer 1, got %d", qs.WinningVer[bn])
	}

	// Try to guarantee v2 - should be REJECTED
	qs.OnGuaranteed(wpHash2)
	if qs.WinningVer[bn] != 1 {
		t.Errorf("WinningVer should remain 1 after v2 guarantee attempt, got %d", qs.WinningVer[bn])
	}

	// Try to accumulate v2 - should be REJECTED
	qs.OnAccumulated(wpHash2)
	if qs.Status[bn] == StatusAccumulated {
		t.Error("v2 accumulation should be rejected, but status changed to Accumulated")
	}

	// Accumulate v1 - should succeed
	qs.OnAccumulated(wpHash1)
	if qs.Status[bn] != StatusAccumulated {
		t.Errorf("v1 accumulation should succeed, expected StatusAccumulated, got %v", qs.Status[bn])
	}
}

func TestMaxVersionRetries(t *testing.T) {
	config := DefaultConfig()
	config.MaxVersionRetries = 2
	qs := NewQueueStateWithConfig(0, config)

	bundle := &types.WorkPackageBundle{
		WorkPackage: types.WorkPackage{},
	}

	bn, _ := qs.Enqueue(bundle, 0)

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

	bn, _ := qs.Enqueue(bundle, 0)
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
	config.GuaranteeTimeout = 10 * time.Millisecond // Very short for testing
	qs := NewQueueStateWithConfig(0, config)

	bundle := &types.WorkPackageBundle{
		WorkPackage: types.WorkPackage{},
	}

	bn, _ := qs.Enqueue(bundle, 0)
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

func TestDuplicateExecutionPrevention(t *testing.T) {
	qs := NewQueueState(0)

	bundle := &types.WorkPackageBundle{
		WorkPackage: types.WorkPackage{},
	}

	bn, _ := qs.Enqueue(bundle, 0)
	wpHashV1 := common.Hash{1, 0, 0, 0}
	wpHashV2 := common.Hash{2, 0, 0, 0}

	// Submit v1
	item := qs.Dequeue()
	qs.MarkSubmitted(item, wpHashV1)

	// Timeout and requeue as v2
	qs.OnTimeoutOrFailure(bn)

	// Submit v2
	item = qs.Dequeue()
	qs.MarkSubmitted(item, wpHashV2)

	// v2 gets guaranteed first (race condition)
	qs.OnGuaranteed(wpHashV2)
	if qs.WinningVer[bn] != 2 {
		t.Errorf("Expected WinningVer=2 after v2 guarantee, got %d", qs.WinningVer[bn])
	}

	// Late v1 guarantee arrives - should be REJECTED
	qs.OnGuaranteed(wpHashV1)
	if qs.WinningVer[bn] != 2 {
		t.Errorf("WinningVer should remain 2 after late v1 guarantee, got %d", qs.WinningVer[bn])
	}

	// Both v1 and v2 try to accumulate - only v2 should succeed
	qs.OnAccumulated(wpHashV1)
	if qs.Status[bn] == StatusAccumulated {
		t.Error("v1 should not accumulate (not the winner)")
	}

	qs.OnAccumulated(wpHashV2)
	if qs.Status[bn] != StatusAccumulated {
		t.Errorf("v2 should accumulate (the winner), got %v", qs.Status[bn])
	}

	// Verify item is in Finalized
	if _, ok := qs.Finalized[bn]; !ok {
		t.Error("Expected item in Finalized map")
	}
}

func TestFirstGuaranteeWins(t *testing.T) {
	qs := NewQueueState(0)

	bundle := &types.WorkPackageBundle{
		WorkPackage: types.WorkPackage{},
	}

	bn, _ := qs.Enqueue(bundle, 0)
	wpHashV1 := common.Hash{1, 1, 1}
	wpHashV2 := common.Hash{2, 2, 2}
	wpHashV3 := common.Hash{3, 3, 3}

	// Submit v1
	item := qs.Dequeue()
	qs.MarkSubmitted(item, wpHashV1)
	qs.OnTimeoutOrFailure(bn)

	// Submit v2
	item = qs.Dequeue()
	qs.MarkSubmitted(item, wpHashV2)
	qs.OnTimeoutOrFailure(bn)

	// Submit v3
	item = qs.Dequeue()
	qs.MarkSubmitted(item, wpHashV3)

	// v1 guarantee arrives first (late but first)
	qs.OnGuaranteed(wpHashV1)
	if qs.WinningVer[bn] != 1 {
		t.Errorf("Expected WinningVer=1, got %d", qs.WinningVer[bn])
	}

	// v2 and v3 guarantees should be rejected
	qs.OnGuaranteed(wpHashV2)
	qs.OnGuaranteed(wpHashV3)
	if qs.WinningVer[bn] != 1 {
		t.Errorf("WinningVer should remain 1, got %d", qs.WinningVer[bn])
	}

	// Only v1 can accumulate
	qs.OnAccumulated(wpHashV1)
	if qs.Status[bn] != StatusAccumulated {
		t.Errorf("v1 should accumulate, got %v", qs.Status[bn])
	}
}

func TestFastRetry(t *testing.T) {
	config := DefaultConfig()
	config.SubmitRetryInterval = 10 * time.Millisecond // Very short for testing
	config.MaxSubmitRetries = 3
	qs := NewQueueStateWithConfig(0, config)

	bundle := &types.WorkPackageBundle{
		WorkPackage: types.WorkPackage{},
	}

	bn, _ := qs.Enqueue(bundle, 0)
	wpHash := common.Hash{1, 2, 3}

	// Submit v1
	item := qs.Dequeue()
	qs.MarkSubmitted(item, wpHash)

	if item.SubmitAttempts != 1 {
		t.Errorf("Expected SubmitAttempts=1, got %d", item.SubmitAttempts)
	}

	now := time.Now()
	item.SubmittedAt = now.Add(-25 * time.Millisecond)
	item.LastSubmitAt = now.Add(-25 * time.Millisecond)

	// CheckTimeouts should trigger fast retry (requeue same version)
	qs.CheckTimeouts()

	// Should be back in queue with same version
	if qs.Status[bn] != StatusQueued {
		t.Errorf("Expected StatusQueued after fast retry, got %v", qs.Status[bn])
	}

	queuedItem := qs.Queued[bn]
	if queuedItem == nil {
		t.Fatal("Expected item in queue after fast retry")
	}
	if queuedItem.Version != 1 {
		t.Errorf("Expected version 1 (same version retry), got %d", queuedItem.Version)
	}
	if queuedItem.SubmitAttempts != 1 {
		t.Errorf("Expected SubmitAttempts=1 preserved, got %d", queuedItem.SubmitAttempts)
	}

	// Retry submission #2
	item = qs.Dequeue()
	qs.MarkSubmitted(item, wpHash)

	if item.SubmitAttempts != 2 {
		t.Errorf("Expected SubmitAttempts=2, got %d", item.SubmitAttempts)
	}
	if item.WPHash != wpHash {
		t.Errorf("Expected same hash on retry, got different hash")
	}

	now = time.Now()
	item.SubmittedAt = now.Add(-25 * time.Millisecond)
	item.LastSubmitAt = now.Add(-25 * time.Millisecond)
	qs.CheckTimeouts()

	// Should be back in queue for attempt #3
	if qs.Status[bn] != StatusQueued {
		t.Errorf("Expected StatusQueued for retry #3, got %v", qs.Status[bn])
	}

	// Retry submission #3
	item = qs.Dequeue()
	qs.MarkSubmitted(item, wpHash)

	if item.SubmitAttempts != 3 {
		t.Errorf("Expected SubmitAttempts=3, got %d", item.SubmitAttempts)
	}

	// Wait and try to retry again - should NOT retry (max attempts reached)
	now = time.Now()
	item.SubmittedAt = now.Add(-25 * time.Millisecond)
	item.LastSubmitAt = now.Add(-25 * time.Millisecond)
	qs.CheckTimeouts()

	// Should still be in Submitted status (no more fast retries)
	if qs.Status[bn] != StatusSubmitted {
		t.Errorf("Expected StatusSubmitted after max fast retries, got %v", qs.Status[bn])
	}
}

func TestFastRetryVsSlowTimeout(t *testing.T) {
	config := DefaultConfig()
	config.SubmitRetryInterval = 10 * time.Millisecond
	config.MaxSubmitRetries = 3 // 3 total attempts: 1 initial + 2 retries
	config.GuaranteeTimeout = 50 * time.Millisecond // Short for testing
	qs := NewQueueStateWithConfig(0, config)

	bundle := &types.WorkPackageBundle{
		WorkPackage: types.WorkPackage{},
	}

	bn, _ := qs.Enqueue(bundle, 0)
	wpHash := common.Hash{1, 2, 3}

	// Submit v1 (attempt #1)
	item := qs.Dequeue()
	qs.MarkSubmitted(item, wpHash)
	if item.SubmitAttempts != 1 {
		t.Errorf("Expected SubmitAttempts=1, got %d", item.SubmitAttempts)
	}

	// Fast retry #1
	now := time.Now()
	item.SubmittedAt = now.Add(-25 * time.Millisecond)
	item.LastSubmitAt = now.Add(-25 * time.Millisecond)
	qs.CheckTimeouts()
	if qs.Status[bn] != StatusQueued || qs.CurrentVer[bn] != 1 {
		t.Error("Expected fast retry with same version")
	}

	// Submit again (attempt #2)
	item = qs.Dequeue()
	qs.MarkSubmitted(item, wpHash)
	if item.SubmitAttempts != 2 {
		t.Errorf("Expected SubmitAttempts=2, got %d", item.SubmitAttempts)
	}

	// Fast retry #2
	now = time.Now()
	item.SubmittedAt = now.Add(-25 * time.Millisecond)
	item.LastSubmitAt = now.Add(-25 * time.Millisecond)
	qs.CheckTimeouts()
	if qs.Status[bn] != StatusQueued || qs.CurrentVer[bn] != 1 {
		t.Error("Expected fast retry #2 with same version")
	}

	// Submit again (attempt #3 - last retry)
	item = qs.Dequeue()
	if item == nil {
		t.Fatal("Expected item to dequeue for attempt #3")
	}
	qs.MarkSubmitted(item, wpHash)
	if item.SubmitAttempts != 3 {
		t.Errorf("Expected SubmitAttempts=3, got %d", item.SubmitAttempts)
	}

	// Try fast retry again - should NOT retry (max attempts reached)
	now = time.Now()
	item.SubmittedAt = now.Add(-25 * time.Millisecond)
	item.LastSubmitAt = now.Add(-25 * time.Millisecond)
	qs.CheckTimeouts()
	if qs.Status[bn] != StatusSubmitted {
		t.Errorf("Expected StatusSubmitted (no more fast retries), got %v", qs.Status[bn])
	}

	// Wait for GuaranteeTimeout to trigger slow timeout (version increment)
	now = time.Now()
	item.SubmittedAt = now.Add(-60 * time.Millisecond)
	item.LastSubmitAt = now.Add(-60 * time.Millisecond)
	qs.CheckTimeouts()

	// Should now increment version (slow timeout)
	if qs.Status[bn] != StatusQueued {
		t.Errorf("Expected StatusQueued after slow timeout, got %v", qs.Status[bn])
	}
	if qs.CurrentVer[bn] != 2 {
		t.Errorf("Expected version incremented to 2, got %d", qs.CurrentVer[bn])
	}

	queuedItem := qs.Queued[bn]
	if queuedItem == nil {
		t.Fatal("Expected item in queue after slow timeout")
	}
	if queuedItem.Version != 2 {
		t.Errorf("Expected version 2 after slow timeout, got %d", queuedItem.Version)
	}
	if queuedItem.SubmitAttempts != 0 {
		t.Errorf("Expected SubmitAttempts reset to 0 for new version, got %d", queuedItem.SubmitAttempts)
	}
}
