package fuzz

import (
	//"errors"

	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"
)

func (er *ExecutionReport) String() string {
	//jsonBytes, err := json.MarshalIndent(er, "", "  ")
	jsonBytes, err := json.Marshal(er)
	if err != nil {
		return fmt.Sprintf("Error marshaling ExecutionReport: %v", err)
	}
	return string(jsonBytes)
}

var fuzzModeList = map[string]bool{
	// Enabled by default:
	"assurances": true,
	"safrole":    true,

	// Planned to be enabled:
	"fallback":            true,
	"orderedaccumulation": true,

	// Good to have (under development):
	"authorization":      false,
	"recenthistory":      false,
	"blessed":            false,
	"basichostfunctions": false,
	"disputes":           false,
	"gas":                false,
	"finalization":       false,
}

func getModeStatus() (enabledModes []string, disabledModes []string) {
	enabledModes = make([]string, 0)
	disabledModes = make([]string, 0)
	for mode, enabled := range fuzzModeList {
		if enabled {
			enabledModes = append(enabledModes, mode)
		} else {
			disabledModes = append(disabledModes, mode)
		}
	}
	return enabledModes, disabledModes
}
func CheckModes(mode string) (bool, error) {
	enabledModes, _ := getModeStatus()
	modeEnabled, found := fuzzModeList[mode]
	if !found {
		return false, fmt.Errorf("mode Unknown. Must be one of %v", enabledModes)
	}
	if found && !modeEnabled {
		return false, fmt.Errorf("mode Suppressed. Must be one of %v", enabledModes)
	}
	return true, nil
}

func InitFuzzStorage(testDir string) (*storage.StateDBStorage, error) {
	if _, err := os.Stat(testDir); os.IsNotExist(err) {
		err = os.MkdirAll(testDir, os.ModePerm)
		if err != nil {
			return nil, fmt.Errorf("failed to create directory /tmp/fuzz: %v", err)
		}
	}

	sdb_storage, err := storage.NewStateDBStorage(testDir)
	if err != nil {
		return nil, fmt.Errorf("error with storage: %v", err)
	}
	return sdb_storage, nil

}

// DeduplicateStateTransitions removes duplicate state transitions based on prestate->poststate root pairs
// This handles cases where the same STF exists in both .bin and .json formats
func DeduplicateStateTransitions(stfs []*statedb.StateTransition) []*statedb.StateTransition {
	seen := make(map[string]*statedb.StateTransition)

	for _, stf := range stfs {
		// Create unique key from prestate and poststate roots
		preStateRoot := stf.PreState.StateRoot.Hex()
		postStateRoot := stf.PostState.StateRoot.Hex()
		key := preStateRoot + "->" + postStateRoot

		// first one wins for dedup
		if _, exists := seen[key]; !exists {
			seen[key] = stf
		}
	}

	// back to slice form
	deduplicated := make([]*statedb.StateTransition, 0, len(seen))
	for _, stf := range seen {
		deduplicated = append(deduplicated, stf)
	}

	return deduplicated
}

// Helper function to create deduplication key for consistency
func CreateSTFDeduplicationKey(stf *statedb.StateTransition) string {
	preStateRoot := stf.PreState.StateRoot.Hex()
	postStateRoot := stf.PostState.StateRoot.Hex()
	return preStateRoot + "->" + postStateRoot
}

type ExecutionReport struct {
	Seed                []byte               `json:"seed"`
	PostStateRoot       common.Hash          `json:"post_state_root"`
	TargetPostStateRoot common.Hash          `json:"target_post_state_root"`
	Block               types.Block          `json:"block"`
	PreState            statedb.StateKeyVals `json:"pre_state"`
	PostState           statedb.StateKeyVals `json:"post_state"`
	TargetPostState     statedb.StateKeyVals `json:"target_result"`
	Error               string               `json:"error,omitempty"`
}

