package recompiler

import (
	"encoding/binary"
)

func generateXnorOp64(inst Instruction) []byte {
	reg1, reg2, dst := extractThreeRegs(inst.Args)
	src1 := regInfoList[reg1]
	src2 := regInfoList[reg2]
	dstReg := regInfoList[dst]

	var code []byte
	// XNOR: dst = ~(src1 ^ dst)
	// Step 1: MOV dst, src1
	code = append(code, emitMovRegToReg64(dstReg, src1)...)
	// Step 2: XOR dst, src2
	code = append(code, emitXorReg64(dstReg, src2)...)
	// Step 3: NOT dst
	code = append(code, emitNotReg64(dstReg)...)

	return code
}

func generateOrInvOp64(inst Instruction) []byte {
	rA, rB, rDst := extractThreeRegs(inst.Args)
	srcA := regInfoList[rA]
	srcB := regInfoList[rB]
	dst := regInfoList[rDst]

	var code []byte
	conflict := (rDst == rA)
	if conflict {
		// 0) save original A in RAX
		code = append(code, emitPushReg(RAX)...) // PUSH RAX

		//    MOV RAX, srcA
		code = append(code, emitMovRegToReg64(RAX, srcA)...)
	}

	// 1) MOV dst, srcB        ; dst ← B
	code = append(code, emitMovRegToReg64(dst, srcB)...)

	// 2) NOT dst              ; dst = ^B
	code = append(code, emitNotReg64(dst)...)

	// 3) OR dst, src          ; dst = (~B) | X
	if conflict {
		//    OR dst, RAX       ; use our saved original A
		code = append(code, emitOrReg64(dst, RAX)...)

		//    restore RAX
		code = append(code, emitPopReg(RAX)...) // POP RAX
	} else {
		//    OR dst, srcA      ; original non-conflict path
		code = append(code, emitOrReg64(dst, srcA)...)
	}

	return code
}

func generateAndInvOp64(inst Instruction) []byte {
	reg1, reg2, dstIdx := extractThreeRegs(inst.Args)
	src1 := regInfoList[reg1]
	src2 := regInfoList[reg2]
	dst := regInfoList[dstIdx]
	tmp := BaseReg // e.g., r12

	var code []byte

	// 1) PUSH tmp to preserve original value
	if tmp.Name == "r12" {
		code = append(code, emitPushReg(tmp)...) // PUSH R12
	} else {
		panic("Unsupported BaseReg for push")
	}

	// 2) MOV tmp, src2
	code = append(code, emitMovRegToReg64(tmp, src2)...)

	// 3) NOT tmp
	code = append(code, emitNotReg64(tmp)...)

	// 4) AND tmp, src1
	code = append(code, emitAndReg64(tmp, src1)...)

	// 5) MOV dst, tmp
	code = append(code, emitMovRegToReg64(dst, tmp)...)

	// 6) POP tmp to restore
	if tmp.Name == "r12" {
		code = append(code, emitPopReg(tmp)...) // POP R12
	} else {
		panic("Unsupported BaseReg for pop")
	}

	return code
}

// rem unsigned 64-bit with B==0 ⇒ result=A

// generateRemUOp64 emits code for dst = src1 % src2 (unsigned)
// Uses Method B: conditional spill/restore of RAX/RDX and optimized MOV/XCHG
func generateRemUOp64(inst Instruction) []byte {
	srcIdx, src2Idx, dstIdx := extractThreeRegs(inst.Args)
	src1 := regInfoList[srcIdx]
	src2 := regInfoList[src2Idx]
	dst := regInfoList[dstIdx]
	buf := []byte{}

	// Determine if dst conflicts with RAX/RDX
	isDstRAX := dstIdx == 0
	isDstRDX := dstIdx == 2

	// 0) Conditional spill: only push if we need to restore later
	if !isDstRAX {
		buf = append(buf, emitPushReg(RAX)...)
	}
	if !isDstRDX {
		buf = append(buf, emitPushReg(RDX)...)
	}

	// 1) MOV dividend into RAX (if src1 != RAX)
	if srcIdx != 0 {
		buf = append(buf, emitMovRaxFromRegRemUOp64(src1)...)
	}

	// 2) TEST divisor != 0
	buf = append(buf, emitTestReg64(src2, src2)...)

	// JE zeroDiv placeholder
	jeOff := len(buf)
	buf = append(buf, X86_PREFIX_0F, X86_OP2_JE, 0, 0, 0, 0)

	// 3) Normal path: clear RDX, DIV, move remainder
	// XOR RDX, RDX
	buf = append(buf, emitXorReg64(RDX, RDX)...)

	isSrc2RAX := src2Idx == 0
	isSrc2RDX := src2Idx == 2

	if isSrc2RAX {
		// If divisor is RAX, use the spilled original RAX from [RSP+8]
		buf = append(buf, emitDivMemStackRemUOp64RAX()...)
	} else if isSrc2RDX {
		// If divisor is RDX, use the spilled original RDX from [RSP]
		buf = append(buf, emitDivMemStackRemUOp64RDX()...)
	} else {
		buf = append(buf, emitDivRegRemUOp64(src2)...)
	}

	// Move remainder into dst
	if isDstRAX {
		// XCHG RAX, RDX => remainder ends up in RAX
		buf = append(buf, emitXchgRaxRdxRemUOp64()...)
	} else if !isDstRDX {
		// MOV dst, RDX
		buf = append(buf, emitMovDstFromRdxRemUOp64(dst)...)
	}

	// 4) JMP end placeholder
	jmpOff := len(buf)
	buf = append(buf, X86_OP_JMP_REL32, 0, 0, 0, 0)

	// zeroDiv: divisor was zero => dst = dividend (in RAX)
	zeroOff := len(buf)
	if !isDstRAX {
		buf = append(buf, emitMovDstFromRaxRemUOp64(dst)...)
	}

	// end: restore if spilled
	endOff := len(buf)
	if !isDstRDX {
		buf = append(buf, emitPopReg(RDX)...)
	}
	if !isDstRAX {
		buf = append(buf, emitPopReg(RAX)...)
	}

	// Patch JE and JMP offsets
	binary.LittleEndian.PutUint32(buf[jeOff+2:], uint32(zeroOff-(jeOff+6)))
	binary.LittleEndian.PutUint32(buf[jmpOff+1:], uint32(endOff-(jmpOff+5)))

	return buf
}

