package storage

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"reflect"
	"sync"

	evmtypes "github.com/jam-duna/jamduna/types"
	"github.com/jam-duna/jamduna/common"
	log "github.com/jam-duna/jamduna/log"
	"github.com/jam-duna/jamduna/types"
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

// SharedManagers holds thread-safe managers shared across NewSession instances.
type SharedManagers struct {
	UBT     *UBTTreeManager
	Service *ServiceStateManager
}

// SessionContexts holds per-instance isolated state (copied on NewSession).
type SessionContexts struct {
	JAM *JAMTrieSession
	UBT *UBTExecContext
}

// WitnessCache holds witness data for EVM execution.
type WitnessCache struct {
	Storageshard map[common.Address]evmtypes.ContractStorage
	Code         map[common.Address][]byte
	UBTProofs    map[common.Address]evmtypes.UBTMultiproof
	mutex        sync.RWMutex
}

// Infrastructure holds external dependencies.
type Infrastructure struct {
	JAMDA           types.JAMDA
	TelemetryClient types.TelemetryClient
	NodeID          uint16
	LogChan         chan LogMessage
}

// StorageHub is the main storage facade. Trees are accessed by root hash (root-first model).
type StorageHub struct {
	Persist           *PersistenceStore
	Shared            SharedManagers
	Session           SessionContexts
	Witness           WitnessCache
	Infra             Infrastructure
	CheckpointManager *CheckpointTreeManager

	mu   sync.RWMutex // Protects Root, keys, isSessionCopy (not shared across NewSession)
	Root common.Hash
	keys map[common.Hash]bool

	WorkPackageContext       context.Context
	BlockContext             context.Context
	BlockAnnouncementContext context.Context
	SendTrace                bool

	isSessionCopy bool
}

const (
	ImportDASegmentShardPrefix  = "is_"
	ImportDAJustificationPrefix = "ij_"
	AuditDABundlePrefix         = "ab_"
	AuditDASegmentShardPrefix   = "as_"
	AuditDAJustificationPrefix  = "aj_"
)

