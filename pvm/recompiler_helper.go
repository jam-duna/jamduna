package pvm

import (
	"encoding/binary"
	"unsafe"
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
	{"r12", 4, 1},
}

var pvmByteCodeToX86Code = map[byte]func(Instruction) []byte{
	// A.5.1. Instructions without Arguments
	TRAP:        generateTrap,
	FALLTHROUGH: generateFallthrough,

	// JumpType = DIRECT_JUMP
	JUMP:          generateJump,        // jump is managed by the VM using the TruePC
	LOAD_IMM_JUMP: generateLoadImmJump, // does a LoadImm in x86 side, but the jump is managed by the VM

	// JumpType = INDIRECT_JUMP (all of these have some register visible upon return which is added to some JumpIndirectOffset)
	JUMP_IND:          generateFallthrough,         // jump is managed by the VM using the JumpSourceRegister
	LOAD_IMM_JUMP_IND: generateLoadImmJumpIndirect, // does a LoadImm in x86 side, but the jump is managed by the VM

	// JumpType = CONDITIONAL (x86code sets r15 to 1 or 0, VM uses this for branching to TruePC)
	BRANCH_EQ_IMM:   generateBranchImm(0x84),
	BRANCH_NE_IMM:   generateBranchImm(0x85),
	BRANCH_LT_U_IMM: generateBranchImm(0x82),
	BRANCH_LE_U_IMM: generateBranchImm(0x86),
	BRANCH_GE_U_IMM: generateBranchImm(0x83),
	BRANCH_GT_U_IMM: generateBranchImm(0x87),
	BRANCH_LT_S_IMM: generateBranchImm(0x8C),
	BRANCH_LE_S_IMM: generateBranchImm(0x8E),
	BRANCH_GE_S_IMM: generateBranchImm(0x8D),
	BRANCH_GT_S_IMM: generateBranchImm(0x8F),
	BRANCH_EQ:       generateCompareBranch(0x84),
	BRANCH_NE:       generateCompareBranch(0x85),
	BRANCH_LT_U:     generateCompareBranch(0x82),
	BRANCH_LT_S:     generateCompareBranch(0x8C),
	BRANCH_GE_U:     generateCompareBranch(0x83),
	BRANCH_GE_S:     generateCompareBranch(0x8D),

	// A.5.2. Instructions with Arguments of One Immediate. InstructionI1
	ECALLI: generateSyscall,

	// A.5.3. Instructions with Arguments of One Register and One Extended Width Immediate.
	LOAD_IMM_64: generateLoadImm64,

	// A.5.4. Instructions with Arguments of Two Immediates.
	STORE_IMM_U8: generateStoreImmGeneric(
		0xC6, // MOV r/m8, imm8
		0x00, // no prefix
		func(v uint64) []byte { return []byte{byte(v)} },
	),
	STORE_IMM_U16: generateStoreImmGeneric(
		0xC7, // MOV r/m16, imm16 (with 0x66 prefix)
		0x66,
		func(v uint64) []byte { return encodeU16(uint16(v)) },
	),
	STORE_IMM_U32: generateStoreImmGeneric(
		0xC7, // MOV r/m32, imm32
		0x00,
		func(v uint64) []byte { return encodeU32(uint32(v)) },
	),
	STORE_IMM_U64: generateStoreImmU64,

	// A.5.6. Instructions with Arguments of One Register & Two Immediates.
	LOAD_IMM: generateLoadImm32,
	LOAD_U8:  generateLoadWithBase([]byte{0x0F, 0xB6}, false), // MOVZX r32, byte ptr [Base+disp32]
	LOAD_I8:  generateLoadWithBase([]byte{0x0F, 0xBE}, true),  // MOVSX r32, byte ptr [Base+disp32]
	LOAD_U16: generateLoadWithBase([]byte{0x0F, 0xB7}, false), // MOVZX r32, word ptr [Base+disp32]

	LOAD_I16:  generateLoadWithBase([]byte{0x0F, 0xBF}, true), // MOVSX r32, word ptr [Base+disp32]
	LOAD_U32:  generateLoadWithBase([]byte{0x8B}, false),      // MOV r32, dword ptr [Base+disp32]
	LOAD_I32:  generateLoadWithBase([]byte{0x63}, true),       // MOVSXD r64, dword ptr [Base+disp32]
	LOAD_U64:  generateLoadWithBase([]byte{0x8B}, true),       // MOV r64, qword ptr [Base+disp32],
	STORE_U8:  generateStoreWithBase(0x88, 0x00, false),       // MOV byte ptr [Base+disp32], r8
	STORE_U16: generateStoreWithBase(0x89, 0x66, false),       // MOV word ptr [Base+disp32], r16
	STORE_U32: generateStoreWithBase(0x89, 0x00, false),       // MOV dword ptr [Base+disp32], r32
	STORE_U64: generateStoreWithBase(0x89, 0x00, true),        // MOV qword ptr [Base+disp32], r64  (REX.W=1)

	// A.5.7. Instructions with Arguments of One Register & Two Immediates.
	STORE_IMM_IND_U8:  generateStoreImmIndU8,
	STORE_IMM_IND_U16: generateStoreImmIndU16,
	STORE_IMM_IND_U32: generateStoreImmIndU32,
	STORE_IMM_IND_U64: generateStoreImmIndU64,

	// A.5.9. Instructions with Arguments of Two Registers.
	MOVE_REG:              generateMoveReg,
	SBRK:                  generateSyscall,
	COUNT_SET_BITS_64:     generateBitCount64,
	COUNT_SET_BITS_32:     generateBitCount32,
	LEADING_ZERO_BITS_64:  generateLeadingZeros64,
	LEADING_ZERO_BITS_32:  generateLeadingZeros32,
	TRAILING_ZERO_BITS_64: generateTrailingZeros64,
	TRAILING_ZERO_BITS_32: generateTrailingZeros32,
	SIGN_EXTEND_8:         generateSignExtend8,
	SIGN_EXTEND_16:        generateSignExtend16,
	ZERO_EXTEND_16:        generateZeroExtend16,
	REVERSE_BYTES:         generateReverseBytes64,

	// A.5.10. Instructions with Arguments of Two Registers & One Immediate.
	STORE_IND_U8:      generateStoreIndirect(0x88, 1),
	STORE_IND_U16:     generateStoreIndirect(0x89, 2),
	STORE_IND_U32:     generateStoreIndirect(0x89, 4),
	STORE_IND_U64:     generateStoreIndirect(0x89, 8),
	LOAD_IND_U8:       generateLoadInd(0x00, LOAD_IND_U8, false),
	LOAD_IND_I8:       generateLoadIndSignExtend(0x00, LOAD_IND_I8, false),
	LOAD_IND_U16:      generateLoadInd(0x66, LOAD_IND_U16, false),
	LOAD_IND_I16:      generateLoadIndSignExtend(0x66, LOAD_IND_I16, false),
	LOAD_IND_U32:      generateLoadInd(0x00, LOAD_IND_U32, false),
	LOAD_IND_I32:      generateLoadIndSignExtend(0x00, LOAD_IND_I32, true),
	LOAD_IND_U64:      generateLoadInd(0x00, LOAD_IND_U64, true),
	ADD_IMM_32:        generateBinaryImm32,
	AND_IMM:           generateImmBinaryOp64(0x81, 4),
	XOR_IMM:           generateImmBinaryOp64(0x81, 6),
	OR_IMM:            generateImmBinaryOp64(0x81, 1),
	MUL_IMM_32:        generateImmMulOp32,
	SET_LT_U_IMM:      generateImmSetCondOp32(0x92), // SETB / below unsigned
	SET_LT_S_IMM:      generateImmSetCondOp32(0x9C), // SETL / below signed
	SET_GT_U_IMM:      generateImmSetCondOp32(0x97), // SETA / above unsigned
	SET_GT_S_IMM:      generateImmSetCondOp32(0x9F), // SETG / above signed
	SHLO_L_IMM_32:     generateImmShiftOp32(0xC1, 4, false),
	SHLO_R_IMM_32:     generateImmShiftOp32(0xC1, 5, false),
	SHLO_L_IMM_ALT_32: generateImmShiftOp32Alt(4),
	SHLO_R_IMM_ALT_32: generateImmShiftOp32(0xC1, 5, true),
	SHAR_R_IMM_ALT_32: generateImmShiftOp32(0xC1, 7, true),
	NEG_ADD_IMM_32:    generateNegAddImm32,
	CMOV_IZ_IMM:       generateCmovIzImm,
	CMOV_NZ_IMM:       generateCmovIzImm,
	ADD_IMM_64:        generateImmBinaryOp64(0x81, 0),
	MUL_IMM_64:        generateImmMulOp64,
	SHLO_L_IMM_64:     generateImmShiftOp64(0xC1, 4),
	SHLO_R_IMM_64:     generateImmShiftOp64(0xC1, 5),
	SHAR_R_IMM_32:     generateImmShiftOp32(0xC1, 7, false),
	SHAR_R_IMM_64:     generateImmShiftOp64(0xC1, 7),
	NEG_ADD_IMM_64:    generateNegAddImm64,
	SHLO_L_IMM_ALT_64: generateImmShiftOp64Alt(4),
	SHLO_R_IMM_ALT_64: generateImmShiftOp64Alt(5),
	SHAR_R_IMM_ALT_64: generateImmShiftOp64Alt(7),
	ROT_R_64_IMM:      generateImmShiftOp64(0xC1, 1),
	ROT_R_64_IMM_ALT:  generateImmShiftOp64(0xC1, 1),
	ROT_R_32_IMM_ALT:  generateImmShiftOp32(0xC1, 1, true),
	ROT_R_32_IMM:      generateRotateRight32Imm,

	// A.5.13. Instructions with Arguments of Three Registers.
	ADD_32:        generateBinaryOp32(0x01), // add
	SUB_32:        generateBinaryOp32(0x29), // sub
	MUL_32:        generateMul32,
	DIV_U_32:      generateDivUOp32,
	DIV_S_32:      generateDivSOp32,
	REM_U_32:      generateRemUOp32,
	REM_S_32:      generateRemSOp32,
	SHLO_L_32:     generateShiftOp32(0xD3, 4),
	SHLO_R_32:     generateShiftOp32(0xD3, 5),
	SHAR_R_32:     generateShiftOp32(0xD3, 7),
	ADD_64:        generateBinaryOp64(0x01), // add
	SUB_64:        generateBinaryOp64(0x29), // sub
	MUL_64:        generateMul64,            // imul
	DIV_U_64:      generateDivUOp64,
	DIV_S_64:      generateDivSOp64,
	REM_U_64:      generateRemUOp64,
	REM_S_64:      generateRemSOp64,
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
	AND_INV:       generateAndInvOp64,
	OR_INV:        generateOrInvOp64,
	XNOR:          generateXnorOp64,
	MAX:           generateCmovCmpOp64(0x4F),
	MAX_U:         generateCmovCmpOp64(0x47),
	MIN:           generateCmovCmpOp64(0x4C),
	MIN_U:         generateCmovCmpOp64(0x42),
}

