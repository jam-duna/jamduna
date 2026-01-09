package storage

import (
	"fmt"

	"github.com/colorfulnotion/jam/common"
)

// ReadCodeFromTree reconstructs contract code from a UBT tree snapshot.
func ReadCodeFromTree(tree *UnifiedBinaryTree, address common.Address) ([]byte, uint32, error) {
	if tree == nil {
		return nil, 0, nil
	}

	basicKey := GetBasicDataKey(defaultUBTProfile, address)
	value, found, _ := tree.Get(&basicKey)
	if !found {
		return nil, 0, nil
	}

	basicData := DecodeBasicDataLeaf(value)
	codeSize := basicData.CodeSize
	if codeSize == 0 {
		return nil, 0, nil
	}

	numChunks := (codeSize + 30) / 31
	code := make([]byte, 0, codeSize)

	for chunkID := uint64(0); chunkID < uint64(numChunks); chunkID++ {
		chunkKey := GetCodeChunkKey(defaultUBTProfile, address, chunkID)
		chunkData, found, _ := tree.Get(&chunkKey)
		if !found {
			return nil, 0, fmt.Errorf("chunk %d not found", chunkID)
		}

		chunkBytes := chunkData[1:32]
		remaining := int(codeSize) - len(code)
		if remaining < 31 {
			chunkBytes = chunkBytes[:remaining]
		}
		code = append(code, chunkBytes...)
		if len(code) >= int(codeSize) {
			break
		}
	}

	return code, codeSize, nil
}

// ReadStorageFromTree reads a storage slot value from a UBT tree snapshot.
func ReadStorageFromTree(tree *UnifiedBinaryTree, address common.Address, storageKey [32]byte) ([32]byte, bool, error) {
	if tree == nil {
		return [32]byte{}, false, nil
	}

	key := GetStorageSlotKey(defaultUBTProfile, address, storageKey)
	value, found, _ := tree.Get(&key)
	return value, found, nil
}
