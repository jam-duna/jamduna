package pvm

import "math/bits"

// A.5.9. Instructions with Arguments of Two Registers.

func handleMOVE_REG(vm *VM, operands []byte) {
	// Inline register extraction with minimal bounds checking
	regByte := operands[0]
	registerIndexD := int(regByte & 0x0F)
	registerIndexA := int(regByte >> 4)

	// Use branchless bounds check with bitwise operations
	// Since indices are 4-bit (0-15), we only need to clamp values > 12
	if registerIndexD > 12 {
		registerIndexD = 12
	}
	if registerIndexA > 12 {
		registerIndexA = 12
	}

	vm.register[registerIndexD] = vm.register[registerIndexA]
	if PvmTrace {
		dumpMov(registerIndexD, registerIndexA, vm.register[registerIndexA])
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleSBRK(vm *VM, operands []byte) {
	// Inline register extraction
	regByte := operands[0]
	registerIndexD := int(regByte & 0x0F)
	registerIndexA := int(regByte >> 4)
	if registerIndexD > 12 { registerIndexD = 12 }
	if registerIndexA > 12 { registerIndexA = 12 }
	
	valueA := vm.register[registerIndexA]

	if valueA == 0 {
		vm.register[registerIndexD] = uint64(vm.GetCurrentHeapPointer())
		vm.pc += 1 + uint64(len(operands))
		return
	}

	result := uint64(vm.GetCurrentHeapPointer())
	next_page_boundary := P_func(vm.GetCurrentHeapPointer())
	new_heap_pointer := uint64(vm.GetCurrentHeapPointer()) + valueA

	if new_heap_pointer > uint64(next_page_boundary) {
		final_boundary := P_func(uint32(new_heap_pointer))
		idx_start := next_page_boundary / Z_P
		idx_end := final_boundary / Z_P
		page_count := idx_end - idx_start

		vm.allocatePages(idx_start, page_count)
	}
	vm.SetCurrentHeapPointer(uint32(new_heap_pointer))
	if PvmTrace {
		dumpTwoRegs("SBRK", registerIndexD, registerIndexA, valueA, result)
	}
	vm.register[registerIndexD] = result
	vm.pc += 1 + uint64(len(operands))
}

func handleCOUNT_SET_BITS_64(vm *VM, operands []byte) {
	// Inline register extraction
	regByte := operands[0]
	registerIndexD := int(regByte & 0x0F)
	registerIndexA := int(regByte >> 4)
	if registerIndexD > 12 { registerIndexD = 12 }
	if registerIndexA > 12 { registerIndexA = 12 }
	
	valueA := vm.register[registerIndexA]
	result := uint64(bits.OnesCount64(valueA))
	if PvmTrace {
		dumpTwoRegs("COUNT_SET_BITS_64", registerIndexD, registerIndexA, valueA, result)
	}
	vm.register[registerIndexD] = result
	vm.pc += 1 + uint64(len(operands))
}

func handleCOUNT_SET_BITS_32(vm *VM, operands []byte) {
	// Inline register extraction
	regByte := operands[0]
	registerIndexD := int(regByte & 0x0F)
	registerIndexA := int(regByte >> 4)
	if registerIndexD > 12 { registerIndexD = 12 }
	if registerIndexA > 12 { registerIndexA = 12 }
	
	valueA := vm.register[registerIndexA]
	result := uint64(bits.OnesCount32(uint32(valueA)))
	if PvmTrace {
		dumpTwoRegs("COUNT_SET_BITS_32", registerIndexD, registerIndexA, valueA, result)
	}
	vm.register[registerIndexD] = result
	vm.pc += 1 + uint64(len(operands))
}

func handleLEADING_ZERO_BITS_64(vm *VM, operands []byte) {
	// Inline register extraction
	regByte := operands[0]
	registerIndexD := int(regByte & 0x0F)
	registerIndexA := int(regByte >> 4)
	if registerIndexD > 12 { registerIndexD = 12 }
	if registerIndexA > 12 { registerIndexA = 12 }
	
	valueA := vm.register[registerIndexA]
	result := uint64(bits.LeadingZeros64(valueA))
	if PvmTrace {
		dumpTwoRegs("LEADING_ZERO_BITS_64", registerIndexD, registerIndexA, valueA, result)
	}
	vm.register[registerIndexD] = result
	vm.pc += 1 + uint64(len(operands))
}

func handleLEADING_ZERO_BITS_32(vm *VM, operands []byte) {
	// Inline register extraction
	regByte := operands[0]
	registerIndexD := int(regByte & 0x0F)
	registerIndexA := int(regByte >> 4)
	if registerIndexD > 12 { registerIndexD = 12 }
	if registerIndexA > 12 { registerIndexA = 12 }
	
	valueA := vm.register[registerIndexA]
	result := uint64(bits.LeadingZeros32(uint32(valueA)))
	if PvmTrace {
		dumpTwoRegs("LEADING_ZERO_BITS_32", registerIndexD, registerIndexA, valueA, result)
	}
	vm.register[registerIndexD] = result
	vm.pc += 1 + uint64(len(operands))
}

func handleTRAILING_ZERO_BITS_64(vm *VM, operands []byte) {
	// Inline register extraction
	regByte := operands[0]
	registerIndexD := int(regByte & 0x0F)
	registerIndexA := int(regByte >> 4)
	if registerIndexD > 12 { registerIndexD = 12 }
	if registerIndexA > 12 { registerIndexA = 12 }
	
	valueA := vm.register[registerIndexA]
	result := uint64(bits.TrailingZeros64(valueA))
	if PvmTrace {
		dumpTwoRegs("TRAILING_ZERO_BITS_64", registerIndexD, registerIndexA, valueA, result)
	}
	vm.register[registerIndexD] = result
	vm.pc += 1 + uint64(len(operands))
}

func handleTRAILING_ZERO_BITS_32(vm *VM, operands []byte) {
	// Inline register extraction
	regByte := operands[0]
	registerIndexD := int(regByte & 0x0F)
	registerIndexA := int(regByte >> 4)
	if registerIndexD > 12 { registerIndexD = 12 }
	if registerIndexA > 12 { registerIndexA = 12 }
	
	valueA := vm.register[registerIndexA]
	result := uint64(bits.TrailingZeros32(uint32(valueA)))
	if PvmTrace {
		dumpTwoRegs("TRAILING_ZERO_BITS_32", registerIndexD, registerIndexA, valueA, result)
	}
	vm.register[registerIndexD] = result
	vm.pc += 1 + uint64(len(operands))
}

func handleSIGN_EXTEND_8(vm *VM, operands []byte) {
	// Inline register extraction
	regByte := operands[0]
	registerIndexD := int(regByte & 0x0F)
	registerIndexA := int(regByte >> 4)
	if registerIndexD > 12 { registerIndexD = 12 }
	if registerIndexA > 12 { registerIndexA = 12 }
	
	valueA := vm.register[registerIndexA]
	result := uint64(int8(valueA & 0xFF))
	if PvmTrace {
		dumpTwoRegs("SIGN_EXTEND_8", registerIndexD, registerIndexA, valueA, result)
	}
	vm.register[registerIndexD] = result
	vm.pc += 1 + uint64(len(operands))
}

func handleSIGN_EXTEND_16(vm *VM, operands []byte) {
	// Inline register extraction
	regByte := operands[0]
	registerIndexD := int(regByte & 0x0F)
	registerIndexA := int(regByte >> 4)
	if registerIndexD > 12 { registerIndexD = 12 }
	if registerIndexA > 12 { registerIndexA = 12 }
	
	valueA := vm.register[registerIndexA]
	result := uint64(int16(valueA & 0xFFFF))
	if PvmTrace {
		dumpTwoRegs("SIGN_EXTEND_16", registerIndexD, registerIndexA, valueA, result)
	}
	vm.register[registerIndexD] = result
	vm.pc += 1 + uint64(len(operands))
}

func handleZERO_EXTEND_16(vm *VM, operands []byte) {
	// Inline register extraction
	regByte := operands[0]
	registerIndexD := int(regByte & 0x0F)
	registerIndexA := int(regByte >> 4)
	if registerIndexD > 12 { registerIndexD = 12 }
	if registerIndexA > 12 { registerIndexA = 12 }
	
	valueA := vm.register[registerIndexA]
	result := valueA & 0xFFFF
	if PvmTrace {
		dumpTwoRegs("ZERO_EXTEND_16", registerIndexD, registerIndexA, valueA, result)
	}
	vm.register[registerIndexD] = result
	vm.pc += 1 + uint64(len(operands))
}

func handleREVERSE_BYTES(vm *VM, operands []byte) {
	// Inline register extraction
	regByte := operands[0]
	registerIndexD := int(regByte & 0x0F)
	registerIndexA := int(regByte >> 4)
	if registerIndexD > 12 { registerIndexD = 12 }
	if registerIndexA > 12 { registerIndexA = 12 }
	
	valueA := vm.register[registerIndexA]
	result := bits.ReverseBytes64(valueA)
	if PvmTrace {
		dumpTwoRegs("REVERSE_BYTES", registerIndexD, registerIndexA, valueA, result)
	}
	vm.register[registerIndexD] = result
	vm.pc += 1 + uint64(len(operands))
}
