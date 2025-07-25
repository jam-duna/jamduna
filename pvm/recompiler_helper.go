package pvm

import (
	"encoding/binary"
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
	JUMP_IND:          generateJumpIndirect,        // jump is managed by the VM using the JumpSourceRegister
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
	STORE_IND_U8:      generateStoreIndirect(1),
	STORE_IND_U16:     generateStoreIndirect(2),
	STORE_IND_U32:     generateStoreIndirect(4),
	STORE_IND_U64:     generateStoreIndirect(8),
	LOAD_IND_U8:       generateLoadInd(0x00, LOAD_IND_U8, false),
	LOAD_IND_I8:       generateLoadIndSignExtend(0x00, LOAD_IND_I8, false),
	LOAD_IND_U16:      generateLoadInd(0x66, LOAD_IND_U16, false),
	LOAD_IND_I16:      generateLoadIndSignExtend(0x66, LOAD_IND_I16, false),
	LOAD_IND_U32:      generateLoadInd(0x00, LOAD_IND_U32, false),
	LOAD_IND_I32:      generateLoadIndSignExtend(0x00, LOAD_IND_I32, true),
	LOAD_IND_U64:      generateLoadInd(0x00, LOAD_IND_U64, true),
	ADD_IMM_32:        generateBinaryImm32,
	AND_IMM:           generateImmBinaryOp64(4),
	XOR_IMM:           generateImmBinaryOp64(6),
	OR_IMM:            generateImmBinaryOp64(1),
	MUL_IMM_32:        generateImmMulOp32,
	SET_LT_U_IMM:      generateImmSetCondOp32New(0x92), // SETB / below unsigned
	SET_LT_S_IMM:      generateImmSetCondOp32New(0x9C), // SETL / below signed
	SET_GT_U_IMM:      generateImmSetCondOp32New(0x97), // SETA / above unsigned
	SET_GT_S_IMM:      generateImmSetCondOp32New(0x9F), // SETG / above signed
	SHLO_L_IMM_32:     generateImmShiftOp32SHLO(4),
	SHLO_R_IMM_32:     generateImmShiftOp32(0xC1, 5, false),
	SHLO_L_IMM_ALT_32: generateImmShiftOp32Alt(4),
	SHLO_R_IMM_ALT_32: generateImmShiftOp32(0xC1, 5, true),
	SHAR_R_IMM_ALT_32: generateImmShiftOp32(0xC1, 7, true),
	NEG_ADD_IMM_32:    generateNegAddImm32,
	CMOV_IZ_IMM:       generateCmovImm(true),
	CMOV_NZ_IMM:       generateCmovImm(false),
	ADD_IMM_64:        generateImmBinaryOp64(0),
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
	ADD_32: generateBinaryOp32(0x01), // add
	SUB_32: generateBinaryOp32(0x29), // sub

	MUL_32:        generateMul32,
	DIV_U_32:      generateDivUOp32,
	DIV_S_32:      generateDivSOp32,
	REM_U_32:      generateRemUOp32,
	REM_S_32:      generateRemSOp32,
	SHLO_L_32:     generateSHLO_L_32(),
	SHLO_R_32:     generateSHLO_R_32(),
	SHAR_R_32:     generateShiftOp32SHAR(0xD3, 7),
	ADD_64:        generateBinaryOp64(0x01), // add
	SUB_64:        generateBinaryOp64(0x29), // sub
	MUL_64:        generateMul64,            // imul
	DIV_U_64:      generateDivUOp64,
	DIV_S_64:      generateDivSOp64,
	REM_U_64:      generateRemUOp64,
	REM_S_64:      generateRemSOp64,
	SHLO_L_64:     generateSHLO_L_64(),
	SHLO_R_64:     generateShiftOp64B(0xD3, 5),
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
	ROT_L_64:      generateROTL64(),
	ROT_L_32:      generateShiftOp32(0xD3, 0),
	ROT_R_64:      generateShiftOp64(0xD3, 1),
	ROT_R_32:      generateROT_R_32(),
	AND_INV:       generateAndInvOp64,
	OR_INV:        generateOrInvOp64,
	XNOR:          generateXnorOp64,
	MAX:           generateMax(),
	MAX_U:         generateMinOrMaxU(true),
	MIN:           generateMin(),
	MIN_U:         generateMinU(),
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
func encodeMovImm64ToMem(memAddr uint64, imm uint64) []byte {
	// this instruction only do once
	// move it on top of the start code to save push & pop

	var code []byte

	// PUSH RAX and RDX (save scratch registers)
	code = append(code, 0x50) // PUSH RAX
	code = append(code, 0x52) // PUSH RDX

	// MOVABS RAX, memAddr
	code = append(code, 0x48, 0xB8) // MOV RAX, imm64
	code = append(code, encodeU64(memAddr)...)

	// MOVABS RDX, imm
	code = append(code, 0x48, 0xBA) // MOV RDX, imm64
	code = append(code, encodeU64(imm)...)

	// MOV [RAX], RDX
	code = append(code, 0x48, 0x89, 0x10) // MOV QWORD PTR [RAX], RDX

	// POP RDX and RAX (restore)
	code = append(code, 0x5A) // POP RDX
	code = append(code, 0x58) // POP RAX

	return code
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
		code = append(code, encodeMovImm(BaseRegIndex, uint64(rvm.realMemAddr))...)
	}
	return code
}

// Load memory[addr:uint64] into dst:r64, using RCX as the default temporary register.
// If dst is RCX itself, use RAX instead as the temporary register.
func generateLoadMemToReg(dst X86Reg, addr uint64) []byte {
	var code []byte
	// replace like mov rax, qword ptr [r12 - 0x36]
	// Choose temporary register: default is RCX, but if dst==RCX use RAX
	var tempReg X86Reg
	var pushOp, popOp byte
	var movAbsRex, movAbsOpcode byte
	// fmt.Printf("generateLoadMemToReg: dst=%s, addr=0x%x\n", dst.Name, addr)
	if dst.Name == "rcx" {
		// Use RAX as temporary register
		tempReg = regInfoList[0] // RAX
		pushOp = 0x50            // PUSH RAX
		popOp = 0x58             // POP  RAX
		movAbsRex = 0x48         // REX.W=1, REX.B=0
		movAbsOpcode = 0xB8      // MOVABS RAX, imm64
	} else {
		// Default to RCX
		tempReg = regInfoList[1] // RCX
		pushOp = 0x51            // PUSH RCX
		popOp = 0x59             // POP  RCX
		movAbsRex = 0x48 | 0x02  // REX.W=1, REX.B=1
		movAbsOpcode = 0xB9      // MOVABS RCX, imm64
	}

	// 1) Save temporary register
	code = append(code, pushOp)

	// 2) MOVABS tempReg, addr
	code = append(code, movAbsRex, movAbsOpcode)
	code = append(code, encodeU64(addr)...)

	// 3) MOV dst, [tempReg]
	//    ModR/M: mod=00 (memory), reg=dst, rm=tempReg
	rex := byte(0x48) // REX.W = 1
	if dst.REXBit == 1 {
		rex |= 0x04 // REX.R for reg field
	}
	if tempReg.REXBit == 1 {
		rex |= 0x01 // REX.B for rm field
	}
	modrm := byte((dst.RegBits << 3) | tempReg.RegBits)
	code = append(code, rex, 0x8B, modrm)

	// 4) Restore temporary register
	code = append(code, popOp)

	return code
}

// Increment the 64-bit value at memory[addr:uint64] by 1.
func generateIncMem(addr uint64) []byte {
	code := []byte{}

	// 1) Save RCX
	code = append(code, 0x51) // PUSH RCX

	// 2) Load target address into RCX: MOVABS RCX, imm64
	code = append(code, 0x48, 0xB9)         // REX.W + opcode for MOVABS RCX, imm64
	code = append(code, encodeU64(addr)...) // imm64 (little-endian)

	// 3) Increment [RCX] as a 64-bit integer: INC QWORD PTR [RCX]
	//    FF /0 ⇒ inc r/m64; mod=00 rm=RCX(1) ⇒ modrm = 0x01
	code = append(code, 0x48, 0xFF, 0x01)

	// 4) Restore RCX
	code = append(code, 0x59) // POP RCX

	return code
}
func (rvm *RecompilerVM) RestoreRegisterInX86() []byte {
	restoredCode := make([]byte, 0)
	for i := 0; i < regSize; i++ {
		reg := regInfoList[i]
		// MOV rX, [dumpAddr + i*8]
		movInstr := generateLoadMemToReg(reg, uint64(rvm.regDumpAddr)+uint64(i*8))
		restoredCode = append(restoredCode, movInstr...)
	}
	return restoredCode
}

func generateJumpRegMem(srcReg X86Reg, rel int32) []byte {
	var code []byte

	// Step 1: REX prefix
	rex := byte(0x48) // REX.W (64-bit)
	if srcReg.REXBit == 1 {
		rex |= 0x01 // REX.B for rm
	}
	code = append(code, rex)

	// Step 2: Opcode for JMP r/m64
	code = append(code, 0xFF)

	mod := byte(0b10 << 6)  // mod = 10 (disp32)
	reg := byte(0b100 << 3) // /4 = JMP

	if srcReg.RegBits == 4 { // r12 or rsp → needs SIB
		modrm := mod | reg | 0b100 // rm = 100 means SIB follows
		code = append(code, modrm)

		// SIB: scale=0, index=100 (none), base=100 (r12/rsp)
		sib := byte(0b00_100_100)
		code = append(code, sib)
	} else {
		modrm := mod | reg | srcReg.RegBits
		code = append(code, modrm)
	}

	// disp32
	disp := make([]byte, 4)
	binary.LittleEndian.PutUint32(disp, uint32(rel))
	code = append(code, disp...)

	return code
}

// ================================================================================================
// Emit Helper Functions
// ================================================================================================

// buildREX constructs a REX prefix byte for x86-64 instructions
// w: REX.W (64-bit operand size)
// r: REX.R (extension of ModR/M reg field)
// x: REX.X (extension of SIB index field)
// b: REX.B (extension of ModR/M rm field, SIB base field, or opcode reg field)
func buildREX(w, r, x, b bool) byte {
	rex := byte(0x40) // REX prefix base
	if w {
		rex |= 0x08 // W bit
	}
	if r {
		rex |= 0x04 // R bit
	}
	if x {
		rex |= 0x02 // X bit
	}
	if b {
		rex |= 0x01 // B bit
	}
	return rex
}

// buildModRM constructs a ModR/M byte
// mod: addressing mode (00, 01, 10, 11)
// reg: register/opcode field (3 bits)
// rm: register/memory field (3 bits)
func buildModRM(mod, reg, rm byte) byte {
	return (mod << 6) | ((reg & 0x07) << 3) | (rm & 0x07)
}

