# PVM Performance Benchmark

This package measures the performance of the PVM recompiler, providing statistics on compilation speed and code generation.

## Metrics

- **Compile Time**: Time taken to compile PVM bytecode to x86
- **PVM Instruction Count**: Total number of PVM instructions
- **PVM Basic Block Count**: Total number of basic blocks
- **X86 Instruction Count**: Total number of generated x86 instructions
- **X86 Code Size**: Size of generated x86 code in bytes
- **X86/PVM Ratio**: Expansion ratio from PVM to x86 instructions

## Usage

### Run Benchmark with Charts (Recommended)

Generates PNG charts and saves JSON results to `results/` directory:

```bash
cd pvm/performance
go test -v -run TestBenchmarkWithCharts
```

Output files in `results/`:
- `benchmark_YYYYMMDD_HHMMSS.json` - Full benchmark data
- `compile_time_YYYYMMDD_HHMMSS.png` - Compile time by file
- `instruction_count_YYYYMMDD_HHMMSS.png` - PVM vs X86 instruction comparison
- `basic_block_count_YYYYMMDD_HHMMSS.png` - Basic block count by file
- `x86_expansion_ratio_YYYYMMDD_HHMMSS.png` - X86/PVM instruction ratio
- `code_size_comparison_YYYYMMDD_HHMMSS.png` - PVM vs X86 code size (KB)
- `compile_speed_YYYYMMDD_HHMMSS.png` - Scatter plot of time vs instructions

### Run All Benchmarks (Table Format)

```bash
go test -v -run TestBenchmarkAllPVMFiles
```

### Run All Benchmarks (JSON Format)

```bash
go test -v -run TestBenchmarkAllPVMFilesJSON
```

### Sort by Compile Time (Slowest First)

```bash
go test -v -run TestBenchmarkSortedByCompileTime
```

### Sort by PVM Instruction Count (Largest First)

```bash
go test -v -run TestBenchmarkSortedByInstructionCount
```

## Programmatic Usage

```go
import "github.com/colorfulnotion/jam/pvm/performance"

// From raw PVM bytecode
stats, err := performance.BenchmarkFromRaw(rawCode)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Compile time: %s\n", stats.CompileTime)
fmt.Printf("PVM instructions: %d\n", stats.PVMInstructionCount)
fmt.Printf("PVM basic blocks: %d\n", stats.PVMBasicBlockCount)
fmt.Printf("X86 instructions: %d\n", stats.X86InstructionCount)

// From parsed components
stats, err := performance.Benchmark(code, bitmask, jumpTable)

// Calculate aggregate statistics
allStats := []*performance.CompileStats{stats1, stats2, ...}
agg := performance.CalculateAggregate(allStats)
fmt.Printf("Total compile time: %s\n", agg.TotalCompileTime)
fmt.Printf("Average compile time: %s\n", agg.AverageCompileTime)

// Generate charts
config := performance.DefaultChartConfig()
config.OutputDir = "my_results"
err = performance.GenerateAllCharts(allStats, config)
```

## Test Data

The benchmarks automatically scan all `.pvm` files in the `services/*/` directory.
