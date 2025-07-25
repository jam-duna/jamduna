package pvm

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
		code = append(code, emitPushReg(regInfoList[0])...) // PUSH RAX

		//    MOV RAX, srcA
		code = append(code, emitMovRegToReg64(regInfoList[0], srcA)...)
	}

	// 1) MOV dst, srcB        ; dst ← B
	code = append(code, emitMovRegToReg64(dst, srcB)...)

	// 2) NOT dst              ; dst = ^B
	code = append(code, emitNotReg64(dst)...)

	// 3) OR dst, src          ; dst = (~B) | X
	if conflict {
		//    OR dst, RAX       ; use our saved original A
		code = append(code, emitOrReg64(dst, regInfoList[0])...)

		//    restore RAX
		code = append(code, emitPopReg(regInfoList[0])...) // POP RAX
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
		buf = append(buf, 0x50) // PUSH RAX
	}
	if !isDstRDX {
		buf = append(buf, 0x52) // PUSH RDX
	}

	// 1) MOV dividend into RAX (if src1 != RAX)
	if srcIdx != 0 {
		rex := byte(0x48) // REX.W
		if src1.REXBit == 1 {
			rex |= 0x01 // REX.B
		}
		buf = append(buf,
			rex, 0x8B,
			byte(0xC0|(0<<3)|src1.RegBits),
		)
	}

	// 2) TEST divisor != 0
	rex := byte(0x48)
	// REX.R for src2 in reg field, REX.B for rm
	if src2.REXBit == 1 {
		rex |= 0x04 | 0x01
	}
	buf = append(buf,
		rex, 0x85,
		byte(0xC0|(src2.RegBits<<3)|src2.RegBits),
	)

	// JE zeroDiv placeholder
	jeOff := len(buf)
	buf = append(buf, 0x0F, 0x84, 0, 0, 0, 0)

	// 3) Normal path: clear RDX, DIV, move remainder
	// XOR RDX, RDX
	rex = 0x48
	if regInfoList[2].REXBit == 1 {
		rex |= 0x04 | 0x01 // REX.R+REX.B for RDX
	}
	buf = append(buf, rex, 0x31, byte(0xC0|(2<<3)|2))
	isSrc2RAX := src2Idx == 0
	isSrc2RDX := src2Idx == 2

	if isSrc2RAX {
		// If divisor is RAX, use the spilled original RAX from [RSP+8]
		buf = append(buf,
			0x48,             // REX.W
			0xF7, 0xB4, 0x24, // F7 /6 rm=4 (SIB follows)
			0x08, 0x00, 0x00, 0x00, // disp32 = 8
		)
	} else if isSrc2RDX {
		// If divisor is RDX, use the spilled original RDX from [RSP]
		buf = append(buf,
			0x48,       // REX.W
			0xF7, 0x34, // F7 /6 rm=4 (SIB follows)
			0x24, // SIB: scale=0,index=RSP,base=RSP
		)
	} else {
		rex = 0x48
		if src2.REXBit == 1 {
			rex |= 0x01
		}
		buf = append(buf,
			rex, 0xF7,
			byte(0xC0|(6<<3)|src2.RegBits),
		)
	}

	// Move remainder into dst
	if isDstRAX {
		// XCHG RAX, RDX => remainder ends up in RAX
		buf = append(buf, 0x48, 0x87, 0xD0)
	} else if !isDstRDX {
		// MOV dst, RDX
		rex = 0x48
		if regInfoList[2].REXBit == 1 {
			rex |= 0x04 // REX.R
		}
		if dst.REXBit == 1 {
			rex |= 0x01 // REX.B
		}
		buf = append(buf,
			rex, 0x89,
			byte(0xC0|(2<<3)|dst.RegBits),
		)
	}

	// 4) JMP end placeholder
	jmpOff := len(buf)
	buf = append(buf, 0xE9, 0, 0, 0, 0)

	// zeroDiv: divisor was zero => dst = dividend (in RAX)
	zeroOff := len(buf)
	if !isDstRAX {
		rex = 0x48
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		buf = append(buf,
			rex, 0x89,
			byte(0xC0|(0<<3)|dst.RegBits),
		)
	}

	// end: restore if spilled
	endOff := len(buf)
	if !isDstRDX {
		buf = append(buf, 0x5A) // POP RDX
	}
	if !isDstRAX {
		buf = append(buf, 0x58) // POP RAX
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
		buf = append(buf, 0x50) // PUSH RAX
	}
	if !isDstRDX {
		buf = append(buf, 0x52) // PUSH RDX
	}

	// MOV RAX, src1
	rex := byte(0x48)
	if srcInfo.REXBit == 1 {
		rex |= 0x01
	}
	buf = append(buf, rex, 0x8B, byte(0xC0|(0<<3)|srcInfo.RegBits))

	// TEST src2, src2 (b==0?)
	rex = 0x48
	if src2Info.REXBit == 1 {
		rex |= 0x04 | 0x01
	}
	buf = append(buf, rex, 0x85, byte(0xC0|(src2Info.RegBits<<3)|src2Info.RegBits))

	// JE zeroDiv
	jeZeroOff := len(buf)
	buf = append(buf, 0x0F, 0x84, 0, 0, 0, 0)

	// Overflow check a==INT64_MIN
	// MOV RDX, 0x8000000000000000
	buf = append(buf,
		0x48, 0xBA,
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x80,
		// CMP RAX, RDX
		0x48, 0x39, byte(0xC0|(2<<3)|0),
	)
	// JNE doRem
	jneOff := len(buf)
	buf = append(buf, 0x0F, 0x85, 0, 0, 0, 0)

	// Overflow check b==-1
	if isSrc2RDX {
		// CMP [RSP], imm32(-1)
		buf = append(buf,
			0x48,       // REX.W
			0x81, 0x3C, // 81 /7 r/m64, imm32; mod=00, reg=7, rm=4 => 0x3C
			0x24,                   // SIB: scale=0,index=RSP,base=RSP
			0xFF, 0xFF, 0xFF, 0xFF, // imm32 = -1
		)
	} else {
		// CMP RDX, imm32(-1)
		rex = 0x48
		if src2Info.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | (7 << 3) | src2Info.RegBits)
		buf = append(buf, rex, 0x81, modrm, 0xFF, 0xFF, 0xFF, 0xFF)
	}
	// JE overflowCase
	jeOvfOff := len(buf)
	buf = append(buf, 0x0F, 0x84, 0, 0, 0, 0)

	// doRem: CQO + IDIV
	doRemOff := len(buf)
	buf = append(buf, 0x48, 0x99) // CQO

	if isSrc2RDX {
		// IDIV qword ptr [RSP]
		buf = append(buf,
			0x48,       // REX.W
			0xF7, 0x3C, // F7 /7 rm=4 (SIB)
			0x24, // SIB: scale=0,index=RSP,base=RSP
		)
	} else {
		rex = 0x48
		if src2Info.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | (7 << 3) | src2Info.RegBits)
		buf = append(buf, rex, 0xF7, modrm)
	}

	// Move remainder to dst
	if isDstRAX {
		buf = append(buf, 0x48, 0x87, 0xD0) // XCHG RAX,RDX
	} else if !isDstRDX {
		// MOV dst, RDX
		rex = 0x48
		if dstInfo.REXBit == 1 {
			rex |= 0x01
		}
		buf = append(buf, rex, 0x89, byte(0xC0|(2<<3)|dstInfo.RegBits))
	}

	// JMP end
	jmpEndOff := len(buf)
	buf = append(buf, 0xE9, 0, 0, 0, 0)

	// zeroDiv: b==0 ⇒ result=a (in RAX)
	zeroOff := len(buf)
	if !isDstRAX {
		rex = 0x48
		if dstInfo.REXBit == 1 {
			rex |= 0x01
		}
		buf = append(buf, rex, 0x89, byte(0xC0|(0<<3)|dstInfo.RegBits))
	}
	buf = append(buf, 0xE9, 0, 0, 0, 0)
	jmpZeroEnd := len(buf) - 4

	// overflowCase: result = 0
	ovfOff := len(buf)
	rex = 0x48
	if dstInfo.REXBit == 1 {
		rex |= 0x05
	}
	buf = append(buf, rex, 0x31, byte(0xC0|(dstInfo.RegBits<<3)|dstInfo.RegBits))
	buf = append(buf, 0xE9, 0, 0, 0, 0)
	jmpOvfEnd := len(buf) - 4

	// end: restore
	endOff := len(buf)
	if !isDstRDX {
		buf = append(buf, 0x5A) // POP RDX
	}
	if !isDstRAX {
		buf = append(buf, 0x58) // POP RAX
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

	code := []byte{0x50, 0x52} // push rax; push rdx

	// 1) MOV EAX, src32
	rex := byte(0x40)
	if srcInfo.REXBit == 1 {
		rex |= 0x01
	}
	modrm := byte(0xC0 | (0 << 3) | srcInfo.RegBits)
	code = append(code, rex, 0x8B, modrm)

	// 2) TEST src2, src2
	rex = 0x40
	if src2Info.REXBit == 1 {
		rex |= 0x05
	} // REX.R|REX.B for high regs
	modrm = byte(0xC0 | (src2Info.RegBits << 3) | src2Info.RegBits)
	code = append(code, rex, 0x85, modrm)

	// 3) JNE doDiv
	jneOff := len(code)
	code = append(code, 0x0F, 0x85, 0, 0, 0, 0)

	// 4) divisor==0 → MOVSXD dst, EAX  (sign‐extend low32→full64)
	rex = 0x48 // REX.W
	if dstInfo.REXBit == 1 {
		rex |= 0x04
	} // REX.R for dst
	modrm = byte(0xC0 | (dstInfo.RegBits << 3)) // rm=0 => EAX
	code = append(code, rex, 0x63, modrm)       // 0x63=/r MOVSXD r64, r/m32
	// 5) JMP end
	jmpOff := len(code)
	code = append(code, 0xE9, 0, 0, 0, 0)

	// doDiv:
	doDiv := len(code)

	//    a) XOR EDX, EDX
	code = append(code, 0x40, 0x31, byte(0xC0|(2<<3)|2))

	//    b) DIV r/m32
	rex = byte(0x40)
	if src2Info.REXBit == 1 {
		rex |= 0x01
	}
	modrm = byte(0xC0 | (6 << 3) | src2Info.RegBits)
	code = append(code, rex, 0xF7, modrm)

	//    c) MOV dst, EDX
	rex = byte(0x40)
	if dstInfo.REXBit == 1 {
		rex |= 0x01
	}
	modrm = byte(0xC0 | (2 << 3) | dstInfo.RegBits)
	code = append(code, rex, 0x89, modrm)

	// patch JNE→doDiv and JMP→end, restore, etc...
	// patch JNE → doDiv
	end := len(code)
	binary.LittleEndian.PutUint32(code[jneOff+2:], uint32(doDiv-(jneOff+6)))
	// patch JMP → end
	binary.LittleEndian.PutUint32(code[jmpOff+1:], uint32(end-(jmpOff+5)))

	// Restore
	code = append(code, 0x5A, 0x58)
	return code
}