// emitREXModRM emits REX prefix and ModR/M byte for reg-reg operations
func emitREXModRM(w bool, dst, src X86Reg, regField byte) []byte {
	rex := buildREX(w, dst.REXBit == 1, false, src.REXBit == 1)
	modrm := buildModRM(0x03, regField, src.RegBits) // mod=11 for register
	return []byte{rex, modrm}
}

// ================================================================================================
// Basic MOV Instructions
// ================================================================================================

// emitMovRegToReg64 emits: MOV dst, src (64-bit)
func emitMovRegToReg64(dst, src X86Reg) []byte {
	rex := buildREX(true, src.REXBit == 1, false, dst.REXBit == 1)
	modrm := buildModRM(0x03, src.RegBits, dst.RegBits)
	return []byte{rex, 0x89, modrm}
}

// emitMovRegToReg32 emits: MOV dst, src (32-bit)
func emitMovRegToReg32(dst, src X86Reg) []byte {
	rex := buildREX(false, src.REXBit == 1, false, dst.REXBit == 1)
	modrm := buildModRM(0x03, src.RegBits, dst.RegBits)
	return []byte{rex, 0x89, modrm}
}

// emitMovImmToReg64 emits: MOV dst, imm64
func emitMovImmToReg64(dst X86Reg, val uint64) []byte {
	rex := buildREX(true, false, false, dst.REXBit == 1)
	opcode := byte(0xB8 + dst.RegBits)
	result := []byte{rex, opcode}
	result = append(result, encodeU64(val)...)
	return result
}

// emitMovSignExtImm32ToReg64 emits: MOV dst, imm64 (sign-extended from imm32)
func emitMovSignExtImm32ToReg64(dst X86Reg, val uint32) []byte {
	// sign-extend the 32-bit imm into a 64-bit value
	imm64 := uint64(int64(int32(val)))
	return emitMovImmToReg64(dst, imm64)
}

// emitMovImmToReg32 emits: MOV dst, imm32
func emitMovImmToReg32(dst X86Reg, val uint32) []byte {
	var result []byte
	if dst.REXBit == 1 {
		rex := buildREX(false, false, false, true)
		result = append(result, rex)
	}
	opcode := byte(0xB8 + dst.RegBits)
	result = append(result, opcode)
	result = append(result, encodeU32(val)...)
	return result
}

// ================================================================================================
// Arithmetic Instructions
// ================================================================================================

// emitAddReg64 emits: ADD dst, src (64-bit)
func emitAddReg64(dst, src X86Reg) []byte {
	rex := buildREX(true, src.REXBit == 1, false, dst.REXBit == 1)
	modrm := buildModRM(0x03, src.RegBits, dst.RegBits)
	return []byte{rex, 0x01, modrm}
}

// emitSubReg64 emits: SUB dst, src (64-bit)
func emitSubReg64(dst, src X86Reg) []byte {
	rex := buildREX(true, src.REXBit == 1, false, dst.REXBit == 1)
	modrm := buildModRM(0x03, src.RegBits, dst.RegBits)
	return []byte{rex, 0x29, modrm}
}

// emitAddReg32 emits: ADD dst, src (32-bit)
func emitAddReg32(dst, src X86Reg) []byte {
	rex := buildREX(false, src.REXBit == 1, false, dst.REXBit == 1)
	modrm := buildModRM(0x03, src.RegBits, dst.RegBits)
	return []byte{rex, 0x01, modrm}
}

// emitSubReg32 emits: SUB dst, src (32-bit)
func emitSubReg32(dst, src X86Reg) []byte {
	rex := buildREX(false, src.REXBit == 1, false, dst.REXBit == 1)
	modrm := buildModRM(0x03, src.RegBits, dst.RegBits)
	return []byte{rex, 0x29, modrm}
}

// ================================================================================================
// Logical Instructions
// ================================================================================================

// emitAndReg64 emits: AND dst, src (64-bit)
func emitAndReg64(dst, src X86Reg) []byte {
	rex := buildREX(true, src.REXBit == 1, false, dst.REXBit == 1)
	modrm := buildModRM(0x03, src.RegBits, dst.RegBits)
	return []byte{rex, 0x21, modrm}
}

// emitOrReg64 emits: OR dst, src (64-bit)
func emitOrReg64(dst, src X86Reg) []byte {
	rex := buildREX(true, src.REXBit == 1, false, dst.REXBit == 1)
	modrm := buildModRM(0x03, src.RegBits, dst.RegBits)
	return []byte{rex, 0x09, modrm}
}

// emitXorReg64 emits: XOR dst, src (64-bit)
func emitXorReg64(dst, src X86Reg) []byte {
	rex := buildREX(true, src.REXBit == 1, false, dst.REXBit == 1)
	modrm := buildModRM(0x03, src.RegBits, dst.RegBits)
	return []byte{rex, 0x31, modrm}
}

// emitAndReg32 emits: AND dst, src (32-bit)
func emitAndReg32(dst, src X86Reg) []byte {
	rex := buildREX(false, src.REXBit == 1, false, dst.REXBit == 1)
	modrm := buildModRM(0x03, src.RegBits, dst.RegBits)
	return []byte{rex, 0x21, modrm}
}

// emitOrReg32 emits: OR dst, src (32-bit)
func emitOrReg32(dst, src X86Reg) []byte {
	rex := buildREX(false, src.REXBit == 1, false, dst.REXBit == 1)
	modrm := buildModRM(0x03, src.RegBits, dst.RegBits)
	return []byte{rex, 0x09, modrm}
}

// emitXorReg32 emits: XOR dst, src (32-bit)
func emitXorReg32(dst, src X86Reg) []byte {
	rex := buildREX(false, src.REXBit == 1, false, dst.REXBit == 1)
	modrm := buildModRM(0x03, src.RegBits, dst.RegBits)
	return []byte{rex, 0x31, modrm}
}

// ================================================================================================
// Unary Instructions
// ================================================================================================

// emitNotReg64 emits: NOT reg (64-bit)
func emitNotReg64(reg X86Reg) []byte {
	rex := buildREX(true, false, false, reg.REXBit == 1)
	modrm := buildModRM(0x03, 2, reg.RegBits) // /2 for NOT
	return []byte{rex, 0xF7, modrm}
}

// emitNotReg32 emits: NOT reg (32-bit)
func emitNotReg32(reg X86Reg) []byte {
	rex := buildREX(false, false, false, reg.REXBit == 1)
	modrm := buildModRM(0x03, 2, reg.RegBits) // /2 for NOT
	return []byte{rex, 0xF7, modrm}
}

// ================================================================================================
// Comparison Instructions
// ================================================================================================

// emitCmpReg64 emits: CMP a, b (64-bit)
func emitCmpReg64(a, b X86Reg) []byte {
	rex := buildREX(true, a.REXBit == 1, false, b.REXBit == 1)
	modrm := buildModRM(0x03, a.RegBits, b.RegBits)
	return []byte{rex, 0x39, modrm}
}

// emitCmpReg32 emits: CMP a, b (32-bit)
func emitCmpReg32(a, b X86Reg) []byte {
	rex := buildREX(false, a.REXBit == 1, false, b.REXBit == 1)
	modrm := buildModRM(0x03, a.RegBits, b.RegBits)
	return []byte{rex, 0x39, modrm}
}

// ================================================================================================
// Push/Pop Instructions
// ================================================================================================

// emitPushReg emits: PUSH reg
func emitPushReg(reg X86Reg) []byte {
	if reg.REXBit == 1 {
		return []byte{0x41, byte(0x50 + reg.RegBits)}
	}
	return []byte{byte(0x50 + reg.RegBits)}
}

// emitPopReg emits: POP reg
func emitPopReg(reg X86Reg) []byte {
	if reg.REXBit == 1 {
		return []byte{0x41, byte(0x58 + reg.RegBits)}
	}
	return []byte{byte(0x58 + reg.RegBits)}
}

// ================================================================================================
// Complex Instructions
// ================================================================================================

// emitPopCnt64 emits: POPCNT dst, src (64-bit)
func emitPopCnt64(dst, src X86Reg) []byte {
	rex := buildREX(true, dst.REXBit == 1, false, src.REXBit == 1)
	modrm := buildModRM(0x03, dst.RegBits, src.RegBits)
	return []byte{0xF3, rex, 0x0F, 0xB8, modrm}
}

// emitPopCnt32 emits: POPCNT dst, src (32-bit)
func emitPopCnt32(dst, src X86Reg) []byte {
	rex := buildREX(false, dst.REXBit == 1, false, src.REXBit == 1)
	modrm := buildModRM(0x03, dst.RegBits, src.RegBits)
	return []byte{0xF3, rex, 0x0F, 0xB8, modrm}
}

// emitLzcnt64 emits: LZCNT dst, src (64-bit)
func emitLzcnt64(dst, src X86Reg) []byte {
	rex := buildREX(true, dst.REXBit == 1, false, src.REXBit == 1)
	modrm := buildModRM(0x03, dst.RegBits, src.RegBits)
	return []byte{0xF3, rex, 0x0F, 0xBD, modrm}
}

// emitLzcnt32 emits: LZCNT dst, src (32-bit)
func emitLzcnt32(dst, src X86Reg) []byte {
	rex := buildREX(false, dst.REXBit == 1, false, src.REXBit == 1)
	modrm := buildModRM(0x03, dst.RegBits, src.RegBits)
	return []byte{0xF3, rex, 0x0F, 0xBD, modrm}
}

// emitTzcnt64 emits: TZCNT dst, src (64-bit)
func emitTzcnt64(dst, src X86Reg) []byte {
	rex := buildREX(true, dst.REXBit == 1, false, src.REXBit == 1)
	modrm := buildModRM(0x03, dst.RegBits, src.RegBits)
	return []byte{0xF3, rex, 0x0F, 0xBC, modrm}
}

// emitTzcnt32 emits: TZCNT dst, src (32-bit)
func emitTzcnt32(dst, src X86Reg) []byte {
	rex := buildREX(false, dst.REXBit == 1, false, src.REXBit == 1)
	modrm := buildModRM(0x03, dst.RegBits, src.RegBits)
	return []byte{0xF3, rex, 0x0F, 0xBC, modrm}
}

// emitMovsx64From8 emits: MOVSX dst, src (8-bit to 64-bit sign extend)
func emitMovsx64From8(dst, src X86Reg) []byte {
	rex := buildREX(true, dst.REXBit == 1, false, src.REXBit == 1)
	modrm := buildModRM(0x03, dst.RegBits, src.RegBits)
	return []byte{rex, 0x0F, 0xBE, modrm}
}

