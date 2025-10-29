package types

import (
	"github.com/colorfulnotion/jam/common"
)

type TestService struct {
	ServiceCode     uint32
	FileName        string
	CodeHash        common.Hash
	Code            []byte
	ServiceName     string
	MetadataAndCode []byte
	Preimages       []string
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
