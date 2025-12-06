package storage

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/colorfulnotion/jam/common"
	"github.com/ethereum/go-ethereum/rlp"
)

// TestBlockAccessListRLPEncoding verifies RLP encoding round-trip
func TestBlockAccessListRLPEncoding(t *testing.T) {
	// Create a simple BAL with one account and one balance change
	var preBalance, postBalance common.Hash
	// 100 wei in last 16 bytes (big-endian uint128)
	postBalance[31] = 0x64

	bal := &BlockAccessList{
		Accounts: []AccountChanges{
			{
				Address: common.HexToAddress("0x1234567890123456789012345678901234567890"),
				BalanceChanges: []BalanceChange{
					{
						BlockAccessIndex: 1,
						PreBalance:       preBalance,
						PostBalance:      postBalance,
					},
				},
			},
		},
	}

	// Encode
	encoded, err := rlp.EncodeToBytes(bal)
	if err != nil {
		t.Fatalf("RLP encoding failed: %v", err)
	}

	// Decode
	var decoded BlockAccessList
	err = rlp.DecodeBytes(encoded, &decoded)
	if err != nil {
		t.Fatalf("RLP decoding failed: %v", err)
	}

	// Verify structure
	if len(decoded.Accounts) != 1 {
		t.Fatalf("Expected 1 account, got %d", len(decoded.Accounts))
	}

	if decoded.Accounts[0].Address != bal.Accounts[0].Address {
		t.Errorf("Address mismatch: expected %x, got %x",
			bal.Accounts[0].Address, decoded.Accounts[0].Address)
	}

	if len(decoded.Accounts[0].BalanceChanges) != 1 {
		t.Fatalf("Expected 1 balance change, got %d", len(decoded.Accounts[0].BalanceChanges))
	}

	if decoded.Accounts[0].BalanceChanges[0].BlockAccessIndex != 1 {
		t.Errorf("BlockAccessIndex mismatch: expected 1, got %d",
			decoded.Accounts[0].BalanceChanges[0].BlockAccessIndex)
	}

	if decoded.Accounts[0].BalanceChanges[0].PostBalance != postBalance {
		t.Errorf("PostBalance mismatch: expected %x, got %x",
			postBalance, decoded.Accounts[0].BalanceChanges[0].PostBalance)
	}
}

// TestBlockAccessListHashDeterminism verifies hash is deterministic
func TestBlockAccessListHashDeterminism(t *testing.T) {
	createBAL := func() *BlockAccessList {
		var balance common.Hash
		balance[31] = 0x64 // 100 wei

		return &BlockAccessList{
			Accounts: []AccountChanges{
				{
					Address: common.HexToAddress("0x1234"),
					BalanceChanges: []BalanceChange{
						{
							BlockAccessIndex: 1,
							PreBalance:       common.Hash{},
							PostBalance:      balance,
						},
					},
				},
			},
		}
	}

	// Create two identical BALs
	bal1 := createBAL()
	bal2 := createBAL()

	// Verify same content produces same hash
	hash1 := bal1.Hash()
	hash2 := bal2.Hash()

	if hash1 != hash2 {
		t.Errorf("Hash not deterministic: %x != %x", hash1, hash2)
	}

	// Run 100 times to ensure no randomness
	for i := 0; i < 100; i++ {
		balN := createBAL()
		hashN := balN.Hash()
		if hashN != hash1 {
			t.Errorf("Hash not deterministic on iteration %d: %x != %x", i, hashN, hash1)
		}
	}
}

// TestEmptyBlockAccessListHash verifies empty BAL hash
func TestEmptyBlockAccessListHash(t *testing.T) {
	bal := &BlockAccessList{Accounts: []AccountChanges{}}
	hash := bal.Hash()

	// Compute expected hash by encoding empty BAL
	encoded, err := rlp.EncodeToBytes(bal)
	if err != nil {
		t.Fatalf("RLP encoding failed: %v", err)
	}

	expectedHash := common.Blake2Hash(encoded)

	if hash != expectedHash {
		t.Errorf("Empty BAL hash mismatch: expected %x, got %x", expectedHash, hash)
	}

	// Verify it's not a zero hash
	if hash == (common.Hash{}) {
		t.Error("Empty BAL should not have zero hash")
	}
}

// TestBlockAccessListWithMultipleAccounts verifies sorting and ordering
func TestBlockAccessListWithMultipleAccounts(t *testing.T) {
	addr1 := common.HexToAddress("0x1111")
	addr2 := common.HexToAddress("0x2222")

	bal := &BlockAccessList{
		Accounts: []AccountChanges{
			{
				Address: addr2, // Intentionally out of order
				NonceChanges: []NonceChange{
					{BlockAccessIndex: 1, PreNonce: [8]byte{}, PostNonce: [8]byte{1}},
				},
			},
			{
				Address: addr1,
				NonceChanges: []NonceChange{
					{BlockAccessIndex: 1, PreNonce: [8]byte{}, PostNonce: [8]byte{2}},
				},
			},
		},
	}

	// Hash should be deterministic regardless of input order
	hash1 := bal.Hash()

	// Reverse order
	bal.Accounts[0], bal.Accounts[1] = bal.Accounts[1], bal.Accounts[0]
	hash2 := bal.Hash()

	// Should be different because we're hashing the actual order, not sorted
	// (sorting happens in assembleAccountChanges, not in Hash())
	if hash1 == hash2 {
		t.Error("Expected different hashes for different account orders")
	}
}

