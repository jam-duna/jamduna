package fuzz

import (
	//"errors"

	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strings"

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
		errMsg := fmt.Sprintf("Mode Unknown. Must be one of %v", enabledModes)
		return false, fmt.Errorf(errMsg)
	}
	if found && !modeEnabled {
		errMsg := fmt.Sprintf("Mode Suppressed. Must be one of %v", enabledModes)
		return false, fmt.Errorf(errMsg)
	}
	return true, nil
}

func InitFuzzStorage(testDir string) (*storage.StateDBStorage, error) {
	if _, err := os.Stat(testDir); os.IsNotExist(err) {
		err = os.MkdirAll(testDir, os.ModePerm)
		if err != nil {
			return nil, fmt.Errorf("Failed to create directory /tmp/fuzz: %v", err)
		}
	}

	sdb_storage, err := storage.NewStateDBStorage(testDir)
	if err != nil {
		return nil, fmt.Errorf("Error with storage: %v", err)
	}
	return sdb_storage, nil

}

func ReadStateTransitionBIN(filename string) (stf *statedb.StateTransition, err error) {
	stBytes, err := os.ReadFile(filename)
	if err != nil {
		fmt.Printf("Error reading file %s: %v\n", filename, err)
		return nil, fmt.Errorf("Error reading file %s: %v", filename, err)
	}
	// Decode st from stBytes
	b, _, err := types.Decode(stBytes, reflect.TypeOf(statedb.StateTransition{}))
	if err != nil {
		fmt.Printf("Error decoding block %s: %v\n", filename, err)
		return nil, fmt.Errorf("Error decoding block %s: %v", filename, err)
	}
	st, ok := b.(statedb.StateTransition)
	if !ok {
		return nil, fmt.Errorf("failed to type assert decoded data to StateTransition; got type %T", b)
	}
	stf = &st // Assign the address of the resulting value to the pointer 'stf'.
	return stf, nil
}

func ReadStateTransitionJSON(path string) (*statedb.StateTransition, error) {
	file, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not read file %s: %w", path, err)
	}

	var st statedb.StateTransition
	if err := json.Unmarshal(file, &st); err != nil {
		return nil, fmt.Errorf("could not unmarshal json from %s: %w", path, err)
	}
	return &st, nil
}

func ReadStateTransition(filename string) (stf *statedb.StateTransition, err error) {
	if filename == "" {
		return nil, fmt.Errorf("filename cannot be empty")
	}
	if len(filename) > 0 && filename[len(filename)-4:] == ".bin" {
		return ReadStateTransitionBIN(filename)
	}
	return ReadStateTransitionJSON(filename)
}

func ReadStateTransitions(baseDir, dir string) (stfs []*statedb.StateTransition, err error) {
	stfs = make([]*statedb.StateTransition, 0)
	state_transitions_dir := filepath.Join(baseDir, dir, "state_transitions")
	stFiles, err := os.ReadDir(state_transitions_dir)
	if err != nil {
		return stfs, fmt.Errorf("failed to read directory: %v", err)
	}
	fmt.Printf("Selected Dir: %v\n", dir)
	file_idx := 0
	useJSON := true
	useBIN := true
	for _, file := range stFiles {
		//fmt.Printf("Processing file: %s\n", file.Name())
		if strings.HasSuffix(file.Name(), ".bin") || strings.HasSuffix(file.Name(), ".json") {
			stPath := filepath.Join(state_transitions_dir, file.Name())
			isJSON := strings.HasSuffix(file.Name(), ".json")
			isBin := strings.HasSuffix(file.Name(), ".bin")
			if useJSON && isJSON {
				//fmt.Printf("Reading JSON file: %s\n", stPath)
				stf, err := ReadStateTransition(stPath)
				if err != nil {
					log.Printf("Error reading state transition file %s: %v\n", file.Name(), err)
					continue
				}
				stfs = append(stfs, stf)
				file_idx++
			}
			if useBIN && isBin {
				fmt.Printf("Reading BIN file: %s\n", stPath)
				stf, err := ReadStateTransition(stPath)
				if err != nil {
					log.Printf("Error reading state transition file %s: %v\n", file.Name(), err)
					continue
				}
				stfs = append(stfs, stf)
				file_idx++
			}
		}
	}
	fmt.Printf("Loaded %v state transitions\n", len(stfs))
	return stfs, nil
}

func ReadStateTransitionsOLD(baseDir, dir string) (stfs []*statedb.StateTransition, err error) {
	stfs = make([]*statedb.StateTransition, 0)
	state_transitions_dir := filepath.Join(baseDir, dir, "state_transitions")
	stFiles, err := os.ReadDir(state_transitions_dir)
	if err != nil {
		return stfs, fmt.Errorf("failed to read directory: %v", err)
	}
	fmt.Printf("Selected Dir: %v\n", dir)
	file_idx := 0
	for _, file := range stFiles {
		if strings.HasSuffix(file.Name(), ".bin") {
			file_idx++
			// Extract epoch and phase from filename `${epoch}_${phase}.bin`
			// parts := strings.Split(strings.TrimSuffix(file.Name(), ".bin"), "_")

			// Read the st file
			stPath := filepath.Join(state_transitions_dir, file.Name())
			stBytes, err := os.ReadFile(stPath)
			if err != nil {
				log.Printf("Error reading block file %s: %v\n", file.Name(), err)
				continue
			}

			// Decode st from stBytes
			b, _, err := types.Decode(stBytes, reflect.TypeOf(statedb.StateTransition{}))
			if err != nil {
				log.Printf("Error decoding block %s: %v\n", file.Name(), err)
				continue
			}
			// Store the state transition in the stateTransitions map
			stf := b.(statedb.StateTransition)
			stfs = append(stfs, &stf)
		} else if strings.HasSuffix(file.Name(), ".json") {
			file_idx++
			// Read the st file
			stPath := filepath.Join(state_transitions_dir, file.Name())
			stBytes, err := os.ReadFile(stPath)
			if err != nil {
				log.Printf("Error reading block file %s: %v\n", file.Name(), err)
				continue
			}
			var st statedb.StateTransition
			err = json.Unmarshal(stBytes, &st)
			if err != nil {
				log.Printf("Error decoding block %s: %v\n", file.Name(), err)
				continue
			}
			stfs = append(stfs, &st)
		}
	}
	fmt.Printf("Loaded %v state transitions\n", len(stfs))
	return stfs, nil
}

type ExecutionReport struct {
	Seed            []byte               `json:"seed"`
	Block           types.Block          `json:"block"`
	PreState        statedb.StateKeyVals `json:"pre_state"`
	PostState       statedb.StateKeyVals `json:"post_state"`
	TargetPostState statedb.StateKeyVals `json:"target_result"`
	Error           string               `json:"error,omitempty"`
}

// MarshalJSON implements the json.Marshaler interface for ExecutionReport.
func (r *ExecutionReport) MarshalJSON() ([]byte, error) {
	type jsonExecutionReport struct {
		Seed            string               `json:"seed"`
		Block           types.Block          `json:"block"`
		PreState        statedb.StateKeyVals `json:"pre_state"`
		PostState       statedb.StateKeyVals `json:"post_state"`
		TargetPostState statedb.StateKeyVals `json:"target_result"`
		Error           string               `json:"error,omitempty"`
	}

	report := jsonExecutionReport{
		Seed:            fmt.Sprintf("0x%x", r.Seed),
		Block:           r.Block,
		PreState:        r.PreState,
		PostState:       r.PostState,
		TargetPostState: r.TargetPostState,
		Error:           r.Error,
	}

	return json.Marshal(report)
}
