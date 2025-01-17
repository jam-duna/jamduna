package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/erasurecoding"
	"github.com/colorfulnotion/jam/node"
	"github.com/colorfulnotion/jam/types"
)

// Generate the test vector by generating random data.
func generateTestVector(size int, fix bool, numPieces int) (map[string]interface{}, error) {
	var numpieces int
	if fix {
		size += size % 2     // Make sure numpieces is even
		numpieces = size / 2 // One chunk has N pieces and each piece is 2 bytes
	} else {
		numPieces += numPieces % 2 // Make sure numpieces is even
		numpieces = numPieces
	}

	fmt.Printf("numpieces: %d\n", numpieces)
	/*
	   Erasure coding has 1/3 data shards and generates 2/3 parity shards.
	   So, we need to generate 1/3 data shards.
	*/
	generateSize := size * types.TotalValidators / 3
	b := node.GenerateRandomData(generateSize)
	b = common.PadToMultipleOfN(b, 2)
	chunks, err := erasurecoding.Encode(b, numpieces)
	if err != nil {
		return nil, err
	}

	// Prepare JSON structure
	segments := []map[string]interface{}{}
	for i := 0; i < len(chunks); i += 7 {
		end := i + 7
		if end > len(chunks) {
			end = len(chunks)
		}
		segmentEC := []string{}
		for _, chunk := range chunks[i:end] {
			segmentEC = append(segmentEC, fmt.Sprintf("%x", chunk))
		}
		segments = append(segments, map[string]interface{}{
			"segment_ec": segmentEC,
		})
	}

	result := map[string]interface{}{
		"data": fmt.Sprintf("%x", b),
		"segment": map[string]interface{}{
			"segments": segments,
		},
	}

	return result, nil
}

// Generate the test vector for erasure coding and save it as JSON.
func main() {
	network := "small"

	sizes := []int{1, 129, 1025, 4105, 10241, 66666, 129999, 258000, 520000, 1048577}
	pieceNum := []int{1, 2, 12, 128, 256, 4096, 16384}

	for _, size := range sizes {
		// Handle fixed numPieces
		result, err := generateTestVector(size, true, 0)
		if err != nil {
			fmt.Printf("Error generating test vector for size %d with fixed numPieces: %v\n", size, err)
			continue
		}

		dir := filepath.Join("output", network, "fixed")
		if err := os.MkdirAll(dir, os.ModePerm); err != nil {
			fmt.Printf("Error creating directory: %v\n", err)
			continue
		}

		fileName := fmt.Sprintf("test_vector_%d.json", size)
		filePath := filepath.Join(dir, fileName)
		file, err := os.Create(filePath)
		if err != nil {
			fmt.Printf("Error creating file: %v\n", err)
			continue
		}
		defer file.Close()

		encoder := json.NewEncoder(file)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(result); err != nil {
			fmt.Printf("Error writing JSON to file: %v\n", err)
		}
	}

	for _, size := range sizes {
		for _, numPieces := range pieceNum {
			// Handle varying numPieces
			result, err := generateTestVector(size, false, numPieces)
			if err != nil {
				fmt.Printf("Error generating test vector for size %d and numPieces %d: %v\n", size, numPieces, err)
				continue
			}

			dir := filepath.Join("output", network, "variable")
			if err := os.MkdirAll(dir, os.ModePerm); err != nil {
				fmt.Printf("Error creating directory: %v\n", err)
				continue
			}

			fileName := fmt.Sprintf("test_vector_%d_pieces_%d.json", size, numPieces)
			filePath := filepath.Join(dir, fileName)
			file, err := os.Create(filePath)
			if err != nil {
				fmt.Printf("Error creating file: %v\n", err)
				continue
			}
			defer file.Close()

			encoder := json.NewEncoder(file)
			encoder.SetIndent("", "  ")
			if err := encoder.Encode(result); err != nil {
				fmt.Printf("Error writing JSON to file: %v\n", err)
			}
		}
	}
}
