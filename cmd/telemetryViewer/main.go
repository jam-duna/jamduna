package main

import (
	"bufio"
	"embed"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/jam-duna/jamduna/common"
	"github.com/jam-duna/jamduna/telemetry"
	"github.com/jam-duna/jamduna/types"
	"github.com/gorilla/websocket"
)

//go:embed templates/* peerIdentification.json
var templateFS embed.FS

// PeerInfo from peerIdentification.json
type PeerConfig struct {
	Node   int    `json:"node"`
	Name   string `json:"name"`
	Role   string `json:"role"`
	PeerID string `json:"peer_id"` // base32 encoded
}

type PeerIdentification struct {
	Validators []PeerConfig `json:"validators"`
}

// TelemetryEvent represents a parsed telemetry event
type TelemetryEvent struct {
	Type          string `json:"type"`          // "log" for logViewer compat
	Node          int    `json:"node"`          // Validator index (0-5) or -1 if unknown
	Line          string `json:"line"`          // Formatted log line for display
	Timestamp     string `json:"timestamp"`
	EventType     string `json:"eventType"`
	Data          string `json:"data"`
	PeerAddr      string `json:"peerAddr"`
	NodeName      string `json:"nodeName"`
	NodeRole      string `json:"nodeRole"`
	Level         string `json:"level"`
	Discriminator int    `json:"discriminator"`
}

// TelemetryViewer manages the web UI and telemetry connections
type TelemetryViewer struct {
	telemetryAddr string
	webAddr       string
	logFile       string

	// Peer identification mapping
	peerMap map[string]PeerConfig // peer_id (base32) -> PeerConfig

	// WebSocket clients
	clients   map[*websocket.Conn]bool
	clientsMu sync.RWMutex
	broadcast chan TelemetryEvent

	// Event buffer for new clients
	eventBuffer   []TelemetryEvent
	eventBufferMu sync.RWMutex
	maxEvents     int

	// Node tracking
	nodes   map[int]*NodeInfo // nodeId -> NodeInfo
	nodesMu sync.RWMutex

	// Stats
	stats   *Stats
	statsMu sync.RWMutex
}

type NodeInfo struct {
	NodeID       int       `json:"nodeId"`
	NodeName     string    `json:"nodeName"`
	NodeRole     string    `json:"nodeRole"`
	PeerID       string    `json:"peerId"`       // base32 encoded
	PeerIDHex    string    `json:"peerIdHex"`    // hex encoded for display
	PeerIP       string    `json:"peerIp"`       // IP:port from NodeInfo
	Addr         string    `json:"addr"`
	ImplName     string    `json:"implName"`     // polkajam, jamduna, etc
	ImplVersion  string    `json:"implVersion"`
	ConnectedAt  time.Time `json:"connectedAt"`
	LastEventAt  time.Time `json:"lastEventAt"`
	EventCount   int       `json:"eventCount"`
	IsConnected  bool      `json:"isConnected"`
}

