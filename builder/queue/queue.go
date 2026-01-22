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
	StatusQueued WorkPackageBundleStatus = iota
	StatusSubmitted
	StatusGuaranteed
	StatusAccumulated
	StatusFinalized
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

const (
	DefaultMaxQueueDepth       = 12
	DefaultMaxInflight         = 3  // Max items in Submitted state (cores blocked waiting for guarantee)
	DefaultMaxTxsPerBundle     = 5
	DefaultQueueDepthBuffer    = 4
	DefaultSubmitRetryInterval = 6 * time.Second // Fast retry interval (1 JAM block)
	DefaultMaxSubmitRetries    = 8               // Max fast retries within validity window

	// GuaranteeTimeout must exceed refine validity window (max 42s) to prevent duplicate guarantees.
	// Set to 60s = 42s validity + 18s safety margin.
	DefaultGuaranteeTimeout      = 60 * time.Second
	DefaultAccumulateTimeout     = 60 * time.Second
	DefaultRetentionWindow       = 100
	DefaultMaxVersionRetries     = 5
	DefaultMaxAccumulateTimeouts = 3 // Max accumulate timeouts before marking guaranteed bundle as failed
)

type TxPoolProvider interface {
	GetPendingOnlyCount() int
	GetInBundleCount() int
}

type QueueConfig struct {
	MaxQueueDepth         int
	MaxInflight           int
	SubmitRetryInterval   time.Duration
	MaxSubmitRetries      int
	GuaranteeTimeout      time.Duration
	AccumulateTimeout     time.Duration
	RetentionWindow       int
	MaxVersionRetries     int
	MaxAccumulateTimeouts int
	MaxTxsPerBundle       int
	QueueDepthBuffer      int
}

func DefaultConfig() QueueConfig {
	return QueueConfig{
		MaxQueueDepth:         DefaultMaxQueueDepth,
		MaxInflight:           DefaultMaxInflight,
		SubmitRetryInterval:   DefaultSubmitRetryInterval,
		MaxSubmitRetries:      DefaultMaxSubmitRetries,
		GuaranteeTimeout:      DefaultGuaranteeTimeout,
		AccumulateTimeout:     DefaultAccumulateTimeout,
		RetentionWindow:       DefaultRetentionWindow,
		MaxVersionRetries:     DefaultMaxVersionRetries,
		MaxAccumulateTimeouts: DefaultMaxAccumulateTimeouts,
		MaxTxsPerBundle:       DefaultMaxTxsPerBundle,
		QueueDepthBuffer:      DefaultQueueDepthBuffer,
	}
}

type QueueItem struct {
	BundleID                   string
	BlockNumber                uint64
	Version                    int
	EventID                    uint64
	AddTS                      time.Time
	Bundle                     *types.WorkPackageBundle
	WPHash                     common.Hash // Changes on resubmit due to RefineContext
	SubmittedAt                time.Time
	GuaranteedAt               time.Time
	Status                     WorkPackageBundleStatus
	CoreIndex                  uint16
	OriginalExtrinsics         []types.ExtrinsicsBlobs      // Before UBT witness prepending
	OriginalWorkItemExtrinsics [][]types.WorkItemExtrinsic  // Before UBT witness prepending
	TransactionHashes          []common.Hash
	PreRoot                    common.Hash // State root before execution
	PostRoot                   common.Hash // State root after execution
	SubmitAttempts             int
	LastSubmitAt               time.Time
	AccumulateTimeoutCount     int
	BlockCommitment            common.Hash // Stable content hash for root lookup
}

type BlockCommitmentInfo struct {
	PreRoot     common.Hash
	PostRoot    common.Hash
	TxHashes    []common.Hash
	BlockNumber uint64
}

