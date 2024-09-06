package types

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	// "reflect"
	"testing"

	// "github.com/colorfulnotion/jam/types"
	"github.com/stretchr/testify/assert"
)

func Test_Json(t *testing.T) {
	testCases := []struct {
		jsonFile     string
		binFile      string
		parsedType   interface{}
		expectedType interface{}
	}{
		// The commented-out sections contain negative numbers, which cause encoding and decoding errors in the test vectors.
		{"assurances_extrinsic.json", "assurances_extrinsic.bin", &[]SAssurance{}, &[]Assurance{}},
		{"work_package.json", "work_package.bin", &SWorkPackage{}, &CWorkPackage{}},
		{"disputes_extrinsic.json", "disputes_extrinsic.bin", &SDispute{}, &Dispute{}},
		{"extrinsic.json", "extrinsic.bin", &SExtrinsicData{}, &CExtrinsicData{}},
		{"block.json", "block.bin", &SBlock{}, &CBlock{}},
		{"tickets_extrinsic.json", "tickets_extrinsic.bin", &[]STicket{}, &[]Ticket{}},
		{"work_item.json", "work_item.bin", &SWorkItem{}, &CWorkItem{}},
		{"guarantees_extrinsic.json", "guarantees_extrinsic.bin", &[]SGuarantee{}, &[]CGuarantee{}},
		{"header_0.json", "header_0.bin", &SBlockHeader{}, &CBlockHeader{}},
		{"header_1.json", "header_1.bin", &SBlockHeader{}, &CBlockHeader{}},
		{"preimages_extrinsic.json", "preimages_extrinsic.bin", &[]SPreimages{}, &[]Preimages{}},
		{"refine_context.json", "refine_context.bin", &RefineContext{}, &RefineContext{}},
		{"work_report.json", "work_report.bin", &SWorkReport{}, &CWorkReport{}},
		{"work_result_0.json", "work_result_0.bin", &SWorkResult{}, &WorkResult{}},
		{"work_result_1.json", "work_result_1.bin", &SWorkResult{}, &WorkResult{}},
	}

	for _, tc := range testCases {
		t.Run(tc.jsonFile, func(t *testing.T) {
			jsonPath := filepath.Join("../jamtestvectors/codec/data", tc.jsonFile)
			binPath := filepath.Join("../jamtestvectors/codec/data", tc.binFile)

			// Read and unmarshal JSON file
			jsonData, err := os.ReadFile(jsonPath)
			if err != nil {
				t.Fatalf("failed to read JSON file: %v", err)
			}

			err = json.Unmarshal(jsonData, tc.parsedType)
			if err != nil {
				t.Fatalf("failed to unmarshal JSON data: %v", err)
			}
			fmt.Printf("Unmarshaled %s\n", jsonPath)

			fmt.Println("Parsed:", tc.parsedType)

			// Deserialize if necessary
			switch parsed := tc.parsedType.(type) {
			case *[]SAssurance:
				assurances := make([]Assurance, 0)
				for _, sAssurance := range *parsed {
					a, err := sAssurance.Deserialize()
					if err != nil {
						t.Fatalf("failed to deserialize SAssurance: %v", err)
					}
					fmt.Println("Deserialized:", a)
					assurances = append(assurances, a)
				}
				tc.expectedType = &assurances
			case *SWorkPackage:
				*tc.expectedType.(*CWorkPackage), err = parsed.Deserialize()
				if err != nil {
					t.Fatalf("failed to deserialize SWorkPackage: %v", err)
				}
				fmt.Println("Deserialized:", *tc.expectedType.(*CWorkPackage))
			case *SDispute:
				*tc.expectedType.(*Dispute), err = parsed.Deserialize()
				if err != nil {
					t.Fatalf("failed to deserialize SDispute: %v", err)
				}
				fmt.Println("Deserialized:", *tc.expectedType.(*Dispute))
			case *SExtrinsicData:
				*tc.expectedType.(*CExtrinsicData), err = parsed.Deserialize()
				if err != nil {
					t.Fatalf("failed to deserialize SExtrinsicData: %v", err)
				}
				fmt.Println("Deserialized:", *tc.expectedType.(*CExtrinsicData))
			case *SBlock:
				*tc.expectedType.(*CBlock), err = parsed.Deserialize()
				if err != nil {
					t.Fatalf("failed to deserialize SBlock: %v", err)
				}
				fmt.Println("Deserialized:", *tc.expectedType.(*CBlock))
			case *[]STicket:
				tickets := make([]Ticket, 0)
				for _, sTicket := range *parsed {
					ticket, err := sTicket.Deserialize()
					if err != nil {
						t.Fatalf("failed to deserialize STicket: %v", err)
					}
					fmt.Println("Deserialized:", ticket)
					tickets = append(tickets, ticket)
				}
				tc.expectedType = &tickets
			case *SWorkItem:
				*tc.expectedType.(*CWorkItem), err = parsed.Deserialize()
				if err != nil {
					t.Fatalf("failed to deserialize SWorkItem: %v", err)
				}
				fmt.Println("Deserialized:", *tc.expectedType.(*CWorkItem))
			case *SBlockHeader:
				*tc.expectedType.(*CBlockHeader), err = parsed.Deserialize()
				if err != nil {
					t.Fatalf("failed to deserialize SHeader: %v", err)
				}
				fmt.Println("Deserialized:", *tc.expectedType.(*CBlockHeader))
			case *RefineContext:
				tc.expectedType = tc.parsedType
			case *SWorkReport:
				*tc.expectedType.(*CWorkReport), err = parsed.Deserialize()
				if err != nil {
					t.Fatalf("failed to deserialize SReport: %v", err)
				}
				fmt.Println("Deserialized:", *tc.expectedType.(*CWorkReport))
			case *SWorkResult:
				*tc.expectedType.(*WorkResult) = parsed.Deserialize()
				fmt.Println("Deserialized:", *tc.expectedType.(*WorkResult))
			case *[]SPreimages:
				preimages := make([]Preimages, 0)
				for _, sPreimages := range *parsed {
					preimage, err := sPreimages.Deserialize()
					if err != nil {
						t.Fatalf("failed to deserialize SPreimages: %v", err)
					}
					fmt.Println("Deserialized:", preimage)
					preimages = append(preimages, preimage)
				}
				tc.expectedType = &preimages
			case *[]SGuarantee:
				guarantees := make([]CGuarantee, 0)
				for _, sGuarantee := range *parsed {
					guarantee, err := sGuarantee.Deserialize()
					if err != nil {
						t.Fatalf("failed to deserialize SGuarantee: %v", err)
					}
					fmt.Println("Deserialized:", guarantee)
					guarantees = append(guarantees, guarantee)
				}
				tc.expectedType = &guarantees
			}

			fmt.Println("Expected:", tc.expectedType)

			// Encode the struct to bytes
			encodedBytes := Encode(tc.expectedType)

			fmt.Printf("Encoded: %x\n\n", encodedBytes)

			decoded, _ := Decode(encodedBytes, reflect.TypeOf(tc.expectedType))
			fmt.Println("Decoded:", decoded)

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
		})
	}
}
