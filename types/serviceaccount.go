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
	CodeHash  common.Hash `json:"code_hash"`
	Balance   uint32      `json:"balance"`
	GasLimitG uint64      `json:"gas_limit_g"`
	GasLimitM uint64      `json:"gas_limit_m"`
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
	return fmt.Sprintf("ServiceAccount{CodeHash: %v, Balance: %d, GasLimitG: %d, GasLimitM: %d}",
		s.CodeHash.Hex(), s.Balance, s.GasLimitG, s.GasLimitM)
}
