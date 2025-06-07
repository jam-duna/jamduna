package pvm

import (
	"encoding/binary"
	"fmt"
	"strings"
	"syscall"
	"unsafe"

	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
	"golang.org/x/arch/x86/x86asm"
)

type X86Reg struct {
	Name    string
	RegBits byte // 3-bit code for ModRM/SIB
	REXBit  byte // 1 if register index >= 8
}

var regInfoList = []X86Reg{
	{"rax", 0, 0}, // Commonly used as return value register
	{"rcx", 1, 0}, // Used for loop counters or intermediates
	{"rdx", 2, 0}, // Often paired with rax for mul/div
	{"rbx", 3, 0},
	{"rsi", 6, 0}, // Often used as function argument
	{"rdi", 7, 0}, // Often used as function argument
	{"r8", 0, 1},  // Typically function argument #5
	{"r9", 1, 1},
	{"r10", 2, 1},
	{"r11", 3, 1},
	{"r13", 5, 1},
	{"r14", 6, 1},
	{"r15", 7, 1},
	// the base register for memory dump
	{"r12", 4, 1},
}

var pvmByteCodeToX86Code = map[byte]func(Instruction) ([]byte, error){
	// A.5.1. Instructions without Arguments
	TRAP: func(inst Instruction) ([]byte, error) {
		return []byte{}, nil
	},
	FALLTHROUGH: func(inst Instruction) ([]byte, error) {
		return []byte{0x90}, nil
	},
	// A.5.2. Instructions with Arguments of One Immediate. InstructionI1
	ECALLI: generateSyscall(),

	// A.5.3. Instructions with Arguments of One Register and One Extended Width Immediate.
	LOAD_IMM_64: generateLoadImm64(),

	// A.5.4. Instructions with Arguments of Two Immediates.
	STORE_IMM_U8:  generateStoreImmU8(),
	STORE_IMM_U16: generateStoreImmU16(),
	STORE_IMM_U32: generateStoreImmU32(),
	STORE_IMM_U64: generateStoreImmU64(),

	// A.5.5. Instructions with Arguments of One Offset.
	JUMP: generateJumpRel32(),

	// A.5.6. Instructions with Arguments of One Register & Two Immediates.
	JUMP_IND:  generateJumpIndirect(),
	LOAD_IMM:  generateLoadImm32(),
	LOAD_U8:   generateLoadU8(),
	LOAD_I8:   generateLoadI8(),
	LOAD_U16:  generateLoadU16(),
	LOAD_I16:  generateLoadI16(),
	LOAD_U32:  generateLoadU32(),
	LOAD_I32:  generateLoadI32(),
	LOAD_U64:  generateLoadU64(),
	STORE_U8:  generateStoreU8(),
	STORE_U16: generateStoreU16(),
	STORE_U32: generateStoreU32(),
	STORE_U64: generateStoreU64(),

	// A.5.7. Instructions with Arguments of One Register & Two Immediates.
	STORE_IMM_IND_U8:  generateStoreImmIndU8(),
	STORE_IMM_IND_U16: generateStoreImmIndU16(),
	STORE_IMM_IND_U32: generateStoreImmIndU32(),
	STORE_IMM_IND_U64: generateStoreImmIndU64(),

	// A.5.8. Instructions with Arguments of One Register, One Immediate and One Offset.
	LOAD_IMM_JUMP:   generateLoadImmJump(),
	BRANCH_EQ_IMM:   generateBranchImm(0x74), // JE
	BRANCH_NE_IMM:   generateBranchImm(0x75), // JNE
	BRANCH_LT_U_IMM: generateBranchImm(0x72), // JB
	BRANCH_LE_U_IMM: generateBranchImm(0x76), // JBE
	BRANCH_GE_U_IMM: generateBranchImm(0x73), // JAE
	BRANCH_GT_U_IMM: generateBranchImm(0x77), // JA
	BRANCH_LT_S_IMM: generateBranchImm(0x7C), // JL
	BRANCH_LE_S_IMM: generateBranchImm(0x7E), // JLE
	BRANCH_GE_S_IMM: generateBranchImm(0x7D), // JGE
	BRANCH_GT_S_IMM: generateBranchImm(0x7F), // JG

	// A.5.9. Instructions with Arguments of Two Registers.
	MOVE_REG:              generateMoveReg(),
	SBRK:                  generateSyscall(),
	COUNT_SET_BITS_64:     generateBitCount64(),
	COUNT_SET_BITS_32:     generateBitCount32(),
	LEADING_ZERO_BITS_64:  generateLeadingZeros64(),
	LEADING_ZERO_BITS_32:  generateLeadingZeros32(),
	TRAILING_ZERO_BITS_64: generateTrailingZeros64(),
	TRAILING_ZERO_BITS_32: generateTrailingZeros32(),
	SIGN_EXTEND_8:         generateSignExtend8(),
	SIGN_EXTEND_16:        generateSignExtend16(),
	ZERO_EXTEND_16:        generateZeroExtend16(),
	REVERSE_BYTES:         generateReverseBytes64(),

	// A.5.10. Instructions with Arguments of Two Registers & One Immediate.
	STORE_IND_U8:      generateStoreIndirect(0x88, false, 1),
	STORE_IND_U16:     generateStoreIndirect(0x89, true, 2),
	STORE_IND_U32:     generateStoreIndirect(0x89, false, 4),
	STORE_IND_U64:     generateStoreIndirect(0x89, false, 8),
	LOAD_IND_U8:       generateLoadInd(0x8A, false, 1),
	LOAD_IND_I8:       generateLoadIndSignExtend(0x0F, 0xBE, false),
	LOAD_IND_U16:      generateLoadInd(0x8B, false, 2),
	LOAD_IND_I16:      generateLoadIndSignExtend(0x0F, 0xBF, false),
	LOAD_IND_U32:      generateLoadInd(0x8B, false, 4),
	LOAD_IND_I32:      generateLoadIndSignExtend(0x63, 0x00, true),
	LOAD_IND_U64:      generateLoadInd(0x8B, true, 8),
	ADD_IMM_32:        generateBinaryImm32(),
	AND_IMM:           generateImmBinaryOp64(0x81, 4),
	XOR_IMM:           generateImmBinaryOp64(0x81, 6),
	OR_IMM:            generateImmBinaryOp64(0x81, 1),
	MUL_IMM_32:        generateImmMulOp32(),
	SET_LT_U_IMM:      generateImmSetCondOp32(0x92), // SETB / below unsigned
	SET_LT_S_IMM:      generateImmSetCondOp32(0x9C), // SETL / below signed
	SET_GT_U_IMM:      generateImmSetCondOp32(0x97), // SETA / above unsigned
	SET_GT_S_IMM:      generateImmSetCondOp32(0x9F), // SETG / above signed
	SHLO_L_IMM_32:     generateImmShiftOp32(0xC1, 4, false),
	SHLO_R_IMM_32:     generateImmShiftOp32(0xC1, 5, false),
	SHLO_L_IMM_ALT_32: generateImmShiftOp32Alt(0xC1, 4),
	SHLO_R_IMM_ALT_32: generateImmShiftOp32(0xC1, 5, true),
	SHAR_R_IMM_ALT_32: generateImmShiftOp32(0xC1, 7, true),
	NEG_ADD_IMM_32:    generateNegAddImm32(),
	CMOV_IZ_IMM:       generateCmovIzImm(),
	CMOV_NZ_IMM:       generateCmovIzImm(),
	ADD_IMM_64:        generateImmBinaryOp64(0x81, 0),
	MUL_IMM_64:        generateImmMulOp64(),
	SHLO_L_IMM_64:     generateImmShiftOp64(0xC1, 4),
	SHLO_R_IMM_64:     generateImmShiftOp64(0xC1, 5),
	SHAR_R_IMM_32:     generateImmShiftOp32(0xC1, 7, false),
	SHAR_R_IMM_64:     generateImmShiftOp64(0xC1, 7),
	NEG_ADD_IMM_64:    generateNegAddImm64(),
	SHLO_L_IMM_ALT_64: generateImmShiftOp64Alt(0xC1, 4),
	SHLO_R_IMM_ALT_64: generateImmShiftOp64Alt(0xC1, 5),
	SHAR_R_IMM_ALT_64: generateImmShiftOp64Alt(0xC1, 7),
	ROT_R_64_IMM:      generateImmShiftOp64(0xC1, 1),
	ROT_R_64_IMM_ALT:  generateImmShiftOp64(0xC1, 1),
	ROT_R_32_IMM_ALT:  generateImmShiftOp32(0xC1, 1, true),
	ROT_R_32_IMM:      generateRotateRight32Imm(),

	// A.5.11. Instructions with Arguments of Two Registers & One Offset.
	BRANCH_EQ:   generateCompareBranch(0x0F, 0x84),
	BRANCH_NE:   generateCompareBranch(0x0F, 0x85),
	BRANCH_LT_U: generateCompareBranch(0x0F, 0x82),
	BRANCH_LT_S: generateCompareBranch(0x0F, 0x8C),
	BRANCH_GE_U: generateCompareBranch(0x0F, 0x83),
	BRANCH_GE_S: generateCompareBranch(0x0F, 0x8D),

	// A.5.12. Instruction with Arguments of Two Registers and Two Immediates.
	LOAD_IMM_JUMP_IND: generateLoadImmJumpIndirect(),

	// A.5.13. Instructions with Arguments of Three Registers.
	ADD_32:        generateBinaryOp32(0x01), // add
	SUB_32:        generateBinaryOp32(0x29), // sub
	MUL_32:        generateMul32(),
	DIV_U_32:      generateDivUOp32(),
	DIV_S_32:      generateDivSOp32(),
	REM_U_32:      generateRemUOp32(),
	REM_S_32:      generateRemSOp32(),
	SHLO_L_32:     generateShiftOp32(0xD3, 4),
	SHLO_R_32:     generateShiftOp32(0xD3, 5),
	SHAR_R_32:     generateShiftOp32(0xD3, 7),
	ADD_64:        generateBinaryOp64(0x01), // add
	SUB_64:        generateBinaryOp64(0x29), // sub
	MUL_64:        generateMul64(),          // imul
	DIV_U_64:      generateDivUOp64(),
	DIV_S_64:      generateDivSOp64(),
	REM_U_64:      generateRemUOp64(),
	REM_S_64:      generateRemSOp64(),
	SHLO_L_64:     generateShiftOp64(0xD3, 4),
	SHLO_R_64:     generateShiftOp64(0xD3, 5),
	SHAR_R_64:     generateShiftOp64(0xD3, 7),
	AND:           generateBinaryOp64(0x21),
	XOR:           generateBinaryOp64(0x31),
	OR:            generateBinaryOp64(0x09),
	MUL_UPPER_S_S: generateMulUpperOp64("signed"),
	MUL_UPPER_U_U: generateMulUpperOp64("unsigned"),
	MUL_UPPER_S_U: generateMulUpperOp64("mixed"),
	SET_LT_U:      generateSetCondOp64(0x92),
	SET_LT_S:      generateSetCondOp64(0x9C),
	CMOV_IZ:       generateCmovOp64(0x44),
	CMOV_NZ:       generateCmovOp64(0x45),
	ROT_L_64:      generateShiftOp64(0xD3, 0),
	ROT_L_32:      generateShiftOp32(0xD3, 0),
	ROT_R_64:      generateShiftOp64(0xD3, 1),
	ROT_R_32:      generateShiftOp32(0xD3, 1),
	AND_INV:       generateAndInvOp64(),
	OR_INV:        generateOrInvOp64(),
	XNOR:          generateXnorOp64(),
	MAX:           generateCmovCmpOp64(0x4F),
	MAX_U:         generateCmovCmpOp64(0x47),
	MIN:           generateCmovCmpOp64(0x4C),
	MIN_U:         generateCmovCmpOp64(0x42),
}

