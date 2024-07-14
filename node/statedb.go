package node

import (
	//"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/common"
	"github.com/syndtr/goleveldb/leveldb"
)

// StateDB interface defines methods for reading and writing key-value pairs
type StateDB interface {
	ReadKV(key common.Hash) ([]byte, error)
	WriteKV(key common.Hash, value []byte) error
}

// StateDBStorage struct to hold the LevelDB instance
type StateDBStorage struct {
	db *leveldb.DB
}

// NewStateDBStorage initializes a new LevelDB store
func NewStateDBStorage(path string) (*StateDBStorage, error) {
	db, err := leveldb.OpenFile(path, nil)
	if err != nil {
		return nil, err
	}
	return &StateDBStorage{db: db}, nil
}

// ReadKV reads a value for a given key from the LevelDB store
func (store *StateDBStorage) ReadKV(key common.Hash) ([]byte, error) {
	data, err := store.db.Get(key.Bytes(), nil)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// WriteKV writes a key-value pair to the LevelDB store
func (store *StateDBStorage) WriteKV(key common.Hash, value []byte) error {
	return store.db.Put(key.Bytes(), value, nil)
}

// Close closes the LevelDB store
func (store *StateDBStorage) Close() error {
	return store.db.Close()
}
