package grandpa

import (
	"fmt"
	"reflect"

	"github.com/jam-duna/jamduna/types"
)

/*
CE 150: GRANDPA Commit
This is sent by each voting validator to all other voting validators, indicating that they have finalized a block in a given round.

Validator -> Validator

--> Round Number ++ Set Id ++ Commit
--> FIN
<-- FIN
*/

// https://spec.polkadot.network/chap-networking#defn-grandpa-commit-msg
// GrandpaCommitMessage is used for CE150 network protocol
// Wire format: Round Number ++ Set Id ++ Commit
// Commit = Header Hash ++ Slot ++ len++[Signed Precommit]
// Signed Precommit = Vote ++ Message Signature ++ Ed25519 Public
type GrandpaCommitMessage struct {
	Round  uint64
	SetId  uint32
	Commit types.GrandpaCommit
}

func (gc *GrandpaCommitMessage) String() string {
	return types.ToJSON(gc)
}

func (gc *GrandpaCommitMessage) ToBytes() ([]byte, error) {
	bytes, err := types.Encode(gc)
	if err != nil {
		return nil, nil
	}
	return bytes, nil
}

func (gc *GrandpaCommitMessage) FromBytes(data []byte) error {
	decoded, _, err := types.Decode(data, reflect.TypeOf(GrandpaCommitMessage{}))
	if err != nil {
		return err
	}
	*gc = decoded.(GrandpaCommitMessage)
	if err != nil {
		return err
	}
	return nil
}

func (g *Grandpa) NewFinalCommitMessage(block *types.Block, round uint64) (GrandpaCommitMessage, error) {
	commit := GrandpaCommitMessage{}
	cjf, ok := g.GetCJF(round)
	if !ok {
		return commit, fmt.Errorf("NewFinalCommitMessage: no grandpa justification found, block hash: %v", block.Header.Hash())
	}
	if len(cjf.Votes) == 0 {
		return commit, fmt.Errorf("NewFinalCommitMessage: no votes in grandpa justification for round %d, block hash: %v", round, block.Header.Hash())
	}
	commit.Round = g.GetRoundGrandpaState(round).voting_round_number
	commit.SetId = uint32(g.authority_set_id)

	// Convert grandpa SignedPrecommit to types.GrandpaSignedPrecommit
	precommits := make([]types.GrandpaSignedPrecommit, len(cjf.Votes))
	for i, vote := range cjf.Votes {
		precommits[i] = types.GrandpaSignedPrecommit{
			Vote: types.GrandpaVote{
				HeaderHash: vote.Vote.HeaderHash,
				Slot:       vote.Vote.Slot,
			},
			MessageSignature: vote.MessageSignature,
			Ed25519Pub:       vote.Ed25519Pub,
		}
	}

	commit.Commit = types.GrandpaCommit{
		HeaderHash: cjf.Votes[0].Vote.HeaderHash,
		Slot:       block.Header.Slot,
		Precommits: precommits,
	}
	return commit, nil
}

func (g *Grandpa) ProcessCommitMessage(grandpaCommit GrandpaCommitMessage) error {
	round := grandpaCommit.Round
	setId := grandpaCommit.SetId
	cjf, _ := g.GetCJF(round)
	if cjf == nil {
		return fmt.Errorf("ProcessCommitMessage: no CJF found for round %d", round)
	}

	// Get the authority set for this round
	authorities := g.GetCurrentScheduledAuthoritySet(round)
	authoritySet := make(map[types.Ed25519Key]bool)
	for _, validator := range authorities {
		authoritySet[validator.Ed25519] = true
	}

	weight := 0
	c := grandpaCommit.Commit
	for _, precommit := range c.Precommits {
		// Skip precommits that don't match the commit target
		if precommit.Vote.HeaderHash != c.HeaderHash || precommit.Vote.Slot != c.Slot {
			continue
		}

		// Verify the voter is in the authority set
		if !authoritySet[precommit.Ed25519Pub] {
			continue
		}

		// Verify the Ed25519 signature
		if !VerifyPrecommitSignature(precommit, round, setId) {
			continue
		}

		weight += int(1)
	}

	if weight < g.GetSupermajorityThreshold(round) {
		return fmt.Errorf("ProcessCommitMessage: not enough precommit weight in commit message, got %d, need %d", weight, g.GetSupermajorityThreshold(round))
	}

	return nil
}
