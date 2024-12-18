package sp1

/*
#cgo LDFLAGS: -L./target/release -lsp1 -ldl
#include <stdint.h>
#include <stdlib.h>
#include <sp1.h>
*/
import "C"
import (
	//	"errors"
	"unsafe"
)

func verifyGroth16Proof(proof []byte, vk []byte, public_values []byte) bool {
	if len(public_values) == 0 {
		return C.sp1_groth16_verify(
			(*C.uchar)(unsafe.Pointer(&proof[0])),
			(*C.uchar)(unsafe.Pointer(&vk[0])),
			(*C.uchar)(unsafe.Pointer(&vk[0])),
			(C.size_t)(0),
		) == 1
	} else {
		return C.sp1_groth16_verify(
			(*C.uchar)(unsafe.Pointer(&proof[0])),
			(*C.uchar)(unsafe.Pointer(&vk[0])),
			(*C.uchar)(unsafe.Pointer(&public_values[0])),
			(C.size_t)(len(public_values)),
		) == 1
	}
}

// VerifyGroth16 is the main interface to verify Groth16 proofs
func VerifyGroth16(proof []byte, vk string, pub []byte) bool {
	// Ensure proof length matches expected size
	if len(proof) != 260 {
		return false
	}

	// Ensure vk string is valid and has the expected length
	if len(vk) != 66 { // Length of "0x" + 64 hex chars
		return false
	}

	// Convert vk string (hex) to byte array
	vkBytes := []byte(vk)

	// Call the FFI function
	verified := verifyGroth16Proof(proof, vkBytes, pub)
	return verified
}
