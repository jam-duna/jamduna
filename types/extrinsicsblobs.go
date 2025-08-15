package types

import (
	"encoding/hex"
	"encoding/json"

	"github.com/colorfulnotion/jam/common"
)

type ExtrinsicsBlobs [][]byte

func (eb ExtrinsicsBlobs) Bytes() []byte {
	data, _ := Encode(eb)
	return data
}

func (e ExtrinsicsBlobs) MarshalJSON() ([]byte, error) {
	hexStrings := make([]string, len(e))
	for i, b := range e {
		hexStrings[i] = "0x" + hex.EncodeToString(b)
	}
	return json.Marshal(hexStrings)
}

func (e *ExtrinsicsBlobs) UnmarshalJSON(data []byte) error {
	var hexStrings []string
	if err := json.Unmarshal(data, &hexStrings); err != nil {
		return err
	}

	blobs := make([][]byte, len(hexStrings))
	for i, hexStr := range hexStrings {
		blobs[i] = common.FromHex(hexStr)
	}
	*e = blobs
	return nil
}
