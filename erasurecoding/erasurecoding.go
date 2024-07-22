package erasurecoding

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"unsafe"
)

/*
#cgo linux   LDFLAGS: -L${SRCDIR}/lib/linux -ltest_vectors
#cgo darwin  LDFLAGS: -L${SRCDIR}/lib/darwin -ltest_vectors
#include <stdlib.h>
#include "erasurecoding.h"
*/
import "C"

type AtomicShards struct {
	Data    string `json:"data"`
	Segment struct {
		Segments []struct {
			SegmentEC []string `json:"segment_ec"`
		} `json:"segments"`
	} `json:"segment"`
}

func Encode(original []byte) ([]byte, error) {
	// Encode the original data
	input := C.CBytes(original)
	defer C.free(input)
	inputLen := C.size_t(len(original))
	if len(original) == 0 {
		return []byte{}, nil
	}
	numSegments := (len(original)-1)/4104 + 1
	output := make([]byte, numSegments*12312)
	outputPtr := C.CBytes(output)
	defer C.free(outputPtr)
	outputLen := C.size_t(len(output))
	encodedLen := int(C.encode_segments((*C.uchar)(input), inputLen, (*C.uchar)(outputPtr), outputLen))

	if encodedLen <= 0 {
		return []byte{}, fmt.Errorf("encoding failed with error code: %d", int(encodedLen))
	}

	encodedOutput := C.GoBytes(outputPtr, C.int(encodedLen))
	return encodedOutput, nil
}

func Decode(packageSize uint32, encoded []byte, available []byte) ([]byte, error) {
	if len(encoded) == 0 {
		return []byte{}, nil
	}
	if len(available) != 1026 {
		return nil, fmt.Errorf("available length must be 1026 bytes")
	}
	count := 0
	for _, val := range available {
		if val == 1 {
			count++
		}
	}
	if count < 342 {
		return nil, fmt.Errorf("insufficient available elements for decoding")
	}

	numSegments := ((int(packageSize) - 1) / 4104) + 1

	result := make([]byte, numSegments*4104)
	for segment := 0; segment < numSegments; segment++ {
		decodeOutput := make([]byte, 4104)
		decodeOutputPtr := C.CBytes(decodeOutput)
		defer C.free(decodeOutputPtr)

		encodedData := encoded[segment*12312 : (segment+1)*12312]
		//fmt.Printf("segment %d out of %d decoding from %d bytes out of %d\n", segment, numSegments, len(encodedData), len(encoded))
		decodedLen := int(C.decode_segment(C.int(segment), (*C.uchar)(unsafe.Pointer(&encodedData[0])), (*C.uchar)(unsafe.Pointer(&available[0])), (*C.uchar)(decodeOutputPtr)))
		if decodedLen <= 0 {
			return nil, fmt.Errorf("decoding failed with error code: %d", decodedLen)
		}
		decodedOutput := C.GoBytes(decodeOutputPtr, C.int(decodedLen))
		copy(result[segment*4104:], decodedOutput)
	}
	return result[:packageSize], nil
}

// GetShards with padding
func GetSubshards(data []byte) [][][]byte {
	segments := (len(data)) / 1026 / 12
	shards := 1026
	shardLength := 12
	array := make([][][]byte, segments)
	for i := 0; i < segments; i++ {
		array[i] = make([][]byte, shards)
		for j := 0; j < shards; j++ {
			array[i][j] = make([]byte, shardLength)
			for k := 0; k < shardLength; k++ {
				index := i*12312 + j*12 + k
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

// GetShards without padding
func GetSubshardsAtomic(data []byte, original []byte) [][][]byte {
	segmentsNum := len(original) / 4104
	datashardslength := (((len(original)%4104)-1)/684 + 1) * 2
	segments := (len(data)) / 1026 / 12
	shards := 1026
	shardLength := 12
	array := make([][][]byte, segments)
	for i := 0; i < segments; i++ {
		if i < segmentsNum {
			array[i] = make([][]byte, shards)
			for j := 0; j < shards; j++ {
				array[i][j] = make([]byte, shardLength)
				for k := 0; k < shardLength; k++ {
					index := i*12312 + j*12 + k
					if index < len(data) {
						array[i][j][k] = data[index]
					} else {
						array[i][j][k] = 0 // padding
					}
				}
			}
		} else {
			array[i] = make([][]byte, shards)
			for j := 0; j < shards; j++ {
				array[i][j] = make([]byte, datashardslength)
				for k := 0; k < datashardslength; k++ {
					index := i*12312 + j*12 + k
					if index < len(data) {
						array[i][j][k] = data[index]
					} else {
						array[i][j][k] = 0 // padding
					}
				}
			}
		}

	}

	return array
}

// Converts the 3D array to JSON with hex encoding and saves files
func ConvertToJSON(data []byte, encodedArray [][][]byte, filePath string) error {
	originalDataHex := hex.EncodeToString(data)

	segments := make([]struct {
		SegmentEC []string `json:"segment_ec"`
	}, len(encodedArray))
	for i, arr2D := range encodedArray {
		segmentShards := make([]string, len(arr2D))
		for j, arr1D := range arr2D {
			hexShard := ""
			for _, v := range arr1D {
				hexShard += hex.EncodeToString([]byte{v})
			}
			segmentShards[j] = hexShard
		}
		segments[i] = struct {
			SegmentEC []string `json:"segment_ec"`
		}{SegmentEC: segmentShards}
	}

	jsonOutput := AtomicShards{
		Data: originalDataHex,
		Segment: struct {
			Segments []struct {
				SegmentEC []string `json:"segment_ec"`
			} `json:"segments"`
		}{
			Segments: segments,
		},
	}

	jsonData, err := json.MarshalIndent(jsonOutput, "", "  ")
	if err != nil {
		return err
	}

	// Save to the specified file path
	err = ioutil.WriteFile(filePath, jsonData, 0644)
	if err != nil {
		return err
	}

	return nil
}
