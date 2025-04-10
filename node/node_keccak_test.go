//go:build network
// +build network

package node

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/pvm"
	"github.com/stretchr/testify/assert"
)

func TestKeccak256(t *testing.T) {
	// read code from the file
	code, err1 := os.ReadFile(common.GetFilePath("/services/keccak256.pvm"))
	if err1 != nil {
		t.Errorf("Error reading file: %v", err1)
	}

	nodes, err := SetUpNodes(JCEDefault, numNodes)
	if err != nil {
		t.Fatalf("Error setting up nodes: %v\n", err)
	}
	// Wait for nodes to be ready
	fmt.Println("Waiting for nodes to be ready...")
	time.Sleep(2 * time.Second)
	targetdb := nodes[0].statedb
	fmt.Printf("keccak256 Code is: %v\n", code)
	input_byte := []byte("Hello, world!")
	vm := pvm.NewVMFromCode(0, code, 0, targetdb)
	vm.Ram.WriteRAMBytes(100, input_byte)
	vm.SetRegisterValue(10, 100)
	vm.SetRegisterValue(11, uint64(len(input_byte)))
	err = vm.Execute(5)
	if err != nil {
		t.Errorf("Error executing VM: %v", err)
	}
	// Check the result
	result, _ := vm.GetRegisterValue(10)
	fmt.Printf("A3: %v\n", result)
	result, _ = vm.GetRegisterValue(11)
	fmt.Printf("A4: %v\n", result)

	// read from the memory
	resultBytes, _ := vm.Ram.ReadRAMBytes(10, 32)
	fmt.Printf("Result: %x\n", resultBytes)

}

func TestXOR(t *testing.T) {
	// read code from the file
	code, err1 := os.ReadFile(common.GetFilePath("/services/xor.pvm"))
	if err1 != nil {
		t.Errorf("Error reading file: %v", err1)
	}
	nodes, err := SetUpNodes(JCEDefault, numNodes)
	if err != nil {
		t.Fatalf("Error setting up nodes: %v\n", err)
	}
	_ = nodes
	// Wait for nodes to be ready
	fmt.Println("Waiting for nodes to be ready...")
	time.Sleep(2 * time.Second)
	targetdb := nodes[0].statedb
	fmt.Printf("XOR Code is: %v\n", code)
	vm := pvm.NewVMFromCode(0, code, 0, targetdb)
	test_bytes := []byte{0x01, 0x02, 0x03, 0x04}
	vm.Ram.WriteRAMBytes(25, test_bytes)
	test_bytes2 := []byte{0x05, 0x06, 0x07, 0x08}
	vm.Ram.WriteRAMBytes(100, test_bytes2)
	vm.SetRegisterValue(10, 25)
	vm.SetRegisterValue(11, 100)
	err = vm.Execute(5)
	if err != nil {
		t.Errorf("Error executing VM: %v", err)
	}
	// Check the result
	result1, _ := vm.GetRegisterValue(10)
	fmt.Printf("A3: %v\n", result1)
	result2, _ := vm.GetRegisterValue(11)
	fmt.Printf("A4: %v\n", result2)
	resultXor := make([]byte, 8)
	for i := 0; i < 4; i++ {
		resultXor[i] = test_bytes[i] ^ test_bytes2[i]
	}

	fmt.Printf("ResultXor: %v\n", resultXor)
	result, _ := vm.Ram.ReadRAMBytes(uint32(result1), 8)
	fmt.Printf("Result: %v\n", result)
}

