package pvm

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
	"syscall"
	"time"
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
	const memSize = 4*1024*1024*1024 + 1024*1024 // 4GB + 1MB
	mem, err := syscall.Mmap(
		-1, 0, memSize,
		syscall.PROT_NONE,
		syscall.MAP_ANON|syscall.MAP_PRIVATE,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to mmap memory: %v", err)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to mmap regDump memory: %v", err)
	}
	regDumpAddr := uintptr(unsafe.Pointer(&mem[0]))
	// mem protect the first 1MB as R/W
	if err := syscall.Mprotect(mem[:1024*1024], syscall.PROT_READ|syscall.PROT_WRITE); err != nil {
		return nil, fmt.Errorf("failed to mprotect memory: %v", err)
	}

	if debugRecompiler {
		// print with size
		fmt.Printf("Register dump memory address: 0x%X (size: %d bytes)\n", regDumpAddr, len(mem[:1024*1024]))

		realMemAddruint := uint64(uintptr(unsafe.Pointer(&mem[1024*1024])))
		fmt.Printf("Real memory address: 0x%X (size: %d bytes)\n", realMemAddruint, len(mem))
		fmt.Printf("RecompilerVM initialized with memory size: %d bytes\n", len(mem))
	}

	// Assemble the VM
	rvm := &RecompilerVM{
		VM:          vm,
		realMemory:  mem[1024*1024:], // 1MB reserved for register dump
		realMemAddr: uintptr(unsafe.Pointer(&mem[1024*1024])),
		regDumpMem:  mem[:1024*1024],
		regDumpAddr: regDumpAddr,

		JumpTableMap:    make([]uint64, 0),
		InstMapX86ToPVM: make(map[int]uint32),
		InstMapPVMToX86: make(map[uint32]int),

		pc_addr:    uint64(regDumpAddr + uintptr((len(regInfoList)+1)*8)),
		dirtyPages: make(map[int]bool),

		isChargingGas:   true, // default to charging gas
		isPCCounting:    true, // default to counting PC
		IsBlockCounting: true, // default to not counting basic blocks
	}

	rvm.current_heap_pointer = rvm.VM.Ram.GetCurrentHeapPointer()
	rvm.VM.Ram = rvm

	return rvm, nil
}

// add jump indirects
const entryPatch = 0x99999999

func (vm *RecompilerVM) initStartCode() {

	vm.startCode = append(vm.startCode, encodeMovImm(BaseRegIndex, uint64(vm.realMemAddr))...)
	// initialize registers: mov rX, imm from vm.register
	for i := 0; i < regSize; i++ {
		immVal, _ := vm.Ram.ReadRegister(i)
		code := encodeMovImm(i, immVal)
		fmt.Printf("Initialize Register %d (%s) = %d\n", i, regInfoList[i].Name, immVal)
		vm.startCode = append(vm.startCode, code...)
	}
	gasRegMemAddr := uint64(vm.regDumpAddr) + uint64(len(regInfoList)*8)
	fmt.Printf("Initialize Gas %d = %d\n", gasRegMemAddr, vm.Gas)
	vm.startCode = append(vm.startCode, encodeMovImm64ToMem(gasRegMemAddr, uint64(vm.Gas))...)

	// padding with jump to the entry point
	vm.startCode = append(vm.startCode, 0xE9) // JMP rel32
	// use entryPatch as a placeholder 0x99999999
	patch := make([]byte, 4)
	binary.LittleEndian.PutUint32(patch, entryPatch)
	vm.startCode = append(vm.startCode, patch...)

	// Build exit code in temporary buffer
	exitCode := encodeMovImm(BaseRegIndex, uint64(vm.regDumpAddr))
	for i := 0; i < len(regInfoList); i++ {
		if i == BaseRegIndex {
			continue // skip R12 into [R12]
		}
		offset := byte(i * 8)
		exitCode = append(exitCode, encodeMovRegToMem(i, BaseRegIndex, offset)...)
	}
	vm.exitCode = append(exitCode, 0xC3)
}
func (vm *RecompilerVM) GetDirtyPages() []int {
	dirtyPages := make([]int, 0)
	for pageIndex, _ := range vm.dirtyPages {
		if vm.dirtyPages[pageIndex] {
			dirtyPages = append(dirtyPages, pageIndex)
		}
	}
	return dirtyPages
}

