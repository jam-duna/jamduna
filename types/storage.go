package types

import (
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/colorfulnotion/jam/common"
)

// Telemetry-related types moved from telemetry package to avoid circular dependencies

// BlockOutline represents the outline of a block
type BlockOutline struct {
	SizeInBytes          uint32      // Size in bytes
	HeaderHash           common.Hash // Header Hash
	NumTickets           uint32      // Number of tickets
	NumPreimages         uint32      // Number of preimages
	PreimagesSizeInBytes uint32      // Total size of preimages in bytes
	NumGuarantees        uint32      // Number of guarantees
	NumAssurances        uint32      // Number of assurances
	NumDisputeVerdicts   uint32      // Number of dispute verdicts
}

// WorkItemOutline represents the outline of a work item
type WorkItemOutline struct {
	ServiceID          uint32   // Service ID
	PayloadSize        uint32   // Payload size
	RefineGasLimit     uint64   // Refine gas limit
	AccumulateGasLimit uint64   // Accumulate gas limit
	ExtrinsicsLength   uint32   // Sum of extrinsic lengths
	ImportSpecs        [][]byte // Import specifications (encoded)
	NumExportedSegs    uint16   // Number of exported segments
}

// WorkPackageOutline represents the outline of a work package
type WorkPackageOutline struct {
	SizeInBytes      uint32            // Work-package size in bytes, excluding extrinsic data
	WorkPackageHash  common.Hash       // Work-Package Hash
	AnchorHash       common.Hash       // Header Hash (Anchor)
	LookupAnchorSlot uint32            // Slot (Lookup anchor slot)
	Prerequisites    []common.Hash     // Prerequisites
	WorkItems        []WorkItemOutline // Work items
}

// WorkReportOutline represents the outline of a work report
type WorkReportOutline struct {
	WorkReportHash common.Hash // Work-Report Hash
	BundleSize     uint32      // Bundle size in bytes
	ErasureRoot    common.Hash // Erasure-Root
	SegmentsRoot   common.Hash // Segments-Root
}

// GuaranteeOutline represents the outline of a guarantee
type GuaranteeOutline struct {
	WorkReportHash common.Hash // Work-Report Hash
	Slot           uint32      // Slot
	Guarantors     []uint16    // Validator indices of guarantors
}

// IsAuthorizedCost represents the cost of authorization
type IsAuthorizedCost struct {
	TotalGasUsed      uint64 // Total gas used
	TotalTimeNs       uint64 // Total elapsed wall-clock time in nanoseconds
	LoadCompileTimeNs uint64 // Time taken to load and compile the code, in nanoseconds
	HostCallsGasUsed  uint64 // Gas used by host calls
	HostCallsTimeNs   uint64 // Time spent in host calls in nanoseconds
}

// RefineCost represents the cost of refinement
type RefineCost struct {
	TotalGasUsed           uint64 // Total gas used
	TotalTimeNs            uint64 // Total elapsed wall-clock time in nanoseconds
	LoadCompileTimeNs      uint64 // Time taken to load and compile the code, in nanoseconds
	HistoricalLookupGas    uint64 // Gas for historical_lookup calls
	HistoricalLookupTimeNs uint64 // Time for historical_lookup calls
	MachineExpungeGas      uint64 // Gas for machine/expunge calls
	MachineExpungeTimeNs   uint64 // Time for machine/expunge calls
	PeekPokeGas            uint64 // Gas for peek/poke/pages calls
	PeekPokeTimeNs         uint64 // Time for peek/poke/pages calls
	InvokeGas              uint64 // Gas for invoke calls
	InvokeTimeNs           uint64 // Time for invoke calls
	OtherGas               uint64 // Gas for other host calls
	OtherTimeNs            uint64 // Time for other host calls
}

