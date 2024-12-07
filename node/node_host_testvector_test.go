package node

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/pvm"
	//"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

type ByteSlice []byte

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

type PageForTest struct {
	Start    uint32    `json:"start"`
	Contents ByteSlice `json:"contents"`
}
type RefineMForTest struct {
	P ByteSlice    `json:"P"`
	U *PageForTest `json:"U"`
	I uint32       `json:"I"`
}

type RefineM_mapForTest map[uint32]*RefineMForTest

type ServiceAccountForTest struct {
	Storage  map[string]ByteSlice `json:"s_map"`
	Lookup   map[string][]uint32  `json:"l_map"`
	Preimage map[string]ByteSlice `json:"p_map"`

	CodeHash  string `json:"code_hash"`
	Balance   uint64 `json:"balance"`
	GasLimitG uint64 `json:"min_item_gas"`
	GasLimitM uint64 `json:"min_memo_gas"`
}

type PartialStateForTest struct {
	D                  map[uint32]*ServiceAccountForTest `json:"D"`
	UpcomingValidators types.Validators                  `json:"upcoming_validators"`
	QueueWorkReport    types.AuthorizationQueue          `json:"authorizations_pool"`
	PrivilegedState    types.Kai_state                   `json:"privileged_state"`
}

type DeferredTransferForTest struct {
	SenderIndex   uint32    `json:"sender_index"`
	ReceiverIndex uint32    `json:"receiver_index"`
	Amount        uint64    `json:"amount"`
	Memo          ByteSlice `json:"memo"`
	GasLimit      uint64    `json:"gas_limit"`
}

type XContextForTest struct {
	D map[uint32]*ServiceAccountForTest `json:"D"`
	I uint32                            `json:"I"`
	S uint32                            `json:"S"`
	U *PartialStateForTest              `json:"U"`
	T []DeferredTransferForTest         `json:"T"`
}

type RefineTestcase struct {
	Name                    string                `json:"name"`
	InitalGas               uint64                `json:"initial-gas"`
	InitialRegs             []uint64              `json:"initial-regs"`
	InitialMemoryPermission []pvm.PermissionRange `json:"initial-memory-permission"`
	InitialMemory           []PageForTest         `json:"initial-memory"`

	InitialRefineM_map   RefineM_mapForTest `json:"initial-refine-map"` // m in refine function
	InitialExportSegment []ByteSlice        `json:"initial-export-segment"`

	InitialImportSegment    []ByteSlice `json:"initial-import-segment"`
	InitialExportSegmentIdx uint64      `json:"initial-export-segment-index"`

	ExpectedGas    uint64        `json:"expected-gas"`
	ExpectedRegs   []uint64      `json:"expected-regs"`
	ExpectedMemory []PageForTest `json:"expected-memory"`

	ExpectedRefineM_map   RefineM_mapForTest `json:"expected-refine-map"`
	ExpectedExportSegment []ByteSlice        `json:"expected-export-segment"`
}

type AccumulateTestcase struct {
	Name                    string                `json:"name"`
	InitalGas               uint64                `json:"initial-gas"`
	InitialRegs             []uint64              `json:"initial-regs"`
	InitialMemoryPermission []pvm.PermissionRange `json:"initial-memory-permission"`
	InitialMemory           []PageForTest         `json:"initial-memory"`

	InitialXcontent_x *XContextForTest `json:"initial-xcontent-x"`
	InitialXcontent_y XContextForTest  `json:"initial-xcontent-y"`
	InitialTimeslot   uint32           `json:"initial-timeslot"`

	ExpectedGas    uint64        `json:"expected-gas"`
	ExpectedRegs   []uint64      `json:"expected-regs"`
	ExpectedMemory []PageForTest `json:"expected-memory"`

	ExpectedXcontent_x *XContextForTest `json:"expected-xcontent-x"`
	ExpectedXcontent_y XContextForTest  `json:"expected-xcontent-y"`
}

type GeneralTestcase struct {
	Name                    string                `json:"name"`
	InitalGas               uint64                `json:"initial-gas"`
	InitialRegs             []uint64              `json:"initial-regs"`
	InitialMemoryPermission []pvm.PermissionRange `json:"initial-memory-permission"`
	InitialMemory           []PageForTest         `json:"initial-memory"`

	InitialServiceAccount ServiceAccountForTest             `json:"initial-service-account"`
	InitialServiceIndex   uint32                            `json:"initial-service-index"`
	InitialDelta          map[uint32]*ServiceAccountForTest `json:"initial-delta"`

	ExpectedGas    uint64        `json:"expected-gas"`
	ExpectedRegs   []uint64      `json:"expected-regs"`
	ExpectedMemory []PageForTest `json:"expected-memory"`

	ExpectedXServiceAccount ServiceAccountForTest `json:"expected-service-account"`
}

var errorCaseNames = map[uint64]string{
	pvm.OK:   "OK",
	pvm.OOB:  "OOB",
	pvm.NONE: "NONE",
	pvm.CASH: "CASH",
	pvm.FULL: "FULL",
	pvm.HUH:  "HUH",
	pvm.WHO:  "WHO",
	pvm.LOW:  "LOW",
	pvm.HIGH: "HIGH",
	// Define other error codes and their names as needed
}

