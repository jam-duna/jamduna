package statedb

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/pvm"
	"github.com/colorfulnotion/jam/types"
)

//	func printColoredJSONDiff(diffStr string) {
//		for _, line := range strings.Split(diffStr, "\n") {
//			switch {
//			case strings.HasPrefix(line, "-"):
//				fmt.Println(colorRed + line + colorReset)
//			case strings.HasPrefix(line, "+"):
//				fmt.Println(colorGreen + line + colorReset)
//			default:
//				fmt.Println(line)
//			}
//		}
//	}
func runSingleSTFTest(t *testing.T, filename string, content string, pvmBackend string) {
	t.Helper()

	testDir := "/tmp/test_locala"
	test_storage, err := initStorage(testDir)
	if err != nil {
		t.Errorf("❌ [%s] Error initializing storage: %v", filename, err)
		return
	}
	defer test_storage.Close()

	stf, err := parseSTFFile(filename, content)
	if err != nil {
		t.Errorf("❌ [%s] Failed to parse STF: %v", filename, err)
		return
	}

	diffs, err := CheckStateTransitionWithOutput(test_storage, &stf, nil, pvmBackend)
	if err == nil {
		fmt.Printf("✅ [%s] PostState.StateRoot %s matches\n", filename, stf.PostState.StateRoot)
		return
	}

	HandleDiffs(diffs)
	t.Errorf("❌ [%s] Test failed: %v", filename, err)
}

func parseSTFFile(filename, content string) (StateTransition, error) {
	var stf StateTransition
	var err error
	if strings.HasSuffix(filename, ".bin") {
		stf0, _, err := types.Decode([]byte(content), reflect.TypeOf(StateTransition{}))
		if err == nil {
			stf = stf0.(StateTransition)
		}
	} else {
		err = json.Unmarshal([]byte(content), &stf)
	}
	return stf, err
}

func TestStateTransitionNoSandbox(t *testing.T) {
	pvm.PvmLogging = true
	pvm.PvmTrace = true                // enable PVM trace for this test
	pvm.VMsCompare = true              // enable VM comparison for this test
	filename := "traces/00000019.json" // javajam 0.6.7 -- see https://github.com/jam-duna/jamtestnet/issues/231#issuecomment-3144487797
	//filename = "../jamtestvectors/traces/storage_light/00000016.json"
	filename = "../jamtestvectors/traces/preimages/00000057.json"
	filename = "../jamtestvectors/traces/storage/00000060.json"

	content, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("failed to read file %s: %v", filename, err)
	}
	log.InitLogger("debug")
	log.EnableModule(log.PvmAuthoring)
	log.EnableModule("pvm_validator")
	t.Run(filepath.Base(filename), func(t *testing.T) {
		runSingleSTFTest(t, filename, string(content), pvm.BackendInterpreter)
	})
}

func TestStateTransitionSandbox(t *testing.T) {
	pvm.VMsCompare = true // enable VM comparison for this test
	pvm.PvmLogging = true
	pvm.UseTally = false         // enable tally for this test
	pvm.SetUseEcalli500(false)   // use ecalli500 for log check in x86
	pvm.SetDebugRecompiler(true) // enable debug mode for recompiler
	filename := "./00000019.json"
	filename = "../jamtestvectors/traces/storage/00000060.json"
	// filename = "../jamtestvectors/traces/storage_light/00000016.json"

	content, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("failed to read file %s: %v", filename, err)
	}
	log.InitLogger("debug")
	log.EnableModule(log.PvmAuthoring)
	log.EnableModule("pvm_validator")
	t.Run(filepath.Base(filename), func(t *testing.T) {
		runSingleSTFTest(t, filename, string(content), pvm.BackendRecompilerSandbox)

	})
}

func TestStateTransitionRecompiler(t *testing.T) {
	pvm.VMsCompare = true // enable VM comparison for this test
	pvm.PvmLogging = true
	pvm.UseTally = false         // enable tally for this test
	pvm.SetUseEcalli500(false)   // use ecalli500 for log check in x86
	pvm.SetDebugRecompiler(true) // enable debug mode for recompiler
	filename := "./00000019.json"
	filename = "../jamtestvectors/traces/storage_light/00000016.json"

	content, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("failed to read file %s: %v", filename, err)
	}
	log.InitLogger("debug")
	log.EnableModule(log.PvmAuthoring)
	log.EnableModule("pvm_validator")
	t.Run(filepath.Base(filename), func(t *testing.T) {
		runSingleSTFTest(t, filename, string(content), pvm.BackendRecompiler)

	})
}

