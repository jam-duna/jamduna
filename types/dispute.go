package types

import (
//"github.com/colorfulnotion/jam/common"
)

type VerdictMarker struct {
}

type OffenderMarker struct {
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
	WorkReportHash []byte `json:"target"` // WorkReportHash (ByteArray32 in disputes.asn)
	Epoch          uint32 `json:"age"`    // EpochIndex (U32 in disputes.asn)
	Votes          []Vote `json:"votes"`  // DisputeJudgements
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
