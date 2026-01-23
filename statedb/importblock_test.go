package statedb

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	_ "net/http/pprof"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"runtime/pprof"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/pvm"
	"github.com/colorfulnotion/jam/pvm/interpreter"
	"github.com/colorfulnotion/jam/pvm/pvmtypes"
	"github.com/colorfulnotion/jam/types"
)

var EnablePVMOutputLogs = true

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

	diffs, err := CheckStateTransitionWithOutput(test_storage, &stf, nil, pvmBackend, runPrevalidation, "")

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

func runSingleSTFTest(t *testing.T, filename string, content string, pvmBackend string, runPrevalidation bool) error {
	t.Helper()
	start := time.Now()
	t0 := time.Now()
	testDir := t.TempDir()

	test_storage, err := initStorage(testDir)
	if err != nil {
		t.Errorf("❌ [%s] Error initializing storage: %v", filename, err)
		return err
	}
	defer test_storage.Close()
	benchRec.Add("runSingleSTFTest:initStorage", time.Since(t0))

	t0 = time.Now()
	stf, err := parseSTFFile(filename, content)
	if err != nil {
		t.Errorf("❌ [%s] Failed to parse STF: %v", filename, err)
		return err
	}
	benchRec.Add("runSingleSTFTest:parseSTFFile", time.Since(t0))

	// logDir controls PVM output logging - controlled by EnablePVMOutputLogs flag
	// Set EnablePVMOutputLogs = true to write outputs to disk for debugging
	var logDir string
	if EnablePVMOutputLogs {
		// Create logDir from the last two path components
		// e.g. xxx/storage_light/00000001.bin -> "storage_light/00000001"
		parts := strings.Split(filepath.ToSlash(filename), "/")
		if len(parts) >= 2 {
			dir := parts[len(parts)-2]
			base := strings.TrimSuffix(parts[len(parts)-1], filepath.Ext(parts[len(parts)-1]))
			logDir = dir + "/" + base
		} else {
			logDir = strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
		}
	}

	t0 = time.Now()
	diffs, err := CheckStateTransitionWithOutput(test_storage, &stf, nil, pvmBackend, runPrevalidation, logDir)
	benchRec.Add("runSingleSTFTest:CheckStateTransitionWithOutput", time.Since(t0))
	elapsed := time.Since(start)

	// Determine if this STF expects a failure (PreState == PostState means invalid block)
	expectsFailure := stf.PreState.StateRoot == stf.PostState.StateRoot

	if err == nil {
		if expectsFailure {
			// Block was invalid and we correctly rejected it (no state change)
			fmt.Printf("👍 [%s] Invalid block correctly rejected, StateRoot unchanged %s (%.5fms)\n", filename, stf.PostState.StateRoot, float64(elapsed.Nanoseconds())/1000000)
		} else {
			// Block was valid and we correctly processed it
			fmt.Printf("✅ [%s] PostState.StateRoot %s matches (%.5fms)\n", filename, stf.PostState.StateRoot, float64(elapsed.Nanoseconds())/1000000)
		}
		return nil
	}

	if err.Error() == "OMIT" {
		//	t.Skipf("⚠️ OMIT: Test case for [%s] is marked to be omitted.", filename)
	}

	if expectsFailure {
		fmt.Printf("👍 [%s] Expected failure correctly caught: %v (%.5fms)\n", filename, err, float64(elapsed.Nanoseconds())/1000000)
		return nil
	}

	HandleDiffs(diffs)
	t.Errorf("❌ [%s] Test failed: %v (%.5fms)", filename, err, float64(elapsed.Nanoseconds())/1000000)
	return err
}

func parseSTFFile(filename, content string) (StateTransition, error) {
	var stf StateTransition
	var err error
	if strings.HasSuffix(filename, ".bin") {
		t0 := time.Now()
		stf0, _, err := types.Decode([]byte(content), reflect.TypeOf(StateTransition{}))
		if err == nil {
			stf = stf0.(StateTransition)
		}
		benchRec.Add("parseSTFFile:Decode", time.Since(t0))
	} else {
		t0 := time.Now()
		err = json.Unmarshal([]byte(content), &stf)
		benchRec.Add("parseSTFFile:Unmarshal", time.Since(t0))
	}
	return stf, err
}

