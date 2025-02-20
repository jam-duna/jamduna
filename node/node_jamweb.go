package node

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/statedb"
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
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("read error: %v", err)
			}
			break
		}
		log.Printf("Received message from client: %s", message)
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
		log.Println("Upgrade error:", err)
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

func (n *Node) runJamWeb(basePort uint16) {
	basePort += 999
	addr := fmt.Sprintf("0.0.0.0:%v", basePort) // for now just node 0 will handle all

	n.hub = newHub()
	go n.hub.run()

	// WebSocket endpoint at /ws.
	log.Printf("JAM Web Server started. Listening on %s", addr)
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
		decoder := json.NewDecoder(r.Body)
		if err := decoder.Decode(&req); err != nil {
			http.Error(w, "Invalid JSON request", http.StatusBadRequest)
			return
		}

		// Dispatch based on the method.
		var result string
		switch req.Method {
		case "jam_getBlockByHash":
			if len(req.Params) < 1 {
				http.Error(w, "Missing parameter for jam_getBlockByHash", http.StatusBadRequest)
				return
			}
			headerHash := common.HexToHash(req.Params[0])
			sdb, ok := n.getStateDBByHeaderHash(headerHash)
			if ok && sdb.Block != nil {
				result = sdb.Block.String()
			} else {
				http.Error(w, "Block not found", http.StatusNotFound)
				return
			}
			break
		case "jam_getState":
			if len(req.Params) < 1 {
				http.Error(w, "Missing parameter for jam_getState", http.StatusBadRequest)
				return
			}
			headerHash := common.HexToHash(req.Params[0])
			sdb, ok := n.getStateDBByHeaderHash(headerHash)
			if ok {
				result = sdb.JamState.Snapshot(&statedb.StateSnapshotRaw{}).String()
			} else {
				http.Error(w, "State not found", http.StatusNotFound)
				return
			}
			break
		default:
			http.Error(w, "Unsupported method", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Write([]byte(result))
	})

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal("ListenAndServe error: ", err)
	}
}
