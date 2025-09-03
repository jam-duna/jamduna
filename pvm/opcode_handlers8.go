package pvm

import "encoding/binary"

// A.5.8. Instructions with Arguments of One Register, One Immediate and One Offset.
func handleLOAD_IMM_JUMP(vm *VM, operands []byte) {
	var registerIndexA int
	var vx uint64
	var vy0 int64
	if len(operands) == 0 {
		registerIndexA, vx, vy0 = 0, 0, 0
	} else {
		firstByte := int(operands[0])
		registerIndexA = min(12, firstByte%16)
		lx := min(4, (firstByte/16)%8)
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
			shift := 64 - 8*ly
			vy0 = int64(decoded<<shift) >> shift
		} else {
			shift := 64 - 8*ly
			vy0 = int64(0<<shift) >> shift
		}
	}

	vm.register[registerIndexA] = vx
	target := uint64(int64(vm.pc) + vy0)
	if PvmTrace {
		dumpLoadImmJump("LOAD_IMM_JUMP", registerIndexA, vx)
	}
	vm.branch(target, true)
}

func handleBRANCH_EQ_IMM(vm *VM, operands []byte) {
	var registerIndexA int
	var vx uint64
	var vy0 int64
	if len(operands) == 0 {
		registerIndexA, vx, vy0 = 0, 0, 0
	} else {
		firstByte := int(operands[0])
		registerIndexA = min(12, firstByte%16)
		lx := min(4, (firstByte/16)%8)
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
			shift := 64 - 8*ly
			vy0 = int64(decoded<<shift) >> shift
		} else {
			shift := 64 - 8*ly
			vy0 = int64(0<<shift) >> shift
		}
	}

	valueA := vm.register[registerIndexA]
	target := uint64(int64(vm.pc) + vy0)
	taken := valueA == vx
	if PvmTrace {
		dumpBranchImm("BRANCH_EQ_IMM", registerIndexA, valueA, vx, target, false, taken)
	}
	if taken {
		vm.branch(target, true)
	} else {
		vm.pc += uint64(1 + len(operands))
	}
}

func handleBRANCH_NE_IMM(vm *VM, operands []byte) {
	var registerIndexA int
	var vx uint64
	var vy0 int64
	if len(operands) == 0 {
		registerIndexA, vx, vy0 = 0, 0, 0
	} else {
		firstByte := int(operands[0])
		registerIndexA = min(12, firstByte%16)
		lx := min(4, (firstByte/16)%8)
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
			shift := 64 - 8*ly
			vy0 = int64(decoded<<shift) >> shift
		} else {
			shift := 64 - 8*ly
			vy0 = int64(0<<shift) >> shift
		}
	}

	valueA := vm.register[registerIndexA]
	target := uint64(int64(vm.pc) + vy0)
	taken := valueA != vx
	if PvmTrace {
		dumpBranchImm("BRANCH_NE_IMM", registerIndexA, valueA, vx, target, false, taken)
	}
	if taken {
		vm.branch(target, true)
	} else {
		vm.pc += uint64(1 + len(operands))
	}
}

func handleBRANCH_LT_U_IMM(vm *VM, operands []byte) {
	var registerIndexA int
	var vx uint64
	var vy0 int64
	if len(operands) == 0 {
		registerIndexA, vx, vy0 = 0, 0, 0
	} else {
		firstByte := int(operands[0])
		registerIndexA = min(12, firstByte%16)
		lx := min(4, (firstByte/16)%8)
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
			shift := 64 - 8*ly
			vy0 = int64(decoded<<shift) >> shift
		} else {
			shift := 64 - 8*ly
			vy0 = int64(0<<shift) >> shift
		}
	}

	valueA := vm.register[registerIndexA]
	target := uint64(int64(vm.pc) + vy0)
	taken := valueA < vx
	if PvmTrace {
		dumpBranchImm("BRANCH_LT_U_IMM", registerIndexA, valueA, vx, target, false, taken)
	}
	if taken {
		vm.branch(target, true)
	} else {
		vm.pc += uint64(1 + len(operands))
	}
}

func handleBRANCH_LE_U_IMM(vm *VM, operands []byte) {
	var registerIndexA int
	var vx uint64
	var vy0 int64
	if len(operands) == 0 {
		registerIndexA, vx, vy0 = 0, 0, 0
	} else {
		firstByte := int(operands[0])
		registerIndexA = min(12, firstByte%16)
		lx := min(4, (firstByte/16)%8)
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
			shift := 64 - 8*ly
			vy0 = int64(decoded<<shift) >> shift
		} else {
			shift := 64 - 8*ly
			vy0 = int64(0<<shift) >> shift
		}
	}

	valueA := vm.register[registerIndexA]
	target := uint64(int64(vm.pc) + vy0)
	taken := valueA <= vx
	if PvmTrace {
		dumpBranchImm("BRANCH_LE_U_IMM", registerIndexA, valueA, vx, target, false, taken)
	}
	if taken {
		vm.branch(target, true)
	} else {
		vm.pc += uint64(1 + len(operands))
	}
}

func handleBRANCH_GE_U_IMM(vm *VM, operands []byte) {
	var registerIndexA int
	var vx uint64
	var vy0 int64
	if len(operands) == 0 {
		registerIndexA, vx, vy0 = 0, 0, 0
	} else {
		firstByte := int(operands[0])
		registerIndexA = min(12, firstByte%16)
		lx := min(4, (firstByte/16)%8)
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
			shift := 64 - 8*ly
			vy0 = int64(decoded<<shift) >> shift
		} else {
			shift := 64 - 8*ly
			vy0 = int64(0<<shift) >> shift
		}
	}

	valueA := vm.register[registerIndexA]
	target := uint64(int64(vm.pc) + vy0)
	taken := valueA >= vx
	if PvmTrace {
		dumpBranchImm("BRANCH_GE_U_IMM", registerIndexA, valueA, vx, target, false, taken)
	}
	if taken {
		vm.branch(target, true)
	} else {
		vm.pc += uint64(1 + len(operands))
	}
}

