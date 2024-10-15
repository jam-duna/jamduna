package node

import (
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
	"github.com/quic-go/quic-go"
)

/*
CE 142: Preimage announcement
Announcement of possession of a requested preimage.
This should be used by non-validator nodes to introduce preimages.

The recipient of the announcement is expected to follow up by requesting the preimage using protocol 143,
provided the preimage has been requested on chain and the recipient is not already in possession of it.

Preimage announcements should not be forwarded to other validators; validators should propagate preimages
only be including them in blocks they author.

Hash = [u8; 32]

Node -> Validator

--> Hash
--> FIN
<-- FIN
*/
func (p *Peer) SendPreimageAnnouncement(preimageHash common.Hash) (err error) {
	stream, err := p.openStream(CE142_PreimageAnnouncement)
	// --> Hash
	err = sendQuicBytes(stream, preimageHash.Bytes())
	if err != nil {
		return err
	}
	return nil
}

// TODO: William to review
func (n *Node) onPreimageAnnouncement(stream quic.Stream, msg []byte, peerID uint16) (err error) {
	preimageHash := common.BytesToHash(msg)
	preimageAnnouncement := types.PreimageAnnouncement{
		ValidatorIndex: peerID,
		PreimageHash:   preimageHash,
	}
	n.preimageAnnouncementsCh <- preimageAnnouncement
	return nil
}
