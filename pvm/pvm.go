package pvm

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	regSize = 13
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
	code       []byte
	pc         int // Program counter
	terminated bool
	hostCall   bool
	ram        []byte
	register   []uint32
	hostEnv    JAMEnvironment
}

type Program struct {
	JSize int
	Z     int // 1 byte
	CSize int
	J     []byte
	Code  []byte
	K     []byte
}

// Define opcodes and instructions
const (
	TRAP              = 0
	FALLTHROUGH       = 17
	ADD_REG           = 8
	ECALL             = 78
	STORE_IMM_U8      = 62
	STORE_IMM_U16     = 79
	STORE_IMM_U32     = 38
	JUMP              = 5
	JUMP_IND          = 19
	LOAD_IMM          = 4
	LOAD_U8           = 60
	LOAD_U16          = 76
	LOAD_U32          = 10
	STORE_U8          = 71
	STORE_U16         = 69
	STORE_U32         = 22
	STORE_IMM_IND_U8  = 26
	STORE_IMM_IND_U16 = 54
	STORE_IMM_IND_U32 = 13
	LOAD_IMM_JUMP     = 6
	BRANCH_EQ_IMM     = 7
	BRANCH_NE_IMM     = 15
	BRANCH_LT_U_IMM   = 44
	BRANCH_LE_U_IMM   = 59
	BRANCH_GE_U_IMM   = 52
	BRANCH_GT_U_IMM   = 50
	BRANCH_LT_S_IMM   = 32
	BRANCH_LE_S_IMM   = 46
	BRANCH_GE_S_IMM   = 45
	BRANCH_GT_S_IMM   = 53
	MOVE_REG          = 82
	SBRK              = 87
	STORE_IND_U8      = 16
	STORE_IND_U16     = 29
	STORE_IND_U32     = 3
	LOAD_IND_U8       = 11
	LOAD_IND_I8       = 21
	LOAD_IND_U16      = 37
	LOAD_IND_I16      = 33
	LOAD_IND_U32      = 1
	ADD_IMM           = 2
	SUB_IMM           = 102
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
	BRANCH_EQ         = 24
	BRANCH_NE         = 30
	BRANCH_LT_U       = 47
	BRANCH_LT_S       = 48
	BRANCH_GE_U       = 41
	BRANCH_GE_S       = 43
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
	LOAD_IMM_JUMP_IND = 42

	SET_LT_U = 36
	SET_LT_S = 58
	SHLO_L   = 55
	SHLO_R   = 51
	SHAR_R   = 77

	CMOV_IZ     = 83
	CMOV_NZ     = 84
	CMOV_IMM_IZ = 85 // not in the paper

)

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
	return &Program{
		JSize: int(p[0]),
		Z:     int(p[1]),
		CSize: int(p[2]),
		J:     p[3 : 3+p[0]],
		Code:  p[3+p[0] : 3+p[0]+p[2]],
		K:     p[3+p[0]+p[2]:],
	}

}

// NewVM initializes a new VM with a given program
func NewVM(code []byte, initialRegs []uint32, initialPC int, pagemap []PageMap, pages []Page) *VM {
	p := parseProgram(code)
	fmt.Printf("Code: %v K: %v\n", p.Code, p.K)
	vm := &VM{
		code:     p.Code,
		register: make([]uint32, regSize),
		pc:       initialPC,
		ram:      make([]byte, 4096*64),
	}
	for _, pg := range pages {
		for i, b := range pg.Contents {
			vm.ram[pg.Address+uint32(i)] = b
		}
	}
	copy(vm.register, initialRegs)
	return vm
}

// Execute runs the program until it terminates
func (vm *VM) Execute() error {
	for !vm.terminated {
		if err := vm.step(); err != nil {
			return err
		}
	}
	return nil
}

