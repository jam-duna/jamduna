package pvm

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/colorfulnotion/jam/common"
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
	dumpSize   = 0x100000
	PageSize   = 4096                   // 4 KiB
	TotalMem   = 4 * 1024 * 1024 * 1024 // 4 GiB
	TotalPages = TotalMem / PageSize    // 1,048,576 pages
)

/*
0 ---------------------- dumpAddr

	Register * 13 , 13 * 8 = 104

14 ---------------------

	Gas * 8 , 8 bytes

15 ---------------------

	pcBytes * 4 , 4 bytes
	empty 4 bytes for alignment

16 ---------------------

	blockCounter * 8 , 8 bytes
*/
const (
	gasSlotIndex          = 14 // Gas is at index 14
	pcSlotIndex           = 15 // PC is at index 15
	blockCounterSlotIndex = 16 // Block counter is at index 16

	indirectJumpPointSlot = 20 // Indirect jump point is at index 20
)

type CompilerVM struct {
	*VM
	*CompilerRam
	mu        sync.Mutex
	startCode []byte
	exitCode  []byte

	realCode []byte
	codeAddr uintptr

	x86Blocks map[uint64]*BasicBlock // by x86 PC
	x86PC     uint64
	x86Code   []byte

	JumpTableOffset  uint64         // offset for the jump table in x86Code
	JumpTableOffset2 uint64         // offset for the jump table in x86Code
	JumpTableMap     []uint64       // maps PVM PC to the index of the x86code (djump only)
	InstMapPVMToX86  map[uint32]int // maps PVM PC to the x86 PC index
	InstMapX86ToPVM  map[int]uint32 // maps x86 PC to the PVM PC

	djumpTableFunc []byte
	djumpAddr      uintptr // address of the jump table in x86Code

	//debug tool
	x86Instructions map[int]x86asm.Inst
	pc_addr         uint64
	r12             uint64
	isChargingGas   bool
	isPCCounting    bool
	IsBlockCounting bool // whether to count basic blocks
}

func NewCompilerVM(vm *VM) (*CompilerVM, error) {
	currentHeapPointer := vm.Ram.GetCurrentHeapPointer()
	ram, err := NewCompilerRam() // 1MB for register dump
	if err != nil {
		return nil, fmt.Errorf("failed to create CompilerRam: %w", err)
	}
	// Assemble the VM
	rvm := &CompilerVM{
		VM: vm,

		JumpTableMap:    make([]uint64, 0),
		InstMapX86ToPVM: make(map[int]uint32),
		InstMapPVMToX86: make(map[uint32]int),

		pc_addr: uint64(ram.regDumpAddr + uintptr((len(regInfoList)+1)*8)),

		isChargingGas:   true,         // default to charging gas
		isPCCounting:    useEcalli500, // default to counting PC
		IsBlockCounting: useEcalli500, // default to not counting basic blocks
	}

	rvm.VM.Ram = ram
	rvm.VM.Ram.SetCurrentHeapPointer(currentHeapPointer)
	rvm.CompilerRam = ram
	return rvm, nil
}

func NewCompilerVMFromRam(vm *VM, ram *CompilerRam) (*CompilerVM, error) {
	// Assemble the VM
	rvm := &CompilerVM{
		VM: vm,

		JumpTableMap:    make([]uint64, 0),
		InstMapX86ToPVM: make(map[int]uint32),
		InstMapPVMToX86: make(map[uint32]int),

		pc_addr: uint64(ram.regDumpAddr + uintptr((len(regInfoList)+1)*8)),

		isChargingGas:   true,         // default to charging gas
		isPCCounting:    useEcalli500, // default to counting PC
		IsBlockCounting: useEcalli500, // default to not counting basic blocks
	}

	rvm.VM.Ram = ram
	rvm.CompilerRam = ram
	return rvm, nil
}

// add jump indirects
const entryPatch = 0x99999999

func (vm *CompilerVM) initStartCode() {

	vm.startCode = append(vm.startCode, encodeMovImm(BaseRegIndex, uint64(vm.CompilerRam.realMemAddr))...)
	// initialize registers: mov rX, imm from vm.register
	for i := 0; i < regSize; i++ {
		immVal, _ := vm.Ram.ReadRegister(i)
		code := encodeMovImm(i, immVal)
		if showDisassembly {
			fmt.Printf("Initialize Register %d (%s) = %d\n", i, regInfoList[i].Name, immVal)
		}
		vm.startCode = append(vm.startCode, code...)
	}
	gasRegMemAddr := uint64(vm.CompilerRam.regDumpAddr) + uint64(len(regInfoList)*8)
	if showDisassembly {
		fmt.Printf("Initialize Gas %d = %d\n", gasRegMemAddr, vm.Gas)
	}
	vm.startCode = append(vm.startCode, encodeMovImm64ToMem(gasRegMemAddr, uint64(vm.Gas))...)

	// padding with jump to the entry point
	vm.startCode = append(vm.startCode, X86_OP_JMP_REL32) // JMP rel32
	// use entryPatch as a placeholder 0x99999999
	patch := make([]byte, 4)
	binary.LittleEndian.PutUint32(patch, entryPatch)
	vm.startCode = append(vm.startCode, patch...)

	// Build exit code in temporary buffer
	exitCode := encodeMovImm(BaseRegIndex, uint64(vm.CompilerRam.regDumpAddr))
	for i := 0; i < len(regInfoList); i++ {
		if i == BaseRegIndex {
			continue // skip R12 into [R12]
		}
		offset := byte(i * 8)
		exitCode = append(exitCode, encodeMovRegToMem(i, BaseRegIndex, offset)...)
	}
	vm.exitCode = append(exitCode, X86_OP_RET)
}
func (vm *CompilerRam) GetDirtyPages() []int {
	dirtyPages := make([]int, 0)
	for pageIndex, _ := range vm.dirtyPages {
		if vm.dirtyPages[pageIndex] {
			dirtyPages = append(dirtyPages, pageIndex)
		}
	}
	return dirtyPages
}

