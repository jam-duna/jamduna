package pvm

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"
)

// Define the structure for the JSON data
type TestCase struct {
	Name           string        `json:"name"`
	InitialRegs    []int         `json:"initial-regs"`
	InitialPC      int           `json:"initial-pc"`
	InitialPageMap []interface{} `json:"initial-page-map"`
	InitialMemory  []interface{} `json:"initial-memory"`
	Code           []int         `json:"code"`
	ExpectedStatus string        `json:"expected-status"`
	ExpectedRegs   []int         `json:"expected-regs"`
	ExpectedPC     int           `json:"expected-pc"`
	ExpectedMemory []interface{} `json:"expected-memory"`
}

func pvm_test(tc TestCase) (string, []int, int, []interface{}) {
	// TODO: setup VM
	return "trap", tc.InitialRegs, tc.InitialPC, tc.InitialMemory
}

func TestPVM(t *testing.T) {
	// Directory containing the JSON files
	dir := "../jamtestvectors/pvm"

	// Read all files in the directory
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		if !strings.Contains(file.Name(), ".json") {
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

		// Run the PVM test
		status, regs, pc, memory := pvm_test(testCase)

		// Check the status

		if status != testCase.ExpectedStatus {
			//t.Errorf("Status mismatch for test %s: expected %s, got %s", testCase.Name, testCase.ExpectedStatus, status)
		}

		// Check the registers
		if !equalIntSlices(regs, testCase.ExpectedRegs) {
			//t.Errorf("Register mismatch for test %s: expected %v, got %v", testCase.Name, testCase.ExpectedRegs, regs)
		}

		// Check the program counter
		if pc != testCase.ExpectedPC {
			//t.Errorf("Program counter mismatch for test %s: expected %d, got %d", testCase.Name, testCase.ExpectedPC, pc)
		}

		// Check the memory
		if !equalInterfaceSlices(memory, testCase.ExpectedMemory) {
			//t.Errorf("Memory mismatch for test %s: expected %v, got %v", testCase.Name, testCase.ExpectedMemory, memory)
		}

	}
}

// Helper function to compare two integer slices
func equalIntSlices(a, b []int) bool {
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

// Helper function to compare two slices of interfaces
func equalInterfaceSlices(a, b []interface{}) bool {
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
