package performance

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/colorfulnotion/jam/pvm/program"
)

// TestBenchmarkAllPVMFiles runs performance benchmarks on all .pvm files in services directory
func TestBenchmarkAllPVMFiles(t *testing.T) {
	pattern := filepath.Join("..", "..", "services", "*", "*.pvm")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("Glob failed: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("No .pvm files found with pattern %s", pattern)
	}

	var allStats []*CompileStats

	// Print header
	fmt.Println("=" + strings.Repeat("=", 119))
	fmt.Printf("%-40s %12s %10s %10s %12s %12s %8s\n",
		"File", "CompileTime", "PVM Instr", "PVM BB", "X86 Instr", "X86 Bytes", "Ratio")
	fmt.Println("-" + strings.Repeat("-", 119))

	for _, path := range matches {
		if strings.Contains(path, "_blob") {
			continue
		}

		name := filepath.Base(path)

		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("Failed to read file %s: %v", path, err)
			continue
		}

		prog, _, _, _, _, _, _, err := program.DecodeProgram(data)
		if err != nil {
			t.Errorf("Failed to decode program %s: %v", path, err)
			continue
		}

		stats, err := Benchmark(prog.Code, prog.K, prog.J)
		if err != nil {
			t.Errorf("Benchmark failed for %s: %v", path, err)
			continue
		}

		stats.FileName = name
		stats.FilePath = path
		allStats = append(allStats, stats)

		// Print individual result
		fmt.Printf("%-40s %12s %10d %10d %12d %12d %8.2fx\n",
			truncateString(name, 40),
			stats.CompileTime,
			stats.PVMInstructionCount,
			stats.PVMBasicBlockCount,
			stats.X86InstructionCount,
			stats.X86CodeSize,
			stats.X86ToPVMInstructionRatio)
	}

	// Calculate and print aggregate statistics
	agg := CalculateAggregate(allStats)

	fmt.Println("-" + strings.Repeat("-", 119))
	fmt.Println("\nAggregate Statistics:")
	fmt.Println("=" + strings.Repeat("=", 60))
	fmt.Printf("Total files processed:          %d\n", agg.TotalFiles)
	fmt.Printf("Total compile time:             %s\n", agg.TotalCompileTime)
	fmt.Printf("Average compile time:           %s\n", agg.AverageCompileTime)
	fmt.Printf("Min compile time:               %s\n", agg.MinCompileTime)
	fmt.Printf("Max compile time:               %s\n", agg.MaxCompileTime)
	fmt.Println("-" + strings.Repeat("-", 60))
	fmt.Printf("Total PVM code size:            %d bytes\n", agg.TotalPVMCodeSize)
	fmt.Printf("Total PVM instructions:         %d\n", agg.TotalPVMInstructions)
	fmt.Printf("Total PVM basic blocks:         %d\n", agg.TotalPVMBasicBlocks)
	fmt.Println("-" + strings.Repeat("-", 60))
	fmt.Printf("Total X86 code size:            %d bytes\n", agg.TotalX86CodeSize)
	fmt.Printf("Total X86 instructions:         %d\n", agg.TotalX86Instructions)
	fmt.Println("-" + strings.Repeat("-", 60))
	fmt.Printf("Avg X86/PVM code ratio:         %.2fx\n", agg.AverageX86ToPVMCodeRatio)
	fmt.Printf("Avg X86/PVM instruction ratio:  %.2fx\n", agg.AverageX86ToPVMInstructionRatio)
	fmt.Println("=" + strings.Repeat("=", 60))
}

// TestBenchmarkAllPVMFilesJSON outputs results in JSON format
func TestBenchmarkAllPVMFilesJSON(t *testing.T) {
	pattern := filepath.Join("..", "..", "services", "*", "*.pvm")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("Glob failed: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("No .pvm files found with pattern %s", pattern)
	}

	var allStats []*CompileStats

	for _, path := range matches {
		if strings.Contains(path, "_blob") {
			continue
		}

		name := filepath.Base(path)

		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("Failed to read file %s: %v", path, err)
			continue
		}

		prog, _, _, _, _, _, _, err := program.DecodeProgram(data)
		if err != nil {
			t.Errorf("Failed to decode program %s: %v", path, err)
			continue
		}

		stats, err := Benchmark(prog.Code, prog.K, prog.J)
		if err != nil {
			t.Errorf("Benchmark failed for %s: %v", path, err)
			continue
		}

		stats.FileName = name
		stats.FilePath = path
		allStats = append(allStats, stats)
	}

	agg := CalculateAggregate(allStats)

	result := struct {
		Files     []*CompileStats `json:"files"`
		Aggregate *AggregateStats `json:"aggregate"`
	}{
		Files:     allStats,
		Aggregate: agg,
	}

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal JSON: %v", err)
	}

	fmt.Println(string(jsonData))
}

