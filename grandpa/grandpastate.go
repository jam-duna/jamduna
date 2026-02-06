package grandpa

import (
	"reflect"

	"github.com/jam-duna/jamduna/types"
)

/*
CE 151: GRANDPA State

This is sent by each voting validator to all other voting validators and informs them of the latest round it is participating in.

Validator -> Validator

--> Round Number ++ Set Id ++ Slot
--> FIN
<-- FIN
*/

type GrandpaStateMessage struct {
	Round uint64
	SetId uint32
	Slot  uint32
}

func (s *GrandpaStateMessage) String() string {
	return types.ToJSON(s)
}

func (s *GrandpaStateMessage) ToBytes() ([]byte, error) {
	bytes, err := types.Encode(s)
	if err != nil {
		return nil, nil
	}
	return bytes, nil
}

func (s *GrandpaStateMessage) FromBytes(data []byte) error {
	decoded, _, err := types.Decode(data, reflect.TypeOf(GrandpaStateMessage{}))
	if err != nil {
		return err
	}
	*s = decoded.(GrandpaStateMessage)
	if err != nil {
		return err
	}
	return nil
}

type JustificationElement struct {
	Signature types.Ed25519Signature
	Voter     types.Ed25519Key
}

// GRANDPA Compact Justification Format
// https://spec.polkadot.network/chap-networking#defn-grandpa-justifications-compact
type GrandpaCJF struct {
	Votes          []SignedPrecommit
	Justifications []JustificationElement
}

// https://spec.polkadot.network/sect-finality#defn-sign-round-vote
type GrandpaVoteUnsignedData struct {
	Message         Message `json:"message"`
	Round           uint64  `json:"round"`
	Authorities_set uint64  `json:"authorities_set"`
}

func getGrandpaVoteUnsignedBytes(data GrandpaVoteUnsignedData) []byte {
	bytes := data.Bytes()
	if bytes == nil {
		return nil
	}
	return append([]byte("jam_grandpa_vote"), bytes...)
}

type GrandpaState struct {
	// in Grandpa Protocol, the authorities are represented by Ed25519 keys
	grandpa_authorities []types.Validator // V
	voting_round_number uint64            // r
}

func NewGrandpaState(authorities []types.Validator, voting_round_number uint64) *GrandpaState {
	return &GrandpaState{
		grandpa_authorities: authorities,
		voting_round_number: voting_round_number,
	}
}

func (g *GrandpaState) GetSupermajorityThreshold() int {
	return (2 * len(g.grandpa_authorities)) / 3
}

func (g *GrandpaState) SetAuthorities(authorities []types.Validator) {
	g.grandpa_authorities = authorities
}

func (g *GrandpaState) SetVotingRoundNumber(r uint64) {
	g.voting_round_number = r
}

func GetGrandpaVote(b *types.Block) Vote {
	return Vote{
		HeaderHash: b.Header.Hash(),
		Slot:       b.Header.Slot,
	}
}

func (g GrandpaVoteUnsignedData) Bytes() []byte {
	bytes, err := types.Encode(g)
	if err != nil {
		return nil
	}
	return bytes
}

func (g *GrandpaState) GetVoteUnsignedBytes(block *types.Block, stage GrandpaStage, setID uint32) []byte {
	vote := GetGrandpaVote(block)
	unsignedData := GrandpaVoteUnsignedData{
		Message: Message{
			Stage: stage,
			Vote:  vote,
		},
		Round:           g.voting_round_number,
		Authorities_set: uint64(setID),
	}
	return getGrandpaVoteUnsignedBytes(unsignedData)
}

func (g *GrandpaState) SignVote(ed25519_priv types.Ed25519Priv, block *types.Block, stage GrandpaStage, setID uint32) types.Ed25519Signature {
	data := g.GetVoteUnsignedBytes(block, stage, setID)
	sig := types.Ed25519Sign(ed25519_priv, data)
	return sig
}

func (g *GrandpaState) VerifyVoteMessage(voteMessage GrandpaVote) bool {
	unsigned_bytes := voteMessage.GetUnsignedBytesFromVoteMessage()
	voter_key := voteMessage.SignedMessage.Ed25519Pub
	signature := voteMessage.SignedMessage.Signature
	return types.Ed25519Verify(voter_key, unsigned_bytes, signature)
}

func (g *Grandpa) ProcessStateMessage(state GrandpaStateMessage) error {
	return nil
}
