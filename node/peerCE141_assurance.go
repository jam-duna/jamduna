package node

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
	"github.com/quic-go/quic-go"
)

/*
CE 141: Assurance distribution
Distribution of an availability assurance ready for inclusion in a block.

Assurers should distribute availability assurances approximately 2 seconds before each slot, to all possible block authors. Note that the assurer set and the block author set should switch over to the next validator set for the distribution 2 seconds before a new epoch -- the assurances extrinsic and block seal are both checked using the posterior keysets.

Anchor Hash = [u8; 32]
Bitfield = [u8; 43] (One bit per core)
Ed25519 Signature = [u8; 64]
Assurance = Anchor Hash ++ Bitfield ++ Ed25519 Signature

Assurer -> Validator

--> Assurance
--> FIN
<-- FIN
*/
type JAMSNPAssuranceDistribution struct {
	Anchor    common.Hash                      `json:"anchor"`
	Bitfield  [types.Avail_bitfield_bytes]byte `json:"bitfield"`
	Signature types.Ed25519Signature           `json:"signature"`
}

type AssuranceObject struct {
	Anchor     common.Hash                      `json:"anchor"`
	Bitfield   [types.Avail_bitfield_bytes]byte `json:"bitfield"`
	Signature  types.Ed25519Signature           `json:"signature"`
	Ed25519Key types.Ed25519Key                 `json:"ed25519_key"`
}

// ToBytes serializes the JAMSNPAssurance struct into a byte array
func (assurance *JAMSNPAssuranceDistribution) ToBytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Serialize Anchor (32 bytes for common.Hash)
	if _, err := buf.Write(assurance.Anchor[:]); err != nil {
		return nil, err
	}

	// Serialize Bitfield (Avail_bitfield_bytes bytes)
	if _, err := buf.Write(assurance.Bitfield[:]); err != nil {
		return nil, err
	}

	// Serialize Signature (64 bytes for Ed25519Signature)
	if _, err := buf.Write(assurance.Signature[:]); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// FromBytes deserializes a byte array into a JAMSNPAssurance struct
func (assurance *JAMSNPAssuranceDistribution) FromBytes(data []byte) error {
	buf := bytes.NewReader(data)

	// Deserialize Anchor (32 bytes for common.Hash)
	if _, err := io.ReadFull(buf, assurance.Anchor[:]); err != nil {
		return err
	}

	// Deserialize Bitfield (Avail_bitfield_bytes bytes)
	if _, err := io.ReadFull(buf, assurance.Bitfield[:]); err != nil {
		return err
	}

	// Deserialize Signature (64 bytes for Ed25519Signature)
	if _, err := io.ReadFull(buf, assurance.Signature[:]); err != nil {
		return err
	}

	return nil
}

// SendAssurance sends an assurance to the peer
// variadic kv is used to pass key-value pairs to telemetry
func (p *Peer) SendAssurance(ctx context.Context, a *types.Assurance, eventID uint64) error {
	req := &JAMSNPAssuranceDistribution{
		Anchor:    a.Anchor,
		Bitfield:  a.Bitfield,
		Signature: a.Signature,
	}

	reqBytes, err := req.ToBytes()
	if err != nil {
		return fmt.Errorf("ToBytes[CE141_AssuranceDistribution]: %w", err)
	}

	code := uint8(CE141_AssuranceDistribution)
	stream, err := p.openStream(ctx, code)
	if err != nil {
		// Telemetry: Assurance send failed (event 127)
		p.node.telemetryClient.AssuranceSendFailed(eventID, p.PeerKey(), err.Error())
		return fmt.Errorf("openStream[CE141_AssuranceDistribution]: %w", err)
	}
	defer stream.Close()

	if err := sendQuicBytes(ctx, stream, reqBytes, p.SanKey(), code); err != nil {
		// Telemetry: Assurance send failed (event 127)
		p.node.telemetryClient.AssuranceSendFailed(eventID, p.PeerKey(), err.Error())
		return fmt.Errorf("sendQuicBytes[CE141_AssuranceDistribution]: %w", err)
	}

	// Telemetry: Assurance sent (event 128)
	p.node.telemetryClient.AssuranceSent(eventID, p.PeerKey())

	return nil
}

func (n *Node) onAssuranceDistribution(ctx context.Context, stream quic.Stream, msg []byte, peerKey string) error {
	defer stream.Close()

	// Get peer to access its PeerID for telemetry
	peer, ok := n.peersByPubKey[peerKey]
	if !ok {
		return fmt.Errorf("onAssuranceDistribution: peer not found for key %s", peerKey)
	}

	var newReq JAMSNPAssuranceDistribution
	if err := newReq.FromBytes(msg); err != nil {
		// Telemetry: Assurance receive failed (event 130)
		n.telemetryClient.AssuranceReceiveFailed(PubkeyBytes(peer.Validator.Ed25519.SAN()), err.Error())
		return fmt.Errorf("onAssuranceDistribution: failed to decode message: %w", err)
	}

	// Telemetry: Assurance received (event 131)
	n.telemetryClient.AssuranceReceived(PubkeyBytes(peer.Validator.Ed25519.SAN()), newReq.Anchor)

	// Find the correct validator index by verifying signature against all validators
	// The CE141 protocol does NOT include validatorIndex, so we must derive it

	assurance := AssuranceObject{
		Anchor:     newReq.Anchor,
		Bitfield:   newReq.Bitfield,
		Ed25519Key: peer.Validator.Ed25519,
		Signature:  newReq.Signature,
	}

	select {
	case <-ctx.Done():
		return fmt.Errorf("onAssuranceDistribution: context cancelled: %w", ctx.Err())
	case n.assurancesCh <- assurance:
		log.Trace(log.Quic, "onAssuranceDistribution received", "peerKey", peerKey, "anchor", newReq.Anchor)
	default:
		log.Warn(log.Quic, "onAssuranceDistribution: assurance channel full, dropping", "peerKey", peerKey)
	}

	return nil
}

func (n *Node) findValidatorIndexByAnchor(anchor common.Hash, ed25519key types.Ed25519Key) (uint16, error) {
	safrole := n.statedb.GetSafrole()
	statedb := n.statedb
	if statedb != nil && statedb.Block.Header.Hash() == anchor {
		for index, v := range safrole.CurrValidators {
			if bytes.Equal(v.Ed25519[:], ed25519key[:]) {
				return uint16(index), nil
			}
		}
	} else {
		statedb := n.statedbMap[anchor]
		if statedb != nil {
			safrole = statedb.GetSafrole()
			for index, v := range safrole.CurrValidators {
				if bytes.Equal(v.Ed25519[:], ed25519key[:]) {
					return uint16(index), nil
				}
			}
		}
	}
	return 0, fmt.Errorf("signature does not match any known validator")
}
