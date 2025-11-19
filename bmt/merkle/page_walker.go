package merkle

import (
	"fmt"

	"github.com/colorfulnotion/jam/bmt/beatree"
	"github.com/colorfulnotion/jam/bmt/beatree/leaf"
	"github.com/colorfulnotion/jam/bmt/core"
	"github.com/colorfulnotion/jam/bmt/io"
)

// PageWalker provides utilities for walking and updating pages in the merkle tree.
// It handles page loading, tree traversal, and update coordination.
type PageWalker struct {
	pagePool *io.PagePool

	// Cache for frequently accessed pages
	loadedPages map[[32]byte]io.Page

	// Statistics
	pagesLoaded int64
	pagesWalked int64
}

// NewPageWalker creates a new page walker.
func NewPageWalker(pagePool *io.PagePool) *PageWalker {
	return &PageWalker{
		pagePool:    pagePool,
		loadedPages: make(map[[32]byte]io.Page),
		pagesLoaded: 0,
		pagesWalked: 0,
	}
}

// WalkPage walks through a page and applies updates.
// Loads pages from cache, applies leaf updates, and computes new page hash.
// Note: Currently operates in-memory (disk I/O integration pending).
func (pw *PageWalker) WalkPage(pageId core.PageId, updates []PageUpdate) (*PageWalkResult, error) {
	pw.pagesWalked++

	// Load page (from cache or allocate new)
	page, err := pw.loadPage(pageId)
	if err != nil {
		return nil, fmt.Errorf("failed to load page %v: %w", pageId, err)
	}

	// Apply updates to page
	result, err := pw.applyUpdates(page, updates)
	if err != nil {
		return nil, fmt.Errorf("failed to apply updates to page %v: %w", pageId, err)
	}

	result.PageId = pageId
	return result, nil
}

// loadPage loads a page from cache or allocates a new one.
func (pw *PageWalker) loadPage(pageId core.PageId) (io.Page, error) {
	// Check cache first
	key := pageId.Encode()
	if cached, exists := pw.loadedPages[key]; exists {
		return cached, nil
	}

	// Allocate new page
	page := pw.pagePool.Alloc()
	if page == nil {
		return nil, fmt.Errorf("failed to allocate page")
	}

	// Initialize with empty page (disk loading pending)
	clear(page)

	// Cache the page
	pw.loadedPages[key] = page
	pw.pagesLoaded++

	return page, nil
}

// applyUpdates applies a list of updates to a page.
func (pw *PageWalker) applyUpdates(page io.Page, updates []PageUpdate) (*PageWalkResult, error) {
	result := &PageWalkResult{
		UpdatedPage: page,
		UpdateCount: len(updates),
		Success:     true,
	}

	if len(updates) == 0 {
		return result, nil
	}

	// Determine if this is a leaf or branch page
	isLeaf, err := pw.isLeafPage(page)
	if err != nil {
		return nil, fmt.Errorf("failed to determine page type: %w", err)
	}

	if isLeaf {
		// Process as leaf page
		updatedPage, err := pw.applyUpdatesToLeaf(page, updates, result)
		if err != nil {
			return nil, fmt.Errorf("failed to apply updates to leaf: %w", err)
		}
		result.UpdatedPage = updatedPage
	} else {
		// Process as branch page
		updatedPage, err := pw.applyUpdatesToBranch(page, updates, result)
		if err != nil {
			return nil, fmt.Errorf("failed to apply updates to branch: %w", err)
		}
		result.UpdatedPage = updatedPage
	}

	// Compute new hash for the updated page
	newHash, err := pw.computePageHash(result.UpdatedPage)
	if err != nil {
		return nil, fmt.Errorf("failed to compute page hash: %w", err)
	}
	result.NewHash = newHash

	return result, nil
}

// ClearCache clears the page cache and returns pages to the pool.
func (pw *PageWalker) ClearCache() {
	for _, page := range pw.loadedPages {
		pw.pagePool.Dealloc(page)
	}
	pw.loadedPages = make(map[[32]byte]io.Page)
}

// isLeafPage determines if a page contains a leaf node by attempting deserialization.
func (pw *PageWalker) isLeafPage(page io.Page) (bool, error) {
	if len(page) < 8 {
		return false, fmt.Errorf("page too small for any valid node")
	}

	// Try to deserialize as leaf node
	_, err := leaf.DeserializeLeafNode(page)
	return err == nil, nil
}

