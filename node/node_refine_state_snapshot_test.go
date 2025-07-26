package node

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math/rand"
	"reflect"
	"runtime"
	"runtime/pprof"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"text/tabwriter"
	"time"

	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/pvm"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"
	"github.com/nsf/jsondiff"

	_ "net/http/pprof"
	"os"
)

const (
	algo_stf    = "test/00000017.bin"
	algo_bundle = "test/00000017_0x0ca8503921ff22a02a7f216b76126e98ce1c2186b0312c279cb106eba8c9018f_0_3_guarantor.bin"
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

func ReadStateTransition(filename string) (stf *statedb.StateTransition, err error) {
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
	filename_stf := algo_stf
	filename_bundle := algo_bundle

	stf, err := ReadStateTransition(filename_stf)
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

func DebugBundleSnapshot(b *types.WorkPackageBundleSnapshot) {
	// Corrected tabwriter initialization and added "N" column
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "TYPE\tID\tN\tITER\tWORK ITEM\tSERVICE\tPAYLOAD\tREFINE GAS\tACCUMULATE GAS")
	fmt.Fprintln(w, "----\t--\t-\t----\t---------\t-------\t-------\t----------\t--------------")

	for workItemIdx, wi := range b.Bundle.WorkPackage.WorkItems {
		if workItemIdx == 0 {
			// Added placeholder for "N" column in AUTH row
			fmt.Fprintf(w, "AUTH\t-\t-\t1\tWorkItem[%d]\t%d\t%x\t%d\t%d\n",
				workItemIdx, wi.Service, wi.Payload, wi.RefineGasLimit, wi.AccumulateGasLimit)
		} else {
			algoPayload := wi.Payload
			if len(algoPayload) >= 2 {
				algoID := uint(algoPayload[0])
				n := uint(algoPayload[1])
				algoIter := n * n * n

				fmt.Fprintf(w, "ALGO\t%d\t%d\t%d\tWorkItem[%d]\t%d\t%x\t%d\t%d\n",
					algoID, n, algoIter, workItemIdx, wi.Service, wi.Payload, wi.RefineGasLimit, wi.AccumulateGasLimit)
			}
		}
	}
	w.Flush()
}

func TestRefineAlgo3(t *testing.T) {
	pvm.PvmLogging = false
	pvm.PvmTrace = false
	pvm.RecordTime = true
	initPProf(t)
	filename_stf := algo_stf
	filename_bundle := algo_bundle

	stf, err := ReadStateTransition(filename_stf)
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
	// each of these programs fails before {32, 64, 96, 128, 192, 255}
	failsBefore := make(map[int][]uint8)
	failsBefore[32] = []uint8{36, 53, 80, 81, 93, 95, 96, 125, 160, 161}
	failsBefore[64] = []uint8{5, 7, 10, 76, 78, 79, 82, 87}
	failsBefore[96] = []uint8{3, 4, 13, 27, 58, 73, 85, 149}
	failsBefore[128] = []uint8{16, 18, 29, 56, 57, 72, 74, 75, 104}
	failsBefore[192] = []uint8{15, 24, 26, 33, 55, 69, 77, 84, 134, 142}
	failsBefore[255] = []uint8{19, 59, 71, 88, 135, 136, 152}
	// Attempting 64K we fail on these before 3s
	// SIGILL
	failsBefore[26999] = []uint8{20, 21, 23, 25, 28, 30, 31, 32, 34, 35, 38, 39, 40, 44, 50, 51, 52, 61, 62, 63, 68, 70, 83, 91, 92, 94, 98, 111, 112, 115, 118, 119, 123, 124, 126, 127, 128, 129, 131, 134, 137, 138, 139, 144, 145, 148, 150, 151, 153, 154, 157, 166}
	// SIGSEGV
	failsBefore[27000] = []uint8{0, 2, 6, 11, 12}
	// smashing
	failsBefore[27001] = []uint8{12, 14, 17, 22, 45, 47, 113, 130, 140, 141, 143}
	// this worked at 40^3 but not at 70^3
	failsBefore[64000] = []uint8{43, 54, 67, 86, 105, 122, 156, 169}
	// this worked at 70^3 but not at 100^3
	failsBefore[421875] = []uint8{65, 66, 89, 99, 117, 132, 133, 147, 158}
	// this worked at 100^3 but not at 128^3
	failsBefore[2097152] = []uint8{42, 97, 103, 109, 110, 114, 146}
	pickRandom := true
	rand.Seed(time.Now().UnixNano())
	for i := 0; i < 170; i++ {
		willFail := false
		for _, skip := range failsBefore {
			if slices.Contains(skip, uint8(i)) {
				willFail = true
			}
		}
		if willFail {
			continue
		}
		gasUsed := make(map[string]uint)
		backends := []string{pvm.BackendRecompiler}
		for _, pvmBackend := range backends {
			modified_wp := bundle_snapshot.Bundle.WorkPackage
			modified_wp.WorkItems[1].RefineGasLimit = 4_000_000_000
			if pickRandom {
				algo_payload := GenerateAlgoPayload(40)
				modified_wp.WorkItems[1].Payload = algo_payload
				fmt.Printf("PAYLOAD %v %s\n", algo_payload, pvmBackend)
			} else {
				algo_payload := make([]byte, 2)
				algo_payload[0] = byte(i)
				algo_payload[1] = byte(128)

				modified_wp.WorkItems[1].Payload = algo_payload
				fmt.Printf("PROGRAM %d %s\n", algo_payload[0], pvmBackend)
			}
			wp_hash := modified_wp.Hash()
			bundle_snapshot.PackageHash = wp_hash
			bundle_snapshot.Bundle.WorkPackage = modified_wp
			levelDBPath := fmt.Sprintf("/tmp/testdb%d-%s", i, pvmBackend)
			store, err := storage.NewStateDBStorage(levelDBPath)
			if err != nil {
				t.Fatalf("Failed to create storage: %v", err)
			}
			algo_gasused, _, algo_res_err := testRefineStateTransition(pvmBackend, store, bundle_snapshot, stf, t)
			if algo_res_err != nil {
				fmt.Printf("ERR %s\n", err)
			}
			gasUsed[pvmBackend] = algo_gasused
		}
		fmt.Printf("Gas used for program %d: %v\n", i, gasUsed)
	}
}

/*
	type WorkItem struct {
		Service            uint32              `json:"service"`              // s: the identifier of the service to which it relates
		CodeHash           common.Hash         `json:"code_hash"`            // c: the code hash of the service at the time of reporting
		Payload            []byte              `json:"payload"`              // y: a payload blob
		RefineGasLimit     uint64              `json:"refine_gas_limit"`     // g: a refine gas limit
		AccumulateGasLimit uint64              `json:"accumulate_gas_limit"` // a: an accumulate gas limit
		ImportedSegments   []ImportSegment     `json:"import_segments"`      // i: a sequence of imported data segments
		Extrinsics         []WorkItemExtrinsic `json:"extrinsic"`            // x: extrinsic
		ExportCount        uint16              `json:"export_count"`         // e: the number of data segments exported by this work item
	}
*/

func findLastSuccessN(t *testing.T, algoID int, pvmBackend string, stf *statedb.StateTransition) int {
	// Helper function to run a single test for a given n.
	runTest := func(n int) (bool, error) {
		done := make(chan error, 1)
		timeout := 30 * time.Second // Timeout for a single iteration run

		go func() {
			bundle_snapshot, err := ReadBundleSnapshot(algo_bundle)
			if err != nil {
				done <- fmt.Errorf("failed to read bundle snapshot: %v", err)
				return
			}
			levelDBPath := fmt.Sprintf("/tmp/testdb%d-iter%d", algoID, n)
			store, err := storage.NewStateDBStorage(levelDBPath)
			if err != nil {
				done <- fmt.Errorf("failed to create storage: %v", err)
				return
			}
			defer os.RemoveAll(levelDBPath)

			algo_payload := []byte{byte(algoID), byte(n)} // pvm code handles n^3 internally
			modified_wp := bundle_snapshot.Bundle.WorkPackage
			modified_wp.WorkItems[1].Payload = algo_payload
			modified_wp.WorkItems[0].RefineGasLimit = types.RefineGasAllocation / 5
			modified_wp.WorkItems[1].RefineGasLimit = types.RefineGasAllocation/2 + 4*(types.RefineGasAllocation/5)
			bundle_snapshot.PackageHash = modified_wp.Hash()
			bundle_snapshot.Bundle.WorkPackage = modified_wp

			_, algo_res_status, algo_res_err := testRefineStateTransition(pvmBackend, store, bundle_snapshot, stf, t)

			if algo_res_err != nil {
				done <- fmt.Errorf("critical execution error: %v", algo_res_err)
				return
			}

			if algo_res_status == "OK" {
				DebugBundleSnapshot(bundle_snapshot)
				done <- nil // Success
			} else {
				DebugBundleSnapshot(bundle_snapshot)
				done <- fmt.Errorf("failed with status: %s", algo_res_status)
			}
		}()

		select {
		case err := <-done:
			if err != nil {
				fmt.Printf("  - Algo %d, n=%d: FAILED (%v)\n", algoID, n, err)
				return false, err
			}
			fmt.Printf("  - Algo %d, n=%d: SUCCESS\n", algoID, n)
			return true, nil
		case <-time.After(timeout):
			fmt.Printf("  - Algo %d, n=%d: FAILED (Timed out after %s)\n", algoID, n, timeout)
			return false, fmt.Errorf("timeout")
		}
	}

	// 1. Check base case n=1
	success, _ := runTest(1)
	if !success {
		return 0 // Fails even at n=1
	}

	// 2. Exponential search to find the failure boundary
	lastSuccess := 1
	bound := 2
	for bound <= 255 {
		success, _ := runTest(bound)
		if success {
			lastSuccess = bound
			bound *= 2
		} else {
			break // Found the failure boundary
		}
	}

	// 3. Binary search within the identified range [lastSuccess, bound]
	low, high := lastSuccess, bound
	finalSuccess := lastSuccess

	for low <= high {
		mid := low + (high-low)/2
		if mid == 0 {
			break
		} // Should not happen if we start low at 1

		success, _ := runTest(mid)
		if success {
			finalSuccess = mid // This is a potential answer
			low = mid + 1      // Try for a higher n
		} else {
			high = mid - 1 // Too high, reduce the search space
		}
	}

	return finalSuccess
}

func TestRefineAutoAlgo(t *testing.T) {
	pvm.PvmLogging = false
	pvm.PvmTrace = false
	pvm.RecordTime = true
	initPProf(t)

	stf, err := ReadStateTransition(algo_stf)
	if err != nil {
		t.Fatalf("failed to read state transition from file %s: %v", algo_stf, err)
	}

	var mu sync.Mutex
	algoSuccessLevels := make(map[uint8]int)

	algoID_start := 0
	algoID_end := 170
	for algoID := algoID_start; algoID <= algoID_end; algoID++ {
		t.Run(fmt.Sprintf("Algo_%d", algoID), func(t *testing.T) {
			fmt.Printf("--- Searching for last success for Algorithm ID: %d ---\n", algoID)
			pvmBackend := pvm.BackendRecompiler
			if runtime.GOOS != "linux" {
				pvmBackend = pvm.BackendInterpreter
				log.Warn(log.Node, fmt.Sprintf("COMPILER Not Supported. Defaulting to interpreter"))
			}

			lastN := findLastSuccessN(t, algoID, pvmBackend, stf)

			mu.Lock()
			algoSuccessLevels[uint8(algoID)] = lastN
			mu.Unlock()

			fmt.Printf("--- Result for Algo %d: Last successful n = %d ---\n", algoID, lastN)
		})
	}

	t.Log("All tests complete. Saving results to JSON.")

	// Extract and sort the map keys to ensure a deterministic JSON output order.
	keys := make([]int, 0, len(algoSuccessLevels))
	for k := range algoSuccessLevels {
		keys = append(keys, int(k))
	}
	sort.Ints(keys)

	// Manually construct the JSON object to guarantee the sorted key order.
	var sb strings.Builder
	sb.WriteString("{\n")
	for i, k := range keys {
		if i > 0 {
			sb.WriteString(",\n")
		}
		// Format as "algo_key": value
		sb.WriteString(fmt.Sprintf("  \"algo_%d\": %d", k, algoSuccessLevels[uint8(k)]))
	}
	sb.WriteString("\n}\n")
	jsonData := []byte(sb.String())

	err = os.WriteFile("test/algo_success_report.json", jsonData, 0644)
	if err != nil {
		t.Fatalf("Failed to write JSON report to file: %v", err)
	}

	t.Log("Successfully wrote analysis to algo_success_report.json")
}

func testRefineStateTransition(pvmBackend string, store *storage.StateDBStorage, bundle_snapshot *types.WorkPackageBundleSnapshot, stf *statedb.StateTransition, t *testing.T) (algo_gasused uint, algo_res_status string, algo_res_err error) {
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
	//t.Logf("[%s] Work Report[Hash: %v. PVM Elapsed: %v]\n%v\n", pvmBackend, re_workReport.Hash(), wr_pvm_elapsed, re_workReport.String())
	t.Logf("[%s] Work Report[Hash: %v. PVM Elapsed: %v]\n", pvmBackend, re_workReport.Hash(), wr_pvm_elapsed)
	if reexecuted_snapshot == nil {
		t.Fatalf("Reexecuted snapshot is nil")
	}
	algo_res := re_workReport.Results[1]
	algo_res_errCode := algo_res.Result.Err
	algo_res_status = types.ResultCode(algo_res_errCode)
	if algo_res_errCode != types.WORKRESULT_OK {
		algo_res_err = fmt.Errorf(types.ResultCode(algo_res_errCode))
	} else {
		algo_res_status = "OK"
	}
	algo_gasused = algo_res.GasUsed
	return algo_gasused, algo_res_status, algo_res_err
}
