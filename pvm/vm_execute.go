package pvm

import (
	"encoding/binary"
	"fmt"
	"sync"
	"unsafe"

	"github.com/colorfulnotion/jam/types"
)

type RecompilerVM struct {
	*VM
	mu         sync.Mutex
	startCode  []byte
	exitCode   []byte
	realMemory []byte

	regDumpMem  []byte
	regDumpAddr uintptr

	realCode []byte
	codeAddr uintptr

	basicBlocks map[uint64]*BasicBlock // by PVM PC
	x86Blocks   map[uint64]*BasicBlock // by x86 PC
	x86PC       uint64
	x86Code     []byte

	JumpTableOffset  uint64      // offset for the jump table in x86Code
	JumpTableOffset2 uint64      // offset for the jump table in x86Code
	JumpTableMap     map[int]int // maps PVM PC to the index of the x86code

	djumpTableFunc []byte
	djumpAddr      uintptr // address of the jump table in x86Code
}

func (vm *RecompilerVM) Compile(startStep uint64) {
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

	// add exitCode
	block = NewBasicBlock(vm.x86PC)
	block.X86Code = vm.exitCode
	block.JumpType = TERMINATED
	vm.basicBlocks[block.X86PC] = block
	vm.appendBlock(block)
}

func encodeJumpIndirects(jumps []uint32) ([]byte, uint64, uint64) {
	// hop over all the jumps next to the "start" code which populates the registers
	jumpOffset := uint32(6 * len(jumps))
	code := []byte{0x90, 0xE9, byte(jumpOffset), byte(jumpOffset >> 8), byte(jumpOffset >> 16), byte(jumpOffset >> 24)}
	// these are JUMPs that JUMP_IND could reach
	JumpTableOffset := uint64(len(code))
	JumpTableOffset2 := uint64(jumpOffset)
	fmt.Printf("*** JumpTableOffset: %d, JumpTableOffset2: %d\n", JumpTableOffset, JumpTableOffset2)
	for range jumps {
		code = append(code, []byte{0x58, 0xE9, 0xFE, 0xFE, 0xFE, 0xFE}...) // 6 bytes -- pop rax, jmp [placeholder]
	}
	return code, JumpTableOffset, JumpTableOffset2
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
		if inst.Opcode == ECALLI {
			// 1. Dump registers to memory.
			// 2. Set up C ABI registers (rdi, esi).
			opcode := uint32(types.DecodeE_l([]byte{inst.Args[0]}))
			code = append(vm.DumpRegisterToMemory(true), EmitCallToEcalliStub(uintptr(unsafe.Pointer(vm)), int(opcode))...)
		} else if inst.Opcode == SBRK {
			rvmPtr := uintptr(unsafe.Pointer(vm))
			// mov rdi, rvmPtr
			stub := encodeMovRdiImm64(uint64(rvmPtr))
			// mov esi, args[0]
			stub = append(stub, encodeMovEsiImm32(uint32(inst.Args[0]))...)
			// movabs rax, <address of Sbrk>
			addr := GetSbrkAddress()
			stub = append(stub, encodeMovabsRaxImm64(uint64(addr))...)
			// call rax
			stub = append(stub, encodeCallRax()...)
			code = append(code, stub...)
		}
		if translateFunc, ok := pvmByteCodeToX86Code[inst.Opcode]; ok {
			if i == len(block.Instructions)-1 {
				block.LastInstructionOffset = len(code)
			}
			codeLen := len(code)
			code = append(code, translateFunc(inst)...)
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

	fmt.Printf("Translated block at PVM PC %d to x86 PC %x with %d instructions: %s\n%s", startPC, block.X86PC, len(block.Instructions), block.String(), Disassemble(block.X86Code))
	return block
}

func (vm *RecompilerVM) setJumpMetadata(block *BasicBlock, opcode byte, operands []byte, pc uint64) {
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

func (vm *RecompilerVM) finalizeJumpTargets(J []uint32) {
	end := vm.findEndBlock(TERMINATED)
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
			vm.patchJump(block, end.X86PC)
		} else if block.JumpType == TERMINATED {
			vm.patchJumpIndirectTable(block, J)
		} else if block.JumpType == INDIRECT_JUMP {
			// For JUMP_IND [implements vm.pc = uint64(vm.J[((valueA+vx)/2)-1])]
			vm.patchJumpIndirect(block)
		}
	}
}

func (vm *RecompilerVM) patchJumpIndirect(block *BasicBlock) {
	codeAddress := make([]byte, 8)
	binary.LittleEndian.PutUint64(codeAddress, uint64(vm.djumpAddr))
	fmt.Printf("Patching indirect jump with code address 0x%x\n", vm.djumpAddr)
	for c := block.X86PC; c < block.X86PC+uint64(len(block.X86Code)); c++ {
		if vm.x86Code[c] == 0x48 && vm.x86Code[c+1] == 0xB8 { // MOV RAX, base_address
			// base_address is at c+2 .. c+10
			copy(vm.x86Code[c+2:c+10], codeAddress[:])
			fmt.Printf("PATCHED\n")
			return
		}

	}
	fmt.Printf("NOT PATCHED\n")
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

// patchJumpTable updates each instruction in the block to jump to the target PC -- works with JUMP and LOAD_IMM_JUMP
func (vm *RecompilerVM) patchJumpIndirectTable(block *BasicBlock, J []uint32) {
	fmt.Printf("---- patchJumpIndirectTable %v\n", J)
	for i := 0; i < len(J); i++ {
		targetPC := vm.basicBlocks[uint64(J[i])].X86PC
		startOfInst := uint64(6*(i)) + vm.JumpTableOffset
		endOfInst := uint64(6*(i+1)) + vm.JumpTableOffset
		offset := int32(targetPC) - int32(endOfInst)
		fmt.Printf("J[%d]=%d Patching indirect jump at [0x%x:0x%x] to target 0x%x (offset %d)\n",
			i, J[i], startOfInst, endOfInst, targetPC, offset)
		binary.LittleEndian.PutUint32(vm.x86Code[startOfInst+2:startOfInst+6], uint32(offset))
	}
}

func (vm *RecompilerVM) appendBlock(block *BasicBlock) {
	startLen := len(vm.x86Code)
	vm.x86Code = append(vm.x86Code, block.X86Code...)
	block.X86PC = vm.x86PC
	vm.x86PC += uint64(len(block.X86Code))

	for i, pvmPC := range vm.J {

		if idx, exists := block.pvmPC_TO_x86Index[pvmPC]; exists {
			vm.JumpTableMap[i] = startLen + idx
		}
	}
}

func (vm *RecompilerVM) findEndBlock(jumpType int) *BasicBlock {
	for _, b := range vm.basicBlocks {
		if b.JumpType == jumpType {
			return b
		}
	}
	return nil
}
