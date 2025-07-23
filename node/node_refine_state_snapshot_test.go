package node

import (
	"bufio"
	"encoding/json"
	"fmt"
	"reflect"
	"runtime/pprof"
	"testing"

	"github.com/colorfulnotion/jam/pvm"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/storage"
	"github.com/nsf/jsondiff"

	"github.com/colorfulnotion/jam/types"

	_ "net/http/pprof"
	"os"
)

func CompareJSON(obj1, obj2 interface{}) string {
	json1, err1 := json.Marshal(obj1)
	json2, err2 := json.Marshal(obj2)
	if err1 != nil || err2 != nil {
		return "Error marshalling JSON"
	}
	opts := jsondiff.DefaultJSONOptions()
	diff, diffStr := jsondiff.Compare(json1, json2, &opts)

	if diff == jsondiff.FullMatch {
		return "JSONs are identical"
	}
	return fmt.Sprintf("Diff detected:\n%s", diffStr)
}

func TestCompareLogs(t *testing.T) {
	f1, err := os.Open("interpreter/10_refine.json")
	if err != nil {
		t.Fatalf("failed to open interpreter/vm_log.json: %v", err)
	}
	defer f1.Close()

	f2, err := os.Open("recompiler_sandbox/10_refine.json")
	if err != nil {
		t.Fatalf("failed to open recompiler_sandbox/vm_log.json: %v", err)
	}
	defer f2.Close()

	s1 := bufio.NewScanner(f1)
	s2 := bufio.NewScanner(f2)

	var i int
	for {

		has1 := s1.Scan()
		has2 := s2.Scan()

		if err := s1.Err(); err != nil {
			t.Fatalf("error scanning vm_log.json at line %d: %v", i, err)
		}
		if err := s2.Err(); err != nil {
			t.Fatalf("error scanning vm_log_recompiler.json at line %d: %v", i, err)
		}

		// both files ended → success
		if !has1 && !has2 {
			break
		}
		if i == 0 {
			i++
			continue
		}
		// one ended early → length mismatch
		if has1 != has2 {
			t.Fatalf("log length mismatch at index %d: has vm_log=%v, has vm_log_recompiler=%v", i, has1, has2)
		}

		// unmarshal each line into your entry type
		var orig, recp pvm.VMLog
		if err := json.Unmarshal(s1.Bytes(), &orig); err != nil {
			t.Fatalf("failed to unmarshal line %d of vm_log.json: %v", i, err)
		}
		if err := json.Unmarshal(s2.Bytes(), &recp); err != nil {
			t.Fatalf("failed to unmarshal line %d of vm_log_recompiler.json: %v", i, err)
		}

		// compare
		if !reflect.DeepEqual(orig.OpStr, recp.OpStr) || !reflect.DeepEqual(orig.Operands, recp.Operands) || !reflect.DeepEqual(orig.Registers, recp.Registers) || !reflect.DeepEqual(orig.Gas, recp.Gas) {
			fmt.Printf("Difference at index %d:\nOriginal: %+v\nRecompiler: %+v\n", i, orig, recp)
			if diff := CompareJSON(orig, recp); diff != "" {
				fmt.Println("Differences:", diff)
				t.Fatalf("differences at index %d: %s", i, diff)
			}
		} else if i%100000 == 0 {
			fmt.Printf("Index %d: no difference %s\n", i, s1.Bytes())
		}
		i++
	}
}

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
func initPProf(t *testing.T) {
	// CPU profile
	cpuF, err := os.Create("cpu.pprof")
	if err != nil {
		t.Fatalf("could not create cpu profile: %v", err)
	}
	if err := pprof.StartCPUProfile(cpuF); err != nil {
		t.Fatalf("could not start cpu profile: %v", err)
	}
	// ensure we stop CPU profiling and close file
	t.Cleanup(func() {
		pprof.StopCPUProfile()
		cpuF.Close()
	})

	// heap profile
	memF, err := os.Create("mem.pprof")
	if err != nil {
		t.Fatalf("could not create mem profile: %v", err)
	}
	// write heap at end
	t.Cleanup(func() {
		if err := pprof.WriteHeapProfile(memF); err != nil {
			t.Fatalf("could not write heap profile: %v", err)
		}
		memF.Close()
	})
}

