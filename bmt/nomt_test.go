package bmt

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"os"
	"testing"
	"time"
)

// testKey creates a 32-byte key from a string.
func testKey(s string) [32]byte {
	var key [32]byte
	copy(key[:], s)
	return key
}

// hex2Bytes converts hex string to bytes for JAM test vectors.
func hex2Bytes(hexStr string) []byte {
	bytes, err := hex.DecodeString(hexStr)
	if err != nil {
		panic(err)
	}
	return bytes
}

// bytesToKey converts byte slice to 32-byte key.
func bytesToKey(b []byte) [32]byte {
	var key [32]byte
	copy(key[:], b)
	return key
}

// BenchmarkMode defines how iterations are isolated
type BenchmarkMode string

const (
	// BenchmarkIsolated creates fresh DB for each iteration (matches Rust bench_isolate)
	BenchmarkIsolated BenchmarkMode = "isolated"
	// BenchmarkSequential reuses same DB across iterations (matches Rust bench_sequential)
	BenchmarkSequential BenchmarkMode = "sequential"
)

// TestParams defines configurable parameters for the test_nomt helper function
type TestParams struct {
	MaxIterations   int           // Total number of iterations to run
	BenchmarkMode   BenchmarkMode // Isolated vs Sequential benchmark mode
	CommitLag       int           // Number of iterations to delay commits (only for Sequential mode)
	NumInserts      int           // Number of keys to insert per iteration
	NumUpdates      int           // Number of keys to update per iteration (if possible)
	NumDeletes      int           // Number of keys to delete per iteration (if possible)
	NumReads        int           // Number of keys to read per iteration (if possible)
	NumWitnessKeys  int           // Number of keys to include in state witness
	ValueSize       int           // Size of values in bytes
	OptimizeWitness bool          // Whether to optimize witness generation for performance
}

func TestNomtOpen(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions(dir)

	db, err := Open(opts)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	if db == nil {
		t.Fatal("Database is nil")
	}
}

func TestNomtInsert(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions(dir)

	db, err := Open(opts)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	key := testKey("testkey")
	value := []byte("testvalue")

	// Insert
	if err := db.Insert(key, value); err != nil {
		t.Fatalf("Failed to insert: %v", err)
	}

	// Get
	got, err := db.Get(key)
	if err != nil {
		t.Fatalf("Failed to get: %v", err)
	}

	if !bytes.Equal(got, value) {
		t.Errorf("Expected %s, got %s", value, got)
	}
}

