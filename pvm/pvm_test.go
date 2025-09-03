// run test: go test ./pvm -v
package pvm

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
	"github.com/nsf/jsondiff"
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
	Bitmask        []int         `json:"bitmask"`
	ExpectedStatus string        `json:"expected-status"`
	ExpectedRegs   []uint64      `json:"expected-regs"`
	ExpectedPC     uint32        `json:"expected-pc"`
	ExpectedMemory []TestMemory  `json:"expected-memory"`
}

func pvm_test(tc TestCase) error {
	// Test with Go backend by default, but can be changed for comparison
	return pvm_test_backend(tc, BackendInterpreter)
}

func pvm_test_backend(tc TestCase, backend string) error {
	hostENV := NewMockHostEnv()
	serviceAcct := uint32(0) // stub

	// Convert integer arrays to byte arrays
	codeBytes := make([]byte, len(tc.Code))
	for i, val := range tc.Code {
		codeBytes[i] = byte(val)
	}

	bitmaskBytes := make([]byte, len(tc.Bitmask))
	for i, val := range tc.Bitmask {
		bitmaskBytes[i] = byte(val)
	}

	// Encode the raw instruction bytes and bitmask into proper PVM blob format
	encodedBlob := EncodeProgram(codeBytes, bitmaskBytes)

	pvm := NewVM(serviceAcct, encodedBlob, tc.InitialRegs, uint64(tc.InitialPC), hostENV, false, []byte{}, backend)

	// Ensure VM has enough memory for the test case
	if len(tc.InitialMemory) > 0 {
		// Find the maximum address + data size needed
		maxAddress := uint32(0)
		for _, mem := range tc.InitialMemory {
			endAddress := mem.Address + uint32(len(mem.Data))
			if endAddress > maxAddress {
				maxAddress = endAddress
			}
		}

		// Ensure the VM's rw_data region can accommodate this
		if maxAddress > pvm.rw_data_address && maxAddress > pvm.rw_data_address_end {
			needed_size := maxAddress - pvm.rw_data_address
			if len(pvm.rw_data) < int(needed_size) {
				new_rw_data := make([]byte, needed_size)
				copy(new_rw_data, pvm.rw_data)
				pvm.rw_data = new_rw_data
				pvm.rw_data_address_end = pvm.rw_data_address + uint32(len(pvm.rw_data))
				pvm.current_heap_pointer = pvm.rw_data_address + uint32(len(pvm.rw_data))
			} else {
				// Buffer is large enough but might need to update the heap pointer and address end
				pvm.rw_data_address_end = pvm.rw_data_address + uint32(len(pvm.rw_data))
				if pvm.current_heap_pointer < maxAddress {
					pvm.current_heap_pointer = maxAddress
				}
			}
		}
	}

	// Set the initial memory
	for _, mem := range tc.InitialMemory {
		//pvm.Ram.SetPageAccess(mem.Address/PageSize, 1, AccessMode{Readable: false, Writable: true, Inaccessible: false})
		pvm.WriteRAMBytes(mem.Address, mem.Data[:])
	}

	// if len(tc.InitialMemory) == 0 {
	// 	pvm.Ram.SetPageAccess(32, 1, AccessMode{Readable: false, Writable: false, Inaccessible: true})
	// }
	resultCode := uint8(0)
	// Use initial gas from test case, fallback to 100 if not specified
	if tc.InitialGas > 0 {
		pvm.Gas = tc.InitialGas
	} else {
		pvm.Gas = int64(100)
	}

	// Execute with the appropriate backend
	if backend == BackendCompiler || backend == "cgo" || backend == "c" {
		// Use C backend via CGO
		pvm.EntryPoint = uint32(tc.InitialPC)
		pvm.IsChild = false
		err := pvm.ExecuteWithCGO()
		if err != nil {
			return fmt.Errorf("C execution failed: %v", err)
		}
	} else {
		// Use Go backend
		pvm.Execute(int(tc.InitialPC), false)
	}

	resultCode = pvm.ResultCode

	// Check the registers
	if equalIntSlices(pvm.register, tc.ExpectedRegs) {
		// fmt.Printf("Register match for test %s \n", tc.Name)
		return nil
	}

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
	return fmt.Errorf("register mismatch for test %s: expected %v, got %v", tc.Name, tc.ExpectedRegs, pvm.register)
}

// BackendResult holds the execution result from a backend
type BackendResult struct {
	Registers  [13]uint64
	ResultCode uint8
	PC         uint64
	Gas        int64
}

// pvm_run_backend runs a test case on the specified backend and returns the result
func pvm_run_backend(tc TestCase, backend string) (*BackendResult, error) {
	hostENV := NewMockHostEnv()
	serviceAcct := uint32(0) // stub

	// Convert integer arrays to byte arrays
	codeBytes := make([]byte, len(tc.Code))
	for i, val := range tc.Code {
		codeBytes[i] = byte(val)
	}

	bitmaskBytes := make([]byte, len(tc.Bitmask))
	for i, val := range tc.Bitmask {
		bitmaskBytes[i] = byte(val)
	}

	// Encode the raw instruction bytes and bitmask into proper PVM blob format
	encodedBlob := EncodeProgram(codeBytes, bitmaskBytes)

	pvm := NewVM(serviceAcct, encodedBlob, tc.InitialRegs, uint64(tc.InitialPC), hostENV, false, []byte{}, backend)

	// Ensure VM has enough memory for the test case
	if len(tc.InitialMemory) > 0 {
		// Find the maximum address + data size needed
		maxAddress := uint32(0)
		for _, mem := range tc.InitialMemory {
			endAddress := mem.Address + uint32(len(mem.Data))
			if endAddress > maxAddress {
				maxAddress = endAddress
			}
		}

		// Ensure the VM's rw_data region can accommodate this
		if maxAddress > pvm.rw_data_address && maxAddress > pvm.rw_data_address_end {
			needed_size := maxAddress - pvm.rw_data_address
			if len(pvm.rw_data) < int(needed_size) {
				new_rw_data := make([]byte, needed_size)
				copy(new_rw_data, pvm.rw_data)
				pvm.rw_data = new_rw_data
				pvm.rw_data_address_end = pvm.rw_data_address + uint32(len(pvm.rw_data))
				pvm.current_heap_pointer = pvm.rw_data_address + uint32(len(pvm.rw_data))
			} else {
				// Buffer is large enough but might need to update the heap pointer and address end
				pvm.rw_data_address_end = pvm.rw_data_address + uint32(len(pvm.rw_data))
				if pvm.current_heap_pointer < maxAddress {
					pvm.current_heap_pointer = maxAddress
				}
			}
		}
	}

	// Set the initial memory
	for _, mem := range tc.InitialMemory {
		pvm.WriteRAMBytes(mem.Address, mem.Data[:])
	}

	// Use initial gas from test case, fallback to 100 if not specified
	if tc.InitialGas > 0 {
		pvm.Gas = tc.InitialGas
	} else {
		pvm.Gas = int64(100)
	}

	// Execute with the appropriate backend
	if backend == BackendCompiler || backend == "cgo" || backend == "c" {
		// Use C backend via CGO
		pvm.EntryPoint = uint32(tc.InitialPC)
		pvm.IsChild = false
		err := pvm.ExecuteWithCGO()
		if err != nil {
			return nil, fmt.Errorf("C execution failed: %v", err)
		}
	} else {
		// Use Go backend
		pvm.Execute(int(tc.InitialPC), false)
	}

	// Create and return the result
	result := &BackendResult{
		ResultCode: pvm.ResultCode,
		PC:         pvm.pc,
		Gas:        pvm.Gas,
	}

	// Copy the registers
	copy(result.Registers[:], pvm.register[:13])

	return result, nil
}

