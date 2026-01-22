package storage

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"sort"

	evmtypes "github.com/colorfulnotion/jam/builder/evm/types"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
)

// SnapshotMetadata contains snapshot file metadata
type SnapshotMetadata struct {
	Height      uint64
	Root        common.Hash
	NumEntries  uint32
	Timestamp   uint64
	Description string
}

// LoadFromSnapshot initializes a UBT tree from a compressed key-value snapshot.
func (s *StorageHub) LoadFromSnapshot(
	snapshotPath string,
	snapshotHeight uint64,
	expectedRoot common.Hash,
) (*UnifiedBinaryTree, error) {
	// 1. Open gzipped snapshot
	f, err := os.Open(snapshotPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open snapshot: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress snapshot: %w", err)
	}
	defer gz.Close()

	// 2. Read snapshot data
	snapshotData, err := io.ReadAll(gz)
	if err != nil {
		return nil, fmt.Errorf("failed to read snapshot: %w", err)
	}

	// 3. Parse as state delta (snapshot is just full state as delta)
	snapshot, err := evmtypes.DeserializeUBTStateDelta(snapshotData)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize snapshot: %w", err)
	}

	// 4. Build tree from snapshot
	tree := NewUnifiedBinaryTree(Config{Profile: defaultUBTProfile})
	for i := uint32(0); i < snapshot.NumEntries; i++ {
		offset := i * 64
		var keyBytes [32]byte
		copy(keyBytes[:], snapshot.Entries[offset:offset+32])
		var value [32]byte
		copy(value[:], snapshot.Entries[offset+32:offset+64])
		tree.Insert(TreeKeyFromBytes(keyBytes), value)
	}

	root := tree.RootHash()
	actualRoot := common.BytesToHash(root[:])

	// 5. CRITICAL: Verify snapshot integrity
	if actualRoot != expectedRoot {
		return nil, fmt.Errorf("snapshot root mismatch: expected %x, got %x (height %d, entries %d)",
			expectedRoot, actualRoot, snapshotHeight, snapshot.NumEntries)
	}

	log.Info(log.SDB, "Loaded snapshot",
		"height", snapshotHeight,
		"entries", snapshot.NumEntries,
		"root", actualRoot.Hex(),
		"path", snapshotPath)

	return tree, nil
}

// ReplayToHead replays deltas from snapshot to current head
func (s *StorageHub) ReplayToHead(
	snapshotTree *UnifiedBinaryTree,
	snapshotHeight uint64,
	headHeight uint64,
	blockLoader func(uint64) (*evmtypes.EvmBlockPayload, error),
) (*UnifiedBinaryTree, error) {
	if headHeight < snapshotHeight {
		return nil, fmt.Errorf("head height %d < snapshot height %d", headHeight, snapshotHeight)
	}

	if headHeight == snapshotHeight {
		// No replay needed
		return snapshotTree, nil
	}

	// Copy snapshot tree to avoid mutating pinned checkpoint.
	currentTree := snapshotTree.Copy()

	// Replay each block from snapshot+1 to head
	for h := snapshotHeight + 1; h <= headHeight; h++ {
		block, err := blockLoader(h)
		if err != nil {
			return nil, fmt.Errorf("failed to load block %d: %w", h, err)
		}

		if block.UBTStateDelta == nil || block.UBTStateDelta.NumEntries == 0 {
			return nil, fmt.Errorf("block %d missing state delta", h)
		}

		expectedRoot := common.BytesToHash(block.UBTRoot[:])
		newTree, root, err := ApplyStateDelta(currentTree, block.UBTStateDelta)
		if err != nil {
			return nil, fmt.Errorf("replay failed at block %d: %w", h, err)
		}
		actualRoot := common.BytesToHash(root[:])
		if actualRoot != expectedRoot {
			return nil, fmt.Errorf("replay root mismatch at block %d: expected %x, got %x", h, expectedRoot, actualRoot)
		}

		currentTree = newTree

		if h%100 == 0 {
			log.Info(log.SDB, "Replay progress",
				"height", h,
				"remaining", headHeight-h,
				"percent", (h-snapshotHeight)*100/(headHeight-snapshotHeight))
		}
	}

	// Verify final tree root matches head block root.
	// Each iteration validates per-block root, but this is a final sanity check.
	headBlock, err := blockLoader(headHeight)
	if err != nil {
		return nil, fmt.Errorf("failed to load head block %d for validation: %w", headHeight, err)
	}
	expectedHeadRoot := common.BytesToHash(headBlock.UBTRoot[:])
	root := currentTree.RootHash()
	actualHeadRoot := common.BytesToHash(root[:])
	if actualHeadRoot != expectedHeadRoot {
		return nil, fmt.Errorf("final root mismatch at head %d: expected %x, got %x",
			headHeight, expectedHeadRoot, actualHeadRoot)
	}

	log.Info(log.SDB, "Replay complete",
		"from", snapshotHeight,
		"to", headHeight,
		"blocks", headHeight-snapshotHeight,
		"final_root", actualHeadRoot.Hex())

	return currentTree, nil
}

