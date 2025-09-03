package pvm

import "encoding/binary"

// A.5.4. Instructions with Arguments of Two Immediates.

func handleSTORE_IMM_U8(vm *VM, operands []byte) {
	var vx, vy uint64
	if len(operands) == 0 {
		vx, vy = 0, 0
	} else {
		lx := min(4, int(operands[0])%8)
		ly := min(4, max(1, len(operands)-lx-1))
		
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
		
		if 1+lx+ly <= len(operands) {
			slice := operands[1+lx:1+lx+ly]
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
			shift := uint(64 - 8*ly)
			vy = uint64(int64(decoded<<shift) >> shift)
		} else {
			shift := uint(64 - 8*ly)
			vy = uint64(int64(0<<shift) >> shift)
		}
	}
	
	addr := uint32(vx)
	errCode := vm.WriteRAMBytes8(addr, uint8(vy))
	if errCode != OK {
		vm.panic(errCode)
		return
	}
	if PvmTrace {
		dumpStoreGeneric("STORE_IMM_U8", uint64(addr), "imm", vy&0xff, 8)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleSTORE_IMM_U16(vm *VM, operands []byte) {
	var vx, vy uint64
	if len(operands) == 0 {
		vx, vy = 0, 0
	} else {
		lx := min(4, int(operands[0])%8)
		ly := min(4, max(1, len(operands)-lx-1))
		
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
		
		if 1+lx+ly <= len(operands) {
			slice := operands[1+lx:1+lx+ly]
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
			shift := uint(64 - 8*ly)
			vy = uint64(int64(decoded<<shift) >> shift)
		} else {
			shift := uint(64 - 8*ly)
			vy = uint64(int64(0<<shift) >> shift)
		}
	}
	
	addr := uint32(vx)
	value := uint16(vy)
	errCode := vm.WriteRAMBytes16(addr, value)
	if errCode != OK {
		vm.panic(errCode)
		return
	}
	if PvmTrace {
		dumpStoreGeneric("STORE_IMM_U16", uint64(addr), "imm", uint64(value), 16)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleSTORE_IMM_U32(vm *VM, operands []byte) {
	var vx, vy uint64
	if len(operands) == 0 {
		vx, vy = 0, 0
	} else {
		lx := min(4, int(operands[0])%8)
		ly := min(4, max(1, len(operands)-lx-1))
		
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
		
		if 1+lx+ly <= len(operands) {
			slice := operands[1+lx:1+lx+ly]
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
			shift := uint(64 - 8*ly)
			vy = uint64(int64(decoded<<shift) >> shift)
		} else {
			shift := uint(64 - 8*ly)
			vy = uint64(int64(0<<shift) >> shift)
		}
	}
	
	addr := uint32(vx)
	value := uint32(vy)
	errCode := vm.WriteRAMBytes32(addr, value)
	if errCode != OK {
		vm.panic(errCode)
		return
	}
	if PvmTrace {
		dumpStoreGeneric("STORE_IMM_U32", uint64(addr), "imm", uint64(value), 32)
	}
	vm.pc += 1 + uint64(len(operands))
}

func handleSTORE_IMM_U64(vm *VM, operands []byte) {
	var vx, vy uint64
	if len(operands) == 0 {
		vx, vy = 0, 0
	} else {
		lx := min(4, int(operands[0])%8)
		ly := min(4, max(1, len(operands)-lx-1))
		
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
		
		if 1+lx+ly <= len(operands) {
			slice := operands[1+lx:1+lx+ly]
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
			shift := uint(64 - 8*ly)
			vy = uint64(int64(decoded<<shift) >> shift)
		} else {
			shift := uint(64 - 8*ly)
			vy = uint64(int64(0<<shift) >> shift)
		}
	}
	
	addr := uint32(vx)
	errCode := vm.WriteRAMBytes64(addr, vy)
	if errCode != OK {
		vm.panic(errCode)
		return
	}
	if PvmTrace {
		dumpStoreGeneric("STORE_IMM_U64", uint64(addr), "imm", vy, 64)
	}
	vm.pc += 1 + uint64(len(operands))
}
