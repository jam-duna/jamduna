package erasurecoding

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"unsafe"
)

/*
#cgo LDFLAGS: -L./tools/test_vectors/target/release -ltest_vectors
#include <stdlib.h>
#include "erasurecoding.h"
*/
import "C"

func Encode(original []byte) ([]byte, error) {
	// Encode the original data
	input := C.CBytes(original)
	defer C.free(input)
	inputLen := C.size_t(len(original))
	if len(original) == 0 {
		return []byte{}, nil
	}
	numSegments := (len(original)-1)/4096 + 1
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
	if len(encoded) == 0 {
		return []byte{}, nil
	}

	numSegments := ((int(packageSize) - 1) / 4096) + 1

	result := make([]byte, numSegments*4096)
	available := make([]byte, 1026)
	for i := 0; i < 1026; i++ {
		available[i] = 1
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

func GetSubshards(data []byte) [][][]byte {
	x := (len(data)) / 1026 / 12
	y := 1026
	z := 12
	array := make([][][]byte, x)
	for i := 0; i < x; i++ {
		array[i] = make([][]byte, y)
		for j := 0; j < y; j++ {
			array[i][j] = make([]byte, z)
			for k := 0; k < z; k++ {
				index := i*4096 + j*12 + k
				if index < len(data) {
					array[i][j][k] = data[index]
				} else {
					array[i][j][k] = 0 // padding
				}
			}
		}
	}

	return array
}

// Used for JSON output=============================================================
// SegmentShards holds the segments and their shards in hex
type SegmentShards struct {
	Segment []string `json:"segment"`
}

// JSONOutput holds the original data and segments in JSON format
type JSONOutput struct {
	Data    string          `json:"data"`
	Segment []SegmentShards `json:"segments"`
}

// Converts the 3D array to JSON with hex encoding and saves to codewords.json
func ConvertToJSON(data []byte, encodedArray [][][]byte) error {
	originalDataHex := hex.EncodeToString(data)

	segments := make([]SegmentShards, len(encodedArray))
	for i, arr2D := range encodedArray {
		var segmentShards []string
		for _, arr1D := range arr2D {
			hexShard := ""
			for _, v := range arr1D {
				hexShard += hex.EncodeToString([]byte{v})
			}
			segmentShards = append(segmentShards, hexShard)
		}
		segments[i] = SegmentShards{Segment: segmentShards}
	}

	jsonOutput := JSONOutput{
		Data:    originalDataHex,
		Segment: segments,
	}

	jsonData, err := json.MarshalIndent(jsonOutput, "", "  ")
	if err != nil {
		return err
	}

	// Save to codewords.json
	err = ioutil.WriteFile("codewords.json", jsonData, 0644)
	if err != nil {
		return err
	}

	return nil
}
