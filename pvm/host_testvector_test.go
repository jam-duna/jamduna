//go:build network_test
// +build network_test

package pvm

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

const show_detail = false

type ByteSlice []byte

// Gas related constants
var (
	InitialGas  = uint64(10000)
	ExpectedGas = uint64(9990)
)

// Service account related constants
// raw key, value
var (
	StorageKey      = ByteSlice{0, 0, 0, 0}
	StorageKeyLen   = uint32(len(StorageKey))
	StorageValue    = ByteSlice{0, 0, 0, 0}
	StorageValueLen = uint32(len(StorageValue))

	PreimageBlob          = ByteSlice{15, 15, 14, 14, 13, 13, 12, 12, 11, 11, 10, 10}
	PreimageBlobHash      = common.Blake2Hash(PreimageBlob)
	PreimageBlobHashBytes = PreimageBlobHash.Bytes()
	PreimageBlobLen       = uint32(len(PreimageBlob))

	LookupValue = []uint32{100000}

	Service_0_Balance   = uint64(1000)
	Service_0_GasLimitG = uint64(100)
	Service_0_GasLimitM = uint64(200)

	Service_1_Balance   = uint64(1000)
	Service_1_GasLimitG = uint64(100)
	Service_1_GasLimitM = uint64(100)
)

// internal key, value (for service 0) (Not apply D.1 C)
var (
	ServiceIndex_0       = uint32(0)
	ServiceIndex_0_Bytes = make(ByteSlice, 4)
)

func init() {
	binary.LittleEndian.PutUint32(ServiceIndex_0_Bytes, ServiceIndex_0)
}

var (
	Service_0_StorageKeyForHost         = common.Blake2Hash(append(ServiceIndex_0_Bytes, StorageKey...))
	Service_0_StorageKeyInternal        = common.Compute_storageKey_internal(Service_0_StorageKeyForHost)
	Service_0_PreimageBLobKeyForHost    = common.Blake2Hash(PreimageBlobHashBytes)
	Service_0_PreimageBLobKeyInternal   = common.Compute_preimageBlob_internal(Service_0_PreimageBLobKeyForHost)
	Service_0_PreimageLookupKeyInternal = common.Compute_preimageLookup_internal(PreimageBlobHash, PreimageBlobLen)
)

// internal key, value (for service 1) (Not apply D.1 C)
var (
	ServiceIndex_1       = uint32(1)
	ServiceIndex_1_Bytes = make(ByteSlice, 4)
)

func init() {
	binary.LittleEndian.PutUint32(ServiceIndex_1_Bytes, ServiceIndex_1)
}

var (
	Service_1_StorageKeyForHost         = common.Blake2Hash(append(ServiceIndex_1_Bytes, StorageKey...))
	Service_1_StorageKeyInternal        = common.Compute_storageKey_internal(Service_1_StorageKeyForHost)
	Service_1_PreimageBlobKeyForHost    = common.Blake2Hash(PreimageBlobHashBytes)
	Service_1_PreimageBlobKeyInternal   = common.Compute_preimageBlob_internal(Service_1_PreimageBlobKeyForHost)
	Service_1_PreimageLookupKeyInternal = common.Compute_preimageLookup_internal(PreimageBlobHash, PreimageBlobLen)
)

// serialized key, value (for service 0) (Apply D.1 C)
var (
	Service_0_StorageKeySerialized       = common.ComputeC_sh(ServiceIndex_0, Service_0_StorageKeyInternal)
	Service_0_PrimageBlobKeySerialized   = common.ComputeC_sh(ServiceIndex_0, Service_0_PreimageBLobKeyInternal)
	Service_0_PrimageLookupKeySerialized = common.ComputeC_sh(ServiceIndex_0, Service_0_PreimageLookupKeyInternal)
)

// serialized key, value (for service 1) (Apply D.1 C)
var (
	Service_1_StorageKeySerialized       = common.ComputeC_sh(ServiceIndex_1, Service_1_StorageKeyInternal)
	Service_1_PrimageBlobKeySerialized   = common.ComputeC_sh(ServiceIndex_1, Service_1_PreimageBlobKeyInternal)
	Service_1_PrimageLookupKeySerialized = common.ComputeC_sh(ServiceIndex_1, Service_1_PreimageLookupKeyInternal)
)

// Service account 0
var (
	ServiceAccount_0 = &types.ServiceAccount{
		Storage: map[common.Hash]types.StorageObject{
			Service_0_StorageKeyForHost: {
				Value: StorageValue,
			},
		},
		Lookup: map[common.Hash]types.LookupObject{
			PreimageBlobHash: {
				T: LookupValue,
				Z: PreimageBlobLen,
			},
		},
		Preimage: map[common.Hash]types.PreimageObject{
			Service_0_PreimageBLobKeyForHost: {
				Preimage: PreimageBlob,
			},
		},
		CodeHash:  PreimageBlobHash,
		Balance:   Service_0_Balance,
		GasLimitG: Service_0_GasLimitG,
		GasLimitM: Service_0_GasLimitM,
	}
	t                 = ServiceAccount_0
	e                 = []interface{}{t.CodeHash, t.Balance, t.ComputeThreshold(), t.GasLimitG, t.GasLimitM, t.NumStorageItems, t.StorageSize}
	m_for_hostinfo, _ = types.Encode(e)
)

// Service account 1
var (
	ServiceAccount_1 = &types.ServiceAccount{
		Storage: map[common.Hash]types.StorageObject{
			Service_1_StorageKeyForHost: {
				Value: StorageValue,
			},
		},
		Lookup: map[common.Hash]types.LookupObject{
			PreimageBlobHash: {
				T: LookupValue,
				Z: PreimageBlobLen,
			},
		},
		Preimage: map[common.Hash]types.PreimageObject{
			Service_1_PreimageBlobKeyForHost: {
				Preimage: PreimageBlob,
			},
		},
		CodeHash:  PreimageBlobHash,
		Balance:   Service_1_Balance,
		GasLimitG: Service_1_GasLimitG,
		GasLimitM: Service_1_GasLimitM,
	}
)

// WorkPackage related constants
var (
	WorkPackage        types.WorkPackage
	EncodedWorkPackage ByteSlice
)

func init() {
	jsonData := `
	{
		"authorization": "0x0102030405",
		"auth_code_host": 305419896,
		"authorizer": {
			"code_hash": "0x022e5e165cc8bd586404257f5cd6f5a31177b5c951eb076c7c10174f90006eef",
			"params": "0x00010203040506070809"
		},
		"context": {
			"anchor": "0xc0564c5e0de0942589df4343ad1956da66797240e2a2f2d6f8116b5047768986",
			"state_root": "0xf6967658df626fa39cbfb6014b50196d23bc2cfbfa71a7591ca7715472dd2b48",
			"beefy_root": "0x9329de635d4bbb8c47cdccbbc1285e48bf9dbad365af44b205343e99dea298f3",
			"lookup_anchor": "0x60751ab5b251361fbfd3ad5b0e84f051ccece6b00830aed31a5354e00b20b9ed",
			"lookup_anchor_slot": 33,
			"prerequisites": []
		},
		"items": [
			{
				"service": 16909060,
				"code_hash": "0x70a50829851e8f6a8c80f92806ae0e95eb7c06ad064e311cc39107b3219e532e",
				"payload": "0x0102030405",
				"refine_gas_limit": 42,
				"accumulate_gas_limit": 42,
				"import_segments": [
					{
						"tree_root": "0x461236a7eb29dcffc1dd282ce1de0e0ed691fc80e91e02276fe8f778f088a1b8",
						"index": 0
					},
					{
						"tree_root": "0xe7cb536522c1c1b41fff8021055b774e929530941ea12c10f1213c56455f29ad",
						"index": 1
					},
					{
						"tree_root": "0xb0a487a4adf6a0eda5d69ddd2f8b241cf44204f0ff793e993e5e553b7862a1dc",
						"index": 2
					}
				],
				"extrinsic": [
					{
						"hash": "0x381a0e351c5593018bbc87dd6694695caa1c0c1ddb24e70995da878d89495bf1",
						"len": 16
					},
					{
						"hash": "0x6c437d85cd8327f42a35d427ede1b5871347d3aae7442f2df1ff80f834acf17a",
						"len": 17
					}
				],
				"export_count": 4
			},
			{
				"service": 84281096,
				"code_hash": "0xfcfc857dab216daf41f409c2012685846e4d34aedfeacaf84d9adfebda73fae6",
				"payload": "0x030201",
				"refine_gas_limit": 77,
				"accumulate_gas_limit": 77,
				"import_segments": [
					{
						"tree_root": "0x3e5d0bea78537414bd1cfdaeb0f22d743bcaba5dbffacbabce8457f4cd78f69b",
						"index": 0
					},
					{
						"tree_root": "0xb7f8dffa65971832ec9e19719debc04b1ccd9ad27187a4943807ca756962481b",
						"index": 1
					}
				],
				"extrinsic": [
					{
						"hash": "0x80b628780612e8928705018d1ced53b2f76607ad026a86e4f36c99ac0491f8eb",
						"len": 32
					},
					{
						"hash": "0x47f142b4488bbd34e59afc60a0daedc6020e8be52a3cedecbd75e93ee9908adf",
						"len": 33
					},
					{
						"hash": "0xcc2f47030ff8a9fe8d02e8eb87e86d4db05b57258d30f9d81acb3b280d04e877",
						"len": 34
					},
					{
						"hash": "0x8b57a796a5d07cb04cc1614dfc2acb3f73edc712d7f433619ca3bbe66bb15f49",
						"len": 10
					}
				],
				"export_count": 7
			}
		]
	}
	`
	err := json.Unmarshal([]byte(jsonData), &WorkPackage)
	if err != nil {
		fmt.Println("Error parsing JSON:", err)
		return
	}

	EncodedWorkPackage, _ = types.Encode(WorkPackage)
}

func TestWorkPackageParse(t *testing.T) {
	fmt.Printf("WorkPackage: %s\n", WorkPackage.String())

	// show hash(extrinsic)
	fmt.Printf("common.Blake2Hash(extrinsic): %s\n", common.Blake2Hash(Extrinsic).String())
}

// import segemt
var (
	ImportSegment = ByteSlice{10, 10, 11, 11, 12, 12, 13, 13, 14, 14, 15, 15}
)

// authorizer output
var (
	AuthorizerOutput = ByteSlice{9, 8, 7, 6, 5, 4, 3, 2, 1, 0}
)

