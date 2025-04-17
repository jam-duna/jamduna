package node

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/rpc"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"
	"github.com/gorilla/websocket"
)

const (
	SubBestBlock       = "subscribeBestBlock"
	SubFinalizedBlock  = "subscribeFinalizedBlock"
	SubStatistics      = "subscribeStatistics"
	SubServiceInfo     = "subscribeServiceInfo"
	SubServiceValue    = "subscribeServiceValue"
	SubServicePreimage = "subscribeServicePreimage"
	SubServiceRequest  = "subscribeServiceRequest"
	SubWorkPackage     = "subscribeWorkPackage"
	debugWeb           = log.JamwebMonitoring
)

// Struct for incoming WebSocket requests
type SubscriptionRequest struct {
	Method      string                 `json:"method"` //
	Params      map[string]interface{} `json:"params"` // "serviceID", "hash" (storage key or preimage hash), "isFinalized" (true/false)
	hash        common.Hash
	isFinalized bool
}

// Broadcasters for each subscription type
// subscribeBestBlock - Subscribe to updates of the head of the "best" chain, as returned by bestBlock.
// subscribeFinalizedBlock - Subscribe to updates of the latest finalized block, as returned by finalizedBlock.
func (h *Hub) ReceiveLatestBlock(block *types.Block, sdb *statedb.StateDB, isFinalized bool) (err error) {
	var data []byte
	update := sdb.GetStateUpdates()
	for client := range h.clients {
		log.Trace(debugWeb, "ReceiveLatestBlock", "clientsubs", client.String())
		if client.BestBlock != nil {
			req := client.BestBlock
			payload := types.WSPayload{
				Method: req.Method,
				Result: types.SubBlockResult{
					BlockHash:  block.Hash(),
					HeaderHash: block.Header.Hash(),
				},
			}
			data, err = json.Marshal(payload)
			client.sendData(data)
		}
		if client.FinalizedBlock != nil {
			req := client.FinalizedBlock
			payload := types.WSPayload{
				Method: req.Method,
				Result: types.SubBlockResult{
					BlockHash:  block.Hash(),
					HeaderHash: block.Header.Hash(),
				},
			}
			data, err = json.Marshal(payload)
			client.sendData(data)
		}
		if client.Statistics != nil {
			req := client.Statistics
			payload := types.WSPayload{
				Method: req.Method,
				Result: types.SubStatisticsResult{
					HeaderHash: block.Header.Hash(),
					Statistics: sdb.JamState.ValidatorStatistics,
				},
			}
			data, err = json.Marshal(payload)
			client.sendData(data)
		}
		for wph, req := range client.WorkPackages {
			if req != nil {
				res, ok := update.WorkPackageUpdates[wph]
				if !ok || res == nil {
					continue
				}
				payload := types.WSPayload{
					Method: req.Method,
					Result: res,
				}
				data, err = json.Marshal(payload)
				client.sendData(data)
				if res.Status == "accumulated" {
					log.Info(module, "WORKPACKAGE ACCUMULATED", "wph", wph, "data", string(data))
					delete(client.WorkPackages, wph)
				} else {
					log.Trace(module, "WORKPACKAGE UPDATED", "wph", wph, "data", string(data))
				}
			}
		}

		for serviceID, reqs := range client.Services {
			upd := update.ServiceUpdates[serviceID]
			if upd == nil {
				continue
			}
			// TODO: delete
			for _, req := range reqs {
				switch req.Method {
				case SubServiceInfo:
					payload := types.WSPayload{
						Method: SubServiceInfo,
						Result: types.SubServiceInfoResult{
							ServiceID: serviceID,
							Info:      upd.ServiceInfo.Info,
						},
					}
					data, err = json.Marshal(payload)
					client.sendData(data)
				case SubServiceValue:
					if upd.ServiceValue == nil {
						continue
					}
					if req.hash == (common.Hash{}) {
						for k, v := range upd.ServiceValue {
							payload := types.WSPayload{
								Method: SubServiceValue,
								Result: v,
							}
							data, err = json.Marshal(payload)
							client.sendData(data)
							log.Info(module, SubServiceValue, "gen", string(data), "k", k, "v", v)
						}
					} else {
						v, ok := upd.ServiceValue[req.hash]
						if !ok || v == nil {
							continue
						}
						payload := types.WSPayload{
							Method: SubServiceValue,
							Result: v,
						}
						data, err = json.Marshal(payload)
						client.sendData(data)
					}

				case SubServicePreimage:
					if upd.ServicePreimage == nil {
						continue
					}

					res, ok := upd.ServicePreimage[req.hash]
					if !ok || res == nil {
						continue
					}
					payload := types.WSPayload{
						Method: SubServicePreimage,
						Result: res,
					}
					data, err = json.Marshal(payload)
					client.sendData(data)

				case SubServiceRequest:
					if upd.ServiceRequest == nil {
						continue
					}
					res, ok := upd.ServiceRequest[req.hash]
					if !ok || res == nil {
						continue
					}
					payload := types.WSPayload{
						Method: SubServiceRequest,
						Result: res,
					}
					data, err = json.Marshal(payload)
					client.sendData(data)
				}
			}
		}
	}
	return nil
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type Client struct {
	hub  *Hub            `json:"-"` // cannot marshal functions or pointers to complex structs
	conn *websocket.Conn `json:"-"`
	send chan []byte     `json:"-"`

	// subscription requests
	BestBlock      *SubscriptionRequest                 `json:"bestBlock,omitempty"`
	FinalizedBlock *SubscriptionRequest                 `json:"finalizedBlock,omitempty"`
	Statistics     *SubscriptionRequest                 `json:"statistics,omitempty"`
	WorkPackages   map[common.Hash]*SubscriptionRequest `json:"workPackages,omitempty"`
	Services       map[uint32][]*SubscriptionRequest    `json:"services,omitempty"`
}

func (c *Client) String() string {
	type clientView struct {
		BestBlock      *SubscriptionRequest              `json:"bestBlock,omitempty"`
		FinalizedBlock *SubscriptionRequest              `json:"finalizedBlock,omitempty"`
		Statistics     *SubscriptionRequest              `json:"statistics,omitempty"`
		WorkPackages   map[string]*SubscriptionRequest   `json:"workPackages,omitempty"`
		Services       map[uint32][]*SubscriptionRequest `json:"services,omitempty"`
	}

	// Convert WorkPackages keys to string (assuming common.Hash implements String())
	workPackages := make(map[string]*SubscriptionRequest, len(c.WorkPackages))
	for k, v := range c.WorkPackages {
		workPackages[k.String()] = v
	}

	view := clientView{
		BestBlock:      c.BestBlock,
		FinalizedBlock: c.FinalizedBlock,
		Statistics:     c.Statistics,
		WorkPackages:   workPackages,
		Services:       c.Services,
	}

	b, err := json.MarshalIndent(view, "", "  ")
	if err != nil {
		return fmt.Sprintf("Client{error: %v}", err)
	}
	return string(b)
}

// Hub manages client registration and broadcasting
type Hub struct {
	clients    map[*Client]bool
	register   chan *Client
	unregister chan *Client
	broadcast  chan []byte
	ctx        context.Context
	cancel     context.CancelFunc
}

func newHub(ctx context.Context) *Hub {
	cctx, cancel := context.WithCancel(ctx)
	return &Hub{
		clients:    make(map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan []byte),
		ctx:        cctx,
		cancel:     cancel,
	}
}

func (h *Hub) run(wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case <-h.ctx.Done():
			for client := range h.clients {
				close(client.send)
			}
			return

		case client := <-h.register:
			h.clients[client] = true

		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}

		case message := <-h.broadcast:
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
		}
	}
}

