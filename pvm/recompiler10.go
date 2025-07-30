package pvm

func generateMax() func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		aIdx, bIdx, dstIdx := extractThreeRegs(inst.Args)
		a := regInfoList[aIdx]
		b := regInfoList[bIdx]
		dst := regInfoList[dstIdx]

		// If dst==a (and a!=b) then we'll clobber `a` when we MOV b→dst,
		// so we need to stash it in RAX first.
		useTmp := dstIdx == aIdx && aIdx != bIdx

		var code []byte

		// 1) CMP a, b  — signed compare original a vs b
		code = append(code, emitCmpReg64Max(a, b)...)

		// 2) If needed, save original a → RAX
		if useTmp {
			code = append(code, emitPushReg(RAX)...)
			// MOV RAX, a
			code = append(code, emitMovRegToRegMax(RAX, a)...)
		}

		// 3) MOV dst, b  — skip if dst == b
		if dstIdx != bIdx {
			code = append(code, emitMovRegFromRegMax(dst, b)...)
		}

		// 4) CMOVGE dst, src
		{
			src := a
			if useTmp {
				src = RAX
			}
			code = append(code, emitCmovgeMax(dst, src)...)
		}

		// 5) Restore RAX if we pushed it
		if useTmp {
			code = append(code, emitPopReg(RAX)...)
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

		// If dst==a (and a!=b) then MOV dst←b would clobber a,
		// so we need to stash a in RAX.
		useTmp := dstIdx == aIdx && aIdx != bIdx

		var code []byte

		// 1) CMP a, b  — signed compare original a vs b
		code = append(code, emitCmpReg64(b, a)...)

		// 2) If needed, save original a → RAX
		if useTmp {
			code = append(code, emitPushReg(RAX)...) // PUSH RAX
			// MOV RAX, a
			code = append(code, emitMovRegToRegWithManualConstruction(RAX, a)...)
		}

		// 3) MOV dst, b  — skip if dst == b
		if dstIdx != bIdx {
			code = append(code, emitMovRegToReg64(dst, b)...)
		}

		// 4) CMOVLE dst, src  — if A ≤ B signed, move A (or saved-A in RAX) into dst
		{
			src := a
			if useTmp {
				src = RAX
			}
			code = append(code, emitCmovcc(X86_OP2_CMOVLE, dst, src)...)
		}

		// 5) Restore RAX if we pushed it
		if useTmp {
			code = append(code, emitPopReg(RAX)...) // POP RAX
		}

		return code
	}
}

// generateMaxU returns a code‐gen function for unsigned MAX.
func generateMaxU() func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		aIdx, bIdx, dstIdx := extractThreeRegs(inst.Args)
		a := regInfoList[aIdx]
		b := regInfoList[bIdx]
		dst := regInfoList[dstIdx]

		var code []byte

		// choose the CMOV opcode: for MAX_U use CMOVB (X86_OP2_CMOVB), for MIN_U use CMOVA (X86_OP2_CMOVA)
		cmov := byte(X86_OP2_CMOVB) // MAX_U: CMOVB (A<B → move B)

		if dstIdx == bIdx {
			// alias: dst and B collide. Save B in RCX, then:
			//   CMP A,B
			//   MOV dst,A
			//   CMOV{B|A} dst,RCX
			//   POP RCX

			// 1) PUSH RCX
			code = append(code, emitPushReg(RCX)...)

			// 2) MOV RCX, B
			code = append(code, emitMovRegToRegWithManualConstruction(RCX, b)...) // REVIEW

			// 3) CMP A, B
			code = append(code, emitCmpReg64(b, a)...)

			// 4) MOV dst, A
			if dstIdx != aIdx {
				code = append(code, emitMovRegToReg64(dst, a)...)
			}

			// 5) CMOV{B|A} dst, RCX
			code = append(code, emitCmovcc(cmov, dst, RCX)...)

			// 6) POP RCX
			code = append(code, emitPopReg(RCX)...)

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
		// use RCX as scratch

		var code []byte
		if dstIdx == bIdx {
			// dst aliases B → save B in tmp first

			// 1) PUSH RCX
			code = append(code, emitPushReg(RCX)...)

			// 2) MOV RCX, B   ; tmp = original B
			code = append(code, emitMovRcxRegMinU(b)...) // REVIEW

			// 3) CMP A, B    ; compare A vs original B
			code = append(code, emitCmpReg64(b, a)...) // REVIEW

			// 4) MOV dst, A   ; dst←A
			code = append(code, emitMovRegToRegMinU(dst, a)...) // REVIEW

			// 5) CMOVA dst, RCX ; if A>B unsigned then dst←original B
			code = append(code, emitCmovaMinU(dst, RCX)...) // REVIEW

			// 6) POP RCX
			code = append(code, emitPopReg(RCX)...)

			return code
		}

		// Non-alias case: dst != B

		// 1) MOV dst, A
		code = append(code, emitMovRegToRegMinU(dst, a)...) // REVIEW

		// 2) CMP dst, B
		code = append(code, emitCmpReg64(b, dst)...) // REVIEW

		// 3) CMOVA dst, B
		code = append(code, emitCmovaMinU(dst, b)...) // REVIEW

		return code
	}
}

