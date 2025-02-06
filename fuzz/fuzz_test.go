package fuzz

import (
	"fmt"
	"testing"

	"github.com/colorfulnotion/jam/jamerrors"
	"github.com/colorfulnotion/jam/statedb"
)

const (
	STF_Safrole    = "safrole"
	STF_Disputes   = "disputes"
	STF_Fallback   = "fallback"
	STF_Guarantees = "guarantees"
	STF_Reports    = "reports"
	STF_Assurances = "assurances"
	STF_Generic    = "generic"
	Base_Dir       = "../cmd/importblocks/data/"
)

func readSTF(baseDir, targeted_mode string, t *testing.T) ([]*statedb.StateTransition, error) {
	stfs, err := ReadStateTransitions(baseDir, targeted_mode)
	if err == nil {
		return stfs, nil
	}
	fmt.Printf("%v-mode specific data unavailable.. using STF Generic data instead", targeted_mode)
	stfs_generic, err := ReadStateTransitions(baseDir, STF_Generic)
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

func testFuzzAll(t *testing.T) {
	mode_list := []string{STF_Safrole, STF_Assurances}
	combinedSTFs := make([]*statedb.StateTransition, 0)
	for _, mode := range mode_list {
		stfs, _ := readSTF(Base_Dir, mode, t)
		if len(stfs) > 0 {
			combinedSTFs = append(combinedSTFs, stfs...)
		}
	}
	testFuzz(t, mode_list, combinedSTFs)
}

func TestFuzz(t *testing.T) {
	testFuzzAll(t)
	//testFuzzAssurances(t)
	//testFuzzSafrole(t)
	//testFuzzReports(t)
	//testFuzzDisputes(t)
}
