package fuzz

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/jam-duna/jamduna/common"
	"github.com/jam-duna/jamduna/pvm"
	"github.com/jam-duna/jamduna/storage"

	// "github.com/jam-duna/jamduna/refine"
	"github.com/jam-duna/jamduna/statedb"
	"github.com/jam-duna/jamduna/types"
)

// StateTransition represents a complete state transition for debugging
type StateTransition struct {
	PreState  statedb.StateSnapshotRaw `json:"pre_state"`
	Block     types.Block              `json:"block"`
	PostState statedb.StateSnapshotRaw `json:"post_state"`
}

// DefaultPruneDepth is the number of slots of state history to keep in memory.
const DefaultPruneDepth = 64

// PruneInterval is how often (in slots) to run the pruning process.
const PruneInterval = 32

type stateEntry struct {
	stateRoot common.Hash
	slot      uint32
}

type exportEntry struct {
	segmentsHash common.Hash
	slot         uint32
}

type Target struct {
	listener       net.Listener
	socketPath     string
	targetInfo     PeerInfo
	store          *storage.StorageHub         // Storage backend for the state database.
	stateDB        *statedb.StateDB            // This can be used to manage state transitions.
	stateDBMap     map[common.Hash]stateEntry  // [headerHash] -> {stateRoot, slot} for fork detection cache
	pvmBackend     string                      // Backend to use for state transitions, e.g., "Interpreter" or "Compiler".
	exportsLookup  map[common.Hash]exportEntry // [workPackageHash or exportsRoot] -> {segmentsHash, slot}
	exportsData    map[common.Hash][][]byte    // [segmentsHash] -> exportedSegments
	debugState     bool                        // Enable detailed state debugging output
	dumpStf        bool                        // Dump state transition files for debugging
	dumpLocation   string                      // Directory path for STF dump files
	lastPrunedSlot uint32                      // Last slot where pruning was performed
	sessionID      string                      // Unique session ID for this socket connection (persists for lifetime of target)
}

// NewTarget creates a new target instance, armed with a specific test case.
func NewTarget(socketPath string, targetInfo PeerInfo, pvmBackend string, debugState bool, dumpStf bool, dumpLocation string) *Target {
	levelDBPath := fmt.Sprintf("/tmp/target_%d", time.Now().Unix())
	store, err := storage.NewStorageHub(levelDBPath, nil, nil, 0)
	if err != nil {
		log.Fatalf("Failed to create state DB storage: %v", err)
	}
	if strings.ToLower(pvmBackend) != pvm.BackendInterpreter && strings.ToLower(pvmBackend) != pvm.BackendCompiler {
		pvmBackend = pvm.BackendInterpreter
		fmt.Printf("Invalid PVM backend specified. Defaulting to Interpreter\n")
	} else if runtime.GOOS != "linux" && strings.ToLower(pvmBackend) != pvm.BackendInterpreter {
		pvmBackend = pvm.BackendInterpreter
		fmt.Printf("Defaulting to Interpreter\n")
	}

	sessionID := fmt.Sprintf("%d", time.Now().UnixNano())

	return &Target{
		socketPath:     socketPath,
		targetInfo:     targetInfo,
		store:          store,
		stateDB:        nil,
		stateDBMap:     make(map[common.Hash]stateEntry),
		pvmBackend:     pvmBackend,
		exportsLookup:  make(map[common.Hash]exportEntry),
		exportsData:    make(map[common.Hash][][]byte),
		debugState:     debugState,
		dumpStf:        dumpStf,
		dumpLocation:   dumpLocation,
		lastPrunedSlot: 0,
		sessionID:      sessionID,
	}
}

// Start begins listening on the Unix socket for incoming fuzzer connections.
func (t *Target) Start() error {
	if err := os.RemoveAll(t.socketPath); err != nil {
		return fmt.Errorf("%sfailed to remove old socket file: %w%s", common.ColorRed, err, common.ColorReset)
	}

	listener, err := net.Listen("unix", t.socketPath)
	if err != nil {
		return fmt.Errorf("%sfailed to listen on socket %s: %w%s", common.ColorRed, t.socketPath, err, common.ColorReset)
	}
	t.listener = listener
	log.Printf("Target listening on %s\n", t.socketPath)

	for {
		conn, err := t.listener.Accept()
		if err != nil {
			return nil // Listener was closed.
		}
		log.Println("Fuzzer connected.")
		go t.handleConnection(conn)
	}
}

// Stop closes the listener and cleans up resources.
func (t *Target) Stop() {
	if t.listener != nil {
		t.listener.Close()
	}
	os.RemoveAll(t.socketPath)
	log.Println("Target stopped.")
}

