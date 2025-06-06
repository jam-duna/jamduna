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
	{"r12", 4, 1}, // Callee-saved, safe for general use

	{"r14", 6, 1},
	{"r15", 7, 1},

	{"r13", 5, 1},
}

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

func NewRecompilerVM(vm *VM) (*RecompilerVM, error) {
	// allocate real memory
	memSize := 4096
	mem, err := syscall.Mmap(
		-1, 0, memSize,
		syscall.PROT_READ|syscall.PROT_WRITE|syscall.PROT_EXEC,
		syscall.MAP_ANON|syscall.MAP_PRIVATE,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to mmap memory: %v", err)
	}

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
	code, err := encodeMovImm(13, uint64(regDumpAddr))
	if err != nil {
		return nil, err
	}
	rvm.exitCode = append(rvm.exitCode, code...)

	// for each register, mov [r12 + i*8], rX
	for i := 0; i < len(regInfoList); i++ {
		off := byte(i * 8)
		dumpInstr, err := encodeMovRegToMem(i, 13, off)
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
	MOVE_REG: generateMoveReg(),
	SBRK:     generateSyscall(),

	// A.5.9. Instructions with Arguments of Two Registers & One Immediate.
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
	STORE_IND_U8:          generateStoreIndirect(0x88, false, 1),
	STORE_IND_U16:         generateStoreIndirect(0x89, true, 2),
	STORE_IND_U32:         generateStoreIndirect(0x89, false, 4),
	STORE_IND_U64:         generateStoreIndirect(0x89, false, 8),
	LOAD_IND_U8:           generateLoadInd(0x8A, false, 1),
	LOAD_IND_I8:           generateLoadIndSignExtend(0x0F, 0xBE, false),
	LOAD_IND_U16:          generateLoadInd(0x8B, false, 2),
	LOAD_IND_I16:          generateLoadIndSignExtend(0x0F, 0xBF, false),
	LOAD_IND_U32:          generateLoadInd(0x8B, false, 4),
	LOAD_IND_I32:          generateLoadIndSignExtend(0x63, 0x00, true),
	LOAD_IND_U64:          generateLoadInd(0x8B, true, 8),
	ADD_IMM_32:            generateBinaryImm32(0x81, 0x00),
	AND_IMM:               generateImmBinaryOp32(0x81, 4),
	XOR_IMM:               generateImmBinaryOp32(0x81, 6),
	OR_IMM:                generateImmBinaryOp32(0x81, 1),
	MUL_IMM_32:            generateImmMulOp32(),
	SET_LT_U_IMM:          generateImmSetCondOp32(0x72),
	SET_LT_S_IMM:          generateImmSetCondOp32(0x7C),
	SHLO_L_IMM_32:         generateImmShiftOp32(0xC1, 4),
	SHLO_R_IMM_32:         generateImmShiftOp32(0xC1, 5),
	SHLO_L_IMM_ALT_32:     generateImmShiftOp32(0xC1, 4),
	SHLO_R_IMM_ALT_32:     generateImmShiftOp32(0xC1, 5),
	SHAR_R_IMM_ALT_32:     generateImmShiftOp32(0xC1, 7),
	ADD_IMM_64:            generateImmBinaryOp64(0x81, 0),
	MUL_IMM_64:            generateImmMulOp64(),
	SHLO_L_IMM_64:         generateImmShiftOp64(0xC1, 4),
	SHLO_R_IMM_64:         generateImmShiftOp64(0xC1, 5),
	SHAR_R_IMM_64:         generateImmShiftOp64(0xC1, 7),
	NEG_ADD_IMM_64:        generateNegAddImm64(),
	SHLO_L_IMM_ALT_64:     generateImmShiftOp64(0xC1, 4),
	SHLO_R_IMM_ALT_64:     generateImmShiftOp64(0xC1, 5),
	SHAR_R_IMM_ALT_64:     generateImmShiftOp64(0xC1, 7),
	ROT_R_64_IMM:          generateImmShiftOp64(0xC1, 1),
	ROT_R_64_IMM_ALT:      generateImmShiftOp64(0xC1, 1),
	ROT_R_32_IMM_ALT:      generateImmShiftOp32(0xC1, 1),
	ROT_R_32_IMM:          generateRotateRight32Imm(),

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
	MUL_32:        generateBinaryOp32(0x0F), // imul with 0F AF /r sequence
	DIV_U_32:      generateDivUOp32(),
	REM_U_32:      generateRemUOp32(),
	REM_S_32:      generateRemSOp32(),
	SHLO_L_32:     generateShiftOp32(0xD3, 4),
	SHLO_R_32:     generateShiftOp32(0xD3, 5),
	SHAR_R_32:     generateShiftOp32(0xD3, 7),
	ADD_64:        generateBinaryOp64(0x01), // add
	SUB_64:        generateBinaryOp64(0x29), // sub
	MUL_64:        generateBinaryOp64(0x0F), // imul
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

func generateCompareBranch(prefix byte, opcode byte) func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		if len(inst.Args) < 3 {
			return nil, fmt.Errorf("Compare branch requires two registers and a 4-byte rel32")
		}
		src := regInfoList[min(12, int(inst.Args[0]))]
		dst := regInfoList[min(12, int(inst.Args[1]))]
		offset := inst.Args[2:6]
		rex := byte(0x40)
		if src.REXBit == 1 {
			rex |= 0x04
		}
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | (src.RegBits << 3) | dst.RegBits)
		return append([]byte{rex, 0x39, modrm, prefix, opcode}, offset...), nil
	}
}

