package pvm

import (
	"encoding/binary"
)

func generateMax() func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		aIdx, bIdx, dstIdx := extractThreeRegs(inst.Args)
		a := regInfoList[aIdx]
		b := regInfoList[bIdx]
		dst := regInfoList[dstIdx]

		var code []byte

		// 1) MOV dst, B
		rex := byte(0x48)
		if b.REXBit == 1 {
			rex |= 0x04
		} // REX.R = B
		if dst.REXBit == 1 {
			rex |= 0x01
		} // REX.B = dst
		modrm := byte(0xC0 | (b.RegBits << 3) | dst.RegBits)
		code = append(code, rex, 0x89, modrm)

		// 2) CMP A, B  → CMP r/m=A, r=B  (signed flags from A-B)
		rex = byte(0x48)
		if b.REXBit == 1 {
			rex |= 0x04
		} // REX.R = B
		if a.REXBit == 1 {
			rex |= 0x01
		} // REX.B = A
		modrm = byte(0xC0 | (b.RegBits << 3) | a.RegBits)
		code = append(code, rex, 0x39, modrm)

		// 3) CMOVGE dst, A  (0F 4D /r) — if A >= B signed, move A
		rex = byte(0x48)
		if a.REXBit == 1 {
			rex |= 0x04
		} // REX.R = A
		if dst.REXBit == 1 {
			rex |= 0x01
		} // REX.B = dst
		modrm = byte(0xC0 | (a.RegBits << 3) | dst.RegBits)
		code = append(code, rex, 0x0F, 0x4D, modrm)

		return code
	}
}

func generateMin() func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		aIdx, bIdx, dstIdx := extractThreeRegs(inst.Args)
		a := regInfoList[aIdx]
		b := regInfoList[bIdx]
		dst := regInfoList[dstIdx]

		var code []byte

		// 1) MOV dst, B
		rex := byte(0x48)
		if b.REXBit == 1 {
			rex |= 0x04
		} // REX.R = B
		if dst.REXBit == 1 {
			rex |= 0x01
		} // REX.B = dst
		modrm := byte(0xC0 | (b.RegBits << 3) | dst.RegBits)
		code = append(code, rex, 0x89, modrm)

		// 2) CMP A, B  → CMP r/m=A, r=B  (signed flags from A-B)
		rex = byte(0x48)
		if b.REXBit == 1 {
			rex |= 0x04
		} // REX.R = B
		if a.REXBit == 1 {
			rex |= 0x01
		} // REX.B = A
		modrm = byte(0xC0 | (b.RegBits << 3) | a.RegBits)
		code = append(code, rex, 0x39, modrm)

		// 3) CMOVL dst, A  (0F 4C /r) — if A < B signed, move A into dst
		rex = byte(0x48)
		if a.REXBit == 1 {
			rex |= 0x04
		} // REX.R = A
		if dst.REXBit == 1 {
			rex |= 0x01
		} // REX.B = dst
		modrm = byte(0xC0 | (a.RegBits << 3) | dst.RegBits)
		code = append(code, rex, 0x0F, 0x4C, modrm)

		return code
	}
}

