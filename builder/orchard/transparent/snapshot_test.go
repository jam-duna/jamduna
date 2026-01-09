package transparent

import (
	"path/filepath"
	"testing"
)

func TestSnapshot_ExportImport(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	store, err := NewTransparentStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Add test UTXOs
	for i := uint32(0); i < 5; i++ {
		txid := [32]byte{}
		txid[0] = byte(i)

		outpoint := OutPoint{TxID: txid, Index: i}
		utxo := UTXO{
			Value:        1000 * uint64(i+1),
			ScriptPubKey: []byte{byte(i)},
			Height:       100 + i,
		}

		err = store.AddUTXO(outpoint, utxo)
		if err != nil {
			t.Fatalf("Failed to add UTXO %d: %v", i, err)
		}
	}

	// Export snapshot
	snapshot, err := store.ExportSnapshot(12345)
	if err != nil {
		t.Fatalf("Failed to export snapshot: %v", err)
	}

	if snapshot.Height != 12345 {
		t.Errorf("Snapshot height mismatch: got %d, want 12345", snapshot.Height)
	}

	if len(snapshot.UTXOs) != 5 {
		t.Errorf("Snapshot UTXO count mismatch: got %d, want 5", len(snapshot.UTXOs))
	}

	// Create new store
	dbPath2 := filepath.Join(tempDir, "test2.db")
	store2, err := NewTransparentStore(dbPath2)
	if err != nil {
		t.Fatalf("Failed to create second store: %v", err)
	}
	defer store2.Close()

	// Import snapshot
	err = store2.ImportSnapshot(snapshot)
	if err != nil {
		t.Fatalf("Failed to import snapshot: %v", err)
	}

	// Verify UTXO count
	size, err := store2.GetUTXOSize()
	if err != nil {
		t.Fatalf("Failed to get UTXO size: %v", err)
	}

	if size != 5 {
		t.Errorf("Imported UTXO count mismatch: got %d, want 5", size)
	}

	// Verify root matches
	root1, err := store.GetUTXORoot()
	if err != nil {
		t.Fatalf("Failed to get root from original store: %v", err)
	}

	root2, err := store2.GetUTXORoot()
	if err != nil {
		t.Fatalf("Failed to get root from imported store: %v", err)
	}

	if root1 != root2 {
		t.Errorf("Root mismatch after import: %x != %x", root1, root2)
	}
}

func TestSnapshot_SaveLoadFile(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	snapshotPath := filepath.Join(tempDir, "snapshot.json")

	store, err := NewTransparentStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Add test UTXO
	txid := [32]byte{1, 2, 3}
	outpoint := OutPoint{TxID: txid, Index: 0}
	utxo := UTXO{
		Value:        5000,
		ScriptPubKey: []byte{0xaa, 0xbb},
		Height:       100,
	}

	err = store.AddUTXO(outpoint, utxo)
	if err != nil {
		t.Fatalf("Failed to add UTXO: %v", err)
	}

	// Export snapshot
	snapshot, err := store.ExportSnapshot(12345)
	if err != nil {
		t.Fatalf("Failed to export snapshot: %v", err)
	}

	// Save to file
	err = SaveSnapshotToFile(snapshot, snapshotPath)
	if err != nil {
		t.Fatalf("Failed to save snapshot to file: %v", err)
	}

	// Load from file
	loaded, err := LoadSnapshotFromFile(snapshotPath)
	if err != nil {
		t.Fatalf("Failed to load snapshot from file: %v", err)
	}

	if loaded.Height != snapshot.Height {
		t.Errorf("Height mismatch: got %d, want %d", loaded.Height, snapshot.Height)
	}

	if loaded.UTXORoot != snapshot.UTXORoot {
		t.Errorf("Root mismatch: got %x, want %x", loaded.UTXORoot, snapshot.UTXORoot)
	}

	if len(loaded.UTXOs) != len(snapshot.UTXOs) {
		t.Errorf("UTXO count mismatch: got %d, want %d", len(loaded.UTXOs), len(snapshot.UTXOs))
	}
}

func TestSnapshot_RootVerification(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	store, err := NewTransparentStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Add UTXO
	txid := [32]byte{1}
	outpoint := OutPoint{TxID: txid, Index: 0}
	utxo := UTXO{Value: 1000, ScriptPubKey: []byte{0x01}, Height: 100}

	err = store.AddUTXO(outpoint, utxo)
	if err != nil {
		t.Fatalf("Failed to add UTXO: %v", err)
	}

	// Export snapshot
	snapshot, err := store.ExportSnapshot(100)
	if err != nil {
		t.Fatalf("Failed to export snapshot: %v", err)
	}

	// Corrupt the root
	snapshot.UTXORoot[0] ^= 0xFF

	// Create new store
	dbPath2 := filepath.Join(tempDir, "test2.db")
	store2, err := NewTransparentStore(dbPath2)
	if err != nil {
		t.Fatalf("Failed to create second store: %v", err)
	}
	defer store2.Close()

	// Import should fail due to root mismatch
	err = store2.ImportSnapshot(snapshot)
	if err == nil {
		t.Errorf("Expected import to fail with corrupted root, but it succeeded")
	}
}

func TestSnapshot_ClearUTXOs(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	store, err := NewTransparentStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Add UTXOs
	for i := uint32(0); i < 3; i++ {
		txid := [32]byte{}
		txid[0] = byte(i)

		outpoint := OutPoint{TxID: txid, Index: i}
		utxo := UTXO{Value: 1000, ScriptPubKey: []byte{byte(i)}, Height: 100}

		err = store.AddUTXO(outpoint, utxo)
		if err != nil {
			t.Fatalf("Failed to add UTXO %d: %v", i, err)
		}
	}

	// Verify 3 UTXOs
	size, err := store.GetUTXOSize()
	if err != nil {
		t.Fatalf("Failed to get UTXO size: %v", err)
	}
	if size != 3 {
		t.Errorf("UTXO size before clear: got %d, want 3", size)
	}

	// Clear all UTXOs
	err = store.clearAllUTXOs()
	if err != nil {
		t.Fatalf("Failed to clear UTXOs: %v", err)
	}

	// Verify 0 UTXOs
	size, err = store.GetUTXOSize()
	if err != nil {
		t.Fatalf("Failed to get UTXO size after clear: %v", err)
	}
	if size != 0 {
		t.Errorf("UTXO size after clear: got %d, want 0", size)
	}
}

func TestSnapshot_EmptySet(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	store, err := NewTransparentStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Export snapshot of empty set
	snapshot, err := store.ExportSnapshot(0)
	if err != nil {
		t.Fatalf("Failed to export empty snapshot: %v", err)
	}

	if len(snapshot.UTXOs) != 0 {
		t.Errorf("Empty snapshot should have 0 UTXOs, got %d", len(snapshot.UTXOs))
	}

	// Empty root should be all zeros
	emptyRoot := [32]byte{}
	if snapshot.UTXORoot != emptyRoot {
		t.Errorf("Empty snapshot root should be zeros, got %x", snapshot.UTXORoot)
	}
}
