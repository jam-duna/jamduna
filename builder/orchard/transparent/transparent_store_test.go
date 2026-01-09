package transparent

import (
	
	"path/filepath"
	"testing"

	"golang.org/x/crypto/blake2b"
)

func TestTransparentStore_BasicOperations(t *testing.T) {
	// Create temporary directory
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	// Open store
	store, err := NewTransparentStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Create test outpoint
	txid := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
		17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	outpoint := OutPoint{TxID: txid, Index: 0}

	// Create test UTXO
	utxo := UTXO{
		Value:        1000000,
		ScriptPubKey: []byte{0x76, 0xa9, 0x14}, // P2PKH prefix
		Height:       12345,
	}

	// Add UTXO
	err = store.AddUTXO(outpoint, utxo)
	if err != nil {
		t.Fatalf("Failed to add UTXO: %v", err)
	}

	// Retrieve UTXO
	retrieved, err := store.GetUTXO(outpoint)
	if err != nil {
		t.Fatalf("Failed to get UTXO: %v", err)
	}

	if retrieved.Value != utxo.Value {
		t.Errorf("Value mismatch: got %d, want %d", retrieved.Value, utxo.Value)
	}
	if retrieved.Height != utxo.Height {
		t.Errorf("Height mismatch: got %d, want %d", retrieved.Height, utxo.Height)
	}

	// Check UTXO size
	size, err := store.GetUTXOSize()
	if err != nil {
		t.Fatalf("Failed to get UTXO size: %v", err)
	}
	if size != 1 {
		t.Errorf("UTXO size mismatch: got %d, want 1", size)
	}

	// Spend UTXO
	err = store.SpendUTXO(outpoint)
	if err != nil {
		t.Fatalf("Failed to spend UTXO: %v", err)
	}

	// Verify UTXO is gone
	size, err = store.GetUTXOSize()
	if err != nil {
		t.Fatalf("Failed to get UTXO size: %v", err)
	}
	if size != 0 {
		t.Errorf("UTXO size after spend: got %d, want 0", size)
	}
}

