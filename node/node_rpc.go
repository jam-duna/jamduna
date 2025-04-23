package node

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/rpc"
	"os"
	"reflect"
	"runtime"
	"strconv"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

type Jam struct {
	*NodeContent
}

var MethodDescriptionMap = map[string]string{
	"Functions":   "Functions() -> functions description",
	"NodeCommand": "NodeCommand(command string) -> will pass the command to the node",

	"Block":            "Block(headerHash hexstring) -> string",
	"BestBlock":        "BestBlock(headerHash hexstring) -> string",
	"FinalizedBlock":   "FinalizedBlock(headerHash hexstring) -> string",
	"Parent":           "Parent(headerHash hexstring) -> string",
	"StateRoot":        "StateRoot(headerHash hexstring) -> string",
	"BeefyRoot":        "BeefyRoot(headerHash hexstring) -> string",
	"State":            "State(headerHash hexstring) -> string",
	"Statistics":       "Statistics(headerHash hexstring) -> string",
	"ServiceInfo":      "ServiceInfo(serviceIndex string) -> string",
	"ServicePreimage":  "ServicePreimage(serviceIndex string, preimage hexstring) -> hexstring",
	"ServiceRequest":   "ServiceRequest(serviceIndex string, preimage hexstring, length string) -> json string",
	"SubmitPreimage":   "SubmitPreimage(serviceIndex string, preimage hexstring) -> string",
	"ServiceValue":     "ServiceValue(serviceIndex string, key hexstring) -> hexstring",
	"WorkPackage":      "WorkPackage(workPackageHash string) -> json WorkReport",
	"Code":             "Code(serviceIndex string) -> json string",
	"ListServices":     "ListServices() -> json string",
	"AuditWorkPackage": "AuditWorkPackage(workPackageHash string) -> json WorkReport",
	"Segment":          "Segment(requestedHash string, index int) -> hex string",
	"Encode":           "Encode(objectType string, input string) -> hexstring",
	"Decode":           "Decode(objectType string, input string) -> json string",
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
	case "SetLog":
		module := req[1]
		value := req[2]
		BoolVal, err := strconv.ParseBool(value)
		if err != nil {
			*res = fmt.Sprintf("Invalid value for flag %s: %s", module, value)
			return err
		}
		if BoolVal {
			log.EnableModule(module)
		} else {
			log.DisableModule(module)
		}
	case "StackTrace":
		debugtrace := make([]byte, 1<<20)
		runtime.Stack(debugtrace, true)
		*res = string(debugtrace)
		return nil
	default:
		*res = fmt.Sprintf("Unknown command %s", command)
		return fmt.Errorf("Unknown command %s", command)
	}
	return nil
}

func (j *Jam) GetAvailabilityAssignments(req []string, res *string) error {
	if len(req) != 1 {
		return fmt.Errorf("Invalid number of arguments")
	}
	coreIdxStr := req[0]
	codeIdx, err := strconv.ParseUint(coreIdxStr, 10, 32)
	if err != nil {
		return err
	}
	rho_state := j.statedb.JamState.AvailabilityAssignments[codeIdx]
	rho_stateStr := rho_state.String()
	//fmt.Printf("JAM SERVER GetAvailabilityAssignments @ coreIdx=%d rho=%v\n", codeIdx, rho_stateStr)
	*res = rho_stateStr
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
		*res = fmt.Sprintf("%s", sdb.StateRoot)
		return nil
	}
	return fmt.Errorf("Unknown header hash %s", headerHash)
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

	recentBlocks := sdb.JamState.Snapshot(&statedb.StateSnapshotRaw{}).RecentBlocks
	if len(recentBlocks) > 0 {
		*res = recentBlocks[len(recentBlocks)-1].String()
		return nil
	}
	return fmt.Errorf("No recent blocks")
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
	*res = sdb.JamState.Snapshot(&statedb.StateSnapshotRaw{}).String()
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
	*res = sdb.JamState.Snapshot(&statedb.StateSnapshotRaw{}).ValidatorStatistics.String()
	return nil
}

