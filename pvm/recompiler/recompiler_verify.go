package recompiler

import (
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

// EnableVerifyMode enables verification mode where each execution step is compared
// against pre-recorded trace data. Unlike interpreter verify mode, this only verifies
// opcode, pc, gas, and pre-execution registers (not memory operations).
var EnableVerifyMode = false

// VerifyBaseDir is the base directory containing trace subdirectories (e.g., "0x.../auth", "0x.../0_39711455")
var VerifyBaseDir = ""

// RecompilerTraceReader reads pre-recorded trace data for recompiler verification
// It reads the same format as interpreter traces but only uses opcode, pc, gas, and registers
type RecompilerTraceReader struct {
	logDir string

	// Readers for each trace stream
	opcodeReader *gzip.Reader
	pcReader     *gzip.Reader
	gasReader    *gzip.Reader
	regReaders   [13]*gzip.Reader

	// Files (for cleanup)
	files []*os.File

	// Buffers for reading
	opcodeBuf []byte
	pcBuf     []byte
	gasBuf    []byte
	regBufs   [13][]byte

	// Reusable step buffer to avoid allocations
	stepBuffer RecompilerTraceStep

	// Current step counter
	stepCounter uint64

	// Whether we've reached EOF
	eof bool
}

// RecompilerTraceStep represents a single step from the trace for verification
// Note: This uses POST-execution register values from the trace
// This struct is designed to be reused to avoid allocations
type RecompilerTraceStep struct {
	StepNumber uint64
	Opcode     uint8
	PC         uint64
	Gas        uint64
	Registers  [13]uint64
}

// Reset clears the step for reuse
func (s *RecompilerTraceStep) Reset() {
	s.StepNumber = 0
	s.Opcode = 0
	s.PC = 0
	s.Gas = 0
	for i := range s.Registers {
		s.Registers[i] = 0
	}
}

// CopyFrom copies data from another step
func (s *RecompilerTraceStep) CopyFrom(other *RecompilerTraceStep) {
	s.StepNumber = other.StepNumber
	s.Opcode = other.Opcode
	s.PC = other.PC
	s.Gas = other.Gas
	s.Registers = other.Registers
}

// NewRecompilerTraceReader creates a new RecompilerTraceReader from a trace directory
func NewRecompilerTraceReader(logDir string) (*RecompilerTraceReader, error) {
	tr := &RecompilerTraceReader{
		logDir:    logDir,
		files:     make([]*os.File, 0, 16),
		opcodeBuf: make([]byte, 1),
		pcBuf:     make([]byte, 8),
		gasBuf:    make([]byte, 8),
	}
	for i := 0; i < 13; i++ {
		tr.regBufs[i] = make([]byte, 8)
	}

	var err error

	// Open opcode.gz
	tr.opcodeReader, err = tr.openGzipFile("opcode.gz")
	if err != nil {
		tr.Close()
		return nil, fmt.Errorf("failed to open opcode.gz: %w", err)
	}

	// Open pc.gz
	tr.pcReader, err = tr.openGzipFile("pc.gz")
	if err != nil {
		tr.Close()
		return nil, fmt.Errorf("failed to open pc.gz: %w", err)
	}

	// Open gas.gz
	tr.gasReader, err = tr.openGzipFile("gas.gz")
	if err != nil {
		tr.Close()
		return nil, fmt.Errorf("failed to open gas.gz: %w", err)
	}

	// Open r0.gz - r12.gz
	for i := 0; i < 13; i++ {
		tr.regReaders[i], err = tr.openGzipFile(fmt.Sprintf("r%d.gz", i))
		if err != nil {
			tr.Close()
			return nil, fmt.Errorf("failed to open r%d.gz: %w", i, err)
		}
	}

	return tr, nil
}

// openGzipFile opens a gzip file and returns the reader
func (tr *RecompilerTraceReader) openGzipFile(filename string) (*gzip.Reader, error) {
	path := filepath.Join(tr.logDir, filename)
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	tr.files = append(tr.files, f)

	gr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	return gr, nil
}

// ReadStep reads the next step from the trace files
// IMPORTANT: The returned pointer points to an internal buffer that will be
// overwritten on the next call. Callers must copy the data if they need to retain it.
func (tr *RecompilerTraceReader) ReadStep() (*RecompilerTraceStep, error) {
	if tr.eof {
		return nil, io.EOF
	}

	// Reuse the internal buffer instead of allocating
	step := &tr.stepBuffer
	step.StepNumber = tr.stepCounter

	// Read opcode (1 byte)
	n, err := io.ReadFull(tr.opcodeReader, tr.opcodeBuf)
	if err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			tr.eof = true
			return nil, io.EOF
		}
		return nil, fmt.Errorf("failed to read opcode at step %d: %w", tr.stepCounter, err)
	}
	if n != 1 {
		tr.eof = true
		return nil, io.EOF
	}
	step.Opcode = tr.opcodeBuf[0]

	// Read PC (8 bytes, little-endian)
	_, err = io.ReadFull(tr.pcReader, tr.pcBuf)
	if err != nil {
		return nil, fmt.Errorf("failed to read pc at step %d: %w", tr.stepCounter, err)
	}
	step.PC = binary.LittleEndian.Uint64(tr.pcBuf)

	// Read Gas (8 bytes, little-endian)
	_, err = io.ReadFull(tr.gasReader, tr.gasBuf)
	if err != nil {
		return nil, fmt.Errorf("failed to read gas at step %d: %w", tr.stepCounter, err)
	}
	step.Gas = binary.LittleEndian.Uint64(tr.gasBuf)

	// Read registers r0-r12 (8 bytes each, little-endian)
	for i := 0; i < 13; i++ {
		_, err = io.ReadFull(tr.regReaders[i], tr.regBufs[i])
		if err != nil {
			return nil, fmt.Errorf("failed to read r%d at step %d: %w", i, tr.stepCounter, err)
		}
		step.Registers[i] = binary.LittleEndian.Uint64(tr.regBufs[i])
	}

	tr.stepCounter++
	return step, nil
}

