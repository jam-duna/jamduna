// run test: go test ./pvm -v
package pvm

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
)

const (
	show_test_case = false
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
	Code           []byte        `json:"program"`
	ExpectedStatus string        `json:"expected-status"`
	ExpectedRegs   []uint64      `json:"expected-regs"`
	ExpectedPC     uint32        `json:"expected-pc"`
	ExpectedMemory []TestMemory  `json:"expected-memory"`
}

func pvm_test(tc TestCase) (error, int) {
	var num_mismatch int
	fmt.Printf("Test case: %s\n", tc.Name)

	// if tc.Name != "inst_div_signed_64" {
	// 	return nil, 0
	// }

	hostENV := NewMockHostEnv()
	serviceAcct := uint32(0) // stub
	// metadata, c := types.SplitMetadataAndCode(tc.Code)
	pvm := NewVM(serviceAcct, tc.Code, tc.InitialRegs, uint64(tc.InitialPC), hostENV, false, []byte{})
	// Set the initial memory
	for _, mem := range tc.InitialMemory {
		pvm.Ram.SetPageAccess(mem.Address/PageSize, 1, AccessMode{Readable: false, Writable: true, Inaccessible: false})
		pvm.Ram.WriteRAMBytes(mem.Address, mem.Data[:])
	}
	// Set the initial page map
	for _, page := range tc.InitialPageMap {
		pvm.Ram.SetPageAccess(page.Address/PageSize, page.Length/PageSize, AccessMode{Readable: !page.IsWritable, Writable: page.IsWritable, Inaccessible: false})
	}

	// if len(tc.InitialMemory) == 0 {
	// 	pvm.Ram.SetPageAccess(32, 1, AccessMode{Readable: false, Writable: false, Inaccessible: true})
	// }

	pvm.Execute(int(tc.InitialPC), false)
	// Check the registers
	if equalIntSlices(pvm.register, tc.ExpectedRegs) {
		fmt.Printf("Register match for test %s \n", tc.Name)
	} else {
		fmt.Printf("Register mismatch for test %s: expected %v, got %v \n", tc.Name, tc.ExpectedRegs, pvm.register)
		num_mismatch++
	}

	// t.Log("pvm_test")
	/*
		// Check the status
			if status != testCase.ExpectedStatus {
				//t.Errorf("Status mismatch for test %s: expected %s, got %s", testCase.Name, testCase.ExpectedStatus, status)
			}


			// Check the program counter
			if pc != testCase.ExpectedPC {
				//t.Errorf("Program counter mismatch for test %s: expected %d, got %d", testCase.Name, testCase.ExpectedPC, pc)
			}

			// Check the memory
			if !equalInterfaceSlices(memory, testCase.ExpectedMemory) {
				//t.Errorf("Memory mismatch for test %s: expected %v, got %v", testCase.Name, testCase.ExpectedMemory, memory)
			}
	*/
	return nil, num_mismatch // "trap", tc.InitialRegs, tc.InitialPC, tc.InitialMemory
}

func TestReadWriteRAM(t *testing.T) {
	ram := NewRAM()

	// Test page allocation
	pageIndex := uint32(0)
	page, err := ram.getOrAllocatePage(pageIndex)
	if err != nil {
		t.Fatalf("Failed to allocate page: %v", err)
	}
	if page == nil {
		t.Fatalf("Page allocation returned nil")
	}

	// Test access mode setup
	accessMode := AccessMode{
		Inaccessible: false,
		Writable:     true,
		Readable:     true,
	}
	ram.SetPageAccess(pageIndex, 1, accessMode)
	allocatedPage, err := ram.getOrAllocatePage(pageIndex)
	if err != nil {
		t.Fatalf("Failed to retrieve allocated page: %v", err)
	}
	if !allocatedPage.Access.Readable || !allocatedPage.Access.Writable || allocatedPage.Access.Inaccessible {
		t.Fatalf("Access mode was not set correctly")
	}

	// Test data writing
	dataToWrite := []byte("hello")
	address := uint32(0)
	result := ram.WriteRAMBytes(address, dataToWrite)
	if result != OK {
		t.Fatalf("Failed to write data to RAM: result code %d", result)
	}

	// Test data reading
	readData, result := ram.ReadRAMBytes(address, uint32(len(dataToWrite)))
	if result != OK {
		t.Fatalf("Failed to read data from RAM: result code %d", result)
	}
	if string(readData) != string(dataToWrite) {
		t.Fatalf("Data mismatch: expected %s, got %s", string(dataToWrite), string(readData))
	}
}