type QueueState struct {
	mu                          sync.RWMutex
	config                      QueueConfig
	serviceID                   uint32
	Queued                      map[uint64]*QueueItem
	Inflight                    map[uint64]*QueueItem
	Status                      map[uint64]WorkPackageBundleStatus
	CurrentVer                  map[uint64]int
	HashByVer                   map[uint64]map[int]common.Hash
	HashToBlock                 map[common.Hash]uint64 // Reverse lookup: wpHash -> blockNumber
	WinningVer                  map[uint64]int         // First guaranteed version wins
	Finalized                   map[uint64]*QueueItem
	nextBlockNumber             uint64
	eventIDCounter              uint64
	duplicateGuaranteeRejected  int
	duplicateAccumulateRejected int
	nonWinningVersionsCanceled  int
	onStatusChange              func(blockNumber uint64, oldStatus, newStatus WorkPackageBundleStatus)
	onAccumulated               func(wpHash common.Hash, blockNumber uint64, txHashes []common.Hash)
	onFailed                    func(wpHash common.Hash, blockNumber uint64, txHashes []common.Hash)
	BlockCommitmentData         map[common.Hash]*BlockCommitmentInfo
	txPoolProvider              TxPoolProvider
}

func NewQueueState(serviceID uint32) *QueueState {
	return NewQueueStateWithConfig(serviceID, DefaultConfig())
}

func NewQueueStateWithConfig(serviceID uint32, config QueueConfig) *QueueState {
	return &QueueState{
		config:              config,
		serviceID:           serviceID,
		Queued:              make(map[uint64]*QueueItem),
		Inflight:            make(map[uint64]*QueueItem),
		Status:              make(map[uint64]WorkPackageBundleStatus),
		CurrentVer:          make(map[uint64]int),
		HashByVer:           make(map[uint64]map[int]common.Hash),
		HashToBlock:         make(map[common.Hash]uint64),
		WinningVer:          make(map[uint64]int),
		Finalized:           make(map[uint64]*QueueItem),
		BlockCommitmentData: make(map[common.Hash]*BlockCommitmentInfo),
		nextBlockNumber:     1, // Start from block 1 (0 is genesis)
		eventIDCounter:      0,
	}
}

func (qs *QueueState) SetTxPoolProvider(provider TxPoolProvider) {
	qs.mu.Lock()
	defer qs.mu.Unlock()
	qs.txPoolProvider = provider
}

func (qs *QueueState) getMaxQueueDepth() int {
	if qs.txPoolProvider == nil {
		return qs.config.MaxQueueDepth
	}

	pendingTxs := qs.txPoolProvider.GetPendingOnlyCount()
	inBundleTxs := qs.txPoolProvider.GetInBundleCount()
	totalInflightTxs := pendingTxs + inBundleTxs

	maxTxsPerBundle := qs.config.MaxTxsPerBundle
	if maxTxsPerBundle <= 0 {
		maxTxsPerBundle = DefaultMaxTxsPerBundle
	}

	// Calculate: ceil(totalInflightTxs / maxTxsPerBundle) + buffer
	bundlesNeeded := (totalInflightTxs + maxTxsPerBundle - 1) / maxTxsPerBundle
	dynamicDepth := bundlesNeeded + qs.config.QueueDepthBuffer

	// Use the larger of dynamic calculation or static minimum
	if dynamicDepth < qs.config.MaxQueueDepth {
		return qs.config.MaxQueueDepth
	}
	return dynamicDepth
}

func (qs *QueueState) SetStatusChangeCallback(cb func(blockNumber uint64, oldStatus, newStatus WorkPackageBundleStatus)) {
	qs.mu.Lock()
	defer qs.mu.Unlock()
	qs.onStatusChange = cb
}

func (qs *QueueState) SetOnAccumulated(cb func(wpHash common.Hash, blockNumber uint64, txHashes []common.Hash)) {
	qs.mu.Lock()
	defer qs.mu.Unlock()
	qs.onAccumulated = cb
}

func (qs *QueueState) SetOnFailed(cb func(wpHash common.Hash, blockNumber uint64, txHashes []common.Hash)) {
	qs.mu.Lock()
	defer qs.mu.Unlock()
	qs.onFailed = cb
}

