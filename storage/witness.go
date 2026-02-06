package storage

import (
	"encoding/binary"
	"fmt"
	"math/big"

	evmtypes "github.com/jam-duna/jamduna/types"
	"github.com/jam-duna/jamduna/common"
	log "github.com/jam-duna/jamduna/log"
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
// ROOT-FIRST FLOW (preferred):
//   1. CreateSnapshotFromRoot(parentRoot) - Creates snapshot cloned from specific root
//   2. SetActiveRoot(snapshotRoot) - Sets which root to use for reads
//   3. ApplyWritesToTree(root, blob) - Apply writes to tree, returns new root
//   4. CommitAsCanonical(postRoot) - On accumulation: set as canonical
//   5. DiscardTree(root) - On failure: remove tree from store
//
// ===== Root-First Snapshot Methods =====

// CreateSnapshotFromRoot creates a new UBT snapshot by cloning from the specified parent root.
// Returns the root hash of the parent (which equals the snapshot's root until writes are applied).
//
// IMPORTANT: The snapshot is NOT stored in treeStore yet because its root equals parentRoot
// until writes are applied. Storing it would overwrite or conflict with the parent tree.
// After applying writes via ApplyWritesToTree, the resulting tree is stored under the new root.
//
// This is the root-first replacement for CreateSnapshotForBlock.
// Instead of relying on block number chaining, the caller explicitly specifies
// which state to clone from, enabling standalone pre/post per bundle.
//
// Typical workflow:
//   1. preRoot := GetCanonicalRoot() or parent bundle's postRoot
//   2. snapshot, _ := CreateSnapshotFromRoot(preRoot) // Returns preRoot, creates internal clone
//   3. SetActiveRoot(preRoot) // Use parent for reads during execution
//   4. ... execute refine ...
//   5. postRoot, _ := ApplyWritesToTree(preRoot, contractWrites) // Clone, apply, store under postRoot
//   6. EnqueueWithRoots(bundle, preRoot, postRoot)
func (store *StorageHub) CreateSnapshotFromRoot(parentRoot common.Hash) (common.Hash, error) {
	// Verify parent root exists
	_, exists := store.GetTreeByRootTyped(parentRoot)
	if !exists {
		return common.Hash{}, fmt.Errorf("CreateSnapshotFromRoot: parent root not found %s", parentRoot.Hex())
	}

	// NOTE: We don't clone or store here - ApplyWritesToTree does the clone-on-write.
	// The caller should use parentRoot as preRoot, and ApplyWritesToTree will produce postRoot.

	log.Info(log.EVM, "CreateSnapshotFromRoot: validated parent root",
		"parentRoot", parentRoot.Hex())

	return parentRoot, nil
}

// ApplyWritesToTree applies contract writes to the tree at the given root.
// Returns the new root hash after writes are applied.
// The modified tree is stored in treeStore under the new root.
//
// This is the root-first replacement for ApplyWritesToActiveSnapshot.
// The caller specifies exactly which tree to modify by its root.
func (store *StorageHub) ApplyWritesToTree(root common.Hash, blob []byte) (common.Hash, error) {
	if len(blob) == 0 {
		return root, nil // No changes, return same root
	}

	// Get the original tree (use typed internal method)
	origTree, exists := store.GetTreeByRootTyped(root)
	if !exists {
		return common.Hash{}, fmt.Errorf("ApplyWritesToTree: tree not found at root %s", root.Hex())
	}

	// Clone before mutation to preserve root immutability.
	// The original tree at `root` must remain unchanged so historical roots stay valid.
	// This enables safe resubmission - the preRoot always points to the correct pre-state.
	snapshot := origTree.Copy()

	// Apply writes to the cloned snapshot
	if err := applyContractWritesToTree(blob, snapshot); err != nil {
		return common.Hash{}, fmt.Errorf("ApplyWritesToTree: failed to apply writes: %w", err)
	}

	// Compute new root and store the cloned snapshot under it
	newRootBytes := snapshot.RootHash()
	newRoot := common.BytesToHash(newRootBytes[:])

	// Store cloned snapshot under new root (always store, even if root unchanged,
	// since the snapshot is a distinct object from the original)
	store.StoreTreeWithRoot(newRoot, snapshot)

	log.Info(log.EVM, "ApplyWritesToTree: applied writes (copy-on-write)",
		"oldRoot", root.Hex(),
		"newRoot", newRoot.Hex(),
		"blobSize", len(blob))

	return newRoot, nil
}

// GetActiveTree returns the UBT tree for reads.
// Priority: pinnedTree > activeRoot > canonical.
func (store *StorageHub) GetActiveTree() interface{} {
	return store.Session.UBT.GetActiveTree()
}

// GetActiveTreeTyped returns the typed UBT tree for reads.
// Priority: pinnedTree > activeRoot > canonical.
func (store *StorageHub) GetActiveTreeTyped() *UnifiedBinaryTree {
	return store.Session.UBT.GetActiveTreeTyped()
}
