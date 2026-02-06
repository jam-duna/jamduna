package performance

import (
	"time"

	"github.com/jam-duna/jamduna/pvm/program"
	"github.com/jam-duna/jamduna/pvm/recompiler"
)

// CompileStats contains performance statistics for a single compilation
type CompileStats struct {
	// File information
	FileName string `json:"file_name"`
	FilePath string `json:"file_path"`

	// Timing
	CompileTime time.Duration `json:"compile_time"`

	// PVM statistics
	PVMCodeSize        int `json:"pvm_code_size"`
	PVMInstructionCount int `json:"pvm_instruction_count"`
	PVMBasicBlockCount  int `json:"pvm_basic_block_count"`

	// X86 statistics
	X86CodeSize        int `json:"x86_code_size"`
	X86InstructionCount int `json:"x86_instruction_count"`

	// Ratios
	X86ToPVMCodeRatio        float64 `json:"x86_to_pvm_code_ratio"`
	X86ToPVMInstructionRatio float64 `json:"x86_to_pvm_instruction_ratio"`
}

// AggregateStats contains aggregated statistics across all files
type AggregateStats struct {
	TotalFiles int `json:"total_files"`

	// Timing
	TotalCompileTime   time.Duration `json:"total_compile_time"`
	AverageCompileTime time.Duration `json:"average_compile_time"`
	MinCompileTime     time.Duration `json:"min_compile_time"`
	MaxCompileTime     time.Duration `json:"max_compile_time"`

	// PVM totals
	TotalPVMCodeSize        int `json:"total_pvm_code_size"`
	TotalPVMInstructions    int `json:"total_pvm_instructions"`
	TotalPVMBasicBlocks     int `json:"total_pvm_basic_blocks"`

	// X86 totals
	TotalX86CodeSize        int `json:"total_x86_code_size"`
	TotalX86Instructions    int `json:"total_x86_instructions"`

	// Average ratios
	AverageX86ToPVMCodeRatio        float64 `json:"average_x86_to_pvm_code_ratio"`
	AverageX86ToPVMInstructionRatio float64 `json:"average_x86_to_pvm_instruction_ratio"`
}

// Benchmark measures compilation performance for a PVM program
func Benchmark(code []byte, bitmask []byte, jumpTable []uint32) (*CompileStats, error) {
	stats := &CompileStats{
		PVMCodeSize: len(code),
	}

	// Count PVM instructions and basic blocks using the bitmask
	for i := 0; i < len(bitmask); i++ {
		if bitmask[i]&1 == 1 {
			stats.PVMInstructionCount++
		}
		if bitmask[i]&2 == 2 {
			stats.PVMBasicBlockCount++
		}
	}

	// Create compiler and measure compilation time
	compiler := recompiler.NewX86Compiler(code)
	if err := compiler.SetBitMask(bitmask); err != nil {
		return nil, err
	}
	if err := compiler.SetJumpTable(jumpTable); err != nil {
		return nil, err
	}

	// Measure compile time
	start := time.Now()
	x86Code, _, _, _ := compiler.CompileX86Code(0)
	stats.CompileTime = time.Since(start)

	// X86 statistics
	stats.X86CodeSize = len(x86Code)
	x86Instructions := recompiler.DisassembleInstructions(x86Code)
	stats.X86InstructionCount = len(x86Instructions)

	// Calculate ratios
	if stats.PVMCodeSize > 0 {
		stats.X86ToPVMCodeRatio = float64(stats.X86CodeSize) / float64(stats.PVMCodeSize)
	}
	if stats.PVMInstructionCount > 0 {
		stats.X86ToPVMInstructionRatio = float64(stats.X86InstructionCount) / float64(stats.PVMInstructionCount)
	}

	return stats, nil
}

// BenchmarkFromProgram measures compilation performance from a Program struct
func BenchmarkFromProgram(prog *program.Program) (*CompileStats, error) {
	return Benchmark(prog.Code, prog.K, prog.J)
}

// BenchmarkFromRaw measures compilation performance from raw PVM bytecode
func BenchmarkFromRaw(rawCode []byte) (*CompileStats, error) {
	prog, _, _, _, _, _, _, err := program.DecodeProgram(rawCode)
	if err != nil {
		return nil, err
	}
	return Benchmark(prog.Code, prog.K, prog.J)
}

// CalculateAggregate computes aggregate statistics from a slice of CompileStats
func CalculateAggregate(stats []*CompileStats) *AggregateStats {
	if len(stats) == 0 {
		return &AggregateStats{}
	}

	agg := &AggregateStats{
		TotalFiles:     len(stats),
		MinCompileTime: stats[0].CompileTime,
		MaxCompileTime: stats[0].CompileTime,
	}

	var totalCodeRatio, totalInstrRatio float64

	for _, s := range stats {
		// Timing
		agg.TotalCompileTime += s.CompileTime
		if s.CompileTime < agg.MinCompileTime {
			agg.MinCompileTime = s.CompileTime
		}
		if s.CompileTime > agg.MaxCompileTime {
			agg.MaxCompileTime = s.CompileTime
		}

		// PVM totals
		agg.TotalPVMCodeSize += s.PVMCodeSize
		agg.TotalPVMInstructions += s.PVMInstructionCount
		agg.TotalPVMBasicBlocks += s.PVMBasicBlockCount

		// X86 totals
		agg.TotalX86CodeSize += s.X86CodeSize
		agg.TotalX86Instructions += s.X86InstructionCount

		// Ratios
		totalCodeRatio += s.X86ToPVMCodeRatio
		totalInstrRatio += s.X86ToPVMInstructionRatio
	}

	// Calculate averages
	agg.AverageCompileTime = agg.TotalCompileTime / time.Duration(len(stats))
	agg.AverageX86ToPVMCodeRatio = totalCodeRatio / float64(len(stats))
	agg.AverageX86ToPVMInstructionRatio = totalInstrRatio / float64(len(stats))

	return agg
}
