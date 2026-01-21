package storage

import (
	"fmt"
	"runtime"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
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

	// Debug: trace SetRoot calls to detect race conditions on shared sdb
	var caller string
	if pc, _, line, ok := runtime.Caller(1); ok {
		caller = fmt.Sprintf("%s:%d", runtime.FuncForPC(pc).Name(), line)
	}
	log.Debug(log.SDB, "SetRoot called",
		"ptr", fmt.Sprintf("%p", t),
		"from", currentRoot.Hex(),
		"to", root.Hex(),
		"caller", caller)

	if currentRoot == root {
		t.trieDB.ClearStagedOps()
		return nil
	}

	// Use the MerkleTree's SetRoot to reconstruct the tree from the historical root
	if err := t.trieDB.SetRoot(root); err != nil {
		log.Error(log.SDB, "SetRoot failed",
			"ptr", fmt.Sprintf("%p", t),
			"from", currentRoot.Hex(),
			"to", root.Hex(),
			"err", err)
		return fmt.Errorf("failed to set trie root to %s: %w", root.Hex(), err)
	}

	// Update our cached root
	t.Root = root
	return nil
}

func (t *StateDBStorage) GetRoot() common.Hash {
	return t.Root
}

func (t *StateDBStorage) ClearStagedOps() {
	t.trieDB.ClearStagedOps()
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
		Root: t.trieDB.Root, // Start with current tree state
		backend: &sharedTrieBackend{
			writeBatch: make(map[common.Hash][]byte),
		},
		stagedInserts: make(map[common.Hash][]byte),
		stagedDeletes: make(map[common.Hash]bool),
	}

	// Copy any existing write batch
	t.trieDB.backend.batchMutex.Lock()
	for k, v := range t.trieDB.backend.writeBatch {
		tempTree.backend.writeBatch[k] = make([]byte, len(v))
		copy(tempTree.backend.writeBatch[k], v)
	}
	t.trieDB.backend.batchMutex.Unlock()

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

// CloneTrieView creates an isolated view of the storage with its own trie Root pointer.
// The new view shares the committed writeBatch (node data via backend pointer) but has its own:
// - Root pointer (SetRoot won't affect other views)
// - Staged operations (uncommitted changes are isolated)
//
// This enables concurrent operations (auditor, authoring, importing) to work on
// different state roots without race conditions on the shared MerkleTree.Root.
//
// IMPORTANT: The clone shares the MerkleTree's backend (writeBatch + batchMutex) via pointer,
// ensuring synchronized access to committed node data. The clone also shares the LevelDB
// connection and read-only resources.
//
// UBT STATE SHARING: The clone shares the SAME sharedUBTState pointer as the original.
// This includes treeStore, canonicalRoot, and the mutex protecting them.
// This is safe because all access is synchronized via the shared mutex.
//
// PINNED STATE: The clone inherits the current pinnedStateRoot and pinnedTree from the parent.
// This is critical for Phase 2 witness generation where the parent has pinned to a pre-state root
// and the clone (created via NewStateDBFromStateRoot) needs to use that same pinned tree.
//
// The clone does NOT share activeRoot - each clone maintains its own execution context.
//
// Usage: Call this when creating a StateDB copy that needs to operate at a different root.
func (t *StateDBStorage) CloneTrieView() types.JAMStorage {
	// Copy pinnedStateRoot if set (make a copy of the pointer's value)
	var clonedPinnedStateRoot *common.Hash
	t.mutex.RLock()
	if t.pinnedStateRoot != nil {
		rootCopy := *t.pinnedStateRoot
		clonedPinnedStateRoot = &rootCopy
	}
	pinnedTree := t.pinnedTree // Safe to share - tree is immutable once in treeStore
	t.mutex.RUnlock()

	return &StateDBStorage{
		trieDB:        t.trieDB.CloneView(), // Isolated trie view (shares backend for writeBatch)
		Root:          t.Root,
		stagedInserts: make(map[common.Hash][]byte),
		stagedDeletes: make(map[common.Hash]bool),
		keys:          make(map[common.Hash]bool),

		// Shared read-only resources
		db:              t.db, // LevelDB connection - thread-safe
		logChan:         t.logChan,
		jamda:           t.jamda,
		telemetryClient: t.telemetryClient,
		nodeID:          t.nodeID,

		// SHARED UBT state: Clone references the SAME sharedUBTState pointer.
		// This includes treeStore, canonicalRoot, ubtRoots, and their mutex.
		// All access is synchronized via ubtState.mutex.
		ubtState: t.ubtState, // SHARED - same pointer, same mutex

		// Per-clone state - not shared
		activeRoot: common.Hash{}, // Each clone has its own active execution context

		// INHERITED pinned state: Clone gets the parent's pinned tree at clone time.
		// This is essential for Phase 2 witness verification where the parent has pinned
		// to a pre-state root and the clone needs access to that same tree.
		pinnedStateRoot: clonedPinnedStateRoot,
		pinnedTree:      pinnedTree,

		// SHARED service state: Clone references the SAME sharedServiceState pointer.
		// All access is synchronized via serviceState.mutex.
		serviceState: t.serviceState, // SHARED - same pointer, same mutex

		// Mark as clone for debugging/tracking (not for restricting operations anymore)
		isClone: true,

		// UBT read log: Per-clone state for witness generation
		// Clone gets its own empty read log but inherits the enabled flag
		// so reads during Phase 2 are tracked for the pre-witness
		ubtReadLog:        make([]types.UBTRead, 0),
		ubtReadLogEnabled: true, // Enable by default so Phase 2 tracking works
	}
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

// GetAllKeyValues retrieves key-value pairs tracked in the in-memory keys map
// Note: This only returns keys inserted during the current session via the keys map.
// For full trie traversal (including service storage), use GetStateByRange directly.
func (t *StateDBStorage) GetAllKeyValues() []types.KeyVal {
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