// AccumulateCost represents the cost of accumulation
type AccumulateCost struct {
	NumAccumulateCalls    uint32 // Number of accumulate calls
	NumTransfersProcessed uint32 // Number of transfers processed
	NumItemsAccumulated   uint32 // Number of items accumulated
	TotalGasUsed          uint64 // Total gas used
	TotalTimeNs           uint64 // Total elapsed wall-clock time in nanoseconds
	LoadCompileTimeNs     uint64 // Time taken to load and compile the code, in nanoseconds
	ReadWriteGas          uint64 // Gas for read/write calls
	ReadWriteTimeNs       uint64 // Time for read/write calls in nanoseconds
	LookupGas             uint64 // Gas for lookup calls
	LookupTimeNs          uint64 // Time for lookup calls in nanoseconds
	QueryGas              uint64 // Gas for query/solicit/forget/provide calls
	QueryTimeNs           uint64 // Time for query/solicit/forget/provide calls in nanoseconds
	InfoGas               uint64 // Gas for info/new/upgrade/eject calls
	InfoTimeNs            uint64 // Time for info/new/upgrade/eject calls in nanoseconds
	TransferGas           uint64 // Gas for transfer calls
	TransferTimeNs        uint64 // Time for transfer calls in nanoseconds
	TransferProcessingGas uint64 // Total gas charged for transfer processing by destination services
	OtherGas              uint64 // Gas for other host calls
	OtherTimeNs           uint64 // Time for other host calls in nanoseconds
}

// ServiceAccumulateCost represents service-specific accumulation cost
type ServiceAccumulateCost struct {
	ServiceID uint32
	Cost      AccumulateCost
}

// SegmentShardRequest represents a segment shard request
type SegmentShardRequest struct {
	ImportSegmentID uint16 // Index in overall list of work-package imports, or 2^15 plus index of a proven page
	ShardIndex      uint16 // Shard index
}

// NodeInfo represents node information for telemetry
type NodeInfo struct {
	JAMParameters     []byte      // JAM Parameters as returned by the fetch host call
	GenesisHeaderHash common.Hash // Genesis header hash
	PeerID            [32]byte    // Ed25519 public key
	PeerAddress       [16]byte    // IPv6 address
	PeerPort          uint16      // Port
	NodeFlags         uint32      // Bitmask of node flags (bit 0: PVM recompiler=1, interpreter=0)
	NodeName          string      // Name of node implementation (max 32 chars)
	NodeVersion       string      // Version of node implementation (max 32 chars)
	GrayPaperVersion  string      // Gray Paper version implemented (max 16 chars)
	Note              string      // Freeform note (max 512 chars)
}

// KeyVal represents a key-value pair for storage operations
type KeyVal struct {
	Key   [31]byte `json:"key"`
	Value []byte   `json:"value"`
}

// VerkleRead tracks a single Verkle tree read operation
// Used to build witnesses for EVM execution validation
type VerkleRead struct {
	VerkleKey  common.Hash    // Full 32-byte verkle key (31-byte stem + 1-byte suffix)
	Address    common.Address // Address being read (for metadata extraction)
	KeyType    uint8          // 0=BasicData, 1=CodeHash, 2=CodeChunk, 3=Storage
	Extra      uint64         // ChunkID for code chunks
	StorageKey common.Hash    // Full storage key for storage reads
	TxIndex    uint32         // Transaction index within work package (0=pre-exec, 1..n=txs)
}

func (kv *KeyVal) UnmarshalJSON(data []byte) error {
	var raw struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	keyBytes := common.FromHex(raw.Key)
	if len(keyBytes) > 31 {
		return fmt.Errorf("key too long: got %d bytes, expected at most 31", len(keyBytes))
	}
	copy(kv.Key[:], keyBytes)
	kv.Value = common.FromHex(raw.Value)
	return nil
}

func (kv KeyVal) MarshalJSON() ([]byte, error) {
	end := len(kv.Key)
	for end > 0 && kv.Key[end-1] == 0 {
		end--
	}
	keyHex := "0x"
	if end > 0 {
		keyHex += hex.EncodeToString(kv.Key[:end])
	}
	valueHex := "0x"
	if len(kv.Value) > 0 {
		valueHex += hex.EncodeToString(kv.Value)
	}
	return json.Marshal(struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}{
		Key:   keyHex,
		Value: valueHex,
	})
}