func (t *Target) SetStateDB(target_state *statedb.StateDB) {
	oldStateRoot := common.Hash{}
	if t.stateDB != nil {
		oldStateRoot = t.stateDB.StateRoot
	}
	if t.debugState {
		log.Printf("%s[SET_STATE_DB]%s Old StateRoot: %s -> New StateRoot: %s", common.ColorYellow, common.ColorReset, oldStateRoot.Hex(), target_state.StateRoot.Hex())
		log.Printf("%s[SET_STATE_DB]%s HeaderHash: %s, Block: %v", common.ColorYellow, common.ColorReset, target_state.HeaderHash.Hex(), target_state.Block != nil)
	}
	t.stateDB = target_state
}

// handleConnection manages a single fuzzer session.
func (t *Target) handleConnection(conn net.Conn) {
	defer conn.Close()
	defer log.Println("Fuzzer disconnected.")

	for {
		msg, err := readMessage(conn)
		if err != nil {
			if err != io.EOF {
				log.Printf("%sError reading message: %v%s\n", common.ColorRed, err, common.ColorReset)
			}
			return
		}

		var response *Message
		switch {
		case msg.PeerInfo != nil:
			response = t.onPeerInfo(msg.PeerInfo)
		case msg.Initialize != nil:
			response = t.onInitialize(msg.Initialize)
		case msg.SetState != nil:
			response = t.onSetState(msg.SetState)
		case msg.ImportBlock != nil:
			response = t.onImportBlock(msg.ImportBlock)
		case msg.GetState != nil:
			response = t.onGetState(msg.GetState)
		case msg.RefineBundle != nil:
			//response = t.onRefineBundle(msg.RefineBundle)
		case msg.GetExports != nil:
			response = t.onGetExports(msg.GetExports)
		default:
			log.Printf("%sReceived unknown or unexpected message type%s", common.ColorRed, common.ColorReset)
			return // Terminate on unexpected messages.
		}

		if err := sendMessage(conn, response); err != nil {
			log.Printf("%sError sending response: %v%s\n", common.ColorRed, err, common.ColorReset)
			return
		}
	}
}

// --- Message Handlers ---

func (t *Target) onPeerInfo(fuzzerInfo PeerInfo) *Message {
	log.Printf("%s[INCOMING REQ]%s PeerInfo", common.ColorBlue, common.ColorReset)
	log.Printf("%sReceived handshake from fuzzer: %s%s", common.ColorGray, fuzzerInfo.GetName(), common.ColorReset)
	log.Printf("%sFuzzer ↓ \n%s%s", common.ColorGray, fuzzerInfo.PrettyString(true), common.ColorReset)
	log.Printf("%s[OUTGOING RSP]%s PeerInfo", common.ColorGreen, common.ColorReset)
	return &Message{PeerInfo: t.targetInfo}
}

