package storage

import (
	"encoding/binary"
	"fmt"
	"math/big"

	evmtypes "github.com/colorfulnotion/jam/builder/evm/types"
	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/ethereum/go-ethereum/crypto"
)

// CompareWriteKeySets checks that the builder's post-proof keys match the guarantor's write keys.
// Values are ignored; this is meant to close the "missing key-set validation" gap.
func CompareWriteKeySets(builderWriteKeys [][32]byte, guarantorWrites map[[32]byte][32]byte) error {
	builderSet := make(map[[32]byte]struct{}, len(builderWriteKeys))
	for _, key := range builderWriteKeys {
		builderSet[key] = struct{}{}
	}

	if len(builderSet) != len(guarantorWrites) {
		return fmt.Errorf("Write set size mismatch: builder=%d guarantor=%d", len(builderSet), len(guarantorWrites))
	}

	for key := range builderSet {
		if _, ok := guarantorWrites[key]; !ok {
			return fmt.Errorf("Builder wrote key %x but guarantor didn't", key[:8])
		}
	}
	for key := range guarantorWrites {
		if _, ok := builderSet[key]; !ok {
			return fmt.Errorf("Guarantor wrote key %x but builder didn't include it in proof", key[:8])
		}
	}

	return nil
}

// KeyMetadata stores metadata for a tree key.
type KeyMetadata struct {
	KeyType    uint8    // 0=BasicData, 1=CodeHash, 2=CodeChunk, 3=Storage
	Address    [20]byte // Contract address
	Extra      uint64   // Additional data (chunk_id for code, etc.)
	StorageKey [32]byte // Full 32-byte storage key (only for KeyType=3)
	TxIndex    uint32   // Transaction index within work package (0=pre-exec, 1..n=txs)
}

// ApplyContractWrites applies all contract writes to the current UBT tree.
// This includes code deployments and storage updates.
func (store *StateDBStorage) ApplyContractWrites(blob []byte) error {
	if len(blob) == 0 {
		return nil
	}

	if store.CurrentUBT == nil {
		return fmt.Errorf("no current UBT tree available")
	}

	offset := 0

	for offset < len(blob) {
		// Parse header
		if len(blob)-offset < 29 {
			return fmt.Errorf("invalid contract witness blob: insufficient header bytes at offset %d", offset)
		}

		address := blob[offset : offset+20]
		kind := blob[offset+20]
		payloadLength := uint32(blob[offset+21]) | uint32(blob[offset+22])<<8 |
			uint32(blob[offset+23])<<16 | uint32(blob[offset+24])<<24
		_ = uint32(blob[offset+25]) | uint32(blob[offset+26])<<8 |
			uint32(blob[offset+27])<<16 | uint32(blob[offset+28])<<24 // txIndex (unused here, but needed for blob parsing)
		offset += 29

		if len(blob)-offset < int(payloadLength) {
			return fmt.Errorf("invalid contract witness blob: insufficient payload bytes at offset %d", offset)
		}

		payload := blob[offset : offset+int(payloadLength)]
		offset += int(payloadLength)

		// Apply writes based on kind
		switch kind {
		case 0x00: // Code
			if err := applyCodeWrites(address, payload, store.CurrentUBT); err != nil {
				return fmt.Errorf("failed to apply code writes: %v", err)
			}

		case 0x01: // Storage shard
			if err := applyStorageWrites(address, payload, store.CurrentUBT); err != nil {
				return fmt.Errorf("failed to apply storage writes: %v", err)
			}

		case 0x02: // Balance
			if err := applyBalanceWrites(address, payload, store.CurrentUBT); err != nil {
				return fmt.Errorf("failed to apply balance writes: %v", err)
			}

		case 0x06: // Nonce
			if err := applyNonceWrites(address, payload, store.CurrentUBT); err != nil {
				return fmt.Errorf("failed to apply nonce writes: %v", err)
			}

		default:
			log.Warn(log.EVM, "ApplyContractWrites: unknown kind", "kind", kind)
		}
	}

	return nil
}