// Implements DIV unsigned: r64_dst = uint64(r32_src1 / r32_src2),
// but if (src2 & 0xFFFFFFFF)==0 then dst = 0xFFFFFFFFFFFFFFFF.
// Preserves RAX and RDX.
func generateDivUOp32(inst Instruction) []byte {
	srcIdx1, srcIdx2, dstIdx := extractThreeRegs(inst.Args)
	src1Info := regInfoList[srcIdx1]
	src2Info := regInfoList[srcIdx2]
	dstInfo := regInfoList[dstIdx]

	code := []byte{
		0x50, // push rax
		0x52, // push rdx
	}

	// 1) MOV EAX, src1
	rex := byte(0x40)
	if src1Info.REXBit == 1 {
		rex |= 0x01
	}
	code = append(code, rex, 0x8B, byte(0xC0|(0<<3)|src1Info.RegBits))

	// 2) TEST src2, src2
	rex = 0x40
	if src2Info.REXBit == 1 {
		rex |= 0x05
	} // REX.R+REX.B for high regs
	code = append(code, rex, 0x85, byte(0xC0|(src2Info.RegBits<<3)|src2Info.RegBits))

	// 3) JNE doDiv
	jneOff := len(code)
	code = append(code, 0x0F, 0x85, 0, 0, 0, 0) // placeholder

	// --- divisor == 0 path: produce maxUint64 via XOR/NOT ---

	// XOR r64_dst, r64_dst  → zero
	rex = 0x48
	if dstInfo.REXBit == 1 {
		rex |= 0x05
	} // REX.W + REX.R+REX.B
	code = append(code, rex, 0x31, byte(0xC0|(dstInfo.RegBits<<3)|dstInfo.RegBits))

	// NOT r/m64, r64  → invert to all ones
	rex = 0x48
	if dstInfo.REXBit == 1 {
		rex |= 0x01
	} // only REX.B for rm
	code = append(code, rex, 0xF7, byte(0xC0|(2<<3)|dstInfo.RegBits))

	// JMP end
	jmpOff := len(code)
	code = append(code, 0xE9, 0, 0, 0, 0)

	// --- doDiv: normal unsigned divide ---

	doDiv := len(code)

	// XOR EDX, EDX
	code = append(code, 0x40, 0x31, byte(0xC0|(2<<3)|2))

	// DIV r/m32 = src2
	rex = 0x40
	if src2Info.REXBit == 1 {
		rex |= 0x01
	}
	code = append(code, rex, 0xF7, byte(0xC0|(6<<3)|src2Info.RegBits))

	// MOV r64_dst, EAX (zero-extends quotient)
	rex = 0x48
	if dstInfo.REXBit == 1 {
		rex |= 0x01
	}
	code = append(code, rex, 0x89, byte(0xC0|(0<<3)|dstInfo.RegBits))

	// --- patch jumps ---
	end := len(code)
	binary.LittleEndian.PutUint32(code[jneOff+2:], uint32(doDiv-(jneOff+6)))
	binary.LittleEndian.PutUint32(code[jmpOff+1:], uint32(end-(jmpOff+5)))

	// restore RDX, RAX
	code = append(code, 0x5A, 0x58)

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
	code = append(code, emitPushReg(regInfoList[0])...) // PUSH RAX
	code = append(code, emitPushReg(regInfoList[2])...) // PUSH RDX

	// 1) MOV EAX, src
	code = append(code, emitMovEaxFromReg32(srcInfo)...)

	// 2) TEST src2, src2  (b==0?)
	code = append(code, emitTestRegReg32(src2Info)...)

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

	// 5) CMP EAX, 0x80000000  (a == MinInt32?)
	code = append(code, emitCmpRegImm32MinInt(X86Reg{RegBits: 0, REXBit: 0})...) // EAX

	// 6) JNE normal_div
	jneOvf1 := len(code)
	code = append(code, emitJne32()...)

	// 7) CMP src2, -1       (b == -1?)
	code = append(code, emitCmpRegImmByte(src2Info, 0xFF)...)

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

	// IDIV r/m32 = src2
	code = append(code, emitIdiv32(src2Info)...)

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
		tmp := regInfoList[0] // RAX

		// 0) PUSH RAX
		code = append(code, emitPushReg64(tmp)...)

		// 1) MOV EAX, r32_src2
		code = append(code, emitMovReg32(tmp, src2)...)

		// 2) MOV r32_dst, r/m32 src1
		code = append(code, emitMovReg32(dst, src1)...)

		// 3) IMUL r32_dst, r32_tmp
		code = append(code, emitImulReg32(dst, tmp)...)

		// 4) MOVSXD r64_dst, r32_dst  ; sign-extend low 32 bits into 64
		code = append(code, emitMovsxd64(dst, dst)...)

		// 5) POP RAX
		code = append(code, emitPopReg64(tmp)...)
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
			// 1) push rax
			code = append(code, emitPushRax()...)

			// 2) mov eax, src2d
			code = append(code, emitMovEaxFromReg32(src2)...)

			// 3) mov dst32, src1d
			code = append(code, emitMovRegReg32(dst, src1)...)

			// 4) dst32 = dst32 <op> eax
			code = append(code, emitBinaryOpRegEax32(opcode, dst)...)

			// 5) sign‑extend into 64 bits
			code = append(code, emitMovsxd64(dst, dst)...)

			// 6) pop rax
			code = append(code, emitPopRax()...)
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
		code = append(code, 0x51)

		// 2) MOV RCX, src2  -> CL = shift count
		{
			// MOV r64_reg, r/m64 = 8B /r
			rex := byte(0x48)
			// REX.R = reg field = 1 (RCX); no REX.R needed since RCX is low 3 bits
			if src2.REXBit == 1 {
				rex |= 0x01 // REX.B because src2 might be r8+
			}
			modrm := byte(0xC0 | (1 << 3) | src2.RegBits) // reg=1, rm=src2
			code = append(code, rex, 0x8B, modrm)
		}

		// 3) MOV dst, src1
		{
			rex := byte(0x48)
			if src1.REXBit == 1 {
				rex |= 0x04
			} // REX.R
			if dst.REXBit == 1 {
				rex |= 0x01
			} // REX.B
			modrm := byte(0xC0 | (src1.RegBits << 3) | dst.RegBits)
			code = append(code, rex, 0x89, modrm)
		}

		// 4) SHIFT dst by CL
		{
			rex := byte(0x48)
			if dst.REXBit == 1 {
				rex |= 0x01
			} // REX.B
			modrm := byte(0xC0 | (regField << 3) | dst.RegBits)
			code = append(code, rex, opcode, modrm)
		}

		// 5) POP RCX
		code = append(code, 0x59)

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
			code = append(code, 0x51)

			// mov rcx, r64_b
			//   REX.W=1, REX.B=b.REXBit
			code = append(code,
				buildREX(true, false, false, b.REXBit == 1),
				0x8B,                        // MOV r64_reg, r64_rm
				byte(0xC0|(1<<3)|b.RegBits), // reg=1 (RCX), rm=b (FIXED)
			)
		}

		// ─── 2) COPY valueA → dst ───
		if src1Idx != dstIdx {
			// MOV r/m64: 0x89 /r
			code = append(code,
				buildREX(true, a.REXBit == 1, false, dst.REXBit == 1),
				0x89,
				byte(0xC0|(a.RegBits<<3)|dst.RegBits),
			)
		}

		// ─── 3) ROL dst, CL ───
		// Opcode: D3 /0 = ROL r/m64, CL
		code = append(code,
			buildREX(true, false, false, dst.REXBit == 1),
			0xD3,
			byte(0xC0|(0<<3)|dst.RegBits), // /0 = ROL, rm=dst
		)

		// ─── 4) RESTORE RCX ───
		if src2Idx != 1 && dstIdx != 1 {
			code = append(code, 0x59) // pop rcx
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
			opcode   = byte(0xD3) // D3 /4 = SHL r/m64, CL
			regField = byte(4)    // /4 = SHL
		)

		src1Idx, src2Idx, dstIdx := extractThreeRegs(inst.Args)
		src1 := regInfoList[src1Idx]
		src2 := regInfoList[src2Idx]
		dst := regInfoList[dstIdx]

		var code []byte

		emitMOV := func(srcReg, dstReg X86Reg) {
			code = append(code, emitMovRegToReg64(dstReg, srcReg)...)
		}

		emitSHL := func(target X86Reg) {
			code = append(code, emitShiftOp64(opcode, regField, target)...)
		}

		emitXCHG := func(a, b X86Reg) {
			code = append(code, emitXchgRegReg64(a, b)...)
		}

		emitPUSH := func(r X86Reg) {
			code = append(code, emitPushReg64(r)...)
		}

		emitPOP := func(r X86Reg) {
			code = append(code, emitPopReg64(r)...)
		}

		emitMOVByteToCL := func(src X86Reg) {
			code = append(code, emitMovByteToCL(src)...)
		}

		// Constants for RCX/RAX indices
		const (
			raxIdx = 0
			rcxIdx = 1
		)
		rcx := regInfoList[rcxIdx]
		rax := regInfoList[raxIdx]

		// Case 1: shift==dst
		if src2Idx == dstIdx {
			if dstIdx == rcxIdx {
				// shift==dst==RCX
				emitPUSH(rax)        // save RAX
				emitMOV(src2, rax)   // RAX := old shift
				emitMOV(src1, dst)   // RCX := src
				emitMOVByteToCL(rax) // CL := low byte of old shift
				emitSHL(dst)         // SHL RCX, CL
				emitPOP(rax)         // restore RAX
			} else {
				// shift==dst!=RCX
				emitPUSH(rcx)      // save RCX
				emitMOV(src2, rcx) // RCX := old shift
				emitMOV(src1, dst) // dst := src
				emitSHL(dst)       // SHL dst, CL
				emitPOP(rcx)       // restore RCX
			}

			// Case 2: dst==RCX!=shift
		} else if dstIdx == rcxIdx {
			if src2Idx == raxIdx {
				// shift in RAX
				emitPUSH(rax)
				emitMOV(src1, rcx)
				emitMOV(src2, rax)
				emitXCHG(rax, rcx)
				emitSHL(rax)
				emitMOV(rax, rcx)
				emitPOP(rax)
			} else {
				// shift elsewhere
				emitPUSH(rax)
				emitMOV(src1, rax)
				emitMOV(src2, rcx)
				emitSHL(rax)
				emitMOV(rax, rcx)
				emitPOP(rax)
			}

			// Case 3: dst!=RCX && shift!=dst
		} else {
			emitMOV(src1, dst)
			if src2Idx == rcxIdx {
				emitSHL(dst)
			} else if src1Idx == rcxIdx {
				emitMOV(src2, rcx)
				emitSHL(dst)
			} else {
				emitXCHG(rcx, src2)
				emitSHL(dst)
				emitXCHG(rcx, src2)
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
			code = append(code, 0x50) // PUSH RAX
		}
		if saveRDX {
			code = append(code, 0x52) // PUSH RDX
		}

		// Move src1 into RAX: MOV RAX, src1
		code = append(code, emitMovRegToRegWithManualConstruction(regInfoList[0], src1)...)

		// Multiply based on mode
		if mode == "signed" {
			code = append(code, emitImulReg64(src2)...)
		} else {
			// For "unsigned" and "mixed", use unsigned MUL
			code = append(code, emitMulReg64(src2)...)
		}

		// Move high result (RDX) to destination if needed
		if dstIdx != 2 {
			code = append(code, emitMovRegToReg64(dst, regInfoList[2])...)
		}

		// Restore in reverse order
		if saveRDX {
			code = append(code, 0x5A) // POP RDX
		}
		if saveRAX {
			code = append(code, 0x58) // POP RAX
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
// commutative: 	ADD (0x01), OR (0x09), AND (0x21), and XOR (0x31)
// non-commutative: SUB (0x29)
func generateBinaryOp64(opcode byte) func(inst Instruction) []byte {
	// Determine if the operation is commutative based on its opcode.
	// This avoids breaking the function's public signature.
	isCommutative := false
	switch opcode {
	case 0x01: // ADD
		isCommutative = true
	case 0x09: // OR
		isCommutative = true
	case 0x21: // AND
		isCommutative = true
	case 0x31: // XOR
		isCommutative = true
		// Note: SUB (0x29) and IMUL (0x0F AF) are not commutative and will correctly
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

	if opcode == 0x0F { // IMUL dst, src => 0F AF /r
		// For IMUL, the destination register is in the REX.R field.
		return emitImul64(*reg1, *reg2)
	}

	// Use specialized helpers for standard opcodes
	switch opcode {
	case 0x01: // ADD
		return emitAddReg64(*reg1, *reg2)
	case 0x29: // SUB
		return emitSubReg64(*reg1, *reg2)
	case 0x21: // AND
		return emitAndReg64(*reg1, *reg2)
	case 0x09: // OR
		return emitOrReg64(*reg1, *reg2)
	case 0x31: // XOR
		return emitXorReg64(*reg1, *reg2)
	default:
		// Fallback for unknown opcodes - use helper function based on opcode
		switch opcode {
		case 0x87: // XCHG
			return emitXchgReg64(*reg1, *reg2)
		default:
			// For other unknown opcodes, we'll need to implement specific helpers
			// This is a safety fallback that shouldn't be reached in normal operation
			rex := buildREX(true, reg2.REXBit == 1, false, reg1.REXBit == 1)
			modrm := byte(0xC0 | (reg2.RegBits << 3) | reg1.RegBits)
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
		code = append(code, emitPushRcx()...)

		// 2) MOV ECX, r32_srcB
		code = append(code, emitMovEcxFromReg32(srcB)...)

		// 3) MOV r32_dst, r32_srcA  (zero-extends high half)
		code = append(code, emitMovRegReg32(dst, srcA)...)

		// 4) SHL r/m32(dst), CL
		code = append(code, emitShl32ByCl(dst)...)

		// 5) sign-extend the 32-bit result → 64-bit
		code = append(code, emitMovsxd64(dst, dst)...)

		// 6) restore RCX
		code = append(code, emitPopRcx()...)

		return code
	}
}

func generateShiftOp32SHAR(opcode byte, regField byte) func(inst Instruction) []byte {
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
			code = append(code, emitPushRcx()...)
		}

		// 2) Load shift count into ECX (CL)
		if bIdx != 1 {
			code = append(code, emitMovEcxFromReg32(srcB)...)
		}

		// 3) Mask CL to [0..31] (optional; CPU does this implicitly)
		code = append(code, emitAndRegImm8(regInfoList[1], 0x1F)...) // AND ECX, 0x1F

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
			code = append(code, emitMovsxdReg64Reg32(dst, scratch)...)
		} else {
			code = append(code, emitMovsxdReg64Reg32(dst, dst)...)
		}

		// 7) Restore RCX and scratch
		if needRCX {
			code = append(code, emitPopRcx()...)
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
		code = append(code, 0x51) // PUSH RCX

		// 2) MOV ECX, r32_srcB   ; load shift count into CL without touching srcB
		//    8B /r = MOV r32, r/m32
		//    buildREX(w=false, R=false, X=false, B=srcB.REXBit)
		modrm := byte(0xC0 | (1 << 3) | (srcB.RegBits & 7)) // reg=1 (ECX), rm=srcB
		code = append(code,
			buildREX(false, false, false, srcB.REXBit == 1),
			0x8B, modrm,
		)

		// 3) MOV r32_dst, r32_srcA
		//    89 /r = MOV r/m32, r32
		//    buildREX(w=false, R=srcA.REXBit, X=false, B=dst.REXBit)
		modrm = byte(0xC0 | ((srcA.RegBits & 7) << 3) | (dst.RegBits & 7))
		code = append(code,
			buildREX(false, srcA.REXBit == 1, false, dst.REXBit == 1),
			0x89, modrm,
		)

		// 4) SHR r32_dst, CL
		//    D3 /5 = SHR r/m32, CL
		rex := byte(0x40)
		if dst.REXBit == 1 {
			rex |= 0x01
		} // REX.B = dst
		modrm = byte(0xC0 | (5 << 3) | (dst.RegBits & 7))
		code = append(code, rex, 0xD3, modrm)

		// 5) RESTORE RCX
		code = append(code, 0x59) // POP RCX

		// 6) MOVSXD r64_dst, r/m32  (sign‑extend 32→64)
		//    63 /r
		modrm = byte(0xC0 | ((dst.RegBits & 7) << 3) | (dst.RegBits & 7))
		code = append(code,
			buildREX(true, dst.REXBit == 1, false, dst.REXBit == 1),
			0x63, modrm,
		)

		return code
	}
}

func generateROT_R_32() func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		opcode := byte(0xD3)
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
			code = append(code, 0x51) // PUSH RCX

			// 2) MOV ECX, dst  ; CL = old shift count
			rex := byte(0x40)
			if dst.REXBit == 1 {
				rex |= 0x01
			} // REX.B
			mod := byte(0xC0 | (dst.RegBits << 3) | byte(rcxIdx))
			code = append(code, rex, 0x8B, mod) // 8B /r = MOV r32, r/m32

			// 3) MOV dst, srcA
			rex = byte(0x40)
			if srcA.REXBit == 1 {
				rex |= 0x04
			} // REX.R
			if dst.REXBit == 1 {
				rex |= 0x01
			} // REX.B
			mod = byte(0xC0 | (srcA.RegBits << 3) | dst.RegBits)
			code = append(code, rex, 0x89, mod) // 89 /r = MOV r/m32, r32

			// 4) ROR dst, CL
			rex = byte(0x40)
			if dst.REXBit == 1 {
				rex |= 0x01
			}
			mod = byte(0xC0 | (regField << 3) | dst.RegBits)
			code = append(code, rex, opcode, mod)

			// 5) restore RCX
			code = append(code, 0x59) // POP RCX

			return code
		}

		// ── Normal case: dst != shift ─────────────────────────────
		// 1) MOV dst, srcA
		rex1 := byte(0x40)
		if srcA.REXBit == 1 {
			rex1 |= 0x04
		}
		if dst.REXBit == 1 {
			rex1 |= 0x01
		}
		m1 := byte(0xC0 | (srcA.RegBits << 3) | dst.RegBits)
		code = append(code, rex1, 0x89, m1)

		// 2) XCHG ECX, srcB      ; put shift into CL
		rexX := byte(0x40)
		if srcB.REXBit == 1 {
			rexX |= 0x04
		}
		mX := byte(0xC0 | (srcB.RegBits << 3) | byte(rcxIdx))
		code = append(code, rexX, 0x87, mX)

		// 3) ROR dst, CL
		rex2 := byte(0x40)
		if dst.REXBit == 1 {
			rex2 |= 0x01
		}
		m2 := byte(0xC0 | (regField << 3) | dst.RegBits)
		code = append(code, rex2, opcode, m2)

		// 4) XCHG ECX, srcB      ; restore original CL/source
		code = append(code, rexX, 0x87, mX)

		return code
	}
}

