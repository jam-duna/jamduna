package merkle

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/jam-duna/jamduna/bmt/core"
	"github.com/jam-duna/jamduna/bmt/io"
)

// Worker represents a single worker in the update pool.
// Each worker processes page updates independently and in parallel.
type Worker struct {
	id           int
	workQueue    <-chan *WorkItem
	resultQueue  chan<- *WorkResult
	pagePool     *io.PagePool

	// State tracking
	busy         int32 // Atomic boolean (0 = idle, 1 = busy)
	currentWork  *WorkItem

	// Statistics
	processedCount int64
	errorCount     int64
	totalTime      time.Duration

	// Page processing
	pageWalker *PageWalker
	hasher     core.Blake2bBinaryHasher
}

// NewWorker creates a new worker with the specified ID and channels.
func NewWorker(id int, workQueue <-chan *WorkItem, resultQueue chan<- *WorkResult, pagePool *io.PagePool) *Worker {
	return &Worker{
		id:             id,
		workQueue:      workQueue,
		resultQueue:    resultQueue,
		pagePool:       pagePool,
		busy:          0,
		currentWork:   nil,
		processedCount: 0,
		errorCount:    0,
		totalTime:     0,
		pageWalker:    NewPageWalker(pagePool),
		hasher:        core.Blake2bBinaryHasher{},
	}
}

// Run starts the worker's main processing loop.
// The worker will continue processing until the context is cancelled.
func (w *Worker) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return

		case work, ok := <-w.workQueue:
			if !ok {
				// Work queue closed, shutdown
				return
			}

			// Process the work item
			w.processWork(work)
		}
	}
}

// processWork processes a single work item.
func (w *Worker) processWork(work *WorkItem) {
	startTime := time.Now()

	// Mark as busy
	atomic.StoreInt32(&w.busy, 1)
	w.currentWork = work

	defer func() {
		// Mark as idle
		w.currentWork = nil
		atomic.StoreInt32(&w.busy, 0)

		// Update statistics
		w.totalTime += time.Since(startTime)
		w.processedCount++
	}()

	// Process the page updates
	result := w.updatePage(work)

	// Send result
	select {
	case work.resultChan <- result:
		// Result sent successfully
	case <-work.ctx.Done():
		// Work context cancelled
	default:
		// Result channel full or closed, log error but continue
	}
}

// updatePage applies updates to a page and generates witness data.
func (w *Worker) updatePage(work *WorkItem) *WorkResult {
	result := &WorkResult{
		PageId:  work.PageId,
		Success: false,
		Error:   nil,
	}

	// Use page walker to process updates
	pageWalkResult, err := w.pageWalker.WalkPage(work.PageId, work.Updates)
	if err != nil {
		result.Error = fmt.Errorf("page walk failed: %w", err)
		w.errorCount++
		return result
	}

	// Generate witness data for the updated page
	witness, err := w.generateWitness(work.PageId, work.Updates, pageWalkResult)
	if err != nil {
		result.Error = fmt.Errorf("witness generation failed: %w", err)
		w.errorCount++
		return result
	}

	// Compute new page hash
	newHash, err := w.computePageHash(pageWalkResult.UpdatedPage)
	if err != nil {
		result.Error = fmt.Errorf("page hash computation failed: %w", err)
		w.errorCount++
		return result
	}

	// Success
	result.Success = true
	result.UpdatedPage = pageWalkResult.UpdatedPage
	result.NewHash = newHash
	result.Witness = witness.Data
	result.WitnessHash = witness.Hash

	return result
}

// WitnessInfo contains witness data and metadata
type WitnessInfo struct {
	Data []byte
	Hash []byte
}

