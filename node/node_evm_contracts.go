package node

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/big"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb"
	types "github.com/colorfulnotion/jam/types"
)

// PRECOMPILES defines all precompile contracts to be loaded during genesis
type Precompile struct {
	Address      common.Address
	BytecodePath string
}

var PRECOMPILES = []Precompile{
	{common.HexToAddress("0x01"), "../services/evm/contracts/usdm-runtime.bin"},
	{common.HexToAddress("0xFF"), "../services/evm/contracts/math-runtime.bin"},
}
var usdmAddress = PRECOMPILES[0].Address
var mathAddress = PRECOMPILES[1].Address
var issuerAddress = common.HexToAddress("0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266")

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

// tx_to_objectID converts a transaction hash to a receipt object ID
// Uses the raw transaction hash directly as the ObjectID (no modification)
// The ObjectKind discriminator is stored in ObjectRef.TxSlotKind field (upper 8 bits)
func tx_to_objectID(txHash common.Hash) common.Hash {
	return txHash // Use raw hash directly - no modification
}

// ssr_to_objectID constructs SSR object ID: [20B address][11B zero][1B kind=0x02]
func ssr_to_objectID(address common.Address) common.Hash {
	var objectID common.Hash
	copy(objectID[:20], address[:])
	// Bytes 20..31 stay zero
	objectID[31] = byte(ObjectKindSSRMetadata)
	return objectID
}

