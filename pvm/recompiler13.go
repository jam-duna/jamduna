package pvm

import (
	"encoding/binary"
)

func generateXnorOp64(inst Instruction) []byte {
	reg1, reg2, dst := extractThreeRegs(inst.Args)
	src1 := regInfoList[reg1]
	src2 := regInfoList[reg2]
	dstReg := regInfoList[dst]

	code := []byte{
		0x48 | (src1.REXBit<<2 | dstReg.REXBit), 0x89, 0xC0 | (src1.RegBits << 3) | dstReg.RegBits,
		0x48 | (dstReg.REXBit | src2.REXBit<<2), 0x31, 0xC0 | (src2.RegBits << 3) | dstReg.RegBits,
		0x48 | (dstReg.REXBit << 0), 0xF7, 0xD0 | dstReg.RegBits,
	}
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
		code = append(code, 0x50) // PUSH RAX

		//    MOV RAX, srcA
		//    opcode: 0x89 /r  (MOV r/m64, r64)
		//    dest=r/m=RAX (low3=0), src=srcA
		rex := byte(0x48) // REX.W
		if srcA.REXBit != 0 {
			rex |= 0x04
		} // REX.R = 1 if srcA ≥ 8
		// no REX.B because dest is RAX (index 0)
		modrm := byte(0xC0 | ((srcA.RegBits & 7) << 3))
		code = append(code, rex, 0x89, modrm)
	}

	// 1) MOV dst, srcB        ; dst ← B
	{
		rex := byte(0x48) // REX.W
		if srcB.REXBit != 0 {
			rex |= 0x04
		} // REX.R = srcB
		if dst.REXBit != 0 {
			rex |= 0x01
		} // REX.B = dst
		modrm := byte(0xC0 | ((srcB.RegBits & 7) << 3) | (dst.RegBits & 7))
		code = append(code, rex, 0x89, modrm)
	}

	// 2) NOT dst              ; dst = ^B
	{
		rex := byte(0x48) // REX.W
		if dst.REXBit != 0 {
			rex |= 0x01
		} // REX.B = dst
		modrm := byte(0xC0 | (2 << 3) | (dst.RegBits & 7)) // /2 = NOT
		code = append(code, rex, 0xF7, modrm)
	}

	// 3) OR dst, src          ; dst = (~B) | X
	if conflict {
		//    OR dst, RAX       ; use our saved original A
		rex := byte(0x48) // REX.W
		// REX.R=0 (RAX is reg field=0), REX.B if dst ≥ 8
		if dst.REXBit != 0 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | ((0) << 3) | (dst.RegBits & 7))
		code = append(code, rex, 0x09, modrm)

		//    restore RAX
		code = append(code, 0x58) // POP RAX
	} else {
		//    OR dst, srcA      ; original non-conflict path
		rex := byte(0x48) // REX.W
		if srcA.REXBit != 0 {
			rex |= 0x04
		} // REX.R = srcA
		if dst.REXBit != 0 {
			rex |= 0x01
		} // REX.B = dst
		modrm := byte(0xC0 | ((srcA.RegBits & 7) << 3) | (dst.RegBits & 7))
		code = append(code, rex, 0x09, modrm)
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
		code = append(code, 0x41, 0x54) // PUSH R12
	} else {
		panic("Unsupported BaseReg for push")
	}

	// 2) MOV tmp, src2
	rexMov := byte(0x48)
	if src2.REXBit == 1 {
		rexMov |= 0x04 // REX.R
	}
	if tmp.REXBit == 1 {
		rexMov |= 0x01 // REX.B
	}
	modrmMov := byte(0xC0 | (src2.RegBits << 3) | tmp.RegBits)
	code = append(code, rexMov, 0x89, modrmMov)

	// 3) NOT tmp
	rexNot := byte(0x48)
	if tmp.REXBit == 1 {
		rexNot |= 0x01
	}
	code = append(code, rexNot, 0xF7, 0xD0|tmp.RegBits)

	// 4) AND tmp, src1
	rexAnd := byte(0x48)
	if tmp.REXBit == 1 {
		rexAnd |= 0x01
	}
	if src1.REXBit == 1 {
		rexAnd |= 0x04
	}
	modrmAnd := byte(0xC0 | (src1.RegBits << 3) | tmp.RegBits)
	code = append(code, rexAnd, 0x21, modrmAnd)

	// 5) MOV dst, tmp
	rexDst := byte(0x48)
	if tmp.REXBit == 1 {
		rexDst |= 0x04
	}
	if dst.REXBit == 1 {
		rexDst |= 0x01
	}
	modrmDst := byte(0xC0 | (tmp.RegBits << 3) | dst.RegBits)
	code = append(code, rexDst, 0x89, modrmDst)

	// 6) POP tmp to restore
	if tmp.Name == "r12" {
		code = append(code, 0x41, 0x5C) // POP R12
	} else {
		panic("Unsupported BaseReg for pop")
	}

	return code
}

// Implements REM signed 64-bit: r64_dst = sign_extend(r64_src % r64_src2)

