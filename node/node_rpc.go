package node

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/rpc"
	"os"
	"reflect"
	"strconv"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
	"golang.org/x/exp/rand"
)

type Jam struct {
	*NodeContent
}

type NodeClient struct {
	PeerInfo *PeerInfo
	Client   *rpc.Client
}

// ----------------- client side -----------------
func (nc *NodeClient) GetBuildVersion() (string, error) {
	var result string
	err := nc.Client.Call("jam.GetBuildVersion", []string{}, &result)
	return result, err
}

func (nc *NodeClient) GetCurrJCE() (uint32, error) {
	var resultStr string
	err := nc.Client.Call("jam.GetCurrJCE", []string{}, &resultStr)
	if err != nil {
		return 0, err
	}
	var result uint32
	_, err = fmt.Sscanf(resultStr, "%d", &result)
	return result, err
}

func (c *NodeClient) AddPreimage(preimage []byte) (common.Hash, error) {
	var codeHash common.Hash
	err := c.Client.Call("jam.AddPreimage", preimage, &codeHash)
	return codeHash, err
}

func (c *NodeClient) GetRefineContext() (types.RefineContext, error) {
	var jsonStr string
	err := c.Client.Call("jam.GetRefineContext", []string{}, &jsonStr)
	if err != nil {
		return types.RefineContext{}, err
	}

	var context types.RefineContext
	err = json.Unmarshal([]byte(jsonStr), &context)
	if err != nil {
		return types.RefineContext{}, fmt.Errorf("failed to unmarshal refine context: %w", err)
	}
	return context, nil
}

func (c *NodeClient) SendWorkPackage(workPackageReq types.WorkPackageRequest) error {
	// Marshal the WorkPackageRequest to JSON
	reqBytes, err := json.Marshal(workPackageReq)
	if err != nil {
		fmt.Errorf("failed to marshal work package request: %w", err)
	}

	fmt.Printf("NodeClient SendWorkPackage:%v | ExtrinsBlobs:%x | %s\n", workPackageReq.WorkPackage.Hash(), workPackageReq.ExtrinsicsBlobs, string(reqBytes))

	// Prepare the request as a one-element string slice
	req := []string{string(reqBytes)}

	var res string
	// Call the remote RPC method
	err = c.Client.Call("jam.SendWorkPackage", req, &res)
	if err != nil {
		return err
	}
	return nil
}

func (c *NodeClient) GetServiceStorage(serviceIndex uint32, storageHash common.Hash) ([]byte, bool, error) {
	req := []string{
		strconv.FormatUint(uint64(serviceIndex), 10),
		storageHash.Hex(),
	}
	var res string
	err := c.Client.Call("jam.GetServiceStorage", req, &res)
	if err != nil {
		return nil, false, err
	}
	storageBytes := common.Hex2Bytes(res)
	return storageBytes, true, nil
}

func (c *NodeClient) SendPreimageAnnouncement(serviceIndex uint32, preimage []byte) error {
	serviceIndexStr := strconv.FormatUint(uint64(serviceIndex), 10)
	preimageStr := common.Bytes2Hex(preimage)
	req := []string{serviceIndexStr, preimageStr}

	var res string
	err := c.Client.Call("jam.SendPreimageAnnouncement", req, &res)
	if err != nil {
		return err
	}
	return nil
}

func (c *NodeClient) GetServicePreimage(serviceIndex uint32, codeHash common.Hash) ([]byte, error) {
	serviceIndexStr := strconv.FormatUint(uint64(serviceIndex), 10)
	codeHashStr := codeHash.Hex()
	req := []string{serviceIndexStr, codeHashStr}

	var res string
	err := c.Client.Call("jam.GetServicePreimage", req, &res)
	if err != nil {
		return nil, err
	}

	preimage := common.Hex2Bytes(res)
	return preimage, nil
}

