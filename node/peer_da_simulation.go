package node

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/big"

	"reflect"

	"sync"
	"time"

	"github.com/colorfulnotion/jam/common"

	"github.com/colorfulnotion/jam/log"
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

type DA_announcement struct {
	Hash   common.Hash
	PeerId uint16
}

type DA_request struct {
	Hash       common.Hash
	ShardIndex uint16
}

type DA_response struct {
	Hash       common.Hash
	ShardIndex uint16
	Data       []byte
}

type Chunk struct {
	Hash common.Hash
	Data []byte
}

func (n *Node) onDA_Announcement(stream quic.Stream, msg []byte) (err error) {
	defer stream.Close()
	var newReq DADistributeECChunk
	// Deserialize byte array back into the struct
	err = newReq.FromBytes(msg)
	if err != nil {
		fmt.Printf("Node %d Error deserializing onWorkReportDistribution: %v\n", n.id, err)
		// panic(11113)
		return nil
	}
	log.Trace(debugDA, "onDA_Announcement", "n", n.id, "newReq.Data", newReq.Data)
	n.chunkMap.Store(common.Hash(newReq.SegmentRoot), newReq.Data)
	return nil
}

func (p *Peer) DA_Request(an DA_request) ([]byte, error) {
	if p.PeerID == p.node.id {
		return p.node.DA_Lookup(an.Hash, uint32(an.ShardIndex))
	}
	stream, err := p.openStream(CE202_DA_Request)
	if err != nil {
		return nil, err
	}
	an_bytes, err := types.Encode(an)
	if err != nil {
		return nil, err
	}
	err = sendQuicBytes(stream, an_bytes)
	if err != nil {
		return nil, err
	}
	chunk_bytes, err := receiveQuicBytes(stream)
	if err != nil {
		return nil, err
	}
	return chunk_bytes, nil
}

func (n *Node) DA_Lookup(hash common.Hash, validator_idx uint32) ([]byte, error) {

	// check the chunkbox
	if chunks, ok := n.chunkBox[hash]; ok {
		chunk := chunks[validator_idx]
		return chunk, nil
	}
	return nil, fmt.Errorf("chunk not found")
}
func (n *Node) onDA_Request(stream quic.Stream, msg []byte) (err error) {
	defer stream.Close()
	an, _, err := types.Decode(msg, reflect.TypeOf(DA_request{}))
	if err != nil {
		return err
	}
	hash := an.(DA_request).Hash
	validator_idx := an.(DA_request).ShardIndex
	chunk, err := n.DA_Lookup(hash, uint32(validator_idx))
	if err != nil {
		return err
	}
	err = sendQuicBytes(stream, chunk)
	return err
}

func (p *Peer) DA_Reconstruction(req DA_request) ([]byte, uint16, error) {
	// // handle selfRequest case
	// isSelfRequesting := req.ShardIndex == uint16(p.PeerID)
	// if isSelfRequesting {
	// 	chunk := p.node.chunkMap[req.Hash]
	// 	return chunk, p.PeerID, nil
	// }
	stream, err := p.openStream(CE203_DA_Reconstruction)
	if err != nil {
		return nil, 0, err
	}
	req_bytes, err := types.Encode(req)
	if err != nil {
		return nil, 0, err
	}
	// check if the encode is successful
	err = sendQuicBytes(stream, req_bytes)
	if err != nil {
		return nil, 0, err
	}
	// receive the chunk
	resp, err := receiveQuicBytes(stream)
	if err != nil {
		return nil, 0, err
	}

	return resp, p.PeerID, nil
}

func (n *Node) onDA_Reconstruction(stream quic.Stream, msg []byte) (err error) {
	defer stream.Close()
	req, _, err := types.Decode(msg, reflect.TypeOf(DA_request{}))
	if err != nil {
		return err
	}
	log.Trace(debugDA, "onDA_Reconstruction", "n", n.String(), "h", req.(DA_request).Hash)

	value, ok := n.chunkMap.Load(req.(DA_request).Hash)
	if !ok {
		return fmt.Errorf("hash %v not found in chunkMap", req.(DA_request).Hash)
	}
	chunk := value.([]byte)

	err = sendQuicBytes(stream, chunk)
	return nil
}

func (n *Node) onDA_Announced(stream quic.Stream, msg []byte) (err error) {
	defer stream.Close()
	var newReq DAAnnouncement
	// Deserialize byte array back into the struct
	err = newReq.FromBytes(msg)
	if err != nil {
		fmt.Println("Error deserializing onWorkReportDistribution:", err)
		panic(11114)
		return
	}
	// if n.id == 0 {
	// 	for !common.CompareBytes(newReq.SegmentRoot, n.currentHash[:]) {
	// 		time.Sleep(10 * time.Millisecond)
	// 	}
	// }
	// for !common.CompareBytes(newReq.SegmentRoot, n.currentHash[:]) {
	// 	time.Sleep(10 * time.Millisecond)
	// }
	n.currentHash = common.Hash(newReq.SegmentRoot)
	n.announcement = newReq.State
	return nil
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

// set node n announcement to false
func (n *Node) setAnnouncement(dataHash common.Hash, nodeIndex int, state bool) error {
	da := DAAnnouncement{
		SegmentRoot: dataHash[:],
		State:       state,
	}
	n.dataHashStreamsMu.Lock()
	streams, found := n.dataHashStreams[dataHash]
	if found {
		for _, s := range streams {
			_ = s.Close()
		}
		delete(n.dataHashStreams, dataHash)
	}
	n.dataHashStreamsMu.Unlock()

	// Send message to nodeIndex and request for send new data
	for id, p := range n.peersInfo {
		if id != uint16(nodeIndex) {
			continue
		}
		reqBytes, err := da.ToBytes()
		if err != nil {
			fmt.Printf("Error encoding DAAnnouncement: %v\n", err)
			return err
		}
		stream, err := p.openStream(CE204_DA_Announcemented)
		if err != nil {
			fmt.Printf("Error opening stream: %v\n", err)
			return err
		}
		err = sendQuicBytes(stream, reqBytes)
		if err != nil {
			fmt.Printf("Error sending bytes: %v\n", err)
			return err
		}
	}
	return nil
}

func (p *Peer) SendDistributionECChunks(distributeECChunk DADistributeECChunk) (err error) {
	stream, err := p.openStream(CE201_DA_Announcement)
	if err != nil {
		return err
	}

	// Register stream
	dataHash := common.Hash(distributeECChunk.SegmentRoot)
	p.node.dataHashStreamsMu.Lock()
	p.node.dataHashStreams[dataHash] = append(p.node.dataHashStreams[dataHash], stream)
	p.node.dataHashStreamsMu.Unlock()

	// Close stream when done
	defer func() {
		p.node.dataHashStreamsMu.Lock()
		slice := p.node.dataHashStreams[dataHash]
		for i, s := range slice {
			if s == stream {
				slice = append(slice[:i], slice[i+1:]...)
				break
			}
		}
		p.node.dataHashStreams[dataHash] = slice
		p.node.dataHashStreamsMu.Unlock()

		if closeErr := stream.Close(); closeErr != nil {
			fmt.Printf("SendDistributionECChunks: close stream error: %v\n", closeErr)
		}
	}()

	newReqs := distributeECChunk
	chunkBytes := newReqs.Bytes()

	err = sendQuicBytes(stream, chunkBytes)
	if err != nil {
		return fmt.Errorf("SendDistributionECChunks: sendQuicBytes error: %v", err)
	}
	return nil
}

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
