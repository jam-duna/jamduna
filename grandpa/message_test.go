package grandpa

import (
	"math/rand"
	"reflect"
	"testing"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

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
	privates, keys, err := InitializeAuthoritySet(1)
	if err != nil {
		t.Fatalf("Failed to init ed25519 key")
	}
	grandpa := NewGrandpa(block_tree, privates[0], keys, &root)
	grandpa.SetScheduledAuthoritySet(0, keys, 0)
	grandpa.InitRoundState(0, block_tree.Copy())
	grandpa.InitTrackers(0)
	vote := grandpa.NewPrecommitVoteMessage(nextblock, 0)
	voteBytes, err := vote.ToBytes()
	if err != nil {
		t.Fatalf("Failed to encode vote message")
	}
	new_vote := VoteMessage{}
	err = new_vote.FromBytes(voteBytes)
	if err != nil {
		t.Fatalf("Failed to decode vote message")
	}
	// use reflect.DeepEqual to compare two structs
	if !reflect.DeepEqual(vote, new_vote) {
		t.Fatalf("Encode/Decode message failed")
	}

}
