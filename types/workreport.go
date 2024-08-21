package types

import (
	"github.com/colorfulnotion/jam/common"
)

/*
11.1.1. Work Report. See Equation 117. A work-report, of the set W, is defined as a tuple of:
* the work-package specification $s$,
* the refinement context $x$,
* the core-index $c$ (i.e. on which the work is done)
* the authorizer hash $a$ and
* output ${\bf o}$ and
* $r$, the results of the evaluation of each of the items in the package r, which is always at least one item and may be no more than I items.
*/
// WorkReport represents a work report.
type WorkReport struct {
	AvailabilitySpec     AvailabilitySpecification `json:"availability"`
	AuthorizerHash       common.Hash               `json:"authorizer_hash"`
	Core                 int                       `json:"total_size"`
	Output               []byte                    `json:"output"`
	RefinementContext    RefinementContext         `json:"refinement_context"`
	PackageSpecification string                    `json:"package_specification"`
	Results              []WorkResult              `json:"results"`
}
