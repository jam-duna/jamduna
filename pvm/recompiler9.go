package pvm

// A.5.9. Instructions with Arguments of Two Registers.

func generateMoveReg(inst Instruction) []byte {
	dstIdx, srcIdx := extractTwoRegisters(inst.Args)
	dst := regInfoList[dstIdx]
	src := regInfoList[srcIdx]
	rex := byte(0x48)
	if src.REXBit == 1 {
		rex |= 0x04
	}
	if dst.REXBit == 1 {
		rex |= 0x01
	}
	modrm := byte(0xC0 | (src.RegBits << 3) | dst.RegBits)
	return []byte{rex, 0x89, modrm}
}

func generateBitCount64(inst Instruction) []byte {
	dstReg, srcReg := extractTwoRegisters(inst.Args)
	dst := regInfoList[dstReg]
	src := regInfoList[srcReg]

	// REX.W + REX.R if dst high + REX.B if src high
	rex := byte(0x48)
	if dst.REXBit == 1 {
		rex |= 0x04
	} // REX.R
	if src.REXBit == 1 {
		rex |= 0x01
	} // REX.B

	modrm := byte(0xC0 | (dst.RegBits << 3) | src.RegBits)
	return []byte{0xF3, rex, 0x0F, 0xB8, modrm}
}

func generateBitCount32(inst Instruction) []byte {
	dstReg, srcReg := extractTwoRegisters(inst.Args)
	dst := regInfoList[dstReg]
	src := regInfoList[srcReg]
	modrm := byte(0xC0 | (dst.RegBits << 3) | src.RegBits)
	return []byte{0xF3, 0x0F, 0xB8, modrm}
}
func generateLeadingZeros64(inst Instruction) []byte {
	dstIdx, srcIdx := extractTwoRegisters(inst.Args)
	dst := regInfoList[dstIdx]
	src := regInfoList[srcIdx]

	// Build REX prefix: base 0x40, W=1, R for reg (dst), B for rm (src)
	rex := byte(0x40 | 0x08)
	if dst.REXBit == 1 {
		rex |= 0x04 // REX.R
	}
	if src.REXBit == 1 {
		rex |= 0x01 // REX.B
	}
	// ModRM: mod=11 (register), reg=dst.RegBits, rm=src.RegBits
	modrm := byte(0xC0 | (dst.RegBits << 3) | src.RegBits)

	// LZCNT r64, r/m64  →  F3 0F BD /r
	return []byte{
		0xF3,
		rex,
		0x0F,
		0xBD,
		modrm,
	}
}

func generateLeadingZeros32(inst Instruction) []byte {
	dstIdx, srcIdx := extractTwoRegisters(inst.Args)
	dst := regInfoList[dstIdx]
	src := regInfoList[srcIdx]

	// Build a REX prefix with W=0 (32-bit):
	//  0x40 base, +0x04 if we need REX.R (dst high), +0x01 if REX.B (src high)
	rex := byte(0x40)
	if dst.REXBit == 1 {
		rex |= 0x04 // REX.R → dst in the reg field
	}
	if src.REXBit == 1 {
		rex |= 0x01 // REX.B → src in the rm field
	}

	// ModRM: mod=11, reg=dst.RegBits, rm=src.RegBits
	modrm := byte(0xC0 | (dst.RegBits << 3) | src.RegBits)

	// LZCNT r32, r/m32  →  F3 0F BD /r
	return []byte{
		0xF3,
		rex,
		0x0F,
		0xBD,
		modrm,
	}
}

func generateTrailingZeros32(inst Instruction) []byte {
	dstIdx, srcIdx := extractTwoRegisters(inst.Args)
	dst := regInfoList[dstIdx]
	src := regInfoList[srcIdx]

	// Build a REX prefix for 32-bit op:
	// 0x40 base, +0x04 if we need REX.R (dst), +0x01 if REX.B (src)
	rex := byte(0x40)
	if dst.REXBit == 1 {
		rex |= 0x04 // REX.R
	}
	if src.REXBit == 1 {
		rex |= 0x01 // REX.B
	}

	// ModRM: mod=11, reg=dst.RegBits, rm=src.RegBits
	modrm := byte(0xC0 | (dst.RegBits << 3) | src.RegBits)

	// TZCNT r32, r/m32 → F3 0F BC /r
	//
	// Writing to a 32-bit subregister clears the high 32 bits of the full register,
	// giving you exactly uint64(bits.TrailingZeros32(uint32(valueA))).
	return []byte{
		0xF3,
		rex,
		0x0F,
		0xBC,
		modrm,
	}
}

