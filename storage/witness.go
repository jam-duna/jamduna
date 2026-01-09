package storage

import (
	"encoding/binary"
	"fmt"

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
