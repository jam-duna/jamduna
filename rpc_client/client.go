package rpcclient

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/rpc"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/colorfulnotion/jam/common"

	//	"github.com/colorfulnotion/jam/node"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

type NodeClient struct {
	coreIndex  uint16
	client     *rpc.Client
	baseClient *rpc.Client
	baseIdx    uint16
	servers    []string

	mu sync.Mutex
}

func NewNodeClient(coreIndex uint16, servers []string) (*NodeClient, error) {
	baseclient, err := rpc.Dial("tcp", servers[0])
	if err != nil {
		return nil, err
	}
	return &NodeClient{
		baseClient: baseclient,
		client:     nil,
		coreIndex:  coreIndex,
		servers:    servers,
	}, nil
}

func (c *NodeClient) GetClient() *rpc.Client {
	idx, err := c.GetCoreCoWorkersPeers()
	if err != nil {
		return c.baseClient
	}

	client, err := rpc.Dial("tcp", c.servers[idx[0]])
	if err != nil {
		return c.baseClient
	}
	c.client = client
	return client
}

func (c *NodeClient) Close() error {
	//return c.Client.Close()
	return nil
}

// ----------------- client side -----------------
func (c *NodeClient) GetCoreCoWorkersPeers() (idx []uint16, err error) {
	var jsonStr string
	err = c.baseClient.Call("jam.GetCoreCoWorkersPeers", []string{fmt.Sprintf("%d", c.coreIndex)}, &jsonStr)
	if err != nil {
		return
	}
	err = json.Unmarshal([]byte(jsonStr), &idx)
	if err != nil {
		return
	}
	return idx, nil
}

func (c *NodeClient) GetState() (s *statedb.StateSnapshot) {
	var jsonStr string
	err := c.GetClient().Call("jam.State", []string{"latest"}, &jsonStr)
	if err != nil {
		return nil
	}
	var snapshot statedb.StateSnapshot
	err = json.Unmarshal([]byte(jsonStr), &snapshot)
	if err != nil {
		return nil
	}
	return &snapshot
}

func (nc *NodeClient) GetCurrJCE() (uint32, error) {
	var result uint32
	err := nc.GetClient().Call("jam.GetCurrJCE", struct{}{}, &result)
	return result, err
}

func (c *NodeClient) AddPreimage(preimage []byte) (common.Hash, error) {
	var codeHash common.Hash
	err := c.GetClient().Call("jam.AddPreimage", preimage, &codeHash)
	return codeHash, err
}

