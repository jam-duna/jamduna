package types

import (
	"encoding/json"
	"reflect"

	"github.com/colorfulnotion/jam/common"
)

const (
	RESULT_OK       = 0 //if o ∈ Y
	RESULT_OOG      = 1 //if o = ∞
	RESULT_PANIC    = 2 //if o = ☇
	RESULT_BAD_CODE = 3 //if o = ⊚
	RESULT_OVERSIZE = 4 //if o = ⊖
	RESULT_OOB      = 5 //if o = BAD
	RESULT_FAULT    = 6 //if o = BIG
)

const (
	PVM_HALT  = 0 // regular halt ∎
	PVM_PANIC = 1 // panic ☇
	PVM_FAULT = 2 // out-of-gas ∞
	PVM_HOST  = 3 // host-call̵ h
	PVM_OOG   = 4 // page-fault F
)

// 11.1.4. Work Result. Equation 11.6. We finally come to define a work result, L, which is the data conduit by which services’ states may be altered through the computation done within a work-package.
type WorkResult struct {
	ServiceID   uint32      `json:"service_id"`
	CodeHash    common.Hash `json:"code_hash"`
	PayloadHash common.Hash `json:"payload_hash"`
	Gas         uint64      `json:"accumulate_gas"`
	Result      Result      `json:"result"`
	// NEW in 0.6.4 -- see C.23 which specifies ordering of { u, i, x, z, e }
	GasUsed             uint `json:"gas_used"`        // u
	NumImportedSegments uint `json:"imports"`         // i
	NumExtrinsics       uint `json:"extrinsic_count"` // x
	NumBytesExtrinsics  uint `json:"extrinsic_size"`  // z
	NumExportedSegments uint `json:"exports"`         // e
}

type RefineLoad struct {
	GasUsed             uint `json:"gas_used"`        // u
	NumImportedSegments uint `json:"imports"`         // i
	NumExtrinsics       uint `json:"extrinsic_count"` // x
	NumBytesExtrinsics  uint `json:"extrinsic_size"`  // z
	NumExportedSegments uint `json:"exports"`         // e
}

type Result struct {
	Ok  []byte `json:"ok,omitempty"`
	Err uint8  `json:"err,omitempty"`
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
		case RESULT_OVERSIZE:
			return []byte{4}
		case RESULT_OOB:
			return []byte{5}
		case RESULT_FAULT:
			return []byte{6}
		default:
			return []byte{R.Err}
		}
	}
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
	case 5:
		return Result{
			Ok:  nil,
			Err: RESULT_FAULT,
		}, length
	default:
		return Result{
			Ok:  nil,
			Err: data[0],
		}, length
	}
}

func (a *WorkResult) UnmarshalJSON(data []byte) error {
	var s struct {
		ServiceID   uint32                 `json:"service_id"`
		CodeHash    common.Hash            `json:"code_hash"`
		PayloadHash common.Hash            `json:"payload_hash"`
		Gas         uint64                 `json:"accumulate_gas"`
		Result      map[string]interface{} `json:"result"`
		RefineLoad  RefineLoad             `json:"refine_load"`
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

	a.ServiceID = s.ServiceID
	a.CodeHash = s.CodeHash
	a.PayloadHash = s.PayloadHash
	a.Gas = s.Gas
	a.Result = result
	a.GasUsed = s.RefineLoad.GasUsed
	a.NumImportedSegments = s.RefineLoad.NumImportedSegments
	a.NumExtrinsics = s.RefineLoad.NumExtrinsics
	a.NumBytesExtrinsics = s.RefineLoad.NumBytesExtrinsics
	a.NumExportedSegments = s.RefineLoad.NumExportedSegments
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
		ServiceID   uint32                 `json:"service_id"`
		CodeHash    common.Hash            `json:"code_hash"`
		PayloadHash common.Hash            `json:"payload_hash"`
		Gas         uint64                 `json:"accumulate_gas"`
		Result      map[string]interface{} `json:"result"`
		RefineLoad  RefineLoad             `json:"refine_load"`
	}{
		ServiceID:   a.ServiceID,
		CodeHash:    a.CodeHash,
		PayloadHash: a.PayloadHash,
		Gas:         a.Gas,
		Result:      result,
		RefineLoad: RefineLoad{
			GasUsed:             a.GasUsed,
			NumImportedSegments: a.NumImportedSegments,
			NumExtrinsics:       a.NumExtrinsics,
			NumBytesExtrinsics:  a.NumBytesExtrinsics,
			NumExportedSegments: a.NumExportedSegments,
		},
	})
}

// helper function to print the WorkReport
func (a *WorkResult) String() string {
	return ToJSON(a)
}
func (a *Result) String() string {
	return ToJSON(a)
}