func generateCmovImm(isZero bool) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		dstIdx, condIdx, imm := extractTwoRegsOneImm(inst.Args)
		dst := regInfoList[dstIdx]
		cond := regInfoList[condIdx]

		var code []byte

		// 1) PUSH RCX
		code = append(code, emitPushReg(RCX)...)

		// 2) MOVABS RCX, imm64
		code = append(code, emitMovImmToReg64(RCX, uint64(imm))...)

		// 3) TEST cond, cond   → sets ZF=1 iff cond==0
		code = append(code, emitTestReg64(cond, cond)...)

		// 4) CMOVcc dst, RCX
		//    CMOVE (X86_OP2_CMOVE) if isZero==true, otherwise CMOVNE (X86_OP2_CMOVNE)
		cmovOp := byte(X86_OP2_CMOVE)
		if !isZero {
			cmovOp = X86_OP2_CMOVNE
		}
		code = append(code, emitCmovcc(cmovOp, dst, RCX)...)

		// 5) POP RCX
		code = append(code, emitPopReg(RCX)...)

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
		code = append(code, emitMovRegToReg32(dst, dst)...) // REVIEW: this is the same as below??

		// 2) ROR r/m32(dst), imm8 - use shift helper with subcode 1 for ROR
		code = append(code, emitShiftRegImm(dst, X86_OP_GROUP2_RM_IMM8, X86_REG_ROR, byte(imm&X86_SHIFT_MASK_32))...)

		// 3) MOVSXD r64_dst, r32_dst  ; sign‑extend low 32→64
		code = append(code, emitMovsxd64(dst, dst)...)

		return code
	}

	// ─── Non‑conflict (src!=dst) ───
	// 1) MOV r32_dst, r32_src
	code = append(code, emitMovRegToReg32(dst, src)...)
	// 2) ROR r/m32(dst), imm8 - use shift helper with subcode 1 for ROR
	code = append(code, emitShiftRegImm(dst, X86_OP_GROUP2_RM_IMM8, X86_REG_ROR, byte(imm&X86_SHIFT_MASK_32))...)
	// 3) MOVSXD r64_dst, r32_dst
	code = append(code, emitMovsxd64(dst, dst)...)

	return code
}