// TestSlotWriteOrdering verifies multiple writes to same slot are ordered
func TestSlotWriteOrdering(t *testing.T) {
	storageKey := common.HexToHash("0xabcd")

	bal := &BlockAccessList{
		Accounts: []AccountChanges{
			{
				Address: common.HexToAddress("0x1234"),
				StorageChanges: []SlotChanges{
					{
						Key: storageKey,
						Writes: []SlotWrite{
							{BlockAccessIndex: 1, PreValue: common.Hash{}, PostValue: common.HexToHash("0x01")},
							{BlockAccessIndex: 2, PreValue: common.HexToHash("0x01"), PostValue: common.HexToHash("0x02")},
							{BlockAccessIndex: 3, PreValue: common.HexToHash("0x02"), PostValue: common.HexToHash("0x03")},
						},
					},
				},
			},
		},
	}

	hash1 := bal.Hash()

	// Reverse write order
	writes := bal.Accounts[0].StorageChanges[0].Writes
	writes[0], writes[2] = writes[2], writes[0]
	hash2 := bal.Hash()

	// Hashes should be different (order matters)
	if hash1 == hash2 {
		t.Error("Expected different hashes for different write orders")
	}
}

// TestParseAugmentedPreStateReads verifies witness parsing
func TestParseAugmentedPreStateReads(t *testing.T) {
	// Build a fake pre-state witness with 1 entry
	witness := make([]byte, 0)

	// Header: [32B pre_root][4B count]
	preRoot := common.HexToHash("0x1234")
	witness = append(witness, preRoot[:]...)

	count := uint32(1)
	countBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(countBytes, count)
	witness = append(witness, countBytes...)

	// Entry: 161 bytes
	// [32B verkle_key][1B key_type][20B address][8B extra]
	// [32B storage_key][32B pre_value][32B post_value][4B txIndex]

	verkleKey := common.HexToHash("0xaaaa")
	witness = append(witness, verkleKey[:]...)

	keyType := uint8(KeyTypeStorage)
	witness = append(witness, keyType)

	address := common.HexToAddress("0x5678")
	witness = append(witness, address[:]...)

	extra := uint64(0)
	extraBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(extraBytes, extra)
	witness = append(witness, extraBytes...)

	storageKey := common.HexToHash("0xbbbb")
	witness = append(witness, storageKey[:]...)

	preValue := common.HexToHash("0xcccc")
	witness = append(witness, preValue[:]...)

	postValue := common.HexToHash("0xdddd")
	witness = append(witness, postValue[:]...)

	txIndex := uint32(42)
	txIndexBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(txIndexBytes, txIndex)
	witness = append(witness, txIndexBytes...)

	// Add proof_len (4 bytes) to make it valid
	proofLen := uint32(0)
	proofLenBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(proofLenBytes, proofLen)
	witness = append(witness, proofLenBytes...)

	// Parse
	reads, err := parseAugmentedPreStateReads(witness)
	if err != nil {
		t.Fatalf("Parsing failed: %v", err)
	}

	if len(reads) != 1 {
		t.Fatalf("Expected 1 read entry, got %d", len(reads))
	}

	entry := reads[0]
	if entry.VerkleKey != verkleKey {
		t.Errorf("VerkleKey mismatch: expected %x, got %x", verkleKey, entry.VerkleKey)
	}

	if entry.KeyType != keyType {
		t.Errorf("KeyType mismatch: expected %d, got %d", keyType, entry.KeyType)
	}

	if entry.Address != address {
		t.Errorf("Address mismatch: expected %x, got %x", address, entry.Address)
	}

	if entry.StorageKey != storageKey {
		t.Errorf("StorageKey mismatch: expected %x, got %x", storageKey, entry.StorageKey)
	}

	if entry.PreValue != preValue {
		t.Errorf("PreValue mismatch: expected %x, got %x", preValue, entry.PreValue)
	}

	if entry.PostValue != postValue {
		t.Errorf("PostValue mismatch: expected %x, got %x", postValue, entry.PostValue)
	}

	if entry.TxIndex != txIndex {
		t.Errorf("TxIndex mismatch: expected %d, got %d", txIndex, entry.TxIndex)
	}
}

