package node

import (
	"fmt"
	"testing"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

func TestNodeAudit(t *testing.T) {
	genesisConfig, peers, peerList, validatorSecrets, nodePaths, err := SetupQuicNetwork()
	if err != nil {
		t.Fatalf("Error setting up nodes: %v\n", err)
	}

	nodes := make([]*Node, numNodes)
	for i := 0; i < numNodes; i++ {
		node, err := newNode(uint32(i), validatorSecrets[i], &genesisConfig, peers, peerList, DAFlag, nodePaths[i])
		if err != nil {
			t.Fatalf("Failed to create node %d: %v\n", i, err)
		}
		nodes[i] = node
	}
	// Wait for nodes to be ready
	fmt.Println("Waiting for nodes to be ready...")
	time.Sleep(1 * time.Second)
	fmt.Println("===Audit Start===")
	for _, n := range nodes {
		if n.statedb == nil {
			fmt.Printf("Node[%d] state is nil\n", n.id)
		}
		fmt.Printf("Node[%d] add dummy Rhostate\n", n.id)
		n.AddDummyStateAudit()
		fmt.Printf("Node[%d] state is ok\n", n.id)

	}
	for _, n := range nodes {
		err := n.Tiny_Audit()
		if err != nil {
			t.Fatalf("tiny audit error: %v\n", err.Error())
		}
	}

	//delay for a while
	time.Sleep(1 * time.Second)
	fmt.Println("===Done Audit, Start to make dispute===")
	nodes[0].AddDummyBlockAudit()
	//generate dispute
	err = nodes[0].MakeDisputes()
	if err != nil {
		t.Fatalf("Failed to generate dispute: %v\n", err)
	}
	fmt.Println("Dispute generated successfully")
	nodes[0].statedb.Block.Extrinsic.Disputes.Print()

}
func TestNodeAssurance(t *testing.T) {
	genesisConfig, peers, peerList, validatorSecrets, nodePaths, err := SetupQuicNetwork()
	if err != nil {
		t.Fatalf("Error setting up nodes: %v\n", err)
	}

	nodes := make([]*Node, numNodes)
	for i := 0; i < numNodes; i++ {
		node, err := newNode(uint32(i), validatorSecrets[i], &genesisConfig, peers, peerList, DAFlag, nodePaths[i])
		if err != nil {
			t.Fatalf("Failed to create node %d: %v\n", i, err)
		}
		nodes[i] = node
	}
	// Wait for nodes to be ready
	fmt.Println("Waiting for nodes to be ready...")
	time.Sleep(1 * time.Second)
	for _, n := range nodes {
		if n.statedb == nil {
			fmt.Printf("Node[%d] state is nil\n", n.id)
		}
		n.AddDummyStateAssurance()
	}
	//generate assurance
	fmt.Println("Generating assurance...")
	for _, n := range nodes {
		assurance, err := n.GenerateAssurance()
		if err != nil {
			t.Fatalf("Failed to generate assurance for node %d: %v", n.id, err)
		}
		fmt.Printf("Node[%d] Anchor: %v\n", n.id, assurance.Anchor)
		fmt.Printf("Node[%d] BitField: %08b\n", n.id, assurance.Bitfield)
		fmt.Printf("Node[%d] Signature: %x\n", n.id, assurance.Signature)
		fmt.Printf("Node[%d] ValidatorIndex: %d\n", n.id, assurance.ValidatorIndex)
	}
}

// helper function
func (n *Node) AddDummyStateAssurance() error {
	n.statedb.JamState.AvailabilityAssignments = [types.TotalCores]*statedb.Rho_state{}
	n.UpdateAssurancesBucket([]types.WorkReport{})
	for i := 0; i < 2; i++ {
		fmt.Printf("Node[%d] add dummy Rhostate\n", n.id)
		n.statedb.JamState.AvailabilityAssignments[i] = &statedb.Rho_state{
			WorkReport: types.WorkReport{
				AvailabilitySpec: types.AvailabilitySpecifier{
					WorkPackageHash: common.BytesToHash(append([]byte("test"), byte(i))),
				},
				CoreIndex: uint16(i),
			},
		}
		//dummy parent hash
		fmt.Printf("Node[%d] add dummy ParentHash\n", n.id)
		n.statedb.Block = &types.Block{
			Header: types.BlockHeader{
				Parent: common.BytesToHash(append([]byte("test"), byte(i))),
			},
		}
		fmt.Printf("Node[%d] add dummy IsPackageRecieved\n", n.id)
		n.assurancesBucket[common.BytesToHash(append([]byte("test"), byte(i)))] = types.IsPackageRecieved{
			ExportedSegments: true,
			WorkReportBundle: true,
		}
	}
	return nil
}

// helper function
func (n *Node) AddDummyStateAudit() {
	n.statedb.JamState.AvailabilityAssignments = [types.TotalCores]*statedb.Rho_state{
		&statedb.Rho_state{
			WorkReport: types.WorkReport{
				AvailabilitySpec: types.AvailabilitySpecifier{
					WorkPackageHash: common.BytesToHash([]byte("test123")),
				},
				CoreIndex: uint16(0),
			},
		},
		&statedb.Rho_state{
			WorkReport: types.WorkReport{
				AvailabilitySpec: types.AvailabilitySpecifier{
					WorkPackageHash: common.BytesToHash([]byte("test456")),
				},
				CoreIndex: uint16(1),
			},
		},
	}
	fmt.Printf("Node[%d] add dummy AvailableWorkReport\n", n.id)
	n.statedb.AvailableWorkReport = make([]types.WorkReport, 0)
	n.statedb.AvailableWorkReport = append(n.statedb.AvailableWorkReport, n.statedb.JamState.AvailabilityAssignments[0].WorkReport)
	n.statedb.AvailableWorkReport = append(n.statedb.AvailableWorkReport, n.statedb.JamState.AvailabilityAssignments[1].WorkReport)
}

func (n *Node) AddDummyBlockAudit() {
	n.statedb.Block = &types.Block{
		Header: types.BlockHeader{
			Parent: common.BytesToHash([]byte("test")),
		},
		Extrinsic: types.ExtrinsicData{
			Disputes: types.Dispute{
				Verdict: []types.Verdict{},
				Culprit: []types.Culprit{},
				Fault:   []types.Fault{},
			},
		},
	}
}
