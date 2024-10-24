package node

import (
	"fmt"

	//	"encoding/json"
	//	"encoding/binary"

	//	"github.com/colorfulnotion/jam/pvm"

	"github.com/colorfulnotion/jam/common"
	//	"github.com/colorfulnotion/jam/erasurecoding"
	//	"github.com/colorfulnotion/jam/trie"
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

func ComputeOrderedExportedNPChunks(ecChunksArr [][]types.DistributeECChunk) (segmentsShardsAll [][]types.ConformantECChunk) {
	numNodes := types.TotalValidators

	segmentsShardsAll = make([][]types.ConformantECChunk, numNodes)
	for shardIdx := 0; shardIdx < types.TotalValidators; shardIdx++ {
		segmentsShardsAll[shardIdx] = make([]types.ConformantECChunk, 0)
	}

	// len(ecChunksArr) = len(exportSegment) + ceil(len(exportSegment)/64)
	for segment_item_idx, segment_item := range ecChunksArr {
		if len(segment_item)%types.TotalValidators != 0 {
			panic(fmt.Sprintf("Invalid segment_item_idx:%v Got len=%v pageChunk %v\n", segment_item_idx, len(segment_item), segment_item))
		}
		for chunkIdx, ecChunk := range segment_item {
			shardIdx := uint32(chunkIdx % numNodes)
			single_segment_shard := types.ConformantECChunk{Data: ecChunk.Data, ShardIndex: shardIdx}
			segmentsShardsAll[shardIdx] = append(segmentsShardsAll[shardIdx], single_segment_shard)
		}
	}

	return segmentsShardsAll
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

	/*
		    peerIdentifiers := make([]string, len(ecChunks))
		    requestObjs := make([]interface{}, len(ecChunks))

		    for i, ecChunk := range ecChunks {
		        peerIdx := uint32(i % numNodes)
		        peerIdentifier, err := n.getPeerByIndex(peerIdx)
		        if err != nil {
		            return err
		        }
		        peerIdentifiers[i] = peerIdentifier
		        requestObjs[i] = ecChunk
		    }

			responses, err := n.makeRequests(peerIdentifiers, requestObjs, types.TotalValidators, types.QuicIndividualTimeout,  types.QuicOverallTimeout)
			if err != nil {
				fmt.Printf("[N%v] DistributeEcChunks MakeRequests Errors %v\n", n.id, err)
				return err
			}
			fmt.Printf("DistributeEcChunks resp=%v\n", len(responses))
	*/
	return nil
}
