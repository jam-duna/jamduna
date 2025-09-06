package fuzz

import (
	"context"
	"encoding/binary"
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
	"github.com/colorfulnotion/jam/refine"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"
)

// Target represents the server-side implementation that the fuzzer connects to.
type Target struct {
	listener   net.Listener
	socketPath string
	targetInfo PeerInfo
	store      *storage.StateDBStorage     // Storage backend for the state database.
	stateDB    *statedb.StateDB            // This can be used to manage state transitions.
	stateDBMap map[common.Hash]common.Hash // [headererHash]posteriorStateRoot
	pvmBackend string                      // Backend to use for state transitions, e.g., "Interpreter" or "Compiler".
}

// NewTarget creates a new target instance, armed with a specific test case.
func NewTarget(socketPath string, targetInfo PeerInfo, pvmBackend string) *Target {
	levelDBPath := fmt.Sprintf("/tmp/target_%d", time.Now().Unix())
	store, err := storage.NewStateDBStorage(levelDBPath)
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
		socketPath: socketPath,
		targetInfo: targetInfo,
		store:      store,
		stateDB:    nil,
		stateDBMap: make(map[common.Hash]common.Hash),
		pvmBackend: pvmBackend,
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
		case msg.SetState != nil:
			response = t.onSetState(msg.SetState)
		case msg.ImportBlock != nil:
			response = t.onImportBlock(msg.ImportBlock)
		case msg.GetState != nil:
			response = t.onGetState(msg.GetState)
		case msg.RefineBundle != nil:
			response = t.onRefineBundle(msg.RefineBundle)
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

func (t *Target) onPeerInfo(fuzzerInfo *PeerInfo) *Message {
	log.Printf("%s[INCOMING REQ]%s PeerInfo", common.ColorBlue, common.ColorReset)
	log.Printf("%sReceived handshake from fuzzer: %s%s", common.ColorGray, fuzzerInfo.Name, common.ColorReset)
	log.Printf("%sFuzzer ↓ \n%s%s", common.ColorGray, fuzzerInfo.PrettyString(true), common.ColorReset)
	log.Printf("%s[OUTGOING RSP]%s PeerInfo", common.ColorGreen, common.ColorReset)
	return &Message{PeerInfo: &t.targetInfo}
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
	log.Printf("%sReceived ImportBlock request for HeaderHash: %s%s", common.ColorGray, headerHash.Hex(), common.ColorReset)

	// apply state transition using block and the current stateDB
	if t.stateDB == nil {
		log.Printf("%sError: Target has no state initialized. Please set the state first.%s", common.ColorRed, common.ColorReset)
		return &Message{StateRoot: &common.Hash{}}
	}
	pvmBackend := t.pvmBackend
	preState := t.stateDB
	preStateRoot := preState.StateRoot

	startTime := time.Now()
	postState, jamErr := statedb.ApplyStateTransitionFromBlock(preState, context.Background(), req, nil, pvmBackend)
	transitionDuration := time.Since(startTime)

	if jamErr != nil {
		log.Printf("%s[IMPORTBK ERR] %v (took %.3fms)%s", common.ColorRed, jamErr, float64(transitionDuration.Nanoseconds())/1e6, common.ColorReset)
		return &Message{StateRoot: &preStateRoot}
	}
	log.Printf("%sState transition applied (took %.3fms)%s", common.ColorGray, float64(transitionDuration.Nanoseconds())/1e6, common.ColorReset)
	log.Printf("%sPost State_Root: %v%s", common.ColorGray, postState.StateRoot.Hex(), common.ColorReset)
	log.Printf("%sPost HeaderHash: %v%s", common.ColorGray, headerHash.Hex(), common.ColorReset)
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
	log.Printf("Received GetState request headerHash: %s", headerHash.Hex())
	if t.stateDBMap[headerHash] == (common.Hash{}) {
		log.Printf("%sError: No state found for headerHash: %s%s", common.ColorRed, headerHash.Hex(), common.ColorReset)
		return &Message{State: &statedb.StateKeyVals{}}
	}

	// Fetch back the state root from the map
	stateRoot := t.stateDBMap[headerHash]
	log.Printf("Recovering state for headerHash: %s with stateRoot: %s", headerHash.Hex(), stateRoot.Hex())
	recoveredStateDB := t.stateDB.Copy()
	recoveredStateDB.RecoverJamState(stateRoot)

	stateKeyVals := statedb.StateKeyVals{
		KeyVals: recoveredStateDB.GetAllKeyValues(),
	}
	log.Printf("%s[OUTGOING RSP]%s State", common.ColorGreen, common.ColorReset)
	return &Message{State: &stateKeyVals}
}

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
		// Return empty WorkReport on error
		workReport = &types.WorkReport{}
	} else {
		// Extract WorkReport from the executed bundle snapshot
		workReport = &bundleSnapshot.Report

		log.Printf("%sBundle execution completed (took %.3fms)%s", common.ColorGray, float64(executionDuration.Nanoseconds())/1e6, common.ColorReset)
		// Log the executed report hash for comparison
		executedReportHash := workReport.Hash()
		log.Printf("%sExec ReportHash: %s%s", common.ColorGray, executedReportHash.Hex(), common.ColorReset)
		//log.Printf("%sExec WorkReport: %s%s", common.ColorGray, workReport.String(), common.ColorReset)
	}
	log.Printf("%s[OUTGOING RSP]%s WorkReport", common.ColorGreen, common.ColorReset)
	return &Message{WorkReport: workReport}
}

func sendMessage(conn net.Conn, msg *Message) error {
	encodedBody, err := encode(msg)
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

	return decode(body)
}
