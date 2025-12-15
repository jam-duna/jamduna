package node

import (
	"context"
	"encoding/binary"
	"fmt"
	"reflect"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	telemetry "github.com/colorfulnotion/jam/telemetry"
	trie "github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
	"github.com/quic-go/quic-go"
)

/*
CE 148: Segment request
Request for one or more segments.

This protocol should be used by guarantors or builders to request import segments from other
guarantors in order to complete work-package bundles.

The number of segments requested in a single stream should not exceed 3072.

If the guarantor fails to return the valid data, the requestor should fall back to using CE 139/140.

Segment Index = u16
Import-Proof = len++[Hash]
Segments-Root = [u8; 32]

Guarantor -> Guarantor

--> [Segments-Root ++ len++[Segment Index]]
--> FIN
<-- [Segment]
<-- [Import-Proof]
<-- FIN
*/

// SegmentRequestItem represents a single segment-root request within a batch
type SegmentRequestItem struct {
	SegmentsRoot   common.Hash // 32 bytes
	SegmentIndices []uint16    // Variable length
}

// SegmentRequest represents a CE 148 segment request (array of items per spec)
// Spec: --> [Segments-Root ++ len++[Segment Index]]
type SegmentRequest struct {
	Items []SegmentRequestItem
}

// ToBytes encodes a SegmentRequest to bytes
func (r *SegmentRequest) ToBytes() []byte {
	var data []byte

	// Encode array length
	data = append(data, types.E(uint64(len(r.Items)))...)

	for _, item := range r.Items {
		// Segments-Root (32 bytes)
		data = append(data, item.SegmentsRoot.Bytes()...)

		// len++[Segment Index]
		data = append(data, types.E(uint64(len(item.SegmentIndices)))...)
		for _, idx := range item.SegmentIndices {
			data = append(data, telemetry.Uint16ToBytes(idx)...)
		}
	}

	return data
}

// FromBytes decodes a SegmentRequest from bytes
func (r *SegmentRequest) FromBytes(data []byte) error {
	if len(data) < 1 {
		return fmt.Errorf("invalid segment request: too short")
	}

	// Decode array length
	numItems, bytesRead := types.DecodeE(data)
	if bytesRead == 0 {
		return fmt.Errorf("invalid segment request: failed to decode item count")
	}
	if numItems == 0 {
		return fmt.Errorf("invalid segment request: no items in request")
	}
	data = data[bytesRead:]

	r.Items = make([]SegmentRequestItem, numItems)
	totalSegments := 0

	for i := uint64(0); i < numItems; i++ {
		if len(data) < 32 {
			return fmt.Errorf("invalid segment request: insufficient data for segments root %d", i)
		}

		// Segments-Root (32 bytes)
		r.Items[i].SegmentsRoot = common.BytesToHash(data[:32])
		data = data[32:]

		// len++[Segment Index]
		numIndices, bytesRead := types.DecodeE(data)
		if bytesRead == 0 {
			return fmt.Errorf("invalid segment request: failed to decode segment count for item %d", i)
		}
		data = data[bytesRead:]

		if numIndices == 0 {
			return fmt.Errorf("invalid segment request: empty segment indices for item %d", i)
		}
		totalSegments += int(numIndices)
		if totalSegments > types.MaxExports {
			return fmt.Errorf("invalid segment request: too many total segments requested (%d > %d)", totalSegments, types.MaxExports)
		}

		r.Items[i].SegmentIndices = make([]uint16, numIndices)
		for j := uint64(0); j < numIndices; j++ {
			if len(data) < 2 {
				return fmt.Errorf("invalid segment request: insufficient data for segment index %d/%d", i, j)
			}
			r.Items[i].SegmentIndices[j] = binary.LittleEndian.Uint16(data[:2])
			data = data[2:]
		}
	}

	return nil
}

// TotalSegmentCount returns the total number of segments across all items
func (r *SegmentRequest) TotalSegmentCount() int {
	total := 0
	for _, item := range r.Items {
		total += len(item.SegmentIndices)
	}
	return total
}

// SegmentResponse represents a response to a CE 148 segment request
type SegmentResponse struct {
	Segment     []byte        // Variable length segment data
	ImportProof []common.Hash // Variable length proof (list of hashes)
}

// ToBytes encodes a SegmentResponse to bytes (for a single segment)
func (r *SegmentResponse) ToBytes() []byte {
	var data []byte

	// Segment data (raw bytes, length will be sent separately via QUIC framing)
	data = append(data, r.Segment...)

	// Import-Proof: len++[Hash]
	data = append(data, types.E(uint64(len(r.ImportProof)))...)
	for _, hash := range r.ImportProof {
		data = append(data, hash.Bytes()...)
	}

	return data
}

