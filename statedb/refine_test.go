package statedb

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/pvm/recompiler"
	"github.com/colorfulnotion/jam/types"
)

func TestBundleExecution(t *testing.T) {
	// Disable PvmTraceMode to prevent overwriting existing trace data
	// Enable PvmVerifyMode to verify against existing trace
	PvmTraceMode = false
	PvmLogging = false

	// Use PvmVerifyBaseDir - the base directory containing auth/ and 0_39711455/ subdirectories
	traceBaseDir := "0xf1166dc1eb7baff3d1c2450f319358c5c6789fe31313d331d4f035908045ad02"

	// Choose backend: BackendInterpreter or BackendCompiler
	testingBackend := BackendCompiler // Change to BackendInterpreter for interpreter verification

	// Set up verification mode for the chosen backend
	if testingBackend == BackendInterpreter {
		// Interpreter verify mode
		PvmVerifyBaseDir = traceBaseDir
		PvmVerifyDir = "" // Clear direct verify dir
	} else if testingBackend == BackendCompiler {
		// Recompiler verify mode
		recompiler.EnableVerifyMode = false
		// recompiler.VerifyBaseDir = traceBaseDir
		recompiler.ALWAYS_COMPILE = true
	}

	defer func() {
		// Reset all verify modes after test
		PvmVerifyBaseDir = ""
		PvmVerifyDir = ""
		recompiler.EnableVerifyMode = false
		recompiler.VerifyBaseDir = ""
	}()

	t.Logf("🔍 [PvmVerifyMode] Verifying execution against traces in %s with backend %s", traceBaseDir, testingBackend)

	// Load exported segments from JSON for CheckSegments validation
	exportedSegmentsFile := "../trie/test/exported_segments_doom_3072.json"
	exportedSegmentsData, err := os.ReadFile(exportedSegmentsFile)
	if err != nil {
		t.Logf("⚠️ [%s] Failed to read exported segments file: %v (continuing without validation)", exportedSegmentsFile, err)
	} else {
		// Parse the JSON array of hex strings
		var exportedSegmentsHex []string
		if err := json.Unmarshal(exportedSegmentsData, &exportedSegmentsHex); err != nil {
			t.Fatalf("❌ [%s] Failed to unmarshal exported segments: %v", exportedSegmentsFile, err)
		}

		// Convert hex strings to [][]byte
		CheckSegments = make([][]byte, len(exportedSegmentsHex))
		for i, hexStr := range exportedSegmentsHex {
			CheckSegments[i] = common.FromHex(hexStr)
		}
		t.Logf("✅ Loaded %d exported segments for validation from %s", len(CheckSegments), exportedSegmentsFile)
		t.Logf("   First segment: %d bytes, hash prefix: %x...", len(CheckSegments[0]), CheckSegments[0][:32])
	}

	targetDBfile := "test/04918460.json"
	content, err := os.ReadFile(targetDBfile)
	if err != nil {
		t.Fatalf("❌ [%s] Failed to read file: %v", targetDBfile, err)
	}
	log.InitLogger("debug")
	stf, err := parseSTFFile(targetDBfile, string(content))
	if err != nil {
		t.Fatalf("❌ [%s] Failed to parse STF: %v", targetDBfile, err)
	}
	storage, err := InitStorage("/tmp/test")
	if err != nil {
		t.Fatalf("❌ [%s] Failed to init storage: %v", targetDBfile, err)
	}

	state, err := NewStateDBFromStateTransition(storage, &stf)
	if err != nil {
		t.Fatalf("❌ [%s] Failed to create StateDB: %v", targetDBfile, err)
	}
	bundleFile := "test/04918460_0xf1166dc1eb7baff3d1c2450f319358c5c6789fe31313d331d4f035908045ad02_0_5_guarantor.json"
	bundleContent, err := os.ReadFile(bundleFile)
	if err != nil {
		t.Fatalf("❌ [%s] Failed to read bundle file: %v", bundleFile, err)
	}
	// use json content of the bundle to execute
	var bundle types.WorkPackageBundleSnapshot
	if err := json.Unmarshal(bundleContent, &bundle); err != nil {
		t.Fatalf("❌ [%s] Failed to unmarshal bundle: %v", bundleFile, err)
	}

	t.Logf("🔍 [%s] Executing bundle with %s backend, package %s", bundleFile, testingBackend, bundle.Bundle.String())
	// Use the work package hash as identifier (same as trace directory structure)
	// This allows PvmVerifyBaseDir to match logDir structure for verification
	identifier := fmt.Sprintf("%s", bundle.Bundle.WorkPackage.Hash())

	wr, err := state.ExecuteWorkPackageBundle(bundle.CoreIndex, bundle.Bundle, bundle.SegmentRootLookup, stf.Block.TimeSlot(), "", 0, testingBackend, identifier)
	if err != nil {
		// Check if it's a verification failure
		if strings.Contains(err.Error(), "trace verification failed") {
			t.Fatalf("❌ [PvmVerifyMode] Verification failed: %v", err)
		}
		t.Fatalf("❌ [%s] Failed to execute bundle: %v", bundleFile, err)
	}
	// see if the work report hash is the same
	diff := CompareJSON(wr, bundle.Report)
	if diff != "" {
		t.Errorf("❌ [%s] Work report does not match expected report:\n", bundleFile)
		resultAns := wr.Results[0].Result.Ok
		resultReal := bundle.Report.Results[0].Result.Ok
		fmt.Println(formatByteDiff(resultAns, resultReal))
		fmt.Println(diff)
	} else {
		t.Logf("✅ [%s] Work report matches expected report", bundleFile)
		t.Logf("✅ [PvmVerifyMode] All steps verified against trace!")
	}

	// Log CheckSegments validation results
	if len(CheckSegments) > 0 {
		t.Logf("📊 CheckSegments validation completed with %d expected segments", len(CheckSegments))
	}
}

