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
	Output                 []byte      `json:"output"`
	Error                  string      `json:"error"`
}

// see 12.3 Wrangling - Eq 159
type WrangledWorkResult struct {
	// Note this is Output OR Error
	Output []byte `json:"output"`
	// l
	PayloadHash         common.Hash `json:"payload_hash"`
	AuthorizationOutput []byte      `json:"authorization_output"`
	WorkPackageHash     common.Hash `json:"output"`

	// matched with error
	Error string `json:"error"`
}

func (wr *WorkResult) Wrangle(authorizationOutput []byte, workPackageHash common.Hash) WrangledWorkResult {
	return WrangledWorkResult{
		Output:              wr.Output,
		PayloadHash:         wr.PayloadHash,
		AuthorizationOutput: authorizationOutput,
		WorkPackageHash:     workPackageHash,
		Error:               wr.Error,
	}
}
