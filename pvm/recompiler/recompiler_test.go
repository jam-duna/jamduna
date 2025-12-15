package recompiler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

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
	ExpectedStatus    string            `json:"expected-status"`
	ExpectedRegs      []uint64          `json:"expected-regs"`
	ExpectedPC        uint32            `json:"expected-pc"`
	ExpectedMemory    []TestMemory      `json:"expected-memory"`
	ExpectedGas       int64             `json:"expected-gas"`
	BasicBlockGasCost map[uint32]uint64 `json:"block-gas-costs"`
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
	if expectedCodeStr == "page-fault" || expectedCodeStr == "out-of-gas" {
		expectedCodeStr = "panic"
	}

	if resultCodeStr != expectedCodeStr {
		return fmt.Errorf("result code mismatch for test %s: expected %s, got %s", tc.Name, expectedCodeStr, resultCodeStr)
	}

	if GasMode == GasModeBasicBlock {
		for pvmPC, expectedGas := range tc.BasicBlockGasCost {
			actualBlock := rvm.compiler.GetBasicBlock(uint64(pvmPC))
			if actualBlock == nil || uint64(actualBlock.GasUsage) != expectedGas {
				return fmt.Errorf("gas mismatch for basic block at PVM PC %d in test %s: expected %d, got %d", pvmPC, tc.Name, expectedGas, actualBlock.GasUsage)
			}
		}
	}

	// expectedGas := tc.ExpectedGas
	// actualGas := rvm.Gas
	// if expectedGas != actualGas {
	// 	return fmt.Errorf("gas mismatch for test %s: expected %d, got %d", tc.Name, expectedGas, actualGas)
	// }

	return nil
}
func recompiler_integration_tests(tc TestCase) error {
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

	rvm.Gas = 0
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
	rvm.x86Code, rvm.djumpAddrOffset, rvm.InstMapPVMToX86, rvm.InstMapX86ToPVM = rvm.compiler.CompileX86Code(rvm.pc)
	rvm.Close()
	var basicBlockDiffs []BasicBlockDiff
	if GasMode == GasModeBasicBlock {
		for pvmPC, expectedGas := range tc.BasicBlockGasCost {
			actualBlock := rvm.compiler.GetBasicBlock(uint64(pvmPC))
			if actualBlock == nil || uint64(actualBlock.GasUsage) != expectedGas {
				var instructions []string
				for _, inst := range actualBlock.Instructions {
					instructions = append(instructions, inst.String())
				}
				basicBlockDiffs = append(basicBlockDiffs, BasicBlockDiff{
					PvmPC:        pvmPC,
					ExpectedGas:  expectedGas,
					Instructions: instructions,
					ActualGas:    uint64(actualBlock.GasUsage),
				})
			}
		}
	}
	// sort the basicBlockDiffs by PvmPC
	sort.Slice(basicBlockDiffs, func(i, j int) bool {
		return basicBlockDiffs[i].PvmPC < basicBlockDiffs[j].PvmPC
	})

	// if there are any diffs, return an error
	if len(basicBlockDiffs) > 0 {
		diffBytes, _ := json.MarshalIndent(basicBlockDiffs, "", "  ")
		os.WriteFile(fmt.Sprintf("basic_block_diffs_%s.json", tc.Name), diffBytes, 0644)
		return fmt.Errorf("basic block gas mismatches for test %s: different blocks %d", tc.Name, len(basicBlockDiffs))
	}

	return nil
}

type BasicBlockDiff struct {
	PvmPC        uint32   `json:"pc"`
	ExpectedGas  uint64   `json:"expected-gas"`
	Instructions []string `json:"instructions,omitempty"`
	ActualGas    uint64   `json:"actual-gas"`
}

