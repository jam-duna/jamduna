package core

import (
	"encoding/hex"
	"testing"

	"golang.org/x/crypto/blake2b"
)

func TestGetGpBit(t *testing.T) {
	// Test key: 0b10000000 00000000 ... (MSB set)
	key := make([]byte, 32)
	key[0] = 0b10000000

	if !getGpBit(key, 0) {
		t.Error("First bit (MSB) should be 1")
	}
	if getGpBit(key, 1) {
		t.Error("Second bit should be 0")
	}
	if getGpBit(key, 7) {
		t.Error("Last bit of first byte should be 0")
	}

	// Test key: 0b01000000 ... (second bit set)
	key[0] = 0b01000000
	if getGpBit(key, 0) {
		t.Error("First bit should be 0")
	}
	if !getGpBit(key, 1) {
		t.Error("Second bit should be 1")
	}
}

func TestBuildEmptyTree(t *testing.T) {
	mockHash := func(data []byte) [32]byte {
		var result [32]byte
		result[0] = byte(len(data))
		return result
	}

	var kvs []KVPair
	tree := BuildGpTree(kvs, 0, mockHash)

	if tree.Hash != Terminator {
		t.Error("Empty tree should have terminator hash")
	}
	if tree.Key != nil {
		t.Error("Empty tree should have nil key")
	}
	if tree.Left != nil || tree.Right != nil {
		t.Error("Empty tree should have no children")
	}
}

func TestBuildSingleLeaf(t *testing.T) {
	mockHash := func(data []byte) [32]byte {
		var result [32]byte
		if len(data) >= 64 {
			result[0] = 0xFF // Mark as leaf encoding
		}
		return result
	}

	var key KeyPath
	for i := range key {
		key[i] = 0x42
	}
	value := []byte("test")

	kvs := []KVPair{{Key: key, Value: value}}
	tree := BuildGpTree(kvs, 0, mockHash)

	if tree.Key == nil || *tree.Key != key {
		t.Error("Single leaf should have the key")
	}
	if tree.Left != nil || tree.Right != nil {
		t.Error("Leaf should have no children")
	}
	if tree.Hash[0] != 0xFF {
		t.Error("Should be marked as leaf")
	}
}

func TestBuildTwoLeaves(t *testing.T) {
	mockHash := func(data []byte) [32]byte {
		var result [32]byte
		result[0] = byte(len(data))
		return result
	}

	// Keys that differ in first bit
	var key1, key2 KeyPath
	key1[0] = 0b00000000 // First bit = 0
	key2[0] = 0b10000000 // First bit = 1

	kvs := []KVPair{
		{Key: key1, Value: []byte("value1")},
		{Key: key2, Value: []byte("value2")},
	}

	tree := BuildGpTree(kvs, 0, mockHash)

	// Should be a branch node
	if tree.Left == nil || tree.Right == nil {
		t.Error("Should be a branch node with children")
	}
	if tree.Key != nil {
		t.Error("Branch node should have nil key")
	}

	// Left child should have key1
	if tree.Left.Key == nil || *tree.Left.Key != key1 {
		t.Error("Left child should have key1")
	}

	// Right child should have key2
	if tree.Right.Key == nil || *tree.Right.Key != key2 {
		t.Error("Right child should have key2")
	}
}

