package types

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/colorfulnotion/jam/common"
)

func TestParam(t *testing.T) {
	// t.Skip("Skipping param test")

	data := common.FromHex("0a00000000000000010000000000000064000000000000000200200000000c000000809698000000000080f0fa020000000000ca9a3b00000000002d3101000000000800100008000300180000000300080006005000040080000500060000fa00008070d20000093d0004000000000c00000204000000c0000080000000000c00000a000000")
	var params Parameters
	params.FromBytes(data)
	t.Logf("params: %+v", params)
	paramBytes, err := ParameterBytes()
	if err != nil {
		t.Fatalf("ParameterBytes error: %v", err)
	}
	if !bytes.Equal(paramBytes, data) {
		var our Parameters
		our.FromBytes(paramBytes)
		t.Logf("our: %s", our.String())
		t.Logf("orig: %s", params.String())
		t.Fatalf("ParameterBytes do not match original data")
	}
	t.Logf("✅ ParameterBytes match original data")
	fmt.Printf("ParameterBytes: %x\n", paramBytes)
	fmt.Printf("ParameterString: %s\n", params.String())
	params.PrintFieldsHex()

}
