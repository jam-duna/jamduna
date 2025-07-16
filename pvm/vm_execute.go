package pvm

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sync"
	"unsafe"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
	"golang.org/x/arch/x86/x86asm"
)

type RecompilerVM struct {
	*VM
	mu          sync.Mutex
	startCode   []byte
	exitCode    []byte
	realMemory  []byte
	realMemAddr uintptr

	regDumpMem  []byte
	regDumpAddr uintptr

	realCode []byte
	codeAddr uintptr

	basicBlocks map[uint64]*BasicBlock // by PVM PC
	x86Blocks   map[uint64]*BasicBlock // by x86 PC
	x86PC       uint64
	x86Code     []byte

	JumpTableOffset  uint64         // offset for the jump table in x86Code
	JumpTableOffset2 uint64         // offset for the jump table in x86Code
	JumpTableMap     []uint64       // maps PVM PC to the index of the x86code (djump only)
	InstMapPVMToX86  map[uint32]int // maps PVM PC to the x86 PC index
	InstMapX86ToPVM  map[int]uint32 // maps x86 PC to the PVM PC

	djumpTableFunc []byte
	djumpAddr      uintptr // address of the jump table in x86Code

	//debug tool
	x86Instructions      map[int]x86asm.Inst
	dirtyPages           map[int]bool
	pc_addr              uint64
	r12                  uint64
	current_heap_pointer uint32
	isChargingGas        bool
	isPCCounting         bool
	IsBlockCounting      bool // whether to count basic blocks

	basicBlockExecutionCounter map[uint64]int // PVM PC to execution count

	OP_tally map[string]*X86InstTally `json:"tally"`
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
func (vm *RecompilerVM) translateBasicBlock(startPC uint64) *BasicBlock {
	pc := startPC
	block := NewBasicBlock(vm.x86PC)

	hitBasicBlock := false
	for pc < uint64(len(vm.code)) {
		op := vm.code[pc]

		olen := vm.skip(pc)
		operands := vm.code[pc+1 : pc+1+olen]
		if pc == 0 && op == JUMP && debugRecompiler {
			fmt.Printf("JUMP at PC %d with operands %x\n", pc, operands)
			fmt.Printf("operands length: %d\n", olen)
			fmt.Printf("code hash %v", common.Blake2Hash(vm.code))
		}
		if op == ECALLI {
			lx := uint32(types.DecodeE_l(operands))
			host_func_id := int(lx)
			block.GasUsage += int64(vm.chargeGas(host_func_id))
			if debugRecompiler && false {
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
		if inst.Opcode == ECALLI {
			// 1. Dump registers to memory.
			// 2. Set up C ABI registers (rdi, esi).
			opcode := uint32(types.DecodeE_l(inst.Args))
			Ecallcode := append(vm.DumpRegisterToMemory(true), EmitCallToEcalliStub(uintptr(unsafe.Pointer(vm)), int(opcode))...)
			Ecallcode = append(Ecallcode, vm.RestoreRegisterInX86()...)
			codeLen := len(code)
			code = append(code, Ecallcode...)
			block.pvmPC_TO_x86Index[uint32(inst.Pc)] = codeLen
		} else if inst.Opcode == SBRK {
			dstIdx, srcIdx := extractTwoRegisters(inst.Args)
			Sbrkcode := append(vm.DumpRegisterToMemory(true), EmitCallToSbrkStub(uintptr(unsafe.Pointer(vm)), uint32(srcIdx), uint32(dstIdx))...)
			Sbrkcode = append(Sbrkcode, vm.RestoreRegisterInX86()...)
			codeLen := len(code)
			code = append(code, Sbrkcode...)
			block.pvmPC_TO_x86Index[uint32(inst.Pc)] = codeLen
		} else if translateFunc, ok := pvmByteCodeToX86Code[inst.Opcode]; ok {
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
					gasMemAddr := uint64(vm.regDumpAddr + uintptr(len(regInfoList)*8))
					code = append(code, generateGasCheck(gasMemAddr, uint32(block.GasUsage))...)
				}
				if vm.IsBlockCounting {
					basicBlockCounterAddr := pc_addr + 8
					code = append(code, generateIncMem(basicBlockCounterAddr)...)
				}
			}
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
	str := vm.Disassemble(block.X86Code)
	if showDisassembly {
		fmt.Printf("Translated block at PVM PC %d to x86 PC %x with %d instructions: %s\n%s", startPC, block.X86PC, len(block.Instructions), block.String(), str)
	}
	return block
}

func (vm *RecompilerVM) setJumpMetadata(block *BasicBlock, opcode byte, operands []byte, pc uint64) {
	switch {
	case opcode == JUMP:
		block.JumpType = DIRECT_JUMP
		block.TruePC = pc + uint64(extractOneOffset(operands))
		if pc == 0 {
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

func (vm *RecompilerVM) finalizeJumpTargets(J []uint32) {
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
			destFalse, ok := vm.basicBlocks[block.PVMNextPC]
			if !ok {
				return
			}
			vm.patchJumpConditional(block, destTrue.X86PC, destFalse.X86PC)
		} else if block.JumpType == TRAP_JUMP {
			// vm.patchJump(block, end.X86PC)
		} else if block.JumpType == TERMINATED {
			vm.patchJumpIndirectTable(J)
		} else if block.JumpType == INDIRECT_JUMP {
			// For JUMP_IND [implements vm.pc = uint64(vm.J[((valueA+vx)/2)-1])]
			vm.patchJumpIndirect(block)
		}
	}
}

func (vm *RecompilerVM) patchJumpIndirect(block *BasicBlock) {
	codeAddress := make([]byte, 8)
	binary.LittleEndian.PutUint64(codeAddress, uint64(vm.djumpAddr))
	//	fmt.Printf("Patching indirect jump with code address 0x%x\n", vm.djumpAddr)
	patchData := []byte{0xFE, 0xFE, 0xFE, 0xFE, 0xFE, 0xFE, 0xFE, 0xFE}
	for c := block.X86PC; c < block.X86PC+uint64(len(block.X86Code)); c++ {

		if c+8 <= uint64(len(vm.x86Code)) && bytes.Equal(vm.x86Code[c:c+8], patchData) {
			// base_address is at c+2 .. c+10
			copy(vm.x86Code[c:c+8], codeAddress[:])
			// fmt.Printf("PATCHED\n")
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
func (vm *RecompilerVM) patchJumpIndirectTable(J []uint32) {
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

func (vm *RecompilerVM) appendBlock(block *BasicBlock) {
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
		if debugRecompiler && false {
			fmt.Printf("Mapped PVM PC %d to x86 PC %x\n", pvm_pc, x86_realpc)
		}
	}
}
