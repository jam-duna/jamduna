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
	DefaultMaxQueueDepth = 12  // Maximum items waiting to be submitted
	DefaultMaxInflight   = 3   // Maximum items in Submitted state (cores blocked)

	// Fast retry configuration (same version, network failure recovery)
	// Uses RecentHistorySize window (48s) for safe retries within anchor validity
	DefaultSubmitRetryInterval = 6 * time.Second // Retry same version every 1 JAM block
	DefaultMaxSubmitRetries    = 8               // Max submission attempts (1 initial + 7 retries = 8 total attempts over ~48s)

	// CRITICAL: GuaranteeTimeout must exceed the refine validity window to prevent on-chain duplicates
	// When v1 times out and v2 is submitted, v1 must be expired on-chain before v2 can be guaranteed.
	// Otherwise, both v1 and v2 could be guaranteed by validators, causing duplicate execution.
	//
	// THREE validation constraints exist for guarantee age (all checked in VerifyGuarantee):
	// 1. checkAssignment (statedb/guarantees.go:183): diff <= (currentSlot % RotationPeriod) + RotationPeriod
	//    - RotationPeriod = 4, so max age varies: 4-7 slots (24-42s) depending on rotation position
	//    - Returns ErrGReportEpochBeforeLast if violated
	// 2. checkRecentBlock (statedb/guarantees.go:388): anchor/state/beefy in RecentBlocks (size=8)
	//    - Max age: RecentHistorySize × SecondsPerSlot = 8 × 6s = 48 seconds
	//    - Returns ErrGAnchorNotRecent if violated
	// 3. checkTimeSlotHeader (statedb/guarantees.go:429): LookupAnchorSlot >= currentSlot - 24
	//    - Max age: LookupAnchorMaxAge × SecondsPerSlot = 24 × 6s = 144 seconds
	//    - Returns ErrGReportEpochBeforeLast if violated
	//
	// The BINDING constraint is checkAssignment's rotation period check (runs first):
	// - Minimum validity: 4 slots = 24 seconds (when currentSlot % 4 = 0)
	// - Maximum validity: 7 slots = 42 seconds (when currentSlot % 4 = 3)
	//
	// To ensure v1 expired before v2 submitted IN ALL CASES, use maximum validity:
	// - Maximum validity window: 7 × 6 = 42 seconds
	// - Safety margin: 2 slots = 12 seconds (for network delays)
	//
	// Total timeout = 42s + 12s = 54s (9 JAM blocks)
	// This ensures v1's guarantee slot is too old (fails checkAssignment) before we resubmit as v2.
	DefaultGuaranteeTimeout = 54 * time.Second

	DefaultAccumulateTimeout = 60 * time.Second // Time before Guaranteed item without accumulation is considered failed (10 JAM blocks)
	DefaultRetentionWindow   = 100              // Number of finalized blocks to retain
	DefaultMaxVersionRetries = 5                // Maximum resubmission attempts before dropping
)

// QueueConfig holds configuration for the work package queue
type QueueConfig struct {
	MaxQueueDepth       int
	MaxInflight         int
	SubmitRetryInterval time.Duration // Fast retry: resubmit same version every N seconds
	MaxSubmitRetries    int           // Fast retry: max attempts before slow timeout
	GuaranteeTimeout    time.Duration // Slow timeout: increment version after refine expiry
	AccumulateTimeout   time.Duration
	RetentionWindow     int
	MaxVersionRetries   int
}

// DefaultConfig returns the default queue configuration
func DefaultConfig() QueueConfig {
	return QueueConfig{
		MaxQueueDepth:       DefaultMaxQueueDepth,
		MaxInflight:         DefaultMaxInflight,
		SubmitRetryInterval: DefaultSubmitRetryInterval,
		MaxSubmitRetries:    DefaultMaxSubmitRetries,
		GuaranteeTimeout:    DefaultGuaranteeTimeout,
		AccumulateTimeout:   DefaultAccumulateTimeout,
		RetentionWindow:     DefaultRetentionWindow,
		MaxVersionRetries:   DefaultMaxVersionRetries,
	}
}