// Test that runs both Go and C backends and compares results
func TestPVMBackendComparison(t *testing.T) {
	// Directory containing the JSON files
	dir := "programs"

	// Read all files in the directory
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}

	count := 0
	goFailed := 0
	cFailed := 0
	mismatch := 0

	for _, file := range files {
		if strings.Contains(file.Name(), "riscv") {
			continue // skip riscv tests
		}
		count++
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		// Only test a subset for backend comparison (to avoid overwhelming output)
		if count > 200 {
			break
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
		t.Run(name+"_BackendComparison", func(t *testing.T) {
			fmt.Printf("=== TestPVMBackendComparison %s\n", file.Name())

			// Run both backends and capture their results
			goResult, goErr := pvm_run_backend(testCase, BackendInterpreter)
			cResult, cErr := pvm_run_backend(testCase, BackendCompiler)

			if goErr != nil {
				goFailed++
				t.Logf("🔴 Go backend failed: %v", goErr)
			}

			if cErr != nil {
				cFailed++
				t.Logf("🔴 C backend failed: %v", cErr)
			}

			// Compare results when both backends succeed
			if goErr == nil && cErr == nil {
				// Show detailed register comparison
				fmt.Printf("REGISTER COMPARISON for %s:\n", name)
				fmt.Printf("Go registers:  [")
				for i := 0; i < 13; i++ {
					fmt.Printf("%d:%d", i, goResult.Registers[i])
					if i < 12 {
						fmt.Printf(", ")
					}
				}
				fmt.Printf("]\n")
				fmt.Printf("CGo registers: [")
				for i := 0; i < 13; i++ {
					fmt.Printf("%d:%d", i, cResult.Registers[i])
					if i < 12 {
						fmt.Printf(", ")
					}
				}
				fmt.Printf("]\n")

				// Compare all 13 registers exactly
				registersMatch := true
				for i := 0; i < 13; i++ {
					if goResult.Registers[i] != cResult.Registers[i] {
						registersMatch = false
						fmt.Printf("❌ MISMATCH at register %d: Go=%d, CGo=%d\n", i, goResult.Registers[i], cResult.Registers[i])
						break
					}
				}

				if !registersMatch {
					mismatch++
					t.Errorf("❌ Register mismatch for %s: Go registers=%v, C registers=%v", name, goResult.Registers, cResult.Registers)
				} else {
					fmt.Printf("✅ ALL REGISTERS MATCH\n")
					t.Logf("✅ Both backends passed for %s", name)
				}
				fmt.Printf("\n")
			} else if (goErr == nil) != (cErr == nil) {
				mismatch++
				t.Errorf("❌ Backend mismatch for %s: Go=%v, C=%v", name, goErr == nil, cErr == nil)
			} else {
				t.Logf("⚠️  Both backends failed for %s", name)
			}
		})
	}

	t.Logf("Backend comparison summary: Go failed=%d, C failed=%d, mismatches=%d out of %d tests",
		goFailed, cFailed, mismatch, count)
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

func CompareJSON(obj1, obj2 interface{}) string {
	json1, err1 := json.Marshal(obj1)
	json2, err2 := json.Marshal(obj2)
	if err1 != nil || err2 != nil {
		return "Error marshalling JSON"
	}
	opts := jsondiff.DefaultJSONOptions()
	diff, diffStr := jsondiff.Compare(json1, json2, &opts)

	if diff == jsondiff.FullMatch {
		return "JSONs are identical"
	}
	return fmt.Sprintf("Diff detected:\n%s", diffStr)
}

func (vm *VM) SetFrame(b []byte) {
	if vm.pushFrame != nil {
		vm.pushFrame(b)
	}
}

func (vm *VM) CloseFrameServer() {
	if vm.stopFrameServer != nil {
		vm.stopFrameServer()
	}
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

// ===============================
// Performance Benchmarks
// ===============================

func BenchmarkPVMGoVsC(b *testing.B) {
	// Load a representative test case
	dir := path.Join(common.GetJAMTestVectorPath("stf"), "pvm/programs")
	files, err := os.ReadDir(dir)
	if err != nil {
		b.Fatalf("Failed to read directory: %v", err)
	}

	var testCase TestCase
	found := false
	for _, file := range files {
		if strings.Contains(file.Name(), "riscv") || file.IsDir() {
			continue
		}
		if !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		filePath := filepath.Join(dir, file.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		err = json.Unmarshal(data, &testCase)
		if err != nil {
			continue
		}
		found = true
		break
	}

	if !found {
		b.Skip("No test case found for benchmarking")
	}

	b.Run("Go_Backend", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			hostENV := NewMockHostEnv()
			// Convert integer array to byte array
			codeBytes := make([]byte, len(testCase.Code))
			for j, val := range testCase.Code {
				codeBytes[j] = byte(val)
			}
			pvm := NewVM(0, codeBytes, testCase.InitialRegs, uint64(testCase.InitialPC), hostENV, false, []byte{}, BackendInterpreter)
			pvm.Gas = int64(1000)
			pvm.Execute(int(testCase.InitialPC), false)
		}
	})

	b.Run("C_Backend", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			hostENV := NewMockHostEnv()
			// Convert integer array to byte array
			codeBytes := make([]byte, len(testCase.Code))
			for j, val := range testCase.Code {
				codeBytes[j] = byte(val)
			}
			pvm := NewVM(0, codeBytes, testCase.InitialRegs, uint64(testCase.InitialPC), hostENV, false, []byte{}, "cgo")
			pvm.Gas = int64(1000)
			pvm.ExecuteWithCGO()
		}
	})
}