func FindAndReadJSONFiles(dirPath, keyword string) ([]string, []string, error) {
	var matchingFiles []string
	var fileContents []string

	// Walk through the directory
	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Check if file contains the keyword and has .json extension
		if !info.IsDir() && strings.Contains(info.Name(), keyword) && filepath.Ext(info.Name()) == ".json" {
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

func SetupNodeEnv(t *testing.T) *Node {
	network := "tiny"
	epoch0Timestamp, peers, peerList, validatorSecrets, nodePaths, err := SetupQuicNetwork(network)
	if err != nil {
		t.Fatalf("Error setting up nodes: %v\n", err)
	}
	numNodes := 1
	nodes := make([]*Node, numNodes)
	for i := 0; i < numNodes; i++ {
		node, err := newNode(uint16(i), validatorSecrets[i], getGenesisFile(network), epoch0Timestamp, peers, peerList, ValidatorFlag, nodePaths[i], basePort+i)
		if err != nil {
			t.Fatalf("Failed to create node %d: %v\n", i, err)
		}
		nodes[i] = node
	}

	// give some time for nodes to come up
	for {
		time.Sleep(1 * time.Second)
		if nodes[0].statedb.GetSafrole().CheckFirstPhaseReady() {
			break
		}
	}
	return nodes[0]
}

type Testcase interface {
	GetInitialGas() uint64
	GetInitialRegs() []uint64
	GetInitialMemoryPermission() []pvm.PermissionRange
	GetInitialMemory() []PageForTest

	GetExpectedGas() uint64
	GetExpectedRegs() []uint64
	GetExpectedMemory() []PageForTest
	GetName() string
}

func (tc RefineTestcase) GetInitialGas() uint64    { return tc.InitalGas }
func (tc RefineTestcase) GetInitialRegs() []uint64 { return tc.InitialRegs }
func (tc RefineTestcase) GetInitialMemoryPermission() []pvm.PermissionRange {
	return tc.InitialMemoryPermission
}
func (tc RefineTestcase) GetInitialMemory() []PageForTest  { return tc.InitialMemory }
func (tc RefineTestcase) GetExpectedGas() uint64           { return tc.ExpectedGas }
func (tc RefineTestcase) GetExpectedRegs() []uint64        { return tc.ExpectedRegs }
func (tc RefineTestcase) GetExpectedMemory() []PageForTest { return tc.ExpectedMemory }
func (tc RefineTestcase) GetName() string                  { return tc.Name }

func (tc AccumulateTestcase) GetInitialGas() uint64    { return tc.InitalGas }
func (tc AccumulateTestcase) GetInitialRegs() []uint64 { return tc.InitialRegs }
func (tc AccumulateTestcase) GetInitialMemoryPermission() []pvm.PermissionRange {
	return tc.InitialMemoryPermission
}
func (tc AccumulateTestcase) GetInitialMemory() []PageForTest  { return tc.InitialMemory }
func (tc AccumulateTestcase) GetExpectedGas() uint64           { return tc.ExpectedGas }
func (tc AccumulateTestcase) GetExpectedRegs() []uint64        { return tc.ExpectedRegs }
func (tc AccumulateTestcase) GetExpectedMemory() []PageForTest { return tc.ExpectedMemory }
func (tc AccumulateTestcase) GetName() string                  { return tc.Name }

func (tc GeneralTestcase) GetInitialGas() uint64    { return tc.InitalGas }
func (tc GeneralTestcase) GetInitialRegs() []uint64 { return tc.InitialRegs }
func (tc GeneralTestcase) GetInitialMemoryPermission() []pvm.PermissionRange {
	return tc.InitialMemoryPermission
}
func (tc GeneralTestcase) GetInitialMemory() []PageForTest  { return tc.InitialMemory }
func (tc GeneralTestcase) GetExpectedGas() uint64           { return tc.ExpectedGas }
func (tc GeneralTestcase) GetExpectedRegs() []uint64        { return tc.ExpectedRegs }
func (tc GeneralTestcase) GetExpectedMemory() []PageForTest { return tc.ExpectedMemory }
func (tc GeneralTestcase) GetName() string                  { return tc.Name }

func InitPvmBase(vm *pvm.VM, tc Testcase) {
	vm.Gas = int64(tc.GetInitialGas())
	for i, reg := range tc.GetInitialRegs() {
		vm.WriteRegister(i, reg)
	}
	for _, perm := range tc.GetInitialMemoryPermission() {
		vm.SetAccessMode(perm.Start, perm.Length, pvm.AccessMode(perm.Mode))
	}
	for _, mem := range tc.GetInitialMemory() {
		vm.WriteRAMBytes(mem.Start, mem.Contents)
	}
}

func InitPvmRefine(vm *pvm.VM, testcase RefineTestcase) {
	// Initialize RefineM_map
	vm.RefineM_map = make(map[uint32]*pvm.RefineM)
	for k, v := range testcase.InitialRefineM_map {
		page := &pvm.Page{
			Start:    v.U.Start,
			Contents: v.U.Contents,
		}
		vm.RefineM_map[k] = &pvm.RefineM{
			P: v.P,
			U: page,
			I: v.I,
		}
	}

	// Initialize Export Segments
	vm.Exports = make([][]byte, len(testcase.InitialExportSegment))
	for i, bs := range testcase.InitialExportSegment {
		vm.Exports[i] = make([]byte, len(bs))
		copy(vm.Exports[i], bs)
	}

	// Initialize Import Segments
	vm.Imports = make([][]byte, len(testcase.InitialImportSegment))
	for i, bs := range testcase.InitialImportSegment {
		vm.Imports[i] = make([]byte, len(bs))
		copy(vm.Imports[i], bs)
	}

	vm.ExportSegmentIndex = uint32(testcase.InitialExportSegmentIdx) // check
}

func InitPvmAccumulate(vm *pvm.VM, testcase AccumulateTestcase) {
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
	vm.X = XContext
	vm.Timeslot = testcase.InitialTimeslot
}

func InitPvmGeneral(vm *pvm.VM, testcase GeneralTestcase) {
	// Initialize ServiceAccount
	sa, err := ConvertToServiceAccount(&testcase.InitialServiceAccount)
	if err != nil {
		fmt.Printf("Error converting ServiceAccount: %v\n", err)
		return
	}
	vm.ServiceAccount = sa
	// Initialize Delta map
	vm.Delta = make(map[uint32]*types.ServiceAccount)
	for k, v := range testcase.InitialDelta {
		sa, err := ConvertToServiceAccount(v)
		if err != nil {
			fmt.Printf("Error converting ServiceAccount: %v\n", err)
			return
		}
		vm.Delta[k] = sa
	}
	vm.Service_index = testcase.InitialServiceIndex
}

// Compare functions
func CompareBase(vm *pvm.VM, testcase Testcase) {
	passed := true
	// Compare Gas
	expectedGas := testcase.GetExpectedGas()
	if expectedGas != 0 && vm.Gas != int64(expectedGas) {
		fmt.Printf("Gas mismatch. Expected: %d, Got: %d\n", expectedGas, vm.Gas)
		passed = false
	}
	// Compare Registers
	expectedRegs := testcase.GetExpectedRegs()
	if len(expectedRegs) > 0 {
		vmRegs := vm.ReadRegisters()
		if len(vmRegs) != len(expectedRegs) {
			fmt.Printf("Registers length mismatch. Expected: %d, Got: %d\n", len(expectedRegs), len(vmRegs))
			passed = false
		} else {
			for i, reg := range expectedRegs {
				if vmRegs[i] != reg {
					fmt.Printf("Register[%d] mismatch. Expected: %d, Got: %d\n", i, reg, vmRegs[i])
					passed = false
				}
			}
		}
	}
	// Compare Memory
	expectedMemory := testcase.GetExpectedMemory()
	if len(expectedMemory) > 0 {
		for _, expectedMem := range expectedMemory {
			actualMemory, _ := vm.ReadRAMBytes(expectedMem.Start, len(expectedMem.Contents))
			if !equalByteSlices(actualMemory, expectedMem.Contents) {
				fmt.Printf("Memory mismatch at address %d. Expected: %v, Got: %v\n", expectedMem.Start, expectedMem.Contents, actualMemory)
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

func CompareRefine(vm *pvm.VM, testcase RefineTestcase) {
	passed := true
	RefineM_map_test := ConvertToRefineM_map(testcase.InitialRefineM_map)
	// Compare RefineM_map
	if len(testcase.ExpectedRefineM_map) > 0 {
		if !pvm.CompareRefineMMaps(vm.RefineM_map, RefineM_map_test) {
			fmt.Printf("RefineM.P mismatch. Expected: %+v, Got: %+v\n", testcase.ExpectedRefineM_map, vm.RefineM_map)
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

func CompareAccumulate(vm *pvm.VM, testcase AccumulateTestcase) {
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

func CompareGeneral(vm *pvm.VM, testcase GeneralTestcase) {
	passed := true
	// Compare ServiceAccount
	expectedSA, err := ConvertToServiceAccount(&testcase.ExpectedXServiceAccount)
	if err != nil {
		fmt.Printf("Error converting expected ServiceAccount: %v\n", err)
		return
	}
	if !reflect.DeepEqual(vm.ServiceAccount, expectedSA) {
		fmt.Printf("ServiceAccount mismatch. Expected: %+v, Got: %+v\n", expectedSA, vm.ServiceAccount)
		passed = false
	}
	// Compare Delta map (if needed)
	// for k, v := range testcase.InitialDelta {
	// 	expectedSA, err := ConvertToServiceAccount(&v)
	// 	if err != nil {
	// 		fmt.Printf("Error converting expected ServiceAccount: %v\n", err)
	// 		return
	// 	}
	// 	if !reflect.DeepEqual(vm.Delta[k], expectedSA) {
	// 		fmt.Printf("Delta[%d] mismatch. Expected: %+v, Got: %+v\n", k, expectedSA, vm.Delta[k])
	// 		passed = false
	// 	}
	// }
	if passed {
		fmt.Printf("Case %s general pass\n", testcase.GetName())
	} else {
		fmt.Printf("Case %s general fail\n", testcase.GetName())
	}
}

// Main test functions
// Test all refine test vectors
func TestRefine(t *testing.T) {
	node := SetupNodeEnv(t)

	functions := []string{
		"Import", "Export",
	}
	for _, name := range functions {
		hostidx, _ := pvm.GetHostFunctionDetails(name)
		dirPath := "../jamtestvectors/host_function"

		files, contents, err := FindAndReadJSONFiles(dirPath, name)
		if err != nil {
			fmt.Println("Error:", err)
			return
		}
		for i, content := range contents {
			targetStateDB := node.getPVMStateDB()
			vm := pvm.NewVMFortest(targetStateDB)
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
	node := SetupNodeEnv(t)

	functions := []string{"New", "Solicit", "Forget", "Transfer"}
	functions = []string{"New"}
	for _, name := range functions {
		hostidx, _ := pvm.GetHostFunctionDetails(name)
		dirPath := "../jamtestvectors/host_function"

		files, contents, err := FindAndReadJSONFiles(dirPath, name)
		if err != nil {
			fmt.Println("Error:", err)
			return
		}
		for i, content := range contents {
			targetStateDB := node.getPVMStateDB()
			vm := pvm.NewVMFortest(targetStateDB)
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
	node := SetupNodeEnv(t)

	functions := []string{
		"Read", "Write",
	}
	for _, name := range functions {
		hostidx, errcase := pvm.GetHostFunctionDetails(name)
		fmt.Printf("hostidx: %d, errcase: %v\n", hostidx, errcase)
		dirPath := "../jamtestvectors/host_function"

		files, contents, err := FindAndReadJSONFiles(dirPath, name)
		if err != nil {
			fmt.Println("Error:", err)
			return
		}
		for i, content := range contents {
			targetStateDB := node.getPVMStateDB()
			vm := pvm.NewVMFortest(targetStateDB)
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

// Generate test vectors
func GenerateTestVectors(t *testing.T, dirPath string, functions []string, errorCases map[string][]uint64, templateFileName string, testCaseType string) {
	// Read the template
	templatePath := filepath.Join(dirPath, templateFileName)
	templateContent, err := os.ReadFile(templatePath)
	if err != nil {
		fmt.Printf("Failed to read template file: %v\n", err)
		return
	}
	for _, name := range functions {
		hostidx, _ := pvm.GetHostFunctionDetails(name)
		fmt.Printf("hostidx: %d\n", hostidx)

		// Get error cases for the function
		errCaseList, exists := errorCases[name]
		if !exists {
			fmt.Printf("No error cases defined for function %s\n", name)
			continue
		}
		for _, errcase := range errCaseList {
			fmt.Printf("Processing function: %s, errcase: %d\n", name, errcase)
			// Depending on testCaseType, unmarshal into appropriate struct
			var testcase interface{}
			switch testCaseType {
			case "Refine":
				testcase = &RefineTestcase{}
			case "Accumulate":
				testcase = &AccumulateTestcase{}
			case "General":
				testcase = &GeneralTestcase{}
			default:
				fmt.Printf("Unknown testCaseType: %s\n", testCaseType)
				continue
			}
			err := json.Unmarshal(templateContent, testcase)
			if err != nil {
				fmt.Printf("Failed to parse JSON template: %v\n", err)
				continue
			}
			// Modify the test case
			switch tc := testcase.(type) {
			case *RefineTestcase:
				tc.ExpectedRegs[7] = errcase
				// If error case is OOB, modify initial-memory-permission mode to 0
				if errcase == pvm.OOB {
					for i := range tc.InitialMemoryPermission {
						tc.InitialMemoryPermission[i].Mode = 0
					}
				}
				// Update test case name
				errcaseName, exists := errorCaseNames[errcase]
				if !exists {
					fmt.Printf("Unknown error case code: %d\n", errcase)
					continue
				}
				tc.Name = fmt.Sprintf("host%s%s", name, errcaseName)
			case *AccumulateTestcase:
				tc.ExpectedRegs[7] = errcase
				if errcase == pvm.OOB {
					for i := range tc.InitialMemoryPermission {
						tc.InitialMemoryPermission[i].Mode = 0
					}
				}
				errcaseName, exists := errorCaseNames[errcase]
				if !exists {
					fmt.Printf("Unknown error case code: %d\n", errcase)
					continue
				}
				tc.Name = fmt.Sprintf("host%s%s", name, errcaseName)
			case *GeneralTestcase:
				tc.ExpectedRegs[7] = errcase
				if errcase == pvm.OOB {
					for i := range tc.InitialMemoryPermission {
						tc.InitialMemoryPermission[i].Mode = 0
					}
				}
				errcaseName, exists := errorCaseNames[errcase]
				if !exists {
					fmt.Printf("Unknown error case code: %d\n", errcase)
					continue
				}
				tc.Name = fmt.Sprintf("host%s%s", name, errcaseName)
			default:
				fmt.Printf("Unknown test case type\n")
				continue
			}
			// Marshal back to JSON
			modifiedContent, err := json.MarshalIndent(testcase, "", "  ")
			if err != nil {
				fmt.Printf("Failed to marshal modified test case: %v\n", err)
				continue
			}
			// Save to file
			errcaseName, _ := errorCaseNames[errcase]
			filename := fmt.Sprintf("host%s%s.json", name, errcaseName)
			outputPath := filepath.Join(dirPath, filename)
			err = os.WriteFile(outputPath, modifiedContent, 0644)
			if err != nil {
				fmt.Printf("Failed to write modified test case to file: %v\n", err)
				continue
			}
			fmt.Printf("Generated test vector: %s\n", outputPath)
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

func TestGenerateRefineTestVectors(t *testing.T) {
	dirPath := "../jamtestvectors/host_function"
	functions := []string{"Import", "Export"}
	errorCases := map[string][]uint64{
		"Import": {pvm.OK, pvm.OOB, pvm.NONE},
		"Export": {pvm.OK, pvm.OOB, pvm.FULL},
	}
	templateFileName := "./Templets/hostRefineTemplet.json"
	testCaseType := "Refine"
	GenerateTestVectors(t, dirPath, functions, errorCases, templateFileName, testCaseType)

	var (
		testcase RefineTestcase
		filename string
		filePath string
		err      error
	)

	// Modify hostImportOK.json here
	// Clear the test case
	testcase = RefineTestcase{}
	filename = "hostImportOK.json"
	filePath = filepath.Join(dirPath, filename)

	err = ReadJSONFile(filePath, &testcase)
	if err != nil {
		fmt.Printf("Failed to read test case: %v\n", err)
		return
	}

	testcase.InitialRegs[7] = 0 // the 0th import segment
	testcase.ExpectedRegs[7] = pvm.OK

	testcase.InitialRegs[8] = 4278124544 // 0xFEFF0000
	testcase.ExpectedRegs[8] = 4278124544

	testcase.InitialRegs[9] = 12 // segement length
	testcase.ExpectedRegs[9] = 12

	testcase.InitialMemoryPermission = []pvm.PermissionRange{
		{Start: 4278124544, Length: 12, Mode: 2},
	}

	testcase.InitialMemory = []PageForTest{}
	testcase.ExpectedMemory = []PageForTest{
		{Start: 4278124544, Contents: ByteSlice{10, 10, 11, 11, 12, 12, 13, 13, 14, 14, 15, 15}},
	}

	testcase.InitialRefineM_map = RefineM_mapForTest{}
	testcase.ExpectedRefineM_map = RefineM_mapForTest{}

	testcase.InitialImportSegment = []ByteSlice{
		{10, 10, 11, 11, 12, 12, 13, 13, 14, 14, 15, 15},
	}

	err = WriteJSONFile(filePath, testcase)
	if err != nil {
		fmt.Printf("Failed to write test case: %v\n", err)
		return
	}

	// Modify hostImportOOB.json here
	// Clear the test case
	testcase = RefineTestcase{}
	filename = "hostImportOOB.json"
	filePath = filepath.Join(dirPath, filename)

	err = ReadJSONFile(filePath, &testcase)
	if err != nil {
		fmt.Printf("Failed to read test case: %v\n", err)
		return
	}

	testcase.InitialRegs[7] = 0 // the 0th import segment
	testcase.ExpectedRegs[7] = pvm.OOB

	testcase.InitialRegs[8] = 4278124544 // 0xFEFF0000
	testcase.ExpectedRegs[8] = 4278124544

	testcase.InitialRegs[9] = 12 // segement length
	testcase.ExpectedRegs[9] = 12

	testcase.InitialMemoryPermission = []pvm.PermissionRange{
		{Start: 4278124544, Length: 12, Mode: 0},
	} // set mode to 0 to simulate OOB error

	testcase.InitialMemory = []PageForTest{}
	testcase.ExpectedMemory = []PageForTest{}

	testcase.InitialRefineM_map = RefineM_mapForTest{}
	testcase.ExpectedRefineM_map = RefineM_mapForTest{}

	testcase.InitialImportSegment = []ByteSlice{
		{10, 10, 11, 11, 12, 12, 13, 13, 14, 14, 15, 15},
	}

	err = WriteJSONFile(filePath, testcase)
	if err != nil {
		fmt.Printf("Failed to write test case: %v\n", err)
		return
	}

	// Modify hostImportNONE.json here
	// Clear the test case
	testcase = RefineTestcase{}
	filename = "hostImportNONE.json"
	filePath = filepath.Join(dirPath, filename)

	err = ReadJSONFile(filePath, &testcase)
	if err != nil {
		fmt.Printf("Failed to read test case: %v\n", err)
		return
	}

	testcase.InitialRegs[7] = 9999 // Simulate NONE error
	testcase.ExpectedRegs[7] = pvm.NONE

	testcase.InitialRegs[8] = 4278124544 // 0xFEFF0000
	testcase.ExpectedRegs[8] = 4278124544

	testcase.InitialRegs[9] = 12 // segement length
	testcase.ExpectedRegs[9] = 12

	testcase.InitialMemoryPermission = []pvm.PermissionRange{
		{Start: 4278124544, Length: 12, Mode: 2},
	}
	testcase.InitialMemory = []PageForTest{}
	testcase.ExpectedMemory = []PageForTest{}

	testcase.InitialRefineM_map = RefineM_mapForTest{}
	testcase.ExpectedRefineM_map = RefineM_mapForTest{}

	testcase.InitialImportSegment = []ByteSlice{
		{10, 10, 11, 11, 12, 12, 13, 13, 14, 14, 15, 15},
	}

	err = WriteJSONFile(filePath, testcase)
	if err != nil {
		fmt.Printf("Failed to write test case: %v\n", err)
		return
	}

	// Modify hostExportOK.json here
	// Clear the test case
	testcase = RefineTestcase{}
	filename = "hostExportOK.json"
	filePath = filepath.Join(dirPath, filename)

	err = ReadJSONFile(filePath, &testcase)
	if err != nil {
		fmt.Printf("Failed to read test case: %v\n", err)
		return
	}

	testcase.InitialRegs[7] = 4278124544                                                                     // 0xFEFF0000
	testcase.ExpectedRegs[7] = testcase.InitialExportSegmentIdx + uint64(len(testcase.InitialExportSegment)) // 0 + 1

	testcase.InitialRegs[8] = 12 // Export segment length
	testcase.ExpectedRegs[8] = 12

	testcase.InitialMemoryPermission = []pvm.PermissionRange{
		{Start: 4278124544, Length: 12, Mode: 2},
	}

	testcase.InitialMemory = []PageForTest{
		{Start: 4278124544, Contents: ByteSlice{10, 10, 11, 11, 12, 12, 13, 13, 14, 14, 15, 15}},
	}
	testcase.ExpectedMemory = []PageForTest{
		{Start: 4278124544, Contents: ByteSlice{10, 10, 11, 11, 12, 12, 13, 13, 14, 14, 15, 15}},
	}

	testcase.InitialRefineM_map = RefineM_mapForTest{}
	testcase.ExpectedRefineM_map = RefineM_mapForTest{}

	testcase.InitialExportSegment = []ByteSlice{
		{},
	}
	testcase.ExpectedExportSegment = []ByteSlice{
		{}, {10, 10, 11, 11, 12, 12, 13, 13, 14, 14, 15, 15, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
	}
	testcase.InitialExportSegmentIdx = 0

	err = WriteJSONFile(filePath, testcase)
	if err != nil {
		fmt.Printf("Failed to write test case: %v\n", err)
		return
	}

	// Modify hostExportOOB.json here
	// Clear the test case
	testcase = RefineTestcase{}
	filename = "hostExportOOB.json"
	filePath = filepath.Join(dirPath, filename)

	err = ReadJSONFile(filePath, &testcase)
	if err != nil {
		fmt.Printf("Failed to read test case: %v\n", err)
		return
	}

	testcase.InitialRegs[7] = 4278124544 // 0xFEFF0000
	testcase.ExpectedRegs[7] = pvm.OOB

	testcase.InitialRegs[8] = 12 // Export segment length
	testcase.ExpectedRegs[8] = 12

	testcase.InitialMemoryPermission = []pvm.PermissionRange{
		{Start: 4278124544, Length: 12, Mode: 0},
	} // set mode to 0 to simulate OOB error

	testcase.InitialMemory = []PageForTest{}
	testcase.ExpectedMemory = []PageForTest{}

	testcase.InitialRefineM_map = RefineM_mapForTest{}
	testcase.ExpectedRefineM_map = RefineM_mapForTest{}

	testcase.InitialExportSegment = []ByteSlice{{}}
	testcase.ExpectedExportSegment = []ByteSlice{{}}
	testcase.InitialExportSegmentIdx = 0

	err = WriteJSONFile(filePath, testcase)
	if err != nil {
		fmt.Printf("Failed to write test case: %v\n", err)
		return
	}

	// Modify hostExportFULL.json here
	// Clear the test case
	testcase = RefineTestcase{}
	filename = "hostExportFULL.json"
	filePath = filepath.Join(dirPath, filename)

	err = ReadJSONFile(filePath, &testcase)
	if err != nil {
		fmt.Printf("Failed to read test case: %v\n", err)
		return
	}

	testcase.InitialRegs[7] = 4278124544 // 0xFEFF0000
	testcase.ExpectedRegs[7] = pvm.FULL

	testcase.InitialRegs[8] = 12 // Export segment length
	testcase.ExpectedRegs[8] = 12

	testcase.InitialMemoryPermission = []pvm.PermissionRange{
		{Start: 4278124544, Length: 12, Mode: 2},
	}

	testcase.InitialMemory = []PageForTest{
		{Start: 4278124544, Contents: ByteSlice{10, 10, 11, 11, 12, 12, 13, 13, 14, 14, 15, 15}},
	}
	testcase.ExpectedMemory = []PageForTest{
		{Start: 4278124544, Contents: ByteSlice{10, 10, 11, 11, 12, 12, 13, 13, 14, 14, 15, 15}},
	}

	testcase.InitialRefineM_map = RefineM_mapForTest{}
	testcase.ExpectedRefineM_map = RefineM_mapForTest{}

	testcase.InitialExportSegment = []ByteSlice{{}}
	testcase.ExpectedExportSegment = []ByteSlice{{}}
	testcase.InitialExportSegmentIdx = 9999 // Simulate FULL error

	err = WriteJSONFile(filePath, testcase)
	if err != nil {
		fmt.Printf("Failed to write test case: %v\n", err)
		return
	}
}

// Generate Accumulate test vectors
func TestGenerateAccumulateTestVectors(t *testing.T) {
	dirPath := "../jamtestvectors/host_function"
	functions := []string{"New"}
	errorCases := map[string][]uint64{
		"New":      {pvm.OK, pvm.OOB, pvm.CASH},
		"Solicit":  {pvm.OK, pvm.OOB, pvm.FULL, pvm.HUH},
		"Forget":   {pvm.OK, pvm.OOB, pvm.HUH},
		"Transfer": {pvm.OK, pvm.WHO, pvm.CASH, pvm.LOW, pvm.HIGH},
	}
	templateFileName := "./Templets/hostAccumulateTemplet.json"
	testCaseType := "Accumulate"
	GenerateTestVectors(t, dirPath, functions, errorCases, templateFileName, testCaseType)

	var (
		testcase AccumulateTestcase
		filename string
		filePath string
		err      error
		s        *ServiceAccountForTest
		a        *ServiceAccountForTest
		s_edited *ServiceAccountForTest
	)

	// Modify hostNewOK.json here
	// Clear the test case
	testcase = AccumulateTestcase{}
	filename = "hostNewOK.json"
	filePath = filepath.Join(dirPath, filename)
	err = ReadJSONFile(filePath, &testcase)
	if err != nil {
		fmt.Printf("Failed to read test case: %v\n", err)
		return
	}
	testcase.InitialRegs[7] = 4278124544 // 0xFEFF0000
	testcase.ExpectedRegs[7] = 1         // new service index (should be check)

	testcase.InitialRegs[8] = 12 // code length (should be check)
	testcase.ExpectedRegs[8] = 12

	testcase.InitialRegs[9] = 100 // Gas limit g
	testcase.ExpectedRegs[9] = 100

	testcase.InitialRegs[10] = 200 // Gas limit m
	testcase.ExpectedRegs[10] = 200

	testcase.InitialMemoryPermission = []pvm.PermissionRange{
		{Start: 4278124544, Length: 32, Mode: 2},
	}

	testcase.InitialMemory = []PageForTest{
		{Start: 4278124544, Contents: ByteSlice{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
	}
	testcase.ExpectedMemory = []PageForTest{
		{Start: 4278124544, Contents: ByteSlice{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
	}

	a = &ServiceAccountForTest{}
	a.Storage = map[string]ByteSlice{}
	a.Preimage = map[string]ByteSlice{}
	a.Lookup = map[string][]uint32{
		"0x0000000000000000000000000000000000000000000000000000000000000000": {},
	} // E4(l) ⌢ H(h)2...30
	a.CodeHash = "0x0000000000000000000000000000000000000000000000000000000000000000"
	a.Balance = 213
	a.GasLimitG = 100
	a.GasLimitM = 200

	s = testcase.InitialXcontent_x.U.D[0]

	testcase.ExpectedXcontent_x = &XContextForTest{
		D: make(map[uint32]*ServiceAccountForTest),
		I: 555,
		S: 0,
		U: &PartialStateForTest{
			D: make(map[uint32]*ServiceAccountForTest),
		},
		T: []DeferredTransferForTest{},
	}
	s_edited = DeepCopyServiceAccount(s)
	testcase.ExpectedXcontent_x.U.D[1] = a
	testcase.ExpectedXcontent_x.U.D[0] = s_edited
	testcase.ExpectedXcontent_x.U.D[0].Balance -= a.Balance

	err = WriteJSONFile(filePath, testcase)
	if err != nil {
		fmt.Printf("Failed to write test case: %v\n", err)
		return
	}

	// Modify hostNewOOB.json here
	// Clear the test case
	testcase = AccumulateTestcase{}
	filename = "hostNewOOB.json"
	filePath = filepath.Join(dirPath, filename)
	err = ReadJSONFile(filePath, &testcase)
	if err != nil {
		fmt.Printf("Failed to read test case: %v\n", err)
		return
	}
	testcase.InitialRegs[7] = 4278124544 // 0xFEFF0000
	testcase.ExpectedRegs[7] = pvm.OOB   // new service index (should be check)

	testcase.InitialRegs[8] = 12 // code length (should be check)
	testcase.ExpectedRegs[8] = 12

	testcase.InitialRegs[9] = 100 // Gas limit g
	testcase.ExpectedRegs[9] = 100

	testcase.InitialRegs[10] = 200 // Gas limit m
	testcase.ExpectedRegs[10] = 200

	testcase.InitialMemoryPermission = []pvm.PermissionRange{
		{Start: 4278124544, Length: 32, Mode: 0}, // set mode to 0 to simulate OOB error
	}

	testcase.InitialMemory = []PageForTest{}
	testcase.ExpectedMemory = []PageForTest{}

	testcase.ExpectedXcontent_x = testcase.InitialXcontent_x

	err = WriteJSONFile(filePath, testcase)
	if err != nil {
		fmt.Printf("Failed to write test case: %v\n", err)
		return
	}

	// Modify hostNewCASH.json here
	// Clear the test case
	testcase = AccumulateTestcase{}
	filename = "hostNewCASH.json"
	filePath = filepath.Join(dirPath, filename)
	err = ReadJSONFile(filePath, &testcase)
	if err != nil {
		fmt.Printf("Failed to read test case: %v\n", err)
		return
	}
	testcase.InitialRegs[7] = 4278124544 // 0xFEFF0000
	testcase.ExpectedRegs[7] = pvm.CASH  // new service index (should be check)

	testcase.InitialRegs[8] = 12 // code length (should be check)
	testcase.ExpectedRegs[8] = 12

	testcase.InitialRegs[9] = 100 // Gas limit g
	testcase.ExpectedRegs[9] = 100

	testcase.InitialRegs[10] = 200 // Gas limit m
	testcase.ExpectedRegs[10] = 200

	testcase.InitialMemoryPermission = []pvm.PermissionRange{
		{Start: 4278124544, Length: 32, Mode: 2},
	}

	testcase.InitialMemory = []PageForTest{
		{Start: 4278124544, Contents: ByteSlice{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
	}
	testcase.ExpectedMemory = []PageForTest{
		{Start: 4278124544, Contents: ByteSlice{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
	}

	a = &ServiceAccountForTest{}
	a.Storage = map[string]ByteSlice{}
	a.Preimage = map[string]ByteSlice{}
	a.Lookup = map[string][]uint32{
		"0x0000000000000000000000000000000000000000000000000000000000000000": {},
	} // E4(l) ⌢ H(h)2...30
	a.CodeHash = "0x0000000000000000000000000000000000000000000000000000000000000000"
	a.Balance = 213
	a.GasLimitG = 100
	a.GasLimitM = 200

	testcase.InitialXcontent_x.U.D[0].Balance = 300 // Simulate CASH error

	testcase.ExpectedXcontent_x = DeepCopyXContextForTest(testcase.InitialXcontent_x)

	testcase.ExpectedXcontent_x.U.D[0].Balance = 300 - 213 // Simulate CASH error

	err = WriteJSONFile(filePath, testcase)
	if err != nil {
		fmt.Printf("Failed to write test case: %v\n", err)
		return
	}
}

// Generate General test vectors
func TestGenerateGeneralTestVectors(t *testing.T) {
	dirPath := "../jamtestvectors/host_function"
	functions := []string{"Read", "Write"}
	errorCases := map[string][]uint64{
		"Read":  {pvm.OK, pvm.OOB, pvm.NONE},
		"Write": {pvm.OK, pvm.OOB, pvm.FULL, pvm.OOB},
	}
	templateFileName := "./Templets/hostGeneralTemplet.json"
	testCaseType := "General"
	GenerateTestVectors(t, dirPath, functions, errorCases, templateFileName, testCaseType)
}

// some useful functions
func equal2DByte(a, b [][]byte) bool {
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

func CompareRAMs(ram1, ram2 *pvm.RAM) bool {
	// Handle nil cases
	if ram1 == nil && ram2 == nil {
		return true // Both are nil, considered equal
	}
	if ram1 == nil || ram2 == nil {
		return false // One is nil, the other is not
	}

	// Compare memory maps
	if !reflect.DeepEqual(ram1.Memory, ram2.Memory) {
		return false
	}

	// Compare permissions slices
	if !reflect.DeepEqual(ram1.Permissions, ram2.Permissions) {
		return false
	}

	return true
}

// Convert XContextForTest to types.XContext
func ConvertToXContext(xcft *XContextForTest) (*types.XContext, error) {
	xc := &types.XContext{
		D: make(map[uint32]*types.ServiceAccount),
		I: xcft.I,
		S: xcft.S,
		T: []types.DeferredTransfer{},
		U: nil,
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

	// Convert D map
	for k, v := range xcft.D {
		sa, err := ConvertToServiceAccount(v)
		if err != nil {
			return nil, err
		}
		xc.D[k] = sa
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
		D: make(map[uint32]*ServiceAccountForTest),
		I: xc.I,
		S: xc.S,
		T: []DeferredTransferForTest{},
		U: nil,
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

	// Convert D map
	for k, v := range xc.D {
		saft := ConvertToServiceAccountForTest(v)
		xcft.D[k] = saft
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
		UpcomingValidators: psft.UpcomingValidators,
		QueueWorkReport:    psft.QueueWorkReport,
		PrivilegedState:    psft.PrivilegedState,
	}

	// Convert D map
	for k, v := range psft.D {
		sa, err := ConvertToServiceAccount(v)
		if err != nil {
			return nil, err
		}
		ps.D[k] = sa
	}

	return ps, nil
}

// Convert types.PartialState to PartialStateForTest
func ConvertToPartialStateForTest(ps *types.PartialState) *PartialStateForTest {
	psft := &PartialStateForTest{
		D:                  make(map[uint32]*ServiceAccountForTest),
		UpcomingValidators: ps.UpcomingValidators,
		QueueWorkReport:    ps.QueueWorkReport,
		PrivilegedState:    ps.PrivilegedState,
	}

	// Convert D map
	for k, v := range ps.D {
		saft := ConvertToServiceAccountForTest(v)
		psft.D[k] = saft
	}

	return psft
}

// Convert ServiceAccountForTest to ServiceAccount
func ConvertToServiceAccount(saft *ServiceAccountForTest) (*types.ServiceAccount, error) {
	sa := &types.ServiceAccount{
		CodeHash:  common.HexToHash(saft.CodeHash),
		Balance:   saft.Balance,
		GasLimitG: saft.GasLimitG,
		GasLimitM: saft.GasLimitM,
		Dirty:     false,
		Storage:   make(map[common.Hash]types.StorageObject),
		Lookup:    make(map[common.Hash]types.LookupObject),
		Preimage:  make(map[common.Hash]types.PreimageObject),
	}

	// Convert Storage map
	for k, v := range saft.Storage {
		keyHash := common.HexToHash(k)
		sa.Storage[keyHash] = types.StorageObject{Value: v}
	}

	// Convert Lookup map
	for k, v := range saft.Lookup {
		keyHash := common.HexToHash(k)
		sa.Lookup[keyHash] = types.LookupObject{T: v} // check
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
		Lookup:    make(map[string][]uint32),
		Preimage:  make(map[string]ByteSlice),
	}

	// Convert Storage map
	for k, v := range sa.Storage {
		keyStr := k.Hex()
		saft.Storage[keyStr] = v.Value
	}

	// Convert Lookup map
	for k, v := range sa.Lookup {
		keyStr := k.Hex()
		saft.Lookup[keyStr] = v.T
	}

	// Convert Preimage map
	for k, v := range sa.Preimage {
		keyStr := k.Hex()
		saft.Preimage[keyStr] = v.Preimage
	}

	return saft
}

func ConvertToRefineM_map(refineM_mapFT map[uint32]*RefineMForTest) map[uint32]*pvm.RefineM {
	refineM_map := make(map[uint32]*pvm.RefineM)
	for k, v := range refineM_mapFT {
		page := &pvm.Page{
			Start:    v.U.Start,
			Contents: v.U.Contents,
		}
		refineM_map[k] = &pvm.RefineM{
			P: v.P,
			U: page,
			I: v.I,
		}
	}
	return refineM_map
}

func ConvertToRefineM_mapForTest(refineM_map map[uint32]*pvm.RefineM) map[uint32]*RefineMForTest {
	refineM_mapFT := make(map[uint32]*RefineMForTest)
	for k, v := range refineM_map {
		page := &PageForTest{
			Start:    v.U.Start,
			Contents: v.U.Contents,
		}
		refineM_mapFT[k] = &RefineMForTest{
			P: v.P,
			U: page,
			I: v.I,
		}
	}
	return refineM_mapFT
}

func DeepCopyServiceAccount(sa *ServiceAccountForTest) *ServiceAccountForTest {
	newSA := &ServiceAccountForTest{
		Storage:   make(map[string]ByteSlice),
		Preimage:  make(map[string]ByteSlice),
		Lookup:    make(map[string][]uint32),
		CodeHash:  sa.CodeHash,
		Balance:   sa.Balance,
		GasLimitG: sa.GasLimitG,
		GasLimitM: sa.GasLimitM,
	}
	for k, v := range sa.Storage {
		newSA.Storage[k] = append([]byte(nil), v...)
	}
	for k, v := range sa.Preimage {
		newSA.Preimage[k] = append([]byte(nil), v...)
	}
	for k, v := range sa.Lookup {
		newSA.Lookup[k] = append([]uint32(nil), v...)
	}
	return newSA
}

func DeepCopyPartialStateForTest(psft *PartialStateForTest) *PartialStateForTest {
	newPSFT := &PartialStateForTest{
		D:                  make(map[uint32]*ServiceAccountForTest),
		UpcomingValidators: psft.UpcomingValidators,
		QueueWorkReport:    psft.QueueWorkReport,
		PrivilegedState:    psft.PrivilegedState,
	}
	for k, v := range psft.D {
		newPSFT.D[k] = DeepCopyServiceAccount(v)
	}
	return newPSFT
}

func DeepCopyXContextForTest(xcft *XContextForTest) *XContextForTest {
	newXCFT := &XContextForTest{
		D: make(map[uint32]*ServiceAccountForTest),
		T: make([]DeferredTransferForTest, len(xcft.T)),
		I: xcft.I,
		S: xcft.S,
		U: nil,
	}
	for k, v := range xcft.D {
		newXCFT.D[k] = DeepCopyServiceAccount(v)
	}
	for i, v := range xcft.T {
		newXCFT.T[i] = DeferredTransferForTest{
			SenderIndex:   v.SenderIndex,
			ReceiverIndex: v.ReceiverIndex,
			Amount:        v.Amount,
			Memo:          append([]byte(nil), v.Memo...),
			GasLimit:      v.GasLimit,
		}
	}
	if xcft.U != nil {
		newXCFT.U = DeepCopyPartialStateForTest(xcft.U)
	}
	return newXCFT
}

func ComputeStorageKey(s uint32, key []byte) {
	h := common.Compute_storageKey_internal(s, key)
	account_storage_key := common.ComputeC_sh(s, h)
	fmt.Printf("account_storage_key: %v\n", account_storage_key.Bytes())
}

func ComputePreimageBlobKey(s uint32, blob_hash common.Hash) {
	ap_internal_key := common.Compute_preimageBlob_internal(blob_hash)
	account_preimage_hash := common.ComputeC_sh(s, ap_internal_key)
	fmt.Printf("account_preimage_hash: %v\n", account_preimage_hash)
}

func ComputePreimageLookupKey(s uint32, blob_hash common.Hash, blob_len uint32) {
	al_internal_key := common.Compute_preimageLookup_internal(blob_hash, blob_len)
	account_lookuphash := common.ComputeC_sh(s, al_internal_key)
	fmt.Printf("account_lookuphash: %v\n", account_lookuphash)
}

func TestComputePreimageLookupKey(t *testing.T) {
	al_internal_key := common.Compute_preimageLookup_internal(common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000000"), 12)
	account_lookuphash := common.ComputeC_sh(1, al_internal_key)
	fmt.Printf("account_lookuphash: %v\n", account_lookuphash)
}
