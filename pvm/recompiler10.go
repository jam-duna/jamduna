package pvm

func generateMax() func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		aIdx, bIdx, dstIdx := extractThreeRegs(inst.Args)
		a := regInfoList[aIdx]
		b := regInfoList[bIdx]
		dst := regInfoList[dstIdx]
		// RAX is regInfoList[0]
		rax := regInfoList[0]

		// If dst==a (and a!=b) then we'll clobber `a` when we MOV b→dst,
		// so we need to stash it in RAX first.
		useTmp := dstIdx == aIdx && aIdx != bIdx

		var code []byte

		// 1) CMP a, b  — signed compare original a vs b
		{
			rex := byte(0x48)
			if b.REXBit == 1 {
				rex |= 0x04
			} // REX.R = b
			if a.REXBit == 1 {
				rex |= 0x01
			} // REX.B = a
			modrm := byte(0xC0 | (b.RegBits << 3) | a.RegBits)
			code = append(code, rex, 0x39, modrm) // 39 /r = CMP r/m64, r64
		}

		// 2) If needed, save original a → RAX
		if useTmp {
			code = append(code, 0x50) // PUSH RAX
			// MOV RAX, a
			rex := byte(0x48)
			if a.REXBit == 1 {
				rex |= 0x01
			} // REX.B = a
			// REX.R remains 0 because dest=RAX has REXBit=0
			modrm := byte(0xC0 | (rax.RegBits << 3) | a.RegBits)
			code = append(code, rex, 0x8B, modrm) // 8B /r = MOV r64, r/m64
		}

		// 3) MOV dst, b  — skip if dst == b
		if dstIdx != bIdx {
			rex := byte(0x48)
			if b.REXBit == 1 {
				rex |= 0x04
			} // REX.R = b
			if dst.REXBit == 1 {
				rex |= 0x01
			} // REX.B = dst
			modrm := byte(0xC0 | (b.RegBits << 3) | dst.RegBits)
			code = append(code, rex, 0x89, modrm) // 89 /r = MOV r/m64, r64
		}

		// 4) CMOVGE dst, src
		{
			src := a
			if useTmp {
				src = rax
			}
			rex := byte(0x48)
			if dst.REXBit == 1 {
				rex |= 0x04
			} // REX.R = dst
			if src.REXBit == 1 {
				rex |= 0x01
			} // REX.B = src
			modrm := byte(0xC0 | (dst.RegBits << 3) | src.RegBits)
			code = append(code, rex, 0x0F, 0x4D, modrm) // 0F 4D /r = CMOVGE r64, r/m64
		}

		// 5) Restore RAX if we pushed it
		if useTmp {
			code = append(code, 0x58) // POP RAX
		}

		return code
	}
}

func generateMax2() func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		aIdx, bIdx, dstIdx := extractThreeRegs(inst.Args)
		a := regInfoList[aIdx]
		b := regInfoList[bIdx]
		dst := regInfoList[dstIdx]
		// RAX is regInfoList[0]
		rax := regInfoList[0]

		var code []byte

		// If dst==a (and a!=b) we'll overwrite 'a' when MOV b→dst, so save 'a' in RAX.
		useTmp := dstIdx == aIdx && aIdx != bIdx

		// 0) if needed, PUSH RAX and MOV RAX←a
		if useTmp {
			// push rax
			code = append(code, emitPushRax()...)
			// mov rax, a
			code = append(code, emitMovRegToReg64(rax, a)...)
		}

		// 1) MOV dst, b   (skip when dst==b)
		if dstIdx != bIdx {
			code = append(code, emitMovRegToReg64(dst, b)...)
		}

		// 2) CMP a, b   ; signed compare A vs B
		code = append(code, emitCmpReg64(b, a)...)

		// 3) CMOVGE dst, src   (if A>=B signed, move A—or saved-A in RAX—into dst)
		{
			src := a
			if useTmp {
				src = rax
			}
			code = append(code, emitCmovcc(0x4D, dst, src)...)
		}

		// 4) if we pushed RAX, restore it
		if useTmp {
			code = append(code, emitPopRax()...)
		}

		return code
	}
}