// Close closes all open files
func (tr *RecompilerTraceReader) Close() {
	if tr.opcodeReader != nil {
		tr.opcodeReader.Close()
	}
	if tr.pcReader != nil {
		tr.pcReader.Close()
	}
	if tr.gasReader != nil {
		tr.gasReader.Close()
	}
	for i := 0; i < 13; i++ {
		if tr.regReaders[i] != nil {
			tr.regReaders[i].Close()
		}
	}
	for _, f := range tr.files {
		if f != nil {
			f.Close()
		}
	}
}

// GetStepCount returns the current step count
func (tr *RecompilerTraceReader) GetStepCount() uint64 {
	return tr.stepCounter
}

// RecompilerTraceVerifier wraps a RecompilerTraceReader and provides verification functionality
// Memory-optimized: uses pre-allocated buffers instead of dynamic allocations
type RecompilerTraceVerifier struct {
	reader *RecompilerTraceReader

	// Mismatch tracking
	FirstMismatchStep uint64
	FirstMismatch     *RecompilerTraceMismatch
	MismatchCount     int

	// Step counting
	VerifiedStepCount uint64 // Number of steps actually verified (excluding skipped ECALLI/SBRK)
	SkippedStepCount  uint64 // Number of ECALLI/SBRK steps skipped

	// Options
	StopOnFirstMismatch bool
	Verbose             bool

	// Buffered step for pre-register comparison (pre-allocated, not pointer)
	// Since trace has post-execution registers but recompiler has pre-execution,
	// we need to compare current step's pre-regs with previous step's post-regs
	prevStep    RecompilerTraceStep
	hasPrevStep bool // tracks if prevStep has valid data

	// Recent steps history for debugging (last 20 ground truth steps)
	// Pre-allocated array of structs instead of pointers
	recentSteps    [20]RecompilerTraceStep
	recentStepsIdx int // circular buffer index

	// Recent skipped ECALLI/SBRK steps for debugging (last 10)
	// Pre-allocated array of structs instead of pointers
	recentSkippedSteps    [10]RecompilerTraceStep
	recentSkippedStepsIdx int // circular buffer index

	// Track the last step that modified each register (for debugging)
	// Simplified: only store step number and opcode, not full step info
	lastRegModifyStep [13]uint64
	lastRegModifyOp   [13]uint8
	lastRegModifyPC   [13]uint64 // PC when register was last modified
	prevRegisters     [13]uint64 // Previous registers to detect changes
}