// readPump handles WebSocket reads and subscription management
func (c *Client) sendData(data []byte) {
	log.Trace(debugWeb, "sendData", "data", string(data))
	select {
	case c.send <- data:
	default:
	}
}

// readPump handles WebSocket reads and subscription management
func (c *Client) addSubscription(serviceID uint32, req *SubscriptionRequest) {
	log.Trace(debugWeb, "addSubscription", "serviceID", serviceID, "method", req.Method, "h", req.hash)
	if _, ok := c.Services[serviceID]; !ok {
		c.Services[serviceID] = make([]*SubscriptionRequest, 0)
	}
	// make sure its unique
	for _, r := range c.Services[serviceID] {
		if r.hash == req.hash {
			return
		}
	}
	c.Services[serviceID] = append(c.Services[serviceID], req)
}

func fetchHashAttr(req *SubscriptionRequest, key string) common.Hash {
	if val, ok := req.Params[key]; ok {
		if str, ok := val.(string); ok {
			return common.HexToHash(str)
		}
	}
	return common.Hash{}
}

func fetchUint32Attr(req *SubscriptionRequest, key string) (uint32, bool) {
	val, ok := req.Params[key]
	if !ok {
		return 0, false
	}

	switch v := val.(type) {
	case int:
		return uint32(v), true
	case float64:
		return uint32(v), true
	case string:
		n, err := strconv.ParseUint(v, 10, 32)
		if err != nil {
			return 0, false
		}
		return uint32(n), true
	default:
		return 0, false
	}
}