// rem signed 64-bit with overflow case ⇒ result=0
func generateRemSOp64(inst Instruction) []byte {
	srcIdx, src2Idx, dstIdx := extractThreeRegs(inst.Args)
	srcInfo := regInfoList[srcIdx]
	src2Info := regInfoList[src2Idx]
	dstInfo := regInfoList[dstIdx]

	code := []byte{}

	// 0) save RAX, RDX
	code = append(code, 0x50, 0x52) // push rax; push rdx

	// 1) MOV RAX, r64_src
	rex := byte(0x48) // REX.W
	if srcInfo.REXBit == 1 {
		rex |= 0x01
	} // REX.B for rm=src
	modrm := byte(0xC0 | (0 << 3) | srcInfo.RegBits) // reg=0 (RAX), rm=src
	code = append(code, rex, 0x8B, modrm)

	// 2) TEST r64_src2, r64_src2  (to detect divisor=0)
	rex = 0x48
	if src2Info.REXBit == 1 {
		rex |= 0x04 | 0x01
	} // REX.R & REX.B, since reg=src2, rm=src2
	modrm = byte(0xC0 | (src2Info.RegBits << 3) | src2Info.RegBits)
	code = append(code, rex, 0x85, modrm) // 85 /r = TEST r/m64, r64

	// 3) JE zeroDiv
	jeZeroOff := len(code)
	code = append(code, 0x0F, 0x84, 0, 0, 0, 0) // placeholder for JE to zeroDiv

	// 4) Check overflow: if RAX == MinInt64
	//    MOV RDX, 0x8000000000000000
	code = append(code,
		0x48, 0xBA, // REX.W + MOV RDX, imm64
		0x00, 0x00, 0x00, 0x00, // low dword = 0
		0x00, 0x00, 0x00, 0x80, // high dword = 0x80000000
	)
	//    CMP RAX, RDX
	code = append(code,
		0x48, 0x39, // REX.W + CMP r/m64, r64
		byte(0xC0|(2<<3)), // mod=11, reg=2=RDX, rm=0=RAX
	)
	//    JNE checkNegOne
	jneNeg1Off := len(code)
	code = append(code, 0x0F, 0x85, 0, 0, 0, 0)

	// 5) Check divisor == -1  (signed overflow pair)
	//    CMP r64_src2, imm32(-1)
	rex = 0x48
	if src2Info.REXBit == 1 {
		rex |= 0x01
	} // REX.B for rm=src2
	modrm = byte(0xC0 | (7 << 3) | src2Info.RegBits) // /7= CMP
	code = append(code, rex, 0x81, modrm)
	code = append(code, 0xFF, 0xFF, 0xFF, 0xFF) // imm32 = -1

	//    JE overflowCase
	jeOvfOff := len(code)
	code = append(code, 0x0F, 0x84, 0, 0, 0, 0)

	// --- normal rem path: do CQO/IDIV ---

	// label doRem:
	doRemOff := len(code)

	// a) CQO
	code = append(code, 0x48, 0x99)

	// b) IDIV r/m64 (src2)
	rex = 0x48
	if src2Info.REXBit == 1 {
		rex |= 0x01
	} // REX.B
	modrm = byte(0xC0 | (7 << 3) | src2Info.RegBits) // /7 = IDIV
	code = append(code, rex, 0xF7, modrm)

	// c) MOVSXD dst, RDX
	rex = 0x48
	if dstInfo.REXBit == 1 {
		rex |= 0x04
	} // REX.R for reg=dst
	modrm = byte(0xC0 | (dstInfo.RegBits << 3) | 0x2) // rm=2=RDX
	code = append(code, rex, 0x63, modrm)

	// 6) JMP end
	jmpEndOff := len(code)
	code = append(code, 0xE9, 0, 0, 0, 0)

	// --- zeroDiv label ---
	zeroDivOff := len(code)

	// MOV dst, RAX  -> copy full 64-bit valueA into dst
	rex = 0x48
	if dstInfo.REXBit == 1 {
		rex |= 0x01
	} // REX.B for rm=dst
	modrm = byte(0xC0 | (0 << 3) | dstInfo.RegBits) // reg=0 (RAX), rm=dst
	code = append(code, rex, 0x89, modrm)

	// JMP end
	code = append(code, 0xE9, 0, 0, 0, 0)
	jmpZeroEndOff := len(code) - 4

	// --- overflowCase label ---
	ovfOff := len(code)
	// XOR dst, dst  -> result = 0
	rex = 0x48
	if dstInfo.REXBit == 1 {
		// need REX.R (for reg) _and_ REX.B (for rm) to target the same high register
		rex |= 0x05 // 0x04 | 0x01
	}
	modrm = byte(0xC0 | (dstInfo.RegBits << 3) | dstInfo.RegBits)
	code = append(code, rex, 0x31, modrm)

	// JMP end
	code = append(code, 0xE9, 0, 0, 0, 0)
	jmpOvfEndOff := len(code) - 4

	// --- end label ---
	endOff := len(code)
	// restore RDX, RAX
	code = append(code, 0x5A, 0x58)

	// Patch JE zeroDiv
	binary.LittleEndian.PutUint32(code[jeZeroOff+2:], uint32(zeroDivOff-(jeZeroOff+6)))
	// Patch JNE to checkNegOne
	binary.LittleEndian.PutUint32(code[jneNeg1Off+2:], uint32(doRemOff-(jneNeg1Off+6)))
	// Patch JE overflowCase
	binary.LittleEndian.PutUint32(code[jeOvfOff+2:], uint32(ovfOff-(jeOvfOff+6)))
	// Patch JMP end (normal path)
	binary.LittleEndian.PutUint32(code[jmpEndOff+1:], uint32(endOff-(jmpEndOff+5)))
	// Patch JMP zeroDiv→end
	binary.LittleEndian.PutUint32(code[jmpZeroEndOff:], uint32(endOff-(jmpZeroEndOff+4)))
	// Patch JMP ovf→end
	binary.LittleEndian.PutUint32(code[jmpOvfEndOff:], uint32(endOff-(jmpOvfEndOff+4)))

	return code
}