// initDJumpFunc initializes the dynamic jump table function for indirect jumps.
// It generates x86 code that performs a series of checks and dispatches to the correct handler
// based on the value in jumpIndTempReg (typically RCX). The generated code handles the following cases:
//
//  1. If jumpIndTempReg == 0xFFFF0000, return (ret_stub).
//  2. If jumpIndTempReg == 0, trigger a panic (panic_stub).
//  3. If jumpIndTempReg > threshold, trigger a panic (panic_stub).
//  4. If jumpIndTempReg is odd (jumpIndTempReg % 2 != 0), trigger a panic (panic_stub).
//
// If all checks pass, it computes the handler address and jumps to the appropriate handler.
// The function also patches the generated code with the correct relative offsets for jumps and handlers.
func (vm *CompilerVM) initDJumpFunc(x86CodeLen int) {
	type pending struct {
		jeOff   int
		handler uintptr
	}

	code := make([]byte, 0)
	var pendings []pending

	// ==== Pre-checks: CMP/JE placeholders ====

	// (a) If jumpIndTempReg == 0xFFFF0000, jump to ret_stub
	rex := byte(X86_REX_BASE)
	if jumpIndTempReg.REXBit == 1 {
		rex |= X86_REX_B // REX.B
	}
	modrm := byte(X86_MOD_REGISTER<<6 | (7 << 3) | jumpIndTempReg.RegBits)
	code = append(code,
		rex,
		X86_OP_GROUP1_RM_IMM32, // CMP r/m32, imm32
		modrm,
		0x00, 0x00, 0xFF, 0xFF, // 0xFFFF0000
	)
	offJEret := len(code)
	code = append(code, emitJeInitDJump()...)

	// (b) If jumpIndTempReg == 0, jump to panic_stub
	rex = X86_REX_W_PREFIX
	if jumpIndTempReg.REXBit == 1 {
		rex |= X86_REX_B
	}
	modrm = byte(0xF8 | jumpIndTempReg.RegBits)
	code = append(code, rex, X86_OP_GROUP1_RM_IMM8, modrm, X86_IMM_0)
	offJE0 := len(code)
	code = append(code, emitJeInitDJump()...)

	// (c) If jumpIndTempReg > threshold, jump to panic_stub
	threshold := uint32(len(vm.JumpTableMap)) * uint32(Z_A)
	var thr [4]byte
	binary.LittleEndian.PutUint32(thr[:], threshold)
	rex = X86_REX_W_PREFIX
	if jumpIndTempReg.REXBit == 1 {
		rex |= X86_REX_B
	}
	modrm = byte(0xF8 | jumpIndTempReg.RegBits)
	code = append(code, rex, X86_OP_GROUP1_RM_IMM32, modrm)
	code = append(code, thr[:]...)
	offJA := len(code)
	code = append(code, emitJaInitDJump()...)

	// (d) If jumpIndTempReg is odd (jumpIndTempReg % 2 != 0), jump to panic_stub
	rex = X86_REX_W_PREFIX
	if jumpIndTempReg.REXBit == 1 {
		rex |= X86_REX_B
	}
	modrm = byte(X86_MOD_REGISTER<<6 | jumpIndTempReg.RegBits)
	code = append(code, rex, X86_OP_GROUP3_RM, modrm, X86_IMM_1, 0, 0, 0)
	offJNZ := len(code)
	code = append(code, emitJneInitDJump()...)

	// Collect panic jump offsets for later patching
	panicOffs := []int{offJE0 + 2, offJA + 2, offJNZ + 2}

	// ==== Continue execution after checks ====
	// Compute (jumpIndTempReg / 2 - 1) * 7, then load current PC into RAX

	// 1. SAR jumpIndTempReg, 1    ; Arithmetic right shift by 1 (signed division by 2)
	rex = byte(X86_REX_W_PREFIX)
	if jumpIndTempReg.REXBit == 1 {
		rex |= X86_REX_B
	}
	modrm = byte(X86_MOD_REGISTER<<6 | (7 << 3) | jumpIndTempReg.RegBits)
	code = append(code,
		rex,
		X86_OP_GROUP2_RM_IMM8,
		modrm,
		X86_IMM_1,
	)

	// 2. SUB jumpIndTempReg, 1
	rex = byte(X86_REX_W_PREFIX)
	if jumpIndTempReg.REXBit == 1 {
		rex |= X86_REX_B
	}
	modr := byte(X86_MOD_REGISTER<<6 | (5 << 3) | jumpIndTempReg.RegBits)
	code = append(code,
		rex,
		X86_OP_GROUP1_RM_IMM8,
		modr,
		X86_IMM_1,
	)

	// 3. IMUL jumpIndTempReg, jumpIndTempReg, 7
	rex = byte(X86_REX_W_PREFIX)
	if jumpIndTempReg.REXBit == 1 {
		rex |= (X86_REX_R | X86_REX_B)
	}
	modrm = byte(X86_MOD_REGISTER<<6 | (jumpIndTempReg.RegBits << 3) | jumpIndTempReg.RegBits)
	imm := []byte{0x07, 0x00, 0x00, 0x00}
	code = append(code, rex, X86_OP_IMUL_R_RM, modrm)
	code = append(code, imm...)

	// 4. PUSH RAX
	code = append(code, emitPushReg(RAX)...)

	// 5. LEA RAX, [RIP+0] ; Load current instruction address into RAX
	code = append(code, emitLeaRaxRipInitDJump()...)

	// 6. ADD jumpIndTempReg, RAX
	rex = byte(X86_REX_W_PREFIX)
	if jumpIndTempReg.REXBit == 1 {
		rex |= X86_REX_B
	}
	modrm = byte(X86_MOD_REGISTER<<6 | (0 << 3) | jumpIndTempReg.RegBits)
	code = append(code, rex, X86_OP_ADD_RM_R, modrm)

	// 7. POP RAX
	code = append(code, emitPopReg(RAX)...)

	// 8. ADD jumpIndTempReg, imm32 (handler address offset, to be patched later)
	handlerAddOff := len(code) + 3
	code = append(code, rex, X86_OP_GROUP1_RM_IMM32, modrm, 0xEF, 0xBE, 0xAD, 0xDE)

	// 9. JMP jumpIndTempReg
	rex = X86_REX_W_PREFIX
	if jumpIndTempReg.REXBit == 1 {
		rex |= X86_REX_B
	}
	modrm = 0xE0 | jumpIndTempReg.RegBits
	code = append(code, rex, 0xFF, modrm)

	// Collect handler jump locations for patching
	for _, idx := range vm.JumpTableMap {
		pendings = append(pendings, pending{
			handler: uintptr(idx),
		})
	}

	// Patch panic jumps to point to the panic stub
	panicStubAddr := vm.codeAddr + uintptr(len(code))
	for _, off := range panicOffs {
		rel := int32(int64(panicStubAddr) - int64(vm.codeAddr) - int64(off) - 4)
		binary.LittleEndian.PutUint32(code[off:], uint32(rel))
	}

	// Panic stub: POP jumpIndTempReg, then UD2 (undefined instruction)
	code = append(code, rex, byte(0x58|jumpIndTempReg.RegBits))
	code = append(code, emitUd2InitDJump()...)

	// Patch JE ret stub to point to the return stub
	retStubAddr := vm.codeAddr + uintptr(len(code))
	relRet := int32(int64(retStubAddr) - int64(vm.codeAddr) - int64(offJEret) - 6)
	binary.LittleEndian.PutUint32(code[offJEret+2:offJEret+6], uint32(relRet))

	// Return stub: POP jumpIndTempReg, then exit code
	code = append(code, emitPopReg(jumpIndTempReg)...)
	code = append(code, vm.exitCode...)

	// Patch handler jumps for each pending handler
	for i, p := range pendings {
		// POP jumpIndTempReg then JMP handler
		pendings[i].jeOff = len(code)
		code = append(code, rex, byte(0x58|jumpIndTempReg.RegBits))
		toMinus := x86CodeLen + len(code) - int(p.handler)
		num := int32(-toMinus - 5)
		buf := make([]byte, 4)
		binary.LittleEndian.PutUint32(buf, uint32(num))
		code = append(code, emitJmpE9InitDJump()...)
		code = append(code, buf...)
	}

	if len(pendings) == 0 {
		fmt.Println("No pending handlers found, skipping handler patching.")
		vm.djumpTableFunc = code
		return
	}
	firstPending := pendings[0]
	toAdd := firstPending.jeOff - handlerAddOff + 7
	// Patch the handler address offset into the ADD instruction
	uint64ToAdd := uint64(toAdd)
	binary.LittleEndian.PutUint32(code[handlerAddOff:], uint32(uint64ToAdd))
	vm.djumpTableFunc = code
}

