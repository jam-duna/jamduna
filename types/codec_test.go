package types

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/colorfulnotion/jam/common"
	"github.com/stretchr/testify/assert"
)

func TestCodec(t *testing.T) {
	testCases := []struct {
		jsonFile     string
		binFile      string
		expectedType interface{}
	}{
		{"assurances_extrinsic.json", "assurances_extrinsic.bin", []Assurance{}},
		{"work_package.json", "work_package.bin", WorkPackage{}},
		{"disputes_extrinsic.json", "disputes_extrinsic.bin", Dispute{}},
		{"extrinsic.json", "extrinsic.bin", ExtrinsicData{}},
		{"block.json", "block.bin", Block{}},
		{"tickets_extrinsic.json", "tickets_extrinsic.bin", []Ticket{}},
		{"work_item.json", "work_item.bin", WorkItem{}},
		{"guarantees_extrinsic.json", "guarantees_extrinsic.bin", []Guarantee{}},
		{"header_0.json", "header_0.bin", BlockHeader{}},
		{"header_1.json", "header_1.bin", BlockHeader{}},
		{"preimages_extrinsic.json", "preimages_extrinsic.bin", []Preimages{}},
		{"refine_context.json", "refine_context.bin", RefineContext{}},
		{"work_report.json", "work_report.bin", WorkReport{}},
		{"work_result_0.json", "work_result_0.bin", WorkResult{}},
		{"work_result_1.json", "work_result_1.bin", WorkResult{}},
	}
	for _, tc := range testCases {
		t.Run(tc.jsonFile, func(t *testing.T) {
			jsonPath := filepath.Join("../jamtestvectors/codec/data", tc.jsonFile)
			binPath := filepath.Join("../jamtestvectors/codec/data", tc.binFile)

			fmt.Printf("\n\n\nTesting\n")

			// Read Codec
			expectedCodec, err := os.ReadFile(binPath)

			fmt.Printf("\nexpectedCodec: %x\n", expectedCodec)
			if err != nil {
				t.Fatalf("failed to read codec file: %v", err)
			}
			codecDecodedStruct, _, err := Decode(expectedCodec, reflect.TypeOf(tc.expectedType))
			if err != nil {
				t.Fatalf("failed to decode codec data: %v", err)
			}
			//fmt.Printf("Recovered Strcuct from codec: %v\n", codecDecodedStruct)

			// Read JSON file
			expectedJson, err := os.ReadFile(jsonPath)
			if err != nil {
				t.Fatalf("failed to read JSON file: %v", err)
			}

			jsonDecodedStruct := reflect.New(reflect.TypeOf(tc.expectedType)).Interface()

			err = json.Unmarshal(expectedJson, &jsonDecodedStruct)
			if err != nil {
				t.Fatalf("failed to unmarshal JSON data: %v", err)
			}
			fmt.Printf("Unmarshaled %s\n", jsonPath)
			fmt.Println("type: ", reflect.TypeOf(jsonDecodedStruct))
			fmt.Printf("Recovered Strcuct from json: %v\n", jsonDecodedStruct)

			// Getting Codec Result
			codec_via_json_source, err := Encode(jsonDecodedStruct)
			if err != nil {
				t.Fatalf("failed to encode codec data: %v", err)
			}
			codec_via_codec_source, err := Encode(codecDecodedStruct)
			if err != nil {
				t.Fatalf("failed to encode codec data: %v", err)
			}
			fmt.Printf("[Codec Testing] json->codec:\n%x\n", codec_via_json_source)

			// Getting JSON Result
			json_via_codec_source, err := json.MarshalIndent(codecDecodedStruct, "", "  ")
			if err != nil {
				t.Fatalf("failed to marshal JSON data: %v", err)
			}
			json_via_json_source, err := json.MarshalIndent(jsonDecodedStruct, "", "  ")
			if err != nil {
				t.Fatalf("failed to marshal JSON data: %v", err)
			}
			fmt.Printf("[JSON Testing] codec->json:\n%s\n", string(json_via_codec_source))

			fmt.Printf("codec_via_codec_source: %x\n", codec_via_codec_source)

			// let's E2E work on every direction

			// Test 0: codec(codecDecodedStruct) = self
			assert.Equal(t, expectedCodec, codec_via_codec_source, "codec -> struct -> codec Failure")

			// Test 1: json(jsonDecodedStrcut) = self
			assert.JSONEq(t, string(expectedJson), string(json_via_codec_source), "json -> struct -> json Failure")

			// Test 2: codec(codecDecodedStruct) = codec(jsonDecodedStrcut)
			assert.Equal(t, codec_via_json_source, codec_via_codec_source, "json -> struct -> codec Failure")

			// Test 3: JSON(codecDecodedStruct) = JSON(jsonDecodedStrcut)
			assert.JSONEq(t, string(json_via_codec_source), string(json_via_json_source), "json -> struct -> codec Failure")

			// Test 4: codecDecodedStruct = jsonDecodedStrcut
			// if reflect.DeepEqual(&codecDecodedStruct, &jsonDecodedStrcut) {
			// 	fmt.Println("The structs are equal")
			// } else {
			// 	t.Fatalf("codecDecodedStruct <> jsonDecodedStrcut mismatch!")
			// }
		})
	}
}

func TestMapCodec(t *testing.T) {
	hash1 := common.Hash{0x01, 0x02, 0x03}
	hash2 := common.Hash{0x04, 0x05, 0x06}
	hash3 := common.Hash{0x07, 0x08, 0x09}
	a := map[common.Hash]common.Hash{
		hash1: hash2,
		hash2: hash1,
		hash3: hash3,
	}
	fmt.Println("a: ", a)
	encoded, err := Encode(a)
	if err != nil {
		t.Fatalf("failed to encode map: %v", err)
	}
	fmt.Printf("encoded: %x\n", encoded)
	decoded, _, err := Decode(encoded, reflect.TypeOf(a))
	if err != nil {
		t.Fatalf("failed to decode map: %v", err)
	}
	fmt.Println("decoded: ", decoded)
	if !reflect.DeepEqual(a, decoded) {
		t.Fatalf("expected %v, got %v", a, decoded)
	}
}
