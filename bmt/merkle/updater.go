package merkle

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/colorfulnotion/jam/bmt/cache"
	"github.com/colorfulnotion/jam/bmt/core"
	"github.com/colorfulnotion/jam/bmt/io"
)

// Updater coordinates parallel merkle tree updates using worker pools and page caching.
// It manages the overall update process, including page elision, witness generation,
// and root hash computation.
type Updater struct {
	// Worker pool for parallel processing
	updatePool *UpdatePool

	// Page cache for hot pages
	pageCache *cache.PageCache

	// Configuration
	pagePool     *io.PagePool
	hasher       core.Blake2bBinaryHasher
	elisionLimit int // Minimum leaves to avoid elision

	// Update coordination
	mu             sync.RWMutex
	activeUpdates  map[[32]byte]*UpdateSession
	pendingUpdates []BatchUpdate

	// Statistics
	stats UpdaterStats
}

// UpdateSession tracks an ongoing update operation.
type UpdateSession struct {
	PageId    core.PageId
	Updates   []PageUpdate
	StartTime time.Time
	Context   context.Context
	Cancel    context.CancelFunc
}

// PageUpdateBatch represents updates for a single page.
type PageUpdateBatch struct {
	PageId  core.PageId
	Updates []PageUpdate
}

// PageResult represents the result of updating a page.
type PageResult struct {
	PageId core.PageId
	Page   io.Page
}

// BatchUpdate represents a batch of updates to be processed together.
type BatchUpdate struct {
	PageIds  []core.PageId
	Updates  map[[32]byte][]PageUpdate
	Priority int
}

// UpdaterStats tracks updater performance metrics.
type UpdaterStats struct {
	TotalUpdates     int64
	SuccessfulUpdates int64
	FailedUpdates    int64
	AverageLatency   time.Duration
	RootHashUpdates  int64
	WitnessesGenerated int64
	PagesElided      int64
}

// NewUpdater creates a new merkle updater with the specified configuration.
func NewUpdater(workerCount int, cacheSize int, pagePool *io.PagePool) *Updater {
	updatePool := NewUpdatePool(workerCount, pagePool)
	pageCache := cache.NewPageCache(cacheSize, pagePool)

	return &Updater{
		updatePool:     updatePool,
		pageCache:      pageCache,
		pagePool:       pagePool,
		hasher:         core.Blake2bBinaryHasher{},
		elisionLimit:   20, // Elide pages with < 20 leaves
		activeUpdates:  make(map[[32]byte]*UpdateSession),
		pendingUpdates: make([]BatchUpdate, 0),
		stats:          UpdaterStats{},
	}
}

// UpdateTreeParallel performs parallel updates on multiple pages and computes new root hash.
// This is the main entry point for merkle tree updates.
func (u *Updater) UpdateTreeParallel(rootPageId core.PageId, updates []PageUpdateBatch) (*UpdateResult, error) {
	startTime := time.Now()

	u.mu.Lock()
	u.stats.TotalUpdates++
	u.mu.Unlock()

	// Create update context
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Process updates in parallel
	results, err := u.processUpdatesParallel(ctx, updates)
	if err != nil {
		u.mu.Lock()
		u.stats.FailedUpdates++
		u.mu.Unlock()
		return nil, fmt.Errorf("parallel update failed: %w", err)
	}

	// Compute new root hash
	newRootHash, err := u.computeNewRootHash(rootPageId, results)
	if err != nil {
		u.mu.Lock()
		u.stats.FailedUpdates++
		u.mu.Unlock()
		return nil, fmt.Errorf("root hash computation failed: %w", err)
	}

	// Generate consolidated witness
	witness, err := u.generateConsolidatedWitness(results)
	if err != nil {
		u.mu.Lock()
		u.stats.FailedUpdates++
		u.mu.Unlock()
		return nil, fmt.Errorf("witness generation failed: %w", err)
	}

	// Verify the generated witness
	if err := u.VerifyConsolidatedWitness(witness, newRootHash); err != nil {
		u.mu.Lock()
		u.stats.FailedUpdates++
		u.mu.Unlock()
		return nil, fmt.Errorf("witness verification failed: %w", err)
	}

	// Update statistics
	elapsed := time.Since(startTime)
	u.mu.Lock()
	u.stats.SuccessfulUpdates++
	u.stats.RootHashUpdates++
	u.stats.WitnessesGenerated++

	// Update average latency (simple moving average)
	if u.stats.AverageLatency == 0 {
		u.stats.AverageLatency = elapsed
	} else {
		u.stats.AverageLatency = (u.stats.AverageLatency + elapsed) / 2
	}
	u.mu.Unlock()

	result := &UpdateResult{
		RootPageId:   rootPageId,
		NewRootHash:  newRootHash,
		UpdatedPages: make([]PageResult, 0, len(results)),
		Witness:      witness,
		Success:      true,
		Elapsed:      elapsed,
	}

	// Collect updated pages
	for _, workResult := range results {
		if workResult.Success {
			result.UpdatedPages = append(result.UpdatedPages, PageResult{
				PageId: workResult.PageId,
				Page:   workResult.UpdatedPage,
			})
		}
	}

	return result, nil
}

