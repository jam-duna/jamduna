package pvm

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"syscall"
	"unsafe"
)

type CompilerRam struct {
	realMemory           []byte
	realMemAddr          uintptr
	regDumpMem           []byte
	regDumpAddr          uintptr
	dirtyPages           map[int]bool
	current_heap_pointer uint32
}

func NewCompilerRam() (*CompilerRam, error) {
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

	if debugCompiler {
		// print with size
		fmt.Printf("Register dump memory address: 0x%X (size: %d bytes)\n", regDumpAddr, len(mem[:1024*1024]))

		realMemAddruint := uint64(uintptr(unsafe.Pointer(&mem[1024*1024])))
		fmt.Printf("Real memory address: 0x%X (size: %d bytes)\n", realMemAddruint, len(mem))
		fmt.Printf("CompilerVM initialized with memory size: %d bytes\n", len(mem))
	}
	realMem := mem[1024*1024:]
	regMem := mem[:1024*1024]
	return &CompilerRam{
		realMemory:  realMem,
		realMemAddr: uintptr(unsafe.Pointer(&realMem[0])),
		regDumpMem:  regMem,
		regDumpAddr: uintptr(unsafe.Pointer(&regMem[0])),
		dirtyPages:  make(map[int]bool),
	}, nil
}
func (rvm *CompilerRam) Close() error {
	// The underlying mmap'd memory is the union of regDumpMem and realMemory.
	// regDumpMem is the first 1MB, realMemory is the rest.
	// Both slices share the same backing array, so unmap once using the full region.
	if rvm.regDumpMem == nil && rvm.realMemory == nil {
		return nil
	}
	// Find the full mmap'd region by combining regDumpMem and realMemory
	var fullRegion []byte
	if cap(rvm.regDumpMem) > cap(rvm.realMemory) {
		fullRegion = rvm.regDumpMem[:cap(rvm.regDumpMem)]
	} else {
		fullRegion = rvm.realMemory[:cap(rvm.realMemory)]
	}
	err := syscall.Munmap(fullRegion)
	// Clear references
	rvm.realMemory = nil
	rvm.regDumpMem = nil
	rvm.dirtyPages = nil
	return err
}

// GetMemAssess checks access rights for a memory range using mprotect probe.
// DANGER: This function uses unsafe operations and can cause SIGSEGV if the address is invalid or inaccessible.
func (rvm *CompilerRam) GetMemAssess(address uint32, length uint32) (byte, error) {
	pageIndex := int(address / PageSize)
	if pageIndex < 0 || pageIndex >= TotalPages {
		return PageInaccessible, fmt.Errorf("invalid address")
	}
	start := pageIndex * PageSize

	// Attempt to read a byte to test access
	defer func() {
		_ = recover() // Catch panic from SIGSEGV
	}()
	basePtr := rvm.realMemAddr
	memSlice := unsafe.Slice((*byte)(unsafe.Pointer(basePtr+uintptr(start))), PageSize)
	// Read a byte to check if the page is accessible
	_ = memSlice[0]         // This will panic if the page is not accessible
	return PageMutable, nil // if no panic, assume readable
}

// ReadMemory reads data from a specific address in the memory if it's readable.
func (rvm *CompilerRam) ReadMemory(address uint32, length uint32) (data []byte, err error) {
	// recover panic from SIGSEGV
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("ReadMemory panic: %v\n", r)
			data = nil
			err = fmt.Errorf("memory access violation at address %x: %v", address, r)
			return
		}
	}()
	pageIndex := int(address / PageSize)
	if pageIndex < 0 || pageIndex >= TotalPages {
		return nil, fmt.Errorf("invalid address %x for page index %d", address, pageIndex)
	}

	access, err := rvm.GetMemAssess(address, length)
	if err != nil {
		return nil, fmt.Errorf("failed to get memory access: %w", err)
	}
	if access == PageInaccessible {
		return nil, fmt.Errorf("memory at address %x is not readable", address)
	}

	start := pageIndex * PageSize
	offset := int(address % PageSize)
	end := offset + int(length)

	if end > PageSize {
		// return nil, fmt.Errorf("read exceeds page size at address %x", address)
	}

	data = make([]byte, length)
	copy(data, rvm.realMemory[start+offset:start+end])
	return data, nil
}

