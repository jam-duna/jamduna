package types

import (
	//"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/colorfulnotion/jam/common"
)

const (
	ServiceAccountPrefix = 255
)

// ServiceAccount represents a service account.
type ServiceAccount struct {
	StorageDict         map[common.Hash]string `json:"storage_dict"`
	PreimageLookupDictP map[common.Hash]string `json:"preimage_lookup_dict_p"`
	PreimageLookupDictL map[common.Hash]int    `json:"preimage_lookup_dict_l"`
	CodeHash            common.Hash            `json:"code_hash"`
	Balance             int                    `json:"balance"`
	GasLimitG           int                    `json:"gas_limit_g"`
	GasLimitM           int                    `json:"gas_limit_m"`
}

// Convert the ServiceAccount to a byte slice.
func (s *ServiceAccount) Bytes() ([]byte, error) {
	// Serialize the ServiceAccount struct to JSON bytes
	data, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// Create a ServiceAccount from a byte slice.
func ServiceAccountFromBytes(data []byte) (*ServiceAccount, error) {
	var s ServiceAccount
	// Deserialize the JSON bytes into a ServiceAccount struct
	err := json.Unmarshal(data, &s)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// Convert the ServiceAccount to a human-readable string.
func (s *ServiceAccount) String() string {
	return fmt.Sprintf("ServiceAccount{StorageDict: %v, PreimageLookupDictP: %v, PreimageLookupDictL: %v, CodeHash: %v, Balance: %d, GasLimitG: %d, GasLimitM: %d}",
		s.StorageDict, s.PreimageLookupDictP, s.PreimageLookupDictL, s.CodeHash.Hex(), s.Balance, s.GasLimitG, s.GasLimitM)
}
