package storage

import (
	"bytes"
	"fmt"
	"reflect"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
)

const blockFinalizedKey = "blk_finalized"

// StoreBlock stores a block with multiple indices for efficient retrieval
// Indices created:
// - header_<headerhash> -> blockHash
// - blk_<blockHash> -> encoded block
// - blk_<slot> -> encoded block
// - child_<parentHeaderHash>_<headerHash> -> blockHash
func (s *StateDBStorage) StoreBlock(blk *types.Block, id uint16, slotTimestamp uint64) error {
	headerHash := blk.Header.Hash()
	blockHash := blk.Hash()

	// Index 1: header_<headerhash> -> blockHash
	headerPrefix := []byte("header_")
	storeKey := append(headerPrefix, headerHash[:]...)
	if err := s.WriteRawKV(storeKey, blockHash[:]); err != nil {
		return fmt.Errorf("failed to store header index: %w", err)
	}

	// Encode block once for storage
	encodedBlk, err := types.Encode(blk)
	if err != nil {
		return fmt.Errorf("failed to encode block: %w", err)
	}

	// Index 2: blk_<blockHash> -> encoded block
	blockPrefix := []byte("blk_")
	blkStoreKey := append(blockPrefix, blockHash[:]...)
	if err := s.WriteRawKV(blkStoreKey, encodedBlk); err != nil {
		return fmt.Errorf("failed to store block by hash: %w", err)
	}

	// Index 3: blk_<slot> -> encoded block
	slotStoreKey := append(blockPrefix, common.Uint32ToBytes(blk.Header.Slot)...)
	if err := s.WriteRawKV(slotStoreKey, encodedBlk); err != nil {
		return fmt.Errorf("failed to store block by slot: %w", err)
	}

	// Index 4: child_<ParentHeaderHash>_<headerHash> -> blockHash
	// This enables efficient retrieval of all children for a given parent
	childPrefix := []byte("child_")
	childStoreKey := append(childPrefix, blk.Header.ParentHeaderHash[:]...)
	childStoreKey = append(childStoreKey, headerHash[:]...)
	if err := s.WriteRawKV(childStoreKey, blockHash[:]); err != nil {
		return fmt.Errorf("failed to store child index: %w", err)
	}

	return nil
}

func (s *StateDBStorage) StoreCatchupMassage(round uint64, setId uint32, data []byte) error {
	storeKey := append([]byte("catchup_"), common.Uint64ToBytes(round)...)
	storeKey = append(storeKey, common.Uint32ToBytes(setId)...)
	if err := s.WriteRawKV(storeKey, data); err != nil {
		return fmt.Errorf("failed to store catchup message: %w", err)
	}
	return nil
}

func (s *StateDBStorage) GetCatchupMassage(round uint64, setId uint32) ([]byte, bool, error) {
	storeKey := append([]byte("catchup_"), common.Uint64ToBytes(round)...)
	storeKey = append(storeKey, common.Uint32ToBytes(setId)...)
	data, ok, err := s.ReadRawKV(storeKey)
	if err != nil {
		return nil, false, fmt.Errorf("failed to read catchup message: %w", err)
	}
	if !ok {
		return nil, false, nil
	}
	return data, true, nil
}

// StoreFinalizedBlock stores the finalized block at a well-known key
func (s *StateDBStorage) StoreFinalizedBlock(blk *types.Block) error {
	return s.WriteRawKV([]byte(blockFinalizedKey), blk.Bytes())
}

// GetFinalizedBlock retrieves the finalized block
func (s *StateDBStorage) GetFinalizedBlock() (*types.Block, error) {
	encodedBlk, ok, err := s.ReadRawKV([]byte(blockFinalizedKey))
	if err != nil {
		return nil, fmt.Errorf("failed to read finalized block: %w", err)
	}
	if !ok {
		return nil, nil
	}

	blk, _, err := types.Decode(encodedBlk, reflect.TypeOf(types.Block{}))
	if err != nil {
		return nil, fmt.Errorf("failed to decode finalized block: %w", err)
	}

	b := blk.(types.Block)
	return &b, nil
}

// GetFinalizedBlockInternal retrieves the finalized block with an existence flag
func (s *StateDBStorage) GetFinalizedBlockInternal() (*types.Block, bool, error) {
	encodedBlk, ok, err := s.ReadRawKV([]byte(blockFinalizedKey))
	if err != nil {
		return nil, false, fmt.Errorf("failed to read finalized block: %w", err)
	}
	if !ok {
		return nil, false, nil
	}

	blk, _, err := types.Decode(encodedBlk, reflect.TypeOf(types.Block{}))
	if err != nil {
		return nil, false, fmt.Errorf("failed to decode finalized block: %w", err)
	}

	b := blk.(types.Block)
	return &b, true, nil
}

