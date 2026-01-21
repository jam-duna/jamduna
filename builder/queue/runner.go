package queue

import (
	"context"
	"sync"
	"time"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
)

// Submitter is a callback for submitting a work package bundle to a specific core.
// Returns the work package hash and error.
type Submitter func(bundle *types.WorkPackageBundle, coreIndex uint16) (common.Hash, error)

// BundleBuilder is a callback for building/rebuilding a bundle with fresh RefineContext.
// This is called when an item needs to be resubmitted due to timeout or failure.
// The QueueStats parameter allows the callback to calculate dynamic anchor offset.
type BundleBuilder func(item *QueueItem, stats QueueStats) (*types.WorkPackageBundle, error)

// Runner manages the queue submission loop
type Runner struct {
	mu sync.RWMutex

	queue     *QueueState
	serviceID uint32

	// Callbacks
	submitter     Submitter
	bundleBuilder BundleBuilder

	// Control
	tickInterval time.Duration
	stopCh       chan struct{}
	running      bool

	// Submission window timing (seconds before next timeslot to start submission)
	submissionWindowStart int // e.g., 3 seconds before timeslot
	submissionWindowEnd   int // e.g., 1 second before timeslot

	// Contiguous accumulation tracking for canonical commits
	accumulatedRoots  map[uint64]common.Hash // blockNumber -> postRoot
	highestContiguous uint64                 // highest block where 1..N are all accumulated

	// Pending head tracking: the postRoot of the most recent Phase 1 block
	// This is used to chain Phase 1 blocks across multiple batches, even before
	// bundles accumulate (which updates canonicalRoot). Without this, each new
	// batch would start from canonicalRoot and re-execute the same transactions.
	pendingHeadRoot common.Hash
}

// RunnerConfig holds configuration for the runner
type RunnerConfig struct {
	TickInterval          time.Duration
	SubmissionWindowStart int // Seconds before timeslot to start submission window
	SubmissionWindowEnd   int // Seconds before timeslot to end submission window
}

// DefaultRunnerConfig returns default runner configuration
func DefaultRunnerConfig() RunnerConfig {
	return RunnerConfig{
		TickInterval:          time.Second, // Check every second
		SubmissionWindowStart: 3,           // Start 3 seconds before timeslot
		SubmissionWindowEnd:   1,           // End 1 second before timeslot
	}
}

// NewRunner creates a new queue runner
func NewRunner(queue *QueueState, serviceID uint32, submitter Submitter, bundleBuilder BundleBuilder) *Runner {
	config := DefaultRunnerConfig()
	return &Runner{
		queue:                 queue,
		serviceID:             serviceID,
		submitter:             submitter,
		bundleBuilder:         bundleBuilder,
		tickInterval:          config.TickInterval,
		submissionWindowStart: config.SubmissionWindowStart,
		submissionWindowEnd:   config.SubmissionWindowEnd,
		stopCh:                make(chan struct{}),
		accumulatedRoots:      make(map[uint64]common.Hash),
	}
}

// Start begins the queue processing loop
func (r *Runner) Start(ctx context.Context) {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return
	}
	r.running = true
	r.stopCh = make(chan struct{})
	r.mu.Unlock()

	log.Info(log.Node, "Queue Runner: Starting",
		"service", r.serviceID,
		"tickInterval", r.tickInterval)

	go r.runLoop(ctx)
}

// Stop stops the queue processing loop
func (r *Runner) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.running {
		return
	}

	close(r.stopCh)
	r.running = false

	log.Info(log.Node, "Queue Runner: Stopped", "service", r.serviceID)
}

// SetOnAccumulated sets a callback to be invoked when bundles accumulate
// This callback receives the work package hash, block number, and transaction hashes
// Thread-safe: can be called before or after runner starts
func (r *Runner) SetOnAccumulated(callback func(wpHash common.Hash, blockNumber uint64, txHashes []common.Hash)) {
	r.queue.SetOnAccumulated(callback)
}

// RootBasedStorage is the interface for root-first state management.
// Used by SetOnAccumulatedWithRoots for standalone pre/post per bundle.
type RootBasedStorage interface {
	// CommitAsCanonical sets the given root as the new canonical state.
	CommitAsCanonical(root common.Hash) error

	// DiscardTree removes a tree from storage by root.
	DiscardTree(root common.Hash) bool
}