// onInitialize processes Initialize messages (V1 protocol)
func (t *Target) onInitialize(req *Initialize) *Message {

	log.Printf("%s[INCOMING REQ]%s Initialize", common.ColorBlue, common.ColorReset)
	log.Printf("%sReceived Initialize request with %d ancestry items and %d KVs%s",
		common.ColorGray, len(req.Ancestry), len(req.KeyVals.KeyVals), common.ColorReset)

	headerHash := req.Header.Hash()
	log.Printf("%sInitializing state with header: %s%s", common.ColorGray, headerHash.Hex(), common.ColorReset)

	// Log ancestry information if provided
	if len(req.Ancestry) > 0 {
		log.Printf("%sAncestry provided:%s", common.ColorGray, common.ColorReset)
		for i, ancestor := range req.Ancestry {
			//TODO: The lookup anchor of each report in the guarantees extrinsic (E_G) must be part of the last imported headers in the chain
			log.Printf("%s  [%d] Header: %s (Slot: %d) %s", common.ColorGray, i, ancestor.HeaderHash.Hex(), ancestor.Slot, common.ColorReset)
		}
	}

	// Reset the trie to avoid pollution from previous state
	// This ensures the state root is computed purely from the provided KeyVals
	t.store.ResetTrie()

	// Reset lastPrunedSlot if this Initialize is for an earlier slot (new test started)
	if req.Header.Slot < t.lastPrunedSlot {
		if t.debugState {
			log.Printf("%s[INIT_RESET]%s Slot %d: Trie reset, lastPrunedSlot (was %d) -> 0 (new test detected), stateDBMap kept (%d entries)%s", common.ColorYellow, common.ColorReset, req.Header.Slot, t.lastPrunedSlot, len(t.stateDBMap), common.ColorReset)
		}
		t.lastPrunedSlot = 0
	} else {
		if t.debugState {
			log.Printf("%s[INIT_RESET]%s Slot %d: Trie reset, lastPrunedSlot remains %d (continuing test), stateDBMap kept (%d entries)%s", common.ColorYellow, common.ColorReset, req.Header.Slot, t.lastPrunedSlot, len(t.stateDBMap), common.ColorReset)
		}
	}
	// Note: stateDBMap is NOT cleared - it accumulates across the session and pruning removes old entries
	// Note: headerHash -> stateRoot mappings in LevelDB persist across trie resets within the same session
	// Each fuzzer run (socket connection) has a unique sessionID namespace

	startTime := time.Now()
	recoveredStateDB, err := statedb.NewStateDBFromStateKeyVals(t.store, &req.KeyVals)
	recoveredStateDBStateRoot := recoveredStateDB.StateRoot
	if err != nil {
		stateRecoveryDuration := time.Since(startTime)
		log.Printf("%sState recovery failed (took %.3fms): %v%s", common.ColorRed, float64(stateRecoveryDuration.Nanoseconds())/1e6, err, common.ColorReset)
		return &Message{StateRoot: &recoveredStateDBStateRoot}
	}

	t.SetStateDB(recoveredStateDB)
	stateRecoveryDuration := time.Since(startTime)
	log.Printf("%sState recovery completed (took %.3fms)%s", common.ColorGray, float64(stateRecoveryDuration.Nanoseconds())/1e6, common.ColorReset)
	log.Printf("%sState_Root: %v%s", common.ColorGray, recoveredStateDBStateRoot.Hex(), common.ColorReset)
	log.Printf("%sHeaderHash: %v%s", common.ColorGray, headerHash.Hex(), common.ColorReset)

	t.stateDBMap[headerHash] = stateEntry{stateRoot: recoveredStateDBStateRoot, slot: req.Header.Slot}
	if err := t.storeHeaderStateRoot(headerHash, recoveredStateDBStateRoot); err != nil {
		log.Printf("%sWarning: failed to persist headerHash->stateRoot mapping: %v%s", common.ColorYellow, err, common.ColorReset)
	}
	log.Printf("%s[OUTGOING RSP]%s StateRoot", common.ColorGreen, common.ColorReset)
	return &Message{StateRoot: &recoveredStateDBStateRoot}
}

// onSetState validates the received state against the test vector's PreState.
func (t *Target) onSetState(req *HeaderWithState) *Message {
	log.Printf("%s[INCOMING REQ]%s SetState", common.ColorBlue, common.ColorReset)
	//log.Printf("Received SetState request with %d key-value pairs.", len(req.State.KeyVals))
	sky := statedb.StateKeyVals{
		KeyVals: req.State.KeyVals,
	}
	header := req.Header
	headerHash := header.Hash()
	log.Printf("%sSetting state with header: %s (%d KVs)%s", common.ColorGray, headerHash.Hex(), len(sky.KeyVals), common.ColorReset)
	if t.stateDB != nil {
		//log.Printf("%sWarning: Target already has a state initialized. Overwriting with new state.%s", colorYellow, common.ColorReset)
	}

	// Reset the trie to avoid pollution from previous state
	t.store.ResetTrie()
	t.stateDBMap = make(map[common.Hash]stateEntry)
	t.lastPrunedSlot = 0
	// Note: headerHash -> stateRoot mappings in LevelDB persist across trie resets within the same session
	// Each fuzzer run (socket connection) has a unique sessionID namespace

	startTime := time.Now()
	recovered_statedb, err := statedb.NewStateDBFromStateKeyVals(t.store, &sky)
	recovered_statedb_stateRoot := recovered_statedb.StateRoot
	if err != nil {
		stateRecoveryDuration := time.Since(startTime)
		log.Printf("%sState recovery failed (took %.3fms): %v%s", common.ColorRed, float64(stateRecoveryDuration.Nanoseconds())/1e6, err, common.ColorReset)
		return &Message{StateRoot: &recovered_statedb_stateRoot}
	}
	t.SetStateDB(recovered_statedb)
	stateRecoveryDuration := time.Since(startTime)
	log.Printf("%sState recovery completed (took %.3fms)%s", common.ColorGray, float64(stateRecoveryDuration.Nanoseconds())/1e6, common.ColorReset)
	//log.Printf("%sStateDB initialized with %d key-value pairs%s", common.ColorGray, len(sky.KeyVals), common.ColorReset)
	log.Printf("%sState_Root: %v%s", common.ColorGray, recovered_statedb_stateRoot.Hex(), common.ColorReset)
	log.Printf("%sHeaderHash: %v%s", common.ColorGray, headerHash.Hex(), common.ColorReset)

	t.stateDBMap[headerHash] = stateEntry{stateRoot: recovered_statedb_stateRoot, slot: header.Slot}
	if err := t.storeHeaderStateRoot(headerHash, recovered_statedb_stateRoot); err != nil {
		log.Printf("%sWarning: failed to persist headerHash->stateRoot mapping: %v%s", common.ColorYellow, err, common.ColorReset)
	}
	log.Printf("%s[OUTGOING RSP]%s StateRoot", common.ColorGreen, common.ColorReset)
	return &Message{StateRoot: &recovered_statedb_stateRoot}
}

