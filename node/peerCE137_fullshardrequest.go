package node

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/colorfulnotion/jam/common"
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

func (p *Peer) SendFullShardRequest(erasureRoot common.Hash, shardIndex uint16) (bundleShard []byte, concatSegmentShards []byte, encodedPath []byte, err error) {
	if p.node.store.SendTrace {
		tracer := p.node.store.Tp.Tracer("NodeTracer")
		_, span := tracer.Start(context.Background(), fmt.Sprintf("[N%d] SendFullShardRequest", p.node.store.NodeID))
		// p.node.UpdateFullShardContext(ctx)
		defer span.End()
	}

	code := uint8(CE137_FullShardRequest)
	stream, err := p.openStream(code)
	if err != nil {
		return
	}
	defer stream.Close()
	req := &JAMSNPShardRequest{
		ErasureRoot: erasureRoot,
		ShardIndex:  shardIndex,
	}

	reqBytes, err := req.ToBytes()
	if err != nil {
		return
	}
	err = sendQuicBytes(stream, reqBytes, p.PeerID, code)
	if err != nil {
		return
	}

	// <-- Bundle Shard
	bundleShard, err = receiveQuicBytes(stream, p.PeerID, code)
	if err != nil {
		return
	}

	// <-- [Segment Shard] (Should include all exported and proof segment shards with the given index)
	concatSegmentShards, err = receiveQuicBytes(stream, p.PeerID, code)
	if err != nil {
		return
	}

	// <-- Justification
	encodedPath, err = receiveQuicBytes(stream, p.PeerID, code)
	if err != nil {
		return
	}
	return
}

// guarantor receives CE137 request from assurer by erasureRoot and shard index
func (n *Node) onFullShardRequest(stream quic.Stream, msg []byte) (err error) {
	defer stream.Close()
	var req JAMSNPShardRequest
	// Deserialize byte array back into the struct
	err = req.FromBytes(msg)
	if err != nil {
		fmt.Println("Error deserializing:", err)
		return
	}

	bundleShard, exported_segments_and_proofpageShards, encodedPath, ok, err := n.GetFullShard_Guarantor(req.ErasureRoot, req.ShardIndex)
	if err != nil {
		fmt.Printf("onFullShardRequest ERR %v\n", err)
		return err
	}
	if !ok {
		return fmt.Errorf("Not found")
	}

	// <-- Bundle Shard
	err = sendQuicBytes(stream, bundleShard, n.id, CE137_FullShardRequest)
	if err != nil {
		fmt.Printf("onFullShardRequest ERR %v\n", err)
		return err
	}

	// <-- [Segment Shard] (Should include all exported and proof segment shards with the given index)
	err = sendQuicBytes(stream, exported_segments_and_proofpageShards, n.id, CE137_FullShardRequest)
	if err != nil {
		return err
	}

	// <-- Justification
	err = sendQuicBytes(stream, encodedPath, n.id, CE137_FullShardRequest)
	if err != nil {
		return err
	}

	// <-- FIN
	return nil
}