// processUpdatesParallel processes all page updates in parallel using the worker pool.
func (u *Updater) processUpdatesParallel(ctx context.Context, updates []PageUpdateBatch) ([]*WorkResult, error) {
	if len(updates) == 0 {
		return nil, nil
	}

	// Submit all work items to the pool
	workItems := make([]*WorkItem, 0, len(updates))
	for _, updateBatch := range updates {
		// Check if page should be elided
		if len(updateBatch.Updates) < u.elisionLimit {
			u.mu.Lock()
			u.stats.PagesElided++
			u.mu.Unlock()
			continue
		}

		workItem := &WorkItem{
			PageId:   updateBatch.PageId,
			Updates:  updateBatch.Updates,
			Priority: len(updateBatch.Updates), // Higher priority for more updates
			ctx:      ctx,
		}
		workItems = append(workItems, workItem)
	}

	// Process batch through worker pool
	results, err := u.updatePool.ProcessBatch(workItems)
	if err != nil {
		return nil, fmt.Errorf("batch processing failed: %w", err)
	}

	return results, nil
}

// computeNewRootHash computes the new root hash after all updates.
func (u *Updater) computeNewRootHash(rootPageId core.PageId, results []*WorkResult) ([]byte, error) {
	if len(results) == 0 {
		// No updates - compute hash of empty tree
		return u.computeEmptyTreeHash(), nil
	}

	// For single page updates (common case), compute leaf hash directly
	if len(results) == 1 && results[0].Success {
		return u.computeLeafHash(results[0])
	}

	// For multiple pages, build tree structure and compute root
	return u.computeTreeRootHash(rootPageId, results)
}

// computeEmptyTreeHash returns the hash of an empty tree.
func (u *Updater) computeEmptyTreeHash() []byte {
	// Use terminator node hash as per GP trie specification
	terminatorHash := u.hasher.Hash([]byte{0x00}) // Terminator encoding
	result := make([]byte, 32)
	copy(result, terminatorHash[:])
	return result
}

// computeLeafHash computes the hash for a single leaf page.
func (u *Updater) computeLeafHash(result *WorkResult) ([]byte, error) {
	if !result.Success || len(result.NewHash) == 0 {
		return nil, fmt.Errorf("invalid work result for leaf hash computation")
	}

	// For leaf nodes, the NewHash should already be the proper leaf hash
	// computed by the worker including the page content
	leafHash := make([]byte, 32)
	copy(leafHash, result.NewHash)
	return leafHash, nil
}

// computeTreeRootHash computes the root hash for a multi-page tree.
func (u *Updater) computeTreeRootHash(rootPageId core.PageId, results []*WorkResult) ([]byte, error) {
	// Build page hash map
	pageHashes := make(map[[32]byte][]byte)
	for _, result := range results {
		if result.Success && len(result.NewHash) > 0 {
			encodedId := result.PageId.Encode()
			pageHashes[encodedId] = result.NewHash
		}
	}

	// Get root page hash
	rootEncoded := rootPageId.Encode()
	if rootHash, exists := pageHashes[rootEncoded]; exists {
		return rootHash, nil
	}

	// If root page wasn't updated, we need to compute it from children
	return u.computeBranchHash(rootPageId, pageHashes)
}

