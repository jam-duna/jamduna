package fuzz

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/colorfulnotion/jam/jamerrors"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

const (
	STF_Safrole    = "safrole"
	STF_Disputes   = "disputes"
	STF_Fallback   = "fallback"
	STF_Guarantees = "guarantees"
	STF_Reports    = "reports"
	STF_Assurances = "assurances"
	STF_Generic    = "generic"
	Base_Dir       = "../cmd/importblocks/"
)

func readStateTransitions(baseDir, dir string) (stfs []*statedb.StateTransition, err error) {
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
			fmt.Printf("STF#%03d %v\n", file_idx, stPath)
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
			fmt.Printf("STF#%03d %s\n", file_idx, types.PrintObject(stf))
			stfs = append(stfs, &stf)
		}
	}
	fmt.Printf("Loaded %v state transitions\n", len(stfs))
	return stfs, nil
}

func readSTF(baseDir, targeted_mode string, t *testing.T) ([]*statedb.StateTransition, error) {
	stfs, err := readStateTransitions(baseDir, targeted_mode)
	if err == nil {
		return stfs, nil
	}
	fmt.Printf("%v-mode specific data unavailable.. using STF Generic data instead", targeted_mode)
	stfs_generic, err := readStateTransitions(baseDir, STF_Generic)
	if err != nil {
		t.Fatalf("Unable to test %v fuzzing: %v", targeted_mode, err)
		return nil, fmt.Errorf("Error reading state transitions for STF Generic: %v", err)
	}
	return stfs_generic, nil
}

func testFuzz(t *testing.T, modes []string, stfs []*statedb.StateTransition) {
	testDir := "/tmp/fuzzer_local"

	sdb_storage, err := InitFuzzStorage(testDir)
	if err != nil {
		t.Errorf("Error with storage: %v", err)
	}

	for _, stf := range stfs {
		//sdb, _ := statedb.NewStateDBFromSnapshotRaw(sdb_storage, &stf.PreState)
		mutatedStf, expectedErr, possibleErrs := selectImportBlocksError(sdb_storage, modes, stf)
		if expectedErr != nil || len(possibleErrs) > 0 {
			errActual := statedb.CheckStateTransition(sdb_storage, mutatedStf, nil)
			if errActual == expectedErr {
				//... do nothing
			} else {
				t.Errorf("[Fuzz Failed!!] Actual %v | Expected %v", jamerrors.GetErrorStr(errActual), jamerrors.GetErrorStr(expectedErr))
			}
		}
	}

	sdb_storage.Close()
}

func testFuzzSafrole(t *testing.T) {
	stfs, _ := readSTF(Base_Dir, STF_Safrole, t)
	testFuzz(t, []string{STF_Safrole}, stfs)
}

func testFuzzReports(t *testing.T) {
	stfs, _ := readSTF(Base_Dir, STF_Reports, t)
	testFuzz(t, []string{STF_Reports}, stfs)
}

func testFuzzAssurances(t *testing.T) {
	stfs, _ := readSTF(Base_Dir, STF_Assurances, t)
	testFuzz(t, []string{STF_Assurances}, stfs)
}

func testFuzzDisputes(t *testing.T) {
	stfs, _ := readSTF(Base_Dir, STF_Disputes, t)
	testFuzz(t, []string{STF_Disputes}, stfs)
}

func TestFuzz(t *testing.T) {
	testFuzzAssurances(t)
	//testFuzzSafrole(t)
	//testFuzzReports(t)
	//testFuzzDisputes(t)
}