// SendSegmentRequest sends a segment request to a peer (requester side) - single segment root
// This is a convenience wrapper around SendSegmentRequestBatch for the common single-root case
func (p *Peer) SendSegmentRequest(ctx context.Context, segmentsRoot common.Hash, segmentIndices []uint16, eventID uint64) ([]SegmentResponse, error) {
	items := []SegmentRequestItem{{
		SegmentsRoot:   segmentsRoot,
		SegmentIndices: segmentIndices,
	}}
	return p.SendSegmentRequestBatch(ctx, items, eventID)
}

// SendSegmentRequestBatch sends a batched segment request to a peer (requester side)
// Supports multiple segment-roots in one request per spec: --> [Segments-Root ++ len++[Segment Index]]
func (p *Peer) SendSegmentRequestBatch(ctx context.Context, items []SegmentRequestItem, eventID uint64) ([]SegmentResponse, error) {
	code := uint8(CE148_SegmentRequest)

	// Validate request
	if len(items) == 0 {
		return nil, fmt.Errorf("no segment request items provided")
	}

	totalSegments := 0
	var allIndices []uint16
	for _, item := range items {
		if len(item.SegmentIndices) == 0 {
			return nil, fmt.Errorf("empty segment indices for root %s", item.SegmentsRoot.Hex())
		}
		totalSegments += len(item.SegmentIndices)
		allIndices = append(allIndices, item.SegmentIndices...)
	}
	if totalSegments > types.MaxExports {
		return nil, fmt.Errorf("too many total segments requested: %d > %d", totalSegments, types.MaxExports)
	}

	// Telemetry: Sending segment request (event 173)
	p.node.telemetryClient.SendingSegmentRequest(eventID, p.PeerKey(), allIndices)

	stream, err := p.openStream(ctx, code)
	if err != nil {
		// Telemetry: Segment request failed (event 175)
		p.node.telemetryClient.SegmentRequestFailed(eventID, err.Error())
		return nil, fmt.Errorf("openStream[CE148_SegmentRequest]: %w", err)
	}

	// Build request
	req := SegmentRequest{Items: items}

	// --> [Segments-Root ++ len++[Segment Index]]
	reqBytes := req.ToBytes()
	if err := sendQuicBytes(ctx, stream, reqBytes, p.Validator.Ed25519.String(), code); err != nil {
		// Telemetry: Segment request failed (event 175)
		p.node.telemetryClient.SegmentRequestFailed(eventID, err.Error())
		return nil, fmt.Errorf("sendQuicBytes[CE148_SegmentRequest]: %w", err)
	}

	// Telemetry: Segment request sent (event 176)
	p.node.telemetryClient.SegmentRequestSent(eventID)

	// --> FIN
	stream.Close()

	// <-- [Segment] - all segments concatenated in single message
	allSegmentsBytes, err := receiveQuicBytes(ctx, stream, p.Validator.Ed25519.String(), code)
	if err != nil {
		p.node.telemetryClient.SegmentRequestFailed(eventID, err.Error())
		return nil, fmt.Errorf("receiveQuicBytes[CE148_SegmentRequest] segments: %w", err)
	}

	// <-- [Import-Proof] - all import proofs concatenated in single message
	allProofsBytes, err := receiveQuicBytes(ctx, stream, p.Validator.Ed25519.String(), code)
	if err != nil {
		p.node.telemetryClient.SegmentRequestFailed(eventID, err.Error())
		return nil, fmt.Errorf("receiveQuicBytes[CE148_SegmentRequest] proofs: %w", err)
	}

	// Parse segments - split by SegmentSize (4104 bytes)
	responses := make([]SegmentResponse, 0, totalSegments)
	for i := 0; i+types.SegmentSize <= len(allSegmentsBytes); i += types.SegmentSize {
		responses = append(responses, SegmentResponse{
			Segment: allSegmentsBytes[i : i+types.SegmentSize],
		})
	}

	// Parse import proofs - each is len++[Hash]
	proofData := allProofsBytes
	for i := 0; i < len(responses) && len(proofData) > 0; i++ {
		proof, bytesConsumed, err := decodeImportProofWithLength(proofData)
		if err != nil {
			p.node.telemetryClient.SegmentRequestFailed(eventID, err.Error())
			return nil, fmt.Errorf("decodeImportProof[CE148_SegmentRequest] segment %d: %w", i, err)
		}
		responses[i].ImportProof = proof
		proofData = proofData[bytesConsumed:]
	}

	// Telemetry: Segments transferred (event 178)
	p.node.telemetryClient.SegmentsTransferred(eventID)

	log.Trace(log.Node, "CE148-SendSegmentRequestBatch",
		"node", p.node.id,
		"peerKey", p.Validator.Ed25519.ShortString(),
		"numItems", len(items),
		"numRequested", totalSegments,
		"numReceived", len(responses),
	)

	return responses, nil
}

