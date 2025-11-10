package node

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"reflect"
	"runtime"
	"strconv"

	_ "net/http/pprof"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb"
	types "github.com/colorfulnotion/jam/types"
)

type Jam struct {
	*NodeContent
	node JNode // Reference to the full node that implements JNode interface
}

var MethodDescriptionMap = map[string]string{
	"Functions":   "Functions() -> functions description",
	"NodeCommand": "NodeCommand(command string) -> will pass the command to the node",

	"Block":                "Block(headerHash hexstring) -> string",
	"BestBlock":            "BestBlock(headerHash hexstring) -> string",
	"FinalizedBlock":       "FinalizedBlock(headerHash hexstring) -> string",
	"LatestFinalizedBlock": "LatestFinalizedBlock() -> string",
	"Parent":               "Parent(headerHash hexstring) -> string",
	"StateRoot":            "StateRoot(headerHash hexstring) -> string",
	"BeefyRoot":            "BeefyRoot(headerHash hexstring) -> string",
	"State":                "State(headerHash hexstring) -> string",
	"Statistics":           "Statistics(headerHash hexstring) -> string",
	"ServiceInfo":          "ServiceInfo(serviceIndex string) -> string",
	"ServicePreimage":      "ServicePreimage(serviceIndex string, preimage hexstring) -> hexstring",
	"ServiceRequest":       "ServiceRequest(serviceIndex string, preimage hexstring, length string) -> json string",
	"SubmitPreimage":       "SubmitPreimage(serviceIndex string, preimage hexstring) -> string",
	"ServiceValue":         "ServiceValue(serviceIndex string, key hexstring) -> hexstring",
	"WorkPackage":          "WorkPackage(workPackageHash string) -> json WorkReport",
	"Code":                 "Code(serviceIndex string) -> json string",
	"ListServices":         "ListServices() -> json string",
	"AuditWorkPackage":     "AuditWorkPackage(workPackageHash string) -> json WorkReport",

	"Encode": "Encode(objectType string, input string) -> hexstring",
	"Decode": "Decode(objectType string, input string) -> json string",

	// Ethereum JSON-RPC methods
	// node_rpc_evmnetwork.go - Network/Metadata
	"ChainId":     "ChainId() -> chain ID hex",
	"Accounts":    "Accounts() -> account addresses array",
	"GasPrice":    "GasPrice() -> gas price hex",
	"EstimateGas": "EstimateGas(txObj json, blockNumber string) -> gas estimate hex",
	"GetCode":     "GetCode(address string, blockNumber string) -> bytecode hex",

	// node_rpc_evmcontracts.go - State Access
	"GetBalance":          "GetBalance(address string, blockNumber string) -> uint256 hex",
	"GetStorageAt":        "GetStorageAt(address string, position string, blockNumber string) -> value hex",
	"GetTransactionCount": "GetTransactionCount(address string, blockNumber string) -> uint256 hex",

	// node_rpc_evmtx.go - Transaction
	"GetTransactionReceipt":               "GetTransactionReceipt(txHash string) -> receipt json",
	"GetTransactionByHash":                "GetTransactionByHash(txHash string) -> transaction json",
	"GetTransactionByBlockHashAndIndex":   "GetTransactionByBlockHashAndIndex(blockHash string, index string) -> transaction json",
	"GetTransactionByBlockNumberAndIndex": "GetTransactionByBlockNumberAndIndex(blockNumber string, index string) -> transaction json",
	"GetLogs":                             "GetLogs(filter json) -> logs json array",
	"SendRawTransaction":                  "SendRawTransaction(signedTxData hex) -> txHash",
	"Call":                                "Call(txObj json, blockNumber string) -> data hex",

	// node_rpc_evmblock.go - Block + Transaction pool management methods
	"BlockNumber":      "BlockNumber() -> block number hex",
	"GetBlockByHash":   "GetBlockByHash(blockHash string, fullTx bool) -> block json",
	"GetBlockByNumber": "GetBlockByNumber(blockNumber string, fullTx bool) -> block json",
	"TxPoolStatus":     "TxPoolStatus() -> pool statistics json",
	"TxPoolContent":    "TxPoolContent() -> pending and queued transactions json",
	"TxPoolInspect":    "TxPoolInspect() -> human readable pool summary",
}

type NodeStatusServer struct {
	Host             string           `json:"host"`
	IsSync           bool             `json:"is_sync"`
	IsAuditing       bool             `json:"is_auditing"`
	IsTicketSending  bool             `json:"is_ticket_sending"`
	AuthoringStatus  string           `json:"authoring_status"`
	CurrentBlockInfo JAMSNP_BlockInfo `json:"current_block_info"`
}

func (j *Jam) Functions(req []string, res *string) error {
	*res = ""
	maxKeyLen := 0
	for k := range MethodDescriptionMap {
		if len(k) > maxKeyLen {
			maxKeyLen = len(k)
		}
	}
	format := fmt.Sprintf("%%-%ds: %%s\n", maxKeyLen)
	for k, v := range MethodDescriptionMap {
		*res += fmt.Sprintf(format, k, v)
	}
	return nil
}

func (j *Jam) NodeCommand(req []string, res *string) error {
	command := req[0]
	switch command {
	case "SetFlag":
		flag := req[1]
		value := req[2]
		flagValue, err := strconv.ParseBool(value)
		if err != nil {
			*res = fmt.Sprintf("Invalid value for flag %s: %s", flag, value)
			return err
		}
		switch flag {
		case "audit":
			j.nodeSelf.AuditFlag = flagValue
		case "ticket_send":
			j.nodeSelf.SetSendTickets(flagValue)
		default:
			*res = fmt.Sprintf("Unknown flag %s", flag)
		}
	case "StackTrace":
		debugtrace := make([]byte, 1<<20)
		runtime.Stack(debugtrace, true)
		*res = string(debugtrace)
		return nil
	case "GetNodeStatus":
		host_name, err := os.Hostname()
		if err != nil {
			*res = fmt.Sprintf("Error getting hostname: %s", err)
			return err
		}
		j.nodeSelf.latest_block_mutex.Lock()
		nodeStatus := NodeStatusServer{
			Host:             host_name,
			IsSync:           j.nodeSelf.GetIsSync(),
			IsAuditing:       j.nodeSelf.AuditFlag,
			IsTicketSending:  j.nodeSelf.sendTickets,
			AuthoringStatus:  j.nodeSelf.author_status,
			CurrentBlockInfo: *j.nodeSelf.latest_block,
		}
		j.nodeSelf.latest_block_mutex.Unlock()
		nodeStatusJson, err := json.Marshal(nodeStatus)
		if err != nil {
			*res = fmt.Sprintf("error marshalling node status: %s", err)
		}
		*res = string(nodeStatusJson)
	case "GetConnections":
		for _, peer := range j.nodeSelf.peersInfo {
			peer.connectionMu.Lock()
			defer peer.connectionMu.Unlock()
			if peer.conn != nil {
				*res += fmt.Sprintf("Peer ID: %d, peerAddress :%s\n", peer.PeerID, peer.PeerAddr)
			} else {
				*res += fmt.Sprintf("Peer ID: %d, peerAddress :%s (not connected)\n", peer.PeerID, peer.PeerAddr)
			}
		}

	default:
		*res = fmt.Sprintf("unknown command %s", command)
		return fmt.Errorf("unknown command %s", command)
	}
	return nil
}

