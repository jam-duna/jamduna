package statedb

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/colorfulnotion/jam/common"
	"io/ioutil"
	"os"
	"testing"
)

type ShuffleTestCase struct {
	Input   uint32   `json:"input"`
	Entropy string   `json:"entropy"`
	Output  []uint32 `json:"output"`
}

// shuffle_test reads a JSON file of test cases and performs the tests.
func TestShuffle(t *testing.T) {
	// Read the input file
	fn := common.GetFilePath("/jamtestvectors/shuffle/shuffle_tests.json")
	file, err := os.Open(fn)
	if err != nil {
		t.Fatalf("Failed to open testcases file: %v", err)
	}
	defer file.Close()

	data, err := ioutil.ReadAll(file)
	if err != nil {
		t.Fatalf("Failed to read testcases file: %v", err)
	}

	var testCases []ShuffleTestCase
	if err := json.Unmarshal(data, &testCases); err != nil {
		t.Fatalf("Failed to parse testcases JSON: %v", err)
	}

	for _, tc := range testCases {
		// Create the input sequence
		sequence := make([]uint32, tc.Input)
		for i := uint32(0); i < tc.Input; i++ {
			sequence[i] = i
		}

		// Decode the entropy
		entropyBytes, err := hex.DecodeString(tc.Entropy)
		if err != nil {
			t.Errorf("Invalid entropy in test case: %v", err)
			continue
		}
		if len(entropyBytes) != 32 {
			t.Errorf("Entropy must be 32 bytes in test case: %v", tc.Entropy)
			continue
		}

		var entropyHash [32]byte
		copy(entropyHash[:], entropyBytes)

		// Perform the shuffle
		shuffled := ShuffleFromHash(sequence, entropyHash)

		// Compare the result with the expected output
		if len(shuffled) != len(tc.Output) {
			t.Errorf("Test case failed for input %d: expected %v, got %v", tc.Input, tc.Output, shuffled)
			continue
		}

		for i := range shuffled {
			if shuffled[i] != tc.Output[i] {
				t.Errorf("Test case failed for input %d: expected %v, got %v", tc.Input, tc.Output, shuffled)
				break
			}
		}
		fmt.Printf("Passed len=%d\n", len(shuffled))
	}
}
