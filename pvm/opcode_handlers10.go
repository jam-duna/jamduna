package pvm

import (
	"encoding/binary"
	"math/bits"

	"github.com/colorfulnotion/jam/types"
)

// A.5.10 Two Registers and One Immediate handlers - STORE operations

func (vm *VM) handleSTORE_IND_U8(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx := extractTwoRegsOneImm(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	addr := uint32((uint64(valueB) + vx) % (1 << 32))
	errCode := vm.Ram.WriteRAMBytes(addr, []byte{byte(uint8(valueA))})
	dumpStoreGeneric("STORE_IND_U8", uint64(addr), reg(registerIndexA), valueA&0xff, 8)
	if errCode != OK {
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		vm.terminated = true
		vm.Fault_address = uint32(errCode)
		return
	}
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleSTORE_IND_U16(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx := extractTwoRegsOneImm(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	addr := uint32((uint64(valueB) + vx) % (1 << 32))
	errCode := vm.Ram.WriteRAMBytes(addr, types.E_l(valueA%(1<<16), 2))
	dumpStoreGeneric("STORE_IND_U16", uint64(addr), reg(registerIndexA), valueA&0xffff, 16)
	if errCode != OK {
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		vm.terminated = true
		vm.Fault_address = uint32(errCode)
		return
	}
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleSTORE_IND_U32(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx := extractTwoRegsOneImm(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	addr := uint32((uint64(valueB) + vx) % (1 << 32))
	errCode := vm.Ram.WriteRAMBytes(addr, types.E_l(valueA%(1<<32), 4))
	dumpStoreGeneric("STORE_IND_U32", uint64(addr), reg(registerIndexA), valueA&0xffffffff, 32)
	if errCode != OK {
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		vm.terminated = true
		vm.Fault_address = uint32(errCode)
		return
	}
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleSTORE_IND_U64(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx := extractTwoRegsOneImm(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	addr := uint32((uint64(valueB) + vx) % (1 << 32))
	errCode := vm.Ram.WriteRAMBytes64(addr, valueA)
	if PvmTrace {
		dumpStoreGeneric("STORE_IND_U64", uint64(addr), reg(registerIndexA), valueA, 64)
	}
	if errCode != OK {
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		vm.terminated = true
		vm.Fault_address = uint32(errCode)
		return
	}
	vm.pc += 1 + uint64(len(operands))
}

// LOAD operations

func (vm *VM) handleLOAD_IND_U8(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx := extractTwoRegsOneImm(operands)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	addr := uint32((uint64(valueB) + vx) % (1 << 32))
	value, errCode := vm.Ram.ReadRAMBytes(addr, 1)
	if errCode != OK {
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		vm.terminated = true
		vm.Fault_address = uint32(errCode)
		return
	}
	result := uint64(uint8(value[0]))
	dumpLoadGeneric("LOAD_IND_U8", registerIndexA, uint64(addr), result, 8, false)
	vm.Ram.WriteRegister(registerIndexA, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleLOAD_IND_I8(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx := extractTwoRegsOneImm(operands)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	addr := uint32((uint64(valueB) + vx) % (1 << 32))
	value, errCode := vm.Ram.ReadRAMBytes(addr, 1)
	if errCode != OK {
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		vm.terminated = true
		vm.Fault_address = uint32(errCode)
		return
	}
	result := uint64(int8(value[0]))
	dumpLoadGeneric("LOAD_IND_I8", registerIndexA, uint64(addr), result, 8, true)
	vm.Ram.WriteRegister(registerIndexA, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleLOAD_IND_U16(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx := extractTwoRegsOneImm(operands)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	addr := uint32((uint64(valueB) + vx) % (1 << 32))
	value, errCode := vm.Ram.ReadRAMBytes(addr, 2)
	if errCode != OK {
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		vm.terminated = true
		vm.Fault_address = uint32(errCode)
		return
	}
	result := types.DecodeE_l(value)
	dumpLoadGeneric("LOAD_IND_U16", registerIndexA, uint64(addr), result, 16, false)
	vm.Ram.WriteRegister(registerIndexA, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleLOAD_IND_I16(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx := extractTwoRegsOneImm(operands)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	addr := uint32((uint64(valueB) + vx) % (1 << 32))
	value, errCode := vm.Ram.ReadRAMBytes(addr, 2)
	if errCode != OK {
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		vm.terminated = true
		vm.Fault_address = uint32(errCode)
		return
	}
	result := uint64(int16(types.DecodeE_l(value)))
	dumpLoadGeneric("LOAD_IND_I16", registerIndexA, uint64(addr), result, 16, true)
	vm.Ram.WriteRegister(registerIndexA, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleLOAD_IND_U32(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx := extractTwoRegsOneImm(operands)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	addr := uint32((uint64(valueB) + vx) % (1 << 32))
	value, errCode := vm.Ram.ReadRAMBytes(addr, 4)
	if errCode != OK {
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		vm.terminated = true
		vm.Fault_address = uint32(errCode)
		return
	}
	result := types.DecodeE_l(value)
	dumpLoadGeneric("LOAD_IND_U32", registerIndexA, uint64(addr), result, 32, false)
	vm.Ram.WriteRegister(registerIndexA, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleLOAD_IND_I32(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx := extractTwoRegsOneImm(operands)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	addr := uint32((uint64(valueB) + vx) % (1 << 32))
	value, errCode := vm.Ram.ReadRAMBytes(addr, 4)
	if errCode != OK {
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		vm.terminated = true
		vm.Fault_address = uint32(errCode)
		return
	}
	result := uint64(int32(types.DecodeE_l(value)))
	dumpLoadGeneric("LOAD_IND_I32", registerIndexA, uint64(addr), result, 32, true)
	vm.Ram.WriteRegister(registerIndexA, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleLOAD_IND_U64(opcode byte, operands []byte) {

	registerIndexA := min(12, int(operands[0]&0x0F))
	registerIndexB := min(12, int(operands[0]>>4))
	lx := min(4, max(0, len(operands)-1))
	vx := x_encode(types.DecodeE_l(operands[1:1+lx]), uint32(lx))
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	value, errCode := vm.Ram.ReadRAMBytes(uint32((uint64(valueB) + vx)), 8)
	if errCode != OK {
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		vm.terminated = true
		vm.Fault_address = uint32(errCode)
		return
	}
	if PvmTrace {
		dumpLoadGeneric("LOAD_IND_U64", registerIndexA, uint64((uint64(valueB) + vx)), binary.LittleEndian.Uint64(value), 64, false)
	}
	vm.Ram.WriteRegister(registerIndexA, binary.LittleEndian.Uint64(value))
	vm.pc += 1 + uint64(len(operands))
}

// Arithmetic operations

func (vm *VM) handleADD_IMM_32(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx := extractTwoRegsOneImm(operands)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := x_encode((valueB+vx)%(1<<32), 4)
	dumpBinOp("+", registerIndexA, registerIndexB, vx, result)
	vm.Ram.WriteRegister(registerIndexA, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleADD_IMM_64(opcode byte, operands []byte) {
	registerIndexA := min(12, int(operands[0]&0x0F))
	registerIndexB := min(12, int(operands[0]>>4))
	lx := min(4, max(0, len(operands)-1))
	vx := x_encode(types.DecodeE_l(operands[1:1+lx]), uint32(lx))
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := valueB + vx
	if PvmTrace {
		dumpBinOp("+", registerIndexA, registerIndexB, vx, result)
	}
	vm.Ram.WriteRegister(registerIndexA, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleAND_IMM(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx := extractTwoRegsOneImm(operands)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := valueB & vx
	dumpBinOp("&", registerIndexA, registerIndexB, vx, result)
	vm.Ram.WriteRegister(registerIndexA, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleXOR_IMM(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx := extractTwoRegsOneImm(operands)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := valueB ^ vx
	dumpBinOp("^", registerIndexA, registerIndexB, vx, result)
	vm.Ram.WriteRegister(registerIndexA, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleOR_IMM(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx := extractTwoRegsOneImm(operands)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := valueB | vx
	dumpBinOp("|", registerIndexA, registerIndexB, vx, result)
	vm.Ram.WriteRegister(registerIndexA, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleMUL_IMM_32(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx := extractTwoRegsOneImm(operands)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := x_encode((valueB*vx)%(1<<32), 4)
	dumpBinOp("*", registerIndexA, registerIndexB, vx, result)
	vm.Ram.WriteRegister(registerIndexA, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleMUL_IMM_64(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx := extractTwoRegsOneImm(operands)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := valueB * vx
	dumpBinOp("*", registerIndexA, registerIndexB, vx, result)
	vm.Ram.WriteRegister(registerIndexA, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleSET_LT_U_IMM(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx := extractTwoRegsOneImm(operands)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := boolToUint(valueB < vx)
	dumpCmpOp("<u", registerIndexA, registerIndexB, vx, result)
	vm.Ram.WriteRegister(registerIndexA, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleSET_LT_S_IMM(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx := extractTwoRegsOneImm(operands)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := boolToUint(int64(valueB) < int64(vx))
	dumpCmpOp("<s", registerIndexA, registerIndexB, vx, result)
	vm.Ram.WriteRegister(registerIndexA, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleSET_GT_U_IMM(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx := extractTwoRegsOneImm(operands)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := boolToUint(valueB > vx)
	dumpCmpOp("u>", registerIndexA, registerIndexB, vx, result)
	vm.Ram.WriteRegister(registerIndexA, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleSET_GT_S_IMM(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx := extractTwoRegsOneImm(operands)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := boolToUint(int64(valueB) > int64(vx))
	dumpCmpOp("s>", registerIndexA, registerIndexB, vx, result)
	vm.Ram.WriteRegister(registerIndexA, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleNEG_ADD_IMM_32(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx := extractTwoRegsOneImm(operands)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := x_encode((vx-valueB)%(1<<32), 4)
	dumpBinOp("-+", registerIndexA, registerIndexB, vx, result)
	vm.Ram.WriteRegister(registerIndexA, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleNEG_ADD_IMM_64(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx := extractTwoRegsOneImm(operands)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := vx - valueB
	dumpBinOp("-+", registerIndexA, registerIndexB, vx, result)
	vm.Ram.WriteRegister(registerIndexA, result)
	vm.pc += 1 + uint64(len(operands))
}

// Shift operations

func (vm *VM) handleSHLO_L_IMM_32(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx := extractTwoRegsOneImm(operands)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := x_encode(valueB<<(vx&63)%(1<<32), 4)
	dumpShiftOp("<<", registerIndexA, registerIndexB, vx, result)
	vm.Ram.WriteRegister(registerIndexA, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleSHLO_L_IMM_64(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx := extractTwoRegsOneImm(operands)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := valueB << (vx & 63)
	dumpShiftOp("<<", registerIndexA, registerIndexB, vx, result)
	vm.Ram.WriteRegister(registerIndexA, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleSHLO_R_IMM_32(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx := extractTwoRegsOneImm(operands)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := x_encode(uint64(uint32(valueB)>>(vx&31)), 4)
	dumpShiftOp(">>", registerIndexA, registerIndexB, vx, result)
	vm.Ram.WriteRegister(registerIndexA, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleSHLO_R_IMM_64(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx := extractTwoRegsOneImm(operands)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := valueB >> (vx & 63)
	dumpShiftOp(">>", registerIndexA, registerIndexB, vx, result)
	vm.Ram.WriteRegister(registerIndexA, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleSHAR_R_IMM_32(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx := extractTwoRegsOneImm(operands)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := uint64(int64(int32(valueB) >> (vx & 31)))
	dumpShiftOp(">>", registerIndexA, registerIndexB, vx, result)
	vm.Ram.WriteRegister(registerIndexA, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleSHAR_R_IMM_64(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx := extractTwoRegsOneImm(operands)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := uint64(int64(valueB) >> (vx & 63))
	dumpShiftOp(">>", registerIndexA, registerIndexB, vx, result)
	vm.Ram.WriteRegister(registerIndexA, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleSHLO_L_IMM_ALT_32(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx := extractTwoRegsOneImm(operands)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := x_encode(vx<<(valueB&63)%(1<<32), 4)
	dumpShiftOp("<<", registerIndexA, registerIndexB, vx, result)
	vm.Ram.WriteRegister(registerIndexA, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleSHLO_L_IMM_ALT_64(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx := extractTwoRegsOneImm(operands)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := vx << (valueB & 63)
	dumpShiftOp("<<", registerIndexA, registerIndexB, vx, result)
	vm.Ram.WriteRegister(registerIndexA, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleSHLO_R_IMM_ALT_32(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx := extractTwoRegsOneImm(operands)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := x_encode(vx>>(valueB&63)%(1<<32), 4)
	dumpShiftOp(">>", registerIndexA, registerIndexB, vx, result)
	vm.Ram.WriteRegister(registerIndexA, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleSHLO_R_IMM_ALT_64(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx := extractTwoRegsOneImm(operands)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := vx >> (valueB & 63)
	dumpShiftOp(">>", registerIndexA, registerIndexB, vx, result)
	vm.Ram.WriteRegister(registerIndexA, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleSHAR_R_IMM_ALT_32(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx := extractTwoRegsOneImm(operands)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := uint64(int32(vx) >> (valueB & 31))
	dumpShiftOp(">>", registerIndexA, registerIndexB, vx, result)
	vm.Ram.WriteRegister(registerIndexA, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleSHAR_R_IMM_ALT_64(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx := extractTwoRegsOneImm(operands)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := uint64(int64(vx) >> (valueB & 63))
	dumpShiftOp(">>", registerIndexA, registerIndexB, vx, result)
	vm.Ram.WriteRegister(registerIndexA, result)
	vm.pc += 1 + uint64(len(operands))
}

// Rotation operations

func (vm *VM) handleROT_R_64_IMM(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx := extractTwoRegsOneImm(operands)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := bits.RotateLeft64(valueB, -int(vx&63))
	dumpRotOp("ROT_R_64_IMM", reg(registerIndexA), reg(registerIndexB), vx, result)
	vm.Ram.WriteRegister(registerIndexA, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleROT_R_64_IMM_ALT(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx := extractTwoRegsOneImm(operands)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := bits.RotateLeft64(vx, -int(valueB&63))
	dumpRotOp("ROT_R_64_IMM_ALT", reg(registerIndexA), reg(registerIndexB), vx, result)
	vm.Ram.WriteRegister(registerIndexA, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleROT_R_32_IMM(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx := extractTwoRegsOneImm(operands)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := x_encode(uint64(bits.RotateLeft32(uint32(valueB), -int(vx&31))), 4)
	dumpRotOp("ROT_R_32_IMM", reg(registerIndexA), reg(registerIndexB), vx, result)
	vm.Ram.WriteRegister(registerIndexA, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleROT_R_32_IMM_ALT(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx := extractTwoRegsOneImm(operands)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := x_encode(uint64(bits.RotateLeft32(uint32(vx), -int(valueB&31))), 4)
	dumpRotOp("ROT_R_32_IMM_ALT", reg(registerIndexA), reg(registerIndexB), vx, result)
	vm.Ram.WriteRegister(registerIndexA, result)
	vm.pc += 1 + uint64(len(operands))
}

// Conditional move operations

func (vm *VM) handleCMOV_IZ_IMM(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx := extractTwoRegsOneImm(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	result := vx
	if valueB != 0 {
		result = valueA
	}
	dumpCmovOp("== 0", registerIndexA, registerIndexB, vx, valueA, result, true)
	vm.Ram.WriteRegister(registerIndexA, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleCMOV_NZ_IMM(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx := extractTwoRegsOneImm(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
	var result uint64
	if valueB != 0 {
		result = vx
	} else {
		result = valueA
	}
	dumpCmovOp("!= 0", registerIndexA, registerIndexB, vx, valueA, result, false)
	vm.Ram.WriteRegister(registerIndexA, result)
	vm.pc += 1 + uint64(len(operands))
}

// A.5.11 Two Registers and One Offset handlers

func (vm *VM) handleBRANCH_EQ(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx0 := extractTwoRegsOneOffset(operands)
	vx := uint64(int64(vm.pc) + int64(vx0))
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)

	taken := valueA == valueB
	dumpBranch("BRANCH_EQ", registerIndexA, registerIndexB, valueA, valueB, vx, taken)
	if taken {
		vm.branch(vx, true)
	} else {
		vm.pc += uint64(1 + len(operands))
	}
}

func (vm *VM) handleBRANCH_NE(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx0 := extractTwoRegsOneOffset(operands)
	vx := uint64(int64(vm.pc) + int64(vx0))
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)

	taken := valueA != valueB
	dumpBranch("BRANCH_NE", registerIndexA, registerIndexB, valueA, valueB, vx, taken)
	if taken {
		vm.branch(vx, true)
	} else {
		vm.pc += uint64(1 + len(operands))
	}
}

func (vm *VM) handleBRANCH_LT_U(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx0 := extractTwoRegsOneOffset(operands)
	vx := uint64(int64(vm.pc) + int64(vx0))
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)

	taken := valueA < valueB
	dumpBranch("BRANCH_LT_U", registerIndexA, registerIndexB, valueA, valueB, vx, taken)
	if taken {
		vm.branch(vx, true)
	} else {
		vm.pc += uint64(1 + len(operands))
	}
}

func (vm *VM) handleBRANCH_LT_S(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx0 := extractTwoRegsOneOffset(operands)
	vx := uint64(int64(vm.pc) + int64(vx0))
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)

	taken := int64(valueA) < int64(valueB)
	dumpBranch("BRANCH_LT_S", registerIndexA, registerIndexB, valueA, valueB, vx, taken)
	if taken {
		vm.branch(vx, true)
	} else {
		vm.pc += uint64(1 + len(operands))
	}
}

func (vm *VM) handleBRANCH_GE_U(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx0 := extractTwoRegsOneOffset(operands)
	vx := uint64(int64(vm.pc) + int64(vx0))
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)

	taken := valueA >= valueB
	dumpBranch("BRANCH_GE_U", registerIndexA, registerIndexB, valueA, valueB, vx, taken)
	if taken {
		vm.branch(vx, true)
	} else {
		vm.pc += uint64(1 + len(operands))
	}
}

func (vm *VM) handleBRANCH_GE_S(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx0 := extractTwoRegsOneOffset(operands)
	vx := uint64(int64(vm.pc) + int64(vx0))
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)

	taken := int64(valueA) >= int64(valueB)
	dumpBranch("BRANCH_GE_S", registerIndexA, registerIndexB, valueA, valueB, vx, taken)
	if taken {
		vm.branch(vx, true)
	} else {
		vm.pc += uint64(1 + len(operands))
	}
}
