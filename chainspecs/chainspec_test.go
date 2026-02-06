package chainspecs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"testing"

	"github.com/jam-duna/jamduna/common"
	"github.com/jam-duna/jamduna/grandpa"
	"github.com/jam-duna/jamduna/types"
)

func TestGenerateConfigFile(t *testing.T) {
	var devCfg DevConfig
	devCfg.GenesisValidators = make([]GenesisValidator, 6)
	devCfg.ID = "dev"
	validators, _, _ := grandpa.GenerateValidatorSecretSet(6)
	for i, validator := range validators {
		devCfg.GenesisValidators[i].NetAddr = fmt.Sprintf("127.0.0.1:%d", 40000+i)
		devCfg.GenesisValidators[i].PeerID = common.ToSAN(validator.Ed25519[:])
		devCfg.GenesisValidators[i].Bandersnatch = common.Bytes2String(validator.Bandersnatch[:])
	}
	jsonBytes, err := json.MarshalIndent(devCfg, "", "    ")
	if err != nil {
		t.Fatalf("Failed to marshal config: %v", err)
	}
	name := "dev-config.json"
	file, err := os.Create(name)
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}
	defer file.Close()
	if _, err := file.Write(jsonBytes); err != nil {
		t.Fatalf("Failed to write JSON to file: %v", err)
	}

	t.Logf("✅ Config file written to %s", name)
}

func TestParameterIsTheSame(t *testing.T) {
	var target string
	switch runtime.GOOS {
	case "linux":
		target = "linux-amd64/polkajam-spec.json"
	case "darwin":
		target = "mac-arm64/polkajam-spec.json"
	default:
		t.Fatalf("Unsupported platform: %s", runtime.GOOS)
	}
	var chainSpec ChainSpec
	// unmarshal the JSON data into the struct
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("Failed to read file %s: %v", target, err)
	}
	if err := json.Unmarshal(data, &chainSpec); err != nil {
		t.Fatalf("Failed to unmarshal JSON from %s: %v", target, err)
	}
	paramBytesString := chainSpec.ProtocolParameters
	paramBytes := common.FromHex(paramBytesString)
	our_params, _ := types.ParameterBytes()
	if !bytes.Equal(paramBytes, our_params) {
		t.Fatalf("Parameter bytes do not match expected value")
	}
	t.Logf("✅ Parameter bytes match expected value")
	decoded_data, _, err := types.Decode(paramBytes, reflect.TypeOf(&types.Parameters{}))
	if err != nil {
		t.Fatalf("Failed to decode parameters: %v", err)
	}
	fmt.Printf("Decoded Parameters: %+v\n", decoded_data.(*types.Parameters).String())
}