func generateLoadImmJumpIndirect() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		if len(inst.Args) < 2 {
			return nil, fmt.Errorf("LOAD_IMM_JUMP_IND requires dest reg and memory reg")
		}
		dst := regInfoList[min(12, int(inst.Args[0]))]
		src := regInfoList[min(12, int(inst.Args[1]))]
		rex := byte(0x48)
		if dst.REXBit == 1 {
			rex |= 0x04
		}
		if src.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0x00 | (dst.RegBits << 3) | src.RegBits)
		mov := []byte{rex, 0x8B, modrm}
		jmp := []byte{rex, 0xFF, byte(0xE0 | dst.RegBits)}
		return append(mov, jmp...), nil
	}
}

func generateLoadImmJump() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		if len(inst.Args) < 2 {
			return nil, fmt.Errorf("LOAD_IMM_JUMP requires reg and imm64")
		}
		r := regInfoList[min(12, int(inst.Args[0]))]
		imm := inst.Args[1:9]
		opcode := 0xB8 + r.RegBits
		rex := byte(0x48)
		if r.REXBit == 1 {
			rex |= 0x01
		}
		mov := append([]byte{rex, opcode}, imm...)
		jmp := []byte{0xFF, byte(0xE0 + r.RegBits)}
		return append(mov, jmp...), nil
	}
}

func generateSyscall() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		return []byte{0x0F, 0x05}, nil
	}
}

func generateRotateRight32Imm() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		if len(inst.Args) < 2 {
			return nil, fmt.Errorf("ROT_R_32_IMM requires dst and imm")
		}
		dst := regInfoList[min(12, int(inst.Args[0]))]
		imm := inst.Args[1]
		rex := byte(0x40)
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | (1 << 3) | dst.RegBits) // 001b for ROR
		return []byte{rex, 0xC1, modrm, imm}, nil
	}
}
func generateMoveReg() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		if len(inst.Args) < 2 {
			return nil, fmt.Errorf("MOVE_REG requires src and dst")
		}
		src := regInfoList[min(12, int(inst.Args[0]))]
		dst := regInfoList[min(12, int(inst.Args[1]))]
		rex := byte(0x48)
		if src.REXBit == 1 {
			rex |= 0x04
		}
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | (src.RegBits << 3) | dst.RegBits)
		return []byte{rex, 0x89, modrm}, nil
	}
}

// ----
func generateCmovOp64(opcode byte) func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		args := inst.Args
		if len(args) != 2 {
			return nil, fmt.Errorf("CMOV requires 2 args, got %d", len(args))
		}

		src := min(12, int(args[0]&0x0F))
		dst := min(12, int(args[1]))

		srcReg := regInfoList[src]
		dstReg := regInfoList[dst]

		rex := byte(0x48) // REX.W for 64-bit
		if srcReg.REXBit == 1 {
			rex |= 0x04 // REX.R
		}
		if dstReg.REXBit == 1 {
			rex |= 0x01 // REX.B
		}

		modrm := byte(0xC0 | (srcReg.RegBits << 3) | dstReg.RegBits)
		code := []byte{rex, 0x0F, opcode, modrm}
		return code, nil
	}
}

func generateStoreIndirect(opcode byte, is16bit bool, size int) func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		if len(inst.Args) < 2 {
			return nil, fmt.Errorf("STORE_IND requires src and base register")
		}
		src := regInfoList[min(12, int(inst.Args[0]))]
		base := regInfoList[min(12, int(inst.Args[1]))]

		rex := byte(0x40)
		if size == 8 {
			rex |= 0x08 // REX.W
		}
		if src.REXBit == 1 {
			rex |= 0x04 // REX.R
		}
		if base.REXBit == 1 {
			rex |= 0x01 // REX.B
		}

		code := []byte{}
		if is16bit {
			code = append(code, 0x66)
		}
		modrm := byte(0x80 | (src.RegBits << 3) | base.RegBits) // mod = 10, disp = 0
		code = append(code, rex, opcode, modrm, 0x00)           // disp = 0
		return code, nil
	}
}

func generateLoadInd(opcode byte, is64bit bool, size int) func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		if len(inst.Args) < 2 {
			return nil, fmt.Errorf("LOAD_IND requires dest and base register")
		}
		dst := regInfoList[min(12, int(inst.Args[0]))]
		base := regInfoList[min(12, int(inst.Args[1]))]

		rex := byte(0x40)
		if is64bit {
			rex |= 0x08 // REX.W
		}
		if dst.REXBit == 1 {
			rex |= 0x04 // REX.R
		}
		if base.REXBit == 1 {
			rex |= 0x01 // REX.B
		}

		modrm := byte(0x80 | (dst.RegBits << 3) | base.RegBits) // mod = 10
		return []byte{rex, opcode, modrm, 0x00}, nil            // disp = 0
	}
}

func generateLoadIndSignExtend(prefix byte, opcode byte, is64bit bool) func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		if len(inst.Args) < 2 {
			return nil, fmt.Errorf("LOAD_IND_SIGN_EXTEND requires dest and base")
		}
		dst := regInfoList[min(12, int(inst.Args[0]))]
		base := regInfoList[min(12, int(inst.Args[1]))]

		rex := byte(0x40)
		if is64bit {
			rex |= 0x08 // REX.W
		}
		if dst.REXBit == 1 {
			rex |= 0x04 // REX.R
		}
		if base.REXBit == 1 {
			rex |= 0x01 // REX.B
		}

		modrm := byte(0x80 | (dst.RegBits << 3) | base.RegBits) // mod = 10
		return []byte{prefix, rex, opcode, modrm, 0x00}, nil    // disp = 0
	}
}

