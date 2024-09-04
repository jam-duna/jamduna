package types

import (
	"encoding/json"
	"fmt"
)

type CommandConfig struct {
	Help      bool   `json:"help"`
	NodeName  string `json:"nodename"`
	DataDir   string `json:"datadir"`
	Port      int    `json:"port"`
	RPC       bool   `json:"rpc"`
	LogJson   bool   `json:"logjson"`
	Genesis   string `json:"genesis"`
	Safrole   bool   `json:"safrole"`
	Guarantee bool   `json:"guarantee"`
	Assurance bool   `json:"assurance"`
	Auditing  bool   `json:"auditing"`
	Disputes  bool   `json:"disputes"`
	Preimages bool   `json:"preimages"`
	DA        bool   `json:"da"`
}

// String method returns the CommandConfig as a formatted JSON string
func (c *CommandConfig) String() string {
	jsonData, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Sprintf("Error marshaling JSON: %v", err)
	}
	return string(jsonData)
}
