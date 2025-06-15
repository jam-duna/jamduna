package pvm

import "encoding/binary"

// A.5.10. Instructions with Arguments of Two Registers & One Immediate.

func generateCmovCmpOp64(cc byte) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		regAIdx, regBIdx, imm := extractTwoRegsOneImm(inst.Args)
		dst := regInfoList[regAIdx]  // destination register (r64_regA)
		cond := regInfoList[regBIdx] // condition register (r64_regB)
		tmp := regInfoList[12]       // scratch r12

		var code []byte

		// 1) MOVABS tmp, imm64
		rexTmp := byte(0x48) // REX.W
		if tmp.REXBit == 1 {
			rexTmp |= 0x01
		}
		movTmpOp := byte(0xB8 | tmp.RegBits) // B8+rd
		code = append(code, rexTmp, movTmpOp)
		// imm64 little-endian
		for i := 0; i < 8; i++ {
			code = append(code, byte(imm>>(8*i)))
		}

		// 2) MOV r64_dst, r64_dst  (turn regA into its original value)
		// Actually we want preserve original rA in dst already.
		// Skip: rA already holds old value; so no MOV needed.

		// 2) TEST cond, cond  → ZF=1 if cond==0
		rexTest := byte(0x48)
		if cond.REXBit == 1 {
			rexTest |= 0x04 | 0x01
		}
		modrmTest := byte(0xC0 | (cond.RegBits << 3) | cond.RegBits)
		code = append(code, rexTest, 0x85, modrmTest) // 85 /r = TEST r/m64, r64

		// 3) CMOVE dst, tmp  → if ZF=1 (cond==0), move imm into dst
		rexCmov := byte(0x48)
		if dst.REXBit == 1 {
			rexCmov |= 0x04
		}
		if tmp.REXBit == 1 {
			rexCmov |= 0x01
		}
		modrmCmov := byte(0xC0 | (dst.RegBits << 3) | tmp.RegBits)
		code = append(code, rexCmov, 0x0F, cc, modrmCmov) // 0F 44 /r = CMOVE

		return code
	}
}

// Implements CMOV_IZ_IMM: if (r64_cond == 0) r64_dst = imm
func generateCmovIzImm(inst Instruction) []byte {
	// extract: dst‐index, cond‐index, imm64
	dstIdx, condIdx, imm := extractTwoRegsOneImm(inst.Args)
	dst := regInfoList[dstIdx]
	cond := regInfoList[condIdx]

	var code []byte

	// 1) MOVABS r12, imm64
	//    0x49 = REX.W|REX.B, 0xBC = MOVABS r12 (B8+4)
	code = append(code, 0x49, 0xBC)
	for i := 0; i < 8; i++ {
		code = append(code, byte(imm>>(8*i)))
	}

	// 2) TEST cond, cond  → sets ZF if cond==0
	rexTest := byte(0x48) // REX.W
	if cond.REXBit == 1 {
		rexTest |= 0x04 | 0x01 // REX.R and REX.B
	}
	modrmTest := byte(0xC0 | (cond.RegBits << 3) | cond.RegBits)
	code = append(code, rexTest, 0x85, modrmTest) // 85 /r = TEST r/m64, r64

	// 3) CMOVE dst, r12  (0x0F 44 /r)
	rexCmov := byte(0x48) // REX.W
	if dst.REXBit == 1 {
		rexCmov |= 0x04 // REX.R for dst
	}
	rexCmov |= 0x01 // REX.B for r12 (rm=4)
	modrmCmov := byte(0xC0 | (dst.RegBits << 3) | 0x04)
	code = append(code, rexCmov, 0x0F, 0x44, modrmCmov)

	return code
}

