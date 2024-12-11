package statedb

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"testing"

	"github.com/colorfulnotion/jam/types"
	"github.com/stretchr/testify/assert"
)

type Input struct {
	Slot        uint32              `json:"slot"`
	AuthorIndex uint16              `json:"author_index"`
	Extrinsic   types.ExtrinsicData `json:"extrinsic"`
	Reporters   []types.Ed25519Key  `json:"reporters"`
}

type state struct {
	Pi          ValidatorStatistics `json:"pi"`
	Tau         uint32              `json:"tau"`
	Kappa_prime types.Validators    `json:"kappa_prime"`
}

type validator_statistics_test struct {
	Input     Input `json:"input"`
	Prestate  state `json:"pre_state"`
	Poststate state `json:"post_state"`
}

func TestCodecStatistics(t *testing.T) {
	testCases := []struct {
		jsonFile     string
		binFile      string
		expectedType interface{}
	}{
		{"stats_with_empty_extrinsic-1.json", "stats_with_empty_extrinsic-1.bin", validator_statistics_test{}},
		{"stats_with_epoch_change-1.json", "stats_with_epoch_change-1.bin", validator_statistics_test{}},
		{"stats_with_some_extrinsic-1.json", "stats_with_some_extrinsic-1.bin", validator_statistics_test{}},
	}
	for _, tc := range testCases {
		t.Run(tc.jsonFile, func(t *testing.T) {
			jsonPath := filepath.Join("../jamtestvectors/statistics/tiny", tc.jsonFile)
			binPath := filepath.Join("../jamtestvectors/statistics/tiny", tc.binFile)

			fmt.Printf("\n\n\nTesting\n")

			// Read Codec
			expectedCodec, err := os.ReadFile(binPath)

			fmt.Printf("\nexpectedCodec: %x\n", expectedCodec)
			if err != nil {
				t.Fatalf("failed to read codec file: %v", err)
			}
			codecDecodedStruct, _, err := types.Decode(expectedCodec, reflect.TypeOf(tc.expectedType))
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
			codec_via_json_source, err := types.Encode(jsonDecodedStruct)
			if err != nil {
				t.Fatalf("failed to encode codec data: %v", err)
			}
			codec_via_codec_source, err := types.Encode(codecDecodedStruct)
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

			// Remove "output": null, using regex
			re := regexp.MustCompile(`(?m)\s*"output"\s*:\s*null,?`)
			cleanedJson := re.ReplaceAllString(string(expectedJson), "")
			expectedJson = []byte(cleanedJson)
			assert.JSONEq(t, string(expectedJson), string(json_via_codec_source), "json -> struct -> json Failure")

			// Test 2: codec(codecDecodedStruct) = codec(jsonDecodedStrcut)
			assert.Equal(t, codec_via_json_source, codec_via_codec_source, "json -> struct -> codec Failure")

			// Test 3: JSON(codecDecodedStruct) = JSON(jsonDecodedStrcut)
			assert.JSONEq(t, string(json_via_codec_source), string(json_via_json_source), "json -> struct -> codec Failure")

		})
	}
}
