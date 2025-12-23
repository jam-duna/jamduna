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

	if err := sendQuicBytes(ctx, stream, stateBytes, p.SanKey(), code); err != nil {
		return fmt.Errorf("sendQuicBytes[CE151_GrandpaState] failed: %w", err)
	}

	return nil
}

func (n *Node) onGrandpaState(ctx context.Context, stream quic.Stream, msg []byte, peerKey string) error {
	defer stream.Close()

	// Decode: Round Number ++ Set Id ++ Slot
	var state grandpa.GrandpaStateMessage
	if err := state.FromBytes(msg); err != nil {
		return fmt.Errorf("onGrandpaState: decode failed: %w", err)
	}
	// Get peer to access its Ed25519 pubkey for grandpa tracking
	// We store pubkey instead of PeerID because PeerID becomes stale after epoch rotation
	peer, ok := n.peersByPubKey[peerKey]
	if !ok {
		return fmt.Errorf("onGrandpaState: could not find peer for key %s", peerKey)
	}
	n.grandpa.SetWhoRoundReady(state.SetId, state.Round, peer.Validator.Ed25519)
	select {
	case n.grandpa.StateMessageCh <- state:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("onGrandpaState: context canceled before delivery")
	}
}
