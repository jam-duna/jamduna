package grandpa

import (
	"fmt"
	"sync"

	"github.com/colorfulnotion/jam/types"
)

type RoundState struct {
	PreVoteGraph   *types.BlockTree
	PreCommitGraph *types.BlockTree

	GrandpaState   *GrandpaState
	VoteTracker    *sync.Map   // round -> stage -> tracker
	GrandpaCJF_Map *GrandpaCJF // use block hash to hold the grandpa justification, will be used by precommit stage
}

// Get the vote tracker for the stage
func (r *RoundState) GetVoteTracker(stage GrandpaStage) *VoteTracker {
	tracker, ok := r.VoteTracker.Load(stage)
	if !ok {
		return nil
	}
	return tracker.(*VoteTracker)
}

// Get the weight of the unvoted voters
func (r *RoundState) GetUnvotedWeight(stage GrandpaStage) uint64 {
	tracker := r.GetVoteTracker(stage)
	if tracker == nil {
		return 0
	}
	tracker.VoteMutex.RLock()
	defer tracker.VoteMutex.RUnlock()
	weight := uint64(0)
	for _, voter_state := range tracker.voter_state {
		if voter_state.State == NotParticipate {
			weight += voter_state.Staked
		}
	}
	return weight
}

// Definition 87. Set of Total Observed Votes (with_unvoted = false)
// Definition 88. Set of Total Potential Votes (with_unvoted = true)
func (r *RoundState) UpdatePreVoteGraph(with_unvoted bool) error {
	stage := PrevoteStage
	tracker := r.GetVoteTracker(stage)

	// Get equivocation weight (0 if tracker is nil)
	eq_staked := uint64(0)
	if tracker != nil {
		eq_staked = tracker.GetEquivocationWeight()
	}

	block_hashes := r.PreVoteGraph.GetBlockHashes()
	for _, block_hash := range block_hashes {
		weight := uint64(0)
		if tracker != nil {
			weight = tracker.GetVotesWeight(block_hash)
		}
		block_node, ok := r.PreVoteGraph.GetBlockNode(block_hash)
		if !ok {
			return fmt.Errorf("UpdatePreVoteGraph_P: block node not found for block hash %v", block_hash)
		}
		block_node.SetVotesWeight(weight)
	}
	uw := uint64(0)
	if with_unvoted {
		uw = r.GetUnvotedWeight(stage)
	}
	r.PreVoteGraph.UpdateCumulateVotesWeight(uw, eq_staked)
	return nil
}

// Definition 87. Set of Total Observed Votes (with_unvoted = false)
// Definition 88. Set of Total Potential Votes (with_unvoted = true)
func (r *RoundState) UpdatePreCommitGraph(with_unvoted bool) error {
	stage := PrecommitStage
	tracker := r.GetVoteTracker(stage)

	// Get equivocation weight (0 if tracker is nil)
	eq_staked := uint64(0)
	if tracker != nil {
		eq_staked = tracker.GetEquivocationWeight()
	}

	block_hashes := r.PreCommitGraph.GetBlockHashes()
	for _, block_hash := range block_hashes {
		weight := uint64(0)
		if tracker != nil {
			weight = tracker.GetVotesWeight(block_hash)
		}
		block_node, ok := r.PreCommitGraph.GetBlockNode(block_hash)
		if !ok {
			return fmt.Errorf("UpdatePreCommitGraph_P: block node not found for block hash %v", block_hash)
		}
		block_node.SetVotesWeight(weight)

	}
	uw := uint64(0)
	if with_unvoted {
		uw = r.GetUnvotedWeight(stage)
	}
	r.PreCommitGraph.UpdateCumulateVotesWeight(uw, eq_staked)
	return nil
}
