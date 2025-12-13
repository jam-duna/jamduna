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

// SegmentRequest represents a CE 148 segment request
type SegmentRequest struct {
	SegmentsRoot   common.Hash // 32 bytes
	SegmentIndices []uint16    // Variable length
}

// ToBytes encodes a SegmentRequest to bytes
func (r *SegmentRequest) ToBytes() []byte {
	var data []byte

	// Segments-Root (32 bytes)
	data = append(data, r.SegmentsRoot.Bytes()...)

	// len++[Segment Index]
	data = append(data, types.E(uint64(len(r.SegmentIndices)))...)
	for _, idx := range r.SegmentIndices {
		data = append(data, telemetry.Uint16ToBytes(idx)...)
	}

	return data
}

// FromBytes decodes a SegmentRequest from bytes
func (r *SegmentRequest) FromBytes(data []byte) error {
	if len(data) < 32 {
		return fmt.Errorf("invalid segment request: too short")
	}

	// Segments-Root (32 bytes)
	r.SegmentsRoot = common.BytesToHash(data[:32])
	data = data[32:]

	// len++[Segment Index]
	numIndices, bytesRead := types.DecodeE(data)
	if bytesRead == 0 {
		return fmt.Errorf("invalid segment request: failed to decode segment count")
	}
	data = data[bytesRead:]

	if numIndices > types.MaxExports {
		return fmt.Errorf("invalid segment request: too many segments requested (%d > %d)", numIndices, types.MaxExports)
	}

	r.SegmentIndices = make([]uint16, numIndices)
	for i := uint64(0); i < numIndices; i++ {
		if len(data) < 2 {
			return fmt.Errorf("invalid segment request: insufficient data for segment index %d", i)
		}
		r.SegmentIndices[i] = binary.LittleEndian.Uint16(data[:2])
		data = data[2:]
	}

	return nil
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

// SendSegmentRequest sends a segment request to a peer (requester side)
func (p *Peer) SendSegmentRequest(ctx context.Context, segmentsRoot common.Hash, segmentIndices []uint16, eventID uint64) ([]SegmentResponse, error) {
	code := uint8(CE148_SegmentRequest)

	// Validate request
	if len(segmentIndices) == 0 {
		return nil, fmt.Errorf("no segment indices provided")
	}
	if len(segmentIndices) > types.MaxExports {
		return nil, fmt.Errorf("too many segments requested: %d > %d", len(segmentIndices), types.MaxExports)
	}

	// Telemetry: Sending segment request (event 173)
	p.node.telemetryClient.SendingSegmentRequest(eventID, p.GetPeer32(), segmentIndices)

	stream, err := p.openStream(ctx, code)
	if err != nil {
		// Telemetry: Segment request failed (event 175)
		p.node.telemetryClient.SegmentRequestFailed(eventID, err.Error())
		return nil, fmt.Errorf("openStream[CE148_SegmentRequest]: %w", err)
	}

	// Build request
	req := SegmentRequest{
		SegmentsRoot:   segmentsRoot,
		SegmentIndices: segmentIndices,
	}

	// --> [Segments-Root ++ len++[Segment Index]]
	reqBytes := req.ToBytes()
	if err := sendQuicBytes(ctx, stream, reqBytes, p.PeerID, code); err != nil {
		// Telemetry: Segment request failed (event 175)
		p.node.telemetryClient.SegmentRequestFailed(eventID, err.Error())
		return nil, fmt.Errorf("sendQuicBytes[CE148_SegmentRequest]: %w", err)
	}

	// Telemetry: Segment request sent (event 176)
	p.node.telemetryClient.SegmentRequestSent(eventID)

	// --> FIN
	stream.Close()

	// <-- [Segment] - all segments concatenated in single message
	allSegmentsBytes, err := receiveQuicBytes(ctx, stream, p.PeerID, code)
	if err != nil {
		p.node.telemetryClient.SegmentRequestFailed(eventID, err.Error())
		return nil, fmt.Errorf("receiveQuicBytes[CE148_SegmentRequest] segments: %w", err)
	}

	// <-- [Import-Proof] - all import proofs concatenated in single message
	allProofsBytes, err := receiveQuicBytes(ctx, stream, p.PeerID, code)
	if err != nil {
		p.node.telemetryClient.SegmentRequestFailed(eventID, err.Error())
		return nil, fmt.Errorf("receiveQuicBytes[CE148_SegmentRequest] proofs: %w", err)
	}

	// Parse segments - split by SegmentSize (4104 bytes)
	responses := make([]SegmentResponse, 0, len(segmentIndices))
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

	log.Trace(log.Node, "CE148-SendSegmentRequest",
		"node", p.node.id,
		"peer", p.PeerID,
		"segmentsRoot", segmentsRoot,
		"numRequested", len(segmentIndices),
		"numReceived", len(responses),
	)

	return responses, nil
}

// onSegmentRequest handles incoming segment requests (responder side)
func (n *Node) onSegmentRequest(ctx context.Context, stream quic.Stream, msg []byte, peerID uint16) error {
	defer stream.Close()

	// Telemetry: Receiving segment request (event 174)
	eventID := n.telemetryClient.GetEventID()
	n.telemetryClient.ReceivingSegmentRequest(n.PeerID32(peerID))

	// Parse request
	var req SegmentRequest
	if err := req.FromBytes(msg); err != nil {
		log.Error(log.Node, "onSegmentRequest: failed to decode", "err", err)
		// Telemetry: Segment request failed (event 175)
		n.telemetryClient.SegmentRequestFailed(eventID, err.Error())
		return fmt.Errorf("onSegmentRequest: decode failed: %w", err)
	}

	// Telemetry: Segment request received (event 177)
	n.telemetryClient.SegmentRequestReceived(eventID, uint16(len(req.SegmentIndices)))

	log.Trace(log.Node, "CE148-onSegmentRequest INCOMING",
		"node", n.id,
		"peer", peerID,
		"segmentsRoot", req.SegmentsRoot,
		"numSegments", len(req.SegmentIndices),
	)

	code := uint8(CE148_SegmentRequest)

	segments, ok := n.getSegmentsBySegmentRoot(req.SegmentsRoot)
	if !ok {
		// Telemetry: Segment request failed (event 175)
		n.telemetryClient.SegmentRequestFailed(eventID, "segments not found")
		return fmt.Errorf("onSegmentRequest: segments not found for root %s", req.SegmentsRoot.Hex())
	}

	segmentTree := trie.NewCDMerkleTree(segments)
	computedRoot := common.BytesToHash(segmentTree.Root())
	if computedRoot != req.SegmentsRoot {
		err := fmt.Errorf("segment root mismatch: expected %s got %s", req.SegmentsRoot.Hex(), computedRoot.Hex())
		n.telemetryClient.SegmentRequestFailed(eventID, err.Error())
		return err
	}

	// Collect all segments and proofs first
	var allSegments []byte
	var allImportProofs []byte

	for _, segmentIdx := range req.SegmentIndices {
		if int(segmentIdx) >= len(segments) {
			log.Warn(log.Node, "onSegmentRequest: segment index out of range",
				"segmentsRoot", req.SegmentsRoot,
				"segmentIndex", segmentIdx,
				"availableSegments", len(segments))
			n.telemetryClient.SegmentRequestFailed(eventID, fmt.Sprintf("segment %d out of range", segmentIdx))
			return fmt.Errorf("onSegmentRequest: segment %d out of range", segmentIdx)
		}

		proof, err := segmentTree.GenerateCDTJustificationX(int(segmentIdx), 0)
		if err != nil {
			log.Warn(log.Node, "onSegmentRequest: failed to generate justification",
				"segmentsRoot", req.SegmentsRoot,
				"segmentIndex", segmentIdx,
				"err", err)
			n.telemetryClient.SegmentRequestFailed(eventID, err.Error())
			return fmt.Errorf("onSegmentRequest: failed to generate justification for segment %d: %w", segmentIdx, err)
		}

		allSegments = append(allSegments, segments[segmentIdx]...)
		allImportProofs = append(allImportProofs, encodeImportProof(proof)...)
	}

	// <-- [Segment] - all segments concatenated in single message
	if err := sendQuicBytes(ctx, stream, allSegments, n.id, code); err != nil {
		stream.CancelWrite(ErrCECode)
		n.telemetryClient.SegmentRequestFailed(eventID, err.Error())
		return fmt.Errorf("onSegmentRequest: sendQuicBytes segments failed: %w", err)
	}

	// <-- [Import-Proof] - all import proofs concatenated in single message
	if err := sendQuicBytes(ctx, stream, allImportProofs, n.id, code); err != nil {
		stream.CancelWrite(ErrCECode)
		n.telemetryClient.SegmentRequestFailed(eventID, err.Error())
		return fmt.Errorf("onSegmentRequest: sendQuicBytes proofs failed: %w", err)
	}

	// Telemetry: Segments transferred (event 178)
	n.telemetryClient.SegmentsTransferred(eventID)

	log.Trace(log.Node, "CE148-onSegmentRequest SENT",
		"node", n.id,
		"peer", peerID,
		"segmentsRoot", req.SegmentsRoot,
		"numSegments", len(req.SegmentIndices),
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

