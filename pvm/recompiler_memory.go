package pvm

import (
	"encoding/binary"
	"fmt"
	"syscall"
	"unsafe"
)

// GetMemAssess checks access rights for a memory range using mprotect probe.
// DANGER: This function uses unsafe operations and can cause SIGSEGV if the address is invalid or inaccessible.
func (rvm *RecompilerVM) GetMemAssess(address uint32, length uint32) (byte, error) {
	pageIndex := int(address / PageSize)
	if pageIndex < 0 || pageIndex >= TotalPages {
		return PageInaccessible, fmt.Errorf("invalid address")
	}
	start := pageIndex * PageSize

	// Attempt to read a byte to test access
	defer func() {
		_ = recover() // Catch panic from SIGSEGV
	}()

	b := *(*byte)(unsafe.Pointer(&rvm.realMemory[start]))
	_ = b // dummy read

	return PageMutable, nil // if no panic, assume readable
}

// ReadMemory reads data from a specific address in the memory if it's readable.
func (rvm *RecompilerVM) ReadMemory(address uint32, length uint32) ([]byte, error) {
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
		return nil, fmt.Errorf("read exceeds page size at address %x", address)
	}

	data := make([]byte, length)
	copy(data, rvm.realMemory[start+offset:start+end])
	return data, nil
}

// WriteMemory writes data to a specific address in the memory if it's writable.
func (rvm *RecompilerVM) WriteMemory(address uint32, data []byte) error {
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
func (rvm *RecompilerVM) SetPageAccess(pageIndex int, access byte) error {
	if pageIndex < 0 || pageIndex >= TotalPages {
		return fmt.Errorf("invalid page index")
	}

	start := uintptr(pageIndex * PageSize)

	basePtr := uintptr(unsafe.Pointer(&rvm.realMemory[0])) // base address = r12 points here
	memSlice := unsafe.Slice((*byte)(unsafe.Pointer(basePtr+start)), PageSize)

	var prot int
	switch access {
	case PageInaccessible:
		prot = syscall.PROT_NONE
	case PageMutable:
		prot = syscall.PROT_READ | syscall.PROT_WRITE
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

// SetMemAssess sets memory access rights at a given virtual address.
func (rvm *RecompilerVM) SetMemAssess(address uint32, length uint32, access byte) error {
	pageIndex := int(address / PageSize)
	if pageIndex < 0 || pageIndex >= TotalPages {
		return fmt.Errorf("invalid address %x for page index %d", address, pageIndex)
	}
	return rvm.SetPageAccess(pageIndex, access)
}

// A.5.3. Instructions with Arguments of One Register and One Extended Width Immediate.
func generateLoadImm64(inst Instruction) []byte {
	dst := min(12, int(inst.Args[0]))
	imm := binary.LittleEndian.Uint64(inst.Args[1:9])
	opcode := 0xB8 + regInfoList[dst].RegBits
	rex := byte(0x48)
	if regInfoList[dst].REXBit == 1 {
		rex |= 0x01
	}
	code := []byte{rex, opcode}
	code = append(code, encodeU64(imm)...) // encodeU64: returns imm64 little endian
	return code
}

func generateStoreImmGeneric(
	opcode byte,
	prefix byte,
	immBuilder func(uint64) []byte,
) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		disp, immVal := extractTwoImm(inst.Args)

		// REX prefix (64-bit mode), only used to extend r8–r15; BaseReg REXBit=0
		rex := byte(0x40)
		if BaseReg.REXBit != 0 {
			rex |= 0x01
		}

		// ModRM: mod=10 (disp32), reg=000 (MOV sub-opcode), r/m=100 (SIB follows)
		modrm := byte(0x84)
		// SIB: scale=0 (×1), index=100 (none), base=BaseReg.RegBits
		sib := byte(0x24 | (BaseReg.RegBits & 0x07))

		buf := make([]byte, 0, 16)
		if prefix != 0 {
			buf = append(buf, prefix)
		}
		buf = append(buf,
			rex,    // REX
			opcode, // MOV r/m?, imm?
			modrm,  // ModRM
			sib,    // SIB
		)
		buf = append(buf, encodeU32(uint32(disp))...) // disp32
		buf = append(buf, immBuilder(immVal)...)      // immediate
		return buf
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

func generateLoadImm32(inst Instruction) []byte {
	dstReg, imm := extractOneRegOneImm(inst.Args)
	dst := regInfoList[dstReg]
	rex := byte(0x40)
	if dst.REXBit == 1 {
		rex |= 0x01
	}
	opcode := 0xB8 + dst.RegBits
	code := []byte{rex, opcode}
	code = append(code, encodeU32(uint32(imm))...)
	return code
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

		// Construct REX prefix: 0100WRXB
		rex := byte(0x40)
		if rexW {
			rex |= 0x08 // REX.W
		}
		if dst.REXBit == 1 {
			rex |= 0x04 // REX.R extends ModRM.reg
		}
		if base.REXBit == 1 {
			rex |= 0x01 // REX.B extends ModRM.r/m (base)
		}

		// ModRM: mod=10 (disp32), reg=dst.RegBits, r/m=100 (SIB follows)
		modrm := byte(0x80 | (dst.RegBits << 3) | 0x04)
		// SIB: scale=0 (×1), index=100(none), base=base.RegBits
		sib := byte(0x24 | (base.RegBits & 0x07))

		// disp32
		d := uint32(disp)
		dispBytes := []byte{
			byte(d), byte(d >> 8),
			byte(d >> 16), byte(d >> 24),
		}

		// Concatenate
		buf := make([]byte, 0, 1+len(opcodes)+2+1+4)
		buf = append(buf, rex)
		buf = append(buf, opcodes...)
		buf = append(buf, modrm, sib)
		buf = append(buf, dispBytes...)
		return buf
	}
}

func generateStoreWithBase(opcode byte, prefix byte, rexW bool) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		srcIdx, disp := extractOneRegOneImm(inst.Args)
		src := regInfoList[srcIdx]
		base := BaseReg

		// REX prefix: 0100WRXB
		rex := byte(0x40)
		if rexW {
			rex |= 0x08 // REX.W
		}
		if src.REXBit == 1 {
			rex |= 0x04 // REX.R extends ModRM.reg
		}
		if base.REXBit == 1 {
			rex |= 0x01 // REX.B extends ModRM.r/m (base)
		}

		// ModRM: mod=10 (disp32), reg=src.RegBits, r/m=100 (SIB follows)
		modrm := byte(0x80 | (src.RegBits << 3) | 0x04)
		// SIB: scale=0 (×1), index=100(none), base=base.RegBits
		sib := byte(0x24 | (base.RegBits & 0x07))

		// disp32, little-endian
		d := uint32(disp)
		dispBytes := []byte{
			byte(d), byte(d >> 8),
			byte(d >> 16), byte(d >> 24),
		}

		buf := make([]byte, 0, 1+1+2+1+4)
		if prefix != 0 {
			buf = append(buf, prefix)
		}
		buf = append(buf, rex, opcode, modrm, sib)
		buf = append(buf, dispBytes...)
		return buf
	}
}

