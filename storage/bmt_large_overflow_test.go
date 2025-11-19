package storage

import (
	"bytes"
	"fmt"
	"os"
	"testing"
)

// TestLargeOverflowChaining tests overflow values requiring >15 pages.
// This exercises the overflow page pointer chaining mechanism.
// With OVERFLOW_BODY_SIZE ~16KB per page, we need >240KB to exceed 15 pages.
func TestLargeOverflowChaining(t *testing.T) {
	fmt.Println("\n=== Testing Large Overflow Value Chaining (>15 pages) ===")

	tmpDir, err := os.MkdirTemp("", "bmt-large-overflow-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	fmt.Printf("Using directory: %s\n", tmpDir)

	// Create a VERY large value that will require >15 pages
	// Let's use 300KB to ensure we need more than 15 pages
	largeSize := 300 * 1024 // 300KB
	largeValue := make([]byte, largeSize)
	// Fill with pattern for verification
	for i := range largeValue {
		largeValue[i] = byte(i % 256)
	}

	fmt.Printf("Created large value: %d bytes (%.1f KB)\n", largeSize, float64(largeSize)/1024)

	// Also create some normal values for comparison
	testData := [][2][]byte{
		// Large overflow value (>15 pages)
		{hex2Bytes("aaaa0000000000000000000000000000000000000000000000000000000000aa"), largeValue},
		// Small inline value
		{hex2Bytes("bbbb0000000000000000000000000000000000000000000000000000000000bb"), hex2Bytes("deadbeef01")},
		// Medium overflow value (≤15 pages)
		{hex2Bytes("cccc0000000000000000000000000000000000000000000000000000000000cc"), make([]byte, 100*1024)}, // 100KB
	}

	// Fill the medium value with pattern
	for i := range testData[2][1] {
		testData[2][1][i] = byte((i + 128) % 256)
	}

	// ========== SESSION 1: Insert and flush ==========
	fmt.Println("\n--- SESSION 1: Insert large overflow value ---")

	storage1, err := NewStateDBStorage(tmpDir, nil, nil, 0)
	if err != nil {
		t.Fatalf("Session 1: Failed to create storage: %v", err)
	}

	for i, kv := range testData {
		fmt.Printf("Session 1 - Insert #%d: key=%x, value_len=%d bytes (%.1f KB)\n",
			i, kv[0][:8], len(kv[1]), float64(len(kv[1]))/1024)
		storage1.Insert(kv[0], kv[1])
	}

	// Flush to disk
	root1, err := storage1.Flush()
	if err != nil {
		t.Fatalf("Session 1: Flush failed: %v", err)
	}
	fmt.Printf("\nSession 1 committed root: %v\n", root1)

	// Verify values are readable in same session after flush
	fmt.Println("\n--- SESSION 1: Verify values after flush (same session) ---")
	for i, kv := range testData {
		val, exists, err := storage1.Get(kv[0])
		if err != nil {
			t.Fatalf("Session 1 (after flush): Get #%d failed: %v", i, err)
		}
		if !exists {
			t.Errorf("❌ Session 1: Value #%d not found after flush!", i)
		} else if !bytes.Equal(val, kv[1]) {
			t.Errorf("❌ Session 1: Value #%d mismatch after flush! Expected %d bytes, got %d bytes",
				i, len(kv[1]), len(val))
			// Check first few bytes for pattern
			if len(val) > 10 && len(kv[1]) > 10 {
				t.Errorf("   Expected first 10 bytes: %x", kv[1][:10])
				t.Errorf("   Got first 10 bytes:      %x", val[:10])
			}
		} else {
			fmt.Printf("✅ Session 1: Value #%d readable after flush (%d bytes)\n", i, len(val))
		}
	}

	storage1.Close()
	fmt.Println("\nSession 1 CLOSED")

	// ========== SESSION 2: Reopen and verify ==========
	fmt.Println("\n--- SESSION 2: Reopen from disk ---")

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
			t.Errorf("❌ Session 2: Value #%d not found after restart!", i)
		} else if !bytes.Equal(val, kv[1]) {
			t.Errorf("❌ Session 2: Value #%d mismatch after restart! Expected %d bytes, got %d bytes",
				i, len(kv[1]), len(val))
			// Check pattern
			if len(val) > 10 && len(kv[1]) > 10 {
				t.Errorf("   Expected first 10 bytes: %x", kv[1][:10])
				t.Errorf("   Got first 10 bytes:      %x", val[:10])
			}
			// Check last few bytes too
			if len(val) >= 10 && len(kv[1]) >= 10 {
				t.Errorf("   Expected last 10 bytes: %x", kv[1][len(kv[1])-10:])
				t.Errorf("   Got last 10 bytes:      %x", val[len(val)-10:])
			}
		} else {
			// Verify full content matches
			allMatch := true
			for j := range val {
				if val[j] != kv[1][j] {
					t.Errorf("❌ Session 2: Value #%d byte mismatch at offset %d: expected %d, got %d",
						i, j, kv[1][j], val[j])
					allMatch = false
					break
				}
			}
			if allMatch {
				fmt.Printf("✅ Session 2: Value #%d fully verified after restart (%d bytes, all bytes match)\n", i, len(val))
			}
		}
	}

	// Test OverlayRoot computation with large overflow values
	fmt.Println("\n--- SESSION 2: Test OverlayRoot with large overflow values ---")
	overlayRoot2, err := storage2.OverlayRoot()
	if err != nil {
		t.Fatalf("Session 2: OverlayRoot failed: %v", err)
	}
	fmt.Printf("Session 2 OverlayRoot: %v\n", overlayRoot2)

	if !bytes.Equal(overlayRoot2[:], root1[:]) {
		t.Errorf("❌ OverlayRoot != committed root!")
		t.Errorf("OverlayRoot:  %x", overlayRoot2)
		t.Errorf("Committed:    %x", root1)
	} else {
		fmt.Println("✅ Session 2: OverlayRoot matches committed root")
	}

	fmt.Println("\n✅ ALL LARGE OVERFLOW CHAINING TESTS PASSED!")
	fmt.Println("   - Verified 300KB value (>15 pages) can be written and read back")
	fmt.Println("   - Verified overflow page pointer chaining works correctly")
	fmt.Println("   - Verified data integrity across session boundaries")
	fmt.Println("   - Verified OverlayRoot computation with large values")
}
