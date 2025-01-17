package node

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"reflect"
	"runtime/pprof"
	"sync/atomic"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/erasurecoding"
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
	if debugDA {
		fmt.Printf("Node %d Get newReq[%d].Data: %v\n", n.id, n.id, newReq.Data)
	}
	n.chunkMap.Store(common.Hash(newReq.SegmentRoot), newReq.Data)

	if n.id == types.TotalValidators-1 {
		n.currentHash = common.BytesToHash(newReq.SegmentRoot)
		n.announcement = true

	}
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
	if debugDA {
		fmt.Printf("%s received DA_Reconstruction request for hash %v\n", n.String(), req.(DA_request).Hash)
	}
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
	n.announcement = false
	return nil
}

func getRandomSize() int {
	// sizes := []int{2, 128, 1024, 10240, 1048576} // {2Byte, 128Bytes, 1KBytes, 10KBytes, 1MBytes}
	sizes := []int{129, 1025, 4105, 10241, 66666, 129999, 258000, 520000, 1048577}
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

func (n *Node) RunDASimulation(pprofFile *os.File, pprofTime time.Duration) error {
	// n.chunkMap = make(map[common.Hash][]byte)
	lastValidator := types.TotalValidators - 1
	n.announcement = false
	time1 := time.Now()
	time3 := time.Now()
	closedPProf := false
	n.currentHash = common.Hash{}
	var dataHash common.Hash
	dataHash = common.Hash{}
	currentTotalIncomingStreams := int64(0)
	for {
		if debugDA {
			if n.id == 0 || n.id == uint16(lastValidator) {
				fmt.Printf("Node %d closed: %v n.curre: %v dataHash: %v n.announ: %v\n", n.id, closedPProf, n.currentHash, dataHash, n.announcement)
			}
		}
		if n.id == 0 || n.id == uint16(lastValidator) {
			if debugDA {
				time4 := time.Now()
				if time4.Sub(time3) > time.Second {
					fmt.Printf("Node %d n.announcement: %v\n", n.id, n.announcement)
					time3 = time4
				}
			}
		}
		if atomic.LoadInt64(&n.totalIncomingStreams) > 10 {
			if currentTotalIncomingStreams != atomic.LoadInt64(&n.totalIncomingStreams) {
				fmt.Printf("Current Node %d total totalIncomingStreams: %d\n", n.id, atomic.LoadInt64(&n.totalIncomingStreams))
				currentTotalIncomingStreams = atomic.LoadInt64(&n.totalIncomingStreams)
			}
		}
		// Main logic to simulate DA
		if n.id == 0 && !n.announcement {
			if debugDADist {
				fmt.Printf("1X->")
			}
			currentTime := time.Now()
			hour := currentTime.Hour()
			minute := currentTime.Minute()
			second := currentTime.Second()
			millisecond := currentTime.Nanosecond() / int(time.Millisecond)

			fmt.Printf("Node %d start time %02d:%02d:%02d.%03d\n", n.id, hour, minute, second, millisecond)
			size := getRandomSize()
			numpieces := size / 2 // One chunk have N pieces and each piece is 2 bytes
			/*
			 Erasure coding have 1/3 data shard and generate 2/3 parity shard
			 So, we need to generate 1/3 data shard
			*/
			generateSize := size * types.TotalValidators / 3
			b := generateRandomData(generateSize)
			dataHash := common.Blake2Hash(b)
			if debugDA {
				fmt.Printf("Len(b)%d, size %d, numpieces %d, generateSize %d\n", len(b), size, numpieces, generateSize)
			}
			timeA := time.Now()
			chunks, err := erasurecoding.Encode(b, numpieces)
			timeB := time.Now()
			if err != nil {
				fmt.Printf("Error encoding data: %v\n", err)
				panic(5)
			}
			if debugDA {
				fmt.Printf("Node %d generated data chunk[%d][%d][%d]\n", n.id, len(chunks), len(chunks[0]), len(chunks[0][0]))
			}
			if debugDADist {
				fmt.Printf("2X->")
			}
			bECChunks := make([]DADistributeECChunk, len(chunks[0]))

			for j := range chunks[0] {
				bECChunks[j] = DADistributeECChunk{
					SegmentRoot: dataHash[:],
					Data:        chunks[0][j],
				}
			}

			if debugDADist {
				fmt.Printf("3X->")
			}
			if debugDA {
				for j, chunk := range bECChunks {
					fmt.Printf("bECChunks[%d].SegmentRoot: %x\n", j, chunk.SegmentRoot)
				}
			}

			n.chunkBox = make(map[common.Hash][][]byte)
			n.chunkBox[dataHash] = [][]byte{b}

			if debugDADist {
				fmt.Printf("4X->")
			}
			if debugDA {
				fmt.Printf("WorkPackageHash: %x\n", dataHash)
				fmt.Printf("[N%d] Encoded data: %x\n", n.id, b)
			}

			ecChunks := make([][][]byte, 1)
			ecChunks[0] = make([][]byte, len(bECChunks))

			for j, chunk := range bECChunks {
				ecChunks[0][j] = chunk.Data
			}

			if debugDA {
				decode, err := n.decode(ecChunks, true, 6)
				if err != nil {
					fmt.Printf("ecChunks: %x\n", ecChunks)
					fmt.Printf("dataHash: %v\n", dataHash)
					fmt.Printf("Error decoding data: %v\n", err)
				}
				decodeHash := common.Blake2Hash(decode)
				bHash := common.Blake2Hash(b)
				if !common.CompareBytes(decodeHash[:], bHash[:]) {
					if debugDA {
						fmt.Printf("[N%d] Decoded data: %x\noriginal b: %x\n", n.id, decode, b)
					}
					fmt.Printf("Decoded data is not equal to original data\n")
					fmt.Printf("Node %d Decoded data is not equal to original data, when data length = %d\n", n.id, len(b))
				}
			}
			if debugDADist {
				fmt.Printf("5X->")
			}
			n.announcement = true
			n.broadcast(bECChunks)
			if debugDADist {
				fmt.Printf("6X->")
			}
			if debugDADist {
				fmt.Printf("7X\n")
			}

			timeC := time.Now()
			encTime := timeB.Sub(timeA).Milliseconds()
			distTime := timeC.Sub(timeB).Milliseconds()
			fmt.Printf("Node %d broadcasted the hash\t%x...%x, len(data) %d, enc=%dms dist=%dms\n", n.id, dataHash[:4], dataHash[len(dataHash)-4:], len(b), encTime, distTime)
		} else if n.id == uint16(lastValidator) {
			reconstructReqs := make([]DA_request, types.TotalValidators)
			if len(n.currentHash) == 0 || !n.announcement {
				continue
			} else {
				dataHash = n.currentHash
			}
			if debugDARecon {
				fmt.Printf("1->")
			}
			if debugDA {
				fmt.Printf("[N%d] Try to get data_hash: %x\n", n.id, dataHash)
			}
			timeX := time.Now()
			for j := range reconstructReqs {
				reconstructReqs[j].Hash = dataHash
				reconstructReqs[j].ShardIndex = uint16(j)
			}

			if debugDARecon {
				fmt.Printf("2->")
			}
			reqs := make([]interface{}, len(reconstructReqs))
			for j, req := range reconstructReqs {
				reqs[j] = req
				if reqs[j].(DA_request).Hash != dataHash {
					fmt.Printf("Hash mismatch: %x\n", reqs[j].(DA_request).Hash)
					continue
				}
			}

			if debugDARecon {
				fmt.Printf("3->")
			}
			resps, err := n.makeRequests(reqs, types.TotalValidators/3, time.Duration(20)*time.Second, time.Duration(50)*time.Second)
			if err != nil {
				fmt.Printf("Error making requests: %v\n", err)
				continue
			}

			timeY := time.Now()
			encodedData := make([][]byte, types.TotalValidators)
			for _, resp := range resps {
				daResp, ok := resp.(DA_response)
				if !ok {
					fmt.Printf("Unexpected response type: %T\n", resp)
					continue
				}

				if debugDA {
					fmt.Printf("[N%d] Get response from node[%d] Data = %x\n", n.id, daResp.ShardIndex, daResp.Data)
				}
				encodedData[daResp.ShardIndex] = daResp.Data
			}

			if debugDARecon {
				fmt.Printf("4->")
			}
			encodedThreeDimData := make([][][]byte, 1)
			encodedThreeDimData[0] = encodedData
			dataLength := 0
			for _, data := range encodedData {
				if len(data) > 0 {
					dataLength = len(data)
					break
				}
			}
			if debugDARecon {
				fmt.Printf("5->")
			}
			decodedData, err := erasurecoding.Decode(encodedThreeDimData, dataLength/2)
			if err != nil {
				if debugDA {
					fmt.Printf("Error decoding data: %v\n", err)
				}
				continue
			}
			if debugDARecon {
				fmt.Printf("6->")
			}

			timeZ := time.Now()
			decodeDataHash := common.Blake2Hash(decodedData)
			var da DAAnnouncement
			da.SegmentRoot = dataHash[:]
			if debugDARecon {
				fmt.Printf("7->")
			}

			// Reset the setting
			n.announcement = false
			err = n.resetAnnouncement(da, 0)
			if err != nil {
				continue
			}
			if debugDARecon {
				fmt.Printf("8\n")
			}
			reqTime := timeY.Sub(timeX).Milliseconds()
			decTime := timeZ.Sub(timeY).Milliseconds()
			fmt.Printf("Node %d decoded data hash\t%x...%x, len(data) %d, req=%dms dec=%dms\n", n.id, decodeDataHash[:4], decodeDataHash[len(decodeDataHash)-4:], len(decodedData), reqTime, decTime)

			// Print the end time
			currentTime := time.Now()
			hour := currentTime.Hour()
			minute := currentTime.Minute()
			second := currentTime.Second()
			millisecond := currentTime.Nanosecond() / int(time.Millisecond)

			fmt.Printf("Node %d end time %02d:%02d:%02d.%03d\n", n.id, hour, minute, second, millisecond)

			// Check if the decoded data is equal to the original data
			if !common.CompareBytes(decodeDataHash[:], dataHash[:]) {
				if debugDA {
					fmt.Printf("[N%d] Decoded data:  %x\n", n.id, decodedData)
				}
				fmt.Printf("Node %d Decoded data is not equal to original data, when data length = %d\n", n.id, len(decodedData))
			}
		}
		if n.id == uint16(lastValidator) && n.announcement {
			var da DAAnnouncement
			da.SegmentRoot = dataHash[:]
			n.resetAnnouncement(da, 0)
			fmt.Printf("Reset Node 0 announcement sccessfully\n")
		}
		if debugDAPProf {
			if pprofFile != nil && !closedPProf {
				time2 := time.Now()
				if time2.Sub(time1) > pprofTime {
					pprof.StopCPUProfile()
					pprofFile.Close()
					closedPProf = true
					// fmt.Printf("Node %d Port %d Stopped pprof profiling.\n", n.id, n.id+9000)
				}
			}
		}
	}
	return nil
}

// set node n announcement to false
func (n *Node) resetAnnouncement(da DAAnnouncement, nodeIndex int) error {
	dataHash := common.BytesToHash(da.SegmentRoot)

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
