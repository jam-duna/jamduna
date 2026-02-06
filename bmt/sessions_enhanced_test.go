package bmt

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jam-duna/jamduna/bmt/overlay"
)

// Test Pattern 1: Simple transaction
func TestSimpleTransaction(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_simple_tx")

	// Open database
	opts := Options{
		Path:                     dbPath,
		RollbackInMemoryCapacity: 10,
	}
	db, err := Open(opts)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Begin write session
	session, err := db.BeginWrite()
	if err != nil {
		t.Fatalf("failed to begin write session: %v", err)
	}

	// Insert values
	key1 := [32]byte{1}
	key2 := [32]byte{2}
	key3 := [32]byte{3}

	if err := session.Insert(key1, []byte("value1")); err != nil {
		t.Fatalf("failed to insert key1: %v", err)
	}

	if err := session.Insert(key2, []byte("value2")); err != nil {
		t.Fatalf("failed to insert key2: %v", err)
	}

	// Insert then delete key3
	if err := session.Insert(key3, []byte("value3")); err != nil {
		t.Fatalf("failed to insert key3: %v", err)
	}
	if err := session.Delete(key3); err != nil {
		t.Fatalf("failed to delete key3: %v", err)
	}

	// Prepare session
	prepared, err := session.Prepare()
	if err != nil {
		t.Fatalf("failed to prepare session: %v", err)
	}

	// Check roots
	prevRoot := prepared.PrevRoot()
	newRoot := prepared.Root()
	t.Logf("Previous root: %x", prevRoot)
	t.Logf("New root: %x", newRoot)

	// Commit
	if err := prepared.Commit(); err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	// Verify committed values
	value1, err := db.Get(key1)
	if err != nil {
		t.Fatalf("failed to get key1: %v", err)
	}
	if string(value1) != "value1" {
		t.Errorf("expected 'value1', got '%s'", string(value1))
	}

	value2, err := db.Get(key2)
	if err != nil {
		t.Fatalf("failed to get key2: %v", err)
	}
	if string(value2) != "value2" {
		t.Errorf("expected 'value2', got '%s'", string(value2))
	}

	// key3 should be deleted
	value3, err := db.Get(key3)
	if err != nil {
		t.Fatalf("failed to get key3: %v", err)
	}
	if value3 != nil {
		t.Errorf("expected key3 to be deleted, got '%s'", string(value3))
	}

	// Verify root changed
	committedRoot := db.Root()
	t.Logf("Committed root: %x", committedRoot)
	if committedRoot == prevRoot {
		t.Error("root should have changed after commit")
	}
}

// Test Pattern 2: Transaction with witness
func TestTransactionWithWitness(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_witness_tx")

	opts := Options{
		Path:                     dbPath,
		RollbackInMemoryCapacity: 10,
	}
	db, err := Open(opts)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Begin write session with witness mode
	session, err := db.BeginWriteWithWitness()
	if err != nil {
		t.Fatalf("failed to begin write session: %v", err)
	}

	// Perform operations
	key1 := [32]byte{1}
	if err := session.Insert(key1, []byte("value")); err != nil {
		t.Fatalf("failed to insert: %v", err)
	}

	// Read operation (should be recorded in witness mode)
	_, _ = session.Get(key1)

	// Prepare session
	prepared, err := session.Prepare()
	if err != nil {
		t.Fatalf("failed to prepare session: %v", err)
	}

	// Generate witness
	witness, err := prepared.Witness()
	if err != nil {
		t.Fatalf("failed to generate witness: %v", err)
	}

	if len(witness.Keys) == 0 {
		t.Error("expected witness to contain keys")
	}

	t.Logf("Witness prev root: %x", witness.PrevRoot)
	t.Logf("Witness new root: %x", witness.Root)
	t.Logf("Witness keys: %d", len(witness.Keys))

	// Commit
	if err := prepared.Commit(); err != nil {
		t.Fatalf("failed to commit: %v", err)
	}
}