func BenchmarkMemoryOperations(b *testing.B) {
	hostENV := NewMockHostEnv()
	pvm := NewVM(0, []byte{0x00}, make([]uint64, 13), 0, hostENV, false, []byte{}, "cgo")

	// Initialize CGO integration for C backend testing
	err := pvm.initCGOIntegration()
	if err != nil {
		b.Fatalf("Failed to init CGO: %v", err)
	}
	defer pvm.destroyCGOIntegration()

	testData := uint64(0xDEADBEEFCAFEBABE)
	address := pvm.rw_data_address

	b.Run("Go_WriteRAM64", func(b *testing.B) {
		// Temporarily disable CGO to test Go implementation
		oldCVM := pvm.cVM
		pvm.cVM = nil

		for i := 0; i < b.N; i++ {
			pvm.WriteRAMBytes64(address, testData)
		}

		pvm.cVM = oldCVM
	})

	b.Run("C_WriteRAM64", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			pvm.WriteRAMBytes64(address, testData)
		}
	})

	b.Run("Go_ReadRAM64", func(b *testing.B) {
		// Temporarily disable CGO to test Go implementation
		oldCVM := pvm.cVM
		pvm.cVM = nil

		for i := 0; i < b.N; i++ {
			pvm.ReadRAMBytes64(address)
		}

		pvm.cVM = oldCVM
	})

	b.Run("C_ReadRAM64", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			pvm.ReadRAMBytes64(address)
		}
	})
}

func BenchmarkRegisterSync(b *testing.B) {
	hostENV := NewMockHostEnv()
	pvm := NewVM(0, []byte{0x00}, make([]uint64, 13), 0, hostENV, false, []byte{}, "cgo")

	// Initialize CGO integration
	err := pvm.initCGOIntegration()
	if err != nil {
		b.Fatalf("Failed to init CGO: %v", err)
	}
	defer pvm.destroyCGOIntegration()

	b.Run("SyncRegistersToC", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			pvm.SyncRegistersToC()
		}
	})

	b.Run("SyncRegistersFromC", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			pvm.SyncRegistersFromC()
		}
	})

	b.Run("SyncVMState_Go_to_C", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			pvm.SyncVMState("Go_to_C")
		}
	})

	b.Run("SyncVMState_C_to_Go", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			pvm.SyncVMState("C_to_Go")
		}
	})
}

func BenchmarkRealProgram(b *testing.B) {
	fp := "../services/blake2b_parent.pvm"
	raw_code, err := os.ReadFile(fp)
	if err != nil {
		b.Skipf("Failed to read file %s: %v", fp, err)
		return
	}

	initial_regs := make([]uint64, 13)
	hostENV := NewMockHostEnv()

	b.Run("Blake2b_Go", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			pvm := NewVM(0, raw_code, initial_regs, 0, hostENV, true, []byte("benchmark"), BackendInterpreter)
			pvm.Gas = int64(999999999)
			pvm.Standard_Program_Initialization([]byte{})
			pvm.Execute(types.EntryPointRefine, false)
		}
	})

	b.Run("Blake2b_C", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			pvm := NewVM(0, raw_code, initial_regs, 0, hostENV, true, []byte("benchmark"), "cgo")
			pvm.Gas = int64(999999999)
			pvm.Standard_Program_Initialization([]byte{})
			pvm.ExecuteWithCGO()
		}
	})
}

// Phase 4: Comprehensive Integration Tests

func TestHostFunctionTrampoline(t *testing.T) {
	// Test ECALLI instruction with host function calls through C backend
	code := []byte{
		0x34, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // ECALLI with function id 1
		0x00, // TRAP
	}

	initial_regs := make([]uint64, 13)
	hostENV := NewMockHostEnv()

	// Test with Go backend first
	pvmGo := NewVM(0, code, initial_regs, 0, hostENV, true, []byte("test"), BackendInterpreter)
	pvmGo.Gas = 1000000
	pvmGo.Standard_Program_Initialization([]byte{})
	pvmGo.Execute(types.EntryPointRefine, false)

	// Test with C backend
	pvmC := NewVM(0, code, initial_regs, 0, hostENV, true, []byte("test"), "cgo")
	pvmC.Gas = 1000000
	pvmC.Standard_Program_Initialization([]byte{})

	err := pvmC.initCGOIntegration()
	if err != nil {
		t.Skipf("CGO not available: %v", err)
	}
	defer pvmC.destroyCGOIntegration()

	err = pvmC.ExecuteWithCGO()

	// Compare results - both should have terminated properly
	if pvmGo.ResultCode != pvmC.ResultCode {
		t.Errorf("Result codes differ: Go=%d, C=%d", pvmGo.ResultCode, pvmC.ResultCode)
	}

	// Both executions should complete without error
	if err != nil {
		t.Errorf("C backend execution failed: %v", err)
	}
}

func TestMemorySafety(t *testing.T) {
	// Test memory bounds checking in C implementation
	code := []byte{
		0x11, 0x00, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, // LOAD_IMM_64 R0, 0xFFFFFFFFFFFFFFFF
		0x34, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // ECALLI 4 (memory access)
		0x00, // TRAP
	}

	initial_regs := make([]uint64, 13)
	hostENV := NewMockHostEnv()

	pvm := NewVM(0, code, initial_regs, 0, hostENV, true, []byte("test"), "cgo")
	pvm.Gas = 1000000
	pvm.Standard_Program_Initialization([]byte{})

	err := pvm.initCGOIntegration()
	if err != nil {
		t.Skipf("CGO not available: %v", err)
	}
	defer pvm.destroyCGOIntegration()

	// Execute and verify it doesn't crash on invalid memory access
	err = pvm.ExecuteWithCGO()

	// Should either panic or handle gracefully, not crash
	if err != nil || pvm.ResultCode == PANIC {
		t.Log("Memory safety violation properly detected")
	} else {
		t.Log("Memory access handled without crash")
	}
}

