package grandpa

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/colorfulnotion/jam/types"
)

func SetupGrandpaNodes(authorities_num int, genesis_blk types.Block) []*Grandpa {
	privates, keys, err := InitializeAuthoritySet(authorities_num)
	if err != nil {
		panic(err)
	}

	nodes := make([]*Grandpa, authorities_num)
	for i := 0; i < authorities_num; i++ {
		block_tree := types.NewBlockTree(&types.BT_Node{
			Parent:    nil,
			Block:     genesis_blk.Copy(),
			Height:    0,
			Finalized: true,
		})
		nodes[i] = NewGrandpa(block_tree, privates[i], keys, genesis_blk.Copy())
	}
	return nodes
}

// every nodes has broadcast channel, and every node will broadcast the vote message to the channel
// come up with a way to simulate the network

func runNodes(nodes []*Grandpa) {
	for _, node := range nodes {
		go func(node *Grandpa) {
			for {
				select {
				case vote := <-node.BroadcastVoteChan:
					stage := vote.SignMessage.Message.Stage
					switch stage {
					case PrimaryProposeStage:
						fmt.Printf("[v%d]Broadcast PrimaryPropose block_hash = %v\n", node.GetSelfVoterIndex(node.Last_Completed_Round), vote.SignMessage.Message.Vote.BlockHash.String_short())
					case PrevoteStage:
						fmt.Printf("[v%d]Broadcast Prevote block_hash = %v\n", node.GetSelfVoterIndex(node.Last_Completed_Round), vote.SignMessage.Message.Vote.BlockHash.String_short())
					case PrecommitStage:
						fmt.Printf("[v%d]Broadcast Precommit block_hash = %v\n", node.GetSelfVoterIndex(node.Last_Completed_Round), vote.SignMessage.Message.Vote.BlockHash.String_short())
					}
					for _, n := range nodes {
						if n == node {
							continue
						}
						switch stage {
						case PrevoteStage:
							err := n.ProcessPreVoteMessage(vote)
							if err != nil {
								fmt.Println(err)
							} else {
								// fmt.Printf("[v%d]<-[v%d] get PreVote block_hash = %v\n ", n.GetSelfVoterIndex(node.Last_Completed_Round), node.GetSelfVoterIndex(node.Last_Completed_Round), vote.SignMessage.Message.Vote.BlockHash.String_short())
							}
						case PrecommitStage:
							err := n.ProcessPreCommitMessage(vote)
							if err != nil {
								fmt.Println(err)
							} else {
								// fmt.Printf("[v%d]<-[v%d] get Precommit block_hash = %v\n ", n.GetSelfVoterIndex(node.Last_Completed_Round), node.GetSelfVoterIndex(node.Last_Completed_Round), vote.SignMessage.Message.Vote.BlockHash.String_short())
							}
						case PrimaryProposeStage:
							err := n.ProcessPrimaryProposeMessage(vote)
							if err != nil && node == nodes[0] {
								fmt.Println(err)
							} else {
								// fmt.Printf("[v%d]<-[v%d] get PrimaryPropose block_hash = %v\n ", n.GetSelfVoterIndex(node.Last_Completed_Round), node.GetSelfVoterIndex(node.Last_Completed_Round), vote.SignMessage.Message.Vote.BlockHash.String_short())
							}
						}

					}
				case commit := <-node.BroadcastCommitChan:
					for _, n := range nodes {
						_ = n
						_ = commit

					}
				case err := <-node.ErrorChan:
					fmt.Println(err)
				}
			}
		}(node)
	}
}

func addBlock(nodes []*Grandpa, block *types.Block) {
	for _, node := range nodes {
		node.block_tree.AddBlock(block)
	}
}

func TestRunNodes(t *testing.T) {
	genesis_blk := fakeroot()
	nodes := SetupGrandpaNodes(9, genesis_blk)
	runNodes(nodes)
	time.Sleep(10 * time.Second)
}
func TestFunctions(t *testing.T) {
	genesis_blk := fakeroot()
	nodes := SetupGrandpaNodes(9, genesis_blk)
	runNodes(nodes)
	new_block := fakeblock(genesis_blk)
	addBlock(nodes, new_block)
	time.Sleep(10 * time.Second)
	for _, node := range nodes {
		node.InitRoundState(1, node.block_tree.Copy())
	}

}

func TestPlayRound(t *testing.T) {
	limit := time.After(300 * time.Second) //run five minutes
	genesis_blk := fakeroot()
	nodes := SetupGrandpaNodes(9, genesis_blk)
	runNodes(nodes)

	ctx := context.Background()

	for _, node := range nodes {
		node.PlayGrandpaRound(ctx, 1)
	}
	tmp_blk := genesis_blk
	graph_server := types.NewGraphServer()
	go graph_server.StartServer()
	ticker := time.NewTicker(6 * time.Second)
	ticker2 := time.NewTicker(1 * time.Second)
loop:
	for {
		select {
		case <-ticker.C:
			new_block := fakeblock(tmp_blk)
			addBlock(nodes, new_block)
			fmt.Printf("New Block : %v\n", new_block.Header.Hash().String_short())
			tmp_blk = *new_block
		case <-ticker2.C:
			graph_server.Update(nodes[0].block_tree)
		case <-limit:
			break loop
		}

	}

}