// onSegmentRequest handles incoming segment requests (responder side)
// Supports batched requests with multiple segment-roots per spec
func (n *Node) onSegmentRequest(ctx context.Context, stream quic.Stream, msg []byte, peerKey string) error {
	defer stream.Close()

	// Get peer to access its PeerID for telemetry
	peer, ok := n.peersByPubKey[peerKey]
	if !ok {
		return fmt.Errorf("onSegmentRequest: peer not found for key %s", peerKey)
	}

	// Telemetry: Receiving segment request (event 174)
	eventID := n.telemetryClient.GetEventID()
	n.telemetryClient.ReceivingSegmentRequest(PubkeyBytes(peer.Validator.Ed25519.String()))

	// Parse request
	var req SegmentRequest
	if err := req.FromBytes(msg); err != nil {
		stream.CancelWrite(ErrInvalidData)
		log.Error(log.Node, "onSegmentRequest: failed to decode", "err", err)
		// Telemetry: Segment request failed (event 175)
		n.telemetryClient.SegmentRequestFailed(eventID, err.Error())
		return fmt.Errorf("onSegmentRequest: decode failed: %w", err)
	}

	// Telemetry: Segment request received (event 177)
	n.telemetryClient.SegmentRequestReceived(eventID, uint16(req.TotalSegmentCount()))

	log.Trace(log.Node, "CE148-onSegmentRequest INCOMING",
		"node", n.id,
		"peerKey", peerKey,
		"numItems", len(req.Items),
		"totalSegments", req.TotalSegmentCount(),
	)

	code := uint8(CE148_SegmentRequest)

	// Collect all segments and proofs across all request items
	var allSegments []byte
	var allImportProofs []byte

	for _, item := range req.Items {
		segments, ok := n.getSegmentsBySegmentRoot(item.SegmentsRoot)
		if !ok {
			// Telemetry: Segment request failed (event 175)
			n.telemetryClient.SegmentRequestFailed(eventID, "segments not found")
			return fmt.Errorf("onSegmentRequest: segments not found for root %s", item.SegmentsRoot.Hex())
		}

		segmentTree := trie.NewCDMerkleTree(segments)
		computedRoot := common.BytesToHash(segmentTree.Root())
		if computedRoot != item.SegmentsRoot {
			err := fmt.Errorf("segment root mismatch: expected %s got %s", item.SegmentsRoot.Hex(), computedRoot.Hex())
			n.telemetryClient.SegmentRequestFailed(eventID, err.Error())
			return err
		}

		for _, segmentIdx := range item.SegmentIndices {
			if int(segmentIdx) >= len(segments) {
				log.Warn(log.Node, "onSegmentRequest: segment index out of range",
					"segmentsRoot", item.SegmentsRoot,
					"segmentIndex", segmentIdx,
					"availableSegments", len(segments))
				n.telemetryClient.SegmentRequestFailed(eventID, fmt.Sprintf("segment %d out of range", segmentIdx))
				return fmt.Errorf("onSegmentRequest: segment %d out of range", segmentIdx)
			}

			proof, err := segmentTree.GenerateCDTJustificationX(int(segmentIdx), 0)
			if err != nil {
				log.Warn(log.Node, "onSegmentRequest: failed to generate justification",
					"segmentsRoot", item.SegmentsRoot,
					"segmentIndex", segmentIdx,
					"err", err)
				n.telemetryClient.SegmentRequestFailed(eventID, err.Error())
				return fmt.Errorf("onSegmentRequest: failed to generate justification for segment %d: %w", segmentIdx, err)
			}

			allSegments = append(allSegments, segments[segmentIdx]...)
			allImportProofs = append(allImportProofs, encodeImportProof(proof)...)
		}
	}

	// <-- [Segment] - all segments concatenated in single message
	if err := sendQuicBytes(ctx, stream, allSegments, n.GetEd25519Key().String(), code); err != nil {
		stream.CancelWrite(ErrCECode)
		n.telemetryClient.SegmentRequestFailed(eventID, err.Error())
		return fmt.Errorf("onSegmentRequest: sendQuicBytes segments failed: %w", err)
	}

	// <-- [Import-Proof] - all import proofs concatenated in single message
	if err := sendQuicBytes(ctx, stream, allImportProofs, n.GetEd25519Key().String(), code); err != nil {
		stream.CancelWrite(ErrCECode)
		n.telemetryClient.SegmentRequestFailed(eventID, err.Error())
		return fmt.Errorf("onSegmentRequest: sendQuicBytes proofs failed: %w", err)
	}

	// Telemetry: Segments transferred (event 178)
	n.telemetryClient.SegmentsTransferred(eventID)

	log.Trace(log.Node, "CE148-onSegmentRequest SENT",
		"node", n.id,
		"peerKey", peerKey,
		"numItems", len(req.Items),
		"totalSegments", req.TotalSegmentCount(),
	)

	return nil
}