// onImportBlock validates the received block against the test vector's block.
func (t *Target) onImportBlock(req *types.Block) *Message {
	log.Printf("%s[INCOMING REQ]%s ImportBlock", common.ColorBlue, common.ColorReset)
	headerHash := req.Header.Hash()
	fmt.Printf("%sHeaderHash: %s (Slot: %d)\n%s", common.ColorGray, headerHash.Hex(), req.Header.Slot, common.ColorReset)
	fmt.Printf("%sParentHeaderHash: %s\nParent StateRoot: %s\n%s", common.ColorGray, req.Header.ParentHeaderHash.Hex(), req.Header.ParentStateRoot.Hex(), common.ColorReset)

	if t.debugState {
		blockJSON, err := json.MarshalIndent(req, "", "  ")
		if err != nil {
			log.Printf("Error marshaling block: %v", err)
		} else {
			fmt.Printf("%s\n", blockJSON)
		}
	}

	// apply state transition using block and the current stateDB
	if t.stateDB == nil {
		log.Printf("%sError: Target has no state initialized. Please set the state first.%s", common.ColorRed, common.ColorReset)
		return &Message{StateRoot: &common.Hash{}}
	}
	pvmBackend := t.pvmBackend
	preState := t.stateDB
	preStateRoot := preState.StateRoot

	// Check if we need to fork from a different state
	if req.Header.ParentStateRoot != preStateRoot {

		// Look for the correct parent state in our stateDBMap cache
		var foundParentHeaderHash common.Hash
		for headerHash, entry := range t.stateDBMap {
			if entry.stateRoot == req.Header.ParentStateRoot {
				foundParentHeaderHash = headerHash
				if t.debugState {
					log.Printf("%s[FORK_DEBUG]%s Found parent state in cache: HeaderHash=%s has StateRoot=%s%s", common.ColorMagenta, common.ColorReset, headerHash.Hex(), entry.stateRoot.Hex(), common.ColorReset)
				}
				break
			}
		}

		// Create a copy of our current state and recover the parent state
		forkState := preState.Copy()
		if forkState == nil {
			log.Printf("%s[FORK_DEBUG]%s ERROR: failed to copy current state for forking%s", common.ColorRed, common.ColorReset, common.ColorReset)
		} else if err := forkState.InitTrieAndLoadJamState(req.Header.ParentStateRoot); err != nil {
			// Fallback failed - state not in trie storage
			log.Printf("%s[FORK_DEBUG]%s ERROR: unable to recover fork state: %v%s", common.ColorRed, common.ColorReset, err, common.ColorReset)
		} else {
			// Successfully recovered from trie storage
			if foundParentHeaderHash != (common.Hash{}) {
				forkState.HeaderHash = foundParentHeaderHash
			} else {
				// State was pruned from cache but recovered from trie storage
				log.Printf("%s[FORK_RECOVERY]%s Recovered pruned state from trie: %s%s", common.ColorYellow, common.ColorReset, req.Header.ParentStateRoot.Hex(), common.ColorReset)
			}
			if t.debugState {
				log.Printf("%s[FORK_DEBUG]%s Fork state prepared: HeaderHash=%s, StateRoot=%s%s", common.ColorMagenta, common.ColorReset, forkState.HeaderHash.Hex(), forkState.StateRoot.Hex(), common.ColorReset)
			}
			// Use the fork state instead of current state
			preState = forkState
			preStateRoot = forkState.StateRoot
		}
	}

	// Work with a copy of the state to avoid mutation on failure
	stateCopy := preState.Copy()

	startTime := time.Now()
	postState, jamErr := statedb.ApplyStateTransitionFromBlock(0, stateCopy, context.Background(), req, nil, pvmBackend, "")
	transitionDuration := time.Since(startTime)

	if jamErr != nil {
		log.Printf("%s[IMPORTBK ERR] %v (took %.3fms)%s", common.ColorRed, jamErr, float64(transitionDuration.Nanoseconds())/1e6, common.ColorReset)

		// Dump STF files for debugging if enabled
		if t.dumpStf {
			t.dumpStateTransitionFiles(preState, req, nil, jamErr, "failed")
		}

		// V1 protocol sends Error message on ImportBlock failure
		if GetProtocolHandler().GetProtocolVersion() == ProtocolV1 {
			log.Printf("%s[OUTGOING RSP]%s Error", common.ColorGreen, common.ColorReset)
			errorMsg := jamErr.Error()
			return &Message{Error: &errorMsg}
		}

		// V0/V0r protocols return pre-state root on failure
		return &Message{StateRoot: &preStateRoot}
	}

	// Dump STF files for debugging if enabled
	if t.dumpStf {
		t.dumpStateTransitionFiles(preState, req, postState, nil, "success")
	}

	log.Printf("%sState transition applied (took %.3fms)%s", common.ColorGray, float64(transitionDuration.Nanoseconds())/1e6, common.ColorReset)

	if t.debugState {
		log.Printf("%sPost State_Root: %v%s", common.ColorGray, postState.StateRoot.Hex(), common.ColorReset)
		log.Printf("%sPost HeaderHash: %v%s", common.ColorGray, headerHash.Hex(), common.ColorReset)
	}

	// Only update target state on successful import
	postStateRoot := postState.StateRoot
	t.SetStateDB(postState)
	t.stateDBMap[headerHash] = stateEntry{stateRoot: postStateRoot, slot: req.Header.Slot}
	if err := t.storeHeaderStateRoot(headerHash, postStateRoot); err != nil {
		log.Printf("%sWarning: failed to persist headerHash->stateRoot mapping: %v%s", common.ColorYellow, err, common.ColorReset)
	}

	// Prune old states to prevent memory growth
	t.pruneOldStates(req.Header.Slot)

	log.Printf("%s[OUTGOING RSP]%s StateRoot", common.ColorGreen, common.ColorReset)
	return &Message{StateRoot: &postStateRoot}
}

