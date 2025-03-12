package bls

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"testing"
)

// runEncodeDecodeTest runs one encode–decode test with given total shard count V and input data size.
func runEncodeDecodeTest(V, inputSize int, t *testing.T) {
	origCount := V / 3
	if inputSize%origCount != 0 {
		t.Logf("Skipping input size %d for V=%d because inputSize %% (V/3) != 0", inputSize, V)
		return
	}

	shardSize := inputSize / origCount

	// Generate random input data.
	data := make([]byte, inputSize)
	_, err := rand.Read(data)
	if err != nil {
		t.Fatalf("Failed to generate random data: %v", err)
	}

	// Encode the data to get V shards.
	shards, err := Encode(data, V)
	if err != nil {
		t.Fatalf("Encoding failed for V=%d, size=%d: %v", V, inputSize, err)
	}

	// Print all encoded shards (for debugging).
	t.Logf("V=%d, inputSize=%d, shardSize=%d", V, inputSize, shardSize)
	for i, s := range shards {
		t.Logf("Encoded Shard %d: %x", i, s)
	}

	// For decoding, we select exactly origCount shards.
	// For this example, we choose the last origCount shards from the recovery portion.
	indexes := make([]uint32, origCount)
	for i := 0; i < origCount; i++ {
		// Pick indexes from the end (for V=6, this gives [5,4] as in the original example)
		indexes[i] = uint32(V - 1 - i)
	}
	t.Logf("Selected indexes for decoding: %v", indexes)

	// Build the inputs slice for decoding.
	// (Assume that the Decode function expects a slice of shards corresponding to the provided indexes.)
	inputs := make([][]byte, origCount)
	for i, shardIndex := range indexes {
		if int(shardIndex) >= len(shards) {
			t.Fatalf("Shard index %d is out of bounds", shardIndex)
		}
		inputs[i] = shards[shardIndex]
		t.Logf("Input Shard %d (from encoded shard %d): %x", i, shardIndex, inputs[i])
	}

	// Decode the selected shards.
	recovered, err := Decode(inputs, V, indexes, inputSize)
	if err != nil {
		t.Fatalf("Decoding failed for V=%d, size=%d: %v", V, inputSize, err)
	}

	t.Logf("Recovered output: %x", recovered)

	// Verify that the recovered data matches the original.
	if !bytes.Equal(recovered, data) {
		t.Fatalf("Decoded data does not match original for V=%d, size=%d", V, inputSize)
	}
	fmt.Printf("SUCCESS: V = %d and input_len = %d bytes\n", V, len(recovered))
}

func TestEncodeDecode(t *testing.T) {
	// Define various input sizes: 1,
	sizes := []int{32, 684, 4104, 21888, 21824, 15000, 100000, 200000}
	// Define various V values.
	Vs := []int{6}

	for _, V := range Vs {
		for _, size := range sizes {
			t.Run(fmt.Sprintf("V_%d_Size_%d", V, size), func(t *testing.T) {
				runEncodeDecodeTest(V, size, t)
			})
		}
	}
}

func TestSmallEncode(t *testing.T) {
	V := 6
	origCount := V / 3

	// Generate random input data.
	data := []byte{0x14, 0x21, 0x19, 0x9a, 0xdd, 0xac, 0x7c, 0x87, 0x87, 0x3a, 0x00, 0x00}
	inputSize := len(data)

	shardSize := inputSize / origCount
	// Encode the data to get V shards.
	shards, err := Encode(data, V)
	if err != nil {
		t.Fatalf("Encoding failed for V=%d, size=%d: %v", V, inputSize, err)
	}

	// Print all encoded shards (for debugging).
	fmt.Printf("V=%d, inputSize=%d, shardSize=%d %v\n", V, inputSize, shardSize, shards)
	for i, s := range shards {
		fmt.Printf("Encoded Shard %d: %x\n", i, s)
	}
}