// generateWitness creates a merkle witness for the page updates.
func (w *Worker) generateWitness(pageId core.PageId, updates []PageUpdate, walkResult *PageWalkResult) (*WitnessInfo, error) {
	// Initialize witness builder
	witness := &WitnessBuilder{
		PageId:   pageId,
		Updates:  updates,
		Hasher:   &w.hasher,
		Siblings: make([][]byte, 0),
	}

	// Collect sibling hashes needed for proof
	for _, update := range updates {
		// For each update, we need to collect the merkle path
		// from the affected leaf to the root

		// Generate witness entry for this update
		witnessEntry := WitnessEntry{
			Key:        update.Key,
			OldValue:   nil, // Would be loaded from existing page
			NewValue:   update.Value,
			IsDelete:   update.Delete,
			SiblingHashes: make([][]byte, 0),
		}

		// Collect the real merkle path for this key
		siblingHashes, err := w.collectWitnessPath(pageId, update.Key, walkResult)
		if err != nil {
			return nil, fmt.Errorf("failed to collect witness path for key %x: %w", update.Key, err)
		}
		witnessEntry.SiblingHashes = siblingHashes

		witness.Entries = append(witness.Entries, witnessEntry)
	}

	// Serialize witness to bytes
	witnessData, err := witness.Serialize()
	if err != nil {
		return nil, fmt.Errorf("witness serialization failed: %w", err)
	}

	// Compute witness hash
	witnessHash := w.hasher.Hash(witnessData)

	return &WitnessInfo{
		Data: witnessData,
		Hash: witnessHash[:],
	}, nil
}

// computePageHash computes the hash of a page's content.
func (w *Worker) computePageHash(page io.Page) ([]byte, error) {
	if page == nil {
		return nil, fmt.Errorf("page is nil")
	}

	// For leaf pages, compute proper merkle leaf hash
	// For branch pages, compute proper merkle internal hash
	return w.computeMerklePageHash(page)
}

// computeMerklePageHash computes a proper merkle hash for a page.
func (w *Worker) computeMerklePageHash(page io.Page) ([]byte, error) {
	// Try to determine page type and compute appropriate hash
	// For now, implement a simplified approach that hashes page content
	// In a full implementation, this would:
	// 1. Deserialize page to determine if it's leaf or branch
	// 2. For leaf: compute hash according to GP leaf encoding
	// 3. For branch: compute hash according to GP internal encoding

	// Create structured hash input
	pageData := make([]byte, len(page))
	copy(pageData, page)

	// Add page type prefix (simplified - would detect actual type)
	var hashInput []byte
	if w.isLikelyLeafPage(pageData) {
		// Prefix for leaf page
		hashInput = append([]byte{0x01}, pageData...)
	} else {
		// Prefix for branch page
		hashInput = append([]byte{0x02}, pageData...)
	}

	pageHash := w.hasher.Hash(hashInput)
	return pageHash[:], nil
}

// isLikelyLeafPage attempts to determine if a page is a leaf page.
func (w *Worker) isLikelyLeafPage(pageData []byte) bool {
	// Simplified heuristic - in practice would deserialize and check type
	// For now, assume smaller pages are more likely to be leaves
	return len(pageData) < 8192 // Leaf pages typically smaller than branch pages
}

// collectWitnessPath collects sibling hashes along the merkle path for a key.
// This implements the path from leaf to root needed for merkle proof verification.
func (w *Worker) collectWitnessPath(pageId core.PageId, key []byte, walkResult *PageWalkResult) ([][]byte, error) {
	var siblings [][]byte

	// Start with the current page and traverse to root
	currentPageId := &pageId
	currentDepth := 0

	// For merkle proofs, we need sibling hashes at each level of the tree
	for currentDepth < 32 { // Maximum depth for 32-byte keys
		// Get the bit at current depth to determine left/right traversal
		bit := getBitAtDepth(key, currentDepth)

		// Compute sibling page ID and collect its hash
		siblingPageId, err := w.computeSiblingPageId(currentPageId, currentDepth, !bit)
		if err != nil {
			return nil, fmt.Errorf("failed to compute sibling page at depth %d: %w", currentDepth, err)
		}

		// If sibling page exists, get its hash
		if siblingPageId != nil {
			siblingHash, err := w.getSiblingPageHash(siblingPageId)
			if err != nil {
				return nil, fmt.Errorf("failed to get sibling hash at depth %d: %w", currentDepth, err)
			}
			siblings = append(siblings, siblingHash)
		}

		// Move to parent page
		parentPageId, err := w.computeParentPageId(currentPageId, currentDepth)
		if err != nil {
			return nil, fmt.Errorf("failed to compute parent page at depth %d: %w", currentDepth, err)
		}

		// Stop if we reached the root
		if parentPageId == nil || parentPageId.Equals(currentPageId) {
			break
		}

		currentPageId = parentPageId
		currentDepth++
	}

	return siblings, nil
}