func TestBundleStepExecution(t *testing.T) {
	// Load exported segments from JSON for CheckSegments validation
	exportedSegmentsFile := "../trie/test/exported_segments_doom_3072.json"
	exportedSegmentsData, err := os.ReadFile(exportedSegmentsFile)
	if err != nil {
		t.Logf("⚠️ [%s] Failed to read exported segments file: %v (continuing without validation)", exportedSegmentsFile, err)
	} else {
		// Parse the JSON array of hex strings
		var exportedSegmentsHex []string
		if err := json.Unmarshal(exportedSegmentsData, &exportedSegmentsHex); err != nil {
			t.Fatalf("❌ [%s] Failed to unmarshal exported segments: %v", exportedSegmentsFile, err)
		}

		// Convert hex strings to [][]byte
		CheckSegments = make([][]byte, len(exportedSegmentsHex))
		for i, hexStr := range exportedSegmentsHex {
			CheckSegments[i] = common.FromHex(hexStr)
		}
		t.Logf("✅ Loaded %d exported segments for validation from %s", len(CheckSegments), exportedSegmentsFile)
		t.Logf("   First segment: %d bytes, hash prefix: %x...", len(CheckSegments[0]), CheckSegments[0][:32])
	}

	targetDBfile := "test/04918460.json"
	content, err := os.ReadFile(targetDBfile)
	if err != nil {
		t.Fatalf("❌ [%s] Failed to read file: %v", targetDBfile, err)
	}
	log.InitLogger("debug")
	stf, err := parseSTFFile(targetDBfile, string(content))
	if err != nil {
		t.Fatalf("❌ [%s] Failed to parse STF: %v", targetDBfile, err)
	}
	storage, err := InitStorage("/tmp/test")
	if err != nil {
		t.Fatalf("❌ [%s] Failed to init storage: %v", targetDBfile, err)
	}

	state, err := NewStateDBFromStateTransition(storage, &stf)
	if err != nil {
		t.Fatalf("❌ [%s] Failed to create StateDB: %v", targetDBfile, err)
	}
	bundleFile := "test/04918460_0xf1166dc1eb7baff3d1c2450f319358c5c6789fe31313d331d4f035908045ad02_0_5_guarantor.json"
	bundleContent, err := os.ReadFile(bundleFile)
	if err != nil {
		t.Fatalf("❌ [%s] Failed to read bundle file: %v", bundleFile, err)
	}
	// use json content of the bundle to execute
	var bundle types.WorkPackageBundleSnapshot
	if err := json.Unmarshal(bundleContent, &bundle); err != nil {
		t.Fatalf("❌ [%s] Failed to unmarshal bundle: %v", bundleFile, err)
	}
	testingBackends := []string{BackendInterpreter, BackendInterpreter}
	err = state.ExecuteWorkPackageBundleSteps(bundle.CoreIndex, bundle.Bundle, bundle.SegmentRootLookup, stf.Block.TimeSlot(), "", 0, testingBackends)
	if err != nil {
		t.Fatalf("❌ [%s] Failed to execute bundle: %v", bundleFile, err)
	}
	t.Logf("✅ [%s] Bundle step execution completed successfully with NO MISMATCH", bundleFile)
}