const BaseRegIndex = 13

var BaseReg = regInfoList[BaseRegIndex]

// use store the original memory address for real memory
// this register is used as base for register dump

// encodeMovImm encodes: mov rX, imm64
func encodeMovImm(regIdx int, imm uint64) []byte {
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
	return append([]byte{prefix, op}, immBytes...)
}

func encodeMovRegToMem(srcIdx, baseIdx int, offset byte) []byte {
	src := regInfoList[srcIdx]
	base := regInfoList[baseIdx]

	rex := byte(0x48)
	if src.REXBit == 1 {
		rex |= 0x04 // REX.R
	}
	if base.REXBit == 1 {
		rex |= 0x01 // REX.B
	}

	if base.RegBits == 4 { // R12
		// Must use SIB encoding when rm=4 (R12)
		modrm := byte(0x40 | (src.RegBits << 3) | 0x04)
		sib := byte(0x20 | base.RegBits) // scale=0, index=none(4), base=R12
		return []byte{rex, 0x89, modrm, sib, offset}
	} else {
		// Normal encoding
		modrm := byte(0x40 | (src.RegBits << 3) | base.RegBits)
		return []byte{rex, 0x89, modrm, offset}
	}
}

// Split a uint64 into its lower 32 bits and higher 32 bits
func splitU64(v uint64) (low uint32, high uint32) {
	low = uint32(v)
	high = uint32(v >> 32)
	return
}

