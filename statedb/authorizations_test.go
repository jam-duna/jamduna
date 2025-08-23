//go:build testing
// +build testing

package statedb

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

type TmpReport struct {
	CoreIndex      uint16      `json:"core"`
	AuthorizerHash common.Hash `json:"auth_hash"`
}

type AuthInput struct {
	Slot    uint32      `json:"slot"`
	Reports []TmpReport `json:"auths"`
}

type AuthState struct {
	AuthorizationsPool [types.TotalCores][]common.Hash `json:"auth_pools"`  // alpha The core αuthorizations pool. α eq 85
	AuthorizationQueue types.AuthorizationQueue        `json:"auth_queues"` // phi - The authorization queue  φ eq 85
}

func (a *AuthState) String() string {
	return ToJSON(a)
}

func ToJSON(v interface{}) string {
	jsonData, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	return string(jsonData)
}

type AuthTestCase struct {
	Input     AuthInput `json:"input"`
	PreState  AuthState `json:"pre_state"`
	PostState AuthState `json:"post_state"`
}

func GetBlockFromAuthInput(input AuthInput) *types.Block {
	eg := make([]types.Guarantee, 0)
	for _, r := range input.Reports {
		eg = append(eg, types.Guarantee{
			Report: types.WorkReport{
				CoreIndex:      uint(r.CoreIndex),
				AuthorizerHash: r.AuthorizerHash,
			},
			Slot: input.Slot,
		})
	}
	return &types.Block{
		Header: types.BlockHeader{
			Slot: input.Slot,
		},
		Extrinsic: types.ExtrinsicData{
			Guarantees: eg,
		},
	}
}

func (j *JamState) GetStateFromAuthState(input AuthInput, authState AuthState) {
	j.SafroleState.Timeslot = input.Slot
	j.AuthorizationsPool = authState.AuthorizationsPool
	j.AuthorizationQueue = authState.AuthorizationQueue
}

func (j *JamState) JamStateToAuthState() AuthState {
	return AuthState{
		AuthorizationsPool: j.AuthorizationsPool,
		AuthorizationQueue: j.AuthorizationQueue,
	}
}

func TestAuthParsing(t *testing.T) {
	// read the json file
	// parse the json file
	json_file := path.Join(common.GetJAMTestVectorPath("stf"), "authorizations/tiny/progress_authorizations-1.json")
	jsonData, err := os.ReadFile(json_file)
	if err != nil {
		t.Fatalf("failed to read JSON file: %v", err)
	}
	var testCase AuthTestCase
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
func VerifyAuths(jsonFile string, exceptErr error) error {
	jsonPath := path.Join(common.GetJAMTestVectorPath("stf"), "authorizations", jsonFile)
	jsonData, err := os.ReadFile(jsonPath)
	if err != nil {
		return fmt.Errorf("failed to read JSON file: %v", err)
	}
	var testCase AuthTestCase
	err = json.Unmarshal(jsonData, &testCase)
	if err != nil {
		return fmt.Errorf("failed to parse JSON file: %v", err)
	}
	var db StateDB
	state := NewJamState()
	db.JamState = state
	db.JamState.GetStateFromAuthState(testCase.Input, testCase.PreState)
	block := GetBlockFromAuthInput(testCase.Input)
	db.Block = block
	err = db.ApplyStateTransitionAuthorizations()
	if err != exceptErr {
		return fmt.Errorf("expected error %v, got %v", exceptErr, err)
	}
	post_state := db.JamState
	post_state_auth := post_state.JamStateToAuthState()
	post_state_from_testcase := testCase.PostState
	if post_state_auth.String() != post_state_from_testcase.String() {
		return fmt.Errorf("expected %v\n got %v", post_state_from_testcase, post_state_auth)
	}
	return nil
}

func TestVerifyAuths(t *testing.T) {
	network_args := *network
	t.Logf("Test case for Authorizations, Network=%s\n", network_args)

	testcase := []string{
		fmt.Sprintf("%s/progress_authorizations-1.json", network_args),
		fmt.Sprintf("%s/progress_authorizations-2.json", network_args),
		fmt.Sprintf("%s/progress_authorizations-3.json", network_args),
	}

	for _, tc := range testcase {
		tc := tc // capture range variable
		t.Run(tc, func(t *testing.T) {
			err := VerifyAuths(tc, nil)
			if err != nil {
				t.Fatalf("failed: %v", err)
			} else {
				fmt.Printf("\033[32mTest case %v passed\033[0m\n", tc)
			}
		})
	}
}
