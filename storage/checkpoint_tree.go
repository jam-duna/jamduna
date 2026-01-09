package storage

import (
	"fmt"
	"sync"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	evmtypes "github.com/colorfulnotion/jam/builder/evm/types"
	lru "github.com/hashicorp/golang-lru/v2"
)

// CheckpointTreeManager manages in-memory UBT tree checkpoints.
type CheckpointTreeManager struct {
	// In-memory checkpoint trees (LRU cache)
	trees *lru.Cache[uint64, *UnifiedBinaryTree]

	// Pinned checkpoints (never evicted)
	pinned map[uint64]*UnifiedBinaryTree

	// Mutex for concurrent access
	mu sync.RWMutex

	// Checkpoint periods
	coarsePeriod uint64 // 7200 blocks (~12h at 2s/block)
	finePeriod   uint64 // 600 blocks (~1h)

	// Block loader for replay (provided by StateDBStorage)
	blockLoader func(uint64) (*evmtypes.EvmBlockPayload, error)
}

// NewCheckpointTreeManager creates a new checkpoint manager
func NewCheckpointTreeManager(
	maxCheckpoints int,
	coarsePeriod uint64,
	finePeriod uint64,
	blockLoader func(uint64) (*evmtypes.EvmBlockPayload, error),
) (*CheckpointTreeManager, error) {
	cache, err := lru.New[uint64, *UnifiedBinaryTree](maxCheckpoints)
	if err != nil {
		return nil, fmt.Errorf("failed to create LRU cache: %w", err)
	}

	return &CheckpointTreeManager{
		trees:        cache,
		pinned:       make(map[uint64]*UnifiedBinaryTree),
		coarsePeriod: coarsePeriod,
		finePeriod:   finePeriod,
		blockLoader:  blockLoader,
	}, nil
}

// GetCheckpoint returns checkpoint tree (from cache, pinned, or rebuilt)
func (cm *CheckpointTreeManager) GetCheckpoint(height uint64) (*UnifiedBinaryTree, error) {
	cm.mu.RLock()

	// Check pinned first (genesis always pinned)
	if tree, ok := cm.pinned[height]; ok {
		cm.mu.RUnlock()
		return tree, nil
	}

	// Check LRU cache
	if tree, ok := cm.trees.Get(height); ok {
		cm.mu.RUnlock()
		return tree, nil
	}

	cm.mu.RUnlock()

	// Rebuild checkpoint
	return cm.rebuildCheckpoint(height)
}

// rebuildCheckpoint rebuilds checkpoint from nearest earlier checkpoint
func (cm *CheckpointTreeManager) rebuildCheckpoint(height uint64) (*UnifiedBinaryTree, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Double-check after acquiring write lock
	if tree, ok := cm.pinned[height]; ok {
		return tree, nil
	}
	if tree, ok := cm.trees.Get(height); ok {
		return tree, nil
	}

	// Find nearest checkpoint < height
	baseHeight, baseTree := cm.findNearestCheckpoint(height)
	if baseTree == nil {
		return nil, fmt.Errorf("no base checkpoint found for height %d (genesis must be pinned)", height)
	}

	// CRITICAL: Copy base tree before replay (prevents mutation of cached checkpoint)
	baseTreeCopy := baseTree.Copy()

	// Verify base checkpoint root matches canonical root at baseHeight
	baseBlock, err := cm.blockLoader(baseHeight)
	if err != nil {
		return nil, fmt.Errorf("failed to load base block %d for verification: %w", baseHeight, err)
	}
	expectedBaseRoot := common.BytesToHash(baseBlock.UBTRoot[:])
	baseHash := baseTreeCopy.RootHash()
	actualBaseRoot := common.BytesToHash(baseHash[:])
	if actualBaseRoot != expectedBaseRoot {
		return nil, fmt.Errorf("base checkpoint %d root mismatch: expected %x, got %x",
			baseHeight, expectedBaseRoot, actualBaseRoot)
	}

	log.Info(log.SDB, "Rebuilding checkpoint",
		"height", height,
		"from", baseHeight,
		"deltas", height-baseHeight)

	// Replay deltas from base to target
	tree, finalRoot, err := cm.replayRange(baseTreeCopy, baseHeight, height)
	if err != nil {
		return nil, fmt.Errorf("failed to rebuild checkpoint %d: %w", height, err)
	}

	// Verify rebuilt tree root matches canonical block root
	block, err := cm.blockLoader(height)
	if err != nil {
		return nil, fmt.Errorf("failed to load block %d for verification: %w", height, err)
	}
	expectedRoot := common.BytesToHash(block.UBTRoot[:])

	if finalRoot != expectedRoot {
		return nil, fmt.Errorf("rebuilt checkpoint %d root mismatch: expected %x, got %x",
			height, expectedRoot, finalRoot)
	}

	// Cache verified checkpoint
	cm.trees.Add(height, tree)

	log.Info(log.SDB, "Rebuilt checkpoint",
		"height", height,
		"from", baseHeight,
		"deltas", height-baseHeight,
		"root", finalRoot.Hex())

	return tree, nil
}