// Handles src2==RDX with spill/restore and memory operand special case
func generateRemSOp64(inst Instruction) []byte {
	srcIdx, src2Idx, dstIdx := extractThreeRegs(inst.Args)
	srcInfo := regInfoList[srcIdx]
	src2Info := regInfoList[src2Idx]
	dstInfo := regInfoList[dstIdx]

	isDstRAX := dstIdx == 0
	isDstRDX := dstIdx == 2
	isSrc2RDX := src2Idx == 2

	buf := []byte{}

	// Conditional spill
	if !isDstRAX {
		buf = append(buf, emitPushReg(RAX)...)
	}
	if !isDstRDX {
		buf = append(buf, emitPushReg(RDX)...)
	}

	// MOV RAX, src1
	buf = append(buf, emitMovRaxFromRegRemSOp64(srcInfo)...)

	// TEST src2, src2 (b==0?)
	buf = append(buf, emitTestReg64(src2Info, src2Info)...)

	// JE zeroDiv
	jeZeroOff := len(buf)
	buf = append(buf, X86_PREFIX_0F, X86_OP2_JE, 0, 0, 0, 0)

	// Overflow check a==INT64_MIN
	// MOV RDX, X86_MIN_INT64
	buf = append(buf, emitMovRdxIntMinRemSOp64()...)
	// CMP RAX, RDX
	buf = append(buf, emitCmpRaxRdxRemSOp64()...)
	// JNE doRem
	jneOff := len(buf)
	buf = append(buf, X86_PREFIX_0F, X86_OP2_JNE, 0, 0, 0, 0)

	// Overflow check b==-1
	if isSrc2RDX {
		// CMP [RSP], imm32(-1)
		buf = append(buf, emitCmpMemStackNeg1RemSOp64()...)
	} else {
		// CMP RDX, imm32(-1)
		buf = append(buf, emitCmpRegNeg1RemSOp64(src2Info)...)
	}
	// JE overflowCase
	jeOvfOff := len(buf)
	buf = append(buf, X86_PREFIX_0F, X86_OP2_JE, 0, 0, 0, 0)

	// doRem: CQO + IDIV
	doRemOff := len(buf)
	buf = append(buf, emitCqoRemSOp64()...)

	if isSrc2RDX {
		// IDIV qword ptr [RSP]
		buf = append(buf, emitIdivMemStackRemSOp64()...)
	} else {
		buf = append(buf, emitIdivRegRemSOp64(src2Info)...)
	}

	// Move remainder to dst
	if isDstRAX {
		buf = append(buf, emitXchgRaxRdxRemUOp64()...) // XCHG RAX,RDX
	} else if !isDstRDX {
		// MOV dst, RDX
		buf = append(buf, emitMovDstFromRdxRemUOp64(dstInfo)...)
	}

	// JMP end
	jmpEndOff := len(buf)
	buf = append(buf, X86_OP_JMP_REL32, 0, 0, 0, 0)

	// zeroDiv: b==0 ⇒ result=a (in RAX)
	zeroOff := len(buf)
	if !isDstRAX {
		buf = append(buf, emitMovDstFromRaxRemUOp64(dstInfo)...)
	}
	buf = append(buf, X86_OP_JMP_REL32, 0, 0, 0, 0)
	jmpZeroEnd := len(buf) - 4

	// overflowCase: result = 0
	ovfOff := len(buf)
	buf = append(buf, emitXorRegRegRemSOp64(dstInfo)...)
	buf = append(buf, X86_OP_JMP_REL32, 0, 0, 0, 0)
	jmpOvfEnd := len(buf) - 4

	// end: restore
	endOff := len(buf)
	if !isDstRDX {
		buf = append(buf, emitPopReg(RDX)...)
	}
	if !isDstRAX {
		buf = append(buf, emitPopReg(RAX)...)
	}

	// patch branch offsets
	binary.LittleEndian.PutUint32(buf[jeZeroOff+2:], uint32(zeroOff-(jeZeroOff+6)))
	binary.LittleEndian.PutUint32(buf[jneOff+2:], uint32(doRemOff-(jneOff+6)))
	binary.LittleEndian.PutUint32(buf[jeOvfOff+2:], uint32(ovfOff-(jeOvfOff+6)))
	binary.LittleEndian.PutUint32(buf[jmpEndOff+1:], uint32(endOff-(jmpEndOff+5)))
	binary.LittleEndian.PutUint32(buf[jmpZeroEnd:], uint32(endOff-(jmpZeroEnd+4)))
	binary.LittleEndian.PutUint32(buf[jmpOvfEnd:], uint32(endOff-(jmpOvfEnd+4)))

	return buf
}
func generateRemUOp32(inst Instruction) []byte {
	srcIdx, src2Idx, dstIdx := extractThreeRegs(inst.Args)
	srcInfo := regInfoList[srcIdx]
	src2Info := regInfoList[src2Idx]
	dstInfo := regInfoList[dstIdx]

	var code []byte

	// Push RAX and RDX
	code = append(code, emitPushReg(RAX)...)
	code = append(code, emitPushReg(RDX)...)

	// 1) MOV EAX, src32
	code = append(code, emitMovEaxFromRegRemUOp32(srcInfo)...)

	// 2) TEST src2, src2
	code = append(code, emitTestReg32(src2Info, src2Info)...)

	// 3) JNE doDiv
	jneOff := len(code)
	code = append(code, emitJne32()...)

	// 4) divisor==0 → MOVSXD dst, EAX  (sign‐extend low32→full64)
	code = append(code, emitMovsxdDstEaxRemUOp32(dstInfo)...)

	// 5) JMP end
	jmpOff := len(code)
	code = append(code, emitJmp32()...)

	// doDiv:
	doDiv := len(code)

	//    a) XOR EDX, EDX
	code = append(code, emitXorReg32(EDX, EDX)...)

	//    b) DIV r/m32 - handle src2 conflicts with RAX/RDX
	isSrc2RAX := src2Idx == 0
	isSrc2RDX := src2Idx == 2

	if isSrc2RAX {
		// If divisor is RAX, use the spilled original RAX from [RSP+8]
		code = append(code, emitDivMemStackRemUOp32RAX()...)
	} else if isSrc2RDX {
		// If divisor is RDX, use the spilled original RDX from [RSP]
		code = append(code, emitDivMemStackRemUOp32RDX()...)
	} else {
		code = append(code, emitDiv32(src2Info)...)
	}

	//    c) MOV dst, EDX
	code = append(code, emitMovDstEdxRemUOp32(dstInfo)...)

	// patch JNE→doDiv and JMP→end, restore, etc...
	// patch JNE → doDiv
	end := len(code)
	binary.LittleEndian.PutUint32(code[jneOff+2:], uint32(doDiv-(jneOff+6)))
	// patch JMP → end
	binary.LittleEndian.PutUint32(code[jmpOff+1:], uint32(end-(jmpOff+5)))

	// Restore
	code = append(code, emitPopRdxRaxRemUOp32()...)
	return code
}

// Implements DIV unsigned: r64_dst = uint64(r32_src1 / r32_src2),
// but if (src2 & X86_MASK_32BIT)==0 then dst = 0xFFFFFFFFFFFFFFFF.
// Preserves RAX and RDX.
func generateDivUOp32(inst Instruction) []byte {
	srcIdx1, srcIdx2, dstIdx := extractThreeRegs(inst.Args)
	src1Info := regInfoList[srcIdx1]
	src2Info := regInfoList[srcIdx2]
	dstInfo := regInfoList[dstIdx]

	var code []byte

	// Push RAX and RDX
	code = append(code, emitPushReg(RAX)...)
	code = append(code, emitPushReg(RDX)...)

	// 1) MOV EAX, src1
	code = append(code, emitMovEaxFromRegDivUOp32(src1Info)...)

	// 2) TEST src2, src2
	code = append(code, emitTestReg32(src2Info, src2Info)...)

	// 3) JNE doDiv
	jneOff := len(code)
	code = append(code, emitJne32()...)

	// --- divisor == 0 path: produce maxUint64 via XOR/NOT ---

	// XOR r64_dst, r64_dst  → zero
	code = append(code, emitXorReg64Reg64(dstInfo)...)

	// NOT r/m64, r64  → invert to all ones
	code = append(code, emitNotReg64DivUOp32(dstInfo)...)

	// JMP end
	jmpOff := len(code)
	code = append(code, emitJmp32()...)

	// --- doDiv: normal unsigned divide ---

	doDiv := len(code)

	// XOR EDX, EDX
	code = append(code, emitXorReg32(EDX, EDX)...)

	// DIV r/m32 = src2 - handle src2 conflicts with RAX/RDX
	isSrc2RAX := srcIdx2 == 0
	isSrc2RDX := srcIdx2 == 2

	if isSrc2RAX {
		// If divisor is RAX, use the spilled original RAX from [RSP+8]
		code = append(code, emitDivMemStackRemUOp32RAX()...)
	} else if isSrc2RDX {
		// If divisor is RDX, use the spilled original RDX from [RSP]
		code = append(code, emitDivMemStackRemUOp32RDX()...)
	} else {
		code = append(code, emitDiv32(src2Info)...)
	}

	// MOV r64_dst, EAX (zero-extends quotient)
	code = append(code, emitMovDstEaxDivUOp32(dstInfo)...)

	// --- patch jumps ---
	end := len(code)
	binary.LittleEndian.PutUint32(code[jneOff+2:], uint32(doDiv-(jneOff+6)))
	binary.LittleEndian.PutUint32(code[jmpOff+1:], uint32(end-(jmpOff+5)))

	// restore RDX, RAX
	code = append(code, emitPopRdxRaxDivUOp32()...)

	return code
}