// TestBuildSlotChanges verifies storage write grouping
func TestBuildSlotChanges(t *testing.T) {
	addr := common.HexToAddress("0x1234")
	key1 := common.HexToHash("0xaaaa")
	key2 := common.HexToHash("0xbbbb")

	writes := []WriteEntry{
		{
			Address:    addr,
			KeyType:    KeyTypeStorage,
			StorageKey: key1,
			PreValue:   common.Hash{},
			PostValue:  common.HexToHash("0x01"),
			TxIndex:    1,
		},
		{
			Address:    addr,
			KeyType:    KeyTypeStorage,
			StorageKey: key1, // Same key, different tx
			PreValue:   common.HexToHash("0x01"),
			PostValue:  common.HexToHash("0x02"),
			TxIndex:    2,
		},
		{
			Address:    addr,
			KeyType:    KeyTypeStorage,
			StorageKey: key2, // Different key
			PreValue:   common.Hash{},
			PostValue:  common.HexToHash("0x03"),
			TxIndex:    1,
		},
	}

	slotMap := buildSlotChanges(writes)

	// Verify structure
	if len(slotMap) != 1 {
		t.Fatalf("Expected 1 address, got %d", len(slotMap))
	}

	addrSlots := slotMap[addr]
	if len(addrSlots) != 2 {
		t.Fatalf("Expected 2 storage keys, got %d", len(addrSlots))
	}

	// Verify key1 has 2 writes
	if len(addrSlots[key1]) != 2 {
		t.Errorf("Expected 2 writes for key1, got %d", len(addrSlots[key1]))
	}

	// Verify key2 has 1 write
	if len(addrSlots[key2]) != 1 {
		t.Errorf("Expected 1 write for key2, got %d", len(addrSlots[key2]))
	}

	// Verify txIndex ordering
	if addrSlots[key1][0].BlockAccessIndex != 1 {
		t.Errorf("First write should have txIndex 1, got %d", addrSlots[key1][0].BlockAccessIndex)
	}

	if addrSlots[key1][1].BlockAccessIndex != 2 {
		t.Errorf("Second write should have txIndex 2, got %d", addrSlots[key1][1].BlockAccessIndex)
	}
}

// TestClassifyStorageAccesses verifies read-only vs modified classification
func TestClassifyStorageAccesses(t *testing.T) {
	addr := common.HexToAddress("0x1234")
	readOnlyKey := common.HexToHash("0xaaaa")
	modifiedKey := common.HexToHash("0xbbbb")

	reads := []ReadEntry{
		{Address: addr, KeyType: KeyTypeStorage, StorageKey: readOnlyKey},
		{Address: addr, KeyType: KeyTypeStorage, StorageKey: modifiedKey},
	}

	writes := []WriteEntry{
		{Address: addr, KeyType: KeyTypeStorage, StorageKey: modifiedKey, TxIndex: 1},
	}

	readOnlyMap := classifyStorageAccesses(reads, writes)

	// Verify only readOnlyKey is in the map
	if len(readOnlyMap) != 1 {
		t.Fatalf("Expected 1 address with read-only accesses, got %d", len(readOnlyMap))
	}

	addrReads := readOnlyMap[addr]
	if len(addrReads) != 1 {
		t.Fatalf("Expected 1 read-only key, got %d", len(addrReads))
	}

	if addrReads[0].Key != readOnlyKey {
		t.Errorf("Expected read-only key %x, got %x", readOnlyKey, addrReads[0].Key)
	}
}

// TestAssembleAccountChanges verifies deterministic assembly and sorting
func TestAssembleAccountChanges(t *testing.T) {
	addr1 := common.HexToAddress("0x2222") // Higher address
	addr2 := common.HexToAddress("0x1111") // Lower address

	storageReadsMap := map[common.Address][]StorageRead{
		addr1: {{Key: common.HexToHash("0xaaaa")}},
	}

	slotChangesMap := map[common.Address]map[common.Hash][]SlotWrite{
		addr2: {
			common.HexToHash("0xbbbb"): {
				{BlockAccessIndex: 1, PreValue: common.Hash{}, PostValue: common.HexToHash("0x01")},
			},
		},
	}

	balanceChangesMap := map[common.Address][]BalanceChange{
		addr1: {{BlockAccessIndex: 1, PreBalance: common.Hash{}, PostBalance: common.HexToHash("0x64")}},
	}

	nonceChangesMap := map[common.Address][]NonceChange{}
	codeChangesMap := map[common.Address][]CodeChange{}

	accounts := assembleAccountChanges(
		storageReadsMap,
		slotChangesMap,
		balanceChangesMap,
		nonceChangesMap,
		codeChangesMap,
	)

	// Verify sorting: addr2 (0x1111) should come before addr1 (0x2222)
	if len(accounts) != 2 {
		t.Fatalf("Expected 2 accounts, got %d", len(accounts))
	}

	if accounts[0].Address != addr2 {
		t.Errorf("First account should be %x, got %x", addr2, accounts[0].Address)
	}

	if accounts[1].Address != addr1 {
		t.Errorf("Second account should be %x, got %x", addr1, accounts[1].Address)
	}

	// Verify addr2 has storage changes
	if len(accounts[0].StorageChanges) != 1 {
		t.Errorf("addr2 should have 1 storage change, got %d", len(accounts[0].StorageChanges))
	}

	// Verify addr1 has balance changes and storage reads
	if len(accounts[1].BalanceChanges) != 1 {
		t.Errorf("addr1 should have 1 balance change, got %d", len(accounts[1].BalanceChanges))
	}

	if len(accounts[1].StorageReads) != 1 {
		t.Errorf("addr1 should have 1 storage read, got %d", len(accounts[1].StorageReads))
	}
}

