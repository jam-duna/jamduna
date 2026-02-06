// +build ignore

// NOTE: This file is disabled because it uses verkle trees (not UBT) and the
// evmverkle package has no source files, only test files. The ExportSnapshot,
// LoadFromSnapshot, and ReplayToHead methods expect *UnifiedBinaryTree but
// this test uses verkle.VerkleNode. Re-enable when verkle support is restored.

package storage

import (
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	evmtypes "github.com/jam-duna/jamduna/types"
	// evmverkle "github.com/jam-duna/jamduna/builder/evm/verkle"
	"github.com/jam-duna/jamduna/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to create snapshot file
// func createSnapshotFile(path string, delta *evmverkle.VerkleStateDelta) error {
// 	f, err := os.Create(path)
// 	if err != nil {
// 		return err
// 	}
// 	defer f.Close()
//
// 	gz := gzip.NewWriter(f)
// 	defer gz.Close()
//
// 	_, err = gz.Write(delta.Serialize())
// 	return err
// }

// TestSnapshotIntegrityRejection verifies corrupted snapshots are rejected
func TestSnapshotIntegrityRejection(t *testing.T) {
	// Create a tree with data and export snapshot
	tree := createTestTree(0x00)
	tree.Commit()
	hashBytes := tree.Hash().Bytes()
	correctRoot := common.BytesToHash(hashBytes[:])

	storage := &StorageHub{}
	tmpDir := t.TempDir()
	snapshotPath := filepath.Join(tmpDir, "snapshot.gz")
	err := storage.ExportSnapshot(tree, 100, snapshotPath)
	require.NoError(t, err)

	// Try to load with WRONG root (off by 1)
	wrongRoot := correctRoot
	wrongRoot[31] = correctRoot[31] ^ 0x01

	_, err = storage.LoadFromSnapshot(snapshotPath, 100, wrongRoot)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "snapshot root mismatch", "Should reject corrupted snapshot")

	// Load with correct root should succeed
	loadedTree, err := storage.LoadFromSnapshot(snapshotPath, 100, correctRoot)
	require.NoError(t, err)
	loadedHashBytes := loadedTree.Hash().Bytes()
	loadedHash := common.BytesToHash(loadedHashBytes[:])
	assert.Equal(t, correctRoot, loadedHash, "Loaded snapshot should match original root")

	t.Log("✅ Corrupted snapshot rejected; valid snapshot loads correctly")
}

// TestSnapshotRoundtrip verifies export+load roundtrip preserves state
func TestSnapshotRoundtrip(t *testing.T) {
	// Create tree with some data
	tree := createTestTree(0x00)

	// Add additional keys
	key1 := make([]byte, 32)
	val1 := make([]byte, 32)
	key1[31] = 0x11
	val1[31] = 0xAA
	tree.Insert(key1, val1, nil)

	tree.Commit()
	origHashBytes := tree.Hash().Bytes()
	origRoot := common.BytesToHash(origHashBytes[:])

	// Export snapshot
	storage := &StorageHub{}
	tmpDir := t.TempDir()
	snapshotPath := filepath.Join(tmpDir, "snapshot_roundtrip.gz")

	err := storage.ExportSnapshot(tree, 200, snapshotPath)
	require.NoError(t, err, "ExportSnapshot should succeed with traversal")

	// Load snapshot
	loadedTree, err := storage.LoadFromSnapshot(snapshotPath, 200, origRoot)
	require.NoError(t, err)

	// Verify roots match
	loadedHashBytes := loadedTree.Hash().Bytes()
	loadedRoot := common.BytesToHash(loadedHashBytes[:])
	assert.Equal(t, origRoot, loadedRoot, "Snapshot roundtrip should preserve root")

	// Verify data preserved
	got, err := loadedTree.Get(key1, nil)
	require.NoError(t, err)
	assert.Equal(t, val1, got, "Snapshot should preserve key/value data")

	t.Log("✅ Snapshot export/load roundtrip successful")
}

