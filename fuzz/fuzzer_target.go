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
	pvmBackend string                      // Backend to use for state transitions, e.g., "Interpreter" or "Recompiler".
}

// NewTarget creates a new target instance, armed with a specific test case.
func NewTarget(socketPath string, targetInfo PeerInfo, pvmBackend string) *Target {
	levelDBPath := fmt.Sprintf("/tmp/target_%d", time.Now().Unix())
	store, err := storage.NewStateDBStorage(levelDBPath)
	if err != nil {
		log.Fatalf("Failed to create state DB storage: %v", err)
	}
	if strings.ToLower(pvmBackend) != pvm.BackendInterpreter && strings.ToLower(pvmBackend) != pvm.BackendRecompiler {
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
		return fmt.Errorf("%sfailed to remove old socket file: %w%s", colorRed, err, colorReset)
	}

	listener, err := net.Listen("unix", t.socketPath)
	if err != nil {
		return fmt.Errorf("%sfailed to listen on socket %s: %w%s", colorRed, t.socketPath, err, colorReset)
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

// handleConnection manages a single fuzzer session.
func (t *Target) handleConnection(conn net.Conn) {
	defer conn.Close()
	defer log.Println("Fuzzer disconnected.")

	for {
		msg, err := readMessage(conn)
		if err != nil {
			if err != io.EOF {
				log.Printf("%sError reading message: %v%s\n", colorRed, err, colorReset)
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
		default:
			log.Printf("%sReceived unknown or unexpected message type%s", colorRed, colorReset)
			return // Terminate on unexpected messages.
		}

		if err := sendMessage(conn, response); err != nil {
			log.Printf("%sError sending response: %v%s\n", colorRed, err, colorReset)
			return
		}
	}
}

// --- Message Handlers ---

func (t *Target) onPeerInfo(fuzzerInfo *PeerInfo) *Message {
	log.Printf("%s[INCOMING REQUEST]%s PeerInfo", colorBlue, colorReset)
	log.Printf("Received handshake from fuzzer: %s", fuzzerInfo.Name)
	log.Printf("%s[OUTGOING RESPONSE]%s PeerInfo", colorGreen, colorReset)
	return &Message{PeerInfo: &t.targetInfo}
}

// onSetState validates the received state against the test vector's PreState.
func (t *Target) onSetState(req *HeaderWithState) *Message {
	log.Printf("%s[INCOMING REQUEST]%s SetState", colorBlue, colorReset)
	log.Printf("Received SetState request with %d key-value pairs.", len(req.State.KeyVals))
	sky := statedb.StateKeyVals{
		KeyVals: req.State.KeyVals,
	}
	header := req.Header
	headerHash := header.Hash()
	log.Printf("Setting state with header: %s", headerHash.Hex())
	if t.stateDB != nil {
		//log.Printf("%sWarning: Target already has a state initialized. Overwriting with new state.%s", colorYellow, colorReset)
	}
	recovered_statedb, err := statedb.NewStateDBFromStateKeyVals(t.store, &sky)
	recovered_statedb_stateRoot := recovered_statedb.StateRoot
	if err != nil {
		//TODO: how to handle this error?
		return &Message{StateRoot: &recovered_statedb_stateRoot}
	}
	t.stateDB = recovered_statedb
	log.Printf("StateDB initialized with %d key-value pairs. stateRoot: %v | HeaderHash: %v", len(sky.KeyVals), recovered_statedb_stateRoot.Hex(), headerHash.Hex())
	t.stateDBMap[headerHash] = recovered_statedb_stateRoot
	log.Printf("%s[OUTGOING RESPONSE]%s StateRoot", colorGreen, colorReset)
	return &Message{StateRoot: &recovered_statedb_stateRoot}
}

// onImportBlock validates the received block against the test vector's block.
func (t *Target) onImportBlock(req *types.Block) *Message {
	log.Printf("%s[INCOMING REQUEST]%s ImportBlock", colorBlue, colorReset)
	log.Printf("Received ImportBlock request for block hash: %s", req.Hash().Hex())

	// apply state transition using block and the current stateDB
	if t.stateDB == nil {
		log.Printf("%sError: Target has no state initialized. Please set the state first.%s", colorRed, colorReset)
		return &Message{StateRoot: &common.Hash{}}
	}
	headerHash := req.Header.Hash()
	pvmBackend := t.pvmBackend
	preState := t.stateDB
	preStateRoot := preState.StateRoot
	postState, jamErr := statedb.ApplyStateTransitionFromBlock(preState, context.Background(), req, nil, pvmBackend)
	if jamErr != nil {
		log.Printf("%sError applying state transition from block: %v%s", colorRed, jamErr, colorReset)
		return &Message{StateRoot: &preStateRoot}
	}
	log.Printf("State transition applied. New stateRoot: %v | HeaderHash: %v", postState.StateRoot.Hex(), headerHash.Hex())
	postStateRoot := postState.StateRoot
	t.stateDBMap[headerHash] = postStateRoot
	log.Printf("%s[OUTGOING RESPONSE]%s StateRoot", colorGreen, colorReset)
	return &Message{StateRoot: &postStateRoot}
}

// onGetState returns the full state based on the test vector.
func (t *Target) onGetState(req *common.Hash) *Message {
	headerHash := *req
	log.Printf("%s[INCOMING REQUEST]%s GetState", colorBlue, colorReset)
	log.Printf("Received GetState request headerHash: %s", headerHash.Hex())
	if t.stateDBMap[headerHash] == (common.Hash{}) {
		log.Printf("%sError: No state found for headerHash: %s%s", colorRed, headerHash.Hex(), colorReset)
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
	log.Printf("%s[OUTGOING RESPONSE]%s State", colorGreen, colorReset)
	return &Message{State: &stateKeyVals}
}

func sendMessage(conn net.Conn, msg *Message) error {
	encodedBody, err := encode(msg)
	if err != nil {
		return fmt.Errorf("%sfailed to encode message: %w%s", colorRed, err, colorReset)
	}

	msgLength := uint32(len(encodedBody))
	if err := binary.Write(conn, binary.LittleEndian, msgLength); err != nil {
		return fmt.Errorf("%sfailed to write message length: %w%s", colorRed, err, colorReset)
	}

	if _, err := conn.Write(encodedBody); err != nil {
		return fmt.Errorf("%sfailed to write message body: %w%s", colorRed, err, colorReset)
	}
	return nil
}

func readMessage(conn net.Conn) (*Message, error) {
	var msgLength uint32
	if err := binary.Read(conn, binary.LittleEndian, &msgLength); err != nil {
		if err == io.EOF {
			return nil, io.EOF
		}
		return nil, fmt.Errorf("%sfailed to read message length: %w%s", colorRed, err, colorReset)
	}

	body := make([]byte, msgLength)
	if _, err := io.ReadFull(conn, body); err != nil {
		return nil, fmt.Errorf("%sfailed to read message body: %w%s", colorRed, err, colorReset)
	}

	return decode(body)
}