type Stats struct {
	TotalEvents      int            `json:"totalEvents"`
	EventsByType     map[string]int `json:"eventsByType"`
	ConnectedNodes   int            `json:"connectedNodes"`
	ErrorCount       int            `json:"errorCount"`
	WarningCount     int            `json:"warningCount"`
	StartTime        time.Time      `json:"startTime"`
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func NewTelemetryViewer(telemetryAddr, webAddr, logFile string) *TelemetryViewer {
	tv := &TelemetryViewer{
		telemetryAddr: telemetryAddr,
		webAddr:       webAddr,
		logFile:       logFile,
		peerMap:       make(map[string]PeerConfig),
		clients:       make(map[*websocket.Conn]bool),
		broadcast:     make(chan TelemetryEvent, 1000),
		eventBuffer:   make([]TelemetryEvent, 0),
		maxEvents:     2000,
		nodes:         make(map[int]*NodeInfo),
		stats: &Stats{
			EventsByType: make(map[string]int),
			StartTime:    time.Now(),
		},
	}

	// Load peer identification
	tv.loadPeerIdentification()

	return tv
}

func (tv *TelemetryViewer) loadPeerIdentification() {
	data, err := templateFS.ReadFile("peerIdentification.json")
	if err != nil {
		log.Printf("Warning: Could not load peerIdentification.json: %v", err)
		return
	}

	var peerIdent PeerIdentification
	if err := json.Unmarshal(data, &peerIdent); err != nil {
		log.Printf("Warning: Could not parse peerIdentification.json: %v", err)
		return
	}

	for _, p := range peerIdent.Validators {
		tv.peerMap[p.PeerID] = p
		log.Printf("Loaded peer: %s -> Node %d (%s)", p.PeerID, p.Node, p.Role)
	}

	log.Printf("Loaded %d peer identifications", len(tv.peerMap))
}

// lookupPeerID looks up the validator index for a given peer ID (base32 or hex)
func (tv *TelemetryViewer) lookupPeerID(peerIDBytes []byte) (int, string, string) {
	// Convert to base32 (lowercase, no padding)
	base32PeerID := base32Encode(peerIDBytes)

	if config, ok := tv.peerMap[base32PeerID]; ok {
		return config.Node, config.Name, config.Role
	}

	return -1, "Unknown", "unknown"
}

// lookupPeerIDBySAN looks up the validator index for a given SAN-encoded peer ID
func (tv *TelemetryViewer) lookupPeerIDBySAN(sanPeerID string) (int, string, string) {
	if config, ok := tv.peerMap[sanPeerID]; ok {
		return config.Node, config.Name, config.Role
	}
	return -1, "Unknown", "unknown"
}

// base32Encode encodes bytes to lowercase base32 without padding (matching polkajam format)
func base32Encode(data []byte) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz234567"
	var result strings.Builder

	bits := uint64(0)
	bitsLeft := 0

	for _, b := range data {
		bits = (bits << 8) | uint64(b)
		bitsLeft += 8

		for bitsLeft >= 5 {
			bitsLeft -= 5
			idx := (bits >> bitsLeft) & 0x1F
			result.WriteByte(alphabet[idx])
		}
	}

	if bitsLeft > 0 {
		idx := (bits << (5 - bitsLeft)) & 0x1F
		result.WriteByte(alphabet[idx])
	}

	return result.String()
}

func (tv *TelemetryViewer) Start() error {
	// Start broadcast handler
	go tv.handleBroadcast()

	// Start telemetry listener
	go tv.startTelemetryListener()

	// If log file specified, also tail it
	if tv.logFile != "" {
		go tv.tailLogFile()
	}

	// Start web server
	return tv.startWebServer()
}

func (tv *TelemetryViewer) startTelemetryListener() {
	listener, err := net.Listen("tcp", tv.telemetryAddr)
	if err != nil {
		log.Printf("Failed to listen on %s: %v", tv.telemetryAddr, err)
		return
	}
	defer listener.Close()

	log.Printf("Telemetry listener started on %s", tv.telemetryAddr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Accept error: %v", err)
			continue
		}
		go tv.handleTelemetryConnection(conn)
	}
}