// generateMinOrMaxU returns a code‐gen function for unsigned MIN or MAX.
// If isMax==false, it implements MIN_U (dst = min(A,B)); if isMax==true, MAX_U.
func generateMinOrMaxU(isMax bool) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		aIdx, bIdx, dstIdx := extractThreeRegs(inst.Args)
		a := regInfoList[aIdx]
		b := regInfoList[bIdx]
		dst := regInfoList[dstIdx]
		tmp := regInfoList[1] // RCX as scratch

		var code []byte

		// choose the CMOV opcode: for MAX_U use CMOVB (0x42), for MIN_U use CMOVA (0x47)
		cmov := byte(0x47) // MIN_U: CMOVA (A>B → move B)
		if isMax {
			cmov = 0x42 // MAX_U: CMOVB (A<B → move B)
		}

		if dstIdx == bIdx {
			// alias: dst and B collide. Save B in RCX, then:
			//   CMP A,B
			//   MOV dst,A
			//   CMOV{B|A} dst,RCX
			//   POP RCX

			// 1) PUSH RCX
			code = append(code, 0x51)

			// 2) MOV RCX, B
			{
				rex := byte(0x48)
				if b.REXBit == 1 {
					rex |= 0x01
				} // REX.B for rm=B
				modrm := byte(0xC0 | (1 << 3) | b.RegBits) // reg=1 (RCX), rm=B
				code = append(code, rex, 0x8B, modrm)      // MOV RCX, r/m64
			}

			// 3) CMP A, B
			{
				rex := byte(0x48)
				if a.REXBit == 1 {
					rex |= 0x01
				} // REX.B for rm=A
				if b.REXBit == 1 {
					rex |= 0x04
				} // REX.R for reg=B
				modrm := byte(0xC0 | (b.RegBits << 3) | a.RegBits) // reg=B, rm=A
				code = append(code, rex, 0x39, modrm)              // CMP r/m64, r64
			}

			// 4) MOV dst, A
			if dstIdx != aIdx {
				rex := byte(0x48)
				if a.REXBit == 1 {
					rex |= 0x04
				} // REX.R for reg=A
				if dst.REXBit == 1 {
					rex |= 0x01
				} // REX.B for rm=dst
				modrm := byte(0xC0 | (a.RegBits << 3) | dst.RegBits)
				code = append(code, rex, 0x89, modrm) // MOV r/m64, r64
			}

			// 5) CMOV{B|A} dst, RCX
			{
				rex := byte(0x48)
				if dst.REXBit == 1 {
					rex |= 0x04
				} // REX.R for reg=dst
				if tmp.REXBit == 1 {
					rex |= 0x01
				} // REX.B for rm=RCX
				modrm := byte(0xC0 | (dst.RegBits << 3) | tmp.RegBits)
				code = append(code, rex, 0x0F, cmov, modrm)
			}

			// 6) POP RCX
			code = append(code, 0x59)

		} else {
			// no alias: dst != B
			//   MOV dst, A
			//   CMP dst, B
			//   CMOV{B|A} dst, B

			// 1) MOV dst, A
			{
				rex := byte(0x48)
				if a.REXBit == 1 {
					rex |= 0x04
				} // REX.R for reg=A
				if dst.REXBit == 1 {
					rex |= 0x01
				} // REX.B for rm=dst
				modrm := byte(0xC0 | (a.RegBits << 3) | dst.RegBits)
				code = append(code, rex, 0x89, modrm) // MOV r/m64, r64
			}

			// 2) CMP dst, B
			{
				rex := byte(0x48)
				if dst.REXBit == 1 {
					rex |= 0x01
				} // REX.B for rm=dst
				if b.REXBit == 1 {
					rex |= 0x04
				} // REX.R for reg=B
				modrm := byte(0xC0 | (b.RegBits << 3) | dst.RegBits)
				code = append(code, rex, 0x39, modrm) // CMP r/m64, r64
			}

			// 3) CMOV{B|A} dst, B
			{
				rex := byte(0x48)
				if dst.REXBit == 1 {
					rex |= 0x04
				} // REX.R for reg=dst
				if b.REXBit == 1 {
					rex |= 0x01
				} // REX.B for rm=B
				modrm := byte(0xC0 | (dst.RegBits << 3) | b.RegBits)
				code = append(code, rex, 0x0F, cmov, modrm)
			}
		}

		return code
	}
}