func TestPVM_Integration(t *testing.T) {
	log.InitLogger("debug")
	// Directory containing the JSON files
	dir := "../../statedb/new_gas_model_test/integration-tests"

	// Read all files in the directory
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}
	count := 0
	num_mismatch := 0
	total_mismatch := 0
	skip := map[string]bool{
		"pinky.json":       false, // skip pinky test for now
		"doom.json":        false, // skip doom test for now
		"prime-sieve.json": false, // skip prime sieve test for now
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
		fmt.Printf("case %s len=%d bytes\n", name, len(testCase.Code))
		t.Run(name, func(t *testing.T) {
			err = recompiler_integration_tests(testCase)
			if err != nil {
				t.Fatalf("❌ [%s] Test failed: %v", name, err)
			} else {
				t.Logf("✅ [%s] Test passed", name)
			}
		})
		total_mismatch += num_mismatch
	}
	// show the match rate
	fmt.Printf("Match rate: %v/%v\n", count-total_mismatch, count)
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
		"inst_ecalli_100.json":  true, // skip ecalli test for now
		"inst_fallthrough.json": true, // skip fallthrough test for now
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
	filename := "../../statedb/programs/inst_load_u8_nok.json"
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
func TestDoom(t *testing.T) {
	serviceAcct := uint32(0) // stub

	// Convert test code to raw instruction bytes (matches Go exactly)
	doomFile := "/tmp/doom_self_playing.pvm"
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
	vm.SetGas(2_000_000_000_000)
	err = vm.SetBitMask(p.K)
	if err != nil {
		t.Fatalf("failed to set bitmask: %v", err)
	}
	err = vm.SetJumpTable(p.J)
	if err != nil {
		t.Fatalf("failed to set jump table: %v", err)
	}
	vm.HostFunc = NewDummyHostFunc(vm)
	vm.Init([]byte{})

	// Check memory at 0x130040 before execution
	checkAddr := uint32(0x130040)
	checkData, _ := vm.ReadRAMBytes(checkAddr, 40)
	fmt.Printf("GO BEFORE execution - Memory at 0x%x: %v\n", checkAddr, checkData)
	fmt.Printf("GO BEFORE execution - As string: %q\n", string(checkData))
	four_gigas := uint64(4 * 1024 * 1024 * 1024)
	vm.SetMemAccess(0, uint32(four_gigas), PageMutable)
	vm.SetHeapPointer(uint32(four_gigas))
	vm.Execute(0)
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

var algoPayloads = []byte{
	0xc2, 0xb8, 0xb4, 0xbb, 0xcb, 0xaa, 0x47, 0xd4, 0xe9, 0xdc, 0x39, 0xce, 0xb8, 0xbc, 0x75, 0x2b, 0x2b, 0x6b, 0x8c, 0x98, 0x88, 0xab, 0xb4, 0xc4, 0x9c, 0x59, 0xc2, 0xcb, 0xbd, 0xa2, 0x96, 0x94, 0xb1, 0x4d, 0xb6, 0xb7, 0xbc, 0x78, 0x72, 0x96, 0x85, 0x0a, 0xa7, 0x0d, 0x77, 0xb6, 0x02, 0xb1, 0xb3, 0xb4, 0xbd, 0xb7, 0xcc, 0xf5,
}

func getGitInfo() (commit, tag string) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	c, _ := cmd.Output()

	cmd = exec.Command("git", "describe", "--tags", "--always")
	t, _ := cmd.Output()

	return string(bytes.TrimSpace(c)), string(bytes.TrimSpace(t))
}

