package interpreter

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ResetMemTrackers resets the global memory tracking variables for each step.
// This should be called at the start of each PVM step.
func ResetMemTrackers() {
	lastMemAddrRead = 0
	lastMemValueRead = 0
	lastMemAddrWrite = 0
	lastMemValueWrite = 0
}

// PvmVerifyMode enables verification mode where each execution step is compared
// against pre-recorded trace data from trace files (opcode.gz, pc.gz, r0-r12.gz, gas.gz, loads.gz, stores.gz)
var PvmVerifyMode = false

// TraceReader reads pre-recorded trace data for verification
type TraceReader struct {
	logDir string

	// Readers for each trace stream
	opcodeReader *gzip.Reader
	pcReader     *gzip.Reader
	gasReader    *gzip.Reader
	regReaders   [13]*gzip.Reader
	loadsReader  *gzip.Reader
	storesReader *gzip.Reader

	// Files (for cleanup)
	files []*os.File

	// Buffers for reading
	opcodeBuf []byte
	pcBuf     []byte
	gasBuf    []byte
	regBufs   [13][]byte
	loadsBuf  []byte
	storesBuf []byte

	// Current step counter
	stepCounter uint64

	// Whether we've reached EOF
	eof bool
}

// TraceStep represents a single step from the trace for verification
type TraceStep struct {
	StepNumber uint64
	Opcode     uint8
	PC         uint64
	Gas        uint64
	Registers  [13]uint64
	LoadAddr   uint32
	LoadValue  uint64
	StoreAddr  uint32
	StoreValue uint64
}

