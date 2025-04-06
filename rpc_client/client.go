package rpcclient

import (
	"encoding/json"
	"fmt"
	"net/rpc"
	"strconv"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

type NodeClient struct {
	*rpc.Client
	server_address string
	state          *statedb.StateSnapshot
}

func NewNodeClient(server_address string) (*NodeClient, error) {
	client, err := rpc.Dial("tcp", server_address)
	if err != nil {
		return nil, err
	}
	return &NodeClient{client, server_address, nil}, nil
}

func (c *NodeClient) RunState() {
	ticker := time.NewTicker(1 * time.Second)
	for {
		select {
		case <-ticker.C:
			res := string("")
			c.Call("jam.GetLatestState", []string{}, &res)
			//unmarshal the json string into a state snapshot
			state := &statedb.StateSnapshot{}
			err := json.Unmarshal([]byte(res), state)
			if err != nil {
				fmt.Printf("State Parse Error: %s\n", err)
				continue
			} else {
				c.state = state
			}
		}
	}
}

func (c *NodeClient) Close() error {
	return c.Client.Close()
}

// ----------------- client side -----------------
func (c *NodeClient) GetState() *statedb.StateSnapshot {
	return c.state
}

func (nc *NodeClient) GetCurrJCE() (uint32, error) {
	var result uint32
	err := nc.Client.Call("jam.GetCurrJCE", struct{}{}, &result)
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

func (c *NodeClient) SubmitWorkPackage(workPackageReq types.WorkPackageRequest) error {
	// Marshal the WorkPackageRequest to JSON
	reqBytes, err := json.Marshal(workPackageReq)
	if err != nil {
		fmt.Errorf("failed to marshal work package request: %w", err)
	}

	// Prepare the request as a one-element string slice
	req := []string{string(reqBytes)}

	var res string
	// Call the remote RPC method
	err = c.Client.Call("jam.SubmitWorkPackage", req, &res)
	if err != nil {
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
	err := c.Client.Call("jam.ServiceValue", req, &res)
	if err != nil {
		return nil, false, err
	}
	storageBytes := common.Hex2Bytes(res)
	return storageBytes, true, nil
}

func (c *NodeClient) NewService(serviceName string) (types.ServiceInfo, error) {
	req := []string{serviceName}
	var res string
	err := c.Client.Call("jam.NewService", req, &res)
	if err != nil {
		return types.ServiceInfo{}, err
	}
	// Unmarshal the JSON response into a ServiceInfo
	var serviceInfo types.ServiceInfo
	err = json.Unmarshal([]byte(res), &serviceInfo)
	if err != nil {
		return types.ServiceInfo{}, fmt.Errorf("failed to unmarshal service info: %w", err)
	}
	return serviceInfo, nil
}
func (c *NodeClient) LoadServices(services []string) (map[string]types.ServiceInfo, error) {
	old_service_index := uint32(0)
	new_service_map := make(map[string]types.ServiceInfo)
	for _, service_name := range services {
		service_info, err := c.NewService(service_name)
		if err != nil {
			return nil, err
		}
		// Wait for the service to be ready
		time.Sleep(2 * types.SecondsPerSlot * time.Second) // this delay is necessary to ensure the first block is ready, nor it will send the wrong anchor slot
		fmt.Printf("NewService %s %v\n", service_name, service_info)
		for {
			new_service_index, err := c.GetBootstrapService()
			if err != nil {
				fmt.Printf("%v\n", err)
				time.Sleep(1 * time.Second)
				continue
			}
			if new_service_index != old_service_index && new_service_index != 0 {
				old_service_index = new_service_index

				new_service_map[service_name] = service_info
				fmt.Printf("Service %s loaded with index %d, hash %v\n", service_name, service_info.ServiceIndex, service_info.ServiceCodeHash)
				break
			}
		}
	}
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

	var res string
	err := c.Client.Call("jam.SubmitPreimage", req, &res)
	if err != nil {
		return err
	}
	return nil
}

func (c *NodeClient) ServicePreimage(serviceIndex uint32, codeHash common.Hash) ([]byte, error) {
	serviceIndexStr := strconv.FormatUint(uint64(serviceIndex), 10)
	codeHashStr := codeHash.Hex()
	req := []string{serviceIndexStr, codeHashStr}

	var res string
	err := c.Client.Call("jam.ServicePreimage", req, &res)
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
	return &rho_state, nil
}
