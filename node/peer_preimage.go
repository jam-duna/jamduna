package node

import (
	"github.com/colorfulnotion/jam/common"
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
	p.sendCode(CE142_PreimageAnnouncement)
	// --> Hash
	err = p.sendQuicBytes(preimageHash.Bytes())
	if err != nil {
		return err
	}
	// --> FIN
	p.sendFIN()
	// <-- FIN
	p.receiveFIN()
	return nil
}

func (p *Peer) processPreimageAnnouncement(msg []byte) (err error) {
	preimageHash := common.BytesToHash(msg)
	p.node.OnPreimageAnnouncement(p.validatorIndex, preimageHash)
	return nil
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
	p.sendCode(CE143_PreimageRequest)
	err = p.sendQuicBytes(preimageHash.Bytes())
	if err != nil {
		return preimage, err
	}
	preimage, err = p.receiveQuicBytes()
	if err != nil {
		return preimage, err
	}

	// --> FIN
	p.sendFIN()
	// <-- FIN
	p.receiveFIN()

	return preimage, nil
}

func (p *Peer) processPreimageRequest(msg []byte) (err error) {
	// --> Hash
	preimageHash := common.BytesToHash(msg)
	preimage, ok, err := p.node.PreimageLookup(preimageHash)
	if err != nil {
		return err
	}
	if !ok {
		// TODO
	}
	err = p.sendQuicBytes(preimage)
	if err != nil {
		return err
	}
	// --> FIN
	p.receiveFIN()
	// <-- FIN
	p.sendFIN()

	return nil
}