func (tv *TelemetryViewer) handleTelemetryConnection(conn net.Conn) {
	defer conn.Close()

	peerAddr := conn.RemoteAddr().String()
	log.Printf("New telemetry connection from %s", peerAddr)

	// Read node info first and extract peer ID
	nodeID, nodeName, nodeRole, peerID, peerIDHex, peerIP, implName, implVersion := tv.readNodeInfo(conn)

	// Register node
	tv.nodesMu.Lock()
	tv.nodes[nodeID] = &NodeInfo{
		NodeID:      nodeID,
		NodeName:    nodeName,
		NodeRole:    nodeRole,
		PeerID:      peerID,
		PeerIDHex:   peerIDHex,
		PeerIP:      peerIP,
		Addr:        peerAddr,
		ImplName:    implName,
		ImplVersion: implVersion,
		ConnectedAt: time.Now(),
		IsConnected: true,
	}
	tv.nodesMu.Unlock()

	tv.updateConnectedNodes()
	log.Printf("Node connected: %s (Node %d, %s) from %s", nodeName, nodeID, nodeRole, peerAddr)

	// Read events
	for {
		event, err := tv.readEvent(conn, nodeID, nodeName, nodeRole)
		if err != nil {
			if err != io.EOF {
				log.Printf("Error reading from %s: %v", peerAddr, err)
			}
			break
		}
		tv.broadcast <- event
	}

	// Mark node as disconnected
	tv.nodesMu.Lock()
	if node, ok := tv.nodes[nodeID]; ok {
		node.IsConnected = false
	}
	tv.nodesMu.Unlock()

	tv.updateConnectedNodes()
	log.Printf("Node disconnected: %s (Node %d) from %s", nodeName, nodeID, peerAddr)
}

func (tv *TelemetryViewer) readNodeInfo(conn net.Conn) (nodeID int, nodeName, nodeRole, peerID, peerIDHex, peerIP, implName, implVersion string) {
	// Defaults
	nodeID = -1
	nodeName = "Unknown"
	nodeRole = "unknown"
	peerIP = ""
	implName = "unknown"
	implVersion = "unknown"

	var length uint32
	if err := binary.Read(conn, binary.LittleEndian, &length); err != nil {
		log.Printf("Failed to read node info length: %v", err)
		return
	}

	data := make([]byte, length)
	if _, err := io.ReadFull(conn, data); err != nil {
		log.Printf("Failed to read node info content: %v", err)
		return
	}

	// Parse NodeInfo - try two strategies:
	// 1. Scan for IPv4-mapped address pattern (0000ffff) - works for PolkaJam
	// 2. Parse using E()-encoded JAM params length - works for jamduna
	//
	// Structure:
	// - Protocol version (1 byte)
	// - JAM Parameters (E() length + bytes)
	// - Genesis hash (32 bytes)
	// - Peer ID (32 bytes)
	// - IPv6 Address (16 bytes) + Port (2 bytes)
	// - Flags (4 bytes)
	// - Strings: name, version, gp_version, note

	peerIDOffset := -1

	// Strategy 1: Look for IPv4-mapped address pattern (PolkaJam format)
	for i := 32; i < len(data)-22; i++ {
		if data[i] == 0 && data[i+1] == 0 &&
			data[i+2] == 0 && data[i+3] == 0 && data[i+4] == 0 && data[i+5] == 0 &&
			data[i+6] == 0 && data[i+7] == 0 && data[i+8] == 0 && data[i+9] == 0 &&
			data[i+10] == 0xff && data[i+11] == 0xff {
			peerIDOffset = i - 32
			break
		}
	}

	// Strategy 2: Parse structure using E() encoding (jamduna format)
	if peerIDOffset < 0 && len(data) > 1 {
		offset := 1 // Skip protocol version
		jamParamsLen, varintBytes := decodeVarint(data[offset:])
		offset += varintBytes + int(jamParamsLen) // Skip JAM params
		offset += 32                               // Skip genesis hash
		if offset >= 0 && offset+32 <= len(data) {
			peerIDOffset = offset
		}
	}

	if peerIDOffset < 0 || peerIDOffset+32 > len(data) {
		log.Printf("NodeInfo: could not find peer ID in %d bytes (tried both strategies)", length)
		return
	}

	// Extract Peer ID (32 bytes)
	peerIDBytes := data[peerIDOffset : peerIDOffset+32]
	peerIDHex = hex.EncodeToString(peerIDBytes)
	peerID = common.ToSAN(peerIDBytes)
	nodeID, nodeName, nodeRole = tv.lookupPeerIDBySAN(peerID)

	// Extract IP address (16 bytes IPv6) and port (2 bytes) after peer ID
	ipOffset := peerIDOffset + 32
	if ipOffset+18 <= len(data) {
		ipBytes := data[ipOffset : ipOffset+16]
		portBytes := data[ipOffset+16 : ipOffset+18]
		port := uint16(portBytes[0]) | uint16(portBytes[1])<<8 // little-endian

		// Check if it's an IPv4-mapped IPv6 address (::ffff:x.x.x.x)
		if ipBytes[10] == 0xff && ipBytes[11] == 0xff {
			// IPv4-mapped: last 4 bytes are the IPv4 address
			peerIP = fmt.Sprintf("%d.%d.%d.%d:%d", ipBytes[12], ipBytes[13], ipBytes[14], ipBytes[15], port)
		} else {
			// Full IPv6 - format as [IPv6]:port
			ip := net.IP(ipBytes)
			peerIP = fmt.Sprintf("[%s]:%d", ip.String(), port)
		}
	}

	// Parse strings after peer ID + IPv6 (16) + port (2) + flags (4) = 22 bytes
	strOffset := peerIDOffset + 32 + 22
	if strOffset < len(data) {
		var strBytes int
		implName, strBytes = decodeString(data[strOffset:])
		strOffset += strBytes
		if strOffset < len(data) {
			implVersion, strBytes = decodeString(data[strOffset:])
		}
	}

	log.Printf("Parsed node info: SAN=%s, NodeID=%d, Name=%s, IP=%s, Impl=%s/%s",
		peerID, nodeID, nodeName, peerIP, implName, implVersion)

	return
}

