package node

import (
	"context"
	"fmt"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/grandpa"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
)

// this function will be called when the nodes finish audited the genesis block
var genesisBlockHash common.Hash

func (n *Node) StartGrandpa(b *types.Block) {
	log.Debug(debugGrandpa, "GRANDPA START")
	if n.block_tree != nil {

		return
	}

	if b.GetParentHeaderHash() == (genesisBlockHash) {
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
		case vote := <-n.grandpaPreCommitMessageCh:
			err := n.grandpa.ProcessPreCommitMessage(vote)
			if err != nil {
				fmt.Println(err)
			}
		case vote := <-n.grandpaPrimaryMessageCh:
			err := n.grandpa.ProcessPrimaryProposeMessage(vote)
			if err != nil {
				fmt.Println(err)
			}
		case vote := <-n.grandpa.BroadcastVoteChan:
			go n.broadcast(vote)
		case commit := <-n.grandpa.BroadcastCommitChan:
			go n.broadcast(commit)
		case <-ticker.C:
			// n.grandpa.OnTimeout()
		case err := <-n.grandpa.ErrorChan:
			log.Error(debugGrandpa, "GRANDPA ERROR", "err", err)

		case status := <-n.grandpa.GrandpaStatusChan:
			log.Debug(debugGrandpa, "GRANDPA Status", "status", status)
		}

	}
}
