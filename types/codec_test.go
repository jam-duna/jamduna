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

const (
	debugCodec = false
)

func TestCodec(t *testing.T) {
	testCases := []struct {
		jsonFile     string
		binFile      string
		expectedType interface{}
	}{
		// {"assurances_extrinsic.json", "assurances_extrinsic.bin", []Assurance{}},
		{"block.json", "block.bin", Block{}},
		// {"disputes_extrinsic.json", "disputes_extrinsic.bin", Dispute{}},
		// {"extrinsic.json", "extrinsic.bin", ExtrinsicData{}},
		// {"guarantees_extrinsic.json", "guarantees_extrinsic.bin", []Guarantee{}},
		// {"header_0.json", "header_0.bin", BlockHeader{}},
		// {"header_1.json", "header_1.bin", BlockHeader{}},
		// {"preimages_extrinsic.json", "preimages_extrinsic.bin", []Preimages{}},
		// {"refine_context.json", "refine_context.bin", RefineContext{}},
		// {"tickets_extrinsic.json", "tickets_extrinsic.bin", []Ticket{}},
		// {"work_item.json", "work_item.bin", WorkItem{}},
		// {"work_package.json", "work_package.bin", WorkPackage{}},
		// {"work_report.json", "work_report.bin", WorkReport{}},
		// {"work_result_0.json", "work_result_0.bin", WorkResult{}},
		// {"work_result_1.json", "work_result_1.bin", WorkResult{}},
	}
	for _, tc := range testCases {
		t.Run(tc.jsonFile, func(t *testing.T) {
			jsonPath := filepath.Join("../jamtestvectors/codec/data", tc.jsonFile)
			binPath := filepath.Join("../jamtestvectors/codec/data", tc.binFile)

			// Read Codec
			expectedCodec, err := os.ReadFile(binPath)
			if err != nil {
				t.Fatalf("failed to read codec file: %v", err)
			}
			codecDecodedStruct, _, err := Decode(expectedCodec, reflect.TypeOf(tc.expectedType))
			if err != nil {
				t.Fatalf("failed to decode codec data: %v", err)
			}
			if debugCodec {
				fmt.Printf("\nexpectedCodec: %x\n", expectedCodec)
				fmt.Printf("Recovered Strcuct from codec: %v\n", codecDecodedStruct)
			}
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
			if debugCodec {
				fmt.Printf("Unmarshaled %s\n", jsonPath)
				fmt.Println("type: ", reflect.TypeOf(jsonDecodedStruct))
				fmt.Printf("Recovered Struct from json: %v\n", jsonDecodedStruct)
			}
			// Getting Codec Result
			codec_via_json_source, err := Encode(jsonDecodedStruct)
			if err != nil {
				t.Fatalf("failed to encode codec data: %v", err)
			}
			codec_via_codec_source, err := Encode(codecDecodedStruct)
			if err != nil {
				t.Fatalf("failed to encode codec data: %v", err)
			}
			if debugCodec {
				fmt.Printf("[Codec Testing] json->codec:\n%x\n", codec_via_json_source)
			}
			// Getting JSON Result
			json_via_codec_source, err := json.MarshalIndent(codecDecodedStruct, "", "  ")
			if err != nil {
				t.Fatalf("failed to marshal JSON data: %v", err)
			}
			json_via_json_source, err := json.MarshalIndent(jsonDecodedStruct, "", "  ")
			if err != nil {
				t.Fatalf("failed to marshal JSON data: %v", err)
			}
			if debugCodec {
				fmt.Printf("[JSON Testing] codec->json:\n%s\n", string(json_via_codec_source))
				fmt.Printf("codec_via_codec_source: %x\n", codec_via_codec_source)
			}

			// let's E2E work on every direction

			// Test 0: codec(codecDecodedStruct) = self
			assert.Equal(t, expectedCodec, codec_via_codec_source, "codec -> struct -> codec Failure")

			// Test 1: json(jsonDecodedStrcut) = self
			assert.JSONEq(t, string(expectedJson), string(json_via_codec_source), "json -> struct -> json Failure")

			// Test 2: codec(codecDecodedStruct) = codec(jsonDecodedStrcut)
			assert.Equal(t, codec_via_json_source, codec_via_codec_source, "json -> struct -> codec Failure")

			// Test 3: JSON(codecDecodedStruct) = JSON(jsonDecodedStrcut)
			assert.JSONEq(t, string(json_via_codec_source), string(json_via_json_source), "json -> struct -> codec Failure")

			if reflect.DeepEqual(tc.expectedType, Block{}) {
				var block Block
				err = json.Unmarshal(expectedJson, &block)
				if err != nil {
					fmt.Println("Error unmarshalling JSON:", err)
					return
				}

				assert.Equal(t, block.Header.ExtrinsicHash.String(), block.Extrinsic.Hash().String(), "Block.Extrinsic.Hash() is wrong")
			}

			// Test 4: codecDecodedStruct = jsonDecodedStrcut
			// if reflect.DeepEqual(&codecDecodedStruct, &jsonDecodedStrcut) {
			// 	fmt.Println("The structs are equal")
			// } else {
			// 	t.Fatalf("codecDecodedStruct <> jsonDecodedStrcut mismatch!")
			// }
		})
	}
}

func TestMapMarshal(t *testing.T) {
	h := Hash2Hash{
		common.Hex2Hash("123"): common.Hex2Hash("456"),
		common.Hex2Hash("456"): common.Hex2Hash("789"),
	}
	// test marshal
	str, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if debugCodec {
		fmt.Println(string(str))
	}
	// test unmarshal
	var h2 Hash2Hash
	err = json.Unmarshal(str, &h2)
	if err != nil {
		t.Fatal(err)
	}
	if debugCodec {
		fmt.Println(h2)
	}
}

func TestAccumulateHistoryJson(t *testing.T) {
	example_json := `[
  [
    "0x7c6845fc0cc3f79490528713ca8c4e301be6d99b65dcad8950fdb4b5fd8ecb3e",
    "0xdcd83ca57a97a766fca5ba37987d31bfba80520a111a4d301f58cd79f01ee30d"
  ],
  [
    "0x0e4a8686815f94af46524df5d4ad88b457fdcddd9521975c173387d33435d2c7",
    "0xc7bea4c4d8dbb0f11f48625fd4438941b58278f2863876d328d773794c642375"
  ],
  [
    "0x8e6a12ed8500c0f6b48d125eac7a3e5b7c005b79214688474d084b8999b1dccd",
    "0x9e3cb9fc51842f3ba0273cfad9cb9fae7f35a142eccfdf40178fa1fc611a69a2"
  ],
  [
    "0x22efeab1be79dc0f7ff26832cdb8cd53f866a48423c7ec6420fcc7f3f95ccded",
    "0x2b1cf51be1dca3ffeefa46a9bf611fb204646cbbf32c94303abdfc1e0024d69b"
  ],
  [],
  [],
  [],
  [],
  [],
  [],
  [],
  []
]`
	var c15 [EpochLength]AccumulationHistory
	err := json.Unmarshal([]byte(example_json), &c15)
	if err != nil {
		t.Error(err)
	}
	out, _ := json.Marshal(c15)
	fmt.Printf("C15: %s\n", string(out))
}