// awaiting 64 bit
func TestPVM(t *testing.T) {
	// Directory containing the JSON files
	dir := "../jamtestvectors/pvm/programs"

	// Read all files in the directory
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}
	count := 0
	num_mismatch := 0
	total_mismatch := 0
	for _, file := range files {
		count++
		if file.IsDir() {
			continue
		}

		if !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		filePath := filepath.Join(dir, file.Name())
		data, err := ioutil.ReadFile(filePath)
		if err != nil {
			t.Fatalf("Failed to read file %s: %v", filePath, err)
		}

		var testCase TestCase
		err = json.Unmarshal(data, &testCase)
		if err != nil {
			t.Fatalf("Failed to unmarshal JSON from file %s: %v", filePath, err)
		}
		err, num_mismatch = pvm_test(testCase)
		if err != nil {
			t.Fatalf("%v", err)
		}
		total_mismatch += num_mismatch
	}
	// show the match rate
	fmt.Printf("Match rate: %v/%v\n", count-total_mismatch, count)
}

func TestRevm(t *testing.T) {
	log.InitLogger("info")
	fp := "../services/revm_test.pvm"
	raw_code, err := os.ReadFile(fp)
	if err != nil {
		t.Fatalf("Failed to read file %s: %v", fp, err)
		return
	}
	fmt.Printf("Read %d bytes from %s\n", len(raw_code), fp)

	initial_regs := make([]uint64, 13)
	initial_pc := uint64(0)
	hostENV := NewMockHostEnv()
	metadata := "revm_test"
	pvm := NewVM(0, raw_code, initial_regs, initial_pc, hostENV, true, []byte(metadata))

	a := make([]byte, 0)
	pvm.Gas = int64(9999999999999999)

	start := time.Now()

	Standard_Program_Initialization(pvm, a)

	// pvm.Ram.DebugStatus()

	fmt.Printf("PVM start execution...\n")
	pvm.Execute(types.EntryPointRefine, false)

	fmt.Printf("pvm.pc: %d, gas: %d, vm.ResultCode: %d, vm.Fault_address: %d\n", pvm.pc, pvm.Gas, pvm.ResultCode, pvm.Fault_address)
	elapsed := time.Since(start)
	fmt.Printf("Execution took %s\n", elapsed)
}

func TestDoom(t *testing.T) {
	log.InitLogger("info")
	fp := "../services/doom.pvm"
	raw_code, err := os.ReadFile(fp)
	if err != nil {
		t.Fatalf("Failed to read file %s: %v", fp, err)
		return
	}
	fmt.Printf("Read %d bytes from %s\n", len(raw_code), fp)

	initial_regs := make([]uint64, 13)
	initial_pc := uint64(0)
	hostENV := NewMockHostEnv()
	metadata := "revm_test"
	pvm := NewVM(0, raw_code, initial_regs, initial_pc, hostENV, true, []byte(metadata))

	a := make([]byte, 0)
	pvm.Gas = int64(9999999999999999)

	start := time.Now()

	Standard_Program_Initialization(pvm, a)

	// pvm.Ram.DebugStatus()

	fmt.Printf("PVM start execution...\n")
	pvm.Execute(types.EntryPointRefine, false)

	fmt.Printf("pvm.pc: %d, gas: %d, vm.ResultCode: %d, vm.Fault_address: %d\n", pvm.pc, pvm.Gas, pvm.ResultCode, pvm.Fault_address)
	elapsed := time.Since(start)
	fmt.Printf("Execution took %s\n", elapsed)
}

func TestHelloWorld(t *testing.T) {
	log.InitLogger("info")
	fp := "../services/hello_world.pvm"
	raw_code, err := os.ReadFile(fp)
	if err != nil {
		t.Fatalf("Failed to read file %s: %v", fp, err)
		return
	}
	fmt.Printf("Read %d bytes from %s\n", len(raw_code), fp)

	initial_regs := make([]uint64, 13)
	initial_pc := uint64(0)
	hostENV := NewMockHostEnv()
	metadata := "revm_test"
	pvm := NewVM(0, raw_code, initial_regs, initial_pc, hostENV, true, []byte(metadata))

	a := make([]byte, 0)
	pvm.Gas = int64(9999999999999999)

	start := time.Now()

	Standard_Program_Initialization(pvm, a)

	// pvm.Ram.DebugStatus()

	fmt.Printf("PVM start execution...\n")
	pvm.Execute(types.EntryPointRefine, false)

	fmt.Printf("pvm.pc: %d, gas: %d, vm.ResultCode: %d, vm.Fault_address: %d\n", pvm.pc, pvm.Gas, pvm.ResultCode, pvm.Fault_address)
	elapsed := time.Since(start)
	fmt.Printf("Execution took %s\n", elapsed)
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