// findNearestCheckpoint finds highest checkpoint < height (must hold lock)
func (cm *CheckpointTreeManager) findNearestCheckpoint(height uint64) (uint64, *UnifiedBinaryTree) {
	best := uint64(0)
	var bestTree *UnifiedBinaryTree

	// Check pinned checkpoints first (genesis)
	for h, tree := range cm.pinned {
		if h < height && h >= best {
			best = h
			bestTree = tree
		}
	}

	// Check LRU cache
	for _, h := range cm.trees.Keys() {
		if h < height && h > best {
			if tree, ok := cm.trees.Peek(h); ok {
				best = h
				bestTree = tree
			}
		}
	}

	return best, bestTree
}

// replayRange replays blocks from baseHeight+1 to targetHeight using delta replay
func (cm *CheckpointTreeManager) replayRange(
	baseTree *UnifiedBinaryTree,
	baseHeight uint64,
	targetHeight uint64,
) (*UnifiedBinaryTree, common.Hash, error) {
	currentTree := baseTree
	var finalRoot common.Hash

	for h := baseHeight + 1; h <= targetHeight; h++ {
		block, err := cm.blockLoader(h)
		if err != nil {
			return nil, common.Hash{}, fmt.Errorf("failed to load block %d: %w", h, err)
		}

		expectedRoot := common.BytesToHash(block.UBTRoot[:])

		// Check if block has delta
		if block.UBTStateDelta != nil && block.UBTStateDelta.NumEntries > 0 {
			replayedTree, root, err := ApplyStateDelta(currentTree, block.UBTStateDelta)
			if err != nil {
				return nil, common.Hash{}, fmt.Errorf("delta replay failed at block %d: %w", h, err)
			}
			actualRoot := common.BytesToHash(root[:])
			if actualRoot != expectedRoot {
				return nil, common.Hash{}, fmt.Errorf("delta root mismatch at block %d: expected %x, got %x", h, expectedRoot, actualRoot)
			}
			currentTree = replayedTree
			finalRoot = actualRoot

			log.Trace(log.SDB, "Replayed delta",
				"height", h,
				"entries", block.UBTStateDelta.NumEntries,
				"root", expectedRoot.Hex())
		} else {
			// No delta in block - this shouldn't happen in normal operation
			// Fall back to just using the expected root without verification
			log.Warn(log.SDB, "Block missing state delta",
				"height", h,
				"root", expectedRoot.Hex())

			// Cannot replay without delta - tree may be incorrect
			// Return error to force use of snapshot or different checkpoint
			return nil, common.Hash{}, fmt.Errorf("block %d missing state delta (cannot rebuild checkpoint)", h)
		}

		// Log progress every 100 blocks
		if (h-baseHeight)%100 == 0 {
			log.Debug(log.SDB, "Replay progress",
				"height", h,
				"remaining", targetHeight-h,
				"currentRoot", finalRoot.Hex())
		}
	}

	return currentTree, finalRoot, nil
}

// AddCheckpoint stores a checkpoint tree in LRU cache
func (cm *CheckpointTreeManager) AddCheckpoint(height uint64, tree *UnifiedBinaryTree) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.trees.Add(height, tree)

	log.Debug(log.SDB, "Added checkpoint to LRU",
		"height", height)
}

// PinCheckpoint pins a checkpoint so it's never evicted (e.g., genesis, snapshots)
func (cm *CheckpointTreeManager) PinCheckpoint(height uint64, tree *UnifiedBinaryTree) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.pinned[height] = tree

	root := tree.RootHash()
	log.Info(log.SDB, "Pinned checkpoint",
		"height", height,
		"root", common.BytesToHash(root[:]).Hex())
}

// UnpinCheckpoint removes a checkpoint from pinned set (use with caution)
func (cm *CheckpointTreeManager) UnpinCheckpoint(height uint64) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	delete(cm.pinned, height)

	log.Info(log.SDB, "Unpinned checkpoint", "height", height)
}

// ShouldCreateCheckpoint determines if checkpoint needed at height
func (cm *CheckpointTreeManager) ShouldCreateCheckpoint(height uint64) bool {
	if height == 0 {
		return true // Genesis
	}
	if height%cm.coarsePeriod == 0 {
		return true // Coarse checkpoint (e.g., every 7200 blocks)
	}
	if height%cm.finePeriod == 0 {
		return true // Fine checkpoint (e.g., every 600 blocks)
	}
	return false
}

// ListCheckpoints returns all checkpoint heights (pinned + cached)
func (cm *CheckpointTreeManager) ListCheckpoints() []uint64 {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	heights := make([]uint64, 0, len(cm.pinned)+cm.trees.Len())

	// Add pinned checkpoints
	for h := range cm.pinned {
		heights = append(heights, h)
	}

	// Add LRU cached checkpoints
	for _, h := range cm.trees.Keys() {
		heights = append(heights, h)
	}

	return heights
}

// IsPinned returns whether a checkpoint is pinned
func (cm *CheckpointTreeManager) IsPinned(height uint64) bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	_, ok := cm.pinned[height]
	return ok
}

// Stats returns checkpoint manager statistics
func (cm *CheckpointTreeManager) Stats() map[string]interface{} {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	return map[string]interface{}{
		"pinned_count": len(cm.pinned),
		"lru_count":    cm.trees.Len(),
		"total_count":  len(cm.pinned) + cm.trees.Len(),
	}
}
