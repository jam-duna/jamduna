package pvm

import (
	"encoding/binary"
)

// ================================================================================================
// Data-Driven PVM Bytecode to x86 Mapping
// ================================================================================================

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
	BRANCH_EQ_IMM:   generateBranchImm(X86_OP2_JE),
	BRANCH_NE_IMM:   generateBranchImm(X86_OP2_JNE),
	BRANCH_LT_U_IMM: generateBranchImm(X86_OP2_JB),
	BRANCH_LE_U_IMM: generateBranchImm(X86_OP2_JBE),
	BRANCH_GE_U_IMM: generateBranchImm(X86_OP2_JAE),
	BRANCH_GT_U_IMM: generateBranchImm(X86_OP2_JA),
	BRANCH_LT_S_IMM: generateBranchImm(X86_OP2_JL),
	BRANCH_LE_S_IMM: generateBranchImm(X86_OP2_JLE),
	BRANCH_GE_S_IMM: generateBranchImm(X86_OP2_JGE),
	BRANCH_GT_S_IMM: generateBranchImm(X86_OP2_JG),
	BRANCH_EQ:       generateCompareBranch(X86_OP2_JE),
	BRANCH_NE:       generateCompareBranch(X86_OP2_JNE),
	BRANCH_LT_U:     generateCompareBranch(X86_OP2_JB),
	BRANCH_LT_S:     generateCompareBranch(X86_OP2_JL),
	BRANCH_GE_U:     generateCompareBranch(X86_OP2_JAE),
	BRANCH_GE_S:     generateCompareBranch(X86_OP2_JGE),

	// A.5.2. Instructions with Arguments of One Immediate. InstructionI1

	// A.5.3. Instructions with Arguments of One Register and One Extended Width Immediate.
	LOAD_IMM_64: generateLoadImm64,

	// A.5.4. Instructions with Arguments of Two Immediates.
	STORE_IMM_U8: generateStoreImmGeneric(
		X86_OP_MOV_RM_IMM8, // MOV r/m8, imm8
		X86_NO_PREFIX,      // no prefix
		func(v uint64) []byte { return []byte{byte(v)} },
	),
	STORE_IMM_U16: generateStoreImmGeneric(
		X86_OP_MOV_RM_IMM, // MOV r/m16, imm16 (with 0x66 prefix)
		X86_PREFIX_66,
		func(v uint64) []byte { return encodeU16(uint16(v)) },
	),
	STORE_IMM_U32: generateStoreImmGeneric(
		X86_OP_MOV_RM_IMM, // MOV r/m32, imm32
		X86_NO_PREFIX,
		func(v uint64) []byte { return encodeU32(uint32(v)) },
	),
	STORE_IMM_U64: generateStoreImmU64,

	// A.5.6. Instructions with Arguments of One Register & Two Immediates.
	LOAD_IMM: generateLoadImm32,
	LOAD_U8:  generateLoadWithBase([]byte{X86_PREFIX_0F, X86_OP2_MOVZX_R_RM8}, false),  // MOVZX r32, byte ptr [Base+disp32]
	LOAD_I8:  generateLoadWithBase([]byte{X86_PREFIX_0F, X86_OP2_MOVSX_R_RM8}, true),   // MOVSX r32, byte ptr [Base+disp32]
	LOAD_U16: generateLoadWithBase([]byte{X86_PREFIX_0F, X86_OP2_MOVZX_R_RM16}, false), // MOVZX r32, word ptr [Base+disp32]

	LOAD_I16:  generateLoadWithBase([]byte{X86_PREFIX_0F, X86_OP2_MOVSX_R_RM16}, true), // MOVSX r32, word ptr [Base+disp32]
	LOAD_U32:  generateLoadWithBase([]byte{X86_OP_MOV_R_RM}, false),                    // MOV r32, dword ptr [Base+disp32]
	LOAD_I32:  generateLoadWithBase([]byte{X86_OP_MOVSXD}, true),                       // MOVSXD r64, dword ptr [Base+disp32]
	LOAD_U64:  generateLoadWithBase([]byte{X86_OP_MOV_R_RM}, true),                     // MOV r64, qword ptr [Base+disp32],
	STORE_U8:  generateStoreWithBase(X86_OP_MOV_RM8_R8, X86_NO_PREFIX, false),          // MOV byte ptr [Base+disp32], r8
	STORE_U16: generateStoreWithBase(X86_OP_MOV_RM_R, X86_PREFIX_66, false),            // MOV word ptr [Base+disp32], r16
	STORE_U32: generateStoreWithBase(X86_OP_MOV_RM_R, X86_NO_PREFIX, false),            // MOV dword ptr [Base+disp32], r32
	STORE_U64: generateStoreWithBase(X86_OP_MOV_RM_R, X86_NO_PREFIX, true),             // MOV qword ptr [Base+disp32], r64  (REX.W=1)

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
	LOAD_IND_U8:       generateLoadInd(X86_NO_PREFIX, LOAD_IND_U8, false),
	LOAD_IND_I8:       generateLoadIndSignExtend(X86_NO_PREFIX, LOAD_IND_I8, false),
	LOAD_IND_U16:      generateLoadInd(X86_PREFIX_66, LOAD_IND_U16, false),
	LOAD_IND_I16:      generateLoadIndSignExtend(X86_PREFIX_66, LOAD_IND_I16, false),
	LOAD_IND_U32:      generateLoadInd(X86_NO_PREFIX, LOAD_IND_U32, false),
	LOAD_IND_I32:      generateLoadIndSignExtend(X86_NO_PREFIX, LOAD_IND_I32, true),
	LOAD_IND_U64:      generateLoadInd(X86_NO_PREFIX, LOAD_IND_U64, true),
	ADD_IMM_32:        generateBinaryImm32,
	AND_IMM:           generateImmBinaryOp64(X86_REG_AND),
	XOR_IMM:           generateImmBinaryOp64(X86_REG_XOR),
	OR_IMM:            generateImmBinaryOp64(X86_REG_OR),
	MUL_IMM_32:        generateImmMulOp32,
	SET_LT_U_IMM:      generateImmSetCondOp32New(X86_OP2_SETB), // SETB / below unsigned
	SET_LT_S_IMM:      generateImmSetCondOp32New(X86_OP2_SETL), // SETL / below signed
	SET_GT_U_IMM:      generateImmSetCondOp32New(X86_OP2_SETA), // SETA / above unsigned
	SET_GT_S_IMM:      generateImmSetCondOp32New(X86_OP2_SETG), // SETG / above signed
	SHLO_L_IMM_32:     generateImmShiftOp32SHLO(X86_REG_SHL),
	SHLO_R_IMM_32:     generateImmShiftOp32(X86_OP_GROUP2_RM_IMM8, X86_REG_SHR, false),
	SHLO_L_IMM_ALT_32: generateImmShiftOp32Alt(X86_REG_SHL),
	SHLO_R_IMM_ALT_32: generateImmShiftOp32(X86_OP_GROUP2_RM_IMM8, X86_REG_SHR, true),
	SHAR_R_IMM_ALT_32: generateImmShiftOp32(X86_OP_GROUP2_RM_IMM8, X86_REG_SAR, true),
	NEG_ADD_IMM_32:    generateNegAddImm32,
	CMOV_IZ_IMM:       generateCmovImm(true),
	CMOV_NZ_IMM:       generateCmovImm(false),
	ADD_IMM_64:        generateImmBinaryOp64(X86_REG_ADD),
	MUL_IMM_64:        generateImmMulOp64,
	SHLO_L_IMM_64:     generateImmShiftOp64(X86_OP_GROUP2_RM_IMM8, X86_REG_SHL),
	SHLO_R_IMM_64:     generateImmShiftOp64(X86_OP_GROUP2_RM_IMM8, X86_REG_SHR),
	SHAR_R_IMM_32:     generateImmShiftOp32(X86_OP_GROUP2_RM_IMM8, X86_REG_SAR, false),
	SHAR_R_IMM_64:     generateImmShiftOp64(X86_OP_GROUP2_RM_IMM8, X86_REG_SAR),
	NEG_ADD_IMM_64:    generateNegAddImm64,
	SHLO_L_IMM_ALT_64: generateImmShiftOp64Alt(X86_REG_SHL),
	SHLO_R_IMM_ALT_64: generateImmShiftOp64Alt(X86_REG_SHR),
	SHAR_R_IMM_ALT_64: generateImmShiftOp64Alt(X86_REG_SAR),
	ROT_R_64_IMM:      generateImmShiftOp64(X86_OP_GROUP2_RM_IMM8, X86_REG_ROR),
	ROT_R_64_IMM_ALT:  generateImmShiftOp64(X86_OP_GROUP2_RM_IMM8, X86_REG_ROR),
	ROT_R_32_IMM_ALT:  generateImmShiftOp32(X86_OP_GROUP2_RM_IMM8, X86_REG_ROR, true),
	ROT_R_32_IMM:      generateRotateRight32Imm,

	// A.5.13. Instructions with Arguments of Three Registers.
	ADD_32: generateBinaryOp32(X86_OP_ADD_RM_R), // add
	SUB_32: generateBinaryOp32(X86_OP_SUB_RM_R), // sub

	MUL_32:        generateMul32,
	DIV_U_32:      generateDivUOp32,
	DIV_S_32:      generateDivSOp32,
	REM_U_32:      generateRemUOp32,
	REM_S_32:      generateRemSOp32,
	SHLO_L_32:     generateSHLO_L_32(),
	SHLO_R_32:     generateSHLO_R_32(),
	SHAR_R_32:     generateShiftOp32SHAR(),
	ADD_64:        generateBinaryOp64(X86_OP_ADD_RM_R), // add
	SUB_64:        generateBinaryOp64(X86_OP_SUB_RM_R), // sub
	MUL_64:        generateMul64,                       // imul
	DIV_U_64:      generateDivUOp64,
	DIV_S_64:      generateDivSOp64,
	REM_U_64:      generateRemUOp64,
	REM_S_64:      generateRemSOp64,
	SHLO_L_64:     generateSHLO_L_64(),
	SHLO_R_64:     generateShiftOp64B(X86_OP_GROUP2_RM_CL, X86_REG_SHR),
	SHAR_R_64:     generateShiftOp64(X86_OP_GROUP2_RM_CL, X86_REG_SAR),
	AND:           generateBinaryOp64(X86_OP_AND_RM_R),
	XOR:           generateBinaryOp64(X86_OP_XOR_RM_R),
	OR:            generateBinaryOp64(X86_OP_OR_RM_R),
	MUL_UPPER_S_S: generateMulUpperOp64("signed"),
	MUL_UPPER_U_U: generateMulUpperOp64("unsigned"),
	MUL_UPPER_S_U: generateMulUpperOp64("mixed"),
	SET_LT_U:      generateSetCondOp64(X86_OP2_SETB),
	SET_LT_S:      generateSetCondOp64(X86_OP2_SETL),
	CMOV_IZ:       generateCmovOp64(X86_OP2_CMOVE),
	CMOV_NZ:       generateCmovOp64(X86_OP2_CMOVNE),
	ROT_L_64:      generateROTL64(),
	ROT_L_32:      generateShiftOp32(X86_REG_ROL),
	ROT_R_64:      generateShiftOp64(X86_OP_GROUP2_RM_CL, X86_REG_ROR),
	ROT_R_32:      generateROT_R_32(),
	AND_INV:       generateAndInvOp64,
	OR_INV:        generateOrInvOp64,
	XNOR:          generateXnorOp64,
	MAX:           generateMax(),
	MAX_U:         generateMaxU(),
	MIN:           generateMin(),
	MIN_U:         generateMinU(),
}

