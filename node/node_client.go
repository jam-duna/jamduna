package node

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/rpc"
	"strconv"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

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

func (c *NodeClient) GetState() (*statedb.StateSnapshot, error) {
	var jsonStr string
	err := c.Client.Call("jam.State", []string{}, &jsonStr)
	if err != nil {
		return &statedb.StateSnapshot{}, err
	}
	var sdb statedb.StateSnapshot
	err = json.Unmarshal([]byte(jsonStr), &sdb)
	if err != nil {
		return &statedb.StateSnapshot{}, fmt.Errorf("failed to unmarshal refine context: %w", err)
	}
	return &sdb, nil
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

func (c *NodeClient) GetService(serviceID uint32) (sa *types.ServiceAccount, ok bool, err error) {
	var jsonStr string
	err = c.Client.Call("jam.Service", []string{}, &jsonStr)
	if err != nil {
		return &types.ServiceAccount{}, false, err
	}

	var service types.ServiceAccount
	err = json.Unmarshal([]byte(jsonStr), &service)
	if err != nil {
		return nil, false, fmt.Errorf("failed to unmarshal refine context: %w", err)
	}
	return &service, true, nil

}

func (c *NodeClient) SubmitAndWaitForWorkPackage(ctx context.Context, workPackageReq *WorkPackageRequest) (wph common.Hash, err error) {
	workPackage := workPackageReq.WorkPackage

	rc, err := c.GetRefineContext()
	if err != nil {
		return wph, err
	}
	workPackage.RefineContext = rc
	wph = workPackage.Hash()
	err = c.SendWorkPackage(workPackageReq)
	if err != nil {
		return wph, err
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return wph, fmt.Errorf("Timed out waiting for work package %s to be accumulated", wph)
		case <-ticker.C:
			sdb, err := c.GetState()
			if err != nil {
				return wph, err
			}
			history := sdb.AccumulationHistory[types.EpochLength-1]
			for _, h := range history.WorkPackageHash {
				if h == wph {
					return wph, nil
				}
			}
		}
	}
}

func (c *NodeClient) SendWorkPackage(workPackageReq *WorkPackageRequest) error {
	// Marshal the WorkPackageRequest to JSON
	reqBytes, err := json.Marshal(workPackageReq)
	if err != nil {
		return fmt.Errorf("failed to marshal work package request: %w", err)
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

func (c *NodeClient) SubmitAndWaitForPreimage(ctx context.Context, serviceIndex uint32, preimage []byte) error {
	err := c.SubmitPreimage(serviceIndex, preimage)
	if err != nil {
		return err
	}
	preimageHash := common.Blake2Hash(preimage)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("Timed out waiting for preimage")
		case <-ticker.C:
			img, err := c.GetServicePreimage(serviceIndex, preimageHash)
			if err != nil {
				log.Error(module, "SubmitAndWaitForPreimage", "err", err)
			}
			if bytes.Compare(img, preimage) == 0 {
				return nil
			}
		}
	}
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
