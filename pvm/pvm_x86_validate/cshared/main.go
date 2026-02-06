package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"unsafe"

	"github.com/jam-duna/jamduna/pvm/recompiler"
)

//export GetX86Bytes
func GetX86Bytes(opcode C.uchar, operands *C.uchar, operandsLen C.int, outLen *C.int) *C.uchar {
	var opBytes []byte
	if operandsLen > 0 {
		opBytes = C.GoBytes(unsafe.Pointer(operands), operandsLen)
	}

	x86, err := recompiler.GetX86Bytes(byte(opcode), opBytes)
	if err != nil {
		*outLen = 0
		return nil
	}
	*outLen = C.int(len(x86))
	if len(x86) == 0 {
		return nil
	}
	return (*C.uchar)(C.CBytes(x86))
}

//export FreeBuffer
func FreeBuffer(ptr *C.uchar) {
	if ptr == nil {
		return
	}
	C.free(unsafe.Pointer(ptr))
}

func main() {}
