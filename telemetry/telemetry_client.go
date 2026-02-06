package telemetry

import (
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/jam-duna/jamduna/common"
	"github.com/jam-duna/jamduna/types"
)

// Type aliases to use types from types package
type NodeInfo = types.NodeInfo

// TelemetryClient manages the lifecycle of the connection to the telemetry server.
type TelemetryClient struct {
	addr        string
	conn        net.Conn
	connMu      sync.Mutex
	nextEventID uint64
	eventIDMu   sync.Mutex
	disabled    bool     // if true, telemetry is disabled (no-op)
	nodeInfo    NodeInfo // stored for reconnection
	reconnecting bool    // true if reconnect goroutine is running
}

// NewNoOpTelemetryClient creates a disabled telemetry client that does nothing
func NewNoOpTelemetryClient() *TelemetryClient {
	return &TelemetryClient{
		disabled: true,
	}
}

// NewTelemetryClient builds a telemetry client targeting the given host/port.
func NewTelemetryClient(host, port string) *TelemetryClient {
	return &TelemetryClient{
		addr:        fmt.Sprintf("%s:%s", host, port),
		nextEventID: 0,
	}
}

// GetEventID returns a new unique event ID for linking related telemetry events.
// Event IDs are assigned sequentially starting from 0 as per JIP-3 specification.
func (c *TelemetryClient) GetEventID(...interface{}) uint64 {
	c.eventIDMu.Lock()
	defer c.eventIDMu.Unlock()
	id := c.nextEventID
	c.nextEventID++
	return id
}

// Connect establishes a TCP connection to the telemetry server, sends the node information message,
// and stores the connection for later reuse.
func (c *TelemetryClient) Connect(nodeInfo NodeInfo) error {
	c.connMu.Lock()
	defer c.connMu.Unlock()

	if c.conn != nil {
		return fmt.Errorf("telemetry client already connected to %s", c.addr)
	}

	// Store nodeInfo for reconnection
	c.nodeInfo = nodeInfo

	conn, err := net.Dial("tcp", c.addr)
	if err != nil {
		// Start background reconnection
		c.startReconnect()
		return fmt.Errorf("failed to connect to telemetry server at %s: %w", c.addr, err)
	}

	// Send node information message
	if err := sendNodeInfo(conn, nodeInfo); err != nil {
		conn.Close()
		c.startReconnect()
		return fmt.Errorf("failed to send node info: %w", err)
	}

	c.conn = conn
	return nil
}

// startReconnect starts a background goroutine to reconnect to the telemetry server.
// Must be called with connMu held.
func (c *TelemetryClient) startReconnect() {
	if c.reconnecting || c.disabled {
		return
	}
	c.reconnecting = true
	go c.reconnectLoop()
}

// reconnectLoop attempts to reconnect to the telemetry server with exponential backoff.
func (c *TelemetryClient) reconnectLoop() {
	backoff := time.Second
	maxBackoff := 30 * time.Second

	for {
		time.Sleep(backoff)

		c.connMu.Lock()
		if c.disabled {
			c.reconnecting = false
			c.connMu.Unlock()
			return
		}

		// Try to connect
		conn, err := net.Dial("tcp", c.addr)
		if err != nil {
			c.connMu.Unlock()
			// Exponential backoff
			backoff = backoff * 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}

		// Send node information message
		if err := sendNodeInfo(conn, c.nodeInfo); err != nil {
			conn.Close()
			c.connMu.Unlock()
			backoff = backoff * 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}

		// Success - store connection and exit loop
		c.conn = conn
		c.reconnecting = false
		c.connMu.Unlock()
		return
	}
}

// Conn returns the underlying telemetry connection, if established.
func (c *TelemetryClient) Conn() net.Conn {
	return c.conn
}

