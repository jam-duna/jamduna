package node

import (
	"bytes"
	"context"
	"encoding/binary"
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
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
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
	ServiceValue   map[common.Hash][]byte
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
		log.Error(module, "NewNodeClient", "endpoint", server, "err", err)
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
		ServiceValue:   make(map[common.Hash][]byte),
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
			log.Warn(module, "listenWebSocket read error", "err", err)
			c.reconnectWebSocket()
			time.Sleep(reconnectDelay)
			continue
		}

		var envelope envelope
		if err := json.Unmarshal(msg, &envelope); err != nil {
			log.Warn(module, "listenWebSocket: Failed to parse WebSocket envelope", "err", err)
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
		log.Error(module, "reconnectWebSocket: Failed to reconnect", "err", err)
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
			log.Warn(module, "listenWebSocket: Failed to parse BlockAnnouncement", "err", err)
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
			log.Warn(module, "listenWebSocket: Failed to parse BlockAnnouncement", "err", err)
			return err
		}
		// fmt.Printf("Finalized block: %s\n", result.BlockHash)

	case SubStatistics:
		var payload struct {
			HeaderHash string                    `json:"headerHash"`
			Statistics types.ValidatorStatistics `json:"statistics"`
		}
		if err := json.Unmarshal(envelope.Result, &payload); err != nil {
			log.Warn(module, "listenWebSocket: Failed to parse StatisticsUpdate", "err", err)
			return err
		}
		c.Statistics = payload.Statistics

	case SubServiceInfo:
		var payload struct {
			ServiceID uint32               `json:"serviceID"`
			Info      types.ServiceAccount `json:"info"`
		}
		if err := json.Unmarshal(envelope.Result, &payload); err != nil {
			log.Warn(module, "listenWebSocket: Failed to parse ServiceInfoUpdate", "err", err)
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
			log.Warn(module, "listenWebSocket: Failed to parse SubServiceValue", "err", err)
			return err
		}
		c.ServiceValue[payload.Hash] = common.Hex2Bytes(payload.Value)

	case SubServicePreimage:
		var payload struct {
			ServiceID uint32      `json:"serviceID"`
			Hash      common.Hash `json:"hash"`
			Preimage  string      `json:"preimage"`
		}
		if err := json.Unmarshal(envelope.Result, &payload); err != nil {
			log.Warn(module, "listenWebSocket: Failed to parse ServiceInfoUpdate", "err", err)
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
			log.Warn(module, "listenWebSocket: Failed to parse SubServiceRequest", "err", err)
			return err
		}
		c.ServiceRequest[payload.Hash] = payload.Timeslots

	case SubWorkPackage:
		var payload struct {
			WorkPackageHash common.Hash `json:"workPackageHash"`
			Status          string      `json:"status"`
		}
		if err := json.Unmarshal(envelope.Result, &payload); err != nil {
			log.Warn(module, "listenWebSocket: Failed to parse SubWorkPackage", "err", err)
			return err
		}
		c.WorkPackage[payload.WorkPackageHash] = payload.Status

	default:
		log.Warn(module, "listenWebSocket: Failed to parse method", "method", envelope.Method)
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
		log.Error(module, "GetClient: GetCoreCoWorkersPeers", "err", err)
		return c.baseClient
	}

	// Select random peer
	selected := idx[rand.Intn(len(idx))]
	if selected == 5 {
		selected = idx[0]
	}
	client, err := rpc.Dial("tcp", c.server)
	if err != nil {
		log.Error(module, "GetClient: Dial", "selected", selected, "c.server", c.server, "err", err)
		return c.baseClient
	}
	c.client = client
	return client
}

func (c *NodeClient) CallWithRetry(method string, args interface{}, reply interface{}, possibleCores ...uint16) error {
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
			log.Warn(module, "BaseClient broken, reconnecting")
			c.baseClient.Close()
			c.baseClient, _ = rpc.Dial("tcp", c.server)
		}

		log.Warn(module, "CallWithRetry", "method", method, "attempt", attempt+1, "err", err)
		// Reconnect
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("failed after %d retries: %w", maxRetries, err)
}

