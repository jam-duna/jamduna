package statedb

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/colorfulnotion/jam/types"
	"github.com/stretchr/testify/assert"
)

const (
	debugCodec = false
)

type Input struct {
	Slot        uint32              `json:"slot"`
	AuthorIndex uint16              `json:"author_index"`
	Extrinsic   types.ExtrinsicData `json:"extrinsic"`
}

type state struct {
	Pi          types.TrueStatistics `json:"statistics"`
	Tau         uint32               `json:"slot"`
	Kappa_prime types.Validators     `json:"curr_validators"`
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
			testcase_name := tc.jsonFile
			// Read Codec
			expectedCodec, err := os.ReadFile(binPath)
			_ = expectedCodec
			if err != nil {
				t.Fatalf("failed to read codec file: %v", err)
			}
			// Read JSON file
			expectedJson, err := os.ReadFile(jsonPath)
			if err != nil {
				t.Fatalf("failed to read JSON file: %v", err)
			}

			jsonDecodedStructPtr := reflect.New(reflect.TypeOf(tc.expectedType)).Interface()
			err = json.Unmarshal(expectedJson, jsonDecodedStructPtr)
			if err != nil {
				t.Fatalf("failed to unmarshal JSON data: %v", err)
			}
			jsonDecodedStruct := reflect.ValueOf(jsonDecodedStructPtr).Elem().Interface()

			newJson, err := json.Marshal(jsonDecodedStruct)
			if err != nil {
				t.Fatalf("failed to marshal JSON data: %v", err)
			}

			// Compare json to struct to json back is the same
			// encode the jsonDecodedStruct to json again and compare
			// test_1 := false // make sure the structure reading from json is correct=> json decode truth
			test_2 := false // make sure the structure reading from codec is correct=> encode truth
			test_3 := false // make sure the structure reading from codec is correct=> use json decode truth to make sure

			// jsonEncoded, err := json.Marshal(jsonDecodedStruct)
			// if err != nil {
			// 	t.Fatalf("failed to marshal JSON data: %v", err)
			// }
			// test_1 = assert.JSONEq(t, string(expectedJson), string(jsonEncoded))
			// if !test_1 {
			// 	fmt.Printf("Expected JSON: \n")
			// 	JsonPrint(jsonDecodedStruct)
			// 	t.Fatalf("Case %s: JSON data not equal", testcase_name)
			// }

			// compare the json struct to codec encoded bytes
			codecEncoded, err := types.EncodeDebug(jsonDecodedStruct, expectedCodec)
			if err != nil {
				t.Fatalf("failed to encode JSON data: %v", err)
			}
			test_2 = assert.Equal(t, expectedCodec, codecEncoded)
			if !test_2 {
				t.Errorf("Case %s: Codec data not equal :\n Encoded=\n%x", testcase_name, codecEncoded)
			}
			codecDecodedStruct, _, err := types.Decode(expectedCodec, reflect.TypeOf(tc.expectedType))
			if err != nil {
				t.Errorf("Case %s: failed to decode codec data: %v (encode, decode)", testcase_name, err)
			}
			// use codecDecodedStrust to json to compare with jsonDecodedStruct
			codecDecodedJson, err := json.Marshal(codecDecodedStruct)
			if err != nil {
				t.Fatalf("failed to marshal JSON data: %v", err)
			}
			test_3 = assert.JSONEq(t, string(newJson), string(codecDecodedJson))
			if !test_3 {
				diff := CompareJSON(jsonDecodedStruct, codecDecodedStruct)
				t.Fatalf("Case %s: JSON data not equal (encode, decode)\n%s", testcase_name, diff)
			}
			if test_2 && test_3 {
				fmt.Printf("\033[32m Passed Case %s:\033[0m\n", testcase_name)
			} else {
				fmt.Printf("\033[31mCase %s: Failed\033[0m\n", testcase_name)
				fmt.Printf("Expected JSON: %s\n", string(expectedJson))
			}
		})
	}
}
func JsonPrint(v interface{}) {
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
}
