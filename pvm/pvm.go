package pvm

import (
	"errors"
	"fmt"
)

// Define the types used in the PVM
type Instruction struct {
	opcode   byte
	operands []uint32
}

type Program struct {
	code       []Instruction
	pc         int // Program counter
	terminated bool
	hostCall   bool
}

// Define the VM state
type VM struct {
	program  *Program
	ram      []uint32
	register []uint32
	hostEnv  JAMEnvironment
}

// NewVM initializes a new VM with a given program
func NewVM(program *Program, ramSize int, regSize int) *VM {
	return &VM{
		program:  program,
		ram:      make([]uint32, ramSize),
		register: make([]uint32, regSize),
	}
}

// Define opcodes and instructions
const (
	TRAP = iota
	FALLTHROUGH
	JUMP
	LOAD_IMM_JUMP
	LOAD_IMM_JUMP_IND
	BRANCH_EQ
	BRANCH_NE
	BRANCH_GE_U
	BRANCH_GE_S
	BRANCH_LT_U
	BRANCH_LT_S
	BRANCH_EQ_IMM
	BRANCH_NE_IMM
	BRANCH_LT_U_IMM
	BRANCH_LT_S_IMM
	BRANCH_LE_U_IMM
	BRANCH_LE_S_IMM
	BRANCH_GE_U_IMM
	BRANCH_GE_S_IMM
	BRANCH_GT_U_IMM
	BRANCH_GT_S_IMM
	DJUMP
	ECALL
	STORE_IMM_U8
	STORE_IMM_U16
	STORE_IMM_U32
	LOAD_IMM
	LOAD_U8
	LOAD_U16
	LOAD_U32
	STORE_U8
	STORE_U16
	STORE_U32
	STORE_IMM_IND_U8
	STORE_IMM_IND_U16
	STORE_IMM_IND_U32
	MOVE_REG
	SBRK
	STORE_IND_U8
	STORE_IND_U16
	STORE_IND_U32
	LOAD_IND_U8
	LOAD_IND_I8
	LOAD_IND_U16
	LOAD_IND_I16
	LOAD_IND_U32
	ADD_IMM
	AND_IMM
	XOR_IMM
	OR_IMM
	MUL_IMM
	MUL_UPPER_S_S_IMM
	MUL_UPPER_U_U_IMM
	SET_LT_U_IMM
	SET_LT_S_IMM
	SHLO_R_IMM
	SHLO_L_IMM
	SHAR_R_IMM
	NEG_ADD_IMM
	SET_GT_U_IMM
	SET_GT_S_IMM
	SHLO_R_IMM_ALT
	SHLO_L_IMM_ALT
	BRANCH_EQ_REG
	BRANCH_NE_REG
	BRANCH_LT_U_REG
	BRANCH_LT_S_REG
	BRANCH_GE_U_REG
	BRANCH_GE_S_REG
	LOAD_IMM_JUMP_IND2
	ADD_REG
	AND_REG
	XOR_REG
	OR_REG
	MUL_REG
	MUL_UPPER_S_S_REG
	MUL_UPPER_U_U_REG
	MUL_UPPER_S_U_REG
	DIV_U
	DIV_S
	REM_U
	REM_S
	CMOV_IZ
	CMOV_NZ
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

// Execute runs the program until it terminates
func (vm *VM) Execute() error {
	for !vm.program.terminated {
		if err := vm.step(); err != nil {
			return err
		}
	}
	return nil
}

// step performs a single step in the PVM
func (vm *VM) step() error {
	if vm.program.pc < 0 || vm.program.pc >= len(vm.program.code) {
		return errors.New("program counter out of bounds")
	}

	instr := vm.program.code[vm.program.pc]
	switch instr.opcode {
	case TRAP, FALLTHROUGH:
		vm.program.terminated = true
	case JUMP:
		if err := vm.branch(instr.operands, vm.program.pc+1); err != nil {
			return err
		}
	case LOAD_IMM_JUMP:
		if err := vm.loadImmJump(instr.operands); err != nil {
			return err
		}
	case LOAD_IMM_JUMP_IND:
		if err := vm.loadImmJumpInd(instr.operands); err != nil {
			return err
		}
	case BRANCH_EQ:
		if err := vm.branchEq(instr.operands); err != nil {
			return err
		}
	case BRANCH_NE:
		if err := vm.branchNe(instr.operands); err != nil {
			return err
		}
	case BRANCH_GE_U:
		if err := vm.branchGeU(instr.operands); err != nil {
			return err
		}
	case BRANCH_GE_S:
		if err := vm.branchGeS(instr.operands); err != nil {
			return err
		}
	case BRANCH_LT_U:
		if err := vm.branchLtU(instr.operands); err != nil {
			return err
		}
	case BRANCH_LT_S:
		if err := vm.branchLtS(instr.operands); err != nil {
			return err
		}
	case BRANCH_EQ_IMM:
		if err := vm.branchEqImm(instr.operands); err != nil {
			return err
		}
	case BRANCH_NE_IMM:
		if err := vm.branchNeImm(instr.operands); err != nil {
			return err
		}
	case BRANCH_LT_U_IMM:
		if err := vm.branchLtUImm(instr.operands); err != nil {
			return err
		}
	case BRANCH_LT_S_IMM:
		if err := vm.branchLtSImm(instr.operands); err != nil {
			return err
		}
	case BRANCH_LE_U_IMM:
		if err := vm.branchLeUImm(instr.operands); err != nil {
			return err
		}
	case BRANCH_LE_S_IMM:
		if err := vm.branchLeSImm(instr.operands); err != nil {
			return err
		}
	case BRANCH_GE_U_IMM:
		if err := vm.branchGeUImm(instr.operands); err != nil {
			return err
		}
	case BRANCH_GE_S_IMM:
		if err := vm.branchGeSImm(instr.operands); err != nil {
			return err
		}
	case BRANCH_GT_U_IMM:
		if err := vm.branchGtUImm(instr.operands); err != nil {
			return err
		}
	case BRANCH_GT_S_IMM:
		if err := vm.branchGtSImm(instr.operands); err != nil {
			return err
		}
	case DJUMP:
		if err := vm.dynamicJump(instr.operands); err != nil {
			return err
		}
	case ECALL:
		if err := vm.ecall(instr.operands); err != nil {
			return err
		}
	case STORE_IMM_U8, STORE_IMM_U16, STORE_IMM_U32:
		if err := vm.storeImm(instr.opcode, instr.operands); err != nil {
			return err
		}
	case LOAD_IMM:
		if err := vm.loadImm(instr.operands); err != nil {
			return err
		}
	case LOAD_U8, LOAD_U16, LOAD_U32:
		if err := vm.load(instr.opcode, instr.operands); err != nil {
			return err
		}
	case STORE_U8, STORE_U16, STORE_U32:
		if err := vm.store(instr.opcode, instr.operands); err != nil {
			return err
		}
	case STORE_IMM_IND_U8, STORE_IMM_IND_U16, STORE_IMM_IND_U32:
		if err := vm.storeImmInd(instr.opcode, instr.operands); err != nil {
			return err
		}
	case STORE_IND_U8, STORE_IND_U16, STORE_IND_U32:
		if err := vm.storeInd(instr.opcode, instr.operands); err != nil {
			return err
		}
	case LOAD_IND_U8, LOAD_IND_I8, LOAD_IND_U16, LOAD_IND_I16, LOAD_IND_U32:
		if err := vm.loadInd(instr.opcode, instr.operands); err != nil {
			return err
		}
	case ADD_IMM, AND_IMM, XOR_IMM, OR_IMM, MUL_IMM, MUL_UPPER_S_S_IMM, MUL_UPPER_U_U_IMM, SET_LT_U_IMM, SET_LT_S_IMM:
		if err := vm.aluImm(instr.opcode, instr.operands); err != nil {
			return err
		}
	case SHLO_R_IMM, SHLO_L_IMM, SHAR_R_IMM, NEG_ADD_IMM, SET_GT_U_IMM, SET_GT_S_IMM, SHLO_R_IMM_ALT, SHLO_L_IMM_ALT:
		if err := vm.shiftImm(instr.opcode, instr.operands); err != nil {
			return err
		}
	case BRANCH_EQ_REG, BRANCH_NE_REG, BRANCH_LT_U_REG, BRANCH_LT_S_REG, BRANCH_GE_U_REG, BRANCH_GE_S_REG:
		if err := vm.branchReg(instr.opcode, instr.operands); err != nil {
			return err
		}
	case LOAD_IMM_JUMP_IND2:
		if err := vm.loadImmJumpInd2(instr.operands); err != nil {
			return err
		}
	case ADD_REG, AND_REG, XOR_REG, OR_REG, MUL_REG, MUL_UPPER_S_S_REG, MUL_UPPER_U_U_REG, MUL_UPPER_S_U_REG, DIV_U, DIV_S, REM_U, REM_S, CMOV_IZ, CMOV_NZ:
		if err := vm.aluReg(instr.opcode, instr.operands); err != nil {
			return err
		}
	case MOVE_REG:
		if err := vm.moveReg(instr.operands); err != nil {
			return err
		}
	case SBRK:
		if err := vm.sbrk(instr.operands); err != nil {
			return err
		}
	default:
		// Handle host call
		halt, err := vm.handleHostCall(instr.opcode, instr.operands)
		if err != nil {
			return err
		}
		if halt {
			// Host-call halt condition
			vm.program.hostCall = true
			vm.program.pc-- // Decrement PC to point to the host-call instruction
			return nil
		}
	}

	vm.program.pc += 1 + skip(instr.opcode)
	return nil
}

// handleHostCall handles host-call instructions
func (vm *VM) handleHostCall(opcode byte, operands []uint32) (bool, error) {
	if vm.hostEnv == nil {
		return false, errors.New("no host environment configured")
	}
	return vm.hostEnv.InvokeHostCall(opcode, operands, vm)
}

// skip calculates the skip distance based on the opcode
func skip(opcode byte) int {
	switch opcode {
	case JUMP, LOAD_IMM_JUMP, LOAD_IMM_JUMP_IND, LOAD_IMM_JUMP_IND2:
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
func (vm *VM) readRAM(address int) (uint32, error) {
	if address < 0 || address >= len(vm.ram) {
		return 0, errors.New("RAM access out of bounds")
	}
	return vm.ram[address], nil
}

func (vm *VM) writeRAM(address int, value uint32) error {
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
func (vm *VM) dynamicJump(operands []uint32) error {
	if len(operands) != 1 {
		return errors.New("invalid operand length for dynamic jump")
	}
	a := int(operands[0])
	const ZA = 4

	if a == 0 || a > 0x7FFFFFFF {
		return errors.New("dynamic jump address out of bounds")
	}

	targetIndex := a/ZA - 1
	if targetIndex < 0 || targetIndex >= len(vm.program.code) {
		return errors.New("dynamic jump target index out of bounds")
	}

	vm.program.pc = targetIndex
	return nil
}

// Implement ecall logic
func (vm *VM) ecall(operands []uint32) error {
	// Implement ecall logic here
	return nil
}

// Implement storeImm logic
func (vm *VM) storeImm(opcode byte, operands []uint32) error {
	if len(operands) < 2 {
		return errors.New("invalid operand length for store immediate")
	}
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

// Implement loadImm logic
func (vm *VM) loadImm(operands []uint32) error {
	if len(operands) != 2 {
		return errors.New("invalid operand length for load immediate")
	}
	registerIndex := int(operands[0])
	value := operands[1]
	return vm.writeRegister(registerIndex, value)
}

// Implement load logic
func (vm *VM) load(opcode byte, operands []uint32) error {
	if len(operands) < 2 {
		return errors.New("invalid operand length for load")
	}
	address := int(operands[0])
	registerIndex := int(operands[1])

	switch opcode {
	case LOAD_U8:
		value, err := vm.readRAM(address)
		if err != nil {
			return err
		}
		return vm.writeRegister(registerIndex, value)
	case LOAD_U16:
		if len(operands) < 3 {
			return errors.New("invalid operand length for load u16")
		}
		value, err := vm.readRAM(address)
		if err != nil {
			return err
		}
		value2, err := vm.readRAM(address + 1)
		if err != nil {
			return err
		}
		return vm.writeRegister(registerIndex, value+value2<<8)
	case LOAD_U32:
		if len(operands) < 5 {
			return errors.New("invalid operand length for load u32")
		}
		value, err := vm.readRAM(address)
		if err != nil {
			return err
		}
		value2, err := vm.readRAM(address + 1)
		if err != nil {
			return err
		}
		value3, err := vm.readRAM(address + 2)
		if err != nil {
			return err
		}
		value4, err := vm.readRAM(address + 3)
		if err != nil {
			return err
		}
		return vm.writeRegister(registerIndex, value+value2<<8+value3<<16+value4<<24)
	default:
		return fmt.Errorf("unknown load opcode: %d", opcode)
	}
}

// Implement store logic
func (vm *VM) store(opcode byte, operands []uint32) error {
	if len(operands) < 2 {
		return errors.New("invalid operand length for store")
	}
	address := int(operands[0])
	registerIndex := int(operands[1])

	value, err := vm.readRegister(registerIndex)
	if err != nil {
		return err
	}

	switch opcode {
	case STORE_U8:
		return vm.writeRAM(address, value)
	case STORE_U16:
		if len(operands) < 3 {
			return errors.New("invalid operand length for store u16")
		}
		value2, err := vm.readRegister(registerIndex + 1)
		if err != nil {
			return err
		}
		if err := vm.writeRAM(address, value); err != nil {
			return err
		}
		return vm.writeRAM(address+1, value2)
	case STORE_U32:
		if len(operands) < 5 {
			return errors.New("invalid operand length for store u32")
		}
		value2, err := vm.readRegister(registerIndex + 1)
		if err != nil {
			return err
		}
		value3, err := vm.readRegister(registerIndex + 2)
		if err != nil {
			return err
		}
		value4, err := vm.readRegister(registerIndex + 3)
		if err != nil {
			return err
		}
		if err := vm.writeRAM(address, value); err != nil {
			return err
		}
		if err := vm.writeRAM(address+1, value2); err != nil {
			return err
		}
		if err := vm.writeRAM(address+2, value3); err != nil {
			return err
		}
		return vm.writeRAM(address+3, value4)
	default:
		return fmt.Errorf("unknown store opcode: %d", opcode)
	}
}

// Implement storeImmInd logic
func (vm *VM) storeImmInd(opcode byte, operands []uint32) error {
	if len(operands) < 3 {
		return errors.New("invalid operand length for store immediate indirect")
	}
	registerIndex := int(operands[0])
	offset := int(operands[1])
	value := operands[2]

	address, err := vm.readRegister(registerIndex)
	if err != nil {
		return err
	}

	switch opcode {
	case STORE_IMM_IND_U8:
		return vm.writeRAM(int(address)+offset, value)
	case STORE_IMM_IND_U16:
		if len(operands) < 4 {
			return errors.New("invalid operand length for store immediate indirect u16")
		}
		value2 := operands[3]
		if err := vm.writeRAM(int(address)+offset, value); err != nil {
			return err
		}
		return vm.writeRAM(int(address)+offset+1, value2)
	case STORE_IMM_IND_U32:
		if len(operands) < 6 {
			return errors.New("invalid operand length for store immediate indirect u32")
		}
		value2 := operands[3]
		value3 := operands[4]
		value4 := operands[5]
		if err := vm.writeRAM(int(address)+offset, value); err != nil {
			return err
		}
		if err := vm.writeRAM(int(address)+offset+1, value2); err != nil {
			return err
		}
		if err := vm.writeRAM(int(address)+offset+2, value3); err != nil {
			return err
		}
		return vm.writeRAM(int(address)+offset+3, value4)
	default:
		return fmt.Errorf("unknown store immediate indirect opcode: %d", opcode)
	}
}

// Implement loadImmJump logic
func (vm *VM) loadImmJump(operands []uint32) error {
	if len(operands) != 2 {
		return errors.New("invalid operand length for load immediate jump")
	}
	condition := operands[0]
	target := int(operands[1])

	if condition != 0 {
		if target < 0 || target >= len(vm.program.code) {
			return errors.New("load immediate jump target out of bounds")
		}
		vm.program.pc = target
	}
	return nil
}

// Implement loadImmJumpInd logic
func (vm *VM) loadImmJumpInd(operands []uint32) error {
	if len(operands) != 2 {
		return errors.New("invalid operand length for load immediate jump indirect")
	}
	index := int(operands[0])
	offset := int(operands[1])

	address, err := vm.readRegister(index)
	if err != nil {
		return err
	}

	target := int(address) + offset
	if target < 0 || target >= len(vm.program.code) {
		return errors.New("load immediate jump target out of bounds")
	}

	vm.program.pc = target
	return nil
}

// Implement moveReg logic
func (vm *VM) moveReg(operands []uint32) error {
	if len(operands) != 2 {
		return errors.New("invalid operand length for move register")
	}
	srcIndex := int(operands[0])
	destIndex := int(operands[1])

	value, err := vm.readRegister(srcIndex)
	if err != nil {
		return err
	}
	return vm.writeRegister(destIndex, value)
}

// Implement sbrk logic
func (vm *VM) sbrk(operands []uint32) error {
	if len(operands) != 1 {
		return errors.New("invalid operand length for sbrk")
	}
	amount := int(operands[0])
	newRAM := make([]uint32, len(vm.ram)+amount)
	copy(newRAM, vm.ram)
	vm.ram = newRAM
	return nil
}

// Implement branch logic
func (vm *VM) branch(operands []uint32, currentPc int) error {
	if len(operands) != 2 {
		return errors.New("invalid operand length for branch")
	}
	condition := operands[0]
	target := int(operands[1])

	// Implement the condition checking and branch logic here
	if condition != 0 {
		if target < 0 || target >= len(vm.program.code) {
			return errors.New("branch target out of bounds")
		}
		vm.program.pc = target
	} else {
		vm.program.pc = currentPc
	}

	return nil
}

// Implement branchEq logic
func (vm *VM) branchEq(operands []uint32) error {
	if len(operands) != 3 {
		return errors.New("invalid operand length for branch equal")
	}
	registerIndexA := int(operands[0])
	registerIndexB := int(operands[1])
	target := int(operands[2])

	valueA, err := vm.readRegister(registerIndexA)
	if err != nil {
		return err
	}
	valueB, err := vm.readRegister(registerIndexB)
	if err != nil {
		return err
	}

	if valueA == valueB {
		if target < 0 || target >= len(vm.program.code) {
			return errors.New("branch target out of bounds")
		}
		vm.program.pc = target
	}
	return nil
}

// Implement branchNe logic
func (vm *VM) branchNe(operands []uint32) error {
	if len(operands) != 3 {
		return errors.New("invalid operand length for branch not equal")
	}
	registerIndexA := int(operands[0])
	registerIndexB := int(operands[1])
	target := int(operands[2])

	valueA, err := vm.readRegister(registerIndexA)
	if err != nil {
		return err
	}
	valueB, err := vm.readRegister(registerIndexB)
	if err != nil {
		return err
	}

	if valueA != valueB {
		if target < 0 || target >= len(vm.program.code) {
			return errors.New("branch target out of bounds")
		}
		vm.program.pc = target
	}
	return nil
}

// Implement branchGeU logic
func (vm *VM) branchGeU(operands []uint32) error {
	if len(operands) != 3 {
		return errors.New("invalid operand length for branch greater or equal unsigned")
	}
	registerIndexA := int(operands[0])
	registerIndexB := int(operands[1])
	target := int(operands[2])

	valueA, err := vm.readRegister(registerIndexA)
	if err != nil {
		return err
	}
	valueB, err := vm.readRegister(registerIndexB)
	if err != nil {
		return err
	}

	if uint(valueA) >= uint(valueB) {
		if target < 0 || target >= len(vm.program.code) {
			return errors.New("branch target out of bounds")
		}
		vm.program.pc = target
	}
	return nil
}

// Implement branchGeS logic
func (vm *VM) branchGeS(operands []uint32) error {
	if len(operands) != 3 {
		return errors.New("invalid operand length for branch greater or equal signed")
	}
	registerIndexA := int(operands[0])
	registerIndexB := int(operands[1])
	target := int(operands[2])

	valueA, err := vm.readRegister(registerIndexA)
	if err != nil {
		return err
	}
	valueB, err := vm.readRegister(registerIndexB)
	if err != nil {
		return err
	}

	if int(valueA) >= int(valueB) {
		if target < 0 || target >= len(vm.program.code) {
			return errors.New("branch target out of bounds")
		}
		vm.program.pc = target
	}
	return nil
}

// Implement branchLtU logic
func (vm *VM) branchLtU(operands []uint32) error {
	if len(operands) != 3 {
		return errors.New("invalid operand length for branch less than unsigned")
	}
	registerIndexA := int(operands[0])
	registerIndexB := int(operands[1])
	target := int(operands[2])

	valueA, err := vm.readRegister(registerIndexA)
	if err != nil {
		return err
	}
	valueB, err := vm.readRegister(registerIndexB)
	if err != nil {
		return err
	}

	if uint(valueA) < uint(valueB) {
		if target < 0 || target >= len(vm.program.code) {
			return errors.New("branch target out of bounds")
		}
		vm.program.pc = target
	}
	return nil
}

// Implement branchLtS logic
func (vm *VM) branchLtS(operands []uint32) error {
	if len(operands) != 3 {
		return errors.New("invalid operand length for branch less than signed")
	}
	registerIndexA := int(operands[0])
	registerIndexB := int(operands[1])
	target := int(operands[2])

	valueA, err := vm.readRegister(registerIndexA)
	if err != nil {
		return err
	}
	valueB, err := vm.readRegister(registerIndexB)
	if err != nil {
		return err
	}

	if int(valueA) < int(valueB) {
		if target < 0 || target >= len(vm.program.code) {
			return errors.New("branch target out of bounds")
		}
		vm.program.pc = target
	}
	return nil
}

// Implement branchEqImm logic
func (vm *VM) branchEqImm(operands []uint32) error {
	if len(operands) != 3 {
		return errors.New("invalid operand length for branch equal immediate")
	}
	registerIndex := int(operands[0])
	immediate := operands[1]
	target := int(operands[2])

	value, err := vm.readRegister(registerIndex)
	if err != nil {
		return err
	}

	if value == immediate {
		if target < 0 || target >= len(vm.program.code) {
			return errors.New("branch target out of bounds")
		}
		vm.program.pc = target
	}
	return nil
}

// Implement branchNeImm logic
func (vm *VM) branchNeImm(operands []uint32) error {
	if len(operands) != 3 {
		return errors.New("invalid operand length for branch not equal immediate")
	}
	registerIndex := int(operands[0])
	immediate := operands[1]
	target := int(operands[2])

	value, err := vm.readRegister(registerIndex)
	if err != nil {
		return err
	}

	if value != immediate {
		if target < 0 || target >= len(vm.program.code) {
			return errors.New("branch target out of bounds")
		}
		vm.program.pc = target
	}
	return nil
}

// Implement branchLtUImm logic
func (vm *VM) branchLtUImm(operands []uint32) error {
	if len(operands) != 3 {
		return errors.New("invalid operand length for branch less than unsigned immediate")
	}
	registerIndex := int(operands[0])
	immediate := operands[1]
	target := int(operands[2])

	value, err := vm.readRegister(registerIndex)
	if err != nil {
		return err
	}

	if uint(value) < uint(immediate) {
		if target < 0 || target >= len(vm.program.code) {
			return errors.New("branch target out of bounds")
		}
		vm.program.pc = target
	}
	return nil
}

// Implement branchLtSImm logic
func (vm *VM) branchLtSImm(operands []uint32) error {
	if len(operands) != 3 {
		return errors.New("invalid operand length for branch less than signed immediate")
	}
	registerIndex := int(operands[0])
	immediate := operands[1]
	target := int(operands[2])

	value, err := vm.readRegister(registerIndex)
	if err != nil {
		return err
	}

	if int(value) < int(immediate) {
		if target < 0 || target >= len(vm.program.code) {
			return errors.New("branch target out of bounds")
		}
		vm.program.pc = target
	}
	return nil
}

// Implement branchLeUImm logic
func (vm *VM) branchLeUImm(operands []uint32) error {
	if len(operands) != 3 {
		return errors.New("invalid operand length for branch less or equal unsigned immediate")
	}
	registerIndex := int(operands[0])
	immediate := operands[1]
	target := int(operands[2])

	value, err := vm.readRegister(registerIndex)
	if err != nil {
		return err
	}

	if uint(value) <= uint(immediate) {
		if target < 0 || target >= len(vm.program.code) {
			return errors.New("branch target out of bounds")
		}
		vm.program.pc = target
	}
	return nil
}

// Implement branchLeSImm logic
func (vm *VM) branchLeSImm(operands []uint32) error {
	if len(operands) != 3 {
		return errors.New("invalid operand length for branch less or equal signed immediate")
	}
	registerIndex := int(operands[0])
	immediate := operands[1]
	target := int(operands[2])

	value, err := vm.readRegister(registerIndex)
	if err != nil {
		return err
	}

	if int(value) <= int(immediate) {
		if target < 0 || target >= len(vm.program.code) {
			return errors.New("branch target out of bounds")
		}
		vm.program.pc = target
	}
	return nil
}

// Implement branchGeUImm logic
func (vm *VM) branchGeUImm(operands []uint32) error {
	if len(operands) != 3 {
		return errors.New("invalid operand length for branch greater or equal unsigned immediate")
	}
	registerIndex := int(operands[0])
	immediate := operands[1]
	target := int(operands[2])

	value, err := vm.readRegister(registerIndex)
	if err != nil {
		return err
	}

	if uint(value) >= uint(immediate) {
		if target < 0 || target >= len(vm.program.code) {
			return errors.New("branch target out of bounds")
		}
		vm.program.pc = target
	}
	return nil
}

// Implement branchGeSImm logic
func (vm *VM) branchGeSImm(operands []uint32) error {
	if len(operands) != 3 {
		return errors.New("invalid operand length for branch greater or equal signed immediate")
	}
	registerIndex := int(operands[0])
	immediate := operands[1]
	target := int(operands[2])

	value, err := vm.readRegister(registerIndex)
	if err != nil {
		return err
	}

	if int(value) >= int(immediate) {
		if target < 0 || target >= len(vm.program.code) {
			return errors.New("branch target out of bounds")
		}
		vm.program.pc = target
	}
	return nil
}

// Implement branchGtUImm logic
func (vm *VM) branchGtUImm(operands []uint32) error {
	if len(operands) != 3 {
		return errors.New("invalid operand length for branch greater than unsigned immediate")
	}
	registerIndex := int(operands[0])
	immediate := operands[1]
	target := int(operands[2])

	value, err := vm.readRegister(registerIndex)
	if err != nil {
		return err
	}

	if uint(value) > uint(immediate) {
		if target < 0 || target >= len(vm.program.code) {
			return errors.New("branch target out of bounds")
		}
		vm.program.pc = target
	}
	return nil
}

// Implement branchGtSImm logic
func (vm *VM) branchGtSImm(operands []uint32) error {
	if len(operands) != 3 {
		return errors.New("invalid operand length for branch greater than signed immediate")
	}
	registerIndex := int(operands[0])
	immediate := operands[1]
	target := int(operands[2])

	value, err := vm.readRegister(registerIndex)
	if err != nil {
		return err
	}

	if int(value) > int(immediate) {
		if target < 0 || target >= len(vm.program.code) {
			return errors.New("branch target out of bounds")
		}
		vm.program.pc = target
	}
	return nil
}

// Implement storeInd logic
func (vm *VM) storeInd(opcode byte, operands []uint32) error {
	if len(operands) != 3 {
		return errors.New("invalid operand length for store indirect")
	}
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
func (vm *VM) loadInd(opcode byte, operands []uint32) error {
	if len(operands) < 3 {
		return errors.New("invalid operand length for load indirect")
	}
	registerIndex := int(operands[0])
	offset := int(operands[1])
	destRegisterIndex := int(operands[2])

	address, err := vm.readRegister(registerIndex)
	if err != nil {
		return err
	}

	switch opcode {
	case LOAD_IND_U8:
		value, err := vm.readRAM(int(address) + offset)
		if err != nil {
			return err
		}
		return vm.writeRegister(destRegisterIndex, value)
	case LOAD_IND_I8:
		value, err := vm.readRAM(int(address) + offset)
		if err != nil {
			return err
		}
		if value > 127 {
			value -= 256
		}
		return vm.writeRegister(destRegisterIndex, value)
	case LOAD_IND_U16:
		if len(operands) < 4 {
			return errors.New("invalid operand length for load indirect u16")
		}
		value, err := vm.readRAM(int(address) + offset)
		if err != nil {
			return err
		}
		value2, err := vm.readRAM(int(address) + offset + 1)
		if err != nil {
			return err
		}
		return vm.writeRegister(destRegisterIndex, value+value2<<8)
	case LOAD_IND_I16:
		if len(operands) < 4 {
			return errors.New("invalid operand length for load indirect i16")
		}
		value, err := vm.readRAM(int(address) + offset)
		if err != nil {
			return err
		}
		value2, err := vm.readRAM(int(address) + offset + 1)
		if err != nil {
			return err
		}
		if value2 > 127 {
			value2 -= 256
		}
		return vm.writeRegister(destRegisterIndex, value+value2<<8)
	case LOAD_IND_U32:
		if len(operands) < 6 {
			return errors.New("invalid operand length for load indirect u32")
		}
		value, err := vm.readRAM(int(address) + offset)
		if err != nil {
			return err
		}
		value2, err := vm.readRAM(int(address) + offset + 1)
		if err != nil {
			return err
		}
		value3, err := vm.readRAM(int(address) + offset + 2)
		if err != nil {
			return err
		}
		value4, err := vm.readRAM(int(address) + offset + 3)
		if err != nil {
			return err
		}
		return vm.writeRegister(destRegisterIndex, value+value2<<8+value3<<16+value4<<24)
	default:
		return fmt.Errorf("unknown load indirect opcode: %d", opcode)
	}
}

// Implement ALU operations with immediate values
func (vm *VM) aluImm(opcode byte, operands []uint32) error {
	if len(operands) != 3 {
		return errors.New("invalid operand length for ALU immediate")
	}
	registerIndexA := int(operands[0])
	immediate := operands[1]
	registerIndexB := int(operands[2])

	valueA, err := vm.readRegister(registerIndexA)
	if err != nil {
		return err
	}

	var result uint32
	switch opcode {
	case ADD_IMM:
		result = valueA + immediate
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
		if uint(valueA) < uint(immediate) {
			result = 1
		} else {
			result = 0
		}
	case SET_LT_S_IMM:
		if int(valueA) < int(immediate) {
			result = 1
		} else {
			result = 0
		}
	default:
		return fmt.Errorf("unknown ALU immediate opcode: %d", opcode)
	}

	return vm.writeRegister(registerIndexB, result)
}

// Implement shift operations with immediate values
func (vm *VM) shiftImm(opcode byte, operands []uint32) error {
	if len(operands) != 3 {
		return errors.New("invalid operand length for shift immediate")
	}
	registerIndexA := int(operands[0])
	immediate := operands[1]
	registerIndexB := int(operands[2])

	valueA, err := vm.readRegister(registerIndexA)
	if err != nil {
		return err
	}

	var result uint32
	switch opcode {
	case SHLO_R_IMM:
		result = valueA << immediate
	case SHLO_L_IMM:
		result = valueA >> immediate
	case SHAR_R_IMM:
		result = uint32(int(valueA) >> immediate)
	case NEG_ADD_IMM:
		result = uint32(-int(valueA) + int(immediate))
	case SET_GT_U_IMM:
		if uint(valueA) > uint(immediate) {
			result = 1
		} else {
			result = 0
		}
	case SET_GT_S_IMM:
		if int(valueA) > int(immediate) {
			result = 1
		} else {
			result = 0
		}
	case SHLO_R_IMM_ALT:
		result = valueA << immediate
	case SHLO_L_IMM_ALT:
		result = valueA >> immediate
	default:
		return fmt.Errorf("unknown shift immediate opcode: %d", opcode)
	}

	return vm.writeRegister(registerIndexB, result)
}

// Implement branch logic for two registers and one offset
func (vm *VM) branchReg(opcode byte, operands []uint32) error {
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
	case BRANCH_EQ_REG:
		if valueA == valueB {
			vm.program.pc += offset
		}
	case BRANCH_NE_REG:
		if valueA != valueB {
			vm.program.pc += offset
		}
	case BRANCH_LT_U_REG:
		if uint(valueA) < uint(valueB) {
			vm.program.pc += offset
		}
	case BRANCH_LT_S_REG:
		if int(valueA) < int(valueB) {
			vm.program.pc += offset
		}
	case BRANCH_GE_U_REG:
		if uint(valueA) >= uint(valueB) {
			vm.program.pc += offset
		}
	case BRANCH_GE_S_REG:
		if int(valueA) >= int(valueB) {
			vm.program.pc += offset
		}
	default:
		return fmt.Errorf("unknown branch opcode: %d", opcode)
	}

	return nil
}

// Implement loadImmJumpInd2 logic for two registers and two immediates
func (vm *VM) loadImmJumpInd2(operands []uint32) error {
	if len(operands) != 4 {
		return errors.New("invalid operand length for load immediate jump indirect 2")
	}
	//registerIndexA := int(operands[0])
	registerIndexB := int(operands[1])
	immediateA := operands[2]
	immediateB := operands[3]

	/*	valueA, err := vm.readRegister(registerIndexA)
		if err != nil {
			return err
		} */
	valueB, err := vm.readRegister(registerIndexB)
	if err != nil {
		return err
	}

	address := int(valueB) + int(immediateA)
	target := (address + int(immediateB)) % (1 << 32)

	if target < 0 || target >= len(vm.program.code) {
		return errors.New("load immediate jump target out of bounds")
	}

	vm.program.pc = target
	return nil
}

// Implement ALU operations with register values
func (vm *VM) aluReg(opcode byte, operands []uint32) error {
	if len(operands) != 3 {
		return errors.New("invalid operand length for ALU register")
	}
	registerIndexA := int(operands[0])
	registerIndexB := int(operands[1])
	registerIndexD := int(operands[2])

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
			return errors.New("division by zero")
		}
		result = valueA / valueB
	case DIV_S:
		if valueB == 0 {
			return errors.New("division by zero")
		}
		if valueA == 0x80 && valueB == 0xFF {
			return errors.New("overflow in signed division")
		}
		result = uint32(int(valueA) / int(valueB))
	case REM_U:
		if valueB == 0 {
			return errors.New("division by zero")
		}
		result = valueA % valueB
	case REM_S:
		if valueB == 0 {
			return errors.New("division by zero")
		}
		if valueA == 0x80 && valueB == 0xFF {
			return errors.New("overflow in signed remainder")
		}
		result = uint32(int(valueA) % int(valueB))
	case CMOV_IZ:
		if valueB == 0 {
			result = 0
		} else {
			result = valueA
		}
	case CMOV_NZ:
		if valueB == 0 {
			result = valueA
		} else {
			result = 0
		}
	default:
		return fmt.Errorf("unknown ALU register opcode: %d", opcode)
	}

	return vm.writeRegister(registerIndexD, result)
}