// Implements r32_dst = r32_src rotate_right imm8
func generateRotateRight32Imm(inst Instruction) []byte {
	dstIdx, srcIdx, imm := extractTwoRegsOneImm(inst.Args)
	dst := regInfoList[dstIdx]
	src := regInfoList[srcIdx]

	var code []byte

	// 1) MOV r32_dst, r32_src
	rex1 := byte(0x40)
	if src.REXBit == 1 {
		rex1 |= 0x04
	} // REX.R
	if dst.REXBit == 1 {
		rex1 |= 0x01
	} // REX.B
	modrm1 := byte(0xC0 | (src.RegBits << 3) | dst.RegBits)
	code = append(code, rex1, 0x89, modrm1)

	// 2) ROR r/m32(dst), imm8
	rex2 := byte(0x40)
	if dst.REXBit == 1 {
		rex2 |= 0x01
	}
	modrm2 := byte(0xC0 | (1 << 3) | dst.RegBits) // /1 for ROR
	code = append(code, rex2, 0xC1, modrm2, byte(imm&0xFF))

	return code
}

// Implements r64_dst = r64_src << imm8  (or >>, SAR, ROR with different subcodes)
func generateImmShiftOp64(opcode byte, subcode byte) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		dstIdx, srcIdx, imm := extractTwoRegsOneImm(inst.Args)
		dst := regInfoList[dstIdx]
		src := regInfoList[srcIdx]

		var code []byte

		// 1) MOV r64_dst, r64_src
		rex1 := byte(0x48)
		if src.REXBit == 1 {
			rex1 |= 0x04
		}
		if dst.REXBit == 1 {
			rex1 |= 0x01
		}
		modrm1 := byte(0xC0 | (src.RegBits << 3) | dst.RegBits)
		code = append(code, rex1, 0x89, modrm1)

		// 2) opcode r/m64(dst), imm8
		rex2 := byte(0x48)
		if dst.REXBit == 1 {
			rex2 |= 0x01
		}
		modrm2 := byte(0xC0 | (subcode << 3) | dst.RegBits)
		code = append(code, rex2, opcode, modrm2, byte(imm&0xFF))

		return code
	}
}

// ALT variant: dst = imm64 op (src64 & 63)
//
//	subcode = 4 (SHL), 5 (SHR), 7 (SAR)
func generateImmShiftOp64Alt(subcode byte) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		dstIdx, srcIdx, imm := extractTwoRegsOneImm(inst.Args)
		dst := regInfoList[dstIdx]
		src := regInfoList[srcIdx]

		var code []byte

		// 1) MOVABS r64_dst, imm64
		//    REX.W + B8+rd + imm64
		rexMov := byte(0x48)
		if dst.REXBit == 1 {
			rexMov |= 0x01 // REX.B for dst high regs
		}
		movOp := byte(0xB8 | dst.RegBits)
		code = append(code, rexMov, movOp)
		for i := 0; i < 8; i++ {
			code = append(code, byte(imm>>(8*i)))
		}

		// 2) XCHG RCX, r64_src  ; swap count into CL, save old RCX in src
		rexX := byte(0x48)
		if src.REXBit == 1 {
			rexX |= 0x04 // REX.R for reg=src
		}
		// reg = src.RegBits, rm = 1 (RCX)
		modrmX := byte(0xC0 | (src.RegBits << 3) | 0x01)
		code = append(code, rexX, 0x87, modrmX) // 87 /r = XCHG r/m64, r64

		// 3) D3 /subcode r/m64(dst), CL  ; 64-bit CL‐counted shift
		rexSh := byte(0x48)
		if dst.REXBit == 1 {
			rexSh |= 0x01 // REX.B for dst
		}
		modrmSh := byte(0xC0 | (subcode << 3) | dst.RegBits)
		code = append(code, rexSh, 0xD3, modrmSh)

		// 4) XCHG RCX, r64_src  ; restore original RCX
		code = append(code, rexX, 0x87, modrmX)

		return code
	}
}

