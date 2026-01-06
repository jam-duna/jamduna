package performance

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/colorfulnotion/jam/pvm/program"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

const (
	defaultBackendRuns   = 3
	defaultInitialGas    = 1_000_000_000
	defaultOutputDirName = "results"
)

func VM_EXECUTE(code []byte, backend string, initialGas uint64, arg []byte, jamReady bool) (duration time.Duration, result uint8) {
	hostENV := statedb.NewMockHostEnv()
	vm := statedb.NewVM(0, code, make([]uint64, 13), 0, 4096, hostENV, jamReady, []byte{}, backend, initialGas)
	if vm == nil {
		return 0, types.WORKDIGEST_PANIC
	}
	start := time.Now()
	vm.ExecuteWithBackend(arg, 0, "")
	duration = time.Since(start)
	result = vm.GetResultCode()
	return duration, result
}

func TestArithmeticBenchBackendPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance benchmark in short mode.")
	}

	pvmPath := filepath.Join("..", "..", "services", "test_services", "arithmetic-bench", "arithmetic-bench.pvm")
	if _, err := os.Stat(pvmPath); os.IsNotExist(err) {
		t.Skipf("arithmetic-bench.pvm not found at %s. Run 'make test-services' first.", pvmPath)
	}

	code, err := os.ReadFile(pvmPath)
	if err != nil {
		t.Fatalf("Failed to read %s: %v", pvmPath, err)
	}

	codeForVM := code
	jamReady := false
	if _, _, _, _, _, _, _, err := program.DecodeProgram(codeForVM); err == nil {
		jamReady = true
	} else {
		if _, stripped := types.SplitMetadataAndCode(codeForVM); len(stripped) != len(codeForVM) {
			if _, _, _, _, _, _, _, err := program.DecodeProgram(stripped); err == nil {
				codeForVM = stripped
				jamReady = true
			}
		}
	}

	iterations, err := parseBenchIterations()
	if err != nil {
		t.Fatalf("Failed to parse iterations: %v", err)
	}

	runs, err := parseBenchRuns()
	if err != nil {
		t.Fatalf("Failed to parse run count: %v", err)
	}

	initialGas, err := parseBenchInitialGas()
	if err != nil {
		t.Fatalf("Failed to parse initial gas: %v", err)
	}

	backends := []string{
		statedb.BackendInterpreter,
		statedb.BackendCompiler,
	}

	var results []BackendBenchResult

	for _, iter := range iterations {
		arg := make([]byte, 8)
		binary.LittleEndian.PutUint64(arg, iter)

		for _, backend := range backends {
			runDurations := make([]int64, 0, runs)
			for i := 0; i < runs; i++ {
				duration, result := VM_EXECUTE(codeForVM, backend, initialGas, arg, jamReady)
				if result != types.WORKDIGEST_OK {
					t.Fatalf("Backend %s failed at iter=%d: result=%d", backend, iter, result)
				}
				runDurations = append(runDurations, duration.Nanoseconds())
			}

			avgNs, minNs, maxNs := summarizeDurations(runDurations)
			results = append(results, BackendBenchResult{
				Backend:        backend,
				Iterations:     iter,
				RunDurationsNs: runDurations,
				AvgNs:          avgNs,
				MinNs:          minNs,
				MaxNs:          maxNs,
			})

			t.Logf("backend=%s iterations=%d avg=%s min=%s max=%s",
				backend,
				iter,
				time.Duration(avgNs),
				time.Duration(minNs),
				time.Duration(maxNs),
			)
		}
	}

	report := BackendBenchReport{
		Program:          "arithmetic-bench",
		ProgramPath:      pvmPath,
		InitialGas:       initialGas,
		RunsPerIteration: runs,
		Iterations:       iterations,
		Backends:         backends,
		Results:          results,
		GeneratedAt:      time.Now().UTC(),
	}

	resultsDir := defaultOutputDirName
	if err := os.MkdirAll(resultsDir, 0755); err != nil {
		t.Fatalf("Failed to create results directory: %v", err)
	}

	jsonPath := filepath.Join(resultsDir, "backend_performance.json")
	jsonData, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal JSON: %v", err)
	}
	if err := os.WriteFile(jsonPath, jsonData, 0644); err != nil {
		t.Fatalf("Failed to write JSON: %v", err)
	}
	t.Logf("Saved JSON results: %s", jsonPath)

	config := DefaultChartConfig()
	config.OutputDir = resultsDir

	runtimeChart := filepath.Join(resultsDir, "backend_runtime.png")
	if err := GenerateBackendRuntimeChart(results, config, runtimeChart); err != nil {
		t.Fatalf("Failed to generate runtime chart: %v", err)
	}
	t.Logf("Saved runtime chart: %s", runtimeChart)

	speedupChart := filepath.Join(resultsDir, "backend_speedup.png")
	if err := GenerateBackendSpeedupChart(results, config, speedupChart, statedb.BackendInterpreter, statedb.BackendCompiler); err != nil {
		t.Fatalf("Failed to generate speedup chart: %v", err)
	}
	t.Logf("Saved speedup chart: %s", speedupChart)
}

func TestMemoryBenchBackendPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance benchmark in short mode.")
	}

	pvmPath := filepath.Join("..", "..", "services", "test_services", "memory-bench", "memory-bench.pvm")
	if _, err := os.Stat(pvmPath); os.IsNotExist(err) {
		t.Skipf("memory-bench.pvm not found at %s. Run 'make test-services' first.", pvmPath)
	}

	code, err := os.ReadFile(pvmPath)
	if err != nil {
		t.Fatalf("Failed to read %s: %v", pvmPath, err)
	}

	codeForVM := code
	jamReady := false
	if _, _, _, _, _, _, _, err := program.DecodeProgram(codeForVM); err == nil {
		jamReady = true
	} else {
		if _, stripped := types.SplitMetadataAndCode(codeForVM); len(stripped) != len(codeForVM) {
			if _, _, _, _, _, _, _, err := program.DecodeProgram(stripped); err == nil {
				codeForVM = stripped
				jamReady = true
			}
		}
	}

	iterations, err := parseBenchIterations()
	if err != nil {
		t.Fatalf("Failed to parse iterations: %v", err)
	}

	runs, err := parseBenchRuns()
	if err != nil {
		t.Fatalf("Failed to parse run count: %v", err)
	}

	initialGas, err := parseBenchInitialGas()
	if err != nil {
		t.Fatalf("Failed to parse initial gas: %v", err)
	}

	memSize, err := parseMemoryBenchSize()
	if err != nil {
		t.Fatalf("Failed to parse memory size: %v", err)
	}

	memStride, err := parseMemoryBenchStride()
	if err != nil {
		t.Fatalf("Failed to parse memory stride: %v", err)
	}

	backends := []string{
		statedb.BackendInterpreter,
		statedb.BackendCompiler,
	}

	var results []BackendBenchResult

	for _, iter := range iterations {
		arg := buildMemoryBenchArgs(iter, memSize, memStride)

		for _, backend := range backends {
			runDurations := make([]int64, 0, runs)
			for i := 0; i < runs; i++ {
				duration, result := VM_EXECUTE(codeForVM, backend, initialGas, arg, jamReady)
				if result != types.WORKDIGEST_OK {
					t.Fatalf("Backend %s failed at iter=%d: result=%d", backend, iter, result)
				}
				runDurations = append(runDurations, duration.Nanoseconds())
			}

			avgNs, minNs, maxNs := summarizeDurations(runDurations)
			results = append(results, BackendBenchResult{
				Backend:        backend,
				Iterations:     iter,
				RunDurationsNs: runDurations,
				AvgNs:          avgNs,
				MinNs:          minNs,
				MaxNs:          maxNs,
			})

			t.Logf("backend=%s iterations=%d size=%d stride=%d avg=%s min=%s max=%s",
				backend,
				iter,
				memSize,
				memStride,
				time.Duration(avgNs),
				time.Duration(minNs),
				time.Duration(maxNs),
			)
		}
	}

	report := BackendBenchReport{
		Program:          "memory-bench",
		ProgramPath:      pvmPath,
		InitialGas:       initialGas,
		RunsPerIteration: runs,
		Iterations:       iterations,
		Backends:         backends,
		Results:          results,
		GeneratedAt:      time.Now().UTC(),
	}

	resultsDir := defaultOutputDirName
	if err := os.MkdirAll(resultsDir, 0755); err != nil {
		t.Fatalf("Failed to create results directory: %v", err)
	}

	jsonPath := filepath.Join(resultsDir, "memory_backend_performance.json")
	jsonData, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal JSON: %v", err)
	}
	if err := os.WriteFile(jsonPath, jsonData, 0644); err != nil {
		t.Fatalf("Failed to write JSON: %v", err)
	}
	t.Logf("Saved JSON results: %s", jsonPath)

	config := DefaultChartConfig()
	config.OutputDir = resultsDir

	runtimeChart := filepath.Join(resultsDir, "memory_backend_runtime.png")
	if err := GenerateBackendRuntimeChart(results, config, runtimeChart); err != nil {
		t.Fatalf("Failed to generate runtime chart: %v", err)
	}
	t.Logf("Saved runtime chart: %s", runtimeChart)

	speedupChart := filepath.Join(resultsDir, "memory_backend_speedup.png")
	if err := GenerateBackendSpeedupChart(results, config, speedupChart, statedb.BackendInterpreter, statedb.BackendCompiler); err != nil {
		t.Fatalf("Failed to generate speedup chart: %v", err)
	}
	t.Logf("Saved speedup chart: %s", speedupChart)
}

