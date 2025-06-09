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
	reg1, reg2, dst := extractThreeRegs(inst.Args)
	src1 := regInfoList[reg1]
	src2 := regInfoList[reg2]
	dstReg := regInfoList[dst]

	code := []byte{
		0x48 | (src1.REXBit<<2 | dstReg.REXBit), 0x89, 0xC0 | (src1.RegBits << 3) | dstReg.RegBits,
		0x48 | (dstReg.REXBit << 0), 0xF7, 0xD0 | dstReg.RegBits,
		0x48 | (dstReg.REXBit | src2.REXBit<<2), 0x09, 0xC0 | (src2.RegBits << 3) | dstReg.RegBits,
	}
	return code
}

func generateAndInvOp64(inst Instruction) []byte {
	reg1, reg2, dst := extractThreeRegs(inst.Args)
	src1 := regInfoList[reg1]
	src2 := regInfoList[reg2]
	dstReg := regInfoList[dst]

	code := []byte{
		0x48 | (src1.REXBit<<2 | dstReg.REXBit), 0x89, 0xC0 | (src1.RegBits << 3) | dstReg.RegBits,
		0x48 | (dstReg.REXBit << 0), 0xF7, 0xD0 | dstReg.RegBits,
		0x48 | (dstReg.REXBit | src2.REXBit<<2), 0x21, 0xC0 | (src2.RegBits << 3) | dstReg.RegBits,
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
		byte(0xC0|(2<<3)|0), // mod=11, reg=2=RDX, rm=0=RAX
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
	modrm = byte(0xC0 | (dstInfo.RegBits << 3) | 0x0) // rm=0 => EAX
	code = append(code, rex, 0x63, modrm)             // 0x63=/r MOVSXD r64, r/m32
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
	code = append(code, rex, 0x81, byte(0xC0|(7<<3)|0), // 81 /7 id
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
	code = append(code, rex, 0x63, byte(0xC0|(dstInfo.RegBits<<3)|0))

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
	code = append(code, rex, 0x63, byte(0xC0|(dstInfo.RegBits<<3)|0))

	// patch end jumps
	endPos := len(code)
	binary.LittleEndian.PutUint32(code[jmpEnd+1:], uint32(endPos-(jmpEnd+5)))
	binary.LittleEndian.PutUint32(code[jmpOvfEnd+1:], uint32(endPos-(jmpOvfEnd+5)))

	// -- Restore RDX, RAX --
	code = append(code, 0x5A, 0x58)

	return code
}

// Implements a 32-bit register-register MUL (MUL_32): r32_dst = r32_src * r32_src2
func generateMul32(inst Instruction) []byte {
	srcIdx, srcIdx2, dstReg := extractThreeRegs(inst.Args)
	src := regInfoList[srcIdx]
	src2 := regInfoList[srcIdx2]
	dst := regInfoList[dstReg]

	// 1) MOV r32_dst, r32_src
	rexMov := byte(0x40) // REX.W=0, only extension bits
	if src.REXBit == 1 {
		rexMov |= 0x04 // REX.R selects high bit for src
	}
	if dst.REXBit == 1 {
		rexMov |= 0x01 // REX.B selects high bit for dst
	}
	modrmMov := byte(0xC0 | (src.RegBits << 3) | dst.RegBits)
	code := []byte{rexMov, 0x89, modrmMov} // MOV r/m32(dst), r32(src)

	// 2) IMUL r32_dst, r32_src2 (two-operand form)
	rexMul := byte(0x40)
	if dst.REXBit == 1 {
		rexMul |= 0x04 // REX.R for dest in ModRM.reg
	}
	if src2.REXBit == 1 {
		rexMul |= 0x01 // REX.B for src2 in ModRM.rm
	}
	modrmMul := byte(0xC0 | (dst.RegBits << 3) | src2.RegBits)
	code = append(code, rexMul, 0x0F, 0xAF, modrmMul) // IMUL r32(dst), r/m32(src2)

	return code
}

// Implements a 64-bit register-register MUL (MUL_64): r64_dst = r64_src * r64_src2
func generateMul64(inst Instruction) []byte {
	srcIdx, srcIdx2, dstReg := extractThreeRegs(inst.Args)
	src := regInfoList[srcIdx]
	src2 := regInfoList[srcIdx2]
	dst := regInfoList[dstReg]

	// 1) MOV r64_dst, r64_src
	rexMov := byte(0x48) // REX.W=1, default no REX.R/X/B
	if src.REXBit == 1 {
		rexMov |= 0x04 // REX.R selects high bit for src
	}
	if dst.REXBit == 1 {
		rexMov |= 0x01 // REX.B selects high bit for dst
	}
	modrmMov := byte(0xC0 | (src.RegBits << 3) | dst.RegBits)
	code := []byte{rexMov, 0x89, modrmMov} // MOV r/m64(dst), r64(src)

	// 2) IMUL r64_dst, r64_src2 (two-operand form)
	rexMul := byte(0x48) // REX.W=1
	if dst.REXBit == 1 {
		rexMul |= 0x04 // REX.R for dest in ModRM.reg
	}
	if src2.REXBit == 1 {
		rexMul |= 0x01 // REX.B for src2 in ModRM.rm
	}
	modrmMul := byte(0xC0 | (dst.RegBits << 3) | src2.RegBits)
	code = append(code, rexMul, 0x0F, 0xAF, modrmMul) // IMUL r64(dst), r/m64(src2)

	return code
}

// Performs dst = src1 <op> src2 (32-bit), then sign-extends into 64-bit dst.
func generateBinaryOp32(opcode byte) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		reg1, reg2, dstIdx := extractThreeRegs(inst.Args)

		dstReg := regInfoList[dstIdx]
		src1Reg := regInfoList[reg1]
		src2Reg := regInfoList[reg2]

		var code []byte

		// 1) MOV r32_dst, r32_src1
		rex1 := byte(0x40)
		if src1Reg.REXBit == 1 {
			rex1 |= 0x04
		} // REX.R
		if dstReg.REXBit == 1 {
			rex1 |= 0x01
		} // REX.B
		modrm1 := byte(0xC0 | (src1Reg.RegBits << 3) | dstReg.RegBits)
		code = append(code, rex1, 0x89, modrm1)

		// 2) OP   r/m32(dst), r32(src2)
		rex2 := byte(0x40)
		if src2Reg.REXBit == 1 {
			rex2 |= 0x04
		}
		if dstReg.REXBit == 1 {
			rex2 |= 0x01
		}
		modrm2 := byte(0xC0 | (src2Reg.RegBits << 3) | dstReg.RegBits)
		if opcode == 0x0F {
			// IMUL r32, r/m32
			code = append(code, rex2, 0x0F, 0xAF, modrm2)
		} else {
			// ADD/SUB/etc.
			code = append(code, rex2, opcode, modrm2)
		}

		// 3) Sign-extend the 32-bit result into 64 bits:
		//    MOVSXD r64_dst, r/m32(dst)  (0x63 /r with REX.W)
		rexSX := byte(0x48) // REX.W=1
		if dstReg.REXBit == 1 {
			// REX.R for reg field, REX.B for rm field
			rexSX |= 0x05
		}
		modrmSX := byte(0xC0 | (dstReg.RegBits << 3) | dstReg.RegBits)
		code = append(code, rexSX, 0x63, modrmSX)

		return code
	}
}