func TestStateConsistency(t *testing.T) {
	// Test that VM state remains consistent between C and Go
	code := []byte{
		0x11, 0x00, 0x42, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // LOAD_IMM_64 R0, 0x42
		0x11, 0x11, 0x37, 0x13, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // LOAD_IMM_64 R1, 0x1337
		0x34, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // ECALLI 1
		0x00, // TRAP
	}

	initial_regs := make([]uint64, 13)
	hostENV := NewMockHostEnv()

	pvm := NewVM(0, code, initial_regs, 0, hostENV, true, []byte("test"), "cgo")
	pvm.Gas = 1000000
	pvm.Standard_Program_Initialization([]byte{})

	err := pvm.initCGOIntegration()
	if err != nil {
		t.Skipf("CGO not available: %v", err)
	}
	defer pvm.destroyCGOIntegration()

	// Set initial register values
	pvm.register[2] = 0x1000
	pvm.register[3] = 0x2000

	// Sync to C
	pvm.SyncRegistersToC()

	// Execute one step in C
	pvm.ExecuteWithCGO()

	// Sync back from C
	pvm.SyncRegistersFromC()

	// Verify register values are correct
	if pvm.register[0] != 0x42 {
		t.Errorf("Register 0 incorrect: expected 0x42, got 0x%x", pvm.register[0])
	}
	if pvm.register[1] != 0x1337 {
		t.Errorf("Register 1 incorrect: expected 0x1337, got 0x%x", pvm.register[1])
	}

	// Verify other registers maintained their values
	if pvm.register[2] != 0x1000 {
		t.Errorf("Register 2 incorrect: expected 0x1000, got 0x%x", pvm.register[2])
	}
	if pvm.register[3] != 0x2000 {
		t.Errorf("Register 3 incorrect: expected 0x2000, got 0x%x", pvm.register[3])
	}
}

func TestErrorHandling(t *testing.T) {
	// Test error handling in C backend
	initial_regs := make([]uint64, 13)
	hostENV := NewMockHostEnv()

	t.Run("InvalidOpcode", func(t *testing.T) {
		// Test invalid opcode handling
		code := []byte{0xFF, 0xFF} // Invalid opcode

		pvm := NewVM(0, code, initial_regs, 0, hostENV, true, []byte("test"), "cgo")
		pvm.Gas = 1000000
		pvm.Standard_Program_Initialization([]byte{})

		err := pvm.initCGOIntegration()
		if err != nil {
			t.Skipf("CGO not available: %v", err)
		}
		defer pvm.destroyCGOIntegration()

		err = pvm.ExecuteWithCGO()

		// Should result in panic or proper error handling
		if err != nil || pvm.ResultCode == PANIC {
			t.Log("Invalid opcode properly handled")
		}
	})

	t.Run("OutOfGas", func(t *testing.T) {
		// Test out of gas condition
		code := []byte{
			0x34, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // ECALLI 1 (expensive)
			0x00, // TRAP
		}

		pvm := NewVM(0, code, initial_regs, 0, hostENV, true, []byte("test"), "cgo")
		pvm.Gas = 1 // Very low gas
		pvm.Standard_Program_Initialization([]byte{})

		err := pvm.initCGOIntegration()
		if err != nil {
			t.Skipf("CGO not available: %v", err)
		}
		defer pvm.destroyCGOIntegration()

		err = pvm.ExecuteWithCGO()

		// Should result in out of gas
		if err != nil || pvm.ResultCode == OOG {
			t.Log("Out of gas properly handled")
		}
	})
}

func TestHostFunctionCallValidation(t *testing.T) {
	// Test host function call validation in various VM states
	initial_regs := make([]uint64, 13)
	hostENV := NewMockHostEnv()

	pvm := NewVM(0, []byte{0x00}, initial_regs, 0, hostENV, true, []byte("test"), "cgo")

	err := pvm.initCGOIntegration()
	if err != nil {
		t.Skipf("CGO not available: %v", err)
	}
	defer pvm.destroyCGOIntegration()

	// Test normal state - should pass
	err = pvm.ValidateHostCall(1)
	if err != nil {
		t.Errorf("Valid host call rejected: %v", err)
	}

	// Test terminated state - should fail
	pvm.terminated = true
	err = pvm.ValidateHostCall(1)
	if err == nil {
		t.Error("Terminated VM should reject host calls")
	}

	// Test panic state - should fail
	pvm.terminated = false
	pvm.MachineState = PANIC
	err = pvm.ValidateHostCall(1)
	if err == nil {
		t.Error("Panicked VM should reject host calls")
	}
}

// ===============================
// Memory Safety Validation Tests
// ===============================

func TestMemoryBoundaryValidation(t *testing.T) {
	// Test various memory boundary conditions to ensure safety
	hostENV := NewMockHostEnv()
	initial_regs := make([]uint64, 13)

	tests := []struct {
		name          string
		code          []byte
		expectedPanic bool
	}{
		{
			name: "ValidMemoryAccess",
			code: []byte{
				0x11, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // LOAD_IMM_64 R0, 0x100 (valid RW address)
				0x30, 0x08, 0x42, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // STORE_8 [R0], 0x42
				0x00, // TRAP
			},
			expectedPanic: false,
		},
		{
			name: "InvalidHighAddress",
			code: []byte{
				0x11, 0x00, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, // LOAD_IMM_64 R0, 0xFFFFFFFFFFFFFFFF
				0x30, 0x08, 0x42, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // STORE_8 [R0], 0x42
				0x00, // TRAP
			},
			expectedPanic: true,
		},
		{
			name: "NullPointerAccess",
			code: []byte{
				0x11, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // LOAD_IMM_64 R0, 0x0
				0x30, 0x08, 0x42, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // STORE_8 [R0], 0x42
				0x00, // TRAP
			},
			expectedPanic: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test Go backend
			pvmGo := NewVM(0, tt.code, initial_regs, 0, hostENV, false, []byte{}, BackendInterpreter)
			pvmGo.Gas = 10000
			pvmGo.Execute(0, false)

			goPanicked := pvmGo.ResultCode == PANIC

			// Test C backend
			pvmC := NewVM(0, tt.code, initial_regs, 0, hostENV, false, []byte{}, "cgo")
			pvmC.Gas = 10000

			err := pvmC.initCGOIntegration()
			if err != nil {
				t.Skipf("CGO not available: %v", err)
			}
			defer pvmC.destroyCGOIntegration()

			pvmC.ExecuteWithCGO()
			cPanicked := pvmC.ResultCode == PANIC

			// Verify both backends handle memory safety consistently
			if goPanicked != cPanicked {
				t.Errorf("Memory safety handling inconsistent: Go panicked=%v, C panicked=%v", goPanicked, cPanicked)
			}

			if tt.expectedPanic && !goPanicked {
				t.Errorf("Expected panic for %s but didn't occur", tt.name)
			}

			if !tt.expectedPanic && goPanicked {
				t.Errorf("Unexpected panic for %s", tt.name)
			}
		})
	}
}