// emitMovsx64From16 emits: MOVSX dst, src (16-bit to 64-bit sign extend)
func emitMovsx64From16(dst, src X86Reg) []byte {
	rex := buildREX(true, dst.REXBit == 1, false, src.REXBit == 1)
	modrm := buildModRM(0x03, dst.RegBits, src.RegBits)
	return []byte{rex, 0x0F, 0xBF, modrm}
}

// emitMovzx64From16 emits: MOVZX dst, src (16-bit to 64-bit zero extend)
func emitMovzx64From16(dst, src X86Reg) []byte {
	rex := buildREX(true, dst.REXBit == 1, false, src.REXBit == 1)
	modrm := buildModRM(0x03, dst.RegBits, src.RegBits)
	return []byte{rex, 0x0F, 0xB7, modrm}
}

// emitBswap64 emits: BSWAP dst (64-bit byte swap)
func emitBswap64(dst X86Reg) []byte {
	rex := buildREX(true, false, false, dst.REXBit == 1)
	opcode := byte(0xC8 + dst.RegBits)
	return []byte{rex, 0x0F, opcode}
}

// ================================================================================================
// Memory Operations
// ================================================================================================

// emitMovRegFromMem emits: MOV dst, [base + disp] (64-bit)
func emitMovRegFromMem(dst X86Reg, base X86Reg, disp int32) []byte {
	rex := buildREX(true, dst.REXBit == 1, false, base.REXBit == 1)
	code := []byte{rex, 0x8B}

	// ModR/M and displacement
	if disp == 0 && base.RegBits != 5 { // RBP needs displacement
		modrm := buildModRM(0x00, dst.RegBits, base.RegBits)
		code = append(code, modrm)
	} else if disp >= -128 && disp <= 127 {
		modrm := buildModRM(0x01, dst.RegBits, base.RegBits)
		code = append(code, modrm, byte(disp))
	} else {
		modrm := buildModRM(0x02, dst.RegBits, base.RegBits)
		code = append(code, modrm)
		dispBytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(dispBytes, uint32(disp))
		code = append(code, dispBytes...)
	}
	return code
}

// emitMovMemFromReg emits: MOV [base + disp], src (64-bit)
func emitMovMemFromReg(base X86Reg, disp int32, src X86Reg) []byte {
	rex := buildREX(true, src.REXBit == 1, false, base.REXBit == 1)
	code := []byte{rex, 0x89}

	// ModR/M and displacement
	if disp == 0 && base.RegBits != 5 { // RBP needs displacement
		modrm := buildModRM(0x00, src.RegBits, base.RegBits)
		code = append(code, modrm)
	} else if disp >= -128 && disp <= 127 {
		modrm := buildModRM(0x01, src.RegBits, base.RegBits)
		code = append(code, modrm, byte(disp))
	} else {
		modrm := buildModRM(0x02, src.RegBits, base.RegBits)
		code = append(code, modrm)
		dispBytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(dispBytes, uint32(disp))
		code = append(code, dispBytes...)
	}
	return code
}

// ================================================================================================
// Immediate Operations
// ================================================================================================

// emitCmpRegImm32 emits: CMP reg, imm32 (signed 32-bit immediate)
func emitCmpRegImm32(reg X86Reg, imm int32) []byte {
	rex := buildREX(true, false, false, reg.REXBit == 1)

	if imm >= -128 && imm <= 127 {
		// Use 8-bit immediate form: CMP r/m64, imm8
		modrm := buildModRM(0x03, 7, reg.RegBits) // /7 for CMP
		return []byte{rex, 0x83, modrm, byte(imm)}
	} else {
		// Use 32-bit immediate form: CMP r/m64, imm32
		modrm := buildModRM(0x03, 7, reg.RegBits) // /7 for CMP
		immBytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(immBytes, uint32(imm))
		result := []byte{rex, 0x81, modrm}
		result = append(result, immBytes...)
		return result
	}
}

// emitAddRegImm32 emits: ADD reg, imm32 (signed 32-bit immediate)
func emitAddRegImm32(reg X86Reg, imm int32) []byte {
	rex := buildREX(true, false, false, reg.REXBit == 1)

	if imm >= -128 && imm <= 127 {
		// Use 8-bit immediate form: ADD r/m64, imm8
		modrm := buildModRM(0x03, 0, reg.RegBits) // /0 for ADD
		return []byte{rex, 0x83, modrm, byte(imm)}
	} else {
		// Use 32-bit immediate form: ADD r/m64, imm32
		modrm := buildModRM(0x03, 0, reg.RegBits) // /0 for ADD
		immBytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(immBytes, uint32(imm))
		result := []byte{rex, 0x81, modrm}
		result = append(result, immBytes...)
		return result
	}
}

// ================================================================================================
// Conditional Instructions
// ================================================================================================

// emitNegReg64 emits: NEG reg64 (negate register)
func emitNegReg64(reg X86Reg) []byte {
	rex := buildREX(true, false, false, reg.REXBit == 1)
	modrm := byte(0xD8 | reg.RegBits) // ModR/M: mod=11, reg=3 (/3), rm=reg
	return []byte{rex, 0xF7, modrm}   // F7 /3 = NEG r/m64
}

// emitAddRegImm8 emits: ADD reg64, imm8
func emitAddRegImm8(reg X86Reg, imm int8) []byte {
	rex := buildREX(true, false, false, reg.REXBit == 1)
	modrm := buildModRM(3, 0, reg.RegBits)     // reg=0 (/0)
	return []byte{rex, 0x83, modrm, byte(imm)} // 83 /0 ib = ADD r/m64, imm8
}

// emitShiftRegImm1 emits: shift reg, 1 (D1 /subcode)
func emitShiftRegImm1(reg X86Reg, subcode byte) []byte {
	rex := buildREX(true, false, false, reg.REXBit == 1)
	modrm := buildModRM(3, subcode, reg.RegBits)
	return []byte{rex, 0xD1, modrm}
}

// emitShiftRegImm emits: shift reg, imm8 (opcode /subcode ib)
func emitShiftRegImm(reg X86Reg, opcode, subcode byte, imm byte) []byte {
	rex := buildREX(true, false, false, reg.REXBit == 1)
	modrm := buildModRM(3, subcode, reg.RegBits)
	return []byte{rex, opcode, modrm, imm}
}

// emitTestReg64 emits: TEST dst, src (test and set flags)
func emitTestReg64(dst, src X86Reg) []byte {
	rex := buildREX(true, src.REXBit == 1, false, dst.REXBit == 1)
	modrm := buildModRM(3, src.RegBits, dst.RegBits)
	return []byte{rex, 0x85, modrm} // 85 /r = TEST r/m64, r64
}

// emitCmovcc emits: CMOVcc dst, src (conditional move)
func emitCmovcc(cc byte, dst, src X86Reg) []byte {
	rex := buildREX(true, dst.REXBit == 1, false, src.REXBit == 1)
	modrm := buildModRM(0x03, dst.RegBits, src.RegBits)
	return []byte{rex, 0x0F, cc, modrm}
}

// emitSetcc emits: SETcc reg (set byte on condition)
func emitSetcc(cc byte, reg X86Reg) []byte {
	rex := buildREX(false, false, false, reg.REXBit == 1)
	modrm := buildModRM(0x03, 0, reg.RegBits)
	return []byte{rex, 0x0F, cc, modrm}
}

// ================================================================================================
// Jump Instructions
// ================================================================================================

// emitJcc emits: Jcc rel32 (conditional jump)
func emitJcc(cc byte, rel32 int32) []byte {
	result := []byte{0x0F, cc}
	relBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(relBytes, uint32(rel32))
	result = append(result, relBytes...)
	return result
}

// emitJmp emits: JMP rel32 (unconditional jump)
func emitJmp(rel32 int32) []byte {
	result := []byte{0xE9}
	relBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(relBytes, uint32(rel32))
	result = append(result, relBytes...)
	return result
}

// ================================================================================================
// Additional Specialized Instructions
// ================================================================================================

// emitMovzx64From8 emits: MOVZX dst, src (8-bit to 64-bit zero extend)
func emitMovzx64From8(dst X86Reg) []byte {
	rex := buildREX(true, dst.REXBit == 1, false, dst.REXBit == 1)
	modrm := buildModRM(0x03, dst.RegBits, dst.RegBits)
	return []byte{rex, 0x0F, 0xB6, modrm}
}

// emitMovsxd emits: MOVSXD dst, src (32-bit to 64-bit sign extend)
func emitMovsxd(dst, src X86Reg) []byte {
	rex := buildREX(true, dst.REXBit == 1, false, src.REXBit == 1)
	modrm := buildModRM(0x03, dst.RegBits, src.RegBits)
	return []byte{rex, 0x63, modrm}
}

// emitMovByteToCL emits: MOV CL, src_low8
func emitMovByteToCL(src X86Reg) []byte {
	rex := buildREX(false, false, false, src.REXBit == 1)
	modrm := buildModRM(0x03, 1, src.RegBits) // CL is RCX low byte (reg=1)
	return []byte{rex, 0x88, modrm}
}

// emitMovAbsToReg emits: MOVABS dst, imm64
func emitMovAbsToReg(dst X86Reg, val uint64) []byte {
	return emitMovImmToReg64(dst, val) // Same as MOV reg, imm64
}

// emitMovFromStack emits: MOV dst, [RSP + offset]
func emitMovFromStack(dst X86Reg, offset int) []byte {
	// For now, use a simple approach - this may need adjustment based on actual register layout
	return emitMovRegFromMem(dst, BaseReg, int32(offset))
}

// emitTest64 emits: TEST reg, reg (64-bit)
func emitTest64(reg X86Reg) []byte {
	rex := buildREX(true, reg.REXBit == 1, false, reg.REXBit == 1)
	modrm := buildModRM(0x03, reg.RegBits, reg.RegBits)
	return []byte{rex, 0x85, modrm}
}

// emitTest32 emits: TEST reg, reg (32-bit)
func emitTest32(reg X86Reg) []byte {
	rex := buildREX(false, reg.REXBit == 1, false, reg.REXBit == 1)
	modrm := buildModRM(0x03, reg.RegBits, reg.RegBits)
	return []byte{rex, 0x85, modrm}
}

// emitImul64 emits: IMUL dst, src (64-bit)
func emitImul64(dst, src X86Reg) []byte {
	rex := buildREX(true, dst.REXBit == 1, false, src.REXBit == 1)
	modrm := buildModRM(0x03, dst.RegBits, src.RegBits)
	return []byte{rex, 0x0F, 0xAF, modrm}
}