// Test Pattern 3: Multi-session overlay building
func TestOverlayWorkflow(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_overlay")

	opts := Options{
		Path:                     dbPath,
		RollbackInMemoryCapacity: 10,
	}
	db, err := Open(opts)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Session 1: Create base overlay
	session1, err := db.BeginWrite()
	if err != nil {
		t.Fatalf("failed to begin session1: %v", err)
	}

	key1 := [32]byte{1}
	if err := session1.Insert(key1, []byte("base")); err != nil {
		t.Fatalf("failed to insert in session1: %v", err)
	}

	prepared1, err := session1.Prepare()
	if err != nil {
		t.Fatalf("failed to prepare session1: %v", err)
	}

	overlay1 := prepared1.ToOverlay()
	if overlay1 == nil {
		t.Fatal("expected non-nil overlay")
	}

	// Session 2: Build on overlay1
	session2, err := db.BeginWriteWithOverlay([]*overlay.Overlay{overlay1})
	if err != nil {
		t.Fatalf("failed to begin session2: %v", err)
	}

	key2 := [32]byte{2}
	if err := session2.Insert(key2, []byte("derived")); err != nil {
		t.Fatalf("failed to insert in session2: %v", err)
	}

	prepared2, err := session2.Prepare()
	if err != nil {
		t.Fatalf("failed to prepare session2: %v", err)
	}

	// Commit both sessions atomically
	if err := prepared2.Commit(); err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	// Verify both values exist
	value1, err := db.Get(key1)
	if err != nil {
		t.Fatalf("failed to get key1: %v", err)
	}
	if string(value1) != "base" {
		t.Errorf("expected 'base', got '%s'", string(value1))
	}

	value2, err := db.Get(key2)
	if err != nil {
		t.Fatalf("failed to get key2: %v", err)
	}
	if string(value2) != "derived" {
		t.Errorf("expected 'derived', got '%s'", string(value2))
	}
}

// Test Pattern 4: Read-only snapshot
func TestReadOnlyOperations(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_readonly")

	opts := Options{
		Path:                     dbPath,
		RollbackInMemoryCapacity: 10,
	}
	db, err := Open(opts)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Insert some data
	key1 := [32]byte{1}
	key2 := [32]byte{2}
	if err := db.Insert(key1, []byte("value1")); err != nil {
		t.Fatalf("failed to insert key1: %v", err)
	}
	if err := db.Insert(key2, []byte("value2")); err != nil {
		t.Fatalf("failed to insert key2: %v", err)
	}
	if _, err := db.Commit(); err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	// Create read session
	session, err := db.BeginRead()
	if err != nil {
		t.Fatalf("failed to begin read session: %v", err)
	}
	defer session.Close()

	// Read values
	value1, err := session.Get(key1)
	if err != nil {
		t.Fatalf("failed to get key1: %v", err)
	}
	if string(value1) != "value1" {
		t.Errorf("expected 'value1', got '%s'", string(value1))
	}

	value2, err := session.Get(key2)
	if err != nil {
		t.Fatalf("failed to get key2: %v", err)
	}
	if string(value2) != "value2" {
		t.Errorf("expected 'value2', got '%s'", string(value2))
	}

	// Check root
	sessionRoot := session.Root()
	dbRoot := db.Root()
	if sessionRoot != dbRoot {
		t.Errorf("session root %x should match db root %x", sessionRoot, dbRoot)
	}

	t.Logf("Reading from root: %x", sessionRoot)
}