func generateBinaryImm32(opcode byte, ext byte) func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		if len(inst.Args) < 2 {
			return nil, fmt.Errorf("BINARY_IMM32 requires dst and imm32")
		}
		dst := regInfoList[min(12, int(inst.Args[0]))]
		imm := inst.Args[1:5]
		rex := byte(0x40)
		if dst.REXBit == 1 {
			rex |= 0x01 // REX.B
		}
		modrm := byte(0xC0 | (ext << 3) | dst.RegBits)
		return append([]byte{rex, opcode, modrm}, imm...), nil
	}
}

func generateLoadImm64() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		if len(inst.Args) < 2 {
			return nil, fmt.Errorf("LOAD_IMM_64 requires dest register and imm64")
		}
		dst := min(12, int(inst.Args[0]))
		imm := binary.LittleEndian.Uint64(inst.Args[1:9])
		opcode := 0xB8 + regInfoList[dst].RegBits
		rex := byte(0x48)
		if regInfoList[dst].REXBit == 1 {
			rex |= 0x01
		}
		code := []byte{rex, opcode}
		code = append(code, encodeU64(imm)...) // encodeU64: returns imm64 little endian
		return code, nil
	}
}

func generateStoreU8() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		if len(inst.Args) < 2 {
			return nil, fmt.Errorf("STORE_U8 requires src and dst")
		}
		src := regInfoList[min(12, int(inst.Args[0]))]
		dst := regInfoList[min(12, int(inst.Args[1]))]
		rex := byte(0x40)
		if src.REXBit == 1 {
			rex |= 0x04
		}
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | (src.RegBits << 3) | dst.RegBits)
		return []byte{rex, 0x88, modrm}, nil
	}
}

func generateStoreImmU8() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		if len(inst.Args) < 2 {
			return nil, fmt.Errorf("STORE_IMM_U8 requires memory register and imm8")
		}
		rm := regInfoList[min(12, int(inst.Args[0]))]
		imm := inst.Args[1]
		rex := byte(0x40)
		if rm.REXBit == 1 {
			rex |= 0x01 // REX.B
		}
		modrm := byte(0xC0 | (0x00 << 3) | rm.RegBits)
		return []byte{rex, 0xC6, modrm, imm}, nil
	}
}

func generateStoreImmU16() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		if len(inst.Args) < 6 {
			return nil, fmt.Errorf("STORE_IMM_U16 requires 2-byte imm and 4-byte addr")
		}
		addr := binary.LittleEndian.Uint32(inst.Args[0:4])
		imm := binary.LittleEndian.Uint16(inst.Args[4:6])
		code := []byte{0x66, 0xC7, 0x04, 0x25}
		code = append(code, encodeU32(addr)...) // mov [abs32], imm16
		code = append(code, encodeU16(imm)...)
		return code, nil
	}
}

func generateStoreImmU32() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		if len(inst.Args) < 8 {
			return nil, fmt.Errorf("STORE_IMM_U32 requires 4-byte imm and 4-byte addr")
		}
		addr := binary.LittleEndian.Uint32(inst.Args[0:4])
		imm := binary.LittleEndian.Uint32(inst.Args[4:8])
		code := []byte{0xC7, 0x04, 0x25}
		code = append(code, encodeU32(addr)...) // mov [abs32], imm32
		code = append(code, encodeU32(imm)...)
		return code, nil
	}
}

func generateStoreImmU64() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		if len(inst.Args) < 12 {
			return nil, fmt.Errorf("STORE_IMM_U64 requires 8-byte imm and 4-byte addr")
		}
		addr := binary.LittleEndian.Uint32(inst.Args[0:4])
		imm := binary.LittleEndian.Uint64(inst.Args[4:12])
		// mov rax, imm64; mov [addr], rax
		code := []byte{0x48, 0xB8}
		code = append(code, encodeU64(imm)...)  // mov rax, imm64
		code = append(code, 0x48, 0xA3)         // mov [abs32], rax
		code = append(code, encodeU32(addr)...) // [abs32]
		return code, nil
	}
}

func generateLoadImm32() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		if len(inst.Args) < 5 {
			return nil, fmt.Errorf("LOAD_IMM requires 1-byte dest register and 4-byte imm32")
		}
		dst := regInfoList[min(12, int(inst.Args[0]))]
		imm := binary.LittleEndian.Uint32(inst.Args[1:5])
		rex := byte(0x40)
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		opcode := 0xB8 + dst.RegBits
		code := []byte{rex, opcode}
		code = append(code, encodeU32(imm)...)
		return code, nil
	}
}

func generateLoadU8() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		if len(inst.Args) < 2 {
			return nil, fmt.Errorf("LOAD_U8 requires src and dst")
		}
		src := regInfoList[min(12, int(inst.Args[0]))]
		dst := regInfoList[min(12, int(inst.Args[1]))]
		rex := byte(0x40)
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | (dst.RegBits << 3) | src.RegBits)
		return []byte{rex, 0x0F, 0xB6, modrm}, nil // movzx dst, byte ptr [src]
	}
}

func generateLoadI8() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		if len(inst.Args) < 2 {
			return nil, fmt.Errorf("LOAD_I8 requires src and dst")
		}
		src := regInfoList[min(12, int(inst.Args[0]))]
		dst := regInfoList[min(12, int(inst.Args[1]))]
		rex := byte(0x40)
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | (dst.RegBits << 3) | src.RegBits)
		return []byte{rex, 0x0F, 0xBE, modrm}, nil // movsx dst, byte ptr [src]
	}
}

