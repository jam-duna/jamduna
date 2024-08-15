package erasurecoding

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
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

// This function is used to read the test vector (old base 64 from PR4/Cheme) from the JSON file
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

// This function is used to read the test vector (new hex from Gav provided) from the JSON file
func readTestVectorAtom(jsonFilePath string) ([]byte, []byte, error) {
	currentDir, err := os.Getwd()
	if err != nil {
		return nil, nil, err
	}

	fullPath := filepath.Join(currentDir, jsonFilePath)

	jsonData, err := ioutil.ReadFile(fullPath)
	if err != nil {
		return nil, nil, err
	}

	var jsonOutput AtomicShards
	err = json.Unmarshal(jsonData, &jsonOutput)
	if err != nil {
		return nil, nil, err
	}

	data, err := hex.DecodeString(jsonOutput.Data)
	if err != nil {
		return nil, nil, err
	}

	var flattenedSubshards []byte
	for _, segment := range jsonOutput.Segment.Segments {
		for _, subshardBase64 := range segment.SegmentEC {
			subshard, err := hex.DecodeString(subshardBase64)
			if err != nil {
				return nil, nil, err
			}
			subshardPadded := make([]byte, 12)
			copy(subshardPadded, subshard)
			flattenedSubshards = append(flattenedSubshards, subshardPadded...)
		}
	}

	return data, flattenedSubshards, nil
}

func test_encode_decode(t *testing.T, size uint32) error {
	fmt.Printf("\nTesting size %d\n", size)
	// Generate random byte array of the specified size
	original := make([]byte, size)
	_, err := rand.Read(original)
	if err != nil {
		t.Fatalf("Error generating random bytes: %v", err)
	}
	fmt.Printf("Original data %x\n", original)

	// Encode the original data
	encodedOutput, err0 := Encode(original)
	if err0 != nil {
		t.Fatalf("Error in Encode: %v", err0)
	}
	fmt.Printf("Encoded output:\n")
	print3DByteArray(encodedOutput)

	// Pseudo availability of all subshards, randomly "erase" some subshards according to the availableCount value
	availability := make([][]bool, len(encodedOutput))
	availableCount := K // number of available subshards per segment.
	for i := 0; i < len(encodedOutput); i++ {
		availability[i] = make([]bool, N)
		for j := 0; j < N; j++ {
			if j < N-availableCount {
				availability[i][j] = false
			} else {
				availability[i][j] = true
			}
		}

		// Shuffle the available subshards
		rand.Shuffle(N, func(k, l int) {
			availability[i][k], availability[i][l] = availability[i][l], availability[i][k]
		})

		// Erasure simulation
		for j := 0; j < N; j++ {
			if !availability[i][j] {
				encodedOutput[i][j] = nil
			}
		}
	}
	fmt.Printf("Erased output:\n")
	print3DByteArray(encodedOutput)

	decodedOutput, err2 := Decode(encodedOutput)
	if err2 != nil {
		t.Fatalf("Error in Decode: %v", err2)
	}
	fmt.Printf("Decoded output %x\n", decodedOutput)

	// Check if the decoded output matches the original input
	for i := 0; i < len(original); i++ {
		if original[i] != decodedOutput[i] {
			t.Fatalf("Decoded output does not match the original input for size %d", size)
		}
	}
	fmt.Printf("Decoded output matches the original input for size %d\n", size)

	return nil
}

// TestEC tests the Encode and Decode functions (also simulate the availability(erasure) of subshards)
func TestEC(t *testing.T) {
	sizes := []uint32{
		1, 32, 684, // one subshard point only
		4096,   // one page only for subshad
		4104,   // one page padded
		15000,  // unaligned padded 4 pages
		21824,  // min size with full 64 byte aligned chunk.
		21888,  // aligned full parallelized subshards.
		100000, // larger
		200000, // larger 2r
	}

	rand.Seed(time.Now().UnixNano())
	for _, size := range sizes {
		err := test_encode_decode(t, size)
		if err != nil {
			t.Fatalf("ERR %v", err)
		}
	}
}

// This function is used to generate test vectors
func TestGenerateTestVectors(t *testing.T) {
	sizes := []uint32{
		1, 32, 684, // one subshard point only
		4096,   // one page only for subshad
		4104,   // one page padded
		15000,  // unaligned padded 4 pages
		21824,  // min size with full 64 byte aligned chunk.
		21888,  // aligned full parallelized subshards.
		100000, // larger
		200000, // larger 2r
	}

	//fixed random seed for repeatability
	rand.Seed(1000003)
	for _, size := range sizes {
		original := make([]byte, size)
		_, err := rand.Read(original)
		if err != nil {
			t.Fatalf("Error generating random bytes: %v", err)
		}

		encodedOutput, err0 := Encode(original)
		if err0 != nil {
			t.Fatalf("Error in Encode: %v", err0)
		}

		//filename := fmt.Sprintf("package_%d.json", size)
		filename := fmt.Sprintf("./vectors/atomic_testvectors/package_%d", size)
		err = ConvertToJSON(original, encodedOutput, filename)
		if err != nil {
			t.Fatalf("Error in ConvertToJSON: %v", err)
		}
	}
}

// This function is used to test the Encode and Decode functions using the test vectors
func TestTestVectors(t *testing.T) {
	data, encoded, err := readTestVectorAtom("./vectors/old_vector/batch_1")
	if err != nil {
		t.Fatalf("Error reading test vector: %v", err)
	}
	encodeddata := make([][][]byte, 1)
	encodeddata[0] = make([][]byte, 1026)
	for i := 0; i < 1026; i++ {
		encodeddata[0][i] = make([]byte, 64)
		copy(encodeddata[0][i], encoded[i*12:(i+1)*12])
	}
	//decode
	decodedOutput, err2 := Decode(encodeddata)
	if err2 != nil {
		t.Fatalf("Error in Decode: %v", err2)
	}
	if !compareBytes(data, decodedOutput) {
		t.Fatalf("Decoded output does not match the original input")
	}
}

func compareBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}
