package node

import (
	"context"
	"fmt"

	"github.com/colorfulnotion/jam/grandpa"
	"github.com/quic-go/quic-go"
)

/*
CE 151: GRANDPA State
This is sent by each voting validator to all other voting validators.

Validator -> Validator

--> Round Number ++ Set Id ++ Slot
--> FIN
<-- FIN
*/

func (p *Peer) SendGrandpaState(ctx context.Context, state grandpa.GrandpaStateMessage) error {
	code := uint8(CE151_GrandpaState)
	stream, err := p.openStream(ctx, code)
	if err != nil {
		return fmt.Errorf("openStream[CE151_GrandpaState] failed: %w", err)
	}
	defer stream.Close()

	// Round Number ++ Set Id ++ Slot
	stateBytes, err := state.ToBytes()
	if err != nil {
		return fmt.Errorf("GrandpaStateMessage.ToBytes failed: %w", err)
	}

	if err := sendQuicBytes(ctx, stream, stateBytes, p.PeerID, code); err != nil {
		return fmt.Errorf("sendQuicBytes[CE151_GrandpaState] failed: %w", err)
	}

	return nil
}

func (n *Node) onGrandpaState(ctx context.Context, stream quic.Stream, msg []byte, peerId uint16) error {
	defer stream.Close()

	// Decode: Round Number ++ Set Id ++ Slot
	var state grandpa.GrandpaStateMessage
	if err := state.FromBytes(msg); err != nil {
		return fmt.Errorf("onGrandpaState: decode failed: %w", err)
	}
	n.grandpa.SetWhoRoundReady(state.SetId, state.Round, peerId)
	select {
	case n.grandpa.StateMessageCh <- state:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("onGrandpaState: context canceled before delivery")
	}
}
