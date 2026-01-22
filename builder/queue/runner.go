package queue

import (
	"context"
	"sync"
	"time"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
)

type Submitter func(bundle *types.WorkPackageBundle, coreIndex uint16) (common.Hash, error)
type BundleBuilder func(item *QueueItem, stats QueueStats) (*types.WorkPackageBundle, error)

type Runner struct {
	mu sync.RWMutex

	queue     *QueueState
	serviceID uint32

	submitter             Submitter
	bundleBuilder         BundleBuilder
	tickInterval          time.Duration
	stopCh                chan struct{}
	running               bool
	submissionWindowStart int
	submissionWindowEnd   int
	accumulatedRoots  map[uint64]common.Hash // Track roots for out-of-order accumulation
	highestContiguous uint64                 // Highest block N where 1..N are all accumulated
	pendingHeadRoot   common.Hash            // Chains pending blocks before accumulation catches up
}

type RunnerConfig struct {
	TickInterval          time.Duration
	SubmissionWindowStart int
	SubmissionWindowEnd   int
}

func DefaultRunnerConfig() RunnerConfig {
	return RunnerConfig{
		TickInterval:          time.Second,
		SubmissionWindowStart: 3,
		SubmissionWindowEnd:   1,
	}
}

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

func (r *Runner) SetOnAccumulated(callback func(wpHash common.Hash, blockNumber uint64, txHashes []common.Hash)) {
	r.queue.SetOnAccumulated(callback)
}

type RootBasedStorage interface {
	CommitAsCanonical(root common.Hash) error
	DiscardTree(root common.Hash) bool
}

// SetOnAccumulatedWithRoots handles out-of-order accumulation by tracking roots and only
// committing as canonical when blocks 1..N are contiguously accumulated.
func (r *Runner) SetOnAccumulatedWithRoots(storage RootBasedStorage, userCallback func(wpHash common.Hash, blockNumber uint64, txHashes []common.Hash)) {
	r.queue.SetOnAccumulated(func(wpHash common.Hash, blockNumber uint64, txHashes []common.Hash) {
		item := r.queue.GetItemByBlockNumber(blockNumber)

		var postRoot common.Hash
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
		if postRoot == (common.Hash{}) {
			if commitmentData := r.queue.GetBlockCommitmentDataByBlockNumber(blockNumber); commitmentData != nil {
				postRoot = commitmentData.PostRoot
				log.Debug(log.Node, "SetOnAccumulatedWithRoots: found BlockCommitmentData by blockNumber",
					"wpHash", wpHash.Hex(),
					"blockNumber", blockNumber,
					"postRoot", postRoot.Hex())
			}
		}
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
			r.mu.Lock()
			r.accumulatedRoots[blockNumber] = postRoot
			for r.accumulatedRoots[r.highestContiguous+1] != (common.Hash{}) {
				r.highestContiguous++
			}
			commitBlockNumber := r.highestContiguous
			commitRoot := r.accumulatedRoots[commitBlockNumber]
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
			r.mu.Unlock()
		}
		if userCallback != nil {
			userCallback(wpHash, blockNumber, txHashes)
		}
	})
}

func (r *Runner) SetOnFailedWithRoots(storage RootBasedStorage, userCallback func(wpHash common.Hash, blockNumber uint64, txHashes []common.Hash)) {
	r.queue.SetOnFailed(func(wpHash common.Hash, blockNumber uint64, txHashes []common.Hash) {
		item := r.queue.GetItemByWPHash(wpHash)
		if item != nil && item.PostRoot != (common.Hash{}) && item.PostRoot != item.PreRoot {
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
		if userCallback != nil {
			userCallback(wpHash, blockNumber, txHashes)
		}
	})
}

func (r *Runner) SetOnFailed(callback func(wpHash common.Hash, blockNumber uint64, txHashes []common.Hash)) {
	r.queue.SetOnFailed(callback)
}

func (r *Runner) IsRunning() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.running
}

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

