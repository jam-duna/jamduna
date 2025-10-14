package recompiler_c

// #cgo CFLAGS: -I${SRCDIR}/../include
// #cgo linux,amd64 LDFLAGS: -L${SRCDIR}/../../../ffi -lcompiler.linux_amd64
// #cgo linux,arm64 LDFLAGS: -L${SRCDIR}/../../../ffi -lcompiler.linux_arm64
// #cgo darwin,amd64 LDFLAGS: -L${SRCDIR}/../../../ffi -lcompiler.mac_amd64
// #cgo darwin,arm64 LDFLAGS: -L${SRCDIR}/../../../ffi -lcompiler.mac_arm64
// #cgo windows,amd64 LDFLAGS: -L${SRCDIR}/../../../ffi -lcompiler.windows_amd64
/*
#include <stdlib.h>
#include <stdint.h>
#include <string.h>
#include "compiler.h"
*/
import "C"
import (
	"fmt"
	"unsafe"
)

// C_Compiler implements the Compiler interface using C JIT compiler
type C_Compiler struct {
	cCompiler *C.compiler_t
	code      []byte
}

// NewC_Compiler creates a new C compiler instance
// Matches Go's NewX86Compiler(code []byte) *X86Compiler
func NewC_Compiler(code []byte) *C_Compiler {
	if len(code) == 0 {
		return nil
	}

	// Create C compiler with bytecode
	cCode := (*C.uint8_t)(unsafe.Pointer(&code[0]))
	cCompiler := C.compiler_create(cCode, C.uint32_t(len(code)))
	if cCompiler == nil {
		return nil
	}

	return &C_Compiler{
		cCompiler: cCompiler,
		code:      code,
	}
}

// SetJumpTable sets the jump table (matches Go's SetJumpTable)
func (c *C_Compiler) SetJumpTable(j []uint32) error {
	if c.cCompiler == nil {
		return fmt.Errorf("compiler not initialized")
	}

	if len(j) == 0 {
		return nil
	}

	cJ := (*C.uint32_t)(unsafe.Pointer(&j[0]))
	result := C.compiler_set_jump_table(c.cCompiler, cJ, C.uint32_t(len(j)))

	if result != 0 {
		return fmt.Errorf("SetJumpTable failed with code %d", result)
	}
	return nil
}

// SetBitMask sets the bitmask (matches Go's SetBitMask)
func (c *C_Compiler) SetBitMask(bitmask []byte) error {
	if c.cCompiler == nil {
		return fmt.Errorf("compiler not initialized")
	}

	if len(bitmask) == 0 {
		return nil
	}

	cBitmask := (*C.uint8_t)(unsafe.Pointer(&bitmask[0]))
	result := C.compiler_set_bitmask(c.cCompiler, cBitmask, C.uint32_t(len(bitmask)))

	if result != 0 {
		return fmt.Errorf("SetBitMask failed with code %d", result)
	}
	return nil
}

// CompileX86Code compiles PVM bytecode to x86 machine code
// Matches Go's CompileX86Code(startPC uint64) (x86code []byte, djumpAddr uintptr, InstMapPVMToX86 map[uint32]int, InstMapX86ToPVM map[int]uint32)
func (c *C_Compiler) CompileX86Code(startPC uint64) (
	x86code []byte,
	djumpAddr uintptr,
	InstMapPVMToX86 map[uint32]int,
	InstMapX86ToPVM map[int]uint32,
) {
	if c.cCompiler == nil {
		return nil, 0, nil, nil
	}

	var x86CodePtr *C.uint8_t
	var x86CodeSize C.uint32_t
	var djumpAddrC C.uintptr_t

	// Call C compilation function
	result := C.compiler_compile(
		c.cCompiler,
		C.uint64_t(startPC),
		&x86CodePtr,
		&x86CodeSize,
		&djumpAddrC,
	)

	if result != 0 {
		return nil, 0, nil, nil
	}

	// Copy x86 code to Go slice
	x86code = C.GoBytes(unsafe.Pointer(x86CodePtr), C.int(x86CodeSize))
	djumpAddr = uintptr(djumpAddrC)

	// Get PC mappings
	InstMapPVMToX86 = c.getPVMToX86Map()
	InstMapX86ToPVM = c.getX86ToPVMMap()

	return x86code, djumpAddr, InstMapPVMToX86, InstMapX86ToPVM
}

// getPVMToX86Map retrieves the PVM PC to x86 offset mapping
func (c *C_Compiler) getPVMToX86Map() map[uint32]int {
	var count C.uint32_t
	mapPtr := C.compiler_get_pvm_to_x86_map(c.cCompiler, &count)

	if mapPtr == nil || count == 0 {
		return make(map[uint32]int)
	}

	// Convert C array to Go map
	entries := (*[1 << 30]C.pc_map_entry_t)(unsafe.Pointer(mapPtr))[:count:count]
	result := make(map[uint32]int, count)

	for i := 0; i < int(count); i++ {
		result[uint32(entries[i].pvm_pc)] = int(entries[i].x86_offset)
	}

	return result
}

// getX86ToPVMMap retrieves the x86 offset to PVM PC mapping
func (c *C_Compiler) getX86ToPVMMap() map[int]uint32 {
	var count C.uint32_t
	mapPtr := C.compiler_get_x86_to_pvm_map(c.cCompiler, &count)

	if mapPtr == nil || count == 0 {
		return make(map[int]uint32)
	}

	// Convert C array to Go map
	entries := (*[1 << 30]C.pc_map_entry_t)(unsafe.Pointer(mapPtr))[:count:count]
	result := make(map[int]uint32, count)

	for i := 0; i < int(count); i++ {
		result[int(entries[i].x86_offset)] = uint32(entries[i].pvm_pc)
	}

	return result
}

// Destroy frees the C compiler resources
func (c *C_Compiler) Destroy() {
	if c.cCompiler != nil {
		C.compiler_destroy(c.cCompiler)
		c.cCompiler = nil
	}
}

// Ensure C_Compiler implements Compiler interface at compile time
var _ interface {
	SetJumpTable(j []uint32) error
	SetBitMask(bitmask []byte) error
	CompileX86Code(startPC uint64) (x86code []byte, djumpAddr uintptr, InstMapPVMToX86 map[uint32]int, InstMapX86ToPVM map[int]uint32)
} = (*C_Compiler)(nil)
