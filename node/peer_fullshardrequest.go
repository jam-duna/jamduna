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
	if err := binary.Write(buf, binary.BigEndian, req.ShardIndex); err != nil {
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
	if err := binary.Read(buf, binary.BigEndian, &req.ShardIndex); err != nil {
		return err
	}

	return nil
}

func (p *Peer) SendFullShardRequest(erasureRoot common.Hash, shardIndex uint16) (bundleShard []byte, concatSegmentShards []byte, justification []byte, err error) {
	// TODO: add span for SendFullShardRequest => [Segment Shard]+Justification here
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
	err = sendQuicBytes(stream, reqBytes)
	if err != nil {
		return
	}
	if debugA {
		fmt.Printf("%s SendFullShardRequest(erasureRoot=%v, shardIndex=%d)\n", p.String(), req.ErasureRoot, req.ShardIndex)
	}
	// <-- Bundle Shard
	bundleShard, err = receiveQuicBytes(stream)
	if err != nil {
		return
	}
	if debugA {
		fmt.Printf("%s SendFullShardRequest received %d bytes for bundleShard\n", p.String(), len(bundleShard))
	}
	// <-- [Segment Shard] (Should include all exported and proof segment shards with the given index)
	concatSegmentShards, err = receiveQuicBytes(stream)
	if err != nil {
		return
	}

	// <-- Justification
	justification, err = receiveQuicBytes(stream)
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
	if debugA {
		fmt.Printf("%s onFullShardRequest(erasureRoot=%v, shardIndex=%d)\n", n.String(), req.ErasureRoot, req.ShardIndex)
	}
	_, _, bundleShard, segmentShards, f_justification, ok, err := n.GetFullShard_Guarantor(req.ErasureRoot, req.ShardIndex)
	if err != nil {
		fmt.Printf("onFullShardRequest ERR0 %v\n", err)
		return err
	}
	if !ok {
		return fmt.Errorf("Not found")
	}
	// <-- Bundle Shard
	err = sendQuicBytes(stream, bundleShard)
	if err != nil {
		fmt.Printf("onFullShardRequest ERR1 %v\n", err)
		return err
	}

	// <-- [Segment Shard] (Should include all exported and proof segment shards with the given index)
	err = sendQuicBytes(stream, common.ConcatenateByteSlices(segmentShards))
	if err != nil {
		return err
	}

	// <-- Justification
	err = sendQuicBytes(stream, f_justification)
	if err != nil {
		return err
	}

	// <-- FIN
	return nil
}
