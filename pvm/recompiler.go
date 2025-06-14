package pvm

import (
	"encoding/binary"
	"fmt"
	"runtime/debug"
	"strings"
	"syscall"
	"unsafe"

	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
	"golang.org/x/arch/x86/x86asm"
)

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
	// Allocate 4GB virtual memory region (not accessed directly here)
	const memSize = 4 * 1024 * 1024 * 1024 // 4GB
	mem, err := syscall.Mmap(
		-1, 0, memSize,
		syscall.PROT_NONE,
		syscall.MAP_ANON|syscall.MAP_PRIVATE,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to mmap memory: %v", err)
	}

	// Allocate memory to dump registers into
	dumpSize := len(regInfoList) * 8
	if dumpSize < 104 {
		dumpSize = 104
	}
	dumpMem, err := syscall.Mmap(
		-1, 0, dumpSize,
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_ANON|syscall.MAP_PRIVATE,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to mmap regDump memory: %v", err)
	}
	regDumpAddr := uintptr(unsafe.Pointer(&dumpMem[0]))
	//fmt.Printf("🧠 NewRecompilerVM: regDumpAddr = 0x%x (%d bytes)\n", regDumpAddr, dumpSize)

	// Build exit code in temporary buffer
	exitCode := encodeMovImm(BaseRegIndex, uint64(regDumpAddr))
	for i := 0; i < len(regInfoList); i++ {
		if i == BaseRegIndex {
			continue // skip R12 into [R12]
		}
		offset := byte(i * 8)
		exitCode = append(exitCode, encodeMovRegToMem(i, BaseRegIndex, offset)...)
	}

	exitCode = append(exitCode, 0xC3)

	// Allocate executable memory for exitCode
	execMem, err := syscall.Mmap(
		-1, 0, len(exitCode),
		syscall.PROT_READ|syscall.PROT_WRITE|syscall.PROT_EXEC,
		syscall.MAP_ANON|syscall.MAP_PRIVATE,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to mmap exec memory: %v", err)
	}
	copy(execMem, exitCode)

	// Assemble the VM
	rvm := &RecompilerVM{
		VM:          vm,
		realMemory:  mem,
		regDumpMem:  dumpMem,
		regDumpAddr: regDumpAddr,
		exitCode:    execMem,
	}
	//vm.Ram = rvm
	rvm.initStartCode()

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

func (vm *RecompilerVM) Translate(bb *BasicBlock) (err error) {

	defer func() {
		if r := recover(); r != nil {
			log.Error(vm.logging, "RecompilerVM Translate panic", "error", r)
			debug.PrintStack()
			vm.ResultCode = types.RESULT_PANIC
			vm.terminated = true
			err = fmt.Errorf("Translate panic: %v", r)
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
func (vm *RecompilerVM) ExecuteX86Code(x86code []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Error(vm.logging, "RecompilerVM ExecuteX86Code panic", "error", r)
			debug.PrintStack()
			vm.ResultCode = types.WORKRESULT_PANIC
			vm.terminated = true
			err = fmt.Errorf("ExecuteX86Code panic: %v", r)
		}
	}()

	codeAddr, err := syscall.Mmap(
		-1, 0, len(x86code),
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_ANON|syscall.MAP_PRIVATE,
	)
	if err != nil {
		return fmt.Errorf("failed to mmap exec code: %w", err)
	}
	copy(codeAddr, x86code)
	err = syscall.Mprotect(codeAddr, syscall.PROT_READ|syscall.PROT_EXEC)
	if err != nil {
		return fmt.Errorf("failed to mprotect exec code: %w", err)
	}

	// fmt.Printf("Executing at %p\n", &codeAddr[0])
	// fmt.Printf("DumpMem base: 0x%x\n", vm.regDumpAddr)

	crashed, err := ExecuteX86(codeAddr, vm.regDumpMem)

	if crashed == -1 || err != nil {
		vm.ResultCode = types.WORKRESULT_PANIC
		vm.terminated = true
		fmt.Printf("PANIC in ExecuteX86Code: %v\n", err)
		return fmt.Errorf("ExecuteX86 crash detected (return -1)")
	}

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

func generateTrap(inst Instruction) []byte {
	rel := 0xDEADBEEF
	return []byte{
		0xE9,                      // opcode
		byte(rel), byte(rel >> 8), // disp[0..1]
		byte(rel >> 16), byte(rel >> 24),
	}
}

func generateFallthrough(inst Instruction) []byte {
	return []byte{0x90}
}

func generateJump(inst Instruction) []byte {
	// For direct jumps, we can just append a jump instruction to a X86 PC
	x86Code := []byte{0xE9}  // JMP rel32
	jumpOffset := 0xDEADBEEF // placeholder for the jump offset, which we compute AFTER we know X86PC for each block
	x86Code = append(x86Code, byte(jumpOffset), byte(jumpOffset>>8), byte(jumpOffset>>16), byte(jumpOffset>>24))
	return x86Code
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

// generateBranchImm emits:
//
//	REX.W + CMP r64, imm32         (7 bytes total)
//	0x0F, jcc                      (2 bytes)
//	rel32 placeholder (true‐target) (4 bytes)
//	0xE9                           (1 byte JMP opcode)
//	rel32 placeholder (false‐target)(4 bytes)
//
// Total = 18 bytes
func generateBranchImm(jcc byte) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		regIdx, imm, _ := extractOneRegOneImmOneOffset(inst.Args)
		r := regInfoList[regIdx]

		// REX.W + REX.B if needed
		rex := byte(0x48)
		if r.REXBit == 1 {
			rex |= 0x01
		}

		// CMP r64, imm32 → opcode 0x81 /7 id
		modrm := byte(0xC0 | (0x7 << 3) | r.RegBits)
		disp := int32(imm)

		return []byte{
			// 7-byte compare
			rex, 0x81, modrm,
			byte(disp), byte(disp >> 8), byte(disp >> 16), byte(disp >> 24),

			// 2-byte conditional jump
			0x0F, jcc,

			// 4-byte placeholder for true target
			0, 0, 0, 0,

			// 1-byte unconditional JMP opcode
			0xE9,

			// 4-byte placeholder for false target
			0, 0, 0, 0,
		}
	}
}

func generateCompareBranch(jcc byte) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		aIdx, bIdx, _ := extractTwoRegsOneOffset(inst.Args)
		rA := regInfoList[aIdx]
		rB := regInfoList[bIdx]

		// REX.W + REX.R + REX.B
		rex := byte(0x48)
		if rB.REXBit == 1 {
			rex |= 0x04
		}
		if rA.REXBit == 1 {
			rex |= 0x01
		}

		// CMP r/m64, r64 (0x39 /r)
		modrm := byte(0xC0 | (rB.RegBits << 3) | rA.RegBits)

		return []byte{
			rex, 0x39, modrm, // 00–02
			0x0F, jcc, // 03–04
			0, 0, 0, 0, // 05–08 (relTrue placeholder)
			0xE9,       // 09 (JMP opcode)
			0, 0, 0, 0, // 10–13 (relFalse placeholder)
		}
	}
}