// SetOnAccumulatedWithRoots sets up callbacks that commit state by root when bundles accumulate.
// Uses BlockCommitmentData (Phase 1) as authoritative PostRoot, falls back to QueueItem.PostRoot.
func (r *Runner) SetOnAccumulatedWithRoots(storage RootBasedStorage, userCallback func(wpHash common.Hash, blockNumber uint64, txHashes []common.Hash)) {
	r.queue.SetOnAccumulated(func(wpHash common.Hash, blockNumber uint64, txHashes []common.Hash) {
		item := r.queue.GetItemByBlockNumber(blockNumber)

		var postRoot common.Hash

		// Try BlockCommitmentData via item.BlockCommitment
		if item != nil && item.BlockCommitment != (common.Hash{}) {
			if commitmentData := r.queue.GetBlockCommitmentData(item.BlockCommitment); commitmentData != nil {
				postRoot = commitmentData.PostRoot
				log.Debug(log.Node, "SetOnAccumulatedWithRoots: using BlockCommitmentData",
					"wpHash", wpHash.Hex(),
					"blockNumber", blockNumber,
					"commitment", item.BlockCommitment.Hex(),
					"postRoot", postRoot.Hex())
			}
		}

		// Fallback: search BlockCommitmentData by blockNumber (handles pruned QueueItem)
		if postRoot == (common.Hash{}) {
			if commitmentData := r.queue.GetBlockCommitmentDataByBlockNumber(blockNumber); commitmentData != nil {
				postRoot = commitmentData.PostRoot
				log.Debug(log.Node, "SetOnAccumulatedWithRoots: found BlockCommitmentData by blockNumber",
					"wpHash", wpHash.Hex(),
					"blockNumber", blockNumber,
					"postRoot", postRoot.Hex())
			}
		}

		// Last resort: item.PostRoot
		if postRoot == (common.Hash{}) && item != nil && item.PostRoot != (common.Hash{}) {
			postRoot = item.PostRoot
			log.Warn(log.Node, "SetOnAccumulatedWithRoots: using item.PostRoot fallback",
				"wpHash", wpHash.Hex(),
				"blockNumber", blockNumber,
				"postRoot", postRoot.Hex())
		}

		if postRoot == (common.Hash{}) {
			log.Error(log.Node, "SetOnAccumulatedWithRoots: no PostRoot found, cannot commit",
				"wpHash", wpHash.Hex(),
				"blockNumber", blockNumber,
				"hasItem", item != nil)
		} else {
			// Store accumulated root and advance contiguous pointer
			r.mu.Lock()
			r.accumulatedRoots[blockNumber] = postRoot

			// Advance highestContiguous while next block exists
			for r.accumulatedRoots[r.highestContiguous+1] != (common.Hash{}) {
				r.highestContiguous++
			}
			commitBlockNumber := r.highestContiguous
			commitRoot := r.accumulatedRoots[commitBlockNumber]
			r.mu.Unlock()

			// Commit the highest contiguous block's root as canonical
			if commitRoot != (common.Hash{}) {
				if err := storage.CommitAsCanonical(commitRoot); err != nil {
					log.Error(log.Node, "💧 ACCUMULATE_FAIL",
						"accumulatedBlock", blockNumber,
						"canonicalBlock", commitBlockNumber,
						"postRoot", commitRoot.Hex(),
						"error", err)
				} else {
					log.Info(log.Node, "💧 ACCUMULATE",
						"accumulatedBlock", blockNumber,
						"canonicalBlock", commitBlockNumber,
						"highestContiguous", commitBlockNumber,
						"postRoot", commitRoot.Hex(),
						"txCount", len(txHashes))
				}
			}
		}

		// Invoke user callback (e.g., remove txs from txpool)
		if userCallback != nil {
			userCallback(wpHash, blockNumber, txHashes)
		}
	})
}