// Implements dst = regA op (regB & 31) without clobbering regB or RCX,
// and for SAR (regField=7) sign‐extends the 32‐bit result into 64 bits.
func generateShiftOp32(opcode byte, regField byte) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		regAIdx, regBIdx, dstIdx := extractThreeRegs(inst.Args)
		srcA := regInfoList[regAIdx]
		srcB := regInfoList[regBIdx]
		dst := regInfoList[dstIdx]
		var code []byte

		// 1) MOV r32_dst, r32_srcA
		code = append(code, emitMovReg32(dst, srcA)...)

		// 2) XCHG ECX, r32_srcB (swap shift count into CL)
		code = append(code, emitXchgReg32Reg32(srcB, regInfoList[1])...) // regInfoList[1] is ECX

		// 3) SHIFT r/m32(dst), CL
		code = append(code, emitShiftReg32ByCl(dst, regField)...)

		// 4) XCHG ECX, r32_srcB (restore ECX and srcB)
		code = append(code, emitXchgReg32Reg32(srcB, regInfoList[1])...)

		// 5) If SAR (regField==7), sign-extend 32->64: MOVSXD r64_dst, r/m32(dst)
		if regField == 7 {
			code = append(code, emitMovsxdReg64Reg32(dst, dst)...)
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

	code := []byte{
		0x50, // push rax
		0x52, // push rdx
	}

	// 1) MOV RAX, src
	rex := byte(0x48)
	if srcInfo.REXBit == 1 {
		rex |= 0x01
	} // REX.B for rm=src
	code = append(code, rex, 0x8B, byte(0xC0|(0<<3)|srcInfo.RegBits))

	// 2) TEST src2, src2  (b==0?)
	rex = 0x48
	if src2Info.REXBit == 1 {
		rex |= 0x05
	} // REX.R+REX.B for reg=rm=src2
	code = append(code, rex, 0x85, byte(0xC0|(src2Info.RegBits<<3)|src2Info.RegBits))

	// 3) JNE div_not_zero
	jneNotZero := len(code)
	code = append(code, 0x0F, 0x85, 0, 0, 0, 0)

	// --- b == 0 path: dst = maxUint64 via XOR/NOT ---
	// XOR r64_dst, r64_dst
	rex = 0x48
	if dstInfo.REXBit == 1 {
		rex |= 0x05
	}
	code = append(code, rex, 0x31, byte(0xC0|(dstInfo.RegBits<<3)|dstInfo.RegBits))
	// NOT r/m64 dst
	rex = 0x48
	if dstInfo.REXBit == 1 {
		rex |= 0x01
	}
	code = append(code, rex, 0xF7, byte(0xC0|(2<<3)|dstInfo.RegBits))

	// 4) JMP end_all
	jmpEndAll := len(code)
	code = append(code, 0xE9, 0, 0, 0, 0)

	// --- div_not_zero: a/b, but check MinInt64 / -1 overflow ---
	divNotZeroPos := len(code)
	binary.LittleEndian.PutUint32(code[jneNotZero+2:], uint32(divNotZeroPos-(jneNotZero+6)))

	// 5) MOVABS RDX, MinInt64 (0x8000000000000000)
	//    opcode: REX.W + (B8+rd=RDX) + imm64
	code = append(code,
		0x48,                   // REX.W
		0xBA,                   // B8+2 for RDX
		0x00, 0x00, 0x00, 0x00, // low 32 bits
		0x00, 0x00, 0x00, 0x80, // high 32 bits = 0x80
	)

	// 6) CMP RAX, RDX  (check a==MinInt64)
	//    opcode: REX.W + 39 /r
	rex = byte(0x48)
	if srcInfo.REXBit == 1 {
		rex |= 0x01
	} // REX.B for rm=RAX
	// RDX in reg field, RAX in rm
	code = append(code, rex, 0x39,
		byte(0xC0|(srcInfo.RegBits)|(2<<3)), // mod=11, reg=2(RDX), rm=0(RAX)
	)

	// 7) JNE normal_div
	jneNorm1 := len(code)
	code = append(code, 0x0F, 0x85, 0, 0, 0, 0)

	// 8) CMP src2, -1  (b == -1?)
	//    opcode: REX.W + 83 /7 ib
	rex = 0x48
	if src2Info.REXBit == 1 {
		rex |= 0x01
	}
	code = append(code, rex, 0x83,
		byte(0xC0|(7<<3)|src2Info.RegBits), // mod=11, reg=7(CMP), rm=src2
		0xFF,                               // imm8 = -1
	)

	// 9) JNE normal_div
	jneNorm2 := len(code)
	code = append(code, 0x0F, 0x85, 0, 0, 0, 0)

	// --- overflow path: result = uint64(a) = original RAX ---
	// MOV r/m64, RAX
	rex = 0x48
	if dstInfo.REXBit == 1 {
		rex |= 0x01
	}
	code = append(code, rex, 0x89, byte(0xC0|(0<<3)|dstInfo.RegBits))

	// 10) JMP end_all
	jmpEndAll2 := len(code)
	code = append(code, 0xE9, 0, 0, 0, 0)

	// --- normal_div: do CQO; IDIV; MOV dst, RAX ---
	normalDivPos := len(code)
	// patch JNEs
	binary.LittleEndian.PutUint32(code[jneNorm1+2:], uint32(normalDivPos-(jneNorm1+6)))
	binary.LittleEndian.PutUint32(code[jneNorm2+2:], uint32(normalDivPos-(jneNorm2+6)))

	// CQO
	code = append(code, 0x48, 0x99)
	// IDIV r/m64 = src2
	rex = 0x48
	if src2Info.REXBit == 1 {
		rex |= 0x01
	}
	code = append(code, rex, 0xF7, byte(0xC0|(7<<3)|src2Info.RegBits))
	// MOV dst, RAX
	rex = 0x48
	if dstInfo.REXBit == 1 {
		rex |= 0x01
	}
	code = append(code, rex, 0x89, byte(0xC0|(0<<3)|dstInfo.RegBits))

	// patch JMPs to end_all
	endAllPos := len(code)
	binary.LittleEndian.PutUint32(code[jmpEndAll+1:], uint32(endAllPos-(jmpEndAll+5)))
	binary.LittleEndian.PutUint32(code[jmpEndAll2+1:], uint32(endAllPos-(jmpEndAll2+5)))

	// restore RDX, RAX
	code = append(code, 0x5A, 0x58)

	return code
}
func generateDivUOp64(inst Instruction) []byte {
	srcIdx1, srcIdx2, dstIdx := extractThreeRegs(inst.Args)
	src1 := regInfoList[srcIdx1]
	src2 := regInfoList[srcIdx2]
	dst := regInfoList[dstIdx]

	code := []byte{}

	// ── spill RAX/RDX on the stack ──
	code = append(code,
		0x50, // push rax
		0x52, // push rdx
	)

	// 1) MOV RAX, src1
	rex := byte(0x48)
	if src1.REXBit == 1 {
		rex |= 0x01
	} // REX.B for src1
	code = append(code, rex, 0x8B, byte(0xC0|src1.RegBits))

	// 2) TEST src2, src2
	rex = 0x48
	if src2.REXBit == 1 {
		rex |= 0x05
	} // REX.R + REX.B
	code = append(code, rex, 0x85, byte(0xC0|(src2.RegBits<<3)|src2.RegBits))

	// 3) JNE to doDiv
	jnePos := len(code)
	code = append(code, 0x0F, 0x85, 0, 0, 0, 0)

	// ── src2 == 0: dst = maxUint64 ──
	// XOR dst,dst
	rex = 0x48
	if dst.REXBit == 1 {
		rex |= 0x05
	} // REX.R + REX.B
	code = append(code, rex, 0x31, byte(0xC0|(dst.RegBits<<3)|dst.RegBits))
	// NOT dst
	rex = 0x48
	if dst.REXBit == 1 {
		rex |= 0x01
	} // REX.B
	code = append(code, rex, 0xF7, byte(0xC0|(2<<3)|dst.RegBits))

	// JMP end
	jmpPos := len(code)
	code = append(code, 0xE9, 0, 0, 0, 0)

	// ── doDiv: normal unsigned divide ──
	doDivPos := len(code)
	binary.LittleEndian.PutUint32(code[jnePos+2:], uint32(doDivPos-(jnePos+6)))

	// zero‑extend dividend: XOR RDX,RDX
	code = append(code, 0x48, 0x31, 0xD2)

	// DIV r/m64 = src2  (F7 /6)
	rex = 0x48
	if src2.REXBit == 1 {
		rex |= 0x01
	}
	code = append(code, rex, 0xF7, byte(0xC0|(6<<3)|src2.RegBits))

	// MOV dst, RAX
	rex = 0x48
	if dst.REXBit == 1 {
		rex |= 0x01
	}
	code = append(code, rex, 0x89, byte(0xC0|(0<<3)|dst.RegBits))

	// patch JMP end
	endPos := len(code)
	binary.LittleEndian.PutUint32(code[jmpPos+1:], uint32(endPos-(jmpPos+5)))

	// ── restore RDX/RAX from stack ──
	code = append(code,
		0x5A, // pop rdx
		0x58, // pop rax
	)

	return code
}

