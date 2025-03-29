package node

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"net/http"
	"net/rpc"
	"time"

	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/storage"
	"github.com/gorilla/websocket"
)

// upgrader upgrades HTTP connections to WebSocket connections.
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// In production, adjust the CheckOrigin function as needed.
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// Hub maintains the set of active clients and broadcasts messages to them.
type Hub struct {
	// Registered clients.
	clients map[*Client]bool

	// Inbound messages to be broadcast to all clients.
	broadcast chan []byte

	// Register requests from clients.
	register chan *Client

	// Unregister requests from clients.
	unregister chan *Client
}

// newHub creates a new Hub.
func newHub() *Hub {
	return &Hub{
		broadcast:  make(chan []byte),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
	}
}

// run listens on the hub channels and manages client connections.
func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true

		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}

		case message := <-h.broadcast:
			for client := range h.clients {
				// Nonblocking send; if the client's send channel is full, drop the client.
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

// Client represents a single WebSocket connection.
type Client struct {
	hub  *Hub
	conn *websocket.Conn
	// Buffered channel of outbound messages.
	send chan []byte
}

// readPump pumps messages from the WebSocket connection to the hub.
// (In this example we simply log any messages received.)
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(512)
	// Set a deadline to detect dead connections.
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			// Log unexpected close errors.
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseNormalClosure, websocket.CloseNoStatusReceived) {
				log.Crit("jamweb", "IsUnexpectedCloseError", err)
			}
			break
		}
		log.Debug("jamweb", "Received message from client", message)
		// Optionally process incoming messages here.
	}
}

// writePump pumps messages from the hub to the WebSocket connection.
func (c *Client) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			// Set a deadline for writing.
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				// The hub closed the channel.
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			// Write the message as a text message.
			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Write any queued messages in the same WebSocket message.
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
			}
			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			// Send a ping to maintain the connection.
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// serveWs upgrades the HTTP request to a WebSocket connection and registers the client.
func serveWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
	// Upgrade the HTTP connection.
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error("jamweb", "serveWs Upgrade error", err)
		return
	}
	client := &Client{
		hub:  hub,
		conn: conn,
		send: make(chan []byte, 256),
	}
	hub.register <- client

	// Launch goroutines to handle reading and writing.
	go client.writePump()
	go client.readPump()
}

func (n *NodeContent) RunApplyBlockAndWeb(block_data_dir string, port uint16, storage *storage.StateDBStorage) {
	// read all the block json from the block_data_dir
	files, err := ioutil.ReadDir(block_data_dir)
	if err != nil {
		log.Crit("jamweb", "ReadDir error", err)
		return
	}

	// Filter out non-JSON files
	var jsonFiles []os.FileInfo
	for _, file := range files {
		if filepath.Ext(file.Name()) == ".json" {
			jsonFiles = append(jsonFiles, file)
		}
	}
	files = jsonFiles
	// the files need to be sorted by name
	// so that the blocks are applied in order

	for _, file := range files {
		if file.IsDir() {
			continue
		}
		filePath := filepath.Join(block_data_dir, file.Name())
		data, err := ioutil.ReadFile(filePath)
		if err != nil {
			log.Crit("jamweb", "ReadFile error", err)
			continue
		}
		var stf statedb.StateTransition
		if err := json.Unmarshal(data, &stf); err != nil {
			log.Crit("jamweb", "Unmarshal error", err)
			continue
		}
		err = json.Unmarshal(data, &stf)
		if err != nil {
			log.Crit("jamweb", "Unmarshal error", err)
			continue
		}
		new_statedb, err := statedb.NewStateDBFromSnapshotRaw(storage, &(stf.PostState))
		if err != nil {
			log.Crit("jamweb", "NewStateDBFromSnapshotRaw error", err)
			continue
		}

		block := stf.Block
		new_statedb.Block = &block
		err = n.StoreBlock(&block, 9999, false)
		if err != nil {
			log.Crit("jamweb", "StoreBlock error", err)
			continue
		}
		// Apply the block to the state
		fmt.Printf("applied block parent %v, header %v, timeslot %v\n", block.Header.ParentHeaderHash, block.Header.Hash(), block.Header.Slot)
		n.addStateDB(new_statedb)
	}
	// apply the block to the state
	go n.runJamWeb(port, 8080) // fix this if you want to use this
	for {
		time.Sleep(1 * time.Second)
	}
}

func (n *NodeContent) runJamWeb(basePort uint16, port int) {
	// basePort += 999
	addr := fmt.Sprintf("0.0.0.0:%v", basePort) // for now just node 0 will handle all

	n.hub = newHub()
	go n.hub.run()

	// WebSocket endpoint at /ws.
	log.Info("jamweb", "JAM Web Server started", "addr", addr)
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(n.hub, w, r)
	})

	// HTTP endpoint at "/" to process JSON-RPC requests like these https://docs.jamcha.in/basics/rpc
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

		// Decode the JSON request.
		var req struct {
			JSONRPC string   `json:"jsonrpc"`
			Method  string   `json:"method"`
			Params  []string `json:"params"`
			ID      int      `json:"id"`
		}
		rpc_port := port + 1200 //base port + 1200 -- 13370+1200
		rpc_address := fmt.Sprintf("localhost:%v", rpc_port)
		client, err := rpc.Dial("tcp", rpc_address)
		decoder := json.NewDecoder(r.Body)
		if err := decoder.Decode(&req); err != nil {
			http.Error(w, "Invalid JSON request", http.StatusBadRequest)
			return
		}
		method := req.Method
		args := req.Params
		fmt.Printf("method %v, params %v\n", method, args)

		// Dispatch based on the method.
		var result string
		err = client.Call(method, args, &result)
		if err != nil {
			fmt.Printf("RPC call failed %v\n", err)
			http.Error(w, "RPC call failed:"+err.Error(), http.StatusBadRequest)
			return
		}
		// Encode the JSON response.
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Write([]byte(result))
	})

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Crit("jamweb", "ListenAndServe error", err)
	}
}