// QueueItem represents a work package in the queue with version tracking
type QueueItem struct {
	BundleID    string                   // Persistent identifier that survives resubmissions (format: "B-{blockNumber}")
	BlockNumber uint64                   // Rollup block number (may be provisional)
	Version     int                      // Version for resubmission tracking
	EventID     uint64                   // Telemetry event ID
	AddTS       time.Time                // When item was added to queue
	Bundle      *types.WorkPackageBundle // The actual bundle to submit
	WPHash      common.Hash              // Work package hash (computed after BuildBundle) - CHANGES on resubmit!
	SubmittedAt  time.Time               // When item was submitted (zero if not submitted)
	GuaranteedAt time.Time               // When item was guaranteed (zero if not guaranteed)
	Status      WorkPackageBundleStatus  // Current status
	CoreIndex   uint16                   // Target core index for submission

	// OriginalExtrinsics stores the transaction extrinsics BEFORE UBT witness prepending.
	// Used for rebuilding bundles on resubmission - BuildBundle prepends a fresh witness,
	// so we need the original txs to avoid double-prepending.
	OriginalExtrinsics []types.ExtrinsicsBlobs

	// OriginalWorkItemExtrinsics stores the WorkItems[].Extrinsics metadata BEFORE UBT witness prepending.
	// BuildBundle also prepends metadata entries, so we need to restore the original metadata on rebuild.
	OriginalWorkItemExtrinsics [][]types.WorkItemExtrinsic

	// TransactionHashes stores the transaction hashes included in this bundle
	// Used to remove transactions from txpool only after accumulation (not after enqueue)
	TransactionHashes []common.Hash

	// Fast retry tracking: resubmit same version on network failures
	SubmitAttempts int       // Number of times this version was submitted
	LastSubmitAt   time.Time // Last submission attempt timestamp
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

	// Reverse lookup: wpHash -> blockNumber (for guarantee/accumulate events)
	// This is updated every time a bundle is submitted with a new hash
	HashToBlock map[common.Hash]uint64

	// WinningVer tracks which version was first guaranteed (immutable once set)
	// This enforces "first guarantee wins" to prevent duplicate execution
	WinningVer map[uint64]int

	// Finalized items (for verification and retention)
	Finalized map[uint64]*QueueItem

	// Next block number to assign
	nextBlockNumber uint64

	// Event ID generator
	eventIDCounter uint64

	// Operational metrics (for observability)
	duplicateGuaranteeRejected  int // Counter for rejected duplicate guarantees
	duplicateAccumulateRejected int // Counter for rejected duplicate accumulations
	nonWinningVersionsCanceled  int // Counter for canceled non-winning versions

	// Callbacks for queue events
	onStatusChange   func(blockNumber uint64, oldStatus, newStatus WorkPackageBundleStatus)
	onAccumulated    func(wpHash common.Hash, txHashes []common.Hash) // Called when bundle accumulates
	onFailed         func(blockNumber uint64, txHashes []common.Hash) // Called when bundle fails permanently (max retries exceeded)
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
		HashToBlock:     make(map[common.Hash]uint64),
		WinningVer:      make(map[uint64]int),
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

// SetOnAccumulated sets the callback for accumulation events (thread-safe)
func (qs *QueueState) SetOnAccumulated(cb func(wpHash common.Hash, txHashes []common.Hash)) {
	qs.mu.Lock()
	defer qs.mu.Unlock()
	qs.onAccumulated = cb
}

// SetOnFailed sets the callback for failure events (thread-safe)
// Called when a bundle fails permanently (max retries exceeded)
func (qs *QueueState) SetOnFailed(cb func(blockNumber uint64, txHashes []common.Hash)) {
	qs.mu.Lock()
	defer qs.mu.Unlock()
	qs.onFailed = cb
}

// nextEventID generates a unique event ID
func (qs *QueueState) nextEventID() uint64 {
	qs.eventIDCounter++
	return qs.eventIDCounter
}

// Enqueue adds a new bundle to the queue for a specific target core
// Returns the assigned block number and error if queue is full
func (qs *QueueState) Enqueue(bundle *types.WorkPackageBundle, coreIndex uint16) (uint64, error) {
	qs.mu.Lock()
	defer qs.mu.Unlock()

	if len(qs.Queued) >= qs.config.MaxQueueDepth {
		return 0, fmt.Errorf("queue full: %d items queued (max %d)", len(qs.Queued), qs.config.MaxQueueDepth)
	}

	blockNumber := qs.nextBlockNumber
	qs.nextBlockNumber++

	bundleID := fmt.Sprintf("B-%d", blockNumber)
	wpHash := bundle.WorkPackage.Hash()

	item := &QueueItem{
		BundleID:    bundleID,
		BlockNumber: blockNumber,
		Version:     1,
		EventID:     qs.nextEventID(),
		AddTS:       time.Now(),
		Bundle:      bundle,
		WPHash:      wpHash,
		Status:      StatusQueued,
		CoreIndex:   coreIndex,
	}

	qs.Queued[blockNumber] = item
	qs.Status[blockNumber] = StatusQueued
	qs.CurrentVer[blockNumber] = 1

	if qs.HashByVer[blockNumber] == nil {
		qs.HashByVer[blockNumber] = make(map[int]common.Hash)
	}
	qs.HashByVer[blockNumber][1] = wpHash
	qs.HashToBlock[wpHash] = blockNumber // Reverse lookup

	log.Info(log.Node, "Queue: Enqueued work package",
		"service", qs.serviceID,
		"bundleID", bundleID,
		"blockNumber", blockNumber,
		"coreIndex", coreIndex,
		"wpHash", wpHash.Hex(),
		"queueSize", len(qs.Queued))

	return blockNumber, nil
}

// EnqueueWithOriginalExtrinsics adds a new bundle to the queue along with original extrinsics
// The originalExtrinsics are the transaction extrinsics BEFORE UBT witness prepending,
// used for rebuilding bundles on resubmission without double-prepending the witness.
// The originalWorkItemExtrinsics are the WorkItems[].Extrinsics metadata BEFORE UBT witness prepending.
func (qs *QueueState) EnqueueWithOriginalExtrinsics(bundle *types.WorkPackageBundle, originalExtrinsics []types.ExtrinsicsBlobs, originalWorkItemExtrinsics [][]types.WorkItemExtrinsic, coreIndex uint16, txHashes []common.Hash) (uint64, error) {
	qs.mu.Lock()
	defer qs.mu.Unlock()

	if len(qs.Queued) >= qs.config.MaxQueueDepth {
		return 0, fmt.Errorf("queue full: %d items queued (max %d)", len(qs.Queued), qs.config.MaxQueueDepth)
	}

	blockNumber := qs.nextBlockNumber
	qs.nextBlockNumber++

	bundleID := fmt.Sprintf("B-%d", blockNumber)
	wpHash := bundle.WorkPackage.Hash()

	item := &QueueItem{
		BundleID:                   bundleID,
		BlockNumber:                blockNumber,
		Version:                    1,
		EventID:                    qs.nextEventID(),
		AddTS:                      time.Now(),
		Bundle:                     bundle,
		WPHash:                     wpHash,
		Status:                     StatusQueued,
		CoreIndex:                  coreIndex,
		OriginalExtrinsics:         originalExtrinsics,
		OriginalWorkItemExtrinsics: originalWorkItemExtrinsics,
		TransactionHashes:          txHashes,
	}

	qs.Queued[blockNumber] = item
	qs.Status[blockNumber] = StatusQueued
	qs.CurrentVer[blockNumber] = 1

	if qs.HashByVer[blockNumber] == nil {
		qs.HashByVer[blockNumber] = make(map[int]common.Hash)
	}
	qs.HashByVer[blockNumber][1] = wpHash
	qs.HashToBlock[wpHash] = blockNumber // Reverse lookup

	// Count original extrinsics (transactions before UBT witness prepending)
	txCount := 0
	for _, blobs := range originalExtrinsics {
		txCount += len(blobs)
	}
	log.Info(log.Node, "Queue: Enqueued work package with original extrinsics",
		"service", qs.serviceID,
		"bundleID", bundleID,
		"blockNumber", blockNumber,
		"coreIndex", coreIndex,
		"wpHash", wpHash.Hex(),
		"txCount", txCount,
		"queueSize", len(qs.Queued))

	return blockNumber, nil
}

// inflight returns the count of items blocking core slots (only Submitted state)
// Once guaranteed, the core is free for new submissions - we just track guaranteed
// items until they accumulate for cleanup purposes.
func (qs *QueueState) inflight() int {
	count := 0
	for _, status := range qs.Status {
		if status == StatusSubmitted {
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
	oldHash := item.WPHash
	item.Status = StatusSubmitted
	now := time.Now()
	if item.SubmittedAt.IsZero() {
		item.SubmittedAt = now
	}
	item.LastSubmitAt = now
	item.SubmitAttempts++
	item.WPHash = wpHash

	qs.Inflight[item.BlockNumber] = item
	qs.Status[item.BlockNumber] = StatusSubmitted

	// Initialize HashByVer map for this block number if needed
	if qs.HashByVer[item.BlockNumber] == nil {
		qs.HashByVer[item.BlockNumber] = make(map[int]common.Hash)
	}
	qs.HashByVer[item.BlockNumber][item.Version] = wpHash

	// CRITICAL: Register the NEW hash in reverse lookup for guarantee matching
	// The wpHash may have changed due to RefineContext changes on resubmission
	qs.HashToBlock[wpHash] = item.BlockNumber

	if qs.onStatusChange != nil {
		qs.onStatusChange(item.BlockNumber, oldStatus, StatusSubmitted)
	}

	// Calculate inflight count
	inflightCount := qs.inflight()

	// Log hash change if this is a resubmission with new hash
	hashChanged := oldHash != wpHash && oldHash != (common.Hash{})

	log.Info(log.Node, "Queue: Marked submitted (tracking hash for guarantee)",
		"service", qs.serviceID,
		"bundleID", item.BundleID,
		"blockNumber", item.BlockNumber,
		"version", item.Version,
		"coreIndex", item.CoreIndex,
		"hashChanged", hashChanged,
		"wpHash", wpHash.Hex(),
		"inflightCount", inflightCount,
		"queuedRemaining", len(qs.Queued))
}

// cancelOtherVersions removes non-winning versions from Queued/Inflight and invalidates their hashes.
// This prevents duplicate execution when an older version wins the guarantee race.
//
// IMPORTANT: This only provides LOCAL consistency. It cannot prevent on-chain duplicates
// if multiple versions were already submitted to validators. The GuaranteeTimeout (54s)
// is configured to exceed the rotation period validity window (42s maximum) to ensure old
// versions are invalid on-chain before resubmission, which is the only true guarantee against duplicates.
func (qs *QueueState) cancelOtherVersions(bn uint64, winningVer int) {
	canceledCount := 0

	// If there's a queued item with a different version, remove it
	if item, ok := qs.Queued[bn]; ok && item.Version != winningVer {
		log.Info(log.Node, "Queue: Canceling queued non-winning version",
			"service", qs.serviceID,
			"blockNumber", bn,
			"canceledVersion", item.Version,
			"winningVersion", winningVer,
			"wpHash", item.WPHash.Hex())
		delete(qs.Queued, bn)
		canceledCount++
	}

	// If there's an inflight item with a different version, remove it
	// This handles the case where a non-winning version was submitted but an older version won
	if item, ok := qs.Inflight[bn]; ok && item.Version != winningVer {
		log.Info(log.Node, "Queue: Canceling inflight non-winning version",
			"service", qs.serviceID,
			"blockNumber", bn,
			"canceledVersion", item.Version,
			"winningVersion", winningVer,
			"wpHash", item.WPHash.Hex())
		delete(qs.Inflight, bn)
		// Don't update Status here - the winning version will set it when it's processed
		// Status[bn] may still be StatusSubmitted or already StatusGuaranteed depending on timing
		canceledCount++
	}

	// Invalidate hash mappings for all non-winning versions
	if versions, ok := qs.HashByVer[bn]; ok {
		for ver, hash := range versions {
			if ver != winningVer {
				log.Debug(log.Node, "Queue: Invalidating hash for non-winning version",
					"service", qs.serviceID,
					"blockNumber", bn,
					"version", ver,
					"wpHash", hash.Hex())
				delete(qs.HashToBlock, hash)
			}
		}
	}

	if canceledCount > 0 {
		qs.nonWinningVersionsCanceled += canceledCount
		log.Info(log.Node, "Queue: Completed non-winning version cleanup",
			"service", qs.serviceID,
			"blockNumber", bn,
			"winningVersion", winningVer,
			"canceledCount", canceledCount,
			"totalCanceled", qs.nonWinningVersionsCanceled)
	}
}

// OnGuaranteed handles a guarantee event for a work package hash
func (qs *QueueState) OnGuaranteed(wpHash common.Hash) {
	qs.mu.Lock()
	defer qs.mu.Unlock()

	// Log incoming guarantee with hash details
	log.Info(log.Node, "Queue: OnGuaranteed called",
		"wpHash", wpHash.Hex(),
		"hashToBlockSize", len(qs.HashToBlock),
		"hashByVerSize", len(qs.HashByVer))

	bn, ver := qs.lookupByHash(wpHash)
	if bn == 0 {
		// Log all known hashes to help debug
		var knownHashes []string
		for blockNum, versions := range qs.HashByVer {
			for v, h := range versions {
				knownHashes = append(knownHashes, fmt.Sprintf("bn=%d,v=%d,hash=%s", blockNum, v, h.Hex()[:16]))
			}
		}
		log.Warn(log.Node, "Queue: OnGuaranteed for UNKNOWN hash - NOT FOUND in queue",
			"incomingHash", wpHash.Hex(),
			"knownHashCount", len(knownHashes),
			"knownHashes", knownHashes)
		return
	}

	// CRITICAL: Enforce "first guarantee wins" to prevent duplicate execution
	// Check if a winning version has already been chosen for this block
	if winningVer, exists := qs.WinningVer[bn]; exists {
		if ver != winningVer {
			qs.duplicateGuaranteeRejected++
			log.Warn(log.Node, "❌ Queue: REJECTING guarantee for non-winning version (duplicate execution prevention)",
				"service", qs.serviceID,
				"blockNumber", bn,
				"rejectedVersion", ver,
				"winningVersion", winningVer,
				"wpHash", wpHash.Hex(),
				"totalDuplicatesRejected", qs.duplicateGuaranteeRejected)
			return
		}
		// This is the winning version - proceed normally
		log.Debug(log.Node, "Queue: OnGuaranteed for already-chosen winning version",
			"blockNumber", bn,
			"version", ver)
	} else {
		// First guarantee for this block - this version wins!
		qs.WinningVer[bn] = ver
		qs.CurrentVer[bn] = ver
		log.Info(log.Node, "🏆 Queue: First guarantee - this version WINS (canceling all others)",
			"service", qs.serviceID,
			"blockNumber", bn,
			"winningVersion", ver,
			"wpHash", wpHash.Hex())
		// Cancel all other versions (queued items and hash mappings)
		qs.cancelOtherVersions(bn, ver)
	}

	oldStatus := qs.Status[bn]
	qs.Status[bn] = StatusGuaranteed

	var bundleID string
	// Check both Inflight and Queued - item might be in queue for resubmission
	if item, ok := qs.Inflight[bn]; ok {
		item.Status = StatusGuaranteed
		item.GuaranteedAt = time.Now()
		item.Version = ver // Update to the version that was actually guaranteed
		item.WPHash = wpHash
		bundleID = item.BundleID
	} else if item, ok := qs.Queued[bn]; ok {
		// Item was requeued but older version got guaranteed - move it to Inflight
		item.Status = StatusGuaranteed
		item.GuaranteedAt = time.Now()
		item.Version = ver
		item.WPHash = wpHash
		bundleID = item.BundleID
		qs.Inflight[bn] = item
		delete(qs.Queued, bn)
		log.Info(log.Node, "Queue: Moved item from Queued to Inflight after guarantee",
			"bundleID", bundleID,
			"blockNumber", bn)
	}

	if qs.onStatusChange != nil {
		qs.onStatusChange(bn, oldStatus, StatusGuaranteed)
	}

	// Calculate new inflight count
	inflightCount := qs.inflight()

	log.Info(log.Node, "🔒 Queue: Marked guaranteed",
		"service", qs.serviceID,
		"bundleID", bundleID,
		"blockNumber", bn,
		"version", ver,
		"wpHash", wpHash.Hex(),
		"oldStatus", oldStatus.String(),
		"newInflightCount", inflightCount,
		"queuedCount", len(qs.Queued))
}

// OnAccumulated handles an accumulation event for a work package hash
func (qs *QueueState) OnAccumulated(wpHash common.Hash) {
	qs.mu.Lock()

	bn, ver := qs.lookupByHash(wpHash)
	if bn == 0 {
		qs.mu.Unlock()
		log.Debug(log.Node, "Queue: OnAccumulated for unknown hash", "wpHash", wpHash.Hex())
		return
	}

	// CRITICAL: Only accept accumulation for the winning version
	// This prevents duplicate execution when multiple versions were guaranteed
	if winningVer, exists := qs.WinningVer[bn]; exists {
		if ver != winningVer {
			qs.duplicateAccumulateRejected++
			qs.mu.Unlock()
			log.Warn(log.Node, "❌ Queue: REJECTING accumulation for non-winning version (duplicate execution prevention)",
				"service", qs.serviceID,
				"blockNumber", bn,
				"rejectedVersion", ver,
				"winningVersion", winningVer,
				"wpHash", wpHash.Hex(),
				"totalDuplicatesRejected", qs.duplicateAccumulateRejected)
			return
		}
	} else {
		// Accumulation without prior guarantee - this shouldn't happen but accept it as winner
		log.Warn(log.Node, "Queue: OnAccumulated without prior guarantee - accepting as winner",
			"service", qs.serviceID,
			"blockNumber", bn,
			"version", ver,
			"wpHash", wpHash.Hex())
		qs.WinningVer[bn] = ver
		qs.CurrentVer[bn] = ver
		qs.cancelOtherVersions(bn, ver)
	}

	oldStatus := qs.Status[bn]
	qs.Status[bn] = StatusAccumulated

	var bundleID string
	var txHashes []common.Hash
	var callback func(common.Hash, []common.Hash)
	if item, ok := qs.Inflight[bn]; ok {
		item.Status = StatusAccumulated
		bundleID = item.BundleID
		txHashes = item.TransactionHashes
		// Move to Finalized and remove from Inflight - accumulation completes the lifecycle
		qs.Finalized[bn] = item
		delete(qs.Inflight, bn)
	} else if item, ok := qs.Queued[bn]; ok {
		// Item was requeued but older version got accumulated
		item.Status = StatusAccumulated
		bundleID = item.BundleID
		txHashes = item.TransactionHashes
		// Move directly to Finalized
		qs.Finalized[bn] = item
		delete(qs.Queued, bn)
	}

	if qs.onStatusChange != nil {
		qs.onStatusChange(bn, oldStatus, StatusAccumulated)
	}

	// Capture callback and txHashes to invoke outside the lock
	if qs.onAccumulated != nil && len(txHashes) > 0 {
		callback = qs.onAccumulated
	}

	// Prune old finalized items
	qs.pruneOlder(bn)

	// Calculate new inflight count - accumulation frees up a slot!
	inflightCount := qs.inflight()

	log.Info(log.Node, "📦 Queue: Marked accumulated (slot freed!)",
		"service", qs.serviceID,
		"bundleID", bundleID,
		"blockNumber", bn,
		"version", ver,
		"wpHash", wpHash.Hex(),
		"oldStatus", oldStatus.String(),
		"newInflightCount", inflightCount,
		"inflightMapSize", len(qs.Inflight),
		"queuedCount", len(qs.Queued))

	// Release lock before invoking callback to avoid blocking the queue
	qs.mu.Unlock()

	// Invoke callback outside the lock - it may do I/O (remove from txpool, logging)
	if callback != nil {
		callback(wpHash, txHashes)
	}
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

	// CRITICAL: Only accept finalization for the winning version
	if winningVer, exists := qs.WinningVer[bn]; exists {
		if ver != winningVer {
			log.Warn(log.Node, "❌ Queue: REJECTING finalization for non-winning version",
				"service", qs.serviceID,
				"blockNumber", bn,
				"rejectedVersion", ver,
				"winningVersion", winningVer,
				"wpHash", wpHash.Hex())
			return
		}
	} else {
		// Finalization without prior guarantee - shouldn't happen but accept as winner
		log.Warn(log.Node, "Queue: OnFinalized without prior guarantee - accepting as winner",
			"service", qs.serviceID,
			"blockNumber", bn,
			"version", ver,
			"wpHash", wpHash.Hex())
		qs.WinningVer[bn] = ver
		qs.CurrentVer[bn] = ver
		qs.cancelOtherVersions(bn, ver)
	}

	oldStatus := qs.Status[bn]
	qs.Status[bn] = StatusFinalized

	// Move from inflight or queued to finalized
	if item, ok := qs.Inflight[bn]; ok {
		item.Status = StatusFinalized
		qs.Finalized[bn] = item
		delete(qs.Inflight, bn)
	} else if item, ok := qs.Queued[bn]; ok {
		item.Status = StatusFinalized
		qs.Finalized[bn] = item
		delete(qs.Queued, bn)
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

	now := time.Now()

	// Find all block numbers that need to be requeued (failedBN and all subsequent in-flight)
	var toRequeue []uint64
	for bn := range qs.Inflight {
		if bn >= failedBN {
			toRequeue = append(toRequeue, bn)
		}
	}

	// Track failed items to notify callback after releasing lock
	type failedItem struct {
		blockNumber uint64
		txHashes    []common.Hash
	}
	var failedItems []failedItem

	for _, bn := range toRequeue {
		item := qs.Inflight[bn]
		if item == nil {
			continue
		}

		// CRITICAL: Double-check status before requeuing - another goroutine may have
		// processed a guarantee event between timeout detection and this function call.
		// This prevents duplicate execution of the same transactions.
		currentStatus := qs.Status[bn]
		if currentStatus == StatusGuaranteed || currentStatus == StatusAccumulated || currentStatus == StatusFinalized {
			log.Info(log.Node, "Queue: OnTimeoutOrFailure skipping - bundle already guaranteed/accumulated",
				"service", qs.serviceID,
				"blockNumber", bn,
				"currentStatus", currentStatus.String(),
				"wpHash", item.WPHash.Hex())
			continue
		}

		// Check retry limit
		newVersion := qs.CurrentVer[bn] + 1
		if newVersion > qs.config.MaxVersionRetries {
			log.Warn(log.Node, "Queue: Max retries exceeded, dropping block",
				"service", qs.serviceID,
				"blockNumber", bn,
				"version", newVersion-1,
				"txCount", len(item.TransactionHashes))
			qs.Status[bn] = StatusFailed
			delete(qs.Inflight, bn)
			// Track for callback notification
			if len(item.TransactionHashes) > 0 {
				failedItems = append(failedItems, failedItem{
					blockNumber: bn,
					txHashes:    item.TransactionHashes,
				})
			}
			continue
		}

		// Increment version and requeue
		qs.CurrentVer[bn] = newVersion
		delete(qs.Status, bn) // Clear status (will be set to Queued)

		item.Version = newVersion
		item.AddTS = now
		item.Status = StatusQueued
		item.SubmittedAt = time.Time{}  // Reset submission time
		item.LastSubmitAt = time.Time{} // Reset retry tracking
		item.SubmitAttempts = 0         // Reset retry counter for new version

		qs.Queued[bn] = item
		qs.Status[bn] = StatusQueued
		delete(qs.Inflight, bn)

		log.Info(log.Node, "🔄 Queue: Requeued after failure",
			"service", qs.serviceID,
			"blockNumber", bn,
			"newVersion", newVersion,
			"wpHash", item.WPHash.Hex())
	}

	// Capture callback to invoke outside lock
	callback := qs.onFailed
	qs.mu.Unlock()

	// Invoke callback for each failed item
	if callback != nil {
		for _, failed := range failedItems {
			callback(failed.blockNumber, failed.txHashes)
		}
	}
}

// lookupByHash finds block number and version for a given work package hash
// Uses the reverse lookup map first (O(1)), falls back to scanning HashByVer
func (qs *QueueState) lookupByHash(wpHash common.Hash) (blockNumber uint64, version int) {
	// Fast path: use reverse lookup map
	if bn, ok := qs.HashToBlock[wpHash]; ok {
		// Find the version for this hash
		if versions, ok := qs.HashByVer[bn]; ok {
			for ver, hash := range versions {
				if hash == wpHash {
					return bn, ver
				}
			}
		}
		// Found in reverse map but not in HashByVer - shouldn't happen, but return what we have
		return bn, qs.CurrentVer[bn]
	}

	// Slow path fallback: scan all versions (for backwards compatibility)
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
			// CRITICAL: Remove all hashes for this block number from reverse lookup
			// to prevent late guarantee/accumulation events from resurrecting pruned blocks
			if versions, ok := qs.HashByVer[bn]; ok {
				for _, hash := range versions {
					delete(qs.HashToBlock, hash)
				}
			}

			delete(qs.Finalized, bn)
			delete(qs.Status, bn)
			delete(qs.CurrentVer, bn)
			delete(qs.HashByVer, bn)
			delete(qs.WinningVer, bn)
		}
	}
}

// CheckTimeouts checks for timed out submissions and triggers requeuing
func (qs *QueueState) CheckTimeouts() {
	qs.mu.Lock()
	defer qs.mu.Unlock()

	now := time.Now()

	for bn, item := range qs.Inflight {
		var timeout time.Duration
		var refTime time.Time
		var timeoutType string

		switch item.Status {
		case StatusSubmitted:
			if item.SubmittedAt.IsZero() {
				continue
			}

			// TWO-TIER TIMEOUT STRATEGY:
			// 1. Fast retry (same version): Resubmit same bundle within RecentHistorySize window (48s)
			//    - Safe because same hash = idempotent (no duplicate execution)
			//    - Recovers from transient network failures quickly
			// 2. Slow timeout (new version): After refine expiry (54s), increment version
			//    - Only after v1's refine is expired on-chain, preventing duplicate guarantees

			elapsed := now.Sub(item.SubmittedAt)
			sinceLastSubmit := now.Sub(item.LastSubmitAt)

			// Fast retry window: min(RecentHistorySize × SecondsPerSlot, GuaranteeTimeout)
			recentHistoryWindow := time.Duration(types.RecentHistorySize*types.SecondsPerSlot) * time.Second
			fastRetryWindow := recentHistoryWindow
			if qs.config.GuaranteeTimeout < fastRetryWindow {
				fastRetryWindow = qs.config.GuaranteeTimeout
			}

			// Slow timeout after refine expiry
			timeout = qs.config.GuaranteeTimeout
			refTime = item.SubmittedAt
			timeoutType = "guarantee"

			if elapsed > timeout {
				// CRITICAL: Before incrementing version, check if ANY version was already guaranteed
				currentStatus := qs.Status[bn]
				if currentStatus == StatusGuaranteed || currentStatus == StatusAccumulated || currentStatus == StatusFinalized {
					log.Info(log.Node, "Queue: Timeout detected but bundle already guaranteed/accumulated - skipping requeue",
						"service", qs.serviceID,
						"blockNumber", bn,
						"version", item.Version,
						"currentStatus", currentStatus.String(),
						"wpHash", item.WPHash.Hex())
					continue
				}

				log.Warn(log.Node, "Queue: Refine expired - incrementing version",
					"service", qs.serviceID,
					"blockNumber", bn,
					"version", item.Version,
					"attempts", item.SubmitAttempts,
					"elapsed", elapsed.Round(time.Second),
					"timeout", timeout,
					"wpHash", item.WPHash.Hex())

				// Unlock and call OnTimeoutOrFailure to increment version
				qs.mu.Unlock()
				qs.OnTimeoutOrFailure(bn)
				qs.mu.Lock()
				return // Restart checking after modification
			}

			// Check for fast retry within retry window
			if elapsed < fastRetryWindow &&
				sinceLastSubmit >= qs.config.SubmitRetryInterval &&
				item.SubmitAttempts < qs.config.MaxSubmitRetries {

				log.Info(log.Node, "Queue: Fast retry same version (network failure recovery)",
					"service", qs.serviceID,
					"blockNumber", bn,
					"version", item.Version,
					"attempt", item.SubmitAttempts+1,
					"maxAttempts", qs.config.MaxSubmitRetries,
					"elapsed", elapsed.Round(time.Second),
					"sinceLastSubmit", sinceLastSubmit.Round(time.Second),
					"wpHash", item.WPHash.Hex())

				// Requeue with same version for fast retry
				item.Status = StatusQueued
				qs.Status[bn] = StatusQueued
				qs.Queued[bn] = item
				delete(qs.Inflight, bn)
				continue
			}

		case StatusGuaranteed:
			// Use AccumulateTimeout from GuaranteedAt (more lenient)
			if item.GuaranteedAt.IsZero() {
				continue
			}
			timeout = qs.config.AccumulateTimeout
			refTime = item.GuaranteedAt
			timeoutType = "accumulate"

			elapsed := now.Sub(refTime)
			remaining := timeout - elapsed

			// Log detailed timeout status for debugging
			if elapsed > timeout/2 {
				log.Debug(log.Node, "Queue: Timeout check",
					"service", qs.serviceID,
					"blockNumber", bn,
					"version", item.Version,
					"status", item.Status.String(),
					"timeoutType", timeoutType,
					"elapsed", elapsed.Round(time.Second),
					"timeout", timeout,
					"remaining", remaining.Round(time.Second))
			}

			if elapsed > timeout {
				log.Warn(log.Node, "Queue: Accumulate timeout - will requeue",
					"service", qs.serviceID,
					"blockNumber", bn,
					"version", item.Version,
					"elapsed", elapsed.Round(time.Second),
					"wpHash", item.WPHash.Hex())

				qs.mu.Unlock()
				qs.OnTimeoutOrFailure(bn)
				qs.mu.Lock()
				return
			}

		default:
			continue
		}
	}
}

// Stats returns queue statistics
type QueueStats struct {
	QueuedCount                int
	InflightCount              int
	FinalizedCount             int
	SubmittedCount             int
	GuaranteedCount            int
	AccumulatedCount           int
	DuplicateGuaranteeRejected int // Counter for rejected duplicate guarantees
	DuplicateAccumulateRejected int // Counter for rejected duplicate accumulations
	NonWinningVersionsCanceled int // Counter for canceled non-winning versions
}

// GetStats returns current queue statistics
func (qs *QueueState) GetStats() QueueStats {
	qs.mu.RLock()
	defer qs.mu.RUnlock()

	stats := QueueStats{
		QueuedCount:                len(qs.Queued),
		InflightCount:              len(qs.Inflight),
		FinalizedCount:             len(qs.Finalized),
		DuplicateGuaranteeRejected: qs.duplicateGuaranteeRejected,
		DuplicateAccumulateRejected: qs.duplicateAccumulateRejected,
		NonWinningVersionsCanceled: qs.nonWinningVersionsCanceled,
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
