package grandpa

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/colorfulnotion/jam/types"
	"github.com/stretchr/testify/assert"
)

// in this file, we will test some senarios of the grandpa protocol

func TestVoteGraph(t *testing.T) {
	t.Run("case_fork", case_fork)
	t.Run("case_fork_unvoted", case_fork_unvoted)
	t.Run("case_tri_fork", case_tri_fork)
	t.Run("case_equvocation", case_equvocation)
	t.Run("case_multi_equvocation", case_multi_equvocation)
}

/*case 1
   A (finalized)
   |
   B (finalized)
   |
   C
  / \
 D   E
(D, E) both get half prevotes
*/
// c should be finalized
func case_fork(t *testing.T) {
	genesis_blk := fakeroot()
	nodes := SetupGrandpaNodes(9, genesis_blk)
	new_block := fakeblock(genesis_blk)
	runNodes(nodes)
	addBlock(nodes, new_block)
	for _, node := range nodes {
		go node.round_finalized_cheat(1, new_block)
	}
	time.Sleep(1 * time.Second)
	new_block = fakeblock(*new_block)
	addBlock(nodes, new_block)
	new_block2 := fakeblock(*new_block)
	new_block3 := fakeblock(*new_block)
	addBlock(nodes, new_block2)
	addBlock(nodes, new_block3)
	time.Sleep(2 * time.Second)
	for _, node := range nodes {
		node.InitRoundState(2, node.block_tree.Copy())
	}
	for i, node := range nodes {

		if i >= 5 {
			node.sendDummyVote(2, PrevoteStage, new_block2)

		} else {
			node.sendDummyVote(2, PrevoteStage, new_block3)
		}

	}
	time.Sleep(500 * time.Millisecond)
	for _, node := range nodes {
		ghost, _, err := node.GrandpaGhost(2)
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, ghost, new_block.Header.Hash())
		node.sendDummyVote(2, PrecommitStage, new_block2)
	}

	time.Sleep(1 * time.Second)
	for _, node := range nodes {
		Finalizable, err := node.Finalizable(2)
		if err != nil {
			fmt.Println(err)
		}
		if !Finalizable {
			round_state, err := node.GetRoundState(2)
			if err != nil {
				fmt.Println(err)

			}
			round_state.PreVoteGraph.Print()
			round_state, err = node.GetRoundState(1)
			if err != nil {
				fmt.Println(err)

			}
			round_state.PreCommitGraph.Print()

			ghost, ghost_node, _ := node.GrandpaGhost(2)
			best_final_candidate, best_final_candidate_node, _ := node.BestFinalCandidate(2)
			best_final_candidate_m1, best_final_candidate_m1_node, _ := node.BestFinalCandidate(1)
			fmt.Printf("ghost = %s\n", ghost.String())
			fmt.Printf("best_final_candidate = %s\n", best_final_candidate.String())
			fmt.Printf("best_final_candidate_m1 = %s\n", best_final_candidate_m1.String())
			if !ghost_node.IsAncestor(best_final_candidate_m1_node) {
				fmt.Println("ghost is not ancestor of best_final_candidate_m1")
			}
			if !ghost_node.IsAncestor(best_final_candidate_node) {
				fmt.Println("ghost is not ancestor of best_final_candidate")
			}

			t.Fatalf("node %d is not completable", node.GetSelfVoterIndex(2))
		}
		err = node.AttemptToFinalizeAtRound(2)
		if err != nil {
			fmt.Println(err)
		}

	}
	time.Sleep(2 * time.Second)
	nodes[0].block_tree.Print()
	assert.Equal(t, nodes[0].block_tree.GetLastFinalizedBlock().Block.Header.Hash(), new_block.Header.Hash())
}