// Generic function to write a 32-bit immediate to [Base+disp]
func emitStoreImm32(buf []byte, disp uint32, imm uint32) []byte {
	base := BaseReg

	// REX prefix: W=0, R=0, X=0, B=base.REXBit
	rex := byte(0x40)
	if base.REXBit != 0 {
		rex |= 0x01
	}
	// MOV [Base+disp], imm32 → opcode 0xC7 /0 + ModRM/SIB
	modrm := byte(0x84) // mod=10, reg=000, r/m=100→SIB
	sib := byte(0x24 | (base.RegBits & 0x07))

	// disp32 LE
	dispBytes := encodeU32(disp)
	immBytes := encodeU32(imm)

	buf = append(buf, rex, 0xC7, modrm, sib)
	buf = append(buf, dispBytes...)
	buf = append(buf, immBytes...)
	return buf
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

func (rvm *RecompilerVM) DumpRegisterToMemory(move_back bool) []byte {
	code := encodeMovImm(BaseRegIndex, uint64(rvm.regDumpAddr))
	// for each register, mov [BaseRegIndex + i*8], rX
	for i := 0; i < len(regInfoList); i++ {
		off := byte(i * 8)
		dumpInstr := encodeMovRegToMem(i, BaseRegIndex, off)
		code = append(code, dumpInstr...)
	}

	if move_back {
		// restore the base register to BaseRegIndex
		code = append(code, encodeMovImm(BaseRegIndex, uint64(uintptr(unsafe.Pointer(&rvm.realMemory[0]))))...)
	}
	return code
}