// SetOnFailedWithRoots sets up callbacks that discard trees by root when bundles fail.
// This is the root-first version for standalone pre/post per bundle.
// Uses the PostRoot stored in QueueItem to discard the failed bundle's state.
func (r *Runner) SetOnFailedWithRoots(storage RootBasedStorage, userCallback func(wpHash common.Hash, blockNumber uint64, txHashes []common.Hash)) {
	r.queue.SetOnFailed(func(wpHash common.Hash, blockNumber uint64, txHashes []common.Hash) {
		// Look up the QueueItem to get its PostRoot
		item := r.queue.GetItemByWPHash(wpHash)
		if item != nil && item.PostRoot != (common.Hash{}) && item.PostRoot != item.PreRoot {
			// Root-first path: discard the PostRoot tree (failed bundle's state)
			discarded := storage.DiscardTree(item.PostRoot)
			if discarded {
				log.Info(log.Node, "Discarded PostRoot tree for failed bundle",
					"wpHash", wpHash.Hex(),
					"blockNumber", blockNumber,
					"preRoot", item.PreRoot.Hex(),
					"postRoot", item.PostRoot.Hex(),
					"txCount", len(txHashes))
			}
		} else {
			log.Warn(log.Node, "SetOnFailedWithRoots: missing PostRoot, nothing to discard",
				"wpHash", wpHash.Hex(),
				"blockNumber", blockNumber)
		}

		// Invoke user callback (e.g., unlock txs in txpool)
		if userCallback != nil {
			userCallback(wpHash, blockNumber, txHashes)
		}
	})
}

// SetOnFailed sets a callback to be invoked when bundles fail permanently (max retries exceeded)
// This callback receives the wpHash, block number, and transaction hashes, allowing the caller to:
// - Discard pending UBT writes keyed by wpHash
// - Unlock transactions in the txpool so they can be re-included in future bundles
// Thread-safe: can be called before or after runner starts
func (r *Runner) SetOnFailed(callback func(wpHash common.Hash, blockNumber uint64, txHashes []common.Hash)) {
	r.queue.SetOnFailed(callback)
}

// IsRunning returns whether the runner is active
func (r *Runner) IsRunning() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.running
}

// runLoop is the main processing loop
func (r *Runner) runLoop(ctx context.Context) {
	ticker := time.NewTicker(r.tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.Stop()
			return
		case <-r.stopCh:
			return
		case <-ticker.C:
			r.tick()
		}
	}
}

// tick processes one iteration of the queue
func (r *Runner) tick() {
	// Check for timeouts first
	r.queue.CheckTimeouts()

	// Check if we're in the submission window
	if !r.inSubmissionWindow() {
		// Log queue status periodically when not in window
		stats := r.queue.GetStats()
		if stats.QueuedCount > 0 || stats.InflightCount > 0 {
			now := time.Now()
			timeslotDuration := 6 * time.Second
			epochStart := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
			elapsed := now.Sub(epochStart)
			positionInSlot := elapsed % timeslotDuration
			secondsUntilNext := timeslotDuration - positionInSlot

			log.Debug(log.Node, "Queue Runner: Waiting for submission window",
				"queued", stats.QueuedCount,
				"inflight", stats.InflightCount,
				"submitted", stats.SubmittedCount,
				"guaranteed", stats.GuaranteedCount,
				"secondsUntilNextSlot", secondsUntilNext.Seconds(),
				"windowStart", r.submissionWindowStart,
				"windowEnd", r.submissionWindowEnd)
		}
		return
	}

	// Log that we entered the submission window with detailed status
	stats := r.queue.GetStats()
	log.Info(log.Node, "Queue Runner: In submission window",
		"queued", stats.QueuedCount,
		"inflightMapSize", stats.InflightCount,
		"submitted", stats.SubmittedCount,
		"guaranteed", stats.GuaranteedCount,
		"accumulated", stats.AccumulatedCount,
		"finalized", stats.FinalizedCount,
		"canSubmit", r.queue.CanSubmit())

	// Try to submit items while we can
	for r.queue.CanSubmit() {
		item := r.queue.Dequeue()
		if item == nil {
			break
		}

		// Check if bundle needs rebuild:
		// 1. Resubmission (version > 1) always needs rebuild for fresh RefineContext
		// 2. First submission with stale anchor needs rebuild
		needsRebuild := item.Version > 1
		if !needsRebuild && r.bundleBuilder != nil && item.Bundle != nil {
			// Check if anchor is getting stale
			// G15 check: anchor hash must exist in RecentBlocks.B_H (last 8 blocks = 48 seconds)
			// We use a safety margin of 2 blocks (~12 seconds) to account for network delays
			const anchorSafetyMargin uint32 = 2
			const recentHistorySize uint32 = 8 // types.RecentHistorySize
			anchorSlot := item.Bundle.WorkPackage.RefineContext.LookupAnchorSlot
			currentSlot := common.ComputeTimeSlot("JAM")
			if currentSlot > anchorSlot {
				anchorAge := currentSlot - anchorSlot
				if anchorAge > (recentHistorySize - anchorSafetyMargin) {
					log.Info(log.Node, "Queue Runner: Bundle anchor getting stale, will rebuild",
						"service", r.serviceID,
						"blockNumber", item.BlockNumber,
						"anchorSlot", anchorSlot,
						"currentSlot", currentSlot,
						"anchorAge", anchorAge,
						"threshold", recentHistorySize-anchorSafetyMargin)
					needsRebuild = true
				}
			}
		}

		if needsRebuild && r.bundleBuilder != nil {
			stats := r.queue.GetStats()
			newBundle, err := r.bundleBuilder(item, stats)
			if err != nil {
				log.Error(log.Node, "Queue Runner: Failed to rebuild bundle",
					"service", r.serviceID,
					"blockNumber", item.BlockNumber,
					"version", item.Version,
					"error", err)
				// Put back in queue for retry
				r.queue.mu.Lock()
				r.queue.Queued[item.BlockNumber] = item
				r.queue.mu.Unlock()
				continue
			}
			item.Bundle = newBundle
		}

		// Submit the bundle to the target core
		wpHash, err := r.submitter(item.Bundle, item.CoreIndex)
		if err != nil {
			log.Error(log.Node, "Queue Runner: Submission failed",
				"service", r.serviceID,
				"blockNumber", item.BlockNumber,
				"version", item.Version,
				"error", err)
			// Put back in queue for retry
			r.queue.mu.Lock()
			r.queue.Queued[item.BlockNumber] = item
			r.queue.mu.Unlock()
			continue
		}

		// Mark as submitted
		r.queue.MarkSubmitted(item, wpHash)

		// Get work item count for logging
		workItemCount := 0
		extrinsicCount := 0
		if item.Bundle != nil {
			workItemCount = len(item.Bundle.WorkPackage.WorkItems)
			for _, wi := range item.Bundle.WorkPackage.WorkItems {
				extrinsicCount += len(wi.Extrinsics)
			}
		}

		log.Info(log.Node, "🚀 Work package SUBMITTED to network",
			"wpHash", wpHash.Hex(),
			"service", r.serviceID,
			"blockNumber", item.BlockNumber,
			"version", item.Version,
			"workItems", workItemCount,
			"extrinsics", extrinsicCount,
			"status", "awaiting guarantee (E_G)")
	}
}

