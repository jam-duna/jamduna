package node

import (
	"context"
	"fmt"
	"reflect"

	bls "github.com/colorfulnotion/jam/bls"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
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
type JAMEpochFinalized struct {
	ValidatorIndex uint16        `json:"validator_index"`
	Epoch          uint32        `json:"epoch"`
	BeefyHash      common.Hash   `json:"beefy_hash"`
	Signature      bls.Signature `json:"signature"`
}

func (j *JAMEpochFinalized) ToBytes() ([]byte, error) {
	return types.Encode(j)
}

func (j *JAMEpochFinalized) String() string {
	return types.ToJSON(j)
}

func (j *JAMEpochFinalized) FromBytes(data []byte) error {
	decoded, _, err := types.Decode(data, reflect.TypeOf(JAMEpochFinalized{}))
	if err != nil {
		return err
	}
	*j = decoded.(JAMEpochFinalized)
	return nil
}

func (p *Peer) SendEpochFinalized(ctx context.Context, epochFinalized JAMEpochFinalized) error {
	code := uint8(CE154_EpochFinalized)
	stream, err := p.openStream(ctx, code)
	if err != nil {
		return fmt.Errorf("openStream[CE154_EpochFinalized] failed: %w", err)
	}
	defer stream.Close()

	// Send: Epoch ++ HeaderHash ++ BLS Signature
	reqBytes, err := epochFinalized.ToBytes()
	if err != nil {
		return fmt.Errorf("WarpSyncRequest.ToBytes failed: %w", err)
	}

	if err := sendQuicBytes(ctx, stream, reqBytes, p.SanKey(), code); err != nil {
		return fmt.Errorf("sendQuicBytes[CE154_EpochFinalized] failed: %w", err)
	}

	return nil
}

func (n *Node) onEpochFinalized(ctx context.Context, stream quic.Stream, msg []byte, peerKey string) error {
	defer stream.Close()

	// Decode: Epoch ++ BeefyHash ++ BLS Signature
	var epochFinalized JAMEpochFinalized
	if err := epochFinalized.FromBytes(msg); err != nil {
		stream.CancelRead(ErrInvalidData)
		return fmt.Errorf("onEpochFinalized: decode failed: %w", err)
	}

	// Process the BLS signature
	if n.grandpa != nil {

		// if err := n.grandpa.ProcessBLSSignature(epochFinalized.Epoch, epochFinalized.BeefyHash, epochFinalized.Signature, epochFinalized.ValidatorIndex); err != nil {
		// 	return fmt.Errorf("ProcessBLSSignature failed: %w", err)
		// }
	}

	return nil
}