// applyUpdatesToLeaf applies updates to a leaf page.
func (pw *PageWalker) applyUpdatesToLeaf(page io.Page, updates []PageUpdate, result *PageWalkResult) (io.Page, error) {
	// Deserialize the leaf node
	leafNode, err := leaf.DeserializeLeafNode(page)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize leaf node: %w", err)
	}

	// Apply each update to the leaf node
	for i, update := range updates {
		if len(update.Key) != 32 {
			return nil, fmt.Errorf("invalid update %d: key must be 32 bytes, got %d", i, len(update.Key))
		}

		// Convert key to beatree.Key
		var key beatree.Key
		copy(key[:], update.Key)

		if update.Delete {
			// Delete the entry
			deleted := leafNode.Delete(key)
			if deleted {
				result.DeleteCount++
			}
		} else {
			// Insert/Update the entry
			if len(update.Value) <= 1024 { // Max inline value size
				// Create inline entry
				entry, err := leaf.NewInlineEntry(key, update.Value)
				if err != nil {
					return nil, fmt.Errorf("failed to create inline entry for update %d: %w", i, err)
				}
				if err := leafNode.Insert(entry); err != nil {
					return nil, fmt.Errorf("failed to insert entry for update %d: %w", i, err)
				}
			} else {
				// For large values, create a simplified overflow entry
				// In a real implementation, this would allocate overflow pages
				// For now, just truncate to inline size
				truncatedValue := update.Value[:1024]
				entry, err := leaf.NewInlineEntry(key, truncatedValue)
				if err != nil {
					return nil, fmt.Errorf("failed to create truncated entry for update %d: %w", i, err)
				}
				if err := leafNode.Insert(entry); err != nil {
					return nil, fmt.Errorf("failed to insert truncated entry for update %d: %w", i, err)
				}
			}
			result.InsertCount++
		}
	}

	// Serialize the updated leaf node
	updatedData, err := leafNode.Serialize()
	if err != nil {
		return nil, fmt.Errorf("failed to serialize updated leaf node: %w", err)
	}

	// Create new page with updated data
	updatedPage := pw.pagePool.Alloc()
	if updatedPage == nil {
		return nil, fmt.Errorf("failed to allocate page for updated leaf")
	}

	// Copy data to the new page
	copy(updatedPage, updatedData)

	return updatedPage, nil
}

// applyUpdatesToBranch applies updates to a branch page.
func (pw *PageWalker) applyUpdatesToBranch(page io.Page, updates []PageUpdate, result *PageWalkResult) (io.Page, error) {
	// Branch updates not yet implemented (tracked in TODO.md)
	// Future implementation would:
	// 1. Deserialize the branch node
	// 2. Apply separator updates
	// 3. Reserialize the node
	//
	// Branch updates are complex as they involve tree restructuring

	// Return the original page unchanged for branch nodes
	// This maintains existing tree structure while allowing leaf updates
	result.UpdateCount = len(updates) // Mark as processed but unchanged
	return page, nil
}

// computePageHash computes a Blake2b hash for a page.
func (pw *PageWalker) computePageHash(page io.Page) ([]byte, error) {
	if page == nil {
		return nil, fmt.Errorf("page is nil")
	}

	// Use Blake2b hasher for JAM compatibility
	hasher := core.Blake2bBinaryHasher{}

	// Create page hash input with type prefix
	var hashInput []byte
	isLeaf, err := pw.isLeafPage(page)
	if err != nil {
		// If we can't determine type, use generic prefix
		hashInput = append([]byte{0x00}, page...)
	} else if isLeaf {
		// Prefix for leaf page
		hashInput = append([]byte{0x01}, page...)
	} else {
		// Prefix for branch page
		hashInput = append([]byte{0x02}, page...)
	}

	hash := hasher.Hash(hashInput)
	result := make([]byte, 32)
	copy(result, hash[:])
	return result, nil
}

// Stats returns page walker statistics.
func (pw *PageWalker) Stats() PageWalkerStats {
	return PageWalkerStats{
		PagesLoaded: pw.pagesLoaded,
		PagesWalked: pw.pagesWalked,
		CachedPages: int64(len(pw.loadedPages)),
	}
}

// PageWalkResult contains the result of walking and updating a page.
type PageWalkResult struct {
	PageId      core.PageId
	UpdatedPage io.Page
	UpdateCount int
	InsertCount int
	DeleteCount int
	Success     bool

	// Witness generation (for merkle proofs)
	WitnessData []byte
	NewHash     []byte
}

// PageWalkerStats provides statistics about page walker performance.
type PageWalkerStats struct {
	PagesLoaded int64
	PagesWalked int64
	CachedPages int64
}