package statedb

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
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
	start := time.Now()
	t0 := time.Now()
	testDir := t.TempDir()

	test_storage, err := initStorage(testDir)
	if err != nil {
		t.Errorf("❌ [%s] Error initializing storage: %v", filename, err)
		return
	}
	defer test_storage.Close()
	benchRec.Add("runSingleSTFTest:initStorage", time.Since(t0))

	t0 = time.Now()
	stf, err := parseSTFFile(filename, content)
	if err != nil {
		t.Errorf("❌ [%s] Failed to parse STF: %v", filename, err)
		return
	}
	benchRec.Add("runSingleSTFTest:parseSTFFile", time.Since(t0))

	t0 = time.Now()
	diffs, err := CheckStateTransitionWithOutput(test_storage, &stf, nil, pvmBackend, runPrevalidation)
	benchRec.Add("runSingleSTFTest:CheckStateTransitionWithOutput", time.Since(t0))
	elapsed := time.Since(start)
	if err == nil {
		fmt.Printf("✅ [%s] PostState.StateRoot %s matches (%.5fms)\n", filename, stf.PostState.StateRoot, float64(elapsed.Nanoseconds())/1000000)
		return
	} else {
		fmt.Printf("Fail With Error: %v\n", err)
	}
	if err.Error() == "OMIT" {
		//	t.Skipf("⚠️ OMIT: Test case for [%s] is marked to be omitted.", filename)
	}

	HandleDiffs(diffs)
	t.Errorf("❌ [%s] Test failed: %v (%.5fms)", filename, err, float64(elapsed.Nanoseconds())/1000000)
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

func TestStateTransitionInterpreter(t *testing.T) {
	PvmLogging = false

	filename := path.Join(common.GetJAMTestVectorPath("traces"), "preimages_light/00000060.bin")
	content, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("failed to read file %s: %v", filename, err)
	}
	log.InitLogger("debug")
	log.EnableModule(log.PvmAuthoring)
	log.EnableModule("pvm_validator")
	t.Run(filepath.Base(filename), func(t *testing.T) {
		runSingleSTFTest(t, filename, string(content), BackendGoInterpreter, false)
	})
}

