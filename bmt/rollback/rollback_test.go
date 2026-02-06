package rollback

import (
	"bytes"
	"testing"

	"github.com/jam-duna/jamduna/bmt/beatree"
)

// Simple in-memory state for testing
type testState struct {
	data map[beatree.Key][]byte
}

func newTestState() *testState {
	return &testState{
		data: make(map[beatree.Key][]byte),
	}
}

func testKey(s string) beatree.Key {
	var keyBytes [32]byte
	copy(keyBytes[:], s)
	return beatree.KeyFromBytes(keyBytes[:])
}

func (ts *testState) lookup(key beatree.Key) ([]byte, error) {
	val, ok := ts.data[key]
	if !ok {
		return nil, nil
	}
	return val, nil
}

func (ts *testState) apply(changeset map[beatree.Key]*beatree.Change) error {
	for key, change := range changeset {
		if change.IsDelete() {
			delete(ts.data, key)
		} else {
			ts.data[key] = change.Value
		}
	}
	return nil
}

func TestRollbackCommit(t *testing.T) {
	state := newTestState()
	dir := t.TempDir()

	cfg := Config{
		SegLogDir:        dir,
		InMemoryCapacity: 10,
	}

	rb, err := NewRollback(state.lookup, state.apply, cfg)
	if err != nil {
		t.Fatalf("Failed to create rollback: %v", err)
	}
	defer rb.Close()

	// Create changeset
	key := testKey("testkey")
	changeset := map[beatree.Key]*beatree.Change{
		key: beatree.NewInsertChange([]byte("value1")),
	}

	// Commit delta
	recordId, err := rb.Commit(changeset)
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	if recordId != 1 {
		t.Errorf("Expected RecordId 1, got %d", recordId)
	}

	// Apply changeset to state
	state.apply(changeset)

	// Verify value in state
	value, _ := state.lookup(key)
	if !bytes.Equal(value, []byte("value1")) {
		t.Errorf("Expected value1, got %s", value)
	}
}

func TestRollbackSingle(t *testing.T) {
	state := newTestState()
	dir := t.TempDir()

	cfg := Config{
		SegLogDir:        dir,
		InMemoryCapacity: 10,
	}

	rb, err := NewRollback(state.lookup, state.apply, cfg)
	if err != nil {
		t.Fatalf("Failed to create rollback: %v", err)
	}
	defer rb.Close()

	key := testKey("testkey")

	// Insert value
	changeset1 := map[beatree.Key]*beatree.Change{
		key: beatree.NewInsertChange([]byte("value1")),
	}

	_, err = rb.Commit(changeset1)
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}
	state.apply(changeset1)

	// Verify insert
	value, _ := state.lookup(key)
	if !bytes.Equal(value, []byte("value1")) {
		t.Errorf("Expected value1, got %s", value)
	}

	// Update value
	changeset2 := map[beatree.Key]*beatree.Change{
		key: beatree.NewInsertChange([]byte("value2")),
	}

	_, err = rb.Commit(changeset2)
	if err != nil {
		t.Fatalf("Failed to commit update: %v", err)
	}
	state.apply(changeset2)

	// Verify update
	value, _ = state.lookup(key)
	if !bytes.Equal(value, []byte("value2")) {
		t.Errorf("Expected value2, got %s", value)
	}

	// Rollback single
	if err := rb.RollbackSingle(); err != nil {
		t.Fatalf("Failed to rollback: %v", err)
	}

	// Should be back to value1
	value, _ = state.lookup(key)
	if !bytes.Equal(value, []byte("value1")) {
		t.Errorf("Expected value1 after rollback, got %s", value)
	}
}

