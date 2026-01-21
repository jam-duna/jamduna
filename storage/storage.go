package storage

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"reflect"
	"sync"

	evmtypes "github.com/colorfulnotion/jam/builder/evm/types"
	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
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

const defaultUBTProfile = JAMProfile

func ubtTreeKeyToHash(key TreeKey) common.Hash {
	keyBytes := key.ToBytes()
	return common.BytesToHash(keyBytes[:])
}

type LogMessage struct {
	Payload     interface{}
	Description string
	Timeslot    uint32
	Self        bool
}

// sharedUBTState holds UBT tree state that is shared between the original storage
// and its clones. This ensures proper mutex synchronization when multiple instances
// access the same treeStore map.
type sharedUBTState struct {
	// treeStore: Unified store for all UBT trees by their root hash.
	treeStore map[common.Hash]*UnifiedBinaryTree
	mutex     sync.RWMutex

	// canonicalRoot: The state root of the canonical (committed) state.
	canonicalRoot common.Hash

	// DEPRECATED: ubtRoots - kept for interface compatibility
	ubtRoots      map[common.Hash]*UnifiedBinaryTree
	ubtRootsMutex sync.RWMutex
}

// sharedServiceState holds service-scoped state that is shared between the original
// storage and its clones. This ensures proper mutex synchronization when multiple
// instances access the same service maps.
type sharedServiceState struct {
	// Service-scoped state roots
	// Key: "vr_{serviceID}_{blockNumber}" → state root
	serviceUBTRoots map[string]common.Hash

	// Service-scoped JAM state roots (links service block to JAM state)
	// Key: "jsr_{serviceID}_{blockNumber}" → ServiceBlockIndex
	serviceJAMStateRoots map[string]*types.ServiceBlockIndex

	// Service block hash index
	// Key: "bhash_{serviceID}_{blockHash}" → blockNumber
	serviceBlockHashIndex map[string]uint64

	// Latest rollup block number per service
	latestRollupBlock map[uint32]uint64

	// Mutex for all service-scoped maps
	mutex sync.RWMutex
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
	Storageshard map[common.Address]evmtypes.ContractStorage // address → complete storage
	Code         map[common.Address][]byte                   // address → bytecode
	witnessMutex sync.RWMutex                                // Protects witness caches
	UBTProofs    map[common.Address]evmtypes.UBTMultiproof   // address → UBT multiproof

	// ===== Root-First State Model =====
	//
	// The storage system uses a root-first state model where all trees are accessed
	// by their state root hash, not by block number. This enables:
	// - Standalone pre/post per bundle (each bundle carries its own preRoot/postRoot)
	// - Safe resubmission without affecting other bundles
	// - Clean invalidation by root (no orphaned indexes)
	//
	// Key principle: The canonical state is "the tree at canonicalRoot", not a mutable global.

	// ubtState: Shared UBT state (treeStore + canonicalRoot) with synchronized mutex.
	// This pointer is shared between the original and all clones, ensuring
	// thread-safe access to the tree store from concurrent operations.
	ubtState *sharedUBTState

	// activeRoot: The state root being used for the current execution context.
	// Set before execution begins, cleared after. Used by GetActiveTree.
	// NOTE: This is PER-INSTANCE, not shared with clones.
	activeRoot common.Hash

	// UBT read log: Tracks all UBT-backed reads during EVM execution
	// This is the authoritative source for BuildUBTWitness
	ubtReadLog        []types.UBTRead
	ubtReadLogMutex   sync.Mutex // Protects ubtReadLog
	ubtReadLogEnabled bool       // Phase 1 support: when false, AppendUBTRead is a no-op

	// Pinned state root for Phase 1 verification
	// When set, reads use the tree at this root instead of canonical
	pinnedStateRoot *common.Hash
	pinnedTree      *UnifiedBinaryTree

	// Checkpoint manager for proof serving (UBT checkpoint tree)
	// Manages in-memory checkpoint trees with pinned (genesis, snapshots) + LRU cache
	CheckpointManager *CheckpointTreeManager

	// Multi-rollup support: Service-scoped state tracking
	// Each service maintains its own EVM block history and state
	// This pointer is shared between the original and all clones.
	serviceState *sharedServiceState

	// Pending writes and block-number snapshots are removed in root-first mode.

	// isClone indicates this is a CloneTrieView instance (JAM trie ops only).
	// EVM/UBT operations will panic on clones to prevent silent failures.
	isClone bool
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
	// Ensure the directory exists before opening LevelDB
	if err := os.MkdirAll(path, 0755); err != nil {
		return nil, fmt.Errorf("failed to create leveldb directory %s: %w", path, err)
	}

	log.Info(log.Node, "NewStateDBStorage: opening LevelDB", "path", path, "nodeID", nodeID)

	db, err := leveldb.OpenFile(path, nil)
	if err != nil {
		log.Warn(log.Node, "NewStateDBStorage: LevelDB open failed, using memory storage", "path", path, "err", err)
		// Fallback to memory storage if file fails
		memStorage := leveldbstorage.NewMemStorage()
		db, err = leveldb.Open(memStorage, nil)
		if err != nil {
			return nil, err
		}
	} else {
		log.Info(log.Node, "NewStateDBStorage: LevelDB opened successfully", "path", path)
	}

	// Create empty trie database
	trieDB := NewMerkleTree(nil)

	// Create genesis tree for initial state
	genesisTree := NewUnifiedBinaryTree(Config{Profile: defaultUBTProfile})
	genesisRootBytes := genesisTree.RootHash()
	genesisRoot := common.BytesToHash(genesisRootBytes[:])

	// Create shared UBT state (will be shared with clones)
	ubtState := &sharedUBTState{
		treeStore:     make(map[common.Hash]*UnifiedBinaryTree),
		canonicalRoot: genesisRoot,
		ubtRoots:      make(map[common.Hash]*UnifiedBinaryTree), // DEPRECATED but kept for compatibility
	}

	// Store genesis tree in treeStore
	ubtState.treeStore[genesisRoot] = genesisTree

	// Create shared service state (will be shared with clones)
	serviceState := &sharedServiceState{
		serviceUBTRoots:       make(map[string]common.Hash),
		serviceJAMStateRoots:  make(map[string]*types.ServiceBlockIndex),
		serviceBlockHashIndex: make(map[string]uint64),
		latestRollupBlock:     make(map[uint32]uint64),
	}

	s := &StateDBStorage{
		db:                    db,
		logChan:               make(chan LogMessage, 100),
		jamda:                 jamda,
		telemetryClient:       telemetryClient,
		nodeID:                nodeID,
		trieDB:                trieDB,
		stagedInserts:         make(map[common.Hash][]byte),
		stagedDeletes:         make(map[common.Hash]bool),
		keys:                  make(map[common.Hash]bool),
		// Root-first state model: shared UBT state
		ubtState:              ubtState,
		activeRoot:            common.Hash{}, // No active root initially (per-instance)
		ubtReadLogEnabled:     true,          // Default: logging enabled (normal operation)
		// Service-scoped state: shared with clones
		serviceState:          serviceState,
	}

	// Initialize checkpoint manager (UBT checkpoint tree)
	// maxCheckpoints: 100 (keeps ~100 fine checkpoints in LRU)
	// coarsePeriod: 7200 blocks (~12h at 2s/block)
	// finePeriod: 600 blocks (~1h at 2s/block)
	checkpointManager, err := NewCheckpointTreeManager(
		100,                 // maxCheckpoints
		7200,                // coarsePeriod
		600,                 // finePeriod
		s.LoadBlockByHeight, // blockLoader callback
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create checkpoint manager: %w", err)
	}
	s.CheckpointManager = checkpointManager

	// Get initial root from trie
	initialRoot := trieDB.GetRoot()
	s.Root = initialRoot

	return s, nil
}

