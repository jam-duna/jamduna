package types

import (
	"github.com/jam-duna/jamduna/common"
)

type TestService struct {
	AccountKey  []byte
	PreimageKey []byte
	ServiceCode uint32
	FileName    string
	CodeHash    common.Hash
	//Code        []byte
	CodeLen     uint32
	ServiceName string
	//	MetadataAndCode []byte
	Storage map[common.Hash][]byte
}

type ServiceInfo struct {
	ServiceIndex    uint32      `json:"ServiceIndex"`
	ServiceCodeHash common.Hash `json:"ServiceCodeHash"`
}

type ServiceSummary struct {
	ServiceID          uint32             `json:"service"`
	ServiceName        string             `json:"metadata"`
	LastRefineSlot     uint32             `json:"last_refine_slot"`
	LastAccumulateSlot uint32             `json:"last_accumulate_slot"`
	Statistics         *ServiceStatistics `json:"statistics"`
}

func (s *ServiceSummary) String() string {
	return ToJSON(s)
}
