// run test: go test ./statedb -v
package statedb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/pvm/recompiler"
	"github.com/colorfulnotion/jam/types"
)

// memory_for test
type TestMemory struct {
	Address uint32 `json:"address"`
	Data    []byte `json:"contents"`
}

type TestPageMap struct {
	Address    uint32 `json:"address"`
	Length     uint32 `json:"length"`
	IsWritable bool   `json:"is-writable"` // true if the memory is written to, false if it is read from
}

// TestCase
type TestCase struct {
	Name           string        `json:"name"`
	InitialRegs    []uint64      `json:"initial-regs"`
	InitialPC      uint32        `json:"initial-pc"`
	InitialPageMap []TestPageMap `json:"initial-page-map"`
	InitialMemory  []TestMemory  `json:"initial-memory"`
	InitialGas     int64         `json:"initial-gas"`
	Code           []int         `json:"program"`
	//Bitmask        []int         `json:"bitmask"`
	ExpectedStatus string       `json:"expected-status"`
	ExpectedRegs   []uint64     `json:"expected-regs"`
	ExpectedPC     uint32       `json:"expected-pc"`
	ExpectedMemory []TestMemory `json:"expected-memory"`
}

func pvm_test(tc TestCase, testMode string) error {
	// Test with Go backend by default, but can be changed for comparison
	switch testMode {
	case "recompiler":
		return recompiler_test(tc)
	case "C_FFI":
		return pvm_test_backend(tc, BackendInterpreter)
	default:
		return pvm_test_backend(tc, BackendInterpreter)
	}
}

