package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jam-duna/jamduna/common"
	"github.com/jam-duna/jamduna/types"
)

func TestBMTFromSTF(t *testing.T) {
	fuzzPath := os.Getenv("JAM_CONFORMANCE_PATH")
	if fuzzPath == "" {
		t.Fatalf("JAM_CONFORMANCE_PATH environment variable is not set")
	}
	targetDir := ""

	jamConformancePath, err := filepath.Abs(filepath.Join(fuzzPath, targetDir))
	if err != nil {
		t.Fatalf("failed to get jam conformance path: %v", err)
	}

	//testFile := "fuzz-reports/0.7.1/traces/1761552708/00000048"
	testFile := "fuzz-reports/0.7.1/traces/1761653246/00006616"

	// Test both JSON and binary versions
	t.Run("JSON", func(t *testing.T) {
		testStateTransitionFile(t, filepath.Join(jamConformancePath, testFile+".json"))
	})

	t.Run("Binary", func(t *testing.T) {
		testStateTransitionFile(t, filepath.Join(jamConformancePath, testFile+".bin"))
	})
}

type StateSnapshotRaw struct {
	StateRoot common.Hash    `json:"state_root"`
	KeyVals   []types.KeyVal `json:"keyvals"`
}

type StateTransition struct {
	PreState  StateSnapshotRaw `json:"pre_state"`
	Block     types.Block      `json:"block"`
	PostState StateSnapshotRaw `json:"post_state"`
}

func testStateTransitionFile(t *testing.T, filePath string) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	var stateTransition StateTransition

	if strings.HasSuffix(filePath, ".bin") {
		decoded, _, err := types.Decode(content, reflect.TypeOf(StateTransition{}))
		if err != nil {
			t.Fatalf("Failed to decode binary: %v", err)
		}
		stateTransition = decoded.(StateTransition)
	} else {
		if err := json.Unmarshal(content, &stateTransition); err != nil {
			t.Fatalf("Failed to parse JSON: %v", err)
		}
	}

	t.Logf("Loaded %d key-values", len(stateTransition.PreState.KeyVals))
	t.Logf("Expected state root: %s", stateTransition.PreState.StateRoot.Hex())

	tree := setupStorage()
	defer tree.Close()

	for i, kv := range stateTransition.PreState.KeyVals {
		if i < 5 {
			valuePreview := kv.Value
			if len(kv.Value) > 16 {
				valuePreview = kv.Value[:16]
			}
			t.Logf("Insert %d: key=%x (%d bytes), value len=%d, first bytes=%x", i, kv.Key[:], len(kv.Key[:]), len(kv.Value), valuePreview)
		}
		tree.Insert(kv.Key[:], kv.Value)
	}

	t.Logf("Inserted %d entries total", len(stateTransition.PreState.KeyVals))

	actualRoot, err := tree.Flush()
	if err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	t.Logf("After Flush, root = %s", actualRoot.Hex())

	t.Logf("Actual root:   %s", actualRoot.Hex())
	t.Logf("Expected root: %s", stateTransition.PreState.StateRoot.Hex())

	if actualRoot != stateTransition.PreState.StateRoot {
		t.Errorf("Root mismatch!\nActual:   %s\nExpected: %s",
			actualRoot.Hex(), stateTransition.PreState.StateRoot.Hex())
	}
}
