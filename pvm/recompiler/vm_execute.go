package recompiler

import (
	"encoding/binary"
	"fmt"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

// Global variable to hold the address of DebugPrintInstruction
// This is set via CGO at runtime
var debugPrintInstructionAddr uint64

func (vm *X86Compiler) Compile(startStep uint64) {
	// Optimization 1: Pre-allocate maps with estimated capacity
	estimatedBlocks := uint64(len(vm.code)) / 4 // ~4 instructions per block average
	if estimatedBlocks < 32 {
		estimatedBlocks = 32
	}

	// init the recompiler with pre-allocated capacity
	vm.basicBlocks = make(map[uint64]*BasicBlock, estimatedBlocks)
	vm.x86Blocks = make(map[uint64]*BasicBlock, estimatedBlocks)
	vm.x86PC = 0

	// Optimization 2: Pre-allocate x86Code with estimated size (4x expansion ratio)
	estimatedCodeSize := uint64(len(vm.code)) * 4
	vm.x86Code = make([]byte, 0, estimatedCodeSize)

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

// u32 only - Optimized version with fixed-size array
func GenerateMovMemImm(addr uint64, imm uint32) []byte {
	// Total length: 1 (push rax) + 2 (REX,B8) +8 (addr) +2 (C7,modrm) +4 (imm32) +1 (pop rax) = 18 bytes
	// Use array instead of slice for better performance
	var code [18]byte
	idx := 0

	// push rax
	code[idx] = 0x50
	idx++

	// movabs rax, addr  =>  48 B8 <addr64 little-endian>
	code[idx] = 0x48
	code[idx+1] = 0xB8
	idx += 2

	// Direct bytes manipulation instead of creating temporary slice
	binary.LittleEndian.PutUint64(code[idx:idx+8], addr)
	idx += 8

	// mov dword ptr [rax], imm32  =>  C7 00 <imm32 little-endian>
	code[idx] = 0xC7
	code[idx+1] = 0x00
	idx += 2

	binary.LittleEndian.PutUint32(code[idx:idx+4], imm)
	idx += 4

	// pop rax
	code[idx] = 0x58

	// Return slice view of the array - this is much faster than append operations
	return code[:]
}

const nextx86SlotPatch = 0x8686_8686_8686_8686

func (vm *X86Compiler) translateBasicBlock(startPC uint64) *BasicBlock {
	pc := startPC
	block := NewBasicBlock(vm.x86PC)

	// Optimization 3: Pre-allocate instruction slice with typical capacity
	block.Instructions = make([]Instruction, 0, 8)

	hitBasicBlock := false
	chargeGasInstrunctionIndex := 0
	for pc < uint64(len(vm.code)) {
		op := vm.code[pc]

		olen := vm.skip(pc)
		operands := vm.code[pc+1 : pc+1+olen]
		if pc == 0 && op == JUMP && showDisassembly {
			fmt.Printf("JUMP at PC %d with operands %x\n", pc, operands)
			fmt.Printf("operands length: %d\n", olen)
			fmt.Printf("code hash %v", common.Blake2Hash(vm.code))
		}
		instruction := Instruction{
			Opcode: op,
			Args:   operands,
			Pc:     pc,
		}
		instruction.GetArgs()

		block.Instructions = append(block.Instructions, instruction)
		if op == ECALLI && !vm.IsChild {
			// Only charge ECALLI extra gas for parent VMs
			// Child VMs return to parent on ECALLI without extra gas charge
			lx := uint32(types.DecodeE_l(operands))
			host_func_id := int(lx)
			block.Instructions[chargeGasInstrunctionIndex].GasUsage += int64(vm.chargeGas(host_func_id))
			if debugRecompiler && false {
				fmt.Printf("ECALLI at PC %d with operands %x\n", pc, operands)
			}
		}
		pc0 := pc
		pc += uint64(olen) + 1
		block.Instructions[chargeGasInstrunctionIndex].GasUsage += 1
		if instruction.IsBranchInstruction() && vm.gasMode == GasModeBasicBlock {
			if pc+uint64(instruction.Offset1) >= uint64(len(vm.code)) {
				if debugGasModel {
					fmt.Printf("branch target 1 out of range pc=%d, offset1=%d so put trap\n", pc, instruction.Offset1)
				}
				block.Instructions[len(block.Instructions)-1].BranchTarget1 = 0
				block.Instructions[len(block.Instructions)-1].BranchTarget2 = vm.code[pc]
			} else {

				block.Instructions[len(block.Instructions)-1].BranchTarget1 = vm.code[pc0+uint64(instruction.Offset1)]
				block.Instructions[len(block.Instructions)-1].BranchTarget2 = vm.code[pc]
				if debugGasModel {
					fmt.Printf("branch target 1 pc=%d, offset1=%d so target1=%d %s\n", pc, instruction.Offset1, pc+uint64(instruction.Offset1), opcode_str(vm.code[pc+uint64(instruction.Offset1)]))
					fmt.Printf("branch target 2 pc=%d, offset2=%d so target2=%d %s\n", pc, instruction.Offset2, pc, opcode_str(vm.code[pc]))
				}
			}

		}
		if IsBasicBlockInstruction(op) {
			vm.setJumpMetadata(block, op, operands, pc0)
			hitBasicBlock = true
			break
		} else if pc >= uint64(len(vm.code)) && vm.gasMode == GasModeBasicBlock {
			trap_instruction := Instruction{
				Opcode: TRAP,
				Args:   nil,
				Pc:     pc,
			}
			block.Instructions = append(block.Instructions, trap_instruction)
			vm.setJumpMetadata(block, op, operands, pc0)
			break
		}

		chargeGasInstrunctionIndex = len(block.Instructions)
	}
	if debugGasModel {
		fmt.Printf("---basic block pc=%d---\n", startPC)
		for i, inst := range block.Instructions {
			fmt.Printf("[%d] %s srcReg=%d, destReg=%d\n", i, inst.String(), inst.SourceRegs, inst.DestRegs)
		}
	}
	if vm.gasMode == GasModeBasicBlock {
		block.GasModel.TransitionCycle(block.Instructions)
	}

	if len(block.Instructions) == 0 {
		return nil
	}

	// Optimization 4: Improved capacity estimation based on instruction types
	estimatedCodeSize := 32 // Base overhead
	for _, inst := range block.Instructions {
		switch inst.Opcode {
		case ECALLI:
			estimatedCodeSize += 80 // ECALLI generates more code
		default:
			estimatedCodeSize += 10 // Average instruction size
		}
	}
	code := make([]byte, 0, estimatedCodeSize)

	for i, inst := range block.Instructions {
		codeLen := len(code)

		if i == 0 {
			pvm_pc := uint32(inst.Pc)
			pc_addr := vm.pc_addr
			if vm.isPCCounting {
				code = append(code, GenerateMovMemImm(uint64(pc_addr), pvm_pc)...)
			}

			if vm.IsBlockCounting {
				basicBlockCounterAddr := pc_addr + 8
				code = append(code, generateIncMem(basicBlockCounterAddr)...)
			}
		}
		if vm.isChargingGas && vm.gasMode == GasModeInstruction {
			code = append(code, generateGasCheck(uint32(inst.GasUsage))...)
		} else if vm.isChargingGas && vm.gasMode == GasModeBasicBlock && i == 0 {
			gas_usage_int := max(1, block.GasModel.CycleCounter-3)
			gas_usage := uint32(gas_usage_int)
			code = append(code, generateGasCheck(gas_usage)...)
			block.GasUsage = gas_usage
		}
		// Insert debug tracing call if enabled (also used for verification mode)
		// Skip ECALLI and SBRK because they dump registers themselves and have special handling
		if (EnableDebugTracing || EnableVerifyMode) && inst.Opcode != ECALLI && inst.Opcode != SBRK {
			code = append(code, generateDebugCall(inst.Opcode, inst.Pc)...)
		}
		if inst.Opcode == ECALLI {
			block.needPatchNextx86Pc = true
			opcode := uint32(types.DecodeE_l(inst.Args))

			// Optimization 5: Build ECALLI code directly into main buffer to eliminate intermediate allocations
			vmStateCode, _ := BuildWriteContextSlotCode(vmStateSlotIndex, HOST, 8)
			hostIdCode, _ := BuildWriteContextSlotCode(hostFuncIdIndex, uint64(opcode), 4)
			pcCode, _ := BuildWriteContextSlotCode(pcSlotIndex, inst.Pc, 8)
			// next x86 instruction address
			nextx86Code, _ := BuildWriteContextSlotCode(nextx86SlotIndex, nextx86SlotPatch, 8)
			dumpCode := DumpRegisterToMemory(false)

			// Direct append to main code buffer - no intermediate slice
			code = append(code, vmStateCode...)
			code = append(code, hostIdCode...)
			code = append(code, pcCode...)
			// Record the offset where nextx86Code starts (relative to this instruction's start)
			nextx86CodeOffset := len(code) - codeLen
			code = append(code, nextx86Code...)
			code = append(code, dumpCode...)
			code = append(code, X86_OP_RET)
			// Record the total ECALLI code length (return address = instruction start + this length)
			ecalliTotalLen := len(code) - codeLen

			block.pvmPC_TO_x86Index[uint32(inst.Pc)] = codeLen
			// Store dynamic offsets for patching
			block.ecalliNextx86Offset = nextx86CodeOffset
			block.ecalliTotalLen = ecalliTotalLen
		} else if inst.Opcode == SBRK {
			block.needPatchNextx86Pc = true
			dstIdx, srcIdx := extractTwoRegisters(inst.Args)
			// vm.sbrkRegA[inst.Pc] = int(srcIdx)
			sbrkACode, _ := BuildWriteContextSlotCode(sbrkAIndex, uint64(srcIdx), 4)
			// vm.sbrkRegD[inst.Pc] = int(dstIdx)
			sbrkDCode, _ := BuildWriteContextSlotCode(sbrkDIndex, uint64(dstIdx), 4)
			vmStateCode, _ := BuildWriteContextSlotCode(vmStateSlotIndex, SBRK, 8)
			pcCode, _ := BuildWriteContextSlotCode(pcSlotIndex, inst.Pc, 8)
			// next x86 instruction address
			nextx86Code, _ := BuildWriteContextSlotCode(nextx86SlotIndex, nextx86SlotPatch, 8)
			dumpCode := DumpRegisterToMemory(false)
			// Direct append to main code buffer - no intermediate slice
			code = append(code, sbrkACode...)
			code = append(code, sbrkDCode...)
			code = append(code, vmStateCode...)
			code = append(code, pcCode...)
			// Record the offset where nextx86Code starts (relative to this instruction's start)
			nextx86CodeOffset := len(code) - codeLen
			code = append(code, nextx86Code...)
			code = append(code, dumpCode...)
			code = append(code, X86_OP_RET)
			// Record the total SBRK code length (return address = instruction start + this length)
			sbrkTotalLen := len(code) - codeLen

			block.pvmPC_TO_x86Index[uint32(inst.Pc)] = codeLen
			// Store dynamic offsets for patching
			block.sbrkNextx86Offset = nextx86CodeOffset
			block.sbrkTotalLen = sbrkTotalLen
		} else if translateFunc, ok := pvmByteCodeToX86Code[inst.Opcode]; ok {
			if i == len(block.Instructions)-1 {
				block.LastInstructionOffset = len(code)
			}

			additionalCode := translateFunc(inst)
			code = append(code, additionalCode...)
			block.pvmPC_TO_x86Index[uint32(inst.Pc)] = codeLen
		}
	}

	// add a trap if we didn't hit a basic block instruction at the end
	if !hitBasicBlock {
		trapCode := generateTrap(Instruction{
			Opcode: TRAP,
			Args:   nil,
		})
		code = append(code, trapCode...)
		block.JumpType = TRAP_JUMP
	}

	block.X86Code = code
	block.PVMNextPC = pc
	vm.basicBlocks[startPC] = block
	vm.x86Blocks[block.X86PC] = block
	if debugGasModel {
		fmt.Printf("basic block pc %d gas %d ,instruction length %d\n", startPC, block.GasUsage, len(block.Instructions))
	}

	vm.appendBlock(block)

	if showDisassembly {
		str := Disassemble(block.X86Code)
		fmt.Printf("Translated block at PVM PC %d to x86 PC %x with %d instructions: %s\n%s", startPC, block.X86PC, len(block.Instructions), block.String(), str)
	}
	return block
}

func (vm *X86Compiler) setJumpMetadata(block *BasicBlock, opcode byte, operands []byte, pc uint64) {
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

func (vm *X86Compiler) finalizeJumpTargets(J []uint32) {
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

func (vm *X86Compiler) patchJumpConditional(block *BasicBlock, targetTruePC uint64) {
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
func (vm *X86Compiler) patchJump(block *BasicBlock, targetPC uint64) {
	endOfBlock := block.X86PC + uint64(len(block.X86Code))
	offset := int32(targetPC) - int32(endOfBlock)
	binary.LittleEndian.PutUint32(vm.x86Code[endOfBlock-4:endOfBlock], uint32(offset))
}

// patchJumpTable updates each instruction in the block to jump to the target PC -- works with JUMP and LOAD_IMM_JUMP
func (vm *X86Compiler) patchJumpIndirectTable(J []uint32) {
	for i := 0; i < len(J); i++ {
		targetBlock, exist := vm.basicBlocks[uint64(J[i])]
		if !exist {
			panic(fmt.Sprintf("target block for J[%d]=%d not found in patchJumpIndirectTable", i, J[i]))
		}
		targetPC := targetBlock.X86PC
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

// patching next pc const
const EcalliCodeIdx = 146
const SbrkCodeIdx = 185
const nextPcStartOffset = 119 // remember to add the code index

func (vm *X86Compiler) appendBlock(block *BasicBlock) {
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
		if block.needPatchNextx86Pc && (inst.Opcode == SBRK || inst.Opcode == ECALLI) {
			// Use dynamic offset instead of hardcoded EcalliCodeIdx/SbrkCodeIdx
			var nextx86Offset, totalLen int
			if inst.Opcode == ECALLI {
				nextx86Offset = block.ecalliNextx86Offset
				totalLen = block.ecalliTotalLen
			} else {
				nextx86Offset = block.sbrkNextx86Offset
				totalLen = block.sbrkTotalLen
			}
			// The nextx86Code contains an 8-byte placeholder at offset 3 within nextx86Code
			// (after PUSH RAX [1 byte] + MOV opcode [2 bytes] = 3 bytes)
			patchOffset := x86_realpc + nextx86Offset + 3
			// nextX86Addr is the address after the entire ECALLI/SBRK code block
			nextX86Addr := uint64(x86_realpc + totalLen)
			binary.LittleEndian.PutUint64(vm.x86Code[patchOffset:patchOffset+8], nextX86Addr)
		}
		if debugRecompiler && false {
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
	code = append(code, 0x79, 0x00) // JNS rel8 - if gas is sufficient, jump to skip_trap
	code = append(code, 0x0F, 0x0B) // UD2 - trigger trap
	// skip_trap:
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

func generateAddMem64Imm32(reg X86Reg, offset int64, gasCost uint32) []byte {
	var code []byte

	// --- REX Prefix ---
	rex := byte(0x48) // REX.W = 1
	if reg.REXBit == 1 {
		rex |= 0x01 // REX.B = 1
	}
	code = append(code, rex)

	// --- Opcode: 81 /0 (ADD r/m64, imm32) ---
	code = append(code, 0x81)

	// --- ModRM ---
	// mod = 10 (disp32), reg = 0 (ADD), rm = reg.RegBits
	modrm := byte(0x80 | (0 << 3) | (reg.RegBits & 0x07))
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

// generateDebugCall generates x86 code to call DebugPrintInstruction(opcode, pc, reg_dump_addr)
// This function:
// 1. Saves caller-saved registers (System V AMD64 ABI: RAX, RCX, RDX, RSI, RDI, R8-R11)
// 2. Sets up arguments: RDI=opcode, RSI=pc, RDX=R12 (register dump address)
// 3. Calls DebugPrintInstruction
// 4. Restores caller-saved registers
func generateDebugCall(opcode byte, pc uint64) []byte {
	var code []byte

	// Get the function pointer address using a callback approach
	// We'll use a global variable set at runtime
	debugFuncAddr := debugPrintInstructionAddr
	dumpCode := DumpRegisterToMemory(true)
	code = append(code, dumpCode...)
	// Save caller-saved registers (System V AMD64 ABI)
	// We need to save: RAX, RCX, RDX, RSI, RDI, R8, R9, R10, R11
	code = append(code, 0x50)       // push rax
	code = append(code, 0x51)       // push rcx
	code = append(code, 0x52)       // push rdx
	code = append(code, 0x56)       // push rsi
	code = append(code, 0x57)       // push rdi
	code = append(code, 0x41, 0x50) // push r8
	code = append(code, 0x41, 0x51) // push r9
	code = append(code, 0x41, 0x52) // push r10
	code = append(code, 0x41, 0x53) // push r11

	// Prepare arguments (System V AMD64 ABI: RDI, RSI, RDX, RCX, R8, R9)
	// Argument 1 (RDI): opcode (uint32)
	// mov edi, opcode
	code = append(code, 0xBF) // MOV EDI, imm32
	imm32 := make([]byte, 4)
	binary.LittleEndian.PutUint32(imm32, uint32(opcode))
	code = append(code, imm32...)

	// Argument 2 (RSI): pc (uint64)
	// movabs rsi, pc
	code = append(code, 0x48, 0xBE) // MOV RSI, imm64
	imm64 := make([]byte, 8)
	binary.LittleEndian.PutUint64(imm64, pc)
	code = append(code, imm64...)

	// Argument 3 (RDX): reg_dump_addr (void*)
	// BaseReg currently points to realMemAddr, regDumpAddr is at BaseReg - dumpSize.
	code = append(code, emitMovRegToReg64(RDX, BaseReg)...)
	code = append(code, emitSubRegImm32Force81(RDX, uint32(dumpSize))...)

	// Load function address into RAX
	// movabs rax, debugFuncAddr
	code = append(code, 0x48, 0xB8) // MOV RAX, imm64
	funcImm64 := make([]byte, 8)
	binary.LittleEndian.PutUint64(funcImm64, debugFuncAddr)
	code = append(code, funcImm64...)

	// Call the function
	// call rax
	code = append(code, 0xFF, 0xD0) // CALL RAX

	// Restore caller-saved registers (in reverse order)
	code = append(code, 0x41, 0x5B) // pop r11
	code = append(code, 0x41, 0x5A) // pop r10
	code = append(code, 0x41, 0x59) // pop r9
	code = append(code, 0x41, 0x58) // pop r8
	code = append(code, 0x5F)       // pop rdi
	code = append(code, 0x5E)       // pop rsi
	code = append(code, 0x5A)       // pop rdx
	code = append(code, 0x59)       // pop rcx
	code = append(code, 0x58)       // pop rax

	return code
}
