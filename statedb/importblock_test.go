package statedb

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/pvm"
	"github.com/colorfulnotion/jam/types"
)

func SaveReportTrace(path string, obj interface{}) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		panic(fmt.Sprintf("failed to create directory %s: %v", dir, err))
	}
	fmt.Printf("!!! Saving report trace to %s\n", path)
	err := types.SaveObject(path, obj, true)
	if err != nil {
		panic(err)
	}
}

func runSingleSTFTestAndSave(t *testing.T, filename string, content string, pvmBackend string, runPrevalidation bool, sourceBasePath string, destinationPath string) {
	t.Helper()
	testDir := t.TempDir()

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

	diffs, err := CheckStateTransitionWithOutput(test_storage, &stf, nil, pvmBackend, runPrevalidation)

	if err != nil && err.Error() == "OMIT" {
		//	t.Skipf("⚠️ OMIT: Test case for [%s] is marked to be omitted.", filename)
		//	return
	}

	relativePath, relErr := filepath.Rel(sourceBasePath, filename)
	if relErr != nil {
		t.Fatalf("Could not get relative path for %s from source base %s: %v", filename, sourceBasePath, relErr)
	}
	newFilename := strings.TrimSuffix(relativePath, filepath.Ext(relativePath)) + ".json"
	finalFilename := strings.ReplaceAll(newFilename, string(filepath.Separator), "_")

	outputPath := filepath.Join(destinationPath, "generated_reports", finalFilename)

	SaveReportTrace(outputPath, &stf)
	fmt.Printf("📄 Saved report to %s\n", outputPath)

	if err == nil {
		fmt.Printf("✅ [%s] PostState.StateRoot %s matches\n", filename, stf.PostState.StateRoot)
	} else {
		HandleDiffs(diffs)
		t.Errorf("❌ [%s] Test failed: %v", filename, err)
	}
}

