package types

import (
	"reflect"

	"github.com/colorfulnotion/jam/bls"
)

type WarpSyncResponse struct {
	Fragments []WarpSyncFragment
}

type WarpSyncFragment struct {
	Header        BlockHeader
	Justification bls.Signature
}

func (w *WarpSyncResponse) ToBytes() ([]byte, error) {
	bytes, err := Encode(w)
	if err != nil {
		return nil, nil
	}
	return bytes, nil
}

func (w *WarpSyncResponse) FromBytes(data []byte) error {
	decoded, _, err := Decode(data, reflect.TypeOf(WarpSyncResponse{}))
	if err != nil {
		return err
	}
	*w = decoded.(WarpSyncResponse)
	if err != nil {
		return err
	}
	return nil
}