// TestReplayToHead verifies delta replay from snapshot works
func TestReplayToHead(t *testing.T) {
	// Create snapshot tree (block 100)
	snapshotTree := createTestTree(0x00)
	snapshotHashBytes := snapshotTree.Hash().Bytes()
	snapshotRoot := common.BytesToHash(snapshotHashBytes[:])

	// Create blocks 101-103 with deltas
	blocks := map[uint64]*evmtypes.EvmBlockPayload{
		100: {
			Number:     100,
			VerkleRoot: snapshotRoot,
		},
	}

	currentTree := snapshotTree
	for h := uint64(101); h <= 103; h++ {
		key := make([]byte, 32)
		val := make([]byte, 32)
		key[31] = byte(h)
		val[31] = byte(h * 10)

		newTree := currentTree.Copy()
		newTree.Insert(key, val, nil)
		newTree.Commit()
		hashBytes := newTree.Hash().Bytes()
		root := common.BytesToHash(hashBytes[:])

		delta := &evmtypes.VerkleStateDelta{
			NumEntries: 1,
			Entries:    make([]byte, 64),
		}
		copy(delta.Entries[0:32], key)
		copy(delta.Entries[32:64], val)

		blocks[h] = &evmtypes.EvmBlockPayload{
			Number:           uint32(h),
			VerkleRoot:       root,
			VerkleStateDelta: delta,
		}

		currentTree = newTree
	}

	loader := mockBlockLoader(blocks)

	// Replay from snapshot (100) to head (103)
	storage := &StorageHub{}
	replayedTree, err := storage.ReplayToHead(snapshotTree, 100, 103, loader)
	require.NoError(t, err)
	require.NotNil(t, replayedTree)

	// Verify replayed tree matches expected head root
	replayedHashBytes := replayedTree.Hash().Bytes()
	replayedRoot := common.BytesToHash(replayedHashBytes[:])

	expectedRoot := common.BytesToHash(blocks[103].VerkleRoot[:])
	assert.Equal(t, expectedRoot, replayedRoot, "Replayed tree root should match head")

	// Verify snapshot tree unchanged (replay used copy)
	snapshotAfterHashBytes := snapshotTree.Hash().Bytes()
	snapshotAfterRoot := common.BytesToHash(snapshotAfterHashBytes[:])
	assert.Equal(t, snapshotRoot, snapshotAfterRoot, "Snapshot tree should be unchanged")

	t.Log("✅ Replay from snapshot to head successful")
	t.Log("✅ Snapshot tree unchanged (replay used copy)")
}

// TestReplayToHeadNoReplayNeeded verifies snapshot at head needs no replay
func TestReplayToHeadNoReplayNeeded(t *testing.T) {
	snapshotTree := createTestTree(0x00)
	snapshotHashBytes := snapshotTree.Hash().Bytes()
	snapshotRoot := common.BytesToHash(snapshotHashBytes[:])

	blocks := map[uint64]*evmtypes.EvmBlockPayload{
		100: {Number: 100, VerkleRoot: snapshotRoot},
	}
	loader := mockBlockLoader(blocks)

	storage := &StorageHub{}
	replayedTree, err := storage.ReplayToHead(snapshotTree, 100, 100, loader)
	require.NoError(t, err)

	// Should return same tree (no replay needed)
	replayedHashBytes := replayedTree.Hash().Bytes()
	replayedRoot := common.BytesToHash(replayedHashBytes[:])
	assert.Equal(t, snapshotRoot, replayedRoot, "No replay needed when snapshot == head")

	t.Log("✅ No replay needed when snapshot height == head height")
}

// TestReplayToHeadMissingDelta verifies error when block lacks delta
func TestReplayToHeadMissingDelta(t *testing.T) {
	snapshotTree := createTestTree(0x00)
	snapshotHashBytes := snapshotTree.Hash().Bytes()
	snapshotRoot := common.BytesToHash(snapshotHashBytes[:])

	blocks := map[uint64]*evmtypes.EvmBlockPayload{
		100: {Number: 100, VerkleRoot: snapshotRoot},
		101: {Number: 101, VerkleRoot: snapshotRoot, VerkleStateDelta: nil}, // Missing delta
	}
	loader := mockBlockLoader(blocks)

	storage := &StorageHub{}
	_, err := storage.ReplayToHead(snapshotTree, 100, 101, loader)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing verkle delta", "Should error on missing delta")

	t.Log("✅ Missing delta correctly rejected during replay")
}

