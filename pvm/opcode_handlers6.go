package pvm

import "encoding/binary"

// A.5.6. Instructions with Arguments of One Register & One Immediate.

func handleJUMP_IND(vm *VM, operands []byte) {
	var registerIndexA int
	var vx uint64
	if len(operands) == 0 {
		registerIndexA, vx = 0, 0
	} else {
		registerIndexA = min(12, int(operands[0])%16)
		lx := min(4, max(1, len(operands)-1))
		if 1+lx <= len(operands) {
			slice := operands[1:1+lx]
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
		} else {
			shift := uint(64 - 8*lx)
			vx = uint64(int64(0<<shift) >> shift)
		}
	}
	
	valueA := vm.register[registerIndexA]
	target := (valueA + vx) & 0xffffffff
	if PvmTrace {
		dumpBranchImm("JUMP_IND", registerIndexA, valueA, vx, target, false, true)
	}
	vm.djump(target)
}

func handleLOAD_IMM(vm *VM, operands []byte) {
	var registerIndexA int
	var vx uint64
	if len(operands) == 0 {
		registerIndexA, vx = 0, 0
	} else {
		registerIndexA = min(12, int(operands[0])%16)
		lx := min(4, max(1, len(operands)-1))
		if 1+lx <= len(operands) {
			slice := operands[1:1+lx]
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
		} else {
			shift := uint(64 - 8*lx)
			vx = uint64(int64(0<<shift) >> shift)
		}
	}
	
	vm.register[registerIndexA] = vx
	if PvmTrace {
		dumpLoadImm("LOAD_IMM", registerIndexA, vx, vx, 64, false)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleLOAD_U8(vm *VM, operands []byte) {
	var registerIndexA int
	var addr uint32
	if len(operands) == 0 {
		registerIndexA, addr = 0, uint32(0)
	} else {
		registerIndexA = min(12, int(operands[0])%16)
		lx := min(4, max(1, len(operands)-1))
		if 1+lx <= len(operands) {
			slice := operands[1:1+lx]
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
			addr = uint32(uint64(int64(decoded<<shift) >> shift))
		} else {
			shift := uint(64 - 8*lx)
			addr = uint32(uint64(int64(0<<shift) >> shift))
		}
	}
	
	value, errCode := vm.ReadRAMBytes8(addr)
	if errCode != OK {
		vm.panic(errCode)
		return
	}
	result := uint64(value)
	vm.register[registerIndexA] = result
	if PvmTrace {
		dumpLoadGeneric("LOAD_U8", registerIndexA, uint64(addr), result, 8, false)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleLOAD_I8(vm *VM, operands []byte) {
	var registerIndexA int
	var addr uint32
	if len(operands) == 0 {
		registerIndexA, addr = 0, uint32(0)
	} else {
		registerIndexA = min(12, int(operands[0])%16)
		lx := min(4, max(1, len(operands)-1))
		if 1+lx <= len(operands) {
			slice := operands[1:1+lx]
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
			addr = uint32(uint64(int64(decoded<<shift) >> shift))
		} else {
			shift := uint(64 - 8*lx)
			addr = uint32(uint64(int64(0<<shift) >> shift))
		}
	}
	
	value, errCode := vm.ReadRAMBytes8(addr)
	if errCode != OK {
		vm.panic(errCode)
		return
	}
	result := uint64(value)
	if value&0x80 != 0 {
		result |= 0xFFFFFFFFFFFFFF00
	}
	vm.register[registerIndexA] = result
	if PvmTrace {
		dumpLoadGeneric("LOAD_I8", registerIndexA, uint64(addr), result, 8, true)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleLOAD_U16(vm *VM, operands []byte) {
	var registerIndexA int
	var addr uint32
	if len(operands) == 0 {
		registerIndexA, addr = 0, uint32(0)
	} else {
		registerIndexA = min(12, int(operands[0])%16)
		lx := min(4, max(1, len(operands)-1))
		if 1+lx <= len(operands) {
			slice := operands[1:1+lx]
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
			addr = uint32(uint64(int64(decoded<<shift) >> shift))
		} else {
			shift := uint(64 - 8*lx)
			addr = uint32(uint64(int64(0<<shift) >> shift))
		}
	}
	
	value, errCode := vm.ReadRAMBytes16(addr)
	if errCode != OK {
		vm.panic(errCode)
		return
	}
	result := uint64(value)
	vm.register[registerIndexA] = result
	if PvmTrace {
		dumpLoadGeneric("LOAD_U16", registerIndexA, uint64(addr), result, 16, false)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleLOAD_I16(vm *VM, operands []byte) {
	var registerIndexA int
	var addr uint32
	if len(operands) == 0 {
		registerIndexA, addr = 0, uint32(0)
	} else {
		registerIndexA = min(12, int(operands[0])%16)
		lx := min(4, max(1, len(operands)-1))
		if 1+lx <= len(operands) {
			slice := operands[1:1+lx]
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
			addr = uint32(uint64(int64(decoded<<shift) >> shift))
		} else {
			shift := uint(64 - 8*lx)
			addr = uint32(uint64(int64(0<<shift) >> shift))
		}
	}
	
	value, errCode := vm.ReadRAMBytes16(addr)
	if errCode != OK {
		vm.panic(errCode)
		return
	}
	result := uint64(value)
	if value&0x8000 != 0 {
		result |= 0xFFFFFFFFFFFF0000
	}
	vm.register[registerIndexA] = result
	if PvmTrace {
		dumpLoadGeneric("LOAD_I16", registerIndexA, uint64(addr), result, 16, true)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleLOAD_U32(vm *VM, operands []byte) {
	var registerIndexA int
	var addr uint32
	if len(operands) == 0 {
		registerIndexA, addr = 0, uint32(0)
	} else {
		registerIndexA = min(12, int(operands[0])%16)
		lx := min(4, max(1, len(operands)-1))
		if 1+lx <= len(operands) {
			slice := operands[1:1+lx]
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
			addr = uint32(uint64(int64(decoded<<shift) >> shift))
		} else {
			shift := uint(64 - 8*lx)
			addr = uint32(uint64(int64(0<<shift) >> shift))
		}
	}
	
	value, errCode := vm.ReadRAMBytes32(addr)
	if errCode != OK {
		vm.panic(errCode)
		return
	}
	result := uint64(value)
	vm.register[registerIndexA] = result
	if PvmTrace {
		dumpLoadGeneric("LOAD_U32", registerIndexA, uint64(addr), result, 32, false)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleLOAD_I32(vm *VM, operands []byte) {
	var registerIndexA int
	var addr uint32
	if len(operands) == 0 {
		registerIndexA, addr = 0, uint32(0)
	} else {
		registerIndexA = min(12, int(operands[0])%16)
		lx := min(4, max(1, len(operands)-1))
		if 1+lx <= len(operands) {
			slice := operands[1:1+lx]
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
			addr = uint32(uint64(int64(decoded<<shift) >> shift))
		} else {
			shift := uint(64 - 8*lx)
			addr = uint32(uint64(int64(0<<shift) >> shift))
		}
	}
	
	value, errCode := vm.ReadRAMBytes32(addr)
	if errCode != OK {
		vm.panic(errCode)
		return
	}
	result := uint64(value)
	if value&0x80000000 != 0 {
		result |= 0xFFFFFFFF00000000
	}
	vm.register[registerIndexA] = result
	if PvmTrace {
		dumpLoadGeneric("LOAD_I32", registerIndexA, uint64(addr), result, 32, true)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleLOAD_U64(vm *VM, operands []byte) {
	var registerIndexA int
	var addr uint32
	if len(operands) == 0 {
		registerIndexA, addr = 0, uint32(0)
	} else {
		registerIndexA = min(12, int(operands[0])%16)
		lx := min(4, max(1, len(operands)-1))
		if 1+lx <= len(operands) {
			slice := operands[1:1+lx]
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
			addr = uint32(uint64(int64(decoded<<shift) >> shift))
		} else {
			shift := uint(64 - 8*lx)
			addr = uint32(uint64(int64(0<<shift) >> shift))
		}
	}
	
	value, errCode := vm.ReadRAMBytes64(addr)
	if errCode != OK {
		vm.panic(errCode)
		return
	}
	vm.register[registerIndexA] = value
	if PvmTrace {
		dumpLoadGeneric("LOAD_U64", registerIndexA, uint64(addr), value, 64, false)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleSTORE_U8(vm *VM, operands []byte) {
	var registerIndexA int
	var addr uint32
	if len(operands) == 0 {
		registerIndexA, addr = 0, uint32(0)
	} else {
		registerIndexA = min(12, int(operands[0])%16)
		lx := min(4, max(1, len(operands)-1))
		if 1+lx <= len(operands) {
			slice := operands[1:1+lx]
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
			addr = uint32(uint64(int64(decoded<<shift) >> shift))
		} else {
			shift := uint(64 - 8*lx)
			addr = uint32(uint64(int64(0<<shift) >> shift))
		}
	}
	
	valueA := vm.register[registerIndexA]
	errCode := vm.WriteRAMBytes8(addr, uint8(valueA))
	if errCode != OK {
		vm.panic(errCode)
		return
	}
	if PvmTrace {
		dumpStoreGeneric("STORE_U8", uint64(addr), reg(registerIndexA), valueA&0xff, 8)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleSTORE_U16(vm *VM, operands []byte) {
	var registerIndexA int
	var addr uint32
	if len(operands) == 0 {
		registerIndexA, addr = 0, uint32(0)
	} else {
		registerIndexA = min(12, int(operands[0])%16)
		lx := min(4, max(1, len(operands)-1))
		if 1+lx <= len(operands) {
			slice := operands[1:1+lx]
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
			addr = uint32(uint64(int64(decoded<<shift) >> shift))
		} else {
			shift := uint(64 - 8*lx)
			addr = uint32(uint64(int64(0<<shift) >> shift))
		}
	}
	
	valueA := vm.register[registerIndexA]
	value := uint16(valueA)
	errCode := vm.WriteRAMBytes16(addr, value)
	if errCode != OK {
		vm.panic(errCode)
		return
	}
	if PvmTrace {
		dumpStoreGeneric("STORE_U16", uint64(addr), reg(registerIndexA), uint64(value), 16)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleSTORE_U32(vm *VM, operands []byte) {
	var registerIndexA int
	var addr uint32
	if len(operands) == 0 {
		registerIndexA, addr = 0, uint32(0)
	} else {
		registerIndexA = min(12, int(operands[0])%16)
		lx := min(4, max(1, len(operands)-1))
		if 1+lx <= len(operands) {
			slice := operands[1:1+lx]
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
			addr = uint32(uint64(int64(decoded<<shift) >> shift))
		} else {
			shift := uint(64 - 8*lx)
			addr = uint32(uint64(int64(0<<shift) >> shift))
		}
	}
	
	valueA := vm.register[registerIndexA]
	value := uint32(valueA)
	errCode := vm.WriteRAMBytes32(addr, value)
	if errCode != OK {
		vm.panic(errCode)
		return
	}
	if PvmTrace {
		dumpStoreGeneric("STORE_U32", uint64(addr), reg(registerIndexA), uint64(value), 32)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleSTORE_U64(vm *VM, operands []byte) {
	var registerIndexA int
	var addr uint32
	if len(operands) == 0 {
		registerIndexA, addr = 0, uint32(0)
	} else {
		registerIndexA = min(12, int(operands[0])%16)
		lx := min(4, max(1, len(operands)-1))
		if 1+lx <= len(operands) {
			slice := operands[1:1+lx]
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
			addr = uint32(uint64(int64(decoded<<shift) >> shift))
		} else {
			shift := uint(64 - 8*lx)
			addr = uint32(uint64(int64(0<<shift) >> shift))
		}
	}
	
	valueA := vm.register[registerIndexA]
	errCode := vm.WriteRAMBytes64(addr, valueA)
	if errCode != OK {
		vm.panic(errCode)
		return
	}
	if PvmTrace {
		dumpStoreGeneric("STORE_U64", uint64(addr), reg(registerIndexA), valueA, 64)
	}
	vm.pc += 1 + uint64(len(operands))
}