// TestSplitWitnessSections verifies dual-proof witness splitting
func TestSplitWitnessSections(t *testing.T) {
	// Build a fake dual-proof witness
	witness := make([]byte, 0)

	// Pre-state section
	preRoot := common.HexToHash("0x1111")
	witness = append(witness, preRoot[:]...)

	readCount := uint32(1)
	readCountBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(readCountBytes, readCount)
	witness = append(witness, readCountBytes...)

	// Add 1 entry (161 bytes)
	entry := make([]byte, 161)
	witness = append(witness, entry...)

	// Pre-proof length
	preProofLen := uint32(100)
	preProofLenBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(preProofLenBytes, preProofLen)
	witness = append(witness, preProofLenBytes...)

	// Pre-proof data
	preProof := make([]byte, 100)
	witness = append(witness, preProof...)

	preStateSectionLen := len(witness)

	// Post-state section
	postRoot := common.HexToHash("0x2222")
	witness = append(witness, postRoot[:]...)

	writeCount := uint32(2)
	writeCountBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(writeCountBytes, writeCount)
	witness = append(witness, writeCountBytes...)

	// Add 2 entries (161 bytes each)
	for i := 0; i < 2; i++ {
		entry := make([]byte, 161)
		witness = append(witness, entry...)
	}

	// Post-proof length
	postProofLen := uint32(200)
	postProofLenBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(postProofLenBytes, postProofLen)
	witness = append(witness, postProofLenBytes...)

	// Post-proof data
	postProof := make([]byte, 200)
	witness = append(witness, postProof...)

	// Split
	preState, postState, err := SplitWitnessSections(witness)
	if err != nil {
		t.Fatalf("Split failed: %v", err)
	}

	// Verify pre-state section length
	if len(preState) != preStateSectionLen {
		t.Errorf("Pre-state section length mismatch: expected %d, got %d",
			preStateSectionLen, len(preState))
	}

	// Verify post-state section length
	expectedPostLen := 32 + 4 + (161 * 2) + 4 + 200
	if len(postState) != expectedPostLen {
		t.Errorf("Post-state section length mismatch: expected %d, got %d",
			expectedPostLen, len(postState))
	}

	// Verify roots
	var preRootParsed, postRootParsed common.Hash
	copy(preRootParsed[:], preState[0:32])
	copy(postRootParsed[:], postState[0:32])

	if preRootParsed != preRoot {
		t.Errorf("Pre-root mismatch: expected %x, got %x", preRoot, preRootParsed)
	}

	if postRootParsed != postRoot {
		t.Errorf("Post-root mismatch: expected %x, got %x", postRoot, postRootParsed)
	}
}

// TestBuildBalanceChanges verifies balance extraction from BasicData
func TestBuildBalanceChanges(t *testing.T) {
	addr := common.HexToAddress("0x1234")

	// Create a BasicData write with balance in bytes 16-31
	var preValue, postValue common.Hash
	// Put 100 wei in bytes 16-31 (big-endian)
	binary.BigEndian.PutUint64(postValue[24:32], 100)

	writes := []WriteEntry{
		{
			Address:  addr,
			KeyType:  KeyTypeBasicData,
			PreValue: preValue,
			PostValue: postValue,
			TxIndex:  1,
		},
	}

	balanceMap := buildBalanceChanges(writes)

	if len(balanceMap) != 1 {
		t.Fatalf("Expected 1 address with balance changes, got %d", len(balanceMap))
	}

	changes := balanceMap[addr]
	if len(changes) != 1 {
		t.Fatalf("Expected 1 balance change, got %d", len(changes))
	}

	change := changes[0]
	if change.BlockAccessIndex != 1 {
		t.Errorf("Expected BlockAccessIndex 1, got %d", change.BlockAccessIndex)
	}

	// Verify PostBalance has the value in bytes 16-31
	expectedBalance := common.Hash{}
	copy(expectedBalance[16:], postValue[16:])

	if !bytes.Equal(change.PostBalance[:], expectedBalance[:]) {
		t.Errorf("PostBalance mismatch: expected %x, got %x",
			expectedBalance, change.PostBalance)
	}
}

