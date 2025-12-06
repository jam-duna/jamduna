package storage

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/statedb/evmtypes"
	"github.com/colorfulnotion/jam/types"
	"github.com/ethereum/go-verkle"
	"github.com/syndtr/goleveldb/leveldb"
	leveldbstorage "github.com/syndtr/goleveldb/leveldb/storage"
)

// ShowKeyVals displays KeyVals with enhanced value information
// - Shows value length
// - For values > 32 bytes: shows Blake2 hash of value
// - For values <= 32 bytes: shows actual value content
func ShowKeyVals(keyvals []types.KeyVal, label string) {
	fmt.Printf("\n=== %s - %d keys ===\n", label, len(keyvals))
	for _, kv := range keyvals {
		// Reconstruct full 32-byte key from 31-byte KeyVal.Key
		var fullKey [32]byte
		copy(fullKey[:31], kv.Key[:])

		if len(kv.Value) > 64 {
			fmt.Printf("  Key: 0x%x, %x (%d bytes)\n", fullKey, kv.Value[0:64], len(kv.Value))
		} else {
			fmt.Printf("  Key: 0x%x, %x (%d bytes)\n", fullKey, kv.Value, len(kv.Value))
		}

	}
	fmt.Printf("=== End %s ===\n\n", label)
}

// kvAlias type for davxy traces
type kvAlias struct {
	Key        string `json:"key"`
	Value      string `json:"value"`
	StructType string `json:"type,omitempty"`
	Metadata   string `json:"metadata,omitempty"`
}