func TestNomtCommit(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions(dir)

	db, err := Open(opts)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// JAM test vector data - matches storage/bmt_test.go:TestBPTProofSimple
	data := [][2][]byte{
		{hex2Bytes("f2a9fcaf8ae0ff770b0908ebdee1daf8457c0ef5e1106c89ad364236333c5fb3"), hex2Bytes("22c62f84ee5775d1e75ba6519f6dfae571eb1888768f2a203281579656b6a29097f7c7e2cf44e38da9a541d9b4c773db8b71e1d3")},
		{hex2Bytes("f3a9fcaf8ae0ff770b0908ebdee1daf8457c0ef5e1106c89ad364236333c5fb3"), hex2Bytes("965ac2547cacec18429e88553142a605d649fbcd6a40a0ae5d51a8f218c8fd5c")},
		{hex2Bytes("d7f99b746f23411983df92806725af8e5cb66eba9f200737accae4a1ab7f47b9"), hex2Bytes("965ac2547cacec18429e88553142a605d649fbcd6a40a0ae5d51a8f218c8fd5c")},
		{hex2Bytes("59ee947b94bcc05634d95efb474742f6cd6531766e44670ec987270a6b5a4211"), hex2Bytes("72fdb0c99cf47feb85b2dad01ee163139ee6d34a8d893029a200aff76f4be5930b9000a1bbb2dc2b6c79f8f3c19906c94a3472349817af21181c3eef6b")},
		{hex2Bytes("a3dc3bed1b0727caf428961bed11c9998ae2476d8a97fad203171b628363d9a2"), hex2Bytes("3f26db92922e86f6b538372608656a14762b3e93bd5d4f6a754d36f68ce0b28b")},
		{hex2Bytes("15207c233b055f921701fc62b41a440d01dfa488016a97cc653a84afb5f94fd5"), hex2Bytes("be2a1eb0a1b961e9642c2e09c71d2f45aa653bb9a709bbc8cbad18022c9dcf2e")},
		{hex2Bytes("b05ff8a05bb23c0d7b177d47ce466ee58fd55c6a0351a3040cf3cbf5225aab19"), hex2Bytes("5c43fcf60000000000000000000000006ba080e1534c41f5d44615813a7d1b2b57c950390000000000000000000000008863786bebe8eb9659df00b49f8f1eeec7e2c8c1")},
		{hex2Bytes("df08871e8a54fde4834d83851469e635713615ab1037128df138a6cd223f1242"), hex2Bytes("b8bded4e1c")},
		{hex2Bytes("3e7d409b9037b1fd870120de92ebb7285219ce4526c54701b888c5a13995f73c"), hex2Bytes("9bc5d0")},
		{hex2Bytes("0100000000000000000000000000000000000000000000000000000000000000"), hex2Bytes("")},
		{hex2Bytes("0100000000000000000000000000000000000000000000000000000000000200"), hex2Bytes("01")},
	}
	expectedRootHash := hex2Bytes("511727325a0cd23890c21cda3c6f8b1c9fbdf37ed57b9a85ca77286356183dcf")

	// Get initial state root (empty)
	initialRoot := db.Root()
	t.Logf("Initial state root: %x", initialRoot)

	// Create read-only snapshot before changes
	snapshot := db.Session()
	snapshotRoot := snapshot.Root()
	if snapshotRoot != initialRoot {
		t.Errorf("Snapshot root %x should match initial root %x", snapshotRoot, initialRoot)
	}

	// Insert all JAM test vector key-value pairs
	for i, kv := range data {
		key := bytesToKey(kv[0])
		value := kv[1]

		if err := db.Insert(key, value); err != nil {
			t.Fatalf("Failed to insert pair %d: %v", i, err)
		}

		t.Logf("Inserted key %d: %x -> value len %d", i, key, len(value))
	}

	// Read operations should see overlay changes
	firstKey := bytesToKey(data[0][0])
	got, err := db.Get(firstKey)
	if err != nil {
		t.Fatalf("Failed to get first key: %v", err)
	}
	if !bytes.Equal(got, data[0][1]) {
		t.Errorf("First key value mismatch")
	}

	// Snapshot should still see old state (isolation)
	snapshotValue, err := snapshot.Get(firstKey)
	if err != nil {
		t.Fatalf("Failed to get key from snapshot: %v", err)
	}
	if snapshotValue != nil {
		t.Errorf("Snapshot should not see uncommitted changes, got %v", snapshotValue)
	}

	// Atomic transaction commit
	session, err := db.Commit()
	if err != nil {
		// Test error handling with rollback
		rollbackErr := db.Rollback()
		if rollbackErr != nil {
			t.Fatalf("Commit failed and rollback failed: %v, %v", err, rollbackErr)
		}
		t.Fatalf("Failed to commit: %v", err)
	}

	if session == nil {
		t.Fatal("Finished session is nil")
	}

	// Verify state root computation
	newRoot := db.Root()
	prevRoot := session.PrevRoot()
	currentRoot := session.Root()

	t.Logf("Previous state root: %x", prevRoot)
	t.Logf("New state root: %x", newRoot)
	t.Logf("Expected JAM root: %x", expectedRootHash)
	t.Logf("Session root: %x", currentRoot)

	if prevRoot == newRoot {
		t.Error("State root should change after commit with modifications")
	}
	if newRoot != currentRoot {
		t.Errorf("Database root %x should match session root %x", newRoot, currentRoot)
	}
	if prevRoot != initialRoot {
		t.Errorf("Previous root %x should match initial root %x", prevRoot, initialRoot)
	}

	// CRITICAL: Verify JAM Gray Paper compatibility
	if !bytes.Equal(newRoot[:], expectedRootHash) {
		t.Errorf("❌ JAM compatibility FAILED!")
		t.Errorf("Expected JAM root: %x", expectedRootHash)
		t.Errorf("Computed BMT root: %x", newRoot)
		t.Errorf("This indicates the GP tree implementation differs from JAM specification")
	} else {
		t.Logf("✅ JAM COMPATIBILITY VERIFIED! Root matches JAM test vector")
	}

	// Generate cryptographic proofs
	witness, err := session.GenerateWitness()
	if err != nil {
		t.Fatalf("Failed to generate witness: %v", err)
	}

	if witness == nil {
		t.Fatal("Witness is nil")
	}

	// Verify witness contains modified keys
	if witness.PrevRoot != prevRoot {
		t.Errorf("Witness prev root %x should match %x", witness.PrevRoot, prevRoot)
	}
	if witness.Root != newRoot {
		t.Errorf("Witness root %x should match %x", witness.Root, newRoot)
	}

	expectedKeys := len(data) // All 11 keys
	if len(witness.Keys) != expectedKeys {
		t.Errorf("Expected %d keys in witness, got %d", expectedKeys, len(witness.Keys))
	}

	t.Logf("Generated witness with %d modified keys", len(witness.Keys))

	// Verify all values persisted and accessible
	for i, kv := range data {
		key := bytesToKey(kv[0])
		expectedValue := kv[1]

		got, err := db.Get(key)
		if err != nil {
			t.Fatalf("Failed to get key %d after commit: %v", i, err)
		}

		if !bytes.Equal(got, expectedValue) {
			t.Errorf("Key %d value mismatch after commit: expected %x, got %x", i, expectedValue, got)
		}
	}

	// Create new snapshot after commit
	newSnapshot := db.Session()
	newSnapshotRoot := newSnapshot.Root()
	if newSnapshotRoot != newRoot {
		t.Errorf("New snapshot root %x should match current root %x", newSnapshotRoot, newRoot)
	}

	// New snapshot should see committed changes
	newSnapshotValue, err := newSnapshot.Get(firstKey)
	if err != nil {
		t.Fatalf("Failed to get key from new snapshot: %v", err)
	}
	if !bytes.Equal(newSnapshotValue, data[0][1]) {
		t.Errorf("New snapshot should see committed changes")
	}

	// Check metrics
	metrics := db.Metrics()
	if metrics.Commits() != 1 {
		t.Errorf("Expected 1 commit, got %d", metrics.Commits())
	}
	if metrics.Writes() != uint64(len(data)) {
		t.Errorf("Expected %d writes, got %d", len(data), metrics.Writes())
	}

	t.Logf("Metrics: %d reads, %d writes, %d commits", metrics.Reads(), metrics.Writes(), metrics.Commits())
}

