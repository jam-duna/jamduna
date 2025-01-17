package erasurecoding

import (
	"bytes"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/colorfulnotion/jam/common"
)

// Helper function to pad data to the nearest multiple of dataShard * shardPieces * 2
func padData(data []byte, dataShard int, shardPieces int) ([]byte, int) {
	originalLength := len(data)
	segmentSize := dataShard * int(shardPieces) * 2
	if segmentSize == 0 {
		panic("segmentSize cannot be zero")
	}
	padLength := segmentSize - (len(data) % segmentSize)
	if len(data)%segmentSize != 0 {
		data = append(data, make([]byte, padLength)...)
	}
	return data, originalLength
}

func TestEncodeDecode(t *testing.T) {
	dataShard, _ := GetCodingRate()

	testCases := []struct {
		name        string
		data        []byte
		shardPieces int
	}{
		{
			name:        "Empty data",
			data:        []byte{},
			shardPieces: 1,
		},
		{
			name:        "Small data less than one shard",
			data:        []byte{1, 2, 3, 4}, // Needs to be even bytes (2 GFPoints)
			shardPieces: 1,
		},
		{
			name:        "Exact one shard",
			data:        make([]byte, dataShard*2), // Each shardPiece is dataShard GFPoints, each GFPoint is 2 bytes
			shardPieces: 1,
		},
		{
			name:        "Multiple shards",
			data:        make([]byte, dataShard*3*2), // 3 shardPieces
			shardPieces: 1,
		},
		{
			name:        "Data spanning multiple shards with remainder",
			data:        make([]byte, dataShard*5*2+100), // 5 shardPieces + 100 bytes
			shardPieces: 1,
		},
		{
			name:        "Maximum shardPieces",
			data:        make([]byte, dataShard*10*2), // 10 shardPieces
			shardPieces: 10,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			paddedData, originalLength := padData(tc.data, dataShard, tc.shardPieces)
			encoded, err := Encode(paddedData, tc.shardPieces)
			if err != nil {
				t.Errorf("Error encoding data: %v", err)
				return
			}
			decoded, err := Decode(encoded, tc.shardPieces)
			if err != nil {
				t.Errorf("Error decoding data: %v", err)
				return
			}

			// Slice decoded data to the original length
			if originalLength > len(decoded) {
				t.Errorf("Decoded data length (%d) is shorter than original data length (%d)", len(decoded), originalLength)
				return
			}
			decoded = decoded[:originalLength]

			if !bytes.Equal(decoded, tc.data) {
				t.Errorf("Decoded data does not match original for case '%s'", tc.name)
			}
		})
	}
}

func TestEncodeDecodeWithPartialShards(t *testing.T) {
	K, N := GetCodingRate()
	data := common.Hex2Bytes("0xeffa2e260ad2206b38ebe7cf0a7fa4892161004271be226b4131f8248560e084085305d4e82cdbd79d1ec7f0f3abfa067c519e8c82dab34a75b436e789511690c30ecb3c6dc8ea09")

	originalLength := len(data)

	encodedSegments, err := Encode(data, K)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	if len(encodedSegments) != 1 {
		t.Fatalf("Expected 1 encoded segment, got %d", len(encodedSegments))
	}
	segment := encodedSegments[0]

	if len(segment) != N {
		t.Fatalf("Expected %d shards in the encoded segment, got %d", N, len(segment))
	}

	rand.Seed(time.Now().UnixNano())
	shardIndices := rand.Perm(N)[:K]
	selectedShards := make([][][]byte, 1)
	shards := make([][]byte, N)
	for _, idx := range shardIndices {
		shards[idx] = segment[idx]
	}
	selectedShards[0] = shards

	decodedData, err := Decode(selectedShards, K)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if originalLength > len(decodedData) {
		t.Fatalf("Decoded data length (%d) is shorter than original data length (%d)", len(decodedData), originalLength)
	}
	decodedData = decodedData[:originalLength]

	if !bytes.Equal(decodedData, data) {
		t.Errorf("Decoded data does not match original.\nDecoded: %v\nOriginal: %v", decodedData, data)
	}
}

