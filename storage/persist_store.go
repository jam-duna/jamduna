package storage

import (
	"fmt"

	"github.com/jam-duna/jamduna/common"
	"github.com/syndtr/goleveldb/leveldb"
	leveldbstorage "github.com/syndtr/goleveldb/leveldb/storage"
)

// PersistenceStore wraps LevelDB for raw key-value persistence.
// This is the foundational persistence layer - no trie logic here.
// Thread-safe: LevelDB handles its own synchronization.
type PersistenceStore struct {
	db *leveldb.DB
}

// NewPersistenceStore opens or creates a LevelDB database at the given path.
// If path is empty, uses in-memory storage.
func NewPersistenceStore(path string) (*PersistenceStore, error) {
	var db *leveldb.DB
	var err error

	if path == "" {
		memStorage := leveldbstorage.NewMemStorage()
		db, err = leveldb.Open(memStorage, nil)
	} else {
		db, err = leveldb.OpenFile(path, nil)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to open database at %s: %w", path, err)
	}

	return &PersistenceStore{db: db}, nil
}

// NewMemoryPersistenceStore creates an in-memory PersistenceStore for testing.
func NewMemoryPersistenceStore() (*PersistenceStore, error) {
	return NewPersistenceStore("")
}

// Get retrieves a value by key. Returns (nil, false, nil) if not found.
func (ps *PersistenceStore) Get(key []byte) ([]byte, bool, error) {
	data, err := ps.db.Get(key, nil)
	if err == leveldb.ErrNotFound {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("Get %x: %w", key, err)
	}
	return data, true, nil
}

func (ps *PersistenceStore) Put(key []byte, value []byte) error {
	return ps.db.Put(key, value, nil)
}

func (ps *PersistenceStore) Delete(key []byte) error {
	return ps.db.Delete(key, nil)
}

// GetWithPrefix returns all key-value pairs with the given prefix.
// Returns pairs sorted by key order.
func (ps *PersistenceStore) GetWithPrefix(prefix []byte) ([][2][]byte, error) {
	iter := ps.db.NewIterator(nil, nil)
	defer iter.Release()

	var results [][2][]byte

	// Seek to the first key that might have the prefix
	for ok := iter.Seek(prefix); ok; ok = iter.Next() {
		key := iter.Key()

		// Check if still within prefix range
		if len(key) < len(prefix) {
			break
		}
		match := true
		for i := 0; i < len(prefix); i++ {
			if key[i] != prefix[i] {
				match = false
				break
			}
		}
		if !match {
			break
		}

		// Copy key and value to avoid iterator reuse issues
		keyCopy := make([]byte, len(key))
		copy(keyCopy, key)
		valueCopy := make([]byte, len(iter.Value()))
		copy(valueCopy, iter.Value())

		results = append(results, [2][]byte{keyCopy, valueCopy})
	}

	if err := iter.Error(); err != nil {
		return nil, fmt.Errorf("GetWithPrefix %x: %w", prefix, err)
	}

	return results, nil
}

// GetHash returns error if not found (unlike Get which returns found=false).
func (ps *PersistenceStore) GetHash(key common.Hash) ([]byte, error) {
	data, err := ps.db.Get(key.Bytes(), nil)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (ps *PersistenceStore) PutHash(key common.Hash, value []byte) error {
	return ps.db.Put(key.Bytes(), value, nil)
}

func (ps *PersistenceStore) DeleteHash(key common.Hash) error {
	return ps.db.Delete(key.Bytes(), nil)
}

func (ps *PersistenceStore) Close() error {
	return ps.db.Close()
}

// DB returns the underlying LevelDB instance for advanced operations.
// Use sparingly - prefer the wrapper methods.
func (ps *PersistenceStore) DB() *leveldb.DB {
	return ps.db
}
