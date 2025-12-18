// logviewer serves a web UI for viewing multiple JAM node logs with live tailing
package main

import (
	"bufio"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

//go:embed viewer.html
var viewerHTML embed.FS

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type LogMessage struct {
	Node int    `json:"node"`
	Line string `json:"line"`
}

type Hub struct {
	clients    map[*websocket.Conn]bool
	broadcast  chan LogMessage
	register   chan *websocket.Conn
	unregister chan *websocket.Conn
	mu         sync.RWMutex
}

func newHub() *Hub {
	return &Hub{
		clients:    make(map[*websocket.Conn]bool),
		broadcast:  make(chan LogMessage, 256),
		register:   make(chan *websocket.Conn),
		unregister: make(chan *websocket.Conn),
	}
}

func (h *Hub) run() {
	for {
		select {
		case conn := <-h.register:
			h.mu.Lock()
			h.clients[conn] = true
			h.mu.Unlock()
			log.Printf("Client connected (%d total)", len(h.clients))

		case conn := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[conn]; ok {
				delete(h.clients, conn)
				conn.Close()
			}
			h.mu.Unlock()
			log.Printf("Client disconnected (%d total)", len(h.clients))

		case msg := <-h.broadcast:
			h.mu.RLock()
			data, _ := json.Marshal(msg)
			for conn := range h.clients {
				err := conn.WriteMessage(websocket.TextMessage, data)
				if err != nil {
					conn.Close()
					go func(c *websocket.Conn) { h.unregister <- c }(conn)
				}
			}
			h.mu.RUnlock()
		}
	}
}

func tailFile(path string, nodeID int, hub *Hub, wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		file, err := os.Open(path)
		if err != nil {
			log.Printf("Waiting for %s...", path)
			time.Sleep(2 * time.Second)
			continue
		}

		// Seek to end
		file.Seek(0, io.SeekEnd)

		reader := bufio.NewReader(file)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					time.Sleep(100 * time.Millisecond)
					continue
				}
				log.Printf("Error reading %s: %v", path, err)
				file.Close()
				break
			}

			if len(line) > 1 {
				hub.broadcast <- LogMessage{
					Node: nodeID,
					Line: line[:len(line)-1], // Remove trailing newline
				}
			}
		}
	}
}

func wsHandler(hub *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("WebSocket upgrade failed: %v", err)
			return
		}

		hub.register <- conn

		// Keep connection alive, handle pings
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				hub.unregister <- conn
				break
			}
		}
	}
}

func main() {
	port := flag.Int("port", 8080, "HTTP server port")
	logDir := flag.String("dir", ".", "Directory containing log files")
	pattern := flag.String("pattern", `polkajam-(\d+)\.log`, "Log file pattern (regex with node ID capture group)")
	flag.Parse()

	hub := newHub()
	go hub.run()

	// Find and tail log files
	re := regexp.MustCompile(*pattern)
	files, err := filepath.Glob(filepath.Join(*logDir, "*.log"))
	if err != nil {
		log.Fatalf("Failed to glob log files: %v", err)
	}

	var wg sync.WaitGroup
	for _, file := range files {
		base := filepath.Base(file)
		matches := re.FindStringSubmatch(base)
		if len(matches) < 2 {
			continue
		}

		var nodeID int
		fmt.Sscanf(matches[1], "%d", &nodeID)

		log.Printf("Tailing %s (node %d)", file, nodeID)
		wg.Add(1)
		go tailFile(file, nodeID, hub, &wg)
	}

	if len(files) == 0 {
		log.Printf("No log files found in %s matching pattern %s", *logDir, *pattern)
	}

	// HTTP handlers
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		data, _ := viewerHTML.ReadFile("viewer.html")
		w.Header().Set("Content-Type", "text/html")
		w.Write(data)
	})

	http.HandleFunc("/ws", wsHandler(hub))

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("Log viewer running at http://localhost%s", addr)
	log.Printf("Press Ctrl+C to stop")

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
