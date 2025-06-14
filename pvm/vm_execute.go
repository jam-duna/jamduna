package pvm

import (
	"encoding/binary"
	"sync"
)

type RecompilerVM struct {
	*VM
	mu          sync.Mutex
	startCode   []byte
	exitCode    []byte
	realMemory  []byte
	regDumpMem  []byte
	regDumpAddr uintptr

	basicBlocks map[uint64]*BasicBlock // by PVM PC
	x86Blocks   map[uint64]*BasicBlock // by x86 PC
	x86PC       uint64
	x86Code     []byte
}

func (vm *RecompilerVM) Compile(startStep uint64) (map[uint64]*BasicBlock, map[uint64]*BasicBlock, []byte) {
	// init the recompiler
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

	block = NewBasicBlock(vm.x86PC)
	block.X86Code = vm.exitCode
	block.JumpType = TERMINATED
	vm.basicBlocks[block.X86PC] = block
	vm.appendBlock(block)
	vm.finalizeJumpTargets()

	return vm.basicBlocks, vm.x86Blocks, vm.x86Code
}

func (vm *RecompilerVM) translateBasicBlock(startPC uint64) *BasicBlock {
	pc := startPC
	block := NewBasicBlock(vm.x86PC)

	hitBasicBlock := false
	for pc < uint64(len(vm.code)) {
		op := vm.code[pc]
		olen := vm.skip(pc)
		operands := vm.code[pc+1 : pc+1+olen]
		block.AddInstruction(op, operands, int(pc), pc)
		pc0 := pc
		pc += uint64(olen) + 1
		if IsBasicBlockInstruction(op) {
			vm.setJumpMetadata(block, op, operands, pc0, int(olen))
			hitBasicBlock = true
			break
		}
	}

	if len(block.Instructions) == 0 {
		return nil
	}

	// translate the instructions to x86 code
	var code []byte
	for _, inst := range block.Instructions {
		if translateFunc, ok := pvmByteCodeToX86Code[inst.Opcode]; ok {
			code = append(code, translateFunc(inst)...)
		}
	}
	// add a trap if we didn't hit a basic block instruction at the end
	if hitBasicBlock == false {
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
	return block
}

func (vm *RecompilerVM) setJumpMetadata(block *BasicBlock, opcode byte, operands []byte, pc uint64, olen int) {
	switch {
	case opcode == JUMP:
		block.JumpType = DIRECT_JUMP
		block.TruePC = pc + uint64(extractOneOffset(operands))
	case opcode == LOAD_IMM_JUMP:
		_, _, vy := extractOneRegOneImmOneOffset(operands)
		block.JumpType = DIRECT_JUMP
		block.TruePC = pc + uint64(vy)
	case opcode == JUMP_IND:
		reg, imm := extractOneRegOneImm(operands)
		block.JumpType = INDIRECT_JUMP
		block.IndirectSourceRegister = reg
		block.IndirectJumpOffset = uint64(imm)
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
	case 81 <= opcode && opcode <= 90: // BRANCH_EQ_IMM
		_, _, offset := extractOneRegOneImmOneOffset(operands)
		block.JumpType = CONDITIONAL
		block.TruePC = pc + uint64(offset)
	default:
		block.JumpType = FALLTHROUGH_JUMP
	}
}

func (vm *RecompilerVM) finalizeJumpTargets() {
	for _, block := range vm.basicBlocks {
		if block.JumpType == DIRECT_JUMP {
			dest := vm.basicBlocks[block.TruePC]
			vm.patchJump(block, dest.X86PC)
		} else if block.JumpType == CONDITIONAL {
			destTrue, ok := vm.basicBlocks[block.TruePC]
			if !ok {
				return
			}
			destFalse, ok := vm.basicBlocks[block.PVMNextPC]
			if !ok {
				return
			}
			vm.patchJumpConditional(block, destTrue.X86PC, destFalse.X86PC)
		} else if block.JumpType == TRAP_JUMP {
			end := vm.findEndBlock()
			vm.patchJump(block, end.X86PC)
		} else if block.JumpType == INDIRECT_JUMP {
			// For INDIRECT_JUMP, we can't path the jump.
		}
	}
}
func (vm *RecompilerVM) patchJumpConditional(block *BasicBlock, targetTruePC, targetFalsePC uint64) {
	blockEnd := block.X86PC + uint64(len(block.X86Code))

	// The instruction after Jcc+imm32 lives at blockEnd-5
	ipAfterJcc := blockEnd - 5

	// relTrue is relative to that IP
	relTrue := int32(targetTruePC - ipAfterJcc)
	// relFalse is relative to the IP after the JMP opcode+imm32, i.e. blockEnd
	relFalse := int32(targetFalsePC - blockEnd)

	// Patch the 4‐byte Jcc immediate at blockEnd-9 .. blockEnd-5
	startTrue := blockEnd - 9
	endTrue := blockEnd - 5
	binary.LittleEndian.PutUint32(
		vm.x86Code[startTrue:endTrue],
		uint32(relTrue),
	)

	// Patch the 4‐byte JMP immediate at blockEnd-4 .. blockEnd
	startFalse := blockEnd - 4
	binary.LittleEndian.PutUint32(
		vm.x86Code[startFalse:blockEnd],
		uint32(relFalse),
	)
}

// patchJump updates the last instruction in the block to jump to the target PC -- works with JUMP and LOAD_IMM_JUMP
func (vm *RecompilerVM) patchJump(block *BasicBlock, targetPC uint64) {
	endOfBlock := block.X86PC + uint64(len(block.X86Code))
	offset := int32(targetPC) - int32(endOfBlock)
	binary.LittleEndian.PutUint32(vm.x86Code[endOfBlock-4:endOfBlock], uint32(offset))
}

func (vm *RecompilerVM) appendBlock(block *BasicBlock) {
	vm.x86Code = append(vm.x86Code, block.X86Code...)
	block.X86PC = vm.x86PC
	vm.x86PC += uint64(len(block.X86Code))
}

func (vm *RecompilerVM) findEndBlock() *BasicBlock {
	for _, b := range vm.basicBlocks {
		if b.JumpType == TERMINATED {
			return b
		}
	}
	return nil
}
