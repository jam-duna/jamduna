//go:build linux && amd64
// +build linux,amd64

package recompiler

/*
#cgo CFLAGS: -Wall
#include <stdlib.h>
#include <string.h>
#include "x86_execute_linux_amd64.h"
*/
import "C"
import (
	"fmt"
	"runtime"
	"time"
	"unsafe"
)

// ExecuteX86 runs raw x86 machine code, returns:
// 0 = success
// -1 = caught SIGSEGV
// -2 = mmap failed
// -3 = mprotect failed
func ExecuteX86(code []byte, regBuf []byte) (ret int, usec int64, err error) {
	defer func() {
		if r := recover(); r != nil {
			ret = -1
			err = fmt.Errorf("panic during execution: %v", r)
		}
	}()

	if len(regBuf) < 14*8 {
		return -1, 0, fmt.Errorf("regBuf too small: need ≥ %d bytes", 14*8)
	}

	// Allocate executable memory in C
	codePtr := unsafe.Pointer(&code[0])
	// Pass pointer to register dump buffer
	regPtr := unsafe.Pointer(&regBuf[0])

	// Time the call in microseconds
	start := time.Now()
	runtime.LockOSThread()
	r := C.execute_x86((*C.uint8_t)(codePtr), C.size_t(len(code)), regPtr)
	runtime.UnlockOSThread()
	usec = time.Since(start).Microseconds()
	return int(r), usec, err
}
