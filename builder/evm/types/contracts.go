package types

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
)

// PRECOMPILES defines all precompile contracts to be loaded during genesis
type Precompile struct {
	Address      common.Address
	BytecodePath string
}

var PRECOMPILES = []Precompile{
	{common.HexToAddress("0x01"), "../services/evm/contracts/usdm-runtime.bin"},
	{common.HexToAddress("0x02"), "../services/evm/contracts/gov-runtime.bin"},
	{common.HexToAddress("0x03"), "../services/evm/contracts/math-runtime.bin"},
}
var UsdmAddress = PRECOMPILES[0].Address
var GovAddress = PRECOMPILES[1].Address
var MathAddress = PRECOMPILES[2].Address
var IssuerAddress = common.HexToAddress("0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266")

// ObjectKind represents the type of DA object, matching the Rust enum
type ObjectKind byte

const (
	// Code object - bytecode storage (kind=0x00)
	ObjectKindCode ObjectKind = 0x00
	// Storage shard object (kind=0x01)
	ObjectKindStorageShard ObjectKind = 0x01
	// SSR metadata object (kind=0x02)
	ObjectKindSSRMetadata ObjectKind = 0x02
	// Receipt object (kind=0x03)
	ObjectKindReceipt ObjectKind = 0x03
	// Transaction object (kind=0x04)
	ObjectKindTransaction ObjectKind = 0x04
	// Block - block object (kind=0x05)
	ObjectKindBlock ObjectKind = 0x05
)

// TxToObjectID converts a transaction hash to a receipt object ID
// Uses the raw transaction hash directly as the ObjectID (no modification)
// The ObjectKind discriminator is stored in ObjectRef.TxSlotKind field (upper 8 bits)
func TxToObjectID(txHash common.Hash) common.Hash {
	return txHash // Use raw hash directly - no modification
}

// SsrToObjectID constructs SSR object ID: [20B address][11B zero][1B kind=0x02]
func SsrToObjectID(address common.Address) common.Hash {
	var objectID common.Hash
	copy(objectID[:20], address[:])
	// Bytes 20..31 stay zero
	objectID[31] = byte(ObjectKindSSRMetadata)
	return objectID
}

// ShardToObjectID constructs storage shard object ID: [20B address][1B 0xFF][1B ld][7B prefix56][2B zero][1B kind=0x01]
// Note: This is a simplified version - full implementation would need ShardId parameter
func ShardToObjectID(address common.Address, shardID []byte) common.Hash {
	var objectID common.Hash
	copy(objectID[:20], address[:])
	objectID[20] = 0xFF
	if len(shardID) >= 8 {
		objectID[21] = shardID[0]           // ld
		copy(objectID[22:29], shardID[1:8]) // prefix56
	}
	// Bytes 29..31 stay zero for future use (except byte 31)
	objectID[31] = byte(ObjectKindStorageShard)
	return objectID
}

// CodeToObjectID constructs code object ID: [20B address][11B zero][1B kind=0x00]
func CodeToObjectID(address common.Address) common.Hash {
	var objectID common.Hash
	copy(objectID[:20], address[:])
	// Bytes 20..31 stay zero
	objectID[31] = byte(ObjectKindCode)
	return objectID
}

// ComputeBalanceStorageKey computes the storage key for an address balance
// This key is used to read from the USDM contract (0x01) storage
// In USDM.sol, balanceOf mapping is at slot 0
// Storage key = keccak256(abi.encode(address, 0))
// Must be used with readContractStorageValue(stateDB, usdmAddress, storageKey)
func ComputeBalanceStorageKey(address common.Address) common.Hash {
	return ComputeStorageKey(address.Bytes(), 0)
}

// ComputeNonceStorageKey computes the storage key for an address nonce
// This key is used to read from the USDM contract (0x01) storage
// In USDM.sol, nonces mapping is at slot 1
// Storage key = keccak256(abi.encode(address, 1))
// Must be used with readContractStorageValue(stateDB, usdmAddress, storageKey)
func ComputeNonceStorageKey(address common.Address) common.Hash {
	return ComputeStorageKey(address.Bytes(), 1)
}

