package pvm

import (
	"github.com/colorfulnotion/jam/types"
)

// A.5.10. Instructions with Arguments of Two Registers & One Immediate.
func extractDoublet(args []byte) (reg1, reg2 int, imm uint64) {
	reg1 = min(12, int(args[0]&0x0F))
	reg2 = min(12, int(args[0]>>4))
	lx := min(4, max(0, len(args)-1))
	if lx == 0 {
		lx = 1
		args = append(args, 0)
	}
	imm = x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
	return
}

func generateRotateRight32Imm() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		dstReg, _, imm := extractDoublet(inst.Args)
		dst := regInfoList[dstReg]
		//src := regInfoList[srcReg]
		rex := byte(0x40)
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | (1 << 3) | dst.RegBits) // 001b for ROR
		return []byte{rex, 0xC1, modrm, byte(imm & 0xFF)}, nil
	}
}

func generateImmShiftOp64(opcode byte, subcode byte) func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		dstReg, _, imm := extractDoublet(inst.Args)
		dst := regInfoList[dstReg]
		//src := regInfoList[srcReg]
		rex := byte(0x48)
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | (subcode << 3) | dst.RegBits)
		return []byte{rex, opcode, modrm, byte(imm & 0xFF)}, nil
	}
}

func generateNegAddImm64() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		dstReg, _, imm := extractDoublet(inst.Args)
		dst := regInfoList[dstReg]
		rex := byte(0x48)
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		// mov dst, imm32
		mov := []byte{rex, 0xC7, 0xC0 | dst.RegBits}
		mov = append(mov, encodeU32(uint32(imm))...) // imm32 little endian
		// not dst
		not := []byte{rex, 0xF7, 0xD0 | dst.RegBits}
		// add dst, 1
		add := []byte{rex, 0x83, 0xC0 | dst.RegBits, 0x01}
		return append(append(mov, not...), add...), nil
	}
}

// TODO: make this different for 32-bit and 64-bit
func generateNegAddImm32() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		dstReg, _, imm := extractDoublet(inst.Args)
		dst := regInfoList[dstReg]
		rex := byte(0x48)
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		// mov dst, imm32
		mov := []byte{rex, 0xC7, 0xC0 | dst.RegBits}
		mov = append(mov, encodeU32(uint32(imm))...) // imm32 little endian
		// not dst
		not := []byte{rex, 0xF7, 0xD0 | dst.RegBits}
		// add dst, 1
		add := []byte{rex, 0x83, 0xC0 | dst.RegBits, 0x01}
		return append(append(mov, not...), add...), nil
	}
}

func generateImmShiftOp32(opcode byte, subcode byte) func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		dstReg, _, imm := extractDoublet(inst.Args)
		dst := regInfoList[dstReg]
		//src := regInfoList[srcReg]
		rex := byte(0x40)
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | (subcode << 3) | dst.RegBits)
		return []byte{rex, opcode, modrm, byte(imm & 0xFF)}, nil
	}
}

func generateImmSetCondOp32(setcc byte) func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		dstReg, _, imm := extractDoublet(inst.Args)
		dst := regInfoList[dstReg]
		tmp := regInfoList[0] // temp reg
		// mov tmp, imm32
		cmp := []byte{0xB8 + tmp.RegBits}
		cmp = append(cmp, encodeU32(uint32(imm))...)
		// cmp dst, tmp
		cmpr := []byte{0x39, 0xC0 | (tmp.RegBits << 3) | dst.RegBits}
		// setcc dst
		setccInst := []byte{0x0F, setcc, 0xC0 | dst.RegBits}
		return append(append(cmp, cmpr...), setccInst...), nil
	}
}

// Implements dst := src * imm (32-bit immediate)
func generateImmMulOp32() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		dstReg, srcReg, imm := extractDoublet(inst.Args)
		src := regInfoList[srcReg]
		dst := regInfoList[dstReg]

		// 1) MOV r32_dst, r32_src
		rexMov := byte(0x40) // no W, only extension bits
		if src.REXBit == 1 {
			rexMov |= 0x04 // REX.R for src in ModRM.reg
		}
		if dst.REXBit == 1 {
			rexMov |= 0x01 // REX.B for dst in ModRM.rm
		}
		modrmMov := byte(0xC0 | (src.RegBits << 3) | dst.RegBits)
		code := []byte{rexMov, 0x89, modrmMov} // MOV r/m32(dst), r32(src)

		// 2) IMUL r32_dst, r32_src, imm32
		rexMul := byte(0x40)
		if dst.REXBit == 1 {
			rexMul |= 0x04 // REX.R for dst in ModRM.reg
		}
		if src.REXBit == 1 {
			rexMul |= 0x01 // REX.B for src in ModRM.rm
		}
		modrmMul := byte(0xC0 | (dst.RegBits << 3) | src.RegBits)
		code = append(code, rexMul, 0x69, modrmMul)
		code = append(code, encodeU32(uint32(imm))...) // imm32 little-endian
		return code, nil
	}
}

// Implements dst := src * imm (64-bit immediate)
func generateImmMulOp64() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		dstReg, srcReg, imm := extractDoublet(inst.Args)
		src := regInfoList[srcReg]
		dst := regInfoList[dstReg]

		// 1) MOV r64_dst, r64_src
		rexMov := byte(0x48) // W=1 for 64-bit
		if src.REXBit == 1 {
			rexMov |= 0x04 // REX.R for src
		}
		if dst.REXBit == 1 {
			rexMov |= 0x01 // REX.B for dst
		}
		modrmMov := byte(0xC0 | (src.RegBits << 3) | dst.RegBits)
		code := []byte{rexMov, 0x89, modrmMov} // MOV r/m64(dst), r64(src)

		// 2) IMUL r64_dst, r64_src, imm32 (sign-extended)
		rexMul := byte(0x48)
		if dst.REXBit == 1 {
			rexMul |= 0x04 // REX.R for dst
		}
		if src.REXBit == 1 {
			rexMul |= 0x01 // REX.B for src
		}
		modrmMul := byte(0xC0 | (dst.RegBits << 3) | src.RegBits)
		code = append(code, rexMul, 0x69, modrmMul)
		code = append(code, encodeU32(uint32(imm))...) // imm32
		return code, nil
	}
}