// JAMDA represents the JAM Data Availability interface
type JAMDA interface {
	FetchJAMDASegments(workPackageHash common.Hash, indexStart uint16, indexEnd uint16, payloadLength uint32) (payload []byte, err error)
	StoreBundleSpecSegments(as *AvailabilitySpecifier, d AvailabilitySpecifierDerivation, b WorkPackageBundle, segments [][]byte)
	BuildBundleFromWPQueueItem(wpQueueItem *WPQueueItem) (bundle WorkPackageBundle, segmentRootLookup SegmentRootLookup, err error)
}

// TelemetryClient represents the telemetry client interface
type TelemetryClient interface {
	// Core telemetry methods
	GetEventID(...interface{}) uint64
	Close() error
	Connect(nodeInfo NodeInfo) error

	// Block authoring and importing
	Authoring(slot uint32, parentHeaderHash common.Hash)
	AuthoringFailed(eventID uint64, reason string)
	Authored(eventID uint64, blockOutline BlockOutline)
	Importing(slot uint32, blockOutline BlockOutline)
	BlockVerificationFailed(eventID uint64, reason string)
	BlockVerified(eventID uint64)
	BlockExecutionFailed(eventID uint64, reason string)
	BlockExecuted(eventID uint64, services []ServiceAccumulateCost)
	AccumulateResultAvailable(slot uint32, headerHash common.Hash)

	// Work package and guarantees
	WorkPackageSubmission(eventID uint64, peerID [32]byte, workPackageOutline WorkPackageOutline)
	WorkPackageBeingShared(peerID [32]byte)
	WorkPackageFailed(eventID uint64, reason string)
	DuplicateWorkPackage(eventID uint64, coreIndex uint16, workPackageHash common.Hash)
	WorkPackageReceived(eventID uint64, workPackageOutline WorkPackageOutline)
	Authorized(eventID uint64, cost IsAuthorizedCost)
	ExtrinsicDataReceived(eventID uint64)
	ImportsReceived(eventID uint64)
	SharingWorkPackage(eventID uint64, peerID [32]byte, workPackageOutline WorkPackageOutline)
	WorkPackageSharingFailed(eventID uint64, peerID [32]byte, reason string)
	BundleSent(eventID uint64, peerID [32]byte)
	Refined(eventID uint64, refineCosts []RefineCost)
	WorkReportBuilt(eventID uint64, workReportOutline WorkReportOutline)
	WorkReportSignatureSent(eventID uint64)
	WorkReportSignatureReceived(eventID uint64, peerID [32]byte, workReportHash common.Hash)

	// Guarantees
	GuaranteeBuilt(eventID uint64, guaranteeOutline GuaranteeOutline)
	SendingGuarantee(eventID uint64, peerID [32]byte)
	GuaranteeSendFailed(eventID uint64, reason string)
	GuaranteeSent(eventID uint64)
	GuaranteesDistributed(eventID uint64)
	ReceivingGuarantee(peerID [32]byte)
	GuaranteeReceiveFailed(eventID uint64, reason string)
	GuaranteeReceived(eventID uint64, guaranteeOutline GuaranteeOutline)
	GuaranteeDiscarded(guaranteeOutline GuaranteeOutline, discardReason byte)

	// Assurances
	SendingShardRequest(peerID [32]byte, erasureRoot common.Hash, shardIndex uint16)
	ReceivingShardRequest(peerID [32]byte)
	ShardRequestFailed(eventID uint64, reason string)
	ShardRequestSent(eventID uint64)
	ShardRequestReceived(eventID uint64, erasureRoot common.Hash, shardIndex uint16)
	ShardsTransferred(eventID uint64)
	DistributingAssurance(headerHash common.Hash, availabilityBitfield []byte)
	AssuranceSendFailed(eventID uint64, peerID [32]byte, reason string)
	AssuranceSent(eventID uint64, peerID [32]byte)
	AssuranceDistributed(eventID uint64)
	AssuranceReceiveFailed(peerID [32]byte, reason string)
	AssuranceReceived(peerID [32]byte, headerHash common.Hash)
	ContextAvailable(workReportHash common.Hash, coreIndex uint16, slot uint32, spec AvailabilitySpecifier)
	AssuranceProvided(assurance Assurance)

	// Block announcements and requests
	BlockAnnouncementStreamOpened(peerID [32]byte, connectionSide byte)
	BlockAnnouncementStreamClosed(peerID [32]byte, connectionSide byte, reason string)
	BlockAnnounced(peerID [32]byte, connectionSide byte, slot uint32, headerHash common.Hash)
	BlockAnnouncementMalformed(peerID [32]byte, reason string)
	SendingBlockRequest(peerID [32]byte, headerHash common.Hash, direction byte, maxBlocks uint32)
	ReceivingBlockRequest(peerID [32]byte)
	BlockRequestFailed(eventID uint64, reason string)
	BlockRequestSent(eventID uint64)
	BlockRequestReceived(eventID uint64, headerHash common.Hash, direction byte, maxBlocks uint32)
	BlockTransferred(eventID uint64, slot uint32, blockOutline BlockOutline, isLast bool)

	// Bundle recovery
	SendingBundleShardRequest(eventID uint64, peerID [32]byte, shardIndex uint16)
	ReceivingBundleShardRequest(peerID [32]byte)
	BundleShardRequestFailed(eventID uint64, reason string)
	BundleShardRequestSent(eventID uint64)
	BundleShardRequestReceived(eventID uint64, erasureRoot common.Hash, shardIndex uint16)
	BundleShardTransferred(eventID uint64)
	ReconstructingBundle(eventID uint64, isTrivial bool)
	BundleReconstructed(eventID uint64)
	SendingBundleRequest(eventID uint64, peerID [32]byte)
	ReceivingBundleRequest(peerID [32]byte)
	BundleRequestFailed(eventID uint64, reason string)
	BundleRequestSent(eventID uint64)
	BundleRequestReceived(eventID uint64, erasureRoot common.Hash)
	BundleTransferred(eventID uint64)

	// Preimages
	PreimageAnnouncementFailed(peerID [32]byte, connectionSide byte, reason string)
	PreimageAnnounced(peerID [32]byte, connectionSide byte, serviceID uint32, hash common.Hash, preimageLength uint32)
	AnnouncedPreimageForgotten(serviceID uint32, hash common.Hash, preimageLength uint32, forgetReason byte)
	SendingPreimageRequest(peerID [32]byte, hash common.Hash)
	ReceivingPreimageRequest(peerID [32]byte)
	PreimageRequestFailed(eventID uint64, reason string)
	PreimageRequestSent(eventID uint64)
	PreimageRequestReceived(eventID uint64, hash common.Hash)
	PreimageTransferred(eventID uint64, preimageLength uint32)
	PreimageDiscarded(hash common.Hash, preimageLength uint32, discardReason byte)

	// Safrole (ticket generation)
	GeneratingTickets(epochIndex uint32)
	TicketGenerationFailed(eventID uint64, reason string)
	TicketsGenerated(eventID uint64, vrfOutputs [][32]byte)
	TicketTransferFailed(peerID [32]byte, connectionSide byte, wasCE132 bool, reason string)
	TicketTransferred(peerID [32]byte, connectionSide byte, wasCE132 bool, epochIndex uint32, attemptNumber byte, vrfOutput [32]byte)

	// Segments
	WorkPackageHashMapped(eventID uint64, workPackageHash common.Hash, segmentsRoot common.Hash)
	SegmentsRootMapped(eventID uint64, segmentsRoot common.Hash, erasureRoot common.Hash)
	SendingSegmentShardRequest(eventID uint64, peerID [32]byte, usingCE140 bool, requests []SegmentShardRequest)
	ReceivingSegmentShardRequest(peerID [32]byte, usingCE140 bool)
	SegmentShardRequestFailed(eventID uint64, reason string)
	SegmentShardRequestSent(eventID uint64)
	SegmentShardRequestReceived(eventID uint64, numSegmentShards uint16)
	SegmentShardsTransferred(eventID uint64)
	ReconstructingSegments(eventID uint64, segmentIDs []uint16, isTrivial bool)
	SegmentReconstructionFailed(eventID uint64, reason string)
	SegmentsReconstructed(eventID uint64)
	SegmentVerificationFailed(eventID uint64, failedIndices []uint16, reason string)
	SegmentsVerified(eventID uint64, verifiedIndices []uint16)
	SendingSegmentRequest(eventID uint64, peerID [32]byte, segmentIndices []uint16)
	ReceivingSegmentRequest(peerID [32]byte)
	SegmentRequestFailed(eventID uint64, reason string)
	SegmentRequestSent(eventID uint64)
	SegmentRequestReceived(eventID uint64, numSegments uint16)
	SegmentsTransferred(eventID uint64)

	// Status and networking
	DroppedEvents(lastDroppedTimestamp uint64, numDropped uint64)
	Status(totalPeers, validatorPeers, blockAnnouncementStreamPeers uint32, guaranteesPerCore []byte, shardsInAvailabilityStore uint32, shardsSize uint64, preimagesInPool, preimagesSize uint32)
	BestBlockChanged(slot uint32, headerHash common.Hash)
	FinalizedBlockChanged(slot uint32, headerHash common.Hash)
	SyncStatusChanged(isSynced bool)
	ConnectionRefused(peerAddress [16]byte, peerPort uint16)
	ConnectingIn(peerAddress [16]byte, peerPort uint16)
	ConnectInFailed(eventID uint64, reason string)
	ConnectedIn(eventID uint64, peerID [32]byte)
	ConnectingOut(peerID [32]byte, peerAddress [16]byte, peerPort uint16)
	ConnectOutFailed(eventID uint64, reason string)
	ConnectedOut(eventID uint64)
	Disconnected(peerID [32]byte, connectionSide *byte, reason string)
	PeerMisbehaved(peerID [32]byte, reason string)
}

