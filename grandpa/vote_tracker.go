package grandpa

import (
	"fmt"
	"sync"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

// vote tracker is used to track the votes of the grandpa protocol
// differentiate the votes of different rounds and stages
type VoteTracker struct {
	VoteMutex    sync.RWMutex
	block_hashes map[uint32]common.Hash
	stage        GrandpaStage
	round        uint64
	votes        map[types.Ed25519Key]map[common.Hash]*SignedMessage // v_id -> block hash -> vote
	voter_state  map[types.Ed25519Key]VoterState
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
func (v *VoteTracker) GetVoterState(key types.Ed25519Key) VoterState {
	return v.voter_state[key]
}

// set the state of the voter
func (v *VoteTracker) SetVoterState(voter types.Ed25519Key, state byte) {
	voterState := v.voter_state[voter]
	voterState.State = state
	v.voter_state[voter] = voterState
}

// get the staked amount of the voter
func (v *VoteTracker) GetVoterStaked(voter types.Ed25519Key) uint64 {
	return v.voter_state[voter].Staked
}

// Initialize a new vote tracker
func NewVoteTracker(stage GrandpaStage, round uint64, voter_staked map[types.Ed25519Key]uint64) *VoteTracker {
	voter_state := make(map[types.Ed25519Key]VoterState)
	for key, staked := range voter_staked {
		voter_state[key] = VoterState{
			State:  NotParticipate,
			Staked: staked,
		}

	}
	vote_tracker := &VoteTracker{
		block_hashes: make(map[uint32]common.Hash),
		stage:        stage,
		round:        round,
		votes:        make(map[types.Ed25519Key]map[common.Hash]*SignedMessage),
		voter_state:  voter_state,
	}
	return vote_tracker
}

// Add a vote to the vote tracker and update the voter state
func (v *VoteTracker) AddVote(voteInfo SignedMessage, stage GrandpaStage) error {
	v.VoteMutex.Lock()
	defer v.VoteMutex.Unlock()
	voter := voteInfo.Ed25519Pub
	if stage != v.stage {
		return fmt.Errorf("AddVote:stage mismatch")
	}
	if _, ok := v.votes[voter]; !ok {
		v.votes[voter] = make(map[common.Hash]*SignedMessage)
	}
	if _, exists := v.votes[voter][voteInfo.Message.Vote.HeaderHash]; exists {
		return fmt.Errorf("AddVote: vote already exists: block hash %v, round %v, stage %v from v%d", voteInfo.Message.Vote.HeaderHash, v.round, v.stage, voter)
	}
	v.block_hashes[voteInfo.Message.Vote.Slot] = voteInfo.Message.Vote.HeaderHash
	v.votes[voter][voteInfo.Message.Vote.HeaderHash] = &voteInfo
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
			if vote.Message.Vote.HeaderHash == block {
				if v.GetVoterState(vote.Ed25519Pub).State != Equivocation {
					weight += v.GetVoterStaked(vote.Ed25519Pub)
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
	visited := make(map[types.Ed25519Key]bool)
	for _, votes := range v.votes {
		for _, vote := range votes {
			if v.GetVoterState(vote.Ed25519Pub).State == Equivocation {
				if _, ok := visited[vote.Ed25519Pub]; ok {
					continue
				}
				weight += v.GetVoterStaked(vote.Ed25519Pub)
				visited[vote.Ed25519Pub] = true
			}
		}
	}
	return weight
}

// Get the Primary Propose of the vote tracker
func (v *VoteTracker) GetPrimaryPropose(ed25519_pub types.Ed25519Key) (common.Hash, error) {
	if v == nil {
		return common.Hash{}, fmt.Errorf("GetPrimaryPropose: nil vote tracker")
	}
	v.VoteMutex.RLock()
	defer v.VoteMutex.RUnlock()
	if len(v.votes[ed25519_pub]) > 1 {
		return common.Hash{}, fmt.Errorf("GetPrimaryPropose: multiple votes from the primary propose")
	} else if len(v.votes[ed25519_pub]) == 0 {
		return common.Hash{}, fmt.Errorf("GetPrimaryPropose: no vote found")
	}
	for hash, vote := range v.votes[ed25519_pub] {
		if hash != vote.Message.Vote.HeaderHash {
			return common.Hash{}, fmt.Errorf("GetPrimaryPropose: hash mismatch")
		}
		return hash, nil
	}
	return common.Hash{}, fmt.Errorf("GetPrimaryPropose: no vote found")
}

// GetAllVotes returns all votes in the tracker
// Caller should acquire VoteMutex before calling this
func (v *VoteTracker) GetAllVotes() map[types.Ed25519Key]map[common.Hash]*SignedMessage {
	return v.votes
}
