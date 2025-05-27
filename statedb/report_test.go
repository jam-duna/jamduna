//go:build testing
// +build testing

package statedb

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/jamerrors"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/trie"
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
	PriorServiceAccountState []ServiceItem                   `json:"accounts"`
	CoresStatistics          []types.CoreStatistics          `json:"cores_statistics"`
	ServiceStatistics        types.ServiceStatisticsKeyPairs `json:"services_statistics"`
}
type ServiceItem struct {
	ServiceID uint32      `json:"id"`
	Service   WrapService `json:"data"`
}
type WrapService struct {
	Service `json:"service"`
}
type Service struct {
	CodeHash   common.Hash `json:"code_hash"`
	Balance    uint64      `json:"balance"`
	MinItemGas uint64      `json:"min_item_gas"`
	MinMemoGas uint64      `json:"min_memo_gas"`
	CodeSize   uint64      `json:"bytes"`
	Items      uint32      `json:"items"`
}

func ServiceToSeviceAccount(s []ServiceItem) map[uint32]*types.ServiceAccount {
	result := make(map[uint32]*types.ServiceAccount)
	for _, value := range s {
		result[value.ServiceID] = &types.ServiceAccount{
			ServiceIndex:    value.ServiceID,
			CodeHash:        value.Service.CodeHash,
			Balance:         value.Service.Balance,
			GasLimitG:       value.Service.MinItemGas,
			GasLimitM:       value.Service.MinMemoGas,
			StorageSize:     value.Service.CodeSize,
			NumStorageItems: value.Service.Items,
			Mutable:         true,
			Storage:         make(map[common.Hash]*types.StorageObject),
			Lookup:          make(map[common.Hash]*types.LookupObject),
			Preimage:        make(map[common.Hash]*types.PreimageObject),
		}
		result[value.ServiceID].WriteLookup(value.Service.CodeHash, value.Service.Items, []uint32{})
	}
	return result
}

func (j *JamState) GetStateFromReportState(r StateReport) map[uint32]*types.ServiceAccount {
	j.AvailabilityAssignments = r.AvailabilityAssignments
	j.SafroleState.CurrValidators = r.CurrValidators
	j.SafroleState.PrevValidators = r.PrevValidators
	j.SafroleState.Entropy = r.Entropy
	j.RecentBlocks = r.RecentBlocks
	j.AuthorizationsPool = r.AuthorizationsPool
	var serviceAccounts = ServiceToSeviceAccount(r.PriorServiceAccountState)
	for i, core := range r.CoresStatistics {
		j.ValidatorStatistics.CoreStatistics[i] = core
	}
	j.ValidatorStatistics.ServiceStatistics = make(map[uint32]types.ServiceStatistics)
	for _, service_stats := range r.ServiceStatistics {
		j.ValidatorStatistics.ServiceStatistics[uint32(service_stats.ServiceIndex)] = service_stats.ServiceStatistics
	}
	return serviceAccounts
}

