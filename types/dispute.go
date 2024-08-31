package types

import (
	"encoding/json"
	"fmt"
	"github.com/colorfulnotion/jam/common"
)

type VerdictMarker struct {
	WorkReportHash []common.Hash `json:"verdict_mark"` // WorkReportHash (ByteArray32 in disputes.asn)
}

type OffenderMarker struct {
	OffenderKey []PublicKey `json:"offender_mark"`
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

type SDispute struct {
	Verdict []SVerdict `json:"verdicts"`
	Culprit []SCulprit `json:"culprits"`
	Fault   []SFault   `json:"faults"`
}

type Verdict struct {
	Target common.Hash `json:"target"` // WorkReportHash (ByteArray32 in disputes.asn)
	Epoch  uint32      `json:"age"`    // EpochIndex (U32 in disputes.asn)
	Votes  []Vote      `json:"votes"`  // DisputeJudgements
}

type SVerdict struct {
	Target common.Hash `json:"target"` // WorkReportHash (ByteArray32 in disputes.asn)
	Epoch  uint32      `json:"age"`    // EpochIndex (U32 in disputes.asn)
	Votes  []SVote     `json:"votes"`  // DisputeJudgements
}

type Culprit struct {
	Target    common.Hash `json:"target"`    // WorkReportHash (ByteArray32 in disputes.asn)
	Key       PublicKey   `json:"key"`       // Ed25519Key (ByteArray32 in disputes.asn)
	Signature []byte      `json:"signature"` // Ed25519Signature (ByteArray64 in disputes.asn)
}

type SCulprit struct {
	Target    common.Hash `json:"target"`    // WorkReportHash (ByteArray32 in disputes.asn)
	Key       string      `json:"key"`       // Ed25519Key (ByteArray32 in disputes.asn)
	Signature string      `json:"signature"` // Ed25519Signature (ByteArray64 in disputes.asn)
}

type Fault struct {
	WorkReportHash common.Hash `json:"target"`    // WorkReportHash (ByteArray32 in disputes.asn)
	Voting         bool        `json:"vote"`      // vote (BOOLEAN in disputes.asn)
	Key            PublicKey   `json:"key"`       // Ed25519Key (ByteArray32 in disputes.asn)
	Signature      []byte      `json:"signature"` // Ed25519Signature (ByteArray64 in disputes.asn)
}

type SFault struct {
	WorkReportHash common.Hash `json:"target"`    // WorkReportHash (ByteArray32 in disputes.asn)
	Voting         bool        `json:"vote"`      // vote (BOOLEAN in disputes.asn)
	Key            string      `json:"key"`       // Ed25519Key (ByteArray32 in disputes.asn)
	Signature      string      `json:"signature"` // Ed25519Signature (ByteArray64 in disputes.asn)
}

func (t Dispute) DeepCopy() (Dispute, error) {
	var copiedDispute Dispute

	// Serialize the original Dispute to JSON
	data, err := json.Marshal(t)
	if err != nil {
		return copiedDispute, err
	}

	// Deserialize the JSON back into a new Dispute instance
	err = json.Unmarshal(data, &copiedDispute)
	if err != nil {
		return copiedDispute, err
	}

	return copiedDispute, nil
}

// Bytes returns the bytes of the Dispute
func (a *Dispute) Bytes() []byte {
	enc, err := json.Marshal(a)
	if err != nil {
		// Handle the error according to your needs.
		fmt.Println("Error marshaling JSON:", err)
		return nil
	}
	return enc
}

func (a *Dispute) Hash() common.Hash {
	data := a.Bytes()
	if data == nil {
		// Handle the error case
		return common.Hash{}
	}
	return common.BytesToHash(common.ComputeHash(data))
}

func (s *SFault) Deserialize() (Fault, error) {
	keyBytes := common.FromHex(s.Key)
	var key PublicKey
	copy(key[:], keyBytes)

	signature := common.FromHex(s.Signature)

	return Fault{
		WorkReportHash: s.WorkReportHash,
		Voting:         s.Voting,
		Key:            key,
		Signature:      signature,
	}, nil
}

func (s *SCulprit) Deserialize() (Culprit, error) {
	keyBytes := common.FromHex(s.Key)
	var key PublicKey
	copy(key[:], keyBytes)

	return Culprit{
		Target:    s.Target,
		Key:       key,
		Signature: common.FromHex(s.Signature),
	}, nil
}

func (s *SVerdict) Deserialize() (Verdict, error) {
	votes := make([]Vote, len(s.Votes))
	for i, sv := range s.Votes {
		vote, err := sv.Deserialize()
		if err != nil {
			return Verdict{}, err
		}
		votes[i] = vote
	}

	return Verdict{
		Target: s.Target,
		Epoch:  s.Epoch,
		Votes:  votes,
	}, nil
}

func (s *SDispute) Deserialize() (Dispute, error) {
	verdicts := make([]Verdict, len(s.Verdict))
	for i, sv := range s.Verdict {
		v, err := sv.Deserialize()
		if err != nil {
			return Dispute{}, err
		}
		verdicts[i] = v
	}

	culprits := make([]Culprit, len(s.Culprit))
	for i, sc := range s.Culprit {
		c, err := sc.Deserialize()
		if err != nil {
			return Dispute{}, err
		}
		culprits[i] = c
	}

	faults := make([]Fault, len(s.Fault))
	for i, sf := range s.Fault {
		f, err := sf.Deserialize()
		if err != nil {
			return Dispute{}, err
		}
		faults[i] = f
	}

	return Dispute{
		Verdict: verdicts,
		Culprit: culprits,
		Fault:   faults,
	}, nil
}

func (s *SOffenderMarker) Deserialize() (*OffenderMarker, error) {
	keys := make([]PublicKey, len(s.OffenderKey))
	for i, keyStr := range s.OffenderKey {
		keyBytes := common.FromHex(keyStr)
		copy(keys[i][:], keyBytes)
	}

	return &OffenderMarker{
		OffenderKey: keys,
	}, nil
}
