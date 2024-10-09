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

type RefineContext struct {
	Anchor           common.Hash   `json:"anchor"`
	StateRoot        common.Hash   `json:"state_root"`
	BeefyRoot        common.Hash   `json:"beefy_root"`
	LookupAnchor     common.Hash   `json:"lookup_anchor"`
	LookupAnchorSlot uint32        `json:"lookup_anchor_slot"`
	Prerequisite     *Prerequisite `json:"prerequisite"`
}

type Prerequisite common.Hash

func (P *Prerequisite) Encode() []byte {
	if P == nil {
		return []byte{0}
	}
	encoded := []byte{1}
	encodedPrerequisite, err := Encode(*P)
	if err != nil {
		return []byte{}
	}
	encoded = append(encoded, encodedPrerequisite...)
	return encoded
}

func (target *Prerequisite) Decode(data []byte) (interface{}, uint32) {
	if data[0] == 0 {
		return nil, 1
	}
	var decoded Prerequisite
	length := uint32(1)
	prerequisite, l, err := Decode(data[length:], reflect.TypeOf(common.Hash{}))
	if err != nil {
		return nil, length
	}
	length += l
	decoded = Prerequisite(prerequisite.(common.Hash))
	return &decoded, length
}

func (p *Prerequisite) Hash() common.Hash {
	return common.Hash(*p)
}
