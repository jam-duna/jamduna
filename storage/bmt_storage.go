package storage

import (
	"fmt"
	"runtime"

	evmtypes "github.com/jam-duna/jamduna/types"
	"github.com/jam-duna/jamduna/common"
	log "github.com/jam-duna/jamduna/log"
	"github.com/jam-duna/jamduna/types"
)

const (
	debug        = "trie"
	stateKeySize = 32
)

func normalizeKey32(src []byte) common.Hash {
	var hash common.Hash
	copy(hash[:], src)
	return hash
}

func (t *StorageHub) SetRoot(root common.Hash) error {
	t.mu.RLock()
	currentRoot := t.Root
	t.mu.RUnlock()

	var caller string
	if pc, _, line, ok := runtime.Caller(1); ok {
		caller = fmt.Sprintf("%s:%d", runtime.FuncForPC(pc).Name(), line)
	}
	log.Debug(log.SDB, "SetRoot called", "ptr", fmt.Sprintf("%p", t), "from", currentRoot.Hex(), "to", root.Hex(), "caller", caller)

	if currentRoot == root {
		t.Session.JAM.Rollback()
		return nil
	}

	if err := t.Session.JAM.SetRoot(root); err != nil {
		log.Error(log.SDB, "SetRoot failed", "ptr", fmt.Sprintf("%p", t), "from", currentRoot.Hex(), "to", root.Hex(), "err", err)
		return fmt.Errorf("failed to set trie root to %s: %w", root.Hex(), err)
	}

	t.mu.Lock()
	t.Root = root
	t.mu.Unlock()
	return nil
}

func (t *StorageHub) GetRoot() common.Hash {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.Root
}

func (t *StorageHub) ClearStagedOps() {
	t.Session.JAM.Rollback()
}

// OverlayRoot computes the root hash if staged operations were applied, without committing.
func (t *StorageHub) OverlayRoot() (common.Hash, error) {
	trie := t.Session.JAM.Trie()

	trie.stagedMutex.Lock()
	numOps := len(trie.stagedInserts) + len(trie.stagedDeletes)
	if numOps == 0 {
		currentRoot := trie.GetRoot()
		trie.stagedMutex.Unlock()
		return currentRoot, nil
	}

	deletes := make(map[common.Hash]bool)
	inserts := make(map[common.Hash][]byte)
	for k, v := range trie.stagedDeletes {
		deletes[k] = v
	}
	for k, v := range trie.stagedInserts {
		inserts[k] = make([]byte, len(v))
		copy(inserts[k], v)
	}
	trie.stagedMutex.Unlock()

	tempTree := &MerkleTree{
		Root:          trie.Root,
		backend:       &JAMTrieBackend{writeBatch: t.Session.JAM.Backend().CopyWriteBatch()},
		stagedInserts: make(map[common.Hash][]byte),
		stagedDeletes: make(map[common.Hash]bool),
	}

	tempTree.treeMutex.Lock()
	for key := range deletes {
		if tempTree.Root != nil {
			tempTree.Root = tempTree.deleteNode(tempTree.Root, key[:], 0)
		}
	}
	for key, value := range inserts {
		keySlice := key[:]
		_, err := tempTree.findNode(tempTree.Root, keySlice, 0)
		encodedLeaf := trieLeaf(keySlice, value, tempTree)
		tempTree.levelDBSetLeaf(encodedLeaf, value, keySlice)
		if err != nil {
			if tempTree.Root == nil {
				tempTree.Root = &Node{Hash: trieComputeHash(encodedLeaf), Key: keySlice}
			} else {
				tempTree.Root = tempTree.insertNode(tempTree.Root, keySlice, value, 0)
			}
		} else {
			tempTree.updateTree(tempTree.Root, keySlice, value, 0)
		}
	}
	tempTree.treeMutex.Unlock()
	return tempTree.GetRoot(), nil
}

func (t *StorageHub) Flush() (common.Hash, error) {
	newRoot, err := t.Session.JAM.Finish()
	if err != nil {
		return common.Hash{}, fmt.Errorf("trie commit failed: %v", err)
	}
	t.mu.Lock()
	t.Root = newRoot
	t.mu.Unlock()
	return newRoot, nil
}
func (t *StorageHub) Commit() (common.Hash, error) {
	return t.GetRoot(), nil
}

func (t *StorageHub) Close() error {
	return nil
}

// NewSession creates an isolated session with its own root and staged operations.
// Shares backend/managers (thread-safe), gets fresh UBT context and witness cache.
func (t *StorageHub) NewSession() types.StorageSession {
	newJAMSession := t.Session.JAM.CopySession()
	return &StorageHub{
		Persist:           t.Persist,
		Shared:            t.Shared,
		Session:           SessionContexts{JAM: newJAMSession, UBT: t.Session.UBT.Clone()},
		Witness:           WitnessCache{Storageshard: make(map[common.Address]evmtypes.ContractStorage), Code: make(map[common.Address][]byte), UBTProofs: make(map[common.Address]evmtypes.UBTMultiproof)},
		Infra:             t.Infra,
		CheckpointManager: t.CheckpointManager,
		Root:              newJAMSession.Root(),
		keys:              make(map[common.Hash]bool),
		isSessionCopy:     true,
	}
}

func (t *StorageHub) Insert(keyBytes []byte, value []byte) {
	key := normalizeKey32(keyBytes)
	t.Session.JAM.Insert(key[:], value)
	t.mu.Lock()
	t.keys[key] = true
	t.mu.Unlock()
}