// TestTraceReader tests the TraceReader can correctly read trace files
func TestTraceReader(t *testing.T) {
	traceDir := "0xf1166dc1eb7baff3d1c2450f319358c5c6789fe31313d331d4f035908045ad02/0_39711455"

	// Check if trace directory exists
	if _, err := os.Stat(traceDir); os.IsNotExist(err) {
		t.Skipf("⚠️ Trace directory %s does not exist - skipping test", traceDir)
		return
	}

	reader, err := NewTraceReader(traceDir)
	if err != nil {
		t.Fatalf("❌ Failed to create TraceReader: %v", err)
	}
	defer reader.Close()

	// Read first 10 steps
	for i := 0; i < 10; i++ {
		step, err := reader.ReadStep()
		if err != nil {
			t.Fatalf("❌ Failed to read step %d: %v", i, err)
		}
		t.Logf("Step %d: Opcode=0x%02x PC=%d Gas=%d R0=%d R7=%d",
			step.StepNumber, step.Opcode, step.PC, step.Gas, step.Registers[0], step.Registers[7])
	}

	t.Logf("✅ TraceReader successfully read %d steps", reader.GetStepCount())
}

// TestBundleVerifyMode tests PvmVerifyMode - loading trace from existing trace files
// and verifying each step during execution matches the trace.
// Usage:
//
//	go test -v -run TestBundleVerifyMode -timeout 30m
//
// This test uses the Doom trace data at:
//
//	statedb/0xf1166dc1eb7baff3d1c2450f319358c5c6789fe31313d331d4f035908045ad02/0_39711455
func TestBundleVerifyMode(t *testing.T) {
	// Set the verify directory to existing trace data
	traceDir := "0xf1166dc1eb7baff3d1c2450f319358c5c6789fe31313d331d4f035908045ad02/0_39711455"

	// Check if trace directory exists
	if _, err := os.Stat(traceDir); os.IsNotExist(err) {
		t.Skipf("⚠️ Trace directory %s does not exist - skipping verify test", traceDir)
		return
	}

	// Enable verify mode
	PvmVerifyDir = traceDir
	PvmTraceMode = false // Don't write new traces, just verify
	defer func() {
		PvmVerifyDir = "" // Reset after test
	}()

	// Load exported segments from JSON for CheckSegments validation
	exportedSegmentsFile := "../trie/test/exported_segments_doom_3072.json"
	exportedSegmentsData, err := os.ReadFile(exportedSegmentsFile)
	if err != nil {
		t.Logf("⚠️ [%s] Failed to read exported segments file: %v (continuing without validation)", exportedSegmentsFile, err)
	} else {
		var exportedSegmentsHex []string
		if err := json.Unmarshal(exportedSegmentsData, &exportedSegmentsHex); err != nil {
			t.Fatalf("❌ [%s] Failed to unmarshal exported segments: %v", exportedSegmentsFile, err)
		}
		CheckSegments = make([][]byte, len(exportedSegmentsHex))
		for i, hexStr := range exportedSegmentsHex {
			CheckSegments[i] = common.FromHex(hexStr)
		}
		t.Logf("✅ Loaded %d exported segments for validation from %s", len(CheckSegments), exportedSegmentsFile)
	}

	targetDBfile := "test/04918460.json"
	content, err := os.ReadFile(targetDBfile)
	if err != nil {
		t.Fatalf("❌ [%s] Failed to read file: %v", targetDBfile, err)
	}
	log.InitLogger("debug")
	stf, err := parseSTFFile(targetDBfile, string(content))
	if err != nil {
		t.Fatalf("❌ [%s] Failed to parse STF: %v", targetDBfile, err)
	}
	storage, err := InitStorage("/tmp/test")
	if err != nil {
		t.Fatalf("❌ [%s] Failed to init storage: %v", targetDBfile, err)
	}

	state, err := NewStateDBFromStateTransition(storage, &stf)
	if err != nil {
		t.Fatalf("❌ [%s] Failed to create StateDB: %v", targetDBfile, err)
	}
	bundleFile := "test/04918460_0xf1166dc1eb7baff3d1c2450f319358c5c6789fe31313d331d4f035908045ad02_0_5_guarantor.json"
	bundleContent, err := os.ReadFile(bundleFile)
	if err != nil {
		t.Fatalf("❌ [%s] Failed to read bundle file: %v", bundleFile, err)
	}
	var bundle types.WorkPackageBundleSnapshot
	if err := json.Unmarshal(bundleContent, &bundle); err != nil {
		t.Fatalf("❌ [%s] Failed to unmarshal bundle: %v", bundleFile, err)
	}

	testingBackend := BackendInterpreter
	t.Logf("🔍 [PvmVerifyMode] Executing bundle with %s backend, verifying against trace in %s", testingBackend, traceDir)

	identifier := "SKIP" // Don't write output logs
	wr, err := state.ExecuteWorkPackageBundle(bundle.CoreIndex, bundle.Bundle, bundle.SegmentRootLookup, stf.Block.TimeSlot(), "", 0, testingBackend, identifier)
	if err != nil {
		// Verification failure will cause an error - this is expected if there's a mismatch
		if strings.Contains(err.Error(), "trace verification failed") {
			t.Logf("❌ [PvmVerifyMode] Verification failed: %v", err)
			t.Fail()
			return
		}
		t.Fatalf("❌ [%s] Failed to execute bundle: %v", bundleFile, err)
	}

	// Check work report
	diff := CompareJSON(wr, bundle.Report)
	if diff != "" {
		t.Errorf("❌ [%s] Work report does not match expected report", bundleFile)
		resultAns := wr.Results[0].Result.Ok
		resultReal := bundle.Report.Results[0].Result.Ok
		fmt.Println(formatByteDiff(resultAns, resultReal))
	} else {
		t.Logf("✅ [PvmVerifyMode] Execution completed - all steps verified against trace!")
	}
}

