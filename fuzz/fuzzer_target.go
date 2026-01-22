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

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/pvm"
	"github.com/colorfulnotion/jam/storage"
	// "github.com/colorfulnotion/jam/refine"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

// StateTransition represents a complete state transition for debugging
type StateTransition struct {
	PreState  statedb.StateSnapshotRaw `json:"pre_state"`
	Block     types.Block              `json:"block"`
	PostState statedb.StateSnapshotRaw `json:"post_state"`
}

// Target represents the server-side implementation that the fuzzer connects to.
type Target struct {
	listener      net.Listener
	socketPath    string
	targetInfo    PeerInfo
	store         *storage.StorageHub     // Storage backend for the state database.
	stateDB       *statedb.StateDB            // This can be used to manage state transitions.
	stateDBMap    map[common.Hash]common.Hash // [headererHash]posteriorStateRoot
	pvmBackend    string                      // Backend to use for state transitions, e.g., "Interpreter" or "Compiler".
	exportsLookup map[common.Hash]common.Hash // [workPackageHash or exportsRoot] -> segmentsHash
	exportsData   map[common.Hash][][]byte    // [segmentsHash] -> exportedSegments
	debugState    bool                        // Enable detailed state debugging output
	dumpStf       bool                        // Dump state transition files for debugging
	dumpLocation  string                      // Directory path for STF dump files
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

	return &Target{
		socketPath:    socketPath,
		targetInfo:    targetInfo,
		store:         store,
		stateDB:       nil,
		stateDBMap:    make(map[common.Hash]common.Hash),
		pvmBackend:    pvmBackend,
		exportsLookup: make(map[common.Hash]common.Hash),
		exportsData:   make(map[common.Hash][][]byte),
		debugState:    debugState,
		dumpStf:       dumpStf,
		dumpLocation:  dumpLocation,
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
	t.stateDBMap = make(map[common.Hash]common.Hash)

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

	t.stateDBMap[headerHash] = recoveredStateDBStateRoot
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
	t.stateDBMap = make(map[common.Hash]common.Hash)

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

	t.stateDBMap[headerHash] = recovered_statedb_stateRoot
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

		// Look for the correct parent state in our stateDBMap
		var foundParentHeaderHash common.Hash
		for headerHash, stateRoot := range t.stateDBMap {
			if stateRoot == req.Header.ParentStateRoot {
				foundParentHeaderHash = headerHash
				if t.debugState {
					log.Printf("%s[FORK_DEBUG]%s Found parent state: HeaderHash=%s has StateRoot=%s%s", common.ColorMagenta, common.ColorReset, headerHash.Hex(), stateRoot.Hex(), common.ColorReset)
				}
				break
			}
		}

		if foundParentHeaderHash != (common.Hash{}) {

			// Create a copy of our current state and recover the parent state
			forkState := preState.Copy()
			if forkState == nil {
				log.Printf("%s[FORK_DEBUG]%s ERROR: failed to copy current state for forking%s", common.ColorRed, common.ColorReset, common.ColorReset)
			} else if err := forkState.InitTrieAndLoadJamState(req.Header.ParentStateRoot); err != nil {
				log.Printf("%s[FORK_DEBUG]%s ERROR: unable to recover fork state: %v%s", common.ColorRed, common.ColorReset, err, common.ColorReset)
			} else {
				forkState.HeaderHash = foundParentHeaderHash
				//forkState.StateRoot = req.Header.ParentStateRoot // Now set inside InitTrieAndLoadJamState
				if t.debugState {
					log.Printf("%s[FORK_DEBUG]%s Fork state prepared: HeaderHash=%s, StateRoot=%s%s", common.ColorMagenta, common.ColorReset, forkState.HeaderHash.Hex(), forkState.StateRoot.Hex(), common.ColorReset)
				}
				// Use the fork state instead of current state
				preState = forkState
				preStateRoot = forkState.StateRoot
			}
		} else {
			log.Printf("%s[FORK_DEBUG]%s ERROR: Could not find parent state with StateRoot=%s%s", common.ColorRed, common.ColorReset, req.Header.ParentStateRoot.Hex(), common.ColorReset)
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
	t.stateDBMap[headerHash] = postStateRoot
	log.Printf("%s[OUTGOING RSP]%s StateRoot", common.ColorGreen, common.ColorReset)
	return &Message{StateRoot: &postStateRoot}
}

// onGetState returns the full state based on the test vector.
func (t *Target) onGetState(req *common.Hash) *Message {
	headerHash := *req
	log.Printf("%s[INCOMING REQ]%s GetState", common.ColorBlue, common.ColorReset)
	log.Printf("%sReceived GetState request headerHash: %s%s", common.ColorGray, headerHash.Hex(), common.ColorReset)

	if t.stateDBMap[headerHash] == (common.Hash{}) {
		log.Printf("%s[GET_STATE_DEBUG]%s Error: No state found for headerHash: %s%s", common.ColorRed, common.ColorReset, headerHash.Hex(), common.ColorReset)
		return &Message{State: &statedb.StateKeyVals{}}
	}

	// Fetch back the state root from the map
	stateRoot := t.stateDBMap[headerHash]
	//log.Printf("%s[GET_STATE_DEBUG]%s Recovering state for headerHash: %s with stateRoot: %s%s", common.ColorGray, common.ColorReset, headerHash.Hex(), stateRoot.Hex(), common.ColorReset)
	//log.Printf("%s[GET_STATE_DEBUG]%s Current stateDB: HeaderHash=%s, StateRoot=%s%s", common.ColorGray, common.ColorReset, t.stateDB.HeaderHash.Hex(), t.stateDB.StateRoot.Hex(), common.ColorReset)

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
				t.storeExportedSegments(workPackageHash, workReport.AvailabilitySpec.ExportedSegmentRoot, exportedSegments)
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
func (t *Target) storeExportedSegments(workPackageHash, exportsRoot common.Hash, exportedSegments [][]byte) {
	segmentsHash := common.BytesToHash(append([]byte("segments"), workPackageHash.Bytes()...))

	t.exportsData[segmentsHash] = exportedSegments

	t.exportsLookup[workPackageHash] = segmentsHash

	if exportsRoot != (common.Hash{}) {
		t.exportsLookup[exportsRoot] = segmentsHash
	}

	log.Printf("%sStored %d exported segments with hash %s for work package %s%s",
		common.ColorGray, len(exportedSegments), segmentsHash.Hex(), workPackageHash.Hex(), common.ColorReset)
}

// getExportedSegments retrieves exported segments by looking up via workPackageHash or exportsRoot
func (t *Target) getExportedSegments(requestHash common.Hash) ([][]byte, bool) {
	// Look up the segments hash by the requested hash
	// This could be either a work package hash or an exported segment root
	segmentsHash, found := t.exportsLookup[requestHash]
	if !found {
		log.Printf("%sNo exported segments lookup found for hash %s%s", common.ColorRed, requestHash.Hex(), common.ColorReset)
		return nil, false
	}

	exportedSegments, dataFound := t.exportsData[segmentsHash]
	if !dataFound {
		log.Printf("%sNo exported segments data found for segments hash %s%s", common.ColorRed, segmentsHash.Hex(), common.ColorReset)
		return nil, false
	}

	log.Printf("%sFound %d exported segments via segments hash %s for request hash %s%s",
		common.ColorGray, len(exportedSegments), segmentsHash.Hex(), requestHash.Hex(), common.ColorReset)
	return exportedSegments, true
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