func parseSnapShotRawFile(filename, content string) (StateSnapshotRaw, error) {
	var state StateSnapshotRaw
	var err error
	if strings.HasSuffix(filename, ".bin") {
		state0, _, err := types.Decode([]byte(content), reflect.TypeOf(StateSnapshotRaw{}))
		if err == nil {
			state = state0.(StateSnapshotRaw)
		}
	} else {
		err = json.Unmarshal([]byte(content), &state)
	}
	return state, err
}

func TestStateTransitionInterpreter(t *testing.T) {
	interpreter.PvmLogging = false

	filename := path.Join(common.GetJAMTestVectorPath("traces"), "preimages_light/00000060.bin")
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

func TestTracesInterpreter(t *testing.T) {
	interpreter.PvmTraceMode = false
	interpreter.PvmLogging = false
	pvmtypes.DebugHostFunctions = false
	log.InitLogger("debug")

	// Define all the directories you want to test in a single slice.
	testDirs := []string{
		path.Join(common.GetJAMTestVectorPath("traces"), "fallback"),
		path.Join(common.GetJAMTestVectorPath("traces"), "safrole"),
		path.Join(common.GetJAMTestVectorPath("traces"), "storage_light"),
		path.Join(common.GetJAMTestVectorPath("traces"), "preimages_light"),
		path.Join(common.GetJAMTestVectorPath("traces"), "fuzzy_light"),
		path.Join(common.GetJAMTestVectorPath("traces"), "storage"),
		path.Join(common.GetJAMTestVectorPath("traces"), "preimages"),
		path.Join(common.GetJAMTestVectorPath("traces"), "fuzzy"),
	}
	// Iterate over each directory.
	for _, dir := range testDirs {
		// Create a local copy of dir for the sub-test to capture correctly.
		// This avoids issues where the sub-tests might all run with the last value of 'dir'.
		currentDir := dir

		t.Run(fmt.Sprintf("%s", filepath.Base(currentDir)), func(t *testing.T) {
			entries, err := os.ReadDir(currentDir)
			if err != nil {
				// Use t.Fatalf to stop the test for this directory if we can't read it.
				t.Fatalf("failed to read directory %s: %v", currentDir, err)
			}

			for _, e := range entries {
				if e.IsDir() || !strings.HasSuffix(e.Name(), ".bin") || (e.Name() == "00000000.bin" || e.Name() == "genesis.bin") {
					continue
				}

				filename := filepath.Join(currentDir, e.Name())
				content, err := os.ReadFile(filename)
				if err != nil {
					// Use t.Errorf to report the error but continue with other files.
					t.Errorf("failed to read file %s: %v", filename, err)
					continue
				}

				//				fmt.Printf("Running test for file: %s\n", filename)

				// Run the actual test logic for each file as a distinct sub-test.
				t.Run(e.Name(), func(t *testing.T) {
					runSingleSTFTest(t, filename, string(content), pvm.BackendInterpreter, false)
				})
				runtime.GC()
			}
		})
	}
	//(t)
}
func TestTracesRecompiler(t *testing.T) {
	log.InitLogger("debug")

	cpuFile, err := os.Create("cpu.prof")
	if err != nil {
		t.Fatalf("could not create CPU profile: %v", err)
	}
	if err := pprof.StartCPUProfile(cpuFile); err != nil {
		t.Fatalf("could not start CPU profile: %v", err)
	}

	pvmtypes.DebugHostFunctions = false
	// Define all the directories you want to test in a single slice.
	testDirs := []string{
		path.Join(common.GetJAMTestVectorPath("traces"), "fallback"),
		path.Join(common.GetJAMTestVectorPath("traces"), "safrole"),
		path.Join(common.GetJAMTestVectorPath("traces"), "preimages_light"),
		path.Join(common.GetJAMTestVectorPath("traces"), "storage_light"),
		path.Join(common.GetJAMTestVectorPath("traces"), "storage"),
		path.Join(common.GetJAMTestVectorPath("traces"), "preimages"),
		path.Join(common.GetJAMTestVectorPath("traces"), "fuzzy"),
		path.Join(common.GetJAMTestVectorPath("traces"), "fuzzy_light"),
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
				if e.IsDir() || !strings.HasSuffix(e.Name(), ".bin") || (e.Name() == "00000000.bin" || e.Name() == "genesis.bin") {
					continue
				}

				filename := filepath.Join(currentDir, e.Name())
				content, err := os.ReadFile(filename)
				if err != nil {
					// Use t.Errorf to report the error but continue with other files.
					t.Errorf("failed to read file %s: %v", filename, err)
					continue
				}

				//				fmt.Printf("Running test for file: %s\n", filename)

				// Run the actual test logic for each file as a distinct sub-test.
				t.Run(e.Name(), func(t *testing.T) {
					runSingleSTFTest(t, filename, string(content), pvm.BackendCompiler, false)
				})
			}
		})
	}
	dump_performance(t)

	// Stop CPU profiling
	pprof.StopCPUProfile()
	cpuFile.Close()

	// Write heap profile
	heapFile, err := os.Create("heap.prof")
	if err != nil {
		t.Fatalf("could not create heap profile: %v", err)
	}
	runtime.GC() // Run GC first to get more accurate heap information
	if err := pprof.WriteHeapProfile(heapFile); err != nil {
		t.Fatalf("could not write heap profile: %v", err)
	}
	heapFile.Close()

	fmt.Println("\n========================================")
	fmt.Println("Profiling complete! Files have been saved:")
	fmt.Println("  - cpu.prof")
	fmt.Println("  - heap.prof")
	fmt.Println("")
	fmt.Println("  go tool pprof -http=:8080 cpu.prof")
	fmt.Println("  go tool pprof -http=:8081 heap.prof")
	fmt.Println("========================================")
}
func dump_performance(t *testing.T) {
	return
	rows := BenchRows() // expose recorder snapshot via a small helper (see below)
	if len(rows) == 0 {
		t.Fatalf("no timing rows recorded; did you run with -tags benchprofile ?")
	}

	fmt.Println("\n=== statedb: Top 40 by TOTAL time ===")
	for i, r := range rows {
		if i == 40 {
			break
		}
		fmt.Printf("%-45s  total=%-12s count=%-4d mean=%-10s p95=%-10s max=%-10s\n",
			r.Name, r.Total, r.Count, r.Mean, r.P95, r.Max)
	}

}
func TestSingleCompare(t *testing.T) {
	// DO NOT CHANGE THIS
	log.InitLogger("debug")
	interpreter.PvmLogging = false

	filename := path.Join(common.GetJAMTestVectorPath("traces"), "fuzzy/00000174.bin")
	content, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("failed to read file %s: %v", filename, err)
	}
	runSingleSTFTest(t, filename, string(content), pvm.BackendCompiler, false)
}