// ===== Root-First State Access Methods =====
//
// These methods provide root-based state access, replacing block-number-based
// or implicit CurrentUBT access. All state should be accessed by root hash.

// panicIfClone panics if this is a CloneTrieView instance.
// Used to prevent clones from modifying shared canonical state (canonicalRoot).
// Clones CAN read from treeStore and set their own activeRoot.
func (s *StateDBStorage) panicIfClone(operation string) {
	if s.isClone {
		panic(fmt.Sprintf("operation %q modifies canonical state - not allowed on CloneTrieView", operation))
	}
}

// GetCanonicalRoot returns the state root of the canonical (committed) state.
func (s *StateDBStorage) GetCanonicalRoot() common.Hash {
	s.ubtState.mutex.RLock()
	defer s.ubtState.mutex.RUnlock()
	return s.ubtState.canonicalRoot
}

// SetCanonicalRoot updates the canonical state root.
// This should only be called by CommitAsCanonical after accumulation.
func (s *StateDBStorage) SetCanonicalRoot(root common.Hash) error {
	s.panicIfClone("SetCanonicalRoot")
	s.ubtState.mutex.Lock()
	defer s.ubtState.mutex.Unlock()

	if _, exists := s.ubtState.treeStore[root]; !exists {
		return fmt.Errorf("cannot set canonical root to %s: tree not found in treeStore", root.Hex())
	}
	s.ubtState.canonicalRoot = root

	return nil
}

// GetTreeByRoot returns the UBT tree with the given state root.
// Returns (tree, true) if found, (nil, false) if not.
func (s *StateDBStorage) GetTreeByRoot(root common.Hash) (interface{}, bool) {
	s.ubtState.mutex.RLock()
	defer s.ubtState.mutex.RUnlock()
	tree, exists := s.ubtState.treeStore[root]
	if !exists {
		return nil, false
	}
	return tree, true
}

// GetTreeByRootTyped returns the typed UBT tree (for internal use).
func (s *StateDBStorage) GetTreeByRootTyped(root common.Hash) (*UnifiedBinaryTree, bool) {
	s.ubtState.mutex.RLock()
	defer s.ubtState.mutex.RUnlock()
	tree, exists := s.ubtState.treeStore[root]
	return tree, exists
}

// GetCanonicalTree returns the UBT tree at the canonical root.
// This is the replacement for accessing CurrentUBT directly.
// GetCanonicalTree returns the UBT tree at the canonical root.
// This is the replacement for accessing CurrentUBT directly.
// Returns interface{} to satisfy EVMJAMStorage interface.
func (s *StateDBStorage) GetCanonicalTree() interface{} {
	return s.GetCanonicalTreeTyped()
}

// GetCanonicalTreeTyped returns the typed UBT tree at the canonical root (internal use).
func (s *StateDBStorage) GetCanonicalTreeTyped() *UnifiedBinaryTree {
	s.ubtState.mutex.RLock()
	defer s.ubtState.mutex.RUnlock()
	return s.ubtState.treeStore[s.ubtState.canonicalRoot]
}

// StoreTree adds a tree to the treeStore with its root hash.
// If a tree with this root already exists, this is a no-op.
// tree must be *UnifiedBinaryTree.
// Safe for clones: writes are mutex-protected and don't affect canonical state.
func (s *StateDBStorage) StoreTree(tree interface{}) common.Hash {
	ubt, ok := tree.(*UnifiedBinaryTree)
	if !ok {
		log.Error(log.EVM, "StoreTree: tree is not *UnifiedBinaryTree")
		return common.Hash{}
	}
	rootBytes := ubt.RootHash()
	root := common.BytesToHash(rootBytes[:])

	s.ubtState.mutex.Lock()
	defer s.ubtState.mutex.Unlock()

	if _, exists := s.ubtState.treeStore[root]; !exists {
		s.ubtState.treeStore[root] = ubt
	}
	return root
}

// StoreTreeTyped adds a typed tree (for internal use).
// Safe for clones: writes are mutex-protected and don't affect canonical state.
func (s *StateDBStorage) StoreTreeTyped(tree *UnifiedBinaryTree) common.Hash {
	rootBytes := tree.RootHash()
	root := common.BytesToHash(rootBytes[:])

	s.ubtState.mutex.Lock()
	defer s.ubtState.mutex.Unlock()

	if _, exists := s.ubtState.treeStore[root]; !exists {
		s.ubtState.treeStore[root] = tree
	}
	return root
}

// StoreTreeWithRoot adds a tree to the treeStore with an explicit root.
// Use this when you've already computed the root hash.
// Safe for clones: writes are mutex-protected and don't affect canonical state.
func (s *StateDBStorage) StoreTreeWithRoot(root common.Hash, tree *UnifiedBinaryTree) {
	s.ubtState.mutex.Lock()
	defer s.ubtState.mutex.Unlock()
	s.ubtState.treeStore[root] = tree
}

// CloneTreeFromRoot creates a deep copy of the tree at the given root.
// Returns error if no tree exists at that root.
// Safe for clones: this is a read operation that creates a new tree.
func (s *StateDBStorage) CloneTreeFromRoot(root common.Hash) (interface{}, error) {
	s.ubtState.mutex.RLock()
	tree, exists := s.ubtState.treeStore[root]
	s.ubtState.mutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("no tree found at root %s", root.Hex())
	}

	return tree.Copy(), nil
}

// CloneTreeFromRootTyped creates a typed deep copy (for internal use).
// Safe for clones: this is a read operation that creates a new tree.
func (s *StateDBStorage) CloneTreeFromRootTyped(root common.Hash) (*UnifiedBinaryTree, error) {
	s.ubtState.mutex.RLock()
	tree, exists := s.ubtState.treeStore[root]
	s.ubtState.mutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("no tree found at root %s", root.Hex())
	}

	return tree.Copy(), nil
}