func generateLoadU16() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		if len(inst.Args) < 2 {
			return nil, fmt.Errorf("LOAD_U16 requires src and dst")
		}
		src := regInfoList[min(12, int(inst.Args[0]))]
		dst := regInfoList[min(12, int(inst.Args[1]))]
		rex := byte(0x40)
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | (dst.RegBits << 3) | src.RegBits)
		return []byte{rex, 0x0F, 0xB7, modrm}, nil // movzx dst, word ptr [src]
	}
}

func generateLoadI16() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		if len(inst.Args) < 2 {
			return nil, fmt.Errorf("LOAD_I16 requires src and dst")
		}
		src := regInfoList[min(12, int(inst.Args[0]))]
		dst := regInfoList[min(12, int(inst.Args[1]))]
		rex := byte(0x40)
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | (dst.RegBits << 3) | src.RegBits)
		return []byte{rex, 0x0F, 0xBF, modrm}, nil // movsx dst, word ptr [src]
	}
}

func generateLoadU32() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		if len(inst.Args) < 2 {
			return nil, fmt.Errorf("LOAD_U32 requires src and dst")
		}
		src := regInfoList[min(12, int(inst.Args[0]))]
		dst := regInfoList[min(12, int(inst.Args[1]))]
		rex := byte(0x40)
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | (dst.RegBits << 3) | src.RegBits)
		return []byte{rex, 0x8B, modrm}, nil // mov dst, dword ptr [src]
	}
}

func generateLoadI32() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		if len(inst.Args) < 2 {
			return nil, fmt.Errorf("LOAD_I32 requires src and dst")
		}
		src := regInfoList[min(12, int(inst.Args[0]))]
		dst := regInfoList[min(12, int(inst.Args[1]))]
		rex := byte(0x48)
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | (dst.RegBits << 3) | src.RegBits)
		return []byte{rex, 0x63, modrm}, nil // movsxd dst, dword ptr [src]
	}
}

func generateLoadU64() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		if len(inst.Args) < 2 {
			return nil, fmt.Errorf("LOAD_U64 requires src and dst")
		}
		src := regInfoList[min(12, int(inst.Args[0]))]
		dst := regInfoList[min(12, int(inst.Args[1]))]
		rex := byte(0x48)
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | (dst.RegBits << 3) | src.RegBits)
		return []byte{rex, 0x8B, modrm}, nil // mov dst, qword ptr [src]
	}
}

func generateStoreU16() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		if len(inst.Args) < 2 {
			return nil, fmt.Errorf("STORE_U16 requires src and dst")
		}
		src := regInfoList[min(12, int(inst.Args[0]))]
		dst := regInfoList[min(12, int(inst.Args[1]))]
		rex := byte(0x40)
		if src.REXBit == 1 {
			rex |= 0x04
		}
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | (src.RegBits << 3) | dst.RegBits)
		return []byte{0x66, rex, 0x89, modrm}, nil // mov word ptr [dst], src
	}
}

func generateStoreU32() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		if len(inst.Args) < 2 {
			return nil, fmt.Errorf("STORE_U32 requires src and dst")
		}
		src := regInfoList[min(12, int(inst.Args[0]))]
		dst := regInfoList[min(12, int(inst.Args[1]))]
		rex := byte(0x40)
		if src.REXBit == 1 {
			rex |= 0x04
		}
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | (src.RegBits << 3) | dst.RegBits)
		return []byte{rex, 0x89, modrm}, nil // mov dword ptr [dst], src
	}
}

func generateStoreU64() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		if len(inst.Args) < 2 {
			return nil, fmt.Errorf("STORE_U64 requires src and dst")
		}
		src := regInfoList[min(12, int(inst.Args[0]))]
		dst := regInfoList[min(12, int(inst.Args[1]))]
		rex := byte(0x48)
		if src.REXBit == 1 {
			rex |= 0x04
		}
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | (src.RegBits << 3) | dst.RegBits)
		return []byte{rex, 0x89, modrm}, nil // mov qword ptr [dst], src
	}
}

func generateStoreImmIndU8() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		if len(inst.Args) < 2 {
			return nil, fmt.Errorf("STORE_IMM_IND_U8 requires dst and imm")
		}
		dst := regInfoList[min(12, int(inst.Args[0]))]
		imm := inst.Args[1]
		rex := byte(0x40)
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | (0x00 << 3) | dst.RegBits) // /0 encoding
		return []byte{rex, 0xC6, modrm, imm}, nil       // mov byte ptr [dst], imm8
	}
}

func generateStoreImmIndU16() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		if len(inst.Args) < 3 {
			return nil, fmt.Errorf("STORE_IMM_IND_U16 requires dst and imm16")
		}
		dst := regInfoList[min(12, int(inst.Args[0]))]
		imm := binary.LittleEndian.Uint16(inst.Args[1:3])
		rex := byte(0x40)
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | (0x00 << 3) | dst.RegBits)                       // /0 encoding
		return append([]byte{0x66, rex, 0xC7, modrm}, encodeU16(imm)...), nil // mov word ptr [dst], imm16
	}
}

func generateStoreImmIndU32() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		if len(inst.Args) < 5 {
			return nil, fmt.Errorf("STORE_IMM_IND_U32 requires dst and imm32")
		}
		dst := regInfoList[min(12, int(inst.Args[0]))]
		imm := binary.LittleEndian.Uint32(inst.Args[1:5])
		rex := byte(0x40)
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | (0x00 << 3) | dst.RegBits)
		return append([]byte{rex, 0xC7, modrm}, encodeU32(imm)...), nil // mov dword ptr [dst], imm32
	}
}