// step performs a single step in the PVM
func (vm *VM) step() error {
	if vm.pc < 0 || vm.pc >= len(vm.code) {
		return errors.New("program counter out of bounds")
	}

	instr := vm.code[vm.pc]
	opcode := instr
	x := vm.pc + 4
	if x > len(vm.code) {
		x = len(vm.code)
	}
	operands := vm.code[vm.pc+1:]
	fmt.Printf("%d opcode %d - operands: %v\n", vm.pc, opcode, operands)
	switch instr {
	case TRAP, FALLTHROUGH:
		fmt.Printf("TERMINATED\n")
		vm.terminated = true
	case JUMP:
		if err := vm.branch(operands, 0); err != nil {
			return err
		}
	case LOAD_IMM_JUMP:
		if err := vm.loadImmJump(operands); err != nil {
			return err
		}
	case LOAD_IMM_JUMP_IND:
		if err := vm.loadImmJumpInd(operands); err != nil {
			return err
		}
	case BRANCH_EQ, BRANCH_NE, BRANCH_LT_S, BRANCH_GE_U, BRANCH_GE_S:
		if err := vm.branchReg(opcode, operands); err != nil {
			return err
		}
	case BRANCH_EQ_IMM, BRANCH_NE_IMM, BRANCH_LT_U_IMM, BRANCH_LT_S_IMM, BRANCH_LE_U_IMM, BRANCH_LE_S_IMM, BRANCH_GE_U_IMM, BRANCH_GE_S_IMM, BRANCH_GT_U_IMM, BRANCH_GT_S_IMM:
		if err := vm.branchCond(opcode, operands); err != nil {
			return err
		}
	case ECALL:
		if err := vm.ecall(operands); err != nil {
			return err
		}
	case STORE_IMM_U8, STORE_IMM_U16, STORE_IMM_U32:
		if err := vm.storeImm(opcode, operands); err != nil {
			return err
		}
	case LOAD_IMM:
		if err := vm.loadImm(operands); err != nil {
			return err
		}
	case LOAD_U8, LOAD_U16, LOAD_U32:
		if err := vm.load(opcode, operands); err != nil {
			return err
		}
	case STORE_U8, STORE_U16, STORE_U32:
		if err := vm.store(opcode, operands); err != nil {
			return err
		}
	case STORE_IMM_IND_U8, STORE_IMM_IND_U16, STORE_IMM_IND_U32:
		if err := vm.storeImmInd(opcode, operands); err != nil {
			return err
		}
	case STORE_IND_U8, STORE_IND_U16, STORE_IND_U32:
		if err := vm.storeInd(opcode, operands); err != nil {
			return err
		}
	case LOAD_IND_U8, LOAD_IND_I8, LOAD_IND_U16, LOAD_IND_I16, LOAD_IND_U32:
		if err := vm.loadInd(opcode, operands); err != nil {
			return err
		}
	case ADD_IMM, SUB_IMM, AND_IMM, XOR_IMM, OR_IMM, MUL_IMM, MUL_UPPER_S_S_IMM, MUL_UPPER_U_U_IMM, SET_LT_U_IMM, SET_LT_S_IMM:
		fmt.Printf("XXXXXX\n")
		if err := vm.aluImm(opcode, operands); err != nil {
			return err
		}
	case SHLO_R_IMM, SHLO_L_IMM, SHAR_R_IMM, NEG_ADD_IMM, SET_GT_U_IMM, SET_GT_S_IMM, SHLO_R_IMM_ALT, SHLO_L_IMM_ALT, SHAR_L_IMM_ALT:
		if err := vm.shiftImm(opcode, operands); err != nil {
			return err
		}
	case ADD_REG, SUB_REG, AND_REG, XOR_REG, OR_REG, MUL_REG, MUL_UPPER_S_S_REG, MUL_UPPER_U_U_REG, MUL_UPPER_S_U_REG, DIV_U, DIV_S, REM_U, REM_S, CMOV_IZ, CMOV_NZ, SHLO_L, SHLO_R, SHAR_R, SET_LT_U, SET_LT_S:
		if err := vm.aluReg(opcode, operands); err != nil {
			return err
		}
	case MOVE_REG:
		if err := vm.moveReg(operands); err != nil {
			return err
		}
	case SBRK:
		if err := vm.sbrk(operands); err != nil {
			return err
		}
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

	vm.pc += 1 + skip(opcode)
	return nil
}

// handleHostCall handles host-call instructions
func (vm *VM) handleHostCall(opcode byte, operands []byte) (bool, error) {
	if vm.hostEnv == nil {
		return false, errors.New("no host environment configured")
	}
	return vm.hostEnv.InvokeHostCall(opcode, operands, vm)
}

// skip calculates the skip distance based on the opcode
func skip(opcode byte) int {
	switch opcode {
	case JUMP, LOAD_IMM_JUMP, LOAD_IMM_JUMP_IND:
		return 1
	case BRANCH_EQ, BRANCH_NE, BRANCH_GE_U, BRANCH_GE_S, BRANCH_LT_U, BRANCH_LT_S:
		return 2
	case BRANCH_EQ_IMM, BRANCH_NE_IMM, BRANCH_LT_U_IMM, BRANCH_LT_S_IMM, BRANCH_LE_U_IMM, BRANCH_LE_S_IMM, BRANCH_GE_U_IMM, BRANCH_GE_S_IMM, BRANCH_GT_U_IMM, BRANCH_GT_S_IMM:
		return 3
	default:
		return 0
	}
}

// Implement additional functions for RAM and register access as per the specification
func (vm *VM) readRAM(address uint32) (byte, error) {
	if address < 0 || address >= uint32(len(vm.ram)) {
		return 0, errors.New("RAM access out of bounds")
	}
	return vm.ram[address], nil
}

func (vm *VM) readRAMBytes(address uint32, numBytes int) ([]byte, error) {
	return vm.ram[address:(address + uint32(numBytes))], nil
}

func (vm *VM) writeRAMBytes(address uint32, value []byte) error {
	if address < 0 || address >= uint32(len(vm.ram)) {
		return errors.New("RAM access out of bounds")
	}
	for i, b := range value {
		vm.ram[address+uint32(i)] = b
	}
	return nil
}
func (vm *VM) writeRAM(address int, value byte) error {
	if address < 0 || address >= len(vm.ram) {
		return errors.New("RAM access out of bounds")
	}
	vm.ram[address] = value
	return nil
}

func (vm *VM) readRegister(index int) (uint32, error) {
	if index < 0 || index >= len(vm.register) {

		return 0, errors.New("Register access out of bounds")
	}
	fmt.Printf(" REGISTERS %v (index=%d => %d)\n", vm.register, index, vm.register[index])
	return vm.register[index], nil
}

func (vm *VM) writeRegister(index int, value uint32) error {
	if index < 0 || index >= len(vm.register) {
		return errors.New("Register access out of bounds")
	}
	vm.register[index] = value
	return nil
}

// Implement the dynamic jump logic
func (vm *VM) dynamicJump(operands []byte) error {
	if len(operands) != 1 {
		return errors.New("invalid operand length for dynamic jump")
	}
	a := int(operands[0])
	const ZA = 4

	if a == 0 || a > 0x7FFFFFFF {
		return errors.New("dynamic jump address out of bounds")
	}

	targetIndex := a/ZA - 1
	if targetIndex < 0 || targetIndex >= len(vm.code) {
		return errors.New("dynamic jump target index out of bounds")
	}

	vm.pc = targetIndex
	return nil
}

// Implement ecall logic
func (vm *VM) ecall(operands []byte) error {
	// Implement ecall logic here
	return nil
}

// Implement storeImm logic
func (vm *VM) storeImm(opcode byte, operands []byte) error {
	address := int(operands[0])
	value := operands[1]

	switch opcode {
	case STORE_IMM_U8:
		if err := vm.writeRAM(address, value); err != nil {
			return err
		}
	case STORE_IMM_U16:
		if len(operands) < 3 {
			return errors.New("invalid operand length for store immediate u16")
		}
		value2 := operands[2]
		if err := vm.writeRAM(address, value); err != nil {
			return err
		}
		if err := vm.writeRAM(address+1, value2); err != nil {
			return err
		}
	case STORE_IMM_U32:
		if len(operands) < 5 {
			return errors.New("invalid operand length for store immediate u32")
		}
		value2 := operands[2]
		value3 := operands[3]
		value4 := operands[4]
		if err := vm.writeRAM(address, value); err != nil {
			return err
		}
		if err := vm.writeRAM(address+1, value2); err != nil {
			return err
		}
		if err := vm.writeRAM(address+2, value3); err != nil {
			return err
		}
		if err := vm.writeRAM(address+3, value4); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown store immediate opcode: %d", opcode)
	}

	return nil
}

// load_imm (opcode 4)
func (vm *VM) loadImm(operands []byte) error {
	registerIndex := int(operands[0])
	immediate := get_elided_uint32(operands[1:])
	return vm.writeRegister(registerIndex, immediate)
}

// LOAD_U8, LOAD_U16, LOAD_U32
func (vm *VM) load(opcode byte, operands []byte) error {
	registerIndex := int(operands[0])
	address := get_elided_uint32(operands[1:])

	switch opcode {
	case LOAD_U8:
		value, err := vm.readRAM(address)
		if err != nil {
			return err
		}
		fmt.Printf(" LOAD_u8: %d %v\n", address, value)
		return vm.writeRegister(registerIndex, uint32(value))
	case LOAD_U16:
		value, err := vm.readRAMBytes(address, 2)
		if err != nil {
			return err
		}
		fmt.Printf(" LOAD_u16: %d %v\n", address, value)
		return vm.writeRegister(registerIndex, uint32(binary.BigEndian.Uint16(value)))
	case LOAD_U32:
		value, err := vm.readRAMBytes(address, 4)
		if err != nil {
			return err
		}
		fmt.Printf(" LOAD_u32: %d %v\n", address, value)
		return vm.writeRegister(registerIndex, uint32(binary.BigEndian.Uint32(value)))
	default:
		return fmt.Errorf("unknown load opcode: %d", opcode)
	}
}

func get_elided_uint32(o []byte) uint32 {
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
func (vm *VM) store(opcode byte, operands []byte) error {
	registerIndex := int(operands[0])

	value, err := vm.readRegister(registerIndex)
	if err != nil {
		return err
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
		if err := vm.writeRAMBytes(address, b16); err != nil {
			return err
		}
	case STORE_U32:
		b32 := make([]byte, 4)
		binary.BigEndian.PutUint32(b32, value)
		fmt.Printf(" STORE_U32: %d %v\n", address, b32)
		if err := vm.writeRAMBytes(address, b32); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown store opcode: %d", opcode)
	}
	return nil
}

// storeImmInd implements STORE_IMM_{U8, U16, U32}
func (vm *VM) storeImmInd(opcode byte, operands []byte) error {
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
		b32 := make([]byte, 2)
		binary.BigEndian.PutUint32(b32, immediate)
		return vm.writeRAMBytes(address, b32)
	default:
		return fmt.Errorf("unknown store immediate indirect opcode: %d", opcode)
	}
}

// Implement loadImmJump logic
func (vm *VM) loadImmJump(operands []byte) error {
	condition := operands[0]
	target := int(operands[1])

	if condition != 0 {
		if target < 0 || target >= len(vm.code) {
			return errors.New("load immediate jump target out of bounds")
		}
		vm.pc = target
	}
	return nil
}

// Implement loadImmJumpInd logic
func (vm *VM) loadImmJumpInd(operands []byte) error {
	index := int(operands[0])
	offset := int(operands[1])

	address, err := vm.readRegister(index)
	if err != nil {
		return err
	}

	target := int(address) + offset
	if target < 0 || target >= len(vm.code) {
		return errors.New("load immediate jump target out of bounds")
	}

	vm.pc = target
	return nil
}

// move_reg (opcode 82)
func (vm *VM) moveReg(operands []byte) error {

	destIndex, srcIndex := splitRegister(operands[0])
	value, err := vm.readRegister(srcIndex)
	if err != nil {
		return err
	}
	fmt.Printf(" moveReg r[%d]=r[%d]=%d\n", destIndex, srcIndex, value)
	return vm.writeRegister(destIndex, value)
}

// TODO: Implement sbrk logic
func (vm *VM) sbrk(operands []byte) error {
	/*	srcIndex, destIndex := splitRegister(operands[0])

		amount := int(operands[0])
		newRAM := make([]byte, len(vm.ram)+amount)
		copy(newRAM, vm.ram)
		vm.ram = newRAM */
	return nil
}

// TODO: Implement branch logic
func (vm *VM) branch(operands []byte, currentPc int) error {
	if len(operands) != 2 {
		return errors.New("invalid operand length for branch")
	}
	condition := operands[0]
	target := int(operands[1])

	// Implement the condition checking and branch logic here
	if condition != 0 {
		if target < 0 || target >= len(vm.code) {
			return errors.New("branch target out of bounds")
		}
		vm.pc = target
	} else {
		vm.pc = currentPc
	}

	return nil
}

func (vm *VM) branchCond(opcode byte, operands []byte) error {
	registerIndex := int(operands[0])
	immediate := operands[1]
	target := int(operands[2])

	value, err := vm.readRegister(registerIndex)
	if err != nil {
		return err
	}
	switch opcode {
	case BRANCH_EQ_IMM:
		if byte(value) == immediate {
			if target < 0 || target >= len(vm.code) {
				return errors.New("branch target out of bounds")
			}
			vm.pc = target
		}
	case BRANCH_NE_IMM:
		if byte(value) != immediate {
			if target < 0 || target >= len(vm.code) {
				return errors.New("branch target out of bounds")
			}
			vm.pc = target
		}
	case BRANCH_LT_U_IMM:
		if uint(value) < uint(immediate) {
			if target < 0 || target >= len(vm.code) {
				return errors.New("branch target out of bounds")
			}
			vm.pc = target
		}
	case BRANCH_LT_S_IMM:
		if int(value) < int(immediate) {
			if target < 0 || target >= len(vm.code) {
				return errors.New("branch target out of bounds")
			}
			vm.pc = target
		}
	case BRANCH_LE_U_IMM:
		if uint(value) <= uint(immediate) {
			if target < 0 || target >= len(vm.code) {
				return errors.New("branch target out of bounds")
			}
			vm.pc = target
		}
	case BRANCH_LE_S_IMM:
		if int(value) <= int(immediate) {
			if target < 0 || target >= len(vm.code) {
				return errors.New("branch target out of bounds")
			}
			vm.pc = target
		}
	case BRANCH_GE_U_IMM:
		if uint(value) >= uint(immediate) {
			if target < 0 || target >= len(vm.code) {
				return errors.New("branch target out of bounds")
			}
			vm.pc = target
		}
	case BRANCH_GE_S_IMM:
		if int(value) >= int(immediate) {
			if target < 0 || target >= len(vm.code) {
				return errors.New("branch target out of bounds")
			}
			vm.pc = target
		}
	case BRANCH_GT_U_IMM:
		if uint(value) > uint(immediate) {
			if target < 0 || target >= len(vm.code) {
				return errors.New("branch target out of bounds")
			}
			vm.pc = target
		}
	case BRANCH_GT_S_IMM:
		if int(value) > int(immediate) {
			if target < 0 || target >= len(vm.code) {
				return errors.New("branch target out of bounds")
			}
			vm.pc = target
		}
	}
	return nil

}

func (vm *VM) storeInd(opcode byte, operands []byte) error {
	registerIndex := int(operands[0])
	offset := int(operands[1])
	value := operands[2]

	address, err := vm.readRegister(registerIndex)
	if err != nil {
		return err
	}

	switch opcode {
	case STORE_IND_U8:
		return vm.writeRAM(int(address)+offset, value)
	case STORE_IND_U16:
		if len(operands) < 4 {
			return errors.New("invalid operand length for store indirect u16")
		}
		value2 := operands[3]
		if err := vm.writeRAM(int(address)+offset, value); err != nil {
			return err
		}
		return vm.writeRAM(int(address)+offset+1, value2)
	case STORE_IND_U32:
		if len(operands) < 5 {
			return errors.New("invalid operand length for store indirect u32")
		}
		value2 := operands[3]
		value3 := operands[4]
		if err := vm.writeRAM(int(address)+offset, value); err != nil {
			return err
		}
		if err := vm.writeRAM(int(address)+offset+1, value2); err != nil {
			return err
		}
		if err := vm.writeRAM(int(address)+offset+2, value3); err != nil {
			return err
		}
		return vm.writeRAM(int(address)+offset+3, value3)
	default:
		return fmt.Errorf("unknown store indirect opcode: %d", opcode)
	}
}

// Implement loadInd logic
func (vm *VM) loadInd(opcode byte, operands []byte) error {
	destRegisterIndex, registerIndexB := splitRegister(operands[0])

	wB, err := vm.readRegister(registerIndexB)
	if err != nil {
		return err
	}
	immediate := get_elided_uint32(operands[1:])

	address := wB + immediate
	switch opcode {
	case LOAD_IND_U8:
		value, err := vm.readRAM(address)
		if err != nil {
			return err
		}
		return vm.writeRegister(destRegisterIndex, uint32(value))
	case LOAD_IND_I8:
		value, err := vm.readRAM(address)
		if err != nil {
			return err
		}
		v := int(value)
		if v > 127 {
			v -= 256
		}
		return vm.writeRegister(destRegisterIndex, uint32(v))
	case LOAD_IND_U16:
		value, err := vm.readRAMBytes(address, 2)
		if err != nil {
			return err
		}
		return vm.writeRegister(destRegisterIndex, uint32(binary.BigEndian.Uint16(value)))
	case LOAD_IND_I16:
		value, err := vm.readRAMBytes(address, 2)
		if err != nil {
			return err
		}
		return vm.writeRegister(destRegisterIndex, uint32(binary.BigEndian.Uint16(value)))
	case LOAD_IND_U32:
		value, err := vm.readRAMBytes(address, 4)
		if err != nil {
			return err
		}
		return vm.writeRegister(destRegisterIndex, uint32(binary.BigEndian.Uint32(value)))
	default:
		return fmt.Errorf("unknown load indirect opcode: %d", opcode)
	}
}

func splitRegister(operand byte) (int, int) {
	registerIndexA := int(operand & 0xF)
	registerIndexB := int((operand >> 4) & 0xF)
	return registerIndexA, registerIndexB
}

// Implement ALU operations with immediate values
func (vm *VM) aluImm(opcode byte, operands []byte) error {
	registerIndexD, registerIndexA := splitRegister(operands[0])
	immediate := get_elided_uint32(operands[1:])
	fmt.Printf("a=%d d=%d immediate=%d\n", registerIndexA, registerIndexD, immediate)

	valueA, err := vm.readRegister(registerIndexA)
	if err != nil {
		return err
	}

	var result uint32
	switch opcode {
	case ADD_IMM:
		result = valueA + immediate
	case SUB_IMM:
		result = valueA - immediate
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
		immediate0 := get_elided_int32(operands[1:])
		if int32(valueA) < int32(immediate0) {
			result = 1
		} else {
			result = 0
		}
	default:
		return fmt.Errorf("unknown ALU immediate opcode: %d", opcode)
	}
	return vm.writeRegister(registerIndexD, result)
}

// Implement shift operations with immediate values
func (vm *VM) shiftImm(opcode byte, operands []byte) error {
	registerIndexB, registerIndexA := splitRegister(operands[0])
	immediate := get_elided_uint32(operands[1:])

	valueA, err := vm.readRegister(registerIndexA)
	if err != nil {
		return err
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
		immediate2 := get_elided_int32(operands[1:])
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
		return fmt.Errorf("unknown shift immediate opcode: %d", opcode)
	}
	fmt.Printf(" shiftImm r[%d]=%d immediate=%d r[%d]=%d\n", registerIndexA, valueA, immediate, registerIndexB, result)
	return vm.writeRegister(registerIndexB, result)
}

// Implement branch logic for two registers and one offset
func (vm *VM) branchReg(opcode byte, operands []byte) error {
	if len(operands) != 3 {
		return errors.New("invalid operand length for branch with registers")
	}
	registerIndexA := int(operands[0])
	registerIndexB := int(operands[1])
	offset := int(operands[2])

	valueA, err := vm.readRegister(registerIndexA)
	if err != nil {
		return err
	}
	valueB, err := vm.readRegister(registerIndexB)
	if err != nil {
		return err
	}

	switch opcode {
	case BRANCH_EQ:
		if valueA == valueB {
			vm.pc += offset
		}
	case BRANCH_NE:
		if valueA != valueB {
			vm.pc += offset
		}
	case BRANCH_LT_U:
		if uint(valueA) < uint(valueB) {
			vm.pc += offset
		}
	case BRANCH_LT_S:
		if int(valueA) < int(valueB) {
			vm.pc += offset
		}
	case BRANCH_GE_U:
		if uint(valueA) >= uint(valueB) {
			vm.pc += offset
		}
	case BRANCH_GE_S:
		if int(valueA) >= int(valueB) {
			vm.pc += offset
		}
	default:
		return fmt.Errorf("unknown branch opcode: %d", opcode)
	}

	return nil
}

// Implement ALU operations with register values
func (vm *VM) aluReg(opcode byte, operands []byte) error {

	registerIndexD, registerIndexA := splitRegister(operands[0]) // 0x79
	registerIndexB := int(operands[1])
	valueA, err := vm.readRegister(registerIndexA)
	if err != nil {
		return err
	}
	valueB, err := vm.readRegister(registerIndexB)
	if err != nil {
		return err
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
			return errors.New("overflow in signed remainder")
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
		result = valueA >> (valueB % 32)
		mask := int32((1<<valueB)-1) << (32 - valueB)
		result |= uint32(mask)
	default:
		return fmt.Errorf("unknown ALU register opcode: %d", opcode)
	}

	fmt.Printf(" aluReg - rA[%d]=%d  regB[%d]=%d regD[%d]=%d\n", registerIndexA, valueA, registerIndexB, valueB, registerIndexD, result)
	return vm.writeRegister(registerIndexD, result)
}