func (c *NodeClient) SendCommand(command []string, nodeID int) {
	addr := c.server
	client, err := rpc.Dial("tcp", addr)
	if err != nil {
		log.Error(module, "SendCommand: Dial", "addr", addr, "err", err)
		return
	}
	defer client.Close()

	var response string
	err = client.Call("jam.NodeCommand", command, &response)
	if err != nil {
		log.Error(module, "SendCommand: jam.NodeCommand", "addr", addr, "err", err)
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
		log.Error(module, "BroadcastCommand", "addr", c.server, "err", err)
		return
	}
	defer client.Close()

	var response string
	err = client.Call("jam.NodeCommand", command, &response)
	if err != nil {
		log.Error(module, "BroadcastCommand", "addr", c.server, "err", err)
		return
	}
	log.Info(module, "BroadcastCommand: Response", "addr", c.server, "response", response)
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
func (c *NodeClient) SubmitWorkPackage(workPackageReq *WorkPackageRequest) error {
	// Marshal the WorkPackageRequest to JSON
	reqBytes, err := json.Marshal(workPackageReq)
	if err != nil {
		log.Warn(module, "SubmitWorkPackage", "err", err)
		return fmt.Errorf("failed to marshal work package request: %w", err)
	}

	// Prepare the request as a one-element string slice
	req := []string{string(reqBytes)}

	var res string
	// Call the remote RPC method
	err = c.GetClient().Call("jam.SubmitWorkPackage", req, &res)
	if err != nil {
		log.Warn(module, "SubmitWorkPackage: jam.SubmitWorkPackage", "err", err)
		return err
	}
	return nil
}

func (c *NodeClient) SubmitAndWaitForWorkPackage(ctx context.Context, workPackageReq *WorkPackageRequest) (workPackageHash common.Hash, err error) {
	wphs, err := c.SubmitAndWaitForWorkPackages(ctx, []*WorkPackageRequest{workPackageReq})
	if err != nil {
		return workPackageHash, err
	}
	return wphs[0], nil
}

func (c *NodeClient) SubmitAndWaitForWorkPackages(ctx context.Context, reqs []*WorkPackageRequest) ([]common.Hash, error) {
	log.Info(module, "NodeClient SubmitAndWaitForWorkPackages", "reqLen", len(reqs))
	workPackageHashes := make([]common.Hash, len(reqs))
	workPackageLastStatus := make(map[common.Hash]string)

	identifierToIndex := make(map[string]int)
	// Initialize refine context and identifier map
	refineCtx, err := c.GetRefineContext()
	if err != nil {
		return workPackageHashes, err
	}
	for i, req := range reqs {
		identifierToIndex[req.Identifier] = i
		rc := refineCtx.Clone()
		req.WorkPackage.RefineContext = *rc
	}

	// Populate prerequisite hashes
	for _, req := range reqs {
		if len(req.Prerequisites) == 0 {
			continue
		}
		prereqHashes := make([]common.Hash, 0, len(req.Prerequisites))
		for _, prereqID := range req.Prerequisites {
			if idx, ok := identifierToIndex[prereqID]; ok {
				prereqHashes = append(prereqHashes, reqs[idx].WorkPackage.Hash())
			} else {
				log.Warn(module, "Unknown prerequisite identifier", "identifier", prereqID)
			}
		}
		req.WorkPackage.RefineContext.Prerequisites = prereqHashes
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
			log.Warn(module, "Failed to submit work package", "identifier", req.Identifier, "err", err)
			return workPackageHashes, err
		}
		workPackageLastStatus[hash] = "submitted"
		log.Info(module, "Subscribe", "id", req.Identifier, "h", hash, "prereqids", req.Prerequisites, "h", req.WorkPackage.RefineContext.Prerequisites)
	}

	// Wait for accumulation
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	states := time.NewTicker(types.SecondsPerSlot * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return workPackageHashes, ctx.Err()
		case <-states.C:
			state, err := c.GetState("latest")
			if err == nil {
				numacc := 0
				for _, workPackageHash := range workPackageHashes {
					for _, ah := range state.AccumulationHistory {
						for _, h := range ah.WorkPackageHash {
							if workPackageHash == h {
								log.Info(module, "Work package accumulated", "hash", workPackageHash.Hex())
								numacc++
							}
						}
					}
				}
				if numacc == len(workPackageHashes) {
					log.Info(module, "All work packages accumulated")
					return workPackageHashes, nil
				}
			}
		case <-ticker.C:
			numacc := 0
			for _, workPackageHash := range workPackageHashes {
				if status, ok := c.WorkPackage[workPackageHash]; ok {
					if status == "accumulated" {
						log.Info(module, "Work package accumulated", "hash", workPackageHash.Hex())
						numacc++
					} else if status == "guaranteed" {
						if workPackageLastStatus[workPackageHash] == "submitted" {
							workPackageLastStatus[workPackageHash] = "guaranteed"
							log.Info(module, "Work package guaranteed", "hash", workPackageHash.Hex())
						}
					} else {
						log.Info(module, fmt.Sprintf("Work package status:%s", status), "hash", workPackageHash.Hex())
					}
				}
			}
			if numacc == len(workPackageHashes) {
				log.Info(module, "All work packages accumulated")
				return workPackageHashes, nil
			}

		}
	}

}

