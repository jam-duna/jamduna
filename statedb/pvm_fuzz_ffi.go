//go:build pvm_ffi
// +build pvm_ffi

// PVM Fuzz FFI - Rust FFI wrapper for generating valid PolkaVM program blobs

package statedb

/*
#cgo LDFLAGS: -L/root/go/src/github.com/colorfulnotion/polkavm/fuzz/target/release -lpolkavm_ffi -ldl -lpthread -lm
#include <stdint.h>
#include <stdlib.h>

// BlobResult from Rust FFI
typedef struct {
    uint8_t* data;
    size_t len;
    int32_t error;
} BlobResult;

// FFI functions from polkavm_ffi
extern BlobResult build_program_blob(const uint8_t* data, size_t len);
extern void free_blob(uint8_t* data, size_t len);
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// BuildProgramBlobFromRust calls the Rust FFI to generate a valid ProgramBlob
// from arbitrary fuzzing data using the `arbitrary` crate
func BuildProgramBlobFromRust(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty input data")
	}

	cData := (*C.uint8_t)(unsafe.Pointer(&data[0]))
	cLen := C.size_t(len(data))

	result := C.build_program_blob(cData, cLen)

	if result.error != 0 {
		return nil, fmt.Errorf("rust FFI error code: %d", result.error)
	}

	if result.data == nil || result.len == 0 {
		return nil, fmt.Errorf("empty blob returned")
	}

	// Copy data to Go slice
	blob := C.GoBytes(unsafe.Pointer(result.data), C.int(result.len))

	// Free the Rust-allocated memory
	C.free_blob(result.data, result.len)

	return blob, nil
}
