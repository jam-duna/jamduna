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

	"github.com/colorfulnotion/jam/common"
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

func pvm_test(t *testing.T, tc TestCase) (error, int) {
	var num_mismatch int
	fmt.Printf("Test case: %s\n", tc.Name)

	// if tc.Name != "inst_div_signed_64" {
	// 	return nil, 0
	// }

	hostENV := NewMockHostEnv()
	serviceAcct := uint32(0) // stub
	pvm := NewVM(serviceAcct, tc.Code, tc.InitialRegs, uint64(tc.InitialPC), hostENV, false)
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

	pvm.Execute(int(tc.InitialPC))
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
		err, num_mismatch = pvm_test(t, testCase)
		if err != nil {
			t.Fatalf("%v", err)
		}
		total_mismatch += num_mismatch
	}
	// show the match rate
	fmt.Printf("Match rate: %v/%v\n", count-total_mismatch, count)
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

// Helper function to compare two byte slices
func equalByteSlices_simple(a []byte, b []byte) bool {

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

func testPVM_invoke(t *testing.T) {

	// put code into ram
	// get the address of the code and length (po, pz)
	// put it into the register (7,8,9) = (po, pz, program counter)
	code, err1 := os.ReadFile(common.GetFilePath("/services/xor.pvm"))
	vm := NewVMFromCode(0, code, 0, nil)
	// put the code into the ram and get the address and length
	if err1 != nil {
		t.Errorf("Error reading file: %v", err1)
	}
	var code_length = uint64(len(code))
	vm.Ram.WriteRAMBytes(100, code)
	vm.SetRegisterValue(7, 100)
	vm.SetRegisterValue(8, code_length)
	vm.SetRegisterValue(9, 0)
	vm.hostMachine()
	// get the vm number
	vm2_num, _ := vm.GetRegisterValue(7)
	fmt.Printf("New VM number: %v\n", vm2_num)
	// set up the register for Poke and put the input data into the ram
	//let [n, s, o, z] = ω7...11,, n= which vm, s = start address, o = n start address, z = length
	// we should set o to other register
	test_bytes := []byte{0x01, 0x02, 0x03, 0x04}
	vm.Ram.WriteRAMBytes(25, test_bytes)
	vm.SetRegisterValue(7, vm2_num)
	vm.SetRegisterValue(8, 25)
	vm.SetRegisterValue(9, 25)
	vm.SetRegisterValue(10, 4)
	vm.hostPoke()

	test_bytes2 := []byte{0x05, 0x06, 0x07, 0x08}
	vm.Ram.WriteRAMBytes(100, test_bytes2)
	vm.SetRegisterValue(7, vm2_num)
	vm.SetRegisterValue(8, 100)
	vm.SetRegisterValue(9, 100)
	vm.SetRegisterValue(10, 4)

	// set up the memory for the vm (gas + register)
	// set up the register that can let the code run ==> pointer + length
	//13 regs
	regs := []uint64{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 25, 100, 0}
	vm.PutGasAndRegistersToMemory(300, 100, regs)
	// if err != OK {
	// 	t.Errorf("Error putting gas and registers to memory: %v", err)
	// }
	// read the input and then hash it
	// put it back to the memory
	// put the pointer and length to the register
	// it will return back to the parent vm memory
	vm.SetRegisterValue(7, vm2_num)
	vm.SetRegisterValue(8, 300)
	vm.hostInvoke()
	// use that pointer and length to read the memory
	gas2, regs2, errCode := vm.GetGasAndRegistersFromMemory(300)
	if errCode != OK {
		t.Errorf("Error getting gas and registers from memory: %v", errCode)
	}
	fmt.Printf("Gas: %v\n", gas2)
	fmt.Printf("Registers: %v\n", regs2[10])
	fmt.Printf("Registers: %v\n", regs2[11])
	vm.SetRegisterValue(7, vm2_num)
	vm.SetRegisterValue(8, regs2[10])
	vm.SetRegisterValue(9, regs2[10])
	vm.SetRegisterValue(10, regs2[11])
	vm.hostPeek()
	data, errcode := vm.Ram.ReadRAMBytes(uint32(regs2[10]), uint32(regs2[11]))
	if errcode != OK {
		t.Errorf("Error reading data from memory: %v", errcode)
	}
	fmt.Printf("Data: %v\n", data)
	// except (4,4,4,12)
	vm.SetRegisterValue(7, vm2_num)
	vm.hostExpunge()
}
