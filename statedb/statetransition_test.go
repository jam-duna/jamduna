package statedb

import (
	"encoding/json"
	"os"
	"reflect"
	//"strings"
	"fmt"
	"testing"
	//"strconv"

	//"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

func TestParseSLookup(s *testing.T) {
	x, y := parse_SLookup("s=2953942612|h=0x6a0d4a19d199505713fc65f531038e73f1d885645632c8ae503c4f0c4d5e19a7 l=3|t=[29 31] tlen=2")
	fmt.Printf("x: %v y:%v\n", x, y)
}

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
	decoded, _, err := types.Decode(encoded, reflect.TypeOf(StateTransition{}))
	if err != nil {
		t.Fatal(err)
	}

	encoded2, err := types.Encode(decoded)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(encoded, encoded2) {
		t.Fatal("encoded and encoded2 are not equal")
	}
}