// NewTraceReader creates a new TraceReader from a trace directory
func NewTraceReader(logDir string) (*TraceReader, error) {
	tr := &TraceReader{
		logDir:    logDir,
		files:     make([]*os.File, 0, 18),
		opcodeBuf: make([]byte, 1),
		pcBuf:     make([]byte, 8),
		gasBuf:    make([]byte, 8),
		loadsBuf:  make([]byte, 12), // addr(4) + value(8)
		storesBuf: make([]byte, 12),
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

	// Open loads.gz
	tr.loadsReader, err = tr.openGzipFile("loads.gz")
	if err != nil {
		tr.Close()
		return nil, fmt.Errorf("failed to open loads.gz: %w", err)
	}

	// Open stores.gz
	tr.storesReader, err = tr.openGzipFile("stores.gz")
	if err != nil {
		tr.Close()
		return nil, fmt.Errorf("failed to open stores.gz: %w", err)
	}

	return tr, nil
}

// openGzipFile opens a gzip file and returns the reader
func (tr *TraceReader) openGzipFile(filename string) (*gzip.Reader, error) {
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
func (tr *TraceReader) ReadStep() (*TraceStep, error) {
	if tr.eof {
		return nil, io.EOF
	}

	step := &TraceStep{
		StepNumber: tr.stepCounter,
	}

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

	// Read loads (addr:4 + value:8 = 12 bytes)
	_, err = io.ReadFull(tr.loadsReader, tr.loadsBuf)
	if err != nil {
		return nil, fmt.Errorf("failed to read loads at step %d: %w", tr.stepCounter, err)
	}
	step.LoadAddr = binary.LittleEndian.Uint32(tr.loadsBuf[:4])
	step.LoadValue = binary.LittleEndian.Uint64(tr.loadsBuf[4:])

	// Read stores (addr:4 + value:8 = 12 bytes)
	_, err = io.ReadFull(tr.storesReader, tr.storesBuf)
	if err != nil {
		return nil, fmt.Errorf("failed to read stores at step %d: %w", tr.stepCounter, err)
	}
	step.StoreAddr = binary.LittleEndian.Uint32(tr.storesBuf[:4])
	step.StoreValue = binary.LittleEndian.Uint64(tr.storesBuf[4:])

	tr.stepCounter++
	return step, nil
}

// Close closes all open files
func (tr *TraceReader) Close() {
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
	if tr.loadsReader != nil {
		tr.loadsReader.Close()
	}
	if tr.storesReader != nil {
		tr.storesReader.Close()
	}
	for _, f := range tr.files {
		if f != nil {
			f.Close()
		}
	}
}

// GetStepCount returns the current step count
func (tr *TraceReader) GetStepCount() uint64 {
	return tr.stepCounter
}

// TraceVerifier wraps a TraceReader and provides verification functionality
type TraceVerifier struct {
	reader *TraceReader

	// Mismatch tracking
	FirstMismatchStep uint64
	FirstMismatch     *TraceMismatch
	MismatchCount     int

	// Options
	StopOnFirstMismatch bool
	Verbose             bool
}

// TraceMismatch describes a mismatch between expected and actual state
type TraceMismatch struct {
	Step     uint64
	Field    string // "opcode", "pc", "gas", "r0"-"r12", "load_addr", "load_value", "store_addr", "store_value"
	Expected interface{}
	Actual   interface{}
}

func (m *TraceMismatch) String() string {
	return fmt.Sprintf("Step %d: %s mismatch - expected %v, got %v",
		m.Step, m.Field, m.Expected, m.Actual)
}

// NewTraceVerifier creates a new TraceVerifier from a trace directory
func NewTraceVerifier(logDir string) (*TraceVerifier, error) {
	reader, err := NewTraceReader(logDir)
	if err != nil {
		return nil, err
	}
	return &TraceVerifier{
		reader:              reader,
		StopOnFirstMismatch: true,
		Verbose:             false,
	}, nil
}

// VerifyStep verifies a single step against the expected trace data
// Returns nil if verification passes, or a TraceMismatch if there's a mismatch
// prevPC is the PC *before* executing the step (matches trace format)
// registers should be the state *after* executing the step
func (tv *TraceVerifier) VerifyStep(
	opcode uint8,
	prevPC uint64,
	gas uint64,
	registers [13]uint64,
	loadAddr uint32,
	loadValue uint64,
	storeAddr uint32,
	storeValue uint64,
) *TraceMismatch {
	expected, err := tv.reader.ReadStep()
	if err != nil {
		if err == io.EOF {
			return nil // End of trace, nothing to verify
		}
		return &TraceMismatch{
			Step:     tv.reader.stepCounter,
			Field:    "read_error",
			Expected: "trace data",
			Actual:   err.Error(),
		}
	}

	// Compare opcode
	if expected.Opcode != opcode {
		mismatch := &TraceMismatch{
			Step:     expected.StepNumber,
			Field:    "opcode",
			Expected: expected.Opcode,
			Actual:   opcode,
		}
		tv.recordMismatch(mismatch)
		if tv.StopOnFirstMismatch {
			return mismatch
		}
	}

	// Compare PC (prevPC in trace format)
	if expected.PC != prevPC {
		mismatch := &TraceMismatch{
			Step:     expected.StepNumber,
			Field:    "pc",
			Expected: expected.PC,
			Actual:   prevPC,
		}
		tv.recordMismatch(mismatch)
		if tv.StopOnFirstMismatch {
			return mismatch
		}
	}

	// Compare Gas
	if expected.Gas != gas {
		mismatch := &TraceMismatch{
			Step:     expected.StepNumber,
			Field:    "gas",
			Expected: expected.Gas,
			Actual:   gas,
		}
		tv.recordMismatch(mismatch)
		if tv.StopOnFirstMismatch {
			return mismatch
		}
	}

	// Compare registers
	for i := 0; i < 13; i++ {
		if expected.Registers[i] != registers[i] {
			mismatch := &TraceMismatch{
				Step:     expected.StepNumber,
				Field:    fmt.Sprintf("r%d", i),
				Expected: expected.Registers[i],
				Actual:   registers[i],
			}
			tv.recordMismatch(mismatch)
			if tv.StopOnFirstMismatch {
				return mismatch
			}
		}
	}

	// Compare loads
	if expected.LoadAddr != loadAddr {
		mismatch := &TraceMismatch{
			Step:     expected.StepNumber,
			Field:    "load_addr",
			Expected: expected.LoadAddr,
			Actual:   loadAddr,
		}
		tv.recordMismatch(mismatch)
		if tv.StopOnFirstMismatch {
			return mismatch
		}
	}
	if expected.LoadValue != loadValue {
		mismatch := &TraceMismatch{
			Step:     expected.StepNumber,
			Field:    "load_value",
			Expected: expected.LoadValue,
			Actual:   loadValue,
		}
		tv.recordMismatch(mismatch)
		if tv.StopOnFirstMismatch {
			return mismatch
		}
	}

	// Compare stores
	if expected.StoreAddr != storeAddr {
		mismatch := &TraceMismatch{
			Step:     expected.StepNumber,
			Field:    "store_addr",
			Expected: expected.StoreAddr,
			Actual:   storeAddr,
		}
		tv.recordMismatch(mismatch)
		if tv.StopOnFirstMismatch {
			return mismatch
		}
	}
	if expected.StoreValue != storeValue {
		mismatch := &TraceMismatch{
			Step:     expected.StepNumber,
			Field:    "store_value",
			Expected: expected.StoreValue,
			Actual:   storeValue,
		}
		tv.recordMismatch(mismatch)
		if tv.StopOnFirstMismatch {
			return mismatch
		}
	}

	if tv.Verbose && expected.StepNumber%1000000 == 0 {
		fmt.Printf("[PvmVerify] Step %d verified OK (PC=%d, Gas=%d)\n",
			expected.StepNumber, prevPC, gas)
	}

	return nil
}

func (tv *TraceVerifier) recordMismatch(m *TraceMismatch) {
	if tv.FirstMismatch == nil {
		tv.FirstMismatchStep = m.Step
		tv.FirstMismatch = m
	}
	tv.MismatchCount++
}

// GetStepCount returns the current step count
func (tv *TraceVerifier) GetStepCount() uint64 {
	return tv.reader.stepCounter
}

// Close closes the underlying reader
func (tv *TraceVerifier) Close() {
	if tv.reader != nil {
		tv.reader.Close()
	}
}

// Summary returns a summary of the verification
func (tv *TraceVerifier) Summary() string {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("Verification completed: %d steps\n", tv.reader.stepCounter))
	if tv.MismatchCount == 0 {
		buf.WriteString("✅ All steps verified successfully!\n")
	} else {
		buf.WriteString(fmt.Sprintf("❌ Found %d mismatch(es)\n", tv.MismatchCount))
		if tv.FirstMismatch != nil {
			buf.WriteString(fmt.Sprintf("First mismatch: %s\n", tv.FirstMismatch.String()))
		}
	}
	return buf.String()
}

// SkipSteps skips the given number of steps in the trace
func (tv *TraceVerifier) SkipSteps(count uint64) error {
	for i := uint64(0); i < count; i++ {
		_, err := tv.reader.ReadStep()
		if err != nil {
			return err
		}
	}
	return nil
}