func (vm *CompilerVM) Close() error {
	var errs []error

	if vm.x86Code != nil {
		if err := syscall.Munmap(vm.x86Code); err != nil {
			errs = append(errs, fmt.Errorf("x86Code: %w", err))
		}
		vm.x86Code = nil
	}

	if len(errs) > 0 {
		return fmt.Errorf("Close encountered errors: %v", errs)
	}

	return nil
}

func (rvm *CompilerVM) Disassemble(code []byte) string {
	var sb strings.Builder
	offset := 0
	if rvm.x86Instructions == nil {
		rvm.x86Instructions = make(map[int]x86asm.Inst)
	}
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
		rvm.x86Instructions[offset] = inst
		offset += length
	}
	return sb.String()
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

func (vm *CompilerVM) ExecuteX86Code(x86code []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Error(vm.logging, "CompilerVM ExecuteX86Code panic", "error", r)
			debug.PrintStack()
			vm.ResultCode = types.WORKDIGEST_PANIC
			vm.MachineState = PANIC
			vm.terminated = true
			err = fmt.Errorf("ExecuteX86Code panic: %v", r)
		}
	}()
	vm.initDJumpFunc(len(x86code))
	codeAddr, err := syscall.Mmap(
		-1, 0, len(x86code)+len(vm.djumpTableFunc),
		syscall.PROT_READ|syscall.PROT_WRITE|syscall.PROT_EXEC,
		syscall.MAP_ANON|syscall.MAP_PRIVATE,
	)
	if err != nil {
		return fmt.Errorf("failed to mmap exec code: %w", err)
	}

	vm.realCode = codeAddr
	vm.codeAddr = uintptr(unsafe.Pointer(&vm.realCode[0]))
	if err != nil {
		return fmt.Errorf("failed to mmap djumpTableFunc: %w", err)
	}
	// ca := make([]byte, 8)
	// binary.LittleEndian.PutUint64(ca, uint64(vm.codeAddr))
	// for c := 0; c < int(vm.JumpTableOffset2); c++ {
	// 	if x86code[c] == 0xba && x86code[c+1] == 0xef && x86code[c+2] == 0xef && x86code[c+3] == 0xef {
	// 		copy(x86code[c+1:c+9], ca)
	// 	}
	// }
	vm.djumpAddr = vm.codeAddr + uintptr(len(x86code))
	vm.finalizeJumpTargets(vm.J)

	copy(codeAddr, x86code)
	copy(codeAddr[len(x86code):], vm.djumpTableFunc)
	err = syscall.Mprotect(codeAddr, syscall.PROT_READ|syscall.PROT_EXEC)
	if err != nil {
		return fmt.Errorf("failed to mprotect exec code: %w", err)
	}

	save := false
	if save {
		// Ensure the directory exists
		if err := syscall.Mkdir("test_code", 0755); err != nil && err != syscall.EEXIST {
			return fmt.Errorf("failed to create directory: %w", err)
		}
		file, err := syscall.Open("test_code/output.bin", syscall.O_WRONLY|syscall.O_CREAT|syscall.O_TRUNC, 0644)
		if err != nil {
			return fmt.Errorf("failed to open output file: %w", err)
		}
		defer syscall.Close(file)
		// write the x86 code to the file
		if _, err := syscall.Write(file, x86code); err != nil {
			return fmt.Errorf("failed to write x86 code to file: %w", err)
		}
		// write the djumpTableFunc to the file
	}
	if showDisassembly {
		str := vm.Disassemble(vm.realCode)
		fmt.Printf("ALL COMBINED Disassembled x86 code:\n%s\n", str)
	}
	crashed, _, err := ExecuteX86(codeAddr, vm.regDumpMem)
	for i := 0; i < regSize; i++ {
		regValue := binary.LittleEndian.Uint64(vm.regDumpMem[i*8:])
		if showDisassembly {
			fmt.Printf("%s = %d\n", regInfoList[i].Name, regValue)
		}
		vm.Ram.WriteRegister(i, regValue)
	}
	//vm.SetIdentifier(fmt.Sprintf("%d", msec))
	if crashed == -1 || err != nil {
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		vm.terminated = true
		fmt.Printf("PANIC in ExecuteX86Code: %v\n", err)
		fmt.Printf("codeAddr: 0x%x\n", vm.codeAddr)
		fmt.Printf("djumpAddr: 0x%x\n", vm.djumpAddr)
		fmt.Printf("sbrk address: 0x%x\n", GetSbrkAddress())
		fmt.Printf("Ecall address: 0x%x\n", GetEcalliAddress())
		return fmt.Errorf("ExecuteX86 crash detected (return -1)")
	}

	for i := 0; i < regSize; i++ {
		regValue := binary.LittleEndian.Uint64(vm.regDumpMem[i*8:])
		if showDisassembly {
			fmt.Printf("%s = %d\n", regInfoList[i].Name, regValue)
		}
		vm.Ram.WriteRegister(i, regValue)
	}
	return nil
}