func (r *Runner) tick() {
	r.queue.CheckTimeouts()
	if !r.inSubmissionWindow() {
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

	stats := r.queue.GetStats()
	log.Info(log.Node, "Queue Runner: In submission window",
		"queued", stats.QueuedCount,
		"inflightMapSize", stats.InflightCount,
		"submitted", stats.SubmittedCount,
		"guaranteed", stats.GuaranteedCount,
		"accumulated", stats.AccumulatedCount,
		"finalized", stats.FinalizedCount,
		"canSubmit", r.queue.CanSubmit())

	for r.queue.CanSubmit() {
		item := r.queue.Dequeue()
		if item == nil {
			break
		}

		needsRebuild := item.Version > 1
		if !needsRebuild && r.bundleBuilder != nil && item.Bundle != nil {
			const anchorSafetyMargin uint32 = 2
			const recentHistorySize uint32 = 8
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
				r.queue.mu.Lock()
				r.queue.Queued[item.BlockNumber] = item
				r.queue.mu.Unlock()
				continue
			}
			item.Bundle = newBundle
		}
		wpHash, err := r.submitter(item.Bundle, item.CoreIndex)
		if err != nil {
			log.Error(log.Node, "Queue Runner: Submission failed",
				"service", r.serviceID,
				"blockNumber", item.BlockNumber,
				"version", item.Version,
				"error", err)
			r.queue.mu.Lock()
			r.queue.Queued[item.BlockNumber] = item
			r.queue.mu.Unlock()
			continue
		}
		r.queue.MarkSubmitted(item, wpHash)
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

// inSubmissionWindow returns true when we're 1-3 seconds before the next JAM timeslot.
// JAM uses 6-second timeslots; submitting late in a slot gives validators time to process.
func (r *Runner) inSubmissionWindow() bool {
	now := time.Now()
	timeslotDuration := 6 * time.Second
	epochStart := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	elapsed := now.Sub(epochStart)
	positionInSlot := elapsed % timeslotDuration
	secondsUntilNext := timeslotDuration - positionInSlot
	windowStart := time.Duration(r.submissionWindowStart) * time.Second
	windowEnd := time.Duration(r.submissionWindowEnd) * time.Second

	return secondsUntilNext <= windowStart && secondsUntilNext >= windowEnd
}

func (r *Runner) GetQueue() *QueueState {
	return r.queue
}

func (r *Runner) PeekNextBlockNumber() uint64 {
	return r.queue.PeekNextBlockNumber()
}

func (r *Runner) ReserveNextBlockNumber() uint64 {
	return r.queue.ReserveNextBlockNumber()
}

func (r *Runner) GetStats() QueueStats {
	return r.queue.GetStats()
}

func (r *Runner) EnqueueBundle(bundle *types.WorkPackageBundle, coreIndex uint16) (uint64, error) {
	return r.queue.Enqueue(bundle, coreIndex)
}

func (r *Runner) EnqueueBundleWithOriginalExtrinsics(bundle *types.WorkPackageBundle, originalExtrinsics []types.ExtrinsicsBlobs, originalWorkItemExtrinsics [][]types.WorkItemExtrinsic, coreIndex uint16, txHashes []common.Hash) (uint64, error) {
	return r.queue.EnqueueWithOriginalExtrinsics(bundle, originalExtrinsics, originalWorkItemExtrinsics, coreIndex, txHashes)
}

func (r *Runner) EnqueueBundleWithRoots(bundle *types.WorkPackageBundle, originalExtrinsics []types.ExtrinsicsBlobs, originalWorkItemExtrinsics [][]types.WorkItemExtrinsic, coreIndex uint16, txHashes []common.Hash, preRoot, postRoot common.Hash) (uint64, error) {
	return r.queue.EnqueueWithRoots(bundle, originalExtrinsics, originalWorkItemExtrinsics, coreIndex, txHashes, preRoot, postRoot)
}

func (r *Runner) EnqueueBundleWithReservedBlockNumber(bundle *types.WorkPackageBundle, originalExtrinsics []types.ExtrinsicsBlobs, originalWorkItemExtrinsics [][]types.WorkItemExtrinsic, coreIndex uint16, txHashes []common.Hash, blockNumber uint64, preRoot, postRoot, blockCommitment common.Hash) error {
	return r.queue.EnqueueWithReservedBlockNumber(bundle, originalExtrinsics, originalWorkItemExtrinsics, coreIndex, txHashes, blockNumber, preRoot, postRoot, blockCommitment)
}

func (r *Runner) StoreBlockCommitmentData(commitment common.Hash, info *BlockCommitmentInfo) {
	r.queue.StoreBlockCommitmentData(commitment, info)
}

func (r *Runner) HandleGuaranteed(wpHash common.Hash) {
	r.queue.OnGuaranteed(wpHash)
}

func (r *Runner) HandleAccumulated(wpHash common.Hash) {
	r.queue.OnAccumulated(wpHash)
}

func (r *Runner) HandleFinalized(wpHash common.Hash) {
	r.queue.OnFinalized(wpHash)
}

func (r *Runner) GetPendingHeadRoot() common.Hash {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.pendingHeadRoot
}

func (r *Runner) SetPendingHeadRoot(root common.Hash) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pendingHeadRoot = root
}

// GetChainHeadRoot returns pendingHeadRoot if set, otherwise canonical root.
func (r *Runner) GetChainHeadRoot(canonicalRoot common.Hash) common.Hash {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.pendingHeadRoot != (common.Hash{}) {
		return r.pendingHeadRoot
	}
	return canonicalRoot
}

func (r *Runner) ResetPendingHeadToCanonical(canonicalRoot common.Hash) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pendingHeadRoot = canonicalRoot
}