// extrinsic
var (
	Extrinsic = ByteSlice{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
)

// Variables for machine related test vectors
var (
	// Initial machine pc
	init_program_counter = uint64(0)

	// Useful program and expected values
	stote_u32_program     = ByteSlice{0, 0, 5, 61, 7, 0, 0, 2, 1} // u32 [0x20000] = a0
	expected_store_u32_pc = uint64(5)

	trap_program     = ByteSlice{0, 0, 1, 0, 1} // call trap to cause a PANIC
	expected_trap_pc = uint64(0)

	host_gas_program     = ByteSlice{0, 0, 1, 10, 1} // call host_gas to cause a HOST or OOG
	expected_host_gas_pc = uint64(1)
)

// HALT
var (
	HALT_program                 = stote_u32_program // used to cause HALT
	HALT_init_machine_gas        = uint64(10)
	HALT_init_machine_gas_bytes  = make(ByteSlice, 8)
	HALT_init_machine_regs       = []uint64{0, 0, 0, 0, 0, 0, 0, 100, 0, 0, 0, 0, 0}
	HALT_init_machine_regs_bytes = make(ByteSlice, 104)
	HALT_init_pc                 = uint64(0)
	HALT_init_machine_pages      = map[uint32]*PageForTest{
		32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
	}

	HALT_expected_machine_gas        = uint64(10)
	HALT_expected_machine_gas_bytes  = make(ByteSlice, 8)
	HALT_expected_machine_regs       = []uint64{0, 0, 0, 0, 0, 0, 0, 100, 0, 0, 0, 0, 0}
	HALT_expected_machine_regs_bytes = make(ByteSlice, 104)
	HALT_expected_pc                 = expected_store_u32_pc
	HALT_expected_machine_pages      = map[uint32]*PageForTest{
		32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
	}
)

func init() {
	binary.LittleEndian.PutUint64(HALT_init_machine_gas_bytes, HALT_init_machine_gas)
	for i, reg := range HALT_init_machine_regs {
		binary.LittleEndian.PutUint64(HALT_init_machine_regs_bytes[i*8:], reg)
	}
	binary.LittleEndian.PutUint64(HALT_expected_machine_gas_bytes, HALT_expected_machine_gas)
	for i, reg := range HALT_expected_machine_regs {
		binary.LittleEndian.PutUint64(HALT_expected_machine_regs_bytes[i*8:], reg)
	}
}

// PANIC
var (
	PANIC_program                 = trap_program // used to cause PANIC
	PANIC_init_machine_gas        = uint64(10)
	PANIC_init_machine_gas_bytes  = make(ByteSlice, 8)
	PANIC_init_machine_regs       = []uint64{0, 0, 0, 0, 0, 0, 0, 100, 0, 0, 0, 0, 0}
	PANIC_init_machine_regs_bytes = make(ByteSlice, 104)
	PANIC_init_pc                 = init_program_counter
	PANIC_init_machine_pages      = map[uint32]*PageForTest{
		32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
	}

	PANIC_expected_machine_gas        = PANIC_init_machine_gas
	PANIC_expected_machine_gas_bytes  = make(ByteSlice, 8)
	PANIC_expected_machine_regs       = []uint64{0, 0, 0, 0, 0, 0, 0, 100, 0, 0, 0, 0, 0}
	PANIC_expected_machine_regs_bytes = make(ByteSlice, 104)
	PANIC_expected_pc                 = expected_trap_pc
	PANIC_expected_machine_pages      = map[uint32]*PageForTest{
		32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
	}
)

func init() {
	binary.LittleEndian.PutUint64(PANIC_init_machine_gas_bytes, PANIC_init_machine_gas)
	for i, reg := range PANIC_init_machine_regs {
		binary.LittleEndian.PutUint64(PANIC_init_machine_regs_bytes[i*8:], reg)
	}
	binary.LittleEndian.PutUint64(PANIC_expected_machine_gas_bytes, PANIC_expected_machine_gas)
	for i, reg := range PANIC_expected_machine_regs {
		binary.LittleEndian.PutUint64(PANIC_expected_machine_regs_bytes[i*8:], reg)
	}
}

// OOG
var (
	OOG_program                 = host_gas_program
	OOG_init_machine_gas        = uint64(5) // used to cause OOG
	OOG_init_machine_gas_bytes  = make(ByteSlice, 8)
	OOG_init_machine_regs       = []uint64{0, 0, 0, 0, 0, 0, 0, 100, 0, 0, 0, 0, 0}
	OOG_init_machine_regs_bytes = make(ByteSlice, 104)
	OOG_init_pc                 = init_program_counter
	OOG_init_machine_pages      = map[uint32]*PageForTest{
		32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
	}

	OOG_expected_machine_gas        = OOG_init_machine_gas
	OOG_expected_machine_gas_bytes  = make(ByteSlice, 8)
	OOG_expected_machine_regs       = []uint64{0, 0, 0, 0, 0, 0, 0, 100, 0, 0, 0, 0, 0}
	OOG_expected_machine_regs_bytes = make(ByteSlice, 104)
	OOG_expected_pc                 = expected_host_gas_pc
	OOG_expected_machine_pages      = map[uint32]*PageForTest{
		32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
	}
)

func init() {
	binary.LittleEndian.PutUint64(OOG_init_machine_gas_bytes, OOG_init_machine_gas)
	for i, reg := range OOG_init_machine_regs {
		binary.LittleEndian.PutUint64(OOG_init_machine_regs_bytes[i*8:], reg)
	}
	binary.LittleEndian.PutUint64(OOG_expected_machine_gas_bytes, OOG_expected_machine_gas)
	for i, reg := range OOG_expected_machine_regs {
		binary.LittleEndian.PutUint64(OOG_expected_machine_regs_bytes[i*8:], reg)
	}
}

// FAULT
var (
	FAULT_program                 = stote_u32_program
	FAULT_init_machine_gas        = uint64(10)
	FAULT_init_machine_gas_bytes  = make(ByteSlice, 8)
	FAULT_init_machine_regs       = []uint64{0, 0, 0, 0, 0, 0, 0, 100, 0, 0, 0, 0, 0}
	FAULT_init_machine_regs_bytes = make(ByteSlice, 104)
	FAULT_init_pc                 = uint64(0)
	FAULT_init_machine_pages      = map[uint32]*PageForTest{
		32: {Value: make(ByteSlice, 0), Access: AccessMode{Inaccessible: true}}, // cause FAULT
	}

	FAULT_expected_machine_gas        = uint64(10)
	FAULT_expected_machine_gas_bytes  = make(ByteSlice, 8)
	FAULT_expected_machine_regs       = []uint64{0, 0, 0, 0, 0, 0, 0, 100, 0, 0, 0, 0, 0}
	FAULT_expected_machine_regs_bytes = make(ByteSlice, 104)
	FAULT_expected_pc                 = FAULT_init_pc
	FAULT_expected_machine_pages      = map[uint32]*PageForTest{
		32: {Value: make(ByteSlice, 0), Access: AccessMode{Inaccessible: true}}, // cause FAULT
	}
)

func init() {
	binary.LittleEndian.PutUint64(FAULT_init_machine_gas_bytes, FAULT_init_machine_gas)
	for i, reg := range FAULT_init_machine_regs {
		binary.LittleEndian.PutUint64(FAULT_init_machine_regs_bytes[i*8:], reg)
	}
	binary.LittleEndian.PutUint64(FAULT_expected_machine_gas_bytes, FAULT_expected_machine_gas)
	for i, reg := range FAULT_expected_machine_regs {
		binary.LittleEndian.PutUint64(FAULT_expected_machine_regs_bytes[i*8:], reg)
	}
}

// HOST
var (
	HOST_program                 = host_gas_program
	HOST_init_machine_gas        = uint64(10)
	HOST_init_machine_gas_bytes  = make(ByteSlice, 8)
	HOST_init_machine_regs       = []uint64{0, 0, 0, 0, 0, 0, 0, 100, 0, 0, 0, 0, 0}
	HOST_init_machine_regs_bytes = make(ByteSlice, 104)
	HOST_init_pc                 = uint64(0)
	HOST_init_machine_pages      = map[uint32]*PageForTest{
		32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
	}

	HOST_expected_machine_gas        = uint64(0)
	HOST_expected_machine_gas_bytes  = make(ByteSlice, 8)
	HOST_expected_machine_regs       = []uint64{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	HOST_expected_machine_regs_bytes = make(ByteSlice, 104)
	HOST_expected_pc                 = expected_host_gas_pc
	HOST_expected_machine_pages      = map[uint32]*PageForTest{
		32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
	}
)

func init() {
	binary.LittleEndian.PutUint64(HOST_init_machine_gas_bytes, HOST_init_machine_gas)
	for i, reg := range HOST_init_machine_regs {
		binary.LittleEndian.PutUint64(HOST_init_machine_regs_bytes[i*8:], reg)
	}

	binary.LittleEndian.PutUint64(HOST_expected_machine_gas_bytes, HOST_expected_machine_gas)
	for i, reg := range HOST_expected_machine_regs {
		binary.LittleEndian.PutUint64(HOST_expected_machine_regs_bytes[i*8:], reg)
	}
}

func (b ByteSlice) MarshalJSON() ([]byte, error) {
	arr := make([]int, len(b))
	for i, v := range b {
		arr[i] = int(v)
	}
	return json.Marshal(arr)
}

func (b *ByteSlice) UnmarshalJSON(data []byte) error {
	var arr []int
	if err := json.Unmarshal(data, &arr); err != nil {
		return err
	}
	*b = make([]byte, len(arr))
	for i, v := range arr {
		(*b)[i] = byte(v)
	}
	return nil
}

type LookupObjectForTest struct {
	T []uint32 `json:"t"`
	L uint32   `json:"l"`
}
type RAMForTest struct {
	Pages map[uint32]*PageForTest `json:"pages"` // The pages in the RAM
}
type ServiceAccountForTest struct {
	// Need serviceIndex here
	Storage  map[string]ByteSlice           `json:"s_map"`
	Lookup   map[string]LookupObjectForTest `json:"l_map"`
	Preimage map[string]ByteSlice           `json:"p_map"`

	CodeHash string `json:"code_hash"`
	Balance  uint64 `json:"balance"`
	// GasLimitG uint64 `json:"min_item_gas"`
	// GasLimitM uint64 `json:"min_memo_gas"`
	GasLimitG uint64 `json:"g"`
	GasLimitM uint64 `json:"m"`
}

type PageForTest struct {
	Value  ByteSlice  `json:"value"`  // The data stored in the page
	Access AccessMode `json:"access"` // The access mode of the page
}

type RefineMForTest struct {
	P ByteSlice   `json:"P"`
	U *RAMForTest `json:"U"`
	I uint64      `json:"I"`
}

type RefineM_mapForTest map[uint32]*RefineMForTest

type PartialStateForTest struct {
	D map[uint32]*ServiceAccountForTest `json:"D"`
	I types.Validators                  `json:"I"`
	Q types.AuthorizationQueue          `json:"Q"`
	X types.Kai_state                   `json:"X"`
}

type DeferredTransferForTest struct {
	SenderIndex   uint32    `json:"sender_index"`
	ReceiverIndex uint32    `json:"receiver_index"`
	Amount        uint64    `json:"amount"`
	Memo          ByteSlice `json:"memo"`
	GasLimit      uint64    `json:"gas_limit"`
}

type XContextForTest struct {
	I uint32                    `json:"I"`
	S uint32                    `json:"S"`
	U *PartialStateForTest      `json:"U"`
	T []DeferredTransferForTest `json:"T"`
	Y string                    `json:"Y"`
}

type RefineTestcase struct {
	Name          string            `json:"name"`
	InitialGas    uint64            `json:"initial-gas"`
	InitialRegs   map[uint32]uint64 `json:"initial-regs"`
	InitialMemory RAMForTest        `json:"initial-memory"`

	InitialRefineM_map   RefineM_mapForTest `json:"initial-refine-map"` // m in refine function
	InitialExportSegment []ByteSlice        `json:"initial-export-segment"`

	InitialWorkItemIndex    uint32            `json:"initial-work-item-index"`
	InitialWorkPackage      types.WorkPackage `json:"initial-work-package"`
	InitialAuthorizerOutput ByteSlice         `json:"initial-authorizer-output"`
	InitialExtrinsic        ByteSlice         `json:"initial-extrinsic"`

	InitialImportSegment    []ByteSlice                       `json:"initial-import-segment"`
	InitialExportSegmentIdx uint32                            `json:"initial-export-segment-index"`
	InitialServiceIndex     uint32                            `json:"initial-service-index"`
	InitialDelta            map[uint32]*ServiceAccountForTest `json:"initial-delta"`
	InitialTimeslot         uint32                            `json:"initial-timeslot"`

	ExpectedGas        uint64            `json:"expected-gas"`
	ExpectedResultCode uint8             `json:"expected-result-code"`
	ExpectedRegs       map[uint32]uint64 `json:"expected-regs"`
	ExpectedMemory     RAMForTest        `json:"expected-memory"`

	ExpectedRefineM_map   RefineM_mapForTest `json:"expected-refine-map"`
	ExpectedExportSegment []ByteSlice        `json:"expected-export-segment"`
}

type AccumulateTestcase struct {
	Name          string            `json:"name"`
	InitialGas    uint64            `json:"initial-gas"`
	InitialRegs   map[uint32]uint64 `json:"initial-regs"`
	InitialMemory RAMForTest        `json:"initial-memory"`

	InitialXcontent_x *XContextForTest `json:"initial-xcontent-x"`
	InitialXcontent_y XContextForTest  `json:"initial-xcontent-y"`
	InitialTimeslot   uint32           `json:"initial-timeslot"`

	ExpectedGas        uint64            `json:"expected-gas"`
	ExpectedResultCode uint8             `json:"expected-result-code"`
	ExpectedRegs       map[uint32]uint64 `json:"expected-regs"`
	ExpectedMemory     RAMForTest        `json:"expected-memory"`

	ExpectedXcontent_x *XContextForTest `json:"expected-xcontent-x"`
	ExpectedXcontent_y XContextForTest  `json:"expected-xcontent-y"`
}

type GeneralTestcase struct {
	Name          string            `json:"name"`
	InitialGas    uint64            `json:"initial-gas"`
	InitialRegs   map[uint32]uint64 `json:"initial-regs"`
	InitialMemory RAMForTest        `json:"initial-memory"`

	InitialServiceAccount *ServiceAccountForTest            `json:"initial-service-account"`
	InitialServiceIndex   uint32                            `json:"initial-service-index"`
	InitialDelta          map[uint32]*ServiceAccountForTest `json:"initial-delta"`

	ExpectedGas        uint64                            `json:"expected-gas"`
	ExpectedResultCode uint8                             `json:"expected-result-code"`
	ExpectedRegs       map[uint32]uint64                 `json:"expected-regs"`
	ExpectedMemory     RAMForTest                        `json:"expected-memory"`
	ExpectedDelta      map[uint32]*ServiceAccountForTest `json:"expected-delta"`

	ExpectedServiceAccount *ServiceAccountForTest `json:"expected-service-account"`
}

var HosterrorCaseNames = map[uint64]string{
	OK:   "OK",
	OOB:  "OOB",
	NONE: "NONE",
	CASH: "CASH",
	FULL: "FULL",
	HUH:  "HUH",
	WHO:  "WHO",
	LOW:  "LOW",
}

var PVMerrorCaseNames = map[uint64]string{
	types.PVM_HALT:  "HALT",
	types.PVM_PANIC: "PANIC",
	types.PVM_FAULT: "FAULT",
	types.PVM_HOST:  "HOST",
	types.PVM_OOG:   "OOG",
}

func FindAndReadJSONFiles(dirPath, keyword string) ([]string, []string, error) {
	var matchingFiles []string
	var fileContents []string

	// Walk through the directory
	if show_detail {
		fmt.Printf("FindAndReadJSONFiles %s\n", dirPath)
	}

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Check if file contains the keyword and has .json extension
		if !info.IsDir() && strings.Contains(info.Name(), keyword) && filepath.Ext(info.Name()) == ".json" {
			if show_detail {
				fmt.Printf("%s:%s\n", keyword, info.Name())
			}

			matchingFiles = append(matchingFiles, path)
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			fileContents = append(fileContents, string(content))
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	return matchingFiles, fileContents, nil
}

type Testcase interface {
	GetInitialGas() uint64
	GetInitialRegs() map[uint32]uint64
	GetInitialMemory() RAMForTest

	GetExpectedGas() uint64
	GetExpectedResultCode() uint8
	GetExpectedRegs() map[uint32]uint64
	GetExpectedMemory() RAMForTest
	GetName() string
}

func (tc RefineTestcase) GetInitialGas() uint64              { return tc.InitialGas }
func (tc RefineTestcase) GetInitialRegs() map[uint32]uint64  { return tc.InitialRegs }
func (tc RefineTestcase) GetInitialMemory() RAMForTest       { return tc.InitialMemory }
func (tc RefineTestcase) GetExpectedGas() uint64             { return tc.ExpectedGas }
func (tc RefineTestcase) GetExpectedResultCode() uint8       { return tc.ExpectedResultCode }
func (tc RefineTestcase) GetExpectedRegs() map[uint32]uint64 { return tc.ExpectedRegs }
func (tc RefineTestcase) GetExpectedMemory() RAMForTest      { return tc.ExpectedMemory }
func (tc RefineTestcase) GetName() string                    { return tc.Name }

func (tc AccumulateTestcase) GetInitialGas() uint64              { return tc.InitialGas }
func (tc AccumulateTestcase) GetInitialRegs() map[uint32]uint64  { return tc.InitialRegs }
func (tc AccumulateTestcase) GetInitialMemory() RAMForTest       { return tc.InitialMemory }
func (tc AccumulateTestcase) GetExpectedGas() uint64             { return tc.ExpectedGas }
func (tc AccumulateTestcase) GetExpectedResultCode() uint8       { return tc.ExpectedResultCode }
func (tc AccumulateTestcase) GetExpectedRegs() map[uint32]uint64 { return tc.ExpectedRegs }
func (tc AccumulateTestcase) GetExpectedMemory() RAMForTest      { return tc.ExpectedMemory }
func (tc AccumulateTestcase) GetName() string                    { return tc.Name }

func (tc GeneralTestcase) GetInitialGas() uint64              { return tc.InitialGas }
func (tc GeneralTestcase) GetInitialRegs() map[uint32]uint64  { return tc.InitialRegs }
func (tc GeneralTestcase) GetInitialMemory() RAMForTest       { return tc.InitialMemory }
func (tc GeneralTestcase) GetExpectedGas() uint64             { return tc.ExpectedGas }
func (tc GeneralTestcase) GetExpectedResultCode() uint8       { return tc.ExpectedResultCode }
func (tc GeneralTestcase) GetExpectedRegs() map[uint32]uint64 { return tc.ExpectedRegs }
func (tc GeneralTestcase) GetExpectedMemory() RAMForTest      { return tc.ExpectedMemory }
func (tc GeneralTestcase) GetName() string                    { return tc.Name }

func InitPvmBase(vm *VM, tc Testcase) {
	vm.Gas = int64(tc.GetInitialGas())
	for i, reg := range tc.GetInitialRegs() {
		vm.WriteRegister(int(i), reg)
	}
	for page_addr, page := range tc.GetInitialMemory().Pages {
		vm.Ram.SetPageAccess(page_addr, 1, AccessMode{Writable: true})
		vm.Ram.WriteRAMBytes(page_addr*PageSize, page.Value)
		vm.Ram.SetPageAccess(page_addr, 1, page.Access)
	}
}

func InitPvmRefine(vm *VM, testcase RefineTestcase) {
	// Initialize RefineM_map
	vm.RefineM_map = make(map[uint32]*RefineM)
	vm.RefineM_map = ConvertToRefineM_map(testcase.InitialRefineM_map)

	// Initialize Export Segments
	vm.Exports = make([][]byte, len(testcase.InitialExportSegment))
	for i, bs := range testcase.InitialExportSegment {
		vm.Exports[i] = make([]byte, len(bs))
		copy(vm.Exports[i], bs)
	}

	// Initialize WorkPackage
	vm.WorkItemIndex = testcase.InitialWorkItemIndex
	vm.WorkPackage = testcase.InitialWorkPackage
	vm.Authorization = testcase.InitialAuthorizerOutput
	if vm.Extrinsics == nil {
		vm.Extrinsics = make([][]byte, 0)
	}
	if len(vm.Extrinsics) == 0 {
		vm.Extrinsics = append(vm.Extrinsics, []byte{})
	}
	vm.Extrinsics[0] = testcase.InitialExtrinsic

	// Initialize Import Segments
	vm.Imports = make([][][]byte, len(testcase.InitialImportSegment))
	for i, bs := range testcase.InitialImportSegment {
		vm.Imports[i] = make([][]byte, len(bs))
		// TODO:copy(vm.Imports[i][:], bs[:])
	}

	vm.ExportSegmentIndex = testcase.InitialExportSegmentIdx
	vm.Service_index = testcase.InitialServiceIndex
	vm.Delta = make(map[uint32]*types.ServiceAccount)
	for service_idx, v := range testcase.InitialDelta {
		sa, err := ConvertToServiceAccount(v, service_idx)
		if err != nil {
			fmt.Printf("Error converting ServiceAccount: %v\n", err)
			return
		}
		vm.Delta[service_idx] = sa
	}
	vm.Timeslot = testcase.InitialTimeslot
}

func InitPvmAccumulate(vm *VM, testcase AccumulateTestcase) {
	// Convert initial XContext_x
	var XContext *types.XContext
	if testcase.InitialXcontent_x != nil {
		var err error
		XContext, err = ConvertToXContext(testcase.InitialXcontent_x)
		if err != nil {
			fmt.Printf("Error converting XContext_x: %v\n", err)
			return
		}
	}

	// Initialize vm.XContext and vm.XContextY
	service_account, _ := XContext.GetX_s()
	if service_account != nil {
		service_account.ALLOW_MUTABLE()
	}
	vm.X = XContext
	vm.Timeslot = testcase.InitialTimeslot
}

func InitPvmGeneral(vm *VM, testcase GeneralTestcase) {
	// Initialize ServiceAccount
	sa, err := ConvertToServiceAccount(testcase.InitialServiceAccount, testcase.InitialServiceIndex)
	if err != nil {
		fmt.Printf("Error converting ServiceAccount: %v\n", err)
		return
	}
	vm.ServiceAccount = sa
	// Initialize Delta map
	vm.Delta = make(map[uint32]*types.ServiceAccount)
	vm.X = &types.XContext{}
	vm.X.U = &types.PartialState{}
	vm.X.U.D = make(map[uint32]*types.ServiceAccount)

	for service_idx, v := range testcase.InitialDelta {
		sa, err := ConvertToServiceAccount(v, service_idx)
		if err != nil {
			fmt.Printf("Error converting ServiceAccount: %v\n", err)
			return
		}
		vm.Delta[service_idx] = sa
		vm.X.U.D[service_idx] = sa
	}
	vm.Service_index = testcase.InitialServiceIndex
}

// Compare functions
func CompareBase(vm *VM, testcase Testcase) {
	passed := true
	// Compare Gas
	expectedGas := int64(testcase.GetExpectedGas())
	if expectedGas != 0 && vm.Gas != expectedGas {
		fmt.Printf("Gas mismatch. Expected: %d, Got: %d\n", expectedGas, vm.Gas)
		passed = false
	}
	// Compare ResultCode
	expectedResultCode := testcase.GetExpectedResultCode()
	if expectedResultCode != 0 && vm.ResultCode != expectedResultCode {
		fmt.Printf("ResultCode mismatch. Expected: %d, Got: %d\n", expectedResultCode, vm.ResultCode)
		passed = false
	}

	// Compare Registers
	expectedRegs := testcase.GetExpectedRegs()
	if len(expectedRegs) > 0 {
		vmRegs := vm.ReadRegisters()
		for i, reg := range expectedRegs {
			if vmRegs[i] != reg {
				fmt.Printf("Register[%d] mismatch. Expected: %d, Got: %d\n", i, reg, vmRegs[i])
				passed = false
			}
		}
	}
	// Compare Memory
	expectedMemory := testcase.GetExpectedMemory().Pages
	if len(expectedMemory) > 0 {
		for page_addr, page := range expectedMemory {
			vm.Ram.SetPageAccess(page_addr, 1, AccessMode{Readable: true})
			actualMemory, _ := vm.Ram.ReadRAMBytes(page_addr*PageSize, uint32(len(page.Value)))
			if !equalByteSlices(actualMemory, page.Value) {
				fmt.Printf("Memory mismatch at address %d. Expected: %v, Got: %v\n", page_addr*PageSize, page.Value, actualMemory)
				passed = false
			}
		}
	}
	if passed {
		fmt.Printf("Case %s base pass\n", testcase.GetName())
	} else {
		fmt.Printf("Case %s base fail\n", testcase.GetName())
	}
}

func CompareRefine(vm *VM, testcase RefineTestcase) {
	passed := true
	RefineM_map_test := ConvertToRefineM_map(testcase.ExpectedRefineM_map)
	// Compare RefineM_map
	if len(testcase.ExpectedRefineM_map) > 0 {
		if !reflect.DeepEqual(vm.RefineM_map, RefineM_map_test) {
			fmt.Printf("RefineMAP mismatch. Expected: %+v, Got: %+v\n", testcase.ExpectedRefineM_map, vm.RefineM_map)

			jsonData, err := json.Marshal(testcase.ExpectedRefineM_map)
			if err != nil {
				fmt.Println("Error formatting JSON:", err)
				return
			}
			fmt.Printf("ActualXContext_x: %s\n", string(jsonData))

			fmt.Println("=====================================================")

			jsonData, err = json.Marshal(vm.RefineM_map)
			if err != nil {
				fmt.Println("Error formatting JSON:", err)
				return
			}
			fmt.Printf("ExpectedXcontent_x: %s\n", string(jsonData))

			passed = false
		}
	}
	// Compare Export Segment
	if len(testcase.ExpectedExportSegment) > 0 {
		if !equal2DByteSlices(vm.Exports, testcase.ExpectedExportSegment) {
			fmt.Printf("Export segment mismatch. Expected: %v, Got: %v\n", testcase.ExpectedExportSegment, vm.Exports)
			passed = false
		}
	}
	if passed {
		fmt.Printf("Case %s refine pass\n", testcase.Name)
	} else {
		fmt.Printf("Case %s refine fail\n", testcase.Name)
	}
}

func CompareAccumulate(vm *VM, testcase AccumulateTestcase) {
	// Convert vm.XContext back to XContextForTest
	passed := true
	actualXContext_x, err := ConvertToXContextForTest(vm.X)
	if err != nil {
		fmt.Printf("Error converting actual XContext_x: %v\n", err)
		passed = false
		return
	}

	if !reflect.DeepEqual(actualXContext_x, testcase.ExpectedXcontent_x) {
		fmt.Printf("XContext_x mismatch. Expected: %v, Got: %v\n", testcase.ExpectedXcontent_x, actualXContext_x)
		passed = false
	}

	// 	passed = false
	// }
	// Similarly for XContextY
	// actualXContext_y, err := ConvertToXContextForTest(&vm.Y)
	// if err != nil {
	// 	fmt.Printf("Error converting actual XContext_y: %v\n", err)
	// 	return
	// }
	// if !reflect.DeepEqual(actualXContext_y, testcase.ExpectedXcontent_y) {
	// 	fmt.Printf("XContext_y mismatch. Expected: %+v, Got: %+v\n", testcase.ExpectedXcontent_y, actualXContext_y)
	//	passed = false
	// }
	if passed {
		fmt.Printf("Case %s accumulate pass\n", testcase.Name)
	} else {
		fmt.Printf("Case %s accumulate fail\n", testcase.Name)
	}
}

func CompareGeneral(vm *VM, testcase GeneralTestcase) {
	passed := true
	// Compare ServiceAccount
	actualServiceAccount := ConvertToServiceAccountForTest(vm.ServiceAccount)

	if !reflect.DeepEqual(actualServiceAccount, testcase.ExpectedServiceAccount) {
		fmt.Printf("ServiceAccount mismatch.\n Got: %+v, \n Expected: %+v\n", actualServiceAccount, testcase.ExpectedServiceAccount)
		passed = false
	}
	// Compare Delta map (if needed)
	for k, v := range vm.Delta {
		actualServiceAccount := ConvertToServiceAccountForTest(v)

		if !reflect.DeepEqual(actualServiceAccount, testcase.ExpectedDelta[k]) {
			fmt.Printf("Delta[%d] mismatch.\n Got: %+v, \n Expected: %+v\n", k, actualServiceAccount, testcase.ExpectedDelta[k])
			passed = false
		}
	}
	if passed {
		fmt.Printf("Case %s general pass\n", testcase.GetName())
	} else {
		fmt.Printf("Case %s general fail\n", testcase.GetName())
	}
}

// Main test functions
// Test all refine test vectors
func TestRefine(t *testing.T) {
	// node, err := SetUpNode()
	// if err != nil {
	// 	panic("Error setting up nodes: %v\n")
	// }

	functions := []string{
		"Historical_lookup", // William
		"Fetch",             // William
		"Export",            // William
		"Machine",           // Shawn
		"Peek",              // Shawn
		"Poke",              // Shawn
		"Zero",              // Shawn
		"Void",              // Shawn
		"Invoke",            // Shawn
		"Expunge",           // Shawn
	}
	for _, name := range functions {
		hostidx, _ := GetHostFunctionDetails(name)
		dirPath := "../jamtestvectors/host_function"

		files, contents, err := FindAndReadJSONFiles(dirPath, name)
		if err != nil {
			fmt.Println("Error:", err)
			return
		}
		for i, content := range contents {
			// targetStateDB := node.getPVMStateDB()
			hostENV := NewMockHostEnv()
			vm := NewVMFortest(hostENV)

			// if !strings.Contains(files[i], "OK") {
			// 	continue
			// }

			fmt.Println("--------------------------------------------------")
			fmt.Printf("Testing file %s\n", files[i])
			var testcase RefineTestcase
			err := json.Unmarshal([]byte(content), &testcase)
			if err != nil {
				fmt.Printf("Failed to parse JSON for file %s: %v\n", files[i], err)
				continue
			}
			InitPvmBase(vm, testcase)
			InitPvmRefine(vm, testcase)
			vm.InvokeHostCall(hostidx)
			CompareBase(vm, testcase)
			CompareRefine(vm, testcase)

		}
	}
}

// Test all accumulate test vectors
func TestAccumulate(t *testing.T) {
	// node, err := SetUpNode()
	// if err != nil {
	// 	panic("Error setting up nodes: %v\n")
	// }

	functions := []string{
		// "Bless", // Michael+Sourabh
		// "Assign", // Michael+Sourabh
		// "Designate", // Michael+Sourabh
		// "Checkpoint", // Michael+Sourabh
		"New",      // William
		"Upgrade",  // William
		"Transfer", // William
		"Eject",
		"Query",
		"Solicit", // William
		"Forget",  // William
		"Yield",
	}

	for _, name := range functions {
		hostidx, _ := GetHostFunctionDetails(name)
		dirPath := "../jamtestvectors/host_function"

		files, contents, err := FindAndReadJSONFiles(dirPath, name)
		if err != nil {
			fmt.Println("Error:", err)
			return
		}
		for i, content := range contents {
			// targetStateDB := node.getPVMStateDB()

			hostENV := NewMockHostEnv()
			vm := NewVMFortest(hostENV)

			fmt.Println("--------------------------------------------------")
			fmt.Printf("Testing file %s\n", files[i])
			var testcase AccumulateTestcase
			err := json.Unmarshal([]byte(content), &testcase)
			if err != nil {
				fmt.Printf("Failed to parse JSON for file %s: %v\n", files[i], err)
				continue
			}
			InitPvmBase(vm, testcase)
			InitPvmAccumulate(vm, testcase)
			vm.InvokeHostCall(hostidx)
			CompareBase(vm, testcase)
			CompareAccumulate(vm, testcase)
		}
	}
}

// Test all general test vectors
func TestGeneral(t *testing.T) {
	// node, err := SetUpNode()
	// if err != nil {
	// 	panic("Error setting up nodes: %v\n")
	// }

	functions := []string{
		// "Gas", // Michael
		"Lookup", // William
		"Read",   // William
		"Write",  // William
		"Info",   // Shawn
		// "Sp1Groth16Verify", // Sourabh
	}
	for _, name := range functions {
		hostidx, _ := GetHostFunctionDetails(name)
		dirPath := "../jamtestvectors/host_function"

		files, contents, err := FindAndReadJSONFiles(dirPath, name)
		if err != nil {
			fmt.Println("Error:", err)
			return
		}
		for i, content := range contents {
			// targetStateDB := node.getPVMStateDB()
			hostENV := NewMockHostEnv()
			vm := NewVMFortest(hostENV)

			fmt.Println("--------------------------------------------------")
			fmt.Printf("Testing file %s\n", files[i])
			var testcase GeneralTestcase
			err := json.Unmarshal([]byte(content), &testcase)
			if err != nil {
				fmt.Printf("Failed to parse JSON for file %s: %v\n", files[i], err)
				continue
			}
			InitPvmBase(vm, testcase)
			InitPvmGeneral(vm, testcase)
			vm.InvokeHostCall(hostidx)
			CompareBase(vm, testcase)
			CompareGeneral(vm, testcase)
		}
	}
}

func ReadJSONFile(filePath string, target interface{}) error {
	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %v", err)
	}

	err = json.Unmarshal(fileContent, target)
	if err != nil {
		return fmt.Errorf("failed to unmarshal JSON: %v", err)
	}
	return nil
}

func WriteJSONFile(filePath string, data interface{}) error {
	content, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %v", err)
	}

	err = os.WriteFile(filePath, content, 0644)
	if err != nil {
		return fmt.Errorf("failed to write file: %v", err)
	}
	return nil
}

func equal2DByteSlices(a [][]byte, b []ByteSlice) bool {
	// Check for nil or different lengths
	if a == nil && b == nil {
		return true // Both are nil, considered equal
	}
	if a == nil || b == nil {
		return false // One is nil, the other is not
	}
	if len(a) != len(b) {
		return false // Different number of inner slices
	}

	// Compare each inner slice
	for i := range a {
		if !equalByteSlices(a[i], b[i]) {
			return false // Found a mismatch
		}
	}

	return true
}

func equalByteSlices(a, b []byte) bool {
	// Treat nil and empty slices as equal
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Convert XContextForTest to types.XContext
func ConvertToXContext(xcft *XContextForTest) (*types.XContext, error) {
	xc := &types.XContext{
		I: xcft.I,
		S: xcft.S,
		T: []types.DeferredTransfer{},
		U: nil,
	}

	if xcft.Y != "" {
		hashValue := common.HexToHash(xcft.Y)
		xc.Y = hashValue
	}

	for _, dt := range xcft.T {
		var memo [128]byte
		copy(memo[:], dt.Memo)
		xc.T = append(xc.T, types.DeferredTransfer{
			SenderIndex:   dt.SenderIndex,
			ReceiverIndex: dt.ReceiverIndex,
			Amount:        dt.Amount,
			Memo:          memo,
			GasLimit:      dt.GasLimit,
		})
	}

	// Convert U if not nil
	if xcft.U != nil {
		u, err := ConvertToPartialState(xcft.U)
		if err != nil {
			return nil, err
		}
		xc.U = u
	}

	return xc, nil
}

// Convert types.XContext to XContextForTest
func ConvertToXContextForTest(xc *types.XContext) (*XContextForTest, error) {
	xcft := &XContextForTest{
		I: xc.I,
		S: xc.S,
		T: []DeferredTransferForTest{},
		U: nil,
	}
	empty_hash := common.Hash{}
	if xc.Y != empty_hash {
		xcft.Y = xc.Y.String()
	}

	for _, dt := range xc.T {
		xcft.T = append(xcft.T, DeferredTransferForTest{
			SenderIndex:   dt.SenderIndex,
			ReceiverIndex: dt.ReceiverIndex,
			Amount:        dt.Amount,
			Memo:          dt.Memo[:],
			GasLimit:      dt.GasLimit,
		})
	}

	// Convert U if not nil
	if xc.U != nil {
		uft := ConvertToPartialStateForTest(xc.U)
		xcft.U = uft
	}

	return xcft, nil
}

// Convert PartialStateForTest to types.PartialState
func ConvertToPartialState(psft *PartialStateForTest) (*types.PartialState, error) {
	ps := &types.PartialState{
		D:                  make(map[uint32]*types.ServiceAccount),
		UpcomingValidators: psft.I,
		QueueWorkReport:    psft.Q,
		PrivilegedState:    psft.X,
	}

	// Convert D map
	for service_idx, v := range psft.D {
		sa, err := ConvertToServiceAccount(v, service_idx)
		if err != nil {
			return nil, err
		}
		ps.D[service_idx] = sa
	}

	return ps, nil
}

// Convert types.PartialState to PartialStateForTest
func ConvertToPartialStateForTest(ps *types.PartialState) *PartialStateForTest {
	psft := &PartialStateForTest{
		D: make(map[uint32]*ServiceAccountForTest),
		I: ps.UpcomingValidators,
		Q: ps.QueueWorkReport,
		X: ps.PrivilegedState,
	}

	// Convert D map
	for service_idx, v := range ps.D {
		saft := ConvertToServiceAccountForTest(v)
		psft.D[service_idx] = saft
	}

	return psft
}

// Convert ServiceAccountForTest to ServiceAccount
func ConvertToServiceAccount(saft *ServiceAccountForTest, serviceIdx uint32) (*types.ServiceAccount, error) {
	sa := &types.ServiceAccount{
		ServiceIndex: serviceIdx,
		CodeHash:     common.HexToHash(saft.CodeHash),
		Balance:      saft.Balance,
		GasLimitG:    saft.GasLimitG,
		GasLimitM:    saft.GasLimitM,
		Dirty:        false,
		Storage:      make(map[common.Hash]types.StorageObject),
		Lookup:       make(map[common.Hash]types.LookupObject),
		Preimage:     make(map[common.Hash]types.PreimageObject),
		Mutable:      true,
	}

	// Convert Storage map
	for k, v := range saft.Storage {
		keyByte := common.Hex2Hash(k)
		sa.Storage[keyByte] = types.StorageObject{Value: v}
	}

	// Convert Lookup map
	for k, v := range saft.Lookup {
		keyHash := common.HexToHash(k)
		sa.Lookup[keyHash] = types.LookupObject{T: v.T, Z: v.L}
	}

	// Convert Preimage map
	for k, v := range saft.Preimage {
		keyHash := common.HexToHash(k)
		sa.Preimage[keyHash] = types.PreimageObject{Preimage: v}
	}

	return sa, nil
}

// Convert ServiceAccount to ServiceAccountForTest
func ConvertToServiceAccountForTest(sa *types.ServiceAccount) *ServiceAccountForTest {
	saft := &ServiceAccountForTest{
		CodeHash:  sa.CodeHash.Hex(),
		Balance:   sa.Balance,
		GasLimitG: sa.GasLimitG,
		GasLimitM: sa.GasLimitM,
		Storage:   make(map[string]ByteSlice),
		Lookup:    make(map[string]LookupObjectForTest),
		Preimage:  make(map[string]ByteSlice),
	}
	if saft.Balance == 0 && saft.CodeHash == "0x0000000000000000000000000000000000000000000000000000000000000000" { // Empty account
		saft.CodeHash = ""
	}

	// Convert Storage map
	for k, v := range sa.Storage {
		// common.Hash to string
		if v.Deleted != true {
			keyStr := k.Hex()
			saft.Storage[keyStr] = v.Value
		}
	}

	// Convert Lookup map
	for k, v := range sa.Lookup {
		if !v.Deleted {
			keyStr := k.Hex()
			saft.Lookup[keyStr] = LookupObjectForTest{
				T: v.T,
				L: v.Z,
			}
		}
	}

	// Convert Preimage map
	for k, v := range sa.Preimage {
		if !v.Deleted {
			keyStr := k.Hex()
			saft.Preimage[keyStr] = v.Preimage
		}
	}

	return saft
}

func ConvertToRefineM_map(refineM_mapFT map[uint32]*RefineMForTest) map[uint32]*RefineM {
	if refineM_mapFT == nil {
		return make(map[uint32]*RefineM)
	}
	refineM_map := make(map[uint32]*RefineM)
	for k, v := range refineM_mapFT {
		if v == nil {
			continue
		}
		ram := NewRAM()

		for page_addr, page := range v.U.Pages {
			ram.SetPageAccess(page_addr, 1, page.Access)
			ram.WriteRAMBytes(page_addr*PageSize, page.Value)
		}

		refineM_map[k] = &RefineM{
			P: v.P,
			U: ram,
			I: v.I,
		}
	}
	return refineM_map
}

func ConvertToRefineM_mapForTest(refineM_map map[uint32]*RefineM) map[uint32]*RefineMForTest {
	if refineM_map == nil {
		return make(map[uint32]*RefineMForTest)
	}

	refineM_mapFT := make(map[uint32]*RefineMForTest)
	for k, v := range refineM_map {

		RAMForTest := &RAMForTest{
			Pages: make(map[uint32]*PageForTest),
		}

		for page_addr, page := range v.U.Pages {
			RAMForTest.Pages[page_addr] = &PageForTest{
				Value:  page.Value,
				Access: page.Access,
			}
		}
		refineM_mapFT[k] = &RefineMForTest{
			P: v.P,
			U: RAMForTest,
			I: v.I,
		}
	}
	return refineM_mapFT
}

// Generate General test vectors
func TestGenerateGeneralTestVectors(t *testing.T) {
	dirPath := "../jamtestvectors/host_function"

	testCases := []struct {
		filename               string
		initialGas             uint64
		expectedGas            uint64
		initialRegs            map[uint32]uint64
		expectedRegs           map[uint32]uint64
		initialMemory          RAMForTest
		expectedMemory         RAMForTest
		initialServiceAccount  *ServiceAccountForTest
		expectedServiceAccount *ServiceAccountForTest
		initialDelta           map[uint32]*ServiceAccountForTest
		expectedDelta          map[uint32]*ServiceAccountForTest
		initialServiceIndex    uint32
	}{
		{
			filename:    "./Lookup/hostLookupOK_s.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7:  0,
				8:  32 * PageSize,
				9:  32 * PageSize,
				10: 100,
			},
			expectedRegs: map[uint32]uint64{
				7:  12, // ength of the preimage
				8:  32 * PageSize,
				9:  32 * PageSize,
				10: 100,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Writable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlob, Access: AccessMode{Writable: true}},
				},
			},
			initialServiceAccount: &ServiceAccountForTest{
				Storage: map[string]ByteSlice{
					Service_0_StorageKeyForHost.String(): StorageValue,
				},
				Lookup: map[string]LookupObjectForTest{
					PreimageBlobHash.String(): {
						T: LookupValue,
						L: PreimageBlobLen,
					},
				},
				Preimage: map[string]ByteSlice{
					Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
				},
				CodeHash:  PreimageBlobHash.String(),
				Balance:   Service_0_Balance,
				GasLimitG: Service_0_GasLimitG,
				GasLimitM: Service_0_GasLimitM,
			},
			expectedServiceAccount: &ServiceAccountForTest{
				Storage: map[string]ByteSlice{
					Service_0_StorageKeyForHost.String(): StorageValue,
				},
				Lookup: map[string]LookupObjectForTest{
					PreimageBlobHash.String(): {
						T: LookupValue,
						L: PreimageBlobLen,
					},
				},
				Preimage: map[string]ByteSlice{
					Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
				},
				CodeHash:  PreimageBlobHash.String(),
				Balance:   Service_0_Balance,
				GasLimitG: Service_0_GasLimitG,
				GasLimitM: Service_0_GasLimitM,
			},
			initialDelta:        map[uint32]*ServiceAccountForTest{},
			expectedDelta:       map[uint32]*ServiceAccountForTest{},
			initialServiceIndex: 0,
		},
		{
			filename:    "./Lookup/hostLookupOK_d_omega7.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7:  0,
				8:  32 * PageSize,
				9:  32 * PageSize,
				10: 100,
			},
			expectedRegs: map[uint32]uint64{
				7:  12, // ength of the preimage
				8:  32 * PageSize,
				9:  32 * PageSize,
				10: 100,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Writable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlob, Access: AccessMode{Writable: true}},
				},
			},
			initialServiceAccount: &ServiceAccountForTest{
				Storage:  map[string]ByteSlice{},
				Lookup:   map[string]LookupObjectForTest{},
				Preimage: map[string]ByteSlice{},
			},
			expectedServiceAccount: &ServiceAccountForTest{
				Storage:  map[string]ByteSlice{},
				Lookup:   map[string]LookupObjectForTest{},
				Preimage: map[string]ByteSlice{},
			},
			initialDelta: map[uint32]*ServiceAccountForTest{
				0: {
					Storage: map[string]ByteSlice{
						Service_0_StorageKeyForHost.String(): StorageValue,
					},
					Lookup: map[string]LookupObjectForTest{
						PreimageBlobHash.String(): {
							T: LookupValue,
							L: PreimageBlobLen,
						},
					},
					Preimage: map[string]ByteSlice{
						Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
					},
					CodeHash:  PreimageBlobHash.String(),
					Balance:   Service_0_Balance,
					GasLimitG: Service_0_GasLimitG,
					GasLimitM: Service_0_GasLimitM,
				},
			},
			expectedDelta: map[uint32]*ServiceAccountForTest{
				0: {
					Storage: map[string]ByteSlice{
						Service_0_StorageKeyForHost.String(): StorageValue,
					},
					Lookup: map[string]LookupObjectForTest{
						PreimageBlobHash.String(): {
							T: LookupValue,
							L: PreimageBlobLen,
						},
					},
					Preimage: map[string]ByteSlice{
						Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
					},
					CodeHash:  PreimageBlobHash.String(),
					Balance:   Service_0_Balance,
					GasLimitG: Service_0_GasLimitG,
					GasLimitM: Service_0_GasLimitM,
				},
			},
			initialServiceIndex: 999, // read from register 7
		},
		{
			filename:    "./Lookup/hostLookupOOB.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7:  0,
				8:  32 * PageSize,
				9:  32 * PageSize,
				10: 100,
			},
			expectedRegs: map[uint32]uint64{
				7:  OOB,
				8:  32 * PageSize,
				9:  32 * PageSize,
				10: 100,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Inaccessible: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Inaccessible: true}},
				},
			},
			initialServiceAccount: &ServiceAccountForTest{
				Storage:  map[string]ByteSlice{},
				Lookup:   map[string]LookupObjectForTest{},
				Preimage: map[string]ByteSlice{},
			},
			expectedServiceAccount: &ServiceAccountForTest{
				Storage:  map[string]ByteSlice{},
				Lookup:   map[string]LookupObjectForTest{},
				Preimage: map[string]ByteSlice{},
			},
			initialDelta: map[uint32]*ServiceAccountForTest{
				0: {
					Storage: map[string]ByteSlice{
						Service_0_StorageKeyForHost.String(): StorageValue,
					},
					Lookup: map[string]LookupObjectForTest{
						PreimageBlobHash.String(): {
							T: LookupValue,
							L: PreimageBlobLen,
						},
					},
					Preimage: map[string]ByteSlice{
						Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
					},
					CodeHash:  PreimageBlobHash.String(),
					Balance:   Service_0_Balance,
					GasLimitG: Service_0_GasLimitG,
					GasLimitM: Service_0_GasLimitM,
				},
			},
			expectedDelta: map[uint32]*ServiceAccountForTest{
				0: {
					Storage: map[string]ByteSlice{
						Service_0_StorageKeyForHost.String(): StorageValue,
					},
					Lookup: map[string]LookupObjectForTest{
						PreimageBlobHash.String(): {
							T: LookupValue,
							L: PreimageBlobLen,
						},
					},
					Preimage: map[string]ByteSlice{
						Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
					},
					CodeHash:  PreimageBlobHash.String(),
					Balance:   Service_0_Balance,
					GasLimitG: Service_0_GasLimitG,
					GasLimitM: Service_0_GasLimitM,
				},
			},
			initialServiceIndex: 1,
		},
		{
			filename:    "./Lookup/hostLookupNONE.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7:  0,
				8:  32 * PageSize,
				9:  32 * PageSize,
				10: 100,
			},
			expectedRegs: map[uint32]uint64{
				7:  NONE,
				8:  32 * PageSize,
				9:  32 * PageSize,
				10: 100,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: make(ByteSlice, 32), Access: AccessMode{Writable: true}}, // Invalid key lead to NONE error
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: make(ByteSlice, 32), Access: AccessMode{Writable: true}}, // Invalid key lead to NONE error
				},
			},
			initialServiceAccount: &ServiceAccountForTest{
				Storage:  map[string]ByteSlice{},
				Lookup:   map[string]LookupObjectForTest{},
				Preimage: map[string]ByteSlice{},
			},
			expectedServiceAccount: &ServiceAccountForTest{
				Storage:  map[string]ByteSlice{},
				Lookup:   map[string]LookupObjectForTest{},
				Preimage: map[string]ByteSlice{},
			},
			initialDelta: map[uint32]*ServiceAccountForTest{
				0: {
					Storage: map[string]ByteSlice{
						Service_0_StorageKeyForHost.String(): StorageValue,
					},
					Lookup: map[string]LookupObjectForTest{
						PreimageBlobHash.String(): {
							T: LookupValue,
							L: PreimageBlobLen,
						},
					},
					Preimage: map[string]ByteSlice{
						Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
					},
					CodeHash:  PreimageBlobHash.String(),
					Balance:   Service_0_Balance,
					GasLimitG: Service_0_GasLimitG,
					GasLimitM: Service_0_GasLimitM,
				},
			},
			expectedDelta: map[uint32]*ServiceAccountForTest{
				0: {
					Storage: map[string]ByteSlice{
						Service_0_StorageKeyForHost.String(): StorageValue,
					},
					Lookup: map[string]LookupObjectForTest{
						PreimageBlobHash.String(): {
							T: LookupValue,
							L: PreimageBlobLen,
						},
					},
					Preimage: map[string]ByteSlice{
						Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
					},
					CodeHash:  PreimageBlobHash.String(),
					Balance:   Service_0_Balance,
					GasLimitG: Service_0_GasLimitG,
					GasLimitM: Service_0_GasLimitM,
				},
			},
			initialServiceIndex: 1,
		},
		{
			filename:    "./Read/hostReadOK_s.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7:  0,
				8:  32 * PageSize,
				9:  uint64(StorageKeyLen),
				10: 32 * PageSize,
				11: 100,
			},
			expectedRegs: map[uint32]uint64{
				7:  uint64(StorageValueLen), // Length of the stoage
				8:  32 * PageSize,
				9:  uint64(StorageKeyLen),
				10: 32 * PageSize,
				11: 100,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: StorageKey, Access: AccessMode{Writable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: StorageKey, Access: AccessMode{Writable: true}},
				},
			},
			initialServiceAccount: &ServiceAccountForTest{
				Storage: map[string]ByteSlice{
					Service_0_StorageKeyForHost.String(): StorageValue,
				},
				Lookup: map[string]LookupObjectForTest{
					PreimageBlobHash.String(): {
						T: LookupValue,
						L: PreimageBlobLen,
					},
				},
				Preimage: map[string]ByteSlice{
					Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
				},
				CodeHash:  PreimageBlobHash.String(),
				Balance:   Service_0_Balance,
				GasLimitG: Service_0_GasLimitG,
				GasLimitM: Service_0_GasLimitM,
			},
			expectedServiceAccount: &ServiceAccountForTest{
				Storage: map[string]ByteSlice{
					Service_0_StorageKeyForHost.String(): StorageValue,
				},
				Lookup: map[string]LookupObjectForTest{
					PreimageBlobHash.String(): {
						T: LookupValue,
						L: PreimageBlobLen,
					},
				},
				Preimage: map[string]ByteSlice{
					Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
				},
				CodeHash:  PreimageBlobHash.String(),
				Balance:   Service_0_Balance,
				GasLimitG: Service_0_GasLimitG,
				GasLimitM: Service_0_GasLimitM,
			},
			initialDelta:        map[uint32]*ServiceAccountForTest{},
			expectedDelta:       map[uint32]*ServiceAccountForTest{},
			initialServiceIndex: 0,
		},
		{
			filename:    "./Read/hostReadOK_d_omega7.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7:  0,
				8:  32 * PageSize,
				9:  uint64(StorageKeyLen),
				10: 32 * PageSize,
				11: 100,
			},
			expectedRegs: map[uint32]uint64{
				7:  uint64(StorageValueLen), // Length of the stoage
				8:  32 * PageSize,
				9:  uint64(StorageKeyLen),
				10: 32 * PageSize,
				11: 100,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: StorageKey, Access: AccessMode{Writable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: StorageKey, Access: AccessMode{Writable: true}},
				},
			},
			initialServiceAccount: &ServiceAccountForTest{
				Storage:  map[string]ByteSlice{},
				Lookup:   map[string]LookupObjectForTest{},
				Preimage: map[string]ByteSlice{},
			},
			expectedServiceAccount: &ServiceAccountForTest{
				Storage:  map[string]ByteSlice{},
				Lookup:   map[string]LookupObjectForTest{},
				Preimage: map[string]ByteSlice{},
			},
			initialDelta: map[uint32]*ServiceAccountForTest{
				0: {
					Storage: map[string]ByteSlice{
						Service_0_StorageKeyForHost.String(): StorageValue,
					},
					Lookup: map[string]LookupObjectForTest{
						PreimageBlobHash.String(): {
							T: LookupValue,
							L: PreimageBlobLen,
						},
					},
					Preimage: map[string]ByteSlice{
						Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
					},
					CodeHash:  PreimageBlobHash.String(),
					Balance:   Service_0_Balance,
					GasLimitG: Service_0_GasLimitG,
					GasLimitM: Service_0_GasLimitM,
				},
			},
			expectedDelta: map[uint32]*ServiceAccountForTest{
				0: {
					Storage: map[string]ByteSlice{
						Service_0_StorageKeyForHost.String(): StorageValue,
					},
					Lookup: map[string]LookupObjectForTest{
						PreimageBlobHash.String(): {
							T: LookupValue,
							L: PreimageBlobLen,
						},
					},
					Preimage: map[string]ByteSlice{
						Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
					},
					CodeHash:  PreimageBlobHash.String(),
					Balance:   Service_0_Balance,
					GasLimitG: Service_0_GasLimitG,
					GasLimitM: Service_0_GasLimitM,
				},
			},
			initialServiceIndex: 999, // read from delta
		},
		{
			filename:    "./Read/hostReadOOB.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7:  0,
				8:  32 * PageSize,
				9:  uint64(StorageKeyLen),
				10: 32 * PageSize,
				11: 100,
			},
			expectedRegs: map[uint32]uint64{
				7:  OOB,
				8:  32 * PageSize,
				9:  uint64(StorageKeyLen),
				10: 32 * PageSize,
				11: 100,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: StorageKey, Access: AccessMode{Inaccessible: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: StorageKey, Access: AccessMode{Inaccessible: true}},
				},
			},
			initialServiceAccount: &ServiceAccountForTest{
				Storage: map[string]ByteSlice{
					Service_0_StorageKeyForHost.String(): StorageValue,
				},
				Lookup: map[string]LookupObjectForTest{
					PreimageBlobHash.String(): {
						T: LookupValue,
						L: PreimageBlobLen,
					},
				},
				Preimage: map[string]ByteSlice{
					Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
				},
				CodeHash:  PreimageBlobHash.String(),
				Balance:   Service_0_Balance,
				GasLimitG: Service_0_GasLimitG,
				GasLimitM: Service_0_GasLimitM,
			},
			expectedServiceAccount: &ServiceAccountForTest{
				Storage: map[string]ByteSlice{
					Service_0_StorageKeyForHost.String(): StorageValue,
				},
				Lookup: map[string]LookupObjectForTest{
					PreimageBlobHash.String(): {
						T: LookupValue,
						L: PreimageBlobLen,
					},
				},
				Preimage: map[string]ByteSlice{
					Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
				},
				CodeHash:  PreimageBlobHash.String(),
				Balance:   Service_0_Balance,
				GasLimitG: Service_0_GasLimitG,
				GasLimitM: Service_0_GasLimitM,
			},
			initialDelta:        map[uint32]*ServiceAccountForTest{},
			expectedDelta:       map[uint32]*ServiceAccountForTest{},
			initialServiceIndex: 0,
		},
		{
			filename:    "./Read/hostReadNONE.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7:  0,
				8:  32 * PageSize,
				9:  uint64(StorageKeyLen),
				10: 32 * PageSize,
				11: 100,
			},
			expectedRegs: map[uint32]uint64{
				7:  NONE,
				8:  32 * PageSize,
				9:  uint64(StorageKeyLen),
				10: 32 * PageSize,
				11: 100,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: ByteSlice{1, 1, 1, 1}, Access: AccessMode{Writable: true}}, // Invalid key lead to NONE error
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: ByteSlice{1, 1, 1, 1}, Access: AccessMode{Writable: true}},
				},
			},
			initialServiceAccount: &ServiceAccountForTest{
				Storage: map[string]ByteSlice{
					Service_0_StorageKeyForHost.String(): StorageValue,
				},
				Lookup: map[string]LookupObjectForTest{
					PreimageBlobHash.String(): {
						T: LookupValue,
						L: PreimageBlobLen,
					},
				},
				Preimage: map[string]ByteSlice{
					Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
				},
				CodeHash:  PreimageBlobHash.String(),
				Balance:   Service_0_Balance,
				GasLimitG: Service_0_GasLimitG,
				GasLimitM: Service_0_GasLimitM,
			},
			expectedServiceAccount: &ServiceAccountForTest{
				Storage: map[string]ByteSlice{
					Service_0_StorageKeyForHost.String(): StorageValue,
				},
				Lookup: map[string]LookupObjectForTest{
					PreimageBlobHash.String(): {
						T: LookupValue,
						L: PreimageBlobLen,
					},
				},
				Preimage: map[string]ByteSlice{
					Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
				},
				CodeHash:  PreimageBlobHash.String(),
				Balance:   Service_0_Balance,
				GasLimitG: Service_0_GasLimitG,
				GasLimitM: Service_0_GasLimitM,
			},
			initialDelta:        map[uint32]*ServiceAccountForTest{},
			expectedDelta:       map[uint32]*ServiceAccountForTest{},
			initialServiceIndex: 0,
		},
		{
			filename:    "./Write/hostWriteOK.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7:  32 * PageSize,
				8:  uint64(StorageKeyLen),
				9:  33 * PageSize,
				10: 4,
			},
			expectedRegs: map[uint32]uint64{
				7:  uint64(StorageValueLen), // length of the pervious storage value
				8:  uint64(StorageKeyLen),
				9:  33 * PageSize,
				10: 4,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: StorageKey, Access: AccessMode{Writable: true}},
					33: {Value: ByteSlice{1, 2, 3, 4}, Access: AccessMode{Writable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: StorageKey, Access: AccessMode{Writable: true}},
					33: {Value: ByteSlice{1, 2, 3, 4}, Access: AccessMode{Writable: true}},
				},
			},
			initialServiceAccount: &ServiceAccountForTest{
				Storage: map[string]ByteSlice{
					Service_0_StorageKeyForHost.String(): StorageValue,
				},
				Lookup: map[string]LookupObjectForTest{
					PreimageBlobHash.String(): LookupObjectForTest{
						T: LookupValue,
						L: PreimageBlobLen,
					},
				},
				Preimage: map[string]ByteSlice{
					Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
				},
				CodeHash:  PreimageBlobHash.String(),
				Balance:   Service_0_Balance,
				GasLimitG: Service_0_GasLimitG,
				GasLimitM: Service_0_GasLimitM,
			},
			expectedServiceAccount: &ServiceAccountForTest{
				Storage: map[string]ByteSlice{
					Service_0_StorageKeyForHost.String(): ByteSlice{1, 2, 3, 4},
				},
				Lookup: map[string]LookupObjectForTest{
					PreimageBlobHash.String(): {
						T: LookupValue,
						L: PreimageBlobLen,
					},
				},
				Preimage: map[string]ByteSlice{
					Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
				},
				CodeHash:  PreimageBlobHash.String(),
				Balance:   Service_0_Balance,
				GasLimitG: Service_0_GasLimitG,
				GasLimitM: Service_0_GasLimitM,
			},
			initialDelta:        map[uint32]*ServiceAccountForTest{},
			expectedDelta:       map[uint32]*ServiceAccountForTest{},
			initialServiceIndex: 0,
		},
		{
			filename:    "./Write/hostWriteOOB.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7:  32 * PageSize,
				8:  uint64(StorageKeyLen),
				9:  33 * PageSize,
				10: 4,
			},
			expectedRegs: map[uint32]uint64{
				7:  OOB,
				8:  uint64(StorageKeyLen),
				9:  33 * PageSize,
				10: 4,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: StorageKey, Access: AccessMode{Inaccessible: true}},
					33: {Value: ByteSlice{1, 2, 3, 4}, Access: AccessMode{Inaccessible: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: StorageKey, Access: AccessMode{Inaccessible: true}},
					33: {Value: ByteSlice{1, 2, 3, 4}, Access: AccessMode{Inaccessible: true}},
				},
			},
			initialServiceAccount: &ServiceAccountForTest{
				Storage: map[string]ByteSlice{
					Service_0_StorageKeyForHost.String(): StorageValue,
				},
				Lookup: map[string]LookupObjectForTest{
					PreimageBlobHash.String(): LookupObjectForTest{
						T: LookupValue,
						L: PreimageBlobLen,
					},
				},
				Preimage: map[string]ByteSlice{
					Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
				},
				CodeHash:  PreimageBlobHash.String(),
				Balance:   Service_0_Balance,
				GasLimitG: Service_0_GasLimitG,
				GasLimitM: Service_0_GasLimitM,
			},
			expectedServiceAccount: &ServiceAccountForTest{
				Storage: map[string]ByteSlice{
					Service_0_StorageKeyForHost.String(): StorageValue,
				},
				Lookup: map[string]LookupObjectForTest{
					PreimageBlobHash.String(): LookupObjectForTest{
						T: LookupValue,
						L: PreimageBlobLen,
					},
				},
				Preimage: map[string]ByteSlice{
					Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
				},
				CodeHash:  PreimageBlobHash.String(),
				Balance:   Service_0_Balance,
				GasLimitG: Service_0_GasLimitG,
				GasLimitM: Service_0_GasLimitM,
			},
			initialDelta:        map[uint32]*ServiceAccountForTest{},
			expectedDelta:       map[uint32]*ServiceAccountForTest{},
			initialServiceIndex: 0,
		},
		{
			filename:    "./Write/hostWriteFULL.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7:  32 * PageSize,
				8:  uint64(StorageKeyLen),
				9:  33 * PageSize,
				10: 4,
			},
			expectedRegs: map[uint32]uint64{
				7:  FULL,
				8:  uint64(StorageKeyLen),
				9:  33 * PageSize,
				10: 4,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: StorageKey, Access: AccessMode{Writable: true}},
					33: {Value: ByteSlice{1, 2, 3, 4}, Access: AccessMode{Writable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: StorageKey, Access: AccessMode{Writable: true}},
					33: {Value: ByteSlice{1, 2, 3, 4}, Access: AccessMode{Writable: true}},
				},
			},
			initialServiceAccount: &ServiceAccountForTest{
				Storage: map[string]ByteSlice{
					Service_0_StorageKeyForHost.String(): StorageValue,
				},
				Lookup: map[string]LookupObjectForTest{
					PreimageBlobHash.String(): {
						T: LookupValue,
						L: PreimageBlobLen,
					},
				},
				Preimage: map[string]ByteSlice{
					Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
				},
				CodeHash:  PreimageBlobHash.String(),
				Balance:   90, //FULL error
				GasLimitG: Service_0_GasLimitG,
				GasLimitM: Service_0_GasLimitM,
			},
			expectedServiceAccount: &ServiceAccountForTest{
				Storage: map[string]ByteSlice{
					Service_0_StorageKeyForHost.String(): StorageValue,
				},
				Lookup: map[string]LookupObjectForTest{
					PreimageBlobHash.String(): LookupObjectForTest{
						T: LookupValue,
						L: PreimageBlobLen,
					},
				},
				Preimage: map[string]ByteSlice{
					Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
				},
				CodeHash:  PreimageBlobHash.String(),
				Balance:   90, //FULL error
				GasLimitG: Service_0_GasLimitG,
				GasLimitM: Service_0_GasLimitM,
			},
			initialDelta:        map[uint32]*ServiceAccountForTest{},
			expectedDelta:       map[uint32]*ServiceAccountForTest{},
			initialServiceIndex: 0,
		},
		{
			filename:    "./Write/hostWriteNONE.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7:  32 * PageSize,
				8:  uint64(StorageKeyLen),
				9:  33 * PageSize,
				10: 0, // delete the storage cause NONE
			},
			expectedRegs: map[uint32]uint64{
				7:  NONE,
				8:  uint64(StorageKeyLen),
				9:  33 * PageSize,
				10: 0, // delete the storage cause NONE
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: StorageKey, Access: AccessMode{Writable: true}}, // Invalid key lead to NONE error
					33: {Value: StorageValue, Access: AccessMode{Writable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: StorageKey, Access: AccessMode{Writable: true}},
					33: {Value: StorageValue, Access: AccessMode{Writable: true}},
				},
			},
			initialServiceAccount: &ServiceAccountForTest{
				Storage: map[string]ByteSlice{
					Service_0_StorageKeyForHost.String(): StorageValue,
				},
				Lookup: map[string]LookupObjectForTest{
					PreimageBlobHash.String(): {
						T: LookupValue,
						L: PreimageBlobLen,
					},
				},
				Preimage: map[string]ByteSlice{
					Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
				},
				CodeHash:  PreimageBlobHash.String(),
				Balance:   Service_0_Balance,
				GasLimitG: Service_0_GasLimitG,
				GasLimitM: Service_0_GasLimitM,
			},
			expectedServiceAccount: &ServiceAccountForTest{
				Storage: map[string]ByteSlice{}, // the storage was deleted
				Lookup: map[string]LookupObjectForTest{
					PreimageBlobHash.String(): {
						T: LookupValue,
						L: PreimageBlobLen,
					},
				},
				Preimage: map[string]ByteSlice{
					Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
				},
				CodeHash:  PreimageBlobHash.String(),
				Balance:   Service_0_Balance,
				GasLimitG: Service_0_GasLimitG,
				GasLimitM: Service_0_GasLimitM,
			},
			initialDelta:        map[uint32]*ServiceAccountForTest{},
			expectedDelta:       map[uint32]*ServiceAccountForTest{},
			initialServiceIndex: 0,
		},
		{
			filename:    "./Info/hostInfoOK.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7: 0,
				8: 32 * PageSize,
			},
			expectedRegs: map[uint32]uint64{
				7: OK,
				8: 32 * PageSize,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: ByteSlice(m_for_hostinfo), Access: AccessMode{Writable: true}},
				},
			},
			initialServiceAccount: &ServiceAccountForTest{
				Storage:  map[string]ByteSlice{},
				Lookup:   map[string]LookupObjectForTest{},
				Preimage: map[string]ByteSlice{},
			},
			expectedServiceAccount: &ServiceAccountForTest{
				Storage:  map[string]ByteSlice{},
				Lookup:   map[string]LookupObjectForTest{},
				Preimage: map[string]ByteSlice{},
			},
			initialDelta: map[uint32]*ServiceAccountForTest{
				0: {
					Storage: map[string]ByteSlice{
						Service_0_StorageKeyForHost.String(): StorageValue,
					},
					Lookup: map[string]LookupObjectForTest{
						PreimageBlobHash.String(): LookupObjectForTest{
							T: LookupValue,
							L: PreimageBlobLen,
						},
					},
					Preimage: map[string]ByteSlice{
						Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
					},
					CodeHash:  PreimageBlobHash.String(),
					Balance:   Service_0_Balance,
					GasLimitG: Service_0_GasLimitG,
					GasLimitM: Service_0_GasLimitM,
				},
			},
			expectedDelta: map[uint32]*ServiceAccountForTest{
				0: {
					Storage: map[string]ByteSlice{
						Service_0_StorageKeyForHost.String(): StorageValue,
					},
					Lookup: map[string]LookupObjectForTest{
						PreimageBlobHash.String(): LookupObjectForTest{
							T: LookupValue,
							L: PreimageBlobLen,
						},
					},
					Preimage: map[string]ByteSlice{
						Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
					},
					CodeHash:  PreimageBlobHash.String(),
					Balance:   Service_0_Balance,
					GasLimitG: Service_0_GasLimitG,
					GasLimitM: Service_0_GasLimitM,
				},
			},
			initialServiceIndex: 0,
		},
		{
			filename:    "./Info/hostInfoOOB.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7: 0,
				8: 32 * PageSize,
			},
			expectedRegs: map[uint32]uint64{
				7: OOB,
				8: 32 * PageSize,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: make(ByteSlice, 0), Access: AccessMode{Inaccessible: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: make(ByteSlice, 0), Access: AccessMode{Inaccessible: true}},
				},
			},
			initialServiceAccount: &ServiceAccountForTest{
				Storage:  map[string]ByteSlice{},
				Lookup:   map[string]LookupObjectForTest{},
				Preimage: map[string]ByteSlice{},
			},
			expectedServiceAccount: &ServiceAccountForTest{
				Storage:  map[string]ByteSlice{},
				Lookup:   map[string]LookupObjectForTest{},
				Preimage: map[string]ByteSlice{},
			},
			initialDelta: map[uint32]*ServiceAccountForTest{
				0: {
					Storage: map[string]ByteSlice{
						Service_0_StorageKeyForHost.String(): StorageValue,
					},
					Lookup: map[string]LookupObjectForTest{
						PreimageBlobHash.String(): LookupObjectForTest{
							T: LookupValue,
							L: PreimageBlobLen,
						},
					},
					Preimage: map[string]ByteSlice{
						Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
					},
					CodeHash:  PreimageBlobHash.String(),
					Balance:   Service_0_Balance,
					GasLimitG: Service_0_GasLimitG,
					GasLimitM: Service_0_GasLimitM,
				},
			},
			expectedDelta: map[uint32]*ServiceAccountForTest{
				0: {
					Storage: map[string]ByteSlice{
						Service_0_StorageKeyForHost.String(): StorageValue,
					},
					Lookup: map[string]LookupObjectForTest{
						PreimageBlobHash.String(): LookupObjectForTest{
							T: LookupValue,
							L: PreimageBlobLen,
						},
					},
					Preimage: map[string]ByteSlice{
						Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
					},
					CodeHash:  PreimageBlobHash.String(),
					Balance:   Service_0_Balance,
					GasLimitG: Service_0_GasLimitG,
					GasLimitM: Service_0_GasLimitM,
				},
			},
			initialServiceIndex: 0,
		},
		{
			filename:    "./Info/hostInfoNONE.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7: 999, // Invalid service index
				8: 32 * PageSize,
			},
			expectedRegs: map[uint32]uint64{
				7: NONE,
				8: 32 * PageSize,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
				},
			},
			initialServiceAccount: &ServiceAccountForTest{
				Storage:  map[string]ByteSlice{},
				Lookup:   map[string]LookupObjectForTest{},
				Preimage: map[string]ByteSlice{},
			},
			expectedServiceAccount: &ServiceAccountForTest{
				Storage:  map[string]ByteSlice{},
				Lookup:   map[string]LookupObjectForTest{},
				Preimage: map[string]ByteSlice{},
			},
			initialDelta: map[uint32]*ServiceAccountForTest{
				0: {
					Storage: map[string]ByteSlice{
						Service_0_StorageKeyForHost.String(): StorageValue,
					},
					Lookup: map[string]LookupObjectForTest{
						PreimageBlobHash.String(): LookupObjectForTest{
							T: LookupValue,
							L: PreimageBlobLen,
						},
					},
					Preimage: map[string]ByteSlice{
						Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
					},
					CodeHash:  PreimageBlobHash.String(),
					Balance:   Service_0_Balance,
					GasLimitG: Service_0_GasLimitG,
					GasLimitM: Service_0_GasLimitM,
				},
			},
			expectedDelta: map[uint32]*ServiceAccountForTest{
				0: {
					Storage: map[string]ByteSlice{
						Service_0_StorageKeyForHost.String(): StorageValue,
					},
					Lookup: map[string]LookupObjectForTest{
						PreimageBlobHash.String(): LookupObjectForTest{
							T: LookupValue,
							L: PreimageBlobLen,
						},
					},
					Preimage: map[string]ByteSlice{
						Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
					},
					CodeHash:  PreimageBlobHash.String(),
					Balance:   Service_0_Balance,
					GasLimitG: Service_0_GasLimitG,
					GasLimitM: Service_0_GasLimitM,
				},
			},
			initialServiceIndex: 0,
		},
	}

	for _, tc := range testCases {
		filePath := filepath.Join(dirPath, tc.filename)
		dir := filepath.Dir(filePath)
		if err := os.MkdirAll(dir, os.ModePerm); err != nil {
			fmt.Printf("Failed to create directory %s: %v\n", dir, err)
			continue
		}

		var testcase GeneralTestcase
		if err := ReadJSONFile(filePath, &testcase); err != nil {
			// If the file is not found, create a new test case
			modifiedContent, err := json.MarshalIndent(testcase, "", "  ")
			if err != nil {
				fmt.Printf("Failed to marshal modified test case: %v\n", err)
				continue
			}
			// Save to file
			outputPath := filepath.Join(dirPath, tc.filename)
			err = WriteJSONFile(outputPath, modifiedContent)
			if err != nil {
				fmt.Printf("Failed to write modified test case to file: %v\n", err)
				continue
			}
		}

		updateGeneralTestCase(&testcase, tc.filename, tc.initialGas, tc.expectedGas, tc.initialRegs, tc.expectedRegs, tc.initialMemory, tc.expectedMemory, tc.initialServiceAccount, tc.expectedServiceAccount, tc.initialDelta, tc.expectedDelta, tc.initialServiceIndex)

		if err := WriteJSONFile(filePath, testcase); err != nil {
			fmt.Printf("Failed to write test case %s: %v\n", tc.filename, err)
		}
	}

	// LATER: "Gas":      {OK},
	// TODO: William
	// "Lookup":   {OK, NONE, OOB},
	// "Read":  {OK, OOB, NONE},
	// "Write": {OK, OOB, FULL, OOB},

	// TODO: Shawn
	// "Info":     {OK, OOB, NONE},

	// TODO: Sourabh
	// "Sp1Groth16Verify":  {OK, OOB, HUH},

}