func generateMinU() func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		aIdx, bIdx, dstIdx := extractThreeRegs(inst.Args)
		a := regInfoList[aIdx]
		b := regInfoList[bIdx]
		dst := regInfoList[dstIdx]
		tmp := regInfoList[1] // use RCX as scratch

		var code []byte

		if dstIdx == bIdx {
			// dst aliases B → save B in tmp first

			// 1) PUSH RCX
			code = append(code, 0x51)

			// 2) MOV RCX, B   ; tmp = original B
			{
				rex := byte(0x48)
				if b.REXBit == 1 {
					rex |= 0x01
				} // REX.B for rm=B
				modrm := byte(0xC0 | (1 << 3) | b.RegBits) // reg=1(RCX), rm=B
				code = append(code, rex, 0x8B, modrm)      // MOV RCX, [B]
			}

			// 3) CMP A, B    ; compare A vs original B
			{
				rex := byte(0x48)
				if a.REXBit == 1 {
					rex |= 0x01
				} // REX.B for A→rm
				if b.REXBit == 1 {
					rex |= 0x04
				} // REX.R for B→reg
				modrm := byte(0xC0 | (b.RegBits << 3) | a.RegBits)
				code = append(code, rex, 0x39, modrm) // CMP r/m64, r64
			}

			// 4) MOV dst, A   ; dst←A
			{
				rex := byte(0x48)
				if a.REXBit == 1 {
					rex |= 0x04
				} // REX.R for A
				if dst.REXBit == 1 {
					rex |= 0x01
				} // REX.B for dst
				modrm := byte(0xC0 | (a.RegBits << 3) | dst.RegBits)
				code = append(code, rex, 0x89, modrm) // MOV r/m64, r64
			}

			// 5) CMOVA dst, RCX ; if A>B unsigned then dst←original B
			{
				rex := byte(0x48)
				if dst.REXBit == 1 {
					rex |= 0x04
				} // REX.R for dst
				if tmp.REXBit == 1 {
					rex |= 0x01
				} // REX.B for RCX
				modrm := byte(0xC0 | (dst.RegBits << 3) | tmp.RegBits)
				code = append(code, rex, 0x0F, 0x47, modrm) // CMOVA r64, r/m64
			}

			// 6) POP RCX
			code = append(code, 0x59)

			return code
		}

		// Non-alias case: dst != B

		// 1) MOV dst, A
		{
			rex := byte(0x48)
			if a.REXBit == 1 {
				rex |= 0x04
			}
			if dst.REXBit == 1 {
				rex |= 0x01
			}
			modrm := byte(0xC0 | (a.RegBits << 3) | dst.RegBits)
			code = append(code, rex, 0x89, modrm)
		}

		// 2) CMP dst, B
		{
			rex := byte(0x48)
			if dst.REXBit == 1 {
				rex |= 0x01
			}
			if b.REXBit == 1 {
				rex |= 0x04
			}
			modrm := byte(0xC0 | (b.RegBits << 3) | dst.RegBits)
			code = append(code, rex, 0x39, modrm)
		}

		// 3) CMOVA dst, B
		{
			rex := byte(0x48)
			if dst.REXBit == 1 {
				rex |= 0x04
			}
			if b.REXBit == 1 {
				rex |= 0x01
			}
			modrm := byte(0xC0 | (dst.RegBits << 3) | b.RegBits)
			code = append(code, rex, 0x0F, 0x47, modrm)
		}

		return code
	}
}

func generateCmovImm(isZero bool) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		dstIdx, condIdx, imm := extractTwoRegsOneImm(inst.Args)
		dst := regInfoList[dstIdx]
		cond := regInfoList[condIdx]
		scratch := regInfoList[1] // RCX

		var code []byte

		// 1) PUSH RCX
		code = append(code, 0x51)

		// 2) MOVABS RCX, imm64
		{
			// REX.W=1, REX.B if RCX>=r8 (it isn't, so REX.B stays 0)
			rex := byte(0x48)
			opcode := byte(0xB8 + scratch.RegBits) // B9 for RCX
			code = append(code, rex, opcode)
			immBytes := make([]byte, 8)
			binary.LittleEndian.PutUint64(immBytes, uint64(imm))
			code = append(code, immBytes...)
		}

		// 3) TEST cond, cond   → sets ZF=1 iff cond==0
		{
			rex := byte(0x48)
			if cond.REXBit == 1 {
				// cond might be r8–r15, so we need both REX.R and REX.B
				rex |= 0x04 | 0x01
			}
			// 85 /r = TEST r/m64, r64
			modrm := byte(0xC0 | (cond.RegBits << 3) | cond.RegBits)
			code = append(code, rex, 0x85, modrm)
		}

		// 4) CMOVcc dst, RCX
		//    CMOVE (0x44) if isZero==true, otherwise CMOVNE (0x45)
		{
			cmovOp := byte(0x44)
			if !isZero {
				cmovOp = 0x45
			}
			rex := byte(0x48)
			// REX.R = 1 if dest (the reg field) is r8–r15
			if dst.REXBit == 1 {
				rex |= 0x04
			}
			// REX.B = 1 if scratch (the rm field) is r8–r15 (RCX is low, so usually 0)
			if scratch.REXBit == 1 {
				rex |= 0x01
			}
			// ModR/M: reg = dst, rm = scratch
			modrm := byte(0xC0 | (dst.RegBits << 3) | scratch.RegBits)
			code = append(code, rex, 0x0F, cmovOp, modrm)
		}

		// 5) POP RCX
		code = append(code, 0x59)

		return code
	}
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
// FURTHER OPTIMIZATIONS:
// * if imm == 1 -> use SHL/SAR/SHR/ROR directly
func generateImmShiftOp64(opcode, subcode byte) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		dstIdx, srcIdx, imm := extractTwoRegsOneImm(inst.Args)
		dst := regInfoList[dstIdx]
		src := regInfoList[srcIdx]

		// 1) Zero‐shift → either MOV or nothing
		if imm == 0 {
			if srcIdx != dstIdx {
				// MOV only
				rex := byte(0x48 | (byte(src.REXBit) << 2) | byte(dst.REXBit))
				return []byte{rex, 0x89, byte(0xC0 | (src.RegBits << 3) | dst.RegBits)}
			}
			return nil
		}

		// 2) In‐place shift (dst==src)
		if srcIdx == dstIdx {
			// try to drop REX if we can do a 64‐bit shift on a low reg without extension
			useRex := byte(0x48) // we need REX.W=1 for 64-bit
			if dst.REXBit == 1 {
				// sometimes you don’t need REX.B if dst<=7, but REX.W forces the prefix
				useRex |= 0x01
			}
			m := byte(0xC0 | (subcode << 3) | dst.RegBits)
			if imm == 1 {
				return []byte{useRex, 0xD1, m}
			}
			return []byte{useRex, opcode, m, byte(imm)}
		}

		// 3) src!=dst, imm>0 → MOV+shift
		// MOV
		rexMov := byte(0x48 | (byte(src.REXBit) << 2) | (byte(dst.REXBit) << 0))
		mMov := byte(0xC0 | (src.RegBits << 3) | dst.RegBits)

		// SHIFT
		// reuse logic from in‑place case to pick D1 vs C1
		if imm == 1 {
			rexSh := byte(0x48 | byte(dst.REXBit))
			mSh := byte(0xC0 | (subcode << 3) | dst.RegBits)
			return []byte{
				rexMov, 0x89, mMov,
				rexSh, 0xD1, mSh,
			}
		}
		rexSh := byte(0x48 | byte(dst.REXBit))
		mSh := byte(0xC0 | (subcode << 3) | dst.RegBits)
		return []byte{
			rexMov, 0x89, mMov,
			rexSh, opcode, mSh, byte(imm),
		}
	}
}