// computeBranchHash computes hash for a branch node from its children.
func (u *Updater) computeBranchHash(pageId core.PageId, pageHashes map[[32]byte][]byte) ([]byte, error) {
	// For now, implement a simplified branch hash
	// In a full implementation, this would:
	// 1. Load the branch page structure
	// 2. Get child page hashes
	// 3. Compute internal node hash per GP spec

	// Simplified: combine available child hashes
	var combinedHashes []byte
	for _, hash := range pageHashes {
		combinedHashes = append(combinedHashes, hash...)
	}

	if len(combinedHashes) == 0 {
		return u.computeEmptyTreeHash(), nil
	}

	branchHash := u.hasher.Hash(combinedHashes)
	result := make([]byte, 32)
	copy(result, branchHash[:])
	return result, nil
}

// generateConsolidatedWitness generates a single witness covering all updates.
func (u *Updater) generateConsolidatedWitness(results []*WorkResult) ([]byte, error) {
	// Collect all individual witnesses
	witnesses := make([]*Witness, 0, len(results))

	for _, result := range results {
		if result.Success && len(result.Witness) > 0 {
			// Deserialize individual witness
			witness, err := DeserializeWitness(result.Witness)
			if err != nil {
				return nil, fmt.Errorf("failed to deserialize witness for page %v: %w", result.PageId, err)
			}
			witnesses = append(witnesses, witness)
		}
	}

	if len(witnesses) == 0 {
		// Return empty witness for no updates
		emptyWitness := &WitnessBuilder{
			PageId:  core.PageId{},
			Updates: nil,
			Hasher:  &u.hasher,
			Entries: make([]WitnessEntry, 0),
		}
		return emptyWitness.Serialize()
	}

	// Merge witnesses into a single consolidated witness
	consolidated := &WitnessBuilder{
		PageId:  witnesses[0].PageId, // Use first page as representative
		Updates: nil,
		Hasher:  &u.hasher,
		Entries: make([]WitnessEntry, 0),
	}

	// Collect all entries from individual witnesses
	for _, witness := range witnesses {
		consolidated.Entries = append(consolidated.Entries, witness.Entries...)
	}

	return consolidated.Serialize()
}

// VerifyConsolidatedWitness verifies a consolidated witness against the computed root hash.
func (u *Updater) VerifyConsolidatedWitness(witnessData []byte, expectedRootHash []byte) error {
	// Deserialize the witness
	witness, err := DeserializeWitness(witnessData)
	if err != nil {
		return fmt.Errorf("failed to deserialize witness: %w", err)
	}

	// Verify the witness
	if err := witness.VerifyWitness(expectedRootHash, &u.hasher); err != nil {
		return fmt.Errorf("witness verification failed: %w", err)
	}

	return nil
}

// GetCache returns the page cache for external access.
func (u *Updater) GetCache() *cache.PageCache {
	return u.pageCache
}

// GetStats returns current updater statistics.
func (u *Updater) GetStats() UpdaterStats {
	u.mu.RLock()
	defer u.mu.RUnlock()

	// Copy stats to avoid races
	return u.stats
}

// Shutdown gracefully shuts down the updater and all worker pools.
func (u *Updater) Shutdown() {
	// Cancel all active updates
	u.mu.Lock()
	for _, session := range u.activeUpdates {
		session.Cancel()
	}
	u.mu.Unlock()

	// Shutdown worker pool
	u.updatePool.Shutdown()

	// Clear page cache
	u.pageCache.Clear()
}

// UpdateResult contains the result of a parallel merkle tree update.
type UpdateResult struct {
	RootPageId   core.PageId
	NewRootHash  []byte
	UpdatedPages []PageResult
	Witness      []byte
	Success      bool
	Elapsed      time.Duration
}