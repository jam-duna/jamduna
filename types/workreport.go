package types

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"

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

func (m *SegmentRootLookup) UnmarshalJSON(data []byte) error {
	type SegmentRootLookupItemAlias struct {
		WorkPackageHash *common.Hash `json:"work_package_hash"`
		Hash            *common.Hash `json:"hash"`
		SegmentTreeRoot *common.Hash `json:"segment_tree_root"`
		ExportsRoot     *common.Hash `json:"exports_root"`
	}

	var temp []SegmentRootLookupItemAlias
	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}

	*m = make(SegmentRootLookup, len(temp))
	for i, item := range temp {

		if item.WorkPackageHash != nil {
			(*m)[i].WorkPackageHash = *item.WorkPackageHash
		} else if item.Hash != nil {
			(*m)[i].WorkPackageHash = *item.Hash
		} else {
			return fmt.Errorf("missing both work_package_hash and hash fields at index %d", i)
		}

		if item.SegmentTreeRoot != nil {
			(*m)[i].SegmentRoot = *item.SegmentTreeRoot
		} else if item.ExportsRoot != nil {
			(*m)[i].SegmentRoot = *item.ExportsRoot
		} else {
			return fmt.Errorf("missing both segment_tree_root and exports_root fields at index %d", i)
		}
	}

	return nil
}

// MarshalJSON serializes the SegmentRootLookup into JSON
func (s SegmentRootLookup) MarshalJSON() ([]byte, error) {
	return json.Marshal([]SegmentRootLookupItem(s))
}

// // UnmarshalJSON deserializes JSON data into SegmentRootLookup
// func (s *SegmentRootLookup) UnmarshalJSON(data []byte) error {
// 	var items []SegmentRootLookupItem
// 	if err := json.Unmarshal(data, &items); err != nil {
// 		return err
// 	}
// 	*s = items
// 	return nil
// }

type WorkReport struct {
	AvailabilitySpec  AvailabilitySpecifier `json:"package_spec"`
	RefineContext     RefineContext         `json:"context"`
	CoreIndex         uint16                `json:"core_index"`
	AuthorizerHash    common.Hash           `json:"authorizer_hash"`
	AuthOutput        []byte                `json:"auth_output"`
	SegmentRootLookup SegmentRootLookup     `json:"segment_root_lookup"`
	Results           []WorkResult          `json:"results"`
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
		CoreIndex         uint16                `json:"core_index"`
		AuthorizerHash    common.Hash           `json:"authorizer_hash"`
		AuthOutput        string                `json:"auth_output"`
		SegmentRootLookup SegmentRootLookup     `json:"segment_root_lookup"`
		Results           []WorkResult          `json:"results"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	a.AvailabilitySpec = s.AvailabilitySpec
	a.RefineContext = s.RefineContext
	a.CoreIndex = s.CoreIndex
	a.AuthorizerHash = s.AuthorizerHash
	a.AuthOutput = common.FromHex(s.AuthOutput)
	a.SegmentRootLookup = s.SegmentRootLookup
	a.Results = s.Results
	return nil
}

func (a WorkReport) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		AvailabilitySpec  AvailabilitySpecifier `json:"package_spec"`
		RefineContext     RefineContext         `json:"context"`
		CoreIndex         uint16                `json:"core_index"`
		AuthorizerHash    common.Hash           `json:"authorizer_hash"`
		AuthOutput        string                `json:"auth_output"`
		SegmentRootLookup SegmentRootLookup     `json:"segment_root_lookup"`
		Results           []WorkResult          `json:"results"`
	}{
		AvailabilitySpec:  a.AvailabilitySpec,
		RefineContext:     a.RefineContext,
		CoreIndex:         a.CoreIndex,
		AuthorizerHash:    a.AuthorizerHash,
		AuthOutput:        common.HexString(a.AuthOutput),
		SegmentRootLookup: a.SegmentRootLookup,
		Results:           a.Results,
	})
}

// helper function to print the WorkReport
func (a *WorkReport) String() string {
	enc, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		// Handle the error according to your needs.
		return fmt.Sprintf("Error marshaling JSON: %v", err)
	}
	return string(enc)
}
