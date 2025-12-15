package node

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/rpc"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb"
	types "github.com/colorfulnotion/jam/types"
	"github.com/gorilla/websocket"
)

type NodeClient struct {
	// PeerInfo *PeerInfo
	// Client   *rpc.Client
	//coreIndex  uint16
	client     *rpc.Client
	baseClient *rpc.Client
	server     string

	state   *statedb.StateSnapshot
	muState sync.Mutex

	Preimage       map[common.Hash][]byte
	WorkPackage    map[common.Hash]string
	ServiceValue   map[string][]byte
	ServiceInfo    map[uint32]types.ServiceAccount
	ServiceRequest map[common.Hash][]uint32
	HeaderHash     common.Hash
	Statistics     types.ValidatorStatistics

	wsurl   string
	wsConn  *websocket.Conn // websocket connection
	wsMutex sync.Mutex      // to protect writes
}

type envelope struct {
	Method string          `json:"method"`
	Result json.RawMessage `json:"result"`
}

func NewNodeClient(server, wsUrl string) (*NodeClient, error) {
	log.InitLogger("debug")
	baseclient, err := rpc.Dial("tcp", server)
	if err != nil {
		log.Error(log.Node, "NewNodeClient", "endpoint", server, "err", err)
		return nil, err
	}

	c := &NodeClient{
		baseClient:     baseclient,
		client:         nil,
		wsurl:          wsUrl,
		server:         server,
		state:          nil,
		Preimage:       make(map[common.Hash][]byte),
		WorkPackage:    make(map[common.Hash]string),
		ServiceValue:   make(map[string][]byte),
		ServiceInfo:    make(map[uint32]types.ServiceAccount),
		ServiceRequest: make(map[common.Hash][]uint32),
	}
	c.ConnectWebSocket(wsUrl)
	return c, nil
}

func (c *NodeClient) SetJCEManager(jceManager *ManualJCEManager) (err error) {
	return nil
}
func (c *NodeClient) GetJCEManager() (jceManager *ManualJCEManager, err error) {
	return nil, nil
}

func (c *NodeClient) ConnectWebSocket(url string) error {
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return fmt.Errorf("failed to connect websocket: %w", err)
	}
	c.wsConn = conn
	go c.listenWebSocket()
	return nil
}
func (c *NodeClient) listenWebSocket() {
	reconnectDelay := time.Second

	for {
		if c.wsConn == nil {
			return
		}
		_, msg, err := c.wsConn.ReadMessage()
		if err != nil {
			log.Warn(log.Node, "listenWebSocket read error", "err", err)
			c.reconnectWebSocket()
			time.Sleep(reconnectDelay)
			continue
		}

		var envelope envelope
		if err := json.Unmarshal(msg, &envelope); err != nil {
			log.Warn(log.Node, "listenWebSocket: Failed to parse WebSocket envelope", "err", err)
			continue
		}
		c.handleEnvelope(&envelope)
	}
}

func (c *NodeClient) reconnectWebSocket() {
	c.wsMutex.Lock()
	defer c.wsMutex.Unlock()

	if c.wsConn != nil {
		c.wsConn.Close()
		c.wsConn = nil
	}

	wsUrl := os.Getenv("WS_URL") // save ws URL when connecting
	if wsUrl == "" {
		wsUrl = c.wsurl
	}
	conn, _, err := websocket.DefaultDialer.Dial(wsUrl, nil)
	if err != nil {
		log.Error(log.Node, "reconnectWebSocket: Failed to reconnect", "err", err)
		return
	}
	c.wsConn = conn
	go c.listenWebSocket()
}

