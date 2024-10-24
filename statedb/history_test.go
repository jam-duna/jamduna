package statedb

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
	"github.com/stretchr/testify/assert"
)

// Input struct definition
type HInput struct {
	HeaderHash      common.Hash `json:"header_hash"`
	ParentStateRoot common.Hash `json:"parent_state_root"`
	// This is "r" in Eq (82), which is derived from (162)
	AccumulateRoot common.Hash `json:"accumulate_root"`

	WorkPackages []common.Hash `json:"work_packages"`
}

// PreState and PostState struct definitions
type HState struct {
	Beta BeefyPool `json:"beta"`
}

type HOutput struct{}

func (H *HOutput) Encode() []byte {
	return []byte{}
}

func (H *HOutput) Decode(data []byte) (interface{}, uint32) {
	return nil, 0
}

// HistoryData struct definition
type HistoryData struct {
	Input     HInput   `json:"input"`
	PreState  HState   `json:"pre_state"`
	Output    *HOutput `json:"output"`
	PostState HState   `json:"post_state"`
}

func TestRecentHistory(t *testing.T) {
	testCases := []struct {
		jsonFile     string
		binFile      string
		expectedType interface{}
	}{
		{"progress_blocks_history-1.json", "progress_blocks_history-1.bin", &HistoryData{}},
		{"progress_blocks_history-2.json", "progress_blocks_history-2.bin", &HistoryData{}},
		{"progress_blocks_history-3.json", "progress_blocks_history-3.bin", &HistoryData{}},
		{"progress_blocks_history-4.json", "progress_blocks_history-4.bin", &HistoryData{}},
	}
	for _, tc := range testCases {
		t.Run(tc.jsonFile, func(t *testing.T) {
			jsonPath := filepath.Join("../jamtestvectors/history/data", tc.jsonFile)
			binPath := filepath.Join("../jamtestvectors/history/data", tc.binFile)

			targetedStructType := reflect.TypeOf(tc.expectedType)

			fmt.Printf("\n\n\nTesting %v\n", targetedStructType)
			// Read and unmarshal JSON file
			jsonData, err := os.ReadFile(jsonPath)
			if err != nil {
				t.Fatalf("failed to read JSON file: %v", err)
			}

			err = json.Unmarshal(jsonData, tc.expectedType)
			if err != nil {
				t.Fatalf("failed to unmarshal JSON data: %v", err)
			}
			fmt.Printf("Unmarshaled %s\n", jsonPath)
			fmt.Printf("Expected: %v\n", tc.expectedType)
			// Encode the struct to bytes
			encodedBytes, err := types.Encode(tc.expectedType)
			if err != nil {
				t.Fatalf("failed to encode data: %v", err)
			}
			fmt.Printf("Encoded: %x\n\n", encodedBytes)

			decodedStruct, _, err := types.Decode(encodedBytes, targetedStructType)
			if err != nil {
				t.Fatalf("failed to decode data: %v", err)
			}
			fmt.Printf("Decoded:  %v\n\n", decodedStruct)

			// Marshal the struct to JSON
			encodedJSON, err := json.MarshalIndent(decodedStruct, "", "  ")
			if err != nil {
				t.Fatalf("failed to marshal JSON data: %v", err)
			}
			fmt.Printf("Encoded JSON:\n%s\n", encodedJSON)

			// output bin file
			// err = os.WriteFile("./output.bin", encodedBytes, 0644)
			// if err != nil {
			// 	t.Fatalf("failed to write binary file: %v", err)
			// }

			// Read the expected bytes from the binary file
			expectedBytes, err := os.ReadFile(binPath)
			if err != nil {
				t.Fatalf("failed to read binary file: %v", err)
			}
			assert.Equal(t, expectedBytes, encodedBytes, "encoded bytes do not match expected bytes")

			if false {
				decoded, _, err := types.Decode(expectedBytes, reflect.TypeOf(tc.expectedType))
				if err != nil {
					t.Fatalf("failed to decode data: %v", err)
				}
				encodedBytes2, err := types.Encode(decoded)
				if err != nil {
					t.Fatalf("failed to encode data: %v", err)
				}
				// Compare the encoded bytes with the expected bytes
				assert.Equal(t, expectedBytes, encodedBytes2, "encoded bytes do not match expected bytes")
			}

			// Compare the encoded JSON with the original JSON
			assert.JSONEq(t, string(jsonData), string(encodedJSON), "encoded JSON does not match original JSON")
		})
	}
}
func TestRecentHistoryWithMMR(t *testing.T) {
	testCases := []struct {
		jsonFile     string
		binFile      string
		expectedType interface{}
	}{
		{"progress_blocks_history-1.json", "progress_blocks_history-1.bin", &HistoryData{}},
		{"progress_blocks_history-2.json", "progress_blocks_history-2.bin", &HistoryData{}},
		// {"progress_blocks_history-3.json", "progress_blocks_history-3.bin", &HistoryData{}},
		// {"progress_blocks_history-4.json", "progress_blocks_history-4.bin", &HistoryData{}},
	}
	mmr := trie.NewMMR(types.Keccak)
	for _, tc := range testCases {
		// for i, tc := range testCases {
		// if i != 1 {
		// 	continue
		// }
		t.Run(tc.jsonFile, func(t *testing.T) {

			jsonPath := filepath.Join("../jamtestvectors/history/data", tc.jsonFile)

			targetedStructType := reflect.TypeOf(tc.expectedType)

			fmt.Printf("\n\n\nTesting %v\n", targetedStructType)
			// Read and unmarshal JSON file
			jsonData, err := os.ReadFile(jsonPath)
			if err != nil {
				t.Fatalf("failed to read JSON file: %v", err)
			}

			var data HistoryData
			err = json.Unmarshal(jsonData, &data)

			mmr.PrintTree()
			fmt.Printf("data.Input.AccumulateRoot[:] %x\n", data.Input.AccumulateRoot[:])
			mmr.Append(data.Input.AccumulateRoot[:])

			mmr.PrintTree()
		})
	}
}