// rem unsigned 64-bit with B==0 ⇒ result=A
func generateRemUOp64(inst Instruction) []byte {
	srcIdx, src2Idx, dstIdx := extractThreeRegs(inst.Args)
	srcInfo := regInfoList[srcIdx]
	src2Info := regInfoList[src2Idx]
	dstInfo := regInfoList[dstIdx]

	code := []byte{}

	// Save RAX, RDX
	code = append(code, 0x50, 0x52) // push rax; push rdx

	// 1) MOV RAX, r64_src
	rex := byte(0x48) // REX.W
	if srcInfo.REXBit == 1 {
		rex |= 0x01 // REX.B for rm = src
	}
	modrm := byte(0xC0 | (0 << 3) | srcInfo.RegBits) // reg=0 (RAX), rm=src
	code = append(code, rex, 0x8B, modrm)

	// 2) TEST r64_src2, r64_src2  (check divisor==0?)
	rex = 0x48 // REX.W
	if src2Info.REXBit == 1 {
		rex |= 0x05 // REX.R (0x04) + REX.B (0x01)
	}
	modrm = byte(0xC0 | (src2Info.RegBits << 3) | src2Info.RegBits)
	code = append(code, rex, 0x85, modrm) // 85 /r = TEST r/m64, r64

	// 3) JNE doDiv
	jneOff := len(code)
	code = append(code, 0x0F, 0x85, 0, 0, 0, 0) // placeholder

	// 4) divisor==0 path: MOV dst, RAX
	rex = 0x48
	if dstInfo.REXBit == 1 {
		rex |= 0x01
	}
	modrm = byte(0xC0 | (0 << 3) | dstInfo.RegBits)
	code = append(code, rex, 0x89, modrm) // 89 /r = MOV r/m64, r64

	// 5) JMP end
	jmpOff := len(code)
	code = append(code, 0xE9, 0, 0, 0, 0)

	// label doDiv:
	doDiv := len(code)

	// a) XOR RDX, RDX (zero-extend dividend)
	code = append(code, 0x48, 0x31, byte(0xC0|(2<<3)|2)) // 31 /r = XOR r/m64, r64

	// b) DIV r/m64
	rex = 0x48
	if src2Info.REXBit == 1 {
		rex |= 0x01
	}
	modrm = byte(0xC0 | (6 << 3) | src2Info.RegBits) // /6 = DIV
	code = append(code, rex, 0xF7, modrm)

	// c) MOV dst, RDX
	rex = 0x48
	if dstInfo.REXBit == 1 {
		rex |= 0x01
	}
	modrm = byte(0xC0 | (2 << 3) | dstInfo.RegBits) // reg=2 RDX
	code = append(code, rex, 0x89, modrm)

	// label end:
	end := len(code)
	// patch JNE→doDiv
	binary.LittleEndian.PutUint32(code[jneOff+2:], uint32(doDiv-(jneOff+6)))
	// patch JMP→end
	binary.LittleEndian.PutUint32(code[jmpOff+1:], uint32(end-(jmpOff+5)))

	// Restore RDX, RAX
	code = append(code, 0x5A, 0x58) // pop rdx; pop rax

	return code
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

	code := []byte{
		0x50, // push rax
		0x52, // push rdx
	}

	// 1) MOV EAX, src
	rex := byte(0x40)
	if srcInfo.REXBit == 1 {
		rex |= 0x01
	} // REX.B for rm=src
	code = append(code, rex, 0x8B, byte(0xC0|(0<<3)|srcInfo.RegBits))

	// 2) TEST src2, src2  (b==0?)
	rex = 0x40
	if src2Info.REXBit == 1 {
		rex |= 0x05
	} // REX.R+REX.B
	code = append(code, rex, 0x85, byte(0xC0|(src2Info.RegBits<<3)|src2Info.RegBits))

	// 3) JNE div_not_zero
	jneDiv := len(code)
	code = append(code, 0x0F, 0x85, 0, 0, 0, 0)

	// --- b == 0: dst = maxUint64 via XOR/NOT ---
	rex = 0x48
	if dstInfo.REXBit == 1 {
		rex |= 0x05
	}
	code = append(code, rex, 0x31, byte(0xC0|(dstInfo.RegBits<<3)|dstInfo.RegBits))
	rex = 0x48
	if dstInfo.REXBit == 1 {
		rex |= 0x01
	}
	code = append(code, rex, 0xF7, byte(0xC0|(2<<3)|dstInfo.RegBits))

	// 4) JMP end
	jmpEnd := len(code)
	code = append(code, 0xE9, 0, 0, 0, 0)

	// fix up div_not_zero
	divPos := len(code)
	binary.LittleEndian.PutUint32(code[jneDiv+2:], uint32(divPos-(jneDiv+6)))

	// 5) CMP EAX, 0x80000000  (a == MinInt32?)
	rex = 0x40
	code = append(code, rex, 0x81, byte(0xC0|(7<<3)), // 81 /7 id
		0x00, 0x00, 0x00, 0x80)

	// 6) JNE normal_div
	jneOvf1 := len(code)
	code = append(code, 0x0F, 0x85, 0, 0, 0, 0)

	// 7) CMP src2, -1       (b == -1?)
	rex = 0x40
	if src2Info.REXBit == 1 {
		rex |= 0x01
	}
	code = append(code, rex, 0x83, byte(0xC0|(7<<3)|src2Info.RegBits), 0xFF)

	// 8) JNE normal_div
	jneOvf2 := len(code)
	code = append(code, 0x0F, 0x85, 0, 0, 0, 0)

	// --- overflow path: dst = uint64(a) via MOVSXD dst, EAX ---
	rex = 0x48
	if dstInfo.REXBit == 1 {
		rex |= 0x04
	} // REX.W+REX.R
	code = append(code, rex, 0x63, byte(0xC0|(dstInfo.RegBits<<3)))

	// 9) JMP end
	jmpOvfEnd := len(code)
	code = append(code, 0xE9, 0, 0, 0, 0)

	// fix up overflow jumps
	normPos := len(code)
	binary.LittleEndian.PutUint32(code[jneOvf1+2:], uint32(normPos-(jneOvf1+6)))
	binary.LittleEndian.PutUint32(code[jneOvf2+2:], uint32(normPos-(jneOvf2+6)))

	// --- normal_div: CDQ; IDIV; MOVSXD dst, EAX ---

	// CDQ (sign-extend EAX → EDX)
	code = append(code, 0x99)

	// IDIV r/m32 = src2
	rex = 0x40
	if src2Info.REXBit == 1 {
		rex |= 0x01
	}
	code = append(code, rex, 0xF7, byte(0xC0|(7<<3)|src2Info.RegBits))

	// MOVSXD dst, EAX
	rex = 0x48
	if dstInfo.REXBit == 1 {
		rex |= 0x04
	}
	code = append(code, rex, 0x63, byte(0xC0|(dstInfo.RegBits<<3)))

	// patch end jumps
	endPos := len(code)
	binary.LittleEndian.PutUint32(code[jmpEnd+1:], uint32(endPos-(jmpEnd+5)))
	binary.LittleEndian.PutUint32(code[jmpOvfEnd+1:], uint32(endPos-(jmpOvfEnd+5)))

	// -- Restore RDX, RAX --
	code = append(code, 0x5A, 0x58)

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

	// Helper to build ModR/M
	modrm := func(reg, rm byte) byte {
		return 0xC0 | ((reg & 7) << 3) | (rm & 7)
	}

	// ─── Handle conflict (dst == src2) by spilling src2 into RAX ───
	if dstIdx == srcIdx2 {
		tmp := regInfoList[0] // RAX

		// 0) PUSH RAX
		code = append(code, 0x50)

		// 1) MOV EAX, r32_src2
		//    8B /r   → MOV r32 (reg), r/m32 (rm)
		//    reg field = tmp, rm field = src2
		code = append(code,
			emitRex(false, tmp.REXBit == 1, false, src2.REXBit == 1),
			0x8B,
			modrm(tmp.RegBits, src2.RegBits),
		)

		// 2) MOV r32_dst, r/m32 src1
		//    8B /r   → MOV r32 (dest), r/m32 (src)
		code = append(code,
			emitRex(false, dst.REXBit == 1, false, src1.REXBit == 1),
			0x8B,
			modrm(dst.RegBits, src1.RegBits),
		)

		// 3) IMUL r32_dst, r32_tmp
		//    0F AF /r → IMUL r32 (reg=dst), r/m32 (rm=tmp)
		code = append(code,
			emitRex(false, dst.REXBit == 1, false, tmp.REXBit == 1),
			0x0F, 0xAF,
			modrm(dst.RegBits, tmp.RegBits),
		)

		// 4) MOVSXD r64_dst, r32_dst  ; sign-extend low 32 bits into 64
		//    63 /r → MOVSXD r64 (reg=dst), r/m32 (rm=dst)
		code = append(code,
			emitRex(true, dst.REXBit == 1, false, dst.REXBit == 1),
			0x63,
			modrm(dst.RegBits, dst.RegBits),
		)

		// 5) POP RAX
		code = append(code, 0x58)
		return code
	}

	// ─── No conflict: dst != src2 ───

	// 1) MOV r32_dst, r/m32 src1
	code = append(code,
		emitRex(false, dst.REXBit == 1, false, src1.REXBit == 1),
		0x8B,
		modrm(dst.RegBits, src1.RegBits),
	)

	// 2) IMUL r32_dst, r32_src2
	code = append(code,
		emitRex(false, dst.REXBit == 1, false, src2.REXBit == 1),
		0x0F, 0xAF,
		modrm(dst.RegBits, src2.RegBits),
	)

	// 3) MOVSXD r64_dst, r32_dst
	code = append(code,
		emitRex(true, dst.REXBit == 1, false, dst.REXBit == 1),
		0x63,
		modrm(dst.RegBits, dst.RegBits),
	)

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
		if scratch.REXBit == 1 {
			code = append(code, 0x41)
		}
		code = append(code, 0x50|scratch.RegBits)

		// MOV scratch, src2
		rex := byte(0x48)
		if src2.REXBit == 1 {
			rex |= 0x04
		}
		if scratch.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | (src2.RegBits << 3) | scratch.RegBits)
		code = append(code, rex, 0x89, modrm)

		// Treat scratch as the new src2
		src2 = scratch
	}

	// MOV dst, src1
	rexMov := byte(0x48)
	if src1.REXBit == 1 {
		rexMov |= 0x04
	}
	if dst.REXBit == 1 {
		rexMov |= 0x01
	}
	modrmMov := byte(0xC0 | (src1.RegBits << 3) | dst.RegBits)
	code = append(code, rexMov, 0x89, modrmMov)

	// IMUL dst, src2
	rexMul := byte(0x48)
	if dst.REXBit == 1 {
		rexMul |= 0x04
	}
	if src2.REXBit == 1 {
		rexMul |= 0x01
	}
	modrmMul := byte(0xC0 | (dst.RegBits << 3) | src2.RegBits)
	code = append(code, rexMul, 0x0F, 0xAF, modrmMul)

	// Restore scratch if used
	if dstIdx == src2Idx && dstIdx != src1Idx {
		// pop scratch
		if scratch.REXBit == 1 {
			code = append(code, 0x41)
		}
		code = append(code, 0x58|scratch.RegBits)
	}

	return code
}

