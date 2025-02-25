package fuzz

import (
	//"errors"

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

type Fuzzer struct {
	sdbStorage *storage.StateDBStorage
}

func NewFuzzer(storageDir string) (*Fuzzer, error) {
	sdbStorage, err := storage.NewStateDBStorage(storageDir)
	if err != nil {
		return nil, err
	}
	return &Fuzzer{sdbStorage: sdbStorage}, nil
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

func (fs *Fuzzer) RunImplementationRPCServer() {
	runRPCServer(fs.sdbStorage)
}

func (fs *Fuzzer) RunInternalRPCServer() {
	runRPCServerInternal(fs.sdbStorage)
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

func ReadStateTransitions(baseDir, dir string) (stfs []*statedb.StateTransition, err error) {
	stfs = make([]*statedb.StateTransition, 0)
	state_transitions_dir := filepath.Join(baseDir, dir, "state_transitions")
	stFiles, err := os.ReadDir(state_transitions_dir)
	if err != nil {
		//panic(fmt.Sprintf("failed to read directory: %v\n", err))
		return stfs, fmt.Errorf("failed to read directory: %v", err)
	}
	fmt.Printf("Selected Dir: %v\n", dir)
	file_idx := 0
	for _, file := range stFiles {
		if strings.HasSuffix(file.Name(), ".bin") {
			file_idx++
			// Extract epoch and phase from filename `${epoch}_${phase}.bin`
			parts := strings.Split(strings.TrimSuffix(file.Name(), ".bin"), "_")
			if len(parts) != 2 {
				log.Printf("Invalid block filename format: %s\n", file.Name())
				continue
			}

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
		}
	}
	fmt.Printf("Loaded %v state transitions\n", len(stfs))
	return stfs, nil
}
