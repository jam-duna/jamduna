package pvm

import (
	"encoding/binary"
	"fmt"
	"unsafe"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

func (vm *CompilerVM) Compile(startStep uint64) {
	// init the compiler
	vm.basicBlocks = make(map[uint64]*BasicBlock)
	vm.x86Blocks = make(map[uint64]*BasicBlock)
	vm.x86PC = 0
	vm.x86Code = make([]byte, 0)

	// emit start block
	block := NewBasicBlock(vm.x86PC)
	block.X86Code = vm.startCode
	block.JumpType = FALLTHROUGH_JUMP
	vm.basicBlocks[0] = block
	vm.appendBlock(block)

	pc := startStep
	for pc < uint64(len(vm.code)) {
		block := vm.translateBasicBlock(pc)
		if block == nil {
			break
		}
		pc = block.PVMNextPC
	}

	// add exitCode
	block = NewBasicBlock(vm.x86PC)
	block.JumpType = TERMINATED
	vm.basicBlocks[block.X86PC] = block
	vm.appendBlock(block)
}

// u32 only
func GenerateMovMemImm(addr uint64, imm uint32) []byte {
	// Total length: 1 (push rax) + 2 (REX,B8) +8 (addr) +2 (C7,modrm) +4 (imm32) +1 (pop rax) = 18 bytes
	code := make([]byte, 0, 18)

	// push rax
	code = append(code, 0x50)

	// movabs rax, addr  =>  48 B8 <addr64 little-endian>
	code = append(code, 0x48, 0xB8)
	addrBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(addrBytes, addr)
	code = append(code, addrBytes...)

	// mov dword ptr [rax], imm32  =>  C7 00 <imm32 little-endian>
	code = append(code, 0xC7, 0x00)
	immBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(immBytes, imm)
	code = append(code, immBytes...)

	// pop rax
	code = append(code, 0x58)

	return code
}
func (vm *CompilerVM) translateBasicBlock(startPC uint64) *BasicBlock {
	pc := startPC
	block := NewBasicBlock(vm.x86PC)

	hitBasicBlock := false
	for pc < uint64(len(vm.code)) {
		op := vm.code[pc]

		olen := vm.skip(pc)
		operands := vm.code[pc+1 : pc+1+olen]
		if pc == 0 && op == JUMP && showDisassembly {
			fmt.Printf("JUMP at PC %d with operands %x\n", pc, operands)
			fmt.Printf("operands length: %d\n", olen)
			fmt.Printf("code hash %v", common.Blake2Hash(vm.code))
		}
		if op == ECALLI {
			lx := uint32(types.DecodeE_l(operands))
			host_func_id := int(lx)
			block.GasUsage += int64(vm.chargeGas(host_func_id))
			if debugCompiler && false {
				fmt.Printf("ECALLI at PC %d with operands %x\n", pc, operands)
			}
		}
		block.AddInstruction(op, operands, int(pc), pc)
		pc0 := pc
		pc += uint64(olen) + 1
		block.GasUsage += 1
		if IsBasicBlockInstruction(op) {
			vm.setJumpMetadata(block, op, operands, pc0)

			hitBasicBlock = true

			break
		}
	}

	if len(block.Instructions) == 0 {
		return nil
	}

	// translate the instructions to x86 code
	var code []byte
	for i, inst := range block.Instructions {
		codeLen := len(code)
		if i == 0 {
			pvm_pc := uint32(inst.Pc)
			pc_addr := vm.pc_addr
			if vm.isPCCounting {
				code = append(code, GenerateMovMemImm(uint64(pc_addr), pvm_pc)...)
			}
			if useEcalli500 {
				Ecallcode := append(vm.DumpRegisterToMemory(true), EmitCallToEcalliStub(uintptr(unsafe.Pointer(vm)), int(500))...)
				Ecallcode = append(Ecallcode, vm.RestoreRegisterInX86()...)
				code = append(code, Ecallcode...)
			}
			if vm.isChargingGas {
				code = append(code, generateGasCheck(uint32(block.GasUsage))...)
			}
			if vm.IsBlockCounting {
				basicBlockCounterAddr := pc_addr + 8
				code = append(code, generateIncMem(basicBlockCounterAddr)...)
			}
		}
		if inst.Opcode == ECALLI {
			// 1. Dump registers to memory.
			// 2. Set up C ABI registers (rdi, esi).
			opcode := uint32(types.DecodeE_l(inst.Args))
			if opcode != LOG && vm.IsChild {
				// panic
				code = append(code, emitTrap()...)
				continue
			}
			Ecallcode := append(vm.DumpRegisterToMemory(true), EmitCallToEcalliStub(uintptr(unsafe.Pointer(vm)), int(opcode))...)
			Ecallcode = append(Ecallcode, vm.RestoreRegisterInX86()...)
			code = append(code, Ecallcode...)
			block.pvmPC_TO_x86Index[uint32(inst.Pc)] = codeLen
		} else if inst.Opcode == SBRK {
			dstIdx, srcIdx := extractTwoRegisters(inst.Args)
			Sbrkcode := append(vm.DumpRegisterToMemory(true), EmitCallToSbrkStub(uintptr(unsafe.Pointer(vm)), uint32(srcIdx), uint32(dstIdx))...)
			Sbrkcode = append(Sbrkcode, vm.RestoreRegisterInX86()...)
			code = append(code, Sbrkcode...)
			block.pvmPC_TO_x86Index[uint32(inst.Pc)] = codeLen
		} else if translateFunc, ok := pvmByteCodeToX86Code[inst.Opcode]; ok {
			if i == len(block.Instructions)-1 {
				block.LastInstructionOffset = len(code)
			}
			additionalCode := translateFunc(inst)
			//disassemble the additionalCode and build the tally map
			if UseTally {
				vm.DisassembleAndTally(inst.Opcode, additionalCode)
			}
			code = append(code, additionalCode...)
			block.pvmPC_TO_x86Index[uint32(inst.Pc)] = codeLen
		}
	}
	// add a trap if we didn't hit a basic block instruction at the end
	if !hitBasicBlock {
		code = append(code, generateTrap(Instruction{
			Opcode: TRAP,
			Args:   nil,
		})...)
		block.JumpType = TRAP_JUMP
	}

	block.X86Code = code
	block.PVMNextPC = pc
	vm.basicBlocks[startPC] = block
	vm.x86Blocks[block.X86PC] = block

	vm.appendBlock(block)

	if showDisassembly {
		str := vm.Disassemble(block.X86Code)
		fmt.Printf("Translated block at PVM PC %d to x86 PC %x with %d instructions: %s\n%s", startPC, block.X86PC, len(block.Instructions), block.String(), str)
	}
	return block
}

func (vm *CompilerVM) setJumpMetadata(block *BasicBlock, opcode byte, operands []byte, pc uint64) {
	switch {
	case opcode == JUMP:
		block.JumpType = DIRECT_JUMP
		block.TruePC = pc + uint64(extractOneOffset(operands))
		if pc == 0 && showDisassembly {
			fmt.Printf("JUMP at PC %d with operands %x, TruePC %d\n", pc, operands, block.TruePC)
		}
	case opcode == LOAD_IMM_JUMP:
		_, _, vy := extractOneRegOneImmOneOffset(operands)
		block.JumpType = DIRECT_JUMP
		block.TruePC = pc + uint64(vy)
	case opcode == JUMP_IND:
		reg, imm := extractOneRegOneImm(operands)
		block.JumpType = INDIRECT_JUMP
		block.IndirectSourceRegister = reg
		block.IndirectJumpOffset = uint64(imm)
		//fmt.Printf("JUMP_IND at PC %d with reg %d and imm %d\n", pc, reg, imm)
	case opcode == LOAD_IMM_JUMP_IND:
		_, reg, _, vy := extractTwoRegsAndTwoImmediates(operands)
		block.JumpType = INDIRECT_JUMP
		block.IndirectSourceRegister = reg
		block.IndirectJumpOffset = uint64(vy)
	case opcode == TRAP:
		block.JumpType = TRAP_JUMP
	case 170 <= opcode && opcode <= 175: // BRANCH_EQ
		_, _, offset := extractTwoRegsOneOffset(operands)
		block.JumpType = CONDITIONAL
		block.TruePC = pc + uint64(offset)
		block.PVMNextPC = pc + 1 + uint64(len(operands)) // skip the next instructionss
	case 81 <= opcode && opcode <= 90: // BRANCH_EQ_IMM
		_, _, offset := extractOneRegOneImmOneOffset(operands)
		block.JumpType = CONDITIONAL
		block.TruePC = pc + uint64(offset)
		//fmt.Printf("BRANCH_EQ_IMM at PC %d with operands %x, TruePC %d\n", pc, operands, block.TruePC)
	default:
		block.JumpType = FALLTHROUGH_JUMP
	}
}

func (vm *CompilerVM) finalizeJumpTargets(J []uint32) {
	// end := vm.findEndBlock(TERMINATED)
	for _, block := range vm.basicBlocks {
		if block.JumpType == DIRECT_JUMP {
			x86PC, ok := vm.InstMapPVMToX86[uint32(block.TruePC)]
			if !ok {
				panic(fmt.Sprintf("PVM PC %d not found in InstMapPVMToX86", block.TruePC))
			}
			vm.patchJump(block, uint64(x86PC))
		} else if block.JumpType == CONDITIONAL {
			destTrue, ok := vm.basicBlocks[block.TruePC]
			if !ok {
				return
			}
			vm.patchJumpConditional(block, destTrue.X86PC)
		} else if block.JumpType == TRAP_JUMP {
			// vm.patchJump(block, end.X86PC)
		} else if block.JumpType == TERMINATED {
			vm.patchJumpIndirectTable(J)
		}
	}
}

func (vm *CompilerVM) patchJumpConditional(block *BasicBlock, targetTruePC uint64) {
	blockEnd := block.X86PC + uint64(len(block.X86Code))

	// The instruction after Jcc+imm32 lives at blockEnd-5
	ipAfterJcc := blockEnd

	// relTrue is relative to that IP
	relTrue := int32(targetTruePC - ipAfterJcc)
	// relFalse is relative to the IP after the JMP opcode+imm32, i.e. blockEnd

	// Patch the 4‐byte JMP immediate at blockEnd-4 .. blockEnd
	startTrue := blockEnd - 4
	binary.LittleEndian.PutUint32(
		vm.x86Code[startTrue:blockEnd],
		uint32(relTrue),
	)
}

// patchJump updates the last instruction in the block to jump to the target PC -- works with JUMP and LOAD_IMM_JUMP
func (vm *CompilerVM) patchJump(block *BasicBlock, targetPC uint64) {
	endOfBlock := block.X86PC + uint64(len(block.X86Code))
	offset := int32(targetPC) - int32(endOfBlock)
	binary.LittleEndian.PutUint32(vm.x86Code[endOfBlock-4:endOfBlock], uint32(offset))
}

// patchJumpTable updates each instruction in the block to jump to the target PC -- works with JUMP and LOAD_IMM_JUMP
func (vm *CompilerVM) patchJumpIndirectTable(J []uint32) {
	for i := 0; i < len(J); i++ {
		targetPC := vm.basicBlocks[uint64(J[i])].X86PC
		startOfInst := uint64(6*(i)) + vm.JumpTableOffset
		endOfInst := uint64(6*(i+1)) + vm.JumpTableOffset
		offset := int32(targetPC) - int32(endOfInst)
		if false {
			fmt.Printf("patchJumpIndirectTable J[%d]=%d Patching indirect jump at [0x%x:0x%x] to target 0x%x (offset %d)\n",
				i, J[i], startOfInst, endOfInst, targetPC, offset)
			binary.LittleEndian.PutUint32(vm.x86Code[startOfInst+2:startOfInst+6], uint32(offset))
		}
	}
}

func (vm *CompilerVM) appendBlock(block *BasicBlock) {
	startLen := len(vm.x86Code)
	vm.x86Code = append(vm.x86Code, block.X86Code...)
	block.X86PC = vm.x86PC
	vm.x86PC += uint64(len(block.X86Code))

	for _, pvmPC := range vm.J {
		if idx, exists := block.pvmPC_TO_x86Index[pvmPC]; exists {
			vm.JumpTableMap = append(vm.JumpTableMap, uint64(startLen+idx))
		}
	}
	for _, inst := range block.Instructions {
		pvm_pc := uint32(inst.Pc)
		idx := block.pvmPC_TO_x86Index[pvm_pc]
		x86_realpc := startLen + idx
		vm.InstMapX86ToPVM[x86_realpc] = pvm_pc
		vm.InstMapPVMToX86[pvm_pc] = x86_realpc
		if debugCompiler && false {
			fmt.Printf("Mapped PVM PC %d to x86 PC %x\n", pvm_pc, x86_realpc)
		}
	}
}

func generateGasCheck(gasCharge uint32) []byte {
	var code []byte
	// 4G + 14*8
	offset := int64(14*8 - dumpSize)
	// fmt.Printf("generateGasCheck: memAddr=0x%X, gasCharge=%d, offset=%d\n", memAddr, gasCharge, offset)
	code = append(code, generateSubMem64Imm32(BaseReg, offset, gasCharge)...)

	// JNS skip_trap
	jumpPos := len(code)
	code = append(code, 0x79, 0x00) // JNS rel8
	code = append(code, 0x0F, 0x0B)

	// Patch rel8
	code[jumpPos+1] = byte(len(code) - (jumpPos + 2)) // calculate relative jump distance

	// fmt.Printf("disassembled gas check code: %s\n", Disassemble(code))
	return code
}

func generateSubMem64Imm32(reg X86Reg, offset int64, gasCost uint32) []byte {
	var code []byte

	// --- REX Prefix ---
	rex := byte(0x48) // REX.W = 1
	if reg.REXBit == 1 {
		rex |= 0x01 // REX.B = 1
	}
	code = append(code, rex)

	// --- Opcode: 81 /5 (SUB r/m64, imm32) ---
	code = append(code, 0x81)

	// --- ModRM ---
	// mod = 10 (disp32), reg = 5 (SUB), rm = reg.RegBits
	modrm := byte(0x80 | (5 << 3) | (reg.RegBits & 0x07))
	code = append(code, modrm)

	// --- SIB (required for r12/r13/r14/r15) ---
	if reg.RegBits&0x07 == 4 {
		// SIB: scale=0, index=none(100), base=100 (RSP/r12)
		sib := byte(0x24)
		code = append(code, sib)
	}

	// --- disp32 ---
	disp := make([]byte, 4)
	binary.LittleEndian.PutUint32(disp, uint32(offset))
	code = append(code, disp...)

	// --- imm32 ---
	imm := make([]byte, 4)
	binary.LittleEndian.PutUint32(imm, uint32(gasCost))
	code = append(code, imm...)

	return code
}
