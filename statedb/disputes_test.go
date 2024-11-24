package statedb

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

type DAvailabilityAssignments [types.TotalCores]*DRho_state
type DRho_state struct {
	DummyWorkReport [353]byte `json:"dummy_work_report"`
	Timeout         uint32    `json:"timeout"`
}

type DJamState struct {
	Psi    Psi_state                `json:"psi"`
	Rho    DAvailabilityAssignments `json:"rho"`
	Tau    uint32                   `json:"tau"`
	Kappa  types.Validators         `json:"kappa"`
	Lambda types.Validators         `json:"lambda"`
}

type DInput struct {
	Disputes types.Dispute `json:"disputes"`
}

type DisputeData struct {
	Input    DInput    `json:"input"`
	PreState DJamState `json:"pre_state"`
	// Output    DOutput   `json:"output"`
	PostState DJamState `json:"post_state"`
}

func (a *DRho_state) UnmarshalJSON(data []byte) error {
	var s struct {
		DummyWorkReport string `json:"dummy_work_report"`
		Timeout         uint32 `json:"timeout"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	dummy_work_report := common.FromHex(s.DummyWorkReport)
	copy(a.DummyWorkReport[:], dummy_work_report)
	a.Timeout = s.Timeout

	return nil
}

func (a DRho_state) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		DummyWorkReport string `json:"dummy_work_report"`
		Timeout         uint32 `json:"timeout"`
	}{
		DummyWorkReport: common.HexString(a.DummyWorkReport[:]),
		Timeout:         a.Timeout,
	})
}

func TestDisputeState(t *testing.T) {
	testCases := []struct {
		jsonFile     string
		binFile      string
		expectedType interface{}
	}{
		{"progress_with_bad_signatures-1.json", "progress_with_bad_signatures-1.bin", &DisputeData{}},
		{"progress_with_bad_signatures-2.json", "progress_with_bad_signatures-2.bin", &DisputeData{}},
		{"progress_with_culprits-1.json", "progress_with_culprits-1.bin", &DisputeData{}},
		{"progress_with_culprits-2.json", "progress_with_culprits-2.bin", &DisputeData{}},
		{"progress_with_culprits-3.json", "progress_with_culprits-3.bin", &DisputeData{}},
		{"progress_with_culprits-4.json", "progress_with_culprits-4.bin", &DisputeData{}},
		{"progress_with_culprits-5.json", "progress_with_culprits-5.bin", &DisputeData{}},
		{"progress_with_culprits-6.json", "progress_with_culprits-6.bin", &DisputeData{}},
		{"progress_with_culprits-7.json", "progress_with_culprits-7.bin", &DisputeData{}},
		{"progress_with_faults-1.json", "progress_with_faults-1.bin", &DisputeData{}},
		{"progress_with_faults-2.json", "progress_with_faults-2.bin", &DisputeData{}},
		{"progress_with_faults-3.json", "progress_with_faults-3.bin", &DisputeData{}},
		{"progress_with_faults-4.json", "progress_with_faults-4.bin", &DisputeData{}},
		{"progress_with_faults-5.json", "progress_with_faults-5.bin", &DisputeData{}},
		{"progress_with_faults-6.json", "progress_with_faults-6.bin", &DisputeData{}},
		{"progress_with_faults-7.json", "progress_with_faults-7.bin", &DisputeData{}},
		{"progress_with_no_verdicts-1.json", "progress_with_no_verdicts-1.bin", &DisputeData{}},
		{"progress_with_verdict_signatures_from_previous_set-1.json", "progress_with_verdict_signatures_from_previous_set-1.bin", &DisputeData{}},
		{"progress_with_verdict_signatures_from_previous_set-2.json", "progress_with_verdict_signatures_from_previous_set-2.bin", &DisputeData{}},
		{"progress_with_verdicts-1.json", "progress_with_verdicts-1.bin", &DisputeData{}},
		{"progress_with_verdicts-2.json", "progress_with_verdicts-2.bin", &DisputeData{}},
		{"progress_with_verdicts-3.json", "progress_with_verdicts-3.bin", &DisputeData{}},
		{"progress_with_verdicts-4.json", "progress_with_verdicts-4.bin", &DisputeData{}},
		{"progress_with_verdicts-5.json", "progress_with_verdicts-5.bin", &DisputeData{}},
		{"progress_with_verdicts-6.json", "progress_with_verdicts-6.bin", &DisputeData{}},
	}
	for _, tc := range testCases {
		t.Run(tc.jsonFile, func(t *testing.T) {
			jsonPath := filepath.Join("../jamtestvectors/disputes/tiny", tc.jsonFile)
			// binPath := filepath.Join("../jamtestvectors/history/data", tc.binFile)

			targetedStructType := reflect.TypeOf(tc.expectedType)

			fmt.Printf("\n\n\nTesting %v\n", targetedStructType)
			// Read and unmarshal JSON file
			jsonData, err := os.ReadFile(jsonPath)
			if err != nil {
				t.Fatalf("failed to read JSON file: %v", err)
			}

			err = json.Unmarshal(jsonData, tc.expectedType)
			if err != nil {
				t.Fatalf("failed to unmarshal JSON data: %v", err)
			}
			fmt.Printf("Unmarshaled %s\n", jsonPath)
			fmt.Printf("Expected: %v\n", tc.expectedType)
			// Encode the struct to bytes
			encodedBytes, err := types.Encode(tc.expectedType)
			if err != nil {
				t.Fatalf("failed to encode struct: %v", err)
			}

			fmt.Printf("Encoded: %x\n\n", encodedBytes)

			decodedStruct, _, err := types.Decode(encodedBytes, targetedStructType)
			if err != nil {
				t.Fatalf("failed to decode bytes: %v", err)
			}
			fmt.Printf("Decoded:  %v\n\n", decodedStruct)

			// Marshal the struct to JSON
			encodedJSON, err := json.MarshalIndent(decodedStruct, "", "  ")
			if err != nil {
				t.Fatalf("failed to marshal JSON data: %v", err)
			}
			fmt.Printf("Encoded JSON:\n%s\n", encodedJSON)

			// output bin file
			// err = os.WriteFile("./output.bin", encodedBytes, 0644)
			// if err != nil {
			// 	t.Fatalf("failed to write binary file: %v", err)
			// }

			// Read the expected bytes from the binary file
			// expectedBytes, err := os.ReadFile(binPath)
			// if err != nil {
			// 	t.Fatalf("failed to read binary file: %v", err)
			// }
			// assert.Equal(t, expectedBytes, encodedBytes, "encoded bytes do not match expected bytes")

			// if false {
			// 	decoded, _ := types.Decode(expectedBytes, reflect.TypeOf(tc.expectedType))
			// 	encodedBytes2 := types.Encode(decoded)
			// 	// Compare the encoded bytes with the expected bytes
			// 	assert.Equal(t, expectedBytes, encodedBytes2, "encoded bytes do not match expected bytes")
			// }

			// // Compare the encoded JSON with the original JSON
			// assert.JSONEq(t, string(jsonData), string(encodedJSON), "encoded JSON does not match original JSON")
		})
	}
}
