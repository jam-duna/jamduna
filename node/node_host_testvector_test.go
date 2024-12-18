//go:build network
// +build network

package node

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/pvm"
	"github.com/colorfulnotion/jam/types"
)

func SetUpNode() (*Node, error) {
	nodes, err := SetUpNodes(1)
	if err != nil {
		panic(1)
	}
	return nodes[0], err

}

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

type RAMForTest struct {
	Pages map[uint32]*PageForTest `json:"pages"` // The pages in the RAM
}
type ServiceAccountForTest struct {
	Storage  map[string]ByteSlice `json:"s_map"`
	Lookup   map[string][]uint32  `json:"l_map"`
	Preimage map[string]ByteSlice `json:"p_map"`

	CodeHash  string `json:"code_hash"`
	Balance   uint64 `json:"balance"`
	GasLimitG uint64 `json:"min_item_gas"`
	GasLimitM uint64 `json:"min_memo_gas"`
}

type PageForTest struct {
	Value  ByteSlice      `json:"value"`  // The data stored in the page
	Access pvm.AccessMode `json:"access"` // The access mode of the page
}

type RefineMForTest struct {
	P ByteSlice   `json:"P"`
	U *RAMForTest `json:"U"`
	I uint32      `json:"I"`
}

type RefineM_mapForTest map[uint32]*RefineMForTest

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
	Name          string            `json:"name"`
	InitialGas    uint64            `json:"initial-gas"`
	InitialRegs   map[uint32]uint64 `json:"initial-regs"`
	InitialMemory RAMForTest        `json:"initial-memory"`

	InitialRefineM_map   RefineM_mapForTest `json:"initial-refine-map"` // m in refine function
	InitialExportSegment []ByteSlice        `json:"initial-export-segment"`

	InitialImportSegment    []ByteSlice `json:"initial-import-segment"`
	InitialExportSegmentIdx uint32      `json:"initial-export-segment-index"`

	ExpectedGas    uint64            `json:"expected-gas"`
	ExpectedRegs   map[uint32]uint64 `json:"expected-regs"`
	ExpectedMemory RAMForTest        `json:"expected-memory"`

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

	ExpectedGas    uint64            `json:"expected-gas"`
	ExpectedRegs   map[uint32]uint64 `json:"expected-regs"`
	ExpectedMemory RAMForTest        `json:"expected-memory"`

	ExpectedXcontent_x *XContextForTest `json:"expected-xcontent-x"`
	ExpectedXcontent_y XContextForTest  `json:"expected-xcontent-y"`
}

