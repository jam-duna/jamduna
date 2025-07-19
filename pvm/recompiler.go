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

		isChargingGas:   true,         // default to charging gas
		isPCCounting:    useEcalli500, // default to counting PC
		IsBlockCounting: useEcalli500, // default to not counting basic blocks
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
		if showDisassembly {
			fmt.Printf("Initialize Register %d (%s) = %d\n", i, regInfoList[i].Name, immVal)
		}
		vm.startCode = append(vm.startCode, code...)
	}
	gasRegMemAddr := uint64(vm.regDumpAddr) + uint64(len(regInfoList)*8)
	if showDisassembly {
		fmt.Printf("Initialize Gas %d = %d\n", gasRegMemAddr, vm.Gas)
	}
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
	// ==== Pre-checks: CMP/JE placeholders ====

	// (a) jumpIndTempReg == 0xFFFF0000 → ret_stub
	// REX prefix: 0x40 | B
	rex := byte(0x40)
	if jumpIndTempReg.REXBit == 1 {
		rex |= 0x01 // REX.B
	}
	modrm := byte(0xC0 | (7 << 3) | jumpIndTempReg.RegBits)
	code = append(code,
		rex,
		0x81, // CMP r/m32, imm32
		modrm,
		0x00, 0x00, 0xFF, 0xFF, // 0xFFFF0000
	)
	offJEret := len(code)
	code = append(code, 0x0F, 0x84, 0, 0, 0, 0)

	// (b) jumpIndTempReg == 0 → panic_stub
	rex = 0x48
	if jumpIndTempReg.REXBit == 1 {
		rex |= 0x01
	}
	modrm = byte(0xF8 | jumpIndTempReg.RegBits)
	code = append(code, rex, 0x83, modrm, 0x00)
	offJE0 := len(code)
	code = append(code, 0x0F, 0x84, 0, 0, 0, 0)

	// (c) jumpIndTempReg > threshold → panic_stub
	threshold := uint32(len(vm.JumpTableMap)) * uint32(Z_A)
	var thr [4]byte
	binary.LittleEndian.PutUint32(thr[:], threshold)
	rex = 0x48
	if jumpIndTempReg.REXBit == 1 {
		rex |= 0x01
	}
	modrm = byte(0xF8 | jumpIndTempReg.RegBits)
	code = append(code, rex, 0x81, modrm)
	code = append(code, thr[:]...)
	offJA := len(code)
	code = append(code, 0x0F, 0x87, 0, 0, 0, 0)

	// (d) jumpIndTempReg % 2 != 0 → panic_stub
	rex = 0x48
	if jumpIndTempReg.REXBit == 1 {
		rex |= 0x01
	}
	modrm = byte(0xC0 | jumpIndTempReg.RegBits)
	code = append(code, rex, 0xF7, modrm, 0x01, 0, 0, 0)
	offJNZ := len(code)
	code = append(code, 0x0F, 0x85, 0, 0, 0, 0)

	// Collect panic placeholders
	panicOffs := []int{offJE0 + 2, offJA + 2, offJNZ + 2}

	// ==== Continue execution after checks ====
	// ==== Continue execution after checks ====
	// 计算 (jumpIndTempReg / 2 - 1) * 7，然后把当前 PC 加载到 RAX

	//  1. SAR jumpIndTempReg, 1    ; 带符号右移 1 位，相当于 signed_div R12,2
	//     opcode: REX.W + C1 /7 ib
	rex = byte(0x48) // REX.W
	if jumpIndTempReg.REXBit == 1 {
		rex |= 0x01 // REX.B 扩展到 R12..R15
	}
	// /7 → reg=7，mod=11（0xC0），rm=jumpIndTempReg.RegBits
	modrm = byte(0xC0 | (7 << 3) | jumpIndTempReg.RegBits)
	code = append(code,
		rex,   // REX 前缀
		0xC1,  // opcode: shift r/m64 by imm8
		modrm, // ModR/M (reg=7→SAR)
		0x01,  // imm8 = 1
	)

	// 假設 jumpIndTempReg 是 r12 .RegBits=5, jumpIndTempReg.REXBit=0

	// 2. SUB RCX, 1
	rex = byte(0x48)                // REX.W
	if jumpIndTempReg.REXBit == 1 { // REX.B 扩展到 R12..R15
		rex |= 0x01
	}
	modr := byte(0xC0 | (5 << 3) | jumpIndTempReg.RegBits) // 0xC0 | 0x28 | 0x04 == 0xEC
	code = append(code,
		rex,  // 0x49
		0x83, // opcode: ADD/SUB r/m64, imm8
		modr, // 0xEC
		0x01, // imm8 = 1
	)

	// REX.W + REX.R + REX.B 都要設
	rex = byte(0x48) // W=1
	if jumpIndTempReg.REXBit == 1 {
		rex |= 0x05 // REX.R=1, REX.B=1
	}
	// mod=11, reg=jumpIndTempReg.RegBits, rm=jumpIndTempReg.RegBits
	modrm = byte(0xC0 | (jumpIndTempReg.RegBits << 3) | jumpIndTempReg.RegBits)
	// imm32 = 7
	imm := []byte{0x07, 0x00, 0x00, 0x00}
	code = append(code, rex, 0x69, modrm)
	code = append(code, imm...)

	// // PUSH R12
	// code = append(code,
	// 	0x41,                 // REX.B
	// 	0x50|jumpIndTempReg.RegBits, // 0x50|4 = 0x54
	// )

	// push rax
	code = append(code, 0x50) // PUSH RAX
	//  4. LEA RAX, [RIP+0]     ; 获取当前指令地址到 RAX
	//     opcode: 48 8D 05 00 00 00 00
	code = append(code, 0x48, 0x8D, 0x05, 0x00, 0x00, 0x00, 0x00)

	rex = byte(0x48)
	if jumpIndTempReg.REXBit == 1 {
		rex |= 0x01 // REX.B
	}
	modrm = byte(0xC0 | (0 << 3) | jumpIndTempReg.RegBits)
	code = append(code, rex, 0x01, modrm)

	code = append(code, 0x58) // POP RAX

	handlerAddOff := len(code) + 3
	code = append(code, rex, 0x81, modrm, 0xEF, 0xBE, 0xAD, 0xDE) // add jumpIndTempReg, imm32

	// jmp jumpIndTempReg
	rex = 0x48
	if jumpIndTempReg.REXBit == 1 {
		rex |= 0x01
	}
	modrm = 0xE0 | jumpIndTempReg.RegBits
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
	code = append(code, rex, byte(0x58|jumpIndTempReg.RegBits), 0x0F, 0x0B) // POP jumpIndTempReg; UD2

	// Patch ret stub JE
	retStubAddr := vm.codeAddr + uintptr(len(code))
	relRet := int32(int64(retStubAddr) - int64(vm.codeAddr) - int64(offJEret) - 6)
	binary.LittleEndian.PutUint32(code[offJEret+2:offJEret+6], uint32(relRet))
	// finish ret stub
	code = emitPopReg(code, jumpIndTempReg) // POP jumpIndTempReg
	code = append(code, vm.exitCode...)

	// Patch handler jumps
	for i, p := range pendings {
		// POP jumpIndTempReg then JMP handler
		pendings[i].jeOff = len(code)
		code = append(code, rex, byte(0x58|jumpIndTempReg.RegBits))
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
	toAdd := firstPending.jeOff - handlerAddOff + 7
	// Patch into the Add instruction
	// fmt.Printf("To add to BaseReg: %d\n", toAdd)
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
	crashed, msec, err := ExecuteX86(codeAddr, vm.regDumpMem)
	for i := 0; i < regSize; i++ {
		regValue := binary.LittleEndian.Uint64(vm.regDumpMem[i*8:])
		if showDisassembly {
			fmt.Printf("%s = %d\n", regInfoList[i].Name, regValue)
		}
		vm.Ram.WriteRegister(i, regValue)
	}
	vm.SetIdentifier(fmt.Sprintf("%d", msec))
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
		if showDisassembly {
			fmt.Printf("%s = %d\n", regInfoList[i].Name, regValue)
		}
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
	str := vm.Disassemble(vm.realCode)
	if showDisassembly {
		fmt.Printf("ALL COMBINED Disassembled x86 code:\n%s\n", str)
	}
	crashed, msec, err := ExecuteX86(codeAddr, vm.regDumpMem)
	vm.SetIdentifier(fmt.Sprintf("%d", msec))
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
		if showDisassembly {
			fmt.Printf("%s = %d\n", regInfoList[i].Name, regValue)
		}
		vm.Ram.WriteRegister(i, regValue)
	}
	return nil
}

