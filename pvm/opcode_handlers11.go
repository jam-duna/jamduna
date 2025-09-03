package pvm

import "encoding/binary"

// A.5.11. Instructions with Arguments of Two Registers and One Offset. (BRANCH_{EQ/NE/...})
func handleBRANCH_EQ(vm *VM, operands []byte) {
	var registerIndexA, registerIndexB int
	var vx0 int64
	if len(operands) == 0 {
		registerIndexA, registerIndexB, vx0 = 0, 0, 0
	} else {
		firstByte := int(operands[0])
		registerIndexA = min(12, firstByte%16)
		registerIndexB = min(12, firstByte/16)
		
		lx := min(4, max(0, len(operands)-1))
		if lx == 0 {
			lx = 1
		}
		
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
			shift := 64 - 8*lx
			vx0 = int64(decoded<<shift) >> shift
		} else {
			shift := 64 - 8*lx
			vx0 = int64(0<<shift) >> shift
		}
	}
	
	target := uint64(int64(vm.pc) + vx0)
	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	taken := valueA == valueB
	if taken {
		vm.branch(target, true)
	} else {
		vm.pc += uint64(1 + len(operands))
	}
	if PvmTrace {
		dumpBranch("BRANCH_EQ", registerIndexA, registerIndexB, valueA, valueB, target, taken)
	}
}

func handleBRANCH_NE(vm *VM, operands []byte) {
	var registerIndexA, registerIndexB int
	var vx0 int64
	if len(operands) == 0 {
		registerIndexA, registerIndexB, vx0 = 0, 0, 0
	} else {
		firstByte := int(operands[0])
		registerIndexA = min(12, firstByte%16)
		registerIndexB = min(12, firstByte/16)
		
		lx := min(4, max(0, len(operands)-1))
		if lx == 0 {
			lx = 1
		}
		
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
			shift := 64 - 8*lx
			vx0 = int64(decoded<<shift) >> shift
		} else {
			shift := 64 - 8*lx
			vx0 = int64(0<<shift) >> shift
		}
	}
	
	target := uint64(int64(vm.pc) + vx0)
	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	taken := valueA != valueB
	if taken {
		vm.branch(target, true)
	} else {
		vm.pc += uint64(1 + len(operands))
	}
	if PvmTrace {
		dumpBranch("BRANCH_NE", registerIndexA, registerIndexB, valueA, valueB, target, taken)
	}
}

func handleBRANCH_LT_U(vm *VM, operands []byte) {
	var registerIndexA, registerIndexB int
	var vx0 int64
	if len(operands) == 0 {
		registerIndexA, registerIndexB, vx0 = 0, 0, 0
	} else {
		firstByte := int(operands[0])
		registerIndexA = min(12, firstByte%16)
		registerIndexB = min(12, firstByte/16)
		
		lx := min(4, max(0, len(operands)-1))
		if lx == 0 {
			lx = 1
		}
		
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
			shift := 64 - 8*lx
			vx0 = int64(decoded<<shift) >> shift
		} else {
			shift := 64 - 8*lx
			vx0 = int64(0<<shift) >> shift
		}
	}
	
	target := uint64(int64(vm.pc) + vx0)
	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	taken := valueA < valueB
	if taken {
		vm.branch(target, true)
	} else {
		vm.pc += uint64(1 + len(operands))
	}
	if PvmTrace {
		dumpBranch("BRANCH_LT_U", registerIndexA, registerIndexB, valueA, valueB, target, taken)
	}
}

func handleBRANCH_LT_S(vm *VM, operands []byte) {
	var registerIndexA, registerIndexB int
	var vx0 int64
	if len(operands) == 0 {
		registerIndexA, registerIndexB, vx0 = 0, 0, 0
	} else {
		firstByte := int(operands[0])
		registerIndexA = min(12, firstByte%16)
		registerIndexB = min(12, firstByte/16)
		
		lx := min(4, max(0, len(operands)-1))
		if lx == 0 {
			lx = 1
		}
		
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
			shift := 64 - 8*lx
			vx0 = int64(decoded<<shift) >> shift
		} else {
			shift := 64 - 8*lx
			vx0 = int64(0<<shift) >> shift
		}
	}
	
	target := uint64(int64(vm.pc) + vx0)
	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	taken := int64(valueA) < int64(valueB)
	if taken {
		vm.branch(target, true)
	} else {
		vm.pc += uint64(1 + len(operands))
	}
	if PvmTrace {
		dumpBranch("BRANCH_LT_S", registerIndexA, registerIndexB, valueA, valueB, target, taken)
	}
}

func handleBRANCH_GE_U(vm *VM, operands []byte) {
	var registerIndexA, registerIndexB int
	var vx0 int64
	if len(operands) == 0 {
		registerIndexA, registerIndexB, vx0 = 0, 0, 0
	} else {
		firstByte := int(operands[0])
		registerIndexA = min(12, firstByte%16)
		registerIndexB = min(12, firstByte/16)
		
		lx := min(4, max(0, len(operands)-1))
		if lx == 0 {
			lx = 1
		}
		
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
			shift := 64 - 8*lx
			vx0 = int64(decoded<<shift) >> shift
		} else {
			shift := 64 - 8*lx
			vx0 = int64(0<<shift) >> shift
		}
	}
	
	target := uint64(int64(vm.pc) + vx0)
	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	taken := valueA >= valueB
	if taken {
		vm.branch(target, true)
	} else {
		vm.pc += uint64(1 + len(operands))
	}
	if PvmTrace {
		dumpBranch("BRANCH_GE_U", registerIndexA, registerIndexB, valueA, valueB, target, taken)
	}
}

func handleBRANCH_GE_S(vm *VM, operands []byte) {
	var registerIndexA, registerIndexB int
	var vx0 int64
	if len(operands) == 0 {
		registerIndexA, registerIndexB, vx0 = 0, 0, 0
	} else {
		firstByte := int(operands[0])
		registerIndexA = min(12, firstByte%16)
		registerIndexB = min(12, firstByte/16)
		
		lx := min(4, max(0, len(operands)-1))
		if lx == 0 {
			lx = 1
		}
		
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
			shift := 64 - 8*lx
			vx0 = int64(decoded<<shift) >> shift
		} else {
			shift := 64 - 8*lx
			vx0 = int64(0<<shift) >> shift
		}
	}
	
	target := uint64(int64(vm.pc) + vx0)
	valueA := vm.register[registerIndexA]
	valueB := vm.register[registerIndexB]
	taken := int64(valueA) >= int64(valueB)
	if taken {
		vm.branch(target, true)
	} else {
		vm.pc += uint64(1 + len(operands))
	}
	if PvmTrace {
		dumpBranch("BRANCH_GE_S", registerIndexA, registerIndexB, valueA, valueB, target, taken)
	}
}
