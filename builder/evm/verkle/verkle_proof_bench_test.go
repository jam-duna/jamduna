package verkle

import (
	"testing"
	"time"

	"github.com/colorfulnotion/jam/common"
	evmtypes "github.com/colorfulnotion/jam/builder/evm/types"
	verkle "github.com/ethereum/go-verkle"
)

// Test helpers
func createTestTree(suffix byte) verkle.VerkleNode {
	tree := verkle.New()
	key := make([]byte, 32)
	val := make([]byte, 32)
	key[31] = suffix
	val[31] = suffix
	tree.Insert(key, val, nil)
	tree.Commit()
	return tree
}

func mockBlockLoader(blocks map[uint64]*evmtypes.EvmBlockPayload) func(uint64) (*EvmBlockPayload, error) {
	return func(blockNum uint64) (*EvmBlockPayload, error) {
		block, ok := blocks[blockNum]
		if !ok {
			return nil, nil
		}
		var delta *VerkleStateDelta
		if block.VerkleStateDelta != nil {
			delta = &VerkleStateDelta{
				NumEntries: block.VerkleStateDelta.NumEntries,
				Entries:    block.VerkleStateDelta.Entries,
			}
		}
		return &EvmBlockPayload{
			VerkleRoot:       block.VerkleRoot,
			VerkleStateDelta: delta,
		}, nil
	}
}

// BenchmarkProofCacheHit measures cached proof retrieval latency
func BenchmarkProofCacheHit(b *testing.B) {
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

	cm, _ := NewCheckpointTreeManager(10, 7200, 600, loader)
	cm.PinCheckpoint(0, genesisTree)

	ps := NewProofService(cm, 1000, 10*time.Minute)

	key := make([]byte, 32)
	key[31] = 0xAA
	proofKeys := [][]byte{key}

	// Warm up cache
	ps.GenerateProof(0, proofKeys)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ps.GenerateProof(0, proofKeys)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkProofCacheMiss measures proof generation latency (no replay)
func BenchmarkProofCacheMiss(b *testing.B) {
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

	cm, _ := NewCheckpointTreeManager(10, 7200, 600, loader)
	cm.PinCheckpoint(0, genesisTree)

	ps := NewProofService(cm, 1000, 10*time.Minute)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Use unique key each iteration to avoid cache hits
		key := make([]byte, 32)
		key[28] = byte(i >> 24)
		key[29] = byte(i >> 16)
		key[30] = byte(i >> 8)
		key[31] = byte(i)

		_, err := ps.GenerateProof(0, [][]byte{key})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkProofWithReplay measures proof generation with delta replay
func BenchmarkProofWithReplay(b *testing.B) {
	// Create genesis
	genesisTree := createTestTree(0x00)
	genesisHashBytes := genesisTree.Hash().Bytes()
	genesisRoot := common.BytesToHash(genesisHashBytes[:])

	// Create 10 blocks with deltas
	blocks := map[uint64]*evmtypes.EvmBlockPayload{
		0: {Number: 0, VerkleRoot: genesisRoot},
	}

	currentTree := genesisTree
	for h := uint64(1); h <= 10; h++ {
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
	cm, _ := NewCheckpointTreeManager(10, 7200, 600, loader)
	cm.PinCheckpoint(0, genesisTree)

	ps := NewProofService(cm, 1000, 10*time.Minute)

	key := make([]byte, 32)
	key[31] = 0xFF

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Generate proof at block 10 (requires 10-block replay from genesis)
		_, err := ps.GenerateProof(10, [][]byte{key})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkProofMultipleKeys measures proof generation for multiple keys
func BenchmarkProofMultipleKeys(b *testing.B) {
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

	cm, _ := NewCheckpointTreeManager(10, 7200, 600, loader)
	cm.PinCheckpoint(0, genesisTree)

	ps := NewProofService(cm, 1000, 10*time.Minute)

	// Create 100 keys
	keys := make([][]byte, 100)
	for i := 0; i < 100; i++ {
		key := make([]byte, 32)
		key[30] = byte(i >> 8)
		key[31] = byte(i)
		keys[i] = key
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ps.GenerateProof(0, keys)
		if err != nil {
			b.Fatal(err)
		}
	}
}
