package node

import (
	"fmt"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

// -----Custom methods for tiny QUIC EC experiment-----
func (n *Node) processNPECChunk(chunk types.DistributeECChunk) error {
	// Serialize the chunk
	serialized, err := types.Encode(types.ECChunkResponse{
		SegmentRoot: chunk.SegmentRoot,
		Data:        chunk.Data,
	})
	if err != nil {
		return err
	}

	key := fmt.Sprintf("DA-%v", common.Hash(chunk.SegmentRoot))
	fmt.Printf("ECChunkReceive Store key %s | val %x\n", key, serialized)

	// Save the chunk to the local storage
	err = n.WriteRawKV(key, serialized)
	if err != nil {
		return err
	}
	err = n.store.WriteKV(common.Hash(chunk.RootHash), chunk.BlobMeta)
	if err != nil {
		return err
	}

	fmt.Printf(" -- [N%d] saved DistributeECChunk common.Hash(chunk.RootHash)=%s\nchunk.BlobMeta=%x\nchunk.SegmentRoot=%s\nchunk.Data=%x\n", n.id, common.Hash(chunk.RootHash), chunk.BlobMeta, common.Hash(chunk.SegmentRoot), chunk.Data)
	return nil
}

func ComputeOrderedNPBundleChunks(ecChunks []types.DistributeECChunk) (bundleShards []types.ConformantECChunk) {
	numNodes := types.TotalValidators
	if len(ecChunks) != numNodes {
		panic("should be exactly numNodes")
	}

	bundleShards = make([]types.ConformantECChunk, numNodes)

	for chunkIdx, ecChunk := range ecChunks {
		shardIdx := uint32(chunkIdx % numNodes)
		bundleShards[shardIdx] = types.ConformantECChunk{Data: ecChunk.Data, ShardIndex: shardIdx}
	}

	return bundleShards
}