func (vm *CompilerVM) ExecuteX86CodeWithEntry(x86code []byte, entry uint32) (err error) {
	startTime := time.Now()
	vm.initDJumpFunc(len(x86code))
	codeAddr, err := syscall.Mmap(
		-1, 0, len(x86code)+len(vm.djumpTableFunc),
		syscall.PROT_READ|syscall.PROT_WRITE|syscall.PROT_EXEC,
		syscall.MAP_ANON|syscall.MAP_PRIVATE,
	)
	if err != nil {
		return fmt.Errorf("failed to mmap exec code: %w", err)
	}

	vm.realCode = codeAddr
	vm.codeAddr = uintptr(unsafe.Pointer(&vm.realCode[0]))
	if err != nil {
		return fmt.Errorf("failed to mmap djumpTableFunc: %w", err)
	}
	// ca := make([]byte, 8)
	// binary.LittleEndian.PutUint64(ca, uint64(vm.codeAddr))
	// for c := 0; c < int(vm.JumpTableOffset2); c++ {
	// 	if x86code[c] == 0xba && x86code[c+1] == 0xef && x86code[c+2] == 0xef && x86code[c+3] == 0xef {
	// 		copy(x86code[c+1:c+9], ca)
	// 	}
	// }
	vm.djumpAddr = vm.codeAddr + uintptr(len(x86code))
	vm.finalizeJumpTargets(vm.J)

	var patchInstIdx = -1
	entryPatchImm := entryPatch
	// use entryPatch as a placeholder 0x99999999
	//get the x86 pc
	x86PC, ok := vm.InstMapPVMToX86[entry]
	if !ok && entry != 0 {
		return fmt.Errorf("entry %d not found in InstMapPVMToX86", entry)
	}
	if debugCompiler {
		fmt.Printf("Executing code at x86 PC: %d (PVM PC: %d)\n", x86PC, entry)
	}
	patch := make([]byte, 4)
	binary.LittleEndian.PutUint32(patch, entryPatch)
	for i := 0; i < len(x86code)-5; i++ {
		if x86code[i] == 0xE9 && // JMP rel32
			x86code[i+1] == 0x99 &&
			x86code[i+2] == 0x99 &&
			x86code[i+3] == 0x99 &&
			x86code[i+4] == 0x99 {
			// found a placeholder for the entry patch
			patchInstIdx = i
			// replace it with the actual entry patch
			binary.LittleEndian.PutUint32(x86code[i+1:i+5], uint32(x86PC-i-5))
			if showDisassembly {
				fmt.Printf("Patching entry point at index %d with 0x%X\n", patchInstIdx, entryPatchImm)
			}
			break
		}
	}
	vm.WriteContextSlot(indirectJumpPointSlot, uint64(vm.djumpAddr), 8)

	// if patchInstIdx == -1 {
	// 	return fmt.Errorf("no entry patch placeholder found in x86 code")
	// }
	copy(codeAddr, x86code)
	copy(codeAddr[len(x86code):], vm.djumpTableFunc)
	err = syscall.Mprotect(codeAddr, syscall.PROT_READ|syscall.PROT_EXEC)
	if err != nil {
		return fmt.Errorf("failed to mprotect exec code: %w", err)
	}

	save := false
	if save {
		// Ensure the directory exists
		if err := syscall.Mkdir("test_code", 0755); err != nil && err != syscall.EEXIST {
			return fmt.Errorf("failed to create directory: %w", err)
		}
		file, err := syscall.Open("test_code/output.bin", syscall.O_WRONLY|syscall.O_CREAT|syscall.O_TRUNC, 0644)
		if err != nil {
			return fmt.Errorf("failed to open output file: %w", err)
		}
		defer syscall.Close(file)
		// write the x86 code to the file
		if _, err := syscall.Write(file, x86code); err != nil {
			return fmt.Errorf("failed to write x86 code to file: %w", err)
		}
		// write the djumpTableFunc to the file
	}

	if showDisassembly {
		str := vm.Disassemble(vm.realCode)
		fmt.Printf("ALL COMBINED Disassembled x86 code:\n%s\n", str)
	}
	vm.compileTime += common.Elapsed(startTime)
	startTime = time.Now()
	crashed, _, err := ExecuteX86(codeAddr, vm.regDumpMem)
	//vm.SetIdentifier(fmt.Sprintf("%d", msec))
	for i := 0; i < regSize; i++ {
		regValue := binary.LittleEndian.Uint64(vm.regDumpMem[i*8:])
		if showDisassembly {
			fmt.Printf("%s = %d\n", regInfoList[i].Name, regValue)
		}
		vm.Ram.WriteRegister(i, regValue)
	}
	gas, err := vm.ReadContextSlot(gasSlotIndex)
	if err != nil {
		return fmt.Errorf("failed to read gas from context slot: %w", err)
	}
	vm.Gas = int64(gas)
	if crashed == -1 || err != nil {
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		vm.terminated = true
		fmt.Printf("PANIC in ExecuteX86Code: %v\n", err)
		fmt.Printf("codeAddr: 0x%x\n", vm.codeAddr)
		fmt.Printf("djumpAddr: 0x%x\n", vm.djumpAddr)
		fmt.Printf("realMemory address: 0x%x\n", vm.realMemAddr)
		fmt.Printf("sbrk address: 0x%x\n", GetSbrkAddress())
		fmt.Printf("Ecall address: 0x%x\n", GetEcalliAddress())
		return fmt.Errorf("ExecuteX86 crash detected (return -1)")
	}

	for i := 0; i < regSize; i++ {
		regValue := binary.LittleEndian.Uint64(vm.regDumpMem[i*8:])
		if showDisassembly {
			fmt.Printf("%s = %d\n", regInfoList[i].Name, regValue)
		}
		vm.Ram.WriteRegister(i, regValue)
	}
	vm.executionTime = common.Elapsed(startTime)
	return nil
}

