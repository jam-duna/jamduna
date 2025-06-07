package pvm

import (
	"github.com/colorfulnotion/jam/types"
)

// A.5.5. Instructions with Arguments of One Offset.
func extractOneOffset(args []byte) (vx int64) {
	lx := min(4, len(args))
	if lx == 0 {
		lx = 1
		args = append(args, 0)
	}
	vx = z_encode(types.DecodeE_l(args[0:lx]), uint32(lx))
	return vx
}


// TERMINATOR: JUMP rel32 in PolkaVM — do not emit an x86 jmp here.
func generateJumpRel32() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		// PolkaVM runtime will advance the PC by inst.Args,
		// then re–dispatch.  We just fall through into the
		// standard register write-back + RET epilogue.
/*
		vx := extractOneOffset(inst.Args)
		// Cast to signed 32-bit
		disp := int32(vx)
		// Little-endian bytes
		b0 := byte(disp)
		b1 := byte(disp >> 8)
		b2 := byte(disp >> 16)
		b3 := byte(disp >> 24)
		// E9 = JMP rel32
		return []byte{0xE9, b0, b1, b2, b3}, nil
*/
		return []byte{}, nil
	}
}
	// A.5.6. Instructions with Arguments of One Register & Two Immediates.
// Implements: JMP QWORD PTR [r64_reg + disp32]
//
//	where regIdx is the base register and vx is the signed 32-bit offset.
func generateJumpIndirect() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		// Extract base register index and displacement
		regIdx, vx := extractOneReg2Imm(inst.Args)
		r := regInfoList[regIdx]

		// REX.W = 1 for 64-bit operand size; REX.B if using r8–r15
		rex := byte(0x48)
		if r.REXBit == 1 {
			rex |= 0x01
		}

		// Opcode: FF /4 → JMP r/m64
		// ModRM: mod=10 (disp32 follows), reg=100b (4), rm = r.RegBits
		modrm := byte(0x80 | (0x04 << 3) | r.RegBits)

		// Encode vx as signed 32-bit little endian
		disp := int32(vx)
		b0 := byte(disp)
		b1 := byte(disp >> 8)
		b2 := byte(disp >> 16)
		b3 := byte(disp >> 24)

		return []byte{rex, 0xFF, modrm, b0, b1, b2, b3}, nil
	}
}

// A.5.8. Instructions with Arguments of One Register, One Immediate and One Offset.
func extractOneRegOneImmOneOffset(args []byte) (registerIndexA int, vx uint64, vy int64) {
	registerIndexA = min(12, int(args[0])%16)
	lx := min(4, (int(args[0]) / 16 % 8))
	ly := min(4, max(0, len(args)-lx-1))
	if ly == 0 {
		ly = 1
		args = append(args, 0)
	}

	vx = x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
	vy = z_encode(types.DecodeE_l(args[1+lx:1+lx+ly]), uint32(ly))
	return registerIndexA, vx, vy
}

// Implements: r64_dst = vx; then PC += vy (signed 32-bit relative jump)
func generateLoadImmJump() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		// extractOneRegOneImmOneOffset returns (dstReg, vx, vy)
		dstIdx, vx, vy := extractOneRegOneImmOneOffset(inst.Args)
		dst := regInfoList[dstIdx]

		// 1) MOV r64_dst, imm64(vx)
		rex := byte(0x48) // REX.W=1
		if dst.REXBit == 1 {
			rex |= 0x01 // REX.B for high registers
		}
		movOpcode := byte(0xB8 | dst.RegBits)
		immBytes := make([]byte, 8)
		for i := 0; i < 8; i++ {
			immBytes[i] = byte(vx >> (8 * i))
		}
		movInst := append([]byte{rex, movOpcode}, immBytes...)

		// 2) JMP rel32(vy)
		// E9 opcode, then 32-bit signed little-endian displacement
		disp := int32(vy)
		rel := []byte{
			byte(disp),
			byte(disp >> 8),
			byte(disp >> 16),
			byte(disp >> 24),
		}
		jmpInst := append([]byte{0xE9}, rel...)

		return append(movInst, jmpInst...), nil
	}
}

