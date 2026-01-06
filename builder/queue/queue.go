// Package queue provides work package queue management for JAM rollup builders.
// It handles the lifecycle of work packages from creation through finalization,
// supporting parallel submission and automatic resubmission on failures.
package queue

import (
	"fmt"
	"sync"
	"time"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
)

// WorkPackageBundleStatus represents the current state of a work package in the pipeline
type WorkPackageBundleStatus int

const (
	// StatusQueued means the work package is waiting to be submitted
	StatusQueued WorkPackageBundleStatus = iota
	// StatusSubmitted means the work package has been sent to a guarantor (CE146)
	StatusSubmitted
	// StatusGuaranteed means the work package has received sufficient guarantees
	StatusGuaranteed
	// StatusAccumulated means the work package has been accumulated into chain state
	StatusAccumulated
	// StatusFinalized means the work package is finalized and immutable
	StatusFinalized
	// StatusFailed means the work package failed and will not be retried
	StatusFailed
)

func (s WorkPackageBundleStatus) String() string {
	switch s {
	case StatusQueued:
		return "Queued"
	case StatusSubmitted:
		return "Submitted"
	case StatusGuaranteed:
		return "Guaranteed"
	case StatusAccumulated:
		return "Accumulated"
	case StatusFinalized:
		return "Finalized"
	case StatusFailed:
		return "Failed"
	default:
		return fmt.Sprintf("Unknown(%d)", s)
	}
}

// Configuration parameters for the queue
const (
	DefaultMaxQueueDepth     = 8           // Maximum items waiting to be submitted
	DefaultMaxInflight       = 3           // Maximum items in Submitted or Guaranteed state
	DefaultSubmissionTimeout = 30 * time.Second // Time before Submitted item is considered failed
	DefaultGuaranteeTimeout  = 18 * time.Second // Time before Guaranteed item is considered failed (3 JAM blocks)
	DefaultRetentionWindow   = 100         // Number of finalized blocks to retain
	DefaultMaxVersionRetries = 5           // Maximum resubmission attempts before dropping
)

// QueueConfig holds configuration for the work package queue
type QueueConfig struct {
	MaxQueueDepth     int
	MaxInflight       int
	SubmissionTimeout time.Duration
	GuaranteeTimeout  time.Duration
	RetentionWindow   int
	MaxVersionRetries int
}

// DefaultConfig returns the default queue configuration
func DefaultConfig() QueueConfig {
	return QueueConfig{
		MaxQueueDepth:     DefaultMaxQueueDepth,
		MaxInflight:       DefaultMaxInflight,
		SubmissionTimeout: DefaultSubmissionTimeout,
		GuaranteeTimeout:  DefaultGuaranteeTimeout,
		RetentionWindow:   DefaultRetentionWindow,
		MaxVersionRetries: DefaultMaxVersionRetries,
	}
}

// QueueItem represents a work package in the queue with version tracking
type QueueItem struct {
	BlockNumber uint64                   // Rollup block number (may be provisional)
	Version     int                      // Version for resubmission tracking
	EventID     uint64                   // Telemetry event ID
	AddTS       time.Time                // When item was added to queue
	Bundle      *types.WorkPackageBundle // The actual bundle to submit
	WPHash      common.Hash              // Work package hash (computed after BuildBundle)
	SubmittedAt time.Time                // When item was submitted (zero if not submitted)
	Status      WorkPackageBundleStatus  // Current status

	// OriginalExtrinsics stores the transaction extrinsics BEFORE Verkle witness prepending.
	// Used for rebuilding bundles on resubmission - BuildBundle prepends a fresh witness,
	// so we need the original txs to avoid double-prepending.
	OriginalExtrinsics []types.ExtrinsicsBlobs

	// OriginalWorkItemExtrinsics stores the WorkItems[].Extrinsics metadata BEFORE Verkle witness prepending.
	// BuildBundle also prepends metadata entries, so we need to restore the original metadata on rebuild.
	OriginalWorkItemExtrinsics [][]types.WorkItemExtrinsic
}