func GetFuzzReportsPath(subDir ...string) (string, error) {
	fuzzPath := os.Getenv("JAM_CONFORMANCE_PATH")
	if fuzzPath == "" {
		return "", fmt.Errorf("JAM_CONFORMANCE_PATH environment variable is not set")
	}
	targetDir := "" // Default sub dir
	if len(subDir) > 0 {
		targetDir = subDir[0]
	}
	return filepath.Abs(filepath.Join(fuzzPath, targetDir))
}

func GetDunaReportsPath(subDir ...string) (string, error) {
	dunaPath := os.Getenv("DUNA_CONFORMANCE_PATH")
	if dunaPath == "" {
		return "", fmt.Errorf("DUNA_CONFORMANCE_PATH environment variable is not set")
	}
	return filepath.Abs(filepath.Join(dunaPath, filepath.Join(subDir...)))
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
	backend := pvm.BackendCompiler
	if runtime.GOOS != "linux" {
		backend = pvm.BackendInterpreter
	}
	//backend = pvm.BackendInterpreter
	t.Logf("Using backend: %s", backend)

	fileMap := make(map[string]string)

	jamConformancePath, err := GetFuzzReportsPath()
	if err != nil {
		t.Fatalf("failed to get fuzz reports path: %v", err)
	}
	/*
			--- PASS: TestFuzzTrace/fuzz-reports/0.7.2/traces/1767871405_1375/00000033.bin (0.08s)
		    --- PASS: TestFuzzTrace/fuzz-reports/0.7.2/traces/1767871405_1375/00000034.bin (0.16s)
		    --- PASS: TestFuzzTrace/fuzz-reports/0.7.2/traces/1767871405_1375/00000035.bin (0.10s)
		    --- PASS: TestFuzzTrace/fuzz-reports/0.7.2/traces/1767871405_1375/00000036.bin (0.05s)
		    --- PASS: TestFuzzTrace/fuzz-reports/0.7.2/traces/1767871405_1375/00000037.bin (0.09s)
	*/

	fileMap["1767871405_1375_33"] = "fuzz-reports/0.7.2/traces/1767871405_1375/00000033.bin"
	fileMap["1767871405_1375_34"] = "fuzz-reports/0.7.2/traces/1767871405_1375/00000034.bin"
	fileMap["1767871405_1375_35"] = "fuzz-reports/0.7.2/traces/1767871405_1375/00000035.bin"
	fileMap["1767871405_1375_36"] = "fuzz-reports/0.7.2/traces/1767871405_1375/00000036.bin"
	fileMap["1767871405_1375_37"] = "fuzz-reports/0.7.2/traces/1767871405_1375/00000037.bin"
	interpreter.PvmLogging = false
	//	DebugHostFunctions = true
	log.InitLogger("debug")
	// log.EnableModule(log.PvmAuthoring)
	// log.EnableModule("pvm_validator")
	log.EnableModule(log.SDB)

	tc := []string{"1767871405_1375_33", "1767871405_1375_34", "1767871405_1375_35", "1767871405_1375_36", "1767871405_1375_37"}

	interpreter.PvmLogging = false
	for _, team := range tc {
		filename, exists := fileMap[team]
		if !exists {
			t.Fatalf("team %s not found in fileMap", team)
		}

		filename = filepath.Join(jamConformancePath, filename)

		content, err := os.ReadFile(filename)
		if err != nil {
			t.Fatalf("failed to read file %s: %v", filename, err)
		}

		t.Run(filepath.Base(filename), func(t *testing.T) {

			runSingleSTFTest(t, filename, string(content), backend, true)
		})
	}
}
func TestTaintSingleFuzzTrace(t *testing.T) {
	// Force interpreter backend to enable taint tracking
	backend := pvm.BackendInterpreter
	t.Logf("Using backend: %s (forced for taint tracking)", backend)

	fileMap := make(map[string]string)

	jamConformancePath, err := GetFuzzReportsPath()
	if err != nil {
		t.Fatalf("failed to get fuzz reports path: %v", err)
	}

	fileMap["1768816138"] = "fuzz-reports/0.7.2/traces/1768816138/00000310.bin"
	interpreter.PvmLogging = false
	//	DebugHostFunctions = true
	log.InitLogger("debug")
	// log.EnableModule(log.PvmAuthoring)
	// log.EnableModule("pvm_validator")
	log.EnableModule(log.SDB)

	tc := []string{"1768816138"}

	interpreter.PvmLogging = false
	for _, team := range tc {
		filename, exists := fileMap[team]
		if !exists {
			t.Fatalf("team %s not found in fileMap", team)
		}

		filename = filepath.Join(jamConformancePath, filename)

		content, err := os.ReadFile(filename)
		if err != nil {
			t.Fatalf("failed to read file %s: %v", filename, err)
		}

		t.Run(filepath.Base(filename), func(t *testing.T) {
			runSingleSTFTestWithTaint(t, filename, string(content), backend, true)
		})
	}
}

