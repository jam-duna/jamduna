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
	x86Code     []byte // x86 code to execute
	startCode   []byte
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
		startCode:   nil,
		exitCode:    nil,
	}
	rvm.initStartCode()

	// build exitCode: dump registers into regDumpMem
	// first: mov r12, regDumpAddr (use r12 as base)
	rvm.exitCode = encodeMovImm(BaseRegIndex, uint64(regDumpAddr))

	// for each register, mov [r12 + i*8], rX
	for i := 0; i < len(regInfoList); i++ {
		off := byte(i * 8)
		dumpInstr := encodeMovRegToMem(i, BaseRegIndex, off)
		rvm.exitCode = append(rvm.exitCode, dumpInstr...)
	}
	return rvm, nil
}

func (vm *RecompilerVM) initStartCode() {
	vm.startCode = encodeMovImm(BaseRegIndex, uint64(uintptr(unsafe.Pointer(&vm.realMemory[0]))))
	// initialize registers: mov rX, imm from vm.register
	for i := 0; i < len(vm.register); i++ {
		immVal := vm.register[i]
		code := encodeMovImm(i, immVal)
		fmt.Printf("Initialize %s = %d\n", regInfoList[i].Name, immVal)
		vm.startCode = append(vm.startCode, code...)
	}
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

	// if vm.exitCode != nil {
	// 	if err := syscall.Munmap(vm.exitCode); err != nil {
	// 		errs = append(errs, fmt.Errorf("exitCode: %w", err))
	// 	}
	// 	vm.exitCode = nil
	// }

	if len(errs) > 0 {
		return fmt.Errorf("Close encountered errors: %v", errs)
	}
	return nil
}

func (vm *RecompilerVM) Translate(bb *BasicBlock) error {
	defer func() {
		if r := recover(); r != nil {
			log.Error(vm.logging, "RecompilerVM Translate panic", "error", r)
			vm.ResultCode = types.PVM_PANIC
			vm.terminated = true
		}
	}()
	vm.initStartCode()
	vm.x86Code = vm.startCode
	for _, instruction := range bb.Instructions {
		opcode := instruction.Opcode
		fmt.Printf("Translating instruction %s (%d)\n", opcode_str(opcode), opcode)
		if translateFunc, exists := pvmByteCodeToX86Code[opcode]; exists {
			code := translateFunc(instruction)
			vm.x86Code = append(vm.x86Code, code...)
		} else {
			return fmt.Errorf("unknown opcode %s", opcode_str(opcode))
		}
	}
	lastInstruction := bb.Instructions[len(bb.Instructions)-1]
	if !IsBasicBlockInstruction(lastInstruction.Opcode) {
		bb.AddInstruction(TRAP, nil, 1000, 1000)
	}
	fmt.Printf("%s\n", bb.String())
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

// A.5.2. Instructions with Arguments of One Immediate. InstructionI1
func generateSyscall(inst Instruction) []byte {
	return []byte{0x0F, 0x05}
}

func generateFallthrough(inst Instruction) []byte {
	return []byte{0x90}
}

// LOAD_IMM_JUMP (only the “LOAD_IMM” portion)
func generateLoadImmJump(inst Instruction) []byte {
	// extractOneRegOneImmOneOffset returns (dstIdx, vx, vy)
	dstIdx, vx, _ := extractOneRegOneImmOneOffset(inst.Args)
	r := regInfoList[dstIdx]

	// Build REX prefix: REX.W plus REX.B if dst is r8–r15
	rex := byte(0x48) // 01001000b = REX.W=1
	if r.REXBit == 1 {
		rex |= 0x01 // set REX.B
	}

	// Opcode B8+rd  = MOV r64, imm64
	movOp := byte(0xB8 + r.RegBits)

	// Little‐endian encode the 64-bit immediate
	immBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(immBytes, vx)

	// [REX][MOV-opcode][imm64]
	return append([]byte{rex, movOp}, immBytes...)
}

// LOAD_IMM_JUMP_IND
func generateLoadImmJumpIndirect(inst Instruction) []byte {
	dstIdx, _, vx, _ := extractTwoRegsAndTwoImmediates(inst.Args)
	r := regInfoList[dstIdx]
	fmt.Printf("**** LOAD_IMM_JUMP_IND generateLoadImmJumpIndirect dstIdx %d vx=%d\n", dstIdx, vx)

	// Build REX prefix: REX.W plus REX.B if dst is r8–r15
	rex := byte(0x48) // 01001000b = REX.W=1
	if r.REXBit == 1 {
		rex |= 0x01 // set REX.B
	}

	// Opcode B8+rd  = MOV r64, imm64
	movOp := byte(0xB8 + r.RegBits)

	// Little‐endian encode the 64-bit immediate
	immBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(immBytes, vx)

	// [REX][MOV-opcode][imm64]
	return append([]byte{rex, movOp}, immBytes...)
}

// BRANCH_{EQ/NE/...}_IMM: r15 = (r64_reg jcc imm32) ? 1 : 0
func generateBranchImm(jcc byte) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		// unpack: register, immediate to compare, and branch offset (ignored here)
		regIdx, vx, _ := extractOneRegOneImmOneOffset(inst.Args)
		r := regInfoList[regIdx]

		// 1) CMP r64, imm32  →  opcode 0x81 /7 id
		rex := byte(0x48) // REX.W=1
		if r.REXBit == 1 {
			rex |= 0x01
		} // REX.B
		modrm := byte(0xC0 | (0x07 << 3) | r.RegBits)
		disp := int32(vx)
		b0 := byte(disp)
		b1 := byte(disp >> 8)
		b2 := byte(disp >> 16)
		b3 := byte(disp >> 24)
		code := []byte{rex, 0x81, modrm, b0, b1, b2, b3}

		// 2) SETcc r15b  →  0x0F, jcc, ModRM (mod=11, rm=r15.RegBits)
		dst15 := regInfoList[15]
		rexSet := byte(0x40)
		if dst15.REXBit == 1 {
			rexSet |= 0x01
		} // REX.B for r15b
		modrmSet := byte(0xC0 | dst15.RegBits)
		code = append(code, rexSet, 0x0F, jcc, modrmSet)

		// 3) MOVZX r64_r15, r15b  →  REX.W + 0F B6 /r
		rexZX := byte(0x48)
		if dst15.REXBit == 1 {
			rexZX |= 0x01
		}
		modrmZX := byte(0xC0 | (dst15.RegBits << 3) | dst15.RegBits)
		code = append(code, rexZX, 0x0F, 0xB6, modrmZX)
		return code
	}
}