// Implements signed 32-bit division with special cases:
//
//	if b == 0            → result = maxUint64
//	else if a==MinInt32 && b==-1 → result = uint64(a)
//	else                 → result = uint64(int64(a/b))
func generateDivSOp32(inst Instruction) []byte {
	srcIdx, src2Idx, dstIdx := extractThreeRegs(inst.Args)
	srcInfo := regInfoList[srcIdx]
	src2Info := regInfoList[src2Idx]
	dstInfo := regInfoList[dstIdx]

	code := []byte{}
	code = append(code, emitPushReg(RAX)...) // PUSH RAX
	code = append(code, emitPushReg(RDX)...) // PUSH RDX

	// 1) MOV EAX, src
	code = append(code, emitMovEaxFromReg32(srcInfo)...)

	// 2) TEST src2, src2  (b==0?)
	code = append(code, emitTestReg32(src2Info, src2Info)...)

	// 3) JNE div_not_zero
	jneDiv := len(code)
	code = append(code, emitJne32()...)

	// --- b == 0: dst = maxUint64 via XOR/NOT ---
	code = append(code, emitXorReg64Reg64(dstInfo)...)
	code = append(code, emitNotReg64(dstInfo)...)

	// 4) JMP end
	jmpEnd := len(code)
	code = append(code, emitJmp32()...)

	// fix up div_not_zero
	divPos := len(code)
	binary.LittleEndian.PutUint32(code[jneDiv+2:], uint32(divPos-(jneDiv+6)))

	// 5) CMP EAX, X86_MIN_INT32  (a == MinInt32?)
	code = append(code, emitCmpRegImm32MinInt(X86Reg{RegBits: 0, REXBit: 0})...) // EAX

	// 6) JNE normal_div
	jneOvf1 := len(code)
	code = append(code, emitJne32()...)

	// 7) CMP src2, -1       (b == -1?)
	code = append(code, emitCmpRegImmByte(src2Info, X86_NEG_ONE)...)

	// 8) JNE normal_div
	jneOvf2 := len(code)
	code = append(code, emitJne32()...)

	// --- overflow path: dst = uint64(a) via MOVSXD dst, EAX ---
	code = append(code, emitMovsxd64(dstInfo, X86Reg{RegBits: 0, REXBit: 0})...) // MOVSXD dst, EAX

	// 9) JMP end
	jmpOvfEnd := len(code)
	code = append(code, emitJmp32()...)

	// fix up overflow jumps
	normPos := len(code)
	binary.LittleEndian.PutUint32(code[jneOvf1+2:], uint32(normPos-(jneOvf1+6)))
	binary.LittleEndian.PutUint32(code[jneOvf2+2:], uint32(normPos-(jneOvf2+6)))

	// --- normal_div: CDQ; IDIV; MOVSXD dst, EAX ---

	// CDQ (sign-extend EAX → EDX)
	code = append(code, emitCdq()...)

	// IDIV r/m32 = src2 - handle src2 conflict with RDX
	isSrc2RDX := src2Idx == 2
	if isSrc2RDX {
		// If divisor is RDX, use the spilled original RDX from [RSP]
		code = append(code, emitIdivMemStackRemSOp32RDX()...)
	} else {
		code = append(code, emitIdiv32(src2Info)...)
	}

	// MOVSXD dst, EAX
	code = append(code, emitMovsxd64(dstInfo, X86Reg{RegBits: 0, REXBit: 0})...) // MOVSXD dst, EAX

	// patch end jumps
	endPos := len(code)
	binary.LittleEndian.PutUint32(code[jmpEnd+1:], uint32(endPos-(jmpEnd+5)))
	binary.LittleEndian.PutUint32(code[jmpOvfEnd+1:], uint32(endPos-(jmpOvfEnd+5)))

	// -- Restore RDX, RAX --
	code = append(code, emitPopRdxRax()...)

	return code
}

// Implements a 32-bit register-register signed MUL (MUL_32):
//
//	r32_dst = r32_src1 * r32_src2   (then sign-extend back to r64_dst)
//
// If dst == src2, we spill src2 into RAX first.
func generateMul32(inst Instruction) []byte {
	srcIdx1, srcIdx2, dstIdx := extractThreeRegs(inst.Args)
	src1 := regInfoList[srcIdx1]
	src2 := regInfoList[srcIdx2]
	dst := regInfoList[dstIdx]

	var code []byte

	// ─── Handle conflict (dst == src2) by spilling src2 into RAX ───
	if dstIdx == srcIdx2 {
		tmp := RAX // RAX

		// 0) PUSH RAX
		code = append(code, emitPushReg(tmp)...)

		// 1) MOV EAX, r32_src2
		code = append(code, emitMovReg32(tmp, src2)...)

		// 2) MOV r32_dst, r/m32 src1
		code = append(code, emitMovReg32(dst, src1)...)

		// 3) IMUL r32_dst, r32_tmp
		code = append(code, emitImulReg32(dst, tmp)...)

		// 4) MOVSXD r64_dst, r32_dst  ; sign-extend low 32 bits into 64
		code = append(code, emitMovsxd64(dst, dst)...)

		// 5) POP RAX
		code = append(code, emitPopReg(tmp)...)
		return code
	}

	// ─── No conflict: dst != src2 ───

	// 1) MOV r32_dst, r/m32 src1
	code = append(code, emitMovReg32(dst, src1)...)

	// 2) IMUL r32_dst, r32_src2
	code = append(code, emitImulReg32(dst, src2)...)

	// 3) MOVSXD r64_dst, r32_dst
	code = append(code, emitMovsxd64(dst, dst)...)

	return code
}

// generateMul64 multiplies two 64-bit operands and stores the low 64 bits of the result in dst
// It handles aliasing between dst and source registers by using BaseReg as a scratch when needed
func generateMul64(inst Instruction) []byte {
	src1Idx, src2Idx, dstIdx := extractThreeRegs(inst.Args)
	src1 := regInfoList[src1Idx]
	src2 := regInfoList[src2Idx]
	dst := regInfoList[dstIdx]
	scratch := BaseReg

	var code []byte
	// If dst aliases src2 (and not src1), preserve src2 in scratch
	if dstIdx == src2Idx && dstIdx != src1Idx {
		// push scratch
		code = append(code, emitPushReg(scratch)...)

		// MOV scratch, src2
		code = append(code, emitMovRegToReg64(scratch, src2)...)

		// Treat scratch as the new src2
		src2 = scratch
	}

	// MOV dst, src1
	code = append(code, emitMovRegToReg64(dst, src1)...)

	// IMUL dst, src2
	code = append(code, emitImul64(dst, src2)...)

	// Restore scratch if used
	if dstIdx == src2Idx && dstIdx != src1Idx {
		// pop scratch
		code = append(code, emitPopReg(scratch)...)
	}

	return code
}

// Note: emitRex is replaced by buildREX from recompiler_helper.go for consistency