func generateStoreImmIndU64() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		if len(inst.Args) < 9 {
			return nil, fmt.Errorf("STORE_IMM_IND_U64 requires dst and imm64")
		}
		dst := regInfoList[min(12, int(inst.Args[0]))]
		imm := binary.LittleEndian.Uint64(inst.Args[1:9])
		rex := byte(0x48)
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | (0x00 << 3) | dst.RegBits)
		return append([]byte{rex, 0xC7, modrm}, encodeU64(imm)...), nil // mov qword ptr [dst], imm64
	}
}

func generateBranchImm(opcode byte) func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		if len(inst.Args) < 1 {
			return nil, fmt.Errorf("BRANCH_IMM requires 1-byte relative offset")
		}
		offset := inst.Args[0]
		return []byte{opcode, offset}, nil
	}
}

func generateJumpRel32() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		if len(inst.Args) < 4 {
			return nil, fmt.Errorf("JUMP requires 4-byte rel offset")
		}
		offset := binary.LittleEndian.Uint32(inst.Args[:4])
		return append([]byte{0xE9}, encodeU32(offset)...), nil
	}
}

func generateJumpIndirect() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		if len(inst.Args) < 1 {
			return nil, fmt.Errorf("JUMP_IND requires 1 register")
		}
		reg := regInfoList[min(12, int(inst.Args[0]))]
		rex := byte(0x48)
		if reg.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xE0 | reg.RegBits) // FF /4 with mod = 11
		return []byte{rex, 0xFF, modrm}, nil
	}
}

func generateShiftOp64(opcode byte, regField byte) func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		args := inst.Args
		if len(args) != 1 {
			return nil, fmt.Errorf("shift op requires 1 arg (target), got %d", len(args))
		}
		reg := min(12, int(args[0]))
		regInfo := regInfoList[reg]

		rex := byte(0x48)
		if regInfo.REXBit == 1 {
			rex |= 0x01
		}

		modrm := byte(0xC0 | (regField << 3) | regInfo.RegBits)
		return []byte{rex, opcode, modrm}, nil
	}
}

func generateRemUOp64() func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		code, err := generateDivUOp64()(inst)
		if err != nil {
			return nil, err
		}
		return code, nil
	}
}

func generateRemSOp64() func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		code, err := generateDivSOp64()(inst)
		if err != nil {
			return nil, err
		}
		return code, nil
	}
}

func generateBitCount64() func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		// popcnt r64, r/m64 → F3 48 0F B8 /r
		args := inst.Args
		reg := min(12, int(args[0]))
		regInfo := regInfoList[reg]
		rex := byte(0x48)
		if regInfo.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | (regInfo.RegBits << 3) | regInfo.RegBits)
		return []byte{0xF3, rex, 0x0F, 0xB8, modrm}, nil
	}
}

func generateBitCount32() func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		// popcnt r32, r/m32 → F3 0F B8 /r
		args := inst.Args
		reg := min(12, int(args[0]))
		regInfo := regInfoList[reg]
		modrm := byte(0xC0 | (regInfo.RegBits << 3) | regInfo.RegBits)
		return []byte{0xF3, 0x0F, 0xB8, modrm}, nil
	}
}

func generateLeadingZeros64() func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		// lzcnt r64, r/m64 → F3 48 0F BD /r
		args := inst.Args
		reg := min(12, int(args[0]))
		regInfo := regInfoList[reg]
		rex := byte(0x48)
		if regInfo.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | (regInfo.RegBits << 3) | regInfo.RegBits)
		return []byte{0xF3, rex, 0x0F, 0xBD, modrm}, nil
	}
}

func generateLeadingZeros32() func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		// lzcnt r32, r/m32 → F3 0F BD /r
		args := inst.Args
		reg := min(12, int(args[0]))
		regInfo := regInfoList[reg]
		modrm := byte(0xC0 | (regInfo.RegBits << 3) | regInfo.RegBits)
		return []byte{0xF3, 0x0F, 0xBD, modrm}, nil
	}
}

func generateTrailingZeros64() func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		// tzcnt r64, r/m64 → F3 48 0F BC /r
		args := inst.Args
		reg := min(12, int(args[0]))
		regInfo := regInfoList[reg]
		rex := byte(0x48)
		if regInfo.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | (regInfo.RegBits << 3) | regInfo.RegBits)
		return []byte{0xF3, rex, 0x0F, 0xBC, modrm}, nil
	}
}

func generateTrailingZeros32() func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		// tzcnt r32, r/m32 → F3 0F BC /r
		args := inst.Args
		reg := min(12, int(args[0]))
		regInfo := regInfoList[reg]
		modrm := byte(0xC0 | (regInfo.RegBits << 3) | regInfo.RegBits)
		return []byte{0xF3, 0x0F, 0xBC, modrm}, nil
	}
}

func generateSignExtend8() func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		// movsx r64, r/m8 → REX.W 0F BE /r
		args := inst.Args
		reg := min(12, int(args[0]))
		regInfo := regInfoList[reg]
		modrm := byte(0xC0 | (regInfo.RegBits << 3) | regInfo.RegBits)
		return []byte{0x48, 0x0F, 0xBE, modrm}, nil
	}
}

func generateSignExtend16() func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		// movsx r64, r/m16 → REX.W 0F BF /r
		args := inst.Args
		reg := min(12, int(args[0]))
		regInfo := regInfoList[reg]
		modrm := byte(0xC0 | (regInfo.RegBits << 3) | regInfo.RegBits)
		return []byte{0x48, 0x0F, 0xBF, modrm}, nil
	}
}

func generateZeroExtend16() func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		// movzx r64, r/m16 → REX.W 0F B7 /r
		args := inst.Args
		reg := min(12, int(args[0]))
		regInfo := regInfoList[reg]
		modrm := byte(0xC0 | (regInfo.RegBits << 3) | regInfo.RegBits)
		return []byte{0x48, 0x0F, 0xB7, modrm}, nil
	}
}

