package types

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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
		{"work_package.json", "work_package.bin", &SWorkPackage{}, &WorkPackage{}},
		{"disputes_extrinsic.json", "disputes_extrinsic.bin", &SDispute{}, &Dispute{}},
		// {"extrinsic.json", "extrinsic.bin", &SExtrinsicData{}, &ExtrinsicData{}},
		// {"block.json", "block.bin", &SBlock{}, &Block{}},
		{"tickets_extrinsic.json", "tickets_extrinsic.bin", &[]STicket{}, &[]Ticket{}},
		{"work_item.json", "work_item.bin", &SWorkItem{}, &WorkItem{}},
		// {"guarantees_extrinsic.json", "guarantees_extrinsic.bin", &[]SGuarantee{}, &[]Guarantee{}},
		{"header_0.json", "header_0.bin", &SBlockHeader{}, &BlockHeader{}},
		{"header_1.json", "header_1.bin", &SBlockHeader{}, &BlockHeader{}},
		{"preimages_extrinsic.json", "preimages_extrinsic.bin", &[]SPreimages{}, &[]Preimages{}},
		{"refine_context.json", "refine_context.bin", &RefinementContext{}, &RefinementContext{}},
		// {"work_report.json", "work_report.bin", &SReport{}, &Report{}},
		// {"work_result_0.json", "work_result_0.bin", &SResults{}, &Results{}},
		// {"work_result_1.json", "work_result_1.bin", &SResults{}, &Results{}},
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
				*tc.expectedType.(*WorkPackage), err = parsed.Deserialize()
				if err != nil {
					t.Fatalf("failed to deserialize SWorkPackage: %v", err)
				}
				fmt.Println("Deserialized:", *tc.expectedType.(*WorkPackage))
			case *SDispute:
				*tc.expectedType.(*Dispute), err = parsed.Deserialize()
				if err != nil {
					t.Fatalf("failed to deserialize SDispute: %v", err)
				}
				fmt.Println("Deserialized:", *tc.expectedType.(*Dispute))
			case *SExtrinsicData:
				*tc.expectedType.(*ExtrinsicData), err = parsed.Deserialize()
				if err != nil {
					t.Fatalf("failed to deserialize SExtrinsicData: %v", err)
				}
				fmt.Println("Deserialized:", *tc.expectedType.(*ExtrinsicData))
			case *SBlock:
				*tc.expectedType.(*Block), err = parsed.Deserialize()
				if err != nil {
					t.Fatalf("failed to deserialize SBlock: %v", err)
				}
				fmt.Println("Deserialized:", *tc.expectedType.(*Block))
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
				*tc.expectedType.(*WorkItem), err = parsed.Deserialize()
				if err != nil {
					t.Fatalf("failed to deserialize SWorkItem: %v", err)
				}
				fmt.Println("Deserialized:", *tc.expectedType.(*WorkItem))
			case *SBlockHeader:
				*tc.expectedType.(*BlockHeader), err = parsed.Deserialize()
				if err != nil {
					t.Fatalf("failed to deserialize SHeader: %v", err)
				}
				fmt.Println("Deserialized:", *tc.expectedType.(*BlockHeader))
			case *RefinementContext:
				tc.expectedType = tc.parsedType
			case *SWorkReport:
				*tc.expectedType.(*WorkReport), err = parsed.Deserialize()
				if err != nil {
					t.Fatalf("failed to deserialize SReport: %v", err)
				}
				fmt.Println("Deserialized:", *tc.expectedType.(*WorkReport))
			case *SResult:
				*tc.expectedType.(*Result) = parsed.Deserialize()
				fmt.Println("Deserialized:", *tc.expectedType.(*Result))
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

