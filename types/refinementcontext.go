package types

import (
	"github.com/colorfulnotion/jam/common"
)

/*
11.1.2. Refinement Context. See Eq 119. A refinement context, denoted by the set ${\cal X}$, describes the context of the chain at the point that the report’s corresponding work-package was evaluated. It identifies:
* two historical blocks, the anchor  header hash $a$
* its associated posterior state-root $s$
* posterior Beefy root $b$
* the lookupanchor header hash $l$
* timeslot $t$
* the hash of an optional prerequisite work-package p.
*/

// Work Context 11.4 C.24
type RefineContext struct {
	Anchor           common.Hash   `json:"anchor"`             // a
	StateRoot        common.Hash   `json:"state_root"`         // s
	BeefyRoot        common.Hash   `json:"beefy_root"`         // b
	LookupAnchor     common.Hash   `json:"lookup_anchor"`      // l
	LookupAnchorSlot uint32        `json:"lookup_anchor_slot"` // t
	Prerequisites    []common.Hash `json:"prerequisites"`      // p
}

func (rc *RefineContext) Clone() *RefineContext {
	return &RefineContext{
		Anchor:           rc.Anchor,
		StateRoot:        rc.StateRoot,
		BeefyRoot:        rc.BeefyRoot,
		LookupAnchor:     rc.LookupAnchor,
		LookupAnchorSlot: rc.LookupAnchorSlot,
		Prerequisites:    rc.Prerequisites,
	}
}

func (rc *RefineContext) String() string {
	return ToJSON(rc)
}