// ExportSnapshot creates compressed key-value snapshot from checkpoint
func (s *StorageHub) ExportSnapshot(
	tree *UnifiedBinaryTree,
	height uint64,
	outputPath string,
) error {
	// Extract all key-value pairs from tree
	snapshot, err := extractAllKeysFromTree(tree)
	if err != nil {
		return fmt.Errorf("failed to extract keys from tree: %w", err)
	}

	// Create output file
	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create snapshot file: %w", err)
	}
	defer f.Close()

	// Gzip compress
	gz := gzip.NewWriter(f)
	defer gz.Close()

	// Serialize and write
	snapshotBytes := snapshot.Serialize()
	if _, err := gz.Write(snapshotBytes); err != nil {
		return fmt.Errorf("failed to write snapshot: %w", err)
	}

	log.Info(log.SDB, "Exported snapshot",
		"height", height,
		"entries", snapshot.NumEntries,
		"size_bytes", len(snapshotBytes),
		"path", outputPath)

	return nil
}

// extractAllKeysFromTree walks the tree and extracts all key-value pairs.
func extractAllKeysFromTree(tree *UnifiedBinaryTree) (*evmtypes.UBTStateDelta, error) {
	if tree == nil {
		return nil, fmt.Errorf("tree is nil")
	}

	entries := tree.Iter()
	keys := make([][32]byte, 0, len(entries))
	values := make(map[[32]byte][32]byte, len(entries))
	for _, entry := range entries {
		keyBytes := entry.Key.ToBytes()
		keys = append(keys, keyBytes)
		values[keyBytes] = entry.Value
	}

	// Sort by key (deterministic ordering)
	sort.Slice(keys, func(i, j int) bool {
		return bytes.Compare(keys[i][:], keys[j][:]) < 0
	})

	delta := &evmtypes.UBTStateDelta{
		NumEntries: uint32(len(keys)),
		Entries:    make([]byte, len(keys)*64),
	}

	for i, key := range keys {
		offset := i * 64
		copy(delta.Entries[offset:offset+32], key[:])
		value := values[key]
		copy(delta.Entries[offset+32:offset+64], value[:])
	}

	return delta, nil
}

// ColdStart performs full cold start from snapshot file
func (s *StorageHub) ColdStart(
	snapshotPath string,
	snapshotHeight uint64,
	expectedRoot common.Hash,
	headHeight uint64,
	blockLoader func(uint64) (*evmtypes.EvmBlockPayload, error),
	checkpointMgr *CheckpointTreeManager,
) error {
	// 1. Load snapshot and verify integrity
	snapshotTree, err := s.LoadFromSnapshot(snapshotPath, snapshotHeight, expectedRoot)
	if err != nil {
		return fmt.Errorf("snapshot load failed: %w", err)
	}

	// 2. CRITICAL: Pin snapshot as checkpoint (never evicted)
	checkpointMgr.PinCheckpoint(snapshotHeight, snapshotTree)

	log.Info(log.SDB, "Snapshot pinned as checkpoint", "height", snapshotHeight)

	// 3. Replay deltas from snapshot to head
	if headHeight > snapshotHeight {
		currentTree, err := s.ReplayToHead(snapshotTree, snapshotHeight, headHeight, blockLoader)
		if err != nil {
			return fmt.Errorf("replay to head failed: %w", err)
		}

		// 4. Set as canonical tree (root-first model)
		rootBytes := currentTree.RootHash()
		currentRoot := common.BytesToHash(rootBytes[:])
		s.StoreTreeWithRoot(currentRoot, currentTree)
		if err := s.SetCanonicalRoot(currentRoot); err != nil {
			return fmt.Errorf("failed to set canonical root: %w", err)
		}

		log.Info(log.SDB, "Cold start complete",
			"snapshot_height", snapshotHeight,
			"head_height", headHeight,
			"blocks_replayed", headHeight-snapshotHeight,
			"canonicalRoot", currentRoot.Hex())
	} else {
		// No replay needed - snapshot is at head
		copiedTree := snapshotTree.Copy()
		rootBytes := copiedTree.RootHash()
		snapshotRoot := common.BytesToHash(rootBytes[:])
		s.StoreTreeWithRoot(snapshotRoot, copiedTree)
		if err := s.SetCanonicalRoot(snapshotRoot); err != nil {
			return fmt.Errorf("failed to set canonical root: %w", err)
		}

		log.Info(log.SDB, "Cold start complete (no replay needed)",
			"snapshot_height", snapshotHeight,
			"canonicalRoot", snapshotRoot.Hex())
	}

	return nil
}