// runSingleSTFTestWithTaint is a variant that enables taint tracking and prints trace after execution
func runSingleSTFTestWithTaint(t *testing.T, filename string, content string, pvmBackend string, runPrevalidation bool) error {
	t.Helper()

	// Enable taint tracking globally before execution
	interpreter.EnableTaintTrackingGlobal = true
	defer func() {
		interpreter.EnableTaintTrackingGlobal = false
	}()

	start := time.Now()
	t0 := time.Now()
	testDir := t.TempDir()

	test_storage, err := initStorage(testDir)
	if err != nil {
		t.Errorf("❌ [%s] Error initializing storage: %v", filename, err)
		return err
	}
	defer test_storage.Close()
	benchRec.Add("runSingleSTFTest:initStorage", time.Since(t0))

	t0 = time.Now()
	stf, err := parseSTFFile(filename, content)
	if err != nil {
		t.Errorf("❌ [%s] Failed to parse STF: %v", filename, err)
		return err
	}
	benchRec.Add("runSingleSTFTest:parseSTFFile", time.Since(t0))

	var logDir string
	if EnablePVMOutputLogs {
		parts := strings.Split(filepath.ToSlash(filename), "/")
		if len(parts) >= 2 {
			dir := parts[len(parts)-2]
			base := strings.TrimSuffix(parts[len(parts)-1], filepath.Ext(parts[len(parts)-1]))
			logDir = dir + "/" + base
		} else {
			logDir = strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
		}
	}

	t0 = time.Now()
	diffs, err := CheckStateTransitionWithOutput(test_storage, &stf, nil, pvmBackend, runPrevalidation, logDir)
	benchRec.Add("runSingleSTFTest:CheckStateTransitionWithOutput", time.Since(t0))
	elapsed := time.Since(start)

	// Note: Taint trace will be printed by individual VMs during execution
	// The trace is automatically printed when taint tracking is enabled

	if err == nil {
		fmt.Printf("✅ [%s] PostState.StateRoot %s matches (%.5fms)\n", filename, stf.PostState.StateRoot, float64(elapsed.Nanoseconds())/1000000)
		return nil
	} else {
		fmt.Printf("Fail With Error: %v\n", err)
	}
	if err.Error() == "OMIT" {
		//	t.Skipf("⚠️ OMIT: Test case for [%s] is marked to be omitted.", filename)
	}

	HandleDiffs(diffs)
	t.Errorf("❌ [%s] Test failed: %v (%.5fms)", filename, err, float64(elapsed.Nanoseconds())/1000000)
	return err
}