func generateRemSOp32(inst Instruction) []byte {
	srcIdx, src2Idx, dstIdx := extractThreeRegs(inst.Args)
	srcInfo := regInfoList[srcIdx]
	src2Info := regInfoList[src2Idx]
	dstInfo := regInfoList[dstIdx]

	code := make([]byte, 0, 96)

	// -- prologue: save RAX, RDX ---------------------------------------------
	code = append(code, emitPushRaxRdx()...)

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
	code = append(code, emitCmpRegImmByte(src2Info, 0xFF)...)

	// 7) JNE    doDiv             ; if not -1, go doDiv
	jneDivOff2 := len(code)
	code = append(code, emitJne32()...)

	// --- overflow case: INT32_MIN % -1 → remainder = 0 -----
	// 8) XOR    EAX, EAX          ; clear EAX to 0
	code = append(code, emitXorEaxEax()...)

	// 9) MOVSXD dst, EAX         ; sign‐extend 0 into dst
	code = append(code, emitMovsxdReg64(dstInfo, X86Reg{RegBits: 0, REXBit: 0})...)

	// 10) JMP    end
	jmpOff := len(code)
	code = append(code, emitJmp32()...)

	// --- doDiv path -----------------------------------------------
	doDiv := len(code)
	// 11) CDQ                   ; sign‐extend EAX → EDX:EAX
	code = append(code, emitCdq()...)
	// 12) IDIV   src2
	code = append(code, emitIdiv32(src2Info)...)
	// 13) MOVSXD dst, EDX      ; move remainder into dst
	code = append(code, emitMovsxdReg64(dstInfo, X86Reg{RegBits: 2, REXBit: 0})...)
	// 14) JMP    end
	jmpOff2 := len(code)
	code = append(code, emitJmp32()...)

	// --- zeroDiv label: divisor=0 ----------------------------------
	zeroDiv := len(code)
	// 15) MOVSXD dst, EAX      ; remainder = dividend
	code = append(code, emitMovsxdReg64(dstInfo, X86Reg{RegBits: 0, REXBit: 0})...)

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
// Uses TEST to set ZF, then CMOVE (0x44) on that flag.
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
