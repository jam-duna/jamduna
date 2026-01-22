package storage

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"

	"github.com/colorfulnotion/jam/common"
	"github.com/ethereum/go-ethereum/rlp"
)

// Block Access List (BAL) implementation for JAM EVM
// Based on EIP-7928 adapted for JAM's stateless verification model
//
// Design: Guarantor reconstructs BAL from augmented witness (161-byte entries with txIndex)
// without re-execution, ensuring deterministic block_access_list_hash matching.

// ===== Data Structures =====

// BlockAccessList represents all state accesses in a block
type BlockAccessList struct {
	Accounts []AccountChanges // Sorted by address bytes
}

// AccountChanges tracks all accesses to a single account
type AccountChanges struct {
	Address        common.Address
	StorageReads   []StorageRead   // Read-only storage slots (sorted by key)
	StorageChanges []SlotChanges   // Modified storage slots (sorted by key)
	BalanceChanges []BalanceChange // Ordered by blockAccessIndex
	NonceChanges   []NonceChange   // Ordered by blockAccessIndex
	CodeChanges    []CodeChange    // Ordered by blockAccessIndex
}

// StorageRead represents a read-only storage access
type StorageRead struct {
	Key common.Hash // Full storage key
}

// SlotChanges tracks all writes to a single storage slot
type SlotChanges struct {
	Key    common.Hash
	Writes []SlotWrite // Ordered by blockAccessIndex
}

// SlotWrite represents a single write to a storage slot
type SlotWrite struct {
	BlockAccessIndex uint32
	PreValue         common.Hash // Fixed-size for deterministic RLP
	PostValue        common.Hash
}

// BalanceChange represents a balance modification
type BalanceChange struct {
	BlockAccessIndex uint32
	PreBalance       common.Hash // Big-endian uint128 in last 16 bytes
	PostBalance      common.Hash
}

// NonceChange represents a nonce modification
type NonceChange struct {
	BlockAccessIndex uint32
	PreNonce         [8]byte // Little-endian uint64
	PostNonce        [8]byte
}

// CodeChange represents code deployment
type CodeChange struct {
	BlockAccessIndex uint32
	CodeHash         common.Hash // keccak256 of code
	CodeSize         uint32
}

// ===== Hash Computation =====

// Hash computes deterministic block_access_list_hash using RLP encoding + Blake2b
// Note: Uses Blake2b (JAM standard) not Keccak256 (EIP-7928 uses Keccak for Ethereum)
func (bal *BlockAccessList) Hash() common.Hash {
	encoded, err := rlp.EncodeToBytes(bal)
	if err != nil {
		panic(fmt.Sprintf("BAL RLP encoding failed: %v", err))
	}
	return common.Blake2Hash(encoded)
}

// ===== Witness Parsing =====

// ReadEntry represents a single entry from the pre-state witness section
type ReadEntry struct {
	UBTKey     common.Hash
	KeyType    uint8
	Address    common.Address
	Extra      uint64
	StorageKey common.Hash
	PreValue   common.Hash
	PostValue  common.Hash // Hint only (same as PreValue for reads)
	TxIndex    uint32      // Transaction index (blockAccessIndex)
}

// WriteEntry represents a single entry from the post-state witness section
type WriteEntry struct {
	UBTKey     common.Hash
	KeyType    uint8
	Address    common.Address
	Extra      uint64
	StorageKey common.Hash
	PreValue   common.Hash
	PostValue  common.Hash // Authoritative value from contract witness
	TxIndex    uint32      // Transaction index (blockAccessIndex)
}

// parseAugmentedPreStateReads parses the pre-state section of augmented witness
// Format: [32B pre_root][4B count][161B entries...][4B proof_len][proof...]
func parseAugmentedPreStateReads(preStateWitness []byte) ([]ReadEntry, error) {
	if len(preStateWitness) < 36 {
		return nil, fmt.Errorf("pre-state witness too short: %d bytes", len(preStateWitness))
	}

	offset := 32 // Skip pre_root
	count := binary.BigEndian.Uint32(preStateWitness[offset : offset+4])
	offset += 4

	requiredSize := offset + int(count)*161 + 4 // entries + proof_len
	if len(preStateWitness) < requiredSize {
		return nil, fmt.Errorf("pre-state witness truncated: need %d, got %d", requiredSize, len(preStateWitness))
	}

	reads := make([]ReadEntry, count)
	for i := uint32(0); i < count; i++ {
		entry := &reads[i]

		// 32B UBTKey
		copy(entry.UBTKey[:], preStateWitness[offset:offset+32])
		offset += 32

		// 1B KeyType
		entry.KeyType = preStateWitness[offset]
		offset += 1

		// 20B Address
		copy(entry.Address[:], preStateWitness[offset:offset+20])
		offset += 20

		// 8B Extra
		entry.Extra = binary.BigEndian.Uint64(preStateWitness[offset : offset+8])
		offset += 8

		// 32B StorageKey
		copy(entry.StorageKey[:], preStateWitness[offset:offset+32])
		offset += 32

		// 32B PreValue
		copy(entry.PreValue[:], preStateWitness[offset:offset+32])
		offset += 32

		// 32B PostValue (same as PreValue for reads)
		copy(entry.PostValue[:], preStateWitness[offset:offset+32])
		offset += 32

		// 4B TxIndex
		entry.TxIndex = binary.BigEndian.Uint32(preStateWitness[offset : offset+4])
		offset += 4
	}

	return reads, nil
}

