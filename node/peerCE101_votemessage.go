package node

import (
	"fmt"

	"github.com/colorfulnotion/jam/grandpa"
	"github.com/quic-go/quic-go"
)

// CE101_VoteMessage
func (p *Peer) SendVoteMessage(req grandpa.VoteMessage) error {
	reqBytes, err := req.ToBytes()
	if err != nil {
		return err
	}
	code := uint8(CE101_VoteMessage)
	stream, err := p.openStream(code)
	if err != nil {
		return err
	}
	defer stream.Close()

	err = sendQuicBytes(stream, reqBytes, p.PeerID, code)
	if err != nil {
		return err
	}
	return nil
}

func (n *Node) onVoteMessage(stream quic.Stream, msg []byte) error {
	defer stream.Close()
	vote := grandpa.VoteMessage{}
	err := vote.FromBytes(msg)
	if err != nil {
		return err
	}
	if vote.SignMessage.Message.Stage == grandpa.PrecommitStage {
		n.grandpaPreCommitMessageCh <- vote
	} else if vote.SignMessage.Message.Stage == grandpa.PrevoteStage {
		n.grandpaPreVoteMessageCh <- vote
	} else if vote.SignMessage.Message.Stage == grandpa.PrimaryProposeStage {
		n.grandpaPrimaryMessageCh <- vote
	} else {
		return fmt.Errorf("Invalid stage")
	}
	return nil
}