const BaseRegIndex = 13

var BaseReg = regInfoList[13]

// use store the original memory address for real memory
// this register is used as base for register dump

// encodeMovImm encodes: mov rX, imm64
func encodeMovImm(regIdx int, imm uint64) ([]byte, error) {
	if regIdx < 0 || regIdx >= len(regInfoList) {
		return nil, fmt.Errorf("invalid register index: %d", regIdx)
	}
	reg := regInfoList[regIdx]
	var prefix byte = 0x48
	if reg.REXBit == 1 {
		// For r8..r15, set RE XB
		// since opcode B8+r low bits, high bit handled by REX.B
		// but we need to distinguish r8..r15
		prefix |= 0x01 // REX.B = 1
	}
	// opcode = B8 + low 3 bits
	op := byte(0xB8 + reg.RegBits)

	immBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(immBytes, imm)
	return append([]byte{prefix, op}, immBytes...), nil
}

// encodeMovRegToMem encodes: mov [rBase + offset], rSrc
func encodeMovRegToMem(srcIdx, baseIdx int, offset byte) ([]byte, error) {
	if srcIdx < 0 || srcIdx >= len(regInfoList) || baseIdx < 0 || baseIdx >= len(regInfoList) {
		return nil, fmt.Errorf("invalid register index")
	}
	src := regInfoList[srcIdx]
	base := regInfoList[baseIdx]
	// Build REX prefix: W=1, R=src.REXBit, B=base.REXBit
	rex := byte(0x48)
	if src.REXBit == 1 {
		rex |= 0x04 // REX.R
	}
	if base.REXBit == 1 {
		rex |= 0x01 // REX.B
	}
	// opcode for mov r->m : 0x89
	// ModRM: mod=01 (disp8=offset), reg=src.RegBits, rm=100 (SIB)
	modrm := byte(0x40 | (src.RegBits << 3) | 0x4)
	// SIB: scale=00, index=100 (no index), base=base.RegBits
	sib := byte(0x20 | base.RegBits) // (4<<3) | base.RegBits

	return []byte{rex, 0x89, modrm, sib, offset}, nil
}