// WriteMemory writes data to a specific address in the memory if it's writable.
func (rvm *CompilerRam) WriteMemory(address uint32, data []byte) error {
	pageIndex := int(address / PageSize)
	if pageIndex < 0 || pageIndex >= TotalPages {
		return fmt.Errorf("invalid address %x for page index %d", address, pageIndex)
	}

	access, err := rvm.GetMemAssess(address, uint32(len(data)))
	if err != nil {
		return fmt.Errorf("failed to get memory access: %w", err)
	}
	if access != PageMutable {
		return fmt.Errorf("memory at address %x is not writable", address)
	}

	start := pageIndex * PageSize
	offset := int(address % PageSize)
	copy(rvm.realMemory[start+offset:start+offset+len(data)], data)
	return nil
}

// SetPageAccess sets the memory protection of a single page using BaseReg as memory base.
func (rvm *CompilerRam) SetPageAccess(pageIndex int, access byte) error {
	if pageIndex < 0 || pageIndex >= TotalPages {
		return fmt.Errorf("invalid page index")
	}
	if access != PageInaccessible {
		rvm.dirtyPages[pageIndex] = true // mark page as dirty
	} else {
		delete(rvm.dirtyPages, pageIndex) // mark page as clean
	}

	start := uintptr(pageIndex * PageSize)
	basePtr := rvm.realMemAddr
	memSlice := unsafe.Slice((*byte)(unsafe.Pointer(basePtr+start)), PageSize)

	var prot int
	switch access {
	case PageInaccessible:
		prot = syscall.PROT_NONE
		// fmt.Printf("setting access of @0x%x to inaccessible\n", start)
	case PageMutable:
		prot = syscall.PROT_READ | syscall.PROT_WRITE
		// fmt.Printf("setting access of @0x%x to mutable\n", start)
	case PageImmutable:
		prot = syscall.PROT_READ
	default:
		return fmt.Errorf("unknown access mode")
	}

	if err := syscall.Mprotect(memSlice, prot); err != nil {
		return fmt.Errorf("mprotect failed: %v", err)
	}
	return nil
}
func (rvm *CompilerRam) SetMemAccess(address uint32, length uint32, access byte) error {
	if length == 0 {
		return nil
	}

	// Check for overflow in address + length - 1
	if address > ^uint32(0)-(length-1) {
		return fmt.Errorf("invalid address/length: address=0x%x, length=0x%x", address, length)
	}

	// Calculate start and end page indices
	startPage := int(address / PageSize)
	endAddress := address + length - 1
	endPage := int(endAddress / PageSize)

	// Validate page indices
	if startPage < 0 || startPage >= TotalPages || endPage < 0 || endPage >= TotalPages {
		return fmt.Errorf(
			"invalid address range: 0x%x(len=0x%x) spans pages %d~%d, total pages=%d",
			address, length, startPage, endPage, TotalPages,
		)
	}

	// Set access for each page in the range
	for page := startPage; page <= endPage; page++ {
		// fmt.Printf("Setting access of page %d to %d\n", page, access)
		if err := rvm.SetPageAccess(page, access); err != nil {
			return fmt.Errorf("failed to set access on page %d: %w", page, err)
		}
	}
	return nil
}

func (rvm *CompilerRam) WriteRAMBytes(address uint32, data []byte) (resultCode uint64) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("WriteRamBytes panic: %v\n", r)
			resultCode = OOB // Out of bounds
			return
		}
	}()
	if len(data) == 0 {
		return OK
	}
	err := rvm.WriteMemory(address, data)
	if err != nil {
		fmt.Printf("WriteRamBytes error: %v\n", err)
		return OOB
	}
	return OK
}
func (rvm *CompilerRam) ReadRAMBytes(address uint32, length uint32) (data []byte, resultCode uint64) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("ReadRamBytes panic: %v\n", r)
			data = nil
			resultCode = OOB // Out of bounds
		}
	}()
	if length == 0 {
		return []byte{}, OK
	}
	data, err := rvm.ReadMemory(address, length)
	if err != nil {
		fmt.Printf("ReadRamBytes error: %v\n", err)
		return nil, OOB
	}
	return data, OK
}

