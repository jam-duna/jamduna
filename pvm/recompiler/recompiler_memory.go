package recompiler

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

type RecompilerRam struct {
	realMemory           []byte
	realMemAddr          uintptr
	regDumpMem           []byte
	regDumpAddr          uintptr
	memAccess            map[int]int // Changed from array to map for lazy allocation
	current_heap_pointer uint32
}

const memSize = 4*1024*1024*1024 + 1024*1024 // 4GB + 1MB
const regMemsize = 1024 * 1024               // 1MB for register dump

func NewRecompilerRam() (*RecompilerRam, error) {
	// Allocate 4GB virtual memory region (not accessed directly here)
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
		fmt.Printf("Register dump memory address: 0x%X (size: %d bytes)\n", regDumpAddr, len(mem[:regMemsize]))

		realMemAddruint := uint64(uintptr(unsafe.Pointer(&mem[regMemsize])))
		fmt.Printf("Real memory address: 0x%X (size: %d bytes)\n", realMemAddruint, len(mem[regMemsize:]))
		fmt.Printf("RecompilerVM initialized with memory size: %d bytes\n", len(mem))
	}
	realMem := mem[regMemsize:]
	regMem := mem[:regMemsize]
	return &RecompilerRam{
		realMemory:  realMem,
		realMemAddr: uintptr(unsafe.Pointer(&realMem[0])),
		regDumpMem:  regMem,
		regDumpAddr: uintptr(unsafe.Pointer(&regMem[0])),
		memAccess:   make(map[int]int), // Initialize the map
	}, nil
}
func (rvm *RecompilerRam) Close() error {
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
	return err
}

// GetMemAccess checks access rights for a memory range.
// Returns the most restrictive access level across all pages spanned by the range.
// If any page in the range is inaccessible, returns PageInaccessible.
// If all pages are at least readable but any is immutable, returns PageImmutable.
// Only returns PageMutable if ALL pages in the range are mutable.
func (rvm *RecompilerRam) GetMemAccess(address uint32, length uint32) (int, error) {
	if length == 0 {
		return PageInaccessible, fmt.Errorf("zero length")
	}

	// Check for overflow
	if address > ^uint32(0)-(length-1) {
		return PageInaccessible, fmt.Errorf("address range overflow: addr=0x%x len=0x%x", address, length)
	}

	startPage := int(address / PageSize)
	endAddress := address + length - 1
	endPage := int(endAddress / PageSize)

	if startPage < 0 || startPage >= TotalPages || endPage < 0 || endPage >= TotalPages {
		return PageInaccessible, fmt.Errorf("invalid address range: pages %d~%d out of bounds", startPage, endPage)
	}

	// Check all pages in the range and return the most restrictive access
	// PageInaccessible (0) < PageImmutable (PROT_READ) < PageMutable (PROT_READ|PROT_WRITE)
	mostRestrictive := PageMutable
	for pageIdx := startPage; pageIdx <= endPage; pageIdx++ {
		access, ok := rvm.memAccess[pageIdx]
		if !ok {
			access = PageInaccessible
		}
		if access == PageInaccessible {
			return PageInaccessible, nil // Early return - can't get more restrictive
		}
		if access < mostRestrictive {
			mostRestrictive = access
		}
	}
	return mostRestrictive, nil
}

// ReadMemory reads data from a specific address in the memory if it's readable.
func (rvm *RecompilerRam) ReadMemory(address uint32, length uint32) (data []byte, err error) {
	if length == 0 {
		return []byte{}, nil
	}
	endAddr := address + length
	if endAddr < address {
		return nil, fmt.Errorf("range overflow: addr=%x len=%d", address, length)
	}
	if int(endAddr) > len(rvm.realMemory) {
		return nil, fmt.Errorf("out of bounds: end=%x memlen=%d", endAddr, len(rvm.realMemory))
	}
	access, err := rvm.GetMemAccess(address, length)
	if err != nil {
		return nil, fmt.Errorf("failed to get memory access: %w", err)
	}
	if access == PageInaccessible {
		return nil, fmt.Errorf("memory at address %x is not readable", address)
	}

	return rvm.realMemory[int(address):int(endAddr)], nil
}