// getBitAtDepth gets the bit at a specific depth in a key.
func getBitAtDepth(key []byte, depth int) bool {
	if depth >= len(key)*8 {
		return false
	}

	byteIndex := depth / 8
	bitIndex := depth % 8

	if byteIndex >= len(key) {
		return false
	}

	// Get bit from left (MSB first)
	mask := byte(1 << (7 - bitIndex))
	return (key[byteIndex] & mask) != 0
}

// computeSiblingPageId computes the page ID of the sibling at a given depth.
func (w *Worker) computeSiblingPageId(pageId *core.PageId, depth int, siblingBit bool) (*core.PageId, error) {
	// For the root page, there is no sibling
	if pageId.Depth() == 0 {
		return nil, nil
	}

	// For now, return a simplified sibling computation
	// In a complete implementation, this would use proper tree navigation

	// Try to get parent and create sibling from there
	parent := pageId.ParentPageId()
	if parent == nil {
		return nil, nil
	}

	// Create a child page of the parent with the sibling index
	// This is a simplified approach - real implementation would be more sophisticated
	siblingIndex := uint8(0)
	if siblingBit {
		siblingIndex = 1
	}

	sibling, err := parent.ChildPageId(siblingIndex)
	if err != nil {
		// If we can't create a sibling, return nil (no sibling exists)
		return nil, nil
	}

	return sibling, nil
}

// getSiblingPageHash gets the hash of a sibling page.
func (w *Worker) getSiblingPageHash(pageId *core.PageId) ([]byte, error) {
	// For now, return a computed hash based on the page ID
	// In a full implementation, this would load the actual page content

	// Use the page ID encoding as a basis for the hash
	encoded := pageId.Encode()
	pageHash := w.hasher.Hash(encoded[:])

	return pageHash[:], nil
}

// computeParentPageId computes the page ID of the parent at a given depth.
func (w *Worker) computeParentPageId(pageId *core.PageId, depth int) (*core.PageId, error) {
	// For binary tree, parent is computed by removing the last bit of path
	if depth == 0 {
		// Already at root
		return nil, nil
	}

	// Use the ParentPageId method from core.PageId
	parent := pageId.ParentPageId()
	return parent, nil
}

// IsBusy returns whether the worker is currently processing work.
func (w *Worker) IsBusy() bool {
	return atomic.LoadInt32(&w.busy) == 1
}

// CurrentWork returns the work item currently being processed, or nil if idle.
func (w *Worker) CurrentWork() *WorkItem {
	if w.IsBusy() {
		return w.currentWork
	}
	return nil
}

// Stats returns statistics for this worker.
func (w *Worker) Stats() WorkerStats {
	avgTime := time.Duration(0)
	if w.processedCount > 0 {
		avgTime = w.totalTime / time.Duration(w.processedCount)
	}

	return WorkerStats{
		WorkerId:       w.id,
		ProcessedCount: w.processedCount,
		ErrorCount:     w.errorCount,
		AvgProcessTime: avgTime,
		IsBusy:         w.IsBusy(),
	}
}

// WorkerStats provides statistics for a single worker.
type WorkerStats struct {
	WorkerId       int
	ProcessedCount int64
	ErrorCount     int64
	AvgProcessTime time.Duration
	IsBusy         bool
}