func TestTracesInterpreter(t *testing.T) {
	PvmLogging = false
	DebugHostFunctions = false
	log.InitLogger("debug")

	// Define all the directories you want to test in a single slice.
	testDirs := []string{
		// path.Join(common.GetJAMTestVectorPath("traces"), "fallback"),
		// path.Join(common.GetJAMTestVectorPath("traces"), "safrole"),
		//path.Join(common.GetJAMTestVectorPath("traces"), "storage_light"),
		path.Join(common.GetJAMTestVectorPath("traces"), "preimages_light"),
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
					runSingleSTFTest(t, filename, string(content), BackendGoInterpreter, false)
				})
			}
		})
	}
	dump_performance(t)
}
func TestTracesGoInterpreter(t *testing.T) {
	PvmLogging = false
	DebugHostFunctions = false
	log.InitLogger("debug")

	// Define all the directories you want to test in a single slice.
	testDirs := []string{
		path.Join(common.GetJAMTestVectorPath("traces"), "fallback"),
		path.Join(common.GetJAMTestVectorPath("traces"), "safrole"),
		path.Join(common.GetJAMTestVectorPath("traces"), "storage_light"),
		path.Join(common.GetJAMTestVectorPath("traces"), "preimages_light"),
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
					runSingleSTFTest(t, filename, string(content), BackendGoInterpreter, false)
				})
			}
		})
	}
	dump_performance(t)
}
func TestTracesRecompiler(t *testing.T) {
	log.InitLogger("debug")

	// Define all the directories you want to test in a single slice.
	testDirs := []string{
		//path.Join(common.GetJAMTestVectorPath("traces"), "fallback"),
		//path.Join(common.GetJAMTestVectorPath("traces"), "safrole"),
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
					runSingleSTFTest(t, filename, string(content), BackendCompiler, false)
				})
			}
		})
	}
	dump_performance(t)
}
func dump_performance(t *testing.T) {
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
	PvmLogging = false

	filename := path.Join(common.GetJAMTestVectorPath("traces"), "storage_light/00000001.bin")
	content, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("failed to read file %s: %v", filename, err)
	}
	runSingleSTFTest(t, filename, string(content), BackendGoInterpreter, false)
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
	backend := BackendCompiler
	if runtime.GOOS != "linux" {
		backend = BackendGoInterpreter
	}
	//backend = BackendGoInterpreter
	t.Logf("Using backend: %s", backend)

	fileMap := make(map[string]string)

	jamConformancePath, err := GetFuzzReportsPath()
	if err != nil {
		t.Fatalf("failed to get fuzz reports path: %v", err)
	}

	// MC
	fileMap["1763371127"] = "fuzz-reports/0.7.1/traces/1763371127/00000237.bin" // 9 keys, assign, transfer
	fileMap["1763371379"] = "fuzz-reports/0.7.1/traces/1763371379/00000237.bin" // 9 keys, assign, transfer

	// MC DOUBLE CHECK
	fileMap["1763371098"] = "fuzz-reports/0.7.1/traces/1763371098/00000005.bin" // Ok
	fileMap["1763372279"] = "fuzz-reports/0.7.1/traces/1763372279/00000034.bin" // Ok
	fileMap["1763370844"] = "fuzz-reports/0.7.1/traces/1763370844/00000002.bin" // Ok
	fileMap["1763372314"] = "fuzz-reports/0.7.1/traces/1763372314/00000092.bin" // Ok

	// SN solved - hostRead memRead issue
	fileMap["1763371865"] = "fuzz-reports/0.7.1/traces/1763371865/00000017.bin" // 2 keys (C13 ValidatorStatistics+random) checkpoints + ram access
	fileMap["1763371341"] = "fuzz-reports/0.7.1/traces/1763371341/00000017.bin" // 2 keys (C13 ValidatorStatistics+random) checkpoints + ram access
	// SN solved - degenerate edge case
	fileMap["1763371531"] = "fuzz-reports/0.7.1/traces/1763371531/00000042.bin" // StateRoot 0x0000000000000000000000000000000000000000000000000000000000000000 != ParentStateRoot 0x3e569e90ec47319e1ffeabd8c7f5509811c03a748b9cd57b2c186aa8746e8636
	// SN solved - extrinsics used/num extrinsics swap in statistics issue
	fileMap["1763399245"] = "fuzz-reports/0.7.1/traces/1763399245/00000001.bin" // 1 key: C13 ValidatorStatistics
	// SN solved - new_check issue
	fileMap["1763371998"] = "fuzz-reports/0.7.1/traces/1763371998/00001108.bin" // 4 keys (Random)
	fileMap["1761553047"] = "fuzz-reports/0.7.1/traces/1761553047/00000006.bin"

	fileMap["1761654584"] = "fuzz-reports/0.7.1/traces/1761654584/00000053.bin"

	fileMap["1763488162"] = "fuzz-reports/0.7.1/traces/1763488162//00000017.bin"

	fileMap["1761653246"] = "fuzz-reports/0.7.1/traces/1761653246/00006616.bin"
	fileMap["1761653246-2"] = "fuzz-reports/0.7.1/traces/1761653246/00006617.bin"
	fileMap["1761661586"] = "fuzz-reports/0.7.1/traces/1761661586/00003911.bin"
	fileMap["1761661586-2"] = "fuzz-reports/0.7.1/traces/1761661586/00003912.bin"
	fileMap["1761662449"] = "fuzz-reports/0.7.1/traces/1761662449/00002753.bin"
	fileMap["1761662449-2"] = "fuzz-reports/0.7.1/traces/1761662449/00002754.bin"
	fileMap["1761663151"] = "fuzz-reports/0.7.1/traces/1761663151/00003910.bin"
	fileMap["1761663151-2"] = "fuzz-reports/0.7.1/traces/1761663151/00003911.bin"
	fileMap["1763488465"] = "fuzz-reports/0.7.1/traces/1763488465/00001705.bin"

	//TODO: C13, C16 mismatch
	fileMap["1763489798"] = "fuzz-reports/0.7.1/traces/1763489798//00000950.bin"
	PvmLogging = false
	//	DebugHostFunctions = true
	log.InitLogger("debug")
	// log.EnableModule(log.PvmAuthoring)
	// log.EnableModule("pvm_validator")
	log.EnableModule(log.SDB)

	//tc := []string{"1763371098", "1763372279", "1763370844", "1763372314", "1763488162"}
	//tc := []string{"1763371127", "1763371379", "1763371865", "1763371341", "1763371531", "1763399245", "1763371998", "1761654584", "1761553047", "1763371098", "1763372279", "1763370844", "1763372314"}
	//tc := []string{"1763489798"}
	tc := []string{"1761653246", "1761653246-2", "1761661586", "1761661586-2", "1761662449", "1761662449-2", "1761663151", "1761663151-2", "1763488465"}

	PvmLogging = false
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

func testFuzzTraceInternal(t *testing.T, saveOutput bool) {
	t.Helper()

	PvmLogging = false

	log.InitLogger("debug")
	log.EnableModule(log.PvmAuthoring)
	log.EnableModule("pvm_validator")

	targetVersion := "0.7.1"
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
	backend := BackendCompiler
	if runtime.GOOS != "linux" {
		backend = BackendGoInterpreter
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
				runSingleSTFTestAndSave(t, currentFile, string(content), backend, true, sourcePath, destinationPath)
			} else {
				runSingleSTFTest(t, currentFile, string(content), backend, true)
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