func (rvm *CompilerVM) Execute(entry uint32) {
	startTime := time.Now()
	rvm.pc = 0

	rvm.initStartCode()
	rvm.Compile(rvm.pc)
	rvm.compileTime = common.Elapsed(startTime)
	if err := rvm.ExecuteX86CodeWithEntry(rvm.x86Code, entry); err != nil {
		// we don't have to return this , just print it
		fmt.Printf("ExecuteX86 crash detected: %v\n", err)
	}

}

func (vm *VM) CalculateTally() {
	//	fmt.Println("Basic Block Execution Tally:")
	// Collect and sort PCs
	var pcs []int
	for pc := range vm.basicBlockExecutionCounter {
		pcs = append(pcs, int(pc))
	}
	slices.Sort(pcs)
	for _, pc := range pcs {
		count := vm.basicBlockExecutionCounter[uint64(pc)]
		if count > 0 {
			bb, ok := vm.basicBlocks[uint64(pc)]
			if !ok {
				panic(fmt.Sprintf("Basic Block not found for PC: %d", pc))
			}

			for _, inst := range bb.Instructions {
				if vm.OP_tally != nil {
					_, ok := vm.OP_tally[opcode_str(inst.Opcode)]
					if ok {
						vm.OP_tally[opcode_str(inst.Opcode)].ExeCount += count
						// fmt.Printf("PC: %d, Opcode: %s, Count: %d\n", pc, opcode_str(inst.Opcode), count)
					}
				}
			}
		}
	}
	// fmt.Println("End of Basic Block Execution Tally")
}