// Implements dst64 = imm64 - src64
func generateNegAddImm64(inst Instruction) []byte {
	dstIdx, srcIdx, imm := extractTwoRegsOneImm(inst.Args)
	dst := regInfoList[dstIdx]
	src := regInfoList[srcIdx]

	var code []byte

	// 1) MOVABS r64_dst, imm64
	//    REX.W + B8+rd, then 8‐byte little‐endian immediate
	rexMov := byte(0x48)
	if dst.REXBit == 1 {
		rexMov |= 0x01 // REX.B for high registers
	}
	movOp := byte(0xB8 | dst.RegBits)
	code = append(code, rexMov, movOp)
	for i := 0; i < 8; i++ {
		code = append(code, byte(imm>>(8*i)))
	}

	// 2) SUB r64_dst, r64_src
	//    REX.W + 29 /r
	rexSub := byte(0x48)
	if src.REXBit == 1 {
		rexSub |= 0x04 // REX.R for reg field = src
	}
	if dst.REXBit == 1 {
		rexSub |= 0x01 // REX.B for rm field = dst
	}
	modrmSub := byte(0xC0 | (src.RegBits << 3) | dst.RegBits)
	code = append(code, rexSub, 0x29, modrmSub)

	return code
}

// Implements dst64 = (imm32 - src32) mod 2^64,
// which for a 32-bit imm and 32-bit src means 64-bit wraparound.
func generateNegAddImm32(inst Instruction) []byte {
	dstIdx, srcIdx, imm := extractTwoRegsOneImm(inst.Args)
	dst := regInfoList[dstIdx]
	src := regInfoList[srcIdx]

	var code []byte

	// 1) MOV r64_dst, r64_src
	rex1 := byte(0x48)
	if src.REXBit == 1 {
		rex1 |= 0x04
	} // REX.R
	if dst.REXBit == 1 {
		rex1 |= 0x01
	} // REX.B
	modrm1 := byte(0xC0 | (src.RegBits << 3) | dst.RegBits)
	code = append(code, rex1, 0x89, modrm1)

	// 2) NEG r64_dst  → REX.W + F7 /3
	rex2 := byte(0x48)
	if dst.REXBit == 1 {
		rex2 |= 0x01
	}
	modrm2 := byte(0xC0 | (0x03 << 3) | dst.RegBits) // reg=3 for NEG
	code = append(code, rex2, 0xF7, modrm2)

	// 3) ADD r64_dst, imm32  → REX.W + 81 /0 id
	rex3 := byte(0x48)
	if dst.REXBit == 1 {
		rex3 |= 0x01
	}
	modrm3 := byte(0xC0 | (0x00 << 3) | dst.RegBits) // reg=0 for ADD
	code = append(code, rex3, 0x81, modrm3)
	code = append(code, encodeU32(uint32(imm))...)

	return code
}

