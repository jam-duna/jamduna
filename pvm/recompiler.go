package pvm

import (
	"encoding/binary"
	"fmt"
	"strings"
	"syscall"
	"unsafe"

	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/pvm/x86_execute"
	"github.com/colorfulnotion/jam/types"
	"golang.org/x/arch/x86/x86asm"
)

type RecompilerVM struct {
	*VM
	x86Code     []byte
	exitCode    []byte
	realMemory  []byte
	regDumpMem  []byte  // memory region to dump registers
	regDumpAddr uintptr // base address of regDumpMem
	PageMap     any     // TODO: define PageMap type
}

const (
	PageInaccessible = 0
	PageMutable      = 1
	PageImmutable    = 2
)

const (
	PageSize   = 4096                   // 4 KiB
	TotalMem   = 4 * 1024 * 1024 * 1024 // 4 GiB
	TotalPages = TotalMem / PageSize    // 1,048,576 pages
)

func NewRecompilerVM(vm *VM) (*RecompilerVM, error) {
	// allocate real memory
	const memSize = 4 * 1024 * 1024 * 1024 // 4GB
	mem, err := syscall.Mmap(
		-1, 0, memSize,
		syscall.PROT_NONE,
		syscall.MAP_ANON|syscall.MAP_PRIVATE,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to mmap memory: %v", err)
	}
	// put the memory address to BaseReg
	// mov r12, memAddr

	// allocate memory for register dump: one uint64 per reg
	dumpSize := len(regInfoList) * 8
	dumpMem, err := syscall.Mmap(
		-1, 0, dumpSize,
		syscall.PROT_READ|syscall.PROT_WRITE|syscall.PROT_EXEC,
		syscall.MAP_ANON|syscall.MAP_PRIVATE,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to mmap regDump memory: %v", err)
	}
	regDumpAddr := uintptr(unsafe.Pointer(&dumpMem[0]))

	rvm := &RecompilerVM{
		VM:          vm,
		realMemory:  mem,
		regDumpMem:  dumpMem,
		regDumpAddr: regDumpAddr,
		x86Code:     nil,
		exitCode:    nil,
	}
	code := encodeMovImm(BaseRegIndex, uint64(uintptr(unsafe.Pointer(&mem[0]))))
	rvm.x86Code = append(rvm.x86Code, code...)
	// initialize registers: mov rX, imm from vm.register
	for i := 0; i < len(vm.register); i++ {
		immVal := vm.register[i]
		code := encodeMovImm(i, immVal)
		fmt.Printf("Initialize %s = %d\n", regInfoList[i].Name, immVal)
		rvm.x86Code = append(rvm.x86Code, code...)
	}

	// build exitCode: dump registers into regDumpMem
	// first: mov r12, regDumpAddr (use r12 as base)
	code = encodeMovImm(BaseRegIndex, uint64(regDumpAddr))
	rvm.exitCode = append(rvm.exitCode, code...)

	// for each register, mov [r12 + i*8], rX
	for i := 0; i < len(regInfoList); i++ {
		off := byte(i * 8)
		dumpInstr := encodeMovRegToMem(i, BaseRegIndex, off)
		rvm.exitCode = append(rvm.exitCode, dumpInstr...)
	}
	return rvm, nil
}

func (vm *RecompilerVM) Close() error {
	var errs []error

	if vm.realMemory != nil {
		if err := syscall.Munmap(vm.realMemory); err != nil {
			errs = append(errs, fmt.Errorf("realMemory: %w", err))
		}
		vm.realMemory = nil
	}

	if vm.regDumpMem != nil {
		if err := syscall.Munmap(vm.regDumpMem); err != nil {
			errs = append(errs, fmt.Errorf("regDumpMem: %w", err))
		}
		vm.regDumpMem = nil
	}

	if vm.x86Code != nil {
		if err := syscall.Munmap(vm.x86Code); err != nil {
			errs = append(errs, fmt.Errorf("x86Code: %w", err))
		}
		vm.x86Code = nil
	}

	if vm.exitCode != nil {
		if err := syscall.Munmap(vm.exitCode); err != nil {
			errs = append(errs, fmt.Errorf("exitCode: %w", err))
		}
		vm.exitCode = nil
	}

	if len(errs) > 0 {
		return fmt.Errorf("Close encountered errors: %v", errs)
	}
	return nil
}
func (vm *RecompilerVM) GetBasicBlocks() {
	// 1 block first
	newBlock, err := vm.compileBasicBlock(0)
	if err != nil {
		log.Error(vm.logging, "Error compiling basic block", "error", err)
		vm.ResultCode = types.PVM_PANIC
		vm.terminated = true
	}
	vm.BasicBlocks = append(vm.BasicBlocks, newBlock)
}