// QueueState manages the work package submission pipeline
type QueueState struct {
	mu sync.RWMutex

	// Configuration
	config QueueConfig

	// Service identification
	serviceID uint32

	// Queue of items waiting to be submitted (keyed by block number)
	Queued map[uint64]*QueueItem

	// Items that have been submitted or are in-flight
	Inflight map[uint64]*QueueItem

	// Status tracking by block number
	Status map[uint64]WorkPackageBundleStatus

	// Version tracking for resubmissions
	CurrentVer map[uint64]int

	// Hash tracking by version: blockNumber -> version -> wpHash
	HashByVer map[uint64]map[int]common.Hash

	// Finalized items (for verification and retention)
	Finalized map[uint64]*QueueItem

	// Next block number to assign
	nextBlockNumber uint64

	// Event ID generator
	eventIDCounter uint64

	// Callbacks for queue events
	onStatusChange func(blockNumber uint64, oldStatus, newStatus WorkPackageBundleStatus)
}

// NewQueueState creates a new queue state with default configuration
func NewQueueState(serviceID uint32) *QueueState {
	return NewQueueStateWithConfig(serviceID, DefaultConfig())
}

// NewQueueStateWithConfig creates a new queue state with custom configuration
func NewQueueStateWithConfig(serviceID uint32, config QueueConfig) *QueueState {
	return &QueueState{
		config:          config,
		serviceID:       serviceID,
		Queued:          make(map[uint64]*QueueItem),
		Inflight:        make(map[uint64]*QueueItem),
		Status:          make(map[uint64]WorkPackageBundleStatus),
		CurrentVer:      make(map[uint64]int),
		HashByVer:       make(map[uint64]map[int]common.Hash),
		Finalized:       make(map[uint64]*QueueItem),
		nextBlockNumber: 1, // Start from block 1 (0 is genesis)
		eventIDCounter:  0,
	}
}

// SetStatusChangeCallback sets a callback for status changes
func (qs *QueueState) SetStatusChangeCallback(cb func(blockNumber uint64, oldStatus, newStatus WorkPackageBundleStatus)) {
	qs.mu.Lock()
	defer qs.mu.Unlock()
	qs.onStatusChange = cb
}

// nextEventID generates a unique event ID
func (qs *QueueState) nextEventID() uint64 {
	qs.eventIDCounter++
	return qs.eventIDCounter
}

// Enqueue adds a new bundle to the queue
// Returns the assigned block number and error if queue is full
func (qs *QueueState) Enqueue(bundle *types.WorkPackageBundle) (uint64, error) {
	qs.mu.Lock()
	defer qs.mu.Unlock()

	if len(qs.Queued) >= qs.config.MaxQueueDepth {
		return 0, fmt.Errorf("queue full: %d items queued (max %d)", len(qs.Queued), qs.config.MaxQueueDepth)
	}

	blockNumber := qs.nextBlockNumber
	qs.nextBlockNumber++

	item := &QueueItem{
		BlockNumber: blockNumber,
		Version:     1,
		EventID:     qs.nextEventID(),
		AddTS:       time.Now(),
		Bundle:      bundle,
		WPHash:      bundle.WorkPackage.Hash(),
		Status:      StatusQueued,
	}

	qs.Queued[blockNumber] = item
	qs.Status[blockNumber] = StatusQueued
	qs.CurrentVer[blockNumber] = 1

	if qs.HashByVer[blockNumber] == nil {
		qs.HashByVer[blockNumber] = make(map[int]common.Hash)
	}
	qs.HashByVer[blockNumber][1] = item.WPHash

	log.Info(log.Node, "Queue: Enqueued work package",
		"service", qs.serviceID,
		"blockNumber", blockNumber,
		"wpHash", item.WPHash.Hex(),
		"queueSize", len(qs.Queued))

	return blockNumber, nil
}