// decodeVarint decodes a GP-style E() encoded integer (variable-length natural number)
// Uses types.DecodeE from the JAM codec
func decodeVarint(data []byte) (uint64, int) {
	val, bytesRead := types.DecodeE(data)
	return val, int(bytesRead)
}

func decodeString(data []byte) (string, int) {
	if len(data) == 0 {
		return "", 0
	}

	strLen, bytesRead := decodeVarint(data)
	if bytesRead+int(strLen) > len(data) {
		return "", bytesRead
	}

	return string(data[bytesRead : bytesRead+int(strLen)]), bytesRead + int(strLen)
}

func (tv *TelemetryViewer) readEvent(conn net.Conn, nodeID int, nodeName, nodeRole string) (TelemetryEvent, error) {
	var length uint32
	if err := binary.Read(conn, binary.LittleEndian, &length); err != nil {
		return TelemetryEvent{}, err
	}

	data := make([]byte, length)
	if _, err := io.ReadFull(conn, data); err != nil {
		return TelemetryEvent{}, err
	}

	if len(data) < 9 {
		return TelemetryEvent{}, fmt.Errorf("event too short")
	}

	timestamp := binary.LittleEndian.Uint64(data[:8])
	discriminator := int(data[8])
	payload := data[9:]

	jamTime := common.JceStart.Add(time.Duration(timestamp) * time.Microsecond)
	timeStr := jamTime.UTC().Format("2006-01-02T15:04:05.000Z")

	eventType := getEventTypeName(discriminator)
	decodedData := decodePayload(discriminator, payload)
	level := getEventLevel(eventType)

	// Format line for logViewer compatibility: "2024-01-01 12:34:56 N0 INFO telemetry::EVENT_TYPE message"
	// Convert timestamp to expected format
	tsFormatted := jamTime.UTC().Format("2006-01-02 15:04:05")
	levelUpper := strings.ToUpper(level)
	// Build module path for filtering: telemetry::category::EVENT_TYPE (discriminator)
	// Each event gets its own filter entry with discriminator at the end for display
	category := getEventCategory(discriminator)
	eventWithDiscriminator := fmt.Sprintf("%s (%d)", eventType, discriminator)
	modulePath := fmt.Sprintf("telemetry::%s::%s", category, eventWithDiscriminator)
	line := fmt.Sprintf("%s %s %s %s %s", tsFormatted, nodeName, levelUpper, modulePath, decodedData)

	event := TelemetryEvent{
		Type:          "log",
		Node:          nodeID,
		Line:          line,
		Timestamp:     timeStr,
		EventType:     eventType,
		Data:          decodedData,
		PeerAddr:      conn.RemoteAddr().String(),
		NodeName:      nodeName,
		NodeRole:      nodeRole,
		Level:         level,
		Discriminator: discriminator,
	}

	// Update node stats
	tv.nodesMu.Lock()
	if node, ok := tv.nodes[nodeID]; ok {
		node.LastEventAt = time.Now()
		node.EventCount++
	}
	tv.nodesMu.Unlock()

	return event, nil
}

