package fuzz

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"
)

// Fuzzer is the main engine that drives conformance testing.
// It manages local state computation, test data, and communication with the target.
type Fuzzer struct {
	conn           net.Conn
	socketPath     string
	fuzzerInfo     PeerInfo
	seed           []byte
	store          *storage.StateDBStorage
	internalSTFMap map[common.Hash]*statedb.StateTransition
}

// NewFuzzer creates a new fuzzer instance.
func NewFuzzer(storageDir string, socketPath string, fuzzerInfo PeerInfo) (*Fuzzer, error) {
	sdbStorage, err := storage.NewStateDBStorage(storageDir)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize fuzzer's local storage: %w", err)
	}

	return &Fuzzer{
		socketPath:     socketPath,
		fuzzerInfo:     fuzzerInfo,
		store:          sdbStorage,
		internalSTFMap: make(map[common.Hash]*statedb.StateTransition),
	}, nil
}

// RunImplementationRPCServer starts the RPC server for the implementation.
func (fs *Fuzzer) RunImplementationRPCServer() {
	runRPCServer(fs.store)
}

// RunInternalRPCServer starts the RPC server for the internal fuzzer logic.
func (fs *Fuzzer) RunInternalRPCServer() {
	runRPCServerInternal(fs.seed, fs.store)
}

func (fs *Fuzzer) GenerateExecutionReport(stateTransitionQA *StateTransitionQA, targetStateKeyVals statedb.StateKeyVals) *ExecutionReport {
	stf := stateTransitionQA.STF
	preState := statedb.StateKeyVals{
		KeyVals: stf.PreState.KeyVals,
	}
	postState := statedb.StateKeyVals{
		KeyVals: stf.PostState.KeyVals,
	}

	execReport := ExecutionReport{
		Block:           stf.Block,
		PreState:        preState,
		PostState:       postState,
		Seed:            fs.seed,
		TargetPostState: targetStateKeyVals,
	}
	if stateTransitionQA.Error != nil {
		execReport.Error = stateTransitionQA.Error.Error()
	}
	return &execReport
}

// Connect establishes the network connection to the target implementation.
func (f *Fuzzer) Connect() error {
	conn, err := net.Dial("unix", f.socketPath)
	if err != nil {
		return fmt.Errorf("failed to connect to target socket %s: %w", f.socketPath, err)
	}
	f.conn = conn
	return nil
}

// Close terminates the connection to the target.
func (f *Fuzzer) Close() error {
	if f.conn != nil {
		return f.conn.Close()
	}
	return nil
}

// SetSeed sets the seed for the fuzzer.
func (f *Fuzzer) SetSeed(seed []byte) {
	f.seed = seed
	log.Printf("Fuzzer seed set: 0x%x", f.seed)
}

func (f *Fuzzer) GetSeed() []byte {
	return f.seed
}

func (f *Fuzzer) GenSeed() []byte {
	// Generate a deterministic seed based on the previous seed.
	if f.seed == nil {
		f.seed = make([]byte, 32) // Default seed size
	} else {
		prevSeed := f.seed
		newSeed := common.ComputeHash(prevSeed)
		f.seed = newSeed
	}
	return f.seed
}

// --- Protocol Methods ---

func (f *Fuzzer) Handshake() (*PeerInfo, error) {
	log.Printf("[OUTGOING REQUEST ] PeerInfo - Fuzzer Info: %s", f.fuzzerInfo.Name)
	msgToSend := &Message{PeerInfo: &f.fuzzerInfo}
	if err := f.sendMessage(msgToSend); err != nil {
		return nil, err
	}
	receivedMsg, err := f.readMessage()
	if err != nil {
		return nil, err
	}
	if receivedMsg.PeerInfo == nil {
		return nil, fmt.Errorf("expected PeerInfo from target, but got a different message type")
	}
	log.Printf("[INCOMING RESPONSE] PeerInfo - Target Info: %s", receivedMsg.PeerInfo.Name)
	return receivedMsg.PeerInfo, nil
}

func (f *Fuzzer) SetState(state *HeaderWithState) (*common.Hash, error) {
	log.Printf("[OUTGOING REQUEST ] SetState - Header Hash: %s", state.Header.Hash().Hex())
	msgToSend := &Message{SetState: state}
	if err := f.sendMessage(msgToSend); err != nil {
		return nil, err
	}
	receivedMsg, err := f.readMessage()
	if err != nil {
		return nil, err
	}
	if receivedMsg.StateRoot == nil {
		return nil, fmt.Errorf("expected StateRoot, got different type")
	}
	log.Printf("[INCOMING RESPONSE] StateRoot - Hash: %s", receivedMsg.StateRoot.Hex())
	return receivedMsg.StateRoot, nil
}

func (f *Fuzzer) ImportBlock(block *types.Block) (*common.Hash, error) {
	log.Printf("[OUTGOING REQUEST ] ImportBlock - Hash: %s", block.Header.HeaderHash().Hex())
	msgToSend := &Message{ImportBlock: block}
	if err := f.sendMessage(msgToSend); err != nil {
		return nil, err
	}
	receivedMsg, err := f.readMessage()
	if err != nil {
		return nil, err
	}
	if receivedMsg.StateRoot == nil {
		return nil, fmt.Errorf("expected StateRoot, got different type")
	}
	log.Printf("[INCOMING RESPONSE] StateRoot - Hash: %s", receivedMsg.StateRoot.Hex())
	return receivedMsg.StateRoot, nil
}

func (f *Fuzzer) GetState(hash *common.Hash) (*statedb.StateKeyVals, error) {
	log.Printf("[OUTGOING REQUEST ] GetState - HeaderHash: %s", hash.Hex())
	msgToSend := &Message{GetState: hash}
	if err := f.sendMessage(msgToSend); err != nil {
		return nil, err
	}
	receivedMsg, err := f.readMessage()
	if err != nil {
		return nil, err
	}
	if receivedMsg.State == nil {
		return nil, fmt.Errorf("expected State, got different type")
	}
	log.Printf("[INCOMING RESPONSE] State - %d key-value pairs", len(receivedMsg.State.KeyVals))
	return receivedMsg.State, nil
}

// --- Network Helpers ---

func (f *Fuzzer) sendMessage(msg *Message) error {
	encodedBody, err := encode(msg)
	if err != nil {
		return fmt.Errorf("failed to encode message: %w", err)
	}
	msgLength := uint32(len(encodedBody))
	if err := binary.Write(f.conn, binary.LittleEndian, msgLength); err != nil {
		return fmt.Errorf("failed to write message length: %w", err)
	}
	if _, err := f.conn.Write(encodedBody); err != nil {
		return fmt.Errorf("failed to write message body: %w", err)
	}
	return nil
}

func (f *Fuzzer) readMessage() (*Message, error) {
	var msgLength uint32
	if err := binary.Read(f.conn, binary.LittleEndian, &msgLength); err != nil {
		if err == io.EOF {
			return nil, fmt.Errorf("connection closed by target")
		}
		return nil, fmt.Errorf("failed to read message length: %w", err)
	}
	body := make([]byte, msgLength)
	if _, err := io.ReadFull(f.conn, body); err != nil {
		return nil, fmt.Errorf("failed to read message body: %w", err)
	}
	return decode(body)
}