func (c *NodeClient) handleEnvelope(envelope *envelope) error {
	switch envelope.Method {
	case SubBestBlock:
		var result struct {
			BlockHash  string `json:"blockHash"`
			HeaderHash string `json:"headerHash"`
		}
		if err := json.Unmarshal(envelope.Result, &result); err != nil {
			log.Warn(log.Node, "listenWebSocket: Failed to parse BlockAnnouncement", "err", err)
			return err
		}
		// fmt.Printf("Best block: %s\n", result.BlockHash)
		c.HeaderHash = common.Hex2Hash(result.HeaderHash)

		go c.GetState(result.HeaderHash)
	case SubFinalizedBlock:
		var result struct {
			BlockHash  string `json:"blockHash"`
			HeaderHash string `json:"headerHash"`
		}
		if err := json.Unmarshal(envelope.Result, &result); err != nil {
			log.Warn(log.Node, "listenWebSocket: Failed to parse BlockAnnouncement", "err", err)
			return err
		}
		// fmt.Printf("Finalized block: %s\n", result.BlockHash)

	case SubStatistics:
		var payload struct {
			HeaderHash string                    `json:"headerHash"`
			Statistics types.ValidatorStatistics `json:"statistics"`
		}
		if err := json.Unmarshal(envelope.Result, &payload); err != nil {
			log.Warn(log.Node, "listenWebSocket: Failed to parse StatisticsUpdate", "err", err)
			return err
		}
		c.Statistics = payload.Statistics

	case SubServiceInfo:
		var payload struct {
			ServiceID uint32               `json:"serviceID"`
			Info      types.ServiceAccount `json:"info"`
		}
		if err := json.Unmarshal(envelope.Result, &payload); err != nil {
			log.Warn(log.Node, "listenWebSocket: Failed to parse ServiceInfoUpdate", "err", err)
			return err
		}
		c.ServiceInfo[payload.ServiceID] = payload.Info
	case SubServiceValue:
		var payload struct {
			ServiceID  uint32      `json:"serviceID"`
			HeaderHash common.Hash `json:"headerHash"`
			Slot       uint32      `json:"slot"`
			Hash       common.Hash `json:"hash"`
			Key        string      `json:"key"`
			Value      string      `json:"value"`
		}
		if err := json.Unmarshal(envelope.Result, &payload); err != nil {
			log.Warn(log.Node, "listenWebSocket: Failed to parse SubServiceValue", "err", err)
			return err
		}
		c.ServiceValue[common.Bytes2Hex(payload.Hash[:])] = common.Hex2Bytes(payload.Value)

	case SubServicePreimage:
		var payload struct {
			ServiceID uint32      `json:"serviceID"`
			Hash      common.Hash `json:"hash"`
			Preimage  string      `json:"preimage"`
		}
		if err := json.Unmarshal(envelope.Result, &payload); err != nil {
			log.Warn(log.Node, "listenWebSocket: Failed to parse ServiceInfoUpdate", "err", err)
			return err
		}
		c.Preimage[payload.Hash] = common.Hex2Bytes(payload.Preimage)

	case SubServiceRequest:
		var payload struct {
			ServiceID uint32      `json:"serviceID"`
			Hash      common.Hash `json:"hash"`
			Timeslots []uint32    `json:"timeslots"`
		}
		if err := json.Unmarshal(envelope.Result, &payload); err != nil {
			log.Warn(log.Node, "listenWebSocket: Failed to parse SubServiceRequest", "err", err)
			return err
		}
		c.ServiceRequest[payload.Hash] = payload.Timeslots

	case SubWorkPackage:
		var payload struct {
			WorkPackageHash common.Hash `json:"workPackageHash"`
			Status          string      `json:"status"`
		}
		if err := json.Unmarshal(envelope.Result, &payload); err != nil {
			log.Warn(log.Node, "listenWebSocket: Failed to parse SubWorkPackage", "err", err)
			return err
		}
		c.WorkPackage[payload.WorkPackageHash] = payload.Status

	default:
		log.Warn(log.Node, "listenWebSocket: Failed to parse method", "method", envelope.Method)
	}

	return nil
}

