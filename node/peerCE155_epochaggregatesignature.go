package node

import (
	"context"
	"fmt"
	"reflect"

	bls "github.com/jam-duna/jamduna/bls"
	"github.com/jam-duna/jamduna/common"
	"github.com/jam-duna/jamduna/types"
	"github.com/quic-go/quic-go"
)

/*
CE 154: Epoch Finalized
This is sent by each  validator at the end of an epoch to all other validators.

Validator -> Validator

--> ValidatorIndex ++ Epoch ++ BeefyHash ++ BLS Signature
--> FIN
<-- Catchup
<-- FIN
*/
type JAMEpochAggregateSignature struct {
	ValidatorIndex uint16        `json:"validator_index"`
	Epoch          uint32        `json:"epoch"`
	BeefyHash      common.Hash   `json:"beefy_hash"`
	Signature      bls.Signature `json:"signature"`
}

func (j *JAMEpochAggregateSignature) ToBytes() ([]byte, error) {
	return types.Encode(j)
}

func (j *JAMEpochAggregateSignature) FromBytes(data []byte) error {
	decoded, _, err := types.Decode(data, reflect.TypeOf(JAMEpochAggregateSignature{}))
	if err != nil {
		return err
	}
	*j = decoded.(JAMEpochAggregateSignature)
	return nil
}

func (p *Peer) SendEpochAggregateSignature(ctx context.Context, epochFinalized JAMEpochAggregateSignature) error {
	code := uint8(CE155_EpochAggregateSignature)
	stream, err := p.openStream(ctx, code)
	if err != nil {
		return fmt.Errorf("openStream[CE155_EpochAggregateSignature] failed: %w", err)
	}
	defer stream.Close()

	// Send: Epoch ++ HeaderHash ++ BLS Signature
	reqBytes, err := epochFinalized.ToBytes()
	if err != nil {
		return fmt.Errorf("WarpSyncRequest.ToBytes failed: %w", err)
	}

	if err := sendQuicBytes(ctx, stream, reqBytes, p.SanKey(), code); err != nil {
		return fmt.Errorf("sendQuicBytes[CE155_EpochAggregateSignature] failed: %w", err)
	}

	return nil
}

func (n *Node) onEpochAggregateSignature(ctx context.Context, stream quic.Stream, msg []byte, peerKey string) error {
	defer stream.Close()

	// Decode: Epoch ++ BeefyHash ++ BLS Signature
	var epochFinalized JAMEpochFinalized
	if err := epochFinalized.FromBytes(msg); err != nil {
		stream.CancelRead(ErrInvalidData)
		return fmt.Errorf("onEpochAggregateSignature: decode failed: %w", err)
	}

	// Process the BLS signature
	if n.grandpa != nil {
		// if err := n.grandpa.ProcessAggregateBLSSignature(epochFinalized.Epoch, epochFinalized.BeefyHash, epochFinalized.Signature, epochFinalized.ValidatorIndex); err != nil {
		// 	return fmt.Errorf("ProcessAggregateBLSSignature failed: %w", err)
		// }
	}

	return nil
}
