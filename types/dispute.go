package types

import (
	"github.com/colorfulnotion/jam/common"
	"encoding/json"
	"fmt"
)

type VerdictMarker struct {
	WorkReportHash []common.Hash `json:"verdict_mark"` // WorkReportHash (ByteArray32 in disputes.asn)
}

type OffenderMarker struct {
	OffenderKey []PublicKey `json:"offender_mark"`
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
	//TODO: WorkReportHash shoulbe be common.Hash?
	WorkReportHash common.Hash `json:"target"` // WorkReportHash (ByteArray32 in disputes.asn)
	Epoch          uint32      `json:"age"`    // EpochIndex (U32 in disputes.asn)
	Votes          []Vote      `json:"votes"`  // DisputeJudgements
}

type Culprit struct {
	WorkReportHash []byte    `json:"target"`    // WorkReportHash (ByteArray32 in disputes.asn)
	Key            PublicKey `json:"key"`       // Ed25519Key (ByteArray32 in disputes.asn)
	Signature      []byte    `json:"signature"` // Ed25519Signature (ByteArray64 in disputes.asn)
}

type Fault struct {
	WorkReportHash []byte    `json:"target"`    // WorkReportHash (ByteArray32 in disputes.asn)
	Voting         bool      `json:"vote"`      // vote (BOOLEAN in disputes.asn)
	Key            PublicKey `json:"key"`       // Ed25519Key (ByteArray32 in disputes.asn)
	Signature      []byte    `json:"signature"` // Ed25519Signature (ByteArray64 in disputes.asn)
}

type Vote struct {
	Voting    bool   `json:"vote"`      // true for guilty, false for innocent
	Index     uint16 `json:"index"`     // index of the vote in the list of votes (U16 in disputes.asn)
	Signature []byte `json:"signature"` // signature of the vote (ByteArray64 in disputes.asn)
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
