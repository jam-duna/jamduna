package types

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/colorfulnotion/jam/common"
)

const (
	WORKRESULT_OK = 0

	//{∞, ☇, ⊚, ⊖, BAD, BIG}
	WORKRESULT_OOG        = 1
	WORKRESULT_PANIC      = 2
	WORKRESULT_BAD_EXPORT = 3
	WORKRESULT_OVERSIZE   = 4
	WORKRESULT_BAD        = 5
	WORKRESULT_BIG        = 6
)

var HostResultCodeToString map[uint8]string = map[uint8]string{
	WORKRESULT_OK:    "halt",
	WORKRESULT_OOG:   "out-of-gas",
	WORKRESULT_PANIC: "panic",
}

func ResultCode(code uint8) string {
	str, ok := HostResultCodeToString[code]
	if ok {
		return str
	}
	return fmt.Sprintf("unknown(%d)", code)
}

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
	if R.Err == WORKRESULT_OK {
		ok_byte := R.Ok
		encodedOk, err := Encode(ok_byte)
		if err != nil {
			return nil
		}
		return append([]byte{0}, encodedOk...)
	} else {
		switch R.Err {
		case WORKRESULT_OOG:
			return []byte{1}
		case WORKRESULT_PANIC:
			return []byte{2}
		case WORKRESULT_BAD_EXPORT:
			return []byte{3}
		case WORKRESULT_OVERSIZE:
			return []byte{4}
		case WORKRESULT_BAD:
			return []byte{5}
		case WORKRESULT_BIG:
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
			Err: WORKRESULT_OK,
		}, length + l
	case 1:
		return Result{
			Ok:  nil,
			Err: WORKRESULT_OOG,
		}, length
	case 2:
		return Result{
			Ok:  nil,
			Err: WORKRESULT_PANIC,
		}, length
	case 3:
		return Result{
			Ok:  nil,
			Err: WORKRESULT_BAD_EXPORT,
		}, length
	case 4:
		return Result{
			Ok:  nil,
			Err: WORKRESULT_OVERSIZE,
		}, length
	case 5:
		return Result{
			Ok:  nil,
			Err: WORKRESULT_BAD,
		}, length
	case 6:
		return Result{
			Ok:  nil,
			Err: WORKRESULT_BIG,
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
			Err: WORKRESULT_OK,
		}
	}
	if _, ok := s.Result["out-of-gas"]; ok {
		result = Result{
			Ok:  nil,
			Err: WORKRESULT_OOG,
		}
	}
	if _, ok := s.Result["panic"]; ok {
		result = Result{
			Ok:  nil,
			Err: WORKRESULT_PANIC,
		}
	}
	if _, ok := s.Result["bad-exports"]; ok {
		result = Result{
			Ok:  nil,
			Err: WORKRESULT_BAD_EXPORT,
		}
	}
	// came back refactor when test vector is 0.6.7
	if _, ok := s.Result["bad-code"]; ok {
		result = Result{
			Ok:  nil,
			Err: WORKRESULT_BAD,
		}
	}
	if _, ok := s.Result["code-oversize"]; ok {
		result = Result{
			Ok:  nil,
			Err: WORKRESULT_BIG,
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
	if a.Result.Err == WORKRESULT_OK {
		result = map[string]interface{}{
			"ok": common.HexString(a.Result.Ok),
		}
	} else {
		switch a.Result.Err {
		case WORKRESULT_OOG:
			result = map[string]interface{}{
				"out-of-gas": nil,
			}
		case WORKRESULT_PANIC:
			result = map[string]interface{}{
				"panic": nil,
			}
		case WORKRESULT_BAD_EXPORT:
			result = map[string]interface{}{
				"bad-exports": nil,
			}
		// came back refactor when test vector is 0.6.7
		case WORKRESULT_BAD:
			result = map[string]interface{}{
				"bad-code": nil,
			}
		case WORKRESULT_BIG:
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
	return ToJSONHex(a)
}