type RecompilerVM struct {
	*VM
	x86Code     []byte
	exitCode    []byte
	realMemory  []byte
	regDumpMem  []byte  // memory region to dump registers
	regDumpAddr uintptr // base address of regDumpMem
	PageMap     any     // TODO: define PageMap type
}

const (
	PageInaccessible = 0
	PageMutable      = 1
	PageImmutable    = 2
)

const (
	PageSize   = 4096                   // 4 KiB
	TotalMem   = 4 * 1024 * 1024 * 1024 // 4 GiB
	TotalPages = TotalMem / PageSize    // 1,048,576 pages
)

func NewRecompilerVM(vm *VM) (*RecompilerVM, error) {
	// allocate real memory
	const memSize = 4 * 1024 * 1024 * 1024 // 4GB
	mem, err := syscall.Mmap(
		-1, 0, memSize,
		syscall.PROT_NONE,
		syscall.MAP_ANON|syscall.MAP_PRIVATE,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to mmap memory: %v", err)
	}
	// put the memory address to BaseReg
	// mov r12, memAddr

	// allocate memory for register dump: one uint64 per reg
	dumpSize := len(regInfoList) * 8
	dumpMem, err := syscall.Mmap(
		-1, 0, dumpSize,
		syscall.PROT_READ|syscall.PROT_WRITE|syscall.PROT_EXEC,
		syscall.MAP_ANON|syscall.MAP_PRIVATE,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to mmap regDump memory: %v", err)
	}
	regDumpAddr := uintptr(unsafe.Pointer(&dumpMem[0]))

	rvm := &RecompilerVM{
		VM:          vm,
		realMemory:  mem,
		regDumpMem:  dumpMem,
		regDumpAddr: regDumpAddr,
		x86Code:     nil,
		exitCode:    nil,
	}
	code, err := encodeMovImm(BaseRegIndex, uint64(uintptr(unsafe.Pointer(&mem[0]))))
	if err != nil {
		return nil, fmt.Errorf("failed to encode mov for BaseReg: %v", err)
	}
	rvm.x86Code = append(rvm.x86Code, code...)
	// initialize registers: mov rX, imm from vm.register
	for i := 0; i < len(vm.register); i++ {
		immVal := vm.register[i]
		code, err := encodeMovImm(i, immVal)
		if err != nil {
			return nil, err
		}
		fmt.Printf("Initialize %s = %d\n", regInfoList[i].Name, immVal)
		rvm.x86Code = append(rvm.x86Code, code...)
	}

	// build exitCode: dump registers into regDumpMem
	// first: mov r12, regDumpAddr (use r12 as base)
	code, err = encodeMovImm(BaseRegIndex, uint64(regDumpAddr))
	if err != nil {
		return nil, err
	}
	rvm.exitCode = append(rvm.exitCode, code...)

	// for each register, mov [r12 + i*8], rX
	for i := 0; i < len(regInfoList); i++ {
		off := byte(i * 8)
		dumpInstr, err := encodeMovRegToMem(i, BaseRegIndex, off)
		if err != nil {
			return nil, err
		}
		rvm.exitCode = append(rvm.exitCode, dumpInstr...)
	}

	return rvm, nil
}