// NewStorageHub initializes a new LevelDB store
// Uses memory-based storage if useMemory is true, otherwise uses file-based storage
func NewStorageHub(path string, jamda types.JAMDA, telemetryClient types.TelemetryClient, nodeID uint16) (*StorageHub, error) {
	// Ensure the directory exists before opening LevelDB
	if err := os.MkdirAll(path, 0755); err != nil {
		return nil, fmt.Errorf("failed to create leveldb directory %s: %w", path, err)
	}

	log.Info(log.Node, "NewStorageHub: opening LevelDB", "path", path, "nodeID", nodeID)

	db, err := NewPersistenceStore(path)
	if err != nil {
		log.Warn(log.Node, "NewStorageHub: LevelDB open failed, using memory storage", "path", path, "err", err)
		// Fallback to memory storage if file fails
		db, err = NewMemoryPersistenceStore()
		if err != nil {
			return nil, err
		}
	} else {
		log.Info(log.Node, "NewStorageHub: LevelDB opened successfully", "path", path)
	}

	// Create JAM trie session with new backend
	session := NewJAMTrieSession(nil)

	// Create genesis tree for initial state
	genesisTree := NewUnifiedBinaryTree(Config{Profile: defaultUBTProfile})

	// Create shared UBT tree manager (will be shared with NewSession instances)
	ubtManager := NewUBTTreeManager()
	// Initialize with genesis tree - Reset stores tree and sets canonical root
	ubtManager.Reset(genesisTree)

	// Create shared service state manager (will be shared with NewSession instances)
	serviceState := NewServiceStateManager(db)

	s := &StorageHub{
		Persist: db,
		Shared: SharedManagers{
			UBT:     ubtManager,
			Service: serviceState,
		},
		Session: SessionContexts{
			JAM: session,
			UBT: NewUBTExecContext(ubtManager),
		},
		Witness: WitnessCache{
			Storageshard: make(map[common.Address]evmtypes.ContractStorage),
			Code:         make(map[common.Address][]byte),
			UBTProofs:    make(map[common.Address]evmtypes.UBTMultiproof),
		},
		Infra: Infrastructure{
			JAMDA:           jamda,
			TelemetryClient: telemetryClient,
			NodeID:          nodeID,
			LogChan:         make(chan LogMessage, 100),
		},
		keys: make(map[common.Hash]bool),
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

	// Get initial root from session
	initialRoot := session.Root()
	s.Root = initialRoot

	return s, nil
}

// ===== Root-First State Access Methods =====
//
// These methods provide root-based state access, replacing block-number-based
// or implicit CurrentUBT access. All state should be accessed by root hash.

// panicIfSessionCopy panics if this is a NewSession instance.
// Used to prevent session copies from modifying shared canonical state (canonicalRoot).
// Session copies CAN read from treeStore and set their own activeRoot.
func (s *StorageHub) panicIfSessionCopy(operation string) {
	s.mu.RLock()
	isSession := s.isSessionCopy
	s.mu.RUnlock()
	if isSession {
		panic(fmt.Sprintf("operation %q modifies canonical state - not allowed on NewSession", operation))
	}
}

// GetCanonicalRoot returns the state root of the canonical (committed) state.
// Delegates to UBTTreeManager.
func (s *StorageHub) GetCanonicalRoot() common.Hash {
	return s.Shared.UBT.GetCanonicalRoot()
}

// SetCanonicalRoot updates the canonical state root.
// This should only be called by CommitAsCanonical after accumulation.
// Delegates to UBTTreeManager.
func (s *StorageHub) SetCanonicalRoot(root common.Hash) error {
	s.panicIfSessionCopy("SetCanonicalRoot")
	return s.Shared.UBT.SetCanonicalRoot(root)
}

// GetTreeByRoot returns the UBT tree with the given state root.
// Returns (tree, true) if found, (nil, false) if not.
// Delegates to UBTTreeManager.
func (s *StorageHub) GetTreeByRoot(root common.Hash) (interface{}, bool) {
	return s.Shared.UBT.GetTree(root)
}

// GetTreeByRootTyped returns the typed UBT tree (for internal use).
// Delegates to UBTTreeManager.
func (s *StorageHub) GetTreeByRootTyped(root common.Hash) (*UnifiedBinaryTree, bool) {
	return s.Shared.UBT.GetTree(root)
}

// GetCanonicalTree returns the UBT tree at the canonical root.
// This is the replacement for accessing CurrentUBT directly.
// Returns interface{} to satisfy EVMJAMStorage interface.
// Delegates to UBTTreeManager.
func (s *StorageHub) GetCanonicalTree() interface{} {
	return s.Shared.UBT.GetCanonicalTree()
}

// GetCanonicalTreeTyped returns the typed UBT tree at the canonical root (internal use).
// Delegates to UBTTreeManager.
func (s *StorageHub) GetCanonicalTreeTyped() *UnifiedBinaryTree {
	return s.Shared.UBT.GetCanonicalTree()
}

// StoreTree adds a tree to the treeStore with its root hash.
// If a tree with this root already exists, this is a no-op.
// tree must be *UnifiedBinaryTree.
// Safe for session copies: writes are mutex-protected and don't affect canonical state.
// Delegates to UBTTreeManager.
func (s *StorageHub) StoreTree(tree interface{}) common.Hash {
	ubt, ok := tree.(*UnifiedBinaryTree)
	if !ok {
		log.Error(log.EVM, "StoreTree: tree is not *UnifiedBinaryTree")
		return common.Hash{}
	}
	return s.Shared.UBT.StoreTree(ubt)
}

// StoreTreeTyped adds a typed tree (for internal use).
// Safe for session copies: writes are mutex-protected and don't affect canonical state.
// Delegates to UBTTreeManager.
func (s *StorageHub) StoreTreeTyped(tree *UnifiedBinaryTree) common.Hash {
	return s.Shared.UBT.StoreTree(tree)
}

// StoreTreeWithRoot adds a tree to the treeStore with an explicit root.
// Use this when you've already computed the root hash.
// Safe for session copies: writes are mutex-protected and don't affect canonical state.
// Delegates to UBTTreeManager.
func (s *StorageHub) StoreTreeWithRoot(root common.Hash, tree *UnifiedBinaryTree) {
	s.Shared.UBT.StoreTreeWithRoot(root, tree)
}

// NewTreeFromRoot creates a new independent tree initialized from the state at the given root.
// Returns error if no tree exists at that root.
// Safe for session copies: this is a read operation that creates a new tree.
// Delegates to UBTTreeManager.
func (s *StorageHub) NewTreeFromRoot(root common.Hash) (interface{}, error) {
	return s.Shared.UBT.NewTree(root)
}

// NewTreeFromRootTyped creates a typed new tree (for internal use).
// Safe for session copies: this is a read operation that creates a new tree.
// Delegates to UBTTreeManager.
func (s *StorageHub) NewTreeFromRootTyped(root common.Hash) (*UnifiedBinaryTree, error) {
	return s.Shared.UBT.NewTree(root)
}

// DiscardTree removes a tree from the treeStore.
// Returns true if a tree was removed, false if no tree existed at that root.
// WARNING: Do not discard the canonical root or any root that may be needed for rebuilds.
// Delegates to UBTTreeManager.
func (s *StorageHub) DiscardTree(root common.Hash) bool {
	s.panicIfSessionCopy("DiscardTree")
	return s.Shared.UBT.DiscardTree(root)
}

// GetActiveRoot returns the currently active state root for execution.
// Returns the zero hash if no active root is set.
// NOTE: activeRoot is PER-INSTANCE, not shared. Each session has its own activeRoot.
// Delegates to UBTExecContext.
func (s *StorageHub) GetActiveRoot() common.Hash {
	return s.Session.UBT.GetActiveRoot()
}

// SetActiveRoot sets the state root to use for the current execution context.
// Returns error if no tree exists at that root.
// NOTE: activeRoot is PER-INSTANCE. You must call this on the same storage
// instance that the VM will use for reads (typically the StateDB's NewSession).
// Delegates to UBTExecContext.
func (s *StorageHub) SetActiveRoot(root common.Hash) error {
	return s.Session.UBT.SetActiveRoot(root)
}

// ClearActiveRoot resets the active root to zero (use canonical for reads).
// Delegates to UBTExecContext.
func (s *StorageHub) ClearActiveRoot() {
	s.Session.UBT.ClearActiveRoot()
}

// GetTreeStoreSize returns the number of trees in the store (for debugging/metrics).
// Delegates to UBTTreeManager.
func (s *StorageHub) GetTreeStoreSize() int {
	return s.Shared.UBT.GetTreeStoreSize()
}

// CommitAsCanonical sets the given root as the new canonical state.
// The tree at this root becomes the canonical state that all queries read from.
// Called when a bundle accumulates successfully.
// Delegates to UBTTreeManager.
func (s *StorageHub) CommitAsCanonical(root common.Hash) error {
	s.panicIfSessionCopy("CommitAsCanonical")
	return s.Shared.UBT.CommitAsCanonical(root)
}

// GetJAMDA returns the JAMDA interface instance
func (s *StorageHub) GetJAMDA() types.JAMDA {
	return s.Infra.JAMDA
}

// GetNodeID returns the node ID
func (s *StorageHub) GetNodeID() uint16 {
	return s.Infra.NodeID
}

// GetTelemetryClient returns the telemetry client for emitting events
func (store *StorageHub) GetTelemetryClient() types.TelemetryClient {
	return store.Infra.TelemetryClient
}

// SetTelemetryClient updates the telemetry client used for emitting events.
func (store *StorageHub) SetTelemetryClient(client types.TelemetryClient) {
	store.Infra.TelemetryClient = client
}

// ReadKV reads a value for a given key from the storage
func (store *StorageHub) ReadKV(key common.Hash) ([]byte, error) {
	return store.Persist.GetHash(key)
}

// WriteKV writes a key-value pair to the storage
func (store *StorageHub) WriteKV(key common.Hash, value []byte) error {
	return store.Persist.PutHash(key, value)
}

// DeleteK deletes a key from the storage
func (store *StorageHub) DeleteK(key common.Hash) error {
	return store.Persist.DeleteHash(key)
}

// CloseDB closes the LevelDB database
func (store *StorageHub) CloseDB() error {
	return store.Persist.Close()
}

// StoreUBTTransition stores a UBT tree state by its root hash.
// This method now uses treeStore (root-first model).
func (store *StorageHub) StoreUBTTransition(ubtRoot common.Hash, tree *UnifiedBinaryTree) error {
	if tree == nil {
		return fmt.Errorf("cannot store nil UBT tree")
	}

	store.StoreTreeWithRoot(ubtRoot, tree)
	return nil
}

// GetUBTTreeAtRoot retrieves a UBT tree state by its root hash.
// This method now uses treeStore (root-first model).
func (store *StorageHub) GetUBTTreeAtRoot(ubtRoot common.Hash) (interface{}, bool) {
	return store.GetTreeByRoot(ubtRoot)
}

// GetUBTNodeForBlockNumber maps a block number string to the corresponding UBT tree.
// Uses treeStore with canonicalRoot for "latest" (root-first model).
func (store *StorageHub) GetUBTNodeForBlockNumber(blockNumber string) (interface{}, bool) {
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
func (store *StorageHub) GetBalance(tree interface{}, address common.Address) (common.Hash, error) {
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
func (store *StorageHub) GetNonce(tree interface{}, address common.Address) (uint64, error) {
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

func (store *StorageHub) ReadRawKV(key []byte) ([]byte, bool, error) {
	return store.Persist.Get(key)
}

func (store *StorageHub) ReadRawKVWithPrefix(prefix []byte) ([][2][]byte, error) {
	return store.Persist.GetWithPrefix(prefix)
}

func (store *StorageHub) WriteRawKV(key []byte, value []byte) error {
	return store.Persist.Put(key, value)
}

func (store *StorageHub) DeleteRawK(key []byte) error {
	return store.Persist.Delete(key)
}

func TestTrace(host string) bool {
	return true
}

func (store *StorageHub) InitTracer(serviceName string) error {
	return nil
}

func (store *StorageHub) UpdateWorkPackageContext(ctx context.Context) {
	store.WorkPackageContext = ctx
}

func (store *StorageHub) UpdateBlockContext(ctx context.Context) {
	store.BlockContext = ctx
}

func (store *StorageHub) UpdateBlockAnnouncementContext(ctx context.Context) {
	store.BlockAnnouncementContext = ctx
}

// FetchJAMDASegments fetches DA payload using WorkPackageHash and segment indices
// This method retrieves segments from DA storage and combines them into payload
func (store *StorageHub) FetchJAMDASegments(workPackageHash common.Hash, indexStart uint16, indexEnd uint16, payloadLength uint32) (payload []byte, err error) {
	return store.Infra.JAMDA.FetchJAMDASegments(workPackageHash, indexStart, indexEnd, payloadLength)
}

func (store *StorageHub) CleanWorkPackageContext() {
	store.WorkPackageContext = context.Background()
}

func (store *StorageHub) CleanBlockContext() {
	store.BlockContext = context.Background()
}

func (store *StorageHub) CleanBlockAnnouncementContext() {
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
func (store *StorageHub) WorkReportSearch(h common.Hash) (*types.WorkReport, bool) {
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
func (store *StorageHub) StoreWorkReport(wr *types.WorkReport) error {
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
func (store *StorageHub) GetWarpSyncFragment(setID uint32) (types.WarpSyncFragment, error) {
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
func (store *StorageHub) StoreWarpSyncFragment(setID uint32, fragment types.WarpSyncFragment) error {
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
func (store *StorageHub) FetchBalance(address common.Address, txIndex uint32) ([32]byte, error) {
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
func (store *StorageHub) FetchNonce(address common.Address, txIndex uint32) ([32]byte, error) {
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
func (store *StorageHub) FetchCode(address common.Address, txIndex uint32) ([]byte, uint32, error) {
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
func (store *StorageHub) FetchCodeHash(address common.Address, txIndex uint32) ([32]byte, error) {
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
func (store *StorageHub) FetchStorage(address common.Address, storageKey [32]byte, txIndex uint32) ([32]byte, bool, error) {
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

// AppendUBTRead appends a UBT read to the log (no-op if logging disabled).
func (store *StorageHub) AppendUBTRead(read types.UBTRead) {
	store.Session.UBT.AppendRead(read)
}

func (store *StorageHub) GetUBTReadLog() []types.UBTRead {
	return store.Session.UBT.GetReadLog()
}

func (store *StorageHub) ClearUBTReadLog() {
	store.Session.UBT.ClearReadLog()
}

// SetUBTReadLogEnabled enables/disables UBT read logging.
// Disabled during refine (fast path), enabled for witness generation.
func (store *StorageHub) SetUBTReadLogEnabled(enabled bool) {
	store.Session.UBT.SetReadLogEnabled(enabled)
	log.Debug(log.EVM, "SetUBTReadLogEnabled", "enabled", enabled)
}

func (store *StorageHub) IsUBTReadLogEnabled() bool {
	return store.Session.UBT.IsReadLogEnabled()
}

// GetActiveUBTRoot returns the root hash of the currently active tree.
// Priority: pinnedTree > activeRoot > canonical.
func (store *StorageHub) GetActiveUBTRoot() common.Hash {
	tree := store.GetActiveTreeTyped()
	if tree == nil {
		return common.Hash{}
	}
	root := tree.RootHash()
	return common.BytesToHash(root[:])
}

// PinToStateRoot pins execution to a specific historical state root.
// Returns error if the state root is not available in treeStore.
//
// ROOT-FIRST ONLY: This method now only checks treeStore.
// All trees must be stored via StoreTreeWithRoot or ApplyWritesToTree.
// Delegates to UBTExecContext.
func (store *StorageHub) PinToStateRoot(root common.Hash) error {
	return store.Session.UBT.PinToStateRoot(root)
}

// UnpinState releases the pinned state and returns to normal operation.
// Delegates to UBTExecContext.
func (store *StorageHub) UnpinState() {
	store.Session.UBT.UnpinState()
}

func (store *StorageHub) ResetTrie() {
	store.panicIfSessionCopy("ResetTrie")

	// Create new genesis tree and reset the manager
	genesisTree := NewUnifiedBinaryTree(Config{Profile: defaultUBTProfile})
	store.Shared.UBT.Reset(genesisTree)

	// Reset per-instance UBT context
	store.Session.UBT = NewUBTExecContext(store.Shared.UBT)

	// Reset other state
	store.Session.JAM = NewJAMTrieSession(nil)

	store.mu.Lock()
	store.keys = make(map[common.Hash]bool)
	store.Root = store.Session.JAM.Root()
	store.mu.Unlock()
}

// BuildUBTWitness builds dual UBT witnesses and stores the post-state tree.
// Uses GetActiveTree() to support root-first parallel bundle building.
func (store *StorageHub) BuildUBTWitness(contractWitnessBlob []byte) ([]byte, []byte, error) {
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
func (store *StorageHub) ComputeBlockAccessListHash(preWitness []byte, postWitness []byte) (common.Hash, uint32, uint32, error) {
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

// Multi-Rollup Support: Delegates to ServiceStateManager

// GetUBTNodeForServiceBlock retrieves the UBT tree for a specific service's block
// Uses treeStore for root-first state management.
func (store *StorageHub) GetUBTNodeForServiceBlock(serviceID uint32, blockNumber string) (interface{}, bool) {
	// For "latest"/"pending" queries, return canonical tree
	if blockNumber == "latest" || blockNumber == "pending" {
		tree := store.GetCanonicalTree()
		if tree != nil {
			return tree, true
		}
	}

	// Get latest block for this service
	latestBlock, exists := store.Shared.Service.GetLatestBlock(serviceID)
	if !exists {
		log.Info(log.EVM, "GetUBTNodeForServiceBlock: no indexed blocks",
			"serviceID", serviceID,
			"blockNumber", blockNumber)
		return nil, false
	}

	// Parse block number
	blockNum, err := store.Shared.Service.parseBlockNumber(blockNumber, latestBlock)
	if err != nil {
		return nil, false
	}

	// Lookup UBT root
	ubtRoot, ok := store.Shared.Service.GetUBTRootForBlock(serviceID, blockNum)
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

// StoreServiceBlock persists an EVM block for a specific service.
// Delegates to ServiceStateManager.
func (store *StorageHub) StoreServiceBlock(serviceID uint32, blockIface interface{}, jamStateRoot common.Hash, jamSlot uint32) error {
	block, ok := blockIface.(*evmtypes.EvmBlockPayload)
	if !ok {
		return fmt.Errorf("block must be *evmtypes.EvmBlockPayload")
	}
	return store.Shared.Service.StoreServiceBlock(serviceID, block, jamStateRoot, jamSlot)
}

// GetServiceBlock retrieves an EVM block by service ID and block number.
// Delegates to ServiceStateManager.
func (store *StorageHub) GetServiceBlock(serviceID uint32, blockNumber string) (interface{}, error) {
	return store.Shared.Service.GetServiceBlock(serviceID, blockNumber)
}

// GetTransactionByHash finds a transaction in a service's block history.
// Delegates to ServiceStateManager.
func (store *StorageHub) GetTransactionByHash(serviceID uint32, txHash common.Hash) (*types.Transaction, *types.BlockMetadata, error) {
	return store.Shared.Service.GetTransactionByHash(serviceID, txHash)
}

// GetTxLocation returns the block number and tx index for a transaction hash.
// Delegates to ServiceStateManager.
func (store *StorageHub) GetTxLocation(serviceID uint32, txHash common.Hash) (blockNumber uint32, txIndex uint32, found bool) {
	return store.Shared.Service.GetTxLocation(serviceID, txHash)
}

// GetTransactionReceipt retrieves a full transaction receipt from stored block data.
// Delegates to ServiceStateManager.
func (store *StorageHub) GetTransactionReceipt(serviceID uint32, txHash common.Hash) (interface{}, error) {
	return store.Shared.Service.GetTransactionReceipt(serviceID, txHash)
}

// GetBlockByNumber retrieves full EVM block by service ID and block number.
// Delegates to ServiceStateManager.
func (store *StorageHub) GetBlockByNumber(serviceID uint32, blockNumber string) (*types.EVMBlock, error) {
	return store.Shared.Service.GetBlockByNumber(serviceID, blockNumber)
}

// GetBlockByHash retrieves full EVM block by service ID and block hash.
// Delegates to ServiceStateManager.
func (store *StorageHub) GetBlockByHash(serviceID uint32, blockHash common.Hash) (*types.EVMBlock, error) {
	return store.Shared.Service.GetBlockByHash(serviceID, blockHash)
}

// FinalizeEVMBlock is a helper to finalize an EVM block after accumulation
// This stores the block data and associates it with JAM state
// Call this after accumulate completes for an EVM service
func (store *StorageHub) FinalizeEVMBlock(
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
func (store *StorageHub) InitializeEVMGenesis(
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
func (store *StorageHub) LoadBlockByHeight(height uint64) (*evmtypes.EvmBlockPayload, error) {
	// Use serviceID 1 as default (primary EVM service)
	const defaultServiceID = uint32(1)

	blockKey := fmt.Sprintf("sblock_%d_%d", defaultServiceID, height)
	blockBytes, found, err := store.Persist.Get([]byte(blockKey))
	if err != nil {
		return nil, fmt.Errorf("block %d read error: %w", height, err)
	}
	if !found {
		return nil, fmt.Errorf("block %d not found", height)
	}

	var block evmtypes.EvmBlockPayload
	if err := json.Unmarshal(blockBytes, &block); err != nil {
		return nil, fmt.Errorf("failed to unmarshal block %d: %w", height, err)
	}

	return &block, nil
}
