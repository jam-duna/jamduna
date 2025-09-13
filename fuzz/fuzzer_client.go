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
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"
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
	if strings.ToLower(pvmBackend) != statedb.BackendInterpreter && strings.ToLower(pvmBackend) != statedb.BackendCompiler {
		pvmBackend = statedb.BackendInterpreter
		fmt.Printf("Invalid PVM backend specified. Defaulting to Interpreter\n")
	} else if runtime.GOOS != "linux" && strings.ToLower(pvmBackend) != statedb.BackendInterpreter {
		pvmBackend = statedb.BackendInterpreter
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
		return fmt.Errorf("%sfailed to connect to target socket %s: %w%s", common.ColorRed, f.socketPath, err, common.ColorReset)
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

func (f *Fuzzer) Handshake() (PeerInfo, error) {
	log.Printf("%s[OUTGOING REQ]%s PeerInfo - Fuzzer %s%s%s", common.ColorGreen, common.ColorReset, common.ColorYellow, f.fuzzerInfo.GetName(), common.ColorReset)
	//log.Printf("%sFuzzer Info: %s%s", common.ColorGray, f.fuzzerInfo.PrettyString(true), common.ColorReset)
	msgToSend := &Message{PeerInfo: f.fuzzerInfo}
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
	log.Printf("%s[INCOMING RSP]%s PeerInfo - Target %s%s%s", common.ColorBlue, common.ColorReset, common.ColorYellow, receivedMsg.PeerInfo.GetName(), common.ColorReset)
	log.Printf("%sTarget ↓ \n%s%s", common.ColorGray, receivedMsg.PeerInfo.PrettyString(true), common.ColorReset)

	// Validate FuzzVersion compatibility
	fuzzerVersion := f.fuzzerInfo.GetFuzzVersion()
	targetVersion := receivedMsg.PeerInfo.GetFuzzVersion()
	if fuzzerVersion != targetVersion {
		return nil, fmt.Errorf("FuzzVersion mismatch: fuzzer has version %d, target has version %d - terminating connection", fuzzerVersion, targetVersion)
	}
	//log.Printf("%sFuzzVersion validation passed: both fuzzer and target use version %d%s", common.ColorGreen, fuzzerVersion, common.ColorReset)

	f.targetInfo = receivedMsg.PeerInfo
	return receivedMsg.PeerInfo, nil
}

func (f *Fuzzer) SetState(state *HeaderWithState) (*common.Hash, error) {
	log.Printf("%s[OUTGOING REQ]%s SetState    - Hash: %s", common.ColorGreen, common.ColorReset, state.Header.Hash().Hex())
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
	log.Printf("%s[INCOMING RSP]%s StateRoot   - Hash: %s", common.ColorBlue, common.ColorReset, receivedMsg.StateRoot.Hex())
	return receivedMsg.StateRoot, nil
}

func (f *Fuzzer) Initialize(init *Initialize) (*common.Hash, error) {
	log.Printf("%s[OUTGOING REQ]%s Initialize  - Header: %s %s(AncestryLen:%d)%s", common.ColorGreen, common.ColorReset, init.Header.Hash().Hex(), common.ColorGray, len(init.Ancestry), common.ColorReset)
	msgToSend := &Message{Initialize: init}
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
	log.Printf("%s[INCOMING RSP]%s StateRoot   - Hash: %s", common.ColorBlue, common.ColorReset, receivedMsg.StateRoot.Hex())
	return receivedMsg.StateRoot, nil
}

func (f *Fuzzer) ImportBlock(block *types.Block) (*common.Hash, error) {
	log.Printf("%s[OUTGOING REQ]%s ImportBlock - Hash: %s", common.ColorGreen, common.ColorReset, block.Header.HeaderHash().Hex())
	msgToSend := &Message{ImportBlock: block}
	if err := f.sendMessage(msgToSend); err != nil {
		return nil, err
	}
	receivedMsg, err := f.readMessage()
	if err != nil {
		return nil, err
	}

	// Handle Error response (V1 protocol)
	if receivedMsg.Error != nil {
		log.Printf("%s[INCOMING RSP]%s Error - Rejected By Target%s", common.ColorBlue, common.ColorGray, common.ColorReset)
		return nil, fmt.Errorf("ImportBlock failed - target returned Error")
	}

	// Handle StateRoot response (success case or V0/V0r error with pre-state root)
	if receivedMsg.StateRoot == nil {
		return nil, fmt.Errorf("expected StateRoot or Error, got different type")
	}
	log.Printf("%s[INCOMING RSP]%s StateRoot   - Hash: %s", common.ColorBlue, common.ColorReset, receivedMsg.StateRoot.Hex())
	return receivedMsg.StateRoot, nil
}

func (f *Fuzzer) GetState(hash *common.Hash) (*statedb.StateKeyVals, error) {
	log.Printf("%s[OUTGOING REQ]%s GetState    - HeaderHash: %s", common.ColorGreen, common.ColorReset, hash.Hex())
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
	log.Printf("%s[INCOMING RSP]%s State - %d key-value pairs", common.ColorBlue, common.ColorReset, len(receivedMsg.State.KeyVals))
	return receivedMsg.State, nil
}

func (f *Fuzzer) RefineBundle(refineBundle *types.RefineBundle, expectedWorkReport *types.WorkReport) (*types.WorkReport, error) {
	workPackageHash := refineBundle.Bundle.PackageHash()
	log.Printf("%s[OUTGOING REQ]%s RefineBundle", common.ColorGreen, common.ColorReset)
	fmt.Printf("%s%v%s\n", common.ColorGray, refineBundle.StringL(), common.ColorReset)
	log.Printf("%sWorkPackageHash: %v%s\n", common.ColorGray, workPackageHash.Hex(), common.ColorReset)
	//fmt.Printf("%sOutgoing Bundle: %s%s", common.ColorGray, refineBundle.Bundle.StringL(), common.ColorReset)

	expectedReportHash := expectedWorkReport.Hash()
	log.Printf("%sExp. ReportHash: %s%s", common.ColorGray, expectedReportHash.Hex(), common.ColorReset)
	//fmt.Printf("%sExp WorkReport: %s%s\n", common.ColorGray, expectedWorkReport.String(), common.ColorReset)

	msgToSend := &Message{RefineBundle: refineBundle}
	if err := f.sendMessage(msgToSend); err != nil {
		return nil, err
	}
	receivedMsg, err := f.readMessage()
	if err != nil {
		return nil, err
	}
	if receivedMsg.WorkReport == nil {
		return nil, fmt.Errorf("expected WorkReport from target, but got a different message type")
	}

	// Check for mismatch
	actualReportHash := receivedMsg.WorkReport.Hash()
	log.Printf("%s[INCOMING RSP]%s WorkReport\n", common.ColorBlue, common.ColorReset)
	log.Printf("%sReceived WorkReportHash: %v %s", common.ColorGray, actualReportHash.Hex(), common.ColorReset)

	if expectedReportHash != actualReportHash {
		log.Printf("%sExp: %s%s\n", common.ColorRed, expectedReportHash.Hex(), common.ColorReset)
		log.Printf("%sGot:   %s%s\n", common.ColorRed, actualReportHash.Hex(), common.ColorReset)
		fmt.Printf("%sExp WorkReport: %s%s\n", common.ColorBrightRed, expectedWorkReport.String(), common.ColorReset)
		fmt.Printf("%sAct WorkReport: %s%s\n", common.ColorBrightRed, receivedMsg.WorkReport.String(), common.ColorReset)
		return nil, fmt.Errorf("WorkReport mismatch - expected %s, got %s", expectedReportHash.Hex(), actualReportHash.Hex())
	}
	return receivedMsg.WorkReport, nil
}

func (f *Fuzzer) GetExports(hash *common.Hash) (*[][]byte, error) {
	log.Printf("%s[OUTGOING REQ]%s GetExports  - Hash: %s", common.ColorGreen, common.ColorReset, hash.Hex())
	msgToSend := &Message{GetExports: hash}
	if err := f.sendMessage(msgToSend); err != nil {
		return nil, err
	}
	receivedMsg, err := f.readMessage()
	if err != nil {
		return nil, err
	}
	if receivedMsg.Segments == nil {
		return nil, fmt.Errorf("expected Segments, got different type")
	}
	log.Printf("%s[INCOMING RSP]%s Segments - %d segments", common.ColorBlue, common.ColorReset, len(*receivedMsg.Segments))
	for i, segment := range *receivedMsg.Segments {
		fmt.Printf("%sSegment %d - Hash: %s%s\n", common.ColorGray, i, common.Blake2Hash(segment), common.ColorReset)
	}
	return receivedMsg.Segments, nil
}

// --- Network Helpers ---

func (f *Fuzzer) sendMessage(msg *Message) error {
	encodedBody, err := GetProtocolHandler().Encode(msg)
	if err != nil {
		return fmt.Errorf("%sfailed to encode message: %w%s", common.ColorRed, err, common.ColorReset)
	}
	msgLength := uint32(len(encodedBody))
	if err := binary.Write(f.conn, binary.LittleEndian, msgLength); err != nil {
		return fmt.Errorf("%sfailed to write message length: %w%s", common.ColorRed, err, common.ColorReset)
	}
	if _, err := f.conn.Write(encodedBody); err != nil {
		return fmt.Errorf("%sfailed to write message body: %w%s", common.ColorRed, err, common.ColorReset)
	}
	return nil
}

func (f *Fuzzer) readMessage() (*Message, error) {
	var msgLength uint32
	if err := binary.Read(f.conn, binary.LittleEndian, &msgLength); err != nil {
		if err == io.EOF {
			return nil, fmt.Errorf("%sconnection closed by target%s", common.ColorRed, common.ColorReset)
		}
		return nil, fmt.Errorf("%sfailed to read message length: %w%s", common.ColorRed, err, common.ColorReset)
	}
	body := make([]byte, msgLength)
	if _, err := io.ReadFull(f.conn, body); err != nil {
		return nil, fmt.Errorf("%sfailed to read message body: %w%s", common.ColorRed, err, common.ColorReset)
	}

	msg, err := GetProtocolHandler().Decode(body)
	if err != nil {
		return nil, err
	}

	/*
		if len(body) > 0 {
			log.Printf("DEBUG: Received message with tag=%d, length=%d", body[0], msgLength)
		}
		if msg.Error != nil {
			log.Printf("DEBUG: Decoded as Error message")
		} else if msg.StateRoot != nil {
			log.Printf("DEBUG: Decoded as StateRoot message")
		}
	*/

	return msg, nil
}

// InitializeOrSetState chooses between Initialize (V1) or SetState (V0/V0r) based on protocol version
func (f *Fuzzer) InitializeOrSetState(state *HeaderWithState, ancestry []AncestryItem) (*common.Hash, error) {
	protocolVersion := GetProtocolHandler().GetProtocolVersion()

	if protocolVersion == ProtocolV1 {
		// V1 protocol always uses Initialize (with or without ancestry)
		initMsg := &Initialize{
			Header:   state.Header,
			KeyVals:  state.State,
			Ancestry: ancestry, // May be empty if ancestry not supported
		}
		return f.Initialize(initMsg)
	} else {
		// V0/V0r protocols use SetState
		return f.SetState(state)
	}
}

// Public wrappers for replay tool
func (f *Fuzzer) SendMessage(msg *Message) error {
	return f.sendMessage(msg)
}

func (f *Fuzzer) ReadMessage() (*Message, error) {
	return f.readMessage()
}
