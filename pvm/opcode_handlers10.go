package pvm

import (
	"encoding/binary"
	"math/bits"
)

// A.5.10. Instructions with Arguments of Two Registers and One Immediate.

func handleSTORE_IND_U8(vm *VM, operands []byte) {
	var registerIndexA, registerIndexB int
	var vx uint64
	if len(operands) == 0 {
		registerIndexA, registerIndexB, vx = 0, 0, 0
	} else {
		firstByte := operands[0]
		registerIndexA = min(12, int(firstByte&0x0F))
		registerIndexB = min(12, int(firstByte)/16)

		lx := min(4, max(0, len(operands)-1))
		if lx > 0 && 1+lx <= len(operands) {
			slice := operands[1 : 1+lx]
			var decoded uint64
			switch len(slice) {
			case 1:
				decoded = uint64(slice[0])
			case 2:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8
			case 3:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16
			case 4:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16 | uint64(slice[3])<<24
			default:
				if len(slice) >= 8 {
					decoded = binary.LittleEndian.Uint64(slice[:8])
				} else {
					decoded = 0
					for i, b := range slice {
						decoded |= uint64(b) << (8 * i)
					}
				}
			}
			shift := uint(64 - 8*lx)
			vx = uint64(int64(decoded<<shift) >> shift)
		}
	}

	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	addr := uint32(valueB + vx)
	value := uint8(valueA)
	errCode := vm.WriteRAMBytes8(addr, value)

	if errCode != OK {
		vm.panic(errCode)
		return
	}
	if PvmTrace {
		dumpStoreGeneric("STORE_IND_U8", uint64(addr), reg(registerIndexA), uint64(value), 8)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleSTORE_IND_U16(vm *VM, operands []byte) {
	var registerIndexA, registerIndexB int
	var vx uint64
	if len(operands) == 0 {
		registerIndexA, registerIndexB, vx = 0, 0, 0
	} else {
		firstByte := operands[0]
		registerIndexA = min(12, int(firstByte&0x0F))
		registerIndexB = min(12, int(firstByte)/16)

		lx := min(4, max(0, len(operands)-1))
		if lx > 0 && 1+lx <= len(operands) {
			slice := operands[1 : 1+lx]
			var decoded uint64
			switch len(slice) {
			case 1:
				decoded = uint64(slice[0])
			case 2:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8
			case 3:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16
			case 4:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16 | uint64(slice[3])<<24
			default:
				if len(slice) >= 8 {
					decoded = binary.LittleEndian.Uint64(slice[:8])
				} else {
					decoded = 0
					for i, b := range slice {
						decoded |= uint64(b) << (8 * i)
					}
				}
			}
			shift := uint(64 - 8*lx)
			vx = uint64(int64(decoded<<shift) >> shift)
		}
	}

	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	addr := uint32(valueB + vx)
	value := uint16(valueA)
	errCode := vm.WriteRAMBytes16(addr, value)
	if errCode != OK {
		vm.panic(errCode)
		return
	}
	if PvmTrace {
		dumpStoreGeneric("STORE_IND_U16", uint64(addr), reg(registerIndexA), uint64(value), 16)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleSTORE_IND_U32(vm *VM, operands []byte) {
	var registerIndexA, registerIndexB int
	var vx uint64
	if len(operands) == 0 {
		registerIndexA, registerIndexB, vx = 0, 0, 0
	} else {
		firstByte := operands[0]
		registerIndexA = min(12, int(firstByte&0x0F))
		registerIndexB = min(12, int(firstByte)/16)

		lx := min(4, max(0, len(operands)-1))
		if lx > 0 && 1+lx <= len(operands) {
			slice := operands[1 : 1+lx]
			var decoded uint64
			switch len(slice) {
			case 1:
				decoded = uint64(slice[0])
			case 2:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8
			case 3:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16
			case 4:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16 | uint64(slice[3])<<24
			default:
				if len(slice) >= 8 {
					decoded = binary.LittleEndian.Uint64(slice[:8])
				} else {
					decoded = 0
					for i, b := range slice {
						decoded |= uint64(b) << (8 * i)
					}
				}
			}
			shift := uint(64 - 8*lx)
			vx = uint64(int64(decoded<<shift) >> shift)
		}
	}

	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	addr := uint32(valueB + vx)
	value := uint32(valueA)
	errCode := vm.WriteRAMBytes32(addr, value)
	if errCode != OK {
		vm.panic(errCode)
		return
	}
	if PvmTrace {
		dumpStoreGeneric("STORE_IND_U32", uint64(addr), reg(registerIndexA), uint64(value), 32)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleSTORE_IND_U64(vm *VM, operands []byte) {
	var registerIndexA, registerIndexB int
	var vx uint64
	if len(operands) == 0 {
		registerIndexA, registerIndexB, vx = 0, 0, 0
	} else {
		firstByte := operands[0]
		registerIndexA = min(12, int(firstByte&0x0F))
		registerIndexB = min(12, int(firstByte)/16)

		lx := min(4, max(0, len(operands)-1))
		if lx > 0 && 1+lx <= len(operands) {
			slice := operands[1 : 1+lx]
			var decoded uint64
			switch len(slice) {
			case 1:
				decoded = uint64(slice[0])
			case 2:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8
			case 3:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16
			case 4:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16 | uint64(slice[3])<<24
			default:
				if len(slice) >= 8 {
					decoded = binary.LittleEndian.Uint64(slice[:8])
				} else {
					decoded = 0
					for i, b := range slice {
						decoded |= uint64(b) << (8 * i)
					}
				}
			}
			shift := uint(64 - 8*lx)
			vx = uint64(int64(decoded<<shift) >> shift)
		}
	}

	valueA := vm.register[registerIndexA]
	addr := uint32(vm.register[registerIndexB] + vx)
	errCode := vm.WriteRAMBytes64(addr, valueA)
	if errCode != OK {
		vm.panic(errCode)
		return
	}
	if PvmTrace {
		dumpStoreGeneric("STORE_IND_U64", uint64(addr), reg(registerIndexA), valueA, 64)
	}
	vm.pc += 1 + uint64(len(operands))
}

// LOAD operations

func handleLOAD_IND_U8(vm *VM, operands []byte) {
	var registerIndexA, registerIndexB int
	var vx uint64
	if len(operands) == 0 {
		registerIndexA, registerIndexB, vx = 0, 0, 0
	} else {
		firstByte := operands[0]
		registerIndexA = min(12, int(firstByte&0x0F))
		registerIndexB = min(12, int(firstByte)/16)

		lx := min(4, max(0, len(operands)-1))
		if lx > 0 && 1+lx <= len(operands) {
			slice := operands[1 : 1+lx]
			var decoded uint64
			switch len(slice) {
			case 1:
				decoded = uint64(slice[0])
			case 2:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8
			case 3:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16
			case 4:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16 | uint64(slice[3])<<24
			default:
				if len(slice) >= 8 {
					decoded = binary.LittleEndian.Uint64(slice[:8])
				} else {
					decoded = 0
					for i, b := range slice {
						decoded |= uint64(b) << (8 * i)
					}
				}
			}
			shift := uint(64 - 8*lx)
			vx = uint64(int64(decoded<<shift) >> shift)
		}
	}

	valueB := vm.register[registerIndexB]
	addr := uint32(valueB + vx)
	value, errCode := vm.ReadRAMBytes8(addr)
	if errCode != OK {
		vm.panic(errCode)
		return
	}
	result := uint64(value)
	vm.register[registerIndexA] = result
	if PvmTrace {
		dumpLoadGeneric("LOAD_IND_U8", registerIndexA, uint64(addr), result, 8, false)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleLOAD_IND_I8(vm *VM, operands []byte) {
	var registerIndexA, registerIndexB int
	var vx uint64
	if len(operands) == 0 {
		registerIndexA, registerIndexB, vx = 0, 0, 0
	} else {
		firstByte := operands[0]
		registerIndexA = min(12, int(firstByte&0x0F))
		registerIndexB = min(12, int(firstByte)/16)

		lx := min(4, max(0, len(operands)-1))
		if lx > 0 && 1+lx <= len(operands) {
			slice := operands[1 : 1+lx]
			var decoded uint64
			switch len(slice) {
			case 1:
				decoded = uint64(slice[0])
			case 2:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8
			case 3:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16
			case 4:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16 | uint64(slice[3])<<24
			default:
				if len(slice) >= 8 {
					decoded = binary.LittleEndian.Uint64(slice[:8])
				} else {
					decoded = 0
					for i, b := range slice {
						decoded |= uint64(b) << (8 * i)
					}
				}
			}
			shift := uint(64 - 8*lx)
			vx = uint64(int64(decoded<<shift) >> shift)
		}
	}

	valueB := vm.register[registerIndexB]
	addr := uint32(valueB + vx)
	value, errCode := vm.ReadRAMBytes8(addr)
	if errCode != OK {
		vm.panic(errCode)
		return
	}
	result := uint64(int8(value))
	vm.register[registerIndexA] = result
	if PvmTrace {
		dumpLoadGeneric("LOAD_IND_I8", registerIndexA, uint64(addr), result, 8, true)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleLOAD_IND_U16(vm *VM, operands []byte) {
	var registerIndexA, registerIndexB int
	var vx uint64
	if len(operands) == 0 {
		registerIndexA, registerIndexB, vx = 0, 0, 0
	} else {
		firstByte := operands[0]
		registerIndexA = min(12, int(firstByte&0x0F))
		registerIndexB = min(12, int(firstByte)/16)

		lx := min(4, max(0, len(operands)-1))
		if lx > 0 && 1+lx <= len(operands) {
			slice := operands[1 : 1+lx]
			var decoded uint64
			switch len(slice) {
			case 1:
				decoded = uint64(slice[0])
			case 2:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8
			case 3:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16
			case 4:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16 | uint64(slice[3])<<24
			default:
				if len(slice) >= 8 {
					decoded = binary.LittleEndian.Uint64(slice[:8])
				} else {
					decoded = 0
					for i, b := range slice {
						decoded |= uint64(b) << (8 * i)
					}
				}
			}
			shift := uint(64 - 8*lx)
			vx = uint64(int64(decoded<<shift) >> shift)
		}
	}

	valueB := vm.register[registerIndexB]
	addr := uint32(valueB + vx)
	value, errCode := vm.ReadRAMBytes16(addr)
	if errCode != OK {
		vm.panic(errCode)
		return
	}
	result := uint64(value)
	vm.register[registerIndexA] = result
	if PvmTrace {
		dumpLoadGeneric("LOAD_IND_U16", registerIndexA, uint64(addr), result, 16, false)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleLOAD_IND_I16(vm *VM, operands []byte) {
	var registerIndexA, registerIndexB int
	var vx uint64
	if len(operands) == 0 {
		registerIndexA, registerIndexB, vx = 0, 0, 0
	} else {
		firstByte := operands[0]
		registerIndexA = min(12, int(firstByte&0x0F))
		registerIndexB = min(12, int(firstByte)/16)

		lx := min(4, max(0, len(operands)-1))
		if lx > 0 && 1+lx <= len(operands) {
			slice := operands[1 : 1+lx]
			var decoded uint64
			switch len(slice) {
			case 1:
				decoded = uint64(slice[0])
			case 2:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8
			case 3:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16
			case 4:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16 | uint64(slice[3])<<24
			default:
				if len(slice) >= 8 {
					decoded = binary.LittleEndian.Uint64(slice[:8])
				} else {
					decoded = 0
					for i, b := range slice {
						decoded |= uint64(b) << (8 * i)
					}
				}
			}
			shift := uint(64 - 8*lx)
			vx = uint64(int64(decoded<<shift) >> shift)
		}
	}

	valueB := vm.register[registerIndexB]
	addr := uint32(valueB + vx)
	value, errCode := vm.ReadRAMBytes16(addr)
	if errCode != OK {
		vm.panic(errCode)
		return
	}
	result := uint64(int16(value))
	vm.register[registerIndexA] = result
	if PvmTrace {
		dumpLoadGeneric("LOAD_IND_I16", registerIndexA, uint64(addr), result, 16, true)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleLOAD_IND_U32(vm *VM, operands []byte) {
	var registerIndexA, registerIndexB int
	var vx uint64
	if len(operands) == 0 {
		registerIndexA, registerIndexB, vx = 0, 0, 0
	} else {
		firstByte := operands[0]
		registerIndexA = min(12, int(firstByte&0x0F))
		registerIndexB = min(12, int(firstByte)/16)

		lx := min(4, max(0, len(operands)-1))
		if lx > 0 && 1+lx <= len(operands) {
			slice := operands[1 : 1+lx]
			var decoded uint64
			switch len(slice) {
			case 1:
				decoded = uint64(slice[0])
			case 2:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8
			case 3:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16
			case 4:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16 | uint64(slice[3])<<24
			default:
				if len(slice) >= 8 {
					decoded = binary.LittleEndian.Uint64(slice[:8])
				} else {
					decoded = 0
					for i, b := range slice {
						decoded |= uint64(b) << (8 * i)
					}
				}
			}
			shift := uint(64 - 8*lx)
			vx = uint64(int64(decoded<<shift) >> shift)
		}
	}

	valueB := vm.register[registerIndexB]
	addr := uint32(valueB + vx)
	value, errCode := vm.ReadRAMBytes32(addr)
	if errCode != OK {
		vm.panic(errCode)
		return
	}
	result := uint64(value)
	vm.register[registerIndexA] = result
	if PvmTrace {
		dumpLoadGeneric("LOAD_IND_U32", registerIndexA, uint64(addr), result, 32, false)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleLOAD_IND_I32(vm *VM, operands []byte) {
	var registerIndexA, registerIndexB int
	var vx uint64
	if len(operands) == 0 {
		registerIndexA, registerIndexB, vx = 0, 0, 0
	} else {
		firstByte := operands[0]
		registerIndexA = min(12, int(firstByte&0x0F))
		registerIndexB = min(12, int(firstByte)/16)

		lx := min(4, max(0, len(operands)-1))
		if lx > 0 && 1+lx <= len(operands) {
			slice := operands[1 : 1+lx]
			var decoded uint64
			switch len(slice) {
			case 1:
				decoded = uint64(slice[0])
			case 2:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8
			case 3:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16
			case 4:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16 | uint64(slice[3])<<24
			default:
				if len(slice) >= 8 {
					decoded = binary.LittleEndian.Uint64(slice[:8])
				} else {
					decoded = 0
					for i, b := range slice {
						decoded |= uint64(b) << (8 * i)
					}
				}
			}
			shift := uint(64 - 8*lx)
			vx = uint64(int64(decoded<<shift) >> shift)
		}
	}

	valueB := vm.register[registerIndexB]
	addr := uint32(valueB + vx)
	value, errCode := vm.ReadRAMBytes32(addr)
	if errCode != OK {
		vm.panic(errCode)
		return
	}
	result := uint64(int32(value))
	vm.register[registerIndexA] = result
	if PvmTrace {
		dumpLoadGeneric("LOAD_IND_I32", registerIndexA, uint64(addr), result, 32, true)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleLOAD_IND_U64(vm *VM, operands []byte) {
	registerIndexA := int(operands[0] & 0x0F)
	registerIndexB := int(operands[0] >> 4)

	operandsLen := len(operands)
	operandLen := operandsLen - 1
	if operandLen > 8 {
		operandLen = 8
	}
	var offset uint64
	switch operandLen {
	case 0:
		offset = 0
	case 1:
		// Sign extend 8-bit to 64-bit
		offset = uint64(int64(int8(operands[1])))
	case 2:
		// Sign extend 16-bit to 64-bit
		rawValue := uint16(operands[1]) | uint16(operands[2])<<8
		offset = uint64(int64(int16(rawValue)))
	case 4:
		// Sign extend 32-bit to 64-bit
		rawValue := uint32(operands[1]) | uint32(operands[2])<<8 |
			uint32(operands[3])<<16 | uint32(operands[4])<<24
		offset = uint64(int64(int32(rawValue)))
	case 8:
		offset = binary.LittleEndian.Uint64(operands[1:9])
	default:
		var rawValue uint64
		for i := 0; i < operandLen; i++ {
			rawValue |= uint64(operands[1+i]) << (8 * i)
		}
		// Sign extend to 64-bit
		shift := uint(64 - 8*operandLen)
		offset = uint64(int64(rawValue<<shift) >> shift)
	}

	memoryAddress := uint32(vm.register[registerIndexB] + offset)
	loadedValue, errCode := vm.ReadRAMBytes64(memoryAddress)
	if errCode != OK {
		vm.panic(errCode)
		return
	}
	vm.register[registerIndexA] = loadedValue

	if PvmTrace {
		dumpLoadGeneric("LOAD_IND_U64", registerIndexA, uint64(memoryAddress), loadedValue, 64, false)
	}

	vm.pc += 1 + uint64(operandsLen)
}

// Arithmetic operations

func handleADD_IMM_32(vm *VM, operands []byte) {
	var registerIndexA, registerIndexB int
	var vx uint64
	if len(operands) == 0 {
		registerIndexA, registerIndexB, vx = 0, 0, 0
	} else {
		firstByte := operands[0]
		registerIndexA = min(12, int(firstByte&0x0F))
		registerIndexB = min(12, int(firstByte)/16)

		lx := min(4, max(0, len(operands)-1))
		if lx > 0 && 1+lx <= len(operands) {
			slice := operands[1 : 1+lx]
			var decoded uint64
			switch len(slice) {
			case 1:
				decoded = uint64(slice[0])
			case 2:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8
			case 3:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16
			case 4:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16 | uint64(slice[3])<<24
			default:
				if len(slice) >= 8 {
					decoded = binary.LittleEndian.Uint64(slice[:8])
				} else {
					decoded = 0
					for i, b := range slice {
						decoded |= uint64(b) << (8 * i)
					}
				}
			}
			shift := uint(64 - 8*lx)
			vx = uint64(int64(decoded<<shift) >> shift)
		}
	}

	valueB := vm.register[registerIndexB]
	sum32 := (valueB + vx) & 0xFFFFFFFF
	result := sum32
	if sum32&0x80000000 != 0 {
		result |= 0xFFFFFFFF00000000
	}
	vm.register[registerIndexA] = result
	if PvmTrace {
		dumpBinOp("+", registerIndexA, registerIndexB, vx, result)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleADD_IMM_64(vm *VM, operands []byte) {
	// Extract register indices directly
	registerIndexA := int(operands[0] & 0x0F)
	registerIndexB := int(operands[0] >> 4)

	// Calculate available operand bytes (limit to 8 for immediate value)
	operandLen := len(operands) - 1
	if operandLen > 8 {
		operandLen = 8
	}

	// Combined decode and sign-extend operation to avoid intermediate values
	var vx uint64
	if operandLen == 0 {
		vx = 0
	} else if operandLen >= 8 {
		// 8-byte operands
		vx = binary.LittleEndian.Uint64(operands[1:9])
	} else {
		// For shorter operands, decode and sign-extend in one step
		var rawValue uint64
		operandBytes := operands[1 : 1+operandLen]

		// Inline little-endian decode for common cases
		switch operandLen {
		case 1:
			rawValue = uint64(operandBytes[0])
		case 2:
			rawValue = uint64(operandBytes[0]) | uint64(operandBytes[1])<<8
		case 3:
			rawValue = uint64(operandBytes[0]) | uint64(operandBytes[1])<<8 | uint64(operandBytes[2])<<16
		case 4:
			rawValue = uint64(operandBytes[0]) | uint64(operandBytes[1])<<8 |
				uint64(operandBytes[2])<<16 | uint64(operandBytes[3])<<24
		default:
			// For 5-7 bytes, construct directly
			rawValue = 0
			for i, b := range operandBytes {
				rawValue |= uint64(b) << (8 * i)
			}
		}

		// Inline sign extension: shift left then arithmetic right shift
		shift := uint(64 - 8*operandLen)
		vx = uint64(int64(rawValue<<shift) >> shift)
	}

	result := vm.register[registerIndexB] + vx
	vm.register[registerIndexA] = result

	if PvmTrace {
		dumpBinOp("+", registerIndexA, registerIndexB, vx, result)
	}

	vm.pc += 1 + uint64(len(operands))
}

func handleAND_IMM(vm *VM, operands []byte) {
	var registerIndexA, registerIndexB int
	var vx uint64
	if len(operands) == 0 {
		registerIndexA, registerIndexB, vx = 0, 0, 0
	} else {
		firstByte := operands[0]
		registerIndexA = min(12, int(firstByte&0x0F))
		registerIndexB = min(12, int(firstByte)/16)

		lx := min(4, max(0, len(operands)-1))
		if lx > 0 && 1+lx <= len(operands) {
			slice := operands[1 : 1+lx]
			var decoded uint64
			switch len(slice) {
			case 1:
				decoded = uint64(slice[0])
			case 2:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8
			case 3:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16
			case 4:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16 | uint64(slice[3])<<24
			default:
				if len(slice) >= 8 {
					decoded = binary.LittleEndian.Uint64(slice[:8])
				} else {
					decoded = 0
					for i, b := range slice {
						decoded |= uint64(b) << (8 * i)
					}
				}
			}
			shift := uint(64 - 8*lx)
			vx = uint64(int64(decoded<<shift) >> shift)
		}
	}

	valueB := vm.register[registerIndexB]
	result := valueB & vx
	vm.register[registerIndexA] = result
	if PvmTrace {
		dumpBinOp("&", registerIndexA, registerIndexB, vx, result)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleXOR_IMM(vm *VM, operands []byte) {
	var registerIndexA, registerIndexB int
	var vx uint64
	if len(operands) == 0 {
		registerIndexA, registerIndexB, vx = 0, 0, 0
	} else {
		firstByte := operands[0]
		registerIndexA = min(12, int(firstByte&0x0F))
		registerIndexB = min(12, int(firstByte)/16)

		lx := min(4, max(0, len(operands)-1))
		if lx > 0 && 1+lx <= len(operands) {
			slice := operands[1 : 1+lx]
			var decoded uint64
			switch len(slice) {
			case 1:
				decoded = uint64(slice[0])
			case 2:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8
			case 3:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16
			case 4:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16 | uint64(slice[3])<<24
			default:
				if len(slice) >= 8 {
					decoded = binary.LittleEndian.Uint64(slice[:8])
				} else {
					decoded = 0
					for i, b := range slice {
						decoded |= uint64(b) << (8 * i)
					}
				}
			}
			shift := uint(64 - 8*lx)
			vx = uint64(int64(decoded<<shift) >> shift)
		}
	}

	valueB := vm.register[registerIndexB]
	result := valueB ^ vx
	vm.register[registerIndexA] = result
	if PvmTrace {
		dumpBinOp("^", registerIndexA, registerIndexB, vx, result)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleOR_IMM(vm *VM, operands []byte) {
	var registerIndexA, registerIndexB int
	var vx uint64
	if len(operands) == 0 {
		registerIndexA, registerIndexB, vx = 0, 0, 0
	} else {
		firstByte := operands[0]
		registerIndexA = min(12, int(firstByte&0x0F))
		registerIndexB = min(12, int(firstByte)/16)

		lx := min(4, max(0, len(operands)-1))
		if lx > 0 && 1+lx <= len(operands) {
			slice := operands[1 : 1+lx]
			var decoded uint64
			switch len(slice) {
			case 1:
				decoded = uint64(slice[0])
			case 2:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8
			case 3:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16
			case 4:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16 | uint64(slice[3])<<24
			default:
				if len(slice) >= 8 {
					decoded = binary.LittleEndian.Uint64(slice[:8])
				} else {
					decoded = 0
					for i, b := range slice {
						decoded |= uint64(b) << (8 * i)
					}
				}
			}
			shift := uint(64 - 8*lx)
			vx = uint64(int64(decoded<<shift) >> shift)
		}
	}

	valueB := vm.register[registerIndexB]
	result := valueB | vx
	vm.register[registerIndexA] = result
	if PvmTrace {
		dumpBinOp("|", registerIndexA, registerIndexB, vx, result)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleMUL_IMM_32(vm *VM, operands []byte) {
	var registerIndexA, registerIndexB int
	var vx uint64
	if len(operands) == 0 {
		registerIndexA, registerIndexB, vx = 0, 0, 0
	} else {
		firstByte := operands[0]
		registerIndexA = min(12, int(firstByte&0x0F))
		registerIndexB = min(12, int(firstByte)/16)

		lx := min(4, max(0, len(operands)-1))
		if lx > 0 && 1+lx <= len(operands) {
			slice := operands[1 : 1+lx]
			var decoded uint64
			switch len(slice) {
			case 1:
				decoded = uint64(slice[0])
			case 2:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8
			case 3:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16
			case 4:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16 | uint64(slice[3])<<24
			default:
				if len(slice) >= 8 {
					decoded = binary.LittleEndian.Uint64(slice[:8])
				} else {
					decoded = 0
					for i, b := range slice {
						decoded |= uint64(b) << (8 * i)
					}
				}
			}
			shift := uint(64 - 8*lx)
			vx = uint64(int64(decoded<<shift) >> shift)
		}
	}

	valueB := vm.register[registerIndexB]
	prod32 := (valueB * vx) & 0xFFFFFFFF
	result := prod32
	if prod32&0x80000000 != 0 {
		result |= 0xFFFFFFFF00000000
	}
	vm.register[registerIndexA] = result
	if PvmTrace {
		dumpBinOp("*", registerIndexA, registerIndexB, vx, result)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleMUL_IMM_64(vm *VM, operands []byte) {
	var registerIndexA, registerIndexB int
	var vx uint64
	if len(operands) == 0 {
		registerIndexA, registerIndexB, vx = 0, 0, 0
	} else {
		firstByte := operands[0]
		registerIndexA = min(12, int(firstByte&0x0F))
		registerIndexB = min(12, int(firstByte)/16)

		lx := min(4, max(0, len(operands)-1))
		if lx > 0 && 1+lx <= len(operands) {
			slice := operands[1 : 1+lx]
			var decoded uint64
			switch len(slice) {
			case 1:
				decoded = uint64(slice[0])
			case 2:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8
			case 3:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16
			case 4:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16 | uint64(slice[3])<<24
			default:
				if len(slice) >= 8 {
					decoded = binary.LittleEndian.Uint64(slice[:8])
				} else {
					decoded = 0
					for i, b := range slice {
						decoded |= uint64(b) << (8 * i)
					}
				}
			}
			shift := uint(64 - 8*lx)
			vx = uint64(int64(decoded<<shift) >> shift)
		}
	}

	valueB := vm.register[registerIndexB]
	result := valueB * vx
	vm.register[registerIndexA] = result
	if PvmTrace {
		dumpBinOp("*", registerIndexA, registerIndexB, vx, result)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleSET_LT_U_IMM(vm *VM, operands []byte) {
	var registerIndexA, registerIndexB int
	var vx uint64
	if len(operands) == 0 {
		registerIndexA, registerIndexB, vx = 0, 0, 0
	} else {
		firstByte := operands[0]
		registerIndexA = min(12, int(firstByte&0x0F))
		registerIndexB = min(12, int(firstByte)/16)

		lx := min(4, max(0, len(operands)-1))
		if lx > 0 && 1+lx <= len(operands) {
			slice := operands[1 : 1+lx]
			var decoded uint64
			switch len(slice) {
			case 1:
				decoded = uint64(slice[0])
			case 2:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8
			case 3:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16
			case 4:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16 | uint64(slice[3])<<24
			default:
				if len(slice) >= 8 {
					decoded = binary.LittleEndian.Uint64(slice[:8])
				} else {
					decoded = 0
					for i, b := range slice {
						decoded |= uint64(b) << (8 * i)
					}
				}
			}
			shift := uint(64 - 8*lx)
			vx = uint64(int64(decoded<<shift) >> shift)
		}
	}

	valueB := vm.register[registerIndexB]
	var result uint64
	if valueB < vx {
		result = 1
	}
	vm.register[registerIndexA] = result
	if PvmTrace {
		dumpCmpOp("<u", registerIndexA, registerIndexB, vx, result)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleSET_LT_S_IMM(vm *VM, operands []byte) {
	var registerIndexA, registerIndexB int
	var vx uint64
	if len(operands) == 0 {
		registerIndexA, registerIndexB, vx = 0, 0, 0
	} else {
		firstByte := operands[0]
		registerIndexA = min(12, int(firstByte&0x0F))
		registerIndexB = min(12, int(firstByte)/16)

		lx := min(4, max(0, len(operands)-1))
		if lx > 0 && 1+lx <= len(operands) {
			slice := operands[1 : 1+lx]
			var decoded uint64
			switch len(slice) {
			case 1:
				decoded = uint64(slice[0])
			case 2:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8
			case 3:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16
			case 4:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16 | uint64(slice[3])<<24
			default:
				if len(slice) >= 8 {
					decoded = binary.LittleEndian.Uint64(slice[:8])
				} else {
					decoded = 0
					for i, b := range slice {
						decoded |= uint64(b) << (8 * i)
					}
				}
			}
			shift := uint(64 - 8*lx)
			vx = uint64(int64(decoded<<shift) >> shift)
		}
	}

	valueB := vm.register[registerIndexB]
	var result uint64
	if int64(valueB) < int64(vx) {
		result = 1
	}
	vm.register[registerIndexA] = result
	if PvmTrace {
		dumpCmpOp("<s", registerIndexA, registerIndexB, vx, result)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleSET_GT_U_IMM(vm *VM, operands []byte) {
	var registerIndexA, registerIndexB int
	var vx uint64
	if len(operands) == 0 {
		registerIndexA, registerIndexB, vx = 0, 0, 0
	} else {
		firstByte := operands[0]
		registerIndexA = min(12, int(firstByte&0x0F))
		registerIndexB = min(12, int(firstByte)/16)

		lx := min(4, max(0, len(operands)-1))
		if lx > 0 && 1+lx <= len(operands) {
			slice := operands[1 : 1+lx]
			var decoded uint64
			switch len(slice) {
			case 1:
				decoded = uint64(slice[0])
			case 2:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8
			case 3:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16
			case 4:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16 | uint64(slice[3])<<24
			default:
				if len(slice) >= 8 {
					decoded = binary.LittleEndian.Uint64(slice[:8])
				} else {
					decoded = 0
					for i, b := range slice {
						decoded |= uint64(b) << (8 * i)
					}
				}
			}
			shift := uint(64 - 8*lx)
			vx = uint64(int64(decoded<<shift) >> shift)
		}
	}

	valueB := vm.register[registerIndexB]
	var result uint64
	if valueB > vx {
		result = 1
	}
	vm.register[registerIndexA] = result
	if PvmTrace {
		dumpCmpOp("u>", registerIndexA, registerIndexB, vx, result)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleSET_GT_S_IMM(vm *VM, operands []byte) {
	var registerIndexA, registerIndexB int
	var vx uint64
	if len(operands) == 0 {
		registerIndexA, registerIndexB, vx = 0, 0, 0
	} else {
		firstByte := operands[0]
		registerIndexA = min(12, int(firstByte&0x0F))
		registerIndexB = min(12, int(firstByte)/16)

		lx := min(4, max(0, len(operands)-1))
		if lx > 0 && 1+lx <= len(operands) {
			slice := operands[1 : 1+lx]
			var decoded uint64
			switch len(slice) {
			case 1:
				decoded = uint64(slice[0])
			case 2:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8
			case 3:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16
			case 4:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16 | uint64(slice[3])<<24
			default:
				if len(slice) >= 8 {
					decoded = binary.LittleEndian.Uint64(slice[:8])
				} else {
					decoded = 0
					for i, b := range slice {
						decoded |= uint64(b) << (8 * i)
					}
				}
			}
			shift := uint(64 - 8*lx)
			vx = uint64(int64(decoded<<shift) >> shift)
		}
	}

	valueB := vm.register[registerIndexB]
	var result uint64
	if int64(valueB) > int64(vx) {
		result = 1
	}
	vm.register[registerIndexA] = result
	if PvmTrace {
		dumpCmpOp("s>", registerIndexA, registerIndexB, vx, result)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleNEG_ADD_IMM_32(vm *VM, operands []byte) {
	var registerIndexA, registerIndexB int
	var vx uint64
	if len(operands) == 0 {
		registerIndexA, registerIndexB, vx = 0, 0, 0
	} else {
		firstByte := operands[0]
		registerIndexA = min(12, int(firstByte&0x0F))
		registerIndexB = min(12, int(firstByte)/16)

		lx := min(4, max(0, len(operands)-1))
		if lx > 0 && 1+lx <= len(operands) {
			slice := operands[1 : 1+lx]
			var decoded uint64
			switch len(slice) {
			case 1:
				decoded = uint64(slice[0])
			case 2:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8
			case 3:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16
			case 4:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16 | uint64(slice[3])<<24
			default:
				if len(slice) >= 8 {
					decoded = binary.LittleEndian.Uint64(slice[:8])
				} else {
					decoded = 0
					for i, b := range slice {
						decoded |= uint64(b) << (8 * i)
					}
				}
			}
			shift := uint(64 - 8*lx)
			vx = uint64(int64(decoded<<shift) >> shift)
		}
	}

	valueB := vm.register[registerIndexB]
	diff32 := (vx - valueB) & 0xFFFFFFFF
	result := diff32
	if diff32&0x80000000 != 0 {
		result |= 0xFFFFFFFF00000000
	}
	vm.register[registerIndexA] = result
	if PvmTrace {
		dumpBinOp("-+", registerIndexA, registerIndexB, vx, result)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleNEG_ADD_IMM_64(vm *VM, operands []byte) {
	var registerIndexA, registerIndexB int
	var vx uint64
	if len(operands) == 0 {
		registerIndexA, registerIndexB, vx = 0, 0, 0
	} else {
		firstByte := operands[0]
		registerIndexA = min(12, int(firstByte&0x0F))
		registerIndexB = min(12, int(firstByte)/16)

		lx := min(4, max(0, len(operands)-1))
		if lx > 0 && 1+lx <= len(operands) {
			slice := operands[1 : 1+lx]
			var decoded uint64
			switch len(slice) {
			case 1:
				decoded = uint64(slice[0])
			case 2:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8
			case 3:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16
			case 4:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16 | uint64(slice[3])<<24
			default:
				if len(slice) >= 8 {
					decoded = binary.LittleEndian.Uint64(slice[:8])
				} else {
					decoded = 0
					for i, b := range slice {
						decoded |= uint64(b) << (8 * i)
					}
				}
			}
			shift := uint(64 - 8*lx)
			vx = uint64(int64(decoded<<shift) >> shift)
		}
	}

	valueB := vm.register[registerIndexB]
	result := vx - valueB
	vm.register[registerIndexA] = result
	if PvmTrace {
		dumpBinOp("-+", registerIndexA, registerIndexB, vx, result)
	}
	vm.pc += 1 + uint64(len(operands))
}

// Shift operations

func handleSHLO_L_IMM_32(vm *VM, operands []byte) {
	var registerIndexA, registerIndexB int
	var vx uint64
	if len(operands) == 0 {
		registerIndexA, registerIndexB, vx = 0, 0, 0
	} else {
		firstByte := operands[0]
		registerIndexA = min(12, int(firstByte&0x0F))
		registerIndexB = min(12, int(firstByte)/16)

		lx := min(4, max(0, len(operands)-1))
		if lx > 0 && 1+lx <= len(operands) {
			slice := operands[1 : 1+lx]
			var decoded uint64
			switch len(slice) {
			case 1:
				decoded = uint64(slice[0])
			case 2:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8
			case 3:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16
			case 4:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16 | uint64(slice[3])<<24
			default:
				if len(slice) >= 8 {
					decoded = binary.LittleEndian.Uint64(slice[:8])
				} else {
					decoded = 0
					for i, b := range slice {
						decoded |= uint64(b) << (8 * i)
					}
				}
			}
			shift := uint(64 - 8*lx)
			vx = uint64(int64(decoded<<shift) >> shift)
		}
	}

	valueB := vm.register[registerIndexB]
	shift32 := (valueB << (vx & 63)) & 0xFFFFFFFF
	result := shift32
	if shift32&0x80000000 != 0 {
		result |= 0xFFFFFFFF00000000
	}
	vm.register[registerIndexA] = result
	if PvmTrace {
		dumpShiftOp("<<", registerIndexA, registerIndexB, vx, result)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleSHLO_L_IMM_64(vm *VM, operands []byte) {
	var registerIndexA, registerIndexB int
	var vx uint64
	if len(operands) == 0 {
		registerIndexA, registerIndexB, vx = 0, 0, 0
	} else {
		firstByte := operands[0]
		registerIndexA = min(12, int(firstByte&0x0F))
		registerIndexB = min(12, int(firstByte)/16)

		lx := min(4, max(0, len(operands)-1))
		if lx > 0 && 1+lx <= len(operands) {
			slice := operands[1 : 1+lx]
			var decoded uint64
			switch len(slice) {
			case 1:
				decoded = uint64(slice[0])
			case 2:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8
			case 3:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16
			case 4:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16 | uint64(slice[3])<<24
			default:
				if len(slice) >= 8 {
					decoded = binary.LittleEndian.Uint64(slice[:8])
				} else {
					decoded = 0
					for i, b := range slice {
						decoded |= uint64(b) << (8 * i)
					}
				}
			}
			shift := uint(64 - 8*lx)
			vx = uint64(int64(decoded<<shift) >> shift)
		}
	}

	valueB := vm.register[registerIndexB]
	result := valueB << (vx & 63)
	vm.register[registerIndexA] = result
	if PvmTrace {
		dumpShiftOp("<<", registerIndexA, registerIndexB, vx, result)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleSHLO_R_IMM_32(vm *VM, operands []byte) {
	var registerIndexA, registerIndexB int
	var vx uint64
	if len(operands) == 0 {
		registerIndexA, registerIndexB, vx = 0, 0, 0
	} else {
		firstByte := operands[0]
		registerIndexA = min(12, int(firstByte&0x0F))
		registerIndexB = min(12, int(firstByte)/16)

		lx := min(4, max(0, len(operands)-1))
		if lx > 0 && 1+lx <= len(operands) {
			slice := operands[1 : 1+lx]
			var decoded uint64
			switch len(slice) {
			case 1:
				decoded = uint64(slice[0])
			case 2:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8
			case 3:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16
			case 4:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16 | uint64(slice[3])<<24
			default:
				if len(slice) >= 8 {
					decoded = binary.LittleEndian.Uint64(slice[:8])
				} else {
					decoded = 0
					for i, b := range slice {
						decoded |= uint64(b) << (8 * i)
					}
				}
			}
			shift := uint(64 - 8*lx)
			vx = uint64(int64(decoded<<shift) >> shift)
		}
	}

	valueB := vm.register[registerIndexB]
	shift32 := uint32(valueB) >> (vx & 31)
	result := uint64(shift32)
	if shift32&0x80000000 != 0 {
		result |= 0xFFFFFFFF00000000
	}
	vm.register[registerIndexA] = result
	if PvmTrace {
		dumpShiftOp(">>", registerIndexA, registerIndexB, vx, result)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleSHLO_R_IMM_64(vm *VM, operands []byte) {
	var registerIndexA, registerIndexB int
	var vx uint64
	if len(operands) == 0 {
		registerIndexA, registerIndexB, vx = 0, 0, 0
	} else {
		firstByte := operands[0]
		registerIndexA = min(12, int(firstByte&0x0F))
		registerIndexB = min(12, int(firstByte)/16)

		lx := min(4, max(0, len(operands)-1))
		if lx > 0 && 1+lx <= len(operands) {
			slice := operands[1 : 1+lx]
			var decoded uint64
			switch len(slice) {
			case 1:
				decoded = uint64(slice[0])
			case 2:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8
			case 3:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16
			case 4:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16 | uint64(slice[3])<<24
			default:
				if len(slice) >= 8 {
					decoded = binary.LittleEndian.Uint64(slice[:8])
				} else {
					decoded = 0
					for i, b := range slice {
						decoded |= uint64(b) << (8 * i)
					}
				}
			}
			shift := uint(64 - 8*lx)
			vx = uint64(int64(decoded<<shift) >> shift)
		}
	}

	valueB := vm.register[registerIndexB]
	result := valueB >> (vx & 63)
	vm.register[registerIndexA] = result
	if PvmTrace {
		dumpShiftOp(">>", registerIndexA, registerIndexB, vx, result)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleSHAR_R_IMM_32(vm *VM, operands []byte) {
	var registerIndexA, registerIndexB int
	var vx uint64
	if len(operands) == 0 {
		registerIndexA, registerIndexB, vx = 0, 0, 0
	} else {
		firstByte := operands[0]
		registerIndexA = min(12, int(firstByte&0x0F))
		registerIndexB = min(12, int(firstByte)/16)

		lx := min(4, max(0, len(operands)-1))
		if lx > 0 && 1+lx <= len(operands) {
			slice := operands[1 : 1+lx]
			var decoded uint64
			switch len(slice) {
			case 1:
				decoded = uint64(slice[0])
			case 2:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8
			case 3:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16
			case 4:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16 | uint64(slice[3])<<24
			default:
				if len(slice) >= 8 {
					decoded = binary.LittleEndian.Uint64(slice[:8])
				} else {
					decoded = 0
					for i, b := range slice {
						decoded |= uint64(b) << (8 * i)
					}
				}
			}
			shift := uint(64 - 8*lx)
			vx = uint64(int64(decoded<<shift) >> shift)
		}
	}

	valueB := vm.register[registerIndexB]
	result := uint64(int64(int32(valueB) >> (vx & 31)))
	vm.register[registerIndexA] = result
	if PvmTrace {
		dumpShiftOp(">>", registerIndexA, registerIndexB, vx, result)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleSHAR_R_IMM_64(vm *VM, operands []byte) {
	var registerIndexA, registerIndexB int
	var vx uint64
	if len(operands) == 0 {
		registerIndexA, registerIndexB, vx = 0, 0, 0
	} else {
		firstByte := operands[0]
		registerIndexA = min(12, int(firstByte&0x0F))
		registerIndexB = min(12, int(firstByte)/16)

		lx := min(4, max(0, len(operands)-1))
		if lx > 0 && 1+lx <= len(operands) {
			slice := operands[1 : 1+lx]
			var decoded uint64
			switch len(slice) {
			case 1:
				decoded = uint64(slice[0])
			case 2:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8
			case 3:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16
			case 4:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16 | uint64(slice[3])<<24
			default:
				if len(slice) >= 8 {
					decoded = binary.LittleEndian.Uint64(slice[:8])
				} else {
					decoded = 0
					for i, b := range slice {
						decoded |= uint64(b) << (8 * i)
					}
				}
			}
			shift := uint(64 - 8*lx)
			vx = uint64(int64(decoded<<shift) >> shift)
		}
	}

	valueB := vm.register[registerIndexB]
	result := uint64(int64(valueB) >> (vx & 63))
	vm.register[registerIndexA] = result
	if PvmTrace {
		dumpShiftOp(">>", registerIndexA, registerIndexB, vx, result)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleSHLO_L_IMM_ALT_32(vm *VM, operands []byte) {
	var registerIndexA, registerIndexB int
	var vx uint64
	if len(operands) == 0 {
		registerIndexA, registerIndexB, vx = 0, 0, 0
	} else {
		firstByte := operands[0]
		registerIndexA = min(12, int(firstByte&0x0F))
		registerIndexB = min(12, int(firstByte)/16)

		lx := min(4, max(0, len(operands)-1))
		if lx > 0 && 1+lx <= len(operands) {
			slice := operands[1 : 1+lx]
			var decoded uint64
			switch len(slice) {
			case 1:
				decoded = uint64(slice[0])
			case 2:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8
			case 3:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16
			case 4:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16 | uint64(slice[3])<<24
			default:
				if len(slice) >= 8 {
					decoded = binary.LittleEndian.Uint64(slice[:8])
				} else {
					decoded = 0
					for i, b := range slice {
						decoded |= uint64(b) << (8 * i)
					}
				}
			}
			shift := uint(64 - 8*lx)
			vx = uint64(int64(decoded<<shift) >> shift)
		}
	}

	valueB := vm.register[registerIndexB]
	shift32 := (vx << (valueB & 63)) & 0xFFFFFFFF
	result := shift32
	if shift32&0x80000000 != 0 {
		result |= 0xFFFFFFFF00000000
	}
	vm.register[registerIndexA] = result
	if PvmTrace {
		dumpShiftOp("<<", registerIndexA, registerIndexB, vx, result)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleSHLO_L_IMM_ALT_64(vm *VM, operands []byte) {
	var registerIndexA, registerIndexB int
	var vx uint64
	if len(operands) == 0 {
		registerIndexA, registerIndexB, vx = 0, 0, 0
	} else {
		firstByte := operands[0]
		registerIndexA = min(12, int(firstByte&0x0F))
		registerIndexB = min(12, int(firstByte)/16)

		lx := min(4, max(0, len(operands)-1))
		if lx > 0 && 1+lx <= len(operands) {
			slice := operands[1 : 1+lx]
			var decoded uint64
			switch len(slice) {
			case 1:
				decoded = uint64(slice[0])
			case 2:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8
			case 3:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16
			case 4:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16 | uint64(slice[3])<<24
			default:
				if len(slice) >= 8 {
					decoded = binary.LittleEndian.Uint64(slice[:8])
				} else {
					decoded = 0
					for i, b := range slice {
						decoded |= uint64(b) << (8 * i)
					}
				}
			}
			shift := uint(64 - 8*lx)
			vx = uint64(int64(decoded<<shift) >> shift)
		}
	}

	valueB := vm.register[registerIndexB]
	result := vx << (valueB & 63)
	vm.register[registerIndexA] = result
	if PvmTrace {
		dumpShiftOp("<<", registerIndexA, registerIndexB, vx, result)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleSHLO_R_IMM_ALT_32(vm *VM, operands []byte) {
	var registerIndexA, registerIndexB int
	var vx uint64
	if len(operands) == 0 {
		registerIndexA, registerIndexB, vx = 0, 0, 0
	} else {
		firstByte := operands[0]
		registerIndexA = min(12, int(firstByte&0x0F))
		registerIndexB = min(12, int(firstByte)/16)

		lx := min(4, max(0, len(operands)-1))
		if lx > 0 && 1+lx <= len(operands) {
			slice := operands[1 : 1+lx]
			var decoded uint64
			switch len(slice) {
			case 1:
				decoded = uint64(slice[0])
			case 2:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8
			case 3:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16
			case 4:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16 | uint64(slice[3])<<24
			default:
				if len(slice) >= 8 {
					decoded = binary.LittleEndian.Uint64(slice[:8])
				} else {
					decoded = 0
					for i, b := range slice {
						decoded |= uint64(b) << (8 * i)
					}
				}
			}
			shift := uint(64 - 8*lx)
			vx = uint64(int64(decoded<<shift) >> shift)
		}
	}

	valueB := vm.register[registerIndexB]
	shift32 := (vx >> (valueB & 63)) & 0xFFFFFFFF
	result := shift32
	if shift32&0x80000000 != 0 {
		result |= 0xFFFFFFFF00000000
	}
	vm.register[registerIndexA] = result
	if PvmTrace {
		dumpShiftOp(">>", registerIndexA, registerIndexB, vx, result)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleSHLO_R_IMM_ALT_64(vm *VM, operands []byte) {
	var registerIndexA, registerIndexB int
	var vx uint64
	if len(operands) == 0 {
		registerIndexA, registerIndexB, vx = 0, 0, 0
	} else {
		firstByte := operands[0]
		registerIndexA = min(12, int(firstByte&0x0F))
		registerIndexB = min(12, int(firstByte)/16)

		lx := min(4, max(0, len(operands)-1))
		if lx > 0 && 1+lx <= len(operands) {
			slice := operands[1 : 1+lx]
			var decoded uint64
			switch len(slice) {
			case 1:
				decoded = uint64(slice[0])
			case 2:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8
			case 3:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16
			case 4:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16 | uint64(slice[3])<<24
			default:
				if len(slice) >= 8 {
					decoded = binary.LittleEndian.Uint64(slice[:8])
				} else {
					decoded = 0
					for i, b := range slice {
						decoded |= uint64(b) << (8 * i)
					}
				}
			}
			shift := uint(64 - 8*lx)
			vx = uint64(int64(decoded<<shift) >> shift)
		}
	}

	valueB := vm.register[registerIndexB]
	result := vx >> (valueB & 63)
	vm.register[registerIndexA] = result
	if PvmTrace {
		dumpShiftOp(">>", registerIndexA, registerIndexB, vx, result)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleSHAR_R_IMM_ALT_32(vm *VM, operands []byte) {
	var registerIndexA, registerIndexB int
	var vx uint64
	if len(operands) == 0 {
		registerIndexA, registerIndexB, vx = 0, 0, 0
	} else {
		firstByte := operands[0]
		registerIndexA = min(12, int(firstByte&0x0F))
		registerIndexB = min(12, int(firstByte)/16)

		lx := min(4, max(0, len(operands)-1))
		if lx > 0 && 1+lx <= len(operands) {
			slice := operands[1 : 1+lx]
			var decoded uint64
			switch len(slice) {
			case 1:
				decoded = uint64(slice[0])
			case 2:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8
			case 3:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16
			case 4:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16 | uint64(slice[3])<<24
			default:
				if len(slice) >= 8 {
					decoded = binary.LittleEndian.Uint64(slice[:8])
				} else {
					decoded = 0
					for i, b := range slice {
						decoded |= uint64(b) << (8 * i)
					}
				}
			}
			shift := uint(64 - 8*lx)
			vx = uint64(int64(decoded<<shift) >> shift)
		}
	}

	valueB := vm.register[registerIndexB]
	result := uint64(int32(vx) >> (valueB & 31))
	vm.register[registerIndexA] = result
	if PvmTrace {
		dumpShiftOp(">>", registerIndexA, registerIndexB, vx, result)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleSHAR_R_IMM_ALT_64(vm *VM, operands []byte) {
	var registerIndexA, registerIndexB int
	var vx uint64
	if len(operands) == 0 {
		registerIndexA, registerIndexB, vx = 0, 0, 0
	} else {
		firstByte := operands[0]
		registerIndexA = min(12, int(firstByte&0x0F))
		registerIndexB = min(12, int(firstByte)/16)

		lx := min(4, max(0, len(operands)-1))
		if lx > 0 && 1+lx <= len(operands) {
			slice := operands[1 : 1+lx]
			var decoded uint64
			switch len(slice) {
			case 1:
				decoded = uint64(slice[0])
			case 2:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8
			case 3:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16
			case 4:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16 | uint64(slice[3])<<24
			default:
				if len(slice) >= 8 {
					decoded = binary.LittleEndian.Uint64(slice[:8])
				} else {
					decoded = 0
					for i, b := range slice {
						decoded |= uint64(b) << (8 * i)
					}
				}
			}
			shift := uint(64 - 8*lx)
			vx = uint64(int64(decoded<<shift) >> shift)
		}
	}

	valueB := vm.register[registerIndexB]
	result := uint64(int64(vx) >> (valueB & 63))
	vm.register[registerIndexA] = result
	if PvmTrace {
		dumpShiftOp(">>", registerIndexA, registerIndexB, vx, result)
	}
	vm.pc += 1 + uint64(len(operands))
}

// Rotation operations

func handleROT_R_64_IMM(vm *VM, operands []byte) {
	var registerIndexA, registerIndexB int
	var vx uint64
	if len(operands) == 0 {
		registerIndexA, registerIndexB, vx = 0, 0, 0
	} else {
		firstByte := operands[0]
		registerIndexA = min(12, int(firstByte&0x0F))
		registerIndexB = min(12, int(firstByte)/16)

		lx := min(4, max(0, len(operands)-1))
		if lx > 0 && 1+lx <= len(operands) {
			slice := operands[1 : 1+lx]
			var decoded uint64
			switch len(slice) {
			case 1:
				decoded = uint64(slice[0])
			case 2:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8
			case 3:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16
			case 4:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16 | uint64(slice[3])<<24
			default:
				if len(slice) >= 8 {
					decoded = binary.LittleEndian.Uint64(slice[:8])
				} else {
					decoded = 0
					for i, b := range slice {
						decoded |= uint64(b) << (8 * i)
					}
				}
			}
			shift := uint(64 - 8*lx)
			vx = uint64(int64(decoded<<shift) >> shift)
		}
	}

	valueB := vm.register[registerIndexB]
	result := bits.RotateLeft64(valueB, -int(vx&63))
	vm.register[registerIndexA] = result
	if PvmTrace {
		dumpRotOp("ROT_R_64_IMM", reg(registerIndexA), reg(registerIndexB), vx, result)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleROT_R_64_IMM_ALT(vm *VM, operands []byte) {
	var registerIndexA, registerIndexB int
	var vx uint64
	if len(operands) == 0 {
		registerIndexA, registerIndexB, vx = 0, 0, 0
	} else {
		firstByte := operands[0]
		registerIndexA = min(12, int(firstByte&0x0F))
		registerIndexB = min(12, int(firstByte)/16)

		lx := min(4, max(0, len(operands)-1))
		if lx > 0 && 1+lx <= len(operands) {
			slice := operands[1 : 1+lx]
			var decoded uint64
			switch len(slice) {
			case 1:
				decoded = uint64(slice[0])
			case 2:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8
			case 3:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16
			case 4:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16 | uint64(slice[3])<<24
			default:
				if len(slice) >= 8 {
					decoded = binary.LittleEndian.Uint64(slice[:8])
				} else {
					decoded = 0
					for i, b := range slice {
						decoded |= uint64(b) << (8 * i)
					}
				}
			}
			shift := uint(64 - 8*lx)
			vx = uint64(int64(decoded<<shift) >> shift)
		}
	}

	valueB := vm.register[registerIndexB]
	result := bits.RotateLeft64(vx, -int(valueB&63))
	vm.register[registerIndexA] = result
	if PvmTrace {
		dumpRotOp("ROT_R_64_IMM_ALT", reg(registerIndexA), reg(registerIndexB), vx, result)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleROT_R_32_IMM(vm *VM, operands []byte) {
	var registerIndexA, registerIndexB int
	var vx uint64
	if len(operands) == 0 {
		registerIndexA, registerIndexB, vx = 0, 0, 0
	} else {
		firstByte := operands[0]
		registerIndexA = min(12, int(firstByte&0x0F))
		registerIndexB = min(12, int(firstByte)/16)

		lx := min(4, max(0, len(operands)-1))
		if lx > 0 && 1+lx <= len(operands) {
			slice := operands[1 : 1+lx]
			var decoded uint64
			switch len(slice) {
			case 1:
				decoded = uint64(slice[0])
			case 2:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8
			case 3:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16
			case 4:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16 | uint64(slice[3])<<24
			default:
				if len(slice) >= 8 {
					decoded = binary.LittleEndian.Uint64(slice[:8])
				} else {
					decoded = 0
					for i, b := range slice {
						decoded |= uint64(b) << (8 * i)
					}
				}
			}
			shift := uint(64 - 8*lx)
			vx = uint64(int64(decoded<<shift) >> shift)
		}
	}

	valueB := vm.register[registerIndexB]
	rot32 := bits.RotateLeft32(uint32(valueB), -int(vx&31))
	result := uint64(rot32)
	if rot32&0x80000000 != 0 {
		result |= 0xFFFFFFFF00000000
	}
	vm.register[registerIndexA] = result
	if PvmTrace {
		dumpRotOp("ROT_R_32_IMM", reg(registerIndexA), reg(registerIndexB), vx, result)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleROT_R_32_IMM_ALT(vm *VM, operands []byte) {
	var registerIndexA, registerIndexB int
	var vx uint64
	if len(operands) == 0 {
		registerIndexA, registerIndexB, vx = 0, 0, 0
	} else {
		firstByte := operands[0]
		registerIndexA = min(12, int(firstByte&0x0F))
		registerIndexB = min(12, int(firstByte)/16)

		lx := min(4, max(0, len(operands)-1))
		if lx > 0 && 1+lx <= len(operands) {
			slice := operands[1 : 1+lx]
			var decoded uint64
			switch len(slice) {
			case 1:
				decoded = uint64(slice[0])
			case 2:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8
			case 3:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16
			case 4:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16 | uint64(slice[3])<<24
			default:
				if len(slice) >= 8 {
					decoded = binary.LittleEndian.Uint64(slice[:8])
				} else {
					decoded = 0
					for i, b := range slice {
						decoded |= uint64(b) << (8 * i)
					}
				}
			}
			shift := uint(64 - 8*lx)
			vx = uint64(int64(decoded<<shift) >> shift)
		}
	}

	valueB := vm.register[registerIndexB]
	rot32 := bits.RotateLeft32(uint32(vx), -int(valueB&31))
	result := uint64(rot32)
	if rot32&0x80000000 != 0 {
		result |= 0xFFFFFFFF00000000
	}
	vm.register[registerIndexA] = result
	if PvmTrace {
		dumpRotOp("ROT_R_32_IMM_ALT", reg(registerIndexA), reg(registerIndexB), vx, result)
	}
	vm.pc += 1 + uint64(len(operands))
}

// Conditional move operations

func handleCMOV_IZ_IMM(vm *VM, operands []byte) {
	var registerIndexA, registerIndexB int
	var vx uint64
	if len(operands) == 0 {
		registerIndexA, registerIndexB, vx = 0, 0, 0
	} else {
		firstByte := operands[0]
		registerIndexA = min(12, int(firstByte&0x0F))
		registerIndexB = min(12, int(firstByte)/16)

		lx := min(4, max(0, len(operands)-1))
		if lx > 0 && 1+lx <= len(operands) {
			slice := operands[1 : 1+lx]
			var decoded uint64
			switch len(slice) {
			case 1:
				decoded = uint64(slice[0])
			case 2:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8
			case 3:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16
			case 4:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16 | uint64(slice[3])<<24
			default:
				if len(slice) >= 8 {
					decoded = binary.LittleEndian.Uint64(slice[:8])
				} else {
					decoded = 0
					for i, b := range slice {
						decoded |= uint64(b) << (8 * i)
					}
				}
			}
			shift := uint(64 - 8*lx)
			vx = uint64(int64(decoded<<shift) >> shift)
		}
	}

	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	result := vx
	if valueB != 0 {
		result = valueA
	}
	vm.register[registerIndexA] = result
	if PvmTrace {
		dumpCmovOp("== 0", registerIndexA, registerIndexB, vx, valueA, result, true)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleCMOV_NZ_IMM(vm *VM, operands []byte) {
	var registerIndexA, registerIndexB int
	var vx uint64
	if len(operands) == 0 {
		registerIndexA, registerIndexB, vx = 0, 0, 0
	} else {
		firstByte := operands[0]
		registerIndexA = min(12, int(firstByte&0x0F))
		registerIndexB = min(12, int(firstByte)/16)

		lx := min(4, max(0, len(operands)-1))
		if lx > 0 && 1+lx <= len(operands) {
			slice := operands[1 : 1+lx]
			var decoded uint64
			switch len(slice) {
			case 1:
				decoded = uint64(slice[0])
			case 2:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8
			case 3:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16
			case 4:
				decoded = uint64(slice[0]) | uint64(slice[1])<<8 | uint64(slice[2])<<16 | uint64(slice[3])<<24
			default:
				if len(slice) >= 8 {
					decoded = binary.LittleEndian.Uint64(slice[:8])
				} else {
					decoded = 0
					for i, b := range slice {
						decoded |= uint64(b) << (8 * i)
					}
				}
			}
			shift := uint(64 - 8*lx)
			vx = uint64(int64(decoded<<shift) >> shift)
		}
	}

	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	var result uint64
	if valueB != 0 {
		result = vx
	} else {
		result = valueA
	}
	vm.register[registerIndexA] = result
	if PvmTrace {
		dumpCmovOp("!= 0", registerIndexA, registerIndexB, vx, valueA, result, false)
	}
	vm.pc += 1 + uint64(len(operands))
}