// shard_to_objectID constructs storage shard object ID: [20B address][1B 0xFF][1B ld][7B prefix56][2B zero][1B kind=0x01]
// Note: This is a simplified version - full implementation would need ShardId parameter
func shard_to_objectID(address common.Address, shardID []byte) common.Hash {
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

// code_to_objectID constructs code object ID: [20B address][11B zero][1B kind=0x00]
func code_to_objectID(address common.Address) common.Hash {
	var objectID common.Hash
	copy(objectID[:20], address[:])
	// Bytes 20..31 stay zero
	objectID[31] = byte(ObjectKindCode)
	return objectID
}

// computeBalanceStorageKey computes the storage key for an address balance
// This key is used to read from the USDM contract (0x01) storage
// In USDM.sol, balanceOf mapping is at slot 0
// Storage key = keccak256(abi.encode(address, 0))
// Must be used with readContractStorageValue(stateDB, usdmAddress, storageKey)
func computeBalanceStorageKey(address common.Address) common.Hash {
	return computeStorageKey(address.Bytes(), 0)
}

// computeNonceStorageKey computes the storage key for an address nonce
// This key is used to read from the USDM contract (0x01) storage
// In USDM.sol, nonces mapping is at slot 1
// Storage key = keccak256(abi.encode(address, 1))
// Must be used with readContractStorageValue(stateDB, usdmAddress, storageKey)
func computeNonceStorageKey(address common.Address) common.Hash {
	return computeStorageKey(address.Bytes(), 1)
}

// computeStorageKey computes the storage key for a mapping slot in Solidity
// For a mapping at slot S with key K: keccak256(abi.encode(K, S))
func computeStorageKey(key []byte, slot uint64) common.Hash {
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
	globalDepth uint8
	entryCount  uint8
	totalKeys   uint32
	version     uint32
	entries     []SSREntry
}

// SSREntry represents a split exception in the SSR
type SSREntry struct {
	d        uint8   // Split point (prefix bit position)
	ld       uint8   // Local depth at this prefix
	prefix56 [7]byte // 56-bit routing prefix
}

// ShardID identifies a storage shard
type ShardID struct {
	ld       uint8   // Local depth
	prefix56 [7]byte // 56-bit prefix
}

// toBytes converts ShardID to byte slice for shard_to_objectID
func (s ShardID) toBytes() []byte {
	result := make([]byte, 8)
	result[0] = s.ld
	copy(result[1:8], s.prefix56[:])
	return result
}

// parseSSRMetadata parses SSR metadata from DA payload
// Format: [1B global_depth][1B entry_count][4B total_keys][4B version][SSREntry...]
// Each SSREntry: [1B d][1B ld][7B prefix56]
func parseSSRMetadata(payload []byte) (*SSRData, error) {
	// SSR header is 10 bytes: 1+1+4+4
	if len(payload) < 10 {
		return nil, fmt.Errorf("SSR payload too small: %d bytes, expected at least 10", len(payload))
	}

	ssr := &SSRData{
		globalDepth: payload[0],
		entryCount:  payload[1],
		totalKeys:   binary.LittleEndian.Uint32(payload[2:6]),
		version:     binary.LittleEndian.Uint32(payload[6:10]),
		entries:     make([]SSREntry, 0),
	}

	// Parse SSR entries (9 bytes each)
	offset := 10
	for i := uint8(0); i < ssr.entryCount; i++ {
		if offset+9 > len(payload) {
			return nil, fmt.Errorf("SSR payload truncated at entry %d", i)
		}
		entry := SSREntry{
			d:  payload[offset],
			ld: payload[offset+1],
		}
		copy(entry.prefix56[:], payload[offset+2:offset+9])
		ssr.entries = append(ssr.entries, entry)
		offset += 9
	}

	log.Debug(log.Node, "parseSSRMetadata",
		"globalDepth", ssr.globalDepth,
		"entryCount", ssr.entryCount,
		"totalKeys", ssr.totalKeys)

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

// resolveShardID resolves which shard a storage key belongs to using SSR
// Algorithm:
// 1. Start with global_depth from SSR header
// 2. Extract 56-bit prefix from key
// 3. Search SSR entries for matching prefix exception
// 4. Return ShardId with resolved depth and masked prefix
func resolveShardID(ssr *SSRData, key common.Hash) ShardID {
	ld := ssr.globalDepth
	keyPrefix56 := takePrefix56(key)

	// Search for matching SSR entry
	for _, entry := range ssr.entries {
		if matchesPrefix(keyPrefix56, entry.prefix56, entry.d) {
			ld = entry.ld
			break
		}
	}

	return ShardID{
		ld:       ld,
		prefix56: maskPrefix56(keyPrefix56, ld),
	}
}

// parseSSRPayloadForStorageKey parses the SSR payload (shard data) to find the value for a given storage key
func parseSSRPayloadForStorageKey(ssrPayload []byte, storageKey common.Hash) (common.Hash, error) {
	// Parse SSR shard data format based on services/evm/src/sharding.rs
	// Format: [2B count] + [entries: 32B key_h + 32B value] * count

	log.Debug(log.Node, "parseSSRPayloadForStorageKey",
		"payloadLen", len(ssrPayload),
		"storageKey", common.Bytes2Hex(storageKey.Bytes()))

	// Check minimum size (2 bytes for count)
	if len(ssrPayload) < 2 {
		log.Warn(log.Node, "parseSSRPayloadForStorageKey: Payload too small", "size", len(ssrPayload))
		return common.Hash{}, nil // Return zero value
	}

	// Parse entry count (2 bytes little-endian)
	entryCount := binary.LittleEndian.Uint16(ssrPayload[0:2])
	expectedLen := 2 + (int(entryCount) * 64) // 2 + count * (32+32)

	if len(ssrPayload) < expectedLen {
		log.Warn(log.Node, "parseSSRPayloadForStorageKey: Payload truncated",
			"expected", expectedLen, "actual", len(ssrPayload), "entries", entryCount)
		return common.Hash{}, nil // Return zero value
	}

	log.Debug(log.Node, "parseSSRPayloadForStorageKey: Parsing shard",
		"entryCount", entryCount, "expectedLen", expectedLen)

	// The storageKey is already keccak256(abi.encode(address, slot)) - use it directly
	// Bootstrap and effects.rs store this hash as key_h, not keccak256(key_h)
	storageKeyHash := storageKey.Bytes()

	// Search through entries
	offset := 2
	for i := uint16(0); i < entryCount; i++ {
		// Extract key hash (32 bytes) and value (32 bytes)
		keyHash := ssrPayload[offset : offset+32]
		value := ssrPayload[offset+32 : offset+64]

		log.Trace(log.Node, "parseSSRPayloadForStorageKey: Checking entry",
			"index", i,
			"keyHash", common.Bytes2Hex(keyHash),
			"value", common.Bytes2Hex(value))

		// Compare key hashes
		if bytes.Equal(keyHash, storageKeyHash[:]) {
			log.Debug(log.Node, "parseSSRPayloadForStorageKey: Found matching entry",
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
	log.Debug(log.Node, "parseSSRPayloadForStorageKey: Key not found in shard",
		"storageKey", common.Bytes2Hex(storageKey.Bytes()),
		"searchedEntries", entryCount)

	return common.Hash{}, nil
}

// Storage and Object Reading Helper Methods

// ReadObjectRef reads ObjectRef bytes from service storage and deserializes them
// Parameters:
// - stateDB: The stateDB to read from, if nil uses n.statedb
// - serviceCode: The service code to read from
// - objectID: The object ID (typically a transaction hash or other identifier)
// Returns:
// - ObjectRef: The deserialized ObjectRef struct
// - bool: true if found, false if not found
// - error: any error that occurred
func (n *NodeContent) ReadObjectRef(stateDB *statedb.StateDB, serviceCode uint32, objectID common.Hash) (*types.ObjectRef, bool, error) {
	if stateDB == nil {
		stateDB = n.statedb
	}
	// Read raw ObjectRef bytes from service storage
	objectRefBytes, found, err := stateDB.ReadServiceStorage(serviceCode, objectID.Bytes())
	if err != nil {
		return nil, false, fmt.Errorf("failed to read ObjectRef from service storage: %v", err)
	}
	if !found {
		return nil, false, nil // ObjectRef not found
	}

	// Deserialize ObjectRef from storage data
	offset := 0
	objRef, err := types.DeserializeObjectRef(objectRefBytes, &offset)
	if err != nil {
		return nil, false, fmt.Errorf("failed to deserialize ObjectRef: %v", err)
	}

	return &objRef, true, nil
}

// readContractStorageValue reads a storage value from any contract at a specific storage key
func (n *NodeContent) readContractStorageValue(stateDB *statedb.StateDB, contractAddress common.Address, storageKey common.Hash) (common.Hash, error) {
	// 1. Read SSR metadata to resolve shard ID
	ssrObjectID := ssr_to_objectID(contractAddress)
	witness, found, err := stateDB.ReadStateWitness(statedb.EVMServiceCode, ssrObjectID, true)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to read SSR metadata for %s: %v", contractAddress.String(), err)
	}
	if !found {
		log.Trace(log.Node, "readContractStorageValue: SSR metadata not found, returning zero",
			"contractAddress", contractAddress.String(), "storageKey", common.Bytes2Hex(storageKey.Bytes()))
		return common.Hash{}, nil // Contract doesn't exist yet
	}

	// 2. Parse SSR metadata to get global_depth and entries
	ssrData, err := parseSSRMetadata(witness.Payload)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to parse SSR metadata: %v", err)
	}

	// 3. Resolve shard ID for the storage key
	shardID := resolveShardID(ssrData, storageKey)

	log.Debug(log.Node, "readContractStorageValue: Resolved shard",
		"storageKey", common.Bytes2Hex(storageKey.Bytes()),
		"shardLD", shardID.ld,
		"shardPrefix", common.Bytes2Hex(shardID.prefix56[:]))

	// 4. Read the shard from DA
	shardObjectID := shard_to_objectID(contractAddress, shardID.toBytes())
	witness, found, err = stateDB.ReadStateWitness(statedb.EVMServiceCode, shardObjectID, true)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to read shard: %v", err)
	}
	if !found {
		log.Debug(log.Node, "readContractStorageValue: Shard not found, returning zero",
			"shardObjectID", common.Bytes2Hex(shardObjectID.Bytes()))
		return common.Hash{}, nil // Shard doesn't exist yet
	}
	shardPayload := witness.Payload
	// 5. Parse the shard payload to extract the storage value
	value, err := parseSSRPayloadForStorageKey(shardPayload, storageKey)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to parse shard payload: %v", err)
	}

	return value, nil
}

// readUSDMStorageValue reads a storage value from the USDM contract at a specific slot for a given address
func (n *NodeContent) readUSDMStorageValue(stateDB *statedb.StateDB, userAddress common.Address, slot uint8) (common.Hash, error) {
	// 1. Compute the storage key for the specific slot and user address
	var storageKey common.Hash
	switch slot {
	case 0: // balanceOf mapping
		storageKey = computeBalanceStorageKey(userAddress)
	case 1: // nonces mapping
		storageKey = computeNonceStorageKey(userAddress)
	default:
		return common.Hash{}, fmt.Errorf("unsupported storage slot: %d", slot)
	}

	// 3. Use the generic contract storage reader
	return n.readContractStorageValue(stateDB, usdmAddress, storageKey)
}

// GetCode returns the bytecode at a given address
func (n *NodeContent) GetCode(address common.Address, blockNumber string) ([]byte, error) {
	// Resolve block number to state
	stateDB, err := n.resolveBlockNumberToState(blockNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve block number %s: %v", blockNumber, err)
	}

	// Read contract code from DA
	codeObjectID := code_to_objectID(address)
	witness, found, err := stateDB.ReadStateWitness(statedb.EVMServiceCode, codeObjectID, true)
	if err != nil {
		return nil, fmt.Errorf("failed to read contract code: %v", err)
	}
	if !found {
		// Return empty bytecode for EOA accounts
		return []byte{}, nil
	}

	return witness.Payload, nil
}

// GetBalance fetches the balance of an address from JAM State/DA
func (n *NodeContent) GetBalance(address common.Address, blockNumber string) (common.Hash, error) {
	// Resolve block number to state
	stateDB, err := n.resolveBlockNumberToState(blockNumber)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to resolve block number %s: %v", blockNumber, err)
	}

	// Compute storage key for balance in USDM contract
	storageKey := computeBalanceStorageKey(address)
	log.Debug(log.Node, "GetBalance", "address", address.String(), "storageKey", storageKey.String(), "usdmAddress", usdmAddress.String())

	// Read from SSR-sharded storage
	value, err := n.readContractStorageValue(stateDB, usdmAddress, storageKey)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to read balance: %v", err)
	}

	log.Debug(log.Node, "GetBalance result", "address", address.String(), "value", value.String())
	return value, nil
}