func fetchBoolAttr(req *SubscriptionRequest, key string) (bool, bool) {
	val, ok := req.Params[key]
	if !ok {
		return false, false
	}

	switch v := val.(type) {
	case bool:
		return v, true
	case string:
		return v == "true", true
	default:
		log.Warn(debugWeb, "unsupported bool attr type", key, fmt.Sprintf("%T", val))
		return false, false
	}
}

// readPump handles WebSocket reads and subscription management
func (c *Client) readPump(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(512)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		select {
		case <-ctx.Done():
			return
		default:
			_, message, err := c.conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err) {
					log.Trace(debugWeb, "WebSocket close error", err)
				}
				return
			}
			log.Info(module, "Received message from client", "msg", string(message))

			var req SubscriptionRequest
			if err := json.Unmarshal(message, &req); err != nil {
				log.Warn(module, "Invalid subscription message", err)
				continue
			}
			req.isFinalized = false
			serviceID := uint32(0)
			if req.Method == SubServiceInfo || req.Method == SubServiceValue || req.Method == SubServicePreimage || req.Method == SubServiceRequest {
				req.isFinalized, _ = fetchBoolAttr(&req, "isFinalized")
				serviceID, _ = fetchUint32Attr(&req, "serviceID")
			}

			switch req.Method {
			case SubServiceInfo:
				// Handle subscription to service info
				c.addSubscription(serviceID, &req)
				break
			case SubServiceValue:
				// Handle subscription to service value
				req.hash = fetchHashAttr(&req, "hash")
				log.Info(module, "fetchHashAttr", "method", req.Method, "h", req.hash)
				c.addSubscription(serviceID, &req)
				break
			case SubServicePreimage:
				// Handle subscription to service preimage
				req.hash = fetchHashAttr(&req, "hash")
				c.addSubscription(serviceID, &req)
				break
			case SubServiceRequest:
				// Handle subscription to service request
				req.hash = fetchHashAttr(&req, "hash")
				c.addSubscription(serviceID, &req)
				break
			case SubBestBlock:
				// Handle subscription to best block
				req.isFinalized = false
				c.BestBlock = &req
			case SubFinalizedBlock:
				// Handle subscription to finalized block
				req.isFinalized = true
				c.FinalizedBlock = &req
			case SubStatistics:
				// Handle subscription to statistics
				c.Statistics = &req
			case SubWorkPackage:
				// Handle subscription to work package
				req.hash = common.HexToHash(req.Params["hash"].(string))
				c.WorkPackages[req.hash] = &req
			default:
				log.Warn(debugWeb, "Unknown subscription method", req.Method)
			}
			log.Trace(debugWeb, "Subscribed", "method", req.Method, "h", req.hash)
		}
	}
}

