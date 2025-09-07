// run test: go test ./statedb -v
package statedb

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/colorfulnotion/jam/log"
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

func pvm_test(tc TestCase) error {
	// Test with Go backend by default, but can be changed for comparison
	return pvm_test_backend(tc, BackendInterpreter)
}

func pvm_test_backend(tc TestCase, backend string) error {
	hostENV := NewMockHostEnv()
	serviceAcct := uint32(0) // stub

	// Convert test code to raw instruction bytes
	rawCodeBytes := make([]byte, len(tc.Code))
	for i, val := range tc.Code {
		rawCodeBytes[i] = byte(val)
	}

	pvm := NewVM(serviceAcct, rawCodeBytes, tc.InitialRegs, uint64(tc.InitialPC), 4096, hostENV, false, []byte{}, backend)
	defer pvm.Destroy()

	// Setup memory
	for _, mem := range tc.InitialMemory {
		pvm.WriteRAMBytes(mem.Address, mem.Data[:])
	}

	// setup Gas
	var initialGas int64 = tc.InitialGas
	if tc.InitialGas == 0 {
		initialGas = 10000 // Default gas for tests
	}

	// Set gas in C VM
	pvm.SetGas(initialGas)

	pvm.EntryPoint = uint32(tc.InitialPC)
	pvm.IsChild = false
	err := pvm.Execute()
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

// BackendResult holds the execution result from a backend
type BackendResult struct {
	Registers  [13]uint64
	ResultCode uint8
	PC         uint64
	Gas        int64
}

func TestPVMAll(t *testing.T) {
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
	for _, file := range files {

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
			err = pvm_test(testCase)
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

func TestAddImm32(t *testing.T) {
	log.InitLogger("debug")
	filename := "programs/inst_add_imm_32.json"
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
		err = pvm_test(tc)
		if err != nil {
			t.Errorf("CGO backend test failed for %s: %v", tc.Name, err)
		}
	})
}

func TestBranchEqNok(t *testing.T) {
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
		err = pvm_test(tc)
		if err != nil {
			t.Errorf("CGO backend test failed for %s: %v", tc.Name, err)
		}
	})
}

func TestLoadU32(t *testing.T) {
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
		err = pvm_test(tc)
		if err != nil {
			t.Errorf("CGO backend test failed for %s: %v", tc.Name, err)
		}
	})
}

func TestEcalli(t *testing.T) {
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

	err = pvm_test(tc)
	if err != nil {
		t.Errorf("CGO backend test failed for %s: %v", tc.Name, err)
	}
}
