package pvm

import "fmt"

func extractRegTriplet(args []byte) (reg1, reg2, dst int, err error) {
	if len(args) != 2 {
		return 0, 0, 0, fmt.Errorf("requires 2 args")
	}
	reg1 = min(12, int(args[0]&0x0F))
	reg2 = min(12, int(args[0]>>4))
	dst = min(12, int(args[1]))
	return
}

func generateCmovCmpOp64(cc byte) func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		reg1, reg2, dst, err := extractRegTriplet(inst.Args)
		if err != nil {
			return nil, fmt.Errorf("CMOV_CMP %w", err)
		}

		src1 := regInfoList[reg1]
		src2 := regInfoList[reg2]
		dstReg := regInfoList[dst]

		code := []byte{
			0x48 | (src1.REXBit<<2 | src2.REXBit), 0x39, 0xC0 | (src2.RegBits << 3) | src1.RegBits,
			0x48 | (src1.REXBit<<2 | dstReg.REXBit), 0x8B, 0xC0 | (src1.RegBits << 3) | dstReg.RegBits,
			0x48 | (src2.REXBit<<2 | dstReg.REXBit), 0x0F, cc, 0xC0 | (src2.RegBits << 3) | dstReg.RegBits,
		}
		return code, nil
	}
}

func generateXnorOp64() func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		reg1, reg2, dst, err := extractRegTriplet(inst.Args)
		if err != nil {
			return nil, fmt.Errorf("XNOR %w", err)
		}
		src1 := regInfoList[reg1]
		src2 := regInfoList[reg2]
		dstReg := regInfoList[dst]

		code := []byte{
			0x48 | (src1.REXBit<<2 | dstReg.REXBit), 0x89, 0xC0 | (src1.RegBits << 3) | dstReg.RegBits,
			0x48 | (dstReg.REXBit | src2.REXBit<<2), 0x31, 0xC0 | (src2.RegBits << 3) | dstReg.RegBits,
			0x48 | (dstReg.REXBit << 0), 0xF7, 0xD0 | dstReg.RegBits,
		}
		return code, nil
	}
}

func generateOrInvOp64() func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		reg1, reg2, dst, err := extractRegTriplet(inst.Args)
		if err != nil {
			return nil, fmt.Errorf("OR_INV %w", err)
		}
		src1 := regInfoList[reg1]
		src2 := regInfoList[reg2]
		dstReg := regInfoList[dst]

		code := []byte{
			0x48 | (src1.REXBit<<2 | dstReg.REXBit), 0x89, 0xC0 | (src1.RegBits << 3) | dstReg.RegBits,
			0x48 | (dstReg.REXBit << 0), 0xF7, 0xD0 | dstReg.RegBits,
			0x48 | (dstReg.REXBit | src2.REXBit<<2), 0x09, 0xC0 | (src2.RegBits << 3) | dstReg.RegBits,
		}
		return code, nil
	}
}

func generateAndInvOp64() func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		reg1, reg2, dst, err := extractRegTriplet(inst.Args)
		if err != nil {
			return nil, fmt.Errorf("AND_INV %w", err)
		}
		src1 := regInfoList[reg1]
		src2 := regInfoList[reg2]
		dstReg := regInfoList[dst]

		code := []byte{
			0x48 | (src1.REXBit<<2 | dstReg.REXBit), 0x89, 0xC0 | (src1.RegBits << 3) | dstReg.RegBits,
			0x48 | (dstReg.REXBit << 0), 0xF7, 0xD0 | dstReg.RegBits,
			0x48 | (dstReg.REXBit | src2.REXBit<<2), 0x21, 0xC0 | (src2.RegBits << 3) | dstReg.RegBits,
		}
		return code, nil
	}
}

func generateRemSOp64() func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		code, err := generateDivSOp64()(inst)
		if err != nil {
			return nil, err
		}
		return code, nil
	}
}
func generateRemUOp64() func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		code, err := generateDivUOp64()(inst)
		if err != nil {
			return nil, err
		}
		return code, nil
	}
}

func generateRemUOp32() func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		code, err := generateDivUOp32()(inst)
		if err != nil {
			return nil, err
		}
		// result is in EDX
		return code, nil
	}
}