func testFuzzTraceInternal(t *testing.T, saveOutput bool) {
	t.Helper()

	interpreter.PvmLogging = false

	log.InitLogger("debug")
	log.EnableModule(log.PvmAuthoring)
	log.EnableModule("pvm_validator")

	targetVersion := "0.7.2"
	excludedTeams := []string{""}

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
	backend := pvm.BackendCompiler
	if runtime.GOOS != "linux" {
		backend = pvm.BackendInterpreter
	}
	bugMap := map[string]int{}
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
				runSingleSTFTestAndSave(t, currentFile, string(content), backend, true, sourcePath, destinationPath)
			} else {
				err := runSingleSTFTest(t, currentFile, string(content), backend, true)
				if err != nil {
					bugMap[err.Error()]++
				}
			}
		})
	}
	fmt.Printf("=== Fuzz Trace Test Summary ===\n")
	if len(bugMap) == 0 {
		fmt.Printf("All tests passed successfully!\n")
	} else {
		for err, count := range bugMap {
			fmt.Printf("Error: %v | Occurrences: %d\n", err, count)
		}
		t.Fatalf("Some tests failed. See summary above.")
	}
}

func TestFuzzTraceIndependent(t *testing.T) {
	testFuzzTraceInternal(t, false)
}

func TestPublishFuzzTrace(t *testing.T) {
	testFuzzTraceInternal(t, true)
}