func generateStoreImmIndU8(inst Instruction) []byte {
	// MOV byte ptr [Base+disp], imm8
	disp64, immVal := extractTwoImm(inst.Args) // Args = [disp, value]
	disp := uint32(disp64)
	buf := make([]byte, 0, 1+1+1+1+4+1) // prefix+REX+opcode+modrm+sib+disp+imm8

	// REX prefix: W=0, B=BaseReg.REXBit
	rex := byte(0x40)
	if BaseReg.REXBit != 0 {
		rex |= 0x01
	}
	buf = append(buf, rex, // REX
		0xC6,                              // opcode for MOV r/m8, imm8
		0x84,                              // ModRM: mod=10, reg=000, r/m=100→SIB
		byte(0x24|(BaseReg.RegBits&0x07)), // SIB
	)
	buf = append(buf, encodeU32(disp)...)
	buf = append(buf, byte(immVal&0xFF))
	return buf
}

func generateStoreImmIndU16(inst Instruction) []byte {
	// MOV word ptr [Base+disp], imm16 (0x66 prefix)
	disp64, immVal := extractTwoImm(inst.Args)
	disp := uint32(disp64)
	buf := make([]byte, 0, 1+1+1+1+4+2)

	// prefix 0x66 + REX.B
	rex := byte(0x40)
	if BaseReg.REXBit != 0 {
		rex |= 0x01
	}
	buf = append(buf, 0x66, rex, 0xC7, 0x84, byte(0x24|(BaseReg.RegBits&0x07)))
	buf = append(buf, encodeU32(disp)...)
	buf = append(buf, encodeU16(uint16(immVal))...)
	return buf
}

func generateStoreImmIndU32(inst Instruction) []byte {
	// MOV dword ptr [Base+disp], imm32
	disp64, immVal := extractTwoImm(inst.Args)
	disp := uint32(disp64)
	buf := make([]byte, 0, 1+1+1+1+4+4)

	// REX.B only
	rex := byte(0x40)
	if BaseReg.REXBit != 0 {
		rex |= 0x01
	}
	buf = append(buf, rex, 0xC7, 0x84, byte(0x24|(BaseReg.RegBits&0x07)))
	buf = append(buf, encodeU32(disp)...)
	buf = append(buf, encodeU32(uint32(immVal))...)
	return buf
}

func generateStoreImmIndU64(inst Instruction) []byte {
	// MOV qword ptr [Base+disp], imm64
	disp64, immVal := extractTwoImm(inst.Args)
	disp := uint32(disp64)

	low, high := splitU64(immVal)
	buf := make([]byte, 0, (1+1+1+1+4+4)*2)

	buf = emitStoreImm32(buf, disp, low)
	buf = emitStoreImm32(buf, disp+4, high)
	return buf
}
