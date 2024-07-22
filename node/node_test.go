package node

import (
	"fmt"
	//"sync"
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
	for {
	}

}