// onGetState returns the full state based on the test vector.
func (t *Target) onGetState(req *common.Hash) *Message {
	headerHash := *req
	log.Printf("%s[INCOMING REQ]%s GetState", common.ColorBlue, common.ColorReset)
	log.Printf("%sReceived GetState request headerHash: %s%s", common.ColorGray, headerHash.Hex(), common.ColorReset)

	// First check the cache for fast path
	var stateRoot common.Hash
	entry, exists := t.stateDBMap[headerHash]
	if exists && entry.stateRoot != (common.Hash{}) {
		stateRoot = entry.stateRoot
	} else {
		// State was pruned from cache - try LevelDB persistent mapping
		persistedRoot, found, err := t.getHeaderStateRoot(headerHash)
		if err != nil {
			log.Printf("%s[GET_STATE_DEBUG]%s Error reading persistent mapping for headerHash %s: %v%s", common.ColorRed, common.ColorReset, headerHash.Hex(), err, common.ColorReset)
			return &Message{State: &statedb.StateKeyVals{}}
		}
		if !found || persistedRoot == (common.Hash{}) {
			log.Printf("%s[GET_STATE_DEBUG]%s State not found for headerHash: %s (neither in cache nor LevelDB)%s", common.ColorRed, common.ColorReset, headerHash.Hex(), common.ColorReset)
			return &Message{State: &statedb.StateKeyVals{}}
		}
		stateRoot = persistedRoot
		log.Printf("%s[GET_STATE_DEBUG]%s State pruned from cache but recovered from LevelDB: headerHash=%s, stateRoot=%s%s", common.ColorYellow, common.ColorReset, headerHash.Hex(), stateRoot.Hex(), common.ColorReset)
	}

	recoveredStateDB := t.stateDB.Copy()
	if recoveredStateDB == nil {
		log.Printf("%s[GET_STATE_DEBUG]%s Error: failed to copy state for headerHash: %s%s", common.ColorRed, common.ColorReset, headerHash.Hex(), common.ColorReset)
		return &Message{State: &statedb.StateKeyVals{}}
	}
	if err := recoveredStateDB.InitTrieAndLoadJamState(stateRoot); err != nil {
		log.Printf("%s[GET_STATE_DEBUG]%s Error recovering state: %v%s", common.ColorRed, common.ColorReset, err, common.ColorReset)
		return &Message{State: &statedb.StateKeyVals{}}
	}

	log.Printf("%s[GET_STATE_DEBUG]%s After recovery: HeaderHash=%s, StateRoot=%s%s", common.ColorGray, common.ColorReset, recoveredStateDB.HeaderHash.Hex(), recoveredStateDB.StateRoot.Hex(), common.ColorReset)

	stateKeyVals := statedb.StateKeyVals{
		KeyVals: recoveredStateDB.GetAllKeyValues(),
	}
	log.Printf("%s[OUTGOING RSP]%s State (with %d key-value pairs)", common.ColorGreen, common.ColorReset, len(stateKeyVals.KeyVals))
	return &Message{State: &stateKeyVals}
}

