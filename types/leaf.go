package types

import (
	"github.com/jam-duna/jamduna/common"
)

type ChainLeaf struct {
	HeaderHash common.Hash `json:"headerHash"`
	Timeslot   uint32      `json:"slot"`
}
