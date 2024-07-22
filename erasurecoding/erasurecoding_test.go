package erasurecoding

import (
	"bytes"
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
	// Generate random byte array of the specified size
	//TODO:
	original := make([]byte, size)
	_, err := rand.Read(original)
	if err != nil {
		t.Fatalf("Error generating random bytes: %v", err)
	}

	encodedOutput, err0 := Encode(original)
	if err0 != nil {
		t.Fatalf("Error in Encode: %v", err0)
	}

	// Pseudo availability of all subshards, randomly "erase" some subshards according to the availableCount value
	available := make([]byte, 1026)
	availableCount := 342 // number of available subshards
	for i := 0; i < len(available); i++ {
		if i < availableCount {
			available[i] = 1
		} else {
			available[i] = 0
		}
	}
	rand.Shuffle(len(available), func(i, j int) {
		available[i], available[j] = available[j], available[i]
	})
	fmt.Printf("Number of Available subshards: %d\nAvailability:%x\n", availableCount, available)

	decodedOutput, err2 := Decode(uint32(len(original)), encodedOutput, available)
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

// Use this function to test specific size of data and output as json file
func TestJsonSave(t *testing.T) {

	// Generate random byte array of the specified size

	//fixed random seed for repeatability
	rand.Seed(1000003)

	original := make([]byte, 5000)
	_, err := rand.Read(original)
	if err != nil {
		t.Fatalf("Error generating random bytes: %v", err)
	}

	encodedOutput, err0 := Encode(original)
	if err0 != nil {
		t.Fatalf("Error in Encode: %v", err0)
	}
	fmt.Printf("Encoded output size %d\n", len(encodedOutput))
	encodedArray := GetSubshardsAtomic(encodedOutput, original)
	fmt.Printf("Shape of encodedArray: %d, %d, %d\n", len(encodedArray), len(encodedArray[0]), len(encodedArray[0][0]))
	err = ConvertToJSON(original, encodedArray, "codewords.json")
	if err != nil {
		t.Fatalf("Error in ConvertToJSON: %v", err)
	}

}

// This function is used to test the encode and decode functions with the test vectors provided
// Used to debug the encode and decode functions
func TestErasureCodingDebug(t *testing.T) {
	// Directory containing jamtestvectors JSON files
	dir := "./"
	file := "codewords.json"
	filePath := filepath.Join(dir, file)
	data, subshards, err := readTestVectorAtom(filePath)
	if err != nil {
		t.Fatalf("Failed to read test vector: %v", err)
	}

	fmt.Printf("Data:\n")
	dataHex := hex.EncodeToString(data)
	fmt.Println(dataHex)

	subshardsHex := hex.EncodeToString(subshards)
	for i := 0; i < len(subshardsHex); i += 24 {
		end := i + 24
		if end > len(subshardsHex) {
			end = len(subshardsHex)
		}
		fmt.Println(subshardsHex[i:end])
	}

	encodedOutput, err0 := Encode(data)
	if err0 != nil {
		t.Fatalf("Error in Encode: %v", err0)
	}

	encodedHex := hex.EncodeToString(encodedOutput)
	for i := 0; i < len(encodedHex); i += 24 {
		end := i + 24
		if end > len(encodedHex) {
			end = len(encodedHex)
		}
		fmt.Println(encodedHex[i:end])
	}

	// ConvertToJSON(data, GetSubshards(encodedOutput), "codewords.json")

	// Check if the encoded output matches the expected output
	if equalBytes(encodedOutput, subshards) {
		fmt.Printf("Encoded output matches the expected output for %s\n", file)
	} else {
		t.Fatalf("Encoded output does not match the expected output for %s", file)
	}

	// Pseudo availability of all subshards, i.e. all subshards are available
	available := make([]byte, 1026)
	for i := 0; i < 1026; i++ {
		available[i] = 1
	}

	decodedOutput, err1 := Decode(uint32(len(data)), subshards, available)
	if err1 != nil {
		t.Fatalf("Error in Decode: %v", err1)
	}

	// Check if the decoded output matches the original input
	if equalBytes(data, decodedOutput) {
		fmt.Printf("Decoded output matches the original input for %s\n", file)
	} else {
		t.Fatalf("Decoded output does not match the original input for %s", file)
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

		encodedArray := GetSubshardsAtomic(encodedOutput, original)

		//filename := fmt.Sprintf("package_%d.json", size)
		filename := fmt.Sprintf("./vectors/atomic_testvectors/package_%d", size)
		err = ConvertToJSON(original, encodedArray, filename)
		if err != nil {
			t.Fatalf("Error in ConvertToJSON: %v", err)
		}
	}
}

func TestGenerateTestVectors684(t *testing.T) {
	//fixed random seed for repeatability
	//colorfulnotion jam [99, 111, 108, 114, 102, 117, 110, 116, 105, 106, 97, 109]
	seed := []int64{
		99, 111, 108, 114, 102, 117, 110, 116, 105, 106, 97, 109,
	}
	size := 684
	batch0 := make([]byte, size*6)
	batch1 := make([]byte, size*6)
	for i := 0; i < len(seed); i++ {
		rand.Seed(seed[i])
		original := make([]byte, size)
		_, err := rand.Read(original)
		if err != nil {
			t.Fatalf("Error generating random bytes: %v", err)
		}
		if i < 6 {
			copy(batch0[i*size:(i+1)*size], original)

		} else {
			copy(batch1[(i-6)*size:(i-5)*size], original)
		}

		encodedOutput, err0 := Encode(original)
		if err0 != nil {
			t.Fatalf("Error in Encode: %v", err0)
		}

		encodedArray := GetSubshardsAtomic(encodedOutput, original)

		//filename := fmt.Sprintf("package_%d.json", size)
		filename := fmt.Sprintf("./vectors/testvector_684/vector_%d", i)
		err = ConvertToJSON(original, encodedArray, filename)
		if err != nil {
			t.Fatalf("Error in ConvertToJSON: %v", err)
		}
	}
	encodedOutputBatch0, err0 := Encode(batch0)
	if err0 != nil {
		t.Fatalf("Error in Encode: %v", err0)
	}
	encodedArrayBatch0 := GetSubshardsAtomic(encodedOutputBatch0, batch0)
	err := ConvertToJSON(batch0, encodedArrayBatch0, "./vectors/testvector_684/batch_0")
	if err != nil {
		t.Fatalf("Error in ConvertToJSON: %v", err)
	}

	encodedOutputBatch1, err0 := Encode(batch1)
	if err0 != nil {
		t.Fatalf("Error in Encode: %v", err0)
	}
	encodedArrayBatch1 := GetSubshardsAtomic(encodedOutputBatch1, batch1)
	err = ConvertToJSON(batch1, encodedArrayBatch1, "./vectors/testvector_684/batch_1")
	if err != nil {
		t.Fatalf("Error in ConvertToJSON: %v", err)
	}

}

// Use this function to test if the encoded output matches the expected output from the Test vector we generated
func TestErasureCoding(t *testing.T) {
	// Directory containing jamtestvectors JSON files
	dir := "./vectors/testvector_684"

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
		data, subshards, err := readTestVectorAtom(filePath)
		if err != nil {
			t.Fatalf("Failed to read test vector: %v", err)
		}

		fmt.Printf("Testing %s\n", file.Name())

		encodedOutput, err0 := Encode(data)
		if err0 != nil {
			t.Fatalf("Error in Encode: %v", err0)
		}
		//print out the encoded output length and subshards length and data length
		fmt.Printf("Encoded output size %d\n", len(encodedOutput))
		fmt.Printf("Subshards size %d\n", len(subshards))
		fmt.Printf("Data size %d\n", len(data))

		// Check if the encoded output matches the expected output
		if equalBytes(encodedOutput, subshards) {
			fmt.Printf("Encoded output matches the expected output for %s\n", file.Name())
		} else {
			t.Fatalf("Encoded output does not match the expected output for %s", file.Name())
		}

		// Pseudo availability of all subshards, i.e. all subshards are available
		available := make([]byte, 1026)
		for i := 0; i < 1026; i++ {
			available[i] = 1
		}

		decodedOutput, err1 := Decode(uint32(len(data)), subshards, available)
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

// Helper function to compare two byte slices
func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			fmt.Printf("Mismatch at index %d\n", i)
			return false
		}

	}
	return true
}