func (vm *RecompilerVM) initDJumpFunc(x86CodeLen int) {
	type pending struct {
		jeOff   int
		handler uintptr
	}

	code := make([]byte, 0)
	var pendings []pending
	rex := byte(0x48) // REX.W
	if BaseReg.REXBit == 1 {
		rex |= 0x01 // +REX.B
	}
	// 1. POP return address into BaseReg
	code = append(code, rex, byte(0x58|BaseReg.RegBits))

	// ==== Pre-checks: CMP/JE placeholders ====

	// (a) BaseReg == 0xFFFF0000 → ret_stub
	code = append(code, 0x57) // PUSH RDI
	// MOV RDI, imm64
	code = append(code, 0x48, 0xBF)
	code = append(code,
		0x00, 0x00, 0xFF, 0xFF, 0x00, 0x00, 0x00, 0x00,
	)
	// CMP BaseReg, RDI
	rex = byte(0x48)
	if BaseReg.REXBit == 1 {
		rex |= 0x01 // REX.B
	}
	modrm := byte(0xC0 | (7 << 3) | BaseReg.RegBits)
	code = append(code, rex, 0x39, modrm)
	offJEret := len(code)
	code = append(code, 0x0F, 0x84, 0, 0, 0, 0) // JE placeholder
	code = append(code, 0x5F)                   // POP RDI

	// (b) BaseReg == 0 → panic_stub
	rex = 0x48
	if BaseReg.REXBit == 1 {
		rex |= 0x01
	}
	modrm = byte(0xF8 | BaseReg.RegBits)
	code = append(code, rex, 0x83, modrm, 0x00)
	offJE0 := len(code)
	code = append(code, 0x0F, 0x84, 0, 0, 0, 0)

	// (c) BaseReg > threshold → panic_stub
	threshold := uint32(len(vm.JumpTableMap)) * uint32(Z_A)
	var thr [4]byte
	binary.LittleEndian.PutUint32(thr[:], threshold)
	rex = 0x48
	if BaseReg.REXBit == 1 {
		rex |= 0x01
	}
	modrm = byte(0xF8 | BaseReg.RegBits)
	code = append(code, rex, 0x81, modrm)
	code = append(code, thr[:]...)
	offJA := len(code)
	code = append(code, 0x0F, 0x87, 0, 0, 0, 0)

	// (d) BaseReg % 2 != 0 → panic_stub
	rex = 0x48
	if BaseReg.REXBit == 1 {
		rex |= 0x01
	}
	modrm = byte(0xC0 | BaseReg.RegBits)
	code = append(code, rex, 0xF7, modrm, 0x01, 0, 0, 0)
	offJNZ := len(code)
	code = append(code, 0x0F, 0x85, 0, 0, 0, 0)

	// Collect panic placeholders
	panicOffs := []int{offJE0 + 2, offJA + 2, offJNZ + 2}

	// ==== Continue execution after checks ====
	za := uint32(Z_A)
	var zaBytes [4]byte
	binary.LittleEndian.PutUint32(zaBytes[:], za)
	// preserve registers
	code = append(code, 0x51, 0x52)                             // PUSH RCX, PUSH RDX
	code = append(code, append([]byte{0xB9}, zaBytes[:]...)...) // MOV ECX, za
	code = append(code, 0x48, 0x99)                             // CQO
	// 1) PUSH RAX
	code = append(code, 0x50) // PUSH RAX

	//  2. MOV RAX, BaseReg
	//     opcode: REX.W + 0x8B /r   (MOV r64, r/m64)
	//     ModR/M: 0b11 | (RAX<<3) | BaseReg
	rex = byte(0x48) // REX.W = 1
	if BaseReg.REXBit == 1 {
		rex |= 0x01 // REX.B
	}
	modrm = byte(0xC0) | (0 << 3) | BaseReg.RegBits
	code = append(code, rex, 0x8B, modrm)

	// 3) CQO
	code = append(code, 0x48, 0x99)

	// 4) IDIV RCX
	code = append(code, 0x48, 0xF7, 0xF9) // IDIV RCX

	//  5. MOV BaseReg, RAX
	//     opcode: REX.W + 0x89 /r   (MOV r/m64, r64)
	//     ModR/M: 0b11 | (RAX<<3) | BaseReg
	rex2 := byte(0x48) // REX.W = 1

	if BaseReg.REXBit == 1 {
		rex2 |= 0x01 // REX.B
	}
	modrm2 := byte(0xC0) | (0 << 3) | BaseReg.RegBits
	code = append(code, rex2, 0x89, modrm2)

	// 6) POP RAX
	code = append(code, 0x58) // POP RAX

	// SUB BaseReg, 1
	rex = 0x48
	if BaseReg.REXBit == 1 {
		rex |= 0x01
	}
	modrm = byte(0xE8 | BaseReg.RegBits)
	code = append(code, rex, 0x83, modrm, 0x01)
	code = append(code, 0x5A, 0x59) // POP RDX, POP RCX
	// 1. First, construct REX
	// For example, for the three-operand IMUL r64, r/m64, imm8:
	// For example, for the three-operand IMUL r64, r/m64, imm8:
	rex = byte(0x48)         // W=1, default 64-bit
	if BaseReg.REXBit == 1 { // If register number ≥ 8
		rex |= 0x4 // REX.R: extend ModRM.reg field
		rex |= 0x1 // REX.B: extend ModRM.r/m field
	}

	// ModRM: mod=11, reg=BaseReg.RegBits, rm=BaseReg.RegBits
	modrm = byte(0xC0 | (BaseReg.RegBits << 3) | BaseReg.RegBits)

	// 6B /r ib → IMUL r64, r/m64, imm8
	code = append(code, rex, 0x6B, modrm, 0x07)

	// add BaseReg, 0xDEADBEEF
	rex = 0x48
	if BaseReg.REXBit == 1 {
		rex |= 0x01
	}
	modrm = byte(0xC0 | BaseReg.RegBits)
	// push rax
	code = append(code, 0x50) // PUSH RAX
	//call 0
	code = append(code, 0xE8, 0, 0, 0, 0) // CALL rel32 placeholder
	// pop rax (get the return address)
	code = append(code, 0x58) // POP RAX
	// add BaseReg, RAX 3
	code = append(code, rex, 0x01, modrm) // ADD BaseReg, RAX
	// pop rax 1
	code = append(code, 0x58) // POP RAX
	handlerAddOff := len(code) + 3
	code = append(code, rex, 0x81, modrm, 0xEF, 0xBE, 0xAD, 0xDE) // add BaseReg, imm32

	// jmp BaseReg
	rex = 0x48
	if BaseReg.REXBit == 1 {
		rex |= 0x01
	}
	modrm = 0xE0 | BaseReg.RegBits
	code = append(code, rex, 0xFF, modrm)
	for _, idx := range vm.JumpTableMap {
		pendings = append(pendings, pending{
			handler: uintptr(idx),
		})
	}
	// jumpOff := len(code)

	// Patch panic jumps
	panicStubAddr := vm.codeAddr + uintptr(len(code))
	for _, off := range panicOffs {
		rel := int32(int64(panicStubAddr) - int64(vm.codeAddr) - int64(off) - 4)
		binary.LittleEndian.PutUint32(code[off:], uint32(rel))
	}
	// panic stub
	code = append(code, rex, byte(0x58|BaseReg.RegBits), 0x0F, 0x0B) // POP BaseReg; UD2

	// Patch ret stub JE
	retStubAddr := vm.codeAddr + uintptr(len(code))
	relRet := int32(int64(retStubAddr) - int64(vm.codeAddr) - int64(offJEret) - 6)
	binary.LittleEndian.PutUint32(code[offJEret+2:offJEret+6], uint32(relRet))
	// finish ret stub
	code = append(code, 0x5F)        // POP RDI
	code = emitPopReg(code, BaseReg) // POP BaseReg
	code = append(code, vm.exitCode...)

	// Patch handler jumps
	for i, p := range pendings {
		// POP BaseReg then JMP handler
		pendings[i].jeOff = len(code)
		code = append(code, rex, byte(0x58|BaseReg.RegBits))
		// 2bytes
		toMinus := x86CodeLen + len(code) - int(p.handler)
		num := int32(-toMinus - 5)
		buf := make([]byte, 4)
		binary.LittleEndian.PutUint32(buf, uint32(num))
		code = append(code, 0xE9)
		// 1 byte
		code = append(code, buf...)
		// 4 bytes
		// total 7 bytes
	}
	if len(pendings) == 0 {
		fmt.Println("No pending handlers found, skipping handler patching.")
		vm.djumpTableFunc = code
		return
	}
	firstPending := pendings[0]
	toAdd := firstPending.jeOff - handlerAddOff + 8
	// Patch into the Add instruction
	fmt.Printf("To add to BaseReg: %d\n", toAdd)
	uint64ToAdd := uint64(toAdd)
	binary.LittleEndian.PutUint32(code[handlerAddOff:], uint32(uint64ToAdd))
	vm.djumpTableFunc = code
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

	if len(errs) > 0 {
		return fmt.Errorf("Close encountered errors: %v", errs)
	}

	return nil
}