// emitImul32 emits: IMUL dst, src (32-bit)
func emitImul32(dst, src X86Reg) []byte {
	rex := buildREX(false, dst.REXBit == 1, false, src.REXBit == 1)
	modrm := buildModRM(0x03, dst.RegBits, src.RegBits)
	return []byte{rex, 0x0F, 0xAF, modrm}
}

// emitDivReg64 emits: DIV r64 (unsigned divide RDX:RAX by r64)
func emitDivReg64(src X86Reg) []byte {
	rex := buildREX(true, false, false, src.REXBit == 1)
	modrm := buildModRM(0x03, 0x06, src.RegBits) // opcode extension /6 for DIV
	return []byte{rex, 0xF7, modrm}
}

// emitDivMem64 emits: DIV m64 with SIB addressing [RSP+disp32]
func emitDivMem64(disp32 uint32) []byte {
	buf := []byte{0x48, 0xF7, 0xB4, 0x24} // REX.W F7 /6 with SIB
	dispBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(dispBytes, disp32)
	buf = append(buf, dispBytes...)
	return buf
}

// emitDivMemRSP64 emits: DIV qword ptr [RSP] (divisor at stack top)
func emitDivMemRSP64() []byte {
	return []byte{0x48, 0xF7, 0x34, 0x24} // REX.W F7 /6 with SIB for [RSP]
}

// emitXchgReg64 emits: XCHG r64, r64
func emitXchgReg64(reg1, reg2 X86Reg) []byte {
	rex := buildREX(true, reg1.REXBit == 1, false, reg2.REXBit == 1)
	modrm := buildModRM(0x03, reg1.RegBits, reg2.RegBits)
	return []byte{rex, 0x87, modrm}
}

// emitIdivReg64 emits: IDIV r64 (signed divide RDX:RAX by r64)
func emitIdivReg64(src X86Reg) []byte {
	rex := buildREX(true, false, false, src.REXBit == 1)
	modrm := buildModRM(0x03, 0x07, src.RegBits) // opcode extension /7 for IDIV
	return []byte{rex, 0xF7, modrm}
}

// emitIdivMemRSP64 emits: IDIV qword ptr [RSP] (signed divide by value at stack top)
func emitIdivMemRSP64() []byte {
	return []byte{0x48, 0xF7, 0x3C, 0x24} // REX.W F7 /7 with SIB for [RSP]
}

// emitCqo emits: CQO (convert quad word to oct word, sign-extend RAX to RDX:RAX)
func emitCqo() []byte {
	return []byte{0x48, 0x99} // REX.W + 99
}

// emitTestReg32 emits: TEST r32, r32
func emitTestReg32(dst, src X86Reg) []byte {
	rex := buildREX(false, src.REXBit == 1, false, dst.REXBit == 1)
	modrm := buildModRM(3, src.RegBits, dst.RegBits)
	return []byte{rex, 0x85, modrm} // 85 /r = TEST r/m32, r32
}

// emitDivReg32 emits: DIV r32 (unsigned divide EDX:EAX by r32)
func emitDivReg32(src X86Reg) []byte {
	rex := buildREX(false, false, false, src.REXBit == 1)
	modrm := buildModRM(0x03, 0x06, src.RegBits) // opcode extension /6 for DIV
	return []byte{rex, 0xF7, modrm}
}

// emitMovsxdReg emits: MOVSXD r64, r32 (sign-extend 32-bit to 64-bit)
func emitMovsxdReg(dst, src X86Reg) []byte {
	rex := buildREX(true, dst.REXBit == 1, false, src.REXBit == 1)
	modrm := buildModRM(0x03, dst.RegBits, src.RegBits)
	return []byte{rex, 0x63, modrm} // 63 /r = MOVSXD r64, r/m32
}

// emitCmpMemRSPImm32 emits: CMP qword ptr [RSP], imm32
func emitCmpMemRSPImm32(imm32 int32) []byte {
	buf := []byte{0x48, 0x81, 0x3C, 0x24} // REX.W, 81 /7, ModRM, SIB
	immBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(immBytes, uint32(imm32))
	buf = append(buf, immBytes...)
	return buf
}

// emitNot64Reg emits: NOT r64 (bitwise NOT)
func emitNot64Reg(reg X86Reg) []byte {
	rex := buildREX(true, false, false, reg.REXBit == 1)
	modrm := buildModRM(0x03, 0x02, reg.RegBits) // opcode extension /2 for NOT
	return []byte{rex, 0xF7, modrm}
}

// emitCdq emits: CDQ (convert double word to quad word, sign-extend EAX to EDX:EAX)
func emitCdq() []byte {
	return []byte{0x99} // 99 = CDQ
}

// emitIdiv32 emits: IDIV r32 (signed divide EDX:EAX by r32)
func emitIdiv32(src X86Reg) []byte {
	rex := buildREX(false, false, false, src.REXBit == 1)
	modrm := buildModRM(0x03, 0x07, src.RegBits) // opcode extension /7 for IDIV
	return []byte{rex, 0xF7, modrm}
}

// emitCmpRegImm32MinInt emits: CMP reg, 0x80000000 (for MinInt32 comparison)
func emitCmpRegImm32MinInt(reg X86Reg) []byte {
	rex := buildREX(false, false, false, reg.REXBit == 1)   // 32-bit operation
	modrm := buildModRM(0x03, 7, reg.RegBits)               // /7 for CMP
	return []byte{rex, 0x81, modrm, 0x00, 0x00, 0x00, 0x80} // 0x80000000 in little-endian
}

// emitMovEaxFromReg32 emits: MOV EAX, reg32 (reads from reg into EAX)
func emitMovEaxFromReg32(src X86Reg) []byte {
	rex := buildREX(false, false, false, src.REXBit == 1) // rm field uses REX.B
	modrm := buildModRM(0x03, 0, src.RegBits)             // reg=0 (EAX), rm=src
	return []byte{rex, 0x8B, modrm}                       // 8B /r = MOV r32, r/m32
}

// emitCmpRegImmByte emits: CMP reg, imm8 (32-bit register compare with 8-bit immediate)
func emitCmpRegImmByte(reg X86Reg, imm byte) []byte {
	rex := buildREX(false, false, false, reg.REXBit == 1) // 32-bit operation
	modrm := buildModRM(0x03, 7, reg.RegBits)             // /7 for CMP
	return []byte{rex, 0x83, modrm, imm}
}

// emitMovsxd64 emits: MOVSXD dst64, src32 (sign-extend 32-bit to 64-bit)
func emitMovsxd64(dst X86Reg, src X86Reg) []byte {
	rex := buildREX(true, dst.REXBit == 1, false, src.REXBit == 1) // 64-bit operation, REX.W=1
	modrm := buildModRM(0x03, dst.RegBits, src.RegBits)
	return []byte{rex, 0x63, modrm} // 63 /r = MOVSXD r64, r/m32
}

// emitPushReg64 emits: PUSH reg64
func emitPushReg64(reg X86Reg) []byte {
	if reg.REXBit == 1 {
		return []byte{0x40 | 0x01, 0x50 + reg.RegBits} // REX.B + PUSH
	}
	return []byte{0x50 + reg.RegBits} // PUSH r64
}

// emitPopReg64 emits: POP reg64
func emitPopReg64(reg X86Reg) []byte {
	if reg.REXBit == 1 {
		return []byte{0x40 | 0x01, 0x58 + reg.RegBits} // REX.B + POP
	}
	return []byte{0x58 + reg.RegBits} // POP r64
}

// emitTestRegReg32 emits: TEST reg32, reg32
func emitTestRegReg32(reg X86Reg) []byte {
	rex := buildREX(false, reg.REXBit == 1, false, reg.REXBit == 1) // 32-bit, R=reg, B=reg
	modrm := buildModRM(0x03, reg.RegBits, reg.RegBits)             // mod=11, reg=reg, rm=reg
	return []byte{rex, 0x85, modrm}
}

// emitXorReg64Reg64 emits: XOR reg64, reg64
func emitXorReg64Reg64(reg X86Reg) []byte {
	rex := buildREX(true, reg.REXBit == 1, false, reg.REXBit == 1) // 64-bit, R=reg, B=reg
	modrm := buildModRM(0x03, reg.RegBits, reg.RegBits)            // mod=11, reg=reg, rm=reg
	return []byte{rex, 0x31, modrm}
}

// emitJne32 emits: JNE rel32 (0x0F 0x85 + 4-byte displacement)
func emitJne32() []byte {
	return []byte{0x0F, 0x85, 0, 0, 0, 0} // 4-byte displacement filled in later
}

// emitJmp32 emits: JMP rel32 (0xE9 + 4-byte displacement)
func emitJmp32() []byte {
	return []byte{0xE9, 0, 0, 0, 0} // 4-byte displacement filled in later
}

// emitPopRdxRax emits: POP RDX; POP RAX
func emitPopRdxRax() []byte {
	return []byte{0x5A, 0x58} // POP RDX, POP RAX
}

// emitPushRax emits: PUSH RAX
func emitPushRax() []byte {
	return []byte{0x50}
}

// emitPopRax emits: POP RAX
func emitPopRax() []byte {
	return []byte{0x58}
}

// emitMovEaxFromReg32 emits: MOV EAX, reg32 (different from emitMovRegReg32)
func emitMovRegFromEax32(dst X86Reg) []byte {
	rex := buildREX(false, false, false, dst.REXBit == 1) // 32-bit, B=dst
	modrm := buildModRM(0x03, 0, dst.RegBits)             // mod=11, reg=0 (EAX), rm=dst
	return []byte{rex, 0x8B, modrm}                       // MOV dst32, EAX
}

// emitMovRegReg32 emits: MOV dst32, src32
func emitMovRegReg32(dst, src X86Reg) []byte {
	rex := buildREX(false, src.REXBit == 1, false, dst.REXBit == 1) // 32-bit, R=src, B=dst
	modrm := buildModRM(0x03, src.RegBits, dst.RegBits)             // mod=11, reg=src, rm=dst
	return []byte{rex, 0x89, modrm}                                 // MOV dst32, src32
}

// emitBinaryOpRegEax32 emits: dst32 = dst32 <op> EAX
func emitBinaryOpRegEax32(opcode byte, dst X86Reg) []byte {
	rex := buildREX(false, false, false, dst.REXBit == 1) // 32-bit, B=dst
	modrm := buildModRM(0x03, 0, dst.RegBits)             // mod=11, reg=0 (EAX), rm=dst
	return []byte{rex, opcode, modrm}
}

// emitBinaryOpRegReg32 emits: dst32 = dst32 <op> src32
func emitBinaryOpRegReg32(opcode byte, dst, src X86Reg) []byte {
	rex := buildREX(false, src.REXBit == 1, false, dst.REXBit == 1) // 32-bit, R=src, B=dst
	modrm := buildModRM(0x03, src.RegBits, dst.RegBits)             // mod=11, reg=src, rm=dst
	return []byte{rex, opcode, modrm}
}