/*
case 1.1

	   A (finalized)
	   |
	   B (finalized)
	   |
	   C
	  / \
	 D   E
	      \
		   F

(E, F) E get 1/3+1 and F get 2/3 prevotes
and unvoted for 1/3 prevotes
E should be finalized
*/
func case_fork_unvoted(t *testing.T) {
	genesis_blk := fakeroot()
	nodes := SetupGrandpaNodes(9, genesis_blk)
	new_block := fakeblock(genesis_blk)
	runNodes(nodes)
	addBlock(nodes, new_block)
	for _, node := range nodes {
		go node.round_finalized_cheat(1, new_block)
	}
	time.Sleep(1 * time.Second)
	new_block = fakeblock(*new_block)
	addBlock(nodes, new_block)
	new_block2 := fakeblock(*new_block)
	new_block_2_1 := fakeblock(*new_block2)
	new_block3 := fakeblock(*new_block)
	addBlock(nodes, new_block2)
	addBlock(nodes, new_block3)
	addBlock(nodes, new_block_2_1)
	time.Sleep(2 * time.Second)
	for _, node := range nodes {
		node.InitRoundState(2, node.block_tree.Copy())
	}
	for i, node := range nodes {

		if i >= 6 && i < 9 {
			node.sendDummyVote(2, PrevoteStage, new_block2)

		} else if i >= 2 && i < 6 {
			node.sendDummyVote(2, PrevoteStage, new_block_2_1)
		}

	}
	time.Sleep(500 * time.Millisecond)
	for _, node := range nodes {
		_, ghost_block, err := node.GrandpaGhost(2)
		if err != nil {
			t.Fatal(err)
		}
		node.sendDummyVote(2, PrecommitStage, ghost_block.Block)
	}

	time.Sleep(1 * time.Second)
	for _, node := range nodes {
		Finalizable, err := node.Finalizable(2)
		if err != nil {
			fmt.Println(err)
		}
		if !Finalizable {
			round_state, err := node.GetRoundState(2)
			if err != nil {
				fmt.Println(err)

			}
			round_state.PreVoteGraph.Print()
			round_state, err = node.GetRoundState(1)
			if err != nil {
				fmt.Println(err)

			}
			round_state.PreCommitGraph.Print()

			ghost, ghost_node, _ := node.GrandpaGhost(2)
			best_final_candidate, best_final_candidate_node, _ := node.BestFinalCandidate(2)
			best_final_candidate_m1, best_final_candidate_m1_node, _ := node.BestFinalCandidate(1)
			fmt.Printf("ghost = %s\n", ghost.String())
			fmt.Printf("best_final_candidate = %s\n", best_final_candidate.String())
			fmt.Printf("best_final_candidate_m1 = %s\n", best_final_candidate_m1.String())
			if !ghost_node.IsAncestor(best_final_candidate_m1_node) {
				fmt.Println("ghost is not ancestor of best_final_candidate_m1")
			}
			if !ghost_node.IsAncestor(best_final_candidate_node) {
				fmt.Println("ghost is not ancestor of best_final_candidate")
			}

			t.Fatalf("node %d is not completable", node.GetSelfVoterIndex(2))
		}
		err = node.AttemptToFinalizeAtRound(2)
		if err != nil {
			fmt.Println(err)
		}

	}
	time.Sleep(2 * time.Second)
	nodes[0].block_tree.Print()
	assert.Equal(t, nodes[0].block_tree.GetLastFinalizedBlock().Block.Header.Hash(), new_block2.Header.Hash())
}

/*case 2
   A (finalized)
   |
   B (finalized)
   |
   C
  /|\
 D E F
D: 33% , E: 33%  F: 33%
*/

func case_tri_fork(t *testing.T) {
	genesis_blk := fakeroot()
	nodes := SetupGrandpaNodes(9, genesis_blk)
	new_block := fakeblock(genesis_blk)
	runNodes(nodes)
	addBlock(nodes, new_block)
	for _, node := range nodes {
		go node.round_finalized_cheat(1, new_block)
	}
	time.Sleep(1 * time.Second)
	new_block = fakeblock(*new_block)
	addBlock(nodes, new_block)
	new_block2 := fakeblock(*new_block)
	new_block3 := fakeblock(*new_block)
	new_block4 := fakeblock(*new_block)
	addBlock(nodes, new_block2)
	addBlock(nodes, new_block3)
	addBlock(nodes, new_block4)
	time.Sleep(2 * time.Second)
	for _, node := range nodes {
		node.InitRoundState(2, node.block_tree.Copy())
	}
	for i, node := range nodes {

		if i >= 3 && i < 6 {
			node.sendDummyVote(2, PrevoteStage, new_block2)
		} else if i >= 6 && i < 9 {
			node.sendDummyVote(2, PrevoteStage, new_block3)
		} else {
			node.sendDummyVote(2, PrevoteStage, new_block4)
		}

	}

	time.Sleep(500 * time.Millisecond)
	for _, node := range nodes {
		ghost, _, err := node.GrandpaGhost(2)
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, ghost, new_block.Header.Hash())
		node.sendDummyVote(2, PrecommitStage, new_block2)
	}

	time.Sleep(1 * time.Second)
	for _, node := range nodes {
		completable, err := node.Finalizable(2)
		if err != nil {
			fmt.Println(err)
		}
		fmt.Printf("[v%d] completable = %v\n", node.GetSelfVoterIndex(2), completable)
		if !completable {
			round_state, err := node.GetRoundState(2)
			if err != nil {
				fmt.Println(err)
			}
			round_state.PreVoteGraph.Print()
		}
		err = node.AttemptToFinalizeAtRound(2)
		if err != nil {
			fmt.Println(err)
		}

	}
	time.Sleep(2 * time.Second)
	nodes[0].block_tree.Print()
	assert.Equal(t, nodes[0].block_tree.GetLastFinalizedBlock().Block.Header.Hash(), new_block.Header.Hash())
}

