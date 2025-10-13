package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/fuzz"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

// colors for terminal output - trying to match the fuzzer's style
var (
	colorReset     = "\033[0m"
	colorOriginal  = "\033[90m" // grey
	colorCompleted = "\033[90m" // same grey for timing msgs
	colorError     = "\033[31m"
	colorSuccess   = "\033[32m"
)

// defaultBackend can be set at build time via -ldflags "-X main.defaultBackend=compiler"
// Default is interpreter for compatibility
var defaultBackend = statedb.BackendInterpreter

func DisableColors() {
	// turn off colors for CI environments
	colorReset = ""
	colorOriginal = ""
	colorCompleted = ""
	colorError = ""
	colorSuccess = ""
}

type StateTransitionWithFilename struct {
	STF      *statedb.StateTransition
	Filename string
}

type Version struct {
	Major int `json:"major"`
	Minor int `json:"minor"`
	Patch int `json:"patch"`
}

type TargetInfo struct {
	AppName      string  `json:"name"`
	FuzzVersion  int     `json:"fuzz_version"`
	AppVersion   Version `json:"app_version"`
	JamVersion   Version `json:"jam_version"`
	FuzzFeatures int     `json:"features"`
}

type BenchStats struct {
	Steps      int `json:"steps"`
	Successful int `json:"successful"`
	Failed     int `json:"failed"`
	Errors     int `json:"errors"`
	// timing stats in ms
	ImportMinMs  float64 `json:"import_min_ms"`
	ImportMaxMs  float64 `json:"import_max_ms"`
	ImportMeanMs float64 `json:"import_mean_ms"`
	ImportP01Ms  float64 `json:"import_p01_ms"`
	ImportP10Ms  float64 `json:"import_p10_ms"`
	ImportP25Ms  float64 `json:"import_p25_ms"`
	ImportP50Ms  float64 `json:"import_p50_ms"`
	ImportP75Ms  float64 `json:"import_p75_ms"`
	ImportP90Ms  float64 `json:"import_p90_ms"`
	ImportP99Ms  float64 `json:"import_p99_ms"`
	ImportStdDev float64 `json:"import_std_dev_ms"`
}

type TransitionResult struct {
	Filename   string  `json:"filename"`
	TimeSlot   uint32  `json:"time_slot"`
	DurationMs float64 `json:"duration_ms"`
	Success    bool    `json:"success"`
	Error      string  `json:"error,omitempty"`
}

type JSONReport struct {
	Generated   string             `json:"generated"`
	TestDir     string             `json:"test_dir"`
	TotalTimeMs float64            `json:"total_time_ms"`
	Info        TargetInfo         `json:"info"`
	Stats       BenchStats         `json:"stats"`
	Results     []TransitionResult `json:"results"`
}

type BenchmarkStats struct {
	TotalTransitions  int
	SuccessfulRuns    int
	FailedRuns        int
	ErrorRuns         int
	TotalTime         time.Duration
	MinTime           time.Duration
	MaxTime           time.Duration
	TransitionTimes   []time.Duration
	TransitionFiles   []string
	TransitionSlots   []uint32
	TransitionSuccess []bool
	TransitionErrors  []error
	ErrorMessages     map[string]int
	TargetPeerInfo    fuzz.PeerInfo
}

func (bs *BenchmarkStats) AddResult(duration time.Duration, success bool, err error, filename string, timeSlot uint32) {
	if bs.ErrorMessages == nil {
		bs.ErrorMessages = make(map[string]int)
	}

	bs.TotalTransitions++
	bs.TransitionTimes = append(bs.TransitionTimes, duration)
	bs.TransitionFiles = append(bs.TransitionFiles, filename)
	bs.TransitionSlots = append(bs.TransitionSlots, timeSlot)
	bs.TransitionSuccess = append(bs.TransitionSuccess, success)
	bs.TransitionErrors = append(bs.TransitionErrors, err)
	bs.TotalTime += duration

	if err != nil {
		bs.ErrorRuns++
		bs.ErrorMessages[err.Error()]++
	} else if success {
		bs.SuccessfulRuns++
	} else {
		bs.FailedRuns++
	}

	if bs.MinTime == 0 || duration < bs.MinTime {
		bs.MinTime = duration
	}
	if duration > bs.MaxTime {
		bs.MaxTime = duration
	}
}

