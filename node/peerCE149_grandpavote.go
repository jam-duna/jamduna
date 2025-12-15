package node

import (
	"context"
	"fmt"

	"github.com/colorfulnotion/jam/grandpa"
	"github.com/colorfulnotion/jam/log"
	"github.com/quic-go/quic-go"
)

// CE149_GrandpaVote
/*
This is sent by each voting validator to all other voting validators.

Validator -> Validator

--> Round Number ++ Set Id ++ Signed Message
--> FIN
<-- FIN
*/

func (p *Peer) SendGrandpaVote(ctx context.Context, req grandpa.GrandpaVote) error {
	reqBytes, err := req.ToBytes()
	if err != nil {
		return fmt.Errorf("SendGrandpaVote: ToBytes failed: %w", err)
	}

	code := uint8(CE149_GrandpaVote)
	stream, err := p.openStream(ctx, code)
	if err != nil {
		return fmt.Errorf("SendGrandpaVote: openStream failed: %w", err)
	}
	defer stream.Close()

	if err := sendQuicBytes(ctx, stream, reqBytes, p.Validator.Ed25519.String(), code); err != nil {
		return fmt.Errorf("SendGrandpaVote: sendQuicBytes failed: %w", err)
	}
	return nil
}

func (n *Node) onGrandpaVote(ctx context.Context, stream quic.Stream, msg []byte) error {
	defer stream.Close()
	var vote grandpa.GrandpaVote
	if err := vote.FromBytes(msg); err != nil {
		return fmt.Errorf("onGrandpaVote: failed to decode vote: %w", err)
	}

	stage := vote.SignedMessage.Message.Stage
	// log.Info(log.Node, fmt.Sprintf("onGrandpaVote: received %v from peer", vote.GetVoteType()), "n", n.String(), "stage", stage, "vote", vote.String())
	switch stage {
	case grandpa.PrecommitStage:
		select {
		case n.grandpa.PreCommitMessageCh <- vote:
			// success
		case <-ctx.Done():
			return fmt.Errorf("onGrandpaVote: context canceled while sending precommit")
		default:
			log.Warn(log.Node, "onGrandpaVote: PreCommitMessageCh full, dropping vote")

		}

	case grandpa.PrevoteStage:
		select {
		case n.grandpa.PreVoteMessageCh <- vote:
			// success
		case <-ctx.Done():
			return fmt.Errorf("onGrandpaVote: context canceled while sending prevote")
		default:
			log.Warn(log.Node, "onGrandpaVote: PreVoteMessageCh full, dropping vote")

		}

	case grandpa.PrimaryProposeStage:
		select {
		case n.grandpa.PrimaryMessageCh <- vote:
			// success
		case <-ctx.Done():
			return fmt.Errorf("onGrandpaVote: context canceled while sending primary propose")
		default:
			log.Warn(log.Node, "onGrandpaVote: PrimaryMessageCh full, dropping vote")

		}

	default:
		return fmt.Errorf("onGrandpaVote: invalid stage: %d", stage)
	}

	return nil
}
