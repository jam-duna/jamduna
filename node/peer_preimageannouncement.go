package node

import (
	"fmt"

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

Service ID = u32
Preimage Length = u32

Node -> Validator

--> Service ID ++ Hash ++ Preimage Length
--> FIN
<-- FIN
*/

func (p *Peer) SendPreimageAnnouncement(serviceID uint32, preimageHash common.Hash, preimageLen uint32) (err error) {
	stream, err := p.openStream(CE142_PreimageAnnouncement)
	// --> Service ID ++ Hash ++ Preimage Length
	pa := types.PreimageAnnouncement{
		ServiceIndex: serviceID,
		PreimageHash: preimageHash,
		PreimageLen:  preimageLen,
	}
	// TODO: encode it
	paBytes, _ := pa.ToBytes()
	err = sendQuicBytes(stream, paBytes)
	if err != nil {
		return err
	}
	return nil
}

func (n *Node) onPreimageAnnouncement(stream quic.Stream, msg []byte, peerID uint16) (err error) {
	defer stream.Close()
	var preimageAnnouncement types.PreimageAnnouncement
	err = preimageAnnouncement.FromBytes(msg)
	if err != nil {
		fmt.Println("Error deserializing:", err)
		return
	}
	preimageAnnouncement.ValidatorIndex = peerID
	if debugP {
		fmt.Printf("%s received PreimageAnnouncement from %d (%s)\n", n.String(), peerID, preimageAnnouncement.String())
	}
	return n.processPreimageAnnouncements(preimageAnnouncement)

}
