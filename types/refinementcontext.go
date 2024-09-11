package types

import (
	"reflect"

	"github.com/colorfulnotion/jam/common"
)

/*
11.1.2. Refinement Context. See Eq 119. A refinement context, denoted by the set ${\cal X}$, describes the context of the chain at the point that the report’s corresponding work-package was evaluated. It identifies:
* two historical blocks, the anchor  header hash $a$
* its associated posterior state-root $s$
* posterior Beefy root $b$
* the lookupanchor $l$
* header hash $l$
* timeslot $t$
* the hash of an optional prerequisite work-package p.
*/
// RefinementContext represents the context of the chain at the point of evaluation.
// type RefinementContext struct {
// 	Anchor           common.Hash `json:"anchor"`
// 	StateRoot        common.Hash `json:"state_root"`
// 	BeefyRoot        common.Hash `json:"beefy_root"`
// 	LookupAnchor     common.Hash `json:"lookup_anchor"`
// 	LookupAnchorSlot uint32      `json:"lookup_anchor_slot"`
// 	Prerequisite     common.Hash `json:"prerequisite,omitempty"`
// }

type RefineContext struct {
	Anchor           common.Hash   `json:"anchor"`
	StateRoot        common.Hash   `json:"state_root"`
	BeefyRoot        common.Hash   `json:"beefy_root"`
	LookupAnchor     common.Hash   `json:"lookup_anchor"`
	LookupAnchorSlot uint32        `json:"lookup_anchor_slot"`
	Prerequisite     *Prerequisite `json:"prerequisite,omitempty"`
}

type Prerequisite common.Hash

func (P Prerequisite) Encode() []byte {
	encoded := []byte{1}
	encoded = append(encoded, Encode(P)...)
	return encoded
}

func (target Prerequisite) Decode(data []byte) (interface{}, uint32) {
	var decoded Prerequisite
	length := uint32(1)
	prerequisite, l := Decode(data[length:], reflect.TypeOf(common.Hash{}))
	length += l
	decoded = Prerequisite(prerequisite.(common.Hash))
	return decoded, length
}

func (p *Prerequisite) Hash() common.Hash {
	return common.Hash(*p)
}