// DiscardTree removes a tree from the treeStore.
// Returns true if a tree was removed, false if no tree existed at that root.
// WARNING: Do not discard the canonical root or any root that may be needed for rebuilds.
func (s *StateDBStorage) DiscardTree(root common.Hash) bool {
	s.panicIfClone("DiscardTree")
	s.ubtState.mutex.Lock()
	defer s.ubtState.mutex.Unlock()

	if root == s.ubtState.canonicalRoot {
		log.Warn(log.EVM, "DiscardTree: refusing to discard canonical root",
			"root", root.Hex())
		return false
	}

	if _, exists := s.ubtState.treeStore[root]; exists {
		delete(s.ubtState.treeStore, root)
		return true
	}
	return false
}

// GetActiveRoot returns the currently active state root for execution.
// Returns the zero hash if no active root is set.
// NOTE: activeRoot is PER-INSTANCE, not shared. Each clone has its own activeRoot.
func (s *StateDBStorage) GetActiveRoot() common.Hash {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.activeRoot
}

// SetActiveRoot sets the state root to use for the current execution context.
// Returns error if no tree exists at that root.
// NOTE: activeRoot is PER-INSTANCE. You must call this on the same storage
// instance that the VM will use for reads (typically the StateDB's clone).
func (s *StateDBStorage) SetActiveRoot(root common.Hash) error {
	s.ubtState.mutex.RLock()
	_, exists := s.ubtState.treeStore[root]
	s.ubtState.mutex.RUnlock()

	if root != (common.Hash{}) && !exists {
		return fmt.Errorf("cannot set active root to %s: tree not found", root.Hex())
	}

	s.mutex.Lock()
	s.activeRoot = root
	s.mutex.Unlock()
	return nil
}

// ClearActiveRoot resets the active root to zero (use canonical for reads).
func (s *StateDBStorage) ClearActiveRoot() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.activeRoot = common.Hash{}
}

// GetTreeStoreSize returns the number of trees in the store (for debugging/metrics).
func (s *StateDBStorage) GetTreeStoreSize() int {
	s.ubtState.mutex.RLock()
	defer s.ubtState.mutex.RUnlock()
	return len(s.ubtState.treeStore)
}