func (c *NodeClient) safeWriteWebSocket(msg interface{}) error {
	c.wsMutex.Lock()
	defer c.wsMutex.Unlock()
	if c.wsConn == nil {
		return fmt.Errorf("WebSocket not connected")
	}
	fmt.Printf("Sending WebSocket message: %v\n", msg)
	return c.wsConn.WriteJSON(msg)
}

func (c *NodeClient) Subscribe(method string, params map[string]interface{}) error {
	return c.safeWriteWebSocket(map[string]interface{}{
		"method": method,
		"params": params,
	})
}

func (c *NodeClient) Unsubscribe(method string, params map[string]interface{}) error {
	return c.safeWriteWebSocket(map[string]interface{}{
		"method": "unsubscribe",
		"params": map[string]interface{}{
			"method": method,
			"params": params,
		},
	})
}

func (c *NodeClient) GetClient(possibleCores ...uint16) *rpc.Client {
	var coreIdx uint16
	// select random core
	if len(possibleCores) > 0 {
		// this should be a list of cores you have coretime for
		coreIdx = possibleCores[rand.Intn(len(possibleCores))]
	} else {
		coreIdx = uint16(rand.Intn(types.TotalCores))
	}
	idx, err := c.GetCoreCoWorkersPeers(coreIdx)
	if err != nil {
		log.Error(log.Node, "GetClient: GetCoreCoWorkersPeers", "err", err)
		return c.baseClient
	}
	if len(idx) == 0 {
		log.Warn(log.Node, "GetClient: No co-workers found for core", "coreIdx", coreIdx)
		return c.baseClient
	}

	// Select random peer
	selected := idx[rand.Intn(len(idx))]
	if selected == 5 {
		selected = idx[0]
	}
	client, err := rpc.Dial("tcp", c.server)
	if err != nil {
		log.Error(log.Node, "GetClient: Dial", "selected", selected, "c.server", c.server, "err", err)
		return c.baseClient
	}
	c.client = client
	return client
}