func (vm *VM) ResetTally() {
	for _, v := range vm.OP_tally {
		v.ExeCount = 0
	}
}

// Standard_Program_Initialization initializes the program memory and registers
func (vm *CompilerVM) Standard_Program_Initialization(argument_data_a []byte) (err error) {

	if len(argument_data_a) == 0 {
		argument_data_a = []byte{0}
	}
	//1)
	// o_byte
	o_len := len(vm.o_byte)
	if err = vm.SetMemAccess(Z_Z, uint32(o_len), PageMutable); err != nil {
		return fmt.Errorf("SetMemAccess1 failed o_len=%d (o_byte): %w", o_len, err)
	}
	if err = vm.WriteMemory(Z_Z, vm.o_byte); err != nil {
		return fmt.Errorf("WriteMemory failed (o_byte): %w", err)
	}
	if err = vm.SetMemAccess(Z_Z, uint32(o_len), PageImmutable); err != nil {
		return fmt.Errorf("SetMemAccess2 failed (o_byte): %w", err)
	}
	//2)
	//p|o|
	p_o_len := P_func(uint32(o_len))
	if err = vm.SetMemAccess(Z_Z+uint32(o_len), p_o_len, PageImmutable); err != nil {
		return fmt.Errorf("SetMemAccess failed (p_o_byte): %w", err)
	}

	z_o := Z_func(vm.o_size)
	z_w := Z_func(vm.w_size + vm.z*Z_P)
	z_s := Z_func(vm.s)
	requiredMemory := uint64(5*Z_Z + z_o + z_w + z_s + Z_I)
	if requiredMemory > math.MaxUint32 {
		log.Error(vm.logging, "Standard Program Initialization Error")
		return
	}
	// 3)
	// w_byte
	w_addr := 2*Z_Z + z_o
	w_len := uint32(len(vm.w_byte))
	if err = vm.SetMemAccess(w_addr, w_len, PageMutable); err != nil {
		return fmt.Errorf("SetMemAccess failed (w_byte): %w", err)
	}
	if err = vm.WriteMemory(w_addr, vm.w_byte); err != nil {
		return fmt.Errorf("WriteMemory failed (w_byte): %w", err)
	}
	// 4)
	addr4 := 2*Z_Z + z_o + w_len
	little_z := vm.z
	len4 := P_func(w_len) + little_z*Z_P - w_len
	if err = vm.SetMemAccess(addr4, len4, PageMutable); err != nil {
		return fmt.Errorf("SetMemAccess failed (addr4): %w", err)
	}
	// 5)
	addr5 := 0xFFFFFFFF + 1 - 2*Z_Z - Z_I - P_func(vm.s)
	len5 := P_func(vm.s)
	if err = vm.SetMemAccess(addr5, len5, PageMutable); err != nil {
		return fmt.Errorf("SetMemAccess failed (addr5): %w", err)
	}
	// 6)
	argAddr := uint32(0xFFFFFFFF) - Z_Z - Z_I + 1
	if err = vm.SetMemAccess(argAddr, uint32(len(argument_data_a)), PageMutable); err != nil {
		return fmt.Errorf("SetMemAccess failed (argAddr): %w", err)
	}
	if err = vm.WriteMemory(argAddr, argument_data_a); err != nil {
		return fmt.Errorf("WriteMemory failed (argAddr): %w", err)
	}
	// set it back to immutable
	if err = vm.SetMemAccess(argAddr+uint32(len(argument_data_a)), Z_I, PageImmutable); err != nil {
		return fmt.Errorf("SetMemAccess failed (argAddr+len): %w", err)
	}
	// 7)
	addr7 := argAddr + uint32(len(argument_data_a))
	len7 := argAddr + P_func(uint32(len(argument_data_a))) - addr7
	if err = vm.SetMemAccess(addr7, len7, PageImmutable); err != nil {
		return fmt.Errorf("SetMemAccess failed (addr7): %w", err)
	}

	vm.Ram.WriteRegister(0, uint64(0xFFFFFFFF-(1<<16)+1))
	vm.Ram.WriteRegister(1, uint64(0xFFFFFFFF-2*Z_Z-Z_I+1))
	vm.Ram.WriteRegister(7, uint64(argAddr))
	vm.Ram.WriteRegister(8, uint64(uint32(len(argument_data_a))))
	return nil
}

func (vm *CompilerVM) WriteContextSlot(slot_index int, value uint64, size int) error {
	if vm.regDumpAddr == 0 {
		return fmt.Errorf("regDumpAddr is not initialized")
	}
	addr := uintptr(slot_index * 8)

	switch size {
	case 4:
		var buf [8]byte
		binary.LittleEndian.PutUint32(buf[:4], uint32(value))
		copy(vm.regDumpMem[addr:], buf[:])
	case 8:
		var buf [8]byte
		binary.LittleEndian.PutUint64(buf[:], value)
		copy(vm.regDumpMem[addr:], buf[:])
	default:
		return fmt.Errorf("unsupported size: %d", size)
	}
	return nil
}

