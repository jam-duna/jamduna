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

//go:embed viewer.html peerIdentification.json
var viewerHTML embed.FS

// Version info - set at build time via ldflags
var (
	Version   = "dev"
	BuildTime = "unknown"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type LogMessage struct {
	Type string `json:"type"` // "log" or "status"
	Node int    `json:"node"`
	Line string `json:"line"`
}

type NodeStatus struct {
	lastActivity time.Time
	active       bool
}

var (
	nodeStatus   = make(map[int]*NodeStatus)
	nodeStatusMu sync.RWMutex
)

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

	// Initialize node status
	nodeStatusMu.Lock()
	nodeStatus[nodeID] = &NodeStatus{lastActivity: time.Now(), active: true}
	nodeStatusMu.Unlock()

	initialLoad := true // Flag to track if we've done the initial load

	for {
		file, err := os.Open(path)
		if err != nil {
			log.Printf("Waiting for %s...", path)
			time.Sleep(2 * time.Second)
			continue
		}

		// On first open, read from beginning to load existing content
		// On subsequent opens (after file rotation/truncation), also read from beginning
		if initialLoad {
			log.Printf("Loading existing content from %s...", path)
			// Read from beginning (don't seek to end)
			initialLoad = false
		}
		// Note: We always read from current position (beginning for new file)

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
				// Update node activity
				nodeStatusMu.Lock()
				nodeStatus[nodeID].lastActivity = time.Now()
				nodeStatus[nodeID].active = true
				nodeStatusMu.Unlock()

				hub.broadcast <- LogMessage{
					Type: "log",
					Node: nodeID,
					Line: line[:len(line)-1], // Remove trailing newline
				}
			}
		}
	}
}

// monitorNodeStatus checks for inactive nodes and broadcasts status updates
func monitorNodeStatus(hub *Hub) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	inactiveThreshold := 5 * time.Second

	for range ticker.C {
		nodeStatusMu.Lock()
		for nodeID, status := range nodeStatus {
			wasActive := status.active
			status.active = time.Since(status.lastActivity) < inactiveThreshold

			if wasActive && !status.active {
				log.Printf("Node %d: no logs for %v (inactive)", nodeID, inactiveThreshold)
				hub.broadcast <- LogMessage{
					Type: "status",
					Node: nodeID,
					Line: "inactive",
				}
			} else if !wasActive && status.active {
				log.Printf("Node %d: logs resumed (active)", nodeID)
				hub.broadcast <- LogMessage{
					Type: "status",
					Node: nodeID,
					Line: "active",
				}
			}
		}
		nodeStatusMu.Unlock()
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

	// Start node status monitor
	go monitorNodeStatus(hub)

	// HTTP handlers
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		data, _ := viewerHTML.ReadFile("viewer.html")
		w.Header().Set("Content-Type", "text/html")
		w.Write(data)
	})

	http.HandleFunc("/ws", wsHandler(hub))

	http.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"version":   Version,
			"buildTime": BuildTime,
		})
	})

	http.HandleFunc("/peers", func(w http.ResponseWriter, r *http.Request) {
		data, err := viewerHTML.ReadFile("peerIdentification.json")
		if err != nil {
			http.Error(w, "Peer config not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	})

	// Filter settings endpoints - load/save from cmd/logviewer/filter_list.json
	filterFilePath := "filter_list.json" // Relative to where logviewer is run from

	http.HandleFunc("/filters", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			// Try to read filter_list.json from disk
			data, err := os.ReadFile(filterFilePath)
			if err != nil {
				if os.IsNotExist(err) {
					// No filter file exists - return empty object
					w.Header().Set("Content-Type", "application/json")
					w.Write([]byte("{}"))
					return
				}
				http.Error(w, "Failed to read filter file", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write(data)

		case "POST":
			// Save filter settings to disk
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Failed to read request body", http.StatusBadRequest)
				return
			}
			defer r.Body.Close()

			// Validate JSON
			var jsonData interface{}
			if err := json.Unmarshal(body, &jsonData); err != nil {
				http.Error(w, "Invalid JSON", http.StatusBadRequest)
				return
			}

			// Pretty-print the JSON before saving
			prettyJSON, err := json.MarshalIndent(jsonData, "", "  ")
			if err != nil {
				http.Error(w, "Failed to format JSON", http.StatusInternalServerError)
				return
			}

			if err := os.WriteFile(filterFilePath, prettyJSON, 0644); err != nil {
				http.Error(w, "Failed to save filter file", http.StatusInternalServerError)
				return
			}

			log.Printf("Filter settings saved to %s", filterFilePath)
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"status":"saved"}`))

		case "DELETE":
			// Delete filter file
			if err := os.Remove(filterFilePath); err != nil && !os.IsNotExist(err) {
				http.Error(w, "Failed to delete filter file", http.StatusInternalServerError)
				return
			}
			log.Printf("Filter settings deleted")
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"status":"deleted"}`))

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Reload endpoint - reads log files and streams them (with optional limit)
	http.HandleFunc("/reload", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Transfer-Encoding", "chunked")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming not supported", http.StatusInternalServerError)
			return
		}

		// Parse limit from query parameter (0 = unlimited)
		limit := 0
		if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
			fmt.Sscanf(limitStr, "%d", &limit)
		}

		// Find all log files
		logFiles, err := filepath.Glob(filepath.Join(*logDir, "*.log"))
		if err != nil {
			http.Error(w, "Failed to find log files", http.StatusInternalServerError)
			return
		}

		// If limit is set, we need to collect all logs first, then send only the last N
		if limit > 0 {
			var allLogs []LogMessage

			for _, logFile := range logFiles {
				base := filepath.Base(logFile)
				matches := re.FindStringSubmatch(base)
				if len(matches) < 2 {
					continue
				}

				var nodeID int
				fmt.Sscanf(matches[1], "%d", &nodeID)

				file, err := os.Open(logFile)
				if err != nil {
					log.Printf("Reload: failed to open %s: %v", logFile, err)
					continue
				}

				reader := bufio.NewReader(file)
				for {
					line, err := reader.ReadString('\n')
					if err != nil {
						break
					}
					if len(line) > 1 {
						allLogs = append(allLogs, LogMessage{
							Type: "log",
							Node: nodeID,
							Line: line[:len(line)-1],
						})
					}
				}
				file.Close()
			}

			// Send only the last N logs
			start := 0
			if len(allLogs) > limit {
				start = len(allLogs) - limit
				log.Printf("Reload: limiting to last %d logs (total: %d)", limit, len(allLogs))
			}

			encoder := json.NewEncoder(w)
			for i := start; i < len(allLogs); i++ {
				encoder.Encode(allLogs[i])
			}
			flusher.Flush()
			return
		}

		// No limit - stream all logs
		encoder := json.NewEncoder(w)

		for _, logFile := range logFiles {
			base := filepath.Base(logFile)
			matches := re.FindStringSubmatch(base)
			if len(matches) < 2 {
				continue
			}

			var nodeID int
			fmt.Sscanf(matches[1], "%d", &nodeID)

			file, err := os.Open(logFile)
			if err != nil {
				log.Printf("Reload: failed to open %s: %v", logFile, err)
				continue
			}

			reader := bufio.NewReader(file)
			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					break
				}
				if len(line) > 1 {
					msg := LogMessage{
						Type: "log",
						Node: nodeID,
						Line: line[:len(line)-1],
					}
					encoder.Encode(msg)
				}
			}
			file.Close()
			flusher.Flush()
		}
	})

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("Log viewer running at http://localhost%s", addr)
	log.Printf("Press Ctrl+C to stop")

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
