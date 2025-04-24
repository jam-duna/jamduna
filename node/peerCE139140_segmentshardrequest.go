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

// SendSegmentShardRequest sends a segment shard request to the peer and receives the response (kv variadic input is for telemetry)
func (p *Peer) SendSegmentShardRequest(
	ctx context.Context,
	erasureRoot common.Hash,
	shardIndex uint16,
	segmentIndex []uint16,
	withJustification bool,
	kv ...interface{},
) (segmentShards []byte, justifications [][]byte, err error) {

	if p.node.store.SendTrace {
		tracer := p.node.store.Tp.Tracer("NodeTracer")
		_, span := tracer.Start(ctx, fmt.Sprintf("[N%d] SendSegmentShardRequest", p.node.store.NodeID))
		defer span.End()
	}

	if ctx.Err() != nil {
		return nil, nil, ctx.Err()
	}

	code := uint8(CE139_SegmentShardRequest)
	if withJustification {
		code = CE140_SegmentShardRequestP
	}

	stream, err := p.openStream(ctx, code)
	if err != nil {
		return nil, nil, fmt.Errorf("openStream[%d]: %w", code, err)
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
		return nil, nil, fmt.Errorf("ToBytes[SegmentShardRequest]: %w", err)
	}

	if err := sendQuicBytes(ctx, stream, reqBytes, p.PeerID, code); err != nil {
		return nil, nil, fmt.Errorf("sendQuicBytes[%d]: %w", code, err)
	}

	// <-- [Segment Shard]
	segmentShards, err = receiveQuicBytes(ctx, stream, p.PeerID, code)
	if err != nil {
		return nil, nil, fmt.Errorf("receiveQuicBytes[SegmentShard]: %w", err)
	}

	// Optionally receive justifications
	if withJustification {
		justifications, err = receiveMultiple(ctx, stream, len(segmentIndex), p.PeerID, code)
		if err != nil {
			return nil, nil, fmt.Errorf("receiveMultiple[Justifications]: %w", err)
		}
	}

	return segmentShards, justifications, nil
}

// onSegmentShardRequest handles incoming segment shard requests, with kv variadic arguments
func (n *Node) onSegmentShardRequest(ctx context.Context, stream quic.Stream, msg []byte, withJustification bool) (err error) {
	defer stream.Close()

	var req JAMSNPSegmentShardRequest
	code := uint8(CE139_SegmentShardRequest)
	if withJustification {
		code = uint8(CE140_SegmentShardRequestP)
	}

	err = req.FromBytes(msg)
	if err != nil {
		fmt.Println("Error deserializing:", err)
		return fmt.Errorf("onSegmentShardRequest: FromBytes failed: %w", err)
	}

	selected_segmentshards, selected_segment_justifications, ok, err := n.GetSegmentShard_Assurer(req.ErasureRoot, req.ShardIndex, req.SegmentIndex, withJustification)
	if err != nil {
		log.Warn(debugDA, "onSegmentShardRequest:GetSegmentShard_Assurer", "err", err)
		return fmt.Errorf("onSegmentShardRequest: GetSegmentShard_Assurer failed: %w", err)
	}
	if !ok {
		log.Warn(debugDA, "onSegmentShardRequest:GetSegmentShard_Assurer", n.String(), req.ErasureRoot, req.ShardIndex, req.SegmentIndex)
		return fmt.Errorf("onSegmentShardRequest: segment shard not found")
	}

	combined_selected_segmentshards := bytes.Join(selected_segmentshards, nil)

	select {
	case <-ctx.Done():
		return fmt.Errorf("onSegmentShardRequest: context cancelled before sending shard: %w", ctx.Err())
	default:
	}

	err = sendQuicBytes(ctx, stream, combined_selected_segmentshards, n.id, code)
	if err != nil {
		return fmt.Errorf("onSegmentShardRequest: sendQuicBytes segment shard failed: %w", err)
	}

	if withJustification {
		for item_idx, s_j := range selected_segment_justifications {
			s_f := selected_segment_justifications[item_idx]

			if err = sendQuicBytes(ctx, stream, s_f, n.id, code); err != nil {
				return fmt.Errorf("onSegmentShardRequest: sendQuicBytes justification s_f failed (idx %d): %w", item_idx, err)
			}

			if err = sendQuicBytes(ctx, stream, s_j, n.id, code); err != nil {
				return fmt.Errorf("onSegmentShardRequest: sendQuicBytes justification s_j failed (idx %d): %w", item_idx, err)
			}
		}
	}
	return nil
}