func (c *NodeClient) RobustSubmitWorkPackage(workpackage_req *WorkPackageRequest, maxTries int) (workPackageHash common.Hash, err error) {
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
		_, err = c.SubmitAndWaitForWorkPackage(ctx, workpackage_req)
		if err != nil {
			log.Error(module, "SendWorkPackageSubmission", "err", err)
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

func (c *NodeClient) GetAvailabilityAssignments(coreIdx uint32) (*statedb.Rho_state, error) {
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
	var rho_state statedb.Rho_state
	err = json.Unmarshal([]byte(res), &rho_state)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal availability assignment: %w", err)
	}
	return &rho_state, nil
}

func (c *NodeClient) GetSegments(importedSegments []types.ImportSegment) (raw_segments [][]byte, err error) {
	raw_segments = make([][]byte, len(importedSegments))
	for idx, segment := range importedSegments {
		segmentBytes, err := c.Segment(segment.RequestedHash, segment.Index)
		if err != nil {
			return nil, err
		}
		raw_segments[idx] = segmentBytes
	}
	return raw_segments, nil
}

func (c *NodeClient) GetSegmentsByRequestedHash(requestedHash common.Hash) (raw_segments [][]byte, ExportedSegmentLength uint16, err error) {
	return nil, 0, nil
}

func (c *NodeClient) Segment(wphash common.Hash, segmentIndex uint16) ([]byte, error) {
	// Convert the segment index to a string
	segmentIndexStr := strconv.FormatUint(uint64(segmentIndex), 10)
	req := []string{wphash.Hex(), segmentIndexStr}

	var res string
	err := c.CallWithRetry("jam.Segment", req, &res)
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

func (c *NodeClient) NewService(refineContext types.RefineContext, serviceName string, serviceCode []byte, serviceIDs []uint32) (newServiceIdx uint32, err error) {
	serviceCodeHash := common.Blake2Hash(serviceCode)
	fmt.Printf("**** NewService: serviceCodeHash: %s\n", serviceCodeHash.Hex())
	bootstrapCode, err := types.ReadCodeWithMetadata(statedb.BootstrapServiceFile, "bootstrap")
	if err != nil {
		log.Error(module, "NewService: ReadCodeWithMetadata", "err", err)
		return
	}
	bootstrapCodeHash := common.Blake2Hash(bootstrapCode)
	log.Info(module, "NewService: ReadCodeWithMetadata")
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

	var wpr WorkPackageRequest
	wpr.WorkPackage = codeWP
	wpr.ExtrinsicsBlobs = types.ExtrinsicsBlobs{}
	wpHash := codeWP.Hash()
	log.Info(module, "NewService: Submitting WP", "wph", wpHash)

	// submits wp
	ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout)
	defer cancel()
	_, err = c.SubmitAndWaitForWorkPackage(ctx, &wpr)
	if err != nil {
		log.Error(module, "SubmitWorkPackage", "err", err)
		return
	}

	storageKey := common.ServiceStorageKey(0, []byte{0, 0, 0, 0})
	value, _, err := c.GetServiceStorage(0, storageKey)
	if err != nil {
		log.Error(module, "GetServiceStorage", "err", err)
		return
	}
	newServiceIdx = uint32(types.DecodeE_l(value))

	// submits preimage of service code
	ctx, cancel = context.WithTimeout(context.Background(), RefineTimeout)
	defer cancel()
	if err = c.SubmitAndWaitForPreimage(ctx, newServiceIdx, serviceCode); err != nil {
		log.Error(module, "SubmitAndWaitForPreimage", "err", err)
		return
	}

	log.Info(module, "----- NewService", "name", serviceName, "serviceID", newServiceIdx)
	return
}

func (c *NodeClient) LoadServices(services []string) (new_service_map map[string]types.ServiceInfo, err error) {
	new_service_map = make(map[string]types.ServiceInfo)
	serviceIDs := make([]uint32, 0)
	for _, service_name := range services {
		refineContext, err := c.GetRefineContext()
		if err != nil {
			log.Error(module, "LoadServices: GetRefineContext", "service_name", service_name, "err", err)
			return new_service_map, err
		}
		service_path := fmt.Sprintf("/services/%s.pvm", service_name)
		serviceCode, err := types.ReadCodeWithMetadata(service_path, service_name)
		if err != nil {
			log.Error(module, "LoadServices: ReadCodeWithMetadata", "service_name", service_name, "err", err)
			return nil, err
		}
		codeHash := common.Blake2Hash(serviceCode)
		new_serviceIdx, err := c.NewService(refineContext, service_name, serviceCode, serviceIDs)
		if err != nil {
			log.Error(module, "LoadServices: NewService", "service_name", service_name, "err", err)
			return nil, err
		}
		new_service_map[service_name] = types.ServiceInfo{
			ServiceIndex:    new_serviceIdx,
			ServiceCodeHash: codeHash,
		}
		serviceIDs = append(serviceIDs, new_serviceIdx)
	}
	return new_service_map, nil
}

// SERVICE STORAGE
func (c *NodeClient) GetServiceStorage(serviceIndex uint32, storageHash common.Hash) ([]byte, bool, error) {
	req := []string{
		strconv.FormatUint(uint64(serviceIndex), 10),
		storageHash.Hex(),
	}
	var res string
	err := c.CallWithRetry("jam.ServiceValue", req, &res)
	if err != nil {
		return nil, false, err
	}
	storageBytes := common.Hex2Bytes(res)
	return storageBytes, true, nil
}

func (c *NodeClient) WaitForServiceValue(serviceIndex uint32, storageKey common.Hash) (service_index uint32, err error) {
	ctxWait, cancel := context.WithTimeout(context.Background(), RefineTimeout)
	defer cancel()
	c.Subscribe(SubServiceValue, map[string]interface{}{"hash": storageKey.String()})

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctxWait.Done():
			return 0, fmt.Errorf("timed out waiting for service value")
		case <-ticker.C:
			if value, ok := c.ServiceValue[storageKey]; ok {
				service_index = uint32(types.DecodeE_l(value))
				return service_index, nil
			}
		}
	}
}