func handleBRANCH_GT_U_IMM(vm *VM, operands []byte) {
	var registerIndexA int
	var vx uint64
	var vy0 int64
	if len(operands) == 0 {
		registerIndexA, vx, vy0 = 0, 0, 0
	} else {
		firstByte := int(operands[0])
		registerIndexA = min(12, firstByte%16)
		lx := min(4, (firstByte/16)%8)
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
			shift := 64 - 8*ly
			vy0 = int64(decoded<<shift) >> shift
		} else {
			shift := 64 - 8*ly
			vy0 = int64(0<<shift) >> shift
		}
	}

	valueA := vm.register[registerIndexA]
	target := uint64(int64(vm.pc) + vy0)
	taken := valueA > vx
	if PvmTrace {
		dumpBranchImm("BRANCH_GT_U_IMM", registerIndexA, valueA, vx, target, false, taken)
	}
	if taken {
		vm.branch(target, true)
	} else {
		vm.pc += uint64(1 + len(operands))
	}
}

func handleBRANCH_LT_S_IMM(vm *VM, operands []byte) {
	var registerIndexA int
	var vx uint64
	var vy0 int64
	if len(operands) == 0 {
		registerIndexA, vx, vy0 = 0, 0, 0
	} else {
		firstByte := int(operands[0])
		registerIndexA = min(12, firstByte%16)
		lx := min(4, (firstByte/16)%8)
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
			shift := 64 - 8*ly
			vy0 = int64(decoded<<shift) >> shift
		} else {
			shift := 64 - 8*ly
			vy0 = int64(0<<shift) >> shift
		}
	}

	valueA := vm.register[registerIndexA]
	target := uint64(int64(vm.pc) + vy0)
	taken := int64(valueA) < int64(vx)
	if PvmTrace {
		dumpBranchImm("BRANCH_LT_S_IMM", registerIndexA, valueA, vx, target, true, taken)
	}
	if taken {
		vm.branch(target, true)
	} else {
		vm.pc += uint64(1 + len(operands))
	}
}

func handleBRANCH_LE_S_IMM(vm *VM, operands []byte) {
	var registerIndexA int
	var vx uint64
	var vy0 int64
	if len(operands) == 0 {
		registerIndexA, vx, vy0 = 0, 0, 0
	} else {
		firstByte := int(operands[0])
		registerIndexA = min(12, firstByte%16)
		lx := min(4, (firstByte/16)%8)
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
			shift := 64 - 8*ly
			vy0 = int64(decoded<<shift) >> shift
		} else {
			shift := 64 - 8*ly
			vy0 = int64(0<<shift) >> shift
		}
	}

	valueA := vm.register[registerIndexA]
	target := uint64(int64(vm.pc) + vy0)
	taken := int64(valueA) <= int64(vx)
	if PvmTrace {
		dumpBranchImm("BRANCH_LE_S_IMM", registerIndexA, valueA, vx, target, true, taken)
	}
	if taken {
		vm.branch(target, true)
	} else {
		vm.pc += uint64(1 + len(operands))
	}
}

func handleBRANCH_GE_S_IMM(vm *VM, operands []byte) {
	var registerIndexA int
	var vx uint64
	var vy0 int64
	if len(operands) == 0 {
		registerIndexA, vx, vy0 = 0, 0, 0
	} else {
		firstByte := int(operands[0])
		registerIndexA = min(12, firstByte%16)
		lx := min(4, (firstByte/16)%8)
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
			shift := 64 - 8*ly
			vy0 = int64(decoded<<shift) >> shift
		} else {
			shift := 64 - 8*ly
			vy0 = int64(0<<shift) >> shift
		}
	}

	valueA := vm.register[registerIndexA]
	target := uint64(int64(vm.pc) + vy0)
	taken := int64(valueA) >= int64(vx)
	if PvmTrace {
		dumpBranchImm("BRANCH_GE_S_IMM", registerIndexA, valueA, vx, target, true, taken)
	}
	if taken {
		vm.branch(target, true)
	} else {
		vm.pc += uint64(1 + len(operands))
	}
}

func handleBRANCH_GT_S_IMM(vm *VM, operands []byte) {
	var registerIndexA int
	var vx uint64
	var vy0 int64
	if len(operands) == 0 {
		registerIndexA, vx, vy0 = 0, 0, 0
	} else {
		firstByte := int(operands[0])
		registerIndexA = min(12, firstByte%16)
		lx := min(4, (firstByte/16)%8)
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
			shift := 64 - 8*ly
			vy0 = int64(decoded<<shift) >> shift
		} else {
			shift := 64 - 8*ly
			vy0 = int64(0<<shift) >> shift
		}
	}

	valueA := vm.register[registerIndexA]
	target := uint64(int64(vm.pc) + vy0)
	taken := int64(valueA) > int64(vx)
	if PvmTrace {
		dumpBranchImm("BRANCH_GT_S_IMM", registerIndexA, valueA, vx, target, true, taken)
	}
	if taken {
		vm.branch(target, true)
	} else {
		vm.pc += uint64(1 + len(operands))
	}
}
