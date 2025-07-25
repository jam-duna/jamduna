package pvm

// A.5.9. Instructions with Arguments of Two Registers.

func generateMoveReg(inst Instruction) []byte {
	dstIdx, srcIdx := extractTwoRegisters(inst.Args)
	dst := regInfoList[dstIdx]
	src := regInfoList[srcIdx]
	return emitMovRegToReg64(dst, src)
}

func generateBitCount64(inst Instruction) []byte {
	dstReg, srcReg := extractTwoRegisters(inst.Args)
	dst := regInfoList[dstReg]
	src := regInfoList[srcReg]
	return emitPopCnt64(dst, src)
}

func generateBitCount32(inst Instruction) []byte {
	dstReg, srcReg := extractTwoRegisters(inst.Args)
	dst := regInfoList[dstReg]
	src := regInfoList[srcReg]
	return emitPopCnt32(dst, src)
}

func generateLeadingZeros64(inst Instruction) []byte {
	dstIdx, srcIdx := extractTwoRegisters(inst.Args)
	dst := regInfoList[dstIdx]
	src := regInfoList[srcIdx]
	return emitLzcnt64(dst, src)
}

func generateLeadingZeros32(inst Instruction) []byte {
	dstIdx, srcIdx := extractTwoRegisters(inst.Args)
	dst := regInfoList[dstIdx]
	src := regInfoList[srcIdx]
	return emitLzcnt32(dst, src)
}

func generateTrailingZeros32(inst Instruction) []byte {
	dstIdx, srcIdx := extractTwoRegisters(inst.Args)
	dst := regInfoList[dstIdx]
	src := regInfoList[srcIdx]
	return emitTzcnt32(dst, src)
}

func generateTrailingZeros64(inst Instruction) []byte {
	dstReg, srcReg := extractTwoRegisters(inst.Args)
	dst := regInfoList[dstReg]
	src := regInfoList[srcReg]
	return emitTzcnt64(dst, src)
}

func generateSignExtend8(inst Instruction) []byte {
	dstReg, srcReg := extractTwoRegisters(inst.Args)
	dst := regInfoList[dstReg]
	src := regInfoList[srcReg]
	return emitMovsx64From8(dst, src)
}

func generateSignExtend16(inst Instruction) []byte {
	dstReg, srcReg := extractTwoRegisters(inst.Args)
	dst := regInfoList[dstReg]
	src := regInfoList[srcReg]
	return emitMovsx64From16(dst, src)
}

func generateZeroExtend16(inst Instruction) []byte {
	dstReg, srcReg := extractTwoRegisters(inst.Args)
	dst := regInfoList[dstReg]
	src := regInfoList[srcReg]
	return emitMovzx64From16(dst, src)
}

func generateReverseBytes64(inst Instruction) []byte {
	dstReg, _ := extractTwoRegisters(inst.Args)
	dst := regInfoList[dstReg]
	return emitBswap64(dst)
}
