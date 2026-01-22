package storage

import (
	"sync"

	"github.com/colorfulnotion/jam/common"
)

// JAMTrieBackend holds shared infrastructure for JAM trie operations.
// Multiple sessions can share a single backend.
type JAMTrieBackend struct {
	db         *PersistenceStore      // Raw persistence layer (currently unused, in-memory only)
	writeBatch map[common.Hash][]byte // In-memory node cache
	batchMutex sync.Mutex             // Protects writeBatch
	writeMutex sync.Mutex             // Serializes Flush operations
}

func NewJAMTrieBackend(db *PersistenceStore) *JAMTrieBackend {
	return &JAMTrieBackend{
		db:         db,
		writeBatch: make(map[common.Hash][]byte),
	}
}

func (b *JAMTrieBackend) GetNode(hash common.Hash) ([]byte, bool, error) {
	b.batchMutex.Lock()
	defer b.batchMutex.Unlock()

	if value, exists := b.writeBatch[hash]; exists {
		return value, true, nil
	}
	return nil, false, nil
}

// GetNodeLocked retrieves a node. Caller must hold batchMutex.
func (b *JAMTrieBackend) GetNodeLocked(hash common.Hash) ([]byte, bool, error) {
	if value, exists := b.writeBatch[hash]; exists {
		return value, true, nil
	}
	return nil, false, nil
}

func (b *JAMTrieBackend) PutNode(hash common.Hash, data []byte) {
	b.batchMutex.Lock()
	defer b.batchMutex.Unlock()

	b.writeBatch[hash] = data
}

// PutNodeLocked adds a node. Caller must hold batchMutex.
func (b *JAMTrieBackend) PutNodeLocked(hash common.Hash, data []byte) {
	b.writeBatch[hash] = data
}

// Flush is currently a no-op (data stays in memory).
func (b *JAMTrieBackend) Flush() error {
	b.writeMutex.Lock()
	defer b.writeMutex.Unlock()
	return nil
}

func (b *JAMTrieBackend) GetBatchSize() int {
	b.batchMutex.Lock()
	defer b.batchMutex.Unlock()
	return len(b.writeBatch)
}

func (b *JAMTrieBackend) GetBatchInfo(hash common.Hash) (size int, exists bool) {
	b.batchMutex.Lock()
	defer b.batchMutex.Unlock()
	_, exists = b.writeBatch[hash]
	return len(b.writeBatch), exists
}

func (b *JAMTrieBackend) CopyWriteBatch() map[common.Hash][]byte {
	b.batchMutex.Lock()
	defer b.batchMutex.Unlock()

	copy := make(map[common.Hash][]byte, len(b.writeBatch))
	for k, v := range b.writeBatch {
		valueCopy := make([]byte, len(v))
		copyBytes(valueCopy, v)
		copy[k] = valueCopy
	}
	return copy
}

func copyBytes(dst, src []byte) {
	copy(dst, src)
}

func (b *JAMTrieBackend) Lock() {
	b.batchMutex.Lock()
}

func (b *JAMTrieBackend) Unlock() {
	b.batchMutex.Unlock()
}
