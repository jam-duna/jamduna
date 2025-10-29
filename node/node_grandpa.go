package node

import (
	"context"
	"fmt"

	"github.com/colorfulnotion/jam/common"
	grandpa "github.com/colorfulnotion/jam/grandpa"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
)

// this function will be called when the nodes finish audited the genesis block
var genesisBlockHash common.Hash

func (n *Node) StartGrandpa(b *types.Block) {
	log.Trace(log.Grandpa, "GRANDPA START")
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
			n.broadcast(context.TODO(), vote)
		case commit := <-n.grandpa.BroadcastCommitChan:
			n.broadcast(context.TODO(), commit)
		case err := <-n.grandpa.ErrorChan:
			log.Error(log.Grandpa, "GRANDPA ERROR", "err", err)

		case status := <-n.grandpa.GrandpaStatusChan:
			log.Trace(log.Grandpa, "GRANDPA Status", "status", status)
		}

	}
}