func pvm_test_backend(tc TestCase, backend string) error {
	hostENV := NewMockHostEnv()
	serviceAcct := uint32(0) // stub

	// Convert test code to raw instruction bytes
	rawCodeBytes := make([]byte, len(tc.Code))
	for i, val := range tc.Code {
		rawCodeBytes[i] = byte(val)
	}

	// setup Gas
	var initialGas int64 = tc.InitialGas
	if tc.InitialGas == 0 {
		initialGas = 10000 // Default gas for tests
	}

	pvm := NewVM(serviceAcct, rawCodeBytes, tc.InitialRegs, uint64(tc.InitialPC), 4096, hostENV, false, []byte{}, backend, uint64(initialGas))
	defer pvm.Destroy()

	// Setup memory
	for _, mem := range tc.InitialMemory {
		pvm.WriteRAMBytes(mem.Address, mem.Data[:])
	}

	pvm.IsChild = false
	err := pvm.Execute(pvm, uint32(tc.InitialPC))
	if err != nil {
		return fmt.Errorf("C execution failed: %v", err)
	}

	// Check the registers
	actualRegs := pvm.ReadRegisters()
	if equalIntSlices(actualRegs[:], tc.ExpectedRegs) {
		// fmt.Printf("Register match for test %s \n", tc.Name)
		return nil
	}

	resultCode := pvm.ResultCode
	resultCodeStr := types.HostResultCodeToString[resultCode]
	if resultCodeStr == "page-fault" {
		resultCodeStr = "panic"
	}
	expectedCodeStr := tc.ExpectedStatus
	if expectedCodeStr == "page-fault" {
		expectedCodeStr = "panic"
	}
	if resultCodeStr == expectedCodeStr {
		fmt.Printf("Result code match for test %s: %s\n", tc.Name, resultCodeStr)
	} else {
		return fmt.Errorf("result code mismatch for test %s: expected %s, got %s", tc.Name, expectedCodeStr, resultCodeStr)
	}
	return fmt.Errorf("register mismatch for test %s: expected %v, got %v", tc.Name, tc.ExpectedRegs, actualRegs)
}
func recompiler_test(tc TestCase) error {
	var num_mismatch int
	serviceAcct := uint32(0) // stub
	// metadata, c := types.SplitMetadataAndCode(tc.Code)
	// Convert test code to raw instruction bytes
	rawCodeBytes := make([]byte, len(tc.Code))
	for i, val := range tc.Code {
		rawCodeBytes[i] = byte(val)
	}
	fmt.Printf("running test: %s\n", tc.Name)
	hostENV := NewMockHostEnv()
	rvm := NewRecompilerVM(serviceAcct, rawCodeBytes, tc.InitialRegs, uint64(tc.InitialPC), 4096, hostENV, false, []byte{}, 100000, "recompiler")
	// Set the initial memory
	for _, mem := range tc.InitialMemory {
		//pvm.Ram.SetPageAccess(mem.Address/PageSize, 1, AccessMode{Readable: false, Writable: true, Inaccessible: false})
		rvm.SetMemAccess(mem.Address, uint32(len(mem.Data)), recompiler.PageMutable)
		rvm.WriteRAMBytes(mem.Address, mem.Data[:])
	}
	// if len(tc.InitialMemory) == 0 {
	// 	pvm.Ram.SetPageAccess(32, 1, AccessMode{Readable: false, Writable: false, Inaccessible: true})
	// }
	resultCode := uint8(0)
	rvm.Gas = 100000000000
	for _, pm := range tc.InitialPageMap {
		// Set the page access based on the initial page map
		if pm.IsWritable {
			err := rvm.SetMemAccess(pm.Address, pm.Length, recompiler.PageMutable)
			if err != nil {
				return fmt.Errorf("failed to set memory access for address %x: %w", pm.Address, err)
			}
		}
	}

	for _, mem := range tc.InitialMemory {
		// Write the initial memory contents
		rvm.WriteMemory(mem.Address, mem.Data)
	}
	rvm.SetPC(0)
	for i, reg := range tc.InitialRegs {
		rvm.WriteRegister(i, reg)
		fmt.Printf("Register %d initialized to %d\n", i, reg)
	}
	vm := &VM{}
	vm.ExecutionVM = rvm
	rvm.Execute(vm, 0)
	// check the memory
	for _, mem := range tc.ExpectedMemory {
		data, err := rvm.ReadMemory(mem.Address, uint32(len(mem.Data)))
		if err != nil {
			return fmt.Errorf("failed to read memory at address %x: %w", mem.Address, err)
		}
		if !bytes.Equal(data, mem.Data) {
			num_mismatch++
			return fmt.Errorf("Memory mismatch for test %s at address %x: expected %x, got %x \n", tc.Name, mem.Address, mem.Data, data)
		} else {
			fmt.Printf("Memory match for test %s at address %x \n", tc.Name, mem.Address)
		}
	}
	for i, reg := range rvm.ReadRegisters() {
		if reg != tc.ExpectedRegs[i] {
			num_mismatch++
			v := rvm.ReadRegister(i)
			fmt.Printf("MISMATCH expected %v got [%d]=%v in %v\n", tc.ExpectedRegs, i, v, rvm.ReadRegisters())
			return fmt.Errorf("register mismatch for test %s at index %d: expected %d, got %d", tc.Name, i, tc.ExpectedRegs[i], reg)
		}
	}
	resultCode = rvm.MachineState
	// Check the registers
	for i, reg := range rvm.ReadRegisters() {
		if reg != tc.ExpectedRegs[i] {
			fmt.Printf("MISMATCH expected %v got [%d]=%v in %v\n", tc.ExpectedRegs, i, reg, rvm.ReadRegisters())
			return fmt.Errorf("register mismatch for test %s at index %d: expected %d, got %d", tc.Name, i, tc.ExpectedRegs[i], reg)
		}
	}
	rvm.Close()
	return nil
	resultCodeStr := types.HostResultCodeToString[resultCode]
	if resultCodeStr == "page-fault" {
		resultCodeStr = "panic"
	}
	expectedCodeStr := tc.ExpectedStatus
	if expectedCodeStr == "page-fault" {
		expectedCodeStr = "panic"
	}
	if resultCodeStr == expectedCodeStr {
		fmt.Printf("Result code match for test %s: %s\n", tc.Name, resultCodeStr)
	} else {
		return fmt.Errorf("result code mismatch for test %s: expected %s, got %s", tc.Name, expectedCodeStr, resultCodeStr)
	}
	return fmt.Errorf("register mismatch for test %s: expected %v, got %v", tc.Name, tc.ExpectedRegs, rvm.ReadRegisters())
}

// BackendResult holds the execution result from a backend
type BackendResult struct {
	Registers  [13]uint64
	ResultCode uint8
	PC         uint64
	Gas        int64
}

func TestPVMAll(t *testing.T) {
	mode := "C_FFI" // change to "C_FFI" to test the C backend
	log.InitLogger("debug")
	// Directory containing the JSON files
	dir := "programs"

	// Read all files in the directory
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}
	count := 0
	num_mismatch := 0
	total_mismatch := 0
	skip := map[string]bool{
		"inst_ecalli_100.json": true, // skip ecalli test for now
	}
	for _, file := range files {
		if skip[file.Name()] {
			continue
		}
		if strings.Contains(file.Name(), "riscv") {
			continue // skip riscv tests
		}
		count++
		if file.IsDir() {
			continue
		}

		if !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		filePath := filepath.Join(dir, file.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("Failed to read file %s: %v", filePath, err)
		}

		var testCase TestCase
		err = json.Unmarshal(data, &testCase)
		if err != nil {
			t.Fatalf("Failed to unmarshal JSON from file %s: %v", filePath, err)
		}
		name := testCase.Name
		t.Run(name, func(t *testing.T) {
			err = pvm_test(testCase, mode)
			if err != nil {
				t.Errorf("❌ [%s] Test failed: %v", name, err)
			} else {
				t.Logf("✅ [%s] Test passed", name)
			}
		})
		total_mismatch += num_mismatch
	}
	// show the match rate
	fmt.Printf("Match rate: %v/%v\n", count-total_mismatch, count)
}