func (vm *CompilerVM) ReadContextSlot(slot_index int) (uint64, error) {
	if vm.regDumpAddr == 0 {
		return 0, fmt.Errorf("regDumpAddr is not initialized")
	}
	addr := uintptr(slot_index * 8)
	var value uint64
	// just read it out
	data := vm.regDumpMem[addr : addr+8]
	if len(data) < 8 {
		return 0, fmt.Errorf("not enough data to read from regDumpMem at index %d", slot_index)
	}
	value = binary.LittleEndian.Uint64(data)
	return value, nil
}

// A.5.2. Instructions with Arguments of One Immediate. InstructionI1

func generateTrap(inst Instruction) []byte {
	// rel := 0xDEADBEEF
	return emitTrap()
}

func generateFallthrough(inst Instruction) []byte {
	return emitNop()
}

func generateJump(inst Instruction) []byte {
	// For direct jumps, we can just append a jump instruction to a X86 PC
	// The displacement 0xFEFEFEFE will be patched by the VM
	return emitJmpWithPlaceholder()
}

// LOAD_IMM_JUMP (only the “LOAD_IMM” portion)
func generateLoadImmJump(inst Instruction) []byte {
	dstIdx, vx, _ := extractOneRegOneImmOneOffset(inst.Args)
	r := regInfoList[dstIdx]

	var code []byte
	// Generate MOV instruction using matching helper
	code = append(code, emitMovImmToReg64(r, vx)...)

	// Generate JMP instruction using matching helper
	code = append(code, emitJmpWithPlaceholder()...)
	return code
}

var jumpIndTempReg = RCX

func generateJumpIndirect(inst Instruction) []byte {
	// 1) Extract baseIdx and vx
	operands := slices.Clone(inst.Args)
	baseIdx := min(12, int(operands[0])&0x0F)
	lx := min(4, max(0, len(operands))-1)
	if lx == 0 {
		lx = 1
		operands = append(operands, 0)
	}
	vx := x_encode(types.DecodeE_l(operands[1:1+lx]), uint32(lx))
	base := regInfoList[baseIdx]

	buf := make([]byte, 0, 32)

	// 1) PUSH jumpIndTempReg with exact manual construction
	buf = append(buf, emitPushRegJumpIndirect(jumpIndTempReg)...)

	// 2) MOV jumpIndTempReg, [base] with exact manual construction
	buf = append(buf, emitMovRegFromMemJumpIndirect(jumpIndTempReg, base)...)

	// 3) ADD jumpIndTempReg, vx with exact manual construction
	buf = append(buf, emitAddRegImm32_100(jumpIndTempReg, uint32(vx))...)

	// 4) JMP [BaseReg + offset] (already uses emit helper)
	buf = append(buf, generateJumpRegMem(BaseReg, indirectJumpPointSlot*8-dumpSize)...)

	return buf
}

// LOAD_IMM_JUMP_IND with jumpIndTempReg
func generateLoadImmJumpIndirect(inst Instruction) []byte {

	dstIdx, indexRegIdx, vx, vy := extractTwoRegsAndTwoImmediates(inst.Args)
	r := regInfoList[dstIdx]
	indexReg := regInfoList[indexRegIdx]

	var buf []byte

	// 1) PUSH jumpIndTempReg
	buf = append(buf, emitPushRegLoadImmJumpIndirect(jumpIndTempReg)...)

	// 2) MOV jumpIndTempReg, indexReg
	buf = append(buf, emitMovRegToRegLoadImmJumpIndirect(jumpIndTempReg, indexReg)...)

	// 3) MOV r64 (dst) <- imm64 (vx)
	buf = append(buf, emitMovImmToRegLoadImmJumpIndirect(r, vx)...)

	// 4) ADD jumpIndTempReg, vy
	buf = append(buf, emitAddRegImm32LoadImmJumpIndirect(jumpIndTempReg, uint32(vy))...)

	// 5) AND jumpIndTempReg, 0xFFFFFFFF (zero-extend)
	buf = append(buf, emitAndRegImm32LoadImmJumpIndirect(jumpIndTempReg)...)

	// 6) JMP [TempReg + indirectJumpPointSlot*8 - dumpSize]
	buf = append(buf, generateJumpRegMem(BaseReg, indirectJumpPointSlot*8-dumpSize)...)
	return buf
}

// generateBranchImm emits:
//
//	REX.W + CMP r64, imm32         (7 bytes total)
//	0x0F, jcc                      (2 bytes)
//	rel32 placeholder (true‐target) (4 bytes)
//
// Total = 13 bytes
func generateBranchImm(jcc byte) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		regIdx, imm, _ := extractOneRegOneImmOneOffset(inst.Args)
		r := regInfoList[regIdx]

		var buf []byte

		// Generate CMP r64, imm32 using exact matching helper
		buf = append(buf, emitCmpRegImm32Force81(r, int32(imm))...)

		// Generate conditional jump using exact matching helper
		buf = append(buf, emitJccWithPlaceholder(jcc)...)

		return buf
	}
}

