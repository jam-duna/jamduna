package erasurecoding

import (
	"errors"
	"fmt"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
	"github.com/klauspost/reedsolomon"
)

var (
	// Configurations for the erasure coding library. See https://pkg.go.dev/github.com/klauspost/reedsolomon#New
	// TODO: Coding rate has been changed to 342:1023 in the latest version of the GP.
	// CodingRate_K                int = 2 // coding rate = 342:1023. See GP, Appendix H for more details.
	// CodingRate_N                int = 6 // coding rate = 342:1023. See GP, Appendix H for more details.
	// numPieces int = 6 // k = 6, shard size = k * 2. See GP, Appendix H.1 for more details.
	GFPointsSize = 2 //  little-endian Y2 (E2)
)

const (
	dataShards   = 2
	parityShards = 4
)

// Encode encodes the original data into numPieces using Reed-Solomon encoding
func EncodeBundle(original []byte, numPieces int) ([][]byte, error) {
	if len(original) == 0 || numPieces < 2 {
		return nil, errors.New("invalid input data or number of pieces")
	}

	encoder, err := reedsolomon.New(dataShards, parityShards, reedsolomon.WithLeopardGF16(true))
	if err != nil {
		return nil, fmt.Errorf("failed to create encoder: %v", err)
	}

	// Split the original data into data shards
	shards, err := encoder.Split(original)
	if err != nil {
		return nil, fmt.Errorf("failed to split data: %v", err)
	}

	// Encode the shards with Reed-Solomon
	err = encoder.Encode(shards)
	if err != nil {
		return nil, fmt.Errorf("failed to encode data: %v", err)
	}

	return shards, nil
}

// Decode reconstructs the original data from the encoded shards
func DecodeBundle(encodedData [][]byte) ([]byte, error) {
	if len(encodedData) != types.TotalValidators {
		return nil, errors.New("encoded data length does not match number of pieces")
	}

	decoder, err := reedsolomon.New(dataShards, parityShards, reedsolomon.WithLeopardGF16(true))
	if err != nil {
		return nil, fmt.Errorf("failed to create decoder: %v", err)
	}

	// Attempt to reconstruct missing shards
	err = decoder.Reconstruct(encodedData)
	if err != nil {
		return nil, fmt.Errorf("failed to reconstruct data: %v", err)
	}
	return common.ConcatenateByteSlices(encodedData), nil
}

func GetCodingRate() (coding_rate_K int, coding_rate_N int) {
	coding_rate_K = types.W_C / 2
	coding_rate_N = types.TotalValidators
	return coding_rate_K, coding_rate_N
}

// numPieces k or C_k is the number of data-parallel pieces, each of size where p ∈ ⟦Y_WC⟧ = unzip (p).
func Encode(original []byte, numPieces int) ([][][]byte, error) {
	// TODO: Using non-inplace operation on buffer, output is a bit memory inefficient.

	// Get coding rate K, N
	CodingRate_K, CodingRate_N := GetCodingRate()

	// Calculate the shardSize
	shardSize := numPieces * GFPointsSize                // 1 GF point = 2 bytes (E2)
	shardSizeRounded := 64 * ((shardSize + 64 - 1) / 64) // round up to 64 bytes. See https://github.com/klauspost/reedsolomon?tab=readme-ov-file#leopard-compatible-gf16
	dataSegmentSize := shardSize * CodingRate_K

	// Calculate the dataShards (original) and parityShards (redundant) arguments for the RS function
	dataShards := CodingRate_K
	parityShards := (CodingRate_N - CodingRate_K) // N = K + parityShards

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
				for GFPointIndex := 0; GFPointIndex < numPieces; GFPointIndex++ {

					// Please note that the data are stored in the buffer vertically, from top to bottom and left to right.
					leftIndex := segmentIndex*dataSegmentSize + GFPointIndex*dataShards*2 + shardIndex*2
					rightIndex := leftIndex + 1

					offset := (GFPointIndex / 32) * 64
					index := GFPointIndex % 32

					if leftIndex < len(original) {
						buffer[segmentIndex][shardIndex][offset+index] = original[leftIndex]
					}
					if rightIndex < len(original) {
						buffer[segmentIndex][shardIndex][offset+index+32] = original[rightIndex]
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
			for GFPointIndex := 0; GFPointIndex < numPieces; GFPointIndex++ {

				offset := (GFPointIndex / 32) * 64
				index := GFPointIndex % 32

				output[segmentIndex][j][GFPointIndex*2] = buffer[segmentIndex][j][offset+index]
				output[segmentIndex][j][GFPointIndex*2+1] = buffer[segmentIndex][j][offset+index+32]
			}
		}
	}

	return output, nil
}

func Decode(encodedData [][][]byte, numPieces int) ([]byte, error) {

	// Get coding rate K, N
	CodingRate_K, CodingRate_N := GetCodingRate()

	// Calculate the shardSize
	shardSize := numPieces * GFPointsSize // 1 GF point = 2 bytes (E2)
	dataSegmentSize := shardSize * CodingRate_K
	shardSizeRounded := 64 * ((shardSize + 64 - 1) / 64) // round up to 64 bytes. See: 	shardSizeRounded := 64 * ((shardSize + 64 - 1) / 64) // round up to 64 bytes. See https://github.com/klauspost/reedsolomon?tab=readme-ov-file#leopard-compatible-gf16
	numSegments := len(encodedData)

	// Calculate the dataShards (original) and parityShards (redundant) arguments for the RS function
	dataShards := CodingRate_K
	parityShards := (CodingRate_N - CodingRate_K) // N = K + parityShards

	// Initialize the RS decoder
	decoder, err := reedsolomon.New(dataShards, parityShards, reedsolomon.WithLeopardGF16(true)) //, reedsolomon.WithAutoGoroutines(shardSizeRounded))
	if err != nil {
		return nil, err
	}

	// Decode the segments
	data := make([]byte, (numSegments * dataSegmentSize))
	for segmentIndex := 0; segmentIndex < len(encodedData); segmentIndex++ {
		decodedSegment := make([][]byte, CodingRate_N)
		for i := 0; i < len(encodedData[segmentIndex]); i++ {
			decodedSegment[i] = make([]byte, shardSizeRounded)
			if encodedData[segmentIndex][i] == nil {
				decodedSegment[i] = nil
			} else {
				for GFPointIndex := 0; GFPointIndex < numPieces; GFPointIndex++ {

					offset := (GFPointIndex / 32) * 64
					index := GFPointIndex % 32

					decodedSegment[i][offset+index] = encodedData[segmentIndex][i][GFPointIndex*2]
					decodedSegment[i][offset+index+32] = encodedData[segmentIndex][i][GFPointIndex*2+1]
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
			for GFPointIndex := 0; GFPointIndex < numPieces; GFPointIndex++ {

				// Please note that the data are stored in the buffer vertically, from top to bottom and left to right.
				leftIndex := segmentIndex*dataSegmentSize + GFPointIndex*dataShards*2 + shardIndex*2
				rightIndex := leftIndex + 1

				offset := (GFPointIndex / 32) * 64
				index := GFPointIndex % 32

				data[leftIndex] = decodedSegment[shardIndex][offset+index]
				data[rightIndex] = decodedSegment[shardIndex][offset+index+32]
			}
		}
	}

	return data, nil
}

func Print3DByteArray(arr [][][]byte) {
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