func TestFuzzTraceSequential(t *testing.T) {
	log.InitLogger("debug")

	targetVersion := "0.7.2"
	testAllTraces := true // When true, loop through all traces; when false, only test specific cases

	sourcePath, err := GetFuzzReportsPath()
	if err != nil {
		t.Fatalf("failed to get source fuzz reports path: %v", err)
	}

	if testAllTraces {
		// Discover all trace directories dynamically
		tracesDir := filepath.Join(sourcePath, fmt.Sprintf("fuzz-reports/%s/traces", targetVersion))
		entries, err := os.ReadDir(tracesDir)
		if err != nil {
			t.Fatalf("failed to read traces directory %s: %v", tracesDir, err)
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			testCaseID := entry.Name()
			t.Run(testCaseID, func(t *testing.T) {
				runSequentialFuzzTrace(t, sourcePath, targetVersion, testCaseID, false)
			})
		}
	} else {
		// Test specific cases that failed before the SetRoot fix
		testCases := []string{
			"1768816138", // Conformance test failure from w3f/jam-conformance JamZig_m1
		}

		for _, id := range testCases {
			t.Run(id, func(t *testing.T) {
				pvmtypes.DebugHostFunctions = true
				runSequentialFuzzTrace(t, sourcePath, targetVersion, id, true)
			})
		}
	}
}

