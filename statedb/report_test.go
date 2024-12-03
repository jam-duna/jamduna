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

type TestReport struct {
	Input     ReportInput `json:"input"`
	PreState  StateReport `json:"pre_state"`
	PostState StateReport `json:"post_state"`
}

type ReportInput struct {
	Guarantee []types.Guarantee `json:"guarantees"`
	Slot      uint64            `json:"slot"`
}

type StateReport struct {
	AvailabilityAssignments  AvailabilityAssignments         `json:"avail_assignments"`
	CurrValidators           types.Validators                `json:"curr_validators"`
	PrevValidators           types.Validators                `json:"prev_validators"`
	Entropy                  Entropy                         `json:"entropy"`
	Offenders                []types.Ed25519Key              `json:"offenders"`
	RecentBlocks             RecentBlocks                    `json:"recent_blocks"`
	AuthorizationsPool       [types.TotalCores][]common.Hash `json:"auth_pools"`
	PriorServiceAccountState []ServiceItem                   `json:"services"`
}

type ServiceItem struct {
	ServiceID uint32  `json:"id"`
	Service   Service `json:"info"`
}

type Service struct {
	CodeHash   common.Hash `json:"code_hash"`
	Balance    uint64      `json:"balance"`
	MinItemGas uint64      `json:"min_item_gas"`
	MinMemoGas uint64      `json:"min_memo_gas"`
	CodeSize   uint64      `json:"bytes"`
	Items      uint32      `json:"items"`
}

func ServiceToSeviceAccount(s []ServiceItem) map[uint32]types.ServiceAccount {
	result := make(map[uint32]types.ServiceAccount)
	for _, value := range s {
		result[value.ServiceID] = types.ServiceAccount{
			CodeHash:        value.Service.CodeHash,
			Balance:         value.Service.Balance,
			GasLimitG:       value.Service.MinItemGas,
			GasLimitM:       value.Service.MinMemoGas,
			StorageSize:     value.Service.CodeSize,
			NumStorageItems: value.Service.Items,
		}
	}
	return result
}

func (j *JamState) GetStateFromReportState(r StateReport) {
	j.AvailabilityAssignments = r.AvailabilityAssignments
	j.SafroleState.CurrValidators = r.CurrValidators
	j.SafroleState.PrevValidators = r.PrevValidators
	j.SafroleState.Entropy = r.Entropy
	j.RecentBlocks = r.RecentBlocks
	j.AuthorizationsPool = r.AuthorizationsPool
	j.PriorServiceAccountState = ServiceToSeviceAccount(r.PriorServiceAccountState)
}

// give a json file and read it into a StateReport struct
func TestReportParsing(t *testing.T) {
	testCases := []struct {
		jsonFile     string
		binFile      string
		expectedType interface{}
	}{
		// {"not_sorted_guarantor-1.json", "not_sorted_guarantor-1.bin", &TestReport{}},
		{"reports_with_dependencies-1.json", "reports_with_dependencies-1.bin", &TestReport{}},
	}
	for _, tc := range testCases {
		t.Run(tc.jsonFile, func(t *testing.T) {
			jsonPath := filepath.Join("../jamtestvectors/reports/tiny", tc.jsonFile)
			// binPath := filepath.Join("../jamtestvectors/safrole/tiny", tc.binFile)

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
			// marshal the struct to JSON
			expectedJson, err := json.MarshalIndent(tc.expectedType, "", "  ")
			fmt.Printf("Expected: %s\n", expectedJson)
			// Encode the struct to bytes
			encodedBytes, err := types.Encode(tc.expectedType)
			if err != nil {
				t.Fatalf("failed to encode data: %v", err)
			}

			decodedStruct, _, err := types.Decode(encodedBytes, targetedStructType)
			if err != nil {
				t.Fatalf("failed to decode data: %v", err)
			}

			// Marshal the struct to JSON
			encodedJSON, err := json.MarshalIndent(decodedStruct, "", "  ")
			if err != nil {
				t.Fatalf("failed to marshal JSON data: %v", err)
			}
			fmt.Printf("Encoded JSON:\n%s\n", encodedJSON)

			// Unmarshal again to compare
			var decodedStruct2 TestReport
			err = json.Unmarshal(encodedJSON, &decodedStruct2)
			if err != nil {
				t.Fatalf("failed to unmarshal JSON data: %v", err)
			}

			// Compare the two structs
			if !reflect.DeepEqual(tc.expectedType, decodedStruct2) {
				t.Fatalf("decoded struct and decoded struct 2 are not equal")
			}
		})
	}
}
func ReportVerify(jsonFile string, exceptErr bool) error {
	jsonPath := filepath.Join("../jamtestvectors/reports/tiny", jsonFile)
	fmt.Printf("===Start to verify %s===\n", jsonPath)
	// Read and unmarshal JSON file
	jsonData, err := os.ReadFile(jsonPath)
	if err != nil {
		return fmt.Errorf("failed to read JSON file: %v", err)
	}

	var report TestReport
	err = json.Unmarshal(jsonData, &report)
	if err != nil {
		return fmt.Errorf("failed to unmarshal JSON data: %v", err)
	}
	// fmt.Println("unmarshal json data:", report)

	var db StateDB
	state := NewJamState()
	db.JamState = state
	var block types.Block
	db.Block = &block
	db.JamState.GetStateFromReportState(report.PreState)
	db.JamState.SafroleState.Entropy = report.PreState.Entropy
	db.Block.Header.Slot = uint32(report.Input.Slot)
	db.JamState.SafroleState.Timeslot = uint32(report.Input.Slot)
	db.Block.Header.OffendersMark = report.PreState.Offenders
	db.Block.Extrinsic.Guarantees = report.Input.Guarantee
	db.AssignGuarantors(true)
	db.PreviousGuarantors(true)

	err = db.Verify_Guarantees()
	if err != nil && !exceptErr {
		return fmt.Errorf("failed to verify guarantee: %v", err)
	}
	fmt.Printf("guarantor assignments: %v\n", len(db.PreviousGuarantorAssignments))
	if err == nil && exceptErr {
		return fmt.Errorf("Expected have error")
	}
	if err != nil && exceptErr {
		fmt.Printf("Expected error: %v\n", err)
		//check error prefix vs json file name
		// read string until the first '-'
		// if the prefix is not the same as the json file name, return error
		// if the prefix is the same as the json file name, return nil

		error_string := err.Error()
		jsonfilename := jsonFile[:len(jsonFile)-7]
		jsonname_length := len(jsonfilename)
		errorname := error_string[:jsonname_length]
		if errorname != jsonfilename {
			return fmt.Errorf("Error name is not the same as json file name(%s , %s)", errorname, jsonfilename)
		}
	}
	post_state := NewJamState()
	post_state.GetStateFromReportState(report.PostState)
	db.JamState.ProcessGuarantees(db.Block.Guarantees())
	if !exceptErr {
		for i, rho := range db.JamState.AvailabilityAssignments {
			if rho != post_state.AvailabilityAssignments[i] {
				return nil
			}
		}
	}

	return nil
}

