package chainspecs

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/storage"

	"embed"
)

//go:embed *.json
var configFS embed.FS

var networkFile = map[string]string{
	"dev":     "polkajam-spec.json", // dev:     polkajam          gen-spec dev-config.json polkajam-spec.json
	"jamduna": "jamduna-spec.json",  // jamduna: jamduna-linux-amd64 gen-spec dev-config.json jamduna-spec.json
}

func ReadSpec(id string) (spec *ChainSpec, err error) {
	fmt.Printf("Reading spec for %s\n", id)
	var data []byte
	path, ok := networkFile[id]
	if ok {
		fmt.Printf("Reading spec from configFS for %s path=%v\n", id, path)
		data, err = configFS.ReadFile(path)
		if err != nil {
			return spec, err
		}
	} else {
		fmt.Printf("OPEN FILE %s\n", id)
		data, err = os.ReadFile(id)
		if err != nil {
			return spec, err
		}
	}
	//var chainSpec ChainSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return spec, err
	}
	return spec, nil
}

type ChainSpecRaw struct {
	Bootnodes     []string          `json:"bootnodes"`
	ID            string            `json:"id"`
	GenesisHeader string            `json:"genesis_header"`
	GenesisState  map[string]string `json:"genesis_state"`
}
type ChainSpec struct {
	Bootnodes     []string         `json:"bootnodes"`
	ID            string           `json:"id"`
	GenesisHeader []byte           `json:"genesis_header"`
	GenesisState  []statedb.KeyVal `json:"genesis_state"`
}

func (cs ChainSpec) MarshalJSON() ([]byte, error) {
	type tmpChainSpec struct {
		Bootnodes     []string          `json:"bootnodes"`
		ID            string            `json:"id"`
		GenesisHeader string            `json:"genesis_header"`
		GenesisState  map[string]string `json:"genesis_state"`
	}
	tmp := tmpChainSpec{
		Bootnodes:     cs.Bootnodes,
		ID:            cs.ID,
		GenesisHeader: common.Bytes2String(cs.GenesisHeader),
		GenesisState:  make(map[string]string),
	}
	for _, kv := range cs.GenesisState {
		key_string := common.Bytes2String(kv.Key[:])
		value_string := common.Bytes2String(kv.Value)
		tmp.GenesisState[key_string] = value_string
	}
	return json.Marshal(tmp)
}

func (cs *ChainSpec) UnmarshalJSON(data []byte) error {
	type tmpChainSpec struct {
		Bootnodes     []string          `json:"bootnodes"`
		ID            string            `json:"id"`
		GenesisHeader string            `json:"genesis_header"`
		GenesisState  map[string]string `json:"genesis_state"`
	}
	tmp := tmpChainSpec{}
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}
	cs.Bootnodes = tmp.Bootnodes
	cs.ID = tmp.ID
	cs.GenesisHeader = common.FromHex(tmp.GenesisHeader)
	cs.GenesisState = make([]statedb.KeyVal, 0, len(tmp.GenesisState))
	for k, v := range tmp.GenesisState {
		keyBytes := common.FromHex(k)
		value := common.FromHex(v)
		var keyArr [31]byte
		copy(keyArr[:], keyBytes)
		cs.GenesisState = append(cs.GenesisState, statedb.KeyVal{Key: keyArr, Value: value})
	}
	return nil
}

type DevConfig struct {
	ID                string             `json:"id"`
	GenesisValidators []GenesisValidator `json:"genesis_validators"`
}
type GenesisValidator struct {
	PeerID       string `json:"peer_id"`
	Bandersnatch string `json:"bandersnatch"`
	NetAddr      string `json:"net_addr"`
}

func GenSpec(dev DevConfig) (chainSpec *ChainSpec, err error) {
	chainSpec = &ChainSpec{
		ID: dev.ID,
	}
	for _, validator := range dev.GenesisValidators {
		// use the validator's Bandersnatch pubkey and prepend with the SAN
		bootnode := fmt.Sprintf("%s@%s", common.ToSAN(common.FromHex(validator.Bandersnatch)), validator.NetAddr)
		chainSpec.Bootnodes = append(chainSpec.Bootnodes, bootnode)
	}

	tmpDir, err := os.MkdirTemp("", "genesis-*")
	if err != nil {
		return chainSpec, err
	}
	defer os.RemoveAll(tmpDir)

	sdb, err := storage.NewStateDBStorage(tmpDir)
	if err != nil {
		return chainSpec, err
	}

	trace, err := statedb.MakeGenesisStateTransition(sdb, 0, "tiny")
	if err != nil {
		return chainSpec, err
	}
	chainSpec.GenesisState = trace.PostState.KeyVals
	chainSpec.GenesisHeader, _ = trace.Block.Header.Bytes()
	return chainSpec, nil
}
