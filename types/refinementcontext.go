package types

import (
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
type RefinementContext struct {
	Anchor             common.Hash `json:"anchor"`
	PosteriorStateRoot common.Hash `json:"posterior_state_root"`
	PosteriorBeefyRoot common.Hash `json:"posterior_beefy_root"`
	LookupAnchor       common.Hash `json:"lookup_anchor"`
	HeaderHash         common.Hash `json:"header_hash"`
	Timeslot           int         `json:"timeslot"`
	Prerequisite       common.Hash `json:"prerequisite,omitempty"`
}
