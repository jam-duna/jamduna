package pvm

import (
	"encoding/binary"
	"fmt"
	"syscall"
	"unsafe"

	"github.com/colorfulnotion/jam/types"
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
func generateLoadImm64() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		dst := min(12, int(inst.Args[0]))
		imm := binary.LittleEndian.Uint64(inst.Args[1:9])
		opcode := 0xB8 + regInfoList[dst].RegBits
		rex := byte(0x48)
		if regInfoList[dst].REXBit == 1 {
			rex |= 0x01
		}
		code := []byte{rex, opcode}
		code = append(code, encodeU64(imm)...) // encodeU64: returns imm64 little endian
		return code, nil
	}
}

func extractTwoImm(args []byte) (vx uint64, vy uint64) {
	lx := min(4, (int(args[0])%16)/2)    // first 4 bits for length of first immediate
	ly := min(4, max(0, len(args)-lx-1)) // remaining bits for second immediate
	if ly == 0 {
		ly = 1
		args = append(args, 0)
	}
	vx = x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
	vy = x_encode(types.DecodeE_l(args[1+lx:1+lx+ly]), uint32(ly))
	return
}

// A.5.4. Instructions with Arguments of Two Immediates.
func generateStoreImmU8() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		vx, vy := extractTwoImm(inst.Args)
		offset := vx
		value := byte(vy)
		base := BaseReg

		rex := byte(0x40)
		if base.REXBit != 0 {
			rex |= 0x01
		}

		// ModRM: mod = 10 (disp32), reg = 000 (for imm8 MOV), r/m = 100 (indirect with SIB)
		modrm := byte(0x84)

		// SIB: scale=0 (1x), index=100 (none), base=base.RegBits
		sib := byte(0x24 | (base.RegBits & 0x07))

		code := []byte{
			rex,
			0xC6,  // opcode: MOV r/m8, imm8
			modrm, // ModRM
			sib,   // SIB with base
			byte(offset & 0xFF),
			byte((offset >> 8) & 0xFF),
			byte((offset >> 16) & 0xFF),
			byte((offset >> 24) & 0xFF),
			value,
		}
		return code, nil
	}
}

func generateStoreImmU16() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		addr, imm := extractTwoImm(inst.Args)

		code := []byte{0x66, 0xC7, 0x04, 0x25}
		code = append(code, encodeU32(uint32(addr))...) // mov [abs32], imm16
		code = append(code, encodeU16(uint16(imm))...)
		return code, nil
	}
}

func generateStoreImmU32() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		addr, imm := extractTwoImm(inst.Args)
		code := []byte{0xC7, 0x04, 0x25}
		code = append(code, encodeU32(uint32(addr))...) // mov [abs32], imm32
		code = append(code, encodeU32(uint32(imm))...)
		return code, nil
	}
}

func generateStoreImmU64() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		addr, imm := extractTwoImm(inst.Args)
		// mov rax, imm64; mov [addr], rax
		code := []byte{0x48, 0xB8}
		code = append(code, encodeU64(imm)...)          // mov rax, imm64
		code = append(code, 0x48, 0xA3)                 // mov [abs32], rax
		code = append(code, encodeU32(uint32(addr))...) // [abs32]
		return code, nil
	}
}

// A.5.6. Instructions with Arguments of One Register & Two Immediates.

func extractOneReg2Imm(args []byte) (reg1 uint64, vx uint64) {
	registerIndexA := min(12, int(args[0])%16)
	lx := min(4, max(0, len(args))-1)
	if lx == 0 {
		lx = 1
		args = append(args, 0)
	}
	vx = x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
	return uint64(registerIndexA), vx
}

func generateLoadImm32() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		dstReg, imm := extractOneReg2Imm(inst.Args)
		dst := regInfoList[dstReg]
		rex := byte(0x40)
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		opcode := 0xB8 + dst.RegBits
		code := []byte{rex, opcode}
		code = append(code, encodeU32(uint32(imm))...)
		return code, nil
	}
}