func runSingleSTFTest(t *testing.T, filename string, content string, pvmBackend string, runPrevalidation bool) {
	t.Helper()
	testDir := t.TempDir()

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

	diffs, err := CheckStateTransitionWithOutput(test_storage, &stf, nil, pvmBackend, runPrevalidation)
	if err == nil {
		fmt.Printf("✅ [%s] PostState.StateRoot %s matches\n", filename, stf.PostState.StateRoot)
		return
	}
	if err.Error() == "OMIT" {
		//	t.Skipf("⚠️ OMIT: Test case for [%s] is marked to be omitted.", filename)
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
	pvm.PvmLogging = false
	pvm.PvmTrace = false   // enable PVM trace for this test
	pvm.VMsCompare = false // enable VM comparison for this test
	filename := "../jamtestvectors/traces/preimages_light/00000012.json"

	content, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("failed to read file %s: %v", filename, err)
	}
	log.InitLogger("debug")
	log.EnableModule(log.PvmAuthoring)
	log.EnableModule("pvm_validator")
	t.Run(filepath.Base(filename), func(t *testing.T) {
		runSingleSTFTest(t, filename, string(content), pvm.BackendInterpreter, false)
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
		runSingleSTFTest(t, filename, string(content), pvm.BackendRecompilerSandbox, false)

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
		runSingleSTFTest(t, filename, string(content), pvm.BackendRecompiler, false)

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

func TestTracesInterpreter(t *testing.T) {
	log.InitLogger("debug")
	pvm.VMsCompare = true // enable VM comparison for this test
	// pvm.PvmLogging = true

	// Define all the directories you want to test in a single slice.
	testDirs := []string{
		//"../cmd/importblocks/rawdata/safrole/state_transitions/",
		//"../jamtestvectors/traces/fallback",
		//"../jamtestvectors/traces/safrole",
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
					runSingleSTFTest(t, filename, string(content), pvm.BackendInterpreter, false)
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
		currentDir := dir

		t.Run(fmt.Sprintf("Directory_%s", filepath.Base(currentDir)), func(t *testing.T) {
			entries, err := os.ReadDir(currentDir)
			if err != nil {
				t.Fatalf("failed to read directory %s: %v", currentDir, err)
			}

			for _, e := range entries {
				if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") || e.Name() == "00000000.json" {
					continue
				}

				filename := filepath.Join(currentDir, e.Name())
				content, err := os.ReadFile(filename)
				if err != nil {
					t.Errorf("failed to read file %s: %v", filename, err)
					continue
				}

				fmt.Printf("Running test for file: %s\n", filename)

				t.Run(e.Name(), func(t *testing.T) {
					runSingleSTFTest(t, filename, string(content), pvm.BackendRecompiler, false)
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
					runSingleSTFTest(t, filename, string(content), pvm.BackendRecompilerSandbox, false)
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

func GetFuzzReportsPath() (string, error) {
	fuzzPath := os.Getenv("JAM_CONFORMANCE_PATH")
	if fuzzPath == "" {
		return "", fmt.Errorf("JAM_CONFORMANCE_PATH environment variable is not set")
	}
	return filepath.Abs(fuzzPath)
}

// export DUNA_CONFORMANCE_PATH=~/Desktop/reports
func GetDunaReportsPath() (string, error) {
	dunaPath := os.Getenv("DUNA_CONFORMANCE_PATH")
	if dunaPath == "" {
		return "", fmt.Errorf("DUNA_CONFORMANCE_PATH environment variable is not set")
	}
	return filepath.Abs(dunaPath)
}

func findFuzzTestFiles(sourcePath, targetVersion string, excludedTeams []string) ([]string, error) {
	fmt.Printf("Load GP v(%v) Trace files in %s | Excluding: %v\n", targetVersion, sourcePath, excludedTeams)
	var testFiles []string

	err := filepath.WalkDir(sourcePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		for _, team := range excludedTeams {
			if team != "" && strings.Contains(path, team) {
				return nil
			}
		}

		isTestFile := !d.IsDir() &&
			strings.Contains(path, targetVersion) &&
			!strings.Contains(path, "RETIRED") &&
			strings.HasSuffix(d.Name(), ".bin") &&
			d.Name() != "report.bin" && d.Name() != "genesis.bin"

		if isTestFile {
			testFiles = append(testFiles, path)
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("error walking directory %s: %w", sourcePath, err)
	}

	return testFiles, nil
}

func TestSingleFuzzTrace(t *testing.T) {
	pvm.PvmLogging = false
	pvm.PvmTrace = false  // enable PVM trace for this test
	pvm.VMsCompare = true // enable VM comparison for this test
	fileMap := make(map[string]string)

	jamConformancePath, err := GetFuzzReportsPath()
	if err != nil {
		t.Fatalf("failed to get fuzz reports path: %v", err)
	}

	// FAILS due to recent accumulation being updated too early
	fileMap["6995"] = "0.6.7/traces/TESTING/1755796995/00000011.bin"
	fileMap["0509"] = "0.6.7/traces/TESTING/1755530509/00000004.bin"
	fileMap["0728"] = "0.6.7/traces/1755530728/00000008.bin"
	fileMap["1265"] = "0.6.7/traces/1755531265/00000008.bin"
	fileMap["6851"] = "0.6.7/traces/TESTING/1755796851/00000016.bin"

	team := "6995" //6995
	filename, exists := fileMap[team]
	if !exists {
		t.Fatalf("team %s not found in fileMap", team)
	}
	filename = filepath.Join(jamConformancePath, filename)
	content, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("failed to read file %s: %v", filename, err)
	}

	log.InitLogger("debug")
	log.EnableModule(log.PvmAuthoring)
	log.EnableModule("pvm_validator")
	log.EnableModule(log.SDB)
	t.Run(filepath.Base(filename), func(t *testing.T) {
		runSingleSTFTest(t, filename, string(content), pvm.BackendInterpreter, true)
	})
}

func testFuzzTraceInternal(t *testing.T, saveOutput bool) {
	t.Helper()

	pvm.PvmLogging = false
	pvm.PvmTrace = false
	pvm.VMsCompare = false

	log.InitLogger("debug")
	log.EnableModule(log.PvmAuthoring)
	log.EnableModule("pvm_validator")

	targetVersion := "0.6.7"
	excludedTeams := []string{"vinwolf"}

	sourcePath, err := GetFuzzReportsPath()
	if err != nil {
		t.Fatalf("failed to get source fuzz reports path: %v", err)
	}

	var destinationPath string
	if saveOutput {
		destinationPath, err = GetDunaReportsPath()
		if err != nil {
			t.Fatalf("failed to get destination reports path: %v", err)
		}
	}

	testFiles, err := findFuzzTestFiles(sourcePath, targetVersion, excludedTeams)
	if err != nil {
		t.Fatal(err)
	}
	if len(testFiles) == 0 {
		t.Fatalf("No valid .bin files found for version '%s' under %s (after exclusions)", targetVersion, sourcePath)
	}

	for _, filename := range testFiles {
		currentFile := filename
		relPath, _ := filepath.Rel(sourcePath, currentFile)
		testName := filepath.ToSlash(relPath)
		if strings.Contains(testName, "RETIRED") || strings.Contains(testName, "1754982087") || strings.Contains(testName, "1755252727") {
			continue
		}
		t.Run(testName, func(t *testing.T) {
			content, err := os.ReadFile(currentFile)
			if err != nil {
				t.Fatalf("failed to read file %s: %v", currentFile, err)
			}

			if saveOutput {
				t.Parallel()
				runSingleSTFTestAndSave(t, currentFile, string(content), pvm.BackendInterpreter, true, sourcePath, destinationPath)
			} else {
				runSingleSTFTest(t, currentFile, string(content), pvm.BackendInterpreter, true)
			}
		})
	}
}

func TestFuzzTrace(t *testing.T) {
	testFuzzTraceInternal(t, false)
}

func TestPublishFuzzTrace(t *testing.T) {
	testFuzzTraceInternal(t, true)
}