func (rvm *CompilerRam) allocatePages(startPage uint32, count uint32) {
	for i := uint32(0); i < count; i++ {
		pageIndex := startPage + i
		rvm.SetPageAccess(int(pageIndex), PageMutable)
	}
}

// GetCurrentHeapPointer
func (rvm *CompilerRam) GetCurrentHeapPointer() uint32 {
	return rvm.current_heap_pointer
}

func (rvm *CompilerRam) SetCurrentHeapPointer(pointer uint32) {
	rvm.current_heap_pointer = pointer
}

// 	GetCurrentHeapPointer() uint32
// 	SetCurrentHeapPointer(pointer uint32)
// 	ReadRegister(index int) (uint64, uint64)
// 	WriteRegister(index int, value uint64) uint64
// 	ReadRegisters() []uint64

func (rvm *CompilerRam) ReadRegister(index int) (uint64, uint64) {
	if index < 0 || index >= regSize {
		return 0, OOB // Out of bounds
	}
	value_bytes := rvm.regDumpMem[index*8 : (index+1)*8]
	value := binary.LittleEndian.Uint64(value_bytes)
	return value, OK
}

func (rvm *CompilerRam) WriteRegister(index int, value uint64) uint64 {
	if index < 0 || index >= regSize {
		return OOB // Out of bounds
	}
	value_bytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(value_bytes, value)
	start := index * 8
	if start+8 > len(rvm.regDumpMem) {
		return OOB // Out of bounds
	}
	copy(rvm.regDumpMem[start:start+8], value_bytes)
	return OK // Success
}

// ReadRegisters returns a copy of the current register values.
func (rvm *CompilerRam) ReadRegisters() []uint64 {
	registersCopy := make([]uint64, regSize)
	for i := 0; i < regSize; i++ {
		value_bytes := rvm.regDumpMem[i*8 : (i+1)*8]
		registersCopy[i] = binary.LittleEndian.Uint64(value_bytes)
	}
	return registersCopy
}

// A.5.3. Instructions with Arguments of One Register and One Extended Width Immediate.
func generateLoadImm64(inst Instruction) []byte {
	dst := min(12, int(inst.Args[0]))
	imm := binary.LittleEndian.Uint64(inst.Args[1:9])

	// Use helper function instead of manual construction
	return emitMovImmToReg64(regInfoList[dst], imm)
}

func generateStoreImmGeneric(
	opcode byte,
	prefix byte,
	immBuilder func(uint64) []byte,
) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		disp, immVal := extractTwoImm(inst.Args)

		// Use helper function for store immediate with base SIB addressing
		immBytes := immBuilder(immVal)
		return emitStoreImmWithBaseSIB(BaseReg, uint32(disp), immBytes, opcode, prefix)
	}
}

// U64 (x86-64 does not support direct MOV [mem]-> imm64, so we split into two 32-bit writes)
// Complete generateStoreImmU64
func generateStoreImmU64(inst Instruction) []byte {
	disp64, immVal := extractTwoImm(inst.Args)
	disp := uint32(disp64)

	// Split into lower/higher 32 bits
	low32, high32 := splitU64(immVal)

	// Allocate a slice with enough capacity
	buf := make([]byte, 0, (1+1+1+1+4+4)*2)

	// Write lower 32 bits
	buf = emitStoreImm32(buf, disp, low32)
	// Write higher 32 bits, disp+4
	buf = emitStoreImm32(buf, disp+4, high32)

	return buf
}

// A.5.6. Instructions with Arguments of One Register & Two Immediates.
// Emits: REX.W + MOV r64, imm64
// where imm64 is the sign-extended int32(imm) into uint64
func generateLoadImm32(inst Instruction) []byte {
	dstReg, imm := extractOneRegOneImm(inst.Args)
	dst := regInfoList[dstReg]

	// Use helper function for sign-extended 32-bit immediate to 64-bit register
	return emitMovSignExtImm32ToReg64(dst, uint32(imm))
}