func (vm *RecompilerVM) Translate() error {
	defer func() {
		if r := recover(); r != nil {
			log.Error(vm.logging, "RecompilerVM Translate panic", "error", r)
			vm.ResultCode = types.PVM_PANIC
			vm.terminated = true
		}
	}()

	for _, basic_block := range vm.BasicBlocks {
		for _, instruction := range basic_block.Instructions {
			opcode := instruction.Opcode
			fmt.Printf("Translating instruction %s (%d)\n", opcode_str(opcode), opcode)
			if translateFunc, exists := pvmByteCodeToX86Code[opcode]; exists {
				code := translateFunc(instruction)

				vm.x86Code = append(vm.x86Code, code...)
			} else {
				return fmt.Errorf("unknown opcode %s", opcode_str(opcode))
			}
		}
		fmt.Printf("%s\n", basic_block.String())
	}
	// append exitCode, then ret
	vm.x86Code = append(vm.x86Code, vm.exitCode...)
	vm.x86Code = append(vm.x86Code, 0xC3) // ret
	fmt.Printf("Generated x86 code:\n%s\n", Disassemble(vm.x86Code))
	return nil
}

func Disassemble(code []byte) string {
	var sb strings.Builder
	offset := 0

	for offset < len(code) {
		inst, err := x86asm.Decode(code[offset:], 64)
		length := inst.Len
		if err != nil {
			sb.WriteString(fmt.Sprintf("0x%04x: db 0x%02x\n", offset, code[offset]))
			offset++
			continue
		}

		var hexBytes []string
		for i := 0; i < length; i++ {
			hexBytes = append(hexBytes, fmt.Sprintf("%02x", code[offset+i]))
		}
		sb.WriteString(fmt.Sprintf(
			"0x%04x: %-16s %s\n",
			offset,
			strings.Join(hexBytes, " "),
			inst.String(),
		))

		offset += length
	}
	return sb.String()
}

func (vm *RecompilerVM) ExecuteX86Code() error {
	x86code := vm.x86Code
	codeAddr, err := syscall.Mmap(
		-1, 0, len(x86code),
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_ANON|syscall.MAP_PRIVATE,
	)
	if err != nil {
		return fmt.Errorf("failed to mmap exec code: %w", err)
	}
	copy(codeAddr, x86code)
	fmt.Printf("bytecode\n%x\n", x86code)
	err = syscall.Mprotect(codeAddr, syscall.PROT_READ|syscall.PROT_EXEC)
	if err != nil {
		return fmt.Errorf("failed to mprotect exec code: %w", err)
	}

	crashed := x86_execute.ExecuteX86(x86code)

	if crashed == -1 {
		vm.ResultCode = types.PVM_PANIC
		vm.terminated = true
		fmt.Printf("PANIC in ExecuteX86Code\n")
	}
	fmt.Printf("Execution finished, dumping registers\n")

	for i := 0; i < len(vm.register); i++ {
		regValue := binary.LittleEndian.Uint64(vm.regDumpMem[i*8:])
		fmt.Printf("%s = %d\n", regInfoList[i].Name, regValue)
		vm.register[i] = regValue
	}
	return nil
}

func (vm *VM) RunRecompiler() error {
	rvm, err := NewRecompilerVM(vm)
	if err != nil {
		return fmt.Errorf("failed to create recompiler VM: %w", err)
	}
	rvm.GetBasicBlocks()
	if len(rvm.BasicBlocks) == 0 {
		return fmt.Errorf("no basic blocks found")
	}
	fmt.Printf("RecompilerVM has %d basic blocks\n", len(rvm.BasicBlocks))
	if err := rvm.Translate(); err != nil {
		return fmt.Errorf("error translating bytecode: %w", err)
	}
	// Now we have rvm.x86Code ready, we can execute it
	if err := rvm.ExecuteX86Code(); err != nil {
		return fmt.Errorf("error executing x86 code: %w", err)
	}
	return nil
}