// Implements DIV Unsigned: r32_dst = r32_src / r32_src2 (32-bit)
func generateDivUOp32() func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		srcIdx, srcIdx2, dstReg, _ := extractRegTriplet(inst.Args)
		src := regInfoList[srcIdx]
		src2 := regInfoList[srcIdx2]
		dst := regInfoList[dstReg]

		var code []byte
		// 1) MOV EAX, r32_src
		rexMov := byte(0x40)
		if src.REXBit == 1 {
			rexMov |= 0x04
		}
		code = append(code, rexMov, 0x8B, byte(0xC0|(src.RegBits<<3)|0x00))

		// 2) XOR EDX, EDX
		code = append(code, byte(0x40), 0x31, byte(0xC0|(0x02<<3)|0x02))

		// 3) DIV r/m32 (divisor in src2)
		rexDiv := byte(0x40)
		if src2.REXBit == 1 {
			rexDiv |= 0x04
		}
		code = append(code, rexDiv, 0xF7, byte(0xC0|(0x06<<3)|src2.RegBits))

		// 4) MOV r32_dst, EAX
		rexRes := byte(0x40)
		if dst.REXBit == 1 {
			rexRes |= 0x01
		}
		code = append(code, rexRes, 0x89, byte(0xC0|(0x00<<3)|dst.RegBits))

		return code, nil
	}
}

// Implements DIV signed: r32_dst = r32_src / r32_src2 (32-bit)
func generateDivSOp32() func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		srcIdx, srcIdx2, dstReg, _ := extractRegTriplet(inst.Args)
		src := regInfoList[srcIdx]
		src2 := regInfoList[srcIdx2]
		dst := regInfoList[dstReg]

		var code []byte
		// 1) MOV EAX, r32_src
		rexMov := byte(0x40)
		if src.REXBit == 1 {
			rexMov |= 0x04
		}
		code = append(code, rexMov, 0x8B, byte(0xC0|(src.RegBits<<3)|0x00))

		// 2) CDQ -> sign extend EAX into EDX:EAX
		code = append(code, 0x99)

		// 3) IDIV r/m32 (divisor in src2)
		rexDiv := byte(0x40)
		if src2.REXBit == 1 {
			rexDiv |= 0x04
		}
		code = append(code, rexDiv, 0xF7, byte(0xC0|(0x07<<3)|src2.RegBits))

		// 4) MOV r32_dst, EAX
		rexRes := byte(0x40)
		if dst.REXBit == 1 {
			rexRes |= 0x01
		}
		code = append(code, rexRes, 0x89, byte(0xC0|(0x00<<3)|dst.RegBits))

		return code, nil
	}
}

// Implements a 32-bit register-register MUL (MUL_32): r32_dst = r32_src * r32_src2
func generateMul32() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		srcIdx, srcIdx2, dstReg, _ := extractRegTriplet(inst.Args)
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

		return code, nil
	}
}

// Implements a 64-bit register-register MUL (MUL_64): r64_dst = r64_src * r64_src2
func generateMul64() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		srcIdx, srcIdx2, dstReg, _ := extractRegTriplet(inst.Args)
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

		return code, nil
	}
}

func generateBinaryOp32(opcode byte) func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		reg1, reg2, dst, err := extractRegTriplet(inst.Args)
		if err != nil {
			return nil, fmt.Errorf("binary op: %v", err)
		}

		dstReg := regInfoList[dst]
		src1Reg := regInfoList[reg1]
		src2Reg := regInfoList[reg2]

		var code []byte

		// mov dst, src1
		rex1 := byte(0x40)
		if src1Reg.REXBit == 1 {
			rex1 |= 0x04
		}
		if dstReg.REXBit == 1 {
			rex1 |= 0x01
		}
		modrm1 := byte(0xC0 | (src1Reg.RegBits << 3) | dstReg.RegBits)
		code = append(code, rex1, 0x89, modrm1)

		// opcode dst, src2
		rex2 := byte(0x40)
		if src2Reg.REXBit == 1 {
			rex2 |= 0x04
		}
		if dstReg.REXBit == 1 {
			rex2 |= 0x01
		}
		modrm2 := byte(0xC0 | (src2Reg.RegBits << 3) | dstReg.RegBits)
		if opcode == 0x0F {
			code = append(code, rex2, 0x0F, 0xAF, modrm2) // imul
		} else {
			code = append(code, rex2, opcode, modrm2)
		}

		return code, nil
	}
}

func generateShiftOp64(opcode byte, regField byte) func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		args := inst.Args
		reg := min(12, int(args[0]))
		regInfo := regInfoList[reg]

		rex := byte(0x48)
		if regInfo.REXBit == 1 {
			rex |= 0x01
		}

		modrm := byte(0xC0 | (regField << 3) | regInfo.RegBits)
		return []byte{rex, opcode, modrm}, nil
	}
}

