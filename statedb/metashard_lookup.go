package statedb

import (
	"encoding/binary"
	"fmt"

	"github.com/jam-duna/jamduna/common"
	"github.com/jam-duna/jamduna/types"
)

// ShardId identifies a meta-shard (8 bytes: 1 byte ld + 7 bytes prefix56)
type ShardId struct {
	Ld       uint8   // Local depth (0-56)
	Prefix56 [7]byte // 56-bit routing prefix
}

// SplitPoint represents a routing entry in the MetaSSR
type SplitPoint struct {
	D        uint8   // Global depth at time of entry creation
	Ld       uint8   // Local depth for this shard
	Prefix56 [7]byte // 56-bit prefix for routing
}

// MetaSSR is the Sparse Split Registry for meta-shards
type MetaSSR struct {
	GlobalDepth uint8        // Current global depth
	Entries     []SplitPoint // Routing entries
}

// ObjectRefEntry represents an ObjectID → ObjectRef mapping (69 bytes)
type ObjectRefEntry struct {
	ObjectID  common.Hash     // 32 bytes
	ObjectRef types.ObjectRef // 37 bytes (packed format with 40 bits)
}

// MetaShard contains entries for a specific shard
type MetaShard struct {
	MerkleRoot [32]byte // BMT root over entries for inclusion proofs
	Entries    []ObjectRefEntry
}

// ComputeMetaShardObjectID computes the ObjectID for a meta-shard.
// ObjectID format matches Rust `meta_shard_object_id`:
//
//	ld || prefix (ceil(ld/8) bytes) || zero padding to 32 bytes
func ComputeMetaShardObjectID(serviceID uint32, shardID ShardId) common.Hash {
	var result common.Hash
	result[0] = shardID.Ld

	prefixBytes := (shardID.Ld + 7) / 8
	if prefixBytes > 0 {
		copy(result[1:1+prefixBytes], shardID.Prefix56[:prefixBytes])
	}

	return result
}

// ParseMetaShardObjectID decodes the ld||prefix||zero format into a ShardId descriptor.
func ParseMetaShardObjectID(objectID common.Hash) (ShardId, bool) {
	ld := objectID[0]
	if ld > 56 {
		return ShardId{}, false
	}
	prefixBytes := int((ld + 7) / 8)
	if 1+prefixBytes > len(objectID) {
		return ShardId{}, false
	}

	var shardID ShardId
	shardID.Ld = ld
	if prefixBytes > 0 {
		copy(shardID.Prefix56[:], objectID[1:1+prefixBytes])
	}
	for _, b := range objectID[1+prefixBytes:] {
		if b != 0 {
			return ShardId{}, false
		}
	}
	return shardID, true
}

// TakePrefix56 extracts first 56 bits (7 bytes) from 32-byte hash
func TakePrefix56(keyHash common.Hash) [7]byte {
	var prefix [7]byte
	copy(prefix[:], keyHash[:7])
	return prefix
}

// MatchesPrefix checks if keyPrefix matches entryPrefix for first 'bits' bits
func MatchesPrefix(keyPrefix, entryPrefix [7]byte, bits uint8) bool {
	bytesToCompare := int(bits / 8)
	remainingBits := bits % 8

	// Check full bytes
	if bytesToCompare > 0 {
		for i := 0; i < bytesToCompare && i < 7; i++ {
			if keyPrefix[i] != entryPrefix[i] {
				return false
			}
		}
	}

	// Check remaining bits
	if remainingBits > 0 && bytesToCompare < 7 {
		mask := byte(0xFF << (8 - remainingBits))
		if (keyPrefix[bytesToCompare] & mask) != (entryPrefix[bytesToCompare] & mask) {
			return false
		}
	}

	return true
}

// MaskPrefix56 zeroes all bits after the requested depth
func MaskPrefix56(prefix [7]byte, bits uint8) [7]byte {
	var masked [7]byte

	fullBytes := int(bits / 8)
	remainingBits := bits % 8

	if fullBytes > 0 {
		copy(masked[:], prefix[:fullBytes])
	}

	if remainingBits > 0 && fullBytes < 7 {
		masked[fullBytes] = prefix[fullBytes] & byte(0xFF<<(8-remainingBits))
	}

	return masked
}