func generateBinaryOp32(opcode byte) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		r1, r2, rd := extractThreeRegs(inst.Args)
		src1 := regInfoList[r1]
		src2 := regInfoList[r2]
		dst := regInfoList[rd]

		var code []byte
		// fmt.Printf("generateBinaryOp32 (ADD_32/SUB_32): %s (Reg %d) %s (Reg %d) => %s (Reg %d)\n",
		// 	src1.Name, r1, src2.Name, r2, dst.Name, rd)

		// ─── CASE A: conflict dst==src2 ───
		if r2 == rd {
			// choose a scratch register different from operands
			scratchIdx := -1
			for i := range regInfoList {
				if i != r1 && i != r2 {
					scratchIdx = i
					break
				}
			}
			if scratchIdx == -1 {
				panic("generateBinaryOp32: no scratch register available")
			}
			scratch := regInfoList[scratchIdx]

			// 1) push scratch to preserve original value
			code = append(code, emitPushReg(scratch)...)

			// 2) mov scratch32, src2d (save original dst/src2 value)
			code = append(code, emitMovRegReg32(scratch, src2)...)

			// 3) mov dst32, src1d
			code = append(code, emitMovRegReg32(dst, src1)...)

			// 4) dst32 = dst32 <op> scratch32
			code = append(code, emitBinaryOpRegReg32(opcode, dst, scratch)...)

			// 5) sign‑extend into 64 bits
			code = append(code, emitMovsxd64(dst, dst)...)

			// 6) pop scratch (restore register)
			code = append(code, emitPopReg(scratch)...)
			return code
		}

		// ─── CASE B: dst==src1 ───
		if r1 == rd {
			// just do dst32 = dst32 <op> src2d
			code = append(code, emitBinaryOpRegReg32(opcode, dst, src2)...)
			// sign‑extend
			code = append(code, emitMovsxd64(dst, dst)...)
			return code
		}

		// ─── CASE C: no conflict ───
		// 1) MOV dst32, src1d
		code = append(code, emitMovRegReg32(dst, src1)...)
		// 2) dst32 = dst32 <op> src2d
		code = append(code, emitBinaryOpRegReg32(opcode, dst, src2)...)
		// 3) sign‐extend
		code = append(code, emitMovsxd64(dst, dst)...)
		return code
	}
}

// Implements a register‐register arithmetic right shift on 64‐bit values:
//
//	r64_dst = int64(r64_src1) >> (r8_src2 & 63)
func generateShiftOp64B(opcode byte, regField byte) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		src1Idx, src2Idx, dstIdx := extractThreeRegs(inst.Args)
		src1 := regInfoList[src1Idx]
		src2 := regInfoList[src2Idx]
		dst := regInfoList[dstIdx]

		var code []byte

		// 1) PUSH RCX (preserve)
		code = append(code, emitPushReg(RCX)...)

		// 2) MOV RCX, src2  -> CL = shift count
		code = append(code, emitMovRcxFromRegShiftOp64B(src2)...)

		// 3) MOV dst, src1
		code = append(code, emitMovDstFromSrcShiftOp64B(dst, src1)...)

		// 4) SHIFT dst by CL
		code = append(code, emitShiftDstByClShiftOp64B(opcode, regField, dst)...)

		// 5) POP RCX
		code = append(code, emitPopReg(RCX)...)

		return code
	}
}
func generateROTL64() func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		// pull out our three regs
		src1Idx, src2Idx, dstIdx := extractThreeRegs(inst.Args)
		a := regInfoList[src1Idx] // valueA (to be rotated)
		b := regInfoList[src2Idx] // valueB (rotate count)
		dst := regInfoList[dstIdx]

		var code []byte
		// fmt.Printf("generateROTL64:  a: %s (Reg %d), b: %s (Reg %d), dst: %s (Reg %d)\n",
		// 	a.Name, src1Idx,
		// 	b.Name, src2Idx,
		// 	dst.Name, dstIdx)
		// ─── 1) LOAD COUNT INTO CL ───
		// If count isn't already in CL (reg 1) and we won't overwrite CL as the dst:
		if src2Idx != 1 && dstIdx != 1 {
			// push rcx
			code = append(code, emitPushReg(RCX)...)

			// mov rcx, r64_b
			//   REX.W=1, REX.B=b.REXBit
			code = append(code,
				buildREX(true, false, false, b.REXBit == 1),
				X86_OP_MOV_R_RM, // MOV r64_reg, r64_rm
				byte(X86_MOD_REGISTER<<6|(1<<3)|b.RegBits), // reg=1 (RCX), rm=b (FIXED)
			)
		}

		// ─── 2) COPY valueA → dst ───
		if src1Idx != dstIdx {
			// MOV r/m64: X86_OP_MOV_RM_R /r
			code = append(code,
				buildREX(true, a.REXBit == 1, false, dst.REXBit == 1),
				X86_OP_MOV_RM_R,
				byte(X86_MOD_REGISTER<<6|(a.RegBits<<3)|dst.RegBits),
			)
		}

		// ─── 3) ROL dst, CL ───
		// Opcode: X86_OP_GROUP2_RM_CL /0 = ROL r/m64, CL
		code = append(code,
			buildREX(true, false, false, dst.REXBit == 1),
			X86_OP_GROUP2_RM_CL,
			byte(X86_MOD_REGISTER<<6|(X86_REG_ROL<<3)|dst.RegBits), // /0 = ROL, rm=dst
		)

		// ─── 4) RESTORE RCX ───
		if src2Idx != 1 && dstIdx != 1 {
			code = append(code, emitPopReg(RCX)...) // pop rcx
		}

		return code
	}
}

// Implements: r64_dst = int64(r64_src1) >> (r64_src2 & 63)
func generateShiftOp64(opcode byte, regField byte) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		src1Idx, src2Idx, dstIdx := extractThreeRegs(inst.Args)
		src1 := regInfoList[src1Idx]
		src2 := regInfoList[src2Idx]
		dst := regInfoList[dstIdx]

		var code []byte
		// 1) MOV r64_dst, r64_src1
		code = append(code, emitMovRegToReg64(dst, src1)...)

		// 2) XCHG RCX, r64_src2  (save/restore CL)
		code = append(code, emitXchgRcxReg64(src2)...)

		// 3) D3 /n RCX, CL -> shift dst by CL (in RCX low 8 bits)
		code = append(code, emitShiftOp64(opcode, regField, dst)...)

		// 4) XCHG RCX, r64_src2  (restore original RCX)
		code = append(code, emitXchgRcxReg64(src2)...)

		return code
	}
}

