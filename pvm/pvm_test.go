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
	InitialPC      uint32        `json:"initial-pc"`
	InitialPageMap []PageMap     `json:"initial-page-map"`
	InitialMemory  []Page        `json:"initial-memory"`
	Code           []byte        `json:"program"`
	ExpectedStatus string        `json:"expected-status"`
	ExpectedRegs   []uint32      `json:"expected-regs"`
	ExpectedPC     int           `json:"expected-pc"`
	ExpectedMemory []interface{} `json:"expected-memory"`
}

// TestCase for hostfuntion
type hostfun_TestCase struct {
	Name           string    `json:"name"`
	InitialRegs    []uint32  `json:"initial-regs"`
	InitialPageMap []PageMap `json:"initial-page-map"`
	InitialMemory  []Page    `json:"initial-memory"`
	ExpectedRegs   []uint32  `json:"expected-regs"`
	ExpectedMemory []Page    `json:"expected-memory"`
}

func hostfun_test(t *testing.T, hftc hostfun_TestCase) error {
	hostENV := NewMockHostEnv()
	pvm := NewVMforhostfun(hftc.InitialRegs, hftc.InitialPageMap, hftc.InitialMemory, hostENV)
	fmt.Println("PVM initial register: ", pvm.register)
	// fmt.Println("PVM initial ram: ", pvm.ram[131072:131072+10])
	// pvm.hostRead()
	// pvm.hostWrite()
	// pvm.hostExport(0)
	// pvm.hostImport()
	// pvm.hostHistoricalLookup(10)
	pvm.hostSolicit()
	// pvm.hostLookup()

	// Check the registers
	// if equalIntSlices(pvm.register, hftc.ExpectedRegs) {
	// 	fmt.Printf("REGISTER MATCH!\n")
	// } else {
	// 	fmt.Printf("Register mismatch for test %s: expected %v, got %v", hftc.Name, hftc.ExpectedRegs, pvm.register)
	// }

	// Check the rram
	// start_address := hftc.ExpectedMemory[0].Address
	// expected_contents := hftc.ExpectedMemory[0].Contents
	// vm_contents := pvm.ram[start_address : start_address+uint32(len(expected_contents))]

	// if equalByteSlices(expected_contents, vm_contents) {
	// 	fmt.Printf("RAM MATCH!\n")
	// } else {
	// 	expect_result := make([]byte, len(expected_contents))
	// 	vm_result := make([]byte, len(expected_contents))

	// 	for i := range expected_contents {
	// 		expect_result[i] = expected_contents[i]
	// 		vm_result[i] = pvm.ram[start_address+uint32(i)]
	// 	}
	// 	fmt.Printf("RAM mismatch for test %s\n", hftc.Name)
	// 	fmt.Printf("RAM start from %v should be %v", hftc.ExpectedMemory[0].Address, expected_contents)
	// }

	return nil // "trap", tc.InitialRegs, tc.InitialPC, tc.InitialMemory
}

func TestHostfun(t *testing.T) {
	// Directory containing the JSON files
	dir := "../jamtestvectors/host_funtion/tests"

	// input the file that you want to check
	targetFiles := []string{
		// "hostRead_VLEN.json",
		// "hostRead_NONE.json",
		// "hostWrite_l.json",
		// "hostLookup_VLEN.json",
		// "hostLookup_NONE.json",
		"hostSolicit_OK.json",
		// "hostSolicit_HUH.json",
		// "hostHistoricalLookup_VLEN.json",
		// "hostHistoricalLookup_NONE.json",
		// "hostImport_OK.json",
		// "hostImport_NONE.json",
		// "hostExport_OK.json",
		// "hostExport_FULL.json",
	}

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

		// Check if the file is in the targetFiles list
		if !contains(targetFiles, file.Name()) {
			continue
		}

		filePath := filepath.Join(dir, file.Name())

		data, err := ioutil.ReadFile(filePath)
		if err != nil {
			t.Fatalf("Failed to read file %s: %v", filePath, err)
		}
		var testCase hostfun_TestCase
		err = json.Unmarshal(data, &testCase)
		if err != nil {
			t.Fatalf("Failed to unmarshal JSON from file %s: %v", filePath, err)
		}

		fmt.Printf("\n\n** Test %d: %s **\n", i, file.Name())

		err = hostfun_test(t, testCase)
		if err != nil {
			t.Fatalf("%v", err)
		}
	}
}

