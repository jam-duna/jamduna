//go:build linux && amd64
// +build linux,amd64

package pvm

/*
#cgo CFLAGS: -Wall
#include <stdlib.h>
#include "x86_execute_linux_amd64.h"
*/
import "C"
import (
	"fmt"
	"runtime"
	"unsafe"
)

// ExecuteX86 runs raw x86 machine code, returns:
// 0 = success
// -1 = caught SIGSEGV
// -2 = mmap failed
// -3 = mprotect failed
func ExecuteX86(code []byte, regBuf []byte) (ret int, err error) {
	defer func() {
		if r := recover(); r != nil {
			ret = -1
			err = fmt.Errorf("panic during execution: %v", r)
		}
	}()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if len(regBuf) < 14*8 {
		return -1, fmt.Errorf("regBuf too small: need ≥ %d bytes", 14*8)
	}

	// Copy code to C heap
	codePtr := C.CBytes(code)
	defer C.free(codePtr)

	// Get the underlying memory address of the Go slice
	regPtr := unsafe.Pointer(&regBuf[0])

	// Call the C function
	r := C.execute_x86((*C.uint8_t)(codePtr), C.size_t(len(code)), regPtr)
	ret = int(r)

	// When ret == -1, regBuf has already been dumped
	return ret, nil
}
func GetEcalliAddress() uintptr {
	return uintptr(C.get_ecalli_address())
}