// ALT variant: dst = imm64 op (src64 & 63)
// subcode = 4 (SHL), 5 (SHR), 7 (SAR)
func generateImmShiftOp64Alt(subcode byte) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		dstIdx, srcIdx, imm := extractTwoRegsOneImm(inst.Args)
		dst := regInfoList[dstIdx]
		src := regInfoList[srcIdx]
		var code []byte
		sameReg := dstIdx == srcIdx

		if sameReg {
			if BaseReg.REXBit == 1 {
				code = append(code, 0x41)
			}
			code = append(code, 0x50|BaseReg.RegBits)

			dstIdx = BaseRegIndex
			dst = regInfoList[dstIdx]

			rexMov := byte(0x48)
			if src.REXBit == 1 {
				rexMov |= 0x04
			}
			if dst.REXBit == 1 {
				rexMov |= 0x01
			}
			modrm := byte(0xC0 | (src.RegBits << 3) | dst.RegBits)
			code = append(code, rexMov, 0x89, modrm)
		}

		needSaveRCX := dst.Name != "RCX"
		needXchg := src.Name != "RCX"

		if needSaveRCX {
			code = append(code, 0x51)
		}

		rexMov := byte(0x48)
		if dst.REXBit == 1 {
			rexMov |= 0x01
		}
		code = append(code, rexMov, 0xB8|dst.RegBits)
		for i := 0; i < 8; i++ {
			code = append(code, byte(imm>>(8*i)))
		}

		if needXchg {
			rexX := byte(0x48)
			if src.REXBit == 1 {
				rexX |= 0x04
			}
			modrmX := byte(0xC0 | (src.RegBits << 3) | 0x01)
			code = append(code, rexX, 0x87, modrmX)
		}

		rexSh := byte(0x48)
		if dst.REXBit == 1 {
			rexSh |= 0x01
		}
		modrmSh := byte(0xC0 | (subcode << 3) | dst.RegBits)
		code = append(code, rexSh, 0xD3, modrmSh)

		if needXchg {
			rexX := byte(0x48)
			if src.REXBit == 1 {
				rexX |= 0x04
			}
			modrmX := byte(0xC0 | (src.RegBits << 3) | 0x01)
			code = append(code, rexX, 0x87, modrmX)
		}

		if needSaveRCX {
			code = append(code, 0x59)
		}

		if sameReg {
			// move result back to src
			rexMov2 := byte(0x48)
			if BaseReg.REXBit == 1 {
				rexMov2 |= 0x04
			}
			if src.REXBit == 1 {
				rexMov2 |= 0x01
			}
			modrm2 := byte(0xC0 | (BaseReg.RegBits << 3) | src.RegBits)
			code = append(code, rexMov2, 0x89, modrm2)

			if BaseReg.REXBit == 1 {
				code = append(code, 0x41)
			}
			code = append(code, 0x58|BaseReg.RegBits)
		}

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