func (j *Jam) GetLatestState(req []string, res *string) error {
	if len(req) != 0 {
		return fmt.Errorf("Invalid number of arguments")
	}
	sdb := j.statedb
	*res = sdb.JamState.Snapshot(&statedb.StateSnapshotRaw{}).String()
	return nil
}

func (j *Jam) ServiceInfo(req []string, res *string) error {
	if len(req) != 1 {
		return fmt.Errorf("Invalid number of arguments")
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
		return fmt.Errorf("Service not found %d", serviceIndex)
	}
	*res = service.JsonString()
	return nil
}

func (j *Jam) Code(req []string, res *string) error {
	if len(req) != 1 {
		return fmt.Errorf("Invalid number of arguments")
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
		return fmt.Errorf("Invalid number of arguments")
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
		return fmt.Errorf("Invalid number of arguments")
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
		return fmt.Errorf("Invalid number of arguments")
	}
	serviceIndexStr := req[0]
	storage_hashStr := req[1]
	serviceIndex, err := strconv.ParseUint(serviceIndexStr, 10, 32)
	if err != nil {
		return err
	}
	storage_hash := common.HexToHash(storage_hashStr)
	storage, ok, err := j.statedb.ReadServiceStorage(uint32(serviceIndex), storage_hash)
	if err != nil {
		return fmt.Errorf("ReadServiceStorage failed (serviceID=%d, h=%s) %v", serviceIndex, storage_hash, err)
	}
	if !ok {
		return fmt.Errorf("ReadServiceStorage not found (serviceID=%d, h=%s)", serviceIndex, storage_hash)
	}
	*res = common.Bytes2Hex(storage)
	return nil
}

func (n *NodeContent) getSegments(requestedHash common.Hash, index []uint16) (segment [][]byte, justifications [][]common.Hash, err error) {
	si := n.WorkReportSearch(requestedHash)
	if si == nil {
		return nil, nil, fmt.Errorf("requestedHash not found")
	}
	for _, idx := range index {
		si.AddIndex(idx)
	}
	segments, justifications, err := n.reconstructSegments(si)
	if err != nil {
		return nil, nil, err
	}
	if paranoidVerification {
		// for each segment, verify the justification (which is a pageproof)

	}
	return segments, justifications, nil
}

// GetWorkPackageByHash(workPackageHash string) -> json WorkReport
func (j *Jam) WorkPackage(req []string, res *string) error {
	if len(req) != 1 {
		return fmt.Errorf("Invalid number of arguments")
	}
	workPackageHash := common.HexToHash(req[0])
	si := j.WorkReportSearch(workPackageHash)
	if si == nil {
		return fmt.Errorf("Work Package not found")
	}

	workReport := si.WorkReport
	*res = workReport.String()
	return nil
}