// Implements 32-bit immediate‐based shifts and their “ALT” variants.
//   - NORMAL (alt=false):  r32_dst = r32_src op (imm & 31)   → uses C1 /subcode imm8
//   - ALT    (alt=true):   r32_dst = imm op (r32_src & 31)   → mov imm→dst + CL‐based D3 /subcode
//
// Implements 32-bit immediate‐based shifts and their “ALT” variants.
//   - NORMAL (alt=false):  r32_dst = r32_src op (imm & 31)   → uses C1 /subcode imm8
//   - ALT    (alt=true):   r32_dst = imm op (r32_src & 31)    → MOV imm→dst + CL‐based D3 /subcode
//
// Implements 32-bit immediate-based shifts and their “ALT” variants in full 64-bit registers.
//   - NORMAL (alt=false):  dst = sign-extend(src32) op (imm&31)       (SAR), or zero-extend for logical shifts (SHL, SHR)
//   - ALT    (alt=true):   dst = imm64 op (src32&31)
func generateImmShiftOp32Alt(subcode byte) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		dstIdx, srcIdx, imm := extractTwoRegsOneImm(inst.Args)
		dst := regInfoList[dstIdx]
		src := regInfoList[srcIdx]
		var code []byte

		// ALT: immediate on left, register count
		// 1) MOVABS dst64, imm64
		rexMov := byte(0x49) // REX.W + REX.B for r12–r15, but here selects dst
		movOp := byte(0xB8 | dst.RegBits)
		code = append(code, rexMov, movOp)
		for i := 0; i < 8; i++ {
			code = append(code, byte(imm>>(8*i)))
		}
		// 2) XCHG RCX, src64
		rexX := byte(0x48)
		if src.REXBit == 1 {
			rexX |= 0x04
		}
		modrmX := byte(0xC0 | (src.RegBits << 3) | 0x01)
		code = append(code, rexX, 0x87, modrmX) // XCHG r/m64, r64
		// 3) Shift full 64-bit: D3 /subcode dst, CL
		rexSh := byte(0x48)
		if dst.REXBit == 1 {
			rexSh |= 0x01
		}
		modrmSh := byte(0xC0 | (subcode << 3) | dst.RegBits)
		code = append(code, rexSh, 0xD3, modrmSh)
		// 4) restore RCX
		code = append(code, rexX, 0x87, modrmX)

		return code
	}
}

// Implements 32-bit immediate‐based shifts and their “ALT” variants.
//   - NORMAL (alt=false):  r32_dst = r32_src op (imm & 31)   → uses C1 /subcode imm8
//   - ALT    (alt=true):   r32_dst = imm op (r32_src & 31)    → MOV imm→dst + CL‐based D3 /subcode
func generateImmShiftOp32(opcode byte, subcode byte, alt bool) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		dstIdx, srcIdx, imm := extractTwoRegsOneImm(inst.Args)
		dst := regInfoList[dstIdx]
		src := regInfoList[srcIdx]

		var code []byte

		if alt {
			// ALT: immediate value on left, register count via CL
			// 1) MOV r32_dst, imm32 (C7 /0 id)
			rexMov := byte(0x40)
			if dst.REXBit == 1 {
				rexMov |= 0x01
			}
			modrmMov := byte(0xC0 | (0 << 3) | dst.RegBits)
			code = append(code, rexMov, 0xC7, modrmMov)
			code = append(code, encodeU32(uint32(imm))...)

			// 2) XCHG ECX, r32_src  ; load count into CL
			rexX := byte(0x40)
			if src.REXBit == 1 {
				rexX |= 0x04
			}
			modrmX := byte(0xC0 | (src.RegBits << 3) | 0x01)
			code = append(code, rexX, 0x87, modrmX)

			// 3) D3 /subcode r/m32(dst), CL
			rexSh := byte(0x40)
			if dst.REXBit == 1 {
				rexSh |= 0x01
			}
			modrmSh := byte(0xC0 | (subcode << 3) | dst.RegBits)
			code = append(code, rexSh, 0xD3, modrmSh)

			// 4) XCHG ECX, r32_src  ; restore ECX and src
			code = append(code, rexX, 0x87, modrmX)

			// 5) if SAR (subcode==7), sign-extend 32->64: MOVSXD r64_dst, r/m32(dst)
			if subcode == 7 {
				rexSX := byte(0x48)
				if dst.REXBit == 1 {
					rexSX |= 0x05
				}
				modrmSX := byte(0xC0 | (dst.RegBits << 3) | dst.RegBits)
				code = append(code, rexSX, 0x63, modrmSX)
			}
		} else {
			// NORMAL: register value on left, immediate count
			// 1) MOV r32_dst, r32_src (89 /r)
			rexMov := byte(0x40)
			if src.REXBit == 1 {
				rexMov |= 0x04
			}
			if dst.REXBit == 1 {
				rexMov |= 0x01
			}
			modrmMov := byte(0xC0 | (src.RegBits << 3) | dst.RegBits)
			code = append(code, rexMov, 0x89, modrmMov)

			// 2) C1 /subcode r/m32(dst), imm8
			rexSh := byte(0x40)
			if dst.REXBit == 1 {
				rexSh |= 0x01
			}
			modrmSh := byte(0xC0 | (subcode << 3) | dst.RegBits)
			code = append(code, rexSh, opcode, modrmSh, byte(imm&0xFF))

			// 3) if SAR (subcode==7), sign-extend 32->64: MOVSXD r64_dst, r/m32(dst)
			if subcode == 7 {
				rexSX := byte(0x48)
				if dst.REXBit == 1 {
					rexSX |= 0x05
				}
				modrmSX := byte(0xC0 | (dst.RegBits << 3) | dst.RegBits)
				code = append(code, rexSX, 0x63, modrmSX)
			}
		}

		return code
	}
}