// GetBlockByHeader retrieves a block by its header hash
func (s *StateDBStorage) GetBlockByHeader(headerHash common.Hash) (*types.SBlock, error) {
	// Lookup: header_<headerhash> -> blockHash
	headerPrefix := []byte("header_")
	storeKey := append(headerPrefix, headerHash[:]...)
	blockHash, ok, err := s.ReadRawKV(storeKey)
	if err != nil {
		return nil, fmt.Errorf("failed to read header index: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("block not found for header %s", headerHash.Hex())
	}

	// Lookup: blk_<blockHash> -> encoded block
	blockPrefix := []byte("blk_")
	blkStoreKey := append(blockPrefix, blockHash...)
	encodedBlk, ok, err := s.ReadRawKV(blkStoreKey)
	if err != nil {
		return nil, fmt.Errorf("failed to read block: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("block data not found for hash %x", blockHash)
	}

	blk, _, err := types.Decode(encodedBlk, reflect.TypeOf(types.Block{}))
	if err != nil {
		return nil, fmt.Errorf("failed to decode block: %w", err)
	}

	b := blk.(types.Block)
	sb := &types.SBlock{
		Header:     b.Header,
		Extrinsic:  b.Extrinsic,
		HeaderHash: b.Header.Hash(),
		Timestamp:  s.computeSlotTimestamp(b.Header.Slot),
	}
	return sb, nil
}

// GetBlockBySlot retrieves a block by its slot number
func (s *StateDBStorage) GetBlockBySlot(slot uint32) (*types.SBlock, error) {
	// Lookup: blk_<slot> -> encoded block
	slotPrefix := []byte("blk_")
	slotStoreKey := append(slotPrefix, common.Uint32ToBytes(slot)...)
	encodedBlk, ok, err := s.ReadRawKV(slotStoreKey)
	if err != nil {
		return nil, fmt.Errorf("failed to read block by slot: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("block not found for slot %d", slot)
	}

	blk, _, err := types.Decode(encodedBlk, reflect.TypeOf(types.Block{}))
	if err != nil {
		return nil, fmt.Errorf("failed to decode block: %w", err)
	}

	b := blk.(types.Block)
	sb := &types.SBlock{
		Header:     b.Header,
		Extrinsic:  b.Extrinsic,
		HeaderHash: b.Header.Hash(),
		Timestamp:  s.computeSlotTimestamp(b.Header.Slot),
	}
	return sb, nil
}

// GetChildBlocks retrieves all child blocks for a given parent header hash
// Returns raw key-value pairs for efficiency
func (s *StateDBStorage) GetChildBlocks(parentHeaderHash common.Hash) ([][2][]byte, error) {
	// Query: child_<parentHash>_* -> blockHash
	prefix := []byte("child_")
	childStoreKey := append(prefix, parentHeaderHash[:]...)

	keyvals, err := s.ReadRawKVWithPrefix(childStoreKey)
	if err != nil {
		return nil, fmt.Errorf("failed to read child blocks: %w", err)
	}

	log.Trace(log.Node, "GetChildBlocks", "parentHash", parentHeaderHash, "count", len(keyvals))
	return keyvals, nil
}

// computeSlotTimestamp calculates the timestamp for a given slot
// TODO: This should be moved to a time utility package and use proper epoch start time
func (s *StateDBStorage) computeSlotTimestamp(slot uint32) uint64 {
	// Temporary implementation - should use actual JAM_START_TIME from config
	// epochStartTime := time.Date(2025, time.January, 1, 12, 0, 0, 0, time.UTC).Unix()
	// return uint64(epochStartTime) + uint64(slot*types.SecondsPerSlot)

	// For now, use a fixed epoch start
	const epochStart = 1735740000 // 2025-01-01 12:00:00 UTC
	return epochStart + uint64(slot*types.SecondsPerSlot)
}

// stripPrefix removes a prefix from a key and returns the remainder
// Helper function for parsing composite keys
func stripPrefix(key []byte, prefix []byte) ([]byte, error) {
	if !bytes.HasPrefix(key, prefix) {
		return nil, fmt.Errorf("key does not start with the specified prefix")
	}
	return key[len(prefix):], nil
}
