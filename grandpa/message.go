package grandpa

import (
	"fmt"

	"github.com/colorfulnotion/jam/types"
)

func (g *Grandpa) NewPrimaryVoteMessage(block *types.Block, round uint64) VoteMessage {
	vote := VoteMessage{}
	vote.SignMessage.Message.Stage = PrimaryProposeStage
	vote.SignMessage.Message.Vote = GetGrandpaVote(block)
	vote.SignMessage.Signature = g.GetRoundGrandpaState(round).SignVote(g.selfkey, block, PrimaryProposeStage)
	vote.SignMessage.Voter_idx = uint64(g.GetSelfVoterIndex(round))
	vote.Round = g.GetRoundGrandpaState(round).voting_round_number
	vote.AuthoritySet = g.GetRoundGrandpaState(round).authority_set_id
	return vote
}

func (g *Grandpa) NewPrevoteVoteMessage(block *types.Block, round uint64) VoteMessage {
	vote := VoteMessage{}
	vote.SignMessage.Message.Stage = PrevoteStage
	vote.SignMessage.Message.Vote = GetGrandpaVote(block)
	vote.SignMessage.Signature = g.GetRoundGrandpaState(round).SignVote(g.selfkey, block, PrevoteStage)
	vote.SignMessage.Voter_idx = uint64(g.GetSelfVoterIndex(round))
	vote.Round = g.GetRoundGrandpaState(round).voting_round_number
	vote.AuthoritySet = g.GetRoundGrandpaState(round).authority_set_id
	return vote
}
func (g *Grandpa) NewPrecommitVoteMessage(block *types.Block, round uint64) VoteMessage {
	vote := VoteMessage{}
	vote.SignMessage.Message.Stage = PrecommitStage
	vote.SignMessage.Message.Vote = GetGrandpaVote(block)
	vote.SignMessage.Signature = g.GetRoundGrandpaState(round).SignVote(g.selfkey, block, PrecommitStage)
	vote.SignMessage.Voter_idx = uint64(g.GetSelfVoterIndex(round))
	vote.Round = g.GetRoundGrandpaState(round).voting_round_number
	vote.AuthoritySet = g.GetRoundGrandpaState(round).authority_set_id
	return vote
}

func (g *Grandpa) NewFinalCommitMessage(block *types.Block, round uint64) (CommitMessage, error) {
	commit := CommitMessage{}
	cjf, ok := g.GetCJF(round)
	if !ok {
		return commit, fmt.Errorf("NewFinalCommitMessage: no grandpa justification found, block hash: %v", block.Header.Hash())
	}
	commit.Justification = *cjf
	commit.Vote = GetGrandpaVote(block)
	commit.Round = g.GetRoundGrandpaState(round).voting_round_number
	commit.AuthoritySet = g.GetRoundGrandpaState(round).authority_set_id
	return commit, nil
}

// process the PreVote message and add it to the tracker
func (g *Grandpa) ProcessPreVoteMessage(prevote VoteMessage) error {
	g.InitTrackers(prevote.Round)
	round := prevote.Round
	grandpa_state := g.GetRoundGrandpaState(round)
	if grandpa_state == nil {
		g.InitRoundState(round, g.block_tree.Copy())
		grandpa_state = g.GetRoundGrandpaState(round)
	}
	if prevote.SignMessage.Message.Stage != PrevoteStage {
		return fmt.Errorf("message stage error")
	}
	if !grandpa_state.VerifyVoteMessage(prevote) {
		return fmt.Errorf("message signature error")
	} else {
		// add the vote to the tracker
		vote_info := prevote.SignMessage
		stage := prevote.SignMessage.Message.Stage
		tracker := g.GetVoteTracker(round, stage)
		tracker.AddVote(vote_info, stage)
		return nil
	}
}

// process the PreCommit message and add it to the tracker
func (g *Grandpa) ProcessPreCommitMessage(precommit VoteMessage) error {
	g.InitTrackers(precommit.Round)
	round := precommit.Round
	grandpa_state := g.GetRoundGrandpaState(round)
	if grandpa_state == nil {
		g.InitRoundState(round, g.block_tree.Copy())
		grandpa_state = g.GetRoundGrandpaState(round)
	}
	if precommit.SignMessage.Message.Stage != PrecommitStage {
		return fmt.Errorf("message stage error")
	}

	if !grandpa_state.VerifyVoteMessage(precommit) {
		return fmt.Errorf("message signature error")
	} else {

		// add the vote to the tracker
		vote := precommit.SignMessage
		stage := precommit.SignMessage.Message.Stage
		tracker := g.GetVoteTracker(round, stage)
		tracker.AddVote(vote, stage)
		g.ProcessNewCommitMessageCJF(precommit)
		return nil
	}
}

// process the PrimaryPropose message and add it to the tracker
func (g *Grandpa) ProcessPrimaryProposeMessage(primary_propose VoteMessage) error {
	g.InitTrackers(primary_propose.Round)
	round := primary_propose.Round
	grandpa_state := g.GetRoundGrandpaState(round)
	if grandpa_state == nil {
		g.InitRoundState(round, g.block_tree.Copy())
		grandpa_state = g.GetRoundGrandpaState(round)
	}
	if primary_propose.SignMessage.Message.Stage != PrimaryProposeStage {
		return fmt.Errorf("message stage error")
	}
	if !grandpa_state.VerifyVoteMessage(primary_propose) {
		return fmt.Errorf("message signature error")
	} else {
		// add the vote to the tracker
		vote := primary_propose.SignMessage
		stage := primary_propose.SignMessage.Message.Stage
		tracker := g.GetVoteTracker(round, stage)
		err := tracker.AddVote(vote, stage)
		if err != nil {
			return err
		}
		return nil
	}
}

// process the NewCommitMessageCJF message and add it to the cjf (used by non-voter ??)
func (g *Grandpa) ProcessNewCommitMessageCJF(commitMessage VoteMessage) error {
	round := commitMessage.Round
	if commitMessage.SignMessage.Message.Stage != PrecommitStage {
		return fmt.Errorf("message stage error")
	}
	vote := commitMessage.SignMessage.Message.Vote
	voter_idx := commitMessage.SignMessage.Voter_idx
	cjf, _ := g.GetCJF(round)
	if cjf == nil {
		cjf = &GrandpaCJF{
			Justifications: make([]JustificationElement, len(g.GetCurrentScheduledAuthoritySet(round).Authorities)),
			Votes:          make([]Vote, len(g.GetCurrentScheduledAuthoritySet(round).Authorities)),
		}
	}
	cjf.Votes[voter_idx] = vote

	justification := JustificationElement{
		Signature: commitMessage.SignMessage.Signature,
		Voter:     g.GetCurrentScheduledAuthoritySet(round).Authorities[voter_idx],
	}
	cjf.Justifications[voter_idx] = justification
	return nil
}