func generateMin() func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		aIdx, bIdx, dstIdx := extractThreeRegs(inst.Args)
		a := regInfoList[aIdx]
		b := regInfoList[bIdx]
		dst := regInfoList[dstIdx]
		// RAX is regInfoList[0]
		rax := regInfoList[0]

		// If dst==a (and a!=b) then MOV dst←b would clobber a,
		// so we need to stash a in RAX.
		useTmp := dstIdx == aIdx && aIdx != bIdx

		var code []byte

		// 1) CMP a, b  — signed compare original a vs b
		code = append(code, emitCmpReg64(b, a)...)

		// 2) If needed, save original a → RAX
		if useTmp {
			code = append(code, emitPushRax()...) // PUSH RAX
			// MOV RAX, a
			code = append(code, emitMovRegToRegWithManualConstruction(rax, a)...)
		}

		// 3) MOV dst, b  — skip if dst == b
		if dstIdx != bIdx {
			code = append(code, emitMovRegToReg64(dst, b)...)
		}

		// 4) CMOVLE dst, src  — if A ≤ B signed, move A (or saved-A in RAX) into dst
		{
			src := a
			if useTmp {
				src = rax
			}
			code = append(code, emitCmovcc(0x4E, dst, src)...)
		}

		// 5) Restore RAX if we pushed it
		if useTmp {
			code = append(code, emitPopRax()...) // POP RAX
		}

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
			code = append(code, emitPushRcx()...)

			// 2) MOV RCX, B
			code = append(code, emitMovRegToRegWithManualConstruction(tmp, b)...)

			// 3) CMP A, B
			code = append(code, emitCmpReg64(b, a)...)

			// 4) MOV dst, A
			if dstIdx != aIdx {
				code = append(code, emitMovRegToReg64(dst, a)...)
			}

			// 5) CMOV{B|A} dst, RCX
			code = append(code, emitCmovcc(cmov, dst, tmp)...)

			// 6) POP RCX
			code = append(code, emitPopRcx()...)

		} else {
			// no alias: dst != B
			//   MOV dst, A
			//   CMP dst, B
			//   CMOV{B|A} dst, B

			// 1) MOV dst, A
			code = append(code, emitMovRegToReg64(dst, a)...)

			// 2) CMP dst, B
			code = append(code, emitCmpReg64(b, dst)...)

			// 3) CMOV{B|A} dst, B
			code = append(code, emitCmovcc(cmov, dst, b)...)
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
		code = append(code, emitPushRcx()...)

		// 2) MOVABS RCX, imm64
		code = append(code, emitMovImmToReg64(scratch, uint64(imm))...)

		// 3) TEST cond, cond   → sets ZF=1 iff cond==0
		code = append(code, emitTestReg64(cond, cond)...)

		// 4) CMOVcc dst, RCX
		//    CMOVE (0x44) if isZero==true, otherwise CMOVNE (0x45)
		cmovOp := byte(0x44)
		if !isZero {
			cmovOp = 0x45
		}
		code = append(code, emitCmovcc(cmovOp, dst, scratch)...)

		// 5) POP RCX
		code = append(code, emitPopRcx()...)

		return code
	}
}