func (tv *TelemetryViewer) tailLogFile() {
	file, err := os.Open(tv.logFile)
	if err != nil {
		log.Printf("Cannot open log file %s: %v", tv.logFile, err)
		return
	}
	defer file.Close()

	file.Seek(0, io.SeekEnd)

	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		event := tv.parseLogLine(strings.TrimSpace(line))
		if event.EventType != "" {
			tv.broadcast <- event
		}
	}
}

func (tv *TelemetryViewer) parseLogLine(logLine string) TelemetryEvent {
	parts := strings.Split(logLine, "|")
	if len(parts) < 3 {
		return TelemetryEvent{}
	}

	event := TelemetryEvent{
		Type:      "log",
		Node:      -1,
		Line:      logLine,
		Timestamp: parts[0],
		EventType: parts[1],
		Data:      strings.Join(parts[2:], "|"),
		NodeName:  "Log",
		NodeRole:  "unknown",
	}

	event.Level = getEventLevel(event.EventType)
	return event
}

func (tv *TelemetryViewer) handleBroadcast() {
	for event := range tv.broadcast {
		// Update stats
		tv.statsMu.Lock()
		tv.stats.TotalEvents++
		tv.stats.EventsByType[event.EventType]++
		if event.Level == "error" {
			tv.stats.ErrorCount++
		} else if event.Level == "warning" {
			tv.stats.WarningCount++
		}
		tv.statsMu.Unlock()

		// Add to buffer
		tv.eventBufferMu.Lock()
		tv.eventBuffer = append(tv.eventBuffer, event)
		if len(tv.eventBuffer) > tv.maxEvents {
			tv.eventBuffer = tv.eventBuffer[1:]
		}
		tv.eventBufferMu.Unlock()

		// Broadcast to all clients
		tv.clientsMu.RLock()
		for client := range tv.clients {
			err := client.WriteJSON(event)
			if err != nil {
				client.Close()
				delete(tv.clients, client)
			}
		}
		tv.clientsMu.RUnlock()
	}
}

func (tv *TelemetryViewer) updateConnectedNodes() {
	tv.nodesMu.RLock()
	count := 0
	for _, node := range tv.nodes {
		if node.IsConnected {
			count++
		}
	}
	tv.nodesMu.RUnlock()

	tv.statsMu.Lock()
	tv.stats.ConnectedNodes = count
	tv.statsMu.Unlock()
}