// ComputeStorageKey computes the storage key for a mapping slot in Solidity
// For a mapping at slot S with key K: keccak256(abi.encode(K, S))
func ComputeStorageKey(key []byte, slot uint64) common.Hash {
	// Create input: key (32 bytes, left-padded) + slot (32 bytes, big-endian)
	input := make([]byte, 64)

	// Left-pad the key to 32 bytes (for addresses, this means padding with zeros)
	if len(key) <= 32 {
		copy(input[32-len(key):32], key)
	} else {
		copy(input[0:32], key[len(key)-32:])
	}

	// Encode slot as 32-byte big-endian uint256
	binary.BigEndian.PutUint64(input[56:64], slot)

	return common.Keccak256(input)
}

// SSRData represents the parsed SSR metadata
type SSRData struct {
	GlobalDepth uint8
	EntryCount  uint8
	TotalKeys   uint32
	Version     uint32
	Entries     []SSREntry
}

// SSREntry represents a split exception in the SSR
type SSREntry struct {
	D        uint8   // Split point (prefix bit position)
	Ld       uint8   // Local depth at this prefix
	Prefix56 [7]byte // 56-bit routing prefix
}

// ShardID identifies a storage shard
type ShardID struct {
	Ld       uint8   // Local depth
	Prefix56 [7]byte // 56-bit prefix
}

// ToBytes converts ShardID to byte slice for ShardToObjectID
func (s ShardID) ToBytes() []byte {
	result := make([]byte, 8)
	result[0] = s.Ld
	copy(result[1:8], s.Prefix56[:])
	return result
}

// ParseSSRMetadata parses SSR metadata from DA payload
// Format: [1B global_depth][1B entry_count][4B total_keys][4B version][SSREntry...]
// Each SSREntry: [1B d][1B ld][7B prefix56]
func ParseSSRMetadata(payload []byte) (*SSRData, error) {
	// SSR header is 10 bytes: 1+1+4+4
	if len(payload) < 10 {
		return nil, fmt.Errorf("SSR payload too small: %d bytes, expected at least 10", len(payload))
	}

	ssr := &SSRData{
		GlobalDepth: payload[0],
		EntryCount:  payload[1],
		TotalKeys:   binary.LittleEndian.Uint32(payload[2:6]),
		Version:     binary.LittleEndian.Uint32(payload[6:10]),
		Entries:     make([]SSREntry, 0),
	}

	// Parse SSR entries (9 bytes each)
	offset := 10
	for i := uint8(0); i < ssr.EntryCount; i++ {
		if offset+9 > len(payload) {
			return nil, fmt.Errorf("SSR payload truncated at entry %d", i)
		}
		entry := SSREntry{
			D:  payload[offset],
			Ld: payload[offset+1],
		}
		copy(entry.Prefix56[:], payload[offset+2:offset+9])
		ssr.Entries = append(ssr.Entries, entry)
		offset += 9
	}

	log.Debug(log.Node, "ParseSSRMetadata",
		"globalDepth", ssr.GlobalDepth,
		"entryCount", ssr.EntryCount,
		"totalKeys", ssr.TotalKeys)

	return ssr, nil
}

// takePrefix56 extracts first 56 bits (7 bytes) from hash as routing prefix
func takePrefix56(key common.Hash) [7]byte {
	var prefix [7]byte
	copy(prefix[:], key[:7])
	return prefix
}

// matchesPrefix checks if key_prefix matches entry_prefix for first 'bits' bits
func matchesPrefix(keyPrefix, entryPrefix [7]byte, bits uint8) bool {
	bytesToCompare := int(bits / 8)
	remainingBits := bits % 8

	// Compare full bytes
	if !bytes.Equal(keyPrefix[:bytesToCompare], entryPrefix[:bytesToCompare]) {
		return false
	}

	// Compare remaining bits
	if remainingBits > 0 {
		mask := byte(0xFF << (8 - remainingBits))
		if (keyPrefix[bytesToCompare] & mask) != (entryPrefix[bytesToCompare] & mask) {
			return false
		}
	}

	return true
}

