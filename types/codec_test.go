package types

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"reflect"
	"testing"

	"github.com/colorfulnotion/jam/common"
	"github.com/stretchr/testify/assert"
)

func TestCodecTiny(t *testing.T) {
	testCodec(t, "tiny")
}

func TestCodecFull(t *testing.T) {
	//TODO: need to support switching config again
	testCodec(t, "full")
}

func testCodec(t *testing.T, neworkType string) {
	testCases := []struct {
		jsonFile     string
		binFile      string
		expectedType interface{}
	}{
		{"assurances_extrinsic.json", "assurances_extrinsic.bin", []Assurance{}},
		{"block.json", "block.bin", Block{}},
		{"disputes_extrinsic.json", "disputes_extrinsic.bin", Dispute{}},
		{"extrinsic.json", "extrinsic.bin", ExtrinsicData{}},
		{"guarantees_extrinsic.json", "guarantees_extrinsic.bin", []Guarantee{}},
		{"header_0.json", "header_0.bin", BlockHeader{}},
		{"header_1.json", "header_1.bin", BlockHeader{}},
		{"preimages_extrinsic.json", "preimages_extrinsic.bin", []Preimages{}},
		{"refine_context.json", "refine_context.bin", RefineContext{}},
		{"tickets_extrinsic.json", "tickets_extrinsic.bin", []Ticket{}},
		{"work_item.json", "work_item.bin", WorkItem{}},
		{"work_package.json", "work_package.bin", WorkPackage{}},
		{"work_report.json", "work_report.bin", WorkReport{}},
		{"work_result_0.json", "work_result_0.bin", WorkDigest{}},
		{"work_result_1.json", "work_result_1.bin", WorkDigest{}},
	}

	for _, tc := range testCases {
		t.Run(tc.jsonFile, func(t *testing.T) {
			jsonPath := path.Join(common.GetJAMTestVectorPath("codec"), neworkType, tc.jsonFile)
			binPath := path.Join(common.GetJAMTestVectorPath("codec"), neworkType, tc.binFile)
			testcase_name := tc.jsonFile
			// Read Codec
			expectedCodec, err := os.ReadFile(binPath)
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

			// Compare json to struct to json back is the same
			// encode the jsonDecodedStruct to json again and compare
			test_1 := false // make sure the structure reading from json is correct=> json decode truth
			test_2 := false // make sure the structure reading from codec is correct=> encode truth
			test_3 := false // make sure the structure reading from codec is correct=> use json decode truth to make sure

			jsonEncoded, err := json.Marshal(jsonDecodedStruct)
			if err != nil {
				t.Fatalf("failed to marshal JSON data: %v", err)
			}
			test_1 = assert.JSONEq(t, string(expectedJson), string(jsonEncoded))
			if !test_1 {
				t.Errorf("Case %s: JSON data not equal", testcase_name)
			}
			// compare the json struct to codec encoded bytes
			codecEncoded, err := Encode(jsonDecodedStruct)
			if err != nil {
				t.Fatalf("failed to encode JSON data: %v", err)
			}
			test_2 = assert.Equal(t, expectedCodec, codecEncoded)
			if !test_2 {
				t.Errorf("Case %s: Codec data not equal :\n Encoded(len=%d):\n%x \n Expected(len=%d):\n%x", testcase_name, len(codecEncoded), codecEncoded, len(expectedCodec), expectedCodec)
			}
			codecDecodedStruct, _, err := Decode(expectedCodec, reflect.TypeOf(tc.expectedType))
			if err != nil {
				t.Errorf("Case %s: failed to decode codec data: %v (encode, decode)", testcase_name, err)
			}
			// use codecDecodedStrust to json to compare with jsonDecodedStruct
			codecDecodedJson, err := json.Marshal(codecDecodedStruct)
			if err != nil {
				t.Fatalf("failed to marshal JSON data: %v", err)
			}
			test_3 = assert.JSONEq(t, string(expectedJson), string(codecDecodedJson))
			if !test_3 {
				t.Errorf("Case %s: JSON data not equal (encode, decode)", testcase_name)
			}
			if test_1 && test_2 && test_3 {
				//fmt.Printf("\033[32m Passed Case %s:\033[0m\n", testcase_name)
			} else {
				fmt.Printf("\033[31mCase %s: Failed\033[0m\n", testcase_name)
				fmt.Printf("Expected JSON: %s\n", string(expectedJson))
			}
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
	// test unmarshal
	var h2 Hash2Hash
	err = json.Unmarshal(str, &h2)
	if err != nil {
		t.Fatal(err)
	}
}

func JsonPrint(v interface{}) {
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
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
	_, err = json.Marshal(c15)
	if err != nil {
		t.Fatalf("Failed to decode data: %v", err)
	}
}

func TestBundleDecode(t *testing.T) {
	t.Skip("Skipping bundle decode test")
	dataString := "000000000046cc924abd9e9e28afe3c24125a2c2d93b7c4e69b800892ec2ce0adf0836079700fbd9acc42d6f898111916b9ada2e8770de303ef6a04c86ba9e268d17c9376474e741a39404ebbe3cb6c2d83906f6598d04520c1827c54068a725a2435a89b8431c28d7753d0da2a474006b499e96ffb0ee39817ab95a6ac4c4df44e5ce771a258f64a383392af6247b4e0b32fe4168a6873776ac7b42f55069054575f7ff3e56c58f1e00000100000000c7963c799298376e307fb8f19d133c74c0acf4b8c993605678c59f6f36e43ebb80c100e90be226abaccc638a0addae983f814190ba97367f8abc5e21642a1e48f23d57724100000000000040420f000000000040420f000000000040420f0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000105e5f00000000809698000000000000000000"
	data := common.FromHex(dataString)

	// Decode the data
	var bundle WorkPackageBundle
	_, _, err := Decode(data, reflect.TypeOf(bundle))
	if err != nil {
		t.Fatalf("Failed to decode data: %v", err)
	}
}
func TestServiceBytes(t *testing.T) {

	service := uint32(0)
	serviceBytes := common.FromHex("0x00777c5d69ff8de299a9cd68236c0ec673038177773f61303997c8edb16024ccbb733c04000000000010270000000000001027000000000000ba6c0200000000000000000000000000100000000e000000700000004ea6dc6b")

	sa, err := ServiceAccountFromBytes(service, serviceBytes)
	if err != nil {
		return
	}
	fmt.Printf("Service Account 1: %+v\n", sa.String())

	serviceBytes2 := common.FromHex("0x00777c5d69ff8de299a9cd68236c0ec673038177773f61303997c8edb16024ccbb733c04000000000010270000000000001027000000000000926c0200000000000000000000000000100000000e000000700000004ea6dc6b")

	sa2, err := ServiceAccountFromBytes(service, serviceBytes2)
	if err != nil {
		return
	}
	fmt.Printf("Service Account 2: %+v\n", sa2.String())
}
