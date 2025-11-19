package storage

import (
	"bytes"
	"fmt"
	"os"
	"testing"
)

// TestDebugBeatreeReadback tests that values are correctly written and read back
func TestDebugBeatreeReadback(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "bmt-debug-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	storage, err := NewStateDBStorage(tmpDir, nil, nil, 0)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	// Test data with varying value sizes
	testData := []struct {
		key      []byte
		value    []byte
		valueLen int
	}{
		{hex2Bytes("0036003100ff00e6d331de80965ed035b36138fb759148cc0cfe8d67f97df6"), make([]byte, 5), 5},
		{hex2Bytes("004400b500a400aaa0d7f8eab5ca1cca4a0472988422febc68a6b6a944cabf"), make([]byte, 140), 140},
		{hex2Bytes("00450019006a0051135ef0225518c5ddc999419ffbf52cf2d4213d55af782e"), make([]byte, 500), 500},
	}

	// Fill test values with distinct patterns
	for i := range testData {
		for j := 0; j < len(testData[i].value); j++ {
			testData[i].value[j] = byte((i*100 + j) % 256)
		}
	}

	fmt.Println("\n=== BEFORE FLUSH ===")
	for i, td := range testData {
		fmt.Printf("Inserting key #%d: %x (31 bytes), value len=%d\n", i, td.key, len(td.value))
		storage.Insert(td.key, td.value)

		// Verify immediately after insert (from overlay)
		val, exists, err := storage.Get(td.key)
		if err != nil {
			t.Fatalf("Get failed after insert #%d: %v", i, err)
		}
		if !exists {
			t.Errorf("Key #%d not found immediately after insert!", i)
		} else if !bytes.Equal(val, td.value) {
			t.Errorf("Value mismatch after insert #%d: got %d bytes, want %d bytes", i, len(val), len(td.value))
			fmt.Printf("  Expected first 10 bytes: %x\n", td.value[:min(10, len(td.value))])
			fmt.Printf("  Got first 10 bytes:      %x\n", val[:min(10, len(val))])
		} else {
			fmt.Printf("  ✅ Key #%d readable from overlay: %d bytes\n", i, len(val))
		}
	}

	// Flush to disk
	fmt.Println("\n=== FLUSHING ===")
	root, err := storage.Flush()
	if err != nil {
		t.Fatalf("Flush failed: %v", err)
	}
	fmt.Printf("Committed root: %v\n", root)

	// Verify after flush (should read from disk via iterator)
	fmt.Println("\n=== AFTER FLUSH ===")
	for i, td := range testData {
		val, exists, err := storage.Get(td.key)
		if err != nil {
			t.Fatalf("Get failed after flush #%d: %v", i, err)
		}
		if !exists {
			t.Errorf("❌ Key #%d not found after flush!", i)
		} else if !bytes.Equal(val, td.value) {
			t.Errorf("❌ Value mismatch after flush #%d: got %d bytes, want %d bytes", i, len(val), len(td.value))
			fmt.Printf("  Expected first 32 bytes: %x\n", td.value[:min(32, len(td.value))])
			fmt.Printf("  Got first 32 bytes:      %x\n", val[:min(32, len(val))])

			// Check if it's exactly 32 bytes (the smoking gun!)
			if len(val) == 32 && len(td.value) > 32 {
				t.Errorf("  🔥 SMOKING GUN: Got exactly 32 bytes for a %d-byte value!", len(td.value))
				t.Errorf("  This confirms the iterator is returning the cached prefix instead of the full value!")
			}
		} else {
			fmt.Printf("  ✅ Key #%d readable after flush: %d bytes\n", i, len(val))
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
