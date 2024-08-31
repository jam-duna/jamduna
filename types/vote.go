package types

import (
	"encoding/json"
	"fmt"
	"github.com/colorfulnotion/jam/common"
)

type Vote struct {
	Voting    bool   `json:"vote"`      // true for guilty, false for innocent
	Index     uint16 `json:"index"`     // index of the vote in the list of votes (U16 in disputes.asn)
	Signature []byte `json:"signature"` // signature of the vote (ByteArray64 in disputes.asn)
}

type SVote struct {
	Voting    bool   `json:"vote"`      // true for guilty, false for innocent
	Index     uint16 `json:"index"`     // index of the vote in the list of votes (U16 in disputes.asn)
	Signature string `json:"signature"` // signature of the vote (ByteArray64 in disputes.asn)
}

func (t Vote) DeepCopy() (Vote, error) {
	var copiedVote Vote

	// Serialize the original Dispute to JSON
	data, err := json.Marshal(t)
	if err != nil {
		return copiedVote, err
	}

	// Deserialize the JSON back into a new Dispute instance
	err = json.Unmarshal(data, &copiedVote)
	if err != nil {
		return copiedVote, err
	}

	return copiedVote, nil
}

func (v *Vote) Bytes() []byte {
	enc, err := json.Marshal(v)
	if err != nil {
		// Handle the error according to your needs.
		fmt.Println("Error marshaling JSON:", err)
		return nil
	}
	return enc
}

func (v *Vote) Hash() common.Hash {
	data := v.Bytes()
	if data == nil {
		// Handle the error case
		return common.Hash{}
	}
	return common.BytesToHash(common.ComputeHash(data))
}

func (s *SVote) Deserialize() (Vote, error) {
	return Vote{
		Voting:    s.Voting,
		Index:     s.Index,
		Signature: common.FromHex(s.Signature),
	}, nil
}
