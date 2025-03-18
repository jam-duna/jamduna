//go:build network_test
// +build network_test

package node

import (
	"fmt"
	"net/rpc"
	"testing"
	"time"

	"github.com/colorfulnotion/jam/types"
)

func TestArchiveNode(t *testing.T) {
	basePort := uint16(9000)
	bufferTime := 30
	go safroleTest(t, "safrole", SafroleTestEpochLen, basePort, bufferTime)
	epoch0Timestamp, _, peerList, _, _, err := SetupQuicNetwork("", 9000)
	if err != nil {
		t.Fatal(err)
	}
	// Test the archive node
	GenesisStateFile, GenesisBlockFile := GetGenesisFile("tiny")
	seed := make([]byte, 32)
	copy(seed[:], "colorful notion")
	time.Sleep(30 * time.Second)
	archiveNode, err := NewArchiveNode(9999, seed,
		GenesisStateFile, GenesisBlockFile, epoch0Timestamp, peerList, "/tmp/archive_node_test", 13000, 13500)
	if err != nil {
		t.Fatal(err)
	} else {
		fmt.Println("ArchiveNode created successfully")
	}
	graph_server := types.NewGraphServer(3030)
	go graph_server.StartServer()

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-time.After(30 * time.Second):
			return
		case <-ticker.C:
			graph_server.Update(archiveNode.block_tree)
		}
	}

}

func TestRPCGetBlockBySlot(t *testing.T) {
	// Connect to RPC server
	client, err := rpc.Dial("unix", "/tmp/jam_rpc.sock")
	if err != nil {
		t.Fatalf("❌ Failed to connect to RPC server: %v", err)
	}
	defer client.Close()

	// Test GetBlockByHash
	var result string
	err = client.Call("jam.GetBlockBySlot", []string{"12"}, &result)
	if err != nil {
		t.Fatalf("❌ RPC Call Failed: %v", err)
	}

	// Verify response (you can adjust this based on expected output)
	if result == "" {
		t.Fatalf("❌ Expected a block but got an empty result")
	}

	t.Logf("✅ GetBlockByHash Result: %s", result)
}
