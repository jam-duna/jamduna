package trie

import (
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"testing"
)

// TestVector represents a test case in the JSON file
type TestVector struct {
	Input  map[string]string `json:"input"`
	Output string            `json:"output"`
}

func TestMerkleTree(t *testing.T) {
	// Read the JSON file
	filePath := "../jamtestvectors/trie/trie.json"
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read JSON file: %v", err)
	}

	// Parse the JSON file
	var testVectors []TestVector
	err = json.Unmarshal(data, &testVectors)
	if err != nil {
		t.Fatalf("Failed to parse JSON file: %v", err)
	}

	// Run each test case
	for _, testCase := range testVectors {
		// Create the Merkle Tree input from the test case
		var input [][]byte
		for k, v := range testCase.Input {
			key, _ := hex.DecodeString(k)
			value, _ := hex.DecodeString(v)
			input = append(input, append(key, value...))
		}

		// Create the Merkle Tree
		tree := NewMerkleTree(input)

		// Compute the root hash
		rootHash := tree.Root.Hash

		// Compare the computed root hash with the expected output
		expectedHash, _ := hex.DecodeString(testCase.Output)
		if !compareHashes(rootHash, expectedHash) {
			t.Errorf("Root hash mismatch for input %v: got %s, want %s", testCase.Input, hex.EncodeToString(rootHash), testCase.Output)
		}
	}
}

// compareHashes compares two byte slices for equality
func compareHashes(a, b []byte) bool {
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
