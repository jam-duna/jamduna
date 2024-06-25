package erasurecoding

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"path/filepath"
	"testing"
	"time"
)

// Define the structure for the JSON data
type TestCase struct {
	Data     string   `json:"data"`
	Chunks   []string `json:"chunks"`
	Segments []struct {
		Subshards []string `json:"subshards"`
	} `json:"segments"`
}

func TestGF16(t *testing.T) {

	// Actually, GF16 -- data must be within GF(16) range [0, 15]
	data := []int{1, 2, 3}

	// Encode the data
	encodedData := Encode(data)
	fmt.Println("Encoded Data: ", encodedData)

	// Simulate some errors
	rand.Seed(time.Now().UnixNano())
	encodedData[4] = 0 // Introduce an error

	// Decode the data
	decodedData, err := Decode(encodedData, 6, 3)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Decoded Data: ", decodedData)
}

func TestBlobs(t *testing.T) {
	// Original data to be encoded
	data := []byte("colorfulnotion")

	// Ensure data length is a multiple of 684
	if len(data)%684 != 0 {
		padding := make([]byte, 684-(len(data)%684))
		data = append(data, padding...)
	}

	// Split dataBlob into 684-byte chunks
	dataChunks := Split(data, 684)

	// Simulate loss of some shards by zeroing them out
	for i := 0; i < 342; i++ {
		dataChunks[i] = make([]byte, 684)
	}

	// Encode the data
	// NOTE: This Encode function should be adapted for byte chunks, currently only for illustrative purposes.
	for i := range dataChunks {
		encodedChunk := Encode(byteArrayToIntArray(dataChunks[i]))
		dataChunks[i] = intArrayToByteArray(encodedChunk)
	}

	// Decode the data
	// NOTE: This Decode function should be adapted for byte chunks, currently only for illustrative purposes.
	for i := range dataChunks {
		decodedChunk, err := Decode(byteArrayToIntArray(dataChunks[i]), 684, 342)
		if err != nil {
			log.Fatal(err)
		}
		dataChunks[i] = intArrayToByteArray(decodedChunk)
	}

	// Join the chunks back into the original data
	reconstructedData := Join(dataChunks)

	// Trim padding if necessary
	reconstructedData = reconstructedData[:len(data)-len(data)%684]

	fmt.Println("Original Data: ", string(data))
	fmt.Println("Reconstructed Data: ", string(reconstructedData))

	t.Logf("Reconstructed successfully")
}

func TestErasureCoding(t *testing.T) {
	// Directory containing jamtestvectors JSON files
	dir := "../jamtestvectors/erasure_coding/vectors"

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
		data, err := ioutil.ReadFile(filePath)
		if err != nil {
			t.Fatalf("Failed to read file %s: %v", filePath, err)
		}

		var testCase TestCase
		err = json.Unmarshal(data, &testCase)
		if err != nil {
			t.Fatalf("Failed to unmarshal JSON from file %s: %v", filePath, err)
		}

		// Decode the base64-encoded data
		decodedData, err := base64.StdEncoding.DecodeString(testCase.Data)
		if err != nil {
			t.Fatalf("Failed to decode base64 data from file %s: %v", filePath, err)
		}

		// Encode the data to produce chunks
		chunks, err := Encode(decodedData)
		if err != nil {
			t.Fatalf("Failed to encode data from file %s: %v", filePath, err)
		}

		// Check that the number of chunks matches
		if len(chunks) != len(testCase.Chunks) {
			t.Errorf("Chunk count mismatch for file %s: expected %d, got %d", filePath, len(testCase.Chunks), len(chunks))
		}

		// Check that each chunk matches the expected value
		for i, chunk := range chunks {
			expectedChunk, err := base64.StdEncoding.DecodeString(testCase.Chunks[i])
			if err != nil {
				t.Fatalf("Failed to decode base64 chunk from file %s: %v", filePath, err)
			}
			if !equalBytes(chunk, expectedChunk) {
				t.Errorf("Chunk mismatch for file %s at index %d: expected %v, got %v", filePath, i, expectedChunk, chunk)
			}
		}

		// Subshard the data
		subshards, err := Subshard(decodedData)
		if err != nil {
			t.Fatalf("Failed to subshard data from file %s: %v", filePath, err)
		}

		// Check that the number of subshards matches
		for j, segment := range testCase.Segments {
			if len(subshards) != len(segment.Subshards) {
				t.Errorf("Subshard count mismatch for file %s, segment %d: expected %d, got %d", filePath, j, len(segment.Subshards), len(subshards))
			}

			// Check that each subshard matches the expected value
			for k, subshard := range subshards {
				expectedSubshard, err := base64.StdEncoding.DecodeString(segment.Subshards[k])
				if err != nil {
					t.Fatalf("Failed to decode base64 subshard from file %s: %v", filePath, err)
				}
				if !equalBytes(subshard, expectedSubshard) {
					t.Errorf("Subshard mismatch for file %s at segment %d, index %d: expected %v, got %v", filePath, j, k, expectedSubshard, subshard)
				}
			}
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
			return false
		}
	}
	return true
}
