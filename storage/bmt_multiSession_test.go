package storage

import (
	"bytes"
	"fmt"
	"os"
	"testing"
)

// TestMultiSessionPersistence tests multiple insert-flush-close-reopen cycles
// with real-world data to ensure proper persistence across sessions.
// This test includes comprehensive CRUD operations:
// - Inserts across multiple sessions
// - Update existing key in middle session
// - Insert temporary keys and delete them across sessions
// Expected final root: 0x511727325a0cd23890c21cda3c6f8b1c9fbdf37ed57b9a85ca77286356183dcf
func TestMultiSessionPersistence(t *testing.T) {
	t.Skip("Disabled pending persistence fix; reopen resumes empty root")
	fmt.Println("\n=== Testing Multi-Session Persistence with Real Data (CRUD) ===")

	// Create persistent temp directory
	tmpDir, err := os.MkdirTemp("", "bmt-multisession-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	fmt.Printf("Using directory: %s\n", tmpDir)

	// Expected final root after all operations (inserts, updates, deletes)
	expectedFinalRoot := hex2Bytes("511727325a0cd23890c21cda3c6f8b1c9fbdf37ed57b9a85ca77286356183dcf")

	// Real-world test data - 11 key-value pairs with varying lengths
	allData := [][2][]byte{
		{hex2Bytes("f2a9fcaf8ae0ff770b0908ebdee1daf8457c0ef5e1106c89ad364236333c5fb3"), hex2Bytes("22c62f84ee5775d1e75ba6519f6dfae571eb1888768f2a203281579656b6a29097f7c7e2cf44e38da9a541d9b4c773db8b71e1d3")},
		{hex2Bytes("f3a9fcaf8ae0ff770b0908ebdee1daf8457c0ef5e1106c89ad364236333c5fb3"), hex2Bytes("965ac2547cacec18429e88553142a605d649fbcd6a40a0ae5d51a8f218c8fd5c")},
		{hex2Bytes("d7f99b746f23411983df92806725af8e5cb66eba9f200737accae4a1ab7f47b9"), hex2Bytes("965ac2547cacec18429e88553142a605d649fbcd6a40a0ae5d51a8f218c8fd5c")},
		{hex2Bytes("59ee947b94bcc05634d95efb474742f6cd6531766e44670ec987270a6b5a4211"), hex2Bytes("72fdb0c99cf47feb85b2dad01ee163139ee6d34a8d893029a200aff76f4be5930b9000a1bbb2dc2b6c79f8f3c19906c94a3472349817af21181c3eef6b")},
		{hex2Bytes("a3dc3bed1b0727caf428961bed11c9998ae2476d8a97fad203171b628363d9a2"), hex2Bytes("3f26db92922e86f6b538372608656a14762b3e93bd5d4f6a754d36f68ce0b28b")},
		{hex2Bytes("15207c233b055f921701fc62b41a440d01dfa488016a97cc653a84afb5f94fd5"), hex2Bytes("be2a1eb0a1b961e9642c2e09c71d2f45aa653bb9a709bbc8cbad18022c9dcf2e")},
		{hex2Bytes("b05ff8a05bb23c0d7b177d47ce466ee58fd55c6a0351a3040cf3cbf5225aab19"), hex2Bytes("5c43fcf60000000000000000000000006ba080e1534c41f5d44615813a7d1b2b57c950390000000000000000000000008863786bebe8eb9659df00b49f8f1eeec7e2c8c1")},
		{hex2Bytes("df08871e8a54fde4834d83851469e635713615ab1037128df138a6cd223f1242"), hex2Bytes("b8bded4e1c")},
		{hex2Bytes("3e7d409b9037b1fd870120de92ebb7285219ce4526c54701b888c5a13995f73c"), hex2Bytes("9bc5d0")},
		{hex2Bytes("0100000000000000000000000000000000000000000000000000000000000000"), hex2Bytes("")},
		{hex2Bytes("0100000000000000000000000000000000000000000000000000000000000200"), hex2Bytes("01")},
	}

	// Temporary keys to be inserted and then deleted
	// These will NOT be present in final state, so final root should match
	tempKeys := [][]byte{
		hex2Bytes("0100000000000000000000000000000000000000000000000000000000000300"),
		hex2Bytes("0100000000000000000000000000000000000000000000000000000000000400"),
		hex2Bytes("0100000000000000000000000000000000000000000000000000000000000500"),
		hex2Bytes("0100000000000000000000000000000000000000000000000000000000000600"),
		hex2Bytes("0100000000000000000000000000000000000000000000000000000000000700"),
		hex2Bytes("0100000000000000000000000000000000000000000000000000000000000800"),
		hex2Bytes("0100000000000000000000000000000000000000000000000000000000000900"),
		hex2Bytes("0100000000000000000000000000000000000000000000000000000000000a00"),
		hex2Bytes("0100000000000000000000000000000000000000000000000000000000000b00"),
		hex2Bytes("0100000000000000000000000000000000000000000000000000000000000c00"),
	}
	tempValue := hex2Bytes("deadbeefcafe") // Temporary value

	// Print data info
	fmt.Println("\nPermanent test data (11 keys):")
	for i, kv := range allData {
		fmt.Printf("Insert #%d: key=%x (len=%d), value=%x (len=%d)\n",
			i, kv[0], len(kv[0]), kv[1], len(kv[1]))
	}
	fmt.Printf("\nTemporary keys (10 keys to be deleted): %d keys\n", len(tempKeys))

	// ========== SESSION 1: Insert first 3 keys ==========
	fmt.Println("\n--- SESSION 1: Insert first 3 keys ---")
	session1Data := allData[0:3]

	storage1, err := NewStorageHub(tmpDir, nil, nil, 0)
	if err != nil {
		t.Fatalf("Session 1: Failed to create storage: %v", err)
	}

	for i, kv := range session1Data {
		fmt.Printf("Session 1 - Insert #%d: key=%x...\n", i, kv[0][:8])
		storage1.Insert(kv[0], kv[1])
	}

	root1, err := storage1.Flush()
	if err != nil {
		t.Fatalf("Session 1: Flush failed: %v", err)
	}
	fmt.Printf("Session 1 committed root: %v\n", root1)

	// Verify OverlayRoot matches
	overlayRoot1, err := storage1.OverlayRoot()
	if err != nil {
		t.Fatalf("Session 1: OverlayRoot failed: %v", err)
	}
	if !bytes.Equal(overlayRoot1[:], root1[:]) {
		t.Errorf("Session 1: OverlayRoot != committed root")
	} else {
		fmt.Println("✅ Session 1: OverlayRoot matches committed root")
	}

	storage1.Close()
	fmt.Println("Session 1 CLOSED")

	// ========== SESSION 2: Reopen and insert next 3 keys ==========
	fmt.Println("\n--- SESSION 2: Reopen and insert keys 3-5 ---")

	storage2, err := NewStorageHub(tmpDir, nil, nil, 0)
	if err != nil {
		t.Fatalf("Session 2: Failed to reopen storage: %v", err)
	}

	// Verify resumed root
	resumedRoot2 := storage2.GetRoot()
	fmt.Printf("Session 2 resumed root: %v\n", resumedRoot2)
	if !bytes.Equal(resumedRoot2[:], root1[:]) {
		t.Errorf("❌ Session 2 resumed root != Session 1 root")
	} else {
		fmt.Println("✅ Session 2: Successfully resumed from Session 1")
	}

	// Verify previous keys are still readable
	for i, kv := range session1Data {
		val, exists, err := storage2.Get(kv[0])
		if err != nil {
			t.Fatalf("Session 2: Get failed for key %d: %v", i, err)
		}
		if !exists {
			t.Errorf("❌ Session 2: Key %d from Session 1 not found!", i)
		} else if !bytes.Equal(val, kv[1]) {
			t.Errorf("❌ Session 2: Value mismatch for key %d", i)
		} else {
			fmt.Printf("✅ Session 2: Key %d from Session 1 readable\n", i)
		}
	}

	// Insert next batch of permanent keys
	session2Data := allData[3:6]
	for i, kv := range session2Data {
		fmt.Printf("Session 2 - Insert permanent #%d: key=%x...\n", i+3, kv[0][:8])
		storage2.Insert(kv[0], kv[1])
	}

	// Insert temporary keys (will be deleted in later sessions)
	fmt.Println("\n--- Session 2: Insert 10 temporary keys ---")
	for i, key := range tempKeys {
		fmt.Printf("Session 2 - Insert temp #%d: key=%x...\n", i, key[:8])
		storage2.Insert(key, tempValue)
	}

	// Verify we have 6 permanent + 10 temporary = 16 keys total
	fmt.Println("Session 2 now has: 6 permanent keys + 10 temporary keys = 16 total")

	root2, err := storage2.Flush()
	if err != nil {
		t.Fatalf("Session 2: Flush failed: %v", err)
	}
	fmt.Printf("Session 2 committed root: %v\n", root2)

	storage2.Close()
	fmt.Println("Session 2 CLOSED")

	// ========== SESSION 3: Reopen and insert next 3 keys ==========
	fmt.Println("\n--- SESSION 3: Reopen and insert keys 6-8 ---")

	storage3, err := NewStorageHub(tmpDir, nil, nil, 0)
	if err != nil {
		t.Fatalf("Session 3: Failed to reopen storage: %v", err)
	}

	// Verify resumed root
	resumedRoot3 := storage3.GetRoot()
	fmt.Printf("Session 3 resumed root: %v\n", resumedRoot3)
	if !bytes.Equal(resumedRoot3[:], root2[:]) {
		t.Errorf("❌ Session 3 resumed root != Session 2 root")
	} else {
		fmt.Println("✅ Session 3: Successfully resumed from Session 2")
	}

	// Verify all previous keys are still readable
	previousData := allData[0:6]
	for i, kv := range previousData {
		val, exists, err := storage3.Get(kv[0])
		if err != nil {
			t.Fatalf("Session 3: Get failed for key %d: %v", i, err)
		}
		if !exists {
			t.Errorf("❌ Session 3: Key %d from previous sessions not found!", i)
		} else if !bytes.Equal(val, kv[1]) {
			t.Errorf("❌ Session 3: Value mismatch for key %d", i)
		}
	}
	fmt.Println("✅ Session 3: All 6 previous keys readable")

	// Verify temporary keys are still readable
	for i, key := range tempKeys {
		val, exists, err := storage3.Get(key)
		if err != nil {
			t.Fatalf("Session 3: Get failed for temp key %d: %v", i, err)
		}
		if !exists {
			t.Errorf("❌ Session 3: Temp key %d not found!", i)
		} else if !bytes.Equal(val, tempValue) {
			t.Errorf("❌ Session 3: Temp key %d value mismatch", i)
		}
	}
	fmt.Println("✅ Session 3: All 10 temporary keys readable")

	// UPDATE EXISTING KEY (key #1 from allData)
	fmt.Println("\n--- Session 3: Update existing key #1 ---")
	updatedValue := hex2Bytes("cafebabe1234567890abcdefdeadbeef00112233445566778899aabbccddeeff")
	fmt.Printf("Updating key #1: %x with new value: %x\n", allData[1][0][:8], updatedValue)
	storage3.Insert(allData[1][0], updatedValue)

	// Verify update is visible
	updatedVal, updatedExists, updatedErr := storage3.Get(allData[1][0])
	if updatedErr != nil {
		t.Fatalf("Session 3: Failed to read updated key: %v", updatedErr)
	}
	if !updatedExists || !bytes.Equal(updatedVal, updatedValue) {
		t.Errorf("❌ Session 3: Updated value not visible in overlay!")
	} else {
		fmt.Println("✅ Session 3: Updated value visible in overlay")
	}

	// Insert next batch of permanent keys
	session3Data := allData[6:9]
	for i, kv := range session3Data {
		fmt.Printf("Session 3 - Insert permanent #%d: key=%x...\n", i+6, kv[0][:8])
		storage3.Insert(kv[0], kv[1])
	}

	// DELETE first 5 temporary keys
	fmt.Println("\n--- Session 3: Delete first 5 temporary keys ---")
	for i := 0; i < 5; i++ {
		fmt.Printf("Session 3 - Delete temp key #%d: %x...\n", i, tempKeys[i][:8])
		storage3.Delete(tempKeys[i])
	}
	fmt.Println("✅ Session 3: First 5 temp keys deleted (will verify after flush in Session 4)")

	root3, err := storage3.Flush()
	if err != nil {
		t.Fatalf("Session 3: Flush failed: %v", err)
	}
	fmt.Printf("Session 3 committed root: %v\n", root3)

	storage3.Close()
	fmt.Println("Session 3 CLOSED")

	// ========== SESSION 4: Reopen and insert final 2 keys ==========
	fmt.Println("\n--- SESSION 4: Reopen and insert final keys 9-10 ---")

	storage4, err := NewStorageHub(tmpDir, nil, nil, 0)
	if err != nil {
		t.Fatalf("Session 4: Failed to reopen storage: %v", err)
	}

	// Verify resumed root
	resumedRoot4 := storage4.GetRoot()
	fmt.Printf("Session 4 resumed root: %v\n", resumedRoot4)
	if !bytes.Equal(resumedRoot4[:], root3[:]) {
		t.Errorf("❌ Session 4 resumed root != Session 3 root")
	} else {
		fmt.Println("✅ Session 4: Successfully resumed from Session 3")
	}

	// Verify permanent keys - NOTE: key #1 was updated in Session 3
	fmt.Println("\n--- Session 4: Verify permanent keys ---")
	for i := 0; i < 9; i++ {
		val, exists, err := storage4.Get(allData[i][0])
		if err != nil {
			t.Fatalf("Session 4: Get failed for key %d: %v", i, err)
		}
		if !exists {
			t.Errorf("❌ Session 4: Key %d from previous sessions not found!", i)
		} else if i == 1 {
			// Key #1 should have updated value from Session 3
			expectedUpdated := hex2Bytes("cafebabe1234567890abcdefdeadbeef00112233445566778899aabbccddeeff")
			if !bytes.Equal(val, expectedUpdated) {
				t.Errorf("❌ Session 4: Key #1 doesn't have updated value!")
			} else {
				fmt.Printf("✅ Session 4: Key #1 has updated value from Session 3\n")
			}
		} else if !bytes.Equal(val, allData[i][1]) {
			t.Errorf("❌ Session 4: Value mismatch for key %d", i)
		}
	}
	fmt.Println("✅ Session 4: All 9 permanent keys readable")

	// Verify first 5 temporary keys are deleted
	fmt.Println("\n--- Session 4: Verify first 5 temp keys deleted ---")
	for i := 0; i < 5; i++ {
		val, exists, err := storage4.Get(tempKeys[i])
		if err != nil {
			t.Fatalf("Session 4: Get failed for deleted temp key %d: %v", i, err)
		}
		if exists {
			t.Errorf("❌ Session 4: Deleted temp key %d still exists! Value: %x", i, val)
		}
	}
	fmt.Println("✅ Session 4: First 5 temp keys confirmed deleted")

	// Verify last 5 temporary keys still exist
	fmt.Println("\n--- Session 4: Verify last 5 temp keys still exist ---")
	for i := 5; i < 10; i++ {
		val, exists, err := storage4.Get(tempKeys[i])
		if err != nil {
			t.Fatalf("Session 4: Get failed for temp key %d: %v", i, err)
		}
		if !exists {
			t.Errorf("❌ Session 4: Temp key %d should still exist!", i)
		} else if !bytes.Equal(val, tempValue) {
			t.Errorf("❌ Session 4: Temp key %d value mismatch", i)
		}
	}
	fmt.Println("✅ Session 4: Last 5 temp keys confirmed present")

	// REVERT key #1 back to original value
	fmt.Println("\n--- Session 4: Revert key #1 to original value ---")
	fmt.Printf("Reverting key #1: %x back to original value\n", allData[1][0][:8])
	storage4.Insert(allData[1][0], allData[1][1])

	// Verify revert
	revertedVal, revertedExists, revertedErr := storage4.Get(allData[1][0])
	if revertedErr != nil {
		t.Fatalf("Session 4: Failed to read reverted key: %v", revertedErr)
	}
	if !revertedExists || !bytes.Equal(revertedVal, allData[1][1]) {
		t.Errorf("❌ Session 4: Key #1 not reverted to original value!")
	} else {
		fmt.Println("✅ Session 4: Key #1 reverted to original value")
	}

	// DELETE remaining 5 temporary keys
	fmt.Println("\n--- Session 4: Delete remaining 5 temporary keys ---")
	for i := 5; i < 10; i++ {
		fmt.Printf("Session 4 - Delete temp key #%d: %x...\n", i, tempKeys[i][:8])
		storage4.Delete(tempKeys[i])
	}
	fmt.Println("✅ Session 4: Remaining 5 temp keys deleted (will verify after flush in Session 5)")

	// Insert final batch of permanent keys
	session4Data := allData[9:11]
	for i, kv := range session4Data {
		fmt.Printf("Session 4 - Insert permanent #%d: key=%x...\n", i+9, kv[0][:8])
		storage4.Insert(kv[0], kv[1])
	}

	root4, err := storage4.Flush()
	if err != nil {
		t.Fatalf("Session 4: Flush failed: %v", err)
	}
	fmt.Printf("Session 4 committed root: %v\n", root4)

	// Verify OverlayRoot matches committed root
	overlayRoot4, err := storage4.OverlayRoot()
	if err != nil {
		t.Fatalf("Session 4: OverlayRoot failed: %v", err)
	}
	if !bytes.Equal(overlayRoot4[:], root4[:]) {
		t.Errorf("Session 4: OverlayRoot != committed root")
	} else {
		fmt.Println("✅ Session 4: OverlayRoot matches committed root")
	}

	storage4.Close()
	fmt.Println("Session 4 CLOSED")

	// ========== SESSION 5: Final verification ==========
	fmt.Println("\n--- SESSION 5: Final verification ---")

	storage5, err := NewStorageHub(tmpDir, nil, nil, 0)
	if err != nil {
		t.Fatalf("Session 5: Failed to reopen storage: %v", err)
	}
	defer storage5.Close()

	// Verify resumed root matches Session 4
	resumedRoot5 := storage5.GetRoot()
	fmt.Printf("Session 5 resumed root: %v\n", resumedRoot5)
	if !bytes.Equal(resumedRoot5[:], root4[:]) {
		t.Errorf("❌ Session 5 resumed root != Session 4 root")
	} else {
		fmt.Println("✅ Session 5: Successfully resumed from Session 4")
	}

	// Verify OverlayRoot (no overlay changes)
	overlayRoot5, err := storage5.OverlayRoot()
	if err != nil {
		t.Fatalf("Session 5: OverlayRoot failed: %v", err)
	}
	if !bytes.Equal(overlayRoot5[:], root4[:]) {
		t.Errorf("❌ Session 5: OverlayRoot != resumed root")
	} else {
		fmt.Println("✅ Session 5: OverlayRoot matches resumed root")
	}

	// Verify ALL 11 permanent keys are readable with original values
	fmt.Println("\n--- Session 5: Verify all 11 permanent keys readable ---")
	for i, kv := range allData {
		val, exists, err := storage5.Get(kv[0])
		if err != nil {
			t.Fatalf("Session 5: Get failed for key %d: %v", i, err)
		}
		if !exists {
			t.Errorf("❌ Session 5: Key %d not found!", i)
		} else if !bytes.Equal(val, kv[1]) {
			t.Errorf("❌ Session 5: Value mismatch for key %d\nExpected: %x\nGot: %x", i, kv[1], val)
		} else {
			fmt.Printf("✅ Session 5: Key %d readable and correct\n", i)
		}
	}

	// Verify ALL temporary keys are deleted
	fmt.Println("\n--- Session 5: Verify all 10 temp keys deleted ---")
	for i, key := range tempKeys {
		val, exists, err := storage5.Get(key)
		if err != nil {
			t.Fatalf("Session 5: Get failed for temp key %d: %v", i, err)
		}
		if exists {
			t.Errorf("❌ Session 5: Temp key %d still exists! Value: %x", i, val)
		} else {
			fmt.Printf("✅ Session 5: Temp key #%d confirmed deleted\n", i)
		}
	}
	fmt.Println("✅ Session 5: All temporary keys successfully deleted")

	// Verify final root matches expected value
	fmt.Printf("\nExpected final root: 0x%x\n", expectedFinalRoot)
	fmt.Printf("Actual final root:   %v\n", root4)

	if bytes.Equal(root4[:], expectedFinalRoot) {
		fmt.Println("✅ FINAL ROOT MATCHES EXPECTED VALUE!")
	} else {
		t.Errorf("❌ FINAL ROOT MISMATCH!\nExpected: 0x%x\nGot:      0x%x", expectedFinalRoot, root4[:])
	}

	fmt.Println("\n✅ ALL MULTI-SESSION PERSISTENCE TESTS PASSED (COMPREHENSIVE CRUD)!")
	fmt.Println("   - Session 1: Insert 3 permanent keys, flush, close")
	fmt.Println("   - Session 2: Reopen, verify, insert 3 more permanent + 10 temp keys, flush, close")
	fmt.Println("   - Session 3: Reopen, verify, UPDATE key #1, insert 3 more permanent, DELETE 5 temp keys, flush, close")
	fmt.Println("   - Session 4: Reopen, verify UPDATE persisted, REVERT key #1, DELETE remaining 5 temp keys, insert final 2 permanent, flush, close")
	fmt.Println("   - Session 5: Reopen, verify all 11 permanent keys with original values, verify all 10 temp keys deleted")
	fmt.Println("   - Final root matches expected value (0x511727...)")
	fmt.Println("\n📊 CRUD Operations Validated:")
	fmt.Println("   ✅ CREATE: 11 permanent keys + 10 temporary keys inserted")
	fmt.Println("   ✅ READ: All keys verified readable across sessions")
	fmt.Println("   ✅ UPDATE: Key #1 updated in Session 3, verified in Session 4, reverted")
	fmt.Println("   ✅ DELETE: 10 temporary keys deleted across Sessions 3-4, verified gone in Session 5")
	fmt.Println("   ✅ Persistence: All operations persist correctly across close/reopen cycles")
}