func (j *Jam) TraceBlock(req []string, res *string) error {
	if len(req) != 1 {
		return fmt.Errorf("Invalid number of arguments")
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
	s1, err := statedb.ApplyStateTransitionFromBlock(sdb, ctx, block, nil)
	if err != nil {
		log.Error(module, "TraceBlock", "err", err)
		return err
	}
	logs, err := log.GetRecordedLogs()
	if err != nil {
		log.Error(module, "TraceBlock", "block", block.String(), "len(logs)", len(logs), "err", err)
		return err
	}
	log.Info(module, "TraceBlock", "block", block.String(), "len(logs)", len(logs), "s1.StateRoot", s1.StateRoot)
	*res = string(logs)
	return nil
}

// AuditWorkPackageByHash(workPackageHash string) -> json WorkReport
func (j *Jam) AuditWorkPackage(req []string, res *string) error {
	if len(req) != 1 {
		return fmt.Errorf("Invalid number of arguments")
	}
	workPackageHash := common.HexToHash(req[0])
	si := j.WorkReportSearch(workPackageHash)
	if si == nil {
		return fmt.Errorf("Work Package not found")
	}
	workReport := si.WorkReport
	spec := workReport.AvailabilitySpec
	// now call C138 to get bundle_shard from C assurers, do ec reconstruction for b
	// IMPORTANT: within reconstructPackageBundleSegments is a call to VerifyBundle
	workPackageBundle, err := j.reconstructPackageBundleSegments(spec.ErasureRoot, spec.BundleLength, workReport.SegmentRootLookup)
	if err != nil {
		return err
	}
	log.RecordLogs()
	log.EnableModule(log.FirstGuarantorOrAuditor)

	workReport2, _, wr_pvm_elapsed, err := j.executeWorkPackageBundle(workReport.CoreIndex, workPackageBundle, workReport.SegmentRootLookup, true)
	if err != nil {
		return err
	}

	// check that workReport == workReport2
	if workReport.Hash() == workReport2.Hash() {
		log.Info(module, "AuditWorkPackage", "auditResult", workReport.Hash() == workReport2.Hash(), "workReport", workReport, "elapsed", wr_pvm_elapsed)
	} else {
		log.Error(module, "AuditWorkPackage", "auditResult", workReport.Hash() == workReport2.Hash(), "workReport", workReport, "elapsed", wr_pvm_elapsed)
	}
	logs, err := log.GetRecordedLogs()
	if err != nil {
		return err
	}
	*res = string(logs)
	return nil
}

// GetSegment(requestedHash string, index int) -> hex string
func (j *Jam) Segment(req []string, res *string) (err error) {
	if len(req) != 2 {
		return fmt.Errorf("Invalid number of arguments")
	}
	requestedHash := common.HexToHash(req[0])

	var index uint16
	indicesStr := req[1]
	err = json.Unmarshal([]byte(indicesStr), &index)
	if err != nil {
		return fmt.Errorf("Invalid index %s (must be between 0 and export_count-1)", indicesStr)
	}

	indices := make([]uint16, 1)
	indices[0] = index
	segments, justifications, err := j.getSegments(requestedHash, indices)
	if err != nil {
		return err
	}
	type getSegmentResponse struct {
		Segment       []byte        `json:"segment"`
		Justification []common.Hash `json:"justification"`
	}
	if len(segments) != 1 {
		return fmt.Errorf("segment not found")
	}
	response := getSegmentResponse{
		Segment:       segments[0],
		Justification: justifications[0],
	}
	r, err := json.Marshal(response)
	if err != nil {
		return err
	}
	*res = string(r)
	return nil
}

func (j *Jam) SubmitPreimage(req []string, res *string) error {
	if len(req) != 2 {
		return fmt.Errorf("Invalid number of arguments")
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
	log.Info(module, "SubmitPreimage", "service", service_index, "preimage", common.Blake2Hash(preimage), "len", preimage_length)

	// Add it to your own pool
	j.AddPreimageToPool(service_index, preimage)

	// Announce it everyone else with CE142 (and they will request it with CE143, which will be available in the pool from the above)
	err = j.BroadcastPreimageAnnouncement(service_index, preimage_hash, preimage_length, preimage)
	if err != nil {
		log.Error(module, "SubmitPreimage ERR2", "err", err)
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
			return fmt.Errorf("Invalid number of arguments")
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
			return fmt.Errorf("Invalid number of arguments")
		}
		direction := req[2]
		direction_num, err := strconv.Atoi(direction)
		if err != nil {
			return fmt.Errorf("Invalid number of arguments")
		}
		blocks, err := j.nodeSelf.fetchBlocks(block_hash, uint8(direction_num), uint32(num))
		if err != nil {
			*res = fmt.Sprintf("block not found err=%v", err)
		}
		// convert to json
		*res = types.ToJSON(blocks)
		return nil
	}
	*res = fmt.Sprintf("Invalid Request")
	return fmt.Errorf("Invalid Request")
}

func (j *Jam) SubmitWorkPackage(req []string, res *string) error {
	if len(req) != 1 {
		log.Info(module, "SubmitWorkPackage error", "err", req)
		return fmt.Errorf("invalid number of arguments")
	}

	var newReq WorkPackageRequest
	if err := json.Unmarshal([]byte(req[0]), &newReq); err != nil {
		log.Error(module, "SubmitWorkPackage", "err", err)
		return fmt.Errorf("failed to decode WorkPackageRequest: %w", err)
	}

	workPackageHash := newReq.WorkPackage.Hash()
	j.NodeContent.workPackageQueue.Store(workPackageHash, &WPQueueItem{
		workPackage:        newReq.WorkPackage,
		coreIndex:          newReq.CoreIndex,
		extrinsics:         newReq.ExtrinsicsBlobs,
		addTS:              time.Now().Unix(),
		nextAttemptAfterTS: time.Now().Unix(),
	})
	log.Info(module, "SubmitWorkPackage succ", "wph", workPackageHash)

	*res = "OK"
	return nil
}

func (j *Jam) GetRefineContext(req []string, res *string) error {
	if len(req) != 0 {
		return fmt.Errorf("Invalid number of arguments")
	}
	// Access statedb via Node reference
	refinecontext := j.statedb.GetRefineContext() // not sure

	// json marshal the refine context
	*res = refinecontext.String()
	return nil
}

func (j *Jam) ListServices(req []string, res *string) error {
	if len(req) != 0 {
		return fmt.Errorf("Invalid number of arguments")
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
		return fmt.Errorf("Invalid number of arguments")
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
		return fmt.Errorf("Invalid number of arguments")
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
	n.NodeContent.nodeSelf = n
	n.NodeContent.startRPCServerImpl(validatorIndex)
}

func (n *NodeContent) startRPCServerImpl(validatorIndex int) {
	jam := new(Jam)
	jam.NodeContent = n
	// register the rpc methods
	rpc.RegisterName("jam", jam)
	address := fmt.Sprintf(":%d", DefaultTCPPort+validatorIndex)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		fmt.Println("Failed to start RPC server:", err)
		return
	}
	defer listener.Close()
	fmt.Println("RPC server started, listening on", address)

	// Listen for requests
	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("⚠️ Failed to accept connection:", err)
			continue
		}
		go rpc.ServeConn(conn)
	}
}

