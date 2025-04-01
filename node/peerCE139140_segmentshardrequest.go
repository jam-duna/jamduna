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
CE 139/140: Segment shard request
Request for one or more 12-byte segment shards.

This protocol should be used by guarantors to request import segment shards from assurers in order to complete work-package bundles for guaranteeing.

This protocol has two variants: 139 and 140.
In the first variant, the assurer does not provide any justification for the returned segment shards.
In the second variant, the assurer provides a justification for each returned segment shard, allowing the guarantor to immediately assess the correctness of the response.

The justification for a segment shard should be the co-path from the erasure root to the shard

Guarantors should initially use protocol 139 to fetch segment shards.
If a reconstructed import segment is inconsistent with its reconstructed proof, the segment and proof should be reconstructed again,
using shards retrieved with protocol 140. When using this protocol, the guarantor can verify the correctness of each response as it is received,
requesting shards from a different assurer in the case of an incorrect response. If the reconstructed segment and proof are still inconsistent,
then it can be concluded that the erasure root is invalid.

Erasure Root = [u8; 32]
Shard Index = u16
Segment Index = u16
Segment Shard = [u8; 12]
Hash = [u8; 32]
Justification = [Hash OR (Hash ++ Hash) OR Segment Shard]

Guarantor -> Assurer

--> [Erasure Root ++ Shard Index ++ len++[Segment Index]]
--> FIN
<-- [Segment Shard]
[Protocol 140 only] for each segment shard {
[Protocol 140 only]     <-- Justification
[Protocol 140 only] }
<-- FIN
*/
// JAMSNPSegmentShardRequest represents the request structure
type JAMSNPSegmentShardRequest struct {
	ErasureRoot  common.Hash `json:"erasure_root"`
	ShardIndex   uint16      `json:"shard_index"`
	Len          uint8       `json:"len"`
	SegmentIndex []uint16    `json:"segment_index"`
}