func (j *Jam) GetAvailabilityAssignments(req []string, res *string) error {
	if len(req) != 1 {
		return fmt.Errorf("invalid number of arguments")
	}
	coreIdxStr := req[0]
	codeIdx, err := strconv.ParseUint(coreIdxStr, 10, 32)
	if err != nil {
		return err
	}
	CoreState := j.statedb.JamState.AvailabilityAssignments[codeIdx]
	availability_assignment_str := CoreState.String()
	//fmt.Printf("JAM SERVER GetAvailabilityAssignments @ coreIdx=%d availability_assignment=%v\n", codeIdx, availability_assignment_str)
	*res = availability_assignment_str
	return nil
}

func (j *Jam) GetBuildVersion(req []string, res *string) error {
	commitHash := j.NodeContent.nodeSelf.GetBuild()
	*res = commitHash
	return nil
}

func (j *Jam) GetCurrJCE(req []string, res *string) error {
	currJCE := j.NodeContent.nodeSelf.GetCurrJCE()
	*res = fmt.Sprintf("%d", currJCE)
	return nil
}

// Returns the header hash and slot of the parent of the block with the given header hash, or null if this is not known.
func (j *Jam) Parent(req []string, res *string) error {
	if len(req) != 1 {
		return fmt.Errorf("invalid number of arguments: expected 1, got %d", len(req))
	}

	input := req[0]
	headerHash := common.HexToHash(input)

	block, err := j.NodeContent.GetBlockByHeaderHash(headerHash)
	if err != nil {
		return fmt.Errorf("failed to get block by header hash %s: %w", headerHash.String(), err)
	}

	parentHeaderHash := block.Header.ParentHeaderHash
	parentBlock, err := j.NodeContent.GetBlockByHeaderHash(parentHeaderHash)
	if err != nil {
		return fmt.Errorf("failed to get parent of header hash %s: %w", headerHash.String(), err)
	}
	parentBlockSlot := parentBlock.Header.Slot

	type getParentResponse struct {
		ParentHeaderHash common.Hash `json:"parent_header_hash"`
		ParentBlockSlot  uint32      `json:"parent_block_slot"`
	}
	response := getParentResponse{
		ParentHeaderHash: parentHeaderHash,
		ParentBlockSlot:  parentBlockSlot,
	}
	resp, err := json.Marshal(response)
	if err != nil {
		return err
	}

	*res = string(resp)
	return nil
}

// Returns the posterior state root of the block with the given header hash, or null if this is not known.
func (j *Jam) StateRoot(req []string, res *string) error {
	if len(req) != 1 {
		return fmt.Errorf("invalid number of arguments: expected 1, got %d", len(req))
	}

	input := req[0]
	headerHash := common.HexToHash(input)

	sdb, ok := j.statedbMap[headerHash]
	if ok {
		*res = sdb.StateRoot.String()
		return nil
	}
	return fmt.Errorf("unknown header hash %s", headerHash)
}

// Returns the BEEFY root of the block with the given header hash, or null if this is not known.
func (j *Jam) BeefyRoot(req []string, res *string) error {
	if len(req) != 1 {
		return fmt.Errorf("invalid number of arguments: expected 1, got %d", len(req))
	}

	input := req[0]
	headerHash := common.HexToHash(input)

	sdb, ok := j.getStateDBByHeaderHash(headerHash)
	if !ok {
		return fmt.Errorf("state not found for header hash %s", headerHash.String())
	}

	recentBlocks := sdb.JamState.Snapshot(&statedb.StateSnapshotRaw{}, nil).RecentBlocks.B_H
	if len(recentBlocks) > 0 {
		*res = recentBlocks[len(recentBlocks)-1].String()
		return nil
	}
	return fmt.Errorf("no recent blocks")
}

func (j *Jam) Block(req []string, res *string) error {
	if len(req) != 1 {
		return fmt.Errorf("invalid number of arguments: expected 1, got %d", len(req))
	}

	input := req[0]
	var block *types.SBlock // Replace 'Block' with the actual type returned by your methods.
	var err error

	switch input {
	case "latest":
		slot := j.NodeContent.getLatestFinalizedBlockSlot()
		block, err = j.NodeContent.GetStoredBlockBySlot(slot)
		if err != nil {
			return fmt.Errorf("failed to get latest block by slot %d: %w", slot, err)
		}
	case "best":
		slot := j.NodeContent.getBestBlockSlot()
		block, err = j.NodeContent.GetStoredBlockBySlot(slot)
		if err != nil {
			return fmt.Errorf("failed to get best block by slot %d: %w", slot, err)
		}
	default:
		if len(input) < 20 {
			var slot uint32
			parsed, err := strconv.ParseUint(input, 10, 32)
			if err != nil {
				return fmt.Errorf("invalid slot value %q: %w", input, err)
			}
			slot = uint32(parsed)
			block, err = j.NodeContent.GetStoredBlockBySlot(slot)
			if err != nil {
				return fmt.Errorf("failed to get block by slot %s: %w", input, err)
			}
		} else {
			headerHash := common.HexToHash(input)
			block, err = j.NodeContent.GetBlockByHeaderHash(headerHash)
			if err != nil {
				return fmt.Errorf("failed to get block by header hash %s: %w", headerHash.String(), err)
			}
		}
	}

	*res = block.String()
	return nil
}

func (j *Jam) GetCoreCoWorkersPeers(req []string, res *string) (err error) {
	if len(req) != 1 {
		return fmt.Errorf("invalid number of arguments: expected 1, got %d", len(req))
	}

	parsed, err := strconv.ParseUint(req[0], 10, 32)
	if err != nil {
		return fmt.Errorf("invalid value %q: %w", req[0], err)
	}
	coreIndex := uint16(parsed)
	peers := j.NodeContent.GetCoreCoWorkersPeers(coreIndex)

	peerIDs := make([]uint16, len(peers))
	for i := range peers {
		peerIDs[i] = peers[i].PeerID
	}

	jsonstr, err := json.Marshal(peerIDs)
	if err != nil {
		return fmt.Errorf("json.Marshal failed:%v", err)
	}
	*res = string(jsonstr)
	return nil
}

// jam.FinalizedBlock returns the header hash of the latest finalized block.
func (j *Jam) FinalizedBlock(req []string, res *string) error {
	if len(req) != 1 {
		return fmt.Errorf("invalid number of arguments: expected 1, got %d", len(req))
	}

	var block *types.SBlock // Replace 'Block' with the actual type returned by your methods.
	var err error
	slot := j.NodeContent.getBestBlockSlot()
	block, err = j.NodeContent.GetStoredBlockBySlot(slot)
	if err != nil {
		return fmt.Errorf("failed to get best block by slot %d: %w", slot, err)
	}

	*res = block.Header.Hash().String()
	return nil
}