func (c *NodeClient) GetAvailabilityAssignments(coreIdx uint32) (*statedb.Rho_state, error) {
	// Convert coreIdx to a string
	coreIdxStr := strconv.FormatUint(uint64(coreIdx), 10)
	req := []string{coreIdxStr}

	var res string
	// Make the RPC call to "jam.GetAvailabilityAssignments"
	err := c.Client.Call("jam.GetAvailabilityAssignments", req, &res)
	if err != nil {
		return nil, err
	}

	// Unmarshal the JSON response into an AvailabilityAssignment
	var rho_state statedb.Rho_state
	err = json.Unmarshal([]byte(res), &rho_state)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal availability assignment: %w", err)
	}
	var rho_state_empty statedb.Rho_state
	if rho_state_empty.String() == rho_state.String() {
		return nil, err
	}
	//fmt.Printf("GetAvailabilityAssignments @ coreIdx=%d rho=%v\n", coreIdx, rho_state)
	return &rho_state, nil
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
	"ServiceValue":     "ServiceRequest(serviceIndex string, key hexstring) -> hexstring",
	"WorkPackage":      "WorkPackage(workPackageHash string) -> json WorkReport",
	"GetServiceCode":   "GetServiceCode(serviceIndex string) -> json string",
	"ListServices":     "ListServices() -> json string",
	"AuditWorkPackage": "AuditWorkPackage(workPackageHash string) -> json WorkReport",
	"Segment":          "GetSegment(requestedHash string, index int) -> hex string",
	"NewService":       "NewService(serviceName string) -> string",
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
	if len(req) != 1 {
		return fmt.Errorf("Invalid number of arguments")
	}
	select {
	case j.command_chan <- req[0]:
		fmt.Printf("NodeCommand: %s\n", req[0])
		*res = "OK"
	case <-time.After(5 * time.Second):
		*res = "Timeout"
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

func (j *Jam) AddPreimage(preimage []byte, res *common.Hash) error {
	*res = j.NodeContent.AddPreimage(preimage)
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
	service_code_response := ServiceCodeResponse{
		ServiceCode: common.Bytes2Hex(preimage),
		ServiceHash: common.Bytes2Hex(code_hash.Bytes()),
	}
	service_code_response_json, err := json.Marshal(service_code_response)
	if err != nil {
		return fmt.Errorf("json.Marshal failed:%v", err)
	}
	*res = string(service_code_response_json)

	return nil
}

type ServiceCodeResponse struct {
	ServiceCode string `json:"service_code"`
	ServiceHash string `json:"service_code_hash"`
	//Optional parameters return the history of code.
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
	*res = common.Bytes2Hex(preimage)
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
	if err != nil || !ok {
		return fmt.Errorf("ReadServiceStorage failed:%v", err)
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
	//fmt.Printf("AuditWorkPackageByHash: reconstructing package bundle ErasureRoot=%v | bundleLen=%d | SegmentRootLookup=%x\n", spec.ErasureRoot, spec.BundleLength, workReport.SegmentRootLookup)
	workPackageBundle, err := j.reconstructPackageBundleSegments(spec.ErasureRoot, spec.BundleLength, workReport.SegmentRootLookup)
	if err != nil {
		return err
	}
	log.Info(debugP, "!!! AuditWorkPackageByHash reconstructPackageBundleSegments", "package_bundle.ExtrinsicData", fmt.Sprintf("%x", workPackageBundle.ExtrinsicData), "workPackageHash", workPackageBundle.PackageHash())

	workReport2, _, err := j.executeWorkPackageBundle(workReport.CoreIndex, workPackageBundle, workReport.SegmentRootLookup, false)
	if err != nil {
		return err
	}

	// check that workReport == workReport2
	if workReport.Hash() == workReport2.Hash() {
		fmt.Printf("AuditWorkPackageByHash MATCHED %s %s", workReport2.Hash(), workReport2.String())
	} else {
		fmt.Printf("AuditWorkPackageByHash MISMATCH (original %s %s ) != (reexecution %s %s)",
			workReport.Hash(), workReport.String(), workReport2.Hash(), workReport2.String())
	}

	*res = workReport2.String()
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
	if err != nil {
		*res = err.Error()
		return err
	}
	preimage_hash, err := j.statedb.ValidateLookup(&types.Preimages{Requester: service_index, Blob: preimage})
	// preimage announcement
	err = j.BroadcastPreimageAnnouncement(service_index, preimage_hash, preimage_length, preimage)
	if err != nil {
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
		return fmt.Errorf("Invalid number of arguments")
	}
	workpackageReq := req[0]
	var workPackage_req types.WorkPackageRequest
	err := json.Unmarshal([]byte(workpackageReq), &workPackage_req)
	if err != nil {
		return err
	}
	fmt.Printf("JAM Server SendWorkPackage: %v | ExtrinsBlobs:%x | %s\n", workPackage_req.WorkPackage.Hash(), workPackage_req.ExtrinsicsBlobs, workpackageReq)
	core_index := workPackage_req.CoreIndex
	workPackage := workPackage_req.WorkPackage
	extrinsics := workPackage_req.ExtrinsicsBlobs
	// broadcast work package
	core_peers := j.GetCoreCoWorkersPeers(core_index)
	// random pick the index from 0, 1, 2
	randomIdx := rand.Intn(3)
	err = core_peers[randomIdx].SendWorkPackageSubmission(workPackage, extrinsics, core_index)
	if err != nil {
		return err
	}
	*res = "OK"
	return nil
}

func (j *Jam) GetRefineContext(req []string, res *string) error {
	if len(req) != 0 {
		return fmt.Errorf("Invalid number of arguments")
	}
	// get the latest refine context....
	//refinecontext := j.statedb.GetRefineContext()

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
	log.Info(module, "ListServices")
	for i, si := range j.servicesMap {
		log.Info(module, "ListServices", "i", i, "si", si.String())
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

func (j *Jam) NewService(req []string, res *string) error {
	if len(req) != 1 {
		return fmt.Errorf("Invalid number of arguments")
	}
	service_name := req[0]
	service_code, err := j.LoadService(service_name)
	if err != nil {
		*res = err.Error()
		return err
	}
	service_code_hash := common.Blake2Hash(service_code)
	// fmt.Printf("name: %s, codelen: %d, hash: %v\n", service_name, len(service_code), service_code_hash)
	bootstrapCode, err := types.ReadCodeWithMetadata(statedb.BootstrapServiceFile, "bootstrap")
	if err != nil {
		*res = err.Error()
		return err
	}
	bootstrapService := uint32(statedb.BootstrapServiceCode)
	bootstrapCodeHash := common.Blake2Hash(bootstrapCode)
	refine_context := j.statedb.GetRefineContext()
	codeWorkPackage := types.WorkPackage{
		Authorization:         []byte(""),
		AuthCodeHost:          bootstrapService,
		AuthorizationCodeHash: bootstrap_auth_codehash,
		ParameterizationBlob:  []byte{},
		RefineContext:         refine_context,
		WorkItems: []types.WorkItem{
			{
				Service:            bootstrapService,
				CodeHash:           bootstrapCodeHash,
				Payload:            append(service_code_hash.Bytes(), binary.LittleEndian.AppendUint32(nil, uint32(len(service_code)))...),
				RefineGasLimit:     1000,
				AccumulateGasLimit: 1000,
				ImportedSegments:   make([]types.ImportSegment, 0),
				ExportCount:        0,
			},
		},
	}
	j.preimagesMutex.Lock()
	j.preimages[service_code_hash] = service_code
	j.preimagesMutex.Unlock()
	if len(j.statedb.GuarantorAssignments) == 0 {
		*res = fmt.Sprintf("No guarantor assignments, current state on %v\n", j.statedb.Block.Header.Hash())
		return nil
	}

	core0_peers := j.GetCoreCoWorkersPeers(0)
	// ramdom pick the index from 0, 1, 2
	if len(core0_peers) < 3 {
		*res = "Not enough core peers"
		return nil
	}
	randomIdx := rand.Intn(3)
	done := make(chan error, 1)
	go func() {
		done <- core0_peers[randomIdx].SendWorkPackageSubmission(codeWorkPackage, types.ExtrinsicsBlobs{}, 0)
	}()

	select {
	case err = <-done:
		if err != nil {
			fmt.Printf("SendWorkPackageSubmission ERR %v\n", err)
			*res = err.Error()
			return err
		}
	case <-time.After(2 * time.Second):
		// fmt.Println("SendWorkPackageSubmission timed out")
		*res = "SendWorkPackageSubmission timed out"
		return fmt.Errorf("SendWorkPackageSubmission timed out")
	}
	new_service_idx := uint32(0)
	// wait for the new service to be created

	var stateDB *statedb.StateDB
	stateDB = j.statedb
	if stateDB != nil && stateDB.Block != nil {
		for {
			stateDB = j.statedb
			stateRoot := stateDB.Block.GetHeader().ParentStateRoot
			t, _ := trie.InitMerkleTreeFromHash(stateRoot.Bytes(), j.store)
			k := common.ServiceStorageKey(bootstrapService, []byte{0, 0, 0, 0})
			service_account_byte, ok, err := t.GetServiceStorage(bootstrapService, k)
			if err != nil || !ok {
				time.Sleep(1 * time.Second)
				continue
			}
			decoded_new_service_idx := uint32(types.DecodeE_l(service_account_byte))
			service_account, ok, err := j.statedb.GetService(decoded_new_service_idx)
			if err != nil || !ok {
				return fmt.Errorf("GetService failed:%v", err)
			}
			service_account_code_hash := service_account.CodeHash
			if service_code_hash == service_account_code_hash {
				new_service_idx = decoded_new_service_idx
				break
			}
			time.Sleep(1 * time.Second)
		}
		err = j.BroadcastPreimageAnnouncement(new_service_idx, service_code_hash, uint32(len(service_code)), service_code)
		if err != nil {
			log.Error(debugP, "BroadcastPreimageAnnouncement", "err", err)
		}

	}

	service_info := types.ServiceInfo{
		ServiceIndex:    new_service_idx,
		ServiceCodeHash: service_code_hash,
	}
	service_info_json, err := json.Marshal(service_info)
	if err != nil {
		*res = err.Error()
	}
	*res = string(service_info_json)
	fmt.Printf("NewService %s\n", service_name)
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
func (n *Node) StartRPCServer(port int) {
	n.NodeContent.nodeSelf = n
	n.NodeContent.startRPCServerImpl(port)
}

func (n *NodeContent) startRPCServerImpl(port int) {
	jam := new(Jam)
	jam.NodeContent = n
	// register the rpc methods
	rpc.RegisterName("jam", jam)
	rpc_port := port + 1200
	address := fmt.Sprintf(":%d", rpc_port)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		fmt.Println("Failed to start RPC server:", err)
		return
	}
	defer listener.Close()
	fmt.Println("RPC server started, listening on", address)

	// Listen for requests
	for {
		time.Sleep(1 * time.Millisecond)
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

func LoadRPCClients(peerInfoMap map[uint16]*PeerInfo) (nodeClients []*NodeClient, err error) {
	nodeClients = make([]*NodeClient, 0)
	for _, peerInfo := range peerInfoMap {
		nodeClient, err := CreateNodeClient(*peerInfo)
		if err != nil {
			return nil, err
		}
		nodeClients = append(nodeClients, nodeClient)
	}
	return nodeClients, nil
}

func CreateNodeClient(peerInfo PeerInfo) (*NodeClient, error) {
	address := peerInfo.PeerAddr
	// get the port
	host, portStr, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("invalid address: %v", err)
	}
	portInt, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("invalid port: %v", err)
	}
	rpc_address := net.JoinHostPort(host, strconv.Itoa(portInt+1200))
	client, err := rpc.Dial("tcp", rpc_address)
	if err != nil {
		return nil, err
	}
	nodeClient := NodeClient{
		PeerInfo: &peerInfo,
		Client:   client,
	}
	return &nodeClient, nil
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