func (c *Client) writePump(ctx context.Context, wg *sync.WaitGroup) {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
		wg.Done()
	}()

	for {
		select {
		case <-ctx.Done():
			return

		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)
			w.Close()
			for len(c.send) > 0 {
				next := <-c.send
				c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				w, err := c.conn.NextWriter(websocket.TextMessage)
				if err != nil {
					return
				}
				w.Write(next)
				w.Close()
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func serveWs(hub *Hub, w http.ResponseWriter, r *http.Request, wg *sync.WaitGroup) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error(debugWeb, "serveWs Upgrade error", err)
		return
	}
	client := &Client{
		hub:            hub,
		conn:           conn,
		send:           make(chan []byte, 256),
		BestBlock:      nil,
		FinalizedBlock: nil,
		WorkPackages:   make(map[common.Hash]*SubscriptionRequest),
		Services:       make(map[uint32][]*SubscriptionRequest),
	}
	hub.register <- client

	wg.Add(2)
	go client.writePump(hub.ctx, wg)
	go client.readPump(hub.ctx, wg)
}

func (n *NodeContent) RunApplyBlockAndWeb(ctx context.Context, blockDataDir string, port uint16, storage *storage.StateDBStorage) {
	files, err := os.ReadDir(blockDataDir)
	if err != nil {
		log.Crit(debugWeb, "ReadDir error", err)
		return
	}

	for _, file := range files {
		if file.IsDir() || filepath.Ext(file.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(blockDataDir, file.Name()))
		if err != nil {
			log.Crit(debugWeb, "ReadFile error", err)
			continue
		}
		var stf statedb.StateTransition
		if err := json.Unmarshal(data, &stf); err != nil {
			log.Crit(debugWeb, "Unmarshal error", err)
			continue
		}
		newStateDB, err := statedb.NewStateDBFromSnapshotRaw(storage, &stf.PostState)
		if err != nil {
			log.Crit(debugWeb, "StateDB init error", err)
			continue
		}
		newStateDB.Block = &stf.Block
		if err := n.StoreBlock(&stf.Block, 9999, false); err != nil {
			log.Crit(debugWeb, "StoreBlock error", err)
			continue
		}
		n.addStateDB(newStateDB)
	}

	wg := &sync.WaitGroup{}
	go n.runJamWeb(ctx, wg, port, 8080)

	<-ctx.Done()
	wg.Wait()
	log.Info(debugWeb, "Graceful shutdown complete")
}

func (n *NodeContent) runJamWeb(ctx context.Context, wg *sync.WaitGroup, basePort uint16, port int) {
	addr := fmt.Sprintf("0.0.0.0:%v", basePort)
	n.hub = newHub(ctx)

	wg.Add(1)
	go n.hub.run(wg)

	server := &http.Server{
		Addr: addr,
	}

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(n.hub, w, r, wg)
	})

	http.HandleFunc("/rpc", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept")
			w.WriteHeader(http.StatusNoContent)
			return
		} else if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			JSONRPC string   `json:"jsonrpc"`
			Method  string   `json:"method"`
			Params  []string `json:"params"`
			ID      int      `json:"id"`
		}
		rpcAddr := fmt.Sprintf("localhost:%d", port+1300)
		client, err := rpc.Dial("tcp", rpcAddr)
		if err != nil {
			http.Error(w, "RPC dial error", http.StatusInternalServerError)
			return
		}
		defer client.Close()

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		var result string
		if err := client.Call(req.Method, req.Params, &result); err != nil {
			http.Error(w, "RPC error: "+err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Write([]byte(result))
	})

	log.Info(debugWeb, "JAM Web Server started", "addr", addr)

	// Run server in a goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Crit(debugWeb, "ListenAndServe error", err)
		}
	}()

	// Wait for context to be canceled and shut down the server
	<-ctx.Done()
	ctxShutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server.Shutdown(ctxShutdown)
	n.hub.cancel()
}
