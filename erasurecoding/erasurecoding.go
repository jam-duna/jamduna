package erasurecoding

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	"github.com/klauspost/reedsolomon"
)

var (
	// Configurations for the erasure coding library. See https://pkg.go.dev/github.com/klauspost/reedsolomon#New
	// TODO: Coding rate has been changed to 342:1023 in the latest version of the GP.
	K                int = 2 // coding rate = 342:1026. See GP, Appendix H for more details.
	N                int = 6 // coding rate = 342:1026. See GP, Appendix H for more details.
	GFPointsPerShard int = 6 // k = 6, shard size = k * 2. See GP, Appendix H.1 for more details.
)

type AtomicShards struct {
	Data    string `json:"data"`
	Segment struct {
		Segments []struct {
			SegmentEC []string `json:"segment_ec"`
		} `json:"segments"`
	} `json:"segment"`
}

func Encode(original []byte, GFPointsPerShard int) ([][][]byte, error) {
	// TODO: Using non-inplace operation on buffer, output is a bit memory inefficient.

	// Calculate the shardSize
	shardSize := GFPointsPerShard * 2                    // 1 GF point = 2 bytes
	shardSizeRounded := 64 * ((shardSize + 64 - 1) / 64) // round up to 64 bytes. See https://github.com/klauspost/reedsolomon?tab=readme-ov-file#leopard-compatible-gf16
	dataSegmentSize := shardSize * K

	// Calculate the dataShards (original) and parityShards (redundant) arguments for the RS function
	dataShards := K
	parityShards := (N - K) // N = K + parityShards

	// Initialize the RS encoder
	encoder, err := reedsolomon.New(dataShards, parityShards, reedsolomon.WithLeopardGF16(true), reedsolomon.WithAutoGoroutines(shardSizeRounded))
	if err != nil {
		return nil, err
	}

	// Calculate the number of segments, create the buffer and output.
	numSegments := (len(original) + dataSegmentSize - 1) / dataSegmentSize
	buffer := make([][][]byte, numSegments) // Buffer to store the encoded data with padding. (numSegments, dataShards + parityShards, shardSizePadded)
	output := make([][][]byte, numSegments) // Buffer to store the encoded data WITHOUT padding. (numSegments, dataShards + parityShards, shardSize)

	// Fill the buffer/output with the original data and calculate the parity shards for each segment
	for segmentIndex := 0; segmentIndex < numSegments; segmentIndex++ {

		buffer[segmentIndex] = make([][]byte, (dataShards + parityShards))
		output[segmentIndex] = make([][]byte, (dataShards + parityShards))

		for shardIndex := 0; shardIndex < (dataShards + parityShards); shardIndex++ {

			// Initialize the shard filled with zeros
			buffer[segmentIndex][shardIndex] = make([]byte, shardSizeRounded)
			output[segmentIndex][shardIndex] = make([]byte, shardSize)

			if shardIndex < dataShards {

				// Assign the original data to the shard. NOTE: Only the first 6 GF points are used.
				for GFPointIndex := 0; GFPointIndex < GFPointsPerShard; GFPointIndex++ {

					// Please note that the data are stored in the buffer vertically, from top to bottom and left to right.
					leftIndex := segmentIndex*dataSegmentSize + GFPointIndex*dataShards*2 + shardIndex*2
					rightIndex := leftIndex + 1

					if leftIndex < len(original) {
						buffer[segmentIndex][shardIndex][GFPointIndex] = original[leftIndex]
					}
					if rightIndex < len(original) {
						buffer[segmentIndex][shardIndex][GFPointIndex+32] = original[rightIndex]
					}
				}
			}
		}

		// Calculate the parity shards for the i-th segment
		err = encoder.Encode(buffer[segmentIndex])
		if err != nil {
			return nil, err
		}

		// Copy the buffer to the output
		for j := 0; j < (dataShards + parityShards); j++ {
			for GFPointIndex := 0; GFPointIndex < GFPointsPerShard; GFPointIndex++ {
				output[segmentIndex][j][GFPointIndex*2] = buffer[segmentIndex][j][GFPointIndex]
				output[segmentIndex][j][GFPointIndex*2+1] = buffer[segmentIndex][j][GFPointIndex+32]
			}
		}
	}

	return output, nil
}