func (vm *RecompilerVM) GetBasicBlocks() {
	// 1 block first
	newBlock, err := vm.compileBasicBlock(0)
	if err != nil {
		log.Error(vm.logging, "Error compiling basic block", "error", err)
		vm.ResultCode = types.PVM_PANIC
		vm.terminated = true
	}
	vm.BasicBlocks = append(vm.BasicBlocks, newBlock)
}

func (vm *RecompilerVM) Translate() error {
	for _, basic_block := range vm.BasicBlocks {
		for _, instruction := range basic_block.Instructions {
			opcode := instruction.Opcode
			fmt.Printf("Translating instruction %s (%d) at pc %d\n", opcode_str(opcode), opcode, instruction.Pc)
			if translateFunc, exists := pvmByteCodeToX86Code[opcode]; exists {
				code, err := translateFunc(instruction)
				if err != nil {
					return fmt.Errorf("error translating instruction %s at pc %d: %w", opcode_str(opcode), instruction.Pc, err)
				}
				vm.x86Code = append(vm.x86Code, code...)
			} else {
				return fmt.Errorf("unknown opcode %s at pc %d", opcode_str(opcode), instruction.Pc)
			}
		}
		// translate end point
		endPoint := basic_block.EndPoint
		if translateFunc, exists := pvmByteCodeToX86Code[endPoint.Opcode]; exists {
			code, err := translateFunc(endPoint)
			if err != nil {
				return fmt.Errorf("error translating end point instruction %s at pc %d: %w", opcode_str(endPoint.Opcode), endPoint.Pc, err)
			}
			vm.x86Code = append(vm.x86Code, code...)
		} else {
			return fmt.Errorf("unknown end point opcode %s at pc %d", opcode_str(endPoint.Opcode), endPoint.Pc)
		}
		fmt.Printf("%s\n", basic_block.String())
	}
	// append exitCode, then ret
	vm.x86Code = append(vm.x86Code, vm.exitCode...)
	vm.x86Code = append(vm.x86Code, 0xC3) // ret
	fmt.Printf("Generated x86 code:\n%s\n", Disassemble(vm.x86Code))
	return nil
}
func Disassemble(code []byte) string {
	var sb strings.Builder
	offset := 0

	for offset < len(code) {
		inst, err := x86asm.Decode(code[offset:], 64)
		length := inst.Len
		if err != nil {
			sb.WriteString(fmt.Sprintf("0x%04x: db 0x%02x\n", offset, code[offset]))
			offset++
			continue
		}

		var hexBytes []string
		for i := 0; i < length; i++ {
			hexBytes = append(hexBytes, fmt.Sprintf("%02x", code[offset+i]))
		}
		sb.WriteString(fmt.Sprintf(
			"0x%04x: %-16s %s\n",
			offset,
			strings.Join(hexBytes, " "),
			inst.String(),
		))

		offset += length
	}

	return sb.String()
}