// Implements r64_dst = r64_src << imm8  (or >>, SAR, ROR with different subcodes)
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
			code = append(code, emitPushReg(BaseReg)...)

			dstIdx = BaseRegIndex
			dst = regInfoList[dstIdx]

			code = append(code, emitMovRegToRegImmShiftOp64Alt(src, dst)...)
		}

		needSaveRCX := dst.Name != "RCX"
		needXchg := src.Name != "RCX"

		if needSaveRCX {
			code = append(code, emitPushReg(RCX)...)
		}

		code = append(code, emitMovImmToReg64(dst, imm)...)

		if needXchg {
			code = append(code, emitXchgRegRcxImmShiftOp64Alt(src)...)
		}

		code = append(code, emitShiftRegClImmShiftOp64Alt(dst, subcode)...)

		if needXchg {
			code = append(code, emitXchgRegRcxImmShiftOp64Alt(src)...)
		}

		if needSaveRCX {
			code = append(code, emitPopReg(RCX)...)
		}

		if sameReg {
			// move result back to src
			code = append(code, emitMovBaseRegToSrcImmShiftOp64Alt(BaseReg, src)...)

			code = append(code, emitPopReg(BaseReg)...)
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
		if imm64 <= X86_MAX_INT32 {
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
	code = append(code, emitPushReg(RDX)...)
	code = append(code, emitPushReg(RCX)...)

	// 1) If n is invalid → directly zero out
	if n == 0 || n > 8 {
		code = append(code, emitMovImmToDstXEncode(dst)...)
		code = append(code, emitPushReg(RDX)...)
		return append(code, emitPushReg(RCX)...)
	}

	// 2) If n==8 → already 64-bit, no operation needed
	if n == 8 {
		code = append(code, emitPopReg(RCX)...)
		code = append(code, emitPopReg(RDX)...)
	}

	// Below: 1 <= n <= 7

	// 3) MOV r64_rcx, r64_dst   ; copy x → rcx
	code = append(code, emitMovDstToRcxXEncode(dst)...)

	// 4) SHR rcx, imm8          ; q = x >> (8*n-1)
	shift := byte(8*n - 1)
	code = append(code, emitShrRcxImmXEncode(shift)...)

	// 5) MOVABS rdx, factor     ; factor = ^((1<<(8*n))-1)
	mask := uint64((1 << (8 * n)) - 1)
	factor := ^mask
	code = append(code, emitMovAbsRdxXEncode(factor)...)

	// 6) IMUL rdx, rcx          ; rdx = q * factor
	code = append(code, emitImulRdxRcxXEncode()...)

	// 7) ADD r64_dst, r64_rdx   ; x + q*factor
	code = append(code, emitAddDstRdxXEncode(dst)...)

	// 8) restore rcx and rdx
	code = append(code, emitPopReg(RCX)...)
	code = append(code, emitPopReg(RDX)...)

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
	code = append(code, emitMovRegToReg64NegAddImm32(dst, src)...)

	// 2) NEG r64_dst
	code = append(code, emitNegReg64NegAddImm32(dst)...)

	// 3) ADD r64_dst, imm32
	code = append(code, emitAddRegImm32_100(dst, uint32(imm))...)

	// 4) Truncate to 32-bit: MOV r/m32(dst), r32(dst)
	code = append(code, emitMovReg32ToReg32NegAddImm32(dst)...)

	// 5) Sign extend to 64-bit
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
		shiftAmt := byte(imm & X86_SHIFT_MASK_32)
		code = append(code, emitShiftRegImm32(dst, X86_OP_GROUP2_RM_IMM8, subcode, shiftAmt)...)

		// 3) MOVSXD r64_dst, r/m32(dst)  → sign-extend to 64-bit
		code = append(code, emitMovsxd64(dst, dst)...)

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
			code = append(code, emitXchgReg32Reg32(RCX, src)...) // RCX

			// 3) D3 /subcode r/m32(dst), CL - use shift helper
			code = append(code, emitShiftReg32ByCl(dst, subcode)...)

			// 4) XCHG ECX, r32_src  ; restore ECX and src
			code = append(code, emitXchgReg32Reg32(RCX, src)...)

			// 5) if SAR (subcode==7), sign-extend 32->64: MOVSXD r64_dst, r/m32(dst)
			if subcode == 7 {
				code = append(code, emitMovsxd64(dst, dst)...)
			}
		} else {
			// NORMAL: register value on left, immediate count
			// 1) MOV r32_dst, r32_src (89 /r)
			code = append(code, emitMovRegToReg32(dst, src)...)

			// 2) C1 /subcode r/m32(dst), imm8 - 32-bit shift
			code = append(code, emitShiftRegImm32(dst, opcode, subcode, byte(imm&X86_MASK_8BIT))...)

			// 3) if SAR (subcode==7), sign-extend 32->64: MOVSXD r64_dst, r/m32(dst)
			if subcode == 7 {
				code = append(code, emitMovsxd64(dst, dst)...)
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
		scratch := RCX // RCX

		var code []byte

		// --- alias case: dst and src are the same register ---
		if dstIdx == srcIdx {
			// 1) save RCX
			code = append(code, emitPushReg(RCX)...) // PUSH RCX

			// 2) sign‑extend the 32‑bit dst into RCX
			// opcode X86_OP_MOV_R_RM /r = MOV r64, r/m64 (this is MOV RCX, dst)
			code = append(code, emitMovRegToRegWithManualConstruction(scratch, dstInfo)...)

			// 3) clear the low 32 bits of dst
			code = append(code, emitXorReg32(dstInfo, dstInfo)...)

			// 4) signed compare RCX vs imm32
			code = append(code, emitCmpRegImm32Force81(scratch, int32(imm))...)

			// 5) SETcc into low byte of dst
			code = append(code, emitSetccReg8(setcc, dstInfo)...)

			// 6) restore RCX
			code = append(code, emitPopReg(RCX)...) // POP RCX
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
// Implements dst := sign‑extended((src * imm) & X86_MASK_32BIT)
func generateImmMulOp32(inst Instruction) []byte {
	dstReg, srcReg, imm := extractTwoRegsOneImm(inst.Args)
	src := regInfoList[srcReg]
	dst := regInfoList[dstReg]

	// 1) copy src → dst (32‑bit)
	code := append([]byte{}, emitMovRegToReg32(dst, src)...)

	// 2) IMUL dst, src, imm32  (yields 32‑bit truncated product in dst)
	code = append(code, emitImulRegImm32(dst, src, uint32(imm))...)

	// 3) MOVSXD r64_dst, r32_dst  (sign‑extend low 32 bits back to 64 bits)
	code = append(code, emitMovsxd64(dst, dst)...)

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
		return emitMovsxd64(dst, src)
	}

	// 1) MOV r32_dst, r32_src BUT  Skip the MOV when src and dst are the same!!
	if srcReg != dstReg {
		// 1) MOV r32_dst, r32_src
		code = append(code, emitMovRegToReg32(dst, src)...)
	}

	// 2) ADD r32_dst, imm32
	code = append(code, emitAddRegImm32WithManualConstruction(dst, imm)...)

	// 3) MOVSXD r64_dst, r/m32   ; sign-extend low 32→64
	code = append(code, emitMovsxd64(dst, dst)...)
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
		if imm64 >= -X86_MIN_INT32 && imm64 <= X86_MAX_INT32 {
			copySrcToDst()
			code = append(code, emitAluRegImm32(dst, subcode, int32(imm64))...)
			return code
		}

		// 3) Full imm64 via scratch + XCHG swap
		copySrcToDst()
		scratch := RAX

		// swap dst<->scratch: XCHG r64,dst
		code = append(code, emitXchgReg64(scratch, dst)...)

		// MOVABS scratch, imm64 (now scratch contains the old dst value)
		code = append(code, emitMovImmToReg64(scratch, uint64imm)...)

		// ALU dst, scratch  (dst now has old scratch value, scratch has immediate)
		code = append(code, emitAluRegReg(dst, scratch, subcode)...)

		// swap back scratch↔dst: XCHG
		code = append(code, emitXchgReg64(scratch, dst)...)

		return code
	}
}

// generateLoadIndSignExtend returns a function that generates machine code
func generateLoadIndSignExtend(
	prefix byte, // Optional extra prefix (e.g. X86_PREFIX_66), or X86_NO_PREFIX if none
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
			code := emitLoadWithSIB(regA, BaseReg, regB, int32(vx), []byte{X86_PREFIX_0F, X86_OP2_MOVSX_R_RM8}, is64bit, prefix)
			code = append(code, castReg8ToU64(regAIndex)...)
			return code
		case LOAD_IND_I16:
			// X86_PREFIX_66 prefix + MOVSX r64, word ptr [BaseReg + regB*1 + disp32] (0F BF /r)
			code := emitLoadWithSIB(regA, BaseReg, regB, int32(vx), []byte{X86_PREFIX_0F, X86_OP2_MOVSX_R_RM16}, is64bit, X86_PREFIX_66)
			code = append(code, castReg16ToU64(regAIndex)...)
			return code
		case LOAD_IND_I32:
			// MOVSXD r64, dword ptr [BaseReg + regB*1 + disp32] (63 /r)
			return emitLoadWithSIB(regA, BaseReg, regB, int32(vx), []byte{X86_OP_MOVSXD}, is64bit, prefix)
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
	prefix byte, // Optional extra prefix (e.g. X86_PREFIX_66), or X86_NO_PREFIX if none
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
			return emitLoadWithSIB(regA, BaseReg, regB, int32(vx), []byte{X86_PREFIX_0F, X86_OP2_MOVZX_R_RM8}, is64bit, prefix)

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

			code := emitLoadWithSIB(regA, base, index, int32(vx), []byte{X86_PREFIX_0F, X86_OP2_MOVZX_R_RM16}, false, X86_PREFIX_66)
			// AND with X86_MASK_16BIT to ensure proper zero-extension
			code = append(code, emitAluRegImm32(regA, 4, X86_MASK_16BIT)...)
			return code

		case LOAD_IND_U32:
			// zero-extend via 32-bit MOV → clears upper 32 bits in 64-bit reg
			return emitLoadWithSIB(regA, BaseReg, regB, int32(vx), []byte{X86_OP_MOV_R_RM}, false, prefix)

		case LOAD_IND_U64:
			// full 64-bit MOV r64, r/m64
			return emitLoadWithSIB(regA, BaseReg, regB, int32(vx), []byte{X86_OP_MOV_R_RM}, true, prefix)

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
