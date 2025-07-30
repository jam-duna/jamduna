package pvm

// A.5.9. Instructions with Arguments of Two Registers.

func generateMoveReg(inst Instruction) []byte {
	dstIdx, srcIdx := extractTwoRegisters(inst.Args)
	return emitMovRegToReg64(regInfoList[dstIdx], regInfoList[srcIdx])
}

func generateBitCount64(inst Instruction) []byte {
	dstIdx, srcIdx := extractTwoRegisters(inst.Args)
	return emitPopCnt64(regInfoList[dstIdx], regInfoList[srcIdx])
}

func generateBitCount32(inst Instruction) []byte {
	dstIdx, srcIdx := extractTwoRegisters(inst.Args)
	return emitPopCnt32(regInfoList[dstIdx], regInfoList[srcIdx])
}

func generateLeadingZeros64(inst Instruction) []byte {
	dstIdx, srcIdx := extractTwoRegisters(inst.Args)
	return emitLzcnt64(regInfoList[dstIdx], regInfoList[srcIdx])
}

func generateLeadingZeros32(inst Instruction) []byte {
	dstIdx, srcIdx := extractTwoRegisters(inst.Args)
	return emitLzcnt32(regInfoList[dstIdx], regInfoList[srcIdx])
}

func generateTrailingZeros32(inst Instruction) []byte {
	dstIdx, srcIdx := extractTwoRegisters(inst.Args)
	return emitTzcnt32(regInfoList[dstIdx], regInfoList[srcIdx])
}

func generateTrailingZeros64(inst Instruction) []byte {
	dstIdx, srcIdx := extractTwoRegisters(inst.Args)
	return emitTzcnt64(regInfoList[dstIdx], regInfoList[srcIdx])
}

func generateSignExtend8(inst Instruction) []byte {
	dstIdx, srcIdx := extractTwoRegisters(inst.Args)
	return emitMovsx64From8(regInfoList[dstIdx], regInfoList[srcIdx])
}

func generateSignExtend16(inst Instruction) []byte {
	dstIdx, srcIdx := extractTwoRegisters(inst.Args)
	return emitMovsx64From16(regInfoList[dstIdx], regInfoList[srcIdx])
}

func generateZeroExtend16(inst Instruction) []byte {
	dstIdx, srcIdx := extractTwoRegisters(inst.Args)
	return emitMovzx64From16(regInfoList[dstIdx], regInfoList[srcIdx])
}

func generateReverseBytes64(inst Instruction) []byte {
	dstIdx, _ := extractTwoRegisters(inst.Args)
	return emitBswap64(regInfoList[dstIdx])
}