// StorePendingWrites stores contract writes for deferred application.
// Writes are keyed by work package hash and applied only when OnAccumulated fires.
// This prevents CurrentUBT from advancing speculatively before bundle accumulation.
func (store *StateDBStorage) StorePendingWrites(wpHash common.Hash, blob []byte) {
	if len(blob) == 0 {
		return
	}

	store.pendingWritesMutex.Lock()
	defer store.pendingWritesMutex.Unlock()

	// Make a copy of the blob to avoid aliasing issues
	blobCopy := make([]byte, len(blob))
	copy(blobCopy, blob)
	store.pendingWrites[wpHash] = blobCopy

	log.Info(log.EVM, "StorePendingWrites: stored deferred writes",
		"wpHash", wpHash.Hex(),
		"blobSize", len(blob),
		"pendingCount", len(store.pendingWrites))
}

// ApplyPendingWrites applies stored contract writes for the given work package hash.
// Called when OnAccumulated fires to commit the bundle's state changes to CurrentUBT.
// Returns true if writes were found and applied, false if no pending writes existed.
func (store *StateDBStorage) ApplyPendingWrites(wpHash common.Hash) (bool, error) {
	store.pendingWritesMutex.Lock()
	blob, exists := store.pendingWrites[wpHash]
	if exists {
		delete(store.pendingWrites, wpHash)
	}
	store.pendingWritesMutex.Unlock()

	if !exists {
		log.Debug(log.EVM, "ApplyPendingWrites: no pending writes found",
			"wpHash", wpHash.Hex())
		return false, nil
	}

	log.Info(log.EVM, "ApplyPendingWrites: applying deferred writes",
		"wpHash", wpHash.Hex(),
		"blobSize", len(blob))

	if err := store.ApplyContractWrites(blob); err != nil {
		return false, fmt.Errorf("failed to apply pending writes for %s: %w", wpHash.Hex(), err)
	}

	return true, nil
}

// DiscardPendingWrites removes pending writes without applying them.
// Called when a bundle fails (OnFailed) to clean up without modifying CurrentUBT.
func (store *StateDBStorage) DiscardPendingWrites(wpHash common.Hash) bool {
	store.pendingWritesMutex.Lock()
	defer store.pendingWritesMutex.Unlock()

	_, exists := store.pendingWrites[wpHash]
	if exists {
		delete(store.pendingWrites, wpHash)
		log.Info(log.EVM, "DiscardPendingWrites: discarded writes for failed bundle",
			"wpHash", wpHash.Hex(),
			"remainingPending", len(store.pendingWrites))
	}
	return exists
}

// GetPendingWritesCount returns the number of pending write blobs.
// Useful for debugging and monitoring.
func (store *StateDBStorage) GetPendingWritesCount() int {
	store.pendingWritesMutex.RLock()
	defer store.pendingWritesMutex.RUnlock()
	return len(store.pendingWrites)
}

// applyContractWritesToTree applies all contract writes to the given UBT tree.
func applyContractWritesToTree(blob []byte, tree *UnifiedBinaryTree) error {
	if len(blob) == 0 {
		return nil
	}

	offset := 0

	for offset < len(blob) {
		// Parse header
		if len(blob)-offset < 29 {
			return fmt.Errorf("invalid contract witness blob: insufficient header bytes at offset %d", offset)
		}

		address := blob[offset : offset+20]
		kind := blob[offset+20]
		payloadLength := uint32(blob[offset+21]) | uint32(blob[offset+22])<<8 |
			uint32(blob[offset+23])<<16 | uint32(blob[offset+24])<<24
		_ = uint32(blob[offset+25]) | uint32(blob[offset+26])<<8 |
			uint32(blob[offset+27])<<16 | uint32(blob[offset+28])<<24 // txIndex (unused here, but needed for blob parsing)
		offset += 29

		if len(blob)-offset < int(payloadLength) {
			return fmt.Errorf("invalid contract witness blob: insufficient payload bytes at offset %d", offset)
		}

		payload := blob[offset : offset+int(payloadLength)]
		offset += int(payloadLength)

		// Apply writes based on kind
		switch kind {
		case 0x00: // Code
			if err := applyCodeWrites(address, payload, tree); err != nil {
				return fmt.Errorf("failed to apply code writes: %v", err)
			}

		case 0x01: // Storage shard
			if err := applyStorageWrites(address, payload, tree); err != nil {
				return fmt.Errorf("failed to apply storage writes: %v", err)
			}

		case 0x02: // Balance
			if err := applyBalanceWrites(address, payload, tree); err != nil {
				return fmt.Errorf("failed to apply balance writes: %v", err)
			}

		case 0x06: // Nonce
			if err := applyNonceWrites(address, payload, tree); err != nil {
				return fmt.Errorf("failed to apply nonce writes: %v", err)
			}

		default:
			log.Warn(log.EVM, "applyContractWritesToTree: unknown kind", "kind", kind)
		}
	}

	return nil
}

