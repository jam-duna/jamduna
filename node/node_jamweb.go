package node

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/rpc"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/storage"
	"github.com/gorilla/websocket"
)

const debugWeb = log.JamwebMonitoring

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
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

type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
}

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