// Implements “dst = (src1 <cond> src2) ? 1 : 0” in 64-bit registers.
func generateSetCondOp64(cc byte) func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		// Unpack triplet: reg1, reg2, dst
		reg1, reg2, dstIdx, _ := extractRegTriplet(inst.Args)
		src1 := regInfoList[reg1]
		src2 := regInfoList[reg2]
		dst := regInfoList[dstIdx]

		// 1) CMP r/m64=src1, r64=src2  (i.e. compare src1 vs src2)
		//    opcode: REX.W + 0x39 /r
		rexCmp := byte(0x48) // REX.W
		if src2.REXBit == 1 {
			rexCmp |= 0x04
		} // REX.R for the reg field
		if src1.REXBit == 1 {
			rexCmp |= 0x01
		} // REX.B for the rm field
		modrmCmp := byte(0xC0 | (src2.RegBits << 3) | src1.RegBits)
		code := []byte{rexCmp, 0x39, modrmCmp}

		// 2) SETcc r/m8=dst_low  (0F 90+cc /r)
		rexSet := byte(0x40) // no W bit for 8-bit op
		if dst.REXBit == 1 {
			rexSet |= 0x01
		} // REX.B for dst_low
		modrmSet := byte(0xC0 | dst.RegBits) // mod=11, reg field ignored, rm=dst
		code = append(code, rexSet, 0x0F, cc, modrmSet)

		// 3) MOVZX r64=dst, r/m8=dst_low (REX.W + 0F B6 /r)
		rexMovzx := byte(0x48) // REX.W
		if dst.REXBit == 1 {
			rexMovzx |= 0x01
		} // REX.B for r/m8
		modrmMovzx := byte(0xC0 | (dst.RegBits << 3) | dst.RegBits)
		code = append(code, rexMovzx, 0x0F, 0xB6, modrmMovzx)

		return code, nil
	}
}

func generateMulUpperOp64(mode string) func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		reg1, reg2, dst, err := extractRegTriplet(inst.Args)
		if err != nil {
			return nil, fmt.Errorf("MUL_UPPER_64 %w", err)
		}
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
			return nil, fmt.Errorf("mixed signed/unsigned multiply not implemented")
		}

		// mov dst, rdx
		rex3 := byte(0x48 | (dstReg.REXBit << 2))
		modrm3 := byte(0xC0 | (2 << 3) | dstReg.RegBits)
		code = append(code, rex3, 0x89, modrm3)

		return code, nil
	}
}

func generateBinaryOp64(opcode byte) func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		reg1, reg2, dst, err := extractRegTriplet(inst.Args)
		if err != nil {
			return nil, fmt.Errorf("binary op: %v", err)
		}

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

		return code, nil
	}
}

// Implements signed 32-bit shift then zero-extend into 64 bits:
//
//	r32_dst = int32(r32_src) op (r8_src2 & 31)
//	r64_dst = uint64(r32_dst)
func generateShiftOp32(opcode byte, regField byte) func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		srcIdx, srcIdx2, dstIdx, _ := extractRegTriplet(inst.Args)
		src := regInfoList[srcIdx]
		src2 := regInfoList[srcIdx2]
		dst := regInfoList[dstIdx]

		// 1) MOV r32_dst, r32_src
		rex1 := byte(0x40) // no W
		if src.REXBit == 1 {
			rex1 |= 0x04
		} // REX.R
		if dst.REXBit == 1 {
			rex1 |= 0x01
		} // REX.B
		modrm1 := byte(0xC0 | (src.RegBits << 3) | dst.RegBits)
		code := []byte{rex1, 0x89, modrm1}

		// 2) MOV CL, r8_src2
		rex2 := byte(0x40)
		if src2.REXBit == 1 {
			rex2 |= 0x01
		} // REX.B
		modrm2 := byte(0xC0 | (0x01 << 3) | src2.RegBits) // reg=1 (CL)
		code = append(code, rex2, 0x8A, modrm2)

		// 3) SHIFT r/m32(dst), CL  (SAR/SHR/SHL depending on opcode & regField)
		rex3 := byte(0x40)
		if dst.REXBit == 1 {
			rex3 |= 0x01
		}
		modrm3 := byte(0xC0 | (regField << 3) | dst.RegBits)
		code = append(code, rex3, opcode, modrm3)

		// 4) MOV r64_dst, r/m32(dst) → zero-extend 32→64 bits
		rex4 := byte(0x48) // W=1
		if dst.REXBit == 1 {
			rex4 |= 0x01
		}
		modrm4 := byte(0xC0 | (dst.RegBits << 3) | dst.RegBits)
		code = append(code, rex4, 0x8B, modrm4)

		return code, nil
	}
}

