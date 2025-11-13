package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"

	"github.com/colorfulnotion/jam/common"
	telemetry "github.com/colorfulnotion/jam/telemetry"
	"github.com/colorfulnotion/jam/types"
	"github.com/syndtr/goleveldb/leveldb"
	leveldbstorage "github.com/syndtr/goleveldb/leveldb/storage"
)

type JAMStorage interface {
	// Core KV Operations - Low-level key-value access
	ReadRawKV(key []byte) (value []byte, found bool, err error)
	ReadRawKVWithPrefix(prefix []byte) ([][2][]byte, error)
	WriteRawKV(key []byte, val []byte) error
	ReadKV(key common.Hash) ([]byte, error)

	// Block Storage Operations
	// These methods handle block persistence and retrieval with multiple indices
	StoreBlock(blk *types.Block, id uint16, slotTimestamp uint64) error
	StoreFinalizedBlock(blk *types.Block) error
	GetFinalizedBlock() (*types.Block, error)
	GetFinalizedBlockInternal() (*types.Block, bool, error)
	GetBlockByHeader(headerHash common.Hash) (*types.SBlock, error)
	GetBlockBySlot(slot uint32) (*types.SBlock, error)
	GetChildBlocks(parentHeaderHash common.Hash) ([][2][]byte, error)

	// Data Availability - Guarantor Operations
	// Guarantors create and distribute erasure-coded shards
	StoreBundleSpecSegments(
		erasureRoot common.Hash,
		exportedSegmentRoot common.Hash,
		bChunks []types.DistributeECChunk,
		sChunks []types.DistributeECChunk,
		bClubs []common.Hash,
		sClubs []common.Hash,
		bundle []byte,
		encodedSegments []byte,
	) error
	GetGuarantorMetadata(erasureRoot common.Hash) (
		bClubs []common.Hash,
		sClubs []common.Hash,
		bECChunks []types.DistributeECChunk,
		sECChunksArray []types.DistributeECChunk,
		err error,
	)
	GetFullShard(erasureRoot common.Hash, shardIndex uint16) (
		bundleShard []byte,
		segmentShards []byte,
		justification []byte,
		ok bool,
		err error,
	)

	// Data Availability - Assurer Operations
	// Assurers verify and store shards for availability
	StoreFullShardJustification(
		erasureRoot common.Hash,
		shardIndex uint16,
		bClub common.Hash,
		sClub common.Hash,
		encodedPath []byte,
	) error
	GetFullShardJustification(erasureRoot common.Hash, shardIndex uint16) (
		bClubH common.Hash,
		sClubH common.Hash,
		encodedPath []byte,
		err error,
	)
	StoreAuditDA(erasureRoot common.Hash, shardIndex uint16, bundleShard []byte) error
	StoreImportDA(erasureRoot common.Hash, shardIndex uint16, concatenatedShards []byte) error
	GetBundleShard(erasureRoot common.Hash, shardIndex uint16) (
		bundleShard []byte,
		sClub common.Hash,
		justification []byte,
		ok bool,
		err error,
	)
	GetSegmentShard(erasureRoot common.Hash, shardIndex uint16) (
		concatenatedShards []byte,
		ok bool,
		err error,
	)

	// Bundle and Segment Retrieval
	GetBundleByErasureRoot(erasureRoot common.Hash) (types.WorkPackageBundle, bool)
	GetSegmentsBySegmentRoot(segmentRoot common.Hash) ([][]byte, bool)
	FetchJAMDASegments(workPackageHash common.Hash, indexStart uint16, indexEnd uint16, payloadLength uint32) (payload []byte, err error)

	// Work Report Operations
	StoreWorkReport(wr *types.WorkReport) error
	WorkReportSearch(requestedHash common.Hash) (*types.WorkReport, bool)

	// Node Identity and Telemetry
	SetTelemetryClient(client *telemetry.TelemetryClient)
	GetTelemetryClient() *telemetry.TelemetryClient
	GetJAMDA() JAMDA
	GetNodeID() uint16

	// Lifecycle
	Close() error
}

const (
	CheckStateTransition = false
)

type LogMessage struct {
	Payload     interface{}
	Description string
	Timeslot    uint32
	Self        bool
}

