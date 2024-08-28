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

const (
	EntryPointGeneric    = ""
	EntryPointRefine     = "refine"
	EntryPointAccumulate = "accumulate"
	EntryPointOnTransfer = "on_transfer"
	EntryPointAuthorize  = "authorization"
)

// ServiceAccount represents a service account.
type ServiceAccount struct {
	CodeHash  common.Hash 		 `json:"code_hash"`
	Balance   uint64      		 `json:"balance"`
	GasLimitG uint64      		 `json:"gas_limit_g"`
	GasLimitM uint64      		 `json:"gas_limit_m"`
	StorageSize uint64           `json:"a_l"`   //a_l - total number of octets used in storage (9.3)
	NumStorageItems  uint32      `json:"a_i"` 	//a_i - the number of items in storage (9.3)
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

// eq 95
func (s *ServiceAccount) Compute_threshold() uint64 {
	//BS +BI ⋅ai +BL ⋅al
	account_threshold := BaseServiceBalance + MinElectiveServiceItemBalance * uint64(s.NumStorageItems)  + MinElectiveServiceOctetBalance * s.StorageSize
	return account_threshold
}