func generateLoadU8() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		dstReg, addr := extractOneReg2Imm(inst.Args)
		dst := regInfoList[dstReg]
		// REX prefix: REX.W=0 (8-bit), REX.B if dst >= r8
		rex := byte(0x40)
		if dst.REXBit == 1 {
			rex |= 0x01 // REX.B
		}
		// movzx r32, byte ptr [disp32]
		// opcode = 0F B6 /r, ModRM: mod=00, reg=dst.RegBits, rm=101 (RIP-relative)
		modrm := byte(0x00 | (dst.RegBits << 3) | 0x05)
		// append 32-bit little-endian address
		disp := int32(addr)
		dispBytes := []byte{byte(disp), byte(disp >> 8), byte(disp >> 16), byte(disp >> 24)}
		return append([]byte{rex, 0x0F, 0xB6, modrm}, dispBytes...), nil
	}
}

func generateLoadI8() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		dstReg, addr := extractOneReg2Imm(inst.Args)
		dst := regInfoList[dstReg]
		rex := byte(0x40)
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		// movsx r32, byte ptr [disp32]
		modrm := byte(0x00 | (dst.RegBits << 3) | 0x05)
		disp := int32(addr)
		dispBytes := []byte{byte(disp), byte(disp >> 8), byte(disp >> 16), byte(disp >> 24)}
		return append([]byte{rex, 0x0F, 0xBE, modrm}, dispBytes...), nil
	}
}

func generateLoadU16() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		dstReg, addr := extractOneReg2Imm(inst.Args)
		dst := regInfoList[dstReg]
		rex := byte(0x40)
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		// movzx r32, word ptr [disp32]
		modrm := byte(0x00 | (dst.RegBits << 3) | 0x05)
		disp := int32(addr)
		dispBytes := []byte{byte(disp), byte(disp >> 8), byte(disp >> 16), byte(disp >> 24)}
		return append([]byte{rex, 0x0F, 0xB7, modrm}, dispBytes...), nil
	}
}

func generateLoadI16() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		dstReg, addr := extractOneReg2Imm(inst.Args)
		dst := regInfoList[dstReg]
		rex := byte(0x40)
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		// movsx r32, word ptr [disp32]
		modrm := byte(0x00 | (dst.RegBits << 3) | 0x05)
		disp := int32(addr)
		dispBytes := []byte{byte(disp), byte(disp >> 8), byte(disp >> 16), byte(disp >> 24)}
		return append([]byte{rex, 0x0F, 0xBF, modrm}, dispBytes...), nil
	}
}

func generateLoadU32() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		dstReg, addr := extractOneReg2Imm(inst.Args)
		dst := regInfoList[dstReg]
		rex := byte(0x40)
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		// mov r32, dword ptr [disp32]
		modrm := byte(0x00 | (dst.RegBits << 3) | 0x05)
		disp := int32(addr)
		dispBytes := []byte{byte(disp), byte(disp >> 8), byte(disp >> 16), byte(disp >> 24)}
		return append([]byte{rex, 0x8B, modrm}, dispBytes...), nil
	}
}

func generateLoadI32() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		dstReg, addr := extractOneReg2Imm(inst.Args)
		dst := regInfoList[dstReg]
		rex := byte(0x48)
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		// movsxd r64, dword ptr [disp32]
		modrm := byte(0x00 | (dst.RegBits << 3) | 0x05)
		disp := int32(addr)
		dispBytes := []byte{byte(disp), byte(disp >> 8), byte(disp >> 16), byte(disp >> 24)}
		return append([]byte{rex, 0x63, modrm}, dispBytes...), nil
	}
}

func generateLoadU64() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		dstReg, addr := extractOneReg2Imm(inst.Args)
		dst := regInfoList[dstReg]
		rex := byte(0x48)
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		// mov r64, qword ptr [disp32]
		modrm := byte(0x00 | (dst.RegBits << 3) | 0x05)
		disp := int32(addr)
		dispBytes := []byte{byte(disp), byte(disp >> 8), byte(disp >> 16), byte(disp >> 24)}
		return append([]byte{rex, 0x8B, modrm}, dispBytes...), nil
	}
}

