package node

import (
	"context"
	"fmt"

	"github.com/colorfulnotion/jam/grandpa"
	"github.com/quic-go/quic-go"
)

func (p *Peer) SendCommitMessage(ctx context.Context, req grandpa.CommitMessage) error {
	reqBytes, err := req.ToBytes()
	if err != nil {
		return fmt.Errorf("CommitMessage.ToBytes failed: %w", err)
	}

	code := uint8(CE193_CommitMessage)
	stream, err := p.openStream(ctx, code)
	if err != nil {
		return fmt.Errorf("openStream[CE193_CommitMessage] failed: %w", err)
	}
	defer stream.Close()

	if err := sendQuicBytes(ctx, stream, reqBytes, p.PeerID, code); err != nil {
		return fmt.Errorf("sendQuicBytes[CE193_CommitMessage] failed: %w", err)
	}

	return nil
}

func (n *Node) onCommitMessage(ctx context.Context, stream quic.Stream, msg []byte) error {
	defer stream.Close()

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