//go:noescape
func trampoline(entry uintptr)
func (vm *RecompilerVM) ExecuteX86Code() error {
	x86code := vm.x86Code
	codeAddr, err := syscall.Mmap(
		-1, 0, len(x86code),
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_ANON|syscall.MAP_PRIVATE,
	)
	if err != nil {
		return fmt.Errorf("failed to mmap exec code: %w", err)
	}
	copy(codeAddr, x86code)
	fmt.Printf("bytecode\n%x\n", x86code)
	err = syscall.Mprotect(codeAddr, syscall.PROT_READ|syscall.PROT_EXEC)
	if err != nil {
		return fmt.Errorf("failed to mprotect exec code: %w", err)
	}

	entry := uintptr(unsafe.Pointer(&codeAddr[0]))
	fmt.Printf("Executing x86 code\n")
	trampoline(entry)

	for i := 0; i < len(vm.register); i++ {
		regValue := binary.LittleEndian.Uint64(vm.regDumpMem[i*8:])
		fmt.Printf("%s = %d\n", regInfoList[i].Name, regValue)
		vm.register[i] = regValue
	}
	return nil
}

func (vm *VM) RunRecompiler() error {
	rvm, err := NewRecompilerVM(vm)
	if err != nil {
		return fmt.Errorf("failed to create recompiler VM: %w", err)
	}
	rvm.GetBasicBlocks()
	if len(rvm.BasicBlocks) == 0 {
		return fmt.Errorf("no basic blocks found")
	}
	fmt.Printf("RecompilerVM has %d basic blocks\n", len(rvm.BasicBlocks))
	if err := rvm.Translate(); err != nil {
		return fmt.Errorf("error translating bytecode: %w", err)
	}
	// Now we have rvm.x86Code ready, we can execute it
	if err := rvm.ExecuteX86Code(); err != nil {
		return fmt.Errorf("error executing x86 code: %w", err)
	}
	return nil
}

// A.5.2. Instructions with Arguments of One Immediate. InstructionI1
func generateSyscall() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		return []byte{0x0F, 0x05}, nil
	}
}

func encodeU16(v uint16) []byte {
	buf := make([]byte, 2)
	binary.LittleEndian.PutUint16(buf, v)
	return buf
}

func encodeU32(v uint32) []byte {
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, v)
	return buf
}

func encodeU64(v uint64) []byte {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, v)
	return buf
}
