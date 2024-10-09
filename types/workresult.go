package types

import (
	"encoding/json"
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

func (R Result) Encode() []byte {
	if R.Err == RESULT_OK {
		ok_byte := R.Ok
		encodedOk, err := Encode(ok_byte)
		if err != nil {
			return nil
		}
		return append([]byte{0}, encodedOk...)
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
		ok_byte, l, err := Decode(data[length:], reflect.TypeOf([]byte{}))
		if err != nil {
			return nil, length
		}
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

func (a *WorkResult) UnmarshalJSON(data []byte) error {
	var s struct {
		Service     uint32                 `json:"service"`
		CodeHash    common.Hash            `json:"code_hash"`
		PayloadHash common.Hash            `json:"payload_hash"`
		GasRatio    uint64                 `json:"gas_ratio"`
		Result      map[string]interface{} `json:"result"`
	}
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}

	var result Result
	if _, ok := s.Result["ok"]; ok {
		result = Result{
			Ok:  common.FromHex(s.Result["ok"].(string)),
			Err: RESULT_OK,
		}
	}
	if _, ok := s.Result["out-of-gas"]; ok {
		result = Result{
			Ok:  nil,
			Err: RESULT_OOG,
		}
	}
	if _, ok := s.Result["panic"]; ok {
		result = Result{
			Ok:  nil,
			Err: RESULT_PANIC,
		}
	}
	if _, ok := s.Result["bad-code"]; ok {
		result = Result{
			Ok:  nil,
			Err: RESULT_BAD_CODE,
		}
	}
	if _, ok := s.Result["code-oversize"]; ok {
		result = Result{
			Ok:  nil,
			Err: RESULT_OOB,
		}
	}

	a.Service = s.Service
	a.CodeHash = s.CodeHash
	a.PayloadHash = s.PayloadHash
	a.GasRatio = s.GasRatio
	a.Result = result

	return nil
}

func (a WorkResult) MarshalJSON() ([]byte, error) {
	var result map[string]interface{}
	if a.Result.Err == RESULT_OK {
		result = map[string]interface{}{
			"ok": common.HexString(a.Result.Ok),
		}
	} else {
		switch a.Result.Err {
		case RESULT_OOG:
			result = map[string]interface{}{
				"out-of-gas": nil,
			}
		case RESULT_PANIC:
			result = map[string]interface{}{
				"panic": nil,
			}
		case RESULT_BAD_CODE:
			result = map[string]interface{}{
				"bad-code": nil,
			}
		case RESULT_OOB:
			result = map[string]interface{}{
				"code-oversize": nil,
			}
		}
	}

	return json.Marshal(&struct {
		Service     uint32                 `json:"service"`
		CodeHash    common.Hash            `json:"code_hash"`
		PayloadHash common.Hash            `json:"payload_hash"`
		GasRatio    uint64                 `json:"gas_ratio"`
		Result      map[string]interface{} `json:"result"`
	}{
		Service:     a.Service,
		CodeHash:    a.CodeHash,
		PayloadHash: a.PayloadHash,
		GasRatio:    a.GasRatio,
		Result:      result,
	})
}