// applyStorageWrites applies storage writes to the UBT tree.
func applyStorageWrites(address []byte, payload []byte, tree *UnifiedBinaryTree) error {
	entries, err := evmtypes.ParseShardPayload(payload)
	if err != nil {
		return fmt.Errorf("failed to parse shard payload: %v", err)
	}

	addr := common.BytesToAddress(address)
	for _, entry := range entries {
		key := GetStorageSlotKey(defaultUBTProfile, addr, entry.KeyH)
		tree.Insert(key, entry.Value)
	}

	return nil
}

// applyCodeWrites applies code writes to the UBT tree.
// This includes: BasicData (code_size), CodeHash, and CodeChunks.
func applyCodeWrites(address []byte, code []byte, tree *UnifiedBinaryTree) error {
	addr := common.BytesToAddress(address)

	// 1. Insert code chunks
	chunks := ChunkifyCode(code)
	numChunks := len(chunks) / 32
	for chunkID := uint64(0); chunkID < uint64(numChunks); chunkID++ {
		start := int(chunkID * 32)
		var chunk [32]byte
		copy(chunk[:], chunks[start:start+32])
		chunkKey := GetCodeChunkKey(defaultUBTProfile, addr, chunkID)
		tree.InsertRaw(chunkKey, chunk)
	}

	// 2. Insert code hash
	var codeHash [32]byte
	if len(code) == 0 {
		codeHash = emptyCodeHash()
	} else {
		hash := crypto.Keccak256(code)
		copy(codeHash[:], hash)
	}
	codeHashKey := GetCodeHashKey(defaultUBTProfile, addr)
	tree.Insert(codeHashKey, codeHash)

	// 3. Update BasicData with code_size
	basicDataKey := GetBasicDataKey(defaultUBTProfile, addr)
	basicData := BasicDataLeaf{}
	if existing, found, _ := tree.Get(&basicDataKey); found {
		basicData = DecodeBasicDataLeaf(existing)
	}
	basicData.CodeSize = uint32(len(code))
	tree.Insert(basicDataKey, basicData.Encode())

	return nil
}

// applyBalanceWrites applies balance writes to the UBT tree.
func applyBalanceWrites(address []byte, payload []byte, tree *UnifiedBinaryTree) error {
	if len(payload) != 16 {
		return fmt.Errorf("invalid balance payload: expected 16 bytes, got %d", len(payload))
	}

	addr := common.BytesToAddress(address)
	basicDataKey := GetBasicDataKey(defaultUBTProfile, addr)
	basicData := BasicDataLeaf{}
	if existing, found, _ := tree.Get(&basicDataKey); found {
		basicData = DecodeBasicDataLeaf(existing)
	}
	copy(basicData.Balance[:], payload[:16])
	tree.Insert(basicDataKey, basicData.Encode())

	log.Info(log.EVM, "applyBalanceWrites: inserted balance",
		"address", addr.Hex(),
		"balance", new(big.Int).SetBytes(payload[:16]).String())

	return nil
}

// applyNonceWrites applies nonce writes to the UBT tree.
func applyNonceWrites(address []byte, payload []byte, tree *UnifiedBinaryTree) error {
	if len(payload) != 8 {
		return fmt.Errorf("invalid nonce payload: expected 8 bytes, got %d", len(payload))
	}

	addr := common.BytesToAddress(address)
	basicDataKey := GetBasicDataKey(defaultUBTProfile, addr)
	basicData := BasicDataLeaf{}
	if existing, found, _ := tree.Get(&basicDataKey); found {
		basicData = DecodeBasicDataLeaf(existing)
	}
	basicData.Nonce = binary.BigEndian.Uint64(payload)
	tree.Insert(basicDataKey, basicData.Encode())

	return nil
}