// TestColdStart verifies full cold start process
func TestColdStart(t *testing.T) {
	storage := &StorageHub{}
	tmpDir := t.TempDir()
	snapshotPath := filepath.Join(tmpDir, "cold_start.gz")

	// Create snapshot tree at height 100
	snapshotTree := createTestTree(0x00)
	snapshotTree.Commit()
	snapshotHashBytes := snapshotTree.Hash().Bytes()
	snapshotRoot := common.BytesToHash(snapshotHashBytes[:])

	// Export snapshot
	err := storage.ExportSnapshot(snapshotTree, 100, snapshotPath)
	require.NoError(t, err)

	// Create blocks 101-105 with deltas
	blocks := map[uint64]*evmtypes.EvmBlockPayload{
		100: {Number: 100, VerkleRoot: snapshotRoot},
	}

	currentTree := snapshotTree
	for h := uint64(101); h <= 105; h++ {
		key := make([]byte, 32)
		val := make([]byte, 32)
		key[31] = byte(h)
		val[31] = byte(h * 10)

		newTree := currentTree.Copy()
		newTree.Insert(key, val, nil)
		newTree.Commit()
		hashBytes := newTree.Hash().Bytes()
		root := common.BytesToHash(hashBytes[:])

		delta := &evmtypes.VerkleStateDelta{
			NumEntries: 1,
			Entries:    make([]byte, 64),
		}
		copy(delta.Entries[0:32], key)
		copy(delta.Entries[32:64], val)

		blocks[h] = &evmtypes.EvmBlockPayload{
			Number:           uint32(h),
			VerkleRoot:       root,
			VerkleStateDelta: delta,
		}

		currentTree = newTree
	}

	loader := mockBlockLoader(blocks)

	// Create checkpoint manager
	cm, err := NewCheckpointTreeManager(10, 7200, 600, loader)
	require.NoError(t, err)

	// Perform cold start
	err = storage.ColdStart(snapshotPath, 100, snapshotRoot, 105, loader, cm)
	require.NoError(t, err)

	// Verify snapshot was pinned
	assert.True(t, cm.IsPinned(100), "Snapshot should be pinned")

	// Verify current tree matches head
	storage.mutex.Lock()
	currentTreeAfterColdStart := storage.CurrentVerkleTree
	storage.mutex.Unlock()

	currentHashBytes := currentTreeAfterColdStart.Hash().Bytes()
	currentRoot := common.BytesToHash(currentHashBytes[:])

	expectedRoot := common.BytesToHash(blocks[105].VerkleRoot[:])
	assert.Equal(t, expectedRoot, currentRoot, "Current tree should match head after cold start")

	// Verify snapshot checkpoint exists
	snapshotCheckpoint, err := cm.GetCheckpoint(100)
	require.NoError(t, err)
	snapshotCheckpointHashBytes := snapshotCheckpoint.Hash().Bytes()
	snapshotCheckpointRoot := common.BytesToHash(snapshotCheckpointHashBytes[:])
	assert.Equal(t, snapshotRoot, snapshotCheckpointRoot, "Snapshot checkpoint should be accessible")

	t.Log("✅ Cold start complete: snapshot → replay → current tree")
	t.Log("✅ Snapshot pinned as checkpoint")
	t.Log("✅ Current tree matches head")
}

// TestColdStartNoReplay verifies cold start when snapshot == head
func TestColdStartNoReplay(t *testing.T) {
	// Use snapshot with data at head height
	snapshotTree := createTestTree(0x00)
	snapshotTree.Commit()
	snapshotHashBytes := snapshotTree.Hash().Bytes()
	snapshotRoot := common.BytesToHash(snapshotHashBytes[:])

	storage := &StorageHub{}
	tmpDir := t.TempDir()
	snapshotPath := filepath.Join(tmpDir, "cold_start_no_replay.gz")

	// Export snapshot
	err := storage.ExportSnapshot(snapshotTree, 100, snapshotPath)
	require.NoError(t, err)

	blocks := map[uint64]*evmtypes.EvmBlockPayload{
		100: {Number: 100, VerkleRoot: snapshotRoot},
	}
	loader := mockBlockLoader(blocks)

	cm, err := NewCheckpointTreeManager(10, 7200, 600, loader)
	require.NoError(t, err)

	// Cold start with snapshot == head
	err = storage.ColdStart(snapshotPath, 100, snapshotRoot, 100, loader, cm)
	require.NoError(t, err)

	// Verify current tree set correctly
	storage.mutex.Lock()
	currentTree := storage.CurrentVerkleTree
	storage.mutex.Unlock()

	currentHashBytes := currentTree.Hash().Bytes()
	currentRoot := common.BytesToHash(currentHashBytes[:])
	assert.Equal(t, snapshotRoot, currentRoot, "Current tree should match snapshot when no replay")

	t.Log("✅ Cold start with no replay successful")
}
