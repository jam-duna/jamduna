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
	rex := byte(0x48)
	if dst.REXBit == 1 {
		rex |= 0x01
	}
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
	dstReg, srcReg := extractTwoRegisters(inst.Args)
	dst := regInfoList[dstReg]
	src := regInfoList[srcReg]
	rex := byte(0x48)
	if dst.REXBit == 1 {
		rex |= 0x01
	}
	modrm := byte(0xC0 | (dst.RegBits << 3) | src.RegBits)
	return []byte{0xF3, rex, 0x0F, 0xBD, modrm}
}

func generateLeadingZeros32(inst Instruction) []byte {
	dstReg, srcReg := extractTwoRegisters(inst.Args)
	dst := regInfoList[dstReg]
	src := regInfoList[srcReg]
	modrm := byte(0xC0 | (dst.RegBits << 3) | src.RegBits)
	return []byte{0xF3, 0x0F, 0xBD, modrm}
}

func generateTrailingZeros32(inst Instruction) []byte {
	dstReg, srcReg := extractTwoRegisters(inst.Args)
	dst := regInfoList[dstReg]
	src := regInfoList[srcReg]
	modrm := byte(0xC0 | (dst.RegBits << 3) | src.RegBits)
	return []byte{0xF3, 0x0F, 0xBC, modrm}
}

func generateTrailingZeros64(inst Instruction) []byte {
	dstReg, srcReg := extractTwoRegisters(inst.Args)
	dst := regInfoList[dstReg]
	src := regInfoList[srcReg]
	rex := byte(0x48)
	if dst.REXBit == 1 {
		rex |= 0x01
	}
	modrm := byte(0xC0 | (dst.RegBits << 3) | src.RegBits)
	return []byte{0xF3, rex, 0x0F, 0xBC, modrm}
}

func generateSignExtend8(inst Instruction) []byte {
	dstReg, srcReg := extractTwoRegisters(inst.Args)
	dst := regInfoList[dstReg]
	src := regInfoList[srcReg]
	modrm := byte(0xC0 | (dst.RegBits << 3) | src.RegBits)
	return []byte{0x48, 0x0F, 0xBE, modrm}
}

func generateSignExtend16(inst Instruction) []byte {
	dstReg, srcReg := extractTwoRegisters(inst.Args)
	dst := regInfoList[dstReg]
	src := regInfoList[srcReg]
	modrm := byte(0xC0 | (dst.RegBits << 3) | src.RegBits)
	return []byte{0x48, 0x0F, 0xBF, modrm}
}

func generateZeroExtend16(inst Instruction) []byte {
	dstReg, srcReg := extractTwoRegisters(inst.Args)
	dst := regInfoList[dstReg]
	src := regInfoList[srcReg]
	modrm := byte(0xC0 | (dst.RegBits << 3) | src.RegBits)
	return []byte{0x48, 0x0F, 0xB7, modrm}
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