func generateReverseBytes64() func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		// bswap r64 → REX.W 0F C8+rd
		args := inst.Args
		reg := min(12, int(args[0]))
		regInfo := regInfoList[reg]
		opcode := 0xC8 + regInfo.RegBits
		rex := byte(0x48)
		if regInfo.REXBit == 1 {
			rex |= 0x01
		}
		return []byte{rex, 0x0F, opcode}, nil
	}
}

func generateBinaryOp64(opcode byte) func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		args := inst.Args
		if len(args) != 2 {
			return nil, fmt.Errorf("binary op requires 2 args, got %d", len(args))
		}

		reg1 := min(12, int(args[0]&0x0F))
		reg2 := min(12, int(args[0]>>4))
		dst := min(12, int(args[1]))

		dstReg := regInfoList[dst]
		src1Reg := regInfoList[reg1]
		src2Reg := regInfoList[reg2]

		var code []byte

		// mov dst, src1
		rex1 := byte(0x48)
		if src1Reg.REXBit == 1 {
			rex1 |= 0x04 // REX.R
		}
		if dstReg.REXBit == 1 {
			rex1 |= 0x01 // REX.B
		}
		modrm1 := byte(0xC0 | (src1Reg.RegBits << 3) | dstReg.RegBits)
		code = append(code, rex1, 0x89, modrm1)

		// opcode dst, src2
		rex2 := byte(0x48)
		if src2Reg.REXBit == 1 {
			rex2 |= 0x04
		}
		if dstReg.REXBit == 1 {
			rex2 |= 0x01
		}
		modrm2 := byte(0xC0 | (src2Reg.RegBits << 3) | dstReg.RegBits)
		if opcode == 0x0F {
			code = append(code, rex2, 0x0F, 0xAF, modrm2)
		} else {
			code = append(code, rex2, opcode, modrm2)
		}

		return code, nil
	}
}

func generateBinaryOp32(opcode byte) func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		args := inst.Args
		if len(args) != 2 {
			return nil, fmt.Errorf("binary op requires 2 args, got %d", len(args))
		}

		reg1 := min(12, int(args[0]&0x0F))
		reg2 := min(12, int(args[0]>>4))
		dst := min(12, int(args[1]))

		dstReg := regInfoList[dst]
		src1Reg := regInfoList[reg1]
		src2Reg := regInfoList[reg2]

		var code []byte

		// mov dst, src1
		rex1 := byte(0x40)
		if src1Reg.REXBit == 1 {
			rex1 |= 0x04
		}
		if dstReg.REXBit == 1 {
			rex1 |= 0x01
		}
		modrm1 := byte(0xC0 | (src1Reg.RegBits << 3) | dstReg.RegBits)
		code = append(code, rex1, 0x89, modrm1)

		// opcode dst, src2
		rex2 := byte(0x40)
		if src2Reg.REXBit == 1 {
			rex2 |= 0x04
		}
		if dstReg.REXBit == 1 {
			rex2 |= 0x01
		}
		modrm2 := byte(0xC0 | (src2Reg.RegBits << 3) | dstReg.RegBits)
		if opcode == 0x0F {
			code = append(code, rex2, 0x0F, 0xAF, modrm2) // imul
		} else {
			code = append(code, rex2, opcode, modrm2)
		}

		return code, nil
	}
}

func generateDivUOp32() func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		args := inst.Args
		if len(args) != 1 {
			return nil, fmt.Errorf("DIV_U_32 requires 1 arg (divisor), got %d", len(args))
		}
		reg := min(12, int(args[0]))
		divisor := regInfoList[reg]

		rex := byte(0x40)
		if divisor.REXBit == 1 {
			rex |= 0x01 // REX.B
		}

		modrm := byte(0xC0 | (6 << 3) | divisor.RegBits) // /6 for div
		return []byte{0x31, 0xD2, rex, 0xF7, modrm}, nil
	}
}

func generateRemUOp32() func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		code, err := generateDivUOp32()(inst)
		if err != nil {
			return nil, err
		}
		// result is in EDX
		return code, nil
	}
}

func generateRemSOp32() func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		args := inst.Args
		if len(args) != 1 {
			return nil, fmt.Errorf("REM_S_32 requires 1 arg (divisor), got %d", len(args))
		}
		reg := min(12, int(args[0]))
		divisor := regInfoList[reg]

		rex := byte(0x40)
		if divisor.REXBit == 1 {
			rex |= 0x01
		}

		modrm := byte(0xC0 | (7 << 3) | divisor.RegBits) // /7 for idiv
		return []byte{0x99, rex, 0xF7, modrm}, nil
	}
}

func generateDivUOp64() func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		args := inst.Args
		if len(args) != 1 {
			return nil, fmt.Errorf("DIV_U_64 requires 1 arg (divisor), got %d", len(args))
		}
		reg := min(12, int(args[0]))
		divisor := regInfoList[reg]

		rex := byte(0x48)
		if divisor.REXBit == 1 {
			rex |= 0x01 // REX.B
		}

		modrm := byte(0xC0 | (6 << 3) | divisor.RegBits)
		return []byte{0x48, 0x31, 0xD2, rex, 0xF7, modrm}, nil
	}
}