// Implements dst32 = (src32 <cond> imm32) ? 1 : 0
func generateImmSetCondOp32(setcc byte) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		dstIdx, srcIdx, imm := extractTwoRegsOneImm(inst.Args)

		dstInfo := regInfoList[dstIdx]
		srcInfo := regInfoList[srcIdx]

		var code []byte

		// 1) XOR r32_dst, r32_dst  → zero-clear the full 32-bit dst
		rex := byte(0x40) // REX prefix, W=0 for 32-bit ops
		if dstInfo.REXBit == 1 {
			// set both REX.R (for ModRM.reg) and REX.B (for ModRM.rm)
			rex |= 0x04 | 0x01
		}
		// ModRM: mod=11, reg=dst.RegBits, rm=dst.RegBits
		modrm := byte(0xC0 | (dstInfo.RegBits << 3) | dstInfo.RegBits)
		code = append(code, rex, 0x31, modrm) // 0x31 = XOR r/m32, r32

		// 2) CMP r32_src, imm32  → set flags
		rex = byte(0x40)
		if srcInfo.REXBit == 1 {
			rex |= 0x01 // only REX.B, since reg field=7 (/7 = CMP) is not a register
		}
		// ModRM: mod=11, reg=7, rm=src.RegBits
		modrm = byte(0xC0 | (7 << 3) | srcInfo.RegBits)
		code = append(code, rex, 0x81, modrm)          // 0x81 /7 id = CMP r/m32, imm32
		code = append(code, encodeU32(uint32(imm))...) // imm32 little-endian

		// 3) SETcc r/m8_dst → writes 0 or 1 into low byte
		rex = byte(0x40)
		if dstInfo.REXBit == 1 {
			rex |= 0x01 // only REX.B, since reg field=0 is unused here
		}
		// ModRM: mod=11, reg=0 (ignored), rm=dst.RegBits
		modrm = byte(0xC0 | dstInfo.RegBits)
		code = append(code, rex, 0x0F, setcc, modrm)

		return code
	}
}

// Implements dst := src * imm (32-bit immediate)
func generateImmMulOp32(inst Instruction) []byte {
	dstReg, srcReg, imm := extractTwoRegsOneImm(inst.Args)
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
	return code
}

// Implements dst := src * imm (64-bit immediate)
func generateImmMulOp64(inst Instruction) []byte {
	dstReg, srcReg, imm := extractTwoRegsOneImm(inst.Args)
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
	return code
}

