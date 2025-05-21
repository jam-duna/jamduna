package chainspecs

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/statedb"
)

func TestGenerateConfigFile(t *testing.T) {
	var devCfg DevConfig
	devCfg.GenesisValidators = make([]GenesisValidator, 6)
	devCfg.ID = "dev"
	validators, _, _ := statedb.GenerateValidatorSecretSet(6)
	for i, validator := range validators {
		devCfg.GenesisValidators[i].NetAddr = fmt.Sprintf("127.0.0.1:%d", 40000+i)
		devCfg.GenesisValidators[i].PeerID = common.ToSAN(validator.Ed25519[:])
		devCfg.GenesisValidators[i].Ed25519 = common.Bytes2String(validator.Ed25519[:])
	}
	jsonBytes, err := json.MarshalIndent(devCfg, "", "  ")
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
