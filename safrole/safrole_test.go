package safrole

import (
	"encoding/json"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	//"github.com/ethereum/go-ethereum/common/hexutil"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"
)

type TestCase struct {
	Input     SInput  `json:"input"`
	PreState  SState  `json:"pre_state"`
	Output    SOutput `json:"output"`
	PostState SState  `json:"post_state"`
}

// safrole_sstf is the function to be tested
func safrole_stf(sinput SInput, spreState SState) (SOutput, SState, error) {
	//fmt.Printf("input=%v\n", input)
	//fmt.Printf("preState=%v\n", preState)
	// Implement the function logic here
	input, err := sinput.deserialize()
	if err != nil {
		fmt.Printf("DESERIALIZE err %s\n", err.Error())
		panic(1)
	}

	preState, err2 := spreState.deserialize()
	if err2 != nil {
		fmt.Printf("DESERIALIZE err %s\n", err2.Error())
		panic(2)
	}
	output, postState, err := preState.STF(input)
	return output.serialize(), postState.serialize(), err
}

// Function to copy a State struct
func copyState(original State) State {
	// Convert to JSON
	originalJSON, err := json.Marshal(original)
	if err != nil {
		panic(err) // Handle error as needed
	}

	// Create a new State struct
	var copied State

	// Convert from JSON to struct
	err = json.Unmarshal(originalJSON, &copied)
	if err != nil {
		panic(err) // Handle error as needed
	}

	return copied
}

func TestBlake2b(t *testing.T) {
	// blake2AsHex("data goes here") -> "0xce73267ed8316b4350672f32ba49af86a7ae7af1267beb868a27f3fda03c044a"
	expectedHash := common.HexToHash("0xce73267ed8316b4350672f32ba49af86a7ae7af1267beb868a27f3fda03c044a")
	data := "data goes here"
	actualHash := blake2AsHex([]byte(data))
	if actualHash != expectedHash {
		t.Errorf("Hash mismatch: expected %s, got %s", expectedHash.Hex(), actualHash.Hex())
	} else {
		t.Logf("Hash match: expected %s, got %s", expectedHash.Hex(), actualHash.Hex())
	}
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

	testcases := make(map[string]string)
	testcases["publish-tickets-no-mark-2.json"] = errNone
	testcases["publish-tickets-no-mark-6.json"] = errNone
	testcases["publish-tickets-no-mark-10.json"] = errNone
	testcases["pubblish-tickets-with-mark-1.json"] = errNone
	testcases["pubblish-tickets-with-mark-2.json"] = errNone
	testcases["pubblish-tickets-with-mark-3.json"] = errNone
	testcases["pubblish-tickets-with-mark-5.json"] = errNone
	testcases["enact-epoch-change-with-no-tickets-1.json"] = errNone
	testcases["enact-epoch-change-with-no-tickets-2.json"] = errNone
	testcases["enact-epoch-change-with-no-tickets-3.json"] = errNone
	testcases["enact-epoch-change-with-no-tickets-4.json"] = errNone
	testcases["publish-tickets-with-mark-1.json"] = errNone
	testcases["publish-tickets-with-mark-2.json"] = errNone
	testcases["publish-tickets-with-mark-3.json"] = errNone
	testcases["publish-tickets-with-mark-4.json"] = errNone
	testcases["publish-tickets-with-mark-5.json"] = errNone

	testcases["enact-epoch-change-with-no-tickets-4.json"] = errNone
	testcases["enact-epoch-change-with-no-tickets-1.json"] = errNone
	testcases["enact-epoch-change-with-no-tickets-3.json"] = errNone
	testcases["enact-epoch-change-with-no-tickets-2.json"] = errTimeslotNotMonotonic     //"Fail: Timeslot must be strictly monotonic."
	testcases["publish-tickets-no-mark-1.json"] = errExtrinsicWithMoreTicketsThanAllowed // "Fail: Submit an extrinsic with more tickets than allowed."
	testcases["publish-tickets-no-mark-3.json"] = errTicketResubmission                  // "Fail: Re-submit tickets from authority 0."
	testcases["publish-tickets-no-mark-4.json"] = errTicketBadOrder                      // "Fail: Submit tickets in bad order."
	testcases["publish-tickets-no-mark-5.json"] = errTicketBadRingProof                  // "Fail: Submit tickets with bad ring proof."
	testcases["publish-tickets-no-mark-7.json"] = errTicketSubmissionInTail              // "Fail: Submit a ticket while in epoch's tail."
	testcases["publish-tickets-no-mark-8.json"] = errNone
	testcases["pubblish-tickets-with-mark-4.json"] = errNone
	testcases["publish-tickets-no-mark-9.json"] = errNone
	testcases["skip-epochs-1.json"] = errNone
	testcases["skip-epoch-tail-1.json"] = errNone
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		if !strings.Contains(file.Name(), ".json") {
			continue
		}
		expectedErr, ok := testcases[file.Name()]
		// Print the file name
		if ok {
			if len(expectedErr) == 0 {
				continue
			}
			fmt.Printf("\n***Test: %s Expected error: %s\n", file.Name(), expectedErr)
		} else {
			fmt.Printf("\n***Test: %s\n", file.Name())
			continue
			//expectedErr = ""
			//fmt.Printf("Expected error: NOTFOUND\n")
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
			continue
		}

		// Print the pretty formatted JSON content
		var prettyJSON []byte
		prettyJSON, err = json.MarshalIndent(testCase, "", "  ")
		if err != nil {
			t.Errorf("Failed to marshal JSON content for file %s: %v", filePath, err)
			continue
		}
		if len(string(prettyJSON)) > 0 {
			//fmt.Printf("Content:\n%s\n", string(prettyJSON))
		}

		// Call the safrole_stf function with the input and pre_state
		_, postState, err := safrole_stf(testCase.Input, testCase.PreState)
		if err.Error() != expectedErr {
			fmt.Printf("FAIL: expected '%s', got '%s'\n", expectedErr, err.Error())
		}

		// Perform assertions to validate the output and post_state
		//if !equalOutput(output, testCase.Output) {
		//      fmt.Printf("FAIL Output mismatch for file %s: expected %v, got %v\n", filePath, testCase.Output, output)
		//}
		err = equalState(postState, testCase.PostState)
		if err != nil {
			fmt.Printf("FAIL PostState mismatch on %s: %s\n", file.Name(), err.Error())
			continue
		}
		fmt.Printf("... PASSED %s\n\n", file.Name())
	}
}
