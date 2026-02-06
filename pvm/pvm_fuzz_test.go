//go:build pvm_ffi
// +build pvm_ffi

// PVM Fuzz Test - Compares Go Interpreter vs X86 Recompiler
// Uses Rust FFI to generate valid PolkaVM program blobs from arbitrary fuzzing data

package pvm

import (
	"bytes"
	"fmt"
	"runtime"
	"testing"

	"github.com/jam-duna/jamduna/pvm/interpreter"
	"github.com/jam-duna/jamduna/pvm/pvmtypes"
	"github.com/jam-duna/jamduna/pvm/recompiler"
	"github.com/jam-duna/jamduna/pvm/testutil"
)

// FuzzResult holds execution result from a backend
type FuzzResult struct {
	Registers  [13]uint64
	ResultCode uint8
	PC         uint64
	Gas        int64
	Memory     []byte // First 4KB of memory for comparison
}

// executeInterpreter runs the program on VMGo (Go interpreter)
func executeInterpreter(programBlob []byte, initialRegs []uint64, initialGas int64) (*FuzzResult, error) {
	hostENV := testutil.NewMockHostEnv()
	serviceAcct := uint32(0)

	p, err := DecodeProgram_pure_pvm_blob(programBlob)
	if err != nil {
		return nil, fmt.Errorf("failed to decode program: %w", err)
	}
	if p == nil || len(p.Code) == 0 {
		return nil, fmt.Errorf("failed to decode program: empty code")
	}

	vmgo := interpreter.NewVMGo(serviceAcct, p, initialRegs, 0, uint64(initialGas), hostENV)
	if vmgo == nil {
		return nil, fmt.Errorf("failed to create VMGo")
	}
	defer func() {
		vmgo.Destroy()
		runtime.GC()
	}()

	vmgo.IsChild = true // Use instruction-level gas mode for comparison

	// Execute
	fakeVM := &pvmtypes.FakeHostVM{}
	err = vmgo.Execute(fakeVM, 0, "")
	fmt.Printf("VMGo execution finished with error: %v\n", err)

	// Collect result
	result := &FuzzResult{
		Registers:  vmgo.ReadRegisters(),
		ResultCode: vmgo.GetMachineState(),
		PC:         vmgo.GetPC(),
		Gas:        vmgo.Gas,
	}

	// Read first 4KB of memory for comparison
	mem, _ := vmgo.ReadRAMBytes(0, 4096)
	result.Memory = mem

	return result, nil
}

// executeRecompiler runs the program on X86 JIT recompiler
func executeRecompiler(programBlob []byte, initialRegs []uint64, initialGas int64) (*FuzzResult, error) {
	hostENV := testutil.NewMockHostEnv()
	serviceAcct := uint32(0)

	p, err := DecodeProgram_pure_pvm_blob(programBlob)
	if err != nil {
		return nil, fmt.Errorf("failed to decode program: %w", err)
	}
	if p == nil || len(p.Code) == 0 {
		return nil, fmt.Errorf("failed to decode program: empty code")
	}

	rvm := NewRecompilerVMWithoutSetup(serviceAcct, initialRegs, 0, hostENV, false, []byte{}, uint64(initialGas), p)
	if rvm == nil {
		return nil, fmt.Errorf("failed to create recompiler VM")
	}
	defer func() {
		rvm.Destroy()
		runtime.GC()
	}()

	// Execute
	fakeVM := &pvmtypes.FakeHostVM{}
	rvm.Execute(fakeVM, 0, "")

	// Collect result
	result := &FuzzResult{
		Registers:  rvm.ReadRegisters(),
		ResultCode: rvm.GetMachineState(),
		PC:         rvm.GetPC(),
		Gas:        rvm.GetGas(),
	}

	// Read first 4KB of memory for comparison
	mem, _ := rvm.ReadRAMBytes(0, 4096)
	result.Memory = mem

	return result, nil
}

