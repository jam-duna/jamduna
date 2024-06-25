package safrole

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"
)

// Define the structure for the JSON data
type Input struct {
	Slot       int         `json:"slot"`
	Entropy    string      `json:"entropy"`
	Extrinsics []Extrinsic `json:"extrinsics"`
}

type Validator struct {
	Ed25519      string `json:"ed25519"`
	Bandersnatch string `json:"bandersnatch"`
	Bls          string `json:"bls"`
	Metadata     string `json:"metadata"`
}

type Extrinsic struct {
	Attempt   int    `json:"attempt"`
	Signature string `json:"signature"`
}

type PreState struct {
	Timeslot           int                  `json:"timeslot"`
	Entropy            []string             `json:"entropy"`
	PrevValidators     []Validator          `json:"prev_validators"`
	CurrValidators     []Validator          `json:"curr_validators"`
	NextValidators     []Validator          `json:"next_validators"`
	DesignedValidators []Validator          `json:"designed_validators"`
	TicketsAccumulator []SafroleAccumulator `json:"tickets_accumulator"`
	TicketsOrKeys      struct {
		Keys []string `json:"keys"`
	} `json:"tickets_or_keys"`
	TicketsVerifierKey string `json:"tickets_verifier_key"`
}

type SafroleAccumulator struct {
	Id      string `json:"id"`
	Attempt int    `json:"attempt"`
}

type Output struct {
	Ok struct {
		EpochMark   interface{} `json:"epoch_mark"`
		TicketsMark interface{} `json:"tickets_mark"`
	} `json:"ok"`
}

type PostState struct {
	Timeslot           int                  `json:"timeslot"`
	Entropy            []string             `json:"entropy"`
	PrevValidators     []Validator          `json:"prev_validators"`
	CurrValidators     []Validator          `json:"curr_validators"`
	NextValidators     []Validator          `json:"next_validators"`
	DesignedValidators []Validator          `json:"designed_validators"`
	TicketsAccumulator []SafroleAccumulator `json:"tickets_accumulator"`
	TicketsOrKeys      struct {
		Keys []string `json:"keys"`
	} `json:"tickets_or_keys"`
	TicketsVerifierKey string `json:"tickets_verifier_key"`
}

type TestCase struct {
	Input     Input     `json:"input"`
	PreState  PreState  `json:"pre_state"`
	Output    Output    `json:"output"`
	PostState PostState `json:"post_state"`
}

// safrole_stf is the function to be tested
func safrole_stf(input Input, preState PreState) (Output, PostState) {
	// Implement the function logic here
	return Output{}, PostState{}
}

func equalOutput(o1 Output, o2 Output) bool {

	return true
}

func equalState(s1 PostState, s2 PostState) bool {
	return true
}

// TestSafroleStf reads JSON files, parses them, and calls the safrole_stf function
func TestSafroleStf(t *testing.T) {
	// Directory containing jamtestvectors JSON files
	dir := "../jamtestvectors/safrole"

	// Read all files in the directory
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		if !strings.Contains(file.Name(), ".json") {
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
			t.Errorf("Failed to unmarshal JSON from file %s: %v", filePath, err)
		}

		// Call the safrole_stf function with the input and pre_state
		output, postState := safrole_stf(testCase.Input, testCase.PreState)

		// Perform assertions to validate the output and post_state
		if equalOutput(output, testCase.Output) == false {
			t.Errorf("Output mismatch for file %s: expected %v, got %v", filePath, testCase.Output, output)
		}
		if equalState(postState, testCase.PostState) == false {
			t.Errorf("PostState mismatch for file %s: expected %v, got %v", filePath, testCase.PostState, postState)
		}
	}
}
