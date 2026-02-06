package storage

import (
	"fmt"
	"testing"

	evmtypes "github.com/jam-duna/jamduna/types"
	"github.com/jam-duna/jamduna/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock block loader for tests
func mockBlockLoader(blocks map[uint64]*evmtypes.EvmBlockPayload) func(uint64) (*evmtypes.EvmBlockPayload, error) {
	return func(height uint64) (*evmtypes.EvmBlockPayload, error) {
		if block, ok := blocks[height]; ok {
			return block, nil
		}
		return nil, fmt.Errorf("block not found")
	}
}

// Helper to create a test UBT tree with a unique value
func createTestTree(value byte) *UnifiedBinaryTree {
	tree := NewUnifiedBinaryTree(Config{Profile: defaultUBTProfile})
	key := TreeKey{Subindex: value}
	val := [32]byte{}
	val[31] = value
	tree.Insert(key, val)
	return tree
}

func TestCheckpointTreeManager_Creation(t *testing.T) {
	blocks := make(map[uint64]*evmtypes.EvmBlockPayload)
	loader := mockBlockLoader(blocks)

	cm, err := NewCheckpointTreeManager(5, 7200, 600, loader)
	require.NoError(t, err)
	require.NotNil(t, cm)

	assert.Equal(t, uint64(7200), cm.coarsePeriod)
	assert.Equal(t, uint64(600), cm.finePeriod)
}

func TestCheckpointTreeManager_PinCheckpoint(t *testing.T) {
	blocks := make(map[uint64]*evmtypes.EvmBlockPayload)
	loader := mockBlockLoader(blocks)

	cm, err := NewCheckpointTreeManager(5, 7200, 600, loader)
	require.NoError(t, err)

	// Create a test tree
	tree := createTestTree(0x01)

	// Pin at height 0 (genesis)
	cm.PinCheckpoint(0, tree)

	// Verify pinned
	assert.True(t, cm.IsPinned(0))

	// Retrieve pinned checkpoint
	retrieved, err := cm.GetCheckpoint(0)
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	// Verify same tree hash
	assert.Equal(t, tree.RootHash(), retrieved.RootHash())
}

func TestCheckpointTreeManager_AddCheckpoint(t *testing.T) {
	blocks := make(map[uint64]*evmtypes.EvmBlockPayload)
	loader := mockBlockLoader(blocks)

	cm, err := NewCheckpointTreeManager(5, 7200, 600, loader)
	require.NoError(t, err)

	tree1 := createTestTree(0x01)
	tree2 := createTestTree(0x02)

	cm.AddCheckpoint(600, tree1)
	cm.AddCheckpoint(1200, tree2)

	// Retrieve checkpoints
	retrieved1, err := cm.GetCheckpoint(600)
	require.NoError(t, err)
	assert.Equal(t, tree1.RootHash(), retrieved1.RootHash())

	retrieved2, err := cm.GetCheckpoint(1200)
	require.NoError(t, err)
	assert.Equal(t, tree2.RootHash(), retrieved2.RootHash())
}

func TestCheckpointTreeManager_ShouldCreateCheckpoint(t *testing.T) {
	blocks := make(map[uint64]*evmtypes.EvmBlockPayload)
	loader := mockBlockLoader(blocks)

	cm, err := NewCheckpointTreeManager(5, 7200, 600, loader)
	require.NoError(t, err)

	testCases := []struct {
		height   uint64
		expected bool
		reason   string
	}{
		{0, true, "genesis"},
		{600, true, "fine checkpoint"},
		{1200, true, "fine checkpoint"},
		{7200, true, "coarse checkpoint (also fine)"},
		{14400, true, "coarse checkpoint"},
		{599, false, "not a checkpoint"},
		{601, false, "not a checkpoint"},
		{100, false, "not a checkpoint"},
	}

	for _, tc := range testCases {
		result := cm.ShouldCreateCheckpoint(tc.height)
		assert.Equal(t, tc.expected, result, "Height %d (%s)", tc.height, tc.reason)
	}
}

func TestCheckpointTreeManager_LRUEviction(t *testing.T) {
	blocks := make(map[uint64]*evmtypes.EvmBlockPayload)
	loader := mockBlockLoader(blocks)

	// Create manager with small cache (max 3 checkpoints)
	cm, err := NewCheckpointTreeManager(3, 7200, 600, loader)
	require.NoError(t, err)

	// Add 5 checkpoints (should evict 2 oldest)
	for i := uint64(1); i <= 5; i++ {
		tree := createTestTree(byte(i))
		cm.AddCheckpoint(i*600, tree)
	}

	// First two should be evicted
	_, err1 := cm.GetCheckpoint(600)
	assert.Error(t, err1, "First checkpoint should be evicted")

	_, err2 := cm.GetCheckpoint(1200)
	assert.Error(t, err2, "Second checkpoint should be evicted")

	// Last three should still exist
	for i := uint64(3); i <= 5; i++ {
		_, err := cm.GetCheckpoint(i * 600)
		assert.NoError(t, err, "Checkpoint %d should still exist", i*600)
	}
}

// TestGenesisNeverEvicted verifies that pinned genesis survives LRU evictions
func TestGenesisNeverEvicted(t *testing.T) {
	blocks := make(map[uint64]*evmtypes.EvmBlockPayload)
	loader := mockBlockLoader(blocks)

	// Create small LRU cache (max 3 checkpoints)
	cm, err := NewCheckpointTreeManager(3, 7200, 600, loader)
	require.NoError(t, err)

	// Create and pin genesis
	genesisTree := createTestTree(0x00)
	cm.PinCheckpoint(0, genesisTree)

	// Add many checkpoints to trigger evictions
	for i := uint64(1); i <= 10; i++ {
		tree := createTestTree(byte(i))
		cm.AddCheckpoint(i*600, tree)
	}

	// Genesis should still be accessible (pinned)
	retrievedGenesis, err := cm.GetCheckpoint(0)
	require.NoError(t, err)
	assert.Equal(t, genesisTree.RootHash(), retrievedGenesis.RootHash())
	assert.True(t, cm.IsPinned(0), "Genesis should still be pinned")

	t.Log("✅ Genesis survived LRU eviction of 10 checkpoints (max cache size = 3)")
}

// TestCheckpointRebuildVerification verifies that rebuilt checkpoints match canonical roots
func TestCheckpointRebuildVerification(t *testing.T) {
	// Create genesis tree
	genesisTree := createTestTree(0x00)
	genesisHashBytes := genesisTree.RootHash()
	genesisRoot := common.BytesToHash(genesisHashBytes[:])

	// Create block 1 state by applying single delta from genesis
	key1 := TreeKey{Subindex: 0x01}
	val1 := [32]byte{}
	val1[31] = 0x11

	tree1 := genesisTree.Copy()
	tree1.Insert(key1, val1)
	hashBytes1 := tree1.RootHash()
	root1 := common.BytesToHash(hashBytes1[:])

	// Create delta for block 1
	key1Bytes := key1.ToBytes()
	delta1 := &evmtypes.UBTStateDelta{
		NumEntries: 1,
		Entries:    make([]byte, 64),
	}
	copy(delta1.Entries[0:32], key1Bytes[:])
	copy(delta1.Entries[32:64], val1[:])

	// Mock blocks with correct UBT roots and deltas
	blocks := map[uint64]*evmtypes.EvmBlockPayload{
		0: {
			Number:  0,
			UBTRoot: genesisRoot,
		},
		1: {
			Number:        1,
			UBTRoot:       root1,
			UBTStateDelta: delta1,
		},
	}
	loader := mockBlockLoader(blocks)

	cm, err := NewCheckpointTreeManager(5, 7200, 600, loader)
	require.NoError(t, err)

	// Pin genesis
	cm.PinCheckpoint(0, genesisTree)

	// Test 1: Retrieve genesis (should work immediately)
	retrievedGenesis, err := cm.GetCheckpoint(0)
	require.NoError(t, err)
	retrievedGenesisHashBytes := retrievedGenesis.RootHash()
	assert.Equal(t, genesisRoot, common.BytesToHash(retrievedGenesisHashBytes[:]))

	t.Log("✅ Genesis checkpoint verified")

	// Test 2: Rebuild checkpoint at block 1 from genesis using delta replay
	retrieved1, err := cm.GetCheckpoint(1)
	require.NoError(t, err, "Checkpoint 1 rebuild should succeed with delta")
	require.NotNil(t, retrieved1)

	// Verify rebuilt tree matches expected root
	retrieved1HashBytes := retrieved1.RootHash()
	retrieved1Root := common.BytesToHash(retrieved1HashBytes[:])
	assert.Equal(t, root1, retrieved1Root, "Rebuilt checkpoint 1 root should match expected")

	// Verify the value is actually in the rebuilt tree
	retrievedVal, found, err := retrieved1.Get(&key1)
	require.NoError(t, err)
	require.True(t, found, "Key should be found in rebuilt tree")
	assert.Equal(t, val1, retrievedVal, "Rebuilt tree should contain the delta change")

	t.Log("✅ Checkpoint 1 rebuilt from genesis using delta replay")
	t.Log("✅ Root verified:", retrieved1Root.Hex())
}

func TestCheckpointTreeManager_FindNearestCheckpoint(t *testing.T) {
	blocks := make(map[uint64]*evmtypes.EvmBlockPayload)
	loader := mockBlockLoader(blocks)

	cm, err := NewCheckpointTreeManager(10, 7200, 600, loader)
	require.NoError(t, err)

	// Pin genesis
	genesisTree := createTestTree(0x00)
	cm.PinCheckpoint(0, genesisTree)

	// Add some checkpoints
	tree600 := createTestTree(0x06)
	tree1200 := createTestTree(0x0C)

	cm.AddCheckpoint(600, tree600)
	cm.AddCheckpoint(1200, tree1200)

	testCases := []struct {
		targetHeight uint64
		expectedBase uint64
	}{
		{100, 0},     // Should use genesis
		{700, 600},   // Should use 600
		{1500, 1200}, // Should use 1200
		{500, 0},     // Should use genesis
	}

	for _, tc := range testCases {
		baseHeight, baseTree := cm.findNearestCheckpoint(tc.targetHeight)
		assert.Equal(t, tc.expectedBase, baseHeight, "Target %d should use base %d", tc.targetHeight, tc.expectedBase)
		assert.NotNil(t, baseTree, "Base tree should not be nil")
	}
}

func TestCheckpointTreeManager_UnpinCheckpoint(t *testing.T) {
	blocks := make(map[uint64]*evmtypes.EvmBlockPayload)
	loader := mockBlockLoader(blocks)

	cm, err := NewCheckpointTreeManager(5, 7200, 600, loader)
	require.NoError(t, err)

	tree := createTestTree(0x01)
	cm.PinCheckpoint(100, tree)

	assert.True(t, cm.IsPinned(100))

	cm.UnpinCheckpoint(100)

	assert.False(t, cm.IsPinned(100))
}

func TestCheckpointTreeManager_ListCheckpoints(t *testing.T) {
	blocks := make(map[uint64]*evmtypes.EvmBlockPayload)
	loader := mockBlockLoader(blocks)

	cm, err := NewCheckpointTreeManager(10, 7200, 600, loader)
	require.NoError(t, err)

	// Pin genesis
	cm.PinCheckpoint(0, createTestTree(0x00))

	// Add some checkpoints
	cm.AddCheckpoint(600, createTestTree(0x01))
	cm.AddCheckpoint(1200, createTestTree(0x02))

	heights := cm.ListCheckpoints()

	// Should have at least genesis (pinned checkpoints may not show in LRU list)
	assert.True(t, len(heights) >= 1, "Should have at least one checkpoint")

	// Genesis should be accessible
	_, err = cm.GetCheckpoint(0)
	assert.NoError(t, err)
}

func TestCheckpointTreeManager_Stats(t *testing.T) {
	blocks := make(map[uint64]*evmtypes.EvmBlockPayload)
	loader := mockBlockLoader(blocks)

	cm, err := NewCheckpointTreeManager(5, 7200, 600, loader)
	require.NoError(t, err)

	// Initially should have 0 checkpoints
	stats := cm.Stats()
	assert.Equal(t, 0, stats["lru_count"])
	assert.Equal(t, 0, stats["pinned_count"])

	// Pin genesis
	cm.PinCheckpoint(0, createTestTree(0x00))

	// Add some checkpoints
	cm.AddCheckpoint(600, createTestTree(0x01))

	stats = cm.Stats()
	assert.Equal(t, 1, stats["lru_count"])
	assert.Equal(t, 1, stats["pinned_count"])
}