func updateGeneralTestCase(testcase *GeneralTestcase, name string, initialGas, expectedGas uint64, initialRegs, expectedRegs map[uint32]uint64, initialMemory, expectedMemory RAMForTest, initialServiceAccount, expectedServiceAccount *ServiceAccountForTest, initialDelta, expectedDelta map[uint32]*ServiceAccountForTest, initialServiceIndex uint32) {
	start := strings.LastIndex(name, "/") + 1
	end := strings.LastIndex(name, ".json")

	testcase.Name = name[start:end]
	testcase.InitialGas = initialGas
	testcase.ExpectedGas = expectedGas
	testcase.InitialRegs = initialRegs
	testcase.ExpectedRegs = expectedRegs
	testcase.InitialMemory = initialMemory
	testcase.ExpectedMemory = expectedMemory
	testcase.InitialServiceAccount = initialServiceAccount
	testcase.ExpectedServiceAccount = expectedServiceAccount
	testcase.InitialDelta = initialDelta
	testcase.ExpectedDelta = expectedDelta
	testcase.InitialServiceIndex = initialServiceIndex
}

func TestGenerateRefineTestVectors(t *testing.T) {
	dirPath := "../jamtestvectors/host_function"

	testCases := []struct {
		filename              string
		initialGas            uint64
		expectedGas           uint64
		expectedResultCode    uint8
		initialRegs           map[uint32]uint64
		expectedRegs          map[uint32]uint64
		initialMemory         RAMForTest
		expectedMemory        RAMForTest
		initialRefineM_map    map[uint32]*RefineMForTest
		expectedRefineM_map   map[uint32]*RefineMForTest
		initialImportSegment  []ByteSlice
		expectedImportSegment []ByteSlice
		initialExportSegment  []ByteSlice
		expectedExportSegment []ByteSlice

		initialWorkItemIndex    uint32
		initialWorkPackage      types.WorkPackage
		initialAuthorizerOutput ByteSlice
		initialExtrinsic        ByteSlice

		initialSegmentIdx   uint64
		initialServiceIndex uint32
		initialDelta        map[uint32]*ServiceAccountForTest
		initialTimeslot     uint32
	}{
		{
			filename:           "./Fetch/hostFetchOK_0.json",
			initialGas:         InitialGas,
			expectedGas:        ExpectedGas,
			expectedResultCode: OK,
			initialRegs: map[uint32]uint64{
				7:  32 * PageSize,
				8:  0,
				9:  1000,
				10: 0, // data type
				11: 0,
				12: 0,
			},
			expectedRegs: map[uint32]uint64{
				7:  uint64(len(EncodedWorkPackage)),
				8:  0,
				9:  1000,
				10: 0,
				11: 0,
				12: 0,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: ByteSlice{}, Access: AccessMode{Writable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: EncodedWorkPackage, Access: AccessMode{Writable: true}},
				},
			},
			initialImportSegment:    []ByteSlice{ImportSegment},
			initialWorkItemIndex:    0,
			initialWorkPackage:      WorkPackage,
			initialAuthorizerOutput: AuthorizerOutput,
			initialExtrinsic:        Extrinsic,
		},
		{
			filename:           "./Fetch/hostFetchOK_1.json",
			initialGas:         InitialGas,
			expectedGas:        ExpectedGas,
			expectedResultCode: OK,
			initialRegs: map[uint32]uint64{
				7:  32 * PageSize,
				8:  0,
				9:  1000,
				10: 1, // data type
				11: 0,
				12: 0,
			},
			expectedRegs: map[uint32]uint64{
				7:  uint64(len(AuthorizerOutput)),
				8:  0,
				9:  1000,
				10: 1,
				11: 0,
				12: 0,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: ByteSlice{}, Access: AccessMode{Writable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: AuthorizerOutput, Access: AccessMode{Writable: true}},
				},
			},
			initialImportSegment:    []ByteSlice{ImportSegment},
			initialWorkItemIndex:    0,
			initialWorkPackage:      WorkPackage,
			initialAuthorizerOutput: AuthorizerOutput,
			initialExtrinsic:        Extrinsic,
		},
		{
			filename:           "./Fetch/hostFetchOK_2.json",
			initialGas:         InitialGas,
			expectedGas:        ExpectedGas,
			expectedResultCode: OK,
			initialRegs: map[uint32]uint64{
				7:  32 * PageSize,
				8:  0,
				9:  1000,
				10: 2, // data type
				11: 0,
				12: 0,
			},
			expectedRegs: map[uint32]uint64{
				7:  uint64(len(WorkPackage.WorkItems[0].Payload)),
				8:  0,
				9:  1000,
				10: 2,
				11: 0,
				12: 0,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: ByteSlice{}, Access: AccessMode{Writable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: WorkPackage.WorkItems[0].Payload, Access: AccessMode{Writable: true}},
				},
			},
			initialImportSegment:    []ByteSlice{ImportSegment},
			initialWorkItemIndex:    0,
			initialWorkPackage:      WorkPackage,
			initialAuthorizerOutput: AuthorizerOutput,
			initialExtrinsic:        Extrinsic,
		},

		// "Export": {OK, OOB, FULL},
		{
			filename:    "./Export/hostExportOK.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7: 32 * PageSize,
				8: uint64(PreimageBlobLen),
			},
			expectedRegs: map[uint32]uint64{
				7: 1, // Initial index + length
				8: uint64(PreimageBlobLen),
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: ImportSegment, Access: AccessMode{Readable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: ImportSegment, Access: AccessMode{Readable: true}},
				},
			},
			initialExportSegment:  []ByteSlice{{}},
			expectedExportSegment: []ByteSlice{{}, append(ImportSegment, make(ByteSlice, 24)...)},
			initialSegmentIdx:     0,
		},
		{
			filename:    "./Export/hostExportOK_gt_wg.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7: 32 * PageSize,
				8: uint64(len(ImportSegment)) * 3,
			},
			expectedRegs: map[uint32]uint64{
				7: 1, // Initial index + length
				8: uint64(len(ImportSegment)) * 3,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: append(append(append(ImportSegment, ImportSegment...), ImportSegment...), ImportSegment...), Access: AccessMode{Readable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: append(append(append(ImportSegment, ImportSegment...), ImportSegment...), ImportSegment...), Access: AccessMode{Readable: true}},
				},
			},
			initialExportSegment: []ByteSlice{{}},
			expectedExportSegment: []ByteSlice{
				{},
				append(append(ImportSegment, ImportSegment...), ImportSegment...),
			},
			initialSegmentIdx: 0,
		},
		{
			filename:    "./Export/hostExportOOB.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7: 32 * PageSize,
				8: uint64(PreimageBlobLen),
			},
			expectedRegs: map[uint32]uint64{
				7: OOB,
				8: uint64(PreimageBlobLen),
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: ImportSegment, Access: AccessMode{Inaccessible: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: ImportSegment, Access: AccessMode{Inaccessible: true}},
				},
			},
			initialExportSegment:  []ByteSlice{{}},
			expectedExportSegment: []ByteSlice{{}},
			initialSegmentIdx:     0,
		},
		{
			filename:    "./Export/hostExportFULL.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7: 32 * PageSize,
				8: uint64(PreimageBlobLen),
			},
			expectedRegs: map[uint32]uint64{
				7: FULL,
				8: uint64(PreimageBlobLen),
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: ImportSegment, Access: AccessMode{Readable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: ImportSegment, Access: AccessMode{Readable: true}},
				},
			},
			initialExportSegment:  []ByteSlice{{}},
			expectedExportSegment: []ByteSlice{{}},
			initialSegmentIdx:     9999, // Simulate FULL error
		},

		// "Historical_lookup": {OK, PANIC, NONE}
		{
			filename:           "./Historical_lookup/hostHistorical_lookupOK_d_s.json",
			initialGas:         InitialGas,
			expectedGas:        ExpectedGas,
			expectedResultCode: OK,
			initialRegs: map[uint32]uint64{
				7:  NONE,
				8:  32 * PageSize,
				9:  32 * PageSize,
				10: 100,
			},
			expectedRegs: map[uint32]uint64{
				7:  12, // v length
				8:  32 * PageSize,
				9:  32 * PageSize,
				10: 100,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Writable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlob, Access: AccessMode{Writable: true}},
				},
			},
			initialServiceIndex: 0,
			initialDelta: map[uint32]*ServiceAccountForTest{
				0: {
					Storage: map[string]ByteSlice{
						Service_0_StorageKeyForHost.String(): StorageValue,
					},
					Lookup: map[string]LookupObjectForTest{
						PreimageBlobHash.String(): LookupObjectForTest{
							T: []uint32{100},
							L: PreimageBlobLen,
						},
					},
					Preimage: map[string]ByteSlice{
						Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
					},
					CodeHash:  PreimageBlobHash.String(),
					Balance:   100,
					GasLimitG: 100,
					GasLimitM: 100,
				},
			},
			initialTimeslot: 1000,
		},
		{
			filename:           "./Historical_lookup/hostHistorical_lookupOK_d_omega7.json",
			initialGas:         InitialGas,
			expectedGas:        ExpectedGas,
			expectedResultCode: OK,
			initialRegs: map[uint32]uint64{
				7:  0,
				8:  32 * PageSize,
				9:  32 * PageSize,
				10: 100,
			},
			expectedRegs: map[uint32]uint64{
				7:  12, // v length
				8:  32 * PageSize,
				9:  32 * PageSize,
				10: 100,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Writable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlob, Access: AccessMode{Writable: true}},
				},
			},
			initialServiceIndex: 0,
			initialDelta: map[uint32]*ServiceAccountForTest{
				0: {
					Storage: map[string]ByteSlice{
						Service_0_StorageKeyForHost.String(): StorageValue,
					},
					Lookup: map[string]LookupObjectForTest{
						PreimageBlobHash.String(): LookupObjectForTest{
							T: []uint32{100},
							L: PreimageBlobLen,
						},
					},
					Preimage: map[string]ByteSlice{
						Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
					},
					CodeHash:  PreimageBlobHash.String(),
					Balance:   Service_0_Balance,
					GasLimitG: Service_0_GasLimitG,
					GasLimitM: Service_0_GasLimitM,
				},
			},
			initialTimeslot: 1000,
		},
		{
			filename:           "./Historical_lookup/hostHistorical_lookupPANIC.json",
			initialGas:         InitialGas,
			expectedGas:        ExpectedGas,
			expectedResultCode: types.PVM_PANIC,
			initialRegs: map[uint32]uint64{
				7:  0,
				8:  32 * PageSize,
				9:  32 * PageSize,
				10: 100,
			},
			expectedRegs: map[uint32]uint64{
				7:  0,
				8:  32 * PageSize,
				9:  32 * PageSize,
				10: 100,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Inaccessible: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Inaccessible: true}},
				},
			},
			initialServiceIndex: 0,
			initialDelta: map[uint32]*ServiceAccountForTest{
				0: {
					Storage: map[string]ByteSlice{
						Service_0_StorageKeyForHost.String(): StorageValue,
					},
					Lookup: map[string]LookupObjectForTest{
						PreimageBlobHash.String(): LookupObjectForTest{
							T: []uint32{100},
							L: PreimageBlobLen,
						},
					},
					Preimage: map[string]ByteSlice{
						Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
					},
					CodeHash:  PreimageBlobHash.String(),
					Balance:   Service_0_Balance,
					GasLimitG: Service_0_GasLimitG,
					GasLimitM: Service_0_GasLimitM,
				},
			},
			initialTimeslot: 1000,
		},
		{
			filename:           "./Historical_lookup/hostHistorical_lookupNONE.json",
			initialGas:         InitialGas,
			expectedGas:        ExpectedGas,
			expectedResultCode: OK,
			initialRegs: map[uint32]uint64{
				7:  0,
				8:  32 * PageSize,
				9:  32 * PageSize,
				10: 100,
			},
			expectedRegs: map[uint32]uint64{
				7:  NONE,
				8:  32 * PageSize,
				9:  32 * PageSize,
				10: 100,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Writable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Writable: true}},
				},
			},
			initialServiceIndex: 0,
			initialDelta: map[uint32]*ServiceAccountForTest{
				0: {
					Storage: map[string]ByteSlice{
						Service_0_StorageKeyForHost.String(): StorageValue,
					},
					Lookup: map[string]LookupObjectForTest{
						PreimageBlobHash.String(): LookupObjectForTest{
							T: []uint32{}, // Simulate NONE error
							L: PreimageBlobLen,
						},
					},
					Preimage: map[string]ByteSlice{
						Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
					},
					CodeHash:  PreimageBlobHash.String(),
					Balance:   Service_0_Balance,
					GasLimitG: Service_0_GasLimitG,
					GasLimitM: Service_0_GasLimitM,
				},
			},
			initialTimeslot: 1000,
		},
		{
			filename:    "./Machine/hostMachineOK.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7: 32 * PageSize,
				8: uint64(len(stote_u32_program)),
				9: uint64(init_program_counter),
			},
			expectedRegs: map[uint32]uint64{
				7: 1, // the minumm n of the machine
				8: uint64(len(stote_u32_program)),
				9: uint64(init_program_counter),
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: stote_u32_program, Access: AccessMode{Writable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: stote_u32_program, Access: AccessMode{Writable: true}},
				},
			},
			initialRefineM_map: map[uint32]*RefineMForTest{
				0: {
					P: stote_u32_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{},
					},
					I: init_program_counter,
				},
			},
			expectedRefineM_map: map[uint32]*RefineMForTest{
				0: {
					P: stote_u32_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{},
					},
					I: init_program_counter,
				},
				1: {
					P: stote_u32_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{},
					},
					I: init_program_counter,
				},
			},
			initialDelta: map[uint32]*ServiceAccountForTest{},
		},
		{
			filename:    "./Machine/hostMachineOOB.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7: 32 * PageSize,
				8: uint64(len(stote_u32_program)),
				9: uint64(init_program_counter),
			},
			expectedRegs: map[uint32]uint64{
				7: OOB,
				8: uint64(len(stote_u32_program)),
				9: uint64(init_program_counter),
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: stote_u32_program, Access: AccessMode{Inaccessible: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: stote_u32_program, Access: AccessMode{Inaccessible: true}},
				},
			},
			initialRefineM_map:  map[uint32]*RefineMForTest{},
			expectedRefineM_map: map[uint32]*RefineMForTest{},
			initialDelta:        map[uint32]*ServiceAccountForTest{},
		},
		{
			filename:    "./Peek/hostPeekOK.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7:  0,
				8:  33 * PageSize,
				9:  32 * PageSize,
				10: uint64(len(stote_u32_program)),
			},
			expectedRegs: map[uint32]uint64{
				7:  OK,
				8:  33 * PageSize,
				9:  32 * PageSize,
				10: uint64(len(stote_u32_program)),
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					33: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					33: {Value: stote_u32_program, Access: AccessMode{Writable: true}},
				},
			},
			initialRefineM_map: map[uint32]*RefineMForTest{
				0: {
					P: stote_u32_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{
							32: {Value: stote_u32_program, Access: AccessMode{Writable: true}},
						},
					},
					I: init_program_counter,
				},
			},
			expectedRefineM_map: map[uint32]*RefineMForTest{
				0: {
					P: stote_u32_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{
							32: {Value: stote_u32_program, Access: AccessMode{Writable: true}},
						},
					},
					I: init_program_counter,
				},
			},
			initialDelta: map[uint32]*ServiceAccountForTest{},
		},
		{
			filename:    "./Peek/hostPeekOOB.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7:  0,
				8:  33 * PageSize,
				9:  32 * PageSize,
				10: uint64(len(stote_u32_program)),
			},
			expectedRegs: map[uint32]uint64{
				7:  OOB,
				8:  33 * PageSize,
				9:  32 * PageSize,
				10: uint64(len(stote_u32_program)),
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					33: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					33: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
				},
			},
			initialRefineM_map: map[uint32]*RefineMForTest{
				0: {
					P: stote_u32_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{
							32: {Value: stote_u32_program, Access: AccessMode{Inaccessible: true}},
						},
					},
					I: init_program_counter,
				},
			},
			expectedRefineM_map: map[uint32]*RefineMForTest{
				0: {
					P: stote_u32_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{
							32: {Value: stote_u32_program, Access: AccessMode{Inaccessible: true}},
						},
					},
					I: init_program_counter,
				},
			},
			initialDelta: map[uint32]*ServiceAccountForTest{},
		},
		{
			filename:    "./Peek/hostPeekWHO.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7:  999, // Invalid machine index to simulate WHO error
				8:  33 * PageSize,
				9:  32 * PageSize,
				10: uint64(len(stote_u32_program)),
			},
			expectedRegs: map[uint32]uint64{
				7:  WHO,
				8:  33 * PageSize,
				9:  32 * PageSize,
				10: uint64(len(stote_u32_program)),
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					33: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					33: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
				},
			},
			initialRefineM_map: map[uint32]*RefineMForTest{
				0: {
					P: stote_u32_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{
							32: {Value: stote_u32_program, Access: AccessMode{Writable: true}},
						},
					},
					I: init_program_counter,
				},
			},
			expectedRefineM_map: map[uint32]*RefineMForTest{
				0: {
					P: stote_u32_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{
							32: {Value: stote_u32_program, Access: AccessMode{Writable: true}},
						},
					},
					I: init_program_counter,
				},
			},
			initialDelta: map[uint32]*ServiceAccountForTest{},
		},
		{
			filename:    "./Poke/hostPokeOK.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7:  0,
				8:  32 * PageSize,
				9:  33 * PageSize,
				10: uint64(len(stote_u32_program)),
			},
			expectedRegs: map[uint32]uint64{
				7:  OK,
				8:  32 * PageSize,
				9:  33 * PageSize,
				10: uint64(len(stote_u32_program)),
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: stote_u32_program, Access: AccessMode{Writable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: stote_u32_program, Access: AccessMode{Writable: true}},
				},
			},
			initialRefineM_map: map[uint32]*RefineMForTest{
				0: {
					P: stote_u32_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{
							33: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
						},
					},
					I: init_program_counter,
				},
			},
			expectedRefineM_map: map[uint32]*RefineMForTest{
				0: {
					P: stote_u32_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{
							33: {Value: stote_u32_program, Access: AccessMode{Writable: true}},
						},
					},
					I: init_program_counter,
				},
			},
			initialDelta: map[uint32]*ServiceAccountForTest{},
		},
		{
			filename:    "./Poke/hostPokeOOB.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7:  0,
				8:  32 * PageSize,
				9:  33 * PageSize,
				10: uint64(len(stote_u32_program)),
			},
			expectedRegs: map[uint32]uint64{
				7:  OOB,
				8:  32 * PageSize,
				9:  33 * PageSize,
				10: uint64(len(stote_u32_program)),
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: stote_u32_program, Access: AccessMode{Inaccessible: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: stote_u32_program, Access: AccessMode{Inaccessible: true}},
				},
			},
			initialRefineM_map: map[uint32]*RefineMForTest{
				0: {
					P: stote_u32_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{
							33: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
						},
					},
					I: init_program_counter,
				},
			},
			expectedRefineM_map: map[uint32]*RefineMForTest{
				0: {
					P: stote_u32_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{
							33: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
						},
					},
					I: init_program_counter,
				},
			},
			initialDelta: map[uint32]*ServiceAccountForTest{},
		},
		{
			filename:    "./Poke/hostPokeWHO.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7:  999, // Invalid machine index to simulate WHO error
				8:  32 * PageSize,
				9:  33 * PageSize,
				10: uint64(len(stote_u32_program)),
			},
			expectedRegs: map[uint32]uint64{
				7:  WHO,
				8:  32 * PageSize,
				9:  33 * PageSize,
				10: uint64(len(stote_u32_program)),
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: stote_u32_program, Access: AccessMode{Writable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: stote_u32_program, Access: AccessMode{Writable: true}},
				},
			},
			initialRefineM_map: map[uint32]*RefineMForTest{
				0: {
					P: stote_u32_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{
							33: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
						},
					},
					I: init_program_counter,
				},
			},
			expectedRefineM_map: map[uint32]*RefineMForTest{
				0: {
					P: stote_u32_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{
							33: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
						},
					},
					I: init_program_counter,
				},
			},
			initialDelta: map[uint32]*ServiceAccountForTest{},
		},
		{
			filename:    "./Zero/hostZeroOK.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7: 0,
				8: 32,
				9: 1,
			},
			expectedRegs: map[uint32]uint64{
				7: OK,
				8: 32,
				9: 1,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
				},
			},
			initialRefineM_map: map[uint32]*RefineMForTest{
				0: {
					P: stote_u32_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{
							32: {Value: make(ByteSlice, 0), Access: AccessMode{Inaccessible: true}},
						},
					},
					I: init_program_counter,
				},
			},
			expectedRefineM_map: map[uint32]*RefineMForTest{
				0: {
					P: stote_u32_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{
							32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
						},
					},
					I: init_program_counter,
				},
			},
			initialDelta: map[uint32]*ServiceAccountForTest{},
		},
		{
			filename:    "./Zero/hostZeroOOB.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7: 0,
				8: 15, // Invalid page index to simulate OOB error
				9: 1,
			},
			expectedRegs: map[uint32]uint64{
				7: OOB,
				8: 15,
				9: 1,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
				},
			},
			initialRefineM_map: map[uint32]*RefineMForTest{
				0: {
					P: stote_u32_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{
							32: {Value: make(ByteSlice, 0), Access: AccessMode{Inaccessible: true}},
						},
					},
					I: init_program_counter,
				},
			},
			expectedRefineM_map: map[uint32]*RefineMForTest{
				0: {
					P: stote_u32_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{
							32: {Value: make(ByteSlice, 0), Access: AccessMode{Inaccessible: true}},
						},
					},
					I: init_program_counter,
				},
			},
			initialDelta: map[uint32]*ServiceAccountForTest{},
		},
		{
			filename:    "./Zero/hostZeroWHO.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7: 999, // Invalid machine index to simulate WHO error
				8: 32,
				9: 1,
			},
			expectedRegs: map[uint32]uint64{
				7: WHO,
				8: 32,
				9: 1,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
				},
			},
			initialRefineM_map: map[uint32]*RefineMForTest{
				0: {
					P: stote_u32_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{
							32: {Value: make(ByteSlice, 0), Access: AccessMode{Inaccessible: true}},
						},
					},
					I: init_program_counter,
				},
			},
			expectedRefineM_map: map[uint32]*RefineMForTest{
				0: {
					P: stote_u32_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{
							32: {Value: make(ByteSlice, 0), Access: AccessMode{Inaccessible: true}},
						},
					},
					I: init_program_counter,
				},
			},
			initialDelta: map[uint32]*ServiceAccountForTest{},
		},
		{
			filename:    "./Void/hostVoidOK.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7: 0,
				8: 32,
				9: 1,
			},
			expectedRegs: map[uint32]uint64{
				7: OK,
				8: 32,
				9: 1,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
				},
			},
			initialRefineM_map: map[uint32]*RefineMForTest{
				0: {
					P: stote_u32_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{
							32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
						},
					},
					I: init_program_counter,
				},
			},
			expectedRefineM_map: map[uint32]*RefineMForTest{
				0: {
					P: stote_u32_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{
							32: {Value: make(ByteSlice, 0), Access: AccessMode{Inaccessible: true}},
						},
					},
					I: init_program_counter,
				},
			},
			initialDelta: map[uint32]*ServiceAccountForTest{},
		},
		{
			filename:    "./Void/hostVoidOOB.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7: 0,
				8: 32,
				9: 1,
			},
			expectedRegs: map[uint32]uint64{
				7: OOB,
				8: 32,
				9: 1,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
				},
			},
			initialRefineM_map: map[uint32]*RefineMForTest{
				0: {
					P: stote_u32_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{
							32: {Value: make(ByteSlice, 0), Access: AccessMode{Inaccessible: true}}, // Page already inaccessible to simulate OOB error
						},
					},
					I: init_program_counter,
				},
			},
			expectedRefineM_map: map[uint32]*RefineMForTest{
				0: {
					P: stote_u32_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{
							32: {Value: make(ByteSlice, 0), Access: AccessMode{Inaccessible: true}},
						},
					},
					I: init_program_counter,
				},
			},
			initialDelta: map[uint32]*ServiceAccountForTest{},
		},
		{
			filename:    "./Void/hostVoidWHO.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7: 999, // Invalid machine index to simulate WHO error
				8: 32,
				9: 1,
			},
			expectedRegs: map[uint32]uint64{
				7: WHO,
				8: 32,
				9: 1,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
				},
			},
			initialRefineM_map: map[uint32]*RefineMForTest{
				0: {
					P: stote_u32_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{
							32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
						},
					},
					I: init_program_counter,
				},
			},
			expectedRefineM_map: map[uint32]*RefineMForTest{
				0: {
					P: stote_u32_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{
							32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
						},
					},
					I: init_program_counter,
				},
			},
			initialDelta: map[uint32]*ServiceAccountForTest{},
		},
		{
			filename:    "./Invoke/hostInvokeOOB.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7: 0,
				8: 32 * PageSize,
			},
			expectedRegs: map[uint32]uint64{
				7: OOB,
				8: 32 * PageSize,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: make(ByteSlice, 112), Access: AccessMode{Inaccessible: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: make(ByteSlice, 112), Access: AccessMode{Inaccessible: true}},
				},
			},
			initialRefineM_map: map[uint32]*RefineMForTest{
				0: {
					P: HOST_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{
							32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
						},
					},
					I: HOST_init_pc,
				},
			},
			expectedRefineM_map: map[uint32]*RefineMForTest{
				0: {
					P: HOST_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{
							32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
						},
					},
					I: 0,
				},
			},
			initialDelta: map[uint32]*ServiceAccountForTest{},
		},
		{
			filename:    "./Invoke/hostInvokeWHO.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7: 999, // Invalid machine index to simulate WHO error
				8: 32 * PageSize,
			},
			expectedRegs: map[uint32]uint64{
				7: WHO,
				8: 32 * PageSize,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: make(ByteSlice, 112), Access: AccessMode{Writable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: make(ByteSlice, 112), Access: AccessMode{Writable: true}},
				},
			},
			initialRefineM_map: map[uint32]*RefineMForTest{
				0: {
					P: HOST_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{
							32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
						},
					},
					I: HOST_init_pc,
				},
			},
			expectedRefineM_map: map[uint32]*RefineMForTest{
				0: {
					P: HOST_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{
							32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
						},
					},
					I: 0,
				},
			},
			initialDelta: map[uint32]*ServiceAccountForTest{},
		},
		{
			filename:    "./Invoke/hostInvokeHALT.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7: 0,
				8: 32 * PageSize,
			},
			expectedRegs: map[uint32]uint64{
				7: types.PVM_HALT,
				8: 32 * PageSize,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: append(ByteSlice(HALT_expected_machine_gas_bytes), ByteSlice(HALT_expected_machine_regs_bytes)...), Access: AccessMode{Writable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: append(ByteSlice(HALT_expected_machine_gas_bytes), ByteSlice(HALT_expected_machine_regs_bytes)...), Access: AccessMode{Writable: true}},
				},
			},
			initialRefineM_map: map[uint32]*RefineMForTest{
				0: {
					P: HALT_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{
							32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
						},
					},
					I: HALT_init_pc,
				},
			},
			expectedRefineM_map: map[uint32]*RefineMForTest{
				0: {
					P: HALT_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{
							32: {Value: ByteSlice{100, 0, 0, 0}, Access: AccessMode{Writable: true}},
						},
					},
					I: HALT_expected_pc,
				},
			},
			initialDelta: map[uint32]*ServiceAccountForTest{},
		},
		{
			filename:    "./Invoke/hostInvokePANIC.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7: 0,
				8: 32 * PageSize,
			},
			expectedRegs: map[uint32]uint64{
				7: types.PVM_PANIC,
				8: 32 * PageSize,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: append(ByteSlice(PANIC_init_machine_gas_bytes), ByteSlice(PANIC_init_machine_regs_bytes)...), Access: AccessMode{Writable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: append(ByteSlice(PANIC_expected_machine_gas_bytes), ByteSlice(PANIC_expected_machine_regs_bytes)...), Access: AccessMode{Writable: true}},
				},
			},
			initialRefineM_map: map[uint32]*RefineMForTest{
				0: {
					P: PANIC_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{
							32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
						},
					},
					I: PANIC_init_pc,
				},
			},
			expectedRefineM_map: map[uint32]*RefineMForTest{
				0: {
					P: PANIC_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{
							32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
						},
					},
					I: PANIC_expected_pc,
				},
			},
			initialDelta: map[uint32]*ServiceAccountForTest{},
		},
		{
			filename:    "./Invoke/hostInvokeOOG.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7: 0,
				8: 32 * PageSize,
			},
			expectedRegs: map[uint32]uint64{
				7: types.PVM_OOG,
				8: 32 * PageSize,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: append(OOG_init_machine_gas_bytes, OOG_init_machine_regs_bytes...), Access: AccessMode{Writable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: append(OOG_expected_machine_gas_bytes, OOG_expected_machine_regs_bytes...), Access: AccessMode{Writable: true}},
				},
			},
			initialRefineM_map: map[uint32]*RefineMForTest{
				0: {
					P: OOG_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{
							32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
						},
					},
					I: OOG_init_pc,
				},
			},
			expectedRefineM_map: map[uint32]*RefineMForTest{
				0: {
					P: OOG_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{
							32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
						},
					},
					I: OOG_expected_pc,
				},
			},
			initialDelta: map[uint32]*ServiceAccountForTest{},
		},
		{
			filename:    "./Invoke/hostInvokeFAULT.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7: 0,
				8: 32 * PageSize,
			},
			expectedRegs: map[uint32]uint64{
				7: types.PVM_FAULT,
				8: 32 * PageSize,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: append(ByteSlice(FAULT_init_machine_gas_bytes), ByteSlice(FAULT_init_machine_regs_bytes)...), Access: AccessMode{Writable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: append(ByteSlice(FAULT_expected_machine_gas_bytes), ByteSlice(FAULT_expected_machine_regs_bytes)...), Access: AccessMode{Writable: true}},
				},
			},
			initialRefineM_map: map[uint32]*RefineMForTest{
				0: {
					P: FAULT_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{
							32: {Value: make(ByteSlice, 0), Access: AccessMode{Inaccessible: true}},
						},
					},
					I: FAULT_init_pc,
				},
			},
			expectedRefineM_map: map[uint32]*RefineMForTest{
				0: {
					P: FAULT_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{
							32: {Value: make(ByteSlice, 0), Access: AccessMode{Inaccessible: true}},
						},
					},
					I: FAULT_expected_pc,
				},
			},
			initialDelta: map[uint32]*ServiceAccountForTest{},
		},
		{
			filename:    "./Invoke/hostInvokeHOST.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7: 0,
				8: 32 * PageSize,
			},
			expectedRegs: map[uint32]uint64{
				7: types.PVM_HOST,
				8: GAS,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: append(ByteSlice(HOST_init_machine_gas_bytes), ByteSlice(HOST_init_machine_regs_bytes)...), Access: AccessMode{Writable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: append(ByteSlice(HOST_expected_machine_gas_bytes), ByteSlice(HOST_expected_machine_regs_bytes)...), Access: AccessMode{Writable: true}},
				},
			},
			initialRefineM_map: map[uint32]*RefineMForTest{
				0: {
					P: HOST_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{
							32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
						},
					},
					I: HOST_init_pc,
				},
			},
			expectedRefineM_map: map[uint32]*RefineMForTest{
				0: {
					P: HOST_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{
							32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
						},
					},
					I: HOST_expected_pc,
				},
			},
			initialDelta: map[uint32]*ServiceAccountForTest{},
		},
		{
			filename:    "./Expunge/hostExpungeOK.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7: 1, // the machine index to be expunged
			},
			expectedRegs: map[uint32]uint64{
				7: OK,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
				},
			},
			initialRefineM_map: map[uint32]*RefineMForTest{
				0: {
					P: HALT_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{
							32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
						},
					},
					I: HALT_init_pc,
				},
				1: {
					P: HALT_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{
							32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
						},
					},
					I: HALT_init_pc,
				},
			},
			expectedRefineM_map: map[uint32]*RefineMForTest{
				0: {
					P: HALT_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{
							32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
						},
					},
					I: HALT_init_pc,
				},
			},
			initialDelta: map[uint32]*ServiceAccountForTest{},
		},
		{
			filename:    "./Expunge/hostExpungeWHO.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7: 999, // Invalid machine index to simulate WHO error
			},
			expectedRegs: map[uint32]uint64{
				7: WHO,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
				},
			},
			initialRefineM_map: map[uint32]*RefineMForTest{
				0: {
					P: HALT_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{
							32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
						},
					},
					I: HALT_init_pc,
				},
				1: {
					P: HALT_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{
							32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
						},
					},
					I: HALT_init_pc,
				},
			},
			expectedRefineM_map: map[uint32]*RefineMForTest{
				0: {
					P: HALT_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{
							32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
						},
					},
					I: HALT_init_pc,
				},
				1: {
					P: HALT_program,
					U: &RAMForTest{
						Pages: map[uint32]*PageForTest{
							32: {Value: make(ByteSlice, 0), Access: AccessMode{Writable: true}},
						},
					},
					I: HALT_init_pc,
				},
			},
			initialDelta: map[uint32]*ServiceAccountForTest{},
		},
		// TODO: Shawn: 7 host functions
		// "Machine":  {OK, OOB},
		// "Peek":     {OK, OOB, WHO},
		// "Poke":     {OK, OOB, WHO},
		// "Zero":     {OK, OOB, WHO},
		// "Void":     {OK, OOB, WHO},
		// "Invoke":   {OK, OOB, WHO, HOST, FAULT, OOB, PANIC},
		// "Expunge":  {OK, OOB, WHO},
	}

	for _, tc := range testCases {
		filePath := filepath.Join(dirPath, tc.filename)
		dir := filepath.Dir(filePath)
		if err := os.MkdirAll(dir, os.ModePerm); err != nil {
			fmt.Printf("Failed to create directory %s: %v\n", dir, err)
			continue
		}

		var testcase RefineTestcase
		if err := ReadJSONFile(filePath, &testcase); err != nil {
			// If the file is not found, create a new test case
			modifiedContent, err := json.MarshalIndent(testcase, "", "  ")
			if err != nil {
				fmt.Printf("Failed to marshal modified test case: %v\n", err)
				continue
			}
			// Save to file
			outputPath := filepath.Join(dirPath, tc.filename)
			err = WriteJSONFile(outputPath, modifiedContent)
			if err != nil {
				fmt.Printf("Failed to write modified test case to file: %v\n", err)
				continue
			}
		}

		updateRefineTestCase(&testcase, tc.filename, tc.initialGas, tc.expectedGas, tc.expectedResultCode, tc.initialRegs, tc.expectedRegs, tc.initialMemory, tc.expectedMemory, tc.initialRefineM_map, tc.expectedRefineM_map, tc.initialImportSegment, tc.expectedImportSegment, tc.initialExportSegment, tc.expectedExportSegment, tc.initialSegmentIdx, tc.initialServiceIndex, tc.initialDelta, tc.initialTimeslot, tc.initialWorkItemIndex, tc.initialWorkPackage, tc.initialAuthorizerOutput, tc.initialExtrinsic)

		if err := WriteJSONFile(filePath, testcase); err != nil {
			fmt.Printf("Failed to write test case %s: %v\n", tc.filename, err)
		}
	}
}