// ===== Multi-Snapshot UBT System =====
//
// For parallel bundle building, each bundle needs its own UBT snapshot that chains
// from the previous bundle's post-state. This prevents the "CurrentUBT drift" problem
// where bundles built in parallel all read from the same canonical state S0, making
// their state transitions incompatible when applied sequentially.
//
// Flow:
//   1. CreateSnapshotForBlock(blockNumber) - Creates a snapshot chained from previous block
//   2. SetActiveSnapshot(blockNumber) - Sets which snapshot to use for reads
//   3. ApplyContractWritesToActiveSnapshot(blob) - Apply writes to the active snapshot
//   4. CommitSnapshot(blockNumber) - On OnAccumulated: Commit snapshot to canonical
//   5. InvalidateSnapshotsFrom(blockNumber) - On OnFailed: Discard snapshot and descendants

// CreateSnapshotForBlock creates a new UBT snapshot for the given block number.
// The snapshot is cloned from:
//   - The previous block's pending snapshot if it exists (chaining)
//   - Otherwise from CurrentUBT (canonical state)
//
// If a snapshot for this block already exists, this is a no-op (idempotent).
func (store *StateDBStorage) CreateSnapshotForBlock(blockNumber uint64) error {
	store.pendingSnapshotsMutex.Lock()
	defer store.pendingSnapshotsMutex.Unlock()

	if _, exists := store.pendingSnapshots[blockNumber]; exists {
		log.Info(log.EVM, "CreateSnapshotForBlock: snapshot already exists, reusing",
			"blockNumber", blockNumber)
		return nil
	}

	// Find parent snapshot: previous block's pending snapshot or CurrentUBT
	var parentTree *UnifiedBinaryTree
	if blockNumber > 0 {
		if parentSnapshot, exists := store.pendingSnapshots[blockNumber-1]; exists {
			parentTree = parentSnapshot
			log.Info(log.EVM, "CreateSnapshotForBlock: chaining from previous pending snapshot",
				"blockNumber", blockNumber,
				"parentBlock", blockNumber-1)
		}
	}
	if parentTree == nil {
		// No pending parent snapshot - use CurrentUBT.
		// This is safe because we never delete snapshots, so if the parent existed
		// it would have been found above. If it doesn't exist, CurrentUBT is correct
		// (either this is block 1 cloning from genesis, or the parent was never created).
		parentTree = store.CurrentUBT
		log.Info(log.EVM, "CreateSnapshotForBlock: cloning from canonical CurrentUBT",
			"blockNumber", blockNumber)
	}

	if parentTree == nil {
		return fmt.Errorf("no parent tree available for snapshot at block %d", blockNumber)
	}

	// Clone the parent tree
	snapshot := parentTree.Copy()
	store.pendingSnapshots[blockNumber] = snapshot

	// Maintain ordered list of snapshot block numbers
	store.snapshotOrder = insertSorted(store.snapshotOrder, blockNumber)

	log.Info(log.EVM, "CreateSnapshotForBlock: created snapshot",
		"blockNumber", blockNumber,
		"pendingCount", len(store.pendingSnapshots))

	return nil
}

// SetActiveSnapshot sets which snapshot to use for subsequent reads.
// Pass blockNumber=0 to read from canonical CurrentUBT.
func (store *StateDBStorage) SetActiveSnapshot(blockNumber uint64) error {
	if blockNumber == 0 {
		store.pendingSnapshotsMutex.Lock()
		store.activeSnapshotBlock = 0
		store.pendingSnapshotsMutex.Unlock()
		log.Debug(log.EVM, "SetActiveSnapshot: using canonical CurrentUBT")
		return nil
	}

	store.pendingSnapshotsMutex.RLock()
	_, exists := store.pendingSnapshots[blockNumber]
	store.pendingSnapshotsMutex.RUnlock()

	if !exists {
		return fmt.Errorf("no snapshot exists for block %d", blockNumber)
	}

	store.pendingSnapshotsMutex.Lock()
	store.activeSnapshotBlock = blockNumber
	store.pendingSnapshotsMutex.Unlock()

	log.Debug(log.EVM, "SetActiveSnapshot: using snapshot",
		"blockNumber", blockNumber)
	return nil
}

