package types

import (
	"encoding/json"
	"fmt"
)

type CommandConfig struct {
	Help            bool   `json:"help"`
	NodeName        string `json:"nodename"`
	DataDir         string `json:"datadir"`
	Epoch0Timestamp int    `json:"ts"`
	Port            int    `json:"port"`
	Genesis         string `json:"genesis"`
	Ed25519         string `json:"genesis"`
	Bls             string `json:"bls"`
	Bandersnatch    string `json:"bandersnatch"`
}

// String method returns the CommandConfig as a formatted JSON string
func (c *CommandConfig) String() string {
	jsonData, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Sprintf("Error marshaling JSON: %v", err)
	}
	return string(jsonData)
}
