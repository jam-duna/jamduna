package node

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
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
// JAMSNPSegmentShardRequestItem represents a single erasure-root request within a batch
type JAMSNPSegmentShardRequestItem struct {
	ErasureRoot  common.Hash `json:"erasure_root"`
	ShardIndex   uint16      `json:"shard_index"`
	SegmentIndex []uint16    `json:"segment_index"`
}

// JAMSNPSegmentShardRequest represents the request structure (array of items per spec)
// Spec: --> [Erasure Root ++ Shard Index ++ len++[Segment Index]]
type JAMSNPSegmentShardRequest struct {
	Items []JAMSNPSegmentShardRequestItem
}

// ToBytes serializes the JAMSNPSegmentShardRequest struct into a byte array
func (req *JAMSNPSegmentShardRequest) ToBytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Encode array length
	lenBytes := types.E(uint64(len(req.Items)))
	if _, err := buf.Write(lenBytes); err != nil {
		return nil, err
	}

	for _, item := range req.Items {
		// Serialize ErasureRoot (32 bytes for common.Hash)
		if _, err := buf.Write(item.ErasureRoot[:]); err != nil {
			return nil, err
		}

		// Serialize ShardIndex (2 bytes)
		if err := binary.Write(buf, binary.LittleEndian, item.ShardIndex); err != nil {
			return nil, err
		}

		segIndexBytes, err := types.Encode(item.SegmentIndex)
		if err != nil {
			return nil, err
		}

		if _, err := buf.Write(segIndexBytes); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

// FromBytes deserializes a byte array into a JAMSNPSegmentShardRequest struct
func (req *JAMSNPSegmentShardRequest) FromBytes(data []byte) error {
	if len(data) < 1 {
		return fmt.Errorf("JAMSNPSegmentShardRequest: data too short")
	}

	// Decode array length
	numItems, bytesRead := types.DecodeE(data)
	if bytesRead == 0 {
		return fmt.Errorf("JAMSNPSegmentShardRequest: failed to decode item count")
	}
	if numItems == 0 {
		return fmt.Errorf("JAMSNPSegmentShardRequest: no items in request")
	}
	data = data[bytesRead:]

	req.Items = make([]JAMSNPSegmentShardRequestItem, numItems)
	totalSegments := 0

	for i := uint64(0); i < numItems; i++ {
		if len(data) < 34 {
			return fmt.Errorf("JAMSNPSegmentShardRequest: insufficient data for item %d", i)
		}

		// Deserialize ErasureRoot (32 bytes for common.Hash)
		copy(req.Items[i].ErasureRoot[:], data[:32])
		data = data[32:]

		// Deserialize ShardIndex (2 bytes)
		req.Items[i].ShardIndex = binary.LittleEndian.Uint16(data[:2])
		data = data[2:]

		// Decode segment indices
		var segmentIndex []uint16
		decodedData, bytesConsumed, decodeErr := types.Decode(data, reflect.TypeOf(segmentIndex))
		if decodeErr != nil {
			return fmt.Errorf("JAMSNPSegmentShardRequest - decode segment index for item %d Err: %w", i, decodeErr)
		}

		req.Items[i].SegmentIndex = decodedData.([]uint16)
		data = data[bytesConsumed:]

		if len(req.Items[i].SegmentIndex) == 0 {
			return fmt.Errorf("JAMSNPSegmentShardRequest: empty segment indices for item %d", i)
		}
		totalSegments += len(req.Items[i].SegmentIndex)
		if totalSegments > types.MaxExports {
			return fmt.Errorf("JAMSNPSegmentShardRequest: too many total segments requested (%d > %d)", totalSegments, types.MaxExports)
		}
	}

	return nil
}

// TotalSegmentCount returns the total number of segments across all items
func (req *JAMSNPSegmentShardRequest) TotalSegmentCount() int {
	total := 0
	for _, item := range req.Items {
		total += len(item.SegmentIndex)
	}
	return total
}

// SendSegmentShardRequest sends a segment shard request to the peer (single erasure-root convenience wrapper)
func (p *Peer) SendSegmentShardRequest(
	ctx context.Context,
	erasureRoot common.Hash,
	shardIndex uint16,
	segmentIndex []uint16,
	withJustification bool,
	eventID uint64,
) (segmentShards []byte, justifications [][]byte, err error) {
	items := []JAMSNPSegmentShardRequestItem{{
		ErasureRoot:  erasureRoot,
		ShardIndex:   shardIndex,
		SegmentIndex: segmentIndex,
	}}
	return p.SendSegmentShardRequestBatch(ctx, items, withJustification, eventID)
}

// SendSegmentShardRequestBatch sends a batched segment shard request to the peer
// Supports multiple erasure-roots in one request per spec: --> [Erasure Root ++ Shard Index ++ len++[Segment Index]]
func (p *Peer) SendSegmentShardRequestBatch(
	ctx context.Context,
	items []JAMSNPSegmentShardRequestItem,
	withJustification bool,
	eventID uint64,
) (segmentShards []byte, justifications [][]byte, err error) {

	if ctx.Err() != nil {
		return nil, nil, ctx.Err()
	}

	// Validate request
	if len(items) == 0 {
		return nil, nil, fmt.Errorf("no segment shard request items provided")
	}

	code := uint8(CE139_SegmentShardRequest)
	if withJustification {
		code = CE140_SegmentShardRequestP
	}

	// Build segment shard requests for telemetry and validate
	var requests []telemetry.SegmentShardRequest
	totalSegments := 0
	for _, item := range items {
		if len(item.SegmentIndex) == 0 {
			return nil, nil, fmt.Errorf("empty segment indices for erasure root %s", item.ErasureRoot.Hex())
		}
		for _, segIdx := range item.SegmentIndex {
			requests = append(requests, telemetry.SegmentShardRequest{
				ImportSegmentID: segIdx,
				ShardIndex:      item.ShardIndex,
			})
		}
		totalSegments += len(item.SegmentIndex)
	}
	if totalSegments > types.MaxExports {
		return nil, nil, fmt.Errorf("too many total segments requested: %d > %d", totalSegments, types.MaxExports)
	}

	// Telemetry: Sending segment shard request (event 162)
	p.node.telemetryClient.SendingSegmentShardRequest(eventID, p.PeerKey(), withJustification, requests)

	stream, err := p.openStream(ctx, code)
	if err != nil {
		// Telemetry: Segment shard request failed (event 164)
		p.node.telemetryClient.SegmentShardRequestFailed(eventID, err.Error())
		return nil, nil, fmt.Errorf("openStream[%d]: %w", code, err)
	}

	req := &JAMSNPSegmentShardRequest{Items: items}

	reqBytes, err := req.ToBytes()
	if err != nil {
		stream.Close()
		// Telemetry: Segment shard request failed (event 164)
		p.node.telemetryClient.SegmentShardRequestFailed(eventID, err.Error())
		return nil, nil, fmt.Errorf("ToBytes[SegmentShardRequest]: %w", err)
	}

	// --> [Erasure Root ++ Shard Index ++ len++[Segment Index]]
	if err := sendQuicBytes(ctx, stream, reqBytes, p.SanKey(), code); err != nil {
		// Telemetry: Segment shard request failed (event 164)
		p.node.telemetryClient.SegmentShardRequestFailed(eventID, err.Error())
		return nil, nil, fmt.Errorf("sendQuicBytes[%d]: %w", code, err)
	}
	// --> FIN
	stream.Close()

	// Telemetry: Segment shard request sent (event 165)
	p.node.telemetryClient.SegmentShardRequestSent(eventID)

	// <-- [Segment Shard]
	segmentShards, err = receiveQuicBytes(ctx, stream, p.SanKey(), code)
	if err != nil {
		// Telemetry: Segment shard request failed (event 164)
		p.node.telemetryClient.SegmentShardRequestFailed(eventID, err.Error())
		return nil, nil, fmt.Errorf("receiveQuicBytes[SegmentShard]: %w", err)
	}

	// Optionally receive justifications
	if withJustification {
		justifications, err = receiveMultiple(ctx, stream, totalSegments, p.SanKey(), code)
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

// onSegmentShardRequest handles incoming segment shard requests (supports batched requests)
func (n *Node) onSegmentShardRequest(ctx context.Context, stream quic.Stream, msg []byte, withJustification bool, peerKey string) (err error) {
	defer stream.Close()

	// Get peer to access its PeerID for telemetry
	peer, ok := n.peersByPubKey[peerKey]
	if !ok {
		return fmt.Errorf("onSegmentShardRequest: peer not found for key %s", peerKey)
	}

	var req JAMSNPSegmentShardRequest
	code := uint8(CE139_SegmentShardRequest)
	if withJustification {
		code = uint8(CE140_SegmentShardRequestP)
	}

	// Telemetry: Receiving segment shard request (event 163)
	eventID := n.telemetryClient.GetEventID()
	n.telemetryClient.ReceivingSegmentShardRequest(PubkeyBytes(peer.Validator.Ed25519.SAN()), withJustification)

	err = req.FromBytes(msg)
	if err != nil {
		stream.CancelWrite(ErrInvalidData)
		log.Warn(log.DA, "onSegmentShardRequest:FromBytes", "err", err, "CE139/140ReqMsg", fmt.Sprintf("0x%x", msg))
		// Telemetry: Segment shard request failed (event 164)
		n.telemetryClient.SegmentShardRequestFailed(eventID, err.Error())
		return fmt.Errorf("onSegmentShardRequest: FromBytes failed: %w", err)
	}
	log.Debug(log.G, "onSegmentShardRequest:FromBytes Received", "CE139/140ReqMsg", fmt.Sprintf("0x%x", msg), "numItems", len(req.Items), "totalSegments", req.TotalSegmentCount())

	// Telemetry: Segment shard request received (event 166)
	n.telemetryClient.SegmentShardRequestReceived(eventID, uint16(req.TotalSegmentCount()))

	// Collect all segment shards and justifications across all request items
	var allSegmentShards [][]byte
	var allJustifications [][]byte

	for _, item := range req.Items {
		selected_segmentshards, selected_segment_justifications, ok, err := n.GetSegmentShard_Assurer(item.ErasureRoot, item.ShardIndex, item.SegmentIndex, withJustification)
		if err != nil {
			stream.CancelWrite(ErrKeyNotFound)
			log.Warn(log.DA, "onSegmentShardRequest:GetSegmentShard_Assurer", "err", err)
			// Telemetry: Segment shard request failed (event 164)
			n.telemetryClient.SegmentShardRequestFailed(eventID, err.Error())
			return fmt.Errorf("onSegmentShardRequest: GetSegmentShard_Assurer failed: %w", err)
		}
		if !ok {
			stream.CancelWrite(ErrKeyNotFound)
			log.Warn(log.DA, "onSegmentShardRequest:GetSegmentShard_Assurer", "n", n.String(), "erasureRoot", item.ErasureRoot, "shardIndex", item.ShardIndex, "segmentIndex", item.SegmentIndex)
			// Telemetry: Segment shard request failed (event 164)
			n.telemetryClient.SegmentShardRequestFailed(eventID, "segment shard not found")
			return fmt.Errorf("onSegmentShardRequest: segment shard not found")
		}
		allSegmentShards = append(allSegmentShards, selected_segmentshards...)
		if withJustification {
			allJustifications = append(allJustifications, selected_segment_justifications...)
		}
	}

	combined_selected_segmentshards := bytes.Join(allSegmentShards, nil)

	select {
	case <-ctx.Done():
		return fmt.Errorf("onSegmentShardRequest: context cancelled before sending shard: %w", ctx.Err())
	default:
	}

	err = sendQuicBytes(ctx, stream, combined_selected_segmentshards, n.GetEd25519Key().SAN(), code)
	if err != nil {
		stream.CancelWrite(ErrCECode)
		// Telemetry: Segment shard request failed (event 164)
		n.telemetryClient.SegmentShardRequestFailed(eventID, err.Error())
		return fmt.Errorf("onSegmentShardRequest: sendQuicBytes segment shard failed: %w", err)
	}

	if withJustification {
		for item_idx, s_j := range allJustifications {
			if err = sendQuicBytes(ctx, stream, s_j, n.GetEd25519Key().SAN(), code); err != nil {
				stream.CancelWrite(ErrCECode)
				// Telemetry: Segment shard request failed (event 164)
				n.telemetryClient.SegmentShardRequestFailed(eventID, err.Error())
				return fmt.Errorf("onSegmentShardRequest: sendQuicBytes justification failed (idx %d): %w", item_idx, err)
			}
		}
	}

	// Telemetry: Segment shards transferred (event 167)
	n.telemetryClient.SegmentShardsTransferred(eventID)

	return nil
}