// StateDBStorage struct to hold the LevelDB instance or in-memory map
type StateDBStorage struct {
	db      *leveldb.DB
	memMap  map[string][]byte // In-memory storage when useMemory=true
	mutex   sync.RWMutex      // Protects memMap for concurrent access
	logChan chan LogMessage

	// JAM Data Availability interface
	jamda JAMDA

	WorkPackageContext       context.Context
	BlockContext             context.Context
	BlockAnnouncementContext context.Context
	SendTrace                bool
	nodeID                   uint16

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
func NewStateDBStorage(path string, jamda JAMDA, telemetryClient *telemetry.TelemetryClient, nodeID uint16) (*StateDBStorage, error) {
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
		jamda:           jamda,
		telemetryClient: telemetryClient,
		nodeID:          nodeID,
	}
	return &s, nil
}

// GetJAMDA returns the JAMDA interface instance
func (s *StateDBStorage) GetJAMDA() JAMDA {
	return s.jamda
}

// GetNodeID returns the node ID
func (s *StateDBStorage) GetNodeID() uint16 {
	return s.nodeID
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

	data, err := store.db.Get(key.Bytes(), nil)
	if err != nil {
		return nil, fmt.Errorf("ReadKV %v Err: %v", key, err)
	}
	return data, nil
}

// WriteKV writes a key-value pair to the storage
func (store *StateDBStorage) WriteKV(key common.Hash, value []byte) error {

	return store.db.Put(key.Bytes(), value, nil)
}

// DeleteK deletes a key from the storage
func (store *StateDBStorage) DeleteK(key common.Hash) error {

	return store.db.Delete(key.Bytes(), nil)
}

// Close closes the storage
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

// SpecIndex holds a WorkReport and associated segment indices
type SpecIndex struct {
	WorkReport types.WorkReport `json:"spec"`
	Indices    []uint16         `json:"indices"`
}

// String returns JSON representation of SpecIndex
func (si *SpecIndex) String() string {
	jsonBytes, err := json.Marshal(si)
	if err != nil {
		return fmt.Sprintf("%v", err)
	}
	return string(jsonBytes)
}

// AddIndex adds an index to the SpecIndex if not already present
func (si *SpecIndex) AddIndex(idx uint16) bool {
	if !common.Uint16Contains(si.Indices, idx) {
		si.Indices = append(si.Indices, idx)
		return true
	}
	return false
}

// generateSpecKey generates the storage key for a request hash
// requestHash (packageHash(wp) or SegmentRoot(e)) -> ErasureRoot(u)
func generateSpecKey(requestHash common.Hash) string {
	return fmt.Sprintf("rtou_%v", requestHash)
}

// WorkReportSearch looks up the erasureRoot, exportedSegmentRoot, workpackageHash
// for either kind of hash: segment root OR workPackageHash
func (store *StateDBStorage) WorkReportSearch(h common.Hash) (*types.WorkReport, bool) {
	wrBytes, ok, err := store.ReadRawKV([]byte(generateSpecKey(h)))
	if err != nil || !ok {
		return nil, false
	}

	wr, _, err := types.Decode(wrBytes, reflect.TypeOf(types.WorkReport{}))
	if err != nil {
		return nil, false
	}

	workReport := wr.(types.WorkReport)
	return &workReport, true
}

// StoreWorkReport stores a WorkReport with mappings for workPackageHash, segmentRoot, and erasureRoot
func (store *StateDBStorage) StoreWorkReport(wr *types.WorkReport) error {
	spec := wr.AvailabilitySpec
	erasureRoot := spec.ErasureRoot
	segmentRoot := spec.ExportedSegmentRoot
	workpackageHash := spec.WorkPackageHash

	wrBytes, err := types.Encode(wr)
	if err != nil {
		return err
	}

	// Write 3 mappings:
	// (a) workpackageHash => spec
	// (b) segmentRoot => spec
	// (c) erasureRoot => spec
	if err := store.WriteRawKV([]byte(generateSpecKey(workpackageHash)), wrBytes); err != nil {
		return err
	}
	if err := store.WriteRawKV([]byte(generateSpecKey(segmentRoot)), wrBytes); err != nil {
		return err
	}
	if err := store.WriteRawKV([]byte(generateSpecKey(erasureRoot)), wrBytes); err != nil {
		return err
	}

	return nil
}