func TestPVMstepJsonDiff(t *testing.T) {
	var testdata, testdata_rcp pvm.VMLogs

	// Load JSON 1
	json1, err := os.ReadFile("interpreter/vm_log.json")
	if err != nil {
		t.Fatalf("failed to read file test_case/vm_log.json: %v", err)
	}
	err = json.Unmarshal(json1, &testdata)
	if err != nil {
		t.Fatalf("failed to unmarshal test_case/vm_log.json: %v", err)
	}

	// Load JSON 2
	json2, err := os.ReadFile("recompiler_sandbox/vm_log.json")
	if err != nil {
		t.Fatalf("failed to read file test_case/vm_log_recompiler.json: %v", err)
	}
	err = json.Unmarshal(json2, &testdata_rcp)
	if err != nil {
		t.Fatalf("failed to unmarshal test_case/vm_log_recompiler.json: %v", err)
	}

	for i, ans := range testdata {
		// find the difference in the two JSONs
		if i >= len(testdata_rcp) {
			t.Fatalf("len(testdata_rcp)=%d len(testdata)=%d", len(testdata_rcp), len(testdata))
		}
		rcp_ans := testdata_rcp[i]
		if !reflect.DeepEqual(ans, rcp_ans) {
			fmt.Printf("Difference found in index %d:\n", i)
			fmt.Printf("Original: %+v\n", ans)
			fmt.Printf("Recompiler: %+v\n", rcp_ans)

			// Print the differences
			diff := CompareJSON(ans, rcp_ans)
			if diff != "" {
				fmt.Println("Differences:", diff)
				t.Fatalf("Differences found in index %d: %s", i, diff)

			}
		}
	}
}

func TestTraces(t *testing.T) {
	log.InitLogger("debug")
	pvm.VMsCompare = true // enable VM comparison for this test
	// pvm.PvmLogging = true

	// Define all the directories you want to test in a single slice.
	testDirs := []string{
		//"../cmd/importblocks/rawdata/safrole/state_transitions/",
		"../jamtestvectors/traces/fallback",
		"../jamtestvectors/traces/safrole",
		"../jamtestvectors/traces/preimages_light",
		"../jamtestvectors/traces/preimages",
		"../jamtestvectors/traces/storage_light",
		"../jamtestvectors/traces/storage",
	}

	// Iterate over each directory.
	for _, dir := range testDirs {
		// Create a local copy of dir for the sub-test to capture correctly.
		// This avoids issues where the sub-tests might all run with the last value of 'dir'.
		currentDir := dir

		t.Run(fmt.Sprintf("Directory_%s", filepath.Base(currentDir)), func(t *testing.T) {
			entries, err := os.ReadDir(currentDir)
			if err != nil {
				// Use t.Fatalf to stop the test for this directory if we can't read it.
				t.Fatalf("failed to read directory %s: %v", currentDir, err)
			}

			for _, e := range entries {
				if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") || e.Name() == "00000000.json" {
					continue
				}

				filename := filepath.Join(currentDir, e.Name())
				content, err := os.ReadFile(filename)
				if err != nil {
					// Use t.Errorf to report the error but continue with other files.
					t.Errorf("failed to read file %s: %v", filename, err)
					continue
				}

				fmt.Printf("Running test for file: %s\n", filename)

				// Run the actual test logic for each file as a distinct sub-test.
				t.Run(e.Name(), func(t *testing.T) {
					runSingleSTFTest(t, filename, string(content), pvm.BackendInterpreter)
				})
			}
		})
	}
}