// Implements: dst := src + imm (32-bit)
func generateBinaryImm32() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		dstReg, srcReg, imm := extractDoublet(inst.Args)
		dst := regInfoList[dstReg]
		src := regInfoList[srcReg]

		// 1) MOV r32_dst, r32_src
		// REX.W=0, but include REX if either reg ≥ r8 for extension
		rexMov := byte(0x40)
		if src.REXBit == 1 {
			rexMov |= 0x04 // REX.R
		}
		if dst.REXBit == 1 {
			rexMov |= 0x01 // REX.B
		}
		// Opcode 0x89 /r: MOV r/m32, r32
		modrmMov := byte(0xC0 | (src.RegBits << 3) | dst.RegBits)
		code := []byte{rexMov, 0x89, modrmMov}

		// 2) ADD r32_dst, imm32
		// REX.W=0, but include REX if dst ≥ r8
		rexAdd := byte(0x40)
		if dst.REXBit == 1 {
			rexAdd |= 0x01 // REX.B
		}
		// Opcode 0x81 /0: ADD r/m32, imm32
		modrmAdd := byte(0xC0 | (0 << 3) | dst.RegBits)
		code = append(code, rexAdd, 0x81, modrmAdd)
		// append 4-byte little-endian immediate
		code = append(code, encodeU32(uint32(imm))...)

		return code, nil
	}
}

// Implements a 64-bit immediate binary op (e.g. ADD_IMM_64)
func generateImmBinaryOp64(opcode byte, subcode byte) func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		dstReg, srcReg, imm := extractDoublet(inst.Args)
		dst := regInfoList[dstReg]
		src := regInfoList[srcReg]

		// 1) MOV r64_dst, r64_src
		rexMov := byte(0x48)
		if src.REXBit == 1 {
			rexMov |= 0x04
		}
		if dst.REXBit == 1 {
			rexMov |= 0x01
		}
		modrmMov := byte(0xC0 | (src.RegBits << 3) | dst.RegBits)
		code := []byte{rexMov, 0x89, modrmMov}

		// 2) OPCODE r64_dst, imm32 (sign-extended)
		rexOp := byte(0x48)
		if dst.REXBit == 1 {
			rexOp |= 0x01
		}
		modrmOp := byte(0xC0 | (subcode << 3) | dst.RegBits)
		code = append(code, rexOp, opcode, modrmOp)
		code = append(code, encodeU32(uint32(imm))...)

		return code, nil
	}
}

func generateLoadIndSignExtend(prefix byte, opcode byte, is64bit bool) func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		dstReg, srcReg, _ := extractDoublet(inst.Args)
		dst := regInfoList[dstReg]
		src := regInfoList[srcReg]

		rex := byte(0x40)
		if is64bit {
			rex |= 0x08 // REX.W
		}
		if dst.REXBit == 1 {
			rex |= 0x04 // REX.R
		}
		if src.REXBit == 1 {
			rex |= 0x01 // REX.B
		}

		modrm := byte(0x80 | (dst.RegBits << 3) | src.RegBits) // mod = 10
		return []byte{prefix, rex, opcode, modrm, 0x00}, nil   // disp = 0
	}
}

func generateLoadInd(opcode byte, is64bit bool, size int) func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		dstReg, srcReg, _ := extractDoublet(inst.Args)
		dst := regInfoList[dstReg]
		src := regInfoList[srcReg]

		rex := byte(0x40)
		if is64bit {
			rex |= 0x08 // REX.W
		}
		if dst.REXBit == 1 {
			rex |= 0x04 // REX.R
		}
		if src.REXBit == 1 {
			rex |= 0x01 // REX.B
		}

		modrm := byte(0x80 | (dst.RegBits << 3) | src.RegBits) // mod = 10
		return []byte{rex, opcode, modrm, 0x00}, nil           // disp = 0
	}
}

func generateStoreIndirect(opcode byte, is16bit bool, size int) func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		dstReg, srcReg, _ := extractDoublet(inst.Args)
		src := regInfoList[srcReg]
		dst := regInfoList[dstReg]

		rex := byte(0x40)
		if size == 8 {
			rex |= 0x08 // REX.W
		}
		if src.REXBit == 1 {
			rex |= 0x04 // REX.R
		}
		if dst.REXBit == 1 {
			rex |= 0x01 // REX.B
		}

		code := []byte{}
		if is16bit {
			code = append(code, 0x66)
		}
		modrm := byte(0x80 | (src.RegBits << 3) | dst.RegBits) // mod = 10, disp = 0
		code = append(code, rex, opcode, modrm, 0x00)          // disp = 0
		return code, nil
	}
}

func generateImmBinaryOp32(opcode byte, subcode byte) func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		dstReg, _, imm := extractDoublet(inst.Args)
		dst := regInfoList[dstReg]
		// src := regInfoList[srcReg]
		rex := byte(0x40)
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | (subcode << 3) | dst.RegBits)
		code := []byte{rex, opcode, modrm}
		code = append(code, encodeU32(uint32(imm))...) // imm32
		return code, nil
	}
}