// maskPrefix56 masks prefix to first 'bits' bits, zero out the rest
func maskPrefix56(prefix [7]byte, bits uint8) [7]byte {
	if bits > 56 {
		bits = 56 // Clamp to maximum
	}

	var masked [7]byte
	fullBytes := int(bits / 8)
	remainingBits := bits % 8

	// Copy full bytes
	copy(masked[:fullBytes], prefix[:fullBytes])

	// Mask remaining bits
	if remainingBits > 0 && fullBytes < 7 {
		mask := byte(0xFF << (8 - remainingBits))
		masked[fullBytes] = prefix[fullBytes] & mask
	}

	return masked
}

// ResolveShardID resolves which shard a storage key belongs to using SSR
// Algorithm:
// 1. Start with global_depth from SSR header
// 2. Extract 56-bit prefix from key
// 3. Search SSR entries for matching prefix exception
// 4. Return ShardId with resolved depth and masked prefix
func ResolveShardID(ssr *SSRData, key common.Hash) ShardID {
	ld := ssr.GlobalDepth
	keyPrefix56 := takePrefix56(key)

	// Search for matching SSR entry
	for _, entry := range ssr.Entries {
		if matchesPrefix(keyPrefix56, entry.Prefix56, entry.D) {
			ld = entry.Ld
			break
		}
	}

	return ShardID{
		Ld:       ld,
		Prefix56: maskPrefix56(keyPrefix56, ld),
	}
}

// ParseShardPayload parses a storage shard payload into a list of EvmEntry items
// Post-SSR Format: [2B count] + [entries: 32B key_h + 32B value] * count
// No merkle_root field after witness transition
func ParseShardPayload(shardPayload []byte) ([]EvmEntry, error) {
	// Check minimum size (2 bytes count)
	if len(shardPayload) < 2 {
		return nil, fmt.Errorf("shard payload too small: %d bytes", len(shardPayload))
	}

	// Parse entry count (2 bytes little-endian)
	entryCount := binary.LittleEndian.Uint16(shardPayload[0:2])
	expectedLen := 2 + (int(entryCount) * 64)

	if len(shardPayload) < expectedLen {
		return nil, fmt.Errorf("shard payload truncated: expected %d bytes, got %d", expectedLen, len(shardPayload))
	}

	// Parse all entries
	entries := make([]EvmEntry, entryCount)
	offset := 2
	for i := uint16(0); i < entryCount; i++ {
		keyHash := shardPayload[offset : offset+32]
		value := shardPayload[offset+32 : offset+64]

		entries[i] = EvmEntry{
			KeyH:  common.BytesToHash(keyHash),
			Value: common.BytesToHash(value),
		}

		offset += 64
	}

	return entries, nil
}

// ParseSSRPayloadForStorageKey parses the SSR payload (shard data) to find the value for a given storage key
func ParseSSRPayloadForStorageKey(ssrPayload []byte, storageKey common.Hash) (common.Hash, error) {
	// Parse SSR shard data format based on services/evm/src/da.rs:serialize_shard
	// Format: [32B merkle_root] + [2B count] + [entries: 32B key_h + 32B value] * count

	log.Debug(log.Node, "ParseSSRPayloadForStorageKey",
		"payloadLen", len(ssrPayload),
		"storageKey", common.Bytes2Hex(storageKey.Bytes()))

	// Check minimum size (32 bytes merkle root + 2 bytes count)
	if len(ssrPayload) < 34 {
		log.Warn(log.Node, "ParseSSRPayloadForStorageKey: Payload too small", "size", len(ssrPayload))
		return common.Hash{}, nil // Return zero value
	}

	// Skip merkle root (32 bytes), parse entry count (2 bytes little-endian)
	entryCount := binary.LittleEndian.Uint16(ssrPayload[32:34])
	expectedLen := 34 + (int(entryCount) * 64) // 32 + 2 + count * (32+32)

	if len(ssrPayload) < expectedLen {
		log.Warn(log.Node, "ParseSSRPayloadForStorageKey: Payload truncated",
			"expected", expectedLen, "actual", len(ssrPayload), "entries", entryCount)
		return common.Hash{}, nil // Return zero value
	}

	log.Debug(log.Node, "ParseSSRPayloadForStorageKey: Parsing shard",
		"entryCount", entryCount, "expectedLen", expectedLen)

	// The storageKey is already keccak256(abi.encode(address, slot)) - use it directly
	// Bootstrap and effects.rs store this hash as key_h, not keccak256(key_h)
	storageKeyHash := storageKey.Bytes()

	// Search through entries (starting after merkle_root + count)
	offset := 34
	for i := uint16(0); i < entryCount; i++ {
		// Extract key hash (32 bytes) and value (32 bytes)
		keyHash := ssrPayload[offset : offset+32]
		value := ssrPayload[offset+32 : offset+64]

		log.Trace(log.Node, "ParseSSRPayloadForStorageKey: Checking entry",
			"index", i,
			"keyHash", common.Bytes2Hex(keyHash),
			"value", common.Bytes2Hex(value))

		// Compare key hashes
		if bytes.Equal(keyHash, storageKeyHash[:]) {
			log.Debug(log.Node, "ParseSSRPayloadForStorageKey: Found matching entry",
				"index", i,
				"storageKey", common.Bytes2Hex(storageKey.Bytes()),
				"keyHash", common.Bytes2Hex(keyHash),
				"value", common.Bytes2Hex(value))

			// Return the value as common.Hash
			return common.BytesToHash(value), nil
		}

		offset += 64
	}

	// Key not found in this shard, return zero value
	log.Debug(log.Node, "ParseSSRPayloadForStorageKey: Key not found in shard",
		"storageKey", common.Bytes2Hex(storageKey.Bytes()),
		"searchedEntries", entryCount)

	return common.Hash{}, nil
}

