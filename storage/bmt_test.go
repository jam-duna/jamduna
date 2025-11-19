package storage

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"os"
	"testing"
)

func hex2Bytes(hexStr string) []byte {
	bytes, err := hex.DecodeString(hexStr)
	if err != nil {
		panic(err)
	}
	return bytes
}

// setupStorage creates a temporary StateDBStorage for testing
func setupStorage() *StateDBStorage {
	tmpDir, err := os.MkdirTemp("", "nomt-test-*")
	if err != nil {
		panic(fmt.Sprintf("failed to create temp directory: %v", err))
	}

	storage, err := NewStateDBStorage(tmpDir, nil, nil, 0)
	if err != nil {
		panic(fmt.Sprintf("failed to create storage: %v", err))
	}

	return storage
}

// TestBPTProofSimple tests fresh inserts with GP encoding - matches trie/bmt_test.go:TestBPTProofSimple
func TestBPTProofSimple(t *testing.T) {
	data := [][2][]byte{
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
	expectedRootHash := hex2Bytes("511727325a0cd23890c21cda3c6f8b1c9fbdf37ed57b9a85ca77286356183dcf")

	// Create an empty Merkle Tree with unique temp directory
	tree := setupStorage()
	defer tree.Close()
	fmt.Printf("Initial root hash: %s\n", tree.GetRoot())

	// Insert key-value pairs one by one
	for index, kv := range data {
		key := kv[0]
		value := kv[1]
		fmt.Printf("Insert #%d: key=%x (len=%d), value=%x (len=%d)\n", index, key, len(key), value, len(value))
		tree.Insert(key, value)
	}

	// Test OverlayRoot() - compute root from overlay WITHOUT committing
	previewRoot, err := tree.OverlayRoot()
	if err != nil {
		t.Fatalf("OverlayRoot failed: %v", err)
	}
	fmt.Printf("Preview root (before flush): %v\n", previewRoot)

	// Preview root should match expected root (since all data is in overlay)
	if !bytes.Equal(previewRoot[:], expectedRootHash) {
		t.Errorf("OverlayRoot mismatch!\nGot:      %x\nExpected: %x", previewRoot, expectedRootHash)
	} else {
		fmt.Println("✅ OverlayRoot matches expected (before flush)!")
	}

	// Flush to commit all inserts
	rootHash, err := tree.Flush()
	if err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	fmt.Printf("Flushed root hash: %v\n", rootHash)
	fmt.Printf("Expected root hash: 0x%x\n", expectedRootHash)

	// Verify OverlayRoot() matched Flush() root
	if !bytes.Equal(previewRoot[:], rootHash[:]) {
		t.Errorf("OverlayRoot != Flush root!\nPreview:  %v\nFlushed:  %v", previewRoot, rootHash)
	} else {
		fmt.Println("✅ OverlayRoot() matches Flush() root!")
	}

	// Verify root hash matches expected
	if !bytes.Equal(rootHash[:], expectedRootHash) {
		t.Errorf("Root hash mismatch!\nGot:      %s\nExpected: %s", rootHash, expectedRootHash)
	} else {
		fmt.Println("✅ Root hash matches JAM!")
	}
}

// TestBPTUpdateExisting tests updating an existing key - this is the failing test in GPTESTS.md
func TestBPTUpdateExisting(t *testing.T) {
	// Initial data - same 11 keys as TestBPTProofSimple
	data := [][2][]byte{
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
	expectedRootHash1 := hex2Bytes("511727325a0cd23890c21cda3c6f8b1c9fbdf37ed57b9a85ca77286356183dcf")

	// Create storage
	tree := setupStorage()
	defer tree.Close()
	for _, kv := range data {
		tree.Insert(kv[0], kv[1])
	}

	// Flush to commit
	rootHash1, err := tree.Flush()
	if err != nil {
		t.Fatalf("Initial flush failed: %v", err)
	}

	fmt.Printf("Initial root hash: %s\n", rootHash1)
	if !bytes.Equal(rootHash1[:], expectedRootHash1) {
		t.Errorf("Initial root hash mismatch!\nGot:      %s\nExpected: %s", rootHash1, expectedRootHash1)
	}

	// Update key at index 5 with new value (32 bytes to match Rust test)
	updateKey := hex2Bytes("15207c233b055f921701fc62b41a440d01dfa488016a97cc653a84afb5f94fd5")
	newValue := hex2Bytes("deadbeefcafebabe0123456789abcdef0123456789abcdef0123456789abcdef")
	expectedRootHash2 := hex2Bytes("25c7077af793fb40bc28cbd908f18c80039b8c5e440837ba156a8928c8360416")

	fmt.Printf("\nUpdating key: %x\nNew value: %x\n", updateKey, newValue)
	tree.Insert(updateKey, newValue)

	// Flush the update
	rootHash2, err := tree.Flush()
	if err != nil {
		t.Fatalf("Update flush failed: %v", err)
	}

	fmt.Printf("Updated root hash: %v\n", rootHash2)
	fmt.Printf("Expected root hash: %x\n", expectedRootHash2)

	// Verify the incremental update produces the correct root hash
	if !bytes.Equal(rootHash2[:], expectedRootHash2) {
		t.Errorf("❌ Updated root hash mismatch!\nGot:      %s\nExpected: %s", hex.EncodeToString(rootHash2[:]), hex.EncodeToString(expectedRootHash2))
	} else {
		fmt.Println("✅ Updated root hash matches!")
	}
}
