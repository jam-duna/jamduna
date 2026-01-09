package rpc

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPersistentTxIndex(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "transparent_tx_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create store
	store, err := NewTransparentTxStore(tempDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Test 1: Simple test of index persistence without full transaction parsing
	// We'll test the core functionality directly
	testTxid := "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
	blockHeight := uint32(12345)

	// Add directly to index (simulating a confirmed transaction)
	err = store.persistTxIndex(testTxid, blockHeight)
	if err != nil {
		t.Fatalf("Failed to persist tx index: %v", err)
	}
	store.txIndex[testTxid] = blockHeight

	// Test 2: Verify transaction is indexed at correct height
	height, exists := store.GetTransactionBlockHeight(testTxid)
	if !exists {
		t.Fatal("Transaction not found after confirmation")
	}
	if height != blockHeight {
		t.Fatalf("Expected height %d, got %d", blockHeight, height)
	}

	// Test 3: Close and reopen store to test persistence
	store.Close()

	store2, err := NewTransparentTxStore(tempDir)
	if err != nil {
		t.Fatalf("Failed to reopen store: %v", err)
	}
	defer store2.Close()

	// Verify transaction is still indexed after restart
	height, exists = store2.GetTransactionBlockHeight(testTxid)
	if !exists {
		t.Fatal("Transaction not found after store restart")
	}
	if height != blockHeight {
		t.Fatalf("Expected height %d after restart, got %d", blockHeight, height)
	}

	// Test 4: Check index stats
	stats, err := store2.GetIndexStats()
	if err != nil {
		t.Fatalf("Failed to get index stats: %v", err)
	}

	memSize, ok := stats["memoryIndexSize"].(int)
	if !ok || memSize != 1 {
		t.Fatalf("Expected memory index size 1, got %v", memSize)
	}

	t.Logf("Index stats: %+v", stats)
}

func TestIndexPerformance(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "transparent_tx_perf_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store, err := NewTransparentTxStore(tempDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Benchmark adding multiple transactions to index
	numTxs := 1000
	start := time.Now()

	for i := 0; i < numTxs; i++ {
		txid := "test_txid_" + string(rune(i))
		height := uint32(i + 1)

		err := store.persistTxIndex(txid, height)
		if err != nil {
			t.Fatalf("Failed to persist tx %d: %v", i, err)
		}

		store.txIndex[txid] = height
	}

	indexTime := time.Since(start)
	t.Logf("Indexed %d transactions in %v (%.2f tx/sec)",
		numTxs, indexTime, float64(numTxs)/indexTime.Seconds())

	// Benchmark lookups
	start = time.Now()
	for i := 0; i < numTxs; i++ {
		txid := "test_txid_" + string(rune(i))
		height, exists := store.GetTransactionBlockHeight(txid)
		if !exists {
			t.Fatalf("Transaction %s not found", txid)
		}
		if height != uint32(i+1) {
			t.Fatalf("Wrong height for tx %s: expected %d, got %d", txid, i+1, height)
		}
	}
	lookupTime := time.Since(start)
	t.Logf("Looked up %d transactions in %v (%.2f lookups/sec)",
		numTxs, lookupTime, float64(numTxs)/lookupTime.Seconds())

	// Verify database file exists
	indexPath := filepath.Join(tempDir, "transparent_index")
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		t.Fatal("Index database directory does not exist")
	}

	stats, err := store.GetIndexStats()
	if err != nil {
		t.Fatalf("Failed to get stats: %v", err)
	}
	t.Logf("Final index stats: %+v", stats)
}