// RouteKey finds the meta-shard ID for a given object key using MetaSSR routing
func RouteKey(keyHash common.Hash, metaSSR *MetaSSR) (ShardId, error) {
	if metaSSR == nil {
		return ShardId{}, fmt.Errorf("MetaSSR is nil")
	}

	keyPrefix := TakePrefix56(keyHash)

	// Start from the global depth and walk down split exceptions like Rust resolve_shard_id
	currentLd := metaSSR.GlobalDepth
	currentPrefix := MaskPrefix56(keyPrefix, currentLd)

	for {
		foundSplit := false

		for i := range metaSSR.Entries {
			entry := &metaSSR.Entries[i]
			if entry.D == currentLd && MatchesPrefix(keyPrefix, entry.Prefix56, entry.D) {
				currentLd = entry.Ld
				currentPrefix = MaskPrefix56(keyPrefix, currentLd)
				foundSplit = true
				break
			}
		}

		if !foundSplit {
			break
		}
	}

	return ShardId{
		Ld:       currentLd,
		Prefix56: currentPrefix,
	}, nil
}

// DeserializeMetaSSR deserializes MetaSSR from DA format
// Format: [1B global_depth][entries...]
// Each entry: [1B d][1B ld][7B prefix56]
func DeserializeMetaSSR(data []byte) (*MetaSSR, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("MetaSSR data empty")
	}

	globalDepth := data[0]

	// Calculate number of entries from remaining data
	if (len(data)-1)%9 != 0 {
		return nil, fmt.Errorf("MetaSSR data invalid: %d bytes after global_depth, expected multiple of 9", len(data)-1)
	}

	entryCount := (len(data) - 1) / 9
	entries := make([]SplitPoint, 0, entryCount)
	offset := 1

	for i := 0; i < entryCount; i++ {
		if offset+9 > len(data) {
			return nil, fmt.Errorf("MetaSSR entry %d truncated", i)
		}

		entry := SplitPoint{
			D:  data[offset],
			Ld: data[offset+1],
		}
		copy(entry.Prefix56[:], data[offset+2:offset+9])
		entries = append(entries, entry)
		offset += 9
	}

	return &MetaSSR{
		GlobalDepth: globalDepth,
		Entries:     entries,
	}, nil
}

// DeserializeMetaShard deserializes a meta-shard from DA format and validates the header
// Format: [8B shard_id][32B merkle_root][2B count][entries...]
// Each entry: [32B object_id][36B object_ref]
//
// Security: validates that the header (ld, prefix56) hashes to the expected ObjectID to
// prevent malformed witnesses from routing to a different shard than the object key.
func DeserializeMetaShard(data []byte, expectedObjectID common.Hash, serviceID uint32) (ShardId, *MetaShard, error) {
	if len(data) < 42 { // 8 (shard_id) + 32 (merkle_root) + 2 (count)
		return ShardId{}, nil, fmt.Errorf("meta-shard data too short: need 42 bytes minimum, got %d", len(data))
	}

	// Parse shard_id header (8 bytes)
	shardID := ShardId{
		Ld: data[0],
	}
	copy(shardID.Prefix56[:], data[1:8])

	// Validate object_id derived from header matches expected (Rust enforces this)
	computedObjectID := ComputeMetaShardObjectID(serviceID, shardID)
	if computedObjectID != expectedObjectID {
		return ShardId{}, nil, fmt.Errorf("meta-shard header mismatch: expected %s computed %s", expectedObjectID.Hex(), computedObjectID.Hex())
	}

	// Parse merkle_root (32 bytes)
	var merkleRoot [32]byte
	copy(merkleRoot[:], data[8:40])

	// Parse entry count (2 bytes)
	count := binary.LittleEndian.Uint16(data[40:42])
	// fmt.Printf("DEBUG: Meta-shard deserialize: count=%d, data_len=%d, bytes[40:42]=%x\n",
	// 	count, len(data), data[40:42])
	// fmt.Printf("DEBUG:   First 50 bytes: %x\n", data[:min(50, len(data))])
	// fmt.Printf("DEBUG:   Bytes [8:42] (merkle+count): %x\n", data[8:42])

	entries := make([]ObjectRefEntry, 0, count)
	offset := 42

	for i := uint16(0); i < count; i++ {
		if offset+69 > len(data) { // 32 (object_id) + 37 (object_ref)
			return ShardId{}, nil, fmt.Errorf("meta-shard entry %d truncated", i)
		}

		var entry ObjectRefEntry
		copy(entry.ObjectID[:], data[offset:offset+32])
		offset += 32 // Advance past object_id

		// Deserialize ObjectRef (37 bytes) - DeserializeObjectRef advances offset by 37
		objRef, err := types.DeserializeObjectRef(data, &offset)
		if err != nil {
			return ShardId{}, nil, fmt.Errorf("failed to deserialize ObjectRef at entry %d: %v", i, err)
		}
		entry.ObjectRef = objRef

		entries = append(entries, entry)
	}

	return shardID, &MetaShard{
		MerkleRoot: merkleRoot,
		Entries:    entries,
	}, nil
}

// Helper function to convert uint32 to little-endian bytes
func uint32ToLE(val uint32) []byte {
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, val)
	return buf
}