// jam.LatestFinalizedBlock
func (j *Jam) LatestFinalizedBlock(req []string, res *string) error {

	var block *types.Block // Replace 'Block' with the actual type returned by your methods.
	var err error

	block, err = j.NodeContent.GetFinalizedBlock()
	if err != nil {
		return fmt.Errorf("failed to get finalized block: %w", err)
	}
	*res = block.String()
	return nil
}

func (j *Jam) BestBlock(req []string, res *string) error {
	if len(req) != 1 {
		return fmt.Errorf("invalid number of arguments: expected 1, got %d", len(req))
	}

	var block *types.SBlock // Replace 'Block' with the actual type returned by your methods.
	var err error

	slot := j.NodeContent.getBestBlockSlot()
	block, err = j.NodeContent.GetStoredBlockBySlot(slot)
	if err != nil {
		return fmt.Errorf("failed to get best block by slot %d: %w", slot, err)
	}

	*res = block.Header.Hash().String()
	return nil
}

// see GP 11.1.2 Refinement Context where there TWO historical blocks A+B but only A has to be in RecentBlocks
func (n *NodeContent) getRefineContext(prereqs ...common.Hash) types.RefineContext {
	// TODO: approx finality by 5 blocks
	finalityApproxConst := 5
	anchor := common.Hash{}
	stateRoot := common.Hash{}
	beefyRoot := common.Hash{}
	s := n.statedb
	if len(s.JamState.RecentBlocks.B_H) > finalityApproxConst {
		idx := len(s.JamState.RecentBlocks.B_H) - finalityApproxConst
		anchorBlock := s.JamState.RecentBlocks.B_H[idx]
		anchor = anchorBlock.HeaderHash   // header hash a must be in s.JamState.RecentBlocks
		stateRoot = anchorBlock.StateRoot // state root s must be in s.JamState.RecentBlocks
		beefyRoot = anchorBlock.B         // beefy root b must be in s.JamState.RecentBlocks
	}
	sb, err := n.GetBlockByHeaderHash(anchor)
	if err != nil {
		log.Error(log.Node, "getRefineContext", "error", err, "anchor", anchor.String())
		return types.RefineContext{}
	}

	// B) LOOKUP ANCHOR
	return types.RefineContext{
		// A) ANCHOR
		Anchor:    anchor,
		StateRoot: stateRoot,
		BeefyRoot: beefyRoot,
		// B) LOOKUP ANCHOR
		LookupAnchor:     anchor,
		LookupAnchorSlot: sb.Header.Slot,
		Prerequisites:    prereqs,
	}
}

func (n *NodeContent) getLatestFinalizedBlockSlot() uint32 {
	n.statedbMutex.Lock()
	defer n.statedbMutex.Unlock()
	return n.statedb.GetTimeslot()
}

func (n *NodeContent) getBestBlockSlot() uint32 {
	return n.getLatestFinalizedBlockSlot()
}

func (j *Jam) State(req []string, res *string) error {
	if len(req) != 1 {
		return fmt.Errorf("invalid number of arguments: expected 1, got %d", len(req))
	}

	input := req[0]
	var headerHash common.Hash

	switch input {
	case "latest":
		slot := j.NodeContent.getLatestFinalizedBlockSlot()
		block, err := j.NodeContent.GetStoredBlockBySlot(slot)
		if err != nil {
			return fmt.Errorf("failed to get latest block by slot %d: %w", slot, err)
		}
		headerHash = block.Header.Hash()
	case "best":
		slot := j.NodeContent.getBestBlockSlot()
		block, err := j.NodeContent.GetStoredBlockBySlot(slot)
		if err != nil {
			return fmt.Errorf("failed to get best block by slot %d: %w", slot, err)
		}
		headerHash = block.Header.Hash()
	default:
		headerHash = common.HexToHash(input)
	}

	sdb, ok := j.getStateDBByHeaderHash(headerHash)
	if !ok {
		return fmt.Errorf("state not found for header hash %s", headerHash.String())
	}
	*res = sdb.JamState.Snapshot(&statedb.StateSnapshotRaw{}, sdb.GetStateUpdates()).String()
	return nil
}

func (j *Jam) Statistics(req []string, res *string) error {
	if len(req) != 1 {
		return fmt.Errorf("invalid number of arguments: expected 1, got %d", len(req))
	}

	input := req[0]
	var headerHash common.Hash

	switch input {
	case "latest":
		slot := j.NodeContent.getLatestFinalizedBlockSlot()
		block, err := j.NodeContent.GetStoredBlockBySlot(slot)
		if err != nil {
			return fmt.Errorf("failed to get latest block by slot %d: %w", slot, err)
		}
		headerHash = block.Header.Hash()
	case "best":
		slot := j.NodeContent.getBestBlockSlot()
		block, err := j.NodeContent.GetStoredBlockBySlot(slot)
		if err != nil {
			return fmt.Errorf("failed to get best block by slot %d: %w", slot, err)
		}
		headerHash = block.Header.Hash()
	default:
		headerHash = common.HexToHash(input)
	}

	sdb, ok := j.getStateDBByHeaderHash(headerHash)
	if !ok {
		return fmt.Errorf("state not found for header hash %s", headerHash.String())
	}
	*res = sdb.JamState.Snapshot(&statedb.StateSnapshotRaw{}, nil).ValidatorStatistics.String()
	return nil
}

func (j *Jam) GetLatestState(req []string, res *string) error {
	if len(req) != 0 {
		return fmt.Errorf("invalid number of arguments")
	}
	sdb := j.statedb
	*res = sdb.JamState.Snapshot(&statedb.StateSnapshotRaw{}, sdb.GetStateUpdates()).String()
	return nil
}

func (j *Jam) ServiceInfo(req []string, res *string) error {
	if len(req) != 1 {
		return fmt.Errorf("invalid number of arguments")
	}
	serviceIndexStr := req[0]
	serviceIndex, err := strconv.ParseUint(serviceIndexStr, 10, 32)
	if err != nil {
		return err
	}
	service, ok, err := j.statedb.GetService(uint32(serviceIndex))
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("service not found %d", serviceIndex)
	}
	*res = service.JsonString()
	return nil
}

func (j *Jam) Code(req []string, res *string) error {
	if len(req) != 1 {
		return fmt.Errorf("invalid number of arguments")
	}
	serviceIndexStr := req[0]
	serviceIndex, err := strconv.ParseUint(serviceIndexStr, 10, 32)
	if err != nil {
		return err
	}
	service, ok, err := j.statedb.GetService(uint32(serviceIndex))
	if err != nil || !ok {
		return fmt.Errorf("GetService failed:%v", err)
	}
	code_hash := service.CodeHash
	preimage, ok, err := j.statedb.ReadServicePreimageBlob(uint32(serviceIndex), code_hash)
	if err != nil || !ok {
		return fmt.Errorf("ReadServicePreimageBlob failed:%v", err)
	}

	type serviceCodeResponse struct {
		CodeHash string `json:"code_hash"`
		Metadata string `json:"metadata"`
		Code     string `json:"rawbytes"`
		Length   uint32 `json:"length"`
	}
	metadata, rawBytes := types.SplitMetadataAndCode(preimage)
	length := uint32(len(rawBytes))
	service_code_response := serviceCodeResponse{
		Metadata: metadata,
		Code:     common.Bytes2Hex(rawBytes),
		CodeHash: common.Bytes2Hex(code_hash.Bytes()),
		Length:   length,
	}
	service_code_response_json, err := json.Marshal(service_code_response)
	if err != nil {
		return fmt.Errorf("json.Marshal failed:%v", err)
	}
	*res = string(service_code_response_json)

	return nil
}