func TestEncodeDecodeData(t *testing.T) {
	dataShard, _ := GetCodingRate()
	data := common.Hex2Bytes("0x04201b2ae248e161634d4a5a0185996efe8e2086e57de000000000000000000000000000000")
	shardPieces := int(6)
	paddedData, originalLength := padData(data, dataShard, shardPieces)
	encoded, err := Encode(paddedData, shardPieces)
	if err != nil {
		t.Errorf("Error encoding data: %v", err)
		return
	}
	decoded, err := Decode(encoded, shardPieces)
	if err != nil {
		t.Errorf("Error decoding data: %v", err)
		return
	}
	decoded = decoded[:originalLength]
	if !bytes.Equal(decoded, data) {
		t.Errorf("Decoded data does not match original")
	}
}

func TestEncodeDecodeEmptyData(t *testing.T) {
	data := []byte{}
	shardPieces := int(1)
	encoded, err := Encode(data, shardPieces)
	if err != nil {
		t.Errorf("Error encoding empty data: %v", err)
		return
	}
	if len(encoded) != 0 {
		t.Errorf("Expected encoded length 0, got %d", len(encoded))
	}
	decoded, err := Decode(encoded, shardPieces)
	if err != nil {
		t.Errorf("Error decoding empty data: %v", err)
		return
	}
	if len(decoded) != 0 {
		t.Errorf("Expected decoded data length 0, got %d", len(decoded))
	}
}

func TestEncodeDecodeSingleShard(t *testing.T) {
	dataShard, _ := GetCodingRate()
	shardPieces := int(1)
	data := make([]byte, dataShard*2)
	for i := 0; i < dataShard*2; i++ {
		data[i] = byte(i % 256)
	}
	paddedData, originalLength := padData(data, dataShard, shardPieces)
	encoded, err := Encode(paddedData, shardPieces)
	if err != nil {
		t.Errorf("Error encoding single shard data: %v", err)
		return
	}
	if len(encoded) != 1 {
		t.Errorf("Expected 1 encoded segment, got %d", len(encoded))
	}
	for _, shard := range encoded[0] {
		if len(shard) != int(shardPieces)*2 {
			t.Errorf("Expected shard length %d, got %d", shardPieces*2, len(shard))
		}
	}
	decoded, err := Decode(encoded, shardPieces)
	if err != nil {
		t.Errorf("Error decoding single shard data: %v", err)
		return
	}
	if originalLength > len(decoded) {
		t.Errorf("Decoded data length (%d) is shorter than original data length (%d)", len(decoded), originalLength)
		return
	}
	decoded = decoded[:originalLength]
	if !bytes.Equal(decoded, data) {
		t.Errorf("Decoded data does not match original for single shard")
	}
}

func TestEncodeDecodeMultipleShards(t *testing.T) {
	dataShard, _ := GetCodingRate()
	shardPieces := int(6)
	data := make([]byte, dataShard*3) // 3 shardPieces
	for i := 0; i < len(data); i++ {
		data[i] = byte(i % 256)
	}
	paddedData, originalLength := padData(data, dataShard, shardPieces)
	encoded, err := Encode(paddedData, shardPieces)
	if err != nil {
		t.Errorf("Error encoding multiple shards data: %v", err)
		return
	}
	expectedSegments := len(paddedData) / (dataShard * int(shardPieces) * 2)
	if len(encoded) != expectedSegments {
		t.Errorf("Expected %d encoded segments, got %d", expectedSegments, len(encoded))
	}
	for _, segment := range encoded {
		_, TotalValidators := GetCodingRate()
		if len(segment) != TotalValidators {
			t.Errorf("Expected %d shards per segment, got %d", TotalValidators, len(segment))
		}
		for _, shard := range segment {
			if len(shard) != int(shardPieces)*2 {
				t.Errorf("Expected shard length %d, got %d", shardPieces*2, len(shard))
			}
		}
	}
	decoded, err := Decode(encoded, shardPieces)
	if err != nil {
		t.Errorf("Error decoding multiple shards data: %v", err)
		return
	}
	if originalLength > len(decoded) {
		t.Errorf("Decoded data length (%d) is shorter than original data length (%d)", len(decoded), originalLength)
		return
	}
	decoded = decoded[:originalLength]
	if !bytes.Equal(decoded, data) {
		fmt.Printf("decoded = %v, data = %v\n", decoded, data)
		t.Errorf("Decoded data does not match original for multiple shards")
	}
}