// TestBuildCodeChanges verifies code size extraction from BasicData cross-reference
func TestBuildCodeChanges(t *testing.T) {
	addr := common.HexToAddress("0x1234567890123456789012345678901234567890")

	// Create BasicData write with code size = 1024 bytes in bytes 5-7 (3-byte big-endian)
	var basicDataPreValue, basicDataPostValue common.Hash
	codeSize := uint32(1024)
	basicDataPostValue[5] = byte(codeSize >> 16) // Most significant byte
	basicDataPostValue[6] = byte(codeSize >> 8)
	basicDataPostValue[7] = byte(codeSize) // Least significant byte

	// Create CodeHash write
	var codeHashPreValue, codeHashPostValue common.Hash
	codeHashPostValue[0] = 0xaa // Dummy code hash
	codeHashPostValue[31] = 0xbb

	writes := []WriteEntry{
		{
			Address:   addr,
			KeyType:   KeyTypeBasicData,
			PreValue:  basicDataPreValue,
			PostValue: basicDataPostValue,
			TxIndex:   1,
		},
		{
			Address:   addr,
			KeyType:   KeyTypeCodeHash,
			PreValue:  codeHashPreValue,
			PostValue: codeHashPostValue,
			TxIndex:   1,
		},
	}

	codeMap := buildCodeChanges(writes)

	if len(codeMap) != 1 {
		t.Fatalf("Expected 1 address with code changes, got %d", len(codeMap))
	}

	codeChanges, exists := codeMap[addr]
	if !exists {
		t.Fatalf("Expected code changes for address %s", addr.Hex())
	}

	if len(codeChanges) != 1 {
		t.Fatalf("Expected 1 code change, got %d", len(codeChanges))
	}

	cc := codeChanges[0]
	if cc.BlockAccessIndex != 1 {
		t.Errorf("Expected BlockAccessIndex 1, got %d", cc.BlockAccessIndex)
	}

	if cc.CodeHash != codeHashPostValue {
		t.Errorf("Code hash mismatch")
	}

	if cc.CodeSize != 1024 {
		t.Errorf("Expected CodeSize 1024, got %d", cc.CodeSize)
	}
}

// TestBuildCodeChangesWithoutBasicData verifies CodeSize defaults to 0 if BasicData not found
func TestBuildCodeChangesWithoutBasicData(t *testing.T) {
	addr := common.HexToAddress("0x1234567890123456789012345678901234567890")

	// Only CodeHash write, no BasicData
	var codeHashPreValue, codeHashPostValue common.Hash
	codeHashPostValue[0] = 0xaa

	writes := []WriteEntry{
		{
			Address:   addr,
			KeyType:   KeyTypeCodeHash,
			PreValue:  codeHashPreValue,
			PostValue: codeHashPostValue,
			TxIndex:   1,
		},
	}

	codeMap := buildCodeChanges(writes)

	if len(codeMap) != 1 {
		t.Fatalf("Expected 1 address with code changes, got %d", len(codeMap))
	}

	codeChanges, exists := codeMap[addr]
	if !exists {
		t.Fatalf("Expected code changes for address %s", addr.Hex())
	}

	if len(codeChanges) != 1 {
		t.Fatalf("Expected 1 code change, got %d", len(codeChanges))
	}

	cc := codeChanges[0]
	if cc.CodeSize != 0 {
		t.Errorf("Expected CodeSize 0 (default when BasicData not found), got %d", cc.CodeSize)
	}
}