type GeneralTestcase struct {
	Name          string            `json:"name"`
	InitialGas    uint64            `json:"initial-gas"`
	InitialRegs   map[uint32]uint64 `json:"initial-regs"`
	InitialMemory RAMForTest        `json:"initial-memory"`

	InitialServiceAccount ServiceAccountForTest             `json:"initial-service-account"`
	InitialServiceIndex   uint32                            `json:"initial-service-index"`
	InitialDelta          map[uint32]*ServiceAccountForTest `json:"initial-delta"`

	ExpectedGas    uint64            `json:"expected-gas"`
	ExpectedRegs   map[uint32]uint64 `json:"expected-regs"`
	ExpectedMemory RAMForTest        `json:"expected-memory"`

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
	fmt.Printf("FindAndReadJSONFiles %s\n", dirPath)
	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Check if file contains the keyword and has .json extension
		if !info.IsDir() && strings.Contains(info.Name(), keyword) && filepath.Ext(info.Name()) == ".json" {
			fmt.Printf("%s:%s\n", keyword, info.Name())
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
	GetExpectedRegs() map[uint32]uint64
	GetExpectedMemory() RAMForTest
	GetName() string
}

func (tc RefineTestcase) GetInitialGas() uint64              { return tc.InitialGas }
func (tc RefineTestcase) GetInitialRegs() map[uint32]uint64  { return tc.InitialRegs }
func (tc RefineTestcase) GetInitialMemory() RAMForTest       { return tc.InitialMemory }
func (tc RefineTestcase) GetExpectedGas() uint64             { return tc.ExpectedGas }
func (tc RefineTestcase) GetExpectedRegs() map[uint32]uint64 { return tc.ExpectedRegs }
func (tc RefineTestcase) GetExpectedMemory() RAMForTest      { return tc.ExpectedMemory }
func (tc RefineTestcase) GetName() string                    { return tc.Name }

func (tc AccumulateTestcase) GetInitialGas() uint64              { return tc.InitialGas }
func (tc AccumulateTestcase) GetInitialRegs() map[uint32]uint64  { return tc.InitialRegs }
func (tc AccumulateTestcase) GetInitialMemory() RAMForTest       { return tc.InitialMemory }
func (tc AccumulateTestcase) GetExpectedGas() uint64             { return tc.ExpectedGas }
func (tc AccumulateTestcase) GetExpectedRegs() map[uint32]uint64 { return tc.ExpectedRegs }
func (tc AccumulateTestcase) GetExpectedMemory() RAMForTest      { return tc.ExpectedMemory }
func (tc AccumulateTestcase) GetName() string                    { return tc.Name }

func (tc GeneralTestcase) GetInitialGas() uint64              { return tc.InitialGas }
func (tc GeneralTestcase) GetInitialRegs() map[uint32]uint64  { return tc.InitialRegs }
func (tc GeneralTestcase) GetInitialMemory() RAMForTest       { return tc.InitialMemory }
func (tc GeneralTestcase) GetExpectedGas() uint64             { return tc.ExpectedGas }
func (tc GeneralTestcase) GetExpectedRegs() map[uint32]uint64 { return tc.ExpectedRegs }
func (tc GeneralTestcase) GetExpectedMemory() RAMForTest      { return tc.ExpectedMemory }
func (tc GeneralTestcase) GetName() string                    { return tc.Name }

func InitPvmBase(vm *pvm.VM, tc Testcase) {
	vm.Gas = int64(tc.GetInitialGas())
	for i, reg := range tc.GetInitialRegs() {
		vm.WriteRegister(int(i), reg)
	}
	for page_addr, page := range tc.GetInitialMemory().Pages {
		vm.Ram.SetPageAccess(page_addr, 1, page.Access)
		vm.Ram.WriteRAMBytes(page_addr*pvm.PageSize, page.Value)
	}
}

func InitPvmRefine(vm *pvm.VM, testcase RefineTestcase) {
	// Initialize RefineM_map
	vm.RefineM_map = make(map[uint32]*pvm.RefineM)
	vm.RefineM_map = ConvertToRefineM_map(testcase.InitialRefineM_map)

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

	vm.ExportSegmentIndex = testcase.InitialExportSegmentIdx
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
	expectedMemory := testcase.GetExpectedMemory().Pages
	if len(expectedMemory) > 0 {
		for page_addr, page := range expectedMemory {
			actualMemory, _ := vm.Ram.ReadRAMBytes(page_addr*pvm.PageSize, uint32(len(page.Value)))
			if !equalByteSlices(actualMemory, page.Value) {
				fmt.Printf("Memory mismatch at address %d. Expected: %v, Got: %v\n", page_addr*pvm.PageSize, page.Value, actualMemory)
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
		if !CompareRefineMMaps(vm.RefineM_map, RefineM_map_test) {
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
	node, err := SetUpNode()
	if err != nil {
		panic("Error setting up nodes: %v\n")
	}

	functions := []string{
		"Historical_lookup", // William
		"Import",            // William
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
		fmt.Printf("%s\n", name)
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
	node, err := SetUpNode()
	if err != nil {
		panic("Error setting up nodes: %v\n")
	}

	functions := []string{
		// "Bless", // Michael+Sourabh
		// "Assign", // Michael+Sourabh
		// "Designate", // Michael+Sourabh
		// "Checkpoint", // Michael+Sourabh
		"New",      // William
		"Upgrade",  // William
		"Transfer", // William
		"Quit",     // William
		"Solicit",  // William
		"Forget",   // William
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
	node, err := SetUpNode()
	if err != nil {
		panic("Error setting up nodes: %v\n")
	}

	functions := []string{
		// "Gas", // Michael
		"Lookup",           // William
		"Read",             // William
		"Write",            // William
		"Info",             // Shawn
		"Sp1Groth16Verify", // Sourabh
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
					for _, page := range tc.InitialMemory.Pages {
						page.Access.Inaccessible = true
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
					for _, page := range tc.InitialMemory.Pages {
						page.Access.Inaccessible = true
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
					for _, page := range tc.InitialMemory.Pages {
						page.Access.Inaccessible = true
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

// Generate General test vectors
func TestGenerateGeneralTestVectors(t *testing.T) {
	dirPath := "../jamtestvectors/host_function"
	functions := []string{"Lookup", "Read", "Write", "Info", "Sp1Groth16Verify"}
	errorCases := map[string][]uint64{
		// B.6 General Functions
		"Gas":              {pvm.OK},
		"Lookup":           {pvm.OK, pvm.NONE, pvm.OOB},
		"Read":             {pvm.OK, pvm.OOB, pvm.NONE},
		"Write":            {pvm.OK, pvm.OOB, pvm.FULL, pvm.OOB},
		"Info":             {pvm.OK, pvm.OOB, pvm.NONE},
		"Sp1Groth16Verify": {pvm.OK, pvm.OOB, pvm.HUH},
	}
	templateFileName := "./templates/hostGeneralTemplate.json"
	testCaseType := "General"
	GenerateTestVectors(t, dirPath, functions, errorCases, templateFileName, testCaseType)

	// LATER: "Gas":      {pvm.OK},
	// TODO: William
	// "Lookup":   {pvm.OK, pvm.NONE, pvm.OOB},
	// "Read":  {pvm.OK, pvm.OOB, pvm.NONE},
	// "Write": {pvm.OK, pvm.OOB, pvm.FULL, pvm.OOB},

	// TODO: Shawn
	// "Info":     {pvm.OK, pvm.OOB, pvm.NONE},

	// TODO: Sourabh
	// "Sp1Groth16Verify":  {pvm.OK, pvm.OOB, pvm.HUH},

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

func CompareRefineMMaps(map1, map2 pvm.RefineM_map) bool {
	if len(map1) != len(map2) {
		return false
	}

	for key, val1 := range map1 {
		val2, exists := map2[key]
		if !exists {
			return false
		}

		if !CompareRefineM(val1, val2) {
			return false
		}
	}

	return true
}

func CompareRefineM(m1, m2 *pvm.RefineM) bool {
	if m1 == nil || m2 == nil {
		return m1 == m2
	}

	if !bytes.Equal(m1.P, m2.P) {
		return false
	}

	if !CompareRAM(m1.U, m2.U) {
		return false
	}

	if m1.I != m2.I {
		return false
	}

	return true
}

// ComparePage compares two MemoryPage instances for equality.
func ComparePage(page1, page2 *pvm.Page) bool {
	if page1 == nil || page2 == nil {
		return page1 == page2
	}

	// Compare access modes
	if page1.Access != page2.Access {
		return false
	}

	// Compare contents
	if (page1.Value == nil) != (page2.Value == nil) {
		return false
	}
	if page1.Value != nil && !bytes.Equal(page1.Value, page2.Value) {
		return false
	}

	return true
}

// CompareRAM compares two RAM instances for equality.
func CompareRAM(ram1, ram2 *pvm.RAM) bool {
	if ram1 == nil || ram2 == nil {
		return ram1 == ram2
	}

	// Compare the number of allocated pages
	if len(ram1.Pages) != len(ram2.Pages) {
		return false
	}

	// Compare each page in the RAM
	for pageIndex, page1 := range ram1.Pages {
		page2, exists := ram2.Pages[pageIndex]
		if !exists || !ComparePage(page1, page2) {
			return false
		}
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
		keyByte := common.Hex2Hash(k)
		sa.Storage[keyByte] = types.StorageObject{Value: v}
	}

	// Convert Lookup map
	for k, v := range saft.Lookup {
		keyHash := common.HexToHash(k)
		sa.Lookup[keyHash] = types.LookupObject{T: v}
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
		// common.Hash to string
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
		ram := pvm.NewRAM()

		for page_addr, page := range v.U.Pages {
			ram.SetPageAccess(page_addr, 1, page.Access)
			ram.WriteRAMBytes(page_addr*pvm.PageSize, page.Value)
		}
		refineM_map[k] = &pvm.RefineM{
			P: v.P,
			U: ram,
			I: v.I,
		}
	}
	return refineM_map
}

func ConvertToRefineM_mapForTest(refineM_map map[uint32]*pvm.RefineM) map[uint32]*RefineMForTest {
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

func TestGenerateRefineTestVectors(t *testing.T) {
	dirPath := "../jamtestvectors/host_function"
	templateFileName := "./templates/hostRefineTemplate.json"
	testCaseType := "Refine"
	functions := []string{"Historical_lookup", "Import", "Export", "Machine", "Peek", "Poke", "Zero", "Void", "Invoke", "Expunge"}
	errorCases := map[string][]uint64{
		// B.8 Refine Functions
		"Historical_lookup": {pvm.OK, pvm.OOB},
		"Import":            {pvm.OK, pvm.OOB, pvm.NONE},
		"Export":            {pvm.OK, pvm.OOB, pvm.FULL},
		"Machine":           {pvm.OK, pvm.OOB},
		"Peek":              {pvm.OK, pvm.OOB, pvm.WHO},
		"Poke":              {pvm.OK, pvm.OOB, pvm.WHO},
		"Zero":              {pvm.OK, pvm.OOB, pvm.WHO},
		"Void":              {pvm.OK, pvm.OOB, pvm.WHO},
		"Invoke":            {pvm.OK, pvm.OOB, pvm.WHO, pvm.HOST, pvm.FAULT, pvm.OOB, pvm.PANIC},
		"Expunge":           {pvm.OK, pvm.OOB, pvm.WHO},
	}

	GenerateTestVectors(t, dirPath, functions, errorCases, templateFileName, testCaseType)

	testCases := []struct {
		filename          string
		initialRegs       map[uint32]uint64
		expectedRegs      map[uint32]uint64
		initialMemory     RAMForTest
		expectedMemory    RAMForTest
		initialSegment    []ByteSlice
		expectedSegment   []ByteSlice
		initialSegmentIdx uint64
	}{
		// TODO: William
		// "Historical_lookup":  {pvm.OK, pvm.OOB},
		// "Import": {pvm.OK, pvm.OOB, pvm.NONE},
		{
			filename: "hostImportOK.json",
			initialRegs: map[uint32]uint64{
				7: 0,
				8: 32 * pvm.PageSize,
				9: 12,
			},
			expectedRegs: map[uint32]uint64{
				7: pvm.OK,
				8: 32 * pvm.PageSize,
				9: 12,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: ByteSlice{}, Access: pvm.AccessMode{Writable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: ByteSlice{10, 10, 11, 11, 12, 12, 13, 13, 14, 14, 15, 15}, Access: pvm.AccessMode{Writable: true}},
				},
			},
			initialSegment: []ByteSlice{{10, 10, 11, 11, 12, 12, 13, 13, 14, 14, 15, 15}},
		},
		{
			filename: "hostImportOOB.json",
			initialRegs: map[uint32]uint64{
				7: 0,
				8: 32 * pvm.PageSize,
				9: 12,
			},
			expectedRegs: map[uint32]uint64{
				7: pvm.OOB,
				8: 32 * pvm.PageSize,
				9: 12,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: ByteSlice{}, Access: pvm.AccessMode{Inaccessible: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: ByteSlice{}, Access: pvm.AccessMode{Inaccessible: true}},
				},
			},
			initialSegment: []ByteSlice{{10, 10, 11, 11, 12, 12, 13, 13, 14, 14, 15, 15}},
		},
		{
			filename: "hostImportNONE.json",
			initialRegs: map[uint32]uint64{
				7: 9999, // Simulate NONE error
				8: 32 * pvm.PageSize,
				9: 12,
			},
			expectedRegs: map[uint32]uint64{
				7: pvm.NONE,
				8: 32 * pvm.PageSize,
				9: 12,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: ByteSlice{}, Access: pvm.AccessMode{Writable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: ByteSlice{}, Access: pvm.AccessMode{Writable: true}},
				},
			},
			initialSegment: []ByteSlice{{10, 10, 11, 11, 12, 12, 13, 13, 14, 14, 15, 15}},
		},
		// "Export": {pvm.OK, pvm.OOB, pvm.FULL},
		{
			filename: "hostExportOK.json",
			initialRegs: map[uint32]uint64{
				7: 32 * pvm.PageSize,
				8: 12,
			},
			expectedRegs: map[uint32]uint64{
				7: 1, // Initial index + length
				8: 12,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: ByteSlice{10, 10, 11, 11, 12, 12, 13, 13, 14, 14, 15, 15}, Access: pvm.AccessMode{Readable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: ByteSlice{10, 10, 11, 11, 12, 12, 13, 13, 14, 14, 15, 15}, Access: pvm.AccessMode{Readable: true}},
				},
			},
			initialSegment:    []ByteSlice{{}},
			expectedSegment:   []ByteSlice{{}, {10, 10, 11, 11, 12, 12, 13, 13, 14, 14, 15, 15, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
			initialSegmentIdx: 0,
		},
		{
			filename: "hostExportOOB.json",
			initialRegs: map[uint32]uint64{
				7: 32 * pvm.PageSize,
				8: 12,
			},
			expectedRegs: map[uint32]uint64{
				7: pvm.OOB,
				8: 12,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: ByteSlice{10, 10, 11, 11, 12, 12, 13, 13, 14, 14, 15, 15}, Access: pvm.AccessMode{Inaccessible: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: ByteSlice{10, 10, 11, 11, 12, 12, 13, 13, 14, 14, 15, 15}, Access: pvm.AccessMode{Inaccessible: true}},
				},
			},
			initialSegment:    []ByteSlice{{}},
			expectedSegment:   []ByteSlice{{}},
			initialSegmentIdx: 0,
		},
		{
			filename: "hostExportFULL.json",
			initialRegs: map[uint32]uint64{
				7: 4278124544, // 0xFEFF0000
				8: 12,
			},
			expectedRegs: map[uint32]uint64{
				7: pvm.FULL,
				8: 12,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: ByteSlice{10, 10, 11, 11, 12, 12, 13, 13, 14, 14, 15, 15}, Access: pvm.AccessMode{Readable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: ByteSlice{10, 10, 11, 11, 12, 12, 13, 13, 14, 14, 15, 15}, Access: pvm.AccessMode{Readable: true}},
				},
			},
			initialSegment:    []ByteSlice{{}},
			expectedSegment:   []ByteSlice{{}},
			initialSegmentIdx: 9999, // Simulate FULL error
		},
		// TODO: Shawn: 7 host functions
		// "Machine":  {pvm.OK, pvm.OOB},
		// "Peek":     {pvm.OK, pvm.OOB, pvm.WHO},
		// "Poke":     {pvm.OK, pvm.OOB, pvm.WHO},
		// "Zero":     {pvm.OK, pvm.OOB, pvm.WHO},
		// "Void":     {pvm.OK, pvm.OOB, pvm.WHO},
		// "Invoke":   {pvm.OK, pvm.OOB, pvm.WHO, pvm.HOST, pvm.FAULT, pvm.OOB, pvm.PANIC},
		// "Expunge":  {pvm.OK, pvm.OOB, pvm.WHO},
	}

	for _, tc := range testCases {
		filePath := filepath.Join(dirPath, tc.filename)

		var testcase RefineTestcase
		if err := ReadJSONFile(filePath, &testcase); err != nil {
			fmt.Printf("Failed to read test case %s: %v\n", tc.filename, err)
			continue
		}

		updateRefineTestCase(&testcase, tc.initialRegs, tc.expectedRegs, tc.initialMemory, tc.expectedMemory, tc.initialSegment, tc.expectedSegment, tc.initialSegmentIdx)

		if err := WriteJSONFile(filePath, testcase); err != nil {
			fmt.Printf("Failed to write test case %s: %v\n", tc.filename, err)
		}
	}
}

func updateRefineTestCase(testcase *RefineTestcase, initialRegs, expectedRegs map[uint32]uint64, initialMemory, expectedMemory RAMForTest, initialSegment, expectedSegment []ByteSlice, initialSegmentIdx uint64) {
	testcase.InitialRegs = initialRegs
	testcase.ExpectedRegs = expectedRegs
	testcase.InitialMemory = initialMemory
	testcase.ExpectedMemory = expectedMemory
	testcase.InitialImportSegment = initialSegment
	testcase.InitialExportSegmentIdx = uint32(initialSegmentIdx)
	testcase.ExpectedExportSegment = expectedSegment
}

func TestGenerateAccumulateTestVectors(t *testing.T) {
	dirPath := "../jamtestvectors/host_function"
	templateFileName := "./templates/hostAccumulateTemplate.json"
	testCaseType := "Accumulate"
	functions := []string{"New", "Upgrade", "Solicit", "Forget", "Quit", "Transfer"}
	errorCases := map[string][]uint64{
		// B.7 Accumulate Functions
		//"Bless":    {pvm.OK, pvm.OOB, pvm.WHO},
		//"Assign":   {pvm.OK, pvm.OOB, pvm.CORE},
		//"Designate":   {pvm.OK, pvm.OOB},
		//"Checkpoint":   {pvm.OK},
		"New":      {pvm.OK, pvm.OOB, pvm.CASH},
		"Upgrade":  {pvm.OK, pvm.OOB},
		"Solicit":  {pvm.OK, pvm.OOB, pvm.FULL, pvm.HUH},
		"Forget":   {pvm.OK, pvm.OOB, pvm.HUH},
		"Quit":     {pvm.OK, pvm.OOB, pvm.WHO, pvm.LOW},
		"Transfer": {pvm.OK, pvm.WHO, pvm.CASH, pvm.LOW, pvm.HIGH},
	}

	GenerateTestVectors(t, dirPath, functions, errorCases, templateFileName, testCaseType)

	testCases := []struct {
		filename       string
		initialRegs    map[uint32]uint64
		expectedRegs   map[uint32]uint64
		initialMemory  RAMForTest
		expectedMemory RAMForTest
		sourceAccount  *ServiceAccountForTest
		targetAccount  *ServiceAccountForTest
		balanceChange  uint64
	}{
		// 		"New":      {pvm.OK, pvm.OOB, pvm.CASH},
		{
			filename: "hostNewOK.json",
			initialRegs: map[uint32]uint64{
				7:  32 * pvm.PageSize,
				8:  12,
				9:  100,
				10: 200,
			},
			expectedRegs: map[uint32]uint64{
				7:  1,
				8:  12,
				9:  100,
				10: 200,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: ByteSlice{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, Access: pvm.AccessMode{Readable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: ByteSlice{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, Access: pvm.AccessMode{Readable: true}},
				},
			},
			sourceAccount: &ServiceAccountForTest{
				Balance: 300,
			},
			targetAccount: &ServiceAccountForTest{
				Balance: 213,
			},
			balanceChange: 213,
		},
		{
			filename: "hostNewOOB.json",
			initialRegs: map[uint32]uint64{
				7:  32 * pvm.PageSize,
				8:  12,
				9:  100,
				10: 200,
			},
			expectedRegs: map[uint32]uint64{
				7:  pvm.OOB,
				8:  12,
				9:  100,
				10: 200,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: ByteSlice{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, Access: pvm.AccessMode{Inaccessible: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: ByteSlice{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, Access: pvm.AccessMode{Inaccessible: true}},
				},
			},
			sourceAccount: nil,
			targetAccount: nil,
			balanceChange: 0,
		},
		{
			filename: "hostNewCASH.json",
			initialRegs: map[uint32]uint64{
				7:  32 * pvm.PageSize,
				8:  12,
				9:  100,
				10: 200,
			},
			expectedRegs: map[uint32]uint64{
				7:  pvm.CASH,
				8:  12,
				9:  100,
				10: 200,
			},
			initialMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: ByteSlice{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, Access: pvm.AccessMode{Readable: true}},
				},
			},
			expectedMemory: RAMForTest{
				Pages: map[uint32]*PageForTest{
					32: {Value: ByteSlice{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, Access: pvm.AccessMode{Readable: true}},
				},
			},
			sourceAccount: &ServiceAccountForTest{
				Balance: 300,
			},
			targetAccount: &ServiceAccountForTest{
				Balance: 213,
			},
			balanceChange: 213,
		},
		// William 1 (first) => 5 (last)
		// 5 "Upgrade":      {pvm.OK, pvm.OOB},
		// 2 "Solicit":  {pvm.OK, pvm.OOB, pvm.FULL, pvm.HUH},
		// 3 "Forget":   {pvm.OK, pvm.OOB, pvm.HUH},
		// 4 "Quit": {pvm.OK, pvm.OOB, pvm.WHO, pvm.LOW},
		// 1 "Transfer": {pvm.OK, pvm.WHO, pvm.CASH, pvm.LOW, pvm.HIGH},
	}

	for _, tc := range testCases {
		filePath := filepath.Join(dirPath, tc.filename)

		var testcase AccumulateTestcase
		if err := ReadJSONFile(filePath, &testcase); err != nil {
			fmt.Printf("Failed to read test case %s: %v\n", tc.filename, err)
			continue
		}

		updateAccumulateTestCase(&testcase, tc.initialRegs, tc.expectedRegs, tc.initialMemory, tc.expectedMemory, tc.sourceAccount, tc.targetAccount, tc.balanceChange)

		if err := WriteJSONFile(filePath, testcase); err != nil {
			fmt.Printf("Failed to write test case %s: %v\n", tc.filename, err)
		}
	}
}

func updateAccumulateTestCase(testcase *AccumulateTestcase, initialRegs, expectedRegs map[uint32]uint64, initialMemory, expectedMemory RAMForTest, sourceAccount, targetAccount *ServiceAccountForTest, balanceChange uint64) {
	testcase.InitialRegs = initialRegs
	testcase.ExpectedRegs = expectedRegs
	testcase.InitialMemory = initialMemory
	testcase.ExpectedMemory = expectedMemory

	if sourceAccount != nil && targetAccount != nil {
		if testcase.InitialXcontent_x == nil {
			testcase.InitialXcontent_x = &XContextForTest{
				U: &PartialStateForTest{
					D: map[uint32]*ServiceAccountForTest{},
				},
			}
		}
		if testcase.ExpectedXcontent_x == nil {
			testcase.ExpectedXcontent_x = DeepCopyXContextForTest(testcase.InitialXcontent_x)
		}
		if testcase.ExpectedXcontent_x.U != nil && testcase.ExpectedXcontent_x.U.D != nil {
			testcase.ExpectedXcontent_x.U.D[0] = sourceAccount
			testcase.ExpectedXcontent_x.U.D[1] = targetAccount
			testcase.ExpectedXcontent_x.U.D[0].Balance -= balanceChange
		}
	}
}