// use store the original memory address for real memory
// this register is used as base for register dump

// encodeMovImm encodes: mov rX, imm64
func encodeMovImm(regIdx int, imm uint64) []byte {
	reg := regInfoList[regIdx]
	rex := buildREX(true, false, false, reg.REXBit == 1)
	// opcode = B8 + low 3 bits
	op := byte(X86_OP_MOV_R_IMM + reg.RegBits)

	immBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(immBytes, imm)
	return append([]byte{rex, op}, immBytes...)
}

func encodeMovRegToMem(srcIdx, baseIdx int, offset byte) []byte {
	src := regInfoList[srcIdx]
	base := regInfoList[baseIdx]

	rex := buildREX(true, src.REXBit == 1, false, base.REXBit == 1)

	if base.RegBits == 4 { // R12
		// Must use SIB encoding when rm=4 (R12)
		modrm := buildModRM(1, src.RegBits, 4) // mod=01 (disp8), reg=src, rm=100 (SIB)
		sib := buildSIB(0, 4, base.RegBits)    // scale=0, index=none(4), base=R12
		return []byte{rex, X86_OP_MOV_RM_R, modrm, sib, offset}
	} else {
		// Normal encoding
		modrm := buildModRM(1, src.RegBits, base.RegBits) // mod=01 (disp8), reg=src, rm=base
		return []byte{rex, X86_OP_MOV_RM_R, modrm, offset}
	}
}
func encodeMovImm64ToMem(memAddr uint64, imm uint64) []byte {
	// this instruction only do once
	// move it on top of the start code to save push & pop

	var code []byte

	// PUSH RAX and RDX (save scratch registers)
	code = append(code, X86_OP_PUSH_R)   // PUSH RAX
	code = append(code, X86_OP_PUSH_R+2) // PUSH RDX

	// MOVABS RAX, memAddr
	code = append(code, X86_REX_W_PREFIX, X86_OP_MOV_RAX_IMM64) // MOV RAX, imm64
	code = append(code, encodeU64(memAddr)...)

	// MOVABS RDX, imm
	code = append(code, X86_REX_W_PREFIX, X86_OP_MOV_RDX_IMM64) // MOV RDX, imm64
	code = append(code, encodeU64(imm)...)

	// MOV [RAX], RDX
	code = append(code, X86_REX_W_PREFIX, X86_OP_MOV_RM_R, X86_MODRM_RAX_RDX) // MOV QWORD PTR [RAX], RDX

	// POP RDX and RAX (restore)
	code = append(code, X86_OP_POP_R+2) // POP RDX
	code = append(code, X86_OP_POP_R)   // POP RAX

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
	rex := buildREX(false, false, false, base.REXBit != 0)
	// MOV [Base+disp], imm32 → opcode 0xC7 /0 + ModRM/SIB
	modrm := buildModRM(2, 0, 4)        // mod=10 (disp32), reg=000, r/m=100→SIB
	sib := buildSIB(0, 4, base.RegBits) // scale=0, index=none, base=base register

	// disp32 LE
	dispBytes := encodeU32(disp)
	immBytes := encodeU32(imm)

	buf = append(buf, rex, X86_OP_MOV_RM_IMM, modrm, sib)
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

	// Choose temporary register: default is RCX, but if dst==RCX use RAX
	var tempReg X86Reg
	if dst.Name == "rcx" {
		tempReg = RAX
	} else {
		tempReg = RCX
	}

	// 1) Save temporary register
	code = append(code, emitPushReg(tempReg)...)

	// 2) MOVABS tempReg, addr
	code = append(code, emitMovImmToReg64(tempReg, addr)...)

	// 3) MOV dst, [tempReg]
	code = append(code, emitMovMemToReg64(dst, tempReg)...)

	// 4) Restore temporary register
	code = append(code, emitPopReg(tempReg)...)

	return code
}

// Increment the 64-bit value at memory[addr:uint64] by 1.
func generateIncMem(addr uint64) []byte {
	var code []byte
	rcx := RCX

	// 1) Save RCX
	code = append(code, emitPushReg(rcx)...)

	// 2) Load target address into RCX: MOVABS RCX, imm64
	code = append(code, emitMovImmToReg64(rcx, addr)...)

	// 3) Increment [RCX] as a 64-bit integer: INC QWORD PTR [RCX]
	code = append(code, emitIncMemIndirect(rcx)...)

	// 4) Restore RCX
	code = append(code, emitPopReg(rcx)...)

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
	// Use the new emit helper function
	return emitJmpRegMemDisp(srcReg, rel)
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
	rex := byte(X86_REX_BASE) // REX prefix base
	if w {
		rex |= X86_REX_W // W bit
	}
	if r {
		rex |= X86_REX_R // R bit
	}
	if x {
		rex |= X86_REX_X // X bit
	}
	if b {
		rex |= X86_REX_B // B bit
	}
	return rex
}

// buildModRM constructs a ModR/M byte
// mod: addressing mode (00, 01, 10, 11)
// reg: register/opcode field (3 bits)
// rm: register/memory field (3 bits)
func buildModRM(mod, reg, rm byte) byte {
	return (mod << 6) | ((reg & X86_MOD_REG_MASK) << 3) | (rm & X86_MOD_REG_MASK)
}

// buildSIB constructs a SIB (Scale-Index-Base) byte
// scale: scale factor (0=1x, 1=2x, 2=4x, 3=8x)
// index: index register (0-7, 4=none)
// base: base register (0-7)
func buildSIB(scale, index, base byte) byte {
	return (scale << 6) | ((index & X86_MOD_REG_MASK) << 3) | (base & X86_MOD_REG_MASK)
}

// ================================================================================================
// Basic MOV Instructions
// ================================================================================================

// emitMovRegToReg64 emits: MOV dst, src (64-bit)
func emitMovRegToReg64(dst, src X86Reg) []byte {
	rex := buildREX(true, src.REXBit == 1, false, dst.REXBit == 1)
	modrm := buildModRM(X86_MOD_REGISTER, src.RegBits, dst.RegBits)
	return []byte{rex, X86_OP_MOV_RM_R, modrm}
}

// emitMovRegToReg32 emits: MOV dst, src (32-bit)
func emitMovRegToReg32(dst, src X86Reg) []byte {
	rex := buildREX(false, src.REXBit == 1, false, dst.REXBit == 1)
	modrm := buildModRM(X86_MOD_REGISTER, src.RegBits, dst.RegBits)
	return []byte{rex, X86_OP_MOV_RM_R, modrm}
}

// emitMovImmToReg64 emits: MOV dst, imm64
func emitMovImmToReg64(dst X86Reg, val uint64) []byte {
	rex := buildREX(true, false, false, dst.REXBit == 1)
	opcode := byte(X86_OP_MOV_R_IMM + dst.RegBits)
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
	opcode := byte(X86_OP_MOV_R_IMM + dst.RegBits)
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
	modrm := buildModRM(X86_MOD_REGISTER, src.RegBits, dst.RegBits)
	return []byte{rex, X86_OP_ADD_RM_R, modrm}
}

// emitSubReg64 emits: SUB dst, src (64-bit)
func emitSubReg64(dst, src X86Reg) []byte {
	rex := buildREX(true, src.REXBit == 1, false, dst.REXBit == 1)
	modrm := buildModRM(X86_MOD_REGISTER, src.RegBits, dst.RegBits)
	return []byte{rex, X86_OP_SUB_RM_R, modrm}
}

// ================================================================================================
// Logical Instructions
// ================================================================================================

// emitAndReg64 emits: AND dst, src (64-bit)
func emitAndReg64(dst, src X86Reg) []byte {
	rex := buildREX(true, src.REXBit == 1, false, dst.REXBit == 1)
	modrm := buildModRM(X86_MOD_REGISTER, src.RegBits, dst.RegBits)
	return []byte{rex, X86_OP_AND_RM_R, modrm}
}

// emitOrReg64 emits: OR dst, src (64-bit)
func emitOrReg64(dst, src X86Reg) []byte {
	rex := buildREX(true, src.REXBit == 1, false, dst.REXBit == 1)
	modrm := buildModRM(X86_MOD_REGISTER, src.RegBits, dst.RegBits)
	return []byte{rex, X86_OP_OR_RM_R, modrm}
}

// emitXorReg64 emits: XOR dst, src (64-bit)
func emitXorReg64(dst, src X86Reg) []byte {
	rex := buildREX(true, src.REXBit == 1, false, dst.REXBit == 1)
	modrm := buildModRM(X86_MOD_REGISTER, src.RegBits, dst.RegBits)
	return []byte{rex, X86_OP_XOR_RM_R, modrm}
}

// emitXorReg32 emits: XOR dst, src (32-bit)
func emitXorReg32(dst, src X86Reg) []byte {
	rex := buildREX(false, src.REXBit == 1, false, dst.REXBit == 1)
	modrm := buildModRM(X86_MOD_REGISTER, src.RegBits, dst.RegBits)
	return []byte{rex, X86_OP_XOR_RM_R, modrm}
}

// ================================================================================================
// Unary Instructions
// ================================================================================================

// emitNotReg64 emits: NOT reg (64-bit)
func emitNotReg64(reg X86Reg) []byte {
	rex := buildREX(true, false, false, reg.REXBit == 1)
	modrm := buildModRM(X86_MOD_REGISTER, X86_REG_NOT, reg.RegBits) // /2 for NOT
	return []byte{rex, X86_OP_UNARY_RM, modrm}
}

// ================================================================================================
// Comparison Instructions (X86_OP_CMP_RM_R = 0x39)
// ================================================================================================

// emitCmpReg64 emits: CMP a, b (64-bit)
func emitCmpReg64(a, b X86Reg) []byte {
	rex := buildREX(true, a.REXBit == 1, false, b.REXBit == 1)
	modrm := buildModRM(X86_MOD_REGISTER, a.RegBits, b.RegBits)
	return []byte{rex, X86_OP_CMP_RM_R, modrm}
}