func (j *Jam) ServicePreimage(req []string, res *string) error {
	if len(req) != 2 {
		return fmt.Errorf("invalid number of arguments")
	}
	serviceIndexStr := req[0]
	codeHashStr := req[1]
	serviceIndex, err := strconv.ParseUint(serviceIndexStr, 10, 32)
	if err != nil {
		return err
	}
	codeHash := common.HexToHash(codeHashStr)
	preimage, ok, err := j.statedb.ReadServicePreimageBlob(uint32(serviceIndex), codeHash)
	if err != nil || !ok {
		return err
	}
	metadata, rawBytes := types.SplitMetadataAndCode(preimage)
	length := uint32(len(preimage))

	type servicePreimageResponse struct {
		Metadata string `json:"metadata"`
		RawBytes string `json:"rawbytes"`
		Length   uint32 `json:"length"`
	}
	response := servicePreimageResponse{
		Metadata: metadata,
		RawBytes: common.Bytes2Hex(rawBytes),
		Length:   length,
	}
	r, err := json.Marshal(response)
	if err != nil {
		return err
	}
	*res = string(r)
	return nil
}

// req = [serviceIndex, preimage hash]
func (j *Jam) ServiceRequest(req []string, res *string) error {
	if len(req) != 3 {
		return fmt.Errorf("invalid number of arguments")
	}
	serviceIndexStr := req[0]
	codeHashStr := req[1]
	lengthStr := req[2]
	serviceIndex, err := strconv.ParseUint(serviceIndexStr, 10, 32)
	if err != nil {
		return err
	}
	codeHash := common.HexToHash(codeHashStr)
	length, err := strconv.ParseUint(lengthStr, 10, 32)
	if err != nil {
		*res = err.Error()
		return err
	}
	lookup, ok, err := j.statedb.ReadServicePreimageLookup(uint32(serviceIndex), codeHash, uint32(length))
	if err != nil || !ok {
		return fmt.Errorf("ReadServicePreimageLookup failed:%v", err)
	}

	// encode to json
	lookupJson, err := json.Marshal(lookup)
	if err != nil {
		return fmt.Errorf("json.Marshal failed:%v", err)
	}
	*res = string(lookupJson)
	return nil
}

// req = [serviceIndex, preimage hash]
func (j *Jam) ServiceValue(req []string, res *string) error {
	if len(req) != 2 {
		return fmt.Errorf("invalid number of arguments")
	}
	serviceIndexStr := req[0]
	storage_hashStr := req[1]
	serviceIndex, err := strconv.ParseUint(serviceIndexStr, 10, 32)
	if err != nil {
		return err
	}
	//storage_hash := common.HexToHash(storage_hashStr)
	storage_byte := common.FromHex(storage_hashStr)
	storage, ok, err := j.statedb.ReadServiceStorage(uint32(serviceIndex), storage_byte)
	if err != nil {
		return fmt.Errorf("ReadServiceStorage failed (serviceID=%d, h=%s) %v", serviceIndex, storage_hashStr, err)
	}
	if !ok {
		return fmt.Errorf("ReadServiceStorage not found (serviceID=%d, h=%s)", serviceIndex, storage_hashStr)
	}
	*res = common.Bytes2Hex(storage)
	return nil
}

// GetWorkPackageByHash(workPackageHash string) -> json WorkReport
func (j *Jam) WorkPackage(req []string, res *string) error {
	if len(req) != 1 {
		return fmt.Errorf("invalid number of arguments")
	}
	workPackageHash := common.HexToHash(req[0])
	si := j.WorkReportSearch(workPackageHash)
	if si == nil {
		return fmt.Errorf("work Package not found")
	}

	workReport := si.WorkReport
	*res = workReport.String()
	return nil
}

func (j *Jam) TraceBlock(req []string, res *string) error {
	if len(req) != 1 {
		return fmt.Errorf("invalid number of arguments")
	}
	headerHash := common.HexToHash(req[0])
	sblk, err := j.NodeContent.GetBlockByHeaderHash(headerHash)
	if err != nil {
		return fmt.Errorf("failed to get block by header hash %s: %w", headerHash.String(), err)
	}
	log.RecordLogs()
	log.EnableModule(log.PvmAuthoring)
	log.EnableModule(log.GeneralAuthoring)

	block := &types.Block{
		Header:    sblk.Header,
		Extrinsic: sblk.Extrinsic,
	}
	sdb, err := statedb.NewStateDBFromBlock(j.NodeContent.store, block)
	if err != nil {
		return fmt.Errorf("NewStateDBBlock %s %s", headerHash.String(), err)
	}

	sdb.Id = block.Header.AuthorIndex

	ctx, cancel := context.WithTimeout(context.Background(), MediumTimeout)
	defer cancel()
	s1, err := statedb.ApplyStateTransitionFromBlock(0, sdb, ctx, block, nil, statedb.BackendInterpreter)
	if err != nil {
		log.Error(log.Node, "TraceBlock", "err", err)
		return err
	}
	logs, err := log.GetRecordedLogs()
	if err != nil {
		log.Error(log.Node, "TraceBlock", "block", block.String(), "len(logs)", len(logs), "err", err)
		return err
	}
	log.Info(log.Node, "TraceBlock", "block", block.String(), "len(logs)", len(logs), "s1.StateRoot", s1.StateRoot)
	*res = string(logs)
	return nil
}

// AuditWorkPackageByHash(workPackageHash string) -> json WorkReport
func (j *Jam) AuditWorkPackage(req []string, res *string) error {
	if len(req) != 1 {
		return fmt.Errorf("invalid number of arguments")
	}
	workPackageHash := common.HexToHash(req[0])
	si := j.WorkReportSearch(workPackageHash)
	if si == nil {
		return fmt.Errorf("work Package not found")
	}
	workReport := si.WorkReport
	spec := workReport.AvailabilitySpec
	// now call C138 to get bundle_shard from C assurers, do ec reconstruction for b
	// IMPORTANT: within reconstructPackageBundleSegments is a call to VerifyBundle
	workPackageBundle, err := j.reconstructPackageBundleSegments(spec, workReport.SegmentRootLookup, workReport.CoreIndex)
	if err != nil {
		return err
	}
	log.RecordLogs()
	log.EnableModule(log.FirstGuarantorOrAuditor)

	workReport2, err := j.executeWorkPackageBundle(uint16(workReport.CoreIndex), workPackageBundle, workReport.SegmentRootLookup, j.statedb.GetTimeslot(), true, 0)
	if err != nil {
		return err
	}

	// check that workReport == workReport2
	if workReport.Hash() == workReport2.Hash() {
		log.Info(log.Node, "AuditWorkPackage", "auditResult", workReport.Hash() == workReport2.Hash(), "workReport", workReport)
	} else {
		log.Error(log.Node, "AuditWorkPackage", "auditResult", workReport.Hash() == workReport2.Hash(), "workReport", workReport)
	}
	logs, err := log.GetRecordedLogs()
	if err != nil {
		return err
	}
	*res = string(logs)
	return nil
}