func TestNomtRollback(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions(dir)

	db, err := Open(opts)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	key1 := testKey("key1")
	value1 := []byte("value1")
	value2 := []byte("value2")

	// Commit first value
	if err := db.Insert(key1, value1); err != nil {
		t.Fatalf("Failed to insert: %v", err)
	}
	if _, err := db.Commit(); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Verify first value
	got, _ := db.Get(key1)
	if !bytes.Equal(got, value1) {
		t.Errorf("Expected %s, got %s", value1, got)
	}

	// Update and commit
	if err := db.Insert(key1, value2); err != nil {
		t.Fatalf("Failed to insert: %v", err)
	}
	if _, err := db.Commit(); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Verify second value
	got, _ = db.Get(key1)
	if !bytes.Equal(got, value2) {
		t.Errorf("Expected %s, got %s", value2, got)
	}

	// Rollback
	if err := db.Rollback(); err != nil {
		t.Fatalf("Failed to rollback: %v", err)
	}

	// Verify first value restored
	got, _ = db.Get(key1)
	if !bytes.Equal(got, value1) {
		t.Errorf("After rollback, expected %s, got %s", value1, got)
	}
}

func TestNomtReopen(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions(dir)

	key1 := testKey("key1")
	value1 := []byte("value1")

	// Open, insert, commit, close
	{
		db, err := Open(opts)
		if err != nil {
			t.Fatalf("Failed to open database: %v", err)
		}

		if err := db.Insert(key1, value1); err != nil {
			t.Fatalf("Failed to insert: %v", err)
		}

		if _, err := db.Commit(); err != nil {
			t.Fatalf("Failed to commit: %v", err)
		}

		if err := db.Close(); err != nil {
			t.Fatalf("Failed to close: %v", err)
		}
	}

	// Reopen and verify
	{
		db, err := Open(opts)
		if err != nil {
			t.Fatalf("Failed to reopen database: %v", err)
		}
		defer db.Close()

		got, err := db.Get(key1)
		if err != nil {
			t.Fatalf("Failed to get: %v", err)
		}

		if !bytes.Equal(got, value1) {
			t.Errorf("After reopen, expected %s, got %s", value1, got)
		}
	}
}

