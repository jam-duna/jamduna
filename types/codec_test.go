package types

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCodec(t *testing.T) {
	testCases := []struct {
		jsonFile     string
		binFile      string
		expectedType interface{}
	}{
		{"assurances_extrinsic.json", "assurances_extrinsic.bin", &[]Assurance{}},
		{"work_package.json", "work_package.bin", &WorkPackage{}},
		{"disputes_extrinsic.json", "disputes_extrinsic.bin", &Dispute{}},
		{"extrinsic.json", "extrinsic.bin", &ExtrinsicData{}},
		{"block.json", "block.bin", &Block{}},
		{"tickets_extrinsic.json", "tickets_extrinsic.bin", &[]Ticket{}},
		{"work_item.json", "work_item.bin", &WorkItem{}},
		{"guarantees_extrinsic.json", "guarantees_extrinsic.bin", &[]Guarantee{}},
		{"header_0.json", "header_0.bin", &BlockHeader{}},
		{"header_1.json", "header_1.bin", &BlockHeader{}},
		{"preimages_extrinsic.json", "preimages_extrinsic.bin", &[]Preimages{}},
		{"refine_context.json", "refine_context.bin", &RefineContext{}},
		{"work_report.json", "work_report.bin", &WorkReport{}},
		{"work_result_0.json", "work_result_0.bin", &WorkResult{}},
		{"work_result_1.json", "work_result_1.bin", &WorkResult{}},
	}
	for _, tc := range testCases {
		t.Run(tc.jsonFile, func(t *testing.T) {
			jsonPath := filepath.Join("../jamtestvectors/codec/data", tc.jsonFile)
			binPath := filepath.Join("../jamtestvectors/codec/data", tc.binFile)

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
			encodedBytes := Encode(tc.expectedType)

			fmt.Printf("Encoded: %x\n\n", encodedBytes)

			decodedStruct, _ := Decode(encodedBytes, targetedStructType)
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
				decoded, _ := Decode(expectedBytes, reflect.TypeOf(tc.expectedType))
				encodedBytes2 := Encode(decoded)
				// Compare the encoded bytes with the expected bytes
				assert.Equal(t, expectedBytes, encodedBytes2, "encoded bytes do not match expected bytes")
			}

			// Compare the encoded JSON with the original JSON
			assert.JSONEq(t, string(jsonData), string(encodedJSON), "encoded JSON does not match original JSON")
		})
	}
}