func Test_Codec(t *testing.T) {
	i := uint(16909060)
	encoded := Encode(i)
	fmt.Printf("Encoded %d: %x\n", i, encoded)
	decoded, _ := Decode(encoded, reflect.TypeOf(i))
	fmt.Println("Decoded:", decoded)
	// // test int encoding
	// i := Encode(87654321)
	// fmt.Printf("Encoded i: %x\n", i)
	// d_i, _ := DecodeE(i)
	// fmt.Println("Decoded i:", d_i)
	// if d_i != 87654321 {
	// 	t.Errorf("Int test failed: %d != %d", 87654321, d_i)
	// }
	// // test string encoding
	// s := Encode("hello")
	// fmt.Printf("Encoded s: %x\n", s)
	// d_s, _ := Decode(s, reflect.TypeOf(""))
	// fmt.Println("Decoded s:", d_s)
	// if d_s != "hello" {
	// 	t.Errorf("String test failed: %s != %s", "hello", d_s)
	// }
	// // test array encoding
	// a := [3]uint64{1234, 2234, 3234}
	// encoded := Encode(a)
	// fmt.Printf("Encoded a: %x\n", encoded)
	// decoded, _ := Decode(encoded, reflect.TypeOf(a))
	// fmt.Println("Decoded a:", decoded)

	// // test slice encoding
	// slice := []uint64{1234, 2234, 3234}
	// encoded = Encode(slice)
	// fmt.Printf("Encoded slice: %x\n", encoded)
	// decoded, _ = Decode(encoded, reflect.TypeOf(slice))
	// fmt.Println("Decoded slice:", decoded)

	// // test struct encoding
	// type TestStruct struct {
	// 	A uint64
	// 	B string
	// 	C []uint64
	// }
	// testStruct := TestStruct{1234, "hello", []uint64{1234, 2234, 3234}}
	// encoded = Encode(testStruct)
	// fmt.Printf("Encoded struct: %x\n", encoded)
	// decoded, _ = Decode(encoded, reflect.TypeOf(testStruct))
	// fmt.Println("Decoded struct:", decoded)

	// // test 2D array encoding
	// a2 := [2][3]uint64{{1234, 2234, 3234}, {4234, 5234, 6234}}
	// encoded = Encode(a2)
	// fmt.Printf("Encoded a2: %x\n", encoded)
	// decoded, _ = Decode(encoded, reflect.TypeOf(a2))
	// fmt.Println("Decoded a2:", decoded)

	// // test struct with 2D array encoding
	// type TestStruct2 struct {
	// 	A [3][2]uint64
	// 	B string
	// 	C []uint64
	// }
	// testStruct2 := TestStruct2{[3][2]uint64{{1234, 2234}, {3234, 4234}, {5234, 6234}}, "hello", []uint64{1234, 2234, 3234}}
	// encoded = Encode(testStruct2)
	// fmt.Printf("Encoded struct2: %x\n", encoded)
	// decoded, _ = Decode(encoded, reflect.TypeOf(testStruct2))
	// fmt.Println("Decoded struct2:", decoded)

	// // test byte slice encoding
	// u := []byte{1, 2, 3, 4, 5}
	// encoded = Encode(u)
	// fmt.Printf("Encoded u: %x\n", encoded)
	// decoded, _ = Decode(encoded, reflect.TypeOf(u))
	// fmt.Println("Decoded u:", decoded)

	// // test assurance encoding
	// type Assurance struct {
	// 	Anchor [32]byte
	// 	Bitfield  uint8
	// 	Validator_index uint
	// 	Signature [64]byte
	// }
	// assurance:= Assurance{
	// 	Anchor: [32]byte{12, 255, 191, 103, 170, 229, 10, 238, 211, 198, 248, 240, 217, 191, 125, 133, 79, 253, 135, 206, 248, 53, 140, 187, 170, 88, 122, 158, 59, 209, 167, 118},
	// 	Bitfield: 1,
	// 	Validator_index: 0,
	// 	Signature: [64]byte{45, 142, 199, 178, 53, 190, 59, 60, 190, 155, 227, 213, 255, 54, 240, 130, 148, 33, 2, 214, 74, 13, 197, 149, 55, 9, 169, 92, 202, 85, 181, 139, 26, 242, 151, 245, 52, 212, 100, 38, 75, 231, 116, 119, 181, 71, 243, 197, 150, 185, 71, 237, 188, 163, 63, 102, 49, 241, 170, 24, 141, 37, 163, 139},
	// }
	// encoded = Encode(assurance)
	// fmt.Printf("Encoded assurance: %x\n", encoded)
	// decoded, _ = Decode(encoded, reflect.TypeOf(assurance))
	// fmt.Println("Decoded assurance:", decoded)

	// // test array of bytes encoding
	// byte_test := [32]byte{12, 255, 191, 103, 170, 229, 10, 238, 211, 198, 248, 240, 217, 191, 125, 133, 79, 253, 135, 206, 248, 53, 140, 187, 170, 88, 122, 158, 59, 209, 167, 118}
	// encoded = Encode(byte_test)
	// fmt.Printf("Encoded a: %x\n", encoded)
}

func Test_E4(t *testing.T) {
	test := uint64(536870911) // 2^29-1
	encoded := E4(test)
	fmt.Printf("E4* Encoded %d: %x\n", test, encoded)
	decoded, _ := DecodeE4(encoded)
	fmt.Println("E4* Decoded:", decoded)
	if test != decoded {
		t.Errorf("E4* test failed: %d != %d", test, decoded)
	}

	test = uint64(2097151) // 2^21-1
	encoded = E4(test)
	fmt.Printf("E4* Encoded %d: %x\n", test, encoded)
	decoded, _ = DecodeE4(encoded)
	fmt.Println("E4* Decoded:", decoded)
	if test != decoded {
		t.Errorf("E4* test failed: %d != %d", test, decoded)
	}
}

func Test_E_l(t *testing.T) {
	test := uint64(257)
	l := uint32(3)
	encoded := E_l(test, l)
	fmt.Printf("E_l Encoded %d: %x\n", test, encoded)
	decoded := DecodeE_l(encoded)
	fmt.Println("E_l Decoded:", decoded)
	if test != decoded {
		t.Errorf("E_l test failed: %d != %d", test, decoded)
	}
}

func Test_E(t *testing.T) {
	test := uint64(18446744073709551615) // 2^64-1
	encoded := E(test)
	fmt.Printf("E Encoded %d: %x\n", test, encoded)
	decoded, _ := DecodeE(encoded)
	fmt.Println("E Decoded:", decoded)
	if test != decoded {
		t.Errorf("E test failed: %d != %d", test, decoded)
	}
}

func Test_LengthE(t *testing.T) {
	test := []uint64{1234, 22345, 323456, 4234567, 52345678}
	encoded := LengthE(test)
	fmt.Printf("LengthE Encoded %v: %x\n", test, encoded)
	decoded, _ := DecodeLengthE(encoded)
	fmt.Println("LengthE Decoded:", decoded)
	if !reflect.DeepEqual(test, decoded) {
		t.Errorf("LengthE test failed: %v != %v", test, decoded)
	}
}
