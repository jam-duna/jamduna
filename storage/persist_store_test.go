package storage

import (
	"testing"

	"github.com/colorfulnotion/jam/common"
)

func TestPersistenceStore_BasicOperations(t *testing.T) {
	// Create in-memory store
	ps, err := NewMemoryPersistenceStore()
	if err != nil {
		t.Fatalf("Failed to create memory store: %v", err)
	}
	defer ps.Close()

	// Test Put and Get
	key := []byte("test-key")
	value := []byte("test-value")

	if err := ps.Put(key, value); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	got, found, err := ps.Get(key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if !found {
		t.Fatal("Expected key to be found")
	}
	if string(got) != string(value) {
		t.Errorf("Get returned %q, want %q", got, value)
	}

	// Test Get non-existent key
	_, found, err = ps.Get([]byte("non-existent"))
	if err != nil {
		t.Fatalf("Get non-existent failed: %v", err)
	}
	if found {
		t.Error("Expected key not to be found")
	}

	// Test Delete
	if err := ps.Delete(key); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, found, err = ps.Get(key)
	if err != nil {
		t.Fatalf("Get after delete failed: %v", err)
	}
	if found {
		t.Error("Expected key to be deleted")
	}
}

func TestPersistenceStore_HashOperations(t *testing.T) {
	ps, err := NewMemoryPersistenceStore()
	if err != nil {
		t.Fatalf("Failed to create memory store: %v", err)
	}
	defer ps.Close()

	// Test PutHash and GetHash
	key := common.HexToHash("0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef")
	value := []byte("hash-value")

	if err := ps.PutHash(key, value); err != nil {
		t.Fatalf("PutHash failed: %v", err)
	}

	got, err := ps.GetHash(key)
	if err != nil {
		t.Fatalf("GetHash failed: %v", err)
	}
	if string(got) != string(value) {
		t.Errorf("GetHash returned %q, want %q", got, value)
	}

	// Test DeleteHash
	if err := ps.DeleteHash(key); err != nil {
		t.Fatalf("DeleteHash failed: %v", err)
	}

	_, err = ps.GetHash(key)
	if err == nil {
		t.Error("Expected error for deleted key")
	}
}

func TestPersistenceStore_GetWithPrefix(t *testing.T) {
	ps, err := NewMemoryPersistenceStore()
	if err != nil {
		t.Fatalf("Failed to create memory store: %v", err)
	}
	defer ps.Close()

	// Store some keys with a common prefix
	prefix := []byte("prefix_")
	keys := [][]byte{
		[]byte("prefix_a"),
		[]byte("prefix_b"),
		[]byte("prefix_c"),
		[]byte("other_key"),
	}

	for _, key := range keys {
		if err := ps.Put(key, []byte("value-"+string(key))); err != nil {
			t.Fatalf("Put failed: %v", err)
		}
	}

	// Get with prefix
	results, err := ps.GetWithPrefix(prefix)
	if err != nil {
		t.Fatalf("GetWithPrefix failed: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("Expected 3 results, got %d", len(results))
	}

	// Verify all returned keys have the prefix
	for _, kv := range results {
		key := kv[0]
		if len(key) < len(prefix) {
			t.Errorf("Key %q is shorter than prefix", key)
			continue
		}
		for i := 0; i < len(prefix); i++ {
			if key[i] != prefix[i] {
				t.Errorf("Key %q does not have prefix %q", key, prefix)
				break
			}
		}
	}
}

func TestPersistenceStore_DB(t *testing.T) {
	ps, err := NewMemoryPersistenceStore()
	if err != nil {
		t.Fatalf("Failed to create memory store: %v", err)
	}
	defer ps.Close()

	// Verify DB() returns non-nil
	db := ps.DB()
	if db == nil {
		t.Error("DB() returned nil")
	}
}
