package node

import (
	"fmt"
	"sync"
	"testing"
)

func TestNodes(t *testing.T) {
	peers := make([]string, numNodes)
	for i := 0; i < numNodes; i++ {
		peers[i] = fmt.Sprintf(quicAddr, 9000+i)
	}

	nodes := make([]*Node, numNodes)
	for i := 0; i < numNodes; i++ {
		node, err := newNode(i, peers)
		if err != nil {
			t.Fatalf("Failed to create node %d: %v", i, err)
		}
		nodes[i] = node
	}

	var wg sync.WaitGroup
	for _, node := range nodes {
		wg.Add(1)
		go func(n *Node) {
			defer wg.Done()
			n.runClient()
		}(node)
	}
	wg.Wait()
}