func updateRefineTestCase(testcase *RefineTestcase, name string, initialGas, expectedGas uint64, expectedResultCode uint8, initialRegs, expectedRegs map[uint32]uint64, initialMemory, expectedMemory RAMForTest, initialRefineM_map, expectedRefineM_map map[uint32]*RefineMForTest, initialImportSegment, expectedImportSegment, initialExportSegment, expectedExportSegment []ByteSlice, initialSegmentIdx uint64, initialServiceIndex uint32, initialDelta map[uint32]*ServiceAccountForTest, initialTimeslot uint32, initialWorkItemIndex uint32, initialWorkPackage types.WorkPackage, initialAuthorizerOutput ByteSlice, initialExtrinsic ByteSlice) {
	start := strings.LastIndex(name, "/") + 1
	end := strings.LastIndex(name, ".json")

	testcase.Name = name[start:end]
	testcase.InitialGas = initialGas
	testcase.ExpectedGas = expectedGas
	testcase.ExpectedResultCode = expectedResultCode
	testcase.InitialRegs = initialRegs
	testcase.ExpectedRegs = expectedRegs
	testcase.InitialMemory = initialMemory
	testcase.ExpectedMemory = expectedMemory
	testcase.InitialRefineM_map = initialRefineM_map
	testcase.ExpectedRefineM_map = expectedRefineM_map
	testcase.InitialImportSegment = initialImportSegment

	testcase.InitialExportSegment = initialExportSegment
	testcase.ExpectedExportSegment = expectedExportSegment

	testcase.InitialExportSegmentIdx = uint32(initialSegmentIdx)
	testcase.InitialServiceIndex = initialServiceIndex
	testcase.InitialDelta = initialDelta
	testcase.InitialTimeslot = initialTimeslot

	testcase.InitialWorkItemIndex = initialWorkItemIndex
	testcase.InitialWorkPackage = initialWorkPackage
	testcase.InitialAuthorizerOutput = initialAuthorizerOutput
	testcase.InitialExtrinsic = initialExtrinsic
}

