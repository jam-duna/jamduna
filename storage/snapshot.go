package storage

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb/evmtypes"
	evmverkle "github.com/colorfulnotion/jam/builder/evm/verkle"
	"github.com/ethereum/go-verkle"
)

// SnapshotMetadata contains snapshot file metadata
type SnapshotMetadata struct {
	Height      uint64
	Root        common.Hash
	NumEntries  uint32
	Timestamp   uint64
	Description string
}

// LoadFromSnapshot initializes verkle tree from compressed key-value snapshot
func (s *StateDBStorage) LoadFromSnapshot(
	snapshotPath string,
	snapshotHeight uint64,
	expectedRoot common.Hash,
) (verkle.VerkleNode, error) {
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

	// 3. Parse as VerkleStateDelta (snapshot is just full state as delta)
	snapshot := &evmverkle.VerkleStateDelta{}
	if err := snapshot.Deserialize(snapshotData); err != nil {
		return nil, fmt.Errorf("failed to deserialize snapshot: %w", err)
	}

	// 4. Build tree from snapshot
	tree := verkle.New()
	for i := uint32(0); i < snapshot.NumEntries; i++ {
		offset := i * 64
		key := snapshot.Entries[offset : offset+32]
		value := snapshot.Entries[offset+32 : offset+64]

		if err := tree.Insert(key, value, nil); err != nil {
			return nil, fmt.Errorf("snapshot insert failed at entry %d: %w", i, err)
		}
	}

	tree.Commit()
	hashBytes := tree.Hash().Bytes()
	actualRoot := common.BytesToHash(hashBytes[:])

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
func (s *StateDBStorage) ReplayToHead(
	snapshotTree verkle.VerkleNode,
	snapshotHeight uint64,
	headHeight uint64,
	blockLoader func(uint64) (*evmtypes.EvmBlockPayload, error),
) (verkle.VerkleNode, error) {
	if headHeight < snapshotHeight {
		return nil, fmt.Errorf("head height %d < snapshot height %d", headHeight, snapshotHeight)
	}

	if headHeight == snapshotHeight {
		// No replay needed
		return snapshotTree, nil
	}

	// CRITICAL: Copy snapshot tree to avoid mutating pinned checkpoint
	currentTree := snapshotTree.Copy()

	// Replay each block from snapshot+1 to head
	for h := snapshotHeight + 1; h <= headHeight; h++ {
		block, err := blockLoader(h)
		if err != nil {
			return nil, fmt.Errorf("failed to load block %d: %w", h, err)
		}

		if block.VerkleStateDelta == nil || block.VerkleStateDelta.NumEntries == 0 {
			return nil, fmt.Errorf("block %d missing verkle delta", h)
		}

		// Convert evmverkle.VerkleStateDelta to storage.VerkleStateDelta
		storageDelta := &evmverkle.VerkleStateDelta{
			NumEntries: block.VerkleStateDelta.NumEntries,
			Entries:    block.VerkleStateDelta.Entries,
		}

		expectedRoot := common.BytesToHash(block.VerkleRoot[:])
		// Use standalone helper instead of dummy StateDBStorage{}
		newTree, err := evmverkle.ReplayVerkleStateDelta(currentTree, storageDelta, expectedRoot)
		if err != nil {
			return nil, fmt.Errorf("replay failed at block %d: %w", h, err)
		}

		currentTree = newTree

		if h%100 == 0 {
			log.Info(log.SDB, "Replay progress",
				"height", h,
				"remaining", headHeight-h,
				"percent", (h-snapshotHeight)*100/(headHeight-snapshotHeight))
		}
	}

	// CRITICAL: Verify final tree root matches head block root
	// (Each iteration validates per-block root, but this is a final sanity check)
	headBlock, err := blockLoader(headHeight)
	if err != nil {
		return nil, fmt.Errorf("failed to load head block %d for validation: %w", headHeight, err)
	}
	expectedHeadRoot := common.BytesToHash(headBlock.VerkleRoot[:])
	currentTree.Commit()
	currentHashBytes := currentTree.Hash().Bytes()
	actualHeadRoot := common.BytesToHash(currentHashBytes[:])
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
func (s *StateDBStorage) ExportSnapshot(
	tree verkle.VerkleNode,
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

// extractAllKeysFromTree walks the tree and extracts all key-value pairs
func extractAllKeysFromTree(tree verkle.VerkleNode) (*evmverkle.VerkleStateDelta, error) {
	if tree == nil {
		return nil, fmt.Errorf("tree is nil")
	}

	// Collect all key-value pairs by traversing the tree
	pairs := make(map[[32]byte][]byte)
	if err := walkTree(tree, pairs); err != nil {
		return nil, fmt.Errorf("tree traversal failed: %w", err)
	}

	// Convert map to sorted slice for deterministic output
	keys := make([][32]byte, 0, len(pairs))
	for key := range pairs {
		keys = append(keys, key)
	}

	// Sort by key (deterministic ordering)
	sort.Slice(keys, func(i, j int) bool {
		return bytes.Compare(keys[i][:], keys[j][:]) < 0
	})

	// Build flattened entries
	delta := &evmverkle.VerkleStateDelta{
		NumEntries: uint32(len(keys)),
		Entries:    make([]byte, len(keys)*64),
	}

	for i, key := range keys {
		offset := i * 64
		copy(delta.Entries[offset:offset+32], key[:])
		copy(delta.Entries[offset+32:offset+64], pairs[key])
	}

	return delta, nil
}

// walkTree recursively walks the verkle tree and collects all key-value pairs
func walkTree(node verkle.VerkleNode, pairs map[[32]byte][]byte) error {
	if node == nil {
		return nil
	}

	// Type-assert to check node type
	switch n := node.(type) {
	case *verkle.InternalNode:
		// Internal node: recursively walk children
		children := n.Children()
		for _, child := range children {
			if child != nil {
				if err := walkTree(child, pairs); err != nil {
					return err
				}
			}
		}

	case *verkle.LeafNode:
		// Leaf node: extract all key-value pairs
		// Leaf nodes store up to 256 values (suffix 0-255)
		for i := 0; i < 256; i++ {
			key := n.Key(i)
			if key == nil {
				continue
			}

			value := n.Value(i)
			if value == nil {
				continue
			}

			// Copy to fixed-size array
			var keyArr [32]byte
			copy(keyArr[:], key)

			pairs[keyArr] = value
		}

	default:
		// Unknown node type (Empty, StatelessInternal, etc.)
		// Skip these nodes
		log.Trace(log.SDB, "Skipping unknown node type during tree walk", "type", fmt.Sprintf("%T", n))
	}

	return nil
}

// ColdStart performs full cold start from snapshot file
func (s *StateDBStorage) ColdStart(
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

		// 4. Set as current tree
		s.mutex.Lock()
		s.CurrentVerkleTree = currentTree
		s.mutex.Unlock()

		log.Info(log.SDB, "Cold start complete",
			"snapshot_height", snapshotHeight,
			"head_height", headHeight,
			"blocks_replayed", headHeight-snapshotHeight)
	} else {
		// No replay needed - snapshot is at head
		s.mutex.Lock()
		s.CurrentVerkleTree = snapshotTree.Copy()
		s.mutex.Unlock()

		log.Info(log.SDB, "Cold start complete (no replay needed)",
			"snapshot_height", snapshotHeight)
	}

	return nil
}
