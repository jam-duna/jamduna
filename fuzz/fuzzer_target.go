package fuzz

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"time"

	"github.com/colorfulnotion/jam/common"
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
}

// NewTarget creates a new target instance, armed with a specific test case.
func NewTarget(socketPath string, targetInfo PeerInfo) *Target {
	levelDBPath := fmt.Sprintf("/tmp/target_%d", time.Now().Unix())
	store, err := storage.NewStateDBStorage(levelDBPath)
	if err != nil {
		log.Fatalf("Failed to create state DB storage: %v", err)
	}
	return &Target{
		socketPath: socketPath,
		targetInfo: targetInfo,
		store:      store,
		stateDB:    nil,
		stateDBMap: make(map[common.Hash]common.Hash),
	}
}

// Start begins listening on the Unix socket for incoming fuzzer connections.
func (t *Target) Start() error {
	if err := os.RemoveAll(t.socketPath); err != nil {
		return fmt.Errorf("failed to remove old socket file: %w", err)
	}

	listener, err := net.Listen("unix", t.socketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on socket %s: %w", t.socketPath, err)
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
				log.Printf("Error reading message: %v\n", err)
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
			log.Println("Received unknown or unexpected message type")
			return // Terminate on unexpected messages.
		}

		if err := sendMessage(conn, response); err != nil {
			log.Printf("Error sending response: %v\n", err)
			return
		}
	}
}

// --- Message Handlers ---

func (t *Target) onPeerInfo(fuzzerInfo *PeerInfo) *Message {
	log.Printf("[INCOMING REQUEST] PeerInfo")
	log.Printf("Received handshake from fuzzer: %s", fuzzerInfo.Name)
	log.Printf("[OUTGOING RESPONSE] PeerInfo")
	return &Message{PeerInfo: &t.targetInfo}
}

// onSetState validates the received state against the test vector's PreState.
func (t *Target) onSetState(req *HeaderWithState) *Message {
	log.Printf("[INCOMING REQUEST] SetState")
	log.Printf("Received SetState request with %d key-value pairs.", len(req.State.KeyVals))
	sky := statedb.StateKeyVals{
		KeyVals: req.State.KeyVals,
	}
	header := req.Header
	headerHash := header.Hash()
	log.Printf("Setting state with header: %s", headerHash.Hex())
	if t.stateDB != nil {
		log.Println("Warning: Target already has a state initialized. Overwriting with new state.")
	}
	recovered_statedb, err := statedb.NewStateDBFromStateKeyVals(t.store, &sky)
	recovered_statedb_stateRoot := recovered_statedb.StateRoot
	if err != nil {
		//TODO: how to handle this error?
		return &Message{StateRoot: &common.Hash{}}
	}
	t.stateDB = recovered_statedb
	log.Printf("StateDB initialized with %d key-value pairs. stateRoot: %v | HeaderHash: %v", len(sky.KeyVals), recovered_statedb_stateRoot.Hex(), headerHash.Hex())
	t.stateDBMap[headerHash] = recovered_statedb_stateRoot
	log.Printf("[OUTGOING RESPONSE] StateRoot")
	return &Message{StateRoot: &recovered_statedb_stateRoot}
}

// onImportBlock validates the received block against the test vector's block.
func (t *Target) onImportBlock(req *types.Block) *Message {
	log.Printf("[INCOMING REQUEST] ImportBlock")
	log.Printf("Received ImportBlock request for block hash: %s", req.Hash().Hex())

	// apply state transition using block and the current stateDB
	if t.stateDB == nil {
		log.Println("Error: Target has no state initialized. Please set the state first.")
		return &Message{StateRoot: &common.Hash{}}
	}
	headerHash := req.Header.Hash()
	pvmBackend := "interpreter" // or any other backend you want to use
	preState := t.stateDB
	postState, jamErr := statedb.ApplyStateTransitionFromBlock(preState, context.Background(), req, nil, pvmBackend)
	if jamErr != nil {
		log.Printf("Error applying state transition from block: %v", jamErr)
		return &Message{StateRoot: &common.Hash{}}
	}
	log.Printf("State transition applied. New stateRoot: %v | HeaderHash: %v", postState.StateRoot.Hex(), headerHash.Hex())
	postStateRoot := postState.StateRoot
	t.stateDBMap[headerHash] = postStateRoot
	log.Printf("[OUTGOING RESPONSE] StateRoot")
	return &Message{StateRoot: &postStateRoot}
}

// onGetState returns the full state based on the test vector.
func (t *Target) onGetState(req *common.Hash) *Message {
	headerHash := *req
	log.Printf("[INCOMING REQUEST] GetState")
	log.Printf("Received GetState request headerHash: %s", headerHash.Hex())
	if t.stateDBMap[headerHash] == (common.Hash{}) {
		log.Printf("Error: No state found for headerHash: %s", headerHash.Hex())
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
	log.Printf("[OUTGOING RESPONSE] State")
	return &Message{State: &stateKeyVals}
}

// --- Generic Network Functions (can be shared) ---

func sendMessage(conn net.Conn, msg *Message) error {
	encodedBody, err := encode(msg)
	if err != nil {
		return fmt.Errorf("failed to encode message: %w", err)
	}

	msgLength := uint32(len(encodedBody))
	if err := binary.Write(conn, binary.LittleEndian, msgLength); err != nil {
		return fmt.Errorf("failed to write message length: %w", err)
	}

	if _, err := conn.Write(encodedBody); err != nil {
		return fmt.Errorf("failed to write message body: %w", err)
	}
	return nil
}

func readMessage(conn net.Conn) (*Message, error) {
	var msgLength uint32
	if err := binary.Read(conn, binary.LittleEndian, &msgLength); err != nil {
		if err == io.EOF {
			return nil, io.EOF
		}
		return nil, fmt.Errorf("failed to read message length: %w", err)
	}

	body := make([]byte, msgLength)
	if _, err := io.ReadFull(conn, body); err != nil {
		return nil, fmt.Errorf("failed to read message body: %w", err)
	}

	return decode(body)
}
