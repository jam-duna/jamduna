package node

import (
	"github.com/colorfulnotion/jam/grandpa"
	"github.com/quic-go/quic-go"
)

func (p *Peer) SendCommitMessage(req grandpa.CommitMessage) error {
	reqBytes, err := req.ToBytes()
	if err != nil {
		return err
	}
	code := uint8(CE102_CommitMessage)
	stream, err := p.openStream(code)
	if err != nil {
		return err
	}
	defer stream.Close()
	if err != nil {
		return err
	}
	err = sendQuicBytes(stream, reqBytes, p.PeerID, code)
	if err != nil {
		return err
	}
	return nil
}

func (n *Node) onCommitMessage(stream quic.Stream, msg []byte) error {
	defer stream.Close()
	commit := grandpa.CommitMessage{}
	err := commit.FromBytes(msg)
	if err != nil {
		return err
	}
	n.grandpaCommitMessageCh <- commit
	return nil
}
