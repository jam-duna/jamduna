package erasurecoding

import (
    "fmt"
    "unsafe"
)

/*
#cgo LDFLAGS: -L. -ltest_vectors
#include <stdlib.h>
#include "erasurecoding.h"
*/
import "C"

func Encode(original []byte) ([]byte, error) {
    // Encode the original data
    input := C.CBytes(original)
    defer C.free(input)
    inputLen := C.size_t(len(original))
    numSegments := len(original)/4096 + 1
    output := make([]byte, numSegments*12312)
    outputPtr := C.CBytes(output)
    defer C.free(outputPtr)
    outputLen := C.size_t(len(output))

    encodedLen := int(C.encode_segments((*C.uchar)(input), inputLen, (*C.uchar)(outputPtr), outputLen))
    if encodedLen <= 0 {
        return []byte{}, fmt.Errorf("Encoding failed with error code: %d", int(encodedLen))
    }
    encodedOutput := C.GoBytes(outputPtr, C.int(encodedLen))
    return encodedOutput, nil
}

func Decode(packageSize uint32, encoded []byte) ([]byte, error) {
    numSegments := (int(packageSize) / 4096) + 1
    result := make([]byte, numSegments*4096)
    available := make([]byte, 1026)
    for i := 0 ; i < 1026; i++ {
       available[i] = 1;
    }
    for segment := 0; segment < numSegments; segment++ {
        decodeOutput := make([]byte, 4096)
        decodeOutputPtr := C.CBytes(decodeOutput)
        defer C.free(decodeOutputPtr)

        encodedData := encoded[segment*12312 : (segment+1)*12312]
        //fmt.Printf("segment %d out of %d decoding from %d bytes out of %d\n", segment, numSegments, len(encodedData), len(encoded))
        decodedLen := int(C.decode_segment(C.int(segment), (*C.uchar)(unsafe.Pointer(&encodedData[0])), (*C.uchar)(unsafe.Pointer(&available[0])), (*C.uchar)(decodeOutputPtr)))
        if decodedLen <= 0 {
            return nil, fmt.Errorf("Decoding failed with error code: %d", decodedLen)
        }
        decodedOutput := C.GoBytes(decodeOutputPtr, C.int(decodedLen))
        copy(result[segment*4096:], decodedOutput)
    }
    return result[:packageSize], nil
}