// Implements: dst := sign_extend( i32(src) + imm32 )
func generateBinaryImm32(inst Instruction) []byte {
	dstReg, srcReg, imm := extractTwoRegsOneImm(inst.Args)
	dst := regInfoList[dstReg]
	src := regInfoList[srcReg]

	// 1) MOV r32_dst, r32_src
	rexMov := byte(0x40) // no REX.W
	if src.REXBit == 1 {
		rexMov |= 0x04
	} // REX.R
	if dst.REXBit == 1 {
		rexMov |= 0x01
	} // REX.B
	// 0x89 /r : MOV r/m32, r32
	modrmMov := byte(0xC0 | (src.RegBits << 3) | dst.RegBits)
	code := []byte{rexMov, 0x89, modrmMov}

	// 2) ADD r32_dst, imm32
	rexAdd := byte(0x40)
	if dst.REXBit == 1 {
		rexAdd |= 0x01
	} // REX.B
	// 0x81 /0 id : ADD r/m32, imm32
	modrmAdd := byte(0xC0 | (0 << 3) | dst.RegBits)
	code = append(code, rexAdd, 0x81, modrmAdd)
	code = append(code, encodeU32(uint32(imm))...)

	// 3) MOVSXD r64_dst, r/m32   ; sign-extend low 32→64
	rexSX := byte(0x48) // REX.W
	if dst.REXBit == 1 {
		// set REX.R (bit2) and REX.B (bit0) so both reg-field and rm-field map to dst+8
		rexSX |= 0x05 // 0x04|0x01
	}
	modrmSX := byte(0xC0 | (dst.RegBits << 3) | dst.RegBits)
	code = append(code, rexSX, 0x63, modrmSX)
	return code
}

// Implements a 64-bit immediate binary op (e.g. ADD_IMM_64)
func generateImmBinaryOp64(opcode byte, subcode byte) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		dstReg, srcReg, imm := extractTwoRegsOneImm(inst.Args)
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

		return code
	}
}

// generateLoadIndSignExtend returns a function that generates machine code
func generateLoadIndSignExtend(
	prefix byte, // Optional extra prefix (e.g. 0x66), or 0 if none
	opcode byte, // IR opcode (LOAD_IND_I8, LOAD_IND_I16, LOAD_IND_I32)
	is64bit bool, // true for 64-bit mode
) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		// Extract dest/regA, src/regB, and immediate vx from inst.Args
		regAIndex, regBIndex, vx := extractTwoRegsOneImm(inst.Args)
		regA := regInfoList[regAIndex]
		regB := regInfoList[regBIndex]

		// Construct REX prefix: 0x40 base, W=1 for 64-bit, R/X/B for high registers
		var rex byte = 0x40
		if is64bit {
			rex |= 0x08 // W
		}
		if regA.REXBit != 0 {
			rex |= 0x04 // R
		}
		if regB.REXBit != 0 {
			rex |= 0x02 // X (as SIB.index)
		}
		if BaseReg.REXBit != 0 {
			rex |= 0x01 // B (as SIB.base)
		}

		// ModRM: mod=10 (disp32), reg=regA, rm=100 (SIB)
		modRM := byte(0x02<<6) | (regA.RegBits << 3) | 0x04
		// SIB: scale=0(1), index=regB, base=BaseReg
		sib := byte(0<<6) | (regB.RegBits << 3) | BaseReg.RegBits

		// disp32 as little-endian
		disp := make([]byte, 4)
		binary.LittleEndian.PutUint32(disp, uint32(int32(vx)))

		switch opcode {
		case LOAD_IND_I8:
			// MOVSX r64, byte ptr [BaseReg + regB*1 + disp32] (0F BE /r)
			var code []byte
			if prefix != 0 {
				code = append(code, prefix)
			}
			code = append(code,
				rex,
				0x0F, 0xBE,
				modRM, sib,
			)
			code = append(code, disp...)
			code = append(code, castReg8ToU64(regAIndex)...)
			return code
		case LOAD_IND_I16:
			// 0x66 prefix + MOVSX r64, word ptr [BaseReg + regB*1 + disp32] (0F BF /r)
			code := append(
				[]byte{0x66, rex, 0x0F, 0xBF, modRM, sib},
				disp...,
			)
			code = append(code, castReg16ToU64(regAIndex)...)
			return code
		case LOAD_IND_I32:
			// MOVSXD r64, dword ptr [BaseReg + regB*1 + disp32] (63 /r)
			var code []byte
			if prefix != 0 {
				code = append(code, prefix)
			}
			code = append(code,
				rex,
				0x63,
				modRM, sib,
			)
			code = append(code, disp...)
			return code
		default:
			panic("generateLoadIndSignExtend: invalid opcode")
		}
	}
}

