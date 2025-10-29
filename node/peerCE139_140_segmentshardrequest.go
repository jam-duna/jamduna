package node

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"reflect"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	telemetry "github.com/colorfulnotion/jam/telemetry"
	"github.com/colorfulnotion/jam/types"
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
	Len          uint        `json:"len"`
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

	segIndexBytes, err := types.Encode(req.SegmentIndex)
	if err != nil {
		return nil, err
	}

	if _, err := buf.Write(segIndexBytes); err != nil {
		return nil, err
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

	remaining := data[34:] // 32 bytes for ErasureRoot + 2 bytes for ShardIndex

	var segmentIndex []uint16
	decodedData, _, decodeErr := types.Decode(remaining, reflect.TypeOf(segmentIndex))
	if decodeErr != nil {
		return fmt.Errorf("JAMSNPSegmentShardRequest - decode segment index Err: %w", decodeErr)
	}

	req.SegmentIndex = decodedData.([]uint16)
	req.Len = uint(len(req.SegmentIndex))

	return nil
}

// SendSegmentShardRequest sends a segment shard request to the peer and receives the response (kv variadic input is for telemetry)
func (p *Peer) SendSegmentShardRequest(
	ctx context.Context,
	erasureRoot common.Hash,
	shardIndex uint16,
	segmentIndex []uint16,
	withJustification bool,
	eventID uint64,
) (segmentShards []byte, justifications [][]byte, err error) {

	if ctx.Err() != nil {
		return nil, nil, ctx.Err()
	}

	code := uint8(CE139_SegmentShardRequest)
	if withJustification {
		code = CE140_SegmentShardRequestP
	}

	// Build segment shard requests for telemetry
	requests := make([]telemetry.SegmentShardRequest, len(segmentIndex))
	for i, segIdx := range segmentIndex {
		requests[i] = telemetry.SegmentShardRequest{
			ImportSegmentID: segIdx,
			ShardIndex:      shardIndex,
		}
	}

	// Telemetry: Sending segment shard request (event 162)
	p.node.telemetryClient.SendingSegmentShardRequest(eventID, p.GetPeer32(), withJustification, requests)

	stream, err := p.openStream(ctx, code)
	if err != nil {
		// Telemetry: Segment shard request failed (event 164)
		p.node.telemetryClient.SegmentShardRequestFailed(eventID, err.Error())
		return nil, nil, fmt.Errorf("openStream[%d]: %w", code, err)
	}

	req := &JAMSNPSegmentShardRequest{
		ErasureRoot:  erasureRoot,
		ShardIndex:   shardIndex,
		Len:          uint(len(segmentIndex)),
		SegmentIndex: segmentIndex,
	}

	reqBytes, err := req.ToBytes()
	if err != nil {
		stream.Close()
		// Telemetry: Segment shard request failed (event 164)
		p.node.telemetryClient.SegmentShardRequestFailed(eventID, err.Error())
		return nil, nil, fmt.Errorf("ToBytes[SegmentShardRequest]: %w", err)
	}

	// --> [Erasure Root ++ Shard Index ++ len++[Segment Index]]
	if err := sendQuicBytes(ctx, stream, reqBytes, p.PeerID, code); err != nil {
		// Telemetry: Segment shard request failed (event 164)
		p.node.telemetryClient.SegmentShardRequestFailed(eventID, err.Error())
		return nil, nil, fmt.Errorf("sendQuicBytes[%d]: %w", code, err)
	}
	// --> FIN
	stream.Close()

	// Telemetry: Segment shard request sent (event 165)
	p.node.telemetryClient.SegmentShardRequestSent(eventID)

	// <-- [Segment Shard]
	segmentShards, err = receiveQuicBytes(ctx, stream, p.PeerID, code)
	if err != nil {
		// Telemetry: Segment shard request failed (event 164)
		p.node.telemetryClient.SegmentShardRequestFailed(eventID, err.Error())
		return nil, nil, fmt.Errorf("receiveQuicBytes[SegmentShard]: %w", err)
	}

	// Optionally receive justifications
	if withJustification {
		justifications, err = receiveMultiple(ctx, stream, len(segmentIndex), p.PeerID, code)
		if err != nil {
			// Telemetry: Segment shard request failed (event 164)
			p.node.telemetryClient.SegmentShardRequestFailed(eventID, err.Error())
			return nil, nil, fmt.Errorf("receiveMultiple[Justifications]: %w", err)
		}
	}

	// Telemetry: Segment shards transferred (event 167)
	p.node.telemetryClient.SegmentShardsTransferred(eventID)

	return segmentShards, justifications, nil
}

