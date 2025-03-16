package node

import (
	"fmt"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
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

func (n *Node) BroadcastPreimageAnnouncement(serviceID uint32, preimageHash common.Hash, preimageLen uint32, preimage []byte) (err error) {
	pa := types.PreimageAnnouncement{
		ServiceIndex: serviceID,
		PreimageHash: preimageHash,
		PreimageLen:  preimageLen,
	}
	n.preimagesMutex.Lock()
	n.preimages[preimageHash] = preimage
	n.preimagesMutex.Unlock()
	preimageLookup := types.Preimages{
		Requester: uint32(serviceID),
		Blob:      preimage,
	}

	log.Debug(debugP, "BroadcastPreimageAnnouncement", "n", n.String(), "p", pa.String())
	n.processPreimage(preimageLookup)

	go n.broadcast(pa)
	return nil
}

func (n *Node) runPreimages() {
	// ticker here to avoid high CPU usage
	pulseTicker := time.NewTicker(20 * time.Millisecond)
	defer pulseTicker.Stop()

	for {
		select {
		case <-pulseTicker.C:
			// Small pause to reduce CPU load when channels are quiet
		case preimageAnnouncement := <-n.preimageAnnouncementsCh:
			err := n.processPreimageAnnouncements(preimageAnnouncement)
			if err != nil {
				fmt.Printf("%s processPreimages: %v\n", n.String(), err)
			}
		}
	}
}

func (n *Node) processPreimageAnnouncements(preimageAnnouncement types.PreimageAnnouncement) (err error) {
	// initiate CE143_PreimageRequest
	validatorIndex := preimageAnnouncement.ValidatorIndex
	p, ok := n.peersInfo[validatorIndex]
	if !ok {
		return fmt.Errorf("Invalid validator index %d", validatorIndex)
	}
	serviceIndex := preimageAnnouncement.ServiceIndex
	preimageHash := preimageAnnouncement.PreimageHash

	log.Debug(debugP, "processPreimageAnnouncements", "n", n.String(), "validatorIndex", validatorIndex, "serviceIndex", serviceIndex, "preimageHash", preimageHash)

	preimage, err := p.SendPreimageRequest(preimageAnnouncement.PreimageHash)
	if err != nil {
		return err
	}

	lookup := types.Preimages{
		Requester: uint32(serviceIndex),
		Blob:      preimage,
	}
	n.processPreimage(lookup)

	return nil
}

func (p *Peer) SendPreimageAnnouncement(pa *types.PreimageAnnouncement) (err error) {
	stream, err := p.openStream(CE142_PreimageAnnouncement)
	if err != nil {
		return err
	}
	defer stream.Close()
	// --> Service ID ++ Hash ++ Preimage Length
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
		log.Error(debugP, "onPreimageAnnouncement", "err", err)
		return
	}
	preimageAnnouncement.ValidatorIndex = peerID
	log.Trace(debugP, "onPreimageAnnouncement", "n", n.String(), "peerID", peerID, "p", preimageAnnouncement.String())
	return n.processPreimageAnnouncements(preimageAnnouncement)

}

/*
CE 143: Preimage request
Request for a preimage of the given hash.

This should be used to request:

Preimages announced via protocol 142.
Missing preimages of hashes in the lookup extrinsics of new blocks.
Requests for a preimage should be made to nodes that have announced possession of either the preimage itself or of a block containing the hash of the preimage in its lookup extrinsic.

Note that this protocol is essentially the same as protocol 136 (work-report request), but the hash is expected to be checked against a different database
(in the case of this protocol, the preimage lookup database).

Hash = [u8; 32]
Preimage = [u8]

Node -> Node

--> Hash
--> FIN
<-- Preimage
<-- FIN
*/
func (p *Peer) SendPreimageRequest(preimageHash common.Hash) (preimage []byte, err error) {
	stream, err := p.openStream(CE143_PreimageRequest)
	err = sendQuicBytes(stream, preimageHash.Bytes())
	if err != nil {
		return preimage, err
	}
	time.Sleep(10 * time.Second)
	preimage, err = receiveQuicBytes(stream)
	if err != nil {
		return preimage, err
	}
	return preimage, nil
}

func (n *Node) onPreimageRequest(stream quic.Stream, msg []byte) (err error) {
	defer stream.Close()
	// --> Hash
	preimageHash := common.BytesToHash(msg)
	preimage, ok, err := n.PreimageLookup(preimageHash)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	err = sendQuicBytes(stream, preimage)
	if err != nil {
		return err
	}
	// <-- FIN
	return nil
}
