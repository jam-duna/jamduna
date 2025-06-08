package x86_execute

/*
#cgo CFLAGS: -Wall
#include <stdlib.h>
#include "x86_execute.h"
*/
import "C"
import (
	"unsafe"
)

// ExecuteX86 runs raw x86 machine code, returns:
// 0 = success
// -1 = caught SIGSEGV
// -2 = mmap failed
// -3 = mprotect failed
func ExecuteX86(code []byte) int {
	ptr := (*C.uint8_t)(C.CBytes(code))
	defer C.free(unsafe.Pointer(ptr))

	return int(C.execute_x86(ptr, C.size_t(len(code))))
}
