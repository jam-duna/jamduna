package storage

import (
	"fmt"

	evmtypes "github.com/colorfulnotion/jam/builder/evm/types"
	"github.com/colorfulnotion/jam/common"
)

// InitWitnessCache initializes the witness cache maps
func (store *StorageHub) InitWitnessCache() {
	store.Witness.mutex.Lock()
	defer store.Witness.mutex.Unlock()

	if store.Witness.Storageshard == nil {
		store.Witness.Storageshard = make(map[common.Address]evmtypes.ContractStorage)
	}
	if store.Witness.Code == nil {
		store.Witness.Code = make(map[common.Address][]byte)
	}
	if store.Witness.UBTProofs == nil {
		store.Witness.UBTProofs = make(map[common.Address]evmtypes.UBTMultiproof)
	}
}

// SetContractStorage stores contract storage in the witness cache
func (store *StorageHub) SetContractStorage(address common.Address, storage interface{}) {
	store.Witness.mutex.Lock()
	defer store.Witness.mutex.Unlock()

	if store.Witness.Storageshard == nil {
		store.Witness.Storageshard = make(map[common.Address]evmtypes.ContractStorage)
	}
	// Type assert to evmtypes.ContractStorage
	if cs, ok := storage.(evmtypes.ContractStorage); ok {
		store.Witness.Storageshard[address] = cs
		fmt.Printf("🔍 SetContractStorage called: address=%s, entries=%d\n", address.Hex(), len(cs.Shard.Entries))
	} else {
		fmt.Printf("❌ SetContractStorage type assertion failed for address=%s\n", address.Hex())
	}
}

// GetContractStorage retrieves contract storage from the witness cache
// Returns (storage, found) where storage is evmtypes.ContractStorage wrapped in interface{}
func (store *StorageHub) GetContractStorage(address common.Address) (interface{}, bool) {
	store.Witness.mutex.RLock()
	defer store.Witness.mutex.RUnlock()

	if store.Witness.Storageshard == nil {
		return nil, false
	}
	storage, found := store.Witness.Storageshard[address]
	if !found {
		return nil, false
	}
	return storage, true
}

// SetCode stores bytecode in the witness cache
func (store *StorageHub) SetCode(address common.Address, code []byte) {
	store.Witness.mutex.Lock()
	defer store.Witness.mutex.Unlock()

	if store.Witness.Code == nil {
		store.Witness.Code = make(map[common.Address][]byte)
	}
	store.Witness.Code[address] = code
}

// GetCode retrieves bytecode from the witness cache
// Returns (code, found)
func (store *StorageHub) GetCode(address common.Address) ([]byte, bool) {
	store.Witness.mutex.RLock()
	defer store.Witness.mutex.RUnlock()

	if store.Witness.Code == nil {
		return nil, false
	}
	code, found := store.Witness.Code[address]
	return code, found
}

// ReadStorageFromCache reads a single storage value from the witness cache
// Returns (value, found) where found=true if the value was in the cache
func (store *StorageHub) ReadStorageFromCache(contractAddress common.Address, storageKey common.Hash) (common.Hash, bool) {
	store.Witness.mutex.RLock()
	defer store.Witness.mutex.RUnlock()

	if store.Witness.Storageshard == nil {
		return common.Hash{}, false
	}

	contractStorage, exists := store.Witness.Storageshard[contractAddress]
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
func (store *StorageHub) ClearWitnessCache() {
	store.Witness.mutex.Lock()
	defer store.Witness.mutex.Unlock()

	store.Witness.Storageshard = make(map[common.Address]evmtypes.ContractStorage)
	store.Witness.Code = make(map[common.Address][]byte)
	store.Witness.UBTProofs = make(map[common.Address]evmtypes.UBTMultiproof)
}