// RecompilerTraceMismatch describes a mismatch between expected and actual state
type RecompilerTraceMismatch struct {
	Step     uint64
	Field    string // "opcode", "pc", "gas", "r0"-"r12"
	Expected interface{}
	Actual   interface{}
}

func (m *RecompilerTraceMismatch) String() string {
	return fmt.Sprintf("Step %d: %s mismatch - expected %v, got %v",
		m.Step, m.Field, m.Expected, m.Actual)
}

// NewRecompilerTraceVerifier creates a new RecompilerTraceVerifier from a trace directory
func NewRecompilerTraceVerifier(logDir string) (*RecompilerTraceVerifier, error) {
	reader, err := NewRecompilerTraceReader(logDir)
	if err != nil {
		return nil, err
	}
	return &RecompilerTraceVerifier{
		reader:              reader,
		StopOnFirstMismatch: true,
		Verbose:             false,
	}, nil
}

// VerifyStepPreRegisters verifies a single step using PRE-execution register values
// This is the recompiler's verification method since it captures registers BEFORE execution.
//
// The trace format stores POST-execution state, so we need to handle this differently:
// - For step N, the recompiler has PRE-execution registers (before executing instruction N)
// - The trace has POST-execution registers (after executing instruction N)
// - So we compare recompiler's step N pre-regs with trace's step N-1 post-regs
//
// For opcode and PC, we compare directly with the current step in the trace.
// ECALLI and SBRK are skipped because they have special handling in the recompiler.
func (tv *RecompilerTraceVerifier) VerifyStepPreRegisters(
	opcode uint8,
	pc uint64,
	preRegisters [13]uint64,
) *RecompilerTraceMismatch {

	// Read steps from trace, skipping ECALLI and SBRK which are not verified by recompiler
	var currentStep *RecompilerTraceStep
	var err error
	for {
		currentStep, err = tv.reader.ReadStep()
		if err != nil {
			if err == io.EOF {
				return nil // End of trace, nothing to verify
			}
			return &RecompilerTraceMismatch{
				Step:     tv.reader.stepCounter,
				Field:    "read_error",
				Expected: "trace data",
				Actual:   err.Error(),
			}
		}

		// Skip ECALLI (10) in trace - recompiler doesn't verify these
		// Note: SBRK is now verified via HandleSbrk, so we don't skip it here
		if currentStep.Opcode == ECALLI {
			// Store as previous step to keep register tracking in sync (copy value)
			tv.prevStep.CopyFrom(currentStep)
			tv.hasPrevStep = true
			// Track in recent history using circular buffer
			tv.recentSteps[tv.recentStepsIdx].CopyFrom(currentStep)
			tv.recentStepsIdx = (tv.recentStepsIdx + 1) % 20
			// Track in recent skipped steps history
			tv.recentSkippedSteps[tv.recentSkippedStepsIdx].CopyFrom(currentStep)
			tv.recentSkippedStepsIdx = (tv.recentSkippedStepsIdx + 1) % 10
			// Track register modifications (simplified - no full step copy)
			for i := 0; i < 13; i++ {
				if currentStep.Registers[i] != tv.prevRegisters[i] {
					tv.lastRegModifyStep[i] = currentStep.StepNumber
					tv.lastRegModifyOp[i] = currentStep.Opcode
					tv.lastRegModifyPC[i] = currentStep.PC
				}
				tv.prevRegisters[i] = currentStep.Registers[i]
			}
			tv.SkippedStepCount++
			continue
		}
		break
	}
	tv.VerifiedStepCount++
	if tv.VerifiedStepCount%1000000 == 0 {
		runtime.GC()
	}

	// Helper to print mismatch with recent history and panic
	printMismatchAndPanic := func(mismatch *RecompilerTraceMismatch) {
		fmt.Printf("\n❌ [RecompilerVerify] Step %d: %s mismatch - expected %v, got %v\n",
			mismatch.Step, mismatch.Field, mismatch.Expected, mismatch.Actual)
		fmt.Printf("   Opcode: %s (0x%02x), CurrentPC: %d\n", opcode_str(opcode), opcode, currentStep.PC)

		// If this is a register mismatch, show when that register was last modified
		if len(mismatch.Field) >= 2 && mismatch.Field[0] == 'r' {
			var regIdx int
			fmt.Sscanf(mismatch.Field, "r%d", &regIdx)
			if regIdx >= 0 && regIdx < 13 {
				fmt.Printf("   >>> Register %s was last modified at step %d by opcode %s (0x%02x) at PC %d\n",
					mismatch.Field, tv.lastRegModifyStep[regIdx],
					opcode_str(tv.lastRegModifyOp[regIdx]), tv.lastRegModifyOp[regIdx],
					tv.lastRegModifyPC[regIdx])
			}
		}

		fmt.Printf("   Pre-Registers (recompiler): ")
		for i := 0; i < 13; i++ {
			fmt.Printf("r%d=%d ", i, preRegisters[i])
		}
		fmt.Printf("\n")
		fmt.Printf("   Post-Registers (ground truth from prev step): ")
		if tv.hasPrevStep {
			for i := 0; i < 13; i++ {
				fmt.Printf("r%d=%d ", i, tv.prevStep.Registers[i])
			}
		} else {
			fmt.Printf("(no previous step)")
		}
		fmt.Printf("\n")
		fmt.Printf("   Current Step Post-Registers (ground truth): ")
		for i := 0; i < 13; i++ {
			fmt.Printf("r%d=%d ", i, currentStep.Registers[i])
		}
		fmt.Printf("\n")

		// Print recent 20 ground truth steps from circular buffer
		fmt.Printf("\n=== Recent 20 Ground Truth Steps ===\n")
		for i := 0; i < 20; i++ {
			idx := (tv.recentStepsIdx - 1 - i + 20) % 20
			step := &tv.recentSteps[idx]
			if step.StepNumber > 0 { // Check if slot has valid data
				fmt.Printf("  [Step %d] Opcode: %s (0x%02x), PC: %d\n",
					step.StepNumber, opcode_str(step.Opcode), step.Opcode, step.PC)
				fmt.Printf("           Post-Regs: ")
				for j := 0; j < 13; j++ {
					fmt.Printf("r%d=%d ", j, step.Registers[j])
				}
				fmt.Printf("\n")
			}
		}
		fmt.Printf("=== End Recent Steps ===\n\n")

		panic(fmt.Sprintf("Recompiler verification failed at step %d: %s", mismatch.Step, mismatch.String()))
	}

	// Compare opcode (should match current step)
	if currentStep.Opcode != opcode {
		mismatch := &RecompilerTraceMismatch{
			Step:     currentStep.StepNumber,
			Field:    "opcode",
			Expected: currentStep.Opcode,
			Actual:   opcode,
		}
		tv.recordMismatch(mismatch)
		if tv.StopOnFirstMismatch {
			printMismatchAndPanic(mismatch)
			return mismatch
		}
	}

	// Compare PC (should match current step - this is the PC BEFORE execution)
	if currentStep.PC != pc {
		mismatch := &RecompilerTraceMismatch{
			Step:     currentStep.StepNumber,
			Field:    "pc",
			Expected: currentStep.PC,
			Actual:   pc,
		}
		tv.recordMismatch(mismatch)
		if tv.StopOnFirstMismatch {
			printMismatchAndPanic(mismatch)
			return mismatch
		}
	}

	// Compare pre-execution registers with PREVIOUS step's post-execution registers
	// For step 0, there's no previous step, so we skip register comparison
	if tv.hasPrevStep {
		// Collect ALL mismatched registers first
		var mismatches []*RecompilerTraceMismatch
		for i := 0; i < 13; i++ {
			if tv.prevStep.Registers[i] != preRegisters[i] {
				mismatch := &RecompilerTraceMismatch{
					Step:     currentStep.StepNumber,
					Field:    fmt.Sprintf("r%d", i),
					Expected: tv.prevStep.Registers[i],
					Actual:   preRegisters[i],
				}
				mismatches = append(mismatches, mismatch)
				tv.recordMismatch(mismatch)
			}
		}

		// If there are any mismatches, print all of them and panic
		if len(mismatches) > 0 && tv.StopOnFirstMismatch {
			fmt.Printf("\n❌ [RecompilerVerify] Step %d: %d register mismatch(es) detected\n",
				currentStep.StepNumber, len(mismatches))
			fmt.Printf("   Opcode: %s (0x%02x), CurrentPC: %d\n", opcode_str(opcode), opcode, currentStep.PC)

			// Print all mismatched registers with their last modification info
			fmt.Printf("\n=== ALL Mismatched Registers ===\n")
			for _, m := range mismatches {
				var regIdx int
				fmt.Sscanf(m.Field, "r%d", &regIdx)
				fmt.Printf("   %s: expected %d (0x%x), got %d (0x%x)\n",
					m.Field, m.Expected, m.Expected, m.Actual, m.Actual)
				fmt.Printf("       Last modified at step %d by opcode %s (0x%02x) at PC %d\n",
					tv.lastRegModifyStep[regIdx],
					opcode_str(tv.lastRegModifyOp[regIdx]), tv.lastRegModifyOp[regIdx],
					tv.lastRegModifyPC[regIdx])
			}
			fmt.Printf("=== End Mismatched Registers ===\n")

			fmt.Printf("\n   Pre-Registers (recompiler): ")
			for i := 0; i < 13; i++ {
				fmt.Printf("r%d=%d ", i, preRegisters[i])
			}
			fmt.Printf("\n")
			fmt.Printf("   Post-Registers (ground truth from prev step): ")
			for i := 0; i < 13; i++ {
				fmt.Printf("r%d=%d ", i, tv.prevStep.Registers[i])
			}
			fmt.Printf("\n")

			// Print recent 20 ground truth steps from circular buffer
			fmt.Printf("\n=== Recent 20 Ground Truth Steps ===\n")
			for i := 0; i < 20; i++ {
				idx := (tv.recentStepsIdx - 1 - i + 20) % 20
				step := &tv.recentSteps[idx]
				if step.StepNumber > 0 {
					fmt.Printf("  [Step %d] Opcode: %s (0x%02x), PC: %d\n",
						step.StepNumber, opcode_str(step.Opcode), step.Opcode, step.PC)
					fmt.Printf("           Post-Regs: ")
					for j := 0; j < 13; j++ {
						fmt.Printf("r%d=%d ", j, step.Registers[j])
					}
					fmt.Printf("\n")
				}
			}
			fmt.Printf("=== End Recent Steps ===\n\n")

			// Print recent skipped ECALLI/SBRK steps from circular buffer
			hasSkipped := false
			for i := 0; i < 10; i++ {
				if tv.recentSkippedSteps[i].StepNumber > 0 {
					hasSkipped = true
					break
				}
			}
			if hasSkipped {
				fmt.Printf("=== Recent Skipped ECALLI/SBRK Steps (potential causes) ===\n")
				for i := 0; i < 10; i++ {
					idx := (tv.recentSkippedStepsIdx - 1 - i + 10) % 10
					step := &tv.recentSkippedSteps[idx]
					if step.StepNumber > 0 {
						fmt.Printf("  [Step %d] Opcode: %s (0x%02x), PC: %d\n",
							step.StepNumber, opcode_str(step.Opcode), step.Opcode, step.PC)
						fmt.Printf("           Post-Regs: ")
						for j := 0; j < 13; j++ {
							fmt.Printf("r%d=%d ", j, step.Registers[j])
						}
						fmt.Printf("\n")
					}
				}
				fmt.Printf("=== End Skipped Steps ===\n\n")
			}

			panic(fmt.Sprintf("Recompiler verification failed at step %d: %d register mismatch(es)",
				currentStep.StepNumber, len(mismatches)))
		}
	}

	// Update recent steps history using circular buffer (copy value, not pointer)
	tv.recentSteps[tv.recentStepsIdx].CopyFrom(currentStep)
	tv.recentStepsIdx = (tv.recentStepsIdx + 1) % 20

	// Track register modifications (simplified - no full step copy)
	for i := 0; i < 13; i++ {
		if currentStep.Registers[i] != tv.prevRegisters[i] {
			tv.lastRegModifyStep[i] = currentStep.StepNumber
			tv.lastRegModifyOp[i] = currentStep.Opcode
			tv.lastRegModifyPC[i] = currentStep.PC
		}
		tv.prevRegisters[i] = currentStep.Registers[i]
	}

	// Store current step as previous for next iteration (copy value)
	tv.prevStep.CopyFrom(currentStep)
	tv.hasPrevStep = true

	if tv.Verbose && currentStep.StepNumber%1000000 == 0 {
		fmt.Printf("[RecompilerVerify] Step %d verified OK (PC=%d)\n",
			currentStep.StepNumber, pc)
	}

	return nil
}

