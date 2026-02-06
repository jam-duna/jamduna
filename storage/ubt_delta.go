package storage

import (
	"fmt"

	evmtypes "github.com/jam-duna/jamduna/types"
)

// ApplyStateDelta applies a flattened key/value delta to a UBT tree.
// The delta format is [key(32B) || value(32B)] entries.
func ApplyStateDelta(tree *UnifiedBinaryTree, delta *evmtypes.UBTStateDelta) (*UnifiedBinaryTree, [32]byte, error) {
	if tree == nil {
		return nil, [32]byte{}, fmt.Errorf("nil UBT tree")
	}
	if delta == nil || delta.NumEntries == 0 {
		return tree, tree.RootHash(), nil
	}

	expectedLen := int(delta.NumEntries) * 64
	if len(delta.Entries) != expectedLen {
		return nil, [32]byte{}, fmt.Errorf("delta length mismatch: got %d, expected %d", len(delta.Entries), expectedLen)
	}

	for i := 0; i < int(delta.NumEntries); i++ {
		offset := i * 64
		var keyBytes [32]byte
		copy(keyBytes[:], delta.Entries[offset:offset+32])
		var value [32]byte
		copy(value[:], delta.Entries[offset+32:offset+64])

		key := TreeKeyFromBytes(keyBytes)
		tree.Insert(key, value)
	}

	root := tree.RootHash()
	return tree, root, nil
}