/*
	func (t *Target) onRefineBundle(req *types.RefineBundle) *Message {
		workPackageHash := req.Bundle.PackageHash()
		log.Printf("%s[INCOMING REQ]%s RefineBundle", common.ColorBlue, common.ColorReset)
		core := req.Core
		bundle := req.Bundle
		segRootMappings := req.SegmentRootMappings

		log.Printf("%sWorkPackageHash: %v (bundleLen: %d, core: %d)%s", common.ColorGray, workPackageHash.Hex(), len(bundle.Bytes()), core, common.ColorReset)
		//log.Printf("%sReceived Bundle: %s%s", common.ColorGray, bundle.StringL(), common.ColorReset)

		startTime := time.Now()
		bundleSnapshot, err := refine.ExecuteWorkPackageBundleSkipAuth(
			t.stateDB,
			t.pvmBackend,
			t.stateDB.GetTimeslot(), // timeslot
			core,                    // workPackageCoreIndex from RefineBundle
			bundle,
			segRootMappings,
			t.stateDB.GetTimeslot(), // slot
			req.AuthGasUsed,         // authGasUsed from RefineBundle
			req.AuthTrace,           // authTrace from RefineBundle
		)
		executionDuration := time.Since(startTime)

		var workReport *types.WorkReport
		if err != nil {
			log.Printf("%s[REFINEBK ERR] %v (took %.3fms)%s", common.ColorRed, err, float64(executionDuration.Nanoseconds())/1e6, common.ColorReset)
			workReport = &types.WorkReport{}
		} else {
			workReport = &bundleSnapshot.Report

			_, exportedSegments, refineErr := refine.ExecuteWBRefinement(
				t.stateDB,
				t.pvmBackend,
				t.stateDB.GetTimeslot(),
				core,
				bundle,
				types.Result{Ok: req.AuthTrace}, // Synthetic auth result
			)

			if refineErr == nil && len(exportedSegments) > 0 {
				t.storeExportedSegments(workPackageHash, workReport.AvailabilitySpec.ExportedSegmentRoot, exportedSegments, t.stateDB.GetTimeslot())
			}

			log.Printf("%sBundle execution completed (took %.3fms)%s", common.ColorGray, float64(executionDuration.Nanoseconds())/1e6, common.ColorReset)
			executedReportHash := workReport.Hash()
			log.Printf("%sExec ReportHash: %s%s", common.ColorGray, executedReportHash.Hex(), common.ColorReset)
			//log.Printf("%sExec WorkReport: %s%s", common.ColorGray, workReport.String(), common.ColorReset)
		}
		log.Printf("%s[OUTGOING RSP]%s WorkReport", common.ColorGreen, common.ColorReset)
		return &Message{WorkReport: workReport}
	}
*/
func (t *Target) onGetExports(req *common.Hash) *Message {
	log.Printf("%s[INCOMING REQ]%s GetExports - Hash: %s", common.ColorBlue, common.ColorReset, req.Hex())

	exportedSegments, found := t.getExportedSegments(*req)
	if !found {
		// Return empty segments array
		emptySegments := make([][]byte, 0)
		log.Printf("%s[OUTGOING RSP]%s Segments - 0 segments (not found)", common.ColorGreen, common.ColorReset)
		return &Message{Segments: &emptySegments}
	}

	log.Printf("%s[OUTGOING RSP]%s Segments - %d segments", common.ColorGreen, common.ColorReset, len(exportedSegments))
	return &Message{Segments: &exportedSegments}
}

// storeExportedSegments stores exported segments with indirection to avoid data duplication
func (t *Target) storeExportedSegments(workPackageHash, exportsRoot common.Hash, exportedSegments [][]byte, slot uint32) {
	segmentsHash := common.BytesToHash(append([]byte("segments"), workPackageHash.Bytes()...))

	t.exportsData[segmentsHash] = exportedSegments

	t.exportsLookup[workPackageHash] = exportEntry{segmentsHash: segmentsHash, slot: slot}

	if exportsRoot != (common.Hash{}) {
		t.exportsLookup[exportsRoot] = exportEntry{segmentsHash: segmentsHash, slot: slot}
	}

	log.Printf("%sStored %d exported segments with hash %s for work package %s%s",
		common.ColorGray, len(exportedSegments), segmentsHash.Hex(), workPackageHash.Hex(), common.ColorReset)
}

// getExportedSegments retrieves exported segments by looking up via workPackageHash or exportsRoot
func (t *Target) getExportedSegments(requestHash common.Hash) ([][]byte, bool) {
	// Look up the segments hash by the requested hash
	// This could be either a work package hash or an exported segment root
	entry, found := t.exportsLookup[requestHash]
	if !found {
		log.Printf("%sNo exported segments lookup found for hash %s%s", common.ColorRed, requestHash.Hex(), common.ColorReset)
		return nil, false
	}

	exportedSegments, dataFound := t.exportsData[entry.segmentsHash]
	if !dataFound {
		log.Printf("%sNo exported segments data found for segments hash %s%s", common.ColorRed, entry.segmentsHash.Hex(), common.ColorReset)
		return nil, false
	}

	log.Printf("%sFound %d exported segments via segments hash %s for request hash %s%s",
		common.ColorGray, len(exportedSegments), entry.segmentsHash.Hex(), requestHash.Hex(), common.ColorReset)
	return exportedSegments, true
}