func (tv *TelemetryViewer) startWebServer() error {
	tmpl, err := template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return fmt.Errorf("failed to parse templates: %w", err)
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		tmpl.ExecuteTemplate(w, "index.html", nil)
	})

	http.HandleFunc("/ws", tv.handleWebSocket)

	http.HandleFunc("/api/stats", func(w http.ResponseWriter, r *http.Request) {
		tv.statsMu.RLock()
		defer tv.statsMu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tv.stats)
	})

	http.HandleFunc("/api/nodes", func(w http.ResponseWriter, r *http.Request) {
		tv.nodesMu.RLock()
		defer tv.nodesMu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tv.nodes)
	})

	http.HandleFunc("/api/events", func(w http.ResponseWriter, r *http.Request) {
		tv.eventBufferMu.RLock()
		defer tv.eventBufferMu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tv.eventBuffer)
	})

	http.HandleFunc("/peers", func(w http.ResponseWriter, r *http.Request) {
		data, err := templateFS.ReadFile("peerIdentification.json")
		if err != nil {
			http.Error(w, "Peer config not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	})

	// /reload endpoint - returns all buffered events as newline-delimited JSON
	http.HandleFunc("/reload", func(w http.ResponseWriter, r *http.Request) {
		tv.eventBufferMu.RLock()
		defer tv.eventBufferMu.RUnlock()

		w.Header().Set("Content-Type", "application/x-ndjson")
		// Return events as newline-delimited JSON (one event per line)
		encoder := json.NewEncoder(w)
		for _, event := range tv.eventBuffer {
			encoder.Encode(event)
		}
	})

	// /filters endpoint - stub for filter persistence (returns empty for now)
	http.HandleFunc("/filters", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "DELETE" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method == "POST" {
			// Accept but ignore filter saves for now
			w.WriteHeader(http.StatusOK)
			return
		}
		// GET - return empty filters
		w.Write([]byte("{}"))
	})

	// /version endpoint
	http.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"version": "1.0.0",
			"name":    "JAM Telemetry Viewer",
		})
	})

	log.Printf("Web UI available at http://%s", tv.webAddr)
	return http.ListenAndServe(tv.webAddr, nil)
}

func (tv *TelemetryViewer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	tv.clientsMu.Lock()
	tv.clients[conn] = true
	tv.clientsMu.Unlock()

	// Send buffered events
	tv.eventBufferMu.RLock()
	for _, event := range tv.eventBuffer {
		conn.WriteJSON(event)
	}
	tv.eventBufferMu.RUnlock()

	// Keep connection alive
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			tv.clientsMu.Lock()
			delete(tv.clients, conn)
			tv.clientsMu.Unlock()
			conn.Close()
			break
		}
	}
}