func generateDivSOp64() func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		args := inst.Args
		if len(args) != 1 {
			return nil, fmt.Errorf("DIV_S_64 requires 1 arg (divisor), got %d", len(args))
		}
		reg := min(12, int(args[0]))
		divisor := regInfoList[reg]

		rex := byte(0x48)
		if divisor.REXBit == 1 {
			rex |= 0x01
		}

		modrm := byte(0xC0 | (7 << 3) | divisor.RegBits)
		return []byte{0x48, 0x99, rex, 0xF7, modrm}, nil
	}
}

func generateShiftOp32(opcode byte, regField byte) func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		args := inst.Args
		if len(args) != 1 {
			return nil, fmt.Errorf("shift op requires 1 arg (target), got %d", len(args))
		}
		reg := min(12, int(args[0]))
		regInfo := regInfoList[reg]

		rex := byte(0x40)
		if regInfo.REXBit == 1 {
			rex |= 0x01
		}

		modrm := byte(0xC0 | (regField << 3) | regInfo.RegBits)
		return []byte{rex, opcode, modrm}, nil
	}
}

func generateImmBinaryOp32(opcode byte, subcode byte) func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		args := inst.Args
		if len(args) < 2 {
			return nil, fmt.Errorf("binary imm op32 requires reg + imm, got %d", len(args))
		}
		dst := regInfoList[min(12, int(args[0]))]
		imm := args[1]
		rex := byte(0x40)
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | (subcode << 3) | dst.RegBits)
		return []byte{rex, opcode, modrm, imm}, nil
	}
}

func generateImmMulOp32() func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		args := inst.Args
		if len(args) < 2 {
			return nil, fmt.Errorf("mul imm32 requires reg + imm, got %d", len(args))
		}
		dst := regInfoList[min(12, int(args[0]))]
		rex := byte(0x40)
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | (dst.RegBits << 3) | dst.RegBits)
		imm := args[1]
		return []byte{rex, 0x69, modrm, imm, 0x00, 0x00, 0x00}, nil // imul dst, dst, imm32
	}
}

func generateImmSetCondOp32(setcc byte) func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		args := inst.Args
		if len(args) < 2 {
			return nil, fmt.Errorf("setcc imm32 requires reg + imm, got %d", len(args))
		}
		tmp := regInfoList[0] // temp reg
		dst := regInfoList[min(12, int(args[0]))]
		imm := args[1]
		cmp := []byte{0xB8 + tmp.RegBits, imm, 0x00, 0x00, 0x00}      // mov tmp, imm
		cmpr := []byte{0x39, 0xC0 | (tmp.RegBits << 3) | dst.RegBits} // cmp dst, tmp
		setccInst := []byte{0x0F, setcc, 0xC0 | dst.RegBits}          // setcc dst
		return append(append(cmp, cmpr...), setccInst...), nil
	}
}

func generateImmShiftOp32(opcode byte, subcode byte) func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		args := inst.Args
		if len(args) < 2 {
			return nil, fmt.Errorf("shift imm32 requires reg + imm, got %d", len(args))
		}
		dst := regInfoList[min(12, int(args[0]))]
		imm := args[1]
		rex := byte(0x40)
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | (subcode << 3) | dst.RegBits)
		return []byte{rex, opcode, modrm, imm}, nil
	}
}

func generateImmBinaryOp64(opcode byte, subcode byte) func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		args := inst.Args
		if len(args) < 2 {
			return nil, fmt.Errorf("binary imm op64 requires reg + imm, got %d", len(args))
		}
		dst := regInfoList[min(12, int(args[0]))]
		rex := byte(0x48)
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | (subcode << 3) | dst.RegBits)
		imm := args[1]
		return []byte{rex, opcode, modrm, imm}, nil
	}
}

func generateImmMulOp64() func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		args := inst.Args
		if len(args) < 2 {
			return nil, fmt.Errorf("mul imm64 requires reg + imm, got %d", len(args))
		}
		dst := regInfoList[min(12, int(args[0]))]
		rex := byte(0x48)
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | (dst.RegBits << 3) | dst.RegBits)
		imm := args[1]
		return []byte{rex, 0x69, modrm, imm, 0x00, 0x00, 0x00}, nil // imul dst, dst, imm32
	}
}

func generateImmShiftOp64(opcode byte, subcode byte) func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		args := inst.Args
		if len(args) < 2 {
			return nil, fmt.Errorf("shift imm64 requires reg + imm, got %d", len(args))
		}
		dst := regInfoList[min(12, int(args[0]))]
		imm := args[1]
		rex := byte(0x48)
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | (subcode << 3) | dst.RegBits)
		return []byte{rex, opcode, modrm, imm}, nil
	}
}

func generateNegAddImm64() func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		args := inst.Args
		if len(args) < 2 {
			return nil, fmt.Errorf("neg_add_imm64 requires reg + imm, got %d", len(args))
		}
		dst := regInfoList[min(12, int(args[0]))]
		imm := args[1]
		rex := byte(0x48)
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		mov := []byte{rex, 0xC7, 0xC0 | dst.RegBits, imm, 0x00, 0x00, 0x00} // mov dst, imm
		not := []byte{rex, 0xF7, 0xD0 | dst.RegBits}                        // not dst
		add := []byte{rex, 0x83, 0xC0 | dst.RegBits, 1}                     // add dst, 1
		return append(append(mov, not...), add...), nil
	}
}