// emitRex builds a REX prefix byte from its four flag bits.
func emitRex(w, r, x, b bool) byte {
	var rex byte = 0x40
	if w {
		rex |= 0x08
	} // REX.W
	if r {
		rex |= 0x04
	} // REX.R
	if x {
		rex |= 0x02
	} // REX.X (unused)
	if b {
		rex |= 0x01
	} // REX.B
	return rex
}

func generateBinaryOp32(opcode byte) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		r1, r2, rd := extractThreeRegs(inst.Args)
		src1 := regInfoList[r1]
		src2 := regInfoList[r2]
		dst := regInfoList[rd]

		var code []byte

		// ─── CASE A: conflict dst==src2 ───
		if r2 == rd {
			// 1) swap dst<->src1 in 1 instr
			code = append(code,
				emitRex(false, src1.REXBit == 1, false, dst.REXBit == 1),
				0x87, // XCHG r/m32, r32
				0xC0|(src1.RegBits<<3)|dst.RegBits,
			)
			// 2) dst32 = dst32 <op> src1d
			code = append(code,
				emitRex(false, src1.REXBit == 1, false, dst.REXBit == 1),
				opcode,
				0xC0|(src1.RegBits<<3)|dst.RegBits,
			)
			// 3) sign‐extend into 64b
			code = append(code,
				emitRex(true, dst.REXBit == 1, false, dst.REXBit == 1),
				0x63, // MOVSXD r64,r/m32
				0xC0|(dst.RegBits<<3)|dst.RegBits,
			)
			return code
		}

		// ─── CASE B: dst==src1 ───
		if r1 == rd {
			// 1) dst32 = dst32 <op> src2d
			code = append(code,
				emitRex(false, src2.REXBit == 1, false, dst.REXBit == 1),
				opcode,
				0xC0|(src2.RegBits<<3)|dst.RegBits,
			)
			// 2) sign‐extend
			code = append(code,
				emitRex(true, dst.REXBit == 1, false, dst.REXBit == 1),
				0x63,
				0xC0|(dst.RegBits<<3)|dst.RegBits,
			)
			return code
		}

		// ─── CASE C: no conflict ───
		// 1) MOV dst32, src1d
		code = append(code,
			emitRex(false, src1.REXBit == 1, false, dst.REXBit == 1),
			0x89, // MOV r/m32, r32
			0xC0|(src1.RegBits<<3)|dst.RegBits,
		)
		// 2) dst32 = dst32 <op> src2d
		code = append(code,
			emitRex(false, src2.REXBit == 1, false, dst.REXBit == 1),
			opcode,
			0xC0|(src2.RegBits<<3)|dst.RegBits,
		)
		// 3) MOVSXD dst64, dst32
		code = append(code,
			emitRex(true, dst.REXBit == 1, false, dst.REXBit == 1),
			0x63,
			0xC0|(dst.RegBits<<3)|dst.RegBits,
		)
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

