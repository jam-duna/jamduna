package recompiler

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/jam-duna/jamduna/pvm/program"
	"github.com/jam-duna/jamduna/types"
)

const test_data_dir = "../testdata/new_gas_model_test/tests/programs/"

func TestPVM_NewAll(t *testing.T) {

	// Read all files in the directory
	files, err := os.ReadDir(test_data_dir)
	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}
		// if not a .json file, skip
		if len(file.Name()) < 6 || file.Name()[len(file.Name())-5:] != ".json" {
			continue
		}
		filePath := test_data_dir + file.Name()
		// read the json file
		data, err := os.ReadFile(filePath)
		if err != nil {
			t.Errorf("Failed to read file %s: %v", file.Name(), err)
			continue
		}

		var testCase TestCaseNew
		err = json.Unmarshal(data, &testCase)
		if err != nil {
			t.Errorf("Failed to unmarshal JSON from file %s: %v", file.Name(), err)
			continue
		}
		t.Run(testCase.Name, func(t *testing.T) {
			RunTestCase(t, testCase)
		})
	}
}

func RunTestCase(t *testing.T, tc TestCaseNew) {
	serviceAcct := uint32(0) // stub
	// metadata, c := types.SplitMetadataAndCode(tc.Code)
	// Convert test code to raw instruction bytes
	rawCodeBytes := tc.Program
	fmt.Printf("running test: %s\n", tc.Name)
	var p *program.Program
	var o_size, w_size, z, s uint32
	var o_byte, w_byte []byte
	var err error

	p, err = program.DecodeCorePart(rawCodeBytes)
	if err != nil {
		t.Fatalf("Failed to decode program: %v", err)
	}
	o_size = 0
	w_size = uint32(4096)
	z = 0
	s = 0
	o_byte = []byte{}
	w_byte = make([]byte, w_size)

	rvm, _ := NewRecompilerVM(serviceAcct, p.Code, make([]uint64, 13), uint64(tc.InitialPC))

	rvm.SetMemoryBounds(o_size, w_size, z, s, o_byte, w_byte)
	// w - read-write
	rw_data_address := uint32(2*Z_Z) + Z_func(o_size)
	rw_data_address_end := rw_data_address + P_func(w_size)
	current_heap_pointer := rw_data_address_end
	rvm.SetHeapPointer(current_heap_pointer)
	rvm.SetBitMask(p.K)
	rvm.SetJumpTable(p.J)
	rvm.SetPC(0)
	rvm.Gas = int64(tc.InitialGas)
	rvm.HostFunc = NewDummyHostFunc(rvm)
	fmt.Printf("running test: %s\n", tc.Name)
	for _, step := range tc.Steps {
		switch step.Kind {
		case "map":
			if step.IsWritable {
				rvm.SetMemAccess(step.Address, step.Length, PageMutable)
			} else {
				rvm.SetMemAccess(step.Address, step.Length, PageImmutable)
			}
		case "write":
			rvm.WriteMemory(step.Address, step.Contents)
		case "set-reg":
			rvm.WriteRegister(step.Reg, step.Value)
		case "run":
			rvm.Execute(uint32(rvm.pc))
		case "assert":
			err := checkAssert(rvm, step)
			if err != nil {
				t.Fatalf("Test %s failed at assert step: %v", tc.Name, err)
			}
		default:
			t.Fatalf("Test %s has unknown step kind: %s", tc.Name, step.Kind)
		}
	}

}
func checkAssert(rvm *RecompilerVM, step Step) error {
	resultCode := rvm.MachineState

	resultCodeStr := types.HostResultCodeToString[resultCode]
	if resultCodeStr == "page-fault" {
		resultCodeStr = "panic"
	}
	expectedCodeStr := step.Status
	if expectedCodeStr == "page-fault" || expectedCodeStr == "out-of-gas" {
		expectedCodeStr = "panic"
	}

	if resultCodeStr != expectedCodeStr {
		return fmt.Errorf("expected status %s, got %s", expectedCodeStr, resultCodeStr)
	}

	if step.Gas != 0 {
		if uint64(rvm.Gas) != step.Gas {
			return fmt.Errorf("expected gas %d, got %d", step.Gas, rvm.Gas)
		}
	}

	if step.PC != 0 {
		pc := rvm.GetPC()
		if pc != step.PC {
			return fmt.Errorf("expected PC %d, got %d", step.PC, rvm.pc)
		}
	}

	if len(step.Regs) != 0 {
		for i, expectedReg := range step.Regs {
			actualReg := rvm.ReadRegister(i)
			if actualReg != expectedReg {
				return fmt.Errorf("expected reg[%d] = %d, got %d", i, expectedReg, actualReg)
			}
		}
	}

	for _, memAssert := range step.Memory {
		actualContents, _ := rvm.ReadMemory(memAssert.Address, uint32(len(memAssert.Contents)))
		for i, expectedByte := range memAssert.Contents {
			if actualContents[i] != expectedByte {
				return fmt.Errorf("expected memory at address %d to have byte %d at offset %d, got %d", memAssert.Address, expectedByte, i, actualContents[i])
			}
		}
	}

	return nil
}
