package node

import (
	"context"
	"fmt"

	"github.com/colorfulnotion/jam/grandpa"
	"github.com/quic-go/quic-go"
)

/*
CE 150: GRANDPA Commit
This is sent by each voting validator to all other voting validators.

Validator -> Validator

--> Round Number ++ Set Id ++ Commit
--> FIN
<-- FIN
*/

func (p *Peer) SendCommitMessage(ctx context.Context, commit grandpa.CommitMessage) error {
	code := uint8(CE150_CommitMessage)
	stream, err := p.openStream(ctx, code)
	if err != nil {
		return fmt.Errorf("openStream[CE150_CommitMessage] failed: %w", err)
	}
	defer stream.Close()

	// Round Number ++ Set Id ++ Commit
	commitBytes, err := commit.ToBytes()
	if err != nil {
		return fmt.Errorf("CommitMessage.ToBytes failed: %w", err)
	}

	if err := sendQuicBytes(ctx, stream, commitBytes, p.PeerID, code); err != nil {
		return fmt.Errorf("sendQuicBytes[CE150_CommitMessage] failed: %w", err)
	}

	return nil
}

func (n *Node) onCommitMessage(ctx context.Context, stream quic.Stream, msg []byte) error {
	defer stream.Close()

	// Decode: Round Number ++ Set Id ++ Commit
	var commit grandpa.CommitMessage
	if err := commit.FromBytes(msg); err != nil {
		return fmt.Errorf("onCommitMessage: decode failed: %w", err)
	}

	select {
	case n.grandpaCommitMessageCh <- commit:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("onCommitMessage: context canceled before delivery")
	}
}
