package node

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/colorfulnotion/jam/types"
	"github.com/quic-go/quic-go"
)

/*
CE 153: Warp Sync Request
A request for a number of warp sync fragments starting at (and including) a given set id.
The responding node should return as many sequential warp sync fragments as it can until the latest set has been reached or the message size is too big. If the responding node is unable to respond it should stop the stream.

Node -> Node

--> Set Id
--> FIN
<-- len++[Warp Sync Fragment]
<-- FIN
*/

func (p *Peer) SendWarpSyncRequest(ctx context.Context, req uint32) (types.WarpSyncResponse, error) {
	code := uint8(CE153_WarpSyncRequest)
	stream, err := p.openStream(ctx, code)
	if err != nil {
		return types.WarpSyncResponse{}, fmt.Errorf("openStream[CE153_WarpSyncRequest] failed: %w", err)
	}
	defer stream.Close()

	// Send: Set Id (uint32 in little-endian)
	reqBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(reqBytes, req)

	if err := sendQuicBytes(ctx, stream, reqBytes, p.SanKey(), code); err != nil {
		return types.WarpSyncResponse{}, fmt.Errorf("sendQuicBytes[CE153_WarpSyncRequest] failed: %w", err)
	}

	// Receive: Catchup
	respBytes, err := receiveQuicBytes(ctx, stream, p.SanKey(), code)
	if err != nil {
		return types.WarpSyncResponse{}, fmt.Errorf("receiveQuicBytes[CE153_WarpSyncRequest] failed: %w", err)
	}

	var response types.WarpSyncResponse
	if err := response.FromBytes(respBytes); err != nil {
		return types.WarpSyncResponse{}, fmt.Errorf("WarpSyncResponse.FromBytes failed: %w", err)
	}

	return response, nil
}

func (n *Node) onWarpSyncRequest(ctx context.Context, stream quic.Stream, msg []byte, peerKey string) error {
	defer stream.Close()

	// Decode: Set Id (uint32 in little-endian)
	if len(msg) < 4 {
		stream.CancelWrite(ErrInvalidData)
		return fmt.Errorf("onWarpSyncRequest: invalid message length: expected at least 4 bytes, got %d", len(msg))
	}
	req := binary.LittleEndian.Uint32(msg[:4])

	// Build the WarpSyncResponse using the node's GRANDPA state
	grandpa := n.grandpa.GetOrInitializeGrandpa(n.grandpa.CurrentSetID)
	response, err := grandpa.GetWarpSyncResponse(req)
	if err != nil {
		// If we can't get fragments, return empty response instead of error
		response = types.WarpSyncResponse{Fragments: []types.WarpSyncFragment{}}
	}

	// Send response
	respBytes, err := response.ToBytes()
	if err != nil {
		stream.CancelWrite(ErrInvalidData)
		return fmt.Errorf("onWarpSyncRequest: response.ToBytes failed: %w", err)
	}

	code := uint8(CE153_WarpSyncRequest)
	if err := sendQuicBytes(ctx, stream, respBytes, n.GetEd25519Key().SAN(), code); err != nil {
		stream.CancelWrite(ErrCECode)
		return fmt.Errorf("onWarpSyncRequest: sendQuicBytes failed: %w", err)
	}

	return nil
}