// parseAugmentedPostStateWrites parses the post-state section of augmented witness
// Format: [32B post_root][4B count][161B entries...][4B proof_len][proof...]
func parseAugmentedPostStateWrites(postStateWitness []byte) ([]WriteEntry, error) {
	if len(postStateWitness) < 36 {
		return nil, fmt.Errorf("post-state witness too short: %d bytes", len(postStateWitness))
	}

	offset := 32 // Skip post_root
	count := binary.BigEndian.Uint32(postStateWitness[offset : offset+4])
	offset += 4

	requiredSize := offset + int(count)*161 + 4 // entries + proof_len
	if len(postStateWitness) < requiredSize {
		return nil, fmt.Errorf("post-state witness truncated: need %d, got %d", requiredSize, len(postStateWitness))
	}

	writes := make([]WriteEntry, count)
	for i := uint32(0); i < count; i++ {
		entry := &writes[i]

		// 32B UBTKey
		copy(entry.UBTKey[:], postStateWitness[offset:offset+32])
		offset += 32

		// 1B KeyType
		entry.KeyType = postStateWitness[offset]
		offset += 1

		// 20B Address
		copy(entry.Address[:], postStateWitness[offset:offset+20])
		offset += 20

		// 8B Extra
		entry.Extra = binary.BigEndian.Uint64(postStateWitness[offset : offset+8])
		offset += 8

		// 32B StorageKey
		copy(entry.StorageKey[:], postStateWitness[offset:offset+32])
		offset += 32

		// 32B PreValue
		copy(entry.PreValue[:], postStateWitness[offset:offset+32])
		offset += 32

		// 32B PostValue
		copy(entry.PostValue[:], postStateWitness[offset:offset+32])
		offset += 32

		// 4B TxIndex
		entry.TxIndex = binary.BigEndian.Uint32(postStateWitness[offset : offset+4])
		offset += 4
	}

	return writes, nil
}

// ===== BAL Construction =====

const (
	KeyTypeBasicData = 0
	KeyTypeCodeHash  = 1
	KeyTypeCodeChunk = 2
	KeyTypeStorage   = 3
)

// buildSlotChanges groups storage writes by address and key, ordered by blockAccessIndex
func buildSlotChanges(writes []WriteEntry) map[common.Address]map[common.Hash][]SlotWrite {
	slotMap := make(map[common.Address]map[common.Hash][]SlotWrite)

	for _, write := range writes {
		if write.KeyType != KeyTypeStorage {
			continue
		}

		if slotMap[write.Address] == nil {
			slotMap[write.Address] = make(map[common.Hash][]SlotWrite)
		}

		slotMap[write.Address][write.StorageKey] = append(
			slotMap[write.Address][write.StorageKey],
			SlotWrite{
				BlockAccessIndex: write.TxIndex,
				PreValue:         write.PreValue,
				PostValue:        write.PostValue,
			},
		)
	}

	return slotMap
}

// buildBalanceChanges extracts balance changes from BasicData writes
func buildBalanceChanges(writes []WriteEntry) map[common.Address][]BalanceChange {
	changes := make(map[common.Address][]BalanceChange)

	for _, write := range writes {
		if write.KeyType != KeyTypeBasicData {
			continue
		}

		// Extract balance from BasicData (bytes 16-31)
		var preBalance, postBalance common.Hash
		copy(preBalance[16:], write.PreValue[16:])
		copy(postBalance[16:], write.PostValue[16:])

		changes[write.Address] = append(changes[write.Address], BalanceChange{
			BlockAccessIndex: write.TxIndex,
			PreBalance:       preBalance,
			PostBalance:      postBalance,
		})
	}

	return changes
}

// buildNonceChanges extracts nonce changes from BasicData writes
func buildNonceChanges(writes []WriteEntry) map[common.Address][]NonceChange {
	changes := make(map[common.Address][]NonceChange)

	for _, write := range writes {
		if write.KeyType != KeyTypeBasicData {
			continue
		}

		// Extract nonce from BasicData (bytes 8-15, little-endian)
		var preNonce, postNonce [8]byte
		copy(preNonce[:], write.PreValue[8:16])
		copy(postNonce[:], write.PostValue[8:16])

		changes[write.Address] = append(changes[write.Address], NonceChange{
			BlockAccessIndex: write.TxIndex,
			PreNonce:         preNonce,
			PostNonce:        postNonce,
		})
	}

	return changes
}

