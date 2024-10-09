package types

import "reflect"

type TransferMemo struct {
	S uint32
	D uint32
	A uint32
	M uint32
	G uint32
}

// Create a TransferMemo from a byte slice.
func TransferMemoFromBytes(data []byte) (*TransferMemo, error) {
	var t TransferMemo
	// Deserialize the JSON bytes into a ServiceAccount struct
	decoded, _, err := Decode(data, reflect.TypeOf(TransferMemo{}))
	if err != nil {
		return nil, err
	}
	t = decoded.(TransferMemo)
	return &t, nil
}
