package verkle

import (
	"fmt"

	"github.com/colorfulnotion/jam/common"
	"github.com/ethereum/go-verkle"
)

// replayVerkleStateDelta applies a delta to a tree and verifies the resulting root.
// Standalone helper so callers don't need a StateDBStorage instance.
func replayVerkleStateDelta(
	baseTree verkle.VerkleNode,
	delta *VerkleStateDelta,
	expectedRoot common.Hash,
) (verkle.VerkleNode, error) {
	if baseTree == nil {
		return nil, fmt.Errorf("base tree is nil")
	}

	// Validate delta structure
	if delta.NumEntries*64 != uint32(len(delta.Entries)) {
		return nil, fmt.Errorf("invalid delta: entries length mismatch (num_entries=%d, len=%d)",
			delta.NumEntries, len(delta.Entries))
	}

	// Work on a copy to avoid mutating the caller's tree
	tree := baseTree.Copy()

	// Apply all writes
	for i := uint32(0); i < delta.NumEntries; i++ {
		offset := i * 64
		key := delta.Entries[offset : offset+32]
		value := delta.Entries[offset+32 : offset+64]

		// Insert key-value pair into tree
		if err := tree.Insert(key, value, nil); err != nil {
			return nil, fmt.Errorf("failed to insert key %x at index %d: %w", key, i, err)
		}
	}

	// Compute root after all inserts
	tree.Commit()
	hashBytes := tree.Hash().Bytes()
	actualRoot := common.BytesToHash(hashBytes[:])

	// Verify root matches expected
	if actualRoot != expectedRoot {
		return nil, fmt.Errorf("root mismatch after replay: expected %x, got %x",
			expectedRoot, actualRoot)
	}

	return tree, nil
}

// ReplayVerkleStateDelta is a thin wrapper for compatibility with existing callers.
func ReplayVerkleStateDelta(
	baseTree verkle.VerkleNode,
	delta *VerkleStateDelta,
	expectedRoot common.Hash,
) (verkle.VerkleNode, error) {
	return replayVerkleStateDelta(baseTree, delta, expectedRoot)
}

// ReplayBlockSequence applies deltas from blocks startHeight+1 to endHeight
// This is used for checkpoint rebuilding and cold start
func ReplayBlockSequence(
	baseTree verkle.VerkleNode,
	startHeight uint64,
	endHeight uint64,
	getBlockFunc func(uint64) (*VerkleStateDelta, common.Hash, error),
) (verkle.VerkleNode, common.Hash, error) {
	if baseTree == nil {
		return nil, common.Hash{}, fmt.Errorf("base tree is nil")
	}

	// Start from a copy of the provided base tree (e.g., checkpoint at startHeight)
	tree := baseTree.Copy()
	var finalRoot common.Hash

	// Replay each block's delta
	for h := startHeight + 1; h <= endHeight; h++ {
		delta, expectedRoot, err := getBlockFunc(h)
		if err != nil {
			return nil, common.Hash{}, fmt.Errorf("failed to load block %d: %w", h, err)
		}

		// Apply delta with root verification
		newTree, err := replayVerkleStateDelta(tree, delta, expectedRoot)
		if err != nil {
			return nil, common.Hash{}, fmt.Errorf("replay failed at block %d: %w", h, err)
		}

		tree = newTree
		finalRoot = expectedRoot
	}

	return tree, finalRoot, nil
}