/*
case 3 E,F equvocation

	  A (finalized)
	  |
	  B (finalized)
	  |
	  C
	 /|\
	D E F

D: 33% , E: 33%  F: 33%
*/
func case_equvocation(t *testing.T) {
	genesis_blk := fakeroot()
	nodes := SetupGrandpaNodes(9, genesis_blk)
	new_block := fakeblock(genesis_blk)
	runNodes(nodes)
	addBlock(nodes, new_block)
	for _, node := range nodes {
		go node.round_finalized_cheat(1, new_block)
	}
	time.Sleep(1 * time.Second)
	new_block = fakeblock(*new_block)
	addBlock(nodes, new_block)
	new_block2 := fakeblock(*new_block)
	new_block3 := fakeblock(*new_block)
	new_block4 := fakeblock(*new_block)
	addBlock(nodes, new_block2)
	addBlock(nodes, new_block3)
	addBlock(nodes, new_block4)
	time.Sleep(2 * time.Second)
	for _, node := range nodes {
		node.InitRoundState(2, node.block_tree.Copy())
	}
	fmt.Printf("nodes length = %d\n", len(nodes))
	for i, node := range nodes {

		if i >= 3 && i < 6 {
			node.sendDummyVote(2, PrevoteStage, new_block2)
			node.sendDummyVote(2, PrecommitStage, new_block2)
		} else if i >= 6 && i < 9 {
			node.sendDummyVote(2, PrevoteStage, new_block3)
			node.sendDummyVote(2, PrecommitStage, new_block3)
		} else {
			node.sendDummyVote(2, PrevoteStage, new_block4)
			node.sendDummyVote(2, PrecommitStage, new_block4)
			node.sendDummyVote(2, PrevoteStage, new_block3)
			node.sendDummyVote(2, PrecommitStage, new_block3)
		}

	}
	fmt.Printf("equvocation on block %s\n", new_block3.Header.Hash().String())
	time.Sleep(1 * time.Second)
	for _, node := range nodes {
		completable, err := node.Finalizable(2)
		if err != nil {
			fmt.Println(err)
		}
		fmt.Printf("[v%d] completable = %v\n", node.GetSelfVoterIndex(2), completable)
		if completable {
			round_state, err := node.GetRoundState(2)
			if err != nil {
				fmt.Println(err)
			}
			round_state.PreVoteGraph.Print()
		}
		err = node.AttemptToFinalizeAtRound(2)
		if err != nil {
			fmt.Println(err)
		}

	}
	time.Sleep(2 * time.Second)
	nodes[0].block_tree.Print()
	assert.Equal(t, nodes[0].block_tree.GetLastFinalizedBlock().Block.Header.Hash(), new_block.Header.Hash())
}