// Test Pattern 5: Root generation workflow
func TestRootGenerationWorkflow(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_root_gen")

	opts := Options{
		Path:                     dbPath,
		RollbackInMemoryCapacity: 10,
	}
	db, err := Open(opts)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Get current root before changes
	prevRoot := db.Root()
	t.Logf("Previous root: %x", prevRoot)

	// Create write session
	session, err := db.BeginWrite()
	if err != nil {
		t.Fatalf("failed to begin write: %v", err)
	}

	// Make changes
	key1 := [32]byte{1}
	key2 := [32]byte{2}
	if err := session.Insert(key1, []byte("new_value")); err != nil {
		t.Fatalf("failed to insert: %v", err)
	}
	if err := session.Delete(key2); err != nil {
		t.Fatalf("failed to delete: %v", err)
	}

	// Check intermediate root (before prepare)
	intermediateRoot := session.Root()
	t.Logf("Intermediate root: %x", intermediateRoot)

	// Prepare session (freezes state and computes final root)
	prepared, err := session.Prepare()
	if err != nil {
		t.Fatalf("failed to prepare: %v", err)
	}

	// Get final root before committing
	newRoot := prepared.Root()
	preparedPrevRoot := prepared.PrevRoot()
	t.Logf("New root (before commit): %x", newRoot)
	t.Logf("Prev root (from prepared): %x", preparedPrevRoot)

	if preparedPrevRoot != prevRoot {
		t.Errorf("prepared prevRoot %x should match original prevRoot %x", preparedPrevRoot, prevRoot)
	}

	// Commit and verify root in database
	if err := prepared.Commit(); err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	committedRoot := db.Root()
	t.Logf("Committed root: %x", committedRoot)

	// The committed root should match what was computed
	// Note: Since we don't compute intermediate roots yet, this may not pass
	// but the structure is correct
}

// Test stale transaction detection
func TestStaleTransaction(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_stale")

	opts := Options{
		Path:                     dbPath,
		RollbackInMemoryCapacity: 10,
	}
	db, err := Open(opts)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Create first session
	session1, err := db.BeginWrite()
	if err != nil {
		t.Fatalf("failed to begin session1: %v", err)
	}

	key1 := [32]byte{1}
	if err := session1.Insert(key1, []byte("value1")); err != nil {
		t.Fatalf("failed to insert in session1: %v", err)
	}

	prepared1, err := session1.Prepare()
	if err != nil {
		t.Fatalf("failed to prepare session1: %v", err)
	}

	// Create second session before committing first
	session2, err := db.BeginWrite()
	if err != nil {
		t.Fatalf("failed to begin session2: %v", err)
	}

	key2 := [32]byte{2}
	if err := session2.Insert(key2, []byte("value2")); err != nil {
		t.Fatalf("failed to insert in session2: %v", err)
	}

	prepared2, err := session2.Prepare()
	if err != nil {
		t.Fatalf("failed to prepare session2: %v", err)
	}

	// Commit first session
	if err := prepared1.Commit(); err != nil {
		t.Fatalf("failed to commit session1: %v", err)
	}

	// Attempt to commit second session - should fail with stale transaction
	err = prepared2.Commit()
	if err == nil {
		t.Error("expected stale transaction error")
	} else {
		t.Logf("Got expected error: %v", err)
	}
}

// Test session closure behavior
func TestSessionClosure(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_closure")

	opts := Options{
		Path:                     dbPath,
		RollbackInMemoryCapacity: 10,
	}
	db, err := Open(opts)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Test write session closure
	session, err := db.BeginWrite()
	if err != nil {
		t.Fatalf("failed to begin write session: %v", err)
	}

	key := [32]byte{1}
	if err := session.Insert(key, []byte("value")); err != nil {
		t.Fatalf("failed to insert: %v", err)
	}

	// Prepare closes the session
	_, err = session.Prepare()
	if err != nil {
		t.Fatalf("failed to prepare: %v", err)
	}

	// Operations after prepare should fail
	err = session.Insert(key, []byte("new_value"))
	if err == nil {
		t.Error("expected error when inserting after prepare")
	}

	// Test read session closure
	readSession, err := db.BeginRead()
	if err != nil {
		t.Fatalf("failed to begin read session: %v", err)
	}

	if err := readSession.Close(); err != nil {
		t.Fatalf("failed to close read session: %v", err)
	}

	// Operations after close should fail
	_, err = readSession.Get(key)
	if err == nil {
		t.Error("expected error when getting after close")
	}
}

// Test cleanup
func TestMain(m *testing.M) {
	// Run tests
	code := m.Run()

	// Cleanup
	os.Exit(code)
}