func (c *NodeClient) GetRefineContext() (types.RefineContext, error) {
	var jsonStr string
	err := c.GetClient().Call("jam.GetRefineContext", []string{}, &jsonStr)
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

func (c *NodeClient) SubmitWorkPackage(workPackageReq types.WorkPackageRequest) error {
	// Marshal the WorkPackageRequest to JSON
	reqBytes, err := json.Marshal(workPackageReq)
	if err != nil {
		return fmt.Errorf("failed to marshal work package request: %w", err)
	}

	// Prepare the request as a one-element string slice
	req := []string{string(reqBytes)}

	var res string
	// Call the remote RPC method
	err = c.GetClient().Call("jam.SubmitWorkPackage", req, &res)
	if err != nil {
		fmt.Printf("SubmitWorkPackage err2%v", err)
		return err
	}
	return nil
}

func (c *NodeClient) ServiceValue(serviceIndex uint32, storageHash common.Hash) ([]byte, bool, error) {
	req := []string{
		strconv.FormatUint(uint64(serviceIndex), 10),
		storageHash.Hex(),
	}
	var res string
	err := c.GetClient().Call("jam.ServiceValue", req, &res)
	if err != nil {
		return nil, false, err
	}
	storageBytes := common.Hex2Bytes(res)
	return storageBytes, true, nil
}

func (c *NodeClient) WaitForPreimage(serviceIndex uint32, preimage []byte) (err error) {
	ctxWait, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	preimageHash := common.Blake2Hash(preimage)
	for {
		select {
		case <-ctxWait.Done():
			return fmt.Errorf("Timed out waiting for preimage to appear")
		case <-time.After(1 * time.Second):
			_, _, l, err := c.ServicePreimage(serviceIndex, preimageHash)
			if err != nil {
				continue
			}
			if uint32(len(preimage)) == l {
				return nil
			}

		}
	}
}

func HasReport(s *statedb.StateSnapshot, workPackageHash common.Hash) bool {
	for _, b := range s.RecentBlocks {
		for _, x := range b.Reported {
			if x.WorkPackageHash == workPackageHash {
				return true
			}
		}
	}
	return false
}

func (c *NodeClient) WaitForWorkPackage(coreIndex uint16, workPackageHash common.Hash) (err error) {
	ctxWait, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
waitPending:
	for {
		select {
		case <-ctxWait.Done():
			return fmt.Errorf("Timed out waiting for work report to appear")
		case <-time.After(1 * time.Second):
			s := c.GetState()
			if s != nil {
				if HasReport(s, workPackageHash) {
					break waitPending
				}
				core0 := s.AvailabilityAssignments[coreIndex]
				if core0 != nil {
					if core0.WorkReport.AvailabilitySpec.WorkPackageHash == workPackageHash {

						break waitPending
					}
				}
			} else {

			}
		}
	}
	fmt.Printf("Found WPH in reported..")
waitClear:
	for {
		select {
		case <-ctxWait.Done():
			return fmt.Errorf("Timed out waiting for work report to clear")
		case <-time.After(1 * time.Second):
			s := c.GetState()
			if s != nil {
				core0 := s.AvailabilityAssignments[coreIndex]
				if core0 == nil {
					break waitClear
				}
			} else {

			}
		}
	}
	fmt.Printf("cleared\n")
	return nil
}

func (c *NodeClient) NewService(refineContext types.RefineContext, serviceName string, serviceCode []byte, serviceIDs []uint32) (newServiceIdx uint32, err error) {
	serviceCodeHash := common.Blake2Hash(serviceCode)
	bootstrapCode, err := types.ReadCodeWithMetadata(statedb.BootstrapServiceFile, "bootstrap")
	if err != nil {
		return
	}
	bootstrapCodeHash := common.Blake2Hash(bootstrapCode)
	bootstrapService := uint32(statedb.BootstrapServiceCode)
	var auth_code_bytes, _ = os.ReadFile(common.GetFilePath(statedb.BootStrapNullAuthFile))
	var auth_code = statedb.AuthorizeCode{
		PackageMetaData:   []byte("bootstrap"),
		AuthorizationCode: auth_code_bytes,
	}

	var auth_code_encoded_bytes, _ = auth_code.Encode()
	bootstrap_auth_codehash := common.Blake2Hash(auth_code_encoded_bytes) //pu

	codeWP := types.WorkPackage{
		Authorization:         []byte(""),
		AuthCodeHost:          bootstrapService,
		AuthorizationCodeHash: bootstrap_auth_codehash,
		ParameterizationBlob:  []byte{},
		RefineContext:         refineContext,
		WorkItems: []types.WorkItem{{
			Service:            bootstrapService,
			CodeHash:           bootstrapCodeHash,
			Payload:            append(serviceCodeHash.Bytes(), binary.LittleEndian.AppendUint32(nil, uint32(len(serviceCode)))...),
			RefineGasLimit:     1000,
			AccumulateGasLimit: 1000,
			ImportedSegments:   nil,
			ExportCount:        0,
		}},
	}

	var wpr types.WorkPackageRequest
	wpr.CoreIndex = 0 // this is OK
	wpr.WorkPackage = codeWP
	wpr.ExtrinsicsBlobs = types.ExtrinsicsBlobs{}
	wpHash := codeWP.Hash()
	fmt.Printf("Submitting WP %s\n", wpHash)
	err = c.SubmitWorkPackage(wpr)
	if err != nil {
		fmt.Printf("SubmitWorkPackage ERR %v", err)
		return
	}
	err = c.WaitForWorkPackage(wpr.CoreIndex, wpHash)
	if err != nil {
		fmt.Printf("WaitForWorkPackage ERR %v", err)
		return
	}
	newServiceIdx, err = c.GetBootstrapService()
	if err != nil {
		fmt.Printf("GetBootstrapService ERR %v", err)
		return
	}

	if err = c.SubmitPreimage(newServiceIdx, serviceCode); err != nil {
		fmt.Printf("SubmitPreimage ERR %v", err)
		return
	}
	err = c.WaitForPreimage(newServiceIdx, serviceCode)
	if err != nil {
		fmt.Printf("WaitForPreimage ERR %v", err)
		return
	}
	fmt.Printf("NewService %d\n", newServiceIdx)
	return
}

func (c *NodeClient) LoadServices(services []string) (new_service_map map[string]types.ServiceInfo, err error) {
	fmt.Printf("LoadServices: NewServices %v\n", services)
	new_service_map = make(map[string]types.ServiceInfo)
	serviceIDs := make([]uint32, 0)
	for _, service_name := range services {

		refineContext, err := c.GetRefineContext()
		if err != nil {
			return new_service_map, err
		}
		service_path := fmt.Sprintf("/services/%s.pvm", service_name)
		serviceCode, err := types.ReadCodeWithMetadata(service_path, service_name)
		if err != nil {
			return nil, err
		}
		codeHash := common.Blake2Hash(serviceCode)
		new_serviceIdx, err := c.NewService(refineContext, service_name, serviceCode, serviceIDs)
		if err != nil {
			return nil, err
		}
		new_service_map[service_name] = types.ServiceInfo{
			ServiceIndex:    new_serviceIdx,
			ServiceCodeHash: codeHash,
		}
		serviceIDs = append(serviceIDs, new_serviceIdx)
	}
	fmt.Printf("LoadServices: DONE\n")
	return new_service_map, nil
}

func (c *NodeClient) GetBootstrapService() (service_index uint32, err error) {
	k := common.ServiceStorageKey(0, []byte{0, 0, 0, 0})
	service_account_byte, ok, err := c.ServiceValue(0, k)
	if err != nil || !ok {
		return 0, fmt.Errorf("failed to get bootstrap service: %v", err)
	}
	service_index = uint32(types.DecodeE_l(service_account_byte))
	return service_index, nil
}

func (c *NodeClient) SubmitPreimage(serviceIndex uint32, preimage []byte) error {
	serviceIndexStr := strconv.FormatUint(uint64(serviceIndex), 10)
	preimageStr := common.Bytes2Hex(preimage)
	req := []string{serviceIndexStr, preimageStr}
	fmt.Printf("NodeClient: SubmitPreimage(service=%d, preimage=%s, %d bytes)\n", serviceIndex, common.Blake2Hash(preimage), len(preimage))
	var res string
	err := c.GetClient().Call("jam.SubmitPreimage", req, &res)
	if err != nil {
		return err
	}
	return nil
}

func (c *NodeClient) ServicePreimage(serviceIndex uint32, codeHash common.Hash) ([]byte, string, uint32, error) {
	serviceIndexStr := strconv.FormatUint(uint64(serviceIndex), 10)
	codeHashStr := codeHash.Hex()
	req := []string{serviceIndexStr, codeHashStr}

	var res string
	err := c.GetClient().Call("jam.ServicePreimage", req, &res)
	if err != nil {
		return nil, "", 0, err
	}

	type servicePreimageResponse struct {
		Metadata string `json:"metadata"`
		RawBytes string `json:"rawbytes"`
		Length   uint32 `json:"length"`
	}
	var parsed servicePreimageResponse
	_ = json.Unmarshal([]byte(res), &parsed)

	metadata := parsed.Metadata
	length := parsed.Length
	rawBytes := common.Hex2Bytes(parsed.RawBytes)
	return rawBytes, metadata, length, nil
}

func (c *NodeClient) GetAvailabilityAssignments(coreIdx uint32) (*statedb.Rho_state, error) {
	// Convert coreIdx to a string
	coreIdxStr := strconv.FormatUint(uint64(coreIdx), 10)
	req := []string{coreIdxStr}

	var res string
	// Make the RPC call to "jam.GetAvailabilityAssignments"
	err := c.GetClient().Call("jam.GetAvailabilityAssignments", req, &res)
	if err != nil {
		return nil, err
	}

	// Unmarshal the JSON response into an AvailabilityAssignment
	var rho_state statedb.Rho_state
	err = json.Unmarshal([]byte(res), &rho_state)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal availability assignment: %w", err)
	}
	return &rho_state, nil
}

func (c *NodeClient) Segment(wphash common.Hash, segmentIndex uint16) ([]byte, error) {
	// Convert the segment index to a string
	segmentIndexStr := strconv.FormatUint(uint64(segmentIndex), 10)
	req := []string{wphash.Hex(), segmentIndexStr}

	var res string
	err := c.GetClient().Call("jam.Segment", req, &res)
	if err != nil {
		return nil, err
	}

	// Convert the hex string back to bytes
	// segmentBytes := common.Hex2Bytes(res)

	type getSegmentResponse struct {
		Segment       []byte        `json:"segment"`
		Justification []common.Hash `json:"justification"`
	}
	var parsed getSegmentResponse
	_ = json.Unmarshal([]byte(res), &parsed)

	segmentBytes := parsed.Segment

	return segmentBytes, nil
}