func (c *NodeClient) CallWithRetry(method string, args interface{}, reply interface{}, possibleCores ...uint16) error {
	if method == "jam.SubmitPreimage" {
		fmt.Printf("CallWithRetry: method=%s, possibleCores=%v\n", method, possibleCores)
	} else {
		fmt.Printf("CallWithRetry: method=%s, args=%v\n", method, args)
	}
	const maxRetries = 3
	var err error
	for attempt := 0; attempt < maxRetries; attempt++ {
		client := c.GetClient(possibleCores...)
		if client == nil {
			return fmt.Errorf("no available RPC client")
		}
		err = client.Call(method, args, reply)
		if err == nil {
			return nil
		}
		if strings.Contains(err.Error(), "ReadServiceStorage not found") {
			return nil
		}
		if strings.Contains(err.Error(), "connection reset") || strings.Contains(err.Error(), "broken pipe") {
			log.Warn(log.Node, "BaseClient broken, reconnecting")
			c.baseClient.Close()
			c.baseClient, _ = rpc.Dial("tcp", c.server)
		}

		log.Warn(log.Node, "CallWithRetry", "method", method, "attempt", attempt+1, "err", err)
		// Reconnect
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("failed after %d retries: %w", maxRetries, err)
}

func (c *NodeClient) SendCommand(command []string, nodeID int) {
	addr := c.server
	client, err := rpc.Dial("tcp", addr)
	if err != nil {
		log.Error(log.Node, "SendCommand: Dial", "addr", addr, "err", err)
		return
	}
	defer client.Close()

	var response string
	err = client.Call("jam.NodeCommand", command, &response)
	if err != nil {
		log.Error(log.Node, "SendCommand: jam.NodeCommand", "addr", addr, "err", err)
		return
	}
	decoded, err := strconv.Unquote(`"` + response + `"`)
	if err != nil {
		decoded = strings.ReplaceAll(response, `\n`, "\n")
		decoded = strings.ReplaceAll(decoded, `\t`, "\t")
	}
	fmt.Printf("response: %s\n", decoded)
}

func (c *NodeClient) BroadcastCommand(command []string, exceptNode []int) {
	client, err := rpc.Dial("tcp", c.server)
	if err != nil {
		log.Error(log.Node, "BroadcastCommand", "addr", c.server, "err", err)
		return
	}
	defer client.Close()

	var response string
	err = client.Call("jam.NodeCommand", command, &response)
	if err != nil {
		log.Error(log.Node, "BroadcastCommand", "addr", c.server, "err", err)
		return
	}
	log.Info(log.Node, "BroadcastCommand: Response", "addr", c.server, "response", response)
}

func (c *NodeClient) Close() error {
	//return c.Client.Close()
	return nil
}

func (c *NodeClient) GetCoreCoWorkersPeers(coreIdx uint16) (idx []uint16, err error) {
	var jsonStr string
	err = c.baseClient.Call("jam.GetCoreCoWorkersPeers", []string{fmt.Sprintf("%d", coreIdx)}, &jsonStr)
	if err != nil {
		return
	}
	err = json.Unmarshal([]byte(jsonStr), &idx)
	if err != nil {
		return
	}
	return idx, nil
}

func (c *NodeClient) GetState(headerHash string) (sdb *statedb.StateSnapshot, err error) {
	var jsonStr string
	err = c.CallWithRetry("jam.State", []string{headerHash}, &jsonStr)
	if err != nil {
		return
	}
	var snapshot statedb.StateSnapshot
	err = json.Unmarshal([]byte(jsonStr), &snapshot)
	if err != nil {
		return sdb, err
	}
	c.muState.Lock()
	c.state = &snapshot
	c.muState.Unlock()

	return &snapshot, nil
}

func (c *NodeClient) GetCurrJCE() (result uint32, err error) {
	err = c.CallWithRetry("jam.GetCurrJCE", struct{}{}, &result)
	return result, err
}

func (c *NodeClient) ReadGlobalDepth(serviceID uint32) (result uint8, err error) {
	err = c.CallWithRetry("jam.ReadGlobalDepth", []string{fmt.Sprintf("%d", serviceID)}, &result)
	return result, err
}

func (c *NodeClient) GetRefineContext() (types.RefineContext, error) {
	var jsonStr string
	err := c.baseClient.Call("jam.GetRefineContext", []string{}, &jsonStr)
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

// WORK PACKAGE
func (c *NodeClient) SubmitWorkPackage(workPackageReq *types.WorkPackageBundle) error {
	// Marshal the WorkPackageBundle to JSON
	reqBytes, err := json.Marshal(workPackageReq)
	if err != nil {
		log.Warn(log.Node, "SubmitWorkPackage", "err", err)
		return fmt.Errorf("failed to marshal work package request: %w", err)
	}

	// Prepare the request as a one-element string slice
	req := []string{string(reqBytes)}

	var res string
	// Call the remote RPC method
	err = c.GetClient().Call("jam.SubmitWorkPackage", req, &res)
	if err != nil {
		log.Warn(log.Node, "SubmitWorkPackage: jam.SubmitWorkPackage", "err", err)
		return err
	}
	return nil
}

func (c *NodeClient) SubmitAndWaitForWorkPackageBundle(ctx context.Context, workPackageReq *types.WorkPackageBundle) (stateRoot common.Hash, ts uint32, err error) {
	stateRoots, timeslots, err := c.SubmitAndWaitForWorkPackageBundles(ctx, []*types.WorkPackageBundle{workPackageReq})
	if err != nil {
		return common.Hash{}, 0, err
	}
	return stateRoots[0], timeslots[0], nil
}

func (c *NodeClient) SubmitAndWaitForWorkPackageBundles(ctx context.Context, reqs []*types.WorkPackageBundle) ([]common.Hash, []uint32, error) {
	log.Info(log.Node, "NodeClient SubmitAndWaitForWorkPackageBundles", "reqLen", len(reqs))
	workPackageHashes := make([]common.Hash, len(reqs))
	stateRoots := make([]common.Hash, len(reqs))
	timeslots := make([]uint32, len(reqs))
	workPackageLastStatus := make(map[common.Hash]string)

	// Initialize refine context
	refineCtx, err := c.GetRefineContext()
	if err != nil {
		return stateRoots, timeslots, err
	}
	for _, req := range reqs {
		rc := refineCtx.Clone()
		req.WorkPackage.RefineContext = *rc
	}

	// Compute hashes and track accumulation status
	for i, req := range reqs {
		hash := req.WorkPackage.Hash()
		workPackageHashes[i] = hash
		workPackageLastStatus[hash] = "pending"

		c.Subscribe(SubWorkPackage, map[string]interface{}{
			"hash": hash.String(),
		})
		if err := c.SubmitWorkPackage(req); err != nil {
			log.Warn(log.Node, "Failed to submit work package", "hash", hash, "err", err)
			return stateRoots, timeslots, err
		}
		workPackageLastStatus[hash] = "submitted"
		log.Info(log.Node, "Subscribe", "h", hash)
	}

	// Wait for accumulation
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	states := time.NewTicker(types.SecondsPerSlot * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return stateRoots, timeslots, ctx.Err()
		case <-states.C:
			state, err := c.GetState("latest")
			if err == nil {
				numacc := 0
				for i, workPackageHash := range workPackageHashes {
					for _, ah := range state.AccumulationHistory {
						for _, h := range ah.WorkPackageHash {
							if workPackageHash == h {
								log.Info(log.Node, "Work package accumulated", "hash", workPackageHash.Hex())
								numacc++
								// TODO: Get state root from NodeClient - for now use empty hash
								stateRoots[i] = common.Hash{}
								timeslots[i] = state.Timeslot
							}
						}
					}
				}
				if numacc == len(workPackageHashes) {
					log.Info(log.Node, "All work packages accumulated")
					return stateRoots, timeslots, nil
				}
			}
		case <-ticker.C:
			numacc := 0
			for _, workPackageHash := range workPackageHashes {
				if status, ok := c.WorkPackage[workPackageHash]; ok {
					switch status {
					case "accumulated":
						log.Info(log.Node, "Work package accumulated", "hash", workPackageHash.Hex())
						numacc++
					case "guaranteed":
						if workPackageLastStatus[workPackageHash] == "submitted" {
							workPackageLastStatus[workPackageHash] = "guaranteed"
							log.Info(log.Node, "Work package guaranteed", "hash", workPackageHash.Hex())
						}
					default:
						log.Info(log.Node, fmt.Sprintf("Work package status:%s", status), "hash", workPackageHash.Hex())
					}
				}
			}
			if numacc == len(workPackageHashes) {
				state, err := c.GetState("latest")
				if err == nil {
					for i := range workPackageHashes {
						stateRoots[i] = common.Hash{}
						timeslots[i] = state.Timeslot
					}
				}
				log.Info(log.Node, "All work packages accumulated")
				return stateRoots, timeslots, nil
			}

		}
	}

}

func (c *NodeClient) RobustSubmitWorkPackage(workpackage_req *types.WorkPackageBundle, maxTries int) (workPackageHash common.Hash, err error) {
	tries := 0
	for tries < maxTries {
		refine_context, err := c.GetRefineContext()
		if err != nil {
			return workPackageHash, err
		}
		workpackage_req.WorkPackage.RefineContext = refine_context
		workPackageHash = workpackage_req.WorkPackage.Hash()
		ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout)
		defer cancel()
		_, _, err = c.SubmitAndWaitForWorkPackageBundle(ctx, workpackage_req)
		if err != nil {
			log.Error(log.Node, "SendWorkPackageSubmission", "err", err)
			tries = tries + 1
		} else {
			return workPackageHash, nil
		}
	}
	return workPackageHash, fmt.Errorf("timeout after maxTries %d", maxTries)
}

