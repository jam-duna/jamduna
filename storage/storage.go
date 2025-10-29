package storage

import (
	"bytes"
	"context"
	"fmt"
	"sync"

	"github.com/colorfulnotion/jam/common"
	telemetry "github.com/colorfulnotion/jam/telemetry"
	"github.com/syndtr/goleveldb/leveldb"
	leveldbstorage "github.com/syndtr/goleveldb/leveldb/storage"
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

// StateDBStorage struct to hold the LevelDB instance or in-memory map
type StateDBStorage struct {
	db       *leveldb.DB
	memMap   map[string][]byte // In-memory storage when useMemory=true
	mutex    sync.RWMutex      // Protects memMap for concurrent access
	logChan  chan LogMessage
	memBased bool // Track whether this instance is memory-based

	// JAM Data Availability interface
	jamda JAMDA

	WorkPackageContext       context.Context
	BlockContext             context.Context
	BlockAnnouncementContext context.Context
	SendTrace                bool
	NodeID                   uint16

	// Telemetry client for emitting events
	telemetryClient *telemetry.TelemetryClient
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
func NewStateDBStorage(path string, jamda JAMDA, telemetryClient *telemetry.TelemetryClient) (*StateDBStorage, error) {
	if useMemory {
		return newMemoryStateDBStorage(jamda, telemetryClient)
	}
	return newFileStateDBStorage(path, jamda, telemetryClient)
}

// newMemoryStateDBStorage creates a memory-only storage using Go map (internal helper)
func newMemoryStateDBStorage(jamda JAMDA, telemetryClient *telemetry.TelemetryClient) (*StateDBStorage, error) {
	s := StateDBStorage{
		db:              nil, // No LevelDB when using pure memory
		memMap:          make(map[string][]byte),
		logChan:         make(chan LogMessage, 100),
		memBased:        true,
		jamda:           jamda,
		telemetryClient: telemetryClient,
	}
	return &s, nil
}

// newFileStateDBStorage creates a file-based storage (internal helper)
func newFileStateDBStorage(path string, jamda JAMDA, telemetryClient *telemetry.TelemetryClient) (*StateDBStorage, error) {
	db, err := leveldb.OpenFile(path, nil)
	if err != nil {
		// Fallback to memory storage if file fails
		memStorage := leveldbstorage.NewMemStorage()
		db, err = leveldb.Open(memStorage, nil)
		if err != nil {
			return nil, err
		}
	}

	s := StateDBStorage{
		db:              db,
		logChan:         make(chan LogMessage, 100),
		memBased:        false,
		jamda:           jamda,
		telemetryClient: telemetryClient,
	}
	return &s, nil
}

// IsMemoryBased returns whether this storage instance is using memory-based storage
func (store *StateDBStorage) IsMemoryBased() bool {
	return store.memBased
}

// GetTelemetryClient returns the telemetry client for emitting events
func (store *StateDBStorage) GetTelemetryClient() *telemetry.TelemetryClient {
	return store.telemetryClient
}

// SetTelemetryClient updates the telemetry client used for emitting events.
func (store *StateDBStorage) SetTelemetryClient(client *telemetry.TelemetryClient) {
	store.telemetryClient = client
}

// ReadKV reads a value for a given key from the storage
func (store *StateDBStorage) ReadKV(key common.Hash) ([]byte, error) {
	if store.memBased {
		store.mutex.RLock()
		defer store.mutex.RUnlock()
		data, exists := store.memMap[string(key.Bytes())]
		if !exists {
			return nil, fmt.Errorf("ReadKV %v: key not found", key)
		}
		// Return a copy to prevent external modifications
		result := make([]byte, len(data))
		copy(result, data)
		return result, nil
	}

	data, err := store.db.Get(key.Bytes(), nil)
	if err != nil {
		return nil, fmt.Errorf("ReadKV %v Err: %v", key, err)
	}
	return data, nil
}

// WriteKV writes a key-value pair to the storage
func (store *StateDBStorage) WriteKV(key common.Hash, value []byte) error {
	if store.memBased {
		store.mutex.Lock()
		defer store.mutex.Unlock()
		// Store a copy to prevent external modifications
		valueCopy := make([]byte, len(value))
		copy(valueCopy, value)
		store.memMap[string(key.Bytes())] = valueCopy
		return nil
	}

	return store.db.Put(key.Bytes(), value, nil)
}

// DeleteK deletes a key from the storage
func (store *StateDBStorage) DeleteK(key common.Hash) error {
	if store.memBased {
		store.mutex.Lock()
		defer store.mutex.Unlock()
		delete(store.memMap, string(key.Bytes()))
		return nil
	}

	return store.db.Delete(key.Bytes(), nil)
}

// Close closes the storage
func (store *StateDBStorage) Close() error {
	if store.memBased {
		store.mutex.Lock()
		defer store.mutex.Unlock()
		// Clear the map
		store.memMap = nil
		return nil
	}

	return store.db.Close()
}

func (store *StateDBStorage) ReadRawKV(key []byte) ([]byte, bool, error) {
	if store.memBased {
		store.mutex.RLock()
		defer store.mutex.RUnlock()
		data, exists := store.memMap[string(key)]
		if !exists {
			return nil, false, nil
		}
		// Return a copy to prevent external modifications
		result := make([]byte, len(data))
		copy(result, data)
		return result, true, nil
	}

	data, err := store.db.Get(key, nil)
	if err == leveldb.ErrNotFound {
		return nil, false, nil
	} else if err != nil {
		return nil, false, fmt.Errorf("ReadRawKV %x Err: %v", key, err)
	}
	return data, true, nil
}

func (store *StateDBStorage) ReadRawKVWithPrefix(prefix []byte) ([][2][]byte, error) {
	if store.memBased {
		store.mutex.RLock()
		defer store.mutex.RUnlock()

		var keyvals [][2][]byte

		for k, v := range store.memMap {
			if bytes.HasPrefix([]byte(k), prefix) {
				// Make copies to prevent external modifications
				keyCopy := make([]byte, len(k))
				copy(keyCopy, []byte(k))
				valueCopy := make([]byte, len(v))
				copy(valueCopy, v)
				keyval := [2][]byte{keyCopy, valueCopy}
				keyvals = append(keyvals, keyval)
			}
		}
		return keyvals, nil
	}

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
	if store.memBased {
		store.mutex.Lock()
		defer store.mutex.Unlock()
		// Store copies to prevent external modifications
		valueCopy := make([]byte, len(value))
		copy(valueCopy, value)
		store.memMap[string(key)] = valueCopy
		return nil
	}

	return store.db.Put(key, value, nil)
}

func (store *StateDBStorage) DeleteRawK(key []byte) error {
	if store.memBased {
		store.mutex.Lock()
		defer store.mutex.Unlock()
		delete(store.memMap, string(key))
		return nil
	}

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

// FetchJAMDASegments fetches DA payload using WorkPackageHash and segment indices
// This method retrieves segments from DA storage and combines them into payload
func (store *StateDBStorage) FetchJAMDASegments(workPackageHash common.Hash, indexStart uint16, indexEnd uint16, payloadLength uint32) (payload []byte, err error) {
	return store.jamda.FetchJAMDASegments(workPackageHash, indexStart, indexEnd, payloadLength)
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