func (tv *RecompilerTraceVerifier) recordMismatch(m *RecompilerTraceMismatch) {
	if tv.FirstMismatch == nil {
		tv.FirstMismatchStep = m.Step
		tv.FirstMismatch = m
	}
	tv.MismatchCount++
}

// GetStepCount returns the current step count
func (tv *RecompilerTraceVerifier) GetStepCount() uint64 {
	return tv.reader.stepCounter
}

// Close closes the underlying reader
func (tv *RecompilerTraceVerifier) Close() {
	if tv.reader != nil {
		tv.reader.Close()
	}
}

// Summary returns a summary of the verification
func (tv *RecompilerTraceVerifier) Summary() string {
	var summary string
	summary = fmt.Sprintf("Recompiler Verification: %d verified, %d skipped (ECALLI/SBRK), %d total trace steps\n",
		tv.VerifiedStepCount, tv.SkippedStepCount, tv.reader.stepCounter)
	if tv.MismatchCount == 0 {
		summary += "✅ All steps verified successfully!\n"
	} else {
		summary += fmt.Sprintf("❌ Found %d mismatch(es)\n", tv.MismatchCount)
		if tv.FirstMismatch != nil {
			summary += fmt.Sprintf("First mismatch: %s\n", tv.FirstMismatch.String())
		}
	}
	return summary
}

// SkipSteps skips the given number of steps in the trace
func (tv *RecompilerTraceVerifier) SkipSteps(count uint64) error {
	for i := uint64(0); i < count; i++ {
		step, err := tv.reader.ReadStep()
		if err != nil {
			return err
		}
		// Keep track of last step for pre-register comparison (copy value)
		tv.prevStep.CopyFrom(step)
		tv.hasPrevStep = true
	}
	return nil
}