// inSubmissionWindow checks if we're within the submission timing window
// Returns true if we're 2-3 seconds before the next JAM timeslot
func (r *Runner) inSubmissionWindow() bool {
	// Get current time within JAM timeslot (6 seconds)
	now := time.Now()
	timeslotDuration := 6 * time.Second

	// Calculate position within current timeslot
	epochStart := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	elapsed := now.Sub(epochStart)
	positionInSlot := elapsed % timeslotDuration
	secondsUntilNext := timeslotDuration - positionInSlot

	// Check if we're in the submission window
	// Window is [submissionWindowEnd, submissionWindowStart] seconds before next slot
	windowStart := time.Duration(r.submissionWindowStart) * time.Second
	windowEnd := time.Duration(r.submissionWindowEnd) * time.Second

	return secondsUntilNext <= windowStart && secondsUntilNext >= windowEnd
}

// GetQueue returns the underlying queue state
func (r *Runner) GetQueue() *QueueState {
	return r.queue
}

// PeekNextBlockNumber returns what the next block number will be without incrementing.
// Use this to set up per-bundle roots BEFORE calling BuildBundle.
func (r *Runner) PeekNextBlockNumber() uint64 {
	return r.queue.PeekNextBlockNumber()
}

// ReserveNextBlockNumber increments and returns the next block number.
// Use this during Phase 1 batch building to reserve block numbers before Phase 2 enqueue.
func (r *Runner) ReserveNextBlockNumber() uint64 {
	return r.queue.ReserveNextBlockNumber()
}

// GetStats returns the current queue statistics
func (r *Runner) GetStats() QueueStats {
	return r.queue.GetStats()
}

// EnqueueBundle is a convenience method to enqueue a bundle for a specific core
func (r *Runner) EnqueueBundle(bundle *types.WorkPackageBundle, coreIndex uint16) (uint64, error) {
	return r.queue.Enqueue(bundle, coreIndex)
}

