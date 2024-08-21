package types

import (
	"github.com/colorfulnotion/jam/common"
)

// 11.1.4. Work Result. Equation 121. We finally come to define a work result, L, which is the data conduit by which services’ states may be altered through the computation done within a work-package.
type WorkResult struct {
	Service                int         `json:"service_index"`
	CodeHash               common.Hash `json:"code_hash"`
	PayloadHash            common.Hash `json:"payload_hash"`
	GasPrioritizationRatio uint32      `json:"gas_prioritization_ratio"`
	Output                 []byte      `json:"refinement_context"`
	Error                  string      `json:"error"`
}