// emitPushRcx emits: PUSH RCX
func emitPushRcx() []byte {
	return []byte{0x51}
}

// emitPopRcx emits: POP RCX
func emitPopRcx() []byte {
	return []byte{0x59}
}

// emitShiftOp64 emits: shift operation on reg64 by CL
func emitShiftOp64(opcode byte, regField byte, reg X86Reg) []byte {
	rex := buildREX(true, false, false, reg.REXBit == 1) // 64-bit, B=reg
	modrm := buildModRM(0x03, regField, reg.RegBits)     // mod=11, reg=regField, rm=reg
	return []byte{rex, opcode, modrm}
}

// emitRol64ByCl emits: ROL reg64, CL
func emitRol64ByCl(reg X86Reg) []byte {
	rex := buildREX(true, false, false, reg.REXBit == 1) // 64-bit, B=reg
	modrm := buildModRM(0x03, 0, reg.RegBits)            // mod=11, reg=0 (/0 for ROL), rm=reg
	return []byte{rex, 0xD3, modrm}
}

// emitXchgRcxReg64 emits: XCHG RCX, reg64
func emitXchgRcxReg64(reg X86Reg) []byte {
	rex := buildREX(true, reg.REXBit == 1, false, false) // 64-bit, R=reg, rm=RCX(1)
	modrm := buildModRM(0x03, reg.RegBits, 1)            // mod=11, reg=reg, rm=1 (RCX)
	return []byte{rex, 0x87, modrm}
}

// emitXchgRegReg64 emits: XCHG reg1, reg2
func emitXchgRegReg64(reg1, reg2 X86Reg) []byte {
	rex := buildREX(true, reg1.REXBit == 1, false, reg2.REXBit == 1) // 64-bit, R=reg1, B=reg2
	modrm := buildModRM(0x03, reg1.RegBits, reg2.RegBits)            // mod=11, reg=reg1, rm=reg2
	return []byte{rex, 0x87, modrm}
}

// emitPushRdx emits: PUSH RDX
func emitPushRdx() []byte {
	return []byte{0x52}
}

// emitPopRdx emits: POP RDX
func emitPopRdx() []byte {
	return []byte{0x5A}
}

// emitMulReg64 emits: MUL reg64 (unsigned)
func emitMulReg64(reg X86Reg) []byte {
	rex := buildREX(true, false, false, reg.REXBit == 1) // 64-bit, B=reg
	modrm := buildModRM(0x03, 4, reg.RegBits)            // mod=11, reg=4 (/4 for MUL), rm=reg
	return []byte{rex, 0xF7, modrm}
}

// emitImulReg64 emits: IMUL reg64 (signed)
func emitImulReg64(reg X86Reg) []byte {
	rex := buildREX(true, false, false, reg.REXBit == 1) // 64-bit, B=reg
	modrm := buildModRM(0x03, 5, reg.RegBits)            // mod=11, reg=5 (/5 for IMUL), rm=reg
	return []byte{rex, 0xF7, modrm}
}

// emitMovEcxFromReg32 emits: MOV ECX, reg32
func emitMovEcxFromReg32(src X86Reg) []byte {
	rex := buildREX(false, false, false, src.REXBit == 1) // 32-bit, B=src
	modrm := buildModRM(0x03, 1, src.RegBits)             // mod=11, reg=1 (ECX), rm=src
	return []byte{rex, 0x8B, modrm}
}

// emitShl32ByCl emits: SHL reg32, CL
func emitShl32ByCl(reg X86Reg) []byte {
	rex := buildREX(false, false, false, reg.REXBit == 1) // 32-bit, B=reg
	modrm := buildModRM(0x03, 4, reg.RegBits)             // mod=11, reg=4 (/4 for SHL), rm=reg
	return []byte{rex, 0xD3, modrm}
}

// emitMovReg32 emits: MOV dst32, src32 (32-bit register to register move)
func emitMovReg32(dst X86Reg, src X86Reg) []byte {
	rex := buildREX(false, dst.REXBit == 1, false, src.REXBit == 1) // 32-bit operation
	modrm := buildModRM(0x03, dst.RegBits, src.RegBits)
	return []byte{rex, 0x8B, modrm} // 8B /r = MOV r32, r/m32
}

// emitImulReg32 emits: IMUL dst32, src32 (32-bit signed multiply)
func emitImulReg32(dst X86Reg, src X86Reg) []byte {
	rex := buildREX(false, dst.REXBit == 1, false, src.REXBit == 1) // 32-bit operation
	modrm := buildModRM(0x03, dst.RegBits, src.RegBits)
	return []byte{rex, 0x0F, 0xAF, modrm} // 0F AF /r = IMUL r32, r/m32
}

// emitCmpReg64Reg64 emits: CMP src1_64, src2_64 (64-bit register compare)
func emitCmpReg64Reg64(src1 X86Reg, src2 X86Reg) []byte {
	rex := buildREX(true, src2.REXBit == 1, false, src1.REXBit == 1) // 64-bit, R=src2, B=src1
	modrm := buildModRM(0x03, src2.RegBits, src1.RegBits)
	return []byte{rex, 0x39, modrm} // 39 /r = CMP r/m64, r64
}

// emitSetccReg8 emits: SETcc r/m8 (set byte on condition)
func emitSetccReg8(cc byte, dst X86Reg) []byte {
	rex := buildREX(false, false, false, dst.REXBit == 1) // 8-bit operation, B=dst
	modrm := buildModRM(0x03, 0, dst.RegBits)
	return []byte{rex, 0x0F, cc, modrm} // 0F 90+cc /r = SETcc r/m8
}

// emitMovzxReg64Reg8 emits: MOVZX dst64, src8 (zero-extend 8-bit to 64-bit)
func emitMovzxReg64Reg8(dst X86Reg, src X86Reg) []byte {
	rex := buildREX(true, dst.REXBit == 1, false, src.REXBit == 1) // 64-bit dest, R=dst, B=src
	modrm := buildModRM(0x03, dst.RegBits, src.RegBits)
	return []byte{rex, 0x0F, 0xB6, modrm} // 0F B6 /r = MOVZX r64, r/m8
}

// emitMovsxdReg64Reg32 emits: MOVSXD dst64, src32 (sign-extend 32-bit to 64-bit)
func emitMovsxdReg64Reg32(dst X86Reg, src X86Reg) []byte {
	rex := buildREX(true, dst.REXBit == 1, false, src.REXBit == 1) // 64-bit operation, R=dst, B=src
	modrm := buildModRM(0x03, dst.RegBits, src.RegBits)
	return []byte{rex, 0x63, modrm} // 63 /r = MOVSXD r64, r/m32
}

// emitAndRegImm8 emits: AND reg32, imm8
func emitAndRegImm8(dst X86Reg, imm byte) []byte {
	rex := buildREX(false, false, false, dst.REXBit == 1) // 32-bit operation, B=dst
	// Use 83 /4 ib form for 8-bit immediate
	modrm := buildModRM(0x03, 4, dst.RegBits) // reg=4 for AND
	return []byte{rex, 0x83, modrm, imm}
}

// emitSarReg32ByCl emits: SAR reg32, CL
func emitSarReg32ByCl(dst X86Reg) []byte {
	rex := buildREX(false, false, false, dst.REXBit == 1) // 32-bit operation, B=dst
	modrm := buildModRM(0x03, 7, dst.RegBits)             // reg=7 for SAR
	return []byte{rex, 0xD3, modrm}                       // D3 /7 = SAR r/m32, CL
}

// emitShrReg32ByCl emits: SHR reg32, CL
func emitShrReg32ByCl(dst X86Reg) []byte {
	rex := buildREX(false, false, false, dst.REXBit == 1) // 32-bit operation, B=dst
	modrm := buildModRM(0x03, 5, dst.RegBits)             // reg=5 for SHR
	return []byte{rex, 0xD3, modrm}                       // D3 /5 = SHR r/m32, CL
}

// emitXchgReg32Reg32 emits: XCHG reg1_32, reg2_32
func emitXchgReg32Reg32(reg1 X86Reg, reg2 X86Reg) []byte {
	rex := buildREX(false, reg1.REXBit == 1, false, reg2.REXBit == 1) // 32-bit operation, R=reg1, B=reg2
	modrm := buildModRM(0x03, reg1.RegBits, reg2.RegBits)
	return []byte{rex, 0x87, modrm} // 87 /r = XCHG r/m32, r32
}

// emitRorReg32ByCl emits: ROR reg32, CL
func emitRorReg32ByCl(dst X86Reg) []byte {
	rex := buildREX(false, false, false, dst.REXBit == 1) // 32-bit operation, B=dst
	modrm := buildModRM(0x03, 1, dst.RegBits)             // reg=1 for ROR
	return []byte{rex, 0xD3, modrm}                       // D3 /1 = ROR r/m32, CL
}

// emitShlReg32ByCl emits: SHL reg32, CL
func emitShlReg32ByCl(dst X86Reg) []byte {
	rex := buildREX(false, false, false, dst.REXBit == 1) // 32-bit operation, B=dst
	modrm := buildModRM(0x03, 4, dst.RegBits)             // reg=4 for SHL
	return []byte{rex, 0xD3, modrm}                       // D3 /4 = SHL r/m32, CL
}

// emitShiftReg32ByCl emits: SHIFT reg32, CL with specified regField
// regField: 4=SHL, 5=SHR, 7=SAR, etc.
func emitShiftReg32ByCl(dst X86Reg, regField byte) []byte {
	rex := buildREX(false, false, false, dst.REXBit == 1) // 32-bit operation, B=dst
	modrm := buildModRM(0x03, regField, dst.RegBits)
	return []byte{rex, 0xD3, modrm} // D3 /n = SHIFT r/m32, CL
}

// emitXorRdxRdx emits: XOR RDX, RDX (zero RDX for unsigned division)
func emitXorRdxRdx() []byte {
	return []byte{0x48, 0x31, 0xD2} // 48 31 D2 = XOR RDX, RDX
}

// emitJeRel32 emits: JE rel32 (conditional jump equal)
func emitJeRel32() []byte {
	return []byte{0x0F, 0x84, 0, 0, 0, 0} // 0F 84 = JE rel32 (placeholder offset)
}

// emitPushRaxRdx emits PUSH RAX; PUSH RDX
func emitPushRaxRdx() []byte {
	return []byte{0x50, 0x52}
}

