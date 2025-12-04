package statedb

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/colorfulnotion/jam/pvm/recompiler"
	"github.com/colorfulnotion/jam/types"
)

func TestBundleExecution(t *testing.T) {
	targetDBfile := "test/00000017.json"
	content, err := os.ReadFile(targetDBfile)
	if err != nil {
		t.Fatalf("❌ [%s] Failed to read file: %v", targetDBfile, err)
	}
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
	bundleFile := "test/00000017_0xb742a969e5cf3d3208e67330d8583e9af33abf16bf9dde8c9aaad4b40b1daa72_0_3_guarantor.json"
	bundleContent, err := os.ReadFile(bundleFile)
	if err != nil {
		t.Fatalf("❌ [%s] Failed to read bundle file: %v", bundleFile, err)
	}
	// use json content of the bundle to execute
	var bundle types.WorkPackageBundleSnapshot
	if err := json.Unmarshal(bundleContent, &bundle); err != nil {
		t.Fatalf("❌ [%s] Failed to unmarshal bundle: %v", bundleFile, err)
	}

	testingBackend := BackendCompiler
	if testingBackend == BackendCompiler {
		// Enable debug tracing for compiler backend
		PvmLogging = true
		recompiler.EnableDebugTracing = false
		recompiler.ALWAYS_COMPILE = true
	} else {
		PvmLogging = true
	}

	wr, err := state.ExecuteWorkPackageBundle(bundle.CoreIndex, bundle.Bundle, bundle.SegmentRootLookup, stf.Block.TimeSlot(), "", 0, testingBackend)
	if err != nil {
		t.Fatalf("❌ [%s] Failed to execute bundle: %v", bundleFile, err)
	}
	t.Logf("✅ [%s] Bundle executed successfully: %s", bundleFile, wr.String())
	// see if the work report hash is the same
	diff := CompareJSON(wr, bundle.Report)
	if diff != "" {
		t.Errorf("❌ [%s] Work report does not match expected report:\n%s", bundleFile, diff)
	} else {
		t.Logf("✅ [%s] Work report matches expected report", bundleFile)
	}

}
