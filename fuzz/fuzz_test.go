package fuzz

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/colorfulnotion/jam/jamerrors"
	"github.com/colorfulnotion/jam/pvm"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/storage"
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
	Base_Dir       = "../cmd/importblocks/rawdata/"
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

func testFuzz(t *testing.T, Base_Dir string, modes []string, stfs []*statedb.StateTransition) {
	testDir := "/tmp/fuzzer_local"
	sdb_storage, err := InitFuzzStorage(testDir)
	if err != nil {
		t.Errorf("Error with storage: %v", err)
	}
	dumpMode := modes[0]
	if len(modes) > 1 {
		dumpMode = "generic"
	}
	Cleanup(Base_Dir, dumpMode)
	seed := []byte("duna")
	for _, stf := range stfs {
		oSlot, oEpoch, oPhase, mutatedSTFs, possibleErrs := selectAllImportBlocksErrors(seed, sdb_storage, modes, stf)
		if len(possibleErrs) > 0 {
			for errIdx, expectedErr := range possibleErrs {
				mutatedStf := mutatedSTFs[errIdx]
				errActual := statedb.CheckStateTransition(sdb_storage, &mutatedStf, nil, pvm.BackendInterpreter)
				if errActual == expectedErr {
					// write as dump
					stChallenge := statedb.StateTransitionChallenge{
						PreState: mutatedStf.PreState,
						Block:    mutatedStf.Block,
					}
					writeDebug(Base_Dir, dumpMode, jamerrors.GetErrorCodeWithName(errActual), uint32(oEpoch), oPhase, stChallenge)
					fmt.Printf("[#%v e=%v,m=%03d] Fuzzed ready for dump %v\n", oSlot, oEpoch, oPhase, jamerrors.GetErrorCodeWithName(errActual))
				} else {
					t.Errorf("[#%v e=%v,m=%03d] Fuzz Failed!! Actual %v | Expected %v", oSlot, oEpoch, oPhase, jamerrors.GetErrorCodeWithName(errActual), jamerrors.GetErrorName(expectedErr))
				}
			}
		}
	}

	sdb_storage.Close()
}

func testFuzzGeneric(t *testing.T) {
	stfs, _ := readSTF(Base_Dir, STF_Generic, t)
	//mode_list := []string{STF_Safrole, STF_Assurances}
	mode_list := []string{STF_Safrole}
	testFuzz(t, Base_Dir, mode_list, stfs)
}

func testFuzzSafrole(t *testing.T) {
	stfs, _ := readSTF(Base_Dir, STF_Safrole, t)
	testFuzz(t, Base_Dir, []string{STF_Safrole}, stfs)
}

func testFuzzReports(t *testing.T) {
	stfs, _ := readSTF(Base_Dir, STF_Reports, t)
	testFuzz(t, Base_Dir, []string{STF_Reports}, stfs)
}

func testFuzzAssurances(t *testing.T) {
	stfs, _ := readSTF(Base_Dir, STF_Assurances, t)
	testFuzz(t, Base_Dir, []string{STF_Assurances}, stfs)
}

func testFuzzDisputes(t *testing.T) {
	stfs, _ := readSTF(Base_Dir, STF_Disputes, t)
	testFuzz(t, Base_Dir, []string{STF_Disputes}, stfs)
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
	testFuzz(t, Base_Dir, mode_list, combinedSTFs)
}

func TestFuzz(t *testing.T) {
	//testFuzzAll(t)
	testFuzzAssurances(t)
	testFuzzGeneric(t)
	//testFuzzSafrole(t)
	//testFuzzReports(t)
	//testFuzzDisputes(t)
}

func writeDebug(baseDir string, fuzz_mode string, jamErrorIdentifier string, epoch uint32, phase uint32, obj interface{}) error {
	l := storage.LogMessage{
		Payload: obj,
	}
	return WriteLog(baseDir, fuzz_mode, jamErrorIdentifier, epoch, phase, l)
}

func WriteLog(baseDir string, fuzz_mode string, jamErrorIdentifier string, epoch uint32, phase uint32, logMsg storage.LogMessage) error {
	obj := logMsg.Payload
	structDir := getFuzzDir(baseDir, fuzz_mode)
	// Check if the directories exist, if not create them
	if _, err := os.Stat(structDir); os.IsNotExist(err) {
		err := os.MkdirAll(structDir, os.ModePerm)
		if err != nil {
			return fmt.Errorf("Error creating %v directory: %v\n", fuzz_mode, err)
		}
	}
	//path := fmt.Sprintf("%s/%v_%03d_%v", structDir, epoch, phase, jamErrorIdentifier)
	timeSlot := epoch*types.EpochLength + phase
	path := fmt.Sprintf("%s/%08d_%v", structDir, timeSlot, jamErrorIdentifier)
	if obj != nil {
		types.SaveObject(path, obj, true)
		fmt.Printf("Saved %v to %v\n", fuzz_mode, path)
	}
	return nil
}

func Cleanup(baseDir string, fuzz_mode string) {
	fuzz_dir := getFuzzDir(baseDir, fuzz_mode)
	if _, err := os.Stat(fuzz_dir); err == nil {
		os.RemoveAll(fuzz_dir)
	}
}

func getFuzzDir(baseDir string, fuzz_mode string) string {
	parentDir := filepath.Dir(filepath.Dir(baseDir))
	fuzzedDir := filepath.Join(parentDir, "fuzzed")
	structDir := fmt.Sprintf("%s/%v", fuzzedDir, fuzz_mode)
	return structDir
}
