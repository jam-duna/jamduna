package node

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/big"

	"sync"
	"time"

	"github.com/colorfulnotion/jam/common"

	"github.com/colorfulnotion/jam/types"
	"github.com/quic-go/quic-go"
)

type DADistributeECChunk struct {
	SegmentRoot []byte `json:"segment_root"`
	Data        []byte `json:"data"`
}

type DADistributeECChunks []DADistributeECChunk

type DAAnnouncement struct {
	SegmentRoot []byte `json:"segment_root"`
	State       bool   `json:"state"`
}
type DABroadcasted struct {
	SegmentRoot []byte `json:"segment_root"`
}

func (ann *DAAnnouncement) ToBytes() ([]byte, error) {
	buf := new(bytes.Buffer)
	// Serialize HeaderHash (32 bytes)
	if _, err := buf.Write(ann.SegmentRoot); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, ann.State); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
func (ann *DABroadcasted) ToBytes() ([]byte, error) {
	buf := new(bytes.Buffer)
	// Serialize HeaderHash (32 bytes)
	if _, err := buf.Write(ann.SegmentRoot); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (ann *DAAnnouncement) FromBytes(data []byte) error {
	buf := bytes.NewReader(data)

	// Deserialize HeaderHash (32 bytes)
	ann.SegmentRoot = make([]byte, 32)
	if _, err := buf.Read(ann.SegmentRoot); err != nil {
		return err
	}
	if err := binary.Read(buf, binary.LittleEndian, &ann.State); err != nil {
		return err
	}
	return nil

}

// Marshal marshals DistributeECChunk into JSON
func (d *DADistributeECChunk) Marshal() ([]byte, error) {
	return json.Marshal(d)
}

// Unmarshal unmarshals JSON data into DistributeECChunk
func (d *DADistributeECChunk) Unmarshal(data []byte) error {
	return json.Unmarshal(data, d)
}

func (d *DADistributeECChunk) Bytes() []byte {
	jsonData, _ := d.Marshal()
	return jsonData
}

func (d *DADistributeECChunk) FromBytes(data []byte) error {
	return json.Unmarshal(data, d)
}

func (d *DADistributeECChunk) String() string {
	return string(d.Bytes())
}

type Chunk struct {
	Hash common.Hash
	Data []byte
}

func getRandomSize() int {
	// sizes := []int{2, 128, 1024, 10240, 1048576} // {2Byte, 128Bytes, 1KBytes, 10KBytes, 1MBytes}
	sizes := []int{129, 1025, 4105, 10241, 66666, 129999, 258000, 520000, 1048577}
	// sizes := []int{129, 1025}
	sizes = oddToEven(sizes)
	idx, _ := rand.Int(rand.Reader, big.NewInt(int64(len(sizes))))
	size := sizes[idx.Int64()]
	return size
}

func oddToEven(nums []int) []int {
	for i, num := range nums {
		if num%2 != 0 {
			nums[i] = num + 1
		}
	}
	return nums
}

func GenerateRandomData(size int) []byte {
	data := make([]byte, size)
	_, _ = rand.Read(data)
	return data
}

func generateRandomData(size int) []byte {
	data := make([]byte, size)
	_, _ = rand.Read(data)
	return data
}

var errorStats = struct {
	mu       sync.Mutex
	category map[string]int
	count    int64
}{category: make(map[string]int)}

func (n *Node) checkAndReconnect(reconnectTimes int) error {
	connections := make([]bool, types.TotalValidators)
	const retryPrintTimes = 500
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstError error

	// Timeout context for connection attempts
	const connectionTimeout = 400 * time.Millisecond

	for id, p := range n.peersInfo {
		if n.id == uint16(id) {
			continue
		}
		wg.Add(1)

		go func(id uint16, p *Peer) {
			defer wg.Done()

			quicConfig := GenerateQuicConfig()
			ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
			defer cancel()

			// Connection check and handling
			if p.conn == nil || p.conn.Context().Err() != nil {
				// Lock to safely update connection state
				p.connectionMu.Lock()
				defer p.connectionMu.Unlock()

				if reconnectTimes > retryPrintTimes {
					fmt.Printf("Connection lost with %s after %d seconds, reconnecting...\n",
						p.PeerAddr, reconnectTimes*100/1000)
				}

				// Attempt to close existing connection if it exists
				if p.conn != nil {
					_ = p.conn.CloseWithError(0, "Connection lost, reconnecting...")
				}

				// Try to reconnect
				conn, err := quic.DialAddr(ctx, p.PeerAddr, p.node.tlsConfig, quicConfig)
				if err != nil {
					if reconnectTimes > retryPrintTimes {
						fmt.Printf("Reconnection failed for %s: %v\n", p.PeerAddr, err)
					}

					// Capture the first error safely
					mu.Lock()
					if firstError == nil {
						firstError = fmt.Errorf("failed to reconnect to %s: %v", p.PeerAddr, err)
					}
					mu.Unlock()
					return
				}

				p.conn = conn
				if reconnectTimes > retryPrintTimes {
					fmt.Printf("Reconnected to %s successfully\n", p.PeerAddr)
				}

				mu.Lock()
				connections[p.PeerID] = true
				mu.Unlock()
			} else {
				if reconnectTimes > retryPrintTimes {
					fmt.Printf("Connection with %s is still alive\n", p.PeerAddr)
				}

				mu.Lock()
				connections[p.PeerID] = true
				mu.Unlock()
			}
		}(id, p)
	}

	wg.Wait()

	// Check if any connection is still down
	for i, connected := range connections {
		if !connected {
			return fmt.Errorf("Node %d connection lost with %d", n.id, i)
		}
	}

	// Return the first reconnection error encountered, if any
	if firstError != nil {
		return firstError
	}

	return nil
}

func printErrorStatistics() {
	errorStats.mu.Lock()
	defer errorStats.mu.Unlock()
	fmt.Println("----- Error Statistics -----")
	for category, count := range errorStats.category {
		fmt.Printf("%s: %d occurrences\n", category, count)
	}
	fmt.Println("----------------------------")
}

func categorizeAndCountError(err error) {
	if err == nil {
		return
	}
	errorStr := err.Error()

	// Remove the error message after the colon
	errorStats.mu.Lock()
	errorStats.category[errorStr]++
	errorStats.count++
	errorStats.mu.Unlock()
}
