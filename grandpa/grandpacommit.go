package grandpa

import (
	"fmt"
	"reflect"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
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
// Commit = Header Hash ++ Slot ++ len++[Signed Precommit]
// Signed Precommit = Vote ++ Message Signature ++ Ed25519 Public
type GrandpaCommit struct {
	Round      uint64
	SetId      uint32
	HeaderHash common.Hash       `json:"block_hash"`
	Slot       uint32            `json:"slot"`
	Precommits []SignedPrecommit `json:"precommits"`
}

func (c *GrandpaCommit) ToBytes() ([]byte, error) {
	bytes, err := types.Encode(c)
	if err != nil {
		return nil, nil
	}
	return bytes, nil
}

func (c *GrandpaCommit) FromBytes(data []byte) error {
	decoded, _, err := types.Decode(data, reflect.TypeOf(GrandpaCommit{}))
	if err != nil {
		return err
	}
	*c = decoded.(GrandpaCommit)
	if err != nil {
		return err
	}
	return nil
}

func (g *Grandpa) NewFinalCommitMessage(block *types.Block, round uint64) (GrandpaCommit, error) {
	commit := GrandpaCommit{}
	cjf, ok := g.GetCJF(round)
	if !ok {
		return commit, fmt.Errorf("NewFinalCommitMessage: no grandpa justification found, block hash: %v", block.Header.Hash())
	}
	if len(cjf.Votes) == 0 {
		return commit, fmt.Errorf("NewFinalCommitMessage: no votes in grandpa justification for round %d, block hash: %v", round, block.Header.Hash())
	}
	commit.Round = g.GetRoundGrandpaState(round).voting_round_number
	commit.SetId = uint32(g.authority_set_id)
	commit.HeaderHash = cjf.Votes[0].Vote.HeaderHash
	commit.Precommits = cjf.Votes
	return commit, nil
}

func (g *Grandpa) ProcessCommitMessage(commit GrandpaCommit) error {
	round := commit.Round
	cjf, _ := g.GetCJF(round)
	if cjf == nil {
		return fmt.Errorf("ProcessCommitMessage: no CJF found for round %d", round)
	}
	weight := 0
	for _, precommit := range commit.Precommits {
		if precommit.Vote.HeaderHash != commit.HeaderHash || precommit.Vote.Slot != commit.Slot {
			// invalid precommit vote
			continue
		}
		// TODO: verify the signature against the authority set
		weight += int(g.Voter_Staked[types.Ed25519Key(precommit.Ed25519Pub)])
	}

	if weight < g.GetSupermajorityThreshold(uint64(commit.SetId)) {
		return fmt.Errorf("ProcessCommitMessage: not enough precommit weight in commit message, got %d, need %d", weight, g.GetSupermajorityThreshold(uint64(commit.SetId)))
	}

	return nil
}
