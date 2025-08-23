package types

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"

	"github.com/colorfulnotion/jam/common"
)

/*
11.1.1. Work Report. See Equation 117. A work-report, of the set W, is defined as a tuple of:
* the work-package specification $s$,
* the refinement context $x$,
* the core-index $c$ (i.e. on which the work is done)
* the authorizer hash $a$ and
* output ${\bf o}$ and
* $r$, the results of the evaluation of each of the items in the package r, which is always at least one item and may be no more than I items.
*/
// WorkReport represents a work report.

/*
SegmentRootLookupItem ::= SEQUENCE {
    work-package-hash WorkPackageHash,
    segment-tree-root OpaqueHash
}
*/

type SegmentRootLookupItem struct {
	WorkPackageHash common.Hash `json:"work_package_hash"`
	SegmentRoot     common.Hash `json:"segment_tree_root"`
}

// SegmentRootLookup represents a list of SegmentRootLookupItem
type SegmentRootLookup []SegmentRootLookupItem

type SegmentRootLookupItemHistory struct {
	WorkPackageHash common.Hash `json:"hash"`
	SegmentRoot     common.Hash `json:"exports_root"`
}

// SegmentRootLookup represents a list of SegmentRootLookupItem
type SegmentRootLookupHistory []SegmentRootLookupItemHistory

type WorkReport struct { // 0.7.0 C.27 11.2
	AvailabilitySpec  AvailabilitySpecifier `json:"package_spec"`        // s
	RefineContext     RefineContext         `json:"context"`             // bold-c
	CoreIndex         uint                  `json:"core_index"`          // c
	AuthorizerHash    common.Hash           `json:"authorizer_hash"`     // a
	AuthGasUsed       uint                  `json:"auth_gas_used"`       // g
	Trace             []byte                `json:"auth_output"`         // t -- now called Trace
	SegmentRootLookup SegmentRootLookup     `json:"segment_root_lookup"` // l
	Results           []WorkDigest          `json:"results"`             // d
}

// eq 190
type WorkReportNeedAudit struct {
	Q [TotalCores]WorkReport `json:"available_work_report"`
}

type WorkReportSelection struct {
	WorkReport WorkReport `json:"work_report"`
	Core       uint16     `json:"core_index"`
}

func (a *WorkReport) GetWorkPackageHash() common.Hash {
	return a.AvailabilitySpec.WorkPackageHash
}

// computeWorkReportBytes abstracts the process of generating the bytes to be signed or verified.
func (a *WorkReport) computeWorkReportBytes() []byte {
	return append([]byte(X_G), common.ComputeHash(a.Bytes())...)
}

func ComputeWorkReportSignBytesWithHash(a common.Hash) []byte {
	return append([]byte(X_G), a.Bytes()...)
}

func (a *WorkReport) Sign(Ed25519Secret []byte, validatorIndex uint16) (gc GuaranteeCredential) {
	gc.ValidatorIndex = validatorIndex
	unsign_bytes := a.computeWorkReportBytes()
	sig := ed25519.Sign(Ed25519Secret, unsign_bytes)
	copy(gc.Signature[:], sig[:])
	return gc
}

func (a *WorkReport) ValidateSignature(publicKey []byte, signature []byte) error {
	workReportBytes := a.computeWorkReportBytes()
	if !ed25519.Verify(publicKey, workReportBytes, signature) {
		return errors.New("invalid signature")
	}
	return nil
}

// Bytes returns the bytes of the Assurance
func (a *WorkReport) Bytes() []byte {
	enc, err := Encode(a)
	if err != nil {
		return nil
	}
	return enc
}

func (a *WorkReport) Hash() common.Hash {
	data := a.Bytes()
	if data == nil {
		// Handle the error case
		return common.Hash{}
	}
	return common.Blake2Hash(data)
}

func (a *WorkReport) UnmarshalJSON(data []byte) error {
	var s struct {
		AvailabilitySpec  AvailabilitySpecifier `json:"package_spec"`
		RefineContext     RefineContext         `json:"context"`
		CoreIndex         uint                  `json:"core_index"`
		AuthorizerHash    common.Hash           `json:"authorizer_hash"`
		Trace             string                `json:"auth_output"`
		SegmentRootLookup SegmentRootLookup     `json:"segment_root_lookup"`
		Results           []WorkDigest          `json:"results"`
		AuthGasUsed       uint                  `json:"auth_gas_used"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	a.AvailabilitySpec = s.AvailabilitySpec
	a.RefineContext = s.RefineContext
	a.CoreIndex = s.CoreIndex
	a.AuthorizerHash = s.AuthorizerHash
	a.Trace = common.FromHex(s.Trace)
	a.SegmentRootLookup = s.SegmentRootLookup
	a.Results = s.Results
	a.AuthGasUsed = s.AuthGasUsed
	return nil
}

func (a WorkReport) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		AvailabilitySpec  AvailabilitySpecifier `json:"package_spec"`
		RefineContext     RefineContext         `json:"context"`
		CoreIndex         uint                  `json:"core_index"`
		AuthorizerHash    common.Hash           `json:"authorizer_hash"`
		Trace             string                `json:"auth_output"` // "trace"
		SegmentRootLookup SegmentRootLookup     `json:"segment_root_lookup"`
		Results           []WorkDigest          `json:"results"`
		AuthGasUsed       uint                  `json:"auth_gas_used"`
	}{
		AvailabilitySpec:  a.AvailabilitySpec,
		RefineContext:     a.RefineContext,
		CoreIndex:         a.CoreIndex,
		AuthorizerHash:    a.AuthorizerHash,
		Trace:             common.HexString(a.Trace),
		SegmentRootLookup: a.SegmentRootLookup,
		Results:           a.Results,
		AuthGasUsed:       a.AuthGasUsed,
	})
}

// helper function to print the WorkReport
func (a *WorkReport) String() string {
	return ToJSON(a)
}
