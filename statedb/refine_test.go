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

	testingBackend := BackendInterpreter
	if testingBackend == BackendCompiler {
		// Enable debug tracing for compiler backend
		PvmLogging = true
		recompiler.EnableDebugTracing = false
		recompiler.ALWAYS_COMPILE = true
	} else {
	}
	t.Logf("🔍 [%s] Executing bundle with %s backend, package %s", bundleFile, testingBackend, bundle.Bundle.String())
	wr, err := state.ExecuteWorkPackageBundle(bundle.CoreIndex, bundle.Bundle, bundle.SegmentRootLookup, stf.Block.TimeSlot(), "", 0, testingBackend, fmt.Sprintf("%s", bundle.Bundle.WorkPackage.Hash()))
	if err != nil {
		t.Fatalf("❌ [%s] Failed to execute bundle: %v", bundleFile, err)
	}
	//t.Logf("✅ [%s] Bundle executed successfully: %s", bundleFile, wr.String())
	// see if the work report hash is the same
	diff := CompareJSON(wr, bundle.Report)
	if diff != "" {
		t.Errorf("❌ [%s] Work report does not match expected report:\n", bundleFile)
		resultAns := wr.Results[0].Result.Ok
		resultReal := bundle.Report.Results[0].Result.Ok
		fmt.Println(formatByteDiff(resultAns, resultReal))
	} else {
		t.Logf("✅ [%s] Work report matches expected report", bundleFile)
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
