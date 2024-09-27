package types

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/colorfulnotion/jam/common"
)

type SOffendersMark struct {
	OffenderKey []string `json:"offender_mark"`
}

type VerdictMarker struct {
	WorkReportHash []common.Hash `json:"verdict_mark"` // WorkReportHash (ByteArray32 in disputes.asn)
}

type OffenderMarker struct {
	OffenderKey []Ed25519Key `json:"offender_mark"`
}

type SOffenderMarker struct {
	OffenderKey []string `json:"offender_mark"`
}

/*
Section 10.2.  The disputes extrinsic, ${\bf E}_D$, may contain one or more verdicts ${\bf v}$.

Dispute` ${\bf E}_D$:
*/
// Disputes represents a one or or more verdicts.
type Dispute struct {
	Verdict []Verdict `json:"verdicts"`
	Culprit []Culprit `json:"culprits"`
	Fault   []Fault   `json:"faults"`
}

type Verdict struct {
	Target common.Hash                   `json:"target"`
	Epoch  uint32                        `json:"age"`
	Votes  [ValidatorsSuperMajority]Vote `json:"votes"`
}

func (v *Verdict) Verify(validators []Validator) error {
	target := v.Target
	for _, vote := range v.Votes {
		signtext := vote.UnsignedBytesWithSalt(target)
		if Ed25519Verify(validators[vote.Index].Ed25519, signtext, vote.Signature) == false {
			return errors.New(fmt.Sprintf("Invalid signature for vote %d", vote.Index))
		}
	}
	return nil
}

func (v *Vote) UnsignedBytesWithSalt(target common.Hash) []byte {
	signtext := target.Bytes()
	if v.Voting {
		signtext = append([]byte(X_True), signtext...)
	} else {
		signtext = append([]byte(X_False), signtext...)
	}
	return signtext
}

type Culprit struct {
	Target    common.Hash      `json:"target"`
	Key       Ed25519Key       `json:"key"`
	Signature Ed25519Signature `json:"signature"`
}

func (c *Culprit) Verify() bool {
	signtext := c.UnsignedBytesWithSalt()
	return Ed25519Verify(c.Key, signtext, c.Signature)
}

func (c *Culprit) UnsignedBytesWithSalt() []byte {
	signtext := c.Target.Bytes()
	signtext = append([]byte(X_G), signtext...)
	return signtext
}

type Fault struct {
	Target    common.Hash      `json:"target"`
	Voting    bool             `json:"vote"`
	Key       Ed25519Key       `json:"key"`
	Signature Ed25519Signature `json:"signature"`
}

func (f *Fault) Verify() bool {
	signtext := f.UnsignedBytesWithSalt()
	return Ed25519Verify(f.Key, signtext, f.Signature)
}

func (f *Fault) UnsignedBytesWithSalt() []byte {
	signtext := f.Target.Bytes()
	if f.Voting {
		signtext = append([]byte(X_True), signtext...)
	} else {
		signtext = append([]byte(X_False), signtext...)
	}
	return signtext
}

func (t Dispute) DeepCopy() (Dispute, error) {
	var copiedDispute Dispute

	// Serialize the original Dispute to JSON
	data := Encode(t)

	// Deserialize the JSON back into a new Dispute instance
	decoded, _ := Decode(data, reflect.TypeOf(Dispute{}))
	copiedDispute = decoded.(Dispute)

	return copiedDispute, nil
}

// Bytes returns the bytes of the Dispute
func (a *Dispute) Bytes() []byte {
	enc := Encode(a)
	return enc
}

func (a *Dispute) Hash() common.Hash {
	data := a.Bytes()
	if data == nil {
		// Handle the error case
		return common.Hash{}
	}
	return common.Blake2Hash(data)
}

func (a *Dispute) Print() {
	fmt.Printf("Dispute:\n")
	fmt.Printf("  Verdicts:\n")
	for _, v := range a.Verdict {
		fmt.Printf("    - Target:    %04x\n", v.Target)
		fmt.Printf("      Epoch:     %d\n", v.Epoch)
		fmt.Printf("      Votes:\n")
		for _, vote := range v.Votes {
			fmt.Printf("        -- Validator:  %d\n", vote.Index)
			fmt.Printf("           Vote:       %t\n", vote.Voting)
			fmt.Printf("           Signature:  %x\n", vote.Signature)
		}
	}
	fmt.Printf("  Culprits:\n")
	for _, c := range a.Culprit {
		fmt.Printf("    - Target:    %x\n", c.Target)
		fmt.Printf("      Key:       %x\n", c.Key)
		fmt.Printf("      Signature: %x\n", c.Signature)
	}
	fmt.Printf("  Faults:\n")
	for _, f := range a.Fault {
		fmt.Printf("    - Target:    %x\n", f.Target)
		fmt.Printf("      Voting:    %t\n", f.Voting)
		fmt.Printf("      Key:       %x\n", f.Key)
		fmt.Printf("      Signature: %x\n", f.Signature)
	}
}