func BytesToHex(b []byte) string {
	// hex.EncodeToString is highly optimized.
	// The "0x" is prepended in a single, efficient string allocation.
	return "0x" + hex.EncodeToString(b)
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

// StateDBStorage struct to hold the trie instance or in-memory map
type StateDBStorage struct {
	// Trie database instance - pure Go implementation
	trieDB        *MerkleTree // Trie database instance (JAM Gray Paper compatible)
	Root          common.Hash
	stagedInserts map[common.Hash][]byte // key -> value
	stagedDeletes map[common.Hash]bool   // key -> true
	stagedMutex   sync.Mutex             // Protects staged operations
	keys          map[common.Hash]bool   // Tracks all keys inserted

	db      *leveldb.DB
	memMap  map[string][]byte // In-memory storage when useMemory=true
	mutex   sync.RWMutex      // Protects memMap for concurrent access
	logChan chan LogMessage

	// JAM Data Availability interface
	jamda types.JAMDA

	WorkPackageContext       context.Context
	BlockContext             context.Context
	BlockAnnouncementContext context.Context
	SendTrace                bool
	nodeID                   uint16
	// Telemetry client for emitting events
	telemetryClient types.TelemetryClient

	// Witness cache (Phase 4+): Direct witness provision for EVM execution
	// These caches are populated after refine execution and persist across queries
	// Maps are protected by witnessMutex for thread-safe access
	Storageshard map[common.Address]evmtypes.ContractStorage  // address → complete storage
	Code         map[common.Address][]byte                    // address → bytecode
	witnessMutex sync.RWMutex                                 // Protects witness caches
	VerkleProofs map[common.Address]evmtypes.VerkleMultiproof // address → verkle multiproof (Phase 2, future)

	// Verkle tree storage: verkleRoot → VerkleNode
	// Stores historical Verkle tree states by their root hash
	// Allows querying state at any specific verkleRoot (e.g., at a specific block)
	verkleRoots      map[common.Hash]verkle.VerkleNode
	verkleRootsMutex sync.RWMutex // Protects verkleRoots map

	// CurrentVerkleTree is the active working tree for the current execution
	// This is used during transaction execution and witness generation
	// After execution completes, the post-state tree is stored in verkleRoots
	CurrentVerkleTree verkle.VerkleNode

	// Verkle read log: Tracks all Verkle tree reads during EVM execution
	// This is the authoritative source for BuildVerkleWitness
	verkleReadLog      []types.VerkleRead
	verkleReadLogMutex sync.Mutex // Protects verkleReadLog
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
func NewStateDBStorage(path string, jamda types.JAMDA, telemetryClient types.TelemetryClient, nodeID uint16) (*StateDBStorage, error) {
	db, err := leveldb.OpenFile(path, nil)
	if err != nil {
		// Fallback to memory storage if file fails
		memStorage := leveldbstorage.NewMemStorage()
		db, err = leveldb.Open(memStorage, nil)
		if err != nil {
			return nil, err
		}
	}

	// Create empty trie database
	trieDB := NewMerkleTree(nil)

	s := &StateDBStorage{
		db:                db,
		logChan:           make(chan LogMessage, 100),
		jamda:             jamda,
		telemetryClient:   telemetryClient,
		nodeID:            nodeID,
		trieDB:            trieDB,
		stagedInserts:     make(map[common.Hash][]byte),
		stagedDeletes:     make(map[common.Hash]bool),
		keys:              make(map[common.Hash]bool),
		verkleRoots:       make(map[common.Hash]verkle.VerkleNode),
		CurrentVerkleTree: verkle.New(),
	}

	// Get initial root from trie
	initialRoot := trieDB.GetRoot()
	s.Root = initialRoot

	return s, nil
}

// GetJAMDA returns the JAMDA interface instance
func (s *StateDBStorage) GetJAMDA() types.JAMDA {
	return s.jamda
}

// GetNodeID returns the node ID
func (s *StateDBStorage) GetNodeID() uint16 {
	return s.nodeID
}

// GetTelemetryClient returns the telemetry client for emitting events
func (store *StateDBStorage) GetTelemetryClient() types.TelemetryClient {
	return store.telemetryClient
}

// SetTelemetryClient updates the telemetry client used for emitting events.
func (store *StateDBStorage) SetTelemetryClient(client types.TelemetryClient) {
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

// CloseDB closes the LevelDB database
func (store *StateDBStorage) CloseDB() error {
	return store.db.Close()
}

// StoreVerkleTransition stores a Verkle tree state by its root hash
// This allows later retrieval of the exact tree state at a specific verkleRoot
func (store *StateDBStorage) StoreVerkleTransition(verkleRoot common.Hash, tree verkle.VerkleNode) error {
	store.verkleRootsMutex.Lock()
	defer store.verkleRootsMutex.Unlock()

	if tree == nil {
		return fmt.Errorf("cannot store nil VerkleNode")
	}

	store.verkleRoots[verkleRoot] = tree
	return nil
}

// GetVerkleTreeAtRoot retrieves a Verkle tree state by its root hash
// Returns the tree and a boolean indicating if it was found
func (store *StateDBStorage) GetVerkleTreeAtRoot(verkleRoot common.Hash) (verkle.VerkleNode, bool) {
	store.verkleRootsMutex.RLock()
	defer store.verkleRootsMutex.RUnlock()

	tree, found := store.verkleRoots[verkleRoot]
	return tree, found
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

// generateWarpSyncKey generates the storage key for a warp sync fragment
func generateWarpSyncKey(setID uint32) string {
	return fmt.Sprintf("warpsync_%d", setID)
}

// GetWarpSyncFragment retrieves a warp sync fragment for a given set ID
func (store *StateDBStorage) GetWarpSyncFragment(setID uint32) (types.WarpSyncFragment, error) {
	fragmentBytes, ok, err := store.ReadRawKV([]byte(generateWarpSyncKey(setID)))
	if err != nil {
		return types.WarpSyncFragment{}, fmt.Errorf("GetWarpSyncFragment: ReadRawKV failed: %w", err)
	}
	if !ok {
		return types.WarpSyncFragment{}, fmt.Errorf("GetWarpSyncFragment: fragment not found for setID %d", setID)
	}

	fragment, _, err := types.Decode(fragmentBytes, reflect.TypeOf(types.WarpSyncFragment{}))
	if err != nil {
		return types.WarpSyncFragment{}, fmt.Errorf("GetWarpSyncFragment: decode failed: %w", err)
	}

	return fragment.(types.WarpSyncFragment), nil
}

// StoreWarpSyncFragment stores a warp sync fragment for a given set ID
func (store *StateDBStorage) StoreWarpSyncFragment(setID uint32, fragment types.WarpSyncFragment) error {
	fragmentBytes, err := types.Encode(&fragment)
	if err != nil {
		return fmt.Errorf("StoreWarpSyncFragment: encode failed: %w", err)
	}

	if err := store.WriteRawKV([]byte(generateWarpSyncKey(setID)), fragmentBytes); err != nil {
		return fmt.Errorf("StoreWarpSyncFragment: WriteRawKV failed: %w", err)
	}

	return nil
}

// ===== EVM State Access Methods (Verkle-based) =====

// FetchBalance fetches balance from Verkle tree
func (store *StateDBStorage) FetchBalance(address common.Address, txIndex uint32) ([32]byte, error) {
	var balance [32]byte

	if store.CurrentVerkleTree == nil {
		return balance, nil // Return zero balance if no tree
	}

	// Compute Verkle key for balance
	verkleKey := evmtypes.BasicDataKey(address[:])

	// Track the read in verkleReadLog
	store.AppendVerkleRead(types.VerkleRead{
		VerkleKey: common.BytesToHash(verkleKey),
		Address:   address,
		KeyType:   0, // BasicData
		Extra:     0,
		TxIndex:   txIndex,
	})

	// Read BasicData from Verkle tree
	basicData, err := store.CurrentVerkleTree.Get(verkleKey, nil)
	if err == nil && len(basicData) >= 32 {
		// Balance is at offset 16-31 in BasicData (16 bytes)
		copy(balance[16:32], basicData[16:32])
	}

	return balance, nil
}

// FetchNonce fetches nonce from Verkle tree
func (store *StateDBStorage) FetchNonce(address common.Address, txIndex uint32) ([32]byte, error) {
	var nonce [32]byte

	if store.CurrentVerkleTree == nil {
		return nonce, nil // Return zero nonce if no tree
	}

	// Compute Verkle key for nonce
	verkleKey := evmtypes.BasicDataKey(address[:])

	// Track the read in verkleReadLog
	store.AppendVerkleRead(types.VerkleRead{
		VerkleKey: common.BytesToHash(verkleKey),
		Address:   address,
		KeyType:   0, // BasicData
		Extra:     0,
		TxIndex:   txIndex,
	})

	// Read BasicData from Verkle tree
	basicData, err := store.CurrentVerkleTree.Get(verkleKey, nil)
	if err == nil && len(basicData) >= 32 {
		// Nonce is at offset 8-15 in BasicData (8 bytes)
		copy(nonce[24:32], basicData[8:16])
	}

	return nonce, nil
}

// FetchCode fetches code from Verkle tree
func (store *StateDBStorage) FetchCode(address common.Address, txIndex uint32) ([]byte, uint32, error) {
	if store.CurrentVerkleTree == nil {
		return nil, 0, nil
	}

	// Read code size from BasicData first
	var codeSize uint32

	basicDataKey := evmtypes.BasicDataKey(address[:])

	// Track BasicData read for Verkle witness
	store.AppendVerkleRead(types.VerkleRead{
		VerkleKey: common.BytesToHash(basicDataKey),
		Address:   address,
		KeyType:   0, // BasicData
		Extra:     0,
		TxIndex:   txIndex,
	})

	basicData, err := store.CurrentVerkleTree.Get(basicDataKey, nil)
	if err == nil && len(basicData) >= 32 {
		// Extract code_size from offset 5-7 (3 bytes, big-endian uint24)
		codeSize = uint32(basicData[5])<<16 | uint32(basicData[6])<<8 | uint32(basicData[7])
	}

	if codeSize == 0 {
		// Track CodeHash read even for EOAs for witness completeness
		codeHashKey := evmtypes.CodeHashKey(address[:])
		store.AppendVerkleRead(types.VerkleRead{
			VerkleKey: common.BytesToHash(codeHashKey),
			Address:   address,
			KeyType:   1, // CodeHash
			Extra:     0,
			TxIndex:   txIndex,
		})
		return nil, 0, nil
	}

	// Calculate number of chunks needed
	numChunks := (codeSize + 30) / 31 // Round up

	// Allocate buffer for reconstructed code
	code := make([]byte, 0, codeSize)

	// Read each chunk
	for chunkID := uint64(0); chunkID < uint64(numChunks); chunkID++ {
		chunkKey := evmtypes.CodeChunkKey(address[:], chunkID)

		// Track the read in verkleReadLog
		store.AppendVerkleRead(types.VerkleRead{
			VerkleKey: common.BytesToHash(chunkKey),
			Address:   address,
			KeyType:   2, // CodeChunk
			Extra:     chunkID,
			TxIndex:   txIndex,
		})

		// Read chunk from tree
		chunkData, err := store.CurrentVerkleTree.Get(chunkKey, nil)
		if err != nil || len(chunkData) < 32 {
			return nil, 0, fmt.Errorf("chunk %d not found", chunkID)
		}

		// Extract code bytes (skip first byte which is push offset metadata)
		codeBytes := chunkData[1:32]

		// For the last chunk, only take the remaining bytes needed
		remainingBytes := int(codeSize) - len(code)
		if remainingBytes < 31 {
			codeBytes = codeBytes[:remainingBytes]
		}

		code = append(code, codeBytes...)

		if len(code) >= int(codeSize) {
			break
		}
	}

	return code, codeSize, nil
}

// FetchCodeHash fetches code hash from Verkle tree
func (store *StateDBStorage) FetchCodeHash(address common.Address, txIndex uint32) ([32]byte, error) {
	var codeHash [32]byte

	if store.CurrentVerkleTree == nil {
		copy(codeHash[:], evmtypes.GetEmptyCodeHash())
		return codeHash, nil
	}

	// Compute Verkle key for code hash
	verkleKey := evmtypes.CodeHashKey(address[:])

	// Track the read in verkleReadLog
	store.AppendVerkleRead(types.VerkleRead{
		VerkleKey: common.BytesToHash(verkleKey),
		Address:   address,
		KeyType:   1, // CodeHash
		Extra:     0,
		TxIndex:   txIndex,
	})

	// Read code hash from Verkle tree
	codeHashData, err := store.CurrentVerkleTree.Get(verkleKey, nil)
	if err == nil && len(codeHashData) >= 32 {
		copy(codeHash[:], codeHashData[:32])
	} else {
		// Code hash not found - return empty code hash
		copy(codeHash[:], evmtypes.GetEmptyCodeHash())
	}

	return codeHash, nil
}

// FetchStorage fetches storage value from Verkle tree
// Returns (value, found, error) where found=false indicates the slot was absent.
func (store *StateDBStorage) FetchStorage(address common.Address, storageKey [32]byte, txIndex uint32) ([32]byte, bool, error) {
	var value [32]byte
	found := false

	if store.CurrentVerkleTree == nil {
		return value, found, nil
	}

	// Compute Verkle key for storage slot
	verkleKey := evmtypes.StorageSlotKey(address[:], storageKey[:])

	// Track the read in verkleReadLog
	store.AppendVerkleRead(types.VerkleRead{
		VerkleKey:  common.BytesToHash(verkleKey),
		Address:    address,
		KeyType:    3, // Storage
		Extra:      0,
		StorageKey: storageKey,
		TxIndex:    txIndex,
	})

	// Read storage value from Verkle tree
	storageData, err := store.CurrentVerkleTree.Get(verkleKey, nil)
	if err == nil && len(storageData) >= 32 {
		copy(value[:], storageData[:32])
		found = true
	}

	return value, found, nil
}

// ===== Verkle Read Log Management =====

// AppendVerkleRead appends a verkle read to the log
func (store *StateDBStorage) AppendVerkleRead(read types.VerkleRead) {
	store.verkleReadLogMutex.Lock()
	defer store.verkleReadLogMutex.Unlock()
	store.verkleReadLog = append(store.verkleReadLog, read)
}

// GetVerkleReadLog returns a copy of the verkle read log
func (store *StateDBStorage) GetVerkleReadLog() []types.VerkleRead {
	store.verkleReadLogMutex.Lock()
	defer store.verkleReadLogMutex.Unlock()
	// Return a copy to prevent external modification
	logCopy := make([]types.VerkleRead, len(store.verkleReadLog))
	copy(logCopy, store.verkleReadLog)
	return logCopy
}

// ClearVerkleReadLog clears the verkle read log
func (store *StateDBStorage) ClearVerkleReadLog() {
	store.verkleReadLogMutex.Lock()
	defer store.verkleReadLogMutex.Unlock()
	store.verkleReadLog = nil
}

// BuildVerkleWitness builds a dual-proof verkle witness and stores the post-state tree
func (store *StateDBStorage) BuildVerkleWitness(contractWitnessBlob []byte) ([]byte, error) {
	if store.CurrentVerkleTree == nil {
		return nil, fmt.Errorf("no verkle tree available")
	}

	// Get the verkle read log
	verkleReadLog := store.GetVerkleReadLog()

	// Build witness using the function from witness.go
	witnessBytes, postVerkleRoot, postTree, err := BuildVerkleWitness(verkleReadLog, contractWitnessBlob, store.CurrentVerkleTree)
	if err != nil {
		return nil, fmt.Errorf("failed to build witness: %w", err)
	}

	// Store the post-state tree
	if err := store.StoreVerkleTransition(postVerkleRoot, postTree); err != nil {
		return nil, fmt.Errorf("failed to store verkle transition: %w", err)
	}

	return witnessBytes, nil
}

// ComputeBlockAccessListHash builds BAL from verkle witness and returns hash + statistics
// Used by both builder (to compute hash for payload) and guarantor (to verify builder's hash)
func (store *StateDBStorage) ComputeBlockAccessListHash(verkleWitness []byte) (common.Hash, uint32, uint32, error) {
	// Split witness into pre-state and post-state sections
	preState, postState, err := SplitWitnessSections(verkleWitness)
	if err != nil {
		return common.Hash{}, 0, 0, fmt.Errorf("failed to split witness: %w", err)
	}

	// Build Block Access List from witness
	bal, err := BuildBlockAccessList(preState, postState)
	if err != nil {
		return common.Hash{}, 0, 0, fmt.Errorf("failed to build BAL: %w", err)
	}

	// Count statistics
	accountCount := uint32(len(bal.Accounts))
	totalChanges := uint32(0)
	for _, account := range bal.Accounts {
		totalChanges += uint32(len(account.StorageReads))
		for _, slotChange := range account.StorageChanges {
			totalChanges += uint32(len(slotChange.Writes))
		}
		totalChanges += uint32(len(account.BalanceChanges))
		totalChanges += uint32(len(account.NonceChanges))
		totalChanges += uint32(len(account.CodeChanges))
	}

	// Compute Blake2b hash of RLP-encoded BAL
	hash := bal.Hash()

	return hash, accountCount, totalChanges, nil
}
