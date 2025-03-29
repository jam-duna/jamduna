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
}

type ServiceInfo struct {
	ServiceIndex    uint32      `json:"ServiceIndex"`
	ServiceCodeHash common.Hash `json:"ServiceCodeHash"`
}
