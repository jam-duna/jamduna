package storage

import (
	"bytes"
	"fmt"
	"testing"
)

// TestOverlayRootAfterCommit tests that OverlayRoot() works correctly AFTER commits.
// This is the critical test that verifies we properly merge committed tree + overlay.
func TestOverlayRootAfterCommit(t *testing.T) {
	fmt.Println("\n=== Testing OverlayRoot After Commit ===")

	// Initial data - 5 keys
	initialData := [][2][]byte{
		{hex2Bytes("f2a9fcaf8ae0ff770b0908ebdee1daf8457c0ef5e1106c89ad364236333c5fb3"), hex2Bytes("22c62f84ee5775d1e75ba6519f6dfae571eb1888768f2a203281579656b6a29097f7c7e2cf44e38da9a541d9b4c773db8b71e1d3")},
		{hex2Bytes("f3a9fcaf8ae0ff770b0908ebdee1daf8457c0ef5e1106c89ad364236333c5fb3"), hex2Bytes("965ac2547cacec18429e88553142a605d649fbcd6a40a0ae5d51a8f218c8fd5c")},
		{hex2Bytes("d7f99b746f23411983df92806725af8e5cb66eba9f200737accae4a1ab7f47b9"), hex2Bytes("965ac2547cacec18429e88553142a605d649fbcd6a40a0ae5d51a8f218c8fd5c")},
		{hex2Bytes("59ee947b94bcc05634d95efb474742f6cd6531766e44670ec987270a6b5a4211"), hex2Bytes("72fdb0c99cf47feb85b2dad01ee163139ee6d34a8d893029a200aff76f4be5930b9000a1bbb2dc2b6c79f8f3c19906c94a3472349817af21181c3eef6b")},
		{hex2Bytes("a3dc3bed1b0727caf428961bed11c9998ae2476d8a97fad203171b628363d9a2"), hex2Bytes("3f26db92922e86f6b538372608656a14762b3e93bd5d4f6a754d36f68ce0b28b")},
	}

	tree := setupStorage()
	defer tree.Close()

	// Step 1: Insert initial data and flush
	fmt.Println("\n--- Step 1: Insert 5 keys and flush ---")
	for i, kv := range initialData {
		fmt.Printf("Insert #%d: key=%x\n", i, kv[0])
		tree.Insert(kv[0], kv[1])
	}

	rootHash1, err := tree.Flush()
	if err != nil {
		t.Fatalf("Initial flush failed: %v", err)
	}
	fmt.Printf("Committed root after step 1: %v\n", rootHash1)

	// Step 2: Test OverlayRoot AFTER commit (should equal committed root)
	fmt.Println("\n--- Step 2: OverlayRoot after commit (no overlay changes) ---")
	previewRoot1, err := tree.OverlayRoot()
	if err != nil {
		t.Fatalf("OverlayRoot failed after commit: %v", err)
	}
	fmt.Printf("OverlayRoot after commit: %v\n", previewRoot1)

	if !bytes.Equal(previewRoot1[:], rootHash1[:]) {
		t.Errorf("❌ OverlayRoot != committed root!\nPreview:   %x\nCommitted: %x", previewRoot1, rootHash1)
	} else {
		fmt.Println("✅ OverlayRoot matches committed root (after commit, no overlay)")
	}

	// Step 3: Update one existing key in overlay (don't flush yet)
	fmt.Println("\n--- Step 3: Update existing key in overlay ---")
	updateKey := hex2Bytes("f3a9fcaf8ae0ff770b0908ebdee1daf8457c0ef5e1106c89ad364236333c5fb3")
	newValue := hex2Bytes("deadbeefcafebabe0123456789abcdef0123456789abcdef0123456789abcdef")
	fmt.Printf("Updating key: %x\nNew value: %x\n", updateKey, newValue)
	tree.Insert(updateKey, newValue)

	// Step 4: OverlayRoot should reflect the overlay update WITHOUT committing
	fmt.Println("\n--- Step 4: OverlayRoot with overlay update (before flush) ---")
	previewRoot2, err := tree.OverlayRoot()
	if err != nil {
		t.Fatalf("OverlayRoot failed with overlay: %v", err)
	}
	fmt.Printf("OverlayRoot with overlay update: %v\n", previewRoot2)

	// Preview root should NOT match the old committed root (we have changes)
	if bytes.Equal(previewRoot2[:], rootHash1[:]) {
		t.Errorf("❌ OverlayRoot unchanged after update! Should differ from committed root.")
	} else {
		fmt.Println("✅ OverlayRoot differs from committed root (overlay has changes)")
	}

	// Step 5: Flush the update and verify OverlayRoot matched Flush result
	fmt.Println("\n--- Step 5: Flush overlay update ---")
	rootHash2, err := tree.Flush()
	if err != nil {
		t.Fatalf("Second flush failed: %v", err)
	}
	fmt.Printf("Committed root after update: %v\n", rootHash2)

	if !bytes.Equal(previewRoot2[:], rootHash2[:]) {
		t.Errorf("❌ OverlayRoot != Flush root!\nPreview: %x\nFlushed: %x", previewRoot2, rootHash2)
	} else {
		fmt.Println("✅ OverlayRoot matches Flush root (preview was accurate)")
	}

	// Step 6: Add a NEW key in overlay (after commit)
	fmt.Println("\n--- Step 6: Add new key in overlay ---")
	newKey := hex2Bytes("0100000000000000000000000000000000000000000000000000000000000300")
	newKeyValue := hex2Bytes("cafebabe")
	fmt.Printf("Adding new key: %x\nValue: %x\n", newKey, newKeyValue)
	tree.Insert(newKey, newKeyValue)

	// Step 7: OverlayRoot should include the new key
	fmt.Println("\n--- Step 7: OverlayRoot with new key (before flush) ---")
	previewRoot3, err := tree.OverlayRoot()
	if err != nil {
		t.Fatalf("OverlayRoot failed with new key: %v", err)
	}
	fmt.Printf("OverlayRoot with new key: %v\n", previewRoot3)

	// Should differ from previous committed root
	if bytes.Equal(previewRoot3[:], rootHash2[:]) {
		t.Errorf("❌ OverlayRoot unchanged after adding new key!")
	} else {
		fmt.Println("✅ OverlayRoot differs (new key in overlay)")
	}

	// Step 8: Flush and verify
	fmt.Println("\n--- Step 8: Final flush ---")
	rootHash3, err := tree.Flush()
	if err != nil {
		t.Fatalf("Final flush failed: %v", err)
	}
	fmt.Printf("Final committed root: %v\n", rootHash3)

	if !bytes.Equal(previewRoot3[:], rootHash3[:]) {
		t.Errorf("❌ OverlayRoot != final Flush root!\nPreview: %x\nFlushed: %x", previewRoot3, rootHash3)
	} else {
		fmt.Println("✅ OverlayRoot matches final Flush root")
	}

	fmt.Println("\n✅ ALL TESTS PASSED: OverlayRoot works correctly after commits!")
}