func (qs *QueueState) StoreBlockCommitmentData(commitment common.Hash, info *BlockCommitmentInfo) {
	if commitment == (common.Hash{}) || info == nil {
		return
	}
	qs.mu.Lock()
	defer qs.mu.Unlock()

	txHashesCopy := make([]common.Hash, len(info.TxHashes))
	copy(txHashesCopy, info.TxHashes)

	qs.BlockCommitmentData[commitment] = &BlockCommitmentInfo{
		PreRoot:     info.PreRoot,
		PostRoot:    info.PostRoot,
		TxHashes:    txHashesCopy,
		BlockNumber: info.BlockNumber,
	}

	log.Debug(log.Node, "Queue: Stored BlockCommitmentData",
		"service", qs.serviceID,
		"commitment", commitment.Hex(),
		"blockNumber", info.BlockNumber,
		"preRoot", info.PreRoot.Hex(),
		"postRoot", info.PostRoot.Hex(),
		"txCount", len(info.TxHashes))
}

func (qs *QueueState) GetBlockCommitmentData(commitment common.Hash) *BlockCommitmentInfo {
	qs.mu.RLock()
	defer qs.mu.RUnlock()
	return qs.BlockCommitmentData[commitment]
}

func (qs *QueueState) GetBlockCommitmentDataByBlockNumber(blockNumber uint64) *BlockCommitmentInfo {
	qs.mu.RLock()
	defer qs.mu.RUnlock()
	for _, info := range qs.BlockCommitmentData {
		if info.BlockNumber == blockNumber {
			return info
		}
	}
	return nil
}

func (qs *QueueState) nextEventID() uint64 {
	qs.eventIDCounter++
	return qs.eventIDCounter
}

func (qs *QueueState) PeekNextBlockNumber() uint64 {
	qs.mu.RLock()
	defer qs.mu.RUnlock()
	return qs.nextBlockNumber
}

func (qs *QueueState) ReserveNextBlockNumber() uint64 {
	qs.mu.Lock()
	defer qs.mu.Unlock()
	blockNumber := qs.nextBlockNumber
	qs.nextBlockNumber++
	return blockNumber
}

