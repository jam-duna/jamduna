package grandpa

import (
	"encoding/json"
	"reflect"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

// GrandpaState is the state of the grandpa protocol
// https://spec.polkadot.network/sect-finality#defn-grandpa-state
type GrandpaState struct {
	grandpa_authorities []types.Ed25519Key // V
	// in Grandpa Protocol, the authorities are represented by Ed25519 keys
	// non-voter node will not be included in the authorities
	authority_set_id uint64 // id_v
	// the Polkadot Host increments this value by one every time a Scheduled Change or a Forced Change occurs.
	// The authority set ID is an unsigned 64-bit integer.
	voting_round_number uint64 // r
}

func NewGrandpaState(Grandpa_authorities []types.Ed25519Key, authority_set_id uint64, voting_round_number uint64) *GrandpaState {
	return &GrandpaState{
		grandpa_authorities: Grandpa_authorities,
		authority_set_id:    authority_set_id,
		voting_round_number: voting_round_number,
	}
}

func (g *GrandpaState) GetSupermajorityThreshold() int {
	return (2 * len(g.grandpa_authorities)) / 3
}

func (g *GrandpaState) SetAuthorities(authorities []types.Ed25519Key) {
	g.grandpa_authorities = authorities
}

func (g *GrandpaState) SetAuthoritySetId(id uint64) {
	g.authority_set_id = id
}

func (g *GrandpaState) SetVotingRoundNumber(r uint64) {
	g.voting_round_number = r
}

// GrandpaStage is the stage of the grandpa protocol
// we have three stages: Prevote, Precommit, PrimaryPropose
type GrandpaStage byte

const (
	PrevoteStage        GrandpaStage = 0
	PrecommitStage      GrandpaStage = 1
	PrimaryProposeStage GrandpaStage = 2
)

type Message struct {
	Stage GrandpaStage `json:"stage"`
	Vote  Vote         `json:"vote"`
}

// https://spec.polkadot.network/sect-finality#defn-vote
type Vote struct {
	BlockHash common.Hash `json:"block_hash"`
	Slot      uint32      `json:"slot"`
}

func GetGrandpaVote(b *types.Block) Vote {
	return Vote{
		BlockHash: b.Header.Hash(),
		Slot:      b.Header.Slot,
	}
}

// https://spec.polkadot.network/sect-finality#defn-sign-round-vote
type GrandpaVoteUnsignedData struct {
	Message         Message `json:"message"`
	Round           uint64  `json:"round"`
	Authorities_set uint64  `json:"authorities_set"`
}

func (g GrandpaVoteUnsignedData) Bytes() []byte {
	bytes, err := types.Encode(g)
	if err != nil {
		return nil
	}
	return bytes
}

func (g *GrandpaState) GetVoteUnsignedBytes(block *types.Block, stage GrandpaStage) []byte {
	vote := GetGrandpaVote(block)
	unsignedData := GrandpaVoteUnsignedData{
		Message: Message{
			Stage: stage,
			Vote:  vote,
		},
		Round:           g.voting_round_number,
		Authorities_set: g.authority_set_id,
	}
	return unsignedData.Bytes()
}

func (g *GrandpaState) SignVote(ed25519_priv types.Ed25519Priv, block *types.Block, stage GrandpaStage) types.Ed25519Signature {
	data := g.GetVoteUnsignedBytes(block, stage)
	sig := types.Ed25519Sign(ed25519_priv, data)
	return sig
}

// https://spec.polkadot.network/chap-networking#defn-grandpa-vote-msg
type SignMessage struct {
	Message   Message                `json:"message"`
	Signature types.Ed25519Signature `json:"signature"`
	Voter_idx uint64                 `json:"voter_idx"`
}

// https://spec.polkadot.network/chap-networking#defn-grandpa-vote-msg
type VoteMessage struct {
	Round        uint64      `json:"round"`
	AuthoritySet uint64      `json:"authority_set"`
	SignMessage  SignMessage `json:"sign_message"`
}

func (v *VoteMessage) ToBytes() ([]byte, error) {
	bytes, err := types.Encode(v)
	if err != nil {
		return nil, nil
	}
	return bytes, nil
}

func (v *VoteMessage) FromBytes(data []byte) error {
	decoded, _, err := types.Decode(data, reflect.TypeOf(VoteMessage{}))
	if err != nil {
		return err
	}
	*v = decoded.(VoteMessage)
	if err != nil {
		return err
	}
	return nil
}

func (v VoteMessage) String() string {
	// use json.Marshal to convert struct to string
	// be pretty
	jsonBytes, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return ""
	}
	return string(jsonBytes)
}

func (v *VoteMessage) GetUnsignedBytesFromVoteMessage() []byte {
	GrandpaVoteUnsignedData := GrandpaVoteUnsignedData{
		Message: Message{
			Stage: v.SignMessage.Message.Stage,
			Vote:  v.SignMessage.Message.Vote,
		},
		Round:           v.Round,
		Authorities_set: v.AuthoritySet,
	}
	return GrandpaVoteUnsignedData.Bytes()
}

func (g *GrandpaState) VerifyVoteMessage(voteMessage VoteMessage) bool {
	unsigned_bytes := voteMessage.GetUnsignedBytesFromVoteMessage()
	voter_key := g.grandpa_authorities[voteMessage.SignMessage.Voter_idx]
	signature := voteMessage.SignMessage.Signature
	return types.Ed25519Verify(voter_key, unsigned_bytes, signature)
}

type JustificationElement struct {
	Signature types.Ed25519Signature
	Voter     types.Ed25519Key
}

// GRANDPA Compact Justification Format
// https://spec.polkadot.network/chap-networking#defn-grandpa-justifications-compact
type GrandpaCJF struct {
	Votes          []Vote
	Justifications []JustificationElement
}

// https://spec.polkadot.network/chap-networking#defn-grandpa-commit-msg
type CommitMessage struct {
	Round         uint64
	AuthoritySet  uint64
	Vote          Vote
	Justification GrandpaCJF
}

func (c *CommitMessage) ToBytes() ([]byte, error) {
	bytes, err := types.Encode(c)
	if err != nil {
		return nil, nil
	}
	return bytes, nil
}

func (c *CommitMessage) FromBytes(data []byte) error {
	decoded, _, err := types.Decode(data, reflect.TypeOf(CommitMessage{}))
	if err != nil {
		return err
	}
	*c = decoded.(CommitMessage)
	if err != nil {
		return err
	}
	return nil
}

// todo
type NeighborMessage struct {
}

// GRANDPA Catch-up Messages
// todo
type CatchUpRequest struct {
}

// Catch-Up Response Message
// todo
type CatchUpResponse struct {
}