func TestTracesRecompiler(t *testing.T) {
	log.InitLogger("debug")
	pvm.VMsCompare = true // enable VM comparison for this test
	// pvm.PvmLogging = true

	// Define all the directories you want to test in a single slice.
	testDirs := []string{
		//"../cmd/importblocks/rawdata/safrole/state_transitions/",
		// "../jamtestvectors/traces/fallback",
		// "../jamtestvectors/traces/safrole",
		"../jamtestvectors/traces/preimages_light",
		"../jamtestvectors/traces/preimages",
		"../jamtestvectors/traces/storage_light",
		"../jamtestvectors/traces/storage",
	}

	// Iterate over each directory.
	for _, dir := range testDirs {
		// Create a local copy of dir for the sub-test to capture correctly.
		// This avoids issues where the sub-tests might all run with the last value of 'dir'.
		currentDir := dir

		t.Run(fmt.Sprintf("Directory_%s", filepath.Base(currentDir)), func(t *testing.T) {
			entries, err := os.ReadDir(currentDir)
			if err != nil {
				// Use t.Fatalf to stop the test for this directory if we can't read it.
				t.Fatalf("failed to read directory %s: %v", currentDir, err)
			}

			for _, e := range entries {
				if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") || e.Name() == "00000000.json" {
					continue
				}

				filename := filepath.Join(currentDir, e.Name())
				content, err := os.ReadFile(filename)
				if err != nil {
					// Use t.Errorf to report the error but continue with other files.
					t.Errorf("failed to read file %s: %v", filename, err)
					continue
				}

				fmt.Printf("Running test for file: %s\n", filename)

				// Run the actual test logic for each file as a distinct sub-test.
				t.Run(e.Name(), func(t *testing.T) {
					runSingleSTFTest(t, filename, string(content), pvm.BackendRecompiler)
				})
			}
		})
	}
}
func TestTracesSandbox(t *testing.T) {
	log.InitLogger("debug")
	pvm.VMsCompare = true // enable VM comparison for this test
	// pvm.PvmLogging = true

	// Define all the directories you want to test in a single slice.
	testDirs := []string{
		//"../cmd/importblocks/rawdata/safrole/state_transitions/",
		// "../jamtestvectors/traces/fallback",
		// "../jamtestvectors/traces/safrole",
		"../jamtestvectors/traces/preimages_light",
		"../jamtestvectors/traces/preimages",
		"../jamtestvectors/traces/storage_light",
		"../jamtestvectors/traces/storage",
	}

	// Iterate over each directory.
	for _, dir := range testDirs {
		// Create a local copy of dir for the sub-test to capture correctly.
		// This avoids issues where the sub-tests might all run with the last value of 'dir'.
		currentDir := dir

		t.Run(fmt.Sprintf("Directory_%s", filepath.Base(currentDir)), func(t *testing.T) {
			entries, err := os.ReadDir(currentDir)
			if err != nil {
				// Use t.Fatalf to stop the test for this directory if we can't read it.
				t.Fatalf("failed to read directory %s: %v", currentDir, err)
			}

			for _, e := range entries {
				if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") || e.Name() == "00000000.json" {
					continue
				}

				filename := filepath.Join(currentDir, e.Name())
				content, err := os.ReadFile(filename)
				if err != nil {
					// Use t.Errorf to report the error but continue with other files.
					t.Errorf("failed to read file %s: %v", filename, err)
					continue
				}

				fmt.Printf("Running test for file: %s\n", filename)

				// Run the actual test logic for each file as a distinct sub-test.
				t.Run(e.Name(), func(t *testing.T) {
					runSingleSTFTest(t, filename, string(content), pvm.BackendRecompilerSandbox)
				})
			}
		})
	}
}
func TestCompareJson(t *testing.T) {
	var testdata1 types.Validator
	var testdata2 types.Validator
	testdata1 = types.Validator{
		Ed25519: types.HexToEd25519Key("0x1"),
	}
	testdata2 = types.Validator{
		Ed25519: types.HexToEd25519Key("0x2"),
	}
	diff := CompareJSON(testdata1, testdata2)
	fmt.Print(diff)
}

func TestCompareLogs(t *testing.T) {
	f1, err := os.Open("interpreter/0_accumulate.json")
	if err != nil {
		t.Fatalf("failed to open interpreter/vm_log.json: %v", err)
	}
	defer f1.Close()

	f2, err := os.Open("recompiler_sandbox/0_accumulate.json")
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
				fmt.Printf("Operation: %s\n", pvm.DisassembleSingleInstruction(orig.Opcode, orig.Operands))
				t.Fatalf("differences at index %d: %s", i, diff)
			}
		} else if i%100000 == 0 {
			fmt.Printf("Index %d: no difference %s\n", i, s1.Bytes())
		}
		i++
	}
}

func TestTracesFuzz(t *testing.T) {
	pvm.PvmLogging = true
	pvm.PvmTrace = true   // enable PVM trace for this test
	pvm.VMsCompare = true // enable VM comparison for this test
	filename := "../jamtestvectors/fuzz-reports/jamduna/jam-duna-target-v0.5-0.6.7_gp-0.6.7/00000001.json"
	filename = "../jamtestvectors/fuzz-reports/jamduna/jam-duna-target-v0.7-0.6.7_gp-0.6.7/1754724115/00000004_mod.json"
	content, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("failed to read file %s: %v", filename, err)
	}
	log.InitLogger("debug")
	log.EnableModule(log.PvmAuthoring)
	log.EnableModule("pvm_validator")
	t.Run(filepath.Base(filename), func(t *testing.T) {
		runSingleSTFTest(t, filename, string(content), pvm.BackendInterpreter)
	})
}
