package statedb

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

type TestReport struct {
	Input     ReportInput `json:"input"`
	PreState  StateReport `json:"pre_state"`
	PostState StateReport `json:"post_state"`
}

type ReportInput struct {
	Guarantee     []types.Guarantee  `json:"guarantees"`
	Slot          uint64             `json:"slot"`
	Entropy       Entropy            `json:"entropy"`
	OffenederMark []types.Ed25519Key `json:"offenders"`
}

type StateReport struct {
	AvailabilityAssignments  AvailabilityAssignments         `json:"avail_assignments"`
	CurrValidators           types.Validators                `json:"curr_validators"`
	PrevValidators           types.Validators                `json:"prev_validators"`
	RecentBlocks             RecentBlocks                    `json:"recent_blocks"`
	AuthorizationsPool       [types.TotalCores][]common.Hash `json:"auth_pools"`
	PriorServiceAccountState map[uint32]types.ServiceAccount `json:"services"`
}

// give a json file and read it into a StateReport struct
func TestReportParsing(t *testing.T) {
	testCases := []struct {
		jsonFile     string
		binFile      string
		expectedType interface{}
	}{
		{"not_sorted_guarantor-1.json", "not_sorted_guarantor-1.bin", &TestReport{}},
		{"reports_with_dependencies-1.json", "reports_with_dependencies-1.bin", &TestReport{}},
	}
	for _, tc := range testCases {
		t.Run(tc.jsonFile, func(t *testing.T) {
			jsonPath := filepath.Join("../jamtestvectors/reports/tiny", tc.jsonFile)
			// binPath := filepath.Join("../jamtestvectors/safrole/tiny", tc.binFile)

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
			// expectedBytes, err := os.ReadFile(binPath)
			// if err != nil {
			// 	t.Fatalf("failed to read binary file: %v", err)
			// }
			// assert.Equal(t, expectedBytes, encodedBytes, "encoded bytes do not match expected bytes")

			// if false {
			// 	decoded, _ := types.Decode(expectedBytes, reflect.TypeOf(tc.expectedType))
			// 	encodedBytes2 := types.Encode(decoded)
			// 	// Compare the encoded bytes with the expected bytes
			// 	assert.Equal(t, expectedBytes, encodedBytes2, "encoded bytes do not match expected bytes")
			// }

			// // Compare the encoded JSON with the original JSON
			// assert.JSONEq(t, string(jsonData), string(encodedJSON), "encoded JSON does not match original JSON")
		})
	}
}