// WriteMemory writes data to a specific address in the memory if it's writable.
// Validates that all pages spanned by the write are mutable before writing.
func (rvm *RecompilerRam) WriteMemory(address uint32, data []byte) error {
	if len(data) == 0 {
		return nil
	}

	// Check for overflow
	endAddr := uint64(address) + uint64(len(data))
	if endAddr > uint64(len(rvm.realMemory)) {
		return fmt.Errorf("out of bounds: write at 0x%x with len %d exceeds memory size %d",
			address, len(data), len(rvm.realMemory))
	}

	// Check that all pages in the range are writable
	access, err := rvm.GetMemAccess(address, uint32(len(data)))
	if err != nil {
		return fmt.Errorf("failed to get memory access: %w", err)
	}
	if access != PageMutable {
		return fmt.Errorf("memory at address 0x%x is not writable (access=%d)", address, access)
	}

	// Direct copy using the address - no page-based offset calculation needed
	copy(rvm.realMemory[address:uint32(endAddr)], data)
	return nil
}

// SetPageAccess sets the memory protection of a single page using BaseReg as memory base.
func (rvm *RecompilerRam) SetPageAccess(pageIndex int, access int) error {
	if pageIndex < 0 || pageIndex >= TotalPages {
		return fmt.Errorf("invalid page index")
	}

	start := uintptr(pageIndex * PageSize)
	basePtr := rvm.realMemAddr
	memSlice := unsafe.Slice((*byte)(unsafe.Pointer(basePtr+start)), PageSize)

	if err := syscall.Mprotect(memSlice, access); err != nil {
		return fmt.Errorf("mprotect failed: %v", err)
	}

	rvm.memAccess[pageIndex] = access
	return nil
}
func (rvm *RecompilerRam) SetPagesAccessRange(startPage, pageCount int, access int) error {
	if startPage < 0 || pageCount <= 0 || startPage+pageCount > TotalPages {
		return fmt.Errorf("invalid page range")
	}
	endPage := startPage + pageCount
	for i := startPage; i < endPage; i++ {
		rvm.memAccess[i] = access
	}

	byteStart := startPage * PageSize
	byteEnd := endPage * PageSize
	mem := rvm.realMemory[byteStart:byteEnd]
	if err := unix.Mprotect(mem, access); err != nil {
		return fmt.Errorf("mprotect failed: %w", err)
	}
	runtime.KeepAlive(rvm.realMemory)
	return nil
}

func (rvm *RecompilerRam) SetMemAccess(address uint32, length uint32, access byte) error {
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
	return rvm.SetPagesAccessRange(startPage, endPage-startPage+1, int(access))
}

func (rvm *RecompilerRam) WriteRAMBytes(address uint32, data []byte) (resultCode uint64) {
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
func (rvm *RecompilerRam) ReadRAMBytes(address uint32, length uint32) (data []byte, resultCode uint64) {
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

func (rvm *RecompilerRam) allocatePages(startPage uint32, count uint32) {
	rvm.SetPagesAccessRange(int(startPage), int(count), PageMutable)
}

// GetCurrentHeapPointer
func (rvm *RecompilerRam) GetCurrentHeapPointer() uint32 {
	return rvm.current_heap_pointer
}

func (rvm *RecompilerRam) SetCurrentHeapPointer(pointer uint32) {
	rvm.current_heap_pointer = pointer
}

// 	GetCurrentHeapPointer() uint32
// 	SetCurrentHeapPointer(pointer uint32)
// 	ReadRegister(index int) (uint64, uint64)
// 	WriteRegister(index int, value uint64) uint64
// 	ReadRegisters() []uint64

func (rvm *RecompilerRam) ReadRegister(index int) uint64 {
	start := index * 8
	return binary.LittleEndian.Uint64(rvm.regDumpMem[start : start+8])
}

func (rvm *RecompilerRam) WriteRegister(index int, value uint64) {
	start := index * 8
	*(*uint64)(unsafe.Pointer(&rvm.regDumpMem[start])) = value
}

// ReadRegisters returns a copy of the current register values.
func (rvm *RecompilerRam) ReadRegisters() [regSize]uint64 {
	return *(*[regSize]uint64)(unsafe.Pointer(&rvm.regDumpMem[0]))
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