func (j *Jam) SubmitPreimage(req []string, res *string) error {
	if len(req) != 2 {
		return fmt.Errorf("invalid number of arguments")
	}
	serviceIndexStr := req[0]
	preimageStr := req[1]
	serviceIndex, err := strconv.ParseUint(serviceIndexStr, 10, 32)
	if err != nil {
		return err
	}
	preimage := common.Hex2Bytes(preimageStr)
	preimage_length := uint32(len(preimage))
	service_index := uint32(serviceIndex)
	preimage_hash := common.Blake2Hash(preimage)
	log.Info(log.Node, "SubmitPreimage", "service", service_index, "preimage", common.Blake2Hash(preimage), "len", preimage_length)

	// Add it to your own pool
	j.AddPreimageToPool(service_index, preimage)

	// Announce it everyone else with CE142 (and they will request it with CE143, which will be available in the pool from the above)
	err = j.BroadcastPreimageAnnouncement(service_index, preimage_hash, preimage_length, preimage)
	if err != nil {
		log.Error(log.Node, "SubmitPreimage ERR2", "err", err)
		*res = err.Error()
		return err
	}

	*res = "OK"
	return nil
}

// req1= mode like: "latest","genesis", or blockhash hexstring, req2= number of blocks, req3 = direction
func (j *Jam) FetchBlocks(req []string, res *string) error {
	if len(req) == 2 {
		mode := req[0]
		num, err := strconv.Atoi(req[1])
		if err != nil {
			return fmt.Errorf("invalid number of arguments")
		}
		switch mode {
		case "genesis":
			// get number from req

			// get the latest block
			blocks, err := j.nodeSelf.fetchBlocks(genesisBlockHash, 0, uint32(num))
			if err != nil {
				*res = fmt.Sprintf("block not found err=%v", err)
			}
			// convert to json
			*res = types.ToJSON(blocks)
			return nil
		case "latest":
			// get the latest blockhash
			latestBlockHash := j.statedb.HeaderHash
			blocks, err := j.nodeSelf.fetchBlocks(latestBlockHash, 1, uint32(num))
			if err != nil {
				*res = fmt.Sprintf("block not found err=%v", err)
			}
			// convert to json
			*res = types.ToJSON(blocks)
			return nil
		default:
			*res = fmt.Sprintf("Invalid mode %s", mode)
		}
	} else if len(req) == 3 {
		blockhashhex := req[0]
		block_hash := common.HexToHash(blockhashhex)
		num, err := strconv.Atoi(req[1])
		if err != nil {
			return fmt.Errorf("invalid number of arguments")
		}
		direction := req[2]
		direction_num, err := strconv.Atoi(direction)
		if err != nil {
			return fmt.Errorf("invalid number of arguments")
		}
		blocks, err := j.nodeSelf.fetchBlocks(block_hash, uint8(direction_num), uint32(num))
		if err != nil {
			*res = fmt.Sprintf("block not found err=%v", err)
		}
		// convert to json
		*res = types.ToJSON(blocks)
		return nil
	}
	*res = "Invalid Request"
	return fmt.Errorf("invalid Request")
}

func (j *Jam) SubmitWorkPackage(req []string, res *string) error {
	if len(req) != 1 {
		log.Info(log.Node, "SubmitWorkPackage error", "err", req)
		return fmt.Errorf("invalid number of arguments")
	}
	var newReq WorkPackageRequest
	if err := json.Unmarshal([]byte(req[0]), &newReq); err != nil {
		log.Error(log.Node, "SubmitWorkPackage", "err", err)
		return fmt.Errorf("failed to decode WorkPackageRequest: %w", err)
	}
	j.NodeContent.SubmitWPSameCore(newReq.WorkPackage, newReq.ExtrinsicsBlobs)
	*res = "OK"
	return nil
}

func (j *Jam) WorkReport(req []string, res *string) error {
	fmt.Printf("jam.WorkReport called with req=%v\n", req)
	if len(req) != 1 {
		return fmt.Errorf("invalid number of arguments")
	}

	requestedHashStr := req[0]
	requestedHash := common.HexToHash(requestedHashStr)
	// Access statedb via Node reference
	wr, err := j.nodeSelf.GetWorkReport(requestedHash)
	if err != nil {
		return fmt.Errorf("failed to get work report: %w", err)
	}

	// json marshal the work report
	*res = wr.String()
	return nil
}

func (j *Jam) ListServices(req []string, res *string) error {
	if len(req) != 0 {
		return fmt.Errorf("invalid number of arguments")
	}

	j.servicesMutex.Lock()
	knownServices := make([]*types.ServiceSummary, 0)
	for _, si := range j.servicesMap {
		knownServices = append(knownServices, si)
	}
	j.servicesMutex.Unlock()
	s, err := json.MarshalIndent(knownServices, "", "    ")
	if err != nil {
		return err
	}
	*res = string(s)
	return nil
}

// encoded type, and input
func (j *Jam) Encode(req []string, res *string) error {
	// use encodeapi to encode the input
	if len(req) != 2 {
		return fmt.Errorf("invalid number of arguments")
	}
	objectType := req[0]
	input := req[1]
	encoded, err := encodeapi(objectType, input)
	if err != nil {
		*res = err.Error()
		return err
	}
	*res = encoded
	return nil
}

func (j *Jam) Decode(req []string, res *string) error {
	// use decodeapi to decode the input
	if len(req) != 2 {
		return fmt.Errorf("invalid number of arguments")
	}
	objectType := req[0]
	input := req[1]
	decoded, err := decodeapi(objectType, input)
	if err != nil {
		*res = err.Error()
		return err
	}
	*res = decoded
	return nil
}

// server ========================================
func (n *Node) StartRPCServer(validatorIndex int) {
	n.NodeContent.startRPCServerImpl(validatorIndex, n)
}

func (n *NodeContent) startRPCServerImpl(validatorIndex int, node JNode) {
	jam := new(Jam)
	jam.NodeContent = n
	jam.node = node
	// register the rpc methods
	rpc.RegisterName("jam", jam)
	// register ethereum rpc methods with eth_ prefix
	rpc.RegisterName("eth", jam)

	// Start TCP RPC server
	tcpAddress := fmt.Sprintf(":%d", DefaultTCPPort+validatorIndex)
	listener, err := net.Listen("tcp", tcpAddress)
	if err != nil {
		fmt.Println("Failed to start TCP RPC server:", err)
		return
	}
	defer listener.Close()
	fmt.Println("RPC server started, listening on", tcpAddress)

	// Start HTTP JSON-RPC server
	httpPort := 8545 + validatorIndex // Standard Ethereum port + offset
	go n.startHTTPJSONRPCServer(httpPort, jam)

	// Listen for TCP RPC requests
	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("⚠️ Failed to accept connection:", err)
			continue
		}
		go rpc.ServeConn(conn)
	}
}