func ParsePeerList(peerListMapFile string) (peerInfoMap map[uint16]*PeerInfo, err error) {
	peerListMapJson, err := os.Open(peerListMapFile)
	if err != nil {
		errStr := fmt.Sprintf("Error Open(peerListFile): %s\n", err)
		return peerInfoMap, fmt.Errorf(errStr)
	}

	err = json.NewDecoder(peerListMapJson).Decode(&peerInfoMap)
	if err != nil {
		errStr := fmt.Sprintf("Error Decode: %s\n", err)
		return peerInfoMap, fmt.Errorf(errStr)
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
	case "WorkResult":
		var workResult types.WorkResult
		err = json.Unmarshal(input, &workResult)
		obj = workResult
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
		var beta_state statedb.Beta_state
		err = json.Unmarshal(input, &beta_state)
		obj = beta_state
	case "C4":
		var c4 statedb.SafroleBasicState
		err = json.Unmarshal(input, &c4)
		obj = c4
	case "C4-Gamma_s":
		var c4gammas statedb.TicketsOrKeys
		err = json.Unmarshal(input, &c4gammas)
		obj = c4gammas
	case "C5":
		var c5 statedb.Psi_state
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
		var kaiState types.Kai_state
		err = json.Unmarshal(input, &kaiState)
		obj = kaiState
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
		return "", errors.New("Unknown object type")
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
		return "", errors.New("Invalid hex input")
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
	case "WorkResult":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.WorkResult{}))
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
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(statedb.Beta_state{}))
	case "C4":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(statedb.SafroleBasicState{}))
	case "C4-Gamma_s":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(statedb.TicketsOrKeys{}))
	case "C5":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(statedb.Psi_state{}))
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
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.Kai_state{}))
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
		return "", errors.New("Unknown object type")
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