func generateMulUpperOp64(mode string) func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		args := inst.Args
		if len(args) != 2 {
			return nil, fmt.Errorf("MUL_UPPER_64 requires 2 args, got %d", len(args))
		}

		src1 := regInfoList[min(12, int(args[0]&0x0F))]
		src2 := regInfoList[min(12, int(args[0]>>4))]
		dst := regInfoList[min(12, int(args[1]))]

		var code []byte

		// mov rax, src1
		rex := byte(0x48 | (src1.REXBit << 2))
		modrm := byte(0xC0 | (src1.RegBits << 3) | 0)
		code = append(code, rex, 0x89, modrm) // mov rax, src1

		// clear rdx
		code = append(code, 0x48, 0x31, 0xD2) // xor rdx, rdx

		// imul/mul src2
		rex2 := byte(0x48 | (src2.REXBit << 2))
		modrm2 := byte(0xC0 | (0x04 << 3) | src2.RegBits) // /4 or /5 for mul/imul
		switch mode {
		case "signed":
			modrm2 = 0xC0 | (0x05 << 3) | src2.RegBits // /5 = imul
			code = append(code, rex2, 0xF7, modrm2)
		case "unsigned":
			modrm2 = 0xC0 | (0x04 << 3) | src2.RegBits // /4 = mul
			code = append(code, rex2, 0xF7, modrm2)
		case "mixed":
			return nil, fmt.Errorf("mixed signed/unsigned multiply not implemented")
		}

		// mov dst, rdx
		rex3 := byte(0x48 | (dst.REXBit << 2))
		modrm3 := byte(0xC0 | (2 << 3) | dst.RegBits)
		code = append(code, rex3, 0x89, modrm3)

		return code, nil
	}
}

func generateSetCondOp64(cc byte) func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		args := inst.Args
		if len(args) != 2 {
			return nil, fmt.Errorf("SET_COND requires 2 args")
		}
		src1 := regInfoList[min(12, int(args[0]&0x0F))]
		src2 := regInfoList[min(12, int(args[0]>>4))]
		dst := regInfoList[min(12, int(args[1]))]

		rex := byte(0x48 | (src1.REXBit << 2) | (src2.REXBit << 1))
		modrm := byte(0xC0 | (src2.RegBits << 3) | src1.RegBits)
		code := []byte{rex, 0x39, modrm} // cmp src1, src2

		rex2 := byte(0x40 | (dst.REXBit << 0))
		modrm2 := byte(0xC0 | (0 << 3) | dst.RegBits)
		code = append(code, rex2, 0x0F, cc, modrm2)

		return code, nil
	}
}

func generateAndInvOp64() func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		args := inst.Args
		if len(args) != 3 {
			return nil, fmt.Errorf("AND_INV requires 3 args")
		}
		src1 := regInfoList[min(12, int(args[0]))]
		src2 := regInfoList[min(12, int(args[1]))]
		dst := regInfoList[min(12, int(args[2]))]

		code := []byte{
			// mov dst, src1
			0x48 | (src1.REXBit<<2 | dst.REXBit), 0x89, 0xC0 | (src1.RegBits << 3) | dst.RegBits,
			// not dst
			0x48 | (dst.REXBit << 0), 0xF7, 0xD0 | dst.RegBits,
			// and dst, src2
			0x48 | (dst.REXBit | src2.REXBit<<2), 0x21, 0xC0 | (src2.RegBits << 3) | dst.RegBits,
		}
		return code, nil
	}
}

func generateOrInvOp64() func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		args := inst.Args
		if len(args) != 3 {
			return nil, fmt.Errorf("OR_INV requires 3 args")
		}
		src1 := regInfoList[min(12, int(args[0]))]
		src2 := regInfoList[min(12, int(args[1]))]
		dst := regInfoList[min(12, int(args[2]))]

		code := []byte{
			0x48 | (src1.REXBit<<2 | dst.REXBit), 0x89, 0xC0 | (src1.RegBits << 3) | dst.RegBits, // mov dst, src1
			0x48 | (dst.REXBit << 0), 0xF7, 0xD0 | dst.RegBits, // not dst
			0x48 | (dst.REXBit | src2.REXBit<<2), 0x09, 0xC0 | (src2.RegBits << 3) | dst.RegBits, // or dst, src2
		}
		return code, nil
	}
}

func generateXnorOp64() func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		args := inst.Args
		if len(args) != 3 {
			return nil, fmt.Errorf("XNOR requires 3 args")
		}
		src1 := regInfoList[min(12, int(args[0]))]
		src2 := regInfoList[min(12, int(args[1]))]
		dst := regInfoList[min(12, int(args[2]))]

		code := []byte{
			0x48 | (src1.REXBit<<2 | dst.REXBit), 0x89, 0xC0 | (src1.RegBits << 3) | dst.RegBits, // mov dst, src1
			0x48 | (dst.REXBit | src2.REXBit<<2), 0x31, 0xC0 | (src2.RegBits << 3) | dst.RegBits, // xor dst, src2
			0x48 | (dst.REXBit << 0), 0xF7, 0xD0 | dst.RegBits, // not dst
		}
		return code, nil
	}
}

func generateCmovCmpOp64(cc byte) func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		args := inst.Args
		if len(args) != 3 {
			return nil, fmt.Errorf("CMOV_CMP requires 3 args")
		}
		src1 := regInfoList[min(12, int(args[0]))]
		src2 := regInfoList[min(12, int(args[1]))]
		dst := regInfoList[min(12, int(args[2]))]

		code := []byte{
			0x48 | (src1.REXBit<<2 | src2.REXBit), 0x39, 0xC0 | (src2.RegBits << 3) | src1.RegBits, // cmp src1, src2
			0x48 | (src1.REXBit<<2 | dst.REXBit), 0x8B, 0xC0 | (src1.RegBits << 3) | dst.RegBits, // mov dst, src1
			0x48 | (src2.REXBit<<2 | dst.REXBit), 0x0F, cc, 0xC0 | (src2.RegBits << 3) | dst.RegBits, // cmovcc dst, src2
		}
		return code, nil
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