func (bs *BenchmarkStats) CalculateAverage() time.Duration {
	if bs.TotalTransitions == 0 {
		return 0
	}
	return bs.TotalTime / time.Duration(bs.TotalTransitions)
}

func (bs *BenchmarkStats) CalculateMedian() time.Duration {
	if len(bs.TransitionTimes) == 0 {
		return 0
	}
	// need to sort to find median
	times := make([]time.Duration, len(bs.TransitionTimes))
	copy(times, bs.TransitionTimes)
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })

	n := len(times)
	if n%2 == 0 {
		return (times[n/2-1] + times[n/2]) / 2
	}
	return times[n/2]
}

func (bs *BenchmarkStats) CalculatePercentile(p float64) time.Duration {
	if len(bs.TransitionTimes) == 0 {
		return 0
	}

	times := make([]time.Duration, len(bs.TransitionTimes))
	copy(times, bs.TransitionTimes)
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })

	n := len(times)
	index := p * float64(n-1)

	if index == float64(int(index)) {
		return times[int(index)]
	}

	lower := int(math.Floor(index))
	upper := int(math.Ceil(index))
	weight := index - math.Floor(index)

	return time.Duration(float64(times[lower]) + weight*float64(times[upper]-times[lower]))
}

func (bs *BenchmarkStats) CalculateStandardDeviation() float64 {
	if len(bs.TransitionTimes) == 0 {
		return 0
	}

	mean := bs.CalculateAverage()
	sum := 0.0

	for _, duration := range bs.TransitionTimes {
		diff := float64(duration - mean)
		sum += diff * diff
	}

	variance := sum / float64(len(bs.TransitionTimes))
	return math.Sqrt(variance)
}

func (bs *BenchmarkStats) GenerateJSONReport(testDir string, totalBenchmarkTime time.Duration) *JSONReport {
	// Create target info
	targetInfo := TargetInfo{
		AppName:    "unknown",
		AppVersion: Version{Major: 0, Minor: 0, Patch: 0},
		JamVersion: Version{Major: 0, Minor: 0, Patch: 0},
	}

	if bs.TargetPeerInfo != nil {
		bs.TargetPeerInfo.SetDefaults()
		targetInfo.AppName = bs.TargetPeerInfo.GetName()
		appVer := bs.TargetPeerInfo.GetAppVersion()
		targetInfo.AppVersion = Version{
			Major: int(appVer.Major),
			Minor: int(appVer.Minor),
			Patch: int(appVer.Patch),
		}
		jamVer := bs.TargetPeerInfo.GetJamVersion()
		targetInfo.JamVersion = Version{
			Major: int(jamVer.Major),
			Minor: int(jamVer.Minor),
			Patch: int(jamVer.Patch),
		}
	}

	// Calculate statistics
	stats := BenchStats{
		Steps:        bs.TotalTransitions,
		Successful:   bs.SuccessfulRuns,
		Failed:       bs.FailedRuns,
		Errors:       bs.ErrorRuns,
		ImportMinMs:  float64(bs.MinTime.Nanoseconds()) / 1e6,
		ImportMaxMs:  float64(bs.MaxTime.Nanoseconds()) / 1e6,
		ImportMeanMs: float64(bs.CalculateAverage().Nanoseconds()) / 1e6,
		ImportP01Ms:  float64(bs.CalculatePercentile(0.01).Nanoseconds()) / 1e6,
		ImportP10Ms:  float64(bs.CalculatePercentile(0.1).Nanoseconds()) / 1e6,
		ImportP25Ms:  float64(bs.CalculatePercentile(0.25).Nanoseconds()) / 1e6,
		ImportP50Ms:  float64(bs.CalculatePercentile(0.5).Nanoseconds()) / 1e6,
		ImportP75Ms:  float64(bs.CalculatePercentile(0.75).Nanoseconds()) / 1e6,
		ImportP90Ms:  float64(bs.CalculatePercentile(0.9).Nanoseconds()) / 1e6,
		ImportP99Ms:  float64(bs.CalculatePercentile(0.99).Nanoseconds()) / 1e6,
		ImportStdDev: bs.CalculateStandardDeviation() / 1e6, // Convert to milliseconds
	}

	// Create individual results
	results := make([]TransitionResult, len(bs.TransitionTimes))
	for i := range bs.TransitionTimes {
		result := TransitionResult{
			Filename:   bs.TransitionFiles[i],
			TimeSlot:   bs.TransitionSlots[i],
			DurationMs: float64(bs.TransitionTimes[i].Nanoseconds()) / 1e6,
			Success:    bs.TransitionSuccess[i],
		}

		if bs.TransitionErrors[i] != nil {
			result.Error = bs.TransitionErrors[i].Error()
		}

		results[i] = result
	}

	return &JSONReport{
		Generated:   time.Now().Format(time.RFC3339),
		TestDir:     testDir,
		TotalTimeMs: float64(totalBenchmarkTime.Nanoseconds()) / 1e6,
		Info:        targetInfo,
		Stats:       stats,
		Results:     results,
	}
}

