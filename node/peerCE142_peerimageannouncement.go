package node

import (
	"context"
	"fmt"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
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

func (n *NodeContent) BroadcastPreimageAnnouncement(serviceID uint32, preimageHash common.Hash, preimageLen uint32, preimage []byte) (err error) {
	pa := types.PreimageAnnouncement{
		ServiceIndex: serviceID,
		PreimageHash: preimageHash,
		PreimageLen:  preimageLen,
	}

	err = n.AddPreimageToPool(serviceID, preimage)
	if err != nil {
		log.Warn(log.P, "BroadcastPreimageAnnouncement:AddPreimageToPool", "err", err, "serviceID", serviceID, "len", len(preimage), "h", common.Blake2Hash(preimage))
		return
	}
	log.Trace(log.Node, "BroadcastPreimageAnnouncement:AddPreimageToPool", "n", n.String(), "p", pa.String())

	ctx, cancel := context.WithTimeout(context.Background(), MediumTimeout)
	defer cancel()                // ensures context is released
	n.nodeSelf.broadcast(ctx, pa) // CE142
	return nil
}

func (p *Peer) SendPreimageAnnouncement(ctx context.Context, pa *types.PreimageAnnouncement) error {
	code := uint8(CE142_PreimageAnnouncement)
	p.AddKnownHash(pa.PreimageHash)

	// Telemetry: Preimage announced (event 191) - sending side
	connectionSide := byte(0) // Local is announcer
	p.node.telemetryClient.PreimageAnnounced(p.PeerKey(), connectionSide, pa.ServiceIndex, pa.PreimageHash, pa.PreimageLen)

	stream, err := p.openStream(ctx, code)
	if err != nil {
		// Telemetry: Preimage announcement failed (event 190)
		p.node.telemetryClient.PreimageAnnouncementFailed(p.PeerKey(), connectionSide, err.Error())
		return fmt.Errorf("openStream[CE142_PreimageAnnouncement]: %w", err)
	}
	defer stream.Close()

	paBytes, err := pa.ToBytes()
	if err != nil {
		// Telemetry: Preimage announcement failed (event 190)
		p.node.telemetryClient.PreimageAnnouncementFailed(p.PeerKey(), connectionSide, err.Error())
		return fmt.Errorf("ToBytes[CE142_PreimageAnnouncement]: %w", err)
	}

	if err := sendQuicBytes(ctx, stream, paBytes, p.Validator.Ed25519.String(), code); err != nil {
		// Telemetry: Preimage announcement failed (event 190)
		p.node.telemetryClient.PreimageAnnouncementFailed(p.PeerKey(), connectionSide, err.Error())
		return fmt.Errorf("sendQuicBytes[CE142_PreimageAnnouncement]: %w", err)
	}

	return nil
}

func (n *Node) onPreimageAnnouncement(ctx context.Context, stream quic.Stream, msg []byte, peerKey string) error {
	defer stream.Close()

	// Telemetry: Preimage announced (event 191) - receiving side
	connectionSide := byte(1) // Remote is announcer

	// Get peer to access its PeerID for telemetry
	p, ok := n.peersByPubKey[peerKey]
	if !ok {
		return fmt.Errorf("onPreimageAnnouncement: peer not found for key %s", peerKey)
	}

	var preimageAnnouncement types.PreimageAnnouncement
	if err := preimageAnnouncement.FromBytes(msg); err != nil {
		log.Error(log.P, "onPreimageAnnouncement: failed to decode", "err", err)
		// Telemetry: Preimage announcement failed (event 190)
		n.telemetryClient.PreimageAnnouncementFailed(PubkeyBytes(p.Validator.Ed25519.String()), connectionSide, err.Error())
		stream.CancelRead(ErrInvalidData)
		return fmt.Errorf("onPreimageAnnouncement: decode failed: %w", err)
	}

	// Telemetry: Preimage announced (event 191)
	n.telemetryClient.PreimageAnnounced(PubkeyBytes(p.Validator.Ed25519.String()), connectionSide, preimageAnnouncement.ServiceIndex, preimageAnnouncement.PreimageHash, preimageAnnouncement.PreimageLen)

	serviceIndex := preimageAnnouncement.ServiceIndex
	preimageHash := preimageAnnouncement.PreimageHash

	preimage, err := p.SendPreimageRequest(ctx, preimageHash)
	if err != nil {
		log.Warn(log.P, "SendPreimageRequest failed", "err", err)
		return fmt.Errorf("SendPreimageRequest failed: %w", err)
	}

	err = n.AddPreimageToPool(serviceIndex, preimage)
	if err != nil {
		log.Warn(log.P, "processPreimageAnnouncements:AddPreimageToPool", "err", err, "serviceID", serviceIndex, "len", len(preimage), "h", common.Blake2Hash(preimage))
		return err
	}
	log.Trace(log.Node, "BroadcastPreimageAnnouncement:AddPreimageToPool", "n", n.String(), "serviceID", serviceIndex, "len", len(preimage), "h", common.Blake2Hash(preimage))
	// received a preimage announcement, mark it as known
	n.peersByPubKey[peerKey].AddKnownHash(preimageAnnouncement.PreimageHash)
	// broadcast the preimage announcement to other peers
	go n.broadcast(ctx, preimageAnnouncement)

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
func (p *Peer) SendPreimageRequest(ctx context.Context, preimageHash common.Hash) ([]byte, error) {
	code := uint8(CE143_PreimageRequest)

	// Telemetry: Sending preimage request (event 193)
	eventID := p.node.telemetryClient.GetEventID()
	p.node.telemetryClient.SendingPreimageRequest(p.PeerKey(), preimageHash)

	stream, err := p.openStream(ctx, code)
	if err != nil {
		// Telemetry: Preimage request failed (event 195)
		p.node.telemetryClient.PreimageRequestFailed(eventID, err.Error())
		return nil, fmt.Errorf("openStream[CE143_PreimageRequest]: %w", err)
	}

	// --> Hash
	respBytes := preimageHash.Bytes()
	if err := sendQuicBytes(ctx, stream, respBytes, p.Validator.Ed25519.String(), code); err != nil {
		// Telemetry: Preimage request failed (event 195)
		p.node.telemetryClient.PreimageRequestFailed(eventID, err.Error())
		return nil, fmt.Errorf("sendQuicBytes[CE143_PreimageRequest]: %w", err)
	}

	// Telemetry: Preimage request sent (event 196)
	p.node.telemetryClient.PreimageRequestSent(eventID)

	// --> FIN
	stream.Close()

	preimage, err := receiveQuicBytes(ctx, stream, p.Validator.Ed25519.String(), code)
	if err != nil {
		// Telemetry: Preimage request failed (event 195)
		p.node.telemetryClient.PreimageRequestFailed(eventID, err.Error())
		return nil, fmt.Errorf("receiveQuicBytes[CE143_PreimageRequest]: %w", err)
	}

	// Telemetry: Preimage transferred (event 198)
	p.node.telemetryClient.PreimageTransferred(eventID, uint32(len(preimage)))

	return preimage, nil
}

func (n *NodeContent) onPreimageRequest(ctx context.Context, stream quic.Stream, msg []byte) error {
	defer stream.Close()

	// Telemetry: Receiving preimage request (event 194)
	// Note: peerID not available in this context, using zero hash as placeholder
	eventID := n.telemetryClient.GetEventID()
	var zeroPeerID [32]byte
	n.telemetryClient.ReceivingPreimageRequest(zeroPeerID)

	preimageHash := common.BytesToHash(msg)

	// Telemetry: Preimage request received (event 197)
	n.telemetryClient.PreimageRequestReceived(eventID, preimageHash)

	preimage, ok := n.extrinsic_pool.GetPreimageByHash(preimageHash)
	if !ok {
		log.Warn(log.P, "onPreimageRequest", "n", n.id, "hash", preimageHash, "msg", "preimage not found")
		// Telemetry: Preimage request failed (event 195)
		n.telemetryClient.PreimageRequestFailed(eventID, "preimage not found")
		stream.CancelWrite(ErrKeyNotFound)
		return nil
	}

	code := uint8(CE143_PreimageRequest)

	respBytes := preimage.Blob
	if err := sendQuicBytes(ctx, stream, respBytes, n.GetEd25519Key().String(), code); err != nil {
		stream.CancelWrite(ErrCECode)
		// Telemetry: Preimage request failed (event 195)
		n.telemetryClient.PreimageRequestFailed(eventID, err.Error())
		return fmt.Errorf("onPreimageRequest: sendQuicBytes failed: %w", err)
	}

	// Telemetry: Preimage transferred (event 198)
	n.telemetryClient.PreimageTransferred(eventID, uint32(len(preimage.Blob)))

	log.Trace(log.P, "onPreimageRequest", "n", n.id, "hash", preimageHash, "size", len(preimage.Blob))
	return nil
}
