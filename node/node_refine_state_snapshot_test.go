package node

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"path/filepath"
	"reflect"
	"runtime"
	"runtime/pprof"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"text/tabwriter"
	"time"

	log "github.com/colorfulnotion/jam/log"
	refine "github.com/colorfulnotion/jam/refine"
	"github.com/colorfulnotion/jam/statedb"
	storage "github.com/colorfulnotion/jam/storage"
	telemetry "github.com/colorfulnotion/jam/telemetry"
	types "github.com/colorfulnotion/jam/types"
	"github.com/nsf/jsondiff"

	_ "net/http/pprof"
	"os"
)

const (
	algo_stf    = "test/00000028.bin"
	algo_bundle = "test/00000028_0x160796ba2f7702def36204c86172246ebf9d71f10e35f92dce734748b4ccdc4a_0_0_guarantor.bin"

	game_of_stf    = "test/03233539.bin"
	game_of_bundle = "test/03233538_0x8d63f7ce582bdf289283594871633c9018384b65f2699890d8321c5441b95c53_1_3233538_guarantor_follower.bin"
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

/*
	func TestCompareLogs(t *testing.T) {
		f1, err := os.Open("interpreter/10_refine.json")
		if err != nil {
			t.Fatalf("failed to open interpreter/vm_log.json: %v", err)
		}
		defer f1.Close()

		f2, err := os.Open("sandbox/10_refine.json")
		if err != nil {
			t.Fatalf("failed to open sandbox/vm_log.json: %v", err)
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
				t.Fatalf("error scanning vm_log_compiler.json at line %d: %v", i, err)
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
				t.Fatalf("log length mismatch at index %d: has vm_log=%v, has vm_log_compiler=%v", i, has1, has2)
			}

			// unmarshal each line into your entry type
			var orig, recp statedb.VMLog
			if err := json.Unmarshal(s1.Bytes(), &orig); err != nil {
				t.Fatalf("failed to unmarshal line %d of vm_log.json: %v", i, err)
			}
			if err := json.Unmarshal(s2.Bytes(), &recp); err != nil {
				t.Fatalf("failed to unmarshal line %d of vm_log_compiler.json: %v", i, err)
			}

			// compare
			if !reflect.DeepEqual(orig.OpStr, recp.OpStr) || !reflect.DeepEqual(orig.Operands, recp.Operands) || !reflect.DeepEqual(orig.Registers, recp.Registers) || !reflect.DeepEqual(orig.Gas, recp.Gas) {
				fmt.Printf("Difference at index %d:\nOriginal: %+v\nCompiler: %+v\n", i, orig, recp)
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
*/
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

func testRefineStateTransitions(t *testing.T, filename_stf, filename_bundle string) {

	statedb.RecordTime = true
	initPProf(t)

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

	fmt.Printf("Read bundle snapshot from file %s: %v\n", filename_bundle, types.ToJSONHexIndent(bundle_snapshot))

	levelDBPath := "/tmp/testdb"
	store, err := storage.NewStateDBStorage(levelDBPath, storage.NewMockJAMDA(), telemetry.NewNoOpTelemetryClient())
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	pvmBackends := []string{statedb.BackendInterpreter} //, statedb.BackendInterpreter
	//pvmBackends := []string{pvm.BackendInterpreter}
	for _, pvmBackend := range pvmBackends {
		if runtime.GOOS != "linux" && pvmBackend == statedb.BackendCompiler {
			t.Logf("pvmBackend=%s on non-linux platform NOT supported", pvmBackend)
			pvmBackend = statedb.BackendInterpreter
		}
		t.Run(fmt.Sprintf("pvmBackend=%s", pvmBackend), func(t *testing.T) {
			testRefineStateTransition(pvmBackend, store, bundle_snapshot, stf, t)
		})
	}
}

func DebugBundleSnapshotOLD(b *types.WorkPackageBundleSnapshot) {
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

func DebugBundleSnapshot(b *types.WorkPackageBundleSnapshot, gasUsed ...uint) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	showGasUsage := len(gasUsed) > 0

	if showGasUsage {
		fmt.Fprintln(w, "TYPE\tID\tN\tITER\tWORK ITEM\tSERVICE\tPAYLOAD\tREFINE GAS\tACCUMULATE GAS\tREFINE GAS USED\tGAS/ITER")
		fmt.Fprintln(w, "----\t--\t-\t----\t---------\t-------\t-------\t----------\t--------------\t---------------\t--------")
	} else {
		fmt.Fprintln(w, "TYPE\tID\tN\tITER\tWORK ITEM\tSERVICE\tPAYLOAD\tREFINE GAS\tACCUMULATE GAS")
		fmt.Fprintln(w, "----\t--\t-\t----\t---------\t-------\t-------\t----------\t--------------")
	}

	for workItemIdx, wi := range b.Bundle.WorkPackage.WorkItems {
		if workItemIdx == 0 { // AUTH row
			line := fmt.Sprintf("AUTH\t-\t-\t1\tWorkItem[%d]\t%d\t%x\t%d\t%d",
				workItemIdx, wi.Service, wi.Payload, wi.RefineGasLimit, wi.AccumulateGasLimit)
			if showGasUsage {
				line += "\t-\t-"
			}
			fmt.Fprintln(w, line)

		} else { // ALGO row
			if len(wi.Payload) >= 2 {
				algoID := uint(wi.Payload[0])
				n := uint(wi.Payload[1])
				algoIter := n * n * n

				// If gas usage is provided, calculate and print the extra info.
				if showGasUsage && algoIter > 0 {
					refineGasUsed := gasUsed[0]
					gasPerIter := refineGasUsed / uint(algoIter)
					fmt.Fprintf(w, "ALGO\t%d\t%d\t%d\tWorkItem[%d]\t%d\t%x\t%d\t%d\t%d\t%d\n",
						algoID, n, algoIter, workItemIdx, wi.Service, wi.Payload,
						wi.RefineGasLimit, wi.AccumulateGasLimit, refineGasUsed, gasPerIter)
				} else {
					// Otherwise, print the original line.
					fmt.Fprintf(w, "ALGO\t%d\t%d\t%d\tWorkItem[%d]\t%d\t%x\t%d\t%d\n",
						algoID, n, algoIter, workItemIdx, wi.Service, wi.Payload,
						wi.RefineGasLimit, wi.AccumulateGasLimit)
				}
			}
		}
	}
	w.Flush()
}

func TestRefineAlgo3(t *testing.T) {
	statedb.PvmLogging = false
	statedb.RecordTime = true
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
		backends := []string{statedb.BackendCompiler}
		for _, pvmBackend := range backends {
			if runtime.GOOS != "linux" {
				pvmBackend = statedb.BackendInterpreter
				log.Warn(log.Node, "compiler Not Supported. Defaulting to interpreter")
			}
			modified_wp := bundle_snapshot.Bundle.WorkPackage
			modified_wp.WorkItems[1].RefineGasLimit = 4_000_000_000
			if pickRandom {
				algo_payload := GenerateAlgoPayload(40, false)
				modified_wp.WorkItems[1].Payload = algo_payload
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
			store, err := storage.NewStateDBStorage(levelDBPath, storage.NewMockJAMDA(), telemetry.NewNoOpTelemetryClient())
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
			store, err := storage.NewStateDBStorage(levelDBPath, storage.NewMockJAMDA(), telemetry.NewNoOpTelemetryClient())
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

			algo_gas_used, algo_res_status, algo_res_err := testRefineStateTransition(pvmBackend, store, bundle_snapshot, stf, t)

			if algo_res_err != nil {
				done <- fmt.Errorf("critical execution error: %v", algo_res_err)
				return
			}

			if algo_res_status == "OK" {
				DebugBundleSnapshot(bundle_snapshot, algo_gas_used)
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

	success, _ := runTest(1)
	if !success {
		return 0 // Fails even at n=1
	}

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

	low, high := lastSuccess, bound
	finalSuccess := lastSuccess

	for low <= high {
		mid := low + (high-low)/2
		if mid == 0 {
			break
		}

		success, _ := runTest(mid)
		if success {
			finalSuccess = mid
			low = mid + 1
		} else {
			high = mid - 1
		}
	}

	return finalSuccess
}

func TestRefineAutoAlgo(t *testing.T) {
	statedb.PvmLogging = false
	statedb.RecordTime = true
	initPProf(t)

	stf, err := ReadStateTransition(algo_stf)
	if err != nil {
		t.Fatalf("failed to read state transition from file %s: %v", algo_stf, err)
	}

	var mu sync.Mutex
	algoSuccessLevels := make(map[uint8]int)

	algoID_start := 0
	algoID_end := 53
	for algoID := algoID_start; algoID <= algoID_end; algoID++ {
		t.Run(fmt.Sprintf("Algo_%d", algoID), func(t *testing.T) {
			fmt.Printf("--- Searching for last success for Algorithm ID: %d ---\n", algoID)
			pvmBackend := statedb.BackendCompiler
			if runtime.GOOS != "linux" {
				pvmBackend = statedb.BackendInterpreter
				log.Warn(log.Node, "compiler not supported - defaulting to interpreter")
			}

			lastN := findLastSuccessN(t, algoID, pvmBackend, stf)

			mu.Lock()
			algoSuccessLevels[uint8(algoID)] = lastN
			mu.Unlock()

			fmt.Printf("--- Result for Algo %d: Last successful n = %d ---\n", algoID, lastN)
		})
	}

	t.Log("All tests complete. Saving results to JSON.")

	keys := make([]int, 0, len(algoSuccessLevels))
	for k := range algoSuccessLevels {
		keys = append(keys, int(k))
	}
	sort.Ints(keys)

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
	//t.Logf("Testing refine state transition with pvmBackend: %s", pvmBackend)
	sdb, err := statedb.NewStateDBFromStateTransitionPost(store, stf)
	if err != nil {
		t.Fatalf("Failed to create state DB from state transition: %v", err)
	}

	id := uint16(3) // Simulated node ID
	simulatedNode := &Node{}
	simulatedNode.NodeContent = NewNodeContent(id, store, pvmBackend)
	simulatedNode.NodeContent.AddStateDB(sdb)

	// execute workpackage bundle
	re_workReport, _, wr_pvm_elapsed, reexecuted_snapshot, err := simulatedNode.executeWorkPackageBundle(uint16(bundle_snapshot.CoreIndex), bundle_snapshot.Bundle, bundle_snapshot.SegmentRootLookup, bundle_snapshot.Slot, false, 0)
	if err != nil {
		t.Fatalf("Error executing work package bundle: %v", err)
	}
	//t.Logf("[%s] Work Report[Hash: %v. PVM Elapsed: %v]\n%v\n", pvmBackend, re_workReport.Hash(), wr_pvm_elapsed, re_workReport.String())
	t.Logf("[%s] Work Report[Hash: %v. PVM Elapsed: %v]\n", pvmBackend, re_workReport.Hash(), wr_pvm_elapsed)
	if reexecuted_snapshot == nil {
		t.Fatalf("Reexecuted snapshot is nil")
	}
	i := 0
	if len(re_workReport.Results) > 1 {
		i = 1
	}
	algo_res := re_workReport.Results[i]
	algo_res_errCode := algo_res.Result.Err
	algo_res_status = types.ResultCode(algo_res_errCode)
	if algo_res_errCode != types.WORKDIGEST_OK {
		algo_res_err = fmt.Errorf("%s", types.ResultCode(algo_res_errCode))
	} else {
		algo_res_status = "OK"
	}
	algo_gasused = algo_res.GasUsed
	return algo_gasused, algo_res_status, algo_res_err
}

func testUnifiedRefineStateTransition(pvmBackend string, store *storage.StateDBStorage, bundle_snapshot *types.WorkPackageBundleSnapshot, stf *statedb.StateTransition, t *testing.T) (algo_gasused uint, algo_res_status string, algo_res_err error) {
	//t.Logf("Testing refine state transition using refine package with pvmBackend: %s", pvmBackend)
	sdb, err := statedb.NewStateDBFromStateTransitionPost(store, stf)
	if err != nil {
		t.Fatalf("Failed to create state DB from state transition: %v", err)
	}

	// Use refine package instead of node's executeWorkPackageBundle
	start := time.Now()
	reexecuted_snapshot, err := refine.ExecuteWorkPackageBundleV1(
		sdb,
		pvmBackend,
		sdb.JamState.SafroleState.Timeslot,
		uint16(bundle_snapshot.CoreIndex),
		bundle_snapshot.Bundle,
		bundle_snapshot.SegmentRootLookup,
		bundle_snapshot.Slot,
	)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Error executing work package bundle with refine package: %v", err)
	}

	t.Logf("[%s-REFINE] Work Report[Hash: %v. Total Elapsed: %v]\n", pvmBackend, reexecuted_snapshot.Report.Hash(), elapsed)

	i := 0
	if len(reexecuted_snapshot.Report.Results) > 1 {
		i = 1
	}
	algo_res := reexecuted_snapshot.Report.Results[i]
	algo_res_errCode := algo_res.Result.Err
	algo_res_status = types.ResultCode(algo_res_errCode)
	if algo_res_errCode != types.WORKDIGEST_OK {
		algo_res_err = fmt.Errorf("Result Code: %s", types.ResultCode(algo_res_errCode))
	} else {
		algo_res_status = "OK"
	}
	algo_gasused = algo_res.GasUsed
	return algo_gasused, algo_res_status, algo_res_err
}

func TestAlgoExecMax(t *testing.T) {
	statedb.PvmLogging = true
	statedb.RecordTime = true

	stf, err := ReadStateTransition(algo_stf)
	if err != nil {
		t.Fatalf("couldn't read state transition file %s: %v", algo_stf, err)
	}
	bundleSnapshot, err := ReadBundleSnapshot(algo_bundle)
	if err != nil {
		t.Fatalf("failed to read bundle snapshot: %v", err)
	}

	reportJSON, err := os.ReadFile("test/algo_success_report.json")
	if err != nil {
		t.Fatalf("couldn't read the report file test/algo_success_report.json: %v", err)
	}

	var successLevels map[string]int
	if err := json.Unmarshal(reportJSON, &successLevels); err != nil {
		t.Fatalf("couldn't parse the JSON report: %v", err)
	}

	// Extract and sort the algorithm IDs numerically.
	algoIDs := make([]int, 0, len(successLevels))
	for key := range successLevels {
		if algoID, err := strconv.Atoi(strings.TrimPrefix(key, "algo_")); err == nil {
			algoIDs = append(algoIDs, algoID)
		}
	}
	sort.Ints(algoIDs)

	for _, algoID := range algoIDs {
		key := fmt.Sprintf("algo_%d", algoID)
		n := successLevels[key]

		t.Run(fmt.Sprintf("Algo_%d_N_%d", algoID, n), func(t *testing.T) {
			fmt.Printf("\nRunning for Algo #%d (n=%d)\n", algoID, n)
			snapshot := bundleSnapshot
			wp := snapshot.Bundle.WorkPackage
			wp.WorkItems[1].Payload = []byte{byte(algoID), byte(n)}
			wp.WorkItems[0].RefineGasLimit = types.RefineGasAllocation / 5
			wp.WorkItems[1].RefineGasLimit = types.RefineGasAllocation/2 + 4*(types.RefineGasAllocation/5)
			snapshot.PackageHash = wp.Hash()
			snapshot.Bundle.WorkPackage = wp
			algo_gas_used := uint(0)

			output := captureOutput(func() {
				pvmBackend := statedb.BackendCompiler
				if runtime.GOOS != "linux" {
					pvmBackend = statedb.BackendInterpreter
				}

				levelDBPath := fmt.Sprintf("/tmp/summary-db-algo%d-n%d", algoID, n)
				store, err := storage.NewStateDBStorage(levelDBPath, storage.NewMockJAMDA(), telemetry.NewNoOpTelemetryClient())
				if err != nil {
					t.Fatalf("failed to create storage: %v", err)
				}
				defer os.RemoveAll(levelDBPath)
				algo_gas_used, _, _ = testRefineStateTransition(pvmBackend, store, snapshot, stf, t)
			})
			DebugBundleSnapshot(snapshot, algo_gas_used)

			fmt.Println(output)
			fmt.Println("----------------------------------------------------------------------------------------------------")
		})
	}
	t.Log("✅ Done generating execution summaries.")
}

func captureOutput(f func()) string {
	originalStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	w.Close()
	os.Stdout = originalStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func testRefineStateTransitionInvoke(pvmBackend string, store *storage.StateDBStorage, bundle_snapshot *types.WorkPackageBundleSnapshot, stf *statedb.StateTransition, t *testing.T) {
	//t.Logf("Testing refine state transition with pvmBackend: %s", pvmBackend)
	// log.InitLogger("debug")
	// log.EnableModule(log.PvmAuthoring)
	// log.EnableModule("pvm_validator")
	sdb, err := statedb.NewStateDBFromStateTransitionPost(store, stf)
	if err != nil {
		t.Fatalf("Failed to create state DB from state transition: %v", err)
	}
	total_simulated_nodes := uint16(1)
	nodes := make([]*Node, total_simulated_nodes)
	for id := uint16(0); id < total_simulated_nodes; id++ {
		nodes[id] = &Node{}
		nodes[id].NodeContent = NewNodeContent(id, store, pvmBackend)
		nodes[id].NodeContent.AddStateDB(sdb)
	}

	var wg sync.WaitGroup
	type result struct {
		workReport         *types.WorkReport
		pvmElapsed         time.Duration
		reexecutedSnapshot interface{}
		err                error
	}
	results := make([]result, total_simulated_nodes)
	startTimes := make([]time.Time, total_simulated_nodes)
	endTimes := make([]time.Time, total_simulated_nodes)

	for id := uint16(0); id < total_simulated_nodes; id++ {
		wg.Add(1)
		go func(idx uint16) {
			defer wg.Done()
			startTimes[idx] = time.Now()
			re_workReport, _, wr_pvm_elapsed, reexecuted_snapshot, err := nodes[idx].executeWorkPackageBundle(
				uint16(bundle_snapshot.CoreIndex),
				bundle_snapshot.Bundle,
				bundle_snapshot.SegmentRootLookup,
				bundle_snapshot.Slot,
				false,
				0,
			)
			endTimes[idx] = time.Now()
			results[idx] = result{
				workReport:         &re_workReport,
				pvmElapsed:         time.Duration(wr_pvm_elapsed),
				reexecutedSnapshot: reexecuted_snapshot,
				err:                err,
			}
		}(id)
	}
	wg.Wait()

	// Check for errors and compare results
	var refReport *types.WorkReport
	for i, res := range results {
		if res.err != nil {
			t.Fatalf("Node %d: Error executing work package bundle: %v", i, res.err)
		}
		if res.reexecutedSnapshot == nil {
			t.Fatalf("Node %d: Reexecuted snapshot is nil", i)
		}
		if i == 0 {
			refReport = res.workReport
		} else {
			if !reflect.DeepEqual(refReport, res.workReport) {
				t.Fatalf("Node %d: WorkReport mismatch with Node 0", i)
			}
		}
	}

	// Print execution times
	for i := 0; i < int(total_simulated_nodes); i++ {
		fmt.Printf("Node %d execution time: %v\n", i, endTimes[i].Sub(startTimes[i]))
	}
	t.Logf("All nodes produced identical results.")
}
func TestRefineInvokeLimit(t *testing.T) {
	statedb.PvmLogging = true
	statedb.RecordTime = true
	initPProf(t)
	filename_stf := game_of_stf
	filename_bundle := game_of_bundle

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
	store, err := storage.NewStateDBStorage(levelDBPath, storage.NewMockJAMDA(), telemetry.NewNoOpTelemetryClient())
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	//pvmBackends := []string{statedb.BackendInterpreter}
	pvmBackend := statedb.BackendInterpreter
	t.Run(fmt.Sprintf("pvmBackend=%s", pvmBackend), func(t *testing.T) {
		testRefineStateTransitionInvoke(pvmBackend, store, bundle_snapshot, stf, t)
	})

}

func testRefine(pvmBackend string, store *storage.StateDBStorage, bundle_snapshot *types.WorkPackageBundleSnapshot, stf *statedb.StateTransition, t *testing.T) (report types.WorkReport) {
	//t.Logf("Testing refine state transition with pvmBackend: %s", pvmBackend)
	sdb, err := statedb.NewStateDBFromStateTransitionPost(store, stf)
	if err != nil {
		t.Fatalf("Failed to create state DB from state transition: %v", err)
	}

	id := uint16(3) // Simulated node ID
	simulatedNode := &Node{}
	simulatedNode.NodeContent = NewNodeContent(id, store, pvmBackend)
	simulatedNode.NodeContent.AddStateDB(sdb)

	// execute workpackage bundle
	re_workReport, _, wr_pvm_elapsed, reexecuted_snapshot, err := simulatedNode.executeWorkPackageBundle(uint16(bundle_snapshot.CoreIndex), bundle_snapshot.Bundle, bundle_snapshot.SegmentRootLookup, bundle_snapshot.Slot, false, 0)
	if err != nil {
		t.Fatalf("Error executing work package bundle: %v", err)
	}
	//t.Logf("[%s] Work Report[Hash: %v. PVM Elapsed: %v]\n%v\n", pvmBackend, re_workReport.Hash(), wr_pvm_elapsed, re_workReport.String())
	t.Logf("[%s] Work Report[Hash: %v. PVM Elapsed: %v]\n", pvmBackend, re_workReport.Hash(), wr_pvm_elapsed)
	if reexecuted_snapshot == nil {
		t.Fatalf("Reexecuted snapshot is nil")
	}
	return re_workReport
}

func TestGenerateTestCase(t *testing.T) {
	type TestCase struct {
		Name           string
		BundleSnapShot *types.WorkPackageBundleSnapshot
	}

	stf_file := "test/game_of_life/state_transitions/state.bin"

	stf, err := ReadStateTransition(stf_file)
	if err != nil {
		t.Fatalf("failed to read state transition from file %s: %v", stf_file, err)
	}

	testCases := make([]TestCase, 0)
	bundlesDir := "test/game_of_life/bundle_snapshots"
	reportsDir := "test/game_of_life/reports"
	// check if the reports directory exists, if not create it
	if _, err := os.Stat(reportsDir); os.IsNotExist(err) {
		if err := os.MkdirAll(reportsDir, 0755); err != nil {
			t.Fatalf("failed to create reports directory %s: %v", reportsDir, err)
		}
		fmt.Printf("Created reports directory: %s\n", reportsDir)
	} else {
		fmt.Printf("Reports directory already exists: %s\n", reportsDir)
	}
	// for loop the files
	files, err := os.ReadDir(bundlesDir)
	if err != nil {
		t.Fatalf("failed to read directory %s: %v", bundlesDir, err)
	}

	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".bin") {
			filePath := filepath.Join(bundlesDir, file.Name())
			bundle_snapshot, err := ReadBundleSnapshot(filePath)
			if err != nil {
				t.Fatalf("failed to read bundle snapshot from file %s: %v", filePath, err)
			}
			testCases = append(testCases, TestCase{
				Name:           strings.TrimSuffix(file.Name(), ".bin"),
				BundleSnapShot: bundle_snapshot,
			})
		}
	}
	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			statedb.PvmLogging = false
			statedb.RecordTime = false
			initPProf(t)

			levelDBPath := fmt.Sprintf("/tmp/testdb-%s", testCase.Name)
			store, err := storage.NewStateDBStorage(levelDBPath, storage.NewMockJAMDA(), telemetry.NewNoOpTelemetryClient())
			if err != nil {
				t.Fatalf("Failed to create storage: %v", err)
			}

			report := testRefine(statedb.BackendInterpreter, store, testCase.BundleSnapShot, stf, t)
			fmt.Printf("Test case %s executed successfully with report: %v\n", testCase.Name, report.Hash().String())

			// write the report to the dir
			jsonFileName := fmt.Sprintf("%s/%s.json", reportsDir, testCase.Name)
			reportJSON, err := json.MarshalIndent(report, "", "  ")
			if err != nil {
				t.Fatalf("failed to marshal report to JSON: %v", err)
			}
			if err := os.WriteFile(jsonFileName, reportJSON, 0644); err != nil {
				t.Fatalf("failed to write report to file %s: %v", jsonFileName, err)
			}
			binFileName := fmt.Sprintf("%s/%s.bin", reportsDir, testCase.Name)
			reportBin, err := types.Encode(&report)
			if err != nil {
				t.Fatalf("failed to encode report to binary: %v", err)
			}
			if err := os.WriteFile(binFileName, reportBin, 0644); err != nil {
				t.Fatalf("failed to write report to file %s: %v", binFileName, err)
			}
			fmt.Printf("Report for test case %s written to %s and %s\n", testCase.Name, jsonFileName, binFileName)
			// Clean up the storage
			if err := store.Close(); err != nil {
				t.Fatalf("failed to close storage: %v", err)
			}
			if err := os.RemoveAll(levelDBPath); err != nil {
				t.Fatalf("failed to remove storage directory %s: %v", levelDBPath, err)
			}
			fmt.Printf("Cleaned up storage for test case %s\n", testCase.Name)
		})
	}
}

func TestAlgoRefineComparison(t *testing.T) {
	statedb.RecordTime = true
	initPProf(t)

	stf, err := ReadStateTransition(algo_stf)
	if err != nil {
		t.Fatalf("failed to read state transition from file %s: %v", algo_stf, err)
	}

	bundle_snapshot, err := ReadBundleSnapshot(algo_bundle)
	if err != nil {
		t.Fatalf("failed to read state transition from file %s: %v", algo_bundle, err)
	}

	levelDBPath := "/tmp/testdb_comparison"
	store, err := storage.NewStateDBStorage(levelDBPath, storage.NewMockJAMDA(), telemetry.NewNoOpTelemetryClient())
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	pvmBackend := statedb.BackendInterpreter

	t.Run("NodeExecuteWorkPackageBundle", func(t *testing.T) {
		testRefineStateTransition(pvmBackend, store, bundle_snapshot, stf, t)
	})

	t.Run("UnifiedRefineStateTransition", func(t *testing.T) {
		testUnifiedRefineStateTransition(pvmBackend, store, bundle_snapshot, stf, t)
	})
}

func TestAlgoRefine(t *testing.T) {
	testRefineStateTransitions(t, algo_stf, algo_bundle)
}
