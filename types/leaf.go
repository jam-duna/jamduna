package types

import (
	"github.com/colorfulnotion/jam/common"
)

type ChainLeaf struct {
	HeaderHash common.Hash `json:"headerHash"`
	Timeslot   uint32      `json:"slot"`
}
