package types

import (
	"github.com/colorfulnotion/jam/common"
)

// 11.1.4. Work Result. Equation 121. We finally come to define a work result, L, which is the data conduit by which services’ states may be altered through the computation done within a work-package.
type WorkResult struct {
	Service     uint32      `json:"service"`
	CodeHash    common.Hash `json:"code_hash"`
	PayloadHash common.Hash `json:"payload_hash"`
	GasRatio    uint64      `json:"gas_ratio"`
	Result      Result      `json:"result"`
}

type Result struct {
	Ok  []byte `json:"ok,omitempty"`
	Err uint8  `json:"err,omitempty"`
}

const (
	RESULT_OK       = 0
	RESULT_OOG      = 1
	RESULT_PANIC    = 2
	RESULT_BAD_CODE = 3
	RESULT_OOB      = 4
	RESULT_FAULT    = 5
)

type SResult struct {
	Ok  string `json:"ok,omitempty"`
	Err uint8  `json:"err,omitempty"`
}

type SWorkResult struct {
	Service     uint32      `json:"service"`
	CodeHash    common.Hash `json:"code_hash"`
	PayloadHash common.Hash `json:"payload_hash"`
	GasRatio    uint64      `json:"gas_ratio"`
	Result      SResult     `json:"result"`
}

// see 12.3 Wrangling - Eq 159
type WrangledWorkResult struct {
	// Note this is Output OR Error
	Output Result `json:"output"`
	// l
	PayloadHash         common.Hash `json:"payload_hash"`
	AuthorizationOutput []byte      `json:"authorization_output"`
	WorkPackageHash     common.Hash `json:"output"`

	// matched with error
	Error string `json:"error"`
}

func (wr *WorkResult) Wrangle(authorizationOutput []byte, workPackageHash common.Hash) WrangledWorkResult {
	return WrangledWorkResult{
		Output:              wr.Result,
		PayloadHash:         wr.PayloadHash,
		AuthorizationOutput: authorizationOutput,
		WorkPackageHash:     workPackageHash,
	}
}

func (s *SWorkResult) Deserialize() WorkResult {
	return WorkResult{
		Service:     s.Service,
		CodeHash:    s.CodeHash,
		PayloadHash: s.PayloadHash,
		GasRatio:    s.GasRatio,
		Result:      s.Result.Deserialize(),
	}
}

func (s *SResult) Deserialize() Result {
	return Result{
		Ok:  common.FromHex(s.Ok),
		Err: s.Err,
	}
}
