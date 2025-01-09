package main

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
)

func readStateTransitions(dir string) (stfs []*statedb.StateTransition, err error) {
	fmt.Printf("fuzzing %v\n", dir)
	stfs = make([]*statedb.StateTransition, 0)
	state_transitions_dir := filepath.Join(dir, "state_transitions")
	stFiles, err := os.ReadDir(state_transitions_dir)
	if err != nil {
		panic(fmt.Sprintf("failed to read directory: %v\n", err))
		return stfs, fmt.Errorf("failed to read directory: %v", err)
	}
	for _, file := range stFiles {
		if strings.HasSuffix(file.Name(), ".bin") {
			// Extract epoch and phase from filename `${epoch}_${phase}.bin`
			parts := strings.Split(strings.TrimSuffix(file.Name(), ".bin"), "_")
			if len(parts) != 2 {
				log.Printf("Invalid block filename format: %s\n", file.Name())
				continue
			}

			// Read the st file
			stPath := filepath.Join(state_transitions_dir, file.Name())
			fmt.Printf("reading %v\n", stPath)
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
	fmt.Printf("Read %v state transitions\n", len(stfs))
	return stfs, nil
}

func testFuzz(t *testing.T, modes []string, stfs []*statedb.StateTransition) {
	testDir := "/tmp/fuzz"
	if _, err := os.Stat(testDir); os.IsNotExist(err) {
		err = os.MkdirAll(testDir, os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create directory /tmp/fuzz: %v", err)
		}
	}
	sdb_storage, err := storage.NewStateDBStorage(testDir)
	if err != nil {
		panic(fmt.Sprintf("Error with storage: %v", err))
		log.Fatalf("Error with storage: %v", err)
	}
	for _, stf := range stfs {
		//sdb, _ := statedb.NewStateDBFromSnapshotRaw(sdb_storage, &stf.PreState)
		mutatedStf, expectedErr, possibleErrs := selectImportBlocksError(sdb_storage, modes, stf)
		if expectedErr != nil {
			fmt.Printf("Expected error: %v | %v possibleErrs = %v\n", jamerrors.GetErrorStr(expectedErr), len(possibleErrs), jamerrors.GetErrorStrs(possibleErrs))
			errActual := statedb.CheckStateTransition(sdb_storage, mutatedStf, nil)
			if errActual == expectedErr {
				t.Logf("[fuzzed!] %v", jamerrors.GetErrorStr(errActual))
			} else {
				t.Errorf("[fuzzed failed!] Actual %v | Expected %v", jamerrors.GetErrorStr(errActual), jamerrors.GetErrorStr(expectedErr))
			}
		}
	}
}

func testFuzzSafrole(t *testing.T) {
	stfs, err := readStateTransitions(STF_Safrole)
	if err != nil {
		t.Fatalf("Error reading state transitions: %v", err)
	}
	testFuzz(t, []string{STF_Safrole}, stfs)
}

func testFuzzReports(t *testing.T) {
	stfs, err := readStateTransitions(STF_Reports)
	if err != nil {
		t.Fatalf("Error reading state transitions: %v", err)
	}
	testFuzz(t, []string{STF_Reports}, stfs)
}

func testFuzzAssurances(t *testing.T) {
	stfs, err := readStateTransitions(STF_Assurances)
	if err != nil {
		t.Fatalf("Error reading state transitions: %v", err)
	}
	testFuzz(t, []string{STF_Assurances}, stfs)
}

func testFuzzDisputes(t *testing.T) {
	stfs, err := readStateTransitions(STF_Disputes)
	if err != nil {
		t.Fatalf("Error reading state transitions: %v", err)
	}
	testFuzz(t, []string{STF_Disputes}, stfs)
}

func TestFuzz(t *testing.T) {
	testFuzzSafrole(t)
	testFuzzReports(t)
	testFuzzAssurances(t)
	//testFuzzDisputes(t)
}

func TestMKFuzz(t *testing.T) {
	//testFuzzSafrole(t)
	testFuzzAssurances(t)
	//testFuzzSafrole(t)
	//testFuzzReports(t)
}
