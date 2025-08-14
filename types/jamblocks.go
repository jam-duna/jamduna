package types

import (
	"encoding/json"
)

type ConfigJamBlocks struct {
	HTTP        string
	Socket      string
	Verbose     bool
	NumBlocks   int
	InvalidRate float64
	Statistics  int
	Endpoint    string
	Network     string
	PVMBackend  string
	Seed        string
}

func (c ConfigJamBlocks) MarshalJSON() ([]byte, error) {
	type Alias ConfigJamBlocks
	return json.Marshal(&struct {
		*Alias
	}{
		Alias: (*Alias)(&c),
	})
}

func (c *ConfigJamBlocks) UnmarshalJSON(data []byte) error {
	type Alias ConfigJamBlocks
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(c),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	return nil
}

func (c ConfigJamBlocks) String() string {
	return ToJSON(c)
}