//	result = x_encode(uint64(bits.RotateLeft32(uint32(valueB), -int(imm&31))), 4)
//
// Implements: result = sign‑extend32( bits.RotateRight32(uint32(src), imm&31) )
func generateRotateRight32Imm(inst Instruction) []byte {
	dstIdx, srcIdx, imm := extractTwoRegsOneImm(inst.Args)
	dst := regInfoList[dstIdx]
	src := regInfoList[srcIdx]

	var code []byte

	// ─── Conflict (src==dst) ───
	if srcIdx == dstIdx {
		// 1) MOV r32_dst, r32_dst   ; zero‑extend low 32→64
		code = append(code, emitMovRegToReg32(dst, dst)...)

		// 2) ROR r/m32(dst), imm8 - use shift helper with subcode 1 for ROR
		code = append(code, emitShiftRegImm(dst, 0xC1, 1, byte(imm&31))...)

		// 3) MOVSXD r64_dst, r32_dst  ; sign‑extend low 32→64
		code = append(code, emitMovsxdReg64(dst, dst)...)

		return code
	}

	// ─── Non‑conflict (src!=dst) ───
	// 1) MOV r32_dst, r32_src
	code = append(code, emitMovRegToReg32(dst, src)...)
	// 2) ROR r/m32(dst), imm8 - use shift helper with subcode 1 for ROR
	code = append(code, emitShiftRegImm(dst, 0xC1, 1, byte(imm&31))...)
	// 3) MOVSXD r64_dst, r32_dst
	code = append(code, emitMovsxdReg64(dst, dst)...)

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
				return emitMovRegToReg64(dst, src)
			}
			return nil
		}

		// 2) In‐place shift (dst==src)
		if srcIdx == dstIdx {
			if imm == 1 {
				return emitShiftRegImm1(dst, subcode)
			}
			return emitShiftRegImm(dst, opcode, subcode, byte(imm))
		}

		// 3) src!=dst, imm>0 → MOV+shift
		var code []byte
		code = append(code, emitMovRegToReg64(dst, src)...)
		if imm == 1 {
			code = append(code, emitShiftRegImm1(dst, subcode)...)
		} else {
			code = append(code, emitShiftRegImm(dst, opcode, subcode, byte(imm))...)
		}
		return code
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
	dstIdx, srcIdx, imm64 := extractTwoRegsOneImm(inst.Args)
	dst := regInfoList[dstIdx]

	var code []byte

	// ─── Special‐case when dst==src ───
	if dstIdx == srcIdx {
		// 1) NEG r64_dst   (opcode F7 /3 with ModRM = 11|3|dst)
		code = append(code, emitNegReg64(dst)...)

		// 2) ADD r64_dst, imm8  if possible  (83 /0 ib)
		if imm64 <= 127 {
			code = append(code, emitAddRegImm8(dst, int8(imm64))...)
			return code
		}

		// 3) ADD r64_dst, imm32  if it fits 32 bits  (81 /0 id)
		if imm64 <= 0x7FFFFFFF {
			code = append(code, emitAddRegImm32(dst, int32(imm64))...)
			return code
		}

		// 4) Otherwise fall back to MOVABS+SUB
		// (rare: imm outside signed‐32 range)
	}

	// ─── Fallback for dst!=src or large imm ───
	// MOVABS r64_dst, imm64
	code = append(code, emitMovImmToReg64(dst, imm64)...)

	// SUB r64_dst, r64_src
	src := regInfoList[srcIdx]
	code = append(code, emitSubReg64(dst, src)...)

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
		code = append(code, emitMovImmToReg64(dst, imm)...)
		// 2) XCHG RCX, src64
		code = append(code, emitXchgRcxReg64(src)...)
		// 3) Shift full 64-bit: D3 /subcode dst, CL
		code = append(code, emitShiftRegCl(dst, subcode)...)
		// 4) restore RCX
		code = append(code, emitXchgRcxReg64(src)...)

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
		code = append(code, emitMovRegToReg32(dst, src)...)

		// 2) SHL r/m32(dst), imm8  (C1 /4 ib) - 32-bit shift
		shiftAmt := byte(imm & 0x1F)
		code = append(code, emitShiftRegImm32(dst, 0xC1, subcode, shiftAmt)...)

		// 3) MOVSXD r64_dst, r/m32(dst)  → sign-extend to 64-bit
		code = append(code, emitMovsxdReg64(dst, dst)...)

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
			code = append(code, emitMovImmToReg32(dst, uint32(imm))...)

			// 2) XCHG ECX, r32_src  ; load count into CL
			code = append(code, emitXchgReg32Reg32(regInfoList[1], src)...) // RCX = regInfoList[1]

			// 3) D3 /subcode r/m32(dst), CL - use shift helper
			code = append(code, emitShiftReg32ByCl(dst, subcode)...)

			// 4) XCHG ECX, r32_src  ; restore ECX and src
			code = append(code, emitXchgReg32Reg32(regInfoList[1], src)...)

			// 5) if SAR (subcode==7), sign-extend 32->64: MOVSXD r64_dst, r/m32(dst)
			if subcode == 7 {
				code = append(code, emitMovsxdReg64(dst, dst)...)
			}
		} else {
			// NORMAL: register value on left, immediate count
			// 1) MOV r32_dst, r32_src (89 /r)
			code = append(code, emitMovRegToReg32(dst, src)...)

			// 2) C1 /subcode r/m32(dst), imm8 - 32-bit shift
			code = append(code, emitShiftRegImm32(dst, opcode, subcode, byte(imm&0xFF))...)

			// 3) if SAR (subcode==7), sign-extend 32->64: MOVSXD r64_dst, r/m32(dst)
			if subcode == 7 {
				code = append(code, emitMovsxdReg64(dst, dst)...)
			}
		}

		return code
	}
}

