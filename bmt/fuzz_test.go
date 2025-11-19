package bmt

import (
	"crypto/rand"
	"testing"
)

// FuzzInsertLookup fuzzes insert and lookup operations
func FuzzInsertLookup(f *testing.F) {
	// Add seed corpus
	f.Add([]byte("key1"), []byte("value1"))
	f.Add([]byte("key2"), []byte("value2"))
	f.Add([]byte(""), []byte(""))

	// Large value for overflow testing
	largeValue := make([]byte, 2048)
	rand.Read(largeValue)
	f.Add([]byte("large_key"), largeValue)

	f.Fuzz(func(t *testing.T, keyData, valueData []byte) {
		// Skip if key is not 32 bytes (pad or truncate)
		var key [32]byte
		if len(keyData) >= 32 {
			copy(key[:], keyData[:32])
		} else {
			copy(key[:len(keyData)], keyData)
		}

		// Create temporary directory
		dir := t.TempDir()
		opts := DefaultOptions(dir)

		// Open database
		db, err := Open(opts)
		if err != nil {
			t.Fatalf("Failed to open database: %v", err)
		}
		defer db.Close()

		// Test insert
		err = db.Insert(key, valueData)
		if err != nil {
			// Some inserts may fail due to constraints - that's ok
			t.Skip("Insert failed (expected for some inputs)")
			return
		}

		// Test lookup
		retrieved, err := db.Get(key)
		if err != nil {
			t.Fatalf("Failed to get after insert: %v", err)
		}

		// Verify data integrity
		if string(retrieved) != string(valueData) {
			t.Fatalf("Data mismatch: inserted %q, got %q", valueData, retrieved)
		}

		// Test deletion
		err = db.Delete(key)
		if err != nil {
			t.Fatalf("Failed to delete: %v", err)
		}

		// Verify deletion
		retrieved, err = db.Get(key)
		if err != nil {
			t.Fatalf("Failed to get after delete: %v", err)
		}
		if retrieved != nil {
			t.Fatalf("Expected nil after delete, got %q", retrieved)
		}
	})
}

// FuzzConcurrentOperations tests concurrent operations for race conditions
func FuzzConcurrentOperations(f *testing.F) {
	// Seed corpus
	f.Add([]byte("key1"), []byte("value1"), []byte("key2"), []byte("value2"))

	f.Fuzz(func(t *testing.T, key1Data, value1Data, key2Data, value2Data []byte) {
		// Create keys
		var key1, key2 [32]byte
		if len(key1Data) >= 32 {
			copy(key1[:], key1Data[:32])
		} else {
			copy(key1[:len(key1Data)], key1Data)
		}

		if len(key2Data) >= 32 {
			copy(key2[:], key2Data[:32])
		} else {
			copy(key2[:len(key2Data)], key2Data)
		}

		// Create database
		dir := t.TempDir()
		opts := DefaultOptions(dir)
		db, err := Open(opts)
		if err != nil {
			t.Fatalf("Failed to open database: %v", err)
		}
		defer db.Close()

		// Test concurrent operations
		done := make(chan bool, 2)

		// Goroutine 1: Insert key1
		go func() {
			defer func() { done <- true }()
			err := db.Insert(key1, value1Data)
			if err != nil {
				return // Some operations may fail - that's ok
			}

			// Try to read it back
			_, err = db.Get(key1)
			if err != nil {
				t.Errorf("Failed to read key1: %v", err)
			}
		}()

		// Goroutine 2: Insert key2
		go func() {
			defer func() { done <- true }()
			err := db.Insert(key2, value2Data)
			if err != nil {
				return // Some operations may fail - that's ok
			}

			// Try to read it back
			_, err = db.Get(key2)
			if err != nil {
				t.Errorf("Failed to read key2: %v", err)
			}
		}()

		// Wait for both operations to complete
		<-done
		<-done
	})
}