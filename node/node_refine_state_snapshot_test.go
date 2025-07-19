package node

import (
	"fmt"
	"reflect"

	"testing"

	"github.com/colorfulnotion/jam/pvm"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/storage"

	"github.com/colorfulnotion/jam/types"

	_ "net/http/pprof"
	"os"
)

func ReadStateTransitions(filename string) (stf *statedb.StateTransition, err error) {
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

func ReadBundleSnapshot(filename string) (stf *types.WorkPackageBundleSnapshot, err error) {
	stBytes, err := os.ReadFile(filename)
	if err != nil {
		fmt.Printf("Error reading file %s: %v\n", filename, err)
		return nil, fmt.Errorf("Error reading file %s: %v", filename, err)
	}
	// Decode st from stBytes
	b, _, err := types.Decode(stBytes, reflect.TypeOf(types.WorkPackageBundleSnapshot{}))
	if err != nil {
		fmt.Printf("Error decoding block %s: %v\n", filename, err)
		return nil, fmt.Errorf("Error decoding block %s: %v", filename, err)
	}
	st, ok := b.(types.WorkPackageBundleSnapshot)
	if !ok {
		return nil, fmt.Errorf("failed to type assert decoded data to WorkPackageBundleSnapshot; got type %T", b)
	}
	stf = &st // Assign the address of the resulting value to the pointer 'stf'.
	return stf, nil
}

func TestRefineStateTransitions(t *testing.T) {
	filename_stf := "test/00000022.bin"
	filename_bundle := "test/00000022_0xbe539878a3f9f3bd949d6a77b3debbe15b3861c3596c4eb10a1087c5029f5f3e_1_3_guarantor.bin"

	stf, err := ReadStateTransitions(filename_stf)
	if err != nil {
		t.Fatalf("failed to read state transition from file %s: %v", filename_stf,
			err)
	}
	//fmt.Printf("Read state transition from file %s: %v\n", filename_stf, stf)
	bundle_snapshot, err := ReadBundleSnapshot(filename_bundle)
	if err != nil {
		t.Fatalf("failed to read state transition from file %s: %v", filename_bundle,
			err)
	}

	levelDBPath := "/tmp/testdb"
	store, err := storage.NewStateDBStorage(levelDBPath)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	pvmBackends := []string{pvm.BackendInterpreter, pvm.BackendRecompilerSandbox, pvm.BackendRecompiler}
	for _, pvmBackend := range pvmBackends {
		t.Run(fmt.Sprintf("pvmBackend=%s", pvmBackend), func(t *testing.T) {
			testRefineStateTransition(pvmBackend, store, bundle_snapshot, stf, t)
		})
	}
}

func testRefineStateTransition(pvmBackend string, store *storage.StateDBStorage, bundle_snapshot *types.WorkPackageBundleSnapshot, stf *statedb.StateTransition, t *testing.T) {
	t.Logf("Testing refine state transition with pvmBackend: %s", pvmBackend)
	sdb, err := statedb.NewStateDBFromStateTransitionPost(store, stf)
	if err != nil {
		t.Fatalf("Failed to create state DB from state transition: %v", err)
	}

	id := uint16(3) // Simulated node ID
	simulatedNode := &Node{}
	simulatedNode.NodeContent = NewNodeContent(id, store, pvmBackend)
	simulatedNode.NodeContent.AddStateDB(sdb)

	// // execute workpackage bundle
	re_workReport, _, wr_pvm_elapsed, reexecuted_snapshot, err := simulatedNode.executeWorkPackageBundle(uint16(bundle_snapshot.CoreIndex), bundle_snapshot.Bundle, bundle_snapshot.SegmentRootLookup, bundle_snapshot.Slot, false)
	if err != nil {
		t.Fatalf("Error executing work package bundle: %v", err)
	}
	t.Logf("[%s] Work Report[Hash: %v. PVM Elapsed: %v]\n%v\n", pvmBackend, re_workReport.Hash(), wr_pvm_elapsed, re_workReport.String())
	if reexecuted_snapshot == nil {
		t.Fatalf("Reexecuted snapshot is nil")
	}
}
