package trie

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/colorfulnotion/jam/common"
)

// SnapshotKeyVal represents a key-value pair in the snapshot (matches statedb.KeyVal structure)
type SnapshotKeyVal struct {
	Key   [31]byte `json:"key"`
	Value []byte   `json:"value"`
}

// kvAlias is used for JSON unmarshaling (matches statedb format)
type kvAlias struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// UnmarshalJSON implements custom JSON unmarshaling for SnapshotKeyVal
func (kv *SnapshotKeyVal) UnmarshalJSON(data []byte) error {
	var aux kvAlias
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// strip "0x"
	keyHex := strings.TrimPrefix(aux.Key, "0x")
	valHex := strings.TrimPrefix(aux.Value, "0x")

	// decode key
	rawKey, err := hex.DecodeString(keyHex)
	if err != nil {
		return fmt.Errorf("invalid hex in key %q: %w", aux.Key, err)
	}

	if len(rawKey) != 31 {
		return fmt.Errorf("invalid key length: expected 31 bytes, got %d", len(rawKey))
	}
	copy(kv.Key[:], rawKey[:])

	// decode value
	rawVal, err := hex.DecodeString(valHex)
	if err != nil {
		return fmt.Errorf("invalid hex in value %q: %w", aux.Value, err)
	}
	kv.Value = rawVal

	return nil
}

type Snapshot struct {
	PreState struct {
		StateRoot string           `json:"state_root"`
		KeyVals   []SnapshotKeyVal `json:"keyvals"`
	} `json:"pre_state"`
}

func TestBPTE2E(t *testing.T) {
	//snapshotDir := "/Users/michael/Github/jam-test-vectors/traces/storage"
	p := os.Getenv("JAM_TESTVECTORS_PATH")
	if p == "" {
		t.Log("Warning: $JAM_TESTVECTORS_PATH not set")
		t.Skip()
	}
	snapshotDir := filepath.Join(p, "traces", "storage")

	// Find all JSON files
	pattern := filepath.Join(snapshotDir, "*.json")
	files, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("Failed to glob snapshot files: %v", err)
	}

	if len(files) == 0 {
		t.Fatalf("No snapshot files found in %s", snapshotDir)
	}

	t.Logf("Found %d snapshot files to test", len(files))

	totalSuccesses := 0
	totalFailures := 0
	totalKeys := 0

	for _, file := range files {
		filename := filepath.Base(file)
		t.Logf("\n=== Testing %s ===", filename)

		// Load snapshot
		data, err := ioutil.ReadFile(file)
		if err != nil {
			t.Errorf("%s: Failed to read: %v", filename, err)
			continue
		}

		var snapshot Snapshot
		if err := json.Unmarshal(data, &snapshot); err != nil {
			t.Errorf("%s: Failed to parse: %v", filename, err)
			continue
		}

		expectedRoot := common.HexToHash(snapshot.PreState.StateRoot)
		numKeys := len(snapshot.PreState.KeyVals)
		totalKeys += numKeys

		// Initialize temporary levelDB
		dbPath := "/tmp/bpt_trace_verify_test_" + filename
		DeleteLevelDB(dbPath)
		db, err := InitLevelDB(dbPath)
		if err != nil {
			t.Errorf("%s: Failed to init levelDB: %v", filename, err)
			continue
		}

		// Phase 1: Build tree and verify state root
		tree := NewMerkleTree(nil, db)
		for _, kv := range snapshot.PreState.KeyVals {
			key := kv.Key[:]
			value := kv.Value
			tree.Insert(key, value)
		}

		actualRoot := tree.GetRoot()
		if actualRoot != expectedRoot {
			t.Errorf("%s: State root mismatch!\n  Got:      %s\n  Expected: %s", filename, actualRoot, expectedRoot)
			db.Close()
			DeleteLevelDB(dbPath)
			totalFailures += numKeys
			continue
		} else {
			t.Logf("%s: ✓ StateRoot: %s", filename, actualRoot)
		}

		// Phase 2: Test trace & verify on all keys
		successCount := 0
		failCount := 0

		for i, kv := range snapshot.PreState.KeyVals {
			// Pad 31-byte key to 32 bytes
			key32 := make([]byte, 32)
			copy(key32[:], kv.Key[:])
			value := kv.Value

			// Generate proof
			proof, err := tree.Trace(key32)
			if err != nil {
				t.Errorf("%s Key#%d (%x): Trace failed: %v", filename, i, key32, err)
				failCount++
				continue
			}

			// Convert [][]byte to []common.Hash
			proofHashes := make([]common.Hash, len(proof))
			for j, p := range proof {
				copy(proofHashes[j][:], p)
			}

			// Verify proof
			if !VerifyRaw(key32, value, actualRoot.Bytes(), proofHashes) {
				t.Errorf("%s Key#%d (%x): Verification FAILED", filename, i, key32)
				failCount++
			} else {
				//t.Logf("%s Key#%d (%x): Verified", filename, i, key32)
				successCount++
			}
		}

		// Cleanup
		db.Close()
		DeleteLevelDB(dbPath)

		if failCount > 0 {
			t.Errorf("%s: %d/%d verifications FAILED", filename, failCount, numKeys)
			totalFailures += failCount
		} else {
			t.Logf("%s: ✓ All %d verifications passed", filename, successCount)
		}

		totalSuccesses += successCount
	}

	t.Logf("\n=== FINAL SUMMARY ===")
	t.Logf("Total files tested: %d", len(files))
	t.Logf("Total keys tested: %d", totalKeys)
	t.Logf("Total successful verifications: %d", totalSuccesses)
	t.Logf("Total failed verifications: %d", totalFailures)

	if totalFailures > 0 {
		t.Fatalf("❌ %d total verifications failed across all snapshots", totalFailures)
	}

	t.Log("✓ All verifications passed across all snapshots!")
}