func generateSHLO_L_64() func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		const (
			opcode   = byte(X86_OP_GROUP2_RM_CL) // X86_OP_GROUP2_RM_CL /4 = SHL r/m64, CL
			regField = byte(4)                   // /4 = SHL
		)

		src1Idx, src2Idx, dstIdx := extractThreeRegs(inst.Args)
		src1 := regInfoList[src1Idx]
		src2 := regInfoList[src2Idx]
		dst := regInfoList[dstIdx]

		// Constants for RCX/RAX indices
		const (
			raxIdx = 0
			rcxIdx = 1
		)
		rcx := regInfoList[rcxIdx]
		rax := regInfoList[raxIdx]

		var code []byte

		// Case 1: shift==dst
		if src2Idx == dstIdx {
			if dstIdx == rcxIdx {
				// shift==dst==RCX
				code = append(code, emitPushReg(rax)...)
				code = append(code, emitMovRegToReg64(rax, src2)...)
				code = append(code, emitMovRegToReg64(dst, src1)...)
				code = append(code, emitMovByteToCL(rax)...)
				code = append(code, emitShiftOp64(opcode, regField, dst)...)
				code = append(code, emitPopReg(rax)...)
			} else {
				// shift==dst!=RCX
				code = append(code, emitPushReg(rcx)...)
				code = append(code, emitMovRegToReg64(rcx, src2)...)
				code = append(code, emitMovRegToReg64(dst, src1)...)
				code = append(code, emitShiftOp64(opcode, regField, dst)...)
				code = append(code, emitPopReg(rcx)...)
			}

			// Case 2: dst==RCX!=shift
		} else if dstIdx == rcxIdx {
			if src2Idx == raxIdx {
				// shift in RAX
				code = append(code, emitPushReg(rax)...)
				code = append(code, emitMovRegToReg64(rcx, src1)...)
				code = append(code, emitMovRegToReg64(rax, src2)...)
				code = append(code, emitXchgRegReg64(rax, rcx)...)
				code = append(code, emitShiftOp64(opcode, regField, rax)...)
				code = append(code, emitMovRegToReg64(rcx, rax)...)
				code = append(code, emitPopReg(rax)...)
			} else {
				// shift elsewhere
				code = append(code, emitPushReg(rax)...)
				code = append(code, emitMovRegToReg64(rax, src1)...)
				code = append(code, emitMovRegToReg64(rcx, src2)...)
				code = append(code, emitShiftOp64(opcode, regField, rax)...)
				code = append(code, emitMovRegToReg64(rcx, rax)...)
				code = append(code, emitPopReg(rax)...)
			}

			// Case 3: dst!=RCX && shift!=dst
		} else {
			code = append(code, emitMovRegToReg64(dst, src1)...)
			if src2Idx == rcxIdx {
				code = append(code, emitShiftOp64(opcode, regField, dst)...)
			} else if src1Idx == rcxIdx {
				code = append(code, emitMovRegToReg64(rcx, src2)...)
				code = append(code, emitShiftOp64(opcode, regField, dst)...)
			} else {
				code = append(code, emitXchgRegReg64(rcx, src2)...)
				code = append(code, emitShiftOp64(opcode, regField, dst)...)
				code = append(code, emitXchgRegReg64(rcx, src2)...)
			}
		}

		return code
	}
}

// Implements “dst = (src1 <cond> src2) ? 1 : 0” in 64-bit registers.
// Implements “dst = (src1 <cond> src2) ? 1 : 0” in 64-bit registers.
func generateSetCondOp64(cc byte) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		reg1, reg2, dstIdx := extractThreeRegs(inst.Args)
		src1 := regInfoList[reg1]
		src2 := regInfoList[reg2]
		dst := regInfoList[dstIdx]

		var code []byte

		// 1) CMP r/m64=src1, r64=src2
		code = append(code, emitCmpReg64Reg64(src1, src2)...)

		// 2) SETcc r/m8 = dst_low
		code = append(code, emitSetccReg8(cc, dst)...)

		// 3) MOVZX r64_dst, r/m8(dst_low)
		//    zero‐extends that byte into the full 64-bit dst
		code = append(code, emitMovzxReg64Reg8(dst, dst)...)

		return code
	}
}

// generateMulUpperOp64 multiplies two 64-bit operands and stores the high 64 bits of the result in dst
// mode: "signed" uses signed multiplication (IMUL), "unsigned" uses unsigned multiplication (MUL)
// mode: "mixed" is treated the same as "unsigned" for now
func generateMulUpperOp64(mode string) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		reg1, reg2, dstIdx := extractThreeRegs(inst.Args)
		src1 := regInfoList[reg1]
		src2 := regInfoList[reg2]
		dst := regInfoList[dstIdx]

		var code []byte

		// Simple strategy: always save/restore RAX and RDX if needed
		saveRAX := dstIdx != 0
		saveRDX := dstIdx != 2

		if saveRAX {
			code = append(code, emitPushReg(RAX)...)
		}
		if saveRDX {
			code = append(code, emitPushReg(RDX)...)
		}

		// Move src1 into RAX: MOV RAX, src1
		code = append(code, emitMovRegToRegWithManualConstruction(RAX, src1)...)

		// Multiply based on mode
		if mode == "signed" {
			code = append(code, emitImulReg64(src2)...)
		} else {
			// For "unsigned" and "mixed", use unsigned MUL
			code = append(code, emitMulReg64(src2)...)
		}

		// Move high result (RDX) to destination if needed
		if dstIdx != 2 {
			code = append(code, emitMovRegToReg64(dst, RDX)...)
		}

		// Restore in reverse order
		if saveRDX {
			code = append(code, emitPopReg(RDX)...)
		}
		if saveRAX {
			code = append(code, emitPopReg(RAX)...)
		}

		return code
	}
}

// generateMovRegToReg generates a 64-bit register-to-register MOV instruction.
// It produces the machine code for `MOV dst, src`.
func generateMovRegToReg(dst, src *X86Reg) []byte {
	return emitMovRegToReg64(*dst, *src)
}

// generateBinaryOp64 creates a function that generates code for a three-operand binary operation.
// commutative: 	ADD (X86_OP_ADD_RM_R), OR (X86_OP_OR_RM_R), AND (X86_OP_AND_RM_R), and XOR (X86_OP_XOR_RM_R)
// non-commutative: SUB (X86_OP_SUB_RM_R)
func generateBinaryOp64(opcode byte) func(inst Instruction) []byte {
	// Determine if the operation is commutative based on its opcode.
	// This avoids breaking the function's public signature.
	isCommutative := false
	switch opcode {
	case X86_OP_ADD_RM_R: // ADD
		isCommutative = true
	case X86_OP_OR_RM_R: // OR
		isCommutative = true
	case X86_OP_AND_RM_R: // AND
		isCommutative = true
	case X86_OP_XOR_RM_R: // XOR
		isCommutative = true
		// Note: SUB (X86_OP_SUB_RM_R) and IMUL (X86_PREFIX_0F X86_OP2_IMUL_R_RM) are not commutative and will correctly
		// fall through to the non-commutative path.
	}

	return func(inst Instruction) []byte {
		src1Idx, src2Idx, dstIdx := extractThreeRegs(inst.Args)
		src1 := regInfoList[src1Idx]
		src2 := regInfoList[src2Idx]
		dst := regInfoList[dstIdx]

		// Handle the most difficult aliasing case: OP dst, src1, dst (e.g., ADD rdx, rcx, rdx).
		// Here, a naive `MOV dst, src1` would overwrite the second source operand.
		if dstIdx == src2Idx && dstIdx != src1Idx {
			// OPTIMIZATION: If the operation is commutative, we can swap the source operands.
			// `OP dst, src1, src2` becomes `OP dst, src2, src1`.
			// This transforms `ADD rdx, rcx, rdx` into `ADD rdx, rdx, rcx`.
			// The subsequent `MOV dst, src1` (now `MOV rdx, rdx`) becomes a no-op and is skipped.
			// This results in a single, hyper-efficient `ADD rdx, rcx` instruction.
			if isCommutative {
				src1, src2 = src2, src1
				src1Idx, src2Idx = src2Idx, src1Idx
			} else {
				// For non-commutative operations, we MUST use a scratch register.
				tmp := BaseReg // Use a designated scratch register.
				var code []byte
				// Save scratch register to the stack
				code = append(code, emitPushReg(tmp)...)

				// 1) MOV tmp, src1
				code = append(code, generateMovRegToReg(&tmp, &src1)...)
				// 2) OP tmp, src2
				opCode := generateOp(opcode, &tmp, &src2)
				code = append(code, opCode...)
				// 3) MOV dst, tmp
				code = append(code, generateMovRegToReg(&dst, &tmp)...)

				// Restore scratch register from the stack
				code = append(code, emitPopReg(tmp)...)
				return code
			}
		}

		// Standard, non-destructive path. This is now also used by the commutative-swap optimization.
		var code []byte
		if dstIdx != src1Idx {
			// Step 1: MOV dst, src1
			code = append(code, generateMovRegToReg(&dst, &src1)...)
		}

		// Step 2: OP dst, src2
		opCode := generateOp(opcode, &dst, &src2)
		code = append(code, opCode...)

		return code
	}
}

