package statedb

import (
	"encoding/binary"
	"fmt"

	"github.com/jam-duna/jamduna/common"
	"github.com/jam-duna/jamduna/log"
	"github.com/jam-duna/jamduna/trie"
	"github.com/jam-duna/jamduna/types"
)

// parseMetaShardEntries extracts ObjectRefEntry list from a meta-shard payload.
// Payload format: [8B shard_id][32B merkle_root][2B count][entries...]
// Each entry: [32B object_id][37B object_ref]
func parseMetaShardEntries(payload []byte) ([]ObjectRefEntry, error) {
	if len(payload) < 42 { // 8 + 32 + 2 = minimum header size
		return nil, fmt.Errorf("meta-shard payload too short")
	}

	// Skip shard_id (8 bytes) and merkle_root (32 bytes).
	offset := 40

	// Parse entry count (2 bytes).
	count := binary.LittleEndian.Uint16(payload[offset : offset+2])
	offset += 2

	entries := make([]ObjectRefEntry, 0, count)
	for i := uint16(0); i < count; i++ {
		if offset+69 > len(payload) { // 32 (object_id) + 37 (object_ref)
			return nil, fmt.Errorf("meta-shard entry %d truncated", i)
		}

		var entry ObjectRefEntry
		copy(entry.ObjectID[:], payload[offset:offset+32])
		offset += 32

		// Deserialize ObjectRef (37 bytes).
		objRef, err := types.DeserializeObjectRef(payload, &offset)
		if err != nil {
			return nil, fmt.Errorf("failed to deserialize ObjectRef at entry %d: %v", i, err)
		}
		entry.ObjectRef = objRef
		entries = append(entries, entry)
	}

	return entries, nil
}

// generateMetaShardInclusionProof generates a BMT inclusion proof for an objectID in meta-shard entries.
func generateMetaShardInclusionProof(merkleRoot [32]byte, objectID common.Hash, entries []ObjectRefEntry) ([]common.Hash, error) {
	entryIndex := -1
	for i, entry := range entries {
		if entry.ObjectID == objectID {
			entryIndex = i
			break
		}
	}

	if entryIndex == -1 {
		return nil, fmt.Errorf("objectID %s not found in meta-shard entries", objectID.Hex())
	}

	// Convert entries to BMT key-value pairs format.
	kvPairs := make([][2][]byte, 0, len(entries))
	for _, entry := range entries {
		objectRefBytes := entry.ObjectRef.Serialize()
		kvPairs = append(kvPairs, [2][]byte{
			entry.ObjectID.Bytes(),
			objectRefBytes,
		})
	}

	metaShardTree := trie.NewMerkleTree(kvPairs)
	computedRoot := metaShardTree.GetRoot()
	if computedRoot != common.BytesToHash(merkleRoot[:]) {
		return nil, fmt.Errorf("meta-shard BMT root mismatch: expected %x, computed %x",
			merkleRoot, computedRoot)
	}

	proofBytes, err := metaShardTree.GetPath(objectID.Bytes())
	if err != nil {
		return nil, fmt.Errorf("failed to generate BMT proof for objectID %s: %v", objectID.Hex(), err)
	}

	proof := make([]common.Hash, len(proofBytes))
	for i, proofByte := range proofBytes {
		if len(proofByte) != 32 {
			return nil, fmt.Errorf("invalid proof hash length: expected 32, got %d", len(proofByte))
		}
		copy(proof[i][:], proofByte)
	}

	log.Debug(log.SDB, "Generated meta-shard BMT inclusion proof",
		"objectID", objectID.Hex(),
		"entryIndex", entryIndex,
		"totalEntries", len(entries),
		"proofLength", len(proof),
		"merkleRoot", fmt.Sprintf("%x", merkleRoot))

	return proof, nil
}