func (t *Target) pruneOldStates(currentSlot uint32) {
	// Only prune periodically to reduce overhead
	if currentSlot < t.lastPrunedSlot+PruneInterval {
		if t.debugState {
			log.Printf("%s[PRUNE_SKIP]%s Slot %d: too soon (lastPruned=%d, interval=%d, next=%d)%s", common.ColorGray, common.ColorReset, currentSlot, t.lastPrunedSlot, PruneInterval, t.lastPrunedSlot+PruneInterval, common.ColorReset)
		}
		return
	}

	cutoffSlot := uint32(0)
	if currentSlot > DefaultPruneDepth {
		cutoffSlot = currentSlot - DefaultPruneDepth
	}

	if t.debugState {
		log.Printf("%s[PRUNE_START]%s Slot %d: cutoffSlot=%d, stateCount=%d, lastPruned=%d%s", common.ColorYellow, common.ColorReset, currentSlot, cutoffSlot, len(t.stateDBMap), t.lastPrunedSlot, common.ColorReset)
	}
	t.lastPrunedSlot = currentSlot

	// Count states before pruning
	statesBeforePrune := len(t.stateDBMap)
	exportsBeforePrune := len(t.exportsLookup)

	// Prune old states from stateDBMap
	for headerHash, entry := range t.stateDBMap {
		if entry.slot < cutoffSlot {
			delete(t.stateDBMap, headerHash)
			if t.debugState {
				log.Printf("%s[PRUNE]%s Removed state for HeaderHash=%s at slot %d (cutoff=%d)%s", common.ColorYellow, common.ColorReset, headerHash.Hex(), entry.slot, cutoffSlot, common.ColorReset)
			}
		}
	}

	// Prune old exports from exportsLookup (two-pass to avoid deleting referenced data)
	// First pass: collect old lookup entries to remove
	oldLookupEntries := make([]common.Hash, 0)
	for requestHash, entry := range t.exportsLookup {
		if entry.slot < cutoffSlot {
			oldLookupEntries = append(oldLookupEntries, requestHash)
			if t.debugState {
				log.Printf("%s[PRUNE]%s Marking export lookup for removal: RequestHash=%s at slot %d (cutoff=%d)%s", common.ColorYellow, common.ColorReset, requestHash.Hex(), entry.slot, cutoffSlot, common.ColorReset)
			}
		}
	}

	// Remove old lookup entries
	for _, requestHash := range oldLookupEntries {
		delete(t.exportsLookup, requestHash)
	}

	// Second pass: delete exportsData only if no remaining lookup entries reference it
	referencedHashes := make(map[common.Hash]bool)
	for _, entry := range t.exportsLookup {
		referencedHashes[entry.segmentsHash] = true
	}
	for segmentsHash := range t.exportsData {
		if !referencedHashes[segmentsHash] {
			delete(t.exportsData, segmentsHash)
			if t.debugState {
				log.Printf("%s[PRUNE]%s Removed unreferenced export data: SegmentsHash=%s%s", common.ColorYellow, common.ColorReset, segmentsHash.Hex(), common.ColorReset)
			}
		}
	}

	statesAfterPrune := len(t.stateDBMap)
	exportsAfterPrune := len(t.exportsLookup)

	if t.debugState {
		if statesBeforePrune > statesAfterPrune || exportsBeforePrune > exportsAfterPrune {
			log.Printf("%s[PRUNE]%s Slot %d: Pruned %d states (%d -> %d), %d exports (%d -> %d)%s", common.ColorYellow, common.ColorReset, currentSlot, statesBeforePrune-statesAfterPrune, statesBeforePrune, statesAfterPrune, exportsBeforePrune-exportsAfterPrune, exportsBeforePrune, exportsAfterPrune, common.ColorReset)
		} else {
			log.Printf("%s[PRUNE]%s Slot %d: Nothing to prune (cutoff=%d, all %d states are >= cutoff)%s", common.ColorYellow, common.ColorReset, currentSlot, cutoffSlot, statesBeforePrune, common.ColorReset)
		}
	}
}

// headerStateRootKey creates the LevelDB key for headerHash -> stateRoot mapping
// Keys are namespaced by sessionID to isolate different fuzzer runs
func (t *Target) headerStateRootKey(headerHash common.Hash) []byte {
	prefix := []byte("hh2s_")
	sessionPrefix := append(prefix, []byte(t.sessionID+"_")...)
	return append(sessionPrefix, headerHash.Bytes()...)
}