func generateTrailingZeros64(inst Instruction) []byte {
	dstReg, srcReg := extractTwoRegisters(inst.Args)
	dst := regInfoList[dstReg]
	src := regInfoList[srcReg]

	// build REX: 0x40 base, +0x08 for W=1, +0x04 if dst>=8 (REX.R), +0x01 if src>=8 (REX.B)
	rex := byte(0x48) // 0100_1000 = 0x40|0x08
	if dst.REXBit != 0 {
		rex |= 0x04 // REX.R
	}
	if src.REXBit != 0 {
		rex |= 0x01 // REX.B
	}

	// ModR/M: 11b (reg-reg), reg = dst.low3, rm = src.low3
	modrm := byte(0xC0 |
		((dst.RegBits & 7) << 3) |
		(src.RegBits & 7),
	)

	// F3 0F BC /r = TZCNT r64, r/m64
	return []byte{0xF3, rex, 0x0F, 0xBC, modrm}
}

func generateSignExtend8(inst Instruction) []byte {
	dstReg, srcReg := extractTwoRegisters(inst.Args)
	dst := regInfoList[dstReg]
	src := regInfoList[srcReg]

	// build REX prefix: base + W + R(for dst>=8) + B(for src>=8)
	rex := byte(0x40) // 0b0100_0000: REX base
	rex |= 0x08       // 0b0000_1000: REX.W = 1 → 64-bit operand
	if dst.REXBit != 0 {
		rex |= 0x04 // 0b0000_0100: REX.R = 1 → extend reg field
	}
	if src.REXBit != 0 {
		rex |= 0x01 // 0b0000_0001: REX.B = 1 → extend r/m field
	}

	// ModR/M byte: 11b (register) + dst in reg field + src in r/m field
	modrm := byte(0xC0 |
		((dst.RegBits & 0x7) << 3) |
		(src.RegBits & 0x7),
	)

	// 0F BE /r = MOVSX r64, r/m8
	return []byte{rex, 0x0F, 0xBE, modrm}
}

func generateSignExtend16(inst Instruction) []byte {
	dstReg, srcReg := extractTwoRegisters(inst.Args)
	dst := regInfoList[dstReg]
	src := regInfoList[srcReg]

	// REX.W=1 for 64-bit, plus REX.R if dst index ≥8, REX.B if src index ≥8
	rex := byte(0x48)    // 0100 1000: W=1
	if dst.REXBit != 0 { // dst.REXBit is 1 for regs 8–15
		rex |= 0x04 // 0000 0100 → REX.R
	}
	if src.REXBit != 0 {
		rex |= 0x01 // 0000 0001 → REX.B
	}

	// modrm: 11b (register) + reg field + r/m field (both low 3 bits)
	modrm := byte(0xC0 |
		((dst.RegBits & 7) << 3) |
		(src.RegBits & 7))

	// 0F BF /r = MOVSX r64, r/m16
	return []byte{rex, 0x0F, 0xBF, modrm}
}

func generateZeroExtend16(inst Instruction) []byte {
	dstReg, srcReg := extractTwoRegisters(inst.Args)
	dst := regInfoList[dstReg]
	src := regInfoList[srcReg]

	// Build REX: 0x40 base, +0x08 for W, +0x04 if dst>=8, +0x01 if src>=8
	rex := byte(0x40) // 0100 0000
	rex |= 0x08       // 0000 1000 → REX.W = 1
	if dst.REXBit != 0 {
		rex |= 0x04 // 0000 0100 → REX.R
	}
	if src.REXBit != 0 {
		rex |= 0x01 // 0000 0001 → REX.B
	}

	// ModR/M: 11b = register, reg=dst low3, rm=src low3
	modrm := byte(0xC0 |
		((dst.RegBits & 0x7) << 3) |
		(src.RegBits & 0x7),
	)

	// 0F B7 /r = MOVZX r64, r/m16
	return []byte{rex, 0x0F, 0xB7, modrm}
}

func generateReverseBytes64(inst Instruction) []byte {
	dstReg, _ := extractTwoRegisters(inst.Args)
	dst := regInfoList[dstReg]
	// src := regInfoList[srcReg]

	opcode := 0xC8 + dst.RegBits
	rex := byte(0x48)
	if dst.REXBit == 1 {
		rex |= 0x01
	}
	return []byte{rex, 0x0F, opcode}
}