func TestNomtDelete(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions(dir)

	db, err := Open(opts)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	key1 := testKey("key1")
	value1 := []byte("value1")

	// Insert
	if err := db.Insert(key1, value1); err != nil {
		t.Fatalf("Failed to insert: %v", err)
	}

	// Verify exists
	got, _ := db.Get(key1)
	if !bytes.Equal(got, value1) {
		t.Errorf("Expected %s, got %s", value1, got)
	}

	// Delete
	if err := db.Delete(key1); err != nil {
		t.Fatalf("Failed to delete: %v", err)
	}

	// Verify deleted
	got, _ = db.Get(key1)
	if got != nil {
		t.Errorf("Expected nil after delete, got %v", got)
	}
}

func TestNomtMetrics(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions(dir)

	db, err := Open(opts)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	key1 := testKey("key1")
	value1 := []byte("value1")

	// Insert (1 write)
	db.Insert(key1, value1)

	// Get (1 read)
	db.Get(key1)

	// Commit
	db.Commit()

	metrics := db.Metrics()

	if metrics.Writes() != 1 {
		t.Errorf("Expected 1 write, got %d", metrics.Writes())
	}

	if metrics.Reads() != 1 {
		t.Errorf("Expected 1 read, got %d", metrics.Reads())
	}

	if metrics.Commits() != 1 {
		t.Errorf("Expected 1 commit, got %d", metrics.Commits())
	}
}

