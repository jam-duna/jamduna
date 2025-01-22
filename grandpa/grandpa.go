package grandpa

import (
	"fmt"
	"sync"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

type Grandpa struct {
	block_tree            *types.BlockTree // hold the same block tree as the node itself
	RoundStateMutex       sync.RWMutex
	RoundState            map[uint64]*RoundState
	selfkey               types.Ed25519Priv
	Last_Completed_Round  uint64
	ScheduledAuthoritySet map[uint64]Authority
	Voter_Staked          map[uint64]uint64
	BroadcastVoteChan     chan VoteMessage
	BroadcastCommitChan   chan CommitMessage
	ErrorChan             chan error
	GrandpaStatusChan     chan string
}
type GrandpaRound struct {
	Round        uint64
	Timer1Signal chan bool
	Timer2Signal chan bool
	Ticker       *time.Ticker
}

func (g *Grandpa) InitRoundState(round uint64, block_tree *types.BlockTree) {
	g.RoundStateMutex.Lock()
	defer g.RoundStateMutex.Unlock()
	if _, exists := g.RoundState[round]; !exists {
		g.RoundState[round] = &RoundState{
			GrandpaState:   NewGrandpaState(g.GetCurrentScheduledAuthoritySet(round).Authorities, g.GetCurrentScheduledAuthoritySet(round).AuthoritiesSet, round),
			PreVoteGraph:   block_tree.Copy(),
			PreCommitGraph: block_tree.Copy(),
			GrandpaCJF_Map: NewGrandpaCJF(len(g.GetCurrentScheduledAuthoritySet(round).Authorities)),
			VoteTracker:    &sync.Map{},
		}
	}
}

func (g *Grandpa) GetRoundState(round uint64) (*RoundState, error) {
	g.RoundStateMutex.RLock()
	defer g.RoundStateMutex.RUnlock()
	if _, exists := g.RoundState[round]; !exists {
		return nil, fmt.Errorf("round state not found")
	}
	return g.RoundState[round], nil
}

func (g *Grandpa) GetRoundGrandpaState(round uint64) *GrandpaState {
	g.RoundStateMutex.RLock()
	defer g.RoundStateMutex.RUnlock()
	if _, exists := g.RoundState[round]; !exists {
		return nil
	} else {
		return g.RoundState[round].GrandpaState
	}
}

func (g *Grandpa) GetCJF(round uint64) (*GrandpaCJF, bool) {
	g.RoundStateMutex.RLock()
	defer g.RoundStateMutex.RUnlock()
	if _, exists := g.RoundState[round]; !exists {
		return nil, false
	} else {
		return g.RoundState[round].GrandpaCJF_Map, true
	}
}

func NewGrandpa(block_tree *types.BlockTree, selfkey types.Ed25519Priv, authorities []types.Ed25519Key, genesis_blk *types.Block) *Grandpa {
	staked := make(map[uint64]uint64)
	for i := 0; i < len(authorities); i++ {
		staked[uint64(i)] = 100
	}
	g := &Grandpa{
		block_tree:            block_tree,
		RoundState:            make(map[uint64]*RoundState),
		selfkey:               selfkey,
		Last_Completed_Round:  0,
		ScheduledAuthoritySet: make(map[uint64]Authority),
		Voter_Staked:          staked,
	}
	g.SetScheduledAuthoritySet(0, authorities, 0)
	g.InitRoundState(0, block_tree.Copy())
	g.InitTrackers(0)
	// round_state, err:=g.GetRoundState(0)
	// if err != nil {
	// 	fmt.Printf("Error in GetRoundState: %v\n", err)
	// }
	// round_state. = NewGrandpaState(authorities, 0, 0)
	tempSet := g.ScheduledAuthoritySet[0]
	tempSet.Authorities = authorities
	tempSet.AuthoritiesSet = 0
	g.ScheduledAuthoritySet[0] = tempSet
	g.BroadcastCommitChan = make(chan CommitMessage, 200)
	g.BroadcastVoteChan = make(chan VoteMessage, 200)
	g.GrandpaStatusChan = make(chan string, 200)
	g.ErrorChan = make(chan error, 200)
	g.BroadcastVoteChan <- g.NewPrecommitVoteMessage(genesis_blk, 0)
	g.BroadcastVoteChan <- g.NewPrevoteVoteMessage(genesis_blk, 0)
	commit, _ := g.NewFinalCommitMessage(genesis_blk, 0)
	g.BroadcastCommitChan <- commit
	return g
}

func (g *Grandpa) GetCurrentScheduledAuthoritySet(round uint64) Authority {
	return g.ScheduledAuthoritySet[0] // TODO: need to change to round
}

func (g *Grandpa) GetVoteTracker(round uint64, stage GrandpaStage) *VoteTracker {
	round_state, err := g.GetRoundState(round)
	if err != nil {
		return nil
	}
	if round_state.VoteTracker == nil {
		round_state.VoteTracker = &sync.Map{}
	}
	if _, exists := round_state.VoteTracker.Load(stage); !exists {
		g.InitTrackers(round)
	}
	value, ok := round_state.VoteTracker.Load(stage)
	if !ok {
		return nil
	}
	return value.(*VoteTracker)
}

func (g *Grandpa) GetSupermajorityThreshold(round uint64) int {
	g.RoundStateMutex.RLock()
	defer g.RoundStateMutex.RUnlock()
	total_vote_weight := uint64(0)
	for _, staked := range g.Voter_Staked {
		total_vote_weight += staked
	}
	weight := int(total_vote_weight)
	return weight*2/3 + 1
}

func (g *Grandpa) SetScheduledAuthoritySet(round uint64, authorities []types.Ed25519Key, authoritiesSet uint64) {
	g.ScheduledAuthoritySet[0] = Authority{
		Authorities:    authorities,
		AuthoritiesSet: authoritiesSet,
	}
}

func (g *Grandpa) InitTrackers(round uint64) {
	authorities := g.GetCurrentScheduledAuthoritySet(round).Authorities
	round_state, _ := g.GetRoundState(round)
	if round_state == nil {
		g.InitRoundState(round, g.block_tree.Copy())
		round_state, _ = g.GetRoundState(round)
	}
	tracker := round_state.VoteTracker
	if _, exists := tracker.Load(PrevoteStage); !exists {
		tracker.Store(PrevoteStage, NewVoteTracker(PrevoteStage, round, g.Voter_Staked))
	}
	if _, exists := tracker.Load(PrecommitStage); !exists {
		tracker.Store(PrecommitStage, NewVoteTracker(PrecommitStage, round, g.Voter_Staked))
	}
	if _, exists := tracker.Load(PrimaryProposeStage); !exists {
		tracker.Store(PrimaryProposeStage, NewVoteTracker(PrimaryProposeStage, round, g.Voter_Staked))
	}
	round_state.GrandpaCJF_Map = NewGrandpaCJF(len(authorities))
}

func NewGrandpaCJF(len int) *GrandpaCJF {
	return &GrandpaCJF{
		Votes:          make([]Vote, len),
		Justifications: make([]JustificationElement, len),
	}
}

type Authority struct {
	AuthoritiesSet uint64
	Authorities    []types.Ed25519Key
}

// Algorithm 15. Derive Primary
func DerivePrimary(r uint64, V []types.Ed25519Key) uint64 {
	return r % uint64(len(V))
}

// Algorithm 16. Best Final Candidate
func (g *Grandpa) BestFinalCandidate(round uint64) (common.Hash, *types.BT_Node, error) {
	ghost_hash, ghost, err := g.GrandpaGhost(round)
	if err != nil {
		return common.Hash{}, nil, err
	}
	if round == 0 {
		return ghost_hash, ghost, nil
	} else {
		// get the young blocks from the tree
		round_state, err := g.GetRoundState(round)
		if err != nil {
			return common.Hash{}, nil, err
		}
		round_state.UpdatePreCommitGraph(true)
		precommit_graph := round_state.PreCommitGraph
		// root_hash := precommit_graph.Root.Block.Header.Hash()。
		old_ghost, err := precommit_graph.FindGhost(ghost_hash, uint64(g.GetSupermajorityThreshold(round)))
		if err != nil {
			return ghost_hash, ghost, nil
		}
		old_ghost, ok := g.block_tree.GetBlockNode(old_ghost.Block.Header.Hash())
		older := g.block_tree.ChildOrBrother(ghost, old_ghost)
		if !older {
			return ghost_hash, ghost, nil
		}
		if err != nil || !ok {
			return ghost_hash, ghost, nil
		}
		if old_ghost.Block.GetParentHeaderHash() == ghost.Block.GetParentHeaderHash() {
			return ghost_hash, ghost, nil
		} else {
			return old_ghost.Block.Header.Hash(), old_ghost, nil
		}

	}
}

// Algorithm 17. GRANDPA GHOST
func (g *Grandpa) GrandpaGhost(round uint64) (common.Hash, *types.BT_Node, error) {
	if round == 0 {
		return g.block_tree.GetLastFinalizedBlock().Block.Header.Hash(), g.block_tree.GetLastFinalizedBlock(), nil
	} else {
		best_candidate, best_candidate_block, err := g.BestFinalCandidate(round - 1)
		if err != nil {
			return common.Hash{}, nil, err
		}
		ghost_blocks, _, err := g.block_tree.GetDescendingBlocks(best_candidate)
		if err != nil {
			return common.Hash{}, nil, err
		}
		if len(ghost_blocks) == 0 {
			return best_candidate, best_candidate_block, nil
		}
		latest_block := best_candidate_block
		round_state, err := g.GetRoundState(round)
		if err != nil {
			return common.Hash{}, nil, err
		}
		round_state.UpdatePreVoteGraph(false)
		prevote_graph := round_state.PreVoteGraph
		ghost, err := prevote_graph.FindGhost(best_candidate, uint64(g.GetSupermajorityThreshold(round)))
		if err != nil {
			return latest_block.Block.Header.Hash(), latest_block, nil
		} else {
			ghost_hash := ghost.Block.Header.Hash()
			ghost_block, find := g.block_tree.GetBlockNode(ghost_hash)
			if !find {
				return latest_block.Block.Header.Hash(), latest_block, nil
			}
			if ghost_block.IsAncestor(best_candidate_block) {
				return ghost_hash, ghost_block, nil
			} else {
				return latest_block.Block.Header.Hash(), latest_block, nil
			}
		}
	}
}

// Algorithm 18. Best PreVote Candidate
func (g *Grandpa) BestPreVoteCandidate(round uint64) (common.Hash, *types.BT_Node, error) {
	round_state, err := g.GetRoundState(round)
	if err != nil {
		return common.Hash{}, nil, err
	}
	// get the ghost block
	L_Hash, L_block, err := g.BestFinalCandidate(round - 1)
	if err != nil {
		return common.Hash{}, nil, err
	}
	err = round_state.UpdatePreVoteGraph(true)
	if err != nil {
		return common.Hash{}, nil, err
	}
	prevote_graph := round_state.PreVoteGraph
	ghost, err := prevote_graph.FindGhost(L_Hash, uint64(g.GetSupermajorityThreshold(round)))
	if err != nil {
		return common.Hash{}, L_block, err
	}
	ghost_hash := ghost.Block.Header.Hash()
	// use real block tree
	ghost_block, find := g.block_tree.GetBlockNode(ghost_hash)
	if !find {
		return common.Hash{}, L_block, fmt.Errorf("ghost block not found")
	}
	// if receive the primary
	primary_id := DerivePrimary(round, g.GetCurrentScheduledAuthoritySet(round).Authorities)
	proposed_hash, err_proposed := common.Hash{}, fmt.Errorf("no proposed block")
	tracker := g.GetVoteTracker(round, PrimaryProposeStage)
	proposed_hash, err_proposed = tracker.GetPrimaryPropose(primary_id)
	if err_proposed == nil {
		proposed_block, err := prevote_graph.FindGhost(proposed_hash, uint64(g.GetSupermajorityThreshold(round)))
		if err != nil {

			return ghost_hash, ghost_block, nil
		} else {
			proposed_hash = proposed_block.Block.Header.Hash()
			proposed_block, ok := g.block_tree.GetBlockNode(proposed_hash)
			if !ok {
				return ghost_hash, ghost_block, nil
			} else {
				return proposed_hash, proposed_block, nil
			}
		}
	} else {
		return ghost_hash, ghost_block, nil
	}
}

// Algorithm 19. Attempt To Finalize At Round
func (g *Grandpa) AttemptToFinalizeAtRound(round uint64) error {

	//L
	last_finalized_block := g.block_tree.GetLastFinalizedBlock()
	// last_finalized_block_hash := last_finalized_block.Block.Header.Hash()()

	//E
	best_final_candidate_hash, best_final_candidate, err := g.BestFinalCandidate(round)
	if err != nil {
		return err
	}
	e_bigger_than_l := g.block_tree.ChildOrBrother(best_final_candidate, last_finalized_block)
	round_state, err := g.GetRoundState(round)
	if err != nil {
		return err
	}

	round_state.UpdatePreCommitGraph(false)
	precommit_graph := round_state.PreCommitGraph
	E_node, ok := precommit_graph.GetBlockNode(best_final_candidate_hash)
	if !ok {
		return fmt.Errorf("best final candidate not found")
	}
	e_enough_votes := E_node.GetCumulativeVotesWeight() > uint64(g.GetSupermajorityThreshold(round))

	if e_bigger_than_l && e_enough_votes {
		g.block_tree.FinalizeBlock(best_final_candidate_hash)
	}
	commit, err := g.NewFinalCommitMessage(best_final_candidate.Block, round)
	if err != nil {
		return err
	}
	g.BroadcastCommitChan <- commit
	return nil
}

// Algorithm 20. Finalizable
func (g *Grandpa) Finalizable(round uint64) (bool, error) {
	completable, err := g.Completable(round)
	if err != nil {
		return false, err
	}
	if !completable {
		return false, nil
	}
	_, ghost_block, err := g.GrandpaGhost(round)
	if err != nil {
		return false, err
	}
	_, best_candidate_block, err := g.BestFinalCandidate(round)
	if err != nil {
		return false, err
	}
	_, best_candidate_m1_block, err := g.BestFinalCandidate(round - 1)
	if err != nil {
		return false, err
	}
	first := g.block_tree.ChildOrBrother(best_candidate_block, best_candidate_m1_block)
	second := g.block_tree.ChildOrBrother(ghost_block, best_candidate_block)
	finalize := first && second
	// fmt.Printf("Finalizable: %v, %v, %v\n", first, second, finalize)
	// fmt.Printf("best candidate block %v, ghost block %v, best candidate m1 block %v\n", best_candidate_block.Block.Header.Hash().String_short(), ghost_block.Block.Header.Hash().String_short(), best_candidate_m1_block.Block.Header.Hash().String_short())
	if best_candidate_block != nil && finalize {
		return true, nil
	} else {
		return false, nil
	}

}

// Completable
func (g *Grandpa) Completable(round uint64) (bool, error) {
	ghost, _, err := g.GrandpaGhost(round)
	if err != nil {
		return false, err
	}
	round_state, err := g.GetRoundState(round)
	if err != nil {
		return false, err
	}
	round_state.UpdatePreCommitGraph(false)
	precommit_graph := round_state.PreCommitGraph
	ghost_block, ok := precommit_graph.GetBlockNode(ghost)
	if !ok {
		return false, fmt.Errorf("ghost block not found")
	}
	descendants, _, err := precommit_graph.GetDescendingBlocks(ghost)
	if err != nil {
		return false, err
	}
	for _, descendant := range descendants {
		if descendant.IsAncestor(ghost_block) && descendant.Cumulative_VotseWeight > 0 {
			return true, nil
		}
	}

	return true, nil
}