// compareResults checks if two execution results match and returns all differences
func compareResults(interp, recomp *FuzzResult) error {
	var errors []string

	// Compare machine state/result code
	if interp.ResultCode != recomp.ResultCode {
		errors = append(errors, fmt.Sprintf("result code mismatch: interpreter=%d, recompiler=%d",
			interp.ResultCode, recomp.ResultCode))
	}

	// Compare registers
	for i := 0; i < 13; i++ {
		if interp.Registers[i] != recomp.Registers[i] {
			errors = append(errors, fmt.Sprintf("register %d mismatch: interpreter=%d, recompiler=%d",
				i, interp.Registers[i], recomp.Registers[i]))
		}
	}

	// Compare PC (only if both halted normally)
	if interp.ResultCode == pvmtypes.HALT && recomp.ResultCode == pvmtypes.HALT {
		if interp.PC != recomp.PC {
			errors = append(errors, fmt.Sprintf("PC mismatch: interpreter=%d, recompiler=%d",
				interp.PC, recomp.PC))
		}
	}

	// Compare gas consumed (allow small tolerance for timing differences)
	if recomp.Gas < 0 {
		recomp.Gas = 0
	}
	if interp.Gas != recomp.Gas {
		gasDiff := interp.Gas - recomp.Gas
		if gasDiff < 0 {
			gasDiff = -gasDiff
		}
		errors = append(errors, fmt.Sprintf("gas mismatch: interpreter=%d, recompiler=%d",
			interp.Gas, recomp.Gas))
	}
	// Compare memory (first 4KB) with detailed diff
	if !bytes.Equal(interp.Memory, recomp.Memory) {
		// Find first differing byte for more detail
		minLen := len(interp.Memory)
		if len(recomp.Memory) < minLen {
			minLen = len(recomp.Memory)
		}
		diffCount := 0
		firstDiffIdx := -1
		for i := 0; i < minLen; i++ {
			if interp.Memory[i] != recomp.Memory[i] {
				if firstDiffIdx == -1 {
					firstDiffIdx = i
				}
				diffCount++
			}
		}
		if len(interp.Memory) != len(recomp.Memory) {
			errors = append(errors, fmt.Sprintf("memory length mismatch: interpreter=%d, recompiler=%d",
				len(interp.Memory), len(recomp.Memory)))
		}
		if firstDiffIdx >= 0 {
			errors = append(errors, fmt.Sprintf("memory mismatch: %d bytes differ, first diff at offset %d (interpreter=0x%02x, recompiler=0x%02x)",
				diffCount, firstDiffIdx, interp.Memory[firstDiffIdx], recomp.Memory[firstDiffIdx]))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("%s", joinErrors(errors))
	}
	return nil
}

// joinErrors combines multiple error messages with newlines
func joinErrors(errors []string) string {
	result := ""
	for i, e := range errors {
		if i > 0 {
			result += "\n"
		}
		result += e
	}
	return result
}

//export LD_LIBRARY_PATH=/root/go/src/github.com/colorfulnotion/polkavm/fuzz/target/release:$LD_LIBRARY_PATH
// go test -run=FuzzPVMCorrectness/70932e7e4fa3c464 for single
// go test -fuzz=FuzzPVMCorrectness -fuzztime=30s -run=^$
// FuzzPVMCorrectness is the main fuzz test entry point
//

func FuzzPVMCorrectness(f *testing.F) {
	recompiler.ALWAYS_COMPILE = true
	recompiler.EnableDebugTracing = false
	recompiler.SetShowDisassembly(false)
	recompiler.DebugRecompilerResult = false
	interpreter.PvmLogging = false
	// Add seed corpus
	f.Add([]byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07})
	f.Add([]byte{0x10, 0x20, 0x30, 0x40, 0x50, 0x60, 0x70, 0x80})
	f.Add(make([]byte, 64))
	f.Add(make([]byte, 256))

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 8 {
			t.Skip("input too short")
		}

		// Build program blob from fuzzing data via Rust FFI
		programBlob, err := BuildProgramBlobFromRust(data)
		if err != nil {
			// Invalid input - not a bug, just skip
			t.Skip("failed to build program blob: ", err)
		}

		if len(programBlob) == 0 {
			t.Skip("empty program blob")
		}

		// Initialize registers and gas
		initialRegs := make([]uint64, 13)
		initialGas := int64(1000000)

		// Run on interpreter
		interpResult, err := executeInterpreter(programBlob, initialRegs, initialGas)
		if err != nil {
			t.Skip("interpreter failed: ", err)
		}

		// Run on recompiler
		recompResult, err := executeRecompiler(programBlob, initialRegs, initialGas)
		if err != nil {
			t.Skip("recompiler failed: ", err)
		}

		// Compare results
		if err := compareResults(interpResult, recompResult); err != nil {
			disasm := DisassembleToString(programBlob)
			t.Fatalf("MISMATCH FOUND!\nProgram blob (hex): %x\n%v\n\nDisassembly:\n%s", programBlob, err, disasm)
		}
	})
}