// buildCodeChanges extracts code deployments from CodeHash writes
// Cross-references BasicData writes to extract code size from bytes 5-7 (3-byte big-endian uint24)
func buildCodeChanges(writes []WriteEntry) map[common.Address][]CodeChange {
	changes := make(map[common.Address][]CodeChange)

	// First pass: Extract code sizes from BasicData writes (offset 5-7)
	codeSizeMap := make(map[common.Address]uint32)
	for _, write := range writes {
		if write.KeyType != KeyTypeBasicData {
			continue
		}

		// BasicData PostValue contains code_size at bytes 5-7 (3-byte big-endian)
		// Per EIP-6800: offset 0=version, 5-7=code_size, 8-15=nonce, 16-31=balance
		postValue := write.PostValue[:]
		codeSize := (uint32(postValue[5]) << 16) | (uint32(postValue[6]) << 8) | uint32(postValue[7])
		codeSizeMap[write.Address] = codeSize
	}

	// Second pass: Build CodeChange entries with code size from BasicData
	for _, write := range writes {
		if write.KeyType != KeyTypeCodeHash {
			continue
		}

		// Get code size from BasicData cross-reference (default 0 if not found)
		codeSize := codeSizeMap[write.Address]

		// PostValue contains the code hash
		changes[write.Address] = append(changes[write.Address], CodeChange{
			BlockAccessIndex: write.TxIndex,
			CodeHash:         write.PostValue,
			CodeSize:         codeSize,
		})
	}

	return changes
}

// classifyStorageAccesses separates read-only storage accesses from modified ones
func classifyStorageAccesses(
	reads []ReadEntry,
	writes []WriteEntry,
) map[common.Address][]StorageRead {
	// Build write set for quick lookup
	writeSet := make(map[common.Address]map[common.Hash]struct{})
	for _, write := range writes {
		if write.KeyType != KeyTypeStorage {
			continue
		}
		if writeSet[write.Address] == nil {
			writeSet[write.Address] = make(map[common.Hash]struct{})
		}
		writeSet[write.Address][write.StorageKey] = struct{}{}
	}

	// Classify reads as read-only if not written
	readOnly := make(map[common.Address][]StorageRead)
	for _, read := range reads {
		if read.KeyType != KeyTypeStorage {
			continue
		}

		// Check if this key was written
		if writeSet[read.Address] != nil {
			if _, written := writeSet[read.Address][read.StorageKey]; written {
				continue // Was written, not read-only
			}
		}

		readOnly[read.Address] = append(readOnly[read.Address], StorageRead{
			Key: read.StorageKey,
		})
	}

	return readOnly
}

// assembleAccountChanges merges all change maps into sorted AccountChanges list
func assembleAccountChanges(
	storageReadsMap map[common.Address][]StorageRead,
	slotChangesMap map[common.Address]map[common.Hash][]SlotWrite,
	balanceChangesMap map[common.Address][]BalanceChange,
	nonceChangesMap map[common.Address][]NonceChange,
	codeChangesMap map[common.Address][]CodeChange,
) []AccountChanges {
	// Collect all touched addresses
	addressSet := make(map[common.Address]struct{})
	for addr := range storageReadsMap {
		addressSet[addr] = struct{}{}
	}
	for addr := range slotChangesMap {
		addressSet[addr] = struct{}{}
	}
	for addr := range balanceChangesMap {
		addressSet[addr] = struct{}{}
	}
	for addr := range nonceChangesMap {
		addressSet[addr] = struct{}{}
	}
	for addr := range codeChangesMap {
		addressSet[addr] = struct{}{}
	}

	// Sort addresses lexicographically
	addresses := make([]common.Address, 0, len(addressSet))
	for addr := range addressSet {
		addresses = append(addresses, addr)
	}
	sort.Slice(addresses, func(i, j int) bool {
		return bytes.Compare(addresses[i][:], addresses[j][:]) < 0
	})

	// Build AccountChanges per address
	accounts := make([]AccountChanges, 0, len(addresses))
	for _, addr := range addresses {
		// Sort storage reads by key
		storageReads := storageReadsMap[addr]
		sort.Slice(storageReads, func(i, j int) bool {
			return bytes.Compare(storageReads[i].Key[:], storageReads[j].Key[:]) < 0
		})

		// Build sorted SlotChanges
		slotMap := slotChangesMap[addr]
		storageKeys := make([]common.Hash, 0, len(slotMap))
		for key := range slotMap {
			storageKeys = append(storageKeys, key)
		}
		sort.Slice(storageKeys, func(i, j int) bool {
			return bytes.Compare(storageKeys[i][:], storageKeys[j][:]) < 0
		})

		slotChanges := make([]SlotChanges, 0, len(storageKeys))
		for _, key := range storageKeys {
			writes := slotMap[key]
			// Sort writes by blockAccessIndex
			sort.Slice(writes, func(i, j int) bool {
				return writes[i].BlockAccessIndex < writes[j].BlockAccessIndex
			})
			slotChanges = append(slotChanges, SlotChanges{
				Key:    key,
				Writes: writes,
			})
		}

		// Sort balance/nonce/code changes by blockAccessIndex
		balanceChanges := balanceChangesMap[addr]
		sort.Slice(balanceChanges, func(i, j int) bool {
			return balanceChanges[i].BlockAccessIndex < balanceChanges[j].BlockAccessIndex
		})

		nonceChanges := nonceChangesMap[addr]
		sort.Slice(nonceChanges, func(i, j int) bool {
			return nonceChanges[i].BlockAccessIndex < nonceChanges[j].BlockAccessIndex
		})

		codeChanges := codeChangesMap[addr]
		sort.Slice(codeChanges, func(i, j int) bool {
			return codeChanges[i].BlockAccessIndex < codeChanges[j].BlockAccessIndex
		})

		accounts = append(accounts, AccountChanges{
			Address:        addr,
			StorageReads:   storageReads,
			StorageChanges: slotChanges,
			BalanceChanges: balanceChanges,
			NonceChanges:   nonceChanges,
			CodeChanges:    codeChanges,
		})
	}

	return accounts
}

