package grandpa

import (
	"fmt"
	"reflect"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

// GrandpaStage is the stage of the grandpa protocol
// we have three stages: Prevote, Precommit, PrimaryPropose
type GrandpaStage byte

// Message Type = 0 (Prevote) OR 1 (Precommit) OR 2 (PrimaryPropose) (Single byte)
const (
	PrevoteStage        GrandpaStage = 0
	PrecommitStage      GrandpaStage = 1
	PrimaryProposeStage GrandpaStage = 2
)

// Message = Message Type ++ Vote
type Message struct {
	Stage GrandpaStage `json:"stage"`
	Vote  Vote         `json:"vote"`
}

// Vote = Header Hash ++ Slot
type Vote struct {
	HeaderHash common.Hash `json:"block_hash"`
	Slot       uint32      `json:"slot"`
}
type SignedPrevote struct {
	Vote             Vote                   `json:"vote"`
	MessageSignature types.Ed25519Signature `json:"message_signature"`
	Ed25519Pub       types.Ed25519Key       `json:"ed25519_pub"`
}

type SignedPrecommit struct {
	Vote             Vote                   `json:"vote"`
	MessageSignature types.Ed25519Signature `json:"message_signature"`
	Ed25519Pub       types.Ed25519Key       `json:"ed25519_pub"`
}

// Message ++ Message Signature ++ Ed25519 Public
type SignedMessage struct {
	Message    Message                `json:"message"`
	Signature  types.Ed25519Signature `json:"signature"`
	Ed25519Pub types.Ed25519Key       `json:"ed25519_pub"`
}

type GrandpaVote struct {
	Round         uint64        `json:"round"`
	SetId         uint32        `json:"set_id"`
	SignedMessage SignedMessage `json:"sign_message"`
}

func (v *GrandpaVote) GetVoteType() string {
	stage := v.SignedMessage.Message.Stage
	switch stage {
	case PrevoteStage:
		return "Prevote"
	case PrecommitStage:
		return "Precommit"
	case PrimaryProposeStage:
		return "Primary"
	}
	return "Unknown"
}

func (v *GrandpaVote) ToBytes() ([]byte, error) {
	bytes, err := types.Encode(v)
	if err != nil {
		return nil, nil
	}
	return bytes, nil
}

func (v *GrandpaVote) FromBytes(data []byte) error {
	decoded, _, err := types.Decode(data, reflect.TypeOf(GrandpaVote{}))
	if err != nil {
		return err
	}
	*v = decoded.(GrandpaVote)
	if err != nil {
		return err
	}
	return nil
}

func (v GrandpaVote) String() string {
	return types.ToJSON(v)
}

func (v *GrandpaVote) GetUnsignedBytesFromVoteMessage() []byte {
	GrandpaVoteUnsignedData := GrandpaVoteUnsignedData{
		Message: Message{
			Stage: v.SignedMessage.Message.Stage,
			Vote:  v.SignedMessage.Message.Vote,
		},
		Round:           v.Round,
		Authorities_set: uint64(v.SetId),
	}
	return getGrandpaVoteUnsignedBytes(GrandpaVoteUnsignedData)
}

func (g *Grandpa) NewPrimaryVoteMessage(block *types.Block, round uint64) GrandpaVote {
	vote := GrandpaVote{}
	vote.SignedMessage.Message.Stage = PrimaryProposeStage
	vote.SignedMessage.Message.Vote = GetGrandpaVote(block)
	vote.SignedMessage.Signature = g.GetRoundGrandpaState(round).SignVote(g.selfkey.Ed25519Secret[:], block, PrimaryProposeStage, g.authority_set_id)
	vote.SignedMessage.Ed25519Pub = g.selfkey.Ed25519Pub
	vote.Round = g.GetRoundGrandpaState(round).voting_round_number
	vote.SetId = uint32(g.authority_set_id)
	return vote
}

func (g *Grandpa) NewPrevoteVoteMessage(block *types.Block, round uint64) GrandpaVote {
	vote := GrandpaVote{}
	vote.SignedMessage.Message.Stage = PrevoteStage
	vote.SignedMessage.Message.Vote = GetGrandpaVote(block)
	vote.SignedMessage.Signature = g.GetRoundGrandpaState(round).SignVote(g.selfkey.Ed25519Secret[:], block, PrevoteStage, g.authority_set_id)
	vote.SignedMessage.Ed25519Pub = g.selfkey.Ed25519Pub
	vote.Round = g.GetRoundGrandpaState(round).voting_round_number
	vote.SetId = uint32(g.authority_set_id)
	return vote
}
func (g *Grandpa) NewPrecommitVoteMessage(block *types.Block, round uint64) GrandpaVote {
	vote := GrandpaVote{}
	vote.SignedMessage.Message.Stage = PrecommitStage
	vote.SignedMessage.Message.Vote = GetGrandpaVote(block)
	vote.SignedMessage.Signature = g.GetRoundGrandpaState(round).SignVote(g.selfkey.Ed25519Secret[:], block, PrecommitStage, g.authority_set_id)
	vote.SignedMessage.Ed25519Pub = g.selfkey.Ed25519Pub
	vote.Round = g.GetRoundGrandpaState(round).voting_round_number
	vote.SetId = uint32(g.authority_set_id)
	return vote
}

// process the PreVote message and add it to the tracker
func (g *Grandpa) ProcessPreVoteMessage(prevote GrandpaVote) error {
	g.InitTrackers(prevote.Round)
	round := prevote.Round
	grandpa_state := g.GetRoundGrandpaState(round)
	if grandpa_state == nil {
		g.InitRoundState(round, g.block_tree.Copy())
		grandpa_state = g.GetRoundGrandpaState(round)
	}
	if prevote.SignedMessage.Message.Stage != PrevoteStage {
		return fmt.Errorf("message stage error")
	}
	if !grandpa_state.VerifyVoteMessage(prevote) {
		return fmt.Errorf("message signature error %v", prevote.String())
	} else {
		// add the vote to the tracker
		vote_info := prevote.SignedMessage
		stage := prevote.SignedMessage.Message.Stage
		tracker := g.GetVoteTracker(round, stage)
		if tracker != nil {
			tracker.AddVote(vote_info, stage)
		}
		return nil
	}
}

// process the PreCommit message and add it to the tracker
func (g *Grandpa) ProcessPreCommitMessage(precommit GrandpaVote) error {
	g.InitTrackers(precommit.Round)
	round := precommit.Round
	grandpa_state := g.GetRoundGrandpaState(round)
	if grandpa_state == nil {
		g.InitRoundState(round, g.block_tree.Copy())
		grandpa_state = g.GetRoundGrandpaState(round)
	}
	if precommit.SignedMessage.Message.Stage != PrecommitStage {
		return fmt.Errorf("message stage error")
	}

	if !grandpa_state.VerifyVoteMessage(precommit) {
		return fmt.Errorf("message signature error")
	}
	err := g.ProcessNewCommitMessageCJF(precommit)
	if err != nil {
		return fmt.Errorf("ProcessPreCommitMessage: %w", err)
	}
	// add the vote to the tracker
	vote := precommit.SignedMessage
	stage := precommit.SignedMessage.Message.Stage
	tracker := g.GetVoteTracker(round, stage)
	if tracker != nil {
		tracker.AddVote(vote, stage)
	}
	return nil
}

// process the PrimaryPropose message and add it to the tracker
func (g *Grandpa) ProcessPrimaryProposeMessage(primary_propose GrandpaVote) error {
	g.InitTrackers(primary_propose.Round)
	round := primary_propose.Round
	grandpa_state := g.GetRoundGrandpaState(round)
	if grandpa_state == nil {
		g.InitRoundState(round, g.block_tree.Copy())
		grandpa_state = g.GetRoundGrandpaState(round)
	}
	if primary_propose.SignedMessage.Message.Stage != PrimaryProposeStage {
		return fmt.Errorf("message stage error")
	}
	if !grandpa_state.VerifyVoteMessage(primary_propose) {
		return fmt.Errorf("message signature error")
	}
	// add the vote to the tracker
	vote := primary_propose.SignedMessage
	stage := primary_propose.SignedMessage.Message.Stage
	tracker := g.GetVoteTracker(round, stage)
	if tracker != nil {
		err := tracker.AddVote(vote, stage)
		if err != nil {
			return err
		}
	}
	return nil

}

// process the NewCommitMessageCJF message and add it to the cjf (used by non-voter ??)
func (g *Grandpa) ProcessNewCommitMessageCJF(commitMessage GrandpaVote) error {
	round := commitMessage.Round
	if commitMessage.SignedMessage.Message.Stage != PrecommitStage {
		return fmt.Errorf("message stage error")
	}
	vote := commitMessage.SignedMessage.Message.Vote
	voter := commitMessage.SignedMessage.Ed25519Pub
	cjf, _ := g.GetCJF(round)
	if cjf == nil {
		cjf = &GrandpaCJF{
			Justifications: make([]JustificationElement, len(g.GetCurrentScheduledAuthoritySet(round))),
			Votes:          make([]SignedPrecommit, len(g.GetCurrentScheduledAuthoritySet(round))),
		}
		// Save the newly created CJF back to RoundState
		g.RoundStateMutex.Lock()
		if g.RoundState[round] != nil {
			g.RoundState[round].GrandpaCJF_Map = cjf
		}
		g.RoundStateMutex.Unlock()
	}

	voter_idx := g.GetCurrentAuthoritySetIndex(round, voter)

	if voter_idx == -1 {
		return fmt.Errorf("voter not found in authority set")
	}
	g.RoundStateMutex.Lock()
	defer g.RoundStateMutex.Unlock()
	cjf.Votes[voter_idx] = SignedPrecommit{
		Vote:             vote,
		MessageSignature: commitMessage.SignedMessage.Signature,
		Ed25519Pub:       voter,
	}

	justification := JustificationElement{
		Signature: commitMessage.SignedMessage.Signature,
		Voter:     voter,
	}
	cjf.Justifications[voter_idx] = justification
	return nil
}