// generateXEncode performs sign-extension (n→64) on the dst register, using rcx and rdx internally.
func generateXEncode(dst X86Reg, n uint32) []byte {
	var code []byte

	// 0) Save temporary registers
	code = append(code,
		0x52, // push rdx
		0x51, // push rcx
	)

	// 1) If n is invalid → directly zero out
	if n == 0 || n > 8 {
		rex := byte(0x48) // REX.W=1
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | dst.RegBits)
		code = append(code, rex, 0xC7, modrm) // MOV r64_dst, imm32
		code = append(code, encodeU32(0)...)
		return append(code,
			0x59, // pop rcx
			0x5A, // pop rdx
		)
	}

	// 2) If n==8 → already 64-bit, no operation needed
	if n == 8 {
		return append(code,
			0x59, // pop rcx
			0x5A, // pop rdx
		)
	}

	// Below: 1 <= n <= 7

	// 3) MOV r64_rcx, r64_dst   ; copy x → rcx
	rex1 := byte(0x48)
	if dst.REXBit == 1 {
		rex1 |= 0x04 // REX.R
	}
	code = append(code, rex1, 0x89, byte(0xC0|dst.RegBits<<3|1)) // ModR/M: reg=dst, rm=rcx

	// 4) SHR rcx, imm8          ; q = x >> (8*n-1)
	shift := byte(8*n - 1)
	code = append(code, 0x48, 0xC1, 0xE9, shift) // 0xC1 /5=SHR, modrm E9=(reg=5, rm=1)

	// 5) MOVABS rdx, factor     ; factor = ^((1<<(8*n))-1)
	mask := uint64((1 << (8 * n)) - 1)
	factor := ^mask
	rex2 := byte(0x48) | 0x02               // REX.W + REX.X for rdx
	code = append(code, rex2, byte(0xB8|2)) // MOVABS rdx, imm64
	code = append(code, encodeU64(factor)...)

	// 6) IMUL rdx, rcx          ; rdx = q * factor
	code = append(code, 0x48, 0x0F, 0xAF, byte(0xC0|2<<3|1))

	// 7) ADD r64_dst, r64_rdx   ; x + q*factor
	rex3 := byte(0x48)
	if dst.REXBit == 1 {
		rex3 |= 0x01
	}
	code = append(code, rex3, 0x01, byte(0xC0|2<<3|dst.RegBits))

	// 8) restore rcx and rdx
	code = append(code,
		0x59, // pop rcx
		0x5A, // pop rdx
	)

	return code
}