// EnqueueWithOriginalExtrinsics adds a new bundle to the queue along with original extrinsics
// The originalExtrinsics are the transaction extrinsics BEFORE Verkle witness prepending,
// used for rebuilding bundles on resubmission without double-prepending the witness.
// The originalWorkItemExtrinsics are the WorkItems[].Extrinsics metadata BEFORE Verkle witness prepending.
func (qs *QueueState) EnqueueWithOriginalExtrinsics(bundle *types.WorkPackageBundle, originalExtrinsics []types.ExtrinsicsBlobs, originalWorkItemExtrinsics [][]types.WorkItemExtrinsic) (uint64, error) {
	qs.mu.Lock()
	defer qs.mu.Unlock()

	if len(qs.Queued) >= qs.config.MaxQueueDepth {
		return 0, fmt.Errorf("queue full: %d items queued (max %d)", len(qs.Queued), qs.config.MaxQueueDepth)
	}

	blockNumber := qs.nextBlockNumber
	qs.nextBlockNumber++

	item := &QueueItem{
		BlockNumber:                blockNumber,
		Version:                    1,
		EventID:                    qs.nextEventID(),
		AddTS:                      time.Now(),
		Bundle:                     bundle,
		WPHash:                     bundle.WorkPackage.Hash(),
		Status:                     StatusQueued,
		OriginalExtrinsics:         originalExtrinsics,
		OriginalWorkItemExtrinsics: originalWorkItemExtrinsics,
	}

	qs.Queued[blockNumber] = item
	qs.Status[blockNumber] = StatusQueued
	qs.CurrentVer[blockNumber] = 1

	if qs.HashByVer[blockNumber] == nil {
		qs.HashByVer[blockNumber] = make(map[int]common.Hash)
	}
	qs.HashByVer[blockNumber][1] = item.WPHash

	log.Info(log.Node, "Queue: Enqueued work package with original extrinsics",
		"service", qs.serviceID,
		"blockNumber", blockNumber,
		"wpHash", item.WPHash.Hex(),
		"queueSize", len(qs.Queued))

	return blockNumber, nil
}

// inflight returns the count of items in Submitted or Guaranteed state
func (qs *QueueState) inflight() int {
	count := 0
	for _, status := range qs.Status {
		if status == StatusSubmitted || status == StatusGuaranteed {
			count++
		}
	}
	return count
}

// CanSubmit returns true if another item can be submitted
func (qs *QueueState) CanSubmit() bool {
	qs.mu.RLock()
	defer qs.mu.RUnlock()
	return qs.inflight() < qs.config.MaxInflight && len(qs.Queued) > 0
}

// Dequeue removes the lowest block number item from the queue for submission
// Returns nil if queue is empty or inflight limit reached
func (qs *QueueState) Dequeue() *QueueItem {
	qs.mu.Lock()
	defer qs.mu.Unlock()

	if qs.inflight() >= qs.config.MaxInflight {
		return nil
	}

	if len(qs.Queued) == 0 {
		return nil
	}

	// Find lowest block number
	var lowestBN uint64
	var found bool
	for bn := range qs.Queued {
		if !found || bn < lowestBN {
			lowestBN = bn
			found = true
		}
	}

	if !found {
		return nil
	}

	item := qs.Queued[lowestBN]
	delete(qs.Queued, lowestBN)

	return item
}

// MarkSubmitted marks an item as submitted and moves it to inflight
func (qs *QueueState) MarkSubmitted(item *QueueItem, wpHash common.Hash) {
	qs.mu.Lock()
	defer qs.mu.Unlock()

	oldStatus := qs.Status[item.BlockNumber]
	item.Status = StatusSubmitted
	item.SubmittedAt = time.Now()
	item.WPHash = wpHash

	qs.Inflight[item.BlockNumber] = item
	qs.Status[item.BlockNumber] = StatusSubmitted
	qs.HashByVer[item.BlockNumber][item.Version] = wpHash

	if qs.onStatusChange != nil {
		qs.onStatusChange(item.BlockNumber, oldStatus, StatusSubmitted)
	}

	log.Info(log.Node, "Queue: Marked submitted",
		"service", qs.serviceID,
		"blockNumber", item.BlockNumber,
		"version", item.Version,
		"wpHash", wpHash.Hex())
}

// OnGuaranteed handles a guarantee event for a work package hash
func (qs *QueueState) OnGuaranteed(wpHash common.Hash) {
	qs.mu.Lock()
	defer qs.mu.Unlock()

	bn, ver := qs.lookupByHash(wpHash)
	if bn == 0 {
		log.Debug(log.Node, "Queue: OnGuaranteed for unknown hash", "wpHash", wpHash.Hex())
		return
	}

	// Check if this is the current version
	if ver != qs.CurrentVer[bn] {
		log.Debug(log.Node, "Queue: OnGuaranteed for stale version",
			"blockNumber", bn,
			"version", ver,
			"currentVersion", qs.CurrentVer[bn])
		return
	}

	oldStatus := qs.Status[bn]
	qs.Status[bn] = StatusGuaranteed

	if item, ok := qs.Inflight[bn]; ok {
		item.Status = StatusGuaranteed
	}

	if qs.onStatusChange != nil {
		qs.onStatusChange(bn, oldStatus, StatusGuaranteed)
	}

	log.Info(log.Node, "Queue: Marked guaranteed",
		"service", qs.serviceID,
		"blockNumber", bn,
		"version", ver,
		"wpHash", wpHash.Hex())
}