// TestBenchmarkSortedByCompileTime outputs results sorted by compile time
func TestBenchmarkSortedByCompileTime(t *testing.T) {
	pattern := filepath.Join("..", "..", "services", "*", "*.pvm")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("Glob failed: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("No .pvm files found with pattern %s", pattern)
	}

	var allStats []*CompileStats

	for _, path := range matches {
		if strings.Contains(path, "_blob") {
			continue
		}

		name := filepath.Base(path)

		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		prog, _, _, _, _, _, _, err := program.DecodeProgram(data)
		if err != nil {
			continue
		}

		stats, err := Benchmark(prog.Code, prog.K, prog.J)
		if err != nil {
			continue
		}

		stats.FileName = name
		stats.FilePath = path
		allStats = append(allStats, stats)
	}

	// Sort by compile time (descending)
	sort.Slice(allStats, func(i, j int) bool {
		return allStats[i].CompileTime > allStats[j].CompileTime
	})

	fmt.Println("\nResults sorted by compile time (slowest first):")
	fmt.Println("=" + strings.Repeat("=", 100))
	fmt.Printf("%-40s %12s %10s %10s %12s\n",
		"File", "CompileTime", "PVM Instr", "PVM BB", "X86 Instr")
	fmt.Println("-" + strings.Repeat("-", 100))

	for _, s := range allStats {
		fmt.Printf("%-40s %12s %10d %10d %12d\n",
			truncateString(s.FileName, 40),
			s.CompileTime,
			s.PVMInstructionCount,
			s.PVMBasicBlockCount,
			s.X86InstructionCount)
	}
}

// TestBenchmarkSortedByInstructionCount outputs results sorted by PVM instruction count
func TestBenchmarkSortedByInstructionCount(t *testing.T) {
	pattern := filepath.Join("..", "..", "services", "*", "*.pvm")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("Glob failed: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("No .pvm files found with pattern %s", pattern)
	}

	var allStats []*CompileStats

	for _, path := range matches {
		if strings.Contains(path, "_blob") {
			continue
		}

		name := filepath.Base(path)

		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		prog, _, _, _, _, _, _, err := program.DecodeProgram(data)
		if err != nil {
			continue
		}

		stats, err := Benchmark(prog.Code, prog.K, prog.J)
		if err != nil {
			continue
		}

		stats.FileName = name
		stats.FilePath = path
		allStats = append(allStats, stats)
	}

	// Sort by PVM instruction count (descending)
	sort.Slice(allStats, func(i, j int) bool {
		return allStats[i].PVMInstructionCount > allStats[j].PVMInstructionCount
	})

	fmt.Println("\nResults sorted by PVM instruction count (largest first):")
	fmt.Println("=" + strings.Repeat("=", 100))
	fmt.Printf("%-40s %12s %10s %10s %12s\n",
		"File", "CompileTime", "PVM Instr", "PVM BB", "X86 Instr")
	fmt.Println("-" + strings.Repeat("-", 100))

	for _, s := range allStats {
		fmt.Printf("%-40s %12s %10d %10d %12d\n",
			truncateString(s.FileName, 40),
			s.CompileTime,
			s.PVMInstructionCount,
			s.PVMBasicBlockCount,
			s.X86InstructionCount)
	}
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// TestBenchmarkWithCharts runs benchmarks and generates PNG charts
func TestBenchmarkWithCharts(t *testing.T) {
	pattern := filepath.Join("..", "..", "services", "*", "*.pvm")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("Glob failed: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("No .pvm files found with pattern %s", pattern)
	}

	var allStats []*CompileStats

	fmt.Println("Collecting benchmark data...")
	for _, path := range matches {
		if strings.Contains(path, "_blob") {
			continue
		}

		name := filepath.Base(path)

		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("Failed to read file %s: %v", path, err)
			continue
		}

		prog, _, _, _, _, _, _, err := program.DecodeProgram(data)
		if err != nil {
			t.Errorf("Failed to decode program %s: %v", path, err)
			continue
		}

		stats, err := Benchmark(prog.Code, prog.K, prog.J)
		if err != nil {
			t.Errorf("Benchmark failed for %s: %v", path, err)
			continue
		}

		stats.FileName = name
		stats.FilePath = path
		allStats = append(allStats, stats)
		fmt.Printf("  Processed: %s\n", name)
	}

	// Save JSON results
	resultsDir := "results"
	if err := os.MkdirAll(resultsDir, 0755); err != nil {
		t.Fatalf("Failed to create results directory: %v", err)
	}

	agg := CalculateAggregate(allStats)
	result := struct {
		Files     []*CompileStats `json:"files"`
		Aggregate *AggregateStats `json:"aggregate"`
	}{
		Files:     allStats,
		Aggregate: agg,
	}

	jsonPath := filepath.Join(resultsDir, "benchmark.json")
	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal JSON: %v", err)
	}
	if err := os.WriteFile(jsonPath, jsonData, 0644); err != nil {
		t.Fatalf("Failed to write JSON: %v", err)
	}
	fmt.Printf("\nSaved JSON results: %s\n", jsonPath)

	// Generate charts
	fmt.Println("\nGenerating charts...")
	config := DefaultChartConfig()
	config.OutputDir = resultsDir

	if err := GenerateAllCharts(allStats, config); err != nil {
		t.Fatalf("Failed to generate charts: %v", err)
	}

	// Print summary
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("Benchmark Summary")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Total files:              %d\n", agg.TotalFiles)
	fmt.Printf("Total compile time:       %s\n", agg.TotalCompileTime)
	fmt.Printf("Average compile time:     %s\n", agg.AverageCompileTime)
	fmt.Printf("Total PVM instructions:   %d\n", agg.TotalPVMInstructions)
	fmt.Printf("Total PVM basic blocks:   %d\n", agg.TotalPVMBasicBlocks)
	fmt.Printf("Total X86 instructions:   %d\n", agg.TotalX86Instructions)
	fmt.Printf("Avg expansion ratio:      %.2fx\n", agg.AverageX86ToPVMInstructionRatio)
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("\nResults saved to: %s/\n", resultsDir)
}