// EvmEntry represents a storage key-value pair (64 bytes total)
// Mirrors Rust EvmEntry from contractsharding.rs
type EvmEntry struct {
	KeyH  common.Hash // Storage key hash (32 bytes)
	Value common.Hash // Storage value (32 bytes)
}

// ContractShard represents a single storage shard for a contract
// Mirrors Rust ContractShard from contractsharding.rs
// Post-SSR: No merkle_root field after witness transition
type ContractShard struct {
	ShardID ShardID    // Shard identifier (ld + prefix56)
	Entries []EvmEntry // Sorted by KeyH
}

// Serialize serializes ContractShard to DA format
// Post-SSR Format: [2B count][entries: 32B key_h + 32B value]
func (cs *ContractShard) Serialize() []byte {
	result := make([]byte, 2+len(cs.Entries)*64)
	binary.LittleEndian.PutUint16(result[0:2], uint16(len(cs.Entries)))

	offset := 2
	for _, entry := range cs.Entries {
		copy(result[offset:offset+32], entry.KeyH[:])
		copy(result[offset+32:offset+64], entry.Value[:])
		offset += 64
	}
	return result
}

// DeserializeContractShard deserializes ContractShard from DA format
// Post-SSR Format: [2B count][entries]
func DeserializeContractShard(data []byte) (*ContractShard, error) {
	if len(data) < 2 {
		return nil, fmt.Errorf("ContractShard payload too small: %d bytes", len(data))
	}

	cs := &ContractShard{
		Entries: make([]EvmEntry, 0),
	}

	entryCount := binary.LittleEndian.Uint16(data[0:2])
	expectedLen := 2 + int(entryCount)*64
	if len(data) < expectedLen {
		return nil, fmt.Errorf("ContractShard payload truncated: expected %d, got %d", expectedLen, len(data))
	}

	offset := 2
	for i := uint16(0); i < entryCount; i++ {
		entry := EvmEntry{
			KeyH:  common.BytesToHash(data[offset : offset+32]),
			Value: common.BytesToHash(data[offset+32 : offset+64]),
		}
		cs.Entries = append(cs.Entries, entry)
		offset += 64
	}

	return cs, nil
}

// ContractStorage represents complete storage for a contract
// Mirrors Rust ContractStorage from contractsharding.rs
// Post-SSR: Single flat shard per contract, SSR removed
type ContractStorage struct {
	Shard ContractShard // Single shard containing all contract storage
}

// VerkleMultiproof represents a verkle tree multiproof for state validation
// Phase 2: Stub implementation - to be replaced with actual verkle proof library
type VerkleMultiproof struct {
	// Map of storage key hash to proof path
	Proofs map[common.Hash][]common.Hash
}
