package statedb

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/jamerrors"
	"github.com/colorfulnotion/jam/types"
)

type AssuranceInput struct {
	Slot       uint64            `json:"slot"`
	ParentHash common.Hash       `json:"parent"`
	Assurances []types.Assurance `json:"assurances"`
}

type AssuranceState struct {
	AvailabilityAssignments AvailabilityAssignments `json:"avail_assignments"`
	CurrValidators          types.Validators        `json:"curr_validators"`
}
type AssuranceTestCase struct {
	Input     AssuranceInput `json:"input"`
	PreState  AssuranceState `json:"pre_state"`
	PostState AssuranceState `json:"post_state"`
}

func (j *JamState) GetStateFromAssuranceState(assuranceState AssuranceState) {
	j.AvailabilityAssignments = assuranceState.AvailabilityAssignments
	j.SafroleState.CurrValidators = assuranceState.CurrValidators
}

func TestAssuranceParsing(t *testing.T) {
	// read the json file
	// parse the json file
	json_file := "../jamtestvectors/assurances/tiny/assurance_for_not_engaged_core-1.json"
	jsonData, err := os.ReadFile(json_file)
	if err != nil {
		t.Fatalf("failed to read JSON file: %v", err)
	}
	var testCase AssuranceTestCase
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
	fmt.Printf("Expected: %s\n", expectedJson)
}

func VerifyAssurances(jsonFile string, exceptErr error) error {
	jsonPath := filepath.Join("../jamtestvectors/assurances/tiny", jsonFile)
	jsonData, err := os.ReadFile(jsonPath)
	if err != nil {

	}
	var testCase AssuranceTestCase
	err = json.Unmarshal(jsonData, &testCase)
	if err != nil {
		return fmt.Errorf("failed to parse JSON file: %v", err)
	}
	var db StateDB
	state := NewJamState()
	db.JamState = state
	db.JamState.GetStateFromAssuranceState(testCase.PreState)
	db.GetSafrole().Timeslot = uint32(testCase.Input.Slot)
	var block types.Block
	db.Block = &block
	db.Block.Extrinsic.Assurances = testCase.Input.Assurances
	db.Block.Header.ParentHeaderHash = testCase.Input.ParentHash
	db.Block.Header.Slot = uint32(testCase.Input.Slot)
	db.ParentHeaderHash = testCase.Input.ParentHash
	err = db.ValidateAssurances(db.Block.Extrinsic.Assurances)
	if err != exceptErr {
		return fmt.Errorf("expected error %v, got %v", exceptErr, err)
	}
	fmt.Printf("File Passed: %s\n", jsonFile)
	return nil
}

/*
assurances_with_bad_signature-1🔴		ErrABadSignature
assurances_with_bad_validator_index-1🔴		ErrABadValidatorIndex
assurance_for_not_engaged_core-1🔴		ErrABadCore
assurance_with_bad_attestation_parent-1🔴		ErrABadParentHash
assurances_for_stale_report-1🔴		ErrAStaleReport
*/

func TestVerifyAssurance(t *testing.T) {
	testCase := []struct {
		jsonFile  string
		exceptErr error
	}{
		{"assurances_with_bad_signature-1.json", jamerrors.ErrABadSignature},
		{"assurances_with_bad_validator_index-1.json", jamerrors.ErrABadValidatorIndex},
		{"assurance_for_not_engaged_core-1.json", jamerrors.ErrABadCore},
		{"assurance_with_bad_attestation_parent-1.json", jamerrors.ErrABadParentHash},
		{"assurances_for_stale_report-1.json", jamerrors.ErrAStaleReport},
	}
	for _, tc := range testCase {
		t.Run(tc.jsonFile, func(t *testing.T) {
			err := VerifyAssurances(tc.jsonFile, tc.exceptErr)
			if err != nil {
				t.Fatalf("failed to validate assurance: %v", err)
			}
		})
	}
}