// BuildBlockAccessList constructs BAL from augmented UBT witness
//
// Process:
// 1. Parse augmented pre-state witness for reads (161-byte entries with txIndex)
// 2. Parse augmented post-state witness for writes (161-byte entries with txIndex)
// 3. Build changes using txIndex as blockAccessIndex
// 4. Classify into read-only vs modified
// 5. Sort and deduplicate
// 6. Return BlockAccessList with proper ordering
//
// Note: Uses witness metadata only - guarantor can call this without execution state.
func BuildBlockAccessList(
	preStateWitness []byte,
	postStateWitness []byte,
) (*BlockAccessList, error) {
	// Parse augmented witness entries
	reads, err := parseAugmentedPreStateReads(preStateWitness)
	if err != nil {
		return nil, fmt.Errorf("failed to parse pre-state witness: %w", err)
	}

	writes, err := parseAugmentedPostStateWrites(postStateWitness)
	if err != nil {
		return nil, fmt.Errorf("failed to parse post-state witness: %w", err)
	}

	// Build changes from witness entries
	slotChangesMap := buildSlotChanges(writes)
	balanceChangesMap := buildBalanceChanges(writes)
	nonceChangesMap := buildNonceChanges(writes)
	codeChangesMap := buildCodeChanges(writes)

	// Classify storage reads
	storageReadsMap := classifyStorageAccesses(reads, writes)

	// Assemble AccountChanges with deterministic ordering
	accounts := assembleAccountChanges(
		storageReadsMap,
		slotChangesMap,
		balanceChangesMap,
		nonceChangesMap,
		codeChangesMap,
	)

	return &BlockAccessList{Accounts: accounts}, nil
}

// splitWitnessSections parses the dual-proof witness into pre/post sections
func SplitWitnessSections(witness []byte) (preState, postState []byte, err error) {
	// Parse witness structure:
	// [32B pre_root][4B read_count][read_entries...][4B pre_proof_len][pre_proof...]
	// [32B post_root][4B write_count][write_entries...][4B post_proof_len][post_proof...]

	if len(witness) < 36 {
		return nil, nil, fmt.Errorf("witness too short: %d bytes", len(witness))
	}

	offset := 0

	// Read count
	readCount := binary.BigEndian.Uint32(witness[32:36])
	offset = 36 + int(readCount)*161 // 161 bytes per entry

	// Pre-proof length
	if offset+4 > len(witness) {
		return nil, nil, fmt.Errorf("invalid witness: missing pre_proof_len at offset %d", offset)
	}
	preProofLen := binary.BigEndian.Uint32(witness[offset : offset+4])
	offset += 4 + int(preProofLen)

	if offset+4 > len(witness) {
		return nil, nil, fmt.Errorf("invalid witness: missing pre_extension_count at offset %d", offset)
	}
	extCount := binary.BigEndian.Uint32(witness[offset : offset+4])
	offset += 4
	extSize := int(extCount) * (32 + 31 + 32 + 2 + 248*32)
	offset += extSize

	if offset > len(witness) {
		return nil, nil, fmt.Errorf("invalid witness: pre-section overflow at offset %d", offset)
	}

	// Pre-state section ends here
	preState = witness[:offset]

	// Post-state section starts here
	postState = witness[offset:]

	return preState, postState, nil
}
