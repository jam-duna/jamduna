//go:build testing
// +build testing

package statedb

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/jamerrors"
	"github.com/colorfulnotion/jam/log"
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
	log.Debug(debugA, "Expected", expectedJson)
}

func VerifyAssurances(jsonFile string, exceptErr error) error {
	jsonPath := filepath.Join("../jamtestvectors/assurances/", jsonFile)
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
	err = db.ValidateAssurancesWithSig(db.Block.Extrinsic.Assurances)
	if err != exceptErr {
		return fmt.Errorf("expected error %v, got %v", exceptErr, err)
	}
	return nil
}

/*
no_assurances-1🟢
Progress with an empty assurances extrinsic.
some_assurances-1 🟢
Several assurances contributing to establishing availability supermajority for some of the cores.
no_assurances_with_stale_report-1 🟢
Progress with an empty assurances extrinsic.
Stale work report assignment is removed (but not returned in the output).
assurances_with_bad_signature-1🔴
One assurance has a bad signature.
assurances_with_bad_validator_index-1🔴
One assurance has a bad validator index.
assurance_for_not_engaged_core-1🔴
One assurance targets a core without any assigned work report.
assurance_with_bad_attestation_parent-1🔴
One assurance has a bad attestation parent hash.
assurances_for_stale_report-1🔴
One assurance targets a core with a stale report.
We are lenient on the stale report as far as it is available.
assurers_not_sorted_or_unique-1🔴
Assurers not sorted.
assurers_not_sorted_or_unique-2🔴
Duplicate assurer.
*/

func TestVerifyAssurance(t *testing.T) {
	network_args := *network
	fmt.Printf("Test Cases For Assurances, Network=%s\n", network_args)
	testCase := []struct {
		jsonFile  string
		exceptErr error
	}{
		{fmt.Sprintf("%s/no_assurances-1.json", network_args), nil},
		{fmt.Sprintf("%s/some_assurances-1.json", network_args), nil},
		{fmt.Sprintf("%s/no_assurances_with_stale_report-1.json", network_args), nil},
		{fmt.Sprintf("%s/assurances_with_bad_signature-1.json", network_args), jamerrors.ErrABadSignature},
		{fmt.Sprintf("%s/assurances_with_bad_validator_index-1.json", network_args), jamerrors.ErrABadValidatorIndex},
		{fmt.Sprintf("%s/assurance_for_not_engaged_core-1.json", network_args), jamerrors.ErrABadCore},
		{fmt.Sprintf("%s/assurance_with_bad_attestation_parent-1.json", network_args), jamerrors.ErrABadParentHash},
		{fmt.Sprintf("%s/assurances_for_stale_report-1.json", network_args), jamerrors.ErrAStaleReport},
		{fmt.Sprintf("%s/assurers_not_sorted_or_unique-1.json", network_args), jamerrors.ErrANotSortedAssurers},
		{fmt.Sprintf("%s/assurers_not_sorted_or_unique-2.json", network_args), jamerrors.ErrADuplicateAssurer},
	}
	for _, tc := range testCase {
		t.Run(tc.jsonFile, func(t *testing.T) {
			err := VerifyAssurances(tc.jsonFile, tc.exceptErr)
			if err != nil {
				t.Fatalf("failed to validate assurance: %v", err)
			} else {
				fmt.Printf("\033[32mAssurance Test case %s passed\033[0m\n", tc.jsonFile)
			}
		})
	}
}
