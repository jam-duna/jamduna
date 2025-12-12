package x86gencheck

/*
#cgo CFLAGS: -I../recompiler_c/include -std=c99
#cgo LDFLAGS: -L${SRCDIR}/../recompiler_c/lib ${SRCDIR}/../recompiler_c/lib/libcompiler.a

#include <stdlib.h>
#include <stdint.h>
#include <stdbool.h>

// Forward declare the function we need (instead of including the full header)
int generate_single_instruction_x86(uint8_t opcode, const uint8_t* args, size_t args_size,
                                    uint8_t* output_buffer, size_t buffer_size);
*/
import "C"
import (
	"unsafe"

	"github.com/colorfulnotion/jam/pvm/recompiler"
)

type Instruction = recompiler.Instruction

func GetX86CodeFromGo(opcode byte, inst Instruction) []byte {
	if fn, exists := recompiler.GetPvmeToX86Code()[opcode]; exists {
		return fn(inst)
	}
	return nil
}

func GetX86CodeFromC(opcode byte, inst Instruction) []byte {
	// Prepare args for C function
	var cArgs *C.uint8_t
	argsSize := len(inst.Args)
	if argsSize > 0 {
		cArgs = (*C.uint8_t)(C.CBytes(inst.Args))
		defer C.free(unsafe.Pointer(cArgs))
	}

	// Prepare output buffer for x86 code
	outputBuffer := make([]byte, 256)
	cOutputBuffer := (*C.uint8_t)(unsafe.Pointer(&outputBuffer[0]))

	// Call C function to generate x86 code using the individual translator interface
	codeSize := C.generate_single_instruction_x86(
		C.uint8_t(opcode),
		cArgs,
		C.size_t(argsSize),
		cOutputBuffer,
		C.size_t(len(outputBuffer)),
	)

	// Return the generated x86 code
	if codeSize > 0 {
		return outputBuffer[:codeSize:codeSize]
	}

	return nil
}