func (bs *BenchmarkStats) DumpMetrics() string {
	avg := bs.CalculateAverage()
	median := bs.CalculateMedian()
	p1 := bs.CalculatePercentile(0.01)
	p10 := bs.CalculatePercentile(0.1)
	p25 := bs.CalculatePercentile(0.25)
	p75 := bs.CalculatePercentile(0.75)
	p90 := bs.CalculatePercentile(0.9)
	p99 := bs.CalculatePercentile(0.99)

	result := fmt.Sprintf(`Benchmark Results:
  Total Transitions: %d
  Successful: %d
  Failed: %d
  Errors: %d
  Total Time: %v
  Average Time: %v
  Median Time: %v
  Min Time: %v
  Max Time: %v
  P01 Time: %v
  P10 Time: %v
  P25 Time: %v
  P75 Time: %v
  P90 Time: %v
  P99 Time: %v
  Success Rate: %.2f%%`,
		bs.TotalTransitions,
		bs.SuccessfulRuns,
		bs.FailedRuns,
		bs.ErrorRuns,
		bs.TotalTime,
		avg,
		median,
		bs.MinTime,
		bs.MaxTime,
		p1,
		p10,
		p25,
		p75,
		p90,
		p99,
		float64(bs.SuccessfulRuns)/float64(bs.TotalTransitions)*100,
	)

	if len(bs.ErrorMessages) > 0 {
		result += "\n\nError Summary:"
		for errMsg, count := range bs.ErrorMessages {
			result += fmt.Sprintf("\n  %d: %s", count, errMsg)
		}
	}

	return result
}

func validateBenchConfig(jConfig types.ConfigJamBlocks) {
	if jConfig.Network != "tiny" {
		log.Fatalf("Invalid --network value: %s. Must be 'tiny'.", jConfig.Network)
	}
}

func readStateTransitionsWithFilenames(baseDir string) ([]*StateTransitionWithFilename, error) {
	stfs := make([]*StateTransitionWithFilename, 0)
	stFiles, err := os.ReadDir(baseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %v", err)
	}

	fmt.Printf("Selected Dir: %v\n", baseDir)

	for _, file := range stFiles {
		if strings.HasSuffix(file.Name(), ".bin") || strings.HasSuffix(file.Name(), ".json") {
			stPath := filepath.Join(baseDir, file.Name())

			stf, err := fuzz.ReadStateTransition(stPath)
			if err != nil {
				log.Printf("Error reading state transition file %s: %v\n", file.Name(), err)
				continue
			}

			stfWithFilename := &StateTransitionWithFilename{
				STF:      stf,
				Filename: file.Name(),
			}
			stfs = append(stfs, stfWithFilename)
		}
	}

	return stfs, nil
}

func deduplicateStateTransitionsWithFilenames(stfs []*StateTransitionWithFilename) []*StateTransitionWithFilename {
	seen := make(map[string]*StateTransitionWithFilename)

	for _, stfWithFilename := range stfs {
		// Use shared deduplication key logic for consistency
		key := fuzz.CreateSTFDeduplicationKey(stfWithFilename.STF)

		// first one wins for dedup
		if _, exists := seen[key]; !exists {
			seen[key] = stfWithFilename
		}
	}

	// back to slice form
	deduplicated := make([]*StateTransitionWithFilename, 0, len(seen))
	for _, stfWithFilename := range seen {
		deduplicated = append(deduplicated, stfWithFilename)
	}

	return deduplicated
}

