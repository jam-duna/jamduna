package node

import (
	"bytes"
	"context"
	"fmt"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
	"github.com/quic-go/quic-go"

	"io"
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
func (p *Peer) SendAssurance(ctx context.Context, a *types.Assurance, kv ...interface{}) error {
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

	if p.node.store.SendTrace {
		tracer := p.node.store.Tp.Tracer("NodeTracer")
		_, span := tracer.Start(ctx, fmt.Sprintf("[N%d] SendAssurance", p.node.store.NodeID))
		defer span.End()
	}

	stream, err := p.openStream(ctx, code)
	if err != nil {
		return fmt.Errorf("openStream[CE141_AssuranceDistribution]: %w", err)
	}
	defer stream.Close()

	if err := sendQuicBytes(ctx, stream, reqBytes, p.PeerID, code); err != nil {
		return fmt.Errorf("sendQuicBytes[CE141_AssuranceDistribution]: %w", err)
	}

	return nil
}

func (n *Node) onAssuranceDistribution(ctx context.Context, stream quic.Stream, msg []byte, peerID uint16) error {
	defer stream.Close()

	var newReq JAMSNPAssuranceDistribution
	if err := newReq.FromBytes(msg); err != nil {
		return fmt.Errorf("onAssuranceDistribution: failed to decode message: %w", err)
	}

	assurance := types.Assurance{
		Anchor:         newReq.Anchor,
		Bitfield:       newReq.Bitfield,
		ValidatorIndex: peerID,
		Signature:      newReq.Signature,
	}

	select {
	case <-ctx.Done():
		return fmt.Errorf("onAssuranceDistribution: context cancelled: %w", ctx.Err())
	case n.assurancesCh <- assurance:
		log.Trace(debugStream, "onAssuranceDistribution received", "peerID", peerID, "anchor", newReq.Anchor)
	default:
		log.Warn(debugStream, "onAssuranceDistribution: assurance channel full, dropping", "peerID", peerID)
	}

	return nil
}