// Generic generator: load from [BaseReg+disp32] into dst, with zero-extend/sign-extend/direct load
//
//   - opcodes: actual opcode bytes, e.g. {0x0F,0xB6} (MOVZX r32, r/m8)
//     or {0x8B}          (MOV r32, r/m32)
//     or {0x63}          (MOVSXD r64, r/m32)
//   - rexW:   whether to set REX.W=1 (64-bit operand)
//
// result = [ REX ][ opcodes... ][ ModRM ][ SIB ][ disp32 ]
func generateLoadWithBase(opcodes []byte, rexW bool) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		dstReg, disp := extractOneRegOneImm(inst.Args)
		dst := regInfoList[dstReg]
		base := BaseReg

		// Use helper function for SIB-based load
		return emitLoadWithBaseSIB(dst, base, uint32(disp), opcodes, rexW)
	}
}

func generateStoreWithBase(opcode byte, prefix byte, rexW bool) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		srcIdx, disp := extractOneRegOneImm(inst.Args)
		src := regInfoList[srcIdx]
		base := BaseReg

		// Use helper function for SIB-based store
		return emitStoreWithBaseSIB(src, base, uint32(disp), opcode, prefix, rexW)
	}
}

func generateStoreImmIndU8(inst Instruction) []byte {
	// 1) Extract register index, displacement, and 8-bit immediate value
	regAIndex, disp64, immVal := extractOneReg2Imm(inst.Args)
	idx := regInfoList[regAIndex]
	base := BaseReg
	disp := uint32(disp64)

	// 2) Use helper function for SIB-based store with 8-bit immediate
	immValU8 := uint8(immVal & X86_MASK_8BIT)
	return emitStoreImmIndWithSIB(idx, base, disp, []byte{immValU8}, X86_OP_MOV_RM_IMM8, X86_NO_PREFIX)
}

// generateStoreImmIndU16 generates machine code for MOV word ptr [Base+index*1+disp], imm16
func generateStoreImmIndU16(inst Instruction) []byte {
	// 1) extract register index, displacement, and 16-bit immediate value
	regAIndex, disp64, immVal := extractOneReg2Imm(inst.Args)
	idx := regInfoList[regAIndex]
	base := BaseReg
	disp := uint32(disp64)
	//fmt.Printf("generateStoreImmIndU16: regAIndex=%d, disp64=%d, immVal=%d\n", regAIndex, disp64, immVal)

	// 2) Use helper function for SIB-based store with 16-bit immediate and 0x66 prefix
	imm16 := uint16(immVal & X86_MASK_16BIT)
	immBytes := []byte{byte(imm16), byte(imm16 >> 8)}
	return emitStoreImmIndWithSIB(idx, base, disp, immBytes, X86_OP_MOV_RM_IMM, X86_PREFIX_66)
}

// generateStoreImmIndU32 generates machine code for MOV dword ptr [Base+index*1+disp], imm32
func generateStoreImmIndU32(inst Instruction) []byte {
	// 1) extract register index, displacement, and 32-bit immediate value
	regAIndex, disp64, immVal := extractOneReg2Imm(inst.Args)
	idx := regInfoList[regAIndex]
	base := BaseReg
	disp := uint32(disp64)

	// 2) Use helper function for SIB-based store with 32-bit immediate
	imm32 := uint32(immVal)
	immBytes := []byte{
		byte(imm32), byte(imm32 >> 8),
		byte(imm32 >> 16), byte(imm32 >> 24),
	}
	return emitStoreImmIndWithSIB(idx, base, disp, immBytes, X86_OP_MOV_RM_IMM, X86_NO_PREFIX)
}

// generateStoreImmIndU64 generates machine code for MOV qword ptr [Base+index*1+disp], imm64
func generateStoreImmIndU64(inst Instruction) []byte {
	// 1) extract register index, displacement, and 64-bit immediate value
	regAIndex, disp64, immVal := extractOneReg2Imm(inst.Args)
	idx := regInfoList[regAIndex]
	base := BaseReg
	disp := uint32(disp64)
	//fmt.Printf("generateStoreImmIndU64: regAIndex=%d, disp64=%d, immVal=%d\n", regAIndex, disp64, immVal)

	// 2) split the 64-bit immediate into two 32-bit parts
	low := uint32(immVal & X86_MASK_32BIT)
	high := uint32(immVal >> 32)

	// 3) Use helper functions for both parts
	buf := bytes.NewBuffer(nil)
	buf.Write(emitStoreImmIndU32WithRegs(idx, base, disp, low))
	buf.Write(emitStoreImmIndU32WithRegs(idx, base, disp+4, high))

	return buf.Bytes()
}
