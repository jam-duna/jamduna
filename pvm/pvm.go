package pvm

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

const (
	regSize  = 13
	numCores = 341
	W_C      = 642
	W_S      = 6
	W_X      = 1024
	M        = 128
	V        = 1023
	D        = 28800
)

// PageMap
type PageMap struct {
	Address    uint32 `json:"address"`
	Length     int    `json:"length"`
	IsWritable bool   `json:"is-writable"`
}

// Page holds a byte array
type Page struct {
	Address  uint32 `json:"address"`
	Contents []byte `json:"contents"`
}

type VM struct {
	JSize               int
	Z                   int
	J                   []byte
	code                []byte
	bitmask             string // K in GP
	pc                  uint32 // Program counter
	terminated          bool
	hostCall            bool   // ̵h in GP
	host_func_id        uint32 // h in GP
	ram                 map[uint32][4096]byte
	register            []uint32
	ξ                   uint64
	hostenv             types.HostEnv
	writable_ram_start  uint32
	writable_ram_length uint32
	service_index       uint32

	VMs map[uint32]*VM

	// Work Package Inputs
	extrinsics []types.WorkItemExtrinsic
	payload    []byte
	Imports    [][]byte
	Exports    [][]byte

	// SOLICITS+FORGETS
	Solicits []Solicit
	Forgets  []Forgets
}

type Forgets struct {
}

type Solicit struct {
	BlobHash common.Hash
	Length   uint32
}

type Program struct {
	JSize int
	Z     int // 1 byte
	CSize int
	J     []byte
	Code  []byte
	//K     []byte
	K []string
}

// Appendix A - Instuctions

// A.5.1. Instructions without Arguments.
const (
	TRAP        = 0
	FALLTHROUGH = 17
)

// A.5.2. Instructions with Arguments of One Immediate.
const (
	ECALLI = 78
)

// A.5.3. Instructions with Arguments of Two Immediates.
const (
	STORE_IMM_U8  = 62
	STORE_IMM_U16 = 79
	STORE_IMM_U32 = 38
)

// A.5.4. Instructions with Arguments of One Offset.
const (
	JUMP = 5
)

// A.5.5. Instructions with Arguments of One Register & One Immediate.
const (
	JUMP_IND  = 19
	LOAD_IMM  = 4
	LOAD_U8   = 60
	LOAD_U16  = 76
	LOAD_U32  = 10
	STORE_U8  = 71
	STORE_U16 = 69
	STORE_U32 = 22
)

// A.5.6. Instructions with Arguments of One Register & Two Immediates.
const (
	STORE_IMM_IND_U8  = 26
	STORE_IMM_IND_U16 = 54
	STORE_IMM_IND_U32 = 13
)

// A.5.7. Instructions with Arguments of One Register, One Immediate and One Offset.
const (
	LOAD_IMM_JUMP   = 6
	BRANCH_EQ_IMM   = 7
	BRANCH_NE_IMM   = 15
	BRANCH_LT_U_IMM = 44
	BRANCH_LE_U_IMM = 59
	BRANCH_GE_U_IMM = 52
	BRANCH_GT_U_IMM = 50
	BRANCH_LT_S_IMM = 32
	BRANCH_LE_S_IMM = 46
	BRANCH_GE_S_IMM = 45
	BRANCH_GT_S_IMM = 53
)

// A.5.8. Instructions with Arguments of Two Registers.
const (
	MOVE_REG = 82
	SBRK     = 87
)

// A.5.9. Instructions with Arguments of Two Registers & One Immediate.
const (
	STORE_IND_U8      = 16
	STORE_IND_U16     = 29
	STORE_IND_U32     = 3
	LOAD_IND_U8       = 11
	LOAD_IND_I8       = 21
	LOAD_IND_U16      = 37
	LOAD_IND_I16      = 33
	LOAD_IND_U32      = 1
	ADD_IMM           = 2
	AND_IMM           = 18
	XOR_IMM           = 31
	OR_IMM            = 49
	MUL_IMM           = 35
	MUL_UPPER_S_S_IMM = 65
	MUL_UPPER_U_U_IMM = 63
	SET_LT_U_IMM      = 27
	SET_LT_S_IMM      = 56
	SHLO_L_IMM        = 9
	SHLO_R_IMM        = 14
	SHAR_R_IMM        = 25
	NEG_ADD_IMM       = 40
	SET_GT_U_IMM      = 39
	SET_GT_S_IMM      = 61
	SHLO_R_IMM_ALT    = 72
	SHLO_L_IMM_ALT    = 75
	SHAR_L_IMM_ALT    = 80
	CMOV_IMM_IZ       = 85 //added base on 2024/7/1 GP
	CMOV_IMM_NZ       = 86 //added base on 2024/7/1 GP
)

// A.5.10. Instructions with Arguments of Two Registers & One Offset.
const (
	BRANCH_EQ   = 24
	BRANCH_NE   = 30
	BRANCH_LT_U = 47
	BRANCH_LT_S = 48
	BRANCH_GE_U = 41
	BRANCH_GE_S = 43
)

// A.5.11. Instruction with Arguments of Two Registers and Two Immediates.
const (
	LOAD_IMM_JUMP_IND = 42
)

// A.5.12. Instructions with Arguments of Three Registers.
const (
	ADD_REG           = 8
	SUB_REG           = 20
	AND_REG           = 23
	XOR_REG           = 28
	OR_REG            = 12
	MUL_REG           = 34
	MUL_UPPER_S_S_REG = 67
	MUL_UPPER_U_U_REG = 57
	MUL_UPPER_S_U_REG = 81
	DIV_U             = 68
	DIV_S             = 64
	REM_U             = 73
	REM_S             = 70
	SET_LT_U          = 36
	SET_LT_S          = 58
	SHLO_L            = 55
	SHLO_R            = 51
	SHAR_R            = 77
	CMOV_IZ           = 83
	CMOV_NZ           = 84
)

// Termination Instructions
var T = map[int]struct{}{
	TRAP:            {},
	FALLTHROUGH:     {},
	JUMP:            {},
	JUMP_IND:        {},
	LOAD_IMM_JUMP:   {},
	BRANCH_EQ_IMM:   {},
	BRANCH_NE_IMM:   {},
	BRANCH_LT_U_IMM: {},
	BRANCH_LE_U_IMM: {},
	BRANCH_GE_U_IMM: {},
	BRANCH_GT_U_IMM: {},
	BRANCH_LT_S_IMM: {},
	BRANCH_LE_S_IMM: {},
	BRANCH_GE_S_IMM: {},
	BRANCH_GT_S_IMM: {},
	BRANCH_EQ:       {},
	BRANCH_NE:       {},
	BRANCH_LT_U:     {},
	BRANCH_LT_S:     {},
	BRANCH_GE_U:     {},
	BRANCH_GE_S:     {},
}

