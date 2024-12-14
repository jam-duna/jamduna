package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/jamerrors"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"
)

func readStateTransitions(dir string) (stfs []*statedb.StateTransition, err error) {
	stfs = make([]*statedb.StateTransition, 0)
	stFiles, err := os.ReadDir(dir)
	if err != nil {
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
			stPath := filepath.Join(dir, file.Name())
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
	return stfs, nil
}

func testFuzz(t *testing.T, modes []string, stfs []*statedb.StateTransition) {
	// TODO: hit all the errors with the stfs
	sdb_storage, err := storage.NewStateDBStorage("/tmp/fuzz")
	if err != nil {
		log.Fatalf("Error with storage: %v", err)
	}
	for _, stf := range stfs {
		//sdb, _ := statedb.NewStateDBFromSnapshotRaw(sdb_storage, &stf.PreState)
		err := selectImportBlocksError(modes, stf)
		if err != nil {
			errActual := statedb.CheckStateTransition(sdb_storage, stf, nil, common.Hash{})
			if errActual == err {
				t.Logf("Error %v is expected", err)
			} else {
				t.Errorf("Error %v is not expected", jamerrors.GetErrorStr(err))
			}
		}

	}
}

func testFuzzSafrole(t *testing.T) {
	stfs, _ := readStateTransitions("safrole")
	testFuzz(t, []string{"safrole"}, stfs)
}

func testFuzzReports(t *testing.T) {
	stfs, _ := readStateTransitions("assurances")
	testFuzz(t, []string{"reports"}, stfs)
}

func testFuzzAssurances(t *testing.T) {
	stfs, _ := readStateTransitions("assurances")
	testFuzz(t, []string{"assurances"}, stfs)
}

func testFuzzDisputes(t *testing.T) {
	stfs, _ := readStateTransitions("disputes")
	testFuzz(t, []string{"disputes"}, stfs)
}

func testFuzzPreimages(t *testing.T) {
	stfs, _ := readStateTransitions("disputes")
	testFuzz(t, []string{"preimages"}, stfs)
}

func TestFuzz(t *testing.T) {
	testFuzzSafrole(t)
	testFuzzReports(t)
	testFuzzAssurances(t)
	//testFuzzDisputes(t)
	//testFuzzPreimages(t)
}

func TestMKFuzz(t *testing.T) {
	testFuzzReports(t)
}