// Helper function to compare two integer slices
func equalIntSlices(a, b []uint64) bool {
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

func TestSinglePVM(t *testing.T) {
	log.InitLogger("debug")
	filename := "programs/inst_ecalli_100.json"
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("Failed to read test file %s: %v", filename, err)
	}

	var tc TestCase
	err = json.Unmarshal(data, &tc)
	if err != nil {
		t.Fatalf("Failed to unmarshal test case from %s: %v", filename, err)
	}

	// Debug: Print the program bytes and expected behavior
	t.Logf("Program bytes: %v", tc.Code)
	t.Logf("Initial regs: %v", tc.InitialRegs)
	t.Logf("Expected regs: %v", tc.ExpectedRegs)
	t.Logf("Expected PC: %d", tc.ExpectedPC)
	t.Logf("Expected status: %s", tc.ExpectedStatus)

	// Test with C FFI backend directly
	t.Run("recompiler", func(t *testing.T) {
		err = recompiler_test(tc)
		if err != nil {
			t.Errorf("CGO backend test failed for %s: %v", tc.Name, err)
		}
	})
}

func TestBranchEqNok(t *testing.T) {
	mode := "C_FFI" // change to "C_FFI" to test the C backend
	log.InitLogger("debug")
	filename := "programs/inst_branch_eq_nok.json"
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("Failed to read test file %s: %v", filename, err)
	}

	var tc TestCase
	err = json.Unmarshal(data, &tc)
	if err != nil {
		t.Fatalf("Failed to unmarshal test case from %s: %v", filename, err)
	}

	// Debug: Print the program bytes and expected behavior
	t.Logf("Program bytes: %v", tc.Code)
	t.Logf("Initial regs: %v", tc.InitialRegs)
	t.Logf("Expected regs: %v", tc.ExpectedRegs)
	t.Logf("Expected PC: %d", tc.ExpectedPC)
	t.Logf("Expected status: %s", tc.ExpectedStatus)

	// Test with C FFI backend directly
	t.Run("C_FFI", func(t *testing.T) {
		err = pvm_test(tc, mode)
		if err != nil {
			t.Errorf("CGO backend test failed for %s: %v", tc.Name, err)
		}
	})
}

func TestLoadU32(t *testing.T) {
	mode := "C_FFI" // change to "C_FFI" to test the C backend
	log.InitLogger("debug")
	filename := "programs/inst_load_u32.json"
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("Failed to read test file %s: %v", filename, err)
	}

	var tc TestCase
	err = json.Unmarshal(data, &tc)
	if err != nil {
		t.Fatalf("Failed to unmarshal test case from %s: %v", filename, err)
	}

	// Debug: Print the program bytes and expected behavior
	t.Logf("Program bytes: %v", tc.Code)
	t.Logf("Initial regs: %v", tc.InitialRegs)
	t.Logf("Expected regs: %v", tc.ExpectedRegs)
	t.Logf("Expected PC: %d", tc.ExpectedPC)
	t.Logf("Expected status: %s", tc.ExpectedStatus)

	// Test with C FFI backend directly
	t.Run("C_FFI", func(t *testing.T) {
		err = pvm_test(tc, mode)
		if err != nil {
			t.Errorf("CGO backend test failed for %s: %v", tc.Name, err)
		}
	})
}

func TestEcalli(t *testing.T) {
	mode := "recompiler" // change to "C_FFI" to test the C backend
	log.InitLogger("debug")
	filename := "programs/inst_ecalli_100.json"
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("Failed to read test file %s: %v", filename, err)
	}

	var tc TestCase
	err = json.Unmarshal(data, &tc)
	if err != nil {
		t.Fatalf("Failed to unmarshal test case from %s: %v", filename, err)
	}

	err = pvm_test(tc, mode)
	if err != nil {
		t.Errorf("CGO backend test failed for %s: %v", tc.Name, err)
	}
}