func (t *StorageHub) Get(keyBytes []byte) ([]byte, bool, error) {
	key := normalizeKey32(keyBytes)
	value, ok, err := t.Session.JAM.Get(key[:])
	if err != nil {
		return nil, false, fmt.Errorf("trie get failed: %v", err)
	}
	if !ok {
		return nil, false, nil
	}
	return value, true, nil
}

func (t *StorageHub) Delete(keyBytes []byte) error {
	key := normalizeKey32(keyBytes)
	_, ok, err := t.Get(key[:])
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("key not found: cannot delete non-existent key %x", key)
	}

	if err := t.Session.JAM.Delete(key[:]); err != nil {
		return fmt.Errorf("trie delete failed for key %x: %v", key, err)
	}
	return nil
}

func (t *StorageHub) GetStateByRange(startKey []byte, endKey []byte, maximumSize uint32) ([]types.StateKeyValue, [][]byte, error) {
	return t.Session.JAM.Trie().GetStateByRange(startKey, endKey, maximumSize)
}

// GetAllKeyValues retrieves all key-value pairs from the trie.
// This iterates the entire trie rather than relying on the keys map,
// ensuring we get all keys even after Copy() which creates a new session with empty keys map.
func (t *StorageHub) GetAllKeyValues() []types.KeyVal {
	// Use GetStateByRange with full range (0x00...00 to 0xff...ff) to get all keys
	startKey := make([]byte, 31)
	endKey := make([]byte, 31)
	for i := range endKey {
		endKey[i] = 0xff
	}

	// Use maximum size to get all keys (0xffffffff = 4,294,967,295)
	stateKVs, _, err := t.GetStateByRange(startKey, endKey, 0xffffffff)
	if err != nil {
		// Fall back to empty result on error
		return []types.KeyVal{}
	}

	// Convert StateKeyValue to KeyVal format
	result := make([]types.KeyVal, 0, len(stateKVs))
	for _, skv := range stateKVs {
		var kv types.KeyVal
		copy(kv.Key[:], skv.Key[:])
		kv.Value = skv.Value
		result = append(result, kv)
	}

	return result
}

// GetAllKeyValuesPaged collects all key/value pairs using paged iteration.
// pageSize of 0 defaults to 1024 entries per page.
func (t *StorageHub) GetAllKeyValuesPaged(pageSize uint32) ([]types.KeyVal, error) {
	iter := t.GetStatePageIterator(pageSize)
	var all []types.KeyVal
	for {
		page, ok, err := iter.Next()
		if err != nil {
			return nil, err
		}
		if !ok {
			break
		}
		all = append(all, page...)
	}
	return all, nil
}

// StatePageIterator streams key/value pairs from the trie in ascending order.
type StatePageIterator struct {
	hub      *StorageHub
	pageSize uint32
	nextKey  []byte
	done     bool
}

// GetStatePageIterator returns an iterator over trie key/values.
// pageSize of 0 defaults to 1024.
func (t *StorageHub) GetStatePageIterator(pageSize uint32) *StatePageIterator {
	if pageSize == 0 {
		pageSize = 1024
	}
	startKey := make([]byte, 31) // 0x00...00
	return &StatePageIterator{
		hub:      t,
		pageSize: pageSize,
		nextKey:  startKey,
	}
}

// Next returns the next page of key/values. ok=false means iteration is done.
func (it *StatePageIterator) Next() ([]types.KeyVal, bool, error) {
	if it == nil || it.done {
		return nil, false, nil
	}

	endKey := make([]byte, 31)
	for i := range endKey {
		endKey[i] = 0xff
	}

	stateKVs, _, err := it.hub.GetStateByRange(it.nextKey, endKey, it.pageSize)
	if err != nil {
		it.done = true
		return nil, false, err
	}
	if len(stateKVs) == 0 {
		it.done = true
		return nil, false, nil
	}

	page := make([]types.KeyVal, 0, len(stateKVs))
	for _, skv := range stateKVs {
		var kv types.KeyVal
		copy(kv.Key[:], skv.Key[:])
		kv.Value = skv.Value
		page = append(page, kv)
	}

	lastKey := stateKVs[len(stateKVs)-1].Key
	if next, ok := incrementKey(lastKey[:]); ok {
		it.nextKey = next
	} else {
		it.done = true
	}
	return page, true, nil
}

// incrementKey returns the next lexicographic 31-byte key.
// Returns ok=false if the key overflows (i.e., was all 0xff).
func incrementKey(key []byte) ([]byte, bool) {
	if len(key) != 31 {
		return nil, false
	}
	next := make([]byte, 31)
	copy(next, key)
	for i := len(next) - 1; i >= 0; i-- {
		if next[i] != 0xff {
			next[i]++
			for j := i + 1; j < len(next); j++ {
				next[j] = 0x00
			}
			return next, true
		}
	}
	return nil, false
}

// ForEachKeyValue streams key/value pairs without accumulating them in memory.
// pageSize of 0 defaults to 1024 entries per page.
func (t *StorageHub) ForEachKeyValue(pageSize uint32, fn func([]types.KeyVal) error) error {
	iter := t.GetStatePageIterator(pageSize)
	for {
		page, ok, err := iter.Next()
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		if err := fn(page); err != nil {
			return err
		}
	}
}

func (s *StorageHub) GetKeyValues(keys []common.Hash) []types.KeyVal {
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