func TestEncodeDecodeWithDifferentShardPieces(t *testing.T) {
	dataShard, _ := GetCodingRate()
	shardPieces := int(10)
	data := make([]byte, dataShard*10*2) // 10 shardPieces
	for i := 0; i < len(data); i++ {
		data[i] = byte((i * 3) % 256)
	}
	paddedData, originalLength := padData(data, dataShard, shardPieces)
	encoded, err := Encode(paddedData, shardPieces)
	if err != nil {
		t.Errorf("Error encoding with different shardPieces: %v", err)
		return
	}
	expectedSegments := len(paddedData) / (dataShard * int(shardPieces) * 2)
	if len(encoded) != expectedSegments {
		t.Errorf("Expected %d encoded segments, got %d", expectedSegments, len(encoded))
	}
	for _, segment := range encoded {
		_, TotalValidators := GetCodingRate()
		if len(segment) != TotalValidators {
			t.Errorf("Expected %d shards per segment, got %d", TotalValidators, len(segment))
		}
		for _, shard := range segment {
			if len(shard) != int(shardPieces)*2 {
				t.Errorf("Expected shard length %d, got %d", shardPieces*2, len(shard))
			}
		}
	}
	decoded, err := Decode(encoded, shardPieces)
	if err != nil {
		t.Errorf("Error decoding with different shardPieces: %v", err)
		return
	}
	if originalLength > len(decoded) {
		t.Errorf("Decoded data length (%d) is shorter than original data length (%d)", len(decoded), originalLength)
		return
	}
	decoded = decoded[:originalLength]
	if !bytes.Equal(decoded, data) {
		t.Errorf("Decoded data does not match original for different shardPieces")
	}
}

func TestEncodeDecodeLargeData(t *testing.T) {
	dataShard, _ := GetCodingRate()
	shardPieces := int(1)
	data := make([]byte, dataShard*10*2) // 10 shardPieces
	for i := 0; i < len(data); i++ {
		data[i] = byte((i * 7) % 256)
	}
	paddedData, originalLength := padData(data, dataShard, shardPieces)
	encoded, err := Encode(paddedData, shardPieces)
	if err != nil {
		t.Errorf("Error encoding large data: %v", err)
		return
	}
	expectedSegments := len(paddedData) / (dataShard * int(shardPieces) * 2)
	if len(encoded) != expectedSegments {
		t.Errorf("Expected %d encoded segments, got %d", expectedSegments, len(encoded))
	}
	for _, segment := range encoded {
		_, TotalValidators := GetCodingRate()
		if len(segment) != TotalValidators {
			t.Errorf("Expected %d shards per segment, got %d", TotalValidators, len(segment))
		}
		for _, shard := range segment {
			if len(shard) != int(shardPieces)*2 {
				t.Errorf("Expected shard length %d, got %d", shardPieces*2, len(shard))
			}
		}
	}
	decoded, err := Decode(encoded, shardPieces)
	if err != nil {
		t.Errorf("Error decoding large data: %v", err)
		return
	}
	if originalLength > len(decoded) {
		t.Errorf("Decoded data length (%d) is shorter than original data length (%d)", len(decoded), originalLength)
		return
	}
	decoded = decoded[:originalLength]
	if !bytes.Equal(decoded, data) {
		t.Errorf("Decoded data does not match original for large data")
	}
}

func TestDecodeInvalidShards(t *testing.T) {
	dataShard, _ := GetCodingRate()
	shardPieces := int(1)
	data := make([]byte, dataShard*2)
	for i := 0; i < dataShard*2; i++ {
		data[i] = byte(i % 256)
	}
	paddedData, _ := padData(data, dataShard, shardPieces)
	encoded, err := Encode(paddedData, shardPieces)
	if err != nil {
		t.Errorf("Error encoding data for TestDecodeInvalidShards: %v", err)
		return
	}
	if len(encoded) > 0 && len(encoded[0]) > 0 {
		encoded[0][0][0] = byte(4)
		encoded[0][0][1] = byte(2)
	}

	decode, err := Decode(encoded, shardPieces)
	if err != nil {
		t.Errorf("Error decoding data for TestDecodeInvalidShards: %v", err)
		return
	}
	if bytes.Equal(decode, data) {
		t.Errorf("Decoded data does not match original for invalid shards")
	}
}