/*
case 4  equvocation

		  A (finalized)
		  |
		  B (finalized)
		  |
		  C
		 /|\
		D E F
	   /  |  \
	  G   H   I

G H I has 20% of the votes respectively
E F has 20% of the votes respectively and will have some ramdom equvocation
*/
func case_multi_equvocation(t *testing.T) {
	genesis_blk := fakeroot()
	nodes := SetupGrandpaNodes(9, genesis_blk)
	new_block := fakeblock(genesis_blk)
	runNodes(nodes)
	addBlock(nodes, new_block)
	for _, node := range nodes {
		go node.round_finalized_cheat(1, new_block)
	}
	time.Sleep(1 * time.Second)
	new_block = fakeblock(*new_block)
	addBlock(nodes, new_block)
	new_block2 := fakeblock(*new_block)
	new_block3 := fakeblock(*new_block)
	new_block4 := fakeblock(*new_block)
	addBlock(nodes, new_block2)
	addBlock(nodes, new_block3)
	addBlock(nodes, new_block4)
	new_block5 := fakeblock(*new_block2)
	new_block6 := fakeblock(*new_block3)
	new_block7 := fakeblock(*new_block4)
	addBlock(nodes, new_block5)
	addBlock(nodes, new_block6)
	addBlock(nodes, new_block7)
	voting_set := []*types.Block{new_block2, new_block3, new_block4, new_block5, new_block6, new_block7}
	time.Sleep(2 * time.Second)
	for _, node := range nodes {
		node.InitRoundState(2, node.block_tree.Copy())
	}
	fmt.Printf("nodes length = %d\n", len(nodes))
	for i, node := range nodes {

		if i <= 1 {
			node.sendDummyVote(2, PrevoteStage, new_block7)
			node.sendDummyVote(2, PrecommitStage, new_block7)
		} else if i >= 2 && i < 4 {
			node.sendDummyVote(2, PrevoteStage, new_block3)
			node.sendDummyVote(2, PrecommitStage, new_block3)
		} else if i >= 4 && i < 6 {
			node.sendDummyVote(2, PrevoteStage, new_block4)
			node.sendDummyVote(2, PrecommitStage, new_block4)
		} else if i >= 6 && i < 8 {
			node.sendDummyVote(2, PrevoteStage, new_block5)
			node.sendDummyVote(2, PrecommitStage, new_block5)
		} else if i >= 8 && i < 10 {
			node.sendDummyVote(2, PrevoteStage, new_block6)
			node.sendDummyVote(2, PrecommitStage, new_block6)
		}

		rand_num := rand.Intn(10)
		if rand_num <= 5 {
			node.sendDummyVote(2, PrevoteStage, voting_set[rand_num])
			node.sendDummyVote(2, PrecommitStage, voting_set[rand_num])
		}

	}
	fmt.Printf("equvocation on block %s\n", new_block3.Header.Hash().String())
	time.Sleep(1 * time.Second)
	for _, node := range nodes {
		completable, err := node.Finalizable(2)
		if err != nil {
			fmt.Println(err)
		}
		fmt.Printf("[v%d] completable = %v\n", node.GetSelfVoterIndex(2), completable)
		if completable {
			round_state, err := node.GetRoundState(2)
			if err != nil {
				fmt.Println(err)
			}
			round_state.PreVoteGraph.Print()
		}
		err = node.AttemptToFinalizeAtRound(2)
		if err != nil {
			fmt.Println(err)
		}

	}
	time.Sleep(2 * time.Second)

	// we can't predict the final block hash
	// so I'll print the tree and check if the last finalized block is correct
	roundstate, err := nodes[0].GetRoundState(2)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("PreVoteGraph\n")
	roundstate.PreVoteGraph.Print()
	fmt.Printf("\nPreCommitGraph\n")
	roundstate.PreCommitGraph.Print()
	fmt.Printf("result\n")
	nodes[0].block_tree.Print()
}

func (node *Grandpa) sendDummyVote(round uint64, stage GrandpaStage, block *types.Block) {
	switch stage {
	case PrevoteStage:
		vote := node.NewPrevoteVoteMessage(block, round)
		node.BroadcastVoteChan <- vote
		node.ProcessPreVoteMessage(vote)
	case PrecommitStage:
		vote := node.NewPrecommitVoteMessage(block, round)
		node.BroadcastVoteChan <- vote
		node.ProcessPreCommitMessage(vote)
	case PrimaryProposeStage:
		vote := node.NewPrimaryVoteMessage(block, round)
		node.BroadcastVoteChan <- vote
		node.ProcessPrimaryProposeMessage(vote)
	default:
		fmt.Println("stage error")
	}

}

func (node *Grandpa) round_finalized_cheat(round uint64, vote_block *types.Block) {
	node.InitRoundState(round, node.block_tree.Copy())
	node.sendDummyVote(round, PrevoteStage, vote_block)
	node.sendDummyVote(round, PrecommitStage, vote_block)
	time.Sleep(1 * time.Second)
	node.AttemptToFinalizeAtRound(round)
}