func TestRollbackMultiple(t *testing.T) {
	state := newTestState()
	dir := t.TempDir()

	cfg := Config{
		SegLogDir:        dir,
		InMemoryCapacity: 10,
	}

	rb, err := NewRollback(state.lookup, state.apply, cfg)
	if err != nil {
		t.Fatalf("Failed to create rollback: %v", err)
	}
	defer rb.Close()

	key := testKey("testkey")

	// Commit 1: Insert
	changeset1 := map[beatree.Key]*beatree.Change{
		key: beatree.NewInsertChange([]byte("v1")),
	}
	rec1, _ := rb.Commit(changeset1)
	state.apply(changeset1)

	// Commit 2: Update
	changeset2 := map[beatree.Key]*beatree.Change{
		key: beatree.NewInsertChange([]byte("v2")),
	}
	_, _ = rb.Commit(changeset2)
	state.apply(changeset2)

	// Commit 3: Update
	changeset3 := map[beatree.Key]*beatree.Change{
		key: beatree.NewInsertChange([]byte("v3")),
	}
	_, _ = rb.Commit(changeset3)
	state.apply(changeset3)

	// Verify current value
	value, _ := state.lookup(key)
	if !bytes.Equal(value, []byte("v3")) {
		t.Errorf("Expected v3, got %s", value)
	}

	// Rollback to rec1 (should restore to v1)
	if err := rb.RollbackTo(rec1); err != nil {
		t.Fatalf("Failed to rollback to rec1: %v", err)
	}

	value, _ = state.lookup(key)
	if !bytes.Equal(value, []byte("v1")) {
		t.Errorf("Expected v1 after rollback to rec1, got %s", value)
	}
}

func TestRollbackInsertDelete(t *testing.T) {
	state := newTestState()
	dir := t.TempDir()

	cfg := Config{
		SegLogDir:        dir,
		InMemoryCapacity: 10,
	}

	rb, err := NewRollback(state.lookup, state.apply, cfg)
	if err != nil {
		t.Fatalf("Failed to create rollback: %v", err)
	}
	defer rb.Close()

	key := testKey("testkey")

	// Insert
	changeset1 := map[beatree.Key]*beatree.Change{
		key: beatree.NewInsertChange([]byte("value")),
	}
	_, _ = rb.Commit(changeset1)
	state.apply(changeset1)

	// Verify exists
	value, _ := state.lookup(key)
	if !bytes.Equal(value, []byte("value")) {
		t.Errorf("Expected value, got %s", value)
	}

	// Rollback (should delete)
	if err := rb.RollbackSingle(); err != nil {
		t.Fatalf("Failed to rollback: %v", err)
	}

	// Key should not exist
	value, _ = state.lookup(key)
	if value != nil {
		t.Errorf("Expected nil after rollback, got %s", value)
	}
}

func TestRollbackSync(t *testing.T) {
	state := newTestState()
	dir := t.TempDir()

	cfg := Config{
		SegLogDir:        dir,
		InMemoryCapacity: 10,
	}

	rb, err := NewRollback(state.lookup, state.apply, cfg)
	if err != nil {
		t.Fatalf("Failed to create rollback: %v", err)
	}
	defer rb.Close()

	key := testKey("testkey")
	changeset := map[beatree.Key]*beatree.Change{
		key: beatree.NewInsertChange([]byte("value")),
	}

	_, err = rb.Commit(changeset)
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Sync
	if err := rb.Sync(); err != nil {
		t.Fatalf("Failed to sync: %v", err)
	}
}

func TestRollbackInMemoryCache(t *testing.T) {
	state := newTestState()
	dir := t.TempDir()

	cfg := Config{
		SegLogDir:        dir,
		InMemoryCapacity: 5,
	}

	rb, err := NewRollback(state.lookup, state.apply, cfg)
	if err != nil {
		t.Fatalf("Failed to create rollback: %v", err)
	}
	defer rb.Close()

	// Commit multiple deltas
	for i := 1; i <= 3; i++ {
		var kb [32]byte
		kb[0] = byte(i)
		key := beatree.KeyFromBytes(kb[:])
		changeset := map[beatree.Key]*beatree.Change{
			key: beatree.NewInsertChange([]byte{byte(i)}),
		}
		rb.Commit(changeset)
	}

	// Should have 3 in memory
	if rb.InMemorySize() != 3 {
		t.Errorf("Expected 3 in memory, got %d", rb.InMemorySize())
	}
}
