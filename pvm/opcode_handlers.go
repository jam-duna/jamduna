package pvm

import (
	"math/bits"

	"github.com/colorfulnotion/jam/types"
)

// A.5.7 One Register and Two Immediates handlers

func (vm *VM) handleSTORE_IMM_IND_U8(opcode byte, operands []byte) {
	registerIndexA, vx, vy := extractOneReg2Imm(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	addr := uint32(valueA) + uint32(vx)
	errCode := vm.Ram.WriteRAMBytes(addr, []byte{byte(uint8(vy))})
	dumpStoreGeneric("STORE_IMM_IND_U8", uint64(addr), "imm", vy&0xff, 8)
	if errCode != OK {
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		vm.terminated = true
		vm.Fault_address = uint32(errCode)
		return
	}
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleSTORE_IMM_IND_U16(opcode byte, operands []byte) {
	registerIndexA, vx, vy := extractOneReg2Imm(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	addr := uint32(valueA) + uint32(vx)
	errCode := vm.Ram.WriteRAMBytes(addr, types.E_l(vy%(1<<16), 2))
	dumpStoreGeneric("STORE_IMM_IND_U16", uint64(addr), "imm", vy%(1<<16), 16)
	if errCode != OK {
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		vm.terminated = true
		vm.Fault_address = uint32(errCode)
		return
	}
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleSTORE_IMM_IND_U32(opcode byte, operands []byte) {
	registerIndexA, vx, vy := extractOneReg2Imm(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	addr := uint32(valueA) + uint32(vx)
	errCode := vm.Ram.WriteRAMBytes(addr, types.E_l(vy%(1<<32), 4))
	dumpStoreGeneric("STORE_IMM_IND_U32", uint64(addr), "imm", vy%(1<<32), 32)
	if errCode != OK {
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		vm.terminated = true
		vm.Fault_address = uint32(errCode)
		return
	}
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleSTORE_IMM_IND_U64(opcode byte, operands []byte) {
	registerIndexA, vx, vy := extractOneReg2Imm(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	addr := uint32(valueA) + uint32(vx)
	errCode := vm.Ram.WriteRAMBytes(addr, types.E_l(uint64(vy), 8))
	dumpStoreGeneric("STORE_IMM_IND_U64", uint64(addr), "imm", vy, 64)
	if errCode != OK {
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		vm.terminated = true
		vm.Fault_address = uint32(errCode)
		return
	}
	vm.pc += 1 + uint64(len(operands))
}

// A.5.8 One Register, One Immediate and One Offset handlers

func (vm *VM) handleLOAD_IMM_JUMP(opcode byte, operands []byte) {
	registerIndexA, vx, vy0 := extractOneRegOneImmOneOffset(operands)
	vy := uint64(int64(vm.pc) + vy0)
	vm.Ram.WriteRegister(registerIndexA, vx)
	dumpLoadImmJump("LOAD_IMM_JUMP", registerIndexA, vx)
	vm.branch(vy, true)
}

func (vm *VM) handleBRANCH_EQ_IMM(opcode byte, operands []byte) {
	registerIndexA, vx, vy0 := extractOneRegOneImmOneOffset(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	vy := uint64(int64(vm.pc) + vy0)
	taken := valueA == vx
	dumpBranchImm("BRANCH_EQ_IMM", registerIndexA, valueA, vx, vy, false, taken)
	if taken {
		vm.branch(vy, true)
	} else {
		vm.pc += uint64(1 + len(operands))
	}
}

func (vm *VM) handleBRANCH_NE_IMM(opcode byte, operands []byte) {
	registerIndexA, vx, vy0 := extractOneRegOneImmOneOffset(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	vy := uint64(int64(vm.pc) + vy0)
	taken := valueA != vx
	dumpBranchImm("BRANCH_NE_IMM", registerIndexA, valueA, vx, vy, false, taken)
	if taken {
		vm.branch(vy, true)
	} else {
		vm.pc += uint64(1 + len(operands))
	}
}

func (vm *VM) handleBRANCH_LT_U_IMM(opcode byte, operands []byte) {
	registerIndexA, vx, vy0 := extractOneRegOneImmOneOffset(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	vy := uint64(int64(vm.pc) + vy0)
	taken := valueA < vx
	dumpBranchImm("BRANCH_LT_U_IMM", registerIndexA, valueA, vx, vy, false, taken)
	if taken {
		vm.branch(vy, true)
	} else {
		vm.pc += uint64(1 + len(operands))
	}
}

func (vm *VM) handleBRANCH_LE_U_IMM(opcode byte, operands []byte) {
	registerIndexA, vx, vy0 := extractOneRegOneImmOneOffset(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	vy := uint64(int64(vm.pc) + vy0)
	taken := valueA <= vx
	dumpBranchImm("BRANCH_LE_U_IMM", registerIndexA, valueA, vx, vy, false, taken)
	if taken {
		vm.branch(vy, true)
	} else {
		vm.pc += uint64(1 + len(operands))
	}
}

func (vm *VM) handleBRANCH_GE_U_IMM(opcode byte, operands []byte) {
	registerIndexA, vx, vy0 := extractOneRegOneImmOneOffset(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	vy := uint64(int64(vm.pc) + vy0)
	taken := valueA >= vx
	dumpBranchImm("BRANCH_GE_U_IMM", registerIndexA, valueA, vx, vy, false, taken)
	if taken {
		vm.branch(vy, true)
	} else {
		vm.pc += uint64(1 + len(operands))
	}
}

func (vm *VM) handleBRANCH_GT_U_IMM(opcode byte, operands []byte) {
	registerIndexA, vx, vy0 := extractOneRegOneImmOneOffset(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	vy := uint64(int64(vm.pc) + vy0)
	taken := valueA > vx
	dumpBranchImm("BRANCH_GT_U_IMM", registerIndexA, valueA, vx, vy, false, taken)
	if taken {
		vm.branch(vy, true)
	} else {
		vm.pc += uint64(1 + len(operands))
	}
}

func (vm *VM) handleBRANCH_LT_S_IMM(opcode byte, operands []byte) {
	registerIndexA, vx, vy0 := extractOneRegOneImmOneOffset(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	vy := uint64(int64(vm.pc) + vy0)
	taken := int64(valueA) < int64(vx)
	dumpBranchImm("BRANCH_LT_S_IMM", registerIndexA, valueA, vx, vy, true, taken)
	if taken {
		vm.branch(vy, true)
	} else {
		vm.pc += uint64(1 + len(operands))
	}
}

func (vm *VM) handleBRANCH_LE_S_IMM(opcode byte, operands []byte) {
	registerIndexA, vx, vy0 := extractOneRegOneImmOneOffset(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	vy := uint64(int64(vm.pc) + vy0)
	taken := int64(valueA) <= int64(vx)
	dumpBranchImm("BRANCH_LE_S_IMM", registerIndexA, valueA, vx, vy, true, taken)
	if taken {
		vm.branch(vy, true)
	} else {
		vm.pc += uint64(1 + len(operands))
	}
}

func (vm *VM) handleBRANCH_GE_S_IMM(opcode byte, operands []byte) {
	registerIndexA, vx, vy0 := extractOneRegOneImmOneOffset(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	vy := uint64(int64(vm.pc) + vy0)
	taken := int64(valueA) >= int64(vx)
	dumpBranchImm("BRANCH_GE_S_IMM", registerIndexA, valueA, vx, vy, true, taken)
	if taken {
		vm.branch(vy, true)
	} else {
		vm.pc += uint64(1 + len(operands))
	}
}

func (vm *VM) handleBRANCH_GT_S_IMM(opcode byte, operands []byte) {
	registerIndexA, vx, vy0 := extractOneRegOneImmOneOffset(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	vy := uint64(int64(vm.pc) + vy0)
	taken := int64(valueA) > int64(vx)
	dumpBranchImm("BRANCH_GT_S_IMM", registerIndexA, valueA, vx, vy, true, taken)
	if taken {
		vm.branch(vy, true)
	} else {
		vm.pc += uint64(1 + len(operands))
	}
}

// A.5.9 Two Registers handlers

func (vm *VM) handleMOVE_REG(opcode byte, operands []byte) {
	registerIndexD, registerIndexA := extractTwoRegisters(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	dumpMov(registerIndexD, registerIndexA, valueA)
	vm.Ram.WriteRegister(registerIndexD, valueA)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleSBRK(opcode byte, operands []byte) {
	registerIndexD, registerIndexA := extractTwoRegisters(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)

	if valueA == 0 {
		vm.Ram.WriteRegister(registerIndexD, uint64(vm.Ram.GetCurrentHeapPointer()))
		vm.pc += 1 + uint64(len(operands))
		return
	}

	result := uint64(vm.Ram.GetCurrentHeapPointer())
	next_page_boundary := P_func(vm.Ram.GetCurrentHeapPointer())
	new_heap_pointer := uint64(vm.Ram.GetCurrentHeapPointer()) + valueA

	if new_heap_pointer > uint64(next_page_boundary) {
		final_boundary := P_func(uint32(new_heap_pointer))
		idx_start := next_page_boundary / Z_P
		idx_end := final_boundary / Z_P
		page_count := idx_end - idx_start

		vm.Ram.allocatePages(idx_start, page_count)
	}
	vm.Ram.SetCurrentHeapPointer(uint32(new_heap_pointer))
	dumpTwoRegs("SBRK", registerIndexD, registerIndexA, valueA, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleCOUNT_SET_BITS_64(opcode byte, operands []byte) {
	registerIndexD, registerIndexA := extractTwoRegisters(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	result := uint64(bits.OnesCount64(valueA))
	dumpTwoRegs("COUNT_SET_BITS_64", registerIndexD, registerIndexA, valueA, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleCOUNT_SET_BITS_32(opcode byte, operands []byte) {
	registerIndexD, registerIndexA := extractTwoRegisters(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	result := uint64(bits.OnesCount32(uint32(valueA)))
	dumpTwoRegs("COUNT_SET_BITS_32", registerIndexD, registerIndexA, valueA, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleLEADING_ZERO_BITS_64(opcode byte, operands []byte) {
	registerIndexD, registerIndexA := extractTwoRegisters(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	result := uint64(bits.LeadingZeros64(valueA))
	dumpTwoRegs("LEADING_ZERO_BITS_64", registerIndexD, registerIndexA, valueA, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleLEADING_ZERO_BITS_32(opcode byte, operands []byte) {
	registerIndexD, registerIndexA := extractTwoRegisters(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	result := uint64(bits.LeadingZeros32(uint32(valueA)))
	dumpTwoRegs("LEADING_ZERO_BITS_32", registerIndexD, registerIndexA, valueA, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleTRAILING_ZERO_BITS_64(opcode byte, operands []byte) {
	registerIndexD, registerIndexA := extractTwoRegisters(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	result := uint64(bits.TrailingZeros64(valueA))
	dumpTwoRegs("TRAILING_ZERO_BITS_64", registerIndexD, registerIndexA, valueA, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleTRAILING_ZERO_BITS_32(opcode byte, operands []byte) {
	registerIndexD, registerIndexA := extractTwoRegisters(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	result := uint64(bits.TrailingZeros32(uint32(valueA)))
	dumpTwoRegs("TRAILING_ZERO_BITS_32", registerIndexD, registerIndexA, valueA, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleSIGN_EXTEND_8(opcode byte, operands []byte) {
	registerIndexD, registerIndexA := extractTwoRegisters(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	result := uint64(int8(valueA & 0xFF))
	dumpTwoRegs("SIGN_EXTEND_8", registerIndexD, registerIndexA, valueA, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleSIGN_EXTEND_16(opcode byte, operands []byte) {
	registerIndexD, registerIndexA := extractTwoRegisters(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	result := uint64(int16(valueA & 0xFFFF))
	dumpTwoRegs("SIGN_EXTEND_16", registerIndexD, registerIndexA, valueA, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleZERO_EXTEND_16(opcode byte, operands []byte) {
	registerIndexD, registerIndexA := extractTwoRegisters(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	result := valueA & 0xFFFF
	dumpTwoRegs("ZERO_EXTEND_16", registerIndexD, registerIndexA, valueA, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleREVERSE_BYTES(opcode byte, operands []byte) {
	registerIndexD, registerIndexA := extractTwoRegisters(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	result := bits.ReverseBytes64(valueA)
	dumpTwoRegs("REVERSE_BYTES", registerIndexD, registerIndexA, valueA, result)
	vm.Ram.WriteRegister(registerIndexD, result)
	vm.pc += 1 + uint64(len(operands))
}

// A.5.12 Two Registers and Two Immediates handler

func (vm *VM) handleLOAD_IMM_JUMP_IND(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx, vy := extractTwoRegsAndTwoImmediates(operands)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)

	vm.Ram.WriteRegister(registerIndexA, vx)
	vm.djump((valueB + vy) % (1 << 32))
}
