package node

import (
	"flag"
	"fmt"
	"testing"
)

var mode = flag.String("mode", "fib", "Mode to run the test client")
var isLocal = flag.Bool("isLocal", false, "Run local version")

func TestClient(t *testing.T) {

	flag.Parse()
	testMode := *mode
	local := *isLocal

	addresses, wsUrl := GetAddresses(local)
	//coreIndex := uint16(0)

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

	client.Subscribe("subscribeBestBlock", map[string]interface{}{"finalized": false})

	switch testMode {
	default:
		t.Fatalf("Invalid mode: %s", testMode)
	}
}

func TestCommands(t *testing.T) {
	flag.Parse()
	local := true
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
	// client.BroadcastCommand([]string{"SetLog", log.GeneralAuthoring, "true"}, []int{})
	client.SendCommand([]string{"StackTrace"}, 1)
}
