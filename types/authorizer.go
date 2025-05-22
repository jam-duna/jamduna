package types

import (
	"encoding/json"

	"github.com/colorfulnotion/jam/common"
)

type Authorizer struct {
	CodeHash common.Hash `json:"code_hash"`
	Params   []byte      `json:"params"`
}

func (a *Authorizer) UnmarshalJSON(data []byte) error {
	var s struct {
		CodeHash common.Hash `json:"code_hash"`
		Params   string      `json:"params"`
	}
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}
	a.CodeHash = s.CodeHash
	a.Params = common.FromHex(s.Params)

	return nil
}

func (a Authorizer) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		CodeHash common.Hash `json:"code_hash"`
		Params   string      `json:"params"`
	}{
		CodeHash: a.CodeHash,
		Params:   common.HexString(a.Params),
	})
}
