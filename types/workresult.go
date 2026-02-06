package types

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/jam-duna/jamduna/common"
)

const (
	WORKDIGEST_OK = 0

	//{∞, ☇, ⊚, ⊖, BAD, BIG}
	WORKDIGEST_OOG        = 1
	WORKDIGEST_PANIC      = 2
	WORKDIGEST_BAD_EXPORT = 3
	WORKDIGEST_OVERSIZE   = 4
	WORKDIGEST_BAD        = 5
	WORKDIGEST_BIG        = 6

	// WORKRESULT aliases for WORKDIGEST constants
	WORKRESULT_OK    = WORKDIGEST_OK
	WORKRESULT_OOG   = WORKDIGEST_OOG
	WORKRESULT_PANIC = WORKDIGEST_PANIC
)

var HostResultCodeToString map[uint8]string = map[uint8]string{
	WORKDIGEST_OK:    "halt",
	WORKDIGEST_OOG:   "out-of-gas",
	WORKDIGEST_PANIC: "panic",
}

var ResultCodeToString map[uint8]string = map[uint8]string{
	WORKDIGEST_OK:    "halt",
	WORKDIGEST_OOG:   "out-of-gas",
	WORKDIGEST_PANIC: "panic",
	// RESULT_HOST:  "host-call",
	//RESULT_BAD_EXPORT: "bad-exports",
	//RESULT_BAD:        "bad-code",
	//RESULT_BIG: "code-oversize",
	//RESULT_HALT:       "halt",
	//RESULT_PANIC:      "panic",
	// RESULT_FAULT: "page-fault",
}

func ResultCode(code uint8) string {
	str, ok := ResultCodeToString[code]
	if ok {
		return str
	}
	return fmt.Sprintf("unknown(%d)", code)
}

// 11.1.4. Work Digest 11.6 C.26
type WorkDigest struct {
	ServiceID           uint32      `json:"service_id"`      // s the index of the service whose state is to be altered and thus whose refine code was already executed
	CodeHash            common.Hash `json:"code_hash"`       // c the hash of the code of the service at the time of being reported
	PayloadHash         common.Hash `json:"payload_hash"`    // y the hash of the payload (y) within the work item which was executed in the refine stage to give this result.
	Gas                 uint64      `json:"accumulate_gas"`  // g the accumulate gas limit for executing this item’s accumulate
	Result              Result      `json:"result"`          // r output blob or error code
	GasUsed             uint        `json:"gas_used"`        // u the actual amount of gas used during refinement; i and e the number of segments
	NumImportedSegments uint        `json:"imports"`         // i
	NumExtrinsics       uint        `json:"extrinsic_count"` // x
	NumBytesExtrinsics  uint        `json:"extrinsic_size"`  // z
	NumExportedSegments uint        `json:"exports"`         // e
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
	if R.Err == WORKDIGEST_OK {
		ok_byte := R.Ok
		encodedOk, err := Encode(ok_byte)
		if err != nil {
			return nil
		}
		return append([]byte{0}, encodedOk...)
	} else {
		switch R.Err {
		case WORKDIGEST_OOG:
			return []byte{1}
		case WORKDIGEST_PANIC:
			return []byte{2}
		case WORKDIGEST_BAD_EXPORT:
			return []byte{3}
		case WORKDIGEST_OVERSIZE:
			return []byte{4}
		case WORKDIGEST_BAD:
			return []byte{5}
		case WORKDIGEST_BIG:
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
			Err: WORKDIGEST_OK,
		}, length + l
	case 1:
		return Result{
			Ok:  nil,
			Err: WORKDIGEST_OOG,
		}, length
	case 2:
		return Result{
			Ok:  nil,
			Err: WORKDIGEST_PANIC,
		}, length
	case 3:
		return Result{
			Ok:  nil,
			Err: WORKDIGEST_BAD_EXPORT,
		}, length
	case 4:
		return Result{
			Ok:  nil,
			Err: WORKDIGEST_OVERSIZE,
		}, length
	case 5:
		return Result{
			Ok:  nil,
			Err: WORKDIGEST_BAD,
		}, length
	case 6:
		return Result{
			Ok:  nil,
			Err: WORKDIGEST_BIG,
		}, length
	default:
		return Result{
			Ok:  nil,
			Err: data[0],
		}, length
	}
}

func (a *WorkDigest) UnmarshalJSON(data []byte) error {
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

	for key, value := range s.Result {
		switch key {
		case "ok":
			valStr, ok := value.(string)
			if !ok {
				return fmt.Errorf("expected string for 'ok' result, got %T", value)
			}
			result = Result{
				Ok:  common.FromHex(valStr),
				Err: WORKDIGEST_OK,
			}
		case "out-of-gas", "out_of_gas":
			result = Result{Ok: nil, Err: WORKDIGEST_OOG}
		case "panic":
			result = Result{Ok: nil, Err: WORKDIGEST_PANIC}
		case "bad-exports", "bad_exports":
			result = Result{Ok: nil, Err: WORKDIGEST_BAD_EXPORT}
		case "bad-code", "bad_code":
			result = Result{Ok: nil, Err: WORKDIGEST_BAD}
		case "code-oversize", "code_oversize":
			result = Result{Ok: nil, Err: WORKDIGEST_BIG}
		default:
			return fmt.Errorf("unknown result key found: %s", key)
		}
		break
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

func (a WorkDigest) MarshalJSON() ([]byte, error) {
	var result map[string]interface{}
	if a.Result.Err == WORKDIGEST_OK {
		result = map[string]interface{}{
			"ok": common.HexString(a.Result.Ok),
		}
	} else {
		switch a.Result.Err {
		case WORKDIGEST_OOG:
			result = map[string]interface{}{
				"out-of-gas": nil,
			}
		case WORKDIGEST_PANIC:
			result = map[string]interface{}{
				"panic": nil,
			}
		case WORKDIGEST_BAD_EXPORT:
			result = map[string]interface{}{
				"bad-exports": nil,
			}
		// came back refactor when test vector is 0.6.7
		case WORKDIGEST_BAD:
			result = map[string]interface{}{
				"bad-code": nil,
			}
		case WORKDIGEST_BIG:
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
func (a *WorkDigest) String() string {
	return ToJSON(a)
}
func (a *Result) String() string {
	return ToJSONHex(a)
}