// BRANCH_{EQ/NE/LT_U/...} (register–register): r15 = (rA <cond> rB) ? 1 : 0
func generateCompareBranch(op byte) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		// Unpack: two registers (rA, rB) and an ignored offset
		regA, regB, _ := extractTwoRegsOneOffset(inst.Args)
		rA := regInfoList[regA]
		rB := regInfoList[regB]

		// 1) CMP r64, r64  → opcode 0x39 /r (CMP r/m64, r64)
		rexCmp := byte(0x48) // REX.W
		if rB.REXBit == 1 {
			rexCmp |= 0x04 // REX.R
		}
		if rA.REXBit == 1 {
			rexCmp |= 0x01 // REX.B
		}
		modrmCmp := byte(0xC0 | (rB.RegBits << 3) | rA.RegBits)
		code := []byte{rexCmp, 0x39, modrmCmp}

		// 2) SETcc r15b  →  0x0F, jcc, ModRM (mod=11, rm=r15.RegBits)
		dst15 := regInfoList[15]
		rexSet := byte(0x40)
		if dst15.REXBit == 1 {
			rexSet |= 0x01
		} // REX.B for r15b
		modrmSet := byte(0xC0 | dst15.RegBits)
		code = append(code, rexSet, 0x0F, op, modrmSet)

		// 3) MOVZX r64_r15, r15b  →  REX.W + 0F B6 /r
		rexZX := byte(0x48)
		if dst15.REXBit == 1 {
			rexZX |= 0x01
		}
		modrmZX := byte(0xC0 | (dst15.RegBits << 3) | dst15.RegBits)
		code = append(code, rexZX, 0x0F, 0xB6, modrmZX)

		return code
	}
}