// ClearActiveSnapshot resets to reading from canonical CurrentUBT.
func (store *StateDBStorage) ClearActiveSnapshot() {
	store.pendingSnapshotsMutex.Lock()
	store.activeSnapshotBlock = 0
	store.pendingSnapshotsMutex.Unlock()
}

// GetActiveTree returns the UBT tree that should be used for reads.
// Returns the active snapshot if set, otherwise CurrentUBT.
func (store *StateDBStorage) GetActiveTree() *UnifiedBinaryTree {
	store.pendingSnapshotsMutex.RLock()
	activeBlock := store.activeSnapshotBlock
	if activeBlock > 0 {
		if snapshot, exists := store.pendingSnapshots[activeBlock]; exists {
			store.pendingSnapshotsMutex.RUnlock()
			return snapshot
		}
	}
	store.pendingSnapshotsMutex.RUnlock()
	return store.CurrentUBT
}

// ApplyWritesToActiveSnapshot applies contract writes to the currently active snapshot.
// If no snapshot is active (activeSnapshotBlock=0), returns an error since writes
// should only be applied to snapshots during parallel building, not to canonical state.
func (store *StateDBStorage) ApplyWritesToActiveSnapshot(blob []byte) error {
	if len(blob) == 0 {
		return nil
	}

	store.pendingSnapshotsMutex.RLock()
	activeBlock := store.activeSnapshotBlock
	if activeBlock == 0 {
		store.pendingSnapshotsMutex.RUnlock()
		return fmt.Errorf("no active snapshot set, cannot apply writes")
	}

	snapshot, exists := store.pendingSnapshots[activeBlock]
	store.pendingSnapshotsMutex.RUnlock()

	if !exists {
		return fmt.Errorf("active snapshot for block %d not found", activeBlock)
	}

	log.Info(log.EVM, "ApplyWritesToActiveSnapshot: applying writes to snapshot",
		"blockNumber", activeBlock,
		"blobSize", len(blob))

	return applyContractWritesToTree(blob, snapshot)
}

// CommitSnapshot commits the snapshot for the given block number to canonical state.
// Called when OnAccumulated fires. The snapshot's state becomes the new CurrentUBT.
//
// Handles out-of-order accumulation: since snapshots are chained (block N's snapshot
// includes all state from blocks 1..N), if a later block accumulates first, earlier
// blocks' snapshots are already superseded and don't need to be committed.
func (store *StateDBStorage) CommitSnapshot(blockNumber uint64) error {
	store.pendingSnapshotsMutex.Lock()
	defer store.pendingSnapshotsMutex.Unlock()

	// Check if this block is already superseded by a later committed block
	// Since snapshots chain (block N includes all state from 1..N), if block N+k
	// was already committed, block N's changes are already in CurrentUBT
	if blockNumber <= store.lastCommittedSnapshotBlock {
		log.Info(log.EVM, "CommitSnapshot: block already superseded by later commit",
			"blockNumber", blockNumber,
			"lastCommittedBlock", store.lastCommittedSnapshotBlock)
		// Don't delete - keep snapshots for potential rebuilds
		return nil
	}

	snapshot, exists := store.pendingSnapshots[blockNumber]
	if !exists {
		log.Warn(log.EVM, "CommitSnapshot: no snapshot found for block",
			"blockNumber", blockNumber,
			"lastCommittedBlock", store.lastCommittedSnapshotBlock)
		return nil // Not an error - snapshot might have been committed via pending writes path
	}

	// Commit: Set canonical UBT to the snapshot
	store.CurrentUBT = snapshot
	store.lastCommittedSnapshotBlock = blockNumber

	// NOTE: We intentionally do NOT delete snapshots here.
	// Deleting snapshots causes issues when rebuilding bundles out-of-order:
	// if block N+k commits first, deleting snapshots <= N+k makes it impossible
	// to rebuild block N because its parent snapshot is gone.
	// Snapshots are kept in memory for now. Future optimization: add cleanup
	// when we're certain no in-flight bundles need them.

	log.Info(log.EVM, "CommitSnapshot: committed snapshot to canonical",
		"blockNumber", blockNumber,
		"pendingSnapshots", len(store.pendingSnapshots))

	return nil
}

