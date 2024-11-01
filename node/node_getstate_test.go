package node

import (
	"fmt"
	"testing"
	"time"

	"github.com/colorfulnotion/jam/common"
)

func TestGetState(t *testing.T) {
	genesisConfig, peers, peerList, validatorSecrets, nodePaths, err := SetupQuicNetwork()
	if err != nil {
		t.Fatalf("Error Seeting up nodes: %v\n", err)
	}

	nodes := make([]*Node, numNodes)
	for i := 0; i < numNodes; i++ {
		node, err := newNode(uint16(i), validatorSecrets[i], &genesisConfig, peers, peerList, ValidatorFlag, nodePaths[i], basePort+i)
		if err != nil {
			t.Fatalf("Failed to create node %d: %v\n", i, err)
		}
		//node.state = statedb.ProcessGenesis(genesisAuthorities)
		nodes[i] = node
	}
	// statedb.RunGraph()
	for {
		node := nodes[0]
		var headerHash common.Hash
		if node.statedb.Block != nil {
			fmt.Printf("node.statedb.Block.Header %v\n", node.statedb.Block.Header)
			fmt.Printf("node.statedb.Block.Header.Hash() %v\n", node.statedb.Block.Header.Hash())
			headerHash = node.statedb.Block.Header.Hash()

			node.GetBlockByHeader(headerHash)
		}

		var startKey [31]byte
		var endKey [31]byte
		C12 := common.Hex2Bytes("0x0300000000000000000000000000000000000000000000000000000000000000")
		C13 := common.Hex2Bytes("0x0D00000000000000000000000000000000000000000000000000000000000000")
		copy(startKey[:], C12)

		// use C14
		copy(endKey[:], C13)
		maxSize := uint32(1000000)
		if (headerHash != common.Hash{}) {
			fmt.Printf("GetState----->\n")
			boundarynodes, keyvalues, ok, err := node.GetState(headerHash, startKey, endKey, maxSize)
			fmt.Printf("boundarynodes %x\n", boundarynodes)
			fmt.Printf("keyvalues %x\n", keyvalues)
			if err != nil || ok == false {
				fmt.Printf("Error in GetState %v\n", err)
			}

			boundarynodes, keyvalues, ok, err = node.GetServiceIdxStorage(headerHash, 47, common.Hex2Bytes("cbf888ec0f8ef855fee845e2b986edf94da1b671efa86660dc5ce5be1dee8b05"))
			fmt.Printf("GetServiceIdxStorage----->\n")
			fmt.Printf("boundarynodes %x\n", boundarynodes)
			fmt.Printf("keyvalues %x\n", keyvalues)
			if err != nil || ok == false {
				fmt.Printf("Error in GetState %v\n", err)
			}
		}
		time.Sleep(6 * time.Second)
	}
}

func TestStateRaw(t *testing.T) {
	genesisConfig, peers, peerList, validatorSecrets, nodePaths, err := SetupQuicNetwork()
	if err != nil {
		t.Fatalf("Error Seeting up nodes: %v\n", err)
	}

	nodes := make([]*Node, numNodes)
	for i := 0; i < numNodes; i++ {
		node, err := newNode(uint16(i), validatorSecrets[i], &genesisConfig, peers, peerList, ValidatorFlag, nodePaths[i], basePort+i)
		if err != nil {
			t.Fatalf("Failed to create node %d: %v\n", i, err)
		}
		//node.state = statedb.ProcessGenesis(genesisAuthorities)
		nodes[i] = node
	}
	// statedb.RunGraph()
	for {
		node := nodes[0]
		snapshot := node.statedb.GetJamSnapshot()
		fmt.Println("snapshot:", snapshot)
	}
}
