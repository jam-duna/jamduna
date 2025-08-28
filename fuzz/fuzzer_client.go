package fuzz

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"runtime"
	"strings"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/pvm"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"
)

// ANSI color codes for logging to make output more readable.
const (
	colorReset       = "\033[0m"
	colorRed         = "\033[31m"
	colorGreen       = "\033[32m"
	colorBlue        = "\033[34m"
	colorYellow      = "\033[33m"
	colorMagenta     = "\033[35m"
	colorCyan        = "\033[36m"
	colorGray        = "\033[90m"
	colorBrightRed   = "\033[91m"
	colorBrightGreen = "\033[92m"
	colorBrightWhite = "\033[97m"
)

type Fuzzer struct {
	conn           net.Conn
	socketPath     string
	fuzzerInfo     PeerInfo
	targetInfo     PeerInfo
	seed           []byte
	store          *storage.StateDBStorage
	internalSTFMap map[common.Hash]*statedb.StateTransition
	pvmBackend     string
	reportDir      string
}

// NewFuzzer creates a new fuzzer instance.
func NewFuzzer(storageDir string, reportDir string, socketPath string, fuzzerInfo PeerInfo, pvmBackend string) (*Fuzzer, error) {
	sdbStorage, err := storage.NewStateDBStorage(storageDir)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize fuzzer's local storage: %w", err)
	}
	if strings.ToLower(pvmBackend) != pvm.BackendInterpreter && strings.ToLower(pvmBackend) != pvm.BackendCompiler {
		pvmBackend = pvm.BackendInterpreter
		fmt.Printf("Invalid PVM backend specified. Defaulting to Interpreter\n")
	} else if runtime.GOOS != "linux" && strings.ToLower(pvmBackend) != pvm.BackendInterpreter {
		pvmBackend = pvm.BackendInterpreter
		fmt.Printf("%v Not supported on %v Defaulting to Interpreter\n", pvmBackend, runtime.GOOS)
	}

	return &Fuzzer{
		socketPath:     socketPath,
		fuzzerInfo:     fuzzerInfo,
		store:          sdbStorage,
		reportDir:      reportDir,
		internalSTFMap: make(map[common.Hash]*statedb.StateTransition),
		pvmBackend:     pvmBackend,
	}, nil
}

// RunImplementationRPCServer starts the RPC server for the implementation.
func (fs *Fuzzer) RunImplementationRPCServer() {
	// runRPCServer(fs.store)
}

// RunInternalRPCServer starts the RPC server for the internal fuzzer logic.
func (fs *Fuzzer) RunInternalRPCServer() {
	// runRPCServerInternal(fs.seed, fs.store)
}

func (fs *Fuzzer) GenerateExecutionReport(stateTransitionQA *StateTransitionQA, targetStateKeyVals statedb.StateKeyVals, postStateRoot common.Hash, targetPostStateRoot common.Hash) *ExecutionReport {
	stf := stateTransitionQA.STF
	preState := statedb.StateKeyVals{
		KeyVals: stf.PreState.KeyVals,
	}
	postState := statedb.StateKeyVals{
		KeyVals: stf.PostState.KeyVals,
	}

	execReport := ExecutionReport{
		Block:               stf.Block,
		PostStateRoot:       postStateRoot,
		TargetPostStateRoot: targetPostStateRoot,
		PreState:            preState,
		PostState:           postState,
		Seed:                fs.seed,
		TargetPostState:     targetStateKeyVals,
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
		return fmt.Errorf("%sfailed to connect to target socket %s: %w%s", colorRed, f.socketPath, err, colorReset)
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

func (f *Fuzzer) SetTargetPeerInfo(targetPeerInfo PeerInfo) error {
	f.targetInfo = targetPeerInfo
	return nil
}

func (f *Fuzzer) GetTargetInfo() PeerInfo {
	return f.targetInfo
}

func (f *Fuzzer) GetFuzzerInfo() PeerInfo {
	return f.fuzzerInfo
}

// --- Protocol Methods ---

func (f *Fuzzer) Handshake() (*PeerInfo, error) {
	log.Printf("%s[OUTGOING REQ]%s PeerInfo - Fuzzer Info: %s", colorGreen, colorReset, f.fuzzerInfo.Name)
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
	log.Printf("%s[INCOMING RSP]%s PeerInfo - Target Info: %s", colorBlue, colorReset, receivedMsg.PeerInfo.Name)
	return receivedMsg.PeerInfo, nil
}

func (f *Fuzzer) SetState(state *HeaderWithState) (*common.Hash, error) {
	log.Printf("%s[OUTGOING REQ]%s SetState - Header Hash: %s", colorGreen, colorReset, state.Header.Hash().Hex())
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
	log.Printf("%s[INCOMING RSP]%s StateRoot - Hash: %s", colorBlue, colorReset, receivedMsg.StateRoot.Hex())
	return receivedMsg.StateRoot, nil
}

func (f *Fuzzer) ImportBlock(block *types.Block) (*common.Hash, error) {
	log.Printf("%s[OUTGOING REQ]%s ImportBlock - Hash: %s", colorGreen, colorReset, block.Header.HeaderHash().Hex())
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
	log.Printf("%s[INCOMING RSP]%s StateRoot - Hash: %s", colorBlue, colorReset, receivedMsg.StateRoot.Hex())
	return receivedMsg.StateRoot, nil
}

func (f *Fuzzer) GetState(hash *common.Hash) (*statedb.StateKeyVals, error) {
	log.Printf("%s[OUTGOING REQ]%s GetState - HeaderHash: %s", colorGreen, colorReset, hash.Hex())
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
	log.Printf("%s[INCOMING RSP]%s State - %d key-value pairs", colorBlue, colorReset, len(receivedMsg.State.KeyVals))
	return receivedMsg.State, nil
}

// --- Network Helpers ---

func (f *Fuzzer) sendMessage(msg *Message) error {
	encodedBody, err := encode(msg)
	if err != nil {
		return fmt.Errorf("%sfailed to encode message: %w%s", colorRed, err, colorReset)
	}
	msgLength := uint32(len(encodedBody))
	if err := binary.Write(f.conn, binary.LittleEndian, msgLength); err != nil {
		return fmt.Errorf("%sfailed to write message length: %w%s", colorRed, err, colorReset)
	}
	if _, err := f.conn.Write(encodedBody); err != nil {
		return fmt.Errorf("%sfailed to write message body: %w%s", colorRed, err, colorReset)
	}
	return nil
}

func (f *Fuzzer) readMessage() (*Message, error) {
	var msgLength uint32
	if err := binary.Read(f.conn, binary.LittleEndian, &msgLength); err != nil {
		if err == io.EOF {
			return nil, fmt.Errorf("%sconnection closed by target%s", colorRed, colorReset)
		}
		return nil, fmt.Errorf("%sfailed to read message length: %w%s", colorRed, err, colorReset)
	}
	body := make([]byte, msgLength)
	if _, err := io.ReadFull(f.conn, body); err != nil {
		return nil, fmt.Errorf("%sfailed to read message body: %w%s", colorRed, err, colorReset)
	}
	return decode(body)
}