func getSequentialStateTransitions(test_dir string) ([]*StateTransitionWithFilename, []*statedb.StateTransition, error) {
	all_stfs_with_filenames, err := readStateTransitionsWithFilenames(test_dir)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read raw state transitions: %v", err)
	}

	// dedup based on prestate->poststate roots
	deduplicated_with_filenames := deduplicateStateTransitionsWithFilenames(all_stfs_with_filenames)
	fmt.Printf("Loaded %d state transitions, deduplicated to %d\n", len(all_stfs_with_filenames), len(deduplicated_with_filenames))

	// need raw stfs for parent checking and socket communication
	raw_stfs := make([]*statedb.StateTransition, 0, len(deduplicated_with_filenames))
	for _, stfWithFilename := range deduplicated_with_filenames {
		raw_stfs = append(raw_stfs, stfWithFilename.STF)
	}

	usable_stfs_with_filenames := make([]*StateTransitionWithFilename, 0)
	for _, stfWithFilename := range deduplicated_with_filenames {
		if fuzz.HasParentStfs(raw_stfs, stfWithFilename.STF) {
			usable_stfs_with_filenames = append(usable_stfs_with_filenames, stfWithFilename)
		}
	}

	sort.Slice(usable_stfs_with_filenames, func(i, j int) bool {
		return usable_stfs_with_filenames[i].STF.Block.TimeSlot() < usable_stfs_with_filenames[j].STF.Block.TimeSlot()
	})

	return usable_stfs_with_filenames, raw_stfs, nil
}

func benchmarkStateTransition(fuzzer *fuzz.Fuzzer, stf *statedb.StateTransition, jConfig types.ConfigJamBlocks, useUnixSocket bool, allStfs []*statedb.StateTransition, filename string) (time.Duration, bool, error) {
	stfQA := fuzz.StateTransitionQA{
		STF:     stf,
		Mutated: false,
	}

	var importBlockDuration time.Duration
	var success bool
	var err error

	if useUnixSocket {
		// socket path - time only ImportBlock
		importBlockDuration, success, err = benchmarkImportBlockOnly(fuzzer, &stfQA, jConfig.Verbose, allStfs, filename)
	} else {
		// http fallback method
		startTime := time.Now()
		stfChallenge := stfQA.ToChallenge()
		postSTResp, respOK, httpErr := fuzzer.SendStateTransitionChallenge(jConfig.HTTP, stfChallenge)
		err = httpErr
		importBlockDuration = time.Since(startTime)

		// Show combined result at the end
		log.Printf("%sB#%03d (%s) Completed in %.3fms%s", colorCompleted, stf.Block.Header.Slot, filename, float64(importBlockDuration.Nanoseconds())/1e6, colorReset)
		fmt.Println("")

		if respOK {
			isMatch, _ := fuzzer.ValidateStateTransitionChallengeResponse(&stfQA, postSTResp)
			solverFuzzed := postSTResp.Mutated
			challengerFuzzed := stfQA.Mutated
			success = !challengerFuzzed && !solverFuzzed && isMatch
		} else {
			success = false
		}
	}

	return importBlockDuration, success, err
}

