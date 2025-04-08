package node

import (
	"context"
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

	n.StoreImage(preimageHash, preimage)

	preimageLookup := types.Preimages{
		Requester: uint32(serviceID),
		Blob:      preimage,
	}

	log.Debug(debugP, "BroadcastPreimageAnnouncement", "n", n.String(), "p", pa.String())
	n.processPreimage(preimageLookup)

	ctx, cancel := context.WithTimeout(context.Background(), MediumTimeout)
	go func() {
		defer cancel() // ensures context is released
		_ = n.broadcast(ctx, pa)
	}()
	return nil
}

func (n *NodeContent) StoreImage(preimageHash common.Hash, preimage []byte) {
	n.preimagesMutex.Lock()
	defer n.preimagesMutex.Unlock()
	n.preimages[preimageHash] = preimage
	return
}

func (n *Node) runPreimages() {
	pulseTicker := time.NewTicker(20 * time.Millisecond)
	defer pulseTicker.Stop()

	for {
		select {
		case <-pulseTicker.C:
			// avoid spinning

		case preimageAnnouncement := <-n.preimageAnnouncementsCh:
			// 3s timeout for each announcement
			ctx, cancel := context.WithTimeout(context.Background(), SmallTimeout)

			err := n.processPreimageAnnouncements(ctx, preimageAnnouncement)
			if err != nil {
				fmt.Printf("%s processPreimages: %v\n", n.String(), err)
			}
			cancel()
		}
	}
}

func (n *Node) processPreimageAnnouncements(ctx context.Context, preimageAnnouncement types.PreimageAnnouncement) error {
	validatorIndex := preimageAnnouncement.ValidatorIndex
	p, ok := n.peersInfo[validatorIndex]
	if !ok {
		return fmt.Errorf("invalid validator index %d", validatorIndex)
	}

	serviceIndex := preimageAnnouncement.ServiceIndex
	preimageHash := preimageAnnouncement.PreimageHash

	log.Debug(debugP, "processPreimageAnnouncements",
		"n", n.String(),
		"validatorIndex", validatorIndex,
		"serviceIndex", serviceIndex,
		"preimageHash", preimageHash,
	)

	preimage, err := p.SendPreimageRequest(ctx, preimageHash)
	if err != nil {
		log.Error(debugP, "SendPreimageRequest failed", "err", err)
		return fmt.Errorf("SendPreimageRequest failed: %w", err)
	}

	log.Debug(debugP, "received preimage", "n", n.String(), "size", len(preimage), "preview", fmt.Sprintf("%.64s...", common.Bytes2String(preimage)))

	n.processPreimage(types.Preimages{
		Requester: uint32(serviceIndex),
		Blob:      preimage,
	})

	return nil
}

func (p *Peer) SendPreimageAnnouncement(ctx context.Context, pa *types.PreimageAnnouncement) error {
	code := uint8(CE142_PreimageAnnouncement)

	if p.node.store.SendTrace {
		tracer := p.node.store.Tp.Tracer("NodeTracer")
		_, span := tracer.Start(ctx, fmt.Sprintf("[N%d] SendPreimageAnnouncement", p.node.store.NodeID))
		defer span.End()
	}

	stream, err := p.openStream(ctx, code)
	if err != nil {
		return fmt.Errorf("openStream[CE142_PreimageAnnouncement]: %w", err)
	}
	defer stream.Close()

	paBytes, err := pa.ToBytes()
	if err != nil {
		return fmt.Errorf("ToBytes[CE142_PreimageAnnouncement]: %w", err)
	}

	if err := sendQuicBytes(ctx, stream, paBytes, p.PeerID, code); err != nil {
		return fmt.Errorf("sendQuicBytes[CE142_PreimageAnnouncement]: %w", err)
	}

	return nil
}

func (n *Node) onPreimageAnnouncement(ctx context.Context, stream quic.Stream, msg []byte, peerID uint16) error {
	defer stream.Close()

	var preimageAnnouncement types.PreimageAnnouncement
	if err := preimageAnnouncement.FromBytes(msg); err != nil {
		log.Error(debugP, "onPreimageAnnouncement: failed to decode", "err", err)
		return fmt.Errorf("onPreimageAnnouncement: decode failed: %w", err)
	}

	preimageAnnouncement.ValidatorIndex = peerID

	log.Trace(debugP, "onPreimageAnnouncement",
		"n", n.String(),
		"peerID", peerID,
		"serviceIndex", preimageAnnouncement.ServiceIndex,
		"preimageHash", preimageAnnouncement.PreimageHash,
	)

	return n.processPreimageAnnouncements(ctx, preimageAnnouncement)
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

	stream, err := p.openStream(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("openStream[CE143_PreimageRequest]: %w", err)
	}
	defer stream.Close()

	if err := sendQuicBytes(ctx, stream, preimageHash.Bytes(), p.PeerID, code); err != nil {
		return nil, fmt.Errorf("sendQuicBytes[CE143_PreimageRequest]: %w", err)
	}

	preimage, err := receiveQuicBytes(ctx, stream, p.PeerID, code)
	if err != nil {
		return nil, fmt.Errorf("receiveQuicBytes[CE143_PreimageRequest]: %w", err)
	}

	return preimage, nil
}

func (n *NodeContent) onPreimageRequest(ctx context.Context, stream quic.Stream, msg []byte) error {
	defer stream.Close()

	preimageHash := common.BytesToHash(msg)
	preimage, ok, err := n.PreimageLookup(preimageHash)
	if err != nil {
		return fmt.Errorf("onPreimageRequest: PreimageLookup failed: %w", err)
	}
	if !ok {
		log.Warn(debugP, "onPreimageRequest", "n", n.id, "hash", preimageHash, "msg", "preimage not found")
		return nil
	}

	code := uint8(CE143_PreimageRequest)

	if err := sendQuicBytes(ctx, stream, preimage, n.id, code); err != nil {
		return fmt.Errorf("onPreimageRequest: sendQuicBytes failed: %w", err)
	}

	log.Trace(debugP, "onPreimageRequest", "n", n.id, "hash", preimageHash, "size", len(preimage))
	return nil
}
