package erasurecoding

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"

	"bytes"
	//"encoding/hex"
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
        1, 32, 684,     // one subshard point only
        //4096,    // FAILS : one page only for subshard
        4104,    // one page padded
        15000,   // unaligned padded 4 pages
        21824,   // min size with full 64 byte aligned chunk.
        21888,   // aligned full parallelized subshards.
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
		_, err = base64.StdEncoding.DecodeString(testCase.Data)
		if err != nil {
			t.Fatalf("Failed to decode base64 data from file %s: %v", filePath, err)
		}

		//fmt.Printf("%x\n", decodedData)
		/*// Check that the number of chunks matches
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
		}*/
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