func pvm_test(t *testing.T, tc TestCase) error {
	hostENV := NewMockHostEnv()
	pvm := NewVM(tc.Code, tc.InitialRegs, tc.InitialPC, tc.InitialPageMap, tc.InitialMemory, hostENV)
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

	// input the file that you want to check
	targetFiles := []string{
		// 	"inst_trap.json",
		//  "inst_add.json",
		"ecalli_read_write.json",
		// 	"inst_add_imm.json",
		// 	"inst_add_with_overflow.json",
		// 	"inst_and.json",
		// 	"inst_and_imm.json",

		// 	"inst_cmov_if_zero_imm_nok.json",
		// 	"inst_cmov_if_zero_imm_ok.json",
		// 	"inst_cmov_if_zero_nok.json",
		// 	"inst_cmov_if_zero_ok.json",

		// 	"inst_div_signed.json",
		// 	"inst_div_signed_by_zero.json",
		// 	"inst_div_signed_with_overflow.json",
		// 	"inst_div_unsigned.json",
		// 	"inst_div_unsigned_by_zero.json",
		// 	"inst_div_unsigned_with_overflow.json",

		// 	"inst_jump.json",

		// 	"inst_mul.json",
		// 	"inst_mul_imm.json",

		// 	"inst_or.json",
		// 	"inst_or_imm.json",
		// 	"inst_sub.json",
		// 	"inst_sub_imm.json",
		// 	"inst_sub_with_overflow.json",
		// 	"inst_xor.json",
		// 	"inst_xor_imm.json",

		// 	"inst_load_imm.json",
		// 	"inst_load_u8.json",
		// 	"inst_load_u8_trap.json",

		// 	"inst_move_reg.json",
		// 	"inst_negate_and_add_imm.json",

		// 	"inst_rem_signed.json",
		// 	"inst_rem_signed_by_zero.json",
		// 	"inst_rem_signed_with_overflow.json",
		// 	"inst_rem_unsigned.json",
		// 	"inst_rem_unsigned_by_zero.json",
		// 	"inst_rem_unsigned_with_overflow.json",

		// 	"inst_set_greater_than_unsigned_imm_0.json",
		// 	"inst_set_greater_than_unsigned_imm_1.json",

		// 	"inst_set_less_than_signed_0.json",
		// 	"inst_set_less_than_signed_1.json",
		// 	"inst_set_less_than_signed_imm_0.json",
		// 	"inst_set_less_than_signed_imm_1.json",
		// 	"inst_set_less_than_unsigned_0.json",
		// 	"inst_set_less_than_unsigned_1.json",
		// 	"inst_set_less_than_unsigned_imm_0.json",
		// 	"inst_set_less_than_unsigned_imm_1.json",

		// 	"inst_set_greater_than_signed_imm_0.json",
		// 	"inst_set_greater_than_signed_imm_1.json",

		// 	"inst_shift_arithmetic_right.json",
		// 	"inst_shift_arithmetic_right_imm.json",
		// 	"inst_shift_arithmetic_right_imm_alt.json",
		// 	"inst_shift_arithmetic_right_with_overflow.json",

		// 	"inst_shift_logical_left.json",
		// 	"inst_shift_logical_left_imm.json",
		// 	"inst_shift_logical_left_imm_alt.json",
		// 	"inst_shift_logical_left_with_overflow.json",
		// 	"inst_shift_logical_right.json",
		// 	"inst_shift_logical_right_imm.json",
		// 	"inst_shift_logical_right_imm_alt.json",
		// 	"inst_shift_logical_right_with_overflow.json",

		// 	"inst_store_u8.json",
		// 	"inst_store_u8_trap_inaccessible.json",
		// 	"inst_store_u8_trap_read_only.json",
		// 	"inst_store_u16.json",
		// 	"inst_store_u32.json",

		// 	"inst_branch_eq_imm_nok.json",
		// 	"inst_branch_eq_imm_ok.json",
		// 	"inst_branch_greater_or_equal_signed_imm_ok.json",
		// 	"inst_branch_greater_or_equal_signed_imm_nok.json",
		// 	"inst_branch_greater_or_equal_unsigned_imm_ok.json",
		// 	"inst_branch_greater_or_equal_unsigned_imm_nok.json",
		// 	"inst_branch_greater_signed_imm_nok.json",
		// 	"inst_branch_greater_signed_imm_ok.json",
		// 	"inst_branch_greater_unsigned_imm_nok.json",
		// 	"inst_branch_greater_unsigned_imm_ok.json",
		// 	"inst_branch_less_or_equal_signed_imm_nok.json",
		// 	"inst_branch_less_or_equal_signed_imm_ok.json",
		// 	"inst_branch_less_or_equal_unsigned_imm_nok.json",
		// 	"inst_branch_less_or_equal_unsigned_imm_ok.json",
		// 	"inst_branch_less_signed_imm_nok.json",
		// 	"inst_branch_less_signed_imm_ok.json",
		// 	"inst_branch_less_unsigned_imm_nok.json",
		// 	"inst_branch_less_unsigned_imm_ok.json",
		// 	"inst_branch_not_eq_imm_ok.json",
		// 	"inst_branch_not_eq_imm_nok.json",

		// 	"inst_branch_eq_nok.json",
		// 	"inst_branch_eq_ok.json",
		// 	"inst_branch_not_eq_nok.json",
		// 	"inst_branch_not_eq_ok.json",
		// 	"inst_branch_less_unsigned_nok.json",
		// 	"inst_branch_less_unsigned_ok.json",
		// 	"inst_branch_less_signed_nok.json",
		// 	"inst_branch_less_signed_ok.json",
		// 	"inst_branch_greater_or_equal_signed_nok.json",
		// 	"inst_branch_greater_or_equal_signed_ok.json",
		// 	"inst_branch_greater_or_equal_unsigned_nok.json",
		// 	"inst_branch_greater_or_equal_unsigned_ok.json",

		// "inst_ret_halt.json",
		// "inst_ret_invalid.json",
	}

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

		// Check if the file is in the targetFiles list
		if !contains(targetFiles, file.Name()) {
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

// Helper function to check if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
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

// Helper function to compare two byte slices
func equalByteSlices(a []byte, b []byte) bool {

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