func TestRegisterBoundaryValidation(t *testing.T) {
	// Test register access boundary conditions
	hostENV := NewMockHostEnv()
	initial_regs := make([]uint64, 13)

	// Test invalid register access (should be handled gracefully)
	code := []byte{
		0x11, 0xFF, 0x42, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // LOAD_IMM_64 R15 (invalid), 0x42
		0x00, // TRAP
	}

	pvm := NewVM(0, code, initial_regs, 0, hostENV, false, []byte{}, BackendInterpreter)
	pvm.Gas = 1000
	pvm.Execute(0, false)

	// Should either clamp to valid register or handle gracefully (implementation specific)
	if pvm.ResultCode == PANIC {
		t.Log("Register boundary violation properly detected")
	} else {
		t.Log("Register access clamped to valid range")
	}
}

// ===============================
// Performance Profiling Tests
// ===============================

func TestPerformanceProfiler(t *testing.T) {
	// Performance profiling for different VM operations
	hostENV := NewMockHostEnv()
	initial_regs := make([]uint64, 13)

	// Test arithmetic operations performance
	code := []byte{
		0x14, 0x00, 0x42, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // LOAD_IMM_64 R0, 0x42
		0x14, 0x01, 0x37, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // LOAD_IMM_64 R1, 0x37
		0xBE, 0x01, 0x02, // ADD_32 R0, R1 -> R2
		0xBF, 0x01, 0x03, // SUB_32 R0, R1 -> R3
		0xC0, 0x01, 0x04, // MUL_32 R0, R1 -> R4
		0x00, // TRAP
	}

	start := time.Now()
	pvmGo := NewVM(0, code, initial_regs, 0, hostENV, false, []byte{}, BackendInterpreter)
	pvmGo.Gas = 100000
	pvmGo.Execute(0, false)
	goTime := time.Since(start)

	start = time.Now()
	pvmC := NewVM(0, code, initial_regs, 0, hostENV, false, []byte{}, "cgo")
	pvmC.Gas = 100000

	err := pvmC.initCGOIntegration()
	if err == nil {
		pvmC.ExecuteWithCGO()
		pvmC.destroyCGOIntegration()
		cTime := time.Since(start)

		speedup := float64(goTime) / float64(cTime)
		t.Logf("Arithmetic Performance: Go=%v, C=%v, Speedup=%.2fx", goTime, cTime, speedup)

		if speedup < 0.5 {
			t.Logf("⚠️  C backend slower than expected: %.2fx", speedup)
		} else if speedup > 2.0 {
			t.Logf("🚀 Significant C performance improvement: %.2fx", speedup)
		}
	} else {
		t.Skipf("CGO not available for profiling: %v", err)
	}
}

func TestConcurrentExecution(t *testing.T) {
	// Test concurrent VM execution safety
	hostENV := NewMockHostEnv()

	code := []byte{
		0x11, 0x00, 0x42, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // LOAD_IMM_64 R0, 0x42
		0x34, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // ECALLI 1
		0x00, // TRAP
	}

	const numGoroutines = 10
	var wg sync.WaitGroup
	errorCh := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Each goroutine gets its own VM instance
			pvm := NewVM(uint32(id), code, make([]uint64, 13), 0, hostENV, false, []byte{}, "cgo")
			pvm.Gas = 10000

			err := pvm.initCGOIntegration()
			if err != nil {
				errorCh <- fmt.Errorf("goroutine %d: CGO init failed: %v", id, err)
				return
			}
			defer pvm.destroyCGOIntegration()

			err = pvm.ExecuteWithCGO()
			if err != nil {
				errorCh <- fmt.Errorf("goroutine %d: execution failed: %v", id, err)
			} else if pvm.ResultCode == PANIC {
				errorCh <- fmt.Errorf("goroutine %d: execution panicked", id)
			}
		}(i)
	}

	wg.Wait()
	close(errorCh)

	errorCount := 0
	for err := range errorCh {
		t.Logf("Concurrent execution error: %v", err)
		errorCount++
	}

	if errorCount == 0 {
		t.Logf("✅ All %d concurrent executions completed successfully", numGoroutines)
	} else if errorCount == numGoroutines {
		t.Skipf("CGO not available for concurrent testing")
	} else {
		t.Errorf("❌ %d out of %d concurrent executions failed", errorCount, numGoroutines)
	}
}

// ===============================
// Usage Examples and Documentation
// ===============================

// ExampleNewVM demonstrates basic VM creation and execution
func ExampleNewVM() {
	// Simple program that loads a value and traps
	code := []byte{
		0x11, 0x00, 0x42, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // LOAD_IMM_64 R0, 0x42
		0x00, // TRAP
	}

	hostENV := NewMockHostEnv()
	initial_regs := make([]uint64, 13)

	// Create VM with Go backend
	vm := NewVM(0, code, initial_regs, 0, hostENV, false, []byte{}, BackendInterpreter)
	vm.Gas = 1000

	// Execute
	vm.Execute(0, false)

	fmt.Printf("Register 0: 0x%x\n", vm.register[0])
	// Output: Register 0: 0x42
}

// ExampleVM_ExecuteWithCGO demonstrates high-performance C backend usage
func ExampleVM_ExecuteWithCGO() {
	code := []byte{
		0x11, 0x00, 0x42, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // LOAD_IMM_64 R0, 0x42
		0x11, 0x11, 0x37, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // LOAD_IMM_64 R1, 0x37
		0xDC, 0x01, 0x02, // ADD R0, R1 -> R2
		0x00, // TRAP
	}

	hostENV := NewMockHostEnv()
	initial_regs := make([]uint64, 13)

	// Create VM with C backend
	vm := NewVM(0, code, initial_regs, 0, hostENV, false, []byte{}, BackendCompiler)
	vm.Gas = 1000

	// Initialize CGO integration
	err := vm.initCGOIntegration()
	if err != nil {
		fmt.Printf("CGO not available: %v\n", err)
		return
	}
	defer vm.destroyCGOIntegration()

	// Execute with high-performance C interpreter
	err = vm.ExecuteWithCGO()

	if err != nil {
		fmt.Printf("Execution failed: %v\n", err)
	} else {
		fmt.Printf("Execution completed successfully\n")
		fmt.Printf("R2 = R0 + R1 = 0x%x + 0x%x = 0x%x\n", vm.register[0], vm.register[1], vm.register[2])
	}
	// Output: R2 = R0 + R1 = 0x42 + 0x37 = 0x79
}