// EnqueueBundleWithOriginalExtrinsics enqueues a bundle with original transaction extrinsics and metadata
// The originalExtrinsics are needed for rebuilding on resubmission (to avoid double-prepending UBT witness)
// The originalWorkItemExtrinsics are needed to restore WorkItems[].Extrinsics metadata before rebuilding
func (r *Runner) EnqueueBundleWithOriginalExtrinsics(bundle *types.WorkPackageBundle, originalExtrinsics []types.ExtrinsicsBlobs, originalWorkItemExtrinsics [][]types.WorkItemExtrinsic, coreIndex uint16, txHashes []common.Hash) (uint64, error) {
	return r.queue.EnqueueWithOriginalExtrinsics(bundle, originalExtrinsics, originalWorkItemExtrinsics, coreIndex, txHashes)
}

// EnqueueBundleWithRoots enqueues a bundle with explicit pre/post state roots.
// This is the root-first version for standalone pre/post per bundle.
// preRoot: state root before bundle execution (canonical or parent's postRoot)
// postRoot: state root after bundle execution (computed during BuildBundle)
func (r *Runner) EnqueueBundleWithRoots(bundle *types.WorkPackageBundle, originalExtrinsics []types.ExtrinsicsBlobs, originalWorkItemExtrinsics [][]types.WorkItemExtrinsic, coreIndex uint16, txHashes []common.Hash, preRoot, postRoot common.Hash) (uint64, error) {
	return r.queue.EnqueueWithRoots(bundle, originalExtrinsics, originalWorkItemExtrinsics, coreIndex, txHashes, preRoot, postRoot)
}

// EnqueueBundleWithReservedBlockNumber enqueues a bundle using a pre-reserved block number.
func (r *Runner) EnqueueBundleWithReservedBlockNumber(bundle *types.WorkPackageBundle, originalExtrinsics []types.ExtrinsicsBlobs, originalWorkItemExtrinsics [][]types.WorkItemExtrinsic, coreIndex uint16, txHashes []common.Hash, blockNumber uint64, preRoot, postRoot, blockCommitment common.Hash) error {
	return r.queue.EnqueueWithReservedBlockNumber(bundle, originalExtrinsics, originalWorkItemExtrinsics, coreIndex, txHashes, blockNumber, preRoot, postRoot, blockCommitment)
}

// StoreBlockCommitmentData stores the authoritative state roots for a block commitment.
func (r *Runner) StoreBlockCommitmentData(commitment common.Hash, info *BlockCommitmentInfo) {
	r.queue.StoreBlockCommitmentData(commitment, info)
}

// HandleGuaranteed processes a guarantee event
func (r *Runner) HandleGuaranteed(wpHash common.Hash) {
	r.queue.OnGuaranteed(wpHash)
}

// HandleAccumulated processes an accumulation event
func (r *Runner) HandleAccumulated(wpHash common.Hash) {
	r.queue.OnAccumulated(wpHash)
}

// HandleFinalized processes a finalization event
func (r *Runner) HandleFinalized(wpHash common.Hash) {
	r.queue.OnFinalized(wpHash)
}

// GetPendingHeadRoot returns the current pending head root.
// This is the postRoot of the most recent Phase 1 block, used to chain
// Phase 1 blocks across multiple batches even before bundles accumulate.
// Returns zero hash if no pending head is set.
func (r *Runner) GetPendingHeadRoot() common.Hash {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.pendingHeadRoot
}

// SetPendingHeadRoot updates the pending head root to a new value.
// Call this after each successful Phase 1 execution with the block's postRoot.
func (r *Runner) SetPendingHeadRoot(root common.Hash) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pendingHeadRoot = root
}

// GetChainHeadRoot returns the best root to use for chaining Phase 1 blocks.
// Returns pendingHeadRoot if set, otherwise returns the canonical root from storage.
// This ensures Phase 1 blocks chain correctly even when bundles haven't accumulated yet.
func (r *Runner) GetChainHeadRoot(canonicalRoot common.Hash) common.Hash {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.pendingHeadRoot != (common.Hash{}) {
		return r.pendingHeadRoot
	}
	return canonicalRoot
}

// ResetPendingHeadToCanonical resets the pending head to match the canonical root.
// Call this after accumulation catches up to ensure the chain doesn't diverge.
func (r *Runner) ResetPendingHeadToCanonical(canonicalRoot common.Hash) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pendingHeadRoot = canonicalRoot
}