const (
	NONE = (1 << 32) - 1  // 2^32 - 1
	OOB  = (1 << 32) - 2  // 2^32 - 2
	WHO  = (1 << 32) - 3  // 2^32 - 3
	FULL = (1 << 32) - 4  // 2^32 - 4
	CORE = (1 << 32) - 5  // 2^32 - 5
	CASH = (1 << 32) - 6  // 2^32 - 6
	LOW  = (1 << 32) - 7  // 2^32 - 7
	HIGH = (1 << 32) - 8  // 2^32 - 8
	WAT  = (1 << 32) - 9  // 2^32 - 9
	HUH  = (1 << 32) - 10 // 2^32 - 10
	OK   = 0              // 0

	HALT  = 0              // 0
	PANIC = (1 << 32) - 12 // 2^32 - 12
	FAULT = (1 << 32) - 13 // 2^32 - 13
	HOST  = (1 << 32) - 14 // 2^32 - 14

	BAD = 1111
	S   = 2222
	BIG = 3333
)

func parseProgram(p []byte) *Program {
	reverseString := func(s string) string {
		runes := []rune(s)
		for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
			runes[i], runes[j] = runes[j], runes[i]
		}
		return string(runes)
	}

	fmt.Printf("JSize=%d Z=%d CSize=%d\n", p[0], p[1], p[2])
	kBytes := p[3+p[0]+p[2]:]
	var kCombined string
	for _, b := range kBytes {
		binaryStr := fmt.Sprintf("%08b", b)
		kCombined += reverseString(binaryStr)
	}
	if len(kCombined) > int(p[2]) {
		kCombined = kCombined[:int(p[2])]
	}

	return &Program{
		JSize: int(p[0]),
		Z:     int(p[1]),
		CSize: int(p[2]),
		J:     p[3 : 3+p[0]],
		Code:  p[3+p[0] : 3+p[0]+p[2]],
		K:     []string{kCombined},
	}
}

// NewVM initializes a new VM with a given program
func NewVM(service_index uint32, code []byte, initialRegs []uint32, initialPC uint32, pagemap []PageMap, pages []Page, hostENV types.HostEnv) *VM {
	if len(code) == 0 {
		panic("NO CODE\n")
	}
	p := parseProgram(code)
	fmt.Printf("Code: %v K(bitmask): %v\n", p.Code, p.K[0])
	fmt.Println("================================================================")
	vm := &VM{
		JSize:         p.JSize,
		Z:             p.Z,
		J:             p.J,
		code:          p.Code,
		bitmask:       p.K[0], // pass in bitmask K
		register:      make([]uint32, regSize),
		pc:            initialPC,
		ram:           make(map[uint32][4096]byte),
		hostenv:       hostENV, //check if we need this
		Exports:       make([][]byte, 0),
		service_index: service_index,
	}
	for _, pg := range pages {
		vm.writeRAMBytes(pg.Address, pg.Contents)
	}
	copy(vm.register, initialRegs)
	return vm
}

func NewVMFromCode(serviceIndex uint32, code []byte, i uint32, hostENV types.HostEnv) *VM {
	return NewVM(serviceIndex, code, []uint32{}, i, []PageMap{}, []Page{}, hostENV)
}

func NewForceCreateVM(code []byte, bitmask string, hostENV types.HostEnv) *VM {
	return &VM{
		code:    code,
		bitmask: bitmask,
		hostenv: hostENV,
	}
}

// for hostfuntion test
func NewVMforhostfun(initialRegs []uint32, pagemap []PageMap, pages []Page, hostENV types.HostEnv) *VM {

	vm := &VM{
		register: make([]uint32, regSize),
		ram:      make(map[uint32][4096]byte),
		hostenv:  hostENV,
	}
	for _, pg := range pages {
		vm.writeRAMBytes(pg.Address, pg.Contents)
	}

	if pagemap[0].IsWritable {
		vm.writable_ram_start = uint32(pagemap[0].Address)
		vm.writable_ram_length = uint32(pagemap[0].Length)
	}
	copy(vm.register, initialRegs)
	return vm
}

// Execute runs the program until it terminates
func (vm *VM) Execute(entryPoint string) error {
	if len(entryPoint) > 0 {
		// TODO: lookup entrypoint in jump table
	}
	for !vm.terminated {
		if err := vm.step(); err != nil {
			return err
		}
		// host call invocation
		if vm.hostCall {
			fmt.Println("Invocate Host Funtion: ", vm.host_func_id)
			vm.InvokeHostCall(int(vm.host_func_id))
			vm.hostCall = false
			vm.terminated = false
		}
		fmt.Println("-----------------------------------------------------------------------")
	}
	fmt.Println("last pc: ", vm.pc)
	return nil
}

// set ups extrinsics and payload
func (vm *VM) SetExtrinsicsPayload(extrinsics []types.WorkItemExtrinsic, payload []byte) error {
	vm.extrinsics = extrinsics
	vm.payload = payload
	return nil
}

// set up Imports
func (vm *VM) SetImports(imports [][]byte) error {
	vm.Imports = imports
	return nil
}

// copy a into 2^32 - Z_Q - Z_I and initialize registers
func (vm *VM) SetArgumentInputs(a []byte) error {
	vm.writeRegister(0, 0)
	vm.writeRegister(1, 0xFFFF0000) // 2^32 - 2^16
	vm.writeRegister(2, 0xFEFE0000) // 2^32 - 2 * 2^16 - 2^24
	for r := 3; r < 10; r++ {
		vm.writeRegister(r, 0)
	}
	vm.writeRegister(10, 0xFEFF0000) // 2^32 - 2^16 - 2^24
	vm.writeRegister(11, uint32(len(a)))
	for r := 12; r < 16; r++ {
		vm.writeRegister(r, 0)
	}
	vm.writeRAMBytes(0xFEFF0000, a)
	return nil
}