func (a *Culprit) UnmarshalJSON(data []byte) error {
	var s struct {
		Target    common.Hash `json:"target"`
		Key       string      `json:"key"`
		Signature string      `json:"signature"`
	}
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}
	a.Target = s.Target
	keyBytes := common.FromHex(s.Key)
	copy(a.Key[:], keyBytes)
	signatureBytes := common.FromHex(s.Signature)
	copy(a.Signature[:], signatureBytes)

	return nil
}

type Vote struct {
	Voting    bool             `json:"vote"`      // true for the work report is good, false for the work report is bad
	Index     uint16           `json:"index"`     // validator index
	Signature Ed25519Signature `json:"signature"` // signature of the vote (ByteArray64 in disputes.asn)
}

func (t Vote) DeepCopy() (Vote, error) {
	var copiedVote Vote

	// Serialize the original Dispute to JSON
	data := Encode(t)

	// Deserialize the JSON back into a new Dispute instance
	decoded, _ := Decode(data, reflect.TypeOf(Vote{}))
	copiedVote = decoded.(Vote)
	return copiedVote, nil
}

// func (v *Vote) Bytes() []byte {
// 	enc := Encode(v)
// 	return enc
// }

// func (v *Vote) Hash() common.Hash {
// 	data := v.Bytes()
// 	if data == nil {
// 		// Handle the error case
// 		return common.Hash{}
// 	}
// 	return common.Blake2Hash(data)
// }

func FormDispute(v map[common.Hash]Vote) Dispute {
	//return nil dispute
	_ = v
	return Dispute{}
}

func (a *Vote) UnmarshalJSON(data []byte) error {
	var s struct {
		Voting    bool   `json:"vote"`
		Index     uint16 `json:"index"`
		Signature string `json:"signature"`
	}
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}

	a.Voting = s.Voting
	a.Index = s.Index
	a.Signature = Ed25519Signature(common.FromHex(s.Signature))
	return nil
}

func (a Vote) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Voting    bool   `json:"vote"`
		Index     uint16 `json:"index"`
		Signature string `json:"signature"`
	}{
		Voting:    a.Voting,
		Index:     a.Index,
		Signature: common.HexString(a.Signature[:]),
	})
}

func (v *Vote) Bytes() []byte {
	enc := Encode(v)
	return enc
}

func (v *Vote) Hash() common.Hash {
	data := v.Bytes()
	return common.Blake2Hash(data)
}

func (a *Fault) UnmarshalJSON(data []byte) error {
	var s struct {
		Target    common.Hash `json:"target"`
		Voting    bool        `json:"vote"`
		Key       string      `json:"key"`
		Signature string      `json:"signature"`
	}
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}

	a.Target = s.Target
	a.Voting = s.Voting
	a.Key = Ed25519Key(common.FromHex(s.Key))
	a.Signature = Ed25519Signature(common.FromHex(s.Signature))

	return nil
}

func (a *Culprit) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		Target    common.Hash `json:"target"`
		Key       string      `json:"key"`
		Signature string      `json:"signature"`
	}{
		Target:    a.Target,
		Key:       common.HexString(a.Key[:]),
		Signature: common.HexString(a.Signature[:]),
	})
}

func (a *Fault) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		Target    common.Hash `json:"target"`
		Voting    bool        `json:"vote"`
		Key       string      `json:"key"`
		Signature string      `json:"signature"`
	}{
		Target:    a.Target,
		Voting:    a.Voting,
		Key:       common.HexString(a.Key[:]),
		Signature: common.HexString(a.Signature[:]),
	})
}

func (c *Culprit) Bytes() []byte {
	enc := Encode(c)
	return enc
}

func (c *Culprit) Hash() common.Hash {
	data := c.Bytes()
	return common.Blake2Hash(data)
}

func (f *Fault) Bytes() []byte {
	enc := Encode(f)
	return enc
}

func (f *Fault) Hash() common.Hash {
	data := f.Bytes()
	return common.Blake2Hash(data)
}
