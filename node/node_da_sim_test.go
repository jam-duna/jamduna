//go:build network
// +build network

package node

import (
	"fmt"
	"testing"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

func TestDASimulation(t *testing.T) {
	//
	// Set up nodes
	nodes, err := SetUpNodes(numNodes)
	if err != nil {
		t.Fatalf("Failed to set up nodes: %v", err)
	}

	// simulate the encoding progress
	data := []byte("dummydata")
	data_hash := common.Blake2Hash(data)
	datalen := len(data)
	datas, err := nodes[0].encode(data, false, datalen)
	encoded_data := datas[0]
	if err != nil {
		t.Fatalf("Error encoding data: %v\n", err)
	}
	nodes[0].chunkBox = make(map[common.Hash][][]byte)
	nodes[0].chunkBox[data_hash] = encoded_data
	fmt.Printf("Encoded data: %v\n", encoded_data)
	for _, peer := range nodes[0].peersInfo {
		peer.DA_Announcement(data_hash, 0)
	}
	lastvalidator := types.TotalValidators - 1
	time.Sleep(1 * time.Second)
	// simulate the decoding progress
	reconstruct_reqs := make([]DA_request, types.TotalValidators)
	for i := range reconstruct_reqs {
		reconstruct_reqs[i].Hash = data_hash
		reconstruct_reqs[i].ShardIndex = uint16(i)
	}
	reqs := make([]interface{}, len(reconstruct_reqs))
	for i, req := range reconstruct_reqs {
		reqs[i] = req
		if reqs[i].(DA_request).Hash != data_hash {
			t.Fatalf("Hash mismatch: %v\n", reqs[i].(DA_request).Hash)
		}
	}

	resps, err := nodes[lastvalidator].makeRequests(reqs, types.TotalCores, time.Duration(5)*time.Second, time.Duration(10)*time.Second)
	if err != nil {
		t.Fatalf("Error making requests: %v\n", err)
	}
	encoded_data = make([][]byte, types.TotalValidators)
	for _, resp := range resps {
		daResp, ok := resp.(DA_response)
		if !ok {
			t.Fatalf("Unexpected response type: %T\n", resp)
		}
		if debugDA {
			fmt.Printf("DA Response: %x\n", daResp)
		}
		encoded_data[daResp.ShardIndex] = daResp.Data
	}
	encoded_there_dim_data := make([][][]byte, 1)
	encoded_there_dim_data[0] = encoded_data
	decoded_data, err := nodes[lastvalidator].decode(encoded_there_dim_data, false, datalen)
	if err != nil {
		t.Fatalf("Error decoding data: %v\n", err)
	}
	fmt.Printf("Decoded data : %v\n", decoded_data)
	fmt.Printf("Original data: %v\n", data)
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
