package node

import (
	"context"
	"fmt"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/grandpa"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"
)

// this function will be called when the nodes finish audited the genesis block
func (n *Node) StartGrandpa(b *types.Block) {
	log := fmt.Sprintf("%s StartGrandpa\n", n.String())
	Logger.RecordLogs(storage.Grandpa_status, log, true)
	if n.block_tree != nil {

		return
	}

	if b.GetParentHeaderHash() == (common.Hash{}) {
		genesis_blk := b.Copy()
		n.block_tree = types.NewBlockTree(&types.BT_Node{
			Parent:    nil,
			Block:     genesis_blk,
			Height:    0,
			Finalized: true,
		})

		authorities := make([]types.Ed25519Key, 0)
		for _, validator := range n.statedb.GetSafrole().CurrValidators {
			authorities = append(authorities, validator.Ed25519)
		}
		n.grandpa = grandpa.NewGrandpa(n.block_tree, n.credential.Ed25519Secret[:], authorities, b)
		go n.runGrandpa()
		ctx := context.Background()
		n.grandpa.PlayGrandpaRound(ctx, 1)

	}
}

// this function will be used in receiving the grandpa message and be the bridge between grandpa and network
func (n *Node) runGrandpa() {
	ticker := time.NewTicker(100 * time.Millisecond)
	for {
		select {
		case commit := <-n.grandpaCommitMessageCh:
			_ = commit
		case vote := <-n.grandpaPreVoteMessageCh:
			err := n.grandpa.ProcessPreVoteMessage(vote)
			if err != nil {
				fmt.Println(err)
			}
			// fmt.Printf("%s get vote %v from prevote\n", n.String(), vote.SignMessage.Message.Vote.BlockHash)
		case vote := <-n.grandpaPreCommitMessageCh:
			err := n.grandpa.ProcessPreCommitMessage(vote)
			if err != nil {
				fmt.Println(err)
			}
			// fmt.Printf("%s get vote %v from precommit\n", n.String(), vote.SignMessage.Message.Vote.BlockHash)
		case vote := <-n.grandpaPrimaryMessageCh:
			err := n.grandpa.ProcessPrimaryProposeMessage(vote)
			if err != nil {
				fmt.Println(err)
			}
			// fmt.Printf("%s get vote %v from primary\n", n.String(), vote.SignMessage.Message.Vote.BlockHash)
		case vote := <-n.grandpa.BroadcastVoteChan:
			// fmt.Printf("%s broadcast vote %v, for round %d, stage %v\n", n.String(), vote.SignMessage.Message.Vote.BlockHash, vote.Round, vote.SignMessage.Message.Stage)
			go n.broadcast(vote)
		case commit := <-n.grandpa.BroadcastCommitChan:
			// fmt.Printf("%s broadcast commit %v\n", n.String(), commit.Vote.BlockHash)
			go n.broadcast(commit)
		case <-ticker.C:
			// n.grandpa.OnTimeout()
		case err := <-n.grandpa.ErrorChan:
			Logger.RecordLogs(storage.Grandpa_error, err.Error(), true)
		case status := <-n.grandpa.GrandpaStatusChan:
			Logger.RecordLogs(storage.Grandpa_status, status, true)
		}

	}
}