// storeHeaderStateRoot persists the headerHash -> stateRoot mapping to LevelDB
func (t *Target) storeHeaderStateRoot(headerHash, stateRoot common.Hash) error {
	key := t.headerStateRootKey(headerHash)
	return t.store.Persist.Put(key, stateRoot.Bytes())
}

// getHeaderStateRoot retrieves the stateRoot for a given headerHash from LevelDB
func (t *Target) getHeaderStateRoot(headerHash common.Hash) (common.Hash, bool, error) {
	key := t.headerStateRootKey(headerHash)
	value, found, err := t.store.Persist.Get(key)
	if err != nil || !found {
		return common.Hash{}, false, err
	}
	if len(value) != 32 {
		return common.Hash{}, false, fmt.Errorf("invalid stateRoot length: %d", len(value))
	}
	var stateRoot common.Hash
	copy(stateRoot[:], value)
	return stateRoot, true, nil
}

func sendMessage(conn net.Conn, msg *Message) error {
	encodedBody, err := GetProtocolHandler().Encode(msg)
	if err != nil {
		return fmt.Errorf("%sfailed to encode message: %w%s", common.ColorRed, err, common.ColorReset)
	}

	msgLength := uint32(len(encodedBody))
	if err := binary.Write(conn, binary.LittleEndian, msgLength); err != nil {
		return fmt.Errorf("%sfailed to write message length: %w%s", common.ColorRed, err, common.ColorReset)
	}

	if _, err := conn.Write(encodedBody); err != nil {
		return fmt.Errorf("%sfailed to write message body: %w%s", common.ColorRed, err, common.ColorReset)
	}
	return nil
}

// dumpStateTransitionFiles dumps state transition information for debugging
func (t *Target) dumpStateTransitionFiles(preState *statedb.StateDB, block *types.Block, postState *statedb.StateDB, err error, status string) {
	if block == nil {
		return
	}

	headerHash := block.Header.Hash().Hex()
	slot := block.Header.Slot

	// Ensure the dump directory exists
	if err := os.MkdirAll(t.dumpLocation, 0755); err != nil {
		log.Printf("%s[STF_DUMP_ERR] Failed to create dump directory %s: %v%s", common.ColorRed, t.dumpLocation, err, common.ColorReset)
		return
	}

	filename := fmt.Sprintf("%s/stf_dump_slot%d_%s_%s.json", t.dumpLocation, slot, headerHash, status)

	// Create StateTransition struct
	transition := StateTransition{
		Block: *block,
	}

	// Convert preState to StateSnapshotRaw if available
	if preState != nil {
		transition.PreState = statedb.StateSnapshotRaw{
			StateRoot: preState.StateRoot,
			KeyVals:   preState.GetAllKeyValues(),
		}
	}

	// Convert postState to StateSnapshotRaw if available
	if postState != nil {
		transition.PostState = statedb.StateSnapshotRaw{
			StateRoot: postState.StateRoot,
			KeyVals:   postState.GetAllKeyValues(),
		}
	}

	// Marshal the StateTransition struct
	data, jsonErr := json.MarshalIndent(transition, "", "  ")
	if jsonErr != nil {
		log.Printf("%s[STF_DUMP_ERR] Failed to marshal STF data: %v%s", common.ColorRed, jsonErr, common.ColorReset)
		return
	}

	// Add error information if present (append to file or separate handling)
	if err != nil {
		errorInfo := map[string]interface{}{
			"error":  err.Error(),
			"status": status,
		}
		errorData, _ := json.MarshalIndent(errorInfo, "", "  ")
		data = append(data, []byte("\n// Error information:\n// ")...)
		data = append(data, errorData...)
	}

	writeErr := os.WriteFile(filename, data, 0644)
	if writeErr != nil {
		log.Printf("%s[STF_DUMP_ERR] Failed to write STF file: %v%s", common.ColorRed, writeErr, common.ColorReset)
		return
	}

	log.Printf("%s[STF_DUMP] Wrote %s%s", common.ColorBlue, filename, common.ColorReset)
}

func readMessage(conn net.Conn) (*Message, error) {
	var msgLength uint32
	if err := binary.Read(conn, binary.LittleEndian, &msgLength); err != nil {
		if err == io.EOF {
			return nil, io.EOF
		}
		return nil, fmt.Errorf("%sfailed to read message length: %w%s", common.ColorRed, err, common.ColorReset)
	}

	body := make([]byte, msgLength)
	if _, err := io.ReadFull(conn, body); err != nil {
		return nil, fmt.Errorf("%sfailed to read message body: %w%s", common.ColorRed, err, common.ColorReset)
	}

	return GetProtocolHandler().Decode(body)
}
