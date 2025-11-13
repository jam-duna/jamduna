package storage

import (
	"fmt"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
)

// GenerateErasureRootShardIdxKey creates a storage key for erasure root and shard index
func GenerateErasureRootShardIdxKey(section string, erasureRoot common.Hash, shardIndex uint16) string {
	return fmt.Sprintf("%s_%v_%d", section, erasureRoot, shardIndex)
}

// SplitHashes splits byte data into 32-byte hash chunks
func SplitHashes(data []byte) []common.Hash {
	var chunks []common.Hash
	for i := 0; i < len(data); i += 32 {
		end := i + 32
		if end > len(data) {
			end = len(data)
		}
		chunks = append(chunks, common.BytesToHash(data[i:end]))
	}
	return chunks
}

// SplitBytes splits data into chunks of size NumECPiecesPerSegment * 2
func SplitBytes(data []byte) [][]byte {
	chunkSize := types.NumECPiecesPerSegment * 2
	var chunks [][]byte
	for i := 0; i < len(data); i += chunkSize {
		end := i + chunkSize
		if end > len(data) {
			end = len(data)
		}
		chunks = append(chunks, data[i:end])
	}
	return chunks
}

// SplitToSegmentShards splits concatenated shards into individual segment shards
// Used in CE139/CE140 GetSegmentShard_Assurer
func SplitToSegmentShards(concatenatedShards []byte) (segmentShards [][]byte) {
	fixedSegmentSize := types.NumECPiecesPerSegment * 2
	for i := 0; i < len(concatenatedShards); i += fixedSegmentSize {
		shard := concatenatedShards[i : i+fixedSegmentSize]
		segmentShards = append(segmentShards, shard)
	}
	return segmentShards
}

// SplitCompletExportToSegmentShards splits concatenated shards into segment and proof shards
func SplitCompletExportToSegmentShards(concatenatedShards []byte) (segmentShards [][]byte, proofShards [][]byte) {
	numSegmentPerPageProof := 64
	fixedSegmentSize := types.NumECPiecesPerSegment * 2
	dataLen := len(concatenatedShards)
	numTotalShards := dataLen / fixedSegmentSize
	numProofShards := (numTotalShards + numSegmentPerPageProof) / (1 + numSegmentPerPageProof)
	numSegmentShards := numTotalShards - numProofShards

	if numSegmentShards < 0 || numProofShards < 0 || numSegmentShards+numProofShards != numTotalShards || dataLen%fixedSegmentSize != 0 {
		log.Error(log.Node, "SplitCompletExportToSegmentShards: invalid Seg Computation", "dataLen", dataLen, "fixedSegmentSize", fixedSegmentSize, "numSegmentShards", numSegmentShards, "numProofShards", numProofShards, "numTotalShards", numTotalShards)
		return segmentShards, proofShards
	}

	segmentShards = make([][]byte, 0, numSegmentShards)
	proofShards = make([][]byte, 0, numProofShards)
	currentShardIndex := 0
	for i := 0; i < dataLen; i += fixedSegmentSize {
		shard := concatenatedShards[i : i+fixedSegmentSize]
		if currentShardIndex < numSegmentShards {
			segmentShards = append(segmentShards, shard)
		} else {
			proofShards = append(proofShards, shard)
		}
		currentShardIndex++
	}
	return segmentShards, proofShards
}

// ComputeShardIndex computes shard index from core index and validator index
func ComputeShardIndex(coreIdx uint16, validatorIdx uint16) (shardIndex uint16) {
	/*
	   i = (cR+v) mod V
	   i: shardIdx
	   v: validatorIdx
	   c: coreIdx
	   R: RecoveryThreshold
	   V: TotalValidators
	*/
	return (coreIdx*types.RecoveryThreshold + validatorIdx) % types.TotalValidators
}

