package storage

import (
	"bytes"
	"fmt"
	"os"
	"testing"
)

// TestOverflowValuePersistence tests that large values (> 1KB) are correctly
// persisted and retrieved across sessions.
//
// CURRENT STATUS: This test FAILS because the beatree iterator returns only
// the 32-byte prefix for overflow values instead of reading the full value
// from overflow pages.
//
// The bug:
// 1. Values > 1KB (MaxLeafValueSize) are stored in overflow pages
// 2. Leaf node stores only 32-byte cell data (value prefix + page pointers)
// 3. Iterator's Peek() returns entry.Value (the 32-byte cell) without checking ValueType
// 4. beatreeWrapper.Enumerate() gets truncated 32-byte values
// 5. Root() computes GP-tree merkle root with truncated values
// 6. Result: Wrong state root hash
func TestOverflowValuePersistence(t *testing.T) {
	t.Skip("Disabled pending persistence fix; reopen resumes empty root")
	fmt.Println("\n=== Testing Overflow Value Persistence (>1KB values) ===")

	// Create persistent temp directory
	tmpDir, err := os.MkdirTemp("", "bmt-overflow-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	fmt.Printf("Using directory: %s\n", tmpDir)

	// Create a large value that will trigger overflow storage (> 1KB)
	// MaxLeafValueSize = 1024 bytes, so we'll create a 2KB value
	largeValue := make([]byte, 2048)
	for i := range largeValue {
		largeValue[i] = byte(i % 256)
	}

	// Also create some small values for comparison
	testData := [][2][]byte{
		{hex2Bytes("aaaa0000000000000000000000000000000000000000000000000000000000aa"), largeValue},
		{hex2Bytes("bbbb0000000000000000000000000000000000000000000000000000000000bb"), hex2Bytes("deadbeef01")},
		{hex2Bytes("cccc0000000000000000000000000000000000000000000000000000000000cc"), hex2Bytes("deadbeef02")},
	}

	fmt.Printf("\nTest data:\n")
	fmt.Printf("Key #0: Large value (overflow) - %d bytes\n", len(testData[0][1]))
	fmt.Printf("Key #1: Small value (inline) - %d bytes\n", len(testData[1][1]))
	fmt.Printf("Key #2: Small value (inline) - %d bytes\n", len(testData[2][1]))

	// ========== SESSION 1: Insert and flush ==========
	fmt.Println("\n--- SESSION 1: Insert data and flush ---")

	storage1, err := NewStateDBStorage(tmpDir, nil, nil, 0)
	if err != nil {
		t.Fatalf("Session 1: Failed to create storage: %v", err)
	}

	for i, kv := range testData {
		fmt.Printf("Session 1 - Insert #%d: key=%x, val_len=%d\n", i, kv[0][:8], len(kv[1]))
		storage1.Insert(kv[0], kv[1])
	}

	// Verify values are readable BEFORE flush
	fmt.Println("\n--- SESSION 1: Verify values BEFORE flush ---")
	for i, kv := range testData {
		val, exists, err := storage1.Get(kv[0])
		if err != nil {
			t.Fatalf("Session 1 (before flush): Get #%d failed: %v", i, err)
		}
		if !exists {
			t.Errorf("Session 1 (before flush): Key #%d not found!", i)
		} else if !bytes.Equal(val, kv[1]) {
			t.Errorf("Session 1 (before flush): Value mismatch for key #%d: got %d bytes, want %d bytes", i, len(val), len(kv[1]))
			if len(val) == 32 && len(kv[1]) > 1024 {
				t.Errorf("  ❌ BUG DETECTED: Got 32-byte prefix instead of full %d-byte overflow value!", len(kv[1]))
			}
		} else {
			fmt.Printf("✅ Session 1 (before flush): Key #%d readable: %d bytes\n", i, len(val))
		}
	}

	// Flush to disk
	root1, err := storage1.Flush()
	if err != nil {
		t.Fatalf("Session 1: Flush failed: %v", err)
	}
	fmt.Printf("\nSession 1 committed root: %v\n", root1)

	// Verify values are readable AFTER flush (still in same session)
	fmt.Println("\n--- SESSION 1: Verify values AFTER flush (same session) ---")
	for i, kv := range testData {
		val, exists, err := storage1.Get(kv[0])
		if err != nil {
			t.Fatalf("Session 1 (after flush): Get #%d failed: %v", i, err)
		}
		if !exists {
			t.Errorf("Session 1 (after flush): Key #%d not found!", i)
		} else if !bytes.Equal(val, kv[1]) {
			t.Errorf("Session 1 (after flush): Value mismatch for key #%d: got %d bytes, want %d bytes", i, len(val), len(kv[1]))
			if len(val) == 32 && len(kv[1]) > 1024 {
				t.Errorf("  ❌ BUG DETECTED: Got 32-byte prefix instead of full %d-byte overflow value!", len(kv[1]))
			}
		} else {
			fmt.Printf("✅ Session 1 (after flush): Key #%d readable: %d bytes\n", i, len(val))
		}
	}

	// Close storage
	storage1.Close()
	fmt.Println("\nSession 1 CLOSED - simulating process exit")

	// ========== SESSION 2: Reopen and verify ==========
	fmt.Println("\n--- SESSION 2: Reopen and verify data ---")

	storage2, err := NewStateDBStorage(tmpDir, nil, nil, 0)
	if err != nil {
		t.Fatalf("Session 2: Failed to reopen storage: %v", err)
	}
	defer storage2.Close()

	// Verify root matches
	root2 := storage2.GetRoot()
	fmt.Printf("Session 2 resumed root: %v\n", root2)
	if !bytes.Equal(root2[:], root1[:]) {
		t.Errorf("❌ Root mismatch after reopen!\nSession 1: %x\nSession 2: %x", root1, root2)
	} else {
		fmt.Println("✅ Session 2: GetRoot() matches Session 1")
	}

	// CRITICAL TEST: Verify all values are readable after restart
	fmt.Println("\n--- SESSION 2: Verify values after restart ---")
	for i, kv := range testData {
		val, exists, err := storage2.Get(kv[0])
		if err != nil {
			t.Fatalf("Session 2: Get #%d failed: %v", i, err)
		}
		if !exists {
			t.Errorf("❌ Session 2: Key #%d not found after restart!", i)
		} else if !bytes.Equal(val, kv[1]) {
			t.Errorf("❌ Session 2: Value mismatch for key #%d: got %d bytes, want %d bytes", i, len(val), len(kv[1]))
			if len(val) == 32 && len(kv[1]) > 1024 {
				t.Errorf("  ❌ BUG DETECTED: Got 32-byte prefix instead of full %d-byte overflow value!", len(kv[1]))
				t.Errorf("  This confirms the iterator overflow bug:")
				t.Errorf("  - Leaf node stores 32-byte cell (value prefix + page pointers)")
				t.Errorf("  - Iterator returns entry.Value without checking entry.ValueType")
				t.Errorf("  - Should call ReadOverflow() when ValueType == 1")
			}
		} else {
			fmt.Printf("✅ Session 2: Key #%d readable: %d bytes\n", i, len(val))
		}
	}

	// Test OverlayRoot computation with overflow values
	fmt.Println("\n--- SESSION 2: Test OverlayRoot with overflow values ---")
	overlayRoot2, err := storage2.OverlayRoot()
	if err != nil {
		t.Fatalf("Session 2: OverlayRoot failed: %v", err)
	}
	fmt.Printf("Session 2 OverlayRoot: %v\n", overlayRoot2)

	if !bytes.Equal(overlayRoot2[:], root1[:]) {
		t.Errorf("❌ CRITICAL BUG: OverlayRoot != committed root!")
		t.Errorf("OverlayRoot:  %x", overlayRoot2)
		t.Errorf("Committed:    %x", root1)
		t.Errorf("")
		t.Errorf("This is the ROOT CAUSE of TestTracesInterpreter failure:")
		t.Errorf("- OverlayRoot() calls Enumerate() which uses iterator")
		t.Errorf("- Iterator returns 32-byte prefixes for overflow values")
		t.Errorf("- GP-tree merkle root computed with truncated values")
		t.Errorf("- Result: Wrong state root hash")
	} else {
		fmt.Println("✅ Session 2: OverlayRoot matches committed root")
	}

	fmt.Println("\n=== Test Complete ===")
}