func generateImmSetCondOp32New(setcc byte) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		dstIdx, srcIdx, imm := extractTwoRegsOneImm(inst.Args)
		dstInfo := regInfoList[dstIdx]
		srcInfo := regInfoList[srcIdx]
		scratch := regInfoList[1] // RCX

		var code []byte

		// --- alias case: dst and src are the same register ---
		if dstIdx == srcIdx {
			// 1) save RCX
			code = append(code, emitPushRcx()...) // PUSH RCX

			// 2) sign‑extend the 32‑bit dst into RCX
			// opcode 0x8B /r = MOV r64, r/m64 (this is MOV RCX, dst)
			code = append(code, emitMovRegToRegWithManualConstruction(scratch, dstInfo)...)

			// 3) clear the low 32 bits of dst
			code = append(code, emitXorReg32(dstInfo, dstInfo)...)

			// 4) signed compare RCX vs imm32
			code = append(code, emitCmpRegImm32Force81(scratch, int32(imm))...)

			// 5) SETcc into low byte of dst
			code = append(code, emitSetccReg8(setcc, dstInfo)...)

			// 6) restore RCX
			code = append(code, emitPopRcx()...) // POP RCX
			return code
		}

		// --- non‑alias case: dst != src ---

		// 1) clear dst
		code = append(code, emitXorReg32(dstInfo, dstInfo)...)

		// 2) signed compare src vs imm
		code = append(code, emitCmpRegImm32Force81(srcInfo, int32(imm))...)

		// 3) SETcc into low byte of dst
		code = append(code, emitSetccReg8(setcc, dstInfo)...)

		return code
	}
}

// Implements dst := src * imm (32-bit immediate)
// 8 * 18446744073709542685 => 3389986608 but should be 18446744072804570928
//
//	 generateImmMulOp32: dst=r9 7, src=r10 8, imm=18446744073709542685
//		fmt.Printf("generateImmMulOp32: dst=%s %d, src=%s %d, imm=%d\n", dst.Name, dstReg, src.Name, srcReg, imm)
//
// Implements dst := sign‑extended((src * imm) & 0xFFFFFFFF)
func generateImmMulOp32(inst Instruction) []byte {
	dstReg, srcReg, imm := extractTwoRegsOneImm(inst.Args)
	src := regInfoList[srcReg]
	dst := regInfoList[dstReg]

	// 1) copy src → dst (32‑bit)
	code := append([]byte{}, emitMovRegToReg32(dst, src)...)

	// 2) IMUL dst, src, imm32  (yields 32‑bit truncated product in dst)
	code = append(code, emitImulRegImm32(dst, src, uint32(imm))...)

	// 3) MOVSXD r64_dst, r32_dst  (sign‑extend low 32 bits back to 64 bits)
	code = append(code, emitMovsxdReg64(dst, dst)...)

	return code
}

// Implements dst := src * imm (64-bit immediate)
func generateImmMulOp64(inst Instruction) []byte {
	dstReg, srcReg, imm := extractTwoRegsOneImm(inst.Args)
	src := regInfoList[srcReg]
	dst := regInfoList[dstReg]

	// 1) MOV r64_dst, r64_src
	code := append([]byte{}, emitMovRegToReg64(dst, src)...)

	// 2) IMUL r64_dst, r64_src, imm32 (sign-extended)
	code = append(code, emitImulRegImm64(dst, src, uint32(imm))...)
	return code
}

// Implements: dst := sign_extend( i32(src) + imm32 )
// FURTHER OPTIMIZATIONS:
// 1. if imm is 8bit (-128 to 127) you can do better
func generateBinaryImm32(inst Instruction) []byte {
	dstReg, srcReg, imm := extractTwoRegsOneImm(inst.Args)
	dst := regInfoList[dstReg]
	src := regInfoList[srcReg]
	//fmt.Printf("generateBinaryImm32: dst=%s %d, src=%s %d, imm=%d\n", dst.Name, dstReg, src.Name, srcReg, imm)
	var code []byte
	// SPECIAL CASE: imm == 0 → dst = sign_extend(src32)
	if imm == 0 {
		return emitMovsxdReg64(dst, src)
	}

	// 1) MOV r32_dst, r32_src BUT  Skip the MOV when src and dst are the same!!
	if srcReg != dstReg {
		// 1) MOV r32_dst, r32_src
		code = append(code, emitMovRegToReg32(dst, src)...)
	}

	// 2) ADD r32_dst, imm32
	code = append(code, emitAddRegImm32WithManualConstruction(dst, imm)...)

	// 3) MOVSXD r64_dst, r/m32   ; sign-extend low 32→64
	code = append(code, emitMovsxdReg64(dst, dst)...)
	return code
}

