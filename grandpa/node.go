package grandpa

import (
	"math/rand"
	"sync"

	"github.com/colorfulnotion/jam/bls"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

type MockGrandpaNode struct {
	grandpa   *GrandpaManager
	blockTree *types.BlockTree
	selfKey   types.ValidatorSecret
	allNodes  []*MockGrandpaNode

	mutex sync.Mutex
}

func (m *MockGrandpaNode) FinalizedBlockHeader(headerHash common.Hash) {
	// For testing purposes, we can just log the finalized block header
	log.Info("G", "Node finalized block header: %s", headerHash.Hex())
}

func (m *MockGrandpaNode) FinalizedEpoch(epoch uint32, beefyHash common.Hash, aggregatedSignature bls.Signature) {
	// For testing purposes, we can just log the finalized epoch
	log.Info("G", "Mock node finalized epoch", "epoch", epoch, "beefyHash", beefyHash.Hex())
}

func (m *MockGrandpaNode) Broadcast(msg interface{}, evID ...uint64) {
	m.mutex.Lock()
	nodes := m.allNodes
	m.mutex.Unlock()

	switch v := msg.(type) {
	case GrandpaVote: // CE149
		stage := v.SignedMessage.Message.Stage
		for _, n := range nodes {
			if n == m {
				continue
			}
			switch stage {
			case PrevoteStage:
				select {
				case n.grandpa.PreVoteMessageCh <- v:
				default:
					log.Warn("G", "Broadcast: PreVoteMessageCh full, dropping vote")
				}
			case PrecommitStage:
				select {
				case n.grandpa.PreCommitMessageCh <- v:
				default:
					log.Warn("G", "Broadcast: PreCommitMessageCh full, dropping vote")
				}
			case PrimaryProposeStage:
				select {
				case n.grandpa.PrimaryMessageCh <- v:
				default:
					log.Warn("G", "Broadcast: PrimaryMessageCh full, dropping vote")
				}
			}
		}
	case GrandpaCommitMessage: // CE150
		for _, n := range nodes {
			if n == m {
				continue
			}
			select {
			case n.grandpa.CommitMessageCh <- v:
			default:
				log.Warn("G", "Broadcast: CommitMessageCh full, dropping commit")
			}
		}
	case GrandpaStateMessage: // CE151
		for _, n := range nodes {
			if n == m {
				continue
			}
			select {
			case n.grandpa.StateMessageCh <- v:
			default:
				log.Warn("G", "Broadcast: StateMessageCh full, dropping state message")
			}
		}
	case GrandpaCatchUp: // CE152
		for _, n := range nodes {
			if n == m {
				continue
			}
			select {
			case n.grandpa.CatchUpMessageCh <- v:
			default:
				log.Warn("G", "Broadcast: CatchUpMessageCh full, dropping catch-up message")
			}
		}
	case uint32: // CE 153 Warp Sync Request
		for _, n := range nodes {
			if n == m {
				continue
			}
			select {
			case n.grandpa.WarpSyncRequestCh <- v:
			default:
				log.Warn("G", "Broadcast: WarpSyncRequestCh full, dropping warp sync request")
			}
		}
	}
}

func SetupGrandpaNodes(numNodes int, genesis_blk types.Block) []*MockGrandpaNode {
	validators, secrets, err := statedb.GenerateValidatorSecretSet(numNodes)
	if err != nil {
		panic(err)
	}
	mockNodes := make([]*MockGrandpaNode, numNodes)
	for i := 0; i < numNodes; i++ {
		block_tree := types.NewBlockTree(&types.BT_Node{
			Parent:    nil,
			Block:     genesis_blk.Copy(),
			Height:    0,
			Finalized: true,
		})
		mockNode := &MockGrandpaNode{}
		mockNode.blockTree = block_tree
		mockNode.selfKey = secrets[i]
		mockNode.grandpa = NewGrandpaManager(block_tree, secrets[i])
		mockNode.grandpa.Broadcaster = mockNode
		mockNode.grandpa.Storage = nil
		mockNode.grandpa.AuthoritySet = validators
		grandpa := NewGrandpa(block_tree, secrets[i], validators, genesis_blk.Copy(), mockNode, nil, 0)
		mockNode.grandpa.SetGrandpa(0, grandpa)
		go mockNode.grandpa.RunGrandpa()
		go mockNode.grandpa.RunManager()
		mockNodes[i] = mockNode
	}

	// Set allNodes for each mock node
	for _, mockNode := range mockNodes {
		mockNode.allNodes = mockNodes
	}

	return mockNodes
}
func fakeroot() types.Block {
	return types.Block{
		Header: types.BlockHeader{
			ParentHeaderHash: common.Hash{},
			Slot:             0,
		},
	}
}

func fakeblock(block types.Block) *types.Block {
	return &types.Block{
		Header: types.BlockHeader{
			ParentHeaderHash: block.Header.Hash(),
			AuthorIndex:      uint16(rand.Intn(100)),
		},
	}
}

func addBlock(nodes []*MockGrandpaNode, block *types.Block) {
	for _, node := range nodes {
		node.blockTree.AddBlock(block)
		// Mark blocks as Applied and Audited for testing purposes
		if blockNode, ok := node.blockTree.GetBlockNode(block.Header.Hash()); ok {
			blockNode.Applied = true
			blockNode.Audited = true
		}
	}
}