func benchmarkImportBlockOnly(fuzzer *fuzz.Fuzzer, stfQA *fuzz.StateTransitionQA, verbose bool, allStfs []*statedb.StateTransition, filename string) (time.Duration, bool, error) {
	// Setup initial state (not timed)
	initialStatePayload := &fuzz.HeaderWithState{
		State: statedb.StateKeyVals{KeyVals: stfQA.STF.PreState.KeyVals},
	}
	parentBlock := fuzz.FindParentBlock(allStfs, stfQA)
	if parentBlock != nil {
		initialStatePayload.Header = parentBlock.Header
	} else {
		return 0, false, fmt.Errorf("parent block not found for STF: %s", stfQA.STF.Block.Header.HeaderHash().Hex())
	}

	expectedPreStateRoot := stfQA.STF.PreState.StateRoot

	// Initialize or SetState based on protocol version (not timed)
	maxAncestryDepth := 24
	ancestry := fuzz.BuildAncestry(allStfs, &stfQA.STF.Block, maxAncestryDepth)
	targetPreStateRoot, err := fuzzer.InitializeOrSetState(initialStatePayload, ancestry)
	if err != nil {
		return 0, false, fmt.Errorf("InitializeOrSetState failed: %w", err)
	}

	// Verify pre-state
	if !bytes.Equal(targetPreStateRoot.Bytes(), expectedPreStateRoot.Bytes()) {
		if verbose {
			log.Printf("Pre-state root MISMATCH! Got: %s, Want: %s", targetPreStateRoot.Hex(), expectedPreStateRoot.Hex())
		}
		return 0, false, fmt.Errorf("pre-state root mismatch")
	}

	blockToProcess := &stfQA.STF.Block
	expectedPostStateRoot := stfQA.STF.PostState.StateRoot

	// TIME ONLY THE IMPORTBLOCK OPERATION
	startTime := time.Now()
	targetPostStateRoot, err := fuzzer.ImportBlock(blockToProcess)
	importDuration := time.Since(startTime)

	// Show combined test result and timing in grey text at the end
	if stfQA.Mutated {
		log.Printf("%sFUZZED B#%03d (%s) Completed in %.3fms%s", colorCompleted, stfQA.STF.Block.Header.Slot, filename, float64(importDuration.Nanoseconds())/1e6, colorReset)
	} else {
		log.Printf("%sORIGINAL B#%03d (%s) Completed in %.3fms%s", colorCompleted, stfQA.STF.Block.Header.Slot, filename, float64(importDuration.Nanoseconds())/1e6, colorReset)
	}

	// Add empty line between tests for better readability
	fmt.Println("")

	if err != nil {
		return importDuration, false, fmt.Errorf("ImportBlock failed: %w", err)
	}

	// Determine success based on whether solver detected it as fuzzed
	solverFuzzed := bytes.Equal(targetPostStateRoot.Bytes(), expectedPreStateRoot.Bytes())
	challengerFuzzed := stfQA.Mutated

	// For original data: success = !challengerFuzzed && !solverFuzzed && (post-state matches expected)
	isMatch := bytes.Equal(targetPostStateRoot.Bytes(), expectedPostStateRoot.Bytes())
	success := !challengerFuzzed && !solverFuzzed && isMatch

	return importDuration, success, err
}

// custom log writer - formats with ms precision
type customLogWriter struct{}

func (w *customLogWriter) Write(p []byte) (n int, err error) {
	ts := time.Now().Format("2006/01/02 15:04:05.000")
	msg := string(p)

	// avoid double newlines
	if strings.HasSuffix(msg, "\n") {
		msg = strings.TrimSuffix(msg, "\n")
	}

	fmt.Fprintf(os.Stderr, "%s %s\n", ts, msg)
	return len(p), nil
}

/*
boka		jampy		jamzig		javajam		perf-process.sh	pyjamaz		turbojam
jamduna		jamts		jamzilla	perf		polkajam	spacejam	vinwolf
*/
// helper to convert target name to simple form for filename
func getTargetNameForFile(targetName string) string {

	if strings.Contains(targetName, "duna") {
		return "jamduna"
	}
	if strings.Contains(targetName, "turbo") {
		return "turbojam"
	}

	if strings.Contains(targetName, "polkajam") && strings.Contains(targetName, "compiler") {
		return "polkajam"
	}

	if strings.Contains(targetName, "polkajam") && strings.Contains(targetName, "int") {
		return "polkajam_perf_it"
	}
	if strings.Contains(targetName, "turbo") {
		return "turbojam"
	}

	if strings.Contains(targetName, "boka") {
		return "boka"
	}

	if strings.Contains(targetName, "jampy") {
		return "jampy"
	}
	if strings.Contains(targetName, "jamts") {
		return "jamts"
	}
	if strings.Contains(targetName, "zig") {
		return "jamzig"
	}
	if strings.Contains(targetName, "zilla") {
		return "jamzzilla"
	}
	if strings.Contains(targetName, "java") {
		return "javajam"
	}

	if strings.Contains(targetName, "jamaz") {
		return "pyjamaz"
	}

	if strings.Contains(targetName, "spacejam") {
		return "spacejam"
	}

	if strings.Contains(targetName, "vinwolf") {
		return "vinwolf"
	}
	// fallback - just remove dashes and convert to lowercase
	return strings.ToLower(strings.ReplaceAll(targetName, "-", ""))
}