// onSegmentShardRequest handles incoming segment shard requests, with kv variadic arguments
func (n *Node) onSegmentShardRequest(ctx context.Context, stream quic.Stream, msg []byte, withJustification bool, peerID uint16) (err error) {
	defer stream.Close()

	var req JAMSNPSegmentShardRequest
	code := uint8(CE139_SegmentShardRequest)
	if withJustification {
		code = uint8(CE140_SegmentShardRequestP)
	}

	// Telemetry: Receiving segment shard request (event 163)
	eventID := n.telemetryClient.GetEventID()
	n.telemetryClient.ReceivingSegmentShardRequest(n.PeerID32(peerID), withJustification)

	err = req.FromBytes(msg)
	if err != nil {
		fmt.Println("Error deserializing:", err)
		log.Warn(log.DA, "onSegmentShardRequest:FromBytes", "err", err, "CE139/140ReqMsg", fmt.Sprintf("0x%x", msg))
		// Telemetry: Segment shard request failed (event 164)
		n.telemetryClient.SegmentShardRequestFailed(eventID, err.Error())
		return fmt.Errorf("onSegmentShardRequest: FromBytes failed: %w", err)
	}
	log.Debug(log.G, "onSegmentShardRequest:FromBytes Received", "CE139/140ReqMsg", fmt.Sprintf("0x%x", msg), "ErasureRoot", req.ErasureRoot, "ShardIndex", req.ShardIndex, "Len", req.Len, "SegmentIndex", req.SegmentIndex)

	// Telemetry: Segment shard request received (event 166)
	n.telemetryClient.SegmentShardRequestReceived(eventID, uint16(req.Len))

	selected_segmentshards, selected_segment_justifications, ok, err := n.GetSegmentShard_Assurer(req.ErasureRoot, req.ShardIndex, req.SegmentIndex, withJustification)
	if err != nil {
		stream.CancelWrite(ErrKeyNotFound)
		log.Warn(log.DA, "onSegmentShardRequest:GetSegmentShard_Assurer", "err", err)
		// Telemetry: Segment shard request failed (event 164)
		n.telemetryClient.SegmentShardRequestFailed(eventID, err.Error())
		return fmt.Errorf("onSegmentShardRequest: GetSegmentShard_Assurer failed: %w", err)
	}
	if !ok {
		stream.CancelWrite(ErrKeyNotFound)
		log.Warn(log.DA, "onSegmentShardRequest:GetSegmentShard_Assurer", n.String(), req.ErasureRoot, req.ShardIndex, req.SegmentIndex)
		// Telemetry: Segment shard request failed (event 164)
		n.telemetryClient.SegmentShardRequestFailed(eventID, "segment shard not found")
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
		// Telemetry: Segment shard request failed (event 164)
		n.telemetryClient.SegmentShardRequestFailed(eventID, err.Error())
		return fmt.Errorf("onSegmentShardRequest: sendQuicBytes segment shard failed: %w", err)
	}

	if withJustification {
		for item_idx, s_j := range selected_segment_justifications {
			s_f := selected_segment_justifications[item_idx]

			if err = sendQuicBytes(ctx, stream, s_f, n.id, code); err != nil {
				// Telemetry: Segment shard request failed (event 164)
				n.telemetryClient.SegmentShardRequestFailed(eventID, err.Error())
				return fmt.Errorf("onSegmentShardRequest: sendQuicBytes justification s_f failed (idx %d): %w", item_idx, err)
			}

			if err = sendQuicBytes(ctx, stream, s_j, n.id, code); err != nil {
				// Telemetry: Segment shard request failed (event 164)
				n.telemetryClient.SegmentShardRequestFailed(eventID, err.Error())
				return fmt.Errorf("onSegmentShardRequest: sendQuicBytes justification s_j failed (idx %d): %w", item_idx, err)
			}
		}
	}

	// Telemetry: Segment shards transferred (event 167)
	n.telemetryClient.SegmentShardsTransferred(eventID)

	return nil
}
