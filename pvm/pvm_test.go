package pvm

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"
)

// TestCase
type TestCase struct {
	Name           string        `json:"name"`
	InitialRegs    []uint32      `json:"initial-regs"`
	InitialPC      int           `json:"initial-pc"`
	InitialPageMap []PageMap     `json:"initial-page-map"`
	InitialMemory  []Page        `json:"initial-memory"`
	Code           []byte        `json:"code"`
	ExpectedStatus string        `json:"expected-status"`
	ExpectedRegs   []uint32      `json:"expected-regs"`
	ExpectedPC     int           `json:"expected-pc"`
	ExpectedMemory []interface{} `json:"expected-memory"`
}

func pvm_test(t *testing.T, tc TestCase) error {
	pvm := NewVM(tc.Code, tc.InitialRegs, tc.InitialPC, tc.InitialPageMap, tc.InitialMemory)
	pvm.Execute()
	// Check the registers
	if equalIntSlices(pvm.register, tc.ExpectedRegs) {
		fmt.Printf("REGISTER MATCH!\n")
	} else {
		fmt.Printf("Register mismatch for test %s: expected %v, got %v", tc.Name, tc.ExpectedRegs, pvm.register)
	}
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
	return nil // "trap", tc.InitialRegs, tc.InitialPC, tc.InitialMemory
}

func TestPVM(t *testing.T) {
	// Directory containing the JSON files
	dir := "../jamtestvectors/pvm/programs"

	// Read all files in the directory
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}

	for i, file := range files {
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

		fmt.Printf("\n\n** Test %d: %s **\n", i, file.Name())
		err = pvm_test(t, testCase)
		if err != nil {
			t.Fatalf("%v", err)
		}
	}
}

// Helper function to compare two integer slices
func equalIntSlices(a, b []uint32) bool {
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