func (rvm *RecompilerVM) Disassemble(code []byte) string {
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
	str := vm.Disassemble(vm.realCode)
	if showDisassembly {
		fmt.Printf("ALL COMBINED Disassembled x86 code:\n%s\n", str)
	}

	crashed, err := ExecuteX86(codeAddr, vm.regDumpMem)
	for i := 0; i < regSize; i++ {
		regValue := binary.LittleEndian.Uint64(vm.regDumpMem[i*8:])
		fmt.Printf("%s = %d\n", regInfoList[i].Name, regValue)
		vm.Ram.WriteRegister(i, regValue)
	}
	if crashed == -1 || err != nil {
		vm.ResultCode = types.WORKRESULT_PANIC
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
		fmt.Printf("%s = %d\n", regInfoList[i].Name, regValue)
		vm.Ram.WriteRegister(i, regValue)
	}
	return nil
}

func (vm *RecompilerVM) ExecuteX86CodeWithEntry(x86code []byte, entry uint32) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Error(vm.logging, "RecompilerVM ExecuteX86Code panic", "error", r)
			debug.PrintStack()
			vm.ResultCode = types.WORKRESULT_PANIC
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

	var patchInstIdx = -1
	entryPatchImm := entryPatch
	// use entryPatch as a placeholder 0x99999999
	//get the x86 pc
	x86PC, ok := vm.InstMapPVMToX86[entry]
	if !ok && entry != 0 {
		return fmt.Errorf("entry %d not found in InstMapPVMToX86", entry)
	}
	if debugRecompiler {
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
			fmt.Printf("Patching entry point at index %d with 0x%X\n",
				patchInstIdx, entryPatchImm)
			break
		}
	}
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
	str := vm.Disassemble(vm.realCode)
	if showDisassembly {
		fmt.Printf("ALL COMBINED Disassembled x86 code:\n%s\n", str)
	}

	crashed, err := ExecuteX86(codeAddr, vm.regDumpMem)
	for i := 0; i < regSize; i++ {
		regValue := binary.LittleEndian.Uint64(vm.regDumpMem[i*8:])
		fmt.Printf("%s = %d\n", regInfoList[i].Name, regValue)
		vm.Ram.WriteRegister(i, regValue)
	}
	vm.Gas = int64(binary.LittleEndian.Uint64(vm.regDumpMem[(len(regInfoList))*8:]))
	if crashed == -1 || err != nil {
		vm.ResultCode = types.WORKRESULT_PANIC
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
		fmt.Printf("%s = %d\n", regInfoList[i].Name, regValue)
		vm.Ram.WriteRegister(i, regValue)
	}
	return nil
}