// TestPVMFuzz is a simple non-fuzz test to verify the setup works
func TestPVMFuzz(t *testing.T) {
	// Test with a simple seed
	data := []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f}

	programBlob, err := BuildProgramBlobFromRust(data)
	if err != nil {
		t.Logf("Failed to build program blob (expected for some inputs): %v", err)
		return
	}

	t.Logf("Generated program blob of %d bytes", len(programBlob))

	initialRegs := make([]uint64, 13)
	initialGas := int64(1000000)

	interpResult, err := executeInterpreter(programBlob, initialRegs, initialGas)
	if err != nil {
		t.Fatalf("Interpreter failed: %v", err)
	}

	recompResult, err := executeRecompiler(programBlob, initialRegs, initialGas)
	if err != nil {
		t.Fatalf("Recompiler failed: %v", err)
	}

	if err := compareResults(interpResult, recompResult); err != nil {
		t.Fatalf("Mismatch: %v", err)
	}

	t.Logf("Test passed! Interpreter result: state=%d, gas=%d", interpResult.ResultCode, interpResult.Gas)
	t.Logf("Test passed! Recompiler result: state=%d, gas=%d", recompResult.ResultCode, recompResult.Gas)
}

// TestDisassembler tests the disassembler with the known failing blob
func TestDisassembler(t *testing.T) {
	// The failing blob from fuzz testing
	blob := []byte{0x00, 0x00, 0x04, 0x01, 0x51, 0x00, 0xff, 0x03}

	t.Logf("Testing disassembler with blob: %x", blob)
	disasm := DisassembleToString(blob)
	t.Logf("Disassembly:\n%s", disasm)

	// Also test DisassembleJAMBlob directly
	instructions, err := DisassembleJAMBlob(blob)
	if err != nil {
		t.Fatalf("DisassembleJAMBlob failed: %v", err)
	}

	for _, inst := range instructions {
		t.Logf("  @%d: %s %s [%s]", inst.Offset, inst.Name, inst.Args, inst.RawHex)
	}
}

// TestPVMFuzzManual runs multiple random-like inputs without the fuzz framework
func TestPVMFuzzManual(t *testing.T) {
	testCases := [][]byte{
		{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
		{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a},
		{0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f},
	}

	for i, data := range testCases {
		t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
			programBlob, err := BuildProgramBlobFromRust(data)
			if err != nil {
				t.Logf("Skipping case %d: %v", i, err)
				return
			}

			initialRegs := make([]uint64, 13)
			initialGas := int64(1000000)

			interpResult, err := executeInterpreter(programBlob, initialRegs, initialGas)
			if err != nil {
				t.Logf("Interpreter failed for case %d: %v", i, err)
				return
			}

			recompResult, err := executeRecompiler(programBlob, initialRegs, initialGas)
			if err != nil {
				t.Logf("Recompiler failed for case %d: %v", i, err)
				return
			}

			if err := compareResults(interpResult, recompResult); err != nil {
				disasm := DisassembleToString(programBlob)
				t.Errorf("Case %d mismatch: %v\nProgram blob: %x\n\nDisassembly:\n%s", i, err, programBlob, disasm)
			} else {
				t.Logf("Case %d passed", i)
			}
		})
	}
}