func TestReportVerifyTiny(t *testing.T) {
	// run throgh all the json files in the tiny folder

	// shawn cover
	/*
		bad_code_hash 🔴
		bad_core_index 🔴
		bad_signature 🔴
		core_engaged 🔴
		dependency_missing 🔴
		duplicated_package_in_report 🔴
		future_report_slot 🔴
		no_enough_guarantees 🔴
		not_sorted_guarantor 🔴
		out_of_order_guarantees 🔴
		too_high_work_report_gas 🔴
		bad_validator_index 🔴
		wrong_assignment 🔴
	*/
	testCases := []struct {
		jsonFile string
		except   bool
	}{
		{"not_sorted_guarantor-1.json", true},
		{"reports_with_dependencies-1.json", false},
		{"bad_code_hash-1.json", true},
		{"bad_core_index-1.json", true},
		{"bad_signature-1.json", true},
		{"core_engaged-1.json", true},
		{"dependency_missing-1.json", true},
		{"duplicated_package_in_report-1.json", true},
		{"future_report_slot-1.json", true},
		{"no_enough_guarantees-1.json", true},
		{"not_sorted_guarantor-1.json", true},
		{"out_of_order_guarantees-1.json", true},
		{"too_high_work_report_gas-1.json", true},
		{"bad_validator_index-1.json", true},
		{"wrong_assignment-1.json", true},
	}
	for _, tc := range testCases {
		t.Run(tc.jsonFile, func(t *testing.T) {
			err := ReportVerify(tc.jsonFile, tc.except)
			if err != nil {
				t.Fatalf("failed : %v", err)
			}
			fmt.Printf("===Finish %s===\n", tc.jsonFile)
		})
	}
}

func TestReportVerifyTinyStanley(t *testing.T) {
	// run throgh all the json files in the tiny folder

	// stanley cover
	/*
		anchor_not_recent 🔴
		bad_beefy_mmr 🔴
		bad_service_id 🔴
		bad_state_root 🔴
		duplicate_package_in_recent_history 🔴
		report_before_last_rotation 🔴
		segment_root_lookup_invalid 🔴
		segment_root_lookup_invalid 🔴
	*/
	testCases := []struct {
		jsonFile string
		except   bool
	}{
		{"anchor_not_recent-1.json", true},
		{"bad_beefy_mmr-1.json", true},
		{"bad_service_id-1.json", true},
		{"bad_state_root-1.json", true},
		{"duplicate_package_in_recent_history-1.json", true},
		{"report_before_last_rotation-1.json", true},
		{"segment_root_lookup_invalid-1.json", true},
		{"segment_root_lookup_invalid-2.json", true},
	}
	for _, tc := range testCases {
		t.Run(tc.jsonFile, func(t *testing.T) {
			err := ReportVerify(tc.jsonFile, true)
			if err == nil {
				t.Fatalf("Expected error, but got none for %s", tc.jsonFile)
			}

			fmt.Printf("===Finish %s===\n", tc.jsonFile)
		})
	}

}
