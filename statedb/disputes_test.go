package statedb

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/colorfulnotion/jam/jamerrors"
	"github.com/colorfulnotion/jam/types"
)

type DJamState struct {
	Psi    Psi_state               `json:"psi"`
	Rho    AvailabilityAssignments `json:"rho"`
	Tau    uint32                  `json:"tau"`
	Kappa  types.Validators        `json:"kappa"`
	Lambda types.Validators        `json:"lambda"`
}

func (j *JamState) GetStateFromDJamState(dJamState DJamState) {
	j.DisputesState = dJamState.Psi
	j.AvailabilityAssignments = dJamState.Rho
	j.SafroleState.Timeslot = dJamState.Tau
	j.SafroleState.CurrValidators = dJamState.Kappa
	j.SafroleState.PrevValidators = dJamState.Lambda
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

/*
progress_with_verdicts-1 🔴	disputes	ErrDNotSortedWorkReports
progress_with_verdicts-2 🔴	disputes	ErrDNotUniqueVotes
progress_with_verdicts-3 🔴	disputes	ErrDNotSortedValidVerdicts
progress_with_verdicts-5 🔴	disputes	ErrDNotHomogenousJudgements
progress_with_culprits-1 🔴	disputes	ErrDMissingCulpritsBadVerdict
progress_with_culprits-2 🔴	disputes	ErrDSingleCulpritBadVerdict
progress_with_culprits-3 🔴	disputes	ErrDTwoCulpritsBadVerdictNotSorted
progress_with_culprits-5 🔴	disputes	ErrDAlreadyRecordedVerdict
progress_with_culprits-6 🔴	disputes	ErrDCulpritAlreadyInOffenders
progress_with_culprits-7 🔴	disputes	ErrDOffenderNotPresentVerdict
progress_with_faults-1 🔴	disputes	ErrDMissingFaultsGoodVerdict
progress_with_faults-3 🔴	disputes	ErrDTwoFaultOffendersGoodVerdict
progress_with_faults-5 🔴	disputes	ErrDAlreadyRecordedVerdictWithFaults
progress_with_faults-6 🔴	disputes	ErrDFaultOffenderInOffendersList
progress_with_faults-7 🔴	disputes	ErrDAuditorMarkedOffender
progress_with_bad_signatures-1 🔴	disputes	ErrDBadSignatureInVerdict
progress_with_bad_signatures-2 🔴	disputes	ErrDBadSignatureInCulprits
progress_with_verdict_signatures_from_previous_set-2 🔴	disputes	ErrDAgeTooOldInVerdicts
*/

func TestDisputesJsonParse(t *testing.T) {
	json_file := "../jamtestvectors/disputes/tiny/progress_with_culprits-6.json"
	jsonData, err := os.ReadFile(json_file)
	if err != nil {
		t.Fatalf("failed to read JSON file: %v", err)
	}
	var testCase DisputeData
	err = json.Unmarshal(jsonData, &testCase)
	if err != nil {
		t.Fatalf("failed to parse JSON file: %v", err)
	}
	// check the parsed values
	// pretty print the parsed values
	expectedJson, err := json.MarshalIndent(testCase, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal JSON: %v", err)
	}
	if debugAudit {
		fmt.Printf("Expected: %s\n", expectedJson)
	}
}

func VerifyDisputes(jsonFile string, exceptErr error) error {
	jsonPath := filepath.Join("../jamtestvectors/disputes/tiny", jsonFile)
	jsonData, err := os.ReadFile(jsonPath)
	if err != nil {
		return fmt.Errorf("failed to read JSON file: %v", err)
	}
	var testCase DisputeData
	err = json.Unmarshal(jsonData, &testCase)
	if err != nil {
		return fmt.Errorf("failed to parse JSON file: %v", err)
	}
	var db StateDB
	state := NewJamState()
	state.GetStateFromDJamState(testCase.PreState)
	db.JamState = state
	var block types.Block
	db.Block = &block
	db.Block.Extrinsic.Disputes = testCase.Input.Disputes
	disputes := db.Block.Extrinsic.Disputes
	_, err = db.JamState.IsValidateDispute(&disputes)
	if err != exceptErr {
		return fmt.Errorf("got %v", err)
	}
	fmt.Printf("Disputes PASS: %s\n", jsonFile)
	return nil

}

func VerifyDisputesFull(jsonFile string, exceptErr error) error {
	jsonPath := filepath.Join("../jamtestvectors/disputes/full", jsonFile)
	jsonData, err := os.ReadFile(jsonPath)
	if err != nil {
		return fmt.Errorf("failed to read JSON file: %v", err)
	}
	var testCase DisputeData
	err = json.Unmarshal(jsonData, &testCase)
	if err != nil {
		return fmt.Errorf("failed to parse JSON file: %v", err)
	}
	var db StateDB
	state := NewJamState()
	state.GetStateFromDJamState(testCase.PreState)
	db.JamState = state
	var block types.Block
	db.Block = &block
	db.Block.Extrinsic.Disputes = testCase.Input.Disputes
	disputes := db.Block.Extrinsic.Disputes
	_, err = db.JamState.IsValidateDispute(&disputes)
	if err != exceptErr {
		return fmt.Errorf("got %v", err)
	}
	fmt.Printf("Disputes PASS: %s\n", jsonFile)
	return nil

}

/*
progress_with_no_verdicts-1 🟢

No verdicts, nothing special happens
progress_with_verdicts-1 🔴

Not sorted work reports within a verdict
progress_with_verdicts-2 🔴

Not unique votes within a verdict
progress_with_verdicts-3 🔴

Not sorted, valid verdicts
progress_with_verdicts-4 🟢

Sorted, valid verdicts
progress_with_verdicts-5 🔴

Not homogeneous judgements, but positive votes count is not correct
progress_with_verdicts-6 🟢

Not homogeneous judgements, results in wonky verdict
progress_with_culprits-1 🔴

Missing culprits for bad verdict
progress_with_culprits-2 🔴

Single culprit for bad verdict
progress_with_culprits-3 🔴

Two culprits for bad verdict, not sorted
progress_with_culprits-4 🟢

Two culprits for bad verdict, sorted
progress_with_culprits-5 🔴

Report an already recorded verdict, with culprits
progress_with_culprits-6 🔴

Culprit offender already in the offenders list
progress_with_culprits-7 🔴

Offender relative to a not present verdict
progress_with_faults-1 🔴

Missing faults for good verdict
progress_with_faults-2 🟢

One fault offender for good verdict
progress_with_faults-3 🔴

Two fault offenders for a good verdict, not sorted
progress_with_faults-4 🟢

Two fault offenders for a good verdict, sorted
progress_with_faults-5 🔴

Report an already recorded verdict, with faults
progress_with_faults-6 🔴

Fault offender already in the offenders list
progress_with_faults-7 🔴

Auditor marked as offender, but vote matches the verdict.
progress_invalidates_avail_assignments-1 🟢

Invalidation of availability assignments
progress_with_bad_signatures-1 🔴

Bad signature within the verdict judgements
progress_with_bad_signatures-2 🔴

Bad signature within the culprits sequence
progress_with_verdict_signatures_from_previous_set-1 🟢

Use previous epoch validators set for verdict signatures verification
progress_with_verdict_signatures_from_previous_set-2 🔴

Age too old for verdicts judgements
*/

func TestVerifyDisputesTiny(t *testing.T) {
	testCases := []struct {
		jsonFile    string
		expectedErr error
	}{
		{"progress_with_no_verdicts-1.json", nil},
		{"progress_with_verdicts-1.json", jamerrors.ErrDNotSortedWorkReports},
		{"progress_with_verdicts-2.json", jamerrors.ErrDNotUniqueVotes},
		{"progress_with_verdicts-3.json", jamerrors.ErrDNotSortedValidVerdicts},
		{"progress_with_verdicts-4.json", nil},
		{"progress_with_verdicts-5.json", jamerrors.ErrDNotHomogenousJudgements},
		{"progress_with_verdicts-6.json", nil},
		{"progress_with_culprits-1.json", jamerrors.ErrDMissingCulpritsBadVerdict},
		{"progress_with_culprits-2.json", jamerrors.ErrDSingleCulpritBadVerdict},
		{"progress_with_culprits-3.json", jamerrors.ErrDTwoCulpritsBadVerdictNotSorted},
		{"progress_with_culprits-4.json", nil},
		{"progress_with_culprits-5.json", jamerrors.ErrDAlreadyRecordedVerdict},
		{"progress_with_culprits-6.json", jamerrors.ErrDCulpritAlreadyInOffenders},
		{"progress_with_culprits-7.json", jamerrors.ErrDOffenderNotPresentVerdict},
		{"progress_with_faults-1.json", jamerrors.ErrDMissingFaultsGoodVerdict},
		{"progress_with_faults-2.json", nil},
		{"progress_with_faults-3.json", jamerrors.ErrDTwoFaultOffendersGoodVerdict},
		{"progress_with_faults-4.json", nil},
		{"progress_with_faults-5.json", jamerrors.ErrDAlreadyRecordedVerdictWithFaults},
		{"progress_with_faults-6.json", jamerrors.ErrDFaultOffenderInOffendersList},
		{"progress_with_faults-7.json", jamerrors.ErrDAuditorMarkedOffender},
		{"progress_invalidates_avail_assignments-1.json", nil},
		{"progress_with_bad_signatures-1.json", jamerrors.ErrDBadSignatureInVerdict},
		{"progress_with_bad_signatures-2.json", jamerrors.ErrDBadSignatureInCulprits},
		{"progress_with_verdict_signatures_from_previous_set-1.json", nil},
		{"progress_with_verdict_signatures_from_previous_set-2.json", jamerrors.ErrDAgeTooOldInVerdicts},
	}
	for _, tc := range testCases {
		t.Run(tc.jsonFile, func(t *testing.T) {
			err := VerifyDisputes(tc.jsonFile, tc.expectedErr)
			if err != nil {
				t.Fatalf("Failed in File %s\nexcept error %v\nfailed to verify disputes: %v", tc.jsonFile, tc.expectedErr, err)
			}
		})
	}
}
func TestVerifyDisputesFull(t *testing.T) {
	testCases := []struct {
		jsonFile    string
		expectedErr error
	}{
		{"progress_with_no_verdicts-1.json", nil},
		{"progress_with_verdicts-1.json", jamerrors.ErrDNotSortedWorkReports},
		{"progress_with_verdicts-2.json", jamerrors.ErrDNotUniqueVotes},
		{"progress_with_verdicts-3.json", jamerrors.ErrDNotSortedValidVerdicts},
		{"progress_with_verdicts-4.json", nil},
		{"progress_with_verdicts-5.json", jamerrors.ErrDNotHomogenousJudgements},
		{"progress_with_verdicts-6.json", nil},
		{"progress_with_culprits-1.json", jamerrors.ErrDMissingCulpritsBadVerdict},
		{"progress_with_culprits-2.json", jamerrors.ErrDSingleCulpritBadVerdict},
		{"progress_with_culprits-3.json", jamerrors.ErrDTwoCulpritsBadVerdictNotSorted},
		{"progress_with_culprits-4.json", nil},
		{"progress_with_culprits-5.json", jamerrors.ErrDAlreadyRecordedVerdict},
		{"progress_with_culprits-6.json", jamerrors.ErrDCulpritAlreadyInOffenders},
		{"progress_with_culprits-7.json", jamerrors.ErrDOffenderNotPresentVerdict},
		{"progress_with_faults-1.json", jamerrors.ErrDMissingFaultsGoodVerdict},
		{"progress_with_faults-2.json", nil},
		{"progress_with_faults-3.json", jamerrors.ErrDTwoFaultOffendersGoodVerdict},
		{"progress_with_faults-4.json", nil},
		{"progress_with_faults-5.json", jamerrors.ErrDAlreadyRecordedVerdictWithFaults},
		{"progress_with_faults-6.json", jamerrors.ErrDFaultOffenderInOffendersList},
		{"progress_with_faults-7.json", jamerrors.ErrDAuditorMarkedOffender},
		{"progress_invalidates_avail_assignments-1.json", nil},
		{"progress_with_bad_signatures-1.json", jamerrors.ErrDBadSignatureInVerdict},
		{"progress_with_bad_signatures-2.json", jamerrors.ErrDBadSignatureInCulprits},
		{"progress_with_verdict_signatures_from_previous_set-1.json", nil},
		{"progress_with_verdict_signatures_from_previous_set-2.json", jamerrors.ErrDAgeTooOldInVerdicts},
	}
	for _, tc := range testCases {
		t.Run(tc.jsonFile, func(t *testing.T) {
			err := VerifyDisputesFull(tc.jsonFile, tc.expectedErr)
			if err != nil {
				t.Fatalf("Failed in File %s\nexcept error %v\nfailed to verify disputes: %v", tc.jsonFile, tc.expectedErr, err)
			}
		})
	}
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

			if debugAudit {
				fmt.Printf("\n\n\nTesting %v\n", targetedStructType)
			}

			// Read and unmarshal JSON file
			jsonData, err := os.ReadFile(jsonPath)
			if err != nil {
				t.Fatalf("failed to read JSON file: %v", err)
			}

			err = json.Unmarshal(jsonData, tc.expectedType)
			if err != nil {
				t.Fatalf("failed to unmarshal JSON data: %v", err)
			}
			if debugAudit {
				fmt.Printf("Unmarshaled %s\n", jsonPath)
				fmt.Printf("Expected: %v\n", tc.expectedType)
			}
			// Encode the struct to bytes
			encodedBytes, err := types.Encode(tc.expectedType)
			if err != nil {
				t.Fatalf("failed to encode struct: %v", err)
			}

			if debugAudit {
				fmt.Printf("Encoded: %x\n\n", encodedBytes)
			}
			decodedStruct, _, err := types.Decode(encodedBytes, targetedStructType)
			if err != nil {
				t.Fatalf("failed to decode bytes: %v", err)
			}
			if debugAudit {
				fmt.Printf("Decoded:  %v\n\n", decodedStruct)
			}
			// Marshal the struct to JSON
			encodedJSON, err := json.MarshalIndent(decodedStruct, "", "  ")
			if err != nil {
				t.Fatalf("failed to marshal JSON data: %v", err)
			}
			if debugAudit {
				fmt.Printf("Encoded JSON:\n%s\n", encodedJSON)
			}
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