// PREIMAGES
func (c *NodeClient) SubmitPreimage(serviceIndex uint32, preimage []byte) (err error) {
	serviceIndexStr := strconv.FormatUint(uint64(serviceIndex), 10)
	preimageStr := common.Bytes2Hex(preimage)
	req := []string{serviceIndexStr, preimageStr}

	var res string
	err = c.CallWithRetry("jam.SubmitPreimage", req, &res)
	if err != nil {
		return err
	}
	return nil
}

func (c *NodeClient) SubmitAndWaitForPreimage(ctx context.Context, serviceIndex uint32, preimage []byte) error {
	var wg sync.WaitGroup
	errCh := make(chan error, 1)
	preimageHash := common.Blake2Hash(preimage)

	wg.Add(1)
	go func() {
		defer wg.Done()
		c.Subscribe(SubServicePreimage, map[string]interface{}{
			"serviceID": fmt.Sprintf("%d", serviceIndex),
			"hash":      preimageHash.String(),
		})

		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		states := time.NewTicker(6 * time.Second)
		defer states.Stop()

		for {
			select {
			case <-ctx.Done():
				errCh <- fmt.Errorf("*WaitForPreimage*: context canceled or timed out (serviceID=%d, h=%s, l=%d)", serviceIndex, preimageHash, len(preimage))
				return
			case <-ticker.C:
				if img, ok := c.Preimage[preimageHash]; ok {
					if bytes.Equal(img, preimage) {
						return
					}
					return
				}
			case <-states.C:
				_, _, _, err := c.GetServicePreimage(serviceIndex, preimageHash)
				if err == nil {
					return
				}
			}
		}
	}()

	// Submit preimage
	if err := c.SubmitPreimage(serviceIndex, preimage); err != nil {
		// cancel wait if Submit fails
		errCh <- fmt.Errorf("SubmitPreimage: %w", err)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *NodeClient) GetServicePreimage(serviceIndex uint32, codeHash common.Hash) ([]byte, string, uint32, error) {
	serviceIndexStr := strconv.FormatUint(uint64(serviceIndex), 10)
	codeHashStr := codeHash.Hex()
	req := []string{serviceIndexStr, codeHashStr}

	var res string
	err := c.CallWithRetry("jam.ServicePreimage", req, &res)
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

func (c *NodeClient) GetAvailabilityAssignments(coreIdx uint32) (*statedb.CoreState, error) {
	// Convert coreIdx to a string
	coreIdxStr := strconv.FormatUint(uint64(coreIdx), 10)
	req := []string{coreIdxStr}

	var res string
	// Make the RPC call to "jam.GetAvailabilityAssignments"
	err := c.CallWithRetry("jam.GetAvailabilityAssignments", req, &res)
	if err != nil {
		return nil, err
	}

	// Unmarshal the JSON response into an AvailabilityAssignment
	var CoreState statedb.CoreState
	err = json.Unmarshal([]byte(res), &CoreState)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal availability assignment: %w", err)
	}
	return &CoreState, nil
}

func (nc *NodeClient) GetBuildVersion() (string, error) {
	var result string
	err := nc.CallWithRetry("jam.GetBuildVersion", []string{}, &result)
	return result, err
}

// SERVICE
func (c *NodeClient) GetService(serviceID uint32) (sa *types.ServiceAccount, ok bool, err error) {
	var jsonStr string
	err = c.CallWithRetry("jam.Service", []string{}, &jsonStr)
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

func (c *NodeClient) GetFinalizedBlock() (block *types.Block, err error) {
	var jsonStr string
	err = c.CallWithRetry("jam.LatestFinalizedBlock", []string{}, &jsonStr)
	if err != nil {
		return nil, err
	}
	var blockData types.Block
	err = json.Unmarshal([]byte(jsonStr), &blockData)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal finalized block: %w", err)
	}
	return &blockData, nil
}

func (c *NodeClient) GetWorkReport(requestedHash common.Hash) (wr *types.WorkReport, err error) {
	var jsonStr string
	err = c.CallWithRetry("jam.WorkReport", []string{requestedHash.Hex()}, &jsonStr)
	if err != nil {
		return nil, err
	}

	var workReport types.WorkReport
	err = json.Unmarshal([]byte(jsonStr), &workReport)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal work report: %w", err)
	}
	return &workReport, nil
}