func generateBranchImm(opcode byte) func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		regIdx, vx, vy := extractOneRegOneImmOneOffset(inst.Args)
		src := regInfoList[regIdx]
		pc := inst.Pc
		truePC := int64(inst.Pc) + vy
		falsePC := int64(pc + 1)

		var code []byte

		// 1) cmp   r64_src, imm32
		rex := byte(0x48)
		if src.REXBit == 1 {
			rex |= 0x01
		} // REX.B for extended regs
		modrm := byte(0xC0 | (0x07 << 3) | src.RegBits) // /7 = CMP
		code = append(code, rex, 0x81, modrm)
		code = append(code, encodeU32(uint32(vx))...) // imm32

		// 2) Jcc (near, rel8) over the “true” MOVABS+RET if the condition is *false*
		//    Short‐jump by 6 bytes (one MOVABS opcode + 8‐byte imm + RET = 10 bytes total,
		//    but we only need to skip the MOVABS, not the RET, so rel8=10).
		cond := opcode & 0x0F // e.g. 0x74→0x04 for JE
		code = append(code, byte(0x70|cond), byte(10))

		// 3) MOVABS R12, addrTrue; RET
		code = append(code, 0x49, 0xBC) // REX.W|REX.B + MOVABS r12
		for i := 0; i < 8; i++ {
			code = append(code, byte(truePC>>(8*i)))
		}
		code = append(code, 0xC3) // RET

		// 4) MOVABS R12, addrFalse; RET
		code = append(code, 0x49, 0xBC)
		for i := 0; i < 8; i++ {
			code = append(code, byte(falsePC>>(8*i)))
		}
		code = append(code, 0xC3)

		return code, nil
	}
}

func extractTwoRegsAndTwoImmediates(operands []byte) (vx, vy uint64, registerIndexA, registerIndexB int) {

	originalOperands := make([]byte, len(operands))
	copy(originalOperands, operands)

	registerIndexA = min(12, int(originalOperands[0])%16)
	registerIndexB = min(12, int(originalOperands[0])/16)
	lx := min(4, (int(originalOperands[1]) % 8))
	ly := min(4, max(0, len(originalOperands)-lx-2))
	if ly == 0 {
		ly = 1
		originalOperands = append(originalOperands, 0)
	}

	vx = x_encode(types.DecodeE_l(originalOperands[2:2+lx]), uint32(lx))
	vy = x_encode(types.DecodeE_l(originalOperands[2+lx:2+lx+ly]), uint32(ly))
	return vx, vy, registerIndexA, registerIndexB
}

// A.5.12. Instruction with Arguments of Two Registers and Two Immediates.
func generateLoadImmJumpIndirect() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		vx, vy, dstRegIdx, srcRegIdx := extractTwoRegsAndTwoImmediates(inst.Args)
		dst := regInfoList[dstRegIdx]
		src := regInfoList[srcRegIdx]

		// 1) MOV r64_dst, imm64(vx)
		rexMov := byte(0x48) // REX.W
		if dst.REXBit == 1 {
			rexMov |= 0x01 // REX.B
		}
		// Opcode for MOV r64, imm64 is B8+rd_low3
		movOpcode := byte(0xB8 | dst.RegBits)
		// Little-endian imm64
		immBytes := make([]byte, 8)
		for i := 0; i < 8; i++ {
			immBytes[i] = byte(vx >> (8 * i))
		}
		movInst := append([]byte{rexMov, movOpcode}, immBytes...)

		// 2) JMP QWORD PTR [src + disp32(vy)]
		rexJmp := byte(0x48)
		if src.REXBit == 1 {
			rexJmp |= 0x01 // REX.B
		}
		// Opcode for JMP r/m64 is FF /4
		// ModRM: mod=10 (disp32), reg=100b (JMP), rm=src.RegBits
		modrmJmp := byte(0x80 | (0x04 << 3) | src.RegBits)
		// Little-endian 32-bit displacement
		disp32 := int32(vy)
		dispBytes := []byte{
			byte(disp32), byte(disp32 >> 8), byte(disp32 >> 16), byte(disp32 >> 24),
		}
		jmpInst := append([]byte{rexJmp, 0xFF, modrmJmp}, dispBytes...)

		return append(movInst, jmpInst...), nil
	}
}

// A.5.11. Instructions with Arguments of Two Registers & One Offset.
func extractTwoRegsOneOffset(args []byte) (registerIndexA, registerIndexB int, vx uint64) {
	registerIndexA = min(12, int(args[0])%16)
	registerIndexB = min(12, int(args[0])/16)
	lx := min(4, max(0, len(args)-1))
	if lx == 0 {
		lx = 1
		args = append(args, 0)
	}
	vx = uint64(z_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx)))
	return registerIndexA, registerIndexB, vx
}

func generateCompareBranch(prefix byte, opcode byte) func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		srcReg, dstReg, offset := extractTwoRegsOneOffset(inst.Args)
		src := regInfoList[srcReg]
		dst := regInfoList[dstReg]
		rex := byte(0x40)
		if src.REXBit == 1 {
			rex |= 0x04
		}
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | (src.RegBits << 3) | dst.RegBits)
		return append([]byte{rex, 0x39, modrm, prefix, opcode}, encodeU32(uint32(offset))...), nil
	}
}