// A.5.2. Instructions with Arguments of One Immediate. InstructionI1
func generateSyscall(inst Instruction) []byte {
	return []byte{0x0F, 0x05}
}

// ----------------------------------------------------------------------------
// Map of “terminator” opcodes (the last instruction in a block) to handlers.
// Each handler emits its own x86 bytes and returns a next‐PC.  If it returns
// 0xFFFFFFFF, the VM harness will read r15 for the actual next‐PC.
// ----------------------------------------------------------------------------
var pvmByteCodeToX86CodeJumps = map[byte]func(Instruction, uint32) ([]byte, uint32){
	JUMP:            generateJumpRel32,
	LOAD_IMM_JUMP:   generateLoadImmJump,
	BRANCH_EQ_IMM:   generateBranchImm(0x84),
	BRANCH_NE_IMM:   generateBranchImm(0x85),
	BRANCH_LT_U_IMM: generateBranchImm(0x82),
	BRANCH_LE_U_IMM: generateBranchImm(0x86),
	BRANCH_GE_U_IMM: generateBranchImm(0x83),
	BRANCH_GT_U_IMM: generateBranchImm(0x87),
	BRANCH_LT_S_IMM: generateBranchImm(0x8C),
	BRANCH_LE_S_IMM: generateBranchImm(0x8E),
	BRANCH_GE_S_IMM: generateBranchImm(0x8D),
	BRANCH_GT_S_IMM: generateBranchImm(0x8F),

	JUMP_IND: generateJumpIndirect,

	BRANCH_EQ:   generateCompareBranch(0x0F, 0x84),
	BRANCH_NE:   generateCompareBranch(0x0F, 0x85),
	BRANCH_LT_U: generateCompareBranch(0x0F, 0x82),
	BRANCH_LT_S: generateCompareBranch(0x0F, 0x8C),
	BRANCH_GE_U: generateCompareBranch(0x0F, 0x83),
	BRANCH_GE_S: generateCompareBranch(0x0F, 0x8D),

	LOAD_IMM_JUMP_IND: generateLoadImmJumpIndirect,
}

/*
func (vm *RecompilerVM) Translate2(bb *BasicBlock, pc uint64) ([]byte, error) {
	x86Code := vm.startCode
	for i, inst := range bb.Instructions {
		op := inst.Opcode
		// last instr → use jump‐handler
		if i == len(bb.Instructions)-1 {
			if fn, ok := pvmByteCodeToX86CodeJumps[op]; ok {
				code, nextPC := fn(inst, uint32(pc))
				x86Code = append(x86Code, code...)
				x86Code = append(x86Code, vm.exitCode...)
				x86Code = append(x86Code, 0xC3)
				// TODO: inspect R15 if nextPC==0xFFFFFFFF
				vm.pc = uint64(nextPC)
			} else {
				return nil, fmt.Errorf("unknown terminator %s", opcode_str(op))
			}
		} else {
			// normal op → use the regular map
			if fn, ok := pvmByteCodeToX86Code[op]; ok {
				code := fn(inst)
				x86Code = append(x86Code, code...)
			} else {
				return nil, fmt.Errorf("unknown opcode %s", opcode_str(op))
			}
		}
	}
	return x86Code, nil
}
*/
// ----------------------------------------------------------------------------
// 1) JUMP rel32 (unconditional)
// ----------------------------------------------------------------------------
func generateJumpRel32(inst Instruction, pc uint32) ([]byte, uint32) {
	disp := int32(extractOneOffset(inst.Args))
	return nil, uint32(int64(pc) + int64(disp))
}