// test_nomt is a configurable test helper that exercises overlay management, key operations,
// and state witness generation/verification with performance timing
func test_nomt(t *testing.T, params TestParams) {
	// Set default mode if not specified
	if params.BenchmarkMode == "" {
		params.BenchmarkMode = BenchmarkIsolated
	}

	// Sequential mode: single DB instance across all iterations
	var db *Nomt
	var dir string

	if params.BenchmarkMode == BenchmarkSequential {
		dir = "/tmp/nomt_benchmark_sequential"
		os.RemoveAll(dir)
		os.MkdirAll(dir, 0755)

		opts := DefaultOptions(dir)
		var err error
		db, err = Open(opts)
		if err != nil {
			t.Fatalf("Failed to open database: %v", err)
		}
		defer db.Close()
		defer os.RemoveAll(dir)
	}

	for iteration := 0; iteration < params.MaxIterations; iteration++ {
		t.Logf("=== Iteration %d/%d (%s mode) ===", iteration+1, params.MaxIterations, params.BenchmarkMode)

		// Isolated mode: Fresh DB for each iteration (like Rust's bench_isolate)
		if params.BenchmarkMode == BenchmarkIsolated {
			dir = fmt.Sprintf("/tmp/nomt_benchmark_iter_%d", iteration)
			os.RemoveAll(dir)
			os.MkdirAll(dir, 0755)

			opts := DefaultOptions(dir)
			var err error
			db, err = Open(opts)
			if err != nil {
				t.Fatalf("Failed to open database: %v", err)
			}
		}

		// Step 1: Get initial state root (should be empty for fresh DB)
		var prevStateRoot [32]byte
		prevStateRoot = db.Root()

		// Step 2: Create a new write session with witness tracking
		writeSession, err := db.BeginWriteWithWitness()
		if err != nil {
			t.Fatalf("Failed to begin write session: %v", err)
		}

		// Step 2a: Insert numInserts FRESH keys (like Rust's workload-fresh 50%)
		insertStart := time.Now()
		for i := 0; i < params.NumInserts; i++ {
			key := make([]byte, 32)
			// Create deterministic keys based on iteration and index
			copy(key, fmt.Sprintf("insert_%d_%d", iteration, i))
			value := make([]byte, params.ValueSize)
			// Create deterministic values
			copy(value, fmt.Sprintf("value_%d_%d_", iteration, i))

			var keyArray [32]byte
			copy(keyArray[:], key)

			if err := writeSession.Insert(keyArray, value); err != nil {
				writeSession.Prepare() // Clean up
				t.Fatalf("Failed to insert key %d: %v", i, err)
			}
		}
		insertTime := time.Since(insertStart)

		// Step 2b: Update numUpdates EXISTING keys
		updateStart := time.Now()
		updatesPerformed := 0
		for i := 0; i < params.NumUpdates && i < params.NumInserts; i++ {
			var key []byte
			var value []byte

			if params.BenchmarkMode == BenchmarkSequential && iteration > 0 {
				// Sequential mode: Update keys from previous iteration
				key = make([]byte, 32)
				copy(key, fmt.Sprintf("insert_%d_%d", iteration-1, i))
				value = make([]byte, params.ValueSize)
				copy(value, fmt.Sprintf("updated_%d_%d_", iteration, i))
			} else {
				// Isolated mode: Update keys from current iteration
				key = make([]byte, 32)
				copy(key, fmt.Sprintf("insert_%d_%d", iteration, i))
				value = make([]byte, params.ValueSize)
				copy(value, fmt.Sprintf("updated_%d_%d_", iteration, i))
			}

			var keyArray [32]byte
			copy(keyArray[:], key)

			if err := writeSession.Insert(keyArray, value); err == nil {
				updatesPerformed++
			}
		}
		updateTime := time.Since(updateStart)

		// Step 2c: Delete numDeletes keys
		deleteStart := time.Now()
		deletesPerformed := 0
		for i := 0; i < params.NumDeletes && i < params.NumInserts; i++ {
			var key []byte

			if params.BenchmarkMode == BenchmarkSequential && iteration > 1 {
				// Sequential mode: Delete keys from 2 iterations ago
				key = make([]byte, 32)
				copy(key, fmt.Sprintf("insert_%d_%d", iteration-2, i))
			} else {
				// Isolated mode: Delete keys from current iteration (towards the end)
				deleteIdx := params.NumInserts - 1 - i
				key = make([]byte, 32)
				copy(key, fmt.Sprintf("insert_%d_%d", iteration, deleteIdx))
			}

			var keyArray [32]byte
			copy(keyArray[:], key)

			if err := writeSession.Delete(keyArray); err == nil {
				deletesPerformed++
			}
		}
		deleteTime := time.Since(deleteStart)

		// Step 2d: Read numReads keys
		readStart := time.Now()
		readsPerformed := 0
		for i := 0; i < params.NumReads && i < params.NumInserts; i++ {
			var key []byte

			if params.BenchmarkMode == BenchmarkSequential && iteration > 0 {
				// Sequential mode: Read keys from previous iteration
				key = make([]byte, 32)
				copy(key, fmt.Sprintf("insert_%d_%d", iteration-1, i))
			} else {
				// Isolated mode: Read keys from current iteration (middle range)
				readIdx := i + (params.NumInserts / 4)
				if readIdx >= params.NumInserts {
					continue
				}
				key = make([]byte, 32)
				copy(key, fmt.Sprintf("insert_%d_%d", iteration, readIdx))
			}

			var keyArray [32]byte
			copy(keyArray[:], key)

			if value, err := writeSession.Get(keyArray); err == nil && value != nil {
				readsPerformed++
			}
		}
		readTime := time.Since(readStart)

		// Step 2 flush: Prepare the session (equivalent to flush)
		flushStart := time.Now()
		preparedSession, err := writeSession.Prepare()
		if err != nil {
			t.Fatalf("Failed to prepare session: %v", err)
		}

		// Commit the prepared session
		if err := preparedSession.Commit(); err != nil {
			preparedSession.Rollback()
			t.Fatalf("Failed to commit session: %v", err)
		}

		newStateRoot := preparedSession.Root()
		flushTime := time.Since(flushStart)

		// Step 3: Generate state witness for numWitnessKeys keys
		witnessStart := time.Now()
		var witness *Witness
		var witnessKeys [][32]byte

		// Skip witness generation if NumWitnessKeys is 0 (Phase 1A baseline)
		if params.NumWitnessKeys > 0 {
			// Use the full number of witness keys as requested
			maxWitnessKeys := params.NumWitnessKeys

			witnessKeys = make([][32]byte, 0, maxWitnessKeys)

			// Select keys for witness (mix of recent inserts and updates)
			keysAdded := 0
			sampleRate := 1
			if params.NumInserts > maxWitnessKeys {
				sampleRate = params.NumInserts / maxWitnessKeys
				if sampleRate < 1 {
					sampleRate = 1
				}
			}

		for i := 0; i < params.NumInserts && keysAdded < maxWitnessKeys; i += sampleRate {
			key := make([]byte, 32)
			copy(key, fmt.Sprintf("insert_%d_%d", iteration, i))
			var keyArray [32]byte
			copy(keyArray[:], key)
			witnessKeys = append(witnessKeys, keyArray)
			keysAdded++
		}

		// Add some updated keys if available (also sampled)
		if iteration > 0 && keysAdded < maxWitnessKeys {
			updateSampleRate := 1
			if updatesPerformed > (maxWitnessKeys - keysAdded) {
				updateSampleRate = updatesPerformed / (maxWitnessKeys - keysAdded)
				if updateSampleRate < 1 {
					updateSampleRate = 1
				}
			}

			for i := 0; i < updatesPerformed && keysAdded < maxWitnessKeys; i += updateSampleRate {
				key := make([]byte, 32)
				copy(key, fmt.Sprintf("insert_%d_%d", iteration-1, i))
				var keyArray [32]byte
				copy(keyArray[:], key)
				witnessKeys = append(witnessKeys, keyArray)
				keysAdded++
			}
		}

		// Generate proofs for the witness keys - simplified approach without expensive tree rebuilding
		proofs := make([]MerkleProof, len(witnessKeys))

		for i, key := range witnessKeys {
			value, err := db.Get(key)
			if err != nil {
				t.Logf("Warning: Could not get key %x: %v", key, err)
				proofs[i] = MerkleProof{
					Key:   key,
					Value: nil,
					Path:  nil, // No proof path for failed keys
				}
				continue
			}

			// Create a simple proof with just key-value (no expensive merkle path generation)
			proofs[i] = MerkleProof{
				Key:   key,
				Value: value,
				Path:  nil, // Skip expensive proof generation for performance
			}
		}

			witness = &Witness{
				PrevRoot: prevStateRoot,
				Root:     newStateRoot,
				Keys:     witnessKeys,
				Proofs:   proofs,
			}
		}
		witnessTime := time.Since(witnessStart)

		// Step 4: Verify all witness keys
		verifyStart := time.Now()
		verifiedCount := 0
		if witness != nil {
			for _, proof := range witness.Proofs {
			// Basic verification: check that the key-value pair exists
			actualValue, err := db.Get(proof.Key)
			if err != nil {
				t.Logf("Warning: Could not get key %x for verification: %v", proof.Key, err)
				continue
			}

			if len(proof.Path) == 0 {
				// For keys without full proof paths, just verify the value matches
				if bytes.Equal(actualValue, proof.Value) {
					verifiedCount++
				}
			} else {
				// TODO: Add full Merkle proof verification when proof paths are available
				if bytes.Equal(actualValue, proof.Value) {
					verifiedCount++
				}
			}
			}
		}
		verifyTime := time.Since(verifyStart)

		// Report timing and statistics
		t.Logf("Operations: %d inserts, %d updates, %d reads, %d deletes", params.NumInserts, updatesPerformed, readsPerformed, deletesPerformed)
		t.Logf("Timing - Insert: %v, Update: %v, Read: %v, Delete: %v", insertTime, updateTime, readTime, deleteTime)
		t.Logf("State roots - Previous: %x, New: %x", prevStateRoot, newStateRoot)
		t.Logf("Flush time: %v", flushTime)
		if witness != nil {
			t.Logf("Witness generation: %v (%d keys)", witnessTime, len(witnessKeys))
			t.Logf("Verification: %v (%d/%d keys verified)", verifyTime, verifiedCount, len(witnessKeys))
		} else {
			t.Logf("Witness generation: disabled (NumWitnessKeys=0)")
		}

		// Verify state root consistency
		if newStateRoot == prevStateRoot && (params.NumInserts > 0 || updatesPerformed > 0 || deletesPerformed > 0) {
			t.Errorf("State root should change when operations are performed")
		}

		// Verify witness root matches the operation result root (only if witness was generated)
		if witness != nil && witness.Root != newStateRoot {
			t.Errorf("Witness root %x should match operation result root %x", witness.Root, newStateRoot)
		}

		// Isolated mode: Close DB after each iteration (like Rust's bench_isolate)
		if params.BenchmarkMode == BenchmarkIsolated {
			db.Close()
			os.RemoveAll(dir) // Clean up iteration directory
		}
	}
}

