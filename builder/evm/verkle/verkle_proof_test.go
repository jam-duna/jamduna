package verkle

import (
	"strings"
	"testing"
	"time"

	"github.com/colorfulnotion/jam/common"
	evmtypes "github.com/colorfulnotion/jam/builder/evm/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProofServingNoMutation verifies that proof generation doesn't mutate cached checkpoints
func TestProofServingNoMutation(t *testing.T) {
	// Create genesis tree
	genesisTree := createTestTree(0x00)
	genesisHashBytes := genesisTree.Hash().Bytes()
	genesisRoot := common.BytesToHash(genesisHashBytes[:])

	// Create block 1 state
	key1 := make([]byte, 32)
	val1 := make([]byte, 32)
	key1[31] = 0x01
	val1[31] = 0x11

	tree1 := genesisTree.Copy()
	tree1.Insert(key1, val1, nil)
	tree1.Commit()
	hashBytes1 := tree1.Hash().Bytes()
	root1 := common.BytesToHash(hashBytes1[:])

	// Create delta for block 1
	delta1 := &evmtypes.VerkleStateDelta{
		NumEntries: 1,
		Entries:    make([]byte, 64),
	}
	copy(delta1.Entries[0:32], key1)
	copy(delta1.Entries[32:64], val1)

	// Mock blocks
	blocks := map[uint64]*evmtypes.EvmBlockPayload{
		0: {
			Number:     0,
			VerkleRoot: genesisRoot,
		},
		1: {
			Number:           1,
			VerkleRoot:       root1,
			VerkleStateDelta: delta1,
		},
	}
	loader := mockBlockLoader(blocks)

	// Create checkpoint manager
	cm, err := NewCheckpointTreeManager(10, 7200, 600, loader)
	require.NoError(t, err)
	cm.PinCheckpoint(0, genesisTree)

	// Create proof service
	ps := NewProofService(cm, 100, 5*time.Minute)

	// Get genesis checkpoint before proof generation
	genesisBefore, err := cm.GetCheckpoint(0)
	require.NoError(t, err)
	genesisBeforeHashBytes := genesisBefore.Hash().Bytes()
	genesisRootBefore := common.BytesToHash(genesisBeforeHashBytes[:])

	// Generate proof for block 1 (requires replaying delta on genesis copy)
	proofKeys := [][]byte{key1}
	proof, err := ps.GenerateProof(1, proofKeys)
	require.NoError(t, err)
	require.NotNil(t, proof)
	require.Greater(t, len(proof), 0, "Proof should be non-empty")

	// Get genesis checkpoint after proof generation
	genesisAfter, err := cm.GetCheckpoint(0)
	require.NoError(t, err)
	genesisAfterHashBytes := genesisAfter.Hash().Bytes()
	genesisRootAfter := common.BytesToHash(genesisAfterHashBytes[:])

	// CRITICAL: Verify genesis checkpoint wasn't mutated
	assert.Equal(t, genesisRootBefore, genesisRootAfter,
		"Genesis checkpoint root should be unchanged after proof generation")

	// Verify genesis still doesn't contain key1 (proof was generated on copy)
	genesisVal, err := genesisAfter.Get(key1, nil)
	require.NoError(t, err)
	// Should be nil (not val1) since genesis never had this key
	assert.Nil(t, genesisVal,
		"Genesis checkpoint should not contain key from block 1 (proof used copy)")

	t.Log("✅ Genesis checkpoint unchanged after proof generation")
	t.Log("✅ Proof generated on copy, not cached checkpoint")
}

// TestProofCacheHit verifies that identical requests use cached proofs
func TestProofCacheHit(t *testing.T) {
	// Create genesis tree
	genesisTree := createTestTree(0x00)
	genesisHashBytes := genesisTree.Hash().Bytes()
	genesisRoot := common.BytesToHash(genesisHashBytes[:])

	blocks := map[uint64]*evmtypes.EvmBlockPayload{
		0: {
			Number:     0,
			VerkleRoot: genesisRoot,
		},
	}
	loader := mockBlockLoader(blocks)

	cm, err := NewCheckpointTreeManager(10, 7200, 600, loader)
	require.NoError(t, err)
	cm.PinCheckpoint(0, genesisTree)

	ps := NewProofService(cm, 100, 5*time.Minute)

	// Generate proof first time
	key := make([]byte, 32)
	key[31] = 0xAA
	proofKeys := [][]byte{key}

	proof1, err := ps.GenerateProof(0, proofKeys)
	require.NoError(t, err)

	// Generate same proof again - should hit cache
	proof2, err := ps.GenerateProof(0, proofKeys)
	require.NoError(t, err)

	// Proofs should be identical (byte-for-byte)
	assert.Equal(t, proof1, proof2, "Cached proof should be identical")

	// Cache should have 1 entry
	stats := ps.GetCacheStats()
	assert.Equal(t, 1, stats["cache_size"], "Cache should have 1 entry")

	t.Log("✅ Proof cache hit verified")
}

// TestProofCacheMiss verifies that different keys generate different cache entries
func TestProofCacheMiss(t *testing.T) {
	genesisTree := createTestTree(0x00)
	genesisHashBytes := genesisTree.Hash().Bytes()
	genesisRoot := common.BytesToHash(genesisHashBytes[:])

	blocks := map[uint64]*evmtypes.EvmBlockPayload{
		0: {
			Number:     0,
			VerkleRoot: genesisRoot,
		},
	}
	loader := mockBlockLoader(blocks)

	cm, err := NewCheckpointTreeManager(10, 7200, 600, loader)
	require.NoError(t, err)
	cm.PinCheckpoint(0, genesisTree)

	ps := NewProofService(cm, 100, 5*time.Minute)

	// Generate proof for key1
	key1 := make([]byte, 32)
	key1[31] = 0xAA
	_, err = ps.GenerateProof(0, [][]byte{key1})
	require.NoError(t, err)

	// Generate proof for key2 (different key)
	key2 := make([]byte, 32)
	key2[31] = 0xBB
	_, err = ps.GenerateProof(0, [][]byte{key2})
	require.NoError(t, err)

	// Cache should have 2 entries
	stats := ps.GetCacheStats()
	assert.Equal(t, 2, stats["cache_size"], "Cache should have 2 entries for different keys")

	t.Log("✅ Different keys create separate cache entries")
}

// TestProofReplayFromCheckpoint verifies proof generation with delta replay
func TestProofReplayFromCheckpoint(t *testing.T) {
	// Create genesis tree
	genesisTree := createTestTree(0x00)
	genesisHashBytes := genesisTree.Hash().Bytes()
	genesisRoot := common.BytesToHash(genesisHashBytes[:])

	// Create block 2 by applying 2 deltas
	key1 := make([]byte, 32)
	val1 := make([]byte, 32)
	key1[31] = 0x01
	val1[31] = 0x11

	key2 := make([]byte, 32)
	val2 := make([]byte, 32)
	key2[31] = 0x02
	val2[31] = 0x22

	// Block 1: add key1
	tree1 := genesisTree.Copy()
	tree1.Insert(key1, val1, nil)
	tree1.Commit()
	hashBytes1 := tree1.Hash().Bytes()
	root1 := common.BytesToHash(hashBytes1[:])

	delta1 := &evmtypes.VerkleStateDelta{
		NumEntries: 1,
		Entries:    make([]byte, 64),
	}
	copy(delta1.Entries[0:32], key1)
	copy(delta1.Entries[32:64], val1)

	// Block 2: add key2
	tree2 := tree1.Copy()
	tree2.Insert(key2, val2, nil)
	tree2.Commit()
	hashBytes2 := tree2.Hash().Bytes()
	root2 := common.BytesToHash(hashBytes2[:])

	delta2 := &evmtypes.VerkleStateDelta{
		NumEntries: 1,
		Entries:    make([]byte, 64),
	}
	copy(delta2.Entries[0:32], key2)
	copy(delta2.Entries[32:64], val2)

	blocks := map[uint64]*evmtypes.EvmBlockPayload{
		0: {
			Number:     0,
			VerkleRoot: genesisRoot,
		},
		1: {
			Number:           1,
			VerkleRoot:       root1,
			VerkleStateDelta: delta1,
		},
		2: {
			Number:           2,
			VerkleRoot:       root2,
			VerkleStateDelta: delta2,
		},
	}
	loader := mockBlockLoader(blocks)

	cm, err := NewCheckpointTreeManager(10, 7200, 600, loader)
	require.NoError(t, err)
	cm.PinCheckpoint(0, genesisTree)

	ps := NewProofService(cm, 100, 5*time.Minute)

	// Generate proof at block 2 (should replay 2 deltas from genesis)
	proof, err := ps.GenerateProof(2, [][]byte{key2})
	require.NoError(t, err)
	require.NotNil(t, proof)
	require.Greater(t, len(proof), 0)

	// Verify genesis still unchanged (replay happened on copy)
	genesisAfter, err := cm.GetCheckpoint(0)
	require.NoError(t, err)
	genesisAfterHashBytes := genesisAfter.Hash().Bytes()
	genesisRootAfter := common.BytesToHash(genesisAfterHashBytes[:])
	assert.Equal(t, genesisRoot, genesisRootAfter, "Genesis unchanged after multi-block replay")

	t.Log("✅ Proof generated with 2-block delta replay from genesis")
}

// TestProofMissingDelta verifies error when block lacks delta
func TestProofMissingDelta(t *testing.T) {
	genesisTree := createTestTree(0x00)
	genesisHashBytes := genesisTree.Hash().Bytes()
	genesisRoot := common.BytesToHash(genesisHashBytes[:])

	blocks := map[uint64]*evmtypes.EvmBlockPayload{
		0: {
			Number:     0,
			VerkleRoot: genesisRoot,
		},
		1: {
			Number:           1,
			VerkleRoot:       genesisRoot, // No delta
			VerkleStateDelta: nil,
		},
	}
	loader := mockBlockLoader(blocks)

	cm, err := NewCheckpointTreeManager(10, 7200, 600, loader)
	require.NoError(t, err)
	cm.PinCheckpoint(0, genesisTree)

	ps := NewProofService(cm, 100, 5*time.Minute)

	// Should fail - block 1 missing delta
	key := make([]byte, 32)
	_, err = ps.GenerateProof(1, [][]byte{key})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing verkle delta", "Should error on missing delta")

	t.Log("✅ Missing delta correctly rejected")
}

// TestProofRootVerification verifies final root is checked after replay (Finding 1)
func TestProofRootVerification(t *testing.T) {
	genesisTree := createTestTree(0x00)
	genesisHashBytes := genesisTree.Hash().Bytes()
	genesisRoot := common.BytesToHash(genesisHashBytes[:])

	// Create block 1 with WRONG root (delta is correct but root claim is wrong)
	key1 := make([]byte, 32)
	val1 := make([]byte, 32)
	key1[31] = 0x01
	val1[31] = 0x11

	tree1 := genesisTree.Copy()
	tree1.Insert(key1, val1, nil)
	tree1.Commit()
	// Correct root
	hashBytes1 := tree1.Hash().Bytes()
	correctRoot1 := common.BytesToHash(hashBytes1[:])

	// Create delta (correct)
	delta1 := &evmtypes.VerkleStateDelta{
		NumEntries: 1,
		Entries:    make([]byte, 64),
	}
	copy(delta1.Entries[0:32], key1)
	copy(delta1.Entries[32:64], val1)

	// WRONG root (off by 1 in last byte)
	wrongRoot1 := correctRoot1
	wrongRoot1[31] = correctRoot1[31] ^ 0x01

	blocks := map[uint64]*evmtypes.EvmBlockPayload{
		0: {
			Number:     0,
			VerkleRoot: genesisRoot,
		},
		1: {
			Number:           1,
			VerkleRoot:       wrongRoot1, // WRONG ROOT
			VerkleStateDelta: delta1,
		},
	}
	loader := mockBlockLoader(blocks)

	cm, err := NewCheckpointTreeManager(10, 7200, 600, loader)
	require.NoError(t, err)
	cm.PinCheckpoint(0, genesisTree)

	ps := NewProofService(cm, 100, 5*time.Minute)

	// Should fail - root mismatch (caught either in ReplayVerkleStateDelta or final verification)
	key := make([]byte, 32)
	_, err = ps.GenerateProof(1, [][]byte{key})
	require.Error(t, err)
	assert.True(t,
		strings.Contains(err.Error(), "root mismatch") ||
			strings.Contains(err.Error(), "final root mismatch"),
		"Should detect root mismatch: %v", err)

	t.Log("✅ Root verification correctly rejects mismatched root (Finding 1 fixed)")
}

// TestProofCheckpointSelection verifies actual checkpoints are used (Finding 3)
func TestProofCheckpointSelection(t *testing.T) {
	genesisTree := createTestTree(0x00)
	genesisHashBytes := genesisTree.Hash().Bytes()
	genesisRoot := common.BytesToHash(genesisHashBytes[:])

	blocks := map[uint64]*evmtypes.EvmBlockPayload{
		0: {Number: 0, VerkleRoot: genesisRoot},
	}
	loader := mockBlockLoader(blocks)

	// Create checkpoint manager with fine_period=600, coarse_period=7200
	cm, err := NewCheckpointTreeManager(10, 7200, 600, loader)
	require.NoError(t, err)
	cm.PinCheckpoint(0, genesisTree)

	ps := NewProofService(cm, 100, 5*time.Minute)

	// Request proof at height 1200 (would be fine checkpoint if it existed)
	// Should use genesis since 600/1200 checkpoints don't exist
	key := make([]byte, 32)

	// This should fail because block 1 doesn't exist (can't replay from genesis)
	_, err = ps.GenerateProof(1200, [][]byte{key})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load block", "Should fail to load missing block")

	// Verify it selected genesis (only available checkpoint)
	cpHeight, _, err := ps.findNearestCheckpoint(1200)
	require.NoError(t, err)
	assert.Equal(t, uint64(0), cpHeight, "Should select genesis (only available checkpoint)")

	t.Log("✅ Checkpoint selection uses actual availability, not fabricated heights (Finding 3 fixed)")
}