// give a json file and read it into a StateReport struct
func TestReportParsing(t *testing.T) {
	testCases := []struct {
		jsonFile     string
		binFile      string
		expectedType interface{}
	}{
		// {"not_sorted_guarantor-1.json", "not_sorted_guarantor-1.bin", &TestReport{}},
		// {"segment_root_lookup_invalid-1.json", "segment_root_lookup_invalid-1.bin", &TestReport{}},
		{"report_curr_rotation-1.json", "report_curr_rotation-1.bin", &TestReport{}},
	}
	for _, tc := range testCases {
		t.Run(tc.jsonFile, func(t *testing.T) {
			jsonPath := filepath.Join("../jamtestvectors/reports/tiny", tc.jsonFile)
			// binPath := filepath.Join("../jamtestvectors/safrole/tiny", tc.binFile)

			targetedStructType := reflect.TypeOf(tc.expectedType)

			// Read and unmarshal JSON file
			jsonData, err := os.ReadFile(jsonPath)
			if err != nil {
				t.Fatalf("failed to read JSON file: %v", err)
			}

			err = json.Unmarshal(jsonData, tc.expectedType)
			if err != nil {
				t.Fatalf("failed to unmarshal JSON data: %v", err)
			}
			// marshal the struct to JSON
			expectedJson, err := json.MarshalIndent(tc.expectedType, "", "  ")
			log.Trace(debugG, "Unmarshaled", "jsonPath", jsonPath, "Expected", "expected", expectedJson)
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
			fmt.Printf("encodedJSON=%s\n", encodedJSON)
			log.Trace(debugG, "TestReportParsing", "Encoded JSON", encodedJSON)

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

func ReportVerify(jsonFile string, exceptErr error) error {
	jsonPath := filepath.Join("../jamtestvectors/reports/", jsonFile)

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

	var db StateDB
	rand.Seed(time.Now().UnixNano()) // Seed the random number generator
	db_path := fmt.Sprintf("/tmp/testReport_%d", rand.Intn(100000000))

	sdb, err := storage.NewStateDBStorage(db_path)
	if err != nil {
		return fmt.Errorf("Reports FAIL: failed to create storage: %v", err)
	}
	db = *newEmptyStateDB(sdb)
	state := NewJamState()
	db.JamState = state
	services := db.JamState.GetStateFromReportState(report.PreState)
	for _, service := range services {
		_, err := db.writeAccount(service)
		if err != nil {
			return fmt.Errorf("Reports FAIL: failed to write account: %v", err)
		}
	}
	var block types.Block
	db.Block = &block
	db.JamState.SafroleState.Entropy = report.PreState.Entropy
	db.Block.Header.Slot = uint32(report.Input.Slot)
	db.JamState.SafroleState.Timeslot = uint32(report.Input.Slot)
	db.Block.Header.OffendersMark = report.PreState.Offenders
	db.Block.Extrinsic.Guarantees = report.Input.Guarantee
	db.RotateGuarantors()

	// Guarantees checks
	guarantees := db.Block.Extrinsic.Guarantees
	targetJCE := uint32(report.Input.Slot)
	s := db
	errors := make([]error, 0)
	for _, g := range guarantees {
		if e := s.VerifyGuaranteeBasic(g, targetJCE); e != nil {
			errors = append(errors, e)
		}
	}

	if e := CheckSortedGuarantees(guarantees); e != nil {
		errors = append(errors, e)
	}

	if e := s.checkLength(); e != nil && err == nil {
		errors = append(errors, e)
	}

	for _, g := range guarantees {
		if e := s.checkRecentWorkPackage(g, guarantees); e != nil && err == nil {
			errors = append(errors, e)
		}
		if e := s.checkPrereq(g, guarantees); e != nil && err == nil {
			errors = append(errors, e)
		}
	}

	if len(errors) > 0 && exceptErr == nil {
		fmt.Printf(report.Input.Guarantee[0].Report.String())
		return fmt.Errorf("Reports FAIL: failed to verify guarantee: %v", errors)
	}
	if len(errors) == 0 && exceptErr != nil {
		return fmt.Errorf("Reports FAIL: Expected have error:%v", exceptErr)
	}

	for _, err := range errors {

		if err != nil && exceptErr != nil {
			//check error prefix vs json file name
			// read string until the first '-'
			// if the prefix is not the same as the json file name, return error
			// if the prefix is the same as the json file name, return nil
			if err == exceptErr {
				return nil
			}
		}
	}
	if len(errors) > 0 {
		return fmt.Errorf("Reports FAIL: Expected error: %v, but get %v\n", exceptErr, errors)
	}
	post_state := NewJamState()
	post_state.GetStateFromReportState(report.PostState)
	db.JamState.ProcessGuarantees(context.TODO(), db.Block.Guarantees())
	db.JamState.tallyCoreStatistics(db.Block.Guarantees(), nil, nil)
	db.JamState.tallyServiceStatistics(db.Block.Guarantees(), nil, nil, nil)
	if exceptErr == nil {
		for i, rho := range db.JamState.AvailabilityAssignments {
			if !reflect.DeepEqual(rho, post_state.AvailabilityAssignments[i]) {
				return fmt.Errorf("Reports FAIL: AvailabilityAssignments not match")
			}
		}
		if !reflect.DeepEqual(db.JamState.ValidatorStatistics.CoreStatistics, post_state.ValidatorStatistics.CoreStatistics) {
			return fmt.Errorf("Reports FAIL: ValidatorStatistics CoreStatistics not match")
		}
		if !reflect.DeepEqual(db.JamState.ValidatorStatistics.ServiceStatistics, post_state.ValidatorStatistics.ServiceStatistics) {
			return fmt.Errorf("Reports FAIL: ValidatorStatistics ServiceStatistics not match")
		}
	}

	db.trie.Close()
	trie.DeleteLevelDB()
	return nil
}

func TestReportVerify(t *testing.T) {
	network_args := *network
	fmt.Printf("Report Test Case (Guraantee Verification), Network=%s\n", network_args)
	// run throgh all the json files in the tiny folder
	testCases := []struct {
		jsonFile string
		except   error
	}{
		{fmt.Sprintf("%s/bad_code_hash-1.json", network_args), jamerrors.ErrGBadCodeHash},
		{fmt.Sprintf("%s/bad_core_index-1.json", network_args), jamerrors.ErrGBadCoreIndex},
		{fmt.Sprintf("%s/bad_signature-1.json", network_args), jamerrors.ErrGBadSignature},
		{fmt.Sprintf("%s/core_engaged-1.json", network_args), jamerrors.ErrGCoreEngaged},
		{fmt.Sprintf("%s/dependency_missing-1.json", network_args), jamerrors.ErrGDependencyMissing},
		{fmt.Sprintf("%s/duplicated_package_in_report-1.json", network_args), jamerrors.ErrGDuplicatePackageTwoReports},
		{fmt.Sprintf("%s/future_report_slot-1.json", network_args), jamerrors.ErrGFutureReportSlot},
		{fmt.Sprintf("%s/no_enough_guarantees-1.json", network_args), jamerrors.ErrGInsufficientGuarantees},
		{fmt.Sprintf("%s/not_sorted_guarantor-1.json", network_args), jamerrors.ErrGDuplicateGuarantors},
		{fmt.Sprintf("%s/out_of_order_guarantees-1.json", network_args), jamerrors.ErrGOutOfOrderGuarantee},
		{fmt.Sprintf("%s/too_high_work_report_gas-1.json", network_args), jamerrors.ErrGWorkReportGasTooHigh},
		{fmt.Sprintf("%s/bad_validator_index-1.json", network_args), jamerrors.ErrGBadValidatorIndex},
		{fmt.Sprintf("%s/wrong_assignment-1.json", network_args), jamerrors.ErrGWrongAssignment},
		{fmt.Sprintf("%s/anchor_not_recent-1.json", network_args), jamerrors.ErrGAnchorNotRecent},
		{fmt.Sprintf("%s/bad_beefy_mmr-1.json", network_args), jamerrors.ErrGBadBeefyMMRRoot},
		{fmt.Sprintf("%s/bad_service_id-1.json", network_args), jamerrors.ErrGBadServiceID},
		{fmt.Sprintf("%s/bad_state_root-1.json", network_args), jamerrors.ErrGBadStateRoot},
		{fmt.Sprintf("%s/duplicate_package_in_recent_history-1.json", network_args), jamerrors.ErrGDuplicatePackageRecentHistory},
		{fmt.Sprintf("%s/report_before_last_rotation-1.json", network_args), jamerrors.ErrGReportEpochBeforeLast},
		{fmt.Sprintf("%s/segment_root_lookup_invalid-1.json", network_args), jamerrors.ErrGSegmentRootLookupInvalidNotRecentBlocks},
		{fmt.Sprintf("%s/segment_root_lookup_invalid-2.json", network_args), jamerrors.ErrGSegmentRootLookupInvalidUnexpectedValue},
		{fmt.Sprintf("%s/not_authorized-1.json", network_args), jamerrors.ErrGCoreWithoutAuthorizer},
		{fmt.Sprintf("%s/not_authorized-2.json", network_args), jamerrors.ErrGCoreUnexpectedAuthorizer},
		{fmt.Sprintf("%s/service_item_gas_too_low-1.json", network_args), jamerrors.ErrGServiceItemTooLow},
	}
	for _, tc := range testCases {
		t.Run(tc.jsonFile, func(t *testing.T) {
			err := ReportVerify(tc.jsonFile, tc.except)
			if err != nil {
				t.Fatalf("\033[31mReports FAIL: %v\033[0m", err)
			} else {
				fmt.Printf("\033[32mReports PASS: %s\033[0m\n", tc.jsonFile)
			}
		})
	}

}

// These cases should NOT return any errorduring STF check
func TestReportValidReportCase(t *testing.T) {
	network_args := *network
	fmt.Printf("Report Test Case (Valid Report), Network=%s\n", network_args)
	testCases := []struct {
		jsonFile string
		except   error
		message  string
	}{
		{fmt.Sprintf("%s/report_curr_rotation-1.json", network_args), nil, "Report uses current guarantors rotation."},
		{fmt.Sprintf("%s/report_prev_rotation-1.json", network_args), nil, "Report uses previous guarantors rotation. Previous rotation falls within previous epoch, thus previous epoch validators set is used to construct report core assignment to pick expected guarantors."},
		{fmt.Sprintf("%s/multiple_reports-1.json", network_args), nil, "Multiple good work reports."},
		{fmt.Sprintf("%s/high_work_report_gas-1.json", network_args), nil, "Work report per core gas is very high, still less than the limit."},
		{fmt.Sprintf("%s/many_dependencies-1.json", network_args), nil, "Work report has many dependencies, still less than the limit."},
		{fmt.Sprintf("%s/reports_with_dependencies-1.json", network_args), nil, "Simple report dependency satisfied by another work report in the same extrinsic."},
		{fmt.Sprintf("%s/reports_with_dependencies-2.json", network_args), nil, "Work reports mutual dependency (indirect self-referential dependencies)."},
		{fmt.Sprintf("%s/reports_with_dependencies-3.json", network_args), nil, "Work report direct self-referential dependency."},
		{fmt.Sprintf("%s/reports_with_dependencies-4.json", network_args), nil, "Work report dependency satisfied by recent blocks history."},
		{fmt.Sprintf("%s/reports_with_dependencies-5.json", network_args), nil, "Work report segments tree root lookup dependency satisfied by another work report in the same extrinsic."},
		{fmt.Sprintf("%s/reports_with_dependencies-6.json", network_args), nil, "Work report segments tree root lookup dependency satisfied by recent blocks history."},
		{fmt.Sprintf("%s/big_work_report_output-1.json", network_args), nil, "Work report output is very big, still less than the limit."},
	}
	for _, tc := range testCases {
		t.Run(tc.jsonFile, func(t *testing.T) {
			err := ReportVerify(tc.jsonFile, tc.except)
			if err != nil {
				t.Fatalf("\nCASE: %v\n🟢[Expected] %v\n🔴[Actual]   %v\n", tc.jsonFile, tc.message, err)
			}
			fmt.Printf("\033[32mReports PASS: %s\033[0m\n", tc.jsonFile)
		})
	}

}

// test functionality of the guarantee production

func TestGuaranteeProduction(t *testing.T) {
	// make four guarantees
	// two for core0, two for core1
	guarantees := make([]types.Guarantee, 4)
	for i := 0; i < 4; i++ {
		guarantees[i] = types.Guarantee{
			Report: types.WorkReport{
				CoreIndex: uint(i % 2),
			},
			Slot: uint32(i),
		}
	}
	// TODO: check this
	// guarantees = SortByCoreIndex(guarantees)
	fmt.Printf("len(guarantees)=%d\n", len(guarantees))
}
