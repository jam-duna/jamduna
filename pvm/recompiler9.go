package pvm

import "fmt"

// A.5.9. Instructions with Arguments of Two Registers.
func extractTwoRegisters(args []byte) (reg1, reg2 int) {
	reg1 = min(12, int(args[0]&0x0F))
	reg2 = min(12, int(args[0]>>4))
	return
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

func generateBitCount64() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		dstReg, srcReg := extractTwoRegisters(inst.Args)
		dst := regInfoList[dstReg]
		src := regInfoList[srcReg]
		rex := byte(0x48)
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | (dst.RegBits << 3) | src.RegBits)
		return []byte{0xF3, rex, 0x0F, 0xB8, modrm}, nil
	}
}

func generateBitCount32() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		dstReg, srcReg := extractTwoRegisters(inst.Args)
		dst := regInfoList[dstReg]
		src := regInfoList[srcReg]
		modrm := byte(0xC0 | (dst.RegBits << 3) | src.RegBits)
		return []byte{0xF3, 0x0F, 0xB8, modrm}, nil
	}
}

func generateLeadingZeros64() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		dstReg, srcReg := extractTwoRegisters(inst.Args)
		dst := regInfoList[dstReg]
		src := regInfoList[srcReg]
		rex := byte(0x48)
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | (dst.RegBits << 3) | src.RegBits)
		return []byte{0xF3, rex, 0x0F, 0xBD, modrm}, nil
	}
}

func generateLeadingZeros32() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		dstReg, srcReg := extractTwoRegisters(inst.Args)
		dst := regInfoList[dstReg]
		src := regInfoList[srcReg]
		modrm := byte(0xC0 | (dst.RegBits << 3) | src.RegBits)
		return []byte{0xF3, 0x0F, 0xBD, modrm}, nil
	}
}

func generateTrailingZeros32() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		dstReg, srcReg := extractTwoRegisters(inst.Args)
		dst := regInfoList[dstReg]
		src := regInfoList[srcReg]
		modrm := byte(0xC0 | (dst.RegBits << 3) | src.RegBits)
		return []byte{0xF3, 0x0F, 0xBC, modrm}, nil
	}
}

func generateTrailingZeros64() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		dstReg, srcReg := extractTwoRegisters(inst.Args)
		dst := regInfoList[dstReg]
		src := regInfoList[srcReg]
		rex := byte(0x48)
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | (dst.RegBits << 3) | src.RegBits)
		return []byte{0xF3, rex, 0x0F, 0xBC, modrm}, nil
	}
}

func generateSignExtend8() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		dstReg, srcReg := extractTwoRegisters(inst.Args)
		dst := regInfoList[dstReg]
		src := regInfoList[srcReg]
		modrm := byte(0xC0 | (dst.RegBits << 3) | src.RegBits)
		return []byte{0x48, 0x0F, 0xBE, modrm}, nil
	}
}

func generateSignExtend16() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		dstReg, srcReg := extractTwoRegisters(inst.Args)
		dst := regInfoList[dstReg]
		src := regInfoList[srcReg]
		modrm := byte(0xC0 | (dst.RegBits << 3) | src.RegBits)
		return []byte{0x48, 0x0F, 0xBF, modrm}, nil
	}
}

func generateZeroExtend16() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		dstReg, srcReg := extractTwoRegisters(inst.Args)
		dst := regInfoList[dstReg]
		src := regInfoList[srcReg]
		modrm := byte(0xC0 | (dst.RegBits << 3) | src.RegBits)
		return []byte{0x48, 0x0F, 0xB7, modrm}, nil
	}
}

func generateReverseBytes64() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		dstReg, _ := extractTwoRegisters(inst.Args)
		dst := regInfoList[dstReg]
		// src := regInfoList[srcReg]

		opcode := 0xC8 + dst.RegBits
		rex := byte(0x48)
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		return []byte{rex, 0x0F, opcode}, nil
	}
}
