package node

import (
	"flag"
	"fmt"
	"testing"
	"time"

	"github.com/jam-duna/jamduna/common"
)

func TestClient(t *testing.T) {

	flag.Parse()
	addresses, wsUrl := GetAddresses(true)

	client, err := NewNodeClient(addresses, wsUrl)
	if err != nil {
		t.Fatalf("Error: %s", err)
	}
	defer client.Close()
	err = client.ConnectWebSocket(wsUrl)
	if err != nil {
		fmt.Println("WebSocket connection failed:", err)
		return
	}
	fmt.Printf("Connected to %s\n", wsUrl)

	for {
		time.Sleep(100 * time.Millisecond)
	}
}

func TestCommands(t *testing.T) {
	flag.Parse()
	local := false
	addresses, wsUrl := GetAddresses(local)

	//coreIndex := uint16(0)
	client, err := NewNodeClient(addresses, wsUrl)
	if err != nil {
		t.Fatalf("Error: %s", err)
	}
	defer client.Close()
	// client.BroadcastCommand([]string{"SetFlag", "audit", "true"}, []int{5})
	// client.BroadcastCommand([]string{"SetFlag", "audit", "false"}, []int{5})
	// client.BroadcastCommand([]string{"SetFlag", "ticket_send", "true"}, []int{})
	// client.BroadcastCommand([]string{"SetLog", log.B, "true"}, []int{1, 2, 3, 4, 5})
	// client.SendCommand([]string{"StackTrace"}, 1)
	// client.SendCommand([]string{"GetNodeStatus"}, 3)
	client.SendCommand([]string{"GetConnections"}, 1)

}

type NodeStatus struct {
	NodeID    string
	BlockHash common.Hash
	Slot      uint32
	Timestamp time.Time
	Status    string
}