func (rvm *RecompilerVM) Execute(entry uint32) {
	rvm.pc = 0
	rvm.initStartCode()
	rvm.Compile(rvm.pc)
	tm := time.Now()
	if err := rvm.ExecuteX86CodeWithEntry(rvm.x86Code, entry); err != nil {
		// we don't have to return this , just print it
		fmt.Printf("ExecuteX86 crash detected: %v\n", err)
	}
	if UseTally {
		// timestamp suffix
		ts := time.Now().UnixMilli()
		jsonFile := fmt.Sprintf("test/%s_%d.json", VM_MODE, ts)

		// ensure the directory exists (mkdir -p)
		dir := filepath.Dir(jsonFile)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fmt.Printf("failed to create directory %q: %v\n", dir, err)
		}
		// dump JSON tally
		if err := rvm.TallyJSON(jsonFile); err != nil {
			fmt.Printf("failed to write JSON tally: %v\n", err)
		}
	}

	fmt.Printf("EXECUTEX86 Mode: %s Execution time: %s\n", rvm.Mode, time.Since(tm))
}
func (vm *VM) CalculateTally() {
	fmt.Println("Basic Block Execution Tally:")
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

// Standard_Program_Initialization initializes the program memory and registers
func (vm *RecompilerVM) Standard_Program_Initialization(argument_data_a []byte) (err error) {

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

// A.5.2. Instructions with Arguments of One Immediate. InstructionI1

func generateTrap(inst Instruction) []byte {
	// rel := 0xDEADBEEF
	return []byte{
		// 0xE9,                      // opcode
		// byte(rel), byte(rel >> 8), // disp[0..1]
		// byte(rel >> 16), byte(rel >> 24),
		0xCC, // int3 (breakpoint instruction)
	}
}

func generateFallthrough(inst Instruction) []byte {
	return []byte{0x90}
}

func generateJump(inst Instruction) []byte {
	// For direct jumps, we can just append a jump instruction to a X86 PC
	code := make([]byte, 0)
	return append(code, 0xE9, 0xFE, 0xFE, 0xFE, 0xFE)
}

// LOAD_IMM_JUMP (only the “LOAD_IMM” portion)
func generateLoadImmJump(inst Instruction) []byte {
	dstIdx, vx, _ := extractOneRegOneImmOneOffset(inst.Args)
	r := regInfoList[dstIdx]
	// start with REX.W only (0x48), and then OR in B *only if* r.REXBit==1
	rex := byte(0x48)
	if r.REXBit == 1 {
		rex |= 0x01 // set only the B bit for r8–r15
	}
	movOp := byte(0xB8 + r.RegBits)
	immBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(immBytes, vx)
	code := append([]byte{rex, movOp}, immBytes...)
	// code = append(code, generateJumpTrackerCode()...)
	return append(code, 0xE9, 0xFE, 0xFE, 0xFE, 0xFE) // this will be patched
}

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
	tmp := BaseReg

	buf := make([]byte, 0, 32)
	buf = append(buf,
		0x41, 0x54|BaseReg.RegBits, // push BaseReg (r12..r15)
	)
	// mov baseReg, [base]
	rex := byte(0x48) // REX.W
	if tmp.REXBit == 1 {
		rex |= 0x04
	} // REX.R
	if base.REXBit == 1 {
		rex |= 0x01
	} // REX.B
	buf = append(buf, rex, 0x8B, byte(0xC0|(tmp.RegBits<<3)|base.RegBits))

	// add r11, vx
	rex = 0x48
	if tmp.REXBit == 1 {
		rex |= 0x01
	} // REX.B
	buf = append(buf, rex, 0x81, byte(0xC0|(0<<3)|tmp.RegBits))
	imm32 := make([]byte, 4)
	binary.LittleEndian.PutUint32(imm32, uint32(vx))
	buf = append(buf, imm32...)
	// push rax
	buf = append(buf, 0x41, 0x50|tmp.RegBits) // push tmp (push r11)
	// buf = append(buf, generateJumpTrackerCode()...)
	// MOV tmp, base_address  Load base into tmp (r11)
	rex = 0x49 // REX.W | REX.B (for r8–r15)
	if tmp.REXBit == 1 {
		rex |= 0x01
	}
	movOp := byte(0xB8 + tmp.RegBits)
	buf = append(buf, rex, movOp, 0xFE, 0xFE, 0xFE, 0xFE, 0xFE, 0xFE, 0xFE, 0xFE)
	// JMP BaseReg
	// JMP BaseReg (r11)
	rex = byte(0x49)
	if BaseReg.REXBit == 1 {
		rex |= 0x01 // REX.B
	}
	modrm := byte(0xC0 | (4 << 3) | BaseReg.RegBits) // 0xC0 = 11xxxxxx
	buf = append(buf, rex, 0xFF, modrm)
	return buf
}

// LOAD_IMM_JUMP_IND
func generateLoadImmJumpIndirect(inst Instruction) []byte {

	dstIdx, indexRegIdx, vx, vy := extractTwoRegsAndTwoImmediates(inst.Args)
	r := regInfoList[dstIdx]
	// Build REX prefix: REX.W plus REX.B if dst is r8–r15
	indexReg := regInfoList[indexRegIdx]
	rex := byte(0x48)
	if BaseReg.REXBit == 1 {
		rex |= 0x01 // REX.B
	}
	pushOp := byte(0x50 | BaseReg.RegBits)
	MovCode := []byte{rex, pushOp}
	// MOV BaseReg, indexReg
	rex = byte(0x48) // REX.W
	// REX.R for reg field (BaseReg)
	if BaseReg.REXBit == 1 {
		rex |= 0x04
	}
	// REX.B for rm field (indexReg)
	if indexReg.REXBit == 1 {
		rex |= 0x01
	}
	// mod=11, reg=BaseReg, rm=indexReg
	modrm := byte(0xC0 | (BaseReg.RegBits << 3) | indexReg.RegBits)
	MovCode = append(MovCode, rex, 0x8B, modrm)

	rex = byte(0x48) // 01001000b = REX.W=1
	if r.REXBit == 1 {
		rex |= 0x01 // set REX.B
	}

	// Opcode B8+rd  = MOV r64, imm64
	movOp := byte(0xB8 + r.RegBits)

	// Little‐endian encode the 64-bit immediate
	immBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(immBytes, vx)

	// [REX][MOV-opcode][imm64]
	code := append(MovCode, append([]byte{rex, movOp}, immBytes...)...)

	// ADD BaseReg, vy
	// Build REX.W plus REX.B if BaseReg 是 r8–r15
	rex = 0x48
	if BaseReg.REXBit == 1 {
		rex |= 0x01 // REX.B
	}
	// ModR/M byte: mod=11 (reg-direct), reg=0 (ADD), rm=BaseReg.RegBits
	modrm = byte(0xC0 | BaseReg.RegBits)
	code = append(code, rex, 0x81, modrm)

	// 32-bit immediate vy
	imm32 := make([]byte, 4)
	binary.LittleEndian.PutUint32(imm32, uint32(vy))
	code = append(code, imm32...)
	// AND BaseReg, 0xFFFFFFFF  → zero‐extend BaseReg to 32 bit
	rex = 0x48
	if BaseReg.REXBit == 1 {
		rex |= 0x01 // REX.B for high regs
	}
	// 81 /4 r/m64, imm32 → mod=11, reg=4 (AND), rm=BaseReg
	modrm = byte(0xC0 | (4 << 3) | BaseReg.RegBits)
	code = append(code,
		rex,                    // REX.W[B]
		0x81,                   // opcode for imm32 binary ops
		modrm,                  // ModR/M
		0xFF, 0xFF, 0xFF, 0xFF, // imm32 = 0xFFFFFFFF
	)

	// PUSH BaseReg
	rex = 0x48
	if BaseReg.REXBit == 1 {
		rex |= 0x01
	}
	code = append(code,
		rex,
		byte(0x50|BaseReg.RegBits), // PUSH r64
	)

	// MOV BaseReg, placeholder imm64 (0xFE…FE)
	rex = 0x48
	if BaseReg.REXBit == 1 {
		rex |= 0x01 // REX.B for r8–r15
	}
	movOp = byte(0xB8 | BaseReg.RegBits) // B8+rd
	code = append(code,
		rex,
		movOp,
		0xFE, 0xFE, 0xFE, 0xFE, 0xFE, 0xFE, 0xFE, 0xFE,
	)

	// JMP BaseReg (indirect)
	rex = 0x48
	if BaseReg.REXBit == 1 {
		rex |= 0x01
	}
	// FF /4 r/m64 → mod=11, reg=4 (JMP), rm=BaseReg
	modrm = byte(0xC0 | (4 << 3) | BaseReg.RegBits)
	code = append(code,
		rex,
		0xFF,
		modrm,
	)

	return code
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

		// code := append([]byte{}, generateJumpTrackerCode()...)
		code := make([]byte, 0)
		code = append(code,
			rex, 0x39, modrm, // 00–02
			0x0F, jcc, // 03–04
			0, 0, 0, 0, // 05–08 (relTrue placeholder)
			0xE9,       // 09 (JMP opcode)
			0, 0, 0, 0, // 10–13 (relFalse placeholder)
		)
		return code
	}
}
func (vm *RecompilerVM) GetMemory() (map[int][]byte, map[int]int) {
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
func (vm *RecompilerVM) TakeSnapShot(name string, pc uint32, registers []uint64, gas uint64, failAddress uint64, BaseRegValue uint64, basicBlockNumber uint64) *EmulatorSnapShot {
	memory, pagemap := vm.GetMemory()
	snapshot := &EmulatorSnapShot{
		Name:             name,
		InitialRegs:      registers,
		InitialPC:        pc,
		FailAddress:      failAddress,
		InitialPageMap:   pagemap,
		InitialMemory:    memory,
		InitialGas:       gas,
		Code:             make([]byte, 0), // regenerated anyway
		BaseRegValue:     BaseRegValue,
		BasicBlockNumber: basicBlockNumber,
	}
	return snapshot
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

func (vm *RecompilerVM) AddPVMCount(pvm_OP string) {
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

func (vm *RecompilerVM) DisassembleAndTally(pvm_OP_Code byte, code []byte) string {
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
	return nil
}
