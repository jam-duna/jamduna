package node

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

func UpdateJCESignalSimple(nodes []*Node, initialValue uint32) {
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()

	universalJCE := initialValue
	updateInterval := 6 * time.Second
	lastUpdate := time.Now()

	for {
		<-ticker.C
		if time.Since(lastUpdate) >= updateInterval {
			universalJCE++
			for _, node := range nodes {
				node.SendNewJCE(universalJCE)
			}
			lastUpdate = time.Now()
		}
	}
}

func UpdateJCESignalUniversal(nodes []*Node, initialValue uint32) {
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()

	universalJCE := initialValue

	// Initially, assume all nodes are pending (not yet ready).
	pendingNodes := make([]*Node, len(nodes))
	copy(pendingNodes, nodes)

	for {
		<-ticker.C

		// Check only nodes that haven't completed the current universalJCE.
		newPending := pendingNodes[:0] // reuse underlying array
		for _, node := range pendingNodes {
			if node.GetCompletedJCE() < universalJCE {
				//fmt.Printf("Node %d is NOT ready: completedJCE = %d, expected universalJCE = %d\n", node.id, node.GetCompletedJCE(), universalJCE)
				newPending = append(newPending, node)
			} else {
				//fmt.Printf("Node %d is ready: completedJCE = %d, universalJCE = %d\n", node.id, node.GetCompletedJCE(), universalJCE)
			}
		}
		pendingNodes = newPending

		if len(pendingNodes) > 0 {
			//fmt.Printf("Waiting for %d node(s) to complete JCE %d\n", len(pendingNodes), universalJCE)
		}

		// If all nodes are ready, update universal JCE and push it to every node.
		if len(pendingNodes) == 0 {
			universalJCE++
			fmt.Printf("Deploying new universal JCE: %d\n", universalJCE)
			for _, node := range nodes {
				node.SendNewJCE(universalJCE)
			}
			// Reset pendingNodes to include all nodes for the next round.
			pendingNodes = make([]*Node, len(nodes))
			copy(pendingNodes, nodes)
		}
	}
}

func StartGameOfLifeServer(addr string, path string) func(data []byte) {
	//addr := "localhost:8080"
	//path := "../rpc_client/game_of_life.html"
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	var wsConn *websocket.Conn

	go func() {
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, path)
		})
		http.HandleFunc("/wsgof_rpc", func(w http.ResponseWriter, r *http.Request) {
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				fmt.Println("upgrade error:", err)
				return
			}
			wsConn = conn
			fmt.Println("Client connected")
		})
		go http.ListenAndServe(addr, nil)
		fmt.Printf("Starting Game of Life on %v\n", addr)
	}()

	return func(data []byte) {
		if wsConn != nil {
			fmt.Printf("Sending data to WebSocket client len(%d)\n", len(data))
			wsConn.WriteMessage(websocket.BinaryMessage, data)
		} else {
			fmt.Println("WebSocket connection not established")
		}
	}
}
