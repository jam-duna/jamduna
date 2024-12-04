package statedb

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/colorfulnotion/jam/types"
)

func TestStateTransitionCodec(t *testing.T) {
	preStateJsonPath := "testdata/traces/395479_007.json"
	preStateJsonByte, err := os.ReadFile(preStateJsonPath)
	if err != nil {
		t.Fatal(err)
	}

	blockJsonPath := "testdata/blocks/395479_007.json"
	blockJsonByte, err := os.ReadFile(blockJsonPath)
	if err != nil {
		t.Fatal(err)
	}

	postStateJsonPath := "testdata/traces/395479_008.json"
	postStateJsonByte, err := os.ReadFile(postStateJsonPath)
	if err != nil {
		t.Fatal(err)
	}

	// unmarshal the byte to StateSnapshotRaw
	var preState StateSnapshotRaw
	err = json.Unmarshal(preStateJsonByte, &preState)
	if err != nil {
		t.Fatal(err)
	}

	var block types.Block
	err = json.Unmarshal(blockJsonByte, &block)
	if err != nil {
		t.Fatal(err)
	}

	var postState StateSnapshotRaw
	err = json.Unmarshal(postStateJsonByte, &postState)
	if err != nil {
		t.Fatal(err)
	}

	a := StateTransition{
		PreState:  preState,
		Block:     block,
		PostState: postState,
	}
	encoded, err := types.Encode(a)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("encoded len: %d\n", len(encoded))
	decoded, decoded_len, err := types.Decode(encoded, reflect.TypeOf(StateTransition{}))
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("decoded len: %d\n", decoded_len)

	encoded2, err := types.Encode(decoded)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("encoded2 len: %d\n", len(encoded2))

	if !reflect.DeepEqual(encoded, encoded2) {
		t.Fatal("encoded and encoded2 are not equal")
	}
}