// Implements: r64_dst = int64(r64_src1) >> (r64_src2 & 63),
// without leaving RCX or src2 trashed.
func generateShiftOp64(opcode byte, regField byte) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		src1Idx, src2Idx, dstIdx := extractThreeRegs(inst.Args)
		src1 := regInfoList[src1Idx]
		src2 := regInfoList[src2Idx]
		dst := regInfoList[dstIdx]

		var code []byte

		// 1) MOV r64_dst, r64_src1
		rex1 := byte(0x48) // REX.W
		if src1.REXBit == 1 {
			rex1 |= 0x04
		} // REX.R
		if dst.REXBit == 1 {
			rex1 |= 0x01
		} // REX.B
		modrm1 := byte(0xC0 | (src1.RegBits << 3) | dst.RegBits)
		code = append(code, rex1, 0x89, modrm1)

		// 2) XCHG RCX, r64_src2  (save/restore CL)
		rexX := byte(0x48) // REX.W
		if src2.REXBit == 1 {
			rexX |= 0x04
		} // REX.R for reg field = src2
		// r/m field = RCX.RegBits == 1 (no REX.B needed for rm=1)
		modrmX := byte(0xC0 | (src2.RegBits << 3) | 0x01)
		code = append(code, rexX, 0x87, modrmX) // 87 /r = XCHG r/m64, r64

		// 3) D3 /n RCX, CL -> shift dst by CL (in RCX low 8 bits)
		rex2 := byte(0x48)
		if dst.REXBit == 1 {
			rex2 |= 0x01
		} // REX.B for rm=dst
		modrm2 := byte(0xC0 | (regField << 3) | dst.RegBits)
		code = append(code, rex2, opcode, modrm2)

		// 4) XCHG RCX, r64_src2  (restore original RCX)
		code = append(code, rexX, 0x87, modrmX)

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
		rexCmp := byte(0x48) // REX.W
		if src2.REXBit == 1 {
			rexCmp |= 0x04
		} // REX.R
		if src1.REXBit == 1 {
			rexCmp |= 0x01
		} // REX.B
		modrmCmp := byte(0xC0 | (src2.RegBits << 3) | src1.RegBits)
		code = append(code, rexCmp, 0x39, modrmCmp)

		// 2) SETcc r/m8 = dst_low
		//    (0F 90+cc /r)
		rexSet := byte(0x40)
		if dst.REXBit == 1 {
			rexSet |= 0x01
		} // REX.B for rm=dst_low
		modrmSet := byte(0xC0 | dst.RegBits)
		code = append(code, rexSet, 0x0F, cc, modrmSet)

		// 3) MOVZX r64_dst, r/m8(dst_low)
		//    zero‐extends that byte into the full 64-bit dst
		rexZX := byte(0x48)
		if dst.REXBit == 1 {
			// need REX.R for the reg field and REX.B for the rm field
			rexZX |= 0x05
		}
		// reg = dst, rm = dst
		modrmZX := byte(0xC0 | (dst.RegBits << 3) | dst.RegBits)
		code = append(code, rexZX, 0x0F, 0xB6, modrmZX)

		return code
	}
}

// generateMulUpperOp64 multiplies two 64-bit operands and stores the high 64 bits of the result in dst
// mode: "signed" uses signed multiplication (IMUL), "unsigned" uses unsigned multiplication (MUL)
func generateMulUpperOp64(mode string) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		reg1, reg2, dstIdx := extractThreeRegs(inst.Args)
		src1 := regInfoList[reg1]
		src2 := regInfoList[reg2]
		dst := regInfoList[dstIdx]

		var code []byte

		// Save RAX and RDX
		code = append(code, 0x50) // push rax
		code = append(code, 0x52) // push rdx

		// Move src1 into RAX
		rexMov1 := byte(0x48) // REX.W
		if src1.REXBit == 1 {
			rexMov1 |= 0x01 // REX.B for src1
		}
		modrmMov1 := byte(0xC0 | (0 << 3) | src1.RegBits)
		code = append(code, rexMov1, 0x8B, modrmMov1)

		// Clear RDX
		rexXor := byte(0x48)
		// no rexR, only rm=RDX bit0
		if regInfoList[2].REXBit == 1 {
			rexXor |= 0x01
		}
		modrmXor := byte(0xC0 | (2 << 3) | 2)
		code = append(code, rexXor, 0x31, modrmXor)

		// Multiply RAX by src2
		extension := byte(0x4)
		if mode == "signed" {
			extension = 0x5
		}
		rexMul := byte(0x48) // REX.W
		if src2.REXBit == 1 {
			rexMul |= 0x01 // REX.B for src2
		}
		modrmMul := byte(0xC0 | (extension << 3) | src2.RegBits)
		code = append(code, rexMul, 0xF7, modrmMul)

		// Move upper 64 bits from RDX to dst
		rexMov2 := byte(0x48) // REX.W
		if regInfoList[2].REXBit == 1 {
			rexMov2 |= 0x04 // REX.R for RDX
		}
		if dst.REXBit == 1 {
			rexMov2 |= 0x01 // REX.B for dst
		}
		modrmMov2 := byte(0xC0 | (2 << 3) | dst.RegBits)
		code = append(code, rexMov2, 0x89, modrmMov2)

		// Restore RDX and RAX
		code = append(code, 0x5A) // pop rdx
		code = append(code, 0x58) // pop rax

		return code
	}
}

