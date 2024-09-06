package types

import (
	"reflect"

	"github.com/colorfulnotion/jam/common"
)

const (
	RESULT_OK       = 0
	RESULT_OOG      = 1
	RESULT_PANIC    = 2
	RESULT_BAD_CODE = 3
	RESULT_OOB      = 4
	RESULT_FAULT    = 5
)

// 11.1.4. Work Result. Equation 121. We finally come to define a work result, L, which is the data conduit by which services’ states may be altered through the computation done within a work-package.
type WorkResult struct {
	Service     uint32      `json:"service"`
	CodeHash    common.Hash `json:"code_hash"`
	PayloadHash common.Hash `json:"payload_hash"`
	GasRatio    uint64      `json:"gas_ratio"`
	Result      Result      `json:"result"`
}

type SWorkResult struct {
	Service     uint32                 `json:"service"`
	CodeHash    common.Hash            `json:"code_hash"`
	PayloadHash common.Hash            `json:"payload_hash"`
	GasRatio    uint64                 `json:"gas_ratio"`
	Result      map[string]interface{} `json:"result"`
}

type Result struct {
	Ok  []byte `json:"ok,omitempty"`
	Err uint8  `json:"err,omitempty"`
}

// see 12.3 Wrangling - Eq 159
type WrangledWorkResult struct {
	// Note this is Output OR Error
	// Output Result `json:"output"`
	//Output map[string]interface{} `json:"output"`
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
	var result Result
	if _, ok := s.Result["ok"]; ok {
		result = Result{
			Ok:  common.FromHex(s.Result["ok"].(string)),
			Err: RESULT_OK,
		}
	} else if _, ok := s.Result["out-of-gas"]; ok {
		result = Result{
			Ok:  nil,
			Err: RESULT_OOG,
		}
	} else if _, ok := s.Result["panic"]; ok {
		result = Result{
			Ok:  nil,
			Err: RESULT_PANIC,
		}
	} else if _, ok := s.Result["bad-code"]; ok {
		result = Result{
			Ok:  nil,
			Err: RESULT_BAD_CODE,
		}
	} else if _, ok := s.Result["code-oversize"]; ok {
		result = Result{
			Ok:  nil,
			Err: RESULT_OOB,
		}
	}
	return WorkResult{
		Service:     s.Service,
		CodeHash:    s.CodeHash,
		PayloadHash: s.PayloadHash,
		GasRatio:    s.GasRatio,
		Result:      result,
	}
}

func (R Result) Encode() []byte {
	if R.Err == RESULT_OK {
		ok_byte := R.Ok
		return append([]byte{0}, Encode(ok_byte)...)
	} else {
		switch R.Err {
		case RESULT_OOG:
			return []byte{1}
		case RESULT_PANIC:
			return []byte{2}
		case RESULT_BAD_CODE:
			return []byte{3}
		case RESULT_OOB:
			return []byte{4}
		}
	}
	return nil
}

func (target Result) Decode(data []byte) (interface{}, uint32) {
	length := uint32(1)
	switch data[0] {
	case 0:
		ok_byte, l := Decode(data[length:], reflect.TypeOf([]byte{}))
		return Result{
			Ok:  ok_byte.([]byte),
			Err: RESULT_OK,
		}, length + l
	case 1:
		return Result{
			Ok:  nil,
			Err: RESULT_OOG,
		}, length
	case 2:
		return Result{
			Ok:  nil,
			Err: RESULT_PANIC,
		}, length
	case 3:
		return Result{
			Ok:  nil,
			Err: RESULT_BAD_CODE,
		}, length
	case 4:
		return Result{
			Ok:  nil,
			Err: RESULT_OOB,
		}, length
	}
	return nil, 0
}