// GetSegmentWithProof retrieves a segment and its import proof from the availability store
func (n *NodeContent) GetSegmentWithProof(segmentsRoot common.Hash, segmentIndex uint16) (segment []byte, importProof []common.Hash, found bool) {
	segments, ok := n.getSegmentsBySegmentRoot(segmentsRoot)
	if !ok {
		log.Trace(log.Node, "GetSegmentWithProof: segments not found",
			"segmentsRoot", segmentsRoot,
			"segmentIndex", segmentIndex)
		return nil, nil, false
	}
	if int(segmentIndex) >= len(segments) {
		log.Trace(log.Node, "GetSegmentWithProof: segment index out of range",
			"segmentsRoot", segmentsRoot,
			"segmentIndex", segmentIndex,
			"availableSegments", len(segments))
		return nil, nil, false
	}

	tree := trie.NewCDMerkleTree(segments)
	computedRoot := common.BytesToHash(tree.Root())
	if computedRoot != segmentsRoot {
		log.Warn(log.Node, "GetSegmentWithProof: segment root mismatch",
			"segmentsRoot", segmentsRoot,
			"computedRoot", computedRoot)
		return nil, nil, false
	}

	proof, err := tree.GenerateCDTJustificationX(int(segmentIndex), 0)
	if err != nil {
		log.Warn(log.Node, "GetSegmentWithProof: failed to generate justification",
			"segmentsRoot", segmentsRoot,
			"segmentIndex", segmentIndex,
			"err", err)
		return nil, nil, false
	}

	return segments[segmentIndex], proof, true
}

// getSegmentsBySegmentRoot retrieves the encoded segments for a given segments root
func (n *NodeContent) getSegmentsBySegmentRoot(segmentsRoot common.Hash) (segmentBytes [][]byte, ok bool) {
	segmentsKey := fmt.Sprintf("erasureSegments-%v", segmentsRoot)

	store, err := n.GetStorage()
	if err != nil {
		log.Warn(log.Node, "getSegmentsBySegmentRoot: storage not available", "err", err)
		return nil, false
	}

	segmentsData, found, err := store.ReadRawKV([]byte(segmentsKey))
	if err != nil {
		log.Warn(log.Node, "getSegmentsBySegmentRoot: ReadRawKV failed",
			"segmentsRoot", segmentsRoot,
			"err", err)
		return nil, false
	}
	if !found {
		log.Trace(log.Node, "getSegmentsBySegmentRoot: segments not found",
			"segmentsRoot", segmentsRoot)
		return nil, false
	}

	decoded, _, err := types.Decode(segmentsData, reflect.TypeOf([][]byte{}))
	if err != nil {
		log.Warn(log.Node, "getSegmentsBySegmentRoot: failed to decode segments",
			"segmentsRoot", segmentsRoot,
			"err", err)
		return nil, false
	}

	segments, ok := decoded.([][]byte)
	if !ok {
		log.Warn(log.Node, "getSegmentsBySegmentRoot: unexpected decoded type",
			"segmentsRoot", segmentsRoot,
			"decodedType", fmt.Sprintf("%T", decoded))
		return nil, false
	}

	return segments, true
}

// encodeImportProof encodes an import proof (list of hashes) to bytes
func encodeImportProof(proof []common.Hash) []byte {
	var data []byte

	// len++[Hash]
	data = append(data, types.E(uint64(len(proof)))...)
	for _, hash := range proof {
		data = append(data, hash.Bytes()...)
	}

	return data
}