// generateMovRegToReg generates a 64-bit register-to-register MOV instruction.
// It produces the machine code for `MOV dst, src`.
func generateMovRegToReg(dst, src *X86Reg) []byte {
	// MOV r/m64, r64 (opcode 0x89)
	rex := byte(0x48) // REX.W
	if src.REXBit == 1 {
		rex |= 0x04 // REX.R for src
	}
	if dst.REXBit == 1 {
		rex |= 0x01 // REX.B for dst
	}
	modrm := byte(0xC0 | (src.RegBits << 3) | dst.RegBits)
	return []byte{rex, 0x89, modrm}
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
				if tmp.REXBit == 1 {
					code = append(code, 0x41)
				}
				code = append(code, 0x50|tmp.RegBits) // PUSH tmp

				// 1) MOV tmp, src1
				code = append(code, generateMovRegToReg(&tmp, &src1)...)
				// 2) OP tmp, src2
				opCode := generateOp(opcode, &tmp, &src2)
				code = append(code, opCode...)
				// 3) MOV dst, tmp
				code = append(code, generateMovRegToReg(&dst, &tmp)...)

				// Restore scratch register from the stack
				if tmp.REXBit == 1 {
					code = append(code, 0x41)
				}
				code = append(code, 0x58|tmp.RegBits) // POP tmp
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
	rex := byte(0x48) // REX.W
	if reg2.REXBit == 1 {
		rex |= 0x04 // REX.R for the register field (reg2)
	}
	if reg1.REXBit == 1 {
		rex |= 0x01 // REX.B for the r/m field (reg1)
	}
	modrm := byte(0xC0 | (reg2.RegBits << 3) | reg1.RegBits)

	if opcode == 0x0F { // IMUL dst, src => 0F AF /r
		// For IMUL, the destination register is in the REX.R field.
		rex = byte(0x48) // REX.W
		if reg1.REXBit == 1 {
			rex |= 0x04 // REX.R for reg1 (destination)
		}
		if reg2.REXBit == 1 {
			rex |= 0x01 // REX.B for reg2 (source)
		}
		modrm = byte(0xC0 | (reg1.RegBits << 3) | reg2.RegBits)
		return []byte{rex, 0x0F, 0xAF, modrm}
	}

	// Standard opcodes like ADD, SUB, XOR, etc. (e.g., ADD r/m64, r64)
	return []byte{rex, opcode, modrm}
}