// InvalidateSnapshotsFrom discards all snapshots for blocks >= blockNumber.
// Called when OnFailed fires. Bundles at these block heights need to be rebuilt
// with the correct pre-state.
func (store *StateDBStorage) InvalidateSnapshotsFrom(blockNumber uint64) int {
	store.pendingSnapshotsMutex.Lock()
	defer store.pendingSnapshotsMutex.Unlock()

	invalidated := 0
	for bn := range store.pendingSnapshots {
		if bn >= blockNumber {
			delete(store.pendingSnapshots, bn)
			invalidated++
		}
	}

	// Update snapshotOrder
	store.snapshotOrder = filterLessThan(store.snapshotOrder, blockNumber)

	if invalidated > 0 {
		log.Info(log.EVM, "InvalidateSnapshotsFrom: invalidated snapshots",
			"fromBlock", blockNumber,
			"invalidatedCount", invalidated,
			"remainingSnapshots", len(store.pendingSnapshots))
	}

	return invalidated
}

// InvalidateSnapshot deletes only the snapshot for the specified block number.
// Unlike InvalidateSnapshotsFrom, this does NOT affect snapshots for other blocks.
// Returns true if a snapshot was deleted, false if no snapshot existed for this block.
func (store *StateDBStorage) InvalidateSnapshot(blockNumber uint64) bool {
	store.pendingSnapshotsMutex.Lock()
	defer store.pendingSnapshotsMutex.Unlock()

	if _, exists := store.pendingSnapshots[blockNumber]; !exists {
		return false
	}

	delete(store.pendingSnapshots, blockNumber)
	store.snapshotOrder = removeFromSorted(store.snapshotOrder, blockNumber)

	log.Info(log.EVM, "InvalidateSnapshot: deleted single snapshot",
		"blockNumber", blockNumber,
		"remainingSnapshots", len(store.pendingSnapshots))

	return true
}

// GetSnapshotBlockNumbers returns a sorted list of block numbers with pending snapshots.
// Useful for debugging and monitoring.
func (store *StateDBStorage) GetSnapshotBlockNumbers() []uint64 {
	store.pendingSnapshotsMutex.RLock()
	defer store.pendingSnapshotsMutex.RUnlock()

	result := make([]uint64, len(store.snapshotOrder))
	copy(result, store.snapshotOrder)
	return result
}

// GetPendingSnapshotCount returns the number of pending snapshots.
func (store *StateDBStorage) GetPendingSnapshotCount() int {
	store.pendingSnapshotsMutex.RLock()
	defer store.pendingSnapshotsMutex.RUnlock()
	return len(store.pendingSnapshots)
}

// Helper: Insert value into sorted slice maintaining order
func insertSorted(slice []uint64, val uint64) []uint64 {
	i := 0
	for i < len(slice) && slice[i] < val {
		i++
	}
	// Check if already exists
	if i < len(slice) && slice[i] == val {
		return slice
	}
	// Insert at position i
	slice = append(slice, 0)
	copy(slice[i+1:], slice[i:])
	slice[i] = val
	return slice
}

// Helper: Filter slice to keep only values > threshold
func filterGreaterThan(slice []uint64, threshold uint64) []uint64 {
	result := make([]uint64, 0, len(slice))
	for _, v := range slice {
		if v > threshold {
			result = append(result, v)
		}
	}
	return result
}

// Helper: Filter slice to keep only values < threshold
func filterLessThan(slice []uint64, threshold uint64) []uint64 {
	result := make([]uint64, 0, len(slice))
	for _, v := range slice {
		if v < threshold {
			result = append(result, v)
		}
	}
	return result
}

// Helper: Remove a single value from a sorted slice
func removeFromSorted(slice []uint64, val uint64) []uint64 {
	for i, v := range slice {
		if v == val {
			return append(slice[:i], slice[i+1:]...)
		}
		if v > val {
			break // Not found, sorted slice so no need to continue
		}
	}
	return slice
}