// runSequentialFuzzTrace runs a single sequential fuzz trace test case
func runSequentialFuzzTrace(t *testing.T, sourcePath, targetVersion, testCaseID string, printDiff bool) {
	traceDir := filepath.Join(sourcePath, fmt.Sprintf("fuzz-reports/%s/traces", targetVersion), testCaseID)

	backend := pvm.BackendCompiler
	if runtime.GOOS != "linux" {
		backend = pvm.BackendInterpreter
	}
	fmt.Printf("PVM Backend: %s\n", backend)

	// Find step files by globbing for .bin files (handles any starting number)
	stepFiles, err := filepath.Glob(filepath.Join(traceDir, "*.bin"))
	if err != nil {
		t.Fatalf("failed to glob step files: %v", err)
	}
	sort.Strings(stepFiles) // Ensure files are in order

	if len(stepFiles) == 0 {
		t.Fatalf("No step files found in %s", traceDir)
	}

	// Load the first step to get the initial PreState
	firstStepContent, err := os.ReadFile(stepFiles[0])
	if err != nil {
		t.Fatalf("failed to read first step file: %v", err)
	}
	firstSTF, err := parseSTFFile(stepFiles[0], string(firstStepContent))
	if err != nil {
		t.Fatalf("failed to parse first step: %v", err)
	}

	// Initialize storage and state from first step's PreState
	testDir := t.TempDir()
	test_storage, err := initStorage(testDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer test_storage.Close()

	stateDB, err := NewStateDBFromStateKeyVals(test_storage, &StateKeyVals{KeyVals: firstSTF.PreState.KeyVals})
	if err != nil {
		t.Fatalf("failed to initialize state from PreState: %v", err)
	}

	// Track header hash -> state root mapping for fork handling (mirrors fuzzer target's stateDBMap)
	stateDBMap := make(map[common.Hash]common.Hash)
	// Record initial state with a synthetic genesis header hash
	genesisHeaderHash := firstSTF.Block.Header.ParentHeaderHash
	stateDBMap[genesisHeaderHash] = stateDB.StateRoot
	stateDB.HeaderHash = genesisHeaderHash

	fmt.Printf("Initialized from first step's PreState: StateRoot=%s, Timeslot=%d\n", stateDB.StateRoot.Hex(), stateDB.GetTimeslot())
	fmt.Printf("Found %d step files to process sequentially\n", len(stepFiles))

	// Track results
	successfulImports := 0
	expectedSuccesses := 0

	// Process each step sequentially (simulates fuzzer's ImportBlock messages)
	for stepNum, stepFile := range stepFiles {
		t.Logf("Processing step file: %s", stepFile)
		stepContent, err := os.ReadFile(stepFile)
		if err != nil {
			t.Fatalf("failed to read step file %s: %v", stepFile, err)
		}

		stf, err := parseSTFFile(stepFile, string(stepContent))
		if err != nil {
			t.Fatalf("failed to parse step %d: %v", stepNum+1, err)
		}

		// Check if this step should succeed (pre != post means valid block)
		shouldSucceed := stf.PreState.StateRoot != stf.PostState.StateRoot
		if shouldSucceed {
			expectedSuccesses++
		}

		block := &stf.Block
		headerHash := block.Header.Hash()
		binFileName := filepath.Base(stepFile)

		fmt.Printf("\n=== %s (Step %d): slot=%d ===\n", binFileName, stepNum+1, block.Header.Slot)
		fmt.Printf("  STF expects: %s\n", func() string {
			if shouldSucceed {
				return "SUCCESS (PreState != PostState)"
			}
			return "FAILURE (PreState == PostState, invalid block)"
		}())
		//		t.Logf("  Block.ParentStateRoot: %s", block.Header.ParentStateRoot.Hex())
		//		t.Logf("  Current stateDB: StateRoot=%s, Timeslot=%d", stateDB.StateRoot.Hex(), stateDB.GetTimeslot())

		// Fuzzer protocol: use current stateDB, handle forks by restoring to parent state
		// This mirrors fuzzer_target.go:280-314
		preState := stateDB

		// Check if this is a fork (block's parent doesn't match current state)
		if block.Header.ParentStateRoot != stateDB.StateRoot {
			fmt.Printf("  Fork detected! Block wants parent: %s, we have: %s\n",
				block.Header.ParentStateRoot.Hex(), stateDB.StateRoot.Hex())

			// Look up parent header hash from stateDBMap (mirrors fuzzer target)
			var foundParentHeaderHash common.Hash
			for hdrHash, stateRoot := range stateDBMap {
				if stateRoot == block.Header.ParentStateRoot {
					foundParentHeaderHash = hdrHash
					fmt.Printf("  Found parent in stateDBMap: HeaderHash=%s -> StateRoot=%s\n",
						hdrHash.Hex(), stateRoot.Hex())
					break
				}
			}

			if foundParentHeaderHash != (common.Hash{}) {
				// Try to restore to the parent state from trie
				forkState := stateDB.Copy()
				if err := forkState.InitTrieAndLoadJamState(block.Header.ParentStateRoot); err != nil {
					fmt.Printf("  Failed to restore fork state: %v\n", err)
				} else {
					forkState.HeaderHash = foundParentHeaderHash
					preState = forkState
					fmt.Printf("  Fork state restored: HeaderHash=%s, StateRoot=%s, Timeslot=%d\n",
						preState.HeaderHash.Hex(), preState.StateRoot.Hex(), preState.GetTimeslot())
				}
			} else {
				fmt.Printf("  ERROR: Could not find parent state with StateRoot=%s in stateDBMap\n", block.Header.ParentStateRoot.Hex())
			}
		}
		//t.Logf("  Using preState: StateRoot=%s, Timeslot=%d",preState.StateRoot.Hex(), preState.GetTimeslot())

		// Apply state transition
		stateCopy := preState.Copy()
		postState, jamErr := ApplyStateTransitionFromBlock(0, stateCopy, context.Background(), block, nil, backend, "")

		if jamErr != nil {
			if shouldSucceed {
				t.Errorf("❌ Step %d: Expected success but got error: %v", stepNum+1, jamErr)
				t.Errorf("File Path: %s", stepFile)
			}
			fmt.Printf("  👍 Import FAILED: %v\n", jamErr)
		} else {
			successfulImports++

			// Verify post state matches expected
			if postState.StateRoot != stf.PostState.StateRoot {
				t.Errorf("❌ Step %d: PostState mismatch! Got %s, expected %s", stepNum+1, postState.StateRoot.Hex(), stf.PostState.StateRoot.Hex())
			}
			if printDiff {
				diffs := compareKeyValsWithOutput(stf.PreState.KeyVals, postState.GetAllKeyValues(), stf.PostState.KeyVals)
				HandleDiffs(diffs)
			}
			// Update state (only on success, like the real target)
			stateDB = postState
			stateDBMap[headerHash] = postState.StateRoot

			if !shouldSucceed {
				t.Errorf("❌ Step %d: Expected failure but import succeeded", stepNum+1)

			}
			fmt.Printf("  ✅ Import SUCCESS: PostStateRoot=%s\n", postState.StateRoot.Hex())
		}
	}

	fmt.Printf("\n=== Summary ===")
	fmt.Printf("\nTotal steps: %d\n", len(stepFiles))
	fmt.Printf("Successful imports: %d | Expected successes: %d\n", successfulImports, expectedSuccesses)

	if successfulImports != expectedSuccesses {
		t.Errorf("Import count mismatch: got %d successful, expected %d", successfulImports, expectedSuccesses)
	}
}

// Genesis file structure for parsing
type GenesisTrace struct {
	Header types.BlockHeader `json:"header"`
	State  StateSnapshotRaw  `json:"state"`
}

func parseGenesisFile(filename, content string) (*GenesisTrace, error) {
	var genesis GenesisTrace
	var err error
	if strings.HasSuffix(filename, ".bin") {
		genesis0, _, err := types.Decode([]byte(content), reflect.TypeOf(GenesisTrace{}))
		if err == nil {
			genesis = genesis0.(GenesisTrace)
		}
	} else {
		err = json.Unmarshal([]byte(content), &genesis)
	}
	return &genesis, err
}

type logFile struct {
	file    *os.File
	writer  *bufio.Writer
	lastGas int64
}

func TestLogSplit(t *testing.T) {
	// Open input file
	fn := "156-javajam.log"
	inputFile, err := os.Open(fn)
	if err != nil {
		t.Fatalf("Error opening %s: %v", fn, err)
	}
	defer inputFile.Close()

	// Regular expression to extract gas from the line
	// Looking for pattern: Gas: NUMBER
	gasRegex := regexp.MustCompile(`Gas:\s+(\d+)`)

	var logFiles []*logFile
	scanner := bufio.NewScanner(inputFile)

	lineNum := 0
	for scanner.Scan() {
		line := scanner.Text()
		lineNum++

		// Extract gas from the line
		matches := gasRegex.FindStringSubmatch(line)
		if matches == nil {
			// Skip lines without gas information
			continue
		}

		gas, err := strconv.ParseInt(matches[1], 10, 64)
		if err != nil {
			t.Logf("Warning: Could not parse gas value '%s' on line %d\n", matches[1], lineNum)
			continue
		}

		// Find a matching file where gas is decreasing
		matched := false
		for _, lf := range logFiles {
			if gas < lf.lastGas {
				// Gas decreased, append to this file
				lf.writer.WriteString(line + "\n")
				lf.lastGas = gas
				matched = true
				break
			}
		}

		// If no match found, create a new file
		if !matched {
			fileNum := len(logFiles)
			filename := fmt.Sprintf("%s-%d.txt", fn, fileNum)
			newFile, err := os.Create(filename)
			if err != nil {
				t.Fatalf("Error creating %s: %v", filename, err)
			}

			writer := bufio.NewWriter(newFile)
			writer.WriteString(line + "\n")

			logFile := &logFile{
				file:    newFile,
				writer:  writer,
				lastGas: gas,
			}
			logFiles = append(logFiles, logFile)
			t.Logf("Created %s (starting gas: %d)", filename, gas)
		}
	}

	if err := scanner.Err(); err != nil {
		t.Fatalf("Error reading javajam.log: %v", err)
	}

	// Flush and close all files
	for i, lf := range logFiles {
		lf.writer.Flush()
		lf.file.Close()
		t.Logf("Closed javajam-%d.txt (final gas: %d)", i, lf.lastGas)
	}

	t.Logf("Successfully split %d lines into %d files", lineNum, len(logFiles))
}
