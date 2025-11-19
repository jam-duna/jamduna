package storage

import (
	"fmt"

	"github.com/colorfulnotion/jam/bmt"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

// Ensure bmt package is recognized as used
var _ = bmt.DefaultOptions

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

	if _, exists := t.rootToSeqNum[root]; !exists {
		t.Root = root
		return nil
	}
	return t.RollbackToRoot(root)
}

func (t *StateDBStorage) GetRoot() common.Hash {
	return t.Root
}

// OverlayRoot computes the current merkle root from the staged overlay WITHOUT committing to disk.
// This mirrors Rust NOMT's Session::finish() + FinishedSession::root() workflow.
// Use this instead of Flush() when you only need the root hash and don't want to persist yet.
// Critical for: fuzzing, fork handling, and any scenario where multiple candidate states exist.
// Works correctly even after commits by merging committed tree + staged overlay delta.
func (t *StateDBStorage) OverlayRoot() (common.Hash, error) {
	// Get current root from BMT overlay (no disk I/O)
	// This properly merges committed tree state + staged overlay
	currentRoot, err := t.bmtDB.CurrentRoot()
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to compute staged root: %w", err)
	}
	return common.BytesToHash(currentRoot[:]), nil
}

// Flush commits all staged changes and returns the new root hash
func (t *StateDBStorage) Flush() (common.Hash, error) {

	// Commit current BMT state to get the actual root hash
	session, err := t.bmtDB.Commit()
	if err != nil {
		return common.Hash{}, fmt.Errorf("BMT commit failed: %v", err)
	}

	// Get the committed root hash
	newRoot := session.Root()
	t.Root = common.BytesToHash(newRoot[:])


	// Update root history for rollback support
	t.currentSeqNum++
	t.rootHistory = append(t.rootHistory, t.Root)
	t.rootToSeqNum[t.Root] = t.currentSeqNum

	return t.Root, nil
}

// Commit returns the current root (changes are already committed by Flush)
func (t *StateDBStorage) Commit() (common.Hash, error) {
	// Changes are already committed by Flush, just return current root
	return t.Root, nil
}

func (t *StateDBStorage) Close() error {
	// Close the BMT database
	if t.bmtDB != nil {
		return t.bmtDB.Close()
	}
	return nil
}

// Insert applies the operation directly to BMT
func (t *StateDBStorage) Insert(keyBytes []byte, value []byte) {
	key := normalizeKey32(keyBytes)
	var key32 [32]byte
	copy(key32[:], key[:])

	// Apply directly to BMT
	if err := t.bmtDB.Insert(key32, value); err != nil {
		return
	}

	t.keys[key] = true // Track key
}

// Get retrieves the value directly from BMT
func (t *StateDBStorage) Get(keyBytes []byte) ([]byte, bool, error) {
	key := normalizeKey32(keyBytes)
	var key32 [32]byte
	copy(key32[:], key[:])

	value, err := t.bmtDB.Get(key32)
	if err != nil {
		return nil, false, fmt.Errorf("BMT get failed: %v", err)
	}

	if value == nil {
		// Not found
		return nil, false, nil
	}

	// Found
	return value, true, nil
}

// Delete applies deletion directly to BMT
func (t *StateDBStorage) Delete(keyBytes []byte) error {
	key := normalizeKey32(keyBytes)
	var key32 [32]byte
	copy(key32[:], key[:])

	// Check if the key exists before deletion
	_, ok, err := t.Get(key[:])
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("key not found: cannot delete non-existent key %x", key)
	}

	// Apply deletion directly to BMT
	if err := t.bmtDB.Delete(key32); err != nil {
		return fmt.Errorf("BMT delete failed for key %x: %v", key, err)
	}

	return nil
}

// GetStateByRange stub - not implemented for BMT yet
func (t *StateDBStorage) GetStateByRange(startKey []byte, endKey []byte, maximumSize uint32) ([]types.StateKeyValue, [][]byte, error) {
	return nil, nil, fmt.Errorf("GetStateByRange not implemented for BMT yet")
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

// RollbackToRoot rolls back BMT to a specific historical root by hash
// This translates the root hash to a commit count and calls BMT rollback
func (s *StateDBStorage) RollbackToRoot(targetRoot common.Hash) error {
	// Look up the sequence number for this root
	targetSeq, exists := s.rootToSeqNum[targetRoot]
	if !exists {
		return fmt.Errorf("root %s not found in history", targetRoot.Hex())
	}

	if targetSeq > s.currentSeqNum {
		return fmt.Errorf("cannot rollback to future root (target seqnum %d > current %d)", targetSeq, s.currentSeqNum)
	}

	// Calculate how many commits to rollback
	n := s.currentSeqNum - targetSeq
	if n == 0 {
		// Already at target root
		return nil
	}

	// Call BMT rollback - for now, use multiple single rollbacks
	for i := 0; i < int(n); i++ {
		if err := s.bmtDB.Rollback(); err != nil {
			return fmt.Errorf("BMT rollback failed at step %d/%d: %v", i+1, n, err)
		}
	}

	// Update tracking state
	s.currentSeqNum = targetSeq
	s.Root = targetRoot
	// Truncate history to remove rolled-back commits
	s.rootHistory = s.rootHistory[:targetSeq+1]

	return nil
}
