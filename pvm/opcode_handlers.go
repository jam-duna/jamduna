package pvm

import (
	"encoding/binary"
)

func handleTRAP(vm *VM, operands []byte) {
	vm.panic(0xFF)
	vm.pc += 1
}

func handleFALLTHROUGH(vm *VM, operands []byte) {
	vm.pc += 1
}

func handleECALLI(vm *VM, operands []byte) {

	var lx uint32
	if len(operands) >= 8 {
		lx = uint32(binary.LittleEndian.Uint64(operands[:8]))
	} else {
		decoded := uint64(0)
		for i, b := range operands {
			decoded |= uint64(b) << (8 * i)
		}
		lx = uint32(decoded)
	}
	vm.hostCall = true
	vm.host_func_id = int(lx)
	vm.pc += 1 + uint64(len(operands))

	if vm.IsChild {
		return
	}
	vm.Gas -= int64(vm.chargeGas(vm.host_func_id))
	vm.InvokeHostCall(vm.host_func_id)
	vm.hostCall = false
}

func handleLOAD_IMM_64(vm *VM, operands []byte) {
	registerIndexA := min(12, int(operands[0])%16)
	lx := 8
	slice := operands[1 : 1+lx]
	var vx uint64
	if len(slice) >= 8 {
		vx = binary.LittleEndian.Uint64(slice[:8])
	} else {
		vx = 0
		for i, b := range slice {
			vx |= uint64(b) << (8 * i)
		}
	}
	if PvmTrace {
		dumpLoadImm("LOAD_IMM_64", registerIndexA, uint64(vx), vx, 64, false)
	}
	vm.register[registerIndexA] = uint64(vx)
	vm.pc += 1 + uint64(len(operands))
}

// A.5.5. Instructions with Arguments of One Offset. (JUMP)
func handleJUMP(vm *VM, operands []byte) {
	var vx int64
	lx := min(4, len(operands))
	if lx == 0 {
		vx = 0
	} else {

		slice := operands[0:lx]
		var decoded uint64
		if len(slice) >= 8 {
			decoded = binary.LittleEndian.Uint64(slice[:8])
		} else {
			decoded = 0
			for i, b := range slice {
				decoded |= uint64(b) << (8 * i)
			}
		}
		shift := 64 - 8*lx
		vx = int64(decoded<<shift) >> shift
	}

	if PvmTrace {
		dumpJumpOffset("JUMP", vx, vm.pc)
	}
	vm.branch(uint64(int64(vm.pc)+vx), true)
}

// A.5.12. Instructions with Arguments of Two Registers and Two Immediates. (LOAD_IMM_JUMP_IND)
// This instruction is used to load an immediate value into a register and then jump to an address.

func handleLOAD_IMM_JUMP_IND(vm *VM, operands []byte) {

	if len(operands) < 2 {
		return
	}

	firstByte := int(operands[0])
	registerIndexA := min(12, firstByte%16)
	registerIndexB := min(12, firstByte/16)

	lx := min(4, int(operands[1])%8)
	ly := min(4, max(0, len(operands)-lx-2))
	if ly == 0 {
		ly = 1
	}

	// Extract vx
	var vx uint64
	var vy uint64
	if 2+lx <= len(operands) {
		slice := operands[2 : 2+lx]
		var decoded uint64
		if len(slice) >= 8 {
			decoded = binary.LittleEndian.Uint64(slice[:8])
		} else {
			decoded = 0
			for i, b := range slice {
				decoded |= uint64(b) << (8 * i)
			}
		}
		shift := uint(64 - 8*lx)
		vx = uint64(int64(decoded<<shift) >> shift)
	} else {
		shift := uint(64 - 8*lx)
		vx = uint64(int64(0<<shift) >> shift)
	}

	// Read register B value BEFORE updating register A
	valueB := vm.register[registerIndexB]

	vm.register[registerIndexA] = vx

	// Extract vy
	if 2+lx+ly <= len(operands) {
		slice := operands[2+lx : 2+lx+ly]
		var decoded uint64
		if len(slice) >= 8 {
			decoded = binary.LittleEndian.Uint64(slice[:8])
		} else {
			decoded = 0
			for i, b := range slice {
				decoded |= uint64(b) << (8 * i)
			}
		}
		shift := uint(64 - 8*ly)
		vy = uint64(int64(decoded<<shift) >> shift)
	} else {
		shift := uint(64 - 8*ly)
		vy = uint64(int64(0<<shift) >> shift)
	}
	jumpAddr := (valueB + vy) & 0xFFFFFFFF
	vm.djump(jumpAddr)
}
