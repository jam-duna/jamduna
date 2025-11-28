package types

import (
	"reflect"

	"github.com/colorfulnotion/jam/common"
)

// GRANDPA types used in network protocols and warp sync

// GrandpaVote = Header Hash ++ Slot
type GrandpaVote struct {
	HeaderHash common.Hash `json:"block_hash"`
	Slot       uint32      `json:"slot"`
}

// GrandpaSignedPrecommit = Vote ++ Message Signature ++ Ed25519 Public
type GrandpaSignedPrecommit struct {
	Vote             GrandpaVote      `json:"vote"`
	MessageSignature Ed25519Signature `json:"message_signature"`
	Ed25519Pub       Ed25519Key       `json:"ed25519_pub"`
}

// GrandpaCommit = Header Hash ++ Slot ++ len++[Signed Precommit]
type GrandpaCommit struct {
	HeaderHash common.Hash              `json:"block_hash"`
	Slot       uint32                   `json:"slot"`
	Precommits []GrandpaSignedPrecommit `json:"precommits"`
}

func (c *GrandpaCommit) String() string {
	return ToJSON(c)
}

func (c *GrandpaCommit) ToBytes() ([]byte, error) {
	bytes, err := Encode(c)
	if err != nil {
		return nil, nil
	}
	return bytes, nil
}

func (c *GrandpaCommit) FromBytes(data []byte) error {
	decoded, _, err := Decode(data, reflect.TypeOf(GrandpaCommit{}))
	if err != nil {
		return err
	}
	*c = decoded.(GrandpaCommit)
	return nil
}

// GrandpaJustification = Round Number ++ Set Id ++ Commit ++ Votes Ancestries
type GrandpaJustification struct {
	Round           uint64
	SetId           uint32
	Commit          GrandpaCommit
	VotesAncestries []BlockHeader
}

func (gj *GrandpaJustification) String() string {
	return ToJSON(gj)
}

func (gj *GrandpaJustification) ToBytes() ([]byte, error) {
	bytes, err := Encode(gj)
	if err != nil {
		return nil, nil
	}
	return bytes, nil
}

func (gj *GrandpaJustification) FromBytes(data []byte) error {
	decoded, _, err := Decode(data, reflect.TypeOf(GrandpaJustification{}))
	if err != nil {
		return err
	}
	*gj = decoded.(GrandpaJustification)
	return nil
}
