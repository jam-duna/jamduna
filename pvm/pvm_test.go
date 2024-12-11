package pvm

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

// TestCase
type TestCase struct {
	Name        string   `json:"name"`
	InitialRegs []uint64 `json:"initial-regs"`
	InitialPC   uint32   `json:"initial-pc"`
	//InitialPageMap []PageMap     `json:"initial-page-map"`
	InitialMemory  []Page        `json:"initial-memory"`
	Code           []byte        `json:"program"`
	ExpectedStatus string        `json:"expected-status"`
	ExpectedRegs   []uint64      `json:"expected-regs"`
	ExpectedPC     int           `json:"expected-pc"`
	ExpectedMemory []interface{} `json:"expected-memory"`
}

// TestCase for hostfuntion
type hostfun_TestCase struct {
	Name        string   `json:"name"`
	InitialRegs []uint32 `json:"initial-regs"`
	//InitialPageMap []PageMap `json:"initial-page-map"`
	InitialMemory  []Page   `json:"initial-memory"`
	ExpectedRegs   []uint32 `json:"expected-regs"`
	ExpectedMemory []Page   `json:"expected-memory"`
}

/*
	func TestProgramCodec(t *testing.T) {
		bytecode := []byte{1, 1, 14, 8, 4, 7, 2, 17, 19, 7, 2, 0, 4, 8, 239, 190, 173, 222, 153, 193, 0}

		bytecodeDecoded := // fix DecodeProgram(bytecode)
		PrintProgam(bytecodeDecoded)

		bytecodeEncoded := EncodeProgram(bytecodeDecoded)
		fmt.Println("Encoded: ", bytecodeEncoded)
		if equalByteSlices(bytecode, bytecodeEncoded) {
			fmt.Println("Bytecode match!")
		}

		fmt.Println("--------------------------------------------------")

		bytecode = []byte{
			0, 0, 100,
			4, 0, 1, 4, 1, 0, 0, 254, 254,
			4, 2, 12, 78, 16, 4, 1, 30, 16, 62, 10, 1,
			4, 0, 254, 254, 10, 2, 8, 0, 254, 254,
			22, 1, 8, 0, 254, 254, 8, 33, 1, 22, 1,
			4, 0, 254, 254, 4, 3, 1, 10, 0, 0, 0, 254, 254,
			8, 48, 0, 22, 0, 0, 0, 254, 254, 5, 2,
			4, 0, 0, 0, 254, 254, 4, 1, 12, 78, 17, 0, 38,
			4, 0, 0, 254, 254, 1, 38, 4, 4, 0, 254, 254, 1,
			38, 4, 8, 0, 254, 254, 5, 177, 9, 82, 9, 130,
			32, 65, 130, 4, 5, 105, 32, 16, 244, 0,
		}

		bytecodeDecoded = DecodeProgram(bytecode)
		PrintProgam(bytecodeDecoded)

		bytecodeEncoded = EncodeProgram(bytecodeDecoded)
		fmt.Println("Encoded: ", bytecodeEncoded)
		if equalByteSlices(bytecode, bytecodeEncoded) {
			fmt.Println("Bytecode match!")
		}

}
*/
func TestCodec(t *testing.T) {
	powers := []uint64{
		1 << 0,  // 2^0
		1 << 7,  // 2^7
		1 << 14, // 2^14
		1 << 21, // 2^21
		1 << 28, // 2^28
		1 << 35, // 2^35
		1 << 42, // 2^42
		1 << 49, // 2^49
	}

	for _, p := range powers {
		// encode
		encodedResult := types.E(p)
		fmt.Printf("Input: %d, Encoded: %v\n", p, encodedResult)
		// decode
		decodedValue, length := types.DecodeE(encodedResult)
		fmt.Printf("Decoded Value: %d, Length: %d\n", decodedValue, length)
		if decodedValue != p {
			t.Errorf("Decoded value (%d) does not match original value (%d)", decodedValue, p)
		}
		fmt.Println("--------------------------------------------------")
	}
}

func hostfun_test(t *testing.T, hftc hostfun_TestCase) error {
	//hostENV := NewMockHostEnv()
	//pvm := NewVMforhostfun(hftc.InitialRegs, hftc.InitialPageMap, hftc.InitialMemory, hostENV)
	//fmt.Println("PVM initial register: ", pvm.register)
	// fmt.Println("PVM initial ram: ", pvm.ram[131072:131072+10])
	// pvm.hostRead()
	// pvm.hostWrite()
	// pvm.hostExport(0)
	// pvm.hostImport()
	// pvm.hostHistoricalLookup(10)
	//	pvm.hostSolicit()
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

func testHostfun(t *testing.T) {
	// Directory containing the JSON files
	dir := "../jamtestvectors/host_function/tests"

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
	serviceAcct := uint32(0) // stub
	pvm := NewVM(serviceAcct, tc.Code, tc.InitialRegs, tc.InitialPC, hostENV)
	pvm.Execute(types.EntryPointGeneric)
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

func testReadWriteRAM(t *testing.T) {
	const (
		testSize      = 1024 * 1024 // 4MB
		numIterations = 1000
		memorySize    = 0xFFFFFFFF
	)

	rand.Seed(time.Now().UnixNano())

	for i := 0; i < numIterations; i++ {
		// Initialize the VM with a RAM of sufficient size
		vm := &VM{}

		// Create a random byte pattern of 1MB
		data := make([]byte, testSize)
		rand.Read(data)

		// Choose a random start address within the RAM
		startAddress := uint32(rand.Intn(memorySize - testSize))

		// Write the random byte pattern to the RAM
		status := vm.Ram.WriteRAMBytes(startAddress, data)
		if status != OK {
			t.Fatalf("Iteration %d: Failed to write data to RAM", i)
		}

		// Read the data back from the RAM
		readData, status := vm.Ram.ReadRAMBytes(startAddress, testSize)
		if status != OK {
			t.Fatalf("Iteration %d: Failed to read data from RAM", i)
		}

		// Verify that the data read matches the data written
		if len(readData) != len(data) {
			t.Fatalf("Iteration %d: Length mismatch between written and read data", i)
		}

		for j := range data {
			if data[j] != readData[j] {
				t.Fatalf("Iteration %d: Data mismatch at byte %d", i, j)
			}
		}
		if i%100 == 0 {
			fmt.Printf("%d out of %d\n", i, numIterations)
		}
	}
}

// awaiting 64 bit
func testPVM(t *testing.T) {
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
	err := vm.PutGasAndRegistersToMemory(300, 100, regs)
	if err != OK {
		t.Errorf("Error putting gas and registers to memory: %v", err)
	}
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