func castReg8ToU64(regIdx int) []byte {
	r := regInfoList[regIdx]
	rex := byte(0x48)
	if r.REXBit != 0 {
		rex |= 0x05 // R=1, B=1 for ModRM.reg=r, ModRM.rm=r
	}
	modRM := byte(0xC0 | (r.RegBits << 3) | r.RegBits)
	// Opcode 0F BE /r → MOVSX r64, r/m8
	return []byte{rex, 0x0F, 0xBE, modRM}
}

func castReg16ToU64(regIdx int) []byte {
	r := regInfoList[regIdx]
	rex := byte(0x48)
	if r.REXBit != 0 {
		rex |= 0x05 // R=1, B=1 for ModRM.reg=r, ModRM.rm=r
	}
	modRM := byte(0xC0 | (r.RegBits << 3) | r.RegBits)
	return []byte{rex, 0x0F, 0xBF, modRM}
}

func castReg32ToU64(regIdx int) []byte {
	r := regInfoList[regIdx]
	rex := byte(0x48)
	if r.REXBit != 0 {
		rex |= 0x05
	}
	modRM := byte(0xC0 | (r.RegBits << 3) | r.RegBits)
	return []byte{rex, 0x63, modRM}
}

// generateLoadInd emits
//
//	REX? opcode ModRM
//
// for plain MOV or MOVZX/MOVSXD sequences (with no 0x0F prefix).
// size parameter is unused here but kept for signature symmetry.
func generateLoadInd(
	prefix byte, // Optional extra prefix (e.g. 0x66), or 0 if none
	opcode byte,
	is64bit bool,
) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		// Extract dest/regA, src/regB, and immediate vx from inst.Args
		regAIndex, regBIndex, vx := extractTwoRegsOneImm(inst.Args)
		regA := regInfoList[regAIndex]
		regB := regInfoList[regBIndex]

		// Construct REX prefix: 0x40 base, W=1 for 64-bit, R/X/B for high registers
		var rex byte = 0x40
		if is64bit {
			rex |= 0x08 // W
		}
		if regA.REXBit != 0 {
			rex |= 0x04 // R
		}
		if regB.REXBit != 0 {
			rex |= 0x02 // X (as SIB.index)
		}
		if BaseReg.REXBit != 0 {
			rex |= 0x01 // B (as SIB.base)
		}

		// ModRM: mod=10 (disp32), reg=regA, rm=100 (SIB)
		modRM := byte(0x02<<6) | (regA.RegBits << 3) | 0x04
		// SIB: scale=0(1), index=regB, base=BaseReg
		sib := byte(0<<6) | (regB.RegBits << 3) | BaseReg.RegBits

		// disp32 as little-endian
		disp := make([]byte, 4)
		binary.LittleEndian.PutUint32(disp, uint32(int32(vx)))

		switch opcode {
		case LOAD_IND_U8:
			// zero-extend byte to 64-bit: MOVZX r64, r/m8 (0F B6 /r)
			var code []byte
			if prefix != 0 {
				code = append(code, prefix)
			}
			code = append(code,
				rex,
				0x0F, 0xB6,
				modRM, sib,
			)
			code = append(code, disp...)
			return code

		case LOAD_IND_U16:
			// zero-extend word to 64-bit: 66 + MOVZX r64, r/m16 (0F B7 /r)
			return append(
				append(
					[]byte{0x66, rex, 0x0F, 0xB7, modRM, sib},
					disp...,
				),
			)

		case LOAD_IND_U32:
			// zero-extend dword to 64-bit: MOV r32, r/m32 (8B /r) without REX.W
			// Writing to r32 clears upper 32 bits, so result is zero-extended in r64
			rex32 := byte(0x40)
			if regA.REXBit != 0 {
				rex32 |= 0x04 // R
			}
			if regB.REXBit != 0 {
				rex32 |= 0x02 // X
			}
			if BaseReg.REXBit != 0 {
				rex32 |= 0x01 // B
			}
			var code32 []byte
			if prefix != 0 {
				code32 = append(code32, prefix)
			}
			code32 = append(code32,
				rex32,
				0x8B,
				modRM, sib,
			)
			code32 = append(code32, disp...)
			return code32

		case LOAD_IND_U64:
			// 64-bit MOV: REX.W + MOV r64, r/m64 (8B /r)
			var code64 []byte
			if prefix != 0 {
				code64 = append(code64, prefix)
			}
			code64 = append(code64,
				rex,
				0x8B,
				modRM, sib,
			)
			code64 = append(code64, disp...)
			return code64

		default:
			panic("generateLoadInd: invalid opcode")
		}
	}
}