func parseBenchIterations() ([]uint64, error) {
	env := strings.TrimSpace(os.Getenv("PVM_BENCH_ITERS"))
	if env == "" {
		return buildArithmeticIterations(50_000, 1_000_000, 50_000), nil
	}

	if strings.Count(env, ":") == 2 {
		parts := strings.SplitN(env, ":", 3)
		start, err := strconv.ParseUint(strings.TrimSpace(parts[0]), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid start iteration %q: %w", parts[0], err)
		}
		end, err := strconv.ParseUint(strings.TrimSpace(parts[1]), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid end iteration %q: %w", parts[1], err)
		}
		step, err := strconv.ParseUint(strings.TrimSpace(parts[2]), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid step iteration %q: %w", parts[2], err)
		}
		if step == 0 {
			return nil, fmt.Errorf("iteration step must be > 0")
		}
		if start == 0 || end == 0 {
			return nil, fmt.Errorf("iteration values must be > 0")
		}
		if start > end {
			return nil, fmt.Errorf("start iteration must be <= end iteration")
		}
		return buildArithmeticIterations(start, end, step), nil
	}

	parts := strings.Split(env, ",")
	iterations := make([]uint64, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		val, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid iteration %q: %w", part, err)
		}
		if val == 0 {
			return nil, fmt.Errorf("iteration value must be > 0")
		}
		iterations = append(iterations, val)
	}

	if len(iterations) == 0 {
		return nil, fmt.Errorf("no iterations provided in PVM_BENCH_ITERS")
	}
	return iterations, nil
}

func parseBenchRuns() (int, error) {
	env := strings.TrimSpace(os.Getenv("PVM_BENCH_RUNS"))
	if env == "" {
		return defaultBackendRuns, nil
	}
	val, err := strconv.Atoi(env)
	if err != nil {
		return 0, fmt.Errorf("invalid PVM_BENCH_RUNS: %w", err)
	}
	if val <= 0 {
		return 0, fmt.Errorf("PVM_BENCH_RUNS must be > 0")
	}
	return val, nil
}

func parseBenchInitialGas() (uint64, error) {
	env := strings.TrimSpace(os.Getenv("PVM_BENCH_GAS"))
	if env == "" {
		return defaultInitialGas, nil
	}
	val, err := strconv.ParseUint(env, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid PVM_BENCH_GAS: %w", err)
	}
	if val == 0 {
		return 0, fmt.Errorf("PVM_BENCH_GAS must be > 0")
	}
	return val, nil
}

func parseMemoryBenchSize() (uint64, error) {
	env := strings.TrimSpace(os.Getenv("PVM_MEM_BENCH_SIZE"))
	if env == "" {
		return 64 * 1024, nil
	}
	val, err := strconv.ParseUint(env, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid PVM_MEM_BENCH_SIZE: %w", err)
	}
	if val == 0 {
		return 0, fmt.Errorf("PVM_MEM_BENCH_SIZE must be > 0")
	}
	return val, nil
}

func parseMemoryBenchStride() (uint64, error) {
	env := strings.TrimSpace(os.Getenv("PVM_MEM_BENCH_STRIDE"))
	if env == "" {
		return 64, nil
	}
	val, err := strconv.ParseUint(env, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid PVM_MEM_BENCH_STRIDE: %w", err)
	}
	if val == 0 {
		return 0, fmt.Errorf("PVM_MEM_BENCH_STRIDE must be > 0")
	}
	return val, nil
}

func summarizeDurations(durations []int64) (avg int64, min int64, max int64) {
	if len(durations) == 0 {
		return 0, 0, 0
	}
	min = durations[0]
	max = durations[0]
	var sum int64
	for _, d := range durations {
		sum += d
		if d < min {
			min = d
		}
		if d > max {
			max = d
		}
	}
	avg = sum / int64(len(durations))
	return avg, min, max
}

func buildArithmeticIterations(start, end, step uint64) []uint64 {
	iterations := make([]uint64, 0)
	for v := start; v <= end; v += step {
		iterations = append(iterations, v)
	}
	return iterations
}

func buildMemoryBenchArgs(iters uint64, size uint64, stride uint64) []byte {
	arg := make([]byte, 24)
	binary.LittleEndian.PutUint64(arg[0:8], iters)
	binary.LittleEndian.PutUint64(arg[8:16], size)
	binary.LittleEndian.PutUint64(arg[16:24], stride)
	return arg
}