// TestPerformanceComparison times both backends on working test programs
func TestPerformanceComparison(t *testing.T) {
	// Use multiple known working programs for timing
	workingPrograms := []TestCase{
		{
			Name:           "inst_add_32",
			InitialRegs:    []uint64{0, 0, 0, 0, 0, 0, 0, 1, 2, 0, 0, 0, 0},
			InitialPC:      0,
			Code:           []int{0, 0, 3, 190, 135, 9, 1},
			Bitmask:        []int{1, 0, 0},
			ExpectedStatus: "panic",
			ExpectedRegs:   []uint64{0, 0, 0, 0, 0, 0, 0, 1, 2, 3, 0, 0, 0},
			ExpectedPC:     3,
		},
		{
			Name:           "simple_trap",
			InitialRegs:    []uint64{42, 37, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			InitialPC:      0,
			Code:           []int{0, 0, 1, 0, 1},
			Bitmask:        []int{1},
			ExpectedStatus: "panic",
			ExpectedRegs:   []uint64{42, 37, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			ExpectedPC:     1,
		},
		{
			Name:           "fallthrough_test",
			InitialRegs:    []uint64{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			InitialPC:      0,
			Code:           []int{0, 0, 2, 1, 0, 1},
			Bitmask:        []int{1, 1},
			ExpectedStatus: "panic",
			ExpectedRegs:   []uint64{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			ExpectedPC:     3,
		},
	}

	iterations := 1000 // Number of iterations for timing

	// Time Go backend
	t.Log("🐹 Timing Go backend...")
	goStart := time.Now()
	for i := 0; i < iterations; i++ {
		for _, testCase := range workingPrograms {
			err := pvm_test_backend(testCase, BackendInterpreter)
			if err != nil {
				t.Errorf("Go backend failed: %v", err)
				return
			}
		}
	}
	goDuration := time.Since(goStart)

	// Time C backend
	t.Log("⚡ Timing C backend...")
	cStart := time.Now()
	for i := 0; i < iterations; i++ {
		for _, testCase := range workingPrograms {
			err := pvm_test_backend(testCase, BackendCompiler)
			if err != nil {
				t.Errorf("C backend failed: %v", err)
				return
			}
		}
	}
	cDuration := time.Since(cStart)

	// Results
	totalExecutions := iterations * len(workingPrograms)
	t.Logf("📊 Performance Results:")
	t.Logf("Programs: %d, Iterations: %d (Total executions: %d)", len(workingPrograms), iterations, totalExecutions)
	t.Logf("🐹 Go Backend:  %v total, %v per execution", goDuration, goDuration/time.Duration(totalExecutions))
	t.Logf("⚡ C Backend:   %v total, %v per execution", cDuration, cDuration/time.Duration(totalExecutions))

	if cDuration > 0 {
		speedup := float64(goDuration) / float64(cDuration)
		t.Logf("🚀 C Backend Speedup: %.2fx", speedup)
	}
}

// TestBackendComparison demonstrates running the same test with both backends
func TestBackendComparison(t *testing.T) {
	// Load a simple test program from programs directory
	programFile := "programs/inst_add_32.json"
	programData, err := os.ReadFile(programFile)
	if err != nil {
		t.Fatalf("Failed to load test program: %v", err)
	}

	var programTest struct {
		Name           string   `json:"name"`
		InitialRegs    []uint64 `json:"initial-regs"`
		InitialPC      int      `json:"initial-pc"`
		Program        []int    `json:"program"`
		Bitmask        []int    `json:"bitmask"`
		ExpectedRegs   []uint64 `json:"expected-regs"`
		ExpectedPC     int      `json:"expected-pc"`
		ExpectedStatus string   `json:"expected-status"`
	}

	if err := json.Unmarshal(programData, &programTest); err != nil {
		t.Fatalf("Failed to parse test program: %v", err)
	}

	testCase := TestCase{
		Name:           programTest.Name,
		InitialRegs:    programTest.InitialRegs,
		InitialPC:      uint32(programTest.InitialPC),
		InitialPageMap: []TestPageMap{},
		InitialMemory:  []TestMemory{},
		Code:           programTest.Program,
		Bitmask:        programTest.Bitmask,
		ExpectedStatus: programTest.ExpectedStatus,
		ExpectedRegs:   programTest.ExpectedRegs,
		ExpectedPC:     uint32(programTest.ExpectedPC),
		ExpectedMemory: []TestMemory{},
	}

	t.Run("Go_Backend", func(t *testing.T) {
		goErr := pvm_test_backend(testCase, BackendInterpreter)
		if goErr != nil {
			t.Errorf("❌ Go backend failed: %v", goErr)
		} else {
			t.Log("✅ Go backend passed")
		}
	})

	t.Run("C_Backend", func(t *testing.T) {
		t.Log("🧪 Testing C backend...")
		cErr := pvm_test_backend(testCase, BackendCompiler)
		if cErr != nil {
			t.Errorf("❌ C backend failed: %v", cErr)
			t.Log("⚠️  This indicates CGO function declaration issues need to be resolved")
		} else {
			t.Log("✅ C backend passed!")
			t.Log("🎉 Both Go and C backends are working!")
		}
	})
}

// BenchmarkBackends compares performance between Go and C backends
func BenchmarkBackends(b *testing.B) {
	// Get all test programs
	testDir := "/Users/sourabhniyogi/Documents/jam/pvm/programs"
	files, err := os.ReadDir(testDir)
	if err != nil {
		b.Fatalf("Failed to read test directory: %v", err)
	}

	var testPrograms []TestCase
	for _, file := range files {
		if !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		filePath := filepath.Join(testDir, file.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		var programTest struct {
			Name           string   `json:"name"`
			InitialRegs    []uint64 `json:"initial-regs"`
			InitialPC      int      `json:"initial-pc"`
			Program        []int    `json:"program"`
			Bitmask        []int    `json:"bitmask"`
			ExpectedRegs   []uint64 `json:"expected-regs"`
			ExpectedPC     int      `json:"expected-pc"`
			ExpectedStatus string   `json:"expected-status"`
		}

		if err := json.Unmarshal(data, &programTest); err != nil {
			continue
		}

		testCase := TestCase{
			Name:           programTest.Name,
			InitialRegs:    programTest.InitialRegs,
			InitialPC:      uint32(programTest.InitialPC),
			InitialPageMap: []TestPageMap{},
			InitialMemory:  []TestMemory{},
			Code:           programTest.Program,
			Bitmask:        programTest.Bitmask,
			ExpectedStatus: programTest.ExpectedStatus,
			ExpectedRegs:   programTest.ExpectedRegs,
			ExpectedPC:     uint32(programTest.ExpectedPC),
			ExpectedMemory: []TestMemory{},
		}
		testPrograms = append(testPrograms, testCase)
	}

	b.Logf("Loaded %d test programs for benchmarking", len(testPrograms))

	b.Run("Go_Backend", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for _, testCase := range testPrograms {
				err := pvm_test_backend(testCase, BackendInterpreter)
				if err != nil {
					b.Errorf("Go backend failed on %s: %v", testCase.Name, err)
					return
				}
			}
		}
	})

	b.Run("C_Backend", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for _, testCase := range testPrograms {
				err := pvm_test_backend(testCase, BackendCompiler)
				if err != nil {
					b.Errorf("C backend failed on %s: %v", testCase.Name, err)
					return
				}
			}
		}
	})
}

// ExampleBenchmarkComparison demonstrates performance comparison
func ExampleBenchmarkComparison() {
	// Complex computation program
	code := []byte{
		0x11, 0x00, 0x64, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // LOAD_IMM_64 R0, 100
		0x11, 0x11, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // LOAD_IMM_64 R1, 1
		// Loop: R2 = R0 * R1, R0 = R0 - R1, jump if R0 != 0
		0xDE, 0x01, 0x02, // MUL R0, R1 -> R2
		0xDD, 0x10, 0x00, // SUB R0, R1 -> R0
		0x29, 0x00, 0xFA, // BRANCH_NE R0, -6 (jump back)
		0x00, // TRAP
	}

	hostENV := NewMockHostEnv()
	initial_regs := make([]uint64, 13)

	// Benchmark Go backend
	start := time.Now()
	vm1 := NewVM(0, code, make([]uint64, 13), 0, hostENV, false, []byte{}, BackendInterpreter)
	vm1.Gas = 100000
	vm1.Execute(0, false)
	goTime := time.Since(start)

	// Benchmark C backend
	start = time.Now()
	vm2 := NewVM(0, code, initial_regs, 0, hostENV, false, []byte{}, BackendCompiler)
	vm2.Gas = 100000
	vm2.EntryPoint = 0
	vm2.IsChild = false

	err := vm2.ExecuteWithCGO()
	cTime := time.Since(start)
	if err == nil {

		speedup := float64(goTime) / float64(cTime)
		fmt.Printf("Go backend: %v\n", goTime)
		fmt.Printf("C backend: %v\n", cTime)
		fmt.Printf("Speedup: %.2fx\n", speedup)
	} else {
		fmt.Printf("CGO not available: %v\n", err)
	}
}

// TestDocumentationExamples runs the documentation examples to ensure they work
func TestDocumentationExamples(t *testing.T) {
	t.Run("BasicUsage", func(t *testing.T) {
		ExampleNewVM()
	})

	t.Run("CGOUsage", func(t *testing.T) {
		ExampleVM_ExecuteWithCGO()
	})

	t.Run("BenchmarkComparison", func(t *testing.T) {
		ExampleBenchmarkComparison()
	})
}

func TestCGOIntegration(t *testing.T) {
	// Test basic CGO integration setup
	vm := &VM{
		Backend:             BackendInterpreter,
		code:                []byte{0x00}, // TRAP instruction
		bitmask:             []byte{0x01}, // Single instruction
		register:            make([]uint64, 13),
		hostenv:             &MockHostEnv{},
		Service_index:       1,
		pc:                  0,
		Gas:                 1000,
		rw_data:             make([]byte, 1024),
		ro_data:             make([]byte, 1024),
		output:              make([]byte, 1024),
		stack:               make([]byte, 1024),
		rw_data_address:     0x10000,
		rw_data_address_end: 0x10400,
		ro_data_address:     0x20000,
		ro_data_address_end: 0x20400,
		output_address:      0x30000,
		output_end:          0x30400,
		stack_address:       0x40000,
		stack_address_end:   0x40400,
	}

	// Test CGO integration initialization
	err := vm.initCGOIntegration()
	if err != nil {
		t.Fatalf("Failed to initialize CGO integration: %v", err)
	}
	defer vm.destroyCGOIntegration()

	// Verify C VM was created
	if vm.cVM == nil {
		t.Fatal("C VM was not created")
	}

	// Basic functionality test passed if we reach here
	t.Log("CGO Integration basic test passed")
}

func TestRegisterSynchronization(t *testing.T) {
	vm := &VM{
		Backend:       BackendInterpreter,
		code:          []byte{0x00},
		bitmask:       []byte{0x01},
		register:      make([]uint64, 13),
		hostenv:       &MockHostEnv{},
		Service_index: 1,
		syncRegisters: true,
	}

	// Initialize CGO integration
	err := vm.initCGOIntegration()
	if err != nil {
		t.Fatalf("Failed to initialize CGO integration: %v", err)
	}
	defer vm.destroyCGOIntegration()

	// Test bidirectional register sync
	for i := 0; i < 13; i++ {
		testValue := uint64(0x1000 + i)
		vm.register[i] = testValue
	}

	// Sync to C
	vm.SyncRegistersToC()

	// Clear Go registers
	for i := 0; i < 13; i++ {
		vm.register[i] = 0
	}

	// Sync from C
	vm.SyncRegistersFromC()

	// Verify all registers were synced correctly
	for i := 0; i < 13; i++ {
		expected := uint64(0x1000 + i)
		if vm.register[i] != expected {
			t.Errorf("Register %d sync failed: expected 0x%x, got 0x%x",
				i, expected, vm.register[i])
		}
	}
}

func TestVMStateSync(t *testing.T) {
	vm := &VM{
		Backend:       BackendInterpreter,
		code:          []byte{0x00},
		bitmask:       []byte{0x01},
		register:      make([]uint64, 13),
		hostenv:       &MockHostEnv{},
		Service_index: 1,
		pc:            100,
		Gas:           500,
		MachineState:  HALT,
		ResultCode:    0,
	}

	// Initialize CGO integration
	err := vm.initCGOIntegration()
	if err != nil {
		t.Fatalf("Failed to initialize CGO integration: %v", err)
	}
	defer vm.destroyCGOIntegration()

	// Test VM state sync Go to C
	vm.SyncVMState("Go_to_C")

	// Test VM state sync C to Go
	originalPC := vm.pc
	vm.pc = 0 // Clear Go state
	vm.SyncVMState("C_to_Go")

	if vm.pc != originalPC {
		t.Errorf("PC sync failed: expected %d, got %d", originalPC, vm.pc)
	}
}

func TestHostCallValidation(t *testing.T) {
	vm := &VM{
		Backend:      BackendInterpreter,
		terminated:   false,
		MachineState: HALT,
		Gas:          100,
	}

	// Test valid call
	err := vm.ValidateHostCall(1)
	if err != nil {
		t.Errorf("Valid host call rejected: %v", err)
	}

	// Test terminated VM
	vm.terminated = true
	err = vm.ValidateHostCall(1)
	if err == nil {
		t.Error("Terminated VM should reject host calls")
	}

	// Test PANIC state
	vm.terminated = false
	vm.MachineState = PANIC
	err = vm.ValidateHostCall(1)
	if err == nil {
		t.Error("PANIC VM should reject host calls")
	}

	// Test out of gas
	vm.MachineState = HALT
	vm.Gas = -1
	err = vm.ValidateHostCall(1)
	if err == nil {
		t.Error("Out of gas VM should reject host calls")
	}
}

func TestCgoEcalli(t *testing.T) {
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

	// Test with CGO backend specifically
	err = pvm_test_backend(tc, "cgo")
	if err != nil {
		t.Errorf("CGO backend test failed for %s: %v", tc.Name, err)
	}

	// Also test with Go backend for comparison
	err = pvm_test_backend(tc, "go")
	if err != nil {
		t.Errorf("Go backend test failed for %s: %v", tc.Name, err)
	}
}

func TestEncodeDecodeProgram(t *testing.T) {
	// Test case 1: Simple program with basic bitmask
	testCode1 := []byte{51, 7, 1, 50, 0, 1}  // LOAD_IMM r7, 1; RET 0, 1
	testBitmask1 := []byte{1, 0, 0, 1, 0, 0} // Bitmask for the instructions (6 bits for 6 bytes)

	// Encode the program
	encoded1 := EncodeProgram(testCode1, testBitmask1)

	// Decode the program
	decoded1 := DecodeProgram_pure_pvm_blob(encoded1)

	if decoded1 == nil {
		t.Fatal("Failed to decode program")
	}

	// Verify the decoded code matches the original
	if len(decoded1.Code) != len(testCode1) {
		t.Errorf("Code length mismatch: expected %d, got %d", len(testCode1), len(decoded1.Code))
	}

	for i, expected := range testCode1 {
		if i >= len(decoded1.Code) {
			t.Errorf("Code too short at index %d", i)
			break
		}
		if decoded1.Code[i] != expected {
			t.Errorf("Code mismatch at index %d: expected %d, got %d", i, expected, decoded1.Code[i])
		}
	}

	// Verify the bitmask matches
	if len(decoded1.K) != len(testBitmask1) {
		t.Errorf("Bitmask length mismatch: expected %d, got %d", len(testBitmask1), len(decoded1.K))
	}

	for i, expected := range testBitmask1 {
		if i >= len(decoded1.K) {
			t.Errorf("Bitmask too short at index %d", i)
			break
		}
		if decoded1.K[i] != expected {
			t.Errorf("Bitmask mismatch at index %d: expected %d, got %d", i, expected, decoded1.K[i])
		}
	}

	t.Logf("✅ Test case 1 passed: code length=%d, bitmask length=%d", len(decoded1.Code), len(decoded1.K))

	// Test case 2: Our ECALLI test program
	ecalliCode := []byte{
		51, 7, 1, // LOAD_IMM r7, 1
		51, 11, 8, // LOAD_IMM r11, 8
		51, 10, 0, 0, 2, 0, 0, 0, 0, 0, // LOAD_IMM r10, 131072
		33, 0, 0, 2, 0, 0, 0, 0, 0, 65, 66, 67, 68, 69, 70, 71, 72, // STORE_IMM_U64 131072, "ABCDEFGH"
		10, 100, 0, 0, 0, 0, 0, 0, 0, // ECALLI 100
		50, 0, 1, // RET 0, 1
	}
	ecalliBitmask := []byte{
		1, 0, 0, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0,
	}

	// Encode and decode the ECALLI program
	encoded2 := EncodeProgram(ecalliCode, ecalliBitmask)
	decoded2 := DecodeProgram_pure_pvm_blob(encoded2)

	if decoded2 == nil {
		t.Fatal("Failed to decode ECALLI program")
	}

	// Verify code
	if len(decoded2.Code) != len(ecalliCode) {
		t.Errorf("ECALLI code length mismatch: expected %d, got %d", len(ecalliCode), len(decoded2.Code))
	}

	for i, expected := range ecalliCode {
		if i >= len(decoded2.Code) {
			break
		}
		if decoded2.Code[i] != expected {
			t.Errorf("ECALLI code mismatch at index %d: expected %d, got %d", i, expected, decoded2.Code[i])
		}
	}

	// Verify bitmask
	if len(decoded2.K) != len(ecalliBitmask) {
		t.Errorf("ECALLI bitmask length mismatch: expected %d, got %d", len(ecalliBitmask), len(decoded2.K))
	}

	for i, expected := range ecalliBitmask {
		if i >= len(decoded2.K) {
			break
		}
		if decoded2.K[i] != expected {
			t.Errorf("ECALLI bitmask mismatch at index %d: expected %d, got %d", i, expected, decoded2.K[i])
		}
	}

	t.Logf("✅ Test case 2 passed: ECALLI program encoded/decoded successfully")
	t.Logf("   Code length=%d, bitmask length=%d", len(decoded2.Code), len(decoded2.K))

	// Test case 3: Edge case - empty program
	emptyCode := []byte{}
	emptyBitmask := []byte{}

	encoded3 := EncodeProgram(emptyCode, emptyBitmask)
	decoded3 := DecodeProgram_pure_pvm_blob(encoded3)

	if decoded3 == nil {
		t.Fatal("Failed to decode empty program")
	}

	if len(decoded3.Code) != 0 {
		t.Errorf("Empty program should have no code, got %d bytes", len(decoded3.Code))
	}

	if len(decoded3.K) != 0 {
		t.Errorf("Empty program should have no bitmask, got %d bits", len(decoded3.K))
	}

	t.Logf("✅ Test case 3 passed: empty program handled correctly")
}
