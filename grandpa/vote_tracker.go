package grandpa

import (
	"fmt"
	"sync"

	"github.com/colorfulnotion/jam/common"
)

// vote tracker is used to track the votes of the grandpa protocol
// differentiate the votes of different rounds and stages
type VoteTracker struct {
	VoteMutex    sync.RWMutex
	block_hashes map[uint32]common.Hash
	stage        GrandpaStage
	round        uint64
	votes        map[uint64]map[common.Hash]*SignMessage // v_id -> block hash -> vote
	voter_state  map[uint64]VoterState
}

type VoterState struct {
	State  byte
	Staked uint64
}

const (
	Voted          byte = 2
	Equivocation   byte = 1
	NotParticipate byte = 0
)

// we don't use mutex for below three functions, be careful

// get the state of the voter
func (v *VoteTracker) GetVoterState(voter uint64) VoterState {
	return v.voter_state[voter]
}

// set the state of the voter
func (v *VoteTracker) SetVoterState(voter uint64, state byte) {
	voterState := v.voter_state[voter]
	voterState.State = state
	v.voter_state[voter] = voterState
}

// get the staked amount of the voter
func (v *VoteTracker) GetVoterStaked(voter uint64) uint64 {
	return v.voter_state[voter].Staked
}

// Initialize a new vote tracker
func NewVoteTracker(stage GrandpaStage, round uint64, voter_staked map[uint64]uint64) *VoteTracker {
	voter_state := make(map[uint64]VoterState)
	for i := uint64(0); i < uint64(len(voter_staked)); i++ {
		voter_state[i] = VoterState{
			State:  NotParticipate,
			Staked: voter_staked[i],
		}

	}
	vote_tracker := &VoteTracker{
		block_hashes: make(map[uint32]common.Hash),
		stage:        stage,
		round:        round,
		votes:        make(map[uint64]map[common.Hash]*SignMessage),
		voter_state:  voter_state,
	}
	return vote_tracker
}

// Add a vote to the vote tracker and update the voter state
func (v *VoteTracker) AddVote(voteInfo SignMessage, stage GrandpaStage) error {
	v.VoteMutex.Lock()
	defer v.VoteMutex.Unlock()
	voter := voteInfo.Voter_idx
	if stage != v.stage {
		return fmt.Errorf("AddVote:stage mismatch")
	}
	if _, ok := v.votes[voter]; !ok {
		v.votes[voter] = make(map[common.Hash]*SignMessage)
	}
	if _, exists := v.votes[voter][voteInfo.Message.Vote.BlockHash]; exists {
		return fmt.Errorf("AddVote: vote already exists: block hash %v, round %v, stage %v from v%d", voteInfo.Message.Vote.BlockHash, v.round, v.stage, voter)
	}
	v.block_hashes[voteInfo.Message.Vote.Slot] = voteInfo.Message.Vote.BlockHash
	v.votes[voter][voteInfo.Message.Vote.BlockHash] = &voteInfo
	// update the voter state
	if v.GetVoterState(voter).State == NotParticipate {
		v.voter_state[voter] = VoterState{
			State:  Voted,
			Staked: v.GetVoterStaked(voter),
		}
	} else if v.GetVoterState(voter).State == Voted {
		v.voter_state[voter] = VoterState{
			State:  Equivocation,
			Staked: v.GetVoterStaked(voter),
		}
	}
	return nil
}

// Get the votes of the vote tracker (with staked amount)
func (v *VoteTracker) GetVotesWeight(block common.Hash) uint64 {
	v.VoteMutex.RLock()
	defer v.VoteMutex.RUnlock()
	weight := uint64(0)
	for _, votes := range v.votes {
		for _, vote := range votes {
			if vote.Message.Vote.BlockHash == block {
				if v.GetVoterState(vote.Voter_idx).State != Equivocation {
					weight += v.GetVoterStaked(vote.Voter_idx)
				}
			}
		}
	}
	return weight
}

// voter staked amount, equivocation info voter votes
func (v *VoteTracker) GetEquivocationWeight() uint64 {
	v.VoteMutex.RLock()
	defer v.VoteMutex.RUnlock()
	weight := uint64(0)
	visited := make(map[uint64]bool)
	for _, votes := range v.votes {
		for _, vote := range votes {
			if v.GetVoterState(vote.Voter_idx).State == Equivocation {
				if _, ok := visited[vote.Voter_idx]; ok {
					continue
				}
				weight += v.GetVoterStaked(vote.Voter_idx)
				visited[vote.Voter_idx] = true
			}
		}
	}
	return weight
}

// Get the Primary Propose of the vote tracker
func (v *VoteTracker) GetPrimaryPropose(idx uint64) (common.Hash, error) {
	if v == nil {
		return common.Hash{}, fmt.Errorf("GetPrimaryPropose: nil vote tracker")
	}
	v.VoteMutex.RLock()
	defer v.VoteMutex.RUnlock()
	if len(v.votes[idx]) > 1 {
		return common.Hash{}, fmt.Errorf("GetPrimaryPropose: multiple votes from the primary propose")
	} else if len(v.votes[idx]) == 0 {
		return common.Hash{}, fmt.Errorf("GetPrimaryPropose: no vote found")
	}
	for hash, vote := range v.votes[idx] {
		if hash != vote.Message.Vote.BlockHash {
			return common.Hash{}, fmt.Errorf("GetPrimaryPropose: hash mismatch")
		}
		return hash, nil
	}
	return common.Hash{}, fmt.Errorf("GetPrimaryPropose: no vote found")
}