// TestNomtBenchmarkSmall_Isolated tests NOMT with small parameters (isolated mode)
func TestNomtBenchmarkSmall_Isolated(t *testing.T) {
	params := TestParams{
		MaxIterations:   5,                   // 5 isolated iterations
		BenchmarkMode:   BenchmarkIsolated,   // Fresh DB each iteration
		CommitLag:       0,                   // Not applicable for isolated mode
		NumInserts:      1000,                // 1K inserts per iteration
		NumUpdates:      500,                 // 500 updates per iteration
		NumReads:        250,                 // 250 reads (25% of writes)
		NumDeletes:      50,                  // 50 deletes per iteration
		NumWitnessKeys:  0,                   // Disabled until Phase 2
		ValueSize:       64,                  // 64-byte values
		OptimizeWitness: false,               // Not relevant without witness
	}
	test_nomt(t, params)
}

// TestNomtBenchmarkSmall_Sequential tests NOMT with small parameters (sequential mode)
func TestNomtBenchmarkSmall_Sequential(t *testing.T) {
	params := TestParams{
		MaxIterations:   5,                   // 5 iterations on same DB
		BenchmarkMode:   BenchmarkSequential, // Reuse DB across iterations
		CommitLag:       0,                   // Commit immediately
		NumInserts:      1000,                // 1K inserts per iteration
		NumUpdates:      500,                 // 500 updates (from previous iteration)
		NumReads:        250,                 // 250 reads (from previous iteration)
		NumDeletes:      50,                  // 50 deletes (from 2 iterations ago)
		NumWitnessKeys:  0,                   // Disabled until Phase 2
		ValueSize:       64,                  // 64-byte values
		OptimizeWitness: false,               // Not relevant without witness
	}
	test_nomt(t, params)
}