// Event type mappings
var eventTypeNames = map[int]string{
	0:   "DROPPED",
	10:  "STATUS",
	11:  "BEST_BLOCK_CHANGED",
	12:  "FINALIZED_BLOCK_CHANGED",
	13:  "SYNC_STATUS_CHANGED",
	20:  "CONNECTION_REFUSED",
	21:  "CONNECTING_IN",
	22:  "CONNECT_IN_FAILED",
	23:  "CONNECTED_IN",
	24:  "CONNECTING_OUT",
	25:  "CONNECT_OUT_FAILED",
	26:  "CONNECTED_OUT",
	27:  "DISCONNECTED",
	28:  "PEER_MISBEHAVED",
	40:  "AUTHORING",
	41:  "AUTHORING_FAILED",
	42:  "AUTHORED",
	43:  "IMPORTING",
	44:  "BLOCK_VERIFICATION_FAILED",
	45:  "BLOCK_VERIFIED",
	46:  "BLOCK_EXECUTION_FAILED",
	47:  "BLOCK_EXECUTED",
	48:  "ACCUMULATE_RESULT_AVAILABLE",
	60:  "BLOCK_ANNOUNCEMENT_STREAM_OPENED",
	61:  "BLOCK_ANNOUNCEMENT_STREAM_CLOSED",
	62:  "BLOCK_ANNOUNCED",
	63:  "SENDING_BLOCK_REQUEST",
	64:  "RECEIVING_BLOCK_REQUEST",
	65:  "BLOCK_REQUEST_FAILED",
	66:  "BLOCK_REQUEST_SENT",
	67:  "BLOCK_REQUEST_RECEIVED",
	68:  "BLOCK_TRANSFERRED",
	71:  "BLOCK_ANNOUNCEMENT_MALFORMED",
	80:  "GENERATING_TICKETS",
	81:  "TICKET_GENERATION_FAILED",
	82:  "TICKETS_GENERATED",
	83:  "TICKET_TRANSFER_FAILED",
	84:  "TICKET_TRANSFERRED",
	90:  "WORK_PACKAGE_SUBMISSION",
	91:  "WORK_PACKAGE_BEING_SHARED",
	92:  "WORK_PACKAGE_FAILED",
	93:  "DUPLICATE_WORK_PACKAGE",
	94:  "WORK_PACKAGE_RECEIVED",
	95:  "AUTHORIZED",
	96:  "EXTRINSIC_DATA_RECEIVED",
	97:  "IMPORTS_RECEIVED",
	98:  "SHARING_WORK_PACKAGE",
	99:  "WORK_PACKAGE_SHARING_FAILED",
	100: "BUNDLE_SENT",
	101: "REFINED",
	102: "WORK_REPORT_BUILT",
	103: "WORK_REPORT_SIGNATURE_SENT",
	104: "WORK_REPORT_SIGNATURE_RECEIVED",
	105: "GUARANTEE_BUILT",
	106: "SENDING_GUARANTEE",
	107: "GUARANTEE_SEND_FAILED",
	108: "GUARANTEE_SENT",
	109: "GUARANTEES_DISTRIBUTED",
	110: "RECEIVING_GUARANTEE",
	111: "GUARANTEE_RECEIVE_FAILED",
	112: "GUARANTEE_RECEIVED",
	113: "GUARANTEE_DISCARDED",
	120: "SENDING_SHARD_REQUEST",
	121: "RECEIVING_SHARD_REQUEST",
	122: "SHARD_REQUEST_FAILED",
	123: "SHARD_REQUEST_SENT",
	124: "SHARD_REQUEST_RECEIVED",
	125: "SHARDS_TRANSFERRED",
	126: "DISTRIBUTING_ASSURANCE",
	127: "ASSURANCE_SEND_FAILED",
	128: "ASSURANCE_SENT",
	129: "ASSURANCE_DISTRIBUTED",
	130: "ASSURANCE_RECEIVE_FAILED",
	131: "ASSURANCE_RECEIVED",
	132: "CONTEXT_AVAILABLE",
	133: "ASSURANCE_PROVIDED",
	140: "SENDING_BUNDLE_SHARD_REQUEST",
	141: "RECEIVING_BUNDLE_SHARD_REQUEST",
	142: "BUNDLE_SHARD_REQUEST_FAILED",
	143: "BUNDLE_SHARD_REQUEST_SENT",
	144: "BUNDLE_SHARD_REQUEST_RECEIVED",
	145: "BUNDLE_SHARD_TRANSFERRED",
	146: "RECONSTRUCTING_BUNDLE",
	147: "BUNDLE_RECONSTRUCTED",
	148: "SENDING_BUNDLE_REQUEST",
	149: "RECEIVING_BUNDLE_REQUEST",
	150: "BUNDLE_REQUEST_FAILED",
	151: "BUNDLE_REQUEST_SENT",
	152: "BUNDLE_REQUEST_RECEIVED",
	153: "BUNDLE_TRANSFERRED",
	160: "WORK_PACKAGE_HASH_MAPPED",
	161: "SEGMENTS_ROOT_MAPPED",
	162: "SENDING_SEGMENT_SHARD_REQUEST",
	163: "RECEIVING_SEGMENT_SHARD_REQUEST",
	164: "SEGMENT_SHARD_REQUEST_FAILED",
	165: "SEGMENT_SHARD_REQUEST_SENT",
	166: "SEGMENT_SHARD_REQUEST_RECEIVED",
	167: "SEGMENT_SHARDS_TRANSFERRED",
	168: "RECONSTRUCTING_SEGMENTS",
	169: "SEGMENT_RECONSTRUCTION_FAILED",
	170: "SEGMENTS_RECONSTRUCTED",
	171: "SEGMENT_VERIFICATION_FAILED",
	172: "SEGMENTS_VERIFIED",
	173: "SENDING_SEGMENT_REQUEST",
	174: "RECEIVING_SEGMENT_REQUEST",
	175: "SEGMENT_REQUEST_FAILED",
	176: "SEGMENT_REQUEST_SENT",
	177: "SEGMENT_REQUEST_RECEIVED",
	178: "SEGMENTS_TRANSFERRED",
	190: "PREIMAGE_ANNOUNCEMENT_FAILED",
	191: "PREIMAGE_ANNOUNCED",
	192: "ANNOUNCED_PREIMAGE_FORGOTTEN",
	193: "SENDING_PREIMAGE_REQUEST",
	194: "RECEIVING_PREIMAGE_REQUEST",
	195: "PREIMAGE_REQUEST_FAILED",
	196: "PREIMAGE_REQUEST_SENT",
	197: "PREIMAGE_REQUEST_RECEIVED",
	198: "PREIMAGE_TRANSFERRED",
	199: "PREIMAGE_DISCARDED",
}

