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
func (h *Hub) ReceiveLatestBlock(block *types.Block, sdb *statedb.StateDB, isFinalized bool, subscriptions map[uint32]*types.ServiceSubscription) {
	for serviceID, upd := range subscriptions {
		for client := range h.clients {
			reqs, ok := client.subscriptions[serviceID]
			if !ok {
				continue
			}
			for _, req := range reqs {
				if isFinalized != req.isFinalized {
					continue
				}

				var data []byte
				var err error

				switch req.Method {
				case SubBestBlock, SubFinalizedBlock:
					payload := struct {
						Method string `json:"method"`
						Result struct {
							BlockHash  common.Hash `json:"blockHash"`
							HeaderHash common.Hash `json:"headerHash"`
						} `json:"result"`
					}{
						Method: req.Method,
						Result: struct {
							BlockHash  common.Hash `json:"blockHash"`
							HeaderHash common.Hash `json:"headerHash"`
						}{
							BlockHash:  block.Hash(),
							HeaderHash: block.Header.Hash(),
						},
					}
					data, err = json.Marshal(payload)

				case SubStatistics:
					payload := struct {
						Method string `json:"method"`
						Result struct {
							HeaderHash common.Hash               `json:"headerHash"`
							Statistics types.ValidatorStatistics `json:"statistics"`
						} `json:"result"`
					}{
						Method: SubStatistics,
						Result: struct {
							HeaderHash common.Hash               `json:"headerHash"`
							Statistics types.ValidatorStatistics `json:"statistics"`
						}{
							HeaderHash: block.Header.Hash(),
							Statistics: sdb.JamState.ValidatorStatistics,
						},
					}
					data, err = json.Marshal(payload)

				case SubServiceInfo:
					payload := struct {
						Method string `json:"method"`
						Result struct {
							ServiceID uint32               `json:"service_id"`
							Info      types.ServiceAccount `json:"info"`
						} `json:"result"`
					}{
						Method: SubServiceInfo,
						Result: struct {
							ServiceID uint32               `json:"service_id"`
							Info      types.ServiceAccount `json:"info"`
						}{
							ServiceID: serviceID,
							Info:      *upd.ServiceInfo,
						},
					}
					data, err = json.Marshal(payload)

				case SubServiceValue:
					v, ok := upd.ServiceValue[req.hash]
					if !ok {
						continue
					}
					payload := struct {
						Method string `json:"method"`
						Result struct {
							ServiceID uint32 `json:"service_id"`
							Value     string `json:"value"`
						} `json:"result"`
					}{
						Method: SubServiceValue,
						Result: struct {
							ServiceID uint32 `json:"service_id"`
							Value     string `json:"value"`
						}{
							ServiceID: serviceID,
							Value:     common.Bytes2Hex(v),
						},
					}
					data, err = json.Marshal(payload)

				case SubServicePreimage:
					preimage, ok := upd.ServicePreimage[req.hash]
					if !ok {
						continue
					}
					payload := struct {
						Method string `json:"method"`
						Result struct {
							ServiceID uint32 `json:"service_id"`
							Preimage  string `json:"preimage"`
						} `json:"result"`
					}{
						Method: SubServicePreimage,
						Result: struct {
							ServiceID uint32 `json:"service_id"`
							Preimage  string `json:"preimage"`
						}{
							ServiceID: serviceID,
							Preimage:  common.Bytes2Hex(preimage),
						},
					}
					data, err = json.Marshal(payload)

				case SubServiceRequest:
					timeslots, ok := upd.ServiceRequest[req.hash]
					if !ok {
						continue
					}
					payload := struct {
						Method string `json:"method"`
						Result struct {
							ServiceID uint32   `json:"service_id"`
							Timeslots []uint32 `json:"timeslots"`
						} `json:"result"`
					}{
						Method: SubServiceRequest,
						Result: struct {
							ServiceID uint32   `json:"service_id"`
							Timeslots []uint32 `json:"timeslots"`
						}{
							ServiceID: serviceID,
							Timeslots: timeslots,
						},
					}
					data, err = json.Marshal(payload)

				case SubWorkPackage:
					status, ok := upd.WorkPackage[req.hash]
					if !ok {
						continue
					}
					payload := struct {
						Method string `json:"method"`
						Result struct {
							WorkPackageHash common.Hash `json:"work_package_hash"`
							Status          string      `json:"status"`
						} `json:"result"`
					}{
						Method: SubWorkPackage,
						Result: struct {
							WorkPackageHash common.Hash `json:"work_package_hash"`
							Status          string      `json:"status"`
						}{
							WorkPackageHash: req.hash,
							Status:          status,
						},
					}
					data, err = json.Marshal(payload)
				}

				if err != nil {
					fmt.Printf("JSON marshal error for %s: %v\n", req.Method, err)
					continue
				}

				client.sendData(data)
			}
		}
	}
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// Hub manages client registration and broadcasting
type Hub struct {
	clients    map[*Client]map[uint32]*types.ServiceSubscription
	register   chan *Client
	unregister chan *Client
	broadcast  chan []byte
	ctx        context.Context
	cancel     context.CancelFunc
}

func newHub(ctx context.Context) *Hub {
	cctx, cancel := context.WithCancel(ctx)
	return &Hub{
		clients:    make(map[*Client]map[uint32]*types.ServiceSubscription),
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
			for client, _ := range h.clients {
				close(client.send)
			}
			return

		case client := <-h.register:
			h.clients[client] = make(map[uint32]*types.ServiceSubscription)

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

type Client struct {
	hub           *Hub
	conn          *websocket.Conn
	send          chan []byte
	subscriptions map[uint32][]*SubscriptionRequest
}

// readPump handles WebSocket reads and subscription management
func (c *Client) sendData(data []byte) {
	select {
	case c.send <- data:
	default:
	}
}

// readPump handles WebSocket reads and subscription management
func (c *Client) addSubscription(serviceID uint32, req *SubscriptionRequest) {
	log.Info(module, "addSubscription", "serviceID", serviceID, "method", req.Method)
	if c.subscriptions == nil {
		c.subscriptions = make(map[uint32][]*SubscriptionRequest)
	}
	if _, ok := c.subscriptions[serviceID]; !ok {
		c.subscriptions[serviceID] = make([]*SubscriptionRequest, 0)
	}
	c.subscriptions[serviceID] = append(c.subscriptions[serviceID], req)
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
			log.Info(debugWeb, "Received message from client", message)

			var req SubscriptionRequest
			if err := json.Unmarshal(message, &req); err != nil {
				log.Warn(debugWeb, "Invalid subscription message", err)
				continue
			}
			req.isFinalized = false
			serviceID := uint32(0)
			if req.Method == SubServiceInfo || req.Method == SubServiceValue || req.Method == SubServicePreimage || req.Method == SubServiceRequest {
				// Handle isFinalized (string vs bool)
				switch v := req.Params["isFinalized"].(type) {
				case bool:
					req.isFinalized = v
				case string:
					req.isFinalized = (v == "true")
				default:
					req.isFinalized = false // or leave as-is
				}

				// Handle serviceID (string vs int or float64??)
				switch v := req.Params["serviceID"].(type) {
				case int:
					serviceID = uint32(v)
				case float64:
					serviceID = uint32(v)
				case string:
					val, err := strconv.ParseUint(v, 10, 32)
					if err != nil {
						log.Warn(debugWeb, "invalid serviceID", v)
						return
					}
					serviceID = uint32(val)
				default:
					log.Warn(debugWeb, "unexpected serviceID type", fmt.Sprintf("%T", v))
					return
				}
			}

			switch req.Method {
			case SubServiceInfo:
				// Handle subscription to service info
				c.addSubscription(serviceID, &req)
				break
			case SubServiceValue:
				// Handle subscription to service value
				req.hash = common.HexToHash(req.Params["hash"].(string))
				c.addSubscription(serviceID, &req)
				break
			case SubServicePreimage:
				// Handle subscription to service preimage
				req.hash = common.HexToHash(req.Params["hash"].(string))
				c.addSubscription(serviceID, &req)
				break
			case SubServiceRequest:
				// Handle subscription to service request
				req.hash = common.HexToHash(req.Params["hash"].(string))
				c.addSubscription(serviceID, &req)
				break
			case SubBestBlock:
				// Handle subscription to best block
				req.isFinalized = false
				c.addSubscription(0, &req)
			case SubFinalizedBlock:
				// Handle subscription to finalized block
				req.isFinalized = true
				c.addSubscription(0, &req)
			case SubStatistics:
				// Handle subscription to statistics
				c.addSubscription(0, &req)
			case SubWorkPackage:
				// Handle subscription to work package
				req.hash = common.HexToHash(req.Params["hash"].(string))
				c.addSubscription(0, &req)
			default:
				log.Warn(debugWeb, "Unknown subscription method", req.Method)
			}
			log.Info(debugWeb, "Subscribed", "method", req.Method)
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
			for len(c.send) > 0 {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
			}
			w.Close()

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
	client := &Client{hub: hub, conn: conn, send: make(chan []byte, 256)}
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