// JAMStorage defines the complete interface for JAM blockchain storage
// This includes state management, service operations, data availability, and block storage
type JAMStorage interface {
	Insert(key31 []byte, value []byte)
	Delete(key []byte) error
	Get(key []byte) ([]byte, bool, error)
	Trace(key []byte) ([][]byte, error)
	Flush() (common.Hash, error)

	// State Operations - High-level state management for C1-C16
	SetStates(values [16][]byte)
	GetStates() ([16][]byte, error)
	GetAllKeyValues() []KeyVal
	// TODO: make this a session
	GetRoot() common.Hash
	OverlayRoot() (common.Hash, error) // Compute root from staged overlay without committing (mirrors Rust Session::finish)
	SetRoot(root common.Hash) error

	// Service Operations - per-service account data management
	DeleteService(s uint32) error
	SetService(s uint32, v []byte) error
	GetService(s uint32) ([]byte, bool, error)
	SetPreImageLookup(s uint32, blob_hash common.Hash, blob_len uint32, time_slots []uint32) error
	GetPreImageLookup(s uint32, blob_hash common.Hash, blob_len uint32) ([]uint32, bool, error)
	DeletePreImageLookup(s uint32, blob_hash common.Hash, blob_len uint32) error
	SetServiceStorage(s uint32, k []byte, storageValue []byte) error
	GetServiceStorage(s uint32, k []byte) ([]byte, bool, error)
	GetServiceStorageWithProof(s uint32, k []byte) ([]byte, [][]byte, common.Hash, bool, error)
	DeleteServiceStorage(s uint32, k []byte) error
	SetPreImageBlob(s uint32, blob []byte) error
	GetPreImageBlob(s uint32, blobHash common.Hash) (value []byte, ok bool, err error)
	DeletePreImageBlob(s uint32, blobHash common.Hash) error

	// Core KV Operations - Low-level key-value access
	ReadRawKV(key []byte) (value []byte, found bool, err error)
	ReadRawKVWithPrefix(prefix []byte) ([][2][]byte, error)
	WriteRawKV(key []byte, val []byte) error
	ReadKV(key common.Hash) ([]byte, error)

	// Block Storage Operations
	// These methods handle block persistence and retrieval with multiple indices
	StoreBlock(blk *Block, id uint16, slotTimestamp uint64) error
	StoreFinalizedBlock(blk *Block) error
	GetFinalizedBlock() (*Block, error)
	GetFinalizedBlockInternal() (*Block, bool, error)
	GetBlockByHeader(headerHash common.Hash) (*SBlock, error)
	GetBlockBySlot(slot uint32) (*SBlock, error)
	GetChildBlocks(parentHeaderHash common.Hash) ([][2][]byte, error)

	StoreCatchupMassage(round uint64, setId uint32, data []byte) error
	GetCatchupMassage(round uint64, setId uint32) ([]byte, bool, error)

	// Data Availability - Guarantor Operations
	// Guarantors create and distribute erasure-coded shards
	StoreBundleSpecSegments(
		erasureRoot common.Hash,
		exportedSegmentRoot common.Hash,
		bChunks []DistributeECChunk,
		sChunks []DistributeECChunk,
		bClubs []common.Hash,
		sClubs []common.Hash,
		bundle []byte,
		encodedSegments []byte,
	) error
	GetGuarantorMetadata(erasureRoot common.Hash) (
		bClubs []common.Hash,
		sClubs []common.Hash,
		bECChunks []DistributeECChunk,
		sECChunksArray []DistributeECChunk,
		err error,
	)
	GetFullShard(erasureRoot common.Hash, shardIndex uint16) (
		bundleShard []byte,
		segmentShards []byte,
		justification []byte,
		ok bool,
		err error,
	)
	GetWarpSyncFragment(setID uint32) (WarpSyncFragment, error)
	StoreWarpSyncFragment(setID uint32, fragment WarpSyncFragment) error

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
	GetBundleByErasureRoot(erasureRoot common.Hash) (WorkPackageBundle, bool)
	GetSegmentsBySegmentRoot(segmentRoot common.Hash) ([][]byte, bool)
	FetchJAMDASegments(workPackageHash common.Hash, indexStart uint16, indexEnd uint16, payloadLength uint32) (payload []byte, err error)

	// Work Report Operations
	StoreWorkReport(wr *WorkReport) error
	WorkReportSearch(requestedHash common.Hash) (*WorkReport, bool)

	// Node Identity and Telemetry
	SetTelemetryClient(client TelemetryClient)
	GetTelemetryClient() TelemetryClient
	GetJAMDA() JAMDA
	GetNodeID() uint16

	// Witness Cache Operations (Phase 4+)
	// These methods manage the witness cache for EVM execution
	InitWitnessCache()
	SetContractStorage(address common.Address, storage interface{})
	GetContractStorage(address common.Address) (interface{}, bool)
	SetCode(address common.Address, code []byte)
	GetCode(address common.Address) ([]byte, bool)
	ReadStorageFromCache(contractAddress common.Address, storageKey common.Hash) (common.Hash, bool)
	ClearWitnessCache()

	// EVM State Access Methods (Verkle-based)
	// These methods provide clean interface for EVM state reads without exposing verkle internals
	// All verkle key computation and tree access is encapsulated in the implementation
	// txIndex parameter tracks which transaction in work package caused this access (for BAL construction)
	FetchBalance(address common.Address, txIndex uint32) ([32]byte, error)
	FetchNonce(address common.Address, txIndex uint32) ([32]byte, error)
	FetchCode(address common.Address, txIndex uint32) ([]byte, uint32, error) // returns (code, codeSize, error)
	FetchCodeHash(address common.Address, txIndex uint32) ([32]byte, error)
	FetchStorage(address common.Address, storageKey [32]byte, txIndex uint32) ([32]byte, bool, error)

	// Verkle Read Log Management
	// These methods manage the read log that tracks all verkle reads during execution
	AppendVerkleRead(read VerkleRead)
	GetVerkleReadLog() []VerkleRead
	ClearVerkleReadLog()

	// Verkle Witness Building
	// BuildVerkleWitness builds a dual-proof verkle witness and stores the post-state tree
	// All verkle tree access is encapsulated in the implementation
	// Returns: witnessBytes, error
	BuildVerkleWitness(contractWitnessBlob []byte) (witnessBytes []byte, err error)

	// Contract Write Application
	// ApplyContractWrites applies contract writes (code, storage, balance, nonce) to the current Verkle tree
	// This is called after work items are processed to update the state
	ApplyContractWrites(blob []byte) error

	// Block Access List (BAL) Computation
	// ComputeBlockAccessListHash builds BAL from verkle witness and returns hash + statistics
	// Used by both builder (to compute hash for payload) and guarantor (to verify builder's hash)
	// Returns: hash, accountCount, totalChanges, error
	ComputeBlockAccessListHash(verkleWitness []byte) (common.Hash, uint32, uint32, error)

	// Lifecycle
	Close() error
}
