package statedb

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/jamerrors"
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
		{"segment_root_lookup_invalid-1.json", "segment_root_lookup_invalid-1.bin", &TestReport{}},
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
			// if !assert.Equal(t, tc.expectedType, decodedStruct2) {
			// 	t.Fatalf("decoded struct and decoded struct 2 are not equal")
			// }
		})
	}
}

func TestSignature(t *testing.T) {
	jsonFile := "reports_with_dependencies-1.json"
	jsonPath := filepath.Join("../jamtestvectors/reports/tiny", jsonFile)
	fmt.Printf("===Start to verify %s===\n", jsonPath)
	// Read and unmarshal JSON file
	jsonData, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("failed to read JSON file: %v", err)
	}
	var report TestReport
	err = json.Unmarshal(jsonData, &report)
	if err != nil {
		t.Fatalf("failed to unmarshal JSON data: %v", err)
	}
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
	CurrV := db.GetSafrole().CurrValidators
	guarantee := db.Block.Extrinsic.Guarantees[0]
	err = guarantee.Verify(CurrV) // errBadSignature
	fmt.Printf("Expected error: %v\n", err)

}

func ReportVerify(jsonFile string, exceptErr error) error {
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
	if err != nil && exceptErr == nil {
		return fmt.Errorf("failed to verify guarantee: %v", err)
	}
	if err == nil && exceptErr != nil {
		return fmt.Errorf("Expected have error:%v", exceptErr)
	}
	if err != nil && exceptErr != nil {
		fmt.Printf("Get error: %v\n", err)
		//check error prefix vs json file name
		// read string until the first '-'
		// if the prefix is not the same as the json file name, return error
		// if the prefix is the same as the json file name, return nil
		if err == exceptErr {
			return nil
		} else {
			return fmt.Errorf("Expected error: %v, but get %v\n", exceptErr, err)
		}
	}
	post_state := NewJamState()
	post_state.GetStateFromReportState(report.PostState)
	db.JamState.ProcessGuarantees(db.Block.Guarantees())
	if exceptErr == nil {
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
		bad_code_hash 🔴	reports	ErrGBadCodeHash
		bad_core_index 🔴	reports	ErrGBadCoreIndex
		bad_signature 🔴	reports	ErrGBadSignature
		core_engaged 🔴	reports	ErrGCoreEngaged
		dependency_missing 🔴	reports	ErrGDependencyMissing
		duplicated_package_in_report 🔴	reports	ErrGDuplicatePackageTwoReports
		future_report_slot 🔴	reports	ErrGFutureReportSlot
		no_enough_guarantees 🔴	reports	ErrGInsufficientGuarantees
		not_sorted_guarantor 🔴	reports	ErrGDuplicateGuarantors
		out_of_order_guarantees 🔴	reports	ErrGOutOfOrderGuarantee
		too_high_work_report_gas 🔴	reports	ErrGWorkReportGasTooHigh
		bad_validator_index 🔴	reports	ErrGBadValidatorIndex
		wrong_assignment 🔴	reports	ErrGWrongAssignment
		anchor_not_recent 🔴	reports	ErrGAnchorNotRecent
		bad_beefy_mmr 🔴	reports	ErrGBadBeefyMMRRoot
		bad_service_id 🔴	reports	ErrGBadServiceID
		bad_state_root 🔴	reports	ErrGBadStateRoot
		service_item_gas_too_low 🔴 reports ErrGServiceItemTooLow
		duplicate_package_in_recent_history 🔴	reports	ErrGDuplicatePackageRecentHistory
		report_before_last_rotation 🔴	reports	ErrGReportEpochBeforeLast
		segment_root_lookup_invalid 🔴	reports	ErrGSegmentRootLookupInvalidNotRecentBlocks
		segment_root_lookup_invalid 🔴	reports	ErrGSegmentRootLookupInvalidUnexpectedValue
		not_authorized 🔴	reports	ErrGCoreWithoutAuthorizer
		not_authorized 🔴	reports	ErrGCoreUnexpectedAuthorizer
	*/
	testCases := []struct {
		jsonFile string
		except   error
	}{
		{"bad_code_hash-1.json", jamerrors.ErrGBadCodeHash},
		{"bad_core_index-1.json", jamerrors.ErrGBadCoreIndex},
		{"bad_signature-1.json", jamerrors.ErrGBadSignature},
		{"core_engaged-1.json", jamerrors.ErrGCoreEngaged},
		{"dependency_missing-1.json", jamerrors.ErrGDependencyMissing},
		{"duplicated_package_in_report-1.json", jamerrors.ErrGDuplicatePackageTwoReports},
		{"future_report_slot-1.json", jamerrors.ErrGFutureReportSlot},
		{"no_enough_guarantees-1.json", jamerrors.ErrGInsufficientGuarantees},
		{"not_sorted_guarantor-1.json", jamerrors.ErrGDuplicateGuarantors},
		{"out_of_order_guarantees-1.json", jamerrors.ErrGOutOfOrderGuarantee},
		{"too_high_work_report_gas-1.json", jamerrors.ErrGWorkReportGasTooHigh},
		{"bad_validator_index-1.json", jamerrors.ErrGBadValidatorIndex},
		{"wrong_assignment-1.json", jamerrors.ErrGWrongAssignment},
		{"anchor_not_recent-1.json", jamerrors.ErrGAnchorNotRecent},
		{"bad_beefy_mmr-1.json", jamerrors.ErrGBadBeefyMMRRoot},
		{"bad_service_id-1.json", jamerrors.ErrGBadServiceID},
		{"bad_state_root-1.json", jamerrors.ErrGBadStateRoot},
		{"duplicate_package_in_recent_history-1.json", jamerrors.ErrGDuplicatePackageRecentHistory},
		{"report_before_last_rotation-1.json", jamerrors.ErrGReportEpochBeforeLast},
		{"segment_root_lookup_invalid-1.json", jamerrors.ErrGSegmentRootLookupInvalidNotRecentBlocks},
		{"segment_root_lookup_invalid-2.json", jamerrors.ErrGSegmentRootLookupInvalidUnexpectedValue},
		{"not_authorized-1.json", jamerrors.ErrGCoreWithoutAuthorizer},
		{"not_authorized-2.json", jamerrors.ErrGCoreUnexpectedAuthorizer},
		{"service_item_gas_too_low-1.json", jamerrors.ErrGServiceItemTooLow},
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

	// shawn cover
	/*
		bad_state_root 🔴	reports	ErrGBadStateRoot
		duplicate_package_in_recent_history 🔴	reports	ErrGDuplicatePackageRecentHistory
		segment_root_lookup_invalid 🔴	reports	ErrGSegmentRootLookupInvalidNotRecentBlocks
		segment_root_lookup_invalid 🔴	reports	ErrGSegmentRootLookupInvalidUnexpectedValue
	*/
	testCases := []struct {
		jsonFile string
		except   error
	}{
		// {"anchor_not_recent-1.json", jamerrors.ErrGAnchorNotRecent},

		{"bad_beefy_mmr-1.json", jamerrors.ErrGBadBeefyMMRRoot},
		{"bad_state_root-1.json", jamerrors.ErrGBadStateRoot},
		{"duplicate_package_in_recent_history-1.json", jamerrors.ErrGDuplicatePackageRecentHistory},
		{"segment_root_lookup_invalid-1.json", jamerrors.ErrGSegmentRootLookupInvalidNotRecentBlocks},
		{"segment_root_lookup_invalid-2.json", jamerrors.ErrGSegmentRootLookupInvalidUnexpectedValue},
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

// These cases should NOT return any errorduring STF check
func TestReportValidReportCaseTiny(t *testing.T) {
	/* should cover the following cases

	   report_curr_rotation 🟢
	   Report uses current guarantors rotation.

	   report_prev_rotation 🟢
	   Report uses previous guarantors rotation.
	   Previous rotation falls within previous epoch, thus previous epoch validators set is used to construct report core assignment to pick expected guarantors.

	   multiple_reports 🟢
	   Multiple good work reports.

	   high_work_report_gas 🟢
	   Work report per core gas is very high, still less than the limit.

	   many_dependencies 🟢
	   Work report has many dependencies, still less than the limit.

	   reports_with_dependencies 🟢
	   Simple report dependency satisfied by another work report in the same extrinsic.

	   reports_with_dependencies 🟢
	   Work reports mutual dependency (indirect self-referential dependencies).

	   reports_with_dependencies 🟢
	   Work report direct self-referential dependency.

	   reports_with_dependencies 🟢
	   Work report dependency satisfied by recent blocks history.

	   reports_with_dependencies 🟢
	   Work report segments tree root lookup dependency satisfied by another work report in the same extrinsic.

	   reports_with_dependencies 🟢
	   Work report segments tree root lookup dependency satisfied by recent blocks history.

	   big_work_report_output 🟢
	   Work report output is very big, still less than the limit.
	*/
	testCases := []struct {
		jsonFile string
		except   error
		message  string
	}{
		{"report_curr_rotation-1.json", nil, "Report uses current guarantors rotation."},
		{"report_prev_rotation-1.json", nil, "Report uses previous guarantors rotation. Previous rotation falls within previous epoch, thus previous epoch validators set is used to construct report core assignment to pick expected guarantors."},
		{"multiple_reports-1.json", nil, "Multiple good work reports."},
		{"high_work_report_gas-1.json", nil, "Work report per core gas is very high, still less than the limit."},
		{"many_dependencies-1.json", nil, "Work report has many dependencies, still less than the limit."},
		{"reports_with_dependencies-1.json", nil, "Simple report dependency satisfied by another work report in the same extrinsic."},
		{"reports_with_dependencies-2.json", nil, "Work reports mutual dependency (indirect self-referential dependencies)."},
		{"reports_with_dependencies-3.json", nil, "Work report direct self-referential dependency."},
		{"reports_with_dependencies-4.json", nil, "Work report dependency satisfied by recent blocks history."},
		{"reports_with_dependencies-5.json", nil, "Work report segments tree root lookup dependency satisfied by another work report in the same extrinsic."},
		{"reports_with_dependencies-6.json", nil, "Work report segments tree root lookup dependency satisfied by recent blocks history."},
		{"big_work_report_output-1.json", nil, "Work report output is very big, still less than the limit."},
	}
	for _, tc := range testCases {
		t.Run(tc.jsonFile, func(t *testing.T) {
			err := ReportVerify(tc.jsonFile, tc.except)
			if err != nil {
				t.Fatalf("\nCASE: %v\n🟢[Expected] %v\n🔴[Actual]   %v\n", tc.jsonFile, tc.message, err)
			}
			fmt.Printf("===Finish %s ===\n", tc.jsonFile)
		})
	}
}
