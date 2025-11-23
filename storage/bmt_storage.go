package storage

import (
	"fmt"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

const (
	debug        = "trie"
	stateKeySize = 32 // the "actual" size is 31 but we use common.Hash with the 32 byte being 0 INCLUDING IN METADATA right now
)

// normalizeKey32 ensures all keys are 32 bytes (zero-padded on right if shorter, truncated from right if longer)
// Note we cannot use common.BytesToHash because it pads on the left
func normalizeKey32(src []byte) common.Hash {
	var hash common.Hash
	copy(hash[:], src)
	return hash
}

func (t *StateDBStorage) SetRoot(root common.Hash) error {
	currentRoot := t.GetRoot()
	if currentRoot == root {
		return nil
	}

	return nil
}

func (t *StateDBStorage) GetRoot() common.Hash {
	return t.Root
}

func (t *StateDBStorage) OverlayRoot() (common.Hash, error) {
	// Create a temporary tree clone to compute overlay root without committing
	// Check if there are any staged operations
	t.trieDB.stagedMutex.Lock()
	numOps := len(t.trieDB.stagedInserts) + len(t.trieDB.stagedDeletes)

	if numOps == 0 {
		// No staged operations, return current root
		currentRoot := t.trieDB.GetRoot()
		t.trieDB.stagedMutex.Unlock()
		return currentRoot, nil
	}

	// Take a snapshot of staged operations
	deletes := make(map[common.Hash]bool)
	inserts := make(map[common.Hash][]byte)
	for k, v := range t.trieDB.stagedDeletes {
		deletes[k] = v
	}
	for k, v := range t.trieDB.stagedInserts {
		inserts[k] = make([]byte, len(v))
		copy(inserts[k], v)
	}
	t.trieDB.stagedMutex.Unlock()

	// Create a temporary tree with current state
	tempTree := &MerkleTree{
		Root:          t.trieDB.Root, // Start with current tree state
		writeBatch:    make(map[common.Hash][]byte),
		stagedInserts: make(map[common.Hash][]byte),
		stagedDeletes: make(map[common.Hash]bool),
	}

	// Copy any existing write batch
	t.trieDB.batchMutex.Lock()
	for k, v := range t.trieDB.writeBatch {
		tempTree.writeBatch[k] = make([]byte, len(v))
		copy(tempTree.writeBatch[k], v)
	}
	t.trieDB.batchMutex.Unlock()

	// Apply staged operations to temp tree
	tempTree.treeMutex.Lock()

	// Process deletes first
	for key := range deletes {
		if tempTree.Root != nil {
			tempTree.Root = tempTree.deleteNode(tempTree.Root, key[:], 0)
		}
	}

	// Process inserts/updates
	for key, value := range inserts {
		keySlice := key[:]

		_, err := tempTree.findNode(tempTree.Root, keySlice, 0)
		encodedLeaf := trieLeaf(keySlice, value, tempTree)
		tempTree.levelDBSetLeaf(encodedLeaf, value, keySlice)

		if err != nil {
			// Insert new
			if tempTree.Root == nil {
				tempTree.Root = &Node{
					Hash: trieComputeHash(encodedLeaf),
					Key:  keySlice,
				}
			} else {
				tempTree.Root = tempTree.insertNode(tempTree.Root, keySlice, value, 0)
			}
		} else {
			// Update existing
			tempTree.updateTree(tempTree.Root, keySlice, value, 0)
		}
	}
	tempTree.treeMutex.Unlock()

	// Return the computed root
	return tempTree.GetRoot(), nil
}

// Flush commits all staged changes and returns the new root hash
func (t *StateDBStorage) Flush() (common.Hash, error) {

	// Commit current trie state to get the actual root hash
	newRoot, err := t.trieDB.Flush()
	if err != nil {
		return common.Hash{}, fmt.Errorf("trie commit failed: %v", err)
	}

	// Update the root
	t.Root = newRoot

	return t.Root, nil
}

// Commit returns the current root (changes are already committed by Flush)
func (t *StateDBStorage) Commit() (common.Hash, error) {
	// Changes are already committed by Flush, just return current root
	return t.Root, nil
}

func (t *StateDBStorage) Close() error {
	// Close the trie database (trie doesn't need explicit close)
	if t.trieDB != nil {
		return nil
	}
	return nil
}

// Insert applies the operation directly to trie
func (t *StateDBStorage) Insert(keyBytes []byte, value []byte) {
	key := normalizeKey32(keyBytes)

	// Apply directly to trie
	t.trieDB.Insert(key[:], value)

	t.keys[key] = true // Track key
}

// Get retrieves the value directly from trie
func (t *StateDBStorage) Get(keyBytes []byte) ([]byte, bool, error) {
	key := normalizeKey32(keyBytes)

	value, ok, err := t.trieDB.Get(key[:])
	if err != nil {
		return nil, false, fmt.Errorf("trie get failed: %v", err)
	}

	if !ok {
		// Not found
		return nil, false, nil
	}

	// Found
	return value, true, nil
}

// Delete applies deletion directly to trie
func (t *StateDBStorage) Delete(keyBytes []byte) error {
	key := normalizeKey32(keyBytes)

	// Check if the key exists before deletion
	_, ok, err := t.Get(key[:])
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("key not found: cannot delete non-existent key %x", key)
	}

	// Apply deletion directly to trie
	if err := t.trieDB.Delete(key[:]); err != nil {
		return fmt.Errorf("trie delete failed for key %x: %v", key, err)
	}

	return nil
}

// GetStateByRange retrieves key-value pairs within a range
func (t *StateDBStorage) GetStateByRange(startKey []byte, endKey []byte, maximumSize uint32) ([]types.StateKeyValue, [][]byte, error) {
	return t.trieDB.GetStateByRange(startKey, endKey, maximumSize)
}

// GetAllKeyValues retrieves values for a list of keys from the trie
// If no keys are provided, it defaults to retrieving C1-C16 state keys (0x01-0x10)
func (t *StateDBStorage) GetAllKeyValues() []types.KeyVal {

	// Retrieve values for provided keys
	result := make([]types.KeyVal, 0, len(t.keys))
	for key := range t.keys {
		value, ok, err := t.Get(key[:])
		if err == nil && ok && value != nil {
			var kv types.KeyVal
			copy(kv.Key[:], key[:31])
			kv.Value = value
			result = append(result, kv)
		}
	}

	return result
}

// GetKeyValues retrieves values for a list of keys from the trie
// If no keys are provided, it defaults to retrieving C1-C16 state keys (0x01-0x10)
func (s *StateDBStorage) GetKeyValues(keys []common.Hash) []types.KeyVal {

	// Retrieve values for provided keys
	result := make([]types.KeyVal, 0, len(keys))
	for _, key := range keys {
		value, ok, err := s.Get(key[:])
		if err == nil && ok && value != nil {
			var kv types.KeyVal
			copy(kv.Key[:], key[:31])
			kv.Value = value
			result = append(result, kv)
		}
	}

	return result
}