// generateCompareBranch creates a function to handle conditional branches between two registers.
// It uses the classic, robust two-jump sequence which works regardless of block layout.
// This predictable 14-byte output allows a separate patching pass to safely optimize it.
func generateCompareBranch(jcc byte) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		aIdx, bIdx, _ := extractTwoRegsOneOffset(inst.Args)
		rA := regInfoList[aIdx]
		rB := regInfoList[bIdx]

		// Use helper function for CMP rA, rB (order: CMP b, a due to parameter convention)
		cmpBytes := emitCmpReg64(rB, rA)

		// This robust sequence uses a conditional jump to the true case
		// and an unconditional jump to the false case. It is 14 bytes long.
		code := make([]byte, 0, 14)
		code = append(code, cmpBytes...) // CMP rA, rB
		code = append(code,
			0x0F, jcc, // Jcc rel32 (jump to the "true" branch)
			0, 0, 0, 0, // Placeholder for relTrue dd
		)
		return code
	}
}

func (vm *CompilerVM) GetMemory() (map[int][]byte, map[int]int) {
	memory := make(map[int][]byte)
	pageMap := make(map[int]int)
	for index, _ := range vm.dirtyPages {
		pageMap[index] = 1 // Initialize with 0 access count
	}
	for i := 0; i < TotalPages; i++ {
		if _, ok := pageMap[i]; ok {
			// fmt.Printf("Page %d: access %d\n", i, access)
			data, err := vm.ReadMemory(uint32(i*PageSize), PageSize)
			if err != nil {
				log.Error(vm.logging, "GetMemory ReadMemory failed", "error", err)
				continue
			}
			memory[i] = data
			// fmt.Printf("Page %d: %v\n", i, common.BytesToHash(data))
		}
	}
	return memory, pageMap
}

type X86InstTally struct {
	PVM_OP           string                       `json:"pvm_op"`
	X86_Map          map[string]*X86InternalTally `json:"x86_map"`
	TotalX86         int                          `json:"total_x86_insts"`
	TotalPVM         int                          `json:"total_pvm_insts"`    // number of times PVM_OP appeared in code
	ExeCount         int                          `json:"execution_count"`    // number of times PVM_OP executed
	AverageX86Insts  float64                      `json:"average_x86_insts"`  // TotalX86 / TotalPVM
	WeightedX86Insts float64                      `json:"weighted_x86_insts"` // ExeCount * AverageX86Insts
}

type X86InternalTally struct {
	X86_OP string `json:"x86_op"`
	Count  int    `json:"count"`
}

func (vm *CompilerVM) AddPVMCount(pvm_OP string) {
	if vm.OP_tally == nil {
		vm.OP_tally = make(map[string]*X86InstTally)
	}
	entry, ok := vm.OP_tally[pvm_OP]
	if !ok {
		entry = &X86InstTally{
			PVM_OP:  pvm_OP,
			X86_Map: make(map[string]*X86InternalTally),
		}
		vm.OP_tally[pvm_OP] = entry
	}
	entry.TotalPVM++
}

func (vm *CompilerVM) DisassembleAndTally(pvm_OP_Code byte, code []byte) string {
	pvm_OP := opcode_str(pvm_OP_Code)

	// bump the PVM count once
	vm.AddPVMCount(pvm_OP)

	var sb strings.Builder
	offset := 0
	if vm.x86Instructions == nil {
		vm.x86Instructions = make(map[int]x86asm.Inst)
	}

	for offset < len(code) {
		inst, err := x86asm.Decode(code[offset:], 64)
		length := inst.Len
		if err != nil || length == 0 {
			sb.WriteString(fmt.Sprintf("0x%04x: db 0x%02x\n", offset, code[offset]))
			offset++
			continue
		}

		// tally the individual x86 instruction
		mnemonic := inst.Op.String()
		vm.AddTally(pvm_OP, mnemonic)

		// …rest of your disassembly printing…
		offset += length
	}
	return sb.String()
}

func (vm *VM) AverageTally() {
	if vm.OP_tally == nil {
		return
	}
	for _, entry := range vm.OP_tally {
		appearTimes := entry.TotalPVM
		if appearTimes > 0 {
			entry.AverageX86Insts = float64(entry.TotalX86) / float64(appearTimes)
			entry.WeightedX86Insts = float64(entry.ExeCount) * entry.AverageX86Insts
		}
	}
}

func (vm *VM) AddTally(pvm_OP string, x86_OP string) {
	if vm.OP_tally == nil {
		vm.OP_tally = make(map[string]*X86InstTally)
	}
	entry, ok := vm.OP_tally[pvm_OP]
	if !ok {
		entry = &X86InstTally{
			PVM_OP:  pvm_OP,
			X86_Map: make(map[string]*X86InternalTally),
		}
		vm.OP_tally[pvm_OP] = entry
	}

	x86Entry, ok := entry.X86_Map[x86_OP]
	if !ok {
		x86Entry = &X86InternalTally{
			X86_OP: x86_OP,
		}
		entry.X86_Map[x86_OP] = x86Entry
	}
	x86Entry.Count++
	entry.TotalX86++
}

func (vm *VM) TallyJSON(filePath string) error {
	// 1) Create or truncate the output file
	vm.CalculateTally()
	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("create file %q: %w", filePath, err)
	}
	defer f.Close()

	// 2) Serialize directly
	var data []byte
	if vm.OP_tally == nil {
		data = []byte("{}")
	} else {
		vm.AverageTally() // Calculate averages before marshaling
		data, err = json.MarshalIndent(vm.OP_tally, "", "  ")
		//data, err = json.Marshal(rvm.OP_tally)
		if err != nil {
			return fmt.Errorf("marshal tally to JSON: %w", err)
		}
	}

	// 3) Write out
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("write to file %q: %w", filePath, err)
	}
	vm.ResetTally() // Reset tally after writing
	return nil
}