func main() {
	// setup custom logger w/ ms precision
	log.SetFlags(0)
	log.SetOutput(&customLogWriter{})

	dir := "/tmp/importBlock"
	enableRPC := false
	useUnixSocket := true
	test_dir := "./rawdata"
	report_dir := "./reports"
	max_transitions := 0
	no_color := false
	version := false

	jConfig := types.ConfigJamBlocks{
		HTTP:        "http://localhost:8088/",
		Socket:      "/tmp/jam_target.sock",
		Verbose:     false,
		NumBlocks:   50,
		InvalidRate: 0,
		Statistics:  50,
		Network:     "tiny",
		PVMBackend:  defaultBackend,
		Seed:        "0x44554E41",
	}

	fReg := fuzz.NewFlagRegistry("duna_bench")
	fReg.RegisterFlag("seed", nil, jConfig.Seed, "Seed for random number generation (as hex)", &jConfig.Seed)
	fReg.RegisterFlag("network", "n", jConfig.Network, "JAM network size", &jConfig.Network)
	fReg.RegisterFlag("verbose", nil, jConfig.Verbose, "Enable detailed logging", &jConfig.Verbose)
	fReg.RegisterFlag("statistics", nil, jConfig.Statistics, "Print statistics interval", &jConfig.Statistics)
	fReg.RegisterFlag("dir", nil, dir, "Storage directory", &dir)
	fReg.RegisterFlag("test-dir", nil, test_dir, "Test data directory", &test_dir)
	fReg.RegisterFlag("report-dir", nil, report_dir, "Report directory", &report_dir)
	fReg.RegisterFlag("socket", nil, jConfig.Socket, "Path for the Unix domain socket to connect to", &jConfig.Socket)
	fReg.RegisterFlag("use-unix-socket", nil, useUnixSocket, "Enable to use Unix domain socket for communication", &useUnixSocket)
	fReg.RegisterFlag("pvm-backend", nil, jConfig.PVMBackend, "PVM backend to use (Compiler or Interpreter)", &jConfig.PVMBackend)
	fReg.RegisterFlag("max-transitions", nil, max_transitions, "Maximum number of transitions to benchmark (0 = all)", &max_transitions)
	fReg.RegisterFlag("no-color", nil, no_color, "Disable terminal colors", &no_color)

	fReg.RegisterFlag("version", "v", version, "Display version information", &version)
	fReg.ProcessRegistry()

	benchInfo := fuzz.CreatePeerInfo("duna-bench")
	benchInfo.SetDefaults()

	fmt.Printf("Duna Bench Info:\b\n%s\n\n", benchInfo.PrettyString(false))
	if version {
		return
	}

	fmt.Printf("Config: %v\n", jConfig)
	fmt.Printf("Max transitions: %d (0 = all)\n", max_transitions)

	validateBenchConfig(jConfig)

	// Disable colors if requested
	if no_color {
		DisableColors()
	}

	fuzzer, err := fuzz.NewFuzzer(dir, report_dir, jConfig.Socket, benchInfo, jConfig.PVMBackend)
	if err != nil {
		log.Fatalf("Failed to initialize fuzzer: %v", err)
	}

	seedBytes := common.FromHex(jConfig.Seed)
	fuzzer.SetSeed(seedBytes)

	if enableRPC {
		go func() {
			log.Println("Starting RPC server...")
			fuzzer.RunImplementationRPCServer()
		}()
	}

	usable_stfs_with_filenames, raw_stfs, err := getSequentialStateTransitions(test_dir)
	if err != nil {
		log.Fatalf("Failed to get sequential state transitions: %v", err)
	}

	if len(usable_stfs_with_filenames) == 0 {
		log.Fatalf("No test data available in BaseDir=%v", test_dir)
	}

	if max_transitions > 0 && max_transitions < len(usable_stfs_with_filenames) {
		usable_stfs_with_filenames = usable_stfs_with_filenames[:max_transitions]
		fmt.Printf("Limited to first %d transitions\n", max_transitions)
	}

	fmt.Printf("Found %d sequential state transitions to benchmark\n", len(usable_stfs_with_filenames))

	stats := &BenchmarkStats{}

	if useUnixSocket {
		if err := fuzzer.Connect(); err != nil {
			log.Fatalf("Failed to connect to target: %v", jConfig.Socket)
		}
		defer fuzzer.Close()

		peerInfo, err := fuzzer.Handshake()
		if err != nil {
			log.Fatalf("Handshake failed: %v", err)
		} else {
			log.Printf("Handshake successful: %s", peerInfo.PrettyString(false))
			stats.TargetPeerInfo = peerInfo
		}
	}

	log.Println("[INFO] Starting sequential benchmark of state transitions...")
	benchmarkStart := time.Now()

	for idx, stfWithFilename := range usable_stfs_with_filenames {
		stf := stfWithFilename.STF
		filename := stfWithFilename.Filename

		if jConfig.Verbose {
			fmt.Printf("Benchmarking %s (%d/%d): TimeSlot %d\n",
				filename, idx+1, len(usable_stfs_with_filenames), stf.Block.TimeSlot())
		}

		duration, success, err := benchmarkStateTransition(fuzzer, stf, jConfig, useUnixSocket, raw_stfs, filename)
		stats.AddResult(duration, success, err, filename, stf.Block.TimeSlot())

		if err != nil && jConfig.Verbose {
			log.Printf("%s%s failed: %v%s", colorError, filename, err, colorReset)
		}

		if (idx+1)%jConfig.Statistics == 0 {
			log.Printf("Progress: %d/%d transitions completed\n", idx+1, len(usable_stfs_with_filenames))
			//log.Printf("Current Stats:\n%s\n", stats.DumpMetrics())
			//fmt.Printf("\n")
		}

		time.Sleep(10 * time.Millisecond)
	}

	totalBenchmarkTime := time.Since(benchmarkStart)

	log.Printf("Benchmark completed in %.2fs\n", totalBenchmarkTime.Seconds())
	log.Printf("Final Results:\n%s\n", stats.DumpMetrics())

	testDirName := filepath.Base(test_dir)
	reportSubDir := filepath.Join(report_dir, testDirName)
	reportAllDir := filepath.Join(report_dir, "all")
	os.MkdirAll(reportSubDir, 0755)
	os.MkdirAll(reportAllDir, 0755)

	timestamp := time.Now().Unix()

	// build filename with target info
	targetName := "unknown"
	if stats.TargetPeerInfo != nil {
		targetName = getTargetNameForFile(stats.TargetPeerInfo.GetName())
	}

	// Generate JSON report
	jsonReport := stats.GenerateJSONReport(test_dir, totalBenchmarkTime)
	jsonBytes, err := json.MarshalIndent(jsonReport, "", "  ")
	if err != nil {
		log.Printf("Error generating JSON report: %v", err)
	} else {
		filename := fmt.Sprintf("report_%s_%s_%d.json", testDirName, targetName, timestamp)

		jsonReportFile := filepath.Join(reportSubDir, filename)
		if err := os.WriteFile(jsonReportFile, jsonBytes, 0644); err != nil {
			log.Printf("Warning: Failed to write JSON report file: %v", err)
		} else {
			log.Printf("JSON benchmark report saved to: %s", jsonReportFile)
		}

		jsonReportAllFile := filepath.Join(reportAllDir, filename)
		if err := os.WriteFile(jsonReportAllFile, jsonBytes, 0644); err != nil {
			log.Printf("Warning: Failed to write JSON report file to all directory: %v", err)
		}
	}

	// Generate text report
	textFilename := fmt.Sprintf("report_%s_%s_%d.txt", testDirName, targetName, timestamp)

	// Get target info for text report
	var targetInfoText string
	if stats.TargetPeerInfo != nil {
		targetInfoText = fmt.Sprintf("Target Info:\n%s", stats.TargetPeerInfo.Info())
	} else {
		targetInfoText = "Target Info: Unknown"
	}

	reportContent := fmt.Sprintf(`Duna Bench Report
Generated: %s
Total Benchmark Time: %v
Test Directory: %s

%s

%s

Per-Transition Results:
`, time.Now().Format(time.RFC3339), totalBenchmarkTime, test_dir, targetInfoText, stats.DumpMetrics())

	for i, duration := range stats.TransitionTimes {
		filename := "unknown"
		if i < len(stats.TransitionFiles) {
			filename = stats.TransitionFiles[i]
		}
		reportContent += fmt.Sprintf("%s: %v\n", filename, duration)
	}

	textReportFile := filepath.Join(reportSubDir, textFilename)
	if err := os.WriteFile(textReportFile, []byte(reportContent), 0644); err != nil {
		log.Printf("Warning: Failed to write text report file: %v", err)
	}

	textReportAllFile := filepath.Join(reportAllDir, textFilename)
	if err := os.WriteFile(textReportAllFile, []byte(reportContent), 0644); err != nil {
		log.Printf("Warning: Failed to write text report file to all directory: %v", err)
	}
}