// TestBuildBlockAccessListFromWitness verifies complete BAL construction from dual-proof witness
func TestBuildBlockAccessListFromWitness(t *testing.T) {
	// Build a complete dual-proof witness with reads and writes
	witness := make([]byte, 0)

	// ===== PRE-STATE SECTION (READS) =====
	preRoot := common.HexToHash("0x1111111111111111111111111111111111111111111111111111111111111111")
	witness = append(witness, preRoot[:]...)

	readCountBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(readCountBytes, 3) // 3 read entries
	witness = append(witness, readCountBytes...)

	// Read entry 1: Storage read for address 0xAAAA (read-only - never written)
	addr1 := common.HexToAddress("0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	storageKey1 := common.HexToHash("0xBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB")

	verkleKey1 := common.HexToHash("0xCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC")
	witness = append(witness, verkleKey1[:]...)
	witness = append(witness, byte(KeyTypeStorage)) // key_type
	witness = append(witness, addr1[:]...)          // address (20 bytes)
	witness = append(witness, make([]byte, 8)...)   // extra (8 bytes)
	witness = append(witness, storageKey1[:]...)    // storage_key (32 bytes)
	preValue1 := common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000042")
	witness = append(witness, preValue1[:]...)      // pre_value (32 bytes)
	witness = append(witness, preValue1[:]...)      // post_value (same = read-only) (32 bytes)
	txIndex1 := uint32(0)
	txIndex1Bytes := make([]byte, 4)
	binary.BigEndian.PutUint32(txIndex1Bytes, txIndex1)
	witness = append(witness, txIndex1Bytes...) // tx_index (4 bytes)

	// Read entry 1b: Different storage key for same address that WILL be written
	storageKey1b := common.HexToHash("0x3333333333333333333333333333333333333333333333333333333333333333")
	verkleKey1b := common.HexToHash("0x4444444444444444444444444444444444444444444444444444444444444444")
	witness = append(witness, verkleKey1b[:]...)
	witness = append(witness, byte(KeyTypeStorage))
	witness = append(witness, addr1[:]...)
	witness = append(witness, make([]byte, 8)...)
	witness = append(witness, storageKey1b[:]...)
	preValue1b := common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000011")
	witness = append(witness, preValue1b[:]...)
	witness = append(witness, preValue1b[:]...) // Same = read in pre-state
	witness = append(witness, make([]byte, 4)...)

	// Read entry 2: Balance read for address 0xDDDD
	addr2 := common.HexToAddress("0xDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDD")
	verkleKey2 := common.HexToHash("0xEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEE")
	witness = append(witness, verkleKey2[:]...)
	witness = append(witness, byte(KeyTypeBasicData)) // key_type
	witness = append(witness, addr2[:]...)            // address (20 bytes)
	witness = append(witness, make([]byte, 8)...)     // extra (8 bytes)
	witness = append(witness, make([]byte, 32)...)    // storage_key (unused for BasicData)
	var preBalance common.Hash
	binary.BigEndian.PutUint64(preBalance[24:32], 1000) // 1000 wei in bytes 16-31
	witness = append(witness, preBalance[:]...)         // pre_value
	witness = append(witness, preBalance[:]...)         // post_value (same = read-only)
	witness = append(witness, make([]byte, 4)...)       // tx_index

	// Pre-proof
	preProofLen := uint32(0)
	preProofLenBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(preProofLenBytes, preProofLen)
	witness = append(witness, preProofLenBytes...)

	// ===== POST-STATE SECTION (WRITES) =====
	postRoot := common.HexToHash("0x2222222222222222222222222222222222222222222222222222222222222222")
	witness = append(witness, postRoot[:]...)

	writeCount := uint32(2) // 2 write entries
	writeCountBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(writeCountBytes, writeCount)
	witness = append(witness, writeCountBytes...)

	// Write entry 1: Storage write for addr1 (modify storageKey1b, not storageKey1 which is read-only)
	witness = append(witness, verkleKey1b[:]...)
	witness = append(witness, byte(KeyTypeStorage))
	witness = append(witness, addr1[:]...)
	witness = append(witness, make([]byte, 8)...)
	witness = append(witness, storageKey1b[:]...)
	witness = append(witness, preValue1b[:]...)          // pre_value (0x11)
	postValue1b := common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000100")
	witness = append(witness, postValue1b[:]...)         // post_value (0x100)
	txIndex1Write := uint32(1)
	txIndex1WriteBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(txIndex1WriteBytes, txIndex1Write)
	witness = append(witness, txIndex1WriteBytes...)

	// Write entry 2: Balance write for addr2 (modify balance)
	witness = append(witness, verkleKey2[:]...)
	witness = append(witness, byte(KeyTypeBasicData))
	witness = append(witness, addr2[:]...)
	witness = append(witness, make([]byte, 8)...)
	witness = append(witness, make([]byte, 32)...)
	witness = append(witness, preBalance[:]...) // pre_value (1000 wei)
	var postBalance common.Hash
	binary.BigEndian.PutUint64(postBalance[24:32], 2000) // 2000 wei
	witness = append(witness, postBalance[:]...)         // post_value (2000 wei)
	txIndex2Write := uint32(2)
	txIndex2WriteBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(txIndex2WriteBytes, txIndex2Write)
	witness = append(witness, txIndex2WriteBytes...)

	// Post-proof
	postProofLen := uint32(0)
	postProofLenBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(postProofLenBytes, postProofLen)
	witness = append(witness, postProofLenBytes...)

	// ===== BUILD BAL =====
	preState, postState, err := SplitWitnessSections(witness)
	if err != nil {
		t.Fatalf("SplitWitnessSections failed: %v", err)
	}

	bal, err := BuildBlockAccessList(preState, postState)
	if err != nil {
		t.Fatalf("BuildBlockAccessList failed: %v", err)
	}

	// ===== VERIFY BAL STRUCTURE =====
	if len(bal.Accounts) != 2 {
		t.Fatalf("Expected 2 accounts in BAL, got %d", len(bal.Accounts))
	}

	// Accounts should be sorted by address
	// addr1 (0xAAAA...) should come before addr2 (0xDDDD...)
	if bal.Accounts[0].Address != addr1 {
		t.Errorf("First account should be %s, got %s", addr1.Hex(), bal.Accounts[0].Address.Hex())
	}

	if bal.Accounts[1].Address != addr2 {
		t.Errorf("Second account should be %s, got %s", addr2.Hex(), bal.Accounts[1].Address.Hex())
	}

	// Verify addr1 has 1 read-only storage access and 1 storage write
	acc1 := bal.Accounts[0]
	if len(acc1.StorageReads) != 1 {
		t.Errorf("Expected 1 storage read for addr1, got %d", len(acc1.StorageReads))
	}

	if acc1.StorageReads[0].Key != storageKey1 {
		t.Errorf("Storage read key mismatch: expected %s, got %s",
			storageKey1.Hex(), acc1.StorageReads[0].Key.Hex())
	}

	if len(acc1.StorageChanges) != 1 {
		t.Errorf("Expected 1 storage change for addr1, got %d", len(acc1.StorageChanges))
	}

	if acc1.StorageChanges[0].Key != storageKey1b {
		t.Errorf("Storage change key mismatch: expected %s, got %s",
			storageKey1b.Hex(), acc1.StorageChanges[0].Key.Hex())
	}

	if len(acc1.StorageChanges[0].Writes) != 1 {
		t.Errorf("Expected 1 write for storage slot, got %d", len(acc1.StorageChanges[0].Writes))
	}

	slotWrite := acc1.StorageChanges[0].Writes[0]
	if slotWrite.BlockAccessIndex != 1 {
		t.Errorf("Expected BlockAccessIndex 1, got %d", slotWrite.BlockAccessIndex)
	}

	if slotWrite.PreValue != preValue1b {
		t.Errorf("PreValue mismatch: expected %s, got %s", preValue1b.Hex(), slotWrite.PreValue.Hex())
	}

	if slotWrite.PostValue != postValue1b {
		t.Errorf("PostValue mismatch: expected %s, got %s", postValue1b.Hex(), slotWrite.PostValue.Hex())
	}

	// Verify addr2 has 1 balance change and no storage
	acc2 := bal.Accounts[1]
	if len(acc2.BalanceChanges) != 1 {
		t.Errorf("Expected 1 balance change for addr2, got %d", len(acc2.BalanceChanges))
	}

	balChange := acc2.BalanceChanges[0]
	if balChange.BlockAccessIndex != 2 {
		t.Errorf("Expected BlockAccessIndex 2, got %d", balChange.BlockAccessIndex)
	}

	// Extract balance from bytes 16-31
	var expectedPreBalance, expectedPostBalance common.Hash
	copy(expectedPreBalance[16:], preBalance[16:])
	copy(expectedPostBalance[16:], postBalance[16:])

	if balChange.PreBalance != expectedPreBalance {
		t.Errorf("PreBalance mismatch: expected %x, got %x",
			expectedPreBalance, balChange.PreBalance)
	}

	if balChange.PostBalance != expectedPostBalance {
		t.Errorf("PostBalance mismatch: expected %x, got %x",
			expectedPostBalance, balChange.PostBalance)
	}

	if len(acc2.StorageChanges) != 0 {
		t.Errorf("Expected no storage changes for addr2, got %d", len(acc2.StorageChanges))
	}
}

// TestBALHashRoundTrip verifies hash computation matches builder/guarantor agreement
func TestBALHashRoundTrip(t *testing.T) {
	// Create a realistic BAL with multiple accounts and change types
	addr1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	addr2 := common.HexToAddress("0x2222222222222222222222222222222222222222")
	addr3 := common.HexToAddress("0x3333333333333333333333333333333333333333")

	var balance1Pre, balance1Post common.Hash
	binary.BigEndian.PutUint64(balance1Post[24:32], 1000000)

	var balance2Pre, balance2Post common.Hash
	binary.BigEndian.PutUint64(balance2Pre[24:32], 5000000)
	binary.BigEndian.PutUint64(balance2Post[24:32], 4500000)

	codeHash := common.HexToHash("0xabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd")

	bal := &BlockAccessList{
		Accounts: []AccountChanges{
			{
				Address: addr1,
				BalanceChanges: []BalanceChange{
					{BlockAccessIndex: 1, PreBalance: balance1Pre, PostBalance: balance1Post},
				},
				StorageChanges: []SlotChanges{
					{
						Key: common.HexToHash("0xaaaa"),
						Writes: []SlotWrite{
							{BlockAccessIndex: 1, PreValue: common.Hash{}, PostValue: common.HexToHash("0x01")},
							{BlockAccessIndex: 2, PreValue: common.HexToHash("0x01"), PostValue: common.HexToHash("0x02")},
						},
					},
				},
			},
			{
				Address: addr2,
				BalanceChanges: []BalanceChange{
					{BlockAccessIndex: 3, PreBalance: balance2Pre, PostBalance: balance2Post},
				},
				NonceChanges: []NonceChange{
					{BlockAccessIndex: 3, PreNonce: [8]byte{}, PostNonce: [8]byte{0, 0, 0, 0, 0, 0, 0, 1}},
				},
				StorageReads: []StorageRead{
					{Key: common.HexToHash("0xbbbb")},
					{Key: common.HexToHash("0xcccc")},
				},
			},
			{
				Address: addr3,
				CodeChanges: []CodeChange{
					{BlockAccessIndex: 4, CodeHash: codeHash, CodeSize: 2048},
				},
			},
		},
	}

	// Compute hash (builder)
	builderHash := bal.Hash()

	// Encode to RLP (simulating DA storage)
	encoded, err := rlp.EncodeToBytes(bal)
	if err != nil {
		t.Fatalf("RLP encoding failed: %v", err)
	}

	// Decode (simulating guarantor reconstruction)
	var guarantorBAL BlockAccessList
	err = rlp.DecodeBytes(encoded, &guarantorBAL)
	if err != nil {
		t.Fatalf("RLP decoding failed: %v", err)
	}

	// Compute hash (guarantor)
	guarantorHash := guarantorBAL.Hash()

	// Hashes must match (fraud proof verification)
	if builderHash != guarantorHash {
		t.Errorf("BAL hash mismatch! Builder: %s, Guarantor: %s",
			builderHash.Hex(), guarantorHash.Hex())
	}

	// Verify using Blake2b directly
	expectedHash := common.Blake2Hash(encoded)
	if builderHash != expectedHash {
		t.Errorf("Hash doesn't match direct Blake2b: expected %s, got %s",
			expectedHash.Hex(), builderHash.Hex())
	}

	t.Logf("✅ BAL hash verified: %s", builderHash.Hex())
	t.Logf("   RLP encoded size: %d bytes", len(encoded))
	t.Logf("   Accounts: %d", len(bal.Accounts))
	t.Logf("   Total balance changes: %d", len(bal.Accounts[0].BalanceChanges)+len(bal.Accounts[1].BalanceChanges))
	t.Logf("   Total storage writes: %d", len(bal.Accounts[0].StorageChanges[0].Writes))
}

// TestParseAugmentedPostStateWrites verifies post-state (write) section parsing
func TestParseAugmentedPostStateWrites(t *testing.T) {
	// Build a fake post-state section with 2 write entries
	postState := make([]byte, 0)

	// Header: [32B post_root][4B count]
	postRoot := common.HexToHash("0x9999")
	postState = append(postState, postRoot[:]...)

	count := uint32(2)
	countBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(countBytes, count)
	postState = append(postState, countBytes...)

	// Entry 1: Balance write
	addr1 := common.HexToAddress("0xAAAA")
	verkleKey1 := common.HexToHash("0xBBBB")

	postState = append(postState, verkleKey1[:]...)
	postState = append(postState, byte(KeyTypeBasicData))
	postState = append(postState, addr1[:]...)
	postState = append(postState, make([]byte, 8)...)
	postState = append(postState, make([]byte, 32)...)

	var preBalance, postBalance common.Hash
	binary.BigEndian.PutUint64(postBalance[24:32], 5000)
	postState = append(postState, preBalance[:]...)
	postState = append(postState, postBalance[:]...)

	txIndex1 := uint32(10)
	txIndex1Bytes := make([]byte, 4)
	binary.BigEndian.PutUint32(txIndex1Bytes, txIndex1)
	postState = append(postState, txIndex1Bytes...)

	// Entry 2: Storage write
	addr2 := common.HexToAddress("0xCCCC")
	storageKey2 := common.HexToHash("0xDDDD")
	verkleKey2 := common.HexToHash("0xEEEE")

	postState = append(postState, verkleKey2[:]...)
	postState = append(postState, byte(KeyTypeStorage))
	postState = append(postState, addr2[:]...)
	postState = append(postState, make([]byte, 8)...)
	postState = append(postState, storageKey2[:]...)

	preValue2 := common.HexToHash("0x1111")
	postValue2 := common.HexToHash("0x2222")
	postState = append(postState, preValue2[:]...)
	postState = append(postState, postValue2[:]...)

	txIndex2 := uint32(20)
	txIndex2Bytes := make([]byte, 4)
	binary.BigEndian.PutUint32(txIndex2Bytes, txIndex2)
	postState = append(postState, txIndex2Bytes...)

	// Add proof_len (4 bytes)
	proofLen := uint32(0)
	proofLenBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(proofLenBytes, proofLen)
	postState = append(postState, proofLenBytes...)

	// Parse
	writes, err := parseAugmentedPostStateWrites(postState)
	if err != nil {
		t.Fatalf("Parsing failed: %v", err)
	}

	if len(writes) != 2 {
		t.Fatalf("Expected 2 write entries, got %d", len(writes))
	}

	// Verify entry 1 (balance)
	entry1 := writes[0]
	if entry1.Address != addr1 {
		t.Errorf("Entry1 address mismatch: expected %s, got %s", addr1.Hex(), entry1.Address.Hex())
	}

	if entry1.KeyType != KeyTypeBasicData {
		t.Errorf("Entry1 KeyType mismatch: expected %d, got %d", KeyTypeBasicData, entry1.KeyType)
	}

	if entry1.TxIndex != txIndex1 {
		t.Errorf("Entry1 TxIndex mismatch: expected %d, got %d", txIndex1, entry1.TxIndex)
	}

	if entry1.PostValue != postBalance {
		t.Errorf("Entry1 PostValue mismatch")
	}

	// Verify entry 2 (storage)
	entry2 := writes[1]
	if entry2.Address != addr2 {
		t.Errorf("Entry2 address mismatch: expected %s, got %s", addr2.Hex(), entry2.Address.Hex())
	}

	if entry2.KeyType != KeyTypeStorage {
		t.Errorf("Entry2 KeyType mismatch: expected %d, got %d", KeyTypeStorage, entry2.KeyType)
	}

	if entry2.StorageKey != storageKey2 {
		t.Errorf("Entry2 StorageKey mismatch")
	}

	if entry2.TxIndex != txIndex2 {
		t.Errorf("Entry2 TxIndex mismatch: expected %d, got %d", txIndex2, entry2.TxIndex)
	}

	if entry2.PreValue != preValue2 {
		t.Errorf("Entry2 PreValue mismatch")
	}

	if entry2.PostValue != postValue2 {
		t.Errorf("Entry2 PostValue mismatch")
	}
}