// emitCmpRaxRdxDivSOp64 emits: CMP RAX, RDX (specific construction for DivSOp64)
func emitCmpRaxRdxDivSOp64(src X86Reg) []byte {
	rex := buildREX(true, false, false, src.REXBit == 1)
	modrm := buildModRM(X86_MOD_REGISTER, 2, src.RegBits) // mod=11, reg=2(RDX), rm=src.RegBits
	return []byte{rex, X86_OP_CMP_RM_R, modrm}
}

// emitCmpRaxRdxRemSOp64 emits: CMP RAX, RDX (specific construction for RemSOp64)
func emitCmpRaxRdxRemSOp64() []byte {
	rex := buildREX(true, false, false, false) // W=1, R=0, X=0, B=0
	modrm := buildModRM(0x03, 2, 0)            // mod=11, reg=2 (RDX), rm=0 (RAX)
	return []byte{rex, X86_OP_CMP_RM_R, modrm}
}

// emitCmpRegRegMinU emits: CMP dst, b with exact manual construction for MIN_U non-alias case
func emitCmpRegRegMinU(dst, b X86Reg) []byte {
	return emitCmpReg64(b, dst)
}

// emitCmpReg64Reg64 emits: CMP src1_64, src2_64 (64-bit register compare)
func emitCmpReg64Reg64(src1 X86Reg, src2 X86Reg) []byte {
	rex := buildREX(true, src2.REXBit == 1, false, src1.REXBit == 1) // 64-bit, R=src2, B=src1
	modrm := buildModRM(0x03, src2.RegBits, src1.RegBits)
	return []byte{rex, X86_OP_CMP_RM_R, modrm} // 39 /r = CMP r/m64, r64
}

// emitCmpReg64Max emits: CMP a, b with exact manual construction for MAX function
func emitCmpReg64Max(a, b X86Reg) []byte {
	var result []byte

	rex := buildREX(true, b.REXBit == 1, false, a.REXBit == 1)
	modrm := buildModRM(0x03, b.RegBits, a.RegBits)
	result = append(result, rex, X86_OP_CMP_RM_R, modrm) // 39 /r = CMP r/m64, r64

	return result
}

// emitCmpRegImm32MinInt emits: CMP reg, 0x80000000 (for MinInt32 comparison)
func emitCmpRegImm32MinInt(reg X86Reg) []byte {
	rex := buildREX(false, false, false, reg.REXBit == 1)   // 32-bit operation
	modrm := buildModRM(0x03, 7, reg.RegBits)               // /7 for CMP
	return []byte{rex, 0x81, modrm, 0x00, 0x00, 0x00, 0x80} // 0x80000000 in little-endian
}

// emitCmpRegImm32Force81 emits: CMP reg, imm32 using 0x81 opcode (forces 32-bit immediate form)
func emitCmpRegImm32Force81(reg X86Reg, imm int32) []byte {
	rex := buildREX(true, false, false, reg.REXBit == 1)
	modrm := buildModRM(X86_MOD_REGISTER, X86_REG_CMP, reg.RegBits)
	result := []byte{rex, 0x81, modrm}
	result = append(result, encodeU32(uint32(imm))...)
	return result
}

// ================================================================================================
// Push/Pop Instructions
// ================================================================================================

// emitPushReg emits: PUSH reg
func emitPushReg(reg X86Reg) []byte {
	if reg.REXBit == 1 {
		return []byte{X86_REX_BASE | X86_REX_B, X86_OP_PUSH_R + reg.RegBits}
	}
	return []byte{X86_OP_PUSH_R + reg.RegBits}
}

// emitPopReg emits: POP reg
func emitPopReg(reg X86Reg) []byte {
	if reg.REXBit == 1 {
		return []byte{X86_REX_BASE | X86_REX_B, X86_OP_POP_R + reg.RegBits}
	}
	return []byte{X86_OP_POP_R + reg.RegBits}
}

// ================================================================================================
// Complex Instructions
// ================================================================================================

// emitPopCnt64 emits: POPCNT dst, src (64-bit)
func emitPopCnt64(dst, src X86Reg) []byte {
	rex := buildREX(true, dst.REXBit == 1, false, src.REXBit == 1)
	modrm := buildModRM(X86_MOD_REGISTER, dst.RegBits, src.RegBits)
	return []byte{X86_PREFIX_REP, rex, X86_PREFIX_0F, X86_OP2_POPCNT, modrm}
}

// emitPopCnt32 emits: POPCNT dst, src (32-bit)
func emitPopCnt32(dst, src X86Reg) []byte {
	rex := buildREX(false, dst.REXBit == 1, false, src.REXBit == 1)
	modrm := buildModRM(X86_MOD_REGISTER, dst.RegBits, src.RegBits)
	return []byte{X86_PREFIX_REP, rex, X86_PREFIX_0F, X86_OP2_POPCNT, modrm}
}

// emitLzcnt64 emits: LZCNT dst, src (64-bit)
func emitLzcnt64(dst, src X86Reg) []byte {
	rex := buildREX(true, dst.REXBit == 1, false, src.REXBit == 1)
	modrm := buildModRM(X86_MOD_REGISTER, dst.RegBits, src.RegBits)
	return []byte{X86_PREFIX_REP, rex, X86_PREFIX_0F, X86_OP2_BSR, modrm}
}

// emitLzcnt32 emits: LZCNT dst, src (32-bit)
func emitLzcnt32(dst, src X86Reg) []byte {
	rex := buildREX(false, dst.REXBit == 1, false, src.REXBit == 1)
	modrm := buildModRM(X86_MOD_REGISTER, dst.RegBits, src.RegBits)
	return []byte{X86_PREFIX_REP, rex, X86_PREFIX_0F, X86_OP2_BSR, modrm}
}

// emitTzcnt64 emits: TZCNT dst, src (64-bit)
func emitTzcnt64(dst, src X86Reg) []byte {
	rex := buildREX(true, dst.REXBit == 1, false, src.REXBit == 1)
	modrm := buildModRM(X86_MOD_REGISTER, dst.RegBits, src.RegBits)
	return []byte{X86_PREFIX_REP, rex, X86_PREFIX_0F, X86_OP2_BSF, modrm}
}

// emitTzcnt32 emits: TZCNT dst, src (32-bit)
func emitTzcnt32(dst, src X86Reg) []byte {
	rex := buildREX(false, dst.REXBit == 1, false, src.REXBit == 1)
	modrm := buildModRM(X86_MOD_REGISTER, dst.RegBits, src.RegBits)
	return []byte{X86_PREFIX_REP, rex, X86_PREFIX_0F, X86_OP2_BSF, modrm}
}

// emitMovsx64From8 emits: MOVSX dst, src (8-bit to 64-bit sign extend)
func emitMovsx64From8(dst, src X86Reg) []byte {
	rex := buildREX(true, dst.REXBit == 1, false, src.REXBit == 1)
	modrm := buildModRM(X86_MOD_REGISTER, dst.RegBits, src.RegBits)
	return []byte{rex, X86_PREFIX_0F, X86_OP2_MOVSX_R_RM8, modrm}
}

// emitMovsx64From16 emits: MOVSX dst, src (16-bit to 64-bit sign extend)
func emitMovsx64From16(dst, src X86Reg) []byte {
	rex := buildREX(true, dst.REXBit == 1, false, src.REXBit == 1)
	modrm := buildModRM(X86_MOD_REGISTER, dst.RegBits, src.RegBits)
	return []byte{rex, X86_PREFIX_0F, X86_OP2_MOVSX_R_RM16, modrm}
}

// emitMovzx64From16 emits: MOVZX dst, src (16-bit to 64-bit zero extend)
func emitMovzx64From16(dst, src X86Reg) []byte {
	rex := buildREX(true, dst.REXBit == 1, false, src.REXBit == 1)
	modrm := buildModRM(X86_MOD_REGISTER, dst.RegBits, src.RegBits)
	return []byte{rex, X86_PREFIX_0F, X86_OP2_MOVZX_R_RM16, modrm}
}

// emitBswap64 emits: BSWAP dst (64-bit byte swap)
func emitBswap64(dst X86Reg) []byte {
	rex := buildREX(true, false, false, dst.REXBit == 1)
	opcode := byte(X86_OP2_BSWAP + dst.RegBits)
	return []byte{rex, X86_PREFIX_0F, opcode}
}

// ================================================================================================
// Immediate Operations
// ================================================================================================