func TestTransparentStore_UTXORoot(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	store, err := NewTransparentStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Empty root
	emptyRoot, err := store.GetUTXORoot()
	if err != nil {
		t.Fatalf("Failed to get empty root: %v", err)
	}
	if emptyRoot != [32]byte{} {
		t.Errorf("Empty root should be all zeros")
	}

	// Add UTXOs
	for i := uint32(0); i < 3; i++ {
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

	// Get root with 3 UTXOs
	root1, err := store.GetUTXORoot()
	if err != nil {
		t.Fatalf("Failed to get root: %v", err)
	}

	// Root should be deterministic
	root2, err := store.GetUTXORoot()
	if err != nil {
		t.Fatalf("Failed to get root second time: %v", err)
	}

	if root1 != root2 {
		t.Errorf("Root not deterministic: %x != %x", root1, root2)
	}

	// Root should change after adding UTXO
	txid := [32]byte{99}
	outpoint := OutPoint{TxID: txid, Index: 99}
	utxo := UTXO{Value: 5000, ScriptPubKey: []byte{99}, Height: 200}
	err = store.AddUTXO(outpoint, utxo)
	if err != nil {
		t.Fatalf("Failed to add fourth UTXO: %v", err)
	}

	root3, err := store.GetUTXORoot()
	if err != nil {
		t.Fatalf("Failed to get root after add: %v", err)
	}

	if root1 == root3 {
		t.Errorf("Root should change after adding UTXO")
	}
}

func TestTransparentStore_Transactions(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	store, err := NewTransparentStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Create test transaction
	txData := []byte{0x01, 0x02, 0x03, 0x04, 0x05}
	txid := blake2b.Sum256(txData)
	height := uint32(12345)

	// Store transaction
	err = store.AddTransaction(height, txData, txid)
	if err != nil {
		t.Fatalf("Failed to add transaction: %v", err)
	}

	// Retrieve transaction
	retrieved, err := store.GetTransaction(height, txid)
	if err != nil {
		t.Fatalf("Failed to get transaction: %v", err)
	}

	if len(retrieved) != len(txData) {
		t.Fatalf("Transaction length mismatch: got %d, want %d", len(retrieved), len(txData))
	}

	for i := range txData {
		if retrieved[i] != txData[i] {
			t.Errorf("Transaction data mismatch at byte %d: got %x, want %x", i, retrieved[i], txData[i])
		}
	}
}

func TestTransparentStore_Persistence(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	// Create store and add UTXO
	{
		store, err := NewTransparentStore(dbPath)
		if err != nil {
			t.Fatalf("Failed to create store: %v", err)
		}

		txid := [32]byte{1, 2, 3}
		outpoint := OutPoint{TxID: txid, Index: 0}
		utxo := UTXO{Value: 5000, ScriptPubKey: []byte{0xaa}, Height: 100}

		err = store.AddUTXO(outpoint, utxo)
		if err != nil {
			t.Fatalf("Failed to add UTXO: %v", err)
		}

		store.Close()
	}

	// Reopen store and verify UTXO persisted
	{
		store, err := NewTransparentStore(dbPath)
		if err != nil {
			t.Fatalf("Failed to reopen store: %v", err)
		}
		defer store.Close()

		size, err := store.GetUTXOSize()
		if err != nil {
			t.Fatalf("Failed to get UTXO size: %v", err)
		}

		if size != 1 {
			t.Errorf("UTXO not persisted: size = %d, want 1", size)
		}

		txid := [32]byte{1, 2, 3}
		outpoint := OutPoint{TxID: txid, Index: 0}
		retrieved, err := store.GetUTXO(outpoint)
		if err != nil {
			t.Fatalf("Failed to retrieve UTXO: %v", err)
		}

		if retrieved.Value != 5000 {
			t.Errorf("UTXO value mismatch: got %d, want 5000", retrieved.Value)
		}
	}
}

func TestTransparentStore_GetAllUTXOs(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	store, err := NewTransparentStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Add multiple UTXOs
	expected := make(map[OutPoint]UTXO)
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

		expected[outpoint] = utxo
	}

	// Get all UTXOs
	allUTXOs, err := store.GetAllUTXOs()
	if err != nil {
		t.Fatalf("Failed to get all UTXOs: %v", err)
	}

	if len(allUTXOs) != len(expected) {
		t.Errorf("UTXO count mismatch: got %d, want %d", len(allUTXOs), len(expected))
	}

	for op, expectedUTXO := range expected {
		actualUTXO, exists := allUTXOs[op]
		if !exists {
			t.Errorf("Missing UTXO for outpoint %x:%d", op.TxID, op.Index)
			continue
		}

		if actualUTXO.Value != expectedUTXO.Value {
			t.Errorf("Value mismatch for %x:%d: got %d, want %d",
				op.TxID, op.Index, actualUTXO.Value, expectedUTXO.Value)
		}
	}
}

func TestComputeMerkleRoot_EdgeCases(t *testing.T) {
	tests := []struct {
		name   string
		leaves [][32]byte
	}{
		{
			name:   "empty",
			leaves: [][32]byte{},
		},
		{
			name:   "single",
			leaves: [][32]byte{{1}},
		},
		{
			name: "two",
			leaves: [][32]byte{
				{1}, {2},
			},
		},
		{
			name: "three (odd)",
			leaves: [][32]byte{
				{1}, {2}, {3},
			},
		},
		{
			name: "four (power of 2)",
			leaves: [][32]byte{
				{1}, {2}, {3}, {4},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic
			root := computeMerkleRoot(tt.leaves)

			// Result should be deterministic
			root2 := computeMerkleRoot(tt.leaves)
			if root != root2 {
				t.Errorf("Non-deterministic root")
			}
		})
	}
}
