package statedb

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
)

func TestKV(t *testing.T) {
	jsonStr := `{
    "keyvals": [
        [
            "0x0f00000000000000000000000000000000000000000000000000000000000000",
            "0x000000000000000000000000"
        ],
        [
            "0xff00000000000000000000000000000000000000000000000000000000000000",
            "0x619fe68beeb224fffce8d74982e6e37f554e670548e26ac75486617f83873c2a102700000000000064000000000000006400000000000000930000000000000001000000"
        ]
    ]
}`
	// unmarshal the json string
	a := StateSnapshotRaw{}
	err := json.Unmarshal([]byte(jsonStr), &a)
	if err != nil {
		t.Fatalf("TestKV %v\n", err)
	}

	// encode
	encoded, err := types.Encode(a)
	if err != nil {
		t.Fatalf("TestKV Encode %v\n", err)
	}

	// decode
	decoded, _, err := types.Decode(encoded, reflect.TypeOf(a))
	if err != nil {
		t.Fatalf("TestKV Decode %v\n", err)
	}

	// marshal the struct
	b, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("TestKV json.Marshal %v\n", err)
	}

}

func TestE(t *testing.T) {
	a := []byte{0x80, 0x93}
	decoded, l := types.DecodeE(a)
	log.Debug(module, "TestE", "Decoded", decoded, "Length", l)
}
