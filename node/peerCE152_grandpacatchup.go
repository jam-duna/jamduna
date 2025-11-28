package node

import (
	"context"
	"fmt"

	"github.com/colorfulnotion/jam/grandpa"
	"github.com/quic-go/quic-go"
)

/*
CE 152: GRANDPA CatchUp
Catchup Request. This is sent by a voting validator to another validator if the first validator determines that it is behind the other validator by a
threshold number of rounds (currently set to 2 rounds).

The response includes all votes from the last completed round of the responding validator.

Base Hash and Base Number refer to the base, which is a block all vote targets are a descendent of.

If the responding voter is unable to send the response it should stop the stream.

Base Hash = Header Hash
Base Number = Slot
Catchup = Round Number ++ len++[Signed Prevote] ++ len++[Signed Precommit] ++ Base Hash ++ Base Number

Validator -> Validator

--> Round Number ++ Set Id
--> FIN
<-- Catchup
<-- FIN

*/

func (p *Peer) SendGrandpaCatchUp(ctx context.Context, catchup grandpa.GrandpaCatchUp) (*grandpa.CatchUpResponse, error) {
	code := uint8(CE152_GrandpaCatchUp)
	stream, err := p.openStream(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("openStream[CE152_GrandpaCatchUp] failed: %w", err)
	}
	defer stream.Close()

	// Send: Round Number ++ Set Id ++ Slot
	requestBytes, err := catchup.ToBytes()
	if err != nil {
		return nil, fmt.Errorf("GrandpaCatchUp.ToBytes failed: %w", err)
	}

	// Send request
	if err := sendQuicBytes(ctx, stream, requestBytes, p.PeerID, code); err != nil {
		return nil, fmt.Errorf("sendQuicBytes[CE152_GrandpaCatchUp] request failed: %w", err)
	}

	// Receive response: Catchup
	responseBytes, err := receiveQuicBytes(ctx, stream, p.PeerID, code)
	if err != nil {
		return nil, fmt.Errorf("receiveQuicBytes[CE152_GrandpaCatchUp] response failed: %w", err)
	}

	// Decode response
	var response grandpa.CatchUpResponse
	if err := response.FromBytes(responseBytes); err != nil {
		return nil, fmt.Errorf("failed to decode catchup response: %w", err)
	}

	// TODO: Process the catch-up response
	// The Peer struct doesn't have direct access to the node
	// This should be handled at a higher level
	_ = response
	return &response, nil
}

func (n *Node) onGrandpaCatchUp(ctx context.Context, stream quic.Stream, msg []byte, peerID uint16) error {
	defer stream.Close()

	// Decode: Round Number ++ Set Id ++ Slot
	var catchup grandpa.GrandpaCatchUp
	if err := catchup.FromBytes(msg); err != nil {
		return fmt.Errorf("onGrandpaCatchUp: decode failed: %w", err)
	}

	// Get catchup data from GRANDPA state
	responseBytes, ok, err := n.store.GetCatchupMassage(catchup.Round, catchup.SetId)
	if err != nil {
		return fmt.Errorf("failed to get catchup data: %w", err)
	}
	if !ok {
		return fmt.Errorf("no catchup data found for round %d, set %d", catchup.Round, catchup.SetId)
	}
	// Send response: Catchup
	if err := sendQuicBytes(ctx, stream, responseBytes, peerID, uint8(CE152_GrandpaCatchUp)); err != nil {
		return fmt.Errorf("sendQuicBytes[CE152_GrandpaCatchUp] response failed: %w", err)
	}

	return nil
}