func generateImmBinaryOp64(subcode byte) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		dstReg, srcReg, imm64, uint64imm := extractTwoRegsOneImm64(inst.Args)
		dst := regInfoList[dstReg]
		src := regInfoList[srcReg]
		var code []byte

		// helper: copy src→dst if needed
		copySrcToDst := func() {
			if srcReg != dstReg {
				code = append(code, emitMovRegToReg64(dst, src)...)
			}
		}

		// 1) Small imm8 path: 83 /subcode ib
		if imm64 >= -128 && imm64 <= 127 {
			copySrcToDst()
			code = append(code, emitAluRegImm8(dst, subcode, int8(imm64))...)
			return code
		}

		// 2) Imm32 path: 81 /subcode id
		if imm64 >= -0x80000000 && imm64 <= 0x7FFFFFFF {
			copySrcToDst()
			code = append(code, emitAluRegImm32(dst, subcode, int32(imm64))...)
			return code
		}

		// 3) Full imm64 via scratch + XCHG swap
		copySrcToDst()
		scratch := regInfoList[0]

		// swap dst<->scratch: XCHG r64,dst
		code = append(code, emitXchgReg64(scratch, dst)...)

		// MOVABS scratch, imm64 (now scratch contains the old dst value)
		code = append(code, emitMovAbsToReg(scratch, uint64imm)...)

		// ALU dst, scratch  (dst now has old scratch value, scratch has immediate)
		code = append(code, emitAluRegReg(dst, scratch, subcode)...)

		// swap back scratch↔dst: XCHG
		code = append(code, emitXchgReg64(scratch, dst)...)

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

		switch opcode {
		case LOAD_IND_I8:
			// MOVSX r64, byte ptr [BaseReg + regB*1 + disp32] (0F BE /r)
			code := emitLoadWithSIB(regA, BaseReg, regB, int32(vx), []byte{0x0F, 0xBE}, is64bit, prefix)
			code = append(code, castReg8ToU64(regAIndex)...)
			return code
		case LOAD_IND_I16:
			// 0x66 prefix + MOVSX r64, word ptr [BaseReg + regB*1 + disp32] (0F BF /r)
			code := emitLoadWithSIB(regA, BaseReg, regB, int32(vx), []byte{0x0F, 0xBF}, is64bit, 0x66)
			code = append(code, castReg16ToU64(regAIndex)...)
			return code
		case LOAD_IND_I32:
			// MOVSXD r64, dword ptr [BaseReg + regB*1 + disp32] (63 /r)
			return emitLoadWithSIB(regA, BaseReg, regB, int32(vx), []byte{0x63}, is64bit, prefix)
		default:
			panic("generateLoadIndSignExtend: invalid opcode")
		}
	}
}

func castReg8ToU64(regIdx int) []byte {
	r := regInfoList[regIdx]
	// MOVSX r64, r/m8 - sign extend register to itself
	return emitMovsx64From8(r, r)
}

func castReg16ToU64(regIdx int) []byte {
	r := regInfoList[regIdx]
	// MOVSX r64, r/m16 - sign extend register to itself
	return emitMovsx64From16(r, r)
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

		switch opcode {
		case LOAD_IND_U8:
			// zero-extend byte into the full 64-bit reg: MOVZX r64, r/m8 (0F B6 /r)
			return emitLoadWithSIB(regA, BaseReg, regB, int32(vx), []byte{0x0F, 0xB6}, is64bit, prefix)

		case LOAD_IND_U16:
			// MOVZX r64, r/m16 (0F B7 /r) with special handling for BaseReg==R12
			var base, index X86Reg
			if BaseReg.RegBits == 4 { // r12 as base - swap base and index
				base = regB
				index = BaseReg
			} else {
				base = BaseReg
				index = regB
			}

			code := emitLoadWithSIB(regA, base, index, int32(vx), []byte{0x0F, 0xB7}, false, 0x66)
			// AND with 0x0000FFFF to ensure proper zero-extension
			code = append(code, emitAluRegImm32(regA, 4, 0x0000FFFF)...)
			return code

		case LOAD_IND_U32:
			// zero-extend via 32-bit MOV → clears upper 32 bits in 64-bit reg
			return emitLoadWithSIB(regA, BaseReg, regB, int32(vx), []byte{0x8B}, false, prefix)

		case LOAD_IND_U64:
			// full 64-bit MOV r64, r/m64
			return emitLoadWithSIB(regA, BaseReg, regB, int32(vx), []byte{0x8B}, true, prefix)

		default:
			panic("generateLoadInd: invalid opcode")
		}
	}
}

func generateStoreIndirect(size int) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		srcIdx, dstIdx, disp64 := extractTwoRegsOneImm(inst.Args)

		dstInfo := regInfoList[dstIdx]
		srcInfo := regInfoList[srcIdx]

		return emitStoreWithSIB(srcInfo, BaseReg, dstInfo, int32(disp64), size)
	}
}
