package recompiler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/pvm/program"
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
	ExpectedGas    int64        `json:"expected-gas"`
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
	var p *program.Program
	var o_size, w_size, z, s uint32
	var o_byte, w_byte []byte

	p = program.DecodeCorePart(rawCodeBytes)
	o_size = 0
	w_size = uint32(4096)
	z = 0
	s = 0
	o_byte = []byte{}
	w_byte = make([]byte, w_size)

	rvm, _ := NewRecompilerVM(serviceAcct, p.Code, tc.InitialRegs, uint64(tc.InitialPC))

	rvm.SetMemoryBounds(o_size, w_size, z, s, o_byte, w_byte)
	// w - read-write
	rw_data_address := uint32(2*Z_Z) + Z_func(o_size)
	rw_data_address_end := rw_data_address + P_func(w_size)
	current_heap_pointer := rw_data_address_end
	rvm.SetHeapPointer(current_heap_pointer)
	rvm.SetBitMask(p.K)
	rvm.SetJumpTable(p.J)
	// Set the initial memory
	for _, mem := range tc.InitialMemory {
		//pvm.Ram.SetPageAccess(mem.Address/PageSize, 1, AccessMode{Readable: false, Writable: true, Inaccessible: false})
		rvm.SetMemAccess(mem.Address, uint32(len(mem.Data)), PageMutable)
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
			err := rvm.SetMemAccess(pm.Address, pm.Length, PageMutable)
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
	rvm.Gas = tc.InitialGas
	rvm.HostFunc = NewDummyHostFunc(rvm)
	rvm.Execute(uint32(rvm.pc))
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
	resultCodeStr := types.HostResultCodeToString[resultCode]
	if resultCodeStr == "page-fault" {
		resultCodeStr = "panic"
	}
	expectedCodeStr := tc.ExpectedStatus
	if expectedCodeStr == "page-fault" {
		expectedCodeStr = "panic"
	}

	if resultCodeStr != expectedCodeStr {
		return fmt.Errorf("result code mismatch for test %s: expected %s, got %s", tc.Name, expectedCodeStr, resultCodeStr)
	}

	// expectedGas := tc.ExpectedGas
	// actualGas := rvm.Gas
	// if expectedGas != actualGas {
	// 	return fmt.Errorf("gas mismatch for test %s: expected %d, got %d", tc.Name, expectedGas, actualGas)
	// }

	return nil
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
	dir := "../../statedb/programs"

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
			err = recompiler_test(testCase)
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
	filename := "../../statedb/programs/inst_add_32.json"
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

func TestHashService(t *testing.T) {
	serviceAcct := uint32(0) // stub

	// Convert test code to raw instruction bytes (matches Go exactly)
	doomFile := "../../services/blake2b_child.pvm"
	rawCodeBytes, err := os.ReadFile(doomFile)
	if err != nil {
		t.Fatalf("Failed to read Doom PVM file: %v", err)
	}

	// CRITICAL: Use DecodeCorePart like Go does (matches recompiler_test.go line 59)
	p, o_size, w_size, z, s, o_byte, w_byte := program.DecodeProgram(rawCodeBytes)

	// Create C FFI VM with decoded program code (matches Go line 67)
	vm, err := NewRecompilerVM(serviceAcct, p.Code, make([]uint64, 13), 0)
	if err != nil {
		t.Fatalf("failed to create RecompilerVM: %v", err)
	}
	defer vm.Close()
	rw_data_address := uint32(2*Z_Z) + Z_func(o_size)
	rw_data_address_end := rw_data_address + P_func(w_size)
	current_heap_pointer := rw_data_address_end
	vm.SetHeapPointer(current_heap_pointer)
	vm.SetMemoryBounds(o_size, w_size, z, s, o_byte, w_byte)
	// CRITICAL: Set bitmask and jump table from decoded program (matches Go lines 75-76)
	// This is the correct way! Go calls: rvm.SetBitMask(p.K) and rvm.SetJumpTable(p.J)
	vm.SetGas(10000000)
	err = vm.SetBitMask(p.K)
	if err != nil {
		t.Fatalf("failed to set bitmask: %v", err)
	}
	vm.HostFunc = NewDummyHostFunc(vm)
	vm.Init([]byte{})

	// Check memory at 0x130040 before execution
	checkAddr := uint32(0x130040)
	checkData, _ := vm.ReadRAMBytes(checkAddr, 40)
	fmt.Printf("GO BEFORE execution - Memory at 0x%x: %v\n", checkAddr, checkData)
	fmt.Printf("GO BEFORE execution - As string: %q\n", string(checkData))

	// DEBUG: Print jump table before setting it
	err = vm.SetJumpTable(p.J)
	if err != nil {
		t.Fatalf("failed to set jump table: %v", err)
	}

	vm.Execute(0)
	fmt.Printf("Go registers after execute: %v\n", vm.ReadRegisters())
	return_address := vm.ReadRegister(7)
	fmt.Printf("Go return_address: %d\n", return_address)
	return_length := vm.ReadRegister(8)
	fmt.Printf("Go return_length: %d\n", return_length)
	return_data, _ := vm.ReadRAMBytes(uint32(return_address), uint32(return_length))

	hash_data := common.Hex2Bytes("0xdeadbeef")
	//hash 10 times
	for i := 0; i < 100; i++ {
		hash_data = common.Blake2Hash(hash_data[:]).Bytes()
	}
	if !bytes.Equal(hash_data, return_data) {
		t.Fatalf("Hash mismatch: expected %x, got %x", hash_data, return_data)
	} else {
		fmt.Printf("Hash match: %x\n", return_data)
	}

	// read out the gas
	gasLeft := vm.GetGas()
	fmt.Printf("Gas left after execute: %d\n", gasLeft)
}

func TestEcalli(t *testing.T) {
	log.InitLogger("debug")
	filename := "../../statedb/programs/inst_ecalli_100.json"
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("Failed to read test file %s: %v", filename, err)
	}

	var tc TestCase
	err = json.Unmarshal(data, &tc)
	if err != nil {
		t.Fatalf("Failed to unmarshal test case from %s: %v", filename, err)
	}
	serviceAcct := uint32(0) // stub
	// metadata, c := types.SplitMetadataAndCode(tc.Code)
	// Convert test code to raw instruction bytes
	rawCodeBytes := make([]byte, len(tc.Code))
	for i, val := range tc.Code {
		rawCodeBytes[i] = byte(val)
	}
	fmt.Printf("running test: %s\n", tc.Name)
	var p *program.Program
	var o_size, w_size, z, s uint32
	var o_byte, w_byte []byte

	p = program.DecodeCorePart(rawCodeBytes)
	o_size = 0
	w_size = uint32(4096)
	z = 0
	s = 0
	o_byte = []byte{}
	w_byte = make([]byte, w_size)

	rvm, _ := NewRecompilerVM(serviceAcct, p.Code, tc.InitialRegs, uint64(tc.InitialPC))

	rvm.SetMemoryBounds(o_size, w_size, z, s, o_byte, w_byte)
	// w - read-write
	rw_data_address := uint32(2*Z_Z) + Z_func(o_size)
	rw_data_address_end := rw_data_address + P_func(w_size)
	current_heap_pointer := rw_data_address_end
	rvm.SetHeapPointer(current_heap_pointer)
	rvm.SetBitMask(p.K)
	rvm.SetJumpTable(p.J)
	// Set the initial memory
	for _, mem := range tc.InitialMemory {
		//pvm.Ram.SetPageAccess(mem.Address/PageSize, 1, AccessMode{Readable: false, Writable: true, Inaccessible: false})
		rvm.SetMemAccess(mem.Address, uint32(len(mem.Data)), PageMutable)
		rvm.WriteRAMBytes(mem.Address, mem.Data[:])
	}
	rvm.Gas = 100000000000
	for _, pm := range tc.InitialPageMap {
		// Set the page access based on the initial page map
		if pm.IsWritable {
			err := rvm.SetMemAccess(pm.Address, pm.Length, PageMutable)
			if err != nil {
				t.Fatalf("failed to set memory access for address %x: %v", pm.Address, err)
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
	rvm.Gas = 100000
	rvm.HostFunc = NewDummyHostFunc(rvm)
	rvm.Execute(uint32(0))
}
