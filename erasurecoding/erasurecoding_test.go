package erasurecoding

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"bytes"
	//"encoding/hex"
	"math/rand"
	"path/filepath"

	"testing"
	"time"
)

// Define the structure for the JSON data
type TestCase struct {
	Data        string `json:"data"`
	WorkPackage struct {
		Chunks     []string `json:"chunks"`
		ChunksRoot string   `json:"chunks_root"`
	} `json:"work_package"`
	Segment struct {
		Segments []struct {
			SegmentEC []string `json:"segment_ec"`
		} `json:"segments"`
	} `json:"segment"`
}

func readTestVector(jsonFilePath string) ([]byte, []byte, error) {
	currentDir, err := os.Getwd()
	if err != nil {
		return nil, nil, err
	}

	fullPath := filepath.Join(currentDir, jsonFilePath)

	jsonData, err := ioutil.ReadFile(fullPath)
	if err != nil {
		return nil, nil, err
	}

	var jsonOutput TestCase
	err = json.Unmarshal(jsonData, &jsonOutput)
	if err != nil {
		return nil, nil, err
	}

	data, err := base64.StdEncoding.DecodeString(jsonOutput.Data)
	if err != nil {
		return nil, nil, err
	}

	var flattenedSubshards []byte
	for _, segment := range jsonOutput.Segment.Segments {
		for _, subshardBase64 := range segment.SegmentEC {
			subshard, err := base64.StdEncoding.DecodeString(subshardBase64)
			if err != nil {
				return nil, nil, err
			}
			flattenedSubshards = append(flattenedSubshards, subshard...)
		}
	}

	return data, flattenedSubshards, nil
}

func test_encode_decode(t *testing.T, size uint32) error {
	// Generate random byte array of the specified size
	original := make([]byte, size)
	_, err := rand.Read(original)
	if err != nil {
		t.Fatalf("Error generating random bytes: %v", err)
	}

	encodedOutput, err0 := Encode(original)
	if err0 != nil {
		t.Fatalf("Error in Encode: %v", err0)
	}

	decodedOutput, err2 := Decode(uint32(len(original)), encodedOutput)
	if err2 != nil {
		t.Fatalf("Error in Decode: %v", err2)
	}
	fmt.Printf("Encoded output size %d\n", len(encodedOutput))

	// Check if the decoded output matches the original input
	if bytes.Equal(original, decodedOutput) {
		fmt.Printf("Decoded output matches the original input for size %d\n", size)
		return nil
	} else {
		t.Fatalf("Decoded output does not match the original input for size %d", size)
	}
	return nil
}

func TestEC(t *testing.T) {
	sizes := []uint32{
		1, 32, 684, // one subshard point only
		4096,   // one page only for subshard
		4104,   // one page padded
		15000,  // unaligned padded 4 pages
		21824,  // min size with full 64 byte aligned chunk.
		21888,  // aligned full parallelized subshards.
		100000, // larger
		200000, // larger 2
	}

	rand.Seed(time.Now().UnixNano())
	for _, size := range sizes {
		err := test_encode_decode(t, size)
		if err != nil {
			t.Fatalf("ERR %v", err)
		}
	}
}

func TestErasureCoding(t *testing.T) {
	// Directory containing jamtestvectors JSON files
	dir := "./tools/test_vectors/vectors/"

	// Read all files in the directory
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		filePath := filepath.Join(dir, file.Name())
		//TODO: skip file package_0.json for now
		// if file.Name() == "package_0" {
		// 	continue
		// }
		data, subshards, err := readTestVector(filePath)
		if err != nil {
			t.Fatalf("Failed to read test vector: %v", err)
		}

		fmt.Printf("Testing %s\n", file.Name())

		encodedOutput, err0 := Encode(data)
		if err0 != nil {
			t.Fatalf("Error in Encode: %v", err0)
		}

		// Check if the encoded output matches the expected output
		if equalBytes(encodedOutput, subshards) {
			fmt.Printf("Encoded output matches the expected output for %s\n", file.Name())
		} else {
			t.Fatalf("Encoded output does not match the expected output for %s", file.Name())
		}

		decodedOutput, err1 := Decode(uint32(len(data)), subshards)
		if err1 != nil {
			t.Fatalf("Error in Decode: %v", err1)
		}

		// Check if the decoded output matches the original input
		if equalBytes(data, decodedOutput) {
			fmt.Printf("Decoded output matches the original input for %s\n", file.Name())
		} else {
			t.Fatalf("Decoded output does not match the original input for %s", file.Name())
		}
	}
}

func TestJsonSave(t *testing.T) {

	// Generate random byte array of the specified size
	original := make([]byte, 4096)
	_, err := rand.Read(original)
	if err != nil {
		t.Fatalf("Error generating random bytes: %v", err)
	}

	encodedOutput, err0 := Encode(original)
	if err0 != nil {
		t.Fatalf("Error in Encode: %v", err0)
	}
	fmt.Printf("Encoded output size %d\n", len(encodedOutput))
	encodedArray := GetSubshards(encodedOutput)
	fmt.Printf("Shape of encodedArray: %d, %d, %d\n", len(encodedArray), len(encodedArray[0]), len(encodedArray[0][0]))
	err = ConvertToJSON(original, encodedArray)
	if err != nil {
		t.Fatalf("Error in ConvertToJSON: %v", err)
	}

}

// Helper function to compare two byte slices
func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
