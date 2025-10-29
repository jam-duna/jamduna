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

// SerializeRefineContext serializes RefineContext to fixed binary format for Rust compatibility
// Format: [anchor:32][state_root:32][beefy_root:32][lookup_anchor:32][lookup_anchor_slot:4 LE][prereq_count:4 LE][prerequisites:32*count]
func (rc *RefineContext) SerializeRefineContext() []byte {
	prereqCount := uint32(len(rc.Prerequisites))
	size := 32 + 32 + 32 + 32 + 4 + 4 + (32 * len(rc.Prerequisites))
	buf := make([]byte, 0, size)

	// 1. Anchor (32 bytes)
	buf = append(buf, rc.Anchor[:]...)

	// 2. StateRoot (32 bytes)
	buf = append(buf, rc.StateRoot[:]...)

	// 3. BeefyRoot (32 bytes)
	buf = append(buf, rc.BeefyRoot[:]...)

	// 4. LookupAnchor (32 bytes)
	buf = append(buf, rc.LookupAnchor[:]...)

	// 5. LookupAnchorSlot (4 bytes, little endian)
	buf = append(buf, byte(rc.LookupAnchorSlot), byte(rc.LookupAnchorSlot>>8),
		byte(rc.LookupAnchorSlot>>16), byte(rc.LookupAnchorSlot>>24))

	// 6. Prerequisites count (4 bytes, little endian)
	buf = append(buf, byte(prereqCount), byte(prereqCount>>8),
		byte(prereqCount>>16), byte(prereqCount>>24))

	// 7. Prerequisites (32 bytes each)
	for _, hash := range rc.Prerequisites {
		buf = append(buf, hash[:]...)
	}

	return buf
}