// TestGpTreeJamCompatibility tests GP tree building against JAM test vectors.
// This uses the same test data from JAM's TestBPTProofSimple.
func TestGpTreeJamCompatibility(t *testing.T) {
	// Test data from JAM's trie/bpt_test.go:TestBPTProofSimple (11 key-value pairs)
	testData := []struct {
		keyHex   string
		valueHex string
	}{
		{"f2a9fcaf8ae0ff770b0908ebdee1daf8457c0ef5e1106c89ad364236333c5fb3", "22c62f84ee5775d1e75ba6519f6dfae571eb1888768f2a203281579656b6a29097f7c7e2cf44e38da9a541d9b4c773db8b71e1d3"},
		{"f3a9fcaf8ae0ff770b0908ebdee1daf8457c0ef5e1106c89ad364236333c5fb3", "965ac2547cacec18429e88553142a605d649fbcd6a40a0ae5d51a8f218c8fd5c"},
		{"d7f99b746f23411983df92806725af8e5cb66eba9f200737accae4a1ab7f47b9", "965ac2547cacec18429e88553142a605d649fbcd6a40a0ae5d51a8f218c8fd5c"},
		{"59ee947b94bcc05634d95efb474742f6cd6531766e44670ec987270a6b5a4211", "72fdb0c99cf47feb85b2dad01ee163139ee6d34a8d893029a200aff76f4be5930b9000a1bbb2dc2b6c79f8f3c19906c94a3472349817af21181c3eef6b"},
		{"a3dc3bed1b0727caf428961bed11c9998ae2476d8a97fad203171b628363d9a2", "3f26db92922e86f6b538372608656a14762b3e93bd5d4f6a754d36f68ce0b28b"},
		{"15207c233b055f921701fc62b41a440d01dfa488016a97cc653a84afb5f94fd5", "be2a1eb0a1b961e9642c2e09c71d2f45aa653bb9a709bbc8cbad18022c9dcf2e"},
		{"b05ff8a05bb23c0d7b177d47ce466ee58fd55c6a0351a3040cf3cbf5225aab19", "5c43fcf60000000000000000000000006ba080e1534c41f5d44615813a7d1b2b57c950390000000000000000000000008863786bebe8eb9659df00b49f8f1eeec7e2c8c1"},
		{"df08871e8a54fde4834d83851469e635713615ab1037128df138a6cd223f1242", "b8bded4e1c"},
		{"3e7d409b9037b1fd870120de92ebb7285219ce4526c54701b888c5a13995f73c", "9bc5d0"},
		{"0100000000000000000000000000000000000000000000000000000000000000", ""},
		{"0100000000000000000000000000000000000000000000000000000000000200", "01"},
	}

	// Expected root hash from JAM's Go test
	expectedRootHex := "511727325a0cd23890c21cda3c6f8b1c9fbdf37ed57b9a85ca77286356183dcf"

	// Convert test data to KVPairs
	var kvs []KVPair
	for _, td := range testData {
		keyBytes, _ := hex.DecodeString(td.keyHex)
		valueBytes, _ := hex.DecodeString(td.valueHex)

		var key KeyPath
		copy(key[:], keyBytes)

		kvs = append(kvs, KVPair{Key: key, Value: valueBytes})
	}

	// Build tree using Blake2b-256
	hasher := func(data []byte) [32]byte {
		return blake2b.Sum256(data)
	}

	tree := BuildGpTree(kvs, 0, hasher)

	computedRoot := tree.Hash
	t.Logf("Computed root: %s", hex.EncodeToString(computedRoot[:]))
	t.Logf("Expected root: %s", expectedRootHex)

	if hex.EncodeToString(computedRoot[:]) != expectedRootHex {
		t.Errorf("Root hash mismatch!\nGot:      %s\nExpected: %s",
			hex.EncodeToString(computedRoot[:]),
			expectedRootHex)

		// Print individual leaf hashes for debugging
		t.Log("Individual leaf hashes:")
		for i, kv := range kvs {
			leafData := NewLeafDataGP(kv.Key, kv.Value, hasher(kv.Value))
			encoded := EncodeGpLeaf(leafData)
			leafHash := hasher(encoded[:])
			t.Logf("  Leaf %d: %s", i, hex.EncodeToString(leafHash[:]))
			t.Logf("    Key:    %s", hex.EncodeToString(kv.Key[:8]))
			t.Logf("    Value:  %d bytes, embedded: %t", len(kv.Value), len(kv.Value) <= 32)
		}
	} else {
		t.Log("✅ SUCCESS: Root hash matches JAM test vector!")
	}
}