// ToBytes serializes the JAMSNPSegmentShardRequest struct into a byte array
func (req *JAMSNPSegmentShardRequest) ToBytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Serialize ErasureRoot (32 bytes for common.Hash)
	if _, err := buf.Write(req.ErasureRoot[:]); err != nil {
		return nil, err
	}

	// Serialize ShardIndex (2 bytes)
	if err := binary.Write(buf, binary.LittleEndian, req.ShardIndex); err != nil {
		return nil, err
	}

	// Serialize Len (1 byte)
	if err := binary.Write(buf, binary.LittleEndian, req.Len); err != nil {
		return nil, err
	}

	// Serialize the length of SegmentIndex array (2 bytes for the length)
	segmentIndexLength := uint16(len(req.SegmentIndex))
	if err := binary.Write(buf, binary.LittleEndian, segmentIndexLength); err != nil {
		return nil, err
	}

	// Serialize each SegmentIndex entry (2 bytes each)
	for _, segment := range req.SegmentIndex {
		if err := binary.Write(buf, binary.LittleEndian, segment); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

// FromBytes deserializes a byte array into a JAMSNPSegmentShardRequest struct
func (req *JAMSNPSegmentShardRequest) FromBytes(data []byte) error {
	buf := bytes.NewReader(data)

	// Deserialize ErasureRoot (32 bytes for common.Hash)
	if _, err := io.ReadFull(buf, req.ErasureRoot[:]); err != nil {
		return err
	}

	// Deserialize ShardIndex (2 bytes)
	if err := binary.Read(buf, binary.LittleEndian, &req.ShardIndex); err != nil {
		return err
	}

	// Deserialize Len (1 byte)
	if err := binary.Read(buf, binary.LittleEndian, &req.Len); err != nil {
		return err
	}

	// Deserialize the length of SegmentIndex array (2 bytes)
	var segmentIndexLength uint16
	if err := binary.Read(buf, binary.LittleEndian, &segmentIndexLength); err != nil {
		return err
	}

	// Deserialize each SegmentIndex entry (2 bytes each)
	req.SegmentIndex = make([]uint16, segmentIndexLength)
	for i := 0; i < int(segmentIndexLength); i++ {
		if err := binary.Read(buf, binary.LittleEndian, &req.SegmentIndex[i]); err != nil {
			return err
		}
	}

	return nil
}

func (p *Peer) SendSegmentShardRequest(erasureRoot common.Hash, shardIndex uint16, segmentIndex []uint16, withJustification bool) (segmentShards []byte, justifications [][]byte, err error) {

	if p.node.store.SendTrace {
		tracer := p.node.store.Tp.Tracer("NodeTracer")
		_, span := tracer.Start(context.Background(), fmt.Sprintf("[N%d] SendSegmentShardRequest", p.node.store.NodeID))
		// p.node.UpdateSegmentShardContext(ctx)
		defer span.End()
	}

	code := uint8(CE139_SegmentShardRequest)
	if withJustification {
		code = CE140_SegmentShardRequestP
	}
	stream, err := p.openStream(code)
	if err != nil {
		return
	}
	defer stream.Close()
	req := &JAMSNPSegmentShardRequest{
		ErasureRoot:  erasureRoot,
		ShardIndex:   shardIndex,
		Len:          uint8(len(segmentIndex)),
		SegmentIndex: segmentIndex,
	}

	reqBytes, err := req.ToBytes()
	if err != nil {
		return
	}
	err = sendQuicBytes(stream, reqBytes)
	if err != nil {
		return
	}

	// <-- [Segment Shard]
	segmentShards, err = receiveQuicBytes(stream)
	if err != nil {
		fmt.Printf("%s [SendSegmentShardRequest:receiveQuicBytes] ERR %v\n", p.String(), err)
		// panic(11060)
		return
	}
	if withJustification {
		for j := uint8(0); j < req.Len; j++ {
			var justification []byte
			justification, err = receiveQuicBytes(stream)
			if err != nil {
				return
			}
			justifications = append(justifications, justification)
		}
	}
	return
}

func (n *Node) onSegmentShardRequest(stream quic.Stream, msg []byte, withJustification bool) (err error) {
	defer stream.Close()
	var req JAMSNPSegmentShardRequest
	// Deserialize byte array back into the struct
	err = req.FromBytes(msg)
	if err != nil {
		fmt.Println("Error deserializing:", err)
		return
	}
	selected_segmentshards, selected_segment_justifications, ok, err := n.GetSegmentShard_Assurer(req.ErasureRoot, req.ShardIndex, req.SegmentIndex, withJustification)
	if err != nil {
		log.Error(debugDA, "onSegmentShardRequest:GetSegmentShard_Assurer", "err", err)
		return err
	}
	if !ok {
		log.Warn(debugDA, "onSegmentShardRequest:GetSegmentShard_Assurer", n.String(), req.ErasureRoot, req.ShardIndex, req.SegmentIndex)
		return fmt.Errorf("Not found")
	}
	// <-- Segment Shard
	combined_selected_segmentshards := bytes.Join(selected_segmentshards, nil)
	if false {
		fmt.Printf("%s onSegmentShardRequest: GetSegmentShard_AssurerSimple erasure root %s shard %d (%v) => %d bytes h(concat)=%s\n",
			n.String(), req.ErasureRoot, req.ShardIndex, req.SegmentIndex, len(combined_selected_segmentshards), common.Blake2Hash(combined_selected_segmentshards))
	}

	err = sendQuicBytes(stream, combined_selected_segmentshards)
	if err != nil {
		return
	}

	// <-- [Segment Shard] (Should include all exported and proof segment shards with the given index)
	if withJustification {
		for item_idx, s_j := range selected_segment_justifications {
			s_f := selected_segment_justifications[item_idx]
			err = sendQuicBytes(stream, s_f)
			if err != nil {
				return
			}
			err = sendQuicBytes(stream, s_j)
			if err != nil {
				return
			}
		}
	}

	// <-- FIN
	return nil
}
