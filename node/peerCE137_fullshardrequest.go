package node

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/quic-go/quic-go"
)

/*
CE 137: Full Shard distribution
This protocol should be used by assurers to request their EC shards from the guarantors of a work-report.
The response should include a work-package bundle shard and a sequence of exported/proof segment shards.
The response should also include a "justification", proving the correctness of the shards.

The justification should be the co-path of the path from the erasure root to the shards

Erasure Root = [u8; 32]
Shard Index = u16
Bundle Shard = [u8]
Segment Shard = [u8; 12]
Hash = [u8; 32]
Justification = [Hash OR (Hash ++ Hash)]

Assurer -> Guarantor

--> Erasure Root ++ Shard Index
--> FIN
<-- Bundle Shard
<-- [Segment Shard] (Should include all exported and proof segment shards with the given index)
<-- Justification
<-- FIN
*/

type JAMSNPShardRequest struct {
	ErasureRoot common.Hash `json:"erasure_root"`
	ShardIndex  uint16      `json:"shard_index"`
}

// ToBytes serializes the JAMSNPShardRequest struct into a byte array
func (req *JAMSNPShardRequest) ToBytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Serialize ErasureRoot (32 bytes for common.Hash)
	if _, err := buf.Write(req.ErasureRoot[:]); err != nil {
		return nil, err
	}

	// Serialize ShardIndex (2 bytes)
	if err := binary.Write(buf, binary.LittleEndian, req.ShardIndex); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// FromBytes deserializes a byte array into a JAMSNPShardRequest struct
func (req *JAMSNPShardRequest) FromBytes(data []byte) error {
	buf := bytes.NewReader(data)

	// Deserialize ErasureRoot (32 bytes for common.Hash)
	if _, err := io.ReadFull(buf, req.ErasureRoot[:]); err != nil {
		return err
	}

	// Deserialize ShardIndex (2 bytes)
	if err := binary.Read(buf, binary.LittleEndian, &req.ShardIndex); err != nil {
		return err
	}

	return nil
}

func (p *Peer) SendFullShardRequest(
	ctx context.Context,
	erasureRoot common.Hash,
	shardIndex uint16,
) (bundleShard []byte, concatSegmentShards []byte, encodedPath []byte, err error) {

	if p.node.store.SendTrace {
		tracer := p.node.store.Tp.Tracer("NodeTracer")
		_, span := tracer.Start(ctx, fmt.Sprintf("[N%d] SendFullShardRequest", p.node.store.NodeID))
		defer span.End()
	}

	if ctx.Err() != nil {
		return nil, nil, nil, ctx.Err()
	}

	code := uint8(CE137_FullShardRequest)

	stream, err := p.openStream(ctx, code)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("openStream[CE137]: %w", err)
	}
	defer stream.Close()

	req := &JAMSNPShardRequest{
		ErasureRoot: erasureRoot,
		ShardIndex:  shardIndex,
	}
	reqBytes, err := req.ToBytes()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("ToBytes[ShardRequest]: %w", err)
	}

	if err := sendQuicBytes(ctx, stream, reqBytes, p.PeerID, code); err != nil {
		return nil, nil, nil, fmt.Errorf("sendQuicBytes[CE137]: %w", err)
	}

	// <-- Bundle Shard
	// <-- [Segment Shard] (Should include all exported and proof segment shards with the given index)
	// <-- Justification
	parts, err := receiveMultiple(ctx, stream, 3, p.PeerID, code)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("receiveMultiple[CE137]: %w", err)
	}

	return parts[0], parts[1], parts[2], nil
}

// guarantor receives CE137 request from assurer by erasureRoot and shard index
// guarantor receives CE137 request from assurer by erasureRoot and shard index
func (n *Node) onFullShardRequest(ctx context.Context, stream quic.Stream, msg []byte) error {
	defer stream.Close()

	var req JAMSNPShardRequest
	if err := req.FromBytes(msg); err != nil {
		log.Error(debugDA, "onFullShardRequest: failed to deserialize", "err", err)
		return fmt.Errorf("onFullShardRequest: decode failed: %w", err)
	}

	bundleShard, exportedSegmentsAndProofShards, encodedPath, ok, err := n.GetFullShard_Guarantor(req.ErasureRoot, req.ShardIndex)
	if err != nil {
		stream.CancelWrite(ErrKeyNotFound)
		return fmt.Errorf("onFullShardRequest: GetFullShard_Guarantor failed: %w", err)
	}
	if !ok {
		stream.CancelWrite(ErrKeyNotFound)
		return fmt.Errorf("onFullShardRequest: shard not found for root=%v, shardIndex=%d", req.ErasureRoot, req.ShardIndex)
	}

	code := uint8(CE137_FullShardRequest)

	// <-- Bundle Shard
	if err := sendQuicBytes(ctx, stream, bundleShard, n.id, code); err != nil {
		return fmt.Errorf("onFullShardRequest: send bundleShard failed: %w", err)
	}

	// <-- [Segment Shard] (Should include all exported and proof segment shards with the given index)
	if err := sendQuicBytes(ctx, stream, exportedSegmentsAndProofShards, n.id, code); err != nil {
		return fmt.Errorf("onFullShardRequest: send exportedSegments failed: %w", err)
	}

	// <-- Justification
	if err := sendQuicBytes(ctx, stream, encodedPath, n.id, code); err != nil {
		return fmt.Errorf("onFullShardRequest: send justification failed: %w", err)
	}

	log.Trace(debugDA, "onFullShardRequest completed", "n", n.String(),
		"erasureRoot", req.ErasureRoot,
		"shardIndex", req.ShardIndex,
		"bundleShardLen", len(bundleShard),
		"segmentsLen", len(exportedSegmentsAndProofShards),
		"encodedPathLen", len(encodedPath),
	)

	return nil
}