// generateOp is a helper to generate the binary operation machine code.
// It handles both standard opcodes and the two-byte IMUL opcode.
func generateOp(opcode byte, reg1, reg2 *X86Reg) []byte {
	// For most binary ops, the encoding is OP r/m64, r64.
	// We perform `OP reg1, reg2` which means the destination is reg1.

	if opcode == X86_PREFIX_0F { // IMUL dst, src => X86_PREFIX_0F X86_OP2_IMUL_R_RM /r
		// For IMUL, the destination register is in the REX.R field.
		return emitImul64(*reg1, *reg2)
	}

	// Use specialized helpers for standard opcodes
	switch opcode {
	case X86_OP_ADD_RM_R: // ADD
		return emitAddReg64(*reg1, *reg2)
	case X86_OP_SUB_RM_R: // SUB
		return emitSubReg64(*reg1, *reg2)
	case X86_OP_AND_RM_R: // AND
		return emitAndReg64(*reg1, *reg2)
	case X86_OP_OR_RM_R: // OR
		return emitOrReg64(*reg1, *reg2)
	case X86_OP_XOR_RM_R: // XOR
		return emitXorReg64(*reg1, *reg2)
	default:
		// Fallback for unknown opcodes - use helper function based on opcode
		switch opcode {
		case X86_OP_XCHG_RM_R: // XCHG
			return emitXchgReg64(*reg1, *reg2)
		default:
			// For other unknown opcodes, we'll need to implement specific helpers
			// This is a safety fallback that shouldn't be reached in normal operation
			rex := buildREX(true, reg2.REXBit == 1, false, reg1.REXBit == 1)
			modrm := byte(X86_MOD_REGISTER<<6 | (reg2.RegBits << 3) | reg1.RegBits)
			return []byte{rex, opcode, modrm}
		}
	}
}

func generateSHLO_L_32() func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		srcAIdx, srcBIdx, dstIdx := extractThreeRegs(inst.Args)
		srcA := regInfoList[srcAIdx]
		srcB := regInfoList[srcBIdx]
		dst := regInfoList[dstIdx]

		var code []byte

		// 1) preserve RCX
		code = append(code, emitPushReg(RCX)...)

		// 2) MOV ECX, r32_srcB
		code = append(code, emitMovEcxFromReg32(srcB)...)

		// 3) MOV r32_dst, r32_srcA  (zero-extends high half)
		code = append(code, emitMovRegReg32(dst, srcA)...)

		// 4) SHL r/m32(dst), CL
		code = append(code, emitShl32ByCl(dst)...)

		// 5) sign-extend the 32-bit result → 64-bit
		code = append(code, emitMovsxd64(dst, dst)...)

		// 6) restore RCX
		code = append(code, emitPopReg(RCX)...)

		return code
	}
}

func generateShiftOp32SHAR() func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		aIdx, bIdx, dIdx := extractThreeRegs(inst.Args)
		srcA := regInfoList[aIdx]
		srcB := regInfoList[bIdx]
		dst := regInfoList[dIdx]

		// Decide if dst aliases srcB → need a scratch reg
		needScratch := (bIdx == dIdx)
		var scratchIdx int
		var scratch X86Reg
		if needScratch {
			for _, cand := range []int{11, 10, 9, 8, 0} {
				if cand != aIdx && cand != bIdx && cand != dIdx && cand != 1 {
					scratchIdx = cand
					scratch = regInfoList[cand]
					break
				}
			}
		}

		// 1) Preserve RCX and scratch if needed
		needRCX := (aIdx != 1 && bIdx != 1 && dIdx != 1)
		needScratchPreserve := needScratch
		var code []byte
		if needScratchPreserve {
			code = append(code, emitPushReg(scratch)...)
		}
		if needRCX {
			code = append(code, emitPushReg(RCX)...)
		}

		// 2) Load shift count into ECX (CL)
		if bIdx != 1 {
			code = append(code, emitMovEcxFromReg32(srcB)...)
		}

		// 3) Mask CL to [0..31] (optional; CPU does this implicitly)
		code = append(code, emitAndRegImm8(RCX, X86_SHIFT_MASK_32)...) // AND ECX, X86_SHIFT_MASK_32

		// 4) Move the 32-bit value into either dst or scratch
		if needScratch {
			if aIdx != scratchIdx {
				code = append(code, emitMovReg32(scratch, srcA)...)
			}
		} else {
			if aIdx != dIdx {
				code = append(code, emitMovReg32(dst, srcA)...)
			}
		}

		// 5) SAR working32, CL
		target := dst
		if needScratch {
			target = scratch
		}
		code = append(code, emitSarReg32ByCl(target)...)

		// 6) MOVSXD dst, working32 (sign-extend 32→64)
		if needScratch {
			code = append(code, emitMovsxd64(dst, scratch)...)
		} else {
			code = append(code, emitMovsxd64(dst, dst)...)
		}

		// 7) Restore RCX and scratch
		if needRCX {
			code = append(code, emitPopReg(RCX)...)
		}
		if needScratchPreserve {
			code = append(code, emitPopReg(scratch)...)
		}

		return code
	}
}

func generateSHLO_R_32() func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		regAIdx, regBIdx, dstIdx := extractThreeRegs(inst.Args)
		srcA := regInfoList[regAIdx]
		srcB := regInfoList[regBIdx]
		dst := regInfoList[dstIdx]

		var code []byte

		// 1) SAVE RCX
		code = append(code, emitPushReg(RCX)...) // PUSH RCX

		// 2) MOV ECX, r32_srcB   ; load shift count into CL without touching srcB
		//    8B /r = MOV r32, r/m32
		//    buildREX(w=false, R=false, X=false, B=srcB.REXBit)
		modrm := byte(X86_MOD_REGISTER<<6 | (1 << 3) | (srcB.RegBits & 7)) // reg=1 (ECX), rm=srcB
		code = append(code,
			buildREX(false, false, false, srcB.REXBit == 1),
			X86_OP_MOV_R_RM, modrm,
		)

		// 3) MOV r32_dst, r32_srcA
		//    89 /r = MOV r/m32, r32
		//    buildREX(w=false, R=srcA.REXBit, X=false, B=dst.REXBit)
		modrm = byte(X86_MOD_REGISTER<<6 | ((srcA.RegBits & 7) << 3) | (dst.RegBits & 7))
		code = append(code,
			buildREX(false, srcA.REXBit == 1, false, dst.REXBit == 1),
			X86_OP_MOV_RM_R, modrm,
		)

		// 4) SHR r32_dst, CL
		//    X86_OP_GROUP2_RM_CL /5 = SHR r/m32, CL
		rex := byte(X86_REX_BASE)
		if dst.REXBit == 1 {
			rex |= X86_REX_B
		} // REX.B = dst
		modrm = byte(X86_MOD_REGISTER<<6 | (5 << 3) | (dst.RegBits & 7))
		code = append(code, rex, X86_OP_GROUP2_RM_CL, modrm)

		// 5) RESTORE RCX
		code = append(code, emitPopReg(RCX)...) // POP RCX

		// 6) MOVSXD r64_dst, r/m32  (sign‑extend 32→64)
		//    X86_OP_MOVSXD /r
		modrm = byte(X86_MOD_REGISTER<<6 | ((dst.RegBits & 7) << 3) | (dst.RegBits & 7))
		code = append(code,
			buildREX(true, dst.REXBit == 1, false, dst.REXBit == 1),
			X86_OP_MOVSXD, modrm,
		)

		return code
	}
}