func Decode(encodedData [][][]byte) ([]byte, error) {

	// Calculate the shardSize
	shardSize := GFPointsPerShard * 2 // 1 GF point = 2 bytes
	dataSegmentSize := shardSize * K
	shardSizeRounded := 64 * ((shardSize + 64 - 1) / 64) // round up to 64 bytes. See: 	shardSizeRounded := 64 * ((shardSize + 64 - 1) / 64) // round up to 64 bytes. See https://github.com/klauspost/reedsolomon?tab=readme-ov-file#leopard-compatible-gf16
	numSegments := len(encodedData)

	// Calculate the dataShards (original) and parityShards (redundant) arguments for the RS function
	dataShards := K
	parityShards := (N - K) // N = K + parityShards

	// Initialize the RS decoder
	decoder, err := reedsolomon.New(dataShards, parityShards, reedsolomon.WithLeopardGF16(true)) //, reedsolomon.WithAutoGoroutines(shardSizeRounded))
	if err != nil {
		return nil, err
	}

	// Decode the segments
	data := make([]byte, (numSegments * dataSegmentSize))
	for segmentIndex := 0; segmentIndex < len(encodedData); segmentIndex++ {
		decodedSegment := make([][]byte, N)
		for i := 0; i < len(encodedData[segmentIndex]); i++ {
			decodedSegment[i] = make([]byte, shardSizeRounded)
			if encodedData[segmentIndex][i] == nil {
				decodedSegment[i] = nil
			} else {
				for GFPointIndex := 0; GFPointIndex < GFPointsPerShard; GFPointIndex++ {
					decodedSegment[i][GFPointIndex] = encodedData[segmentIndex][i][GFPointIndex*2]
					decodedSegment[i][GFPointIndex+32] = encodedData[segmentIndex][i][GFPointIndex*2+1]
				}
			}
		}

		// Decode the segment
		err = decoder.ReconstructData(decodedSegment)
		if err != nil {
			return nil, err
		}

		// Copy the decoded data to the output
		for shardIndex := 0; shardIndex < dataShards; shardIndex++ {
			for GFPointIndex := 0; GFPointIndex < GFPointsPerShard; GFPointIndex++ {

				// Please note that the data are stored in the buffer vertically, from top to bottom and left to right.
				leftIndex := segmentIndex*dataSegmentSize + GFPointIndex*dataShards*2 + shardIndex*2
				rightIndex := leftIndex + 1

				data[leftIndex] = decodedSegment[shardIndex][GFPointIndex]
				data[rightIndex] = decodedSegment[shardIndex][GFPointIndex+32]
			}
		}
	}

	return data, nil
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
	err = os.WriteFile(filePath, jsonData, 0644)
	if err != nil {
		return err
	}

	return nil
}

func EncodeTiny(original []byte) ([][]byte, error) {
	if len(original) != 24 {
		return nil, fmt.Errorf("original data length must be 24 bytes")
	}
	originalData := make([][]byte, 12)
	for i := 0; i < 12; i++ {
		originalData[i] = make([]byte, 64)
	}
	for i := 0; i < 4; i++ {
		originalData[i][0] = original[i*2]
		originalData[i][1] = original[i*2+1]
		originalData[i][2] = original[8+i*2]
		originalData[i][3] = original[8+i*2+1]
		originalData[i][4] = original[16+i*2]
		originalData[i][5] = original[16+i*2+1]
	}
	enc, err := reedsolomon.New(4, 8, reedsolomon.WithLeopardGF16(true))
	if err != nil {
		return nil, err
	}

	err = enc.Encode(originalData)
	if err != nil {
		return nil, err
	}

	// 去掉填充部分，轉換為 12x6 的數據
	encodedData := make([][]byte, 12)
	for i := 0; i < 12; i++ {
		encodedData[i] = originalData[i][:6]
	}

	return encodedData, nil
}

func DecodeTiny(encoded [][]byte) ([]byte, error) {
	paddedData := make([][]byte, 12)
	for i := 0; i < 12; i++ {
		paddedData[i] = append(encoded[i], make([]byte, 58)...)
	}

	dec, err := reedsolomon.New(4, 8, reedsolomon.WithLeopardGF16(true))
	if err != nil {
		return nil, err
	}

	err = dec.Reconstruct(paddedData)
	if err != nil {
		return nil, err
	}

	original := make([]byte, 24)
	for i := 0; i < 4; i++ {
		original[i*2] = paddedData[i][0]
		original[i*2+1] = paddedData[i][1]
		original[8+i*2] = paddedData[i][2]
		original[8+i*2+1] = paddedData[i][3]
		original[16+i*2] = paddedData[i][4]
		original[16+i*2+1] = paddedData[i][5]
	}

	return original, nil
}

func print3DByteArray(arr [][][]byte) {
	for i := range arr {
		fmt.Printf("Segment %d:\n", i)
		fmt.Println("----------------")
		for j := range arr[i] {
			for k := range arr[i][j] {
				fmt.Printf("%02x ", arr[i][j][k])
			}
			fmt.Println()
		}
		fmt.Println("----------------")
	}
}
