package pvm

import (
	"math"
	"math/bits"
)

// A.5.13. Instructions with Arguments of Three Registers.

func handleADD_32(vm *VM, operands []byte) {
	registerIndexA := min(12, int(operands[0]&0x0F))
	registerIndexB := min(12, int(operands[0]>>4))
	registerIndexD := min(12, int(operands[1]))

	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	sum32 := uint32(valueA) + uint32(valueB)
	result := uint64(sum32)
	if sum32&0x80000000 != 0 {
		result |= 0xFFFFFFFF00000000
	}
	vm.register[registerIndexD] = result
	if PvmTrace {
		dumpThreeRegOp("ADD_32", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleSUB_32(vm *VM, operands []byte) {
	registerIndexA := min(12, int(operands[0]&0x0F))
	registerIndexB := min(12, int(operands[0]>>4))
	registerIndexD := min(12, int(operands[1]))

	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	diff32 := uint32(valueA) - uint32(valueB)
	result := uint64(diff32)
	if diff32&0x80000000 != 0 {
		result |= 0xFFFFFFFF00000000
	}
	vm.register[registerIndexD] = result
	if PvmTrace {
		dumpThreeRegOp("SUB_32", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleMUL_32(vm *VM, operands []byte) {
	registerIndexA := min(12, int(operands[0]&0x0F))
	registerIndexB := min(12, int(operands[0]>>4))
	registerIndexD := min(12, int(operands[1]))

	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	prod32 := uint32(valueA) * uint32(valueB)
	result := uint64(prod32)
	if prod32&0x80000000 != 0 {
		result |= 0xFFFFFFFF00000000
	}
	vm.register[registerIndexD] = result
	if PvmTrace {
		dumpThreeRegOp("MUL_32", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleDIV_U_32(vm *VM, operands []byte) {
	registerIndexA := min(12, int(operands[0]&0x0F))
	registerIndexB := min(12, int(operands[0]>>4))
	registerIndexD := min(12, int(operands[1]))

	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	var result uint64
	if valueB&0xFFFF_FFFF == 0 {
		result = uint64(math.MaxUint64)
	} else {
		quot32 := uint32(valueA) / uint32(valueB)
		result = uint64(quot32)
		if quot32&0x80000000 != 0 {
			result |= 0xFFFFFFFF00000000
		}
	}
	vm.register[registerIndexD] = result
	if PvmTrace {
		dumpThreeRegOp("DIV_U_32", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleDIV_S_32(vm *VM, operands []byte) {
	registerIndexA := min(12, int(operands[0]&0x0F))
	registerIndexB := min(12, int(operands[0]>>4))
	registerIndexD := min(12, int(operands[1]))

	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	a, b := int32(valueA), int32(valueB)
	var result uint64
	switch {
	case b == 0:
		result = uint64(math.MaxUint64)
	case a == math.MinInt32 && b == -1:
		result = uint64(a)
	default:
		result = uint64(int64(a / b))
	}
	vm.register[registerIndexD] = result
	if PvmTrace {
		dumpThreeRegOp("DIV_S_32", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleREM_U_32(vm *VM, operands []byte) {
	registerIndexA := min(12, int(operands[0]&0x0F))
	registerIndexB := min(12, int(operands[0]>>4))
	registerIndexD := min(12, int(operands[1]))

	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	var result uint64
	if valueB&0xFFFF_FFFF == 0 {
		val32 := uint32(valueA)
		result = uint64(val32)
		if val32&0x80000000 != 0 {
			result |= 0xFFFFFFFF00000000
		}
	} else {
		r := uint32(valueA) % uint32(valueB)
		result = uint64(r)
		if r&0x80000000 != 0 {
			result |= 0xFFFFFFFF00000000
		}
	}
	vm.register[registerIndexD] = result
	if PvmTrace {
		dumpThreeRegOp("REM_U_32", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleREM_S_32(vm *VM, operands []byte) {
	registerIndexA := min(12, int(operands[0]&0x0F))
	registerIndexB := min(12, int(operands[0]>>4))
	registerIndexD := min(12, int(operands[1]))

	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	a, b := int32(valueA), int32(valueB)
	var result uint64
	switch {
	case b == 0:
		result = uint64(a)
	case a == math.MinInt32 && b == -1:
		result = 0
	default:
		result = uint64(int64(a % b))
	}
	vm.register[registerIndexD] = result
	if PvmTrace {
		dumpThreeRegOp("REM_S_32", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleSHLO_L_32(vm *VM, operands []byte) {
	registerIndexA := min(12, int(operands[0]&0x0F))
	registerIndexB := min(12, int(operands[0]>>4))
	registerIndexD := min(12, int(operands[1]))

	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	shift32 := uint32(valueA) << (valueB & 31)
	result := uint64(shift32)
	if shift32&0x80000000 != 0 {
		result |= 0xFFFFFFFF00000000
	}
	vm.register[registerIndexD] = result
	if PvmTrace {
		dumpShiftOp("<<", registerIndexD, registerIndexA, valueB&31, result)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleSHLO_R_32(vm *VM, operands []byte) {
	registerIndexA := min(12, int(operands[0]&0x0F))
	registerIndexB := min(12, int(operands[0]>>4))
	registerIndexD := min(12, int(operands[1]))

	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	shift32 := uint32(valueA) >> (valueB & 31)
	result := uint64(shift32)
	if shift32&0x80000000 != 0 {
		result |= 0xFFFFFFFF00000000
	}
	vm.register[registerIndexD] = result
	if PvmTrace {
		dumpShiftOp(">>", registerIndexD, registerIndexA, valueB&31, result)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleSHAR_R_32(vm *VM, operands []byte) {
	registerIndexA := min(12, int(operands[0]&0x0F))
	registerIndexB := min(12, int(operands[0]>>4))
	registerIndexD := min(12, int(operands[1]))

	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	result := uint64(int32(valueA) >> (valueB & 31))
	vm.register[registerIndexD] = result
	if PvmTrace {
		dumpShiftOp(">>", registerIndexD, registerIndexA, valueB&31, result)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleADD_64(vm *VM, operands []byte) {
	registerIndexA := min(12, int(operands[0]&0x0F))
	registerIndexB := min(12, int(operands[0]>>4))
	registerIndexD := min(12, int(operands[1]))

	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	result := valueA + valueB
	vm.register[registerIndexD] = result
	if PvmTrace {
		dumpThreeRegOp("+", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleSUB_64(vm *VM, operands []byte) {
	registerIndexA := min(12, int(operands[0]&0x0F))
	registerIndexB := min(12, int(operands[0]>>4))
	registerIndexD := min(12, int(operands[1]))

	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	result := valueA - valueB
	vm.register[registerIndexD] = result
	if PvmTrace {
		dumpThreeRegOp("-", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleMUL_64(vm *VM, operands []byte) {
	registerIndexA := min(12, int(operands[0]&0x0F))
	registerIndexB := min(12, int(operands[0]>>4))
	registerIndexD := min(12, int(operands[1]))

	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	result := valueA * valueB
	vm.register[registerIndexD] = result
	if PvmTrace {
		dumpThreeRegOp("*", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleDIV_U_64(vm *VM, operands []byte) {
	registerIndexA := min(12, int(operands[0]&0x0F))
	registerIndexB := min(12, int(operands[0]>>4))
	registerIndexD := min(12, int(operands[1]))

	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	var result uint64
	if valueB == 0 {
		result = uint64(math.MaxUint64)
	} else {
		result = valueA / valueB
	}
	vm.register[registerIndexD] = result
	if PvmTrace {
		dumpThreeRegOp("/", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleDIV_S_64(vm *VM, operands []byte) {
	registerIndexA := min(12, int(operands[0]&0x0F))
	registerIndexB := min(12, int(operands[0]>>4))
	registerIndexD := min(12, int(operands[1]))
	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	var result uint64
	if valueB == 0 {
		result = uint64(math.MaxUint64)
	} else if int64(valueA) == -(1<<63) && int64(valueB) == -1 {
		result = valueA
	} else {
		result = uint64(int64(valueA) / int64(valueB))
	}
	if PvmTrace {
		dumpThreeRegOp("/", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	}
	vm.register[registerIndexD] = result
	vm.pc += 1 + uint64(len(operands))
}

func handleREM_U_64(vm *VM, operands []byte) {
	registerIndexA := min(12, int(operands[0]&0x0F))
	registerIndexB := min(12, int(operands[0]>>4))
	registerIndexD := min(12, int(operands[1]))
	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	var result uint64
	if valueB == 0 {
		result = valueA
	} else {
		result = valueA % valueB
	}
	if PvmTrace {
		dumpThreeRegOp("%", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	}
	vm.register[registerIndexD] = result
	vm.pc += 1 + uint64(len(operands))
}
func smod(a, b int64) int64 {
	if b == 0 {
		return a
	}

	absA := a
	if absA < 0 {
		absA = -absA
	}
	absB := b
	if absB < 0 {
		absB = -absB
	}

	modVal := absA % absB

	if a < 0 {
		return -modVal
	}
	return modVal
}
func handleREM_S_64(vm *VM, operands []byte) {
	registerIndexA := min(12, int(operands[0]&0x0F))
	registerIndexB := min(12, int(operands[0]>>4))
	registerIndexD := min(12, int(operands[1]))
	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	var result uint64
	if int64(valueA) == -(1<<63) && int64(valueB) == -1 {
		result = 0
	} else {
		result = uint64(smod(int64(valueA), int64(valueB)))
	}
	if PvmTrace {
		dumpThreeRegOp("%", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	}
	vm.register[registerIndexD] = result
	vm.pc += 1 + uint64(len(operands))
}

func handleSHLO_L_64(vm *VM, operands []byte) {
	registerIndexA := min(12, int(operands[0]&0x0F))
	registerIndexB := min(12, int(operands[0]>>4))
	registerIndexD := min(12, int(operands[1]))
	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	result := valueA << (valueB & 63)
	if PvmTrace {
		dumpShiftOp("<<", registerIndexD, registerIndexA, valueB&63, result)
	}
	vm.register[registerIndexD] = result
	vm.pc += 1 + uint64(len(operands))
}

func handleSHLO_R_64(vm *VM, operands []byte) {
	registerIndexA := min(12, int(operands[0]&0x0F))
	registerIndexB := min(12, int(operands[0]>>4))
	registerIndexD := min(12, int(operands[1]))
	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	result := valueA >> (valueB & 63)
	if PvmTrace {
		dumpShiftOp(">>", registerIndexD, registerIndexA, valueB&63, result)
	}
	vm.register[registerIndexD] = result
	vm.pc += 1 + uint64(len(operands))
}

func handleSHAR_R_64(vm *VM, operands []byte) {
	registerIndexA := min(12, int(operands[0]&0x0F))
	registerIndexB := min(12, int(operands[0]>>4))
	registerIndexD := min(12, int(operands[1]))
	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	result := uint64(int64(valueA) >> (valueB & 63))
	if PvmTrace {
		dumpShiftOp("SHAR_R_64", registerIndexD, registerIndexA, valueB&63, result)
	}
	vm.register[registerIndexD] = result
	vm.pc += 1 + uint64(len(operands))
}

func handleAND(vm *VM, operands []byte) {
	registerIndexA := min(12, int(operands[0]&0x0F))
	registerIndexB := min(12, int(operands[0]>>4))
	registerIndexD := min(12, int(operands[1]))
	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	result := valueA & valueB
	if PvmTrace {
		dumpThreeRegOp("&", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	}
	vm.register[registerIndexD] = result
	vm.pc += 1 + uint64(len(operands))
}

func handleXOR(vm *VM, operands []byte) {
	registerIndexA := min(12, int(operands[0]&0x0F))
	registerIndexB := min(12, int(operands[0]>>4))
	registerIndexD := min(12, int(operands[1]))
	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	result := valueA ^ valueB
	if PvmTrace {
		dumpThreeRegOp("^", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	}
	vm.register[registerIndexD] = result
	vm.pc += 1 + uint64(len(operands))
}

func handleOR(vm *VM, operands []byte) {
	registerIndexA := min(12, int(operands[0]&0x0F))
	registerIndexB := min(12, int(operands[0]>>4))
	registerIndexD := min(12, int(operands[1]))
	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	result := valueA | valueB
	if PvmTrace {
		dumpThreeRegOp("|", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	}
	vm.register[registerIndexD] = result
	vm.pc += 1 + uint64(len(operands))
}

func handleMUL_UPPER_S_S(vm *VM, operands []byte) {
	registerIndexA := min(12, int(operands[0]&0x0F))
	registerIndexB := min(12, int(operands[0]>>4))
	registerIndexD := min(12, int(operands[1]))
	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	hi, _ := bits.Mul64(valueA, valueB)
	if valueA>>63 == 1 {
		hi -= valueB
	}
	if valueB>>63 == 1 {
		hi -= valueA
	}
	result := hi
	if PvmTrace {
		dumpThreeRegOp("*s", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	}
	vm.register[registerIndexD] = result
	vm.pc += 1 + uint64(len(operands))
}

func handleMUL_UPPER_U_U(vm *VM, operands []byte) {
	registerIndexA := min(12, int(operands[0]&0x0F))
	registerIndexB := min(12, int(operands[0]>>4))
	registerIndexD := min(12, int(operands[1]))
	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	result, _ := bits.Mul64(valueA, valueB)
	if PvmTrace {
		dumpThreeRegOp("*u", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	}
	vm.register[registerIndexD] = result
	vm.pc += 1 + uint64(len(operands))
}

func handleMUL_UPPER_S_U(vm *VM, operands []byte) {
	registerIndexA := min(12, int(operands[0]&0x0F))
	registerIndexB := min(12, int(operands[0]>>4))
	registerIndexD := min(12, int(operands[1]))
	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	hi, _ := bits.Mul64(valueA, valueB)
	if valueA>>63 == 1 {
		hi -= valueB
	}
	result := hi
	if PvmTrace {
		dumpThreeRegOp("*s", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	}
	vm.register[registerIndexD] = result
	vm.pc += 1 + uint64(len(operands))
}

func handleSET_LT_U(vm *VM, operands []byte) {
	registerIndexA := min(12, int(operands[0]&0x0F))
	registerIndexB := min(12, int(operands[0]>>4))
	registerIndexD := min(12, int(operands[1]))
	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	var result uint64
	if valueA < valueB {
		result = 1
	}
	if PvmTrace {
		dumpCmpOp("<u", registerIndexD, registerIndexA, valueB, result)
	}
	vm.register[registerIndexD] = result
	vm.pc += 1 + uint64(len(operands))
}

func handleSET_LT_S(vm *VM, operands []byte) {
	registerIndexA := min(12, int(operands[0]&0x0F))
	registerIndexB := min(12, int(operands[0]>>4))
	registerIndexD := min(12, int(operands[1]))
	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	var result uint64
	if int64(valueA) < int64(valueB) {
		result = 1
	}
	if PvmTrace {
		dumpCmpOp("<s", registerIndexD, registerIndexA, valueB, result)
	}
	vm.register[registerIndexD] = result
	vm.pc += 1 + uint64(len(operands))
}

func handleCMOV_IZ(vm *VM, operands []byte) {
	registerIndexA := min(12, int(operands[0]&0x0F))
	registerIndexB := min(12, int(operands[0]>>4))
	registerIndexD := min(12, int(operands[1]))

	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	if valueB == 0 {
		vm.register[registerIndexD] = valueA
		if PvmTrace {
			dumpCmovOp("CMOV_IZ", registerIndexD, registerIndexB, valueA, valueA, valueA, true)
		}
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleCMOV_NZ(vm *VM, operands []byte) {
	registerIndexA := min(12, int(operands[0]&0x0F))
	registerIndexB := min(12, int(operands[0]>>4))
	registerIndexD := min(12, int(operands[1]))

	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	if valueB != 0 {
		vm.register[registerIndexD] = valueA
		if PvmTrace {
			dumpCmovOp("CMOV_NZ", registerIndexD, registerIndexB, valueA, valueA, valueA, false)
		}
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleROT_L_64(vm *VM, operands []byte) {
	registerIndexA := min(12, int(operands[0]&0x0F))
	registerIndexB := min(12, int(operands[0]>>4))
	registerIndexD := min(12, int(operands[1]))
	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	result := bits.RotateLeft64(valueA, int(valueB&63))
	if PvmTrace {
		dumpRotOp("<<", reg(registerIndexD), reg(registerIndexA), valueB&63, result)
	}
	vm.register[registerIndexD] = result
	vm.pc += 1 + uint64(len(operands))
}

func handleROT_L_32(vm *VM, operands []byte) {
	registerIndexA := min(12, int(operands[0]&0x0F))
	registerIndexB := min(12, int(operands[0]>>4))
	registerIndexD := min(12, int(operands[1]))
	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	rot32 := bits.RotateLeft32(uint32(valueA), int(valueB&31))
	result := uint64(rot32)
	if rot32&0x80000000 != 0 {
		result |= 0xFFFFFFFF00000000
	}
	if PvmTrace {
		dumpRotOp("<<", reg(registerIndexD), reg(registerIndexA), valueB&31, result)
	}
	vm.register[registerIndexD] = result
	vm.pc += 1 + uint64(len(operands))
}

func handleROT_R_64(vm *VM, operands []byte) {
	registerIndexA := min(12, int(operands[0]&0x0F))
	registerIndexB := min(12, int(operands[0]>>4))
	registerIndexD := min(12, int(operands[1]))
	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	result := bits.RotateLeft64(valueA, -int(valueB&63))
	if PvmTrace {
		dumpRotOp(">>", reg(registerIndexD), reg(registerIndexA), valueB&63, result)
	}
	vm.register[registerIndexD] = result
	vm.pc += 1 + uint64(len(operands))
}

func handleROT_R_32(vm *VM, operands []byte) {
	registerIndexA := min(12, int(operands[0]&0x0F))
	registerIndexB := min(12, int(operands[0]>>4))
	registerIndexD := min(12, int(operands[1]))
	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	rot32 := bits.RotateLeft32(uint32(valueA), -int(valueB&31))
	result := uint64(rot32)
	if rot32&0x80000000 != 0 {
		result |= 0xFFFFFFFF00000000
	}
	if PvmTrace {
		dumpRotOp(">>", reg(registerIndexD), reg(registerIndexA), valueB&31, result)
	}
	vm.register[registerIndexD] = result
	vm.pc += 1 + uint64(len(operands))
}

func handleAND_INV(vm *VM, operands []byte) {
	registerIndexA := min(12, int(operands[0]&0x0F))
	registerIndexB := min(12, int(operands[0]>>4))
	registerIndexD := min(12, int(operands[1]))
	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	result := valueA & (^valueB)
	if PvmTrace {
		dumpThreeRegOp("&!", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	}
	vm.register[registerIndexD] = result
	vm.pc += 1 + uint64(len(operands))
}

func handleOR_INV(vm *VM, operands []byte) {
	registerIndexA := min(12, int(operands[0]&0x0F))
	registerIndexB := min(12, int(operands[0]>>4))
	registerIndexD := min(12, int(operands[1]))
	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	result := valueA | (^valueB)
	if PvmTrace {
		dumpThreeRegOp("|!", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	}
	vm.register[registerIndexD] = result
	vm.pc += 1 + uint64(len(operands))
}

func handleXNOR(vm *VM, operands []byte) {
	registerIndexA := min(12, int(operands[0]&0x0F))
	registerIndexB := min(12, int(operands[0]>>4))
	registerIndexD := min(12, int(operands[1]))
	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	result := ^(valueA ^ valueB)
	if PvmTrace {
		dumpThreeRegOp("^!", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	}
	vm.register[registerIndexD] = result
	vm.pc += 1 + uint64(len(operands))
}

func handleMAX(vm *VM, operands []byte) {
	registerIndexA := min(12, int(operands[0]&0x0F))
	registerIndexB := min(12, int(operands[0]>>4))
	registerIndexD := min(12, int(operands[1]))
	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	result := uint64(max(int64(valueA), int64(valueB)))
	if PvmTrace {
		dumpThreeRegOp("max", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	}
	vm.register[registerIndexD] = result
	vm.pc += 1 + uint64(len(operands))
}

func handleMAX_U(vm *VM, operands []byte) {
	registerIndexA := min(12, int(operands[0]&0x0F))
	registerIndexB := min(12, int(operands[0]>>4))
	registerIndexD := min(12, int(operands[1]))
	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	result := max(valueA, valueB)
	if PvmTrace {
		dumpThreeRegOp("max", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	}
	vm.register[registerIndexD] = result
	vm.pc += 1 + uint64(len(operands))
}

func handleMIN(vm *VM, operands []byte) {
	registerIndexA := min(12, int(operands[0]&0x0F))
	registerIndexB := min(12, int(operands[0]>>4))
	registerIndexD := min(12, int(operands[1]))
	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	result := uint64(min(int64(valueA), int64(valueB)))
	if PvmTrace {
		dumpThreeRegOp("min", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	}
	vm.register[registerIndexD] = result
	vm.pc += 1 + uint64(len(operands))
}

func handleMIN_U(vm *VM, operands []byte) {
	registerIndexA := min(12, int(operands[0]&0x0F))
	registerIndexB := min(12, int(operands[0]>>4))
	registerIndexD := min(12, int(operands[1]))
	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	result := min(valueA, valueB)
	if PvmTrace {
		dumpThreeRegOp("minu", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	}
	vm.register[registerIndexD] = result
	vm.pc += 1 + uint64(len(operands))
}