func generateROT_R_32() func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		opcode := byte(X86_OP_GROUP2_RM_CL)
		regField := byte(1)
		// regA = value, regB = shift, dst = destination
		regAIdx, regBIdx, dstIdx := extractThreeRegs(inst.Args)
		srcA := regInfoList[regAIdx]
		srcB := regInfoList[regBIdx]
		dst := regInfoList[dstIdx]

		// Formal indices for RAX/RCX in regInfoList
		const (
			raxIdx = 0
			rcxIdx = 1
		)

		var code []byte

		// ── Special case: shift register == dst ────────────────────
		if regBIdx == dstIdx {
			// 1) save RCX
			code = append(code, emitPushReg(RCX)...) // PUSH RCX

			// 2) MOV ECX, dst  ; CL = old shift count
			rex := byte(X86_REX_BASE)
			if dst.REXBit == 1 {
				rex |= X86_REX_B
			} // REX.B
			mod := byte(X86_MOD_REGISTER<<6 | (dst.RegBits << 3) | byte(rcxIdx))
			code = append(code, rex, X86_OP_MOV_R_RM, mod) // X86_OP_MOV_R_RM /r = MOV r32, r/m32

			// 3) MOV dst, srcA
			rex = byte(X86_REX_BASE)
			if srcA.REXBit == 1 {
				rex |= X86_REX_R
			} // REX.R
			if dst.REXBit == 1 {
				rex |= X86_REX_B
			} // REX.B
			mod = byte(X86_MOD_REGISTER<<6 | (srcA.RegBits << 3) | dst.RegBits)
			code = append(code, rex, X86_OP_MOV_RM_R, mod) // X86_OP_MOV_RM_R /r = MOV r/m32, r32

			// 4) ROR dst, CL
			rex = byte(X86_REX_BASE)
			if dst.REXBit == 1 {
				rex |= X86_REX_B
			}
			mod = byte(X86_MOD_REGISTER<<6 | (regField << 3) | dst.RegBits)
			code = append(code, rex, opcode, mod)

			// 5) restore RCX
			code = append(code, emitPopReg(RCX)...) // POP RCX

			return code
		}

		// ── Normal case: dst != shift ─────────────────────────────
		// 1) MOV dst, srcA
		rex1 := byte(X86_REX_BASE)
		if srcA.REXBit == 1 {
			rex1 |= X86_REX_R
		}
		if dst.REXBit == 1 {
			rex1 |= X86_REX_B
		}
		m1 := byte(X86_MOD_REGISTER<<6 | (srcA.RegBits << 3) | dst.RegBits)
		code = append(code, rex1, X86_OP_MOV_RM_R, m1)

		// 2) XCHG ECX, srcB      ; put shift into CL
		rexX := byte(X86_REX_BASE)
		if srcB.REXBit == 1 {
			rexX |= X86_REX_R
		}
		mX := byte(X86_MOD_REGISTER<<6 | (srcB.RegBits << 3) | byte(rcxIdx))
		code = append(code, rexX, X86_OP_XCHG_RM_R, mX)

		// 3) ROR dst, CL
		rex2 := byte(X86_REX_BASE)
		if dst.REXBit == 1 {
			rex2 |= X86_REX_B
		}
		m2 := byte(X86_MOD_REGISTER<<6 | (regField << 3) | dst.RegBits)
		code = append(code, rex2, opcode, m2)

		// 4) XCHG ECX, srcB      ; restore original CL/source
		code = append(code, rexX, X86_OP_XCHG_RM_R, mX)

		return code
	}
}

// Implements dst = regA op (regB & 31) without clobbering regB or RCX,
// and for SAR (regField=7) sign‐extends the 32‐bit result into 64 bits.
func generateShiftOp32(regField byte) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		regAIdx, regBIdx, dstIdx := extractThreeRegs(inst.Args)
		srcA := regInfoList[regAIdx]
		srcB := regInfoList[regBIdx]
		dst := regInfoList[dstIdx]
		var code []byte

		// 1) MOV r32_dst, r32_srcA
		code = append(code, emitMovReg32(dst, srcA)...)

		// 2) XCHG ECX, r32_srcB (swap shift count into CL)
		code = append(code, emitXchgReg32Reg32(srcB, RCX)...) // RCX is ECX

		// 3) SHIFT r/m32(dst), CL
		code = append(code, emitShiftReg32ByCl(dst, regField)...)

		// 4) XCHG ECX, r32_srcB (restore ECX and srcB)
		code = append(code, emitXchgReg32Reg32(srcB, RCX)...)

		// 5) If SAR (regField==7), sign-extend 32->64: MOVSXD r64_dst, r/m32(dst)
		if regField == 7 {
			code = append(code, emitMovsxd64(dst, dst)...)
		}

		return code
	}
}

// Implements signed 64-bit division with special cases:
//
//	if b == 0                → dst = maxUint64
//	else if a==MinInt64&&b==-1 → dst = uint64(a)  (no overflow wrap)
//	else                     → dst = uint64(int64(a/b))
func generateDivSOp64(inst Instruction) []byte {
	srcIdx, src2Idx, dstIdx := extractThreeRegs(inst.Args)
	srcInfo := regInfoList[srcIdx]
	src2Info := regInfoList[src2Idx]
	dstInfo := regInfoList[dstIdx]

	code := []byte{}

	// Spill RAX/RDX
	code = append(code, emitPushReg(RAX)...)
	code = append(code, emitPushReg(RDX)...)

	// 1) MOV RAX, src
	code = append(code, emitMovRaxFromRegDivSOp64(srcInfo)...)

	// 2) TEST src2, src2  (b==0?)
	code = append(code, emitTestReg64(src2Info, src2Info)...)

	// 3) JNE div_not_zero
	jneNotZero := len(code)
	code = append(code, X86_PREFIX_0F, X86_OP2_JNE, 0, 0, 0, 0)

	// --- b == 0 path: dst = maxUint64 via XOR/NOT ---
	// XOR r64_dst, r64_dst
	code = append(code, emitXorRegRegDivSOp64(dstInfo)...)
	// NOT r/m64 dst
	code = append(code, emitNotRegDivSOp64(dstInfo)...)

	// 4) JMP end_all
	jmpEndAll := len(code)
	code = append(code, X86_OP_JMP_REL32, 0, 0, 0, 0)

	// --- div_not_zero: a/b, but check MinInt64 / -1 overflow ---
	divNotZeroPos := len(code)
	binary.LittleEndian.PutUint32(code[jneNotZero+2:], uint32(divNotZeroPos-(jneNotZero+6)))

	// 5) MOVABS RDX, MinInt64 (0x8000000000000000)
	code = append(code, emitMovRdxMinInt64DivSOp64()...)

	// 6) CMP RAX, RDX  (check a==MinInt64)
	code = append(code, emitCmpRaxRdxDivSOp64(srcInfo)...)

	// 7) JNE normal_div
	jneNorm1 := len(code)
	code = append(code, X86_PREFIX_0F, X86_OP2_JNE, 0, 0, 0, 0)

	// 8) CMP src2, -1  (b == -1?)
	code = append(code, emitCmpRegNeg1DivSOp64(src2Info)...)

	// 9) JNE normal_div
	jneNorm2 := len(code)
	code = append(code, X86_PREFIX_0F, X86_OP2_JNE, 0, 0, 0, 0)

	// --- overflow path: result = uint64(a) = original RAX ---
	// MOV r/m64, RAX
	code = append(code, emitMovDstFromRaxDivUOp64(dstInfo)...)

	// 10) JMP end_all
	jmpEndAll2 := len(code)
	code = append(code, X86_OP_JMP_REL32, 0, 0, 0, 0)

	// --- normal_div: do CQO; IDIV; MOV dst, RAX ---
	normalDivPos := len(code)
	// patch JNEs
	binary.LittleEndian.PutUint32(code[jneNorm1+2:], uint32(normalDivPos-(jneNorm1+6)))
	binary.LittleEndian.PutUint32(code[jneNorm2+2:], uint32(normalDivPos-(jneNorm2+6)))

	// CQO
	code = append(code, emitCqoDivSOp64()...)
	// IDIV r/m64 = src2
	code = append(code, emitIdivRegDivSOp64(src2Info)...)
	// MOV dst, RAX
	code = append(code, emitMovDstFromRaxDivUOp64(dstInfo)...)

	// patch JMPs to end_all
	endAllPos := len(code)
	binary.LittleEndian.PutUint32(code[jmpEndAll+1:], uint32(endAllPos-(jmpEndAll+5)))
	binary.LittleEndian.PutUint32(code[jmpEndAll2+1:], uint32(endAllPos-(jmpEndAll2+5)))

	// restore RDX, RAX
	code = append(code, emitPopReg(RDX)...)
	code = append(code, emitPopReg(RAX)...)

	return code
}
func generateDivUOp64(inst Instruction) []byte {
	srcIdx1, srcIdx2, dstIdx := extractThreeRegs(inst.Args)
	src1 := regInfoList[srcIdx1]
	src2 := regInfoList[srcIdx2]
	dst := regInfoList[dstIdx]

	code := []byte{}

	// ── spill RAX/RDX on the stack ──
	code = append(code, emitPushReg(RAX)...)
	code = append(code, emitPushReg(RDX)...)

	// 1) MOV RAX, src1
	code = append(code, emitMovRaxFromRegDivUOp64(src1)...)

	// 2) TEST src2, src2
	code = append(code, emitTestReg64(src2, src2)...)

	// 3) JNE to doDiv
	jnePos := len(code)
	code = append(code, X86_PREFIX_0F, X86_OP2_JNE, 0, 0, 0, 0)

	// ── src2 == 0: dst = maxUint64 ──
	// XOR dst,dst
	code = append(code, emitXorRegRegDivUOp64(dst)...)
	// NOT dst
	code = append(code, emitNotRegDivUOp64(dst)...)

	// JMP end
	jmpPos := len(code)
	code = append(code, X86_OP_JMP_REL32, 0, 0, 0, 0)

	// ── doDiv: normal unsigned divide ──
	doDivPos := len(code)
	binary.LittleEndian.PutUint32(code[jnePos+2:], uint32(doDivPos-(jnePos+6)))

	// zero‑extend dividend: XOR RDX,RDX
	code = append(code, emitXorReg64(RDX, RDX)...)

	// DIV r/m64 = src2  (F7 /6)
	code = append(code, emitDivRegDivUOp64(src2)...)

	// MOV dst, RAX
	code = append(code, emitMovDstFromRaxDivUOp64(dst)...)

	// patch JMP end
	endPos := len(code)
	binary.LittleEndian.PutUint32(code[jmpPos+1:], uint32(endPos-(jmpPos+5)))

	// ── restore RDX/RAX from stack ──
	code = append(code, emitPopReg(RDX)...)
	code = append(code, emitPopReg(RAX)...)

	return code
}

