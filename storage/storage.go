package storage

import (
	"bytes"
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

const (
	ImportDASegmentShardPrefix   = "is_"
	ImportDAJustificationPrefix  = "ij_"
	AuditDABundlePrefix          = "ab_"
	AuditDASegmentShardPrefix    = "as_"
	AuditDAJustificationPrefix   = "aj_"
)

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

// Helper function to generate a key using a prefix, erasureRoot, and shardIndex
func generateKey(prefix string, erasureRoot common.Hash, shardIndex uint16) []byte {
	var buffer bytes.Buffer
	buffer.WriteString(prefix)
	buffer.Write(erasureRoot.Bytes())
	buffer.WriteByte(byte(shardIndex >> 8))  // high byte
	buffer.WriteByte(byte(shardIndex & 0xff)) // low byte
	return buffer.Bytes()
}

func (store *StateDBStorage) StoreImportDA(erasureRoot common.Hash, shardIndex uint16, segmentShardsI []byte, justificationsI [][]byte) (err error) {
	if len(segmentShardsI) > 0 {
		key := generateKey(ImportDASegmentShardPrefix, erasureRoot, shardIndex)
		if err := store.db.Put(key, segmentShardsI, nil); err != nil {
			return err
		}
	}
	if len(justificationsI) > 0 {
		key := generateKey(ImportDAJustificationPrefix, erasureRoot, shardIndex)
		// Serialize the justifications to a single byte slice if needed
		justificationsBytes := bytes.Join(justificationsI, []byte{0})
		if err := store.db.Put(key, justificationsBytes, nil); err != nil {
			return err
		}
	}
	return nil
}

func (store *StateDBStorage) StoreAuditDA(erasureRoot common.Hash, shardIndex uint16, bundleShard, segmentShards, justification []byte) (err error) {
	if len(bundleShard) > 0 {
		key := generateKey(AuditDABundlePrefix, erasureRoot, shardIndex)
		if err := store.db.Put(key, bundleShard, nil); err != nil {
			return err
		}
	}
	if len(segmentShards) > 0 {
		key := generateKey(AuditDASegmentShardPrefix, erasureRoot, shardIndex)
		if err := store.db.Put(key, segmentShards, nil); err != nil {
			return err
		}
	}
	if len(justification) > 0 {
		key := generateKey(AuditDAJustificationPrefix, erasureRoot, shardIndex)
		if err := store.db.Put(key, justification, nil); err != nil {
			return err
		}
	}
	return nil
}

func (store *StateDBStorage) GetSegmentShard(erasureRoot common.Hash, shardIndex uint16, segmentIndex []uint16) (segmentShards []byte, justifications [][]byte, ok bool, err error) {
	key := generateKey(AuditDASegmentShardPrefix, erasureRoot, shardIndex)
	segmentShards, err = store.db.Get(key, nil)
	if err != nil {
		if err == leveldb.ErrNotFound {
			return nil, nil, false, nil
		}
		return nil, nil, false, err
	}

	key = generateKey(AuditDAJustificationPrefix, erasureRoot, shardIndex)
	justificationBytes, err := store.db.Get(key, nil)
	if err != nil && err != leveldb.ErrNotFound {
		return nil, nil, false, err
	}
	if justificationBytes != nil {
		justifications = bytes.Split(justificationBytes, []byte{0})
	}

	return segmentShards, justifications, true, nil
}

func (store *StateDBStorage) GetShard(erasureRoot common.Hash, shardIndex uint16) (bundleShard, segmentShards, justification []byte, ok bool, err error) {
	key := generateKey(AuditDABundlePrefix, erasureRoot, shardIndex)
	bundleShard, err = store.db.Get(key, nil)
	if err != nil {
		if err == leveldb.ErrNotFound {
			return nil, nil, nil, false, nil
		}
		return nil, nil, nil, false, err
	}

	key = generateKey(AuditDASegmentShardPrefix, erasureRoot, shardIndex)
	segmentShards, err = store.db.Get(key, nil)
	if err != nil && err != leveldb.ErrNotFound {
		return nil, nil, nil, false, err
	}

	key = generateKey(AuditDAJustificationPrefix, erasureRoot, shardIndex)
	justification, err = store.db.Get(key, nil)
	if err != nil && err != leveldb.ErrNotFound {
		return nil, nil, nil, false, err
	}

	return bundleShard, segmentShards, justification, true, nil
}
