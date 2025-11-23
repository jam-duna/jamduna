package grandpa

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

func TestMessage(t *testing.T) {
	// test encode/decode message
	root := fakeroot()
	block_tree := types.NewBlockTree(&types.BT_Node{
		Parent:    nil,
		Block:     &root,
		Height:    0,
		Finalized: true,
	})
	nextblock := fakeblock(root)
	block_tree.AddBlock(nextblock)
	validators, secrets, err := statedb.GenerateValidatorSecretSet(6)
	if err != nil {
		panic(err)
	}
	mockNode := &MockGrandpaNode{}
	grandpa := NewGrandpa(block_tree, secrets[0], validators, &root, mockNode, nil)
	mockNode.grandpa = grandpa
	grandpa.InitRoundState(0, block_tree.Copy())
	grandpa.InitTrackers(0)
	vote := grandpa.NewPrecommitVoteMessage(nextblock, 0)
	voteBytes, err := vote.ToBytes()
	if err != nil {
		t.Fatalf("Failed to encode vote message")
	}
	new_vote := GrandpaVote{}
	err = new_vote.FromBytes(voteBytes)
	if err != nil {
		t.Fatalf("Failed to decode vote message")
	}
	// use reflect.DeepEqual to compare two structs
	if !reflect.DeepEqual(vote, new_vote) {
		t.Fatalf("Encode/Decode message failed")
	}

}

func TestGrandpa(t *testing.T) {
	limit := time.After(300 * time.Second) //run five minutes
	genesis_blk := fakeroot()
	nodes := SetupGrandpaNodes(6, genesis_blk)

	ctx := context.Background()

	for _, node := range nodes {
		node.grandpa.PlayGrandpaRound(ctx, 1)
	}
	tmp_blk := genesis_blk
	basePort := uint16(10000)
	graph_server := types.NewGraphServer(basePort)
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
			graph_server.Update(nodes[0].grandpa.block_tree)
		case <-limit:
			break loop
		}

	}

}