// startHTTPJSONRPCServer starts an HTTP JSON-RPC server
func (n *NodeContent) startHTTPJSONRPCServer(port int, jam *Jam) {
	address := fmt.Sprintf(":%d", port)

	// Create a new ServeMux to avoid conflicts between multiple nodes
	mux := http.NewServeMux()

	// Handle JSON-RPC requests
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Content-Type", "application/json")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			JSONRPC string        `json:"jsonrpc"`
			Method  string        `json:"method"`
			Params  []interface{} `json:"params"`
			ID      interface{}   `json:"id"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		// Convert params to string slice (current RPC methods expect []string)
		stringParams := make([]string, len(req.Params))
		for i, param := range req.Params {
			switch v := param.(type) {
			case string:
				stringParams[i] = v
			case bool:
				stringParams[i] = strconv.FormatBool(v)
			case float64:
				// Check if it's an integer value
				if v == float64(int64(v)) {
					stringParams[i] = strconv.FormatInt(int64(v), 10)
				} else {
					stringParams[i] = strconv.FormatFloat(v, 'f', -1, 64)
				}
			default:
				// For complex types, marshal to JSON string
				jsonBytes, err := json.Marshal(param)
				if err != nil {
					http.Error(w, fmt.Sprintf("Failed to convert param %v to string", param), http.StatusBadRequest)
					return
				}
				stringParams[i] = string(jsonBytes)
			}
		}

		// Call the RPC method
		var result string
		err := callJamMethod(jam, req.Method, stringParams, &result)

		response := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      req.ID,
		}

		if err != nil {
			response["error"] = map[string]interface{}{
				"code":    -32603,
				"message": err.Error(),
			}
		} else {
			// Try to parse result as JSON, if it fails return as string
			var jsonResult interface{}
			if json.Unmarshal([]byte(result), &jsonResult) == nil {
				response["result"] = jsonResult
			} else {
				response["result"] = result
			}
		}

		w.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(w).Encode(response)
	})

	fmt.Printf("HTTP JSON-RPC server started, listening on %s\n", address)
	if err := http.ListenAndServe(address, mux); err != nil {
		fmt.Printf("Failed to start HTTP JSON-RPC server: %v\n", err)
	}
}

// callJamMethod calls the appropriate method on the Jam struct
func callJamMethod(jam *Jam, method string, params []string, result *string) error {
	switch method {
	// Ethereum methods
	case "eth_chainId":
		return jam.ChainId(params, result)
	case "eth_accounts":
		return jam.Accounts(params, result)
	case "eth_gasPrice":
		return jam.GasPrice(params, result)
	case "eth_getBalance":
		return jam.GetBalance(params, result)
	case "eth_getStorageAt":
		return jam.GetStorageAt(params, result)
	case "eth_getTransactionCount":
		return jam.GetTransactionCount(params, result)
	case "eth_getCode":
		return jam.GetCode(params, result)
	case "eth_estimateGas":
		return jam.EstimateGas(params, result)
	case "eth_call":
		return jam.Call(params, result)
	case "eth_sendRawTransaction":
		return jam.SendRawTransaction(params, result)
	case "eth_getTransactionReceipt":
		return jam.GetTransactionReceipt(params, result)
	case "eth_getTransactionByHash":
		return jam.GetTransactionByHash(params, result)
	case "eth_getTransactionByBlockHashAndIndex":
		return jam.GetTransactionByBlockHashAndIndex(params, result)
	case "eth_getTransactionByBlockNumberAndIndex":
		return jam.GetTransactionByBlockNumberAndIndex(params, result)
	case "eth_getLogs":
		return jam.GetLogs(params, result)
	case "eth_blockNumber":
		return jam.BlockNumber(params, result)
	case "eth_getBlockByHash":
		return jam.GetBlockByHash(params, result)
	case "eth_getBlockByNumber":
		return jam.GetBlockByNumber(params, result)
	// JAM methods
	case "jam_txPoolStatus":
		return jam.TxPoolStatus(params, result)
	case "jam_txPoolContent":
		return jam.TxPoolContent(params, result)
	case "jam_txPoolInspect":
		return jam.TxPoolInspect(params, result)
	default:
		return fmt.Errorf("method not found: %s", method)
	}
}

func ParsePeerList(peerListMapFile string) (peerInfoMap map[uint16]*PeerInfo, err error) {
	peerListMapJson, err := os.Open(peerListMapFile)
	if err != nil {
		errStr := fmt.Sprintf("Error Open(peerListFile): %s\n", err)
		return peerInfoMap, errors.New(errStr)
	}

	err = json.NewDecoder(peerListMapJson).Decode(&peerInfoMap)
	if err != nil {
		errStr := fmt.Sprintf("Error Decode: %s\n", err)
		return peerInfoMap, errors.New(errStr)
	}
	peerListMapJson.Close()
	return peerInfoMap, nil
}

// mk's codec api
func encodeapi(objectType string, inp string) (string, error) {
	var err error
	var obj interface{}

	fmt.Printf("encodeapi: objectType=%s\ninput=%s\n", objectType, inp)
	input := []byte(inp)

	// Unmarshal JSON → Go struct → Encode (hex)
	switch objectType {
	case "Block":
		var block types.Block
		err = json.Unmarshal(input, &block)
		obj = block
	case "Ticket":
		var ticket types.Ticket
		err = json.Unmarshal(input, &ticket)
		obj = ticket
	case "Guarantee":
		var guarantee types.Guarantee
		err = json.Unmarshal(input, &guarantee)
		obj = guarantee
	case "Assurance":
		var assurance types.Assurance
		err = json.Unmarshal(input, &assurance)
		obj = assurance
	case "Preimages":
		var preimages types.Preimages
		err = json.Unmarshal(input, &preimages)
		obj = preimages
	case "WorkPackage":
		var workPackage types.WorkPackage
		err = json.Unmarshal(input, &workPackage)
		obj = workPackage
	case "WorkItem":
		var workItem types.WorkItem
		err = json.Unmarshal(input, &workItem)
		obj = workItem
	case "WorkReport":
		var workReport types.WorkReport
		err = json.Unmarshal(input, &workReport)
		obj = workReport
	case "WorkDigest":
		var workDigest types.WorkDigest
		err = json.Unmarshal(input, &workDigest)
		obj = workDigest
	case "Announcement":
		var announcement types.Announcement
		err = json.Unmarshal(input, &announcement)
		obj = announcement
	case "Judgement":
		var judgement types.Judgement
		err = json.Unmarshal(input, &judgement)
		obj = judgement
	case "C1":
		var c1 [types.TotalCores][]common.Hash
		err = json.Unmarshal(input, &c1)
		obj = c1
	case "C2":
		var c2 types.AuthorizationQueue
		err = json.Unmarshal(input, &c2)
		obj = c2
	case "C3":
		var c3 statedb.RecentBlocks
		err = json.Unmarshal(input, &c3)
		obj = c3
	case "C3-Beta":
		var history_state statedb.HistoryState
		err = json.Unmarshal(input, &history_state)
		obj = history_state
	case "C4":
		var c4 statedb.SafroleBasicState
		err = json.Unmarshal(input, &c4)
		obj = c4
	case "C4-Gamma_s":
		var c4gammas statedb.TicketsOrKeys
		err = json.Unmarshal(input, &c4gammas)
		obj = c4gammas
	case "C5":
		var c5 statedb.DisputeState
		err = json.Unmarshal(input, &c5)
		obj = c5
	case "C6":
		var c6 statedb.Entropy
		err = json.Unmarshal(input, &c6)
		obj = c6
	case "C7", "C8", "C9":
		var validators types.Validators
		err = json.Unmarshal(input, &validators)
		obj = validators
	case "C10":
		var availabilityAssignments statedb.AvailabilityAssignments
		err = json.Unmarshal(input, &availabilityAssignments)
		obj = availabilityAssignments
	case "C11":
		var c11 uint32
		err = json.Unmarshal(input, &c11)
		obj = c11
	case "C12":
		var c12 types.PrivilegedServiceState
		err = json.Unmarshal(input, &c12)
		obj = c12
	case "C13":
		var c13 types.ValidatorStatistics
		err = json.Unmarshal(input, &c13)
		obj = c13
	case "C14":
		var c14 [types.EpochLength][]types.AccumulationQueue
		err = json.Unmarshal(input, &c14)
		obj = c14
	case "C15":
		var c15 [types.EpochLength]types.AccumulationHistory
		err = json.Unmarshal(input, &c15)
		obj = c15
	case "JamState":
		var jamstate statedb.StateSnapshot
		err = json.Unmarshal(input, &jamstate)
		obj = jamstate
	case "STF":
		var stf statedb.StateTransition
		err = json.Unmarshal(input, &stf)
		obj = stf
	case "SC":
		var sc statedb.StateTransitionChallenge
		err = json.Unmarshal(input, &sc)
		obj = sc
	case "ServiceAccount":
		// Special case
		var serviceAccount types.ServiceAccount
		err = json.Unmarshal(input, &serviceAccount)
		if err != nil {
			return "", err
		}
		encodedBytes, err := serviceAccount.Bytes()
		if err != nil {
			return "", err
		}
		return common.Bytes2Hex(encodedBytes), nil

	default:
		return "", errors.New("unknown object type")
	}

	if err != nil {
		return "", err
	}

	// Encode the unmarshaled Go object into bytes
	encodedBytes, err := types.Encode(obj)
	if err != nil {
		return "", err
	}

	// Return hex string
	return common.Bytes2Hex(encodedBytes), nil
}

func decodeapi(objectType, input string) (string, error) {
	// Convert input hex → bytes
	encodedBytes := common.Hex2Bytes(input)
	if len(encodedBytes) == 0 {
		return "", errors.New("invalid hex input")
	}

	var err error
	var decodedStruct interface{}

	switch objectType {
	case "Block":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.Block{}))
	case "Ticket":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.Ticket{}))
	case "Guarantee":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.Guarantee{}))
	case "Assurance":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.Assurance{}))
	case "Preimages":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.Preimages{}))
	case "Announcement":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.Announcement{}))
	case "Judgement":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.Judgement{}))
	case "WorkPackage":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.WorkPackage{}))
	case "WorkDigest":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.WorkDigest{}))
	case "WorkReport":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.WorkReport{}))
	case "WorkItem":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.WorkItem{}))
	case "C1":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf([types.TotalCores][]common.Hash{}))
	case "C2":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.AuthorizationQueue{}))
	case "C3":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(statedb.RecentBlocks{}))
	case "C3-Beta":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(statedb.HistoryState{}))
	case "C4":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(statedb.SafroleBasicState{}))
	case "C4-Gamma_s":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(statedb.TicketsOrKeys{}))
	case "C5":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(statedb.DisputeState{}))
	case "C6":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(statedb.Entropy{}))
	case "C7":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.Validators{}))
	case "C8":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.Validators{}))
	case "C9":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.Validators{}))
	case "C10":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(statedb.AvailabilityAssignments{}))
	case "C11":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(uint32(0)))
	case "C12":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.PrivilegedServiceState{}))
	case "C13":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.ValidatorStatistics{}))
	case "C14":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf([types.EpochLength][]types.AccumulationQueue{}))
	case "C15":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf([types.EpochLength]types.AccumulationHistory{}))
	case "JamState":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(statedb.StateSnapshot{}))
	case "STF":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(statedb.StateTransition{}))
	case "SC":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(statedb.StateTransitionChallenge{}))
	case "ServiceAccount":
		decodedStruct, err = types.AccountStateFromBytes(0, encodedBytes)
	default:
		return "", errors.New("unknown object type")
	}

	if err != nil {
		return "", err
	}

	// Convert decoded structure → JSON (indented)
	decodedJSON, err := json.MarshalIndent(decodedStruct, "", "    ")
	if err != nil {
		return "", err
	}

	return string(decodedJSON), nil
}

// Port and network configuration
// Note: Port variables are defined in node_rpc_port.go

// SendRawTransaction submits a signed transaction to the mempool
func (n *NodeContent) SendRawTransaction(signedTxData []byte) (common.Hash, error) {
	// Parse the raw transaction
	tx, err := statedb.ParseRawTransaction(signedTxData)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to parse transaction: %v", err)
	}

	// Recover sender from signature
	sender, err := tx.RecoverSender()
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to recover sender: %v", err)
	}
	tx.From = sender

	// Get or create transaction pool
	if n.txPool == nil {
		n.txPool = NewTxPool()
		log.Info(log.Node, "SendRawTransaction: Created new TxPool")
	}

	// Validate signature - sender recovery already done above, verify it's valid
	if sender == (common.Address{}) {
		return common.Hash{}, fmt.Errorf("invalid signature: unable to recover sender address")
	}

	// Validate nonce against current state
	currentNonce, err := n.statedb.GetTransactionCount(sender)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to get current nonce for validation: %v", err)
	}
	if tx.Nonce < currentNonce {
		return common.Hash{}, fmt.Errorf("nonce too low: transaction nonce %d, account nonce %d", tx.Nonce, currentNonce)
	}

	// Validate balance - sender must have enough to cover value + gas costs
	balance, err := n.statedb.GetBalance(sender)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to get balance for validation: %v", err)
	}
	balanceBig := new(big.Int).SetBytes(balance.Bytes())

	// Calculate total cost: value + (gas * gasPrice)
	gasCost := new(big.Int).Mul(new(big.Int).SetUint64(tx.Gas), tx.GasPrice)
	totalCost := new(big.Int).Add(tx.Value, gasCost)

	if balanceBig.Cmp(totalCost) < 0 {
		return common.Hash{}, fmt.Errorf("insufficient funds: balance %s, required %s (value %s + gas cost %s)",
			balanceBig.String(), totalCost.String(), tx.Value.String(), gasCost.String())
	}

	// Validate gas limit against block gas limit (RefineGasAllocation per work item)
	maxGasLimit := uint64(types.RefineGasAllocation)
	if tx.Gas > maxGasLimit {
		return common.Hash{}, fmt.Errorf("gas limit too high: transaction gas %d exceeds maximum %d", tx.Gas, maxGasLimit)
	}

	// Minimum gas for basic transaction is 1000
	const minTxGas = 1000
	if tx.Gas < minTxGas {
		return common.Hash{}, fmt.Errorf("gas limit too low: transaction gas %d is below minimum %d", tx.Gas, minTxGas)
	}

	// Add transaction to mempool
	err = n.txPool.AddTransaction(tx)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to add transaction to mempool: %v", err)
	}

	log.Info(log.Node, "SendRawTransaction: Transaction added to mempool",
		"hash", tx.Hash.String(),
		"from", tx.From.String(),
		"nonce", tx.Nonce)

	return tx.Hash, nil
}

// JNode interface implementations - JAM-specific methods

func (n *NodeContent) GetService(service uint32) (sa *types.ServiceAccount, ok bool, err error) {
	return n.statedb.GetService(service)
}

func (n *NodeContent) GetServiceStorage(serviceID uint32, storageKey []byte) ([]byte, bool, error) {
	return n.statedb.ReadServiceStorage(serviceID, storageKey)
}

func (n *NodeContent) ReadStateWitnessRef(serviceID uint32, objectID common.Hash, fetchPayloadFromDA bool) (types.StateWitness, bool, error) {
	return n.statedb.ReadStateWitnessRef(serviceID, objectID, fetchPayloadFromDA)
}

func (n *NodeContent) ReadStateWitnessRaw(serviceID uint32, objectID common.Hash) (types.StateWitnessRaw, bool, common.Hash, error) {
	return n.statedb.ReadStateWitnessRaw(serviceID, objectID)
}

func (n *NodeContent) GetStateWitnesses(workReports []*types.WorkReport) ([]types.StateWitness, common.Hash, error) {
	return n.statedb.GetStateWitnesses(workReports)
}

// JNode interface implementations - Basic info methods

func (n *NodeContent) GetChainId() uint64 {
	return n.statedb.GetChainId()
}

func (n *NodeContent) GetAccounts() []common.Address {
	return n.statedb.GetAccounts()
}

func (n *NodeContent) GetGasPrice() uint64 {
	return n.statedb.GetGasPrice()
}

// JNode interface implementations - Contract State methods

func (n *NodeContent) GetBalance(address common.Address, blockNumber string) (common.Hash, error) {
	targetStateDB, err := n.getTargetStateDB(blockNumber)
	if err != nil {
		return common.Hash{}, err
	}
	return targetStateDB.GetBalance(address)
}

func (n *NodeContent) GetStorageAt(address common.Address, position common.Hash, blockNumber string) (common.Hash, error) {
	targetStateDB, err := n.getTargetStateDB(blockNumber)
	if err != nil {
		return common.Hash{}, err
	}
	return targetStateDB.GetStorageAt(address, position)
}

func (n *NodeContent) GetTransactionCount(address common.Address, blockNumber string) (uint64, error) {
	targetStateDB, err := n.getTargetStateDB(blockNumber)
	if err != nil {
		return 0, err
	}
	return targetStateDB.GetTransactionCount(address)
}

func (n *NodeContent) GetCode(address common.Address, blockNumber string) ([]byte, error) {
	targetStateDB, err := n.getTargetStateDB(blockNumber)
	if err != nil {
		return nil, err
	}
	return targetStateDB.GetCode(address)
}

// JNode interface implementations - Transaction Operations

func (n *NodeContent) EstimateGas(from common.Address, to *common.Address, gas uint64, gasPrice uint64, value uint64, data []byte) (uint64, error) {
	return n.statedb.EstimateGas(from, to, gas, gasPrice, value, data, n.pvmBackend)
}

func (n *NodeContent) Call(from common.Address, to *common.Address, gas uint64, gasPrice uint64, value uint64, data []byte, blockNumber string) ([]byte, error) {
	return n.statedb.Call(from, to, gas, gasPrice, value, data, blockNumber, n.pvmBackend)
}

// JNode interface implementations - Transaction Queries

func (n *NodeContent) GetTransactionReceipt(txHash common.Hash) (*statedb.EthereumTransactionReceipt, error) {
	serviceID := uint32(n.GetChainId())
	return n.statedb.GetTransactionReceipt(serviceID, txHash)
}

func (n *NodeContent) GetTransactionByHash(txHash common.Hash) (*statedb.EthereumTransactionResponse, error) {
	serviceID := uint32(n.GetChainId())
	receipt, ref, err := n.statedb.GetTransactionByHash(serviceID, txHash)
	if err != nil {
		return nil, err
	}
	if receipt == nil {
		return nil, nil // Transaction not found
	}

	// Convert the original payload to Ethereum transaction format
	ethTx, err := statedb.ConvertPayloadToEthereumTransaction(receipt)
	if err != nil {
		return nil, fmt.Errorf("failed to convert payload to Ethereum transaction: %v", err)
	}

	// Get block metadata from the receipt's Ref field
	evmBlock, err := n.statedb.ReadBlockByNumber(serviceID, ref.EvmBlock)
	if err != nil {
		// If block can't be read, return transaction without block metadata (pending state)
		log.Warn(log.Node, "GetTransactionByHash: Failed to read block metadata",
			"txHash", txHash.String(), "blockNum", ref.EvmBlock, "error", err)
		return ethTx, nil
	}

	// Populate block metadata
	blockHash := evmBlock.ComputeHash().String()
	blockNumber := fmt.Sprintf("0x%x", ref.EvmBlock)
	txIndex := fmt.Sprintf("0x%x", ref.TxSlot)
	ethTx.BlockHash = &blockHash
	ethTx.BlockNumber = &blockNumber
	ethTx.TransactionIndex = &txIndex

	return ethTx, nil
}

func (n *NodeContent) GetTransactionByBlockHashAndIndex(blockHash common.Hash, index uint32) (*statedb.EthereumTransactionResponse, error) {
	serviceID := uint32(n.GetChainId())
	return n.statedb.GetTransactionByBlockHashAndIndex(serviceID, blockHash, index)
}

func (n *NodeContent) GetTransactionByBlockNumberAndIndex(blockNumber string, index uint32) (*statedb.EthereumTransactionResponse, error) {
	serviceID := uint32(n.GetChainId())
	return n.statedb.GetTransactionByBlockNumberAndIndex(serviceID, blockNumber, index)
}

func (n *NodeContent) GetLogs(fromBlock, toBlock uint32, addresses []common.Address, topics [][]common.Hash) ([]statedb.EthereumLog, error) {
	serviceID := uint32(n.GetChainId())
	return n.statedb.GetLogs(serviceID, fromBlock, toBlock, addresses, topics)
}

// JNode interface implementations - Block Queries

func (n *NodeContent) GetLatestBlockNumber() (uint32, error) {
	serviceID := uint32(n.GetChainId())
	return n.statedb.GetLatestBlockNumber(serviceID)
}

func (n *NodeContent) GetBlockByHash(blockHash common.Hash, fullTx bool) (*statedb.EthereumBlock, error) {
	serviceID := uint32(n.GetChainId())
	return n.statedb.GetBlockByHash(serviceID, blockHash, fullTx)
}

func (n *NodeContent) GetBlockByNumber(blockNumber string, fullTx bool) (*statedb.EthereumBlock, error) {
	serviceID := uint32(n.GetChainId())
	return n.statedb.GetBlockByNumberFormatted(serviceID, blockNumber, fullTx)
}