// A.5.6. Instructions with Arguments of One Register & Two Immediates.
//  2. JUMP_IND: indirect via memory - Implements: JMP QWORD PTR [r64_reg + disp32]
//     where regIdx is the base register and vx is the signed 32-bit offset.
func generateJumpIndirect(inst Instruction, pc uint32) ([]byte, uint32) {
	regIdx, vx := extractOneReg2Imm(inst.Args)
	src := regInfoList[regIdx]
	// mov r15,[src+disp32]
	rex := byte(0x48)
	if regInfoList[15].REXBit == 1 {
		rex |= 0x04
	}
	if src.REXBit == 1 {
		rex |= 0x01
	}
	modrm := byte(0x80 | (regInfoList[15].RegBits << 3) | src.RegBits)
	code := []byte{rex, 0x8B, modrm}
	disp := int32(vx)
	code = append(code, byte(disp), byte(disp>>8), byte(disp>>16), byte(disp>>24))
	return code, 0xFFFFFFFF
}

// A.5.8. Instructions with Arguments of One Register, One Immediate and One Offset.
// LOAD_IMM_JUMP Implements: r64_dst = vx; then PC += vy (signed 32-bit relative jump)
func generateLoadImmJump(inst Instruction, pc uint32) ([]byte, uint32) {
	dst, vx, vy := extractOneRegOneImmOneOffset(inst.Args)
	movDst := encodeMovImm(dst, vx)
	disp := int32(vy)
	return movDst, uint32(int64(pc) + int64(disp))
}

// ----------------------------------------------------------------------------
// 4) BRANCH_?_IMM
// ----------------------------------------------------------------------------
func generateBranchImm(jcc byte) func(Instruction, uint32) ([]byte, uint32) {
	return func(inst Instruction, pc uint32) ([]byte, uint32) {
		regIdx, imm, off := extractOneRegOneImmOneOffset(inst.Args)
		src := regInfoList[regIdx]
		rex := byte(0x48)
		if src.REXBit == 1 {
			rex |= 0x01
		}
		code := []byte{rex, 0x81, byte(0xC0 | (7 << 3) | src.RegBits)}
		code = append(code, encodeU32(uint32(imm))...)
		code = append(code, 0x0F, jcc)
		offp := len(code)
		code = append(code, 0, 0, 0, 0)
		disp := int32(off)
		target := uint32(int64(pc) + int64(disp))
		binary.LittleEndian.PutUint32(code[offp:], uint32(len(code)-(offp+4)))
		return code, target
	}
}

// A.5.12. Instruction with Arguments of Two Registers and Two Immediates.
// - LOAD_IMM_JUMP_IND
func generateLoadImmJumpIndirect(inst Instruction, pc uint32) ([]byte, uint32) {
	vx, vy, dstIdx, srcIdx := extractTwoRegsAndTwoImmediates(inst.Args)
	movDst := encodeMovImm(dstIdx, vx)
	src := regInfoList[srcIdx]
	rex := byte(0x48)
	if regInfoList[15].REXBit == 1 {
		rex |= 0x04
	}
	if src.REXBit == 1 {
		rex |= 0x01
	}
	modrm := byte(0x80 | (regInfoList[15].RegBits << 3) | src.RegBits)
	code := append([]byte{}, movDst...)
	code = append(code, rex, 0x8B, modrm)
	disp := int32(vy)
	code = append(code, byte(disp), byte(disp>>8), byte(disp>>16), byte(disp>>24))
	return code, 0xFFFFFFFF
}

// A.5.11. Instructions with Arguments of Two Registers & One Offset.
// 6) Two‐reg compare+branch
func generateCompareBranch(prefix, op byte) func(Instruction, uint32) ([]byte, uint32) {
	return func(inst Instruction, pc uint32) ([]byte, uint32) {
		lhsIdx, rhsIdx, off := extractTwoRegsOneOffset(inst.Args)
		lhs, rhs := regInfoList[lhsIdx], regInfoList[rhsIdx]
		rex := byte(0x48)
		if rhs.REXBit == 1 {
			rex |= 0x04
		}
		if lhs.REXBit == 1 {
			rex |= 0x01
		}
		code := []byte{rex, 0x39, byte(0xC0 | (rhs.RegBits << 3) | lhs.RegBits)}
		code = append(code, prefix, op)
		offp := len(code)
		code = append(code, 0, 0, 0, 0)
		disp := int32(off)
		target := uint32(int64(pc) + int64(disp))
		mov15t := encodeMovImm(15, uint64(target))
		code = append(code, mov15t...)
		binary.LittleEndian.PutUint32(code[offp:], uint32(len(code)-(offp+4)))
		return code, 0xFFFFFFFF
	}
}