// OnAccumulated handles an accumulation event for a work package hash
func (qs *QueueState) OnAccumulated(wpHash common.Hash) {
	qs.mu.Lock()
	defer qs.mu.Unlock()

	bn, ver := qs.lookupByHash(wpHash)
	if bn == 0 {
		log.Debug(log.Node, "Queue: OnAccumulated for unknown hash", "wpHash", wpHash.Hex())
		return
	}

	// Check if this is the current version
	if ver != qs.CurrentVer[bn] {
		log.Debug(log.Node, "Queue: OnAccumulated for stale version",
			"blockNumber", bn,
			"version", ver,
			"currentVersion", qs.CurrentVer[bn])
		return
	}

	oldStatus := qs.Status[bn]
	qs.Status[bn] = StatusAccumulated

	if item, ok := qs.Inflight[bn]; ok {
		item.Status = StatusAccumulated
	}

	if qs.onStatusChange != nil {
		qs.onStatusChange(bn, oldStatus, StatusAccumulated)
	}

	// Prune old finalized items
	qs.pruneOlder(bn)

	log.Info(log.Node, "Queue: Marked accumulated",
		"service", qs.serviceID,
		"blockNumber", bn,
		"version", ver,
		"wpHash", wpHash.Hex())
}

// OnFinalized handles a finalization event for a work package hash
func (qs *QueueState) OnFinalized(wpHash common.Hash) {
	qs.mu.Lock()
	defer qs.mu.Unlock()

	bn, ver := qs.lookupByHash(wpHash)
	if bn == 0 {
		log.Debug(log.Node, "Queue: OnFinalized for unknown hash", "wpHash", wpHash.Hex())
		return
	}

	// Check if this is the current version
	if ver != qs.CurrentVer[bn] {
		log.Debug(log.Node, "Queue: OnFinalized for stale version",
			"blockNumber", bn,
			"version", ver,
			"currentVersion", qs.CurrentVer[bn])
		return
	}

	oldStatus := qs.Status[bn]
	qs.Status[bn] = StatusFinalized

	// Move from inflight to finalized
	if item, ok := qs.Inflight[bn]; ok {
		item.Status = StatusFinalized
		qs.Finalized[bn] = item
		delete(qs.Inflight, bn)
	}

	if qs.onStatusChange != nil {
		qs.onStatusChange(bn, oldStatus, StatusFinalized)
	}

	log.Info(log.Node, "Queue: Marked finalized",
		"service", qs.serviceID,
		"blockNumber", bn,
		"version", ver,
		"wpHash", wpHash.Hex())
}

// OnTimeoutOrFailure handles timeout or failure for a block number
// It requeues the failed block and all subsequent blocks with new versions
func (qs *QueueState) OnTimeoutOrFailure(failedBN uint64) {
	qs.mu.Lock()
	defer qs.mu.Unlock()

	now := time.Now()

	// Find all block numbers that need to be requeued (failedBN and all subsequent in-flight)
	var toRequeue []uint64
	for bn := range qs.Inflight {
		if bn >= failedBN {
			toRequeue = append(toRequeue, bn)
		}
	}

	for _, bn := range toRequeue {
		item := qs.Inflight[bn]
		if item == nil {
			continue
		}

		// Check retry limit
		newVersion := qs.CurrentVer[bn] + 1
		if newVersion > qs.config.MaxVersionRetries {
			log.Warn(log.Node, "Queue: Max retries exceeded, dropping block",
				"service", qs.serviceID,
				"blockNumber", bn,
				"version", newVersion-1)
			qs.Status[bn] = StatusFailed
			delete(qs.Inflight, bn)
			continue
		}

		// Increment version and requeue
		qs.CurrentVer[bn] = newVersion
		delete(qs.Status, bn) // Clear status (will be set to Queued)

		item.Version = newVersion
		item.AddTS = now
		item.Status = StatusQueued
		item.SubmittedAt = time.Time{} // Reset submission time

		qs.Queued[bn] = item
		qs.Status[bn] = StatusQueued
		delete(qs.Inflight, bn)

		log.Info(log.Node, "Queue: Requeued after failure",
			"service", qs.serviceID,
			"blockNumber", bn,
			"newVersion", newVersion)
	}
}