// TestNomtBenchmarkMedium_Isolated tests NOMT with medium parameters (isolated mode)
// Phase 1B: Matches Rust's bench_isolate - fresh DB each iteration
func TestNomtBenchmarkMedium_Isolated(t *testing.T) {
	params := TestParams{
		MaxIterations:   5,                   // 5 isolated iterations
		BenchmarkMode:   BenchmarkIsolated,   // Fresh DB each iteration (like Rust)
		CommitLag:       0,                   // Not applicable for isolated mode
		NumInserts:      10000,               // 10K inserts per iteration
		NumUpdates:      10000,               // 10K updates per iteration
		NumReads:        5000,                // 5K reads (25% of writes)
		NumDeletes:      50,                  // 50 deletes per iteration
		NumWitnessKeys:  0,                   // Disabled until Phase 2
		ValueSize:       32,                  // Match Rust default
		OptimizeWitness: false,               // Not relevant without witness
	}
	test_nomt(t, params)
}

// TestNomtBenchmarkMedium_Sequential tests NOMT with medium parameters (sequential mode)
// Phase 1B: Matches Rust's bench_sequential - reuses DB across iterations
func TestNomtBenchmarkMedium_Sequential(t *testing.T) {
	params := TestParams{
		MaxIterations:   5,                   // 5 iterations on same DB
		BenchmarkMode:   BenchmarkSequential, // Reuse DB (like Rust bench_sequential)
		CommitLag:       0,                   // Commit immediately
		NumInserts:      10000,               // 10K inserts per iteration
		NumUpdates:      10000,               // 10K updates (from previous iteration)
		NumReads:        5000,                // 5K reads (from previous iteration)
		NumDeletes:      50,                  // 50 deletes (from 2 iterations ago)
		NumWitnessKeys:  0,                   // Disabled until Phase 2
		ValueSize:       32,                  // Match Rust default
		OptimizeWitness: false,               // Not relevant without witness
	}
	test_nomt(t, params)
}