// Implements dst64 = (imm32 - src32) mod 2^64,
// which for a 32-bit imm and 32-bit src means 64-bit wraparound.
// Implements dst64 = (imm32 - src32) mod 2^64, then truncate to 32 bits
func generateNegAddImm32(inst Instruction) []byte {
	dstIdx, srcIdx, imm := extractTwoRegsOneImm(inst.Args)
	dst := regInfoList[dstIdx]
	src := regInfoList[srcIdx]

	var code []byte

	// 1) MOV r64_dst, r64_src
	rex1 := byte(0x48) // REX.W=1
	if src.REXBit == 1 {
		rex1 |= 0x04
	} // REX.R for src
	if dst.REXBit == 1 {
		rex1 |= 0x01
	} // REX.B for dst
	modrm1 := byte(0xC0 | (src.RegBits << 3) | dst.RegBits)
	code = append(code, rex1, 0x89, modrm1)

	// 2) NEG r64_dst
	rex2 := byte(0x48)
	if dst.REXBit == 1 {
		rex2 |= 0x01
	}
	modrm2 := byte(0xC0 | (0x03 << 3) | dst.RegBits) // /3 = NEG
	code = append(code, rex2, 0xF7, modrm2)

	// 3) ADD r64_dst, imm32
	rex3 := byte(0x48)
	if dst.REXBit == 1 {
		rex3 |= 0x01
	}
	modrm3 := byte(0xC0 | (0x00 << 3) | dst.RegBits) // /0 = ADD
	code = append(code, rex3, 0x81, modrm3)
	code = append(code, encodeU32(uint32(imm))...)

	// 4) Truncate to 32-bit: MOV r/m32(dst), r32(dst)
	//    any write to the 32-bit sub-register zeroes the upper 32 bits
	rex4 := byte(0x40) // REX (W=0 means 32-bit operand size)
	// extend reg and rm if dst ∈ r8–r15
	if dst.REXBit == 1 {
		rex4 |= 0x05 // REX.R=1 and REX.B=1 (same reg in both fields)
	}
	modrm4 := byte(0xC0 | (dst.RegBits << 3) | dst.RegBits)
	code = append(code, rex4, 0x89, modrm4)
	code = append(code, generateXEncode(dst, 4)...)

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
func generateImmShiftOp32SHLO(subcode byte) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		dstIdx, srcIdx, imm := extractTwoRegsOneImm(inst.Args)
		dst := regInfoList[dstIdx]
		src := regInfoList[srcIdx]

		var code []byte

		// 1) MOV r32_dst, r32_src → zeros high 32 bits
		rex := byte(0x40)
		if src.REXBit == 1 {
			rex |= 0x04
		} // REX.R for source
		if dst.REXBit == 1 {
			rex |= 0x01
		} // REX.B for destination
		modrm := byte(0xC0 | (src.RegBits << 3) | dst.RegBits)
		code = append(code, rex, 0x89, modrm)

		// 2) SHL r/m32(dst), imm8  (C1 /4 ib)
		rex = byte(0x40)
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		modrm = byte(0xC0 | (subcode << 3) | dst.RegBits)
		shiftAmt := byte(imm & 0x1F)
		code = append(code, rex, 0xC1, modrm, shiftAmt)

		// 3) MOVSXD r64_dst, r/m32(dst)  → sign-extend to 64-bit
		rex = byte(0x48)
		if dst.REXBit == 1 {
			rex |= 0x05
		} // REX.W + REX.R + REX.B
		modrm = byte(0xC0 | (dst.RegBits << 3) | dst.RegBits)
		code = append(code, rex, 0x63, modrm)

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
func generateImmSetCondOp32New(setcc byte) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		dstIdx, srcIdx, imm := extractTwoRegsOneImm(inst.Args)
		dstInfo := regInfoList[dstIdx]
		srcInfo := regInfoList[srcIdx]
		scratch := regInfoList[1] // RCX

		var code []byte

		if dstIdx == srcIdx {
			// Alias: dst and src are the same register → use RCX scratch

			// 1) PUSH RCX
			code = append(code, 0x51)

			// 2) MOV ECX, r32_dst  (save original valueB into ECX)
			//    opcode: 0x8B /r, REX.W=0
			{
				rex := byte(0x40)
				if dstInfo.REXBit == 1 {
					rex |= 0x01 // REX.B for rm field if dst is r8–r15
				}
				// reg = 1 (ECX), rm = dst.RegBits
				modrm := byte(0xC0 | (1 << 3) | dstInfo.RegBits)
				code = append(code, rex, 0x8B, modrm)
			}

			// 3) XOR r32_dst, r32_dst  → clear dst
			{
				rex := byte(0x40)
				if dstInfo.REXBit == 1 {
					// we need REX.R and REX.B so that both fields can address dst
					rex |= 0x04 | 0x01
				}
				modrm := byte(0xC0 | (dstInfo.RegBits << 3) | dstInfo.RegBits)
				code = append(code, rex, 0x31, modrm)
			}

			// 4) CMP ECX, imm32
			{
				rex := byte(0x40)
				// reg = 7 for CMP, rm = scratch.RegBits (ECX)
				modrm := byte(0xC0 | (7 << 3) | scratch.RegBits)
				code = append(code, rex, 0x81, modrm)
				code = append(code, encodeU32(uint32(imm))...)
			}

			// 5) SETcc r/m8_dst
			{
				rex := byte(0x40)
				if dstInfo.REXBit == 1 {
					rex |= 0x01
				}
				// For SETcc the encoding is: REX, 0F, setcc, ModRM
				modrm := byte(0xC0 | dstInfo.RegBits)
				code = append(code, rex, 0x0F, setcc, modrm)
			}

			// 6) POP RCX
			code = append(code, 0x59)

			return code
		}

		// Non-alias case: dst != src

		// 1) XOR r32_dst, r32_dst  → clear dst
		{
			rex := byte(0x40)
			if dstInfo.REXBit == 1 {
				rex |= 0x04 | 0x01
			}
			modrm := byte(0xC0 | (dstInfo.RegBits << 3) | dstInfo.RegBits)
			code = append(code, rex, 0x31, modrm)
		}

		// 2) CMP r32_src, imm32
		{
			rex := byte(0x40)
			if srcInfo.REXBit == 1 {
				rex |= 0x01
			}
			// reg = 7 (CMP), rm = srcInfo.RegBits
			modrm := byte(0xC0 | (7 << 3) | srcInfo.RegBits)
			code = append(code, rex, 0x81, modrm)
			code = append(code, encodeU32(uint32(imm))...)
		}

		// 3) SETcc r/m8_dst
		{
			rex := byte(0x40)
			if dstInfo.REXBit == 1 {
				rex |= 0x01
			}
			modrm := byte(0xC0 | dstInfo.RegBits)
			code = append(code, rex, 0x0F, setcc, modrm)
		}

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
// FURTHER OPTIMIZATIONS:
// 1. if imm is 8bit (-128 to 127) you can do better
func generateBinaryImm32(inst Instruction) []byte {
	dstReg, srcReg, imm := extractTwoRegsOneImm(inst.Args)
	dst := regInfoList[dstReg]
	src := regInfoList[srcReg]

	var code []byte
	// SPECIAL CASE
	if imm == 0 {
		//  just sign‑extend src → dst MOVSXD r64_dst, r32_src
		rex := byte(0x48)
		if dst.REXBit == 1 {
			rex |= 0x05
		}
		modrm := byte(0xC0 | (dst.RegBits << 3) | src.RegBits)
		return []byte{rex, 0x63, modrm}
	}

	// 1) MOV r32_dst, r32_src BUT  Skip the MOV when src and dst are the same!!
	if srcReg != dstReg {
		// 1) MOV r32_dst, r32_src
		rexMov := byte(0x40) // no REX.W
		if src.REXBit == 1 {
			rexMov |= 0x04
		} // REX.R
		if dst.REXBit == 1 {
			rexMov |= 0x01
		} // REX.B
		modrmMov := byte(0xC0 | (src.RegBits << 3) | dst.RegBits)
		code = append(code, rexMov, 0x89, modrmMov)
	}

	// 2) ADD r32_dst, imm32
	// TODO: if imm is 8-bit, we can use a smaller instruction
	// use8 := imm >= -128 && imm <= 127
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

func generateImmBinaryOp64(subcode byte) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		dstReg, srcReg, imm64, uint64imm := extractTwoRegsOneImm64(inst.Args)
		dst := regInfoList[dstReg]
		src := regInfoList[srcReg]

		var code []byte

		// 1) If src!=dst, copy src into dst
		if srcReg != dstReg {
			rex := byte(0x48)
			if src.REXBit == 1 {
				rex |= 0x04
			} // REX.R
			if dst.REXBit == 1 {
				rex |= 0x01
			} // REX.B
			modrm := byte(0xC0 | (src.RegBits << 3) | dst.RegBits)
			code = append(code, rex, 0x89, modrm) // MOV r64_dst, r64_src
		}

		// 2) Can we use a 32-bit immediate?
		if imm64 >= -0x80000000 && imm64 <= 0x7FFFFFFF {
			// 81 /subcode id  → op r/m64, imm32
			rex := byte(0x48)
			if dst.REXBit == 1 {
				rex |= 0x01
			}
			modrm := byte(0xC0 | (subcode << 3) | dst.RegBits)
			code = append(code, rex, 0x81, modrm)
			imm32 := uint32(int32(imm64))
			b := make([]byte, 4)
			binary.LittleEndian.PutUint32(b, imm32)
			code = append(code, b...)
			return code
		}

		// 3) Otherwise: full 64-bit immediate via scratch
		// pick RCX (reg1), or RDX (reg2) if needed, etc.
		scratchReg := byte(1)
		if scratchReg == byte(dstReg) || scratchReg == byte(srcReg) {
			scratchReg = 2
		}
		scratch := regInfoList[scratchReg]

		// -- push scratch --
		pushOp := byte(0x50 + scratch.RegBits)
		if scratch.REXBit == 1 {
			code = append(code, 0x41, pushOp)
		} else {
			code = append(code, pushOp)
		}

		// -- MOVABS scratch, imm64 --
		rexAbs := byte(0x48)
		if scratch.REXBit == 1 {
			rexAbs |= 0x01
		}
		movabs := byte(0xB8 + scratch.RegBits)
		code = append(code, rexAbs, movabs)
		immBytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(immBytes, uint64imm)
		code = append(code, immBytes...)

		// -- perform the operation dst = dst <op> scratch --
		// for ADD: opcode=0x01; AND:0x21; XOR:0x31
		var regRegOp byte
		switch subcode {
		case 0: // ADD
			regRegOp = 0x01
		case 4: // AND
			regRegOp = 0x21
		case 6: // XOR
			regRegOp = 0x31
		default:
			// UNREACHABLE for our use-cases
			regRegOp = 0x01
		}
		rexOp := byte(0x48)
		if scratch.REXBit == 1 {
			rexOp |= 0x04
		} // REX.R = scratch
		if dst.REXBit == 1 {
			rexOp |= 0x01
		} // REX.B = dst
		modrm := byte(0xC0 | (scratch.RegBits << 3) | dst.RegBits)
		code = append(code, rexOp, regRegOp, modrm)

		// -- pop scratch --
		popOp := byte(0x58 + scratch.RegBits)
		if scratch.REXBit == 1 {
			code = append(code, 0x41, popOp)
		} else {
			code = append(code, popOp)
		}

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
			rex |= 0x02 // X (SIB.index)
		}
		if BaseReg.REXBit != 0 {
			rex |= 0x01 // B (SIB.base)
		}

		// ModRM: mod=10 (disp32), reg=regA, rm=100 (SIB)
		modRM := byte(0x02<<6) | (regA.RegBits << 3) | 0x04
		// SIB: scale=0 (1), index=regB, base=BaseReg
		sib := byte(0<<6) | (regB.RegBits << 3) | BaseReg.RegBits

		// disp32 as little-endian
		disp := make([]byte, 4)
		binary.LittleEndian.PutUint32(disp, uint32(int32(vx)))

		switch opcode {
		case LOAD_IND_U8:
			// zero-extend byte into the full 64-bit reg: MOVZX r64, r/m8 (0F B6 /r)
			code := []byte{}
			if prefix != 0 {
				code = append(code, prefix)
			}
			code = append(code,
				rex,        // REX.W=1 if 64-bit, plus R/X/B bits
				0x0F, 0xB6, // MOVZX r64, r/m8
				modRM, sib, // ModRM + SIB
			)
			code = append(code, disp...)
			return code
		case LOAD_IND_U16:
			var rex byte = 0x40
			var needRex bool = false
			if regA.REXBit != 0 {
				rex |= 0x04 // R
				needRex = true
			}
			if regB.REXBit != 0 {
				rex |= 0x02 // X (SIB.index)
				needRex = true
			}
			if BaseReg.REXBit != 0 {
				rex |= 0x01 // B (SIB.base)
				needRex = true
			}

			code := []byte{}
			if prefix != 0 {
				code = append(code, prefix)
			}
			code = append(code, 0x66)
			if needRex {
				code = append(code, rex)
			}

			var modRM, sib byte
			if BaseReg.RegBits == 4 { // r12 as base
				// Swap: use BaseReg as index, regB as base
				modRM = byte(0x02<<6) | (regA.RegBits << 3) | 0x04
				sib = byte(0x00<<6) | (BaseReg.RegBits << 3) | regB.RegBits
			} else {
				// Normal case: BaseReg as base, regB as index
				modRM = byte(0x02<<6) | (regA.RegBits << 3) | 0x04
				sib = byte(0x00<<6) | (regB.RegBits << 3) | BaseReg.RegBits
			}
			code = append(code,
				0x0F, 0xB7, // MOVZX r64, r/m16
				modRM, sib, // ModRM + SIB
			)
			code = append(code, disp...)
			rexForAnd := byte(0x48)
			if regA.REXBit != 0 {
				rexForAnd |= 0x01
			}

			// 81 /4 ib : AND r/m64, imm32
			// ModRM: mod=11, reg=4 (AND opcode ext), rm=regA.RegBits
			modrm := byte(0xC0 | (0x4 << 3) | regA.RegBits)

			andInst := []byte{
				rexForAnd,
				0x81, // AND r/m64, imm32
				modrm,
				0xFF, 0xFF, 0x00, 0x00, // imm32 = 0x0000FFFF
			}
			code = append(code, andInst...)
			return code
		case LOAD_IND_U32:
			// zero-extend via 32-bit MOV → clears upper 32 bits in 64-bit reg
			rex32 := byte(0x40)
			if regA.REXBit != 0 {
				rex32 |= 0x04
			}
			if regB.REXBit != 0 {
				rex32 |= 0x02
			}
			if BaseReg.REXBit != 0 {
				rex32 |= 0x01
			}

			code32 := []byte{}
			if prefix != 0 {
				code32 = append(code32, prefix)
			}
			code32 = append(code32,
				rex32,
				0x8B, // MOV r32, r/m32
				modRM, sib,
			)
			code32 = append(code32, disp...)
			return code32

		case LOAD_IND_U64:
			// full 64-bit MOV r64, r/m64
			code64 := []byte{}
			if prefix != 0 {
				code64 = append(code64, prefix)
			}
			code64 = append(code64,
				rex,
				0x8B,       // MOV r64, r/m64
				modRM, sib, // ModRM + SIB
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