func TestGenerateAccumulateTestVectors(t *testing.T) {
	dirPath := "../jamtestvectors/host_function"

	testCases := []struct {
		filename           string
		initialGas         uint64
		expectedGas        uint64
		initialRegs        map[uint32]uint64
		expectedRegs       map[uint32]uint64
		initialMemory      RAMForTest
		expectedMemory     RAMForTest
		initialXcontent_x  *XContextForTest
		expectedXcontent_x *XContextForTest
		InitialTimeslot    uint32
	}{
		// 		"New":      {OK, PANIC, CASH},
		{
			filename:    "./New/hostNewOK.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7:  32 * PageSize,
				8:  uint64(PreimageBlobLen),
				9:  Service_1_GasLimitG,
				10: Service_1_GasLimitM,
			},
			expectedRegs: map[uint32]uint64{
				7:  1,
				8:  uint64(PreimageBlobLen),
				9:  Service_1_GasLimitG,
				10: Service_1_GasLimitM,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Readable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Readable: true}},
				},
			},
			initialXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: LookupValue,
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
			expectedXcontent_x: &XContextForTest{
				I: 555,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: LookupValue,
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   807,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
						1: {
							Storage:  map[string]ByteSlice{},
							Preimage: map[string]ByteSlice{},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: []uint32{},
									L: PreimageBlobLen,
								},
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   193,
							GasLimitG: Service_1_GasLimitG,
							GasLimitM: Service_1_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
		},
		{
			filename:    "./New/hostNewPANIC.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7:  32 * PageSize,
				8:  uint64(PreimageBlobLen),
				9:  Service_1_GasLimitG,
				10: Service_1_GasLimitM,
			},
			expectedRegs: map[uint32]uint64{
				7:  32 * PageSize,
				8:  uint64(PreimageBlobLen),
				9:  Service_1_GasLimitG,
				10: Service_1_GasLimitM,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Inaccessible: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Inaccessible: true}},
				},
			},
			initialXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: LookupValue,
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
			expectedXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: LookupValue,
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
		},
		{
			filename:    "./New/hostNewCASH.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7:  32 * PageSize,
				8:  uint64(PreimageBlobLen),
				9:  Service_1_GasLimitG,
				10: Service_1_GasLimitM,
			},
			expectedRegs: map[uint32]uint64{
				7:  CASH,
				8:  uint64(PreimageBlobLen),
				9:  Service_1_GasLimitG,
				10: Service_1_GasLimitM,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Readable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Readable: true}},
				},
			},
			initialXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: LookupValue,
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   300, // Simulate CASH error
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
			expectedXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: LookupValue,
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   300, // Simulate CASH error
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
		},
		// 		"Upgrade":  {OK, OOB},
		{
			filename:    "./Upgrade/hostUpgradeOK.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7: 32 * PageSize,
				8: 1000,
				9: 2000,
			},
			expectedRegs: map[uint32]uint64{
				7: OK,
				8: 1000,
				9: 2000,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: make(ByteSlice, 32), Access: AccessMode{Readable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: make(ByteSlice, 32), Access: AccessMode{Readable: true}},
				},
			},
			initialXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: LookupValue,
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
			expectedXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: LookupValue,
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  "0x0000000000000000000000000000000000000000000000000000000000000000",
							Balance:   Service_0_Balance,
							GasLimitG: 1000,
							GasLimitM: 2000,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
		},
		{
			filename:    "./Upgrade/hostUpgradeOOB.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7: 32 * PageSize,
				8: 1000,
				9: 2000,
			},
			expectedRegs: map[uint32]uint64{
				7: OOB,
				8: 1000,
				9: 2000,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: make(ByteSlice, 32), Access: AccessMode{Inaccessible: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: make(ByteSlice, 32), Access: AccessMode{Inaccessible: true}},
				},
			},
			initialXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: LookupValue,
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
			expectedXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: LookupValue,
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
		},
		// 		"Transfer":  {OK, OOB, WHO, CASH, LOW, HIGH},
		{
			filename:    "./Transfer/hostTransferOK.json",
			initialGas:  10000000000000,
			expectedGas: 10000000000000 - 10 - 100 - (1<<32)*100,
			initialRegs: map[uint32]uint64{
				7:  1,
				8:  100,
				9:  100,
				10: 32 * PageSize,
			},
			expectedRegs: map[uint32]uint64{
				7:  OK,
				8:  100,
				9:  100,
				10: 32 * PageSize,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: make(ByteSlice, 128), Access: AccessMode{Readable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: make(ByteSlice, 128), Access: AccessMode{Readable: true}},
				},
			},
			initialXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: LookupValue,
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
						1: {
							Storage: map[string]ByteSlice{
								Service_1_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: LookupValue,
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_1_Balance,
							GasLimitG: Service_1_GasLimitG,
							GasLimitM: Service_1_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
			expectedXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: LookupValue,
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance - 100,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
						1: {
							Storage: map[string]ByteSlice{
								Service_1_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: LookupValue,
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_1_Balance,
							GasLimitG: Service_1_GasLimitG,
							GasLimitM: Service_1_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{
					{
						SenderIndex:   0,
						ReceiverIndex: 1,
						Amount:        100,
						GasLimit:      100,
						Memo:          make(ByteSlice, 128),
					},
				},
			},
		},
		{
			filename:    "./Transfer/hostTransferOOB.json",
			initialGas:  10000000000000,
			expectedGas: 10000000000000 - 10 - 100 - (1<<32)*100,
			initialRegs: map[uint32]uint64{
				7:  1,
				8:  100,
				9:  100,
				10: 32 * PageSize,
			},
			expectedRegs: map[uint32]uint64{
				7:  OOB,
				8:  100,
				9:  100,
				10: 32 * PageSize,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: make(ByteSlice, 32), Access: AccessMode{Inaccessible: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: make(ByteSlice, 32), Access: AccessMode{Inaccessible: true}},
				},
			},
			initialXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: LookupValue,
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
						1: {
							Storage: map[string]ByteSlice{
								Service_1_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: LookupValue,
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_1_Balance,
							GasLimitG: Service_1_GasLimitG,
							GasLimitM: Service_1_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
			expectedXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: LookupValue,
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
						1: {
							Storage: map[string]ByteSlice{
								Service_1_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: LookupValue,
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_1_Balance,
							GasLimitG: Service_1_GasLimitG,
							GasLimitM: Service_1_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
		},
		{
			filename:    "./Transfer/hostTransferWHO.json",
			initialGas:  10000000000000,
			expectedGas: 10000000000000 - 10 - 100 - (1<<32)*100,
			initialRegs: map[uint32]uint64{
				7:  999, // Invalid recipient leads to WHO
				8:  100,
				9:  100,
				10: 32 * PageSize,
			},
			expectedRegs: map[uint32]uint64{
				7:  WHO,
				8:  100,
				9:  100,
				10: 32 * PageSize,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: make(ByteSlice, 32), Access: AccessMode{Readable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: make(ByteSlice, 32), Access: AccessMode{Readable: true}},
				},
			},
			initialXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: LookupValue,
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
						1: {
							Storage: map[string]ByteSlice{
								Service_1_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: LookupValue,
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_1_Balance,
							GasLimitG: Service_1_GasLimitG,
							GasLimitM: Service_1_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
			expectedXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: LookupValue,
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
						1: {
							Storage: map[string]ByteSlice{
								Service_1_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: LookupValue,
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_1_Balance,
							GasLimitG: Service_1_GasLimitG,
							GasLimitM: Service_1_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
		},
		{
			filename:    "./Transfer/hostTransferLOW.json",
			initialGas:  10000000000000,
			expectedGas: 10000000000000 - 10 - 100 - (1<<32)*10,
			initialRegs: map[uint32]uint64{
				7:  1,
				8:  100,
				9:  10, // gas lower than recipient's gas limit m leads to LOW
				10: 32 * PageSize,
			},
			expectedRegs: map[uint32]uint64{
				7:  LOW,
				8:  100,
				9:  10, // gas lower than recipient's gas limit m leads to LOW
				10: 32 * PageSize,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: make(ByteSlice, 32), Access: AccessMode{Readable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: make(ByteSlice, 32), Access: AccessMode{Readable: true}},
				},
			},
			initialXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: LookupValue,
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
						1: {
							Storage: map[string]ByteSlice{
								Service_1_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: LookupValue,
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_1_Balance,
							GasLimitG: Service_1_GasLimitG,
							GasLimitM: Service_1_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
			expectedXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: LookupValue,
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
						1: {
							Storage: map[string]ByteSlice{
								Service_1_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: LookupValue,
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_1_Balance,
							GasLimitG: Service_1_GasLimitG,
							GasLimitM: Service_1_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
		},
		{
			filename:    "./Transfer/hostTransferCASH.json",
			initialGas:  10000000000000,
			expectedGas: 10000000000000 - 10 - 1000 - (1<<32)*100,
			initialRegs: map[uint32]uint64{
				7:  1,
				8:  1000, // Sender has insufficient funds leads to CASH
				9:  100,
				10: 32 * PageSize,
			},
			expectedRegs: map[uint32]uint64{
				7:  CASH,
				8:  1000, // Sender has insufficient funds leads to CASH
				9:  100,
				10: 32 * PageSize,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: make(ByteSlice, 32), Access: AccessMode{Readable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: make(ByteSlice, 32), Access: AccessMode{Readable: true}},
				},
			},
			initialXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: LookupValue,
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
						1: {
							Storage: map[string]ByteSlice{
								Service_1_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: LookupValue,
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_1_Balance,
							GasLimitG: Service_1_GasLimitG,
							GasLimitM: Service_1_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
			expectedXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: LookupValue,
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
						1: {
							Storage: map[string]ByteSlice{
								Service_1_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: LookupValue,
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_1_Balance,
							GasLimitG: Service_1_GasLimitG,
							GasLimitM: Service_1_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
		},
		// Eject
		{
			filename:    "./Eject/hostEjectOK.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7: 1,
				8: 32 * PageSize,
			},
			expectedRegs: map[uint32]uint64{
				7: OK,
				8: 32 * PageSize,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Readable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Readable: true}},
				},
			},
			initialXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: LookupValue,
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
						1: {
							Storage: map[string]ByteSlice{}, // make sure i of service is 2
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: []uint32{10, 100}, // y < t - D leads to OK
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  common.HexString(types.E_l(0, 32)),
							Balance:   Service_1_Balance,
							GasLimitG: Service_1_GasLimitG,
							GasLimitM: Service_1_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
			expectedXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: LookupValue,
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance + Service_1_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
		},
		{
			filename:    "./Eject/hostEjectOOB.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7: 1,
				8: 32 * PageSize,
			},
			expectedRegs: map[uint32]uint64{
				7: OOB,
				8: 32 * PageSize,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Inaccessible: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Inaccessible: true}},
				},
			},
			initialXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: LookupValue,
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
						1: {
							Storage: map[string]ByteSlice{}, // make sure i of service is 2
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: []uint32{10, 100}, // OK to eject
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_1_Balance,
							GasLimitG: Service_1_GasLimitG,
							GasLimitM: Service_1_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
			expectedXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: LookupValue,
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
						1: {
							Storage: map[string]ByteSlice{}, // make sure i of service is 2
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: []uint32{10, 100}, // OK to eject
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_1_Balance,
							GasLimitG: Service_1_GasLimitG,
							GasLimitM: Service_1_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
		},
		{
			filename:    "./Eject/hostEjectWHO.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7: 0, // d equals to xs leads to WHO
				8: 32 * PageSize,
			},
			expectedRegs: map[uint32]uint64{
				7: WHO,
				8: 32 * PageSize,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Readable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Readable: true}},
				},
			},
			initialXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: LookupValue,
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
						1: {
							Storage: map[string]ByteSlice{}, // make sure i of service is 2
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: []uint32{10, 100}, // OK to eject
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  common.HexString(types.E_l(0, 32)),
							Balance:   Service_1_Balance,
							GasLimitG: Service_1_GasLimitG,
							GasLimitM: Service_1_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
			expectedXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: LookupValue,
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
						1: {
							Storage: map[string]ByteSlice{}, // make sure i of service is 2
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: []uint32{10, 100}, // OK to eject
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  common.HexString(types.E_l(0, 32)),
							Balance:   Service_1_Balance,
							GasLimitG: Service_1_GasLimitG,
							GasLimitM: Service_1_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
		},
		{
			filename:    "./Eject/hostEjectHUH.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7: 1,
				8: 32 * PageSize,
			},
			expectedRegs: map[uint32]uint64{
				7: HUH,
				8: 32 * PageSize,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Readable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Readable: true}},
				},
			},
			initialXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: LookupValue,
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
						1: {
							Storage: map[string]ByteSlice{
								Service_1_StorageKeyForHost.String(): StorageValue, // i of service is 2+1 = 3 leads to HUH
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: []uint32{10, 100}, // OK to eject
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  common.HexString(types.E_l(0, 32)),
							Balance:   Service_1_Balance,
							GasLimitG: Service_1_GasLimitG,
							GasLimitM: Service_1_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
			expectedXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: LookupValue,
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
						1: {
							Storage: map[string]ByteSlice{
								Service_1_StorageKeyForHost.String(): StorageValue, // i of service is 2+1 = 3 leads to HUH
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: []uint32{10, 100}, // OK to eject
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  common.HexString(types.E_l(0, 32)),
							Balance:   Service_1_Balance,
							GasLimitG: Service_1_GasLimitG,
							GasLimitM: Service_1_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
		},
		// Query
		{
			filename:    "./Query/hostQueryOK_0_timeslots.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7: 32 * PageSize,
				8: uint64(PreimageBlobLen),
			},
			expectedRegs: map[uint32]uint64{
				7: 0,
				8: 0,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Readable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Readable: true}},
				},
			},
			initialXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: []uint32{}, // 0 timeslots
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
			expectedXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: []uint32{}, // 0 timeslots
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
		},
		{
			filename:    "./Query/hostQueryOK_1_timeslots.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7: 32 * PageSize,
				8: uint64(PreimageBlobLen),
			},
			expectedRegs: map[uint32]uint64{
				7: 1 + (1<<32)*10,
				8: 0,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Readable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Readable: true}},
				},
			},
			initialXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: []uint32{10}, // 1 timeslots
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
			expectedXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: []uint32{10}, // 1 timeslots
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
		},
		{
			filename:    "./Query/hostQueryOK_2_timeslots.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7: 32 * PageSize,
				8: uint64(PreimageBlobLen),
			},
			expectedRegs: map[uint32]uint64{
				7: 2 + (1<<32)*10,
				8: 100,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Readable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Readable: true}},
				},
			},
			initialXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: []uint32{10, 100}, // 2 timeslots
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
			expectedXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: []uint32{10, 100}, // 2 timeslots
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
		},
		{
			filename:    "./Query/hostQueryOK_3_timeslots.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7: 32 * PageSize,
				8: uint64(PreimageBlobLen),
			},
			expectedRegs: map[uint32]uint64{
				7: 3 + (1<<32)*10,
				8: 100 + (1<<32)*1000,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Readable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Readable: true}},
				},
			},
			initialXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: []uint32{10, 100, 1000}, // 3 timeslots
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
			expectedXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: []uint32{10, 100, 1000}, // 3 timeslots
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
		},
		{
			filename:    "./Query/hostQueryOOB.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7: 32 * PageSize,
				8: uint64(PreimageBlobLen),
			},
			expectedRegs: map[uint32]uint64{
				7: OOB,
				8: uint64(PreimageBlobLen),
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Inaccessible: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Inaccessible: true}},
				},
			},
			initialXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: []uint32{}, // 0 timeslots
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
			expectedXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: []uint32{}, // 0 timeslots
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
		},
		{
			filename:    "./Query/hostQueryNONE.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7: 32 * PageSize,
				8: uint64(PreimageBlobLen),
			},
			expectedRegs: map[uint32]uint64{
				7: NONE,
				8: 0,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: make(ByteSlice, 32), Access: AccessMode{Readable: true}}, // invaild preimage blob hash leads to NONE
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: make(ByteSlice, 32), Access: AccessMode{Readable: true}}, // invaild preimage blob hash leads to NONE
				},
			},
			initialXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: []uint32{}, // 0 timeslots
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
			expectedXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: []uint32{}, // 0 timeslots
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
		},

		//    Solicit
		{
			filename:        "./Solicit/hostSolicitOK_no_timeslots.json",
			initialGas:      InitialGas,
			expectedGas:     ExpectedGas,
			InitialTimeslot: 28800 + 1000, // the new anchor is 28800 + 1000
			initialRegs: map[uint32]uint64{
				7: 32 * PageSize,
				8: uint64(PreimageBlobLen),
			},
			expectedRegs: map[uint32]uint64{
				7: OK,
				8: uint64(PreimageBlobLen),
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Readable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Readable: true}},
				},
			},
			initialXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{}, // no timeslots
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
			expectedXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: []uint32{},
									L: 12,
								}, // init timeslots successfully
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
		},
		{
			filename:        "./Solicit/hostSolicitOK_2_timeslots.json",
			initialGas:      InitialGas,
			expectedGas:     ExpectedGas,
			InitialTimeslot: 28800 + 1000, // the new anchor is 28800 + 1000
			initialRegs: map[uint32]uint64{
				7: 32 * PageSize,
				8: uint64(PreimageBlobLen),
			},
			expectedRegs: map[uint32]uint64{
				7: OK,
				8: uint64(PreimageBlobLen),
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Readable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Readable: true}},
				},
			},
			initialXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: []uint32{10, 100}, // length is 2, so inert new anchor
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
			expectedXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: []uint32{10, 100, 28800 + 1000}, // inert new anchor successfully
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
		},
		{
			filename:        "./Solicit/hostSolicitOOB.json",
			initialGas:      InitialGas,
			expectedGas:     ExpectedGas,
			InitialTimeslot: 100,
			initialRegs: map[uint32]uint64{
				7: 32 * PageSize,
				8: uint64(PreimageBlobLen),
			},
			expectedRegs: map[uint32]uint64{
				7: OOB,
				8: uint64(PreimageBlobLen),
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Inaccessible: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Inaccessible: true}},
				},
			},
			initialXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: []uint32{10, 100},
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
			expectedXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: []uint32{10, 100},
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
		},
		{
			filename:        "./Solicit/hostSolicitHUH.json",
			initialGas:      InitialGas,
			expectedGas:     ExpectedGas,
			InitialTimeslot: 100,
			initialRegs: map[uint32]uint64{
				7: 32 * PageSize,
				8: uint64(PreimageBlobLen),
			},
			expectedRegs: map[uint32]uint64{
				7: HUH,
				8: uint64(PreimageBlobLen),
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Readable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Readable: true}},
				},
			},
			initialXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: []uint32{10}, // HUH error
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
			expectedXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): {
									T: []uint32{10}, // HUH error
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
		},
		{
			filename:        "./Solicit/hostSolicitFULL.json",
			initialGas:      InitialGas,
			expectedGas:     ExpectedGas,
			InitialTimeslot: 100,
			initialRegs: map[uint32]uint64{
				7: 32 * PageSize,
				8: uint64(PreimageBlobLen),
			},
			expectedRegs: map[uint32]uint64{
				7: FULL,
				8: uint64(PreimageBlobLen),
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Readable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Readable: true}},
				},
			},
			initialXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: []uint32{10, 100},
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   90, // insufficient balance leads to FULL
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
			expectedXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: []uint32{10, 100},
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   90, // insufficient balance leads to FULL
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
		},
		//   Forget
		{
			filename:        "./Forget/hostForgetOK_3_timeslots.json",
			initialGas:      InitialGas,
			expectedGas:     ExpectedGas,
			InitialTimeslot: 28800 + 1000,
			initialRegs: map[uint32]uint64{
				7: 32 * PageSize,
				8: uint64(PreimageBlobLen),
			},
			expectedRegs: map[uint32]uint64{
				7: OK,
				8: uint64(PreimageBlobLen),
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Readable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Readable: true}},
				},
			},
			initialXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: []uint32{10, 100, 1000}, // anchor len = 3, so adjust anchor
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
			expectedXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: []uint32{1000, 28800 + 1000}, // adjust anchor successfully
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
		},
		{
			filename:        "./Forget/hostForgetOK_2_timeslots.json",
			initialGas:      InitialGas,
			expectedGas:     ExpectedGas,
			InitialTimeslot: 28800 + 1000,
			initialRegs: map[uint32]uint64{
				7: 32 * PageSize,
				8: uint64(PreimageBlobLen),
			},
			expectedRegs: map[uint32]uint64{
				7: OK,
				8: uint64(PreimageBlobLen),
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Readable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Readable: true}},
				},
			},
			initialXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: []uint32{10, 100}, // anchor len = 2, so delete anchor and corresponding preimage blob
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
			expectedXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup:    map[string]LookupObjectForTest{}, // delete anchor successfully
							Preimage:  map[string]ByteSlice{},           // delete corresponding preimage blob successfully
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
		},
		{
			filename:        "./Forget/hostForgetOK_1_timeslots.json",
			initialGas:      InitialGas,
			expectedGas:     ExpectedGas,
			InitialTimeslot: 100,
			initialRegs: map[uint32]uint64{
				7: 32 * PageSize,
				8: uint64(PreimageBlobLen),
			},
			expectedRegs: map[uint32]uint64{
				7: OK,
				8: uint64(PreimageBlobLen),
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Readable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Readable: true}},
				},
			},
			initialXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: []uint32{10}, // anchor len = 1, so insert new anchor
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
			expectedXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: []uint32{10, 100}, // inert new anchor successfully
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
		},
		{
			filename:        "./Forget/hostForgetOK_0_timeslots.json",
			initialGas:      InitialGas,
			expectedGas:     ExpectedGas,
			InitialTimeslot: 28800 + 1000,
			initialRegs: map[uint32]uint64{
				7: 32 * PageSize,
				8: uint64(PreimageBlobLen),
			},
			expectedRegs: map[uint32]uint64{
				7: OK,
				8: uint64(PreimageBlobLen),
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Readable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Readable: true}},
				},
			},
			initialXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: []uint32{}, // anchor len = 0, so delete anchor and corresponding preimage blob
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
			expectedXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup:    map[string]LookupObjectForTest{}, // delete anchor successfully
							Preimage:  map[string]ByteSlice{},           // delete corresponding preimage blob successfully
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
		},
		{
			filename:        "./Forget/hostForgetOOB.json",
			initialGas:      InitialGas,
			expectedGas:     ExpectedGas,
			InitialTimeslot: 100,
			initialRegs: map[uint32]uint64{
				7: 32 * PageSize,
				8: uint64(PreimageBlobLen),
			},
			expectedRegs: map[uint32]uint64{
				7: OOB,
				8: uint64(PreimageBlobLen),
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Inaccessible: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Inaccessible: true}},
				},
			},
			initialXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): LookupObjectForTest{
									T: []uint32{10},
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
			expectedXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{
								PreimageBlobHash.String(): {
									T: []uint32{10},
									L: PreimageBlobLen,
								},
							},
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
		},
		{
			filename:        "./Forget/hostForgetHUH.json",
			initialGas:      InitialGas,
			expectedGas:     ExpectedGas,
			InitialTimeslot: 28800, // y >= t - D leads to HUH
			initialRegs: map[uint32]uint64{
				7: 32 * PageSize,
				8: uint64(PreimageBlobLen),
			},
			expectedRegs: map[uint32]uint64{
				7: HUH,
				8: uint64(PreimageBlobLen),
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Readable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Readable: true}},
				},
			},
			initialXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{}, // HUH error
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
			expectedXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{
						0: {
							Storage: map[string]ByteSlice{
								Service_0_StorageKeyForHost.String(): StorageValue,
							},
							Lookup: map[string]LookupObjectForTest{}, // HUH error
							Preimage: map[string]ByteSlice{
								Service_0_PreimageBLobKeyForHost.String(): PreimageBlob,
							},
							CodeHash:  PreimageBlobHash.String(),
							Balance:   Service_0_Balance,
							GasLimitG: Service_0_GasLimitG,
							GasLimitM: Service_0_GasLimitM,
						},
					},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
			},
		},
		{
			filename:    "./Yield/hostYieldOK.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7: 32 * PageSize,
			},
			expectedRegs: map[uint32]uint64{
				7: OK,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Readable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Readable: true}},
				},
			},
			initialXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
				Y: "0x0000000000000000000000000000000000000000000000000000000000000000",
			},
			expectedXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
				Y: common.HexString(PreimageBlobHashBytes),
			},
		},
		{
			filename:    "./Yield/hostYieldOOB.json",
			initialGas:  InitialGas,
			expectedGas: ExpectedGas,
			initialRegs: map[uint32]uint64{
				7: 32 * PageSize,
			},
			expectedRegs: map[uint32]uint64{
				7: OOB,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Inaccessible: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: PreimageBlobHashBytes, Access: AccessMode{Inaccessible: true}},
				},
			},
			initialXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
				Y: "0x0000000000000000000000000000000000000000000000000000000000000000",
			},
			expectedXcontent_x: &XContextForTest{
				I: 1,
				S: 0,
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{},
					I: types.Validators{},
					Q: types.AuthorizationQueue{},
					X: types.Kai_state{
						Kai_g: map[uint32]uint64{},
					},
				},
				T: []DeferredTransferForTest{},
				Y: "0x0000000000000000000000000000000000000000000000000000000000000000",
			},
		},
	}

	for _, tc := range testCases {
		filePath := filepath.Join(dirPath, tc.filename)
		dir := filepath.Dir(filePath)
		if err := os.MkdirAll(dir, os.ModePerm); err != nil {
			fmt.Printf("Failed to create directory %s: %v\n", dir, err)
			continue
		}

		var testcase AccumulateTestcase
		if err := ReadJSONFile(filePath, &testcase); err != nil {
			// If the file is not found, create a new test case
			modifiedContent, err := json.MarshalIndent(testcase, "", "  ")
			if err != nil {
				fmt.Printf("Failed to marshal modified test case: %v\n", err)
				continue
			}
			// Save to file
			outputPath := filepath.Join(dirPath, tc.filename)
			err = WriteJSONFile(outputPath, modifiedContent)
			if err != nil {
				fmt.Printf("Failed to write modified test case to file: %v\n", err)
				continue
			}
		}

		updateAccumulateTestCase(&testcase, tc.filename, tc.initialGas, tc.expectedGas, tc.initialRegs, tc.expectedRegs, tc.initialMemory, tc.expectedMemory, tc.initialXcontent_x, tc.expectedXcontent_x, tc.InitialTimeslot)

		if err := WriteJSONFile(filePath, testcase); err != nil {
			fmt.Printf("Failed to write test case %s: %v\n", tc.filename, err)
		}
	}
}

func updateAccumulateTestCase(testcase *AccumulateTestcase, name string, initialGas, expectedGas uint64, initialRegs, expectedRegs map[uint32]uint64, initialMemory, expectedMemory RAMForTest, initialXcontent_x, expectedXcontent_x *XContextForTest, initialTimeslot uint32) {
	start := strings.LastIndex(name, "/") + 1
	end := strings.LastIndex(name, ".json")

	testcase.Name = name[start:end]
	testcase.InitialGas = initialGas
	testcase.ExpectedGas = expectedGas
	testcase.InitialRegs = initialRegs
	testcase.ExpectedRegs = expectedRegs
	testcase.InitialMemory = initialMemory
	testcase.ExpectedMemory = expectedMemory
	testcase.InitialTimeslot = initialTimeslot

	if initialXcontent_x != nil {
		testcase.InitialXcontent_x = initialXcontent_x
	}
	if expectedXcontent_x != nil {
		testcase.ExpectedXcontent_x = expectedXcontent_x
	}
}