// SERVICE STORAGE
func (c *NodeClient) GetServiceStorage(serviceIndex uint32, storageKey []byte) ([]byte, bool, error) {
	req := []string{
		strconv.FormatUint(uint64(serviceIndex), 10),
		common.Bytes2Hex(storageKey),
	}
	var res string
	err := c.CallWithRetry("jam.ServiceValue", req, &res)
	if err != nil {
		return nil, false, err
	}
	storageBytes := common.Hex2Bytes(res)
	return storageBytes, true, nil
}

func (c *NodeClient) WaitForServiceValue(serviceIndex uint32, storageKey []byte) (service_index uint32, err error) {
	ctxWait, cancel := context.WithTimeout(context.Background(), RefineTimeout)
	defer cancel()
	c.Subscribe(SubServiceValue, map[string]interface{}{"hash": common.Bytes2Hex(storageKey)})

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctxWait.Done():
			return 0, fmt.Errorf("timed out waiting for service value")
		case <-ticker.C:
			if value, ok := c.ServiceValue[common.Bytes2Hex(storageKey)]; ok {
				service_index = uint32(types.DecodeE_l(value))
				return service_index, nil
			}
		}
	}
}

// FetchJAMDASegments implements the JNode interface for NodeClient
func (c *NodeClient) FetchJAMDASegments(workPackageHash common.Hash, indexStart uint16, indexEnd uint16, payloadLength uint32) (payload []byte, err error) {
	return nil, fmt.Errorf("FetchJAMDASegments not supported for remote NodeClient connections yet")
}