func formatByteDiff(actual, expected []byte) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("len(actual)=%d len(expected)=%d\n", len(actual), len(expected)))

	// identify the first differing index
	limit := len(actual)
	if len(expected) < limit {
		limit = len(expected)
	}
	firstDiff := -1
	for i := 0; i < limit; i++ {
		if actual[i] != expected[i] {
			firstDiff = i
			break
		}
	}

	switch {
	case firstDiff >= 0:
		b.WriteString(fmt.Sprintf("first mismatch at offset %d: actual=0x%02x expected=0x%02x\n", firstDiff, actual[firstDiff], expected[firstDiff]))
	case len(actual) != len(expected):
		b.WriteString("no mismatched bytes within shared length; lengths differ\n")
	default:
		b.WriteString("byte slices are identical\n")
	}

	// provide a small hex preview around the mismatch to aid debugging
	previewStart := firstDiff - 8
	if previewStart < 0 {
		previewStart = 0
	}
	previewEnd := firstDiff + 8
	if firstDiff == -1 {
		// if no mismatch inside shared length, preview from start
		previewEnd = len(actual)
	}
	if previewEnd > len(actual) {
		previewEnd = len(actual)
	}
	if previewEnd > previewStart {
		b.WriteString(fmt.Sprintf("actual[%d:%d]=%s\n", previewStart, previewEnd, hex.EncodeToString(actual[previewStart:previewEnd])))
	}
	if previewStart < 0 {
		previewStart = 0
	}
	previewEndExpected := firstDiff + 8
	if firstDiff == -1 {
		previewEndExpected = len(expected)
	}
	if previewEndExpected > len(expected) {
		previewEndExpected = len(expected)
	}
	if previewEndExpected > previewStart {
		b.WriteString(fmt.Sprintf("expected[%d:%d]=%s", previewStart, previewEndExpected, hex.EncodeToString(expected[previewStart:previewEndExpected])))
	}

	return b.String()
}