// emitMovReg32ToReg32 emits MOV dst32, src32
func emitMovReg32ToReg32(dst, src X86Reg) []byte {
	rex := byte(0x40) // REX for 32-bit operation
	if dst.REXBit == 1 {
		rex |= 0x04 // REX.R
	}
	if src.REXBit == 1 {
		rex |= 0x01 // REX.B
	}
	modrm := byte(0xC0 | (dst.RegBits << 3) | src.RegBits)
	return []byte{rex, 0x8B, modrm}
}

// emitXorEaxEax emits XOR EAX, EAX
func emitXorEaxEax() []byte {
	return []byte{0x31, 0xC0}
}

// emitMovsxdReg64 emits MOVSXD dst64, src32
func emitMovsxdReg64(dst, src X86Reg) []byte {
	rex := byte(0x48) // REX.W for 64-bit operation
	if dst.REXBit == 1 {
		rex |= 0x04 // REX.R
	}
	if src.REXBit == 1 {
		rex |= 0x01 // REX.B
	}
	modrm := byte(0xC0 | (dst.RegBits << 3) | src.RegBits)
	return []byte{rex, 0x63, modrm}
}

// emitCmov64 emits: CMOVcc r64_dst, r64_src (conditional move)
func emitCmov64(cc byte, dst, src X86Reg) []byte {
	rex := byte(0x48) // REX.W for 64-bit operation
	if dst.REXBit == 1 {
		rex |= 0x04 // REX.R
	}
	if src.REXBit == 1 {
		rex |= 0x01 // REX.B
	}
	modrm := byte(0xC0 | (dst.RegBits << 3) | src.RegBits)
	return []byte{rex, 0x0F, cc, modrm}
}

// emitShiftRegCl emits: D3 /subcode r/m64, CL
func emitShiftRegCl(dst X86Reg, subcode byte) []byte {
	rex := byte(0x48)
	if dst.REXBit == 1 {
		rex |= 0x01
	}
	modrm := buildModRM(0x03, subcode, dst.RegBits)
	return []byte{rex, 0xD3, modrm}
}

// emitShiftRegImm32 emits: C1 /subcode r/m32, imm8 (32-bit shift with immediate)
func emitShiftRegImm32(dst X86Reg, opcode, subcode byte, imm byte) []byte {
	rex := byte(0x40)
	if dst.REXBit == 1 {
		rex |= 0x01 // REX.B
	}
	modrm := byte(0xC0 | (subcode << 3) | dst.RegBits)
	return []byte{rex, opcode, modrm, imm}
}

// emitImulRegImm32 emits: IMUL dst, src, imm32 (32-bit multiply with immediate)
func emitImulRegImm32(dst, src X86Reg, imm uint32) []byte {
	rex := byte(0x40)
	if dst.REXBit == 1 {
		rex |= 0x04 // REX.R
	}
	if src.REXBit == 1 {
		rex |= 0x01 // REX.B
	}
	modrm := byte(0xC0 | (dst.RegBits << 3) | src.RegBits)
	result := []byte{rex, 0x69, modrm}
	result = append(result, encodeU32(imm)...)
	return result
}

// emitImulRegImm64 emits: IMUL dst, src, imm32 (64-bit multiply with 32-bit immediate)
func emitImulRegImm64(dst, src X86Reg, imm uint32) []byte {
	rex := byte(0x48) // REX.W for 64-bit operation
	if dst.REXBit == 1 {
		rex |= 0x04 // REX.R
	}
	if src.REXBit == 1 {
		rex |= 0x01 // REX.B
	}
	modrm := byte(0xC0 | (dst.RegBits << 3) | src.RegBits)
	result := []byte{rex, 0x69, modrm}
	result = append(result, encodeU32(imm)...)
	return result
}

// emitCmpRegImm32Force81 emits: CMP reg, imm32 using 0x81 opcode (forces 32-bit immediate form)
func emitCmpRegImm32Force81(reg X86Reg, imm int32) []byte {
	rex := byte(0x48) // REX.W for 64-bit operation
	if reg.REXBit == 1 {
		rex |= 0x01 // REX.B
	}
	modrm := byte(0xC0 | (7 << 3) | reg.RegBits) // reg=7 (CMP), rm=reg
	result := []byte{rex, 0x81, modrm}
	result = append(result, encodeU32(uint32(imm))...)
	return result
}

// emitAluRegImm8 emits: ALU reg64, imm8 (0x83 opcode with subcode)
func emitAluRegImm8(reg X86Reg, subcode byte, imm int8) []byte {
	rex := byte(0x48) // REX.W for 64-bit operation
	if reg.REXBit == 1 {
		rex |= 0x01 // REX.B
	}
	modrm := byte(0xC0 | (subcode << 3) | reg.RegBits)
	return []byte{rex, 0x83, modrm, byte(imm)}
}

// emitAluRegImm32 emits: ALU reg64, imm32 (0x81 opcode with subcode)
func emitAluRegImm32(reg X86Reg, subcode byte, imm int32) []byte {
	rex := byte(0x48) // REX.W for 64-bit operation
	if reg.REXBit == 1 {
		rex |= 0x01 // REX.B
	}
	modrm := byte(0xC0 | (subcode << 3) | reg.RegBits)
	result := []byte{rex, 0x81, modrm}
	result = append(result, encodeU32(uint32(imm))...)
	return result
}

// emitAluRegReg emits: ALU r/m64, r64 (reg2 is source, reg1 is destination)
func emitAluRegReg(reg1, reg2 X86Reg, subcode byte) []byte {
	var opcode byte
	switch subcode {
	case 0:
		opcode = 0x01 // ADD r/m64, r64
	case 4:
		opcode = 0x21 // AND r/m64, r64
	case 6:
		opcode = 0x31 // XOR r/m64, r64
	default:
		opcode = 0x01 // Default to ADD
	}

	rex := byte(0x48) // REX.W for 64-bit operation
	if reg2.REXBit == 1 {
		rex |= 0x04 // REX.R (source register)
	}
	if reg1.REXBit == 1 {
		rex |= 0x01 // REX.B (destination register)
	}
	modrm := byte(0xC0 | (reg2.RegBits << 3) | reg1.RegBits)
	return []byte{rex, opcode, modrm}
}

// emitLoadWithSIB emits: Load instruction with SIB addressing [base + index*1 + disp32]
// Used for patterns like MOVSX reg, [base + index + disp32]
func emitLoadWithSIB(dst, base, index X86Reg, disp32 int32, opcodeBytes []byte, is64bit bool, prefix byte) []byte {
	// Construct REX prefix: 0x40 base, W=1 for 64-bit, R/X/B for high registers
	var rex byte = 0x40
	if is64bit {
		rex |= 0x08 // W
	}
	if dst.REXBit != 0 {
		rex |= 0x04 // R
	}
	if index.REXBit != 0 {
		rex |= 0x02 // X (as SIB.index)
	}
	if base.REXBit != 0 {
		rex |= 0x01 // B (as SIB.base)
	}

	// ModRM: mod=10 (disp32), reg=dst, rm=100 (SIB)
	modRM := byte(0x02<<6) | (dst.RegBits << 3) | 0x04
	// SIB: scale=0(1), index=index, base=base
	sib := byte(0<<6) | (index.RegBits << 3) | base.RegBits

	// disp32 as little-endian
	dispBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(dispBytes, uint32(disp32))

	var code []byte
	if prefix != 0 {
		code = append(code, prefix)
	}
	code = append(code, rex)
	code = append(code, opcodeBytes...)
	code = append(code, modRM, sib)
	code = append(code, dispBytes...)

	return code
}

// emitStoreWithSIB emits: Store instruction with SIB addressing [base + index*1 + disp32]
// Used for patterns like MOV [base + index + disp32], src
func emitStoreWithSIB(src, base, index X86Reg, disp32 int32, size int) []byte {
	var buf []byte

	// 1) For 16-bit word, add operand-size override prefix
	if size == 2 {
		buf = append(buf, 0x66)
	}

	// 2) Compute REX prefix
	rex := byte(0x40)
	if size == 8 {
		rex |= 0x08 // REX.W
	}
	if src.REXBit == 1 {
		rex |= 0x04 // REX.R
	}
	if index.REXBit == 1 {
		rex |= 0x02 // REX.X
	}
	if base.REXBit == 1 {
		rex |= 0x01 // REX.B
	}
	if rex != 0x40 {
		buf = append(buf, rex)
	}

	// 3) Select opcode: 0x88 for byte, 0x89 for word/dword/qword
	movOp := byte(0x89)
	if size == 1 {
		movOp = 0x88
	}
	buf = append(buf, movOp)

	// 4) ModR/M + SIB + disp32
	//    Mod=10 (disp32), Reg=src, RM=100 (SIB)
	modrm := byte((2 << 6) | (src.RegBits << 3) | 0x04)
	//    scale=0×1, index=index, base=base
	sib := byte((0 << 6) | (index.RegBits << 3) | (base.RegBits & 0x07))
	buf = append(buf, modrm, sib)

	// disp32 as little-endian
	buf = append(buf,
		byte(disp32), byte(disp32>>8),
		byte(disp32>>16), byte(disp32>>24),
	)
	return buf
}

// emitCmpRegImm8 emits: CMP reg64, imm8
func emitCmpRegImm8(reg X86Reg, imm int8) []byte {
	rex := byte(0x48) // REX.W
	if reg.REXBit == 1 {
		rex |= 0x01 // REX.B
	}
	modrm := byte(0xF8 | reg.RegBits) // Mod=11, Reg=7 (CMP), RM=reg
	return []byte{rex, 0x83, modrm, byte(imm)}
}

// emitTestRegImm32 emits: TEST reg64, imm32
func emitTestRegImm32(reg X86Reg, imm uint32) []byte {
	rex := byte(0x48) // REX.W
	if reg.REXBit == 1 {
		rex |= 0x01 // REX.B
	}
	modrm := byte(0xC0 | reg.RegBits) // Mod=11, Reg=0 (TEST), RM=reg
	buf := []byte{rex, 0xF7, modrm}
	buf = append(buf, byte(imm), byte(imm>>8), byte(imm>>16), byte(imm>>24))
	return buf
}

// emitTrap emits: INT3 (0x0F 0x0B - breakpoint instruction)
func emitTrap() []byte {
	return []byte{0x0F, 0x0B}
}

// emitNop emits: NOP (0x90 - no operation)
func emitNop() []byte {
	return []byte{0x90}
}

// emitJmpWithPlaceholder emits: JMP rel32 with placeholder bytes 0xFEFEFEFE
func emitJmpWithPlaceholder() []byte {
	return []byte{0xE9, 0xFE, 0xFE, 0xFE, 0xFE}
}

