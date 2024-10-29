package node

import (
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/quic-go/quic-go"
)

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