func TestXOR_Invoke(t *testing.T) {
	// read code from the file
	code, err1 := os.ReadFile(common.GetFilePath("/services/xor.pvm"))
	if err1 != nil {
		t.Errorf("Error reading file: %v", err1)
	}
	nodes, err := SetUpNodes(JCEDefault, numNodes)
	if err != nil {
		t.Fatalf("Error setting up nodes: %v\n", err)
	}
	// Wait for nodes to be ready
	fmt.Println("Waiting for nodes to be ready...")
	time.Sleep(2 * time.Second)
	targetdb := nodes[0].statedb
	fmt.Printf("XOR Code is: %v\n", code)
	vm := pvm.NewVMFromCode(0, code, 0, targetdb)
	var code_length = uint64(len(code))
	// use zero to load the code -- let the pages to be writable
	var valid_memory_address uint32 = 17 * 4096
	vm.Ram.SetPageAccess(17, 10, pvm.AccessMode{Readable: true, Writable: true, Inaccessible: false})
	vm.Ram.DebugStatus()
	vm.Ram.WriteRAMBytes(valid_memory_address+100, code) // load the code into the memory
	// machine
	vm.SetRegisterValue(7, uint64(valid_memory_address+100)) // code pointer
	vm.SetRegisterValue(8, code_length)                      // code length
	vm.SetRegisterValue(9, 0)                                // pc
	fmt.Println("Machine Start")
	vm.HostCheat("machine")
	fmt.Println("Machine End")
	// get the vm number
	vm2_num, _ := vm.GetRegisterValue(7) // get the new vm number
	fmt.Printf("New VM number: %v\n", vm2_num)
	// set up the register for Poke and put the input data into the ram
	//let [n, s, o, z] = ω7...11,, n= which vm, s = start address, o = n start address, z = length
	// we should set o to other register
	// We put the input data into the new vm's memory

	//we use zero to setup the new vm's memory too
	vm.SetRegisterValue(7, vm2_num)
	vm.SetRegisterValue(8, uint64(valid_memory_address)/4096)
	vm.SetRegisterValue(9, 5)
	errcode := vm.HostCheat("zero")
	if errcode != pvm.OK {
		t.Errorf("Error invoking VM: %v", errcode)
	}
	test_bytes := []byte{0x01, 0x02, 0x03, 0x04}
	vm.Ram.WriteRAMBytes(valid_memory_address+500, test_bytes)
	vm.SetRegisterValue(7, vm2_num)
	vm.SetRegisterValue(8, uint64(valid_memory_address+500))
	vm.SetRegisterValue(9, uint64(valid_memory_address+500))
	vm.SetRegisterValue(10, 4)
	fmt.Println("Poke Start")
	vm.HostCheat("poke")
	fmt.Println("Poke End")

	test_bytes2 := []byte{0x05, 0x06, 0x07, 0x08}
	vm.Ram.WriteRAMBytes(valid_memory_address+700, test_bytes2)
	vm.SetRegisterValue(7, vm2_num)
	vm.SetRegisterValue(8, uint64(valid_memory_address+700))
	vm.SetRegisterValue(9, uint64(valid_memory_address+700))
	vm.SetRegisterValue(10, 4)
	fmt.Println("Poke Start")
	vm.HostCheat("poke")
	fmt.Println("Poke End")
	// change the access mode of the page for the new vm first
	vm.SetRegisterValue(7, vm2_num)
	vm.SetRegisterValue(8, 4294967292/4096)
	vm.SetRegisterValue(9, 1)
	errcode = vm.HostCheat("zero")
	if errcode != pvm.OK {
		t.Errorf("Error invoking VM: %v", errcode)
	}
	// set up the memory for the vm (gas + register)
	// set up the register that can let the code run ==> pointer + length
	//13 regs
	regs := []uint64{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, uint64(valid_memory_address + 500), uint64(valid_memory_address + 700), 0}
	vm.PutGasAndRegistersToMemory(valid_memory_address+1500, 100, regs)

	// read it out and check
	gas_reg, errcode := vm.Ram.ReadRAMBytes(valid_memory_address+1500, 112)
	fmt.Printf("Gas and Registers: %v\n", gas_reg)
	vm.SetRegisterValue(7, vm2_num)
	vm.SetRegisterValue(8, uint64(valid_memory_address+1500))

	fmt.Println("Invoke Start")
	errcode = vm.HostCheat("invoke")
	if errcode != pvm.OK {
		t.Errorf("Error invoking VM: %v", errcode)
	}
	if vm.VMs[uint32(vm2_num)] == nil {
		t.Errorf("Error invoking VM: %v", err)
	}
	for i := 0; i < 13; i++ {
		value, err := vm.VMs[uint32(vm2_num)].GetRegisterValue(i)
		if err != pvm.OK {
			t.Errorf("Error getting register value: %v", err)
		}
		fmt.Printf("Regs[%d]: %v\n", i, value)
	}
	fmt.Println("Invoke End")
	// use that pointer and length to read the memory
	gas2, regs2, errCode := vm.GetGasAndRegistersFromMemory(valid_memory_address + 1500)
	if errCode != pvm.OK {
		t.Errorf("Error getting gas and registers from memory: %v", errCode)
	}
	fmt.Printf("Gas: %v\n", gas2)
	fmt.Printf("Register[10]: %v\n", regs2[10])
	fmt.Printf("Register[11]: %v\n", regs2[11])
	vm.SetRegisterValue(7, vm2_num)
	vm.SetRegisterValue(8, uint64(valid_memory_address+5000)) // start address for original vm
	vm.SetRegisterValue(9, regs2[10])                         // start address for new vm
	vm.SetRegisterValue(10, regs2[11])
	fmt.Println("Peek Start")
	time.Sleep(2 * time.Second)
	vm.HostCheat("peek")
	fmt.Println("Peek End")
	// read the memory and compare the result
	data, errcode := vm.Ram.ReadRAMBytes(valid_memory_address+5000, uint32(regs2[11]))
	if errcode != pvm.OK {
		t.Errorf("Error reading data from memory: %v", errcode)
	} else if len(data) == 0 {
		t.Errorf("Error reading data from memory: %v", errcode)
	}
	test_data := make([]byte, len(test_bytes))
	for i := range test_bytes {
		test_data[i] = test_bytes[i] ^ test_bytes2[i]
	}
	fmt.Printf("Data: %v\n", data)
	fmt.Printf("Test Data: %v\n", test_data)
	if assert.Equal(t, test_data, data) {
		fmt.Println("Test Success")
	}
	vm.SetRegisterValue(7, vm2_num)
	vm.HostCheat("expunge")
}
func TestAdd(t *testing.T) {
	// read code from the file
	code, err1 := os.ReadFile(common.GetFilePath("/services/and.pvm"))
	if err1 != nil {
		t.Errorf("Error reading file: %v", err1)
	}
	nodes, err := SetUpNodes(JCEDefault, numNodes)
	if err != nil {
		t.Fatalf("Error setting up nodes: %v\n", err)
	}
	// Wait for nodes to be ready
	fmt.Println("Waiting for nodes to be ready...")
	time.Sleep(2 * time.Second)
	targetdb := nodes[0].statedb
	fmt.Printf("XOR Code is: %v\n", code)
	vm := pvm.NewVMFromCode(0, code, 0, targetdb)
	test_bytes := []byte{0x01, 0x02, 0x03, 0x04}
	vm.Ram.WriteRAMBytes(25, test_bytes)
	test_bytes2 := []byte{0x01, 0x00, 0x00, 0x04}
	vm.Ram.WriteRAMBytes(100, test_bytes2)
	vm.SetRegisterValue(10, 25)
	vm.SetRegisterValue(11, 100)
	err = vm.Execute(5)
	if err != nil {
		t.Errorf("Error executing VM: %v", err)
	}
	// Check the result
	result1, _ := vm.GetRegisterValue(10)
	fmt.Printf("A3: %v\n", result1)
	result2, _ := vm.GetRegisterValue(11)
	fmt.Printf("A4: %v\n", result2)
	resultXor := make([]byte, 8)
	for i := 0; i < 4; i++ {
		resultXor[i] = test_bytes[i] & test_bytes2[i]
	}

	fmt.Printf("ResultXor: %v\n", resultXor)
	result, _ := vm.Ram.ReadRAMBytes(uint32(result1), 8)
	fmt.Printf("Result: %v\n", result)
}
