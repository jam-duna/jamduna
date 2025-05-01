package node

import (
	"encoding/json"
	"flag"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/colorfulnotion/jam/common"
	"github.com/gorilla/websocket"
)

var mode = flag.String("mode", "fib", "Mode to run the test client")
var isLocal = flag.Bool("isLocal", false, "Run local version")

func TestClient(t *testing.T) {

	flag.Parse()
	// testMode := *mode
	local := *isLocal

	addresses, wsUrl := GetAddresses(local)
	//coreIndex := uint16(0)

	client, err := NewNodeClient(addresses, wsUrl[3])
	if err != nil {
		t.Fatalf("Error: %s", err)
	}
	defer client.Close()

	err = client.ConnectWebSocket(wsUrl[3])
	if err != nil {
		fmt.Println("WebSocket connection failed:", err)
		return
	}

	state, err := client.GetState("latest")
	fmt.Printf(state.String())

	// switch testMode {
	// default:
	// 	t.Fatalf("Invalid mode: %s", testMode)
	// }
}

func TestCommands(t *testing.T) {
	flag.Parse()
	local := false
	addresses, wsUrl := GetAddresses(local)

	//coreIndex := uint16(0)
	client, err := NewNodeClient(addresses, wsUrl[0])
	if err != nil {
		t.Fatalf("Error: %s", err)
	}
	defer client.Close()
	// client.BroadcastCommand([]string{"SetFlag", "audit", "true"}, []int{5})
	// client.BroadcastCommand([]string{"SetFlag", "audit", "false"}, []int{5})
	// client.BroadcastCommand([]string{"SetFlag", "ticket_send", "true"}, []int{})
	// client.BroadcastCommand([]string{"SetLog", log.GeneralAuthoring, "true"}, []int{})
	// client.SendCommand([]string{"StackTrace"}, 1)
	client.SendCommand([]string{"GetNodeStatus"}, 3)

}

func TestLatestBlockDEMO(t *testing.T) {
	local := false
	_, wsUrls := GetAddresses(local)

	updates := make(chan NodeStatus, 100)

	for i, url := range wsUrls {
		nodeID := fmt.Sprintf("node-%d", i)
		go connectAndListen(url, nodeID, updates)
	}

	m := model{nodes: initNodes()}
	p := tea.NewProgram(m)

	go func() {
		for update := range updates {
			p.Send(nodeUpdateMsg(update))
		}
	}()

	if _, err := p.Run(); err != nil {
		t.Fatalf("Bubbletea UI failed: %v", err)
	}
}

type NodeStatus struct {
	NodeID    string
	BlockHash common.Hash
	Slot      uint32
	Timestamp time.Time
	Status    string
}

type model struct {
	nodes map[string]NodeStatus
}

// Init implements tea.Model.
func (m model) Init() tea.Cmd {
	return nil
}

func initNodes() map[string]NodeStatus {
	nodes := make(map[string]NodeStatus)
	for i := 0; i <= 5; i++ {
		id := fmt.Sprintf("node-%d", i)
		nodes[id] = NodeStatus{
			NodeID:    id,
			BlockHash: common.Hash{},
			Slot:      0,
			Timestamp: time.Now(),
		}
	}
	return nodes
}

type nodeUpdateMsg NodeStatus

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}

	case nodeUpdateMsg:
		m.nodes[msg.NodeID] = NodeStatus(msg)
		return m, nil
	}

	return m, nil
}

func (m model) View() string {
	var b strings.Builder
	b.WriteString("\nBest Blocks from Nodes\n\n")
	b.WriteString(fmt.Sprintf("%-10s %-16s %-24s %-10s\n", "NodeID", "BlockHash", "Timestamp", "Status"))
	b.WriteString(strings.Repeat("-", 70) + "\n")

	nodeIDs := []string{}
	for id := range m.nodes {
		nodeIDs = append(nodeIDs, id)
	}
	sort.Strings(nodeIDs)

	for _, id := range nodeIDs {
		node := m.nodes[id]
		hash := node.BlockHash.String_short()
		timestamp := node.Timestamp.Format("2006-01-02 15:04:05.000")

		b.WriteString(fmt.Sprintf("%-10s %-16s %-24s %-10s\n",
			node.NodeID,
			hash,
			timestamp,
			node.Status,
		))
	}

	b.WriteString("\nPress q to exit.\n")
	return b.String()
}
func connectAndListen(url string, nodeID string, updates chan<- NodeStatus) {
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		fmt.Printf("WebSocket connect error (%s): %v\n", nodeID, err)
		return
	}
	defer conn.Close()

	subMsg := map[string]interface{}{
		"method": "subscribeBestBlock",
		"params": map[string]interface{}{
			"finalized": false,
		},
	}
	conn.WriteJSON(subMsg)

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			time.Sleep(10 * time.Second)
			conn, _, err := websocket.DefaultDialer.Dial(url, nil)
			if err != nil {
				updates <- NodeStatus{
					NodeID:    nodeID,
					BlockHash: common.Hash{},
					Timestamp: time.Now(),
					Status:    "disconnected",
				}
				continue
			}
			defer conn.Close()
			conn.WriteJSON(subMsg)
			continue
		}

		var envelope struct {
			Method string          `json:"method"`
			Result json.RawMessage `json:"result"`
		}

		if err := json.Unmarshal(msg, &envelope); err != nil {
			fmt.Printf("Invalid envelope (%s): %s\n", nodeID, string(msg))
			continue
		}

		if envelope.Method != "subscribeBestBlock" && envelope.Method != SubBestBlock {
			continue
		}

		var result struct {
			BlockHash  string `json:"blockHash"`
			HeaderHash string `json:"headerHash"`
		}

		if err := json.Unmarshal(envelope.Result, &result); err != nil {
			fmt.Printf("Parse block failed (%s): %v\n", nodeID, err)
			continue
		}

		updates <- NodeStatus{
			NodeID:    nodeID,
			BlockHash: common.Hex2Hash(result.HeaderHash),
			Timestamp: time.Now(),
			Status:    "connected",
		}
	}
}