// Implements DIV signed: r64_dst = r64_src / r64_src2 (64-bit)
func generateDivSOp64() func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		srcIdx, srcIdx2, dstReg, _ := extractRegTriplet(inst.Args)
		src := regInfoList[srcIdx]
		src2 := regInfoList[srcIdx2]
		dst := regInfoList[dstReg]

		var code []byte
		// 1) MOV RAX, r64_src
		rexMov := byte(0x48)
		if src.REXBit == 1 {
			rexMov |= 0x04
		}
		code = append(code, rexMov, 0x8B, byte(0xC0|(src.RegBits<<3)|0x00))

		// 2) CQO (sign extend RAX into RDX:RAX)
		code = append(code, 0x48, 0x99)

		// 3) IDIV r/m64 (src2)
		rexDiv := byte(0x48)
		if src2.REXBit == 1 {
			rexDiv |= 0x04
		}
		code = append(code, rexDiv, 0xF7, byte(0xC0|(0x07<<3)|src2.RegBits))

		// 4) MOV r64_dst, RAX
		rexRes := byte(0x48)
		if dst.REXBit == 1 {
			rexRes |= 0x01
		}
		code = append(code, rexRes, 0x89, byte(0xC0|(0x00<<3)|dst.RegBits))

		return code, nil
	}
}

// TODO: implement DIV unsigned: r64_dst = r64_src / r64_src2
func generateDivUOp64() func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		srcIdx, srcIdx2, dstReg, _ := extractRegTriplet(inst.Args)
		src := regInfoList[srcIdx]
		src2 := regInfoList[srcIdx2]
		dst := regInfoList[dstReg]

		rex := byte(0x48)
		if src.REXBit == 1 {
			rex |= 0x04 // REX.R for src
		}
		if dst.REXBit == 1 {
			rex |= 0x01 // REX.B for dst
		}

		modrm := byte(0xC0 | (6 << 3) | src2.RegBits)
		return []byte{0x48, 0x31, 0xD2, rex, 0xF7, modrm}, nil
	}
}

// Implements REM signed: r32_dst = r32_src % r32_src2 (32-bit)
func generateRemSOp32() func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		srcIdx, srcIdx2, dstReg, _ := extractRegTriplet(inst.Args)
		src := regInfoList[srcIdx]
		src2 := regInfoList[srcIdx2]
		dst := regInfoList[dstReg]

		var code []byte
		// 1) MOV EAX, r32_src
		rexMov := byte(0x40)
		if src.REXBit == 1 {
			rexMov |= 0x04
		}
		code = append(code, rexMov, 0x8B, byte(0xC0|(src.RegBits<<3)|0x00))

		// 2) CDQ
		code = append(code, 0x99)

		// 3) IDIV r/m32 (src2)
		rexDiv := byte(0x40)
		if src2.REXBit == 1 {
			rexDiv |= 0x04
		}
		code = append(code, rexDiv, 0xF7, byte(0xC0|(0x07<<3)|src2.RegBits))

		// 4) MOV r32_dst, EDX (remainder)
		rexRes := byte(0x40)
		if dst.REXBit == 1 {
			rexRes |= 0x01
		}
		code = append(code, rexRes, 0x89, byte(0xC0|(0x02<<3)|dst.RegBits))

		return code, nil
	}
}

// TODO: fix this to use extractRegTriplet
func generateCmovOp64(opcode byte) func(Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		args := inst.Args
		src := min(12, int(args[0]&0x0F))
		dst := min(12, int(args[1]))

		srcReg := regInfoList[src]
		dstReg := regInfoList[dst]

		rex := byte(0x48) // REX.W for 64-bit
		if srcReg.REXBit == 1 {
			rex |= 0x04 // REX.R
		}
		if dstReg.REXBit == 1 {
			rex |= 0x01 // REX.B
		}

		modrm := byte(0xC0 | (srcReg.RegBits << 3) | dstReg.RegBits)
		code := []byte{rex, 0x0F, opcode, modrm}
		return code, nil
	}
}