func (qs *QueueState) Enqueue(bundle *types.WorkPackageBundle, coreIndex uint16) (uint64, error) {
	qs.mu.Lock()
	defer qs.mu.Unlock()

	maxDepth := qs.getMaxQueueDepth()
	if len(qs.Queued) >= maxDepth {
		return 0, fmt.Errorf("queue full: %d items queued (max %d)", len(qs.Queued), maxDepth)
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

func (qs *QueueState) EnqueueWithOriginalExtrinsics(bundle *types.WorkPackageBundle, originalExtrinsics []types.ExtrinsicsBlobs, originalWorkItemExtrinsics [][]types.WorkItemExtrinsic, coreIndex uint16, txHashes []common.Hash) (uint64, error) {
	return qs.EnqueueWithRoots(bundle, originalExtrinsics, originalWorkItemExtrinsics, coreIndex, txHashes, common.Hash{}, common.Hash{})
}

func (qs *QueueState) EnqueueWithRoots(bundle *types.WorkPackageBundle, originalExtrinsics []types.ExtrinsicsBlobs, originalWorkItemExtrinsics [][]types.WorkItemExtrinsic, coreIndex uint16, txHashes []common.Hash, preRoot, postRoot common.Hash) (uint64, error) {
	qs.mu.Lock()
	defer qs.mu.Unlock()

	maxDepth := qs.getMaxQueueDepth()
	if len(qs.Queued) >= maxDepth {
		return 0, fmt.Errorf("queue full: %d items queued (max %d)", len(qs.Queued), maxDepth)
	}

	blockNumber := qs.nextBlockNumber
	qs.nextBlockNumber++

	return qs.enqueueWithBlockNumberAndRoots(bundle, originalExtrinsics, originalWorkItemExtrinsics, coreIndex, txHashes, blockNumber, preRoot, postRoot, common.Hash{})
}

func (qs *QueueState) EnqueueWithReservedBlockNumber(bundle *types.WorkPackageBundle, originalExtrinsics []types.ExtrinsicsBlobs, originalWorkItemExtrinsics [][]types.WorkItemExtrinsic, coreIndex uint16, txHashes []common.Hash, blockNumber uint64, preRoot, postRoot, blockCommitment common.Hash) error {
	qs.mu.Lock()
	defer qs.mu.Unlock()

	maxDepth := qs.getMaxQueueDepth()
	if len(qs.Queued) >= maxDepth {
		return fmt.Errorf("queue full: %d items queued (max %d)", len(qs.Queued), maxDepth)
	}

	_, err := qs.enqueueWithBlockNumberAndRoots(bundle, originalExtrinsics, originalWorkItemExtrinsics, coreIndex, txHashes, blockNumber, preRoot, postRoot, blockCommitment)
	return err
}

func (qs *QueueState) enqueueWithBlockNumber(bundle *types.WorkPackageBundle, originalExtrinsics []types.ExtrinsicsBlobs, originalWorkItemExtrinsics [][]types.WorkItemExtrinsic, coreIndex uint16, txHashes []common.Hash, blockNumber uint64) (uint64, error) {
	return qs.enqueueWithBlockNumberAndRoots(bundle, originalExtrinsics, originalWorkItemExtrinsics, coreIndex, txHashes, blockNumber, common.Hash{}, common.Hash{}, common.Hash{})
}

func (qs *QueueState) enqueueWithBlockNumberAndRoots(bundle *types.WorkPackageBundle, originalExtrinsics []types.ExtrinsicsBlobs, originalWorkItemExtrinsics [][]types.WorkItemExtrinsic, coreIndex uint16, txHashes []common.Hash, blockNumber uint64, preRoot, postRoot, blockCommitment common.Hash) (uint64, error) {
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
		PreRoot:                    preRoot,
		PostRoot:                   postRoot,
		BlockCommitment:            blockCommitment,
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

// inflight counts only StatusSubmitted items (not Guaranteed) to control submission rate.
// Guaranteed items no longer consume inflight slots since they can't be resubmitted.
func (qs *QueueState) inflight() int {
	count := 0
	for _, status := range qs.Status {
		if status == StatusSubmitted {
			count++
		}
	}
	return count
}

func (qs *QueueState) CanSubmit() bool {
	qs.mu.RLock()
	defer qs.mu.RUnlock()
	return qs.inflight() < qs.config.MaxInflight && len(qs.Queued) > 0
}

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

// cancelOtherVersions provides LOCAL consistency only - it cleans up our queue state.
// It cannot prevent on-chain duplicates if multiple versions were already guaranteed by validators.
// That's why GuaranteeTimeout must exceed refine validity to prevent submitting new versions
// while old ones might still be valid on-chain.
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
		log.Debug(log.Node, "Queue: OnGuaranteed for already-chosen winning version",
			"blockNumber", bn,
			"version", ver)
	} else {
		qs.WinningVer[bn] = ver
		qs.CurrentVer[bn] = ver
		log.Info(log.Node, "🏆 Queue: First guarantee - this version WINS (canceling all others)",
			"service", qs.serviceID,
			"blockNumber", bn,
			"winningVersion", ver,
			"wpHash", wpHash.Hex())
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

func (qs *QueueState) OnAccumulated(wpHash common.Hash) {
	qs.mu.Lock()

	bn, ver := qs.lookupByHash(wpHash)
	if bn == 0 {
		qs.mu.Unlock()
		log.Debug(log.Node, "Queue: OnAccumulated for unknown hash", "wpHash", wpHash.Hex())
		return
	}

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
	var callback func(common.Hash, uint64, []common.Hash)
	if item, ok := qs.Inflight[bn]; ok {
		item.Status = StatusAccumulated
		bundleID = item.BundleID
		txHashes = item.TransactionHashes
		qs.Finalized[bn] = item
		delete(qs.Inflight, bn)
	} else if item, ok := qs.Queued[bn]; ok {
		item.Status = StatusAccumulated
		bundleID = item.BundleID
		txHashes = item.TransactionHashes
		qs.Finalized[bn] = item
		delete(qs.Queued, bn)
	}

	if qs.onStatusChange != nil {
		qs.onStatusChange(bn, oldStatus, StatusAccumulated)
	}

	if qs.onAccumulated != nil {
		callback = qs.onAccumulated
	}

	qs.pruneOlder(bn)
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

	qs.mu.Unlock()
	if callback != nil {
		callback(wpHash, bn, txHashes)
	}
}

func (qs *QueueState) OnFinalized(wpHash common.Hash) {
	qs.mu.Lock()
	defer qs.mu.Unlock()

	bn, ver := qs.lookupByHash(wpHash)
	if bn == 0 {
		log.Debug(log.Node, "Queue: OnFinalized for unknown hash", "wpHash", wpHash.Hex())
		return
	}

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

func (qs *QueueState) OnTimeoutOrFailure(failedBN uint64) {
	qs.mu.Lock()

	now := time.Now()
	var toRequeue []uint64
	for bn := range qs.Inflight {
		if bn >= failedBN {
			toRequeue = append(toRequeue, bn)
		}
	}

	type failedItem struct {
		wpHash      common.Hash
		blockNumber uint64
		txHashes    []common.Hash
	}
	var failedItems []failedItem

	for _, bn := range toRequeue {
		item := qs.Inflight[bn]
		if item == nil {
			continue
		}

		currentStatus := qs.Status[bn]
		if currentStatus == StatusGuaranteed || currentStatus == StatusAccumulated || currentStatus == StatusFinalized {
			log.Info(log.Node, "Queue: OnTimeoutOrFailure skipping - bundle already guaranteed/accumulated",
				"service", qs.serviceID,
				"blockNumber", bn,
				"currentStatus", currentStatus.String(),
				"wpHash", item.WPHash.Hex())
			continue
		}

		newVersion := qs.CurrentVer[bn] + 1
		if newVersion > qs.config.MaxVersionRetries {
			log.Warn(log.Node, "Queue: Max retries exceeded, dropping block",
				"service", qs.serviceID,
				"blockNumber", bn,
				"version", newVersion-1,
				"txCount", len(item.TransactionHashes))
			qs.Status[bn] = StatusFailed
			delete(qs.Inflight, bn)
			if len(item.TransactionHashes) > 0 {
				failedItems = append(failedItems, failedItem{
					wpHash:      item.WPHash,
					blockNumber: bn,
					txHashes:    item.TransactionHashes,
				})
			}
			continue
		}

		qs.CurrentVer[bn] = newVersion
		delete(qs.Status, bn)
		item.Version = newVersion
		item.AddTS = now
		item.Status = StatusQueued
		item.SubmittedAt = time.Time{}
		item.LastSubmitAt = time.Time{}
		item.SubmitAttempts = 0

		qs.Queued[bn] = item
		qs.Status[bn] = StatusQueued
		delete(qs.Inflight, bn)

		log.Info(log.Node, "🔄 Queue: Requeued after failure",
			"service", qs.serviceID,
			"blockNumber", bn,
			"newVersion", newVersion,
			"wpHash", item.WPHash.Hex())
	}

	callback := qs.onFailed
	qs.mu.Unlock()
	if callback != nil {
		for _, failed := range failedItems {
			callback(failed.wpHash, failed.blockNumber, failed.txHashes)
		}
	}
}

func (qs *QueueState) GetItemByWPHash(wpHash common.Hash) *QueueItem {
	bn, _ := qs.lookupByHash(wpHash)
	if bn == 0 {
		return nil
	}
	if item, ok := qs.Inflight[bn]; ok && item.WPHash == wpHash {
		return item
	}
	if item, ok := qs.Queued[bn]; ok && item.WPHash == wpHash {
		return item
	}
	if item, ok := qs.Finalized[bn]; ok && item.WPHash == wpHash {
		return item
	}

	return nil
}

func (qs *QueueState) lookupByHash(wpHash common.Hash) (blockNumber uint64, version int) {
	if bn, ok := qs.HashToBlock[wpHash]; ok {
		if versions, ok := qs.HashByVer[bn]; ok {
			for ver, hash := range versions {
				if hash == wpHash {
					return bn, ver
				}
			}
		}
		return bn, qs.CurrentVer[bn]
	}
	for bn, versions := range qs.HashByVer {
		for ver, hash := range versions {
			if hash == wpHash {
				return bn, ver
			}
		}
	}
	return 0, 0
}

func (qs *QueueState) pruneOlder(currentBN uint64) {
	if currentBN <= uint64(qs.config.RetentionWindow) {
		return
	}

	cutoff := currentBN - uint64(qs.config.RetentionWindow)
	for bn := range qs.Finalized {
		if bn < cutoff {
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

// CheckTimeouts implements two-tier timeout strategy:
// 1. Fast retry (same version): For network failures within refine validity window
// 2. Slow timeout (new version): After refine validity expires, increment version and rebuild
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

			elapsed := now.Sub(item.SubmittedAt)
			sinceLastSubmit := now.Sub(item.LastSubmitAt)

			recentHistoryWindow := time.Duration(types.RecentHistorySize*types.SecondsPerSlot) * time.Second
			fastRetryWindow := recentHistoryWindow
			if qs.config.GuaranteeTimeout < fastRetryWindow {
				fastRetryWindow = qs.config.GuaranteeTimeout
			}

			timeout = qs.config.GuaranteeTimeout
			refTime = item.SubmittedAt
			timeoutType = "guarantee"

			if elapsed > timeout {
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

				qs.mu.Unlock()
				qs.OnTimeoutOrFailure(bn)
				qs.mu.Lock()
				return
			}

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

				item.Status = StatusQueued
				qs.Status[bn] = StatusQueued
				qs.Queued[bn] = item
				delete(qs.Inflight, bn)
				continue
			}

		case StatusGuaranteed:
			if item.GuaranteedAt.IsZero() {
				continue
			}
			timeout = qs.config.AccumulateTimeout
			refTime = item.GuaranteedAt
			timeoutType = "accumulate"

			elapsed := now.Sub(refTime)
			remaining := timeout - elapsed
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
				item.AccumulateTimeoutCount++

				log.Warn(log.Node, "Queue: Accumulate timeout for guaranteed bundle",
					"service", qs.serviceID,
					"blockNumber", bn,
					"version", item.Version,
					"elapsed", elapsed.Round(time.Second),
					"accumulateTimeoutCount", item.AccumulateTimeoutCount,
					"maxAccumulateTimeouts", qs.config.MaxAccumulateTimeouts,
					"wpHash", item.WPHash.Hex())

				if item.AccumulateTimeoutCount >= qs.config.MaxAccumulateTimeouts {
					log.Error(log.Node, "Queue: Guaranteed bundle stuck - max accumulate timeouts exceeded, marking as FAILED",
						"service", qs.serviceID,
						"blockNumber", bn,
						"version", item.Version,
						"accumulateTimeoutCount", item.AccumulateTimeoutCount,
						"wpHash", item.WPHash.Hex(),
						"hint", "Bundle was guaranteed but never accumulated. Check accumulate entry point logs on JAM node.")

					qs.Status[bn] = StatusFailed
					failedItem := item
					delete(qs.Inflight, bn)
					delete(qs.WinningVer, bn)
					delete(qs.CurrentVer, bn)
					if qs.onFailed != nil {
						qs.mu.Unlock()
						qs.onFailed(failedItem.WPHash, failedItem.BlockNumber, failedItem.TransactionHashes)
						qs.mu.Lock()
					}
					return
				}

				item.GuaranteedAt = now
				log.Info(log.Node, "Queue: Resetting accumulate timeout window for stuck guaranteed bundle",
					"service", qs.serviceID,
					"blockNumber", bn,
					"remainingAttempts", qs.config.MaxAccumulateTimeouts-item.AccumulateTimeoutCount,
					"wpHash", item.WPHash.Hex())
				return
			}

		default:
			continue
		}
	}
}

type QueueStats struct {
	QueuedCount                int
	InflightCount              int
	FinalizedCount             int
	SubmittedCount             int
	GuaranteedCount             int
	AccumulatedCount            int
	DuplicateGuaranteeRejected  int
	DuplicateAccumulateRejected int
	NonWinningVersionsCanceled  int
}

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
