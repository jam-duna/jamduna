package shielded

import (
	"os"
	"path/filepath"
	"testing"
)

// TestAddCommitmentAndRead tests adding commitments and reading them back.
func TestAddCommitmentAndRead(t *testing.T) {
	// Create temporary directory for test database
	tmpDir, err := os.MkdirTemp("", "shielded_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := NewShieldedStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Add commitment at position 0
	cm1 := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
		17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	if err := store.AddCommitment(0, cm1); err != nil {
		t.Fatalf("Failed to add commitment: %v", err)
	}

	// Add commitment at position 1
	cm2 := [32]byte{32, 31, 30, 29, 28, 27, 26, 25, 24, 23, 22, 21, 20, 19, 18, 17,
		16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	if err := store.AddCommitment(1, cm2); err != nil {
		t.Fatalf("Failed to add commitment: %v", err)
	}

	// List all commitments
	entries, err := store.ListCommitments()
	if err != nil {
		t.Fatalf("Failed to list commitments: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("Expected 2 commitments, got %d", len(entries))
	}

	// Verify first commitment
	if entries[0].Position != 0 {
		t.Errorf("Expected position 0, got %d", entries[0].Position)
	}
	if entries[0].Commitment != cm1 {
		t.Errorf("Commitment at position 0 mismatch")
	}

	// Verify second commitment
	if entries[1].Position != 1 {
		t.Errorf("Expected position 1, got %d", entries[1].Position)
	}
	if entries[1].Commitment != cm2 {
		t.Errorf("Commitment at position 1 mismatch")
	}
}

// TestAddNullifierAndCheck tests adding nullifiers and checking spent status.
func TestAddNullifierAndCheck(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "shielded_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := NewShieldedStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	nf1 := [32]byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x11, 0x22,
		0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0x00,
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}

	// Check nullifier is initially unspent
	spent, err := store.IsNullifierSpent(nf1)
	if err != nil {
		t.Fatalf("Failed to check nullifier: %v", err)
	}
	if spent {
		t.Errorf("Nullifier should not be spent initially")
	}

	// Add nullifier at height 100
	if err := store.AddNullifier(nf1, 100); err != nil {
		t.Fatalf("Failed to add nullifier: %v", err)
	}

	// Check nullifier is now spent
	spent, err = store.IsNullifierSpent(nf1)
	if err != nil {
		t.Fatalf("Failed to check nullifier after add: %v", err)
	}
	if !spent {
		t.Errorf("Nullifier should be spent after adding")
	}

	// List all nullifiers
	nullifiers, err := store.ListNullifiers()
	if err != nil {
		t.Fatalf("Failed to list nullifiers: %v", err)
	}

	if len(nullifiers) != 1 {
		t.Fatalf("Expected 1 nullifier, got %d", len(nullifiers))
	}

	if nullifiers[0] != nf1 {
		t.Errorf("Nullifier mismatch")
	}
}

// TestDeleteCommitment tests commitment deletion for rollback.
func TestDeleteCommitment(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "shielded_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := NewShieldedStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	cm := [32]byte{0xff, 0xee, 0xdd, 0xcc, 0xbb, 0xaa, 0x99, 0x88,
		0x77, 0x66, 0x55, 0x44, 0x33, 0x22, 0x11, 0x00,
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}

	// Add commitment
	if err := store.AddCommitment(42, cm); err != nil {
		t.Fatalf("Failed to add commitment: %v", err)
	}

	// Verify it exists
	entries, err := store.ListCommitments()
	if err != nil {
		t.Fatalf("Failed to list commitments: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("Expected 1 commitment, got %d", len(entries))
	}

	// Delete the commitment
	if err := store.DeleteCommitment(42); err != nil {
		t.Fatalf("Failed to delete commitment: %v", err)
	}

	// Verify it's gone
	entries, err = store.ListCommitments()
	if err != nil {
		t.Fatalf("Failed to list commitments after delete: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("Expected 0 commitments after delete, got %d", len(entries))
	}
}

// TestDeleteNullifier tests nullifier deletion for rollback.
func TestDeleteNullifier(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "shielded_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := NewShieldedStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	nf := [32]byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88,
		0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00,
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}

	// Add nullifier
	if err := store.AddNullifier(nf, 200); err != nil {
		t.Fatalf("Failed to add nullifier: %v", err)
	}

	// Verify it's spent
	spent, err := store.IsNullifierSpent(nf)
	if err != nil {
		t.Fatalf("Failed to check nullifier: %v", err)
	}
	if !spent {
		t.Errorf("Nullifier should be spent")
	}

	// Delete the nullifier
	if err := store.DeleteNullifier(nf); err != nil {
		t.Fatalf("Failed to delete nullifier: %v", err)
	}

	// Verify it's unspent now
	spent, err = store.IsNullifierSpent(nf)
	if err != nil {
		t.Fatalf("Failed to check nullifier after delete: %v", err)
	}
	if spent {
		t.Errorf("Nullifier should be unspent after deletion")
	}
}

// TestCommitmentOrdering tests that commitments are returned in position order.
func TestCommitmentOrdering(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "shielded_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := NewShieldedStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Add commitments in non-sequential order
	cm100 := [32]byte{100}
	cm5 := [32]byte{5}
	cm50 := [32]byte{50}
	cm1 := [32]byte{1}

	if err := store.AddCommitment(100, cm100); err != nil {
		t.Fatalf("Failed to add commitment 100: %v", err)
	}
	if err := store.AddCommitment(5, cm5); err != nil {
		t.Fatalf("Failed to add commitment 5: %v", err)
	}
	if err := store.AddCommitment(50, cm50); err != nil {
		t.Fatalf("Failed to add commitment 50: %v", err)
	}
	if err := store.AddCommitment(1, cm1); err != nil {
		t.Fatalf("Failed to add commitment 1: %v", err)
	}

	// List commitments - should be sorted by position
	entries, err := store.ListCommitments()
	if err != nil {
		t.Fatalf("Failed to list commitments: %v", err)
	}

	if len(entries) != 4 {
		t.Fatalf("Expected 4 commitments, got %d", len(entries))
	}

	// Verify they're in ascending order
	if entries[0].Position != 1 || entries[0].Commitment != cm1 {
		t.Errorf("First entry should be position 1")
	}
	if entries[1].Position != 5 || entries[1].Commitment != cm5 {
		t.Errorf("Second entry should be position 5")
	}
	if entries[2].Position != 50 || entries[2].Commitment != cm50 {
		t.Errorf("Third entry should be position 50")
	}
	if entries[3].Position != 100 || entries[3].Commitment != cm100 {
		t.Errorf("Fourth entry should be position 100")
	}
}

// TestMultipleNullifiers tests handling multiple nullifiers.
func TestMultipleNullifiers(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "shielded_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := NewShieldedStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Add multiple nullifiers
	nf1 := [32]byte{1}
	nf2 := [32]byte{2}
	nf3 := [32]byte{3}

	if err := store.AddNullifier(nf1, 100); err != nil {
		t.Fatalf("Failed to add nullifier 1: %v", err)
	}
	if err := store.AddNullifier(nf2, 101); err != nil {
		t.Fatalf("Failed to add nullifier 2: %v", err)
	}
	if err := store.AddNullifier(nf3, 102); err != nil {
		t.Fatalf("Failed to add nullifier 3: %v", err)
	}

	// Verify all are spent
	for i, nf := range [][32]byte{nf1, nf2, nf3} {
		spent, err := store.IsNullifierSpent(nf)
		if err != nil {
			t.Fatalf("Failed to check nullifier %d: %v", i+1, err)
		}
		if !spent {
			t.Errorf("Nullifier %d should be spent", i+1)
		}
	}

	// List all nullifiers
	nullifiers, err := store.ListNullifiers()
	if err != nil {
		t.Fatalf("Failed to list nullifiers: %v", err)
	}

	if len(nullifiers) != 3 {
		t.Fatalf("Expected 3 nullifiers, got %d", len(nullifiers))
	}
}

// TestPersistence tests that data persists across store close/reopen.
func TestPersistence(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "shielded_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")

	cm := [32]byte{0xde, 0xad, 0xbe, 0xef}
	nf := [32]byte{0xca, 0xfe, 0xba, 0xbe}

	// First session: add data
	{
		store, err := NewShieldedStore(dbPath)
		if err != nil {
			t.Fatalf("Failed to create store: %v", err)
		}

		if err := store.AddCommitment(123, cm); err != nil {
			t.Fatalf("Failed to add commitment: %v", err)
		}
		if err := store.AddNullifier(nf, 456); err != nil {
			t.Fatalf("Failed to add nullifier: %v", err)
		}

		if err := store.Close(); err != nil {
			t.Fatalf("Failed to close store: %v", err)
		}
	}

	// Second session: verify data persisted
	{
		store, err := NewShieldedStore(dbPath)
		if err != nil {
			t.Fatalf("Failed to reopen store: %v", err)
		}
		defer store.Close()

		// Check commitment persisted
		entries, err := store.ListCommitments()
		if err != nil {
			t.Fatalf("Failed to list commitments: %v", err)
		}
		if len(entries) != 1 {
			t.Fatalf("Expected 1 commitment, got %d", len(entries))
		}
		if entries[0].Position != 123 || entries[0].Commitment != cm {
			t.Errorf("Commitment did not persist correctly")
		}

		// Check nullifier persisted
		spent, err := store.IsNullifierSpent(nf)
		if err != nil {
			t.Fatalf("Failed to check nullifier: %v", err)
		}
		if !spent {
			t.Errorf("Nullifier should have persisted as spent")
		}
	}
}