func getEventTypeName(discriminator int) string {
	if name, ok := eventTypeNames[discriminator]; ok {
		return name
	}
	return fmt.Sprintf("UNKNOWN_%d", discriminator)
}

func getEventLevel(eventType string) string {
	if strings.Contains(eventType, "FAILED") || strings.Contains(eventType, "ERROR") {
		return "error"
	}
	if strings.Contains(eventType, "REFUSED") || strings.Contains(eventType, "MISBEHAVED") || strings.Contains(eventType, "DISCARDED") {
		return "warning"
	}
	return "info"
}

// getEventCategory returns the category for a discriminator for hierarchical filtering
// Categories match JIP-3 event groups
func getEventCategory(discriminator int) string {
	switch {
	case discriminator == 0:
		return "Dropped"
	case discriminator >= 10 && discriminator <= 13:
		return "Status"
	case discriminator >= 20 && discriminator <= 28:
		return "Networking"
	case discriminator >= 40 && discriminator <= 48:
		return "Block"
	case discriminator >= 60 && discriminator <= 71:
		return "BlockSync"
	case discriminator >= 80 && discriminator <= 84:
		return "Safrole"
	case discriminator >= 90 && discriminator <= 113:
		return "WorkPackage"
	case discriminator >= 120 && discriminator <= 133:
		return "Assurance"
	case discriminator >= 140 && discriminator <= 153:
		return "Bundle"
	case discriminator >= 160 && discriminator <= 178:
		return "Segment"
	case discriminator >= 190 && discriminator <= 199:
		return "Preimage"
	default:
		return "Unknown"
	}
}

func decodePayload(discriminator int, payload []byte) string {
	if len(payload) == 0 {
		return ""
	}

	// Use the telemetry package's decoder if available
	decoded := telemetry.DecodeEvent(discriminator, payload)
	if decoded != "" {
		return decoded
	}

	// Fallback to hex for unknown events
	if len(payload) > 64 {
		return fmt.Sprintf("0x%x... (%d bytes)", payload[:32], len(payload))
	}
	return fmt.Sprintf("0x%x", payload)
}

func main() {
	telemetryAddr := flag.String("telemetry", "0.0.0.0:9999", "Address to listen for telemetry connections")
	webAddr := flag.String("web", "0.0.0.0:8088", "Address for web UI (8088 to avoid conflict with logViewer on 8080)")
	logFile := flag.String("log", "", "Optional: tail existing telemetry log file")
	flag.Parse()

	fmt.Println("JAM Telemetry Viewer")
	fmt.Println("====================")
	fmt.Printf("Telemetry listener: %s\n", *telemetryAddr)
	fmt.Printf("Web UI: http://%s\n", *webAddr)
	if *logFile != "" {
		fmt.Printf("Tailing log file: %s\n", *logFile)
	}
	fmt.Println()

	viewer := NewTelemetryViewer(*telemetryAddr, *webAddr, *logFile)
	if err := viewer.Start(); err != nil {
		log.Fatalf("Failed to start viewer: %v", err)
	}
}
