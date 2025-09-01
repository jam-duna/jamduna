package pvm

import (
	"math"
	"math/bits"
)

// A.5.13 Three Registers handlers

func (vm *VM) handleADD_32(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, registerIndexD := extractThreeRegs(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := x_encode(uint64(uint32(valueA)+uint32(valueB)), 4)
	dumpThreeRegOp("ADD_32", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleSUB_32(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, registerIndexD := extractThreeRegs(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := x_encode(uint64(uint32(valueA)-uint32(valueB)), 4)
	dumpThreeRegOp("SUB_32", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleMUL_32(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, registerIndexD := extractThreeRegs(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := x_encode(uint64(uint32(valueA)*uint32(valueB)), 4)
	dumpThreeRegOp("MUL_32", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleDIV_U_32(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, registerIndexD := extractThreeRegs(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	var result uint64
	if valueB&0xFFFF_FFFF == 0 {
		result = uint64(math.MaxUint64)
	} else {
		result = x_encode(uint64(uint32(valueA)/uint32(valueB)), 4)
	}
	dumpThreeRegOp("DIV_U_32", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleDIV_S_32(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, registerIndexD := extractThreeRegs(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
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
	dumpThreeRegOp("DIV_S_32", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleREM_U_32(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, registerIndexD := extractThreeRegs(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	var result uint64
	if valueB&0xFFFF_FFFF == 0 {
		result = x_encode(uint64(uint32(valueA)), 4)
	} else {
		r := uint32(valueA) % uint32(valueB)
		result = x_encode(uint64(r), 4)
	}
	dumpThreeRegOp("REM_U_32", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleREM_S_32(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, registerIndexD := extractThreeRegs(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
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
	dumpThreeRegOp("REM_S_32", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleSHLO_L_32(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, registerIndexD := extractThreeRegs(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := x_encode(uint64(uint32(valueA)<<(valueB&31)), 4)
	dumpShiftOp("<<", registerIndexD, registerIndexA, valueB&31, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleSHLO_R_32(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, registerIndexD := extractThreeRegs(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := x_encode(uint64(uint32(valueA)>>(valueB&31)), 4)
	dumpShiftOp(">>", registerIndexD, registerIndexA, valueB&31, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleSHAR_R_32(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, registerIndexD := extractThreeRegs(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := uint64(int32(valueA) >> (valueB & 31))
	dumpShiftOp(">>", registerIndexD, registerIndexA, valueB&31, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleADD_64(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, registerIndexD := extractThreeRegs(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := valueA + valueB
	dumpThreeRegOp("+", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleSUB_64(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, registerIndexD := extractThreeRegs(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := valueA - valueB
	dumpThreeRegOp("-", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleMUL_64(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, registerIndexD := extractThreeRegs(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := valueA * valueB
	dumpThreeRegOp("*", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleDIV_U_64(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, registerIndexD := extractThreeRegs(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	var result uint64
	if valueB == 0 {
		result = uint64(math.MaxUint64)
	} else {
		result = valueA / valueB
	}
	dumpThreeRegOp("/", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleDIV_S_64(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, registerIndexD := extractThreeRegs(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	var result uint64
	if valueB == 0 {
		result = uint64(math.MaxUint64)
	} else if int64(valueA) == -(1<<63) && int64(valueB) == -1 {
		result = valueA
	} else {
		result = uint64(int64(valueA) / int64(valueB))
	}
	dumpThreeRegOp("/", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleREM_U_64(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, registerIndexD := extractThreeRegs(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	var result uint64
	if valueB == 0 {
		result = valueA
	} else {
		result = valueA % valueB
	}
	dumpThreeRegOp("%", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleREM_S_64(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, registerIndexD := extractThreeRegs(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	var result uint64
	if int64(valueA) == -(1<<63) && int64(valueB) == -1 {
		result = 0
	} else {
		result = uint64(smod(int64(valueA), int64(valueB)))
	}
	dumpThreeRegOp("%", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleSHLO_L_64(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, registerIndexD := extractThreeRegs(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := valueA << (valueB & 63)
	dumpShiftOp("<<", registerIndexD, registerIndexA, valueB&63, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleSHLO_R_64(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, registerIndexD := extractThreeRegs(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := valueA >> (valueB & 63)
	dumpShiftOp(">>", registerIndexD, registerIndexA, valueB&63, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleSHAR_R_64(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, registerIndexD := extractThreeRegs(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := uint64(int64(valueA) >> (valueB & 63))
	dumpShiftOp("SHAR_R_64", registerIndexD, registerIndexA, valueB&63, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleAND(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, registerIndexD := extractThreeRegs(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := valueA & valueB
	dumpThreeRegOp("&", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleXOR(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, registerIndexD := extractThreeRegs(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := valueA ^ valueB
	dumpThreeRegOp("^", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleOR(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, registerIndexD := extractThreeRegs(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := valueA | valueB
	dumpThreeRegOp("|", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleMUL_UPPER_S_S(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, registerIndexD := extractThreeRegs(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	hi, _ := bits.Mul64(valueA, valueB)
	if valueA>>63 == 1 {
		hi -= valueB
	}
	if valueB>>63 == 1 {
		hi -= valueA
	}
	result := hi
	dumpThreeRegOp("*s", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleMUL_UPPER_U_U(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, registerIndexD := extractThreeRegs(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result, _ := bits.Mul64(valueA, valueB)
	dumpThreeRegOp("*u", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleMUL_UPPER_S_U(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, registerIndexD := extractThreeRegs(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	hi, _ := bits.Mul64(valueA, valueB)
	if valueA>>63 == 1 {
		hi -= valueB
	}
	result := hi
	dumpThreeRegOp("*s", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleSET_LT_U(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, registerIndexD := extractThreeRegs(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	var result uint64
	if valueA < valueB {
		result = 1
	} else {
		result = 0
	}
	dumpCmpOp("<u", registerIndexD, registerIndexA, valueB, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleSET_LT_S(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, registerIndexD := extractThreeRegs(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	var result uint64
	if int64(valueA) < int64(valueB) {
		result = 1
	} else {
		result = 0
	}
	dumpCmpOp("<s", registerIndexD, registerIndexA, valueB, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleCMOV_IZ(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, registerIndexD := extractThreeRegs(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	if valueB == 0 {
		result := valueA
		dumpCmovOp("CMOV_IZ", registerIndexD, registerIndexB, valueA, valueA, result, true)
		vm.Ram.WriteRegister(registerIndexD, result)
	}
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleCMOV_NZ(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, registerIndexD := extractThreeRegs(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	if valueB != 0 {
		result := valueA
		dumpCmovOp("CMOV_NZ", registerIndexD, registerIndexB, valueA, valueA, result, false)
		vm.Ram.WriteRegister(registerIndexD, result)
	}
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleROT_L_64(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, registerIndexD := extractThreeRegs(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := bits.RotateLeft64(valueA, int(valueB&63))
	dumpRotOp("<<", reg(registerIndexD), reg(registerIndexA), valueB&63, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleROT_L_32(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, registerIndexD := extractThreeRegs(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := x_encode(uint64(bits.RotateLeft32(uint32(valueA), int(valueB&31))), 4)
	dumpRotOp("<<", reg(registerIndexD), reg(registerIndexA), valueB&31, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleROT_R_64(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, registerIndexD := extractThreeRegs(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := bits.RotateLeft64(valueA, -int(valueB&63))
	dumpRotOp(">>", reg(registerIndexD), reg(registerIndexA), valueB&63, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleROT_R_32(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, registerIndexD := extractThreeRegs(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := x_encode(uint64(bits.RotateLeft32(uint32(valueA), -int(valueB&31))), 4)
	dumpRotOp(">>", reg(registerIndexD), reg(registerIndexA), valueB&31, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleAND_INV(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, registerIndexD := extractThreeRegs(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := valueA & (^valueB)
	dumpThreeRegOp("&!", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleOR_INV(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, registerIndexD := extractThreeRegs(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := valueA | (^valueB)
	dumpThreeRegOp("|!", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleXNOR(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, registerIndexD := extractThreeRegs(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := ^(valueA ^ valueB)
	dumpThreeRegOp("^!", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleMAX(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, registerIndexD := extractThreeRegs(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := uint64(max(int64(valueA), int64(valueB)))
	dumpThreeRegOp("max", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleMAX_U(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, registerIndexD := extractThreeRegs(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := max(valueA, valueB)
	dumpThreeRegOp("max", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleMIN(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, registerIndexD := extractThreeRegs(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := uint64(min(int64(valueA), int64(valueB)))
	dumpThreeRegOp("min", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleMIN_U(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, registerIndexD := extractThreeRegs(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := min(valueA, valueB)
	dumpThreeRegOp("minu", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}