package storage

import (
	"bytes"
	"context"
	"fmt"

	"github.com/colorfulnotion/jam/common"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/storage"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

const (
	CheckStateTransition = false
	useMemory            = true
)

type LogMessage struct {
	Payload     interface{}
	Description string
	Timeslot    uint32
	Self        bool
}

// StateDBStorage struct to hold the LevelDB instance
type StateDBStorage struct {
	db       *leveldb.DB
	logChan  chan LogMessage
	memBased bool // Track whether this instance is memory-based

	// OpenTelemetry stuff
	Tp                       *sdktrace.TracerProvider
	WorkPackageContext       context.Context
	BlockContext             context.Context
	BlockAnnouncementContext context.Context
	SendTrace                bool
	NodeID                   uint16
}

const (
	ImportDASegmentShardPrefix  = "is_"
	ImportDAJustificationPrefix = "ij_"
	AuditDABundlePrefix         = "ab_"
	AuditDASegmentShardPrefix   = "as_"
	AuditDAJustificationPrefix  = "aj_"
)

// NewStateDBStorage initializes a new LevelDB store
// Uses memory-based storage if useMemory is true, otherwise uses file-based storage
func NewStateDBStorage(path string) (*StateDBStorage, error) {
	if useMemory {
		return newMemoryStateDBStorage()
	}
	return newFileStateDBStorage(path)
}

// newMemoryStateDBStorage creates a memory-only storage (internal helper)
func newMemoryStateDBStorage() (*StateDBStorage, error) {
	memStorage := storage.NewMemStorage()
	db, err := leveldb.Open(memStorage, nil)
	if err != nil {
		return nil, err
	}

	s := StateDBStorage{
		db:       db,
		logChan:  make(chan LogMessage, 100),
		memBased: true,
	}
	return &s, nil
}

// newFileStateDBStorage creates a file-based storage (internal helper)
func newFileStateDBStorage(path string) (*StateDBStorage, error) {
	db, err := leveldb.OpenFile(path, nil)
	if err != nil {
		return nil, err
	}

	s := StateDBStorage{
		db:       db,
		logChan:  make(chan LogMessage, 100),
		memBased: false,
	}
	return &s, nil
}

// IsMemoryBased returns whether this storage instance is using memory-based storage
func (store *StateDBStorage) IsMemoryBased() bool {
	return store.memBased
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

// DeleteK deletes a key from the LevelDB store
func (store *StateDBStorage) DeleteK(key common.Hash) error {
	return store.db.Delete(key.Bytes(), nil)
}

// Close closes the LevelDB store
func (store *StateDBStorage) Close() error {
	return store.db.Close()
}

func (store *StateDBStorage) ReadRawKV(key []byte) ([]byte, bool, error) {
	data, err := store.db.Get(key, nil)
	if err == leveldb.ErrNotFound {
		return nil, false, nil
	} else if err != nil {
		return nil, false, fmt.Errorf("ReadRawKV %x Err: %v", key, err)
	}
	return data, true, nil
}

func (store *StateDBStorage) ReadRawKVWithPrefix(prefix []byte) ([][2][]byte, error) {
	iter := store.db.NewIterator(nil, nil)
	defer iter.Release()

	var keyvals [][2][]byte

	for iter.Seek(prefix); iter.Valid(); iter.Next() {
		key := iter.Key()
		if !bytes.HasPrefix(key, prefix) {
			break
		}
		keyval := [2][]byte{append([]byte{}, key...), append([]byte{}, iter.Value()...)}
		keyvals = append(keyvals, keyval)
	}
	if err := iter.Error(); err != nil {
		return nil, fmt.Errorf("ReadRawKVWithPrefix %v Err: %v", prefix, err)
	}
	return keyvals, nil
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

func TestTrace(host string) bool {
	return true
}

func (store *StateDBStorage) InitTracer(serviceName string) error {
	return nil
}

func (store *StateDBStorage) UpdateWorkPackageContext(ctx context.Context) {
	store.WorkPackageContext = ctx
}

func (store *StateDBStorage) UpdateBlockContext(ctx context.Context) {
	store.BlockContext = ctx
}

func (store *StateDBStorage) UpdateBlockAnnouncementContext(ctx context.Context) {
	store.BlockAnnouncementContext = ctx
}

func (store *StateDBStorage) CleanWorkPackageContext() {
	store.WorkPackageContext = context.Background()
}

func (store *StateDBStorage) CleanBlockContext() {
	store.BlockContext = context.Background()
}

func (store *StateDBStorage) CleanBlockAnnouncementContext() {
	store.BlockAnnouncementContext = context.Background()
}