func (c *NodeClient) BuildBundle(workPackage types.WorkPackage, extrinsicsBlobs []types.ExtrinsicsBlobs, coreIndex uint16, rawObjectIDs []common.Hash) (b *types.WorkPackageBundle, wr *types.WorkReport, err error) {
	return nil, nil, fmt.Errorf("BuildBundle not supported for remote NodeClient connections yet")
}

// GetSegmentWithProof retrieves a segment and its CDT justification proof via RPC
func (c *NodeClient) GetSegmentWithProof(segmentsRoot common.Hash, segmentIndex uint16) (segment []byte, importProof []common.Hash, found bool) {
	req := []string{
		segmentsRoot.Hex(),
		fmt.Sprintf("%d", segmentIndex),
	}

	var res string
	err := c.CallWithRetry("jam.GetSegmentWithProof", req, &res)
	if err != nil {
		log.Warn(log.Node, "GetSegmentWithProof RPC failed", "segmentsRoot", segmentsRoot, "segmentIndex", segmentIndex, "err", err)
		return nil, nil, false
	}

	type segmentWithProofResponse struct {
		Segment string   `json:"segment"`
		Proof   []string `json:"proof"`
	}

	var response segmentWithProofResponse
	if err := json.Unmarshal([]byte(res), &response); err != nil {
		log.Warn(log.Node, "GetSegmentWithProof: failed to unmarshal response", "err", err)
		return nil, nil, false
	}

	segment = common.Hex2Bytes(response.Segment)
	importProof = make([]common.Hash, len(response.Proof))
	for i, h := range response.Proof {
		importProof[i] = common.HexToHash(h)
	}

	return segment, importProof, true
}