// MarshalJSON implements the json.Marshaler interface for ExecutionReport.
func (r *ExecutionReport) MarshalJSON() ([]byte, error) {
	type jsonExecutionReport struct {
		Seed                string               `json:"seed"`
		PostStateRoot       string               `json:"post_state_root"`
		TargetPostStateRoot string               `json:"target_post_state_root"`
		Block               types.Block          `json:"block"`
		PreState            statedb.StateKeyVals `json:"pre_state"`
		PostState           statedb.StateKeyVals `json:"post_state"`
		TargetPostState     statedb.StateKeyVals `json:"target_result"`
		Error               string               `json:"error,omitempty"`
	}

	report := jsonExecutionReport{
		Seed:                fmt.Sprintf("0x%x", r.Seed),
		PostStateRoot:       fmt.Sprintf("0x%x", r.PostStateRoot),
		TargetPostStateRoot: fmt.Sprintf("0x%x", r.TargetPostStateRoot),
		Block:               r.Block,
		PreState:            r.PreState,
		PostState:           r.PostState,
		TargetPostState:     r.TargetPostState,
		Error:               r.Error,
	}

	return json.Marshal(report)
}

// --- Structs for the JSON Diff Report ---

type ValueDiff struct {
	Exp string `json:"exp"`
	Got string `json:"got"`
}

type KeyValDiff struct {
	Key  string    `json:"key"`
	Diff ValueDiff `json:"diff"`
}

type RootsDiff struct {
	Exp string `json:"exp"`
	Got string `json:"got"`
}

type JsonDiffReport struct {
	Target     PeerInfo     `json:"target"`
	Fuzzer     PeerInfo     `json:"fuzzer"`
	Mutated    bool         `json:"mutated"`
	MutatedErr string       `json:"mutated_err,omitempty"`
	Header     string       `json:"header,omitempty"`
	Roots      RootsDiff    `json:"roots"`
	KeyVals    []KeyValDiff `json:"keyvals"`
}

func (fs *Fuzzer) GenerateJsonDiffReport(report *ExecutionReport) *JsonDiffReport {
	expectedState := make(map[string]string)
	for _, kv := range report.PostState.KeyVals {
		keyHex := fmt.Sprintf("0x%x", kv.Key)
		valHex := fmt.Sprintf("0x%x", kv.Value)
		expectedState[keyHex] = valHex
	}

	actualState := make(map[string]string)
	for _, kv := range report.TargetPostState.KeyVals {
		keyHex := fmt.Sprintf("0x%x", kv.Key)
		valHex := fmt.Sprintf("0x%x", kv.Value)
		actualState[keyHex] = valHex
	}

	allKeys := make(map[string]struct{})
	for k := range expectedState {
		allKeys[k] = struct{}{}
	}
	for k := range actualState {
		allKeys[k] = struct{}{}
	}

	var diffs []KeyValDiff
	for key := range allKeys {
		expVal, expExists := expectedState[key]
		gotVal, gotExists := actualState[key]

		// A diff occurs if the values are different, or if a key exists in one state but not the other.
		if expVal != gotVal {
			diff := KeyValDiff{
				Key: key,
				Diff: ValueDiff{
					Exp: expVal, // Will be empty string if it didn't exist
					Got: gotVal, // Will be empty string if it didn't exist
				},
			}
			// Handle cases where a key is missing from one of the states
			if !expExists {
				diff.Diff.Exp = "null" // Or some other indicator for non-existence
			}
			if !gotExists {
				diff.Diff.Got = "null"
			}
			diffs = append(diffs, diff)
		}
	}

	sort.Slice(diffs, func(i, j int) bool {
		return diffs[i].Key < diffs[j].Key
	})

	diffReport := JsonDiffReport{
		Fuzzer:     fs.GetFuzzerInfo(),
		Target:     fs.GetTargetInfo(),
		MutatedErr: report.Error,
		Header:     report.Block.Header.HeaderHash().Hex(),
		Roots: RootsDiff{
			Exp: fmt.Sprintf("%v", report.PostStateRoot),
			Got: fmt.Sprintf("%v", report.TargetPostStateRoot),
		},
		KeyVals: diffs,
	}
	if report.Error != "" {
		diffReport.Mutated = true
	}
	return &diffReport
}

func (dr *JsonDiffReport) String() string {
	jsonBytes, err := json.MarshalIndent(dr, "", "  ")
	if err != nil {
		return fmt.Sprintf("Error marshaling JsonDiffReport: %v", err)
	}
	return string(jsonBytes)
}