func TestAlgo(t *testing.T) {
	numBlocks := 53

	// Convert test code to raw instruction bytes (matches Go exactly)
	algoFile := "../../services/algo_classic/algo_classic.pvm"
	rawCodeBytes, err := os.ReadFile(algoFile)
	if err != nil {
		t.Fatalf("Failed to read Doom PVM file: %v", err)
	}
	fmt.Printf("# ALGO TEST\n")
	fmt.Printf("## Experiment Info\n")
	// github stuff
	fmt.Printf("- Test Date: %s\n", time.Now().Format(time.RFC1123))
	fmt.Printf("- PVM File: %s\n", algoFile)
	fmt.Printf("- Code Length: %d bytes\n", len(rawCodeBytes))
	commit, tag := getGitInfo()
	fmt.Printf("- Commit: %s\n", commit)
	fmt.Printf("- Tag: %s\n", tag)
	fmt.Printf("## Performance Results\n")
	// CRITICAL: Use DecodeCorePart like Go does (matches recompiler_test.go line 59)
	p, o_size, w_size, z, s, o_byte, w_byte := program.DecodeProgram(rawCodeBytes)

	for n := 0; n <= numBlocks; n++ {
		payload := make([]byte, 2)
		payload[0] = byte(n)
		payload[1] = algoPayloads[n]
		serviceAcct := uint32(0) // stub
		iter := uint64(payload[1])
		iter_3 := iter * iter * iter
		fmt.Printf("### Algo ID %d - Iterations: %d\n", n, iter_3)
		// Create C FFI VM with decoded program code (matches Go line 67)
		vm, err := NewRecompilerVM(serviceAcct, p.Code, make([]uint64, 13), 0)
		if err != nil {
			t.Fatalf("failed to create RecompilerVM: %v", err)
		}

		rw_data_address := uint32(2*Z_Z) + Z_func(o_size)
		rw_data_address_end := rw_data_address + P_func(w_size)
		current_heap_pointer := rw_data_address_end
		vm.SetHeapPointer(current_heap_pointer)
		vm.SetMemoryBounds(o_size, w_size, z, s, o_byte, w_byte)
		// CRITICAL: Set bitmask and jump table from decoded program (matches Go lines 75-76)
		// This is the correct way! Go calls: rvm.SetBitMask(p.K) and rvm.SetJumpTable(p.J)
		og_gas := 20_000_000_000
		vm.SetGas(int64(og_gas))
		err = vm.SetBitMask(p.K)
		if err != nil {
			t.Fatalf("failed to set bitmask: %v", err)
		}
		err = vm.SetJumpTable(p.J)
		if err != nil {
			t.Fatalf("failed to set jump table: %v", err)
		}
		vm.HostFunc = NewDummyHostFunc(vm)
		vm.Init(payload)

		// Check memory at 0x130040 before execution
		// four_gigas := uint64(4 * 1024 * 1024 * 1024)
		// vm.SetMemAccess(0, uint32(four_gigas), PageMutable)
		// vm.SetHeapPointer(uint32(four_gigas))
		// time the execution
		// start := time.Now()
		vm.Execute(0)
		// elapsed := time.Since(start)
		gasUsed := int64(og_gas) - vm.GetGas()
		formatWithCommas := func(n int64) string {
			if n == 0 {
				return "0"
			}
			neg := n < 0
			if neg {
				n = -n
			}
			s := fmt.Sprintf("%d", n)
			var out []byte
			cnt := 0
			for i := len(s) - 1; i >= 0; i-- {
				if cnt == 3 {
					out = append(out, ',')
					cnt = 0
				}
				out = append(out, s[i])
				cnt++
			}
			for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
				out[i], out[j] = out[j], out[i]
			}
			if neg {
				return "-" + string(out)
			}
			return string(out)
		}
		fmt.Printf("- Gas Used: %s\n", formatWithCommas(gasUsed))
		fmt.Printf("- Total Execution Time: %v\n", timeDurationOrRaw(float64(vm.allExecutionTime), vm.allExecutionTime))
		total := float64(vm.allExecutionTime)
		ct := float64(vm.compileTime)
		ht := float64(vm.hostcallTime)
		et := float64(vm.executionTime)
		un := vm.allExecutionTime - vm.compileTime - vm.hostcallTime - vm.executionTime

		ctPct, htPct, etPct, unPct := 0.0, 0.0, 0.0, 0.0
		if total > 0 {
			ctPct = 100.0 * ct / total
			htPct = 100.0 * ht / total
			etPct = 100.0 * et / total
			unPct = 100.0 * float64(un) / total
		}

		fmt.Println()
		fmt.Println("| Section       | Time              | Percent |")
		fmt.Println("|----------------|-------------------|----------|")
		fmt.Printf("| Compile/Load   | %16v | %6.1f%% |\n", vm.compileTime, ctPct)
		fmt.Printf("| Hostcall       | %16v | %6.1f%% |\n", vm.hostcallTime, htPct)
		fmt.Printf("| Execution      | %16v | %6.1f%% |\n", vm.executionTime, etPct)
		fmt.Printf("| Unaccounted    | %16v | %6.1f%% |\n", un, unPct)
		fmt.Println()

		vm.Close()
	}
}
func timeDurationOrRaw(val float64, totalRaw interface{}) interface{} {
	// try to detect if vm.* fields are time.Duration by checking formatting of vm.compileTime via fmt
	// keep simple: if totalRaw is an integer-ish, return time.Duration
	// Note: this small helper keeps output readable for both int and time.Duration representations.
	return time.Duration(val)
}
