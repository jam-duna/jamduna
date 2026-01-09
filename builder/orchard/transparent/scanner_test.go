package transparent

import (
	"fmt"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/blake2b"
)

func TestScanner_ScanBlock(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	store, err := NewTransparentStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	scanner := NewScanner(store)

	// Scan a test block
	blockData := []byte{0x01, 0x02, 0x03}
	txCount, err := scanner.ScanBlock(100, blockData)
	if err != nil {
		t.Fatalf("Failed to scan block: %v", err)
	}

	// Currently returns 0 since it's a placeholder
	if txCount != 0 {
		t.Errorf("Expected 0 transactions, got %d", txCount)
	}
}

func TestScanner_ScanBlockRange(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	store, err := NewTransparentStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	scanner := NewScanner(store)

	// Mock block getter
	blockCount := 0
	getBlock := func(height uint32) ([]byte, error) {
		blockCount++
		return []byte{byte(height)}, nil
	}

	// Scan range [0, 9]
	err = scanner.ScanBlockRange(0, 9, 5, getBlock)
	if err != nil {
		t.Fatalf("Failed to scan block range: %v", err)
	}

	// Should have fetched 10 blocks
	if blockCount != 10 {
		t.Errorf("Expected 10 blocks fetched, got %d", blockCount)
	}
}

func TestScanner_ProcessTransparentTransaction(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	store, err := NewTransparentStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	scanner := NewScanner(store)

	// Process test transaction
	txBytes := []byte{0xaa, 0xbb, 0xcc}
	err = scanner.ProcessTransparentTransaction(100, txBytes)
	if err != nil {
		t.Fatalf("Failed to process transaction: %v", err)
	}

	// Verify transaction was stored
	txid := blake2b.Sum256(txBytes)
	retrieved, err := store.GetTransaction(100, txid)
	if err != nil {
		t.Fatalf("Failed to retrieve transaction: %v", err)
	}

	if len(retrieved) != len(txBytes) {
		t.Errorf("Retrieved transaction length mismatch: got %d, want %d",
			len(retrieved), len(txBytes))
	}
}

func TestScanner_RebuildFromGenesis(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	store, err := NewTransparentStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Add some UTXOs first
	for i := uint32(0); i < 3; i++ {
		txid := [32]byte{}
		txid[0] = byte(i)
		outpoint := OutPoint{TxID: txid, Index: i}
		utxo := UTXO{Value: 1000, ScriptPubKey: []byte{byte(i)}, Height: 100}

		err = store.AddUTXO(outpoint, utxo)
		if err != nil {
			t.Fatalf("Failed to add UTXO: %v", err)
		}
	}

	// Verify we have UTXOs
	sizeBefore, err := store.GetUTXOSize()
	if err != nil {
		t.Fatalf("Failed to get UTXO size: %v", err)
	}
	if sizeBefore != 3 {
		t.Errorf("Expected 3 UTXOs before rebuild, got %d", sizeBefore)
	}

	scanner := NewScanner(store)

	// Mock block getter
	getBlock := func(height uint32) ([]byte, error) {
		return []byte{byte(height)}, nil
	}

	// Rebuild from genesis (should clear existing UTXOs)
	err = scanner.RebuildFromGenesis(5, getBlock)
	if err != nil {
		t.Fatalf("Failed to rebuild from genesis: %v", err)
	}

	// Verify UTXOs were cleared
	sizeAfter, err := store.GetUTXOSize()
	if err != nil {
		t.Fatalf("Failed to get UTXO size after rebuild: %v", err)
	}
	if sizeAfter != 0 {
		t.Errorf("Expected 0 UTXOs after rebuild, got %d", sizeAfter)
	}
}

func TestScanner_VerifyUTXORoot(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	store, err := NewTransparentStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	scanner := NewScanner(store)

	// Add UTXO
	txid := [32]byte{1}
	outpoint := OutPoint{TxID: txid, Index: 0}
	utxo := UTXO{Value: 1000, ScriptPubKey: []byte{0x01}, Height: 100}

	err = store.AddUTXO(outpoint, utxo)
	if err != nil {
		t.Fatalf("Failed to add UTXO: %v", err)
	}

	// Get actual root
	actualRoot, err := store.GetUTXORoot()
	if err != nil {
		t.Fatalf("Failed to get UTXO root: %v", err)
	}

	// Verify with correct root (should pass)
	err = scanner.VerifyUTXORoot(actualRoot)
	if err != nil {
		t.Errorf("VerifyUTXORoot failed with correct root: %v", err)
	}

	// Verify with incorrect root (should fail)
	wrongRoot := actualRoot
	wrongRoot[0] ^= 0xFF
	err = scanner.VerifyUTXORoot(wrongRoot)
	if err == nil {
		t.Errorf("VerifyUTXORoot should have failed with incorrect root")
	}
}

func TestScanner_Checkpointing(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	store, err := NewTransparentStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	scanner := NewScanner(store)

	// Add UTXO for checkpoint logging
	txid := [32]byte{1}
	outpoint := OutPoint{TxID: txid, Index: 0}
	utxo := UTXO{Value: 1000, ScriptPubKey: []byte{0x01}, Height: 100}

	err = store.AddUTXO(outpoint, utxo)
	if err != nil {
		t.Fatalf("Failed to add UTXO: %v", err)
	}

	// Log checkpoint (should not error)
	err = scanner.logCheckpoint(1000)
	if err != nil {
		t.Errorf("logCheckpoint failed: %v", err)
	}
}

func TestScanner_BlockRangeWithError(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	store, err := NewTransparentStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	scanner := NewScanner(store)

	// Mock block getter that fails at height 5
	getBlock := func(height uint32) ([]byte, error) {
		if height == 5 {
			return nil, fmt.Errorf("block not found")
		}
		return []byte{byte(height)}, nil
	}

	// Scan should fail at height 5
	err = scanner.ScanBlockRange(0, 10, 2, getBlock)
	if err == nil {
		t.Errorf("Expected scan to fail, but it succeeded")
	}
}