// Implements a register‐register arithmetic right shift on 64‐bit values:
//
//	r64_dst = int64(r64_src1) >> (r8_src2 & 63)
//
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

func generateMulUpperOp64(mode string) func(Instruction) []byte {
	return func(inst Instruction) []byte {
		reg1, reg2, dst := extractThreeRegs(inst.Args)

		src1 := regInfoList[reg1]
		src2 := regInfoList[reg2]
		dstReg := regInfoList[dst]

		var code []byte

		// mov rax, src1
		rex := byte(0x48 | (src1.REXBit << 2))
		modrm := byte(0xC0 | (src1.RegBits << 3) | 0)
		code = append(code, rex, 0x89, modrm) // mov rax, src1

		// clear rdx
		code = append(code, 0x48, 0x31, 0xD2) // xor rdx, rdx

		// imul/mul src2
		rex2 := byte(0x48 | (src2.REXBit << 2))
		modrm2 := byte(0xC0 | (0x04 << 3) | src2.RegBits) // /4 or /5 for mul/imul
		switch mode {
		case "signed":
			modrm2 = 0xC0 | (0x05 << 3) | src2.RegBits // /5 = imul
			code = append(code, rex2, 0xF7, modrm2)
		case "unsigned":
			modrm2 = 0xC0 | (0x04 << 3) | src2.RegBits // /4 = mul
			code = append(code, rex2, 0xF7, modrm2)
		case "mixed":
			panic("mixed signed/unsigned multiply not implemented")
		}

		// mov dst, rdx
		rex3 := byte(0x48 | (dstReg.REXBit << 2))
		modrm3 := byte(0xC0 | (2 << 3) | dstReg.RegBits)
		code = append(code, rex3, 0x89, modrm3)

		return code
	}
}

func generateBinaryOp64(opcode byte) func(Instruction) []byte {
	return func(inst Instruction) []byte {
		reg1, reg2, dst := extractThreeRegs(inst.Args)

		dstReg := regInfoList[dst]
		src1Reg := regInfoList[reg1]
		src2Reg := regInfoList[reg2]

		var code []byte

		// mov dst, src1
		rex1 := byte(0x48)
		if src1Reg.REXBit == 1 {
			rex1 |= 0x04 // REX.R
		}
		if dstReg.REXBit == 1 {
			rex1 |= 0x01 // REX.B
		}
		modrm1 := byte(0xC0 | (src1Reg.RegBits << 3) | dstReg.RegBits)
		code = append(code, rex1, 0x89, modrm1)

		// opcode dst, src2
		rex2 := byte(0x48)
		if src2Reg.REXBit == 1 {
			rex2 |= 0x04
		}
		if dstReg.REXBit == 1 {
			rex2 |= 0x01
		}
		modrm2 := byte(0xC0 | (src2Reg.RegBits << 3) | dstReg.RegBits)
		if opcode == 0x0F {
			code = append(code, rex2, 0x0F, 0xAF, modrm2)
		} else {
			code = append(code, rex2, opcode, modrm2)
		}

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
	modrm = byte(0xC0 | (dstInfo.RegBits << 3) | 0x0) // rm=0 (EAX)
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
	modrm = byte(0xC0 | (dstInfo.RegBits << 3) | 0x0) // rm=0 (EAX)
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