// go test -run=TestRefineStateTransitions
func TestRefineStateTransitions(t *testing.T) {
	pvm.PvmLogging = false
	pvm.PvmTrace = false
	pvm.RecordTime = true
	initPProf(t)
	filename_stf := "test/00000030.bin"
	filename_bundle := "test/00000031_0x7f8d1854158c7acaba1c3a74accf501a9be01d282c38c3eb141a9c6132faeaf5_0_0_guarantor.bin"

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
	pvmBackends := []string{pvm.BackendRecompiler} //, pvm.BackendInterpreter, pvm.BackendRecompilerSandbox}
	//pvmBackends := []string{pvm.BackendInterpreter}
	for _, pvmBackend := range pvmBackends {
		t.Run(fmt.Sprintf("pvmBackend=%s", pvmBackend), func(t *testing.T) {
			testRefineStateTransition(pvmBackend, store, bundle_snapshot, stf, t)
		})
	}
}

func TestRefineAlgo(t *testing.T) {
	pvm.PvmLogging = false
	pvm.PvmTrace = false
	pvm.RecordTime = false
	initPProf(t)
	filename_stf := "test/00000030.bin"
	filename_bundle := "test/00000031_0x0a698c35d2c4ce8282c7384d1a937ab7dbf5f20af537309299873fedb7cb7c2d_0_0_guarantor.bin"

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
	// each of these programs fails before {32, 64, 96, 128, 192, 255} .. the others all succeed at 255
	//    panics := []uint8{36, 53, 80, 81, 93, 95, 96, 125, 160, 161, 5, 7, 10, 76, 78, 79, 82, 87, 3, 4, 13, 27, 58, 73, 85, 149, 16, 18, 29, 56, 57, 72, 74, 75, 104, 15, 24, 26, 33, 55, 69, 77, 84, 134, 142, 19, 59, 71, 88, 135, 136, 152}
	panics := []uint8{53}
	for _, i := range panics {
		gasUsed := make(map[string]uint)
		backends := []string{pvm.BackendInterpreter}
		for _, pvmBackend := range backends {
			algo_payload := make([]byte, 2)
			algo_payload[0] = byte(i)
			algo_payload[1] = byte(255)
			modified_wp := bundle_snapshot.Bundle.WorkPackage
			modified_wp.WorkItems[1].Payload = algo_payload
			wp_hash := modified_wp.Hash()
			bundle_snapshot.PackageHash = wp_hash
			bundle_snapshot.Bundle.WorkPackage = modified_wp
			fmt.Printf("PROGRAM %d %s\n", algo_payload[0], pvmBackend)
			levelDBPath := fmt.Sprintf("/tmp/testdb%d-%s", i, pvmBackend)
			store, err := storage.NewStateDBStorage(levelDBPath)
			if err != nil {
				t.Fatalf("Failed to create storage: %v", err)
			}
			gasUsed[pvmBackend] = testRefineStateTransition(pvmBackend, store, bundle_snapshot, stf, t)
		}
		fmt.Printf("Gas used for program %d: %v\n", i, gasUsed)
	}
}

func testRefineStateTransition(pvmBackend string, store *storage.StateDBStorage, bundle_snapshot *types.WorkPackageBundleSnapshot, stf *statedb.StateTransition, t *testing.T) uint {
	t.Logf("Testing refine state transition with pvmBackend: %s", pvmBackend)
	sdb, err := statedb.NewStateDBFromStateTransitionPost(store, stf)
	if err != nil {
		t.Fatalf("Failed to create state DB from state transition: %v", err)
	}

	id := uint16(3) // Simulated node ID
	simulatedNode := &Node{}
	simulatedNode.NodeContent = NewNodeContent(id, store, pvmBackend)
	simulatedNode.NodeContent.AddStateDB(sdb)

	// execute workpackage bundle
	re_workReport, _, wr_pvm_elapsed, reexecuted_snapshot, err := simulatedNode.executeWorkPackageBundle(uint16(bundle_snapshot.CoreIndex), bundle_snapshot.Bundle, bundle_snapshot.SegmentRootLookup, bundle_snapshot.Slot, false)
	if err != nil {
		t.Fatalf("Error executing work package bundle: %v", err)
	}
	t.Logf("[%s] Work Report[Hash: %v. PVM Elapsed: %v]\n%v\n", pvmBackend, re_workReport.Hash(), wr_pvm_elapsed, re_workReport.String())
	if reexecuted_snapshot == nil {
		t.Fatalf("Reexecuted snapshot is nil")
	}
	gasused := re_workReport.Results[1].GasUsed
	return gasused
}