func (rvm *RecompilerVM) Execute(entry uint32) {
	rvm.pc = 0
	rvm.initStartCode()
	rvm.Compile(rvm.pc)
	if err := rvm.ExecuteX86CodeWithEntry(rvm.x86Code, entry); err != nil {
		// we don't have to return this , just print it
		fmt.Printf("ExecuteX86 crash detected: %v\n", err)
	}
	if UseTally {
		jsonFile := fmt.Sprintf("test/%s.json", rvm.GetIdentifier())

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

func (vm *RecompilerVM) WriteContextSlot(slot_index int, value uint64, size int) error {
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

func (vm *RecompilerVM) ReadContextSlot(slot_index int) (uint64, error) {
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

var jumpIndTempReg = regInfoList[1] //rcx
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
	// 1) 如果是 r8–r15，就先推 REX.B
	if jumpIndTempReg.REXBit == 1 {
		buf = append(buf, 0x41)
	}
	// 2) 再推 opcode 0x50 + regBits
	buf = append(buf, 0x50|jumpIndTempReg.RegBits)
	// mov baseReg, [base]
	rex := byte(0x48) // REX.W
	if jumpIndTempReg.REXBit == 1 {
		rex |= 0x04
	} // REX.R
	if base.REXBit == 1 {
		rex |= 0x01
	} // REX.B
	buf = append(buf, rex, 0x8B, byte(0xC0|(jumpIndTempReg.RegBits<<3)|base.RegBits))

	// add r11, vx
	rex = 0x48
	if jumpIndTempReg.REXBit == 1 {
		rex |= 0x01
	} // REX.B
	buf = append(buf, rex, 0x81, byte(0xC0|(0<<3)|jumpIndTempReg.RegBits))
	imm32 := make([]byte, 4)
	binary.LittleEndian.PutUint32(imm32, uint32(vx))
	buf = append(buf, imm32...)

	// jmp [BaseReg + indirectJumpPointSlot*8 - dumpSize]
	buf = append(buf, generateJumpRegMem(BaseReg, indirectJumpPointSlot*8-dumpSize)...)

	return buf
}

// LOAD_IMM_JUMP_IND with jumpIndTempReg
func generateLoadImmJumpIndirect(inst Instruction) []byte {

	dstIdx, indexRegIdx, vx, vy := extractTwoRegsAndTwoImmediates(inst.Args)
	r := regInfoList[dstIdx]
	indexReg := regInfoList[indexRegIdx]

	// 1) PUSH jumpIndTempReg
	rex := byte(0x48)
	if jumpIndTempReg.REXBit == 1 {
		rex |= 0x01
	}
	pushOp := byte(0x50 | jumpIndTempReg.RegBits)
	buf := []byte{rex, pushOp}
	// 2) MOV jumpIndTempReg, indexReg
	rex = byte(0x48) // REX.W
	if jumpIndTempReg.REXBit == 1 {
		rex |= 0x04 // REX.R: extend the reg field for jumpIndTempReg
	}
	if indexReg.REXBit == 1 {
		rex |= 0x01 // REX.B: extend the rm field for indexReg
	}
	// mod=11 (register-direct), reg=jumpIndTempReg, rm=indexReg
	modrm := byte(0xC0 | (jumpIndTempReg.RegBits << 3) | indexReg.RegBits)

	buf = append(buf,
		rex,   // now 0x4D for W=1, R=1, B=1 (r12→r12)
		0x8B,  // MOV r64, r/m64
		modrm, // encodes dest=jumpIndTempReg, src=indexReg
	)

	// 3) MOV r64 (dst) <- imm64 (vx)
	rex = 0x48 // REX.W
	// set REX.B if dst in r8–r15
	if r.REXBit == 1 {
		rex |= 0x01
	}
	movOp := byte(0xB8 + r.RegBits)
	imm64 := make([]byte, 8)
	binary.LittleEndian.PutUint64(imm64, vx)
	buf = append(buf, rex, movOp)
	buf = append(buf, imm64...)

	// 4) ADD jumpIndTempReg, vy
	rex = 0x48
	if jumpIndTempReg.REXBit == 1 {
		rex |= 0x01
	}
	// ADD r/m64, imm32 (reg=0)
	modrm = byte(0xC0 | (0 << 3) | jumpIndTempReg.RegBits)
	imm32 := make([]byte, 4)
	binary.LittleEndian.PutUint32(imm32, uint32(vy))
	buf = append(buf, rex, 0x81, modrm)
	buf = append(buf, imm32...)

	// 5) AND jumpIndTempReg, 0xFFFFFFFF (zero-extend)
	rex = 0x48
	if jumpIndTempReg.REXBit == 1 {
		rex |= 0x01
	}
	// AND r/m64, imm32 (reg=4)
	modrm = byte(0xC0 | (4 << 3) | jumpIndTempReg.RegBits)
	buf = append(buf, rex, 0x81, modrm)
	buf = append(buf, []byte{0xFF, 0xFF, 0xFF, 0xFF}...)

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
		}
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

		// REX.W + REX.R + REX.B
		rex := byte(0x48)
		if rB.REXBit == 1 {
			rex |= 0x04 // REX.R for rB
		}
		if rA.REXBit == 1 {
			rex |= 0x01 // REX.B for rA
		}

		// CMP r/m64, r64 (0x39 /r)
		modrm := byte(0xC0 | (rB.RegBits << 3) | rA.RegBits)

		// This robust sequence uses a conditional jump to the true case
		// and an unconditional jump to the false case. It is 14 bytes long.
		code := make([]byte, 0, 14)
		code = append(code,
			rex, 0x39, modrm, // 00–02: CMP rA, rB
			0x0F, jcc, // 03–04: Jcc rel32 (jump to the "true" branch)
			0, 0, 0, 0, // 05–08: Placeholder for relTrue dd
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
