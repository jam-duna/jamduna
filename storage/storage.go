package storage

import (
	"fmt"

	"github.com/colorfulnotion/jam/common"
	"github.com/syndtr/goleveldb/leveldb"
)

type LogMessage struct {
	Payload  interface{}
	Timeslot uint32
	Self     bool
}

// StateDBStorage struct to hold the LevelDB instance
type StateDBStorage struct {
	db      *leveldb.DB
	logChan chan LogMessage
}

// NewStateDBStorage initializes a new LevelDB store
func NewStateDBStorage(path string) (*StateDBStorage, error) {
	db, err := leveldb.OpenFile(path, nil)
	if err != nil {
		return nil, err
	}
	s := StateDBStorage{
		db:      db,
		logChan: make(chan LogMessage, 100),
	}
	return &s, nil
}

// ReadKV reads a value for a given key from the LevelDB store
func (store *StateDBStorage) ReadKV(key common.Hash) ([]byte, error) {
	data, err := store.db.Get(key.Bytes(), nil)
	if err != nil {
		return nil, fmt.Errorf("ReadKV %v Err: %v", key, err)
	}
	return data, nil
}

// WriteKV writes a key-value pair to the LevelDB store
func (store *StateDBStorage) WriteKV(key common.Hash, value []byte) error {
	return store.db.Put(key.Bytes(), value, nil)
}

// Close closes the LevelDB store
func (store *StateDBStorage) DeleteK(key common.Hash) error {
	return store.db.Delete(key.Bytes(), nil)
}

// Close closes the LevelDB store
func (store *StateDBStorage) Close() error {
	return store.db.Close()
}

func (store *StateDBStorage) ReadRawKV(key []byte) ([]byte, error) {
	data, err := store.db.Get(key, nil)
	if err != nil {
		return nil, fmt.Errorf("ReadRawKV %v Err: %v", key, err)
	}
	return data, nil
}

func (store *StateDBStorage) WriteRawKV(key []byte, value []byte) error {
	return store.db.Put(key, value, nil)
}

func (store *StateDBStorage) DeleteRawK(key []byte) error {
	return store.db.Delete(key, nil)
}

func (store *StateDBStorage) WriteLog(obj interface{}, timeslot uint32) {
	msg := LogMessage{
		Payload:  obj,
		Timeslot: timeslot,
	}
	store.logChan <- msg
}

func (store *StateDBStorage) GetChan() chan LogMessage {
	return store.logChan
}