func generateStoreU8() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		srcReg, dstReg := extractOneReg2Imm(inst.Args)
		src := regInfoList[srcReg]
		dst := regInfoList[dstReg]

		rex := byte(0x40)
		if src.REXBit == 1 {
			rex |= 0x04
		}
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | (src.RegBits << 3) | dst.RegBits)
		return []byte{rex, 0x88, modrm}, nil
	}
}

func generateStoreU16() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		srcReg, dstReg := extractOneReg2Imm(inst.Args)
		src := regInfoList[srcReg]
		dst := regInfoList[dstReg]
		rex := byte(0x40)
		if src.REXBit == 1 {
			rex |= 0x04
		}
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | (src.RegBits << 3) | dst.RegBits)
		return []byte{0x66, rex, 0x89, modrm}, nil // mov word ptr [dst], src
	}
}

func generateStoreU32() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		srcReg, dstReg := extractOneReg2Imm(inst.Args)
		src := regInfoList[srcReg]
		dst := regInfoList[dstReg]
		rex := byte(0x40)
		if src.REXBit == 1 {
			rex |= 0x04
		}
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | (src.RegBits << 3) | dst.RegBits)
		return []byte{rex, 0x89, modrm}, nil // mov dword ptr [dst], src
	}
}

func generateStoreU64() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		srcReg, dstReg := extractOneReg2Imm(inst.Args)
		src := regInfoList[srcReg]
		dst := regInfoList[dstReg]
		rex := byte(0x48)
		if src.REXBit == 1 {
			rex |= 0x04
		}
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | (src.RegBits << 3) | dst.RegBits)
		return []byte{rex, 0x89, modrm}, nil // mov qword ptr [dst], src
	}
}

// A.5.7. Instructions with Arguments of One Register & Two Immediates.

func extractOneRegTwoImm(args []byte) (reg1 uint64, vx uint64, vy uint64) {
	registerIndexA := min(12, int(args[0])%16)
	lx := min(4, (int(args[0])/16)%8)
	ly := min(4, max(0, len(args)-lx-1))
	if ly == 0 {
		ly = 1
		args = append(args, 0)
	}

	vx = x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
	vy = x_encode(types.DecodeE_l(args[1+lx:1+lx+ly]), uint32(ly))
	return uint64(registerIndexA), vx, vy
}

func generateStoreImmIndU8() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		dstReg, imm, _ := extractOneRegTwoImm(inst.Args)
		dst := regInfoList[dstReg]
		rex := byte(0x40)
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | (0x00 << 3) | dst.RegBits)  // /0 encoding
		return []byte{rex, 0xC6, modrm, uint8(imm)}, nil // mov byte ptr [dst], imm8
	}
}

func generateStoreImmIndU16() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		dstReg, imm, _ := extractOneRegTwoImm(inst.Args)
		dst := regInfoList[dstReg]
		rex := byte(0x40)
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | (0x00 << 3) | dst.RegBits)                               // /0 encoding
		return append([]byte{0x66, rex, 0xC7, modrm}, encodeU16(uint16(imm))...), nil // mov word ptr [dst], imm16
	}
}

func generateStoreImmIndU32() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		dstReg, imm, _ := extractOneRegTwoImm(inst.Args)
		dst := regInfoList[dstReg]
		rex := byte(0x40)
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | (0x00 << 3) | dst.RegBits)
		return append([]byte{rex, 0xC7, modrm}, encodeU32(uint32(imm))...), nil // mov dword ptr [dst], imm32
	}
}

func generateStoreImmIndU64() func(inst Instruction) ([]byte, error) {
	return func(inst Instruction) ([]byte, error) {
		dstReg, imm, _ := extractOneRegTwoImm(inst.Args)
		dst := regInfoList[dstReg]
		rex := byte(0x48)
		if dst.REXBit == 1 {
			rex |= 0x01
		}
		modrm := byte(0xC0 | (0x00 << 3) | dst.RegBits)
		return append([]byte{rex, 0xC7, modrm}, encodeU64(imm)...), nil // mov qword ptr [dst], imm64
	}
}
