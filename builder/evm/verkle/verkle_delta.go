package verkle

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"
)

// VerkleStateDelta represents state changes in a block
// This is stored in EvmBlockPayload for delta replay
type VerkleStateDelta struct {
	NumEntries uint32
	Entries    []byte // Flattened [key(32B), value(32B)] pairs
}

// Serialize converts delta to bytes for storage in block payload
func (d *VerkleStateDelta) Serialize() []byte {
	buf := make([]byte, 4+len(d.Entries))
	binary.LittleEndian.PutUint32(buf[0:4], d.NumEntries)
	copy(buf[4:], d.Entries)
	return buf
}

// Deserialize parses delta from block payload bytes
func (d *VerkleStateDelta) Deserialize(data []byte) error {
	if len(data) < 4 {
		return fmt.Errorf("delta too short: %d bytes", len(data))
	}

	d.NumEntries = binary.LittleEndian.Uint32(data[0:4])
	expectedLen := 4 + (d.NumEntries * 64)

	if uint32(len(data)) != expectedLen {
		return fmt.Errorf("invalid delta length: got %d, expected %d", len(data), expectedLen)
	}

	d.Entries = make([]byte, len(data)-4)
	copy(d.Entries, data[4:])

	return nil
}

// AugmentedPostStateWrite represents a single write from post-state witness
// Format: [32B verkle_key][32B post_value][4B tx_index]
type AugmentedPostStateWrite struct {
	VerkleKey [32]byte
	PostValue [32]byte
	TxIndex   uint32
}

// parsePostStateWitnessForDelta parses the post-state section of a verkle witness for delta extraction
// Each entry is 161 bytes: [32B key][1B type][20B addr][8B extra][32B storage_key][32B pre][32B post][4B tx_index]
// We only care about the verkle_key (first 32 bytes) and post_value (at offset 129)
func parsePostStateWitnessForDelta(postStateWitness []byte) ([]AugmentedPostStateWrite, error) {
	// Post-state witness format:
	// [32B post_state_root]
	// [4B write_key_count]
	// For each write (161B):
	//   [32B verkle_key]
	//   [1B key_type]
	//   [20B address]
	//   [8B extra]
	//   [32B storage_key]
	//   [32B pre_value]
	//   [32B post_value]
	//   [4B tx_index]
	// [4B post_proof_len]
	// [variable post_proof_bytes]

	if len(postStateWitness) < 36 {
		return nil, fmt.Errorf("post-state witness too short: %d bytes", len(postStateWitness))
	}

	// Skip post_state_root (32 bytes)
	offset := 32

	// Read write_key_count (BigEndian to match Rust serialization)
	writeCount := binary.BigEndian.Uint32(postStateWitness[offset : offset+4])
	offset += 4

	// Verify we have enough data for all entries
	requiredLen := offset + int(writeCount)*161
	if len(postStateWitness) < requiredLen {
		return nil, fmt.Errorf("post-state witness too short for %d writes: got %d, need %d",
			writeCount, len(postStateWitness), requiredLen)
	}

	writes := make([]AugmentedPostStateWrite, writeCount)

	for i := uint32(0); i < writeCount; i++ {
		entryOffset := offset + int(i)*161

		// Extract verkle_key (first 32 bytes)
		copy(writes[i].VerkleKey[:], postStateWitness[entryOffset:entryOffset+32])

		// Extract post_value (at offset 125 = 32+1+20+8+32+32)
		postValueOffset := entryOffset + 125
		copy(writes[i].PostValue[:], postStateWitness[postValueOffset:postValueOffset+32])

		// Extract tx_index (at offset 157 = 129+32-4, last 4 bytes of entry)
		// BigEndian to match Rust serialization
		txIndexOffset := entryOffset + 157
		writes[i].TxIndex = binary.BigEndian.Uint32(postStateWitness[txIndexOffset : txIndexOffset+4])
	}

	return writes, nil
}

// ExtractVerkleStateDelta builds delta from post-state witness
// This is called by the builder after executing transactions
func ExtractVerkleStateDelta(postStateWitness []byte) (*VerkleStateDelta, error) {
	// Parse post-state witness to get writes
	writes, err := parsePostStateWitnessForDelta(postStateWitness)
	if err != nil {
		return nil, fmt.Errorf("failed to parse post-state writes: %w", err)
	}

	// Deduplicate by key (last write wins, based on witness order)
	unique := make(map[[32]byte]AugmentedPostStateWrite, len(writes))
	for _, w := range writes {
		unique[w.VerkleKey] = w
	}

	// Collect keys for deterministic ordering
	keys := make([][32]byte, 0, len(unique))
	for k := range unique {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return bytes.Compare(keys[i][:], keys[j][:]) < 0
	})

	// Build flattened entries
	delta := &VerkleStateDelta{
		NumEntries: uint32(len(keys)),
		Entries:    make([]byte, len(keys)*64),
	}

	for i, key := range keys {
		write := unique[key]
		offset := i * 64
		copy(delta.Entries[offset:offset+32], write.VerkleKey[:])
		copy(delta.Entries[offset+32:offset+64], write.PostValue[:])
	}

	return delta, nil
}

// SizeEstimate returns estimated size of delta in bytes
func (d *VerkleStateDelta) SizeEstimate() int {
	return 4 + int(d.NumEntries)*64
}

// GetEntry returns the key-value pair at index i
func (d *VerkleStateDelta) GetEntry(i uint32) (key [32]byte, value [32]byte, err error) {
	if i >= d.NumEntries {
		return key, value, fmt.Errorf("index %d out of range (total %d entries)", i, d.NumEntries)
	}

	offset := i * 64
	copy(key[:], d.Entries[offset:offset+32])
	copy(value[:], d.Entries[offset+32:offset+64])

	return key, value, nil
}
