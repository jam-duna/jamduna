package node

import (
	"fmt"
	//"sync"
	"encoding/json"
	"github.com/colorfulnotion/jam/safrole"
	"testing"
)

func TestNodes(t *testing.T) {
	seeds, _ := generateSeedSet(numNodes)
	fmt.Printf("seeds %x\n", seeds)

	peers := make([]string, numNodes)
	peerList := make(map[string]NodeInfo)

	validators := make([]safrole.Validator, numNodes)
	for i := 0; i < numNodes; i++ {
		validator, err := safrole.InitValidator(seeds[i], seeds[i])
		if err == nil {
			validators[i] = validator
		} else {
			t.Fatalf("Failed to init validator %d: %v", i, err)
		}
	}

	genesisConfig := safrole.NewGenesisConfig(validators)

	prettyJSON, _ := json.MarshalIndent(validators, "", "  ")
	fmt.Printf("Validators (size:%v) %s\n", numNodes, prettyJSON)

	for i := uint32(0); i < numNodes; i++ {
		addr := fmt.Sprintf(quicAddr, 9000+i)
		peers[i] = addr
		ed25519Key := validators[i].Ed25519.String()
		peerList[ed25519Key] = NodeInfo{
			PeerID:    i,
			PeerAddr:  addr,
			Validator: validators[i],
		}
	}

	// Print out peerList
	prettyPeerList, err := json.MarshalIndent(peerList, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal peerList: %v", err)
	}
	fmt.Printf("PeerList: %s\n", prettyPeerList)

	nodes := make([]*Node, numNodes)
	for i := uint32(0); i < numNodes; i++ {
		validatorSecret, err := safrole.InitValidatorSecret(seeds[i], seeds[i])
		if err != nil {
			t.Fatalf("Failed to init node %d: with secret %v", i, err)
		}
		node, err := newNode(i, validatorSecret, &genesisConfig, peers, peerList)
		if err != nil {
			t.Fatalf("Failed to create node %d: %v", i, err)
		}
		//node.state = safrole.ProcessGenesis(genesisAuthorities)
		nodes[i] = node
	}

	for {
		// Additional test logic or a simple blocking mechanism to keep the test running
	}
}