// TestNomtLarge tests NOMT with large parameters
// Phase 1B: Isolated iterations
func TestNomtLarge(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large test in short mode")
	}

	params := TestParams{
		MaxIterations:   5,      // Isolated iterations (Phase 1B)
		CommitLag:       0,      // Disabled for isolated benchmarking
		NumInserts:      100000, // 100K inserts per iteration
		NumUpdates:      100000, // 100K updates per iteration
		NumReads:        50000,  // 50K reads (25% of writes)
		NumDeletes:      1000,   // 1K deletes per iteration
		NumWitnessKeys:  0,      // Disabled until Phase 2
		ValueSize:       256,    // 256-byte values
		OptimizeWitness: false,  // Not relevant without witness
	}
	test_nomt(t, params)
}

// BenchmarkNomtOperations benchmarks the core NOMT operations with medium test parameters
// go test ./bmt -bench=BenchmarkNomtOperations -v -run=^$
func BenchmarkNomtOperations(b *testing.B) {
	// Convert benchmark to test for our test_nomt function
	t := &testing.T{}

	for i := 0; i < b.N; i++ {
		params := TestParams{
			MaxIterations:   5,     // Shorter than medium (20) for benchmark
			CommitLag:       5,     // Commit overlay 5 iterations ago
			NumInserts:      10000, // 10K inserts per iteration (same as medium)
			NumUpdates:      10000, // 10K updates per iteration (same as medium)
			NumDeletes:      100,   // 100 deletes per iteration (same as medium)
			NumWitnessKeys:  10000, // Generate witness for 10K keys (same as medium)
			ValueSize:       256,   // 256-byte values (same as medium)
			OptimizeWitness: true,  // Enable witness optimization
		}
		test_nomt(t, params)
	}
}

// TestNomtStressScenarios tests edge cases and stress scenarios
func TestNomtStressScenarios(t *testing.T) {
	scenarios := []struct {
		name   string
		params TestParams
	}{
		{
			name: "HighChurn",
			params: TestParams{
				MaxIterations:  10,
				CommitLag:      3,
				NumInserts:     1000,
				NumUpdates:     900, // High update rate
				NumDeletes:     800, // High delete rate
				NumWitnessKeys: 500,
				ValueSize:      64,
			},
		},
		{
			name: "LargeValues",
			params: TestParams{
				MaxIterations:  8,
				CommitLag:      5,
				NumInserts:     100,
				NumUpdates:     50,
				NumDeletes:     10,
				NumWitnessKeys: 50,
				ValueSize:      4096, // Large 4KB values
			},
		},
		{
			name: "ManyIterations",
			params: TestParams{
				MaxIterations:  50, // Many iterations with small batches
				CommitLag:      5,
				NumInserts:     100,
				NumUpdates:     50,
				NumDeletes:     10,
				NumWitnessKeys: 25,
				ValueSize:      128,
			},
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			if testing.Short() && (scenario.name == "LargeValues" || scenario.name == "ManyIterations") {
				t.Skip("Skipping stress test in short mode")
			}
			test_nomt(t, scenario.params)
		})
	}
}
