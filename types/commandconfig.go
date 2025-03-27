package types

type CommandConfig struct {
	Help            bool   `json:"help"`
	NodeName        string `json:"nodename"`
	DataDir         string `json:"datadir"`
	Epoch0Timestamp int    `json:"ts"`
	Port            int    `json:"port"`
	GenesisState    string `json:"genesis_state"`
	GenesisBlock    string `json:"genesis_block"`
	Ed25519         string `json:"ed25519"`
	Bls             string `json:"bls"`
	Bandersnatch    string `json:"bandersnatch"`
	Network         string `json:"network"`
	HostnamePrefix  string `json:"hp"`
}

// String method returns the CommandConfig as a formatted JSON string
func (c *CommandConfig) String() string {
	return ToJSON(c)
}