// emitAddRegImm32 emits: ADD reg, imm32 (signed 32-bit immediate)
func emitAddRegImm32(reg X86Reg, imm int32) []byte {
	rex := buildREX(true, false, false, reg.REXBit == 1)

	if imm >= -128 && imm <= 127 {
		// Use 8-bit immediate form: ADD r/m64, imm8
		modrm := buildModRM(X86_MOD_REGISTER, X86_REG_ADD, reg.RegBits) // /0 for ADD
		return []byte{rex, X86_OP_GROUP1_RM_IMM8, modrm, byte(imm)}
	} else {
		// Use 32-bit immediate form: ADD r/m64, imm32
		modrm := buildModRM(X86_MOD_REGISTER, X86_REG_ADD, reg.RegBits) // /0 for ADD
		immBytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(immBytes, uint32(imm))
		result := []byte{rex, X86_OP_GROUP1_RM_IMM32, modrm}
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
	modrm := buildModRM(3, X86_REG_NEG, reg.RegBits) // ModR/M: mod=11, reg=3 (/3), rm=reg
	return []byte{rex, X86_OP_UNARY_RM, modrm}       // F7 /3 = NEG r/m64
}

// emitAddRegImm8 emits: ADD reg64, imm8
func emitAddRegImm8(reg X86Reg, imm int8) []byte {
	rex := buildREX(true, false, false, reg.REXBit == 1)
	modrm := buildModRM(3, X86_REG_ADD, reg.RegBits)            // reg=0 (/0)
	return []byte{rex, X86_OP_GROUP1_RM_IMM8, modrm, byte(imm)} // 83 /0 ib = ADD r/m64, imm8
}

// emitShiftRegImm1 emits: shift reg, 1 (D1 /subcode)
func emitShiftRegImm1(reg X86Reg, subcode byte) []byte {
	rex := buildREX(true, false, false, reg.REXBit == 1)
	modrm := buildModRM(X86_MOD_REGISTER, subcode, reg.RegBits)
	return []byte{rex, X86_OP_GROUP2_RM_1, modrm}
}

// emitShiftRegImm emits: shift reg, imm8 (opcode /subcode ib)
func emitShiftRegImm(reg X86Reg, opcode, subcode byte, imm byte) []byte {
	rex := buildREX(true, false, false, reg.REXBit == 1)
	modrm := buildModRM(X86_MOD_REGISTER, subcode, reg.RegBits)
	return []byte{rex, opcode, modrm, imm}
}

// emitCmovcc emits: CMOVcc dst, src (conditional move)
func emitCmovcc(cc byte, dst, src X86Reg) []byte {
	rex := buildREX(true, dst.REXBit == 1, false, src.REXBit == 1)
	modrm := buildModRM(X86_MOD_REGISTER, dst.RegBits, src.RegBits)
	return []byte{rex, X86_PREFIX_0F, cc, modrm}
}

// ================================================================================================
// X86_OP_TEST_RM_R helpers
// ================================================================================================

// emitTestReg64 emits: TEST dst, src (test and set flags)
func emitTestReg64(dst, src X86Reg) []byte {
	rex := buildREX(true, src.REXBit == 1, false, dst.REXBit == 1)
	modrm := buildModRM(X86_MOD_REGISTER, src.RegBits, dst.RegBits)
	return []byte{rex, X86_OP_TEST_RM_R, modrm} // 85 /r = TEST r/m64, r64
}

// emitTestReg32 emits: TEST r32, r32
func emitTestReg32(dst, src X86Reg) []byte {
	rex := buildREX(false, src.REXBit == 1, false, dst.REXBit == 1)
	modrm := buildModRM(3, src.RegBits, dst.RegBits)
	return []byte{rex, X86_OP_TEST_RM_R, modrm} // 85 /r = TEST r/m32, r32
}

// emitJne32 emits: JNE rel32 + 4-byte displacement
func emitJne32() []byte {
	return []byte{X86_PREFIX_0F, X86_OP_TEST_RM_R, 0, 0, 0, 0} // 4-byte displacement filled in later
}

// ================================================================================================
// Additional Specialized Instructions
// ================================================================================================

// emitMovByteToCL emits: MOV CL, src_low8
func emitMovByteToCL(src X86Reg) []byte {
	rex := buildREX(false, false, false, src.REXBit == 1)
	modrm := buildModRM(X86_MOD_REGISTER, 1, src.RegBits) // CL is RCX low byte (reg=1)
	return []byte{rex, X86_OP_MOV_RM8_R8, modrm}
}

// emitImul64 emits: IMUL dst, src (64-bit)
func emitImul64(dst, src X86Reg) []byte {
	rex := buildREX(true, dst.REXBit == 1, false, src.REXBit == 1)
	modrm := buildModRM(0x03, dst.RegBits, src.RegBits)
	return []byte{rex, X86_PREFIX_0F, 0xAF, modrm}
}

// emitCdq emits: CDQ (convert double word to quad word, sign-extend EAX to EDX:EAX)
func emitCdq() []byte {
	return []byte{X86_OP_CDQ} // 99 = CDQ
}

// emitIdiv32 emits: IDIV r32 (signed divide EDX:EAX by r32)
func emitIdiv32(src X86Reg) []byte {
	rex := buildREX(false, false, false, src.REXBit == 1)
	modrm := buildModRM(0x03, 0x07, src.RegBits) // opcode extension /7 for IDIV
	return []byte{rex, 0xF7, modrm}
}

// emitCmpRegImmByte emits: CMP reg, imm8 (32-bit register compare with 8-bit immediate)
func emitCmpRegImmByte(reg X86Reg, imm byte) []byte {
	rex := buildREX(false, false, false, reg.REXBit == 1) // 32-bit operation
	modrm := buildModRM(0x03, 7, reg.RegBits)             // /7 for CMP
	return []byte{rex, 0x83, modrm, imm}
}

// emitXorReg64Reg64 emits: XOR reg64, reg64
func emitXorReg64Reg64(reg X86Reg) []byte {
	rex := buildREX(true, reg.REXBit == 1, false, reg.REXBit == 1) // 64-bit, R=reg, B=reg
	modrm := buildModRM(0x03, reg.RegBits, reg.RegBits)            // mod=11, reg=reg, rm=reg
	return []byte{rex, 0x31, modrm}
}

// emitJmp32 emits: JMP rel32 (0xE9 + 4-byte displacement)
func emitJmp32() []byte {
	return []byte{0xE9, 0, 0, 0, 0} // 4-byte displacement filled in later
}

// emitPopRdxRax emits: POP RDX; POP RAX
func emitPopRdxRax() []byte {
	return []byte{0x5A, 0x58} // POP RDX, POP RAX
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

// emitShiftOp64 emits: shift operation on reg64 by CL
func emitShiftOp64(opcode byte, regField byte, reg X86Reg) []byte {
	rex := buildREX(true, false, false, reg.REXBit == 1) // 64-bit, B=reg
	modrm := buildModRM(0x03, regField, reg.RegBits)     // mod=11, reg=regField, rm=reg
	return []byte{rex, opcode, modrm}
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

// emitShl32ByCl emits: SHL reg32, CL
func emitShl32ByCl(reg X86Reg) []byte {
	rex := buildREX(false, false, false, reg.REXBit == 1) // 32-bit, B=reg
	modrm := buildModRM(0x03, 4, reg.RegBits)             // mod=11, reg=4 (/4 for SHL), rm=reg
	return []byte{rex, 0xD3, modrm}
}

// emitImulReg32 emits: IMUL dst32, src32 (32-bit signed multiply)
func emitImulReg32(dst X86Reg, src X86Reg) []byte {
	rex := buildREX(false, dst.REXBit == 1, false, src.REXBit == 1) // 32-bit operation
	modrm := buildModRM(0x03, dst.RegBits, src.RegBits)
	return []byte{rex, X86_PREFIX_0F, 0xAF, modrm} // 0F AF /r = IMUL r32, r/m32
}

// emitSetccReg8 emits: SETcc r/m8 (set byte on condition)
func emitSetccReg8(cc byte, dst X86Reg) []byte {
	rex := buildREX(false, false, false, dst.REXBit == 1) // 8-bit operation, B=dst
	modrm := buildModRM(0x03, 0, dst.RegBits)
	return []byte{rex, X86_PREFIX_0F, cc, modrm} // 0F 90+cc /r = SETcc r/m8
}

// emitMovzxReg64Reg8 emits: MOVZX dst64, src8 (zero-extend 8-bit to 64-bit)
func emitMovzxReg64Reg8(dst X86Reg, src X86Reg) []byte {
	rex := buildREX(true, dst.REXBit == 1, false, src.REXBit == 1) // 64-bit dest, R=dst, B=src
	modrm := buildModRM(0x03, dst.RegBits, src.RegBits)
	return []byte{rex, X86_PREFIX_0F, 0xB6, modrm} // 0F B6 /r = MOVZX r64, r/m8
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

// ================================================================================================
//
//	X86_OP_XCHG_RM_R
//
// ================================================================================================
// emitXchgReg64 emits: XCHG r64, r64
func emitXchgReg64(reg1, reg2 X86Reg) []byte {
	rex := buildREX(true, reg1.REXBit == 1, false, reg2.REXBit == 1)
	modrm := buildModRM(0x03, reg1.RegBits, reg2.RegBits)
	return []byte{rex, X86_OP_XCHG_RM_R, modrm}
}

// emitXchgRegReg64 emits: XCHG reg1, reg2
func emitXchgRegReg64(reg1, reg2 X86Reg) []byte {
	rex := buildREX(true, reg1.REXBit == 1, false, reg2.REXBit == 1) // 64-bit, R=reg1, B=reg2
	modrm := buildModRM(0x03, reg1.RegBits, reg2.RegBits)            // mod=11, reg=reg1, rm=reg2
	return []byte{rex, X86_OP_XCHG_RM_R, modrm}
}

// emitXchgReg32Reg32 emits: XCHG reg1_32, reg2_32
func emitXchgReg32Reg32(reg1 X86Reg, reg2 X86Reg) []byte {
	rex := buildREX(false, reg1.REXBit == 1, false, reg2.REXBit == 1) // 32-bit operation, R=reg1, B=reg2
	modrm := buildModRM(0x03, reg1.RegBits, reg2.RegBits)
	return []byte{rex, X86_OP_XCHG_RM_R, modrm} // 87 /r = XCHG r/m32, r32
}

// emitXchgRaxRdxRemUOp64 emits: XCHG RAX, RDX (specific construction for RemUOp64)
func emitXchgRaxRdxRemUOp64() []byte {
	return []byte{0x48, X86_OP_XCHG_RM_R, 0xD0}
}

func emitXchgRegRcxImmShiftOp64Alt(src X86Reg) []byte {
	rex := buildREX(true, src.REXBit == 1, false, false)
	modrm := buildModRM(0x3, src.RegBits, 0x01)
	return []byte{rex, X86_OP_XCHG_RM_R, modrm}
}

// emitXchgRcxReg64 emits: XCHG RCX, reg64
func emitXchgRcxReg64(reg X86Reg) []byte {
	rex := buildREX(true, reg.REXBit == 1, false, false) // 64-bit, R=reg, rm=RCX(1)
	modrm := buildModRM(0x03, reg.RegBits, 1)            // mod=11, reg=reg, rm=1 (RCX)
	return []byte{rex, X86_OP_XCHG_RM_R, modrm}
}

// emitShiftReg32ByCl emits: SHIFT reg32, CL with specified regField
// regField: 4=SHL, 5=SHR, 7=SAR, etc.
func emitShiftReg32ByCl(dst X86Reg, regField byte) []byte {
	rex := buildREX(false, false, false, dst.REXBit == 1) // 32-bit operation, B=dst
	modrm := buildModRM(0x03, regField, dst.RegBits)
	return []byte{rex, 0xD3, modrm} // D3 /n = SHIFT r/m32, CL
}

// emitJeRel32 emits: JE rel32 (conditional jump equal)
func emitJeRel32() []byte {
	return []byte{X86_PREFIX_0F, 0x84, 0, 0, 0, 0} // 0F 84 = JE rel32 (placeholder offset)
}

// ================================================================================================
// X86_OP_MOV_R_RM = 0x8B
// ================================================================================================
// emitMovReg32ToReg32 emits MOV dst32, src32
func emitMovReg32ToReg32(dst, src X86Reg) []byte {
	rex := buildREX(false, dst.REXBit == 1, false, src.REXBit == 1)
	modrm := buildModRM(X86_MOD_REGISTER, dst.RegBits, src.RegBits)
	return []byte{rex, X86_OP_MOV_R_RM, modrm}
}

// emitMovReg32 emits: MOV dst32, src32 (32-bit register to register move)
func emitMovReg32(dst X86Reg, src X86Reg) []byte {
	rex := buildREX(false, dst.REXBit == 1, false, src.REXBit == 1) // 32-bit operation
	modrm := buildModRM(X86_MOD_REGISTER, dst.RegBits, src.RegBits)
	return []byte{rex, X86_OP_MOV_R_RM, modrm} // 8B /r = MOV r32, r/m32
}

// emitMovEcxFromReg32 emits: MOV ECX, reg32
func emitMovEcxFromReg32(src X86Reg) []byte {
	rex := buildREX(false, false, false, src.REXBit == 1) // 32-bit, B=src
	modrm := buildModRM(X86_MOD_REGISTER, 1, src.RegBits) // mod=11, reg=1 (ECX), rm=src
	return []byte{rex, X86_OP_MOV_R_RM, modrm}
}

// emitMovEaxFromReg32 emits: MOV EAX, reg32 (reads from reg into EAX)
func emitMovEaxFromReg32(src X86Reg) []byte {
	rex := buildREX(false, false, false, src.REXBit == 1) // rm field uses REX.B
	modrm := buildModRM(X86_MOD_REGISTER, 0, src.RegBits) // reg=0 (EAX), rm=src
	return []byte{rex, X86_OP_MOV_R_RM, modrm}            // 8B /r = MOV r32, r/m32
}

func emitMovEaxFromRegDivUOp32(src X86Reg) []byte {
	rex := byte(0x40)
	if src.REXBit == 1 {
		rex |= 0x01
	}
	return []byte{rex, X86_OP_MOV_R_RM, byte(0xC0 | (0 << 3) | src.RegBits)}
}

// emitMovEaxFromRegRemUOp32 emits: MOV EAX, reg32 with exact manual construction for RemUOp32
func emitMovEaxFromRegRemUOp32(src X86Reg) []byte {
	rex := buildREX(false, false, false, src.REXBit == 1)
	modrm := buildModRM(0x3, 0, src.RegBits)
	return []byte{rex, X86_OP_MOV_R_RM, modrm}
}

// emitMovRegToRegLoadImmJumpIndirect emits: MOV dst, src with exact manual construction for LoadImmJumpIndirect
func emitMovRegToRegLoadImmJumpIndirect(dst, src X86Reg) []byte {
	// Manual construction for MOV jumpIndTempReg, indexReg
	rex := buildREX(true, dst.REXBit == 1, false, src.REXBit == 1)
	modrm := buildModRM(0x3, dst.RegBits, src.RegBits)
	return []byte{rex, X86_OP_MOV_R_RM, modrm}
}

// emitMovRaxFromRegDivSOp64 emits: MOV RAX, src (specific construction for DivSOp64)
func emitMovRaxFromRegDivSOp64(src X86Reg) []byte {
	rex := byte(0x48)
	if src.REXBit == 1 {
		rex |= 0x01 // REX.B for rm=src
	}
	return []byte{rex, X86_OP_MOV_R_RM, byte(0xC0 | (0 << 3) | src.RegBits)}
}

// emitMovRaxFromRegDivUOp64 emits: MOV RAX, src (specific construction for DivUOp64)
func emitMovRaxFromRegDivUOp64(src X86Reg) []byte {
	rex := buildREX(true, false, false, src.REXBit == 1)
	modrm := buildModRM(0x03, 0, src.RegBits) // mod=11, reg=0 (RAX), rm=src
	return []byte{rex, X86_OP_MOV_R_RM, modrm}
}

// emitMovRcxFromRegShiftOp64B emits: MOV RCX, reg (specific construction for ShiftOp64B)
func emitMovRcxFromRegShiftOp64B(src X86Reg) []byte {
	rex := buildREX(true, false, false, src.REXBit == 1)
	modrm := buildModRM(0x03, 1, src.RegBits) // mod=11, reg=1 (RCX), rm=src
	return []byte{rex, X86_OP_MOV_R_RM, modrm}
}

// emitMovRaxFromRegRemSOp64 emits: MOV RAX, src (specific construction for RemSOp64)
func emitMovRaxFromRegRemSOp64(src X86Reg) []byte {
	rex := buildREX(true, false, false, src.REXBit == 1)
	modrm := buildModRM(0x03, 0, src.RegBits) // mod=11, reg=0 (RAX), rm=src
	return []byte{rex, X86_OP_MOV_R_RM, modrm}
}

// emitMovRaxFromRegRemUOp64 emits: MOV RAX, reg (specific construction for RemUOp64)
func emitMovRaxFromRegRemUOp64(src X86Reg) []byte {
	rex := buildREX(true, false, false, src.REXBit == 1)
	modrm := buildModRM(0x03, 0, src.RegBits) // mod=11, reg=0 (RAX), rm=src
	return []byte{rex, X86_OP_MOV_R_RM, modrm}
}

// emitMovRcxRegMinU emits: MOV RCX, reg specifically for MIN_U step 2
func emitMovRcxRegMinU(reg X86Reg) []byte {
	rex := buildREX(true, false, false, reg.REXBit == 1)
	modrm := buildModRM(0x03, 1, reg.RegBits)  // mod=11, reg=1(RCX), rm=reg
	return []byte{rex, X86_OP_MOV_R_RM, modrm} // MOV RCX, reg
}

// emitMovRegToRegMax emits: MOV dst, src with exact manual construction for MAX function
func emitMovRegToRegMax(dst, src X86Reg) []byte {
	var result []byte

	rex := buildREX(true, false, false, src.REXBit == 1)
	modrm := buildModRM(0x03, dst.RegBits, src.RegBits)  // mod=11, reg=dst, rm=src
	result = append(result, rex, X86_OP_MOV_R_RM, modrm) // 8B /r = MOV r64, r/m64

	return result
}

// emitMovRegFromMemJumpIndirect emits: MOV dst, [src] with exact manual construction for JUMP_IND
func emitMovRegFromMemJumpIndirect(dst, src X86Reg) []byte {
	var result []byte
	rex := buildREX(true, dst.REXBit == 1, false, src.REXBit == 1)
	modrm := buildModRM(X86_MOD_REGISTER, dst.RegBits, src.RegBits) // mod=11, reg=dst, rm=src
	result = append(result, rex, X86_OP_MOV_R_RM, modrm)
	return result
}

// emitMovRegToRegWithManualConstruction emits: MOV dst, src using exact manual construction (0x8B opcode)
func emitMovRegToRegWithManualConstruction(dst, src X86Reg) []byte {
	// Use centralized REX construction (opcode 0x8B = MOV r64, r/m64)
	rex := buildREX(true, dst.REXBit == 1, false, src.REXBit == 1)
	modrm := buildModRM(X86_MOD_REGISTER, dst.RegBits, src.RegBits)
	return []byte{rex, X86_OP_MOV_R_RM, modrm}
}

// emitXorEaxEax emits XOR EAX, EAX
func emitXorEaxEax() []byte {
	return []byte{0x31, 0xC0}
}

// emitCmov64 emits: CMOVcc r64_dst, r64_src (conditional move)
func emitCmov64(cc byte, dst, src X86Reg) []byte {
	rex := buildREX(true, dst.REXBit == 1, false, src.REXBit == 1)
	modrm := buildModRM(X86_MOD_REGISTER, dst.RegBits, src.RegBits)
	return []byte{rex, X86_PREFIX_0F, cc, modrm}
}

// emitShiftRegCl emits: D3 /subcode r/m64, CL
func emitShiftRegCl(dst X86Reg, subcode byte) []byte {
	rex := buildREX(true, false, false, dst.REXBit == 1)
	modrm := buildModRM(X86_MOD_REGISTER, subcode, dst.RegBits)
	return []byte{rex, 0xD3, modrm}
}

// emitShiftRegImm32 emits: C1 /subcode r/m32, imm8 (32-bit shift with immediate)
func emitShiftRegImm32(dst X86Reg, opcode, subcode byte, imm byte) []byte {
	rex := buildREX(false, false, false, dst.REXBit == 1)
	modrm := buildModRM(X86_MOD_REGISTER, subcode, dst.RegBits)
	return []byte{rex, opcode, modrm, imm}
}

// emitImulRegImm32 emits: IMUL dst, src, imm32 (32-bit multiply with immediate)
func emitImulRegImm32(dst, src X86Reg, imm uint32) []byte {
	rex := buildREX(false, dst.REXBit == 1, false, src.REXBit == 1)
	modrm := buildModRM(X86_MOD_REGISTER, dst.RegBits, src.RegBits)
	result := []byte{rex, 0x69, modrm}
	result = append(result, encodeU32(imm)...)
	return result
}

// emitImulRegImm64 emits: IMUL dst, src, imm32 (64-bit multiply with 32-bit immediate)
func emitImulRegImm64(dst, src X86Reg, imm uint32) []byte {
	rex := buildREX(true, dst.REXBit == 1, false, src.REXBit == 1)
	modrm := buildModRM(X86_MOD_REGISTER, dst.RegBits, src.RegBits)
	result := []byte{rex, 0x69, modrm}
	result = append(result, encodeU32(imm)...)
	return result
}

// emitAluRegImm32 emits: ALU reg64, imm32 (0x81 opcode with subcode)
func emitAluRegImm32(reg X86Reg, subcode byte, imm int32) []byte {
	rex := buildREX(true, false, false, reg.REXBit == 1)
	modrm := buildModRM(X86_MOD_REGISTER, subcode, reg.RegBits)
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

	rex := buildREX(true, reg2.REXBit == 1, false, reg1.REXBit == 1)
	modrm := buildModRM(X86_MOD_REGISTER, reg2.RegBits, reg1.RegBits)
	return []byte{rex, opcode, modrm}
}

// emitLoadWithSIB emits: Load instruction with SIB addressing [base + index*1 + disp32]
// Used for patterns like MOVSX reg, [base + index + disp32]
func emitLoadWithSIB(dst, base, index X86Reg, disp32 int32, opcodeBytes []byte, is64bit bool, prefix byte) []byte {
	// Construct REX prefix: W=1 for 64-bit, R/X/B for high registers
	rex := buildREX(is64bit, dst.REXBit != 0, index.REXBit != 0, base.REXBit != 0)

	// ModRM: mod=10 (disp32), reg=dst, rm=100 (SIB)
	modRM := buildModRM(2, dst.RegBits, 4) // mod=10 (disp32), reg=dst, rm=100 (SIB)
	// SIB: scale=0(1), index=index, base=base
	sib := buildSIB(0, index.RegBits, base.RegBits)

	// disp32 as little-endian
	dispBytes := encodeU32(uint32(disp32))

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
	rex := buildREX(size == 8, src.REXBit == 1, index.REXBit == 1, base.REXBit == 1)
	if rex != 0x40 {
		buf = append(buf, rex)
	}

	// 3) Select opcode: 0x88 for byte, 0x89 for word/dword/qword
	movOp := byte(X86_OP_MOV_RM_R)
	if size == 1 {
		movOp = 0x88
	}
	buf = append(buf, movOp)

	// 4) ModR/M + SIB + disp32
	//    Mod=10 (disp32), Reg=src, RM=100 (SIB)
	modrm := buildModRM(2, src.RegBits, 4) // mod=10 (disp32), reg=src, rm=100 (SIB)
	//    scale=0×1, index=index, base=base
	sib := buildSIB(0, index.RegBits, base.RegBits)
	buf = append(buf, modrm, sib)

	// disp32 as little-endian
	buf = append(buf, encodeU32(uint32(disp32))...)
	return buf
}

// emitTrap emits: INT3
func emitTrap() []byte {
	return []byte{X86_PREFIX_0F, X86_2OP_UD2}
}

// emitNop emits: NOP (0x90 - no operation)
func emitNop() []byte {
	return []byte{X86_OP_NOP}
}

// emitJmpWithPlaceholder emits: JMP rel32 with placeholder bytes 0xFEFEFEFE
func emitJmpWithPlaceholder() []byte {
	return []byte{X86_OP_JMP_REL32, 0xFE, 0xFE, 0xFE, 0xFE}
}

// emitJccWithPlaceholder emits: Jcc rel32 with 4-byte placeholder
func emitJccWithPlaceholder(jcc byte) []byte {
	return []byte{X86_PREFIX_0F, jcc, 0, 0, 0, 0}
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

// Helper functions for initDJumpFunc - these match the exact manual constructions

// emitStoreImmIndWithSIB emits: MOV [base + index*1 + disp32], imm (with SIB addressing)
func emitStoreImmIndWithSIB(idx X86Reg, base X86Reg, disp uint32, imm []byte, opcode byte, prefix byte) []byte {
	var buf []byte

	// Add prefix if specified (e.g., 0x66 for 16-bit)
	if prefix != 0 {
		buf = append(buf, prefix)
	}

	rex := buildREX(false, false, idx.REXBit == 1, base.REXBit == 1)

	buf = append(buf, rex, opcode)

	// ModRM: mod=10 (disp32), reg=000 (/0), r/m=100 (SIB follows)
	buf = append(buf, 0x84)

	// SIB: scale=0(×1)=00, index=idx.RegBits, base=base.RegBits
	sib := buildSIB(0, idx.RegBits, base.RegBits)
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

	rex := buildREX(false, false, false, base.REXBit == 1)

	buf = append(buf, rex, opcode)

	// ModRM: mod=10 (disp32), reg=000 (MOV sub-opcode), r/m=100 (SIB follows)
	buf = append(buf, 0x84)

	// SIB: scale=0 (×1), index=100 (none), base=base.RegBits
	sib := buildSIB(0, 4, base.RegBits)
	buf = append(buf, sib)

	// disp32, little-endian
	buf = append(buf, encodeU32(disp)...)

	// immediate value
	buf = append(buf, imm...)

	return buf
}

// emitLoadWithBaseSIB emits: MOV dst, [base + disp32] with SIB addressing
func emitLoadWithBaseSIB(dst X86Reg, base X86Reg, disp uint32, opcodes []byte, rexW bool) []byte {
	rex := buildREX(rexW, dst.REXBit == 1, false, base.REXBit == 1)

	// ModRM: mod=10 (disp32), reg=dst.RegBits, r/m=100 (SIB follows)
	modrm := buildModRM(0x02, dst.RegBits, 0x04)
	// SIB: scale=0 (×1), index=100(none), base=base.RegBits
	sib := buildSIB(0, 4, base.RegBits)

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
	rex := buildREX(rexW, src.REXBit == 1, false, base.REXBit == 1)

	// ModRM: mod=10 (disp32), reg=src.RegBits, r/m=100 (SIB follows)
	modrm := buildModRM(0x02, src.RegBits, 0x04)
	// SIB: scale=0 (×1), index=100(none), base=base.RegBits
	sib := buildSIB(0, 4, base.RegBits)

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

// emitSubMemImm32 emits: SUB [reg+disp32], imm32 (81 /5)
func emitSubMemImm32(reg X86Reg, disp int32, imm uint32) []byte {
	var result []byte

	// REX.W = 1 for 64-bit operation
	rex := buildREX(true, false, false, reg.REXBit == 1)
	result = append(result, rex)

	// Opcode: 81 /5 (SUB r/m64, imm32)
	result = append(result, 0x81)

	// ModRM: mod = 10 (disp32), reg = 5 (SUB), rm = reg.RegBits
	modrm := buildModRM(0x02, 0x05, reg.RegBits)
	result = append(result, modrm)

	// SIB if needed (for r12/r13/r14/r15 when RegBits == 4)
	if reg.RegBits&0x07 == 4 {
		sib := byte(0x24) // scale=0, index=none(100), base=100 (RSP/r12)
		result = append(result, sib)
	}

	// disp32
	result = append(result, encodeU32(uint32(disp))...)

	// imm32
	result = append(result, encodeU32(imm)...)

	return result
}

// emitJns8 emits: JNS rel8 (short jump if not sign)
func emitJns8(rel8 int8) []byte {
	return []byte{0x79, byte(rel8)}
}

// emitJmpRegMemDisp emits: JMP [reg+disp32] (indirect jump through memory)
func emitJmpRegMemDisp(reg X86Reg, disp int32) []byte {
	var result []byte

	// REX.W = 1 for 64-bit operation
	rex := buildREX(true, false, false, reg.REXBit == 1)
	result = append(result, rex)

	// Opcode: FF /4 (JMP r/m64)
	result = append(result, 0xFF)

	// ModRM: mod = 10 (disp32), reg = 4 (JMP), rm = reg.RegBits
	modrm := buildModRM(0x02, 0x04, reg.RegBits)
	result = append(result, modrm)

	// SIB if needed (for r12/r13/r14/r15 when RegBits == 4)
	if reg.RegBits&0x07 == 4 {
		sib := byte(0x24) // scale=0, index=none(100), base=100 (RSP/r12)
		result = append(result, sib)
	}

	// disp32
	result = append(result, encodeU32(uint32(disp))...)

	return result
}

// emitIncMemIndirect emits: INC QWORD PTR [reg] (increment 64-bit value at [reg])
func emitIncMemIndirect(reg X86Reg) []byte {
	var result []byte

	// REX.W = 1 for 64-bit operation
	rex := buildREX(true, false, false, reg.REXBit == 1)
	result = append(result, rex)

	// Opcode: FF /0 (INC r/m64)
	result = append(result, 0xFF)

	// ModRM: mod = 00 (indirect), reg = 0 (INC), rm = reg.RegBits
	modrm := buildModRM(0x00, 0x00, reg.RegBits)
	result = append(result, modrm)

	// SIB if needed (for certain registers)
	if reg.RegBits&0x07 == 4 {
		sib := byte(0x24) // scale=0, index=none(100), base=100 (RSP/r12)
		result = append(result, sib)
	}

	return result
}

// emitMovMemToReg64 emits: MOV dst, [src] (load 64-bit value from [src] to dst)
func emitMovMemToReg64(dst, src X86Reg) []byte {
	var result []byte

	// REX.W = 1 for 64-bit operation
	rex := buildREX(true, dst.REXBit == 1, false, src.REXBit == 1)
	result = append(result, rex)

	// Opcode: 8B (MOV reg, r/m64)
	result = append(result, 0x8B)

	// ModRM: mod = 00 (indirect), reg = dst.RegBits, rm = src.RegBits
	modrm := buildModRM(0x00, dst.RegBits, src.RegBits)
	result = append(result, modrm)

	// SIB if needed (for certain registers)
	if src.RegBits&0x07 == 4 {
		sib := byte(0x24) // scale=0, index=none(100), base=100 (RSP/r12)
		result = append(result, sib)
	}

	return result
}

// Custom helpers for generateJumpIndirect - exact manual construction matching

// emitPushRegJumpIndirect emits: PUSH reg with exact manual construction for JUMP_IND
func emitPushRegJumpIndirect(reg X86Reg) []byte {
	var result []byte

	// Manual construction matching generateJumpIndirect exactly
	if reg.REXBit == 1 {
		result = append(result, 0x41) // REX.B
	}
	result = append(result, 0x50|reg.RegBits) // PUSH + regBits

	return result
}

// emitAddRegImm32JumpIndirect emits: ADD reg, imm32 with exact manual construction for JUMP_IND
func emitAddRegImm32_100(reg X86Reg, imm uint32) []byte {
	var result []byte

	rex := buildREX(true, false, false, reg.REXBit == 1)
	modrm := buildModRM(0x03, 0, reg.RegBits) // mod=11, reg=0 (ADD), rm=reg
	result = append(result, rex, 0x81, modrm)

	// Add 32-bit immediate (little-endian)
	imm32 := make([]byte, 4)
	binary.LittleEndian.PutUint32(imm32, imm)
	result = append(result, imm32...)

	return result
}

// ================================================================================================
// Custom helpers for  exact manual construction match
// ================================================================================================

// emitCmovgeMax emits: CMOVGE dst, src with exact manual construction for MAX function
func emitCmovgeMax(dst, src X86Reg) []byte {
	var result []byte

	rex := buildREX(true, dst.REXBit == 1, false, src.REXBit == 1)
	modrm := buildModRM(0x03, dst.RegBits, src.RegBits)
	result = append(result, rex, X86_PREFIX_0F, 0x4D, modrm) // 0F 4D /r = CMOVGE r64, r/m64

	return result
}

// emitCmovaMinU emits: CMOVA dst, src with exact manual construction for MIN_U
func emitCmovaMinU(dst, src X86Reg) []byte {
	rex := byte(0x48)
	if dst.REXBit == 1 {
		rex |= 0x04 // REX.R for dst
	}
	if src.REXBit == 1 {
		rex |= 0x01 // REX.B for src
	}
	modrm := byte(0xC0 | (dst.RegBits << 3) | src.RegBits)
	return []byte{rex, X86_PREFIX_0F, 0x47, modrm} // CMOVA r64, r/m64
}

// emitNegReg64NegAddImm32 emits: NEG r64_dst for NegAddImm32 with exact manual construction
func emitNegReg64NegAddImm32(dst X86Reg) []byte {
	rex := buildREX(true, false, false, dst.REXBit == 1)
	modrm := buildModRM(0x03, 0x03, dst.RegBits) // /3 = NEG
	return []byte{rex, 0xF7, modrm}
}

// ================================================================================================
// Custom helpers for generateXEncode - exact manual construction match
// ================================================================================================

// emitMovImmToDstXEncode emits: MOV r64_dst, imm32 (zero) for XEncode with exact manual construction
func emitMovImmToDstXEncode(dst X86Reg) []byte {
	rex := buildREX(true, false, false, dst.REXBit == 1)
	modrm := buildModRM(0x03, 0, dst.RegBits) // mod=11, reg=0, rm=dst
	result := []byte{rex, 0xC7, modrm}        // MOV r64_dst, imm32
	result = append(result, encodeU32(0)...)
	return result
}

// emitShrRcxImmXEncode emits: SHR rcx, imm8 for XEncode with exact manual construction
func emitShrRcxImmXEncode(shift byte) []byte {
	return []byte{0x48, 0xC1, 0xE9, shift} // 0xC1 /5=SHR, modrm E9=(reg=5, rm=1)
}

// emitMovAbsRdxXEncode emits: MOVABS rdx, imm64 for XEncode with exact manual construction
func emitMovAbsRdxXEncode(factor uint64) []byte {
	rex := buildREX(true, false, true, false) // REX.W + REX.X for rdx
	result := []byte{rex, byte(0xB8 | 2)}     // MOVABS rdx, imm64
	result = append(result, encodeU64(factor)...)
	return result
}

// emitImulRdxRcxXEncode emits: IMUL rdx, rcx for XEncode with exact manual construction
func emitImulRdxRcxXEncode() []byte {
	rex := buildREX(true, false, false, false) // W=1, R=0, X=0, B=0
	modrm := buildModRM(0x03, 2, 1)            // mod=11, reg=2 (RDX), rm=1 (RCX)
	return []byte{rex, X86_PREFIX_0F, 0xAF, modrm}
}

// emitAddDstRdxXEncode emits: ADD r64_dst, r64_rdx for XEncode with exact manual construction
func emitAddDstRdxXEncode(dst X86Reg) []byte {
	rex := buildREX(true, false, false, dst.REXBit == 1)
	modrm := buildModRM(0x03, 2, dst.RegBits) // mod=11, reg=2 (RDX), rm=dst
	return []byte{rex, 0x01, modrm}
}

// emitXorRdxRdxRemUOp64 emits: XOR RDX, RDX (specific construction for RemUOp64)
// emitDivMemStackRemUOp64RAX emits: DIV qword ptr [RSP+8] (for src2==RAX case)
func emitDivMemStackRemUOp64RAX() []byte {
	return []byte{
		0x48,             // REX.W
		0xF7, 0xB4, 0x24, // F7 /6 rm=4 (SIB follows)
		0x08, 0x00, 0x00, 0x00, // disp32 = 8
	}
}

// emitDivMemStackRemUOp64RDX emits: DIV qword ptr [RSP] (for src2==RDX case)
func emitDivMemStackRemUOp64RDX() []byte {
	return []byte{
		0x48,       // REX.W
		0xF7, 0x34, // F7 /6 rm=4 (SIB follows)
		0x24, // SIB: scale=0,index=RSP,base=RSP
	}
}

// emitDivRegRemUOp64 emits: DIV reg (specific construction for RemUOp64)
func emitDivRegRemUOp64(src X86Reg) []byte {
	rex := byte(0x48)
	if src.REXBit == 1 {
		rex |= 0x01
	}
	return []byte{rex, 0xF7, byte(0xC0 | (6 << 3) | src.RegBits)}
}

// emitMovRdxIntMinRemSOp64 emits: MOV RDX, 0x8000000000000000 (specific construction for RemSOp64)
func emitMovRdxIntMinRemSOp64() []byte {
	return []byte{
		0x48, 0xBA,
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x80,
	}
}

// emitCmpMemStackNeg1RemSOp64 emits: CMP qword ptr [RSP], -1 (specific construction for RemSOp64)
func emitCmpMemStackNeg1RemSOp64() []byte {
	return []byte{
		0x48,       // REX.W
		0x81, 0x3C, // 81 /7 r/m64, imm32; mod=00, reg=7, rm=4 => 0x3C
		0x24,                   // SIB: scale=0,index=RSP,base=RSP
		0xFF, 0xFF, 0xFF, 0xFF, // imm32 = -1
	}
}

// emitCmpRegNeg1RemSOp64 emits: CMP reg, -1 (specific construction for RemSOp64)
func emitCmpRegNeg1RemSOp64(reg X86Reg) []byte {
	rex := buildREX(true, false, false, reg.REXBit == 1)
	modrm := buildModRM(0x03, 7, reg.RegBits) // mod=11, reg=7 (CMP), rm=reg
	return []byte{rex, 0x81, modrm, 0xFF, 0xFF, 0xFF, 0xFF}
}

// emitCqoRemSOp64 emits: CQO (specific construction for RemSOp64)
func emitCqoRemSOp64() []byte {
	return []byte{0x48, 0x99}
}

// emitIdivMemStackRemSOp64 emits: IDIV qword ptr [RSP] (specific construction for RemSOp64)
func emitIdivMemStackRemSOp64() []byte {
	return []byte{
		0x48,       // REX.W
		0xF7, 0x3C, // F7 /7 rm=4 (SIB)
		0x24, // SIB: scale=0,index=RSP,base=RSP
	}
}

// emitIdivRegRemSOp64 emits: IDIV reg (specific construction for RemSOp64)
func emitIdivRegRemSOp64(reg X86Reg) []byte {
	rex := buildREX(true, false, false, reg.REXBit == 1)
	modrm := buildModRM(0x03, 7, reg.RegBits) // mod=11, reg=7 (IDIV), rm=reg
	return []byte{rex, 0xF7, modrm}
}

// emitXorRegRegRemSOp64 emits: XOR reg, reg (specific construction for RemSOp64)
func emitXorRegRegRemSOp64(reg X86Reg) []byte {
	rex := buildREX(true, reg.REXBit == 1, false, reg.REXBit == 1)
	modrm := buildModRM(0x03, reg.RegBits, reg.RegBits) // mod=11, reg=reg, rm=reg
	return []byte{rex, 0x31, modrm}
}

// ====== Custom helpers for generateShiftOp64B ======

// emitShiftDstByClShiftOp64B emits: SHIFT dst, CL (specific construction for ShiftOp64B)
func emitShiftDstByClShiftOp64B(opcode byte, regField byte, dst X86Reg) []byte {
	rex := buildREX(true, false, false, dst.REXBit == 1)
	modrm := buildModRM(0x03, regField, dst.RegBits) // mod=11, reg=regField, rm=dst
	return []byte{rex, opcode, modrm}
}

// emitXorRegRegDivUOp64 emits: XOR dst, dst (specific construction for DivUOp64)
func emitXorRegRegDivUOp64(dst X86Reg) []byte {
	rex := buildREX(true, dst.REXBit == 1, false, dst.REXBit == 1)
	modrm := buildModRM(0x03, dst.RegBits, dst.RegBits) // mod=11, reg=dst, rm=dst
	return []byte{rex, 0x31, modrm}
}

// emitNotRegDivUOp64 emits: NOT dst (specific construction for DivUOp64)
func emitNotRegDivUOp64(dst X86Reg) []byte {
	rex := byte(0x48)
	if dst.REXBit == 1 {
		rex |= 0x01 // REX.B
	}
	return []byte{rex, 0xF7, byte(0xC0 | (2 << 3) | dst.RegBits)}
}

// emitXorRdxRdxDivUOp64 emits: XOR RDX, RDX (specific construction for DivUOp64)
// emitDivRegDivUOp64 emits: DIV src (specific construction for DivUOp64)
func emitDivRegDivUOp64(src X86Reg) []byte {
	rex := byte(0x48)
	if src.REXBit == 1 {
		rex |= 0x01
	}
	return []byte{rex, 0xF7, byte(0xC0 | (6 << 3) | src.RegBits)}
}

// emitMovDstFromRaxDivUOp64 emits: MOV dst, RAX (specific construction for DivUOp64)
func emitMovDstFromRaxDivUOp64(dst X86Reg) []byte {
	rex := byte(0x48)
	if dst.REXBit == 1 {
		rex |= 0x01
	}
	return []byte{rex, X86_OP_MOV_RM_R, byte(0xC0 | (0 << 3) | dst.RegBits)}
}

// emitMovDstFromSrcShiftOp64B emits: MOV dst, src (specific construction for ShiftOp64B)
func emitMovDstFromSrcShiftOp64B(dst, src X86Reg) []byte {
	rex := buildREX(true, src.REXBit == 1, false, dst.REXBit == 1)
	modrm := buildModRM(0x03, src.RegBits, dst.RegBits) // mod=11, reg=src, rm=dst
	return []byte{rex, X86_OP_MOV_RM_R, modrm}
}

// emitMovDstFromRdxRemUOp64 emits: MOV dst, RDX (specific construction for RemUOp64)
func emitMovDstFromRdxRemUOp64(dst X86Reg) []byte {
	rex := buildREX(true, RDX.REXBit == 1, false, dst.REXBit == 1)
	modrm := buildModRM(0x03, 2, dst.RegBits) // mod=11, reg=2 (RDX), rm=dst
	return []byte{rex, X86_OP_MOV_RM_R, modrm}
}

// emitMovDstFromRaxRemUOp64 emits: MOV dst, RAX (specific construction for RemUOp64)
func emitMovDstFromRaxRemUOp64(dst X86Reg) []byte {
	rex := buildREX(true, false, false, dst.REXBit == 1)
	modrm := buildModRM(0x03, 0, dst.RegBits) // mod=11, reg=0 (RAX), rm=dst
	return []byte{rex, X86_OP_MOV_RM_R, modrm}
}

// emitMovDstToRcxXEncode emits: MOV r64_rcx, r64_dst for XEncode with exact manual construction
func emitMovDstToRcxXEncode(dst X86Reg) []byte {
	rex := buildREX(true, dst.REXBit == 1, false, false)
	modrm := buildModRM(0x03, dst.RegBits, 1) // mod=11, reg=dst, rm=rcx(1)
	return []byte{rex, X86_OP_MOV_RM_R, modrm}
}

// emitMovReg32ToReg32NegAddImm32 emits: MOV r/m32(dst), r32(dst) for NegAddImm32 truncation with exact manual construction
func emitMovReg32ToReg32NegAddImm32(dst X86Reg) []byte {
	rex := buildREX(false, dst.REXBit == 1, false, dst.REXBit == 1) // W=0 for 32-bit
	modrm := buildModRM(0x03, dst.RegBits, dst.RegBits)
	return []byte{rex, X86_OP_MOV_RM_R, modrm}
}

// emitMovRegToReg64NegAddImm32 emits: MOV r64_dst, r64_src for NegAddImm32 with exact manual construction
func emitMovRegToReg64NegAddImm32(dst, src X86Reg) []byte {
	rex := buildREX(true, src.REXBit == 1, false, dst.REXBit == 1)
	modrm := byte(0xC0 | (src.RegBits << 3) | dst.RegBits)
	return []byte{rex, X86_OP_MOV_RM_R, modrm}
}

// emitMovRegToRegMinU emits: MOV dst, src with exact manual construction for MIN_U
func emitMovRegToRegMinU(dst, src X86Reg) []byte {
	rex := buildREX(true, src.REXBit == 1, false, dst.REXBit == 1)
	modrm := buildModRM(0x03, src.RegBits, dst.RegBits)
	return []byte{rex, X86_OP_MOV_RM_R, modrm} // MOV r/m64, r64
}

// emitMovRegFromRegMax emits: MOV dst, src (opposite direction) with exact manual construction for MAX function
func emitMovRegFromRegMax(dst, src X86Reg) []byte {
	var result []byte

	rex := buildREX(true, src.REXBit == 1, false, dst.REXBit == 1)
	modrm := buildModRM(0x03, src.RegBits, dst.RegBits)  // mod=11, reg=src, rm=dst
	result = append(result, rex, X86_OP_MOV_RM_R, modrm) // 89 /r = MOV r/m64, r64

	return result
}

// emitMovRegReg32 emits: MOV dst32, src32
func emitMovRegReg32(dst, src X86Reg) []byte {
	rex := buildREX(false, src.REXBit == 1, false, dst.REXBit == 1) // 32-bit, R=src, B=dst
	modrm := buildModRM(0x03, src.RegBits, dst.RegBits)             // mod=11, reg=src, rm=dst
	return []byte{rex, X86_OP_MOV_RM_R, modrm}                      // MOV dst32, src32
}

func emitMovBaseRegToSrcImmShiftOp64Alt(baseReg, src X86Reg) []byte {
	rex := buildREX(true, baseReg.REXBit == 1, false, src.REXBit == 1)
	modrm := buildModRM(0x3, baseReg.RegBits, src.RegBits)
	return []byte{rex, X86_OP_MOV_RM_R, modrm}
}
func emitMovRegToRegImmShiftOp64Alt(src, dst X86Reg) []byte {
	rex := buildREX(true, src.REXBit == 1, false, dst.REXBit == 1)
	modrm := buildModRM(0x3, src.RegBits, dst.RegBits)
	return []byte{rex, X86_OP_MOV_RM_R, modrm}
}

func emitMovDstEaxDivUOp32(dst X86Reg) []byte {
	rex := buildREX(true, false, false, dst.REXBit == 1)
	modrm := buildModRM(0x3, 0, dst.RegBits)
	return []byte{rex, X86_OP_MOV_RM_R, modrm}
}

// emitMovDstEdxRemUOp32 emits: MOV dst, EDX with exact manual construction for RemUOp32
func emitMovDstEdxRemUOp32(dst X86Reg) []byte {
	rex := buildREX(false, false, false, dst.REXBit == 1)
	modrm := buildModRM(0x3, 2, dst.RegBits)
	return []byte{rex, X86_OP_MOV_RM_R, modrm}
}

// emitXorRegRegDivSOp64 emits: XOR dst, dst (specific construction for DivSOp64)
func emitXorRegRegDivSOp64(dst X86Reg) []byte {
	rex := buildREX(true, dst.REXBit == 1, false, dst.REXBit == 1)
	modrm := buildModRM(0x3, dst.RegBits, dst.RegBits)
	return []byte{rex, 0x31, modrm}
}

// emitNotRegDivSOp64 emits: NOT dst (specific construction for DivSOp64)
func emitNotRegDivSOp64(dst X86Reg) []byte {
	rex := buildREX(true, false, false, dst.REXBit == 1)
	modrm := buildModRM(0x3, 2, dst.RegBits)
	return []byte{rex, 0xF7, modrm}
}

// emitMovRdxMinInt64DivSOp64 emits: MOVABS RDX, MinInt64 (specific construction for DivSOp64)
func emitMovRdxMinInt64DivSOp64() []byte {
	return []byte{
		0x48,                   // REX.W
		0xBA,                   // B8+2 for RDX
		0x00, 0x00, 0x00, 0x00, // low 32 bits
		0x00, 0x00, 0x00, 0x80, // high 32 bits = 0x80
	}
}

// emitCmpRegNeg1DivSOp64 emits: CMP src2, -1 (specific construction for DivSOp64)
func emitCmpRegNeg1DivSOp64(src X86Reg) []byte {
	rex := buildREX(true, false, false, src.REXBit == 1)
	modrm := buildModRM(0x3, 7, src.RegBits)
	return []byte{rex, 0x83, modrm, 0xFF}
}

// emitAluRegImm8 emits: ALU reg64, imm8 (0x83 opcode with subcode)
func emitAluRegImm8(reg X86Reg, subcode byte, imm int8) []byte {
	rex := buildREX(true, false, false, reg.REXBit == 1)
	modrm := buildModRM(X86_MOD_REGISTER, subcode, reg.RegBits)
	return []byte{rex, 0x83, modrm, byte(imm)}
}

// emitCqoDivSOp64 emits: CQO (specific construction for DivSOp64)
func emitCqoDivSOp64() []byte {
	return []byte{0x48, 0x99}
}

// emitIdivRegDivSOp64 emits: IDIV src2 (specific construction for DivSOp64)
func emitIdivRegDivSOp64(src X86Reg) []byte {
	rex := buildREX(true, false, false, src.REXBit == 1)
	modrm := buildModRM(0x3, 7, src.RegBits)
	return []byte{rex, 0xF7, modrm}
}

// ====== Custom helpers for generateLoadImmJumpIndirect ======

// emitPushRegLoadImmJumpIndirect emits: PUSH reg with exact manual construction for LoadImmJumpIndirect
func emitPushRegLoadImmJumpIndirect(reg X86Reg) []byte {
	// Manual construction: PUSH jumpIndTempReg
	rex := buildREX(true, false, false, reg.REXBit == 1)
	pushOp := byte(0x50 | reg.RegBits)
	return []byte{rex, pushOp}
}

// emitMovImmToRegLoadImmJumpIndirect emits: MOV reg, imm64 with exact manual construction for LoadImmJumpIndirect
func emitMovImmToRegLoadImmJumpIndirect(reg X86Reg, imm uint64) []byte {
	// Manual construction for MOV r64, imm64
	rex := buildREX(true, false, false, reg.REXBit == 1)
	movOp := byte(0xB8 + reg.RegBits)
	imm64 := make([]byte, 8)
	binary.LittleEndian.PutUint64(imm64, imm)

	buf := []byte{rex, movOp}
	buf = append(buf, imm64...)
	return buf
}

// emitAddRegImm32LoadImmJumpIndirect emits: ADD reg, imm32 with exact manual construction for LoadImmJumpIndirect
func emitAddRegImm32LoadImmJumpIndirect(reg X86Reg, imm uint32) []byte {
	// Manual construction for ADD jumpIndTempReg, vy
	rex := buildREX(true, false, false, reg.REXBit == 1)
	modrm := buildModRM(0x3, 0, reg.RegBits)
	imm32 := make([]byte, 4)
	binary.LittleEndian.PutUint32(imm32, imm)

	buf := []byte{rex, 0x81, modrm}
	buf = append(buf, imm32...)
	return buf
}

// emitAndRegImm32LoadImmJumpIndirect emits: AND reg, 0xFFFFFFFF with exact manual construction for LoadImmJumpIndirect
func emitAndRegImm32LoadImmJumpIndirect(reg X86Reg) []byte {
	// Manual construction for AND jumpIndTempReg, 0xFFFFFFFF (zero-extend)
	rex := buildREX(true, false, false, reg.REXBit == 1)
	modrm := buildModRM(0x3, 4, reg.RegBits)
	return []byte{rex, 0x81, modrm, 0xFF, 0xFF, 0xFF, 0xFF}
}

// ====== Custom helpers for X86_OP_MOVSXD ======

// emitMovsxdDstEaxRemUOp32 emits: MOVSXD dst, EAX with exact emitDiv32 construction for RemUOp32
func emitMovsxdDstEaxRemUOp32(dst X86Reg) []byte {
	rex := buildREX(true, dst.REXBit == 1, false, false)
	modrm := buildModRM(0x3, dst.RegBits, 0) // rm=0 => EAX
	return []byte{rex, X86_OP_MOVSXD, modrm}
}

// emitMovsxd64 emits: MOVSXD dst64, src32 (sign-extend 32-bit to 64-bit)
func emitMovsxd64(dst X86Reg, src X86Reg) []byte {
	rex := buildREX(true, dst.REXBit == 1, false, src.REXBit == 1) // 64-bit operation, REX.W=1
	modrm := buildModRM(0x03, dst.RegBits, src.RegBits)
	return []byte{rex, X86_OP_MOVSXD, modrm} // 63 /r = MOVSXD r64, r/m32
}

// emitDiv32 emits: DIV r/m32 with exact manual construction for RemUOp32
func emitDiv32(src X86Reg) []byte {
	rex := buildREX(false, false, false, src.REXBit == 1)
	modrm := buildModRM(0x3, 6, src.RegBits)
	return []byte{rex, 0xF7, modrm}
}

// emitPopRdxRaxRemUOp32 emits: POP RDX; POP RAX with exact manual construction for RemUOp32
func emitPopRdxRaxRemUOp32() []byte {
	return []byte{0x5A, 0x58}
}

// Custom helpers for generateDivUOp32
func emitNotReg64DivUOp32(dst X86Reg) []byte {
	rex := buildREX(true, false, false, dst.REXBit == 1)
	modrm := buildModRM(0x3, 2, dst.RegBits)
	return []byte{rex, 0xF7, modrm}
}

func emitPopRdxRaxDivUOp32() []byte {
	return []byte{0x5A, 0x58}
}

func emitShiftRegClImmShiftOp64Alt(dst X86Reg, subcode byte) []byte {
	rex := buildREX(true, false, false, dst.REXBit == 1)
	modrm := buildModRM(0x3, subcode, dst.RegBits)
	return []byte{rex, 0xD3, modrm}
}

func emitLeaRaxRipInitDJump() []byte {
	return []byte{0x48, 0x8D, 0x05, 0x00, 0x00, 0x00, 0x00}
}

func emitJmpE9InitDJump() []byte {
	return []byte{X86_OP_JMP_REL32}
}

func emitUd2InitDJump() []byte {
	return []byte{X86_PREFIX_0F, X86_2OP_UD2}
}

func emitJeInitDJump() []byte {
	return []byte{X86_PREFIX_0F, X86_2OP_JE, 0, 0, 0, 0}
}

func emitJaInitDJump() []byte {
	return []byte{X86_PREFIX_0F, X86_2OP_JA, 0, 0, 0, 0}
}

func emitJneInitDJump() []byte {
	return []byte{X86_PREFIX_0F, X86_2OP_JNE, 0, 0, 0, 0}
}
