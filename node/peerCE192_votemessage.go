package node

import (
	"context"
	"fmt"

	"github.com/colorfulnotion/jam/grandpa"
	"github.com/colorfulnotion/jam/log"
	"github.com/quic-go/quic-go"
)

// CE192_VoteMessage
func (p *Peer) SendVoteMessage(ctx context.Context, req grandpa.VoteMessage) error {
	reqBytes, err := req.ToBytes()
	if err != nil {
		return fmt.Errorf("SendVoteMessage: ToBytes failed: %w", err)
	}

	code := uint8(CE192_VoteMessage)
	stream, err := p.openStream(ctx, code)
	if err != nil {
		return fmt.Errorf("SendVoteMessage: openStream failed: %w", err)
	}
	defer stream.Close()

	if err := sendQuicBytes(ctx, stream, reqBytes, p.PeerID, code); err != nil {
		return fmt.Errorf("SendVoteMessage: sendQuicBytes failed: %w", err)
	}
	return nil
}

func (n *Node) onVoteMessage(ctx context.Context, stream quic.Stream, msg []byte) error {
	defer stream.Close()

	var vote grandpa.VoteMessage
	if err := vote.FromBytes(msg); err != nil {
		return fmt.Errorf("onVoteMessage: failed to decode vote: %w", err)
	}

	stage := vote.SignMessage.Message.Stage
	switch stage {
	case grandpa.PrecommitStage:
		select {
		case n.grandpaPreCommitMessageCh <- vote:
			// success
		case <-ctx.Done():
			return fmt.Errorf("onVoteMessage: context canceled while sending precommit")
		default:
			log.Warn(log.Node, "onVoteMessage: grandpaPreCommitMessageCh full, dropping vote")

		}

	case grandpa.PrevoteStage:
		select {
		case n.grandpaPreVoteMessageCh <- vote:
			// success
		case <-ctx.Done():
			return fmt.Errorf("onVoteMessage: context canceled while sending prevote")
		default:
			log.Warn(log.Node, "onVoteMessage: grandpaPreVoteMessageCh full, dropping vote")

		}

	case grandpa.PrimaryProposeStage:
		select {
		case n.grandpaPrimaryMessageCh <- vote:
			// success
		case <-ctx.Done():
			return fmt.Errorf("onVoteMessage: context canceled while sending primary propose")
		default:
			log.Warn(log.Node, "onVoteMessage: grandpaPrimaryMessageCh full, dropping vote")

		}

	default:
		return fmt.Errorf("onVoteMessage: invalid stage: %d", stage)
	}

	return nil
}