func generateStoreIndirect(opcode byte, size int) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		// extract source register, destination register and displacement
		srcIdx, dstIdx, disp64 := extractTwoRegsOneImm(inst.Args)
		base := BaseReg
		disp := uint32(disp64)

		dstInfo := regInfoList[dstIdx]
		srcInfo := regInfoList[srcIdx]
		dstBits, srcBits := dstInfo.RegBits, srcInfo.RegBits

		var buf []byte

		// For sizes < 8, preserve original dst register and apply mask
		if size != 8 {
			// PUSH dst
			if srcInfo.REXBit == 1 {
				buf = append(buf, 0x41) // REX.B
			}
			buf = append(buf, 0x50|srcBits)

			// Compute mask for AND
			var mask uint32
			switch size {
			case 1:
				mask = 0xFF
			case 2:
				mask = 0xFFFF
			case 4:
				mask = 0xFFFFFFFF
			}

			// REX prefix for AND r/m64, imm32 (REX.W=1)
			rexAnd := byte(0x48)
			if srcInfo.REXBit == 1 {
				rexAnd |= 0x01
			}
			buf = append(buf, rexAnd)

			// AND opcode 0x81 /4 id
			modrm := byte((3 << 6) | (4 << 3) | (srcBits & 0x07))
			buf = append(buf, 0x81, modrm)
			buf = append(buf,
				byte(mask), byte(mask>>8),
				byte(mask>>16), byte(mask>>24),
			)

			// POP dst
			if srcInfo.REXBit == 1 {
				buf = append(buf, 0x41)
			}
			buf = append(buf, 0x58|srcBits)
		}

		// MOV [base + index*1 + disp32], dst
		// Legacy prefix for 16-bit operand size
		if size == 2 {
			buf = append(buf, 0x66)
		}
		// REX prefix: bit7 fixed=0, bit6 fixed=1
		rex := byte(0x40)
		// REX.W for 64-bit operand
		if size == 8 {
			rex |= 0x08
		}
		// REX.R, REX.X, REX.B
		if srcInfo.REXBit == 1 {
			rex |= 0x04
		}
		if dstInfo.REXBit == 1 {
			rex |= 0x02
		}
		if base.REXBit == 1 {
			rex |= 0x01
		}
		// emit REX only if non-default
		if rex != 0x40 {
			buf = append(buf, rex)
		}
		// MOV opcode and addressing
		buf = append(buf, opcode)
		modrm := byte((2 << 6) | (srcBits << 3) | 0x04)
		sib := byte((0 << 6) | (dstBits << 3) | (base.RegBits & 0x07))
		buf = append(buf, modrm, sib)
		buf = append(buf,
			byte(disp), byte(disp>>8),
			byte(disp>>16), byte(disp>>24),
		)

		return buf
	}
}