// emitMovImmToReg64WithManualREX emits: MOV reg64, imm64 using exact manual REX construction
func emitMovImmToReg64WithManualREX(r X86Reg, imm uint64) []byte {
	// Manual REX construction matching original code
	rex := byte(0x48)
	if r.REXBit == 1 {
		rex |= 0x01 // set only the B bit for r8–r15
	}
	movOp := byte(0xB8 + r.RegBits)
	immBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(immBytes, imm)
	code := append([]byte{rex, movOp}, immBytes...)
	return code
}

// emitCmpRegImm32WithManualConstruction emits: CMP reg64, imm32 using exact manual construction
func emitCmpRegImm32WithManualConstruction(r X86Reg, imm int32) []byte {
	// Manual REX construction matching original code
	rex := byte(0x48)
	if r.REXBit == 1 {
		rex |= 0x01
	}

	// CMP r64, imm32 → opcode 0x81 /7 id
	modrm := byte(0xC0 | (0x7 << 3) | r.RegBits)

	return []byte{
		rex, 0x81, modrm,
		byte(imm), byte(imm >> 8), byte(imm >> 16), byte(imm >> 24),
	}
}

// emitJccWithPlaceholder emits: Jcc rel32 with 4-byte placeholder
func emitJccWithPlaceholder(jcc byte) []byte {
	return []byte{0x0F, jcc, 0, 0, 0, 0}
}

// emitPushRegWithManualConstruction emits: PUSH reg using exact manual construction
func emitPushRegWithManualConstruction(reg X86Reg) []byte {
	var buf []byte
	// Manual construction matching original code
	if reg.REXBit == 1 {
		buf = append(buf, 0x41)
	}
	buf = append(buf, 0x50|reg.RegBits)
	return buf
}

// emitMovRegToRegWithManualConstruction emits: MOV dst, src using exact manual construction (0x8B opcode)
func emitMovRegToRegWithManualConstruction(dst, src X86Reg) []byte {
	// Manual construction matching original code (opcode 0x8B = MOV r64, r/m64)
	rex := byte(0x48) // REX.W
	if dst.REXBit == 1 {
		rex |= 0x04 // REX.R
	}
	if src.REXBit == 1 {
		rex |= 0x01 // REX.B
	}
	modrm := byte(0xC0 | (dst.RegBits << 3) | src.RegBits)
	return []byte{rex, 0x8B, modrm}
}

// emitMovRegToRegWithReversedOpcode emits: MOV dst, src using 0x89 opcode (reversed operand encoding)
func emitMovRegToRegWithReversedOpcode(dst, src X86Reg) []byte {
	// Manual construction with 0x89 opcode (MOV r/m64, r64) - src and dst roles swapped in ModR/M
	rex := byte(0x48) // REX.W
	if src.REXBit == 1 {
		rex |= 0x04 // REX.R (source reg is in reg field for 0x89)
	}
	if dst.REXBit == 1 {
		rex |= 0x01 // REX.B (dest reg is in r/m field for 0x89)
	}
	modrm := byte(0xC0 | (src.RegBits << 3) | dst.RegBits) // swapped: src in reg field, dst in r/m
	return []byte{rex, 0x89, modrm}
}

// emitMovRegToRegForMinU emits: MOV dst, src specifically for MIN_U with consistent 0x89 opcode
func emitMovRegToRegForMinU(dst, src X86Reg) []byte {
	// MIN_U consistently uses 0x89 opcode (MOV r/m64, r64)
	rex := byte(0x48) // REX.W
	if src.REXBit == 1 {
		rex |= 0x04 // REX.R
	}
	if dst.REXBit == 1 {
		rex |= 0x01 // REX.B
	}
	modrm := byte(0xC0 | (src.RegBits << 3) | dst.RegBits) // src in reg field, dst in r/m
	return []byte{rex, 0x89, modrm}
}

// emitMovRegToRegSameRegForMinU emits: MOV reg, reg for same register (special case for MIN_U)
func emitMovRegToRegSameRegForMinU(reg X86Reg) []byte {
	// Special case for MIN_U when moving register to itself - use 0x89 opcode with same reg in both fields
	rex := byte(0x48) // REX.W
	if reg.REXBit == 1 {
		rex |= 0x04 // REX.R
		rex |= 0x01 // REX.B
	}
	modrm := byte(0xC0 | (reg.RegBits << 3) | reg.RegBits) // same reg in both fields
	return []byte{rex, 0x89, modrm}
}

// emitMovRegToRegSameReg emits: MOV reg, reg (same register) - special case for optimizations
func emitMovRegToRegSameReg(reg X86Reg) []byte {
	// Special case: MOV reg, reg using 0x89 opcode
	rex := byte(0x48) // REX.W
	if reg.REXBit == 1 {
		rex |= 0x05 // REX.R + REX.B (both set for same register)
	}
	modrm := byte(0xC0 | (reg.RegBits << 3) | reg.RegBits) // same reg in both fields
	return []byte{rex, 0x89, modrm}
}

// emitAddRegImm32WithManualConstruction emits: ADD reg, imm32 using exact manual construction
func emitAddRegImm32WithManualConstruction(reg X86Reg, imm uint64) []byte {
	// Manual construction matching original code but using proper buildREX
	// For 32-bit operations, REX.W should be false
	rex := buildREX(false, false, false, reg.REXBit == 1)
	modrm := buildModRM(0x03, 0, reg.RegBits) // /0 for ADD

	imm32 := make([]byte, 4)
	binary.LittleEndian.PutUint32(imm32, uint32(imm))

	result := []byte{rex, 0x81, modrm}
	result = append(result, imm32...)
	return result
}

// emitAddRegImm32ForJumpIndirect emits: ADD reg, imm32 with 64-bit REX for JUMP_IND context
func emitAddRegImm32ForJumpIndirect(reg X86Reg, imm uint64) []byte {
	// Manual construction matching original JUMP_IND code pattern (64-bit REX)
	rex := byte(0x48) // REX.W = 1 for 64-bit operation size
	if reg.REXBit == 1 {
		rex |= 0x01 // REX.B
	}
	modrm := byte(0xC0 | (0 << 3) | reg.RegBits) // /0 for ADD
	imm32 := make([]byte, 4)
	binary.LittleEndian.PutUint32(imm32, uint32(imm))

	result := []byte{rex, 0x81, modrm}
	result = append(result, imm32...)
	return result
}

// emitPushRegFor64Bit emits: PUSH reg using manual 64-bit prefix construction
func emitPushRegFor64Bit(reg X86Reg) []byte {
	// Manual construction for 64-bit PUSH - different from the jumpIndTempReg version
	rex := byte(0x48)
	if reg.REXBit == 1 {
		rex |= 0x01
	}
	pushOp := byte(0x50 | reg.RegBits)
	return []byte{rex, pushOp}
}

// emitMovRegToRegForLoadImmJumpIndirect emits: MOV dst, src with specific REX construction for LoadImmJumpIndirect
func emitMovRegToRegForLoadImmJumpIndirect(dst, src X86Reg) []byte {
	// Manual construction matching original code for LoadImmJumpIndirect
	rex := byte(0x48) // REX.W
	if dst.REXBit == 1 {
		rex |= 0x04 // REX.R: extend the reg field for dst
	}
	if src.REXBit == 1 {
		rex |= 0x01 // REX.B: extend the rm field for src
	}
	// mod=11 (register-direct), reg=dst, rm=src
	modrm := byte(0xC0 | (dst.RegBits << 3) | src.RegBits)

	return []byte{rex, 0x8B, modrm}
}

// emitAddRegImm32ForLoadImmJumpIndirect emits: ADD reg, imm32 with specific construction for LoadImmJumpIndirect
func emitAddRegImm32ForLoadImmJumpIndirect(reg X86Reg, imm uint64) []byte {
	// Manual construction matching original code for LoadImmJumpIndirect
	rex := byte(0x48)
	if reg.REXBit == 1 {
		rex |= 0x01
	}
	// ADD r/m64, imm32 (reg=0)
	modrm := byte(0xC0 | (0 << 3) | reg.RegBits)
	imm32 := make([]byte, 4)
	binary.LittleEndian.PutUint32(imm32, uint32(imm))

	result := []byte{rex, 0x81, modrm}
	result = append(result, imm32...)
	return result
}

// emitAndRegImm32WithFFs emits: AND reg, 0xFFFFFFFF using exact manual construction
func emitAndRegImm32WithFFs(reg X86Reg) []byte {
	// Manual construction matching original code for zero-extend
	rex := byte(0x48)
	if reg.REXBit == 1 {
		rex |= 0x01
	}
	// AND r/m64, imm32 (reg=4)
	modrm := byte(0xC0 | (4 << 3) | reg.RegBits)

	result := []byte{rex, 0x81, modrm}
	result = append(result, []byte{0xFF, 0xFF, 0xFF, 0xFF}...)
	return result
}

// Helper functions for initDJumpFunc - these match the exact manual constructions

// emitCmpReg32WithImm32ForDJump emits: CMP r/m32, imm32 using 0x40 REX prefix
func emitCmpReg32WithImm32ForDJump(reg X86Reg, imm uint32) []byte {
	// Manual construction: REX prefix 0x40 + B bit
	rex := byte(0x40)
	if reg.REXBit == 1 {
		rex |= 0x01 // REX.B
	}
	modrm := byte(0xC0 | (7 << 3) | reg.RegBits)

	result := []byte{rex, 0x81, modrm}
	immBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(immBytes, imm)
	result = append(result, immBytes...)
	return result
}

// emitCmpRegImm8ForDJump emits: CMP r/m64, imm8 using 0x83 opcode
func emitCmpRegImm8ForDJump(reg X86Reg, imm byte) []byte {
	// Manual construction: REX.W + REX.B
	rex := byte(0x48)
	if reg.REXBit == 1 {
		rex |= 0x01
	}
	modrm := byte(0xF8 | reg.RegBits)
	return []byte{rex, 0x83, modrm, imm}
}

// emitCmpRegImm32ForDJump emits: CMP r/m64, imm32 using 0x81 opcode
func emitCmpRegImm32ForDJump(reg X86Reg, imm uint32) []byte {
	// Manual construction: REX.W + REX.B
	rex := byte(0x48)
	if reg.REXBit == 1 {
		rex |= 0x01
	}
	modrm := byte(0xF8 | reg.RegBits)

	result := []byte{rex, 0x81, modrm}
	immBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(immBytes, imm)
	result = append(result, immBytes...)
	return result
}

// emitTestRegImm32ForDJump emits: TEST r/m64, imm32 using 0xF7 opcode
func emitTestRegImm32ForDJump(reg X86Reg, imm uint32) []byte {
	// Manual construction: REX.W + REX.B
	rex := byte(0x48)
	if reg.REXBit == 1 {
		rex |= 0x01
	}
	modrm := byte(0xC0 | reg.RegBits)

	result := []byte{rex, 0xF7, modrm}
	immBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(immBytes, imm)
	result = append(result, immBytes...)
	return result
}