// CommitAsCanonical sets the given root as the new canonical state.
// The tree at this root becomes the canonical state that all queries read from.
// Called when a bundle accumulates successfully.
func (s *StateDBStorage) CommitAsCanonical(root common.Hash) error {
	s.panicIfClone("CommitAsCanonical")
	s.ubtState.mutex.Lock()
	defer s.ubtState.mutex.Unlock()

	_, exists := s.ubtState.treeStore[root]
	if !exists {
		return fmt.Errorf("cannot commit root %s as canonical: tree not found", root.Hex())
	}

	s.ubtState.canonicalRoot = root

	log.Info(log.EVM, "CommitAsCanonical: updated canonical root",
		"root", root.Hex(),
		"treeStoreSize", len(s.ubtState.treeStore))

	return nil
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

// StoreUBTTransition stores a UBT tree state by its root hash.
// This method now uses treeStore (root-first model).
func (store *StateDBStorage) StoreUBTTransition(ubtRoot common.Hash, tree *UnifiedBinaryTree) error {
	if tree == nil {
		return fmt.Errorf("cannot store nil UBT tree")
	}

	store.StoreTreeWithRoot(ubtRoot, tree)
	return nil
}

// GetUBTTreeAtRoot retrieves a UBT tree state by its root hash.
// This method now uses treeStore (root-first model).
func (store *StateDBStorage) GetUBTTreeAtRoot(ubtRoot common.Hash) (interface{}, bool) {
	return store.GetTreeByRoot(ubtRoot)
}

// GetUBTNodeForBlockNumber maps a block number string to the corresponding UBT tree.
// Uses treeStore with canonicalRoot for "latest" (root-first model).
func (store *StateDBStorage) GetUBTNodeForBlockNumber(blockNumber string) (interface{}, bool) {
	// For "latest" or empty, return the canonical tree
	if blockNumber == "latest" || blockNumber == "" {
		tree := store.GetCanonicalTree()
		if tree != nil {
			return tree, true
		}
		return nil, false
	}
	// For specific block numbers, need to lookup block and get its state root
	// This requires accessing block storage which needs to be implemented
	return nil, false
}

// GetBalance reads balance from the UBT tree using BasicData.
func (store *StateDBStorage) GetBalance(tree interface{}, address common.Address) (common.Hash, error) {
	ubtTree, ok := tree.(*UnifiedBinaryTree)
	if !ok {
		return common.Hash{}, fmt.Errorf("invalid tree type")
	}

	basicKey := GetBasicDataKey(defaultUBTProfile, address)
	value, found, _ := ubtTree.Get(&basicKey)
	if !found {
		return common.Hash{}, nil
	}

	basicData := DecodeBasicDataLeaf(value)
	var balanceHash common.Hash
	copy(balanceHash[16:32], basicData.Balance[:])

	return balanceHash, nil
}

// GetNonce reads nonce from the UBT tree using BasicData.
func (store *StateDBStorage) GetNonce(tree interface{}, address common.Address) (uint64, error) {
	ubtTree, ok := tree.(*UnifiedBinaryTree)
	if !ok {
		return 0, fmt.Errorf("invalid tree type")
	}

	basicKey := GetBasicDataKey(defaultUBTProfile, address)
	value, found, _ := ubtTree.Get(&basicKey)
	if !found {
		return 0, nil
	}

	basicData := DecodeBasicDataLeaf(value)
	return basicData.Nonce, nil
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

// ===== EVM State Access Methods (UBT-backed log, UBT reads) =====

// FetchBalance fetches balance from the UBT tree.
// Uses GetActiveTree() to support multi-snapshot parallel bundle building.
func (store *StateDBStorage) FetchBalance(address common.Address, txIndex uint32) ([32]byte, error) {
	var balance [32]byte

	tree := store.GetActiveTreeTyped()
	if tree == nil {
		return balance, nil // Return zero balance if no tree
	}

	// Compute UBT tree key for balance
	ubtKey := GetBasicDataKey(defaultUBTProfile, address)

	// Track the read in ubtReadLog
	store.AppendUBTRead(types.UBTRead{
		TreeKey: ubtTreeKeyToHash(ubtKey),
		Address: address,
		KeyType: 0, // BasicData
		Extra:   0,
		TxIndex: txIndex,
	})

	// Read BasicData from UBT tree
	if basicDataValue, found, _ := tree.Get(&ubtKey); found {
		basicData := DecodeBasicDataLeaf(basicDataValue)
		copy(balance[16:32], basicData.Balance[:])
	}

	return balance, nil
}

// FetchNonce fetches nonce from the UBT tree.
// Uses GetActiveTree() to support multi-snapshot parallel bundle building.
func (store *StateDBStorage) FetchNonce(address common.Address, txIndex uint32) ([32]byte, error) {
	var nonce [32]byte

	tree := store.GetActiveTreeTyped()
	if tree == nil {
		return nonce, nil // Return zero nonce if no tree
	}

	// Compute UBT tree key for nonce
	ubtKey := GetBasicDataKey(defaultUBTProfile, address)

	// Track the read in ubtReadLog
	store.AppendUBTRead(types.UBTRead{
		TreeKey: ubtTreeKeyToHash(ubtKey),
		Address: address,
		KeyType: 0, // BasicData
		Extra:   0,
		TxIndex: txIndex,
	})

	// Read BasicData from UBT tree
	if basicDataValue, found, _ := tree.Get(&ubtKey); found {
		basicData := DecodeBasicDataLeaf(basicDataValue)
		binary.BigEndian.PutUint64(nonce[24:32], basicData.Nonce)
	}

	return nonce, nil
}

// FetchCode fetches code from the UBT tree.
// Uses GetActiveTree() to support multi-snapshot parallel bundle building.
func (store *StateDBStorage) FetchCode(address common.Address, txIndex uint32) ([]byte, uint32, error) {
	tree := store.GetActiveTreeTyped()
	if tree == nil {
		return nil, 0, nil
	}

	// Read code size from BasicData first
	var codeSize uint32

	ubtBasicKey := GetBasicDataKey(defaultUBTProfile, address)

	// Track BasicData read for UBT witness
	store.AppendUBTRead(types.UBTRead{
		TreeKey: ubtTreeKeyToHash(ubtBasicKey),
		Address: address,
		KeyType: 0, // BasicData
		Extra:   0,
		TxIndex: txIndex,
	})

	basicDataValue, found, _ := tree.Get(&ubtBasicKey)
	if found {
		basicData := DecodeBasicDataLeaf(basicDataValue)
		codeSize = basicData.CodeSize
	}

	if codeSize == 0 {
		// Track CodeHash read even for EOAs for witness completeness
		ubtCodeHashKey := GetCodeHashKey(defaultUBTProfile, address)
		store.AppendUBTRead(types.UBTRead{
			TreeKey: ubtTreeKeyToHash(ubtCodeHashKey),
			Address: address,
			KeyType: 1, // CodeHash
			Extra:   0,
			TxIndex: txIndex,
		})
		return nil, 0, nil
	}

	// Calculate number of chunks needed
	numChunks := (codeSize + 30) / 31 // Round up

	// Allocate buffer for reconstructed code
	code := make([]byte, 0, codeSize)

	// Read each chunk
	for chunkID := uint64(0); chunkID < uint64(numChunks); chunkID++ {
		ubtChunkKey := GetCodeChunkKey(defaultUBTProfile, address, chunkID)

		// Track the read in ubtReadLog
		store.AppendUBTRead(types.UBTRead{
			TreeKey: ubtTreeKeyToHash(ubtChunkKey),
			Address: address,
			KeyType: 2, // CodeChunk
			Extra:   chunkID,
			TxIndex: txIndex,
		})

		// Read chunk from tree
		chunkData, found, _ := tree.Get(&ubtChunkKey)
		if !found {
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

// FetchCodeHash fetches code hash from the UBT tree.
// Uses GetActiveTree() to support multi-snapshot parallel bundle building.
func (store *StateDBStorage) FetchCodeHash(address common.Address, txIndex uint32) ([32]byte, error) {
	var codeHash [32]byte

	tree := store.GetActiveTreeTyped()
	if tree == nil {
		codeHash = emptyCodeHash()
		return codeHash, nil
	}

	// Compute UBT tree key for code hash
	ubtKey := GetCodeHashKey(defaultUBTProfile, address)

	// Track the read in ubtReadLog
	store.AppendUBTRead(types.UBTRead{
		TreeKey: ubtTreeKeyToHash(ubtKey),
		Address: address,
		KeyType: 1, // CodeHash
		Extra:   0,
		TxIndex: txIndex,
	})

	// Read code hash from UBT tree
	codeHashData, found, _ := tree.Get(&ubtKey)
	if found {
		codeHash = codeHashData
	} else {
		// Code hash not found - return empty code hash
		codeHash = emptyCodeHash()
	}

	return codeHash, nil
}

// FetchStorage fetches storage value from the UBT tree.
// Returns (value, found, error) where found=false indicates the slot was absent.
// Uses GetActiveTree() to support multi-snapshot parallel bundle building.
func (store *StateDBStorage) FetchStorage(address common.Address, storageKey [32]byte, txIndex uint32) ([32]byte, bool, error) {
	var value [32]byte
	found := false

	tree := store.GetActiveTreeTyped()
	if tree == nil {
		return value, found, nil
	}

	// Compute UBT tree key for storage slot
	ubtKey := GetStorageSlotKey(defaultUBTProfile, address, storageKey)

	// Track the read in ubtReadLog
	store.AppendUBTRead(types.UBTRead{
		TreeKey:    ubtTreeKeyToHash(ubtKey),
		Address:    address,
		KeyType:    3, // Storage
		Extra:      0,
		StorageKey: storageKey,
		TxIndex:    txIndex,
	})

	// Read storage value from UBT tree
	storageData, ok, _ := tree.Get(&ubtKey)
	if ok {
		value = storageData
		found = true
	}

	return value, found, nil
}

// ===== UBT Read Log Management =====

// AppendUBTRead appends a UBT read to the log.
// If read logging is disabled (Phase 1 mode), this is a no-op.
func (store *StateDBStorage) AppendUBTRead(read types.UBTRead) {
	store.ubtReadLogMutex.Lock()
	defer store.ubtReadLogMutex.Unlock()
	if !store.ubtReadLogEnabled {
		return // Phase 1 mode: skip logging for faster execution
	}
	store.ubtReadLog = append(store.ubtReadLog, read)
}

// GetUBTReadLog returns a copy of the UBT read log.
func (store *StateDBStorage) GetUBTReadLog() []types.UBTRead {
	store.ubtReadLogMutex.Lock()
	defer store.ubtReadLogMutex.Unlock()
	logCopy := make([]types.UBTRead, len(store.ubtReadLog))
	copy(logCopy, store.ubtReadLog)
	return logCopy
}

// ClearUBTReadLog clears the UBT read log.
func (store *StateDBStorage) ClearUBTReadLog() {
	store.ubtReadLogMutex.Lock()
	defer store.ubtReadLogMutex.Unlock()
	store.ubtReadLog = nil
}

// SetUBTReadLogEnabled enables or disables UBT read logging.
// When disabled (Phase 1), AppendUBTRead is a no-op for faster execution.
// When enabled (Phase 2/normal), reads are logged for witness generation.
func (store *StateDBStorage) SetUBTReadLogEnabled(enabled bool) {
	store.ubtReadLogMutex.Lock()
	defer store.ubtReadLogMutex.Unlock()
	store.ubtReadLogEnabled = enabled
	log.Debug(log.EVM, "SetUBTReadLogEnabled", "enabled", enabled)
}

// IsUBTReadLogEnabled returns whether UBT read logging is currently enabled.
func (store *StateDBStorage) IsUBTReadLogEnabled() bool {
	store.ubtReadLogMutex.Lock()
	defer store.ubtReadLogMutex.Unlock()
	return store.ubtReadLogEnabled
}

// GetActiveUBTRoot returns the root hash of the currently active tree.
// Priority: pinnedTree (Phase 2) > activeRoot (root-first) > canonical.
// This is used by ExecutePhase1/BuildBundle to get the post-state root.
func (store *StateDBStorage) GetActiveUBTRoot() common.Hash {
	tree := store.GetActiveTreeTyped()
	if tree == nil {
		return common.Hash{}
	}
	root := tree.RootHash()
	return common.BytesToHash(root[:])
}

// PinToStateRoot pins execution to a specific historical state root.
// Used for Phase 1 verification where we need to re-execute against the same pre-state.
// Returns error if the state root is not available in treeStore.
//
// ROOT-FIRST ONLY: This method now only checks treeStore.
// All trees must be stored via StoreTreeWithRoot or ApplyWritesToTree.
func (store *StateDBStorage) PinToStateRoot(root common.Hash) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	// Check treeStore (root-first state model - the only source)
	store.ubtState.mutex.RLock()
	tree, found := store.ubtState.treeStore[root]
	treeStoreCount := len(store.ubtState.treeStore)
	store.ubtState.mutex.RUnlock()

	if !found {
		// Log available roots for debugging
		store.ubtState.mutex.RLock()
		treeStoreRoots := make([]string, 0, len(store.ubtState.treeStore))
		for r := range store.ubtState.treeStore {
			treeStoreRoots = append(treeStoreRoots, r.Hex()[:10])
		}
		store.ubtState.mutex.RUnlock()

		log.Warn(log.EVM, "PinToStateRoot: root not found in treeStore",
			"requestedRoot", root.Hex(),
			"treeStoreCount", treeStoreCount,
			"availableRoots", treeStoreRoots)
		return fmt.Errorf("state root %s not available for pinning (treeStore only)", root.Hex())
	}

	store.pinnedStateRoot = &root
	store.pinnedTree = tree
	log.Info(log.EVM, "PinToStateRoot: pinned to state root",
		"root", root.Hex(),
		"treeStoreCount", treeStoreCount)
	return nil
}

// UnpinState releases the pinned state and returns to normal operation.
func (store *StateDBStorage) UnpinState() {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	if store.pinnedStateRoot != nil {
		log.Info(log.EVM, "UnpinState", "was_pinned_to", store.pinnedStateRoot.Hex())
	}
	store.pinnedStateRoot = nil
	store.pinnedTree = nil
}

func (store *StateDBStorage) ResetTrie() {
	store.panicIfClone("ResetTrie")

	// Create new genesis tree and set as canonical
	genesisTree := NewUnifiedBinaryTree(Config{Profile: defaultUBTProfile})
	genesisRootBytes := genesisTree.RootHash()
	genesisRoot := common.BytesToHash(genesisRootBytes[:])

	store.ubtState.mutex.Lock()
	store.ubtState.treeStore = make(map[common.Hash]*UnifiedBinaryTree)
	store.ubtState.treeStore[genesisRoot] = genesisTree
	store.ubtState.canonicalRoot = genesisRoot
	store.activeRoot = common.Hash{}
	store.ubtState.mutex.Unlock()

	// Reset other state
	store.trieDB = NewMerkleTree(nil)
	store.stagedInserts = make(map[common.Hash][]byte)
	store.stagedDeletes = make(map[common.Hash]bool)
	store.keys = make(map[common.Hash]bool)
	store.Root = store.trieDB.GetRoot()

	// DEPRECATED: kept for interface compatibility
	store.ubtState.ubtRoots = make(map[common.Hash]*UnifiedBinaryTree)
}

// BuildUBTWitness builds dual UBT witnesses and stores the post-state tree.
// Uses GetActiveTree() to support root-first parallel bundle building.
func (store *StateDBStorage) BuildUBTWitness(contractWitnessBlob []byte) ([]byte, []byte, error) {
	// Use GetActiveTree() to get the correct tree for parallel bundle building
	// This returns pinnedTree > activeRoot tree > canonical tree
	tree := store.GetActiveTreeTyped()
	if tree == nil {
		return nil, nil, fmt.Errorf("no UBT tree available")
	}

	// Get the UBT read log
	ubtReadLog := store.GetUBTReadLog()

	// Build witness using the UBT function from witness.go
	preWitness, postWitness, postUBTRoot, postTree, err := BuildUBTWitness(ubtReadLog, contractWitnessBlob, tree)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build witness: %w", err)
	}

	// Store the post-state tree in treeStore (root-first model)
	store.StoreTreeWithRoot(postUBTRoot, postTree)

	return preWitness, postWitness, nil
}

// ComputeBlockAccessListHash builds BAL from UBT witnesses and returns hash + statistics
// Used by both builder (to compute hash for payload) and guarantor (to verify builder's hash)
func (store *StateDBStorage) ComputeBlockAccessListHash(preWitness []byte, postWitness []byte) (common.Hash, uint32, uint32, error) {
	// Build Block Access List from witnesses
	bal, err := BuildBlockAccessList(preWitness, postWitness)
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

// Multi-Rollup Support: Helper Functions

// ubtRootKey generates the key for storing service UBT roots
// Format: "vr_{serviceID}_{blockNumber}"
func ubtRootKey(serviceID uint32, blockNumber uint32) string {
	return fmt.Sprintf("vr_%d_%d", serviceID, blockNumber)
}

// jamStateRootKey generates the key for storing JAM state root mappings
// Format: "jsr_{serviceID}_{blockNumber}"
func jamStateRootKey(serviceID uint32, blockNumber uint32) string {
	return fmt.Sprintf("jsr_%d_%d", serviceID, blockNumber)
}

// blockHashKey generates the key for block hash to block number lookup
// Format: "bhash_{serviceID}_{blockHash}"
func blockHashKey(serviceID uint32, blockHash common.Hash) string {
	return fmt.Sprintf("bhash_%d_%s", serviceID, blockHash.Hex())
}

// parseBlockNumber parses a block number string into uint32
// Supports: "latest", "earliest", "pending", or hex number (0x...)
func parseBlockNumber(blockNumber string, latestBlock uint64) (uint32, error) {
	switch blockNumber {
	case "latest", "":
		return uint32(latestBlock), nil
	case "earliest":
		return 0, nil
	case "pending":
		return uint32(latestBlock), nil
	default:
		// Try parsing as hex
		if len(blockNumber) > 2 && blockNumber[:2] == "0x" {
			blockNum, err := hex.DecodeString(blockNumber[2:])
			if err != nil {
				return 0, fmt.Errorf("invalid hex block number: %w", err)
			}
			var num uint64
			for _, b := range blockNum {
				num = (num << 8) | uint64(b)
			}
			return uint32(num), nil
		}
		return 0, fmt.Errorf("invalid block number format: %s", blockNumber)
	}
}

// Multi-Rollup Support: Implementation Methods

// GetUBTNodeForServiceBlock retrieves the UBT tree for a specific service's block
// Uses treeStore for root-first state management.
func (store *StateDBStorage) GetUBTNodeForServiceBlock(serviceID uint32, blockNumber string) (interface{}, bool) {
	store.serviceState.mutex.RLock()
	defer store.serviceState.mutex.RUnlock()

	// For "latest"/"pending" queries, return canonical tree
	if blockNumber == "latest" || blockNumber == "pending" {
		tree := store.GetCanonicalTree()
		if tree != nil {
			return tree, true
		}
	}

	// Get latest block for this service
	latestBlock, exists := store.serviceState.latestRollupBlock[serviceID]
	if !exists {
		log.Info(log.EVM, "GetUBTNodeForServiceBlock: no indexed blocks",
			"serviceID", serviceID,
			"blockNumber", blockNumber)
		return nil, false
	}

	// Parse block number
	blockNum, err := parseBlockNumber(blockNumber, latestBlock)
	if err != nil {
		return nil, false
	}

	// Lookup UBT root
	key := ubtRootKey(serviceID, blockNum)
	ubtRoot, ok := store.serviceState.serviceUBTRoots[key]
	if !ok {
		// Block not indexed yet - fall back to canonical for latest block
		if uint64(blockNum) == latestBlock || blockNumber == "latest" || blockNumber == "pending" {
			tree := store.GetCanonicalTree()
			if tree != nil {
				return tree, true
			}
		}
		return nil, false
	}

	// Retrieve UBT tree from treeStore (root-first model)
	tree, found := store.GetTreeByRootTyped(ubtRoot)
	if !found {
		// Tree not found in treeStore - fall back to canonical for latest
		if blockNumber == "latest" || blockNumber == "pending" {
			canonicalTree := store.GetCanonicalTree()
			if canonicalTree != nil {
				return canonicalTree, true
			}
		}
	}
	return tree, found
}

// StoreServiceBlock persists an EVM block for a specific service
func (store *StateDBStorage) StoreServiceBlock(serviceID uint32, blockIface interface{}, jamStateRoot common.Hash, jamSlot uint32) error {
	block, ok := blockIface.(*evmtypes.EvmBlockPayload)
	if !ok {
		return fmt.Errorf("block must be *evmtypes.EvmBlockPayload")
	}

	store.serviceState.mutex.Lock()
	defer store.serviceState.mutex.Unlock()

	blockNumber := block.Number

	// Create ServiceBlockIndex
	index := &types.ServiceBlockIndex{
		ServiceID:    serviceID,
		BlockNumber:  blockNumber,
		BlockHash:    block.WorkPackageHash,
		UBTRoot:      block.UBTRoot,
		JAMStateRoot: jamStateRoot,
		JAMSlot:      jamSlot,
	}

	// Store UBT root mapping
	ubtKey := ubtRootKey(serviceID, blockNumber)
	store.serviceState.serviceUBTRoots[ubtKey] = block.UBTRoot

	// Store JAM state root mapping
	jsrKey := jamStateRootKey(serviceID, blockNumber)
	store.serviceState.serviceJAMStateRoots[jsrKey] = index

	// Store block hash index
	bhKey := blockHashKey(serviceID, block.WorkPackageHash)
	store.serviceState.serviceBlockHashIndex[bhKey] = uint64(blockNumber)

	// Update latest block
	if currentLatest, exists := store.serviceState.latestRollupBlock[serviceID]; !exists || uint64(blockNumber) > currentLatest {
		store.serviceState.latestRollupBlock[serviceID] = uint64(blockNumber)
	}

	// Persist EvmBlockPayload to LevelDB
	// Key: "sblock_{serviceID}_{blockNumber}"
	blockKey := fmt.Sprintf("sblock_%d_%d", serviceID, blockNumber)
	blockBytes, err := json.Marshal(block)
	if err != nil {
		return fmt.Errorf("failed to marshal block: %w", err)
	}
	if err := store.db.Put([]byte(blockKey), blockBytes, nil); err != nil {
		return fmt.Errorf("failed to store block: %w", err)
	}

	// Index transaction hashes for fast eth_getTransactionReceipt lookup
	// Key: "stx_{serviceID}_{txHash}" -> "blockNumber:txIndex" (8 bytes)
	for i, txHash := range block.TxHashes {
		txKey := fmt.Sprintf("stx_%d_%s", serviceID, txHash.Hex())
		txValue := make([]byte, 8)
		binary.LittleEndian.PutUint32(txValue[0:4], blockNumber)
		binary.LittleEndian.PutUint32(txValue[4:8], uint32(i))
		if err := store.db.Put([]byte(txKey), txValue, nil); err != nil {
			log.Warn(log.EVM, "Failed to index transaction hash", "txHash", txHash.Hex(), "err", err)
			// Continue - don't fail the whole block store for index failure
		}
	}

	return nil
}

// GetServiceBlock retrieves an EVM block by service ID and block number
func (store *StateDBStorage) GetServiceBlock(serviceID uint32, blockNumber string) (interface{}, error) {
	store.serviceState.mutex.RLock()
	latestBlock, exists := store.serviceState.latestRollupBlock[serviceID]
	store.serviceState.mutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("no blocks for service %d", serviceID)
	}

	// Parse block number
	blockNum, err := parseBlockNumber(blockNumber, latestBlock)
	if err != nil {
		return nil, err
	}

	// Retrieve from LevelDB
	blockKey := fmt.Sprintf("sblock_%d_%d", serviceID, blockNum)
	blockBytes, err := store.db.Get([]byte(blockKey), nil)
	if err != nil {
		return nil, fmt.Errorf("block not found: %w", err)
	}

	var block evmtypes.EvmBlockPayload
	if err := json.Unmarshal(blockBytes, &block); err != nil {
		return nil, fmt.Errorf("failed to unmarshal block: %w", err)
	}

	return &block, nil
}

// GetTransactionByHash finds a transaction in a service's block history
// Uses tx hash index for O(1) lookup instead of scanning all blocks
func (store *StateDBStorage) GetTransactionByHash(serviceID uint32, txHash common.Hash) (*types.Transaction, *types.BlockMetadata, error) {
	// First, try the tx hash index (fast path)
	txKey := fmt.Sprintf("stx_%d_%s", serviceID, txHash.Hex())
	txValue, err := store.db.Get([]byte(txKey), nil)
	if err == nil && len(txValue) == 8 {
		// Index hit - extract blockNumber and txIndex
		blockNum := binary.LittleEndian.Uint32(txValue[0:4])
		txIndex := binary.LittleEndian.Uint32(txValue[4:8])

		// Load the block to get block hash
		blockKey := fmt.Sprintf("sblock_%d_%d", serviceID, blockNum)
		blockBytes, err := store.db.Get([]byte(blockKey), nil)
		if err == nil {
			var block evmtypes.EvmBlockPayload
			if err := json.Unmarshal(blockBytes, &block); err == nil {
				metadata := &types.BlockMetadata{
					BlockHash:   block.WorkPackageHash,
					BlockNumber: blockNum,
					TxIndex:     txIndex,
				}
				tx := &types.Transaction{
					Hash: txHash,
				}
				return tx, metadata, nil
			}
		}
	}

	// Fallback: scan blocks (for blocks stored before index was added)
	store.serviceState.mutex.RLock()
	latestBlock, exists := store.serviceState.latestRollupBlock[serviceID]
	store.serviceState.mutex.RUnlock()

	if !exists {
		return nil, nil, fmt.Errorf("no blocks for service %d", serviceID)
	}

	for blockNum := latestBlock; blockNum >= 0; blockNum-- {
		blockKey := fmt.Sprintf("sblock_%d_%d", serviceID, blockNum)
		blockBytes, err := store.db.Get([]byte(blockKey), nil)
		if err != nil {
			if blockNum == 0 {
				break
			}
			continue
		}

		var block evmtypes.EvmBlockPayload
		if err := json.Unmarshal(blockBytes, &block); err != nil {
			if blockNum == 0 {
				break
			}
			continue
		}

		// Search transaction hashes in this block
		for txIdx, hash := range block.TxHashes {
			if hash == txHash {
				metadata := &types.BlockMetadata{
					BlockHash:   block.WorkPackageHash,
					BlockNumber: block.Number,
					TxIndex:     uint32(txIdx),
				}
				tx := &types.Transaction{
					Hash: txHash,
				}
				return tx, metadata, nil
			}
		}

		if blockNum == 0 {
			break
		}
	}

	return nil, nil, fmt.Errorf("transaction not found")
}

// GetTxLocation returns the block number and tx index for a transaction hash
// This is a fast O(1) lookup using the tx hash index
func (store *StateDBStorage) GetTxLocation(serviceID uint32, txHash common.Hash) (blockNumber uint32, txIndex uint32, found bool) {
	txKey := fmt.Sprintf("stx_%d_%s", serviceID, txHash.Hex())
	txValue, err := store.db.Get([]byte(txKey), nil)
	if err != nil || len(txValue) != 8 {
		return 0, 0, false
	}
	blockNumber = binary.LittleEndian.Uint32(txValue[0:4])
	txIndex = binary.LittleEndian.Uint32(txValue[4:8])
	return blockNumber, txIndex, true
}

// GetTransactionReceipt retrieves a full transaction receipt from stored block data.
// This is the primary method for eth_getTransactionReceipt when using builder/LevelDB as primary source.
// Returns the TransactionReceipt with all fields populated (Success, UsedGas, Logs, etc.)
// Returns nil if not found. The returned interface{} is *evmtypes.TransactionReceipt.
func (store *StateDBStorage) GetTransactionReceipt(serviceID uint32, txHash common.Hash) (interface{}, error) {
	// First, find the block containing this transaction
	blockNum, txIndex, found := store.GetTxLocation(serviceID, txHash)
	if !found {
		return nil, nil // Transaction not indexed
	}

	// Load the block
	blockKey := fmt.Sprintf("sblock_%d_%d", serviceID, blockNum)
	blockBytes, err := store.db.Get([]byte(blockKey), nil)
	if err != nil {
		return nil, fmt.Errorf("block %d not found: %w", blockNum, err)
	}

	var block evmtypes.EvmBlockPayload
	if err := json.Unmarshal(blockBytes, &block); err != nil {
		return nil, fmt.Errorf("failed to unmarshal block: %w", err)
	}

	// Check if receipt data is available
	if int(txIndex) >= len(block.Transactions) {
		// Receipt data not stored (old blocks or missing data)
		// Return minimal receipt with just the metadata
		return &evmtypes.TransactionReceipt{
			TransactionHash:  txHash,
			BlockHash:        block.WorkPackageHash,
			BlockNumber:      blockNum,
			TransactionIndex: txIndex,
			Timestamp:        block.Timestamp,
		}, nil
	}

	// Get the full receipt
	receipt := block.Transactions[txIndex]

	// Fill in block context if not already set
	if receipt.BlockHash == (common.Hash{}) {
		receipt.BlockHash = block.WorkPackageHash
	}
	if receipt.BlockNumber == 0 {
		receipt.BlockNumber = blockNum
	}
	if receipt.TransactionIndex == 0 && txIndex > 0 {
		receipt.TransactionIndex = txIndex
	}
	if receipt.Timestamp == 0 {
		receipt.Timestamp = block.Timestamp
	}

	return &receipt, nil
}

// GetBlockByNumber retrieves full EVM block by service ID and block number
func (store *StateDBStorage) GetBlockByNumber(serviceID uint32, blockNumber string) (*types.EVMBlock, error) {
	blockIface, err := store.GetServiceBlock(serviceID, blockNumber)
	if err != nil {
		return nil, err
	}

	block, ok := blockIface.(*evmtypes.EvmBlockPayload)
	if !ok {
		return nil, fmt.Errorf("invalid block type")
	}

	// Convert EvmBlockPayload to EVMBlock
	transactions := make([]types.Transaction, len(block.TxHashes))
	for i, txHash := range block.TxHashes {
		transactions[i] = types.Transaction{Hash: txHash}
	}

	receipts := make([]types.Receipt, len(block.Transactions))
	for i, txReceipt := range block.Transactions {
		receipts[i] = types.Receipt{
			TransactionHash: txReceipt.TransactionHash,
			Success:         txReceipt.Success,
			UsedGas:         txReceipt.UsedGas,
		}
	}

	evmBlock := &types.EVMBlock{
		BlockNumber:  block.Number,
		BlockHash:    block.WorkPackageHash,
		ParentHash:   common.Hash{}, // No parent hash in EvmBlockPayload
		Timestamp:    block.Timestamp,
		UBTRoot:      block.UBTRoot,
		Transactions: transactions,
		Receipts:     receipts,
	}

	return evmBlock, nil
}

// GetBlockByHash retrieves full EVM block by service ID and block hash
func (store *StateDBStorage) GetBlockByHash(serviceID uint32, blockHash common.Hash) (*types.EVMBlock, error) {
	store.serviceState.mutex.RLock()
	bhKey := blockHashKey(serviceID, blockHash)
	blockNum, exists := store.serviceState.serviceBlockHashIndex[bhKey]
	store.serviceState.mutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("block hash not found")
	}

	return store.GetBlockByNumber(serviceID, fmt.Sprintf("0x%x", blockNum))
}

// FinalizeEVMBlock is a helper to finalize an EVM block after accumulation
// This stores the block data and associates it with JAM state
// Call this after accumulate completes for an EVM service
func (store *StateDBStorage) FinalizeEVMBlock(
	serviceID uint32,
	blockPayload interface{},
	jamStateRoot common.Hash,
	jamSlot uint32,
) error {
	// Type assert to *evmtypes.EvmBlockPayload
	payload, ok := blockPayload.(*evmtypes.EvmBlockPayload)
	if !ok {
		return fmt.Errorf("invalid block payload type, expected *evmtypes.EvmBlockPayload")
	}

	// Store the block using StoreServiceBlock
	if err := store.StoreServiceBlock(serviceID, payload, jamStateRoot, jamSlot); err != nil {
		return fmt.Errorf("failed to store service block: %w", err)
	}

	// Store the UBT tree snapshot in treeStore (root-first model)
	canonicalTree := store.GetCanonicalTreeTyped()
	if canonicalTree != nil {
		// The canonical tree should already be the post-state tree
		// Store it under the block's UBT root
		store.StoreTreeWithRoot(payload.UBTRoot, canonicalTree)
		log.Info(log.EVM, "Stored UBT tree snapshot",
			"serviceID", serviceID,
			"blockNumber", payload.Number,
			"ubtRoot", payload.UBTRoot.String())
	}

	// Create checkpoint if needed (UBT checkpoint tree)
	if store.CheckpointManager != nil && canonicalTree != nil {
		blockHeight := uint64(payload.Number)

		// Pin genesis checkpoint (never evicted)
		if blockHeight == 0 {
			store.CheckpointManager.PinCheckpoint(blockHeight, canonicalTree)
			log.Info(log.EVM, "Pinned genesis checkpoint",
				"height", blockHeight,
				"ubtRoot", payload.UBTRoot.String())
		} else if store.CheckpointManager.ShouldCreateCheckpoint(blockHeight) {
			// Add to LRU cache for fine/coarse checkpoints
			store.CheckpointManager.AddCheckpoint(blockHeight, canonicalTree)
			log.Info(log.EVM, "Created checkpoint",
				"height", blockHeight,
				"ubtRoot", payload.UBTRoot.String())
		}
	}

	log.Info(log.EVM, "Finalized EVM block",
		"serviceID", serviceID,
		"blockNumber", payload.Number,
		"blockHash", payload.WorkPackageHash.String(),
		"ubtRoot", payload.UBTRoot.String(),
		"jamStateRoot", jamStateRoot.String(),
		"jamSlot", jamSlot)

	return nil
}

// InitializeEVMGenesis creates a genesis block for an EVM service with an initial account balance.
// This sets up the UBT tree with the genesis account and stores block 0.
func (store *StateDBStorage) InitializeEVMGenesis(
	serviceID uint32,
	issuerAddress common.Address,
	startBalance int64,
) (common.Hash, error) {
	log.Info(log.EVM, "InitializeEVMGenesis - Initializing EVM genesis state",
		"serviceID", serviceID,
		"issuerAddress", issuerAddress.Hex(),
		"startBalance", startBalance)

	// Convert startBalance to Wei (18 decimals)
	balanceWei := new(big.Int).Mul(big.NewInt(startBalance), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))

	// Get canonical tree (root-first model)
	canonicalTree := store.GetCanonicalTreeTyped()
	if canonicalTree == nil {
		return common.Hash{}, fmt.Errorf("canonical tree not available")
	}

	// Insert account BasicData (balance + nonce).
	var balance [16]byte
	balanceBytes := balanceWei.Bytes()
	if len(balanceBytes) > 16 {
		return common.Hash{}, fmt.Errorf("balance too large: %d bytes", len(balanceBytes))
	}
	copy(balance[16-len(balanceBytes):], balanceBytes)

	basicData := NewBasicDataLeaf(1, balance, 0)
	basicDataKey := GetBasicDataKey(defaultUBTProfile, issuerAddress)
	canonicalTree.Insert(basicDataKey, basicData.Encode())

	// Compute the updated UBT root
	ubtRoot := canonicalTree.RootHash()
	ubtRootHash := common.BytesToHash(ubtRoot[:])

	// Store the updated tree in treeStore and set as canonical (root-first model)
	store.StoreTreeWithRoot(ubtRootHash, canonicalTree)
	if err := store.SetCanonicalRoot(ubtRootHash); err != nil {
		return common.Hash{}, fmt.Errorf("failed to set canonical root: %w", err)
	}

	// Create genesis block (block 0) for EVM service
	genesisBlock := &evmtypes.EvmBlockPayload{
		Number:              0,
		WorkPackageHash:     common.Hash{},
		SegmentRoot:         common.Hash{},
		PayloadLength:       148,
		NumTransactions:     0,
		Timestamp:           0,
		GasUsed:             0,
		UBTRoot:             ubtRootHash,
		TransactionsRoot:    common.Hash{},
		ReceiptRoot:         common.Hash{},
		BlockAccessListHash: common.Hash{},
		TxHashes:            []common.Hash{},
		ReceiptHashes:       []common.Hash{},
		Transactions:        []evmtypes.TransactionReceipt{},
	}

	// Store genesis block
	if err := store.StoreServiceBlock(serviceID, genesisBlock, common.Hash{}, 0); err != nil {
		return common.Hash{}, fmt.Errorf("failed to store genesis block: %w", err)
	}

	log.Info(log.EVM, "✅ InitializeEVMGenesis complete",
		"stateRoot", ubtRootHash.Hex(),
		"issuerBalance", balanceWei.String())

	return ubtRootHash, nil
}

// LoadBlockByHeight loads a block by height for checkpoint manager
// This is used by CheckpointTreeManager for delta replay
// Currently assumes serviceID 1 (primary EVM service)
// TODO: Support multi-service checkpoint managers
func (store *StateDBStorage) LoadBlockByHeight(height uint64) (*evmtypes.EvmBlockPayload, error) {
	// Use serviceID 1 as default (primary EVM service)
	const defaultServiceID = uint32(1)

	blockKey := fmt.Sprintf("sblock_%d_%d", defaultServiceID, height)
	blockBytes, err := store.db.Get([]byte(blockKey), nil)
	if err != nil {
		return nil, fmt.Errorf("block %d not found: %w", height, err)
	}

	var block evmtypes.EvmBlockPayload
	if err := json.Unmarshal(blockBytes, &block); err != nil {
		return nil, fmt.Errorf("failed to unmarshal block %d: %w", height, err)
	}

	return &block, nil
}