func generateRemSOp32(inst Instruction) []byte {
	srcIdx, src2Idx, dstIdx := extractThreeRegs(inst.Args)
	srcInfo := regInfoList[srcIdx]
	src2Info := regInfoList[src2Idx]
	dstInfo := regInfoList[dstIdx]

	code := make([]byte, 0, 96)

	// -- prologue: save RAX, RDX ---------------------------------------------
	code = append(code, emitPushReg(RAX)...)
	code = append(code, emitPushReg(RDX)...)

	// 1) MOV    EAX, src32
	code = append(code, emitMovReg32ToReg32(X86Reg{RegBits: 0, REXBit: 0}, srcInfo)...)

	// 2) TEST   src2, src2        ; check divisor==0
	code = append(code, emitTestReg32(src2Info, src2Info)...)

	// 3) JE     zeroDiv
	jeZeroOff := len(code)
	code = append(code, emitJeRel32()...)

	// 4) CMP    EAX, 0x80000000    ; detect INT32_MIN
	code = append(code, emitCmpRegImm32MinInt(X86Reg{RegBits: 0, REXBit: 0})...)

	// 5) JNE    doDiv             ; normal path if not INT32_MIN
	jneDivOff := len(code)
	code = append(code, emitJne32()...)

	// 6) CMP    src2, -1          ; divisor == -1 ?
	code = append(code, emitCmpRegImmByte(src2Info, X86_NEG_ONE)...)

	// 7) JNE    doDiv             ; if not -1, go doDiv
	jneDivOff2 := len(code)
	code = append(code, emitJne32()...)

	// --- overflow case: INT32_MIN % -1 → remainder = 0 -----
	// 8) XOR    EAX, EAX          ; clear EAX to 0
	code = append(code, emitXorEaxEax()...)

	// 9) MOVSXD dst, EAX         ; sign‐extend 0 into dst
	code = append(code, emitMovsxd64(dstInfo, X86Reg{RegBits: 0, REXBit: 0})...)

	// 10) JMP    end
	jmpOff := len(code)
	code = append(code, emitJmp32()...)

	// --- doDiv path -----------------------------------------------
	doDiv := len(code)
	// 11) CDQ                   ; sign‐extend EAX → EDX:EAX
	code = append(code, emitCdq()...)
	// 12) IDIV   src2 - handle src2 conflict with RDX
	isSrc2RDX := src2Idx == 2
	if isSrc2RDX {
		// If divisor is RDX, use the spilled original RDX from [RSP]
		code = append(code, emitIdivMemStackRemSOp32RDX()...)
	} else {
		code = append(code, emitIdiv32(src2Info)...)
	}
	// 13) MOVSXD dst, EDX      ; move remainder into dst
	code = append(code, emitMovsxd64(dstInfo, X86Reg{RegBits: 2, REXBit: 0})...)
	// 14) JMP    end
	jmpOff2 := len(code)
	code = append(code, emitJmp32()...)

	// --- zeroDiv label: divisor=0 ----------------------------------
	zeroDiv := len(code)
	// 15) MOVSXD dst, EAX      ; remainder = dividend
	code = append(code, emitMovsxd64(dstInfo, X86Reg{RegBits: 0, REXBit: 0})...)

	// --- end label -------------------------------------------------
	end := len(code)
	// restore RDX, RAX
	code = append(code, emitPopRdxRax()...)

	// ─── patch all the jumps ───────────────────────────────────────────────────
	// JE zeroDiv
	binary.LittleEndian.PutUint32(code[jeZeroOff+2:], uint32(zeroDiv-(jeZeroOff+6)))
	// JNE doDiv (first)
	binary.LittleEndian.PutUint32(code[jneDivOff+2:], uint32(doDiv-(jneDivOff+6)))
	// JNE doDiv (second)
	binary.LittleEndian.PutUint32(code[jneDivOff2+2:], uint32(doDiv-(jneDivOff2+6)))
	// JMP end (overflow path)
	binary.LittleEndian.PutUint32(code[jmpOff+1:], uint32(end-(jmpOff+5)))
	// JMP end (doDiv path)
	binary.LittleEndian.PutUint32(code[jmpOff2+1:], uint32(end-(jmpOff2+5)))

	return code
}

// Implements: if (r64_srcB == 0) r64_dst = r64_srcA
// Uses TEST to set ZF, then CMOVE (X86_OP2_CMOVE) on that flag.
func generateCmovOp64(cc byte) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		srcIdxA, srcIdxB, dstIdx := extractThreeRegs(inst.Args)
		srcA := regInfoList[srcIdxA]
		srcB := regInfoList[srcIdxB]
		dst := regInfoList[dstIdx]

		// 1) TEST r64_srcB, r64_srcB
		code := emitTestReg64(srcB, srcB)

		// 2) CMOVcc r64_dst, r64_srcA
		code = append(code, emitCmov64(cc, dst, srcA)...)

		return code
	}
}
