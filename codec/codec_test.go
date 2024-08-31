package codec

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/colorfulnotion/jam/types"
	"github.com/stretchr/testify/assert"
)

func TestCodec(t *testing.T) {
	testCases := []struct {
		jsonFile     string
		binFile      string
		parsedType   interface{}
		expectedType interface{}
	}{
		{"assurances_extrinsic.json", "assurances_extrinsic.bin", &[]types.SAssurance{}, &[]types.Assurance{}},
		{"work_package.json", "work_package.bin", &types.SWorkPackage{}, &types.WorkPackage{}},
		{"disputes_extrinsic.json", "disputes_extrinsic.bin", &types.SDispute{}, &types.Dispute{}},
		{"extrinsic.json", "extrinsic.bin", &types.SExtrinsicData{}, &types.ExtrinsicData{}},
		{"block.json", "block.bin", &types.SBlock{}, &types.Block{}},
		{"tickets_extrinsic.json", "tickets_extrinsic.bin", &[]types.STicket{}, &[]types.Ticket{}},
		{"work_item.json", "work_item.bin", &types.SWorkItem{}, &types.WorkItem{}},
		{"guarantees_extrinsic.json", "guarantees_extrinsic.bin", &[]types.Guarantee{}, &[]types.Guarantee{}},
		{"header_0.json", "header_0.bin", &types.SBlockHeader{}, &types.BlockHeader{}},
		{"header_1.json", "header_1.bin", &types.SBlockHeader{}, &types.BlockHeader{}},
		{"preimages_extrinsic.json", "preimages_extrinsic.bin", &[]types.PreimageLookup{}, &[]types.PreimageLookup{}},
		{"refine_context.json", "refine_context.bin", &types.RefinementContext{}, &types.RefinementContext{}},
		{"work_report.json", "work_report.bin", &types.WorkReport{}, &types.WorkReport{}},
		{"work_result_0.json", "work_result_0.bin", &types.WorkResult{}, &types.WorkResult{}},
		{"work_result_1.json", "work_result_1.bin", &types.WorkResult{}, &types.WorkResult{}},
	}

	for _, tc := range testCases {
		t.Run(tc.jsonFile, func(t *testing.T) {
			jsonPath := filepath.Join("../jamtestvectors/codec/data", tc.jsonFile)
			binPath := filepath.Join("../jamtestvectors/codec/data", tc.binFile)

			// Read and unmarshal JSON file
			jsonData, err := ioutil.ReadFile(jsonPath)
			if err != nil {
				t.Fatalf("failed to read JSON file: %v", err)
			}

			err = json.Unmarshal(jsonData, tc.parsedType)
			if err != nil {
				t.Fatalf("failed to unmarshal JSON data: %v", err)
			}
			fmt.Printf("Unmarshaled %s\n", jsonPath)

			// Deserialize if necessary
			switch parsed := tc.parsedType.(type) {
			case *[]types.SAssurance:
				assurances := make([]types.Assurance, 0)
				for _, sAssurance := range *parsed {
					a, err := sAssurance.Deserialize()
					if err != nil {
						t.Fatalf("failed to deserialize SAssurance: %v", err)
					}
					assurances = append(assurances, a)
				}
				tc.expectedType = &assurances
			case *types.SWorkPackage:
				*tc.expectedType.(*types.WorkPackage), err = parsed.Deserialize()
				if err != nil {
					t.Fatalf("failed to deserialize SWorkPackage: %v", err)
				}
			case *types.SDispute:
				*tc.expectedType.(*types.Dispute), err = parsed.Deserialize()
				if err != nil {
					t.Fatalf("failed to deserialize SDispute: %v", err)
				}
			case *types.SExtrinsicData:
				*tc.expectedType.(*types.ExtrinsicData), err = parsed.Deserialize()
				if err != nil {
					t.Fatalf("failed to deserialize SExtrinsicData: %v", err)
				}
			case *types.SBlock:
				*tc.expectedType.(*types.Block), err = parsed.Deserialize()
				if err != nil {
					t.Fatalf("failed to deserialize SBlock: %v", err)
				}
			case *[]types.STicket:
				tickets := make([]types.Ticket, 0)
				for _, sTicket := range *parsed {
					ticket, err := sTicket.Deserialize()
					if err != nil {
						t.Fatalf("failed to deserialize STicket: %v", err)
					}
					tickets = append(tickets, ticket)
				}
				tc.expectedType = &tickets
			case *types.SWorkItem:
				*tc.expectedType.(*types.WorkItem), err = parsed.Deserialize()
				if err != nil {
					t.Fatalf("failed to deserialize SWorkItem: %v", err)
				}
			case *types.SBlockHeader:
				*tc.expectedType.(*types.BlockHeader), err = parsed.Deserialize()
				if err != nil {
					t.Fatalf("failed to deserialize SBlockHeader: %v", err)
				}
			}

			// Encode the struct to bytes
			encodedBytes, err := Encode(tc.expectedType)
			if err != nil {
				t.Fatalf("failed to encode to bytes: %v", err)
			}
			fmt.Printf("Encoded: %x\n", encodedBytes)
			// Read the expected bytes from the binary file
			expectedBytes, err := ioutil.ReadFile(binPath)
			if err != nil {
				t.Fatalf("failed to read binary file: %v", err)
			}
			assert.Equal(t, expectedBytes, encodedBytes, "encoded bytes do not match expected bytes")

			if false {
				decoded, err := Decode(expectedBytes, tc.expectedType)
				if err != nil {
					t.Fatalf("failed to decode to bytes: %v", err)
				}
				encodedBytes2, err := Encode(decoded)
				if err != nil {
					t.Fatalf("failed to reencode to bytes: %v", err)
				}
				// Compare the encoded bytes with the expected bytes
				assert.Equal(t, expectedBytes, encodedBytes2, "encoded bytes do not match expected bytes")
			}
		})
	}
}