// lookupByHash finds block number and version for a given work package hash
func (qs *QueueState) lookupByHash(wpHash common.Hash) (blockNumber uint64, version int) {
	for bn, versions := range qs.HashByVer {
		for ver, hash := range versions {
			if hash == wpHash {
				return bn, ver
			}
		}
	}
	return 0, 0
}

// pruneOlder removes finalized items older than retention window
func (qs *QueueState) pruneOlder(currentBN uint64) {
	if currentBN <= uint64(qs.config.RetentionWindow) {
		return
	}

	cutoff := currentBN - uint64(qs.config.RetentionWindow)
	for bn := range qs.Finalized {
		if bn < cutoff {
			delete(qs.Finalized, bn)
			delete(qs.Status, bn)
			delete(qs.CurrentVer, bn)
			delete(qs.HashByVer, bn)
		}
	}
}

// CheckTimeouts checks for timed out submissions and triggers requeuing
func (qs *QueueState) CheckTimeouts() {
	qs.mu.Lock()
	defer qs.mu.Unlock()

	now := time.Now()

	for bn, item := range qs.Inflight {
		if item.SubmittedAt.IsZero() {
			continue
		}

		var timeout time.Duration
		switch item.Status {
		case StatusSubmitted:
			timeout = qs.config.SubmissionTimeout
		case StatusGuaranteed:
			timeout = qs.config.GuaranteeTimeout
		default:
			continue
		}

		if now.Sub(item.SubmittedAt) > timeout {
			log.Warn(log.Node, "Queue: Timeout detected",
				"service", qs.serviceID,
				"blockNumber", bn,
				"status", item.Status.String(),
				"submittedAt", item.SubmittedAt)

			// Unlock and call OnTimeoutOrFailure (which will relock)
			qs.mu.Unlock()
			qs.OnTimeoutOrFailure(bn)
			qs.mu.Lock()
			return // Restart checking after modification
		}
	}
}

// Stats returns queue statistics
type QueueStats struct {
	QueuedCount     int
	InflightCount   int
	FinalizedCount  int
	SubmittedCount  int
	GuaranteedCount int
	AccumulatedCount int
}

// GetStats returns current queue statistics
func (qs *QueueState) GetStats() QueueStats {
	qs.mu.RLock()
	defer qs.mu.RUnlock()

	stats := QueueStats{
		QueuedCount:    len(qs.Queued),
		InflightCount:  len(qs.Inflight),
		FinalizedCount: len(qs.Finalized),
	}

	for _, status := range qs.Status {
		switch status {
		case StatusSubmitted:
			stats.SubmittedCount++
		case StatusGuaranteed:
			stats.GuaranteedCount++
		case StatusAccumulated:
			stats.AccumulatedCount++
		}
	}

	return stats
}

// GetItemByBlockNumber returns the queue item for a block number
func (qs *QueueState) GetItemByBlockNumber(bn uint64) *QueueItem {
	qs.mu.RLock()
	defer qs.mu.RUnlock()

	if item, ok := qs.Queued[bn]; ok {
		return item
	}
	if item, ok := qs.Inflight[bn]; ok {
		return item
	}
	if item, ok := qs.Finalized[bn]; ok {
		return item
	}
	return nil
}

// GetItemByHash returns the queue item for a work package hash
func (qs *QueueState) GetItemByHash(wpHash common.Hash) *QueueItem {
	qs.mu.RLock()
	defer qs.mu.RUnlock()

	bn, _ := qs.lookupByHash(wpHash)
	if bn == 0 {
		return nil
	}

	if item, ok := qs.Queued[bn]; ok {
		return item
	}
	if item, ok := qs.Inflight[bn]; ok {
		return item
	}
	if item, ok := qs.Finalized[bn]; ok {
		return item
	}
	return nil
}
