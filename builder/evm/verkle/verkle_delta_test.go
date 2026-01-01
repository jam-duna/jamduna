package verkle

import (
	"bytes"
	"testing"

	"github.com/colorfulnotion/jam/common"
	"github.com/ethereum/go-verkle"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDeltaReplay verifies end-to-end delta extraction and replay
// This simulates the builder→guarantor flow:
// 1. Builder extracts delta from post-state witness
// 2. Delta is embedded in EvmBlockPayload
// 3. Guarantor replays delta to verify state transition
func TestDeltaReplay(t *testing.T) {
	// Create base tree (pre-state)
	baseTree := verkle.New()

	// Insert some initial state
	key1 := make([]byte, 32)
	key2 := make([]byte, 32)
	key3 := make([]byte, 32)
	key1[31] = 0x01
	key2[31] = 0x02
	key3[31] = 0x03

	value1 := make([]byte, 32)
	value2 := make([]byte, 32)
	value3 := make([]byte, 32)
	value1[31] = 0xAA
	value2[31] = 0xBB
	value3[31] = 0xCC

	require.NoError(t, baseTree.Insert(key1, value1, nil))
	require.NoError(t, baseTree.Insert(key2, value2, nil))
	baseTree.Commit()

	baseHashBytes := baseTree.Hash().Bytes()
	baseRoot := common.BytesToHash(baseHashBytes[:])

	t.Logf("Base tree root: %x", baseRoot)

	// Simulate post-state changes (what would be in witness)
	// Change key2, add key3, key1 unchanged
	newValue2 := make([]byte, 32)
	newValue2[31] = 0xDD // key2 value changed

	// Create post-state tree
	postTree := baseTree.Copy()
	require.NoError(t, postTree.Insert(key2, newValue2, nil)) // Update
	require.NoError(t, postTree.Insert(key3, value3, nil))    // Insert
	postTree.Commit()

	postHashBytes := postTree.Hash().Bytes()
	expectedRoot := common.BytesToHash(postHashBytes[:])

	t.Logf("Post tree root: %x", expectedRoot)

	// Build mock post-state witness in the format parsePostStateWitnessForDelta expects
	// Format: [32B post_root][4B write_count][161B×N writes][4B proof_len][proof]

	// For this test, we'll create a simplified witness with just the writes section
	var key2Arr, key3Arr, newValue2Arr, value3Arr [32]byte
	copy(key2Arr[:], key2)
	copy(key3Arr[:], key3)
	copy(newValue2Arr[:], newValue2)
	copy(value3Arr[:], value3)

	writes := []struct {
		key   [32]byte
		value [32]byte
	}{
		{key2Arr, newValue2Arr}, // Changed
		{key3Arr, value3Arr},    // New
	}

	// Build witness: [32B post_root][4B count][161B entries...]
	witness := make([]byte, 32+4+len(writes)*161+4)
	offset := 0

	// Post root
	copy(witness[offset:], expectedRoot[:])
	offset += 32

	// Write count (little endian)
	witness[offset] = byte(len(writes))
	witness[offset+1] = 0
	witness[offset+2] = 0
	witness[offset+3] = 0
	offset += 4

	// Write entries (161 bytes each)
	for _, w := range writes {
		// [32B verkle_key][1B key_type][20B address][8B extra][32B storage_key][32B pre_value][32B post_value][4B tx_index]
		copy(witness[offset:offset+32], w.key[:])      // verkle_key
		offset += 32
		witness[offset] = 0 // key_type
		offset += 1
		offset += 20 // skip address
		offset += 8  // skip extra
		offset += 32 // skip storage_key
		offset += 32 // skip pre_value
		copy(witness[offset:offset+32], w.value[:]) // post_value
		offset += 32
		offset += 4 // skip tx_index
	}

	// Proof length (0 for mock)
	offset += 4

	t.Logf("Built mock witness: %d bytes", len(witness))

	// PHASE 1: Builder extracts delta from witness
	delta, err := ExtractVerkleStateDelta(witness)
	require.NoError(t, err, "Failed to extract delta from witness")
	require.NotNil(t, delta)

	assert.Equal(t, uint32(2), delta.NumEntries, "Should have 2 entries (key2 changed, key3 new)")
	assert.Equal(t, 2*64, len(delta.Entries), "Entries should be 128 bytes (2 × 64)")

	t.Logf("Extracted delta: %d entries, %d bytes", delta.NumEntries, len(delta.Entries))

	// Verify delta contains correct keys
	for i := uint32(0); i < delta.NumEntries; i++ {
		offset := i * 64
		extractedKey := delta.Entries[offset : offset+32]
		extractedValue := delta.Entries[offset+32 : offset+64]

		// Delta should contain key2 and key3 (sorted)
		if bytes.Equal(extractedKey, key2[:]) {
			assert.Equal(t, newValue2[:], extractedValue, "key2 should have new value")
			t.Logf("✓ Delta entry %d: key2 → new value", i)
		} else if bytes.Equal(extractedKey, key3[:]) {
			assert.Equal(t, value3[:], extractedValue, "key3 should have value3")
			t.Logf("✓ Delta entry %d: key3 → value3", i)
		} else {
			t.Errorf("Unexpected key in delta: %x", extractedKey)
		}
	}

	// PHASE 2: Guarantor replays delta to verify state transition
	replayedTree, err := ReplayVerkleStateDelta(baseTree, delta, expectedRoot)
	require.NoError(t, err, "Failed to replay delta")
	require.NotNil(t, replayedTree)

	// Verify replayed tree matches expected post-state
	replayedHashBytes := replayedTree.Hash().Bytes()
	replayedRoot := common.BytesToHash(replayedHashBytes[:])

	assert.Equal(t, expectedRoot, replayedRoot, "Replayed tree root should match expected post-state root")

	t.Logf("✅ Delta replay successful: %x → %x", baseRoot, replayedRoot)

	// Verify individual values in replayed tree
	val2, err := replayedTree.Get(key2, nil)
	require.NoError(t, err)
	assert.Equal(t, newValue2[:], val2, "key2 should have updated value after replay")

	val3, err := replayedTree.Get(key3, nil)
	require.NoError(t, err)
	assert.Equal(t, value3[:], val3, "key3 should exist after replay")

	t.Logf("✅ All values verified in replayed tree")
}

// TestDeltaReplayRootMismatch verifies that replay fails when root doesn't match
func TestDeltaReplayRootMismatch(t *testing.T) {
	baseTree := verkle.New()

	key := make([]byte, 32)
	value := make([]byte, 32)
	key[31] = 0x01
	value[31] = 0xAA

	require.NoError(t, baseTree.Insert(key, value, nil))
	baseTree.Commit()

	// Create delta with wrong value
	wrongValue := make([]byte, 32)
	wrongValue[31] = 0xBB

	delta := &VerkleStateDelta{
		NumEntries: 1,
		Entries:    make([]byte, 64),
	}
	copy(delta.Entries[0:32], key)
	copy(delta.Entries[32:64], wrongValue)

	// Use a fake expected root
	fakeRoot := common.HexToHash("0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef")

	_, err := ReplayVerkleStateDelta(baseTree, delta, fakeRoot)

	assert.Error(t, err, "Replay should fail with root mismatch")
	assert.Contains(t, err.Error(), "root mismatch", "Error should mention root mismatch")

	t.Logf("✅ Root mismatch correctly detected")
}

// TestDeltaSerializationRoundtrip verifies delta serialization/deserialization
func TestDeltaSerializationRoundtrip(t *testing.T) {
	// Create original delta
	original := &VerkleStateDelta{
		NumEntries: 3,
		Entries:    make([]byte, 3*64),
	}

	// Fill with test data
	for i := 0; i < 3; i++ {
		offset := i * 64
		// Key
		original.Entries[offset+31] = byte(i + 1)
		// Value
		original.Entries[offset+32+31] = byte(0xAA + i)
	}

	// Serialize
	serialized := original.Serialize()
	assert.Equal(t, 4+3*64, len(serialized), "Serialized size should be 4 + entries")

	// Deserialize
	deserialized := &VerkleStateDelta{}
	err := deserialized.Deserialize(serialized)
	require.NoError(t, err)

	// Verify
	assert.Equal(t, original.NumEntries, deserialized.NumEntries)
	assert.Equal(t, original.Entries, deserialized.Entries)

	t.Logf("✅ Serialization roundtrip successful")
}
