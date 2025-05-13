package node

import (
	"encoding/json"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/statedb"
)

type chainSpec struct {
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

func (c *ChainSpec) UnmarshalJSON(data []byte) error {
	var raw chainSpec
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	c.Bootnodes = raw.Bootnodes
	c.ID = raw.ID
	c.GenesisHeader = common.FromHex(raw.GenesisHeader)
	for key, value := range raw.GenesisState {
		var keyVal statedb.KeyVal
		key := common.FromHex(key)
		keyVal.Key = *(*[31]byte)(key[0:31])
		keyVal.Value = common.FromHex(value)
		c.GenesisState = append(c.GenesisState, keyVal)
	}
	return nil
}

func (c *ChainSpec) MarshalJSON() ([]byte, error) {
	type chainSpecAlias chainSpec
	alias := &chainSpecAlias{
		Bootnodes:     c.Bootnodes,
		ID:            c.ID,
		GenesisHeader: common.Bytes2String(c.GenesisHeader),
		GenesisState:  make(map[string]string),
	}
	for _, keyval := range c.GenesisState {
		key := common.Bytes2String(keyval.Key[:])
		value := common.Bytes2String(keyval.Value)
		alias.GenesisState[key] = value
	}
	return json.Marshal(alias)
}