func generateSHLO_L_32() func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		srcAIdx, srcBIdx, dstIdx := extractThreeRegs(inst.Args)
		srcA := regInfoList[srcAIdx]
		srcB := regInfoList[srcBIdx]
		dst := regInfoList[dstIdx]

		var code []byte

		// 1) preserve RCX
		code = append(code, 0x51) // PUSH RCX

		// 2) MOV ECX, r32_srcB
		code = append(code,
			emitRex(false, false, false, srcB.REXBit == 1), // REX.B = srcB?
			0x8B,                         // MOV r32, r/m32
			0xC0|(1<<3)|(srcB.RegBits&7), // reg=1(ECX), rm=srcB
		)

		// 3) MOV r32_dst, r32_srcA  (zero-extends high half)
		code = append(code,
			emitRex(false, srcA.REXBit == 1, false, dst.REXBit == 1), // REX.R=srcA, REX.B=dst
			0x89, // MOV r/m32, r32
			0xC0|((srcA.RegBits&7)<<3)|(dst.RegBits&7),
		)

		// 4) SHL r/m32(dst), CL
		code = append(code,
			emitRex(false, false, false, dst.REXBit == 1), // REX.B=dst
			0xD3,                        // SHL r/m32,CL
			0xC0|(4<<3)|(dst.RegBits&7), // /4 = SHL
		)

		// 5) sign-extend the 32-bit result → 64-bit
		code = append(code,
			emitRex(true, dst.REXBit == 1, false, dst.REXBit == 1), // W=1, R=dst, B=dst
			0x63, // MOVSXD r64, r/m32
			0xC0|((dst.RegBits&7)<<3)|(dst.RegBits&7),
		)

		// 6) restore RCX
		code = append(code, 0x59) // POP RCX

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
			if scratch.REXBit == 1 {
				code = append(code, 0x41, 0x50+scratch.RegBits) // push r8–r15
			} else {
				code = append(code, 0x50+scratch.RegBits) // push rax–rdi
			}
		}
		if needRCX {
			code = append(code, 0x51) // push rcx
		}

		// 2) Load shift count into ECX (CL), using REX.B when srcB ≥ 8
		if bIdx != 1 {
			rex := byte(0x40)
			if srcB.REXBit == 1 {
				rex |= 0x01 // REX.B for rm=srcB
			}
			// ModR/M: reg=1 (ECX), rm=srcB
			modrm := byte(0xC0 | (1 << 3) | srcB.RegBits)
			code = append(code, rex, 0x8B, modrm) // MOV ECX, r/m32
		}

		// 3) Mask CL to [0..31] (optional; CPU does this implicitly)
		code = append(code, 0x83, 0xE1, 0x1F) // AND ECX, 0x1F

		// 4) Move the 32-bit value into either dst or scratch
		if needScratch {
			if aIdx != scratchIdx {
				rex := byte(0x40)
				if srcA.REXBit == 1 {
					rex |= 0x04
				} // REX.R = srcA
				if scratch.REXBit == 1 {
					rex |= 0x01
				} // REX.B = scratch
				modrm := byte(0xC0 | (srcA.RegBits << 3) | scratch.RegBits)
				code = append(code, rex, 0x89, modrm) // MOV scratch, srcA
			}
		} else {
			if aIdx != dIdx {
				rex := byte(0x40)
				if srcA.REXBit == 1 {
					rex |= 0x04
				} // REX.R = srcA
				if dst.REXBit == 1 {
					rex |= 0x01
				} // REX.B = dst
				modrm := byte(0xC0 | (srcA.RegBits << 3) | dst.RegBits)
				code = append(code, rex, 0x89, modrm) // MOV dst, srcA
			}
		}

		// 5) SAR    working32, CL
		{
			rex := byte(0x40)
			target := dst
			if needScratch {
				target = scratch
			}
			if target.REXBit == 1 {
				rex |= 0x01
			} // REX.B = target
			modrm := byte(0xC0 | (regField << 3) | target.RegBits)
			code = append(code, rex, opcode, modrm) // D3 /7 = SAR r/m32, CL
		}

		// 6) MOVSXD dst, working32  (sign-extend 32→64)
		{
			rex := byte(0x48) // REX.W = 1
			if dst.REXBit == 1 {
				rex |= 0x04 // REX.R = dst→reg
				rex |= 0x01 // REX.B = dst→rm
			}
			srcRm := dst.RegBits
			if needScratch {
				srcRm = scratch.RegBits
			}
			modrm := byte(0xC0 | (dst.RegBits << 3) | srcRm)
			code = append(code, rex, 0x63, modrm) // MOVSXD r64, r/m32
		}

		// 7) Restore RCX and scratch
		if needRCX {
			code = append(code, 0x59) // pop rcx
		}
		if needScratchPreserve {
			if scratch.REXBit == 1 {
				code = append(code, 0x41, 0x58+scratch.RegBits) // pop r8–r15
			} else {
				code = append(code, 0x58+scratch.RegBits) // pop rax–rdi
			}
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
		//    emitRex(w=false, R=false, X=false, B=srcB.REXBit)
		modrm := byte(0xC0 | (1 << 3) | (srcB.RegBits & 7)) // reg=1 (ECX), rm=srcB
		code = append(code,
			emitRex(false, false, false, srcB.REXBit == 1),
			0x8B, modrm,
		)

		// 3) MOV r32_dst, r32_srcA
		//    89 /r = MOV r/m32, r32
		//    emitRex(w=false, R=srcA.REXBit, X=false, B=dst.REXBit)
		modrm = byte(0xC0 | ((srcA.RegBits & 7) << 3) | (dst.RegBits & 7))
		code = append(code,
			emitRex(false, srcA.REXBit == 1, false, dst.REXBit == 1),
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
			emitRex(true, dst.REXBit == 1, false, dst.REXBit == 1),
			0x63, modrm,
		)

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
		rex1 := byte(0x40)
		if srcA.REXBit == 1 {
			rex1 |= 0x04
		} // REX.R
		if dst.REXBit == 1 {
			rex1 |= 0x01
		} // REX.B
		modrm1 := byte(0xC0 | (srcA.RegBits << 3) | dst.RegBits)
		code = append(code, rex1, 0x89, modrm1)

		// 2) XCHG ECX, r32_srcB  ; swap shift count into CL
		rexX := byte(0x40)
		if srcB.REXBit == 1 {
			rexX |= 0x04
		} // REX.R
		modrmX := byte(0xC0 | (srcB.RegBits << 3) | 0x01) // rm=1 (ECX)
		code = append(code, rexX, 0x87, modrmX)

		// 3) D3 /n r/m32(dst), CL
		rex2 := byte(0x40)
		if dst.REXBit == 1 {
			rex2 |= 0x01
		} // REX.B
		modrm2 := byte(0xC0 | (regField << 3) | dst.RegBits)
		code = append(code, rex2, opcode, modrm2)

		// 4) XCHG ECX, r32_srcB  ; restore ECX and srcB
		code = append(code, rexX, 0x87, modrmX)

		// 5) If SAR (regField==7), sign-extend 32->64: MOVSXD r64_dst, r/m32(dst)
		if regField == 7 {
			// MOVSXD r64_dst, r/m32(dst)
			rexSX := byte(0x48) // REX.W
			if dst.REXBit == 1 {
				// Extend both reg and rm
				rexSX |= 0x05 // REX.R | REX.B
			}
			modrmSX := byte(0xC0 | (dst.RegBits << 3) | dst.RegBits)
			code = append(code, rexSX, 0x63, modrmSX) // 63 /r = MOVSXD
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

// DIV_U_64: dst = src1 / src2 (unsigned), but if src2==0 then dst=maxUint64.
// Preserves RAX/RDX in r12/r13.
func generateDivUOp64(inst Instruction) []byte {
	srcIdx1, srcIdx2, dstIdx := extractThreeRegs(inst.Args)
	src1 := regInfoList[srcIdx1]
	src2 := regInfoList[srcIdx2]
	dst := regInfoList[dstIdx]

	code := []byte{}

	// -- save RAX → r12, RDX → r13 --
	code = append(code,
		0x49, 0x89, 0xC4, // mov r12, rax
		0x49, 0x89, 0xD5, // mov r13, rdx
	)

	// 1) MOV RAX, src1
	rex := byte(0x48)
	if src1.REXBit == 1 {
		rex |= 0x01
	} // REX.B for rm=src1
	code = append(code, rex, 0x8B, byte(0xC0|src1.RegBits))

	// 2) TEST src2, src2
	rex = 0x48 // REX.W
	if src2.REXBit == 1 {
		rex |= 0x05
	} // REX.R + REX.B
	// mod=11, reg=src2, rm=src2
	code = append(code, rex, 0x85, byte(0xC0|(src2.RegBits<<3)|src2.RegBits))

	// 3) JNE doDiv
	jneOff := len(code)
	code = append(code, 0x0F, 0x85, 0, 0, 0, 0) // placeholder

	// -- src2 == 0 path → dst = maxUint64 via XOR/NOT --
	// XOR r64_dst, r64_dst
	rex = 0x48
	if dst.REXBit == 1 {
		rex |= 0x05
	} // REX.W + REX.R+B
	code = append(code, rex, 0x31, byte(0xC0|(dst.RegBits<<3)|dst.RegBits))
	// NOT r/m64 dst
	rex = 0x48
	if dst.REXBit == 1 {
		rex |= 0x01
	} // REX.B
	code = append(code, rex, 0xF7, byte(0xC0|(2<<3)|dst.RegBits))

	// JMP end
	jmpOff := len(code)
	code = append(code, 0xE9, 0, 0, 0, 0)

	// -- doDiv: normal unsigned divide --
	doDivPos := len(code)
	binary.LittleEndian.PutUint32(code[jneOff+2:], uint32(doDivPos-(jneOff+6)))

	// XOR RDX, RDX
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

	// -- patch JMP end --
	endPos := len(code)
	binary.LittleEndian.PutUint32(code[jmpOff+1:], uint32(endPos-(jmpOff+5)))

	// -- restore RAX/RDX from r12/r13 --
	code = append(code,
		0x49, 0x8B, 0xC4, // mov rax, r12
		0x49, 0x8B, 0xD5, // mov rdx, r13
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
	code = append(code, 0x50, 0x52) // push rax; push rdx

	// 1) MOV    EAX, src32
	rex := byte(0x40) // default REX for 32-bit
	if srcInfo.REXBit == 1 {
		rex |= 0x01
	} // REX.B if src ∈ r8–r15
	modrm := byte(0xC0 | (0 << 3) | srcInfo.RegBits)
	code = append(code, rex, 0x8B, modrm) // mov eax, r32_src

	// 2) TEST   src2, src2        ; check divisor==0
	rex = byte(0x40)
	if src2Info.REXBit == 1 {
		rex |= 0x04 | 0x01
	} // REX.R,Rex.B
	modrm = byte(0xC0 | (src2Info.RegBits << 3) | src2Info.RegBits)
	code = append(code, rex, 0x85, modrm) // test r32_src2, r32_src2

	// 3) JE     zeroDiv
	jeZeroOff := len(code)
	code = append(code, 0x0F, 0x84, 0, 0, 0, 0)

	// 4) CMP    EAX, 0x80000000    ; detect INT32_MIN
	rex = byte(0x40)
	if srcInfo.REXBit == 1 {
		rex |= 0x01
	}
	// 81 /7 id → cmp r/m32, imm32
	modrm = byte(0xC0 | (7 << 3) | srcInfo.RegBits)
	// imm32 = 0x80000000 (LE)
	code = append(code, rex, 0x81, modrm, 0x00, 0x00, 0x00, 0x80)

	// 5) JNE    doDiv             ; normal path if not INT32_MIN
	jneDivOff := len(code)
	code = append(code, 0x0F, 0x85, 0, 0, 0, 0)

	// 6) CMP    src2, -1          ; divisor == -1 ?
	rex = byte(0x40)
	if src2Info.REXBit == 1 {
		rex |= 0x04 | 0x01
	}
	// 83 /7 ib → cmp r/m32, imm8
	modrm = byte(0xC0 | (7 << 3) | src2Info.RegBits)
	code = append(code, rex, 0x83, modrm, 0xFF) // imm8 = -1

	// 7) JNE    doDiv             ; if not -1, go doDiv
	jneDivOff2 := len(code)
	code = append(code, 0x0F, 0x85, 0, 0, 0, 0)

	// --- overflow case: INT32_MIN % -1 → remainder = 0 -----
	// 8) XOR    EAX, EAX          ; clear EAX to 0
	code = append(code, 0x31, 0xC0)

	// 9) MOVSXD dst, EAX         ; sign‐extend 0 into dst
	rex = byte(0x48)
	if dstInfo.REXBit == 1 {
		rex |= 0x04
	} // REX.R for dst
	modrm = byte(0xC0 | (dstInfo.RegBits << 3)) // rm=0 (EAX)
	code = append(code, rex, 0x63, modrm)

	// 10) JMP    end
	jmpOff := len(code)
	code = append(code, 0xE9, 0, 0, 0, 0)

	// --- doDiv path -----------------------------------------------
	doDiv := len(code)
	// 11) CDQ                   ; sign‐extend EAX → EDX:EAX
	code = append(code, 0x99)
	// 12) IDIV   src2
	rex = byte(0x40)
	if src2Info.REXBit == 1 {
		rex |= 0x01
	}
	modrm = byte(0xC0 | (7 << 3) | src2Info.RegBits)
	code = append(code, rex, 0xF7, modrm)
	// 13) MOVSXD dst, EDX      ; move remainder into dst
	rex = byte(0x48)
	if dstInfo.REXBit == 1 {
		rex |= 0x04
	}
	modrm = byte(0xC0 | (dstInfo.RegBits << 3) | 0x2) // rm=2 (EDX)
	code = append(code, rex, 0x63, modrm)
	// 14) JMP    end
	jmpOff2 := len(code)
	code = append(code, 0xE9, 0, 0, 0, 0)

	// --- zeroDiv label: divisor=0 ----------------------------------
	zeroDiv := len(code)
	// 15) MOVSXD dst, EAX      ; remainder = dividend
	rex = byte(0x48)
	if dstInfo.REXBit == 1 {
		rex |= 0x04
	}
	modrm = byte(0xC0 | (dstInfo.RegBits << 3)) // rm=0 (EAX)
	code = append(code, rex, 0x63, modrm)

	// --- end label -------------------------------------------------
	end := len(code)
	// restore RDX, RAX
	code = append(code, 0x5A, 0x58)

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
		rexTest := byte(0x48) // REX.W
		if srcB.REXBit == 1 {
			rexTest |= 0x01
		} // REX.B for rm
		if srcB.REXBit == 1 {
			rexTest |= 0x04
		} // REX.R for reg
		modrmTest := byte(0xC0 | (srcB.RegBits << 3) | srcB.RegBits)
		code := []byte{rexTest, 0x85, modrmTest} // 0x85 = TEST r/m64, r64

		// 2) CMOVcc r64_dst, r64_srcA
		rexCmov := byte(0x48)
		if dst.REXBit == 1 {
			rexCmov |= 0x04
		} // REX.R for reg field = dst
		if srcA.REXBit == 1 {
			rexCmov |= 0x01
		} // REX.B for rm field = srcA
		modrmCmov := byte(0xC0 | (dst.RegBits << 3) | srcA.RegBits)
		code = append(code, rexCmov, 0x0F, cc, modrmCmov)

		return code
	}
}
