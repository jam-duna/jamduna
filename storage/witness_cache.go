package storage

import (
	"fmt"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/statedb/evmtypes"
)

// InitWitnessCache initializes the witness cache maps
func (store *StateDBStorage) InitWitnessCache() {
	store.witnessMutex.Lock()
	defer store.witnessMutex.Unlock()

	if store.Storageshard == nil {
		store.Storageshard = make(map[common.Address]evmtypes.ContractStorage)
	}
	if store.Code == nil {
		store.Code = make(map[common.Address][]byte)
	}
	if store.VerkleProofs == nil {
		store.VerkleProofs = make(map[common.Address]evmtypes.VerkleMultiproof)
	}
}

// SetContractStorage stores contract storage in the witness cache
func (store *StateDBStorage) SetContractStorage(address common.Address, storage interface{}) {
	store.witnessMutex.Lock()
	defer store.witnessMutex.Unlock()

	if store.Storageshard == nil {
		store.Storageshard = make(map[common.Address]evmtypes.ContractStorage)
	}
	// Type assert to evmtypes.ContractStorage
	if cs, ok := storage.(evmtypes.ContractStorage); ok {
		store.Storageshard[address] = cs
		fmt.Printf("🔍 SetContractStorage called: address=%s, entries=%d\n", address.Hex(), len(cs.Shard.Entries))
	} else {
		fmt.Printf("❌ SetContractStorage type assertion failed for address=%s\n", address.Hex())
	}
}

// GetContractStorage retrieves contract storage from the witness cache
// Returns (storage, found) where storage is evmtypes.ContractStorage wrapped in interface{}
func (store *StateDBStorage) GetContractStorage(address common.Address) (interface{}, bool) {
	store.witnessMutex.RLock()
	defer store.witnessMutex.RUnlock()

	if store.Storageshard == nil {
		return nil, false
	}
	storage, found := store.Storageshard[address]
	if !found {
		return nil, false
	}
	return storage, true
}

// SetCode stores bytecode in the witness cache
func (store *StateDBStorage) SetCode(address common.Address, code []byte) {
	store.witnessMutex.Lock()
	defer store.witnessMutex.Unlock()

	if store.Code == nil {
		store.Code = make(map[common.Address][]byte)
	}
	store.Code[address] = code
}

// GetCode retrieves bytecode from the witness cache
// Returns (code, found)
func (store *StateDBStorage) GetCode(address common.Address) ([]byte, bool) {
	store.witnessMutex.RLock()
	defer store.witnessMutex.RUnlock()

	if store.Code == nil {
		return nil, false
	}
	code, found := store.Code[address]
	return code, found
}

// ReadStorageFromCache reads a single storage value from the witness cache
// Returns (value, found) where found=true if the value was in the cache
func (store *StateDBStorage) ReadStorageFromCache(contractAddress common.Address, storageKey common.Hash) (common.Hash, bool) {
	store.witnessMutex.RLock()
	defer store.witnessMutex.RUnlock()

	if store.Storageshard == nil {
		return common.Hash{}, false
	}

	contractStorage, exists := store.Storageshard[contractAddress]
	if !exists {
		return common.Hash{}, false
	}

	// Post-SSR: Search single shard for the storage key
	for _, entry := range contractStorage.Shard.Entries {
		if entry.KeyH == storageKey {
			return entry.Value, true
		}
	}

	// Key not found in shard - return zero value (empty storage slot)
	return common.Hash{}, true // Return zero with found=true (valid empty slot)
}

// ClearWitnessCache clears all witness caches
func (store *StateDBStorage) ClearWitnessCache() {
	store.witnessMutex.Lock()
	defer store.witnessMutex.Unlock()

	store.Storageshard = make(map[common.Address]evmtypes.ContractStorage)
	store.Code = make(map[common.Address][]byte)
	store.VerkleProofs = make(map[common.Address]evmtypes.VerkleMultiproof)
}