func (vm *VM) GetArgumentOutputs(a []byte) ([]byte, uint32) {
	o, _ := vm.readRegister(10)
	l, _ := vm.readRegister(11)
	output, res := vm.readRAMBytes(o, int(l))
	return output, res
}

// step performs a single step in the PVM
func (vm *VM) step() error {
	if vm.pc >= uint32(len(vm.code)) {
		return errors.New("program counter out of bounds")
	}

	instr := vm.code[vm.pc]
	opcode := instr
	len_operands := skip(vm.pc, vm.bitmask)
	x := vm.pc + 4
	if x > uint32(len(vm.code)) {
		x = uint32(len(vm.code))
	}
	operands := vm.code[vm.pc+1 : vm.pc+1+len_operands]
	fmt.Printf("pc: %d opcode: %d - operands: %v, len(operands) = %d\n", vm.pc, opcode, operands, len_operands)
	switch instr {
	case TRAP, FALLTHROUGH:
		fmt.Printf("TERMINATED\n")
		vm.terminated = true
	case JUMP:
		errCode := vm.branch([]byte{1, operands[0]})
		fmt.Println("Error: ", errCode)
		// vm.writeRegister(0, errCode)
	case JUMP_IND:
		vm.djump(operands)
		// vm.writeRegister(0, errCode)
	case LOAD_IMM_JUMP:
		errCode := vm.loadImmJump(operands)
		fmt.Println("Error: ", errCode)
		// vm.writeRegister(0, errCode)
	case LOAD_IMM_JUMP_IND:
		errCode := vm.loadImmJumpInd(operands)
		fmt.Println("Error: ", errCode)
		// vm.writeRegister(0, errCode)
	case BRANCH_EQ, BRANCH_NE, BRANCH_LT_U, BRANCH_LT_S, BRANCH_GE_U, BRANCH_GE_S:
		errCode := vm.branchReg(opcode, operands)
		fmt.Println("Error: ", errCode)
		// vm.writeRegister(0, errCode)
	case BRANCH_EQ_IMM, BRANCH_NE_IMM, BRANCH_LT_U_IMM, BRANCH_LT_S_IMM, BRANCH_LE_U_IMM, BRANCH_LE_S_IMM, BRANCH_GE_U_IMM, BRANCH_GE_S_IMM, BRANCH_GT_U_IMM, BRANCH_GT_S_IMM:
		errCode := vm.branchCond(opcode, operands)
		fmt.Println("Error: ", errCode)
		// vm.writeRegister(0, errCode)
	case ECALLI:
		errCode := vm.ecalli(operands)
		fmt.Println("Error: ", errCode)
		// vm.writeRegister(0, errCode)
		vm.pc += 1 + len_operands

		fmt.Printf("TERMINATED\n")
		vm.terminated = true
	case STORE_IMM_U8, STORE_IMM_U16, STORE_IMM_U32:
		errCode := vm.storeImm(opcode, operands)
		fmt.Println("Error: ", errCode)
		// vm.writeRegister(0, errCode)
		vm.pc += 1 + len_operands
	case LOAD_IMM:
		errCode := vm.loadImm(operands)
		fmt.Println("Error: ", errCode)
		// vm.writeRegister(0, errCode)
		vm.pc += 1 + len_operands
	case LOAD_U8, LOAD_U16, LOAD_U32:
		errCode := vm.load(opcode, operands)
		fmt.Println("Error: ", errCode)
		// vm.writeRegister(0, errCode)
		vm.pc += 1 + len_operands
	case STORE_U8, STORE_U16, STORE_U32:
		errCode := vm.store(opcode, operands)
		fmt.Println("Error: ", errCode)
		// vm.writeRegister(0, errCode)
		vm.pc += 1 + len_operands
	case STORE_IMM_IND_U8, STORE_IMM_IND_U16, STORE_IMM_IND_U32:
		errCode := vm.storeImmInd(opcode, operands)
		fmt.Println("Error: ", errCode)
		// vm.writeRegister(0, errCode)
		vm.pc += 1 + len_operands
	case STORE_IND_U8, STORE_IND_U16, STORE_IND_U32:
		errCode := vm.storeInd(opcode, operands)
		fmt.Println("Error: ", errCode)
		// vm.writeRegister(0, errCode)
		vm.pc += 1 + len_operands
	case LOAD_IND_U8, LOAD_IND_I8, LOAD_IND_U16, LOAD_IND_I16, LOAD_IND_U32:
		errCode := vm.loadInd(opcode, operands)
		fmt.Println("Error: ", errCode)
		// vm.writeRegister(0, errCode)
		vm.pc += 1 + len_operands
	case ADD_IMM, AND_IMM, XOR_IMM, OR_IMM, MUL_IMM, MUL_UPPER_S_S_IMM, MUL_UPPER_U_U_IMM, SET_LT_U_IMM, SET_LT_S_IMM:
		errCode := vm.aluImm(opcode, operands)
		fmt.Println("Error: ", errCode)
		// vm.writeRegister(0, errCode)
		vm.pc += 1 + len_operands
	case CMOV_IMM_IZ, CMOV_IMM_NZ:
		errCode := vm.cmovImm(opcode, operands)
		fmt.Println("Error: ", errCode)
		// vm.writeRegister(0, errCode)
		vm.pc += 1 + len_operands

	case SHLO_R_IMM, SHLO_L_IMM, SHAR_R_IMM, NEG_ADD_IMM, SET_GT_U_IMM, SET_GT_S_IMM, SHLO_R_IMM_ALT, SHLO_L_IMM_ALT, SHAR_L_IMM_ALT:
		errCode := vm.shiftImm(opcode, operands)
		fmt.Println("Error: ", errCode)
		// vm.writeRegister(0, errCode)
		vm.pc += 1 + len_operands
	case ADD_REG, SUB_REG, AND_REG, XOR_REG, OR_REG, MUL_REG, MUL_UPPER_S_S_REG, MUL_UPPER_U_U_REG, MUL_UPPER_S_U_REG, DIV_U, DIV_S, REM_U, REM_S, CMOV_IZ, CMOV_NZ, SHLO_L, SHLO_R, SHAR_R, SET_LT_U, SET_LT_S:
		errCode := vm.aluReg(opcode, operands)
		fmt.Println("Error: ", errCode)
		// vm.writeRegister(0, errCode)
		vm.pc += 1 + len_operands
	case MOVE_REG:
		errCode := vm.moveReg(operands)
		fmt.Println("Error: ", errCode)
		// vm.writeRegister(0, errCode)
		vm.pc += 1 + len_operands
	case SBRK:
		vm.sbrk(operands)
		break
	default:
		vm.terminated = true
		fmt.Printf("----\n")
		return nil
		/* Handle host call
		halt, err := vm.handleHostCall(opcode, operands)
		if err != nil {
			return err
		}
		if halt {
			// Host-call halt condition
			vm.hostCall = true
			vm.pc-- // Decrement PC to point to the host-call instruction
			return nil
		}*/
	}

	//vm.pc += 1 + uint32(skip(opcode))

	return nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func reverseBytes(b []byte) []byte {
	n := len(b)
	for i := 0; i < n/2; i++ {
		b[i], b[n-1-i] = b[n-1-i], b[i]
	}
	return b
}

// handleHostCall handles host-call instructions
func (vm *VM) handleHostCall(opcode byte, operands []byte) (bool, uint32) {
	if vm.hostenv == nil {
		return false, HUH
	}
	// TODO: vm.hostenv.InvokeHostCall(opcode, operands, vm)
	return false, OOB
}

// skip calculates the skip distance based on the opcode
// func skip(opcode byte) uint32 {
// 	switch opcode {
// 	case JUMP, LOAD_IMM_JUMP, LOAD_IMM_JUMP_IND:
// 		return uint32(1)
// 	case BRANCH_EQ, BRANCH_NE, BRANCH_GE_U, BRANCH_GE_S, BRANCH_LT_U, BRANCH_LT_S:
// 		return uint32(2)
// 	case BRANCH_EQ_IMM, BRANCH_NE_IMM, BRANCH_LT_U_IMM, BRANCH_LT_S_IMM, BRANCH_LE_U_IMM, BRANCH_LE_S_IMM, BRANCH_GE_U_IMM, BRANCH_GE_S_IMM, BRANCH_GT_U_IMM, BRANCH_GT_S_IMM:
// 		return uint32(3)
// 	default:
// 		return uint32(0)
// 	}
// }

// skip function calculates the distance to the next instruction
func skip(pc uint32, bitmask string) uint32 {
	// Convert the bitmask string to a slice of bytes
	bitmaskBytes := []byte(bitmask)

	// Iterate through the bitmask starting from the given pc position
	var i uint32
	for i = pc + 1; i < pc+25 && i < uint32(len(bitmaskBytes)); i++ {
		// Ensure we do not access out of bounds
		if bitmaskBytes[i] == '1' {
			return i - pc - 1
		}
	}
	// If no '1' is found within the next 24 positions, check the last index
	if i < pc+25 {
		return i - pc - 1
	}
	return uint32(24)
}

func (vm *VM) get_varpi(opcodes []byte, bitmask string) map[int]struct{} {
	result := make(map[int]struct{})
	for i, opcode := range opcodes {
		if bitmask[i] == '1' {
			if _, exists := T[int(opcode)]; exists {
				result[int(opcode)] = struct{}{}
			}
		}
	}
	return result
}

func (vm *VM) djump(operands []byte) uint32 {

	if len(operands) != 1 {
		return OOB
	}

	registerIndexA := minInt(12, int(operands[0])%16)
	immIndexX := minInt(4, (len(operands)-1)%256)

	var vx uint32
	if 1+immIndexX < len(operands) {
		vx = get_elided_uint32(operands[1 : 1+immIndexX])
	} else {
		vx = 0
	}

	valueA, errCode := vm.readRegister(registerIndexA)
	if errCode != OK {
		return errCode
	}

	var target uint32
	Za := uint32(4)
	terminationInstructions := vm.get_varpi(vm.code, vm.bitmask)

	if (valueA/Za - 1) < uint32(len(vm.J)) {
		target = uint32(vm.J[(valueA/Za - 1)])
	} else {
		target = uint32(99999)
	}

	_, exists := terminationInstructions[int(target)]

	if valueA == uint32((1<<32)-(1<<16)) {
		fmt.Printf("TERMINATED\n")
		vm.terminated = true
	} else if valueA == 0 || valueA > vx*Za || valueA%Za != 0 || exists {
		vm.terminated = true
		return OOB

	} else {
		vm.pc = vm.pc + target
	}

	return OK
}

func (vm *VM) readRAM(address uint32) (byte, uint32) {
	// Determine the page and offset within the page
	page := address / 4096
	offset := address % 4096

	// Retrieve the page from the RAM, return 0 if the page does not exist
	pageData, exists := vm.ram[page]
	if !exists {
		return 0, OK
	}

	// Return the byte at the specified address and the status code OK
	return pageData[offset], OK
}

func (vm *VM) writeRAM(address uint32, value byte) uint32 {
	// Determine the page and offset within the page
	page := address / 4096
	offset := address % 4096

	// Retrieve the page from the RAM or create a new page if it doesn't exist
	pageData, exists := vm.ram[page]
	if !exists {
		pageData = [4096]byte{}
	}

	// Write the byte to the specified address
	pageData[offset] = value

	// Store the updated page back in the RAM
	vm.ram[page] = pageData

	// Return the status code OK
	return OK
}

func (vm *VM) readRAMBytes(address uint32, numBytes int) ([]byte, uint32) {
	var result []byte
	remainingBytes := numBytes
	currentAddress := address

	for remainingBytes > 0 {
		// Determine the current page and offset within the page
		currentPage := currentAddress / 4096
		currentOffset := currentAddress % 4096

		// Determine how many bytes can be read from the current page
		bytesToRead := int(4096 - currentOffset)
		if bytesToRead > remainingBytes {
			bytesToRead = remainingBytes
		}

		// Retrieve the page data, return 0s if the page does not exist
		pageData, exists := vm.ram[currentPage]
		if !exists {
			result = append(result, make([]byte, bytesToRead)...)
		} else {
			result = append(result, pageData[currentOffset:currentOffset+uint32(bytesToRead)]...)
		}

		// Update the current address and the number of remaining bytes
		currentAddress += uint32(bytesToRead)
		remainingBytes -= bytesToRead
	}

	return result, OK
}

func (vm *VM) writeRAMBytes(address uint32, value []byte) uint32 {
	remainingBytes := len(value)
	currentAddress := address
	bytesWritten := 0

	for remainingBytes > 0 {
		// Determine the current page and offset within the page
		currentPage := currentAddress / 4096
		currentOffset := currentAddress % 4096

		// Determine how many bytes can be written to the current page
		bytesToWrite := int(4096 - currentOffset)
		if bytesToWrite > remainingBytes {
			bytesToWrite = remainingBytes
		}

		// Retrieve the page data or create a new page if it doesn't exist
		pageData, exists := vm.ram[currentPage]
		if !exists {
			pageData = [4096]byte{}
		}

		// Write the bytes to the current page
		copy(pageData[currentOffset:], value[bytesWritten:bytesWritten+bytesToWrite])

		// Store the updated page back in the RAM
		vm.ram[currentPage] = pageData

		// Update the current address and the number of remaining bytes
		currentAddress += uint32(bytesToWrite)
		bytesWritten += bytesToWrite
		remainingBytes -= bytesToWrite
	}

	return OK
}

func (vm *VM) readRegister(index int) (uint32, uint32) {
	if index < 0 || index >= len(vm.register) {
		return 0, OOB
	}
	// fmt.Printf(" REGISTERS %v (index=%d => %d)\n", vm.register, index, vm.register[index])
	return vm.register[index], OK
}

func (vm *VM) writeRegister(index int, value uint32) uint32 {
	if index < 0 || index >= len(vm.register) {
		return OOB
	}
	vm.register[index] = value
	fmt.Printf("Register[%d] = %d\n", index, value)
	return OK
}

// Implement the dynamic jump logic
func (vm *VM) dynamicJump(operands []byte) uint32 {
	if len(operands) != 1 {
		return OOB
	}
	a := int(operands[0])
	const ZA = 4

	if a == 0 || a > 0x7FFFFFFF {
		return OOB
	}

	targetIndex := uint32(a/ZA - 1)
	if targetIndex >= uint32(len(vm.code)) {
		return OOB
	}

	vm.pc = targetIndex
	return OK
}

// Implement ecall logic
func (vm *VM) ecalli(operands []byte) uint32 {
	// Implement ecalli logic here
	immediate := get_elided_uint32(operands[0:])
	vm.hostCall = true
	vm.host_func_id = immediate
	return HOST
}

// Implement storeImm logic
func (vm *VM) storeImm(opcode byte, operands []byte) uint32 {
	lx := min(4, int(operands[0]))
	// ly := min(4, max(0, len(operands) - lx -1))
	address := get_elided_uint32(operands[1:(1 + lx)])
	value := operands[(1 + lx):]
	value = reverseBytes(value)
	fmt.Printf("Set address %d, with value: %x\n", address, value)

	switch opcode {
	case STORE_IMM_U8:
		return vm.writeRAMBytes(address, value)
	case STORE_IMM_U16:
		return vm.writeRAMBytes(address, value)
	case STORE_IMM_U32:
		return vm.writeRAMBytes(address, value)
	}

	return OOB
}

// load_imm (opcode 4)
func (vm *VM) loadImm(operands []byte) uint32 {
	registerIndex := int(operands[0])
	immediate := get_elided_uint32(operands[1:])
	return vm.writeRegister(registerIndex, immediate)
}

// LOAD_U8, LOAD_U16, LOAD_U32
func (vm *VM) load(opcode byte, operands []byte) uint32 {
	registerIndex := int(operands[0])
	address := get_elided_uint32(operands[1:])

	switch opcode {
	case LOAD_U8:
		value, errCode := vm.readRAM(address)
		if errCode != OK {
			return errCode
		}
		fmt.Printf(" LOAD_u8: %d %v\n", address, value)
		return vm.writeRegister(registerIndex, uint32(value))
	case LOAD_U16:
		value, errCode := vm.readRAMBytes(address, 2)
		if errCode != OK {
			return errCode
		}
		fmt.Printf(" LOAD_u16: %d %v\n", address, value)
		return vm.writeRegister(registerIndex, uint32(binary.BigEndian.Uint16(value)))
	case LOAD_U32:
		value, errCode := vm.readRAMBytes(address, 4)
		if errCode != OK {
			return errCode
		}
		fmt.Printf(" LOAD_u32: %d %v\n", address, value)
		return vm.writeRegister(registerIndex, uint32(binary.BigEndian.Uint32(value)))
	}
	return OK
}

func get_elided_uint32(o []byte) uint32 {
	/* GP:
	Immediate arguments are encoded in little-endian format with the most-significant bit being the sign bit.
	They may be compactly encoded by eliding more significant octets. Elided octets are assumed to be zero if the MSB of the value is zero, and 255 otherwise.
	*/

	if len(o) < 4 {
		newNumbers := make([]byte, 4)
		copy(newNumbers, o)
		var fillValue byte
		if o[len(o)-1] > 127 {
			fillValue = byte(255)
		} else {
			fillValue = byte(0)
		}
		for i := len(o); i < 4; i++ {
			newNumbers[i] = fillValue
		}
		o = newNumbers
	}

	x := make([]byte, 4)
	if len(o) > 0 {
		x[3] = o[0]
	}
	if len(o) > 1 {
		x[2] = o[1]
	}
	if len(o) > 2 {
		x[1] = o[2]
	}
	if len(o) > 3 {
		x[0] = o[3]
	}

	fmt.Printf("get_elided_uint32 %v from %v\n", x, o)
	return binary.BigEndian.Uint32(x)
}

func get_elided_int32(o []byte) int32 {
	x := make([]byte, 4)
	x[0] = 0xff
	x[1] = 0xff
	x[2] = 0xff
	x[3] = 0xff
	// Copy the input bytes to the right end of x
	copy(x[4-len(o):], o)

	fmt.Printf("get_elided_int32 %v from %v\n", x, o)
	return int32(binary.BigEndian.Uint32(x))
}

// STORE_U8, STORE_U16, STORE_U32
func (vm *VM) store(opcode byte, operands []byte) uint32 {
	registerIndex := int(operands[0])
	value, errCode := vm.readRegister(registerIndex)
	if errCode != OK {
		return errCode
	}

	address := get_elided_uint32(operands[1:])
	switch opcode {
	case STORE_U8:
		b8 := []byte{byte(value & 0xFF)}
		fmt.Printf(" STORE_u8: %d %v\n", address, b8)
		return vm.writeRAMBytes(address, b8)
	case STORE_U16:
		b16 := make([]byte, 2)
		binary.BigEndian.PutUint16(b16, uint16(value&0xFFFF))
		fmt.Printf(" STORE_U16: %d %v\n", address, b16)
		return vm.writeRAMBytes(address, b16)
	case STORE_U32:
		b32 := make([]byte, 4)
		binary.BigEndian.PutUint32(b32, value)
		fmt.Printf(" STORE_U32: %d %v\n", address, b32)
		return vm.writeRAMBytes(address, b32)
	}
	return OOB
}

// storeImmInd implements STORE_IMM_{U8, U16, U32}
func (vm *VM) storeImmInd(opcode byte, operands []byte) uint32 {
	lX := int(operands[1])
	address := get_elided_uint32(operands[1:(1 + lX)])
	immediate := get_elided_uint32(operands[1+lX:])

	switch opcode {
	case STORE_IMM_IND_U8:
		b8 := []byte{byte(immediate & 0xFF)}
		return vm.writeRAMBytes(address, b8)
	case STORE_IMM_IND_U16:
		b16 := make([]byte, 2)
		binary.BigEndian.PutUint16(b16, uint16(immediate&0xFFFF))
		return vm.writeRAMBytes(address, b16)
	case STORE_IMM_IND_U32:
		b32 := make([]byte, 4)
		binary.BigEndian.PutUint32(b32, immediate)
		return vm.writeRAMBytes(address, b32)
	}
	return OOB
}

// Implement loadImmJump logic
func (vm *VM) loadImmJump(operands []byte) uint32 {
	condition := operands[0]
	target := uint32(operands[1])

	if condition != 0 {
		if target < 0 || target >= uint32(len(vm.code)) {
			return OOB
		}
		vm.pc = target
	}
	return OK
}

// Implement loadImmJumpInd logic
func (vm *VM) loadImmJumpInd(operands []byte) uint32 {
	index := int(operands[0])
	offset := uint32(operands[1])

	address, errCode := vm.readRegister(index)
	if errCode != OK {
		return errCode
	}

	target := uint32(address + offset)
	if target >= uint32(len(vm.code)) {
		return OOB
	}

	vm.pc = target
	return OK
}

// move_reg (opcode 82)
func (vm *VM) moveReg(operands []byte) uint32 {

	destIndex, srcIndex := splitRegister(operands[0])
	value, errCode := vm.readRegister(srcIndex)
	if errCode != OK {
		return errCode
	}
	fmt.Printf(" moveReg r[%d]=r[%d]=%d\n", destIndex, srcIndex, value)
	return vm.writeRegister(destIndex, value)
}

func (vm *VM) sbrk(operands []byte) error {
	/*	srcIndex, destIndex := splitRegister(operands[0])

		amount := int(operands[0])
		newRAM := make([]byte, len(vm.ram)+amount)
		copy(newRAM, vm.ram)
		vm.ram = newRAM */
	return nil
}

func (vm *VM) branch(operands []byte) uint32 {
	if len(operands) != 2 {
		return OOB
	}
	condition := operands[0]
	target := uint32(operands[1])

	// Implement the condition checking and branch logic here
	if condition != 0 {
		if target >= uint32(len(vm.code)) {
			return OOB
		}
		vm.pc = vm.pc + target
	}
	return OK
}

func (vm *VM) branchCond(opcode byte, operands []byte) uint32 {
	registerIndexA := minInt(12, int(operands[0])%16)
	immIndexX := minInt(4, int(operands[0])/16)
	immIndexY := minInt(4, (len(operands)-immIndexX-1)%256)
	vx := get_elided_uint32(operands[1 : 1+immIndexX])
	vy := get_elided_uint32(operands[1+immIndexX : 1+immIndexX+immIndexY])

	valueA, errCode := vm.readRegister(registerIndexA)
	if errCode != OK {
		return errCode
	}

	if uint32(immIndexY) >= uint32(len(vm.code)) {
		return OOB
	}
	switch opcode {
	case BRANCH_EQ_IMM:
		if byte(valueA) == byte(vx) {
			vm.branch([]byte{1, byte(vy)})
		} else {
			vm.pc += uint32(1 + len(operands))
			return OK
		}
	case BRANCH_NE_IMM:
		if byte(valueA) != byte(vx) {
			vm.branch([]byte{1, byte(vy)})
		} else {
			vm.pc += uint32(1 + len(operands))
			return OK
		}
	case BRANCH_LT_U_IMM:
		if uint(valueA) < uint(vx) {
			vm.branch([]byte{1, byte(vy)})
		} else {
			vm.pc += uint32(1 + len(operands))
			return OK
		}
	case BRANCH_LT_S_IMM:
		if int32(valueA) < int32(vx) {
			vm.branch([]byte{1, byte(vy)})
		} else {
			vm.pc += uint32(1 + len(operands))
			return OK
		}
	case BRANCH_LE_U_IMM:
		if uint(valueA) <= uint(vx) {
			vm.branch([]byte{1, byte(vy)})
		} else {
			vm.pc += uint32(1 + len(operands))
			return OK
		}
	case BRANCH_LE_S_IMM:
		if int32(valueA) <= int32(vx) {
			vm.branch([]byte{1, byte(vy)})
		} else {
			vm.pc += uint32(1 + len(operands))
			return OK
		}
	case BRANCH_GE_U_IMM:
		if uint(valueA) >= uint(vx) {
			vm.branch([]byte{1, byte(vy)})
		} else {
			vm.pc += uint32(1 + len(operands))
			return OK
		}
	case BRANCH_GE_S_IMM:
		if int32(valueA) >= int32(vx) {
			vm.branch([]byte{1, byte(vy)})
		} else {
			vm.pc += uint32(1 + len(operands))
			return OK
		}
	case BRANCH_GT_U_IMM:
		if uint(valueA) > uint(vx) {
			vm.branch([]byte{1, byte(vy)})
		} else {
			vm.pc += uint32(1 + len(operands))
			return OK
		}
	case BRANCH_GT_S_IMM:
		if int32(valueA) > int32(vx) {
			vm.branch([]byte{1, byte(vy)})
		} else {
			vm.pc += uint32(1 + len(operands))
			return OK
		}
	}
	return OK

}

func (vm *VM) storeInd(opcode byte, operands []byte) uint32 {
	registerIndex := int(operands[0])
	offset := uint32(operands[1])
	address, errCode := vm.readRegister(registerIndex)
	if errCode != OK {
		return OOB
	}

	switch opcode {
	case STORE_IND_U8:
		return vm.writeRAMBytes(address+offset, operands[2:2])
	case STORE_IND_U16:
		return vm.writeRAMBytes(address+offset, operands[2:3])
	case STORE_IND_U32:
		return vm.writeRAMBytes(address+offset, operands[2:5])
	default:
		return OOB
	}

}

// Implement loadInd logic
func (vm *VM) loadInd(opcode byte, operands []byte) uint32 {
	destRegisterIndex, registerIndexB := splitRegister(operands[0])
	wB, errCode := vm.readRegister(registerIndexB)
	if errCode != OK {
		return errCode
	}
	immediate := get_elided_uint32(operands[1:])

	address := wB + immediate
	switch opcode {
	case LOAD_IND_U8:
		value, errCode := vm.readRAM(address)
		if errCode != OK {
			return errCode
		}
		return vm.writeRegister(destRegisterIndex, uint32(value))
	case LOAD_IND_I8:
		value, errCode := vm.readRAM(address)
		if errCode != OK {
			return errCode
		}
		v := int(value)
		if v > 127 {
			v -= 256
		}
		return vm.writeRegister(destRegisterIndex, uint32(v))
	case LOAD_IND_U16:
		value, errCode := vm.readRAMBytes(address, 2)
		if errCode != OK {
			return errCode

		}
		return vm.writeRegister(destRegisterIndex, uint32(binary.BigEndian.Uint16(value)))
	case LOAD_IND_I16:
		value, errCode := vm.readRAMBytes(address, 2)
		if errCode != OK {
			return errCode
		}
		return vm.writeRegister(destRegisterIndex, uint32(binary.BigEndian.Uint16(value)))
	case LOAD_IND_U32:
		value, errCode := vm.readRAMBytes(address, 4)
		if errCode != OK {
			return errCode
		}
		return vm.writeRegister(destRegisterIndex, uint32(binary.BigEndian.Uint32(value)))
	}
	return OOB
}

func splitRegister(operand byte) (int, int) {
	registerIndexA := int(operand & 0xF)
	registerIndexB := int((operand >> 4) & 0xF)
	return registerIndexA, registerIndexB
}

// Implement ALU operations with immediate values
func (vm *VM) aluImm(opcode byte, operands []byte) uint32 {
	registerIndexD, registerIndexA := splitRegister(operands[0])
	immediate := get_elided_uint32(operands[1:])
	fmt.Printf("a=%d d=%d immediate=%d\n", registerIndexA, registerIndexD, immediate)

	valueA, errCode := vm.readRegister(registerIndexA)
	if errCode != OK {
		return errCode
	}

	var result uint32
	switch opcode {
	case ADD_IMM:
		result = valueA + immediate
	// case SUB_IMM:
	// 	result = valueA - immediate
	case AND_IMM:
		result = valueA & immediate
	case XOR_IMM:
		result = valueA ^ immediate
	case OR_IMM:
		result = valueA | immediate
	case MUL_IMM:
		result = valueA * immediate
	case MUL_UPPER_S_S_IMM:
		result = uint32(int(valueA) * int(immediate) >> 32)
	case MUL_UPPER_U_U_IMM:
		result = uint32((uint(valueA) * uint(immediate)) >> 32)
	case SET_LT_U_IMM:
		if uint32(valueA) < uint32(immediate) {
			result = 1
		} else {
			result = 0
		}
	case SET_LT_S_IMM:
		immediate0 := get_elided_uint32(operands[1:]) // use get_elided_int32() will return a negative value

		if int32(valueA) < int32(immediate0) {
			result = 1
		} else {
			result = 0
		}
	default:
		return OOB
	}
	return vm.writeRegister(registerIndexD, result)
}

// Implement cmov_nz_imm, cmov_nz_imm
func (vm *VM) cmovImm(opcode byte, operands []byte) uint32 {
	registerIndexA, registerIndexB := splitRegister(operands[0])
	immediate := get_elided_uint32(operands[1:])
	fmt.Printf("a=%d b=%d immediate=%d\n", registerIndexA, registerIndexB, immediate)

	valueA, errCode := vm.readRegister(registerIndexA)
	if errCode != OK {
		return errCode
	}
	valueB, errCode := vm.readRegister(registerIndexB)
	if errCode != OK {
		return errCode
	}

	var result uint32
	switch opcode {
	case CMOV_IMM_IZ:
		if valueB == 0 {
			result = immediate
		} else {
			result = valueA
		}

	case CMOV_IMM_NZ:
		if valueB != 0 {
			result = immediate
		} else {
			result = valueA
		}

	default:
		return OOB
	}
	return vm.writeRegister(registerIndexA, result)
}

// Implement shift operations with immediate values
func (vm *VM) shiftImm(opcode byte, operands []byte) uint32 {
	registerIndexB, registerIndexA := splitRegister(operands[0])
	immediate := get_elided_uint32(operands[1:])

	valueA, errCode := vm.readRegister(registerIndexA)
	if errCode != OK {
		return errCode
	}
	var result uint32
	switch opcode {

	case SHLO_R_IMM:
		result = valueA >> immediate
	case SHLO_L_IMM:
		result = valueA << immediate
	case SHAR_R_IMM:
		result = valueA >> immediate
		mask := int32((1<<immediate)-1) << (32 - immediate)
		result |= uint32(mask)

	case NEG_ADD_IMM:
		result = uint32(-int(valueA) + int(immediate))
	case SET_GT_U_IMM:
		if uint(valueA) > uint(immediate) {
			result = 1
		} else {
			result = 0
		}
	case SHAR_L_IMM_ALT:

		result = immediate >> valueA
		mask := int32((1<<valueA)-1) << (32 - valueA)
		result |= uint32(mask)
	case SET_GT_S_IMM:
		immediate2 := get_elided_uint32(operands[1:]) // use get_elided_int32() will return a negative value
		if int32(valueA) > int32(immediate2) {
			result = 1
		} else {
			result = 0
		}
		fmt.Printf(" SET_GT_S_IMM %d > %d == %d\n", int32(valueA), int32(immediate), result)
	case SHLO_R_IMM_ALT:
		result = immediate >> valueA
		fmt.Printf(" SHLO_R_IMM_ALT %d >> %d = %d\n", immediate, valueA, result)
	case SHLO_L_IMM_ALT:
		result = immediate << valueA
		fmt.Printf(" SHLO_L_IMM_ALT %d << %d = %d\n", immediate, valueA, result)
	default:
		return OOB
	}
	fmt.Printf(" shiftImm r[%d]=%d immediate=%d r[%d]=%d\n", registerIndexA, valueA, immediate, registerIndexB, result)
	return vm.writeRegister(registerIndexB, result)
}

// Implement branch logic for two registers and one offset
func (vm *VM) branchReg(opcode byte, operands []byte) uint32 {
	if len(operands) != 2 {
		return OOB
	}

	registerIndexA := minInt(12, int(operands[0])%16)
	registerIndexB := minInt(12, int(operands[0])/16)

	offset := uint32(operands[1])

	fmt.Print("registerIndexA: ", registerIndexA)
	fmt.Print("  registerIndexB: ", registerIndexB)
	fmt.Println("  jump step: ", offset)

	valueA, errCode := vm.readRegister(registerIndexA)
	if errCode != OK {
		return errCode
	}
	valueB, errCode := vm.readRegister(registerIndexB)
	if errCode != OK {
		return errCode
	}

	switch opcode {
	case BRANCH_EQ:
		if valueA == valueB {
			vm.branch([]byte{1, byte(offset)})
		} else {
			vm.pc += uint32(1 + len(operands))
			return OK
		}
	case BRANCH_NE:
		if valueA != valueB {
			vm.branch([]byte{1, byte(offset)})
		} else {
			vm.pc += uint32(1 + len(operands))
			return OK
		}
	case BRANCH_LT_U:
		if uint(valueA) < uint(valueB) {
			vm.branch([]byte{1, byte(offset)})
		} else {
			vm.pc += uint32(1 + len(operands))
			return OK
		}
	case BRANCH_LT_S:
		if int32(valueA) < int32(valueB) {
			vm.branch([]byte{1, byte(offset)})
		} else {
			vm.pc += uint32(1 + len(operands))
			return OK
		}
	case BRANCH_GE_U:
		if uint(valueA) >= uint(valueB) {
			vm.branch([]byte{1, byte(offset)})
		} else {
			vm.pc += uint32(1 + len(operands))
			return OK
		}
	case BRANCH_GE_S:
		if int32(valueA) >= int32(valueB) {
			vm.branch([]byte{1, byte(offset)})
		} else {
			vm.pc += uint32(1 + len(operands))
			return OK
		}
	default:
		return OOB
	}

	return OK
}

// Implement ALU operations with register values
func (vm *VM) aluReg(opcode byte, operands []byte) uint32 {

	registerIndexA := minInt(12, int(operands[0])%16)
	registerIndexB := minInt(12, int(operands[0])/16)
	registerIndexD := minInt(12, int(operands[1]))

	valueA, errCode := vm.readRegister(registerIndexA)
	if errCode != OK {
		return errCode
	}
	valueB, errCode := vm.readRegister(registerIndexB)
	if errCode != OK {
		return errCode
	}

	var result uint32
	switch opcode {
	case ADD_REG:
		result = valueA + valueB
	case SUB_REG:
		result = valueA - valueB
	case AND_REG:
		result = valueA & valueB
	case XOR_REG:
		result = valueA ^ valueB
	case OR_REG:
		result = valueA | valueB
	case MUL_REG:
		result = valueA * valueB
	case MUL_UPPER_S_S_REG:
		result = uint32(int(valueA) * int(valueB) >> 32)
	case MUL_UPPER_U_U_REG:
		result = uint32((uint(valueA) * uint(valueB)) >> 32)
	case MUL_UPPER_S_U_REG:
		result = uint32((int(valueA) * int(valueB)) >> 32)
	case DIV_U:
		if valueB == 0 {
			result = 0xFFFFFFFF
		} else {
			result = valueA / valueB
		}
	case DIV_S:
		if valueB == 0 {
			result = 0xFFFFFFFF
		} else if int32(valueA) == -(1<<31) && int32(valueB) == -(1<<0) {
			result = valueA
		} else {
			result = uint32(int32(valueA) / int32(valueB))
		}
	case REM_U:
		if valueB == 0 {
			result = valueA
		} else {
			result = valueA % valueB
		}
	case REM_S:
		fmt.Printf(" REM_S %d %d \n", int32(valueA), int32(valueB))
		if valueB == 0 {
			result = valueA
		} else if valueA == 0x80 && valueB == 0xFF {
			return OOB
		} else {
			result = uint32(int32(valueA) % int32(valueB))
		}
	case CMOV_IZ:
		if valueB == 0 {
			result = valueA
		} else {
			result = 0
		}
	case CMOV_NZ:
		if valueB == 0 {
			result = 0
		} else {
			result = valueA
		}
	case SET_LT_U:
		if valueA < valueB {
			result = 1
		} else {
			result = 0
		}
	case SET_LT_S:
		if int32(valueA) < int32(valueB) {
			result = 1
		} else {
			result = 0
		}

	case SHLO_L:
		result = valueA << (valueB % 32)
	case SHLO_R:
		result = valueA >> (valueB % 32)
	case SHAR_R:
		if int32(valueA)/(1<<(valueB%32)) < 0 && int32(valueA)%(1<<(valueB%32)) != 0 {
			result = uint32((int32(valueA) / (1 << (valueB % 32))) - 1)
		} else {
			result = uint32(int32(valueA) / (1 << (valueB % 32)))
		}

	default:
		return OOB // unknown ALU register
	}

	fmt.Printf(" aluReg - rA[%d]=%d  regB[%d]=%d regD[%d]=%d\n", registerIndexA, valueA, registerIndexB, valueB, registerIndexD, result)
	return vm.writeRegister(registerIndexD, result)
}

// VM Management: CreateVM, GetVM, ExpungeVM
func (vm *VM) CreateVM(serviceAcct uint32, code []byte, i uint32) uint32 {
	maxN := uint32(0)
	for n := range vm.VMs {
		if n > maxN {
			maxN = n
		}
	}
	vm.VMs[maxN+1] = NewVMFromCode(serviceAcct, code, i, vm.hostenv)
	return maxN + 1
}

func (vm *VM) GetVM(n uint32) (*VM, bool) {
	vm, ok := vm.VMs[n]
	if !ok {
		return nil, false
	}
	return vm, true
}

func (vm *VM) ExpungeVM(n uint32) bool {
	_, ok := vm.VMs[n]
	if !ok {
		return false
	}
	vm.VMs[n] = nil
	return true
}