// emitSarRegImm8ForDJump emits: SAR r/m64, imm8 using 0xC1 /7 opcode
func emitSarRegImm8ForDJump(reg X86Reg, imm byte) []byte {
	// Manual construction: REX.W + REX.B
	rex := byte(0x48)
	if reg.REXBit == 1 {
		rex |= 0x01
	}
	// /7 → reg=7，mod=11（0xC0），rm=reg.RegBits
	modrm := byte(0xC0 | (7 << 3) | reg.RegBits)
	return []byte{rex, 0xC1, modrm, imm}
}

// emitSubRegImm8ForDJump emits: SUB r/m64, imm8 using 0x83 /5 opcode
func emitSubRegImm8ForDJump(reg X86Reg, imm byte) []byte {
	// Manual construction: REX.W + REX.B
	rex := byte(0x48)
	if reg.REXBit == 1 {
		rex |= 0x01
	}
	modrm := byte(0xC0 | (5 << 3) | reg.RegBits)
	return []byte{rex, 0x83, modrm, imm}
}

// emitImulRegRegImm32ForDJump emits: IMUL r64, r/m64, imm32 using 0x69 opcode
func emitImulRegRegImm32ForDJump(reg X86Reg, imm uint32) []byte {
	// Manual construction: REX.W + REX.R + REX.B
	rex := byte(0x48) // W=1
	if reg.REXBit == 1 {
		rex |= 0x05 // REX.R=1, REX.B=1
	}
	// mod=11, reg=reg.RegBits, rm=reg.RegBits
	modrm := byte(0xC0 | (reg.RegBits << 3) | reg.RegBits)

	result := []byte{rex, 0x69, modrm}
	immBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(immBytes, imm)
	result = append(result, immBytes...)
	return result
}

// emitPushRAX emits: PUSH RAX
func emitPushRAX() []byte {
	return []byte{0x50}
}

// emitLeaRAXRip emits: LEA RAX, [RIP+0]
func emitLeaRAXRip() []byte {
	return []byte{0x48, 0x8D, 0x05, 0x00, 0x00, 0x00, 0x00}
}

// emitAddRegRegForDJump emits: ADD r/m64, r/m64 using 0x01 opcode
func emitAddRegRegForDJump(dst X86Reg) []byte {
	// Manual construction: REX.W + REX.B (adding RAX to dst)
	rex := byte(0x48)
	if dst.REXBit == 1 {
		rex |= 0x01 // REX.B
	}
	modrm := byte(0xC0 | (0 << 3) | dst.RegBits) // RAX is reg=0
	return []byte{rex, 0x01, modrm}
}

// emitPopRAX emits: POP RAX
func emitPopRAX() []byte {
	return []byte{0x58}
}

// emitAddRegImm32WithPlaceholderForDJump emits: ADD r/m64, imm32 with DEADBEEF placeholder
func emitAddRegImm32WithPlaceholderForDJump(reg X86Reg) []byte {
	// Manual construction: REX.W + REX.B
	rex := byte(0x48)
	if reg.REXBit == 1 {
		rex |= 0x01 // REX.B
	}
	modrm := byte(0xC0 | (0 << 3) | reg.RegBits)
	return []byte{rex, 0x81, modrm, 0xEF, 0xBE, 0xAD, 0xDE}
}

// emitJmpRegForDJump emits: JMP r/m64 using 0xFF /4 opcode
func emitJmpRegForDJump(reg X86Reg) []byte {
	// Manual construction: REX.W + REX.B
	rex := byte(0x48)
	if reg.REXBit == 1 {
		rex |= 0x01
	}
	modrm := byte(0xE0 | reg.RegBits) // /4 = 0b100 shifted to reg field = 0xE0
	return []byte{rex, 0xFF, modrm}
}

// emitPopRegUd2ForDJump emits: POP reg; UD2 (panic stub)
func emitPopRegUd2ForDJump(reg X86Reg) []byte {
	// Manual construction: REX + POP + UD2
	rex := byte(0x48)
	if reg.REXBit == 1 {
		rex |= 0x01
	}
	popOp := byte(0x58 | reg.RegBits)
	return []byte{rex, popOp, 0x0F, 0x0B}
}

// emitPopRegForDJump emits: POP reg for handler jumps
func emitPopRegForDJump(reg X86Reg) []byte {
	// Manual construction: REX + POP
	rex := byte(0x48)
	if reg.REXBit == 1 {
		rex |= 0x01
	}
	popOp := byte(0x58 | reg.RegBits)
	return []byte{rex, popOp}
}

// emitStoreImmIndWithSIB emits: MOV [base + index*1 + disp32], imm (with SIB addressing)
func emitStoreImmIndWithSIB(idx X86Reg, base X86Reg, disp uint32, imm []byte, opcode byte, prefix byte) []byte {
	var buf []byte

	// Add prefix if specified (e.g., 0x66 for 16-bit)
	if prefix != 0 {
		buf = append(buf, prefix)
	}

	// REX prefix: W=0, R=0, X=idx.REXBit, B=base.REXBit
	rex := byte(0x40)
	if idx.REXBit == 1 {
		rex |= 0x02 // REX.X for SIB.index
	}
	if base.REXBit == 1 {
		rex |= 0x01 // REX.B for SIB.base
	}

	buf = append(buf, rex, opcode)

	// ModRM: mod=10 (disp32), reg=000 (/0), r/m=100 (SIB follows)
	buf = append(buf, 0x84)

	// SIB: scale=0(×1)=00, index=idx.RegBits, base=base.RegBits
	sib := byte((0 << 6) | (idx.RegBits << 3) | (base.RegBits & 0x07))
	buf = append(buf, sib)

	// disp32, little-endian
	buf = append(buf,
		byte(disp), byte(disp>>8),
		byte(disp>>16), byte(disp>>24),
	)

	// immediate value
	buf = append(buf, imm...)

	return buf
}

// emitStoreImmIndU32WithRegs emits: MOV [base + index*1 + disp32], imm32 with custom regs
func emitStoreImmIndU32WithRegs(idx X86Reg, base X86Reg, disp uint32, imm32 uint32) []byte {
	immBytes := []byte{
		byte(imm32), byte(imm32 >> 8),
		byte(imm32 >> 16), byte(imm32 >> 24),
	}
	return emitStoreImmIndWithSIB(idx, base, disp, immBytes, 0xC7, 0)
}

// emitStoreImmWithBaseSIB emits: MOV [base + disp32], imm (with optional prefix)
func emitStoreImmWithBaseSIB(base X86Reg, disp uint32, imm []byte, opcode byte, prefix byte) []byte {
	var buf []byte

	// Add prefix if specified (e.g., 0x66 for 16-bit)
	if prefix != 0 {
		buf = append(buf, prefix)
	}

	// REX prefix (64-bit mode), only used to extend r8–r15
	rex := byte(0x40)
	if base.REXBit != 0 {
		rex |= 0x01
	}

	buf = append(buf, rex, opcode)

	// ModRM: mod=10 (disp32), reg=000 (MOV sub-opcode), r/m=100 (SIB follows)
	buf = append(buf, 0x84)

	// SIB: scale=0 (×1), index=100 (none), base=base.RegBits
	sib := byte(0x24 | (base.RegBits & 0x07))
	buf = append(buf, sib)

	// disp32, little-endian
	buf = append(buf, encodeU32(disp)...)

	// immediate value
	buf = append(buf, imm...)

	return buf
}

// emitLoadWithBaseSIB emits: MOV dst, [base + disp32] with SIB addressing
func emitLoadWithBaseSIB(dst X86Reg, base X86Reg, disp uint32, opcodes []byte, rexW bool) []byte {
	// Construct REX prefix: 0100WRXB
	rex := byte(0x40)
	if rexW {
		rex |= 0x08 // REX.W
	}
	if dst.REXBit == 1 {
		rex |= 0x04 // REX.R extends ModRM.reg
	}
	if base.REXBit == 1 {
		rex |= 0x01 // REX.B extends ModRM.r/m (base)
	}

	// ModRM: mod=10 (disp32), reg=dst.RegBits, r/m=100 (SIB follows)
	modrm := byte(0x80 | (dst.RegBits << 3) | 0x04)
	// SIB: scale=0 (×1), index=100(none), base=base.RegBits
	sib := byte(0x24 | (base.RegBits & 0x07))

	// disp32
	dispBytes := encodeU32(disp)

	// Concatenate
	buf := make([]byte, 0, 1+len(opcodes)+2+1+4)
	buf = append(buf, rex)
	buf = append(buf, opcodes...)
	buf = append(buf, modrm, sib)
	buf = append(buf, dispBytes...)
	return buf
}

// emitStoreWithBaseSIB emits: MOV [base + disp32], src with SIB addressing and optional prefix
func emitStoreWithBaseSIB(src X86Reg, base X86Reg, disp uint32, opcode byte, prefix byte, rexW bool) []byte {
	// REX prefix: 0100WRXB
	rex := byte(0x40)
	if rexW {
		rex |= 0x08 // REX.W
	}
	if src.REXBit == 1 {
		rex |= 0x04 // REX.R extends ModRM.reg
	}
	if base.REXBit == 1 {
		rex |= 0x01 // REX.B extends ModRM.r/m (base)
	}

	// ModRM: mod=10 (disp32), reg=src.RegBits, r/m=100 (SIB follows)
	modrm := byte(0x80 | (src.RegBits << 3) | 0x04)
	// SIB: scale=0 (×1), index=100(none), base=base.RegBits
	sib := byte(0x24 | (base.RegBits & 0x07))

	// disp32, little-endian
	dispBytes := encodeU32(disp)

	buf := make([]byte, 0, 1+1+2+1+4)
	if prefix != 0 {
		buf = append(buf, prefix)
	}
	buf = append(buf, rex, opcode, modrm, sib)
	buf = append(buf, dispBytes...)
	return buf
}

// emitCallReg emits: CALL reg
func emitCallReg(reg X86Reg) []byte {
	var result []byte
	if reg.REXBit == 1 {
		rex := buildREX(false, false, false, true)
		result = append(result, rex)
	}
	modrm := buildModRM(0x03, 0x02, reg.RegBits) // /2 for CALL
	result = append(result, 0xFF, modrm)
	return result
}

// emitMovImm32ToReg32 emits: MOV reg32, imm32
func emitMovImm32ToReg32(dst X86Reg, val uint32) []byte {
	var result []byte
	if dst.REXBit == 1 {
		rex := buildREX(false, false, false, true)
		result = append(result, rex)
	}
	opcode := byte(0xB8 + dst.RegBits)
	result = append(result, opcode)
	result = append(result, encodeU32(val)...)
	return result
}
