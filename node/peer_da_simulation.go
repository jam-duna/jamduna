package node

import (
	"fmt"
	"reflect"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
	"github.com/quic-go/quic-go"
)

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

func (p *Peer) DA_Announcement(hash common.Hash, validator_index uint16) error {
	stream, err := p.openStream(CE201_DA_Announcement)
	if err != nil {
		return err
	}
	an := DA_announcement{Hash: hash, PeerId: validator_index}
	an_bytes, err := types.Encode(an)
	if err != nil {
		return err
	}
	err = sendQuicBytes(stream, an_bytes)
	return err
}

func (n *Node) onDA_Announcement(stream quic.Stream, msg []byte) (err error) {
	announcement, _, err := types.Decode(msg, reflect.TypeOf(DA_announcement{}))
	if err != nil {
		return err
	}
	an := announcement.(DA_announcement)
	stream.Close()
	v_idx := n.GetCurrValidatorIndex()
	req := DA_request{
		Hash:       an.Hash,
		ShardIndex: uint16(v_idx),
	}
	fmt.Printf("%s received DA_Announcement for hash %v from v%d\n", n.String(), an.Hash, an.PeerId)
	for i, _ := range n.peersInfo {
		if i == an.PeerId {
			data, err := n.peersInfo[i].DA_Request(req)
			if err != nil {
				return err
			}
			if n.chunkMap == nil {
				n.chunkMap = make(map[common.Hash][]byte)
			}
			fmt.Printf("%s received data for hash %v, data = %v\n", n.String(), an.Hash, data)
			n.chunkMap[an.Hash] = data
		}
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
	fmt.Printf("%s received DA_Reconstruction request for hash %v\n", n.String(), req.(DA_request).Hash)
	chunk := n.chunkMap[req.(DA_request).Hash]
	err = sendQuicBytes(stream, chunk)
	return nil
}

// func (p *Peer) CE138_Request(req CE138_request) ([]byte, uint16, error) {
// 	if req.ShardIndex == uint16(p.PeerID) {
// 		chunk := p.node.chunkMap[req.ErasureRoot]
// 		return chunk, p.PeerID, nil
// 	}
// 	stream, err := p.openStream(CE139_SegmentShardRequest)
// 	if err != nil {
// 		return nil, 0, err
// 	}
// 	req_bytes, err := types.Encode(req)
// 	if err != nil {
// 		return nil, 0, err
// 	}
// 	// check if the encode is successful
// 	err = sendQuicBytes(stream, req_bytes)
// 	if err != nil {
// 		return nil, 0, err
// 	}
// 	// receive the chunk
// 	resp, err := receiveQuicBytes(stream)
// 	if err != nil {
// 		return nil, 0, err
// 	}
// 	return resp, p.PeerID, nil
// }

// ---- test functions ----
func (n *Node) RunDASimulation() error {
	lastvalidator := types.TotalValidators - 1
	var data_hash common.Hash
	if n.id == 0 {
		// simulate the encoding progress
		data := []byte("dummydata")
		data_hash := common.Blake2Hash(data)
		datalen := len(data)
		datas, err := n.encode(data, false, datalen)
		encoded_data := datas[0]
		if err != nil {
			return err
		}
		n.chunkBox = make(map[common.Hash][][]byte)
		n.chunkBox[data_hash] = encoded_data
		fmt.Printf("Encoded data: %v\n", encoded_data)
		for _, peer := range n.peersInfo {
			peer.DA_Announcement(data_hash, 0)
		}
	} else if n.id == uint16(lastvalidator) {
		time.Sleep(5 * time.Second)
		reconstruct_reqs := make([]DA_request, types.TotalValidators)
		for hash := range n.chunkMap {
			data_hash = hash
			break
		}
		for i := range reconstruct_reqs {
			reconstruct_reqs[i].Hash = data_hash
			reconstruct_reqs[i].ShardIndex = uint16(i)
		}
		reqs := make([]interface{}, len(reconstruct_reqs))
		for i, req := range reconstruct_reqs {
			reqs[i] = req
			if reqs[i].(DA_request).Hash != data_hash {
				return fmt.Errorf("Hash mismatch: %v\n", reqs[i].(DA_request).Hash)
			}
		}

		resps, err := n.makeRequests(reqs, types.TotalCores, time.Duration(5)*time.Second, time.Duration(10)*time.Second)
		if err != nil {
			return err
		}
		encoded_data := make([][]byte, types.TotalValidators)
		for _, resp := range resps {
			daResp, ok := resp.(DA_response)
			if !ok {
				return fmt.Errorf("Unexpected response type: %T\n", resp)
			}
			fmt.Printf("DA Response: %v\n", daResp)
			encoded_data[daResp.ShardIndex] = daResp.Data
		}
		encoded_there_dim_data := make([][][]byte, 1)
		encoded_there_dim_data[0] = encoded_data
		decoded_data, err := n.decode(encoded_there_dim_data, false, 12)
		if err != nil {
			return err
		}
		fmt.Printf("Decoded data : %v\n", decoded_data)
	}
	return nil
}