// Close terminates the telemetry connection and clears the stored state.
func (c *TelemetryClient) Close() error {
	c.connMu.Lock()
	defer c.connMu.Unlock()

	// Mark as disabled to stop reconnection attempts
	c.disabled = true

	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

// sendEvent sends a telemetry event message to the telemetry server.
// It automatically prepends the timestamp and discriminator to the event-specific payload.
// This method handles the complete message construction and JAMNP-S framing (length prefix + content).
func (c *TelemetryClient) sendEvent(discriminator byte, eventPayload []byte) {
	// No-op if telemetry is disabled
	if c.disabled {
		return
	}

	c.connMu.Lock()
	conn := c.conn
	if conn == nil {
		// No connection - start reconnect if not already running
		c.startReconnect()
		c.connMu.Unlock()
		return
	}
	c.connMu.Unlock()

	// Build complete message: timestamp + discriminator + event-specific payload
	var message []byte

	// Timestamp (8 bytes, little-endian) - microseconds since JAM Common Era
	timestamp := c.currentTimestamp()
	message = append(message, Uint64ToBytes(timestamp)...)

	// Discriminator (1 byte)
	message = append(message, discriminator)

	// Event-specific payload
	message = append(message, eventPayload...)

	// Send length prefix (little-endian 32-bit)
	lengthBytes := Uint32ToBytes(uint32(len(message)))
	if _, err := conn.Write(lengthBytes); err != nil {
		c.handleWriteError()
		return
	}

	// Send message content
	if _, err := conn.Write(message); err != nil {
		c.handleWriteError()
	}
}

// handleWriteError handles a write error by closing the connection and starting reconnection.
func (c *TelemetryClient) handleWriteError() {
	c.connMu.Lock()
	defer c.connMu.Unlock()

	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	c.startReconnect()
}

// currentTimestamp returns the number of microseconds elapsed since the
// beginning of the Jam "Common Era", clamped at zero for timestamps before the
// epoch. This matches the Timestamp definition in TELEMETRY.md.
func (c *TelemetryClient) currentTimestamp() uint64 {
	now := time.Now().UTC()
	if now.Before(common.JceStart) {
		return 0
	}
	return uint64(now.Sub(common.JceStart) / time.Microsecond)
}

// sendNodeInfo sends the initial node information message to the telemetry server
func sendNodeInfo(conn net.Conn, info NodeInfo) error {
	// Build the message content
	var msg []byte

	// Protocol version (0)
	msg = append(msg, 0)

	// JAM Parameters
	msg = append(msg, types.E(uint64(len(info.JAMParameters)))...)
	msg = append(msg, info.JAMParameters...)

	// Genesis header hash (32 bytes)
	msg = append(msg, info.GenesisHeaderHash.Bytes()...)

	// Peer ID (32 bytes)
	msg = append(msg, info.PeerID[:]...)

	// Peer Address (16 bytes IPv6 + 2 bytes port)
	msg = append(msg, info.PeerAddress[:]...)
	msg = append(msg, Uint16ToBytes(info.PeerPort)...)

	// Node flags (4 bytes)
	msg = append(msg, Uint32ToBytes(info.NodeFlags)...)

	// String<32> (Name)
	msg = append(msg, encodeString(info.NodeName, 32)...)

	// String<32> (Version)
	msg = append(msg, encodeString(info.NodeVersion, 32)...)

	// String<16> (Gray Paper version)
	msg = append(msg, encodeString(info.GrayPaperVersion, 16)...)

	// String<512> (Note)
	msg = append(msg, encodeString(info.Note, 512)...)

	// Send length prefix (little-endian 32-bit)
	if _, err := conn.Write(Uint32ToBytes(uint32(len(msg)))); err != nil {
		return err
	}

	// Send message content
	if _, err := conn.Write(msg); err != nil {
		return err
	}

	return nil
}

// encodeString encodes a string with length prefix and validates max length
func encodeString(s string, maxLen int) []byte {
	bytes := []byte(s)
	if len(bytes) > maxLen {
		bytes = bytes[:maxLen]
	}
	result := types.E(uint64(len(bytes)))
	result = append(result, bytes...)
	return result
}

// Uint64ToBytes converts a uint64 to little-endian bytes
func Uint64ToBytes(n uint64) []byte {
	bytes := make([]byte, 8)
	bytes[0] = byte(n)
	bytes[1] = byte(n >> 8)
	bytes[2] = byte(n >> 16)
	bytes[3] = byte(n >> 24)
	bytes[4] = byte(n >> 32)
	bytes[5] = byte(n >> 40)
	bytes[6] = byte(n >> 48)
	bytes[7] = byte(n >> 56)
	return bytes
}

// Uint32ToBytes converts a uint32 to little-endian bytes
func Uint32ToBytes(n uint32) []byte {
	bytes := make([]byte, 4)
	bytes[0] = byte(n)
	bytes[1] = byte(n >> 8)
	bytes[2] = byte(n >> 16)
	bytes[3] = byte(n >> 24)
	return bytes
}

// Uint16ToBytes converts a uint16 to little-endian bytes
func Uint16ToBytes(n uint16) []byte {
	bytes := make([]byte, 2)
	bytes[0] = byte(n)
	bytes[1] = byte(n >> 8)
	return bytes
}

// ParseTelemetryAddress converts a host and port string into the telemetry wire
// representation of a peer address. The host must parse to an IP address that
// fits in 16 bytes (IPv4 addresses are automatically mapped into IPv6 form).
func ParseTelemetryAddress(host, port string) ([16]byte, uint16, error) {
	var ipBytes [16]byte

	parsedIP := net.ParseIP(host)
	if parsedIP == nil {
		return ipBytes, 0, fmt.Errorf("invalid IP address: %s", host)
	}

	parsedIP = parsedIP.To16()
	if parsedIP == nil {
		return ipBytes, 0, fmt.Errorf("unable to convert IP address to 16 bytes: %s", host)
	}

	copy(ipBytes[:], parsedIP)

	portValue, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return ipBytes, 0, fmt.Errorf("invalid port: %s", port)
	}

	return ipBytes, uint16(portValue), nil
}

// parseTelemetryAddrString parses a "host:port" string into the telemetry peer
// address representation.
func parseTelemetryAddrString(addr string) ([16]byte, uint16, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return [16]byte{}, 0, err
	}
	return ParseTelemetryAddress(host, port)
}