// GetStorageAt reads contract storage at a specific position
func (n *NodeContent) GetStorageAt(address common.Address, position common.Hash, blockNumber string) (common.Hash, error) {
	// Resolve block number to state
	stateDB, err := n.resolveBlockNumberToState(blockNumber)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to resolve block number %s: %v", blockNumber, err)
	}

	// Read from SSR-sharded storage
	value, err := n.readContractStorageValue(stateDB, address, position)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to read storage: %v", err)
	}

	return value, nil
}

// GetTransactionCount fetches the nonce (transaction count) of an address
func (n *NodeContent) GetTransactionCount(address common.Address, blockNumber string) (uint64, error) {
	// Resolve block number to state
	stateDB, err := n.resolveBlockNumberToState(blockNumber)
	if err != nil {
		return 0, fmt.Errorf("failed to resolve block number %s: %v", blockNumber, err)
	}

	// Compute storage key for nonce in USDM contract
	storageKey := computeNonceStorageKey(address)

	// Read from SSR-sharded storage
	nonceHash, err := n.readContractStorageValue(stateDB, usdmAddress, storageKey)
	if err != nil {
		return 0, fmt.Errorf("failed to read nonce: %v", err)
	}

	// Convert hash to uint64
	nonce := new(big.Int).SetBytes(nonceHash.Bytes())
	return nonce.Uint64(), nil
}
